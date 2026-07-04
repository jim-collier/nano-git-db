// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// Read-side access filtering (design "Optional granular access model"): a
// process serves one user, so the catalog simply hides what that user may
// not read - tables, fields, views, and row-granted rows. Write enforcement
// lives in crud (access.go there); this is the UI half.
package schema

import (
	"github.com/jim-collier/nano-git-db/internal/core/crud"
	"github.com/jim-collier/nano-git-db/internal/core/ddl"
)

// applyAccess prunes the catalog to the user's read access. tableAccess is
// the first-wins table rule map gathered while walking the schemas. Runs
// after resolveViews (views bind tables) and before resolveDefaultView (the
// default must be a view the user can open).
func (c *Catalog) applyAccess(api *crud.API, tableAccess map[string]ddl.Access, fieldAccess map[string]map[string]ddl.Access) {
	groups, _, err := api.UserGroups()
	if err != nil {
		// no users/groups tables (bare store): nothing is configured, stay open
		return
	}
	readable := func(t string) bool { return tableAccess[t].AllowedFor("read", groups) }

	kept := c.Tables[:0]
	for _, t := range c.Tables {
		if !readable(t) {
			c.warnf("table %q hidden: read access denied", t)
			delete(c.Fields, t) // Has() now refuses it, so the front-ends 404
			continue
		}
		kept = append(kept, t)
	}
	c.Tables = kept

	for t, fields := range c.Fields {
		fk := fields[:0]
		for _, f := range fields {
			if ac, ok := fieldAccess[t][f]; ok && !ac.AllowedFor("read", groups) {
				continue // silently: a hidden column is not an error
			}
			fk = append(fk, f)
		}
		c.Fields[t] = fk
	}

	// A view needs its own flat rule to pass AND read access to its main
	// (first) table, per the design.
	vk := c.Views[:0]
	for i := range c.Views {
		v := c.Views[i]
		open := groups["owners"] || groups["admins"] || v.Access.Allows(groups)
		if open && len(v.Leaves) > 0 {
			open = readable(v.Leaves[0].Table)
		}
		if !open {
			c.warnf("view %q hidden: access denied", v.Name)
			continue
		}
		vk = append(vk, v)
	}
	c.Views = vk
}

// liveRowsAllowed filters a row-level-access table's rows down to the ones
// the user may see: rows with no access_rows entry (or an entry with no
// grants) are open, granted rows need a shared group. Owners/admins see all.
func (c *Catalog) liveRowsAllowed(api *crud.API, table string, rows []map[string]string) []map[string]string {
	if !c.Features[table].RowLevelAccess {
		return rows
	}
	groups, gids, err := api.UserGroups()
	if err != nil || groups["owners"] || groups["admins"] {
		return rows // hiding is UI-level; the hard gate is crud's
	}
	ar, err := api.Query(`SELECT "id","parent_id" FROM "access_rows"
		WHERE "table_name"=? AND "is_deleted"=0`, table)
	if err != nil || len(ar) == 0 {
		return rows
	}
	blocked := map[string]bool{}
	for _, entry := range ar {
		granted, err := api.Links("access_rows", entry["id"], "groups")
		if err != nil || len(granted) == 0 {
			continue
		}
		ok := false
		for _, g := range granted {
			if gids[g] {
				ok = true
				break
			}
		}
		if !ok {
			blocked[entry["parent_id"]] = true
		}
	}
	if len(blocked) == 0 {
		return rows
	}
	kept := rows[:0]
	for _, r := range rows {
		if !blocked[r["id"]] {
			kept = append(kept, r)
		}
	}
	return kept
}
