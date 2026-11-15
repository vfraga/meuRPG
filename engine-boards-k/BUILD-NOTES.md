# Engine K boards: what a build session needs (from the designer)

Boards: `project/k<slice>-NN-*.dc.html`, a PNG each; `project/canvas.json` ("MeuRPG — Motor K: infusões do artífice"), seven rows: slices 1 to 6 (`k1` to `k6`) and the states (`k9-01` light, `k9-02` the new pieces in dark).
Tools: `python3 tools/build.py [names]` (engine-j's builder with this canvas's classes), `nice -n 19 node tools/render.js project/<file>.dc.html` (`ZOOM=2 CROP=x,y,w,h OUT=…` for a close look). Sources in `src/`; `_fhead`, `_fsheet-dim` and `_abil-faisca` are shared fragments.
Annotations, not UI: the grey numbered captions of the states boards, the dashed "Nota do quadro" boxes, the "Rolada até …" lines and the dashed "ph-box" boxes (a part that stays as it is today).
The decisions behind the boards are in `decisions.md`.

Read against main 6c64f3021. The class, its features and the 20 infusion options are the table's pack (SRD 5.1 has no artificer); board text is our own words.
Where a board extends a screen of main, the piece is main's, with its order, words and sizes; the notes say what each board adds.

## The cast

- **Faísca:** Artífice 6, Armeiro, Gnomo. Inteligência 18, CD de magia 15. Knows 6 infusions, 3 active at most. Gear: Brunea, Escudo, Martelo leve (equipped), Besta leve (carried).
  In slice 1 the six are Defesa Aprimorada, Arma Aprimorada, Disparo Recorrente, Arma Bumerangue (known at level 5) and Armadura da Resistência and Escudo de Repulsão (taken at level 6, `k1-09`). Escudo de Repulsão is the one the app only counts in slice 1.
- **Brasa:** Artífice 3, Artilheiro, Humano. Inteligência 16. Knows 4, 2 active at most. Armadura de couro batido, Besta de mão. The homunculus is "Pino".
- **Tor** (Guerreiro 5) receives items in slices 2 and 3. **Brisa** (Bardo 5) holds Faísca's three infusions of slice 4.
- Each row shows the infusions its slice builds, so Faísca's six differ between rows; inside a row they agree, and never pass her limit of 3 or her level.
- The app has no gender for a character or a player: "quem infunde", "o artífice" for the class, the character's name instead of a pronoun. "Nível", never "círculo". Item names and rarity are the app's own (`names_pt.json`, `magic-items.json`).

## One layout through the six slices

**The block "Infusões"** (`k1-01`, `k1-06`, `k1-08`, `k1-12`) is a panel of its own, on a sheet that has the feature "Infundir Item".
- Place: right after "Equipamento" and before "Características e traços": the fourth column from 1200 px, the block after Equipamento under it.
- Head: the tool icon, "Infusões", and at the right the pips and "Ativas: 2 de 3".
- One quiet line: "Pela regra, você infunde ao fim de um descanso longo. O app não confere." At the limit it is "No limite: infundir outra encerra uma ativa, à sua escolha."
- "Ativas": one row per active infusion: the item's kind icon, the infusion's name, then the bonus chip or the attunement pill ("Sintonizado", "Pede sintonização": the pill of a magic item row), "no Escudo" under it with the parameter ("· fogo"), and the "⋮" at the right. The holder is named only when it is another character ("no Arco longo · com Tor").
- "Conhecidas, fora de uso · N": a fold, open while a slot is free and closed at the limit. One row per known infusion not in use: the kind it accepts, the link icon when it asks for attunement (one legend line under the list: "Pede sintonização"), and "Infundir" (outlined, 96 px).
- One control per row, at the right edge: "⋮" on an active row, "Infundir" on a known one.
- The "⋮" menu: "Mudar de item" and "Encerrar infusão" (danger, with what the item goes back to). Ending asks in place of the row, "Voltar" first (`k9-01` card 11).
- The block has no lock line. The sheet's one lock sentence stays main's, in "Equipamento".

**"Infundir"** (`k1-02` to `k1-04`, `k1-11`, `k2-01`, `k2-03`, `k2-04`) is one sheet of two steps: a bottom sheet on a phone, a 480 px dialog from a tablet up, where the pair of buttons is `app-pair-foot` (outlined first, filled last, 176 px, right-aligned). The master's title ends with "· Faísca".
- Step 1, "O item", always in this order:
  1. the fold "O que a infusão faz";
  2. "De quem é o item": in slice 1 a read-only field with the artificer's name, of the select's height; from slice 2 the select, in the same place. On a +1 infusion in slice 1 the field's hint says "Para o item de outro personagem, o mestre marca o bônus na peça.";
  3. "Em qual item", the same label in every slice: radio rows, the ones the infusion accepts first, the others dashed with the reason;
  4. the parameter, when the infusion has one: the select "Resistência a qual dano".
