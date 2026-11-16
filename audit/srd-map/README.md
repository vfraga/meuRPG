# SRD map: the party's slice

This is a read-only audit of what the app does with each SRD rule the party uses, judged on `origin/main` 6f71eeda4.
There is one row per entry in `features.json`, `traits.json`, `spells.json` and `rules.json`, and the verdicts are the brief's.
`batches/` holds each investigator batch's raw rows and the briefs they worked from.

## Totals

| Part | Rows | runs | runs-part | by-hand-said | by-hand-silent | wrong | by-design | step | unsettled |
|---|---|---|---|---|---|---|---|---|---|
| Features: wizard, warlock, druid, levels 1-12, SRD subclasses | 105 | 39 | 4 | 23 | 14 | 3 | 3 | 19 | 0 |
| Race traits | 38 | 15 | 14 | 2 | 3 | 3 | 1 | 0 | 0 |
| Spells, levels 0-6 (0/1/2/3/4/5/6: 20/38/44/36/28/32/26) | 224 | 27 | 3 | 122 | 43 | 24 | 5 | 0 | 0 |
| General rules: combat 80, conditions 23, resting 7, spellcasting 27, vision 8, environment 10 | 155 | 87 | 26 | 5 | 23 | 7 | 7 | 0 | 0 |

The features include `rule:<class>-*` rows: the parts of Spellcasting, Pact Magic and Wild Shape that the app could get wrong.

## wrong

Except where a line says otherwise, the master can correct each one by hand (undo, the HP correction, a typed condition, an extra d20).
- Warlock:
  - `eldritch-invocation-eldritch-spear`: Eldritch Blast keeps 120 ft, and a player is refused past it.
  - `pact-of-the-chain`: the familiar may make any reaction attack (opportunity attacks included); the master cannot forbid it.
  - `mystic-arcanum-6th-level`: the pick cannot be cast (no 6th-level slot), and no once-a-day use exists.
- Traits:
  - `fey-ancestry`: Sleep still puts the creature to sleep.
  - `halfling-nimbleness`: the grid refuses a move through a hostile creature one size larger. It is not settled whether the master can place the token past it.
  - `high-elf-cantrip`: the cantrip is cast with the first caster class's ability, not Intelligence. With no caster class it has no bonus or DC.
- Spells by level:
  - 0: `acid-splash` (one target; the text allows two), `produce-flame` (range Self read as 0 ft).
  - 1: `cure-wounds`, `healing-word` (heal undead and constructs); `sleep`, `color-spray` (no type exclusions; the condition never ends); `charm-person` (no advantage when the party fights the target).
  - 2: `flame-blade` (attacks the caster), `enlarge-reduce` (willing targets save).
  - 3: `vampiric-touch` (the only target is the caster), `bestow-curse` and `major-image` (concentration kept at slots that drop it), `magic-circle` (1 hour at every slot), `water-breathing` (one target; the text allows ten).
  - 4: `blight` (no undead or plant rule), `confusion` (the radius never grows), `dominate-beast` (no advantage when fought), `faithful-hound` (bites at the cast), `phantasmal-killer` (full damage on the first failed save).
  - 5: `mass-cure-wounds` (heals everyone in the sphere, foes included), `dominate-person` (no advantage when fought).
  - 6: `chain-lightning` (1 target plus 1 per slot), `disintegrate` (any number of targets), `wall-of-ice` (damages everyone in a disc).

  A player is refused a legal target with Acid Splash, Water Breathing, Chain Lightning, and Produce Flame or Vampiric Touch on a map: only the master can cast these as the text says.
- Rules:
  - `combat-creature-size-space`: every combatant takes one square, so a Large creature's reach, cover and opportunity attacks are wrong. This cannot be corrected by hand.
  - `combat-death-save-damage`: damage at 0 HP ignores temporary hit points. The failure itself cannot be edited.
  - `conditions-frightened`: disadvantage applies with the source out of sight, and moving closer is never refused.
  - `resting-long-rest-needs-hit-point`: a long rest at 0 HP still takes off a level of exhaustion.
  - `spellcasting-concentration-damage-save`: damage absorbed by temporary HP opens no save and lowers the DC. The master cannot ask the save.
  - `spellcasting-combining-effects`: a later, weaker casting of a spell replaces a stronger one.
  - `environment-falling-prone`: a pit trap leaves the creature prone after Feather Fall or Slow Fall cancel the damage.

## by-hand-silent

The app applies nothing, or only a part, and no screen text names the rest.
- Wizard: evocation-savant, potent-cantrip (a saved target takes nothing), empowered-evocation, the spellbook's copying, the arcane focus.
- Warlock: pact-of-the-blade, pact-of-the-tome (the book itself), dark-ones-own-luck (the d10 is never added), fiendish-resilience.
- Druid: druidic, druid-lands-stride (the `ignore_difficult_terrain` flag already exists), natures-ward, the druidic focus, Wild Shape equipment.
- Traits: infernal-legacy (only Hellish Rebuke is wired, and its use is shared with Darkness), tinker, trance.
- Spells:
  - 0: chill-touch, fire-bolt, ray-of-frost, shocking-grasp (the riders).
  - 1: animal-friendship, find-familiar.
  - 2: blindness-deafness, enthrall, gust-of-wind, heat-metal, moonbeam, ray-of-enfeeblement, shatter, suggestion, web, and pass-without-trace (the caster only, though RN-22 names it as applied).
  - 3: animate-dead, call-lightning, fear, hypnotic-pattern, slow, wind-wall.
  - 4: banishment, black-tentacles, control-water, ice-storm, polymorph, resilient-sphere.
  - 5: arcane-hand, cloudkill, contagion, hold-monster (no paralyzed, unlike Hold Person), modify-memory.
  - 6: eyebite, flesh-to-stone, freezing-sphere, heal, mass-suggestion, sunbeam, wall-of-thorns, contingency, find-the-path, magic-jar.
