# Combat rehearsal fixes (branch `fix/combat-rehearsal-1`)

Before = `origin/main` b8949efd7. After = `fix/combat-rehearsal-1` f7eb911ed (`idle-after-*` re-shot from e7e7b8049, which fixes the confirmation button label). Each screen is the real component (`NpcCard`, `CombatBar`, `JointCard`) mounted on a throw-away page with mocked data (master's view), rendered at 390 and 1280 px. Files are `<item>-<before|after>-<light|dark>-<390|1280>.png`. Dark exists only for the d20 choice.

| Item | Files | Shows |
| --- | --- | --- |
| 1. NPC attack: "Como rolar o d20" | `npc-*` (light and dark) | Carniçal attacking Toren, who is dying (the mock sends advantage, source "alvo inconsciente", and the automatic critical note). After: the Normal / Vantagem / Desvantagem choice with Vantagem marked "sugerido". Before: no choice, only the line "Vantagem: alvo inconsciente" above the roll buttons. |
| 2. "Próximo turno" on an unacted player turn | `idle-*` | The combat bar on Toren's turn after a click on "Próximo turno". After: the in-place question "Passar a vez de Toren? Ele ainda não agiu." with "Cancelar" and "Passar a vez". Before: the click passes the turn at once, so the bar is shown unchanged (nothing to ask). |
| 3a. Joint turn title | `title-*` | Bar title of a joint turn of two NPCs "Espada voadora 2" and "Espada voadora 3". Before: "Vez das Espada voadoras". After: "Vez das Espadas voadoras". |
| 3b. Stale "A vez passou para" | `stale-*` | `JointCard` fed a sequence: joint turn (Brisa, Toren) in round 1, then Capitão Goblin alone, then Brisa alone in round 2. Before: the line still reads "A vez passou para Capitão Goblin" in round 2. After: the line is empty. |

## Caveats
- Mock data only; no server, so the app header says "Servidor indisponível" and the page around each component is the app shell.
- The "A vez passou para" line is a screen-reader live region (visually hidden in the app). The `stale-*` shots reveal it with an injected dashed box so it can be compared; "(linha vazia)" in the after shots is a label added by the capture, not by the app.
- Not visible in any shot: the server rules for the unconscious target (`combat_rollmode.go`, `combat_reaction_window.go`) and the `by_master` attribution; the backend tests cover them.
- The mocked d20 choice never rolls; "Rolar 2d20 no app" and "Digitar o resultado" are the existing buttons, unchanged.
