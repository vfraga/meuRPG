# Engine L boards: what a build session needs (from the designer)

Boards: `project/<item>-NN-*.dc.html`, a PNG each; `project/canvas.json` ("MeuRPG — Motor L: rolar ao soltar e desfazer o disparo"), two rows: #938 (938-01 to 938-07) and #945 (945-01 to 945-04).
Tools: `python3 tools/build.py [names]` (engine-j's builder, with the classes of this canvas: the typed roll `.ty`, the rays `.rays`, the extra die `.die`, the released line `.chosen`, the mode source `.source`, the done steps `.part2`, the row's "⋮" `.qmore` and its question `.qask`, the readied card `.rcard`), `nice -n 19 node tools/render.js project/<file>.dc.html` (`ZOOM=2 CROP=x,y,w,h OUT=…` for a close look). Sources in `src/`.
Annotations, not UI: the grey numbered captions on the state boards and the dashed "ph-box" (a part that stays as it is today).
The decisions behind the boards are in `decisions.md`.

Read against main 536e6a9e8 (train 131) and the fork's `feat/ready-spells-857` (a4f9c68ff) and `wip/feat/ready-spells-857-web` (d561ac7d3).

## The cast

- Sessão 6, Cripta de Mirathel, Rodada 2, the Ogro's turn. Order: Wight 19, Tor 17, Brisa 14, Pensantus 12, Ogro 9, Jabuti 8, Zumbi 1 at 6. No hidden creature in this combat.
- Tor: Guerreiro 5, Espada longa +6, 1d8 + 3 cortante. He readied the Espada longa on the Ogro, "Quando o ogro passar pela porta", and stands beside the door.
- Pensantus: Mago 7, spell attack +7. He holds Raio de Fogo on the Ogro ("Se o ogro chegar perto de Brisa"); on 938-03 he holds Sono with a 1st-level slot (5d8) in a combat without a map; on 938-06 card 6 he holds Raio Ardente with a 2nd-level slot (three rays), and on 938-07 he casts it on his own turn: two rays on the Wight and one on the Zumbi 1.
- The table rolls physical dice (`DICE_MODE_PHYSICAL`), except where a card says otherwise.
- The app has no gender for a character or a player: sentences name the character ("Tor rola o dado") or say "você" with no adjective that agrees with it.

## Row 1: the roll step of a released readied action (938-01 to 938-07)

The rule: at a table where the dice may be typed, the use of a readied action that rolls is two steps of the same sheet. Nothing is sent before the last button of step 2.

The window (`readied-window-sheet`), step 2 (938-01, 938-02, 938-03):
- Step 1 is the window as it is. Its use button keeps its words: "Atacar o Ogro" for a held attack, "Soltar no Ogro" or "Soltar agora" for a held spell. At `DICE_MODE_PHYSICAL` and `DICE_MODE_PLAYERS_CHOOSE` it opens step 2; at `DICE_MODE_APP` it answers at once.
- Only an action that rolls has a step 2: a held weapon attack, a held spell attack (Raio de Fogo, Raio Ardente) and a held pool spell (Sono, Leque Cromático). Another standard action and a spell with no roll at the cast answer from step 1.
- The header stays ("Ação preparada" or "Soltar Sono", with its subtitle). The body is one line and the picker.
- The line (`.chosen` on the boards, the attack sheet's own "chosen" row without "Trocar"): the icon, a caption ("Ataque preparado", or "Magia preparada · espaço de 1º nível") and "**Espada longa** no Ogro · a 1,5 m", "**Sono** no Ogro e no Zumbi 1", "**Raio Ardente** · 2 raios no Ogro, 1 no Zumbi 1".
- The picker is `app-roll-picker` in its compact size, with the attack sheet's words: "Role 1d20 para Espada longa (+6)", "Role o seu dado e digite o número que saiu (1 a 20).", the field, "+ 6 de bônus", the live total with "Total do ataque".
- With advantage or disadvantage it is `app-multi-roll` with "Primeiro d20" and "Segundo d20" and the hint "… Conta o menor." Above it, one line says why: the tag "Desvantagem" (or "Vantagem") and the server's sentence (`TargetInReach.sources`), as `app-roll-mode-picker` lists them. The window has no mode chooser: the roll uses the mode the server suggests for the target, read from the readier's turn options.
- A pool is `app-roll-picker` with the cast sheet's words: "Role 5d8 para Sono: some os dados", "Digite a soma dos dados, de 5 a 40.", "Total dos dados".
- Raio Ardente uses the per-ray piece below, with "Voltar" beside its button (938-06 card 6).
- An effect's die (Bênção, Perdição) is `app-extra-dice` above the d20, and the button then reads the total, as on the attack sheet (938-06 card 7).
- The foot: the clock line, then "Voltar" (outlined, the back arrow) and the one filled button, side by side and the same size. The filled button is the picker's own confirm ("Confirmar 14", "Confirmar 23"; "Confirmar" for a pair, as `app-multi-roll` writes it; "Confirmar os 3 raios" for the rays). `RollPicker` and `MultiRoll` keep their button inside the component today: they need a way to put "Voltar" beside it (a projected slot in `.actions`, or the sheet's foot driving the picker).
- "Voltar" shows step 1 again with what was picked there. Nothing was sent.
- At `DICE_MODE_PLAYERS_CHOOSE`, step 2 opens with the picker's two buttons, the player's preference filled ("Rolar no app", "Digitar o resultado"), and "Voltar" under them (938-06 card 9).

