// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// Predefined (named) queries: a plain-text sidecar file next to the DDL, in
// the same indentation grammar (design "Predefined queries"). Views reference
// them by name via startup_named_query, and the UIs offer them in a dropdown.
package ddl

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// NamedQuery is one predefined query.
type NamedQuery struct {
	Name   string
	View   string  // view it is offered on; "" = every view
	Sort   float64 // dropdown order; unspecified sorts first in file order
	Active bool
	SQL    string
	Line   int
}

// QueriesPath is the sidecar convention: the DDL path with its extension
// replaced by ".queries" (example.ddl -> example.queries).
func QueriesPath(ddlPath string) string {
	return strings.TrimSuffix(ddlPath, filepath.Ext(ddlPath)) + ".queries"
}

// ScriptPath is the code sidecar: same base name, ".lua". The DDL's code:
// keys name functions defined in it.
func ScriptPath(ddlPath string) string {
	return strings.TrimSuffix(ddlPath, filepath.Ext(ddlPath)) + ".lua"
}

// ParseQueriesFile reads the sidecar; a missing file simply means no queries.
func ParseQueriesFile(path string) ([]NamedQuery, []string, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	queries, warns, err := ParseQueries(b)
	if err != nil {
		return nil, warns, fmt.Errorf("%s: %w", path, err)
	}
	return queries, warns, nil
}

// ParseQueries parses queries text. Soft problems (nameless, duplicate, no
// SQL) drop the entry with a warning, matching DDL load validation; only
// unparseable structure errors. The result is dropdown-ordered: by sort
// value, file order within ties.
func ParseQueries(src []byte) ([]NamedQuery, []string, error) {
	root, warns, err := buildTree(src)
	if err != nil {
		return nil, warns, err
	}
	normalize(root, &warns)
	var out []NamedQuery
	seen := map[string]bool{}
	for _, queryNode := range root.all("query_name") {
		q := NamedQuery{Active: true, Line: queryNode.Line}
		q.Name, _ = Unquote(queryNode.Value)
		if q.Name == "" {
			warns = append(warns, fmt.Sprintf("line %d: query with no name, dropped", queryNode.Line))
			continue
		}
		if seen[q.Name] {
			warns = append(warns, fmt.Sprintf(
				"line %d: query %q already defined; the first definition wins", queryNode.Line, q.Name))
			continue
		}
		for _, child := range queryNode.Children {
			switch strings.ToLower(child.Key) {
			case "view":
				q.View, _ = Unquote(child.Value)
			case "sort":
				if f, ok := AsFloat(child.Value); ok {
					q.Sort = f
				} else if child.Value != "" {
					warns = append(warns, fmt.Sprintf(
						"line %d: query %q sort %q is not a number, ignored", child.Line, q.Name, child.Value))
				}
			case "active":
				if b, ok := AsBool(child.Value); ok {
					q.Active = b
				}
			case "sql":
				if sql, ok := AsSQL(child.Value); ok {
					q.SQL = sql
				} else {
					q.SQL, _ = Unquote(strings.TrimSpace(child.Value)) // backticks preferred, but be lenient
				}
			default:
				warns = append(warns, fmt.Sprintf(
					"line %d: query %q has unknown key %q (ignored)", child.Line, q.Name, child.Key))
			}
		}
		if q.SQL == "" {
			warns = append(warns, fmt.Sprintf("line %d: query %q has no SQL, dropped", queryNode.Line, q.Name))
			continue
		}
		seen[q.Name] = true
		out = append(out, q)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Sort < out[j].Sort })
	return out, warns, nil
}
