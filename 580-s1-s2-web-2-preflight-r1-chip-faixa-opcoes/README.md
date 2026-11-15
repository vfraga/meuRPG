# #580 web 2, preflight round 1: the chip, the kinds strip and the option rows that ask

Light is "claro", dark is "escuro". All shots are taken at deviceScaleFactor 2 from the current build of `feat/table-prereq-580-web-2`.

| File | Board | What it shows |
|---|---|---|
| `580-04-falta-escolher-chip-{390,768,1280}-{claro,escuro}` | 580-04 | Level-up, "Incremento ou talento", "Um talento" with no feat picked. "Falta escolher" is the pending tag (warning ink, the colour of the footer line "Falta escolher o talento."), 12 px above "Aumentar habilidades / Um talento". At 768 the five steps end inside the card's right edge (the label of the current step only). |
| `580-12-tipos-borda-esquerda-768-{claro,escuro}` | 580-12 | Kinds strip at 768, page opened on the first kind ("Classes"): the strip is at its left edge, the fade (40 px) is only at the right where "Talentos" is cut. |
| `580-12-tipos-no-meio-768-{claro,escuro}` | 580-12 | Kind in the middle active ("Magias"): the strip scrolled it to the middle, both edges fade. |
| `580-12-tipos-borda-direita-768-{claro,escuro}` | 580-12 | Page opened on "Armas" (the last kind): the strip scrolled to its right edge, "Armas" is in sight with its frame, the fade is only at the left. |
| `580-12-tipos-tipo-aberto-390-{claro,escuro}` | 580-12 | Phone: there is no strip below 768 px; the kind is the "Tipo" select, which names the open kind ("Armas"). |
| `580-03-opcao-com-o-que-pede-{768,1280}-{claro,escuro}` | 580-03 | "Opção 2" of a subclass feature, with "O que a opção pede": "Nível mínimo" 140 px with "de 20" inside, "Outras opções" with its hint ending "Nunca a própria opção.", "Adicionar pré-requisito" as a text action with a caret, the "⋮" in the option's head, and no doubled outline at the edges of the accent frame. No field has a focus ring (blurred before the shot). |
| `580-03-menu-da-opcao-{768,1280}-{claro,escuro}` | 580-03 | The "⋮" menu of the option's row, open: "Remover opção". |
| `580-03-adicionar-pre-requisito-menu-{768,1280}-{claro,escuro}` | 580-03, 580-02 | The "Adicionar pré-requisito" menu open under the text action: the four kinds, the ones on the list as "Já na lista". |

Not shot, because the app does not have them: the collapsed option rows with their summary line, the accent frame on the open option row, the up/down arrows on an option row, "Como os jogadores veem" with the muted "Pede ...", and the sibling-option chip with its "Opção 1" badge (see the report).
