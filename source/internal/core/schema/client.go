// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// Client startup and table metadata shared by the interactive front-ends
// (TUI, web, Lua) so their bring-up sequence cannot drift.
package schema

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jim-collier/nano-git-db/enc"
	"github.com/jim-collier/nano-git-db/internal/core/crud"
	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/store"
	"github.com/jim-collier/nano-git-db/internal/core/txlog"
)

// Client is one fully-opened database: view migrated, built-ins bootstrapped,
// log replayed, defaults seeded. Callers must Close it.
type Client struct {
	Schema   *ddl.Schema
	Store    *store.Store
	API      *crud.API
	Queries  []ddl.NamedQuery // the .queries sidecar, dropdown-ordered
	Warnings []string

	log *txlog.Log // for StartAutoSync
}

// OpenOpts parameterizes the bring-up. The zero value (only paths + userID set)
// means "no encryption": KeyFile empty -> no key loaded, EncryptPref empty ->
// auto. Registered databases pass their KeyFile and stored preference; ad-hoc
// opens pass config.LocalKey and the --encrypt override.
type OpenOpts struct {
	DDLPath, DBPath, LogDir, UserID string
	KeyFile                         string // encryption key file ("" = none)
	EncryptPref                     string // on|off|auto ("" = auto)
}

// OpenClient is the standard front-end bring-up with no encryption options.
func OpenClient(ddlPath, dbPath, logDir, userID string) (*Client, error) {
	return OpenClientWith(OpenOpts{DDLPath: ddlPath, DBPath: dbPath, LogDir: logDir, UserID: userID})
}

// OpenClientWith parses the DDL, opens and additively migrates the view,
// bootstraps the built-in tables, loads the encryption key (if any), replays
// the tx-log (decrypting field values along the way), then seeds the default
// groups (after the replay, so an already-seeded log is a no-op).
func OpenClientWith(o OpenOpts) (*Client, error) {
	sch, err := ddl.ParseFile(o.DDLPath)
	if err != nil {
		return nil, err
	}
	queries, qw, err := ddl.ParseQueriesFile(ddl.QueriesPath(o.DDLPath))
	if err != nil {
		return nil, err
	}
	st, err := store.Open(o.DBPath)
	if err != nil {
		return nil, err
	}
	fail := func(e error) (*Client, error) {
		st.Close()
		return nil, e
	}
	if err := st.Build(sch); err != nil {
		return fail(err)
	}
	bw, err := Bootstrap(st, sch)
	if err != nil {
		return fail(err)
	}
	lg, err := txlog.Open(o.LogDir)
	if err != nil {
		return fail(err)
	}
	bs, err := Builtins()
	if err != nil {
		return fail(err)
	}
	cipher, hasKey, err := enc.LoadCipher(o.KeyFile) // a corrupt key file is fatal
	if err != nil {
		return fail(err)
	}
	entries, rw, err := lg.ReadAll()
	if err != nil {
		return fail(err)
	}
	ApplyAliases(entries, sch, bs)                     // pre-rename entries -> current names
	unreadable := crud.DecryptEntries(entries, cipher) // before Apply: view holds cleartext
	aw, err := txlog.Apply(st, entries)
	if err != nil {
		return fail(err)
	}
	aw = append(rw, aw...)
	if o.UserID == "" {
		o.UserID = crud.DefaultUserID()
	}
	pref := o.EncryptPref
	if pref == "" {
		pref = "auto"
	}
	api := crud.New(st, lg)
	api.UserID = o.UserID
	api.EnableEncryption(cipher, pref, sch)
	api.EnableFeatures(sch, bs) // before seeding, so the seeded groups get audited
	if err := SeedDefaults(api); err != nil {
		return fail(err)
	}
	aw = append(aw, encryptionWarnings(sch, hasKey, pref, unreadable)...)
	return &Client{
		Schema:   sch,
		Store:    st,
		API:      api,
		Queries:  queries,
		Warnings: append(append(append(sch.Warnings, qw...), bw...), aw...),
		log:      lg,
	}, nil
}

// encryptionWarnings surfaces the degraded/awareness states the design calls
// for: encrypted data the current run cannot read, an always-encrypted schema
// with no key (writes to those fields will be refused), and the "you have the
// key but turned encryption off" case.
func encryptionWarnings(sch *ddl.Schema, hasKey bool, pref string, unreadable int) []string {
	var w []string
	if unreadable > 0 {
		if hasKey {
			w = append(w, fmt.Sprintf("%d encrypted value(s) failed to decrypt (wrong key or tampering); shown empty", unreadable))
		} else {
			w = append(w, fmt.Sprintf("%d encrypted value(s) unreadable - no key file found; shown empty. Obtain the key to read them", unreadable))
		}
	}
	if !hasKey && sch.HasAlwaysEncryption() {
		w = append(w, "this database has always-encrypted fields but no key file; those fields cannot be written until the key is provided")
	}
	if hasKey && pref == "off" {
		w = append(w, "encryption is available (key present) but disabled for you (--encrypt=off); new data is written in the clear")
	}
	return w
}

