# 736 left-out entries

Screens of the entries the rules refuse (issue 736), at 390, 768 and 1280 px, light (claro) and dark (escuro), viewport only, device scale 2.

- `lista`: the master's content list with the header actions as a 2x2 grid with an icon each ("Regras da mesa" included) and the "entradas fora da campanha" panel (K1, K3).
- `linha`: the row of the entry itself in the list of its kind, with the tag "Fora da campanha" (K1), at 390 and 1280 only.
- `menu`: the "⋮" menu of a row, with "Remover".
- `remover`: the question "Remover X?" (the entry is archived, not deleted).
- `editor-nota`: the entry page with the note and the tag "Fora da campanha".
- `editor`: the entry page scrolled to its header: the tag, the note (K2) and "Remover" (the phone has no field: it reads and removes).
- `campanha`: the master's notice on the campaign's "Regras" tab.
- `jogador`: a player's list, with nothing of it.

Made by `e2e/tests/a11y.spec.ts` ("as entradas fora da campanha") with `E2E_SHOTS_DIR` set.
