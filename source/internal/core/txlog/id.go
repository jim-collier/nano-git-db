// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// Row and transaction ids on the wire. A 16-byte UUID renders as 22 characters
// of unpadded base64url instead of 32 of hex, and every log line carries two of
// them, so a line sheds ~20 bytes. That is worth little in git (both forms hold
// the same 128 bits and zlib collapses the difference) but real in the working
// tree and on every replay, which reads the whole log.
//
// Logs written before the switch hold 32-char hex. Both forms are read; the
// fixed 16-byte width tells them apart by length alone, so no sniffing.
//
// NEVER re-encode an existing id. Encrypted values derive their per-value subkey
// from the tx_id and row_id STRINGS (enc.Ctx), and authenticate the row id as
// AAD, so rewriting an id from one form to the other silently strands the
// ciphertext under it. Append-only makes this a non-issue in normal operation -
// GC rotation copies lines verbatim - but a "tidy up the old ids" pass would
// destroy data. Entries keep whatever form they were written with, forever.
package txlog

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

const (
	idRawLen  = 16           // UUID
	idHexLen  = idRawLen * 2 // legacy form, 32
	idTextLen = 22           // unpadded base64url of 16 bytes
)

var idEncoding = base64.RawURLEncoding

// EncodeID renders a raw id in the current wire form.
func EncodeID(raw []byte) string { return idEncoding.EncodeToString(raw) }

// DecodeID reads either wire form. Anything that is not one of the two exact
// widths is rejected rather than guessed at: replay treats a bad row id as a
// skippable entry, and silently decoding junk into 16 plausible bytes would
// write a ghost row instead of warning.
func DecodeID(id string) ([]byte, error) {
	var (
		raw []byte
		err error
	)
	switch len(id) {
	case idTextLen:
		raw, err = idEncoding.DecodeString(id)
	case idHexLen:
		raw, err = hex.DecodeString(id)
	default:
		return nil, fmt.Errorf("bad id %q: want %d or %d characters, got %d",
			id, idTextLen, idHexLen, len(id))
	}
	if err != nil {
		return nil, fmt.Errorf("bad id %q: %w", id, err)
	}
	return raw, nil
}
