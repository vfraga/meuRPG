# SRD map: the party's slice

Read-only audit of what the app does with each SRD rule the party uses, judged on `origin/main` 6f71eeda4.
One row per entry in `features.json`, `traits.json`, `spells.json` and `rules.json`; the verdicts are the brief's.
`batches/` holds the raw rows of each investigator batch and the brief they worked from.

## Part 1: features and traits

### Totals

| Part | Rows | runs | runs-part | by-hand-said | by-hand-silent | wrong | by-design | step | unsettled |
|---|---|---|---|---|---|---|---|---|---|
| Features (wizard, warlock, druid, levels 1-12, with SRD subclasses) | 105 | 39 | 4 | 23 | 14 | 3 | 3 | 19 | 0 |
| Race traits | 38 | 15 | 14 | 2 | 3 | 3 | 1 | 0 | 0 |

Features include `rule:<class>-*` rows for the parts of a class's Spellcasting, Pact Magic or Wild Shape text the app could get wrong.

### wrong

Warlock:
- `feature:eldritch-invocation-eldritch-spear` (2): Eldritch Blast keeps range 120 ft; a player's target past it is refused on a map. The master is not held to reach.
- `feature:pact-of-the-chain` (3): the familiar may attack with any reaction, an opportunity attack included; the SRD ties it to the warlock's Attack action and a forgone attack. The master can undo it, not forbid it.
- `feature:mystic-arcanum-6th-level` (11): the pick is recorded but the cast screen disables it (no 6th-level slot); no once-per-long-rest use exists. The master can only narrate it.

Traits:
- `trait:fey-ancestry`: the immunity to magical sleep is not applied; Sleep puts an elf or half-elf to sleep. The master can undo the cast or remove the condition.
- `trait:halfling-nimbleness`: the grid refuses a halfling's move through a hostile creature one size larger. Whether the master can place the token past it is not settled.
- `trait:high-elf-cantrip`: the cantrip is cast with the first caster class's ability, never Intelligence; a high elf with no caster class has no attack bonus or DC for it.

### by-hand-silent

Wizard:
- `feature:evocation-savant` (2): no copy cost or time exists.
- `feature:potent-cantrip` (6): a saved target of a damage cantrip takes nothing; half is never applied or reminded.
- `feature:empowered-evocation` (10): the Intelligence modifier is never added to evocation damage.
- `rule:wizard-spellbook`: two book spells per level are enforced; a found spell copied into the book, its cost and time, are not.
- `rule:wizard-spellcasting-focus`: components are never enforced, and nothing mentions the focus.

Warlock:
- `feature:pact-of-the-blade` (3): bare note; no pact weapon.
- `feature:pact-of-the-tome` (3): the three cantrips work; the book itself and its replacement are not tracked or reminded.
- `feature:dark-ones-own-luck` (fiend 6): a use counter only; the d10 is never rolled or added, and no text says to.
- `feature:fiendish-resilience` (fiend 10): no effect row; no damage type pick, no resistance.

Druid:
- `feature:druidic` (1): bare note; Druidic is not a language on the sheet.
- `feature:druid-lands-stride` (land 6): no row; difficult terrain still costs double. The flag `ignore_difficult_terrain` already exists.
- `feature:natures-ward` (land 10): no row; no poison immunity, no poisoned immunity, no reminder.
- `rule:druid-druidic-focus`: as the wizard's focus.
- `rule:druid-wild-shape-equipment`: the form always uses the beast's AC and attacks; merging or wearing gear is not modelled or reminded.

Traits:
- `trait:infernal-legacy`: only the Hellish Rebuke reaction is wired; Thaumaturgy and Darkness are typed by the player with no level gate, Darkness has no cast path, and its use is shared with Hellish Rebuke.
- `trait:tinker`: the tool proficiency is listed; the devices are neither modelled nor reminded.
- `trait:trance`: bare note.

### by-design

- `rule:wizard-preparing-spells`, `rule:druid-preparing-spells`: the count is exact; the daily change after a long rest is the master's in the editor (RN-01: a level-up adds prepared spells, none removed).
- `rule:warlock-spell-swap`: the swap of a known spell at a level-up is the master's in the editor (RN-01, RN-12).
- `trait:naturally-stealthy`: Hide is an action roll and the master decides where one can hide (RN-34).

### Coordinator rulings

- An invocation whose only screen text is its one-line summary in the pick list (`option_summaries` in `effects/choices.json`) is `by-hand-said`: the summary states the rule the table applies. 23 invocations are in this case.
- An exception the app does not apply, where the app then refuses a legal action or applies the general rule against the text, is `wrong`, not `by-hand-silent` (Halfling Nimbleness, Mystic Arcanum, Fey Ancestry).

### Other findings (not verdicts)

- `docs/product/rules.md` RN-01 says the master adds the Warlock's Mystic Arcanum in the editor; the level-up asks the pick (`levelup.go`), so the line is stale.
- The Wild Shape sheet says the app does not count the form's time (`web/src/app/shared/wild-shape/wild-shape-sheet.ts`); the backend ends the form after half the druid level in hours.
- The Natural Recovery note reads "1 níveis" at druid level 2.
- The Draconic Ancestry table is only in `effects/choices.json`; the SRD data has no copy to check it against. The ten colours were checked against the SRD table as the investigator knows it.
- The seven Circle of the Land spell lists were checked only against `data/subclasses.json`, which the engine reads faithfully.

### Checks

Each batch was re-judged by the coordinator on at least one row in ten, across verdicts, and on every `wrong` row.
Changes under that check: wizard 1 of 17, warlock 1 of 21, traits B 1 of 18; no batch passed one in ten, so none was redone.
Rows marked `"checked": "test"` were settled by a throwaway pure test of the `rules` package, deleted after the run.
