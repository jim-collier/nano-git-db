// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package tui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/jim-collier/nano-git-db/internal/core/config"
)

// pickResult is the picker's outcome: the paths to open, plus the registry
// record to stamp as opened (nil for an ad-hoc "open existing").
type pickResult struct {
	ddlPath, sqlitePath, logDir string
	cfg                         *config.DBConfig
}

// picker is the startup "which database?" screen shown when no DDL was named:
// the registered databases (unopenable ones flagged with why) plus entries to
// create or open one. It is its own tview app, run before the main App over the
// chosen database.
type picker struct {
	app     *tview.Application
	pages   *tview.Pages
	list    *tview.List
	status  *tview.TextView
	listed  []config.Listed
	actions []func() // per list row; nil for a blank spacer row
	result  *pickResult
}

// pickDatabase runs the picker and returns the chosen database, or nil if the
// user quit without choosing. screen is nil for the real terminal (tests pass a
// simulation screen).
func pickDatabase(screen tcell.Screen) (*pickResult, error) {
	p := &picker{
		app:    tview.NewApplication(),
		pages:  tview.NewPages(),
		list:   tview.NewList(),
		status: tview.NewTextView(),
		listed: config.List(),
	}
	p.build()
	if screen != nil {
		p.app.SetScreen(screen)
	}
	if err := p.app.SetRoot(p.pages, true).EnableMouse(true).Run(); err != nil {
		return nil, err
	}
	return p.result, nil
}

func (p *picker) build() {
	// Each entry is two lines (name + path/hint), the border padding keeps the
	// block off the frame, and blank spacer rows sit between entries, so it
	// reads as a menu instead of a packed grid.
	p.list.ShowSecondaryText(true).
		SetSelectedTextColor(tcell.ColorBlack).
		SetBorder(true).SetTitle(" nano-git-db ").
		SetBorderPadding(1, 1, 3, 3)

	// Build the entries with their actions, then lay them out interleaved with
	// spacer rows. actions[i] indexes list rows 1:1; a nil action = a spacer.
	type entry struct {
		main, sub string
		act       func()
	}
	var entries []entry
	for _, l := range p.listed {
		l := l
		main := l.Name
		sub := l.Dir
		if l.System {
			main += "  (system)"
		}
		if l.Err != nil {
			main = "[!] " + main
			sub = l.Err.Error()
		}
		entries = append(entries, entry{main, sub, func() { p.chooseListed(l) }})
	}
	entries = append(entries,
		entry{"Create new database", "register a new database and open it", p.createForm},
		entry{"Open existing ...", "open a DDL + tx-log without registering", p.openForm})
	for i, e := range entries {
		if i > 0 {
			p.list.AddItem("", "", 0, nil)
			p.actions = append(p.actions, nil)
		}
		p.list.AddItem(e.main, e.sub, 0, nil)
		p.actions = append(p.actions, e.act)
	}
	p.list.SetSelectedFunc(func(i int, _, _ string, _ rune) {
		if i >= 0 && i < len(p.actions) && p.actions[i] != nil {
			p.actions[i]()
		}
	})
	// Up/Down step over the blank spacer rows so the highlight only ever rests
	// on a real entry. First and last rows are always real, so it never runs
	// off the ends.
	p.list.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch {
		case ev.Key() == tcell.KeyDown || ev.Rune() == 'j':
			p.moveSelection(1)
			return nil
		case ev.Key() == tcell.KeyUp || ev.Rune() == 'k':
			p.moveSelection(-1)
			return nil
		}
		return ev
	})

	p.status.SetDynamicColors(true)
	p.setStatus("enter=open | Create/Open to make or open one | q=quit")

	// Center the menu as a bordered panel with margins around it, instead of
	// filling the whole screen. Proportional spacers keep it centred at any
	// terminal size while leaving generous room for long paths.
	panel := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.list, 0, 1, true).
		AddItem(p.status, 1, 0, false)
	root := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(panel, 0, 5, true).
			AddItem(nil, 0, 1, false), 0, 4, true).
		AddItem(nil, 0, 1, false)
	p.pages.AddPage("main", root, true, true)

	p.app.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		// q quits only outside input fields (where q is data).
		if ev.Rune() == 'q' {
			if _, ok := p.app.GetFocus().(*tview.InputField); !ok {
				p.app.Stop()
				return nil
			}
		}
		return ev
	})
}

