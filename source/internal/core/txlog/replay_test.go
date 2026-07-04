// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package txlog

import (
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/store"
)

func newView(t *testing.T, src string) *store.Store {
	t.Helper()
	schema, err := ddl.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "v.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Build(schema); err != nil {
		t.Fatal(err)
	}
	return st
}

const uniqueDDL = "tables:\n" +
	"\ttable: person\n" +
	"\t\tfields:\n" +
	"\t\t\tfield: name\n" +
	"\t\t\t\ttype: string\n" +
	"\t\tuniques:\n" +
	"\t\t\tname\n"

// Two soft-deletes of the same natural key used to collide on the unique index
// and abort the entire replay; partial indexes must make this a non-event.
func TestReplaySurvivesRepeatedSoftDelete(t *testing.T) {
	st := newView(t, uniqueDDL)
	entries := []Entry{
		{Date: "1", TxID: "a", Op: "create", Table: "person", RowID: "01", Field: "name", NewValue: "Ann"},
		{Date: "2", TxID: "b", Op: "mark_delete", Table: "person", RowID: "01"},
		{Date: "3", TxID: "c", Op: "create", Table: "person", RowID: "02", Field: "name", NewValue: "Ann"},
		{Date: "4", TxID: "d", Op: "mark_delete", Table: "person", RowID: "02"},
		{Date: "5", TxID: "e", Op: "create", Table: "person", RowID: "03", Field: "name", NewValue: "Ann"},
	}
	warns, err := Apply(st, entries)
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
	var live, dead int
	if err := st.DB().QueryRow(
		`SELECT SUM("is_deleted"=0), SUM("is_deleted"=1) FROM "person"`).Scan(&live, &dead); err != nil {
		t.Fatal(err)
	}
	if live != 1 || dead != 2 {
		t.Fatalf("live=%d dead=%d, want 1 live + 2 dead", live, dead)
	}
}

// Entries against tables or columns the local schema doesn't know are skipped
// with a warning, never fatal - the log may be newer than the DDL.
func TestReplaySkipsSchemaDrift(t *testing.T) {
	st := newView(t, uniqueDDL)
	entries := []Entry{
		{Date: "1", TxID: "a", Op: "create", Table: "person", RowID: "01", Field: "name", NewValue: "Ann"},
		{Date: "2", TxID: "b", Op: "update", Table: "person", RowID: "01", Field: "no_such_field", NewValue: "x"},
		{Date: "3", TxID: "c", Op: "create", Table: "no_such_table", RowID: "02", Field: "f", NewValue: "y"},
		{Date: "4", TxID: "d", Op: "update", Table: "person", RowID: "01", Field: "name", NewValue: "Anne"},
	}
	warns, err := Apply(st, entries)
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if len(warns) != 2 {
		t.Fatalf("warnings = %v, want 2", warns)
	}
	id, _ := hex.DecodeString("01")
	var name string
	if err := st.DB().QueryRow(`SELECT "name" FROM "person" WHERE "id"=?`, id).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Anne" {
		t.Fatalf("name = %q; good entries after a skipped one must still apply", name)
	}
}

// Concurrent creates of the same natural key merge deterministically: the
// later one is skipped everywhere, not fatal anywhere.
func TestReplaySkipsUniqueLoser(t *testing.T) {
	st := newView(t, uniqueDDL)
	entries := []Entry{
		{Date: "1", TxID: "a", Op: "create", Table: "person", RowID: "01", Field: "name", NewValue: "Ann"},
		{Date: "2", TxID: "b", Op: "create", Table: "person", RowID: "02", Field: "name", NewValue: "Ann"},
	}
	warns, err := Apply(st, entries)
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "UNIQUE") {
		t.Fatalf("warnings = %v, want one unique-loser skip", warns)
	}
}

// Entry-local data errors - a mangled row id, an operation this build does
// not know (possibly a newer client's) - skip with a warning like schema
// drift; they must never abort the whole replay.
func TestReplaySkipsBadEntries(t *testing.T) {
	st := newView(t, uniqueDDL)
	entries := []Entry{
		{Date: "1", TxID: "a", Op: "create", Table: "person", RowID: "01", Field: "name", NewValue: "Ann"},
		{Date: "2", TxID: "b", Op: "frobnicate", Table: "person", RowID: "01"},
		{Date: "3", TxID: "c", Op: "update", Table: "person", RowID: "zz", Field: "name", NewValue: "x"},
		{Date: "4", TxID: "d", Op: "update", Table: "person", RowID: "01", Field: "name", NewValue: "Anne"},
	}
	warns, err := Apply(st, entries)
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if len(warns) != 2 {
		t.Fatalf("warnings = %v, want 2", warns)
	}
	id, _ := hex.DecodeString("01")
	var name string
	if err := st.DB().QueryRow(`SELECT "name" FROM "person" WHERE "id"=?`, id).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Anne" {
		t.Fatalf("name = %q; good entries after skipped ones must still apply", name)
	}
}

// A hard-deleted row must stay dead: later update entries (e.g. surviving a
// future GC of the delete's own creates) must not resurrect a one-field ghost.
func TestReplayDeleteWinsOverLaterUpdates(t *testing.T) {
	st := newView(t, uniqueDDL)
	entries := []Entry{
		{Date: "1", TxID: "a", Op: "create", Table: "person", RowID: "01", Field: "name", NewValue: "Ann"},
		{Date: "2", TxID: "b", Op: "delete", Table: "person", RowID: "01"},
		{Date: "3", TxID: "c", Op: "update", Table: "person", RowID: "01", Field: "name", NewValue: "Ghost"},
		{Date: "4", TxID: "d", Op: "mark_delete", Table: "person", RowID: "01"},
	}
	if _, err := Apply(st, entries); err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	var n int
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM "person"`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d row(s) after hard delete, want 0", n)
	}

	// ...but a later create legitimately reuses the row id.
	if _, err := Apply(st, append(entries,
		Entry{Date: "5", TxID: "e", Op: "create", Table: "person", RowID: "01", Field: "name", NewValue: "Reborn"},
	)); err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM "person"`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("%d row(s) after re-create, want 1", n)
	}
}
