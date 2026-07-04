// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

const recordFile = "config.toml"

// DBConfig is one registered database's record. Paths are stored absolute so a
// record stays valid regardless of the working directory it is opened from.
// Encryption and KeyFile are parsed now for a stable file format; the crypto
// behavior itself lands with the encryption feature.
type DBConfig struct {
	Name       string `toml:"name"`
	DDLPath    string `toml:"ddl_path"`    // git-synced schema file
	LogDir     string `toml:"log_dir"`     // git-synced tx-log + attachments dir
	SQLitePath string `toml:"sqlite_path"` // local unsynced view; rebuilt from the log
	KeyFile    string `toml:"key_file"`    // encryption key; local, defaults beside the record
	Encryption string `toml:"encryption"`  // local field-encryption pref: on|off|auto (default auto)
	LastOpened string `toml:"last_opened"` // RFC3339, refreshed on successful open

	dir string // the record's directory; set on Load/Create, not serialized
}

// Dir is the directory the record lives in (also the default home for its
// local sqlite/key files).
func (c *DBConfig) Dir() string { return c.dir }

// applyDefaults fills the local-file paths that were left blank, keying them to
// the record directory and name so a minimal hand-written config.toml (just a
// name, ddl_path and log_dir) still opens.
func (c *DBConfig) applyDefaults() {
	if c.SQLitePath == "" {
		c.SQLitePath = filepath.Join(c.dir, c.Name+".sqlite")
	}
	if c.KeyFile == "" {
		c.KeyFile = filepath.Join(c.dir, c.Name+".key")
	}
	if c.Encryption == "" {
		c.Encryption = "auto"
	}
}

// Load reads <dir>/config.toml and fills defaults. A missing or malformed file
// is an error the caller reports (discovery marks such a record unopenable).
func Load(dir string) (*DBConfig, error) {
	var cfg DBConfig
	if _, err := toml.DecodeFile(filepath.Join(dir, recordFile), &cfg); err != nil {
		return nil, err
	}
	cfg.dir = dir
	if cfg.Name == "" {
		cfg.Name = filepath.Base(dir) // a nameless record falls back to its dir
	}
	cfg.applyDefaults()
	return &cfg, nil
}

// Save writes the record back to <dir>/config.toml, creating the directory.
func (c *DBConfig) Save() error {
	if c.dir == "" {
		return fmt.Errorf("config: record has no directory")
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(c.dir, recordFile))
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(c)
}

// Touch stamps LastOpened and persists it; a failed save is non-fatal (the
// database still opened), so callers may ignore the error.
func (c *DBConfig) Touch() error {
	c.LastOpened = time.Now().UTC().Format(time.RFC3339)
	return c.Save()
}

// Create registers a new database under the user base and returns its record.
// ddlPath and logDir are resolved to absolute paths; the local sqlite/key paths
// default beside the record. It refuses to clobber an existing record.
func Create(name, ddlPath, logDir string) (*DBConfig, error) {
	if name == "" {
		return nil, fmt.Errorf("config: database name is required")
	}
	base, err := UserBase()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(base, name)
	if _, err := os.Stat(filepath.Join(dir, recordFile)); err == nil {
		return nil, fmt.Errorf("config: database %q already registered at %s", name, dir)
	}
	cfg := &DBConfig{
		Name:    name,
		DDLPath: absPath(ddlPath),
		LogDir:  absPath(logDir),
		dir:     dir,
	}
	cfg.applyDefaults()
	if err := cfg.Save(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// absPath makes a path absolute without failing: a bad cwd just leaves it as
// given, which discovery will later flag as unopenable.
func absPath(path string) string {
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}