The d20 of each ray (938-07, 938-06 card 6):
- One piece, with the same words, in two places: the cast sheet on the caster's own turn and step 2 of the window. It takes the place of the single d20 picker whenever a spell attack has more than one ray and the dice are typed.
- "Role 3d20 para Raio Ardente (+7)" and "Um d20 para cada raio, na ordem dos raios (1 a 20 cada)."
- One row per ray, in the order of the targets and, for each target, of its rays: the target's token, "**Raio 1** · Wight", and the face field at the right, always in the same place. Under the name, "1 a 20" while the field is empty, "15 + 7 = 22" once it is typed, and "Digite um número de 1 a 20" in the danger colour for a bad number. No total of the cast: each ray hits or misses by itself.
- With advantage or disadvantage a row has two fields, in the order the dice came, and the line reads "Conta o menor: 9 + 7 = 16" (or "Conta o maior").
- One filled button, "Confirmar os 3 raios". While a face is missing it is dashed, with the reason above it: "Falta o d20 do raio 3." or "Faltam o d20 do raio 2 e o do raio 3."
- In the cast sheet (938-07) the piece comes after the sheet's own parts, which do not change: the slot picker (scrolled away on the board), "Divida os 3 raios entre os alvos" with its steppers, and "Como rolar o d20". The rows follow the steppers: a ray moved to another target moves its row and empties its field. The button is the sheet's sticky foot.
- At `DICE_MODE_PLAYERS_CHOOSE` the two buttons of the picker come first ("Conjurar e rolar no app", "Digitar o resultado"), and typing opens the rows.
- After it, the cast result and each hit ray's damage are the cast sheet's own, as today.

The wait:
- Arriving at step 2 sets the mark the list and the map already set (`MarkReactionRolling`). It is the one wait of 120 s, counted once from the first mark; "Voltar" does not restart it.
- The player reads the reaction sheet's own line: "Você está rolando: a mesa espera 1 min 46 s. Depois disso a reação passa sozinha." (`playerRollingText`).
- The master's row reads "Tor rola o dado · passa sozinho em 1 min 31 s", with the dice icon, in the place of "No mapa"; the band chip says the same (938-06 card 12). Both of the master's buttons stay live.
- A second device of the same player stays on step 1 and reads "Você está rolando em outro aparelho. Passa sozinho em 1 min 31 s." (938-06 card 11). Both may answer; the first answer wins.
- Another player reads "Esperando a reação de Tor" in every moment.

