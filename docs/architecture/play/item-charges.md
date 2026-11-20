# Item charges and ammunition

The charges of a magic item and the ammunition of a character are spent, rolled and recovered through `ChargesService` (`proto/meurpg/play/v1/charges.proto`), which `play.Service` implements.
What the sheet holds (`SheetMagicItem.charges_left`, `lost`, `last_roll_pending`, and `FullSheet.ammunition`) belongs to the `characters` module.
`play` reaches it through `play.ItemKeeper`, which `characters.Service` implements and `VitalsKeeper` embeds.
The code is `charges.go` and `charges_events.go`.
The rule is in [the product rules](../../product/rules.md).

## Who may call

- `SpendCharges`, `RollLastCharge` and `RecoverAmmunition` are for the character's player and the master.
- A dead character is refused with `CHARACTER_DEAD`.
- The calls work on a locked sheet and need an open game session, in a combat and out of one.
- `SetCharges` and `Dawn` are the master's and have no player.
- `SetAmmunition` needs no session and writes no event: the owner may while the sheet is open to editing, the master always.
- Spending charges is never an action of the turn.

## Spending

`SpendCharges` takes the charges in one transaction with the idempotency key.
It refuses with `CharacterBlocked` `ITEM_DESTROYED`, `ITEM_NO_CHARGES` (none left, or a rolled start the master has not set) and `CHARGES_NOT_ENOUGH`.
The last charge of an item that is lost at once destroys it and writes `item_destroyed`.
The last charge of an item that rolls a d20 sets `last_roll_pending` and waits for `RollLastCharge`.

## The last charge

`RollLastCharge` works only while `last_roll_pending` is true.
The d20 follows the table's dice mode ([Table rules](table-rules.md)): the app rolls it, or the player types the face of a real die.
A 1 loses the item as the catalog says, and a 20 on an item that regains charges gives back a die of them, rolled the same way.
A typed 20 without the regain die's face is refused with `ChargesBlocked` `REGAIN_FACE_NEEDED`.
Any other face only ends the wait.
Two calls with the same idempotency key give one roll: the second reads the first one's answer.
Two calls with different keys give one success: the one that gets the lock second finds no wait and is refused with `invalid_argument` "the item has no last charge to roll", with no blocked reason, also when the first roll destroyed the item.

## Spell scrolls

`Content.SpellScrollNumbers` looks a scroll up by its item key.
It gives the scroll's spell level, its saving throw DC and its attack bonus.
The numbers come from `effects/spell_scrolls.json`.
A key that is no variant of the Spell Scroll gives no answer.
`Content.ScrollSpellOf` says which spell a scroll gives.
It is the one filter of the sheet's read and of the reading, so they never differ.
A spell of another level, one that is off or archived and one that is gone give no spell.

### Reading a scroll

`ChargesService.ReadScroll` reads the scroll of an entry for its character.
The character's player or the master may call it, with an idempotency key.
The check is d20 + the spellcasting modifier against 10 + the spell's level.
A spell of a level the reader casts, a cantrip included, needs no check.
The d20 is the app's or typed per the table's dice rule.
The master may use either.
The answer is a `ScrollReading` with the outcome, the spell, the numbers and whether the entry is gone.

The read is refused, and nothing is spent, when:

- the entry is not a spell scroll;
- the scroll gives no spell (none on it, a spell of another level, one that is off or archived);
- the spell is not on the list of a class the reader casts from;
- the combat is running and it is not the reader's turn, or the economy of the spell's casting time is spent.

The refusal for a spell that is off never names it to a player (RN-23).
A combat in setup has no turn and no economy.
The master is bound by the turn and not by the economy.

A read that needs no check, or whose check passes, keeps the unit.
It writes a `scroll_read` session event, the pass of the cast.
It is no line of the combat log and no undoable kind, so it closes the undo chain.

