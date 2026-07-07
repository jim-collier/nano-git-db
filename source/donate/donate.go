// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// Package donate holds the project's own donation targets, shown by --donate and
// the TUI/web Donate screens.
//
// SECURITY: these are the maintainer's real addresses. A bad edit here - or a
// merged pull request - could silently redirect donations to someone else. They
// are guarded, weakest to strongest:
//   - every change is a visible diff with a clear history, so keep this reviewed;
//   - the values ship as PLACEHOLDER_* that render as "not yet configured" and
//     cannot be copied, so a release can never ask for donations to nothing;
//   - the table is signed with the maintainer's key (held outside the repo) and a
//     go-test gate refuses a table that no longer matches its signature, so a
//     swapped address that was not re-signed fails the build. CanonicalBytes is
//     the exact content that gets signed. See cicd/donation-signing.md.
//
// The addresses stay visible - they are meant to be seen; the signature protects
// that they are the maintainer's and not a substitute.
package donate

import "strings"

// placeholderPrefix marks an address/URL that has not been filled in yet.
const placeholderPrefix = "PLACEHOLDER_"

// SignatureNamespace is the ssh-keygen signing namespace; the sign helper and the
// verify gate must use the same string.
const SignatureNamespace = "donation"

// Intro is the one-line appeal shown above the targets in every front-end.
const Intro = "If you find nano-git-db useful, please consider donating."

// Enabled is the master switch for the whole Donate feature. The open-source build
// leaves it on; the enterprise (commercial) build turns it off before dispatch -
// you do not ask a paying customer to donate. It gates --donate and the TUI/web
// entries, so a disabled build shows no trace of the feature.
var Enabled = true

// Target is one donation channel: a human label, a kind ("crypto" for an address
// or "link" for a URL), and the value.
type Target struct {
	Label string
	Kind  string
	Value string
}

// Configured is false while the value is still a placeholder.
func (t Target) Configured() bool { return !strings.HasPrefix(t.Value, placeholderPrefix) }

// Targets is the fixed-order donation table. Replace every PLACEHOLDER_* with the
// real address/URL, then re-sign (cicd/sign-donations.bash). Order is significant:
// it is part of what the signature covers.
var Targets = []Target{
	{"Bitcoin (BTC)", "crypto", "PLACEHOLDER_BTC_ADDRESS"},
	{"Monero (XMR)", "crypto", "PLACEHOLDER_XMR_ADDRESS"},
	{"Ethereum (ETH)", "crypto", "PLACEHOLDER_ETH_ADDRESS"},
	{"USD Coin (USDC)", "crypto", "PLACEHOLDER_USDC_ADDRESS"},
	{"GitHub Sponsors", "link", "PLACEHOLDER_GITHUB_SPONSORS_URL"},
	{"Liberapay", "link", "PLACEHOLDER_LIBERAPAY_URL"},
	{"Ko-fi", "link", "PLACEHOLDER_KOFI_URL"},
	{"Patreon", "link", "PLACEHOLDER_PATREON_URL"},
}

// HasConfigured reports whether at least one real address/URL is set - i.e. there
// is anything worth protecting or showing yet.
func HasConfigured() bool {
	for _, t := range Targets {
		if t.Configured() {
			return true
		}
	}
	return false
}

// CanonicalBytes is the exact content signed by cicd/sign-donations.bash and
// re-checked by the verify gate - the single source of truth so signer and
// verifier can never disagree. One "label\tkind\tvalue" line per target in order,
// so reordering or editing the table changes these bytes and breaks the signature.
func CanonicalBytes() []byte {
	var b strings.Builder
	for _, t := range Targets {
		b.WriteString(t.Label)
		b.WriteByte('\t')
		b.WriteString(t.Kind)
		b.WriteByte('\t')
		b.WriteString(t.Value)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}
