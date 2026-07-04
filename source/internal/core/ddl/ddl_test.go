// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package ddl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValueScalars(t *testing.T) {
	if !IsNull("NULL") || IsNull("null") {
		t.Fatal("IsNull")
	}
	for _, s := range []string{"true", "1", "Y", "yes", "enabled", "T"} {
		if b, ok := AsBool(s); !ok || !b {
			t.Fatalf("AsBool(%q) want true", s)
		}
	}
	for _, s := range []string{"false", "0", "n", "no", "disabled", "F"} {
		if b, ok := AsBool(s); !ok || b {
			t.Fatalf("AsBool(%q) want false", s)
		}
	}
	if _, ok := AsBool("bool"); ok {
		t.Fatal("AsBool(bool) should not be a boolean")
	}
	nums := map[string]float64{"1,000": 1000, ".1": 0.1, "1.": 1, "0.1": 0.1, "3.005": 3.005, "-2.5": -2.5}
	for s, want := range nums {
		if f, ok := AsFloat(s); !ok || f != want {
			t.Fatalf("AsFloat(%q) = %v,%v want %v", s, f, ok, want)
		}
	}
	if _, ok := AsInt("3.005"); ok {
		t.Fatal("AsInt(3.005) should fail")
	}
	if i, ok := AsInt("1,000"); !ok || i != 1000 {
		t.Fatalf("AsInt(1,000) = %v,%v", i, ok)
	}
	if name, _, ok := AsFunc("fMyFunction()"); !ok || name != "fMyFunction" {
		t.Fatalf("AsFunc = %q,%v", name, ok)
	}
	if sql, ok := AsSQL("`SELECT * FROM t`"); !ok || sql != "SELECT * FROM t" {
		t.Fatalf("AsSQL = %q,%v", sql, ok)
	}
	if s, q := Unquote(`"o'l"`); !q || s != "o'l" {
		t.Fatalf("Unquote = %q,%v", s, q)
	}
}

func TestSplitListRespectsQuotesAndCommas(t *testing.T) {
	got := SplitList(`table7, "old_table79", 'a, b'`)
	want := []string{"table7", "old_table79", "a, b"}
	if len(got) != len(want) {
		t.Fatalf("SplitList len = %v (%q)", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SplitList[%d] = %q want %q", i, got[i], want[i])
		}
	}
}

