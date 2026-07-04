// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package web

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jim-collier/nano-git-db/internal/core/crud"
	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/schema"
	"github.com/jim-collier/nano-git-db/internal/core/store"
	"github.com/jim-collier/nano-git-db/internal/core/txlog"
)

const orgDDL = `
tables:
	table: employees
		fields:
			field: name
				type: string
			field: boss_id
				type: string

views:
	view: "org"
		layout:
			block: "chain"
				table: employees
				type: tree_grid
				parent_field: boss_id
			block: "flat"
				table: employees
				location: chain, right, 50%
				type: grid
				readonly: yes
`

func newViewServer(t *testing.T) (*server, *crud.API) {
	t.Helper()
	sch, err := ddl.Parse([]byte(orgDDL))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "org.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Build(sch); err != nil {
		t.Fatal(err)
	}
	if _, err := schema.Bootstrap(st, sch); err != nil {
		t.Fatal(err)
	}
	lg, err := txlog.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	api := crud.New(st, lg)
	api.UserID = "test"
	bs, err := schema.Builtins()
	if err != nil {
		t.Fatal(err)
	}
	cat, err := schema.NewCatalog(api, sch, bs)
	if err != nil {
		t.Fatal(err)
	}
	s, err := newServer(api, cat)
	if err != nil {
		t.Fatal(err)
	}
	return s, api
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}

func TestViewListedAndPageRenders(t *testing.T) {
	s, _ := newViewServer(t)
	h := s.routes()

	rec := get(t, h, "/")
	if !strings.Contains(rec.Body.String(), `hx-get="/v/org"`) {
		t.Errorf("sidebar should list the view:\n%s", rec.Body.String())
	}
	// the default view ("org", first defined) auto-loads into #main
	if !strings.Contains(rec.Body.String(), `hx-trigger="load"`) {
		t.Errorf("index should auto-load the default view:\n%s", rec.Body.String())
	}

	rec = get(t, h, "/v/org")
	if rec.Code != http.StatusOK {
		t.Fatalf("view page = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"chain: employees (tree_grid)",
		"flat: employees (grid, ro)",
		`hx-get="/v/org/b/0/rows"`,
		`hx-get="/v/org/b/1/rows"`,
		"flex-direction:row", // the right-hint split
	} {
		if !strings.Contains(body, want) {
			t.Errorf("view page missing %q:\n%s", want, body)
		}
	}

	if rec = get(t, h, "/v/nope"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown view = %d, want 404", rec.Code)
	}
	if rec = get(t, h, "/v/org/b/9/rows"); rec.Code != http.StatusNotFound {
		t.Errorf("out-of-range block = %d, want 404", rec.Code)
	}
}

func TestViewBlockRows(t *testing.T) {
	s, api := newViewServer(t)
	h := s.routes()
	boss, err := api.Create("employees", map[string]string{"name": "Ann"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.Create("employees", map[string]string{"name": "Bee", "boss_id": boss}); err != nil {
		t.Fatal(err)
	}

	// block 0: the tree_grid - Bee indented one level under Ann, editable
	body := get(t, h, "/v/org/b/0/rows").Body.String()
	if !strings.Contains(body, "padding-left:1em") {
		t.Errorf("child row should be indented:\n%s", body)
	}
	if strings.Index(body, "Ann") > strings.Index(body, "Bee") {
		t.Errorf("parent must render before child:\n%s", body)
	}
	if !strings.Contains(body, "/edit") {
		t.Errorf("writable block should offer edit:\n%s", body)
	}

	// block 1: the readonly flat grid - no edit affordance
	body = get(t, h, "/v/org/b/1/rows").Body.String()
	if !strings.Contains(body, "Ann") || !strings.Contains(body, "Bee") {
		t.Errorf("flat grid should list both rows:\n%s", body)
	}
	if strings.Contains(body, "/edit") {
		t.Errorf("readonly block must not offer edit:\n%s", body)
	}
}

// Named queries: the view page offers the dropdown, /v/{view}/q runs one,
// and a resolvable startup_named_query auto-loads on open.
func TestViewNamedQueries(t *testing.T) {
	s, api := newViewServer(t)
	if _, err := api.Create("employees", map[string]string{"name": "Ann"}); err != nil {
		t.Fatal(err)
	}
	s.cat.Queries = []ddl.NamedQuery{
		{Name: "Just Ann", View: "org", Active: true,
			SQL: `SELECT "name" FROM "employees" WHERE "name"='Ann' AND "is_deleted"=0`},
		{Name: "Inactive", View: "org", SQL: `SELECT 1`},
	}
	h := s.routes()

	rec := get(t, h, "/v/org")
	body := rec.Body.String()
	if !strings.Contains(body, "<option>Just Ann</option>") {
		t.Fatalf("view page missing query dropdown:\n%s", body)
	}
	if strings.Contains(body, "Inactive") {
		t.Fatal("inactive queries must not be offered")
	}
	if strings.Contains(body, "hx-trigger=\"load\" hx-target=\"#vb-0\"") {
		t.Fatal("no startup query configured, nothing should auto-load")
	}

	rec = get(t, h, "/v/org/q?name=Just+Ann")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Ann") {
		t.Fatalf("query run = %d %s", rec.Code, rec.Body)
	}
	if rec := get(t, h, "/v/org/q?name=nope"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown query = %d", rec.Code)
	}
	if rec := get(t, h, "/v/org/q?name=Inactive"); rec.Code != http.StatusNotFound {
		t.Fatalf("inactive query = %d", rec.Code)
	}

	// startup_named_query resolvable -> auto-load div appears
	// (html/template renders the urlquery-escaped '+' as &#43; in attributes)
	s.cat.Views[0].StartupNamedQuery = "Just Ann"
	body = get(t, h, "/v/org").Body.String()
	if !strings.Contains(body, `hx-trigger="load" hx-target="#vb-0"`) ||
		!strings.Contains(body, "name=Just&#43;Ann") {
		t.Fatalf("startup query should auto-load:\n%s", body)
	}
}