After the answer (938-04, 938-06 cards 4 and 8):
- The window is answered and closed by then, so there is no clock and no "Voltar".
- The same sheet shows the attack sheet's done steps (`app-attack-result`): "Alvo", then "Rolar" with the number, the formula, "dado físico" and the pill "Acertou", "Crítico" or "Errou".
- A hit asks the damage right there, with the attack sheet's damage picker and words: "Acertou: role o dano.", "Role 1d8 para o dano", "Digite a soma dos dados, de 1 a 8. O app soma o modificador.", "+ 3 de modificador", "Dano total", and "Confirmar 8" (the total). It calls `RollDamage` on the pending damage the hit opened. At an app table the button is "Rolar 1d8 no app".
- A miss, and a damage once rolled, end with "Voltar à sessão" (filled), as the release on the map does.
- Raio Ardente shows `app-cast-result` and asks each hit ray's damage, one at a time, with the cast sheet's own damage piece. Sono shows `app-cast-result` ("Sua rolagem", who fell asleep) and "Voltar à sessão".
- A sheet closed before the damage leaves the hit's pending damage waiting. The notice "Falta rolar o dano do seu ataque." with "Rolar o dano" (`action-groups`) shows off the player's turn too, and opens the attack sheet at the damage (`resume`).

The master (938-05, 938-06 cards 5 and 12):
- "Atacar por Tor" and "Soltar por Pensantus" open the same step inside the row, in the 368 px column. No dialog.
- The picker is the master's variant (`outlined`, `hideTotal`, as his other cards): "Rolar no app" and "Digitar por Tor" side by side, then the field with "Digite o número que saiu no d20 de Tor (1 a 20).", "+ 6 de bônus", and "Voltar" beside "Confirmar 14", both outlined. "Rolar no app" stays as a link under them. The master rolls either way whatever the table's mode.
- While the step is open on a screen, that screen does not send the automatic pass of that window. The wait line and the band chip read "Você responde por Tor · o relógio espera".
- The foot of the column says "Tor ainda pode responder no próprio aparelho: vale a primeira resposta."
- A hit the master rolled for Tor asks the damage in the same row, with the same picker.

States (938-06):
- A number out of range: the field turns to the danger colours, "Digite um número de 1 a 20" under it, and the confirm button is dashed with no number.
- The 120 s end while typing: the step gives way to the window's own "gone" sentence ("Passou sozinho: você não respondeu em 2 min.") and "Fechar". The typed number is not sent.
- The window goes away while typing because the master undid the fire: see row 2.
- A refusal is a danger notice at the top of the step; the number stays in the field and both buttons stay live. A refusal spends nothing and leaves the window open.
- A combat with a map releases Sono and Leque Cromático on the map (their area is placed), with the cast sheet's own picker (857-09 card 12). The pool step of the window is for a combat without a map, as 938-03 draws.

## Row 2: the master undoes a fired trigger (945-01 to 945-04)

The row (`reaction-queue`, 945-01):
- Every row of a READIED window that takes an answer has a "⋮" at the end of its name line, 44 × 44 px, named "Mais ações da janela de Tor". Its menu has one item: the undo icon, "Desfazer disparo" and, under it, "A janela fecha e a ação volta a esperar."
- The other reactions of the queue get no "⋮".
- A row whose use is held (`answer_now` false: the roll waits for a die or for other windows) has no "⋮": the server refuses that undo, and the menu would be empty (945-04 card 1).
- The player being on step 2 or on the map does not take the "⋮" away: the window is still unanswered.
- The list of creatures and the two buttons of the row are main's `app-readied-aim` and are not changed. Its line "O alvo que você disse" is the readier's sentence; in the master's row it reads better as "O alvo que Tor disse".

The question (945-02):
- It takes the place of the row's list and buttons, under the name, the description and the wait line: "**Desfazer o disparo?** A janela de Tor fecha e a ação preparada volta a esperar. Nada é gasto." Then "Voltar" (first, with the focus) and "Desfazer disparo" (outlined, the undo icon). `role="alertdialog"`, as the log's own question.
- "Desfazer disparo" calls `UndoLastAction` with the log's `undoable_event_id`.
- While the question is open on a screen, that screen does not send the automatic pass of that window, and the wait line reads "Esperando Tor · o relógio espera".
- While it is sent, both buttons are dashed and the second reads "Desfazendo…".
- A refusal closes the question and shows the row again with a warning notice in it (945-04 card 2): "Outra ação aconteceu antes: confira o registro." (`aborted`), "Não há mais nada para desfazer: só a última ação pode ser desfeita." (`NOTHING_TO_UNDO`), "Uma reação ainda espera resposta. Espere a resposta ou responda no lugar de quem reage." (`REACTION_PENDING`), or the network sentence.

