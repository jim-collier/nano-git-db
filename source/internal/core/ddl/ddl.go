// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// Package ddl parses the nano-git-db DDL into an in-memory schema model.
//
// Three layers: buildTree (node.go) turns text into a generic indentation tree,
// value.go interprets the special value types on demand, and the mappers here
// walk the tree into the typed Schema. Most attributes are optional, so scalars
// that carry an unset/default distinction use pointers (nil = "use the default").
package ddl

import (
	"fmt"
	"os"
	"strings"
)

// Schema is a parsed DDL file.
type Schema struct {
	AppCode     map[string]string // before_open, after_open, ... (DDL `code:`, formerly `methods:`)
	Tunables    map[string]string // tunables: section, raw values (see TunableInt)
	Tables      []Table
	Relations   []Relationship
	Views       []View
	DefaultView string   // ui: default_view; empty means "first view defined"
	Encryption  string   // database-level encryption: always|never|auto ("" = unset)
	Warnings    []string // soft issues; parsing still succeeds
}

// encValues gates the encryption: key like every other load validation. always
// and never lock lower levels; auto (the default) defers up or down.
var encValues = map[string]bool{"": true, "always": true, "never": true, "auto": true}

// encOf reads and validates an encryption: child. An unknown value warns and is
// treated as unset (auto).
func encOf(n *Node, s *Schema, where string) string {
	child := n.child("encryption")
	if child == nil {
		return ""
	}
	value, _ := Unquote(strings.TrimSpace(child.Value))
	value = strings.ToLower(value)
	if !encValues[value] {
		s.Warnings = append(s.Warnings,
			fmt.Sprintf("line %d: %s encryption %q is not always|never|auto, ignored", child.Line, where, value))
		return ""
	}
	return value
}

// knownTunables gets a warning gate like every other load validation; the
// value still stores, so a newer client's tunable survives a round trip.
var knownTunables = map[string]bool{"git_sync_frequency": true, "gc_age_days": true}

// TunableInt reads a tunable as an integer, falling back on absent or
// non-numeric values.
func (s *Schema) TunableInt(key string, def int) int {
	value, ok := s.Tunables[key]
	if !ok {
		return def
	}
	num, ok := AsInt(value)
	if !ok {
		return def
	}
	return num
}

// AccessRule is one allow/deny pair. Empty whitelist means "all".
type AccessRule struct {
	Whitelist []string
	Blacklist []string
}

// Access is table/field-level access split by operation.
type Access struct {
	Read   AccessRule
	Write  AccessRule
	Delete AccessRule
}

// Features are the opt-in per-table capabilities.
type Features struct {
	LocalAttachments bool
	URIAttachments   bool
	Comments         bool
	AuditTrail       bool
	RowLevelAccess   bool
}

// Table is one DDL table.
type Table struct {
	Name       string
	Aliases    []string
	Access     Access
	Fields     []Field
	Code       map[string]string // before_update, after_update (DDL `code:`, formerly `methods:`)
	Uniques    [][]string        // each group of field names (auto-named, indexed)
	Indexes    [][]string        // each index
	Features   Features
	Encryption string // table-level encryption: always|never|auto ("" = unset)
	// NoSystemFields (DDL `system_fields: no`) drops the auto-added
	// is_active/date_created/is_deleted columns; `id` is always managed.
	// Used by the built-in audit_trail table, which defines exactly its own
	// fields per the design.
	NoSystemFields bool
	Line           int
}

// Validation holds a field's validation rules.
type Validation struct {
	Required *bool
	MinLen   *int
	MaxLen   *int
	MinVal   *float64
	MaxVal   *float64
	Regex    string
	Method   string // script function name, raw
}

// FieldUI holds a field's presentation hints.
type FieldUI struct {
	Visible    *bool
	Title      string
	Desc       string
	Order      *float64
	Readonly   *bool
	Width      *int
	Widget     string
	ListType   string
	ListSource string // raw: mixed literals and/or backtick SQL
	Format     string
}

// Field is one column.
type Field struct {
	Name       string
	Aliases    []string
	Access     Access
	Type       string
	Encryption string // field-level encryption: always|never|auto ("" = unset)
	Special    string
	IsActive   *bool
	Default    string // raw: NULL, a func() ref, or a literal
	NullOK     *bool
	EmptyOK    *bool
	Validation Validation
	Code       map[string]string
	UI         FieldUI
	Line       int
}

