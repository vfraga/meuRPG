# Raw sweep: the 15 SRD conditions (5e-SRD-Conditions.json) vs code

Scope: every bullet of each condition's text. Reads = code that branches on the `condition:<key>` string (non-test Go in backend/internal, non-spec TS in web/src/app). "LABEL" = stored and shown, nothing else. Grep-only lines are marked (grep).

Global findings (cross-cutting):
- Only 4 condition keys drive any server behavior: unconscious, blinded, stunned, plus the set cantReact (incapacitated/paralyzed/petrified/stunned/unconscious/blinded) and noSpeed (grappled/restrained/paralyzed/petrified/stunned/unconscious). deafened is only set/cleared (familiarsight.go:49, spells.json:46).
- No condition is enforced on attacks, saves, checks, damage, crits, or movement type. Advantage/disadvantage from conditions: none (no condition-keyed mode anywhere; rules/attacks.go:58 and armor.go:42 are item-keyed).
- Conditions are set by: the master (SetCombatantConditions, combat_conditions.go:57-86 via cleanConditions), 3 spells with no save (spells.json:7 Sleep, :13 Color Spray, :18 Power Word Stun; combat_spells_hp.go:125, :146), traps (traps.json conditions and on-fail conditions, applied in traps_effect.go:183-185 and :213-214, merged in combat_traps.go:200-211 and traps_fire.go:555). spells.json has no save fields (81 lines, grep "save" empty).
- Conditions never expire: duration_pt (traps.json:142 "1 hora", :241 "até se soltar") is carried as text (rules/traps.go:224,238; characters/presets.go:109) and read by no combat code. Removal: master SetCombatantConditions, Heal's Ends (spells.json:46, combat_spells_hp.go:208-221), or druid form end (wildshape.go:119-124).
- Concentration never ends from a condition. Ends only by EndConcentration (creature_cast.go:321-367), master SetCombatantConditions endConcentration (combat_conditions.go:95-98), a new concentration spell (combat_spells.go:418-426), or creature replacement (combat_creatures.go:227). rules.md:329 says this is deliberate: "It does not apply the effects by itself; the master decides."
- Exhaustion: no level counter exists anywhere. No match for "exaust" in play/rules/characters outside names (grep).

## Per-condition

MECHANIC: 2014 conditions / Blinded
- B1 can't see, auto-fails sight checks: no match for "sight check"/"vista"/auto-fail; reads of blinded: combat_opportunity.go:43 (cantReact) and :85-88 comment (a Blinded reactor does not see the mover, so no offer) | SCREEN: conditions.ts:20 (LABEL "Cego") | DOCS: rules.md:329 says no effects are applied | TESTS: combat_spells_hp_test.go:249, :264 (blinded via Color Spray)
- B2 attacks vs it have advantage; its attacks disadvantage: no match | SERVER: no match for condition-keyed advantage/disadvantage | SCREEN: no match | DOCS: no match | TESTS: none
- Extra (not an SRD bullet): combat_theatre.go:365 refuses an opportunity reaction when reactor has cantReact, which includes blinded. The SRD blinded text does not stop reactions. Factual deviation.

MECHANIC: 2014 conditions / Charmed
- B1 can't attack or harmfully target its charmer: no match (grep "charm|enfeit" in play/rules: none) | SCREEN: conditions.ts:22 (LABEL "Enfeitiçado") | DOCS: none | TESTS: none
- B2 charmer has advantage on social checks: no match | SERVER: none | SCREEN: none | DOCS: none

MECHANIC: 2014 conditions / Deafened
- B1 can't hear, auto-fails hearing checks: no match for check logic. Read: familiarsight.go:49 sightConditions (sets blinded+deafened on the combatant while a familiar's eyes are on); spells.json:46 Heal ends blinded and deafened (combat_spells_hp.go:208-221) | SCREEN: conditions.ts:31 (LABEL "Surdo"); familiar-band.ts:27 comment | DOCS: none found | TESTS: familiarsight_test.go:22 (blindAndDeaf)

