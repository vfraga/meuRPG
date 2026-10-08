# Raw sweep: Using Ability Scores (SRD 2014 chapter 7 index set)

MECHANIC: using-ability-scores / six abilities and their scores
SERVER: /home/user/meuRPG/backend/internal/rules/abilities.go:15 maps STR/DEX/CON to "strength"/"dexterity"/"constitution" keys (rest of the six follow the same map).
SCREEN: unsure (not searched in web/src/app for ability display).
DOCS: /home/user/meuRPG/docs/product/glossary.md:80 "Ability (habilidade)... the score ("valor de habilidade"), the modifier and the check".
TESTS: /home/user/meuRPG/backend/internal/rules/abilitymethods_test.go:80 TestAbilityRolls (name only; generation methods, not the modifier table).

MECHANIC: ability-scores-and-modifiers / modifier = floor((score - 10) / 2)
SERVER: /home/user/meuRPG/backend/internal/rules/abilities.go:25-31 func modifier(score) with floor division for scores below 10 too.
SCREEN: unsure (not searched).
DOCS: no match for the modifier table in docs/product (only the glossary entry at glossary.md:80).
TESTS: unsure.

MECHANIC: advantage-and-disadvantage / roll_mode effect is a hint only, never applied to a roll
SERVER: /home/user/meuRPG/backend/internal/rules/effects.go:47-48 roll_mode fields Roll ("advantage"/"disadvantage") and Targets; effects.go:217-224 validation.
SERVER: /home/user/meuRPG/backend/internal/rules/choices.go:21-24 roll_mode becomes a Hint via x.hint(); choices.go:66-80 hint() builds Hint text only.
SERVER: /home/user/meuRPG/backend/internal/characters/derived.go:178-179 hints copied to the sheet as text (SourceKey, Text); no roll input.
SERVER: /home/user/meuRPG/proto/meurpg/rules/v1/rules.proto:927 "The server never applies it on its own: the master decides."
SERVER: no match for any roll path reading d.Hints or a roll mode (grep of backend/internal/play for advantage/vantagem/roll_mode: only the trap search and comments).
SCREEN: /home/user/meuRPG/web/src/app/pages/character-sheet/character-sheet.types.ts:107 comment: advantage on a save against magic shown as a sheet hint.
SCREEN: /home/user/meuRPG/web/src/app/shared/wild-shape/beast-traits.ts:7 "the master applies the advantage they give (the app does not...)".
SCREEN: /home/user/meuRPG/web/src/app/pages/character-sheet/character-sheet.spec.ts:1042 hint text rendered (spec, text only).
DOCS: /home/user/meuRPG/docs/product/glossary.md:89 "A bonus or advantage that depends on the situation... The sheet shows it; the master decides when it applies."
DOCS: /home/user/meuRPG/backend/internal/rules/tablemenu.go:140 (menu HintPT) "com uma situação (tags), o app só lembra e o mestre decide" (the app only remembers and the master decides).
TESTS: /home/user/meuRPG/backend/internal/rules/derive_test.go:185 Gnome Cunning hint (advantage, saves vs magic).
TESTS: /home/user/meuRPG/backend/internal/rules/attacks_test.go:126 heavy-weapon disadvantage hint.

MECHANIC: advantage-and-disadvantage / who can set advantage or disadvantage (source)
SERVER: roll_mode sources are feature/race/item effects in content (/home/user/meuRPG/backend/internal/rules/srd51/effects/races.json:7, :26, :44, :94 save.all/save.int advantage vs poison, charmed, frightened, magic; barbarian.json:12 save.dex advantage vs visible effects).
SERVER: /home/user/meuRPG/backend/internal/rules/attacks.go:58 heavy weapon vs Small race: disadvantage hint (source = equipment).
SERVER: /home/user/meuRPG/backend/internal/rules/armor.go:40-43 stealth disadvantage hint for stealth-penalty armor; armor.go:38 issue text "desvantagem em testes, ataques e testes de resistência de FOR e DES" for armor without proficiency (issue text; mechanical effect unsure).
SCREEN: no UI control to set advantage on a roll (no match in web/src/app/core/play or scene-roll-sheet for vantagem/desvantagem).
DOCS: /home/user/meuRPG/docs/product/stories.md:822 design choice text: "an active check in dim light has no disadvantage (the master decides)".
DOCS CONFLICT: that same story sentence says no disadvantage in dim light for an active check; code applies disadvantage to the dim-light Perception search (see next block). Recorded, not judged.
TESTS: none for the "master decides" path.

