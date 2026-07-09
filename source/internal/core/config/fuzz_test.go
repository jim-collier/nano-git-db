// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// parseHash reads an attacker-controllable stored credential string; it must
// return a zero iteration count on any malformed input and never panic.
func FuzzParseHash(f *testing.F) {
	for _, s := range []string{
		"",
		"pbkdf2-sha256$600000$c2FsdA$aGFzaA",
		"pbkdf2-sha256$$$",
		"pbkdf2-sha256$-1$x$y",
		"scrypt$1$a$b",
		"$$$$$$",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, stored string) {
		iters, _, _ := parseHash(stored)
		if iters < 0 {
			t.Fatalf("negative iteration count from %q", stored)
		}
	})
}

// Load runs the fuzzed bytes through the TOML decoder (library code we depend
// on): a malformed registry record must surface as an error, never a panic.
func FuzzLoadTOML(f *testing.F) {
	for _, s := range []string{
		"",
		"name = \"x\"\nddl_path = \"a.ddl\"\n",
		"name = \nbroken",
		"[[[",
		"encryption = 3\n",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, recordFile), data, 0o600); err != nil {
			t.Skip()
		}
		_, _ = Load(dir)
	})
}
