package play

import (
	"fmt"
)

// The cases of the combat log golden (combat_log_golden_test.go): the events, as JSON the
// way the service writes them. The combatants are Aria (player u1), Bruno (player u2), two
// goblins and a wolf the master keeps hidden.

func logGoldenCases() []logCase {
	return []logCase{
		caseSetupAndBounds(),
		caseMoves(),
		caseSmallKinds(),
		caseResources(),
		caseDeathSaves(),
		caseConditionsAndEffects(),
		caseStatesRidersShapes(),
		caseContests(),
		caseAttacks(),
		caseAttackEdges(),
		caseBare(),
		caseSpells(),
		caseSpellPlaced(),
		caseSpellEdges(),
		caseTraps(),
		caseOpportunity(),
		caseEscudo(),
		caseUndo(),
		caseLasting(),
		caseReactionWindows(),
		caseHeldReactions(),
	}
}

func caseSetupAndBounds() logCase {
	return logCase{name: "setup and bounds", rows: rowsOf([]ev{
		{"s1", eventCombatBegun, p("")},
		{"s2", eventCombatantsAdded, `{"monsters":{"creature_key":"goblin","count":2,"name_hash":"h","rolled":true,"items":[{"id":"npc-gob","hit_points":7,"dice":"2d6","faces":[3,4]},{"id":"npc-lobo","hit_points":11}]}}`},
		{"s3", eventCombatantsAdded, p(`"secret":true`)},
		{"s4", eventActionTaken, `{"combatant_id":"pc-aria","key":"standard:dash"}`},
		{"s5", eventInitiativeSubmitted, p("")},
		{"s6", eventTurnEnded, p("")},
		{"s7", eventActionTaken, `not json`},
		{"s8", eventEncounterEnded, p("")},
	})}
}

func caseMoves() logCase {
	return logCase{name: "moves", rows: rowsOf([]ev{
		{"m1", eventCombatantMoved, p(`"combatant_id":"pc-aria","distance_ft":10,"distance_dft":100`)},
		{"m2", eventCombatantMoved, p(`"combatant_id":"pc-aria","on_turn":true,"distance_ft":15,"distance_dft":155,"spent_dft":155,"move_id":"mv1"`)},
		{"m3", eventCombatantMoved, p(`"combatant_id":"pc-aria","on_turn":true`)},
		{"m4", eventCombatantMoved, p(`"combatant_id":"pc-bruno","on_turn":true,"distance_ft":10,"dragging":true,"crawling":true,"spent_dft":300,"jump":"long","jump_running":true,"landing_difficult":true`)},
		{"m5", eventCombatantMoved, p(`"combatant_id":"pc-aria","on_turn":true,"jump":"high","height_dft":50,"landing_difficult":true`)},
		{"m6", eventCombatantMoved, p(`"combatant_id":"npc-lobo","on_turn":true,"distance_ft":20,"distance_dft":200`)},
		{"m7", eventCombatantMoved, p(`"combatant_id":"pc-aria","contest":{"line":"stood_up"}`)},
		{"m8", eventCombatantMoved, p(`"combatant_id":"pc-aria","on_turn":true,"distance_ft":5,"distance_dft":50,"secret":true`)},
		{"m9", eventCombatantMoved, p(`"combatant_id":"gone","on_turn":true,"distance_ft":5,"distance_dft":50`)},
	})}
}

func caseSmallKinds() logCase {
	return logCase{name: "doors, reveals, actions, hit points, turn parts", rows: rowsOf([]ev{
		{"k1", eventDoorOpened, p(`"combatant_id":"pc-aria","col":3,"row":4`)},
		{"k2", eventCombatantHiddenSet, p(`"combatant_id":"npc-gob","hidden":false`)},
		{"k3", eventCombatantHiddenSet, p(`"combatant_id":"npc-gob","hidden":false,"by_area":true`)},
		{"k4", eventCombatantHiddenSet, p(`"combatant_id":"npc-lobo","hidden":true`)},
		{"k5", eventActionTaken, p(`"combatant_id":"pc-aria","key":"standard:dash"`)},
		{"k6", eventActionTaken, p(`"combatant_id":"pc-aria","key":"feature:second-wind","heal":true,"amount":7,"dice_count":1,"dice_sides":10,"faces":[4],"modifier":3,"total":7,"physical":true`)},
		{"k7", eventActionTaken, p(`"combatant_id":"npc-gob","key":"standard:dodge","heal":true,"amount":3,"dice_count":1,"dice_sides":4,"faces":[3]`)},
		{"k8", eventHitPointsAdjusted, p(`"combatant_id":"npc-gob","mode":"damage","delta":-3,"after":{"hp":4}`)},
		{"k9", eventHitPointsAdjusted, p(`"combatant_id":"npc-gob2","mode":"heal","delta":2`)},
		{"k10", eventTurnPartEnded, p(`"combatant_id":"pc-aria"`)},
		{"k11", eventTrapSearched, p(`"combatant_id":"pc-aria","key":"perception","d20":12,"modifier":3,"total":15,"found":["pt-pit"]`)},
		{"k12", eventRollModeAnswered, p(`"combatant_id":"pc-aria","suggested_mode":"normal","roll_mode":"advantage","requested_mode":"advantage","request_id":"rq1"`)},
		{"k13", eventRollModeAnswered, p(`"combatant_id":"pc-bruno","suggested_mode":"normal","roll_mode":"disadvantage","requested_mode":"advantage","request_id":"rq1","reason_id":"r1"`)},
		{"k14", eventRollModeAnswered, p(`"combatant_id":"pc-aria","canceled":true`)},
		{"k15", eventDoorOpened, p(`"combatant_id":"pc-aria","col":1,"row":1,"fogged":true,"seen_by":["u2"]`)},
		{"k16", eventDoorOpened, p(`"combatant_id":"pc-aria","col":2,"row":2,"fogged":true,"seen_by":[]`)},
	})}
}