// Close releases the client's store.
func (c *Client) Close() error { return c.Store.Close() }

// Catalog is the UI-facing table metadata: which tables exist, in what order,
// which fields are user-editable, which tables soft-delete, and the resolved
// renderable views.
type Catalog struct {
	Tables      []string
	Fields      map[string][]string     // editable (DDL-declared) fields per table
	HasDeleted  map[string]bool         // table has is_deleted (audit_trail does not)
	Features    map[string]ddl.Features // per-table opt-ins, for UI gating
	Views       []ViewSpec              // resolved views, DDL order
	DefaultView string                  // view the UIs open on start; "" only when no views resolved
	Queries     []ddl.NamedQuery        // predefined queries; front-ends copy Client.Queries in
	Warnings    []string                // view-resolution problems worth surfacing
}

// QueriesFor lists the active queries offered on a view, dropdown order.
// Queries with no view are offered everywhere.
func (c *Catalog) QueriesFor(view string) []ddl.NamedQuery {
	var out []ddl.NamedQuery
	for _, q := range c.Queries {
		if q.Active && (q.View == "" || q.View == view) {
			out = append(out, q)
		}
	}
	return out
}

// NamedQuery finds an active query by name, or nil.
func (c *Catalog) NamedQuery(name string) *ddl.NamedQuery {
	for i := range c.Queries {
		if c.Queries[i].Name == name && c.Queries[i].Active {
			return &c.Queries[i]
		}
	}
	return nil
}

// NewCatalog builds the catalog from the view plus one or more schemas.
// Earlier schemas win the ordering and field lists - pass the user schema
// first, then Builtins(). Only tables that exist in the view are listed.
func NewCatalog(api *crud.API, schemas ...*ddl.Schema) (*Catalog, error) {
	c := &Catalog{
		Fields:     map[string][]string{},
		HasDeleted: map[string]bool{},
		Features:   map[string]ddl.Features{},
	}
	rows, err := api.Query(
		`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return nil, err
	}
	existing := map[string]bool{}
	for _, r := range rows {
		existing[r["name"]] = true
	}

	seen := map[string]bool{}
	tableAccess := map[string]ddl.Access{}
	fieldAccess := map[string]map[string]ddl.Access{}
	for _, s := range schemas {
		for _, t := range s.Tables {
			if _, dup := c.Fields[t.Name]; dup {
				continue // earlier schema (the user's) wins
			}
			names := make([]string, len(t.Fields))
			fa := map[string]ddl.Access{}
			for i, f := range t.Fields {
				names[i] = f.Name
				fa[f.Name] = f.Access
			}
			c.Fields[t.Name] = names
			c.Features[t.Name] = t.Features
			tableAccess[t.Name] = t.Access
			fieldAccess[t.Name] = fa
			if existing[t.Name] && !seen[t.Name] {
				seen[t.Name] = true
				c.Tables = append(c.Tables, t.Name)
			}
		}
	}
	var rest []string
	for name := range existing {
		if !seen[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	c.Tables = append(c.Tables, rest...)

	for _, t := range c.Tables {
		cols, err := api.Query(`SELECT name FROM pragma_table_info(?)`, t)
		if err != nil {
			return nil, err
		}
		var plain []string
		for _, r := range cols {
			switch r["name"] {
			case "is_deleted":
				c.HasDeleted[t] = true
			case "id", "is_active", "date_created":
			default:
				plain = append(plain, r["name"])
			}
		}
		if len(c.Fields[t]) == 0 { // view-only fallback: everything but system cols
			c.Fields[t] = plain
		}
	}
	c.resolveViews(schemas...) // after Fields: tree_grid parent_field checks need them
	c.applyAccess(api, tableAccess, fieldAccess)
	c.resolveDefaultView(schemas...) // after access: the default must be openable
	return c, nil
}

// Has reports whether the catalog lists the table - the front-ends' guard
// against a table name arriving from user input.
func (c *Catalog) Has(table string) bool {
	_, ok := c.Fields[table]
	for _, t := range c.Tables {
		if t == table {
			return ok
		}
	}
	return false
}

// ColumnsFor is the grid column set: id first, then the editable fields.
func (c *Catalog) ColumnsFor(table string) []string {
	return append([]string{"id"}, c.Fields[table]...)
}

// LiveRows loads a table's visible rows; soft-deleted ones are hidden when
// the table has is_deleted, and row-level grants the user lacks hide theirs.
func (c *Catalog) LiveRows(api *crud.API, table string) ([]map[string]string, error) {
	q := `SELECT * FROM "` + strings.ReplaceAll(table, `"`, `""`) + `"`
	if c.HasDeleted[table] {
		q += ` WHERE "is_deleted"=0`
	}
	rows, err := api.Query(q)
	if err != nil {
		return nil, err
	}
	return c.liveRowsAllowed(api, table, rows), nil
}
