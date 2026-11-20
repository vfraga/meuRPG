# #857 ready-map screens (branch feat/ready-spells-857-web, head 174442427)

Each screen is a PNG at 390, 768 and 1280 px, light (`claro`) and dark (`escuro`), deviceScaleFactor 2. Name: `857-ready-map-<screen>-<width>-<theme>.png`.

| screen | what it shows | board |
|---|---|---|
| `janela-magia` | readied window of the held Bola de Fogo, "Soltar no mapa" | 857-01 |
| `mapa-magia` | cast sheet in release mode, step 1 (band, map, "Voltar") | 857-02 |
| `area-magia` | area preview of the released zone spell, "Quem está na área" | 857-03 |
| `mestre-magia` | master's "Soltar por Pensantus" dialog with the footer | 857-04 |
| `janela-mover` | window of the held move with "Mover agora" | 857-05 |
| `mover-jogador` | Mover page in release mode, player | 857-06 |
| `mover-mestre` | Mover page, master ("Usar pelo jogador") | 857-07 |
| `fila-espera` | master's queue row waiting for the player | 857-08 card 1 |
| `fila-no-mapa` | master's queue row "No mapa" with the 120 s wait | 857-08 card 2 |
| `fila-mover` | master's queue row of the held move, "Mover por Pensantus" | 857-08 card 4 |
| `janela-mover-agarrado` | "Mover agora" dashed, speed 0 (Agarrado) | 857-09 card 6 |
| `janela-outro-aparelho` | window read on the player's second device | 857-09 |
| `mover-desfeito` | the master undid the trigger with the map open | 857-09 |
| `mapa-muralha` | the held Muralha de Fogo released on the map, the start square marked | 857-09 card 3 |
| `mapa-direcao` | the held Mãos Flamejantes released on the map, the direction set | 857-09 card 4 |
| `folha-desfeita` | the release cast sheet after the master undid the trigger | 857-09 card 1 |
| `mestre-fechou` | the master's release dialog after the window closed | 857-09 |
| `dado-digitado` | the release cast sheet's typed-dice step | 857-09 cards 12-13 |
| `resultado` | the release cast sheet's result, "Voltar à sessão" | 857-09 |
| `janela-passou-sozinha` | the window that passed by itself, "Passou sozinho" | 857-09 |
| `mover-bloqueado` | zero-length "Algo bloqueia o caminho" notice | 857-09 |

Every screen was retaken on 174442427 by the a11y scan with `E2E_SHOTS_DIR`.

`janela-foco/` holds the readied windows of the earlier slices, retaken on 174442427: each window opens with the focus on "Deixar passar", and Chromium draws its focus ring there.
