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
| 🛠️   | Started, and/or partially complete
| ✅   | Complete
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

- ✅ Startup:
	- ✅ If the program is run from an active directory that has a DDL, use that one.
	- ✅ If the program is run without arguments and no DDL is in active directory, search user-level and system-level config directories for defined databases that point to valid DDLs.
		- If there are multiple, allow the user to choose them from an initial screen (or "create new").
		- If a database specified in the config directory cannot be loaded (e.g. can't find the DDL or it's irreversibly corrupt), include the database name in the list, but also with some kind of error indication, and brief text of why it can't be opened.
		- Rebuild anything that is missing and can be rebuilt or defaulted. (e.g. SQLite file.)
		- The list of databases should always include "Create new database", and "Open existing ...". Both will futher prompt for a location for the txlog and associated files. (User-specific, unsynced files go where user config file says. Autogenerate if non-existent.)
	- ✅ If no DDL and nothing in user config, the only options are "Create new database", and "Open existing ..."
	- Done: new `internal/core/config` package - a TOML registry (`BurntSushi/toml`, first new dep in a while; pure-Go, cross-compiles clean, re-scrubbed the vendor tree per the modernc gotcha). Records live at `<os-user-config>/ngdb/<name>/config.toml` (XDG_CONFIG_HOME / %AppData% via `os.UserConfigDir`; system dirs from XDG_CONFIG_DIRS / %ProgramData%, read-only). A record holds name, ddl_path, log_dir, sqlite_path, key_file, encryption, last_opened (key_file + encryption stored now for a stable format; acted on by items 4/5). `config.List` discovers + light-validates (DDL exists and parses; missing sqlite is fine - it rebuilds from the log on open). Startup routing: bare `nanogitdb` and `--tui`/`--serve` with no paths use a lone `*.ddl` in $PWD; else `--tui`/bare show the TUI picker (registered DBs, unopenable ones flagged `[!] name` + reason, plus "Create new database" [registers + opens] and "Open existing ..." [ad-hoc open]); `--serve` has no picker so it errors asking for the triple. Rebuild-missing-sqlite comes free from OpenClient. Headless picker tests via the tcell SimulationScreen. NOTE: "Create new" registers a pointer to an existing DDL - authoring/scaffolding a brand-new DDL file is a separate concern; the `--init`/`--config`/`--encrypt` FLAGS are item 4.

- ✅ Add to README.md:
	- ✅ A section on the bare-minimum, super-simple DDL syntax
		- ✅ Plus an example to make the simplest "to-do" hierarchical database and nested list, with the most minimal features. (E.g. task, status, open and close date, parent task, and related comment list.)
	- ✅ A section with the most basic CLI options to get started.
	- ✅ A link to the full DDL and options syntax in `syntax.md`
		- ✅ Create that file.
	- Done: README gained a "Quick start" (minimal schema; a full to-do example - hierarchical `task` with self-referential `parent_task` + `tree_grid` nested list + `comments` feature; basic CLI walkthrough) and a "Full syntax reference" pointer. Both DDL blocks use real tabs and were parse-verified (the to-do one exercised end-to-end: build/create-parent/create-child/comment/query). New `github/syntax.md` is the complete reference: DDL grammar, field types, system columns, uniques/indexes, features, access model, aliases/renames, relationships, views/blocks, tunables, the `.queries` and `.lua` sidecars, all CLI verbs + `--rename-*` flags, run modes, startup discovery/registry, and env vars (NANOGITDB_USER/HOST). Documents only implemented behavior; example.ddl's not-yet-wired keys are flagged as forward-looking.

- ✅ Partial flag list:
	- ✅ `--init[=]["git repo root to auto-calculate subfolders for (e.g. 'ngdb/dbname/'), or override default with specific 'git-repo-root-path/subfolder(s)/'"]`  ## If no arg specified, and already in a git root or DDL is in $PWD, use it. Otherwise error.
	- ✅ `--config[=]"Alternate user config file folder, name, and/or full filespec"`  ## Default = "~/.config/ngdb/dbname/" (or Windows analogue).
	- ✅ `--[en]crypt[ion][=]on|off|auto[matic]|yes|no|true|false|y|n|t|f|enable[d]|disable[d]`  ## If on, may be overridden by DDL values of 'never'. If off, may be overridden by DDL values of 'always'. Default=auto.
	- Done (`--init`, `--config`): `--init [path]` (cli/init.go) registers the lone `$PWD` *.ddl as a database (name = DDL base name), materializes the view (schema.OpenClient: build/bootstrap/seed), and picks the tx-log dir - inside a git repo -> `<repo-root>/ngdb/<name>` (txlog.RepoRoot added), an explicit dir -> verbatim, no arg + non-git -> `$PWD`. `--config <dir>` / `--config=<dir>` is a global prefix (main.go, recurses after stripping) that repoints the registry via new config.SetBaseOverride; composes with `--init`/`--tui`/etc. Both support the `=` form (splitFlag).
	- Done (`--encrypt`): landed with the encryption item below. `--encrypt[=on|off|auto]` is a global prefix (bare = on; config.NormEncrypt accepts yes/no/true/false/y/n/enabled/disabled); `--init --encrypt=on` mints the per-db key and persists the preference. Composes with `--config`/`--init`.

