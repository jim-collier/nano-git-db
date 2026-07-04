// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// View rendering: the resolved block tree becomes nested flexbox divs, leaves
// become panels that stay empty until their All button asks for rows - same
// contract as the TUI. Styles are computed here (ints only) so the template
// stays dumb.
package web

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"

	"github.com/jim-collier/nano-git-db/internal/core/schema"
)

// blockVM is one block ready for the recursive template.
type blockVM struct {
	View     string
	Title    string
	Leaf     bool
	Index    int          // leaf index; how block rows are addressed
	Style    template.CSS // flex share, set by the parent
	DirStyle template.CSS // container: how children split
	Children []*blockVM
}

func (s *server) blockVM(view string, b *schema.ViewBlock, idx *int) *blockVM {
	vm := &blockVM{View: view}
	if len(b.Children) > 0 {
		dir := "column"
		if b.Dir == "col" {
			dir = "row"
		}
		vm.DirStyle = template.CSS("display:flex;flex-direction:" + dir + ";gap:6px;min-height:0;")
		props := b.Proportions()
		for i, ch := range b.Children {
			c := s.blockVM(view, ch, idx)
			c.Style = template.CSS(fmt.Sprintf("flex:%d 1 0;min-height:0;overflow:auto;", props[i]))
			vm.Children = append(vm.Children, c)
		}
		return vm
	}
	vm.Leaf = true
	vm.Index = *idx
	*idx++
	ro := ""
	if b.Readonly {
		ro = ", ro"
	}
	vm.Title = fmt.Sprintf("%s: %s (%s%s)", b.Name, b.Table, b.Type, ro)
	return vm
}

// view pulls and validates the view path segment; nil means already handled.
func (s *server) view(w http.ResponseWriter, r *http.Request) *schema.ViewSpec {
	v := s.cat.View(r.PathValue("view"))
	if v == nil {
		http.Error(w, "no such view", http.StatusNotFound)
	}
	return v
}

func (s *server) viewPage(w http.ResponseWriter, r *http.Request) {
	v := s.view(w, r)
	if v == nil {
		return
	}
	idx := 0
	root := s.blockVM(v.Name, v.Root, &idx)
	root.Style = "height:85vh;" // anchor the flex tree; children split this
	data := map[string]any{
		"Name": v.Name, "Root": root,
		"Queries": s.cat.QueriesFor(v.Name),
	}
	// startup_named_query: an hx-trigger=load div fetches its dataset into the
	// first block as soon as the view renders. Unresolvable names degrade to
	// the normal empty open.
	if v.StartupNamedQuery != "" && s.cat.NamedQuery(v.StartupNamedQuery) != nil {
		data["StartupQuery"] = v.StartupNamedQuery
	}
	s.render(w, "view.html", data)
}

// viewQuery runs a predefined query; the dataset renders readonly with the
// query's own column order (v1 lands in the view's first block).
func (s *server) viewQuery(w http.ResponseWriter, r *http.Request) {
	if s.view(w, r) == nil {
		return
	}
	q := s.cat.NamedQuery(r.URL.Query().Get("name"))
	if q == nil {
		http.Error(w, "no such query", http.StatusNotFound)
		return
	}
	cols, rows, err := s.api.QueryRows(q.SQL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "qrows.html", map[string]any{"Name": q.Name, "Cols": cols, "Rows": rows})
}

// viewBlockRows loads one leaf block's dataset. Grid and tree_grid share the
// template: a grid is just a tree where everything is a root.
func (s *server) viewBlockRows(w http.ResponseWriter, r *http.Request) {
	v := s.view(w, r)
	if v == nil {
		return
	}
	i, err := strconv.Atoi(r.PathValue("i"))
	if err != nil || i < 0 || i >= len(v.Leaves) {
		http.Error(w, "no such block", http.StatusNotFound)
		return
	}
	b := v.Leaves[i]
	data := map[string]any{
		"Type": b.Type, "Table": b.Table, "Readonly": b.Readonly,
		"Cols": s.cat.ColumnsFor(b.Table),
	}
	switch b.Type {
	case "form":
		// One-record panel; block linking is future work, show the first row.
		rows, err := s.cat.LiveRows(s.api, b.Table)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		row := map[string]string{}
		if len(rows) > 0 {
			row = rows[0]
		}
		type fv struct{ F, V string }
		var pairs []fv
		for _, f := range s.cat.Fields[b.Table] {
			pairs = append(pairs, fv{f, row[f]})
		}
		data["FieldVals"] = pairs
	case "tree_grid":
		tree, err := s.cat.TreeRows(s.api, b.Table, b.ParentField)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data["Rows"] = tree
	default: // grid
		rows, err := s.cat.LiveRows(s.api, b.Table)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tree := make([]schema.TreeRow, len(rows))
		for j, row := range rows {
			tree[j] = schema.TreeRow{Row: row}
		}
		data["Rows"] = tree
	}
	s.render(w, "vrows.html", data)
}
