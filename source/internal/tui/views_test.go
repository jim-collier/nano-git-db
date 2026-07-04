// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

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

func orgSetup(t *testing.T) (*crud.API, *ddl.Schema, *ddl.Schema) {
	return ddlSetup(t, orgDDL)
}

func ddlSetup(t *testing.T, src string) (*crud.API, *ddl.Schema, *ddl.Schema) {
	t.Helper()
	sch, err := ddl.Parse([]byte(src))
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
	return api, sch, bs
}

func TestViewsListedBeforeTables(t *testing.T) {
	api, sch, bs := orgSetup(t)
	a, err := NewApp(api, sch, bs)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.cat.Views) != 1 || a.cat.Views[0].Name != "org" {
		t.Fatalf("want the org view resolved, got %+v", a.cat.Views)
	}
	if a.cat.DefaultView != "org" {
		t.Fatalf("default view = %q want org", a.cat.DefaultView)
	}
	if got, want := a.list.GetItemCount(), len(a.cat.Views)+len(a.cat.Tables); got != want {
		t.Fatalf("list items = %d, want views+tables = %d", got, want)
	}
	main, _ := a.list.GetItemText(0)
	if main != "org (view)" {
		t.Fatalf("first list item should be the view, got %q", main)
	}
}

// TestViewOpensAndLoadsOnSimulationScreen: the default view (first defined,
// no explicit default_view) auto-opens at boot; Esc back to the list, reopen
// it with Enter, load the tree_grid with 'a', hop to the second block with
// Tab and load it too, then back out and quit.
func TestViewOpensAndLoadsOnSimulationScreen(t *testing.T) {
	api, sch, bs := orgSetup(t)
	boss, err := api.Create("employees", map[string]string{"name": "Ann"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.Create("employees", map[string]string{"name": "Bee", "boss_id": boss}); err != nil {
		t.Fatal(err)
	}
	a, err := NewApp(api, sch, bs)
	if err != nil {
		t.Fatal(err)
	}
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(120, 32)
	done := make(chan error, 1)
	go func() { done <- a.RunWith(screen) }()

	waitForN := func(sub string, n int) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if strings.Count(screenText(screen), sub) >= n {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		a.Stop()
		t.Fatalf("%q (x%d) never appeared on screen:\n%s", sub, n, screenText(screen))
	}
	waitFor := func(sub string) { t.Helper(); waitForN(sub, 1) }

	waitFor("chain: employees (tree_grid)")             // default view opened itself
	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone) // back to the list
	waitFor("org (view)")
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // first item = the view
	waitFor("chain: employees (tree_grid)")
	waitFor("flat: employees (grid, ro)")
	screen.InjectKey(tcell.KeyRune, 'a', tcell.ModNone) // load focused tree_grid
	waitFor("Bee")
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyRune, 'a', tcell.ModNone) // load the flat grid too
	waitForN("Bee", 2)
	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone) // back to the list
	screen.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		a.Stop()
		t.Fatal("q did not quit the app")
	}
}

// TestStartupQueryAndPicker: a view with startup_named_query loads that
// query's dataset the moment it opens; 'p' offers the view's queries.
func TestStartupQueryAndPicker(t *testing.T) {
	src := strings.Replace(orgDDL, "view: \"org\"\n",
		"view: \"org\"\n\t\tstartup_named_query: \"Just Ann\"\n", 1)
	api, sch, bs := ddlSetup(t, src)
	if _, err := api.Create("employees", map[string]string{"name": "Ann"}); err != nil {
		t.Fatal(err)
	}
	if _, err := api.Create("employees", map[string]string{"name": "Bee"}); err != nil {
		t.Fatal(err)
	}
	a, err := NewApp(api, sch, bs)
	if err != nil {
		t.Fatal(err)
	}
	a.cat.Queries = []ddl.NamedQuery{
		{Name: "Just Ann", View: "org", Active: true,
			SQL: `SELECT "name" FROM "employees" WHERE "name"='Ann' AND "is_deleted"=0`},
		{Name: "Everyone", Active: true, // no view = offered everywhere
			SQL: `SELECT "name" FROM "employees" WHERE "is_deleted"=0`},
	}
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(120, 32)
	done := make(chan error, 1)
	go func() { done <- a.RunWith(screen) }()

	waitFor := func(sub string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if strings.Contains(screenText(screen), sub) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		a.Stop()
		t.Fatalf("%q never appeared on screen:\n%s", sub, screenText(screen))
	}

	waitFor("Just Ann: 1 rows") // startup query ran on auto-open
	waitFor("Ann")
	screen.InjectKey(tcell.KeyRune, 'p', tcell.ModNone) // query picker
	waitFor("Everyone")
	screen.InjectKey(tcell.KeyDown, 0, tcell.ModNone) // pick the global query
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitFor("Everyone: 2 rows")
	waitFor("Bee")
	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		a.Stop()
		t.Fatal("q did not quit the app")
	}
}
