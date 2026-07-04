// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// Schema-op text rewriting: rename a table or field IN the DDL file while
// recording the old name as an alias, so tx-log entries written before the
// rename still resolve (schema.ApplyAliases does the replay-side mapping).
// Line-based on purpose - comments, blank lines, and the author's formatting
// survive untouched except for the lines that must change.
package ddl

import (
	"strings"
)

// RenameTable rewrites every `table: old` to the new name and records the
// old name on the first one's aliases. Returns the new text and whether the
// table was found.
func RenameTable(src []byte, oldName, newName string) ([]byte, bool) {
	lines := strings.Split(string(src), "\n")
	found, aliased := false, false
	for i := 0; i < len(lines); i++ {
		key, val, ok := lineKeyVal(lines[i])
		if !ok || key != "table" || val != oldName {
			continue
		}
		lines[i] = replaceLineValue(lines[i], newName)
		found = true
		if !aliased {
			aliased = true
			lines = addAlias(lines, i, oldName)
			i++ // the possible insertion shifts the scan by at most one line
		}
	}
	if !found {
		return src, false
	}
	return []byte(strings.Join(lines, "\n")), true
}

// RenameField rewrites `field: old` inside the given table's block(s), same
// contract as RenameTable.
func RenameField(src []byte, table, oldName, newName string) ([]byte, bool) {
	lines := strings.Split(string(src), "\n")
	found, aliased := false, false
	tblIndent := -1 // >=0 while inside the target table's block
	for i := 0; i < len(lines); i++ {
		key, val, ok := lineKeyVal(lines[i])
		if !ok {
			continue // blank/comment-only lines never close a block
		}
		ind := indentWidth(lines[i])
		if tblIndent >= 0 && ind <= tblIndent {
			tblIndent = -1
		}
		if key == "table" {
			if val == table {
				tblIndent = ind
			}
			continue
		}
		if tblIndent < 0 || key != "field" || val != oldName {
			continue
		}
		lines[i] = replaceLineValue(lines[i], newName)
		found = true
		if !aliased {
			aliased = true
			lines = addAlias(lines, i, oldName)
			i++
		}
	}
	if !found {
		return src, false
	}
	return []byte(strings.Join(lines, "\n")), true
}

// lineKeyVal parses one line's `key: value` (comment stripped, value
// unquoted); ok is false for blank, comment-only, or bare-list lines.
func lineKeyVal(line string) (key, val string, ok bool) {
	out, _ := stripComment(line)
	content := strings.TrimSpace(out)
	if content == "" {
		return "", "", false
	}
	p := findColon(content)
	if p < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(content[:p])
	if !isPathKey(key) {
		return "", "", false
	}
	val, _ = Unquote(strings.TrimSpace(content[p+1:]))
	return key, val, true
}

func indentWidth(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}

// replaceLineValue swaps the value after the first colon, preserving the
// key, its indentation, and any trailing comment.
func replaceLineValue(line, newVal string) string {
	out, _ := stripComment(line)
	comment := line[len(out):]
	p := findColon(out)
	if !isIdent(newVal) {
		newVal = `"` + newVal + `"`
	}
	res := out[:p+1] + " " + newVal
	if comment != "" {
		res += "  " + strings.TrimLeft(comment, " ")
	}
	return res
}

// addAlias records the old name on the entity at lines[i]: extends its
// existing aliases: child, or inserts one as the first child.
func addAlias(lines []string, i int, oldName string) []string {
	entIndent := indentWidth(lines[i])
	alias := oldName
	if !isIdent(alias) {
		alias = `"` + alias + `"`
	}
	childIndent := ""
	for j := i + 1; j < len(lines); j++ {
		key, _, ok := lineKeyVal(lines[j])
		if !ok {
			continue // blank/comment/list lines neither close nor anchor
		}
		ind := indentWidth(lines[j])
		if ind <= entIndent {
			break
		}
		if childIndent == "" {
			childIndent = lines[j][:ind] // first child sets the direct-child level
		}
		if ind == len(childIndent) && key == "aliases" {
			out, _ := stripComment(lines[j])
			comment := lines[j][len(out):]
			p := findColon(out)
			cur := strings.TrimSpace(out[p+1:])
			if cur == "" {
				lines[j] = out[:p+1] + " " + alias
			} else {
				lines[j] = strings.TrimRight(out, " \t") + ", " + alias
			}
			if comment != "" {
				lines[j] += "  " + strings.TrimLeft(comment, " ")
			}
			return lines
		}
	}
	if childIndent == "" {
		childIndent = lines[i][:entIndent] + "\t"
	}
	ins := childIndent + "aliases: " + alias
	return append(lines[:i+1], append([]string{ins}, lines[i+1:]...)...)
}