- Step 2, "Confirmar": the summary box (the infusion and the item, "O item está com Faísca.", what it gives), the attunement, "Ativas: 2 de 3. Com esta, 3 de 3.", then the rule line and "Voltar" / "Infundir" (filled). The sheet is as tall as its content.
- Attunement follows the app's rules (`SetAttunement`: each rest opens one, three at once). "Sintonizar agora" shows only when the artificer's attunement of this rest is open and a place is free ("Usa a sintonização deste descanso. Sintonizados: 0 de 3."). Otherwise the line is "Sintonize num descanso; até lá não conta", and for another character's item "Tor sintoniza num descanso; até lá não conta".
- Over the limit, step 2 has the warning "Passa do limite de 3 ativas: uma termina." and the radio group "Qual infusão termina", the oldest first and chosen; the line under it follows the choice ("O Martelo leve volta a ser um martelo comum: sai o +1.") (`k1-04`).
- The app never checks that a long rest ended and the master approves nothing.

**The infused item on its own row** (`k1-05`, `k1-07`, `k1-08`, `k1-12`, `k2-02`, `k3-01`):
- The gear row is main's (`shared/gear-list`): the kind icon, the name (it ellipsizes first), the attunement chip and the bonus chip, the "⋮", the button; "Equipado" with "CA N", "Carregado" or "Carregados", folded from five.
- The infusion adds one line under the row, at the name's x, as the blocked-reason line sits: the tool icon and the infusion's name, then the parameter and, on another character's sheet, "· por Faísca". One line; it ellipsizes last. Its accessible name starts with "Infundido".
- The legend (`app-chip-legend`) gains "Infundido" for the tool icon.
- The numbers already include the infusion; the chip is the magic bonus chip, dashed when it does not count now (the piece is carried, or the attunement is missing).
- The same line sits under each line of "Modificadores de itens", under the attack's name in the sheet's "Ataques" table, in the note line of the combat's attack row (`app-action-row`), and after the chip in the attack sheet's one sub line ("Faísca · +4 · 1d4 + 1 concussão", the chip, the infusion).

## Slice 1: own gear, numbers (k1-01 to k1-12)

Decisions:
- The mark is the icon and the infusion's name, with "Infundido" in the legend and the accessible name.
- **An infusion the built slices do not compute is still infused** (`k1-11`, `k1-12`, `k9-01` card 15). It counts in "Ativas", marks its item and follows the attunement rules; the block row and the confirmation say "Só o texto: a mesa aplica." "Infundir" is never off for that reason. For a replicated item and for Servo Homúnculo before their slices, step 1 lists the object as one row already chosen, and the infusion is a text-only row of "Itens mágicos".
- Armadura da Resistência is a mark on the armor with its damage type: "O app marca a armadura com o tipo de dano. Reduzir o dano fica com a mesa."
- Infusing, moving and ending work on a locked sheet, as equipping does.
- A piece that is already magical is dashed ("Já é mágica (+1): a infusão pede um item comum."); an infused piece's manual "Bônus mágico" group is off with where the bonus comes from (`k9-01` cards 3 and 7).
- Two equal pieces are one row: "A ficha tem 2 cópias: a infusão vale para as duas." (`k9-01` card 3).
- An object that is not a piece of the sheet (a wand, boots, a helmet, a ring, a gem) is one more row of the same list, and the infusion becomes its own row in "Itens mágicos".
- The attack sheet is main's: step 1 advances on the pick and its foot has only "Cancelar" (`k1-07`).
- The master uses the same controls on Faísca's sheet (`k1-08`); his block has the note "Um +1 no item de outro personagem: marque o bônus na peça, pelo “⋮” da peça."
- Level-up, the picks (`k1-09`): the feature's own pick list of the "Escolhas" step ("Infusões Conhecidas (6º nível)", "Escolha 2 opções.", "Limite de 2 opções" on the rest once full). Of the table's 20 infusions, the 4 already known are left out, 14 can be picked, and the 2 that ask a higher level come last, off, with the server's reason.
- Level-up, the swap (`k1-10`): one more group, "Infusões", of main's one panel "Trocar uma escolha" (`app-swap-picker`), never a second panel. The class offers the group at every artificer level; where the table's rule offers swaps too, the other groups share the row of radios. The board adds "ativa · no Martelo leve" beside a pick that is in use, and the warning that confirming the level ends that infusion.
- `k1-08` is the top of the master's page in one 800 px screen, with Equipamento as a placeholder; `k1-12` is the owner's page scrolled to Equipamento, where the whole block ends inside the screen.

