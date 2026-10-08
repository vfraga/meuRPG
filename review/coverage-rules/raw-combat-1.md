# raw-combat-1: Combat part 1 (the-order-of-combat, movement-and-position, actions-in-combat)

Scope: SRD 5.1 sections the-order-of-combat (combat-step-by-step, surprise, initiative, your-turn, reactions), movement-and-position (breaking-up-your-move, difficult-terrain, being-prone, moving-around-other-creatures, flying-movement, creature-size, interacting-with-objects-around-you), actions-in-combat (attack, cast-a-spell, dash, disengage, dodge, help, hide, ready, search, use-an-object, improvised actions). Grapple and shove are not in this chapter.

Key fact for the actions block: TakeAction (backend/internal/play/combat_actions.go:1623) handles every standard action except standard:attack and standard:cast-a-spell (refused at combat_actions.go:1644-1646). Generic path: economy is spent (combat_actions.go:1725-1734), SetCombatantEconomy writes it (combat_actions.go:1749-1753), and only the keys below get extra state: standard:dash (combat_actions.go:1799-1803) and standard:disengage (combat_actions.go:1806-1810). Everything else only spends the action and writes a log line. Keys not in standard_actions.json are refused (combat_actions.go:1689-1692).

## the-order-of-combat

MECHANIC: the-order-of-combat / round and turn cycle (rounds of ~6 s; each participant takes a turn in initiative order; round ends when all have a turn)
SERVER: backend/internal/play/combat_turn.go:63-81 nextTurnGroup walks the groups in order and returns newRound when it wraps past the last group; combat_turn.go:131-143 setCurrent stores round and current combatant; combat_turn.go:165-189 leaveTurn advances the turn when a member leaves
SCREEN: no match for "rodada" in web/src/app/pages/live-session/combat (not searched further, unsure)
DOCS: docs/product/glossary.md:45 "Encounter ... with initiative, rounds and turns"
TESTS: backend/internal/play/combat_rules_test.go:68 TestMR013_NextTurnSkipsTheDefeatedAndCountsRounds

MECHANIC: combat-step-by-step / 1 determine surprise
SERVER: no match for "surprise|surpresa|surpreend|sorpres" in backend/, web/src/app, docs/ (only monster text such as backend/internal/rules/srd51/data/monsters.json:6333 "Surprise Attack", data not state)
SCREEN: no match for "surpresa|surpreso" in web/src/app/pages/live-session/combat and web/src/app/core/combat
DOCS: no match
TESTS: none

MECHANIC: combat-step-by-step / 2 establish positions (GM places everyone)
SERVER: backend/internal/play/combat_move.go:275 forced move is the master's only ("A forced move (a teleport, a token put right) is the master's, and never provokes"); placement otherwise via addParticipants on the token square, backend/internal/play/combat.go:344-400
SCREEN: web/src/app/pages/live-session/combat/order-list/order-list.html:6 "Adicionar combatente" (start dialog, NPCs only per web/src/app/pages/live-session/combat/combat-view.ts:1833)
DOCS: docs/product/rules.md:276 squares and circle movement (positions on the grid)
TESTS: backend/internal/play/combat_test.go:250 TestMR013_TurnOrderAndMovementLeft

MECHANIC: combat-step-by-step / 3 roll initiative (everyone involved)
SERVER: backend/internal/play/combat.go:389-393 each NPC copy rolls its own d20 + bonus at add time (rollInitiative, combat.go:460); a player's character rolls in SubmitInitiative (backend/internal/play/combat.go:474-596, expr at combat.go:544 d20 + InitiativeBonus); BeginCombat refuses while any combatant has no initiative (backend/internal/play/combat.go:700-710)
SCREEN: no match checked beyond the start dialog; not searched further (unsure)
DOCS: docs/product/glossary.md:47 "Initiative ... Each combatant rolls its own, including identical NPCs (RN-19)"
TESTS: backend/internal/play/combat_test.go:400 TestRN19_EachNPCRollsItsOwnInitiative

