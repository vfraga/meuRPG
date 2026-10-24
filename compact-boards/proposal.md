# MeuRPG: compact screens, less scrolling to act

A design proposal for Vinicius and the orchestrator, to brainstorm with Samuel. Prose in English; every piece of UI copy in Portuguese, as the app writes it. Nothing here changes the repository.

## In one screen

The live session grew panel by panel, and every panel is always open, one under the other. The result, measured on the current `main`:

- The master's **Conversa composer sits 4,800 to 5,200 px down the page at 1280 px** (in RP and in combat): the column beside the stage only exists from 1,600 px. A player at 1280 px reaches it at 2,500 to 3,100 px.
- On a phone the "Cena · Ficha · Conversa" tabs **sit at the end of the page** (3,300 px for a player, about 7,500 px for the master), so the unread badge is only seen after scrolling to the bottom.
- The master's **"Próximo turno" is not sticky**: every NPC turn is about three scroll moves (down to roll and apply damage, back up to pass the turn), about 60 per fight of four goblins over five rounds.
- The master's **campaign home is about 4,850 px at 1280** (7,500 at 390): approving a character is four and a half screens down, creating an NPC five to six.
- On a player's phone, **"Mover" is two to three screens below the turn**, under every action group, including the spent ones.

The proposal, in one line each:

1. A **compact session bar** (one 56 px row) replaces the session header on every live screen.
2. A **sticky rail** on the right from 1024 px: the Conversa is always on screen beside the stage; in combat the rail holds "Turno" (what waits for the master and the card of the turn) and "Conversa", beside the order and the map.
3. A **sticky combat bar** with "Próximo turno" for the master; a sticky turn bar with "Mover" and "Encerrar turno" for the player.
4. **Tabs where the jobs differ**: the master's session "Cena · Mapa · Grupo"; the campaign home "Mesa · Preparação · Regras" (each in the URL).
5. On a phone, **"Cena · Ficha · Conversa" pinned to the bottom**, and "Ficha" becomes a panel inside the session instead of leaving it.

