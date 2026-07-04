// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// Package app is the shared entrypoint behind every build's main: it parses the
// global flag prefixes and dispatches to the right front-end. main stays a thin
// wrapper so an external build (e.g. a superset that links extra features) can
// reuse the exact same dispatch by calling app.Run instead of duplicating it.
package app

import (
	"fmt"
	"strings"

	"github.com/jim-collier/nano-git-db/enc"
	"github.com/jim-collier/nano-git-db/internal/cli"
	"github.com/jim-collier/nano-git-db/internal/core/config"
	"github.com/jim-collier/nano-git-db/internal/tui"
	"github.com/jim-collier/nano-git-db/internal/web"
)

// Version is stamped at build time via -ldflags -X (see cicd/build.bash); it
// stays "dev" for a plain `go build`.
var Version = "dev"

// Run dispatches the given args (os.Args[1:]) to a front-end and returns its
// error, if any.
func Run(args []string) error {
	// --version: print the build version and stop. A concise line the CI/CD
	// pipeline can run to confirm the binary executes, without opening the TUI.
	if len(args) > 0 && (args[0] == "--version" || args[0] == "-v") {
		fmt.Println("nanogitdb " + Version)
		return nil
	}
	// --config[=path]: a global prefix that redirects the database registry;
	// strip it, apply the override, then dispatch the remaining args normally.
	if len(args) > 0 {
		if val, ok := splitFlag(args[0], "--config"); ok {
			rest := args[1:]
			if val == "" { // "--config <path>" form
				if len(rest) == 0 {
					return fmt.Errorf("--config needs a directory path")
				}
				val, rest = rest[0], rest[1:]
			}
			config.SetBaseOverride(val)
			return Run(rest)
		}
	}
	// --encrypt[=on|off|auto]: a global prefix setting the local encryption
	// preference for this run (bare --encrypt means on). Encryption is an
	// enterprise feature, so the open-source build (no provider registered)
	// rejects the flag outright rather than accepting a preference it cannot act
	// on.
	if len(args) > 0 {
		if val, ok := splitFlag(args[0], "--encrypt"); ok {
			if !enc.Available() {
				return fmt.Errorf("--encrypt requires the enterprise build; the open-source build cannot encrypt")
			}
			if val == "" {
				val = "on"
			}
			pref, valid := config.NormEncrypt(val)
			if !valid {
				return fmt.Errorf("--encrypt: %q is not on|off|auto", val)
			}
			config.SetEncryptPref(pref)
			return Run(args[1:])
		}
	}

	mode := ""
	if len(args) > 0 {
		mode = args[0]
	}
	// --init[=path]: register a database from a $PWD .ddl (its own mode, not a
	// data verb).
	if val, ok := splitFlag(mode, "--init"); ok {
		rest := args[1:]
		if val != "" {
			rest = append([]string{val}, rest...)
		}
		return cli.Init(rest)
	}
	switch mode {
	case "--tui":
		return tui.Run(args[1:])
	case "--serve":
		return web.Run(args[1:])
	case "--script":
		return cli.Script(args[1:])
	case "":
		// No args at all: the interactive database picker (or a $PWD DDL). The
		// picker is a TUI screen, so this routes there rather than to the CLI.
		return tui.Run(nil)
	default:
		return cli.Run(args)
	}
}

// splitFlag matches a flag given either as "name" or "name=value".
func splitFlag(arg, name string) (value string, ok bool) {
	if arg == name {
		return "", true
	}
	if strings.HasPrefix(arg, name+"=") {
		return arg[len(name)+1:], true
	}
	return "", false
}
