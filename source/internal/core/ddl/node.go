// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package ddl

import (
	"fmt"
	"strings"
)

// Node is one DDL line as an ordered tree. The DDL repeats keys at the same
// level (`table:`, `field:`, `relationship:`, `block:`), so children are an
// ordered slice, not a map. A bare line with no `key:` (a uniques/indexes row)
// is stored with List=true and Key="".
type Node struct {
	Key      string
	Value    string // raw text after the colon, comment-stripped and trimmed
	List     bool
	Line     int // 1-based source line
	Children []*Node
}

// child returns the first child with the given key, or nil.
func (n *Node) child(key string) *Node {
	for _, child := range n.Children {
		if child.Key == key {
			return child
		}
	}
	return nil
}

// all returns every child with the given key, in order.
func (n *Node) all(key string) []*Node {
	var out []*Node
	for _, child := range n.Children {
		if child.Key == key {
			out = append(out, child)
		}
	}
	return out
}

// items returns the bare list-item children (uniques/indexes rows).
func (n *Node) items() []*Node {
	var out []*Node
	for _, child := range n.Children {
		if child.List {
			out = append(out, child)
		}
	}
	return out
}

// buildTree parses DDL text into a node tree by indentation. Indentation must be
// consistent (all tabs or all spaces); the step size is inferred, not fixed.
// Soft issues (swallowed comments, odd indent widths) come back as warnings -
// this DDL targets users without SQL knowledge, so silence is the enemy.
func buildTree(src []byte) (*Node, []string, error) {
	root := &Node{}
	type frame struct {
		w int
		n *Node
	}
	stack := []frame{{-1, root}}
	var indentChar byte // 0 until the first indented line sets it
	var step int        // inferred indent step, from the first indented line
	var warns []string

	lines := strings.Split(string(src), "\n")
	for i, raw := range lines {
		lineNo := i + 1
		raw = strings.TrimRight(raw, "\r")

		stripped, hiddenHash := stripComment(raw)
		content := strings.TrimSpace(stripped)
		if content == "" {
			continue // blank or comment-only
		}
		if hiddenHash {
			warns = append(warns, fmt.Sprintf(
				"line %d: unclosed quote swallows a '#'; it is treated as content, not a comment", lineNo))
		}

		indent := stripped[:len(stripped)-len(strings.TrimLeft(stripped, " \t"))]
		for j := 0; j < len(indent); j++ {
			c := indent[j]
			if indentChar == 0 {
				indentChar = c
			}
			if c != indentChar {
				return nil, warns, fmt.Errorf("line %d: inconsistent indentation (mixed tabs and spaces)", lineNo)
			}
		}
		width := len(indent)
		if width > 0 {
			if step == 0 {
				step = width
			} else if width%step != 0 {
				warns = append(warns, fmt.Sprintf(
					"line %d: indent width %d is not a multiple of the inferred step %d; nesting may not be what it looks like", lineNo, width, step))
			}
		}

		for len(stack) > 1 && stack[len(stack)-1].w >= width {
			stack = stack[:len(stack)-1]
		}
		parent := stack[len(stack)-1].n

		node := &Node{Line: lineNo}
		if colon := findColon(content); colon >= 0 {
			if key := strings.TrimSpace(content[:colon]); isPathKey(key) {
				node.Key = key
				node.Value = strings.TrimSpace(content[colon+1:])
			}
		}
		if node.Key == "" {
			node.List = true
			node.Value = content
		}

		parent.Children = append(parent.Children, node)
		stack = append(stack, frame{width, node})
	}
	return root, warns, nil
}

// stripComment removes an unquoted '#' comment, respecting double-quote,
// single-quote, and backtick spans. hiddenHash reports a '#' inside a quote
// span that never closed - typically a stray apostrophe eating a real comment.
func stripComment(line string) (out string, hiddenHash bool) {
	var quote byte
	var sawHash bool
	for i := 0; i < len(line); i++ {
		c := line[i]
		if quote != 0 {
			switch c {
			case quote:
				quote = 0
			case '#':
				sawHash = true
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			quote = c
		case '#':
			return line[:i], false
		}
	}
	return line, quote != 0 && sawHash
}

// findColon returns the index of the first unquoted ':' , or -1.
func findColon(s string) int {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			quote = c
		case ':':
			return i
		}
	}
	return -1
}

// normalize rewrites a raw tree into canonical shape: slash-path keys expand
// into real nesting, then duplicate non-repeatable keys at each level collapse
// into the first occurrence (children append in document order). Containers
// merging is the feature - long DDLs can restate `tables:` without indent
// archaeology; a scalar redefined to a different value is still almost surely
// a mistake, so the first value wins with a warning.
func normalize(n *Node, warns *[]string) {
	for i, child := range n.Children {
		if strings.Contains(child.Key, "/") {
			n.Children[i] = expandPath(child)
		}
	}
	byKey := map[string]*Node{}
	out := n.Children[:0]
	for _, child := range n.Children {
		if child.Key == "" || repeatable[child.Key] {
			out = append(out, child)
			continue
		}
		first, dup := byKey[child.Key]
		if !dup {
			byKey[child.Key] = child
			out = append(out, child)
			continue
		}
		if child.Value != "" {
			if first.Value == "" {
				first.Value = child.Value
			} else if first.Value != child.Value {
				*warns = append(*warns, fmt.Sprintf(
					"line %d: duplicate key %q (line %d already set it); the first value wins", child.Line, child.Key, first.Line))
			}
		}
		first.Children = append(first.Children, child.Children...)
	}
	n.Children = out
	for _, child := range n.Children {
		normalize(child, warns)
	}
}

// expandPath turns a "database/relationships:" shorthand node into the nested
// chain it abbreviates; the original node becomes the innermost link.
func expandPath(leaf *Node) *Node {
	segments := strings.Split(leaf.Key, "/")
	leaf.Key = segments[len(segments)-1]
	node := leaf
	for i := len(segments) - 2; i >= 0; i-- {
		node = &Node{Key: segments[i], Line: leaf.Line, Children: []*Node{node}}
	}
	return node
}

// isPathKey accepts a bareword key or a slash-joined path of them
// ("database/relationships") - the hierarchy shorthand.
func isPathKey(s string) bool {
	for _, seg := range strings.Split(s, "/") {
		if !isIdent(seg) {
			return false
		}
	}
	return true
}

// isIdent reports whether s is a bareword DDL key.
func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_', c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z':
		case i > 0 && c >= '0' && c <= '9':
		default:
			return false
		}
	}
	return true
}
