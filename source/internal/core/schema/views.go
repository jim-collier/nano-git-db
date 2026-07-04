// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// View resolution: turn the DDL's raw layout blocks into something the
// front-ends can render blindly - tables verified, types normalized, split
// hints reduced to direction + percent. Resolving once here keeps the TUI and
// web renderings from drifting.
package schema

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jim-collier/nano-git-db/internal/core/crud"
	"github.com/jim-collier/nano-git-db/internal/core/ddl"
)

// ViewBlock is one resolved layout block. A leaf (no children) binds a table;
// a container only arranges its children.
type ViewBlock struct {
	Name        string
	Table       string // leaves only
	Type        string // form|grid|tree_grid (leaves only)
	ParentField string // tree_grid only
	Readonly    bool
	Dir         string // how Children split: "row" (stacked) or "col" (side by side)
	Percent     int    // share of the parent split; 0 = split the leftover evenly
	Children    []*ViewBlock
}

// ViewSpec is one renderable view. Root is a synthetic container over the
// view's top-level blocks so front-ends render exactly one block tree. Leaves
// lists the data-bearing blocks in depth-first order; that index is how
// front-ends address a block.
type ViewSpec struct {
	Name              string
	StartupNamedQuery string
	Access            ddl.AccessRule // who may open the view (flat rule)
	Root              *ViewBlock
	Leaves            []*ViewBlock
}

// resolveViews validates every schema view against the catalog's tables.
// Blocks over unknown tables are dropped (warned), unknown block types and
// broken tree_grids degrade to plain grids, and views left with no leaves are
// dropped whole - so the front-ends never see a block they cannot render.
func (c *Catalog) resolveViews(schemas ...*ddl.Schema) {
	seen := map[string]bool{}
	for _, s := range schemas {
		for _, v := range s.Views {
			if v.Name == "" || seen[v.Name] {
				if v.Name != "" {
					c.warnf("view %q: duplicate name, keeping the first", v.Name)
				}
				continue
			}
			seen[v.Name] = true
			spec := ViewSpec{Name: v.Name, StartupNamedQuery: v.StartupNamedQuery, Access: v.Access}
			ro := v.Readonly != nil && *v.Readonly
			if len(v.Layout) > 0 {
				spec.Root = c.resolveBlock(v.Name, ddl.Block{Children: v.Layout}, ro, &spec.Leaves)
			}
			if len(spec.Leaves) == 0 {
				c.warnf("view %q: no renderable blocks, dropped", v.Name)
				continue
			}
			c.Views = append(c.Views, spec)
		}
	}
}

// resolveDefaultView picks the view the UIs open on start: the DDL's
// ui: default_view when it names a usable view, else the first resolved view
// per the design ("defaults to the first one defined").
func (c *Catalog) resolveDefaultView(schemas ...*ddl.Schema) {
	for _, s := range schemas {
		if s.DefaultView == "" {
			continue
		}
		if c.View(s.DefaultView) != nil {
			c.DefaultView = s.DefaultView
			return
		}
		c.warnf("default_view %q is not a usable view, falling back to the first one", s.DefaultView)
		break
	}
	if len(c.Views) > 0 {
		c.DefaultView = c.Views[0].Name
	}
}

// View finds a resolved view by name - the front-ends' guard against a view
// name arriving from user input.
func (c *Catalog) View(name string) *ViewSpec {
	for i := range c.Views {
		if c.Views[i].Name == name {
			return &c.Views[i]
		}
	}
	return nil
}

func (c *Catalog) warnf(format string, args ...any) {
	c.Warnings = append(c.Warnings, fmt.Sprintf(format, args...))
}