func caseResources() logCase {
	return logCase{name: "class resources", rows: rowsOf([]ev{
		{"r1", eventActionTaken, p(`"combatant_id":"pc-aria","target_id":"pc-bruno","key":"feature:lay-on-hands","res":{"kind":"lay_on_hands","spent":5,"healed":5}`)},
		{"r2", eventActionTaken, p(`"combatant_id":"pc-aria","target_id":"pc-bruno","key":"feature:lay-on-hands","res":{"kind":"lay_on_hands","spent":5,"cure":"poison"}`)},
		{"r3", eventActionTaken, p(`"combatant_id":"pc-aria","target_id":"npc-gob","key":"feature:lay-on-hands","res":{"kind":"lay_on_hands","nothing":true,"why":"undead"}`)},
		{"r4", eventActionTaken, p(`"combatant_id":"pc-aria","key":"feature:flexible-casting","res":{"kind":"create_slot","spent":2,"slot_level":1}`)},
		{"r5", eventActionTaken, p(`"combatant_id":"pc-aria","key":"feature:flexible-casting","res":{"kind":"convert_slot","gained":2,"slot_level":1}`)},
		{"r6", eventActionTaken, p(`"combatant_id":"pc-aria","target_id":"pc-bruno","key":"feature:bardic-inspiration","res":{"kind":"bardic_give","sides":8,"expires_round":3,"spent":1}`)},
		{"r7", eventActionTaken, p(`"combatant_id":"pc-aria","target_id":"npc-gob","key":"feature:bardic-inspiration","res":{"kind":"bardic_give","sides":8,"expires_round":3}`)},
		{"r8", eventActionTaken, p(`"combatant_id":"pc-bruno","target_id":"pc-aria","key":"feature:bardic-inspiration","res":{"kind":"bardic_give","sides":6,"expires_round":2}`)},
		{"r9", eventActionTaken, p(`"combatant_id":"pc-aria","key":"feature:x","res":{"kind":"other"}`)},
	})}
}

func caseDeathSaves() logCase {
	return logCase{name: "death saves, death, revival", rows: rowsOf([]ev{
		{"d1", eventDeathSaveRolled, p(`"combatant_id":"pc-aria","d20":14,"death":{"successes":1},"death_outcome":"success"`)},
		{"d2", eventDeathSaveRolled, p(`"combatant_id":"pc-aria","d20":14,"death":{"successes":1},"death_outcome":"success","death_hidden":true`)},
		{"d3", eventDeathSaveRolled, p(`"combatant_id":"pc-aria","d20":3,"death":{"successes":1,"failures":3},"death_outcome":"failure","death_hidden":true`)},
		{"d4", eventDeathSaveRolled, p(`"combatant_id":"pc-aria","d20":16,"death":{"successes":3},"death_outcome":"success","death_hidden":true`)},
		{"d5", eventDeathSaveRolled, p(`"combatant_id":"pc-aria","d20":10,"death":{"successes":3},"death_outcome":"revived","death_hidden":true`)},
		{"d6", eventDeathSaveRolled, p(`"combatant_id":"pc-aria","d20":1,"death_outcome":"critical_failure"`)},
		{"d7", eventDeathSaveRolled, p(`"combatant_id":"pc-aria","d20":20,"death":{"successes":3},"death_outcome":"success"`)},
		{"d8", eventDeathConfirmed, p(`"combatant_id":"pc-aria"`)},
		{"d9", eventCharacterRevived, p(`"combatant_id":"pc-aria"`)},
	})}
}

func caseConditionsAndEffects() logCase {
	return logCase{name: "conditions and effects that ended", rows: rowsOf([]ev{
		{"c1", eventConditionsSet, p(`"combatant_id":"pc-aria","cond_set":true,"conditions":["condition:prone","condition:poisoned"],"cond_before":["condition:prone"]`)},
		{"c2", eventConditionsSet, p(`"combatant_id":"pc-aria","cond_set":true,"conditions":["condition:prone","condition:poisoned"],"cond_before":["condition:prone","condition:poisoned"],"conc_ended":"spell:bless"}`)},
		{"c3", eventConditionsSet, p(`"combatant_id":"pc-aria","cond_set":true,"conditions":["condition:prone"],"cond_before":["condition:prone"]`)},
		{"c4", eventConditionsSet, p(`"combatant_id":"npc-lobo","cond_set":true,"conditions":["condition:prone"]`)},
		{"c5", eventCombatEffectEnded, p(`"combatant_id":"pc-aria","effect_end":{"effect":"shield"}`)},
		{"c6", eventCombatEffectEnded, p(`"combatant_id":"pc-bruno","effect_end":{"effect":"aid","hp_before":20,"hp_after":15,"max_before":25,"max_after":20}`)},
		{"c7", eventCombatEffectEnded, p(`"combatant_id":"pc-bruno"`)},
	})}
}

func caseStatesRidersShapes() logCase {
	return logCase{name: "states, riders, Wild Shape, damage-less kinds", rows: rowsOf([]ev{
		{"t1", eventStateChanged, p(`"combatant_id":"pc-aria","state_kind":"rage","state_started":true`)},
		{"t2", eventStateChanged, p(`"combatant_id":"pc-aria","state_kind":"hunters_mark_target","target_id":"npc-gob","state_reason":"left"`)},
		{"t3", eventHitRider, p(`"combatant_id":"pc-aria","target_id":"npc-gob","rider":{"choice":"stunning_strike","result":"failed","d20":4,"total":6,"dc":13}`)},
		{"t4", eventHitRider, p(`"combatant_id":"pc-aria","target_id":"pc-bruno","rider":{"choice":"stunning_strike","result":"saved","d20":14,"total":16,"dc":13}`)},
		{"t5", eventHitRider, p(`"combatant_id":"pc-aria","target_id":"npc-gob"`)},
		{"t6", eventWildShapeStarted, p(`"combatant_id":"pc-aria","beast":"wolf"`)},
		{"t7", eventWildShapeEnded, p(`"combatant_id":"pc-aria","beast":"wolf","reason":"damage","carried_damage":4`)},
		{"t8", eventWildShapeEnded, p(`"combatant_id":"pc-aria","beast":"wolf","reason":"left"`)},
	})}
}

