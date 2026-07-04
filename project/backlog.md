<!-- markdownlint-disable MD007 -- Unordered list indentation -->
<!-- markdownlint-disable MD010 -- No hard tabs -->
<!-- markdownlint-disable MD033 -- No inline html -->
<!-- markdownlint-disable MD055 -- Table pipe style [Expected: leading_and_trailing; Actual: leading_only; Missing trailing pipe] -->
<!-- markdownlint-disable MD041 -- First line in a file should be a top-level heading -->

<!-- TOC ignore:true -->
# Project backlog

This is a product backlog just for pre-v1.0.0 release. After that, bugs, features, and enhancements will be managed in Github Issues, and/or [todo.md](../todo.md)

<!-- TOC ignore:true -->
## Table of contents
<!-- TOC -->

- [Conventions](#conventions)
- [Backlog](#backlog)
	- [Bugs](#bugs)
	- [New features and enhancements](#new-features-and-enhancements)
	- [Done](#done)
		- [First steps](#first-steps)
		- [Done - Bugs](#done---bugs)
		- [Done - New features and enhancements](#done---new-features-and-enhancements)
	- [Deferred](#deferred)
	- [Canceled](#canceled)
- [Application name ideas](#application-name-ideas)

<!-- /TOC -->

## Conventions

In each section, items are listed approximately from newest to oldest.

| Icon | Status
| :--: | :--
| 🔘   | Not started
| ◐   | Started, and/or partially complete
| ✋   | Defer
| ✔️   | Complete
| 🚫   | Canceled

## Backlog

### Bugs

### New features and enhancements

- 🔘 Web UI: basic login (HIGH PRIORITY). The web server has no authentication today. Localhost binding is the only gate, and every request acts as one default user, so it cannot be safely exposed.
	- On a local machine, identify the user with no password. Use the git repository account name, or the operating system user when there is no repository.
	- Behind a reverse proxy, require a username and password. The stronger methods belong to the enterprise edition.
	- The signed-in user must reach the data layer, so the existing user and group permissions apply to the web view.
	- Settle first how to tell a local machine apart from a proxied one, since a proxy also connects from localhost. Prefer an explicit setting over guessing.

- 🔘 Order of fields in txlog should affect backward of forward schema compatibility.

- ✔️ Startup:
	- ✔️ If the program is run from an active directory that has a DDL, use that one.
	- ✔️ If the program is run without arguments and no DDL is in active directory, search user-level and system-level config directories for defined databases that point to valid DDLs.
		- If there are multiple, allow the user to choose them from an initial screen (or "create new").
		- If a database specified in the config directory cannot be loaded (e.g. can't find the DDL or it's irreversibly corrupt), include the database name in the list, but also with some kind of error indication, and brief text of why it can't be opened.
		- Rebuild anything that is missing and can be rebuilt or defaulted. (e.g. SQLite file.)
		- The list of databases should always include "Create new database", and "Open existing ...". Both will futher prompt for a location for the txlog and associated files. (User-specific, unsynced files go where user config file says. Autogenerate if non-existent.)
	- ✔️ If no DDL and nothing in user config, the only options are "Create new database", and "Open existing ..."
	- Done: new `internal/core/config` package holds a per-db TOML registry. Records live under the OS user-config dir, one `config.toml` each, carrying the DDL path, log dir, sqlite path, key file, and encryption preference. `config.List` discovers them and checks the DDL parses; a missing sqlite is fine since it rebuilds from the log on open. Startup routing: a lone `*.ddl` in the current dir opens directly, otherwise the TUI shows a picker of registered databases plus Create-new and Open-existing. Unopenable databases still list, flagged with the reason.
	- Note: Create-new points at an existing DDL. Authoring a DDL from scratch is a separate concern, as are the flags in the next item.

- ✔️ Add to README.md:
	- ✔️ A section on the bare-minimum, super-simple DDL syntax
		- ✔️ Plus an example to make the simplest "to-do" hierarchical database and nested list, with the most minimal features. (E.g. task, status, open and close date, parent task, and related comment list.)
	- ✔️ A section with the most basic CLI options to get started.
	- ✔️ A link to the full DDL and options syntax in `syntax.md`
		- ✔️ Create that file.
	- Done: README gained a "Quick start" with a minimal schema and a full hierarchical to-do example, plus a basic CLI walkthrough and a pointer to the full syntax reference. Both DDL blocks were parse-verified, and the to-do one runs end to end.
	- Done: new `github/syntax.md` is the complete reference covering DDL grammar, types, system columns, uniques, features, access, renames, views, tunables, the sidecar files, all CLI verbs, run modes, and env vars. It documents only implemented behavior; example.ddl's not-yet-wired keys are flagged as forward-looking.

- ✔️ Partial flag list:
	- ✔️ `--init[=]["git repo root to auto-calculate subfolders for (e.g. 'ngdb/dbname/'), or override default with specific 'git-repo-root-path/subfolder(s)/'"]`  ## If no arg specified, and already in a git root or DDL is in $PWD, use it. Otherwise error.
	- ✔️ `--config[=]"Alternate user config file folder, name, and/or full filespec"`  ## Default = "~/.config/ngdb/dbname/" (or Windows analogue).
	- ✔️ `--[en]crypt[ion][=]on|off|auto[matic]|yes|no|true|false|y|n|t|f|enable[d]|disable[d]`  ## If on, may be overridden by DDL values of 'never'. If off, may be overridden by DDL values of 'always'. Default=auto.
	- Done (`--init`, `--config`): `--init [path]` registers the lone current-dir DDL as a database and materializes its view. It picks the tx-log dir automatically: inside a git repo it goes under `<repo-root>/ngdb/<name>`, an explicit path is taken verbatim, and outside a repo it lands in the current dir. `--config <dir>` is a global prefix that repoints the registry. Both accept the `=` form.
	- Done (`--encrypt`): landed with the encryption item below. `--encrypt[=on|off|auto]` is a global prefix (bare means on). Paired with `--init` it mints the per-db key and persists the preference.

- ◐ Optional ability to store data encrypted in transaction log. (But always decrypted in user's local SQLite.)
	- Done (phase 1, field-value encryption): new `internal/core/crypt` uses AES-256-GCM under a per-value subkey derived from the entry's unique tx_id, so nonce reuse can't happen at any record count. Only field values encrypt; every other column stays clear, since git merge and replay need them. Encryption happens on the write path and decryption just before replay; an undecryptable value binds NULL so ciphertext never reaches the view. All four front-ends inherit it. Verified end to end: sensitive data never appears in the synced log, replay with the key restores it, and without the key it shows empty and warns. Full design is in design.md.
	- Reason: So that the data (in txlog) is not readable by the git hosting company, or anyone that gets ahold of it via git.
	- Gotcha: To still work as a txlog that git can sync and reconcile, some data will need to be stored in the clear - probably everything except for the actual field data values.
		- So some data will necessarily be leaky, except for the actual data values.
	- ✔️ The encryption key will need to be stored outside the repo. (E.g. a random binary -> base64 key stored in a text file, e.g. in the same place the user would store their own custom named queries.)
	- ✔️ The application should have CLI `--init` and `--encrypt=on` flags, that can set this up automatically. Flags can specify both git location, and db name. The machine-visible and user locations can be auto-computed from there, or overridden with additional flags.
		- ✔️ Keys are per-db. Not per-user or per-machine. (But each user must get ahold of the key outside of git methods. E.g. Discord chat or something.)
		- ✔️ Encryption can be turned on and off at will, per-host. Some data fields in the txlog will be stored in the clear, some encrypted. The app should be able to tell which is which (via a mechanism that doesn't add much weight to the txlog), and act accordingly.
			- But by default, if a user has the key file in their user directory for the db in question, fields should be encrypted - unless the user specifically turns encryption off for themselves, for that particular database (persistently).  ## Done: local pref lives in the registry record (unsynced); 'auto' = encrypt-if-key-present.
	- ✔️ If the txlog contains encrypted data, but the user either has no key file, for now, warn them with an instruction to obtain it somehow.
		- But for now, allow them to proceed, able to read and write unencrypted data only. In the future this may be optionally more locked-down by an owner or admin. (In the latter case it might be truly enforceable only if running as a web server.)
	- ✔️ If the txlog contains encrypted data, and the user has they key file but explicitly disabled encryption for themselves for the db in question, warn them so at least they are aware.
		- Same caveat about allowing them to continue, while in the future possibly offering a way to enforce encryption-only.
	- ◐ Additional hierarchical DDL keys:
		- ✔️ `encryption: always|never|auto`  ## Default 'auto', meaning only if initialized with `--encrypt=on`, or enabled later per-user config.  ## Done at database/table/field; outermost always|never locks lower levels; all-auto defers to local pref.
			- 'always' means that encryption is enabled at that level, and can't be overridden lower in the hierarchy (only higher).
				- If an encryption key file is not available, writing data is prohibited, for whatever fields are affected. (And only unencrypted data can be read.)
			- 'never' means that encryption is disabled at that level, and can't be overridden lower in the hierarchy (only higher).
			- 'auto' means whatever the db was initialized with, or user decides. Can be overridden higher OR lower in the heirarchy.
			- Use-cases:
				- Certain field must ALWAYS be encrypted (e.g. SSN.)
				- Certain fields must NEVER be encrypted (maybe some field used by a txlog preprocessor or something).
				- All fields of a certain table must ALWAYS be encrypted.
		- ◐ Levels this label can be applied: "database:", "table:", "field:", "ddl:" (which refers to whole self as a file), "named_queries:", "config:". Basically at any level that ultimately contains fields, and/or whole text files other than the whole txlog itself. (Even though actual encryption happens at the field-level only - this is just different ways to define what fields are or are not encrypted.  ## database/table/field DONE; ddl/named_queries/config (file-level) deferred with file encryption below.
	- 🔘 If the entire database is set with encryption on, then by default the DDL, code file[s], and named queries file in git, are encrypted too. Will need CLI flags to locally decrypt and re-encrypt them, so that they can be edited. (This would also have the benefit of protecting such files from accidental modification.)  ## Deferred (phase 2): file-level encryption of the DDL/.queries/.lua.
	- 🔘 Future enhancements:
		- 🔘 Ability to use multiple keys - e.g. by externally-managed responsibility level, team, etc.
		- 🔘 Ability to use public/private keys in addition to standard symmetric - e.g. able to read the configuration and DDL (and even some fields) with the public key, but cryptographically unable to reencrypt them without the private key (thus making them fundamentally read-only for certain externally-managed roles - in an enforceable way even for this distributed system, that doesn't rely on the data being locked away on a central web server).
		- 🔘 Think of a way that public keys could be securely distributed *in-application* (necessarily being synced by git). E.g., public key(s) and possibly even private keys, are encrypted with the main symmetric key, stored somehow in the main git-synced folder.
		- 🔘 Allow local symmetric keys to be generated with a passphrase.
		- 🔘 Allow re-encrypting existing data with a new key. (Perhaps key management could be done the way LUKS does it - with manageable keys that simply unlock a fixed bigger key. Would have to carefully think through how to accomplish such a thing, with such a distributed architecture.)

### Done

#### First steps

- ✔️ Repo scaffold: `go.mod`, package skeleton, arg-dispatch `main`, size-optimized build script, `.gitignore`. Compiles, vets, and gofmt-clean; all four front-ends stubbed and the web server serves its embedded asset.
- ✔️ DDL parser: `example.ddl` parses into an in-memory schema model through three layers (indent tree, type-directed values, semantic map). Tested, wired to `nanogitdb ddl <file>`.
- ✔️ SQLite view: builds and migrates the local `.sqlite` from the schema using pure-Go `modernc.org/sqlite`, with system columns and uniques/indexes, idempotent. Tested, wired to `nanogitdb build`. On-the-fly migration of existing data is a separate item below.
- ✔️ Transaction log: CSV read/write plus apply log to the SQLite view, with the log as source of truth. Append-only, field-granular replay (create/update/mark_delete/delete) in one tx. Tested, wired to `nanogitdb replay`. Added `row_id` to the entry (see design).
- ✔️ Core CRUD API: the single internal API every front-end and Lua calls, covering Create/Update/SetField/MarkDelete/Delete/Get/Query. Writes are log-first (append to truth, then apply to view), and it owns id, timestamp, and user stamping. Tested including a full rebuild from log. Validation, access, and triggers are separate items.
- ✔️ CLI front-end: full arg-based CRUD over the core, with verbs create/get/update/setnull/markdelete/delete/query each taking the same `<ddl> <sqlite> <logdir>` triple as replay and sync. Stateless and script-friendly until the config file lands. Writes stamp NANOGITDB_USER or the OS username, and `setnull` is its own verb so a literal "NULL" string stays expressible. Schema-op flags are their own backlog item.
- ✔️ Git sync: pull/merge/commit/push of the tx-log dir with a background loop, shelling out to `git`. Append conflicts auto-resolve via `merge=union`, and replay sorts by (date, tx_id) so clients converge. Tested including two-client convergence, wired to `nanogitdb sync`.
- ✔️ Lua host: `gopher-lua` bound to the same core CRUD as `db.{create,get,update,setfield,markdelete,delete,query}`, sandboxed to base/table/string/math/coroutine only with no os/io. Tested, wired to `nanogitdb --script`.
	- Note: now an enterprise-edition feature. The host moved behind the script seam, so the open-source build has no scripting host and drops the gopher-lua dependency.
- ✔️ Auto tables: users, groups, and the opt-in feature tables (m:m, comments, audit, access, attachments), defined in the same DDL users write and embedded in one source file. Built idempotently at startup, with default groups (owners/admins/users/guests) seeded log-first so they replicate. audit_trail opts out of the universal system columns via the new `system_fields: no` key, and access_rows only appears when a table opts in to row_level_access. First-user membership in owners/admins is deferred until user rows exist.
- ✔️ TUI front-end: `tview`/`tcell`, both pure Go with the cross-compile intact. Table list (user tables first, then built-ins), a rows grid that stays empty until a table is opened, a create/edit form that writes only changed fields, and soft/hard delete behind a confirm modal, all through the shared CRUD API. Tested headlessly on tcell's simulation screen, including a full boot-open-quit pass. `nanogitdb --tui <ddl> <sqlite> <logdir>`.
- ✔️ Web UI: stdlib `net/http` + `html/template` + `embed` with vendored, pinned htmx (2.0.4). Table sidebar, a rows grid that renders nothing until a table is asked for, a create/edit form writing only changed fields, and soft/hard delete behind hx-confirm; the 127.0.0.1-only binding is the access control. Front-end bring-up and table metadata live in shared `schema.OpenClient`/`schema.Catalog`, so the four front-ends can't drift. `nanogitdb --serve <ddl> <sqlite> <logdir>`.
- ✔️ CI/CD: a manual pipeline in `cicd/cicd.bash` that builds native (size-reported), cross-compiles windows-amd64/linux-arm64/windows-arm64 (pure Go, no extra toolchains; macOS stays deferred), zips windows into `dist/`, runs test.bash (vet, gofmt, tests, smoke), verifies the module and runs govulncheck, then dogfoods and publishes. Profiling beyond the size report waits until there's something worth profiling.
- ✔️ Supply-chain hardening: vendored deps, `go mod verify`, `-mod=vendor`, and `govulncheck`, all wired into the pipeline. The first govulncheck run found five reachable Go-stdlib vulns; pinning `toolchain go1.26.4` cleared them and it now scans clean.

#### Done - Bugs

#### Done - New features and enhancements

- ✔️ TUI colour themes: selectable, readable, dark and light (default dark)
	- Done: press `T` in the TUI for a picker of six themes - three dark, three light, default dark. Colours are fixed RGB rather than the terminal-palette names tview defaults to, so text stays legible whatever the surrounding terminal looks like. Switching applies live (the database picker included) and the choice persists in a user-global `settings.toml`, so it carries across sessions and databases.

- ✔️ Add "host_name" to the transaction log, in addition to user. (Because the same user might use multiple hosts, and that might be useful info.)
	- Done: `host_name` appended as the last tx-log column, so older logs without it still parse as an empty host and both widths survive a union merge. It's stamped automatically from the OS hostname, overridable with `NANOGITDB_HOST`, the machine analogue of `NANOGITDB_USER`. Metadata only, so replay ignores it. All four front-ends inherit it.

- ✔️ Tx-log garbage collection: mark + collect records older than N days
	- Done as the designed segment rotation, via `gc <ddl> <logdir>`. Entries for rows hard-deleted longer than `gc_age_days` ago (default 90) get dropped, survivors land in a write-once segment file, and the old files retire whole. No log line is ever edited in place, so the union merge stays safe. Reads walk the segments then the live file; new writes recreate it.
	- Note: sync now stages the whole log dir, so both the rotations and the attachments folder replicate. That also closes a replication gap left by the attachments item.
	- Note: if two clients GC at once they just duplicate survivors, which replay collapses on the next pass.

- ✔️ Tunable options (e.g. `git_sync_frequency`, tx-log GC age)
	- Done: a `tunables:` DDL section accepting both `key: value` and `key = value` forms. `git_sync_frequency` (default 60, 0 disables) drives a background git sync loop in the TUI and web UIs when the log dir is a git work tree; a pull that brings new entries full-replays into the view, so two open UIs converge without restarting. `gc_age_days` (default 90) is parsed and waiting for the GC item. Unknown tunables warn but still store.

- ✔️ DDL-defined scripting: triggers, events, before/after methods via the Lua host (now an enterprise-edition feature; the open-source build ignores `code:` hooks)
	- Done for the write path and app open: `code:` keys name functions in a `.lua` sidecar. Field `before_update` can veto or replace a value, table `before_update` can veto and runs after the field hooks, both `after_update` forms fire post-commit, and app `before_open` can veto the open. Trigger code gets the sandboxed `db.*` API, and writes from inside a trigger don't re-fire triggers, so hooks can't recurse. Hooks fire in the core CRUD API so all four front-ends behave identically; replay never fires them. Missing functions warn instead of blocking.
	- Note: sync and UI-lifecycle hooks and `validation: method:` are deferred (see design.md).

- ✔️ CLI schema ops: `--rename-table`, `--rename-field` (updates DDL + SQLite, adds aliases)
	- Done: `--rename-table` and `--rename-field`. The DDL is rewritten line by line so comments and formatting survive, with the old name added to an `aliases:` line, and the SQLite view gets an ALTER TABLE rename. The part that makes it safe: replay translates aliased names in old tx-log entries to the current ones, so a log written before the rename still rebuilds correctly. The log itself is never rewritten, since append-only stays sacred.

- ✔️ Granular access model: view -> table -> field|record, by user groups
	- Done with everything-open-until-configured semantics, so a userless setup keeps working. Identity is the write-stamp user resolved through the users table, with groups via the users/groups m:m. Blacklist wins and an empty whitelist means everyone; owners always pass and admins pass read and write but not delete. Writes are gated in the core CRUD API so no front-end can bypass the table, field, and row-grant rules. Reads are hidden at the catalog instead: unreadable tables and views vanish with a warning, unreadable fields drop from grids and forms, and ungranted rows filter out of row lists.
	- Note: raw SQL `query` stays open by design. The local file is the trust boundary and the write path is the hard gate (documented in design.md).

- ✔️ Predefined queries: plain-text query file, dropdown in the UIs
	- Done: queries live in a `.queries` sidecar next to the DDL, parsed with the same grammar and warn-and-fix validation. The TUI offers them on a view via 'p', loading the dataset into the focused block with the query's own columns; the web view page gets a dropdown and Run. A view's `startup_named_query` naming an active query loads it the moment the view opens in both UIs, and an unresolvable name degrades to the usual empty open. Queries without a `view:` are offered on every view, and `active: no` hides one without deleting it.

- ✔️ Opt-in table features driven by DDL: local + URI attachments, comments, audit trail, row-level access
	- Done in the core CRUD API so all four front-ends behave alike, and everything replicates through the tx-log like ordinary data. Audit capture rides the same commit as the change it records; same-user updates coalesce into one record keeping the oldest old-value per field, and a hard delete snapshots the row. Comments and both attachment kinds get API helpers plus UI across the TUI, web, CLI, and Lua. Copied files land under `<logdir>/attachments/` and sync with the log.
	- Note: Lua has no `attach_file`, since its sandbox has no file access.
	- Note: row-level access grants work here; enforcement belongs to the access-model item below.

- ✔️ Added a new hierarchical levels:
	- 'ui:' above "views:".
		- Also with a new child, "default_view":
			- If no default view specified, defaults to the first one defined.
	- 'database:' above "tables:" and "relationships:".
	- Done: both wrappers parse transparently, so older flat DDLs still work, and the default view opens on startup in the TUI and web UI with blocks still empty per the no-records-until-asked rule. A `default_view` naming an unusable view warns and falls back to the first. The example's `methods:` key was renamed to `code:`, with the old key still read.

- ✔️ DDL hierarchy doesn't have to be unique.
	- As an example, 'tables:'/'table:' can be defined multiple times
		- In that example, treat it as if there's only one 'tables:' definition, logically collapse the children with redundant 'tables:' into one.
	- Applies to all elements, not just 'tables:' and not just top-level.
	- Could be very helpful when lists get long and losing track of indentation could be a problem.
	- Hierarchies can be short-handed. For example, instead of "database:", "tables:", table: "table_name1", you could optionally specify `database/tables/table: "table_name1"`. (Or maybe '.' or '|' delimiter instead of '/'. Research what others do for DDL.)
	- Done: duplicate sections merge into the first occurrence at every level, while entities like table/field/view stay separate. A scalar redefined to a different value keeps the first and warns. The '/' path shorthand expands to real nesting.
	- Note: kept '/' as the delimiter since example.ddl already uses it; '.' collides with numbers and '|' reads as alternation.

- ✔️ Validate the DDL file and transaction log upon load. Fix soft syntax errors (with a warning), that can be safely assumed around. If something is genuinely incorrect too ambiguous to assume around, hard error on startup.
	- Done: DDL load drops nameless or redefined tables and fields and system-column collisions (first definition wins), and treats an unknown field type as text, each with a warning. Mixed indentation stays a hard error. Tx-log load warns per torn line with its file and line number instead of skipping silently; repeated header rows still skip quietly as normal union-merge leftovers. Replay skips and warns on mangled row ids and unknown operations, which may come from a newer client, rather than aborting the whole rebuild. All four front-ends surface the warnings.

- ✔️ On-the-fly schema change: update or rebuild local `.sqlite` when DDL changes with data present
	- Done additively: `build`, `replay`, and `sync` add a column for any field added to the DDL, and replay skips with a warning any log entry for a table or field the local schema doesn't know yet, so a schema edit can never strand an existing database. Type changes and field drops still mean a rebuild from the log, which loses nothing since the log is the truth.

- ✔️ Transaction log enhancements:
	- ✔️ Search and replace problematic symbols, e.g.:
		- Done: every entry is now exactly one physical line with no CSV quoting, so git's line-based union merge can never tear a record. Encoding and decoding happen only at the file boundary; the SQLite view holds real characters.
		- Note: 🗦NULL🗧 now expresses SQL NULL via a new `SetFieldNull`, and 🗦LDELIM🗧/🗦RDELIM🗧 escape literal delimiters so content can't spoof a marker.
		- ✔️ Delimiters for "unsafe" content: 🗦 and 🗧, then what's in-between are "constants" describing the character. For example:
			- 🗦NEWLINE🗧
			- 🗦RETURN🗧
			- 🗦SINGLE_QUOTE🗧
			- 🗦DOUBLE_QUOTE🗧
			- 🗦BACKTICK🗧
			- 🗦BACK_SLASH🗧
			- 🗦FORWARD_SLASH🗧
			- 🗦COMMA🗧
			- 🗦MISC_ILLEGAL🗧 (bell, backspace, etc. - actually those should just be scrubbed before landing)
			- etc.

- ✔️ View block `tree_grid` type + `parent_field`: hierarchical grid over a self-referential (same-table) parent link
	- Done, and it pulled in view rendering itself, since no front-end rendered views yet. Views now resolve once in the core, verifying tables, turning split hints into axis and percent, and inheriting readonly, then render in both TUI and web as nested split blocks. `tree_grid` orders rows depth-first along `parent_field` with indentation, and bad data like orphans, cycles, or a wrong field degrades visibly instead of hiding rows. Blocks open empty per the design and the "All" action loads them; `startup_named_query` hooks in once named queries land.

### Deferred

- 🔘 Native GUI (Fyne/Wails) - rejected for now; both need cgo/webview and break the pure-Go cross-compile

- 🔘 macOS build target - needs a Mac/SDK

### Canceled


## Application name ideas