A failed check spends the unit and writes `consumable_used` with the outcome `scroll_failed`.
In a running combat it also spends the action, the bonus action or the reaction the casting time costs.
The unit and the economy go in one transaction.
Out of a combat nothing of a turn is spent.
`consumable_used` is no undoable kind either, so a failure closes the undo chain as every use of a consumable does.

A retry of the key answers the stored event and spends nothing again.
A key used for another call is refused.

### The pass

`consumable.LatestPass` finds the pass in the session's latest events (`consumable.Event` carries the kind, the combat and the payload of each).
It is the reader's latest read, when that read passed.
It lapses when:

- the reader reads again, whatever the result;
- the reader's turn ends, or the combat ends, after a read in a combat;
- the read is older than the window of the latest events.

A pass read out of a combat has no turn to end and stands until the next read.

### The log line

The read of a failure is a `consumable_used` line with the outcome `SCROLL_FAILED`.
The master and the owner of the reader read the spell, the DC, the d20, the modifier and the total.
Another player reads that the scroll came apart, with no spell and no number.
A spell that is off is named to the owner by neither name nor key.
The master reads it still.
A hidden creature's line is read by no player.

## Undo

A charge cannot be given back by the master's undo.
`item_charge_spent`, `item_charges_restored` and `item_destroyed` are not undoable kinds, so each of them closes the undo chain, and the attack before it can no longer be undone ([Undo](undo.md)).
The master corrects the charges with `SetCharges`.

## The master's correction

`SetCharges` sets the charges left (0 to the maximum, which is also how a rolled start gets its count) and whether the item was lost.
It ends a wait for the d20, writes `item_charges_restored` (or `item_destroyed` when the item is lost) and no line of the combat log.

## The dawn

`Dawn` is the master's "Amanhecer": it gives the items of every character of the campaign their dawn.
A long rest with `TakeRestRequest.restore_dawn` does the same in the rest's own transaction, after the characters' rest and before the rest's event.
A short rest ignores the flag.
The numbers are the keeper's `RestoreDawn` (`characters/dawn.go`), which takes a `link.DawnRoll` and returns the `link.DawnItem`s that regained charges.
`dawn.go` in play rolls the dice with `Service.roller` and writes the events.
The dice are always the app's, also at a table of real dice (RN-18): the master rolls a dawn for the table, as the app rolls the NPCs' dice, and no field of the request types a face.
For each stored row of an entry on a sheet:

- A lost item, an item that waits for its last d20 and an item that never recharges are left as they are.
  An item waiting for its d20 keeps `last_roll_pending` and regains nothing until the roll is made.
- An item that regains charges at dawn gets all of them back or the catalog's dice, cut to the maximum, and an item at its maximum changes nothing and writes nothing.
- An item that waits for dawns after its last charge counts one when it has no charge left, is full on the last dawn of its wait (the figurines wait 7) and starts counting again from 0.
  An item with a charge left counts none, and a count left over is cleared.
- An item that rolls its start and has no row has no count yet, so it is skipped.