func TestBuildTreeNestingAndListItems(t *testing.T) {
	src := "tables:\n\ttable: t1\n\t\tfields:\n\t\t\tfield: a\n\t\t\tfield: b\n\t\tuniques:\n\t\t\ta, b\n"
	root, _, err := buildTree([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	tables := root.child("tables")
	if tables == nil {
		t.Fatal("no tables node")
	}
	tbl := tables.child("table")
	if tbl == nil || tbl.Value != "t1" {
		t.Fatalf("table node = %+v", tbl)
	}
	if fs := tbl.child("fields").all("field"); len(fs) != 2 {
		t.Fatalf("want 2 fields, got %d", len(fs))
	}
	if rows := tbl.child("uniques").items(); len(rows) != 1 || rows[0].Value != "a, b" {
		t.Fatalf("uniques items = %+v", rows)
	}
}

func TestInconsistentIndentErrors(t *testing.T) {
	src := "tables:\n\ttable: t\n        field: a\n" // tab then spaces
	if _, _, err := buildTree([]byte(src)); err == nil {
		t.Fatal("expected inconsistent-indentation error")
	}
}

func TestCommentStrippingIgnoresQuotedHash(t *testing.T) {
	src := "tables:\n\ttable: t\n\t\tfields:\n\t\t\tfield: a\n\t\t\t\tspecial: \"a#b\"  ## real comment\n"
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Tables[0].Fields[0].Special; got != "a#b" {
		t.Fatalf("special = %q want a#b", got)
	}
}

// Parses the real project/example.ddl to guard the full grammar. Path is found
// by climbing so it survives directory moves.
func TestParseExampleDDL(t *testing.T) {
	path := findExample(t)
	s, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if len(s.Tables) != 1 {
		t.Fatalf("tables = %d want 1", len(s.Tables))
	}
	tbl := s.Tables[0]
	if tbl.Name != "table_name1" {
		t.Fatalf("table name = %q", tbl.Name)
	}
	if len(tbl.Aliases) != 2 || tbl.Aliases[1] != "old_table79" {
		t.Fatalf("aliases = %v", tbl.Aliases)
	}
	if len(tbl.Fields) != 2 {
		t.Fatalf("fields = %d want 2", len(tbl.Fields))
	}
	f1 := tbl.Fields[0]
	if f1.Name != "field_name1" || f1.Type != "string" || f1.Default != "NULL" {
		t.Fatalf("field1 = %+v", f1)
	}
	if f1.NullOK == nil || !*f1.NullOK {
		t.Fatal("field1 null_ok want true")
	}
	if f1.UI.Order == nil || *f1.UI.Order != 3.005 {
		t.Fatalf("field1 ui.order = %v", f1.UI.Order)
	}
	if len(tbl.Uniques) != 2 || len(tbl.Uniques[0]) != 2 || tbl.Uniques[1][0] != "field3" {
		t.Fatalf("uniques = %v", tbl.Uniques)
	}
	if len(tbl.Indexes) != 1 || tbl.Indexes[0][0] != "field_name1" {
		t.Fatalf("indexes = %v", tbl.Indexes)
	}
	if tbl.Features.Comments || tbl.Features.AuditTrail {
		t.Fatalf("features should default off: %+v", tbl.Features)
	}

	if len(s.Relations) != 2 || s.Relations[0].Type != "1:m" || s.Relations[1].Type != "m:m" {
		t.Fatalf("relations = %+v", s.Relations)
	}
	if !s.Relations[0].CascadeDelete {
		t.Fatal("relation[0] cascade_delete want true")
	}

	if len(s.Views) != 1 || s.Views[0].Name != "people" {
		t.Fatalf("views = %+v", s.Views)
	}
	if s.DefaultView != "people" {
		t.Fatalf("default view = %q want people", s.DefaultView)
	}
	layout := s.Views[0].Layout
	if len(layout) != 2 || layout[0].Name != "top" || len(layout[0].Children) != 2 {
		t.Fatalf("view layout = %+v", layout)
	}
	if layout[1].Name != "3" || len(layout[1].Location) != 3 || layout[1].Location[0] != "top" {
		t.Fatalf("block 3 = %+v", layout[1])
	}
}

func findExample(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		p := filepath.Join(dir, "project", "example.ddl")
		if _, err := os.Stat(p); err == nil {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Skip("project/example.ddl not found; skipping integration parse")
	return ""
}

// The hierarchy does not have to be unique: duplicate wrappers merge into the
// first occurrence, and "a/b:" is shorthand for nesting - so long DDLs can
// restate a section instead of finding the right indent far above.
func TestHierarchyMergeAndShorthand(t *testing.T) {
	src := "database:\n" +
		"\ttables:\n" +
		"\t\ttable: t1\n" +
		"\t\t\tfields:\n" +
		"\t\t\t\tfield: a\n" +
		"database:\n" + // duplicate wrapper merges
		"\ttables:\n" +
		"\t\ttable: t2\n" +
		"\t\t\tfields:\n" +
		"\t\t\t\tfield: b\n" +
		"database/tables:\n" + // path shorthand
		"\ttable: t3\n" +
		"\t\tfields:\n" +
		"\t\t\tfield: c\n"
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Tables) != 3 || s.Tables[0].Name != "t1" || s.Tables[2].Name != "t3" {
		t.Fatalf("tables = %+v", s.Tables)
	}
	for _, w := range s.Warnings {
		if strings.Contains(w, "duplicate") {
			t.Errorf("container merging must not warn: %s", w)
		}
	}
}

// Old flat DDLs (no database:/ui: wrappers, `methods:` not `code:`) must keep
// parsing unchanged.
func TestFlatLayoutAndMethodsKeyStillParse(t *testing.T) {
	src := "methods:\n" +
		"\tbefore_open: fBoot()\n" +
		"tables:\n" +
		"\ttable: t\n" +
		"\t\tfields:\n" +
		"\t\t\tfield: a\n" +
		"\t\tmethods:\n" +
		"\t\t\tbefore_update: fCheck()\n" +
		"views:\n" +
		"\tview: v\n" +
		"\t\tlayout:\n" +
		"\t\t\tblock: b\n" +
		"\t\t\t\ttable: t\n"
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if s.AppCode["before_open"] != "fBoot()" {
		t.Fatalf("app code = %v", s.AppCode)
	}
	if len(s.Tables) != 1 || s.Tables[0].Code["before_update"] != "fCheck()" {
		t.Fatalf("table code = %+v", s.Tables)
	}
	if len(s.Views) != 1 || s.DefaultView != "" {
		t.Fatalf("views = %+v default = %q", s.Views, s.DefaultView)
	}
}

func TestUIWrapperAndDefaultView(t *testing.T) {
	src := "code:\n" +
		"\tafter_open: fReady()\n" +
		"ui:\n" +
		"\tviews:\n" +
		"\t\tview: alpha\n" +
		"\t\t\tlayout:\n" +
		"\t\t\t\tblock: b\n" +
		"\t\t\t\t\ttable: t\n" +
		"\t\tview: beta\n" +
		"\t\t\tlayout:\n" +
		"\t\t\t\tblock: b\n" +
		"\t\t\t\t\ttable: t\n" +
		"\tdefault_view: \"beta\"\n"
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if s.AppCode["after_open"] != "fReady()" {
		t.Fatalf("app code = %v", s.AppCode)
	}
	if len(s.Views) != 2 || s.DefaultView != "beta" {
		t.Fatalf("views = %+v default = %q", s.Views, s.DefaultView)
	}
}

// Parse-time diagnostics: this DDL targets users without SQL knowledge, so
// mistakes must surface as warnings, not silence or opaque SQLite errors.
func TestParserWarnings(t *testing.T) {
	src := "tables:\n" +
		"\ttable: t\n" +
		"\t\tfields:\n" +
		"\t\t\tfield: a\n" +
		"\t\t\t\ttype: string\n" +
		"\t\t\t\ttype: int\n" + // duplicate key
		"\t\t\tfield: b\n" +
		"\t\t\t\ttitle: John's list # swallowed comment\n" + // unclosed quote hides '#'
		"\t\tuniques:\n" +
		"\t\t\ta, nope\n" // unknown field
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Tables[0].Fields[0].Type; got != "string" {
		t.Errorf("conflicting duplicate scalar: first value must win, got %q", got)
	}
	for _, want := range []string{"duplicate key", "unclosed quote", "unknown field"} {
		found := false
		for _, w := range s.Warnings {
			if strings.Contains(w, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no %q warning in %v", want, s.Warnings)
		}
	}
	// Repeating keys (field, table) must NOT warn.
	for _, w := range s.Warnings {
		if strings.Contains(w, `"field"`) || strings.Contains(w, `"table"`) {
			t.Errorf("false duplicate warning: %s", w)
		}
	}
}

// Load-time validation: what can be safely assumed around is fixed with a
// warning (nameless/duplicate tables and fields dropped, unknown types stored
// as text); nothing here may hard-error.
func TestLoadValidationSoftFixes(t *testing.T) {
	src := "tables:\n" +
		"\ttable: t\n" +
		"\t\tfields:\n" +
		"\t\t\tfield: a\n" +
		"\t\t\t\ttype: strng\n" + // typo'd type
		"\t\t\tfield:\n" + // no name
		"\t\t\tfield: a\n" + // redefined
		"\t\t\tfield: id\n" + // system column collision
		"\ttable: t\n" + // redefined table
		"\t\tfields:\n" +
		"\t\t\tfield: other\n" +
		"\ttable:\n" // no name
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Tables) != 1 || len(s.Tables[0].Fields) != 1 || s.Tables[0].Fields[0].Name != "a" {
		t.Fatalf("want one table with one field, got %+v", s.Tables)
	}
	warns := strings.Join(s.Warnings, "\n")
	for _, want := range []string{
		"unknown type", "field with no name", "already defined",
		"auto-added system field", "table with no name",
		`table "t" already defined`,
	} {
		if !strings.Contains(warns, want) {
			t.Errorf("warnings missing %q:\n%s", want, warns)
		}
	}
}

func TestIrregularIndentWarns(t *testing.T) {
	src := "tables:\n" +
		"  table: t\n" +
		"    fields:\n" +
		"   field: a\n" // 3 spaces on a 2-space file
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range s.Warnings {
		if strings.Contains(w, "indent width") {
			found = true
		}
	}
	if !found {
		t.Errorf("no indent warning in %v", s.Warnings)
	}
}

func TestTunables(t *testing.T) {
	src := "tunables:\n" +
		"\tgit_sync_frequency: 30\n" +
		"\tgc_age_days = 45\n" + // the design doc writes tunables with '='
		"\tmystery: 9\n" +
		"tunables:\n" + // duplicate section merges; first value wins
		"\tgit_sync_frequency: 99\n"
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.TunableInt("git_sync_frequency", 60); got != 30 {
		t.Fatalf("git_sync_frequency = %d", got)
	}
	if got := s.TunableInt("gc_age_days", 90); got != 45 {
		t.Fatalf("gc_age_days = %d", got)
	}
	if got := s.TunableInt("absent", 7); got != 7 {
		t.Fatalf("absent tunable = %d", got)
	}
	joined := strings.Join(s.Warnings, "\n")
	if !strings.Contains(joined, `unknown tunable "mystery"`) {
		t.Fatalf("warnings = %v", s.Warnings)
	}
}
