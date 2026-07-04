// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// Package cli is the arg-based CRUD front-end. Every data verb takes the same
// <ddl> <sqlite> <logdir> triple - stateless and script-friendly until the
// planned config file lands. Writes stamp the user from NANOGITDB_USER, else
// the OS username.
package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jim-collier/nano-git-db/enc"
	"github.com/jim-collier/nano-git-db/internal/core/config"
	"github.com/jim-collier/nano-git-db/internal/core/crud"
	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/schema"
	"github.com/jim-collier/nano-git-db/internal/core/store"
	"github.com/jim-collier/nano-git-db/internal/core/txlog"
	"github.com/jim-collier/nano-git-db/script"
)

// Run is the CLI entry point.
func Run(args []string) error {
	switch {
	case len(args) >= 2 && args[0] == "ddl":
		return dumpDDL(args[1])
	case len(args) >= 3 && args[0] == "build":
		return buildDB(args[1], args[2])
	case len(args) >= 4 && args[0] == "replay":
		return replay(args[1], args[2], args[3])
	case len(args) >= 4 && args[0] == "sync":
		return syncAndReplay(args[1], args[2], args[3])
	case len(args) >= 2 && args[0] == "sync":
		return syncLog(args[1])
	case len(args) >= 3 && args[0] == "gc":
		return gcLog(args[1], args[2])
	case len(args) >= 6 && args[0] == "create":
		return crudCreate(args[1:4], args[4], args[5:])
	case len(args) >= 6 && args[0] == "get":
		return crudGet(args[1:4], args[4], args[5])
	case len(args) >= 7 && args[0] == "update":
		return crudUpdate(args[1:4], args[4], args[5], args[6:])
	case len(args) >= 7 && args[0] == "setnull":
		return crudSetNull(args[1:4], args[4], args[5], args[6])
	case len(args) >= 6 && (args[0] == "markdelete" || args[0] == "delete"):
		return crudDelete(args[0], args[1:4], args[4], args[5])
	case len(args) >= 5 && args[0] == "query":
		return crudQuery(args[1:4], args[4])
	case len(args) >= 7 && args[0] == "comment":
		return crudComment(args[1:4], args[4], args[5], strings.Join(args[6:], " "))
	case len(args) >= 6 && args[0] == "comments":
		return crudComments(args[1:4], args[4], args[5])
	case len(args) >= 7 && args[0] == "attachuri":
		return crudAttach(args[1:4], args[4], args[5], args[6], strings.Join(args[7:], " "), false)
	case len(args) >= 7 && args[0] == "attachfile":
		return crudAttach(args[1:4], args[4], args[5], args[6], strings.Join(args[7:], " "), true)
	case len(args) >= 6 && args[0] == "attachments":
		return crudAttachments(args[1:4], args[4], args[5])
	case len(args) >= 5 && args[0] == "--rename-table":
		return renameTable(args[1], args[2], args[3], args[4])
	case len(args) >= 6 && args[0] == "--rename-field":
		return renameField(args[1], args[2], args[3], args[4], args[5])
	}
	fmt.Println("usage: ngdb <verb> ...")
	fmt.Println("  setup and run modes:")
	fmt.Println("    --init [repo-or-dir]           register the $PWD .ddl as a database")
	fmt.Println("    --config <dir> ...             use an alternate registry dir, then run")
	if enc.Available() { // enterprise build only
		fmt.Println("    --encrypt[=on|off|auto] ...    set the local encryption preference,")
		fmt.Println("                                   then run (--init --encrypt=on mints a key)")
	}
	fmt.Println("    --tui [<ddl> <sqlite> <dir>]   terminal UI (no paths: pick/create a db)")
	fmt.Println("    --serve [<ddl> <sqlite> <dir>] local web UI on 127.0.0.1:8765")
	fmt.Println("    --script <f.lua> <ddl> <sqlite> <dir>   run a Lua script")
	fmt.Println("    --version, -v                  print the version and exit")
	fmt.Println("  schema and log:")
	fmt.Println("    ddl <file>                     parse a DDL and print a summary")
	fmt.Println("    build <ddl> <sqlite>           build/migrate a SQLite view from a DDL")
	fmt.Println("    replay <ddl> <sqlite> <dir>    rebuild the view from a tx-log dir")
	fmt.Println("    sync <logdir>                  commit + pull/push the tx-log via git")
	fmt.Println("    sync <ddl> <sqlite> <dir>      sync, then migrate the view and replay")
	fmt.Println("    gc <ddl> <logdir>              collect entries of long-deleted rows")
	fmt.Println("                                   (gc_age_days tunable, default 90)")
	fmt.Println("  data (all take the same <ddl> <sqlite> <logdir> triple, then):")
	fmt.Println("    create ... <table> f=v [f=v...]        insert a row; prints its id")
	fmt.Println("    get ... <table> <id>                   print one row")
	fmt.Println("    update ... <table> <id> f=v [f=v...]   set fields")
	fmt.Println("    setnull ... <table> <id> <field>       set a field to SQL NULL")
	fmt.Println("    markdelete ... <table> <id>            soft-delete")
	fmt.Println("    delete ... <table> <id>                hard-delete")
	fmt.Println("    query ... <sql>                        read-only SQL against the view")
	fmt.Println("  schema ops (rewrite the DDL file and the SQLite view; the old name")
	fmt.Println("  becomes an alias so existing tx-log entries still replay):")
	fmt.Println("    --rename-table <ddl> <sqlite> <old> <new>")
	fmt.Println("    --rename-field <ddl> <sqlite> <table> <old> <new>")
	fmt.Println("  opt-in features (table must enable them in its DDL features: block):")
	fmt.Println("    comment ... <table> <id> <text>            add a comment to a row")
	fmt.Println("    comments ... <table> <id>                  list a row's comments")
	fmt.Println("    attachuri ... <table> <id> <uri> [desc]    attach a link-in-place URI")
	fmt.Println("    attachfile ... <table> <id> <path> [desc]  copy a file in and attach it")
	fmt.Println("    attachments ... <table> <id>               list a row's attachments")
	fmt.Println("  writes are stamped with NANOGITDB_USER (default: the OS username)")
	return nil
}