MECHANIC: combat-step-by-step / 4 take turns; 5 begin next round
SERVER: backend/internal/play/combat_turn.go:109-126 startTurn clears turns, resets each member (ResetCombatantTurn), sets current; backend/internal/play/combat.go:744 EndTurn ends the part; backend/internal/play/combat.go:671 BeginCombat starts round 1 on first group
SCREEN: web/src/app/pages/live-session/combat/combat-bar/next-turn.ts:38 "Próximo turno" (master's button)
DOCS: docs/product/rules.md:202-204 RN-19 (turn order from initiative)
TESTS: backend/internal/play/combat_test.go:580 TestEndTurnIsIdempotent; backend/internal/play/combat_joint_test.go:139 TestMR013_TheTurnPassesWhenTheLastMemberEnds

MECHANIC: surprise / a surprised creature cannot move or take an action on its first turn
SERVER: no match for "surpris" state; BeginCombat (backend/internal/play/combat.go:671) sets no flag and does not gate the first turn; a combatant's economy starts fresh via ResetCombatantTurn (backend/internal/play/queries.sql:225-233)
SCREEN: no match for "surpresa" in web combat screens
DOCS: no match
TESTS: none

MECHANIC: surprise / a surprised creature cannot take a reaction until its first turn ends
SERVER: no match (no surprise flag; reaction availability only from ReactionUsed, queries.sql:231)
SCREEN: no match
DOCS: no match
TESTS: none

MECHANIC: surprise / surprise decided by Stealth vs passive Perception
SERVER: no match
SCREEN: no match
DOCS: no match
TESTS: none

MECHANIC: initiative / roll: Dexterity check, d20 + modifier per combatant
SERVER: backend/internal/rules/abilities.go:357 initiative = DEX modifier + profBonus(best) (Jack of All Trades); backend/internal/rules/creatures.go:539 creature initiative = DEX modifier; stored as InitiativeBonus, backend/internal/play/combat.go:369 and combat_creatures.go:188
SCREEN: backend/internal/play/combat_view.go:332 shows InitiativeBonus to the master (server); web screen not opened (unsure)
DOCS: docs/product/rules.md:209 identical NPCs roll their own (summon exception)
TESTS: backend/internal/play/combat_test.go:400 TestRN19_EachNPCRollsItsOwnInitiative

MECHANIC: initiative / one roll for an entire group of identical creatures
SERVER: backend/internal/play/combat_creatures.go:126-207 groupRoll: creatures of one group share one roll (summon group); backend/internal/play/combat.go:558-567 SubmitInitiative sets the group total for all members (SetGroupInitiative)
SCREEN: no match checked (unsure)
DOCS: docs/product/rules.md:209 "The one exception is the creatures of a single summon of a player character ... share one initiative roll (summon_group_id)"; docs/product/rules.md:204 "the group does not roll together" (RN-19, NPC copies)
TESTS: backend/internal/play/creatures_test.go:888 TestMR037_ExistingCreaturesJoinACombatAndAGroupSharesOneRoll

MECHANIC: initiative / order from highest total to lowest, same order every round
SERVER: backend/internal/play/combat_rules.go:61-82 orderCombatants sorts by total desc; the order is stored in order_index and not recomputed per round (combat_rules.go:67-81)
SCREEN: web/src/app/pages/live-session/combat/order-list/order-list.html (the order list); not opened line by line (unsure)
DOCS: docs/product/glossary.md:47 (initiative sets turn order)
TESTS: backend/internal/play/combat_rules_test.go:26 TestRN19_OrderByInitiativeThenBonusThenTheMastersPlaces

MECHANIC: initiative / ties: GM decides among monsters, players among their characters; optional d20 re-roll for ties
SERVER: backend/internal/play/combat_rules.go:85-88 sameTie (same total AND same bonus); combat_rules.go:93-103 unresolvedTies; backend/internal/play/combat.go:599-667 SetInitiativeOrder (master only, orders a tied group, sets tie_ordered); no optional d20 tie roll; BeginCombat does not refuse unresolved ties (combat.go:700-710 checks only missing initiative)
SCREEN: backend/internal/play/combat_view.go:249-250 unresolvedTies sent to clients (server); no web match for "empate" in web/src/app/pages/live-session/combat (tieNumbers is number formatting only, opportunity-card.ts:26)
DOCS: no match for empate/desempate
TESTS: backend/internal/play/combat_joint_test.go:606 TestMR013_OrderingATieKeepsTheGroup

MECHANIC: initiative / tie broken by higher Dexterity (SRD text above does not state a Dex tie-break; code uses bonus)
SERVER: backend/internal/play/combat_rules.go:77-78 cmp.Compare(b.InitiativeBonus, a.InitiativeBonus) breaks equal totals by initiative bonus (which is DEX modifier plus Jack of All Trades per backend/internal/rules/abilities.go:357), then order_index (combat_rules.go:79)
SCREEN: no match
DOCS: no match
TESTS: backend/internal/play/combat_rules_test.go:26

MECHANIC: initiative / group of combatants with equal total acts as one joint turn (project rule RN-19/20, not in SRD text)
SERVER: backend/internal/play/combat_turn.go:31-43 groupRuns groups adjacent combatants with the same Initiative TOTAL only (combat_turn.go:35), regardless of InitiativeBonus; a joint turn is then started in startTurn (combat_turn.go:109-126)
SCREEN: web/src/app/pages/live-session/combat/combat-view.ts:1253 area is trap search; joint-turn display documented in web (not opened line by line, unsure)
DOCS: docs/product/rules.md:222 "**Joint turn.** A player sees a group only if it has a player character"
TESTS: backend/internal/play/combat_joint_test.go:87 TestMR013_PlayersWithTheSameInitiativeShareATurn; backend/internal/play/combat_joint_test.go:139 TestMR013_TheTurnPassesWhenTheLastMemberEnds

MECHANIC: initiative / joining a running combat (NPC reinforcements)
SERVER: backend/internal/play/combat.go:991 AddCombatants; NPCs are allowed at any time while not ended (combat.go:1020-1034 uses addParticipants, combat.go:344); each new NPC copy rolls its own initiative (combat.go:389) and is inserted into the order by total; new rows start turn_state 'idle' (queries.sql:168-178 InsertCombatant; migration 00089 default 'idle'), so a newcomer acts from the next time its group comes up, not in the running turn (combat_joint_test.go:362 test)
SCREEN: web/src/app/pages/live-session/combat/order-list/order-list.html:6 "Adicionar combatente"; web/src/app/pages/live-session/combat/start-combat/start-combat-dialog.html:5 "Adicionar combatentes"
DOCS: docs/product/rules.md:209 (identical NPCs roll their own initiative)
TESTS: backend/internal/play/combat_joint_test.go:362 TestMR013_AReinforcementWithTheSameTotalActsFromTheNextTurn; backend/internal/play/combat_monsters_test.go:89 TestMR042_ThreeBanditsJoinTheCombat

MECHANIC: initiative / a player's character joining a running combat
SERVER: backend/internal/play/combat.go:1028-1029 refuses ("a player's character joins only before the combat begins, and once") when status is not setup or the character is already in the combat
SCREEN: no match (start dialog lists NPCs only, combat-view.ts:1833)
DOCS: no match
TESTS: none found

MECHANIC: your-turn / on your turn you can move up to your speed and take one action (either order)
SERVER: movement is not gated by ActionUsed (no ActionUsed check in backend/internal/play/combat_move.go; gate is actsNow/mayAct, combat_actions.go:70-88 and combat_write.go:646-650); action spent at combat_actions.go:1732-1733 and combat_actions.go:1749-1753
SCREEN: web/src/app/pages/live-session/combat/action-groups/action-groups.html:174-179 "Ações padrão" group; economy tiles web/src/app/pages/live-session/combat/action-groups/economy-tiles.ts:11 ("Usada")
DOCS: docs/product/glossary.md:42 "Action economy ... one action, one bonus action, one reaction and movement (the speed, doubled after Dash)"
TESTS: backend/internal/play/combat_test.go:250 TestMR013_TurnOrderAndMovementLeft

MECHANIC: your-turn / bonus action: only when a feature or spell says so; one per turn
SERVER: economy bonus_action spends BonusActionUsed (combat_actions.go:1726-1727); one per turn enforced by economyOption (backend/internal/rules/combat/turn.go:383-384); bonus actions come only from features (backend/internal/rules/actions.go:69-73 featureStandards: Cunning Action, Step of the Wind, Patient Defense; actions.go:161-190 grant_action); bonus action spell limit in turn.go:445-450 SpellLimited
SCREEN: web/src/app/pages/live-session/combat/action-groups/group-state.ts:6 ("Disponível"/"Usada")
DOCS: docs/product/stories.md:293 turn screen groups Ação, Ação bônus, Reação and Movimento
TESTS: backend/internal/play/combat_actions_test.go:2083 TestBonusActionFeaturesTakeTheStandardActionsEffect; backend/internal/play/combat_spells_test.go:2645 TestBonusActionSpellLeavesNoOtherSpellButACantrip

MECHANIC: your-turn / free object interaction (one object during move or action, free)
SERVER: no match for free object interaction (no state tracks it); the only object interaction during a move is opening doors: backend/internal/play/combat_move.go:552 openDoors, called from the move; Use an Object is an action (see actions-in-combat / use-an-object)
SCREEN: no match
DOCS: no match
TESTS: backend/internal/play/combat_doors_test.go:84 TestMR010_AMoveOpensAClosedDoor

MECHANIC: your-turn / communication and other free flourishes
SERVER: no match (nothing recorded; not a rules state)
SCREEN: no match
DOCS: no match
TESTS: none

MECHANIC: your-turn / GM may require an action for an activity
SERVER: no match (GM decision; no code path)
SCREEN: no match
DOCS: no match
TESTS: none

MECHANIC: your-turn / forgo move, action, or anything (end turn early)
SERVER: backend/internal/play/combat.go:744 EndTurn ends the part with no action required; turn passes via leaveTurn/startTurn (combat_turn.go:165-189)
SCREEN: web/src/app/pages/live-session/combat/combat-bar/next-turn.ts:38 "Próximo turno"
DOCS: no match for "encerrar turno" in docs/product (EndTurn exists in proto only as API)
TESTS: backend/internal/play/combat_test.go:580 TestEndTurnIsIdempotent

MECHANIC: reactions / one reaction, used until start of your next turn
SERVER: reaction economy spent at combat_actions.go:1728-1729 (after.ReactionUsed = true) and by an opportunity attack at combat_actions.go:778-779; reset at start of own turn by backend/internal/play/queries.sql:231 ResetCombatantTurn (reaction_used = false), called from combat_turn.go:114-116 startTurn for every member of the group
SCREEN: web/src/app/core/combat/combat-options.ts:45 "Só quando o gatilho acontecer" (REACTION_ONLY reason)
DOCS: docs/product/glossary.md:42 (one reaction)
TESTS: backend/internal/play/combat_spells_test.go:722-723 (reaction and bonus back at start of turn); backend/internal/play/combat_spells_test.go:786 TestOpportunityAttackSpendsTheReaction

MECHANIC: reactions / reaction taken on another creature's turn (interrupts)
SERVER: feature reactions are allowed off turn: combat_actions.go:1697-1699 uses mustReactNowOrOnTurn (combat_actions.go:1843-1861: combat active, not defeated, not down; turn not required); the mover's turn waits on an open opportunity offer (combat_opportunity.go:253 and docs)
SCREEN: web/src/app/pages/live-session/combat/opportunity/opportunity-card.ts:58-59 "Ataque de oportunidade"
DOCS: docs/product/rules.md:315 "The mover's turn waits"
TESTS: backend/internal/play/combat_opportunity_test.go:474 TestMR034_TheMoversTurnWaits

MECHANIC: reactions / after a reaction interrupts, that creature may continue its turn
SERVER: no match (no resume/stack; the mover's turn is held by offers, combat_opportunity_test.go:474)
SCREEN: no match
DOCS: no match
TESTS: none

MECHANIC: opportunity attacks / trigger: a hostile creature with a melee reach leaves its reach by a move on foot
SERVER: backend/internal/play/combat_opportunity.go:95-131 opportunityReactors (hostile side, placed, not defeated, reaction unused at 115, no cantReact condition at 40-45, melee reach from sheet); provokedBy at combat_opportunity.go:145-155 uses grid.LeavesReach; offerOpportunities at combat_opportunity.go:160; move calls it at combat_move.go:472
SCREEN: web/src/app/pages/live-session/combat/opportunity/opportunity-card.ts:58-59; web/src/app/pages/live-session/combat/npc-card/npc-card.html:85 "Oferecer ataque de oportunidade"
DOCS: docs/product/rules.md:297 "**Opportunity attack.** For a straight move ... and a hostile reactor with reach R ft"
TESTS: backend/internal/play/combat_opportunity_test.go:139 TestRN21_LeavingAnEnemysReachOffersAnAttack; combat_opportunity_test.go:194 TestRN21_WhoProvokesAnOpportunityAttack

MECHANIC: opportunity attacks / Disengage suppresses offers for the rest of the turn
SERVER: backend/internal/play/combat_opportunity.go:96 returns no reactors when mover.Disengaged; flag set at combat_actions.go:1806-1810, reset queries.sql:231
SCREEN: web/src/app/pages/live-session/combat/move-page/move-status.ts:58-59 "Com Desengajar, nenhum movimento deste turno provoca isso."
DOCS: docs/product/rules.md:311 "**Disengage** (`standard:disengage`) sets the `disengaged` flag for the turn"
TESTS: backend/internal/play/combat_opportunity_test.go:193-206 (Desengajar case, the mover does not provoke)

MECHANIC: opportunity attacks / a reactor that is incapacitated, paralyzed, petrified, stunned, unconscious or blinded makes none
SERVER: backend/internal/play/combat_opportunity.go:40-45 cantReact list; checked at combat_opportunity.go:~107 ("slices.ContainsFunc(r.Conditions ...")
SCREEN: no match checked (unsure)
DOCS: no match
TESTS: backend/internal/play/combat_opportunity_test.go:679 TestMR034_OpportunityAuthorizationMatrix (not opened in full; unsure)

MECHANIC: readied action / Ready: choose a trigger and a response, act with reaction before your next turn
SERVER: "standard:ready" goes through the generic path: economy action spent (combat_actions.go:1732-1733), no trigger or prepared action stored (no match for "ready|prepar|gatilho|trigger" state in backend/internal/play); the later reaction is an ordinary reaction taken with TakeAction (combat_actions.go:1697-1699)
SCREEN: web/src/app/core/combat/combat-log.ts:453-454 " se prepara" (log text only)
DOCS: no match for Preparar/Ready in docs/product
TESTS: none

MECHANIC: readied action / a readied spell keeps concentration
SERVER: no match (no readied-spell state)
SCREEN: no match
DOCS: no match
TESTS: none

MECHANIC: delay / (the SRD text: "Ready" and "Dodge" are the fallback when undecided; no delay action in SRD text)
SERVER: no match for a delay action; EndTurn only (backend/internal/play/combat.go:744)
SCREEN: no match for "adiar|atrasar|postergar" in web combat screens and backend/internal/play (checked)
DOCS: no match
TESTS: backend/internal/play/combat_test.go:580 TestEndTurnIsIdempotent

## movement-and-position

MECHANIC: movement-and-position / move up to speed, deduct each part of the move
SERVER: backend/internal/play/combat_move.go:151-161 speedDFt (walk or fly, max, times 10 for tenths of a foot; doubled when Dashed); combat_move.go:164-166 movementLeftDFt = speed minus MovementUsedDft; MoveCombatant at combat_move.go:240 charges the cost (grid cost in tenths of a foot)
SCREEN: web/src/app/pages/live-session/combat/move-page/move-status.ts (movement summary; not opened line by line, unsure)
DOCS: docs/product/rules.md:276 "On the map grid, each square is 1.5 m and movement is a **circle**"
TESTS: backend/internal/play/combat_move_test.go:286 TestRN21_TheCircleCostsTheExactLine; backend/internal/play/combat_rules_test.go:146 TestRN21_MovementLeftIsKeptInTenthsOfAFoot

MECHANIC: breaking-up-your-move / move, act, move again (spend some speed before and after the action)
SERVER: movement is not gated by the action (no ActionUsed check in combat_move.go); the movement used accumulates in MovementUsedDft (queries.sql:231 resets it at turn start)
SCREEN: no match checked beyond move page (unsure)
DOCS: docs/product/rules.md:276 (circle movement, remaining speed)
TESTS: backend/internal/play/combat_move_test.go:1231 TestMR034_TheRunningStartAddsUpOnFootAndAnythingBreaksIt (partly; unsure)

MECHANIC: breaking-up-your-move / move between the attacks of an Attack action (Extra Attack)
SERVER: no match for a rule that blocks movement between attacks; MoveCombatant does not check AttacksMade (combat_move.go:240-300 no such check). Movement between attacks is allowed by absence of a check (unsure whether intended)
SCREEN: no match
DOCS: no match
TESTS: none

MECHANIC: breaking-up-your-move / using different speeds (walk and fly, switch mid-move, subtract distance)
SERVER: no match: flies() (backend/internal/play/combat_move.go:131) picks one mover type for the whole move; speedDFt (combat_move.go:151-161) uses max(walk, fly) for everything; no per-speed budget
SCREEN: no match
DOCS: no match
TESTS: backend/internal/play/combat_rules_test.go:174 TestRN21_AFlierIsWhoMovesOnItsFlySpeed

MECHANIC: difficult-terrain / each foot costs 1 extra foot (5 ft per square), counted once per square
SERVER: backend/internal/rules/grid/move.go:249-251 walk adds DFtPerSquare once for a square that is difficult or occupied (not for a flier); move.go:364 same test in MoveUntil; combat_move.go:494-495 LandingDifficult flags a jump landing (no roll, the master's log says it)
SCREEN: web/src/app/pages/live-session/combat (cost shown on move page; not opened, unsure)
DOCS: docs/product/rules.md:285 "**Difficult terrain.** Each difficult-terrain square the line enters ... costs 5 ft more, once per square"
TESTS: backend/internal/play/combat_move_test.go:365 TestRN21_DifficultTerrainAndOtherCreaturesCostMore

MECHANIC: difficult-terrain / a creature's space counts as difficult terrain (hostile or not)
SERVER: backend/internal/rules/grid/move.go:251 occupied square costs 5 ft (occupied is part of the same test); combat_move.go:181-198 occupantsFor (other combatants visible to the mover)
SCREEN: no match checked (unsure)
DOCS: docs/product/rules.md:287 "Passing through the square of a creature that is not hostile ... costs 5 ft more, as difficult terrain"
TESTS: backend/internal/play/combat_move_test.go:365 TestRN21_DifficultTerrainAndOtherCreaturesCostMore

MECHANIC: being-prone / drop prone without spending speed
SERVER: no match (no prone action; combat_move.go:142-143 comment: "Prone, which makes standing up cost half the speed, is not modeled: the app has no action for standing up"); prone is only a condition label (backend/internal/rules/srd51/data/conditions.json:104 condition:prone; names_pt.json:134 "Derrubado")
SCREEN: web/src/app/core/combat/conditions.ts:21 { key: 'condition:prone', name: 'Derrubado' } (a checkbox only)
DOCS: docs/product/rules.md:338 "`condition:prone` is called \"Derrubado\""; docs/product/rules.md:329 "It does not apply the effects by itself; the master decides"
TESTS: none

MECHANIC: being-prone / standing up costs half your speed; cannot if no movement left or speed 0
SERVER: no match (see comment combat_move.go:142-143; noSpeed list combat_move.go:144-147 does not include prone)
SCREEN: no match
DOCS: docs/product/rules.md:329 (conditions are labels; effects applied by the master)
TESTS: none

MECHANIC: being-prone / crawling costs 1 extra foot per foot (3 ft per foot in difficult terrain)
SERVER: no match for crawl (no prone-speed branch in combat_move.go:151-161 or grid/move.go)
SCREEN: no match
DOCS: no match
TESTS: none

MECHANIC: moving-around-other-creatures / move through a non-hostile creature's space
SERVER: backend/internal/rules/grid/move.go:149-152 Mover.passes (not hostile = passes); cost via move.go:251 (+5 ft)
SCREEN: no match checked (unsure)
DOCS: docs/product/rules.md:287 "Passing through the square of a creature that is not hostile to the character costs 5 ft more"
TESTS: backend/internal/play/combat_move_test.go:365 TestRN21_DifficultTerrainAndOtherCreaturesCostMore

MECHANIC: moving-around-other-creatures / move through a hostile creature only if it is at least two sizes larger or smaller
SERVER: backend/internal/rules/grid/move.go:149-152 passes: d >= 2 || d <= -2 on the size rank (move.go:38-41); sizes from combat_move.go:94-97 sizeToGrid
SCREEN: no match
DOCS: docs/product/rules.md:287 "The square of a hostile creature can be crossed only if the creature is at least two sizes larger or smaller"
TESTS: backend/internal/play/combat_move_test.go:365 (creature-passing cases; unsure of exact case)

MECHANIC: moving-around-other-creatures / cannot willingly end move in a creature's space (Tiny exception: Tiny shares Tiny squares)
SERVER: backend/internal/rules/grid/move.go:157-160 canEnd: only Tiny mover into a square with only Tiny passable creatures; walk() refuses end in an occupied square (move.go:255-262 StopOccupied); moveStop at combat_move.go:200-207
SCREEN: no match checked (unsure)
DOCS: docs/product/rules.md:287 "No move ends in another creature's square" (text cut at "No move ends in another cr...")
TESTS: backend/internal/play/combat_move_test.go:334 TestRN21_WallsColumnsAndSqueezesBlockAMove (unsure)

MECHANIC: moving-around-other-creatures / leaving a hostile creature's reach provokes an opportunity attack
SERVER: see opportunity attacks / trigger above (combat_opportunity.go:95-155; offers from combat_move.go:472)
SCREEN: see opportunity attacks
DOCS: docs/product/rules.md:297
TESTS: backend/internal/play/combat_opportunity_test.go:139

MECHANIC: flying-movement / flying creature knocked prone, speed 0, or otherwise unable to move falls (unless hover or magic)
SERVER: no match for falling (grep "fall|cai|queda" in backend/internal/play/combat*.go, backend/internal/rules/combat, backend/internal/rules/grid: none). Speed 0 from conditions is 0 movement (combat_move.go:144-150 noSpeed; no drop/fall)
SCREEN: no match
DOCS: no match
TESTS: backend/internal/play/combat_move_test.go:1382 TestTurnOptionsMovementFollowsTheConditionsThatLeaveNoSpeed (speed 0, no fall asserted)

MECHANIC: flying-movement / a flier moves on its fly speed and ignores difficult terrain
SERVER: backend/internal/play/combat_move.go:131 flies (fly speed >= walk speed); combat_move.go:136-138 moverOf -> grid.Mover{Flier}; backend/internal/rules/grid/move.go:138-141 and move.go:251 (no difficult cost for a flier)
SCREEN: move.go per GetMoveOptions Flier flag (combat_move.go:678, 710)
DOCS: docs/product/rules.md:276 (circle movement); no match for flying docs
TESTS: backend/internal/play/combat_rules_test.go:174 TestRN21_AFlierIsWhoMovesOnItsFlySpeed

MECHANIC: creature-size / space per size (Tiny 2.5 ft, Small/Medium 5 ft, Large 10 ft, Huge 15 ft, Gargantuan 20 ft+)
SERVER: backend/internal/rules/grid/move.go:29-35 size constants (Tiny..Gargantuan); move.go:38-41 rank. Each combatant occupies exactly one grid square: no multi-square footprint found (no match for "footprint|2x2|multi-square" in backend/internal/rules/grid and combat_move.go). Size only decides pass/share (move.go:149-160)
SCREEN: no match checked (unsure)
DOCS: no match for size table in docs/product
TESTS: none for multi-square footprint

MECHANIC: creature-size / squeezing into a space one size smaller: 1 extra foot per foot, disadvantage on attacks and Dex saves; attacks against it have advantage
SERVER: no match for creature squeeze (grid squeeze at backend/internal/rules/grid/move.go:179-180 is the corner rule for walls and closed doors, not creature size); no disadvantage or advantage state applied (no match for disadvantage/advantage in backend/internal/play/combat*.go except traps and perception search)
SCREEN: no match
DOCS: docs/product/rules.md:289 "Walls and cover squares ... and also a move that squeezes through the corner between two of them" (wall squeeze only)
TESTS: backend/internal/play/combat_move_test.go:334 TestRN21_WallsColumnsAndSqueezesBlockAMove (walls only, unsure)

MECHANIC: interacting-with-objects-around-you / one free object interaction during move or action (draw sword, open door, pick up, etc.)
SERVER: no match for free object interaction state; doors are opened by a move: backend/internal/play/combat_move.go:552 openDoors (RN-26), no action cost; other objects: no match
SCREEN: no match
DOCS: no match
TESTS: backend/internal/play/combat_doors_test.go:84 TestMR010_AMoveOpensAClosedDoor

MECHANIC: interacting-with-objects-around-you / a second object costs the action
SERVER: no match beyond the generic action path (use-an-object standard action; see use-an-object)
SCREEN: no match
DOCS: no match
TESTS: none

## actions-in-combat

MECHANIC: actions-in-combat / improvised action (GM says if possible and what roll)
SERVER: no match for improvised actions; TakeAction accepts only keys from standard or feature action lists (backend/internal/play/combat_actions.go:1685-1692, "action_key is not one of the standard or feature actions"); test rejects "standard:fly" (backend/internal/play/combat_actions_test.go:585-587)
SCREEN: no match
DOCS: no match
TESTS: backend/internal/play/combat_actions_test.go:585-587

MECHANIC: attack / one melee or ranged attack; Extra Attack
SERVER: standard:attack is refused in TakeAction and goes to RollAttack (combat_actions.go:1644-1646); action economy by attackOption (backend/internal/rules/combat/turn.go:367-375: AttacksLeft or ReasonActionUsed / ReasonAttacksUsed); AttacksMade and LastAttackKey kept in TurnState (turn.go:19-34); Action spent by first attack (combat_actions.go:1732-1733 generic path does not apply to attack; attack path in RollAttack, see combat_actions.go:619, 671 ReactionUsed checks)
SCREEN: web/src/app/pages/live-session/combat (Ataques group; not opened, unsure)
DOCS: docs/product/glossary.md:42 (action economy)
TESTS: backend/internal/play/combat_actions_test.go:579-580 (Esquivar blocked after attack, second attack blocked)

MECHANIC: cast-a-spell / a spell with casting time 1 action costs the action; reaction and bonus action spells have their own economy
SERVER: standard:cast-a-spell refused in TakeAction (combat_actions.go:1644-1646); spell economy spellEconomy (backend/internal/rules/combat/turn.go:392-402) and spellOption (turn.go:458-494); a casting time of a minute or more is refused (turn.go:477-478 ReasonTooLong); bonus action spell limit (turn.go:445-450)
SCREEN: web/src/app/pages/live-session/combat/cast-sheet/cast-sheet.ts (not opened, unsure)
DOCS: docs/product/stories.md:293 (Magias group under Ação)
TESTS: backend/internal/play/combat_spells_test.go:2645 TestBonusActionSpellLeavesNoOtherSpellButACantrip; combat_spells_test.go:2670 TestSpellThenBonusActionSpellIsRefusedToo

MECHANIC: dash / gain extra movement equal to speed (after modifiers) for the turn
SERVER: TakeAction generic path, then standard == "standard:dash" -> markDashed (backend/internal/play/combat_actions.go:1799-1803; combat_move.go:616-619 markDashed sets dashed via queries.sql:249-252 MarkCombatantDashed); speed doubled in speedDFt (combat_move.go:156-157) and in TurnOptions (backend/internal/rules/combat/turn.go:181-183, uses SpeedWalkFt); reset at turn start (queries.sql:231); undone by undo (backend/internal/play/combat_undo.go:318)
SCREEN: web/src/app/pages/live-session/combat/combat-view.ts:1235-1238 comment "A standard action ('Disparada'): it spends the action; Dash doubles the movement"
DOCS: docs/product/glossary.md:42 "(the speed, doubled after Dash)"
TESTS: backend/internal/play/combat_actions_test.go:591-595 (Pensantus Dash, speed 25 becomes 50, action used); combat_actions_test.go:1169-1177 (Dash undone)

MECHANIC: dash / speed change after Dash (a speed of 15 gives 30 with Dash)
SERVER: speedDFt multiplies the current speed after the walk/fly choice (combat_move.go:151-158); speed reduction is read from the sheet at add time (combat.go:369 SpeedFt copy); no per-turn change of speed
SCREEN: no match
DOCS: no match
TESTS: none

MECHANIC: dash / Cunning Action, Step of the Wind: Dash as a bonus action
SERVER: backend/internal/rules/actions.go:70-71 featureStandards maps feature:cunning-action to standard:dash and step-of-the-wind to standard:dash; TakeAction uses fa.Standard (combat_actions.go:1766-1769) and then markDashed (combat_actions.go:1799-1803)
SCREEN: web/src/app/pages/live-session/combat (feature actions list, not opened line by line)
DOCS: no match
TESTS: backend/internal/play/combat_actions_test.go:2102-2113 (feature:cunning-action:dash)

MECHANIC: disengage / movement does not provoke opportunity attacks for the rest of the turn
SERVER: flag set at backend/internal/play/combat_actions.go:1806-1810 (SetCombatantDisengaged, queries.sql:254-258); read by backend/internal/play/combat_opportunity.go:96; reset at turn start queries.sql:231; undo combat_undo.go:545
SCREEN: web/src/app/pages/live-session/combat/combat-view.ts:650-652 canDisengage (not actionUsed and not disengaged); combat-view.ts:2207-2209 disengage() sends standard:disengage; move-status.ts:58-59 note; action-groups.html:12 and :32 shows "Desengajado"
DOCS: docs/product/rules.md:311 "**Disengage** (`standard:disengage`) sets the `disengaged` flag for the turn"
TESTS: backend/internal/play/combat_move_test.go:1011 TestRN21_DisengageSetsAFlagForTheTurn; combat_opportunity_test.go:193-206

MECHANIC: dodge / until start of next turn, attacks against you have disadvantage if attacker seen; Dex saves with advantage; lost if incapacitated or speed 0
SERVER: no match for a dodge state (no Dodging flag in queries.sql or TurnState, backend/internal/rules/combat/turn.go:13-35). standard:dodge goes through the generic path: economy spent only (combat_actions.go:1732-1733, 1749-1753). No attack or save code reads a dodge flag (no match for "dodg|esquiv" outside feature key backend/internal/rules/actions.go:72 patient-defense)
SCREEN: web/src/app/core/combat/combat-log.ts:457-458 default text " usa ${keyNamePt}" (logs "usa Esquivar"); no dedicated dodge UI found
DOCS: no match for Esquivar/Dodge in docs/product
TESTS: backend/internal/play/combat_actions_test.go:579-580 (Esquivar blocked after attacking); combat_actions_test.go:1937-1941 (log line "R1 Toren: Esquivar")

MECHANIC: help / aid an ally's ability check (advantage on next check before your next turn), or aid an attack vs creature within 5 ft (first attack advantage)
SERVER: no match for a help state. standard:help goes through the generic path, only the action is spent (combat_actions.go:1732-1733, 1749-1753). No advantage is granted to the ally's roll (no match in combat roll code)
SCREEN: no match for "Ajudar" in web combat screens
DOCS: no match for Ajudar/Help in docs/product
TESTS: none

MECHANIC: hide / Dexterity (Stealth) check to hide; benefits in Unseen Attackers and Targets
SERVER: standard:hide goes through the generic path; only the action is spent (combat_actions.go:1732-1733). No hidden flag is set by the action. The combatant field Hidden is the master's visibility toggle (SetCombatantHidden, backend/internal/play/combat.go:927-987), not the Hide action. Cunning Action hide maps to standard:hide (backend/internal/rules/actions.go:70)
SCREEN: web/src/app/core/combat/combat-log.ts:449-450 " se esconde" (log only); order-list.html:164 "Esconder dos jogadores" (master toggle, a different thing)
DOCS: no match for the Hide action in docs/product
TESTS: backend/internal/play/combat_actions_test.go:1353 and :1363 (standard:hide used in a scripted fight; no state asserted, unsure)

MECHANIC: ready / choose trigger and reaction; release with reaction when trigger occurs; one reaction per round
SERVER: see readied action above: economy only; the reaction later is ordinary (combat_actions.go:1697-1699, 1728-1729). Trigger text is not stored. No check that a reaction was readied
SCREEN: web/src/app/core/combat/combat-log.ts:453-454 " se prepara"; web/src/app/core/combat/combat-options.ts:45 "Só quando o gatilho acontecer"
DOCS: no match
TESTS: none

MECHANIC: ready / readied spell: concentration; spell dissipates if concentration breaks
SERVER: no match (no readied spell state)
SCREEN: no match
DOCS: no match
TESTS: none

MECHANIC: search / Wisdom (Perception) or Intelligence (Investigation) check to find something
SERVER: standard:search generic path (only action spent, combat_actions.go:1732-1733). Trap search is its own write: backend/internal/play/traps.go:336-340 refuses when ActionUsed; traps.go:349-353 spends the action (SetCombatantEconomy ActionUsed true); combat log backend/internal/play/combat_log.go:313-316 maps eventTrapSearched to standard:search (master's line only)
SCREEN: web/src/app/pages/live-session/combat/combat-view.ts:1249-1258 standardAction: "standard:search" opens trap search when the route is 'traps', else takeAction; web/src/app/core/combat/combat-log.ts:451-452 " procura ao redor"
DOCS: docs/product/stories.md:824 "in combat it is the Search action"
TESTS: backend/internal/play/traps_test.go:455 TestMR035_SearchingInACombatCostsTheAction

MECHANIC: use-an-object / one object as an action when the object requires an action
SERVER: standard:use-an-object goes through the generic path (only action spent, combat_actions.go:1732-1733, 1749-1753). No object state
SCREEN: web/src/app/core/combat/combat-log.ts:455-456 " usa um objeto" (log only)
DOCS: no match
TESTS: none

## Cross-check notes (factual, from the code above)

- combat_turn.go:35 groups joint turns by equal Initiative total only, while combat_rules.go:77-78 orders equal totals by InitiativeBonus first. Two combatants with equal totals and different bonuses are ordered by bonus but form one joint turn.
- backend/internal/rules/combat/turn.go:181-183 computes Economy.Movement speed from d.SpeedWalkFt, while combat_move.go:155 and combat_actions.go:159 use max(walk, fly). For a flier with fly speed above walk, these two differ. Unsure which one a screen reads.
- The master may take a standard action with the action already used (combat_actions.go:1704-1713; the override applies to non-feature actions), a player may not.
- combat_move.go:142-143 documents that prone and standing up are not modeled.
- No code, doc or test covers surprise, Help, Dodge (beyond economy and log), Hide (beyond economy and log), Ready (beyond economy and log), Use an Object (beyond economy), or falling.
