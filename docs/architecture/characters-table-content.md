# Characters module: table content

The content the master registers (MR-025, RN-23, ADR-0018) is stored, served and takes effect at once, in `characters`.
The engine that checks and merges it is [`Content.With`](rules-table-content.md); this page is the storage, the API, the live read, the switches, the packs and the spell page.
For the module around it see [Characters](characters.md).

## The content source

Every read of rules content in `characters` goes through a `ContentSource` (`contentsource.go`), never through a `*rules.Content` held in the service:

```go
For(ctx context.Context, tx pgx.Tx, campaignID string) (*rules.Content, TableRules, error)
```

- **It returns the campaign's content and table rules.** `TableRules` are the table's choices (RN-23, RN-24); their zero value is the SRD defaults (the player chooses to roll or take the average for level hit points, all ability-score methods, critical doubles the dice, death saves visible, combat with a map, new maps without fog, no reminders): each field says what the table changes, never what the SRD already does.
  The type belongs to `platform/tablerules` (`tablerules.Rules`, a value with no behaviour) and `characters.TableRules` is an alias, so `campaigns` (which stores the rules), `characters`, `maps` and combat share it without importing each other.
- **It receives the caller's transaction** (`nil` outside one): the implementation reads the campaign's content revision inside it, by the rule that no read goes through the pool inside `db.InTx`.
  Whoever is in a transaction passes the `tx`, and the `campaignID` comes from what the call already knows.
- **One content per request.** A request asks for the content once and passes it along; in combat views, `CombatRoster.ContentNames(ctx, tx, campaignID)` returns a names function tied to a content.
  A write asks for the content before the transaction (or inside it, with the `tx`), never after the commit: a failure there would make the client repeat a write that already took effect.
- **`ContentFor`** skips reading the rules, for what does not need them.
  `SRDSource` (the SRD for every campaign, no database) is for tests and for those without a database; `Config.SRD` serves the few points without a campaign.
- **The seams with `play`** carry campaign and transaction where they read the content without knowing which campaign: `CombatRoster.ContentNames`, `CreatureSheet`, `CreatureTurnOptions`, `CreatureSave` and `CreatureEyes`.
- **The `ListContent` catalogue is built per content**: the same content gives the same catalogue, and a table's content has its own.
  At most 8 catalogues are kept (`maxCatalogs`), each held weakly, so the contents of an edited table do not stay alive.

## Storage and API

```mermaid
flowchart LR
    M["master: CreateTableEntry<br/>UpdateTableEntry<br/>ArchiveTableEntry"] --> T["one transaction"]
    T --> R["raises the revision<br/>campaign_content_state"]
    R --> V["checks the whole content<br/>SRD.With(overlay)"]
    V -->|"refused: nothing changes"| X["invalid_argument<br/>one violation per field"]
    V -->|"accepted"| W["writes the entry<br/>campaign_content"]
    S["any read<br/>ContentSource.For(tx)"] --> Q["reads the revision in the tx"]
    Q --> C{"in the cache<br/>(campaign, revision)?"}
    C -->|"yes"| U["returns the *Content"]
    C -->|"no"| B["reads the rows in the same tx<br/>builds SRD.With and stores it"]
    B --> U
```

