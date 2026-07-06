// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package config

import "testing"

func TestWebCredsHashVerify(t *testing.T) {
	c := &WebCreds{}
	if err := c.Set("alice", "correct horse"); err != nil {
		t.Fatal(err)
	}
	if !c.Verify("alice", "correct horse") {
		t.Fatal("right password did not verify")
	}
	if c.Verify("alice", "wrong") {
		t.Fatal("wrong password verified")
	}
	if c.Verify("nobody", "correct horse") {
		t.Fatal("unknown user verified")
	}
	// The stored value is a hash, not the password.
	if c.Users["alice"] == "correct horse" {
		t.Fatal("password stored in the clear")
	}
}

func TestWebCredsSaveLoad(t *testing.T) {
	SetBaseOverride(t.TempDir())
	t.Cleanup(func() { SetBaseOverride("") })

	c := &WebCreds{}
	if err := c.Set("bob", "hunter2"); err != nil {
		t.Fatal(err)
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	got := LoadWebCreds()
	if !got.Verify("bob", "hunter2") {
		t.Fatal("reloaded creds do not verify")
	}
	if got.Verify("bob", "nope") {
		t.Fatal("reloaded creds verified a wrong password")
	}
}

func TestWebModeProxied(t *testing.T) {
	if (&Settings{WebMode: "proxied"}).WebModeProxied() != true {
		t.Fatal(`web_mode "proxied" should be proxied`)
	}
	for _, m := range []string{"", "local", "Proxied", "typo"} {
		if (&Settings{WebMode: m}).WebModeProxied() {
			t.Fatalf("web_mode %q should fall back to local", m)
		}
	}
}
