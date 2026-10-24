# Compact screens, shared pieces (feat/compact-shared): harness captures

**These are HARNESS CAPTURES, not the live stack** (the VM has no Docker daemon or CockroachDB). The real components were rendered in Chromium with fake data: `app-session-header` (real inputs; `LiveSessionSource` faked), `app-session-tabs` (the pinned nav), `app-session-rail` with `appRailPanel` panels, `app-tab-list` and `app-fold`. The page around them (the app bar, the cards, the fake conversation lines and composer, the fake turn bar) is a stand-in drawn by the harness, so it differs from the boards. Rendered from commit `6efe8e360` (branch `feat/compact-shared`) by a throwaway harness in a detached worktree, never committed to the feature branch. Nothing ran against the server or the stream. Fonts are the app's own.

Matrix: widths 320 (568 high), 390 (844), 768 (1024) and 1280 (900), light and dark (`prefers-color-scheme`), device scale 1. File name: `<state>-<width>-<claro|escuro>.png`. `medidas.json` has the measurements of every capture (bar box, buttons, nav, rail, horizontal scroll).

## States

- `barra-jogador`, `barra-jogador-reconectando`, `barra-jogador-rolando`: the player's bar (back link, "Sessão 3", "Ao vivo desde 20:12", "Dados"); reconnecting (the notice under the bar, "Dados" off); a roll on its way ("Rolando…", off).
- `barra-mestre`, `barra-mestre-em-combate` (no "Iniciar combate"), `barra-mestre-reconectando`: the master's bar with "Iniciar combate", "Dados" and "Sessão".
- `barra-mestre-menu`: the "Sessão" menu open (on a phone it has "Iniciar combate" first). `barra-mestre-encerrar`: "Encerrar a sessão 3? Ela não reabre depois." in place under the bar.
- `nav-jogador` (with the fake turn bar right above it) and `nav-mestre`: the pinned nav on a phone, scrolled; at 320 x 568 both bars leave about 400 px of content.
- `trilho-conversa`, `trilho-ficha` (768 and 1280): the rail with its tab list ("Conversa, 3 mensagens novas" and "Ficha"), the panels mounted and one hidden. The rail is not on any live screen yet.
- `abas-e-dobras`: the tab list ("Cena · Mapa · Grupo" with a count) and the three folds ("Ações da cena" open, "Ganchos e anotações" with "Só você vê", "Registro do combate" with "9 entradas").

## Measured

- The bar is 60 px high from 768 px and 56 px on a phone (as C6-10 draws it), in every state, with no horizontal scroll and no button outside the screen; every control is 44 px high.
- The pinned nav ends at the bottom edge of the screen; the turn bar sits right above it.

## Against C6-10 (the approved states board)

Matched: the bar (chevron back link, a 1 px rule, "Sessão 3" 28 px / 800 on a computer and 24 px on a phone, the pill, "desde 20:12", buttons 44 px high with an 8 px radius and 12 px between them); "Sessão" with its word and a chevron at the right; the menu panel (250 px, a `control-line` border, two 44 px items, "Copiar link da sessão" and "Encerrar sessão…"); the end confirmation on the warning surface with a bold question, "Cancelar" focused with a 2 px accent ring and "Confirmar encerramento" outlined in the danger ink; reconnecting with the pill kept, "Dados" on a dashed border and the notice under the bar (without the time on a phone); the three 390 bars (the master's three dots open the "Sessão" menu); tab counts in amber, grená and neutral; the loading, error and empty states of a rail panel; a tab that does not apply is left out; a 390 list wider than the screen scrolls sideways with an edge fade and keeps the open tab in view; a spent group row, folded and open (`abas-e-dobras`, full page).

## Known differences from the boards

1. The board's note under the master's bar ("Num combate, a barra do combate toma o lugar de “Iniciar combate”.", with an info icon) is built as an annotation of the board, not as text on the screen: in a combat the button is simply absent.
2. At 320 px the live pill leaves the bar (there is no room for it beside the master's controls) and "Dados" is an icon; the app bar's "Ao vivo" link still says it.
3. On a phone the master's "Iniciar combate" is the first item of the "Sessão" menu (the ⋯ button), as the board's caption says the menu opens there; the master phone combat board `C6-01` is another unit's.
4. The harness page around the pieces (the app bar, the cards, the fake turn bar and lines) is a stand-in, so it differs from the boards.
5. Not run here: the real e2e and a11y scans (CI runs them) and the layout audit on these captures.