- Rules:
  - Combat: search, free-object-interaction, prone-drop, unseen-attacker, instant-death, stabilize-medicine, stable-regain, knock-out, and the three underwater rows.
  - Resting: interrupted.
  - Spellcasting: the focus, concentration-environment.
  - Vision: dim-light, heavily-obscured-areas, truesight.
  - Environment: falling-damage (only the pit presets), suffocating (two rows), food-water, jump-obstacle, travel-pace.

## by-design

- RN-01: the daily change of prepared spells (wizard, druid) and the warlock's spell swap.
- RN-34: naturally-stealthy.
- RN-22:
  - guidance and resistance: the d4 goes to the first roll that fits.
  - insect-plague, and charmed, deafened and exhaustion 6.
- RN-37: darkness, spike-growth.
- RN-19: initiative-ties, initiative-group-roll.
- RN-31: ready-unheldable-actions.
- RN-03: death-save-three-failures.

## Not claimed anywhere

Neither `docs/product/rules.md` nor `docs/architecture/rules-combat.md` mentions these SRD rules. Each is judged in `rules.json` with `"claimed": false`, to be checked against the SRD text:
- Combat: dropping prone at will; instant death from massive damage (only a code comment cites RN-03); stabilizing with a Medicine check; a stable creature regaining 1 HP after 1d4 hours; knocking a creature out; underwater melee, ranged attacks and fire resistance.
- Rest: a rest interrupted by strenuous activity.
- Spellcasting: somatic components and a free hand; the spellcasting focus; concentration broken by the environment (a wave, a ship's deck).
- Environment: holding breath and suffocating; food and water; jumping over an obstacle; travel pace.

## A floor test for levels 6 to 12

- Of the 30 feature rows at levels 6-12, 22 have no effect row, and circle-spells-3/4 are bare notes.
- A test like `TestEffectsCoverLevels1To5` must accept:
  - **Honest steps, handled by the level-up:**
    - the ability score improvements at 8 and 12 (`isASILevel` reads the key)
    - arcane-tradition, otherworldly-patron and druid-circle improvements at 6 and 10 (markers; the subclass rows bring the feature)
  - **Handled in code by key, with a bare note or no row:** circle-spells-3/4 (`collectAlwaysPrepared`), and mystic-arcanum (the level-up pick, though it is `wrong`).
  - **by-hand-said:** the eight invocations of levels 7-12 (ascendant-step, bewitching-whispers, dreadful-word, lifedrinker, minions-of-chaos, otherworldly-leap, sculptor-of-flesh, whispers-of-the-grave). They have no row, and only their pick-list summary tells the table.
- It should refuse, until fixed: potent-cantrip, empowered-evocation, dark-ones-own-luck (a counter with no d10), fiendish-resilience, druid-lands-stride, natures-ward.

## Unsettled

No row is `unsettled`. One part of a `wrong` row is open: whether the master can drag a halfling's token past a hostile creature.

## Coordinator rulings and checks

- A spell whose cast shows only the generic line ("A magia foi conjurada: o mestre resolve o efeito.", or outside combat "Conjuração registrada. O efeito é narrado pelo mestre.") is `by-hand-said`.
  - Most of the 122 such spells rest on that line alone.
  - Among them are buffs the table must track from memory: invisibility, blur, mirror-image, barkskin, protection-from-evil-and-good, magic-weapon, spider-climb, and the two conjure spells, which put no creature on the map.
  - The level 0 and level 2 batches had counted that line as silent; 24 rows moved under this ruling, as a convention, not a re-judged fact.
- An invocation whose only screen text is its one-line summary in the pick list is `by-hand-said`.
- A rule applied only in part, with no text naming the rest, is `by-hand-silent`, even when the part is the bulk of it (Moonbeam, Web).
- An exception the app ignores, refusing a legal action or applying the general rule against the text, is `wrong`. A missing advantage on a save is `wrong` too.
- Each batch was re-judged on at least one row in ten across verdicts, and on every `wrong` row.
  - Verdicts changed: wizard 1/17, warlock 1/21, traits B 1/18, spells 0 2/20, 1 1/38, 2 1/44, 5 1/32. No batch passed one in ten, so none was redone.
  - `"checked": "test"` rows were settled by a throwaway pure test of the `rules` packages, deleted after the run.

## Doc and text findings (not verdicts)

- RN-01 says the master adds Mystic Arcanum in the editor; the level-up asks for it.
- RN-22 says Flaming Sphere and Sleet Storm roll at the cast; neither has a save in the data.
- RN-37 says a cloud or darkness never changes an attack roll; the code gives the unseen-target disadvantage when fog hides an NPC (which the SRD supports).
- The Wild Shape sheet says the app does not count the form's time; the backend ends the form by itself.
- The Natural Recovery note reads "1 níveis" at druid level 2.
- `corrections.json` takes Divination and Create Food and Water off the druid list; `data/spells.json` still lists them.
- Thirteen spells that affect no creature ask a player to target a creature, because they have no `kind: none` row in `spell_targets.json`.
- The server sends `break_free_ability`, and no web code reads it.
- The Draconic Ancestry table exists only in `effects/choices.json`, so there was no SRD copy to check it against.
