# compact-master-combat: the master's combat, alternative B "Três colunas"

Branch `feat/compact-master-combat`. **These are harness captures, not the real stack** (the VM has no Docker or CockroachDB): the real
`CombatView`, `OrderList`, `CombatBar`, `NpcCard`, `SessionRail` and the tab list, mounted on a page with a fake combat client and a fake
map. File names end in the width and the theme (`-claro`, `-escuro`).

| Set | Files |
| --- | --- |
| 1280, six players | `01-computador-6-jogadores` |
| 1280, eight players, page scrolled (the order sticks, "Mais 4 abaixo") | `02-computador-8-jogadores-rolado` |
| 1280, one player | `03-computador-1-jogador` |
| A row opened in place | `04-linha-aberta` |
| The order folded to a strip | `05-ordem-recolhida` |
| "Mais do combate" in the rail (option C, C7-02/C7-03): closed, "Efeitos" open, "Névoa e luz" open with the order folded and eight players, "Registro" open | `05` to `08` |
| The rail's "Conversa" tab | `09-coluna-conversa` |
| 1100, 1024 | `10`, `11` |
| Initiative setup ("Começar o combate" in the bar) | `12-preparo-da-iniciativa`, `21-celular-preparo` |
| Theatre of the mind | `13-teatro-da-mente`, `22-celular-teatro` |
| 768 (tablet): "Turno", "Ordem" | `14`, `15` |
| 390 (phone): "Turno", "Ordem" (8, row open; 1), "Mapa", "Mais" open | `16` to `20` |

The layout audit (`auditLayout` from `e2e/tests/layout.ts`) found nothing at 320, 390, 1024 and 1280, light and dark, in the six-player,
eight-player, one-player, setup and theatre states.

## Differences from the boards

- The harness has no roll result and no "Aplicar 5 de dano" block in the rail's NPC card (its fixture is not the server's shape): the card
  shows the three stat boxes, the economy chips and the grapple block only.
- The rail's "Conversa" in these captures is a stand-in; its "Enviar" is outlined in the real conversation while a combat runs (`MasterComposer`).
- "Efeitos" has no count in its row (nothing cheap to count); "Armadilhas" and "Tesouro" show the map's points. "Registro" is a closed row, as on the board.
- "Névoa e luz", "Armadilhas", "Tesouro" and "Quebra-cabeças" are the existing panels, projected into their rows of "Mais do combate"; they are not redrawn (the harness stubs are empty, and the panels keep their own heading and card inside the row).
- The map card is its natural height, not stretched to the column: the map's column is sticky and scrolls by itself if it overflows.
- Between 1024 and 1279 px the order sits in the rail under "Turno" (no third column); the initiative setup keeps one column below 1280.
- On the phone the nav's "Grupo" still goes to the campaign page; the "Grupo" panel is a later unit.
- The sticky heights (combat bar, app bar) are constants (`--tri-top: 164px`).
