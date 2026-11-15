# Engine J boards: what a build session needs (from the designer)

Boards: `project/<item>-NN-*.dc.html`, a PNG each; `project/canvas.json` ("MeuRPG — Motor J: soltar no mapa, rituais do livro e Palavras de Interrupção"), three rows: #857 part 2 with #856 (857-01 to 857-09), #587 slice 2 (587-01 to 587-06, with 587-04b), #591 (591-01 to 591-08).
Tools: `python3 tools/build.py [names]` (engine-i's builder, with `[[inc:name]]` for a shared fragment `src/_name.html`, `"base"` maps from `src/_maps.json`, a Large token, the area picker's marks, a wall, the out-of-range dim, and the classes of this canvas), `node tools/render.js project/<file>.dc.html` (`ZOOM=2 CROP=x,y,w,h OUT=…` for a close look). Sources in `src/`.
Annotations, not UI: the grey numbered captions on the state boards, the dashed "Nota do quadro" boxes, the "Rolada até …" and "Rolada: … está acima" lines and the dashed "ph-box" boxes (a part that stays as it is today).
The decisions behind the boards are in `decisions.md`.

Read against main 6c64f3021 (train 129) and the fork's `feat/ready-spells-857` (579eeec2e), `feat/book-of-secrets-587` (a94b8098b) and `wip/feat/cutting-words-591` (648b7e7a7).

## The cast

- Sessão 6, Cripta de Mirathel, Rodada 2, the Ogro's turn. Order: Wight 19, Tor 17, Brisa 14, Pensantus 12, Ogro 9, Jabuti 8, Zumbi 1 and 2 at 6, and a Sombra hidden from the players.
- Pensantus: Mago 7, slots 4, 3, 3 and 1. On his turn he readied Bola de Fogo with a 3rd-level slot, "Quando o ogro passar pela porta". Tor: Guerreiro 5, 9,0 m; he readied a move with the same trigger.
- Mirela: Bruxo 5, Humano, Acólito, Pacto do Tomo, with the invocation Livro de Segredos Antigos. Carisma +3, CD 14, attack +6, two pact slots of 3rd level. The book holds rituals up to 3rd level (half of 5, rounded up).
- Brisa: Bardo 5 of the Colégio do Conhecimento, d8, 3 uses of Inspiração de Bardo. bilbo: Bardo 3 of the same college, d6. Both have "Perguntar" on "Em todos os testes do inimigo".
- The scene of row 3 is "A taverna do Javali Cego". Bartolo is an NPC typed by hand (no stat block); the Capitão bandido is the SRD creature (Enganação +4); the Espião has a hidden token on the map.
- Spell names, schools, casting times and classes are the app's own data (`names_pt.json`, `spells.json`).
- The app has no gender for a character or a player: the catalog's names as they are ("Bruxo 5"), and "quem joga com Mirela" in a sentence.
- A marker on the map is a "token"; "ficha" is the sheet.
- The app writes "3º nível", never "círculo".

## Row 1: releasing with the map (857-01 to 857-09)

The rule: nothing of the map goes into the alertdialog. The window sends the player to the screen that already does it, in a "release" mode, and that screen has a way back.

The window (`readied-window-sheet`, 857-01 and 857-05):
- A held spell whose `SpellTargets.placement` is a point, a direction or a wall keeps the title "Soltar Bola de Fogo" and the two buttons. The second one is "Soltar no mapa" (map icon, filled). It sends no answer: it opens the cast sheet.
- A held move keeps "Ação preparada" and gets "Mover agora" (walk icon, filled) in place of the line "Por aqui só dá para deixar passar." It opens the Mover page.
- Both carry one neutral notice with the map icon: the map opens, the table waits up to 2 min there, nothing is spent before the last button, and there is a way back.
- The note of a held spell reads "Guardada, a magia prende a sua concentração. Deixar passar perde a magia e o espaço." It is true for a spell that keeps its concentration once released (Muralha de Fogo) and for one that does not. `SPELL_NOTE` in `core/combat/reaction-readied.ts` says the concentration ends when the spell leaves: it changes for every held spell.
- "Deixar passar" stays first and keeps the focus.
- With a speed of 0 (`GetMoveOptions` of the readied budget), "Mover agora" is dashed with "Você está Agarrado: o seu deslocamento é 0. Dá para deixar passar." (857-09 card 6).

