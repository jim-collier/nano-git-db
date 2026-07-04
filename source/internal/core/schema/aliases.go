// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// Alias translation: tx-log entries recorded under a table's or field's
// former name (the DDL's aliases:, maintained by the rename schema ops)
// rewrite to the current names before replay, so a rename never strands
// history. The log itself is immutable - old lines keep old names forever.
package schema

import (
	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/txlog"
)

// ApplyAliases rewrites entries in place. First definition wins on
// conflicting aliases, matching every other schema merge in the project.
func ApplyAliases(entries []txlog.Entry, schemas ...*ddl.Schema) {
	tbl := map[string]string{}
	fld := map[string]map[string]string{}
	for _, s := range schemas {
		for _, t := range s.Tables {
			for _, al := range t.Aliases {
				if al != "" && tbl[al] == "" {
					tbl[al] = t.Name
				}
			}
			m := fld[t.Name]
			for _, f := range t.Fields {
				for _, al := range f.Aliases {
					if al == "" {
						continue
					}
					if m == nil {
						m = map[string]string{}
						fld[t.Name] = m
					}
					if m[al] == "" {
						m[al] = f.Name
					}
				}
			}
		}
	}
	if len(tbl) == 0 && len(fld) == 0 {
		return
	}
	for i := range entries {
		if n := tbl[entries[i].Table]; n != "" {
			entries[i].Table = n
		}
		if m := fld[entries[i].Table]; m != nil {
			if n := m[entries[i].Field]; n != "" {
				entries[i].Field = n
			}
		}
	}
}
