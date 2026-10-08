# Rules review R4: combat

- **Scope:** `play` (`combat*.go`, `vitals.go`, `traps_damage.go`), `rules/combat`, `characters` (`combatturn.go`, `vitals.go`, `combatmonsters.go`), the class-feature data that combat reads (`rules/attacks.go`, `rules/actions.go`, `srd51/effects/*.json`), and the web combat screens (`core/combat`, `pages/live-session/combat`).
- **Commit reviewed:** first `ae40f74`, then re-checked after merging `origin/main` at `9ed736b2` (about 35 pull requests later). Everything below is stated against the merged code.
- **SRD:** 5e-database commit `a8abc93b235c158bb8cbf042e54425b9c2fd79b8` (SRD 5.1, `2014/en`).
- **Time spent:** about 2 hours of wall time.
- **Model:** the reviewer ran on `claude-sonnet-5-5`; each candidate was verified by a fresh `sonnet` subagent that received only the description and the SRD reference.
- **Not covered:** the web was read for rules arithmetic only. `core/combat` and the combat sheets compute almost nothing themselves: `hitPointsAfter`, the heal cap, the Shield `+5` text and the critical hints all match the server. No web finding.

## Findings

| id | severity | status | file:line | defect | failure scenario | evidence |
| --- | --- | --- | --- | --- | --- | --- |
| R4-1 | high | confirmed | `rules/srd51/effects/fighter.json:42` | Only `feature:extra-attack-1` has an `extra_attack` effect; `extra-attack-2` (level 11) and `extra-attack-3` (level 20) have none. | Fighter 11 to 19: the app gives 2 attacks per Attack action, the SRD 5.1 Fighter table (Extra Attack) says 3. Fighter 20: 2, the SRD says 4. Fighter 11 / Barbarian 5 gives 2 (should be 3). Turn options, `AttacksLeft` and the "ataques restantes" counters are all wrong for those levels. | `TestRulesReviewCombat_FighterExtraAttack` (`rules`): `fighter 11: AttacksPerAction = 2, SRD says 3`, same for 12 to 19; `fighter 20: ... = 2, SRD says 4` |
| R4-2 | high | confirmed | `rules/attacks.go:28` | `Derived.Attacks` is built only from chosen weapons and damaging cantrips; the unarmed strike never exists. | A Monk with no weapon has zero attacks. By the SRD (Melee Attacks; Monk Martial Arts) it should have an unarmed strike: Monk 1, DEX 16, +5 to hit and 1d4+3 (d6 at 5, d8 at 11, d10 at 17, DEX or STR). Any other class with STR 10 should have +2 to hit and 1 bludgeoning damage. Flurry of Blows and Martial Arts have nothing to hit with. | `TestRulesReviewCombat_UnarmedStrike` (`rules`): `monk1 dex16: no unarmed strike attack line (attacks=0)`, `wizard1 str10: no unarmed strike attack line (attacks=0)` |
| R4-3 | medium | confirmed | `characters/combatturn.go:86` | `basicDerived` sets `Melee` only when the attack's range is 5 ft or less, but `basicAttackOf` stores a reach of 10 ft or more as that range. | Hill Giant (greatclub, reach 10 ft) put in a combat: `Melee=false`. It is treated as a ranged attack and `meleeReachOf` returns 0, so it never makes an opportunity attack (SRD Opportunity Attacks: reach is the creature's own, 10 ft here). 79 SRD creatures are affected (all giants, T. rex, mammoth, hydra, kraken, roc, tarrasque, purple worm, oni, nagas, and others). The architecture doc says the reactor's reach is the sheet's "greatest melee reach", so this is a defect, not a decision. | `TestRulesReviewCombat_ReachTenIsMelee` (`characters`): `hill giant attack "Clava grande" RangeFt=10 Melee=false`; `79 creatures lose their long-reach melee attack in basicDerived` |
| R4-4 | medium | confirmed | `play/combat_spells.go:389`, `rules/combat/turn.go` (`spellOption`) | Each spell is checked only against its own action economy; nothing records that a bonus-action spell was cast. | Cleric 3 casts Healing Word (bonus action) and then Cure Wounds (1 action) in the same turn: accepted. SRD 5.1, Casting Time, Bonus Action: no other spell that turn except a 1-action cantrip. The reverse order (levelled action spell, then bonus-action spell) is allowed too (read from the code, not tested). | `TestRulesReviewCombat_BonusActionSpellLimit` (`play`): `Cure Wounds after Healing Word in the same turn was accepted; SRD allows only a 1-action cantrip` |
| R4-5 | low | fixed on main | `play/combat_actions.go:1595` | Action Surge only checks the resource count; no per-turn record. | Fighter 17 (2 uses) uses Action Surge twice in one turn and gets three actions. SRD Fighter, Action Surge: from level 17 two uses before a rest, but only one on the same turn. A later turn is fine (the test's control line passes). | `TestRulesReviewCombat_ActionSurgeOncePerTurn` (`play`) failed on `ae40f74` (`second Action Surge on the SAME turn was accepted`); after the merge it passes, because `main` now has an `ActionSurged` per-turn flag (`ALREADY_USED_THIS_TURN`). Kept as a regression test. |
| R4-6 | low | confirmed | `rules/attacks.go:52,64` | `biggerDie` upgrades only `Damage`; the versatile line is copied from the weapon. | Monk 17, DEX 20, quarterstaff: one-handed `1d10+5` (right), two-handed `1d8+5`. The Martial Arts die (SRD Monk) replaces the damage in either grip, so it should be `1d10+5`. Level 11 is fine. | `TestRulesReviewCombat_MonkVersatileDie` (`rules`): `monk 17 VersatileDamage = "1d8+5", want "1d10+5"` |
| R4-7 | medium | policy | `play/traps_damage.go:26` (comment only); `play/combat_actions.go` `landDamage`, `ApplyPendingDamage` | No damage type is ever compared with resistances, vulnerabilities or immunities. Only the code comment says "resistances are not modeled"; no RN rule, ADR or architecture decision does. | A fire hit on a fire-resistant creature or a Rage barbarian (bludgeoning, piercing, slashing resistance) lands in full: SRD Damage Resistance and Vulnerability says halve (round down) or double, after bonuses. The master can type a different amount in "Aplicar", and the monster stat block holds the data. Rage only appends a note. | grep: no `resist` in `play`/`rules/combat`; `ApplyDamage` takes no type |
| R4-8 | medium | policy | `characters/combatspells.go:147` | A basic-sheet NPC, including every monster from "Pôr no combate", has `SavingThrows` empty, so a spell save is `d20 + 0`. | Goblin (DEX 14, +2) vs Burning Hands DC 13: the app rolls d20+0. A creature with a listed save (for example a dragon's +7) loses it too. Trap saves do read the stat block (`CreatureSave`), so the two paths disagree. The doc describes "d20 + 0, log says so", but only as a description. | read: `CombatSave` returns `Known:false`; `architecture.md` Casting table |
| R4-9 | medium | policy | `play/combat_actions.go` `RollAttack` | There is no bonus-action attack path. A player cannot roll the off-hand attack of Two-Weapon Fighting or the unarmed strikes of Flurry of Blows or Martial Arts. | Rogue with two shortswords: after the first attack, `RollAttack` answers `ACTION_USED`. SRD Two-Weapon Fighting: a bonus-action attack with the other light weapon (no ability modifier on its damage). The docs say Flurry of Blows "spends the economy and logs"; two-weapon fighting is not documented. | read: `AttacksLeft` is the only gate for non-master |
| R4-10 | low | policy | `play/combat_actions.go:1608`, `playdb` `dashed` | Dash is a boolean, and Cunning Action / Step of the Wind / Patient Defense only log. | Dash twice in a turn (Action Surge) doubles the speed once (SRD: triple). Cunning Action Dash gives no movement and Cunning Action Disengage sets no `disengaged`, so the rogue still provokes. Documented as "spends the economy and logs". | read |
| R4-11 | low | policy | `rules/combat/rolls.go:28` | `ResolveAttack` knows only a natural 20. Improved Critical (19 to 20, Champion) and Superior Critical (18 to 20) are notes; Brutal Critical dice are not added. | Champion 3 rolls a 19: a plain hit, SRD says a critical hit. The Barbarian's extra critical dice are left to the table (Sneak Attack and Divine Smite are stated as the table's). | read |

## Re-check after merging main

- Changed: R4-5 is fixed on `main` (its test now passes).
- Unchanged, tests still fail on the merged branch: R4-1, R4-2, R4-3 (the earlier reach-weapon fix #219 did not touch `basicDerived`), R4-4, R4-6.
- Unchanged by reading the merged code: R4-7 (the comment "resistances are not modeled" is still the only trace), R4-8, R4-9, R4-10, R4-11. The documented decisions below still hold.

## Documented decisions (not findings)

- Advantage and disadvantage ("up to the table", architecture.md, The two-step attack).
- Every condition effect, exhaustion levels and the automatic critical within 5 ft (RN-22: labels only). Movement is the exception: `noSpeed` zeroes the speed for the grappled, restrained, paralyzed, petrified, stunned and unconscious.
- Massive damage and instant death ("stays the master's", architecture.md, Pending damage).
- Initiative order, ties and the joint turn (RN-19), the critical options (RN-24), the table's death-save visibility.
- Sneak Attack, Divine Smite, Rage damage and resistance, Uncanny Dodge, Evasion (text only; documented as "logs").
- Surprise is not built and has no doc line either. I did not turn it into a finding because there is no wrong number.

## Other areas, one line each

- **Rests:** `bardic_inspiration` keeps `recharge: long_rest` at level 5 and up, and Font of Inspiration is only a note (`effects/bard.json`).
- **Sheet:** there is no effect for Aura of Protection (paladin 6), so the paladin's saves lack the CHA bonus.
- **Sheet:** Indomitable (fighter 9/13/17, 1/2/3 uses) has no resource, although the class table has the numbers.
- **Web text:** the death-saves list says damage while down "conta como uma falha" and omits that a critical hit counts two (wording only, not reported).

## Fix direction (confirmed findings)

**R4-1.** Root cause: `fighter.json` maps only `extra-attack-1`. Fix in `effects/` (handwritten data): add `feature:extra-attack-2` with `count: 3` and `feature:extra-attack-3` with `count: 4`, and bump `effects/revision.json` as `CONTRIBUTING.md` says. The same mistake elsewhere: none (Barbarian, Monk, Paladin and Ranger stop at 2); `resourcesAndActions` already takes the best count, so multiclass is right once the data is. Docs: none. Risk: golden snapshots of fighter sheets and the turn-options tests; `TestRulesReviewCombat_FighterExtraAttack` guards it.

**R4-2.** Root cause: attacks come only from `Build.Weapons`. Fix in code (`rules/attacks.go`): always add an `attack:unarmed-strike` line (STR, proficient, flat damage `1 + STR mod`, bludgeoning, melee). With the `monk.martial_arts` handler, use `martialDie`, DEX or STR, and take the better one. `names_pt.json` already has `attack:unarmed-strike`. The same mistake elsewhere: `Derived.Attacks` is the single source, so `CombatSheet` and the sheet screen follow. Docs: architecture.md (sheet attacks). Risk: Pensantus golden file, attack-count assertions in tests; it must not change the Extra Attack counting.

**R4-3.** Root cause: reach is stored in the `RangeFt` field, which `basicDerived` also reads as a range. Fix in code: in `basicDerived` set `Melee` for an attack whose source creature action is melee (store a `melee`/reach flag on the `BasicAttack`, or derive from the monster key's `MonsterDerived` attack). The same mistake elsewhere: `npcfromcreature.go` `basicAttackOf` (the comment "reach 0 at normal reach"), and `reachFt` in `combat_actions.go` for the target list (works today only because it takes the maximum). Docs: architecture.md (Opportunity attacks). Risk: opportunity-attack tests and the NPC sheet round trip.

**R4-4.** Root cause: no per-turn spell record. Fix in code: store `bonus_spell_cast`/`spell_cast` on the combatant, reset it in `ResetCombatantTurn`, and make `CastSpell` and `combat.Options` refuse a levelled spell after a bonus-action spell (and a bonus-action spell after a levelled one), allowing a 1-action cantrip. The same mistake elsewhere: reactions (Shield) are exempt by the SRD text. Docs: architecture.md Casting. Risk: undo restores the flag (`combat_undo.go`); the Sacred Flame cantrip case should be covered by a new test.

**R4-5 (already fixed on main; no action).** Root cause was no per-turn flag. Fix in code: a combatant flag `surged` reset in `ResetCombatantTurn` and restored by undo; refuse the second use with `ACTION_USED`-style reason. Docs: architecture.md (feature actions). Test: `TestRulesReviewCombat_ActionSurgeOncePerTurn`; `TestSecondWindAndActionSurge` guards the single-use case.

**R4-6.** Root cause: `biggerDie` is applied to `Damage` only. Fix in code at `rules/attacks.go:64`: apply `biggerDie` to `w.TwoHandedDamage` too (only when `monkWeapon`). The same mistake elsewhere: `DamageDice`/`VersatileDice` are re-parsed from the text, so they follow. Docs: none. Risk: monk golden files.

## Checklist

| rule | status |
| --- | --- |
| Initiative d20 + DEX (+ Jack of All Trades), order by total, ties (RN-19) | covered by `TestRN19_OrderByInitiativeThenBonusThenTheMastersPlaces`, `TestMR013_*` |
| Attack roll = d20 + ability + proficiency (when proficient) | covered by `rules` derive tests; checked by probe (fighter, rogue, wizard, monk) |
| Advantage / disadvantage | not built (doc) |
| Natural 20 hits and is a critical; natural 1 misses | covered by `rules/combat` `ResolveAttack` tests |
| Critical: dice doubled, modifier once; table option | covered by `TestRN24_TheCriticalFollowsTheTablesRule` |
| Improved Critical, Brutal Critical | finding R4-11 (policy) |
| Resistance, vulnerability, immunity | finding R4-7 (policy) |
| Temporary HP absorb first, replace not stack, not healed | covered by `rules/combat` tests, `vitals_test.go` |
| Healing never above maximum | covered by `ApplyHeal` tests |
| 0 HP, death saves DC 10, 3 and 3, nat 20 = 1 HP, nat 1 = 2 failures | covered by `rules/combat` `DeathSave` tests and the RN-03 play tests |
| Damage at 0 HP = 1 failure, 2 on a critical; stable resets | covered by `failuresWhileDown` play tests |
| Massive damage | not built (doc: the master decides) |
| Spell/trap save, half on success rounded down | covered by `TestRN22`/cast tests and `HalfDamage`; monster save bonus: finding R4-8 (policy) |
| Cover +2 / +5 to AC and DEX saves, total cover | covered by cover tests |
| Conditions: effects on attacks, saves, speed, actions | not built (doc, RN-22); speed 0 is enforced |
| Exhaustion levels | not built (doc, RN-22) |
| One action, bonus action, reaction per turn | covered by `combat_actions_test.go`; bonus-action spell limit: finding R4-4 |
| Dash, Disengage, Dodge, Help, Ready | Dash and Disengage enforced; double Dash and Cunning Action: R4-10 (policy); Dodge, Help, Ready log only (doc) |
| Extra Attack | finding R4-1 |
| Action Surge | finding R4-5 |
| Second Wind (1d10 + fighter level) | covered by `TestSecondWindAndActionSurge` |
| Sneak Attack, Divine Smite, Rage effects, Uncanny Dodge, Evasion | not built (doc) |
| Martial Arts | findings R4-2, R4-6 |
| Cunning Action | R4-10 (policy) |
| Two-weapon fighting | finding R4-9 (policy) |
| Creature attacks and Multiattack | multiattack count covered by `creatures_multiattack_test.go` plus `corrections.json`; reach: finding R4-3 |
| Opportunity attacks | covered by `combat_opportunity_test.go`; reach 10: R4-3 |
| Shield +5 AC until next turn | covered by reaction tests |
| Web: combat arithmetic | checked, no finding |