MECHANIC: advantage-and-disadvantage / one live disadvantage: Perception search in dim light (two d20, lower counts)
SERVER: /home/user/meuRPG/backend/internal/play/traps.go:219-251 active search rolls d20 + bonus, and a second d20 when skill is Perception and in-app; traps.go:251 returns "a Perception search in dim light has disadvantage: roll a second die" when maplink.ErrSearchNeedsTwoDice.
SERVER: /home/user/meuRPG/backend/internal/maps/link/link.go:178-180 ErrSearchNeedsTwoDice doc (square within 3 m seen and lightly obscured).
SERVER: /home/user/meuRPG/backend/internal/maps/traps_play.go:162 obscured flag: "an active Perception check on it has disadvantage (SRD)".
SERVER: /home/user/meuRPG/backend/internal/play/combat_events.go:434-435 D20B second d20 kept in the event.
SCREEN: /home/user/meuRPG/web/src/app/pages/live-session/traps/trap-search-sheet/trap-search-sheet.ts:147 "a procura tem desvantagem. Role de novo e digite o segundo dado" (asks second die when typed).
SCREEN: /home/user/meuRPG/web/src/app/core/traps/trap-errors.ts:13 needs-second-die error.
DOCS: /home/user/meuRPG/proto/meurpg/play/v1/traps.proto:45 "The search has disadvantage there".
DOCS: /home/user/meuRPG/docs/product/stories.md:818-822 (see conflict above).
TESTS: /home/user/meuRPG/backend/internal/play/traps_round2_test.go:400 TestMR035_APerceptionSearchInDimLightHasDisadvantage.
TESTS: /home/user/meuRPG/web/src/app/pages/live-session/traps/trap-search-sheet/trap-search-sheet.spec.ts:115 asks for second die.

MECHANIC: advantage-and-disadvantage / attack rolls (weapon and spell) with advantage or disadvantage
SERVER: /home/user/meuRPG/backend/internal/play/combat_spells.go:582 d20(in, modifier) has no mode parameter; spellAttack at combat_spells.go:597 rolls s.d20(in, sp.ToHit) once.
SERVER: /home/user/meuRPG/backend/internal/rules/attacks.go:58 heavy-weapon disadvantage as hint only.
SCREEN: /home/user/meuRPG/web/src/app/core/combat/attack-flow.ts:23-27 steps "Alvo", "Rolar", "Dano"; no advantage control.
DOCS: /home/user/meuRPG/proto/meurpg/play/v1/combat.proto:639 "Disadvantage at long range or next to an enemy is not applied yet." (deliberately not built, per the comment).
TESTS: unsure.

MECHANIC: advantage-and-disadvantage / ability checks and saves with advantage or disadvantage (roll flows)
SERVER: /home/user/meuRPG/backend/internal/play/scene.go:710-711 RollSceneCheck: bonus := options[0].Bonus; s.d20(in, bonus), one d20, no mode.
SERVER: /home/user/meuRPG/backend/internal/play/combat_spells_view.go:38 saves: one d20 + bonus, no mode.
SCREEN: /home/user/meuRPG/web/src/app/pages/live-session/scene/scene-roll-sheet/scene-roll-sheet.ts:53 buttons "Rolar no app" / "Digitar o resultado" (no mode).
DOCS: no match in docs/product for advantage on scene checks.
TESTS: unsure.

MECHANIC: advantage-and-disadvantage / cancellation (advantage + disadvantage = neither; only one extra d20)
SERVER: no match for cancel/anula logic in backend/internal/rules or backend/internal/play (grep for anula/cancel/neither with vant/adv/dis).
SCREEN: no match.
DOCS: no match.
TESTS: none.

MECHANIC: advantage-and-disadvantage / reroll of one die (Lucky, halfling)
SERVER: no match for a reroll-one-of-two rule (Lucky / sorte) in play or rules.
SCREEN: no match.
DOCS: no match.
TESTS: none.

MECHANIC: proficiency-bonus / applied once per roll, never twice
SERVER: /home/user/meuRPG/backend/internal/rules/abilities.go:333-340 skills(): b = mod + profBonus(level) computed once per skill key.
SERVER: /home/user/meuRPG/backend/internal/rules/abilities.go:344-346 passives add proficiency once via the skill bonus.
SERVER: /home/user/meuRPG/backend/internal/rules/abilities.go:357 initiative adds profBonus(best) once.
SCREEN: unsure.
DOCS: unsure.
TESTS: /home/user/meuRPG/backend/internal/rules/derive_test.go:493-494 (Arcana half, bonus 2).

MECHANIC: proficiency-bonus / half proficiency (Jack of All Trades, rounded down)
SERVER: /home/user/meuRPG/backend/internal/rules/abilities.go:318-319 ProficiencyHalf returns prof/2 (floor).
SERVER: /home/user/meuRPG/backend/internal/rules/api.go:538-539 ProficiencyHalf "adds half, rounded down (Jack of All Trades)".
SERVER: /home/user/meuRPG/backend/internal/rules/abilities.go:348-357 initiative gets half via a proficiency "initiative" effect (Jack of All Trades) when the effect is active.
SERVER: /home/user/meuRPG/backend/internal/rules/abilities.go:299-307 proficiencyLevelOf / levels map; "half" from class/feature data.
SCREEN: /home/user/meuRPG/web/src/app/core/content/content-read.ts:253 (saves listing only; half not shown) — unsure for skill screens.
DOCS: /home/user/meuRPG/docs/product/glossary.md:150 Level-up "Escolhas" for skill or expertise (no half text).
TESTS: /home/user/meuRPG/backend/internal/rules/derive_test.go:493-494 (half, Arcana).

