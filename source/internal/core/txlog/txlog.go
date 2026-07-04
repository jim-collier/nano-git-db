// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// Package txlog is the git-backed CSV transaction log - the source of truth the
// SQLite view is rebuilt from.
//
// The log is append-only and field-granular: each entry records one write. The
// SQLite database is a derived view, produced by replaying entries in order
// (Apply). Git sync and garbage collection are separate concerns.
//
// Note: the original design's field list omits a row identifier, but field-level
// ops can't be applied without one, so RowID is part of every entry (a hex GUID
// of the affected row).
package txlog

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jim-collier/nano-git-db/internal/core/store"
)

// Entry is one transaction-log row. Field is blank for record-level ops
// (create with no value, mark_delete, delete).
type Entry struct {
	TxID     string // per-entry GUID (hex or base64)
	Date     string // GMT, RFC3339
	Table    string
	RowID    string // hex GUID of the affected row
	Field    string
	Op       string // create, update, mark_delete, delete
	NewValue string
	IsNull   bool // set the field to SQL NULL; NewValue is ignored
	UserID   string
	GCReady  bool   // ok_to_garbage_collect
	HostName string // machine that wrote the entry; one user can span hosts
	// Enc marks NewValue as an encrypted token (crypt-layer base64), stored
	// behind encMarker. The txlog itself does no crypto - it only carries the
	// flag and the opaque payload; the crud/crypt layer seals and opens it.
	Enc bool
}

var header = []string{
	"tx_id", "date", "table_name", "row_id", "field_name",
	"operation", "new_value", "user_id", "ok_to_garbage_collect", "host_name",
}

// legacyFields is the record width before host_name was appended. Logs written
// by older clients (and union-merged alongside new ones) still parse: host_name
// stays empty for them. host_name is appended last so every other column keeps
// its index across both widths.
var legacyFields = len(header) - 1

// Log is an append-only CSV transaction log in a directory.
type Log struct {
	path string
}

// Open attaches to (creating if needed) the log directory; the log file is
// txlog.csv within it.
func Open(dir string) (*Log, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Log{path: filepath.Join(dir, "txlog.csv")}, nil
}

// Path returns the CSV file path.
func (l *Log) Path() string { return l.path }

// Dir returns the log directory - the sync/attachments base path.
func (l *Log) Dir() string { return filepath.Dir(l.path) }

// Append writes entries to the end of the log, adding the header to a new file.
// The batch is rendered in memory and lands as a single write(2) on an O_APPEND
// handle, so concurrent appenders (another process, the future web server)
// cannot interleave partial records on a local filesystem.
func (l *Log) Append(entries ...Entry) error {
	_, statErr := os.Stat(l.path)
	newFile := errors.Is(statErr, os.ErrNotExist)

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if newFile {
		if err := w.Write(header); err != nil {
			return err
		}
	}
	for _, e := range entries {
		if err := w.Write(e.record()); err != nil {
			return err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}

	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(buf.Bytes())
	return err
}

// ReadAll returns every entry - retired GC segments first, then the live
// file - plus a warning per line it had to skip. A missing log is not an
// error.
func (l *Log) ReadAll() ([]Entry, []string, error) {
	var out []Entry
	var warns []string
	for _, p := range l.files() {
		e, w, err := readLogFile(p)
		warns = append(warns, w...)
		if err != nil {
			return out, warns, err
		}
		out = append(out, e...)
	}
	return out, warns, nil
}

// readLogFile reads one CSV part. Deliberately forgiving: the log is the
// single source of truth, so one mangled line (a torn union merge, a stray
// edit) must not make the whole log unreadable. Malformed records skip with
// a warning; repeated header rows (a union merge can leave extras) skip
// silently; only I/O errors abort.
func readLogFile(path string) ([]Entry, []string, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	var out []Entry
	var warns []string
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		var perr *csv.ParseError
		if errors.As(err, &perr) {
			warns = append(warns, fmt.Sprintf(
				"log %s line %d: unreadable, skipped (%v)", path, perr.Line, perr.Err))
			continue // one bad line must not take the log hostage
		}
		if err != nil {
			return nil, warns, err
		}
		if rec[0] == header[0] {
			continue // a header row
		}
		if len(rec) != len(header) && len(rec) != legacyFields {
			line, _ := r.FieldPos(0)
			warns = append(warns, fmt.Sprintf(
				"log %s line %d: torn record (%d of %d fields), skipped", path, line, len(rec), len(header)))
			continue
		}
		out = append(out, entryFrom(rec))
	}
	return out, warns, nil
}

func (e Entry) record() []string {
	var val string
	switch {
	case e.IsNull:
		val = nullMarker
	case e.Enc:
		val = encMarker + e.NewValue // token is line-safe base64; stored raw
	default:
		val = encodeSym(e.NewValue)
	}
	return []string{
		e.TxID, e.Date, encodeSym(e.Table), e.RowID, encodeSym(e.Field),
		e.Op, val, encodeSym(e.UserID), strconv.FormatBool(e.GCReady), encodeSym(e.HostName),
	}
}