## Slice 2: another character's item (k2-01 to k2-04)

Decisions:
- "De quem é o item" lists the approved, living player characters of the campaign, by name only for a player; a locked sheet does not stop it.
- **What the artificer's player reads of the holder:** the name and kind of the weapons, the armor and the shield ("Arco longo · arma marcial"). Not whether a piece is equipped, no bonus, no mark, no charge, no lock. A refusal about the kind keeps its sentence ("Pede uma arma com munição."); every refusal that would tell something of the holder's sheet is "Não pode receber esta infusão." (`k2-01`, `k9-01` card 5).
- **The master** reads the class, the level, the lock and the real reasons ("Já infundido por Brasa: Arma Aprimorada.").
- The holder reads the infusion's line with "· por Faísca" on the piece and has no control to end or move it. There is no arrival or end notice: the line is the sign, and its absence that it ended (`k2-02`).
- The holder attunes by the normal process; until then the chip is dashed and the bonus does not count.
- When the holder leaves the campaign or the piece leaves the sheet, the infusion ends: the artificer's count drops and the row returns to "Conhecidas, fora de uso" (`k9-01` card 6). The holder's death ends nothing.
- The master ends from the artificer's sheet; on the holder's sheet he reads whose infusion it is.

## Slice 3: charges (k3-01 to k3-03)

Decisions:
- An infused item with charges has two rows: its gear row, and a row of "Itens mágicos" named by the infusion, with main's pill "Sintonizado", the pips, "3 de 4 cargas", "Volta 1d4 ao amanhecer", "No Escudo · por Faísca" and "Usar" (84 px).
- A reaction uses main's reaction sheet (`reaction-sheet`): "Você foi atingido", "Ogro · Clava grande · Rodada 2", the question that ends with what it spends, the cost as chips ("Reação", "Cargas: 3 de 4", "Por Faísca"), a note, the filled "Usar Escudo de Repulsão" over "Deixar passar" on a phone, and the 30 s line. The push needs no map step.
- The long-rest prompt lists the infused items with the others under "Amanhecer" and keeps main's sentence, "O app rola cada dado do amanhecer por você, também numa mesa de dados físicos."; one line reminds that the artificers may infuse at the end of the rest.
- At 0 charges "Usar" is off with "Sem cargas: volta 1d4 ao amanhecer." and the reaction is not offered (`k9-01` card 16). Ending an infusion loses the charges left.

## Slice 4: effects on the wearer (k4-01, k4-02)

Decisions:
- Every effect is a line of "Modificadores de itens" in the block's three places: the value or an icon, the piece, the effect at the right, and under it the infusion and "por Faísca". A line that does not count now is muted with its reason.
- A resistance also enters "Resistências". The Iniciativa box gets "vantagem" under the number and the Deslocamento box shows the total.
- **A save DC is written as the item's number, "CD 15"**, and the source line names the infusion and who infused it. The wearer reads a number that comes from another player's sheet.
- A granted action is a row of the combat's "Itens" tab, one row per action, in main's row: the name, the economy mark and word, a detail, the infusion's note, and "Usar". A reaction row has no button; its detail has the DC and the charges, and the app offers it when its trigger happens.
- The boots' teleport spends the bonus action and writes the log; the row says "O token, o mestre move."

## Slice 5: Replicar Item Mágico (k5-01, k5-02)

Decisions:
- **The item is fixed when the infusion is learned.** Ticking "Replicar Item Mágico I" in the level-up's pick list opens, on the same page, a pick list of one: "Item de Replicar Item Mágico I", "Escolha 1 item. Ele fica fixo enquanto Brasa conhecer esta infusão.", a search, the items of the lists the artificer's level reaches, each with the app's name, category and rarity. An item the app lacks is dashed with "Não está no app.", under the name the table wrote.
- At infusing time there is no list and no "De quem é o item": step 1 has the fold and one row already chosen, and the sheet's title is "Infundir Mochila de carga" with "Replicar Item Mágico I" in the sub line (`k9-01` card 15). The new item stays with who infuses.
- The result is a normal row of "Itens mágicos" with the tag "Replicado" and the infusion's line, with the item's own charges and attunement. Its "⋮" leads to "Encerrar infusão", never to "Remover item".
- The block's row reads "Replicar Item Mágico I" over "Mochila de carga, em Itens mágicos".

## Slice 6: Servo Homúnculo (k6-01, k6-02)