- ◐ Optional ability to store data encrypted in transaction log. (But always decrypted in user's local SQLite.)
	- Done (phase 1 - field-value encryption, branch `crypt`): new `internal/core/crypt` - AES-256-GCM under a per-value subkey derived (HKDF-SHA256, stdlib, no new dep) from the entry's unique tx_id, so nonce reuse is impossible for any number of records; cleartext context (table/field/row) bound as AEAD so repo tampering fails decryption. Only field values encrypt; all other columns stay clear (git merge + replay need them). Stored as `🗦ENC🗧base64url(nonce||ct||tag)` behind a new txlog marker (collision-safe like NULL; `Entry.Enc`). Encrypt on the write path (crud commit seals the log copy, view gets cleartext); decrypt pass before replay (schema.OpenClientWith + cli replay); undecryptable values bind NULL (never leak ciphertext to the view). All four front-ends inherit it. Verified end-to-end: sensitive data never appears in the synced log; replay with key restores it, without key shows empty + warns.
	- Reason: So that the data (in txlog) is not readable by the git hosting company, or anyone that gets ahold of it via git.
	- Gotcha: To still work as a txlog that git can sync and reconcile, some data will need to be stored in the clear - probably everything except for the actual field data values.
		- So some data will necessarily be leaky, except for the actual data values.
	- ✅ The encryption key will need to be stored outside the repo. (E.g. a random binary -> base64 key stored in a text file, e.g. in the same place the user would store their own custom named queries.)
	- ✅ The application should have CLI `--init` and `--encrypt=on` flags, that can set this up automatically. Flags can specify both git location, and db name. The machine-visible and user locations can be auto-computed from there, or overridden with additional flags.
		- ✅ Keys are per-db. Not per-user or per-machine. (But each user must get ahold of the key outside of git methods. E.g. Discord chat or something.)
		- ✅ Encryption can be turned on and off at will, per-host. Some data fields in the txlog will be stored in the clear, some encrypted. The app should be able to tell which is which (via a mechanism that doesn't add much weight to the txlog), and act accordingly.
			- But by default, if a user has the key file in their user directory for the db in question, fields should be encrypted - unless the user specifically turns encryption off for themselves, for that particular database (persistently).  ## Done: local pref lives in the registry record (unsynced); 'auto' = encrypt-if-key-present.
	- ✅ If the txlog contains encrypted data, but the user either has no key file, for now, warn them with an instruction to obtain it somehow.
		- But for now, allow them to proceed, able to read and write unencrypted data only. In the future this may be optionally more locked-down by an owner or admin. (In the latter case it might be truly enforceable only if running as a web server.)
	- ✅ If the txlog contains encrypted data, and the user has they key file but explicitly disabled encryption for themselves for the db in question, warn them so at least they are aware.
		- Same caveat about allowing them to continue, while in the future possibly offering a way to enforce encryption-only.
	- ◐ Additional hierarchical DDL keys:
		- ✅ `encryption: always|never|auto`  ## Default 'auto', meaning only if initialized with `--encrypt=on`, or enabled later per-user config.  ## Done at database/table/field; outermost always|never locks lower levels; all-auto defers to local pref.
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

- ✅ Repo scaffold: `go.mod`, package skeleton, arg-dispatch `main`, size-optimized build script, `.gitignore` - compiles, vets, gofmt-clean; all 4 front-ends stubbed and the web server serves its embedded asset
- ✅ DDL parser: parse `example.ddl` into an in-memory schema model - 3 layers (indent tree -> type-directed values -> semantic map); tested, wired to `nanogitdb ddl <file>`
- ✅ SQLite view: build/migrate the local `.sqlite` from the schema (pure-Go `modernc.org/sqlite`) - system columns + uniques/indexes, idempotent; tested, wired to `nanogitdb build`. (On-the-fly migration of existing data is a separate item below.)
- ✅ Transaction log: CSV read/write, and apply log -> SQLite view (log is source of truth) - append-only CSV, field-granular replay (create/update/mark_delete/delete) in one tx; tested, wired to `nanogitdb replay`. (Added `row_id` to the entry - see design.)
- ✅ Core CRUD API: the single internal API every front-end and Lua calls - Create/Update/SetField/MarkDelete/Delete/Get/Query; log-first writes (append to truth, then apply to view); owns id/timestamp/user stamping; tested incl. full rebuild-from-log. (Validation/access/triggers are separate items.)
- ✅ CLI front-end: full arg-based CRUD over the core - verbs create/get/update/setnull/markdelete/delete/query, each taking the same `<ddl> <sqlite> <logdir>` triple as replay/sync (stateless and script-friendly until the config file lands); writes stamp NANOGITDB_USER or the OS username; `setnull` is its own verb so a literal "NULL" string stays expressible. Schema-op flags (`--rename-table` etc.) stay their own backlog item.
- ✅ Git sync: pull/merge/commit/push the tx-log dir, auto-resolve, background loop - shells out to `git`; append conflicts auto-resolve via `merge=union`; replay sorts by (date,tx_id) so clients converge; `Syncer.Run` ticker loop; tested incl. two-client convergence; wired to `nanogitdb sync`.
- ✅ Lua host: `gopher-lua`, bind Go<->Lua to the same core CRUD - `db.{create,get,update,setfield,markdelete,delete,query}`; sandboxed (base/table/string/math/coroutine only, no os/io); tested; wired to `nanogitdb --script`.
	- Now an enterprise-edition feature. The host moved behind the script seam, so the open-source build has no scripting host and drops the gopher-lua dependency.
- ✅ Auto tables: users, groups, and the opt-in feature tables (m:m, comments, audit, access, attachments) - defined in the same DDL users write, embedded in one source file; built idempotently at startup; default groups (owners/admins/users/guests) seeded log-first so they replicate; audit_trail opts out of the universal system columns per the design (new DDL key `system_fields: no`); access_rows only appears when a table opts in to row_level_access. First-user membership in owners/admins is deferred until user rows exist.
- ✅ TUI front-end: `tview`/`tcell` (both pure Go, cross-compile intact) - table list (user tables first, then built-ins), rows grid that stays empty until a table is opened (per the design's "no records shown by default"), create/edit form that writes only changed fields, soft/hard delete behind a confirm modal; all through the shared CRUD API. Tested headlessly on tcell's simulation screen, including a full boot-open-quit event-loop pass. `nanogitdb --tui <ddl> <sqlite> <logdir>`.
- ✅ Web UI: stdlib `net/http` + `html/template` + `embed` + vendored/pinned htmx (2.0.4) - table sidebar, rows grid that renders nothing until a table is asked for, create/edit form writing only changed fields, soft/hard delete behind hx-confirm; 127.0.0.1-only binding is the access control. Front-end bring-up (build, bootstrap, replay, seed) and table metadata now live in shared `schema.OpenClient`/`schema.Catalog`, so CLI/Lua/TUI/web cannot drift. `nanogitdb --serve <ddl> <sqlite> <logdir>`.
- ✅ CI/CD: manual pipeline (lint, build, regression tests, profile, cross-compile, publish, dogfood copy) - `cicd/cicd.bash` now runs: build (native, size-reported), cross-compile (windows-amd64, linux-arm64, windows-arm64 - pure Go so no extra toolchains; macOS stays deferred), windows zip into `dist/`, test.bash (vet + gofmt + tests + smoke), `go mod verify` + `govulncheck`, dogfood install, publish (n8git). Profiling beyond the size report is deferred until there is something worth profiling.
- ✅ Supply-chain hardening: vendor deps, `go mod verify`, `-mod=vendor`, `govulncheck` - all wired into the pipeline. First govulncheck run found 5 reachable Go-stdlib vulns; fixed by pinning `toolchain go1.26.4` in go.mod, now scans clean.

#### Done - Bugs

#### Done - New features and enhancements

- ✅ Add "host_name" to the transaction log, in addition to user. (Because the same user might use multiple hosts, and that might be useful info.)
	- Done: `host_name` column appended last to the tx-log entry (so pre-host_name 9-field logs still parse - they read as empty host, and both widths coexist under a git union merge). Stamped automatically in the core CRUD API from the OS hostname (override `NANOGITDB_HOST`), the machine analogue of `user_id`'s `NANOGITDB_USER`. Metadata only - replay doesn't apply it to the view. All four front-ends inherit it (default set in `crud.New`).

- ✅ Tx-log garbage collection: mark + collect records older than N days
	- Done as the designed segment rotation, via `gc <ddl> <logdir>`: entries of rows hard-deleted more than `gc_age_days` ago (default 90) are dropped; survivors land in a write-once `txlog-<stamp>.csv` segment and the old files retire whole - no log line is ever edited in place, so the union merge stays safe. Reads walk segments then the live file; new writes recreate `txlog.csv`. Sync now stages the whole log dir (rotations AND the attachments folder replicate - the latter fixes a gap from the attachments item). Concurrent GC by two clients just duplicates survivors; replay idempotence converges the views and the next pass collapses the extra segment. Eligibility is computed at collect time; the `ok_to_garbage_collect` column remains as a future cross-client mark.

- ✅ Tunable options (e.g. `git_sync_frequency`, tx-log GC age)
	- Done: a `tunables:` DDL section (both `key: value` and the design doc's `key = value` form). `git_sync_frequency` (default 60, 0 disables) now actually drives something: the TUI and web UIs start a background git sync loop when the log dir is a git work tree, and a pull that brings new entries full-replays into the view - so two open UIs converge without restarting. `gc_age_days` (default 90) is parsed and waiting for the GC item. Unknown tunables warn but store.

- ✅ DDL-defined scripting: triggers, events, before/after methods via the Lua host (now an enterprise-edition feature; the open-source build ignores `code:` hooks)
	- Done for the write path + app open: `code:` keys name functions in a `.lua` sidecar (same base name as the DDL). Field `before_update` can veto or replace the value, table `before_update` can veto (and runs after the field hooks, per the design), both `after_update` forms fire post-commit, and app `before_open` can veto the open. Trigger code gets the sandboxed `db.*` API; writes from inside a trigger do not re-fire triggers, so hooks cannot recurse. Hooks fire in the core CRUD API, so all four front-ends behave identically; replay never fires them. Missing functions warn instead of blocking. Deferred: sync and UI-lifecycle hooks, `validation: method:` (noted in design.md).

- ✅ CLI schema ops: `--rename-table`, `--rename-field` (updates DDL + SQLite, adds aliases)
	- Done: `--rename-table <ddl> <sqlite> <old> <new>` and `--rename-field <ddl> <sqlite> <table> <old> <new>`. The DDL file is rewritten line-based (comments and formatting survive; the old name lands on an `aliases:` line, extending one if present), the SQLite view gets ALTER TABLE ... RENAME [COLUMN], and - the part that makes renames safe - replay now translates aliased table/field names in old tx-log entries to the current names, so a log written before the rename still rebuilds a fresh view correctly. The log itself is never rewritten (append-only stays sacred).

- ✅ Granular access model: view -> table -> field|record, by user groups
	- Done with everything-open-until-configured semantics (a userless setup keeps working). Identity = the write-stamp user resolved through the users table; groups via the users<->groups m:m. Blacklist wins, empty whitelist = everyone; owners always pass, admins pass read/write but not delete. Writes are gated in the core CRUD API so no front-end can bypass them (table write/delete rules, per-field write rules, row grants on row_level_access tables); reads are hidden at the catalog: unreadable tables/views vanish with a warning, unreadable fields drop from grids/forms, ungranted rows filter out of row lists. Raw SQL `query` stays open - the local file is the trust boundary, the write path is the hard gate (documented in design.md).

- ✅ Predefined queries: plain-text query file, dropdown in the UIs
	- Done: queries live in a `.queries` sidecar next to the DDL (same base name; `project/example.queries` shows the format), parsed with the same grammar and warn-and-fix validation as the DDL. The TUI offers them on a view via 'p' (dataset loads into the focused block with the query's own columns); the web view page gets a dropdown + Run. A view's `startup_named_query` naming an active query loads it the moment the view opens - in both UIs - and an unresolvable name degrades to the usual empty open. Queries without a `view:` are offered on every view; `active: no` hides one without deleting it.

- ✅ Opt-in table features driven by DDL: local + URI attachments, comments, audit trail, row-level access
	- Done in the core CRUD API so all four front-ends behave alike, and everything replicates through the tx-log like ordinary data. Audit capture rides the same commit as the change it records (same-user updates coalesce into one record per the design, keeping the oldest old-value per field; hard delete snapshots the row). Comments and both attachment kinds have API helpers plus UI: TUI 'c' opens a per-row comments+attachments panel and 'u'/'f' attach, the web edit form gains the same sections, the CLI gains comment/comments/attachuri/attachfile/attachments verbs, and Lua gains db.comment_add/comments/attach_uri/attachments (no attach_file - the sandbox has no file access). Copied files land in `<logdir>/attachments/<id>` and sync with the log; `cp --reflink=auto` behavior comes free via copy_file_range. Row-level access GRANTS work (access_rows + m:m to groups); enforcement belongs to the granular-access-model item below.

- ✅ Added a new hierarchical levels:
	- 'ui:' above "views:".
		- Also with a new child, "default_view":
			- If no default view specified, defaults to the first one defined.
	- 'database:' above "tables:" and "relationships:".
	- Done: both wrappers parse (transparently, so older flat DDLs still work), and the default view now opens on startup in the TUI and web UI - blocks still load empty per the no-records-until-asked rule. A default_view naming an unusable view warns and falls back to the first. The example's `methods:` -> `code:` rename came along (old key still read).

- ✅ DDL hierarchy doesn't have to be unique.
	- As an example, 'tables:'/'table:' can be defined multiple times
		- In that example, treat it as if there's only one 'tables:' definition, logically collapse the children with redundant 'tables:' into one.
	- Applies to all elements, not just 'tables:' and not just top-level.
	- Could be very helpful when lists get long and losing track of indentation could be a problem.
	- Hierarchies can be short-handed. For example, instead of "database:", "tables:", table: "table_name1", you could optionally specify `database/tables/table: "table_name1"`. (Or maybe '.' or '|' delimiter instead of '/'. Research what others do for DDL.)
	- Done: duplicate sections merge into the first occurrence at every level (entities like table/field/view stay separate; a scalar redefined to a different value keeps the first and warns), and '/' path shorthand expands to real nesting. Kept '/' - it's what example.ddl already uses, '.' collides with numbers/TOML-style keys, '|' reads as alternation.

- ✅ Validate the DDL file and transaction log upon load. Fix soft syntax errors (with a warning), that can be safely assumed around. If something is genuinely incorrect too ambiguous to assume around, hard error on startup.
	- Done: DDL load now also drops nameless/redefined tables and fields and system-column collisions (first definition wins) and treats unknown field types as text - each with a warning; mixed indentation stays the hard error. Tx-log load warns per torn/unreadable line (file + line number) instead of skipping silently, repeated header rows still skip quietly (normal union-merge leftovers), and replay skips-and-warns mangled row ids and unknown operations (could be a newer client's) instead of aborting the whole rebuild. All four front-ends surface the warnings (TUI to stderr, since the UI covers stdout).

- ✅ On-the-fly schema change: update or rebuild local `.sqlite` when DDL changes with data present
	- Done additively: `build`/`replay`/`sync` now ALTER TABLE ADD COLUMN for fields added to the DDL, and replay skips (with a warning) log entries for tables/fields the local schema doesn't know yet - so a schema edit can never strand an existing database. Type changes and field drops still mean rebuild-from-log (the log is the truth, so nothing is lost).

- ✅ Transaction log enhancements:
	- ✅ Search and replace problematic symbols, e.g.:
		- Done: every entry is now exactly one physical line (no CSV quoting), so git's line-based union merge can never tear a record. Encoding/decoding happens only at the file boundary; the SQLite view holds real characters. Bonus: 🗦NULL🗧 now expresses SQL NULL (new `SetFieldNull`), and 🗦LDELIM🗧/🗦RDELIM🗧 escape literal delimiters so content can't spoof a marker.
		- ✅ Delimiters for "unsafe" content: 🗦 and 🗧, then what's in-between are "constants" describing the character. For example:
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

- ✅ View block `tree_grid` type + `parent_field`: hierarchical grid over a self-referential (same-table) parent link
	- Done, and it pulled view rendering itself in with it (no front-end rendered views at all yet): views now resolve once in the core (tables verified, split hints -> axis + percent, readonly inheritance) and render in both TUI and web as nested split blocks; tree_grid orders rows depth-first along `parent_field` with indentation, and bad data (orphans, cycles, wrong field) degrades visibly instead of hiding rows. Blocks open empty per the design; the "All" action loads them - `startup_named_query` hooks in when named queries land.

### Deferred

- 🔘 Native GUI (Fyne/Wails) - rejected for now; both need cgo/webview and break the pure-Go cross-compile

- 🔘 macOS build target - needs a Mac/SDK

### Canceled


## Application name ideas