// Relationship is a 1:m or m:m link.
type Relationship struct {
	Type             string // 1:m or m:m
	Parent           string
	Child            string
	ParentIDField    string
	CascadeDelete    bool
	EnableAuditTrail bool
	Line             int
}

// Block is one (possibly nested) view layout block.
type Block struct {
	Name        string
	Table       string
	Type        string // form|grid|tree_grid
	ParentField string // tree_grid: same-table field naming the parent row
	Location    []string
	Readonly    *bool
	Children    []Block
}

// View is a UI view over tables.
type View struct {
	Name              string
	Aliases           []string
	StartupNamedQuery string
	Readonly          *bool
	Access            AccessRule // views use a flat rule, not read/write/delete
	Layout            []Block
	Line              int
}

// Parse turns DDL source into a Schema.
func Parse(src []byte) (*Schema, error) {
	root, warns, err := buildTree(src)
	if err != nil {
		return nil, err
	}
	normalize(root, &warns)
	s := mapSchema(root)
	s.Warnings = append(warns, s.Warnings...)
	return s, nil
}

// mapTunables reads a tunables: section. Both `key: value` children and the
// design doc's `key = value` list form are accepted.
func (s *Schema) mapTunables(n *Node) {
	if s.Tunables == nil {
		s.Tunables = map[string]string{}
	}
	for _, child := range n.Children {
		key, val := child.Key, child.Value
		if child.List {
			var ok bool
			key, val, ok = strings.Cut(child.Value, "=")
			if !ok {
				s.Warnings = append(s.Warnings,
					fmt.Sprintf("line %d: tunable %q is not key: value or key = value, ignored", child.Line, child.Value))
				continue
			}
			key = strings.TrimSpace(key)
		}
		if !knownTunables[key] {
			s.Warnings = append(s.Warnings, fmt.Sprintf("line %d: unknown tunable %q", child.Line, key))
		}
		if _, dup := s.Tunables[key]; dup {
			continue // first value wins, like every other merge
		}
		s.Tunables[key], _ = Unquote(strings.TrimSpace(val))
	}
}

// repeatable keys legitimately recur at one level and stay separate entities;
// every other duplicated key merges into its first occurrence (see normalize).
var repeatable = map[string]bool{
	"table": true, "field": true, "relationship": true, "view": true, "block": true,
	"query_name": true, // the queries sidecar file shares this grammar
}