After it (945-04 card 4):
- The row leaves the queue on every device of the master; a question open on another device closes with it.
- The readier's row is back in "Ações preparadas" with "Disparar", the order keeps "Preparada", and "Próximo turno" is live again.
- The log has no line for the fire and none for its undo.
- A status for a screen reader: "Disparo desfeito: a ação preparada de Tor voltou a esperar."

The player (945-03, 945-04 cards 5 and 6):
- On step 1 the sheet shows the sentence in place of the question, with "Fechar" focused, as a window that went away does.
- On step 2 and on the map the sheet closes by itself and the combat page shows the sentence as a neutral notice with the undo icon and "Entendi", above the turn card. Nothing typed is sent.
- The readied card on the turn card reads "Esperando o gatilho · ■ reação livre" again.
- The sentence is one for every kind of held action: "O mestre desfez o gatilho: a ação preparada (Espada longa) volta a esperar." (the name in brackets: the weapon, the spell, "Mover", or the action). It never agrees with the gender of a name, and it replaces the two sentences of `releaseGoneView`.
- Other players read nothing: "Esperando a reação de Tor" goes away.

## Heights

Every phone screen is 390×844 and every desktop screen is 1280×800.
The step-2 sheets cover 474 px (938-01), 599 px (938-02) and 474 px (938-03) of the phone; the damage sheet covers 573 px (938-04).
The master's column ends at 764 px with the step open (938-05), at 774 px with the menu open over a list of three creatures (945-01) and at 702 px with the question (945-02).
The cast sheet with the three rays fills the phone (938-07): its targets, "Como rolar o d20", the three rows and the button fit one screen once the slot picker is scrolled away.
The state boards are not screens: 938-06 is 1680 px and 945-04 is 1190 px.

Every new screen and state goes into `e2e/tests/a11y.spec.ts`.

Glossary (PT-BR / EN): Rolar ao soltar / Roll on release; Tor rola o dado / Tor rolls the die; Desfazer disparo / Undo the fire; O relógio espera / The clock waits.

## Server changes each session builds

core (#938), on the window's answer (`AnswerReaction`), each named by what it carries:
- The second face of the d20 pair of a held weapon attack or spell attack rolled with advantage or disadvantage. The answer carries one typed number, which becomes one d20 face.
- The faces of the dice an effect adds to the attack roll (Bênção, Perdição, a maneuver die). `RollAttack` takes them; the answer does not.
- The sum of a held pool spell's dice (Sono, Leque Cromático). The answer's one typed number goes to the cast as a d20 face, which a pool spell refuses.
- The d20 faces of each ray of a spell attack with several rays (Raio Ardente): one face, or a pair with advantage or disadvantage, for each ray, in the order of the targets and of the rays each target takes. On the cast (`CastSpell`, which takes one typed d20 and refuses it for more than one ray) and on the window's answer alike.
- What the release rolled, in the answer's response: the attack roll with its outcome and the pending damage it opened, or the cast with each target's result and its pending damages. The response carries the combat and a result with no roll, so the sheet cannot show "Acertou" or ask the damage from it.
- What the reactor's player is busy with (the map, or the dice), beside the mark. The mark is one flag, and the master's row reads "No mapa" for any of them.
- Web only: the window's sheet gets the table's dice mode and the player's preference, as the reaction sheet has them.

core (#945):
- Whether the log's undoable event is the fire of a given window. The web reads one id (`undoable_event_id`) with no entry behind it, since a fire writes no line; a flag on the window, or the fire's event id on it, lets the row show "⋮" only when the undo will work.
- `docs/architecture/play/readying.md` and the product rules in both languages: the master takes a fire back from the queue row.