Decided by Vinicius and Samuel: the master's combat is **B "Três colunas"**, with the order column folding to a strip of tokens; the campaign home is **A "Abas"**, with icons on the tabs and panel titles; questions 3 to 13 as proposed (§6). They added two points: the player's phone combat keeps the map on top and gets two layouts to compare (movement on the map or right under it; actions by kind or by the turn's economy), and a scene can be open to only some players, which the scene shows. After the first review: the player's phone combat keeps the movement in a row under the map (nothing floats on the map) and the actions in tabs that come from the character's turn options (Ataques; Magias only for a caster; a tab named for the class when it grants actions, with uses and pool; Outras); the turn's economy is drawn as shapes (circle Ação, triangle Bônus, square Reação, a walking figure), filled when available; in the Conversa the viewer's own lines sit on the right; the master's scene gets an eye toggle per NPC and per action (new) next to what exists (one image shown at a time, clues revealed to chosen players); and every list of people is drawn with one and with eight players. The boards are in `project/` (canvas "MeuRPG — Telas compactas").

---

## 0. Sources and method

- **Code:** `origin/main` at `336bd2e1d` (21/10/2026), read through `git archive` into the scratchpad. The `meuRPG/` working tree is a detached `05f90e519` (16/10), from before the Conversa (#480), so I did not read screens from it. Nothing under `meuRPG/` was touched.
- **Templates read:** `pages/live-session/live-session.html` and `.scss`, `combat/combat-view.html` and `.scss`, `combat-bar`, `npc-card`, `order-list`, `turn-panel` (and `turn-bar.ts`), `conversation/*` (`session-tabs`, `conversation-panel.scss`, `master-composer`), `scene/scene-open`, `session-header`, `party-panel`, `rest-card`, `effects`, `casting`, `pages/campaign-detail/*`; docs `docs/design/{README,combat,session,components}.md`, `docs/product/stories.md` (MR-011 to MR-013).
- **Screenshots used** (all under `scratch-ms/`): `rehearsal/shots/act3b-29-session-live-master.png`, `act3b-31-scene-live-master.png`, `act3b-33-stage-maga-master.png`, `act3b-37-actions-section-master.png` (master RP at 1280, 09/10); `pr-screens-repo/rehearsal-ui/act3-01-session-master.png`, `act4e-26-confirm-death-dialog-master.png` (master combat 1280); `pr-screens-repo/etapa10-theatre-web/02-mestre-vez-toren-d1280-light.png`; `pr-screens-repo/combat-screens/05-*.png`, `06-combate-do-mestre-dark-390px.png` (full page, 390 x 2,108); `pr-screens-repo/etapa9-nevoa/combate-faixa-390-escuro.png` (full page, 390 x 3,100); `pr-screens-repo/t5-u8-bonus-attacks/monge-rajada-1-390-light.png` and `-1280-light.png` (the player's action groups); `pr-screens-repo/cenas-de-rp/390x844-dark-e3-player-scene.png`, `390x844-light-e5-open-scene-collapsed.png`; `pr-screens-repo/scene-images/painel-desktop-claro.png`, `cena-desktop-claro.png`; `pr-screens-repo/conversa-web/board-MasterDesktop-dark-1280.png` (the approved board: stage left, Conversa right, at 1280); `pr-screens-repo/etapa5-sessao/final-campanha-mestre-desktop-light.png`; the review `pr-screens-repo/rehearsal-ui/README.md`. The newest screenshots predate panels added since (held rolls, effects, casting, free dice, the Conversa), so those heights come from the templates.
- **How heights are estimated:** measured panel heights from the screenshots above (full-page shots divided by the device scale; 1280 x 800 viewport shots for single panels), and, for panels with no screenshot, rows x row height from the template (list rows at least 56 px, buttons 44 px, 48 px for a phone's filled button, panel padding 24 px, gaps 16 to 24 px). The data: a table of 4 characters (Pensantus, Brisa, Toren and one more), 4 enemies (goblins and a cultist), a 24 x 16 map with fog, 3 points, 2 traps, 1 treasure. A "screen" is 736 px at 1280 x 800 (under the 64 px app bar) and 788 px at 390 x 844. Ranges cover a quiet and a busy moment. Expect +-15 %.
- **Steering from Vinicius and Samuel** (their meeting): the master uses a laptop, so the master's screens are designed for 1280 px first (390 must still work); players use phones and laptops about equally; this round is combat, the RP scene outside combat, and the master's campaign home in tabs; the character sheet only where it is part of acting.

## 1. Screen inventory

| # | Screen (route) | Its one job | Who | Device | Scroll problem |
| --- | --- | --- | --- | --- | --- |
| 1 | Campaign home, master (`/campaigns/:id`) | Start or enter the session, prepare, manage the table | Master | Laptop | **Yes**: about 4,850 px at 1280 |
| 2 | Campaign home, player | Enter the session, make and open my character | Player | Both | Some (about 2,800 px at 390) |
| 3 | Live session outside combat, master (`/campaigns/:id/session`) | Run the scene: narrate, stage, images, scene rolls, map, party | Master | Laptop | **Yes**: about 5,100 to 5,700 px at 1280 |
| 4 | Live session outside combat, player | Follow the scene, roll, talk, read my numbers | Player | Both | **Yes** on a phone (3,300 px) and for the Conversa at 1280 |
| 5 | Initiative (combat setup), master | Collect initiatives, place tokens, start | Master | Laptop | Small ("Começar o combate" sits in the side column) |
| 6 | Combat running, master | NPC turns, the waiting answers, HP fixes, pass the turn | Master | Laptop | **Yes**: about 4,800 to 5,600 px at 1280 |
| 7 | Initiative, player | Roll or type the d20 | Player | Both | No |
| 8 | Combat running, player | Act on my turn, react off turn, follow | Player | Both | **Yes** on a phone (3,300 to 3,800 px) |
| 9 | "Mover", "Saltar", "Ver mapa" (`move-page`) | Choose a square | Player (master reads) | Both | No (full page by design) |
| 10 | Combat summary, XP, session summary | Read the result, give XP | Both | Both | No |
| 11 | Open puzzle (player) | Solve it | Player | Both | No |
| 12 | Character sheet (`/characters/:id`) | Read and edit the sheet | Player | Both | Out of this round, except the session's "Ficha" panel (3.5) |
| 13 | Map page and editor, bestiary, encounters, gallery, document, rules, content, spells | Prepare and look up | Master (players read some) | Laptop | Out of this round (the map editor's calibration card far down is known, rehearsal F2) |

## 2. What causes the scroll (the patterns behind every screen)

| Pattern | Where | Cost |
| --- | --- | --- |
| Every panel always open, stacked | Session board (master: about ten panels in the right column at 1280), campaign home (17 panels) | The page is the sum of all panels |
| The session header carries the link field, "Copiar link da sessão" and "Encerrar sessão" | Every live screen | 190 px at 1280, 430 px on the master's phone, 240 px on a player's phone, before anything playable |
| Nothing the master acts on is sticky | Combat ("Próximo turno"), RP (the composer) | Scroll back up or far down for the most repeated actions |
| The Conversa sits under the board below 1600 px | Master and player, RP and combat | 2,500 to 5,200 px down at 1280 |
| Phone section tabs at the end of the page, in the flow | Every live phone screen | The badge "Conversa · 3" is invisible until the bottom |
| A spent action group keeps its full list | Player's combat (phone and 1280) | 250 to 400 px of dashed buttons nobody can press |
| Two blocks show the same thing | Master RP: "Imagens da cena" and "Imagem para os jogadores" | About 290 px |

---

## 3. Screens: today and the proposal

### 3.1 Master's combat (two alternatives)

**Today, top to bottom at 1280** (NPC turn): app bar (sticky) · session header · combat bar ("Encerrar combate", "Próximo turno") · two columns from 1280 (left: map card and "Registro do combate"; right, 500 px: the waiting cards, the NPC card, "Ordem de iniciativa", the deaths block) · "Efeitos" and the round clock · fog ("Ver como", "Luz dos personagens") · traps and treasure · puzzles · "Rolagens da mesa" · Conversa. Below 1280 it is one column (on a phone the card and the order come first). Always visible: only the app bar.

| Section | 1280 | 390 |
| --- | --- | --- |
| App bar | 64 | 56 |
| Session header | 190 | 430 |
| Combat bar (1024 px and up) | 124 | — |
| Map card + log (left) / card + order (right) | 1,750 to 2,150 (the right column wins: card ~650, order 100 + 8 rows of 95 to 190) | card 800 ("Próximo turno" at its foot), order 1,140, map 450, log 150, "Encerrar combate" 70 |
| Effects and clock | 300 to 500 | 400 to 600 |
| "Ver como" + "Luz dos personagens" | 740 | 900 |
| Traps + treasure | 500 to 700 | 700 to 900 |
| "Rolagens da mesa" | 200 | 250 |
| Conversa | 840 | (tab at the end) 80 |
| Footer | 120 | 120 |
| **Total** | **about 4,800 to 5,600 (6.5 to 7.5 screens)** | **about 5,500 to 6,200 (7 to 8 screens)** |

**Actions that need scrolling today at 1280** (page y of the control; the fold is at 800):

| # | Action | Where (y) | Scroll |
| --- | --- | --- | --- |
| 1 | Roll the NPC's attack ("Rolar no app") | ~1,000 | down 1 |
| 2 | Apply the damage it dealt ("Aplicar 5 de dano") | ~1,150 | down 1 |
| 3 | Back to "Próximo turno" | ~300 | up 1 to 2 |
| 4 | "Dano/Cura" on a row low in the order | 1,900 to 2,200 | down 2 to 3 |
| 5 | An effect, "Adicionar efeito", the round clock | ~2,350 | down 3 |
| 6 | "Ver como" a player, change a light | 2,800 to 3,300 | down 4 |
| 7 | Reveal, fire or disarm a trap; mark a treasure | 3,550 to 4,100 | down 5 |
| 8 | Narrate or speak as an NPC in the Conversa | 4,800 to 5,200 | down 6, then up 6 |
| 9 | Read the combat log | ~1,200 | down 1 |

Nine actions; 1 to 3 repeat on every NPC turn. On the phone, add "Próximo turno" itself (~1,290, down 1.5) and the "Conversa" tab (~5,700, down 7).

#### Alternative A, "Mapa e trilho" (recommended)

```text
+--------------------------------------------------------------------------------------------------+
| MeuRPG   Início   Minhas campanhas                          [Anotações]   Minha conta   [Sair]   |
+--------------------------------------------------------------------------------------------------+
| < Mirathel    Sessão 3   (o) Ao vivo desde 20:12                           [Dados]   [Sessão v]  |
+--------------------------------------------------------------------------------------------------+
| Rodada 3 | [G1] Vez do Goblin 1                          [Encerrar combate]  [>| Próximo turno]  |
|          | Em seguida: Pensantus   (!) Falta aplicar 5 de dano                                   |
+----------------------------------------------------------+---------------------------------------+
| Mapa · Cripta do Corvo · 24 x 16            [Ver mapa]   | [Turno · 2]   [Conversa · 3 novas]    |
| +----------------------------------------------------+   | ------------------------------------- |
| |                                                    |   | Dano de Goblin 1                      |
| |                                                    |   | Brisa · 5 de dano cortante            |
| |          the map, about 730 px wide                |   | [Aplicar 5 de dano]   [Não aplicar]   |
| |          (30 px squares on 24 columns)             |   | ------------------------------------- |
| |                                                    |   | Vez do Goblin 1     PV 7 de 7 · CA 15 |
| |                                                    |   | Ataque  (o) Cimitarra  ( ) Arco curto |
| +----------------------------------------------------+   | Alvo    (o) Brisa      ( ) Toren      |
| [ ] Mostrar o alcance    [ ] Mover à força               | [Rolar no app]   Digitar o resultado  |
| (P) Jogador  [C] NPC  [:] Escondido  (@) Vez             | ------------------------------------- |
|                                                          | Ordem de iniciativa     [+ Adicionar] |
| [Registro] [Efeitos · 2] [Névoa e luz] [Armadilhas · 2]  | [G1] Goblin 1   Vez  14    7/7    [:] |
| [Tesouro]                                                | (P) Pensantus        12  23/23        |
| +----------------------------------------------------+   |                       [Dano/Cura] [:] |
| | the chosen tab opens here, one at a time           |   | (P) Brisa            11  14/24  ...   |
| +----------------------------------------------------+   | ... (the rail scrolls inside itself)  |
+----------------------------------------------------------+---------------------------------------+
```

- The session bar (3.6) scrolls away; the **combat bar sticks** under the app bar (72 px: round, the turn, "Em seguida", the waiting tags, "Encerrar combate", "Próximo turno").
- Left, fluid (about 730 px): the map card as today, then **one tab set under the map**: "Registro", "Efeitos", "Névoa e luz", "Armadilhas", "Tesouro", "Quebra-cabeças" (a tab appears only when it applies: no fog, no "Névoa e luz"; no map, only "Registro" and "Efeitos"). One opens at a time, in place.
- Right, the **rail** (420 px, sticky under the bars, 664 px tall at 1280 x 800, its own scroll): "Turno" holds exactly today's side column (the waiting cards, the NPC card, "Ordem de iniciativa", the deaths block); "Conversa" holds the feed and the composer. Counts in the tab names ("Turno · 2", "Conversa · 3 novas").
- From 1600 px the rail splits in two, "Turno" and "Conversa" side by side (the same breakpoint the Conversa uses today).
- Theatre of the mind: the left column shows today's card, movement and "Cobertura dos alvos"; the rail keeps "Turno" and "Conversa".
- Height: the page is about 1,350 px (bars 192, map column 720, the open tab up to 450); the per-turn loop (roll, apply, pass) is in one screen with no page scroll.

#### Alternative B, "Três colunas"

```text
+--------------------------------------------------------------------------------------------------+
| MeuRPG   Início   Minhas campanhas                          [Anotações]   Minha conta   [Sair]   |
+--------------------------------------------------------------------------------------------------+
| < Mirathel    Sessão 3   (o) Ao vivo desde 20:12                           [Dados]   [Sessão v]  |
+--------------------------------------------------------------------------------------------------+
| Rodada 3 | [G1] Vez do Goblin 1                          [Encerrar combate]  [>| Próximo turno]  |
|          | Em seguida: Pensantus   (!) Falta aplicar 5 de dano                                   |
+-------------------------+------------------------------------------+-----------------------------+
| Ordem        [+ Adic.]  | Mapa · Cripta do Corvo      [Ver mapa]   | [Turno · 2] [Conversa · 3]  |
| [G1] Goblin 1   7/7  >  | +--------------------------------------+ | --------------------------- |
|      Vez · Escondido    | |                                      | | Dano de Goblin 1            |
| (P) Pensantus  23/23 >  | |   the map, about 510 px wide         | | Brisa · 5 de dano           |
| (P) Brisa      14/24 >  | |   (21 px squares on 24 columns)      | | [Aplicar 5 de dano]         |
|      Concentração       | |                                      | | [Não aplicar]               |
| [G2] Goblin 2   2/7  >  | +--------------------------------------+ | --------------------------- |
| [CU] Cultista   9/9  >  | (P) Jogador [C] NPC [:] Escondido        | Vez do Goblin 1             |
| (P) Toren      49/49 >  |                                          | PV 7 de 7 · CA 15           |
|                         | [Registro] [Efeitos · 2] [Névoa e luz]   | Ataque (o) Cimitarra        |
| a tap opens the row in  | [Armadilhas · 2] [Tesouro]               |        ( ) Arco curto       |
| place: Dano/Cura,       | +--------------------------------------+ | Alvo   (o) Brisa ( ) Toren  |
| Condições, Cobertura,   | | the chosen tab, in place             | | [Rolar no app]              |
| Revelar, Remover        | +--------------------------------------+ | Digitar o resultado         |
|                         |                                          |                             |
| 56 px rows, sticky      |                                          | sticky, own scroll          |
+-------------------------+------------------------------------------+-----------------------------+
```

- The same sticky combat bar. Three columns: **"Ordem" always visible** on the left (288 px, sticky, compact 56 px rows: token, name, HP number and bar, one state word; a tap opens the row in place with "Dano/Cura", "Condições", "Cobertura", "Revelar aos jogadores", "Remover"), the map in the middle (about 510 px), and "Turno · Conversa" on the right (368 px, sticky).
- The held questions that live inside an order row today move to the "Turno" column, with the other waiting answers; the cover lines (E9-07) live in the opened row.
- Height: about 1,250 px; nothing in the loop scrolls, and the order never leaves the screen.

#### A or B

| | A "Mapa e trilho" | B "Três colunas" |
| --- | --- | --- |
| Map at 1280 (24 columns) | ~730 px, 30 px squares | ~510 px, 21 px squares (rehearsal E9 already found 3-column maps hard to read) |
| Order while acting on the card | Below the card in the same tab (a short scroll inside the rail) | Always on screen |
| Order while reading the Conversa | Not visible (one click back) | Visible |
| Change to existing pieces | The containers only: `order-list`, `npc-card`, the queue cards and `app-conversation` move unchanged | `order-list` redesigned (compact row + opened row), held questions moved out of the rows |
| Below 1280 | Same as A from 1024 (rail 380 px) | Falls back to A below 1280 |

**Decided: B**, with one change: a button in the order's header folds the column to a 64 px strip of tokens (the turn ring, the dashed hidden one, an HP bar each), and the map takes the width back (from 19 to 29 px squares at 1280). The fold lasts until the page reloads; nothing is stored on the device. Boards: `Main.dc.html` (open) and `C1-02` (folded).

**Phone (390, secondary; the same for A and B):**

```text
+------------------------------------------+
| MeuRPG                  [Anotações]  [=] |
+------------------------------------------+
| < Sessão 3  (o) Ao vivo   [Dados] [...]  |
+------------------------------------------+
| Rodada 3 · [G1] Vez do Goblin 1          |
| [Turno · 2]  [Ordem]  [Mapa]             |
+------------------------------------------+
| Dano de Goblin 1 · Brisa · 5 cortante    |
| [Aplicar 5 de dano]    [Não aplicar]     |
| ---------------------------------------- |
| Vez do Goblin 1    PV 7 de 7 · CA 15     |
| Ataque (o) Cimitarra ( ) Arco curto      |
| Alvo   (o) Brisa     ( ) Toren           |
| [Rolar no app]   Digitar o resultado     |
|                                          |
| Mais: Efeitos · Névoa e luz ·            |
| Armadilhas · Tesouro · Registro    v     |
+------------------------------------------+
| (!) Falta aplicar 5 de dano              |
|                     [>| Próximo turno]   |
+------------------------------------------+
| [Combate]    [Grupo]   [Conversa · 3]    |
+------------------------------------------+
```

- A tab list "Turno · Ordem · Mapa" under the bar (one shown at a time); the other panels fold into "Mais" at the end of "Turno".
- A **sticky bottom bar** with "Próximo turno" (and the waiting tag, "Falta aplicar 5 de dano"), like the player's turn bar; the npc-card's own "Próximo turno" goes away so there is one per screen.
- The pinned nav (3.6): "Combate · Grupo · Conversa" ("Grupo" is the party panel with rests and effects; today the master's "Ficha" tab leaves for the campaign page).

**What changes for existing behaviour:**

| Area | Change |
| --- | --- |
| Keyboard order | Bar, then map column (map, tabs under it), then rail (tab list, panel). The rail's tab list is one tab stop with arrow keys (ARIA tabs pattern). `scroll-padding-top` equal to the sticky bars, so focus is never hidden under them (WCAG 2.2, 2.4.11). |
| Accessibility | The rail is an `aside` named "Turno e conversa". Inactive panels stay mounted with `hidden` (the composer's draft and the feed's place survive a tab change); a hidden panel leaves the accessibility tree. Badges are words in the tab's name ("Conversa, 3 mensagens novas"). The sticky bar uses a 1 px `line` border, no shadow (design rule). |
| One filled button | "Próximo turno" stays the filled one; with the Conversa beside it, "Enviar" is drawn outlined during a combat (it is filled today, and both already show together at 1600 px). Ctrl+Enter still sends. |
| e2e | `combat.spec`, `joint-turns.spec`, `move.spec`, `reactions.spec` find "Próximo turno" by name: still one per width. Specs that reach effects, fog, traps or treasure during combat (`effects-master`, `fog`, `traps`, `treasure`, `theatre`) first open the tab under the map: a helper `openCombatTab(page, 'Efeitos')`. `conversa.spec` (1280 px) finds the composer in the rail. `a11y.spec` visits each tab. |

### 3.2 Master's campaign home (two alternatives)

**Today, at 1280:** title and lead · level-up notice · left column (3fr): "Sessão", "Sessões anteriores", "Experiência", "Dados", "Mapas", "Galeria", "Bestiário", "Encontros", "Quebra-cabeças", then "Personagens" (waiting for approval, the players' characters, the NPCs) · right column (2fr): "Regras da mesa", "Conteúdo da mesa", "Magias", "Documento da campanha", "Pacote da campanha", "Membros", "Convites". At 390 the right column follows the left. Always visible: nothing beyond the app bar.

| Section (1280) | Height | Page y |
| --- | --- | --- |
| Header (back link, title, lead) | 240 | 64 |
| Left: Sessão 240 · Sessões anteriores 320 · Experiência 580 · Dados 530 · Mapas 530 · Galeria 330 · Bestiário 160 · Encontros 200 · Quebra-cabeças 280 · Personagens (4 players + 9 NPCs) 1,030 | **4,350** with gaps | 320 to 4,700 |
| Right: 5 link panels of 160 · Membros 360 · Convites 680 | 1,980 | 320 to 2,300 |
| **Total** | **about 4,850 (6.5 screens); about 7,500 at 390** | |

**Actions that need scrolling today at 1280:**

| # | Action | Where (y) | Scroll |
| --- | --- | --- | --- |
| 1 | "Dar XP" | ~1,000 | down 0.5 |
| 2 | Change how the table rolls ("Dados") | ~1,550 | down 1.5 |
| 3 | "Novo mapa", open a map | 2,100 to 2,600 | down 2 to 3 |
| 4 | "Gerar convite" | ~2,250 | down 2 |
| 5 | Open the gallery | ~2,650 | down 3 |
| 6 | Open the bestiary | ~2,980 | down 3.5 |
| 7 | "Montar encontro" | ~3,150 | down 4 |
| 8 | Puzzles | ~3,380 | down 4 |
| 9 | Approve a character ("Esperando aprovação") | ~3,700 | down 4.5 |
| 10 | Open a player's sheet | 3,800 to 4,100 | down 4.5 to 5 |
| 11 | "Novo NPC", open an NPC | 4,150 to 4,700 | down 5 to 6 |
| 12 | Approve a pending member ("Membros") | ~1,500 | down 1 |

Twelve actions; the ones used while preparing a session (maps, NPCs, encounters) are the deepest.

#### Alternative A, "Abas" (recommended)

The session strip stays above the tabs on every tab ("Iniciar sessão" or "Entrar na sessão" and "Copiar link da sessão", plus the pending-choices list of MR-011 when it asks). Then three tabs, each a route:

| Tab | Route | Holds |
| --- | --- | --- |
| "Mesa" (default) | `/campaigns/:id` | Personagens dos jogadores (with "Esperando aprovação" first), Experiência ("Dar XP", marcos, histórico), Sessões anteriores, Membros, Convites |
| "Preparação" | `/campaigns/:id/prep` | Mapas (with "Gerar masmorra" and "Gerar tesouro"), NPCs, Encontros, Quebra-cabeças, Bestiário, Galeria, Documento da campanha |
| "Regras" | `/campaigns/:id/setup` (`/rules` is already the "Regras da mesa" page) | Dados, Regras da mesa, Conteúdo da mesa, Magias, Pacote da campanha |

```text
+--------------------------------------------------------------------------------------------------+
| MeuRPG   Início   Minhas campanhas                          [Anotações]   Minha conta   [Sair]   |
+--------------------------------------------------------------------------------------------------+
| < Minhas campanhas                                                                               |
| Mirathel                               Você é mestre nesta campanha. XP por inimigos derrotados. |
+--------------------------------------------------------------------------------------------------+
| (o) Sessão 4 ao vivo desde 20:12                   [Copiar link da sessão]   [Entrar na sessão]  |
+--------------------------------------------------------------------------------------------------+
| [ Mesa · 1 ]    [ Preparação ]    [ Regras ]                                                     |
+--------------------------------------------------------+-----------------------------------------+
| Personagens dos jogadores                              | Membros (4)                             |
| (!) Esperando aprovação: Brisa · Barda 1   [Revisar]   | Mestre Teste (você) ..... mestre        |
| Pensantus · Mago 3 · de Ana             Travada  >     | Ana ..................... jogador       |
| Toren · Guerreiro 3 · de Rafa           Travada  >     | Rafa .................... jogador       |
| -------------------------------------------------      | Esperando aprovação: 1    [Revisar]     |
| Experiência                              [Dar XP]      | -----------------------------------     |
| Pensantus 2.716 XP · nível 3 · faltam 1.284            | Convites                                |
| Toren     2.716 XP · nível 3                           | 1 de 1 uso · criado 30/09    Usado      |
| Histórico (5)                                  >       | Novo convite  Usos [1]  Validade [7d]   |
| -------------------------------------------------      | [ ] Exigir aprovação do mestre          |
| Sessões anteriores                                     |                     [Gerar convite]     |
| Sessão 3 · 18/10 · 3 h 10 min          [Resumo]        |                                         |
| Sessão 2 · 11/10                       [Resumo]        |                                         |
+--------------------------------------------------------+-----------------------------------------+
```

```text
+--------------------------------------------------------------------------------------------------+
| [ Mesa · 1 ]    [ Preparação ]    [ Regras ]          (tab "Preparação", same header above)      |
+--------------------------------------------------------+-----------------------------------------+
| Mapas (4)    [Novo mapa] [Gerar masmorra] [Tesouro]    | NPCs (9)                [Novo NPC v]    |
| Cripta do Corvo · 24 x 16 · mapa atual          >      | (B) Bartolo · taverneiro · retrato >    |
| Estrada do Vale · 20 x 14                       >      | (L) Lia · guarda                   >    |
| ... 2 mais                                             | ... 7 mais                              |
| -------------------------------------------------      | -----------------------------------     |
| Encontros (3)                  [Montar encontro]       | Galeria · 12 de 200 imagens             |
| Quebra-cabeças (2)              [Novo]                 | [t][t][t][t]       [Abrir a galeria]    |
| Bestiário  334 criaturas do SRD [Abrir bestiário]      | Documento da campanha     [Abrir]       |
+--------------------------------------------------------+-----------------------------------------+
```

```text
+--------------------------------------------------------------------------------------------------+
| [ Mesa · 1 ]    [ Preparação ]    [ Regras ]             (tab "Regras", same header above)       |
+--------------------------------------------------------+-----------------------------------------+
| Dados                                                  | Regras da mesa      [Abrir as regras]   |
| Cada jogador escolhe                                   | Conteúdo da mesa  [Abrir o conteúdo]    |
| Brisa · No app     Toren · Meus próprios dados         | Magias              [Abrir as magias]   |
|                   [Mudar em Regras da mesa]            | Pacote da campanha [Exportar campanha]  |
+--------------------------------------------------------+-----------------------------------------+
```

At 390:

```text
+------------------------------------------+
| MeuRPG                  [Anotações]  [=] |
+------------------------------------------+
| < Minhas campanhas                       |
| Mirathel                                 |
| Você é mestre. XP por inimigos.          |
+------------------------------------------+
| (o) Sessão 4 ao vivo desde 20:12         |
| [        Entrar na sessão        ]       |
| [     Copiar link da sessão      ]       |
+------------------------------------------+
| [Mesa · 1] [Preparação] [Regras]         |
+------------------------------------------+
| Personagens dos jogadores                |
| (!) Brisa espera aprovação [Revisar]     |
| Pensantus · Mago 3 · Travada      >      |
| Toren · Guerreiro 3 · Travada     >      |
| Experiência                [Dar XP]      |
| ...                                      |
| Membros (4) · Convites    [Gerar]        |
+------------------------------------------+
```

- Heights at 1280: "Mesa" about 1,400, "Preparação" about 1,500, "Regras" about 1,000. Every action of the table above is at most one click and one screen away.
- A count on a tab says what waits there ("Mesa · 1": one character waits for approval).
- `app-campaign-characters` splits: the players' part stays on "Mesa", the NPC part ("NPCs", "Novo NPC") goes to "Preparação".
- The player's campaign home keeps one page (no tabs): it has few panels (3.7).

#### Alternative B, "Índice e painéis recolhidos"

```text
+--------------------------------------------------------------------------------------------------+
| MeuRPG   Início   Minhas campanhas                          [Anotações]   Minha conta   [Sair]   |
+--------------------------------------------------------------------------------------------------+
| Mirathel                               Você é mestre nesta campanha. XP por inimigos derrotados. |
+----------------------+---------------------------------------------------------------------------+
| Nesta página         | Sessão 4 ao vivo desde 20:12   [Copiar link da sessão]  [Entrar na sessão]|
| Sessão  (o)          | --------------------------------------------------------------------------|
| Personagens · 1 (!)  | v Personagens dos jogadores · 3 · 1 esperando aprovação                   |
| Experiência          |   (!) Brisa · Barda 1 espera aprovação                           [Revisar]|
| Sessões anteriores   |   Pensantus · Mago 3 · Travada      Toren · Guerreiro 3 · Travada         |
| Mapas · 4            | > Experiência · grupo no nível 3                                  [Dar XP]|
| NPCs · 9             | > Sessões anteriores · 3                                                  |
| Encontros · 3        | > Mapas · 4 · atual: Cripta do Corvo                           [Novo mapa]|
| Quebra-cabeças · 2   | > NPCs · 9                                                      [Novo NPC]|
| Galeria · 12         | > Encontros · 3                                          [Montar encontro]|
| Bestiário            | > Quebra-cabeças · 2                                                      |
| Documento            | > Galeria · 12 de 200                                                     |
| Dados                | > Dados · Cada jogador escolhe                                            |
| Regras e conteúdo    | > Membros · 4      > Convites · 1 ativo                    [Gerar convite]|
| Membros · Convites   | (each header row is 64 px; a row opens its panel in place)                |
| (sticky; jumps)      |                                                                           |
+----------------------+---------------------------------------------------------------------------+
```

- One page, as today, with a **sticky index** on the left from 1024 px ("Nesta página", each entry with its count; a click scrolls to the panel and focuses its heading) and **every panel folded to a 64 px header row** that carries its count and its main button ("Mapas · 4 · [Novo mapa]"). "Sessão" and "Personagens" open by default.
- On a phone the index is a sticky row of chips under the title.
- Height folded: about 1,400 px; each opened panel adds its own height.

#### A or B

| | A "Abas" | B "Índice e painéis" |
| --- | --- | --- |
| Page at 1280 | 1,000 to 1,500 per tab | ~1,400 folded, grows as panels open |
| What the master remembers | Three names | Nothing (everything is listed) |
| Ctrl+F, reading the whole page | Per tab | The whole page |
| Remembering what was open | The URL (a link to "Preparação" works, a reload keeps it) | Nothing: panels reset to folded on each visit (the app keeps no Web Storage; only the session cookie) |
| Change | A route per tab, the characters panel split | A fold on every panel, an index component |

**Decided: A**, with an icon on each tab ("Mesa", "Preparação", "Regras") and on the panel titles. The jobs really differ (run the table, prepare, configure), a URL per tab lets other screens link straight to "Preparação" (the bestiary's "Criar NPC" can come back there), and it does not depend on remembering folds the app may not store. Board: `C2-01`.

**What changes for existing behaviour (A):** the tabs are links in a `nav` named "Seções da campanha" with `aria-current="page"` (route tabs are navigation, not ARIA tabs); a tab change keeps focus on the tab link instead of moving it to the `h1`. e2e: `campaigns`, `characters`, `character-approval`, `invite`, `claim-links`, `xp`, `milestones`, `gold`, `support.ts` ("Novo NPC") go to the tab's URL or click it first; `a11y.spec` visits the three tabs.

### 3.3 Master's live session outside combat (RP)

**Today at 1280:** session header · "Perguntas pendentes" (when any) · "Combate" strip with "Iniciar combate" · the encounters on battle points · board: left column (the open scene: stage, scene images, clues and hooks, rolls, actions; the group check; the open puzzle; the map with "Pontos do mapa") and a 400 px right column ("Ver como", "Luz dos personagens", "Imagem para os jogadores", "Cena de RP" when none is open, "Grupo", "Descanso", "Efeitos em jogo" with "Passar o tempo", "Magias", "Tokens no mapa") · traps and treasure across the width · "Rolagens da mesa" · Conversa.

| Section | 1280 | Page y at 1280 |
| --- | --- | --- |
| App bar, session header, "Combate" strip | 64 + 190 + 104 | 0 to 360 |
| Left: open scene ~1,250, group check ~100, map with points 1,000 to 1,300 | 2,400 to 2,650 | 375 to 3,000 |
| Right: Ver como 440, Luz 300, Imagem 290, Grupo 790, Descanso 140, Efeitos 330, Magias 180, Tokens 420 (+ gaps) | **~3,100** | 375 to 3,450 |
| Traps + treasure | 500 to 700 | 3,465 to 4,100 |
| "Rolagens da mesa" | 200 | ~4,130 |
| Conversa (composer at the bottom) | 840 | 4,355 to 5,195 |
| **Total** | **about 5,100 to 5,700 (7 to 8 screens); about 7,500 at 390** | |

**Actions that need scrolling today at 1280:**

| # | Action | Where (y) | Scroll |
| --- | --- | --- | --- |
| 1 | Narrate, speak as Bartolo, whisper to Brisa | composer ~4,950 | down 6 (and up 6 to the stage) |
| 2 | Read what the players wrote | feed ~4,400 | down 6 |
| 3 | Show a scene image ("Mostrar aos jogadores") or "Imagem para os jogadores" | 1,100 to 1,300 | down 1 |
| 4 | Read the new scene rolls | ~1,300 | down 1 |
| 5 | "Revelar" a clue | ~1,400 | down 1 |
| 6 | "Ajustar" the HP of the third or fourth character | 2,000 to 2,200 | down 2 |
| 7 | "Descanso curto" or "Descanso longo" | ~2,350 | down 2 to 3 |
| 8 | "Passar o tempo" | ~2,700 | down 3 |
| 9 | "Concluir" a long cast in "Magias" | ~2,900 | down 3 |
| 10 | Reveal a point, hide a token | 2,400 to 3,400 | down 3 to 4 |
| 11 | Reveal or fire a trap, place a treasure | ~3,600 | down 4 |

Eleven actions; 1 and 2 are the most frequent of the whole evening.

**Proposal (one):** main tabs "Cena · Mapa · Grupo" and the Conversa in the rail. This is the approved Conversa board at 1280 (`conversa-web/board-MasterDesktop-dark-1280.png`: stage left, Conversa right), which the build moved to 1,600 px to keep the 1280 screens as they were; the tabs free the width it needs.

```text
+--------------------------------------------------------------------------------------------------+
| MeuRPG   Início   Minhas campanhas                          [Anotações]   Minha conta   [Sair]   |
+--------------------------------------------------------------------------------------------------+
| < Mirathel  Sessão 3 (o) Ao vivo desde 20:12      [Iniciar combate]  [Dados]   [Sessão v]        |
+----------------------------------------------------------+---------------------------------------+
| [Cena · 2 rolagens novas]  [Mapa]  [Grupo · 1]           | Conversa                 4 na mesa    |
| -------------------------------------------------------  | ------------------------------------- |
| (cast) Aberta para os jogadores desde 20:20              | (B) Bartolo · você                    |
| Cena: A taverna do Javali Cego                           |     Quartos, só amanhã.               |
|                         [Trocar cena]   [Fechar cena]    | Narração · só para Pensantus e Brisa  |
| Em cena  [Bartolo · Fala agora] [Lia]  [+ Pôr em cena]   |     Vocês sentem um arrepio...        |
|          [Dar a fala] [Tirar de cena]                    | Pensantus                             |
| Imagens  [Vista · À mostra] [Porão] [Mapa]   [+ Galeria] |     Gente de quem, exatamente?        |
|          [ ] Deixar com os jogadores                     | ...                                   |
| Rolagens (nova em cima)  | Ações       Só você vê a CD   | ------------------------------------- |
| Brisa · Intuição 14      | Olhar o salão     CD 14       | Falar como [Bartolo v] Para [Todos]   |
| Toren · Atletismo 9      | Acalmar os cavalos            | +-----------------------------------+ |
| Pistas (1)  [Revelar]    | Ganchos e anotações    >      | | Mensagem                          | |
| [Pedir teste em grupo]                                   | +-----------------------------------+ |
|                                                          | [Imagem da galeria]        [Enviar]   |
| Rolagens da mesa (fim da coluna)                         | sticky, always on screen              |
+----------------------------------------------------------+---------------------------------------+
```

```text
+--------------------------------------------------------------------------------------------------+
| [Cena]  [Mapa]  [Grupo · 1]                         (tab "Mapa": the map gets the main column)   |
+----------------------------------------------------------+---------------------------------------+
| Mapa atual [Cripta do Corvo v]          [Abrir mapa]     | Conversa (same rail, same place)      |
| +----------------------------------------------------+   |                                       |
| |   the map, about 740 px wide, tokens draggable     |   |                                       |
| +----------------------------------------------------+   |                                       |
| [Pontos · 3] [Tokens · 4] [Névoa e luz] [Armadilhas · 2] |                                       |
| [Tesouro · 1] [Encontros · 1]                            |                                       |
| +----------------------------------------------------+   |                                       |
| | Ver como (o) Todos ( ) Pensantus ( ) Brisa ...     |   |                                       |
| | Luz dos personagens: Pensantus [Tocha v] ...       |   |                                       |
| +----------------------------------------------------+   |                                       |
+----------------------------------------------------------+---------------------------------------+
```

```text
+--------------------------------------------------------------------------------------------------+
| [Cena]  [Mapa]  [Grupo · 1]                    (tab "Grupo": the party and the table clock)      |
+----------------------------------------------------------+---------------------------------------+
| Grupo                                       [Dar XP]     | Conversa (same rail, same place)      |
| Pensantus · Mago 3  23/23 =====  1º oooo 2º oo [Ajustar] |                                       |
| Brisa · Barda 3      14/24 ===     1º oo.. [Ajustar]     |                                       |
| Toren · Guerreiro 3  49/49 ======          [Ajustar]     |                                       |
| ------------------------------------------------------   |                                       |
| Descanso   [Descanso curto]  [Descanso longo]            |                                       |
| Efeitos em jogo (2)  [Adicionar efeito] [Exaustão]       |                                       |
| Passar o tempo  [1 min] [10 min] [1 h] [8 h]             |                                       |
| Magias · 1 esperando você          [Concluir]            |                                       |
+----------------------------------------------------------+---------------------------------------+
```

- The session bar carries "Iniciar combate" (outlined, as today); the "Combate" strip's sentence moves into the dialog. The encounters on battle points ("Começar este combate") list in the "Mapa" tab and in the "Iniciar combate" dialog.
- "Perguntas pendentes" stays above the tabs while any question waits (it blocks a player).
- "Cena": the open scene, with "Imagem para os jogadores" folded into the scene's image row (the shown image is "À mostra" there, "Deixar com os jogadores" beside it); the separate panel exists only when no scene is open, next to "Abrir cena". The group check is a button in the scene ("Pedir teste em grupo"). "Rolagens da mesa" closes the column.
- "Mapa": the map takes the whole main column, with one tab set under it: "Pontos", "Tokens", "Névoa e luz" ("Ver como" and "Luz dos personagens"), "Armadilhas", "Tesouro", "Encontros".
- "Grupo": the party ("Ajustar", "Dar XP"), "Descanso", "Efeitos em jogo" with "Passar o tempo", "Magias" (a long cast waiting is the tab's count), the Bardic Inspiration block.
- The default tab is "Cena" while a scene is open, otherwise "Mapa"; opening a scene from "Abrir cena" switches to "Cena" (the master's own action). The tab is a query parameter (`?vista=mapa`), so a reload keeps it and nothing is stored.
- Heights at 1280: "Cena" about 1,300 to 1,450, "Mapa" about 1,500, "Grupo" about 1,300; the Conversa never leaves the screen. Every action of the table above is on screen or one click away.
- Trade-offs: the scene and the map are no longer on screen together at 1280 (today the map starts about 1,400 px below the scene, so they are not together either); the party's HP is one click away (the "Grupo" count says when something waits there); from 1600 px the main column is wide enough to show "Mapa" beside "Cena", a later step.

**Phone (390, secondary):**

```text
+------------------------------------------+
| MeuRPG                  [Anotações]  [=] |
+------------------------------------------+
| < Sessão 3  (o) Ao vivo   [Dados] [...]  |
+------------------------------------------+
| Cena: A taverna do Javali Cego           |
| [Trocar cena]        [Fechar cena]       |
| Em cena: (B) Bartolo fala · (L) Lia  [+] |
| Imagens: [Vista·À mostra][Porão]  >      |
| Rolagens · 2 novas                   v   |
| Ações da cena (3)                    >   |
| Pistas (1)                           >   |
| Mapa · Cripta do Corvo               >   |
| [Pedir teste em grupo]                   |
+------------------------------------------+
| [Cena]      [Grupo · 1]  [Conversa · 3]  |
+------------------------------------------+
```

- The open scene with its parts folded ("Rolagens · 2 novas", "Ações da cena", "Pistas", "Mapa"); the pinned nav "Cena · Grupo · Conversa". Height of "Cena" about 1,800 folded.

**What changes:** keyboard order bar, tab list, panel, rail; the rail is an `aside` named "Conversa"; "Enviar" stays the one filled button (no combat). e2e: `scenes`, `stage`, `scene-images`, `shown-image`, `notes` stay on "Cena"; `fog` ("Luz dos personagens", "Ver como"), `doors`, `traps`, `treasure`, `maps` open "Mapa"; `effects-master` ("Passar o tempo", 14 hits), `rests`, `casting`, `xp` open "Grupo"; `conversa.spec` opens its table at 1280 px (`conversa-support.ts`) and its header comment already assumes the panel beside the stage, which this makes true.

### 3.4 Player's combat

**Today, phone, own turn:** session header (240) · the turn card ("Sua vez, Pensantus", one row of four chips, fixed by E2) · "Avisar na minha vez" · the notices · the action groups (Ação with attacks, spells and the eight "Ações padrão"; Ação bônus; Reação; Movimento with "Mover") · the order strip · the map · the log · the vitals (compact) · the light · "Rolagens da mesa" · the section tabs; the turn bar sticks to the bottom ("Mover 9 m" as text, "Encerrar turno").
**Today, 1280, own turn:** session header · the banner ("Encerrar turno") · three columns: order (232) | map and log, vitals under them | actions (400) · "Rolagens da mesa" · Conversa.

| | 390 | 1280 |
| --- | --- | --- |
| Header + turn card | 296 + 250 | 235 + 145 |
| Action groups (Pensantus, wizard, 6 spells) | 1,300 to 1,700 | 1,300 to 1,700 (right column) |
| Order strip, map, log | 650 | in the middle column |
| Vitals, light, free rolls, tabs | 850 | Conversa 840 after the board |
| **Total** | **about 3,300 to 3,800 (4 to 5 screens)** | **about 3,000 to 3,500** |

**Actions that need scrolling today:**

| # | Action | 390 (y) | 1280 (y) |
| --- | --- | --- | --- |
| 1 | A spell low in the "Ação" list | ~1,200, down 1 | ~1,000, down 0.5 |
| 2 | A bonus action | ~1,650, down 1 to 2 | ~1,450, down 1 |
| 3 | "Mover" | ~2,100, down 2 to 3 | ~1,850, down 1.5 (dragging on the map works) |
| 4 | Look at the map | ~2,300, down 2 | on screen |
| 5 | My HP, slots, concentration | ~2,950, down 3 | ~1,100, down 0.5 |
| 6 | The Conversa | tab ~3,650, down 4 | ~2,400 to 2,950, down 2 to 3 |

**Proposal (one):**

```text
+------------------------------------------+
| MeuRPG                  [Anotações]  [=] |
+------------------------------------------+
| < Sessão 3  (o) Ao vivo          [Dados] |
+------------------------------------------+
| (P) Sua vez, Pensantus                   |
| Rodada 3 · Depois de você: Toren         |
| [Ação o] [Bônus x] [Reação o] [9 m]      |
| PV 23/23 · CA 13 · 1º 3/4 · 2º 2/2       |
| Concentrando: Raio Lunar        [Parar]  |
+------------------------------------------+
| Ação                        Disponível   |
| Raio de Fogo  +5 · 1d10 fogo [Conjurar]  |
| Mísseis Mágicos · 1º         [Conjurar]  |
| Adaga  +4 · 1d4 + 2          [Atacar]    |
| Outras ações (8)                     v   |
| ---------------------------------------- |
| Ação bônus · Usada                   >   |
| Reação · Disponível                  >   |
| ---------------------------------------- |
| Ordem: (P)Pens [G1] (B)Brisa [CU]  ->    |
| [ map card ............ Ver mapa ]       |
| Registro do combate                  >   |
+------------------------------------------+
| [Mover 9 m]            [Encerrar turno]  |
+------------------------------------------+
| [Cena]       [Ficha]     [Conversa · 3]  |
+------------------------------------------+
```

```text
+--------------------------------------------------------------------------------------------------+
| MeuRPG   Início   Minhas campanhas                          [Anotações]   Minha conta   [Sair]   |
+--------------------------------------------------------------------------------------------------+
| < Mirathel    Sessão 3   (o) Ao vivo desde 20:12                                       [Dados]   |
+--------------------------------------------------------------------------------------------------+
| (P) Sua vez, Pensantus · Rodada 3 · Depois: Toren   PV 23/23 · CA 13   [Mover]  [Encerrar turno] |
+----------------------+-------------------------------------------+-------------------------------+
| Ordem                | Mapa · Cripta do Corvo       [Ver mapa]   | [Ações]  [Conversa · 3]       |
| (P) Pensantus   <    | +---------------------------------------+ | ----------------------------- |
| [G1] Goblin 1        | |                                       | | Ação               Disponível |
| (B) Brisa            | |   the map, about 560 px wide          | | Raio de Fogo      [Conjurar]  |
| [CU] Cultista        | |   (drag your own token on your turn)  | | Mísseis Mágicos   [Conjurar]  |
| (T) Toren            | |                                       | | Adaga             [Atacar]    |
|                      | +---------------------------------------+ | Outras ações (8)          v   |
| sticky               | Registro do combate                   >   | Ação bônus · Usada        >   |
|                      | Rolagens da mesa                      >   | Reação · Disponível       >   |
|                      |                                           | Movimento · restam 9 m        |
|                      |                                           | sticky, own scroll            |
+----------------------+-------------------------------------------+-------------------------------+
```

- **"Mover" joins "Encerrar turno"** in the sticky bar on a phone and in the sticky banner at 1280 (it opens the same "Mover" page). The "Movimento" group becomes a read-only line ("Movimento · restam 9 m"), so there is still exactly one "Mover" button per screen (the e2e uses `getByRole('button', { name: 'Mover' })` 20 times; a second button would break strict mode).
- **A spent group folds to one line** ("Ação bônus · Usada", tap to see what it had); the eight "Ações padrão" fold behind "Outras ações (8)" once the Action is spent, and stay open while it is available (question 9).
- **My numbers at the top:** one line under the turn title ("PV 23/23 · CA 13 · 1º 3/4 · 2º 2/2") and the concentration line; the full vitals card is in the "Ficha" panel (3.5).
- At 1280 the actions column becomes a **rail** with "Ações · Conversa", and the order column sticks too.
- Off turn: the turn card shows whose turn it is and my line of numbers; the rail's "Ações" says what I still have ("Reação · Disponível"); reactions keep opening their own dialog.
- Heights: 390 about 1,700 to 2,100 (2.5 screens) with "Mover" and "Encerrar turno" always on screen; 1280 about 1,100 plus the rail.
- Trade-offs: the eight "Ações padrão" all spend the Action, so folding them once it is spent hides nothing usable (a rogue's Cunning Action lives in the bonus group); two bars at the bottom of a phone (turn bar 64 px and nav 56 px) take 120 px, about 21 % of a 320 x 568 screen, which the board must check at that size; at 1280 the compact vitals card stays under the map in the middle column (as today), and its main numbers also show in the sticky banner.

**What changes:** keyboard order turn card, actions, map, bar (the bar stays last in the DOM so it sticks, as today); the folded groups are disclosure buttons with `aria-expanded`. e2e: `combat`, `move`, `doors`, `fog`, `theatre`, `traps`, `creatures-combat` (the "Disparada" button sits behind "Outras ações" after the Action is used: open it first), `effects-player`, `contests`, `reactions`; `a11y.spec` adds the folded and opened states.

### 3.5 Player's live session outside combat (RP)

**Today, phone:** session header · notices · the group check · the scene (stage, "O que você pode fazer", the note) · "O mestre está mostrando" · "Imagens que o mestre deixou" · the map · found treasures, trap activity · vitals · "Revivificar" · "Magias" · the Bardic block · "Rolagens da mesa" · the section tabs. **1280:** the same in two columns (scene, image and map | vitals and "Magias"), then "Rolagens da mesa" and the Conversa.

| | 390 | 1280 |
| --- | --- | --- |
| Header | 296 | 235 |
| Scene with a stage and 5 actions | ~1,000 | ~850 |
| Shown image, left images | ~630 | ~500 |
| Map | ~450 | ~600 |
| Vitals, "Magias" | ~700 | in the side column |
| Free rolls, tabs or Conversa | ~290 | 1,040 |
| **Total** | **about 3,300 (4 screens)** | **about 3,300** |

**Actions that need scrolling today:**

| # | Action | 390 (y) | 1280 (y) |
| --- | --- | --- | --- |
| 1 | "Rolar" on the 4th or 5th scene action | ~1,200, down 1 | on screen |
| 2 | See what the master is showing | ~1,320, down 1 to 2 | ~1,130, down 0.5 |
| 3 | Look at the map | ~2,000, down 2 | ~1,650, down 1 |
| 4 | My HP and slots | ~2,500, down 3 | on screen |
| 5 | "Conjurar" outside combat | ~2,950, down 3 to 4 | ~720, on screen |
| 6 | The Conversa | tab ~3,420, down 4 | ~2,500 to 3,100, down 2 to 3 |

**Proposal (one):**

```text
+------------------------------------------+
| MeuRPG                  [Anotações]  [=] |
+------------------------------------------+
| < Sessão 3  (o) Ao vivo          [Dados] |
+------------------------------------------+
| O mestre está mostrando                  |
| +--------------------------------------+ |
| |  image, at most 40% of the screen    | |
| +--------------------------------------+ |
| Vista da carroça     [Ver em tela cheia] |
+------------------------------------------+
| Cena: A carroça tombada                  |
| (B) Bartolo · Fala agora  (L) Lia        |
| O que você pode fazer                    |
| Procurar pistas · Investigação +6 [Rolar]|
| Seguir os rastros · Sobrev. +1    [Rolar]|
| Acalmar os cavalos · Ades. +1     [Rolar]|
| O mestre vê cada resultado.              |
+------------------------------------------+
| Mapa · Estrada do Vale       [Ver mapa]  |
| Imagens que o mestre deixou (2)      >   |
+------------------------------------------+
| [Cena]       [Ficha]     [Conversa · 3]  |
+------------------------------------------+
```

```text
+------------------------------------------+
| MeuRPG                  [Anotações]  [=] |
+------------------------------------------+
| Pensantus · Mago 3 · Gnomo das Rochas    |
| 23 de 23 PV  =========================== |
| CA 13 · PV temporários 0 · Dados 0 de 3  |
| Espaços  1º oooo 4 livres · 2º oo        |
| Magias  [Conjurar fora do combate]       |
| Conjurando: nada agora                   |
| [Revivificar]  (when the sheet has it)   |
| Abrir a ficha completa ->                |
+------------------------------------------+
| [Cena]       [Ficha]     [Conversa · 3]  |
+------------------------------------------+
```

```text
+--------------------------------------------------------------------------------------------------+
| MeuRPG   Início   Minhas campanhas                          [Anotações]   Minha conta   [Sair]   |
+--------------------------------------------------------------------------------------------------+
| < Mirathel    Sessão 3   (o) Ao vivo desde 20:12                                       [Dados]   |
+------------------------------------------------------------+-------------------------------------+
| O mestre está mostrando    | Cena: A carroça tombada       | [Conversa · 3]   [Ficha]            |
| +------------------------+ | (B) Bartolo · Fala agora      | ----------------------------------- |
| |  image 360 px          | | O que você pode fazer         | (B) Bartolo                         |
| |                        | | Procurar pistas  +6 [Rolar]   |     Quartos, só amanhã.             |
| +------------------------+ | Seguir rastros   +1 [Rolar]   | Brisa                               |
| [Ver em tela cheia]        | Acalmar cavalos  +1 [Rolar]   |     E o porão?                      |
| ---------------------------------------------------------  | ...                                 |
| Mapa · Estrada do Vale                      [Ver mapa]     | ----------------------------------- |
| +-------------------------------------------------------+  | (Como Pensantus) Fora do personagem |
| |   the map, about 760 px wide                          |  | [Mensagem                         ] |
| +-------------------------------------------------------+  | [Só o mestre lê]          [Enviar]  |
| Imagens que o mestre deixou (2)   >   Rolagens da mesa >   | sticky; "Ficha" = PV, CA, magias    |
+------------------------------------------------------------+-------------------------------------+
```

- Phone: the **pinned nav** "Cena · Ficha · Conversa"; "Ficha" opens a **panel inside the session** (vitals, slots, "Magias" with casting outside combat, "Revivificar", "Abrir a ficha completa") instead of leaving for the sheet page, so the stream and the badges stay.
- "O mestre está mostrando" comes **first** while an image is shown (at most 40 % of the screen height; "Ver em tela cheia" as today), then the scene with the stage capped (rehearsal G2: the stage took a whole screen), then the map; "Imagens que o mestre deixou" folds to one row.
- 1280: two columns, image and scene side by side above the map; the **rail** "Conversa · Ficha" sticks on the right.
- Heights: 390 about 1,900 on "Cena"; 1280 about 1,600 with the Conversa always on screen.
- Trade-offs: while an image is shown the scene's actions start about 360 px lower (the image is what the master wants seen; it never takes focus, as today); the "Ficha" panel repeats part of the sheet page, so it shows only the numbers used in play and links to the rest.

**What changes:** e2e `shown-image`, `scenes`, `stage`, `notes`, `casting` (the player's "Conjurar" moves into "Ficha" on a phone), `revivify`, `free-dice` ("Rolagens da mesa" at the end of "Cena"), `conversa`.

### 3.6 Shared pieces (only where the pattern repeats)

| Piece | Where it is used | What it is |
| --- | --- | --- |
| **Session bar** (rework of `app-session-header`) | Every live screen, master and player | One row: back link to the campaign, "Sessão 3", "Ao vivo desde 20:12", "Dados"; the master adds "Iniciar combate" (outside combat) and a "Sessão" menu with "Copiar link da sessão" and "Encerrar sessão". The end asks in place under the bar ("Encerrar a sessão 3? Ela não reabre depois.", "Cancelar" focused, `data-confirm` against a double click), as today. Reconnecting keeps today's notice under the bar. 56 px instead of 190 (1280), 430 (master phone), 240 (player phone). |
| **Rail** (`app-session-rail`, new) | Master combat, master RP, player combat, player RP (4 uses) | A right column from 1024 px, sticky under the bars, as tall as the screen, its own scroll; a tab list (ARIA tabs, arrow keys) with counts in words; panels stay mounted (`hidden` when inactive). |
| **Tab list** (`app-tab-list`, new, used by the rail) | The rail, the tabs under the combat map, the master's "Cena · Mapa · Grupo", the "Mapa" tab's sub-tabs | The same look everywhere; reuse of `app-segmented` is wrong here (it is a radio group, not tabs). |
| **Fold** (`.mr-panel--fold` + a small disclosure header) | Spent action groups, "Outras ações", "Ganchos e anotações", "Registro do combate", scene parts on a phone, campaign B | One look for the three ad hoc toggles that exist today (`scene-open`'s "Ações da cena", `scene-hooks`, the combat log). Open state in memory only. |
| **Pinned phone nav** (`app-session-tabs`, existing) | Every live phone screen | `position: fixed` at the bottom with the safe-area inset; labels "Cena · Ficha · Conversa" (player) and "Cena · Grupo · Conversa" (master; "Combate" during a combat); the sticky turn bar sits right above it. |

The campaign home's route tabs are local to that page (one use), not a shared piece.

```text
+--------------------------------------------------------------------------------------------------+
| < Mirathel    Sessão 3   (o) Ao vivo desde 20:12                           [Dados]   [Sessão v]  |
+------------------------------------------------------------+-------------------------------------+
|                                                            | Copiar link da sessão               |
|                                                            | Encerrar sessão...                  |
+------------------------------------------------------------+-------------------------------------+
| (!) Encerrar a sessão 3? Ela não reabre depois.       [Cancelar]   [Confirmar encerramento]      |
+--------------------------------------------------------------------------------------------------+
```

```text
+------------------------------------------+
| < Sessão 3  (o) Ao vivo          [Dados] |
+------------------------------------------+
```

### 3.7 Other screens with the same problem (short)

| Screen | Problem | Proposal |
| --- | --- | --- |
| Initiative (master) | "Começar o combate" sits in the side column under the surprise card | Put it in the sticky combat bar during setup, where "Próximo turno" will be |
| Player's campaign home | "Regras da mesa", "Conteúdo da mesa", "Magias" are three panels; "Como você rola os dados" is a full panel | One panel "Consultar" with three links; the dice preference folds to its current choice with "Mudar" (about -450 px at 390) |
| Map editor (out of this round) | Calibration card far below the map (rehearsal F2); 657 px of usable height at 1280 x 800 | A later round |

---

## 4. Top five changes by scroll saved

| # | Change | Screens | Saved |
| --- | --- | --- | --- |
| 1 | The Conversa in a sticky rail from 1024 px, and the phone nav pinned | Master RP, master combat, player RP, player combat | Master 1280: the composer from 4,800 to 5,200 px down to on screen (6 screens down and back per narration). Player 1280: from 2,500 to 3,100 to on screen. Phones: the "Conversa" tab and its badge from the end of the page (3,300 to 7,500 px) to always on screen |
| 2 | Master combat B: sticky combat bar, the order, the map and the turn side by side, tabs under the map | Master combat | Page 4,800 to 5,600 → about 1,300 px; the per-turn loop from 3 scroll moves to 0 (about 60 per fight); the order folds to give the map its width |
| 3 | Campaign home in three tabs | Master campaign home | 4,850 → at most 1,500 per tab; 12 actions from 0.5 to 6 screens down to at most one click and one screen |
| 4 | Master RP: "Cena · Mapa · Grupo" | Master RP | 5,100 to 5,700 → about 1,300 to 1,500 per tab; 11 actions on screen or one click away |
| 5 | Player phone combat: "Mover" in the sticky bar, spent groups folded, my numbers at the top | Player combat 390 | "Mover" from 2,100 px to on screen; the page 3,300 to 3,800 → 1,700 to 2,100; my HP from ~2,950 to the top |

Under all five, the session bar takes 130 px (1280), 370 px (master phone) and 180 px (player phone) off every live screen.

## 5. Design boards to draw next (in this order)

| # | Board | Width | Theme | State |
| --- | --- | --- | --- | --- |
| 1 | Combate do mestre · A "Mapa e trilho" | 1280 | claro | Vez do Goblin 1, ataque rolado, "Falta aplicar 5 de dano" em Brisa, aba "Turno" |
| 2 | Combate do mestre · B "Três colunas" | 1280 | claro | O mesmo estado |
| 3 | Campanha do mestre · A "Abas", aba "Mesa" | 1280 | claro | Sessão 4 ao vivo, Brisa esperando aprovação |
| 4 | Campanha do mestre · B "Índice e painéis" | 1280 | claro | Os mesmos dados |
| 5 | Combate do mestre · A, aba "Conversa" | 1280 | escuro | Vez de Pensantus, 3 mensagens novas, "Enviar" contornado |
| 6 | Sessão do mestre · "Cena" com a Conversa ao lado | 1280 | claro | Cena aberta, Bartolo fala, 2 rolagens novas |
| 7 | Combate do jogador · sua vez | 390 | escuro | Pensantus, Ação disponível, Ação bônus usada, barra "Mover 9 m · Encerrar turno", navegação fixa |
| 8 | Cena do jogador | 390 | claro | Imagem à mostra, Bartolo em cena, 5 ações, "Conversa · 3" |
| 9 | Barra da sessão · estados | 390 e 1280 | claro | Jogador, mestre, menu "Sessão" aberto, confirmar encerramento, reconectando |
| 10 | Campanha do mestre · A, aba "Preparação" | 1280 | escuro | 4 mapas, 9 NPCs, 3 encontros |
| 11 | Sessão do mestre · aba "Mapa" | 1280 | escuro | "Névoa e luz" aberta, "Armadilhas · 2" |
| 12 | Combate do jogador · sua vez | 1280 | claro | Trilho "Ações · Conversa", banner fixo com "Mover" |
| 13 | Painel "Ficha" na sessão | 390 | escuro | PV, CA, espaços, "Magias", "Revivificar" |
| 14 | Combate do mestre (a alternativa escolhida) | 390 | escuro | Barra fixa "Próximo turno", navegação "Combate · Grupo · Conversa" |
| 15 | Campanha do mestre · A | 390 | claro | Aba "Mesa" |
| 16 | Trilho, abas e dobras · estados | 1280 e 390 | claro | Contagens, carregando, erro, vazio, aba que não se aplica (some), grupo usado aberto e fechado |

## 6. The questions and their answers

| # | Pergunta | Resposta |
| --- | --- | --- |
| 1 | Combate do mestre no notebook: A "Mapa e trilho" (mapa grande; "Turno" e "Conversa" em abas à direita) ou B "Três colunas" (ordem, mapa e turno lado a lado; mapa menor)? | B "Três colunas", com a coluna da ordem que se recolhe numa faixa de tokens |
| 2 | Página da campanha do mestre: A "Abas" (Mesa · Preparação · Regras) ou B "Uma página com índice e painéis recolhidos"? | A "Abas", com ícones nas abas e nos títulos dos painéis |
| 3 | Os nomes das abas da campanha ("Mesa", "Preparação", "Regras") estão bons? | Sim |
| 4 | "Copiar link da sessão" e "Encerrar sessão" podem ir para um menu "Sessão" no topo da sessão? | Sim |
| 5 | A Conversa fica ao lado da cena a partir de 1024 px (hoje só a partir de 1600 px)? | Sim |
| 6 | No combate, com a Conversa ao lado, "Enviar" fica contornado para "Próximo turno" ser o único botão cheio? | Sim |
| 7 | No celular, "Cena · Ficha · Conversa" fica fixa no rodapé o tempo todo? | Sim; organizar também ações e magias para rolar menos no celular |
| 8 | "Ficha" no celular abre um painel dentro da sessão (PV, CA, espaços, magias e "Abrir a ficha completa") em vez de sair da sessão? | Sim, painel na sessão |
| 9 | As "Ações padrão" (Disparada, Esquivar...) ficam atrás de "Outras ações" no celular? | Sim; o mapa fica sempre no topo do combate do jogador, com duas formas de organizar movimento e ações |
| 10 | Ao abrir uma cena, a aba "Cena" abre sozinha para o mestre? | Sim, só quando o mestre abre; uma cena pode estar aberta só para alguns jogadores |
| 11 | "Imagem para os jogadores" deixa de ser um painel separado quando há cena aberta (fica na faixa "Imagens da cena")? | Sim |
| 12 | Os encontros prontos dos pontos de batalha ficam na aba "Mapa" e no "Iniciar combate"? | Sim |
| 13 | No celular do mestre, a navegação fica "Cena · Grupo · Conversa" ("Combate" durante um combate)? | Sim |

## 7. Docs this would change when built (none changed now)

This is a proposal in `scratch-ms/`; no repository doc changes until a layout is chosen. When it is built: `docs/design/session.md` (the session bar, the rail, the Conversa placement, the phone nav), `docs/design/combat.md` (the master's layout and the player's turn bar), `docs/design/components.md` (rail, tab list, fold), the stories MR-011 ("Sessão" panel and the session link), MR-012 and MR-013 ("In the app" lines) in English and PT-BR, and the progress doc with Samuel's answers.
