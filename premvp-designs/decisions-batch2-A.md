# Decisions on Designer A's open questions (PM-04, PM-06, PM-07), 08/10

1. **No "Pular" on a held window.** The window waits for the master, or for the end of combat (which discards it). A
   timeout would hand a hidden-information decision to a clock.
2. **Palavras de Interrupção:** a setting on the bard's sheet, "Perguntar: em todos os testes do inimigo / só em ataques
   / nunca", default **só em ataques**.
3. **A countered spell's slot is spent.** SRD Counterspell: the creature's spell "fails and has no effect", and casting it
   already expended the slot.
4. **A creature immune to Palavras de Interrupção** (it can't hear the bard, or is immune to being charmed): no prompt and
   no explanation. Nothing leaks.
5. **Destruição Divina against an undead or a fiend:** the server adds the extra 1d8. The **player sees only "+1d8"**,
   never the reason; the master sees the reason. A player never learns a creature's type from the app (RN-10, RN-20).
   The same goes for any extra whose condition depends on hidden facts about the target.
6. **Advantage and disadvantage** (revised after the review): a player may accept the suggested mode or choose
   disadvantage freely. Choosing **advantage** the server did not suggest needs the master's approval: a short prompt in
   the master's queue, with the player's reason. The master may set any mode.
7. **Rests restore resources.**
   - The master's **"Descanso curto"** and **"Descanso longo"** (SRD Resting) restore each resource by its `recharge`.
   - On a short rest, players may spend Hit Dice to heal.
   - A long rest restores HP and half the Hit Dice (at least 1), and ends Ajuda.
   - Draw it in PM-07b: the master's button and the confirmation listing what comes back.
8. **The end of rage:** ask the player, as drawn.
9. **Conjuração Flexível beyond the maximum:** refuse with the reason ("Você já tem o máximo de pontos de feitiçaria");
   the maximum equals the sorcerer level (SRD Font of Magic).
10. **The range disadvantages** (long range, a hostile creature within 5 ft of a ranged attacker): keep them; they are
    SRD rules ("Ranged Attacks").
11. **Labels:** "Em fúria", "Esquivando" and "Marcado" are screen labels: they belong in the web's labels, not in
    `names_pt.json`. For Pack Tactics use "Táticas de Matilha", and mark it for checking against the official
    Portuguese Monster Manual.

12. **The reaction window must not be an oracle** (from the review): a new table rule, "Reações dos inimigos".
    - **"Só quando um inimigo pode reagir"** is the default (faster); its help text says a pause can hint that someone can
      react.
    - **"Sempre"**: every player action against an enemy waits for the master's one-tap answer ("Sem reação" or the
      reaction), so the pause leaks nothing.
    - Vinicius was told the default and can switch it.