// ParseFile reads and parses a DDL file.
func ParseFile(path string) (*Schema, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

func mapSchema(root *Node) *Schema {
	s := &Schema{AppCode: map[string]string{}}
	s.mapSections(root)
	return s
}

// mapSections walks one level of section keys. `database:` and `ui:` are
// transparent wrappers (recursed with the same switch), so both the nested
// layout and the older flat one parse; normalize already merged duplicates,
// so each wrapper appears at most once.
func (s *Schema) mapSections(n *Node) {
	for _, sec := range n.Children {
		switch sec.Key {
		case "code", "methods": // `methods:` renamed to `code:` 2026-07; keep reading both
			collectCode(sec, s.AppCode)
		case "database", "ui":
			s.mapSections(sec)
		case "default_view":
			s.DefaultView, _ = Unquote(sec.Value)
		case "encryption":
			if v, _ := Unquote(strings.TrimSpace(sec.Value)); encValues[strings.ToLower(v)] {
				s.Encryption = strings.ToLower(v)
			} else {
				s.Warnings = append(s.Warnings,
					fmt.Sprintf("line %d: database encryption %q is not always|never|auto, ignored", sec.Line, v))
			}
		case "tunables":
			s.mapTunables(sec)
		case "tables":
			for _, tableNode := range sec.all("table") {
				tbl := parseTable(tableNode, s)
				if tbl.Name == "" {
					s.Warnings = append(s.Warnings,
						fmt.Sprintf("line %d: table with no name, dropped", tableNode.Line))
					continue
				}
				if prev := s.table(tbl.Name); prev != nil {
					s.Warnings = append(s.Warnings, fmt.Sprintf(
						"line %d: table %q already defined on line %d; the first definition wins", tableNode.Line, tbl.Name, prev.Line))
					continue
				}
				s.Tables = append(s.Tables, tbl)
			}
		case "relationships":
			for _, r := range sec.all("relationship") {
				s.Relations = append(s.Relations, parseRelationship(r))
			}
		case "views":
			for _, v := range sec.all("view") {
				s.Views = append(s.Views, parseView(v))
			}
		case "":
			// stray list item; ignore
		default:
			s.Warnings = append(s.Warnings,
				fmt.Sprintf("line %d: unknown section %q (ignored)", sec.Line, sec.Key))
		}
	}
}

func parseTable(n *Node, s *Schema) Table {
	name, _ := Unquote(n.Value)
	t := Table{Name: name, Line: n.Line, Code: map[string]string{}}
	t.Aliases = listOf(n, "aliases")
	t.Encryption = encOf(n, s, "table "+name)
	if systemFields := boolOf(n, "system_fields"); systemFields != nil && !*systemFields {
		t.NoSystemFields = true
	}
	if accessNode := n.child("access"); accessNode != nil {
		t.Access = parseAccessRWD(accessNode)
	}
	if fieldsNode := n.child("fields"); fieldsNode != nil {
		for _, fieldNode := range fieldsNode.all("field") {
			field := parseField(fieldNode, s)
			if field.Name == "" {
				s.Warnings = append(s.Warnings,
					fmt.Sprintf("line %d: field with no name in table %q, dropped", fieldNode.Line, t.Name))
				continue
			}
			// hasField covers both a redefined field and a collision with the
			// auto-added system columns - either would double a column.
			if t.hasField(field.Name) {
				what := "already defined; the first definition wins"
				if field.Name == "id" || !t.NoSystemFields &&
					(field.Name == "is_active" || field.Name == "is_deleted" || field.Name == "date_created") {
					what = "collides with an auto-added system field, dropped"
				}
				s.Warnings = append(s.Warnings,
					fmt.Sprintf("line %d: field %q in table %q %s", fieldNode.Line, field.Name, t.Name, what))
				continue
			}
			t.Fields = append(t.Fields, field)
		}
	}
	if codeNode := firstChild(n, "code", "methods"); codeNode != nil {
		collectCode(codeNode, t.Code)
	}
	if uniquesNode := n.child("uniques"); uniquesNode != nil {
		for _, row := range uniquesNode.items() {
			group := SplitList(row.Value)
			t.Uniques = append(t.Uniques, group)
			warnUnknownFields(s, &t, row.Line, "uniques", group)
		}
	}
	if indexesNode := n.child("indexes"); indexesNode != nil {
		for _, row := range indexesNode.items() {
			group := SplitList(row.Value)
			t.Indexes = append(t.Indexes, group)
			warnUnknownFields(s, &t, row.Line, "indexes", group)
		}
	}
	if featuresNode := n.child("features"); featuresNode != nil {
		t.Features = Features{
			LocalAttachments: boolDefault(featuresNode, "local_attachments"),
			URIAttachments:   boolDefault(featuresNode, "uri_attachments"),
			Comments:         boolDefault(featuresNode, "comments"),
			AuditTrail:       boolDefault(featuresNode, "audit_trail"),
			RowLevelAccess:   boolDefault(featuresNode, "row_level_access"),
		}
	}
	return t
}

// fieldTypes are the DDL's data types (design: string|int|float|bool|
// datetime[_local]|datetime_utc|binary); anything else stores as text.
var fieldTypes = map[string]bool{
	"": true, "string": true, "int": true, "float": true, "bool": true,
	"datetime": true, "datetime_local": true, "datetime_utc": true, "binary": true,
}

func parseField(n *Node, s *Schema) Field {
	name, _ := Unquote(n.Value)
	f := Field{Name: name, Line: n.Line, Code: map[string]string{}}
	f.Aliases = listOf(n, "aliases")
	if accessNode := n.child("access"); accessNode != nil {
		f.Access = parseAccessRWD(accessNode)
	}
	f.Type = strings.ToLower(strOf(n, "type"))
	if !fieldTypes[f.Type] {
		s.Warnings = append(s.Warnings, fmt.Sprintf(
			"line %d: field %q has unknown type %q, stored as text", n.Line, name, f.Type))
	}
	f.Encryption = encOf(n, s, "field "+name)
	f.Special = strOf(n, "special")
	f.IsActive = boolOf(n, "isactive")
	f.Default = rawOf(n, "defaultval")
	f.NullOK = boolOf(n, "null_ok")
	f.EmptyOK = boolOf(n, "empty_ok")
	if validationNode := n.child("validation"); validationNode != nil {
		f.Validation = Validation{
			Required: boolOf(validationNode, "required"),
			MinLen:   intOf(validationNode, "minlen"),
			MaxLen:   intOf(validationNode, "maxlen"),
			MinVal:   floatOf(validationNode, "minval"),
			MaxVal:   floatOf(validationNode, "maxval"),
			Regex:    strOf(validationNode, "regex"),
			Method:   rawOf(validationNode, "method"),
		}
	}
	if codeNode := firstChild(n, "code", "methods"); codeNode != nil {
		collectCode(codeNode, f.Code)
	}
	if uiNode := n.child("ui"); uiNode != nil {
		f.UI = FieldUI{
			Visible:    boolOf(uiNode, "visible"),
			Title:      strOf(uiNode, "title"),
			Desc:       strOf(uiNode, "description"),
			Order:      floatOf(uiNode, "order"),
			Readonly:   boolOf(uiNode, "readonly"),
			Width:      intOf(uiNode, "width"),
			Widget:     strOf(uiNode, "widget"),
			ListType:   strOf(uiNode, "list_type"),
			ListSource: rawOf(uiNode, "list_source"),
			Format:     strOf(uiNode, "format"),
		}
	}
	return f
}

func parseRelationship(n *Node) Relationship {
	return Relationship{
		Type:             strings.ToLower(strOf(n, "type")),
		Parent:           strOf(n, "parent"),
		Child:            strOf(n, "child"),
		ParentIDField:    strOf(n, "parent_id_field"),
		CascadeDelete:    boolDefault(n, "cascade_delete"),
		EnableAuditTrail: boolDefault(n, "enable_audit_trail"),
		Line:             n.Line,
	}
}

func parseView(n *Node) View {
	name, _ := Unquote(n.Value)
	v := View{Name: name, Line: n.Line}
	v.Aliases = listOf(n, "aliases")
	// Renamed from default_named_query 2026-07; keep reading the old key so
	// existing DDLs parse. Unquoted: it must compare equal to a query's name.
	v.StartupNamedQuery, _ = Unquote(rawOf(n, "startup_named_query"))
	if v.StartupNamedQuery == "" {
		v.StartupNamedQuery, _ = Unquote(rawOf(n, "default_named_query"))
	}
	v.Readonly = boolOf(n, "readonly")
	if accessNode := n.child("access"); accessNode != nil {
		v.Access = accessFlat(accessNode)
	}
	if layoutNode := n.child("layout"); layoutNode != nil {
		for _, blockNode := range layoutNode.all("block") {
			v.Layout = append(v.Layout, parseBlock(blockNode))
		}
	}
	return v
}

func parseBlock(n *Node) Block {
	name, _ := Unquote(n.Value)
	b := Block{
		Name:        name,
		Table:       strOf(n, "table"),
		Type:        strOf(n, "type"),
		ParentField: strOf(n, "parent_field"),
		Location:    listOf(n, "location"),
		Readonly:    boolOf(n, "readonly"),
	}
	for _, childNode := range n.all("block") {
		b.Children = append(b.Children, parseBlock(childNode))
	}
	return b
}

func parseAccessRWD(n *Node) Access {
	return Access{
		Read:   accessFlat(n.child("read")),
		Write:  accessFlat(n.child("write")),
		Delete: accessFlat(n.child("delete")),
	}
}

// accessFlat reads whitelist/blacklist from a node. Early DDLs (including the
// original example) misspelled it "blaclist"; keep accepting both so they parse.
func accessFlat(n *Node) AccessRule {
	if n == nil {
		return AccessRule{}
	}
	return AccessRule{
		Whitelist: firstList(n, "whitelist"),
		Blacklist: firstList(n, "blacklist", "blaclist"),
	}
}

// warnUnknownFields flags uniques/indexes rows naming fields the table doesn't
// have - caught here at parse time instead of as an opaque SQLite error later.
func warnUnknownFields(s *Schema, t *Table, line int, kind string, fields []string) {
	for _, name := range fields {
		if !t.hasField(name) {
			s.Warnings = append(s.Warnings, fmt.Sprintf(
				"line %d: %s names unknown field %q in table %q", line, kind, name, t.Name))
		}
	}
}

// table finds an already-parsed table by name.
func (s *Schema) table(name string) *Table {
	for i := range s.Tables {
		if s.Tables[i].Name == name {
			return &s.Tables[i]
		}
	}
	return nil
}

func (t *Table) field(name string) *Field {
	for i := range t.Fields {
		if t.Fields[i].Name == name {
			return &t.Fields[i]
		}
	}
	return nil
}

// HasAlwaysEncryption reports whether any level declares encryption: always -
// i.e. writing needs a key. Used to warn when a key is missing.
func (s *Schema) HasAlwaysEncryption() bool {
	if s.Encryption == "always" {
		return true
	}
	for _, t := range s.Tables {
		if t.Encryption == "always" {
			return true
		}
		for _, f := range t.Fields {
			if f.Encryption == "always" {
				return true
			}
		}
	}
	return false
}

// EncryptionPolicy resolves a field's encryption directive, scanning
// database -> table -> field outermost first. The first always|never locks the
// result - a lower level cannot override it (only a higher one can). If every
// level is auto or unset the result is "auto": no schema lock, so the caller's
// local preference and key presence decide. Unknown tables/fields inherit the
// database (then "auto") level.
func (s *Schema) EncryptionPolicy(table, field string) string {
	levels := []string{s.Encryption}
	if t := s.table(table); t != nil {
		levels = append(levels, t.Encryption)
		if f := t.field(field); f != nil {
			levels = append(levels, f.Encryption)
		}
	}
	for _, level := range levels {
		if level == "always" || level == "never" {
			return level
		}
	}
	return "auto"
}

func (t *Table) hasField(name string) bool {
	switch name {
	case "id":
		return true // id is managed on every table, opt-out or not
	case "is_active", "date_created", "is_deleted":
		return !t.NoSystemFields
	}
	for _, f := range t.Fields {
		if f.Name == name {
			return true
		}
	}
	return false
}

// -- small node accessors --

func collectCode(n *Node, dst map[string]string) {
	for _, child := range n.Children {
		if child.Key != "" && child.Value != "" {
			dst[child.Key] = child.Value
		}
	}
}

func firstChild(n *Node, keys ...string) *Node {
	for _, key := range keys {
		if child := n.child(key); child != nil {
			return child
		}
	}
	return nil
}

func firstList(n *Node, keys ...string) []string {
	for _, key := range keys {
		if child := n.child(key); child != nil {
			return SplitList(child.Value)
		}
	}
	return nil
}

func listOf(n *Node, key string) []string {
	if child := n.child(key); child != nil {
		return SplitList(child.Value)
	}
	return nil
}

func strOf(n *Node, key string) string {
	if child := n.child(key); child != nil {
		s, _ := Unquote(child.Value)
		return s
	}
	return ""
}

func rawOf(n *Node, key string) string {
	if child := n.child(key); child != nil {
		return child.Value
	}
	return ""
}

func boolOf(n *Node, key string) *bool {
	child := n.child(key)
	if child == nil || child.Value == "" {
		return nil
	}
	if b, ok := AsBool(child.Value); ok {
		return &b
	}
	return nil
}

// boolDefault is boolOf with a false default (for keys whose default is off).
func boolDefault(n *Node, key string) bool {
	if val := boolOf(n, key); val != nil {
		return *val
	}
	return false
}

func intOf(n *Node, key string) *int {
	child := n.child(key)
	if child == nil || child.Value == "" {
		return nil
	}
	if i, ok := AsInt(child.Value); ok {
		return &i
	}
	return nil
}

func floatOf(n *Node, key string) *float64 {
	child := n.child(key)
	if child == nil || child.Value == "" {
		return nil
	}
	if f, ok := AsFloat(child.Value); ok {
		return &f
	}
	return nil
}