The cast sheet in release mode (`cast-sheet`, 857-02 and 857-03):
- Title "Soltar Bola de Fogo", subtitle "Reação · a ação preparada de Pensantus", the alarm icon, and the spell's "?".
- A band under the header (`.rel` on the boards): "Preparada: “Quando o ogro passar pela porta”" and, under it, "Espaço de 3º nível · gasto ao preparar". It takes the place of the slot picker. No Metamagic, no ability picker, no other spell.
- The steps are the sheet's own pieces: `app-area-step`, `app-area-map` with its hint, legend, nudge buttons and "Centrar em…", then `app-area-list` and `app-area-notice`. Their look and words stay; the preview they read needs the server change below.
- Step 1 foot: "Confirmar local" (filled) and "Voltar" (outlined, back arrow), which closes the sheet and shows the window again. Step 2 foot: "Conjurar Bola de Fogo" (filled) and "Mudar o local".
- Where the dice may be typed, step 2 has the sheet's `app-roll-picker` in place of "Conjurar Bola de Fogo": "Conjurar e rolar no app" with the link "Digitar o resultado", or the field "Role 8d6 para Bola de Fogo: some os dados" with "Confirmar 27" (857-09 card 12).
- The result step is `app-cast-result`. Its button is "Voltar à sessão": off the caster's turn, "Voltar à sua vez" is false (857-09 card 13).
- A quiet line above the buttons: "A mesa espera você: até 2 min."
- The call is `CastSpell` off the caster's turn, which the server takes as the release while the READIED window is open (`findReadied`). `AnswerReaction` is not used for a placed spell.
- A wall uses its own pieces (`app-wall-controls`, the strip of two steps, "Próximo: quem está nela" beside "Voltar"), with the same band (857-09 card 3). A direction uses "Para onde?" and "Confirmar direção", with no range to check (857-09 card 4).

The Mover page off turn (`move-page`, 857-06):
- "Mover Tor", the same band ("Ação preparada: “…”" over "Fora da sua vez · vez do Ogro · gasta a sua ■ reação"), the lead with the readied budget ("Restam 9,0 m de 9,0 m"), the map, the legend, the zoom and the nudge buttons.
- The cost card says "Mover 3,0 m" and "É o movimento preparado: gasta a ■ Reação e não tira nada do deslocamento da sua vez. Vale um movimento só."
- "Mover para cá" (filled) calls `MoveCombatant`; one move is the use and the page goes back to the session. "Voltar" takes the place of "Cancelar" and shows the window again. No "Andar | Saltar" segment and no Disengage offer: the server's budget is a walk.

The master (857-04, 857-07, 857-08):
- The queue row's second button is live: "Soltar por Pensantus" and "Mover por Tor". The line "Mover é pelo mapa: mova Tor com a janela aberta." goes. "Deixar passar" stays first. In the 368 px column the wait line and the two buttons take the row's full width, under the name and the description, so neither wraps.
- "Soltar por Pensantus" opens the two-column layout of `MasterAreaCast` (the map at the left, "Quem está na área (você vê todos)" at the right) with the band, "Voltar", "Mudar o local" and "Conjurar Bola de Fogo". The master's list has the hidden Sombra and the switch "Revelar as escondidas atingidas", as when he casts for an NPC.
- "Mover por Tor" opens the Mover page with the band "Ação preparada de Tor: … você responde por Tor". The master's map shows the hidden token. Dragging Tor's token on the combat map while the window is open stays the use.
- The foot of both says "Pensantus ainda pode responder no próprio aparelho: vale a primeira resposta."
- The player gets no line while the master answers. When the master finishes, the player's window closes with "A janela fechou e a sua reação foi usada.", and the log line ends with "(pelo mestre)", as `combat-log.ts` writes for what the master does for a player's character.

