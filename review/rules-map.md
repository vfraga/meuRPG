# Rules review: movement, vision and the map (R5)

- Scope: `rules/grid`, `rules/vision`, `rules/combat/jump.go`, `rules/lights.go`, `rules/traps.go`, `play` (move, cover, opportunity, theatre, fog, traps), `maps` (fog, visibility, carried light, doors, traps), the web map and movement screens.
- Commit reviewed: `933df75` (main). SRD: 5e-database `a8abc93b235c158bb8cbf042e54425b9c2fd79b8` (rules text in `5e-SRD-Rules.json`; the Rule-Sections file does not exist at this commit).
- Time spent: about 2 hours. Model: claude-sonnet-5-5 (subagents: sonnet, three of them).
- Not findings, by table decision (RN-21, ADR in decisions.md Q36/Q69/Q70): movement as a straight line between centres (diagonal 7.1 ft), difficult terrain as +5 ft per square entered, range counted in whole squares rounded down. The SRD text has no diagonal rule, so none of these breaks it.

| id | severity | status | file:line | defect | failure scenario | evidence |
|---|---|---|---|---|---|---|
| R5-1 | high | confirmed | `rules/armor.go:46`, `rules/hitpoints.go:69` | The Strength minimum of heavy armor only adds a text hint; walking speed is not reduced. | Human fighter, Str 10, chain mail (needs 13): app 30 ft, SRD 20 ft (Armor table, Strength column: speed -10 ft). Same for splint and plate at Str 14. | `TestRulesReviewMap_HeavyArmorStrengthSpeed`: `human Str 10 chain mail (needs 13): app speed = 30 ft, SRD speed = 20 ft` |
| R5-2 | low | confirmed | `rules/vision/vision.go:167` | A light source with bright radius 0 lights its own square bright. | Dim-only light (Dancing Lights style, bright 0, dim 10): own square is Bright; SRD (Spells, Dancing Lights) says dim light only. A viewer sees `SeenBright`, not `SeenDim`. | `TestRulesReviewMap_DimOnlyLightSourceSquare`: `own square: LightAt = 3, want 2` |
| R5-3 | low | confirmed | `characters/combatturn.go:166-180` | `GetTurnOptions.options.economy.movement` ignores conditions that set speed to 0. | Grappled or restrained player on its turn: combatant view `movement_left_dft` = 0, turn options `left_dft` = 300 (SRD, Conditions: grappled/restrained speed 0). Web reads only the combatant view, so the screen is right. The creature path (`charactersroster.go:486`) is the same by code reading, untested. | `TestRulesReviewMap_TurnOptionsMovementWithGrappled` |
| R5-4 | high | policy | `rules/srd51/effects/rogue.json:12`, `monk.json:19`, `play/combat_actions.go:1633` | Cunning Action and Step of the Wind only spend the bonus action: no extra Dash movement, no Disengage. A second Dash in a turn (Action Surge) is a boolean, so speed is x2, not x3. | Rogue 2 uses Cunning Action to Dash: movement stays 30 ft where the SRD gives 60; Disengage by bonus action still offers opportunity attacks. Docs only describe it (architecture.md:1707 "Spends the economy and logs"). | code reading, no test |
| R5-5 | medium | policy | `play/combat_cover.go:46`, `play/combat_spells.go:545` | Cover for a Dexterity-save area spell is measured from the caster; the spell's point of origin does not exist in `CastSpellRequest`. | Fireball centred 40 ft away: the +2/+5 comes from what lies between caster and target, SRD (Cover) says between the point of origin and the target. | code reading, no test |
| R5-6 | low | policy | `rules/grid/door.go:20`, `rules/grid/cover.go` | A portcullis (`DoorBarred`) gives no cover. | Attack through a portcullis: app none, SRD (Cover) three-quarters (+5). Described in RN-26, no decision behind it. | scratch probe: `cover through portcullis: 0` |
| R5-7 | low | policy | `play/traps.go` (search skill enum), `maps/traps_play.go:368` | Trap search offers only Perception and Investigation; the SRD lets Arcana detect a magic trap at the same DC. | Fire-Breathing Statue (magic): Arcana DC 15 not possible. | code reading |
| R5-8 | n/a | refuted | `rules/lights.go`, `effects/lights.json`, `effects/traps.json`, `rules/combat/jump.go` | Candidates checked and found right: light radii of candle, torch, lamp, hooded lantern, Light, Continual Flame, Daylight; the 8 sample traps (DCs, damage, saves); severity tables; fall dice; long and high jump (Str score, 3 + Str mod, half standing, min 0); race speeds and sizes; monk and barbarian speed bonuses. | | probes and SRD comparison |