Decisions:
- The card is main's `creature-card`: the art, the name, "Servo Homúnculo · Miúdo · infusão de Brasa", CA, PV, Deslocamento, "Ver a ficha", and the text actions "Renomear", "Corrigir PV" for the master, and "Encerrar infusão" in the place of "Dispensar". No "⋮".
- The block's row reads "Servo Homúnculo" over "numa gema · Pino, em Criaturas" and has no control: the action lives on the card.
- In the order, Pino's row comes right after Brasa's, in main's row (number, token, name, one word). The turn band says "em seguida: Pino".
- Brasa's rail has Pino's card with one button, "Comandar Pino" (▲): "Gasta a sua ação bônus: na vez de Pino, Pino faz uma ação do próprio bloco. Sem comando, Pino só Esquiva; mover e reagir não pedem comando."
- Pino's own turn, commanded or not, and Pino at 0 hit points are `k9-01` card 17.

## States (k9-01, k9-02)

Seventeen cards, each tagged with its slice: nothing known (main's pending-choices banner), no active one and the limit, no item accepts and equal pieces, the attunement, another character's item as the player and the master read it, the holder or the piece gone, two artificers and the manual bonus, "Mudar de item", "Não vale agora", the class switched off, the end question, loading, errors and refusals, who sees what, the object that is not a piece and the text-only rows, the carried piece and 0 charges, Pino's turn.
No infusion has a switch of its own: it follows its class (card 10).
`k9-02` draws the new pieces in the dark theme; the limit warning in dark is `k1-04`.

## What the server holds

**The record, one per active infusion:** id, campaign, the artificer's character, the infusion (the option's key; a Replicar slot is its own key), the holder's character (the artificer in slice 1; the field exists from the start), the item (a gear key of the holder's sheet, or a named kind of object for what is not a piece), the parameter (a typed value), `since`, and a link to the item or creature it materialises.
- `since` orders "the oldest". "Mudar de item" resets it: the moved infusion is the newest.
- An add-only migration, with one live record per holder and item.
- Character copy, revive and delete, and the campaign package, carry or end the records.
- The active limit is the class's formula by artificer level against the count of live records; the pack's resource "itens infundidos" is only a number.

Slice 1 needs no later slice's server, and needs all of this:
- **Typed infusion data in table content:** the kind of piece it accepts, whether it asks attunement, the parameter list, the bonus by level, the charges, the replicable lists. Options are text plus menu effects today. Pack-only: no editor screen in slice 1, and the editor keeps the data when an entry is saved.
- **Reads:** on the artificer's sheet, the active records and the known infusions not in use, each with what it accepts and whether the app computes it or only counts it; the list of targets for (infusion, holder) with one reason per refused row.
- **Writes:** infuse (infusion, holder, item, parameter, attune now, and the record to end when over the limit; without it the oldest ends), move, end. The artificer's owner or the master; valid on a locked sheet.
- **Attunement:** the state of an infused item goes through `SetAttunement`, with an entry the call can name, and enters the "N de 3" count and the attunement rows. An infusion that asks attunement gives numbers only while attuned.
- **The manual bonus:** a manual bonus save on an infused key is refused, and a magical piece is refused as a target. A piece leaving the artificer's own sheet ends the record.
- **A non-catalog row in "Itens mágicos"** for an object that is not gear, and for a text-only infusion.
- **The derived sheet:** the gear piece, the item modifier and the attack each carry the infusion's name; the bonus is recomputed at read (it grows at artificer level 10); Disparo Recorrente exempts its weapon from the ammunition count; Foco Arcano Aprimorado needs a spell-attack bonus from an item.
- **Combat:** the infusion's name on the play-side attack option, and a refresh when an infusion ends in the middle of a combat.
- **Session event kinds** (rows in `session_event_kinds`) for infused, moved and ended, and the stream change that makes an open sheet read again.
- **Refusal reasons** in `proto/error-reasons.json`, with their Portuguese sentences: the six of `k9-01` card 13 and the neutral "Não pode receber esta infusão."
- **Level-up:** a swap group "Infusões" at every artificer level by the class's rule, a field for it in the level-up choices, and the preview that says an active one ends. A swap ends the live record of the infusion given up.
- A record whose infusion the sheet no longer knows stays, flagged "not valid now", gives no numbers, takes no slot, and ends when another infusion takes its item.
- **Leak rows** for the attack log: another player reads the result of an attack, never the infusion.