func (p *picker) setStatus(msg string) { p.status.SetText(" " + msg) }

// moveSelection advances the highlight by dir (+1/-1), skipping spacer rows and
// stopping at the ends rather than wrapping.
func (p *picker) moveSelection(dir int) {
	n := p.list.GetItemCount()
	i := p.list.GetCurrentItem() + dir
	for i >= 0 && i < n && p.actions[i] == nil {
		i += dir
	}
	if i >= 0 && i < n {
		p.list.SetCurrentItem(i)
	}
}

// chooseListed opens a registered database, or reports why it can't open.
func (p *picker) chooseListed(l config.Listed) {
	if l.Err != nil {
		p.setStatus("cannot open " + l.Name + ": " + l.Err.Error())
		return
	}
	p.finish(&pickResult{
		ddlPath:    l.Config.DDLPath,
		sqlitePath: l.Config.SQLitePath,
		logDir:     l.Config.LogDir,
		cfg:        l.Config,
	})
}

// finish records the result and ends the picker so the caller can open it.
func (p *picker) finish(r *pickResult) {
	p.result = r
	p.app.Stop()
}

// createForm registers a new database (name required, refuses duplicates) and
// opens it. The DDL and tx-log dir are existing artifacts the record points at;
// the local sqlite/key default beside the record.
func (p *picker) createForm() {
	form := tview.NewForm()
	form.AddInputField("name", "", 0, nil, nil)
	form.AddInputField("ddl path", "", 0, nil, nil)
	form.AddInputField("tx-log dir", "", 0, nil, nil)
	closeForm := func() {
		p.pages.RemovePage("create")
		p.app.SetFocus(p.list)
	}
	form.AddButton("Create", func() {
		name := strings.TrimSpace(text(form, "name"))
		ddlPath := strings.TrimSpace(text(form, "ddl path"))
		logDir := strings.TrimSpace(text(form, "tx-log dir"))
		if name == "" || ddlPath == "" || logDir == "" {
			p.setStatus("name, ddl path and tx-log dir are all required")
			return
		}
		cfg, err := config.Create(name, ddlPath, logDir)
		if err != nil {
			p.setStatus("error: " + err.Error())
			return
		}
		closeForm()
		p.finish(&pickResult{ddlPath: cfg.DDLPath, sqlitePath: cfg.SQLitePath, logDir: cfg.LogDir, cfg: cfg})
	})
	form.AddButton("Cancel", closeForm)
	form.SetCancelFunc(closeForm)
	form.SetBorder(true).SetTitle(" create new database ")
	p.pages.AddPage("create", modal(form, 64, 12), true, true)
	p.app.SetFocus(form)
}

// openForm opens an existing DDL + tx-log without registering it. The sqlite
// view defaults beside the DDL; it rebuilds from the log on open.
func (p *picker) openForm() {
	form := tview.NewForm()
	form.AddInputField("ddl path", "", 0, nil, nil)
	form.AddInputField("tx-log dir", "", 0, nil, nil)
	closeForm := func() {
		p.pages.RemovePage("open")
		p.app.SetFocus(p.list)
	}
	form.AddButton("Open", func() {
		ddlPath := strings.TrimSpace(text(form, "ddl path"))
		logDir := strings.TrimSpace(text(form, "tx-log dir"))
		if ddlPath == "" || logDir == "" {
			p.setStatus("ddl path and tx-log dir are required")
			return
		}
		closeForm()
		p.finish(&pickResult{ddlPath: ddlPath, sqlitePath: config.LocalSQLite(ddlPath), logDir: logDir})
	})
	form.AddButton("Cancel", closeForm)
	form.SetCancelFunc(closeForm)
	form.SetBorder(true).SetTitle(" open existing database ")
	p.pages.AddPage("open", modal(form, 64, 10), true, true)
	p.app.SetFocus(form)
}

// text reads a form input field by label.
func text(form *tview.Form, label string) string {
	return form.GetFormItemByLabel(label).(*tview.InputField).GetText()
}