## Fix direction

**R5-1.** Root cause: `StrMinimum` is read only for a hint. Fix in code: in `armor.go` (or `speedAndSenses`) subtract 10 ft from `SpeedWalkFt` when the Strength score is below `StrMinimum`, unless the race has the dwarf trait (`trait:dwarven-armor-training` / dwarf race key). No content revision needed (data already has `str_minimum`: 13, 15, 15). Same mistake elsewhere: none (`StrMinimum` is used only in `armor.go:46`). Docs: remove "só uma dica" wording if any; add the rule to the speed section of rules.md. Risk: Str changes by magic items must flow through `x.scores`; `TestRulesReviewMap_HeavyArmorStrengthSpeed` and the barbarian heavy-armor test guard it.

**R5-2.** Root cause: `d2 <= BrightFt*BrightFt` is true at distance 0 when BrightFt is 0. Fix: guard with `src.BrightFt > 0 &&`. Same pattern elsewhere: none (darkvision and the other senses are guarded by `> 0`). Docs: none. Guard: the new test plus `TestSensesBeyondDarkvision`.

**R5-3.** Root cause: the turn options copy the speed without the `noSpeed` conditions. Fix in `play`: override `Economy.Movement` in `GetTurnOptions` from `movementLeftDFt(who)`/`speedDFt` (single source), for characters and creatures. Same mistake: `charactersroster.go:486-512`. Docs: architecture.md movement table. Guard: the new test and `TestRN25_ConditionsThatLeaveNoSpeed`.

## Checklist

- Speed, 5 ft squares, circle movement (RN-21, RN-25): table decision, covered by grid tests (`TestMoveCostIsInTenthsOfAFoot`).
- Difficult terrain: covered by `TestDifficultTerrainDoesNotStack`.
- Crawling, standing up from prone, climbing, swimming costs: not built (doc: architecture.md:1823, :2077).
- Jumping: covered by `TestJumpLimits`, `TestJumpsWithAndWithoutARunningStart`.
- Size and space, squeezing: not built (doc: every combatant occupies one square). Tiny sharing: `TestCreaturesOnTheWay`.
- Reach: covered by `TestAReachWeaponHitsAtTenFeet`. Lance disadvantage within 5 ft and long-range disadvantage: not computed (combat area).
- Moving through allies and hostile creatures: covered by `TestCreaturesOnTheWay`.
- Opportunity attacks, Disengage, forced move: covered by `TestLeavesReach`, `combat_opportunity_test.go`; bonus-action Disengage: finding R5-4.
- Cover degrees and bonuses: covered by `TestCoverBetweenAgainstTheCave`; portcullis R5-6; area spells R5-5.
- Area templates on the grid: not built (doc: area spells take any number of targets).
- Theatre of the mind: covered by `combat_theatre_test.go`.
- Light and vision (bright, dim, darkness, darkvision, blindsight, truesight, obscured): covered by `TestVisionAgainstTheCave`, `TestSensesBeyondDarkvision`, `TestPassivePenalty`; dim-only source R5-2. Blinded creatures still see on the fog map: RN-22 says conditions are marks.
- Light source radii: covered by `TestLightPresetsOfTheSRD`.
- Sample traps and severity tables: covered by `TestTrapPresetsOfTheSRD`, `TestTrapSeverityTables`, `TestFallDice`; Arcana R5-7.
- Browser: `core/units.ts` conversions (5 ft = 1.5 m, 10 ft = 3.0 m), `jump-plan.ts`, `move-plan.ts` do display only and match the server; no difference found.

## Other areas (one line each)

- Exhaustion levels 2 and 5 (speed halved, 0) do not change speed: conditions area.
- Evasion is not applied to trap or spell Dexterity saves: spells/sheet area.
- Long range and ranged-attack-in-melee disadvantage are not computed: combat area.