MECHANIC: 2014 conditions / Frightened
- B1 disadvantage on checks and attacks while source in sight: no match | SERVER: none | SCREEN: conditions.ts:18 (LABEL "Amedrontado") | DOCS: none | TESTS: none
- B2 can't willingly move closer to source: no match (combat_move.go noSpeed list at :145 does not include it; no source tracking) | SERVER: none

MECHANIC: 2014 conditions / Grappled
- B1 speed 0, no speed bonus: ENFORCED. combat_move.go:145 noSpeed list; speedDFt returns 0 at combat_move.go:153-156 (opened 125-160) | SCREEN: conditions.ts:17 (LABEL "Agarrado") | DOCS: rules.md:281 (speed 0 for grappled, restrained) | TESTS: combat_theatre_test.go:1095 TestRN25_ConditionsThatLeaveNoSpeed
- B2 ends if grappler incapacitated: no match (no grapple link is stored) | SERVER: none
- B3 ends if moved out of reach (Thunderwave): no match for "agarr|grapple" in play/rules (grep) | SERVER: none

MECHANIC: 2014 conditions / Incapacitated
- B1 can't take actions or reactions: PARTIAL.
  Reactions: combat_opportunity.go:43 cantReact (includes incapacitated); skipped as candidate at combat_opportunity.go:108 and :279; combat_theatre.go:365 refuses. Note: combat_opportunity.go:43 comment says "incapacitated, and what includes it".
  Actions: no match. No condition read in combat_actions.go or combat_write.go (mayAct at combat_write.go:646, grep only shows no condition use). An incapacitated creature still gets Attack/Cast options: unsure, not traced in GetTurnOptions.
  Gap: incapacitated is NOT in noSpeed (combat_move.go:145), so an incapacitated mover may still walk. (Paralyzed/petrified/stunned/unconscious are listed; incapacitated alone is not.)
  SCREEN: conditions.ts:26 (LABEL "Incapacitado") | DOCS: none | TESTS: combat_opportunity_test.go (cantReact cases, grep)

