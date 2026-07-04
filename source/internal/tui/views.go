// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// View rendering: the resolved block tree maps 1:1 onto nested Flexes, leaves
// onto bordered Tables. Per the design a view opens with no records - 'a'
// loads a block (named queries will hook in here once they exist).
package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/schema"
)

// blockPanel is one leaf block and the rows currently behind its table.
type blockPanel struct {
	b    *schema.ViewBlock
	tbl  *tview.Table
	rows []map[string]string // grid
	tree []schema.TreeRow    // tree_grid
}

func (p *blockPanel) rowAt(r int) map[string]string {
	if p.tree != nil && r >= 1 && r <= len(p.tree) {
		return p.tree[r-1].Row
	}
	if r >= 1 && r <= len(p.rows) {
		return p.rows[r-1]
	}
	return nil
}

func (a *App) openView(v *schema.ViewSpec) {
	a.curView = v // for a live theme rebuild to reopen the right screen
	a.cur = ""
	var panels []*blockPanel
	root := a.blockPrim(v.Root, &panels)
	// The view page covers the main status bar, so it carries its own; while
	// it is up, setStatus mirrors into it.
	a.vstatus = tview.NewTextView()
	page := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(root, 0, 1, true).
		AddItem(a.vstatus, 1, 0, false)
	a.setStatus(viewHint)

	for i, p := range panels {
		next := panels[(i+1)%len(panels)]
		p.tbl.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
			switch {
			case ev.Key() == tcell.KeyEscape:
				a.pages.RemovePage("view")
				a.vstatus = nil
				a.curView = nil
				a.app.SetFocus(a.list)
				return nil
			case ev.Key() == tcell.KeyTab:
				a.app.SetFocus(next.tbl)
				return nil
			case ev.Rune() == 'a':
				a.loadBlock(p)
				return nil
			case ev.Rune() == 'p':
				a.queryPicker(v, p)
				return nil
			}
			return ev
		})
		if !p.b.Readonly && p.b.Type != "form" {
			p.tbl.SetSelectedFunc(func(row, _ int) {
				if r := p.rowAt(row); r != nil {
					a.editFormThen(p.b.Table, r, p.tbl, func() { a.loadBlock(p) })
				}
			})
		}
	}

	a.pages.RemovePage("view")
	a.pages.AddPage("view", page, true, true)
	a.app.SetFocus(panels[0].tbl)

	// startup_named_query: its dataset loads as soon as the view opens (v1:
	// into the first block). Silence when no queries file exists at all -
	// then the name cannot resolve and empty-open is the documented behavior.
	if v.StartupNamedQuery != "" && len(a.cat.Queries) > 0 {
		if q := a.cat.NamedQuery(v.StartupNamedQuery); q != nil {
			a.loadQueryInto(panels[0], q)
		} else {
			a.setStatus(fmt.Sprintf("startup query %q not found", v.StartupNamedQuery))
		}
	}
}

const viewHint = "a=load-all p=queries enter=edit tab=next-block esc=back T=theme q=quit"

// loadQueryInto renders a named query's dataset in a leaf block, using the
// query's own column order.
func (a *App) loadQueryInto(p *blockPanel, q *ddl.NamedQuery) {
	cols, rows, err := a.api.QueryRows(q.SQL)
	if err != nil {
		a.setStatus("error: " + err.Error())
		return
	}
	p.tbl.Clear()
	p.rows, p.tree = rows, nil
	a.setStatus(fmt.Sprintf("%s: %d rows | %s", q.Name, len(rows), viewHint))
	for c, name := range cols {
		p.tbl.SetCell(0, c, tview.NewTableCell(name).
			SetSelectable(false).SetAttributes(tcell.AttrBold))
	}
	for r, row := range rows {
		for c, name := range cols {
			p.tbl.SetCell(r+1, c, tview.NewTableCell(cellValue(row, name)))
		}
	}
	if len(rows) > 0 {
		p.tbl.Select(1, 0)
	}
}