Slice 2 adds:
- The holder differs from the artificer. The holder's derived sheet reads the live records that name it; no call writes the holder's stored sheet.
- The narrow list of a holder's pieces (name and kind) and the neutral refusal for the artificer's player; the full list for the master.
- The record ends when the holder leaves the campaign or the piece leaves the sheet. No notice and no "seen" mark.
- End and move stay with the artificer's owner and the master; the holder is refused.
- Leak rows for the party, combat and log views.
- **privacy.md:** a player reads the name and kind of another character's weapons, armor and shield.

Slice 3 adds: the charges left on the record; the maximum and the dawn die from the infusion's own data, not from `effects/item_charges.json`; a use from the sheet; the reaction offers (Escudo de Repulsão, Arma Radiante, Afiador Mental, Armadura da Força Mágica); the dawn of the long rest over the records.

Slice 4 adds:
- The wearer's derivation reads the option's effects with the artificer's numbers (spell DC, artificer level). Each line carries its kind, its source record and whether it counts now.
- The "Itens" tab and item actions in a combat are new: `ActionTabId` is attacks, spells, class and other.
- **privacy.md:** the wearer reads the save DC of an infused item, a number from the artificer's sheet.

Slice 5 adds: the item's key stored with the known infusion when it is learned (a choice inside the level-up choice); the replicable lists by artificer level in the class's data, each entry marked when the app lacks it; the record materialises a magic item entry on the artificer's sheet, flagged as replicated and removed with the record.

Slice 6 adds: a summon from an option and the turn mode "after owner" for a table summon, both work to do; the creature's hit points from the artificer's Intelligence and level; the command that spends the owner's bonus action; Pino's death, which ends the record; a creature without "Dispensar".

## Where the code stands in the way

- **The warlock's invocation swap has no piece on main's web.** The server has it (`can_swap_invocation`, `swap_invocation_key`), and nothing under `web/src/app` reads it. The only swap piece is `pages/level-up/swap-picker` ("Trocar uma escolha"), which lives in the ability step and shows only where the table's rule offers groups. `k1-10` uses that panel; at a level without the ability step it has no home yet.
- A magic bonus is stored per gear key, for armor, shield and weapons, +1 to +3 (`ItemBonus`, `proto/meurpg/characters/v1/characters.proto`): equal pieces cannot be told apart, and nothing gives a spell-attack bonus to a focus.
- No call writes or narrowly reads another character's sheet (slice 2).
- The level-up has no choice inside a choice: the replicated item's pick list (`k5-01`) is new.
- Charges get their maximum only from `backend/internal/rules/srd51/effects/item_charges.json`, keyed by an SRD item (slice 3).
- "Sintonizados: N de 3" is fixed at three (`equipment-panel.html`); the artificer's larger limit at levels 10, 14 and 18 is not drawn.
- A table summon refuses the turn mode "after owner" (`backend/internal/characters/tablecontent.go`, `rules/overlay_summon.go`), and an option cannot carry a summon (slice 6).
- In the order, another player reads a creature as "da Brasa · Ileso": `ofOwner` (`core/combat/creature-names.ts`) picks "do" or "da" from the owner's name. `k6-02` draws main's words.
- The boots' teleport has no move on the map.
- The app has "Aljava eficiente" (`item:efficient-quiver`), the SRD's name of the quiver the table's list calls by another name: if the table agrees it is the same item, the app lacks four of the replicable items, not five.

## Open points

1. An ally's item: a player reads the name and kind of another character's weapons, armor and shield (decision 3).
2. The wearer reads the save DC of an infused item (decision 9).
3. A replicated item, and any infusion that is its own row, stays with who infuses: the app has no way to hand it to another character. Enough, or should slice 5 let the row be given?
4. The level-up swap at a level without the ability step: the panel "Trocar uma escolha" moves to the "Escolhas" step there (drawn in `k1-10`), or the ability step shows for it alone?

## Heights

Phone screens are 390×844 and desktop screens 1280×800, except `k3-03`, 860 px (the long-rest prompt, as 563-07).
Boards with a note under the screen: `k1-09` 1000, `k1-10` 1010, `k4-01` 1166, `k5-01` 1004 (the screen is the first 844 px).
The states boards are not screens: `k9-01` is 3212 px and `k9-02` 390.
On a phone the block is about 575 px with two active rows and four known ones open, and about 375 px at the limit with the fold closed.

Every new screen and state goes into `e2e/tests/a11y.spec.ts`.

Glossary (PT-BR / EN): Infusões / Infusions; Infundir / Infuse; Ativas / Active; Conhecidas, fora de uso / Known, not in use; Infundido / Infused; Mudar de item / Move to another item; Encerrar infusão / End the infusion; De quem é o item / Whose item it is; Só o texto / Text only; Replicado / Replicated; Comandar / Command.
