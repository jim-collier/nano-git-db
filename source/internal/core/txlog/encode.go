// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package txlog

import "strings"

// The on-disk format replaces "unsafe" characters with named markers wrapped in
// U+1F5E6/U+1F5E7 delimiters, e.g. a newline becomes 🗦NEWLINE🗧. This keeps
// every CSV record on one physical line - encoding/csv would otherwise quote a
// newline-bearing value into a multi-line record, and git's line-based
// merge=union could interleave two clients' appends inside it, corrupting the
// log. With commas and quotes encoded too, the writer never needs to quote at
// all, so one entry is always exactly one line.
//
// The same scheme gives NULL a representation: a new_value of exactly 🗦NULL🗧
// means SQL NULL (Entry.IsNull). A literal 🗦 or 🗧 in content is itself
// encoded (🗦LDELIM🗧/🗦RDELIM🗧), so user text can never collide with a marker.
//
// Both directions are single-pass: markers contain the delimiter characters,
// so sequential ReplaceAll calls would re-match their own output.
//
// Encoding happens only at the file boundary (record/entryFrom); in-memory
// entries and the SQLite view always hold the real characters.

const (
	ldelim = '\U0001F5E6' // 🗦
	rdelim = '\U0001F5E7' // 🗧

	nullMarker = "\U0001F5E6NULL\U0001F5E7"
	// encMarker prefixes an encrypted field value; what follows is the
	// crypt-layer base64 token, stored raw (its alphabet has no special chars).
	// A literal user value equal to this marker is escaped by encodeSym (the
	// delimiters become LDELIM/RDELIM), so cleartext can never spoof it - the
	// same collision-safety the NULL sentinel relies on.
	encMarker = "\U0001F5E6ENC\U0001F5E7"
)

var symName = map[rune]string{
	ldelim: "LDELIM",
	rdelim: "RDELIM",
	'\n':   "NEWLINE",
	'\r':   "RETURN",
	'\'':   "SINGLE_QUOTE",
	'"':    "DOUBLE_QUOTE",
	'`':    "BACKTICK",
	'\\':   "BACK_SLASH",
	'/':    "FORWARD_SLASH",
	',':    "COMMA",
}

var symChar = func() map[string]rune {
	m := make(map[string]rune, len(symName))
	for r, n := range symName {
		m[n] = r
	}
	return m
}()

// encodeSym renders a field value for the log file. Control characters other
// than newline/return/tab carry no data worth logging and are scrubbed.
func encodeSym(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { return isIllegal(r) || symName[r] != "" }) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + len(s)/2)
	for _, r := range s {
		switch {
		case isIllegal(r):
		case symName[r] != "":
			b.WriteRune(ldelim)
			b.WriteString(symName[r])
			b.WriteRune(rdelim)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// decodeSym restores a field value read from the log file. In encoded text a
// literal 🗦 only ever starts a marker, so scanning is unambiguous; an unknown
// marker name (or a pre-encoding legacy value) passes through unchanged.
func decodeSym(s string) string {
	if !strings.ContainsRune(s, ldelim) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		if rs[i] != ldelim {
			b.WriteRune(rs[i])
			continue
		}
		end := -1
		for j := i + 1; j < len(rs); j++ {
			if rs[j] == rdelim {
				end = j
				break
			}
		}
		if end < 0 {
			b.WriteRune(rs[i])
			continue
		}
		if r, ok := symChar[string(rs[i+1:end])]; ok {
			b.WriteRune(r)
			i = end
			continue
		}
		b.WriteRune(rs[i])
	}
	return b.String()
}

func isIllegal(r rune) bool {
	if r == '\n' || r == '\r' || r == '\t' {
		return false
	}
	return r < 0x20 || r == 0x7F
}
