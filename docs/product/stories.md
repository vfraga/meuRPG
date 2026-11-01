[Português (Brasil)](../pt-BR/produto/historias.md)

# Stories and acceptance criteria

The MVP has 45 stories: 43 stories plus 2 prerequisites (the invite, MR-002, which leads to MR-003, and the NPCs, MR-005, who are the combat enemies).
Ten more stories are planned for after the MVP.

A story is done when all its criteria pass.
Each criterion becomes an automated test: Playwright for what shows on screen, a Go test for the rule on the server (see [Testing](../testing.md)).
The new system only has to prove its own acceptance criteria.

How to read a story:

- the user-story sentence;
- the priority (MVP, MVP prerequisite or Later);
- the rules it follows (RN-xx, in [Business rules](rules.md)) and the modules it touches;
- the acceptance criteria (Given / when / then);
- "In the app" and "Related", short notes on where to read more.

Stories marked Later are proposals.
The data model is already prepared for MR-021 and MR-022 (see [Data model](../data/README.md)).
Where a behaviour came from a product answer, the [archive](../archive/decisions.md) has it.

## Index

| ID | Area | Priority |
| --- | --- | --- |
| [MR-001](#mr-001-create-a-campaign) | Campaign | MVP |
| [MR-003](#mr-003-join-through-the-invite) | Character | MVP |
| [MR-004](#mr-004-character-sheet-in-the-pdf-format) | Character | MVP |
| [MR-006](#mr-006-locked-sheet) | Character | MVP |
| [MR-008](#mr-008-points-of-interest) | Map | MVP |
| [MR-009](#mr-009-map-without-spoilers) | Map | MVP |
| [MR-011](#mr-011-start-the-session) | Session | MVP |
| [MR-012](#mr-012-follow-the-session) | Session | MVP |
| [MR-013](#mr-013-turn-order) | Combat | MVP |
| [MR-014](#mr-014-your-turn) | Combat | MVP |
| [MR-015](#mr-015-rp-scene-actions) | RP | MVP |
| [MR-016](#mr-016-award-xp) | Progression | MVP |
| [MR-018](#mr-018-campaign-document) | Support | MVP |
| [MR-019](#mr-019-image-gallery) | Support | MVP |
| [MR-024](#mr-024-approve-the-character-from-the-invite) | Character | MVP |
| [MR-028](#mr-028-show-an-image-to-the-players) | Session | MVP |
| [MR-025](#mr-025-register-table-content) | Rules | MVP |
| [MR-010](#mr-010-generate-dungeons) | Dungeon | MVP |
| [MR-029](#mr-029-scene-hooks-and-clues) | RP | MVP |
| [MR-030](#mr-030-player-notes) | RP | MVP |
| [MR-031](#mr-031-npcs-in-the-scene) | RP | MVP |
| [MR-032](#mr-032-combat-highlights) | Combat | MVP |
| [MR-033](#mr-033-print-the-map-with-the-grid) | Map | MVP |
| [MR-034](#mr-034-special-movement) | Combat | MVP |
| [MR-035](#mr-035-traps) | Map | MVP |
| [MR-036](#mr-036-fog-of-war-by-sight) | Map | MVP |
| [MR-037](#mr-037-creatures-of-the-character) | Combat | MVP |
| [MR-038](#mr-038-puzzles) | Session | MVP |
| [MR-039](#mr-039-ai-generated-images-for-dungeons-and-scenes) | Support | MVP |
| [MR-040](#mr-040-level-up-from-the-sheet) | Progression | MVP |
| [MR-041](#mr-041-treasure-and-xp-by-gold) | Map | MVP |
| [MR-042](#mr-042-bestiary) | Combat | MVP |
| [MR-043](#mr-043-generate-encounters) | Combat | MVP |
| [MR-044](#mr-044-generate-treasure) | Map | MVP |
| [MR-045](#mr-045-look-up-spells) | Rules | MVP |
| [MR-048](#mr-048-cast-spells-outside-combat) | Combat | MVP |
| [MR-050](#mr-050-campaign-package-export-and-import) | Campaign | MVP |
| [MR-051](#mr-051-the-scenes-conversation) | Session | MVP |
| [MR-052](#mr-052-the-free-dice-roller) | Session | MVP |
| [MR-049](#mr-049-reserved-characters-and-claim-links) | Character | MVP |
| [MR-053](#mr-053-the-master-acts-for-a-players-character) | Combat | MVP |
| [MR-054](#mr-054-use-a-creatures-action-from-its-turn) | Combat | MVP |
| [MR-055](#mr-055-answer-a-creatures-rider) | Combat | MVP |
| [MR-002](#mr-002-generate-an-invite) | Campaign | MVP (prerequisite) |
| [MR-005](#mr-005-create-npcs) | Character | MVP (prerequisite) |
| [MR-007](#mr-007-import-a-sheet-from-pdf) | Character | Later |
| [MR-017](#mr-017-level-up) | Progression | Later |
| [MR-020](#mr-020-look-up-the-rulebook) | Support | Later |
| [MR-021](#mr-021-copy-a-character) | Character | Later |
| [MR-022](#mr-022-reuse-npcs) | Character | Later |
| [MR-023](#mr-023-hand-over-or-share-the-campaign) | Campaign | Later |
| [MR-026](#mr-026-propose-a-new-race-or-class) | Rules | Later |
| [MR-027](#mr-027-read-the-rules-from-a-pdf) | Rules | Later |
| [MR-046](#mr-046-table-style-feature-by-feature) | Rules | Later |
| [MR-047](#mr-047-more-puzzles) | Session | Later |

## Priority: MVP

### MR-001: Create a campaign

**As a** master, **I want** to create a campaign that groups the sessions, characters and maps, **so that** each table stays organized on its own.

- Priority: MVP
- Rules: RN-05, RN-30
- Modules: campaigns

#### Acceptance criteria
- **Given** I am signed in, **when** I create the campaign "Mirathel", **then** I become its master **and** only its members see it in the list.
- **Given** I am already master of the maximum number of campaigns for an account (10 by default), **when** I create another, **then** nothing is created **and** the form shows a sentence with the maximum (RN-30).
- **Given** the server lets only some e-mails create campaigns, **when** an account with another e-mail tries to create one, **then** it is refused with the reason and the screen explains it; people who join by invite keep playing (RN-30).

#### In the app
- The list at `/campaigns` shows the role in each campaign ("Mestre" or "Jogador") and the "Criar campanha" form. An empty list explains how to create a campaign or join by invite.
- The screen does not know the maximum before the server refuses.
- See [Architecture](../architecture/campaigns.md#campaigns-and-authorization).

### MR-003: Join through the invite

**As a** player, **I want** to create my character through the invite link, **so that** it is linked to the campaign from the start.

- Priority: MVP
- Rules: RN-03
- Modules: characters, campaigns

#### Acceptance criteria
- **Given** a valid invite to "Mirathel", **when** the player opens the link and signs in with Google, **then** they become a player of the campaign and create a player-type character, which the master already sees in the campaign.
- **Given** an expired invite, **when** someone opens the link, **then** they see a clear message **and** nothing is created.
- **Given** a visitor who is not signed in, **when** they open the invite link and sign in, **then** they join in that one step, and the invite token appears in no address the browser requests.
- **Given** a person who is already a member, **when** they accept the invite again, **then** nothing changes.
- **Given** an invite with one use left, **when** two people accept it at once, **then** only one gets in.
- **Given** a player in the campaign, **when** they create their character, **then** it starts as a draft, and the master sees it in the campaign list with the player's name, class and race. The editor is the step-by-step editor of [MR-004](#mr-004-character-sheet-in-the-pdf-format).

#### In the app
- `/invite` reads the token from the address, erases it at once and offers "Entrar para aceitar o convite" to a signed-out visitor.

#### Related
- RN-03: a player creates a new character in this campaign only when the current one dies; the dead character stays in the system (see [Business rules](rules.md)).
- RN-17 describes the sign-in of a player without Google (not built yet; see [ADR-0009](../adr/0009-table-accounts-for-players-without-google.md)).
- When the invite requires approval (RN-15), the player joins as a pending member, creates the character, and it stays pending until the master approves or refuses it. See [MR-024](#mr-024-approve-the-character-from-the-invite).

### MR-004: Character sheet in the PDF format

**As a** player, **I want** to see my sheet in a format close to the official PDF, **so that** I find everything where I am used to.

- Priority: MVP
- Rules: —
- Modules: characters, rules

#### Acceptance criteria
- **Given** a complete character, **when** the player opens the sheet on a phone, **then** they see the sections of the official sheet (abilities, skills, combat, spells, equipment) **and** the calculated values, such as modifiers, armor class, hit points, spell save DC and spell attack, come ready from the server.
- **Given** the sheet's "Equipamento", **when** the player reads it, **then** it lists the armour, the shield, the weapons and the items, then the background's equipment as text ("Do antecedente: …", a table background's or what the player wrote for a custom "Outro" background), then the coins. A versatile weapon shows its two-handed damage under its damage ("Com duas mãos: 1d8+1").
- **Given** the step "Perícias", **when** the race, the subrace or the background already give a skill, **then** it shows checked and locked with where it comes from ("da raça", "do antecedente") and does not count as a pick. The player picks another skill instead of repeating a proficiency, as the SRD allows.
- **Given** a new character made above level 1 in a campaign that levels by XP from defeated enemies, **when** the editor opens, **then** the experience field starts at the XP of its level (6.500 at level 5) and follows the level until the player types in it. A milestone campaign keeps 0.
- **Given** the subclass field, **when** the player edits the class, **then** it offers "Nenhuma", says at which level the class picks it ("O Bárbaro escolhe a subclasse no nível 3.") and changes the subclass when the class changes.
- **Given** the step "Magias", **when** the player opens it, **then** the list shows only spells up to the highest spell level the current level allows, ordered by spell level and then name. A spell already chosen above the limit stays, marked "acima do nível", so it can be unchecked.
  - A class that starts casting later says so ("O Paladino conjura magias a partir do nível 2." at level 1).
  - "Truques" appears only for a class with cantrips, or when the sheet already has one.
  - Each list counts against the class's numbers ("3 de 4 truques escolhidos"). A preparing class (cleric, druid, paladin, wizard) with fewer prepared spells than it takes sees "Prepare até N", and the level-up's "Prepare mais N" counts down with the picks.
- **Given** the step "Habilidades" of a player creating a sheet, **when** the table allows several methods (RN-24, [MR-025](#mr-025-register-table-content)), **then** the player picks among them and the server rolls and keeps the 4d6.
- **Given** the free editor of the ability scores (what the master sees for NPCs, and what editing a sheet without a recorded method keeps), **when** the master opens "Como definir os valores", **then** there are three cards: "Digitar", "Rolar 4d6" and "Conjunto padrão".
  - "Rolar 4d6" rolls 4d6 six times in the browser, strikes through the lowest die of each, lists the results from highest to lowest and has "Rolar de novo", which replaces all six and undoes the placement.
  - The player puts each result on an ability: a selector on a tablet or desktop (choosing a result already used swaps the two), tap the result and then the ability on a phone.
  - "Conjunto padrão" works the same with 15, 14, 13, 12, 10 and 8.
  - While a result has no ability, the editor does not save and says "coloque cada resultado numa habilidade".
  - These rolls are not recorded and are not part of RN-18 (the player's 4d6 of RN-24 are). The sheet stays editable until the first session and the master reviews it.
- **Given** the step "Pontos de vida" with "Rolado", **when** the player opens it, **then** there is one row per level from the 2nd ("Nível 2 (1d12)"), with "Rolar" and "Rolar os níveis que faltam" (only the empty ones; the player may type what came up on a table die).
  - The formula reads "1d12 (8) + 3 (Constituição) = 11 PV", with the final Constitution.
  - A multiclass rolls each level with the die of the class that gives it.
  - The editor does not save while a level has no roll or a roll that does not fit the die, and names the level.
  - What race, class and features add (for example one hit point per level for a Hill Dwarf) comes from the server's preview of the draft and is named in one short line ("+1 PV de raça, classe ou característica"). If the preview fails, the box shows the dice alone and says the preview leaves those hit points out.
- **Given** a spell in the editor, **when** the player taps the "?" beside it, **then** the description opens (dialog on desktop, bottom sheet on phone): name, spell level and school, casting time, range (in metres: 5 ft = 1,5 m), components, duration, the Ritual and Concentração tags and the SRD text in Portuguese, plus "Em níveis superiores" when present.
  - "Ver em inglês" / "Ver em português" flips the text to the SRD's English for the whole app until the page reloads.
  - A spell without a translation shows the English, marked "(em inglês)".

#### Related
- The rules engine calculates the sheet from rules as data. See [ADR-0008](../adr/0008-rules-content-as-data-pure-engine.md) and [Architecture](../architecture/characters.md).

### MR-006: Locked sheet

**As a** master, **I want** the player's sheet to be view-only from the first session on, **so that** only I and the system change it.

- Priority: MVP
- Rules: RN-01
- Modules: characters

#### Acceptance criteria
- **Given** the campaign's first session has started, **when** the player tries to edit the abilities of their own sheet, **then** the server refuses **and** the master can edit the same sheet.
- **Given** no session has started, **when** the player edits the sheet, **then** the change is saved.
- **Given** a character created after the first session, **when** the player edits the sheet before the next session, **then** the change is saved **and**, when the next session starts, the sheet locks.
- **Given** the sheet is locked, **when** the player tries to edit the character's story, **then** the server refuses; **after** the master unlocks that character's story, the player edits and saves, **and** the permission ends when the next session starts.
- **Given** a locked sheet, **when** the player opens its edit address (typed or bookmarked), **then** the screen shows "Ficha travada" (or "Personagem morto") with the reason and "Voltar para a ficha", before any form.
- **Given** the player's sheet is open when the master starts a session, **when** the session starts, **then** the sheet shows "Travada" without a reload ("Editar ficha" gives way to it).

#### In the app
- Starting a session locks the sheets in the same transaction. The player gets a refusal with the reason `SHEET_LOCKED` or `STORY_LOCKED`.

### MR-008: Points of interest

**As a** master, **I want** to create points of interest that open a battle, a submap or an RP scene.

- Priority: MVP
- Rules: —
- Modules: maps

#### Acceptance criteria
- **Given** a campaign map, **when** the master creates a point of type battle, submap or RP scene, **then** the point appears on the map, hidden at first, **and** opening the point leads to the encounter, the submap or the scene.
- **Given** a map created from a gallery image, **when** the master creates it, **then** it starts hidden.
- **Given** a submap point, **when** the master picks its destination, **then** it can lead to another map of the same campaign, never to its own map.
- **Given** a submap point that leads to a map, **when** the master looks at the point's panel in the editor, **then** "Abrir <mapa>" takes the master to that map's editor; **and** in the live session the point's row has "Ir para <mapa>", which makes that map the session's current map (a hidden destination is revealed, as in the "Mapa atual" select).
- **Given** a submap point with no map yet, **when** the master picks "Criar mapa novo…" in "Leva para", names the map (the point's name by default) and chooses a gallery image, **then** the map is created hidden, becomes the point's destination and the point is saved in one step, with "Abrir" to set the new map up.
- **Given** a submap point, **when** the master opens it, **then** the point's card shows its name and description with "Abrir <mapa>". Battle and scene points open the combat encounter and the RP scene (see [MR-013](#mr-013-turn-order) and [MR-015](#mr-015-rp-scene-actions)); outside a session, they show the point's card.
- **Given** a map, **when** the master renames it ("Renomear") or deletes it ("Apagar mapa"), **then** the delete asks in place what goes with it (the points and tokens; the image stays in the gallery) and warns when it is the open session's current map. After deleting, the app returns to the campaign.
- **Given** a map that cannot be deleted (a combat runs on it, a treasure was turned into XP, a treasure was found), **when** the master looks at "Apagar mapa", **then** the button is already grey, still clickable, with the first reason under it; if the server refuses anyway, the question says the same reason.

#### In the app
- In the editor, choosing the type and clicking the map places the point; dragging or the arrow keys move it; "Salvar ponto" saves the panel. On a phone the master only pans, zooms and reveals through the lists. See [Design](../design/session.md#images-shown-to-players) and [Architecture](../architecture/maps/README.md#maps-points-and-tokens).

### MR-009: Map without spoilers

**As a** player, **I want** to see on the map only the points my group already knows, **so that** I get no spoilers.

- Priority: MVP
- Rules: RN-10
- Modules: maps

#### Acceptance criteria
- **Given** a map with one revealed point and one hidden point, **when** the player opens the map, **then** only the revealed one appears **and** the server response does not contain the hidden one.
- **Given** a hidden map, **when** the player asks for it, **then** the answer is the same as for a map that does not exist ("Mapa não encontrado"); a player reaches only revealed maps and the session's current map.
- **Given** a change only to hidden things, **when** it happens during a session, **then** it reaches only the master through the live session.

#### In the app
- The player's screen shows the submap trail, "Mapas revelados" and "Pontos deste mapa". Tokens the master has not made visible never arrive.

### MR-011: Start the session

**As a** master, **I want** to start the session, have the players receive a notification in the app and get a session link to send them, **so that** everyone joins together.

- Priority: MVP
- Rules: RN-06, RN-07
- Modules: play, campaigns, characters

#### Acceptance criteria
- **Given** a campaign with three players, **when** the master starts the session, **then** whoever has the app open sees the notification **and** the master can copy the session link.
- **Given** the session link, **when** someone who is not a member (or is a pending member) opens it, **then** they see "Peça um convite ao mestre", without the campaign name, **and** do not get in.
- **Given** it is the campaign's first session, **when** the master starts the session, **then** the players' sheets lock.
- **Given** player characters whose sheet still has choices open (skills, cantrips, known and prepared spells), **when** the master starts the session, **then** the "Sessão" card lists them ("Ilaria: faltam 1 perícia") and asks "Iniciar mesmo assim?". The sheets lock as they are, and the master decides. With no such sheet the session starts at once. Only the master sees this list.
- **Given** a member with no open session, **when** they open the session page, **then** they see "Nenhuma sessão em andamento", and the page opens the session by itself when the master starts it.

#### In the app
- The notice "A sessão 4 de Mirathel começou." sits under the app bar with "Entrar na sessão", for people who play in the campaign, outside the campaign and session pages. Closing it lasts only for the tab. The app checks for open sessions every 30 seconds while the tab is visible. The on-screen notice is enough: there is no browser push notification (RN-06).
- Other entry points: the "Ao vivo" link in the bar, the "Sessão ao vivo" tag in "Minhas campanhas", and the session strip of the campaign page.
  The master's strip sits above the three tabs, so it is there on every one: "Encerrar sessão", "Copiar link da sessão" and "Entrar na sessão" (the page's one filled button), or "Iniciar sessão" while none is open. The list of sheets with choices open ("Iniciar mesmo assim?") opens under it.
  On a phone the strip keeps "Entrar na sessão" and a "Mais ações da sessão" menu with "Copiar link da sessão" and "Encerrar sessão".
  A player's campaign page shows the strip only while a session is open, with "Entrar na sessão".
- On the session page, a bar of one row at the top holds the way back to the campaign, "Sessão 3", "Ao vivo desde 20:12" and "Dados" for everyone. The master also has "Iniciar combate" (outside a combat) and the "Sessão" menu with "Copiar link da sessão" and "Encerrar sessão", which asks in place ("Encerrar a sessão 3? Ela não reabre depois.", "Cancelar" first). On a phone the nav "Cena · Ficha · Conversa" is pinned to the bottom of the screen.
- See [Architecture](../architecture/play/live-session.md).

#### Related
- RN-07: invites default to 1 use and 7 days; the master picks 1 to 20 uses and 5 minutes to 30 days, and can revoke (see [MR-002](#mr-002-generate-an-invite)).

### MR-012: Follow the session

**As a** player, **I want** to follow my sheet and the current map during the session.

- Priority: MVP
- Rules: RN-02, RN-10, RN-11, RN-20
- Modules: play, maps

#### Acceptance criteria
- **Given** an active session, **when** the master moves a token or the system applies damage to the character, **then** the player's phone shows the change without reloading the page **and** the master's notes never appear (RN-11).
- **Given** the live session, **when** the player looks at the sheet half, **then** they see the hit points, temporary hit points, armor class, hit dice and spell slots of their own character only, never another player's.
- **Given** the master opens "Ajustar" (a bottom sheet on a phone, a dialog on desktop), **when** they correct hit points, temporary hit points, spell slots or hit dice, **then** the owner and the master see the change at once, the value never goes past the sheet's maximum, and after the session ends the sheet says "A sessão acabou" (RN-02).
- **Given** the connection drops, **when** the player looks at the page, **then** it says "Reconectando…" with the time of the last update, and the numbers stay on screen.
- **Given** the master chooses the session's current map, **when** a player is in the session, **then** the player's map follows it; with no map chosen, the notice reads "O mestre ainda não escolheu um mapa."
- **Given** the master moves a visible token, **when** the player has the map open, **then** the token moves on the player's screen without a reload. A token the master hides (an NPC's, for example) reaches only the master. If the player loses sight of the map, they go back to the notice.
- **Given** the master's session screen, **when** the master looks at the map, **then** there are the "Mapa atual" selector, draggable tokens, "Pontos do mapa" and "Tokens no mapa" with "Revelar aos jogadores" and "Esconder". A character's creature has its own row, always visible, and dragging its token moves the creature, not its owner.
- **Given** a map with a grid and no combat on it, **when** the master drags a character's token or moves it from the "Tokens" tab, **then** the token walks toward the square and stops before a wall, a locked door or a square marked "Não dá para passar", the screen says where and why ("O movimento parou antes. O próximo quadrado está marcado como “Não dá para passar”. Tor andou 7,5 m."), and nothing moves until the server answers. Several tokens chosen together move around the first; if one does not fit, nobody moves and the screen says who is in the way.
- **Given** the switch "Os jogadores movem os próprios personagens" is on for the map, **when** a player drags their own token, **then** it walks the same way; a player never moves another character's token, and with the switch off only the master moves tokens. A player never learns why a token stopped at a square they have not seen (RN-10): the screen says only "O movimento parou antes.".
- **Given** the switch is off (the default) or on, **when** a player looks at the map, **then** a line under it says who moves: "O mestre move os tokens agora." with no control, or "Arraste Brisa no mapa ou use “Mover Brisa”. Os outros ficam com o mestre." with the button "Mover Brisa" for their own character only. **When** the master frees or locks the moving while the player looks, **then** the line becomes a polite one-line notice for 8 seconds ("O mestre liberou: agora você move Brisa." or "O mestre voltou a mover os tokens."), and locking closes an open move panel without moving.
- **Given** a combat, **when** the player sees an enemy, **then** they see a word (Ileso, Ferido, Muito ferido, Derrotado), not hit points (RN-20).
- **Given** an attack that hits, **when** it deals damage to an NPC, **then** it is applied at once (temporary hit points first; at 0 hit points the NPC is defeated and leaves the order).
- **Given** an attack or a spell that deals damage to a player's character, **when** it lands, **then** the damage waits for the master, who applies it ("Aplicar 5 de dano", or another amount) or discards it. When applied, the owner and the master see it live. Damage to someone at 0 hit points counts one death-save failure (RN-03). A heal is applied at once and raises someone at 0.
- **Given** the master, **when** they need to fix something, **then** they can edit an NPC's hit points ("Dano/Cura"), undo the last action and read the combat log. The player gets the log only with what they see (RN-20).

#### In the app
- In a combat the player's screen is the map first, with the movement under it ("Restam 9,0 m · 6 quadrados" and "Mover"), then the turn card with their own hit points, armor class and slots, then the actions in tabs; the other creatures' hit points are words, never numbers (RN-20).
- The target of an attack, a spell or "Agarrar"/"Empurrar" is tapped on that map: the page brings it into view with the accent border, the creatures the server offers have a ring, the ones out of reach or behind total cover are dimmed and say why when tapped ("Goblin 3: cobertura total"), and a bar above the nav (a card in the rail from 1024 px) reads "Escolha o alvo · Raio de Fogo" with "Escolher na lista", "Cancelar" and "Confirmar". Magic Missile and Scorching Ray take one tap for each dart or ray ("2 de 3 dardos", "Desfazer"). "Escolher na lista" opens the sheet's own list, the way for the keyboard and the screen reader.
- Outside combat the master's "Tokens" tab (under the map) holds the switch "Os jogadores movem os próprios personagens", "Pôr o grupo no mapa" (a dialog, a sheet on a phone: who comes, "Em fila", "Em bloco" or "Em arco", and the square of the map for the group's corner) and "Selecionar" (a tap on a token adds it to a selection). A box on each character's row chooses it: one chosen is "Mover Tor" (the direction pad, "Quadrados por toque" from 1 to 20, "Tor vai 3 quadrados para a direita · Distância: 4,5 m" and the button), several are "3 selecionados" with "Mover os 3 juntos" and "Limpar seleção". The player's "Mover Brisa" (under the map, in the card on a computer and in a sheet on a phone) is the same panel for their own character alone, with "Brisa andou 2 (3 m)." or "O movimento parou antes." and the reason when the server gives one; on a computer the player may also drag their own token. A click on a square of the map sets the destination; the map draws a ring on each token chosen, a dashed line to the destination and, after a move that stopped early, the line walked, an X where it was headed and "7,5 m · parou antes".
- See [Architecture](../architecture/play/live-session.md) and [Combat](../architecture/play/combat.md#combat).

#### Related
- RN-02: the master can correct hit points and spell slots by hand during the session, and has the final word (see [Business rules](rules.md)).

### MR-013: Turn order

**As a** player, **I want** to see the turn order, where each one is and how far I can move, **so that** I can plan my action.

- Priority: MVP
- Rules: RN-19, RN-20, RN-21
- Modules: play, rules

#### Acceptance criteria
- **Given** a combat with initiative set, **when** the player opens the combat screen, **then** they see the turn order, where each visible combatant is and how much they can still move this turn.
- **Given** a combat with a grid, **when** I open the turn economy or the map, **then** movement also appears in squares ("7,5 m · 5 quadrados"; 1 square = 1,5 m = 5 ft).
- **Given** a map without a grid, **when** the master starts a combat, **then** the app sends them to "Grade do mapa", where they set 4 to 200 squares across; the rows follow the image proportion. A combat does not start without a grid.
- **Given** the master starts a combat ("Iniciar combate"), **when** they fill in the name, the map, the group and how each player rolls, and the NPCs with how many copies and "Escondido no início", **then** each NPC copy rolls its own initiative (RN-19) and the player rolls in the app or types the physical d20 (RN-18).
- **Given** monsters from the bestiary, **when** the master adds them in the same dialog ("Adicionar criatura", search, how many of each and how they start), **then** they enter with the party. "Adicionar combatente" does the same during a combat.
- **Given** the initiative, **when** the master looks at it, **then** the totals show the sum (`1d20 (15) + 4 = 19`), a tie is shown once and the master orders it, "Esperando …" has "Digitar pelo jogador", and "Começar o combate" is blocked with the reason.
- **Given** a combat running, **when** the turn passes, **then** the round goes up after the last combatant, and each one's movement, action and reaction come back at the start of their turn. The master has "Próximo turno" and "Encerrar combate" (which asks first), the player has "Encerrar turno".
- **Given** a combatant the player cannot see, **when** its turn comes, **then** the bar says "Vez do mestre", and no hidden combatant or NPC number reaches the player (RN-20).
- **Given** a player whose character has creatures (a familiar, an animal the master gave, summoned creatures), **when** the initiative is open, **then** they also see "Iniciativa de <nome>" for each creature that has not rolled (MR-037).
- **Given** the player's turn, **when** they open "Mover", **then** they see the range ("Mover 3 m. 2 quadrados para a direita e 1 quadrado para baixo. Depois restam 4,5 m."), "Longe demais: faltam 1,5 m" or "Ocupado" for a refused path, and on desktop can drag the token within range. The app does not let the player go past the turn's movement (RN-21); the master can.
- **Given** a combat ends, **when** it ends, **then** both see "Combate encerrado".
- **Given** combatants side by side in the order with the same initiative total, players included, **when** the turn order is shown, **then** they form a joint turn, in a box with the words "Turno conjunto" and the total. The master sees all groups; a player sees only a group that has a player character (a group of only NPCs is the master's, RN-20).
- **Given** a joint turn, **when** the turn reaches the group, **then** every member's turn starts at once, each with their own action, bonus action, reaction, movement and death save; each member's player ends their own part, the master ends anyone's, and the turn passes only when the last one ends (there is no "end the group's turn" and no reopening a part). Reordering a tie, bringing reinforcements or hiding a member does not change a turn that already began, and a reinforcement with the same total acts from the next turn.
- **Given** the player ends their own part, **when** they tap "Encerrar a minha parte", **then** the app asks first ("Encerrar a sua parte?", with what is still left and "Voltar" first), because a part does not reopen. A double tap ends one part only.
- **Given** a joint turn with a hidden member, **when** the player reads the turn, **then** the hidden member is never named.
- **Given** a joint turn of which one member died, **when** the next round brings the survivor's turn, **then** its Action, bonus action, reaction and movement start fresh, and the line "A vez passou para <nome>" of the earlier turn is gone.

#### In the app
- The player's turn: "Sua vez, Pensantus" with "Rodada 3 · depois de você: Toren" and what the turn still has as shapes (a circle for the Ação, a triangle for the Bônus, a square for the Reação, a walker for the metres left; filled while it is there, dashed once spent). The actions are tabs built from the character's own options ("Ataques", "Magias" for a caster, a tab named for the class when it grants actions, "Outras" for the standard actions); a spent group is one folded line ("Ação bônus · Usada") that opens to what it held. "Mover" is the row under the map on a phone and part of the turn bar from 1024 px, with "Encerrar turno" (a bar above the phone's nav says "Ainda tem" and the shapes left). Off turn the screen says whose turn it is and, from 1024 px, "Reação · Disponível".
- The master's combat is the combat bar (round, whose turn, "Em seguida", what waits, "Próximo turno"), the order, the map and the rail "Turno / Conversa" (with "Mais do combate", a fold per panel) in three columns on a computer, and the tabs "Turno · Ordem · Mapa" with a fixed bar on a phone (see [Design](../design/combat.md#the-masters-combat)).
- Screens: the master's card has one block per member and "Encerrar a parte da Brisa"; the player reads "Vez de Brisa e Toren", "Vez dos Goblins", "Vez das Espadas voadoras" (the plural of a two-word name keeps the noun and its adjective in the plural), "Você encerrou a sua parte" and "O que a Brisa ainda tem".
- The screen follows the live session without reloading.
- See [Combat](../architecture/play/combat.md#combat) and [Design](../design/combat.md).

#### Related
- Each NPC rolls its own initiative (RN-19); the player sees enemy state by a word, never hit points or armor class (RN-20); each grid square is 1,5 m, diagonals included (RN-21). See [Business rules](rules.md).
- The available speed comes from the rules engine. See [ADR-0008](../adr/0008-rules-content-as-data-pure-engine.md).

### MR-014: Your turn

**As a** player, on my turn, **I want** to see my actions, bonus actions and possible attacks.

- Priority: MVP
- Rules: RN-02, RN-03, RN-18, RN-20, RN-22, RN-35
- Modules: play, rules

#### Acceptance criteria

**Actions and spells**
- **Given** a combat, **when** Pensantus's turn comes, **then** the player sees the action, bonus action, reaction and movement available **and** spells without a spell slot appear disabled, each option with its reason in words ("Ação já usada", "Sem espaço de 2º nível ou maior"), still in place and in keyboard reach. Outside the turn everything is disabled ("Não é a sua vez").
- **Given** the screen "O que você pode fazer", **when** the player reads it, **then** it groups Ação (Ataques, Magias, Ações padrão), Ação bônus, Reação and Movimento, each with a state word ("Disponível", "Usada").
- **Given** a character with spells, **when** they open the spell list on their turn, **then** the ones they can cast now come first, then the others; in each group cantrips come first, then by spell level, then by Portuguese name. Shield on the character's own turn sits among the others with "Só fora da sua vez".
- **Given** a spell in the list, **when** the player taps the "?", **then** they see the full description, as in the editor.
- **Given** Pensantus casts Magic Missile (Mísseis Mágicos) with a 1st-level slot, **when** the action is confirmed, **then** the system marks the slot as used and the action as spent at once. A retry does not spend again.
- **Given** the last slot of a level, **when** the player casts with it, **then** the sheet warns "É o seu último espaço de 1º nível" before the button.
- **Given** a spell with no known effect, **when** it is cast, **then** the app says "A magia foi conjurada: o mestre resolve o efeito."
- **Given** an attack, **when** the player attacks, **then** they pick a target (state and distance shown; "Longe demais: alcance de 36 m" is disabled), roll the d20 and then the damage in the app or typed (RN-18), and read the result (`1d20 (13) + 6 = 19`, "Acertou", "Crítico" or "Errou") and the defeated target, never armor class.
- **Given** class abilities, **when** the player taps "Usar" (Second Wind, Action Surge), **then** the use is spent; with no uses left the app says "Sem usos: volta num descanso curto". Extra Attack shows "1 ataque restante".
- **Given** the action or the bonus action is free, **when** the player looks at "Encerrar turno", **then** it is outlined and, with the action free, asks in place; it becomes the filled button when both are spent.

**Advantage and disadvantage**
- **Given** an attack, a spell attack or save, a trap, a puzzle hint, a scene check or a search for traps, **when** the server finds a circumstance that gives advantage or disadvantage (a condition of either creature, a creature one cannot see, long range or a hostile within 5 ft for a ranged attack, Reckless Attack, Danger Sense, Pack Tactics, Dodge, a racial advantage on a save against the kind of source the save has: Bravura against fear, Ancestral Feérico against charm, Esperteza Gnômica on Intelligence, Wisdom and Charisma saves against magic, Resiliência Anã against poison), **then** the roll takes two d20, the higher or the lower counts (they cancel each other out when both apply), the screen lists the reasons the player can see **and** shows both dice, the one that counts marked "vale" and the other "descartado"; with real dice the player types both. A hit within 5 ft of a paralyzed or unconscious target is a critical hit (SRD 5.1, "Advantage and Disadvantage", Conditions).
- **Given** a player's character at 0 HP and dying, **when** a weapon attack or a spell attack targets it, **then** the target counts as unconscious: the attack has advantage, a hit from within 5 ft is a critical hit, and a hit adds one death-save failure, or two for a critical hit (SRD 5.1, Conditions, Unconscious; Damage and Healing, Death Saving Throws).
- **Given** the suggested mode, **when** the player takes disadvantage, **then** they give a reason of 1 to 120 characters **and** the roll goes with it; **when** they want advantage the server did not suggest, **then** they ask the master with a reason ("Pedir ao mestre"), and the master approves, refuses or sets another mode in the queue. The master may set any mode on any roll, always with a reason; on an NPC's attack the master's card has the same "Como rolar o d20" choice (Normal, Vantagem, Desvantagem, the suggested one marked), and the d20 fields follow the chosen mode. Every change is a line in the log; the reason is read by the master and the attacker's player only (RN-20).

**Damage**
- **Given** a weapon hit with an extra on offer (Sneak Attack, Divine Smite with the slot it spends, Hunter's Mark, Colossus Slayer), **when** the player marks the extras and rolls, **then** the damage is rolled by part (the weapon, each extra, the automatic lines of Rage and Great Weapon Fighting) **and** a critical hit doubles the dice of every part and never the fixed numbers; an extra that does not hold is disabled with its reason; with real dice the player types one sum for each part. The master sees the damage by part and takes an extra out with a reason, which gives its once-per-turn use back.
- **Given** Divine Smite against any target, **when** the damage is rolled, **then** the extra die for an undead or fiend is rolled and shown as a line like the others **and** the total a player reads is the damage as rolled, so neither the lines nor the total tell what the target is; only the master reads that the die did not count.
- **Given** a target with a resistance, vulnerability or immunity (a tiefling's fire, a dwarf's poison, a dragonborn's lineage, a raging barbarian's physical damage, a creature's stat block), **when** the damage lands, **then** each type goes through the modifiers (resistance halves, rounding down; the same resistance from two sources counts once) **and** the master and the target's player read each step, while for an NPC the players read only the damage as rolled (RN-20); the master may leave a source out when they apply.

**Barbarian**
- **Given** a barbarian who rages, **when** the rage is on, **then** "Em fúria" shows beside the name, spells are refused, and when the turn ends with no attack on a hostile creature and no damage taken the player is asked "A sua fúria vai acabar?" ("Voltar e atacar" or "Deixar a fúria acabar") before the turn passes; Marcado, Esquivando and Ataque descuidado are states in the same place, each with its end (SRD 5.1, Barbarian, Rage).
- **Given** a Berserker of level 3 or more who taps "Fúria", **when** the screen asks "Entrar em frenesi?", **then** "Fúria com frenesi" starts the rage with "Em frenesi" on it, one melee weapon attack is a bonus action on each of the barbarian's later turns, and the barbarian gains one level of exhaustion when the rage ends ("Só fúria" rages as always, with no exhaustion); any other barbarian is not asked (SRD 5.1, Barbarian, Path of the Berserker, Frenzy).

**Halfling Lucky (RN-35)**
- **Given** a halfling whose attack d20 (a weapon attack or an attack cantrip) shows a 1, typed or rolled in the app, **when** the roll comes back, **then** the sheet asks "Tirou 1: rolar de novo?" before any result: "Rolar de novo" rolls that d20 again (in the app or typed, RN-18) and the new roll stands, even another 1; "Ficar com o 1" keeps the natural 1, a miss; with advantage or disadvantage the question comes for the die that shows the 1, even the one that would not count, and only that die is rolled again; a Bardic Inspiration die the halfling holds is asked about after it, on the new roll; the result and the log say "Sortudo: 1 → 14" (SRD 5.1, Halfling, Lucky).
- **Given** a halfling whose player rolls a d20 that shows a 1 on a scene check, a group check, a trap search (out of a combat, or the Search action of a combat), a puzzle hint try, either side's roll of a contest, an initiative before the combat begins, a death save, the Stealth check of a Hide or the ability check of a Counterspell above the slot's level, or, in a combat, on a spell attack against one target, an effect's saving throw or a concentration save, **when** the roll comes back, **then** the sheet asks "Tirou 1: rolar de novo?" before any result and nothing is written, spent or closed (the save's window stays open); the answer works as on an attack (the new d20 in the app or typed, or "Ficar com o 1"; one die of a pair; the Bardic Inspiration die after it, on the new roll) **and** Reliable Talent counts the new d20, not the 1; a reload asks again; the character rolls nothing else that could ask until it is answered; a roll the master makes for the halfling there (a save or a spell attack for a player's character, a group check rolled for the player) asks too: the master is asked in his own sheet, the player reads the same question, and the first answer wins ("Pending questions" below).
- **Given** a player's halfling whose saving throw the app rolls inside someone else's action (a spell's save, a trap's save, the Dexterity save against Hellish Rebuke), **when** the d20 shows a 1, **then** nobody is asked: the app rolls that die again by itself and the new roll stands, even another 1 (with advantage or disadvantage, only the first die that shows a 1); the result and the log say "Sortudo: 1 → 14" to whoever sees that save's dice; an NPC with a halfling's sheet keeps its 1.

**Pending questions (RN-35)**
- **Given** a roll that waits for a question (Lucky's "Tirou 1: rolar de novo?" or the Bardic Inspiration die), **when** the master answers it for the player, **then** the roll is settled as if the player had answered (the d20 already rolled, the new die in the app or typed, the character the player's own), the player reads the result in the combat or the scene, and the die is spent once; **when** the master tries within 10 seconds of the question being asked, **then** the server refuses it with the seconds left ("Ainda é a vez de Brisa responder. Você pode responder em 3 s.") so that a present player has the first chance, and the player's own answer never waits; **when** the player and the master answer at once after that, **then** the first answer wins and the other is told the roll was answered already.
- **Given** rolls that wait, **when** the master reads the pending questions, **then** the master reads every one of the table (a player's, in a combat or out of one, and an NPC's), each with the d20, the bonus and who rolled; **and** a player reads only their own; **and** another player is told a roll they may not answer does not exist.

**Turn alerts**
- **Given** a combat on the player's screen, **when** the turn passes to a combatant they play (the character or one of its creatures), **then** they are warned once: the phone vibrates where it can, a short chime plays (after any first tap on the page), the turn panel shakes for half a second (a still frame with reduced motion) **and** a hidden tab's title says "● Sua vez"; nothing fires on a page load, on the master's or an NPC's turn, or twice for the same turn, and "Avisar na minha vez (som e vibração)" turns the sound and the vibration off until the page reloads.
- **Given** an open session, **when** the master taps "Chamar a atenção" next to a player's character (in the combat order or in the party panel), **then** that player alone gets the same alert (vibration where it can, the chime, a half-second shake, "● O mestre chama você" in a hidden tab's title, never "Sua vez") and reads "O mestre está chamando você.", in a combat or out of one and even when it is not their turn; the master reads "Chamado." and the button stays off for 10 seconds (the server also refuses a second call to the same character within 10 seconds); a player cannot call, and nothing is shown to the other players.

**Area spells**
- **Given** an area spell (Bola de Fogo, Mãos Flamejantes, Relâmpago, Onda Trovejante) on a combat with a map, **when** the player casts it, **then** they place it on the map (a point for a sphere, a direction for a cone, a line or a cube) **and** the server says who is inside, allies and the caster included, with the cover each one has **from the point of origin**, which counts only for a Dexterity save (RN-21).
- **Given** nobody the caster sees is inside the area, **when** they confirm, **then** the app says the spell still spends the slot and the action and asks again ("Conjurar mesmo assim" or "Mudar o local"). Without a map the list stays and the app only adds the "Ninguém está marcado" confirmation.
- **Given** a hidden creature in the area, **when** the spell is cast, **then** it is hit like any other, and what that does to its hiding is the table rule "Criaturas escondidas atingidas por uma área" (RN-10, RN-24). With "Perguntar a cada vez", every area spell a player casts holds the turn until the master answers ("Esperando o mestre"), a hidden creature in the area or not, so the wait tells nothing; the master sees the area of the last spell on their map and answers with "Revelar", "Manter escondidas" or, with none hit, "Sem escondidas".

**Spells that read hit points, and bringing back the dead**
- **Given** a spell that reads hit points (Sono, Leque Cromático, Palavra de Poder Atordoar e Matar, Estabilizar, Cura Completa), **when** it is cast, **then** the server resolves it with the targets' real hit points, with the dice total rolled in the app or typed **and** the player keeps seeing only "Ileso", "Ferido" or "Muito ferido" (RN-20): the caster sees their own roll and who was affected ("O Goblin 1 adormeceu. O Capitão Goblin não foi afetado."), never a target's hit points. The master sees the total, each creature's hit points and the order, and can change the conditions.
- **Given** a creature that died in the last minute (10 rounds in a combat) and is within touch, **when** a cleric or paladin casts Revivificar on it with a 3rd-level slot and ticks that they have the diamonds, **then** the creature returns with 1 HP **and** the slot, the action and the diamonds are spent **and** the player's list only ever shows creatures the spell reaches now, with no reason for the others (the master reads the reasons and has the switch "Revivificar não funciona nesta morte"). Out of combat the master confirms "Faz menos de 1 minuto" before anything is spent.
- **Given** a dead character, **when** the game master taps "Reviver", **then** it returns with 1 HP, not unconscious and with the death saves cleared, in the combat it keeps its place in the order and acts on its next turn **and** the app refuses, naming the other character, while the player has another living one (RN-03).

**Reactions, conditions and death**
- **Given** a reaction a player may take (Shield, Uncanny Dodge, Hellish Rebuke, Counterspell, Cutting Words, Deflect Missiles, Feather Fall, the opportunity attack) or a concentration save, **when** the window opens, **then** the turn waits and the player answers in a dialog ("Você foi atingido: usar Escudo Arcano?", "Concentração em risco…") with "Deixar passar" and the use button; Escape passes and never decides a concentration save. A reaction answered in the player's own seat is logged as the player's, even while the master's "Usar pelo jogador" and "Deixar passar pelo jogador" are showing; "(respondido pelo mestre)" appears only when the master answered. The player of an NPC's reaction reads "Esperando o mestre" (RN-31). The master's queue is under the NPC card, and "Próximo turno" is blocked with the reason written. The concentration save says its mode in words and, with physical dice, asks for both d20 when the mode has two. The opportunity attack is the text action under "Sua reação" and spends the reaction.
- **Given** contests (grapple, shove, escape, Hide, Help, group checks), **when** the player uses them, **then** they follow the SRD: grapple and shove replace one attack and are decided by Strength (Athletics) against Strength (Athletics) or Dexterity (Acrobatics); escape is an action against the grappler or a fixed DC; a grappler drags at half speed; Hide compares the Stealth total with each creature's passive Perception (a tie keeps the hider noticed); Help gives advantage to a check or an attack; a group check passes when at least half of those asked pass. A contest waits in a reaction window ("Esperando o mestre" or "Esperando <nome>"). A player never reads an NPC's numbers, the escape DC or who noticed a hider.
- **Given** the master asks one, some or all characters for a skill check, an ability check or a saving throw outside a combat, **when** they send it, **then** each roll is judged alone unless they choose a group check, only the characters asked read the request, and the master marks who is surprised.
- **Given** someone at 0 HP, **when** the player looks at the turn, **then** they see "Brisa está caída", the marks and "Rolar teste contra a morte"; on the third failure the character dies only when the master confirms ("Confirmar a morte" / "Ainda não") (RN-03).
- **Given** conditions (the 15 of the SRD; "Derrubado" is Prone) and concentration, **when** they apply, **then** they show as tags under the name, in the player's strip and as a dot on the token, and the player ends their own concentration. The master sets them in "Condições…" in the order's ⋮. When damage hits a concentrating target, the master applying **another value** ("Aplicar outro valor") reads "Teste de Constituição, CD 10". Conditions and concentration are marked and reminded; the master decides the effects (RN-22).
- **Given** a player's character at 0 HP, still making death saves, on its turn with the save not yet rolled, **when** the player is away and the game master reads "Teste contra a morte de Brisa", **then** they roll it ("Rolar no app" or "Digitar o resultado", by the table's dice mode) and the counts move, **and** the turn does not pass until it is rolled ("Próximo turno" is dashed and the bar says "Falta o teste contra a morte"; the server refuses it too, MR-053); the card never shows for an NPC or a stable character, and the other players never read the result (RN-03, RN-24).

**Master's turn and the log**
- **Given** an NPC's turn, **when** the master plays it, **then** the card "Ações do Capitão Goblin" shows hit points, armor class, speed and the attack with the "Alvo" field, which starts on the nearest hostile creature in reach (a player's character, for an NPC; the first of the order among equals), then on the nearest hostile out of reach, and never on an ally unless no other creature exists; it follows the NPC when it moves, until the master picks a target by hand, and keeps the list in the server's order; "Rolar ataque" works in the app or typed ("Acertou contra CA 18 do Toren"), and "Aplicar 5 de dano" / "Não aplicar" (which asks "Descartar o dano de 5?"). While damage is unapplied, "Próximo turno" asks "Há dano sem aplicar. Passar o turno mesmo assim?". Armor class is master-only.
- **Given** the turn of a player's character that has not acted (no Action, no bonus action, no movement spent), **when** the master taps "Próximo turno", **then** the app asks in place "Passar a vez de Toren, que ainda não agiu?" with "Cancelar" and "Passar a vez" (a turn that passed does not come back); an NPC's turn, a character who acted and a character at 0 hit points are not asked: the death save is the whole of their turn, so once it is made "Próximo turno" passes at once.
- **Given** the combat log, **when** anyone reads it, **then** it shows the rounds, newest first, as each audience may read it, with the header "Em andamento desde 20:05". The master undoes the last action by name ("Desfazer o ataque do Capitão Goblin ao Toren (5 de dano)?").

**Rests and class resources**
- **Given** the master takes a short or long rest, **when** they confirm the list of what comes back, **then** the resources recharge as the SRD says; a player spends hit dice by die size on a short rest (with a bard in the party, the first die of each short rest also brings the Song of Rest die).
- **Given** Lay on Hands, Flexible Casting, Metamagic and Bardic Inspiration, **when** the player uses them, **then** each has its own action. The Bardic Inspiration die is used after the d20 and before the result, on an ability check, a weapon or spell attack against one target, or a saving throw the player rolls (including the trap search, the puzzle hint try, an effect's save and the concentration save); saves the app rolls for a target do not ask. Lay on Hands on an undead or construct does nothing and never says why.

#### In the app
- The server calculates the economy, the options and each disabled option's reason; the app only draws them. See [Combat](../architecture/play/combat.md#combat), [Rules and combat](../architecture/rules-combat.md#spell-details), [Attacks and damage](../architecture/play/attacks-and-damage.md), [Casting](../architecture/play/casting.md), [Reactions](../architecture/play/reactions.md) and [Design](../design/combat.md).
- On a phone the attack and cast sheets are bottom sheets; on desktop, dialogs. From 1024 px the turn is a compact strip with "Encerrar turno" on the right.
- "Atacar" and "Conjurar" on a combat with a map ask for the target on the map first (the same targets, reach, cover and disadvantage the sheet's list has: "Longe: desvantagem" is said on the token, not dimmed); "Confirmar" opens the sheet at the roll with the target named ("Adaga em Goblin 1"), and "Escolher na lista" opens it at its list of targets. A spell on an area, on the caster or with no target keeps its own sheet; so do the master's sheets and an opportunity attack.

#### Related
- RN-02: the master can correct hit points and spell slots by hand (see [Business rules](rules.md)).
- Dice (RN-18): the master chooses how the campaign rolls (each player chooses, everyone in the app, or everyone with their own dice) on "Regras da mesa", and sees it in "Dados" on the "Regras" tab; each player keeps their own preference, on the campaign page. With a physical die the player types the sum of the dice and the app adds the modifier. Combat rolls follow this rule.
- The player sees whether they hit or missed and the damage, not armor class or the NPC's roll (RN-20).
- Which actions, reactions and resources the system knows come from the rules engine (rules as data). See [ADR-0008](../adr/0008-rules-content-as-data-pure-engine.md).

### MR-015: RP scene actions

**As a** player, **I want** to see, in a simple list, the actions the master chose for the scene, **so that** I know what I can roll and use outside combat.

- Priority: MVP
- Rules: RN-10, RN-18, RN-20
- Modules: maps, play, rules

#### Acceptance criteria
- **Given** an RP scene with the actions the master chose for it, **when** the player opens the scene, **then** they see the list of actions the master chose, each roll with their own character's bonus already calculated (for example Investigação) **and** abilities that only work in combat do not appear. The player also reads the point's name and description and their passive Perception, Investigation and Insight.
- **Given** a combat (MR-013, MR-014), **when** the player sees the possible actions, **then** the system decides that list, by the D&D rules, never the master. The RP scene is the only place where the master chooses the list.
- **Given** the master picks the actions on the map's scene point, **when** they add one, **then** it is a skill, an ability check or a saving throw, with an optional name (up to 60 characters) and an optional DC (1 to 30); a scene holds at most 20 actions and nothing combat-only. The list can be reordered, edited and removed.
- **Given** a scene point, **when** the master opens the scene in the session, **then** any scene point opens, even one without actions or a hidden one (it stays hidden on the map), and they close it without being asked. Everyone sees the scene open or close live.
- **Given** a player rolls an action, **when** they roll in the app or type the physical die (RN-18), **then** the bonus is added, up to the action's attempts limit. The master sees every roll, with the total and "passou" when there is a DC. The player sees only their own, with "passou" only when the scene shows the DC.
- **Given** a scene, **when** the master turns on "Mostrar a CD aos jogadores" (per scene; off by default), **then** the player sees the DC of the actions that have one and whether their own rolls passed; when off, they see only the total (RN-20). An action without a DC shows nothing extra, and the master always sees the DC.
- **Given** a scene action, **when** the master sets "Tentativas por jogador" (1 to 5, 1 by default, or "Sem limite") and can give a player one more attempt ("Dar mais uma tentativa"), **then** the player rolls up to their limit and sees how many attempts are left ("Restam 2 de 3 tentativas", "Sem mais tentativas"). Closing and reopening the scene resets the counts; lowering the limit below what someone already spent leaves them without attempts, without erasing anything.
- **Given** the master's "Dar mais uma tentativa", **when** they offer it, **then** the button appears only on the character's last roll of that action, asks in place with "Voltar" first and, with the DC shown, is offered only if the roll failed (or when the action has no DC). The player's screen announces it.
- **Given** an RP scene point, **when** the master attaches gallery images to it (up to 8, in an order they choose), **then** only the master sees that list (RN-10), **and** in the live session the open scene shows the images with "Mostrar aos jogadores", the gallery's own action (MR-019), **and** the players see an image only once the master shows it. Deleting the gallery image or the point removes the link.
- **Given** a combat on screen, **when** a scene is open, **then** the scene blocks are not drawn for anyone; the scene returns to the screen when the combat ends.
- **Given** the master who opens a scene, **when** they pick the players who are in it ("Quem está na cena"), **then** only those players, and the master, receive the scene: its name, point, description, actions, rolls, stage, the group check asked in it and every live hint about it (RN-10). With no one picked the scene is open to the whole table, **and** a player who joins the campaign later is in; with a list, the scene is open to exactly those players, and a player who joins later is out.
- **Given** a scene open to only some players, **when** a player who is out reads the scene, rolls an action of it or asks for its stage, **then** they get what they get when no scene is open: no scene, the very refusal of "no scene is open", no portrait (404), and not a live hint about it, so nothing tells them a scene is open.
- **Given** the open scene, **when** the master changes who is in ("Quem está na cena"), **then** a player who comes in sees the scene at once (and the rolls they made before in it, if they had been in), **and** a player who goes out loses it at once, with no scene on their screen; the rolls already made stay with the opening. The players who stay, and those who came in or went out, hear the change; no one else does.
- **Given** the open scene, **when** the master reads it, **then** it says since when it is open and who is in and who is out, with the characters' names ("Aberta desde 20:20 para Pensantus, Brisa"; "Fora da cena: Sálvia e Jabuti"); no player reads that list.
- **Given** a scene open to only some players, **when** the master asks for a group check ("Teste em grupo") without naming characters, **then** it asks the scene's players only, **and** naming a character who is out is refused; a player taken out of the scene stops reading the check and cannot answer it. The master gives an attempt only to a character who is in.
- **Given** a scene open to only some players, **when** the session ends, **then** the checks passed in it count in the summary for the master and for the players who were in, and for no other player.
- **Given** a roll a player made on an action that the master hid afterwards, **when** the session ends, **then** the roll stays in the summary for the players who saw or rolled it, because it happened: the summary counts tries and passes per character, never the action, so it names nothing of it; the rolls list of the scene no longer shows the roll to the players (RN-10).
- **Given** a scene open to only some players, **when** the master turns on "Os jogadores podem entrar" (off by default, changeable while the scene is open, set back to off when the scene closes), **then** a player who is out reads exactly two things of it: its name and that they can join, and nothing else (no point, description, stage, actions, rolls, images or clues); with the switch off they read nothing, as when no scene is open. Every player is told when the switch changes.
- **Given** a scene that lets players join, **when** a player taps "Entrar na cena", **then** they become one of its players at once, read it as any member does and the master sees them marked as having joined by themselves, against the players he chose; joining again changes nothing. The master, a stranger and a pending member cannot join, and with the switch off the refusal is that of "no scene is open".
- **Given** a player who joined a scene, **then** they are one of its players for what follows: an image shown to "the scene's players" reaches them at once, and an image shown to chosen players does not, unless they were chosen.
- **Given** a scene opened to some players, **when** the master opens it to everyone, **then** it is "discovered" for the group and any player may label a note with its name; while it is open to some, only its players discover it, each for themselves: they may label a note with it (also when the master brings them in or they join it, and after they leave it, as what they read they read), and no other player's list of scenes or notes tells its name nor accepts it as a label (the refusal is that of a scene that does not exist).
- **Given** a scene open to only some players, **when** the master names who is in, **then** he may name any player of the campaign, also the player of a dead character: the scene's audience is about players, and that player follows the scene like anyone in it. Rolling an action still needs a living character.
- **Given** a scene open to only some players, **when** the master reveals a clue to a character whose player is out, **then** it is allowed (the master chose the character): the clue is in that player's notes and nothing else of the scene reaches them: no scene, stage, actions, images or scene name, and the clue carries no scene label for them.
- **Given** a scene action, **when** the master marks it "Escondida" (adding it, or later, also with the scene open), **then** no player receives it: not its name, DC, attempts or rolls, on the map, in the open scene, in a live hint or in the history they read (RN-10); the master still sees it, marked, rolls on it for the table and gives attempts on it.
- **Given** a hidden action, **when** a player rolls it anyway, **then** the refusal is exactly the one of an action that does not exist (same error, same message), so it never tells the player that the action is there.
- **Given** a hidden action in the open scene, **when** the master shows it ("Os jogadores veem"), **then** the scene's players see it at once and can roll it; **when** the master hides it again, **then** it leaves their screen at once, with the rolls they made on it. Nothing is erased: showing it again gives them back their rolls and attempts.
- **Given** the master edits, moves or removes an action that is hidden (and stays hidden), **when** the players are watching, **then** only the master's screen hears of it.
- **Given** the group check ("Teste em grupo"), **when** the master asks it, **then** it asks the table for a skill, never for a scene action, so a hidden action is neither offered nor named by it.

#### In the app
- Editor: the point panel's "Ações da cena" and "Imagens da cena" (thumbnails in order with ↑ ↓ and "Tirar da cena", which keeps the image in the gallery; "Escolher da galeria"; "Gerar imagem com IA", whose picture joins the list; "N de 8").
- Session: the master opens "Cena de RP" from the "Cena" tab ("Abrir cena", "Trocar cena", "Fechar cena"; opening one switches to the tab). The open scene is one card: "Em cena" (a row per NPC), "Imagens da cena" (thumbnails; a tap shows one, a whole-map image asks first; the one on show is "À mostra"; the dashed "Da galeria" tile and "Parar de mostrar"), "Ações e rolagens" (a line per action with each roll as a chip that opens the formula and Passou / Não passou; "Teste em grupo" in its header), "Pistas" and the folded "Ganchos e anotações". On a phone the parts fold. The player has the block "Cena" with their bonus and "Rolar".
- Who sees what (RN-10): the first line of the open scene says "Aberta desde 20:20 para" with the players in it as chips (a count and overlapping initials past three) and "Quem está na cena", which opens the chooser of players (a sheet on a phone); under it, who is out ("Toren não está nesta cena.") and, for a scene open to only some players, the switch "Os jogadores podem entrar"; when a player joins by themselves the line says "Toren entrou na cena às 20:41.". Each action has an eye ("Escondida": the players never get it nor its rolls).
- Player screen, outside a scene open to only some players: with the master's switch off nothing of the scene is drawn (the page is as when no scene is open: the map and the images left); with it on, one compact card, "Há uma cena aberta", the scene's name, "Você ainda não está nela. Entre para ver e participar." and a filled "Entrar na cena". Entering shows the scene as any player's, under "Você entrou na cena", with the title focused; if the master turned the switch off meanwhile the card goes and the page says "A cena não está mais aberta para entrar.". See [Design](../design/session.md#the-players-scene-and-the-ficha-panel).
- See [Scenes and stage](../architecture/play/scenes-and-stage.md).

#### Related
- In an RP scene the master chooses the possible actions and the player sees what they can do with their own bonus; in combat the system decides. See [Business rules](rules.md) and [ADR-0008](../adr/0008-rules-content-as-data-pure-engine.md).
- Scenes have skill checks, ability checks and saving throws; spells and abilities are not part of them.

#### Open points
- A package exported with a hidden action brings it back hidden. Is that what the master wants when he shares a campaign with another master?

### MR-016: Award XP

**As a** master, **I want** to give XP to the group for defeated enemies, for gold or for milestones, according to the campaign, or whenever I want.

- Priority: MVP
- Rules: RN-09, RN-12
- Modules: progression

#### Acceptance criteria
- **Given** a campaign in the by-enemies mode and two defeated goblins (50 XP each), **when** the encounter ends, **then** the 100 XP are divided among the four characters of the group, 25 each. A remainder is rounded down, lost and reported ("2 XP se perdem na divisão").
- **Given** a campaign by enemies, **when** the combat ends, **then** the master uses the NPC's challenge rating (ND) or types the XP (both ways count the same) **and** chooses who receives the split. Only living player characters receive XP.
- **Given** the end of a combat, **when** the summary shows "Experiência do combate", **then** "Dar 116 XP a cada um" gives it, and "Agora não" leaves the line "XP do combate ainda não dado". An encounter's XP cannot be given twice.
- **Given** the NPC editor's "Ao ser derrotado", **when** the master sets the ND, **then** it fills the XP, and "Usar 50 XP" goes back to the table's value. A player never reads an NPC's XP (RN-20).
- **Given** a campaign in the by-milestones mode, **when** the master records a milestone, **then** the characters the master chooses (all the group's living characters come pre-checked) are marked "Pode subir de nível" **and** no XP is counted. The mark goes away when the level rises.
- **Given** the by-enemies or by-gold modes, **when** the master gives XP to the group on their own ("Dar XP"), **then** the XP goes into the sheets **and** the campaign history shows who gave it, when and why.
- **Given** the by-gold mode, **when** the master wants to give XP, **then** they type the gold pieces (1 XP per 1 PO, RN-09); with the map's treasures, "Voltar à cidade" converts into XP what the group found ([MR-041](#mr-041-treasure-and-xp-by-gold)), and typing the PO remains valid.
- **Given** a campaign by milestones with the milestones planned beforehand, **when** the master writes them (up to 120 characters each, at most 100, in any order: move up, move down, edit in place, remove asking in place), **then** only the master sees the list.
  - A milestone reached once and then undone cannot be removed, because its awards stay in the history; the app says so and stops offering "Remover".
  - **When** the group reaches one and the master marks it and chooses who levels up, **then** the chosen ones "Pode subir de nível"; the milestone moves to "Marcos alcançados" with the day, hour and who; the order is not enforced.
  - "Dar a mais alguém" gives the same milestone to a character who was left out, as a new award in the history.
  - "Registrar um marco fora da lista" records a milestone that was not planned.
- **Given** the last award, **when** the master taps "Desfazer", **then** the app asks in place and takes back only the last award; an undone planned milestone goes back to planned (if it was the only mark) or stays reached for the others (if it was a "Dar a mais alguém").
- **Given** a player, **when** they look at milestones, **then** they see only reached milestones and their own character (the line appears only after the first milestone, and the empty state never hints that some are planned). A player cannot write.
- **Given** the campaign page, **when** anyone opens "Experiência", **then** they see the history of awards. The sheet shows XP as a read-only block and "Pode subir de nível", also in the group list.

#### In the app
- The player's campaign page updates when it opens or when the tab returns to the foreground; it does not follow the session live. The master's page does while a session is open.
- See [Architecture](../architecture/progression.md#progression-module-xp-and-milestones).

#### Related
- How XP is awarded: the ND or typed XP, the master decides who receives, rounding down, the level-up notice for master and player, and the history for everyone. See [RN-09](rules.md) and [RN-12](rules.md).
- What each character gains on leveling up comes from the rules engine. See [ADR-0008](../adr/0008-rules-content-as-data-pure-engine.md).

### MR-018: Campaign document

**As a** game master, **I want** a campaign document with text, images, and links to maps and sheets that open in a dialog, **so that** I keep the campaign's notes in one place.

- Priority: MVP
- Rules: —
- Modules: campaigns (the document); maps and characters (the images, maps and sheets the links point to)

#### Acceptance criteria
Only the game master sees the document.

- **Given** the campaign document, **when** the game master writes text, adds an image from the gallery and a link to a map and to a sheet, **then** the document shows the image **and** the link opens the map or the sheet in a dialog, without leaving the document.
- **Given** a player, **when** they request the document, **then** the server refuses; the player sees "Só o mestre vê o documento da campanha", and the campaign page does not show the panel.
- **Given** a non-member or a pending member, **when** they request the document, **then** the campaign does not exist for them.
- **Given** a document, **when** the master saves, **then** it is one document per campaign, in Markdown, up to 200 KiB. If someone saved first, the save is refused as a conflict instead of overwriting.
- **Given** a link to a map, **when** it opens, **then** the dialog shows the whole map with every point (hidden ones marked "Escondido"), the caption, "Mapa atual da Sessão N" when the open session is on that map, and "Abrir no editor de mapas".

#### In the app
- Links: `[text](map:<id>)`, `[text](character:<id>)` and `![caption](image:<id>)`. The server stores the text as sent; the app resolves each link with the usual authorization.
- Screens: `/campaigns/:id/document` and the "Documento da campanha" row on the campaign page's "Preparação" tab. See [Design](../design/session.md#campaign-document-and-gallery) and [Architecture](../architecture/campaigns.md#campaign-document).

### MR-019: Image gallery

**As a** game master, **I want** an image gallery, **so that** I can use the images in documents and maps.

- Priority: MVP
- Rules: RN-10
- Modules: maps

#### Acceptance criteria
Limits: JPEG, PNG or WebP, up to 10 MB per image, 300 images and 500 MB per campaign.

- **Given** the game master in the gallery, **when** they upload a JPEG, PNG or WebP image of up to 10 MB, **then** it appears in the gallery **and** the stored file has none of the original's metadata (EXIF). An image with too many pixels is refused.
- **Given** the upload, **when** the master drops several files or taps "Enviar imagem", **then** they are sent one after the other, each with its own progress and "Cancelar envio". The app checks type and size first, and every refusal becomes a Portuguese notice with the file's name.
- **Given** a player, **when** they request the campaign's gallery, **then** the server refuses and the page says "Só o mestre vê a galeria da campanha."; a non-member sees "Campanha não encontrada".
- **Given** an image used on a map, **when** the game master tries to delete it, **then** the app says which map it is on ("Essa imagem está num mapa. Troque a imagem do mapa antes de apagá-la."); the card shows "Usada em Mirathel e arredores".
- **Given** the gallery, **when** the master uses it, **then** images show from newest to oldest, can be renamed in place and deleted with an in-place confirmation, and open in a dialog with previous and next.
- **Given** a form that needs an image ("Novo mapa", the document editor, "Mostrar imagem"), **when** the master opens the picker, **then** they pick an image or upload a new one and pick it at once.

#### In the app
- `/campaigns/:id/gallery` and the "Galeria" panel on the campaign page's "Preparação" tab ("12 de 200 imagens", the 6 newest images, "Enviar imagem" and "Ver todas as 12 imagens"), game master only. See [Design](../design/session.md#campaign-document-and-gallery) and [Architecture](../architecture/maps/images.md).

### MR-024: Approve the character from the invite

**As a** game master, **I want** to approve or refuse the character a player created through the invite, **so that** only characters that make sense for the table stay in the campaign.

- Priority: MVP
- Rules: RN-15
- Modules: campaigns, characters

#### Acceptance criteria
Each invite chooses whether it requires approval, and a refusal deletes the character and the membership.

- **Given** I am the game master of "Mirathel", **when** I generate an invite, **then** I can tick "Exigir aprovação do mestre" **and**, unticked, whoever accepts joins at once.
- **Given** an invite that requires approval, **when** the player accepts it (already signed in, or signing in through the invite), **then** they go straight to creating the character, which is born "Pendente de aprovação" **and**, while they wait, they see only the campaign's name and their own character, which they keep editing.
- **Given** a pending character, **when** the game master opens the campaign, **then** they see it under "Esperando aprovação" and can open the sheet **and**, **when** they approve, the character becomes a draft and the player becomes a player of the campaign.
- **Given** a pending character, **when** the game master refuses it, **then** the character is deleted, the player does not join the campaign **and** needs a new invite to try again.
- **Given** I am a player, or a pending player, **when** I try to approve or refuse a character, **then** the server refuses.
- **Given** a pending player who has not created the character yet, **when** the game master opens the campaign, **then** they see who is pending without a character, with a button to remove them; **and**, after 30 days without a character, the pending membership is deleted automatically.
- **Given** a pending character, **when** the game master taps "Pedir ajustes" and writes what to change (1 to 500 characters, required), **then** the character stays pending, the invite stays valid **and** the player reads the reason on the sheet, edits it and taps "Enviar de novo".
  The game master can approve or refuse at any time, and asking again replaces the reason.
  Only the game master and the owner read the reason.
  It is deleted when the character is approved or refused, or when the character, the campaign or the account is deleted.
- **Given** a pending player of "Mirathel", **when** they accept a plain invite (no approval) to the same campaign, **then** they become a player at once, because a plain invite counts as the game master's approval.
  A character they had waiting is approved too, and the invite spends one use.
  An invite that requires approval, or that is no longer valid, changes nothing.

#### In the app
- A pending member can do only what the flow needs; to everything else the server answers as if the campaign did not exist for them.
- The campaign shows "Esperando a aprovação do mestre" to the pending player.
  The game master's "Personagens" section lists "Esperando aprovação", and the sheet has "Aprovar personagem" and "Recusar personagem" (confirmed with "Confirmar recusa").
- Under "Membros", each person without a character shows "Sem personagem", the join date and the date they leave on their own; "Remover" asks in place, with focus on "Cancelar".
  If the person created the character meanwhile, the list refreshes and points to the pending characters.
  Only the game master sees this.
- See [RN-15](rules.md), [Architecture](../architecture/campaigns.md#pending-member) and [Data model](../data/README.md#modules-and-tables).

#### Related
- Extends [MR-003](#mr-003-join-through-the-invite): the character is born pending approval (see [Character lifecycle](rules.md#character-lifecycle), RN-01).
- Extends [MR-002](#mr-002-generate-an-invite): the game master chooses, per invite, whether it requires approval.

### MR-028: Show an image to the players

**As a** game master, **I want** to show the players an image from the gallery during the session, with an action of its own, separate from the current map, **so that** I can present a portrait, a letter or a scene.

- Priority: MVP
- Rules: RN-10
- Modules: play, maps

#### Acceptance criteria
- **Given** an open session, **when** the game master shows an image from the gallery, **then** everyone in the session sees it at once, without reloading, **and** the current map is still there; **when** the game master stops showing it, the image disappears from the players' screens.
- **Given** the session ended or the game master stopped showing the image, **then** the players no longer receive the image's ID, and a player cannot download it.
- **Given** an image the game master wants the players to keep seeing, **when** they turn on "Deixar com os jogadores" and then stop showing or swap the image, **then** it stays with the players, in an "Imagens que o mestre deixou" list on their session page, until the game master removes it ("Tirar"); the list belongs to the campaign and survives the end of the session.
- **Given** a player, **when** they try to show an image, or the game master tries one from another campaign or without an open session, **then** the server refuses.
- **Given** a shown image, **when** the game master looks at who sees it ("Quem vê a imagem"), **then** it has one audience of three kinds: the whole table; the scene's players; or chosen players, any set of the campaign's players, picked like the conversation's "Para". The audience is the whole table when no scene is open or the open scene is open to everyone, exactly as before; while a scene open to only some players is open, the default is the scene's players, **and** the game master may pick the whole table or chosen players when they show the image.
- **Given** an image shown to the scene's players, **when** the game master changes who is in the scene, **then** a player who comes in sees the image at once and a player who goes out stops seeing it at once; **when** the scene closes, another one opens, or the scene opens to everyone, **then** the image stays with the players who were in at that moment, until the game master stops showing it or shows another: it reaches no one else, and no player gets it retroactively.
- **Given** a shown image, **when** the game master taps "Mostrar a toda a mesa", **then** every player sees it at once, as an image shown with no scene; **when** they tap "Mudar quem vê" and pick the players, **then** the ones picked see it, the others lose it at once (the image goes from their screen, from the live session read and from the image route, which answers 404) and the players who stay hear nothing.
- **Given** the image on screen is deleted from the gallery, **then** only the game master and the players who saw it hear that it stopped being shown; a player outside its audience hears nothing, and the lists of left images they read are unchanged (RN-10).
- **Given** a player outside the image's audience, **then** they never receive it: not the image, its ID or its name in a read, not in a live event (they only hear that an image they saw stopped being shown), not through the image route, not in the summary or the highlights (RN-10).
- **Given** an image with "Deixar com os jogadores" on, **when** it stops being shown, **then** it stays only with the players who were in its audience (the whole table, or those players); the list the other players read does not have it, the image route answers them 404 and their screens hear nothing of it; "Tirar" works as before. Leaving an image again, with another audience, keeps the widest: whoever already had it keeps it and the new players get it too; only "Tirar" (taking it back) narrows it. A "Mudar quem vê" for the list of left images is for later: the audience of a left image does not change until it is taken back.

#### In the app
- One image at a time, independent of the current map; a new session starts with none.
  Deleting the image from the gallery stops showing it and removes it from the list.
  The image's name is the uploaded file's name and is shown to the players as the caption, so the game master can rename it before showing it.
- In the session the master chooses who sees it. With the scene open to the whole table a tap shows the image to the table; with it open to only some players "Mostrar" on a thumbnail is a menu ("Só quem está na cena", "Toda a mesa", "Escolher quem vê…"). Under the thumbnails a line says who sees the image on show ("Pensantus e Brisa veem", "Só Brisa vê", "Toda a mesa vê", "Os jogadores veem"), with "Mostrar a toda a mesa" and "Mudar quem vê", which opens the chooser ("Quem vê “Vista do salão”?") and takes the image away at once from whoever is left out.
- Showing the background of a map with fog on shows a copy of it ("… (névoa)"), so the fog is not given away.
- A player downloads the image only while it is shown and they are in its audience, while it is left with them, or while it is the background of a map they can see.
  Otherwise it is a `404`, even with the ID stored.
  See [RN-10](rules.md) and [Serving images](../architecture/maps/images.md#serving-images).
- Game master screen: with a scene open, the scene's "Imagens da cena" row (the dashed "Da galeria" tile, the "Deixar com os jogadores" switch with its state word on one line, "Parar de mostrar" and the list of images left); with no scene open, the "Imagem para os jogadores" panel ("Mostrar imagem", the gallery picker with "Mostrar aos jogadores", "Trocar imagem", "Parar de mostrar"), the "Deixar com os jogadores" switch ("Ligado"/"Desligado", off whenever a new image is shown) and the "Deixadas com os jogadores" list with "Tirar".
- Player screen: the "O mestre está mostrando" block with the image, its name as caption and "Ver em tela cheia", first on the page on a phone and at most 40 % of the screen high, which appears and disappears live and is announced; and the "Imagens que o mestre deixou (n)" thumbnails, each opening the same full-screen view ("Ver <nome> em tela cheia"), a fold of one row on a phone (the "Imagens" tab of the scene's card while a scene is open) and open beside the scene on a computer, hidden while empty.
  The list is on the session page only, not on the campaign page.
  See [Design](../design/session.md#images-shown-to-players) and [Architecture](../architecture/play/live-session.md#what-the-session-shows).
- The audience is part of the show call and has its own call to change it while the image is shown (`SetShownImage`, `SetShownImageAudience`); the game master reads it on the image, and a player never does.
- Out of scope: a per-player "chest" of every image already shown.
  The list holds only what the game master chose to leave, and a player reads the images left with them.

### MR-025: Register table content

**As a** game master, **I want** to register races, classes, subclasses, backgrounds, spells and rules that are not in the SRD 5.1, and to set my table's rules, **so that** the campaign uses the material my table plays and I have full control of the product.

- Priority: MVP
- Rules: RN-23, RN-24, RN-25
- Modules: rules, characters, campaigns, play, maps

The story covers own classes **and** subclasses, races and subraces, backgrounds, spells, feats, options, class features, creatures, the content pack (export and import), the option switches, the table rules and the campaign's grid.
A player's proposal (MR-026) and reading rules from a PDF (MR-027) are separate, later stories.
The data rules, limits and the server side are in [Table content: rules](../architecture/rules-table-content.md) and [Table content: characters](../architecture/characters-table-content.md).

#### Acceptance criteria

**Own content.**

- **Given** I am the game master of "Mirathel", **when** I register a new class with its hit die, skills, the 20-level table and its features, **then** the class appears in the character editor and in the level-up only in "Mirathel" **and** the sheet calculates its numbers with it (RN-23).
- **Given** I am the game master of "Mirathel", **when** I register a subclass of an SRD class (a Bard college, a Wizard tradition), **then** it appears in that class's subclass choice only in "Mirathel", with its always-prepared spells, if it has any.
- **Given** I register a race, a subrace or a background, **then** it appears in the character editor with everything the editor shows (size, speed, darkvision, "+2 e +1 à escolha", languages, traits, skills, tools, equipment and the feature) **and** a table background's equipment reaches the sheet.
  A race may offer **alternative sets** of ability bonuses ("+2 e +1 ou +1, +1 e +1": I write them in "Valores à escolha", separated by a slash) **and** a **size to pick** (Pequeno or Médio, with the race's own size among them): the character editor shows the sets in the hint under "Raça" and asks "Tamanho" only when the race really offers two; the player picks the abilities in "Escolhas", as for the half-elf: the set (when there are several) and, for each bonus, the abilities that get it, never the same ability twice, and the scores follow the picks; the same picker serves a race with a single "+2 e +1". A new player's sheet cannot be saved before the picks are made, and a sheet that placed the bonuses in "Bônus manuais" before they were picks keeps them.
  The server refuses a bonus above +2 in a set, more than six bonuses, a repeated size and a race that has both a single list and sets.
- **Given** I am the game master of "Mirathel", **when** I register an own spell, **then** it appears for the characters of "Mirathel" who can learn it **and** not in another campaign.
  If it has an attack, or a saving throw and damage, combat resolves it like an SRD spell: the attack, the save with half, healing, several creatures (one more per level), an area, the cantrip by tier, concentration and slots.
- **Given** I register an area spell (a 4.5 m cone, a 6 m radius sphere), **when** I save it, **then** the area appears in its description, **and** in combat the caster chooses the creatures it hits, as with an SRD area spell.
  A spell with range "Pessoal" or "Toque" is valid; "Pessoal" with "one creature" or "several" is refused.
- **Given** content registered in "Mirathel", **when** I open another campaign of mine, **then** it does not appear there: content is per campaign.
- **Given** a feature the game master registers, **when** they pick its effect, **then** they pick from a closed menu (modifier, proficiency, resource, sense, roll mode, action, extra attack, ranged attack, resistance, immunity to a condition, state, weapon rule, weapon of the feature, trigger on an event, choice, maneuver die, note, granted spell).
  An action may spend a resource the entry defines (a feat's action, one its own effects define: a table feat with 3 uses per long rest), and a class or subclass feature may carry an **option list** the player picks from (a table's maneuvers, each with its own effects; "Opções para escolher" in the feature editor).
  Table content never runs code.
- **Given** a table class that casts from a list of its own, **when** I name SRD spells on that list in the class editor ("Magias do SRD na lista da classe"), **then** they join the table spells that name the class: the character prepares and learns them, the level-up offers them and the "Magias" page lists them for the class.
  The server refuses an unknown spell, a table spell, a repeated one and any of them on a class that reuses another class's list.
- **Given** a table subclass that names spells, **when** I mark "As magias da subclasse entram na lista da classe, para escolher", **then** those spells are not prepared for free: they join the class's list for a character of that subclass, who picks them like any other spell of the list, and they count against the spells known or prepared.
  Without the mark they are always prepared. The mark is offered only when the subclass has spells, and the server refuses it otherwise.
- **Given** a feature with a proficiency effect of level "expertise" on a skill, **when** I turn on "Só onde já tem proficiência", **then** the sheet doubles the skill only for a character who is proficient in it from another source, and gives no proficiency by itself.
  The SRD's own expertise works as before.
- **Given** a table feature or feat with an action that spends a resource, **when** I set its cost, **then** each use spends that many (a feature that spends 2 of a pool for a bonus action), the action is off while fewer are left, **and** the server refuses a cost the resource's maximum can never pay, a maximum cost below the cost, and a cost with no resource.
  With a maximum cost the player chooses at use how many to spend, from the cost to the maximum or what is left ("quantos?"); the server reads the uses left and checks the number in the same transaction as the use.
  The action card shows the cost and, with a maximum cost, "Usar" opens a sheet with "Quantos?" (a stepper from the cost to the maximum or what is left), "Para quem" (the player or one other player character, for an action that heals or gives temporary hit points), a line of what the choice will cost and give ("Gasta 3 · cura 3d6") and the roll of the dice; with fewer uses left than the cost the row is off with "Faltam usos".
- **Given** an action that heals or gives temporary hit points (a feature that spends 1 to 3 points to heal 1d6 per point; a feat that gives temporary hit points equal to the level), **when** the player uses it, **then** it heals the dice plus the ability modifier I chose, and each point spent above the cost adds the value per point, to the owner or to one creature picked at use (the owner and one ally).
  The rules are those of the spells: temporary hit points do not stack (the larger is kept), healing never passes the maximum and a dead character is not healed.
  A heal with no dice is refused.
- **Given** a resource, **when** I pick how it comes back, **then** besides a short rest, a long rest, dawn and never, it may return one use when its owner rolls initiative (only when none is left), one use on a short rest (and all on a long rest), or one use on a long rest.
- **Given** a feature or feat with a trigger on an event (after a short or long rest, when a death save fails, when the owner drops to 0 hit points), **when** the event happens to its owner, **then** the trigger gives temporary hit points, heals dice or a share of the maximum, restores uses of a resource of the entry, and may spend a resource to count at all ("once per long rest, stand up with half the maximum when a death save fails"); a rest finished twice never restores a use twice.
  Only the owner's triggers run: an effect on allies who rest nearby stays text.
  A trigger on a failed death save runs on each failed roll (a natural 1, which counts two failures, runs it once) before the failure is added, so one that stands the character up keeps the third failure from killing it and clears the counts; without the resource it does not run and the failure counts.
  A trigger on dropping to 0 hit points answers damage (an attack, a spell or a trap, in a combat or outside one) and not the master's "Ajustar PV", nor damage that kills outright, and never raises a character already dead or dying; the temporary hit points of a long-rest trigger stay after the rest ends the old ones.
  What a trigger gives appears in the line the rest, the death save or the damage already writes, with the audience of that line, and an undo gives back the uses it spent.
  The server refuses an unknown event, a trigger with no result, and a resource the entry does not define.
- **Given** a table class with a spellcasting subclass (a third-caster), **when** I register the subclass with "Esta subclasse conjura", **then** the third-caster table starts at the level it begins, and the subclass's spells come from the class list it casts from.

**Feats.**

- **Given** I am the game master of "Mirathel", **when** I register a feat with its prerequisite (ability scores — all of them or one of them —, a proficiency, "can cast at least one spell", a race or subrace, or a level) and effects from the closed menu, **then** the feat appears in the content list like the other kinds, in "Opções para os jogadores" and in the level-up (if the table uses feats), **and** the sheet of a character who took it lists it as "Talento" with its effects.
  A feat whose prerequisite the character no longer meets stays on the sheet with a notice and does not apply until the prerequisite is back.
- **Given** the "Regras da mesa" rule "Talentos" ("permitidos" or "não usados", default "não usados"), **when** it is "não usados", **then** the level-up offers only the ability score improvement; **when** it is "permitidos", **then** that step offers "Aumentar habilidades" or "Um talento", with the feats the character qualifies for (the server checks the prerequisite) and, for the others, the reason in Portuguese.
- **Given** a feat that gives "+1 to one of two abilities" (or a fixed ability), **when** the player takes it at the level-up, **then** the chosen ability goes up by 1 and never above 20, and the server refuses any other increase.
  A feat that raises an ability already at 20 is still available: the increase is lost and its other effects apply.
  The summary says "Talento: <name>" with its increase, and the sheet lists the feat as "Talento · <class> <level>" instead of the improvement it replaced.
- **Given** a trait, feature, background feature or feat that grants a feat to choose (a `choice` of kind "Talentos"), **when** the player reaches it, **then** the choice is asked where choices are asked (the editor, the pending choices, the level-up), the feat applies while the owner is on the sheet, a feat already taken is not offered again, and the rule "Talentos" does not gate it.
- **Given** I am the game master and open a character's sheet editor, **when** I look at the "Talentos" section, **then** I see the sheet's feats, each with "Remover", and "Adicionar talento" with the campaign's feats (the SRD's and the table's, without the retired ones).
  - When I add one, the save keeps it and its effects reach the sheet, also when the rule "Talentos" is off (the section says so).
  - When the character does not meet the prerequisite, the editor warns me and I may keep it; the sheet then lists the feat without effect, with a notice.
  - When I remove a feat taken in place of an improvement, the improvement comes back.
  - A feat that raises abilities does not ask which ones: I give the increase in "Bônus manuais".
- **Given** I am a player, **then** the editor has no such section and the server refuses a save or a new sheet whose feats differ from the stored ones: a player's feats come from the level-up.
- **Given** a table feat with a ranged attack effect ("Ataque à distância"), **when** its owner attacks in a fight, **then** the reach, the target lists, the roll's armor class and the long range follow it.
  For spells that need an attack roll, the range is twice or three times as long and half or three-quarters cover is ignored.
  For ranged weapons (a thrown weapon at range included), that cover is ignored and long range gives no disadvantage.
  Total cover still blocks the attack, and the roll shows no cover where the cover was ignored.
- **Given** a table race, feat or feature with a **resistance** effect, **when** I pick it, **then** I choose the damage types from the SRD's list (or "Todos os tipos de dano", or the nonmagical bludgeoning, piercing and slashing) **and** the kind on three radio cards: resistance, immunity or vulnerability; "Todos os tipos de dano, menos" and "Enquanto" (always, or only in a rage) are the rarer fields under "Mais opções".
  In a fight the damage that character takes follows it: immunity takes it to 0, resistance halves it (rounded down), vulnerability doubles it (after the resistance), the same protection from two sources counts once, and a limit to nonmagical attacks covers a weapon attack that is not marked magical, never a spell or damage that is no attack.
  The master reads the step in the damage line; the sheet lists the effect as a note.
  A refusal names the field: a damage type that does not exist, an empty list, both "menos" and a list, "all" mixed with other types.
- **Given** a table entry with **immunity to a condition** (one or more of the 15 conditions), **when** a spell, a rider, a trap or a hold would put one of them on that character, **then** it takes none of them: the spell's hit reads "o alvo é imune à condição" for the master, a grapple fails and a shove does not knock prone.
- **Given** a table roll mode "Vantagem ou desvantagem" with a **condition** picked ("Só nos testes de resistência contra a condição"), **when** the character rolls a saving throw against a spell, an effect, a trap or a rider that gives that condition, **then** the roll has the advantage or disadvantage and the roll's reasons say so; with no condition picked, the effect stays a reminder of the situation (its tags) and the master decides.
- **Given** a table feature or feat with an extra damage effect ("Dano extra") that has dice (for example, "once per turn, +1d6 fire on a melee weapon hit"), **when** its owner hits with a weapon it fits, **then** the damage card offers it as an extra to mark, and the dice are rolled beside the weapon's (in the app, or with the physical dice the player types).
  The damage is a line of its own type (the feature's, or the weapon's when it names none), so a resistance to that type takes only that line.
  A critical hit doubles the dice unless the feature says otherwise; a fixed number is never doubled.
  The extra is off, with its reason, on the other kind of weapon (melee or ranged), against a target that is not larger when the feature asks for a larger one, when it was already used this turn (the default is once per turn; "em todo acerto" has no such limit) and when its resource has no use left.
- **Given** a table feature or feat with a cast option ("Opção de magia") that fits a spell (for example, "evocation spells: +2 cold damage, 2 uses per long rest"; "fire or cold damage: swap one for the other"; "one more die per slot level above the spell's own"), **when** its owner opens the cast sheet of a spell it fits, **then** the sheet lists the option; the ones that add by themselves are shown as always on, the others are ticked, and a type swap asks for the new damage type among the types the spell can take.
  The cast rolls the damage with the option: the fixed damage is a line of its own type when the type differs from the spell's, the swap changes the type of the damage, and the extra dice come with the slot chosen.
  An option with a resource spends one use with the slot; with no use left it is off, with the reason, and a cast that ticks it is refused and spends nothing. The master's undo gives the use back.
- **Given** a table option with a die on the character's own rolls (for example, "a d6 on Perception checks", "a d6 on Constitution saves", "a d4 on the initiative", from a pool of two uses), **when** its owner rolls a scene check, a concentration save, an effect save or the initiative the die fits, **then** the roll offers the die; naming it rolls it with the d20 (in the app, or the face typed with physical dice), adds it to the total, shows it by name and spends one use.
  A die that does not fit the roll (another skill or ability), one the character does not have, and one with no use left are refused and spend nothing. A number that is always on is a modifier of the feature, not a die.

**Speeds and spells the table grants.**

- **Given** a feature, a trait or a feat of the table with a modifier of the fly, swim, climb or burrow speed ("Deslocamento de voo", "de natação", "de escalada", "de escavação"; "somar", "pelo menos" or "definir"), **when** a character has it, **then** the sheet lists that speed under the walking speed in meters and feet, **and** it follows the effect's condition (`when`).
  The formula may read `speedwalk()`, the walking speed after the modifiers: a race that swims as fast as it walks writes "definir `speedwalk()`", and a feat that adds 10 ft to the walking speed moves the swimming speed along.
  In combat a creature moves on its fly speed when it is the better of the two; swimming and climbing are on the sheet only, as the grid has no cost for them.
- **Given** an effect "Respira na água", **when** a character has it, **then** the sheet says "Respira na água" in the speed box; the app keeps no air or drowning count.
- **Given** a feat, a trait or a feature with the effect "Magia concedida" (a spell of the SRD or of the table, the level it starts at, the casting ability — Intelligence, Wisdom, Charisma or "o jogador escolhe" —, the resource each cast spends, a switch to cast it with a slot too, a switch for rituals only and a switch to list it among the known spells), **when** a character has it, **then** the sheet lists the spell under "Magias concedidas" with its source, how it is cast and the DC and attack bonus of the grant, **and** the character's known spells, cantrips and prepared numbers stay as they were.
  A leveled spell is cast at its own level, a cantrip scales with the character level, and the material components are not waived.
  When the ability is "o jogador escolhe", the player picks it once, in the sheet's choices, and until then the grant has no numbers.
- **Given** a granted spell, **when** the player casts it, in a fight or outside one, **then** the cast sheet offers it as a way of its own, with no spell slot (a slot only when the grant allows one), using the grant's ability for the attack roll and the saving throw DC.
  A limited grant spends one use of its resource: the second cast with no use left is refused ("A concessão não tem mais usos."), the undo of the first gives it back, and an at-will grant never runs out.
  A ritual-only grant casts as a ritual.
- **Given** the effect editor, **when** I register a granted spell, **then** the server refuses a spell that does not exist, a ritual-only grant on a spell without the ritual tag, a resource the entry does not define and an ability that is none of the four, each at its field.

**Options for the SRD's option sets.**

- **Given** I am the game master of "Mirathel", **when** I register an option for one of the SRD's option sets (an Eldritch Invocation, a Metamagic option, a fighting style, a Pact Boon, a Hunter's Prey, Defensive Tactics, Multiattack or Superior Hunter's Defense) or for the option list of a table class or subclass feature (a table's maneuvers) with its text, its effects from the closed menu and what it asks for (a level in a class, other options, a Pact Boon, a spell the character knows), **then** the choice of that set offers it, in "Mirathel" only, next to the SRD's options, **and** a set with many table options is not cut at 20, **and** an option for a table feature joins the options that feature lists, with the same prerequisites.
- **Given** a table option, **when** a character does not meet what it asks for, **then** the choice lists it blocked with the reason ("Exige o nível 5 de Bruxo. Você está no 3.", "Exige o Pacto do Tomo"), **and** the server refuses a sheet that picks it anyway; **when** the character meets it, **then** the player picks it and its effects (and its action, if it has one) are on the sheet.
- **Given** the "Opções para os jogadores" screen, **when** the master switches a table option off, **then** players are not offered it and the server refuses it as a new pick; a sheet that already has it keeps it.
- **Given** the option editor, **when** I save an option with an unknown set, a class or option or Pact Boon or spell that does not exist, a level outside 1 to 20, or another choice among its effects, **then** the server refuses each at its field, **and** the set of an option never changes after it is created.

**Wild Shape and spell slots.**

- **Given** a table feature that changes Wild Shape, **when** a druid with it opens the list of forms, **then** the list and the choice use the highest challenge rating, the lowest cost in uses and magical attacks if any of its features gives them; a beast above the rating is refused ("WILD_SHAPE_BEAST_NOT_ALLOWED").
  A cost of 0 spends no use, and the undo gives back exactly what the change spent.
- **Given** a table action that turns uses into a spell slot, **when** a character takes it, **then** its uses are spent and a free slot of that level exists until the next long rest; the combat log reads the uses and the slot's level, and the undo takes the slot away and gives the uses back.
- **Given** a table action that spends a spell slot to heal, **when** the player picks a level, **then** that slot is expended and the heal grows by the dice of each level above the first; a level with no free slot is refused and nothing is spent.
- **Given** a table feature that recovers spell slots, **when** the character recovers slots after a short rest, **then** one use of its resource is spent and the slots come back; a slot above its highest level, a combined level above its limit, a rest that is not short and a resource with no use left are refused and nothing changes.

**Class features.**

- **Given** I am the game master of "Mirathel", **when** I register a feature for a class (the SRD's or one of the table) at a class level, with its text and effects, **then** every character of that class reads it from that level on, in "Mirathel" only, like any feature of the class.
- **Given** a class feature that replaces another feature of the class (an invented Ranger feature at level 1 that replaces Favored Enemy), **then** characters of the class no longer have the replaced feature from that level on, **and** the replaced feature is not asked for in their choices; **when** I switch the entry off, **then** the class reads as before and the replaced feature is back.
- **Given** the editor, **when** I save a class feature with a class that does not exist, a level outside 1 to 20, or a replaced feature the class does not have at that level or later (or that it already had before), **then** the server refuses each at its field.

**Creatures.**

- **Given** I am the game master of "Mirathel", **when** I register a creature of my own from an SRD creature (a wolf with 40 hit points, armor class 17, +2 on each attack's damage and a trait of text), **then** it resolves where an SRD creature does: I give it to a player's character, it joins the combats with those numbers, and I can make an NPC from it.
  The SRD creature itself never changes, and the entry's base cannot be swapped for another one later.
  The web app has no editor for creatures: they are created through the API or a content pack.
- **Given** I save a creature with a base the SRD does not have, hit points outside 1 to 9,999, armor class outside 1 to 40, a damage bonus outside -10 to 30, or more than 10 traits, **then** the server refuses each at its field, and a trait named like one of the base's traits or actions, or "Magic Weapons", "Spellcasting" or "Innate Spellcasting", is refused.
- **Given** a creature with no formula already given to a character, **when** I edit the entry's hit points and armor class, **then** the creature keeps the hit points it was given **and** its armor class and its attacks follow the entry.
- **Given** a creature whose hit points are `5 * level()` and whose armor class is `10 + prof() + mod("dex")`, **when** I give it to a level 3 wizard and to a level 5 fighter, **then** each has the hit points, armor class and attack numbers of its own owner.
- **Given** a creature given with formulas, **when** its owner gains a level or I edit the formulas, **then** the creature already given keeps its numbers, and one given afterwards has the new ones.
- **Given** a creature given with formulas, **when** I add, remove, rename or reorder its own attacks, **then** the attacks that stay keep the numbers they were given, a new attack has the entry's static numbers (a renamed attack is treated as new, because an attack is known by its name, so it gets the entry's static numbers too), a removed one is gone, and the creature's hit points, armor class and bonuses do not change.
- **Given** a creature with more than 8 attacks of its own, **then** the server refuses it (`limit` at `table_creature.attacks`; `TestACreatureFormulaRefusalsNameTheirField`).
- **Given** a creature whose third attack has a to-hit formula that does not work, **then** the refusal names that field (`table_creature.attacks[2].to_hit`, `TestMR578_TheFormulaFieldsOfACreatureHaveTheirPaths`).
- **Given** a formula that calls `slot()` in a class, a feature, a spell damage or any field other than a creature's or a summon's count, **then** the server refuses it (`TestSlotIsLegalOnlyInACreature`).
- **Given** I register a spell with a summon (a level 2 spell for the caster alone that brings "Lobo de Prata" and a wolf, with a count of `1 + floor(slot() / 2)` and one more creature for each circle above the spell's own), **when** a player casts it with a slot, **then** the creatures join the character, the combat's initiative and the map, the owner plays them in the tabs of any summon, and a creature of a formula has the numbers of the owner and of the slot used.
  The spell goes through the checks of the SRD's summoning spells: the slot, concentration (a new casting dismisses the creatures of the old one), the replacement of its own creatures when I ask for it, the limit of creatures kept at a time, the dismissal and the undo, which gives the slot back and removes the creatures.
  The creatures may take one initiative roll for the casting or one each.
  The server refuses acting right after the owner, a mount, a spell paid with a resource, a ritual, a plain count above the limit of creatures kept, an unknown creature, a repeated one, more than 8 creatures, a count that does not work and a spell that is not for the caster alone.
  In a combat, creatures that each roll their own initiative need the app's roller, because one typed die is one face.
  Players read the creatures as for any summon: the name and key in the combat, the numbers only the owner and I.
  A player who holds none of the creatures reads the spell in the table's list without the table creatures they do not hold (the SRD's stay), and without its count and limit; the caster reads the creatures she may pick when she casts (`TestATableSpellThatSummonsKeepsItsCreatureAndCountFromThoseWhoHoldNone`).
  (`TestATableSpellSummonsThroughTheSRDChecks`, `TestATableSummonIsRefusedAtTheFieldThatIsWrong`, `TestMR578_ATableSpellSummonsItsCreaturesOutsideACombat`, `TestAFormulaCreatureThatAttacksKeepsItsNumbersFromAThirdPlayer`)
- **Given** a creature given with formulas, **when** I edit its entry to have no formula at all, **then** its armor class in the list and in the combat are still the ones it was given.
- **Given** a creature with formulas that I put in a combat with "Pôr no combate" or make an NPC from, **then** it has the entry's static numbers, because it has no owner.
- **Given** a creature entry, **then** a player finds it in the creature list and reads its sheet only while one of their characters holds it.
  This holds whether the entry is live, archived or switched off.
  The master reads every entry.
  Archiving or switching it off changes nothing for the creatures already given, and nothing is deleted.
- **Given** a monster of a table creature in a combat, **when** I edit its entry and put it in the combat again, **then** the new combatant has the entry's current armor class and attacks.
  The monster already there keeps the numbers it joined with.
- **Given** a table creature with no formula that a character holds, in a combat, **when** I edit its entry, **then** its armor class and attacks follow the entry at once.
  Its hit points stay the ones it was given.
- **Given** a creature of the table in a combat, **then** its hit points are a number only for the master and its owner's player, and its armor class only for the master; the other players read its state word.
- **Given** a table creature in a combat, **then** every player there reads its name and key, as for an SRD summon or an NPC.
  Its stat block, hit points and armor class stay hidden from a player who does not own it.
  This holds even when the entry is archived or switched off.
- **Given** an archived creature entry, **when** I give it to a character, **then** it is given.
- **Given** a creature entry, **when** I switch it on or off, **then** I do it through the API, because the "Opções para os jogadores" screen has no creature group.
- **Given** the summoning spells, Wild Shape and the encounter tools (generator, evaluator and swaps), **then** they offer SRD creatures only, because a table creature does not keep its base's challenge rating.

**Swapping a pick.**

- **Given** the table rule "Trocar uma escolha no aumento de atributo" is on and a Fighter reaches level 4, **when** the level-up reaches the ability step, **then** it also offers to give up one earlier pick of one group (cantrip, fighting style, metamagic option, invocation, pact boon) for another of the same pool, with "Não trocar" as the default, **and** the sheet shows the new pick and not the old one; the new pick still needs the prerequisites of any pick.
- **Given** the rule is off (the default), or the level has no Ability Score Improvement, **then** the level-up offers no swap and the server refuses one (`SWAP_NOT_ALLOWED`).

**Changing, archiving and switching.**

- **Given** a table class that a sheet uses, **when** the game master changes one of its numbers, **then** the sheet shows the new number, locked sheets included, **and** whatever fell outside the rules appears on the sheet as a notice ("A classe mudou") with one sentence per change, without blocking any other edit (RN-23).
  The notice clears itself when the numbers match again.
- **Given** table content that a sheet uses, **when** the game master archives it, **then** the sheet keeps it **and** it no longer appears as a new choice; nothing in the table content is ever deleted.
  An archived entry can still be edited and can be unarchived, and then it is a new choice again.
- **Given** the "Opções para os jogadores" screen, **when** the game master switches off a class, subclass, race, subrace, background, spell, feat, option, or class feature (SRD or table), **then** players no longer see or pick it; what is on, they see in full, with numbers and effects.
  A switched-off class takes its subclasses with it, and a switched-off race its subraces.
- **Given** a switched-off or archived option, **when** a player creates or edits a sheet or levels up, **then** the server refuses it as a new choice, **and** a sheet that already has it keeps working and an unrelated edit is never refused.
  The game master is never refused.
- **Given** a write to the table content or a switch, **when** a session is open, **then** the screens that show the table (the content list, the options, "Magias", the character editor, the level-up and the open sheet) update by themselves.

**Registering well.**

- **Given** I save an entry that breaks a rule, **then** the server lists every violation at once, each at its field, and nothing is saved.
  An entry is limited to 60 features, and a campaign to 1,000 entries and 6,000 effects; a formula is limited to 1,024 bytes.
- **Given** a player, **then** they read every playable entry in full, with "Da mesa", and never an archived or switched-off one, not even by reference, unless their own sheet uses it.
  They never read counts or state, and cannot write.

**Packs.**

- **Given** content registered in "Mirathel", **when** I press "Exportar" on the content list, **then** the app downloads `<campanha>-conteudo.json`; **when** I choose that file in another campaign with "Importar pacote", **then** I see the preview (new, updated, unchanged and refused entries, with the reason of each refusal) **and** "Importar N entradas" writes all of it or nothing.
  An entry with the same key is updated keeping its features' keys, its archive mark and its switch; a new entry is on for the players.
  A retry of the same import writes once.
  The pack is untrusted data, read strictly and checked by the same rules as the editors; the limits are in [Content packs](../architecture/characters-table-content.md#content-packs-exporttablecontent-importtablecontent).
  Only the game master exports and imports.

**Character creation.**

- **Given** a new character entering at the party's level, **when** the player creates it with more than one class (Rafa creates Corvina, Wizard 3 and Cleric 1, at level 4), **then** the editor has one block per class, with the level and the subclass of each (the table's included), **and** the server checks the multiclass prerequisites, as in the level-up.
  Prerequisites, hit points and slots come from the server as notices and numbers.
- **Given** a character with a free-text background (the editor's "Outro"), **when** the player fills it in, **then** they choose, as in the SRD 5.1 rule "Customizing a Background", two skills, two tools or languages in total, one feature (with their own text) and the equipment, **and** the sheet calculates with it.
  What is missing is a notice, never an error.

**Table rules (RN-24).**

- **Given** I am the game master of "Mirathel", **when** I choose on the "Regras da mesa" page how hit points are gained, how ability scores are made, the critical hit and who sees the death saving throws, **then** the server follows the choice in every sheet and every combat; the standard array, point buy and 4d6 appear with the label "SRD 5.2.1 (regras de 2024)".
- **Given** the rule for hit points on level-up, **then** the "roll" rule refuses the average, the "average" rule refuses the die, and the default accepts both; the level-up shows no choice when the rule fixes the way.
- **Given** the ability-score methods the table allows (standard array, point buy with "Restam N pontos", server 4d6, typing 3 to 18), **when** a player makes a new sheet, **then** the server accepts only those methods.
  NPCs and the game master's edits stay free, and a pending player makes their scores too.
- **Given** a campaign that allows 4d6 drop lowest, **when** the player rolls the ability scores of a new sheet, **then** the server rolls and stores them, and rolling again returns the same, until one sheet spends them.
  With physical dice, the player types them once and the campaign's dice rule applies.
- **Given** the critical hit rule ("dados dobrados" or "máximo mais uma rolagem"), **then** it applies to every critical hit: weapon, spell, creature and NPC, with a map or in theatre of the mind.
  With physical dice, the damage step says what to roll ("role os dados duas vezes" or "o máximo mais uma rolagem"), and the total shown before confirming is what the server stores.
- **Given** the rule "Os jogadores veem a CD dos testes de resistência" (off by default), **when** it is off, **then** the player whose creature rolls a saving throw (a zone, an effect at the start or end of a turn, a spell cast by an NPC or by another character, a trap, the Constitution save of a concentration, the Dexterity save against a Hellish Rebuke an NPC casts on them) reads the roll and whether it passed, never the DC; only the master reads it, and the caster's player the DC of their own spell.
  **When** the master turns it on, **then** that player reads the DC of the saves their creatures roll, in the question, the answer, the effect card, the reminder on the damage that threatens a concentration and the log, from the next one on ("CD 14"); another player, who does not roll, still never reads it, and a hidden caster or trap stays hidden.
- **Given** the death saving throws are hidden, **then** the other players read only "Caído", "Estável" or "Morto", with no marks or counts, and the log says "Esta mesa só deixa o dono e o mestre verem os testes contra a morte".
  The owner and the game master see the marks, labelled "Só você e o mestre" and "Dono e mestre".
- **Given** the XP mode, **when** the game master changes it, **then** it changes at once if no XP was given; otherwise the app asks for confirmation with the number of awards and of XP, and nothing is converted.
- **Given** the reminders (up to 20, 1 to 200 characters), **then** the game master edits them and players read them.
- **Given** only the game master, **then** only they write the rules; a player reads them read-only.
  A style ("Estilo da mesa") fills in the dice, combat and fog and leaves each editable; editing by hand gives "Personalizado".

**Grid and combat without a grid (RN-25).**

- **Given** a map drawn with 3 m squares, **when** the game master says "cada quadrado deste desenho vale 3 m" ("Calibrar o quadrado": 1.5 m, 3 m, 4.5 m, 6 m or another multiple of 1.5 m up to 30 m), **then** the app counts four 1.5 m squares in each, and movement, range and fog follow the usual rules.
  "Mudar a grade?" asks first only when the change erases what was painted.
- **Given** a combat the game master starts without a map ("theatre of the mind"), **when** a player moves, **then** they spend movement by number ("Restam 6 m"), with no grid position, **and** the game master judges the range.
  - "Iniciar combate" asks "Como este combate é jogado": "Com mapa" or "Sem mapa (teatro da mente)", with the table rule "combate com mapa" as the default ("Sem mapa" when the session has no current map).
    While the app reads the current map it waits ("Lendo o mapa atual…"); if the read fails, the button stays off with "Não deu para ler o mapa atual" and "Tentar de novo".
  - The mode belongs to the combat and does not change until it ends.
  - Movement never exceeds what is left in the turn; Dash doubles it; out of turn is refused; the game master spends for an NPC and for a player who left; undo restores it.
  - The game master can offer an opportunity attack to a player ("Oferecer ataque de oportunidade"); the question reaches only that player, uses their one reaction of the round, and the game master can follow without waiting ("Seguir sem esperar") or withdraw it ("Retirar a oferta").
  - Attacks, spells and actions work without reach: the player's list holds whoever the character sees and the rules allow attacking, with no distance ("O mestre decide quem está ao alcance"); the game master sets cover for the targets ("Cobertura dos alvos": none, half +2, three-quarters +5, total).
  - What needs a map does not exist: moving, jumping and placing on the map are refused with a reason, and so are traps, doors, fog, a summoned creature's square and the familiar's eyes.
  - Grappled and restrained leave speed 0; paralyzed, petrified, stunned and unconscious cannot walk.
  - A hidden NPC is never named to a player.
  - The game master sees the label "Teatro da mente" and the card "Ações do …" with "Gastar movimento" (in 1.5 m steps); the player sees "Combate sem mapa" and a "Gastar movimento" sheet with "Depois restam 3,0 m", which is only a preview of the server's number.

#### The screens
- **"Conteúdo da mesa"** (`/campaigns/:id/content`, from the campaign's "Conteúdo da mesa" panel): the game master, on a laptop, sees the five kinds with counts (archived included), "7 de 1000 entradas", search, "Mostrar" (Todas, Em uso, Sem fichas, Arquivadas) and rows with the state in words ("Em uso por 2 fichas", "Arquivado · 1 ficha usa").
  An empty campaign says "Nada cadastrado ainda." and that the SRD still applies.
  On a phone the game master only reads and archives.
  A player reads every switched-on entry, kind by kind, with "Da mesa".
- **The editors** (one page, one "Salvar"):
  - The spell: all fields, "Alvo" before the mechanics, "Mais criaturas por nível", "Pessoal" and "Toque", the classes that learn it, and a "Como os jogadores veem" preview.
  - The race and subrace, and the background.
  - The class: name, hit die, "Teste de resistência 1" and "2", how many skills and from which, proficiencies, the multiclass prerequisite, casting (none, full, half or pact; prepared or known; the list, and "Magias do SRD na lista da classe" for a class with a list of its own) and the subclass level.
    The 20-level table starts at the SRD's numbers for the chosen casting and the game master edits it cell by cell; changing the casting asks before redoing an edited table.
    Features have a level and an effect from the menu, with the counter "N de 60".
  - The subclass: features by level, "Esta subclasse conjura", always-prepared spells by class level and, when it has spells, "As magias da subclasse entram na lista da classe, para escolher".
  - The feat: "Novo talento", "Dados do talento", "O que o talento pede", "Salvar talento"; "Aumento de habilidade" is offered only there.
- **Refusals:** each violation returns on the field it names, in Portuguese, also on a table cell and on a collapsed feature, which opens.
  A summary on top says how many fields need adjusting, and what the game master typed stays.
  "Esta entrada mudou enquanto você editava" offers "Recarregar".
  After saving, "N fichas ficaram com aviso" lists the characters left with a notice.
- **"Importar pacote"** (choose a `.json` file; the preview with counts by kind and each refusal's reason; "Importar N entradas", off while an entry is refused) and **"Exportar"**.
- **"Opções para os jogadores"** (`/campaigns/:id/content/options`): one group per kind with a counter ("Raças: 9 de 10 ligadas"), search, "Ligar todas" and "Desligar todas", each option's switch with how many sheets use it and the note of a switched-off parent, all saved at once.
  The editors have a "Disponível para os jogadores" switch.
- **"Regras da mesa"** (`/campaigns/:id/rules`, from the campaign's "Regras da mesa" panel; the game master edits, a player reads): the style, hit points, the ability methods (at least one), the critical hit, the death saves, "Os jogadores veem a CD dos testes de resistência" (off by default), the reminders, the XP mode, "Talentos" ("Não usados" by default, "Permitidos") and the links to the maps' grid and to the table content.
  The dice mode is edited only here.
  See [Design](../design/maps.md#rules).
- **The character editor and the level-up** show the table's classes, subclasses, races, subraces and backgrounds among the SRD's, labelled "Da mesa".
  "Adicionar classe" makes one block per class, with the total level read-only.
  The "Magias" step has a section per class (and per spellcasting subclass), shows the always-prepared spells and drops a spell outside the lists with the reason and a "Ver em Magias" link.
  The level-up summary shows only the rows that change, with "Da mesa" and "Novas características".
  A spell's target and area come ready from the server, in metres ("Cone de 4,5 m").
  The "Habilidades" step shows neither the modifier nor the race bonus; the sheet does.
- **States and weapon rules.** A feature may define a **state** (a stance, a form): a name, a duration in rounds (a formula; empty or 0 lasts until it ends), the situations that end it besides the duration (the player ends it, incapacity, a short rest, a long rest), the action it costs and, optionally, a resource whose use it spends.
  - The sheet and the combat show the state as the card of an action with "Ativar" (it spends the action and the use) and, while it is on, "Encerrar" (free). Starting one that is on, or ending one that is off, is refused and spends nothing.
  - The player's vitals and the game master's view list the states that are on with the rounds left ("2 rodadas"); a round of the owner's turn counts one down, and the state ends when none is left, at 0 hit points or other incapacity when it says so, at the rest it names and when the combat ends (if it has a duration). The game master's correction ends one by hand. In a combat, the combat log has a line each time one begins and each time one ends, and says why it ended (by choice, the rounds ran out, incapacity, the game master, the combat ended); undoing an "Ativar" or an "Encerrar" takes its line away too. Outside a combat there is no combat log, so a rest or a correction writes no line.
  - The other effects of the entry may hold only while a state is on: a resistance, a roll mode, extra damage, a weapon rule and a pseudo-weapon carry "Só enquanto o estado", and any condition may read `active("nome")`. A resistance may also name `rage`.
  - A **weapon rule** changes the weapons its filter picks (all, melee, ranged, finesse, magical, one by name): another ability for the attack or the damage (the better one counts), a bonus from -5 to 10 on the attack or the damage, a melee reach up to 18 m.
  - A **pseudo-weapon** adds an attack of its own to the sheet and the combat (name, dice, damage type, ability, reach or range) that counts as a weapon the character is proficient with, so the weapon rules change it too.
- **"A classe mudou":** the notice on the sheet (owner and game master) and the "O que mudou" sheet, with the server's sentences.
- The SRD 5.2.1 attribution is on the "Créditos" page.
- See [Design](../design/sheet.md#editors), [Design: combat without a grid](../design/combat.md#gridless-combat-teatro-da-mente) and [Architecture: table rules](../architecture/play/table-rules.md#table-rules-in-combat).

**Out of scope:** copying content to another campaign (MR-026), own monsters and magic items, feats outside the SRD, flanking and the hexagonal grid.

#### Related
- [RN-23](rules.md), [RN-24](rules.md), [RN-25](rules.md); ADR-0017 (combat without a grid), ADR-0018 (table content).
- [MR-040](#mr-040-level-up-from-the-sheet) (level-up), [MR-045](#mr-045-look-up-spells).

### MR-010: Generate dungeons

**As a** game master, **I want** to generate a dungeon (rooms, corridors, doors and stairs, with size and style options) and get a map I can edit, **so that** I do not draw everything by hand.

- Priority: MVP
- Rules: RN-10, RN-26
- Modules: rules, maps

#### Acceptance criteria
- **Given** I am the game master of "Mirathel", **when** I ask for a dungeon with a size (21 to 121 squares per side, or what I type), the shape (no shape, ring, cross, L, ellipse, diamond), the room size, the corridor style (maze, winding or straight), the number and kind of doors and the stairs, **then** the app generates a connected map with rooms, corridors, doors and stairs: every room can be reached.
- **Given** the same size, the same options and the same seed, **when** I generate twice, **then** the result is the same.
- **Given** a generated dungeon, **when** it becomes a map, **then** it is a campaign map like the others, hidden from the players (RN-10) and with fog of war on.
  Its base light is "Penumbra" (dim), which the game master can change: a map in "Claro" shows the players the whole floor plan, so a room behind a secret door would show, while in Penumbra, even with a textured image, it shows only to whoever sees it.
  The grid, walls and doors are already painted on the layers, the stairs are submap points, and the list of rooms sits beside the map, game master only, with "Pôr uma cena nesta sala".
- **Given** a generated dungeon, **when** I edit it in the map editor (walls, doors, terrain, points), **then** "Redesenhar" redraws the image from the walls and doors as they are now, without erasing what I painted.
- **Given** a closed door, **when** a character walks into it, **then** it opens, if not locked; a locked door stops the move; a secret door is a wall for the players until I reveal it (RN-26).
- **Given** the generated image, **when** the players see it, **then** it has only floor and walls: doors are drawn over it, from the layer, and no room number appears.

#### In the app
- The generator is deterministic and written clean-room (ADR-0015): its code and data are never copied from a source under a non-commercial licence.
  See [The dungeon generator](../architecture/rules-maps.md#dungeon-generator) and [The map of a generated dungeon](../architecture/maps/dungeons.md#generated-dungeon-maps).
- **"Gerar masmorra"** (`campaigns/:id/maps/dungeon`, from the button beside "Novo mapa" in the Mapas panel; game master only, and on a phone the page says "Gerar masmorra é no notebook."):
  - Size (Mínima 21, Pequena 31, Média 51, Grande 81, Enorme 121 or "Outro" from 21 to 121; the other side is two thirds, odd), shape, the smallest and largest room side, corridors, doors, dead ends, stairs (0 to 4) and the seed with "Outra semente".
  - The preview is the server's, requested a moment after the last change; the app only draws it, with the count "13 portas e 8 passagens".
    An option out of range appears on its field, with icon and text.
  - "Criar o mapa" shows the steps; the map is born hidden, with fog on and base light "Penumbra", and opens in the editor.
    "Parar de esperar" only stops waiting: the server finishes the map anyway, and the screen says so.
- **On the generated map.**
  - The "Salas" list (game master only): number, size, stairs, exits with the true door kind, "Porta com armadilha: …", and the room behind a secret door.
  - "Pôr uma cena nesta sala" puts the RP scene point in the room at once ("Sala N").
  - Stairs are born revealed, and a player sees one only where they see the square; they are drawn as a seal with an arrow and named "Escada", never "Submapa".
- **"Redesenhar"** (offered only while the image is still the generator's) asks in place, first sends the strokes that are waiting, and every refusal has its own sentence (image swapped or grid changed, grid removed, walls changed while drawing, burst of requests).
- **Doors on screen** (see [RN-26](rules.md) and [Doors](../architecture/maps/doors.md)):
  - In the editor: the "Porta" tool in "Pintar" (kind, place, swap, "Tirar a porta"), with the screen's own refusal "uma porta precisa de chão dos dois lados" and a question in place before putting a closed, locked or secret door where a token is.
  - On the map: one mark per door kind (closed, open, locked, grate, secret) in the editor, the session and combat, and a legend made from the doors the map has.
    The padlock and the secret door show only to the game master, with "(só você vê)" and the crossed-out eye; a player never receives a locked or secret door that might arrive (RN-10).
  - In combat: whoever walks into a closed door opens it and the log says "Toren abriu a porta."; a locked door stops the move before it with "A porta está trancada.", and the player's map keeps "Porta fechada".
  - In the session, the game master taps a door on the map (with fog, or the combat's) and opens, closes or locks it in the door sheet; without fog, doors are changed in the editor.
    A secret door has "Revelar a porta secreta", which paints it closed.
    Outside combat only the game master moves tokens, so the door opens from the game master's sheet.
- The generator places no monsters, traps or treasure: for that there are the bestiary, encounters and treasure ([MR-042](#mr-042-bestiary) to [MR-044](#mr-044-generate-treasure)) and traps ([MR-035](#mr-035-traps)).
  [MR-039](#mr-039-ai-generated-images-for-dungeons-and-scenes) uses the generated dungeon to make the image.
- Out of scope: multi-floor dungeons beyond linked stairs, and the hexagonal grid.
  Drawing a dungeon by hand (false walls, water, chests and mimics) stays an idea for the map editor.

### MR-029: Scene hooks and clues

**As a** game master, **I want** to note, for each RP scene, the hooks, the clues and what to say, **so that** I can run the scene without losing the thread.

- Priority: MVP
- Rules: RN-10, RN-20
- Modules: play, maps, notes

#### Acceptance criteria
- **Given** an RP scene on the map, **when** the game master writes the hooks, clues and what to say on it, **then** only the game master sees these notes.
- **Given** a clue in a scene, **when** the game master reveals it to the players they choose, **then** it appears to those players **and** in their notes ([MR-030](#mr-030-player-notes)).
- **Given** an unrevealed clue, **when** a player opens the scene or the notes list, **then** the server sends neither the clue nor its name (RN-10).
- **Given** a scene without actions, **when** the game master opens it, **then** it opens, and the players see it.

#### In the app
- **Hooks** ("Ganchos e anotações"): Markdown up to 4,000 characters, only on a scene point, saved with the point ("Salvar ponto").
- **Clues** ("Pistas"): 1 to 500 characters each, at most 30 per scene ("30 de 30" and the limit sentence), ordered with ↑ ↓, edited in place and removed with a question in place ("Voltar" focused).
  Each shows who has it ("Todos", "Só Brisa" or "Ninguém ainda").
- **Reveal:** only the players the game master chooses receive the clue, and nobody is ticked in advance.
  Each player receives it once; it cannot be undone or hidden again.
  The player keeps a copy of the text, so editing or deleting the clue later does not change what they received.
  Someone offline reads it next time; the players recount what they found as they like, at the table.
- **On screen:** in the open scene, the column of clues and hooks (open on a computer, collapsed on a phone, the padlock and "Só você vê" always visible) and "Revelar", which opens a dialog (a sheet on a phone) with nobody ticked.
  The dashed button "Revelar a pista" says why it does not act, "Marcar todos" is a text action, and the filled button names who receives ("Revelar para Brisa", "Revelar para 2 jogadores", "Revelar para todos").
  Afterwards the clue says "Revelada para todos às 21:20" or "Revelada só para Brisa às 21:26", and "Revelar aos outros" appears on a partly revealed clue.
- The `map:` and `character:` links of the hooks' Markdown appear in the open scene as in the document, but do not open anything there.
- See [Architecture](../architecture/play/scenes-and-stage.md#hooks-clues-and-discoveries-mr-029-mr-030).

### MR-030: Player notes

**As a** player, **I want** a notepad always at hand, on the session page and on the sheet, **so that** I can jot down what happens without leaving the app.

- Priority: MVP
- Rules: RN-10, RN-20
- Modules: notes, maps, play

#### Acceptance criteria
- **Given** I am in the campaign, **when** I write a note on the session page or on the sheet, **then** only I see the note **and** it is kept for the next session.
  The game master and other players never read it, and deleting my account deletes it.
- **Given** a scene I have already discovered (revealed or opened), **when** I tag the note with it, **then** the note shows the scene.
- **Given** a scene I have not discovered yet, **when** I open the scene list to tag, **then** it does not appear, and its name never reaches my phone.

#### In the app
- A note is 1 to 2,000 characters, at most 300 per player per campaign.
  Only an active player has notes.
- The list also carries the clues the game master revealed ("Pista do mestre", read-only, outside the 300), newest first, with a filter by scene.
- A scene is discovered when its point is revealed on the map or the scene is opened in a session (even hidden), for the whole group, and stays discovered if the point is hidden afterwards.
  Revealing is the game master's choice, so a revealed scene enters the notes of the whole group even where no character sees (MR-036); the fog keeps hiding the map.
  Only a discovered scene serves as a tag; an undiscovered one is refused as if it did not exist.
- **On screen:** the "Anotações" button in the app bar, on every page of the player's session; with a new clue it says "Anotações · 1 nova" ("Anotações · 1" below 360 px).
  A clue arriving opens a notice ("O mestre revelou uma pista para você.", "Abrir anotações", and a ✕) that stays until opened or dismissed.
  The "Anotações" sheet (a sheet on a phone, a dialog on a computer) has the list, the filter by scene (only discovered scenes and "Sem cena", each with its count), the two empty states, "Nova anotação" with the scene tag and the 2,000 limit, and edit and delete (delete asks in place).
- On the character sheet (computer), the "Anotações" panel is the first block of the fourth column, with the same notes; they stay editable with the sheet locked.
  The game master never gets the panel, not even on a player's sheet.
- See [Architecture](../architecture/notes.md).

#### Related
- A clue revealed through [MR-029](#mr-029-scene-hooks-and-clues) appears in the player's notes.

### MR-031: NPCs in the scene

**As a** game master, **I want** to show NPC portraits entering and leaving the picture during a scene, as in a visual novel, **so that** the players see who is "there".

- Priority: MVP
- Rules: RN-10
- Modules: play, maps

#### Acceptance criteria
- **Given** an open RP scene, **when** the game master puts an NPC's portrait (from the gallery) on the scene, **then** the players see it enter, live.
- **Given** an NPC on the scene, **when** the game master removes them, **then** they leave the players' picture, live.
- **Given** an NPC who has not entered yet, **when** a player queries the session, **then** the server sends neither the portrait nor the name.
- **Given** an NPC in the scene, **when** a player queries the scene, **then** they receive only the name, the portrait and whether the NPC is speaking: never the kind, HP, AC, XP, sheet or the character's ID (RN-20).
- **Given** an NPC portrait, **when** a player tries to download it, **then** they can only while the NPC is on the scene; off the scene, with the scene closed or swapped, it is `404`.
- **Given** a game master who wants an NPC's portrait, **when** they pick it on the sheet, **then** only an image from the campaign's gallery is valid, and only on an NPC.
- **Given** a PNG portrait with a transparent background, **when** the NPC enters the scene, **then** the transparency is kept and the stage draws it with no box behind, only a baseline.
- **Given** the open scene, **when** a player taps an NPC on the stage, **then** they see the NPC larger, with the name and the portrait, and nothing else: the scene belongs entirely to the game master.
- **Given** an open scene, **when** the game master puts an NPC on the stage hidden ("Escondida dos jogadores"), **then** it takes a place of the 4 and the game master reads it marked, but no player receives it: not its name, portrait or place on the stage, not in the open scene, not in a live hint, not as a target or caster of a cast, not in the history, summary or highlights they read (RN-10); the players are told nothing when it enters or leaves the stage hidden.
- **Given** an NPC on the stage, **when** the game master turns it between "Os jogadores veem" and "Escondida dos jogadores", **then** showing it makes it enter the players' picture live, and hiding it makes it leave, exactly as taking it off would: nothing tells the players that it was hidden rather than taken off.
- **Given** a hidden NPC, **when** the game master taps "Dar a fala", **then** it is shown in the same call and speaks: the players see it enter and speak at once. Hiding the NPC that speaks takes the floor from it.
- **Given** a hidden NPC, **when** the game master speaks in the conversation as it (MR-051), **then** the line names the NPC for the readers he chose, whoever they are, and nothing else of it reaches them: not its portrait (the line carries none and the image route answers `404`), not a place on the stage; speaking in the conversation does not show the NPC, only "Dar a fala" does. A player who is not a reader of the line never learns the name.
- **Given** a hidden NPC's portrait, **when** a player tries to download it, **then** it is `404`, as off the stage; the game master always can.
- **Given** an NPC on the stage already, **when** the game master puts it again with the other hidden flag, **then** nothing changes: the toggle is how to show or hide it. **Given** a hidden NPC, **when** the game master gives it the floor, **then** it is shown, by design: there is no voice off screen.
- **Given** a scene open to only some players, **when** the game master puts an NPC on the stage, **then** only the scene's players receive the stage, the NPCs on it and their portraits, and the stage hints; a player who is out receives no stage, not even to aim a cast at an NPC on it (MR-015, RN-10).
- **Given** a hidden NPC, **when** the game master casts a spell for it outside combat, **then** the cast is the game master's alone and no player can aim at it, not even by its place on the stage; the casts it made in plain sight leave the players' list while it is hidden and come back when it is shown.

#### In the app
- The stage holds up to 4 NPCs per session, in order, with who is speaking; closing or swapping the scene empties it.
  A hidden NPC in combat can enter the scene without being revealed in the combat order.
  See [Architecture](../architecture/play/scenes-and-stage.md#npcs-on-stage).
- The NPC editor has "Retrato" (the short sheet and the Básico step of enemy and boss): initials without an image, "Escolher/Trocar retrato" in the gallery picker, "Remover retrato" with the question in place, all saved with the sheet.
  The portrait appears on the NPC card in combat and in the sheet header.
- The game master sees "Em cena" in the open scene (an eye per NPC: "Escondida dos jogadores" keeps it on the stage for the master only, and a hidden NPC never speaks; "Dar a fala"/"Fala agora", "Tirar de cena", "Pôr em cena" in place or in a sheet on a phone, "4 de 4").
  The players see the stage with cut-outs without a box and the speaker highlighted; the stage disappears when empty.

#### Related
- Uses the images of the [gallery](#mr-019-image-gallery), like [MR-028](#mr-028-show-an-image-to-the-players).

### MR-032: Combat highlights

**As a** table, **I want** a highlights screen at the end of combat, **so that** we celebrate who did what: which player dealt the most damage, healed the most and took the most damage.

- Priority: MVP
- Rules: RN-20
- Modules: play

#### Acceptance criteria
- **Given** a combat that ends, **when** the master ends it, **then** the table sees five categories, "Mais dano causado", "Mais cura", "Tanque", "Golpe final" and "Acertos críticos", each with its number and everyone who tied; a category in which everybody has 0 is left out.
- **Given** a hit of 28 damage on a goblin with 7 HP, **when** the highlights are computed, **then** it counts 7: only the HP that actually came off. Temporary HP that absorbed damage counts as damage dealt and taken.
- **Given** a warlock who hit a goblin with a cantrip for 5 on his turn and answered another creature's turn with Hellish Rebuke for 10, **when** the highlights or the session summary are computed, **then** his damage dealt is 15: damage dealt outside the creature's own turn (a reaction, an opportunity attack, a zone's or an effect's damage later) counts for the creature that dealt it.
- **Given** an action the master undid, or damage nobody applied, **when** the highlights are computed, **then** it does not count.
- **Given** an NPC dealt or took damage, **when** the highlights appear for a player, **then** they show only the players' numbers, never the NPCs' HP, AC or rolls (RN-20).
- **Given** a player, **when** they ask for the highlights of an ended combat, **then** they receive only their own character's row (zeros included) and never another player's; only the master receives the whole table; a combat that has not ended is refused.
- **Given** a session that ended, **when** the master opens the summary, **then** they see the duration, how many combats and scenes there were, "Testes passados fora do combate" ("9 de 12"), the highlights of all combats added up and the table per player ("Pensantus: 3 de 4"); **and** a player sees the winners of each category with their number (without the "de N") and their own result, never another player's (RN-20).
- **Given** the session had scene checks with a DC, **when** the summary is shown, **then** it has the highlight "Mais testes passados fora do combate" (such as deception and intimidation), counting only rolls from scenes that showed the DC to the players and only from actions with a DC: a roll in a scene that hid the DC counts in no category, for anyone.
- **Given** the session had treasure found ([MR-041](#mr-041-treasure-and-xp-by-gold)), **when** the summary is shown, **then** it has the highlight "Mais tesouro encontrado": the gold pieces (PO) each character found in that session, with a treasure found by two split between them, rounded down; a treasure found outside a session, or unmarked later, does not count; every member receives the category, in any XP mode.
- **Given** a session still open, **when** anyone asks for its summary, **then** it is refused: only an ended session has one.
- **Given** a campaign with ended sessions, **when** a member opens the campaign page, **then** "Sessões anteriores" lists them from the newest, each with its number, the day and hours it ran, how long it lasted and "Ver resumo", which opens that session's summary; **and** the master sees the panel with "Nenhuma sessão encerrada ainda" before the first, while a player sees no panel until there is one.
- **Given** the page of an ended session, **when** someone who is not a member, or an address with a number that is no number, opens it, **then** they see one generic page that does not name the campaign; **and** a member who asks for a number the campaign does not have reads "Não há Sessão 9" with a link to the last session.

#### In the app
- **End of combat.** The master's combat summary has "Destaques do combate" and the table "Números de cada jogador".
  The player sees the card "O combate acabou", with "Você" and "Seu resultado" (the five numbers of their own character, zeros included).
- **End of session.** The master lands on "Sessão encerrada" with the numbers, "Resumo da sessão" and the per-player table, and "Voltar à campanha".
  The player gets the card "A sessão acabou" with the winners and "Seu resultado"; "Fechar" leaves the plain notice "A sessão acabou", which is also what a failed read shows.
- **"Mais tesouro encontrado".** The master gets a block with one row per character who found something and the note "Quem não encontrou nada não aparece. Um tesouro encontrado por duas pessoas divide o valor, arredondado para baixo."; the player gets a tile and "Tesouro encontrado" in "Seu resultado".
  Neither carries the treasure's name or time.
- **Past sessions.** The page `/campaigns/<id>/sessions/<number>` draws the same body as the end-of-session screen; "Sessão 4 em andamento" leads to the live session, and the master sees "Não houve combate nesta sessão." when no combat happened.

#### Related
- [MR-041](#mr-041-treasure-and-xp-by-gold): the treasure that feeds "Mais tesouro encontrado".
- See [Architecture](../architecture/play/highlights-and-summary.md#combat-highlights) and [Architecture](../architecture/play/highlights-and-summary.md#session-summary).

### MR-033: Print the map with the grid

**As a** master, **I want** to print the current map, or save it as PDF, with the grid at table scale, **so that** I can play with miniatures.

- Priority: MVP
- Rules: RN-21
- Modules: maps

#### Acceptance criteria
- **Given** a map with a grid, **when** the master opens "Imprimir com a grade", **then** they see the screen "Imprimir o mapa" with the 2.54 cm (one inch) square and A4 paper, and the sheet count: 30 × 20 squares give 76.2 × 50.8 cm, on 9 A4 sheets.
- **Given** the print screen, **when** the master changes the square size (1 to 10 cm, with comma or dot) or the paper (A4, A3, A2, A1, Carta or Ofício, the Brazilian 21.6 × 33 cm), **then** the summary, the sheet preview and "As contas" (one row per paper, in both orientations) change at once; the orientation is the one that uses the fewest sheets, landscape on a tie. With 2 cm on A3, that is 3 sheets in portrait.
- **Given** a map larger than a page, **when** the master prints, **then** the map is split into sheets with a 1 cm margin and 1 cm overlap, with the grid aligned between them, each sheet carrying the label of where to paste it ("Página B2 · cole à direita da B1 e abaixo da A2") and a 5 cm ruler to check the scale.
- **Given** a size outside 1 to 10 cm, **when** the master types it, **then** the screen shows the error, leaves the preview empty and the "Imprimir" button dashed, doing nothing.
- **Given** a print of more than 16 sheets, **when** the master sets it up, **then** an amber warning names the paper that uses the fewest; above 36, the preview stops showing the labels. Nothing is blocked.
- **Given** a map without a grid, **when** the master looks at the map page, **then** the "Imprimir com a grade" button is disabled, with the reason written beside it.
- **Given** a map hidden from the players, **when** the master prints, **then** the whole map comes out: the print is the master's. The player never sees the button or the screen (the route answers "Só o mestre imprime o mapa.").
- **Given** the print, **then** only the image and the grid come out: points, marks and tokens are left out.

#### In the app
- It runs in the browser only, with print styles and the browser's own print or save-as-PDF. The screen asks for 100% scale and no headers and footers, because the browser may shrink the page; the 5 cm ruler on each sheet lets the master check.
- See [Design](../design/maps.md#printing-pagesmapsmap-print).

#### Related
- [RN-21](rules.md).

### MR-034: Special movement

**As a** player, **I want** the app to handle jumping, difficult terrain and cover, **so that** the movement rules apply on screen as they do at the table.

- Priority: MVP
- Rules: RN-21
- Modules: play, rules

#### Acceptance criteria
- **Given** a character with Strength 16, **when** they jump, **then** the app shows the jump's reach in distance and height, calculated from Strength (16 ft long and 6 ft high with a running start, half of each standing), and deducts the distance from movement.
- **Given** a monk who used Step of the Wind, or a Thief with Second-Story Work, **when** they jump, **then** the jump distance is doubled for the turn, or the running long jump gains the Dexterity modifier in feet.
- **Given** a square marked as difficult terrain, **when** the character enters it, **then** each square costs double the movement, and the Dash keeps working together with it.
- **Given** the master paints the map, **when** they use "Pintar", **then** they can mark difficult terrain, walls, cover (half or three-quarters) and light on the grid; the players of a map they see read wall, terrain and cover, never the light; changing the grid or the image erases the painting, and both are refused while a combat is on the map.
- **Given** a target behind cover marked on the map (half or three-quarters), **when** someone attacks it or it resists a Dexterity effect, **then** the server adds +2 or +5 to the AC or to the save, along the line between the two; behind a wall it cannot be targeted. The master can also mark the cover of a combatant for what the map does not show.
- **Given** another creature in the way, **when** the character passes through its space, **then** the space of a non-enemy costs as difficult terrain, an enemy's cannot be crossed (unless the sizes differ by two), and nobody ends the movement in another's space.
- **Given** an NPC's turn on a map, **when** the master drags its token, **then** it walks the way a player's move does: the log says what the line is in metres ("Espada voadora 2 anda 12 m", counted as the app counts every move), the NPC's "Restam 15 m · 10 quadrados" goes down by what the path cost (rubble included), it stops before walls, creatures and squares marked "Não dá para passar", and a drag past the movement left asks "Espada voadora 2 não tem movimento para tanto. Faltam 4,5 m do que resta. Mover mesmo assim, sem gastar o movimento?" with "Mover mesmo assim" (free, no opportunity attacks, for a dash or a push) and "Cancelar". Outside the NPC's turn a drag stays a free placement.
- **Given** an enemy next to the character that sees them and has its reaction, **when** the character leaves its reach without having used Disengage, **then** the app offers an opportunity attack to whoever controls the enemy, and the movement waits for the answer: the attack, "Seguir sem esperar" or a decline. A long jump provokes too; a movement the master marks "Movimento forçado" does not. At 0 HP from the attack, the mover goes back to the square they left, if free.
- **Given** a movement, **when** the master undoes it, **then** the offers it caused go with it.

#### In the app
- **Move page.** "Mover" shows every reachable square, each square's cost, the reason a square is refused and who may provoke; it has "Saltar" with the distance and height the server allows.
- **Cover on screen.** The target list states the cover and its origin ("Meia cobertura (do mapa)", "Três quartos (marcada pelo mestre)") and leaves out who a wall covers entirely; the master's order has "Marcar cobertura" and "Marcar como aliado".
- **Map editor.** On a computer, "Pontos | Pintar" has Terreno difícil, Parede, Cobertura (Meia, Três quartos), Luz (Claro, Penumbra, Escuro) and "Não dá para passar" (a checkerboard: nobody walks into the square, but sight and light pass), "Apagar" and a 1×1 or 3×3 brush; "Camadas" lists it with its own switch.
  Dragging, clicking, Shift to erase and the keyboard all paint. "Camadas" says "Tudo salvo", "Salvando" or "Não salvou".
  Without a grid the tools are dashed ("Defina a grade para pintar e ligar a névoa.").
  Changing the grid, replacing the image and "Esquecer o que foi visto" ask in place first.
  On a phone there is no painting ("Pintar só no computador"). What is drawn in the image, the players see.
- See [Architecture](../architecture/rules-maps.md#grid) and [Architecture](../architecture/maps/editor.md#layers).

#### Related
- [RN-21](rules.md).
- Rules chosen: half cover is a low wall or crates, three-quarters a column or an arrow slit; each square of difficult terrain entered costs 1.5 m more.

### MR-035: Traps

**As a** master, **I want** to place hidden traps on the map, with the DC to notice and to find, the trigger and the effect, **so that** I can surprise the table with the game's rules.

- Priority: MVP
- Rules: RN-10
- Modules: maps, play

#### Acceptance criteria
- **Given** a map, **when** the master places a trap, **then** they set the DC to notice (passive Perception, optional) and to find (Investigation), the area (1×1 to 4×4), the trigger (on entering the area or by hand) and the effect in parts (an attack, damage that always happens, conditions, a saving throw with what happens on a failure and on a pass); only the master sees it. The eight example traps of the SRD are presets, and everything stays editable.
- **Given** a hidden trap, **when** a character (or a creature of theirs) ends a movement within 3 m of it, seeing a square of its area, with passive Perception equal to or greater than the DC to notice (minus 5 in dim light), **then** their player starts to know the trap, and only that player is warned. A trap with no DC to notice is never noticed this way.
- **Given** a hidden trap, **when** a player uses "Procurar armadilhas" and passes a Perception check (against the DC to notice) or an Investigation check (against the DC to find), with the app's d20 or a physical die (RN-18), against a trap within 3 m whose square the character sees, **then** only that player starts to know it.
- **Given** the search, **when** nothing is found or the check fails, **then** the answer reads the same and carries no DC. In a combat the search is the Search action: only on the character's turn, and it spends the action. A Perception search in dim light has disadvantage.
- **Given** "Outra perícia…", **when** a player searches with any other skill, **then** it meets the DC to find only for the traps whose master listed that skill ("Também acham com"; none by default); for any other trap it finds nothing, exactly like a search that fell short.
- **Given** a trap, **when** a character enters its area (a straight-line move stops at the first square of the area; a jump fires only where it lands; a token the master drops inside fires; an NPC never fires one) or the master fires it by hand, **then** the server rolls the attack, the damage and the save of each creature caught, and the trap becomes visible to all as "Disparada". A token fires a trap only on a map the players can see.
- **Given** the effect, **when** it lands, **then** damage to an NPC or a creature is applied at once and conditions go to the combatants; damage to a player character waits for the master (RN-02), who may change the amount, apply it or discard it, in or out of combat, and it survives the end of the combat and of the session.
- **Given** a movement that would enter a trap the character knows, **when** the player plans it, **then** the screen asks "Isso entra no Fosso escondido. Mover assim mesmo?"; a trap they do not know is never named, and a movement cut by it only says it stopped.
- **Given** a combat, **when** the master undoes the firing, **then** the trap is armed again, HP and conditions return and pending damage is erased, and only then the movement.
- **Given** a fired trap, **when** the master disarms it after the table's thieves' tools check, **then** it never fires again.
- **Given** a trap not yet found, fired or revealed, **when** a player queries the map, **then** the server sends not even its position, DCs, effect or preset (RN-10). The master can reveal a trap to chosen players.

#### In the app
- **Master, placing.** "Armadilha" in the points bar opens the panel with the eight presets as radios (each with a line such as "Queda de 6 m, 2d6") and "Começar do zero", the name, "Descrição para você", "CD para notar (Percepção)", "CD para achar (Investigação)", the area, the trigger, the effect parts, who is caught and the state (Armada, Disparada, Desarmada).
  The notice "Não desenhe a armadilha na imagem: os jogadores veem a imagem." sits on the form.
- **Master, in the session.** "Armadilhas do mapa" has one card per trap, with "Quem notaria" (each character with passive Perception, whether on the map, within 3 m, and whether they would notice: "Nota", "Não nota", "Se chegar a 3 m: nota"), "Revelar para…", "Disparar…", "Desarmar", the damage waiting ("Aplicar N de dano" or "Não aplicar") and the "Registro".
  "Disparar…" with nobody marked fires for whoever is in the area, and the screen says so; a firing that caught nobody leaves a visible notice.
- **Player.** "Procurar armadilhas" offers Percepção, Investigação and "Outra perícia…", with the same menu for every trap and the result "Achou" or "Nada"; the notice "Você notou uma armadilha." and the note "Você caiu na armadilha…" on their turn.
- Design choices that are not SRD rules: the 3 m is measured between square centres; the trap's attack goes one by one to the creatures caught; an active check in dim light has no disadvantage.
- See [Architecture](../architecture/maps/editor.md#layers).

#### Related
- [MR-010](stories.md#mr-010-generate-dungeons), [MR-038](#mr-038-puzzles) (a wrong move can fire a map trap).

### MR-036: Fog of war by sight

**As a** player, **I want** to see on the map only what my character sees, **so that** exploration has the suspense of the table.

- Priority: MVP
- Rules: RN-10
- Modules: maps, play, rules

#### Acceptance criteria
- **Given** a map with a grid, **when** the master turns on "Ligar a névoa", **then** each player sees only what their own character sees now or has seen before; what was seen stays darkened, without creatures, until the master asks "Esquecer o que foi visto" or changes the grid or the image.
- **Given** the base light of the map ("Luz de base": Claro, Penumbra or Escuro, dark by default), **when** it is "Claro", **then** the players see the whole floor plan, while creatures and points still show only where their characters see.
- **Given** a light source (a torch, a spell, a custom radius up to 36 m), **when** the master places it on the map, **then** the area around it becomes visible, cut by the walls the master painted; the player sees the light, never the point.
- **Given** a character with 18 m of Darkvision on the sheet, **when** they are in a dark area, **then** they see up to 18 m, in shades of grey.
- **Given** a light a character carries, **when** the player (or the master) sets it, **then** it lights around that character, and only the master and the owner learn about it.
- **Given** "Visão do grupo", **when** it is on, **then** every player sees the union of what all the characters see.
- **Given** a player character, **when** another player looks at the map, **then** the token never disappears; an NPC's token shows only on a square the viewer sees now.
- **Given** the master, **when** they open the map, **then** they see everything, and "Ver como" shows the map as a chosen character sees it. What the player does not see never leaves the server (RN-10): not the image of the unseen part, the points, the layers, the lights or the NPCs.
- **Given** a combat on a fog map, **when** a player looks at the order, the turn, the targets, the log and the opportunity-attack offers, **then** they see only the NPCs their character sees now (with "Visão do grupo", the group); player characters and their creatures never disappear. Movement is planned with the terrain the player knows and cut where something blocks it, naming no wall or creature.
- **Given** a druid in wolf form, **when** they are in a dark room, **then** they see nothing new: the beast has no darkvision, and its senses apply in place of the character's (SRD).
- **Given** a fog map with the base light "Escuro", **when** a player character with no darkvision stands where no light reaches, **then** the master sees "No escuro" with that character's name and the buttons "Ver no mapa" and "Acender luz"; no player sees the warning, and it goes away when a light (an NPC's hidden one too) reaches the square or the character has darkvision.
- **Given** a player with a familiar on the same map, 30 m or less away, **when** they choose "Ver pelos olhos do familiar", **then** they see what the familiar sees, with its senses (the owl's darkvision), and the character is blind and deaf until the player stops; in combat it spends the action and ends at the start of the character's next turn. Other creatures show on the map but see for no one.
- **Given** a map without fog, **when** the players open it, **then** it works as it always did.

#### In the app
- **Master's editor.** "Névoa de guerra" has "Ligar a névoa" (needs the grid), "Luz de base", "Visão do grupo" and "Esquecer o que foi visto", which asks in place; each change goes at once. "Luz" is a point kind with the SRD sources and "Personalizada". The map draws two rings (bright light solid, dim light dashed) and says walls cut light only with fog on, which "Ver como" shows ("Você está vendo o mapa como Toren", "Voltar à sua vista").
- **Player and master screens.** The fog map shows "Carregando o mapa" while it builds, "Seu personagem não está neste mapa", "Luz que você carrega"; the master has "Luz dos personagens". The browser draws what the server sends and counts only squares ("76 quadrados vistos"). The master also sees "No escuro: ..." over the map, and a "No escuro" badge in "Grupo", for characters who see nothing where they stand (no light, no darkvision), with "Ver no mapa" and "Acender luz"; players never see it.
- **The image.** The player receives the picture in pieces made on the server with only the squares seen or remembered; two images that differ only in unknown squares give identical pieces. At the fog edges the picture is slightly darkened. See [Architecture](../architecture/maps/fog.md#per-player-image-tiles).
- See [Architecture](../architecture/maps/editor.md#layers) and [Architecture](../architecture/play/creatures.md#wild-shape).

#### Related
- [MR-037](#mr-037-creatures-of-the-character) (the familiar, Wild Shape), [MR-034](#mr-034-special-movement) (walls and layers), [MR-035](#mr-035-traps) (lights and traps as points).
- Rules chosen: the walls the master paints block sight and light, as in the official rules; an enemy the character does not see does not appear to them, so attacking in the dark (even a Fireball in a suspect corner) is the master's call.

### MR-037: Creatures of the character

**As a** player, **I want** to control a creature of mine (the druid's Wild Shape, the undead of Animate Dead, a familiar), **so that** I can play it in combat with its own sheet.

- Priority: MVP
- Rules: RN-02, RN-20
- Modules: play, rules, characters

#### Acceptance criteria
- **Given** a druid with Wild Shape, **when** they use it, **then** the player gets the creature's sheet, taken from the SRD creatures, and acts with it in combat: the beast's AC, attacks, speeds and senses, the beast's HP separate, and no spells. The list of beasts follows their level (level 2: CR up to 1/4, no flying or swimming; 4: up to 1/2, no flying; 8: up to 1). Using it spends a use of Wild Shape and, in combat, the action.
- **Given** a druid in beast form, **when** the beast falls to 0 HP, **then** they return to their own form and the leftover damage passes to their HP; "Voltar à forma normal" is a bonus action, and healing in the form heals the beast. The master can end the form by correcting the beast's HP to 0.
- **Given** a spell that summons (Find Familiar, Animate Dead, Conjure Animals), **when** the player casts it, **then** the creatures join the character: Find Familiar is a ritual that spends no slot and a new familiar replaces the old one; Animate Dead spends the slot, which decides how many; Conjure Animals offers the four options. Find Steed is not included.
- **Given** Conjure Animals (or Animate Dead) in combat, **when** the creatures enter, **then** they roll one initiative together and act in a joint turn (a house rule for Animate Dead, which the SRD says only of Conjure Animals).
- **Given** the master, **when** they give a creature to a character, **then** any of the 334 SRD creatures can be given; the player or the master can rename it (up to 40 characters) or dismiss it. A character holds at most 40 creatures.
- **Given** a character's creature, **when** its turn comes, **then** it enters the combat order with its own action economy, and the player controls it. The familiar does not attack, except the Pact of the Chain's with its reaction.
- **Given** a character's creature, **when** it takes damage, **then** its HP is separate from the character's HP, shown as a number only to the owner and the master, and the master can correct it (RN-02). At 0 HP it is defeated and dismissed.
- **Given** the concentration on the casting ends, **when** it ends, **then** the creatures of that casting are dismissed (RN-22); the master's undo brings them back.
- **Given** the master places a character's creature token on a map, **when** the group looks, **then** everyone sees it (a group token, never hidden, even in fog).
- **Given** a player who cannot have a creature, **when** they look at another character's sheet, **then** the "Criaturas" panel does not exist for them (RN-20).

#### In the app
- **Sheet.** The "Criaturas" panel has one card per creature ("Corvo · Miúdo · Familiar de Pensantus", AC, HP, speed in metres), "Ver a ficha do Nanquim", "Renomear", "Dispensar" (asks in place) and, for the master, "Corrigir PV" outside combat. Summoning is only during a session ("Agora não há sessão aberta."). The master gives creatures in the campaign's character list with "Dar uma criatura".
- **Combat.** A player who plays more than one combatant gets tabs in the turn bar and the page follows the turn. The creatures' turn page has a block per creature ("Atacar", "Mover o Lobo atroz 1") and "Encerrar a parte dos Lobos".
  The master's order shows creatures as dashed round tokens ("CA 14 · da Sálvia"), the joint turn box and the "Concentração" tag; "Perdeu a concentração" asks in place before dismissing. The player reads "Você perdeu a concentração em Conjurar Animais. Os 2 Lobos atrozes sumiram."
  Wild Shape is "Transformar" in the Action ("restam 2 de 2 usos · volta no descanso curto ou longo"); the beast's turn has the banner "Na forma de Lobo", both HP pools and "Sem magias na forma de fera…". When the beast falls the notice "O Lobo caiu a 0 PV e você voltou à forma normal. 6 de dano passaram para você." stays until tapped.
- See [Design](../design/sheet.md#editors), [Architecture](../architecture/rules-creatures-items.md#creatures), [Architecture](../architecture/play/creatures.md#what-the-combat-does-with-a-creature) and [Architecture](../architecture/play/creatures.md#wild-shape).

#### Related
- [MR-036](#mr-036-fog-of-war-by-sight) (the familiar's eyes), [RN-02](rules.md), [RN-20](rules.md), [RN-22](rules.md).

### MR-038: Puzzles

**As a** master, **I want** to create puzzles the players solve in the app, live in a scene, **so that** I can vary the pace of the session.

- Priority: MVP
- Rules: RN-10, RN-18, RN-27
- Modules: play, maps

#### Acceptance criteria
- **Given** I am the master of "Mirathel", **when** I create a puzzle of one of the six kinds ("Apagar as luzes", the combination lock, the rotating symbols, the riddle, the sequence to repeat or the cipher), **then** it is saved in the campaign and only I see it; I can edit it until the first "Mostrar", and archive it.
- **Given** a 5 by 5 "Apagar as luzes" puzzle, **when** I create it, **then** the server generates a start that has a solution and is never already solved, **and** I see how many presses are enough.
- **Given** an open session, **when** the master shows a puzzle, **then** the players see it and solve it live, all in the same state, with the clue the master wrote, **and** see who made the last move (the character's name).
- **Given** a puzzle shown, **when** two players play at the same time, **then** both moves count, and everyone sees the new state. A move repeated after a lost answer counts once.
- **Given** a riddle, **when** a player types the answer, **then** the server compares it with the answers the master accepts, ignoring case, accents and punctuation, **and** the player never receives the right answer or how close the try was.
- **Given** a sequence to repeat (3 to 8 bells, 3 to 12 steps), **when** the master plays it, **then** the players watch it step by step, only the steps already revealed, **and** then repeat it; the server checks the order.
- **Given** a cipher, **when** the players open it, **then** they see the encrypted message and decipher it with the key they found as a clue in the adventure; the server encrypts it and checks the deciphered message.
- **Given** a puzzle with hints, **when** the master releases the next hint, **then** the players see it; **when** a player passes a skill check against the DC the master set (Investigation, Arcana...), **then** they receive the next hint, only for them (RN-18: in the app or with the physical die). The DC never shows.
- **Given** a split-information puzzle (up to 8 parts, each for a player character), **when** the players open it, **then** each sees only their part of the clue, **and** they must talk to solve it.
- **Given** a puzzle with consequences, **when** a wrong move happens (a wrong combination or answer), **then** what the master chose in "Ao errar" happens: a map trap fires (MR-035), a player's attempt is spent (1 to 10 each), or the limit of moves or minutes comes closer; at the end of the limit the puzzle stops and the master is warned. The trap and the attempt exist only for the three kinds that judge a move, and simultaneous wrong answers are counted exactly.
- **Given** a puzzle shown, **when** the players solve it, **then** the server checks the solution (the player never receives the answer), the puzzle stops, the master is warned **and** what they chose in "Ao resolver" happens: only the notice (default), open a door (RN-26), reveal a map point or reveal a scene clue to whoever solved it (RN-27).
- **Given** a puzzle shown, **when** the master restarts or closes it, **then** the players see the same start again, or stop seeing it; "Gerar outro começo" ("Apagar as luzes" and rotating symbols) is a separate master action.
- **Given** a player, **when** the server answers about a puzzle, **then** it never carries the accepted answers, the next step of the sequence, the unencrypted message, the key, a DC or another player's part (RN-10).

#### In the app
- **Master, list and editor.** "Quebra-cabeças" on the campaign page's "Preparação" tab lists the puzzles with their states, "Editar", "Arquivar" (asked on the row; what was shown stays as session history) and "Mostrar os arquivados". "Novo quebra-cabeça" and "Editar quebra-cabeça" have the form of each kind, the clue, the hints, "Dica por teste de perícia", "Informação dividida", "Ao errar" (one way at a time) and "Ao resolver". Each refusal shows on the field it refers to.
- **Master, in the session.** "Mostrar aos jogadores" and "Ver ao vivo" show the puzzle as the players have it, the last move and who made it, the hints ("Mostrar a próxima dica"), "Gerar outro começo", "Recomeçar" and "Fechar", asked on the screen itself. Only the master reads, with "Só você vê", the accepted answers, the sequence's steps, the message in the clear, the attempts left, the counters and who won a skill hint.
- **Player.** The notice "O mestre mostrou um quebra-cabeça" opens the puzzle in the session page. The riddle answers "Não é isso." with "Tentativas restantes 2 de 3"; the sequence says "Errou o passo 4. A tentativa recomeçou; a Lia errou."; the counters read "Jogadas 7 de 10" and "Tempo 4:48 de 5:00"; "Tentar uma dica · Investigação" gives "Você conseguiu. Esta dica é só sua" or "Não deu desta vez." The 7 × 7 panel fits a 320 × 568 px screen.
- See [Architecture](../architecture/play/puzzles.md), [RN-27](rules.md) and [Design](../design/session.md#puzzles).

#### Related
- [MR-047](#mr-047-more-puzzles): the ideas left for after the MVP.
- [MR-035](#mr-035-traps) (the trap a wrong move can fire), [RN-10](rules.md), [RN-18](rules.md), [RN-26](rules.md), [RN-27](rules.md).

### MR-039: AI-generated images for dungeons and scenes

**As a** game master, **I want** to generate an image from a dungeon, a map or a scene description (scene art, an isometric view of the map, or the map itself with texture, matching the grid) and ask for adjustments, **so that** I can show the table the place I am describing.

- Priority: MVP
- Rules: RN-10, RN-28
- Modules: maps (gallery)

#### Acceptance criteria
- **Given** a generated dungeon ([MR-010](#mr-010-generate-dungeons)) or a scene description, **when** the GM asks for an image with their own text and style, **then** the app generates the image and stores it in the campaign gallery, hidden from the players. Only the master generates.
- **Given** a generated image, **when** the GM writes a new request ("darker", "with a bridge"), **then** the app edits it with knowledge of the scene, without starting over, **and** stores the new one next to the previous one.
- **Given** the campaign's monthly limit (200 images in production) has been reached, **when** the GM asks for another image, **then** the app refuses and says why and when the limit resets. The server has a daily limit of its own.
- **Given** the service refuses a request, returns no image or fails, **when** the GM waits for the image, **then** the app says so in Portuguese ("O serviço não gerou esta imagem") without spending the month's slot.
- **Given** the service returned the image but the server could not keep it, **when** the GM waits for the image, **then** the app says the image was made but could not be stored ("A imagem foi feita, mas não deu para guardá-la"), not that the service did not answer, **and** the attempt does not count against the month. A request that left and timed out with no image still counts, because it may have been billed.
- **Given** a map with a grid (generated or drawn), **when** the GM asks for "O mapa com textura" (the textured map), **then** the app generates the map itself seen from above, with floor and walls in place, cropped and fitted to match the grid, **and** the GM can set it as the map image ("Usar como imagem do mapa") without erasing the layers or what the players have already seen. A map whose image is over 16 megapixels cannot become a textured map, and the screen says so before asking.
- **Given** a map, **when** the GM asks for "Vista isométrica" (isometric view), **then** the app generates the map art in isometric perspective from what the players see now, **and** it goes to the gallery as scene art.
- **Given** scene art or an isometric view made from a map, **when** the app builds the reference drawing, **then** it carries only what the players see now: the squares some character sees, the creatures they see and the unrevealed secret door as a wall; other rooms and hidden enemies are left out, and the image does not show what they have not discovered. On a map without fog, the whole map goes, minus what is hidden from the players.
- **Given** the textured map, **when** the app builds the reference drawing, **then** it starts from the whole map, with the unrevealed secret door as a wall (the fog already hides the map from each player), **and** the image does not show the passage (RN-10, RN-26).
- **Given** a map with tokens, a combat or NPCs in the scene, **when** the GM asks for scene art or an isometric view, **then** they choose which NPCs and enemies appear (gallery portraits go as references), limited to the ones the players see now, **and** only those appear; they can also pick other gallery images as references (up to 10 object images and 4 character images). The textured map takes neither creatures nor character images. No NPC is offered while a combat runs on the map.
- **Given** an image request, **when** the app sends it to the AI service, **then** only the map drawing, the room list (only for the textured map of a generated dungeon), the GM's text and the references they chose are sent, never personal data (RN-28).
- **Given** an image that shows the whole map (the textured map and every adjustment of it), **when** the GM tries to show it to the players, **then** the app asks first ("Esta imagem mostra o mapa inteiro, também o que os jogadores ainda não descobriram.") and never offers it with one tap (RN-10).
- **Given** a generated image, **when** it is named, **then** the name is one the players understand: the GM's, or the map's name and the way (only for a map the players see), or the way and the day ("Arte da cena · 06/10"), or the adjusted image's name plus " (ajuste)"; never "Imagem N", the request text or the name of something hidden (RN-10).

#### In the app
- **"Gerar imagem".** A dialog (a bottom sheet on the phone) opened from the map, a scene point and the gallery. It offers the three ways (the ones that do not fit are dashed with the reason), shows the drawing the server builds, "Quem aparece na imagem", gallery references, what goes to Google under each field and the month's count. While waiting it offers "Cancelar" and "Parar de esperar"; the result has "Mostrar aos jogadores" ([MR-028](#mr-028-show-an-image-to-the-players)), "Pedir um ajuste" and the chain of adjustments.
- **Layers stay the truth.** The model returns one of 10 fixed proportions, so the server pads the map drawing with rock, crops the map back and fits it to the grid. If the image drifts a little from a wall, the GM generates again.
- The session's image picker ("Mostrar uma imagem aos jogadores") marks "Mapa inteiro".
- See [Architecture](../architecture/maps/image-generation.md), [images made from a map](../architecture/maps/image-generation.md#images-made-from-a-map), [Design](../design/session.md#generated-images), [Privacy](../privacy.md#what-generated-images-send-to-google-mr-039-rn-28) and [Operations](../operations/reference.md#generated-images-the-gemini-api).

### MR-040: Level up from the sheet

**As a** player, when my character can level up, **I want** to edit the sheet to add only what the next level gives, **so that** I do not wait for the GM to apply the level for me.

- Priority: MVP
- Rules: RN-01, RN-12, RN-33
- Modules: characters, rules

#### Acceptance criteria
- **Given** a character that "Pode subir de nível" (RN-12), **when** the player opens the locked sheet, **then** only the owner sees the block "Pensantus pode subir de nível" with the reason and the button "Subir para o nível 4"; without that mark the sheet stays read-only (RN-01) and the level-up route answers like a locked sheet.
- **Given** the guided level-up, **when** the player uses it, **then** it adds one class level and only that level's choices, in steps (Habilidades, Vida, Magias, Resumo, and "Escolhas" for the subclass, feature options, skills and expertise); a step with no choice does not exist. Nothing is stored until "Confirmar o nível 4".
- **Given** the guided level-up, **when** the player tries to change anything else on the sheet, **then** the server refuses: the rest stays locked.
- **Given** an edit the D&D 5e rules do not allow, **when** the player submits it, **then** the rules engine refuses it, naming the field and the reason.
- **Given** the hit die of a level, **when** the player rolls in the app, **then** the server rolls and stores the result, and rolling again returns the same one; in a campaign that forces physical dice, the app does not roll and the player types the result (RN-18). A table that allows only the average has no die.
- **Given** a Warlock's level, **when** the number or level of pact slots changes, **then** the summary lists "Espaços do pacto" (for example "1 de 1º nível → 2 de 1º nível").
- **Given** a character whose current hit points are set, **when** the level up raises the maximum, **then** the current rises by the same amount: a wound stays a wound and a character at the maximum stays at the maximum; one whose hit points were never set stays full (RN-12).
- **Given** the player has leveled up, **when** the sheet is saved, **then** it shows the new level, "Pensantus subiu para o nível 4. O mestre foi avisado.", and "Pode subir de nível" clears by itself.
- **Given** the player has leveled up, **when** the GM opens the campaign, **then** they see "Subiu para o nível N" and a notice at the top (for 24 hours, or until dismissed), and "O que mudou" opens the player's choices at that level (ability, hit points and how, cantrips, spells, prepared), with the time. There is no approval and no veto: the GM keeps editing the sheet as always (RN-02).
- **Given** a character that can level up, **when** the player opens the guided level-up with more than one class available, **then** the first step is "Subir em qual classe?": the classes the character has ("Guerreiro, nível 5 → 6") and "Uma classe nova", which opens the other classes with what each asks for and what the character has; a class the character does not qualify for stays in the list, dashed, with "Falta: Carisma 13 (você tem 9)." (SRD 5.1, "Multiclassing").
- **Given** a new class, **when** the player confirms the summary, **then** the footer first asks "Subir em Mago 1? Isso acrescenta uma classe nova à ficha…" with "Voltar" focused; the class joins at level 1 with the average hit points of its die, only the proficiencies of the multiclass table (a skill, and the Bard's instrument, are picked) and no equipment, and the summary says why each number changed (total level, proficiency bonus, hit dice apart, spell slots by the caster level).
- **Given** a class whose main ability the character lacks, or a character whose current class lacks its own, **when** the player asks for it, **then** the server refuses with the class, the ability, the minimum and the score; the master's editor may go past it, and a player's creation of such a sheet is refused too.
- **Given** a player creating a character whose class or race asks for choices, **when** they reach the step "Escolhas", **then** each choice lists its options with the rule in one line, the ones not yet available dotted with the reason, and "Criar personagem" says what is still missing (RN-33).
- **Given** a race, a trait or a background that lets the player pick languages (the Half-Elf's extra language, the Acolyte's two), or a trait, background or feature that lets them pick tools (the Dwarf's artisan's tools, a background's "one type of artisan's tools"), **when** the player reaches the step "Escolhas", **then** each is a choice from the catalog's languages or tools, one each, with the ones the character already has dotted and the reason, and the picks show on the sheet's "Idiomas" and "Proficiências"; the free field "Idiomas" stays for the master's, and what is written there shows on the sheet too. While one is open the sheet says "Falta escolher N idiomas: faça a escolha no passo “Escolhas”" and the save is not stopped (RN-33).
- **Given** a locked sheet with open choices, **when** the owner opens it, **then** it says "N escolhas pendentes" and "Completar" leads to a page with only those; the master sees "N escolhas em aberto" on the campaign (RN-33).
- **Given** a level-up, **when** the character has choices left behind, **then** "Escolhas que ficaram para trás" comes before "Próximo", and an invocation that asks something the sheet lacks is dotted with the reason (RN-33).

#### In the app
- The numbers in "O que muda" and the summary come from the server, which also lists the attacks whose to-hit or damage the level moves. The hit die uses the same roll picker as combat and the scene (RN-18).
- "Cancelar" and "Voltar para a ficha" ask in place before discarding. A stale sheet or an engine refusal shows in place, with the reason, and "Ler a ficha de novo" keeps the choices that still fit.
- See [Architecture](../architecture/characters.md#leveling-up), [Design](../design/session.md#experience-and-level-up) and [RN-01](rules.md).

#### Related
- [MR-017](#mr-017-level-up) (the full level-up screen, after the MVP), [RN-01](rules.md), [RN-12](rules.md).

### MR-041: Treasure and XP by gold

**As a** game master, **I want** to place treasure on the map before the session and convert what the group found into XP, **so that** I can award XP by gold without doing the math on the spot.

- Priority: MVP
- Rules: RN-09
- Modules: maps, progression

#### Acceptance criteria
- **Given** a map, **when** the GM places a treasure (a chest, for example) with a value in gp (a whole number, 0 to 1,000,000) and a description, **then** the treasure is stored on the map, hidden like any other point (RN-10).
- **Given** a treasure the group found, **when** the GM marks it as found by one or more characters, **then** everyone who sees the map sees it, with the description, the value and who found it, and it counts as found and not yet converted.
- **Given** a treasure marked by mistake, **when** the GM takes the mark off, **then** it is hidden again, unless it was already converted.
- **Given** a session is open, **when** a treasure is marked or unmarked, **then** it counts in that session's summary and the players read "Brisa encontrou o Baú de moedas."; a treasure marked outside a session counts in no summary, and the editor says so.
- **Given** found treasures in a gold campaign, **when** the GM uses "Voltar à cidade", **then** the app converts them into XP, 1 XP per gp (RN-09), split equally among the characters the GM chooses (the remainder is lost and the screen says how much), and each treasure is converted only once, even if two awards race.
- **Given** a converted treasure, **when** the GM tries to change its value, kind or mark, or to delete it, **then** the app refuses until the award is undone.
- **Given** the last award was a "Voltar à cidade", **when** the GM undoes it, **then** the treasures return to "found, not converted" and can be converted again; after a newer award the conversion is no longer the last, so it can be undone only once the newer awards are.
- **Given** a campaign that gives XP by enemies or by milestones, **when** a treasure is found, **then** it appears in the session summary and is never converted: there is no "Voltar à cidade".
- **Given** I am a player, **when** I read the history, **then** I see only how many treasures were converted and the gold, never which ones.

#### In the app
- Editor: "Tesouro" in the point bar, with "Descrição para os jogadores", "Valor em ouro", "Marcar como encontrado" and "Desmarcar".
- Gold campaign: in "Experiência" on the "Mesa" tab, the strip "Encontrado, ainda não convertido" and "Voltar à cidade" beside "Dar XP".
- The "Voltar à cidade" dialog lists the found treasures (at most 100 at a time) and shows the math before the GM confirms.
- The GM can still type the gp in "Dar XP por ouro" ([MR-016](#mr-016-award-xp)).

#### Related
- Highlight "Mais tesouro encontrado": [MR-032](#mr-032-combat-highlights). Traps and chests: [MR-035](#mr-035-traps). See [Architecture](../architecture/maps/editor.md#layers).

### MR-042: Bestiary

**As a** game master, **I want** to search the SRD creatures and put one in the combat, **so that** I can build an encounter without making a sheet for every enemy.

- Priority: MVP
- Rules: RN-20, RN-29
- Modules: rules, play, characters

#### Acceptance criteria
- **Given** I am the GM of "Mirathel", **when** I open the "Bestiário", **then** I see the 334 SRD creatures with a Portuguese name, and I search by the Portuguese or the English name and filter by type, size and CR (ND).
- **Given** a creature of the bestiary, **when** I open it, **then** I see its sheet (AC, HP, speed, abilities, attacks and actions), with the SRD text in English.
- **Given** a combat being prepared or running, **when** I put three Goblins from the bestiary (1 to 10 at a time, never more than 40 combatants), **then** they enter as numbered NPCs, hidden by default, each with its own initiative (RN-19) and average HP, or rolled if I ask, **and** the player sees only the word for their state (RN-20, RN-29).
- **Given** no combat is open, **when** I put monsters from the bestiary, **then** the app starts one with them and the group, with a map or in theatre of the mind, as in "Iniciar combate".
- **Given** a combat with bestiary monsters, **when** it ends, **then** XP by enemies counts each one's CR, like an NPC's.
- **Given** a creature of the bestiary, **when** the GM chooses "Criar NPC", **then** the app makes a basic NPC sheet (a minion or a story NPC) with its numbers and up to three attacks, which the GM can rename and edit; the bestiary does not change.
- **Given** a creature with several attacks per action or with spellcasting, **when** I read the NPC made from it, **then** the description says so in Portuguese ("Ataques por ação: 2 (ataque múltiplo)"; the spell save DC, attack bonus and spells); the NPC casts those spells itself, with the creature's save DC and attack bonus, and its slots and daily uses are counted (only the master sees them); a hand-made NPC keeps the count into its next combat until the table's long rest, and a monster in combat starts each fight with all of them; an NPC made from the creature also keeps the uses a day of its actions and its Legendary Resistance uses until then.
- **Given** a monster with Multiattack on its turn (the animated armor: two slams), **when** I open its attack card, **then** it shows "Ataques múltiplos: 2 × Pancada" and I make the routine's attacks one after another, each with its own target; after the first the action reads "1 de 2" (not "Usada"), the second is offered, and the action counts as used when the routine is done or when I choose "Encerrar os ataques" (a choice of the page, not kept on the server: after a reload the rest is offered again); a third attack is refused, the log says each attack, and "Desfazer última ação" takes back the last one.
- **Given** a monster whose Multiattack has several routines or a choice between them, **when** I open its attack card, **then** I pick the routine to make; a creature without Multiattack keeps the card and the turn as before.
- **Given** a creature with Legendary Resistance (3/Day) whose saving throw I roll as the GM (an effect's, or the concentration's) and the roll fails, **when** I ask for Legendary Resistance with the roll, **then** the save becomes a success and one use is spent, the log tells me the roll and the choice ("restam 2"), and the players read only that it succeeded; a roll that succeeds spends nothing, and a creature without the trait or with no use left is refused with nothing changed.
- **Given** a player casts a spell with a saving throw at a creature with Legendary Resistance and it has a use left, **when** the cast waits for me (whether its save passed or failed), **then** I see the roll that failed and "Usar Resistência Lendária (restam K)" for each failure, and nothing to offer when every save passed; the ones I choose become successes with the same dice and spend one use each, the others stand, and the players wait on "Esperando o mestre", then read the ordinary result of the save, the same whether it passed or failed; no window opens for a creature without the trait or without uses, and "Desfazer última ação" on the cast gives the uses back.
- **Given** a player's melee weapon attack hits a creature whose stat block has Parry (Knight, Bandit Captain...) and it has not used its reaction this round, **when** the hit waits for me, **then** I see "Aparar (+N CA)" with the roll and the armor class; if I use it the kept roll is compared with the armor class plus N and a hit that falls short is a miss, if I let it pass the hit lands, and the players wait on "Esperando o mestre", then read an ordinary hit or miss and never the reaction, the bonus or the armor class; the reaction is back at the creature's turn, a second hit in the round opens no window, and "Desfazer última ação" on the Parry gives the reaction back.
- **Given** a creature whose reaction the app cannot resolve (Split, Shriek, Rock Catching...), **when** I read its sheet, **then** I read the reaction as text and no window opens for it.
- **Given** a creature with legendary actions (an adult red dragon: Detect, Tail Attack, Wing Attack), **when** another creature has the turn and I spend one, **then** the cost comes off the 3 it has that round and the players read only the ordinary attack or movement line; I cannot spend on its own turn, while it is incapacitated, beyond what is left or while a reaction waits for an answer, the actions come back at its turn, an option the app cannot resolve is offered as text and still counts, and "Desfazer última ação" gives the cost back.
- **Given** I am a player, **when** I look for the bestiary, **then** I find no entry point, and I never learn which creature a monster is, its CR or its rolled HP.

#### In the app
- The "Bestiário" row of the campaign page's "Preparação" tab (GM only), a list with search and filters, and a page per creature with "Criar NPC" and "Pôr no combate" (the sheet asks "Quantos", the base name, HP "Média" or "Rolar" and "Escondidos no início").
- In the session, the GM's order shows the monster's CR and the turn card links to "Ficha da criatura".

#### Related
- [MR-043](#mr-043-generate-encounters), [MR-044](#mr-044-generate-treasure), [RN-20](rules.md), [RN-29](rules.md). Custom monsters are not part of the MVP. See [Design](../design/combat.md#monsters-creatures-and-encounters).

### MR-043: Generate encounters

**As a** game master, **I want** to build an encounter knowing whether it is easy or deadly for my group, and generate one when I am out of ideas, **so that** I prepare the session faster.

- Priority: MVP
- Rules: RN-29
- Modules: rules, play, maps

#### Acceptance criteria
- **Given** the group of "Mirathel" (the campaign's living player characters, plus the NPCs I put in the group at the level I say), **when** I build an encounter with bestiary creatures and quantities, **then** I see the total XP and the difficulty ("Baixa", "Moderada", "Alta" or "Acima de alta") against the group's budget, with the label "Guia de dificuldade do SRD 5.2.1 (regras de 2024)" and the warning that, with the 2014 monsters, the encounter tends to be a little easier.
- **Given** a built encounter, **when** it has a creature above the group's lowest level plus 3, an empty group, or more creatures than one combat holds, **then** I read a warning.
- **Given** a built encounter, **when** I store it in a battle point on the map, **then** "Começar este combate" puts its monsters in the combat, as in [MR-042](#mr-042-bestiary), with the HP mode and the hidden choice, with a map or in theatre of the mind, and I can add the campaign's own NPCs (a boss, a named villain) in the same dialog, under "NPCs da campanha".
- **Given** a battle point that already holds an encounter, **when** I store another, **then** the app asks before replacing it, and "Tirar o encontro do ponto" removes it.
- **Given** a difficulty and, if I want, a creature type, **when** I ask "Gerar encontro", **then** the app builds one with SRD creatures, a leader and a group, that never exceeds the budget nor brings a creature with CR above the group's lowest level plus 3 (the group's NPCs count, at the level the GM gave); "Gerar outro" makes a new one, and "Trocar criatura" swaps a creature for another of the same XP.
- **Given** the same difficulty, the same options and the same seed, **when** I generate twice, **then** the result is the same.
- **Given** I am a player, **when** I open the builder or a battle point, **then** I read "Só o mestre monta encontros." and never receive the encounter or what it stores (RN-10).

#### In the app
- The "Encontros" panel of the campaign page's "Preparação" tab (GM only), with "Montar encontro", leads to the builder: the group in chips ("Pôr um NPC no grupo", level 1 to 20), a bar with the three bands, the creature list with search, and the generate, swap and store buttons.
- The numbers come from the "XP Budget per Character" table of SRD 5.2.1 (CC BY 4.0), credited on the "Créditos" page. SRD 5.1 has no difficulty table; whether it fits the 2014 monsters is for the table to judge after playing.
- In the session, a battle point that holds an encounter shows today's difficulty and "Começar este combate"; from a battle point on a map with a grid the dialog opens on "Com mapa".

#### Related
- [MR-042](#mr-042-bestiary), [RN-29](rules.md). See [Architecture](../architecture/play/encounters.md#the-encounter-builder).

### MR-044: Generate treasure

**As a** game master, **I want** to generate the treasure of a creature or a lair, with coins, gems, art objects and magic items, **so that** I can put it on the map without inventing everything on the spot.

- Priority: MVP
- Rules: RN-09, RN-10
- Modules: rules, maps, progression, characters

#### Acceptance criteria
- **Given** the group level (by default the lowest level among the living characters, changeable from 1 to 20), **when** I ask for an "Individual" or "De covil" treasure, **then** the app generates the coins (individual) or the coins, gems, art objects and the SRD 5.1 magic items (lair), from our own tables in Portuguese, with the Portuguese name and the value in gp. An artifact never comes out.
- **Given** a generated magic item, **when** I open "Ver descrição", **then** I see the rarity, the value with the label "Valores do SRD 5.2.1 (regras de 2024)" and the SRD text in Portuguese, with "Ver em inglês" for the original. A consumable is worth half the value of its rarity; a spell scroll is worth the whole value.
- **Given** a generated treasure, **when** I put it on a map ("Pôr no mapa"), **then** it becomes a hidden treasure point (RN-10) with the gold (coins, gems and art) as its value in gp and everything in the treasure, in Portuguese, in the description. Magic items never become XP. In a gold campaign the gold becomes XP at "Voltar à cidade" ([MR-041](#mr-041-treasure-and-xp-by-gold)).
- **Given** a treasure I already put on the map, **when** I press the button again by accident, **then** no second point is made; only "Gerar outro" brings the button back.
- **Given** a map without a grid, a square outside the grid, or a map with 200 points, **when** I try to place a treasure, **then** the app refuses and says why.
- **Given** the same request and the same seed, **when** I generate twice, **then** the result is the same, as long as the table content has not changed; if it changed, the app asks me to generate again.
- **Given** I am a player, **when** I look for the treasure page, **then** I read "Só o mestre gera o tesouro da campanha." and never see a placed treasure until it is marked found.

#### In the app
- "Gerar tesouro" in the "Mapas" panel of the campaign page's "Preparação" tab. The result also says what the campaign does with the gold (by enemies and by milestones it is not XP; by gold it converts at "Voltar à cidade").
- SRD 5.1 has no item values and no random treasure tables: the values come from SRD 5.2.1 and the tables of coins, gems and art objects are ours. See [Architecture](../architecture/maps/treasure.md#treasure-generator-mr-044) and [Rules content](../rules-content.md).

#### Related
- [MR-041](#mr-041-treasure-and-xp-by-gold), [MR-042](#mr-042-bestiary), [MR-043](#mr-043-generate-encounters). Custom magic items are not part of the MVP.

### MR-045: Look up spells

**As a** player, **I want** to look up all the table's spells in the app, **so that** I can read what each one does before choosing and during the session.

- Priority: MVP
- Rules: RN-23
- Modules: characters, rules

#### Acceptance criteria
- **Given** I am a player of "Mirathel", **when** I open "Magias", **then** I see all the spells available at the table (the SRD's and the table's that the GM left on), with search by name (Portuguese or English, no matter the accents) and filters by class, spell level and school, **and** each one opens its full description, with the target ("Cone de 4,5 m").
- **Given** a spell the GM turned off in "Opções para os jogadores", **when** I look for it, **then** it does not appear, unless it is on my sheet.
- **Given** a spell the GM turned off or archived while I read the list, **when** the session is open, **then** the list updates without reloading and the open card says "Esta magia não está disponível.".
- **Given** my character, **when** I turn on "Só as que posso aprender", **then** I see only the spells on my class's list up to the level I cast; on a multiclass sheet, those of each class, by the level in it. The GM has no such switch.
- **Given** a spell of the table, **when** I open it, **then** it carries the tag "Da mesa" and the GM's text; an SRD spell carries the Portuguese translation, with "Ver em inglês" and "Créditos". An archived spell reaches only the GM, with "Arquivada".
- **Given** a pending member, **when** they ask for the list, **then** they get nothing: only active members read it.
- **Given** the search, the filters and an open spell, **when** I share or reload the page, **then** they are in the link, and Back closes the spell and brings back the same list.

#### In the app
- `/campaigns/:id/spells`, from the "Magias" panel of the master's "Regras" tab (and of a player's campaign page), for every active member, the GM too. Three columns from 1100 px; below that one column, with "Filtros (n)".

#### Related
- A part of [MR-020](#mr-020-look-up-the-rulebook), with only the spells. Letting the GM accept a spell outside the class list is for after the MVP ([MR-046](#mr-046-table-style-feature-by-feature)). Table content: [MR-025](#mr-025-register-table-content).

### MR-048: Cast spells outside combat

**As a** player, **I want** to cast my spells between fights, **so that** I heal the party, prepare Mage Armor or cast a ritual without opening a combat. **As a** master, **I want** to confirm that the time of a long casting has passed, **so that** the slot is spent and the spell takes effect when the story says so.

- Priority: MVP
- Rules: RN-32, RN-10, RN-18, RN-20
- Modules: play, characters, rules

#### Acceptance criteria
- **Given** a cleric with a free slot and a hurt ally, **when** the player casts Cure Wounds with "Conjurar" and picks the ally, **then** the slot is spent, one roll heals the ally up to the maximum, and the party sees the cast in the log.
- **Given** a wizard with Detect Magic in the spellbook, **when** the player casts it "Como ritual", **then** no slot is spent, the casting takes 10 minutes more and the wizard concentrates on it from the start.
- **Given** a cast that takes minutes or hours, **when** the master taps "Concluir conjuração", **then** the slot is spent and the spell takes effect. **And when** the caster's concentration breaks first, **then** the cast fails and no slot is spent.
- **Given** a spell that lasts and a caster who concentrates on it, **when** the caster casts another concentration spell, **then** the screen asks "Isso encerra Bênção" first, and the first spell ends with the new cast.
- **Given** an active spell, **when** the master or the caster taps "Encerrar", **then** it ends and what it changed goes back (Mage Armor's armor class).
- **Given** a target that wears armor, **when** Mage Armor is cast on it, **then** the cast is refused with the reason.
- **Given** the master casts for an NPC with "Conjurar como NPC", **then** the NPC spends no slot and a cast on an NPC changes nothing about it.
- **Given** an NPC on the stage as a target, **when** a player reads the cast, **then** they read the NPC by its place on the stage and never the character ID, and a cast by an NPC that is not on the stage never reaches them (RN-10, RN-20).
- **Given** a combat running, **when** its caster tries to cast outside it, **then** the cast is refused and the spell is cast in the combat.
- **Given** Bless cast on a character before a fight, **when** the combat starts, **then** the effect enters it with the time it has left in rounds (a round is 6 seconds, a partial round does not count), the character's saves take the d4, and when the combat ends the rounds left go back to game time (RN-22).
- **Given** Aid cast outside combat, **then** the cast is recorded and the master narrates it; the maximum hit points are raised only by an Aid cast in combat.

#### In the app
- "Conjurar" lists the spells the character can cast, with the slots, "Como ritual" when the spell allows it, the time it takes and the targets. "Magias ativas" lists the casts going or lasting, with "Concluir conjuração" (master) and "Encerrar".

#### Related
- [RN-32](rules.md#rn-32-casting-outside-combat). In a combat the spells are cast there. See [Architecture](../architecture/play/casting.md#casting-outside-combat).

### MR-050: Campaign package, export and import

**As a** master, **I want** to export my campaign as one file and create it again from that file, on this server or another, **so that** the table's campaign moves without being typed again.

- Priority: MVP
- Rules: RN-10, RN-30
- Modules: campaignpackage, campaigns, characters, maps, play

**Export** (the "Pacote da campanha" panel of the "Regras" tab, "Exportar campanha"):
- Only the master. The page shows the progress and may be closed. The file (`<campanha>.meurpg.zip`) stays 24 hours ("Baixar de novo") and is then deleted; it is also deleted with the campaign or the account.
- It carries the campaign document, the table rules, the gallery (the image files), the maps (image, grid, layers, doors, lights, traps, points), the NPCs and creatures with the master's notes, the scenes with their actions, clues and hooks, the puzzles, the battle points with their encounters, the treasure points, the table content and the options switched off, and the players' characters (as reserved, with no owner).
- It never carries the players' private notes, the session history and logs, accounts (ids, e-mails, names), owners and members, invites and claim links, or what each player remembers of the fog. A map comes back as prepared: traps armed, treasures unfound.
- Every download checks again that the caller is the campaign's master.

**Import** (the campaigns page, "Importar campanha"):
- The file goes up in parts, so a dropped connection resumes. The upload belongs to the master's account, a person has one at a time, and unfinished parts are deleted an hour after the last one.
- A preview counts what the package holds and lists every problem in plain words. Any problem blocks "Criar campanha": all or nothing, and a failure halfway leaves nothing behind.
- A package is refused when it comes from a newer MeuRPG ("Este pacote é de uma versão mais nova do MeuRPG, e este servidor ainda não sabe abri-lo."), was changed, holds a field this server does not know, is over 200 MiB or 2,000 entries (10 MiB for an entry), is suspiciously compressed, has a name that climbs out of a folder, or refers to something it lacks.
- Every rule of the app applies as if the master had typed the data: names, sizes, DCs, scene actions, traps, puzzles, sheets (with the package's own table content), the caps of a campaign (300 images, 200 maps, 200 points a map, 1,000 table entries, the characters and NPCs cap, which does not count the NPCs made for creatures in a combat; they have their own cap of 1,000) and the account's cap of campaigns ("Você já tem 10 campanhas.") and the creators list. Images go through the upload's pipeline (re-encoded, metadata dropped).
- Ids are never reused: every thing gets a new id and every reference, including the links in the document, follows.
- The players' characters arrive reserved (MR-049): invisible to the players until one claims it with the master's link.

See [Architecture](../architecture/campaign-package.md).

### MR-051: The scene's conversation

**As a** master, **I want** a live text feed in the session where I narrate, speak as my NPCs and whisper to some players, and where the players answer in character or out of it, **so that** a table with a remote master plays without a call, and nobody reads what was not for them.

- Priority: MVP
- Rules: RN-10, RN-18, RN-36
- Modules: play, maps

#### Acceptance criteria
- **Given** an open session, **when** the master sends a line as the narrator, **then** every member reads it, in the order sent, in the session screen and in the session summary.
- **Given** the master speaks as an NPC of the campaign, **then** the line shows the NPC's name and its stage portrait when it has one; an id that is not an NPC of the campaign is refused.
- **Given** a player, **when** they send a line as their own character, **then** it shows the character's name; as another player's character, **then** it is refused (RN-18); out of character, **then** it shows their display name.
- **Given** the master chooses two players, **when** the line is sent, **then** only those two and the master receive its text, image, existence and notice; the third player gets nothing in the read, the stream, the summary or the voice (RN-36).
- **Given** a player chooses "só o mestre", **then** only they and the master read the line; a player cannot choose other players.
- **Given** a line, **when** it is longer than 2,000 characters or empty, **then** it is refused.
- **Given** the master attaches an image of the gallery, **then** the line's readers can open that image and nobody else can through that line; a player cannot attach an image.
- **Given** the master is typing, **then** the players see "O mestre está escrevendo…" for a few seconds; the hint carries no text and expires by itself.
- **Given** a line of the narrator or of an NPC, **when** a reader asks to hear it, **then** the first request makes the voice and keeps it with the line, the next ones are served from the stored file, and a non-reader gets `404`.
- **Given** the voice is not configured, **then** the audio route answers `failed_precondition` `OFF` and the app hides "Ouvir".
- **Given** the same send is repeated with its key, **then** one line exists; too many lines in a short time are refused (`resource_exhausted`).
- **Given** a player is erased, **then** their lines stay and the author reads "Jogador removido".

#### In the app
- The "Conversa" panel beside the stage and the map (the player's, from 1024 px, is a tab of the right column "Conversa · Ficha" beside the character's "Ficha"), and a tab of the pinned nav on the phone, where "Ficha" is a panel inside the session; the master's composer has "Falar como", "Para", "Imagem da galeria" and "Enviar"; the player's has "Como <personagem>" and "Fora do personagem".

#### Related
- [RN-36](rules.md#rn-36-who-reads-a-line-of-the-scenes-conversation), [RN-10](rules.md), [RN-18](rules.md), [MR-028](#mr-028-show-an-image-to-the-players), [MR-039](#mr-039-ai-generated-images-for-dungeons-and-scenes). See [Architecture](../architecture/play/live-session.md#the-conversation-of-the-scene) and [Privacy](../privacy.md#what-the-voice-sends-to-google-mr-051).

### MR-052: The free dice roller

**As a** master or a player, **I want** to roll any dice I ask for, in the session, and choose who sees the roll if I am the master, **so that** the table settles the rolls outside the game's rules (a rumour, a table of treasures, a bet) without a physical die at hand.

- Priority: MVP
- Rules: RN-10, RN-18
- Modules: play

#### Acceptance criteria
- **Given** an open session, **when** a member of the campaign rolls one die among d4, d6, d8, d10, d12, d20 and d100, from 1 to 20 of them, with a modifier from -20 to +20, **then** the server rolls them, and the total is the sum of the dice plus the modifier ("2d6 + 3" with 4 and 4 totals 11).
- **Given** a roll with another die, with 0 or more than 20 dice, or with a modifier beyond +20 or -20, **then** it is refused and nothing is rolled or written.
- **Given** one d20, **when** the roller chooses advantage or disadvantage, **then** two d20 are rolled, the higher (or the lower) counts and the other shows "descartado"; with any other dice the choice is refused.
- **Given** the table plays with physical dice (RN-18), **when** a member rolls here, **then** the app rolls anyway, the log says "Rolagem livre", and the sheet says "Esta é a rolagem do app": the table's dice mode applies to the game's rolls, not to this one.
- **Given** a player rolls, **then** the roller reads the result card (the total in numbers and in words, the formula, each die), and the whole table reads "Rolagem livre · <name>: 2d6 + 3 = 11 (4, 4)" in "Rolagens da mesa" and sees the dice on the screen.
- **Given** the master rolls "Para a mesa", **then** the table reads and sees it as a player's roll.
- **Given** the master rolls "Só eu", **then** only the master reads it, with the "Só você" badge; no player receives it in a read, on the stream or in the summary, and nothing tells a player that it happened, not even a count (RN-10). A player who asks for "Só eu" is refused.
- **Given** a natural 20 or a natural 1 on a single d20, **then** the result says it in words ("20 natural", "1 natural"), not only in colour.
- **Given** the same tap is repeated with its key, **then** one roll exists, and the same key with another request is refused; a double tap on the button rolls once.
- **Given** a member rolls 20 times in a row and once more, **then** the last one is refused (`resource_exhausted`) and the sheet says "Tente de novo em N segundos".
- **Given** the sheet is open, **then** it lists the member's last 10 rolls of the session (the master's secret ones included, with "Só você"), and "Repetir 2d6 + 3" fills the form and rolls the same dice again.
- **Given** the live stream is reconnecting, **then** "Dados" is disabled and the screen says why; **given** no session is open, **then** there is no "Dados" button, and the call is refused.
- **Given** a person who is not an active member of the campaign, **then** the call is refused.

#### In the app
- The "Dados" button in the session's header opens "Rolar dados": a bottom sheet on a phone, a dialog from a tablet up. It has the shortcuts (1d20, 1d6, 2d6, 1d100), the die, "Quantidade (1 a 20)", "Modificador (−20 a +20)", "Como rolar o d20" for a single d20, the master's "Quem vê" ("Para a mesa" or "Só eu"), "Suas últimas rolagens nesta sessão" with "Repetir", and one filled button ("Rolar 2d6 + 3"). The result card has "Mudar dados" and "Rolar … de novo".
- "Rolagens da mesa" under the stage lists the session's free rolls, newest first; the dice overlay, a felt tray where the dice roll and land on the faces the server rolled, shows the others' roll as "<name> rolou".
- The overlay's "Som dos dados" button switches the dice's sound on, quietly; it is off until the player taps it and stays on for the open tab only.
- Rolling is rate limited per person.

#### Related
- [RN-10](rules.md), [RN-18](rules.md), [MR-051](#mr-051-the-scenes-conversation). See [Architecture](../architecture/play/free-dice-roller.md) and [Design](../design/session.md).

### MR-049: Reserved characters and claim links

**As a** game master, **I want** to make characters for my players (or bring them in with the campaign) and send each player a link to take theirs, **so that** the table starts playing without everybody building a sheet first.

- Priority: MVP
- Rules: RN-01, RN-03, RN-10, RN-16
- Modules: characters, campaigns, identity, play

#### Acceptance criteria
- **Given** I am the master, **when** I choose "Criar personagem para um jogador" and save, **then** the character is **reserved**: it has no owner, it appears in "Personagens reservados" and no player sees it, in any list, party view, combat order, map token, session summary or image. It cannot be marked dead.
- **Given** a reserved character, **when** I choose "Gerar link para o jogador" (valid for 1, 7 or 30 days, 7 by default), **then** I get a link `<app>/claim#t=<token>` that the app shows **once**, whole, with "Copiar link"; making another link revokes the previous one.
- **Given** the link and no session, **when** someone opens it, **then** they see the same page as for any link, valid or not (what a claim link is and "Entrar com Google"), with nothing about the campaign or the character, **and** signing in only brings them back to the character's card: signing in never claims.
- **Given** the card, **when** I press "Assumir este personagem", **then** I become a member of the campaign (no approval step; a pending join request is closed) and the owner of the character, and the link is spent.
- **Given** a link that is invalid, expired, used or revoked, or a claim that loses a race with a revoke or with another claim, **when** anyone opens it, **then** they read the same page, "Este link não pode ser usado".
- **Given** a claim link that is not usable, or an invite link that is invalid or expired, **when** anyone opens it, **then** the page also says: "Se o mestre separou um personagem para você, peça o link de assumir."
- **Given** I already have a living character in the campaign, **when** I claim another, **then** I am refused with the one specific reason: I read who the living character is and what to do (a dead one does not count).
- **Given** I am the master and open my own link, **then** I read "Este link é para um jogador" and nothing changes.
- **Given** a reserved character with a link, **when** I revoke it, **then** the row asks in place and the link stops working; **when** a player used it a moment before, **then** I read "Este link já foi usado por …" and the list is read again.
- **Given** a character a player claimed, **when** I choose "Devolver à reserva" (a question in place), **then** the character has no owner again and no player sees it, the player stays a member, and I can send it to someone else.
- **Given** a reserved character, **when** I choose "Excluir" (a question in place), **then** the character and its live link are deleted.
- **Given** a reserved character, **when** a session starts, **then** it is not locked with the others; it locks with the sessions that start after it is claimed.

#### In the app
- The "Personagens reservados" panel of the campaign page's "Mesa" tab, under the players' characters: each row says where its link stands and offers "Gerar link para o jogador" / "Gerar novo link", "Editar", "Revogar o link", "Excluir" and, for a claimed character, "Ver ficha" and "Devolver à reserva".
- The master makes the sheet in the usual editor, in the master's mode (the table's ability-score method and hit-point rule are not applied), and saves with "Salvar como reservado".
- The card at `/claim` offers "Assumir este personagem", "Voltar para minhas campanhas" and "Não é você? Entrar com outra conta".
- Claiming is rate limited per person and per address.

#### Related
- [MR-002](#mr-002-generate-an-invite) and [MR-003](#mr-003-join-through-the-invite) (the other way into a campaign), [MR-024](#mr-024-approve-the-character-from-the-invite) (a pending member), [MR-006](#mr-006-locked-sheet). The package of a campaign ([MR-050](#mr-050-campaign-package-export-and-import)) brings its characters in as reserved. See [Architecture](../architecture/characters.md#reserved-characters-and-claim-links).

### MR-053: The master acts for a player's character

**As a** master, **I want** to play a player character's turn when its player is not there (or does not answer), **so that** the combat does not stop and the table keeps the player's character in the game.

- Priority: MVP
- Rules: RN-03, RN-10, RN-18, RN-19, RN-20
- Modules: play

#### Acceptance criteria
- **Given** a player character's turn, **when** the master reads "Turno", **then** the card of that character shows "Agir por <name>" on every player turn: there is no "absent" mode to switch on first. A character at 0 hit points has no "Agir por": its card is the death save (below).
- **Given** the master taps "Agir por <name>", **then** he gets the character's own turn: its economy, its action tabs built from its sheet, its attacks, spells, features, Wild Shape, summons and familiar, its movement and target picking, as the player has them, and "Voltar ao mestre" (which gives the rail back without ending the turn); the combat bar says "Você age por <name>" and its one filled button reads "Encerrar o turno de <name>" in place of "Próximo turno", so there is one end-of-turn button on the screen.
- **Given** the master acts, **when** he moves, **then** he drags the character's token or uses "Mover" in the map's header (it is not in the rail) while no target is being picked.
- **Given** the master acts, **when** he picks a target, **then** only what the character sees counts (the server's target list for that character: a hidden creature is dimmed, "<name> não vê este alvo"), and the server refuses any other target the same way (RN-10, RN-20).
- **Given** the master did something for a character (an attack, a spell, a feature, a death save, a move, the end of the turn), **then** the combat log says "(pelo mestre)" at the end of the line, for everyone who reads it, the player too when they are back.
- **Given** the player comes back in the middle of the turn, **then** either of them may act; nothing is locked, and the second answer to the same question reads "Já respondido".
- **Given** the master acts, **then** the order's tools (damage, conditions, the master's marks) stay available.
- **Given** the combat is in its setup, **then** "Começar o combate" waits for every initiative; "Rolar por <name>" rolls the missing one with the character's bonus, "Digitar o resultado" types it for a physical die, and the bar says who is missing. The same "Rolar por <name>" is beside each character who has not answered a group check or a check the master asked for in a scene, with the typed result when the table plays with physical dice.
- **Given** a player's optional reaction (Escudo Arcano, Esquiva Sobrenatural...) nobody answered in 10 s, **then** the question goes to the master, at the top of "Turno", with what it changes in numbers (the attack against the armor class, with and without the spell), what it costs, the tag "10 s sem resposta", and "Usar" and "Não usar"; the tab says "Turno · 2" and the combat bar "Falta responder por <name>". The first answer wins (the player's or the master's); the other reads "Já respondido".
- **Given** a character at 0 hit points on its turn, **then** its turn starts with its death save (SRD 5.1) and does not end without it, for the master too: the card leads with "<name> está caído: teste contra a morte", the successes and failures so far, "Rolar por <name>" (the app's dice) and "Digitar o resultado" (a physical die) at the same size; "Próximo turno" is dashed and the bar says "Falta o teste contra a morte". The server refuses the end of that turn until the save is made; the old "Passar sem rolar" is gone.

#### In the app
- "Turno" on a player's turn: the card "Vez do Pensantus" ("Mago 3 · o jogador joga na tela dele"), the hit points, armor class and movement, the economy shapes, "Esperando Pensantus. Se o jogador não estiver, jogue este turno por ele." and one outlined button, "Agir por Pensantus". Acting, the rail starts with the band "Você está agindo por Pensantus" and "Voltar ao mestre", then the shapes, "PV 23 de 23 · CA 12 · 1º ○○○● 2º ○○" and the character's tabs (on a phone without their counts); the target is picked on the map ("Toque no alvo · por Pensantus") and a phone's pinned bar becomes "Encerrar o turno de Pensantus", or the target bar of the map.
- A reaction nobody answered, a held roll and the group check keep their cards at the top of "Turno"; the setup's initiative rows have "Rolar por Brisa" beside "Digitar pelo jogador".

#### Related
- [MR-013](#mr-013-turn-order), [MR-014](#mr-014-your-turn), [RN-03](rules.md), [RN-10](rules.md), [RN-18](rules.md). See [Design](../design/combat.md).

### MR-054: Use a creature's action from its turn

**As a** game master, **I want** to use a monster's breath weapon, gaze or other action from the NPC's card, **so that** the app rolls the saves and the damage the stat block calls for and I do not do it by hand.

- Priority: MVP
- Rules: RN-20, RN-22, RN-29, RN-38, RN-39, RN-41, RN-42
- Modules: play, rules

#### Acceptance criteria
- **Given** a bestiary monster on its turn, **when** I read its card, **then** "Outras ações" lists its actions that ask a saving throw or roll nothing, each with what the app read (the ability and the DC, the area, the damage and "metade se passar", the condition a failure gives) and a "Usar" button; the attacks and the Multiattack are not there, they are made with the attack card (MR-042).
- **Given** an action the app reads only in part or not at all, **when** I read it, **then** the card says so ("O texto diz algo que o app não calcula: leia o lembrete.") and shows the SRD text, in English.
- **Given** a combat on a map and an action with a shape (a cone, a line, a sphere), **when** I press "Usar", **then** I place it on the map as I place a spell's area and press "Usar <action>"; nothing is spent before that, and the server takes who is inside. **Given** a combat with no grid, or an action with no shape, **then** I name the targets (the enemies come marked) and "Usar" waits for at least one.
- **Given** I used the action, **then** the app rolls each target's save against the stat block's DC, the action is spent, and under the list I read for each target "Falhou" or "Passou: metade do dano", the roll against the DC and the condition it got; the damage it opened is there to apply or discard, and a player's character waits for me to apply it.
- **Given** the action's save names a condition, **then** a target that fails gets it unless it is immune or already has it, and the result notes the duration and the repeated save for me to keep; the app counts no time.
- **Given** an action with no roll (Invisibility, Change Shape), **when** I use it, **then** it spends the action, the card shows the SRD text with "Lembrete registrado no registro do combate. A ação foi gasta." and the log says "usou <action>: lembrete, a ação foi gasta".
- **Given** the NPC's action is already spent, **then** every "Usar" is off and says "A ação deste turno já foi usada."; **given** it is not the NPC's turn or I am not the master, the server refuses.
- **Given** I undo the last action, **then** the use goes with its damage and condition.
- **Given** an action with a limit ("Recarga 5–6", "3/dia" or after a rest) that I used, **then** it is spent, and the server refuses it again with nothing changed ("Essa ação da criatura ainda não recarregou. Nada foi gasto." or "Essa ação da criatura não tem mais usos. Nada foi gasto."); an action with uses a day stays available until its uses are gone.
- **Given** a spent Recharge action, **when** its creature's turn starts, **then** the server rolls a d6 for it, in the stat block's order, and a face in its range makes it available again; I read the d6 and the result in the log, as the master and as the player who controls the creature (no other player reads them), a defeated creature does not roll, and an action with uses a day or after a rest does not come back in the combat.
- **Given** I undo the use of a limited action, **then** it is available again or has the use back; the roll at a turn start cannot be undone.
- **Given** an NPC made from a creature spent uses of an action with uses a day or of Legendary Resistance, **when** a new combat starts with it, **then** I read the uses it has left, until the table's long rest gives them back; a monster of the bestiary starts every combat with all its uses, and a spent Recharge action is available again.
- **Given** I am a player who does not control the creature, **then** I read nothing of what it spent, of the roll or of the refusal, and a turn with a roll looks the same as one without (RN-20).
- **Given** I am the player who controls the creature, **then** I read what each limited action has left, the d6 in the log and the reason a spent action is refused.
- **Given** I am a player, **then** I find no "Outras ações", I read the log line with the action's name and, for the targets I see, whether each passed or failed and the damage I take, and never the stat block, the DC or which creature it is (RN-20).

#### In the app
- The NPC card of the master ("Turno"): "Outras ações", with the targets list "Quem a ação atinge" when it applies, one block per action (name, usage, lines, note, SRD text, "Usar") and the result under the list with the pending damage. On a map the placing dialog is "<NPC> usa <action>". See [Design](../design/combat.md#a-creatures-other-actions).

#### Related
- [MR-042](#mr-042-bestiary), [MR-055](#mr-055-answer-a-creatures-rider), [RN-38](rules.md#rn-38-a-creatures-save-actions), [RN-41](rules.md#rn-41-actions-without-a-roll).

### MR-055: Answer a creature's rider

**As a** game master, **I want** the app to offer me the extra effect of a monster's attack once it hits (a ghoul's paralysis, a snake's grapple), **so that** I apply it with the right save and DC, or set it aside.

- Priority: MVP
- Rules: RN-20, RN-29, RN-34, RN-39, RN-40
- Modules: play, rules

#### Acceptance criteria
- **Given** a monster whose attack asks a save of the target when it hits (the ghoul's claws), **when** the damage of a hit lands on a creature of the other side that is still up, **then** I get an offer beside the damage cards: "Pedir teste de resistência de <target> (CD 10, Constituição). Se falhar: Paralisado", with "Aplicar" and "Dispensar".
- **Given** I press "Aplicar", **then** the app rolls the target's save against the DC, opens the extra damage as pending damage when the attack has it (half if the target passes, when the stat block says so), gives the condition to a target that fails (RN-39), and the log says "usou <attack> em <target>: <target> falhou no teste (1d20 (8) + 2 = 10 contra CD 10); ficou Paralisado".
- **Given** a monster whose attack grapples (a constrictor), **when** it hits, **then** the offer reads "Agarrar (CD de fuga 12)"; "Aplicar" gives the condition with no save and the line notes the escape DC.
- **Given** I press "Dispensar", **then** nothing is applied and the log says "dispensou o efeito de <attack> em <target>".
- **Given** the offer was answered, **then** it cannot be undone; **given** I undo the damage that opened it, **then** the offer goes away, and a target the hit downed never gets one. An attack offers its rider once for each damage.
- **Given** I am a player, **then** I never receive the offer and cannot answer one; I read the line of the answer with the attack's name and the target, and the roll of the save only on a character that is mine, never the DC (RN-20).

#### In the app
- The same place as the monk's offers, beside the damage cards: a card with the paw icon, the attack's name and the target, one line of what it does, "Aplicar" and "Dispensar". See [Design](../design/combat.md#a-creatures-other-actions).

#### Related
- [MR-054](#mr-054-use-a-creatures-action-from-its-turn), [RN-40](rules.md#rn-40-a-rider-on-a-creatures-hit).

## Priority: MVP (prerequisite)

### MR-002: Generate an invite

**As a** master, **I want** to generate an invite link, **so that** players can join the campaign.

- Priority: MVP (prerequisite)
- Rules: RN-07
- Modules: campaigns

#### Acceptance criteria
- **Given** I am the master of "Mirathel", **when** I generate an invite, **then** I get a link valid for one person for 7 days **and** the server stores only the hash of the token.
- **Given** I am the master, **when** I choose, **then** I can set from 1 to 20 uses and a validity from 5 minutes to 30 days (the screen offers 1, 7 or 30 days), and the link is shown once, with a warning and a copy button.
- **Given** an invite that has not been used, **when** the master revokes it, **then** the link stops working **and** whoever already joined stays in the campaign.
- **Given** the invites list, **when** the master opens it, **then** each invite shows its uses, validity, whether it requires approval and its state: "Ativo", "Usado", "Expirado" or "Revogado".
- **Given** I am a player in "Mirathel", **when** I try to generate, list or revoke invites, **then** the server refuses.

#### Related
- [RN-07](rules.md). RN-15 (invite with approval) and [MR-024](#mr-024-approve-the-character-from-the-invite) extend this story: the master ticks "Exigir aprovação do mestre" on the invite, the player creates the character through the invite, and the master approves.

### MR-005: Create NPCs

**As a** master, **I want** to create NPCs of each kind (enemy, boss, minion, story), with a full or a basic sheet depending on the kind.

- Priority: MVP (prerequisite)
- Rules: RN-04
- Modules: characters

#### Acceptance criteria
- **Given** I am the master of "Mirathel", **when** I create an enemy or a boss, **then** it has a full sheet, with the calculated values; **when** I create a minion or a story NPC, **then** it has a basic sheet. A sheet of the wrong kind is refused.
- **Given** an NPC, **then** it belongs to the master and stays in the campaign where it was created.
- **Given** I am a player in "Mirathel", **when** I open the campaign, **then** I see no NPC **and** the server refuses if I try to create one.
- **Given** an NPC created in "Mirathel", **when** the master opens another campaign, **then** the NPC does not appear there: using the same NPC in other campaigns is [MR-022](#mr-022-reuse-npcs).

## Priority: Later

Proposals for after the MVP, in the order of the [roadmap](../roadmap.md).

### MR-007: Import a sheet from PDF

**As a** player, **I want** to import my sheet from a PDF, choosing the format (D&D Beyond or a Portuguese sheet), **so that** I do not type everything again.

- Priority: Later
- Rules: RN-08
- Modules: characters

#### Related
- RN-08: no DOCX; only an editable PDF, in the D&D Beyond format or the Portuguese sheet.

### MR-017: Level up

**As a** player, when I level up, **I want** to choose what I gain from my class, subclass or a second class, following the D&D 5e rules.

- Priority: Later
- Rules: RN-12
- Modules: progression, rules

#### Related
- The guided sheet edit is already in the MVP: [MR-040](#mr-040-level-up-from-the-sheet). This story is the complete level-up screen.
- See [ADR-0008](../adr/0008-rules-content-as-data-pure-engine.md).

### MR-020: Look up the rulebook

**As a** master, **I want** to look up the rulebook with a smart search.

- Priority: Later
- Rules: —
- Modules: rules

Spells are in the MVP: [MR-045](#mr-045-look-up-spells).

### MR-021: Copy a character

**As a** player, **I want** to copy my character to another campaign, **so that** I can play both at the same time or a continuation.

- Priority: Later
- Rules: RN-03, RN-23
- Modules: characters

#### Acceptance criteria (proposal)
- **Given** a sheet that uses table content (a class, race or spell the master registered), **when** the player tries to copy it to another campaign, **then** the app refuses and says why: table content is valid only in its own campaign (RN-23, from MR-025).

#### Related
- [RN-03](rules.md).

### MR-022: Reuse NPCs

**As a** master, **I want** to use my NPCs in several campaigns, **so that** I do not recreate the same villain.

- Priority: Later
- Rules: RN-04, RN-23
- Modules: characters

#### Acceptance criteria (proposal)
- **Given** an NPC that uses table content, **when** the master takes it to another campaign, **then** the app refuses and says why (RN-23, from MR-025).

### MR-023: Hand over or share the campaign

**As a** master, **I want** to hand my campaign over to another master, or have a second master in it, **so that** the campaign goes on even if I leave.

- Priority: Later
- Rules: RN-13
- Modules: campaigns

#### Consequence
Deleting the account of whoever created the campaign deletes the whole campaign (see [Privacy](../privacy.md#delete-the-account)). With more than one master, or after a hand-over, the campaign would be deleted only when the last master leaves. See [ADR-0011](../adr/0011-authorization-roles-per-campaign-checked-per-request.md), as a proposal.

### MR-026: Propose a new race or class

**As a** player, **I want** to propose a race or class that does not exist in the app when I create the character, with the PDF or the link to the rules, **so that** the master can read it and decide.

- Priority: Later
- Rules: RN-15 (the same idea of approval as the invite)
- Modules: rules, characters

It builds on MR-025, and the player's proposal is valid only after the master approves it.

#### Acceptance criteria (proposal)
- **Given** I want to play a cook, a fan-made class, **when** I create the character and propose the class with the link to the PDF, **then** the master sees the request **and** the character waits for the decision.
- **Given** a request for a new class, **when** the master approves it and registers its rules (MR-025), **then** the character uses the class; **when** the master refuses, **then** the player picks another class.

### MR-027: Read the rules from a PDF

**As a** master, **I want** to upload the PDF with the rules and see the app register its classes, races and rules by itself, **so that** I do not type everything.

- Priority: Later
- Rules: —
- Modules: rules

It builds on the master's own registration (MR-025), and the master reviews everything before it takes effect.

#### Acceptance criteria (proposal)
- **Given** a rules PDF uploaded by the master, **when** the app reads it, **then** it shows the master what it found **and** nothing is valid in the campaign until the master reviews and approves.
- **Given** an uploaded PDF, **when** processing ends or fails, **then** the file is deleted; a short TTL on the stored file guarantees the deletion even if processing fails.

#### Open points
- Reading a rules PDF automatically needs an AI service, which costs per use and receives the PDF. An official book is under copyright: the app cannot redistribute the text, and the result may appear only to the table. When the PDF cannot be read, the registration stays with the master (MR-025). The app does not keep the PDF: it stays only while it is processed (see [Privacy](../privacy.md#to-be-defined)).

### MR-046: Table style, feature by feature

**As a** master, **I want** to choose, feature by feature, what the app does at my table, **so that** I use the app the way I run games, from all-digital to the classic table.

- Priority: Later
- Rules: RN-24
- Modules: campaigns, play, maps, characters

The MVP has the three ready-made styles of the "Regras da mesa" page (RN-24); the rest of the ideas are kept here.

#### Ideas (from the personas: the narrator master, the physical-table master, the tactical one, the beginner, the improviser and the table without phones)
- The app warns instead of blocking: movement beyond the speed, range, spell slots (the master decides).
- Hide the maps from the players, or the player's phone shows only the sheet.
- Who acts on the phone: the players, or the master enters everything and the players only follow.
- Who levels up: the player from the sheet ([MR-040](#mr-040-level-up-from-the-sheet)) or only the master.
- The state words of enemies ("Ferido"): show or hide.
- The master accepts a spell outside the class list ([MR-045](#mr-045-look-up-spells)).
- One switch per feature, with the ready-made styles as a starting point.

### MR-047: More puzzles

**As a** master, **I want** more puzzle types and more ways to give a reward, **so that** the challenge fits the story of my campaign.

- Priority: Later
- Rules: RN-27
- Modules: play, maps

The MVP already has six types, hints, the hint by skill check, split information, consequences and "Ao resolver" ([MR-038](#mr-038-puzzles)); the rest of the ideas are kept here.

#### Ideas
- Sliding tiles.
- Pressure plates on the map grid (step on the squares in the right order).
- Scales and weights.
- A light beam and mirrors on the grid (using the map's vision).
- Symbols taken from the master's gallery, and a text for each state of the puzzle.
- Rewards: XP, an item placed as treasure on the map, a story note.

## See also

- [Business rules](rules.md)
- [Product vision](vision.md)
- [Glossary](glossary.md)
- [Roadmap](../roadmap.md)
