# Decisions on the batch 4 reviews, Designer A (W7-M, W7-C, W7-I), 09/10 01:10

Binding for the boards' fix round and for the cloud sessions that build them. They win over the boards where they differ.
Sources: `/tmp/pm-review5/review-W7M.md`, `review-W7C.md`, `review-W7I.md`, checked against SRD 5.1.

## W7-M (monsters with their whole stat block)

1. **Who sees the resistance steps:** an NPC's resistances, vulnerabilities and immunities are the master's only
   (RN-20). When a player's character takes damage, the master and that player see the character's own steps.
2. **The player's frames say nothing about what the app hides:** remove the line listing what the player does not see
   (it tells them the creature has recharge and legendary actions). A player reads only what happens.
3. **Fix the numbers:** legendary actions left after Wing Attack (2) and Detect (1) is 0 of 3; the player takes the
   bite's whole damage after their own resistances (piercing plus fire); a skeleton sample that shows its bludgeoning
   vulnerability; one consistent sample per frame (no traits of three creatures on one stat block).
4. **Real SRD data only:** no pixie (not in SRD 5.1) and no "Raio Solar 1/dia"; use a real innate caster from
   `data/monsters.json` (the drow: Dancing Lights at will, Darkness and Faerie Fire 1/day) and the mage's full spell list
   from the data, all five levels.
5. **Immunities:** a creature immune to a condition or a damage type is never offered a save against it; the refusal
   reason is the master's only.
6. **Legendary actions:** only at the end of another creature's turn, one per offer, never while the creature is
   incapacitated; the Wing Attack card has its whole effect (the flight after it).
7. **The data says Fire Breath has no effect on a success; the SRD says half.** The server fixes it through
   `creature_corrections` (with the SRD source), never by parsing the description at runtime; the note says so, and the
   session checks every save action with damage for the same mistake.
8. Draw the 320 px frames of every new card. The app shell's header is not this board's to change.

## W7-C (casting outside combat)

1. **The sample cast must be able to cast its spells:** a Life cleric (Ilaria, Clériga 5, Domínio da Vida) for Bênção,
   Ajuda, Curar Ferimentos (with Discípulo da Vida in the healing line) and Detectar Magia as a ritual of a prepared spell;
   the wizard Pensantus (Mago 5) for Armadura Arcana and Alarme as a ritual from the spellbook. Names exactly as
   `names_pt.json`; the campaign name the same in every frame.
2. **Ritual time is computed:** the spell's casting time plus 10 minutes (Alarme: 1 minute + 10 = 11 minutes). Cite each
   class's "Ritual Casting" feature. The warlock's Book of Ancient Secrets is an SRD invocation: write "o app ainda não
   tem" with its official name.
3. **Longer casting times, as the SRD says:** the caster spends their action each turn and keeps concentration; if
   concentration breaks, the spell fails and no slot is spent. **Combat starting does not cancel the cast**: it goes on,
   using the caster's action on each of their turns. Outside combat there is no clock: the cast completes when the master
   confirms the time has passed ("Concluir conjuração" in the master's queue). Draw the failed cast with a non-ritual
   spell cast with a slot.
4. **Durations are game time, not the wall clock:** show "dura 8 horas", never "até 22:40". The master's long rest is 8
   hours of game time, so effects of 8 hours or less end there; that is how the app ends them, and the note says so.
5. **Targets follow the spell:** range (touch = within 1,5 m on a map, the master's call without one), the number of
   targets (Curar Ferimentos one: radio buttons; Bênção and Ajuda up to three), out-of-range targets disabled with the
   reason. **Never an NPC's HP state** ("Ferida"): a player sees an NPC only by name, and only the NPCs the master shows.
6. One concentration at a time, consistent across frames; "Encerrar" a concentration has a danger-outline confirmation.
7. Mage Armor: touch, a willing creature not wearing armour; the refusal for armour worn is drawn.
8. Draw Ajuda's cast, the master's "Conjurar como NPC" with its pickers, and the 320 px frames of every new piece; no
   text under 14 px; no "Ação usada" outside combat.

## W7-I (items on the sheet)

1. **Fix the numbers and names:** Fighter 4, Força 19, Espada longa +1: attack +7, damage 1d8+5; the Ioun Stone of
   Protection gives +1 AC only; "Cajado do arcano" (names_pt), and every attunement restriction label from
   `attunement:*` ("por um feiticeiro, bruxo ou mago").
2. **Attunement and identification happen in the master's short rest** (SRD: a short rest focused on the item), in the
   same flow as PM-07b's "Descanso curto": the player marks "Sintonizar no próximo descanso curto" (or "Encerrar a
   sintonia"), and the master's short-rest confirmation lists the pending attunements and identifications with the rest
   of what comes back. A cursed item cannot be released. At most 3 attuned items. One transactional path with the rest.
3. **Unidentified items:** the player receives only the look ("Uma espada com runas"), the quantity and whether it is
   equipped. Nothing else: no attunement flag, no category, no charges, no "Beber"/"Usar". It can be equipped, but **its
   effects do not apply until it is identified**; the master's sheet view shows the effect waiting. Identical unidentified
   items never stack. The master's "Identificar" (or the short rest) reveals it, with a log line. Log lines name an item
   as the actor sees it.
4. **Draw the Spell Scroll** per the SRD: a spell on the reader's class list (otherwise unreadable), the spell's normal
   casting time, an ability check with DC 10 + the spell's level when the spell is above the reader's highest slot level,
   the scroll kept when the casting is interrupted, consumed when cast.
5. **Potions:** the four Potion of Healing variants from the data; drinking is an action in combat; "Dar a alguém para
   beber" (SRD: administering a potion to another creature takes an action) is distinct from giving the item.
6. **Wands:** expending the last charge rolls a d20; on a 1 the wand is destroyed, as its text says.
7. **Armour:** donning or doffing armour is refused in combat (SRD: 1, 5 or 10 minutes); a shield takes an action.
8. **Ammunition:** recover half after the battle (a minute of searching); rounding down is the app's rule (the SRD does
   not say), and the note says so.
9. A coin-editing action for the master and the owner; "Transformar em itens" with a preview and a confirmation; every
   new piece's 320 px frame; empty, loading and error states; no text under 14 px; "um descanso curto"; one consistent
   sample cast; no "(SRD)" in player copy; labels such as "Item maravilhoso" go to `names_pt.json` in the server task.
