// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package cli

import (
	"fmt"
	"os"

	"github.com/jim-collier/nano-git-db/internal/core/config"
	"github.com/jim-collier/nano-git-db/internal/core/schema"
	"github.com/jim-collier/nano-git-db/script"
)

// Script is the --script front-end: standard client bring-up (build, bootstrap,
// replay, seed), then run the script against the CRUD API through the scripting
// host. The host is an enterprise feature, so the open-source build (no host
// registered) rejects the flag before doing any work.
func Script(args []string) error {
	if len(args) < 4 {
		return fmt.Errorf("usage: nanogitdb --script <script.lua> <ddl> <sqlite> <logdir>")
	}
	if !script.Available() {
		return fmt.Errorf("--script requires the enterprise build; the open-source build has no scripting host")
	}
	keyFile, pref := config.ResolveEncryption(args[1], nil)
	c, err := schema.OpenClientWith(schema.OpenOpts{
		DDLPath: args[1], DBPath: args[2], LogDir: args[3], KeyFile: keyFile, EncryptPref: pref,
	})
	if err != nil {
		return err
	}
	defer c.Close()
	bs, err := schema.Builtins()
	if err != nil {
		return err
	}
	tw, err := script.Attach(c.API, args[1], args[3], c.Schema, bs)
	if err != nil {
		return err
	}
	for _, w := range append(c.Warnings, tw...) {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	return script.RunFile(c.API, args[0])
}