MECHANIC: proficiency-bonus / expertise (doubled proficiency)
SERVER: /home/user/meuRPG/backend/internal/rules/abilities.go:322-323 ProficiencyExpertise returns 2 * prof.
SERVER: /home/user/meuRPG/backend/internal/rules/abilities.go:293-299 expertise list sets level to Expertise; abilities.go:296-297 IssueExpertise when the skill is not at full proficiency ("Especialização em %s pede proficiência nessa perícia").
SERVER: /home/user/meuRPG/backend/internal/rules/choices.go:118-125 and :152-159 expertise allowance counted (feature Count, ExpertiseChoices); issue when too many.
SERVER: /home/user/meuRPG/backend/internal/rules/api.go:125-128 Expertise field doc (Rogue, Bard).
SCREEN: unsure (not searched for expertise picker in web).
DOCS: /home/user/meuRPG/docs/product/glossary.md:150 "Escolhas" at levels that give "a skill or expertise (RN-01, RN-12, MR-040)".
TESTS: /home/user/meuRPG/backend/internal/rules/derive_test.go:504-506 (Expertise Stealth bonus 6), :512-513 (expertise without proficiency issue).

MECHANIC: proficiency-bonus / no proficiency means no bonus for doubled or halved values (0 x anything)
SERVER: /home/user/meuRPG/backend/internal/rules/abilities.go:310-312 profBonus returns 0 for ProficiencyNone.
SERVER: /home/user/meuRPG/backend/internal/rules/choices.go:28-33 hint for Artificer-style skill proficiency uses max(levels, e.Level) with profBonus; no doubling on a non-proficient skill.
SCREEN: unsure.
DOCS: none.
TESTS: unsure.

MECHANIC: ability-checks / ability check = d20 + modifier (no proficiency), scene action form
SERVER: /home/user/meuRPG/backend/internal/rules/progression.go:229-234 SceneAbility: Bonus = s.Modifier, NamePT "Teste de "+ability.
SERVER: /home/user/meuRPG/backend/internal/rules/progression.go:181-201 SceneCheckName: "Teste de Força" / "Teste de resistência de ..." by key ability:/save:.
SERVER: /home/user/meuRPG/backend/internal/play/scene.go:383-396 player sees bonus only if Known (passive only for perception/investigation/insight).
SCREEN: /home/user/meuRPG/web/src/app/pages/live-session/scene/scene-player/scene-player.html:43-44 bonus text shown ("sc__bonus").
SCREEN: /home/user/meuRPG/web/src/app/core/maps/scene-actions.ts:21 kind 'Teste de habilidade' in the master's action list.
DOCS: /home/user/meuRPG/docs/product/stories.md:336 player's block "Cena" with own bonus and "Rolar".
TESTS: /home/user/meuRPG/backend/internal/rules/progression_test.go:123 TestSceneOptions; :241 TestSceneCheckName.

MECHANIC: ability-checks / roll flow: player rolls a scene check (app or typed die), server decides pass/fail
SERVER: /home/user/meuRPG/backend/internal/play/scene.go:573 RollSceneCheck; scene.go:704-707 ALREADY_ROLLED when attempts used; scene.go:711-720 d20 + bonus; scene.go:723-725 Passed = total >= DC when DC > 0 (line from grep: "ev.Passed = new(roll.Total >= action.DC)").
SERVER: /home/user/meuRPG/backend/internal/play/scene.go:690-697 attempts tally per character per action.
SCREEN: /home/user/meuRPG/web/src/app/pages/live-session/scene/scene-player/scene-player.ts:36-47 "Rolar" opens the roll sheet; attempts text.
SCREEN: /home/user/meuRPG/web/src/app/pages/live-session/scene/scene-roll-sheet/scene-roll-sheet.ts:53 "Rolar no app" / "Digitar o resultado".
SCREEN: /home/user/meuRPG/web/src/app/pages/live-session/scene/scene-player/scene-player.ts:93-94 "Rolada às ..." label after attempts run out.
DOCS: /home/user/meuRPG/docs/product/rules.md:236-240 scene DC per scene, attempts, pass/fail shown only if switch on (RN-20).
DOCS: /home/user/meuRPG/docs/product/stories.md:325 "when off, they see only the total (RN-20)".
TESTS: /home/user/meuRPG/backend/internal/play/summary_test.go:302 TestSummaryCountsAnyNumberOfSceneRolls; :340 TestUnlimitedSceneActionHasARollCap.
TESTS: /home/user/meuRPG/backend/internal/play/play_test.go:128 RollSceneCheck in authorization matrix only.

MECHANIC: typical-difficulty-classes / DC table (5, 10, 15, 20, 25, 30)
SERVER: /home/user/meuRPG/backend/internal/maps/scenes.go:43-44 maxDC = 30 "the highest difficulty class".
SERVER: /home/user/meuRPG/backend/internal/maps/scenes.go:443-449 cleanDC: 0 for none, else 1 to 30.
SERVER: no match for the named tiers ("Muito fácil", "Fácil", "Médio", "Difícil") as DC presets in backend.
SCREEN: /home/user/meuRPG/web/src/app/pages/maps/scene-actions/scene-action-form.ts:59-61 DC (1 to 30) optional; free text.
SCREEN: /home/user/meuRPG/web/src/app/pages/maps/scene-actions/scene-action-form.ts:218-221 DC parse error.
SCREEN: no match for a DC preset picker in web/src/app (tier names only in encounter/creature contexts).
DOCS: no match for the typical DC table in docs/product.
TESTS: unsure.

