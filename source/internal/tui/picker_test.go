// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/jim-collier/nano-git-db/internal/core/config"
)

// isolateConfig points the registry at a throwaway config dir so the picker
// never sees the developer's real databases.
func isolateConfig(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("AppData", t.TempDir())
	t.Setenv("XDG_CONFIG_DIRS", t.TempDir())
	t.Setenv("ProgramData", t.TempDir())
}

func writeValidDDL(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "todo.ddl")
	if err := os.WriteFile(p, []byte(personDDL), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// runPicker drives pickDatabase on a simulation screen and returns a waitFor
// helper plus the result channel.
func runPicker(t *testing.T) (tcell.SimulationScreen, chan *pickResult, func(string)) {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(100, 30)
	out := make(chan *pickResult, 1)
	go func() {
		r, err := pickDatabase(screen)
		if err != nil {
			t.Errorf("pickDatabase: %v", err)
		}
		out <- r
	}()
	waitFor := func(sub string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if strings.Contains(screenText(screen), sub) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("%q never appeared on screen:\n%s", sub, screenText(screen))
	}
	return screen, out, waitFor
}

// A registered, openable database appears and selecting it returns its paths.
func TestPickerSelectsRegistered(t *testing.T) {
	isolateConfig(t)
	ddlPath := writeValidDDL(t)
	cfg, err := config.Create("todo", ddlPath, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	screen, out, waitFor := runPicker(t)
	waitFor("todo")                                    // registered db drew
	waitFor("Create new database")                     // menu entries drew
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // first item = todo

	select {
	case r := <-out:
		if r == nil {
			t.Fatal("expected a pick result")
		}
		if r.ddlPath != cfg.DDLPath || r.logDir != cfg.LogDir {
			t.Fatalf("wrong paths: %+v", r)
		}
		if r.cfg == nil || r.cfg.Name != "todo" {
			t.Fatalf("registry record not returned: %+v", r.cfg)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("picker did not return after selecting a database")
	}
}

// An unopenable record (its DDL is gone) shows with the [!] marker, cannot be
// opened, and q quits the picker with no result.
func TestPickerFlagsBrokenAndQuits(t *testing.T) {
	isolateConfig(t)
	// register a db then delete its DDL
	ddlPath := writeValidDDL(t)
	if _, err := config.Create("broken", ddlPath, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(ddlPath); err != nil {
		t.Fatal(err)
	}

	screen, out, waitFor := runPicker(t)
	waitFor("[!] broken")    // flagged unopenable
	waitFor("DDL not found") // with the reason
	screen.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)

	select {
	case r := <-out:
		if r != nil {
			t.Fatalf("quit should yield no result, got %+v", r)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("q did not quit the picker")
	}
}