func caseContests() logCase {
	lines := []string{"grappled", "grapple_failed", "shove_prone", "shove_pushed", "shove_stays", "shove_failed", "escaped", "escape_failed", "closed", "released", "hide_tried", "hide_applied", "hide_refused", "helped", "surprised", "surprise_cleared"}
	var evs []ev
	for i, line := range lines {
		evs = append(evs, ev{fmt.Sprintf("x%d", i), eventContestResolved, p(`"combatant_id":"pc-aria","target_id":"npc-gob","contest":{"line":"` + line + `"}`)})
	}
	evs = append(evs,
		ev{"x-nolines", eventHideResolved, p(`"combatant_id":"pc-aria","contest":{"purpose":"hide"}`)},
		ev{"x-nocontest", eventHelpGiven, p(`"combatant_id":"pc-aria"`)},
		ev{"x-reveal", eventHideEnded, p(`"combatant_id":"pc-aria","contest":{"line":"hide_revealed"}`)},
		ev{"x-reveal2", eventContestStarted, p(`"combatant_id":"pc-aria","target_id":"npc-lobo","contest":{"line":"hide_revealed"}`)},
		ev{"x-shove", eventShoveResolved, p(`"combatant_id":"pc-bruno","target_id":"pc-aria","contest":{"line":"shove_prone"}`)},
		ev{"x-grapple", eventGrappleReleased, p(`"combatant_id":"pc-bruno","target_id":"npc-lobo","contest":{"line":"released"}`)},
		ev{"x-attempt", eventHideAttempted, p(`"combatant_id":"pc-bruno","contest":{"line":"hide_tried"}`)},
		ev{"x-closed", eventContestClosed, p(`"combatant_id":"pc-bruno","contest":{"line":"closed"}`)},
	)
	return logCase{name: "contests (grapple, shove, hide, help, surprise)", rows: rowsOf(evs)}
}

// attackJSON is an attack roll event.
func attackJSON(actor, target, rest string) string {
	return p(`"combatant_id":"` + actor + `","target_id":"` + target + `","key":"weapon:longsword",` + rest)
}

func caseAttacks() logCase {
	return logCase{name: "attacks and their damage", rows: rowsOf([]ev{
		// A: a hit with the d20 pair, a mode change, a reason, Lucky, extra dice, cover and armor class; the NPC takes the damage at once.
		{"a1", eventAttackRolled, attackJSON("pc-aria", "npc-gob", `"pending_id":"p1","d20":15,"d20_b":9,"counted":0,"modifier":5,"total":20,"outcome":"hit","target_ac":16,"cover":"half","cover_source":"map","cover_bonus":2,`+
			`"suggested_mode":"normal","roll_mode":"advantage","reason_id":"r1","by_master":true,"lucky":{"index":0,"from":1,"to":15},"extra":[{"key":"effect:bless","faces":4,"sign":1,"face":3},{"key":"effect:bless","faces":4,"sign":-1,"face":2,"hidden":true}]`)},
		{"a1d", eventDamageRolled, p(`"pending_id":"p1","combatant_id":"pc-aria","target_id":"npc-gob","amount":9,"damage_type":"slashing","dice_count":3,"dice_sides":8,"faces":[4,3,2],"extra_dice":1,"savage_dice":1,"modifier":3,"critical":true,"critical_max":4,` +
			`"shown":12,"concentration_dc":10,"after":{"hp":0,"defeated":false},"applied":true,"parts":[{"key":"weapon","label":"Espada","count":2,"sides":8,"faces":[4,3],"flat":3,"sum":10,"counted":true,"damage_type":"slashing"},` +
			`{"key":"smite","label":"Destruição","count":1,"sides":8,"faces":[2],"sum":2,"conditional":true,"counted":false,"rider":"vs mortos-vivos","resource":"slot"}],` +
			`"steps":[{"damage_type":"slashing","before":12,"Mods":[{"Kind":"resistance","Source":"monster:goblin","DamageTypes":["slashing"]}]}]`)},
		// B: a critical hit on a player: the master applies other than the roll, a death save failure, Relentless Endurance.
		{"a2", eventAttackRolled, attackJSON("npc-gob", "pc-bruno", `"pending_id":"p2","d20":19,"modifier":4,"total":23,"outcome":"critical","target_ac":15,"crit_on_hit":true`)},
		{"a2d", eventDamageRolled, p(`"pending_id":"p2","combatant_id":"npc-gob","target_id":"pc-bruno","amount":8,"damage_type":"piercing","dice_count":2,"dice_sides":6,"faces":[4,4],"modifier":2,"shown":8,"parts":[{"key":"weapon","count":2,"sides":6,"faces":[4,4],"flat":2,"sum":10,"counted":true}],"steps":[{"damage_type":"piercing","before":10,"Mods":[{"Kind":"resistance","Source":"race:x","DamageTypes":["piercing"]}]}]`)},
		{"a2a", eventDamageApplied, p(`"pending_id":"p2","combatant_id":"pc-bruno","target_id":"pc-bruno","amount":5,"overridden":true,"rolled":8,"relentless":true,"failures_added":1,"concentration_dc":10,"after":{"hp":0},"ignored":["race:x"]`)},
		{"a2r", eventDamagePartRemoved, p(`"pending_id":"p2","combatant_id":"npc-gob","key":"weapon","amount":4,"after":{"hp":0}`)},
		// C: the master applies damage on a character and the table hides the death saves.
		{"a3", eventAttackRolled, attackJSON("npc-gob", "pc-aria", `"pending_id":"p3","d20":12,"modifier":4,"total":16,"outcome":"hit","target_ac":15`)},
		{"a3d", eventDamageRolled, p(`"pending_id":"p3","combatant_id":"npc-gob","target_id":"pc-aria","amount":6,"damage_type":"fire","dice_count":1,"dice_sides":6,"faces":[2],"modifier":4,"concentration_dc":10`)},
		{"a3a", eventDamageApplied, p(`"pending_id":"p3","combatant_id":"pc-aria","target_id":"pc-aria","amount":6,"failures_added":2,"death_hidden":true,"concentration_dc":10,"after":{"hp":0}`)},
		// D: damage rolled, not yet applied, then discarded.
		{"a4", eventAttackRolled, attackJSON("pc-bruno", "pc-aria", `"pending_id":"p4","d20":17,"modifier":3,"total":20,"outcome":"hit"`)},
		{"a4d", eventDamageRolled, p(`"pending_id":"p4","combatant_id":"pc-bruno","target_id":"pc-aria","amount":5,"damage_type":"cold","dice_count":1,"dice_sides":8,"faces":[3],"modifier":2,"concentration_dc":10`)},
		{"a4x", eventDamageDiscarded, p(`"pending_id":"p4","combatant_id":"pc-bruno"`)},
		// E: the attack waits for its damage roll.
		{"a5", eventAttackRolled, attackJSON("pc-aria", "npc-gob2", `"pending_id":"p5","d20":18,"modifier":5,"total":23,"outcome":"hit","target_ac":14`)},
		// F: a miss, no damage; with a requested roll mode.
		{"a6", eventAttackRolled, attackJSON("pc-aria", "npc-gob2", `"d20":3,"modifier":5,"total":8,"outcome":"miss","suggested_mode":"advantage","roll_mode":"normal","request_id":"rq1","physical":true`)},
		// G: a player attacks a hit NPC the damage of which waits and then is rolled for a character taking it at once (the NPC's hit points after).
		{"a7", eventAttackRolled, attackJSON("pc-bruno", "npc-gob", `"pending_id":"p7","d20":14,"modifier":5,"total":19,"outcome":"hit","target_ac":16`)},
		{"a7d", eventDamageRolled, p(`"pending_id":"p7","combatant_id":"pc-bruno","target_id":"npc-gob","amount":7,"shown":9,"damage_type":"bludgeoning","dice_count":1,"dice_sides":8,"faces":[4],"modifier":3,"applied":true,"after":{"hp":0,"defeated":true},"parts":[{"key":"weapon","count":1,"sides":8,"faces":[4],"flat":3,"sum":7,"counted":true}],"concentration_dc":0`)},
		{"a8", eventAttackRolled, attackJSON("pc-aria", "npc-gob2", `"pending_id":"p8","d20":16,"modifier":5,"total":21,"outcome":"hit"`)},
		{"a8d", eventDamageRolled, p(`"pending_id":"p8","combatant_id":"pc-aria","target_id":"npc-gob2","amount":6,"shown":9,"damage_type":"slashing","dice_count":1,"dice_sides":8,"faces":[6],"modifier":0`)},
		{"a8x", eventReactionDeclined, p(`"pending_id":"p8","combatant_id":"npc-gob2"`)},
		// H: a roll with the master as the only reader of the removal reason.
		{"a7r", eventDamagePartRemoved, p(`"pending_id":"p7","combatant_id":"pc-bruno","key":"weapon","amount":3,"reason_id":"r1"`)},
	})}
}

