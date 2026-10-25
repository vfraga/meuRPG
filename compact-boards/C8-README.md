# C8: the master acts for an absent player's character (plan item B4)

The server already lets the master act for any player character on that character's turn, and answer a held reaction question the player leaves unanswered for 10 s. These boards give the master the screen for it. They are built from the master's combat B (C1-02, C7-02), the player's action tabs (C4-03, C6-07) and the target picked on the map (C7-04, C7-05). The generator is `c8.js`. The boards sit in the canvas row "Combate: o mestre age por um personagem cujo jogador não está".

| Board | What it decides |
| --- | --- |
| C8-01, 1280 light (and the same screen dark) | On a player character's turn, the master's "Turno" shows that character's turn card with **"Agir por Pensantus"** (one outlined button). There is no "absent" mode to switch on first: the button is on every player turn. Acting, the rail shows a band **"Você está agindo por Pensantus"** with **"Voltar ao mestre"**, then his economy shapes, HP, AC and slots, and his own action tabs (Ataques, Magias, Mago, Outras, from his sheet as on C4-03). The combat bar says "Você age por Pensantus", and its one filled button reads **"Encerrar o turno de Pensantus"**. The target is picked on the map as on C7-05, and only what Pensantus sees counts: the hidden Cultista is dimmed ("Pensantus não vê este alvo"). "Voltar ao mestre" returns the rail without ending the turn. To move, the master drags his token, or uses "Mover" in the map's header when no target is being picked. |
| C8-02, 390 light | The same on the master's phone (C6-01). Turno shows the card with "Agir por Pensantus", and the pinned bar has "Próximo turno" with "Esperando Pensantus". Acting, the pinned bar becomes "Encerrar o turno de Pensantus"; while a target is picked, the target bar of C7-04 takes its place, saying "Por Pensantus". His four tabs drop their counts at 390, because four tabs with counts do not fit. |
| C8-03, 1280 and 390 light | A character at 0 HP: the turn card is **"Toren está caído: teste contra a morte"**, before anything else. It shows the successes and failures so far (shape and words), one line of the SRD rule, **"Rolar por Toren"** (app dice) and **"Digitar o resultado"** (physical dice), at the same size. "Próximo turno" is dashed until the save is made, and the combat bar says why ("Falta o teste contra a morte"). There is no "Agir por Toren". |
| C8-04, 1280 and 390 light | A held reaction question left unanswered for 10 s comes to the master at the top of "Turno", above the NPC card. It reads "Pensantus pode usar Escudo Arcano", with the numbers (the attack of 16 hits AC 12; with the spell, AC 17, it misses), the cost (the reaction and a 1st-level slot, 3 of 4 free), a "10 s sem resposta" tag, and "Usar" / "Não usar" at the same size, outlined. The tab count ("Turno · 2") and the combat bar ("Falta responder por Pensantus") also say something waits. |
| C8-05, 1280 and 390 light | In any list of who still has to roll, **"Rolar por Brisa"** sits beside the one waiting. The boards show this for a group check in the scene ("Teste em grupo: Furtividade · CD 12", 2 of 3 rolled) and for initiative at the start of combat. Brisa's line says what she rolls with (Furtividade +4, iniciativa +2). "Começar o combate" is dashed until everyone has initiative, and the bar says who is missing. |

## Departures from the brief

- **C8-01:** "Encerrar o turno de Pensantus" is in the combat bar, replacing "Próximo turno", not in the rail. Two end-turn buttons would do the same thing, and on the phone it is also the pinned bar's button. The rail keeps "Voltar ao mestre".
- **C8-04:** the brief's example was "Brisa pode usar Escudo Arcano… CA 13". Brisa is a bard, and Shield is not on the bard's SRD 5.1 list, so the board uses Pensantus (wizard, AC 12), whose sheet has Escudo Arcano (C4-03).

## Open questions

1. Does the player, and the rest of the table, see that the master acted for the character? For example, "pelo mestre" in the log and the feed, or a line on the player's screen when they come back.
2. If the player comes back mid-turn, who has the turn? The boards assume the master keeps it until "Voltar ao mestre" or the end of the turn.
3. When the master acts for a character, is it bound by what that character sees (fog, hidden NPCs)? The boards say yes (the Cultista is dimmed); the server should enforce the same.
4. While acting, can the master still use the master's own tools on the order rows (damage, conditions), or do they wait for "Voltar ao mestre"?
5. Can the master start the combat before every character has initiative (for example, rolling for all who are missing at once)? The boards show "Começar o combate" disabled until everyone has rolled.
6. Should "Rolar por Brisa" also offer "Digitar o resultado" for physical dice, as the death save does? The board shows only the app roll.
7. A reaction the master answers late: if the player answers after the master, which answer wins? The boards assume the first one.
