# The master acts for a player's character (MR-053)

Screens of branch `feat/master-acts-for-player` (head e620b68ad), rendered by the real `CombatView` with mocked data (master's view), 390 and 1280 px, light (`claro`) and dark (`escuro`). Files are `<screen>-<theme>-<width>.png`.

| File | Shows | Board |
| --- | --- | --- |
| `c8-01-card-*` | "Turno" on a player's turn: "Vez de Pensantus", HP/CA/Deslocamento, shapes, "Agir por Pensantus"; the bar keeps "Próximo turno" (phone: "Esperando Pensantus") | C8-01 (lower left), C8-02 step 1 |
| `c8-02-acting-*` | Acting: band "Você está agindo por Pensantus" with "Voltar ao mestre", shapes, numbers, the character's tabs; "Encerrar o turno de Pensantus" and "Você age por Pensantus" | C8-01, C8-02 step 2 |
| `c8-02-target-unseen-*` | Choosing a target on the map for Pensantus ("Toque no alvo · por Pensantus"); the hidden Cultista is listed as `not_seen` and dimmed | C8-01 (right), C8-02 step 3 |
| `c8-03-death-*` | Toren at 0 HP: "Toren está caído: teste contra a morte", marks, "Rolar por Toren", "Digitar o resultado"; "Próximo turno" dashed, "Falta o teste contra a morte" | C8-03 |
| `c8-04-late-*` | A reaction nobody answered in 10 s: "Pensantus pode usar Escudo Arcano", "10 s sem resposta", numbers, cost with the slot count, "Usar" / "Não usar"; bar "Falta responder por Pensantus" | C8-04 |
| `c8-05-initiative-*` | Setup: initiative rows with "Rolar por Brisa" beside "Digitar pelo jogador"; "Começar o combate" blocked | C8-05 (initiative) |

## Known differences from the boards
- The unseen-target chip "Pensantus não vê este alvo" is said on a tap (map chip from 600 px of map, otherwise in the bar's hint); the screenshots show the dimmed token only.
- The slot count reads "(3 livres)": the server sends only the free slots, not the total ("3 de 4").
- C8-05: the setup keeps the existing initiative layout (list and side card), not the board's "Turno" rail; the bar says why in the side card, not "Falta Brisa". The scene's group check is not shown (it already had "Rolar por").
- The rail's "Turno · Conversa" tabs are missing (the mock has no chat); the mock has no class tab, so the 4-tab count drop of C8-02 is not exercised. The map is a flat mock image; "Servidor indisponível" in the header is the mock having no server.
