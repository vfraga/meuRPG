# MeuRPG dress rehearsal: UI/UX review of the 787 screenshots

Reviewed 09/10/2026 against `origin/main` = `78ce6b53` (the commit the rehearsal ran on; **main has not moved since**, so nothing is "fixed on main"), open PuraFome/meuRPG PRs #272 to #278, the fork branches `fix/w6-pm02-area-spells` and `feat/w7-gcs-blob` (the only `feat/w7-*` branch pushed so far; the other wave-7 units have task files only), `scratch-ms/cloud/consolidation/{fix-wave-premvp,rehearsal-verified,class-matrix}.md`, `scratch-ms/cloud/wave7/task-*.md` and `scratch-ms/design/premvp/README.md` (+ boards PM-01 to PM-09, W7-*).

Screenshots live in `/Users/viniciusf/personal/rpg-computer-use/scratch-ms/rehearsal/shots/` (paths below are relative to it). Coverage by viewport: master = 1280 x 800 (355), p1 = phone 390 (178, at 2x), p2 = tablet 768 (138), p3 = 1280 dark/light (116). **Gap: the master was never shot on a phone**, and no state was shot at 320 px. Contrast was judged by eye and with 2x crops (no tool was available to measure); nothing failed visibly, the one dark-theme concern is the dim struck-through "Ação" label of a spent action (shots `c2c-08-mei-stun-offer-p3.png`), which is meant to be dim and is repeated in words ("Usada").

Statuses: **FIXED-PR** (in an open PR or fork branch, not on main yet: re-check after merge), **PLANNED** (a unit or board covers it), **OUTSTANDING**, **BY DESIGN** (no action). "possibly fixed" means the change touches the area but I could not confirm it.

