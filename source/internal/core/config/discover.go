// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jim-collier/nano-git-db/internal/core/ddl"
)

// Listed is one discovered registry entry. An unopenable record still appears -
// with its name (or directory) and the reason - so the picker can show it
// greyed with an explanation rather than hiding a broken database.
type Listed struct {
	Name   string
	Dir    string
	Config *DBConfig // nil when config.toml itself could not be read
	System bool      // from a read-only system base
	Err    error     // nil means openable; else why not
}

// List discovers every registered database under the user base and the system
// bases (user first). Each record is lightly validated: its DDL must exist and
// parse. The sqlite view and attachments are not required - they rebuild from
// the log on open - so their absence is never an error.
func List() []Listed {
	var out []Listed
	if base, err := UserBase(); err == nil {
		out = append(out, scanBase(base, false)...)
	}
	for _, base := range SystemBases() {
		out = append(out, scanBase(base, true)...)
	}
	// Stable, name-first order for a predictable picker.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// scanBase lists the immediate sub-directories of one base that hold a
// config.toml. A missing base is not an error - it just yields nothing.
func scanBase(base string, system bool) []Listed {
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var out []Listed
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(base, entry.Name())
		if _, err := os.Stat(filepath.Join(dir, recordFile)); err != nil {
			continue // not a registered database
		}
		out = append(out, validate(dir, entry.Name(), system))
	}
	return out
}

// validate loads and sanity-checks one record.
func validate(dir, fallbackName string, system bool) Listed {
	listed := Listed{Name: fallbackName, Dir: dir, System: system}
	cfg, err := Load(dir)
	if err != nil {
		listed.Err = fmt.Errorf("unreadable config: %w", err)
		return listed
	}
	listed.Config = cfg
	listed.Name = cfg.Name
	if cfg.DDLPath == "" {
		listed.Err = fmt.Errorf("no ddl_path set")
		return listed
	}
	if _, err := os.Stat(cfg.DDLPath); err != nil {
		listed.Err = fmt.Errorf("DDL not found: %s", cfg.DDLPath)
		return listed
	}
	if _, err := ddl.ParseFile(cfg.DDLPath); err != nil {
		listed.Err = fmt.Errorf("DDL invalid: %w", err)
		return listed
	}
	return listed
}

// FindByDDL returns the registered record whose ddl_path matches ddlPath (by
// absolute path), or nil. This lets the stateless CLI verbs, handed only a DDL,
// find that database's key file and encryption preference in the registry
// rather than guessing a key beside the DDL (which would sit in the git repo).
func FindByDDL(ddlPath string) *DBConfig {
	abs := absPath(ddlPath)
	for _, listed := range List() {
		if listed.Config != nil && absPath(listed.Config.DDLPath) == abs {
			return listed.Config
		}
	}
	return nil
}

// PWDDdl reports a DDL sitting in the current directory, so a database in $PWD
// opens without being named. It uses a lone *.ddl file; zero or several are
// ambiguous and yield ("", false), leaving the caller to fall back to the
// registry picker or to require explicit paths.
func PWDDdl() (string, bool) {
	wd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	matches, err := filepath.Glob(filepath.Join(wd, "*.ddl"))
	if err != nil || len(matches) != 1 {
		return "", false
	}
	return matches[0], true
}

// LocalSQLite is the view path for an unregistered open: beside the DDL, named
// after it. The view is rebuilt from the log, so co-locating it is harmless.
func LocalSQLite(ddlPath string) string {
	ext := filepath.Ext(ddlPath)
	return ddlPath[:len(ddlPath)-len(ext)] + ".sqlite"
}

// LocalKey is the encryption-key path for an unregistered open: beside the DDL,
// named after it. Registered databases use DBConfig.KeyFile instead.
func LocalKey(ddlPath string) string {
	ext := filepath.Ext(ddlPath)
	return ddlPath[:len(ddlPath)-len(ext)] + ".key"
}

// PWDTriple resolves a database in the current directory to the
// (ddl, sqlite, logDir) triple the front-ends open. ok is false when $PWD has
// no unambiguous DDL, so the caller can fall back to the picker or an error.
func PWDTriple() (ddlPath, sqlitePath, logDir string, ok bool) {
	ddlPath, ok = PWDDdl()
	if !ok {
		return "", "", "", false
	}
	return ddlPath, LocalSQLite(ddlPath), filepath.Dir(ddlPath), true
}
