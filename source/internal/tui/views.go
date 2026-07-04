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

func (a *App) openView(view *schema.ViewSpec) {
	a.curView = view // for a live theme rebuild to reopen the right screen
	a.cur = ""
	var panels []*blockPanel
	root := a.blockPrim(view.Root, &panels)
	// The view page covers the main status bar, so it carries its own; while
	// it is up, setStatus mirrors into it.
	a.vstatus = tview.NewTextView()
	page := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(root, 0, 1, true).
		AddItem(a.vstatus, 1, 0, false)
	a.setStatus(viewHint)

	for i, panel := range panels {
		next := panels[(i+1)%len(panels)]
		panel.tbl.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
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
				a.loadBlock(panel)
				return nil
			case ev.Rune() == 'p':
				a.queryPicker(view, panel)
				return nil
			}
			return ev
		})
		if !panel.b.Readonly && panel.b.Type != "form" {
			panel.tbl.SetSelectedFunc(func(row, _ int) {
				if r := panel.rowAt(row); r != nil {
					a.editFormThen(panel.b.Table, r, panel.tbl, func() { a.loadBlock(panel) })
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
	if view.StartupNamedQuery != "" && len(a.cat.Queries) > 0 {
		if query := a.cat.NamedQuery(view.StartupNamedQuery); query != nil {
			a.loadQueryInto(panels[0], query)
		} else {
			a.setStatus(fmt.Sprintf("startup query %q not found", view.StartupNamedQuery))
		}
	}
}

const viewHint = "a=load-all p=queries enter=edit tab=next-block esc=back T=theme q=quit"

// loadQueryInto renders a named query's dataset in a leaf block, using the
// query's own column order.
func (a *App) loadQueryInto(panel *blockPanel, query *ddl.NamedQuery) {
	cols, rows, err := a.api.QueryRows(query.SQL)
	if err != nil {
		a.setStatus("error: " + err.Error())
		return
	}
	panel.tbl.Clear()
	panel.rows, panel.tree = rows, nil
	a.setStatus(fmt.Sprintf("%s: %d rows | %s", query.Name, len(rows), viewHint))
	for c, name := range cols {
		panel.tbl.SetCell(0, c, tview.NewTableCell(name).
			SetSelectable(false).SetAttributes(tcell.AttrBold))
	}
	for r, row := range rows {
		for c, name := range cols {
			panel.tbl.SetCell(r+1, c, tview.NewTableCell(cellValue(row, name)))
		}
	}
	if len(rows) > 0 {
		panel.tbl.Select(1, 0)
	}
}

// queryPicker offers the view's predefined queries; the pick loads into the
// focused block.
func (a *App) queryPicker(view *schema.ViewSpec, panel *blockPanel) {
	queries := a.cat.QueriesFor(view.Name)
	if len(queries) == 0 {
		a.setStatus("no predefined queries for this view")
		return
	}
	list := tview.NewList().ShowSecondaryText(false)
	close := func() {
		a.pages.RemovePage("queries")
		a.app.SetFocus(panel.tbl)
	}
	for i := range queries {
		query := queries[i]
		list.AddItem(query.Name, "", 0, func() {
			close()
			a.loadQueryInto(panel, &query)
		})
	}
	list.SetDoneFunc(close)
	list.SetBorder(true).SetTitle(" queries ")
	a.pages.AddPage("queries", modal(list, 44, len(queries)+4), true, true)
	a.app.SetFocus(list)
}

func (a *App) blockPrim(block *schema.ViewBlock, panels *[]*blockPanel) tview.Primitive {
	if len(block.Children) > 0 {
		flex := tview.NewFlex()
		if block.Dir == "row" {
			flex.SetDirection(tview.FlexRow)
		}
		props := block.Proportions()
		for i, child := range block.Children {
			flex.AddItem(a.blockPrim(child, panels), 0, props[i], i == 0)
		}
		return flex
	}
	tbl := tview.NewTable()
	title := fmt.Sprintf(" %s: %s (%s%s) ", block.Name, block.Table, block.Type, roMark(block.Readonly))
	tbl.SetBorder(true).SetTitle(title)
	if block.Type != "form" {
		tbl.SetSelectable(true, false)
		tbl.SetFixed(1, 0)
	}
	*panels = append(*panels, &blockPanel{b: block, tbl: tbl})
	return tbl
}

func roMark(ro bool) string {
	if ro {
		return ", ro"
	}
	return ""
}

func (a *App) loadBlock(panel *blockPanel) {
	panel.tbl.Clear()
	panel.rows, panel.tree = nil, nil
	switch panel.b.Type {
	case "form":
		// One-record panel; block linking is future work, show the first row.
		rows, err := a.cat.LiveRows(a.api, panel.b.Table)
		if err != nil {
			a.setStatus("error: " + err.Error())
			return
		}
		row := map[string]string{}
		if len(rows) > 0 {
			row = rows[0]
		}
		for i, field := range a.cat.Fields[panel.b.Table] {
			panel.tbl.SetCell(i, 0, tview.NewTableCell(field).SetAttributes(tcell.AttrBold))
			panel.tbl.SetCell(i, 1, tview.NewTableCell(row[field]))
		}
	case "tree_grid":
		tree, err := a.cat.TreeRows(a.api, panel.b.Table, panel.b.ParentField)
		if err != nil {
			a.setStatus("error: " + err.Error())
			return
		}
		panel.tree = tree
		cols := a.cat.ColumnsFor(panel.b.Table)
		a.blockHeader(panel, cols, len(tree))
		for r, treeRow := range tree {
			for c, name := range cols {
				val := cellValue(treeRow.Row, name)
				if c == 0 {
					val = strings.Repeat("  ", treeRow.Depth) + val
				}
				panel.tbl.SetCell(r+1, c, tview.NewTableCell(val))
			}
		}
	default: // grid
		rows, err := a.cat.LiveRows(a.api, panel.b.Table)
		if err != nil {
			a.setStatus("error: " + err.Error())
			return
		}
		panel.rows = rows
		cols := a.cat.ColumnsFor(panel.b.Table)
		a.blockHeader(panel, cols, len(rows))
		for r, row := range rows {
			for c, name := range cols {
				panel.tbl.SetCell(r+1, c, tview.NewTableCell(cellValue(row, name)))
			}
		}
	}
	if panel.b.Type != "form" && (len(panel.rows) > 0 || len(panel.tree) > 0) {
		panel.tbl.Select(1, 0)
	}
}

func (a *App) blockHeader(panel *blockPanel, cols []string, rowCount int) {
	a.setStatus(fmt.Sprintf("%s: %d rows | %s", panel.b.Table, rowCount, viewHint))
	for c, name := range cols {
		panel.tbl.SetCell(0, c, tview.NewTableCell(name).
			SetSelectable(false).SetAttributes(tcell.AttrBold))
	}
}

func cellValue(row map[string]string, col string) string {
	val := row[col]
	if col == "id" && len(val) > 8 {
		val = val[:8]
	}
	return val
}