// caseBare is the attacks again, read with no reasons and requests tables and no names.
func caseBare() logCase {
	c := caseAttacks()
	c.name, c.bare = "attacks read without the combat's tables", true
	return c
}

func caseAttackEdges() logCase {
	return logCase{name: "attack edges: hidden, fogged, gone, retaken", rows: rowsOf([]ev{
		{"e1", eventAttackRolled, attackJSON("pc-aria", "npc-lobo", `"pending_id":"q1","d20":11,"modifier":5,"total":16,"outcome":"hit","secret":true`)},
		{"e2", eventAttackRolled, attackJSON("npc-lobo", "pc-aria", `"d20":11,"modifier":5,"total":16,"outcome":"miss"`)},
		{"e3", eventAttackRolled, attackJSON("pc-aria", "npc-gob", `"d20":11,"modifier":5,"total":16,"outcome":"hit","fogged":true,"seen_by":["u1"]`)},
		{"e4", eventAttackRolled, attackJSON("pc-aria", "npc-gob", `"d20":11,"modifier":5,"total":16,"outcome":"hit","fogged":true,"seen_by":[]`)},
		{"e5", eventAttackRolled, attackJSON("gone", "npc-gob", `"d20":11,"modifier":5,"total":16,"outcome":"hit"`)},
		{"e6", eventAttackRolled, attackJSON("pc-aria", "gone", `"d20":11,"modifier":5,"total":16,"outcome":"hit"`)},
		{"e7", eventDamageRolled, p(`"pending_id":"nowhere","amount":3`)},
		{"e8", eventDamageApplied, p(`"pending_id":"nowhere","amount":3`)},
		{"e9", eventDamageDiscarded, p(`"pending_id":"nowhere"`)},
		{"e10", eventDamagePartRemoved, p(`"pending_id":"nowhere","combatant_id":"pc-aria","key":"weapon","amount":3`)},
		{"e11", eventAttackRolled, attackJSON("pc-bruno", "npc-gob", `"pending_id":"q2","d20":11,"d20_b":14,"counted":1,"modifier":5,"total":19,"outcome":"hit","as_reaction":true,"cover":"total","cover_source":"mark","cover_restricted":true,"cover_seen_by":["u1"]`)},
		{"e12", eventDamageRolled, p(`"pending_id":"q2","combatant_id":"pc-bruno","target_id":"npc-gob","amount":4,"damage_type":"fire","dice_count":1,"dice_sides":6,"faces":[2],"modifier":2,"returned_from":{"col":2},"return_blocked":true,"after":{"hp":3}`)},
		{"e13", eventDamageApplied, p(`"pending_id":"q2","combatant_id":"pc-bruno","target_id":"npc-gob","amount":4,"returned_from":{"col":2},"after":{"hp":3}`)},
	})}
}

// castJSON is a spell cast event with the given hits.
func castJSON(actor, rest string) string {
	return p(`"combatant_id":"` + actor + `","key":"spell:fireball","slot":{"level":3},` + rest)
}