// queryPicker offers the view's predefined queries; the pick loads into the
// focused block.
func (a *App) queryPicker(v *schema.ViewSpec, p *blockPanel) {
	qs := a.cat.QueriesFor(v.Name)
	if len(qs) == 0 {
		a.setStatus("no predefined queries for this view")
		return
	}
	list := tview.NewList().ShowSecondaryText(false)
	close := func() {
		a.pages.RemovePage("queries")
		a.app.SetFocus(p.tbl)
	}
	for i := range qs {
		q := qs[i]
		list.AddItem(q.Name, "", 0, func() {
			close()
			a.loadQueryInto(p, &q)
		})
	}
	list.SetDoneFunc(close)
	list.SetBorder(true).SetTitle(" queries ")
	a.pages.AddPage("queries", modal(list, 44, len(qs)+4), true, true)
	a.app.SetFocus(list)
}

func (a *App) blockPrim(b *schema.ViewBlock, panels *[]*blockPanel) tview.Primitive {
	if len(b.Children) > 0 {
		f := tview.NewFlex()
		if b.Dir == "row" {
			f.SetDirection(tview.FlexRow)
		}
		props := b.Proportions()
		for i, ch := range b.Children {
			f.AddItem(a.blockPrim(ch, panels), 0, props[i], i == 0)
		}
		return f
	}
	tbl := tview.NewTable()
	title := fmt.Sprintf(" %s: %s (%s%s) ", b.Name, b.Table, b.Type, roMark(b.Readonly))
	tbl.SetBorder(true).SetTitle(title)
	if b.Type != "form" {
		tbl.SetSelectable(true, false)
		tbl.SetFixed(1, 0)
	}
	*panels = append(*panels, &blockPanel{b: b, tbl: tbl})
	return tbl
}

func roMark(ro bool) string {
	if ro {
		return ", ro"
	}
	return ""
}

func (a *App) loadBlock(p *blockPanel) {
	p.tbl.Clear()
	p.rows, p.tree = nil, nil
	switch p.b.Type {
	case "form":
		// One-record panel; block linking is future work, show the first row.
		rows, err := a.cat.LiveRows(a.api, p.b.Table)
		if err != nil {
			a.setStatus("error: " + err.Error())
			return
		}
		row := map[string]string{}
		if len(rows) > 0 {
			row = rows[0]
		}
		for i, f := range a.cat.Fields[p.b.Table] {
			p.tbl.SetCell(i, 0, tview.NewTableCell(f).SetAttributes(tcell.AttrBold))
			p.tbl.SetCell(i, 1, tview.NewTableCell(row[f]))
		}
	case "tree_grid":
		tree, err := a.cat.TreeRows(a.api, p.b.Table, p.b.ParentField)
		if err != nil {
			a.setStatus("error: " + err.Error())
			return
		}
		p.tree = tree
		cols := a.cat.ColumnsFor(p.b.Table)
		a.blockHeader(p, cols, len(tree))
		for r, tr := range tree {
			for c, name := range cols {
				v := cellValue(tr.Row, name)
				if c == 0 {
					v = strings.Repeat("  ", tr.Depth) + v
				}
				p.tbl.SetCell(r+1, c, tview.NewTableCell(v))
			}
		}
	default: // grid
		rows, err := a.cat.LiveRows(a.api, p.b.Table)
		if err != nil {
			a.setStatus("error: " + err.Error())
			return
		}
		p.rows = rows
		cols := a.cat.ColumnsFor(p.b.Table)
		a.blockHeader(p, cols, len(rows))
		for r, row := range rows {
			for c, name := range cols {
				p.tbl.SetCell(r+1, c, tview.NewTableCell(cellValue(row, name)))
			}
		}
	}
	if p.b.Type != "form" && (len(p.rows) > 0 || len(p.tree) > 0) {
		p.tbl.Select(1, 0)
	}
}

func (a *App) blockHeader(p *blockPanel, cols []string, n int) {
	a.setStatus(fmt.Sprintf("%s: %d rows | %s", p.b.Table, n, viewHint))
	for c, name := range cols {
		p.tbl.SetCell(0, c, tview.NewTableCell(name).
			SetSelectable(false).SetAttributes(tcell.AttrBold))
	}
}

func cellValue(row map[string]string, col string) string {
	v := row[col]
	if col == "id" && len(v) > 8 {
		v = v[:8]
	}
	return v
}
