# Rules review R8: rests, resources, conditions and the table rules

- Scope: class and race resources, death saves, conditions, exhaustion, standard actions, RN-18, RN-24 (critical options, ability methods, level-up HP).
- Commit reviewed: `9ed736b` (origin/main). SRD 5.1 data commit: `a8abc93b235c158bb8cbf042e54425b9c2fd79b8`.
- Time spent: about 1 hour of session time.
- Model: Sonnet 5.5 (`claude-sonnet-5-5`); the verifying subagents were `sonnet` too.
- Big fact: **rests do not exist in the app yet** (`docs/product/rules.md` RN-02: "rests do not exist yet"), and conditions and exhaustion are labels with no effect (`docs/architecture.md`, "Conditions and concentration (RN-22)": "There is no effect at all"). These are documented, so they are `not built (doc)`, not findings. What a rest will restore is already in the data (the `recharge` of each resource), so a wrong recharge or a missing resource is a real defect.

| id | severity | status | file:line | defect | failure scenario | evidence |
| --- | --- | --- | --- | --- | --- | --- |
| R8-1 | high | confirmed | `backend/internal/rules/combat/vitals.go:99` (`tally`), `backend/internal/play/combat_death.go:137,291` | A character that becomes stable keeps its death-save failures. | Fighter at 0 HP with 2 failures rolls its third success: the app stores stable with 2 failures (SRD, Death Saving Throws: both counts reset to zero when you regain HP or become stable). A later hit at 0 HP then gives 3 failures, so the character is "dying"; the SRD gives 1 failure. | `TestRulesReviewRests_StableResetsTheCounts`: `failures after becoming stable = 2, want 0`; `failures after the first hit on a stable character = 3, want 1` |
| R8-2 | high | confirmed | `backend/internal/rules/srd51/effects/fighter.json` (no entry for `feature:indomitable-*`) | The Fighter's Indomitable has no resource. | Fighter 9 / 13 / 17: SRD Fighter table gives 1 / 2 / 3 uses, long rest. Derive lists only `second_wind` and `action_surge`; the sheet and the live session show no counter. | `TestRulesReviewRests_IndomitableUses`: `level 9: ... 1 use(s), but no resource has a feature:indomitable source` (also 13, 17, 20) |
| R8-3 | medium | confirmed | `backend/internal/rules/srd51/effects/bard.json:8` | Bardic Inspiration comes back only on a long rest at every level. | Bard 5+: Font of Inspiration returns the uses on a short or long rest (SRD, Bard). Derive gives `long_rest`, so the refusal "Sem usos: volta num descanso longo" is wrong, and the feature's own note says it also returns on a short rest. | `TestRulesReviewRests_BardicInspirationRecharge`: `bard 5: ... recharge = "long_rest", want "short_rest"` |
| R8-4 | medium | confirmed | `backend/internal/rules/srd51/effects/{monk,paladin,rogue,warlock}.json` (no entries) | Other features with a stated number of uses have no resource: Wholeness of Body (monk 6, once, long rest), Cleansing Touch (paladin 14, CHA modifier uses, min 1, long rest), Stroke of Luck (rogue 20, once, short or long rest), Dark One's Own Luck (fiend warlock 6, once, short or long rest), Hurl Through Hell (fiend warlock 14, once, long rest). | Monk 6: SRD says once per long rest; the sheet has only `ki`. Same for the other four. | `TestRulesReviewRests_OtherLimitedUseFeatures/*`: `... no resource has source feature:wholeness-of-body` (and the four others) |
| R8-5 | low | policy | `backend/internal/rules/srd51/effects/barbarian.json:5`, `backend/internal/rules/actions.go:33` | Unlimited Rage at barbarian level 20 is stored as 99 uses. | Barbarian 20: SRD table says unlimited; the sheet and the live session show "0 de 99 usados" (`usedWords`). Only a code comment describes it; no document names it a decision. | `TestRulesReviewRests_UnlimitedRageIsNotACounterOf99`: `rage is a counter of 99 uses` |
| R8-6 | low | refuted | `web/src/app/pages/character-editor/hit-points-preview.ts` | The editor's HP preview leaves out Dwarven Toughness / Draconic Resilience. | Hill Dwarf Fighter 3, CON mod +2, rolls 5 and 6: editor 27, server 30. The box itself says it is a preview that does not count race, class or feature HP, so it is not a wrong number. | none needed |
| R8-7 | low | unverified | `backend/internal/rules/actions.go:112-116` | Two classes with the same resource key (Channel Divinity of cleric and paladin) are one pool and "the first wins", so the pool size depends on class order (paladin 3 first / cleric 6 second gives 1 use, the reverse gives 2). | I could not check the SRD multiclassing text for this: the importer's rules file has no such passage. Needs the SRD 5.1 Multiclassing chapter. | none |

Notes for other areas (one line each):
- Death by massive damage (remaining damage at least the maximum HP) is never surfaced by the engine; `combat.ApplyDamage` returns `Excess` but `play` does not read it. The docs make it the master's decision (RN-03), so it is not a finding.
- Improved Critical (Champion, crit on 19-20) is only a text note; `combat.ResolveAttack` crits on a natural 20 only. Combat area.
- Arcane Recovery, Natural Recovery, Font of Magic conversions and Sorcerous Restoration are notes or grant-only; they belong to rests and slots, not built.