The keeper reads the campaign's rows (`ListCampaignItemCharges`), locks each character that has one in the order of their ids, like a spend does, and reads the rows again after the lock, so a spend and a dawn on one item take turns.
Each item that regained charges writes one `item_charges_restored` session event (`regained` outcome, no key, a plain session event and never a line of a combat's log).
The idempotency of a rest is the rest's key: a retry replays the stored `rest_taken` event and never restores again.
`Dawn` has its own key, stored with the `dawn_taken` event, which also lists the items that regained charges, so the retry answers with them as they stand and a dawn that only counted a day for the figurines is not counted twice.
A call that changes nothing still writes `dawn_taken`.
The events are the master's and the owner's, as the charges are (RN-10).
The call tells the streams that the sheets of the characters changed (`vitals_changed`, to the master and the owner), and nothing else.

## Ammunition

`SetAmmunition` sets the pieces of one kind, 0 to 9999.
A kind is counted once the character has a row for it, also at 0, and S2 has no way to remove a kind.
A kind with no row is not counted.
`RecoverAmmunition` takes back half, rounded down, of what each kind fired in the character's latest combat, once for each combat.
It refuses with `ChargesBlocked` `COMBAT_RUNNING` while that combat is not ended, and `ALREADY_RECOVERED` after it ran.
A combat that fired nothing is skipped, and the recovery goes to the newest earlier ended combat that fired some, among the latest ten.
A character that fired nothing in any of them is `invalid_argument`.
It writes the session event `ammunition_recovered`.

## Ammunition in combat

A ranged attack with a weapon that fires ammunition takes one piece in the attack's transaction ([Attacks and damage](attacks-and-damage.md#the-two-step-attack)).
A kind with no row spends nothing: the keeper's `SpendAmmunition` answers `counted` false, and the shot writes no `ammunition_spent` event, sets no `ammo` on the attack's event and so refunds nothing on undo.
The keeper's `SpendAmmunition` starts the count of a combat over (nothing fired, not recovered) when the combat is not the one it last counted.
With none left the attack is refused with `EncounterBlocked` `AMMUNITION_EMPTY`.
Each shot writes the session event `ammunition_spent` with the combat's round, which reads as the line `AMMUNITION_SPENT`.
The undo of the attack gives the piece back: the keeper's `RefundAmmunition` adds it to the stock and lowers the count of what the combat fired, in the undo's transaction, and the line leaves the log with the attack.
It locks the stock row like the shot does, so a refund and a shot at the same time take turns.
When the stock row counts another combat only the piece comes back, and a row left with nothing fired belongs to no combat again.

## Events

The events are `item_charge_spent`, `item_charges_restored`, `item_destroyed`, `dawn_taken`, `ammunition_spent`, `ammunition_recovered` and `consumable_used`.
In a combat that has the character, `SpendCharges` writes `COMBAT_LOG_KIND_ITEM_CHARGE_SPENT` and `RollLastCharge` writes `COMBAT_LOG_KIND_ITEM_LAST_CHARGE_ROLLED`.
An event of `ammunition_spent` reads as `COMBAT_LOG_KIND_AMMUNITION_SPENT`.
The web words the three lines in `core/combat/combat-log-item-use.ts` ("gastou a última carga da Varinha de teia", "rolou 1 no d20 da última carga: ...", "atirou com o Arco longo: 1 flecha, restam 3").
`SetCharges`, `RecoverAmmunition` and any call outside a combat write the session event only, and no line.

## Consumables

`UseConsumable` uses up one unit of a consumable of the sheet: a potion, a scroll or another item of `effects/consumables.json`.
One entry of `FullSheet.magic_items` is one unit, and the keeper's `RemoveConsumable` takes it off the sheet in the call's transaction.
The code is `consumables.go`, and the dice, the healing, the resistance and the payload are in `play/consumable`.
The character's player and the master may call it, a dead user or a dead receiver is refused with `CHARACTER_DEAD`, and it needs an open game session.
The receiver is the user, or `target_character_id` for a gift: the user's entry is the one spent, and the effect is the receiver's.
Out of a combat there is no turn and no adjacency.
A user at 0 hit points does not act and is refused with `COMBATANT_DOWN`, and it may still receive a potion.
A receiver that is an NPC, a reserved character or not of the campaign is `not_found`.
A retry of a key is answered to the character's player or the master only, before anything else is read.
The answer holds the receiver's vitals for the master and for the receiver's own player alone, and the hit points regained and the receiver's id for the giver.
In a combat (`consumables_combat.go`) the use follows the combat's rules, with the same refusals as any action, for a potion:
- The user's combatant must be on its turn (`mustActNow`), and `payConsumableAction` spends its action.
- The extra action of Haste does not pay for it: activating a potion is an action, not Use an Object, so a user whose action is used is refused with `ACTION_USED`.
- The master acts for a player's character under the same turn, action and 5 feet.
- An NPC is not a player's combatant (`wearerIn` finds player combatants only), so an NPC combatant of a running combat is not gated by the turn or the action: the master's own use of an NPC's potion is spent and logged as text with no effect and no line of the combat's log, and an NPC's potion given to a player's character in a running combat is refused with `TARGET_NOT_ADJACENT`.
- A gift reaches a player's character that fights in the combat within 5 feet (`giftReach`; a combat with no map has no distance), else `EncounterBlocked` `TARGET_NOT_ADJACENT`.
- A player's combatant is never hidden from its party, so no hidden creature can be a receiver.
- A user or a receiver that is the only one of the two in a running combat is refused with the same reason.
- A receiver that is dead (the master confirmed it, three failed death saves, or its character died) is refused with `TARGET_DEAD` whatever the item (`refuseTheDead`), so no potion raises it.
- A combat in setup has no turn: the unit is spent and no turn or action is, and nothing is gated.
- Every potion costs the turn and the action, whether or not `effects/potions.json` has an effect for it: a potion that is only text is gated, spent and logged as text, and pays the action.
- A potion is an item of the SRD category `potion` (`rules.Content.IsPotion`), except the SRD's oils, which `rules` names by key in one place (`oils`): applying an oil takes a minute or ten, so it is no action.
- An oil, a scroll and any other consumable is spent and logged as text on any turn with no gate, and the master judges the time of an oil.
- The heal is `healCombatant`, so temporary hit points, the dying state and the death saves follow the combat's rules, and a creature at 0 hit points gets up.
- Out of a combat the heal is `healCharacter`, and both write the request of `healRequest`, so a druid in a beast form heals the beast first.
- A resistance is a lasting effect of the receiver's combatant (`addEffects`, source kind `item`, owner audience) that lasts `rules.PotionResistanceRounds` rounds from the receiver's turn, and a second potion of the same kind ends the first.
- A resistance for a receiver that fights in a combat in setup is put on its combatant, never on the character, whoever uses the potion.
- The effect's card reads "Do item" and the potion's name (`EFFECT_SOURCE_KIND_ITEM`), not "Do mestre".
- The event is written with the combat's id when the user fights in it, so the combat's log and undo chain read it; the line is `COMBAT_LOG_KIND_CONSUMABLE_USED`.
An entry that is no longer on the sheet is `EncounterBlocked` `ITEM_GONE`, in a combat or out of it.
`ListConsumableTargets` is the read behind "Dar a…", and it offers only who the use accepts (`consumableTargets`).
For a potion in a running combat a user that fights gets the player's characters within 5 feet of its combatant, with the distance and the state word (down or on its feet) that the order of the combat gives every player.
For a potion in a running combat a user that does not fight gets the characters that do not fight either, with distance 0 and no state.
For any other consumable, a combat in setup or no combat, the list is the other characters of the party that are not dead, with distance 0 and no state.
The list follows the state word a player already reads (`stateWordFor` in `combat_view.go`, and Death saves in [Rules](../../product/rules.md)): `receiverStanding` lists a creature with three failed death saves that the master has not confirmed as down for a player.
The master's list leaves that creature out, and a death the master confirmed (the combatant defeated, or its character marked dead) is on no list.
The use refuses such a receiver with `TARGET_DEAD` before anything is spent.
A failed read of the open session is an error of the call, and only the absence of an open session is a campaign with no combat.
Outside a running combat the list carries no state, so it tells a player no hit points of another character.
An entry that is no consumable is `invalid_argument` (the keeper's `ConsumableItem`), and an entry that is gone is `ITEM_GONE`.
It has the same authorization as the use, and the master reads the list of the character it acts for.
`rules.Content.IsPotion` says an item is a potion by its SRD category, the oils apart, which is what the combat's gate and the list of receivers read.
`rules.Content.PotionEffect` says what a potion does (`effects/potions.json`).
A healing potion rolls its dice and heals the receiver up to the maximum, and the response says the hit points really regained.
The dice follow the table's dice mode like the d20 of the last charge: the app rolls them, or `typed_faces` holds one face for each die, each a face of the die.
A resistance potion puts a record of `character_effects` on the receiver (source kind `item`) with the damage type and 3600 seconds of game time, the master's and the owner's to read.
The record counts down with `AdvanceGameTime` like any effect outside a combat, and a second potion of the same kind starts the hour again.
Any other consumable, and a potion that is only text, only spends the unit, and the response returns the item's text (outcome `TEXT_ONLY`).
The same holds for an NPC's own potion: an NPC has no hit points and no effects outside a combat, so the use is spent and logged as text, the master tells what it did, and an NPC is never refused as down.
`consumable_used` is written with the key of the call, so a retry returns the first answer and spends nothing again.
Two calls with different keys on one entry give one use: the second finds no entry and is refused with `ITEM_GONE`.
`consumable_used` is not an undoable kind, so it closes the undo chain ([Undo](undo.md)).
The sheet's `effect_pt` says what a potion does ("Cura 2d4 + 2"), and `spell_key` and `spell_name_pt` hold the spell of a scroll that casts its spell and are empty for every other item.

## Privacy

Charges, ammunition and these lines are the master's and the owning player's alone (RN-10).
No other player reads the line or the event (secret), and the stream tells `vitals_changed` to the master and the owner only.
The use of a consumable is read more widely:
- The master, the user's owner and the receiver's owner read the numbers: the dice, the faces, the total, the hit points regained and the resistance a potion left.
- In a combat the line says to the whole table who used which item and on whom ("Tor bebeu uma Poção de cura", "Tor deu uma Poção de cura a Brisa"), without a number (`consumable.Line`).
- The stream tells the master and the owners `vitals_changed`, and tells the table `encounter_changed` and `combat_log_changed` in a combat, and an empty `spell_casts_changed` for any resistance, in a combat or out of one: hints with no content, as every lasting effect sends.
The leak tests are `internal/leaktest/itemuse_test.go` and `internal/leaktest/consumables_test.go`, which also hold the rows of `ListConsumableTargets` (a creature the master hid next to the user) and the streams a third player hears.

## Refusals

`ChargesBlocked` (`charges.proto`) carries `REGAIN_FACE_NEEDED` (with the die's size in `needed`), `COMBAT_RUNNING` and `ALREADY_RECOVERED`.
The web reads it in `core/play/charges-errors.ts`.

## The character sheet

`ChargesClient` (`core/play/charges-client.ts`) wraps the six calls and reads the table's dice (`GetCampaign`).
`charges-errors.ts` words each refusal in Portuguese, by the typed reason: the item's own refusals name the item, and a call that did not get through says nothing changed.
`core/characters/charges.ts` holds the pure parts: an item's state, its words, the four SRD kinds of ammunition, the recovery and the free-text hint.
`SheetCharges` (`pages/character-sheet/charges/sheet-charges.ts`) is built by `createSheetMagicItems` and reads the sheet again after each write.
`EquipmentPanel` draws the item rows with charges, the "Munição" section and the live regions of their results.
A row with `charges_max` 0 is drawn as before.
"Usar" and "Rolar" work on a locked sheet and for the master; "Adicionar munição" follows the right to edit.
"Usar" opens `UseChargesSheet`, which sends the amount under one idempotency key and keeps it across retries.
The last charge opens `LastChargeSheet`, an alert dialog that follows the table's dice (RN-18): the app rolls, or the player types "Face do d20", and a typed 20 may ask for the regain die's face.
Its body says what a 1 and a 20 do for that item, from `SheetMagicItem.recharge` (`lastChargeBody` in `recharge-text.ts`).
The close button and Esc close it and the backdrop does not; closing leaves the row asking "Rolar".
The use sheet and the d20 sheet do not restore the focus themselves: `SheetCharges.focusRow` puts it, with the focus ring, on the item's "Usar" or "Rolar" (or the row itself when the item is lost) once the sheet has been drawn again.
`AddAmmunitionSheet` sends the pieces to add, and `SheetCharges` reads the sheet again and sends the total, since `SetAmmunition` has no delta.
"Recuperar a metade" is off with nothing fired and after the recovery, and goes off during a combat when the server refuses with `COMBAT_RUNNING`.
The free-text hint shows only while the ammunition list is empty and the items text names an arrow, bolt, sling bullet or needle; the text is never converted.
An item row with `SheetMagicItem.recharge` says when its charges return ("Volta 1d6 + 1 ao amanhecer", "Não volta: ..."), the dice of a rolled start, and what a lost item became ("Virou um bordão comum.", "Ficou só o +2 no ataque e no dano.").
The use sheet repeats the recharge line with the most the item holds, and "Sem cargas" names when the item comes back.
A sheet with charges words its lock line as "Travada: usar cargas continua valendo, o resto, só o mestre muda."
The ammunition kinds (`AMMUNITION_KINDS`) also name the catalog weapons that fire them, so the attack row names its kind by the weapon's key.

### Consumables on the sheet

`ChargesClient.useConsumable` and `listConsumableTargets` wrap the two consumable calls, and a use sends one idempotency key for the request it carries.
`core/characters/consumables.ts` holds the pure parts: the rows that group identical consumables, the effect line, the dice of a healing line, the receivers, the turn block and the notice of a use.
The server says what an entry is and costs, and the web reads it instead of deciding.
`SheetMagicItem.consumable` (`SheetConsumable`) is on every entry that `UseConsumable` accepts and empty on every other item.
It carries the kind (potion, oil, scroll, other), whether a use costs the action in a running combat, and the healing dice (count, sides, bonus; zero when the use rolls nothing).
`rules.Content.ConsumableOf` decides it, and `UseConsumable`, `ListConsumableTargets` and the sheet's read all call it.
A save ignores the field and stores none.
`ListConsumableTargetsResponse.in_combat` says the user's character is a combatant of a running combat; the target states are set by the same test.
Only the character's player and the master read both.
An oil is told from a potion by the SRD key list `oils` in `rules`; table content holds no magic items, so a pack has no oil.
A pack oil would need a magic-item entry in table content with an oil category.
`EquipmentPanel` draws one row for every key, with "×2" beside the name and "2 unidades" in visually hidden text, and "Usar" sends the id of the first unit of the group.
The "Itens mágicos" section shows with a magic item, for who can manage them, and while the notice of a use is on screen.
The notice of the last unit therefore keeps its heading and the line "Nenhum item mágico.".
The notice of a use sits at the top of the section, as in the combat.
"Usar" works on a locked sheet, and the lock line names what the sheet holds: potions, oils, scrolls, other items and charges.
A sheet with potions and charges reads "Travada: usar poções e cargas continua valendo; o resto, só o mestre muda."
`ConsumableUse` (`shared/consumable-use/consumable-use.ts`) opens `UseConsumableSheet` with the character the dialog acts for.
The master's title reads "Usar por <personagem>".
It reads the character after a use and words the result as a notice.
A read of the character that a newer one overtakes answers with the newer read's result once it has landed (`NewestRead`), so the notice never counts from an old sheet.
`NewestRead` is keyed by the character it read, so a read of another character never satisfies or overtakes this one's.
A call and what follows it are separate steps: only the call's own failure may say that nothing was spent, and a failed re-read after an answer says the unit was used.
The sheet's magic items and the combat's "Itens" both build one, with the owner's items and what the page knows of the combat.
An entry without `consumable` is no consumable row, whatever its name or its effect line says.
The web keeps no list of oils, no category read and no parsing of `effect_pt` for dice: the verb, the icon, the noun, the Ação mark, the pre-block and the dice read `consumable`.
`inCombat` is always `true` in the combat's "Itens" tab, which exists only in a running combat, and `null` on the sheet.
The sheet learns the combat from `ListConsumableTargetsResponse.in_combat` and says nothing of the cost until that read answers.
A scroll or another item that is not given has no targets read, so the sheet uses what the page knows, and `costs_action` is false for it anyway.
`turnBlock` is `null` on the sheet; in the combat it is `turn` when the turn is not the character's and `action` when the Action is spent (`turnBlockOf`).
Only an item whose `consumable.costs_action` is true opens blocked.
A sheet blocked by the turn or the Action dashes both buttons and says it in the ban line, with a bold lead: "Só na sua vez." or "Ação já usada.".
It reads no list and sends nothing.
A refusal of the turn or of the Action that comes from the server blocks the open sheet the same way.
`UseConsumableSheet` opens on "Dar a…" outlined and "Beber" filled; an oil reads "Usar" and "Usar eu mesmo".
A scroll or another item that is no potion or oil has no "Dar a…": the sheet reads no list for it and says nothing of receivers.
"Dar a…" is dashed when nobody may receive the item or the list could not be read, and then it carries a block icon.
A blocked sheet keeps the open hand on its dashed "Dar a…".
The dashed button is described by the line that says why, or by the error of the read.
The line that comes after the read sits in a polite live region that is already on the page, so it is announced.
The list of receivers is a group of radio rows, and its buttons follow the combat: "Beber eu mesmo" and "Dar a <nome>" in a running combat, "Voltar" and "Dar a <nome>" everywhere else.
The sheet says what the action costs once: the subtitle reads "em combate, gasta a ação" only for an item that costs the action when the server says there is a combat.
In a combat the cost line under the list says only what the potion does to the receiver ("A poção age em Brisa."), with an info icon and no mark.
Out of a combat the first choice reads "Fora de combate: não gasta a vez." and, when someone may receive it, "“Dar a…” lista o grupo todo.".
Once a receiver is chosen it reads "Fora de combate, não gasta a vez." and then what the potion does.
An item that costs no action says "Não gasta a ação: o mestre decide o tempo que isso leva." and, in the list of receivers, "O óleo age em <nome>.".
`ConsumableNoticeCard` draws the notice in the sheet's live region and in the tab of the combat.
`itemWords` words an item by its kind: a potion is "uma Poção" and drunk, anything else (an oil, a scroll) is used.
"Beber" and "Dar a…" follow the table's dice (RN-18): the app rolls the dice of a healing potion, or the player types one face for each die.
After the dice toggle the focus goes to the first die, and back to the toggle when the fields close.
The receivers are the server's list.
In a running combat a potion lists the adjacent characters with their distance and how they stand (`stateWord`), and the sheet says "Dar a quem está ao seu lado".
Any other case lists the party's names with no distance, and the sheet says "Dar a alguém do grupo".
An empty list says "Ninguém a 1,5 m de <nome> para receber…" in a combat, with the name of the character the master acts for, and "Ninguém mais no grupo para receber…" out of one.
`consumableRefusal` (`core/play/charges-errors.ts`) words the refusals of a use by the typed reason, each with a bold lead ("Brisa morreu.", "Essa poção já foi usada.", "Longe demais.").
The refusals that spend nothing say so.
They are the receiver gone or dead, the character dead or no longer at the table, no open session and dice that do not fit the die.
A call that never left says so too, and "Tentar de novo" sends the same call under the same key.
`Unknown`, `Internal` and `DeadlineExceeded` may have spent the unit, so they say "Não sabemos se a unidade foi usada: confira a ficha."
`CHARACTER_DEAD` names the character that died, and `InvalidArgument` speaks of dice only when the player typed them.
The refusals of the turn, of the action and of an item already used say what blocks the use and not that nothing was spent.
An item already used leads with "Essa poção já foi usada." and says "A ficha foi atualizada." only when the page read the sheet again.
When that read fails or a newer one overtakes it, the sheet says "Não conseguimos reler a ficha." with "Tentar de novo".
It keeps both buttons off and says nothing of who may receive the item.
It offers the unit again only once a read lands (`Aftermath.unread`, `UseConsumableData.reread`).
Then it says "Essa poção já foi usada. A ficha foi atualizada.", whether the gone unit came from the use or from the list of receivers.
A give with no receiver chosen sends nothing, so it never becomes the user's own use.
The sentence of a lasting effect ("Quem bebe ganha o efeito “Resistência a fogo” por 1 hora") is the server's.
It comes from `SheetConsumable.effect_title_pt` and `effect_duration_text_pt`, from the same helper as the effect card.
The log line of a use reads `ConsumableUseLogLine.kind` for the verb and `effect_duration_text_pt` for the length of a resistance, and the web writes no rule text of its own.
The generic sentences of `combat-errors-limits.ts` for the same reasons stay for the other callers.
`consumableTargetsRefusal` words a failed read of the list, and never speaks of typed dice or of a receiver who left.
The page reads the character again after a use and after every failure but `Unavailable`, and the sheet follows the answer.
A dead or far receiver is dropped and the list is read again.
A unit that is gone is replaced by another of the same item under a new key.
The sheet closes with the page's notice when none is left.
The notice of a use counts the units from that read, and says nothing of the count when the read is not on screen.
A potion that heals nothing since the receiver is at full hit points is a neutral notice ("<nome> já estava com os PV cheios."), not a success.
A receiver at three failed death saves reads as down in the list of a player, and the server refuses it (`TARGET_DEAD`): the dialog says nothing was spent.
A resistance lands in "Seus efeitos" through the effect cards the server sends.

### Consumables in a combat

`CombatItems` (`pages/live-session/combat/combat-items.ts`) reads the magic items of the character on turn through `ChargesClient.sheetOf`.
That is the player's own, or the one the master acts for.
It groups the consumables into rows (`combatItemRows`).
It reads them when the turn is the character's, when the turn or the Action changes, and after a use.
A read that fails keeps what is shown.
`ActionGroups` draws the "Itens" tab after the class's tab and before "Outras", with the row count, and no tab without a consumable.
Each row has "×N" for a group, the mark of its cost (the Ação when `consumable.costs_action`, none for an oil or a scroll) and "Usar", which opens `UseConsumableSheet` with a combat running.
A spent Action (`actionUsed`, also after the first attack of Extra Attack) folds the rows that cost it to "Ação · Usada", and the oils stay open.
A folded row opens the sheet blocked: both buttons are dashed and the sheet says the Action is spent.
A character that cannot act (stunned, paralyzed and the like) folds them to "Ação · Indisponível".
The row of an item that costs the action is off, and the reason is said once for the group ("Atordoado: não pode agir.").
The hint of the tab says what the server does: a potion costs the action, and an oil or a scroll is spent and shows its text, with no turn and no action, and the master judges the time.
The notice of a use shows at the top of the tab, which stays while the notice is on screen even after the last unit is gone, and is forgotten when the turn passes.
A scroll is a plain "Usar" row that is spent as text.
`CONSUMABLE_USED` log lines (`combat-log-consumable.ts`) say who drank or gave which item.
The heal's dice and hit points come only to the master and the owners of the user and the receiver.
Any other player reads who and what.

## The master's panel

`PartyChargesPanel` (`pages/live-session/party-charges`) is "Cargas e munição" in the master's Grupo tab, above the rest card.
`PartyCharges` (`core/play/party-charges-state.ts`) reads the sheet of each character of the party through `ChargesClient.sheetOf` and derives the rows and the list of what a dawn touches.
"Corrigir" is `SetCharges` (with the "Destruída" switch) or `SetAmmunition`, and the sheets are read again after it.
"Amanhecer" in the panel head is `ChargesService.Dawn`, and its answer lists what each item regained.
The rest card asks "Amanhecer" in a long rest, on by default, and always sends `TakeRestRequest.restore_dawn` explicitly: the row's answer, true when no item waits, false for a short rest.
`recharge-text.ts` words the recharge line ("volta 1d6 + 1") and spaces the dice with the minus sign.
The panel reads the sheets when the party changes and after its own writes, so a charge a player spends shows after the next read.
"Corrigir" and the long rest's question read the sheets again before they show a number, so "eram N" and the dawn's list are fresh.
When that read fails the correction does not open, and the "Amanhecer" row stays on and says the items could not be read.