| Piece | What it does |
| --- | --- |
| Storage | `campaign_content` (one row per entry; `data` is the kind's message as protojson) and `campaign_content_state` (the campaign's revision). It lives in `characters` and not in `campaigns` because `characters` does not write a column of another module. The switched-off keys are `campaign_content_off`. See [Data](../data/README.md). |
| The protos | `rules/v1/table_content.proto`: one message per kind (`TableClass`, `TableSubclass`, `TableRace`, `TableSubrace`, `TableBackground`, `TableSpell`, `TableFeat`, `TableOption`, `TableClassFeature`, `TableCreature`, `TableWeapon`; eleven kinds, `TableContentKind`) one to one with `rules`' `Table*` types, and `TableEntry`, which reads return. The writes and the full read are `TableContentService`; the catalogue stays `ContentService.ListContent`, which also carries the table's entries and the `archived` mark. |
| Writes (master only) | `CreateTableEntry`, `UpdateTableEntry`, `ArchiveTableEntry`, `UnarchiveTableEntry`. Each is one transaction that starts by raising the revision (so two writes of a campaign run one after the other), reads all entries, applies the change and checks the campaign's whole content with `SRD.With`: a refused content changes nothing, the revision included. The error is `invalid_argument` with a `TableContentRefusal` detail, one `TableContentViolation{field, reason}` per engine error, with the path in the body's own names (`table_class.levels[4].features[0].effects[1].value`). The server adds `immutable` (kind, key, parent class or race changed; a feature key the entry never had; a spell going between cantrip and leveled), `duplicate_name` (another entry of the kind has the name, case-insensitive), `size_limit` (64 KiB an entry, `MaxTableEntryBytes`) and `limit` (more features or traits than the engine allows, counted in the handler before the transaction, so a huge body never holds the campaign's revision). A stale entry (`expected_revision` different from the entry's `revision`) is `aborted` with `TableContentBlocked` `STALE`. An archived entry can still be edited and stays archived; archiving twice is `ARCHIVED`, unarchiving one that is not archived is `NOT_ARCHIVED`. Archiving and unarchiving are not changes to the entry: they touch neither its `revision` nor its `updated_at`, but they raise the campaign's revision for the cache. |
| Keys | The entry's key is born from the name (accents removed, `a-z0-9-`, up to 60, with `-2`, `-3`... if taken; the engine's reserved words become a single word) and never changes, not even when the name does. Feature keys are the server's: `feature:class-<start of name>-<hash>--<feature name>@mesa` (and `subclass`, `race`, `subrace`, `background` in place of `class`, with `trait:` and `background-feature:` on the last three). The start of the entry's name is up to 16 characters and the hash is 6 characters of a SHA-256 of the whole entry key, so two long names that start the same never collide and the feature still fits the engine's 60 characters; `--` never appears in a name. A feature keeps its key while it exists: the editor returns it on update and, if it does not, the one of the same level and name takes it back. A key the entry never had is refused (`immutable`). |
| `ListTableEntries` | The entries in full, by kind and name. The master receives all (archived too, with `archived_at`) and, in each, how many sheets use it (`characters_using`, "Em uso por N fichas"). A player receives the entries they may see, in full (numbers and effects), never a count and never the archived or switched-off ones (a subclass or subrace whose class or race is switched off neither, unless their sheet uses it). Of the creature kind a player receives only the entries whose key a character of theirs holds, live, archived or switched off, whole. A pending member does not read it. |
| What a player never receives of an archived entry | The players' catalogue is a copy (`proto.Clone`, filtering the six lists) without archived and switched-off entries. `GetLevelUpOptions` does not offer an archived subclass unless the player's sheet has it. `GetSpellDetails` answers `not_found` for an archived spell unless a sheet of theirs uses it. No archived key reaches a player even by reference: the catalogue, `ListTableEntries` (which also clears archived keys from each body's class lists, granted spells and always-prepared lists), the spell details and the level-up options clear them from the reference lists (`Spell.class_keys`, `list_class_key`, `TableSpell.class_keys`, `TableCasting.list_from`, `TableEffect.spells`...). The catalogues carry what is needed to read the engine's content: `Race.choice_bonuses`, `Race.choice_bonus_sets`, `Race.sizes`, `Race.size`, `Subclass.spellcasting`, `ClassSpellcasting.list_class_key`, `Background.equipment_pt`. |

## The live source

`TableSource` (`tablesource.go`; `NewTableSource(pool, srd, campaigns)`) is one object for content and rules.

- **`For(ctx, tx, campaign)`** reads the revision inside the caller's `tx` and returns `SRD.With(overlay)` and the stored rules (`TableRulesReader.StoredTableRules`, which `campaigns.Service` implements, through the same `tx`); `ContentFor` skips the rules.
  With no revision row (a campaign without content or switches) it returns the SRD itself, building nothing.
- **Outside a transaction,** a hit costs reading the revision through the pool; a miss opens a read transaction that reads the revision and the rows again, so the content stored under a revision is that revision's.
- **The cache** is by (campaign, revision), at most 8 contents (`maxLiveContents`) and at most 24 MiB of stored entry data (`maxLiveBytes`), the newest always staying; the least recently used leaves first.
  A miss builds the content only for whoever asked, reading the rows in the same `tx`, with no shared `singleflight`: a request holding a connection while waiting for another that needs one is how the pool hangs.
  Two that miss at once both build and the second to store wins.
  A write never fills the cache, since what it builds can still roll back.
  Memory and time are in [Operations](../operations/reference.md#memory-budgets); a content of 1,000 entries keeps 4 MB (7 MB with 6,000 effects) and a miss takes 80 to 90 ms, so the eight contents come to 59 MB at most ([Performance](../performance.md#table-content-at-its-limits)).
- **The order against a sheet write.** `CreateCharacter` and `UpdateCharacter` read the content before the transaction and again inside it; if the revision changed, the sheet is checked again with the new content.
  `LevelUpCharacter` reads inside the transaction.
  Serialization conflicts (40001) are retried by `db.InTx`.

## "A classe mudou", on the server

A change takes effect at once, also on locked sheets.
The rules side (`ChangedSince`, `Issue.Keys`, the sentences) is in [Table content](rules-table-content.md#a-classe-mudou).
This is what the server stores and shows:

- Each sheet stores, inside its own document and written by `checkSheet` (create, edit and level-up; what the client sends is ignored), the content revision (`FullSheet.content_revision`), the notices it already had (`known_issues`, each by its identity, `Issue.ID`: code, field and exact sentence, and for a lost feat or option the code, the list its pick is in and the entry's key, with no sentence) and, for a still-flagged entry, the revision at which the sheet was up to date with it (`content_baselines`). Only the campaign's master receives `known_issues`; `content_baselines` is sent to every reader of the sheet, as it holds only keys of entries the sheet itself uses, which its owner reads, and nothing in the web reads it.
- On every read, `Content.ChangedSince` lists the table entries the sheet uses that changed later, and the server shows for each changed entry only the new notices whose keys include it: `DerivedSheet.changed_content` (the key, the name, the field, the revisions, the date of the change, the exact sentences and, in `codes`, the code of the issue each sentence comes from: one per sentence, in the same order, built in `addChangedContent`) and a `table_content_changed` `Issue` per entry. The web tells the shield line apart by the code at its place (`shield_idle`), for each line and never by its words.
- The notice disappears by itself when nothing remains tied to it; a change that left no new notice on that sheet shows nothing; there is no "dismiss".
- Saving the sheet does not clear it: on saving, the notices that were flagged and still stand stay out of `known_issues` and the entry stores the base revision in `content_baselines`, so the next read flags them again; only fixing what the notice says clears it.
- It is a notice, never a `Validate` error.
  So that `Validate` never refuses an unrelated edit because of a change, the server refuses on the entry itself the only change `Validate` checks against a sheet: a spell going from cantrip to leveled.
  The others (a class's casting, a race's bonuses, a background's skills) only generate `Derive` notices.
- Only the owner and the master read it; the player never receives the history of changes or who made them.
- `UpdateTableEntry` returns `affected_characters`: the sheets that use the entry and ended up with new notices tied to it, with the player's name, for the master's "Fichas com aviso" list.

## Archived and switched-off entries as new choices

- `CreateCharacter` and `UpdateCharacter` refuse an archived key that the new sheet has and the earlier one did not (`Content.ArchivedKeys`, comparing the two; a creation's is compared with nothing): `failed_precondition`, `CharacterBlocked` `ARCHIVED_CONTENT`, with the `content_key`.
  `LevelUpCharacter` and `PreviewLevelUp` refuse with `LEVEL_UP_REFUSAL_REASON_ARCHIVED_CHOICE`.
- The same for a switched-off key (or a child of a switched-off class or race), but only for a player: `SWITCHED_OFF_CONTENT` and `LEVEL_UP_REFUSAL_REASON_SWITCHED_OFF_CHOICE`.
  The master is not refused: the switch hides the option from players, not from any sheet the master edits.
- The keys of a sheet include the spell of a scoped pick (`choiceKey=spell:key`), so a pick of a spell counts as the sheet using it.
  A ritual held in the book that stops being a ritual is an issue at its pick (`spell_level`).
  Its owner reads the notice of changed content for it.
- A sheet that already uses the key keeps working (`Derive`, `Validate`, names, level-up), and an unrelated edit is never refused.
- A sheet with an `@mesa` key does not change campaign: `CheckSheetCanMove` (`failed_precondition`, `TABLE_CONTENT_STAYS`, with the first key) exists for it.
  Nothing calls it yet, since characters are not copied between campaigns; whoever builds that must call it before writing.

## Player options and the live hint

The master switches on and off, per campaign, each class, subclass, race, subrace, background, spell, feat, option, class feature, creature and weapon, from the SRD and the table (`campaign_content_off`, one row per switched-off option; no row means on).
The master switches creatures on the options screen, in its "Criaturas" group.
The creature editor has the same switch (`app-players-switch`).
What is switched off never reaches a player, in any read; the master sees everything with the `off` mark.
The engine side is [Switches and hidden keys](rules-table-content.md#switches-and-hidden-keys).

| Piece | What it does |
| --- | --- |
| In the source and cache | `TableSource` reads the switched-off keys in the same transaction as the revision and adds them to the content; the cache stays by (campaign, revision). A campaign that only switched options off already has a revision and a content of its own. |
| What the player never receives | A switched-off option is treated as archived at every point: the catalogue, `ListTableEntries` (which also clears switched-off keys from each body's references), `ListSpells` (`SpellFilter.Hidden`; a hidden `class_key` answers like an unknown one, an empty list, unless a sheet of theirs uses the class), `GetSpellDetails` (`not_found` unless a sheet of theirs uses it), `ListCreatures` (a table creature is left out of the list a player reads unless a character of theirs holds a live creature of that key) and `GetCreature` (`not_found` unless a character of theirs holds one), the level-up options, the feat-choice cards (`PreviewChoices` and the level-up choices), and the reference lists inside other entries. No text names a hidden race, feat or feature, and a feat that asks only for hidden races is left out of the options, the cards and the player's `ListTableEntries`, unless a sheet of theirs has it. A player whose sheet has a hidden race, class or subclass still reads that entry's traits, features and options. Everything but the creatures filters with the single predicate `Content.Hidden`; a table creature is the master's whatever its state, so a player reads it only when a character of theirs holds a live creature of its key (`callerHeldCreatures`). |
| `ListOptionSwitches` (master only) | The screen's list by kind and name: every class, subclass, race, subrace, background, spell, feat, table option, table class feature and table creature of the campaign, each with the key, kind, name, `table`, `off`, `archived`, `hidden`, `parent_key` (a subclass's class, a subrace's race), `level` (a spell's) and `characters_using` (how many player sheets use it, once per sheet). Reads the content and the sheets in one transaction. |
| `SetOptionSwitches` (master only) | Switches up to 1,400 options (`MaxOptionSwitches`: the SRD's 358 and the 1,000 entries of a table, so a whole campaign switches off in one call; about 0.2 s for 1,000 keys) in one transaction: reads the content, checks each key (`invalid_argument` for a key that is not switchable, a repeated one, none or more than 1,400), raises the campaign's revision and stores the set. The response is built without the live source (`SRD.With` of the rows the transaction sees plus the new set), because a write never fills the cache. An archived entry can also be switched. A request that changes nothing rolls the transaction back (the revision does not rise) and notifies nobody. The response carries the revision, how many options changed (`changed`) and those that changed for the players (`options`, the subclasses of a switched-off class included). |
| The live hint | Every write to table content (create, edit, archive, unarchive, the switches, an import) sends `content_changed` (stream field 23, no fields) to everyone watching the campaign, after the commit. It carries neither the entry nor the key (RN-10): each reader reads again with their own role. `characters` calls `Live.PublishContentChanged`, which `play` implements with the puzzles' time gate (one hint per campaign every 250 ms; the last change is never left without a notice) and the hub's `Coalesce`. With no open session nobody listens, and readers read the current content on opening. |

Who listens in the browser is on [the browser side page](characters-web.md#live-updates).

## Content packs (`ExportTableContent`, `ImportTableContent`)

A campaign's own content travels as a pack (MR-025): one file, `{"format": "meurpg.table-content", "version": 1, "name": "...", "entries": [...]}` in proto JSON (`TableContentPack`; the proto field names), each entry a `TableEntry` with only its `key`, `kind`, `name_pt` and body.
The revision, dates, counts, `off` and `archived` belong to the campaign and are left out; the features keep their keys.
Only the master calls either RPC.
The code is `tablecontent_pack.go` and `tablecontent_pack_json.go`.

| Piece | What it does |
| --- | --- |
| Export | One read transaction: every entry of the campaign, archived ones included, sorted by key; the pack's name is the campaign's (`CampaignNames`). |
| Strict reading | The request carries the file as `pack_json` (bytes) because the Connect JSON codec drops unknown fields: `readPackJSON` parses it with `protojson`, walks the JSON against the message and turns every field the message lacks (a misspelled name) into a violation of its entry at its path (`unknown_field`; outside any entry, a `pack.` violation). |
| The pack itself | Checked before the database is read, in both modes, and answered as `invalid_argument` with a `TableContentRefusal` whose fields start with `pack`: at most 3 MiB (`MaxPackBytes`; 1,000 entries are 1.1 to 1.4 MiB), `format` and `version`, at most 1,000 entries (the `limit` and `size_limit` violations of the pack are those of `pack_json`, the file: a `pack` message with more than 1,000 entries, or a `pack_json` over 3 MiB, is refused earlier as `invalid_argument` with a `RequestRefused` detail, see [Request size limits](api.md#request-size-limits)), a one-line label of 80 characters at most, for each entry a kind, a key `<kind>:<slug>@mesa` of that kind, or `option:<set>:<slug>@mesa` with a known set for an option (the SRD's keys, another suffix, capitals and reserved words are `bad_key` and `reserved_key`), no repeated key, exactly the body of its kind and a `name_pt` equal to the body's. |
| Planning | `planImport` runs for each entry what create and update run (the shape, the feature count, the feature keys, the 64 KiB), then compares with the stored one: new, updated (the fields that differ), unchanged, or refused. A feature key the stored entry lacks is kept when it starts with the entry's own stem; the engine refuses one another entry already has. Every violation of an entry is reported. |
| The engine | The whole campaign (what is stored with the pack's entries put in) goes through `Content.With`, with the pack's keys as `Strict`. `With` reports one entry at a time, so each failing pass refuses the entries it names and leaves them out of the next (at most 25 passes, `maxImportPasses`); violations of the whole table (a limit, a stored entry the pack would break) are violations of the pack as a whole. |
| Preview | A read transaction: nothing is written and the revision does not move. One outcome per entry, in the pack's order, and the totals. |
| Apply | One transaction: raises the content revision, plans, then writes every new and updated entry at that one revision, or nothing: a refused entry makes the call `invalid_argument` with all the violations, each with its entry's `key`. An import that changes nothing rolls the bump back. Updated entries keep their `archived_at` and their switch; new ones are on for the players. After the commit: `content_changed` once, and the names of the players of the sheets left with issues. |
| Idempotency | `idempotency_key` is scoped to the campaign (`campaign_content_imports`: the key, the hash of the request, the answer). Inside the transaction, after the bump, the key is looked up: the same request returns the first answer and writes nothing; another request is `invalid_argument`. |
| Leaks | Both RPCs are master-only, and nothing new reaches a player (the leak and authorization tests list them). |

## Table classes and subclasses end to end

The server gives the class editor everything it needs, and the flows of a table class (create, level up, play, change) work end to end; the web draws what the server sends and computes no table.

```mermaid
flowchart LR
    E["class editor"] -->|"GetClassTableDefaults"| D["the SRD's 20-level table,<br/>by casting type"]
    E -->|"GetEffectMenu"| M["the closed effect menu<br/>with Portuguese names"]
    E -->|"CreateTableEntry<br/>UpdateTableEntry"| W["With checks everything"]
    W -->|"refuses"| V["one violation per field<br/>table_class.levels[4].slots[2]"]
    W -->|"accepts"| F["the sheet uses the class<br/>creation, multiclass, level-up, combat"]
    F -->|"the master changes the class"| C["A classe mudou<br/>one sentence per notice"]
```

- **The two reads** (`tablemenu_rpc.go`, master only): `GetClassTableDefaults` and `GetEffectMenu` return `rules.Content.TableDefaults` and `EffectMenu` unchanged (see [Table content](rules-table-content.md#the-class-editors-data)).
- **One field per refusal.** Every refusal of a class or subclass is a `TableContentViolation` with the exact field the editor draws and a stable reason; what `With` says about the whole content keeps the field empty and the 1,000-entry limit has the reason `limit`.
- **Multiclass at creation.** `CreateCharacter` accepts a sheet with more than one class, each with its subclass, the table's included.
  The SRD prerequisites are sheet notices (`multiclass_prerequisite`) for the master and refuse a player's sheet; a subclass before its level is the `subclass_level` notice.
  Hit points follow the table rule (RN-24): with "roll", the calculation is of levels after the first, summing the classes (`HIT_POINTS_RULE` refuses the average and missing rolls), and the first level is the first class's full die.
  Spell slots follow the multiclass caster table.
- **A class's own list names SRD spells.** `TableCasting.srd_spell_keys` goes through `tableCastingOf` into the overlay like the other casting fields and is stored with the body (protojson in `campaign_content.data`; an absent field is an empty list).
  The overlay builder checks it with the class (see [Table content: rules](rules-table-content.md#the-class)), so a refusal is a violation at `table_class.casting.srd_spell_keys[<i>]` or at the field itself.
  `ListSpells`, the sheet's `Derived` and the level-up read the class's list from the campaign's content, so a character of the class prepares and learns those spells and gets no `spell_not_on_list` for them.
- **A subclass whose spells are expanded.** `TableSubclass.spells_expanded` goes through `tableSubclassOf` with the always-prepared list.
  The sheet then has no given spell from the subclass: its spells are on the class's list to choose from, and the `spell_sources` of `PreviewChoices` offer them (`patron_spells`) from their class level on.
- **Expertise only where proficient.** `TableEffect.only_proficient` goes through `tableEffectsOf`; `Derive` reads it when the sheet is worked out, so a change of the character's proficiencies moves the doubled skills with no write.
  The effect menu served by `GetEffectMenu` carries it as the `yes_no` field `only_proficient` of the `proficiency` type.
- **The casting subclass at level-up.** `LevelUpSubclass` carries what a third caster adds on choosing the subclass: `spells`, `spells_kind`, `spell_list_class_key`, `max_spell_level`, and for one that prepares, `prepares` and `prepared_max_after`.
  The subclass's always-prepared spells enter the sheet at their level and do not count against the limit.
- **A table spell in combat** goes through the same `CombatSpell` and the campaign's content: attack, save (half or nothing), damage per level or per cantrip tier, healing with the modifier, concentration, slots and the log work as for an SRD spell.
  `link.Spell` carries `TargetCount` and `TargetPerLevel` and `maxTargetsOf` uses them before the spell's type.
  `GetTurnOptions` carries `SpellTargets.targets_per_level`.

## The other speeds

A table's `speed.fly`, `speed.swim`, `speed.climb` and `speed.burrow` modifiers fill `Derived.SpeedFlyFt`, `SpeedSwimFt`, `SpeedClimbFt` and `SpeedBurrowFt` (0 for none), and a `breathes_water` effect sets `Derived.BreathesWater`.
`combatCharacter` copies the four speeds to `link.Character`, and the combatant keeps them for the master and the character's player.
Combat reads no breath: `breathes_water` stays on the sheet.

## The sheet of a race and a subrace

`Derive` merges a subrace into its race in this order:

1. The race's traits, minus the ones the subrace replaces (`replaces_trait_keys`).
2. The subrace's own traits.
3. The choices: the options the character picked, among them the one of a trait with options.

A replaced trait is not owned: its effects, its choice and its options are gone, and a stored pick for them is reported as not offered.
The skills that only a replaced trait gives are gone too.
The ability bonuses are the race's and the subrace's added, or the subrace's alone when it has `replaces_ability_bonuses`.
Then the race's fixed bonuses and the ones the player places are gone together, no pick is asked for them, and the race's share of the points the player places on abilities is none.
The class's Ability Score Improvement points remain.

The catalog says what a subrace replaces (`replaces_trait_keys`, `replaces_ability_bonuses`, and `replaced_skill_keys` for the race's skills that go with the traits), and the editor's preview follows it.
A player's copy of a subrace drops the replaced keys that are hidden (an archived or switched-off entry).
The table editor has the inputs for them: a trait's "Opções para escolher", and on a subrace the "Substituiu os bônus de <raça>" switch and a checklist of the race's traits.
The checklist reads a table race's saved traits, or an SRD race's from the menu's `race_traits`.
A marked key that is no trait of the race shows as a red row until it is unmarked; the server refuses it at `table_subrace.replaces_trait_keys[i]`.
The "Só você vê" card beside the preview is the master's editor note, never sent to a player.
A player reads the "Substitui" line of the catalog only for the traits they can see: a player who cannot see the race reads none.

## Feat options and picks

`TableFeat.options` is a list of `TableFeature`, at most 60, and `TableEffect` carries `name`, `set` and `cap`; `tableFeatOf` and `tableEffectsOf` copy them to the engine.
The server makes the keys of the options like a feature's (`feature:feat-<start of name>-<hash>--<option name>@mesa`), keeps them on update and refuses one the entry never had (`immutable`); a pack keeps the ones that start with the entry's own stem.
A violation inside an option names `table_feat.options[j].effects[k]...`.
A player's copy takes hidden keys out of the options' effects too.
A feat's own options hide only with the feat; only the options of an option set hide one by one.
The picks of a feat are stored in `feature_choice_keys` and judged with the other choices: a player's save with a pick or an option the feat does not offer is `CHOICE_NOT_OFFERED`.
The editor menu offers the `score.<ability>` targets (mode `add`, with `cap`), the kinds `ability` and `damage_type`, `name`, and `set` as a closed list that leaves out `table_feature`.

## Magic bonuses of the gear

`full.item_bonuses` gives the armor, the shield or a weapon of the sheet a bonus from +1 to +3 (`rules.ItemBonus`).
It is saved with the rest of the full sheet, so it follows the same lock: the owner while the sheet is editable, the master always.
A bonus on armor or on a shield adds to the armor class; on a weapon it adds to the attack and damage of its line and makes the line magical, never a match for the `magic` weapon filter.
A piece that is carried adds nothing, a shield behind a two-handed weapon adds nothing, and a piece that asks for attunement adds nothing until `attuned` is set.
`Derived.ItemModifiers` lists every bonus with its source and says, for the ones that add nothing, why.
The combat reads the attack lines and the armor class of the same derivation.
The bonus belongs to the key: two weapons with one key are one row and share it, so a plain and a +1 copy of one weapon cannot be told apart.

### Infusion records in the derivation

`rules.Build.Infusions` holds the live infusion records of the character as holder.
The loader fills it, and the sheet never stores it.
A record is valid when all of these hold.
The content knows its infusion key, and the host feature has its limit rule.
The character has the host feature and the key is among its `feature_choice_keys`.
The option has not lost its prerequisite.
A gear record's piece is on the sheet, and no older record holds the same piece.
No manual bonus stands on the piece of an item bonus record.
The host feature is on the class (and subclass) the content gives it.
The class level for the bonus and the limit is the character's level in that class.
When two records hold one gear key, the oldest stands and the other is not valid now.
An item bonus record on a key that already has a manual bonus is not valid: the manual bonus stands and the record gives no number.
A record that is not valid gives no number and no name, takes no slot of the limit, and takes no attunement place.
It still has its row in `Derived.Infusions`, with the reason in `ReasonPT`.
When it becomes valid again it can bring the attunement count above the maximum; nothing ends the attunement for it.
`Computed` on a row means the app handles the infusion, with a number or a mark.
`Computed` false is a text only infusion, which the table applies.
`ITEM_BONUS` on a gear key becomes an `ItemBonus` of that key inside the deriver, at `BonusAt(class level)`.
The armor class, the attack lines, `weaponMarks` and `Derived.ItemModifiers` read the one path a manual bonus takes.
`Build.ItemBonuses` of the caller is not changed.
A class level below the first bonus step gives no number and no line.
The bonus follows the manual rules: a carried piece adds nothing, and an infusion that asks for attunement adds nothing until the record is attuned.
The bonus of a key applies to every copy of the key.
`SPELL_FOCUS_BONUS` adds the bonus to `Spellcasting.AttackBonus`, so the spell attack lines carry it.
It lists a line with the target `spell_attack`.
The best focus that counts is the one that applies.
The bonus does not reach the attacks of granted spells.
A focus does not change the numbers of its own weapon.
`REPEATING_AMMUNITION` leaves the weapon's attack without an ammunition key and keeps its improvised melee line.
`RETURNING`, `RESISTANCE_LABEL` and `NONE` change no number; the resistance label's damage type is the record's parameter.
The infusion's name is on the gear entry of its piece, on the attack of its weapon, and on each item modifier line of the record with `infusion_id`.
`Derived.InfusionHosts` has, for each host feature of the character, the live limit at the class level and how many valid records use it.
`rules.AttunedCount` counts a valid record that asks for attunement and is attuned like an attuned magic item.
`MaxAttunements` stays the one place for the maximum.
`rules.Validate` accepts a sheet that has a manual bonus and an infusion on one key: the manual bonus stands.
A save refuses a manual `item_bonuses` entry that is new or changed against the stored sheet on a key that a valid item bonus record infuses (`rules.CheckManualBonuses`).
The refusal is at `full.item_bonuses[i].key` and says to end the infusion or leave the manual bonus off.

## Table options

`TABLE_CONTENT_KIND_OPTION` (`option` rows of `campaign_content`, key `option:<set>:<slug>@mesa`) is an option the master adds to one of the SRD's option sets: an Eldritch Invocation, a Metamagic option, a fighting style, a Pact Boon, a Hunter's Prey, Defensive Tactics, Multiattack or Superior Hunter's Defense, or, in the set `table_feature`, an option of a table feature's option list (`table_option.offered_by` is that feature's key).
The key names the set and the set never changes (`table_option.set`, `immutable`); the slug is unique within the set, and the key may have up to 100 characters (migration 00281 widens `campaign_content_key_valid` and `campaign_content_off_key_valid`).
`TableOption` is a feature (name, text, effects) plus `set` and an `OptionPrerequisite`; the engine side is in [Table content](rules-table-content.md#table-options).
It goes through the same writes, archive mark, switch (`SetOptionSwitches`, `ListOptionSwitches` lists it with the kind), pack, "A classe mudou" notice ("A opção mudou") and player view as the other kinds; a player's copy drops a hidden class, option, pact or spell from the prerequisite and from the effects.
A player's copy of an option leaves `offered_by` blank when the offering feature is hidden to the player, (archived or switched off), so its key never reaches a player.
The choice engine reads it with the SRD options: `GetLevelUpOptions`, `PreviewChoices`, `CompleteCharacterChoices` and the sheet's own save list it in the choice of its set, blocked with its reason when the character lacks a prerequisite, and refuse a pick that does not qualify (`PREREQUISITE_UNMET`) or that the master switched off for the players (`SWITCHED_OFF_CONTENT`).

## Swapping the option of a table feature at a level-up

`TableFeature.swap_levels` travels with the rest of the feature: `tableFeatureOf` copies it into `rules.TableFeature.SwapLevels` (`featureSwapLevelsOf`), and the content pack (`ExportTableContent`, `ImportTableContent`) carries it as protojson, so a pack round-trip keeps it (`TestTheSwapLevelsOfAFeatureTravelInAContentPack`).
`GetLevelUpOptions` and `PreviewLevelUp` append the feature groups of `LevelUpOffer.FeatureSwaps` to `LevelUpOptions.swap_groups` (group = the feature's key, `name_pt` = its name), after the table rule has cleared the ASI swap groups and before the hide-from-player step.
`LevelUpChoices.swap_feature_options` (`LevelUpPickSwap`, at most 20) is read by `levelUpChoicesFromProto` into `rules.LevelUpChoices.FeatureSwaps`; `checkLevelUpKeys` bounds its key lengths; `newLevelUpPlan` refuses with `SWAP_NOT_ALLOWED` (`choices.swap_feature_options`) what the offer does not allow.
`levelUpNames` names the group, the given-up and the taken option, so the level-up's own row names the swap, and earlier `ListLevelUps` rows are not touched.
`LevelUpCharacter` calls the optional `OptionSwaps` hook (`optionswaps.go`, `Service.SetOptionSwaps`) once per level that swaps at least one such option, in the level-up's transaction right after the sheet and the record are written; its error aborts the level, and nothing is connected by default. The hook may run more than once for one level (the transaction retries) and must do nothing outside `tx`. Before it, `endGivenUpOptions` clears in the same transaction what each given-up option had stored in the character's own tables: the used counts of its resources (`character_vitals.resources_used`, for the keys of the resources whose `Source` is the option and that the new sheet no longer defines) and the states it started (`characters.active_states`, keys of the states whose `Source` is the option); what other modules own is theirs to clear through the hook. `FeatureSwapLevels.levels` holds at most 20 entries.

## Table class features

`TABLE_CONTENT_KIND_CLASS_FEATURE` (`classfeature` rows of `campaign_content`, key `classfeature:<slug>@mesa`, migration 00282) is a feature the master adds to a class at a class level, optionally taking the place of one of the class's own.
`TableClassFeature` has `class_key`, `level`, `replaces_feature_key` and the feature body; the key and the class never change (`immutable`), the level, the replaced feature and the body do.
It goes through the same writes, archive mark, switch (`SetOptionSwitches`, `ListOptionSwitches`), pack and player view as the other kinds, and a sheet that has the class gets the notice "A classe mudou" when it changes.
The engine side is [Table class features](rules-table-content.md#table-class-features).
A player's copy drops a hidden class, a hidden replaced feature and the effects' hidden references.
The body may carry a `summon`.
A player's copy keeps only its `creature_keys`: the table creatures the player holds, and the SRD's.

## Table creatures

`TABLE_CONTENT_KIND_CREATURE` (`creature` rows of `campaign_content`, key `creature:<slug>@mesa`, migration 00289) is an SRD creature with adjustments: hit points, armor class, a damage bonus on its attacks, extra text traits and attacks of its own.
Its numbers can be formulas of its owner: `tableCreatureOf` maps all of them, and a violation reads like `table_creature.attacks[2].to_hit`.
`TableCreature` has `base_key` (an SRD creature, which never changes: `immutable` at `table_creature.base_key`) and the adjustments; the limits and refusals are in [Table creatures](rules-table-content.md#table-creatures).
A base the SRD lacks is refused as `dangling_reference` at `table_creature.base_key`, and an entry over 64 KiB as `size_limit`.
It goes through the same writes, archive mark, switch (`SetOptionSwitches`, `ListOptionSwitches` lists it with the kind) and pack as the other kinds.
Its player view is the whole entry or none: `playerReads` lets a creature row through only for a player who holds its key.
Nothing is deleted: the way out of an entry is the archive mark or the switch.

- **Where it resolves.** `GiveCreature` accepts its key where it accepts an SRD creature, and a table spell's `summon` or a table feature's brings it ([Table spell summons](rules-table-content.md#table-spell-summons), [Table feature summons](rules-table-content.md#table-feature-summons)).
  `GetCreature`, `ListCreatures`, `CreateNpcFromCreature` and the combat read it from the campaign's content.
  The NPC made from it has "criatura da mesa" in its description.
  "Pôr no combate" keeps one combat-only NPC per campaign and creature for the SRD's, whose stat blocks never change.
  For a table creature the NPC's key also holds a digest of the numbers of the sheet the entry makes (the description is left out).
  An edit that changes the numbers makes another NPC.
  An edit that changes only the name, or changes the numbers back, finds the NPC it had.
  A monster added after the edit has the entry's current numbers, and the monsters already in a fight keep the ones they joined with.
- **When a number is copied.** `character_creatures.hp_max` is copied from the entry when the creature is given.
  Editing the entry changes no creature already held.
  For an entry with no formula, the armor class (`CharacterCreature.armor_class`) and the actions are read from the content every time.
  A held creature shows the entry's current armor class and attacks with its current damage bonus.
- **Formula creatures.** For an entry with a formula, the creature is created with a snapshot of its numbers (`character_creatures.stat_block`, migration 00297). A creature a table spell summoned has the circle of the cast slot in `slot()`, and one a table feature summoned has 0, and `source` `table` with the spell in `summon_key` (migration 00299).
  `insertSummoned` derives the recipient's sheet with the campaign's content and active states, makes the environment with `rules.OwnerEnv`, runs `ResolveCreature` with the circle of the slot the spell was cast with (0 for a creature the master gave), stores the JSON and takes `hp_max` from the result.
  `CreatureSheet`, `CreatureTurnOptions`, `CreatureSave` and `CreatureCheck` take the creature's ID and read the snapshot, and so does the list's `armor_class`.
  A creature whose row is gone has the content's static numbers (`TestMR578_ACreatureWithoutARowFallsBackToTheStaticNumbers`).
  The snapshot follows neither the owner's later levels nor an edit of the formulas; the text of the creature is read from the content.
  A snapshot is used whenever the row has one, whatever the entry's formulas are (the list and the combat read it the same way).
  A combat read costs one extra row lookup for each table creature held by a character (`statBlockOf`, because an edit can drop the formulas of a creature that has a snapshot); an SRD creature costs none.
  `rules.ResolvedDerived` matches the snapshot's own attacks to the entry's by the key of their action: one in both keeps the snapshot's numbers, one new in the entry, or renamed, has the static numbers, and one the entry lost is dropped.
  `GetCreature` and `ListCreatures` show the content's owner-less stat block, not a snapshot; the creature list's `armor_class` and the combat show the snapshot.
  "Pôr no combate" and "Criar NPC" have no owner and use the entry's static fields.
- **Archived and switched off.** An archived or switched-off entry keeps resolving for the creatures already held.
  They stay in the owner's list and join combats.
  Archived, switched off or live, a table creature is the master's: a player reads it only while a character of theirs holds a live creature of its key.
  `ListCreatures`, `GetCreature` and `ListTableEntries` leave it out for any other player (`GetCreature` answers `not_found`).
  The holder reads it whatever the entry's state, and the master reads every entry and can give it.
- **Who uses it.** `characters_using` is 0 for a creature entry, because a held creature is not part of a sheet.
  No sheet gets the notice "A classe mudou" when a creature entry changes.
- **What it is not.** The SRD spells' summon choices, Wild Shape, and the encounter generator, evaluator and swaps do not offer it.
  The encounter tools refuse its key as unknown content.
- **Packages.** A campaign package carries the creature entry with the rest of the table content.
  A package without the creature of an NPC's link is refused as unknown content.
  A package battle encounter that names a table creature is refused as unknown content.
- **The web app.** The creature editor of "Conteúdo da mesa" makes and edits a creature from an SRD base, which never changes after the first save.
  It edits the name, the fixed numbers and formulas, the traits and the attacks.
  It also edits up to 12 rows of resistance, immunity or vulnerability, each with its damage types and a "Vale contra".
  The "Vale contra" is off for a vulnerability and for a type that is not bludgeoning, piercing or slashing.
  The holder of the creature reads the rows on its sheet (`GetCreature`); the editor is the master's.
  The master switches the creature in the editor.

## Wild Shape and spell slots

`wild_shape`, `slot_recovery` and the `gives_slot` and `spends_slot` fields of `grant_action` are on the effect menu ([rules](rules-table-content.md#the-closed-menu-of-effects)); `tableEffectsOf` maps their proto fields (`max_cr`, `max_cr_from`, `form_cost`, `magical_attacks`, `no_fly`, `no_swim`, `gives_slot`, `spends_slot`, `recover_max_level`, `recover_levels`) like the others.
`combatCharacter` fills `link.Character.WildShapeCost` from `Derived.WildShape.Cost`, and `featureActionOf` fills `link.FeatureAction.GivesSlot` and `SpendsSlot`.
`tableEffectsOf` maps `use` (`featureUseOf`: scopes and `who` as the rules' words, an unknown number as `unknown`, which the rules refuse), `featureActionOf` fills `link.FeatureAction.Use` and `actionOptionToProto` the `ActionOption` fields of a feature use ([Feature use](rules-table-content.md#feature-use)).
The vitals keeper has `GiveSpellSlot`, `SpendSpellSlot` and their undos; a slot made by `GiveSpellSlot` is counted as created, like Flexible Casting's, and goes with the next long rest.
`UseArcaneRecovery` takes the feature key and reads `vitalsMax.recoveries` (`Derived.TableRecoveries`); with no key it is the wizard's own.

## Rerolls, minimums and the critical range

`tableEffectsOf` maps the proto fields `rolls` and `at_most` of `TableEffect` to the effect, and `damage_type` and `value` to a `roll_minimum`; `resource`, `cost`, `when` and `text_pt` are the fields the other effects already use.
A `granted_spell` also maps `uses_cost`, `no_material_components` and `cast_level` (`tableEffectsOf`).
`GrantedSpell`, `CastingGrant` and `SpellGrantOption` carry them from `rules.GrantedSpell` (`Cost`, `NoMaterialComponents`, `CastLevel`) as `uses_cost` (0 for an at-will grant), `no_material_components` and `cast_level`.
The sheet words the cost ("Gasta 2 usos de ...") and the "Sem material" tag.
The cast sheet shows "gasta 2" in the grant line and drops the "M" of the component line behind its "?".
The cost and the waived material are display: the server spends the cost and enforces no component.
`cast_level` is the level the server casts the spell at with no slot: the spell's own when the effect sets none, 0 for a cantrip.
A cast with a slot has the slot's level.
A `modifier` on `critical_range` uses `target`, `mode` (`set`), `value` and `applies` (the scope: `weapon`, `spell` or `both`).
All three are on the effect menu ([reroll](rules-table-content.md#reroll), [roll minimum](rules-table-content.md#roll-minimum), [critical range](rules-table-content.md#critical-range)).
`GetEffectMenu` returns the entries of `reroll` and `roll_minimum` with their fields and the list `roll_applies`, the field `applies` of `modifier` with the list `critical_scopes`, and the target `critical_range` among the modifier targets.
`EffectMenuValue` carries `only_modes`, `min` and `max` for a target of a modifier: the modes it takes (empty is every mode) and the bounds of a literal value (both zero is unbounded).
`menuValues` copies them from `rules.MenuValue` unchanged.
`rerollRulesOf` copies `Derived.Rerolls` into `link.Traits.RerollRules` for combat.

### Flags, damage reduction and carrying capacity

`tableEffectsOf` copies `TableEffect.flag` to `Effect.Flag`; the other fields of `natural_attack` (`attack_name`, `damage_dice`, `damage_type`, `ability`, `reach_ft`) are the ones `pseudo_weapon` already uses.
`traitsOf` (`combattraits.go`) copies `Derived.Flags` into `link.Traits.Flags` and `Derived.DamageReductions` into `link.Traits.DamageReductions` (source, amount, nonmagical), which combat reads through `Traits.HasFlag` and the reduction step.
`derivedToProto` puts `Derived.CarryCapacityLb` in `DerivedSheet.carry_capacity_lb`. The sheet writes it in the app's two units, metric first (`weightWithPounds`, 1 lb = 0,5 kg): "90 kg (180 lb)".
The natural attack lines are in `Derived.Attacks` like any other, so they reach the sheet and the combat with no copy of their own.

## Spell targets and the "Magias" page

Every spell, SRD or table, says who it catches, and the players' "Magias" page (MR-045) reads that, the search and the filters from the server.
The code is in `rules/spelltarget.go`, `rules/spelllist.go`, `characters/spelllist.go` and `characters/spelldetails.go`.

**An SRD spell's target.**
The importer reads 5e-database's `area_of_effect` (shape and size in feet) into `spells.json` (`area_type`, `area_size_ft`) and refuses a shape that is not cone, cube, cylinder, line or sphere and a size that is not a multiple of 5 (see [Rules content](../rules-content.md)).
`srdTarget` decides in this order:

| When | The target |
| --- | --- |
| The spell is in `effects/spell_targets.json` | What the file says |
| The spell has a structured area | `TargetArea`, with the shape and size. The structured data wins; the text is not read. |
| Magic Missile and Scorching Ray | `TargetCreatures`, 3 and 1 more per spell level above the spell's own (`PerSlotLevel` 1): a dart or a ray each, and the same number the app reads in `targets_per_level` |
| A spell attack | `TargetCreature` |
| The SRD text says area ("20-foot radius", "up to three creatures", a Self spell that deals damage or asks for a save...) | `TargetCreatures` without a number (`Count` 0: the caster picks how many; the master judges) |
| Self range | `TargetSelf` ("Só quem conjura") |
| The rest | `TargetCreature`, with `PerSlotLevel` 1 when "at higher levels" says "one additional creature" |

`SpellTarget.LabelPT()` writes the label sent in `SpellDetails.target.label_pt`: "Uma criatura", "Várias criaturas", "Só quem conjura" or the area ("Cone de 4,5 m", "Esfera de 6 m", "Linha de 30 m"), with metres from `MetersPT` (5 ft = 1.5 m, decimal comma, no decimal when whole, kilometres from 1,000 m).
The app writes the rest of the spell (time, range, duration, components) in `spell-details-format.ts`.

**`effects/spell_targets.json`** is handwritten and corrects, in our words, the spells the database or the text gets wrong.
The types are closed: `creature`, `creatures` (with `count` and `per_slot_level`), `area` (shape and size), `self` and `none` ("no creature": a point, an object, a place, nobody to pick); every key is an SRD spell, `label_pt` replaces only the page's text, and the file follows the content revision rule.
For `self` and `none`, `play` lists only the caster, refuses another target (`link.Spell.CasterOnly`) and rolls nobody's save.
The file covers, by kind: one creature (Telekinesis), only the caster (the detect spells, Speak with Plants, Branding Smite, Divine Favor, Flame Blade, Contact Other Plane, Magic Jar, Scrying, Wish), no creature (Arcane Eye, Magnificent Mansion, Dancing Lights, Druidcraft, Fabricate, Light, Major Image, Minor Illusion), several creatures with the SRD's number (Bless and Bane, Mass Healing Word, Prayer of Healing, Mass Suggestion, Telepathic Bond, Water Walk, Wind Walk, Astral Projection, Heroes' Feast), an area the database lacks (Hypnotic Pattern, Plant Growth, Purify Food and Drink, Glyph of Warding), and spells where only the text changes (Flame Strike, Forbiddance, Mirage Arcane, Teleportation Circle, Guards and Wards, Fire Storm).
`TestStructuredAreasAgainstTheText` pins the list of spells where the structured area and the text still disagree; spells that only the text calls area and the file does not correct stay "several creatures" without a number, which blocks nobody since the master is never limited by the number of targets.

**`ContentService.ListSpells`.**
Active members only (the pending member gets `not_found`), `IDEMPOTENT`, with `query`, `class_key`, `levels`, `school_keys`, `character_id`, `page_size` (1 to 400, 100 by default) and `page_token`; it returns the light `Spell`, ordered by Portuguese name, with `total`.

- **The search** finds the Portuguese or English name, without accents and case-insensitive.
- **The class** is its list: the spells that cite it, or the list the table class reuses.
  A casting subclass (the third caster) takes the list of the class it casts from.
  A class without a list gives an empty list.
- **"Só as que posso aprender"** (`character_id`): the list of each casting class on the sheet, up to that class's highest level, cantrips included; in a multiclass, each class's list by its level in it.
  The sheet's owner and the master may ask; another player gets `not_found` and a basic sheet `failed_precondition`.
- **Who sees what.** The player never receives an archived or switched-off spell, nor an archived or switched-off class's key in `class_keys`; the master receives them with `archived` or `off`.
  Switching a class off does not hide its list's spells: only the class key leaves `class_keys`.

**The "Outro" background.**
The free-text background follows SRD 5.1's "Customizing a Background": the sheet's `CustomBackground` has `proficiency_keys` (two tools or languages), `feature_name` and `feature_text` (the player's words) and `equipment` (text).
`Validate` checks the limits (at most 2 tools or languages, each an SRD tool proficiency or language, no repeats; feature name up to 40 characters; text up to 1,000; equipment up to 500).
What is missing is a `Derive` notice (`missing`, one per part), never an error, and only when the sheet already has some of the new parts.
`Derive` applies it: the tools become proficiencies, the languages enter `Languages`, the feature enters `Features` (`background-feature:custom`, origin `background:custom`), and the equipment comes out in `DerivedSheet.background_equipment_pt`.
Level-up changes none of it (`full.custom_background` is locked).

## The lasting effect of a table spell

`TableSpell.effect` (`TableSpellEffect`, with `TableEffectModifier`) goes through `tableSpellEffectOf` into the overlay.
Storage is the protojson of the entry, so there is no column and no migration.
An enum left unset reaches the rules empty, which they read as the default (`applies`, `visibility`, `resistance_kind`) or refuse (`kind`); a number this build does not know reaches them as `unknown` and is refused at its field.
The violation is `table_spell.effect.modifiers[1].value` and the like.
The spell editor of the web app builds its body from its own draft (`spell-draft.ts`), and the effect has two blocks that write it side by side.
"Deixa um efeito nos alvos" (`spell-effect-draft.ts`, `spell-effect.ts`) edits every modifier but the next-hit one: seven kinds of change, "Em quem fica", the conditions, the aura, who sees it and the label.
"Efeito no próximo golpe" (`hit-effect-draft.ts`) edits only the `weapon_hit_damage` modifier, and `spellEffect` writes it last, after the first block's modifiers, so the paths of that block's refusals keep their index.
With only the second block on, the stored effect's `applies`, audience and label go on carrying it; a modifier kind neither block offers goes back untouched.
The editor has no call to tag a draft, so it writes the card's labels itself (`rowTags`), kept equal to `rules.EffectTagsPT` by a table of cases on each side.
A zone is still kept as it was loaded; saving a spell in the editor replaces the body, so a zone written through the API stays only as the draft carried it.
`TableSpellEffect.aura` (`TableEffectAura`) maps through `auraAffectsOf`.
A player's copy of a spell drops `visibility` and `player_label_pt` of an owner-only effect (`forPlayer`, `effectKeepsItsSecrets`), and the aura's radius with it.
How the rules read the effect is in [Table content: rules](rules-table-content.md#the-lasting-effect-of-a-spell).

**A feature's aura.**
`TableEffect.aura` (`TableSpellEffect`) maps through `tableSpellEffectOf`, and `value_ability` and `value_min` of `TableEffectModifier` map with the other fields.
`featureAurasOf` (`combattraits.go`) turns `Derived.Auras` into `link.Traits.Auras`, resolving each ability bonus against the sheet (`max(modifier, value_min)`) and clearing the ability fields.
In the web app the effect picker draws the type `aura` with the spell's block (`shared/spell-effect/`, `feature` input: no switch, no duration, the whole combat), hosted by `app-effect-aura`.
The roll row there has a third way to say the number, "De uma habilidade", with the ability and "No mínimo".
The fields are known inputs for the refusals through `effectInputPaths` (`effect-input-paths.ts`), shared by every editor that lists effects, and the editors pass their `catalog` to name skills, damage types and abilities.
The card's text is `auraItem` in `content-read-rolls.ts`.

**The next-hit effect.**
`TableEffectModifierKind` has `WEAPON_HIT_DAMAGE` (20), and `TableEffectModifier` the fields only it reads (`rules/v1/table_content.proto`):

| Field | Number | Type |
| --- | --- | --- |
| `trigger` | 11 | `TableHitTrigger`: `NEXT_HIT` (1), `NEXT_MELEE_HIT` (2), `EACH_HIT` (3), `MARKED_TARGET` (4) |
| `dice` | 12 | `int32`, 1 to 10 |
| `per_slot_dice` | 13 | `int32`, 0 to 5 |
| `max_dice` | 14 | `int32`, `dice` to 20, 0 is no limit |
| `damage_type` | 15 | SRD damage type key, at most 64 characters |
| `save` | 16 | ability key, at most 16 characters |
| `on_fail_conditions` | 17 | SRD condition keys, each at most 64 characters |
| `condition_duration` | 18 | `TableHitConditionDuration`: `UNTIL_END_OF_NEXT_TURN` (1), `UNTIL_SPELL_END` (2); unset reads as the first |

The die is the existing `die` (field 2).
`tableSpellEffectOf` copies them through `hitTriggerOf` and `hitConditionDurationOf`; an enum left unset reaches the rules empty (refused at `trigger`) and a number this build does not know reaches them as `unknown`.
The API accepts the modifier on a spell that lasts and refuses it at its field, as the rules do: `table_spell.effect.modifiers[0].trigger` for a concentration trigger on a spell without concentration, or `marked_target` on a spell whose target is not One creature; `...dice`, `...max_dice` and the other numbers out of range; `...on_fail_conditions` for a save with no condition or a condition with no save; `...condition_duration` for `spell_end` on a spell that neither lasts a set time, lasts until dispelled nor asks for concentration; `...modifiers[i].kind` for a second `weapon_hit_damage` on the same spell.
A spell with this modifier works only in a combat: casting it outside one is refused before a slot is spent.
`TestTableSpellWeaponHitDamageIsReadFromTheMessage`, `TestTableSpellHitTriggersAreReadOneByOne` and `TestTableSpellHitConditionDurationsAreReadOneByOne` pin the copy.
The spell page of a player reads it through `readHit` (`core/content/hit-effect-read.ts`): the section "Efeito no próximo golpe" with the damage, the save and who sees it.
The preview line says it as "Em cada golpe com arma no alvo marcado: +1d6 do tipo da arma, enquanto durar.".
The play side is in [Lasting effects](play/lasting-effects.md#the-damage-a-spell-adds-to-the-casters-hits).