func caseSpells() logCase {
	return logCase{name: "a spell with several targets", rows: rowsOf([]ev{
		{"sp1", eventSpellCast, castJSON("pc-aria", `"cast_id":"c1","concentrate":true,"conc_ended":"spell:bless","material":true,"cover_users":["u1","u2"],`+
			`"hits":[`+
			`{"target_id":"npc-gob","pending_id":"sp-a","more_pending_ids":["sp-b"],"save":{"d20":8,"bonus":2,"total":10,"dc":14},"cover":"half","cover_source":"map","cover_bonus":2,"target_ac":0},`+
			`{"target_id":"npc-gob2","pending_id":"sp-c","outcome":"hit","d20":13,"d20_b":4,"counted":0,"modifier":5,"total":18,"target_ac":15,"fogged":true,"seen_mask":1},`+
			`{"target_id":"pc-bruno","pending_id":"sp-d","save":{"d20":15,"bonus":1,"total":16,"dc":14,"saved":true},"darts":3},`+
			`{"target_id":"pc-aria","pending_id":"sp-g","outcome":"miss","d20":2,"modifier":5,"total":7}]`)},
		{"sp1d", eventDamageRolled, p(`"pending_id":"sp-a","combatant_id":"pc-aria","damage_type":"fire","dice_count":8,"dice_sides":6,"faces":[1,2,3,4,5,6,1,2],"total":24,"critical":true,"critical_max":3,"critical_max_rule":true,` +
			`"settled":[{"pending_id":"sp-a","target_id":"npc-gob","amount":24,"shown":24,"applied":true,"after":{"hp":0,"defeated":true},"concentration_dc":12},` +
			`{"pending_id":"sp-b","target_id":"npc-gob","amount":4,"applied":true,"after":{"hp":0}},` +
			`{"pending_id":"sp-c","target_id":"npc-gob2","amount":12,"half":true,"shown":24},` +
			`{"pending_id":"sp-d","target_id":"pc-bruno","amount":12,"half":true,"shown":24}]`)},
		{"sp1a", eventDamageApplied, p(`"pending_id":"sp-d","combatant_id":"pc-aria","target_id":"pc-bruno","amount":9,"overridden":true,"rolled":12,"relentless":true,"failures_added":1,"death_hidden":true,"concentration_dc":10,"after":{"hp":0}`)},
		{"sp1x", eventDamageDiscarded, p(`"pending_id":"sp-c","combatant_id":"pc-aria","concentration_dc":10`)},
		{"sp1x2", eventDamageDiscarded, p(`"pending_id":"sp-g","combatant_id":"pc-aria"`)},
	})}
}

func caseSpellPlaced() logCase {
	return logCase{name: "a spell on an area the server placed", rows: rowsOf([]ev{
		{"sq1", eventSpellCast, castJSON("pc-aria", `"placed":true,"cover_users":["u1","u2"],"hits":[`+
			`{"target_id":"npc-lobo","pending_id":"sq-a","hidden_at_cast":true},`+
			`{"target_id":"npc-gob","pending_id":"sq-b","hidden_at_cast":true},`+
			`{"target_id":"gone","pending_id":"sq-c"},`+
			`{"target_id":"npc-gob2","pending_id":"sq-d","fogged":true,"seen_mask":1,"outcome":"hit","d20":9,"modifier":5,"total":14,"lucky":{"index":0,"from":1,"to":9}},`+
			`{"target_id":"pc-bruno","pending_id":"sq-e","save":{"d20":5,"total":5,"dc":14}}]`)},
		{"sq1d", eventDamageRolled, p(`"pending_id":"sq-b","combatant_id":"pc-aria","heal":true,"damage_type":"fire","dice_count":2,"dice_sides":6,"faces":[2,3],"total":5,"settled":[` +
			`{"pending_id":"sq-b","target_id":"npc-gob","amount":3,"shown":5,"applied":true,"after":{"hp":9}},` +
			`{"pending_id":"sq-e","target_id":"pc-bruno","amount":5,"shown":5}]`)},
	})}
}

func caseSpellEdges() logCase {
	return logCase{name: "spell edges: placed area, revive, heal, pool, metamagic, Escudo, secret", rows: rowsOf([]ev{
		{"se1", eventSpellCast, castJSON("pc-aria", `"placed":true,"hits":[{"target_id":"npc-lobo","pending_id":"se-a","hidden_at_cast":true},{"target_id":"npc-gob","pending_id":"se-b"}]`)},
		{"se2", eventSpellCast, p(`"combatant_id":"pc-aria","key":"spell:revivify","slot":{"level":3},"fx_kind":"revive","material":true,"hits":[{"target_id":"pc-bruno","death_round":2,"fx":"affected","pending_id":"se-c"}]`)},
		{"se2d", eventDamageRolled, p(`"pending_id":"se-c","combatant_id":"pc-aria","heal":true,"damage_type":"healing","dice_count":2,"dice_sides":4,"faces":[2,3],"modifier":2,"total":7,"settled":[{"pending_id":"se-c","target_id":"pc-bruno","amount":5,"applied":true,"after":{"hp":5}}]`)},
		{"se3", eventSpellCast, p(`"combatant_id":"pc-aria","key":"spell:bless","slot":{"level":1,"pact":true},"fx_kind":"hp_pool","dice_count":5,"dice_sides":8,"faces":[1,2,3,4,5],"total":15,"modifier":0,"fx_limit":20,"fx_condition":"condition:asleep","res":{"kind":"metamagic","keys":["quickened"],"spent":2},"hits":[{"target_id":"npc-gob","fx":"affected"}]`)},
		{"se4", eventSpellCast, p(`"combatant_id":"npc-gob","key":"spell:fireball","slot":{"level":2},"hits":[{"target_id":"pc-bruno","pending_id":"se-d","save":{"d20":4,"total":5,"dc":13}},{"target_id":"pc-aria","pending_id":"se-e","outcome":"hit","d20":12,"total":17}]`)},
		{"se4r", eventReactionUsed, p(`"combatant_id":"pc-bruno","pending_id":"se-d","stopped":true,"slot":{"level":1}`)},
		{"se4r2", eventReactionUsed, p(`"combatant_id":"pc-aria","pending_id":"se-e","stopped":false,"also_stopped":[{"pending":"se-e","prev_status":"rolled"},{"pending":"unknown","prev_status":"rolled"}],"slot":{"level":1}`)},
		{"se5", eventSpellCast, p(`"combatant_id":"npc-gob","key":"spell:fireball","secret":true,"hits":[{"target_id":"pc-aria"}]`)},
		{"se6", eventSpellCast, p(`"combatant_id":"npc-gob","key":"spell:fireball","hits":[{"target_id":"npc-lobo"}]`)},
		{"se7", eventSpellCast, p(`"combatant_id":"pc-aria","key":"spell:fireball","fogged":true,"seen_by":["u2"],"hits":[]`)},
	})}
}

