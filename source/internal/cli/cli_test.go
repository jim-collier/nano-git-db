// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const personDDL = `
tables:
	table: person
		fields:
			field: name
				type: string
			field: nick
				type: string
`

// capture runs fn with stdout redirected and returns what it printed.
func capture(t *testing.T, fn func() error) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	ferr := fn()
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if ferr != nil {
		t.Fatalf("%v (output: %s)", ferr, out)
	}
	return string(out)
}

func TestCrudVerbsEndToEnd(t *testing.T) {
	dir := t.TempDir()
	ddlPath := filepath.Join(dir, "s.ddl")
	if err := os.WriteFile(ddlPath, []byte(personDDL), 0o644); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "view.sqlite")
	logDir := filepath.Join(dir, "log")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tri := []string{ddlPath, dbPath, logDir}
	run := func(args ...string) string {
		return capture(t, func() error { return Run(args) })
	}
	pre := func(rest ...string) []string {
		return append(append([]string{rest[0]}, tri...), rest[1:]...)
	}

	id := strings.TrimSpace(run(pre("create", "person", "name=Ann", "nick=A")...))
	if len(id) != 32 {
		t.Fatalf("create should print a 32-hex id, got %q", id)
	}

	got := run(pre("get", "person", id)...)
	if !strings.Contains(got, "name: Ann") || !strings.Contains(got, "nick: A") {
		t.Fatalf("get output missing fields:\n%s", got)
	}

	run(pre("update", "person", id, "name=Anne")...)
	got = run(pre("get", "person", id)...)
	if !strings.Contains(got, "name: Anne") {
		t.Fatalf("update did not land:\n%s", got)
	}

	run(pre("setnull", "person", id, "nick")...)
	got = run(pre("query", `SELECT COUNT(*) AS n FROM "person" WHERE "nick" IS NULL`)...)
	if !strings.Contains(got, "1") {
		t.Fatalf("setnull did not produce SQL NULL:\n%s", got)
	}

	run(pre("markdelete", "person", id)...)
	got = run(pre("get", "person", id)...)
	if !strings.Contains(got, "is_deleted: 1") {
		t.Fatalf("markdelete did not set is_deleted:\n%s", got)
	}

	run(pre("delete", "person", id)...)
	got = run(pre("query", `SELECT COUNT(*) AS n FROM "person"`)...)
	if !strings.Contains(got, "0") {
		t.Fatalf("delete left the row behind:\n%s", got)
	}
}

func TestCreateRejectsBadAssign(t *testing.T) {
	dir := t.TempDir()
	ddlPath := filepath.Join(dir, "s.ddl")
	if err := os.WriteFile(ddlPath, []byte(personDDL), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Run([]string{"create", ddlPath, filepath.Join(dir, "v.sqlite"), dir, "person", "no-equals"})
	if err == nil || !strings.Contains(err.Error(), "field=value") {
		t.Fatalf("want a field=value error, got %v", err)
	}
}

// Rename ops end-to-end: rows written under the old names must survive the
// rename in place AND a full rebuild-from-log into a fresh view (the alias
// translation is what makes the second part work).
func TestRenameOpsEndToEnd(t *testing.T) {
	dir := t.TempDir()
	ddlPath := filepath.Join(dir, "s.ddl")
	if err := os.WriteFile(ddlPath, []byte(personDDL), 0o644); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "view.sqlite")
	logDir := filepath.Join(dir, "log")

	out := capture(t, func() error {
		return Run([]string{"create", ddlPath, dbPath, logDir, "person", "name=Ann", "nick=Nan"})
	})
	id := strings.TrimSpace(out)

	capture(t, func() error {
		return Run([]string{"--rename-table", ddlPath, dbPath, "person", "people"})
	})
	capture(t, func() error {
		return Run([]string{"--rename-field", ddlPath, dbPath, "people", "nick", "handle"})
	})

	// the DDL now has the new names with the old ones as aliases
	src, err := os.ReadFile(ddlPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"table: people", "aliases: person", "field: handle", "aliases: nick"} {
		if !strings.Contains(string(src), want) {
			t.Fatalf("DDL missing %q:\n%s", want, src)
		}
	}

	// renamed in place: the existing view answers under the new names
	out = capture(t, func() error {
		return Run([]string{"get", ddlPath, dbPath, logDir, "people", id})
	})
	if !strings.Contains(out, "Ann") || !strings.Contains(out, "handle") {
		t.Fatalf("get after rename: %s", out)
	}

	// rebuild-from-log into a FRESH view: the log still says person/nick
	fresh := filepath.Join(dir, "fresh.sqlite")
	capture(t, func() error { return Run([]string{"replay", ddlPath, fresh, logDir}) })
	out = capture(t, func() error {
		return Run([]string{"query", ddlPath, fresh, logDir,
			`SELECT "name","handle" FROM "people" WHERE "is_deleted"=0`})
	})
	if !strings.Contains(out, "Ann") || !strings.Contains(out, "Nan") {
		t.Fatalf("replayed data missing: %s", out)
	}
}
