# 715 — Trocar uma opção de característica no subir de nível

Board of reference: `engine-boards-k/k1-10-subir-de-nivel-trocar-uma-infusao-dono-390-claro.png` (the group is one more group of the one panel "Trocar uma escolha"; at a level without the ability step the panel is in "Escolhas").

Character: Doran, Guerreiro 4 of the table subclass "Mestre das Posturas" (feature "Posturas", swap allowed at class levels 5 and 6). Level 5 has no ability step; level 6 has one, with the table rule "Trocar uma escolha no aumento de atributo" on, so the panel holds "Estilo de luta" and "Posturas".

Every state exists at 390, 768 and 1280 px, light (`claro`) and dark (`escuro`), scale 2. File: `715-<state>-<width>-<claro|escuro>.png`.

| State | File prefix | What it shows |
|---|---|---|
| Escolhas, before a pick | `715-escolhas-1-antes` | step 3 of 4, panel "Trocar escolhas": "Não trocar" (with its check) on its own full-width row, "Posturas" below it |
| Escolhas, group open | `715-escolhas-2-conjunto-aberto` | "Posturas" open (border and fill, no check); from 768 up "Abrir mão de" and "Pegar no lugar" sit side by side; the hint "Falta escolher. Escolha de que abrir mão e o que pegar no lugar. Sem os dois, a troca não vale." with a warning icon |
| Escolhas, half a swap | `715-escolhas-4-meia-troca` | only "Postura da Garça" chosen ("sai" tag), the hint still up, no "Trocas escolhidas" list |
| Escolhas, after a pick | `715-escolhas-3-depois` | list "Trocas escolhidas" under the chips ("Posturas: Postura da Garça → Postura do Lobo", "Vale ao confirmar o nível."), "sai" on the Garça, "entra" on the Lobo, check on "Posturas" |
| Escolhas, 390 x 844 | `715-escolhas-3-depois-390x844-*` | viewport shots: the natural scroll after the last tap, and (`-topo`) the top of the page |
| Escolhas, 390 x 844 with the session banner | `715-escolhas-3-depois-390x844-faixa-topo` / `-faixa-lista` | the banner of another campaign's session ("A sessão 1 de Mesa do Vale começou.") up, light and dark: the top of the page, and the page scrolled to the list, whose lines are above the sticky bar |
| Escolhas, long names | `715-escolhas-5-nomes-longos` | a group "Posturas Ancestrais do Vale Seco e das Montanhas Geladas" (the chip takes the row) and options of two lines |
| Resumo | `715-resumo` | "Posturas" as a row of the table, "Postura da Garça → Postura do Lobo" (no loose sentence) |
| Resumo, half a swap | `715-resumo-meia-troca` | "Próximo" pressed with half a swap: the summary has no row for it (the half swap is not sent) |
| Ficha, after confirming | `715-ficha-depois` | the sheet at level 5 (not shot again) |
| Habilidades | `715-habilidades-1-antes` | ASI level 6: the panel now comes after "O que muda com o aumento"; "Não trocar" on its row, "Estilo de Luta" and "Posturas" below |
| Habilidades, two swaps | `715-habilidades-2-duas-trocas` | "Trocas escolhidas" with "Estilo de Luta: Defesa → Duelismo" and "Posturas: Postura do Touro → Postura do Urso"; both chips checked, "Posturas" open; at 1280 the same two rows in "O que muda até aqui" |
| Habilidades, "Não trocar" after two swaps | `715-habilidades-3-nao-trocar-depois-de-duas` | both swaps gone, the list gone, the check back on "Não trocar" |
| Resumo, two swaps | `715-resumo-duas-trocas` | the two swaps as rows of the table |
| Ficha, the swap line | `715-ficha-linha-de-troca` (390 and 1280, light only, the feature's block) | "**Troca:** nos níveis 5 e 6 de Guerreiro você pode trocar uma opção desta característica por outra." with the bold lead |

Notes:
- Every state exists at 390, 768 and 1280 px, light (`claro`) and dark (`escuro`), scale 2, except where the table says otherwise; the full-page shots are stitched, so the sticky footer and header of the page can sit in the middle of a tall shot (an artefact of the capture, not of the page).
- The check has one meaning: this group has a swap, or "Não trocar" is the choice. An open group is shown by border and fill alone. In the lists, the chosen rows carry "sai" / "entra" instead of a check.
- "Próximo" is filled at every width when it is enabled; it stays dashed while a choice is missing (the "Habilidades" shots).
- The pick names lose the group's lead in the panel and the rows ("Estilo de Luta: Defesa" is "Defesa" under "Estilo de Luta"); the group is "Estilo de Luta" everywhere (the server's name for the rule's group was "Estilo de luta").
- The banner of the 390 x 844 shots is the one of a session in another campaign of the same player ("Mesa do Vale"); no fixture name.
