// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package txlog

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

// testID renders a short fixture label as a valid id, so tests keep using
// readable stand-ins like "01" against the format's fixed width.
func testID(label string) string {
	var raw [idRawLen]byte
	copy(raw[:], label)
	return EncodeID(raw[:])
}

func TestIDRoundTrip(t *testing.T) {
	raw := []byte("0123456789abcdef")
	text := EncodeID(raw)
	if len(text) != idTextLen {
		t.Fatalf("encoded to %d characters, want %d", len(text), idTextLen)
	}
	got, err := DecodeID(text)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("round trip = %x, want %x", got, raw)
	}
}

// Logs written before the switch carry 32-char hex and must keep replaying.
func TestDecodeIDAcceptsLegacyHex(t *testing.T) {
	raw := []byte("0123456789abcdef")
	got, err := DecodeID(hex.EncodeToString(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("legacy decode = %x, want %x", got, raw)
	}
}

// A malformed id is rejected rather than guessed at: replay skips the entry
// with a warning, where a lucky decode would write a ghost row.
func TestDecodeIDRejectsMalformed(t *testing.T) {
	bad := []string{
		"", "01", "zz",
		strings.Repeat("a", idTextLen-1),
		strings.Repeat("a", idTextLen+1),
		strings.Repeat("+", idTextLen), // not in the base64url alphabet
		strings.Repeat("z", idHexLen),  // right width, not hex
	}
	for _, id := range bad {
		if _, err := DecodeID(id); err == nil {
			t.Fatalf("DecodeID(%q) accepted a malformed id", id)
		}
	}
}

// The log is CSV, so an id must never contain a character that needs quoting.
func TestEncodeIDNeedsNoCSVQuoting(t *testing.T) {
	for b := 0; b < 256; b++ {
		var raw [idRawLen]byte
		for i := range raw {
			raw[i] = byte(b)
		}
		if id := EncodeID(raw[:]); strings.ContainsAny(id, ",\"\r\n") {
			t.Fatalf("id %q needs CSV quoting", id)
		}
	}
}