The wait:
- In the window, before the map: 30 s.
- The first time the reactor's player enters the map (the cast sheet's map step or the Mover page), the 30 s give way to one wait of 2 minutes. It counts once from that first entry. "Voltar" does not bring the 30 s back and does not restart the 2 minutes; entering the map again does not restart them either.
- The player reads "A mesa espera você: até 2 min." on the map and the running count in the window ("Se você não responder, passa sozinho em 1 min 12 s.").
- The master's row reads "No mapa · passa sozinho em 1 min 48 s" while the player is on the map, and "Esperando Pensantus · passa sozinho em 1 min 12 s" once the player is back. The band chip says the same in one line. Both of the master's buttons are live the whole time.
- Another player reads "Esperando a reação de Pensantus" in every moment: no clock, no "no mapa".

States (857-09):
- The window went away while the player was on the map. The map goes away and the only button is "Voltar à sessão".
  - The readied row still waits (the master undid the trigger): "O mestre desfez o gatilho: a Bola de Fogo continua preparada."
  - The row no longer waits: "A janela fechou. A Bola de Fogo não saiu; a magia preparada e o espaço dela se perderam.", or `readiedGoneText`'s sentences for the reaction used, Caído and the broken concentration, and "Passou sozinho: você não respondeu em 2 min."
- Out of range, a wall in the way and nothing chosen are the cast sheet's own notices, with the buttons stacked as in 857-02. With no point that works, "Voltar" and then "Deixar passar".
- A second device of the same player shows the window with "Você abriu o mapa em outro aparelho. Passa sozinho em 1 min 48 s." Both may answer; the first answer wins.
- A refusal spends nothing and leaves the window open. Every error notice has its icon and a bold lead.
- Another player reads the wait above, and then the public cast line. Nothing says a spell, a level or a point before the release (RN-10). A hidden readier reads "Esperando o mestre".
- The Ready sheet offers "Mover" and the placed spells again: `MOVE_PENDING_TEXT` and `SPELL_PENDING_TEXT` go, and `readiedPrompt` gets `releasable` for a move. A combat without a map still refuses a readied move.

## Row 2: the rituals of the book (587-01 to 587-06)

The sheet (`character-sheet/combat-column`, 587-01):
- A single-class panel is titled "Magias de bruxo".
- The book is a block at the end of that panel: the book icon, "Livro de Segredos Antigos", the count, and one line: "Só como ritual, na sessão: o tempo da magia mais 10 minutos, sem espaço. Mirela guarda até o 3º nível."
- The book's rituals are listed in this block only. They leave the level groups of the panel, so no ritual shows twice.
- One row per ritual: the name, then "1º nível · Adivinhação · da invocação" or "… · copiado", the "?" and, on a copied one, the "⋮" with "Tirar do livro".
- Under the rows: "Adicionar ritual" (outlined, full width) and "Copiar leva 2 horas e custa 50 po por nível da magia: combine com o mestre."
- The rituals are cast in the session, in "Conjurar fora do combate", where the only way is "Como ritual" (587-06 card 4). The sheet has no cast button.

Who changes the book:
- The two rituals of the invocation have no "Tirar do livro", for the owner or the master. They change where the invocation's own choice is edited, and they leave with the invocation.
- A copied ritual is added or removed by the owner or the master, on a locked sheet too, as equipping is. The master is who says a ritual was found, so he adds as well (587-05).
- The level cap holds for both. The time and the gold are shown and never deducted.
- The list offers only spells switched on at the table, to the owner and to the master.

