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

## Part 2: spells of levels 0 to 6 on the wizard, warlock or druid list

### Totals

| Level | Rows | runs | runs-part | by-hand-said | by-hand-silent | wrong | by-design |
|---|---|---|---|---|---|---|---|
| 0 | 20 | 2 | 0 | 10 | 4 | 2 | 2 |
| 1 | 38 | 13 | 1 | 17 | 2 | 5 | 0 |
| 2 | 44 | 4 | 1 | 25 | 10 | 2 | 2 |
| 3 | 36 | 5 | 1 | 19 | 6 | 5 | 0 |
| 4 | 28 | 1 | 0 | 16 | 6 | 5 | 0 |
| 5 | 32 | 1 | 0 | 23 | 5 | 2 | 1 |
| 6 | 26 | 1 | 0 | 12 | 10 | 3 | 0 |
| All | 224 | 27 | 3 | 122 | 43 | 24 | 5 |

No row is `step` or `unsettled`.

### wrong

- 0: `acid-splash` (one target, the text allows two), `produce-flame` (range Self read as 0 ft: melee on a map, no ranged disadvantage).
- 1: `cure-wounds`, `healing-word` (heal undead and constructs), `sleep` (undead and charm-immune not skipped; unconscious never ends), `color-spray` (blinded never ends; creatures that cannot see not skipped), `charm-person` (save never at advantage when the party fights the target).
- 2: `flame-blade` (rolls a melee spell attack and fire damage against the caster), `enlarge-reduce` (a save for willing targets).
- 3: `vampiric-touch` (range Self: the only target is the caster, the attack is rolled against them), `bestow-curse` and `major-image` (concentration kept at the slot levels that drop it), `magic-circle` (duration fixed at 1 hour at every slot), `water-breathing` (one target, the text allows ten).
- 4: `blight` (no undead/construct immunity, no plant rule), `confusion` (radius never grows with the slot), `dominate-beast` (no advantage when fought), `faithful-hound` (bites at the cast instead of later turns), `phantasmal-killer` (full damage on the first failed save instead of frightened).
- 5: `mass-cure-wounds` (heals every creature in the sphere, foes included; no pick of six), `dominate-person` (no advantage when fought).
- 6: `chain-lightning` (one target plus one per slot, instead of four), `disintegrate` (an area: any number of targets), `wall-of-ice` (damages everyone in a disc, not the creatures the wall cuts).

The master can correct each by hand (undo, the HP correction, a typed condition, an extra d20), except where a player is refused a legal target (Acid Splash, Water Breathing, Chain Lightning, Produce Flame and Vampiric Touch on a map): there only the master can cast it as the text says.

### by-hand-silent

The app rolls a part (a save, an attack, damage, a zone) and nothing on screen names the rest:
- 0: chill-touch, fire-bolt, ray-of-frost, shocking-grasp (the riders).
- 1: animal-friendship (no charmed), find-familiar (touch spells through the familiar).
- 2: blindness-deafness, enthrall, gust-of-wind, heat-metal, moonbeam (shapechangers, light), pass-without-trace (the caster only; RN-22 names it as applied), ray-of-enfeeblement, shatter, suggestion, web (the Strength check: the server sends `break_free_ability`, no web code reads it).
- 3: animate-dead, call-lightning, fear, hypnotic-pattern, slow, wind-wall.
- 4: banishment, black-tentacles, control-water, ice-storm, polymorph, resilient-sphere.
- 5: arcane-hand, cloudkill, contagion, hold-monster (no paralyzed, unlike Hold Person), modify-memory.
- 6: eyebite, flesh-to-stone, freezing-sphere, heal, mass-suggestion, sunbeam, wall-of-thorns; contingency, find-the-path and magic-jar (outside combat only "<name>: conjuração registrada.").

### by-design

- `guidance`, `resistance`: the d4 is spent by the first roll it fits (RN-22); the SRD lets the target choose.
- `darkness`, `spike-growth`: the zones run; the object form, the light dispel and the Perception check are the master's (RN-37).
- `insect-plague`: the cast rolls; the later saves are the master's (RN-22).

### Coordinator rulings

- A spell whose combat cast shows only the generic line "A magia foi conjurada: o mestre resolve o efeito." (`isPlainCast`), or outside combat "Conjuração registrada. O efeito é narrado pelo mestre.", is `by-hand-said`.
  The line tells the table the app applied nothing and the master resolves the spell's text.
  122 rows are `by-hand-said`, almost all on this line alone; among them are spells the table must track by memory in a fight: invisibility, blur, mirror-image, barkskin, protection-from-evil-and-good, magic-weapon, spider-climb, conjure-minor-elementals and conjure-woodland-beings (no creatures appear).
- A spell where the app rolls a part and no text names the rest is `by-hand-silent`, even when the part is the bulk of it (Moonbeam, Web).
- The level 2 batch had counted the generic line as silent; its 14 such rows were moved to `by-hand-said` under the first ruling. That is a convention, not a re-judged fact, so the batch was not redone.
- Missing advantage on a save the text gives at advantage is `wrong` (Charm Person, Dominate Beast, Dominate Person).

### Other findings (not verdicts)

- RN-22 says Flaming Sphere and Sleet Storm roll their saves at the cast; neither has a save in the data, so nothing is rolled (both rows `by-hand-said`).
- `corrections.json` removes Divination and Create Food and Water from the druid list; `data/spells.json` still lists the druid.
- Spells that affect no creature (Animate Objects, Passwall, Wall of Force, Wall of Stone, Fire Shield, Stone Shape, Secret Chest, Dimension Door, Giant Insect, Mage Hand, Mending, Prestidigitation, Shillelagh) ask a player for a creature target, since they have no `kind: none` row in `spell_targets.json`.
- Seeming takes one target for a player; the text says any number.
- Spells with a casting time of a minute or more are refused in combat and only recorded outside it (RN-32).