func caseTraps() logCase {
	return logCase{name: "traps", rows: rowsOf([]ev{
		{"tr1", eventTrapTriggered, p(`"trap":{"point_id":"pt-pit","map_id":"m1","prev_state":"armed","caught":[` +
			`{"target_id":"npc-gob","attacks":[{"outcome":"hit","d20":12,"d20_b":5,"counted":0,"modifier":4,"total":16,"target_ac":14,"mode":"advantage","sources":[{"k":"condition","e":1,"c":"condition:prone"}]}],"damages":[{"amount":1,"damage_type":"fire","roll_total":1},{"pending_id":"tp1","target_id":"npc-gob","amount":5,"applied":true,"damage_type":"fire","roll_total":5}]},` +
			`{"target_id":"pc-aria","player":true,"character_id":"char-aria","saves":[{"d20":6,"bonus":2,"total":8,"dc":13}],"damages":[{"pending_id":"tp2","target_id":"pc-aria","amount":7,"damage_type":"fire","roll_total":7,"dice_count":2,"dice_sides":6,"faces":[3,4]}],"conditions":["condition:prone"]},` +
			`{"target_id":"npc-lobo","hidden":true,"damages":[{"pending_id":"tp3","target_id":"npc-lobo","amount":2,"damage_type":"fire","roll_total":2}]},` +
			`{"target_id":"npc-gob2","fogged":true,"seen_by":["u1"],"damages":[]}]}`)},
		{"tr1x", eventTrapTriggered, p(`"trap":{"point_id":"pt-pit","map_id":"m1","prev_state":"armed","extends_id":"tr1","caught":[{"target_id":"pc-bruno","damages":[{"pending_id":"tp4","target_id":"pc-bruno","amount":3,"applied":true,"damage_type":"fire","roll_total":3}]},{"target_id":"npc-lobo","damages":[{}]}]}`)},
		{"tr1xx", eventTrapTriggered, p(`"trap":{"point_id":"pt-pit","map_id":"m1","prev_state":"armed","extends_id":"nowhere","caught":[]}`)},
		{"tr1a", eventDamageApplied, p(`"pending_id":"tp2","combatant_id":"pc-aria","target_id":"pc-aria","amount":7,"after":{"hp":3}`)},
		{"tr1b", eventDamageApplied, p(`"pending_id":"tp4","combatant_id":"pc-aria","target_id":"pc-bruno","amount":3,"after":{"hp":3}`)},
		{"tr2", eventTrapTriggered, p(`"secret":true`)},
		{"tr2b", eventTrapTriggered, p(`"trap":{"point_id":"pt-pit","map_id":"m1","prev_state":"armed","caught":[{"target_id":"pc-bruno","player":true,"character_id":"char-bruno"}]}`)},
		{"tr3", eventTrapTriggered, p(`"trap":{"point_id":"pt-gone","map_id":"m1","prev_state":"armed","manual":true}`)},
	})}
}

func caseOpportunity() logCase {
	return logCase{name: "opportunity offers and their answers", rows: rowsOf([]ev{
		{"o1", eventCombatantMoved, p(`"combatant_id":"pc-aria","on_turn":true,"distance_ft":10,"distance_dft":100,"move_id":"mv1"`)},
		{"o1o", eventOpportunityOffered, p(`"combatant_id":"pc-aria","target_id":"npc-gob","offer_id":"of1","move_id":"mv1","col":2,"row":2`)},
		{"o1d", eventReactionDeclined, p(`"offer_id":"of1","combatant_id":"npc-gob"`)},
		{"o2o", eventOpportunityOffered, p(`"combatant_id":"pc-aria","target_id":"pc-bruno","offer_id":"of2","move_id":"mv2","by_hand":true`)},
		{"o2d", eventReactionDeclined, p(`"offer_id":"of2","combatant_id":"pc-bruno"`)},
		{"o3o", eventOpportunityOffered, p(`"combatant_id":"npc-gob","target_id":"pc-bruno","offer_id":"of3","move_id":"mv3","by_hand":true`)},
		{"o4o", eventOpportunityOffered, p(`"combatant_id":"npc-gob","target_id":"npc-gob2","offer_id":"of4","move_id":"mv4","by_hand":true`)},
		{"o5", eventReactionDeclined, p(`"offer_id":"","combatant_id":"npc-gob"`)},
		{"o6", eventReactionDeclined, p(`"offer_id":"nowhere","combatant_id":"npc-gob"`)},
	})}
}

