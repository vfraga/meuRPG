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
- **Equipamento** (`equipment-panel`): "Classe de armadura" with the server's number, then `gear-list`: "Equipado" and "Carregado" as `h3` groups, each piece a row with its icon, its name (a two-handed weapon adds "duas mãos"; the kind is read by screen readers) and an outlined 44 px "Equipar" or "Guardar". The button shows the armor class the server says the change gives ("Guardar CA 16") and its accessible name starts with exactly the visible words, then the piece ("Guardar CA 16 Escudo"); a two-handed weapon in the pack with the shield in hand shows the same ("Equipar CA 16", plus "A classe de armadura vai de 18 para 16" for a screen reader) and a ⚠ on its row. "Carregados (n)" folds from five pieces. A change says "Guardado: Espada longa. Classe de armadura 14." in a live region and puts the focus on the moved piece's button; a refusal is an alert, and when the sheet could not be read again it offers "Tentar de novo". The sheet is read again when the tab is shown again, so a change from another tab does not linger. A dead character's sheet has no buttons; a locked one keeps them (the gear is played). The free-text items, "Do antecedente" and the coins follow. From 1200 px the panel is in the fourth column under the player's notes (the notes stay the first block) and before "Características e traços", beside "Ataques"; under 1200 px it is the single-column sheet's block after the combat numbers, and in the document it follows the saves and skills.
- **Other speeds** (`combat-stats`): under the walking speed of the "Deslocamento" box, one line for each speed beyond walking ("Voo 3 m (10 pés)", "Natação", "Escalada", "Escavação") and "Respira na água" when the table gives it; the box has no extra line for a character with only a walking speed.
- **Resources** (`resource-counters`, `shared/count-pips`): inside a session the sheet shows "Recursos" right after the combat numbers, one read-only box per resource (name, "2 de 3", pips, and when it comes back), then "Espaços de magia" with a row per spell level and the pact slots as "Pacto · 2º nível".
  Outside a session the sheet shows the maximums only. Nothing in the block takes focus.
- **Session vitals** (`pages/live-session/player-vitals`): the HP box with a 2 px `accent` border and a bar (`aria-hidden`: the numbers say it all), the AC shield, temporary HP and hit dice, and the spell slots counted by the free ones ("1 livre de 4"). The states that are on (a stance, a form) are small pills under the name, each with the rounds left ("2 rodadas", nothing for one that lasts until it ends).
  In combat the block is compact. On the master's "Grupo" row the HP bar is 6 px and the words "Abaixo da metade" or "Inconsciente" appear when needed; a druid in a beast form carries a `mr-tag` with the beast and its pool.
  Hit dice are written by size everywhere ("5d10 e 1d6").
- **Creatures** (`creatures-panel`, `creature-card`, `creature-page`): a "Criaturas" panel after "Combate" and "Magias", only for whoever can have a creature; HP numbers reach the owner and the master only (RN-20), and for others the panel does not exist.
  Renaming, dismissing and fixing HP are questions in place (`creature-edit`), never dialogs; a dismissal is a warning `alertdialog` with focus on "Voltar" that states what it costs.
- **XP and level up:** `xp-block` shows XP; `level-up-banner` shows "Pode subir de nível"; the steps page is `pages/level-up`. "A classe mudou" (`changed-content`) lists what a master's change did to this character.

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
