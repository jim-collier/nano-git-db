code:  ## Application-level
	before_open:  ## Function to run when app is loaded, but before showing any UI. Can cancel by returning false. Gets passed <name>, <path to tx log folder>.
	after_open:
	before_sync:
	after_sync:
	before_viewchange:
	after_viewchange:
	before_close:  ## Fires after field then table `before_update`.
	after_close:  ## After everything is committed and the UI goes away.

database:
	tables:
		table: "table_name1"
			aliases: table7, "old_table79"
			access:  ## table-level access by user groups (default: all; admins can always read/write, owners can always delete)
				read:
					whitelist:  ## Defaults to all unless populated
					blacklist:  ## Defaults to none unless populated.
				write:
					whitelist:  ## Defaults to all unless populated
					blacklist:  ## Defaults to none unless populated.
				delete:  ## Any/all records
					whitelist:  ## Defaults to all unless populated
					blacklist:  ## Defaults to none unless populated.
			fields:
				field: field_name1
					access:  ## table-level access by user groups (default: all; admins can always read/write, owners can always delete)
						read:
							whitelist:  ## Defaults to all unless populated
							blacklist:  ## Defaults to none unless populated.
						write:
							whitelist:  ## Defaults to all unless populated
							blacklist:  ## Defaults to none unless populated.
					aliases: field99, "favorite_candy"
					type:          string  #..........: string|int|float|bool|datetime[_local]|datetime_utc|binary  ## datetime[_local] stores UTC but displays in user's local time.
					special:       "%|($0|$2|etc) [and other valid currency symbols]"  ## Defaults internal processing (e.g. % <--> float), defaults a UI format for currency unless overridden, etc.
					isactive:      bool
					defaultval:    NULL  #............: In addition to a static value, a script function name can be given here, in format (without quotes): "fMyFunction()". Args that will be passed: table_name, field_name.
					null_ok:       true  #............: True if not specified.
					empty_ok:      n  #...............: True if not specified.
					validation:  #....................: Not all validations are valid for all data types. Executable spits warnings if so, and ignores what it can. (Or errors if serious enough.)
						required:  no
						minlen:
						maxlen:
						minval:       #...............: For numbers
						maxval:       #...............: For numbers
						regex:     "(?i)[a-z0-9 ]"  #.: As an example.
						method:  #....................: A script function that, if exists, passes inputs (table_name, field_name, value), and pass|fail boolean as return.
					code:
						before_update:  #.............: Function that, if exists, is sent: table_name, field_name, and a value - to validate and/or change. Return pass|fail boolean to cancel update.
						after_update:  #..............: Function that, if exists, is sent: table_name, row_id, field_name, read-only value.
					ui:
						visible: Y
						title: ""  #.............: Defaults to name.
						description: ""  #.......: E.g. for flyover text.
						order: 3.005  #..........: Display and tab order.
						readonly: n
						width:  ## Integer, approx number of characters.
						widget:  ## listbox|editbox|radio|checkbox|image|audio
						list_type: literal|dynamic|lookup  ## Checkbox can only be "literal", listbox can be any. Lookup requires SQL to return `id, string`.
						list_source:  ## E.g. "Bob", "Sally" | `SELECT name FROM ...` | `SELECT id, name FROM ...`
						format:  ## e.g. printf format string. Display-only format after saving.
				field: field_name2
					type:          bool
					defaultval:    false
					null_ok:       no
					empty_ok:      n
			code:
				before_update:  #.............: Function that, if exists, is sent: table_name, and a key:value array of field values [or something], to validate and/or update. Return pass|fail boolean to cancel update.
				after_update:  #..............: Function that, if exists, is sent: table_name, row_id.
			uniques:  ## These get automatically named. These are also indexed.
				field_name1, field_name2
				field3, field5
			indexes:  ## Warn if an identical index is already named as a unique (which is inherently also indexed).
				field_name1
			features:  ## And default values. These are typically shown in a related 1:M inserted grid, that can be drilled into.
				local_attachments: n
				uri_attachments: n
				comments: n  ## Arbitrary number of related comment rows.
				audit_trail: n
				row_level_access: n
			## Fields that all tables get, always and immutably:
			## Even tables "Automatic features that any table can opt-in to" get these:
				## May or may not be visible to UI
					- `id`: Table primary key and first field. It stores a GUID (either binary, hex, or base64 whichever makes the most sense). If binary, then program user-facing interfaces will need to be given [and able to give] hex or base64 representations). Indexed, unique.
					- `is_active`: defaults yes.
					- `date_created`
				## Hidden from UI
					- `is_deleted`: This gets set for record deletion, with garbage collection. Gets auto-added to uniques (except `id`) if not already specified.

database:
	relationships:
		relationship:  ## auto-named
			type:  1:m  ## The relationship is managed by a field on the child entity. This basically just provides optional cascading deletes, and isn't necessary to define, if not doing that.
			parent: table_1
			child: table_2
			parent_id_field: my_parent_id  ## Code makes sure this child field is indexed.
			cascade_delete: y  ## "y" is the main reason to use this feature, otherwise can be redundant.

database/relationships:
	relationship:  ## auto-named
		type:  m:m  ## Implies use of the "Many-to-many relationships" table. Very much not redundant with anything two entities can do themselves.
		parent: table_1
		child: table_2
		cascade_delete: n
		enable_audit_trail: n  ## The audit trail is on the m:m table.

ui:
	views:
		view: "people"
			aliases:
			startup_named_query: ...
			readonly: no
			access:  ## View access by user groups (default: all; admins can always read/write, owners can always delete)
				whitelist:  ## Defaults to all unless populated.
				blacklist:  ## Defaults to none unless populated.
				## Data-level read-or-write access managed at the table level. If the user has no read-access to the top-level table in the view, s/he can't access the view either.
			layout:
				block: "top"
					block: 1
						table: table_people
						type: form
						location:  ## <relative-to>, <above|below|left|right>, <%>
					block: "people-pets"
						table: table_pets
						location: 1, right, 25%  ## Dynamically figures out where to go based on what came before. In this case, it splits block "1", giving itself 25% of the split to the right.
						type: grid
				block: 3
					table: subordinates
					location: "top", below, 25%  ## Dynamically figures out where to go based on what came before. In this case, splits block "top", giving itself 25% of the split below.
					type: tree_grid  ## Hierarchical grid of records from the same table that are hierarchically related.
					parent_field: parent_self_id  ## The field in the table that identifies parent from same table.
					readonly: yes
	default_view: "people"
