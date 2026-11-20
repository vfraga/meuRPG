# #574 part D, round 1 of exp/574d-b: pick refusals on the field

Taken on exp/574d-b at 56e93b5f2.

The states that change in round 1, at 390, 768 and 1280 px, light and dark.

- `1-troca-de-classe-com-pericias-marcadas`: the wizard after the class changes under two ticked skills; the pickers show the new class's list and the first pick that no longer fits shows the server's sentence on its row. No before: the wizard kept the old class's lists.
- `2-recusa-no-campo`: a refused save, with the server's sentence on the skill's row. Before: `../574-pericias-das-listas/2-recusa-na-etapa-*`, a notice above the picker.
- `3-ferramenta-recusada-no-cartao`: a refused tool pick, with the server's sentence on its card. No before: the save showed the generic message.
- `4-aviso-do-mestre-fora-das-listas`: the master reads "Escolha fora das listas." on the sheet. Unchanged by this round; it gains its a11y row.