MECHANIC: 2014 conditions / Invisible
- B1 impossible to see without magic; heavily obscured for hiding: no match (grep "invisib" in Go: none; combat_fog.go has no condition read) | SCREEN: conditions.ts:28 (LABEL "Invisível") | DOCS: none
- B2 attacks vs it disadvantage; its attacks advantage: no match | SERVER: none
- Sources: no condition:invisible in effects/*.json (grep). Manual label only.

MECHANIC: 2014 conditions / Paralyzed
- B1 incapacitated; can't move or speak: PARTIAL. cantReact (combat_opportunity.go:43, combat_theatre.go:365). Can't move: ENFORCED via noSpeed (combat_move.go:145). Speech: no match.
- B2 auto-fails Str and Dex saves: no match (no auto-fail path in combat_spells.go, combat_actions.go, or rules/combat; grep "automatic|auto.?fail" empty) | SCREEN: conditions.ts:29 (LABEL) | DOCS: none
- B3 attacks vs it advantage: no match
- B4 hit within 5 ft is a critical: no match. Crit logic reads only the attacker's sheet (combat_actions.go:730-734 CriticalRange; :785 result.Critical); no 5-ft/condition check. Reach 5 ft exists (combat_actions.go:179, :196) but not joined to condition.

MECHANIC: 2014 conditions / Petrified
- B1 transformed, weight x10, stops aging: no match (LABEL only, conditions.ts:30)
- B2 incapacitated; can't move or speak; unaware: PARTIAL. cantReact (combat_opportunity.go:43), noSpeed (combat_move.go:145). Speech/awareness: no match.
- B3 attacks vs it advantage: no match
- B4 auto-fails Str and Dex saves: no match
- B5 resistance to all damage: no match (damage code reads damage-type keys only: combat_actions.go:54 names; grep "condition" none in damage path)
- B6 immune to poison and disease: no match

MECHANIC: 2014 conditions / Poisoned
- B1 disadvantage on attack rolls and ability checks: no match (LABEL conditions.ts:23 "Envenenado"; names_pt.json:133)
- Source: traps.json:141 poison-needle, save Con DC 15 on_fail poisoned, duration_pt "1 hora" at :142; applied on failed save at traps_effect.go:213-214 (opened 165-230). Duration not enforced. TESTS: combat_fog_test.go:1219 (trap poisoned, grep); combat_opportunity_test.go:873 and combat_spells_test.go:1105 (label only, grep)

MECHANIC: 2014 conditions / Prone
- B1 only movement is crawl (stand costs half): NOT MODELED. combat_move.go:142-143 comment says so | DOCS: none
- B2 disadvantage on its attacks: no match
- B3 attacks vs it: advantage within 5 ft, else disadvantage: no match
- Sources (automatic, no save): traps.json:91, :114, :249, :275 with `conditions` (applied to every creature caught, traps_effect.go:183-185). Hidden pit (traps.json:249) falls prone on enter. combat_move.go:494 jumper falls prone: comment only ("the master's log says it"), no state set (grep). trap-draft.ts:100 default prone (grep). SCREEN: conditions.ts:21 ("Derrubado"); conditions-dialog.ts:23 ("Derrubado"); docs rules.md:338 (`condition:prone` is called "Derrubado") | TESTS: combat_fog_test.go:618, :902 (set prone as label, grep)

MECHANIC: 2014 conditions / Restrained
- B1 speed 0, no speed bonus: ENFORCED combat_move.go:145 (noSpeed); speedDFt at combat_move.go:153-156 | DOCS: rules.md:281 | TESTS: combat_theatre_test.go:1095
- B2 attacks vs it advantage; its attacks disadvantage: no match
- B3 disadvantage on Dex saves: no match
- Source: traps.json:240 (restrained, duration "até se soltar" at :241); applied on area trigger (traps_effect.go:183-185). LABEL conditions.ts:25 "Impedido"

MECHANIC: 2014 conditions / Stunned
- B1 incapacitated; can't move; speaks falteringly: PARTIAL. cantReact (combat_opportunity.go:43), noSpeed (combat_move.go:145). Speech: no match.
- B2 auto-fails Str and Dex saves: no match
- B3 attacks vs it advantage: no match
- Source: spells.json:18 Power Word Stun, hp_threshold 150, no save; giveCondition at combat_spells_hp.go:146 (opened 95-150) | TESTS: none found (grep "stunned" in tests not run)

MECHANIC: 2014 conditions / Unconscious
- B1 incapacitated; can't move or speak; unaware: PARTIAL. cantReact (combat_opportunity.go:43), noSpeed (combat_move.go:145). Druid form ends on unconscious: combat_conditions.go:90-93 (endFormIfAsleep), wildshape.go:119-124 and :150-154 (refuse Wild Shape). Sleep pool skip: combat_spells_hp.go:115 (HPCreature.Unconscious), const at combat_spells_hp.go:32; hp pool rule rules/combat/hpspells.go:49-63. Unaware: no match.
- B2 drops held items and falls prone: no match (no automatic prone or drop; traps only)
- B3 auto-fails Str and Dex saves: no match
- B4 attacks vs it advantage: no match
- B5 hit within 5 ft is a critical: no match (same as Paralyzed B4)
- Source: spells.json:7 Sleep (hp_pool, dice 5d8, condition unconscious), applied via giveCondition (combat_spells_hp.go:125); no save.
- Note: "Caído" (0 HP for a player character) is not this key: combat_actions.go:88 isDown; combat_view.go:369 comment.
- SCREEN: hp-effects.ts:118 (CONDITION_WORDS "adormece"/"Adormeceu") | DOCS: rules.md:230 (Sono reads HP) | TESTS: combat_spells_hp_test.go:193, :203, :212 (Sleep)

MECHANIC: 2014 conditions / Exhaustion
- Levels 1-6 (disadv checks; speed halved; disadv attacks/saves; HP max halved; speed 0; death): NO MECHANIC. No level counter field in combatants (playdb conditions is a string list, playdb/models.go:47). No match for exhaustion level logic in play/rules/characters.
- Long rest reduces by 1 with food/drink: no match for exhaust terms in play/rules (grep).
- Entry-level handling: conditions.ts:24 (LABEL "Exaustão"); names_pt.json:126; immunity lists read only at validation (rules/content.go:1121-1134, rules/creatures.go:106) and exposed to characters (characters/creatures.go:203), not used in combat.
- DOCS: none. TESTS: none.

## Sheet / player visibility and screens

- Server sends conditions to every viewer who sees the combatant: combat_view.go:293 (Conditions), :305-306 (ConditionNamesPt via names(key)); names from play.go:459-490 and :538-540. Hidden combatants excluded (rules.md:336).
- Character sheet: no condition read in pages/character-sheet (grep "condi": only a comment at character-sheet-source.live.ts:342). vitals.ts and party-panel.ts: no condition read (grep). So conditions are NOT on the character sheet.
- Web catalog: web/src/app/core/combat/conditions.ts:17-31 (15 entries, Portuguese names in alpha order, the list the dialog shows); conditionTags at :35-40.
- Master dialog: conditions-dialog.ts:23-30 comment; :37 title "Condições de <label>"; :47 note "Só rótulos: o app não aplica os efeitos. Os jogadores veem as condições de quem eles veem." (quoted). Entry: order-list.html:167 "Condições…" and combat-view.ts:1721 openConditions.
- Tags: combatant-tags.ts (header comment; :15 aria "Condições de"), order-strip.ts:199, order-column.ts:103, combat-map.ts:235 (aria text), combatant-token.ts:56 (dot). All LABEL (grep).
- Effect log words for 3 conditions only: hp-effects.ts:118-120 (unconscious "adormece", blinded "fica cego", stunned "fica atordoado").
- Reminder text: only concentration. combat_actions.go:1215-1219 concentrationDC; combat_log.go:616-618 (master only). No reminder text exists for any other condition (grep).
- Player can end only own concentration: combat-view.ts:1636-1642 (conditions are master's, comment at :1636).

## Docs claims

- rules.md:329: "It does not apply the effects by itself; the master decides. Applying effects automatically is left for after the MVP." (deliberately not built)
- rules.md:281: speed 0 for grappled and restrained; no movement for paralyzed, petrified, stunned, unconscious. Matches combat_move.go:145.
- rules.md:336: "No effect is applied." (Server section)
- stories.md:311: "Conditions and concentration are only marked and reminded; the master decides the effects (RN-22)." (deliberate)
- stories.md:550: TestRN25_ConditionsThatLeaveNoSpeed listed.
- rules.md:30: "The app marks conditions (prone, poisoned...)" (labels only)
- No doc mentions exhaustion levels, auto-crit, auto-fail saves, or advantage from conditions (grep "exaust|crit|auto" in those docs: none for conditions).

## Tests touching conditions (not exhaustive)
- combat_spells_test.go:1099 TestRN22_ConditionsAndTheConcentrationReminder
- combat_theatre_test.go:1095 TestRN25_ConditionsThatLeaveNoSpeed
- combat_spells_hp_test.go:249, :264, :401 (blinded, poisoned, deafened labels), :193-231 (unconscious via Sleep)
- familiarsight_test.go:22, :301
- combat_fog_test.go:618, :902, :1219
- combat_opportunity_test.go:873
- wildshape_review_test.go:57-92 (druid unconscious)
- Web: conditions-dialog.spec.ts (dialog only), hp-effects.spec.ts (words only)

Unsure items: whether incapacitated/unconscious creatures can still take actions via GetTurnOptions (not traced); whether combat_write.go mayAct checks any condition (grep showed none, not fully read); Sleep/Stun tests beyond the lines listed.
