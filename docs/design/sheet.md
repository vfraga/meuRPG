# Character sheet and editors

The sheet mimics the official D&D 5e sheet; the editors that fill it follow the same rules as every other form ([README](README.md#rules-every-screen-follows)).
The browser draws what the server sends and computes no rule: multiclass prerequisites, hit points, spell slots and "A classe mudou" appear as notices and sheet numbers, never as screen maths.

## Sheet

- **Pieces:** the ability medallion, the AC shield, number boxes, proficiency rows and header fields (`pages/character-sheet/ability-medallions`, `sheet-header`, `proficiency-column`, `combat-column`, `features-panel`, `story-panel`).
- **Order:** see [Principles](README.md#principles). One column under 768 px, two on a tablet, four from 1200 px.
- **Pending problems** become one notice under the header; reminders stay in "Características e traços" ([README](README.md#rules-every-screen-follows)).
- **Spell list** (`combat-column`): one `h3` per level ("Truques", "1º nível"); each spell is a 44 px button that opens its description, with `mr-tag` tags below it ("Ritual", "Concentração", "Reação", then the state: "Preparada" with the `success` border, or "No grimório" / "Conhecida" plain) and the 44 px "?" (`app-spell-help`) at the end.
  The state is a word, never colour alone.
- **Granted spells** (`combat-column`): a "Magias concedidas" panel after "Magias" (it exists without a spellcasting class), one row per spell like the spell list's, with the level as the first tag and how it is cast as the others ("À vontade", "Gasta um uso de <recurso>", "Também com espaço", "Só como ritual", "Conhecida"); under the row, the source and the numbers it gives ("Carisma · CD 14 · ataque +6"), or the instruction to pick the casting ability in the sheet's choices.
- **Equipamento** (`equipment-panel`): `gear-list` with "Equipado" and "Carregado" as `h3` groups.
  The "Equipado" head carries "CA N" on the right, the server's number; a screen reader hears "Classe de armadura N" in its place.
  Each piece is one row on one line while its name keeps 96 px.
  Under that, the marks and the chip drop together to a second line under the name, at the name's x; the "⋮" and the button stay at the right edge, on the middle of the whole row.
  A row has the kind icon, the name (it ellipsizes first), "×N" for equal copies and "duas mãos" (dropped when there is no room).
  A carried two-handed weapon that would leave the shield idle has a ⚠.
  The shield it idles has a dashed "Parado" tag.
  The weapon's marks are 22 px icon-only chips, each named by its word.
  A row ends with the magic bonus chip, the "⋮" and an outlined 44 px "Equipar" or "Guardar" of equal width in every row.
  The bonus chip is "+N" with a spark.
  It is solid ("Bônus mágico +1, já somado") when the bonus is added to the numbers now, dashed ("Bônus mágico +1, não conta agora") when it is not.
  The armor and shield rows show no armor class of their own; the "Equipado" head has it.
  The button says only its verb.
  In the combat's fold the cost goes under the verb when the list is under 290 px wide, so the name keeps its 96 px.
  The armor class the server says the change gives is in its accessible name, which starts with the verb and ends with the piece ("Guardar CA 16 Escudo").
  A two-handed weapon in the pack with the shield in hand has the ⚠ and a hidden ", Sacar desativa o escudo: CA 21 → 18" on its row.
  The combat's fold has no hidden text there: its own warning line says it.
  A row with a blocked reason under it is not one line by design.
  A legend line under the list names the marks present, the attunement chips and the bonus chip.
  The legend is the shared `app-chip-legend`, the same piece the combat's "Ataques" tab uses.
  "Carregados (n)" folds from five pieces.
  A change says "Guardado: Espada longa. Classe de armadura 14." in a live region and puts the focus on the moved piece's button.
  A refusal is an alert, and when the sheet could not be read again it offers "Tentar de novo".
  The sheet is read again when the tab is shown again, so a change from another tab does not linger.
  A dead character's sheet has no buttons; a locked one keeps them (the gear is played).
  The free-text items, "Do antecedente" and the coins follow.
  From 1200 px the panel is in the fourth column under the player's notes (the notes stay the first block) and before "Características e traços", beside "Ataques".
  Under 1200 px it is the single-column sheet's block after the combat numbers, and in the document it follows the saves and skills.
- **Marcas das armas** (`weapon-tags`, `weapon-marks-sheet`): a weapon's marks are a crescent "Prateada", a crystal "Adamantina" and a spark "Mágica".
  On its row in "Equipamento" they are icon-only chips named by their word.
  In the "Ataques" table, beside the attack's name in the combat's action rows and after the numbers in the attack sheet's header ("Tor · +6 · 1d8 + 3 cortante · ☾ Prateada") each keeps its icon and its word; a plain weapon has none.
  Every row the viewer may edit has a "⋮" (32 px wide, 44 px tall) before "Equipar" / "Guardar".
  The owner has it before the lock, the master always; never on a dead character, never for another player.
  On a weapon it is named "Marcas · <arma>" and opens a sheet with that title: a bottom sheet on a phone, a dialog from a tablet up.
  The sheet has "A arma é:" and one row for each mark: a checkbox in the accent colour, its icon, the word and what it beats.
  "Prateada" beats "nem prateado", "Adamantina" beats "nem adamantino", and "Mágica" beats the three: "não mágico", "nem prateado", "nem adamantino".
  Then comes the radio group "Bônus mágico" with "Nenhum", "+1", "+2" and "+3" as four equal 44 px segments.
  "Soma no ataque e no dano, e a arma conta como mágica." explains the bonus, and a note gives the reason when the bonus does not count now.
  A single "Salvar" saves marks and bonus in one request, over the whole sheet at the revision the page holds.
  "Salvar" is alone and full width on a phone; in the dialog it is beside "Cancelar", the same size.
  Equal copies of one weapon are one row, and the sheet says "A ficha tem 2 cópias: marcas e bônus valem para as duas." (for more than two, "valem para todas").
  While it saves the button is a muted "Salvando…" and the sheet cannot be dismissed.
  A refusal is an alert in the sheet, which stays open.
  A stale sheet says "A ficha mudou enquanto você editava. Nada foi salvo: feche e abra de novo." and the page reads the sheet again.
  A piece the stored sheet no longer lists is stale in the same way.
  The foot says who edits: "O dono só muda enquanto a ficha não trava. Depois, só o mestre."
  The master sees "Muda a ficha de Tor: depois da trava, só você muda marcas e bônus. Tor os lê na ficha e nos ataques."
  Saved, the panel says "Marcas e bônus salvos." and the focus returns to the weapon's "⋮".
  An equipped weapon's chip is its attack's (the sheet's mark or a table rule); a carried weapon's is the sheet's.
- **Modificadores de itens** (`combat-column`): an open block below the combat numbers, with one line for each piece that adds a magic bonus: "+N · name".
  The target is on the right: "na CA" or "no ataque e no dano".
  The name keeps 96 px beside it; when the target does not fit beside that, it drops under the name, at its x.
  A piece that does not count now is muted and gives its reason once under its line.
  Without entries there is no block.
  The reason of an idle shield is said once: in its line when it has a bonus, else in the note below the numbers (the equipment panel has no note of its own).
- **Bônus no ataque** (`combat-column`, `bonus-chip`): in the "Ataques" table a weapon with a counting magic bonus has the "+N" chip after its marks, from the server's `magic_bonus` of the attack.
  The chip takes the place of "Mágica" on that row; a bonus that does not count shows no chip there.
  A spell has none.
- **Bônus mágico das peças** (`piece-bonus-sheet`, `bonus-group`, `bonus-chip`): armor and shield have the same "⋮", named "Bônus · <peça>", which opens "Bônus · <peça>".
  The sheet has its own description line, with the character's name before it for the master.
  The line is "Soma na CA enquanto a armadura está equipada." or "Soma na CA com o escudo em uso.".
  Then come the radio group "Bônus mágico" and one "Salvar", with the same saving, refusal and stale-sheet behaviour as the weapon's sheet.
  While a save runs the group is inert; after a refusal the focus returns to the radio that had it.
  The bonus is 0 to 3.
  "Nenhum" removes the whole entry, including its attunement.
  Saved, the panel says "Bônus salvo."
  The bonus is added to the armor class or to the weapon's attack and damage only while the piece is equipped and the rules count it (the reason is in the sheet when they do not); otherwise its chip is dashed.
  On a locked sheet the owner has no "⋮" and the panel says "Travada: equipar e guardar continuam valendo; marcas, bônus e itens mágicos, só o mestre muda."
  When the sheet has an item with charges, the lock line says "Travada: usar cargas continua valendo, o resto, só o mestre muda."
  An item with charges is a row with its name, a "Usar" button (84 px wide, 44 px tall), pips, "N de M cargas" and a line for when they come back ("Volta 1d6 + 1 ao amanhecer", "Não volta: ...").
  "Usar" works on a locked sheet and is off at 0 charges, with the reason under the row.
  A rolled start that the master has not set shows "O mestre define as cargas iniciais (1d4 − 1)." and an off "Usar".
  A lost item has a dashed tag, "Destruída", or "Sem magia" with "Virou um bordão comum." or "Ficou só o +2 no ataque e no dano.".
  The last charge of an item that asks for a d20 leaves "Falta rolar o d20 da última carga." and a "Rolar" button on its row.
  "Usar" opens a bottom sheet on a phone and a dialog from a tablet up: "Restam N de M cargas", a stepper "Cargas a gastar" and "Gastar N cargas", which reads "Gastando..." while it waits and shows a refusal in words in place.
  The d20 of the last charge is an alert dialog, "A última carga se foi", that follows the table's dice: "Rolar no app", or "Face do d20" first with "Rolar no app" beside it when the player rolls their own dice.
  A typed 20 on an item that regains charges asks for the face of its regain die, "Face do d8".
  The result of a use or a roll ("Tirou 14. ...") is read in a live region under the list.
- **Munição** (`equipment-panel`): a section of "Equipamento" with one row per counted kind of ammunition and its pieces.
  "Recuperar a metade: 3 flechas" is off during a combat, with nothing spent and after it ran, each with its reason under it; after it ran the line says "Recuperou 3 flechas deste combate. O resto se perdeu.".
  "Adicionar munição" opens a sheet with the four SRD bundles (flechas, virotes, balas de funda, agulhas de zarabatana, 20, 20, 20 and 50).
  While the list is empty, a free-text item that names ammunition gets the hint "“Flechas (20)” parece munição: para o app contar, adicione em Munição." with an "Adicionar" button.
- **Other speeds** (`combat-stats`): under the walking speed of the "Deslocamento" box, one line for each speed beyond walking ("Voo 3 m (10 pés)", "Natação", "Escalada", "Escavação") and "Respira na água" when the table gives it; the box has no extra line for a character with only a walking speed.
- **Resources** (`resource-counters`, `shared/count-pips`): inside a session the sheet shows "Recursos" right after the combat numbers, one read-only box per resource (name, "2 de 3", pips, and when it comes back), then "Espaços de magia" with a row per spell level and the pact slots as "Pacto · 2º nível".
  Outside a session the sheet shows the maximums only. Nothing in the block takes focus.
- **Session vitals** (`pages/live-session/player-vitals`): the HP box with a 2 px `accent` border and a bar (`aria-hidden`: the numbers say it all), the AC shield, temporary HP and hit dice, and the spell slots counted by the free ones ("1 livre de 4"). The states that are on (a stance, a form) are small pills under the name, each with the rounds left ("2 rodadas", nothing for one that lasts until it ends).
  In combat the block is compact. On the master's "Grupo" row the HP bar is 6 px and the words "Abaixo da metade" or "Inconsciente" appear when needed; a druid in a beast form carries a `mr-tag` with the beast and its pool.
  Hit dice are written by size everywhere ("5d10 e 1d6").
- **Creatures** (`creatures-panel`, `creature-card`, `creature-page`): a "Criaturas" panel after "Combate" and "Magias", only for whoever can have a creature; HP numbers reach the owner and the master only (RN-20), and for others the panel does not exist.
  Renaming, dismissing and fixing HP are questions in place (`creature-edit`), never dialogs; a dismissal is a warning `alertdialog` with focus on "Voltar" that states what it costs.
- **The bonded steed (588-23 to 588-26)** (`creature-card`, `shared/creatures/steed-ask`, `creatures-panel/steed-away`, `core/creatures/steed-card.ts`): its card has the tag "Montaria vinculada" (link icon) and the subtitle "Cavalo de guerra · Grande · celestial".
  Its text actions are 44 px targets: "Renomear", the master's "Corrigir PV", "Dispensar" in danger, and "⋮" › "Libertar Estrela" in danger with "Acaba o vínculo: a próxima é outra montaria." under it at 13 px.
  Both questions are asked in place as an `alertdialog`, "Voltar" first and with the focus, the confirm marked `data-confirm`; both call `DismissCreature`, with DISMISS or RELEASE.
  During a session the card reads the combat when the owner's player opens a question, and the chip "● Gasta a sua ação" shows when the combat holds the steed; the master never spends an action.
  A refusal for the turn or the action closes the question and turns "Dispensar" off (muted, the block icon, the word "desligado") with "Só na vez de Lia: dispensar gasta a ação." or "Você já usou a ação desta vez." linked under the actions, until the page says the combat changed.
  A bonded steed that is not among the creatures is a dashed row from `SummonSpellOptions.bonded_steed`: the name, the tag "Fora", the form and type, "Convocar Montaria traz Estrela de volta.", and its own "⋮" › "Libertar Estrela".
  The master reads the summon options too, for that row only.
- **Convocar Montaria (588-21, 588-22)** (`shared/summon-sheet`, `steed-choice`, `steed-pick`, `core/creatures/steed.ts`): the summon sheet with three things of its own.
  On the first cast a form the caster cannot ride carries the warning line "Pequeno demais para você montar" (`creature-choice-list`'s `warning`, with an icon); the form stays pickable.
  Under the list, "Tipo do corcel" is three segments with the app's words (Celestial, Fada, Ínfero), none picked at first; the footer reads "Escolha a forma e escolha o tipo do corcel." until both are picked, and the button is "Convocar Cavalo de guerra".
  With a bonded steed there is no list and no type: one fixed row in the chosen row's frame (the name, the tag "Mesmo corcel", "Cavalo de guerra · celestial · CA 11 · 19 PV"), the line that says it comes back as it was and where to release it, and the button "Convocar Estrela".
  "Como funciona" (`app-fold`) holds the spell's notes and the least Intelligence, closed in a phone's bottom sheet and open in a dialog.
  The footer says "Gasta um espaço de 2º nível ou maior. Só durante uma sessão, fora de combate."; the two refusals read "Agora não há sessão aberta." and "Leva 10 minutos: não dá durante um combate.".
- **XP and level up:** `xp-block` shows XP; `level-up-banner` shows "Pode subir de nível"; the steps page is `pages/level-up`. "A classe mudou" (`changed-content`) lists what a master's change did to this character.
- **Level-up steps bar:** `pages/level-up/steps-bar` draws one segment per step on a phone. From 768 px it draws numbered circles with a line between them, 56 px long and down to 16 px as the row narrows, so the last step is always inside the page. Between 768 and 1023 px only the current step keeps its label on screen; the others keep theirs for screen readers, and the line under the title says the step.

## Editors

- **Steps:** tabs with the step name ([README](README.md#rules-every-screen-follows)); `editor-stepper` renders them.
- **Ability scores:** the method is chosen with `app-method-cards`; to place rolled results, each ability is a picker on desktop and a button that looks like the field on a phone (dashed and "Livre" when empty). A rolled value shows its dice (`dice-result`: the four dice, the lowest struck through; the `aria-label` says everything).
  "Escolher" with no result is text-sized, in `ink-muted`, because it is an instruction.
- **Classes:** one block per class (`class-block`); hit points, spell slots and prerequisites come from the server.
- **Table content** (`pages/content`): lists with "Da mesa" and "Arquivada" marks (`app-table-mark`); one page per entry; the effect menu (`app-effect-picker`; a state, a weapon rule and a pseudo-weapon have their fields there, "Só enquanto o estado" is a text field under the effects that can be gated, and a reach or a range is typed in metres) and the feature card (`app-feature-editor`); the 20-level grid (`app-level-grid`).
  A server refusal lands on the field: the summary is an alert on top ("Não foi possível salvar a magia. 3 campos precisam de ajuste. Nada foi salvo; o que você digitou continua aqui."), focus goes to the first field with an error, and what has no field stays on top.
  "Esta entrada mudou enquanto você editava" is a notice with "Recarregar".
- **Table rules** (`pages/table-rules`): the page is one form with "Salvar regras"; each switch is a real `role="switch"` with its word.
- **Options for players:** the "Opções para os jogadores" switches use the same group-row-switch pattern in every editor; a turned-off option never reaches a player.
- **Text bindings only:** SRD and master text is drawn with text bindings (`app-srd-text`, `app-rule-text`, `app-markdown`), never `innerHTML`.
