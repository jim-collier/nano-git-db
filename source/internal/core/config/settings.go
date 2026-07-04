// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// settingsFile holds user-global (not per-database) preferences. It sits at the
// registry root beside the per-db <name>/ dirs; discovery only scans dirs, so a
// plain file here is never mistaken for a database.
const settingsFile = "settings.toml"

// Settings is the user-global preferences record. Kept deliberately small - only
// things that are the same across every database this user opens.
type Settings struct {
	Theme string `toml:"theme"` // TUI theme name; empty = the built-in default
}

func settingsPath() (string, error) {
	base, err := UserBase()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, settingsFile), nil
}

// LoadSettings reads the global settings, returning zero-value defaults when the
// file is missing or unreadable (a first run, or a fresh --config dir). It never
// errors, so callers can use the result directly.
func LoadSettings() *Settings {
	s := &Settings{}
	if p, err := settingsPath(); err == nil {
		_, _ = toml.DecodeFile(p, s)
	}
	return s
}

// Save writes the global settings, creating the registry root if needed.
func (s *Settings) Save() error {
	p, err := settingsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(s)
}