"Adicionar ritual" (587-02, 587-03):
- A sheet with a close button: the cost notice ("2 horas e 50 po por nível da magia: combine com o mestre. O app não cobra nada."), a search field, and the rituals of every class grouped by level. Each group's header carries its cost ("2º nível · 4 horas e 100 po").
- A row is a radio row: the name, then "Ilusão · 1 ação · Bardo, Clérigo, Patrulheiro".
- A ritual already in the book stays in the list, dashed, with "Já está no livro." (Mensageiro Animal on 587-02). One above the limit stays too, with its reason (587-06 card 1). The levels above the limit come last.
- A spell the master switched off or archived is not in the list or the search, and no sentence counts what is missing (RN-23).
- "Continuar" opens the confirmation: "Adicionar Silêncio?", the spell's line, the warning "Copiar leva 4 horas e custa 100 po. … o app não desconta o ouro nem conta o tempo.", "Voltar" and "Adicionar ao livro" (filled).

Removing (587-04, 587-04b, 587-05):
- "Tirar do livro" asks in place of the row, "Voltar" first, the second button outlined in danger. The text says the ritual can be copied again and that the gold and the time do not change in the app.
- The master's sheet at 1280 has the same block, the same question and "Adicionar ritual", with "A lista traz só magias ligadas na mesa."

States (587-06):
- No invocation any more: the rows stay, muted, with "Não vale agora" (as 580-05), the reason on one line, and "Adicionar ritual" dashed. The session does not offer them.
- The table switched a spell off after it was added: the sheet keeps it. The owner's row does not change. The master's row has the tag "Desligada na mesa", and his question warns that it will not enter the list again.
- Refusals as sentences; one sentence for a switched-off, archived or unknown spell.

## Row 3: Palavras de Interrupção outside a combat (591-01 to 591-08)

One action, one name: "Teste de um NPC" on the button and as the sheet's title. The card is named by its roll ("Capitão bandido · Enganação").

Where it lives: a text button "Teste de um NPC" (voice icon) in the head of "Ações e rolagens" of the open scene, beside "Teste em grupo"; with no scene open, beside "Pedir teste em grupo"; and "Teste de Bartolo" in the "⋮" of a stage NPC, which opens the same sheet with that NPC chosen. No floating control.

The master's sheet (591-01 at 1280, 591-07 at 390):
- "Quem rola": radio rows from `ListNpcCheckPicker`, in three groups: "Em cena" (STAGE), "No mapa" (TOKEN) and "Do bestiário" (a search that ends in one `monster_key`). A row says "Os jogadores veem" or "Escondido dos jogadores" (`hidden`), and "Sem bloco de estatísticas: rola com +0" when `has_stat_block` is false.
- "Teste": one select of the creature's `checks`, the skills and the six abilities, each with its bonus.
- "Como rolar o d20": "Rolar no app" or "Digitar o d20", two equal segments, with the field "Número que saiu no d20 (1 a 20)" under them. The master rolls either way, whatever the table's dice mode.
- "Os bardos veem a criatura e ela os ouve": a checkbox, shown only when the map cannot measure the chosen creature against a bard. Its hint names the creature: "O Capitão bandido não tem token no mapa: é você quem diz. …". The rule has two conditions: the bard sees the creature, and the creature hears the bard.
  - It starts ticked for a creature the players see.
  - It starts unticked for a stage NPC marked "Escondido dos jogadores", with the line "…ninguém é perguntado. Se você marcar, os bardos leem “uma criatura”, sem o nome." (591-08 card 6).
  - With both tokens on the map there is no checkbox: a quiet line says the app measures the 18 m and the sight.
- One filled button that names the roll: "Rolar Enganação do Capitão bandido".

The master's card (591-02 at 1280, 591-06 at 390, 591-08 cards 2 to 5):
- It sits at the top of "Ações e rolagens". Waiting: the warning colours, the d20 box, "d20 (13) + 4 = 17", one row per bard asked in order, and "Continuar sem esperar" (`SkipNpcCheckAsks`), which closes every open question.
- Settled: the neutral colours, "17 − 4 = 13", each bard's row, and an "X".
- The tags of a row: "Esperando Brisa", "Na fila", "Cortou 4", "Deixou passar" and "Sem resposta" (a question closed with no answer).
- With nobody to ask, it comes settled at once with "Ninguém foi perguntado: nenhum bardo podia cortar." The card never says why a bard was left out.
- The settled card survives a reload: the last settled check of the open scene shows until the next check or the scene closes. The "X" hides it on that device only.
- The check is not drawn a second time in "Perguntas pendentes".