MECHANIC: typical-difficulty-classes / DC for a save or trap set by the master (not by the sheet)
SERVER: /home/user/meuRPG/backend/internal/play/traps_effect.go:187-203 save DC = sv.GetDc() (trap's DC).
SERVER: /home/user/meuRPG/proto/meurpg/play/v1/combat.proto:3613-3614 TrapSaveRoll dc "Only the master".
SCREEN: /home/user/meuRPG/web/src/app/pages/maps/point-kinds/trap-point-panel.html:199 DC input (1 to 30 range in trap-draft.ts:293).
DOCS: /home/user/meuRPG/docs/product/stories.md:809 master places the DC to notice, DC to find; "only the master sees it".
TESTS: unsure.

MECHANIC: contests / opposed ability checks (both roll, higher total wins, tie = no change)
SERVER: no match for a contest resolution (compare two check totals) in backend/internal/play or rules (grep: contest|disputa|oposto|opposed → only SRD JSON in srd51/data and an unrelated theatre.ts).
SCREEN: /home/user/meuRPG/web/src/app/core/combat/theatre.ts:133 opposed(a,b) is hostility, not a contest.
SCREEN: no match for contest UI.
DOCS: no match in docs/product for contest.
TESTS: none.
NOTE: grapple/shove/escape and Dex(Stealth) vs passive Perception hide contest: no match (grep for grapple|shove|agarr|empurr|escapar|furtividade in play/rules/combat).

MECHANIC: contests / Dexterity (Stealth) hide contested by Wisdom (Perception) search
SERVER: no match for a hide contest; only the Hide standard action spends economy (/home/user/meuRPG/backend/internal/rules/srd51/effects/standard_actions.json, key "hide").
SERVER: /home/user/meuRPG/backend/internal/rules/armor.go:40-43 stealth-disadvantage armor hint only.
SCREEN: no match.
DOCS: no match.
TESTS: /home/user/meuRPG/backend/internal/rules/derive_test.go:423 stealth disadvantage hint for chain mail.

MECHANIC: skills / skill list and ability mapping (18 skills, skill = ability sub-check)
SERVER: /home/user/meuRPG/backend/internal/rules/api.go:75 MaxSkillKeys = 18.
SERVER: /home/user/meuRPG/backend/internal/rules/abilities.go:329-341 skills(): each skill uses its ability (s.Ability) modifier + profBonus.
SERVER: /home/user/meuRPG/backend/internal/rules/abilities.go:15 ability key map.
SCREEN: /home/user/meuRPG/web/src/app/core/puzzles/puzzle-errors.ts:120 "escolha uma das 18 perícias".
DOCS: /home/user/meuRPG/docs/product/glossary.md:114 passive definition references skill bonus.
TESTS: /home/user/meuRPG/backend/internal/rules/derive_test.go:493.

MECHANIC: skills / variant: skill proficiency with a different ability (Constitution (Athletics) etc.)
SERVER: no match for a variant ability-skill override (grep for variant/variante in abilities.go, choices.go returned none).
SCREEN: no match.
DOCS: no match.
TESTS: none.

MECHANIC: passive-checks / passive score = 10 + bonus (no roll)
SERVER: /home/user/meuRPG/backend/internal/rules/abilities.go:344-346 PassivePerception / PassiveInvestigation / PassiveInsight = 10 + skill bonus.
SERVER: /home/user/meuRPG/backend/internal/rules/creatures.go:552-553 creature passives 10 + skill bonus.
SERVER: /home/user/meuRPG/backend/internal/rules/progression.go:215-223 SceneOptions attaches passive for perception/investigation/insight only.
SERVER: /home/user/meuRPG/backend/internal/play/scene.go:391-393 passive sent to a player only when Known.
SCREEN: /home/user/meuRPG/web/src/app/pages/live-session/scene/scene-player/scene-player.html:18-19 passive shown ("sc__passive").
SCREEN: /home/user/meuRPG/web/src/app/shared/trap-noticers/trap-noticers.html:15 "Percepção passiva contra a CD ... A penumbra tira 5".
DOCS: /home/user/meuRPG/docs/product/glossary.md:114 "10 plus the skill bonus, without rolling".
DOCS: /home/user/meuRPG/docs/product/stories.md:331 player reads passive Perception, Investigation, Insight.
TESTS: /home/user/meuRPG/backend/internal/play/traps_test.go:206 TestMR035_ThePassiveNotice; :328 TestMR035_APassiveNoticeFollowsTheLight.
TESTS: /home/user/meuRPG/backend/internal/rules/creatures_test.go:76 passive per creature.

MECHANIC: passive-checks / advantage +5 and disadvantage -5 on the passive
SERVER: disadvantage -5 only: /home/user/meuRPG/backend/internal/rules/vision/vision.go:94-99 PassivePenalty returns -5 for SeenDim/SeenGrey.
SERVER: /home/user/meuRPG/backend/internal/maps/traps_play.go:169-173 penaltyAt uses vision.PassivePenalty; blindsight exemption at :170-172.
SERVER: /home/user/meuRPG/backend/internal/maps/traps_play.go:266 notice: eyes.Passive + sg.penalty >= NoticeDc.
SERVER: no match for +5 advantage on a passive (no passive path reads advantage).
SCREEN: /home/user/meuRPG/web/src/app/shared/trap-noticers/trap-noticers.html:15 penumbra -5 explained.
DOCS: /home/user/meuRPG/docs/product/glossary.md:114 "with −5 in dim light or in darkness seen through darkvision".
DOCS: /home/user/meuRPG/docs/product/glossary.md:127 "a passive Perception check there loses 5".
TESTS: /home/user/meuRPG/backend/internal/rules/vision/vision_test.go:257 TestPassivePenalty.

MECHANIC: passive-checks / passive notice of a trap after a move (player character ends move within 3 m)
SERVER: /home/user/meuRPG/backend/internal/play/combat_traps.go:448-470 noticeAfterMove calls traps.Notice after a move.
SERVER: /home/user/meuRPG/backend/internal/maps/traps_play.go:266 notice check.
SCREEN: /home/user/meuRPG/web/src/app/shared/trap-noticers/trap-noticers.ts:41 comment on passive score and light penalty per character.
DOCS: /home/user/meuRPG/docs/product/stories.md:817 notice after ending a movement within 3 m, with passive Perception.
TESTS: /home/user/meuRPG/backend/internal/play/traps_notice_test.go:39 TestRN10_ThePassiveNoticeReachesOnlyItsPlayer.

MECHANIC: passive-checks / passive stealth comparison in hiding (passive Perception of creatures vs Stealth)
SERVER: no match (see contests / hide). 
DOCS: no match.
TESTS: none.

MECHANIC: working-together / Help action (team help, one creature)
SERVER: /home/user/meuRPG/backend/internal/rules/srd51/effects/standard_actions.json:8 {"key":"help","name_pt":"Ajudar","economy":"action"}.
SERVER: /home/user/meuRPG/backend/internal/rules/actions.go:102 standard actions appended from data.
SERVER: /home/user/meuRPG/backend/internal/play/combat_actions.go:1799-1811 only standard:dash and standard:disengage have effects; no effect for standard:help (no advantage granted).
SERVER: no match for help granting advantage on an attack or check.
SCREEN: no match for "Ajudar" in /home/user/meuRPG/web/src/app (action name only comes from data).
DOCS: /home/user/meuRPG/proto/meurpg/play/v1/combat.proto:742-746 "The rest only spend the action and go to the log: the app reminds the table of their effects."
DOCS: /home/user/meuRPG/docs/architecture.md:1570 TakeAction lists Help among actions that only spend the economy.
TESTS: /home/user/meuRPG/backend/internal/play/combat_theatre_test.go:923 takes "standard:help".
TESTS: /home/user/meuRPG/backend/internal/rules/actions_test.go:64 standard action names include "Ajudar".

MECHANIC: working-together / lead's ability check with advantage when helped (Help or team)
SERVER: no match for a team-help advantage on a check (no code reads help).
SCREEN: no match.
DOCS: no match.
TESTS: none.

MECHANIC: working-together / group check (at least half the group succeeds)
SERVER: no match for a group check (grep: teste em grupo, group check, metade do grupo, at least half, GroupCheck in backend/internal, web/src/app, proto; only "rollGroup" in play/combat_creatures.go:444-448 which is group initiative for creatures).
SCREEN: no match.
DOCS: no match in docs/product or docs/architecture.md.
TESTS: none.

MECHANIC: using-each-ability / Strength: Athletics (climb, jump, swim) and other STR checks
SERVER: /home/user/meuRPG/backend/internal/rules/combat/jump.go:7-8 and :30 long/high jump uses Força modifier (3 + STR modifier feet high jump).
SERVER: no match for climb/swim/athletics check flow beyond the generic skill options (progression.go:215-223).
SCREEN: /home/user/meuRPG/web/src/app/core/maps/scene-actions.ts:21 generic check kinds.
DOCS: no match for athletics check text.
TESTS: unsure.

MECHANIC: using-each-ability / Strength: carrying capacity (15 x STR), push/drag/lift (30 x STR), size multiplier, encumbrance variant
SERVER: no match for carrying capacity, push/drag/lift, encumbrance or size-weight multiplier (grep for carrying|carga|encumb|capacidade de carga|push|arrast in rules, characters, play).
SCREEN: no match.
DOCS: no match.
TESTS: none.

MECHANIC: using-each-ability / Strength: melee attack and damage add STR
SERVER: /home/user/meuRPG/backend/internal/rules/attacks.go:19-20 "A weapon attacks with STR, or DEX if it is ranged; a finesse weapon takes the better of the two".
SERVER: /home/user/meuRPG/backend/internal/rules/attacks.go:44-50 melee/ranged kind and finesse check.
SERVER: /home/user/meuRPG/backend/internal/rules/attacks.go:76 AbilityMod for the attack.
SCREEN: /home/user/meuRPG/web/src/app/core/combat/attack-flow.ts:23-27 steps (no modifier shown in the step names).
DOCS: unsure.
TESTS: unsure.

MECHANIC: using-each-ability / Dexterity: attack and damage with ranged and finesse weapons
SERVER: /home/user/meuRPG/backend/internal/rules/attacks.go:19-20, :50 (same rule as Strength block above).
SCREEN: unsure.
DOCS: unsure.
TESTS: unsure.

MECHANIC: using-each-ability / Dexterity: initiative (d20 + DEX modifier, Jack of All Trades half via initiative effect)
SERVER: /home/user/meuRPG/backend/internal/rules/abilities.go:348-357 Initiative = modifiers("initiative", DEX mod + profBonus(best)).
SERVER: /home/user/meuRPG/backend/internal/play/combat.go:369 InitiativeBonus stored per participant (the d20 roll line is in the same function, not located: unsure).
SERVER: /home/user/meuRPG/backend/internal/play/combat_creatures.go:440-448 rollGroup: one initiative d20 for a creature group, bonus = first creature's DEX modifier (per comment).
SCREEN: unsure.
DOCS: unsure.
TESTS: /home/user/meuRPG/backend/internal/rules/derive_test.go:153-154 initiative +3; :496-497 DEX +2 + 1 initiative.

MECHANIC: using-each-ability / Dexterity: Armor Class from DEX (armor-dependent)
SERVER: unsure (not located; armor.go:38-43 covers armor proficiency and stealth only).
SCREEN: unsure.
DOCS: unsure.
TESTS: unsure.

MECHANIC: using-each-ability / Dexterity: acrobatics (difficult terrain on landing, DC 10 Acrobatics)
SERVER: /home/user/meuRPG/backend/internal/play/combat_move.go:493-495 comment "SRD: landing in difficult terrain asks for a DC 10 Acrobatics check or the jumper falls prone. The app rolls nothing: the master's log says it."
SERVER: /home/user/meuRPG/backend/internal/play/combat_move.go:496 LandingDifficult flag set.
SCREEN: unsure.
DOCS: unsure.
TESTS: unsure.

MECHANIC: using-each-ability / Dexterity: sleight of hand, stealth as skills
SERVER: skill catalog only (abilities.go:336 skill bonus); no specific sleight/stealth flow.
SCREEN: no match.
DOCS: no match.
TESTS: none.

MECHANIC: using-each-ability / Dexterity: hiding (Stealth vs passive Perception; cannot hide from who sees you)
SERVER: no match for hiding contest or "cannot hide from a creature that sees you" (grep furtividade/stealth/hide in play and rules/combat).
SERVER: /home/user/meuRPG/backend/internal/rules/armor.go:40-43 stealth disadvantage hint only.
SCREEN: no match.
DOCS: no match.
TESTS: /home/user/meuRPG/backend/internal/rules/derive_test.go:423 stealth disadvantage hint.

MECHANIC: using-each-ability / Constitution: hit points (CON modifier added to each Hit Die; retroactive change)
SERVER: /home/user/meuRPG/backend/internal/rules/hitpoints.go:22 con := x.mods[CON].
SERVER: /home/user/meuRPG/backend/internal/rules/hitpoints.go:33 first level hp += max(die+con, 1).
SERVER: hitpoints.go subsequent levels use con (line not located: unsure); retroactive change to max HP on a CON-modifier change: unsure.
SCREEN: unsure.
DOCS: unsure.
TESTS: unsure.

MECHANIC: using-each-ability / Constitution: checks (no skills), hold breath, march, etc.
SERVER: /home/user/meuRPG/backend/internal/rules/progression.go:235-238 SceneSave/SceneAbility generic for any ability key incl. con (ability:con is accepted).
SCREEN: /home/user/meuRPG/web/src/app/core/maps/scene-actions.ts:21-22 "Teste de habilidade" / "Teste de resistência" kinds.
DOCS: no match.
TESTS: unsure.

MECHANIC: using-each-ability / Constitution: concentration save after damage (DC 10 or half damage)
SERVER: /home/user/meuRPG/backend/internal/rules/combat/rolls.go:115-117 concentration DC 10 or half the damage, whichever higher (rounded down).
SERVER: /home/user/meuRPG/proto/meurpg/play/v1/combat.proto:2395 concentration DC rule (RN-22).
SERVER: /home/user/meuRPG/proto/meurpg/play/v1/combat.proto:3466 "The Constitution save DC to keep a concentration the damage threatened".
SCREEN: unsure.
DOCS: /home/user/meuRPG/docs/product/rules.md:336 concentration paragraph (Server: concentration spell cast...).
TESTS: unsure.

MECHANIC: using-each-ability / Intelligence: Arcana, History, Investigation, Nature, Religion checks
SERVER: /home/user/meuRPG/backend/internal/rules/abilities.go:345 PassiveInvestigation = 10 + Investigation bonus (passive Investigation).
SERVER: /home/user/meuRPG/backend/internal/rules/progression.go:215-223 skills options for any skill key.
SCREEN: /home/user/meuRPG/web/src/app/core/puzzles/puzzle-draft.ts:82 "The skill check that wins a hint: both a skill and a DC, or neither."
SERVER: /home/user/meuRPG/backend/internal/play/puzzles_hints.go:17-18 hint won by a skill check (player rolls, master-set DC).
SERVER: /home/user/meuRPG/proto/meurpg/play/v1/puzzles.proto:452-468 hint check skill and DC (1-30) master's only.
SCREEN: /home/user/meuRPG/web/src/app/pages/live-session/puzzles/hint-try/hint-try.ts:13 button names the skill, never the DC.
DOCS: /home/user/meuRPG/proto/meurpg/play/v1/puzzles.proto:608 "The DC is never sent."
TESTS: unsure.

MECHANIC: using-each-ability / Intelligence: spellcasting ability (wizard INT) for spell save DC
SERVER: spell save DC by caster ability: unsure (not located; combat_spells.go:27-51 comments on save vs attack only).
DOCS: unsure.
TESTS: unsure.

MECHANIC: using-each-ability / Wisdom: Perception search for traps (active) and passive notice
SERVER: /home/user/meuRPG/backend/internal/play/traps.go:214-259 active Perception/Investigation search with d20 + skill bonus, DC from trap.
SERVER: /home/user/meuRPG/proto/meurpg/play/v1/traps.proto:20-23 Wisdom (Perception) against DC to notice; Intelligence (Investigation) against DC to find.
SERVER: /home/user/meuRPG/backend/internal/maps/traps_play.go:266 passive notice.
SCREEN: /home/user/meuRPG/web/src/app/pages/live-session/traps/trap-search-sheet/trap-search-sheet.ts:147 second die message.
DOCS: /home/user/meuRPG/docs/product/stories.md:810 "searches and passes a Perception check ... or an Investigation check".
TESTS: /home/user/meuRPG/backend/internal/play/traps_round2_test.go:400.

MECHANIC: using-each-ability / Wisdom: Animal Handling, Insight, Medicine, Survival checks
SERVER: /home/user/meuRPG/backend/internal/rules/abilities.go:346 PassiveInsight = 10 + Insight bonus.
SERVER: no match for medicine/stabilize flow (Estabilizar exists only as spell text in docs/rules data).
SERVER: no match for animal handling / survival flow beyond generic skill options.
SCREEN: no match.
DOCS: /home/user/meuRPG/docs/product/rules.md: no match for Medicina/Sobrevivência.
TESTS: none.

MECHANIC: using-each-ability / Charisma: Deception, Intimidation, Performance, Persuasion checks
SERVER: generic skill options only (/home/user/meuRPG/backend/internal/rules/progression.go:215-223); no special flow.
SCREEN: no match beyond generic scene actions.
DOCS: no match.
TESTS: none.

MECHANIC: using-each-ability / Charisma: spellcasting ability for paladin/sorcerer/warlock/bard
SERVER: unsure (not located).
DOCS: unsure.
TESTS: unsure.

MECHANIC: saving-throws / save = d20 + save bonus (class saves + proficiency)
SERVER: /home/user/meuRPG/backend/internal/rules/abilities.go:208-232 savingThrows(): class saves plus extra save proficiency; SavingThrow.Bonus.
SERVER: /home/user/meuRPG/backend/internal/rules/progression.go:235-238 SceneSave: Bonus = s.Bonus; name "Teste de resistência de ...".
SERVER: /home/user/meuRPG/backend/internal/play/combat_spells_view.go:33-38 saveView: outcome and d20 + bonus for master and target.
SERVER: /home/user/meuRPG/backend/internal/play/combat_events.go:91-92 basic-sheet NPC has no bonus: roll is d20 + 0 and master may overrule.
SERVER: /home/user/meuRPG/backend/internal/play/combat_traps.go:229-233 savesEventOf; combat_traps.go:552-557 trap save roll view.
SERVER: /home/user/meuRPG/backend/internal/play/traps_effect.go:196-201 trap save: d20 + t.save vs DC.
SCREEN: /home/user/meuRPG/web/src/app/pages/character-sheet/proficiency-column/proficiency-column.html:10 savingThrows list.
SCREEN: /home/user/meuRPG/web/src/app/pages/character-sheet/combat-column/combat-column.html:33 "Teste de resistência de {{ ability }}".
DOCS: /home/user/meuRPG/docs/product/glossary.md:82 "Saving throw (teste de resistência)".
TESTS: unsure.

MECHANIC: saving-throws / class saving-throw proficiencies (two per class)
SERVER: /home/user/meuRPG/backend/internal/rules/abilities.go:208-216 starting class's SavingThrows.
SERVER: /home/user/meuRPG/backend/internal/rules/overlay_class.go:188-198 exactly two saves, must differ.
SERVER: /home/user/meuRPG/backend/internal/rules/content.go:616-618 saves list per class.
SCREEN: /home/user/meuRPG/web/src/app/pages/content/class-editor/class-editor.html:18-19 two "Teste de resistência" selects.
DOCS: unsure.
TESTS: unsure.

MECHANIC: saving-throws / DC from the effect (spell DC by caster; trap DC by master)
SERVER: /home/user/meuRPG/backend/internal/play/traps_effect.go:187-203 trap save uses trap's DC (sv.GetDc()).
SERVER: /home/user/meuRPG/proto/meurpg/play/v1/combat.proto:2823-2824 SaveResult dc, master and caster's player only.
SERVER: /home/user/meuRPG/proto/meurpg/play/v1/combat.proto:2816-2820 SaveResult d20 plus bonus; flag for basic NPC.
SCREEN: /home/user/meuRPG/web/src/app/pages/maps/point-kinds/trap-point-panel.html:199 DC input for a save part.
DOCS: unsure.
TESTS: unsure.

MECHANIC: saving-throws / success halves or negates damage (half on save)
SERVER: /home/user/meuRPG/backend/internal/rules/combat/rolls.go:136-139 HalfDamage (rounded down) for "half as much damage on a successful save".
SERVER: /home/user/meuRPG/backend/internal/rules/content.go:807 spellCorrectionSaveSuccess = half, none, other.
SERVER: /home/user/meuRPG/backend/internal/rules/traps.go:50-51 TrapPassHalf; traps.go:395-396 save.on_pass must be half or none.
SCREEN: /home/user/meuRPG/web/src/app/pages/maps/point-kinds/trap-point-panel.ts:69-71 save part with what a pass does.
DOCS: /home/user/meuRPG/docs/product/stories.md:809 trap saving throw effect.
TESTS: unsure.

MECHANIC: saving-throws / death saving throw (d20, no modifier, not a normal save)
SERVER: /home/user/meuRPG/backend/internal/play/combat_death.go:133 RollDeathSave: s.d20(in, 0), no bonus.
SERVER: /home/user/meuRPG/backend/internal/play/combat_death.go:43-53 deathOutcomeOf: 20 revived (1 HP), 1 critical failure, 10+ success, else failure.
SERVER: /home/user/meuRPG/backend/internal/play/combat_death.go:139 DeathOutcome recorded in event.
SCREEN: /home/user/meuRPG/web/src/app/core/combat/death-saves.ts:31 "Teste contra a morte: <roll>".
SCREEN: /home/user/meuRPG/web/src/app/core/combat/combat-log.ts:417 "rola o teste contra a morte".
SCREEN: /home/user/meuRPG/web/src/app/core/combat/combat-errors.ts:141 "Role o teste contra a morte antes de encerrar o turno."
DOCS: /home/user/meuRPG/docs/product/glossary.md:60 "10 or more is a success, less is a failure, a natural 1 is two failures".
DOCS: /home/user/meuRPG/docs/product/stories.md:299 "Rolar teste contra a morte" button (RN-03).
DOCS: /home/user/meuRPG/proto/meurpg/play/v1/combat.proto (death save notes; line not located).
TESTS: /home/user/meuRPG/backend/internal/play/combat_tablerules_test.go:448 TestRN24_HiddenDeathSavesAreTheOwnersAndTheMasters; :588 TestRN24_ARevivalAndADeathStayWithTheOwnerAndTheMaster.
TESTS: /home/user/meuRPG/backend/internal/play/combat_joint_test.go:385 TestMR013_EachMemberOwesItsOwnDeathSave.

MECHANIC: saving-throws / death save visibility (table rule: only owner and master see it)
SERVER: /home/user/meuRPG/backend/internal/play/combat_view.go:50-53 hideDeath: death saves of others hidden when table rule set; never for master.
SERVER: /home/user/meuRPG/backend/internal/play/combat_fog.go:215, :231 hideDeath set from rules.DeathSavesHidden.
DOCS: /home/user/meuRPG/docs/product/stories.md:299 owner's death save.
TESTS: /home/user/meuRPG/backend/internal/play/combat_tablerules_test.go:686 TestRN24_VisibleDeathSavesAreUnchanged.

MECHANIC: saving-throws / saves against spells and traps: one roll per creature, DC compared with total
SERVER: /home/user/meuRPG/backend/internal/play/combat_spells.go:558-560 comment: saving throw's roll per target.
SERVER: /home/user/meuRPG/backend/internal/play/traps_effect.go:187-192 saves for each creature caught, or only on hit when applies_to says so.
SCREEN: /home/user/meuRPG/web/src/app/pages/maps/point-kinds/trap-point-panel.ts:69-71 save part description.
DOCS: /home/user/meuRPG/proto/meurpg/play/v1/combat.proto:3589-3590 every saving throw in order.
TESTS: unsure.

MECHANIC: saving-throws / saves with advantage or disadvantage (GM decides)
SERVER: roll_mode saves exist only as hints (see advantage block above).
SERVER: /home/user/meuRPG/backend/internal/play/combat_spells_view.go:33-38 no advantage input.
SCREEN: no match for vantagem input on save rolls.
DOCS: /home/user/meuRPG/docs/product/glossary.md:89 hint, master decides.
TESTS: /home/user/meuRPG/backend/internal/rules/overlay_test.go:632 Gnome-style advantage hint on a save (feature:gen-none-olhos).

MECHANIC: saving-throws / Gnome Cunning and race save advantage as text only
SERVER: /home/user/meuRPG/backend/internal/rules/srd51/effects/races.json:94 advantage INT/WIS/CHA saves vs magic (text_pt).
SCREEN: /home/user/meuRPG/web/src/app/pages/character-sheet/character-sheet.spec.ts:1042 hint text shown.
DOCS: unsure.
TESTS: /home/user/meuRPG/backend/internal/rules/derive_test.go:185-186 Gnome Cunning hint.
