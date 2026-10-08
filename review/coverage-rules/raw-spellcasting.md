# Raw sweep: Spellcasting (SRD 5.1 chapter "Spellcasting")

Sources read: SRD text from 5e-SRD-Rules.json (sections spellcasting, what-is-a-spell, spell-level, known-and-prepared-spells, spell-slots, cantrips, rituals, casting-a-spell, casting-time, spell-range, components, duration, targets, areas-of-effect, spell-saving-throws, spell-attack-rolls, combining-magical-effects, the-schools-of-magic). Counts computed from backend/internal/rules/srd51/data/spells.json (319 spells) and backend/internal/rules/srd51/effects/spells.json (12 entries).

## Machine coverage counts (SRD 5.1, 319 spells)

- Spells in the SRD data: 319 (data/spells.json).
- Spells with a machine effect registry entry (effects/spells.json): 12. Kinds: summon 3 (spell:animate-dead, spell:conjure-animals, spell:find-familiar), hp_pool 2 (spell:color-spray, spell:sleep), hp_threshold 2 (spell:power-word-kill, spell:power-word-stun), zero_hp_target 1 (spell:spare-the-dying), flat_heal 1 (spell:heal), temp_hp 1 (spell:false-life), max_hp 1 (spell:aid), ignores_cover 1 (spell:sacred-flame).
- Spells with structured damage (damage field, server rolls it when cast): 66.
- Spells with heal_at_slot_level (server opens a heal): 10.
- Spells with attack_type (server rolls the d20, in app or typed): 16.
- Spells with save_ability (server rolls the target's save): 92 (43 of them also have damage).
- Spells with a save but no damage, no heal and no effects entry (the save is rolled and shown, nothing is applied): 49.
- Spells with any server mechanic (damage, heal, attack, save or effects entry, union): 132.
- Spells with no server mechanic at all (slot spent and logged, text only): 187.
- Structured area in data (area_type): 88. Target overrides in effects/spell_targets.json: 41 (self 11, creatures 11, area 10, none 8, creature 1).
- Concentration spells (data flag): 126. Ritual-tagged spells (data flag): 29. Cantrips (level 0): 24.
- Cantrips with character-level damage tables (at_character_level): 10 damage entries.

Note: the "text only" 187 includes spells that impose conditions, move creatures, grant buffs, or create effects, none of which the server applies (see RN-22 in docs).

---

MECHANIC: spellcasting / intro
SERVER: backend/internal/rules/spellcasting.go:39-101 derives each class's casting numbers (save DC = 8 + prof + mod at :55, attack bonus at :55) and the slot table; no per-cast logic here.
SCREEN: web/src/app/pages/live-session/combat/cast-sheet/cast-sheet.ts:131-138 documents the cast sheet (slot radios, targets, last-slot warning, Conjurar button).
DOCS: docs/product/rules.md:329 "It does not apply the effects by itself; the master decides."
TESTS: backend/internal/play/combat_spells_test.go:224 TestMR014_CastingSpendsTheSlot

MECHANIC: what-is-a-spell / effects a spell can have (damage, heal, conditions, HP, summons)
SERVER: backend/internal/play/combat_spells.go:439-462 routes a cast to castHPSpell (for the 12 effects/spells.json entries, combat_spells.go:439) or to resolveOnTarget (combat_spells.go:568-576: attack, save, darts, heal, else nothing). Conditions are set only by the HP-reading kinds: backend/internal/play/combat_spells_hp.go:247-261 giveCondition/setCondition, called from the pool (:125), threshold (:146) and flat heal end-list (:219). No save-failure condition is ever applied (spellSave, combat_spells.go:636-666, only opens damage or heal).
SCREEN: web/src/app/pages/live-session/combat/cast-sheet/cast-sheet.ts:131-138 (sheet text); the result text is built from the server.
DOCS: docs/product/rules.md:230 "Spells that read HP. Sleep, Color Spray, Power Word Stun, Power Word Kill, Spare the Dying and Heal" (server resolves these); docs/product/rules.md:329 "It does not apply the effects by itself"; docs/product/rules.md:329 "the saving throws a spell forces later ... the master calls for the later ones" (text, not built).
TESTS: backend/internal/play/combat_spells_hp_test.go:118 TestMR014_SleepUsesTheRealHitPoints; :236 TestMR014_ColorSprayBlindsByThePool; :269 TestMR014_PowerWordStunAndKillOnNPCs

MECHANIC: spell-level / 0-9 levels, slot must be at least spell level
SERVER: backend/internal/play/combat_spells.go:114-128 slotOf: a cantrip takes no slot; a leveled spell needs a slot of level >= spell level and <= 9, and it must be one of the caster's free slots (options list). backend/internal/rules/spellcasting.go:308-312 (checkSpell) flags a prepared/known spell above maxLevel (character-sheet check, not a cast check).
SCREEN: web/src/app/pages/live-session/combat/cast-sheet/cast-sheet.ts:302-303 "Escolha o espaço de magia." when level > 0 and no slot chosen.
DOCS: docs/product/stories.md:302 "with each level's slots above ("1º nível ○ ✕ ✕ ✕ 1 livre de 4")"
TESTS: backend/internal/rules/spell_level_test.go:5 TestMaxSpellLevelFromSlots; backend/internal/play/combat_spells_test.go:2436 TestFlameStrikeFromASixthLevelSlot

MECHANIC: known-and-prepared-spells / known count and prepared count (sheet side)
SERVER: backend/internal/rules/spellcasting.go:58-70 SpellsKnownMax (known casters) and PreparedMax (prepared casters); :349-355 prepared must be on the spellbook for a wizard; :362-366 known count check; :367-371 prepared count check (issue only, not a block). Checks produce warnings on the sheet (issueChange), they do not stop the cast.
SCREEN: no match for "preparar" swap UI in live-session (prepare happens on the character editor, out of this chapter).
DOCS: docs/product/stories.md:123 "Etapa 'Magias': lista só as magias até o maior nível da magia do nível atual"; docs/product/stories.md:291 (turn options list).
TESTS: none found for the count checks (no spellcasting_test.go in rules/).

MECHANIC: known-and-prepared-spells / preparing and swapping during the session
SERVER: no match for prepare/swap action in backend/internal/play (play/*.go mentions "prepar" only in creature_cast.go, combat_spells.go, combat_reactions.go and puzzles; none is a prepare-spell action). unsure whether a long-rest swap exists outside play.
SCREEN: no match in web/src/app/pages/live-session.
DOCS: no match for swap/prepare in combat.
TESTS: none.

MECHANIC: spell-slots / spending a slot on cast
SERVER: backend/internal/play/combat_spells.go:391-397 spendSlot for a player's character (caster.Kind == kindPlayer); NPCs spend nothing. backend/internal/play/combat_vitals.go:64-70 spendSlot refuses NO_SLOT when none of that level is left. Pact slots carried by slot.Pact (combat_spells.go:124).
SCREEN: web/src/app/pages/live-session/combat/cast-sheet/cast-sheet.ts:410-421 "Espaços de ... : n livres de m" after the cast.
DOCS: docs/product/stories.md:288 "CastSpell casts and spends the slot and the action at once"; docs/product/rules.md:10 RN-02 (spell slots tracked from actions).
TESTS: backend/internal/play/combat_spells_test.go:224 TestMR014_CastingSpendsTheSlot

MECHANIC: spell-slots / long rest restores slots
SERVER: no match for long-rest restoration in backend/internal/play/combat_*.go; unsure (vitals in characters/ not read in this sweep).
SCREEN: no match in this sweep.
DOCS: docs/product/rules.md:10 RN-02 says slots are tracked; restore not located.
TESTS: none.

MECHANIC: spell-slots / casting at a higher level (upcasting): slot level drives damage dice
SERVER: backend/internal/play/combat_spells.go:314-317 slotLevel = slot level; characters/combatspells.go:71 DamageAtChoosing(slotLevel, TotalLevel, pick); backend/internal/rules/spelldetails.go:237-251 DamageAt uses at_slot_level table (atLevel: highest entry at or below the level, spelldetails.go:306-318); :269-290 DamageAtChoosing for scale/alternative choices.
SERVER: backend/internal/rules/spelleffects.go:213-216 SpellEffect scales hp_pool dice (per level above), flat_heal heal, temp_hp and max_hp amounts by (slotLevel - spellLevel).
SERVER: backend/internal/play/combat_spells.go:159-180 maxTargetsOf: Magic Missile darts via combat/rolls.go:127-129 MissileDarts (3 + level-1); Scorching Ray 3 + levels above (combat_spells.go:166); table/SRD target counts add TargetPerLevel per level above (combat_spells.go:173); extra-target text adds one per level (:177).
SCREEN: web/src/app/pages/live-session/combat/cast-sheet/cast-sheet.ts:229-234 slotLevel drives targetRule and darts; cast-sheet.ts:354-358 "n criaturas na área".
DOCS: docs/product/rules.md:344 "with more dice per level above or per cantrip tier".
TESTS: backend/internal/rules/spelldetails_test.go:213 TestSpellDamageGrowsWithTheSlot; backend/internal/play/combat_spells_test.go:2052 TestMagicMissileDartIsAlways1d4Plus1; backend/internal/play/combat_tablespells_test.go:391 TestMR025_ScorchingRayTakesOneMoreRayForEachCircle

MECHANIC: cantrips / cast with no slot
SERVER: backend/internal/play/combat_spells.go:114-120 slotOf: a cantrip with a slot is refused; combat_spells.go:52-76 castableOf: a cantrip with an attack roll is refused as CastSpell (must use RollAttack, :73); a save cantrip is castable here with economy ACTION (:75).
SCREEN: web/src/app/pages/live-session/combat/cast-sheet/cast-sheet.ts:302 slot only required when level > 0.
DOCS: docs/product/stories.md:291 "in each group cantrips first"; rules text in SRD only.
TESTS: backend/internal/play/combat_spells_test.go:2074 TestCantripsAreNotPartOfExtraAttack

MECHANIC: cantrips / damage scales by character level (5, 11, 17)
SERVER: backend/internal/characters/combatspells.go:70-71 DamageAtChoosing(slotLevel, d.TotalLevel); backend/internal/rules/spelldetails.go:237-251 DamageAt reads by_character_level (at_character_level in data) when present, using the character's total level; the 10 SRD damage entries with at_character_level are the cantrip tiers. For "scale"/"alternative" spells (DamageChoice), the character-level table is not read (spelldetails.go:269-290): unsure whether any such cantrip exists.
SCREEN: web/src/app/core/content/spell-draft.ts:253 "Nos níveis 5, 11 e 17 do personagem." (table editor text).
DOCS: docs/product/stories.md:562 "the cantrip by tier"; docs/product/rules.md:344 "per cantrip tier".
TESTS: backend/internal/play/combat_tablespells_test.go:366 TestMR025_ATableCantripGrowsByTheCharactersLevel; backend/internal/characters/combatspells_cast_test.go:49 TestCastKeepsEveryDamageTypeOfTheSpell

MECHANIC: rituals / cast as ritual (no slot, +10 minutes)
SERVER: no ritual flag on CastSpellRequest (proto/meurpg/play/v1/combat.proto:2742-2770: no ritual field). CastSpell (combat_spells.go:192-496) never takes a ritual path; a ritual spell in combat is cast as a normal spell with a slot. The ritual casting path exists only for summons outside combat: backend/internal/play/creature_cast.go:194-196 (ritual means no slot), :242-270 (ritual allowed only if CanRitual). The "+10 minutes" is not applied to any cast. Ritual check for summons: backend/internal/characters/charactercreatures_roster.go:419 (ritual needs a prepared spell or wizard spellbook).
SCREEN: web/src/app/core/combat/combat-options.ts:204-205 shows a "Ritual" tag only; no ritual cast option for generic spells. web/src/app/pages/character-sheet/creatures-panel/summon-sheet.ts:369-376 "Conjurar como ritual · 1 hora · sem gastar espaço" (summons only).
DOCS: docs/product/stories.md:888 "Find Familiar as a ritual, no slot" (summons outside combat only); docs/product/stories.md:888 says nothing about generic rituals.
TESTS: backend/internal/play/combat_spells_test.go: no ritual test found; backend/internal/characters/charactercreatures_roster.go:362 (no test located).

MECHANIC: casting-a-spell / general cast flow (who may cast, economy, slot, targets, roll)
SERVER: backend/internal/play/combat_spells.go:192-496 CastSpell: campaign membership (:196), turn/mayAct (:268), mustActNow (:271), refuseInShape (:274: no spells in a beast form), targets not defeated (:282-285), turn options castable (:289-309), slot (:310), roll checks (:359-379), economy spend (:382-409), concentration (:418-429), per-target effects (:439-462).
SCREEN: web/src/app/pages/live-session/combat/cast-sheet/cast-sheet.ts (cast sheet, "Conjurar" button, :131-138).
DOCS: docs/product/stories.md:288 "CastSpell casts and spends the slot and the action at once (criterion 2)."
TESTS: backend/internal/play/combat_spells_test.go:1896 TestMR014_SpellsAuthorizationMatrix; :1758 TestCombatSpellsAreIdempotent

MECHANIC: casting-a-spell / a spell cannot be cast while in a beast form (druid)
SERVER: backend/internal/play/combat_spells.go:274 refuseInShape (no spells in a beast form, MR-037)
SCREEN: web/src/app/core/combat/combat-errors.ts:170 "Na forma de fera não dá para conjurar."
DOCS: docs/product/rules.md (Wild Shape, not in this chapter).
TESTS: none located in combat_spells_test.go.

MECHANIC: casting-time / action
SERVER: backend/internal/rules/combat/turn.go:392-401 spellEconomy maps "1 action" to EconomyAction; economyOption turn.go:380-390 blocks when action used (ReasonActionUsed); castError combat_spells.go:84-93 disabled reason ACTION_USED (master may ignore: ignoreEconomy).
SCREEN: web/src/app/core/combat/combat-options.ts:201-205 tags only; cast-sheet.ts (economy of the chosen spell) not read in this sweep (unsure).
DOCS: docs/product/stories.md:291 economy-based option ordering.
TESTS: backend/internal/play/combat_spells_test.go:2645 TestBonusActionSpellLeavesNoOtherSpellButACantrip (action path shared)

MECHANIC: casting-time / bonus action spell (one per turn, only a 1-action cantrip alongside)
SERVER: backend/internal/rules/combat/turn.go:454-456 IsFreeCantrip (level 0 and action economy); turn.go:487 ReasonBonusActionSpellLimit blocks a spell after a bonus-action spell unless it is a free cantrip; combat_spells.go:95-100 castError maps it (ignored when master); combat_spells.go:412-417 SetCombatantSpellsCast records spell_cast and bonus_spell_cast (not for free cantrips); combat_spells.go:382-388 spell-before flags kept for undo.
SCREEN: web/src/app/pages/live-session/combat/cast-sheet/cast-sheet.ts (reason text comes from the server; no own rule in the screen).
DOCS: docs/product/stories.md:291 (turn options, no bonus-spell limit text found).
TESTS: backend/internal/play/combat_spells_test.go:2645 TestBonusActionSpellLeavesNoOtherSpellButACantrip; :2670 TestSpellThenBonusActionSpellIsRefusedToo

MECHANIC: casting-time / reaction spell (cast when a trigger happens)
SERVER: backend/internal/rules/combat/turn.go:479 Shield (spell:shield) gets REACTION_ONLY_WHEN_HIT; other reaction spells REACTION_ONLY (combat_spells.go:104-105 refuses a direct CastSpell; "UseReaction, for Escudo"); backend/internal/play/combat_reactions.go:168 UseReaction is the path. The trigger text (e.g. "when a creature you can see attacks you") is not parsed: the server does not check the trigger, the master/player picks the moment.
SCREEN: web/src/app/pages/live-session/combat/cast-sheet/cast-sheet.ts:425-427 "Escudo indisponível: sem espaço de..." (Shield availability text).
DOCS: docs/product/stories.md:291 "Shield on the character's own turn can never be cast and sits among the others with the reason REACTION_ONLY_WHEN_HIT".
TESTS: backend/internal/play/combat_spells_test.go:639 TestShieldTurnsAHitIntoAMiss; :2292 TestShieldAlsoStopsTheOtherHitsAwaitingTheReaction

MECHANIC: casting-time / longer than one action (minutes or hours)
SERVER: backend/internal/rules/combat/turn.go:69-71 ReasonTooLong (CASTING_TIME_TOO_LONG) for casting times of a minute or more: spellEconomy (turn.go:392-401) returns "" and the option is disabled; combat_spells.go:106-107 castError refuses ("this spell takes too long to cast in a fight"); combat_spells.go:298-305 summons only get the specific message. No concentration-per-turn or cast-over-several-turns logic exists: the server does not track a multi-turn cast.
SCREEN: web/src/app/core/combat/combat-options.ts:201-205 (tag only); summon-sheet.ts:369-376 shows the 1-hour ritual text outside combat.
DOCS: docs/product/stories.md:888 summons outside combat only.
TESTS: no test located for CASTING_TIME_TOO_LONG on a generic spell.

MECHANIC: spell-range / reach from the caster (ranged spells, touch, self)
SERVER: backend/internal/play/combat_spells.go:134-144 reachOf: Self reaches the caster alone (unless an area), Touch = meleeReachFt, Ranged = the spell range in feet, others unlimited. checkTargets combat_spells.go:535-553: non-master targets must be within reach (distanceFt), refused with TARGET_OUT_OF_REACH and missingFt (:549-551); NOT_PLACED if a grid position is missing (:546-548); no range check without a grid (:543, theatre). The master is never held to range (:532-534).
SERVER: area spells use the same per-target distance from the caster (the range applies to each combatant, not to a point of origin): combat_spells.go:536-552 (no origin point exists).
SCREEN: web/src/app/pages/live-session/combat/cast-sheet/cast-sheet.ts:236-247 reachFt from SpellRange RANGED, castTargetRows (out-of-reach marking); web/src/app/core/combat/combat-errors.ts:107 "Longe demais: faltam ... para chegar ao alvo."
DOCS: docs/product/stories.md:291 (no range text); docs/product/stories.md:288 CastSpell resolves targets; SRD range text is not in docs.
TESTS: backend/internal/play/combat_tablespells_test.go:429 TestMR025_ASpellForTheCasterAloneHasNobodyToPick; backend/internal/rules/spelltarget_test.go:303 TestTableSpellRangeAndTargetMustAgree

MECHANIC: spell-range / range of the spell read from text (ranged, sight, self, touch, special)
SERVER: backend/internal/rules/spelldetails.go:386-419 parseRange: "N feet", "N mile(s)", self, touch, sight, unlimited, special (anything else is RangeSpecial, unlimited for the server).
SCREEN: web/src/app/shared/spell-details/spell-details-format.ts (range text, not read in detail: unsure).
DOCS: docs/product/stories.md:131 "range (in metres: 5 ft = 1.5 m)".
TESTS: backend/internal/rules/spelldetails_test.go:35 TestParseSpellStrings

MECHANIC: components / verbal (V)
SERVER: no check: CastSpell (combat_spells.go:192-496) reads no component; no silence or gagged state exists (no match for "silence|silêncio|gag|mordaça|silenced" in backend/internal/play or rules). Data parsed only: backend/internal/rules/spelldetails.go:332-340 sets Components.Verbal.
SCREEN: web/src/app/shared/spell-details/spell-details-format.ts:93-100 formatComponents prints "V, S, M" and the material text; web/src/app/shared/spell-details/spell-body.ts:7-8 shows components row.
DOCS: docs/product/stories.md:131 components shown in the "?" dialog; no doc says V/S checks exist.
TESTS: none for component checks.

MECHANIC: components / somatic (S), free hand
SERVER: no check (no free-hand or hands state in CastSpell or combatant columns; no match for "mão livre|free hand|hand" in backend/internal/play/combat_spells.go). unsure about the hand state in the sheet (not read).
SCREEN: no match in cast-sheet.
DOCS: no match.
TESTS: none.

MECHANIC: components / material (M), costly material (gp must be carried), consumed material, focus or pouch
SERVER: no check: no match for material cost, consumed material or focus/pouch in CastSpell. Table validation only: backend/internal/rules/overlay_spell.go:62-76 (M needs material text and only with M), overlay_spell.go:104 stores Material; the SRD material text is kept as MaterialText (spelldetails.go:58-59, :325).
SCREEN: web/src/app/shared/spell-details/spell-details-format.ts:98-100 material shown in English in brackets.
DOCS: docs/product/stories.md:131 "components and duration" shown; no consumed-material rule documented.
TESTS: none.

MECHANIC: duration / instantaneous
SERVER: backend/internal/rules/spelldetails.go:421-443 parseDuration Kind "instantaneous" (DurationInstantaneous, :64); nothing else in play needs it: a damage or heal opened by the cast is the effect.
SCREEN: web/src/app/shared/spell-details/spell-body.ts:7-8 shows duration text.
DOCS: no match.
TESTS: rules/spelldetails_test.go:95 TestSpellDetailsCoverTheCatalog (catalog coverage).

MECHANIC: duration / timed (1 round, 1 minute, 1 hour, 8 hours, etc.) and ticking by round
SERVER: backend/internal/rules/spelldetails.go:421-443 parses Amount and Unit (round, minute, hour, day) but nothing in backend/internal/play or rules/combat reads it: no round counter, no expiry, no per-round tick for any spell. Only concentration ends a spell (see concentration blocks below). Grep for duration/expire/rounds_left in play, rules/combat: no match.
SCREEN: web/src/app/shared/spell-details/spell-body.ts:7-8 shows text only.
DOCS: docs/product/stories.md:131 duration in text only.
TESTS: none.

MECHANIC: duration / until dispelled
SERVER: backend/internal/rules/spelldetails.go:65-66 DurationUntilDispelled (kind only); no dispel action found in play (no match for "dissipar|dispel" in play).
SCREEN: no match.
DOCS: no match.
TESTS: none.

MECHANIC: duration / concentration: start (one at a time)
SERVER: backend/internal/play/combat_spells.go:418-429 a concentration spell sets combatant.concentration_spell (SetCombatantConcentration) and replaces the old one (made.ConcEnded = old, :419-420); the old spell's creatures are dismissed by endSummons (:423; play/combat_creatures.go:352-390, reason "concentration" at :380). A cast of a concentration spell by a combatant with no concentration spell: nothing else changes.
SCREEN: web/src/app/pages/live-session/combat/cast-sheet/cast-sheet.ts:258-290 the "encerra a concentração" line when a concentration spell replaces another; web/src/app/core/combat/combat-log.ts:245-246 "A concentração anterior acabou".
DOCS: docs/product/rules.md:336 "A concentration spell cast puts its name in concentration_spell, and a second one replaces it (the log says which ended)."
TESTS: backend/internal/play/combat_tablespells_test.go:312 TestMR025_SeveralCreaturesConcentrationAndOnlyTheCaster; backend/internal/play/combat_spells_test.go:1099 TestRN22_ConditionsAndTheConcentrationReminder

MECHANIC: duration / concentration: breaks when taking damage (Constitution save, DC 10 or half the damage)
SERVER: backend/internal/rules/combat/rolls.go:117-119 ConcentrationDC = max(10, damage/2). backend/internal/play/combat_actions.go:1211-1219 concentrationDC (0 when not concentrating or no damage taken, using damage after temporary HP); :1159, :1392 set the DC on the damage answer; :1238 castConcentrationDC for a spell's damage (one save per cast, combat_actions.go:1052 comment). The server only reports the DC: the save is NOT rolled by the server, and concentration is NOT ended automatically on damage: the master or the player ends it (see next block).
SCREEN: web/src/app/core/combat/combat-log.ts:154-157 "Teste de Constituição, CD n, para manter a concentração"; web/src/app/pages/live-session/combat/npc-card/pending-damages.html and combat card show the DC (not read in full: unsure).
DOCS: docs/product/rules.md:334 "The reminder number is rules/combat.ConcentrationDC: the larger of 10 and half the damage."; docs/product/rules.md:340 "The app still only reminds the check when the caster takes damage"
TESTS: backend/internal/play/combat_spells_test.go:2523 TestFlameStrikeOnAConcentratingNPCRemindsOneSave; :2550 TestFlameStrikeOnAConcentratingCharacterRemindsOneSave; :2580 TestFlameStrikeDiscardingTheLastDamageStillRemindsTheSave

MECHANIC: duration / concentration: ends when incapacitated (unconscious)
SERVER: no automatic end. backend/internal/play/combat_conditions.go:84-94 setting condition:incapacitated/unconscious does not touch concentration_spell; the only end paths are combat_conditions.go:95-100 (end_concentration on SetCombatantConditions) and creature_cast.go:355-404 EndConcentration via stopConcentrating (:394-404). The conditions set by a spell (giveCondition, combat_spells_hp.go:247-261) also do not end it.
SCREEN: web/src/app/core/combat/combat-notices.ts:123-140 ConcentrationWatch shows the player "perdeu a concentração" from the server's data (only when the server ends it).
DOCS: docs/product/rules.md:336 "the player only ends the concentration of their own character"; docs/product/rules.md:340 "the master decides whether concentration dropped".
TESTS: backend/internal/play/combat_spells_test.go:1099 TestRN22_ConditionsAndTheConcentrationReminder (conditions do not end it, unsure)

MECHANIC: duration / concentration: ends when killed or dropped to 0 HP
SERVER: no automatic end. backend/internal/play/combat_spells_hp.go:269-303 dropToZero (Palavra de Poder Matar) sets Defeated (:274) and the death saves (:297-300) without calling stopConcentrating; backend/internal/play/combat_death.go:247 sets Defeated: true without ending concentration; no match for stopConcentrating outside combat_conditions.go and creature_cast.go.
SCREEN: web/src/app/core/combat/combat-notices.ts:102-111 (lost concentration notice, only from server data).
DOCS: docs/product/rules.md:340 "the master decides whether concentration dropped".
TESTS: none located.

MECHANIC: duration / concentration: ended by the player or the master (no action cost)
SERVER: backend/internal/play/creature_cast.go:355-377 EndConcentration (master or the caster's player; nothing to end is no event); :394-404 stopConcentrating clears concentration_spell and dismisses the creatures of that casting (endSummons, combat_creatures.go:352-390). Also via SetCombatantConditions end_concentration (combat_conditions.go:95-100).
SCREEN: web/src/app/core/combat/combat-client.ts:154-157 and :867-885 (endConcentration flag); web/src/app/core/combat/combat-log.ts:440-441 "deixou de se concentrar".
DOCS: docs/product/rules.md:338 "'Encerrar concentração'" in the master's Condições dialog; docs/product/rules.md:340 "`EndConcentration` (the caster's player or the master) ends the concentration".
TESTS: backend/internal/play/combat_spells_test.go:1099 TestRN22_ConditionsAndTheConcentrationReminder; docs cite TestMR037_ConcentrationEndingDismissesTheCastingsCreatures (not located in this sweep).

MECHANIC: duration / concentration: undo gives it back
SERVER: backend/internal/play/combat_undo.go:585-617 and :727-728 put the concentration back (ConcBefore / ConcEnded) with its creatures.
SCREEN: no match in this sweep for the undo text of concentration.
DOCS: docs/product/rules.md:340 "The master's undo gives back the concentration and the same creatures".
TESTS: backend/internal/play/combat_spells_test.go:1517 TestCombatUndoTakesBackEveryNewAction

MECHANIC: targets / clear path (total cover blocks targeting)
SERVER: backend/internal/play/combat_spells.go:540-541 checkTargets refuses a single-target spell whose target has total cover from the caster (errCoverTotal); area spells are exempt (`!sp.Area`), master is exempt (:532-534). Spells that target a point (area origin) get no obstruction check: the origin logic in SRD ("point of origin on the near side of the wall") is not built.
SERVER: cover is added to attack roll and Dex save: combat_spells.go:563-566 (hit.Cover), :607 (target AC + cover bonus), :643 (Dex save bonus + cover, not for total cover or sacred flame).
SCREEN: web/src/app/pages/live-session/combat/cast-sheet/cast-targets.ts (targets list only; cover text by server: unsure).
DOCS: docs/product/rules.md:250 "Cover never leaks the AC (MR-034)" (cover rule in general).
TESTS: backend/internal/play/combat_spells_test.go: no test named for total cover on spells located in this sweep (unsure).

MECHANIC: targets / yourself (target self if the spell allows)
SERVER: backend/internal/play/combat_spells.go:148-150 selfOnly (caster only, or Self range not an area); :356-358 caster becomes the target when no targets given; :506-508 a Self spell with a target other than the caster is refused. Targeting self for a "creature of your choice" spell: the server accepts the caster as a target (no check for hostility): checkTargets :536-546 skips range for t.ID == caster.ID.
SCREEN: web/src/app/pages/live-session/combat/cast-sheet/cast-targets.ts:17 and cast-sheet.ts:236-247 (target list including the caster: unsure).
DOCS: docs/product/stories.md:561 "Personal and Touch ranges are valid on a table spell".
TESTS: backend/internal/play/combat_tablespells_test.go:429 TestMR025_ASpellForTheCasterAloneHasNobodyToPick; backend/internal/characters/combatspells_test.go:17 TestSpellTargetingFollowsTheSRDText

MECHANIC: targets / how many creatures (single, several, up to N), number chosen by caster
SERVER: backend/internal/rules/spelltarget.go:123-146 srdTarget: structured area first (:132), magic missile/scorching ray (:134-137, 3 targets +1 per level), attack (:138-139 one), text area (:140-141 creatures, count 0 = any number), self (:142-143), else one creature (:145). backend/internal/play/combat_spells.go:159-180 maxTargetsOf limits the count per slot (0 = any); :525-530 refuses more targets than the limit for players (master exempt); :216-218 cap of 10 targets per cast (maxSpellTargets, :47).
SCREEN: web/src/app/pages/live-session/combat/cast-sheet/cast-targets.ts:17 (single radio, several group, darts) and cast-sheet.ts:229-232 targetRule with slotLevel.
DOCS: docs/product/rules.md:344 "one creature, several (with one more per level above, if the master wants), an area ..., or only the caster".
TESTS: backend/internal/rules/spelltarget_test.go:15 TestSRDSpellTargets; :205 TestSpellTargetMaxTargets; play/combat_tablespells_test.go:195 TestMR025_ATableAreaSpellWithASaveAgainstThreeTargets; play/combat_spells_test.go:2127 TestSpellCastIsLimitedToTenTargets

MECHANIC: areas-of-effect / cone
SERVER: rules/spelltarget.go:132-133 structured AreaType "cone" is accepted; no geometry is computed: no cone shape or width function in backend (grep "cone" found only content types and labels: rules/spelltarget.go:172, rules/overlay_spell.go:262, rules/content.go:368, characters/spelldetails.go:179). The caster picks the creatures in the cone by name (combat_spells.go:167-168 area takes any number, checkTargets does not check the cone).
SCREEN: web/src/app/pages/live-session/combat/cast-sheet/cast-targets.ts:17 and :102-140 several-target list for an area (names picked by the caster); web/src/app/core/content/spell-draft.ts:208 "Cone" for the table editor.
DOCS: docs/product/rules.md:344 "In combat, a table area spell works like the SRD's: the caster picks the creatures it catches, with no area drawn on the map."
TESTS: backend/internal/rules/spelltarget_test.go:92 TestStructuredAreasAgainstTheText; backend/internal/play/combat_tablespells_test.go:195 TestMR025_ATableAreaSpellWithASaveAgainstThreeTargets

MECHANIC: areas-of-effect / cube
SERVER: as cone: rules/spelltarget.go:132-133 accepts; no cube geometry (grep found only labels/validation). Caster picks names.
SCREEN: web/src/app/core/content/spell-draft.ts:209 "Cubo" (table editor label); cast-targets.ts:102-140 names.
DOCS: docs/product/rules.md:344 (as cone).
TESTS: rules/spelltarget_test.go:92.

MECHANIC: areas-of-effect / cylinder
SERVER: as cone: no geometry; rules/spelltarget.go:172 label "Cilindro"; caster picks names.
SCREEN: web/src/app/core/content/spell-draft.ts:210 "Cilindro" (table editor).
DOCS: docs/product/rules.md:344.
TESTS: none.

MECHANIC: areas-of-effect / line
SERVER: as cone: no line geometry for spells (backend/internal/rules/grid/line.go:89 Line is movement/sight, not spell area); caster picks names.
SCREEN: web/src/app/core/content/spell-draft.ts:211 "Linha".
DOCS: docs/product/rules.md:344.
TESTS: none.

MECHANIC: areas-of-effect / sphere
SERVER: as cone: no sphere geometry; rules/spelltarget.go:172 "Esfera"; caster picks names. Sphere origin (point within range) is not checked.
SCREEN: web/src/app/core/content/spell-draft.ts:212 "Esfera".
DOCS: docs/product/stories.md:501 "a 6 m radius sphere ... in combat the caster chooses the creatures it hits".
TESTS: rules/spelltarget_test.go:247 TestSpellTargetLabels.

MECHANIC: areas-of-effect / point of origin, spreading around corners, origin within range
SERVER: no origin point is stored or checked; no line-of-effect around corners. Range checked per target from caster (combat_spells.go:543-552). Cover exempt for area (:540). Area spells are the only ones not blocked by total cover.
SCREEN: none (no point picking on map for spells). cast-sheet.ts:358 "criaturas na área" text only.
DOCS: docs/product/rules.md:344 (no origin concept).
TESTS: none.

MECHANIC: spell-saving-throws / DC of the caster
SERVER: backend/internal/rules/spellcasting.go:53-56 SaveDC = 8 + prof + ability mod (server computed). characters/combatspells.go:63 out.SaveDC = sc.SaveDC.
SCREEN: web/src/app/pages/live-session/combat/cast-sheet/cast-sheet.ts (DC shown from server; not read in detail: unsure).
DOCS: docs/product/rules.md:250 cover block only; docs/product/stories.md:291 (no DC text found).
TESTS: backend/internal/play/combat_spells_test.go:2711 TestSpellSaveAgainstACreatureUsesItsStatBlock

MECHANIC: spell-saving-throws / target rolls the save (server rolls it)
SERVER: backend/internal/play/combat_spells.go:636-666 spellSave: save bonus from the target (saveOf; characters/combatspells.go:150-161 CombatSave; basic-sheet NPC without saves gets bonus 0 and Known=false :160); d20 always rolled in app (:646 s.d20(rollInput{inApp:true}, save.Bonus)); success via combat.SaveSucceeded(roll.Total, DC) (:650). A typed physical die is not accepted for a save (only attacks and pools).
SCREEN: web/src/app/core/combat/combat-log.ts (save results in log; not read in detail: unsure).
DOCS: docs/product/stories.md:288 "saving throw rolled by the server with one damage roll for the whole cast".
TESTS: backend/internal/play/combat_spells_test.go:444 TestSaveSpellRollsOnceForTheCast; :2711 TestSpellSaveAgainstACreatureUsesItsStatBlock

MECHANIC: spell-saving-throws / half damage on success, none on success
SERVER: backend/internal/play/combat_spells.go:655-664: if saved and the spell does not say "half" or "other", no damage is opened; with half, damage opened with Half=true (openSpellPending :699-715). Half rounding and application: backend/internal/play/combat_actions.go:1026 (g.Half handled in RollDamage).
SCREEN: web/src/app/core/combat/combat-log.ts (half/none text: unsure).
DOCS: docs/product/stories.md:286? (MR-014 text "the saving throw with half" at docs/product/stories.md:562).
TESTS: backend/internal/play/combat_tablespells_test.go:195 TestMR025_ATableAreaSpellWithASaveAgainstThreeTargets

MECHANIC: spell-saving-throws / Dexterity save: cover bonus; Chama Sagrada ignores cover
SERVER: combat_spells.go:643 cover adds to Dex save unless total cover or IgnoresCover (sacred flame: effects/spells.json "ignores_cover", rules/spelleffects.go:139-145; IgnoresCover at :201).
SCREEN: none.
DOCS: docs/product/rules.md:250 (cover rules; Sacred Flame not mentioned: unsure).
TESTS: backend/internal/rules/spelleffects_test.go:110 TestSacredFlameIgnoresCover

MECHANIC: spell-attack-rolls / attack bonus = spellcasting mod + prof
SERVER: backend/internal/rules/spellcasting.go:55 AttackBonus = prof + mod; characters/combatspells.go:60-62 ToHit = sc.AttackBonus for attack spells; combat_spells.go:599 d20(in, sp.ToHit).
SCREEN: none in this sweep.
DOCS: no match for the number in docs.
TESTS: backend/internal/rules/spelldetails_test.go:282 TestSpellAttacksAndSaves

MECHANIC: spell-attack-rolls / d20 rolled by server or typed physical die
SERVER: backend/internal/play/combat_spells.go:580-594 d20: app roll via dice.Roll or physical face via dice.Physical; combat_spells.go:364-379 spell attack requires roll_in_app or d20_face; typed face only for a single target (:371-373); master or mustRollThisWay (:375-377) enforces RN-18.
SCREEN: web/src/app/pages/live-session/combat/cast-sheet/cast-sheet.ts:492 defaults the lowest free slot; attack roll input in cast-sheet.html (typed/app: unsure).
DOCS: docs/product/rules.md:26 RN-18 "Physical or app dice".
TESTS: backend/internal/play/combat_spells_test.go:1896 TestMR014_SpellsAuthorizationMatrix; (RN-18 roll tests not located in this sweep)

MECHANIC: spell-attack-rolls / hit vs armor class, critical hit doubles dice
SERVER: combat_spells.go:598-630 spellAttack: target AC = armor class + ac bonus + cover bonus (:607); combat.ResolveAttack(sp.ToHit, targetAC, face) (:608); outcome miss/hit/crit (:612-617); damage opened with result.Critical (:624) so the critical doubles dice at roll time (RollDamage, combat_actions.go).
SCREEN: web/src/app/core/combat/combat-log.ts (hit/miss text, not read in detail: unsure).
DOCS: docs/product/stories.md:291 (no attack text).
TESTS: backend/internal/play/combat_tablespells_test.go:116 TestMR025_ATableSpellAttackInCombat; combat_spells_test.go:639 TestShieldTurnsAHitIntoAMiss

MECHANIC: spell-attack-rolls / disadvantage for ranged attack within 5 ft of a hostile creature
SERVER: no match: spellAttack (combat_spells.go:598-630) rolls a plain d20 with no advantage or disadvantage; no 5-ft check found in play for spells (grep "disadvantage|desvantagem" in combat_spells*.go: none).
SCREEN: no match.
DOCS: no match.
TESTS: none.

MECHANIC: combining-magical-effects / same spell does not stack; durations overlap
SERVER: no match for stacking rules in backend/internal/play or rules (grep "combin|stack|empilh" in play/rules: only giveTempHP comment "they do not stack", combat_spells_hp.go:305, which is the temporary HP rule). Temp HP: the target keeps the larger of current and given (combat_spells_hp.go:308-327).
SCREEN: no match.
DOCS: docs/product/stories.md:n/a: no match for "combinar" effects.
TESTS: backend/internal/play/combat_spells_hp_test.go:574 TestFalseLifeGivesTemporaryHitPoints

MECHANIC: the-schools-of-magic / school of magic
SERVER: school is data only (rules/spelldetails.go SpellEntry school via srd51 data); no rule depends on school (grep "school|escola" in play: no spell rule). The SRD says schools have no rules of their own.
SCREEN: web/src/app/shared/spell-details/spell-body.ts:7 "What a spell says, below its title" (school shown in the "?" dialog; unsure).
DOCS: docs/product/stories.md:131 "name, spell level and school".
TESTS: none.

MECHANIC: spell effects not in effects/spells.json: summons, buffs, debuffs, movement (text only)
SERVER: 187 SRD spells have no server mechanic (see counts). Examples of text-only SRD spells with saves but no damage/heal/effect: 49 (save shown, nothing applied). The server never applies a condition from a save failure (combat_spells.go:636-666).
SCREEN: the result shows the save roll and whether it saved; the effect is text for the master (web/src/app/pages/live-session/combat/cast-sheet/cast-result.ts, not read in detail: unsure).
DOCS: docs/product/rules.md:329 "Applying effects automatically is left for after the MVP."
TESTS: none for text-only spells.

MECHANIC: spell-level / upcast of summon spells (Animate Dead at 3rd and 5th circles)
SERVER: rules/spelleffects.go:127-135 summon loads count_per_level; play/creature_cast.go:42-100 prepareSummon uses slotLevel; combatant summon in combat via combat_spells.go:343-355.
SCREEN: web/src/app/pages/character-sheet/creatures-panel/summon-sheet.ts:369-376 (summon ritual and slot text).
DOCS: docs/product/stories.md:888 "Animate Dead, which spends the slot".
TESTS: backend/internal/play/combat_creatures? (test name not located in this sweep); backend/internal/play/combat_creatures_test (unsure).