The bard (591-03, 591-04, 591-05):
- An alertdialog bottom sheet: "O Capitão bandido faz um teste" (or "Uma criatura faz um teste" when the offer's `roller_label` has no name), "Palavras de Interrupção · fora de combate", the question with the die size, the chips "Gasta 1 Inspiração de Bardo", "Você tem 3" and "Não gasta reação", and two quiet lines: what stays with the master, and that the master can go on without the answer.
- "Deixar passar" and "Cortar" (filled). At a table of physical dice, a field "Número que saiu no seu d8 (1 a 8)" and the button reads "Cortar com 4".
- The result: "Você cortou: −4", the die, "Gastou 1 Inspiração de Bardo", "Restam 2", and "Voltar à cena".
- There is no clock.

Where the boards follow the code:
- The app and the docs say "NPC" ("Rolar teste de um NPC" in the rules); the boards say "Teste de um NPC".
- The picker has three sources (MR-058: "not every living NPC").
- A creature with no stat block is not refused: it rolls with +0.
- The bard never reads "Sem efeito": `CuttingWordsAnswered` carries the die and the uses left only.
- On a phone the master's nav is "Cena", "Grupo" and "Conversa": the map is a fold of the scene (`live-session.html`), so 591-06 has no "Mapa" tab.

## Heights

Every phone screen is 390×844 and every desktop screen is 1280×800.
The tallest phone sheets are the cast sheet's two steps (857-02, 857-03), which fill the screen.
The state boards are not screens: 857-08 is 1090 px, 857-09 is 2850, 587-06 is 1340 and 591-08 is 1640.

Every new screen and state goes into `e2e/tests/a11y.spec.ts`.

Glossary (PT-BR / EN): Soltar no mapa / Release on the map; Mover agora / Move now; Rituais do livro / Rituals of the book; Adicionar ritual / Add a ritual; Copiado / Copied; Da invocação / From the invocation; Tirar do livro / Remove from the book; Teste de um NPC / An NPC's check; Cortar / Cut; Continuar sem esperar / Go on without waiting.

## Server changes each session builds

core (#857 part 2, #856):
- A wait mark on the READIED window. The reactor's player sets it on entering the map and clears it on "Voltar". The first set starts one wait of 120 s that no later set or clear restarts. The master's screen reads the mark (on the map or not) and the start of the wait, and passes the window when the 120 s end. The reactor's player reads the mark back, for a second device. It never reaches another player: their wait line stays "Esperando a reação de Pensantus".
- The area preview takes the held slot while the readier's own READIED window is open. It checks the caster's free slots, so a held spell whose last slot of that level went at Ready is refused: the Mago 7 of the boards has one 4th-level slot, and it went when Muralha de Fogo was readied.
- `docs/architecture/play/readying.md`: the sentence on the 30 s pass that can close a window whose player is choosing a square.
- Web only: "nível" for "círculo" in `core/combat/ready.ts`; `SPELL_NOTE`; `MOVE_PENDING_TEXT` and `SPELL_PENDING_TEXT` go.

maps (#587 slice 2; slice 1 has no proto and no web change):
- Markers on the sheet's spell rows: from the book; one of the invocation's two or a copied one; "not valid now" when the invocation is gone; and, for the master alone, "switched off at the table".
- The list of rituals a character may add: each with its level, school, casting time and classes, and the reason when it cannot be picked (already in the book, above the cap). Only spells switched on at the table, for the owner and for the master.
- An add call and a remove call, for the owner and the master, with the level cap for both; the invocation's two rituals are refused by remove.
- The lock exception: add and remove work on a locked sheet.

play-a (#591):
- A flag on each candidate of `ListNpcCheckPickerResponse`: the map can measure this creature against a bard. It decides whether the checkbox shows.
- The read of the last settled check of the open scene, for the master, until the next check or the scene closes (as `GetGroupCheck` answers the latest closed group check).