// dumpDDL parses a DDL file and prints a short summary - a stand-in until the
// real CRUD CLI lands, and a handy parser smoke test.
func dumpDDL(path string) error {
	s, err := ddl.ParseFile(path)
	if err != nil {
		return err
	}
	fmt.Printf("tables=%d relationships=%d views=%d\n", len(s.Tables), len(s.Relations), len(s.Views))
	for _, t := range s.Tables {
		fmt.Printf("  table %q: fields=%d uniques=%d indexes=%d\n",
			t.Name, len(t.Fields), len(t.Uniques), len(t.Indexes))
	}
	for _, w := range s.Warnings {
		fmt.Println("  warning:", w)
	}
	return nil
}

// buildDB parses a DDL and builds (or updates) the SQLite view at dbPath.
func buildDB(ddlPath, dbPath string) error {
	s, err := ddl.ParseFile(ddlPath)
	if err != nil {
		return err
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.Build(s); err != nil {
		return err
	}
	bw, err := schema.Bootstrap(st, s)
	if err != nil {
		return err
	}
	fmt.Printf("built %d table(s) plus built-ins into %s\n", len(s.Tables), dbPath)
	for _, w := range append(s.Warnings, bw...) {
		fmt.Println("  warning:", w)
	}
	fmt.Println("  note: default groups get seeded by replay/sync (they are log-first)")
	return nil
}

// replay rebuilds the SQLite view from a tx-log dir: build schema, then apply
// every logged entry. Demonstrates the core thesis - the log is the truth, the
// SQLite file is a derived, rebuildable view.
func replay(ddlPath, dbPath, logDir string) error {
	s, err := ddl.ParseFile(ddlPath)
	if err != nil {
		return err
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.Build(s); err != nil {
		return err
	}
	bw, err := schema.Bootstrap(st, s)
	if err != nil {
		return err
	}
	lg, err := txlog.Open(logDir)
	if err != nil {
		return err
	}
	entries, rw, err := lg.ReadAll()
	if err != nil {
		return err
	}
	if bs, err := schema.Builtins(); err == nil {
		schema.ApplyAliases(entries, s, bs) // pre-rename entries -> current names
	}
	// Decrypt field values before applying, using this DDL's registered key
	// (else a key beside the DDL); unreadable ones stay empty in the view.
	keyFile, _ := config.ResolveEncryptionForDDL(ddlPath)
	cipher, _, err := enc.LoadCipher(keyFile)
	if err != nil {
		return err
	}
	if n := crud.DecryptEntries(entries, cipher); n > 0 {
		fmt.Printf("  warning: %d encrypted value(s) unreadable (no/wrong key); shown empty\n", n)
	}
	warns, err := txlog.Apply(st, entries)
	if err != nil {
		return err
	}
	warns = append(rw, warns...)
	// Seed AFTER the replay: an already-seeded log has just filled groups, so
	// this is a no-op everywhere but on a brand-new database.
	api := crud.New(st, lg)
	api.UserID = "system"
	if err := schema.SeedDefaults(api); err != nil {
		return err
	}
	fmt.Printf("applied %d log entr%s into %s\n", len(entries), plural(len(entries)), dbPath)
	for _, w := range append(bw, warns...) {
		fmt.Println("  warning:", w)
	}
	for _, t := range s.Tables {
		var live int
		if err := st.DB().QueryRow(
			`SELECT COUNT(*) FROM ` + quoteIdent(t.Name) + ` WHERE "is_deleted"=0`).Scan(&live); err == nil {
			fmt.Printf("  %s: %d live row(s)\n", t.Name, live)
		}
	}
	return nil
}

func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

// syncLog runs one git sync pass over a tx-log directory.
func syncLog(dir string) error {
	lg, err := txlog.Open(dir)
	if err != nil {
		return err
	}
	res, err := txlog.NewSyncer(lg, time.Minute).Sync()
	if err != nil {
		return err
	}
	switch {
	case res.Skipped:
		fmt.Println("not a git repo; nothing to sync (local logging still works)")
	case res.NoRemote && res.Committed:
		fmt.Println("committed locally; no remote configured")
	case res.NoRemote:
		fmt.Println("up to date; no remote configured")
	default:
		fmt.Printf("synced: committed=%v pulled=%v pushed=%v new=%v\n",
			res.Committed, res.Pulled, res.Pushed, res.Changed)
	}
	return nil
}

// syncAndReplay is the full client cycle: reconcile the log with the remote
// first (the DDL may live in the same repo and change too), then migrate the
// view schema additively, then rebuild the view from the merged log.
func syncAndReplay(ddlPath, dbPath, logDir string) error {
	if err := syncLog(logDir); err != nil {
		return err
	}
	return replay(ddlPath, dbPath, logDir)
}

// openAPI wires up the CRUD API for one data verb: parse the DDL, open and
// migrate the view (build + built-ins are idempotent, so the schema is always
// current), open the log. It does NOT replay - keeping the view current across
// clients is what replay/sync are for. Callers must Close the store.
func openAPI(paths []string) (*store.Store, *crud.API, error) {
	s, err := ddl.ParseFile(paths[0])
	if err != nil {
		return nil, nil, err
	}
	st, err := store.Open(paths[1])
	if err != nil {
		return nil, nil, err
	}
	if err := st.Build(s); err != nil {
		st.Close()
		return nil, nil, err
	}
	if _, err := schema.Bootstrap(st, s); err != nil {
		st.Close()
		return nil, nil, err
	}
	lg, err := txlog.Open(paths[2])
	if err != nil {
		st.Close()
		return nil, nil, err
	}
	api := crud.New(st, lg)
	api.UserID = crud.DefaultUserID()
	bs, err := schema.Builtins()
	if err != nil {
		st.Close()
		return nil, nil, err
	}
	// Encryption for writes: find this DDL's registry record (its key lives in
	// the unsynced config dir), else fall back to a key beside the DDL.
	keyFile, pref := config.ResolveEncryptionForDDL(paths[0])
	cipher, _, err := enc.LoadCipher(keyFile)
	if err != nil {
		st.Close()
		return nil, nil, err
	}
	api.EnableEncryption(cipher, pref, s)
	api.EnableFeatures(s, bs)
	tw, err := script.Attach(api, paths[0], paths[2], s, bs)
	if err != nil {
		st.Close()
		return nil, nil, err
	}
	for _, w := range tw {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	return st, api, nil
}

// parseAssigns turns f=v args into a field map. An empty value (f=) is the
// empty string; SQL NULL has its own verb, so no value is magic here.
func parseAssigns(args []string) (map[string]string, error) {
	fields := make(map[string]string, len(args))
	for _, a := range args {
		k, v, ok := strings.Cut(a, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("expected field=value, got %q", a)
		}
		fields[k] = v
	}
	return fields, nil
}

// gcLog collects tx-log entries of rows hard-deleted more than gc_age_days
// ago, via segment rotation (never editing a log file in place). The next
// sync commits the rotation.
func gcLog(ddlPath, logDir string) error {
	s, err := ddl.ParseFile(ddlPath)
	if err != nil {
		return err
	}
	lg, err := txlog.Open(logDir)
	if err != nil {
		return err
	}
	entries, warns, err := lg.ReadAll()
	if err != nil {
		return err
	}
	for _, w := range warns {
		fmt.Println("warning:", w)
	}
	days := s.TunableInt("gc_age_days", 90)
	keep, collected := txlog.GC(entries, txlog.CutoffDays(days))
	if collected == 0 {
		fmt.Printf("nothing to collect (threshold: deleted > %d days ago)\n", days)
		return nil
	}
	seg, err := lg.Rotate(keep)
	if err != nil {
		return err
	}
	fmt.Printf("collected %d entr%s (deleted > %d days ago); %d kept in %s\n",
		collected, plural(collected), days, len(keep), seg)
	if txlog.InRepo(logDir) {
		fmt.Println("run sync to publish the rotation")
	}
	return nil
}

// renameTable renames in the SQLite view first (data preserved; the log is
// the truth anyway, so a half-done rename is always recoverable by replay),
// then rewrites the DDL with the old name kept as an alias.
func renameTable(ddlPath, dbPath, oldName, newName string) error {
	src, err := os.ReadFile(ddlPath)
	if err != nil {
		return err
	}
	out, found := ddl.RenameTable(src, oldName, newName)
	if !found {
		return fmt.Errorf("no table %q in %s", oldName, ddlPath)
	}
	if err := alterView(dbPath, oldName,
		`ALTER TABLE `+quoteIdent(oldName)+` RENAME TO `+quoteIdent(newName)); err != nil {
		return err
	}
	if err := os.WriteFile(ddlPath, out, 0o644); err != nil {
		return fmt.Errorf("view renamed but DDL rewrite failed (rerun to finish): %w", err)
	}
	fmt.Printf("renamed table %s -> %s (old name kept as an alias)\n", oldName, newName)
	return nil
}

func renameField(ddlPath, dbPath, table, oldName, newName string) error {
	src, err := os.ReadFile(ddlPath)
	if err != nil {
		return err
	}
	out, found := ddl.RenameField(src, table, oldName, newName)
	if !found {
		return fmt.Errorf("no field %q in table %q in %s", oldName, table, ddlPath)
	}
	if err := alterView(dbPath, table,
		`ALTER TABLE `+quoteIdent(table)+` RENAME COLUMN `+quoteIdent(oldName)+` TO `+quoteIdent(newName)); err != nil {
		return err
	}
	if err := os.WriteFile(ddlPath, out, 0o644); err != nil {
		return fmt.Errorf("view renamed but DDL rewrite failed (rerun to finish): %w", err)
	}
	fmt.Printf("renamed field %s.%s -> %s (old name kept as an alias)\n", table, oldName, newName)
	return nil
}

// alterView runs one ALTER against the view when the table exists there; a
// view that was never built has nothing to rename.
func alterView(dbPath, table, stmt string) error {
	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	var n int
	if err := st.DB().QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		fmt.Printf("note: %s not present in %s yet; only the DDL changes\n", table, dbPath)
		return nil
	}
	_, err = st.DB().Exec(stmt)
	return err
}

func crudComment(paths []string, table, id, text string) error {
	st, api, err := openAPI(paths)
	if err != nil {
		return err
	}
	defer st.Close()
	cid, err := api.CommentAdd(table, id, text)
	if err != nil {
		return err
	}
	fmt.Println(cid)
	return nil
}

func crudComments(paths []string, table, id string) error {
	st, api, err := openAPI(paths)
	if err != nil {
		return err
	}
	defer st.Close()
	rows, err := api.CommentsFor(table, id)
	if err != nil {
		return err
	}
	for _, r := range rows {
		fmt.Printf("%s\t%s\t%s\n", r["id"][:min(8, len(r["id"]))], r["date_created"], r["comment"])
	}
	return nil
}

func crudAttach(paths []string, table, id, target, desc string, copyIn bool) error {
	st, api, err := openAPI(paths)
	if err != nil {
		return err
	}
	defer st.Close()
	var attID string
	if copyIn {
		attID, err = api.AttachFile(table, id, target, desc)
	} else {
		attID, err = api.AttachURI(table, id, target, desc)
	}
	if err != nil {
		return err
	}
	fmt.Println(attID)
	return nil
}

func crudAttachments(paths []string, table, id string) error {
	st, api, err := openAPI(paths)
	if err != nil {
		return err
	}
	defer st.Close()
	atts, err := api.AttachmentsFor(table, id)
	if err != nil {
		return err
	}
	for _, at := range atts {
		fmt.Printf("%s\t%s\t%s\t%s\n", at.ID[:min(8, len(at.ID))], at.Kind, at.Label, at.Description)
	}
	return nil
}

func crudCreate(paths []string, table string, assigns []string) error {
	fields, err := parseAssigns(assigns)
	if err != nil {
		return err
	}
	st, api, err := openAPI(paths)
	if err != nil {
		return err
	}
	defer st.Close()
	id, err := api.Create(table, fields)
	if err != nil {
		return err
	}
	fmt.Println(id)
	return nil
}

func crudGet(paths []string, table, id string) error {
	st, api, err := openAPI(paths)
	if err != nil {
		return err
	}
	defer st.Close()
	row, ok, err := api.Get(table, id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no row %q in %q", id, table)
	}
	cols := make([]string, 0, len(row))
	for c := range row {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	for _, c := range cols {
		fmt.Printf("%s: %s\n", c, row[c])
	}
	return nil
}

func crudUpdate(paths []string, table, id string, assigns []string) error {
	fields, err := parseAssigns(assigns)
	if err != nil {
		return err
	}
	st, api, err := openAPI(paths)
	if err != nil {
		return err
	}
	defer st.Close()
	return api.Update(table, id, fields)
}

func crudSetNull(paths []string, table, id, field string) error {
	st, api, err := openAPI(paths)
	if err != nil {
		return err
	}
	defer st.Close()
	return api.SetFieldNull(table, id, field)
}

func crudDelete(verb string, paths []string, table, id string) error {
	st, api, err := openAPI(paths)
	if err != nil {
		return err
	}
	defer st.Close()
	if verb == "markdelete" {
		return api.MarkDelete(table, id)
	}
	return api.Delete(table, id)
}

func crudQuery(paths []string, query string) error {
	st, api, err := openAPI(paths)
	if err != nil {
		return err
	}
	defer st.Close()
	rows, err := api.Query(query)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		fmt.Println("(no rows)")
		return nil
	}
	cols := make([]string, 0, len(rows[0]))
	for c := range rows[0] {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	fmt.Println(strings.Join(cols, "\t"))
	for _, r := range rows {
		vals := make([]string, len(cols))
		for i, c := range cols {
			vals[i] = r[c]
		}
		fmt.Println(strings.Join(vals, "\t"))
	}
	return nil
}
