# Changelog

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## v1.0.0-alpha.1

First public alpha. The pre-1.0 feature set is complete.

### Added

- One small static binary, no external dependencies. Runs on Linux, Windows, and macOS.
- Three interfaces over one shared core: a command-line interface, a terminal UI, and a self-hosted local web UI.
- Text-based DDL for the schema. No SQL needed to define or manage a database.
- Append-only transaction log as the source of truth, synced and auto-merged with git. The local SQLite database is a rebuildable view of it.
- Schema changes at any time. Old and new records stay compatible both ways, so there is no migration step.
- Rename tables and fields without losing or rewriting history.
- Views, including hierarchical tree grids over a self-referential parent link.
- Saved queries kept next to the schema and picked from a menu.
- Per-table extras: a comment list per record, an automatic audit trail, and file or link attachments.
- Users and groups, with permissions down to the table, field, and row.
- Automatic cleanup of old deleted records from the log.
- Register databases once and pick from a list at startup.
- Selectable light and dark themes in the terminal UI.
- Web sign-in for proxied mode, so the web UI can be exposed safely.

### Notes

- A separately licensed enterprise edition adds at-rest encryption of the synced log and Lua scripting, in the same binary.