Severity: no screenshot showed a *blocking* defect that is still open (the one blocking functional thing, RH-49 monsters placed inside walls, is fixed in #274). Two items are **high**.

---

## 1. Character sheet (player and master)

| # | Issue | Sev | Screenshot | Status | Cause and fix |
| --- | --- | --- | --- | --- | --- |
| A1 | **The player's private notes come first, before any game number.** On a phone the header card (name, 4 fields, XP card, "Editar ficha") plus the full "Anotações" panel take about two screens before the first ability score; at 768 the notes span the full width above the medallions. `docs/design.md:16` says "the game number comes first". | high | `c2f-12-ficha-p1.png`, `c4e-17-grug-ficha-depois-p1.png`, `act5a-05-p1-sheet-p1.png`, `c3p-13-sheet-final-p2.png` | OUTSTANDING | `web/src/app/pages/character-sheet/character-sheet.scss:190-207`: `.sheet--pnotes` puts `'pnotes'` first in `grid-template-areas` below 1200 px. Fix: move `pnotes` after `combat`/`features` (or collapse it to one "Anotações (N)" row that opens on tap); keep it first only >= 1200 px. |
| A2 | On a phone the combat block (HP, AC, initiative, attacks, spells) comes **after** the 18-line saves-and-skills column. (Derived from the CSS and from where the phone shots stop; no phone shot scrolls that far.) | medium | `c2f-12-ficha-p1.png` (everything above the numbers) | OUTSTANDING | `character-sheet.scss:108` `'abilities' 'proficiencies' 'combat' 'traits' 'notes'` (and `:192`). Fix: on <768 px use `'abilities' 'combat' 'proficiencies' ...`; `docs/design.md:17` says "same order in one column" for the paper sheet, so change both. Possibly touched by W7-I (inventory on the sheet, board W7-Ia): re-check after it lands. |
| A3 | "Jogador sem nome" is printed in the sheet header (the player's own sheet!) and in the master's "Luz dos personagens" and member rows whenever the account has no display name. | medium | `act5e-08-ficha-p1-p1.png`, `c1-review-master.png`, `act3b-33-stage-maga-master.png` | OUTSTANDING | `sheet-header/sheet-header.ts:94` (`?? 'Jogador sem nome'`), `light-panel.ts:48`, `campaign-detail.copy.ts:25`. Fix: hide the "Jogador" field when there is no name (own sheet: show "Você"); in master lists show "Sem nome no perfil" in muted text. |
| A4 | Feature descriptions on the sheet are SRD English text with **no label** saying so; spells, items, the bestiary and wild-shape traits all carry "Texto do SRD 5.1 (em inglês)". A player reads 10 lines of English under "Rajada de Golpes" with no cue it is expected. | medium | `c2m-13-ficha-expandida-p3.png`, `c3w-11-ficha-combate-p3.png`, `c2e-07-features-expanded-p1.png` | OUTSTANDING | `web/src/app/pages/character-sheet/features-panel/features-panel.ts` (comment says "the SRD's English text"; no caption). Fix: one caption under the panel title, same wording and `lang="en"` block as `shared/spell-details/spell-body.html:33`. |
| A5 | "Faltam **1 perícias** para escolher." | low | `act5e-08-ficha-p1-p1.png`, `c4e-17-grug-ficha-depois-p1.png` | OUTSTANDING | `backend/internal/rules/choices.go:152` (unchanged in #276). Fix: use the existing `countPT(n, "perícia", "perícias")` helper (`choices.go:218`). |
| A6 | Negative modifiers on the sheet use a hyphen ("-1"), the rest of the app uses the real minus ("−1", e.g. bestiary, combat). | low | `c3w-11-ficha-combate-p3.png` vs `act3b-02-mage-sheet-master.png` | OUTSTANDING | `core/characters/character-labels.ts:65` `formatModifier` returns `${modifier}`. Fix: `modifier >= 0 ? '+' + m : '−' + Math.abs(m)`. |
| A7 | A pending character's sheet called notes/creatures APIs and showed the notes panel. | medium | `c1-review-master.png` | FIXED-PR #276 (RH-01) | `character-sheet.html` now shows "Aparece quando o mestre aprovar o personagem." |
| A8 | Sheets lock at "Iniciar sessão" with open choices and nothing warns; the player then cannot finish them. | medium | `act5e-08-ficha-p1-p1.png` (warning notice on a locked sheet) | FIXED-PR #276 (warning in `game-session-card`, RH-08); the "Completar escolhas pendentes" screen is PLANNED in K1 / board PM-05d | |
| A9 | Master can only "Aprovar" or "Recusar" (which deletes); the banner says so in a long sentence. | medium | `c1-review-master.png` | PLANNED: `feat/w7-review-revive` / board PM-08a ("Pedir ajustes") | |

## 2. Character creation and editor

| # | Issue | Sev | Screenshot | Status | Cause and fix |
| --- | --- | --- | --- | --- | --- |
| B1 | Phone stepper shows only numbered circles; "Passo 2 de 4" becomes "de 5" once a caster class is picked. | low | `c2f-05-digitar-p1.png` | BY DESIGN (RH-10, `editor-stepper.ts:27`); K1 adds a step (PM-05a "Passo 3 de 6") | Optional: show the current step's name next to the circles. |
| B2 | In the skills step the column headers "Habilidade" and "Especialização" touch ("HabilidadeEspecialização") at 390 px. | medium | `c2f-07-pericias-p1.png` | OUTSTANDING (#276 edits `skill-picker.html/scss`, so re-check) | `character-editor/skill-picker/skill-picker.scss` header row. Fix: abbreviate to "Espec." on <480 px or hide the header on a phone (the checkboxes carry `aria-label`s). |
| B3 | Skill and cantrip pickers say "Nenhuma perícia marcada" / "N truques escolhidos" with no "de M" (the limit only shows as an error after saving). | low | `c2f-07-pericias-p1.png` | possibly fixed by #276 (skill-picker.ts +33, spell-picker `prepareWarning`): re-check | RH-45/RH-36. |
| B4 | The features that need a choice (Estilo de Luta, ancestry, invocations, Favored Enemy, Metamagic, half-elf +1s...) are never asked; the sheet shows "Choose one of the following options..." in English. | high | `c2f-13-estilo-de-luta-p1.png` | PLANNED: `feat/w7-creation-choices` (K1) / boards PM-05a to PM-05d | RH-35/44/46/53/54. |
| B5 | A level-5 character is created with 0 XP ("Faltam 14.000 XP"). | low | `c3c-12-ficha-topo-p1.png` | FIXED-PR #276 (RH-02) | |
| B6 | Weapons multi-select opens over the form and "Criar personagem" does nothing while it is open. | low | `c2f-10-armas-p1.png` | BY DESIGN (Material overlay, RH-05 is tooling) | |

## 3. Level-up

The level-up wizard is the best-finished flow in the set (clear steps, "O que muda", locked-rest-of-sheet note). Only polish:

| # | Issue | Sev | Screenshot | Status | Cause and fix |
| --- | --- | --- | --- | --- | --- |
| C1 | The summary step's cards are about 640 px wide on the 768 tablet while every other step and the page header span the full width. | low | `act5d-21-zeph-resumo-p2.png` vs `act5d-20-zeph-preparadas-p2.png` | OUTSTANDING | `pages/level-up/*summary*.scss` max-width. Fix: same width as the other steps. |
| C2 | The summary does not list weapon attack/damage changes (RH-43). | low | `act5d-07-levelup-resumo-p1.png` | OUTSTANDING (known, `core/levelup/levelup-summary.ts:180-300`) | |
| C3 | No way to add a class at level-up. | medium | `c2d-19-brann-sem-multiclasse-p1.png` | PLANNED: `feat/w7-multiclass-levelup` / boards PM-08b, PM-08c | |

## 4. App shell, header, live banner

| # | Issue | Sev | Screenshot | Status | Cause and fix |
| --- | --- | --- | --- | --- | --- |
| D1 | On a phone the "A sessão 1 de X começou" notice is ~280 px tall (icon, two lines, full-width button, close) and repeats on every page (sheet, level-up, map), pushing the content a third of a screen down; the header already has the "Ao vivo" pill. | medium | `act5d-03-levelup-vida-rolar-p1.png`, `act3c-08-map-p1.png`, `act5a-05-p1-sheet-p1.png` | OUTSTANDING | `shell/live-notice/live-notice.scss` (phone: column layout, `padding-top: 11px`, 44 px button) and `app.html:60`. Fix: one row on a phone (text + "Entrar" text button + close, about 64 px) and hide it on pages of that campaign. |
| D2 | At 768 px, in a session, the app bar wraps: "MeuRPG" sits alone in a ~30 px row at the very top and the nav, "Anotações", "Minha conta" and "Sair" drop to a second row (header ~125 px instead of 64). | medium | `act4c-12-p2-escudo-prompt-p2.png`, `act3d-12-map-fullscreen-p2-p2.png`, `c3c2-17-cura-maos-p2.png` | OUTSTANDING | `app.scss:25-43`: `.app-bar { flex-wrap: wrap; gap: 0 var(--mr-space-8) }` at >= 768 px; the Anotações slot adds a fourth item. Fix: `gap: var(--mr-space-4)` between 768 and 1023 px, or move "Anotações" into the user menu there. |

## 5. Combat and the live session

| # | Issue | Sev | Screenshot | Status | Cause and fix |
| --- | --- | --- | --- | --- | --- |
| E1 | Phone turn: the sticky bar repeated the four tiles and covered ~40% of the screen and the initiative cards. | medium | `act4b-13-p1-combat-view-p1.png`, `c2b-01a-brann-start-p1.png` | FIXED-PR #274 (`turn-bar.ts`: one slim row, no repeat; RH-60) | |
| E2 | Phone turn: the four status tiles are two rows of ~90 px boxes (half empty), then the initiative strip; "Atacar"/"Conjurar" are two to three screens down. The thing the player came for is not prominent. | high | `act4b-13-p1-combat-view-p1.png`, `c4d-06-grug-painel-p1.png`, `act5a-16-p1-combat-p1.png` | OUTSTANDING (#274 only shrinks the bar) | `pages/live-session/combat/turn-panel/turn-panel.scss:92-109`: 2-column grid with `min-height: 88px` per tile on a phone. Fix: one compact row of four chips (about 56 px, icon + word) and put the "Ação"/attacks block directly under the title; order strip below the actions. |
| E3 | Fireball: only a checklist of everyone ("pode ser ninguém"), no area, allies included, a slot spent on nobody (RH-20, RH-25). | high | `act4d-05-p2-fireball-dialog-p2.png` | FIXED-PR `fork/fix/w6-pm02-area-spells` (area picker, "ninguém na área" confirmation, hidden reveal; boards PM-02a-d). Not a PR yet. | |
| E4 | A cantrip dialog lists 1º/2º/3º level all "Sem espaço livre". | low | `act4c-02-p1-raio-lunar-dialogo-p1.png` (levels shown for a leveled spell) and RH-06 shot | FIXED-PR #274 (`cast-flow.ts` slotRows returns [] for level 0) | |
| E5 | "Você foi atingido" modals pile up and cover "Rolar teste contra a morte" after the master answers. | medium | `act4e-03-death-save-dialog-p2.png` | FIXED-PR #274 (`shield-sheet.ts` closes itself with a snackbar; RH-27) | |
| E6 | Spell dialog on a phone: the primary button text "Conjurar Raio Lunar" wraps to two lines with the icon stuck to the first line, next to a one-line "Cancelar". | low | `act4c-02-p1-raio-lunar-dialogo-p1.png` | OUTSTANDING | `combat/cast-sheet/cast-sheet.html:132-145` two equal-width buttons. Fix: stack the buttons below 480 px (as the Shield sheet does) or label it "Conjurar" and keep the name in the title. |
| E7 | Reactions: NPC Shield, Counterspell, Uncanny Dodge etc. have no prompt; the turn does not wait. | medium | `act4c-17-p2-escudo-prompt-goblin1-p2.png` | PLANNED: `feat/w7-reaction-window` (K6/D11) / boards PM-04a-d | |
| E8 | The dead player's summary shows "Combatentes 3, Derrotados 0 de 0" (RH-26). | medium | `act4e-32-p2-summary-p2.png` | possibly fixed by #274 (touches `combat-summary.html/.ts`, "personagem morto"): re-check | Server-side vision cause (`ListPartyVision status='active'`). |
| E9 | In the three-column desktop combat the map is tiny when the explored area is a small part of the grid: tokens ~12 px, nothing readable, black field around it. | medium | `c2c-08-mei-stun-offer-p3.png` | OUTSTANDING | `shared/combat-map/combat-map.scss:11-20` sizes a square as `100cqw / cols`. Fix: crop the viewport to the seen bounding box (+2 squares) or start zoomed on the player's own token as the phone "Mover" page does. |
| E10 | In the 1280 layout the "Usada" pill beside the "Ação" heading has no padding and touches the panel edge. | low | `c2c-08-mei-stun-offer-p3.png` (crop), `act4d-03-form-dropped-p3.png` family | OUTSTANDING | `turn-panel/action-groups` pill; the same pill has padding on the phone (`act5a-19-p1-move-dialog-p1.png`). Fix: same padding at all widths. |
| E11 | Initiative setup says the same thing three times: the warning ("X, Y e Z ainda não rolaram"), a checklist line ("Faltam as iniciativas de..."), and the reason under the disabled button. | low | `act4a-22-initiative-master.png` | OUTSTANDING | `combat/initiative-side/initiative-side.html:8-18`, `:28`, `:47`. Fix: drop the checklist line, keep notice + reason. |
| E12 | In the master's order list, long names wrap badly ("Lirio Voz-de-" / "Prata", "Ilaria / Folhaprata" over three lines with the chip). | low | `act3-01-session-master.png`, `act4e-26-confirm-death-dialog-master.png` | OUTSTANDING (cause not confirmed) | `combat/order-list/order-list.scss:41` `--cols` at >= 768 px. Fix: give the name column a `minmax(9rem, 1fr)`, move "Concentração" below the name. |
| E13 | Monsters placed inside wall squares were never seen by the players (RH-49), monsters at "Começar" overlapping. | high (functional) | `c3b-24-colocados-master.png` | FIXED-PR #274 (`token-spot.ts`) | |
| E14 | Dice: "Digitar o resultado" focus ring is a double outline in the typed field (border + ring). | low | `act3d-20-typed-dice-p2-p2.png` | OUTSTANDING (verify: may be a Playwright focus artefact) | `roll-picker` field, `docs/design.md:226` says the border is the focus. |

## 6. Maps (master)

| # | Issue | Sev | Screenshot | Status | Cause and fix |
| --- | --- | --- | --- | --- | --- |
| F1 | "Pontos | Pintar" and "Pincel 1x1 | 3x3": the labels of the unselected segment sit off-centre (a hidden check icon still reserves space). | low | `act3-04-pintar-master.png`, `act3-14-token-dialog-master.png` | OUTSTANDING | `pages/maps/editor-bar/editor-bar.html:4` + `editor-bar.scss:78` `.chk--hide { visibility: hidden }`. Fix: `display: none` when hidden (and `min-width` on the segment so nothing jumps), or put the check absolutely. |
| F2 | Without a grid, the paint tools are dashed/disabled and the only hint is a line of text; no button next to it, and the calibration card ("Cada quadrado deste desenho vale") is far down the page. | medium | `act3-04-pintar-master.png` | OUTSTANDING | `maps/map-editor/editor-painting.ts:98`, `core/maps/map-errors.ts:89`. Fix: turn the hint into "Defina a grade para pintar" + a button that scrolls to/focuses the calibration card. |
| F3 | New tokens all stacked on one square; default brush "Terreno difícil"; placement ignoring walls (RH-16, RH-11, RH-49). | medium | `act3b-14-token-place-master.png` | FIXED-PR #274 (`token-spot.ts`, `paint-tools.ts`: no tool until chosen) | |
| F4 | A visible rectangular focus ring around headings after a click ("Cada quadrado deste desenho vale", "Mudar para 'por marcos'?"). | low | `act3-06-calibrate-master.png`, `act5c-36-xp-dialog-master.png` | OUTSTANDING (verify: likely programmatic `focus()` on `tabindex=-1` headings after a mouse click) | Use `:focus:not(:focus-visible)` reset on those headings. |

## 7. Scenes, stage, puzzles

| # | Issue | Sev | Screenshot | Status | Cause and fix |
| --- | --- | --- | --- | --- | --- |
| G1 | Stage hint "Escolha um personagem para ver maior" shows even when there is one NPC; reads like an instruction for a missing control. | low | `act3c-04-scene-olhar-p1.png` | OUTSTANDING | `live-session/scene/stage-player/stage-player.html:27`. Fix: show only with 2+ portraits, wording "Toque num retrato para ver maior". |
| G2 | On a phone the stage image takes a whole screen before "O que você pode fazer" (the scene's actions). | medium | `act3c-04-scene-olhar-p1.png` | OUTSTANDING | `stage-player` scss. Fix: cap the stage height at ~45 vh on a phone; actions above the fold. |
| G3 | In the master's scene rolls, a roll with no character name shows a "?" avatar and no name. | low | `act5c-25-master-live-puzzle-master.png` | OUTSTANDING | `scene/scene-roll-line/scene-roll-line.html:3` (`roll().characterName` empty). Fix: fall back to "Personagem removido". |
| G4 | Puzzle counter "Suas tentativas 2 de 2" does not say whether those are used or left (it drops to "1 de 2" after a miss, so it is "left"). | low | `act5c-18-p1-puzzle-open-p1.png`, `act5c-22-p3-wrong-p3.png` | OUTSTANDING | `core/puzzles/puzzle-format.ts:300`. Fix: "Tentativas restantes: 2 de 2". |
| G5 | After the session ends there is no way back to its summary (RH-34a); the end screen only exists once. | medium | `act5e-03-encerrada-master.png` | FIXED-PR #273 (PM-01, past sessions panel + summary route) | |

## 8. Campaign pages

| # | Issue | Sev | Screenshot | Status | Cause and fix |
| --- | --- | --- | --- | --- | --- |
| H1 | A player's campaign page puts "Experiência" ("Nenhum personagem de jogador vivo nesta campanha") and "Como você rola os dados" above "Meus personagens" with "Criar meu personagem", which is what a new player came for. | medium | `c4r-01-invite-p2.png` | OUTSTANDING (the claim-link board PM-09 / `feat/w7-claim-links` adds reserved characters to the same page: re-check the order then) | `campaign-detail.html:61-78`. Fix: for a player render `app-campaign-characters` right after the session card; hide the empty XP panel. |
| H2 | "Criar campanha": the error "Escolha um modo de XP." is glued to (and clipped by) the "Criar campanha" button. | low | `c1-created-master.png` | OUTSTANDING | `pages/campaigns/campaigns.html:20-23`, `campaigns.scss:36` (`.create__form` has no gap above the button). Fix: `margin-top: var(--mr-space-3)` on the submit, or `subscriptSizing: 'dynamic'`. |
| H3 | "Ver em tela cheia" of a shown image: the image stays at ~480 px inside a 690 px dialog with large empty margins. | low | `act3d-12-map-fullscreen-p2-p2.png` | OUTSTANDING (cause not read) | `shown-image-block` / dialog: `object-fit: contain; max-height: calc(100vh - 160px)`. |

---

## Counts

| Status | Count |
| --- | --- |
| Fixed on main | 0 |
| FIXED-PR / branch | 10 (A7, A8, B5, E1, E3, E4, E5, E13, F3, G5) |
| PLANNED | 4 (A9, B4, C3, E7; plus the "Completar escolhas" half of A8) |
| Possibly fixed, re-check after merge | 2 (B3, E8) |
| By design | 2 (B1, B6) |
| **OUTSTANDING** | **28**: high 2 (A1, E2), medium 10 (A2, A3, A4, B2, D1, D2, E9, F2, G2, H1), low 16 (A5, A6, C1, C2, E6, E10, E11, E12, E14, F1, F4, G1, G3, G4, H2, H3) |

(Severity of the rest: no blocking; A1 and E2 are the only high that stay open. B4, E3 and E13 are high and already covered.)

## Outstanding, in order of what a player or master would notice first

1. **Phone turn screen (E2).** The first thing a player does in combat: attacks are two to three screens below four half-empty tiles. Compact the tiles into one row, put the actions first.
2. **Character sheet order (A1, A2).** Notes first, combat block after the skills list. The numbers used in play should be what opens.
3. **Live banner on every phone page (D1)** and **the wrapped app bar at 768 (D2)**: seen on every screen of the evening.
4. **Combat map unreadable in the dark 3-column layout (E9)** and **stage image pushing the scene's actions off the phone (G2).**
5. **"Jogador sem nome" on every sheet (A3)** and **English feature text without a label (A4).**
6. **New player's campaign page order (H1)**, **paint tools disabled with no way forward (F2)**.
7. Polish: the segmented-control label offset (F1), skills header collision (B2), spell button wrap (E6), repeated initiative message (E11), "Usada" pill padding (E10), long names in the order list (E12), grammar "1 perícias" (A5), hyphen vs minus (A6), puzzle counter wording (G4), "?" roll avatar (G3), stage hint copy (G1), create-campaign error spacing (H2), summary card width (C1), fullscreen image size (H3), focus rings on headings (F4) and the double ring on the typed die field (E14).

## Method notes

- No haiku subagents: the file names already group by act and screen, so I reviewed the distinct screens directly (about 90 full-size views plus 2x crops of dense areas).
- Everything was read-only: `git fetch`, `git show origin/main:...`, `git diff origin/main...<branch> -- web/`; no checkout, no worktree, no server.
- Items marked "cause not confirmed" need one look at the component before a fix is written.
