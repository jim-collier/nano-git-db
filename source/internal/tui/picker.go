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
	app    *tview.Application
	pages  *tview.Pages
	list   *tview.List
	status *tview.TextView
	listed []config.Listed
	result *pickResult
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

// createIdx / openIdx are the two trailing menu entries, after the registered
// databases.
func (p *picker) createIdx() int { return len(p.listed) }
func (p *picker) openIdx() int   { return len(p.listed) + 1 }

func (p *picker) build() {
	p.list.ShowSecondaryText(true).SetBorder(true).SetTitle(" choose a database ")
	for _, l := range p.listed {
		main := l.Name
		sub := l.Dir
		if l.System {
			main += "  (system)"
		}
		if l.Err != nil {
			main = "[!] " + main
			sub = l.Err.Error()
		}
		p.list.AddItem(main, sub, 0, nil)
	}
	p.list.AddItem("Create new database", "register a new database and open it", 0, nil)
	p.list.AddItem("Open existing ...", "open a DDL + tx-log without registering", 0, nil)
	p.list.SetSelectedFunc(func(i int, _, _ string, _ rune) { p.choose(i) })

	p.status.SetDynamicColors(true)
	p.setStatus("enter=open | Create/Open to make or open one | q=quit")

	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.list, 0, 1, true).
		AddItem(p.status, 1, 0, false)
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

// choose acts on a list selection: open a registered database, or open one of
// the create/open sub-forms.
func (p *picker) choose(i int) {
	switch {
	case i < len(p.listed):
		l := p.listed[i]
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
	case i == p.createIdx():
		p.createForm()
	case i == p.openIdx():
		p.openForm()
	}
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
