# Decisions on the batch 4 reviews, Designer B (W7-E, W7-X, W7-Z), 09/10 01:15

Binding for the boards' fix round and for the cloud sessions that build them. They win over the boards where they differ.

## W7-E (effects that last), from `/tmp/pm-review5/review-W7E.md`, checked against SRD 5.1

1. **Rules text, as the SRD says it:**
   - Dodge: attacks against you have disadvantage **if you can see the attacker**, you have advantage on Dexterity saves,
     and you lose it if incapacitated or your speed drops to 0.
   - All 15 conditions plus exhaustion: add Enfeitiçado, Surdo and Petrificado to the table, each with its effects.
   - Concentration save: DC 10 or half the damage, **whichever is higher**.
   - Haste's lethargy: the SRD says the target "can't move or take actions" until after its next turn. The app blocks
     movement and the action, and says so; it does not invent more.
   - Web: the 5 ft cube that fire touches burns away in 1 round, dealing 2d4 fire to a creature that starts its turn in
     the fire; draw the restrained creature, the Dexterity save on entering or starting a turn in the web, and escaping
     (an action, a Strength check against the spell's DC).
   - Hold Person targets a humanoid. The player can pick any creature they see; on a target that is not a humanoid the
     player reads "A magia não teve efeito." (what a master would say at the table) and only the master reads why.
2. **One sample cast that can exist:** one concentration per caster (Pensantus does not hold Hold Person and Haste at
   once), the same character data as README's cast in every frame, real SRD creatures only (no "Xamã goblin", no
   "Capitão Goblin": use Goblin and Hobgoblin, or Bugbear), "Restam 1 rodada".
3. **The turn clock:** frames in initiative order; what ends "at the start of X's turn" ends there (Esquivando at the
   start of Toren's turn); a duration in rounds ends at the start (or end, as the spell says) of the caster's turn N
   rounds later, and "Restam N rodadas" counts those; the numbers agree across frames.
4. **RN-10 / RN-20:**
   - A player never sees an NPC's DC: the save prompt names the ability ("Teste de resistência de Sabedoria"), the result
     says passou/falhou. The same for every surface (card, prompt, result, log).
   - When the master turns off "Os jogadores veem este efeito", nothing reveals it: the wait reads "Esperando o mestre"
     (never "Goblin 2 faz um teste"), an advantage it causes shows to players as "Outra fonte" without its name, an
     automatic critical shows as "Crítico" without the reason, its log lines and end are the master's only, and counts
     ("N efeitos terminaram") count only what the player can see. The free label shows to players only while the effect
     is visible.
5. **The server contract, complete:**
   - an end-of-turn save is a **reaction window** of PM-04's mechanism (kind `EFFECT_SAVE`, the same idempotency, closed
     reasons and waiting reason handling), not a second mechanism;
   - the RPCs for adding an effect, changing its duration, its visibility and label, ending it, and exhaustion up and down
     ("Baixar 1 nível": a long rest with food and drink lowers it by 1, SRD);
   - the events and their audiences; the new `AdvantageSource` kinds for conditions; the automatic failure of Strength
     and Dexterity saves (Paralyzed, Stunned, Unconscious, Petrified) and the automatic critical within 5 ft;
   - the order of operations at the start and at the end of a turn (expiries, then start-of-turn damage, then saves),
     idempotent under a double end-turn, an effect ended while its save is open, and concentration lost mid-save;
   - an effect can have several targets (Bless: up to three) under one concentration;
   - exhaustion level 4 halves the HP maximum and clamps the current HP; level 6 is death, with a danger-outline
     confirmation.
6. **Drawing:** the 320 px frames of every new card and the master's cards; "Adicionar efeito" and "Mudar a duração"
   dialogs; the paralysed player's own turn; automatic failure and the automatic critical; Restrained speed 0; Bless
   with physical dice; danger-outline confirmations for "Encerrar" on a concentration, exhaustion 6 and "Manter
   Paralisado"; focus on the switch and radios; no text under 14 px; no "sheet", "Desl." or mixed feet and metres
   (metres, as the app); the README "Lote 4 — Designer B" entry.

## W7-X (contests and special actions), from `/tmp/pm-review5/review-W7X.md`, checked against SRD 5.1

1. **Hiding and attacking (RN-20):** before the attack the player reads only "Você está escondida." (never per enemy, never
   who noticed). The server decides the mode when the target is chosen, and the roll shows like any advantage roll (the
   d20 pair, source "Atacante não visto"), as a master at the table would say "role com vantagem". **The attack gives
   away the position: hiding ends for every creature**, hit or miss (SRD "Unseen Attackers and Targets").
2. **Rules text:** Restrained gives disadvantage on **Dexterity saving throws**, not checks; a creature's grab with a
   fixed "escape DC" (most SRD monsters) is escaped with Athletics or Acrobatics against that DC, so the contest model has
   a kind and an `escape_dc`; the rogue's Cunning Action hides as a bonus action; passive Perception is 10 + the
   Perception bonus, ±5 for advantage or disadvantage, and a tie keeps the hider noticed (the active check must beat it,
   as in any contest the tie keeps the status quo; say so as the app's reading where the SRD is silent).
3. **Every direction of a contest:** a player grappling or shoving an NPC, an NPC grappling or shoving a player (the
   player picks Athletics or Acrobatics and rolls, app die or typed), and a player against a player (the defender
   chooses and rolls). A contest waits through PM-04's reaction window mechanism with its own kind (`CONTEST`) and its own
   waiting reason; players read "Esperando o mestre" or "Esperando <nome>" by the same rule as PM-04. No timeout (as
   PM-04).
4. **Sample data from the SRD only:** no "Capitão Goblin"; use Hobgoblin (passive Perception 10) or Bugbear; Brisa's
   numbers from the README cast (a rogue 5 is proficient in Stealth, with expertise if the cast says so).
5. **Surprise:** the master marks any creature, player characters included; the suggestion compares each hider's
   Stealth with each creature's passive Perception (not the best Stealth), and a party member who is not hiding is
   noticed. The bonus-action restriction on a surprised creature is the app's reading; say so.
6. **Help:** the check form names the task and lasts until the ally's next check for that task or the end of the helper's
   next turn in combat (outside combat, until the master clears it); the attack form is tied to the target and the 5 ft
   rule.
7. **Group check:** players see passou/falhou only when the master shows the DC (as the scene checks today); the master
   sees who has not answered and can roll for them or close the check; "pelo menos metade" counts the characters asked.
8. **Moving a grappled creature:** draw the map frame (the dragged token moves with the grappler, to the square behind
   it), the halved speed shown as "Deslocamento: 4,5 m (metade, arrastando Goblin 1)".
9. **The contract:** event kinds and the RN-10 classification of every new RPC (the leak test requires it), idempotency
   keys, spending the action and the attack in one transaction, the shove's caller and its blocked square (refused with
   the reason, the shove becomes "não sai do lugar"), the log line for a failed grapple.
10. **Drawing:** the 320 px frames of states 2, 3, 5, 6b, 7 and 8, the "vê claramente" control and the refusal, focus
    rings on player frames, no text under 14 px, one scenario per frame.
