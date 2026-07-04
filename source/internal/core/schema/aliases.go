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
	tableAliases := map[string]string{}
	fieldAliases := map[string]map[string]string{}
	for _, s := range schemas {
		for _, table := range s.Tables {
			for _, alias := range table.Aliases {
				if alias != "" && tableAliases[alias] == "" {
					tableAliases[alias] = table.Name
				}
			}
			fieldMap := fieldAliases[table.Name]
			for _, field := range table.Fields {
				for _, alias := range field.Aliases {
					if alias == "" {
						continue
					}
					if fieldMap == nil {
						fieldMap = map[string]string{}
						fieldAliases[table.Name] = fieldMap
					}
					if fieldMap[alias] == "" {
						fieldMap[alias] = field.Name
					}
				}
			}
		}
	}
	if len(tableAliases) == 0 && len(fieldAliases) == 0 {
		return
	}
	for i := range entries {
		if newName := tableAliases[entries[i].Table]; newName != "" {
			entries[i].Table = newName
		}
		if fieldMap := fieldAliases[entries[i].Table]; fieldMap != nil {
			if newName := fieldMap[entries[i].Field]; newName != "" {
				entries[i].Field = newName
			}
		}
	}
}