func caseEscudo() logCase {
	return logCase{name: "Escudo and the reaction line of an attack", rows: rowsOf([]ev{
		{"h1", eventAttackRolled, attackJSON("npc-gob", "pc-aria", `"pending_id":"h-p1","d20":13,"modifier":4,"total":17,"outcome":"hit"`)},
		{"h1r", eventReactionUsed, p(`"combatant_id":"pc-aria","target_id":"npc-gob","pending_id":"h-p1","key":"spell:shield","slot":{"level":1},"stopped":true`)},
		{"h2", eventAttackRolled, attackJSON("npc-lobo", "pc-bruno", `"pending_id":"h-p2","d20":13,"modifier":4,"total":17,"outcome":"hit","attacker_hidden":true`)},
		{"h2r", eventReactionUsed, p(`"combatant_id":"pc-bruno","target_id":"npc-lobo","pending_id":"h-p2","key":"spell:shield","slot":{"level":1},"attacker_hidden":true,"stopped":true`)},
		{"h3r", eventReactionUsed, p(`"combatant_id":"pc-bruno","target_id":"npc-lobo","key":"spell:shield","slot":{"level":1}`)},
		{"h4r", eventReactionUsed, p(`"combatant_id":"pc-bruno","target_id":"gone","key":"spell:shield"`)},
		{"h5r", eventReactionUsed, p(`"combatant_id":"gone","target_id":"npc-gob","key":"spell:shield"`)},
		{"h6r", eventReactionUsed, p(`"combatant_id":"pc-bruno","target_id":"npc-gob2","key":"spell:shield","fogged":true,"seen_by":["u1"]`)},
	}), last: "h1r"}
}

func caseUndo() logCase {
	return logCase{name: "undone events leave no trace", rows: rowsOf([]ev{
		{"u1", eventAttackRolled, attackJSON("pc-aria", "npc-gob", `"pending_id":"u-p1","d20":15,"modifier":5,"total":20,"outcome":"hit"`)},
		{"u1d", eventDamageRolled, p(`"pending_id":"u-p1","combatant_id":"pc-aria","target_id":"npc-gob","amount":5,"damage_type":"fire","dice_count":1,"dice_sides":6,"faces":[3],"modifier":2`)},
		{"u2", eventActionTaken, p(`"combatant_id":"pc-aria","key":"standard:dash"`)},
		{"u3", eventDoorOpened, p(`"col":1,"row":1`)},
		{"u4", eventActionTaken, p(`"combatant_id":"pc-bruno","key":"standard:dash"`)},
		{"uu1", eventActionUndone, p(`"undone_id":"u1d","undone_also":["u2"]`)},
		{"uu2", eventActionUndone, p(`"undone_id":"u3"`)},
		{"uu3", eventActionUndone, `bad`},
	}), last: "u1"}
}

func caseLasting() logCase {
	var evs []ev
	for i, change := range []string{"added", "ended", "duration", "saved", "failed", "damage", "skipped", "temp_hp"} {
		kind := eventLastingChanged
		switch change {
		case "added":
			kind = eventLastingAdded
		case "ended":
			kind = eventLastingEnded
		case "saved", "failed":
			kind = eventLastingSaved
		case "damage":
			kind = eventLastingTriggered
		}
		evs = append(evs, ev{fmt.Sprintf("l%d", i), kind, p(`"combatant_id":"pc-aria","lasting":{"key":"effect:bless","change":"` + change + `","targets":["pc-bruno","npc-lobo","npc-gob"],"ability":"dex","d20":9,"total":12,"dc":14,"amount":4,"damage_type":"fire","reason":"x"}`)})
	}
	evs = append(evs,
		ev{"l-owner", eventLastingAdded, p(`"combatant_id":"pc-aria","lasting":{"key":"effect:bless","change":"added","targets":["pc-bruno"],"owner_only":true}`)},
		ev{"l-visibility", eventLastingVisibility, p(`"combatant_id":"pc-aria","lasting":{"key":"effect:bless","change":"visibility","targets":["pc-bruno"]}`)},
		ev{"l-exhaustion", eventExhaustion, p(`"combatant_id":"pc-aria","lasting":{"key":"condition:exhaustion","change":"added","targets":["pc-bruno","pc-aria"],"level":2,"before":1,"owner_only":true,"reason":"x"}`)},
		ev{"l-nolasting", eventLastingEnded, p(`"combatant_id":"pc-aria"`)},
		ev{"l-lucky", eventLastingSaved, p(`"combatant_id":"pc-aria","lucky":{"index":0,"from":1,"to":9},"lasting":{"key":"effect:bless","change":"saved","targets":["pc-aria"],"d20":9,"total":9}`)},
		ev{"l-secret", eventLastingAdded, p(`"combatant_id":"npc-lobo","secret":true,"lasting":{"key":"effect:bless","change":"added","targets":["npc-lobo"]}`)},
	)
	return logCase{name: "effects that last", rows: rowsOf(evs)}
}

// reactor and other are the combatants of a reaction window line.
type reactionPair struct{ reactor, other string }

