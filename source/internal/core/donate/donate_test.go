// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package donate

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCanonicalBytesFormat(t *testing.T) {
	got := CanonicalBytes()
	if len(got) == 0 || got[len(got)-1] != '\n' {
		t.Fatalf("canonical bytes must be non-empty and end with a newline")
	}
	lines := strings.Split(strings.TrimRight(string(got), "\n"), "\n")
	if len(lines) != len(Targets) {
		t.Fatalf("got %d lines, want one per target (%d)", len(lines), len(Targets))
	}
	for i, line := range lines {
		want := Targets[i].Label + "\t" + Targets[i].Kind + "\t" + Targets[i].Value
		if line != want {
			t.Errorf("line %d = %q, want %q", i, line, want)
		}
	}
}

func TestConfigured(t *testing.T) {
	if (Target{Value: "PLACEHOLDER_BTC_ADDRESS"}).Configured() {
		t.Error("a placeholder value must not count as configured")
	}
	if !(Target{Value: "bc1qexample"}).Configured() {
		t.Error("a real value must count as configured")
	}
	// The shipped table is all placeholders until the maintainer fills it in.
	if HasConfigured() {
		t.Skip("addresses have been filled in; the all-placeholder invariant no longer holds")
	}
}

// TestDonationTableSigned is the tamper gate: once real addresses are set, the
// table must carry a valid signature made with the maintainer's key. It skips
// harmlessly while everything is a placeholder, when ssh-keygen is unavailable, or
// when the out-of-repo trust anchor is not on this machine (e.g. a fresh clone).
func TestDonationTableSigned(t *testing.T) {
	if !HasConfigured() {
		t.Skip("donation addresses are still placeholders - nothing to protect yet")
	}
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen not available")
	}
	_, thisFile, _, _ := runtime.Caller(0)
	pkgDir := filepath.Dir(thisFile)
	anchor := os.Getenv("NGDB_DONATION_ALLOWED_SIGNERS")
	if anchor == "" {
		// pkgDir up to the project root (the dir holding github_floss/ + private/).
		anchor = filepath.Join(pkgDir, "..", "..", "..", "..", "..", "private", "donation_keys", "allowed_signers")
	}
	if _, err := os.Stat(anchor); err != nil {
		t.Skipf("donation trust anchor not present (%s) - cannot verify here", anchor)
	}
	sig := filepath.Join(pkgDir, "donation.sig")
	if _, err := os.Stat(sig); err != nil {
		t.Fatal("real donation addresses are set but donation.sig is missing - run cicd/sign-donations.bash")
	}

	cmd := exec.Command("ssh-keygen", "-Y", "verify",
		"-f", anchor, "-I", SignatureNamespace, "-n", SignatureNamespace, "-s", sig)
	cmd.Stdin = bytes.NewReader(CanonicalBytes())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("donation signature failed to verify: %s - the table was changed without re-signing, or the signature is invalid", strings.TrimSpace(string(out)))
	}
}