func (c *Catalog) resolveBlock(view string, b ddl.Block, inheritRO bool, leaves *[]*ViewBlock) *ViewBlock {
	ro := inheritRO
	if b.Readonly != nil {
		ro = *b.Readonly
	}
	rb := &ViewBlock{Name: b.Name, Readonly: ro}
	rb.Percent = locPercent(b.Location)

	if len(b.Children) > 0 { // container: a table here would be ambiguous
		if b.Table != "" {
			c.warnf("view %q block %q: has child blocks, ignoring its table", view, b.Name)
		}
		// The first positioned child decides the split axis.
		rb.Dir = "row"
		for _, ch := range b.Children {
			if d := locDir(ch.Location); d != "" {
				rb.Dir = d
				break
			}
		}
		for _, ch := range b.Children {
			if rc := c.resolveBlock(view, ch, ro, leaves); rc != nil {
				rb.Children = append(rb.Children, rc)
			}
		}
		if len(rb.Children) == 0 {
			return nil
		}
		return rb
	}

	if b.Table == "" || !c.Has(b.Table) {
		c.warnf("view %q block %q: unknown table %q, dropped", view, b.Name, b.Table)
		return nil
	}
	rb.Table = b.Table
	rb.Type = b.Type
	switch b.Type {
	case "", "grid":
		rb.Type = "grid"
	case "form":
	case "tree_grid":
		rb.ParentField = b.ParentField
		if !fieldOf(c.Fields[b.Table], b.ParentField) {
			c.warnf("view %q block %q: tree_grid parent_field %q is not a field of %q, rendering as grid",
				view, b.Name, b.ParentField, b.Table)
			rb.Type, rb.ParentField = "grid", ""
		}
	default:
		c.warnf("view %q block %q: unknown type %q, rendering as grid", view, b.Name, b.Type)
		rb.Type = "grid"
	}
	*leaves = append(*leaves, rb)
	return rb
}

func fieldOf(fields []string, name string) bool {
	for _, f := range fields {
		if f == name {
			return name != ""
		}
	}
	return false
}

// locDir maps a location hint's direction word to a split axis. v0 places
// blocks in DDL order, so only the axis and percent of the hint are used;
// the relative-to element is ignored.
func locDir(loc []string) string {
	for _, p := range loc {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "left", "right":
			return "col"
		case "above", "below":
			return "row"
		}
	}
	return ""
}

func locPercent(loc []string) int {
	for _, p := range loc {
		p = strings.TrimSpace(p)
		if n, ok := strings.CutSuffix(p, "%"); ok {
			if v, err := strconv.Atoi(strings.TrimSpace(n)); err == nil && v >= 1 && v <= 99 {
				return v
			}
		}
	}
	return 0
}

// Proportions turns the children's percent hints into integer shares that sum
// to 100: hinted children keep their percent, the rest split the remainder.
func (b *ViewBlock) Proportions() []int {
	out := make([]int, len(b.Children))
	left, loose := 100, 0
	for i, ch := range b.Children {
		if ch.Percent > 0 {
			out[i] = ch.Percent
			left -= ch.Percent
		} else {
			loose++
		}
	}
	for i := range out {
		if out[i] == 0 {
			out[i] = max(1, left/max(1, loose))
		}
	}
	return out
}

// TreeRow is one row of a tree_grid dataset: Depth is how far below a root it
// sits along the parent chain.
type TreeRow struct {
	Depth int
	Row   map[string]string
}

// TreeRows orders a table's live rows depth-first along the self-referential
// parentField. Rows with an empty or unknown parent are roots. A cycle would
// never be reached from a root, so any row still unvisited afterwards starts
// its own root - bad data degrades the layout, never hides rows or hangs.
func (c *Catalog) TreeRows(api *crud.API, table, parentField string) ([]TreeRow, error) {
	if !fieldOf(c.Fields[table], parentField) {
		return nil, fmt.Errorf("table %q has no field %q", table, parentField)
	}
	rows, err := c.LiveRows(api, table)
	if err != nil {
		return nil, err
	}
	byID := map[string]bool{}
	for _, r := range rows {
		byID[r["id"]] = true
	}
	kids := map[string][]int{}
	var roots []int
	for i, r := range rows {
		p := r[parentField]
		if p == "" || !byID[p] || p == r["id"] {
			roots = append(roots, i)
		} else {
			kids[p] = append(kids[p], i)
		}
	}
	out := make([]TreeRow, 0, len(rows))
	visited := make([]bool, len(rows))
	var walk func(i, depth int)
	walk = func(i, depth int) {
		if visited[i] {
			return
		}
		visited[i] = true
		out = append(out, TreeRow{Depth: depth, Row: rows[i]})
		for _, k := range kids[rows[i]["id"]] {
			walk(k, depth+1)
		}
	}
	for _, r := range roots {
		walk(r, 0)
	}
	for i := range rows {
		walk(i, 0)
	}
	return out, nil
}