func entryFrom(rec []string) Entry {
	gc, _ := strconv.ParseBool(rec[8])
	e := Entry{
		TxID: rec[0], Date: rec[1], Table: decodeSym(rec[2]), RowID: rec[3],
		Field: decodeSym(rec[4]), Op: rec[5], UserID: decodeSym(rec[7]), GCReady: gc,
	}
	if len(rec) > legacyFields { // new-format record carries host_name last
		e.HostName = decodeSym(rec[9])
	}
	// The NULL and ENC markers are checked on the raw text: a user literal
	// 🗦NULL🗧/🗦ENC🗧 was encoded with escaped delimiters, so it cannot collide.
	switch {
	case rec[6] == nullMarker:
		e.IsNull = true
	case strings.HasPrefix(rec[6], encMarker):
		e.Enc = true
		e.NewValue = rec[6][len(encMarker):] // opaque token; crypt opens it later
	default:
		e.NewValue = decodeSym(rec[6])
	}
	return e
}

// Apply replays entries into the store's SQLite view, in one transaction. The
// view is derived, so this is how the log's truth reaches the queryable copy.
//
// Entries are applied in canonical (date, tx_id) order, not file order. That is
// what makes the view converge across clients: a git union-merge of two clients'
// appends can interleave lines arbitrarily, but every client sorts to the same
// total order before applying.
//
// Replay is the most forgiving code in the tree - the log must always be able
// to rebuild a view. Entries that cannot apply (schema drift, unique collisions
// from merged concurrent writes) are skipped and reported as warnings, never
// fatal; every client skips the same losers because the order is total. A
// hard-deleted row stays dead: later update/mark_delete entries for it are
// skipped rather than resurrecting a ghost via the upsert path (only a later
// create revives the row id).
func Apply(st *store.Store, entries []Entry) ([]string, error) {
	ordered := make([]Entry, len(entries))
	copy(ordered, entries)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Date != ordered[j].Date {
			return ordered[i].Date < ordered[j].Date
		}
		return ordered[i].TxID < ordered[j].TxID
	})

	tx, err := st.DB().Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() // no-op after a successful Commit

	var warns []string
	dead := map[string]bool{} // (table, row) pairs hard-deleted so far
	for _, e := range ordered {
		key := e.Table + "\x00" + e.RowID
		switch e.Op {
		case "create":
			delete(dead, key)
		case "delete":
			dead[key] = true
		default:
			if dead[key] {
				continue
			}
		}
		if err := applyOne(tx, e); err != nil {
			if skippable(err) {
				warns = append(warns, fmt.Sprintf("tx %s (%s/%s) skipped: %v", e.TxID, e.Table, e.Op, err))
				continue
			}
			return warns, fmt.Errorf("tx %s (%s/%s): %w", e.TxID, e.Table, e.Op, err)
		}
	}
	return warns, tx.Commit()
}

// errBadEntry tags data errors local to one log entry (mangled row id, an
// operation this build does not know - possibly written by a newer client).
// They skip with a warning like schema drift does: one bad or foreign entry
// must never stop the whole log from rebuilding the view.
var errBadEntry = errors.New("bad log entry")

// skippable classifies per-entry errors replay tolerates: unknown tables or
// columns (the design says DDL drift is ignored, and Build only migrates
// additively), unique violations (two clients created the same natural key;
// the deterministic order picks the same winner everywhere), and entry-local
// data errors. SQLite rolls back only the failed statement, not the enclosing
// transaction.
func skippable(err error) bool {
	if errors.Is(err, errBadEntry) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "no such table") ||
		strings.Contains(msg, "no such column") ||
		strings.Contains(msg, "UNIQUE constraint failed")
}

func applyOne(tx *sql.Tx, e Entry) error {
	id, err := hex.DecodeString(e.RowID)
	if err != nil {
		return fmt.Errorf("%w: bad row_id %q: %v", errBadEntry, e.RowID, err)
	}
	tbl := quoteIdent(e.Table)

	switch e.Op {
	case "create":
		if _, err := tx.Exec(`INSERT OR IGNORE INTO `+tbl+` ("id") VALUES (?)`, id); err != nil {
			return err
		}
		if e.Field != "" {
			return setField(tx, tbl, e, id)
		}
		return nil
	case "update":
		// Upsert: replay must tolerate the create entry having been GC'd.
		if _, err := tx.Exec(`INSERT OR IGNORE INTO `+tbl+` ("id") VALUES (?)`, id); err != nil {
			return err
		}
		return setField(tx, tbl, e, id)
	case "mark_delete":
		_, err := tx.Exec(`UPDATE `+tbl+` SET "is_deleted"=1 WHERE "id"=?`, id)
		return err
	case "delete":
		_, err := tx.Exec(`DELETE FROM `+tbl+` WHERE "id"=?`, id)
		return err
	default:
		return fmt.Errorf("%w: unknown operation %q", errBadEntry, e.Op)
	}
}

// setField writes one column. Values bind as text (SQLite coerces by column
// affinity), or as NULL when the entry carries the 🗦NULL🗧 sentinel. A still-
// encrypted entry (Enc: the decrypt pass had no key) also binds NULL - the view
// must never hold ciphertext.
func setField(tx *sql.Tx, quotedTable string, e Entry, id []byte) error {
	var val any = e.NewValue
	if e.IsNull || e.Enc {
		val = nil
	}
	_, err := tx.Exec(`UPDATE `+quotedTable+` SET `+quoteIdent(e.Field)+`=? WHERE "id"=?`, val, id)
	return err
}

func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