## Fix direction

**R8-1.** Root cause: `tally` only names the stable outcome; nothing zeroes the counts when it happens. Fix in code: in `combat.tally`, return `Failures: 0` when the outcome is stable, and keep a way to know a character is stable (today the app derives it from `Successes >= 3`, and `deathSaveProto` at `combat_death.go:195` reads that). Simplest: keep 3 successes as the stable marker and zero only the failures. Same mistake elsewhere: `combat_death.go:137-142` stores what `tally` returns, and `failuresWhileDown` (`:291`) already resets successes for a stable target. Docs that change: `docs/architecture.md` (RollDeathSave paragraph), `docs/product/rules.md` RN-03, the glossary line about stable. Guards: `TestRulesReviewRests_StableResetsTheCounts`, `TestRN24_HiddenDeathSavesAreTheOwnersAndTheMasters`, the RN-03 play tests.

**R8-2 and R8-4.** Root cause: handwritten effects are missing. Fix in `effects/` data: add `resource` effects on `feature:indomitable-1-use` (max `1`), `-2-uses` (max `2`) and `-3-uses` (max `3`, or one formula `classLevel("fighter") >= 17 ? 3 : classLevel("fighter") >= 13 ? 2 : 1`, as Action Surge does), long rest; and the five features of R8-4 (Cleansing Touch max `max(1, mod("cha"))`). This bumps the content revision (`effects/revision.json`) as CONTRIBUTING.md says. `names_pt.json` needs `resource:<key>` names. Check `TestSnapshot`-style tests and the golden files in `rules/testdata/golden` (a fighter or paladin golden will change). Same mistake elsewhere: Mystic Arcanum and Signature Spell are documented as master adds, so leave them.

**R8-3.** Root cause: the resource is declared once on level 1 with `long_rest`. Fix in data: either make the recharge depend on level (the schema takes a fixed string; a `font-of-inspiration` effect with a second `resource` of the same key would be skipped because the first one wins in `resourcesAndActions`), or extend the resource effect with a level-conditioned recharge. Code change small, in `actions.go`/`effects.go`. Docs: `docs/architecture.md:1702` (the table says long rest). Guard: `TestRulesReviewRests_BardicInspirationRecharge` plus the `golden` files.

**R8-5.** Decision needed: either show "ilimitado" (a `Max` of 0 with a flag) or document 99 as the decision in `docs/product/rules.md`. Web: `usedWords` in `live-session/vitals.ts`, the adjust sheet and the combat option text.

## Checklist

- Short rest: spend hit dice with CON modifier — `not built (doc)` (rests do not exist yet).
- Long rest: all HP, half the hit dice (min 1), slots, exhaustion −1 — `not built (doc)`.
- Resource uses by level: Rage 2/3/4/5/6/unlimited — covered by `Derive` sweep, matches the SRD (unlimited: `finding R8-5`).
- Channel Divinity (cleric 1/2/3 at 2/6/18; paladin 1), Wild Shape 2, Second Wind 1, Action Surge 1/2 (17), Ki = monk level from 2, Sorcery Points = level from 2, Lay on Hands 5 × level, Divine Sense 1 + CHA mod, Arcane Recovery once — matched the SRD by sweep over levels 1-20, no test dedicated (the golden covers the wizard).
- Recharge per rest: all of the above match the SRD except Bardic Inspiration — `finding R8-3`.
- Bardic Inspiration uses (CHA mod, min 1) — correct in the sweep.
- Indomitable — `finding R8-2`; the other once-per-rest features — `finding R8-4`.
- Arcane Recovery slot recovery, Natural Recovery, Sorcerous Restoration, pact slots on rest — `not built (doc)`.
- The 15 conditions — list correct (`web/src/app/core/combat/conditions.ts`); effects of each — `not built (doc)` (labels only).
- Exhaustion six levels — `not built (doc)` (one label).
- Concentration DC `max(10, damage/2)` — covered by `combat` tests.
- Death saves: DC 10, nat 1 = two failures, nat 20 = 1 HP, damage at 0 HP = 1 failure (2 on a critical), healing resets — covered by `TestHealingSpellRevivesAndResetsDeathSaves` and the RN-03 tests; stable resets counts — `finding R8-1`; instant death — master's decision (RN-03).
- RN-18: typed d20 1-20, typed damage sum within N..N×faces, Second Wind 1d10 + fighter level — covered by `TestRN18_PhysicalRollsAreTypedSums`, `TestSecondWindAndActionSurge`.
- RN-24 critical options (doubled dice; maximum plus one roll; bonus once) — covered by `TestRN24_TheCriticalFollowsTheTablesRule`; code read matches.
- RN-24 ability methods (standard array 15,14,13,12,10,8; point buy 27 with costs 0,1,2,3,4,5,7,9; 4d6 drop lowest; typed 3-18) — read against the SRD 5.2.1 table, matches; covered by `abilitymethods_test.go`.
- RN-24 level-up HP option, hidden death saves — enforced in `characters/levelup.go`; behaviour read, no defect found.
- Standard actions: ten, as the SRD lists (Attack, Cast a Spell, Dash, Disengage, Dodge, Help, Hide, Ready, Search, Use an Object), all `action` economy — matches.
- Action Surge once per turn — enforced (`rules/combat/turn.go:335`).
- Editor HP preview — `R8-6` refuted.