func caseReactionWindows() logCase {
	pairs := []reactionPair{{"pc-aria", "npc-gob"}, {"npc-gob", "pc-aria"}, {"npc-lobo", "pc-aria"}, {"pc-aria", "npc-lobo"}, {"pc-bruno", ""}, {"gone", "pc-aria"}}
	reactions := []struct{ name, kind, js string }{
		{"cs-countered-check", eventReactionAnswered, `"kind":"counterspell","used":true,"countered":true,"spell":"spell:fireball","level":3,"slot":{"level":3},"check":14,"check_dc":13`},
		{"cs-countered-nocheck", eventReactionAnswered, `"kind":"counterspell","used":true,"countered":true,"spell":"spell:fireball","level":3,"slot":{"level":3},"by_master":true`},
		{"cs-notcountered", eventReactionAnswered, `"kind":"counterspell","used":true,"spell":"spell:fireball","level":3,"slot":{"level":3},"check":8,"check_dc":13`},
		{"cs-unused", eventReactionAnswered, `"kind":"counterspell","used":false,"spell":"spell:fireball"`},
		{"cs-nospell", eventReactionAnswered, `"kind":"counterspell","used":true,"countered":true,"spell":"unknown:key","level":2`},
		{"ud", eventReactionAnswered, `"kind":"uncanny_dodge","used":true,"before":10,"after":5,"by_master":true`},
		{"mr-die", eventReactionAnswered, `"kind":"maneuver_reduce","used":true,"maneuver_name":"Aparar","die":4,"die_sides":8,"before":9,"after":5`},
		{"mr-noname", eventReactionAnswered, `"kind":"maneuver_reduce","used":true`},
		{"df-die", eventReactionAnswered, `"kind":"deflect_missiles","used":true,"die":6,"before":9,"after":3,"caught":true`},
		{"df-die-nocatch", eventReactionAnswered, `"kind":"deflect_missiles","used":true,"die":6,"before":9,"after":3`},
		{"df-catch", eventReactionAnswered, `"kind":"deflect_missiles","used":true,"die":0,"caught":true`},
		{"df-none", eventReactionAnswered, `"kind":"deflect_missiles","used":true`},
		{"cw-attack-hit", eventReactionAnswered, `"kind":"cutting_words","used":true,"roll":"attack","die":4,"die_sides":6,"before":17,"after":13,"ac":14,"outcome":"hit"`},
		{"cw-attack-miss", eventReactionAnswered, `"kind":"cutting_words","used":true,"die":4,"die_sides":6,"before":17,"after":13,"ac":14,"outcome":"miss"`},
		{"cw-damage", eventReactionAnswered, `"kind":"cutting_words","used":true,"roll":"damage","die":3,"die_sides":6,"before":9,"after":6`},
		{"cw-test", eventReactionAnswered, `"kind":"cutting_words","used":true,"roll":"test","die":3,"die_sides":6,"before":14,"after":11,"by_master":true`},
		{"cw-ineffect", eventReactionAnswered, `"kind":"cutting_words","used":true,"ineffect":true,"die":3,"die_sides":6,"by_master":true`},
		{"hr-failed", eventReactionAnswered, `"kind":"hellish_rebuke","used":true,"slot":{"level":2},"level":2,"save_d20":7,"save_bonus":2,"save_dc":13,"fire":9`},
		{"hr-saved", eventReactionAnswered, `"kind":"hellish_rebuke","used":true,"racial":true,"level":3,"save_d20":15,"save_bonus":2,"save_dc":13,"saved":true,"fire":5,"by_master":true`},
		{"hr-nodc", eventReactionAnswered, `"kind":"hellish_rebuke","used":true`},
		{"ff", eventReactionAnswered, `"kind":"feather_fall","used":true,"saved_ids":["pc-aria","npc-lobo","pc-bruno"]`},
		{"ff-one", eventReactionAnswered, `"kind":"feather_fall","used":true,"saved_ids":["pc-bruno"]`},
		{"ff-none", eventReactionAnswered, `"kind":"feather_fall","used":true,"saved_ids":["npc-lobo"]`},
		{"conc-kept", eventConcentrationSaveRolled, `"kind":"concentration_save","kept":true,"spell":"spell:bless","save_d20":12,"save_bonus":3,"save_dc":10,"by_master":true`},
		{"conc-kept-nod20", eventConcentrationSaveRolled, `"kind":"concentration_save","kept":true,"spell":"spell:bless"`},
		{"conc-failed", eventConcentrationSaveRolled, `"kind":"concentration_save","spell":"spell:bless","save_d20":4,"save_bonus":3,"save_dc":10`},
		{"master-check", eventReactionAnswered, `"kind":"master_check","used":true`},
		{"unknown-kind", eventReactionAnswered, `"kind":"other","used":true`},
		{"shield-kind", eventReactionAnswered, `"kind":"shield","used":true`},
	}
	var evs []ev
	for _, r := range reactions {
		for i, pr := range pairs {
			evs = append(evs, ev{fmt.Sprintf("%s/%d", r.name, i), r.kind, p(`"combatant_id":"` + pr.reactor + `","target_id":"` + pr.other + `","reaction":{` + r.js + `}`)})
		}
	}
	evs = append(evs,
		ev{"conc-lucky", eventConcentrationSaveRolled, p(`"combatant_id":"pc-aria","lucky":{"index":0,"from":1,"to":12},"reaction":{"kind":"concentration_save","kept":true,"spell":"spell:bless","save_d20":12,"save_bonus":3,"save_dc":10}`)},
		ev{"hr-lucky", eventReactionAnswered, p(`"combatant_id":"pc-aria","target_id":"npc-gob","lucky":{"index":0,"from":1,"to":12},"reaction":{"kind":"hellish_rebuke","used":true,"level":1,"save_d20":12,"save_bonus":3,"save_dc":10,"fire":4}`)},
		ev{"fog-1", eventReactionAnswered, p(`"combatant_id":"npc-gob","target_id":"pc-aria","fogged":true,"seen_by":["u2"],"reaction":{"kind":"uncanny_dodge","used":true,"before":8,"after":4}`)},
		ev{"fog-2", eventReactionAnswered, p(`"combatant_id":"pc-aria","target_id":"npc-gob","fogged":true,"seen_by":[],"reaction":{"kind":"uncanny_dodge","used":true,"before":8,"after":4}`)},
		ev{"secret-1", eventReactionAnswered, p(`"combatant_id":"npc-gob","target_id":"pc-aria","secret":true,"reaction":{"kind":"uncanny_dodge","used":true,"before":8,"after":4}`)},
		ev{"cs-unused-conc", eventConcentrationSaveRolled, p(`"combatant_id":"pc-aria","target_id":"npc-gob","reaction":{"kind":"counterspell","used":false,"spell":"spell:fireball"}`)},
		ev{"noreaction", eventReactionAnswered, p(`"combatant_id":"pc-aria"`)},
	)
	return logCase{name: "reaction windows answered", rows: rowsOf(evs)}
}

func caseHeldReactions() logCase {
	var evs []ev
	for i, roll := range []string{"cast", "attack", "damage", "other"} {
		evs = append(evs, ev{fmt.Sprintf("hd%d", i), eventReactionAnswered, p(`"combatant_id":"pc-aria","secret":true,"reaction":{"kind":"held_dropped","used":true,"roll":"` + roll + `","why":"o alvo saiu do alcance"}`)})
	}
	evs = append(evs,
		ev{"hd-gone", eventReactionAnswered, p(`"combatant_id":"gone","reaction":{"kind":"held_dropped","used":true,"roll":"cast","why":"x"}`)},
		ev{"hold", eventReactionAnswered, p(`"combatant_id":"pc-aria","reaction":{"hold":true,"kind":"counterspell","used":true}`)},
	)
	return logCase{name: "held actions", rows: rowsOf(evs)}
}
