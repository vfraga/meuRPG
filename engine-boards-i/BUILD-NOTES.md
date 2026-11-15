# Engine I boards: what a build session needs (from the designer)

Boards: `project/<item>-NN-*.dc.html`, a PNG each; `project/canvas.json` ("MeuRPG — Motor I: montaria e poções"), three rows: #588 mounted combat (588-01 to 588-10), #588 Find Steed (588-21 to 588-26), #565 (565-01 to 565-07).
Tools: `python3 tools/build.py [names]` (engine-h's builder, with a `<mapsvg>{json}</mapsvg>` map in the fixed map colours, the pair token, the hidden seal, and the mount, order-box, creature-card, picker and dialog classes added), `node tools/render.js project/<file>.dc.html` (`ZOOM=2 CROP=x,y,w,h OUT=…` for a close look). Sources in `src/`.
Annotations, not UI: the grey numbered captions on the state boards, the dashed "Nota do quadro" boxes, the "Rolada até …" and "Rolada: … estão acima" lines and the "ph-box" dashed boxes (a part that stays as it is today).

Read against main 7a2fbcfe (#588 slices 1–2) and the fork's `feat/find-steed-588-s3` (f83a57a9, slice 3). #565 has no branch yet: its boards follow the coordinator's rulings.

## The cast and the timeline

- Tor: Guerreiro 5, Medium, 44 PV, 9,0 m. His gear is the same on every board: Cota de malha, Escudo and Machado de Lenhador equipped, Arco longo carried, 15 flechas counted. CA 18 with the shield, as canvas H.
- The Cavalo de guerra: an NPC of the party that the master marked "Adestrada" (Large, CA 11, 19 PV, 18,0 m). Tor rides it from the start of Rodada 4.
- Jabuti: halfling, Small, 7,5 m. The Mastim: an NPC ally (Medium, CA 12, 5 PV, 12,0 m) with no mark at the start of Rodada 4.
- Lia: Paladina 5, Medium, human, CA 18 (Cota de malha and Escudo), 44 PV, slots 1º ×4 and 2º ×2. Her steed is Estrela (Cavalo de guerra, celestial).
- Pensantus: Mago 3, Int +3, casts up to 2nd level. Brisa: Bardo 3, 24 PV, at 0 PV in #565.
- The map is the Estrada de Mirathel, in Sessão 6.

Rodada 4 runs in this order: Wight 19, Tor on the Cavalo de guerra 17, Zumbi 1 15, Brisa 13, Pensantus 12, Jabuti 11, Mastim 10, Zumbi 2 6.
- Before Jabuti's turn (588-02 to 588-08), Jabuti and the Mastim stand apart and each has its own row.
- On Jabuti's turn the master marks the Mastim "Adestrada" (588-09), and Jabuti mounts it (588-01).
- The states board's hidden and independent mount is the Wight's Esqueleto de cavalo de guerra (initiative 8), never the Mastim.
- 588-03 (Tor dismounts) and 588-07 (the horse falls) are other moments of the same round, drawn for their state.

## #588: mounted combat (588-01 to 588-10)

Move page (`move-page`):
- Not mounted, with `TurnOptions.mount.mounts` (588-01): a "Montaria" panel under the map, one row per `MountChoice`. Each row has the token, the name, "Ao lado · custa 3,8 m" from `cost_dft` with one decimal, and "Montar" (outlined, saddle icon) calling `MountCombatant`.
  - A refusal keeps the button where it is, dashed, with the reason under the name (588-10 card 1): TOO_SMALL "Pequeno demais para você: precisa ser maior.", TOO_FAR "A 4,5 m: chegue ao lado dele primeiro.", NO_MOVEMENT "Falta movimento: custa 3,8 m, restam 1,5 m." (`missing_dft`), MOVE_USED "Você já montou ou desmontou neste movimento.", TAKEN "Já leva Tor."
  - No `mount`, or no rows: no panel (588-10 card 3). A hidden creature is never a `MountChoice`: the server leaves it out.
- Mounted (`mount.mounted_on_id` set; 588-02, 588-03): a segmented "Cavalgar | Desmontar" takes the place of "Andar | Saltar". The rider has no move or jump of its own, and the mount's jump is left out.
  - Cavalgar: the page reads the mount's `GetMoveOptions` (the rider's player asks for the controlled mount). The lead and the cost card speak of the mount's counter ("Restam 12,0 m de 18,0 m ao Cavalo de guerra"). A neutral notice with the link icon says "Montado no Cavalo de guerra. O movimento é dele, e Tor vai junto no mesmo quadrado." "Mover para cá" stays the filled button.
  - Desmontar: the map paints `mount.landing` with the reach tint, the chosen square has a "4,5 m" chip, the card reads "Desmontar aqui · 4,5 m" from `dismount_cost_dft`, and the filled button is "Desmontar aqui" (`DismountCombatant` with col, row).
  - `dismount_refusal` dashes the segment, with the reason under it (588-10 card 2). `stopped_early` is a warning, "Algo bloqueia o caminho: Tor continua montado e nada foi gasto. Escolha outro quadrado." (588-10 card 4).
  - An independent mount: in the rider's own turn, Cavalgar is dashed with "Montaria independente: anda na vez dela." (588-10 card 10).

Tokens (`shared/combatant-token`, `shared/combat-map`):
- The pair is the mount's token with the rider's initial in a small disc on its top right, sitting on an ink saddle crescent. The turn ring goes around the pair, and a legend item "Montado" names it. `_token` in `tools/build.py` has the geometry in square units.
- A player's creature (Estrela) is the app's creature token: round with a dashed outline, named by the order's legend "Criatura de um jogador".
- A hidden combatant is dashed and always carries the eye-off seal (`tk__badge`), on the map and in the lists. On a pair the seal sits top left, clear of the rider.

Order (`order-list`, 588-04, 588-08, 588-09):
- A controlled pair is the existing `app-order-group` box. Its head reads "Montado · iniciativa 17" in place of "Turno conjunto".
- The mount row is indented, with the link icon and "Montaria de Tor · Ileso" for a player, and "Adestrada · leva Tor" for the master.
- An independent mount has no box: each keeps its own row, with the link line on both ("Montado no esqueleto", "Leva o Wight · independente").

Turn (588-04 desktop rail, 588-05 phone panel):
- A pair card in the accent frame. On desktop it reads "Tor e Cavalo de guerra · turno conjunto", with Tor's shapes on one line and the mount's line ("Ação", the walker with its metres). On the phone, the mount's line comes alone under Tor's shapes and "Mover".
- Then three equal outlined buttons with icons, "Disparada", "Desengajar", "Esquivar" (TakeAction with the mount's id), and one line: "Montaria adestrada: só estas três ações; a reação fica livre."
- There is one "Mover" (it moves the pair) and one "Encerrar turno". "Encerrar turno" ends both parts, one call each, as `mine.ts` does for a group. The card stands in for a separate creature tab.
- A down rider dashes the three buttons with "Caído: o mestre conduz o cavalo." (COMBATANT_DOWN, 588-10 card 5).

The rider's save (588-06, master 588-08):
- A sheet (a dialog from 768 px) from `Encounter.mount_save`, titled "Você cai da montaria?" over "Teste de resistência de Destreza".
- The DC is a player's only with the table's `show_save_dc` (campaigns.proto), so the default prompt, result and log carry none. With it, the title gains "· CD 10", the tags "CD 10" and the result "17 contra CD 10" (588-10 card 6). The master always reads it.
- The cause from `MountSaveCause`: "O Cavalo de guerra foi empurrado." or "Você foi derrubado."
- The tags "1d20 + 2" and "Destreza +2", then "Rolar no app" filled and "Digitar o resultado" (the concentration save's typed form, a 44 px field and button, with the second d20 for `mode`). A held roll goes through the existing prompt.
- The master's rail gets a warning card, "Esperando o teste de Tor", with "Rolar por Tor". "Próximo turno" is dashed, and right under it, in ink, "Espera o teste de Tor" (MOUNT_SAVE_PENDING).

The dismount window (588-07): REACTION_KIND_DISMOUNT gets a prompt for the rider's player, where today only the master answers it (`docs/architecture/play/movement.md` says the web has none; update it).
- The title is "A montaria caiu", over "■ Reação · desmontar · Rodada 4".
- The body is "O Cavalo de guerra caiu. Usar a reação para desmontar de pé?" and one line on each answer.
- The buttons are "Cair derrubado" (PASS) and "Desmontar de pé" (USE, filled).
- It has no auto-pass, because PASS throws the rider prone: keep it out of `OPTIONAL_KINDS`. The line "O mestre também pode responder." stays.

The master's mark (588-08 desktop, 588-09 phone): "Mais" › "Montaria…" opens a "Montaria" select in the opened row, as "Marcar cobertura" opens its picker.
- The options are "Nenhuma: não aceita cavaleiro.", "Adestrada: age logo depois de quem monta e só usa Disparada, Desengajar e Esquivar." and "Independente: mantém a própria vez e leva quem monta." The current one has the check and the only highlight. The call is SetCombatantMount.
- While the mount carries a rider, "Nenhuma" is disabled with "Só depois que Tor desmontar." (MOUNT_TAKEN).
- A player's character has no item.
- The column is 288 px wide, too narrow for three segments.

Also:
- An opportunity attack against the pair offers both targets, in the existing list (588-10 card 9).
- Copy fixes in `core/combat/combat-log-mount.ts`: "montou no Cavalo de guerra" and "desmontou do", with `article()`; and the master's line of a fall with no room says "parou no quadrado livre mais perto: não havia lugar ao lado do Cavalo de guerra", never the grid coordinates it writes today.
- RN-10 (588-10 card 7): Brisa sees the pair only with both rows visible, and never "Adestrada". A hidden mount leaves the Wight plain on her map and in her order. The save's window and d20 belong to Tor and the master; Brisa reads "Esperando Tor" and the line without numbers.

## #588: Find Steed (588-21 to 588-26)

`CastSummon` (play.proto on the branch) needs an open session (NO_OPEN_SESSION) and a caster that is not a combatant of a running combat (SUMMON_IN_COMBAT). So the cast button reads "Gasta um espaço de 2º nível ou maior. Só durante uma sessão, fora de combate." Its refusals: "Leva 10 minutos: não dá durante um combate." and "Agora não há sessão aberta." 588-23 and 588-25 are Lia's sheet during the session, out of combat.

Summon sheet (`shared/summon-sheet`):
- First cast (588-21): the slot collapsed to "Espaço de 2º nível · 2 livres · Mudar" and the five forms.
  - `SummonOption.too_small_for_you` adds a warning line, "Pequeno demais para você montar". The form stays pickable: the spell works, and only mounting is refused.
  - Then "Tipo do corcel", three segments with the app's words (`core/creatures/creature-types.ts`): Celestial, Fada, Ínfero. The pick goes in `SummonChoice.creature_type`, and the cast button stays off until it is picked.
  - "Como funciona" folds `notes` and "A Inteligência dele é no mínimo 6." (`min_int`). It is closed on the phone and open on desktop.
  - The cast button reads "Convocar Cavalo de guerra".
- With `bonded_steed` set (588-22, "Mesmo corcel"): one fixed row (the steed's name, the tag "Mesmo corcel", form, type and numbers), no list and no type choice.
  - It adds the line "Estrela volta como era… Para outra forma ou outro tipo, liberte Estrela antes, no bloco Criaturas."
  - The button is "Convocar Estrela".

Creature card (`creatures-panel/creature-card`, 588-23 to 588-25):
- A bonded steed gets the tag "Montaria vinculada" (link icon). Its subtitle carries the `creature_type` word for the owner and the master: "Cavalo de guerra · Grande · celestial".
- Its text actions are 44 px targets: "Renomear" in ink, "Dispensar" in danger, and a "⋮" with "Libertar Estrela" in danger ("Acaba o vínculo: a próxima é outra montaria.", 13 px). The master also has "Corrigir PV", in ink.
- A disabled text action is muted, with the block icon and the word "desligado", and its reason linked under it.
- Each question asks in place, with "Voltar" first and `data-confirm`. Both call `DismissCreature`, with `dismissal` DISMISS or RELEASE.
  - "Dispensar Estrela?" reads "some do mapa e da sua ficha, e você desce de pé… O vínculo fica". In a combat that holds the steed it adds the chip "● Gasta a sua ação". The rider lands on its feet with no window (588-24).
  - "Libertar Estrela?" reads "O vínculo acaba… a próxima Convocar Montaria escolhe de novo a forma e o tipo".
- NOT_YOUR_TURN and ACTION_USED read under the actions: "Só na vez de Lia: dispensar gasta a ação." and "Você já usou a ação desta vez." (588-26 card 4).
- The dismount window opens only when the steed drops to 0 PV under its rider; the steed leaves the combat once it is answered (588-26 card 2).
- A dismissed or vanished steed (588-26 card 1): `ListCharacterCreatures` does not list it. Draw a dashed "Fora" row from `SummonSpellOptions.bonded_steed`, with its "⋮ Libertar", whenever the bonded steed is not among the live creatures.
- `also_steed` (588-26 card 6): the cast sheet of a self-only spell gets the check "Estrela também" ("O corcel que você monta. Um efeito que dura cai nos dois."), and the result shows `SpellCast.steed_note`.
- RN-10 (card 8): another player reads "Estrela · Criatura de Lia" and the state word, never the type or the hit points. `sourcePhrase` (`core/creatures/creature-format.ts`) has no FIND_STEED case; the default "Criatura de Lia" is right for them.

## #565: potions and consumables (565-01 to 565-07)

The server sends each entry's line and a scroll's spell, and the web formats none of them: `SheetMagicItem.effect_pt` (14), `spell_key` (15) and `spell_name_pt` (16).

Sheet (`equipment-panel` magic rows, 565-01, neighbours of the #563 charge rows):
- A consumable row is the item icon (flask for a potion, page for a scroll), the name, and one line from `effect_pt`: "Cura 2d4 + 2", "Resistência a fogo por 1 hora", "Só mostra o texto". "Usar" sits at the right, 84 px wide. A family name keeps the row whole: "Poção de resistência", with the damage type on its line.
- Identical entries (same key) are one row with "×2". "Usar" sends the id of one entry of the group. The server keeps one entry per unit and does not group.
- "Usar" works on a locked sheet, as charges do. The lock line becomes "Travada: usar poções e cargas continua valendo; o resto, só o mestre muda."

The "Usar" sheet (a dialog from 768 px):
- The title is "Usar · Poção de cura", or "Usar por Tor" for the master (565-05). The subtitle is "Restam 2 · em combate, gasta a ação", then the effect line.
- The first state has two buttons, "Dar a…" outlined and "Beber" filled (565-05).
- "Dar a…" (565-02) shows the targets the server sends as radio rows ("Brisa · A 1,5 m · Caída"): adjacent visible creatures in combat, the party outside it. A hidden creature is never sent (565-07 card 3b). Then "O app rola 2d4 + 2 para Brisa." with "Digitar os dados", the cost line "● Gasta a sua ação. A poção age em Brisa.", "Dar a Brisa" filled and "Beber eu mesmo".
- With nobody adjacent, "Dar a…" is dashed with "Ninguém a 1,5 m de você para receber a poção." and no list (card 3a).
- The result (565-03) is a success notice in the turn ("Brisa recebeu 7 PV e acorda. 2d4 + 2: 3 + 2 + 2 = 7. Resta 1 Poção de cura.") and the log line "Tor deu uma Poção de cura a Brisa: 7 PV (…)".
- A resistance potion lands as an effect card in "Seus efeitos", with "por 1 hora · 58 min restantes" in the words of the Wild Shape clock (565-07 card 6). A potion with no automatic effect shows a neutral notice and the item's text (card 7).

Combat (565-03 phone, 565-04 desktop):
- An "Itens" tab after the class's tab, before "Outras", in the format of the other tabs: icon, label, count ("Ataques 2 · Magias 6 · Itens 5 · Outras 8"). It is the brief's "Usar item".
- Each row has the economy mark of its cost and the button: "Usar" (flask icon) for a potion, at an Ação; "Ler" (book icon) for a scroll, at its spell's casting time. A scroll's row is named by its spell ("Bola de Fogo · pergaminho de 3º nível"; "Passo Nebuloso", ▲ Bônus).
- A reaction scroll ("Escudo Arcano", ■ Reação) has no button: "O app oferece quando um ataque acertar você."
- A spent Ação folds the tab to "Ação · Usada", as other groups fold. The tab is absent with no consumables.

Scroll "Ler" (565-04, 565-06, 565-07 card 8):
- Reading takes the spell's casting time.
- A spell on the reader's class list, at a level the reader casts, goes straight to the cast sheet with "Sem espaço (usa o pergaminho)".
- A higher level opens "Ler · Bola de Fogo" first. It explains "Bola de Fogo é de 3º nível, e você conjura até o 2º. Antes, um teste de Inteligência CD 13.", with the spellcasting ability and DC 10 + the spell's level.
  - The DC is shown: it follows from the scroll's spell level, which the reader knows, so it is no master's secret (the coordinator's ruling).
  - It offers "Rolar no app" and "Digitar o d20".
  - A pass opens the cast sheet. A failure reads "o pergaminho se desfez sem efeito" and spends the spell's casting time; the spell only vanishes from the scroll.
- A spell off the class list is a dashed "Ler" with "Não é da lista de Mago."

RN-10 (565-07 card 9): the numbers of a heal reach the drinker's owner, the target's owner and the master. The other players read "Tor bebeu uma Poção de cura" and "Tor deu uma Poção de cura a Brisa", with no number.

Gaps the server needs:
- `effect_pt`, `spell_key` and `spell_name_pt` on `SheetMagicItem` (above).
- No HeldRollKind for the scroll's check: Lucky and Bardic Inspiration apply to ability checks.
- New error reasons with Portuguese sentences: the target is not adjacent or not seen, the item is gone, the spell is off the list, the check is owed.
- "Poção de escalada" is the boards' name; `names_pt.json` says "Poção de escalar" (a data change, with the effects revision).

## Heights

Every phone screen fits 390×844, and every desktop screen fits 1280×800, but one.
565-01 is 1060 px: the Equipamento of Tor holds eleven entries (four pieces of gear, the arrows and six magic items), within the 1.5 screens for eight or more.
The phone summon sheet (588-21) is the tallest sheet, at about 800 px of its 844.
The states boards are not screens: 588-10 is 1690 px, 588-26 is 1330 and 565-07 is 1640.

Every new screen and state goes into `e2e/tests/a11y.spec.ts`.

Glossary (PT-BR / EN): Montaria / Mount; Montado / Mounted; Cavalgar / Ride; Desmontar de pé / Land on your feet; Cair derrubado / Fall prone; Adestrada / Controlled; Independente / Independent; Montaria vinculada / Bonded steed; Mesmo corcel / Same steed; Dispensar / Dismiss; Libertar / Release; Tipo do corcel / Steed type; Itens / Items; Usar / Use; Beber / Drink; Dar a… / Give to…; Ler / Read; Pergaminho de magia / Spell scroll.
