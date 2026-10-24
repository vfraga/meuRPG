# Compact screens, shared pieces (feat/compact-shared): harness captures

**These are HARNESS CAPTURES, not the live stack** (the VM has no Docker daemon or CockroachDB). The real components were rendered in Chromium with fake data: `app-session-header` (real inputs; `LiveSessionSource` faked), `app-session-tabs` (the pinned nav), `app-session-rail` with `appRailPanel` panels, `app-tab-list` and `app-fold`. The page around them (the app bar, the cards, the fake conversation lines and composer, the fake turn bar) is a stand-in drawn by the harness, so it differs from the boards. Rendered from commit `a8d1600f9` (branch `feat/compact-shared`) by a throwaway harness in a detached worktree, never committed to the feature branch. Nothing ran against the server or the stream. Fonts are the app's own.

Matrix: widths 320 (568 high), 390 (844), 768 (1024) and 1280 (900), light and dark (`prefers-color-scheme`), device scale 1. File name: `<state>-<width>-<claro|escuro>.png`. `medidas.json` has the measurements of every capture (bar box, buttons, nav, rail, horizontal scroll).

## States

- `barra-jogador`, `barra-jogador-reconectando`, `barra-jogador-rolando`: the player's bar (back link, "Sessão 3", "Ao vivo desde 20:12", "Dados"); reconnecting (the notice under the bar, "Dados" off); a roll on its way ("Rolando…", off).
- `barra-mestre`, `barra-mestre-em-combate` (no "Iniciar combate"), `barra-mestre-reconectando`: the master's bar with "Iniciar combate", "Dados" and "Sessão".
- `barra-mestre-menu`: the "Sessão" menu open (on a phone it has "Iniciar combate" first). `barra-mestre-encerrar`: "Encerrar a sessão 3? Ela não reabre depois." in place under the bar.
- `nav-jogador` (with the fake turn bar right above it) and `nav-mestre`: the pinned nav on a phone, scrolled; at 320 x 568 both bars leave about 400 px of content.
- `trilho-conversa`, `trilho-ficha` (768 and 1280): the rail with its tab list ("Conversa, 3 mensagens novas" and "Ficha"), the panels mounted and one hidden. The rail is not on any live screen yet.
- `abas-e-dobras`: the tab list ("Cena · Mapa · Grupo" with a count) and the three folds ("Ações da cena" open, "Ganchos e anotações" with "Só você vê", "Registro do combate" with "9 entradas").

## Measured

- The bar is 56 px high at every width, in every state, with no horizontal scroll and no button outside the screen; every control is 44 px high.
- The pinned nav ends at the bottom edge of the screen; the turn bar sits right above it.

## Known differences from the boards

1. The board's "Sessão 3" is larger ; the build uses 28 px (22 px on a phone) so the one row stays 56 px.
2. The board draws a thin chevron as the back link; the build uses the Material `arrow_back` icon (the set the app ships).
3. At 320 px the live pill leaves the bar (there is no room for it beside the master's two icon buttons); the app bar's "Ao vivo" link still says it. On a phone the master's "Dados" is an icon only (aria-label "Dados"); the player's keeps its word down to 359 px.
4. The master's "Iniciar combate" is the first item of the menu on a phone: no master phone board exists yet.
5. The states board for the bar and the rail is not here yet, so the bar's confirm, menu and offline states follow the proposal text; the rail's look (a plain column with its tab list) follows the proposal, not a board.
6. Not run here: the real e2e and a11y scans (CI runs them) and the layout audit on these captures.
