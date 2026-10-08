[Português (Brasil)](../pt-BR/produto/historias.md)

# Stories and acceptance criteria

The MVP has 37 stories: 35 plus 2 prerequisites (the invite, MR-002, which leads to MR-003, and the NPCs, MR-005, who are the combat enemies). Ten more stories are planned for after the MVP.

A story is done when all its criteria pass. Each criterion becomes an automated test: Playwright for what shows on screen, a Go test for the rule on the server. There are no characterization tests of the legacy app; the new system only has to prove its own acceptance criteria.

How to read a story: the user-story sentence, the priority (MVP, MVP prerequisite or Later), the rules (RN-xx, in [Business rules](rules.md)) and modules it touches, the acceptance criteria (Given / when / then), and "In the app", a short summary of what exists now. Stories marked Later are proposals; the data model is already prepared for MR-021 and MR-022 (see [Data model](../data.md)). Where a behaviour came from a product answer, the [archive](../archive/decisions.md) has it.

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
- **Given** I am already master of the maximum number of campaigns for the account (10 by default), **when** I create another, **then** nothing is created **and** the screen says, in place of the form, what the maximum is (RN-30).
- **Given** the server lets only some e-mails create campaigns, **when** an account with another e-mail tries to create one, **then** it is refused with the reason and the screen explains it; people who join by invite keep playing (RN-30).

#### In the app
- Module `campaigns` (see [Architecture](../architecture.md#campaigns-module-and-authorization)).
- Page `/campaigns` (`web/src/app/pages/campaigns/`): the list shows the role in each campaign (Mestre or Jogador), one row per campaign, plus the "Criar campanha" form. An empty list explains how to create a campaign or join by invite.
- The cap per account (RN-30) and the creators' allow-list are enforced by the server; the screen shows the cap sentence in place of the form.
- Tests: `TestMR001_CreatorBecomesMasterAndOnlyMembersSeeTheCampaign`; `TestRN30_TheCampaignCapPerAccount`, `TestRN30_TheCapHoldsUnderConcurrentCreations`, `TestRN30_TheCreatorsAllowList`; the cap sentence in `campaigns.spec.ts` (Vitest); Playwright `o mestre cria uma campanha pela tela e a vê como mestre na lista` (`@MR-001`, `e2e/tests/campaigns.spec.ts`).

### MR-003: Join through the invite

**As a** player, **I want** to create my character through the invite link, **so that** it is linked to the campaign from the start.

- Priority: MVP
- Rules: RN-03
- Modules: characters, campaigns

#### Acceptance criteria
- **Given** a valid invite to "Mirathel", **when** the player opens the link and signs in with Google, **then** they become a player of the campaign and create a player-type character, which the master already sees in the campaign.
- **Given** an expired invite, **when** someone opens the link, **then** they see a clear message **and** nothing is created.

#### In the app
- With a signed-in user, the invite becomes a player membership. Accepting again changes nothing. Two players racing for the last use do not both get in.
- A visitor who is not signed in joins in one step: `POST /auth/login` with the intent `campaign_invite` signs in and accepts the invite, stores nothing in the browser, and lands on `/campaigns/<id>` or on `/invite/error?reason=<code>`.
- Page `/invite` (`web/src/app/pages/invite/invite-accept.ts`): reads the token from the URL fragment, erases it from the URL at once (`history.replaceState`), and accepts the invite (signed in) or offers "Entrar para aceitar o convite" (signed out).
- The player creates their own character, player-type, with `CharacterService.CreateCharacter`. It is born as a draft. The master sees it in the campaign list with the player's display name, class and race (`ListCharacters`). The creation editor is the step-by-step editor of [MR-004](#mr-004-character-sheet-in-the-pdf-format).
- Tests: `TestMR003_ValidInviteMakesTheUserAPlayerTheMasterSees`, `TestMR003_ExpiredInviteGivesAClearErrorAndCreatesNothing`, `TestMR003_PlayerCreatesTheirCharacterAndTheMasterSeesIt`; with a fake provider and CockroachDB: `TestSignInWithAnInviteJoinsTheCampaign`, `TestSignInWithAnUnusableInvite`, `TestSignInWithAnInviteAsAMember`. Playwright (`@MR-003`, `e2e/tests/invite.spec.ts`): `jogador já logado abre o link do convite, entra na campanha e o mestre o vê nos membros`, `visitante sem sessão entra pelo convite, faz login e é adicionado à campanha automaticamente` (it also checks that the token appears in no URL the browser requests), and "o jogador entra pelo convite, cria o personagem e o mestre o vê na campanha".

#### Related
- RN-03: a player creates a new character in this campaign only when the current one dies; the dead character stays in the system (see [Business rules](rules.md)).
- RN-17 covers the login of a player without Google (a per-table handle): the player joins without a password and, when the first 30-day session expires, must set a password or link Google (ADR-0009, option 3).
- When the invite requires approval (RN-15), the player joins as a pending member, goes straight to creating the character, and the character is born pending until the master approves or refuses it. See [MR-024](#mr-024-approve-the-character-from-the-invite).

### MR-004: Character sheet in the PDF format

**As a** player, **I want** to see my sheet in a format close to the official PDF, **so that** I find everything where I am used to.

- Priority: MVP
- Rules: —
- Modules: characters, rules

#### Acceptance criteria
- **Given** a complete character, **when** the player opens the sheet on a phone, **then** they see the sections of the official sheet (abilities, skills, combat, spells, equipment) **and** the calculated values, such as modifiers and spell save DC, come ready from the server.

#### In the app
- `CharacterService.GetCharacter` returns the sheet as the player filled it in, plus the values the server calculates (`DerivedSheet`): abilities and modifiers, saving throws, skills, passives, AC, HP, speed, senses, spell save DC and spell attack, slots (a Warlock's Pact Magic slots in their own list, "Espaços do pacto", since they are all of one level), spells, attacks, features and the sheet's pending items. Test: `TestMR004_SheetComesWithServerCalculatedValues`, with Pensantus (INT 18, +4; DC 14; spell attack +6; AC 13; HP 23). Playwright: "o jogador abre a ficha no celular e vê as seções da ficha oficial com os valores calculados pelo servidor".
- Subclass in the editor: it offers "Nenhuma", says at which level the class picks it ("O Bárbaro escolhe a subclasse no nível 3."), and changes the subclass when the class changes.
- "Magias" step: lists only spells up to the highest spell level of the current level, ordered by spell level and then name. A spell already chosen above the limit stays in the list, marked "acima do nível", so it can be unchecked. Classes that start casting later (Paladin, Ranger) show "O Paladino conjura magias a partir do nível 2." at level 1. "Truques" appears only for a class that has cantrips on its list (Paladin and Ranger do not), or when the sheet already has a cantrip, so it can be unchecked. The highest level per level comes from the server (`ClassSpellcasting.max_spell_level_by_level`, in `ContentService.ListContent`).
- Ability scores, step "Habilidades": "Como definir os valores" has three cards: "Digitar" (the six usual fields), "Rolar 4d6" and "Conjunto padrão".
  - "Rolar 4d6" rolls 4d6 six times in the browser (`crypto.getRandomValues`, unbiased), strikes through the lowest die of each, lists the results from highest to lowest, and has "Rolar de novo", which replaces all six and undoes the placement.
  - The player puts each result on an ability. From tablet width up, each ability is a selector (choosing a result already on another ability swaps the two). On a phone, the player taps the result ("Escolhido") and then the ability, with the words "Livre", "Escolhido" and "em Força".
  - "Conjunto padrão" works the same with 15, 14, 13, 12, 10 and 8, no dice.
  - While a result is left without an ability, the editor does not save and says "coloque cada resultado numa habilidade" (it never turns into six 10s unseen).
  - Nothing is stored: the sheet receives only the placed number. The rolls run in the browser and are not recorded; the sheet stays editable until the first session and the master reviews it. These rolls are not part of RN-18.
- Hit points, "Pontos de vida": "Rolado" shows one row per level from the 2nd ("Nível 2 (1d12)"), "Rolar", "Rolar os níveis que faltam" (only the empty ones; the player can type what came up on a table die), the formula "1d12 (8) + 3 (Constituição) = 11 PV" with the final Constitution (score, race, subrace and manual bonus), and a box with the total so far and the final range. The multiclass rolls each level with the die of the class that gives it. The editor does not save while a level has no roll or a roll that does not fit the die of its level, and the notice under the steps names the level ("Dado de vida do nível 3"). It is a preview: it leaves out the hit points that the race, the class or a feature add (Dwarven Toughness, Draconic Resilience), and the server recalculates the maximum on save (`rules.Derive`).
- Spell "?": beside each spell, a 44 px "?" opens the description (dialog on desktop, bottom sheet on phone): name, spell level and school, casting time, range (in metres: 5 ft = 1.5 m), components and duration in Portuguese, the Ritual and Concentração tags, and the SRD text in English marked `lang="en"` ("Texto do SRD 5.1 (em inglês)", plus "Em níveis superiores" when present). It comes from `ContentService.GetSpellDetails`, fetched on demand and kept only while the page is open. What the formatter cannot translate appears as the raw SRD text with the note "(texto do SRD)".
- Tests: `TestMaxSpellLevelFromSlots`, `TestCatalogMaxSpellLevelByLevel` (`rules`), `TestCatalogToProtoMaxSpellLevel` (`characters`); Vitest `dice.spec.ts`, `hit-points-preview.spec.ts`, `spell-details-format.spec.ts`, `ability-scores.spec.ts`, `hit-points-rolls.spec.ts`, `spell-details.spec.ts` and `character-editor.spec.ts`; Playwright `@MR-004` in `e2e/tests/character-editor.spec.ts` (`o editor da ficha mostra só as magias do nível e deixa tirar a subclasse`, "rolar 4d6 dá seis resultados…", "o conjunto padrão pode ser colocado e trocado…", "os pontos de vida rolados preenchem todos os níveis que faltam", "o \"?\" ao lado da magia abre a descrição…"); `@a11y` in `a11y.spec.ts` ("as rolagens e a descrição da magia passam no axe…").

#### Related
- Rules as data, with formulas in the Expr language, calculate the sheet. See [ADR-0008](../adr/0008-regras-dnd-conteudo-como-dados-motor-puro.md).

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

#### In the app
- A session starts with `PlayService.StartGameSession`, which locks the sheets in the same transaction. The player gets `failed_precondition` with the reason `SHEET_LOCKED` or `STORY_LOCKED`. The master unlocks the story with `SetStoryEditing`; starting a session turns story editing off again.
- The sheet screen is read-only for a locked sheet, with the master's buttons.
- The edit address of a locked sheet (`/campaigns/:id/characters/:characterId/edit`, typed or bookmarked) also shows the lock before any form: the editor reads `Character.can_edit` on open and, without permission, shows "Ficha travada" (or "Personagem morto") with the reason and "Voltar para a ficha", instead of letting the player fill everything in and find out on save.
- Tests, one per criterion: `TestMR006_AfterTheFirstSessionOnlyTheMasterEditsTheSheet`, `TestMR006_BeforeAnySessionThePlayerEditsTheSheet`, `TestMR006_CharacterCreatedAfterTheFirstSessionLocksAtTheNextOne` (and, in `play` with a real session, `TestCharacterCreatedLaterLocksAtTheNextSession`), `TestMR006_AfterTheLockTheStoryNeedsTheMastersPermission` (and, in `play`, `TestStartingASessionTurnsStoryEditingOff`); Playwright `e2e/tests/sheet-lock.spec.ts`, which also covers the locked edit address.

### MR-008: Points of interest

**As a** master, **I want** to create points of interest that open a battle, a submap or an RP scene.

- Priority: MVP
- Rules: —
- Modules: maps

#### Acceptance criteria
- **Given** a campaign map, **when** the master creates a point of type battle, submap or RP scene, **then** the point appears on the map **and** opening the point leads to the encounter, the submap or the scene.

#### In the app
- `MapService` creates, changes, moves and deletes maps and points of the three types (see [Architecture](../architecture.md#maps-module-maps-points-and-tokens)). A map is born from a gallery image and is born hidden; so is a point. A submap point leads to another map of the same campaign, never to its own map. Test: `TestMR008_MasterCreatesPointsOfEachKind`.
- Screens: "Novo mapa" (a name and a gallery image), the campaign's "Mapas" panel, and the master's editor on desktop. In the editor, choosing the type and clicking the map places the point (hidden); dragging or the arrow keys move it; the point panel saves everything together with "Salvar ponto"; "Adicionar token". On a phone the master only pans, zooms and reveals through the lists. See [Design](../design.md#maps-and-the-shown-image).
- Rename and delete a map: in the map header, "Renomear" beside the name and "Apagar mapa" at the right end. Deleting asks right there what goes with it (the points and tokens; the image stays in the gallery) and warns when it is the open session's current map. When the server refuses (a combat runs on the map, or one of its treasures is found or already turned into XP), the box says which, not that the server is unreachable. After deleting, the app returns to the campaign.
- Opening a point: a submap point shows its name and description with "Abrir <mapa>" (first the point's card, then the submap). Battle and scene points open the combat encounter and the RP scene (see [MR-013](#mr-013-turn-order) and [MR-015](#mr-015-rp-scene-actions)); outside a session, opening them shows the point's card.
- Tests: `maps.spec.ts` (`@MR-008`: the master creates the map and the three points on screen, the player opens the submap from the point's card, the master renames the map and deletes it after confirming) and `a11y.spec.ts`.

### MR-009: Map without spoilers

**As a** player, **I want** to see on the map only the points my group already knows, **so that** I get no spoilers.

- Priority: MVP
- Rules: RN-10
- Modules: maps

#### Acceptance criteria
- **Given** a map with one revealed point and one hidden point, **when** the player opens the map, **then** only the revealed one appears **and** the server response does not contain the hidden one.

#### In the app
- `MapService` decides on the server what each person sees (RN-10). A player receives only revealed points, only visible tokens, and only revealed maps or the session's current map. A hidden map is `not_found` for the player, the same as a map that does not exist.
- The player's screen (`/campaigns/<id>/maps/<map>`) draws only what arrived, with the submap trail, "Mapas revelados" and "Pontos deste mapa". A hidden map shows "Mapa não encontrado".
- Tests: `TestMR009_PlayersNeverReceiveHiddenPoints` reads the player's response as the JSON the app receives and checks that the hidden point's ID, name and description are absent, and that a change only to hidden things reaches only the master through the stream. `TestRN10_PlayersCannotOpenHiddenMaps` covers maps and submaps. `maps.spec.ts` (`@MR-009`) opens the map as the player, reads the `GetMap` response the page itself received and checks that it carries neither the ID nor the name of the hidden point.

### MR-011: Start the session

**As a** master, **I want** to start the session, have the players receive a notification in the app and get a session link to send them, **so that** everyone joins together.

- Priority: MVP
- Rules: RN-06, RN-07
- Modules: play, campaigns, characters

#### Acceptance criteria
- **Given** a campaign with three players, **when** the master starts the session, **then** whoever has the app open sees the notification **and** the master can copy the session link.
- **Given** the session link, **when** someone who is not a member opens it, **then** they see "peça um convite ao mestre" **and** do not get in.
- **Given** it is the campaign's first session, **when** the master starts the session, **then** the players' sheets lock.

#### In the app
- `PlayService.StartGameSession` opens the session and, in the same transaction, locks the sheets of the players that are still drafts. Test: `TestRN01_StartingTheFirstSessionLocksPlayerSheetsOnly` (in `play`).
- Notification: `PlayService.ListOpenGameSessions` tells the app, every 30 seconds while the tab is visible, which sessions are open in the person's campaigns (`TestListOpenGameSessions`). The notice "A sessão 4 de Mirathel começou." sits under the app bar with "Entrar na sessão" (for people who play in the campaign, outside the campaign page and the session pages; closing it lasts only for the tab). On the campaign page the "Sessão" card says it instead, and a player's card follows the same poll. RN-06: the on-screen notice is enough; there is no browser push notification.
- Other entry points: the "Ao vivo" link in the bar, the "Sessão ao vivo" tag in "Minhas campanhas", and the campaign's "Sessão" panel with "Entrar na sessão" and "Copiar link da sessão" (copying the link is screen-only).
- Session page `/campaigns/<id>/session`: reads `GetLiveSession` and opens the `WatchGameSession` stream. Both answer `not_found` to a non-member and to a pending member (the screen says "Peça um convite ao mestre", without the campaign name) and `failed_precondition` with `NO_OPEN_SESSION` when no session is open (`TestWatchGameSessionRefuses`, `TestAuthorizationMatrix`). A member with no open session sees "Nenhuma sessão em andamento", and the page opens the session by itself when the master starts it.
- Tests: `live-session.spec.ts` (`@MR-011`), with the player's app open while the master starts the session on screen.

#### Related
- RN-07: invites default to 1 use and 7 days; the master picks 1 to 20 uses and 5 minutes to 30 days, and can revoke (see [MR-002](#mr-002-generate-an-invite)).

### MR-012: Follow the session

**As a** player, **I want** to follow my sheet and the current map during the session.

- Priority: MVP
- Rules: RN-02, RN-10, RN-11, RN-20
- Modules: play, maps

#### Acceptance criteria
- **Given** an active session, **when** the master moves a token or the system applies damage to the character, **then** the player's phone shows the change without reloading the page **and** the master's notes never appear.

#### In the app
- Sheet half, server: the master corrects HP, temporary HP, spell slots and hit dice during the session (`AdjustCharacterVitals`, RN-02). The change reaches the master and the character's owner at once through the `WatchGameSession` stream, never another player. Nothing in the live session carries the master's notes (RN-11). Tests: `TestRN02_MasterAdjustsVitalsDuringSession`, `TestPlayersSeeOnlyTheirOwnVitals`, `TestLiveStreamIsNotBuffered`, `TestRN11_LiveSessionNeverCarriesMasterNotes`.
- Sheet half, screens: the player sees HP, temporary HP, AC, hit dice and spell slots of their own character, and the number changes on screen when the master corrects it, with no reload. The master sees the group and corrects in "Ajustar" (a bottom sheet on phone, a dialog on desktop), which never goes past the sheet's maximum and, after the session ends, says "A sessão acabou". Without a connection the page says "Reconectando…" with the time of the last update, and the numbers stay on screen. Tests: `live-session.spec.ts` (`@MR-012`, `@RN-02`) and `a11y.spec.ts`.
- Map half, server: the master chooses the session's current map (`PlayService.SetCurrentMap`, which also reveals it) and moves tokens (`MapService.PlaceMapToken`). The stream carries `current_map_changed`, `token_moved` and `map_changed`, and a player only hears about what they see (RN-10). Tests: `TestMR012_TokenMovesReachPlayersLive` (a visible token reaches the player; an NPC's hidden token reaches only the master) and `TestSetCurrentMap`.
- Map half, screens: the current map replaces the notice "O mestre ainda não escolheu um mapa." for the player (a preview that opens the whole map) and for the master (the "Mapa atual" selector, draggable tokens, "Pontos do mapa" and "Tokens no mapa" with "Revelar aos jogadores" and "Esconder"; a character's creature has its own row, always visible, with no button, and dragging a creature's token moves the creature, not its owner). The page reads the map again on `map_changed` (if the server answers `not_found`, the player lost sight of the map and goes back to the notice), swaps the map on `current_map_changed` and moves the token on `token_moved` without reading anything. Test: `maps.spec.ts` (`@MR-012`: the master picks the map and moves a token by keyboard; the player's open page shows the map and the new position without reloading).
- In combat, the player sees enemies by a word (Ileso, Ferido, Muito ferido, Derrotado), not by HP (RN-20).
- "The system applies damage": an attack that hits opens a pending damage. On an NPC the damage is applied at once (temporary HP first; at 0 HP it is defeated and leaves the order). On a player's character it waits for the master, who applies it ("Aplicar 5 de dano") or discards it. On applying, the change reaches the owner and the master live (`vitals_changed`). Spell damage follows the same path as attack damage. A heal is applied at once (and raises someone at 0 HP). The master can apply a different amount than the rolled one (`TestMasterAppliesADifferentAmount`). Damage to someone at 0 HP counts one death-save failure (RN-03). The master also edits an NPC's HP ("Dano/Cura"), undoes the last action and reads the combat log, which the player receives only with what they see (RN-20). Tests: `TestMR012_DamageToAnNPCIsAppliedAndDefeatsIt`, `TestRN02_DamageToAPlayerWaitsForTheMaster`, `TestCombatUndoRestoresTheLastAction`, `TestTimelineRound1And2Log`, `TestRN20_PlayersNeverReceiveCAOrHiddenLogEntries`, `TestCombatLogChangedPerAudience`. See [Architecture](../architecture.md#combat).

#### Related
- RN-02: yes, the master can correct HP and spell slots by hand during the session; the master has the final word (see [Business rules](rules.md)).

### MR-013: Turn order

**As a** player, **I want** to see the turn order, where each one is and how far I can move, **so that** I can plan my action.

- Priority: MVP
- Rules: RN-19, RN-20, RN-21
- Modules: play, rules

#### Acceptance criteria
- **Given** a combat with initiative set, **when** the player opens the combat screen, **then** they see the turn order, where each visible combatant is and how much they can still move this turn.
- **Given** a combat with a grid, **when** I open the turn economy or the map, **then** movement also appears in squares ("7,5 m · 5 quadrados").
- **Given** combatants side by side in the order with the same initiative total, players included, **when** the turn order is shown, **then** they form a joint turn, in a box with the words "Turno conjunto" and the total. The master sees all groups; a player sees only a group that has a player character (a group of only NPCs is the master's, RN-20).
- **Given** a joint turn, **when** the turn reaches the group, **then** every member's turn starts at once, each with their own action, bonus action, reaction, movement and death save; each member's player ends their own part, the master ends anyone's, and the turn passes only when the last one ends (there is no "end the group's turn" and no reopening a part).
- **Given** the player ends their own part, **when** they tap "Encerrar a minha parte", **then** the app asks first ("Encerrar a sua parte?", with what is still left and "Voltar" first), because a part does not reopen.

#### In the app
- Server: `CombatService` creates the combat on the map with a grid (the map has `grid_columns`, `MapService.SetMapGrid`), puts in the group and the NPC copies, rolls each NPC's initiative, receives the player's (in the app or the physical d20, RN-18), lets the master order ties, starts the combat, passes the turn (the round goes up after the last; movement, action and reaction come back at the start of each one's turn) and lets people move on the grid within the remaining movement. The player never receives a hidden combatant or an NPC's number (RN-20), and a hidden combatant's turn shows as "Vez do mestre". The map's battle point can point to the combat's map. See [Architecture](../architecture.md#combat).
- Master screens, on the session page: "Combate" with "Iniciar combate" (name, the map with the grid, the group and how each player rolls, the NPCs with how many copies and "Escondido no início"). A map without a grid sends the master to "Grade do mapa" (`/campaigns/:id/maps/:mapId/grid`, 5 to 60 squares across, rows from the image proportion). Initiative: totals with the sum `1d20 (15) + 4 = 19`, a tie shown once with arrows inside the group, "Esperando …" with "Digitar pelo jogador", "Começar o combate" blocked with the reason, "Posições iniciais". Once running: the bar ("Vez do …", "Rodada 2", "Próximo turno", "Encerrar combate", which asks in the bar itself), the map with the grid and all tokens, and the order with HP, "Dano/Cura" for players, reveal and hide, remove and "Adicionar combatente".
- Player screens: initiative via "Rolar no app" or "Digitar o resultado" (per RN-18), the total at 92 px and "Esperando o mestre começar o combate". With the combat running: "Vez do …" (or "Vez do mestre" for a hidden one), "Você é o próximo", the order as cards with only the visible ones and the state words, "Sua vez" with movement, "Mover" and "Encerrar turno", and the "Mover" page (range; "Mover 3 m. 2 quadrados para a direita e 1 quadrado para baixo. Depois restam 4,5 m."; "Longe demais: faltam 1,5 m"; "Ocupado"; on desktop, dragging the token within range). At the end both see "Combate encerrado". The screen follows the live session: `encounter_changed` reads the combat again, `turn_changed` and `combatant_moved` are applied in place.
- Distances in squares: every distance comes from one place, `core/units.ts` (1 square = 1.5 m = 5 ft). The "Movimento" box says "7,5 m de 7,5 m" and "5 quadrados livres"; the turn bar, "Mover 7,5 m · 5 quadrados" (a two-column grid so the number does not wrap); the master's NPC card, "Deslocamento 9 m · 6 quadrados"; the sheet, "7,5 m · 5 quadrados (25 pés)" on one line; the "Mover" page and the action groups use the same functions.
- Joint turn: the group is calculated from the order and the totals when the turn starts (`nextTurnGroup`, `combat_turn.go`) and stored in `combatants.turn_state` (`idle`, `acting`, `ended`, migration `00089`), so reordering a tie, bringing reinforcements or hiding a member does not change a turn that already began.
  - `EndTurn` ends one member's part (`expected_combatant_id` is the member; `aborted` guarantees a tap never ends two parts or one twice) and the turn passes when nobody else acts. Each ended part is a `turn_part_ended` event and a log line ("Brisa encerrou a parte dela"; a hidden member's does not reach the player).
  - Every "is it their turn" check (`MoveCombatant`, attacks, actions, spells, death saves, reactions, `GetTurnOptions`) means "in the turn and the part has not ended".
  - The server sends the player `Encounter.turn_group_ids` (only members they see), `Combatant.turn_part_ended` (only for a group with a player character), `npc_only_groups` (only to name "os Goblins") and, for players of the same group, each other's economy.
  - Screens: the "Turno conjunto" box in the master's and the player's order, the master's card with one block per member and "Encerrar a parte da Brisa", the pill "Turno conjunto com Brisa", "O que a Brisa ainda tem", "Encerrar a minha parte" with the question in place, "Você encerrou a sua parte", "Vez de Brisa e Toren" and "Vez dos Goblins".
- Server tests: `TestMR013_TurnOrderAndMovementLeft`, `TestRN19_EachNPCRollsItsOwnInitiative`, `TestRN20_PlayersNeverReceiveHiddenCombatantsOrNPCNumbers`, `TestRN21_PlayerMovementIsLimitedTheMasterIsNot`, `TestEndTurnIsIdempotent`, `TestStartEncounterNeedsAGrid`, `TestMR013_CombatAuthorizationMatrix`, `TestMR013_CombatEventsPerAudience`, `TestMR013_CombatantsStartOnTheirTokensAndEndWhereTheyStand`. Joint turn: `TestMR013_PlayersWithTheSameInitiativeShareATurn`, `TestMR013_TheTurnPassesWhenTheLastMemberEnds`, `TestMR013_EachMemberHasItsOwnEconomy`, `TestMR013_TheMasterEndsAPartForAnAbsentPlayer`, `TestMR013_APlayerCannotEndAnotherMembersPart`, `TestMR013_ADoubleTapEndsOnePartOnly`, `TestMR013_AnUndoStaysClosedAcrossAPartEnding`, `TestMR013_AMemberWhoLeavesDoesNotBlockTheTurn`, `TestMR013_AReinforcementWithTheSameTotalActsFromTheNextTurn`, `TestMR013_EachMemberOwesItsOwnDeathSave`, `TestMR013_OrderingATieKeepsTheGroup`, `TestMR013_EndTurnPartAuthorization`, `TestRN20_NPCOnlyGroupsStayTheMasters`, `TestRN20_AHiddenMemberIsNeverNamed`.
- Screen tests: `combat.spec.ts` (`@MR-013`: the grid, the start with the group and three hidden goblins, initiative in the app, the tie, "Próximo turno", "Vez do mestre", moving within range and the refusal beyond it, the end, "a distância sai em metros e em quadrados"), `joint-turns.spec.ts` (`@MR-013`, `@RN-20`), `a11y.spec.ts` ("o combate passa no axe…", `scanJointTurnScreens`); Vitest `combat-grid.spec.ts`, `combat-view.spec.ts`, `combat-state.spec.ts`, `combat-errors.spec.ts`, `initiative-setup.spec.ts`, `turn-panel.spec.ts`, `order-list.spec.ts`, `units.spec.ts` (5, 25, 30 and 35 ft, 0 and odd feet), `joint-turn.spec.ts`. The e2e suite has only two signed-in people, so its joint group is Pensantus and Brisa as an allied NPC; the players-only group is covered by the server and Vitest tests.

#### Related
- Each NPC rolls its own initiative (RN-19); the player sees enemy state by a word, never HP or AC (RN-20); each grid square is 1.5 m, diagonals included, and the app does not let the player go past the turn's movement (RN-21). See [Business rules](rules.md).
- The available speed comes from the rules engine (rules as data). See [ADR-0008](../adr/0008-regras-dnd-conteudo-como-dados-motor-puro.md).
- The turn group mixes NPCs and players, and players can also attack together (described above).

### MR-014: Your turn

**As a** player, on my turn, **I want** to see my actions, bonus actions and possible attacks.

- Priority: MVP
- Rules: RN-02, RN-03, RN-18, RN-20, RN-22
- Modules: play, rules

#### Acceptance criteria
- **Given** a combat, **when** Pensantus's turn comes, **then** the player sees the action, bonus action, reaction and movement available **and** spells without a spell slot appear disabled.
- **Given** Pensantus casts Magic Missile (Mísseis Mágicos) with a 1st-level slot, **when** the action is confirmed, **then** the system marks the slot as used.
- **Given** a character with spells, **when** they open the spell list on their turn, **then** the ones they can cast now come first, then the others, each group by spell level.
- **Given** a spell in the list during the session, **when** the player taps the "?", **then** they see the full description, as in the editor.
- **Given** a spell that reads hit points (Sono, Leque Cromático, Palavra de Poder Atordoar e Matar, Estabilizar, Cura Completa), **when** it is cast, **then** the server resolves it with the targets' real HP **and** the player keeps seeing only "Ileso", "Ferido" or "Muito ferido" (RN-20): the caster sees their own roll and who was affected, never a target's HP.

#### In the app
- Rules engine: `rules/combat.Options` calculates the economy (action, bonus action, reaction, movement with Dash), attacks, spells with their possible levels, and the standard and feature actions, and marks each disabled option with a reason code (`NO_SLOT`, `ACTION_USED`, `NO_USES`, `NOT_YOUR_TURN`…). See [Architecture](../architecture.md#combat-engine-and-spell-details). Each spell has structured details (`ContentService.GetSpellDetails`).
- `CombatService.GetTurnOptions` returns the economy (movement in feet), attacks with the targets in sight and the distance (RN-21), spells and standard actions, and, outside the turn, everything disabled with the reason `NOT_YOUR_TURN`. Each target of a spell comes with its distance and darts per level (`spell_targets`).
- Attack in two steps: `RollAttack` (spends the action; the d20 goes to the server, which compares with AC and returns only Acertou, Errou or Crítico) and `RollDamage`. `TakeAction` does the standard actions that only spend the economy (Dash doubles the movement left).
- `CastSpell` casts and **spends the slot and the action at once** (criterion 2). It resolves according to what the spell is (spell attack; saving throw rolled by the server with one damage roll for the whole cast; the darts of Magic Missile; healing; or just a log entry) and sets concentration. A retry does not spend again; `NO_SLOT` carries the minimum level.
- Reactions: Shield (Escudo Arcano) becomes a prompt when a blow hits the character (`reaction_prompts`, `UseReaction`, `DeclineReaction`); the opportunity attack is `RollAttack` with `as_reaction`; Extra Attack allows more than one attack per action; feature actions (Second Wind, Action Surge…) spend the resource's use.
- Also on the server: "Caído", death saves and the master's confirmation (RN-03), conditions and the concentration reminder (RN-22).
- Spell order: the spell list of `GetTurnOptions` already comes in the order of criterion 3 (`rules/combat.Options`: the ones that can be cast now first, then the others; in each group cantrips first, then by spell level, then by Portuguese name). Shield on the character's own turn can never be cast and sits among the others with the reason `REACTION_ONLY_WHEN_HIT`, shown as "Só fora da sua vez".
- Spells that read HP (Sono, Leque Cromático, Palavra de Poder Atordoar e Matar, Estabilizar, Cura Completa) are handwritten content (`effects/spells.json`, four closed types: `hp_pool`, `hp_threshold`, `zero_hp_target`, `flat_heal`). `CastSpell` resolves them with real HP, the NPC's in the combat and the player character's from the live sheet, with the total rolled in the app or typed (`pool_sum`, RN-18) and the condition through the same code as conditions; "Desfazer última ação" gives everything back. The caster sees their own roll and who was affected; the master sees the total, each one's HP and the order; the other players see only who was affected (RN-20). Toll the Dead (Dobre pelos Mortos) is not in SRD 5.1 and does not enter: the repository is public and holds only SRD.
- "Your turn" screen: "O que você pode fazer" groups what the server calculates into Ação (Ataques, Magias, Ações padrão), Ação bônus, Reação and Movimento, each group with a state word ("Disponível", "Usada") and each disabled option with the reason in words ("Ação já usada", "Sem espaço de 2º nível ou maior"), without taking the option out of place or out of keyboard reach. Standard actions are a two-column grid with one reason line. From 1024 px the turn is a compact strip with "Encerrar turno" on the right and the economy boxes go to the panel; from 1280 px the order is a left column.
- Attack sheet: "Atacar com Raio de Fogo" opens a bottom sheet on phone (a dialog on desktop; it scrolls inside, with buttons stuck at the bottom) in three steps, Alvo, Rolar and Dano. The target is a radio group with state and distance ("Longe demais: alcance de 36 m", disabled). The d20 and the damage roll in the app or are typed ("Digite o resultado do dado", 1 to 20; damage N to N × faces) per RN-18. The result shows `1d20 (13) + 6 = 19`, "Acertou", "Crítico" or "Errou" and the defeated target, never AC.
- "Encerrar turno" is outlined while the action or the bonus action is free and becomes the filled button when both are spent; with the action free it asks in place of the button.
- Cast sheet: "Conjurar" opens a phone sheet (a desktop dialog) with the spell slot as radios ("1 livre de 4"; a level without a slot is dashed, "Sem espaço livre"), the targets from `spell_targets` (distance, "Longe demais", the target limit) and Magic Missile's darts in 44 px steps with the counter "3 de 3 dardos distribuídos". The warning "É o seu último espaço de 1º nível" (and the Shield one, when it is the last the character would have) comes before the filled button "Conjurar X". An attack spell swaps the button for the d20 in two ways (RN-18), one per target. The result lists each target (d20, "Falhou" / "Resistiu: metade", "Dardo 1: 1d4 (3) + 1 = 4"), the damage still to roll right below (one roll for the whole cast when it is an area, one per target for darts), "Espaços de 1º nível: 0 livres de 4", "Escudo Arcano indisponível", concentration and "Sua ação foi usada". A spell with no known effect says "A magia foi conjurada: o mestre resolve o efeito." A saving-throw cantrip (Sacred Flame, Chama Sagrada) is cast, not attacked.
- Class abilities have "Usar" (Second Wind, Retomar o Fôlego, rolls the d10 in a sheet and shows the healing; Action Surge says "Você tem outra ação"; `NO_USES` becomes "Sem usos: volta num descanso curto"). Extra Attack shows "1 ataque restante" in the Action with the remaining attacks enabled ("Ataques desta ação já usados" after).
- The Shield prompt: a blow that Shield can stop opens the `alertdialog` "Você foi atingido" on its own, with who attacked and with what when the player sees the attacker (focus on "Não usar", no way out without an answer); the master's card swaps live when the player answers. The opportunity attack is the text action under "Sua reação" (a sheet with melee attacks only, spends the reaction).
- Someone at 0 HP sees "Brisa está caída", the marks and "Rolar teste contra a morte" (RN-03); the master confirms the death in place, at the top of the card ("Confirmar a morte" / "Ainda não"). Conditions (15 from the SRD; "Derrubado" is `prone`) and concentration are in "Condições…" in the order's ⋮, as tags under the name, in the player's strip and as a dot on the token; the player ends their own concentration. The master applies **another value** ("Aplicar outro valor") and, when the target concentrates, reads "Teste de Constituição, CD 10". The log has the sentences for spell, reaction, death save, confirmed death and conditions, each as the server sends it to each audience.
- Master's turn: the card "Ações do Capitão Goblin" (HP, AC, speed, the attack, the "Alvo" field, "Rolar ataque" in the app or typed, "Acertou contra CA 18 do Toren", the damage and "Aplicar 5 de dano" / "Não aplicar", which asks "Descartar o dano de 5?"). On phone and tablet the card is the whole turn and ends with "Próximo turno", with "Encerrar combate" and "Desfazer última ação" at the bottom of the page. While there is unapplied damage, "Próximo turno" asks "Há dano sem aplicar. Passar o turno mesmo assim?". In the order, "CA n" (master only) and "Dano/Cura" also on NPCs (`AdjustCombatantHitPoints`). Two master-only fields: `Combatant.armor_class` and `AttackRoll.target_armor_class`.
- Combat log: `ListCombatLog` (read again at each `combat_log_changed` and at each reconnection) shows the rounds, newest first, with a spell icon on spell attacks. The master undoes the last action by name ("Desfazer o ataque do Capitão Goblin ao Toren (5 de dano)?"). The header line says "Em andamento desde 20:05" for everyone.
- Spell list screen: spells of all economies form one list, in the order the server sends (not reordered), with each level's slots above ("1º nível ○ ✕ ✕ ✕ 1 livre de 4") and the reason repeated only as "Sem espaço". Each row has the 44 px "?", which never fades; the cast sheet has it in its header. It opens the spell details (time, range, components, duration and the SRD text in English) in a sheet on phone and a dialog from tablet up, over the cast sheet without losing the choice. The level is a tag on its own line ("Reação" beside it for Shield); Shield on the character's turn has no button and says "Só fora da sua vez". `SpellDetails` lives in `shared/spell-details/`.
- HP-reading spells on screen: in the cast sheet, Sono asks for the dice total (in the app or typed) and who is in the area, by name only. The player's result says "O Goblin 1 adormeceu. O Capitão Goblin não foi afetado.", their roll (`5d8 (2, 4, 1, 5, 3) = 15`) and no HP. In the log the master reads a card with the total, each creature from lowest to highest HP, the leftover, the word with the icon and "Mudar as condições"; the player reads only the sentence.
- Rules-engine tests (`rules/combat`): `TestOptionsPensantus` (with 1 free 1st-level slot of 4 and 0 of 2 free of 2, Web and Misty Step are `NO_SLOT` with minimum 2 and Magic Missile accepts only the 1st; Fire Bolt +6 1d10), `TestOptionsToren` (Battleaxe +5 1d8+3; Second Wind is a bonus action, 1 use per short rest), `TestOptionsWarlockPactMagic`, `TestOptionsMovement`, `TestSpendSlot`, `TestSpendResource`, `TestMR014_SpellsSortByAvailabilityThenCircle`, `TestResolvePool`, `TestResolveThreshold`, `TestResolveZeroHP`, `TestResolveFlatHeal`; in `rules`: `TestResourcesAndActions`, `TestAttackDice`, `TestGetSpellDetails`, `TestSpellDetailsExamples`.
- Server tests: `TestMR014_TurnOptionsFollowTheEconomy`, `TestRN18_PhysicalRollsAreTypedSums`, `TestMR014_CastingSpendsTheSlot`, `TestTimelineRound3MagicMissile`, `TestSaveSpellRollsOnceForTheCast`, `TestHealingSpellRevivesAndResetsDeathSaves`, `TestShieldTurnsAHitIntoAMiss`, `TestOpportunityAttackSpendsTheReaction`, `TestExtraAttackAllowsTwoAttacks`, `TestSecondWindAndActionSurge`, `TestRN03_DeathSavesAndTheMasterConfirms`, `TestRN22_ConditionsAndTheConcentrationReminder`, `TestMR014_SleepUsesTheRealHitPoints` (the total 15 puts the 7 HP Goblin to sleep and leaves the 27 HP Captain awake, and the player never receives an HP), `TestMR014_SleepSkipsTheUnconsciousAndTheOneAtZero`, `TestMR014_ColorSprayBlindsByThePool`, `TestMR014_PowerWordStunAndKillOnNPCs`, `TestMR014_PowerWordKillOnAPlayerCharacterWaitsForTheMaster`, `TestMR014_SpareTheDyingWorksOnlyAtZero`, `TestMR014_CompleteHealHealsAndEndsBlindnessAndDeafness`, `TestRN18_PoolSpellsFollowTheDiceMode`.
- Screen tests: `combat.spec.ts` (`@MR-012`, `@MR-014`, `@RN-02`, `@RN-03`, `@RN-22`, `@RN-20`: attack with physical dice, attack in the app, end of turn with Dash, the master's attack with apply, discard and undo, "Dano/Cura", the log without the hidden goblin, Magic Missile with the darts and the last slot, the save on two goblins, healing, death saves and confirmation, Shield on both screens, conditions and concentration, another value with the reminder, the fighter with Extra Attack, Second Wind and Action Surge, the opportunity attack, "as magias vêm na ordem do servidor", "Sono em dois goblins e no Capitão"); `a11y.spec.ts` ("agir no combate", "conjurar e cair"); Vitest `core/combat` specs (`combat-dice`, `combat-options`, `combat-log`, `combat-grid` with `tight`, `attack-flow`, `turn-options-state`, `cast-flow`, `death-saves`, `combat-errors`, `cast-result`, `hp-effects`, `hp-spells-log`), component specs (`roll-picker`, `end-turn`, `next-turn`, `action-row`, `order-column`, `action-groups`, `open-spell-details`, `pool-card`), `session-time.spec.ts`, and `TestRN20_PlayersNeverReceiveCAOrHiddenLogEntries`.

#### Related
- RN-02: the master can correct HP and spell slots by hand (see [Business rules](rules.md)).
- Dice (RN-18): the master chooses how the campaign rolls (each player chooses, everyone in the app, or everyone with their own dice) and each player keeps their own preference, on the campaign page (`TestRN18_DiceSettings`, `TestEffectiveDiceMode`, `dice.spec.ts` `@RN-18`). With a physical die the player types the sum of the dice and the app adds the modifier (`dice.Physical`, `TestPhysical`). Combat rolls follow this rule: the attack d20 and damage, the spell-attack d20, healing, the Second Wind d10 and the death save.
- The player sees whether they hit or missed and the damage, not AC or the NPC's roll (RN-20). Conditions and concentration are only marked and reminded; the master decides the effects (RN-22). On the third death-save failure, the character dies only when the master confirms (RN-03).
- Which actions, bonus actions, reactions and resources the system knows comes from the rules engine (rules as data). See [ADR-0008](../adr/0008-regras-dnd-conteudo-como-dados-motor-puro.md).

### MR-015: RP scene actions

**As a** player, **I want** to see, in a simple list, the actions the master chose for the scene, **so that** I know what I can roll and use outside combat.

- Priority: MVP
- Rules: RN-10, RN-18, RN-20
- Modules: maps, play, rules

#### Acceptance criteria
- **Given** an RP scene with the actions the master chose for it, **when** the player opens the scene, **then** they see the list of actions the master chose, each roll with their own character's bonus already calculated (for example Investigação) **and** abilities that only work in combat do not appear.
- **Given** a combat (MR-013, MR-014), **when** the player sees the possible actions, **then** the system decides that list, by the D&D rules — never the master. The RP scene is the only place where the master chooses the list.
- **Given** a scene, **when** the master turns on "Mostrar a CD aos jogadores" (per scene; off by default), **then** the player sees the DC of the actions that have one and whether their own rolls passed; when off, they see only the total (RN-20). An action without a DC shows nothing extra, and the master always sees the DC.
- **Given** a scene action, **when** the master sets the attempts per player (1 by default, another number or no limit) and can give a player one more attempt, **then** the player rolls up to their limit and sees how many attempts are left ("Restam 2 de 3 tentativas", "Sem mais tentativas"). Closing and reopening the scene resets the counts; lowering the limit below what someone already spent leaves them without attempts, without erasing anything.

#### In the app
- The master picks the actions on the map's scene point, one at a time (`AddSceneAction`, `UpdateSceneAction`, `MoveSceneAction`, `RemoveSceneAction`): skill, ability check or saving throw, with an optional name (up to 60 characters) and an optional DC (1 to 30), at most 20, and nothing combat-only (the key is checked against the rules catalogue).
- The master opens the scene in the session (`OpenScene`: any scene point opens, even without actions, and it can be hidden, in which case it stays hidden on the map) and closes it without asking (`CloseScene`); everyone receives `scene_changed`.
- The player reads the scene (`GetOpenScene`) with the point's name and description, the actions with their own character's bonus (from the sheet, `rules.SceneOptions`) and the passive Perception, Investigation and Insight. The DC comes only when the master turns on "Mostrar a CD aos jogadores".
- Rolling (`RollSceneCheck`): the app's d20 or the typed one (RN-18) plus the bonus, up to the action's attempts limit. The master receives `scene_check_rolled` and sees every roll, with the total and "passou" when there is a DC (the scene log). The player sees only their own, with "passou" only when the scene shows the DC.
- Scene options on the server: the scene point has `show_dc` (`UpdateMapPoint` / `CreateMapPoint`) and each action has `max_attempts` (1 to 5, 0 = no limit; `AddSceneAction` / `UpdateSceneAction`). `OpenSceneInfo.show_dc`, `SceneActionView.max_attempts` / `attempts_left` and `SceneRoll.attempts_left` (master only) feed the screen. `GrantSceneAttempt` is "Dar mais uma tentativa".
- Editor: the point panel of a scene point has "Ações da cena" (the list with ↑ ↓ and remove, the inline form with the DC error, the empty state and the 20-action limit). At the top of "Ações da cena" is the switch "Mostrar a CD aos jogadores" (per scene, off at first, saved at once; when on, it shows how the player sees the DC). Each action has "Tentativas por jogador" (a 44 px list: 1 to 5 or "Sem limite", saved at once).
- In the session, the master has "Cena de RP" with "Abrir cena" (the selector, including the hidden scene and the one without actions), the open scene at the top of the map column with the actions, live rolls with Passou / Não passou, and "Trocar cena" / "Fechar cena". The scene can also be opened from the point, in "Pontos do mapa" ("Abrir cena" or "Trocar para esta cena"). The master's open scene says, atop "Ações", "Os jogadores veem a CD" or "Só você vê a CD", each action's limit and, on each roll, "Tentativa 1 de 3" and "Dar mais uma tentativa". That button appears only on the character's last roll of that action, asks in place with "Voltar" first, and, with the DC shown, is offered only if the roll failed — or when the action has no DC.
- The player has the block "Cena" with their own bonus and "Rolar", the rolling sheet (in the app or typing the physical die) and the line "Rolada". They read "CD 12", "Passou · CD 12" / "Não passou · CD 10" (also in the result sheet) and the attempts left: "1 tentativa", "Restam 2 de 3 tentativas", "Restam N tentativas" (above the limit), "Sem mais tentativas", or nothing if the action has no limit. The live region says "O mestre deu mais uma tentativa em …". "Rolar" sits in the same place on every row (the last row of the card on a phone). An action the player exhausted shows the last result and "Rolada às 21:12" (without the DC tag).
- **With a combat on screen, the scene blocks are not drawn** (neither for the master nor for the player): the scene stays open on the server and returns to the screen when the combat ends.
- Server tests: `TestMR015_PlayerSeesTheMastersActionsWithTheirBonus`, `TestMR015_NothingCombatOnlyInAScene`, `TestRN20_APlayerNeverGetsADCOrAnotherPlayersRoll`, `TestMR015_OneRollPerActionWhileTheSceneIsOpen` (the arithmetic with the default limit of 1), `TestRN18_SceneRollsFollowTheDiceMode`, `TestMR015_AHiddenPointCanBeOpenedAndStaysHidden`, `TestMR015_OpeningAndClosingAScene`, `TestMR015_SceneActionRules`, `TestMR015_RollingNeedsALivingCharacter`, `TestSceneAuthorizationMatrix`, `TestMR015_ShowTheDCToPlayers`, `TestMR015_AttemptsPerAction`, `TestMR015_OneMoreAttempt`. See [Architecture](../architecture.md#rp-scenes).
- Screen tests (`e2e/tests/scenes.spec.ts`, `@MR-015`, `@RN-20`): "o mestre escolhe as ações no ponto de cena, abre a cena na sessão, o jogador rola uma no app e uma com o dado físico e o mestre vê as duas com Passou e Não passou", "o jogador rola cada ação uma vez; o mestre fecha a cena sem pergunta e, ao abrir de novo, as ações voltam a poder ser roladas", "o mestre abre uma cena escondida: os jogadores veem a cena e o ponto continua escondido no mapa deles", "o mestre abre a cena pelo ponto na lista da sessão", "o mestre liga a CD da cena e dá 3 tentativas…", "com a CD desligada o jogador não vê CD nem Passou…"; `a11y.spec.ts` ("as cenas de RP passam no axe…"). Vitest covers the actions editor, the selector, the player rows, the rolling sheet, the master's roll line, the text of each `SceneBlocked` reason and the live-region messages (`scene-roll-line.spec.ts`, `scene-player.spec.ts`, `scene-actions.spec.ts`, `scene-view.spec.ts`).

#### Related
- In an RP scene the master chooses the possible actions and the player sees what they can do with their own bonus; in combat the system decides and shows the actions, by the D&D rules. See [Business rules](rules.md) and [ADR-0008](../adr/0008-regras-dnd-conteudo-como-dados-motor-puro.md).
- Scenes have skill checks, ability checks and saving throws (spells and abilities are for after the MVP); the master opens the scene, and a revealed point also opens it; the scene log exists.
- Which abilities appear in the list, and each one's bonus, come from the rules engine (rules as data). See [ADR-0008](../adr/0008-regras-dnd-conteudo-como-dados-motor-puro.md).

### MR-016: Award XP

**As a** master, **I want** to give XP to the group for defeated enemies, for gold or for milestones, according to the campaign, or whenever I want.

- Priority: MVP
- Rules: RN-09, RN-12
- Modules: progression

#### Acceptance criteria
- **Given** a campaign in the by-enemies mode and two defeated goblins (50 XP each), **when** the encounter ends, **then** the 100 XP are divided among the four characters of the group, 25 each.
- **Given** a campaign in the by-milestones mode, **when** the master records a milestone, **then** all characters of the group are marked as ready to level up **and** no XP is counted.
- **Given** the by-enemies or by-gold modes, **when** the master gives XP to the group on their own, **then** the XP goes into the sheets **and** the campaign history shows who gave it, when and why.
- **Given** a campaign by enemies, **when** the combat ends, **then** the master uses the NPC's challenge rating (ND) or types the XP (both ways count the same) **and** chooses who receives the split.
- **Given** a campaign by milestones with the milestones planned beforehand by the master, **when** the group reaches one and the master marks it, **then** the chosen characters "Podem subir de nível".
- **Given** the by-gold mode, **when** the master wants to give XP, **then** they type the gold pieces (PO); with the map's treasures, "Voltar à cidade" converts into XP what the group found ([MR-041](#mr-041-treasure-and-xp-by-gold)), and typing the PO remains valid.

#### In the app
- Module `progression` (see [Architecture](../architecture.md#progression-module-xp-and-milestones)). RPCs: `AwardXP` (by enemies, by gold or one-off), `MarkMilestone`, `UndoLastXPAward`, `ListXPAwards`, `GetCampaignExperience`. Each NPC's XP is on the sheet (`challenge_rating`, `xp_value`) and on the combatant (master only). "Pode subir de nível" (`can_level_up`) is in `GetCharacter` for the master and the owner (RN-12). The remainder of a split is rounded down, lost and reported ("2 XP se perdem na divisão"). In the gold mode, 1 XP per 1 PO (RN-09).
- NPC editor, "Ao ser derrotado": the ND fills in the XP, and "Usar 50 XP" goes back to the table's value.
- End of combat: "Experiência do combate" with "Dar 116 XP a cada um" (and "Agora não", which leaves the line "XP do combate ainda não dado").
- "Dar XP" at any time, by enemies, by gold or one-off, and "Registrar marco". "Experiência" on the campaign page, with the history for everyone and "Desfazer" of the last award, asked in place. The sheet shows XP as a read-only block and "Pode subir de nível" (also in the group list), which updates by itself while there is a session.
- Planned milestones: the master writes milestones beforehand (up to 120 characters each, at most 100, in any order: move up, move down, edit in place, remove asking in place) and only the master sees the list. Marking one as reached and choosing who levels up makes the chosen ones "Podem subir de nível"; the milestone moves to "Marcos alcançados" with the day, hour and who, and the order is not enforced. "Dar a mais alguém" gives the same milestone to a character who was left out, as a new award in the history. Undoing the last award sends the milestone back to planned (if it was the only mark) or leaves it reached for the others (if it was a "Dar a mais alguém"). A player sees only reached milestones and their own character (the line appears only after the first milestone, and the empty state never hints that some are planned). "Registrar um marco fora da lista" remains, as a text action.
- Known limit: the campaign page does not listen to the session stream, so the player's panel updates only when the page opens or when the tab returns to the foreground (`visibilitychange`); live updating there is left for later.
- Server tests: `TestMR016_EnemiesAwardSplitsTheDefeated` (two 50 XP goblins, four characters, 25 each), `TestMR016_MilestoneMarksWithoutXP` (marks everyone, no XP, the mark goes away when the level rises), `TestMR016_ManualAwardIsInTheHistory` (who, when, why, how much), `TestGoldAwardGivesOneXPPerGoldPiece`, `TestRemainderIsLostAndReported`, `TestUndoTakesBackOnlyTheLastAward`, `TestSecondEnemiesAwardForTheSameEncounterIsRefused`, `TestModeMustFitTheCampaign`, `TestOnlyLivingPlayerCharactersGetXP`, `TestPlayersNeverWrite`, `TestAuthorizationMatrix`, `TestRN20_PlayersNeverGetAnNPCsXP`, `TestMR016_PlannedMilestones`, `TestMR016_ReachingAPlannedMilestoneLetsTheChosenLevelUp`, `TestMR016_GiveAReachedMilestoneToSomeoneElse`, `TestMR016_UndoOfAPlannedMilestone`, `TestMR016_AMilestoneMarkedOffTheListIsReachedToo`, `TestRN20_PlayersSeeOnlyReachedMilestones`, `TestMR016_PlannedMilestonesNeedAMilestonesCampaign`.
- Screen tests (Playwright `e2e/tests/xp.spec.ts`, `@MR-016`): "o XP de um combate: o mestre dá pelo resumo, a ficha do jogador sobe ao vivo e o histórico guarda" (also `@RN-12`), "\"Agora não\" deixa o XP do combate para depois, e a linha abre \"Dar XP\" com o motivo e o total", "um prêmio avulso, a qualquer hora: os erros aparecem ao sair do campo e o histórico mostra o prêmio", "por ouro: o mestre digita as peças de ouro e cada um recebe a sua parte", "por marcos: o marco marca quem pode subir de nível, sem nenhum número de XP, e a marca some quando o mestre sobe o nível" (also `@RN-12`), "desfazer o último prêmio: a pergunta fica no lugar, o foco vai para \"Voltar\", e o histórico guarda o desfazer", "o XP que o NPC dá: o ND preenche o XP, o valor digitado fica e \"Usar\" volta ao da tabela" (`@RN-20`); `e2e/tests/milestones.spec.ts` (`@MR-016`, `@RN-12`, `@RN-20`); `a11y.spec.ts` ("as telas de XP passam no axe e nas conferências de layout", `scanMilestoneScreens`). The split arithmetic on screen ("116 XP para cada", "2 XP se perdem na divisão") is proved in Vitest, since the e2e table has one player only.

#### Related
- How XP is awarded: the ND or typed XP, the master decides who receives, rounding down, the level-up notice for master and player, and the history for everyone. See [RN-09](rules.md) and [RN-12](rules.md).
- What each character gains on leveling up comes from the rules engine (rules as data). See [ADR-0008](../adr/0008-regras-dnd-conteudo-como-dados-motor-puro.md).

### MR-018: Campaign document

**As a** game master, **I want** a campaign document with text, images, and links to maps and sheets that open in a dialog, **so that** I keep the campaign's notes in one place.

- Priority: MVP
- Rules: —
- Modules: campaigns (the document); maps and characters (the images, maps and sheets the links point to)

Carried over from the legacy app ([legacy app](../legacy-app.md)).

#### Acceptance criteria
In the MVP only the game master sees the document.

- **Given** the campaign document, **when** the game master writes text, adds an image from the gallery and a link to a map and to a sheet, **then** the document shows the image **and** the link opens the map or the sheet in a dialog, without leaving the document.
- **Given** a player, **when** they request the document, **then** the server refuses (only the game master sees it).

#### In the app
- Screens: `/campaigns/:id/document` and the "Documento da campanha" panel on the campaign page, game master only (see [Design](../design.md#campaign-document), [Architecture](../architecture.md#campaign-document), [Data model](../data.md#tables-by-module)).
- Contract: `CampaignDocumentService` with `GetCampaignDocument` and `UpdateCampaignDocument` (`campaign_document.proto`). One document per campaign, in Markdown, up to 200 KiB. Saving checks the revision that was read (`aborted` if someone saved first). Table: `campaign_documents`.
- Links to the app itself: `[text](map:<id>)`, `[text](character:<id>)` and `![caption](image:<id>)`. The server stores the text as sent and never opens the links; the app resolves them through the usual calls, with the usual authorization.
- The map dialog shows the whole map with every point (hidden ones marked "Escondido"), the caption, "Mapa atual da Sessão N" when the open session is on that map, and "Abrir no editor de mapas".
- A player sees only "Só o mestre vê o documento da campanha", and the campaign page does not show the panel. The server answers `permission_denied` to a player and `not_found` to a non-member or a pending member.
- Tests: `TestMR018_MasterWritesTheCampaignDocument`, `TestMR018_PlayersCannotReadTheDocument`, `TestCampaignDocumentAuthorizationMatrix`, `TestUpdateCampaignDocumentRefusesAStaleRevision`, `TestSavingTheSameDocumentTwiceIsNotAConflict`, `TestUpdateCampaignDocumentFirstSavesRace`, `TestUpdateCampaignDocumentValidates`, `TestCampaignDocumentGoesWithTheCampaign`; Playwright `campaign-document.spec.ts` (`@MR-018`: toolbar, save, reload, the map and sheet dialogs, the conflict between two tabs, the warning when leaving unsaved, deleted targets).

### MR-019: Image gallery

**As a** game master, **I want** an image gallery, **so that** I can use the images in documents and maps.

- Priority: MVP
- Rules: RN-10
- Modules: maps

Carried over from the legacy app.

#### Acceptance criteria
Limits: JPEG, PNG or WebP, up to 10 MB per image, 300 images and 500 MB per campaign.

- **Given** the game master in the gallery, **when** they upload a JPEG, PNG or WebP image of up to 10 MB, **then** it appears in the gallery **and** the stored file has none of the original's metadata (EXIF).
- **Given** a player, **when** they request the campaign's gallery, **then** the server refuses.
- **Given** an image used on a map, **when** the game master tries to delete it, **then** the app says which map it is on.

#### In the app
- Server: the upload (`POST /uploads/images`), images and thumbnails (`GET /images/{id}` and `/images/{id}/thumb`) and `GalleryService` (list, rename, delete). Only JPEG, PNG and WebP are accepted; an image with too many pixels is refused; the image is re-encoded with no metadata (see [Architecture](../architecture.md#maps-module-gallery-and-images)).
- `/campaigns/:id/gallery` (`web/src/app/pages/gallery/`), game master only: the quota; the upload area (the "Enviar imagem" button or dragging files onto the page, several at once, sent one after the other, each with its own progress and "Cancelar envio"); the privacy reminder; a grid from newest to oldest; rename in place; delete with in-place confirmation; the image dialog with previous and next. The app checks type and size before sending, and every refusal, from the app or the server, becomes a Portuguese notice with the file's name. A player sees "Só o mestre vê a galeria da campanha."; a non-member sees "Campanha não encontrada". See [Design](../design.md#gallery-and-images).
- The "Galeria" panel on the campaign page, game master only: the 5 newest images, the quota and "Abrir galeria".
- The gallery picker (`web/src/app/shared/gallery-picker/`), used by the "Novo mapa" form, the document editor and "Mostrar imagem": pick an image, or upload a new one and pick it at once.
- Each image says which maps use it (`GalleryImage.used_in_maps`); the card shows "Usada em Mirathel e arredores". Deleting an image a map uses fails with `failed_precondition` and the `ImageInUse` detail; the card says "Essa imagem está num mapa. Troque a imagem do mapa antes de apagá-la."
- Tests: `TestMR019_MasterUploadsAnImageWithoutItsMetadata` (a JPEG with EXIF and GPS), `TestMR019_PlayersCannotListTheGallery`, `TestMR019_AnImageAMapUsesCannotBeDeleted`, `TestDeletingAnImageAMapUses`, `TestListGalleryImagesShowsWhereEachIsUsed`; Playwright `e2e/tests/gallery.spec.ts` (`@MR-019`: the downloaded file has no EXIF block; a text file named `.png` and a GIF show the Portuguese error; the player sees only the notice; rename, delete, the dialog arrows, focus back on the card) and the axe scans in `a11y.spec.ts`.

### MR-024: Approve the character from the invite

**As a** game master, **I want** to approve or refuse the character a player created through the invite, **so that** only characters that make sense for the table stay in the campaign.

- Priority: MVP
- Rules: RN-15
- Modules: campaigns, characters

#### Acceptance criteria
Each invite chooses whether it requires approval, and a refusal deletes the character and the membership.

- **Given** I am the game master of "Mirathel", **when** I generate an invite, **then** I can tick "Exigir aprovação do mestre" **and**, unticked, the invite works as before: whoever accepts joins at once.
- **Given** an invite that requires approval, **when** the player accepts it (already signed in, or signing in through the invite), **then** they go straight to creating the character, which is born "Pendente de aprovação" **and**, while they wait, they see only the campaign's name and their own character, which they keep editing.
- **Given** a pending character, **when** the game master opens the campaign, **then** they see it under "Esperando aprovação", can open the sheet and, **when** they approve, the character becomes a draft **and** the player becomes a player of the campaign.
- **Given** a pending character, **when** the game master refuses it, **then** the character is deleted, the player does not join the campaign **and** needs a new invite to try again.
- **Given** I am a player, or a pending player, **when** I try to approve or refuse a character, **then** the server refuses.
- **Given** a pending player who has not created the character yet, **when** the game master opens the campaign, **then** they see who is pending without a character, with a button to remove them; **and**, after 30 days without a character, the pending membership is deleted automatically.
- **Given** a pending player of "Mirathel", **when** they accept a plain invite (no approval) to the same campaign, **then** they become a player at once, because a plain invite counts as the game master's approval; if they already had a character waiting, the character is approved too, and the invite spends one use. An invite that requires approval, or that is no longer valid, changes nothing.

#### In the app
- Contracts: `CreateInviteRequest.requires_approval` and `Invite.requires_approval`; `Campaign.awaiting_approval`; `CharacterService.ApproveCharacter` and `RejectCharacter`; `Character.can_approve`; the `NOT_PENDING` and `AWAITING_APPROVAL` reasons of `CharacterBlocked`. Tables: `campaign_invites.requires_approval` and `campaign_members.status` (`active` or `pending`). See [RN-15](rules.md), [Architecture](../architecture.md#pending-member) and [Data model](../data.md#tables-by-module).
- A pending member gets through only the calls on the `authz` list `pendingMayCall`; every other call answers `not_found`, so the member looks like a stranger.
- Pending members without a character: `ListPendingMembers`, `RemovePendingMember`, and the deadline `campaign_members.pending_expires_at` (row TTL, 30 days; creating the character clears it).
- Screens: the "Exigir aprovação do mestre" checkbox in the invite form; the invite takes the pending member straight to "Criar personagem"; the notice "Esperando a aprovação do mestre" on the campaign and on the sheet; the game master's "Esperando aprovação" list in the "Personagens" section; the buttons "Aprovar personagem" and "Recusar personagem" (with "Confirmar recusa") on the sheet. On the campaign page, under "Membros", each person without a character shows the label "Sem personagem", the join date and the date they leave on their own; "Remover" opens a confirmation in place with focus on "Cancelar". If the person created the character meanwhile, the list refreshes and the message points to the pending characters. Only the game master sees this.
- Tests, by criterion:
  - First: `TestMR024_MasterChoosesWhetherAnInviteRequiresApproval`, `TestRN15_InviteWithoutApprovalMakesAPlayerAtOnce`.
  - Second: `TestRN15_AcceptingAnInviteWithApprovalMakesAPendingMember`, `TestSignInWithAnApprovalInviteGoesToCreateTheCharacter`, `TestMR024_PendingPlayerCreatesTheirCharacterAndWaits`.
  - Third and fourth: `TestMR024_MasterApprovesAndThePlayerJoins`, `TestMR024_MasterRejectsAndThePlayerStaysOut`.
  - Fifth: `TestMR024_OnlyTheMasterApprovesOrRejects` and the pending-member columns of `TestAuthorizationMatrix`.
  - Sixth: `TestQ24_MasterSeesAndRemovesPendingMemberWithoutCharacter`, `TestQ24_OnlyTheCampaignsMasterManagesPendingMembers`, `TestQ24_PendingMembershipExpiresAfter30Days`, `TestQ24_CreatingTheCharacterClearsTheDeadline`.
  - Seventh: `TestQ25_PlainInvitePromotesPendingMember` (in `campaigns` and `characters`), `TestQ25_OnlyAWorkingPlainInvitePromotes`.
  - The rule: `TestRN15_PendingCharacterIsNotPartOfTheCampaignYet`, `TestRN15_ApproveAndRejectRace`, and in `authz` `TestRN15_PendingMemberOnlyGetsThroughTheAllowedCalls`, `TestRN15_PendingMemberLooksLikeAStranger`, `TestPendingMayCallIsTheAgreedList`.
  - Playwright `e2e/tests/character-approval.spec.ts` (`@MR-024`: the character created through an approval invite stays pending until approved; the game master refuses and the player stays out; the game master sees and removes someone who joined and has no character) and the axe scans.

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
- **Given** the session ended or the game master stopped showing the image, **then** the players no longer receive the image's ID.
- **Given** an image the game master wants the players to keep seeing, **when** they turn on "Deixar com os jogadores" and then stop showing or swap the image, **then** it stays with the players, in an "Imagens que o mestre deixou" list on their session page, until the game master removes it ("Tirar"); the list belongs to the campaign and survives the end of the session.

#### In the app
- `PlayService.SetShownImage` (game master only, open session only, only an image from the campaign's gallery) stores the image in `game_sessions.shown_image_id`. `GetLiveSession` returns it (`shown_image`, with the name as caption) and the stream carries `shown_image_changed` to everyone. One image at a time, independent of the current map. Deleting the image from the gallery stops showing it. A new session starts with no image. See [Architecture](../architecture.md#what-the-session-shows).
- A player downloads the image (`GET /images/{id}`) only while it is shown, while it is left with the players, or while it is the background of a map they can see; otherwise `404`, even with the ID stored. Their browser asks again on every use (`Cache-Control: private, no-cache`). See [RN-10](rules.md) and [Serving images](../architecture.md#serving-images).
- "Deixar com os jogadores": `SetShownImage` has `keep` (stored in `game_sessions.shown_image_keep`; off whenever a new image is shown). When on, stopping, swapping or ending the session moves the image to the campaign's list (`campaign_left_images`) in the same transaction. `ListLeftImages` (any active member, with or without a session) reads the list; `TakeBackLeftImage` (game master only) removes one; the stream sends `left_images_changed`, a hint without content. Deleting the image from the gallery removes it from the list.
- Game master screen: the "Imagem para os jogadores" panel ("Mostrar imagem", the gallery picker with "Mostrar aos jogadores", "Trocar imagem", "Parar de mostrar"), the "Deixar com os jogadores" switch ("Ligado"/"Desligado") and the "Deixadas com os jogadores" list with "Tirar".
- Player screen: the "O mestre está mostrando" block with the image, its name as caption and "Ver em tela cheia", which appears and disappears live and is announced; and the "Imagens que o mestre deixou" list with "Ver em tela cheia", hidden while empty and without a screen-reader announcement when it changes. The list appears only on the session page, not on the campaign page. See [Design](../design.md#maps-and-the-shown-image).
- The image's name is the uploaded file's name and is shown to the players as the caption, so the game master can rename it before showing it ("covil-secreto-do-lich" would give a secret away).
- Not in scope: automatically keeping every image already shown in a per-player "chest" of handouts would be a new story; the list only holds what the game master chose to leave, and it is the same for every player in the campaign.
- Tests: `TestMR028_MasterShowsAnImageToThePlayers` (show, stop, delete, and the refusals: player, image from another campaign, no open session), `TestMR028_MasterLeavesAnImageWithThePlayers`, `TestRN10_PlayersOnlyFetchImagesTheyCanSee`, and the `SetShownImage`, `ListLeftImages` and `TakeBackLeftImage` rows of the `play` authorization matrices; Playwright `shown-image.spec.ts` (`@MR-028`).

### MR-025: Register table content

**As a** game master, **I want** to register races, classes, subclasses, backgrounds, spells and rules that are not in the SRD 5.1, and to set my table's rules, **so that** the campaign uses the material my table plays and I have full control of the product.

- Priority: MVP
- Rules: RN-23, RN-24, RN-25
- Modules: rules, characters, campaigns, play, maps

The story covers own classes **and** subclasses, races and subraces, backgrounds, spells, house and dice rules, and the campaign's grid. A player's proposal (MR-026) and reading rules from a PDF (MR-027) are separate, later stories.

#### Acceptance criteria
- **Given** I am the game master of "Mirathel", **when** I register a new class with its hit die, skills, the 20-level table and its features, **then** the class appears in the character editor and in the level-up only in "Mirathel" **and** the sheet calculates its numbers with it (RN-23).
- **Given** I am the game master of "Mirathel", **when** I register a subclass of an SRD class (a Bard college, a Wizard tradition), **then** it appears in that class's subclass choice only in "Mirathel", with its always-prepared spells, if it has any.
- **Given** content registered in "Mirathel", **when** I open another campaign of mine, **then** it does not appear there: content is per campaign.
- **Given** I am the game master of "Mirathel", **when** I register an own spell, **then** it appears for the characters of "Mirathel" who can learn it **and** not in another campaign; if it has an attack or a saving throw and damage, combat resolves it like an SRD spell.
- **Given** I register an area spell (a 4.5 m cone, a 6 m radius sphere), **when** I save it, **then** the area appears in its description, **and** in combat the caster chooses the creatures it hits, as with an SRD area spell.
- **Given** a new character entering at the party's level, **when** the player creates it with more than one class (Rafa creates Corvina, Wizard 3 and Cleric 1, at level 4), **then** the editor has one block per class, with the level and the subclass of each (the table's included), **and** the server checks the multiclass prerequisites, as in the level-up.
- **Given** a table class that a sheet uses, **when** the game master changes one of its numbers, **then** the sheet shows the new number, locked sheets included, **and** whatever fell outside the rules appears on the sheet as a notice ("A classe mudou"), without blocking any other edit (RN-23).
- **Given** table content that a sheet uses, **when** the game master archives it, **then** the sheet keeps it **and** it no longer appears as a new choice; nothing in the table content is ever deleted.
- **Given** a feature the game master registers, **when** they pick its effect, **then** they pick from a closed menu (modifier, proficiency, resource, sense, roll mode, action, extra attack, choice, note, granted spell), or leave only the text: table content never runs code.
- **Given** I am the game master of "Mirathel", **when** I choose on the "Regras da mesa" page how hit points are gained, how ability scores are made, the critical hit and who sees the death saving throws, **then** the server follows the choice in every sheet and every combat (RN-24); the standard array, point buy and 4d6 appear with the label "SRD 5.2.1 (regras de 2024)".
- **Given** a campaign that allows 4d6 drop lowest, **when** the player rolls the ability scores of a new sheet, **then** the server rolls and stores them, and rolling again returns the same.
- **Given** a map drawn with 3 m squares, **when** the game master says "cada quadrado deste desenho vale 3 m", **then** the app counts four 1.5 m squares in each, and movement, range and fog follow the usual rules (RN-25).
- **Given** a combat the game master starts without a map ("theatre of the mind"), **when** the player moves, **then** they spend movement by number ("Restam 6 m"), with no grid position, **and** the game master judges the range (RN-25, ADR-0017).
- **Given** the "Opções para os jogadores" screen, **when** the game master switches off a class, subclass, race, subrace, background or spell (SRD or table), **then** players no longer see or pick it; what is on, they see in full, with numbers and effects.
- **Given** an archived table entry, **when** the game master unarchives it, **then** it appears again as a new choice.
- **Given** a character with a free-text background (the editor's "Outro"), **when** the player fills it in, **then** they choose, as in the SRD 5.1 rule "Customizing a Background", two skills, two tools or languages in total, one feature (with their own text) and the equipment, **and** the sheet calculates with it.

#### In the app
**The engine.** `rules.Content.With(Overlay)` adds the table's classes (with the 20-level table and the four casting types), subclasses (including the third-caster and always-prepared spells), races and subraces, backgrounds and spells (with target and area) to the SRD without changing the SRD, and the sheet calculates with them. Content is stored per campaign and served by `TableContentService` (see [Architecture](../architecture.md#live-table-content)).
- Tests (`rules`): `TestWithAddsTheTableContent`, `TestWithDoesNotChangeTheBase`, `TestWithConcurrently`, `TestWithRefusals` (one row per rule: key, `handler`, a choice outside the SRD, a dangling reference, limits, formula, a table without 20 rows), `TestLevelUpSweepTable`, `TestLevelUpSweepTableMulticlass` (table classes from 1 to 20), `TestDeriveTableCharacterGolden`, `TestSpellDetailsOfTableSpells`, `TestThirdCasterSlots`, `TestAlwaysPreparedDoNotCount`, `TestArchivedEntriesStillResolve`, `TestMissingTableKeysNeverPanic`.

**Storing and serving.** The game master creates, edits, archives and unarchives entries of the six kinds.
- The server makes the key from the name and the keys of the features (stable while the feature exists). Every write checks the whole campaign with the same code as the SRD and returns one violation per field, at the exact path (`table_spell.range.distance_ft`, `table_spell.damage[1].dice`, `table_class.levels[4].slots[2]`...), all at once.
- Content takes effect at once, locked sheets included. A sheet that falls outside the rules shows "A classe mudou" (only with a new notice that depends on the entry, with the sentences of what no longer fits; it clears itself when corrected, and saving the sheet does not clear it) and never refuses another edit.
- An archived entry is not a new choice: create, edit and level-up refuse it, and a sheet that already uses it keeps working. An archived entry can still be edited. A spell never goes from cantrip to leveled spell.
- Players read the playable entries in full and never the archived ones (not in the level-up, not in spell details, unless their sheet uses it); no archived key reaches a player, not even by reference. `ListContent` names languages, proficiencies and damage types for every member (a player never reads `language:common`), takes a `character_id` so that a player's sheet keeps what the game master archived or switched off, and create/edit/archive/unarchive responses carry `characters_using`.
- Tests (`characters`): `TestTableContentEveryKindIsCreatedUpdatedArchivedAndBroughtBack`, `TestTableContentFeatureKeysAreStable`, `TestTableContentKeysNeverChangeAndNamesAreUnique`, `TestTableContentRefusalsAreFieldViolations`, `TestTableContentRefusalListsEveryFieldAtItsPath`, `TestTableContentWritesCarryHowManySheetsUseTheEntry`, `TestListContentNamesLanguagesProficienciesAndDamageTypes`, `TestTableContentLimits`, `TestTableContentAccess`, `TestTableContentStaleAndRevision`, `TestTableContentIsLive`, `TestTableContentConcurrentWrites`, `TestTableContentChangeShowsOnTheSheet`, `TestTableContentAnUnrelatedEditIsNeverRefusedForAChange`, `TestTableContentArchivedIsNeverANewChoice`, `TestSheetsThatUseTableContentStayInTheirCampaign`; in `rules` `TestEntryViolationPaths`, `TestAnEntryReportsEveryViolationAtOnce`.

**"Opções para os jogadores" (the option switches).** `TableContentService.SetOptionSwitches` (game master only) switches on and off, in one transaction, up to 700 classes, subclasses, races, subraces, backgrounds and spells of the SRD and of the table (`campaign_content_off`; everything on by default). `ListOptionSwitches` returns the screen's list with each option's state (`off`, `archived`, `hidden`, the parent class or race) and how many player sheets use it. See [Architecture](../architecture.md#player-options-and-the-live-hint).
- A switched-off option is treated as archived for players in every read (the catalog, `ListTableEntries`, `ListSpells`, `GetSpellDetails`, the level-up options and references inside other entries). A switched-off class takes its subclasses with it, and a switched-off race its subraces.
- A new choice of a switched-off option is refused to a player (`SWITCHED_OFF_CONTENT` on create and edit; `SWITCHED_OFF_CHOICE` on level-up). A sheet that already has it keeps working, an unrelated edit is never refused, and the game master is never refused (any sheet they edit may have the option). A sheet with a switched-off class still reaches the subclass level: a subclass is judged only by its own switch (likewise a subrace under the sheet's own switched-off race).
- Each write raises the content revision, and every write to table content and every switch sends the content-free hint `content_changed` to the live session.
- Screen `/campaigns/:id/content/options`: one group per kind with a counter ("Raças: 9 de 10 ligadas"), search, "Ligar todas" and "Desligar todas", each option's switch with how many sheets use it and the note of a switched-off parent, all saved at once; a switched-off option that a sheet uses says the sheet keeps working. The spell, race, subrace, background, class and subclass editors have a "Disponível para os jogadores" switch. The content list and entry page mark a switched-off entry. The content list, the options, "Magias", the character editor, the level-up and the open sheet re-read what they show when the table changes (`content_changed`, only with the session open).
- Tests: `TestRN23_SwitchedOffOptionsAreNeverSeenByPlayers`, `TestRN23_ANewChoiceOfAnOffOptionIsRefused`, `TestRN23_TheRevisionAndTheCacheFollowTheSwitches`, `TestRN10_ContentChangedTellsTheSession`, `TestMR025_TheSwitchesAreTheMastersAlone`, `TestRN23_ConcurrentSwitchesAreOrdered`; Playwright `content-options.spec.ts` (`@MR-025 @RN-23 @MR-045 @RN-10`) and the `a11y.spec.ts` scans.

**Table rules outside combat.** See [RN-24](rules.md) and [Architecture](../architecture.md#table-rules-mr-025-rn-24).
- Hit points on level-up: `TestRN24_TheLevelUpFollowsTheHitPointsRule` (the "roll" rule refuses the average, the "average" rule refuses the die, the default accepts both, and `GetLevelUpOptions` says which applies).
- Ability scores of a new sheet: `TestRN24_TheScoresFollowTheMethod` (standard array, point buy, 4d6 and typing, each accepted and refused; NPCs and game master edits stay free), `TestRN24_ATableCanSwitchMethodsOff`; in `rules` `TestCheckStandardArray`, `TestPointBuy`, `TestCheckTyped`, `TestAbilityRolls`.
- The server's 4d6: `TestRN24_TheServerKeepsTheFourD6` (the same rolls on a second call, spent by one sheet, new for the next; a refused sheet does not spend them), `TestRN24_PhysicalDiceAreTypedOnce` (the player types the dice once, and the campaign's dice rule applies), `TestRN24_ThePendingMemberMakesTheirScoresToo`.
- Style and stored rules: `TestRN24_TableRulesRoundTrip`, `TestRN24_TheStyleIsWorkedOutNotStored` (a style fills in the rules; editing by hand gives "Personalizado"), `TestRN24_OnlyTheMasterWritesTheRules`, `TestRN24_TheRulesRefuseWhatBreaksTheLimits` (20 reminders of 200 characters), `TestRN24_NewMapsStartWithTheFogTheTableChose`.
- XP mode: `TestRN09_TheXPModeChangesAfterCreation`, `TestRN09_ChangingTheXPModeAsksOnceXPWasAwarded` (changes at once when no XP has been given; otherwise asks for `confirm` with the number of awards and of XP; nothing is converted).
- `TestRN24_TheTableRulesAreReadInTheCallersTransaction`: reading the rules inside a write goes through the single-connection pool.
- **The "Regras da mesa" page** (`/campaigns/:id/rules`, from the campaign's "Regras da mesa" panel; the game master edits, a player reads it read-only): the table style on top, which fills in the dice, combat and fog and leaves each editable; hit points; the ability methods (at least one, with the label "SRD 5.2.1 (regras de 2024)" on the three from the SRD 5.2.1); the critical hit; the death saves; the reminders (up to 20, 1 to 200 characters, one line each, no switch: deleting a reminder turns it off); the XP mode with the question in place; the link to the maps' grid and to the table content. The dice mode is edited only here: the campaign's "Dados" panel just states it and links here. "Combate com mapa" is the default stored for new combats. See [Design](../design.md#table-rules-ability-scores-and-grid-calibration-mr-025).
- **The "Habilidades" step of the character editor**, for the player creating the sheet, in the methods the table allows (standard array, point buy with "Restam N pontos", server 4d6, typing 3 to 18), with the method sent along in `CreateCharacter` and the server's refusal given by reason. The app shows neither the modifier nor the race bonus in this step; the server calculates them on the sheet. Level-up follows the table's hit-point rule: no choice when the rule fixes the way.
- **Grid calibration** in the "Grade" panel of the map editor ("Calibrar o quadrado": 1.5 m, 3 m, 4.5 m, 6 m or another multiple of 1.5 m up to 30 m), with "Mudar a grade?" only when the change erases what was painted. The "Créditos" page carries the SRD 5.2.1 attribution, the same as the `NOTICE`.
- Tests: Playwright `e2e/tests/table-rules.spec.ts` (`@RN-24`, `@RN-09`, `@RN-25`) and the Vitest of each piece.

**Table rules inside combat.** The server applies the critical hit ("dados dobrados" or "máximo mais uma rolagem", on every critical hit: weapon, spell, creature and NPC, with a map and in theatre) and hides the death saving throws from anyone but the owner and the game master (the word "Caído" on the combatant, in the log and on the stream; "Estável" and "Morto" stay visible to all). The pending damage carries the rule (`critical_rule`) so the screen can show the die hint. See [RN-24](rules.md) and [Architecture](../architecture.md#table-rules-in-combat).
- **Combat without a grid (theatre of the mind).** The mode belongs to the combat (`encounters.mode`, `grid` or `theatre`), is chosen at the start and never changed, and comes from `StartEncounter(mode)` or, without it, from the table rule "combate com mapa" (RN-24). Tests: `TestRN25_ACombatStartsInOneModeForGood`, `TestRN25_ReadsAndTheStreamCarryTheMode` (reads and the `encounter_changed` hint say the mode).
  - Movement by number: `TestRN25_SpendMovementByNumber` ("Restam 3,0 m", never more than the turn, Dash doubles, out of turn refused, the game master spends for an NPC, undo restores, a hidden NPC does not exist for the player).
  - The opportunity attack the game master offers: `TestRN25_OpportunityAttacksAreOfferedByTheMaster` (the question reaches the right player and nobody else, one reaction per round, refusal, withdrawing the offer, what cannot be offered).
  - Attacks, spells and actions without reach: `TestRN25_AttacksAndCoverWithoutReach` (every target the rules allow, four degrees of cover, a hidden NPC kept out of the player's JSON), `TestRN25_SpellsAndActionsWithoutReach` (a spell with one target, several, an area, touch; Help, Dash, Dodge, Disengage).
  - What does not exist without a map: `TestRN25_WhatNeedsAMapIsRefused` (move, jump and place on the map refused with a reason; an empty "where can I go" is valid; `SpendMovement` and the offer are refused in a combat with a map), `TestRN25_ATrapDoesNotExistInACombatWithoutAMap`, `TestRN25_SummonedCreaturesAndWildShapeWithoutASquare` (summoned creatures without a square, a creature's turn, Wild Shape, and the familiar's eye refused with `FAMILIAR_SIGHT_BLOCKED` / `NO_MAP`).
  - Conditions without speed: `TestRN25_ConditionsThatLeaveNoSpeed` (grappled and restrained have speed 0; paralyzed, petrified, stunned and unconscious cannot walk; with and without a map) and `TestRN25_AGridOfferIsNotWithdrawn`.
  - Hidden NPC (RN-10): `TestRN10_AHiddenNPCIsNeverNamedInATheatreCombat`. The rest is unchanged: `TestRN25_DeathSavesUndoAndTheEnd`; combat on a map is as before.
- **Screens.** The screen draws only what the server sends: it branches on `Encounter.mode` and never does dice, range or rule arithmetic. See [Architecture](../architecture.md#combat-without-a-grid-theatre-of-the-mind) and [Design](../design.md#gridless-combat-gastar-movimento-the-offer-and-hidden-saves-mr-025).
  - "Iniciar combate" asks "Como este combate é jogado": "Com mapa" or "Sem mapa (teatro da mente)", with the table rule "combate com mapa" as default (and "Sem mapa" when the session has no current map). While the session page is still reading the current map, "Iniciar combate" waits, with "Lendo o mapa atual…": the dialog never opens as if there were no map. If that read fails, the button stays off with "Não deu para ler o mapa atual" and "Tentar de novo": a combat started then would be in the theatre of the mind, which does not change afterwards. In "Sem mapa" the dialog shows and asks for neither a map nor a grid, and says once why ("Sem mapa, o app não sabe onde ninguém está."); the mode holds until the combat ends.
  - Game master, no map: the combat strip gets the label "Teatro da mente"; in place of the map, the card "Ações do ..." (of an NPC and, in this mode, also of a player, so the game master can spend the movement of someone who left) with "Gastar movimento" (a number, in 1.5 m steps, never more than what is left), the order, the log ("Toren gastou 6,0 m de movimento") and "Cobertura dos alvos" (no cover, half +2, three-quarters +5, and total, which prevents targeting). From an NPC's card, "Oferecer ataque de oportunidade" (the mover is the one whose turn it is; the game master picks whom they left the reach of) opens the question on the player's phone, and the game master sees the wait with "Seguir sem esperar" and "Retirar a oferta". Following without waiting never takes anyone's reaction. No "Sem quadrado no mapa" notice, no fog, trap or door.
  - Player, no map: the "Combate sem mapa" panel in place of the map; "Gastar movimento" in place of "Mover" opens a sheet with the number and "Depois restam 3,0 m" (a browser preview, a subtraction; the real number is the server's `movement_left_dft`) and a notice that the app does not check path or reach; the target list is whoever the character sees and the rules allow attacking (the server decides who is in), with no distance and no "Longe demais" ("O mestre decide quem está ao alcance"); the opportunity-attack question is the usual one, without a square.
  - Critical hit: with physical dice, the damage step says what to roll by the table rule ("role os dados duas vezes" or "o máximo mais uma rolagem", the maximum as a fixed part the app adds itself; the total shown before confirming is what the server stores), on weapon attacks, spell attacks and the game master's card, with and without a map.
  - Hidden death saves: the other players read only "Caído" (or "Estável"/"Morto"), with no marks or counts, and the log says "Esta mesa só deixa o dono e o mestre verem os testes contra a morte"; the owner and the game master see the marks, with the label "Só você e o mestre" (on the owner's card) and "Dono e mestre" (in the game master's order); the log line that carries only "stabilized" (no d20 or count) says "Brisa estabilizou".
  - Tests: Playwright `e2e/tests/theatre.spec.ts` (`@MR-025`, `@RN-24`, `@RN-25`, `@RN-20`, `@RN-18`), the `a11y.spec.ts` scans, and the Vitest of each piece (`core/combat/theatre.spec.ts`, `critical.spec.ts`, `pages/live-session/combat/theatre/`, `start-combat/`, `death-saves/hidden-death-saves.spec.ts`).

**Spells and the "Outro" background.**
- The target of every spell comes from the server (`SpellDetails.target`): for a table spell, what the game master wrote; for an SRD spell, the structured area of the 5e-database (`area_of_effect`, 88 of the 319 spells) and, only without it, the text. The text comes ready, in metres ("Cone de 4,5 m"). Personal and Touch ranges are valid on a table spell; Personal with "one creature" or "several" is refused. `effects/spell_targets.json` corrects the target of about 40 SRD spells. Tests: `TestSpellAreas` (the importer), `TestSRDSpellTargets`, `TestEverySRDSpellHasATarget`, `TestStructuredAreasAgainstTheText`, `TestSpellTargetOverrides`, `TestSpellTargetOverridesAreChecked`, `TestSpellTargetMaxTargets`, `TestMetersPT`, `TestSpellTargetLabels`, `TestTableSpellTargetsAreTheMasters`, `TestTableSpellRangeAndTargetMustAgree` (`rules`), `TestMR025_TheSpellDetailsSayWhomItReaches` (`characters`).
- A table spell in combat goes through the same `CombatSpell` with the campaign's content: the attack, the saving throw with half, healing, an area against three targets, "several creatures" (one more per level), the cantrip by tier, concentration, slots and the log, with the name in Portuguese. Tests (`play`): `TestMR025_ATableSpellAttackInCombat`, `TestMR025_ATableAreaSpellWithASaveAgainstThreeTargets`, `TestMR025_ATableHealingSpell`, `TestMR025_SeveralCreaturesConcentrationAndOnlyTheCaster`, `TestMR025_ATableCantripGrowsByTheCharactersLevel`.
- The "Outro" background follows the SRD 5.1 rule: two skills, two tools or languages in any mix, the feature (the player's name and text) and the equipment. `Validate` checks the limits, what is missing is a notice (never an error), and `Derive` applies the tools, languages and feature and shows the equipment. Tests: `TestCustomBackgroundDerive`, `TestCustomBackgroundIssues`, `TestCustomBackgroundValidate`, `TestCustomBackgroundIsLockedInTheLevelUp` (`rules`), `TestMR025_TheOutroBackground` (`characters`). The tools and languages of "Outro" come from the catalog (`NamedKey.kind`).
- Table races, subraces and backgrounds carry everything the editor shows (size, speed, darkvision, "+2 e +1 à escolha", languages, traits; skills, tools, languages, equipment and the feature as a note). A table background's equipment reaches the sheet (`background_equipment_pt`).

**Table classes and subclasses, end to end.** See [RN-23](rules.md) and [Architecture](../architecture.md#table-classes-and-subclasses-end-to-end).
- The numbers and the menu come from the server: `TableContentService.GetClassTableDefaults` (proficiency bonus, the levels of ability score improvement, and a 20-row table for each way of casting, from the SRD) and `GetEffectMenu` (the kinds, the fields, the closed lists with Portuguese names, the SRD options, the formula functions and the limits). Tests: `TestTableDefaultsFollowTheSRDTables`, `TestTableDefaultsMakeValidClasses`, `TestEffectMenuIsWhatTheValidatorAccepts`, `TestEffectMenuRequiredFieldsAreRequired`, `TestEffectMenuFormulaHelpers` (`rules`), `TestTableClassEditorStartsFromTheServer` (`characters`).
- Every refusal points at its field: `TestClassRefusalsNameTheirField` (`rules`), `TestTableClassRefusalsPointAtTheirField` (`characters`).
- Multiclass at creation: `TestTableClassMulticlassAtCreation` (Corvina, Wizard 3 and Cleric 1 with the two table subclasses and the domain's always-prepared spells; Ranger 2 and Wizard 2; Fighter 3 with Wizard 2, with the multiclass slots; a prerequisite or an early subclass becomes a notice) and `TestTableClassMulticlassHitPointsFollowTheRule`.
- Creating, leveling up and playing: `TestTableClassCreationAndLevelUp` (Ícaro, level 1 to 5), `TestTableClassThirdCasterCreationAndLevelUp`, `TestTableClassSubclassPickedAtLevelUpGivesItsSpells` (`characters`), `TestMR025_AThirdCasterOfTheTableCastsATableSpell` (`play`, the "Lâmina de Nanquim").
- "A classe mudou" for common changes: `TestChangedClassSentences` (`rules`), `TestTableClassChangeIsTold`, `TestTableClassThirdCasterChangeIsTold` (`characters`): one sentence per change, with the notice tied to the entry, clearing itself when the numbers match again.

**The content screens.** See [Design](../design.md#table-content-the-list-the-editors-and-the-effect-picker-mr-025) and [Design: classes and subclasses](../design.md#class-and-subclass-mr-025).
- **The "Conteúdo da mesa" page** (`/campaigns/:id/content`, from the campaign's "Conteúdo da mesa" panel): on a laptop the game master sees the menu of the five kinds with counts (archived included), "7 de 300 entradas", search, "Mostrar" (Todas, Em uso, Sem fichas, Arquivadas) and rows with the state in words ("Em uso por 2 fichas", "Arquivado · 1 ficha usa"); an empty campaign says "Nada cadastrado ainda." and that the SRD still applies. On a phone the game master only reads and archives (the question is a bottom sheet). A player reads every switched-on entry, kind by kind, with "Da mesa", without counts, state or any button.
- **The editors** (one page, one "Salvar"): the spell (all fields, the "Alvo" before the mechanics, "Mais criaturas por nível" collapsed until asked for, "Pessoal" and "Toque" in range, optional mechanics, the classes that learn it, and the "Como os jogadores veem" preview, written from the form); the race and subrace (the six bonuses, "+2 e +1 à escolha", languages, traits with an effect from the server's menu, the race's subraces); the background (two skills, tools, languages of choice, equipment and the feature). The app sends only the fields of the chosen effect kind.
- **The class editor:** name, hit die, "Teste de resistência 1" and "2", how many skills the player picks and from which, proficiencies, the multiclass prerequisite, casting (none, full, half or pact; prepared or known; the list) and the subclass level. The 20-level table starts at the numbers the server sends (`GetClassTableDefaults`) and the game master edits cell by cell; changing the casting asks before redoing an edited table. Features have a level and an effect from the server's menu, with the counter "N de 60".
- **The subclass editor** (of an SRD or a table class): features by level; "Esta subclasse conjura" brings the third-caster table from the level it starts; always-prepared spells by class level.
- **Refusals and conflicts:** each violation (`field` and `reason`) returns on the field the path names, with the reason in Portuguese (the app never reads `message`), including on a grid cell and on a collapsed feature, which opens. The summary on top says how many fields need adjusting and what the game master typed stays; a violation without a field, or about another entry, stays on top. "Esta entrada mudou enquanto você editava" offers "Recarregar". After saving, "Fichas com aviso" (`AffectedCharacter`) lists the characters left with a notice ("1 ficha ficou com aviso"). On a phone the game master only reads and archives. A player reads the class in full (the table, "Testes de resistência"), without counts and without an archived one.
- **The character editor and level-up with table content.** Table classes, subclasses, races, subraces and backgrounds appear among the SRD's with the label "Da mesa". The first class says how many skills it picks and which saving throws it gives proficiency in; a race with a free bonus says where to put it; the "Outro" has the name, two skills, two tools or languages, the feature and the equipment. "Adicionar classe" makes one block per class (class, level and subclass, the table's included), with the total level read-only and prerequisites, hit points and slots as notices and numbers from the server (never screen arithmetic). The "Magias" step has a section per class (and per spellcasting subclass, the third-caster), by the class list and the spell's level in it, shows the always-prepared spells (when editing, read from the calculated sheet) and drops a spell outside the lists, with the reason and a "Ver em Magias" link. When creating, the editor does not show "preparadas 0 de 6" per class (it shows the saved sheet's `prepared_max` when editing). See [Design](../design.md#the-character-editor-and-level-up-with-table-content-mr-025-mr-040).
- **Level-up:** the summary shows only the rows that change, the slots of a table class's table with "Da mesa" and "Novas características"; a spellcasting subclass (third-caster) brings the "Magias" step, with the list of the class it casts from and, if it prepares, its maximum. An always-prepared spell of a subclass comes in the catalog (`Subclass.always_prepared`), so it shows when creating.
- **"A classe mudou":** the notice on the sheet (owner and game master) and the "O que mudou" sheet, with the server's sentences as they come.
- Tests: Playwright `e2e/tests/content.spec.ts` (`@MR-025`, `@RN-23`, `@RN-10`: two spells, the refusal on the field, the race Corujeiro and the player's character, archiving, what a player never receives), `e2e/tests/classes.spec.ts` (`@MR-025`, `@RN-23`), `e2e/tests/table-sheet.spec.ts` (`@MR-025 @RN-23 @MR-040`: Davi creates Ícaro; Rafa creates Corvina with the two subclasses and the spells of each list; Ícaro levels from 1 to 2; the casting subclass gets the Wizard's spells at level 3; the game master changes the class's skills and the sheet says "Guardião do Vale agora dá 2 perícias no nível 1; esta ficha tem 3." until corrected); the `a11y.spec.ts` scans (`scanTableSheetScreens`, light and dark, 320 to 1280 px); and the Vitest of each piece (`core/content`, `pages/content`, `shared/` `effect-picker`, `feature-editor`, `form-fields`, `option-switches`, `ContentWatcher`, `class-draft`, `class-editor`, `subclass-editor`, `content-violations`, `content-read`, `class-blocks`, `character-editor-classes`, `levelup-classes`, `levelup-summary`, `changed-content`).

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
- **Given** a generated dungeon, **when** it becomes a map, **then** it is a campaign map like the others, hidden from the players (RN-10) and with fog of war on, with the base light "Clara" (bright), which the game master can change to dim or dark (so, even with a textured image, the room behind a secret door shows only to whoever sees it), with the grid, walls and doors already painted on the layers, the stairs as submap points, and the list of rooms beside the map, game master only, with "Pôr uma cena nesta sala".
- **Given** a generated dungeon, **when** I edit it in the map editor (walls, doors, terrain, points), **then** "Redesenhar" redraws the image from the walls and doors as they are now, without erasing what I painted.
- **Given** a closed door, **when** a character walks into it, **then** it opens, if not locked; a secret door is a wall for the players until I reveal it (RN-26).
- **Given** the generated image, **when** the players see it, **then** it has only floor and walls: doors are drawn over it, from the layer, and no room number appears.

#### In the app
- **The generator** (`rules/dungeon`) is pure and deterministic, written clean-room (ADR-0015): the donjon generator is CC BY-NC and its code and data are never copied; a specification written in our own words was checked and implemented by a different agent. See [The dungeon generator](../architecture.md#dungeon-generator-mr-010).
- **The door layer.** Six states per square; a move opens a closed door and stops at a locked one; what the player does and never knows: see [RN-26](rules.md) and [Doors](../architecture.md#doors). Tests: `TestRN10_PlayersNeverSeeASecretDoor`, `TestRN10_FogFiltersTheDoors`, `TestMR010_AMoveOpensAClosedDoor`, `TestMR010_ALockedDoorStopsTheMove`.
- **From dungeon to map.** `DungeonService` previews (`PreviewDungeon`, the same seed gives the same plan), creates the map (`CreateDungeonMap`: hidden, grid equal to the width, fog on with the base light "Clara", walls and doors on the layers matching the generator square by square, stairs as submap points with no destination, the image of only floors and walls in the gallery, and the `generated_dungeons` record), lists the rooms for the game master only (`GetDungeonRooms`), puts the RP scene in the room (`PlaceDungeonScene`, "Sala N") and redraws the image at the same size from the current layers without erasing anything (`RedrawDungeonMap`). See [The map of a generated dungeon](../architecture.md#generated-dungeon-maps). Tests: `TestMR010_PreviewIsDeterministicAndStoresNothing`, `TestMR010_OptionsOutOfRangeAreRefusedByName`, `TestMR010_ADungeonBecomesAMap`, `TestRN10_PlayersReadNothingOfADungeon`, `TestMR010_APlacedSceneIsAnRPSceneInTheRoom`, `TestMR010_RedrawShowsTheMastersEditsAndKeepsTheLayers`, `TestMR010_RedrawNeverChangesTheSize`, `TestMR010_AFailedCreationLeavesNothingBehind`, `TestMR010_CreatingAndRedrawingAreRateLimited`, and `TestRenderDrawsFloorsAndWalls` (`maps/dungeonimg`).
- **Doors on screen.**
  - In the editor: the "Porta" tool in "Pintar" (kind, place, swap, "Tirar a porta"), with the refusal "uma porta precisa de chão dos dois lados" and a question in place before putting a closed, locked or secret door where a token is (RN-26).
  - On the map: one mark per door kind (closed, open, locked, grate, secret) in the editor, the session (fog, player and game master) and combat, and a legend made from the doors the map has. The padlock and the secret door show only to the game master, with "(só você vê)" and the crossed-out eye; the player's screen also drops a locked or secret door that might arrive (RN-10).
  - In combat: whoever walks into a closed door opens it and the log says "Toren abriu a porta."; a locked door stops the move before it, the "Mover" page stays open with "A porta está trancada." and the player's map keeps "Porta fechada".
  - The game master in the session taps a door on the map (with fog, or the combat's) and opens, closes or locks it in the door sheet (a balloon beside the door on a computer, without dimming the map; a bottom sheet on a phone); without fog, doors are changed in the editor; a secret door has "Revelar a porta secreta", which paints it closed. Outside combat the server does not let a player move the token (only the game master), so the door opens from the game master's sheet.
  - Tests: Playwright `e2e/tests/doors.spec.ts` (`@MR-010 @RN-26 @RN-10`) and the door scans in `a11y.spec.ts`.
- **"Gerar masmorra"** (route `campaigns/:id/maps/dungeon`, from the button beside "Novo mapa" in the Mapas panel; game master only, and on a phone the page says "Gerar masmorra é no notebook."): size (Mínima 21, Pequena 31, Média 51, Grande 81, Enorme 121 or "Outro" from 21 to 121; the other side is two thirds, odd), shape, the smallest and largest room side, corridors, doors, dead ends, stairs (0 to 4) and the seed with "Outra semente". The preview is the server's (`PreviewDungeon`), requested a moment after the last change and one at a time; the browser only draws it, with doors and stairs from the map legend and the count "13 portas e 8 passagens". An option out of range appears on its field, with icon and text. "Criar o mapa" (`CreateDungeonMap`, with the name, options and the preview's seed) shows the steps; the map is born hidden, with fog on and base light "Clara" (the page says so), and opens in the editor. "Parar de esperar" only stops waiting: the server finishes the map anyway, and the screen says so.
- **On the generated map.** The "Salas" list (game master only: number, size, stairs, exits with the true door kind, "Porta com armadilha: …", the room behind a secret door); "Pôr uma cena nesta sala" (`PlaceDungeonScene`, the point appears on the map at once); the chosen room with a solid 3 px frame. Stairs (submap points that are **born revealed**, and a player sees one only where they see the square) are drawn as a seal with an arrow on the map, in the legend and in the list ("Escada", never "Submapa") for the game master and the players, through the point's `stairs` field. The editor asks for the room list only if `Map.generated_dungeon` says the map is a dungeon. The layer's walls cover all the rock, and the image has a flat fill under the map's wall mark.
- **"Redesenhar"** (only when the server says the image is still the generator's) asks in place, first sends the strokes that are waiting, and every refusal has its own sentence (image swapped or grid changed, grid removed, walls changed while drawing, burst of requests).
- Tests: Playwright `e2e/tests/dungeon.spec.ts` (`@MR-010 @RN-26 @RN-10`: the same seed gives the same preview, create, the scene in the room, paint and redraw, the player never receives the list) and the generator scan in `a11y.spec.ts`.
- The generator does not place monsters, traps or treasure: for that there are the bestiary, encounters and treasure ([MR-042](#mr-042-bestiary) to [MR-044](#mr-044-generate-treasure)); traps are [MR-035](#mr-035-traps). [MR-039](#mr-039-ai-generated-images-for-dungeons-and-scenes) uses the generated dungeon to make the image.
- Out of scope: multi-floor dungeons beyond linked stairs, and the hexagonal grid. Drawing a dungeon by hand (false walls, water, chests and mimics) stays an idea for the map editor.

### MR-029: Scene hooks and clues

**As a** game master, **I want** to note, for each RP scene, the hooks, the clues and what to say, **so that** I can run the scene without losing the thread.

- Priority: MVP
- Rules: RN-10, RN-20
- Modules: play, maps, notes

#### Acceptance criteria
- **Given** an RP scene on the map, **when** the game master writes the hooks, clues and what to say on it, **then** only the game master sees these notes.
- **Given** a clue in a scene, **when** the game master reveals it, **then** it appears to the players **and** in their notes ([MR-030](#mr-030-player-notes)).
- **Given** an unrevealed clue, **when** a player opens the scene or the notes list, **then** the server sends neither the clue nor its name (RN-10).

#### In the app
- **Hooks:** the column `map_points.hooks` ("Ganchos e anotações"), Markdown up to 4,000 characters, only on a scene point, saved with the point (`UpdateMapPoint`); only the game master receives it (on the map and in the open scene).
- **Clues:** `AddSceneClue`, `UpdateSceneClue`, `MoveSceneClue` and `RemoveSceneClue`, one change per call, 1 to 500 characters, at most 30 per scene; the game master receives each with who has it ("Todos", "Só a Brisa"; "Ninguém ainda" is computed by the screen).
- **Reveal:** `RevealSceneClue(clue, character_ids)`, game master only. Only the players the game master chooses receive the clue, and the screen marks nobody in advance; each player receives it once and it cannot be undone or hidden again. The server stores a copy of the text, so editing or deleting the clue later does not change what the player received. With an open session it becomes the `clue_revealed` event (IDs only) and only whoever received it hears `notes_changed`; someone offline reads it next time. Each player recounts what they found as they like, at the table, in roleplay.
- **A scene without actions:** `OpenScene` opens any scene point. The `SceneBlocked` reason `NO_ACTIONS` stays in the enum but is no longer sent.
- **On screen:** in the editor's scene-point panel, "Pistas" (the ordered list with who has it, "Todos", "Só Brisa" or "Ninguém ainda"; ↑ ↓, edit in place, remove with the question in place and "Voltar" focused; "30 de 30" and the limit sentence) and "Ganchos e anotações" (up to 4,000 characters, with the padlock and "Só você vê"; saved with "Salvar ponto", clues at once). In the open scene, the column of clues and hooks (open on a computer, collapsed on a phone, the padlock always visible) and "Revelar", which opens a dialog (a sheet on a phone) **with nobody ticked**: the dashed button "Revelar a pista" says why it does not act, "Marcar todos" is a text action and the filled button names who receives ("Revelar para Brisa", "Revelar para 2 jogadores", "Revelar para todos"); afterwards, "Revelada para todos às 21:20" or "Revelada só para Brisa às 21:26", and "Revelar aos outros" on a partly revealed clue. Any scene point opens in the picker, even without actions.
- The `map:` and `character:` links of the hooks' Markdown appear in the open scene as in the document, but do not open anything there yet.
- Tests: `TestMR029_TheMastersHooksAndClues`, `TestMR029_ClueLimits`, `TestMR029_ARevealedClueReachesOnlyTheChosenPlayers`, `TestMR029_RevealRules`, `TestClueAuthorizationMatrix`, `TestScenesWithNoActionsOpen`, `TestRN20_PlayersNeverGetHooksOrUnrevealedClues` (reads what the player receives as JSON). Playwright `e2e/tests/notes.spec.ts`: "o mestre escreve três pistas e os ganchos no ponto, abre a cena e revela uma pista; o jogador a recebe nas anotações e nunca vê os ganchos nem a pista que não foi revelada" (`@MR-029 @MR-030 @RN-20`) and "uma cena sem ações abre, e o jogador a vê; a lista das pistas para em 30 e o jogador escreve até 300 anotações"; the screens pass axe and the layout checks in `a11y.spec.ts` (`scanNotesScreens`).

### MR-030: Player notes

**As a** player, **I want** a notepad always at hand, on the session page and on the sheet, **so that** I can jot down what happens without leaving the app.

- Priority: MVP
- Rules: RN-10, RN-20
- Modules: notes, maps, play

#### Acceptance criteria
- **Given** I am in the campaign, **when** I write a note on the session page or on the sheet, **then** only I see the note **and** it is kept for the next session.
- **Given** a scene I have already discovered (revealed or opened), **when** I tag the note with it, **then** the note shows the scene.
- **Given** a scene I have not discovered yet, **when** I open the scene list to tag, **then** it does not appear, and its name never reaches my phone.

#### In the app
- The `notes` module and `NotesService` (`ListNotes`, `CreateNote`, `UpdateNote`, `DeleteNote`, `ListNoteScenes`), for the active player only and only on their own notes: 1 to 2,000 characters, at most 300 per player per campaign. The list also carries the revealed clues ("Pista do mestre", read-only, outside the 300), newest first, with a filter by scene.
- The game master and other players never read a note (`not_found`). Deleting the account deletes the notes.
- A scene is discovered when its point is revealed on the map or the scene is opened in a session (even hidden), for the whole group, and stays discovered if the point is hidden afterwards. This holds with fog of war (MR-036) too: revealing is the game master's choice, so a revealed scene enters the notes of the whole group even in a place no character sees; the fog keeps hiding the map. Only a discovered scene serves as a tag, and the picker (`ListNoteScenes`) lists only those; an undiscovered scene is refused as if it did not exist.
- **On screen:** the "Anotações" button (outlined, with icon and word) in the app bar, on every page of the player's session; with a new clue it says "Anotações · 1 nova" ("Anotações · 1", with `aria-label`, below 360 px). A clue arriving opens a notice ("O mestre revelou uma pista para você.", "Abrir anotações" and a 44 px ✕) that stays until opened or dismissed. The "Anotações" sheet (a sheet on a phone, a dialog on a computer) has the list (the clue marked "Pista do mestre", read-only, with a padlock), the filter by scene (only discovered scenes and "Sem cena", each with its count; back to "Todas" on close), the two empty states, "Nova anotação" with the scene tag and the 2,000 limit, and edit and delete (delete asks in place). It re-reads on `notes_changed` and once after each `ready`.
- On the character sheet (computer), the "Anotações" panel is the first block of the fourth column, with the same notes; they stay editable with the sheet locked, and the game master never gets the panel, not even on a player's sheet.
- Tests: `TestMR030_PlayerNotesArePrivate`, `TestMR030_TagsOnlyDiscoveredScenes`, `TestMR030_NoteLimits`, `TestMR030_DeletingTheAccountDeletesTheNotes`, `TestNotesAuthorizationMatrix`, `TestRN20_PlayersNeverGetHooksOrUnrevealedClues`. Playwright: the two `e2e/tests/notes.spec.ts` tests cited in MR-029 (the player writes the note with a discovered scene, the picker never lists an undiscovered scene, the game master never sees the panel, the sheet shows the 300 limit).

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

#### In the app
- The portrait is the NPC sheet's `portrait_image_id`. The stage holds up to 4 NPCs per session, in order, with who is speaking; closing or swapping the scene empties it. A hidden NPC in combat can enter the scene (that does not reveal them in the combat order). See [Architecture](../architecture.md#npcs-on-stage).
- The NPC editor has "Retrato" (the short sheet and the Básico step of enemy and boss): initials without an image, "Escolher/Trocar retrato" in the gallery picker, "Remover retrato" with the question in place, all saved with the sheet; the portrait appears on the NPC card in combat and in the sheet header.
- The game master sees "Em cena" in the open scene ("Dar a fala"/"Fala agora", "Tirar de cena", "Pôr em cena" in place or in a sheet on a phone, "4 de 4"); the players see the stage with cut-outs without a box, the speaker highlighted, and tap a character to see them larger. The stage disappears when empty.
- The "Pôr em cena" list comes from one call, `ListCharacters`, whose row for each NPC carries `portrait_url` (game master only), without reading the sheets.
- Tests: `TestMR031_TheMasterPutsNPCsOnStage`, `TestMR031_APlayerSeesOnlyNameAndPortrait`, `TestMR031_PortraitsAreVisibleOnlyOnStage`, `TestMR031_DeletingAPortraitClearsIt`, `TestMR031_AnNPCKeepsAPortraitFromTheCampaignsGallery`, `TestMR031_APortraitMustBeAnImageOfTheCampaign`, `TestMR031_TheMastersNPCCardHasThePortrait`, `TestMR031_TheMastersListCarriesThePortraitURL`, `TestStageAuthorizationMatrix`, `TestTransparentPNGKeepsAlpha`; Playwright `stage.spec.ts` (`@MR-031`, `@RN-20`), `a11y.spec.ts` (`scanStageScreens`), and the Vitest of `stage-*`, `portrait-field` and `StageController`.

#### Related
- Uses the images of the [gallery](#mr-019-image-gallery), like [MR-028](#mr-028-show-an-image-to-the-players).

### MR-032: Combat highlights

**As a** table, **I want** a highlights screen at the end of combat, **so that** we celebrate who did what: which player dealt the most damage, healed the most and took the most damage.

- Priority: MVP
- Rules: RN-20
- Modules: play

#### Acceptance criteria
- **Given** a combat that ends, **when** the master ends it, **then** the table sees, per category, the player whose character dealt the most damage, healed the most and took the most damage (the "tank").
- **Given** an NPC dealt or took damage, **when** the highlights screen appears for a player, **then** it shows only the players' numbers, never the NPCs' HP, AC or rolls (RN-20).
- **Given** an ended combat, **when** the table opens the highlights, **then** it sees five categories, "Mais dano causado", "Mais cura", "Tanque", "Golpe final" and "Acertos críticos", each with its number and everyone who tied; a category in which everybody has 0 is left out.
- **Given** a hit of 28 damage on a goblin with 7 HP, **when** the highlights are computed, **then** it counts 7: only the HP that actually came off.
- **Given** an action the master undid, or damage nobody applied, **when** the highlights are computed, **then** it does not count.
- **Given** a player, **when** they ask for the highlights, **then** they receive only their own character's row of the table (zeros included) and never another player's; only the master receives the whole table; a combat that has not ended is refused.
- **Given** the session had treasure found ([MR-041](#mr-041-treasure-and-xp-by-gold)), **when** the session summary is shown, **then** it has the highlight "Mais tesouro encontrado": the gold pieces (PO) each character found in that session, with a treasure found by two split between them, rounded down; a treasure found outside a session, or unmarked later, does not count; every member receives the category, in any XP mode.
- **Given** the session had scene checks with a DC, **when** the session summary is shown, **then** it has the highlight "Mais testes passados fora do combate" (such as deception and intimidation), counting only rolls from scenes that showed the DC to the players and only from actions with a DC: a roll in a scene that hid the DC counts in no category, for anyone.
- **Given** a session that ended, **when** the master opens the summary, **then** they see the duration, how many combats and scenes there were, "Testes passados fora do combate" ("9 de 12"), the highlights of all combats added up and the table per player ("Pensantus: 3 de 4"); **and** the player sees the winners of each category with their number (without the "de N") and their own result, never another player's (RN-20).

#### In the app
- **Combat highlights.** `CombatService.GetCombatHighlights` computes everything from the `session_events` of the ended combat. "Mais dano causado" and "Tanque" also count the temporary HP that absorbed damage.
- **On screen, end of combat.** The master's combat summary has "Destaques do combate" between the summary numbers and the XP, with the table "Números de cada jogador". The player sees the card "O combate acabou" at the top of the session, with "Você" and "Seu resultado" until closed. "Seu resultado" shows the four numbers of their own character, zeros included, from the row the server sends to the player.
- **Session summary.** `PlayService.GetSessionSummary(campaign_id, game_session_id)`, from any member, only for a session that ended (an open one is refused with `SESSION_NOT_ENDED`: its numbers still change, and each combat already has its own highlights). It reuses the highlights code (`tallyHighlights`, `categoriesOf`), adds up the session's combats per character and adds the `CHECKS_PASSED` category. The app knows the summary exists from the `session_ended` stream event, which carries the session id. "Mais tesouro encontrado" is `HIGHLIGHT_KIND_TREASURE_FOUND` and `SessionCharacterSummary.treasure_found_po`, read from `maps` through the `play.MapKeeper.TreasureFoundIn` interface; the number comes from the treasures as they are now, not from events.
- **Session summary on screen.** When the session ends (the master confirms "Encerrar sessão", or the stream sends `session_ended`), the page reads `GetSessionSummary`.
  - The **master** lands on "Sessão encerrada": the duration, the combats, the open scenes and "Testes passados fora do combate 9 de 12". "Resumo da sessão" has the highlights of the whole session (the combat ones and "Mais testes passados fora do combate", with "de 5 tentados" when the winner tried 5) and the table "Testes passados fora do combate" ("Pensantus 3 de 4"), master only, with the note that only scenes that showed the DC count. "Voltar à campanha" is the only button.
  - The **player** gets the card "A sessão acabou" with the duration, the winners and their numbers (without "de N"), "Você" on what they won and "Seu resultado, Pensantus" (the four combat numbers, if they fought, and "Testes passados fora do combate 3 de 4", if they tried any). There is no table for the player. The card replaces the one of a combat still open. "Fechar" and the X leave the plain notice "A sessão acabou". If the summary cannot be read, the page shows that notice.
  - "Mais tesouro encontrado": the master gets a block in "Resumo da sessão" (the table of `shared/highlights`, with the caption "Só conta o que foi marcado durante a sessão."), one row per character who found something (in the order they appeared) with the value in PO, and the note "Quem não encontrou nada não aparece. Um tesouro encontrado por duas pessoas divide o valor, arredondado para baixo."; with no treasure, the row "Nenhum tesouro registrado nesta sessão." with an icon. The category's tile leaves the master's highlights (the block already shows it, in full). The player sees it as a tile of the card "A sessão acabou" (the winner and the value) and in "Seu resultado" ("Tesouro encontrado"). The server sends neither the treasure's name nor the time in the summary, so the row shows only who and how much. "Achado por Brisa" belongs to the highlight only, never to the XP (see [MR-041](#mr-041-treasure-and-xp-by-gold)).
  - The tiles, the per-player table and the card are the same pieces as the end of combat, in `shared/highlights`.
- **Tests.** `TestMR032_HighlightsOfTheAmbush` (the reference combat: Toren 23 damage, Brisa 24 taken, Pensantus and Toren with 2 finishing blows), `TestMR032_HighlightsSkipTheUndoneAndCountWhatHappened`, `TestMR032_OverkillDoesNotCount`, `TestMR032_UndoneActionsAreSkipped`, `TestMR032_DamageTakenIsWhatTheMasterApplied`, `TestMR032_HealingIsWhatWasGivenBack`, `TestMR032_TiesNameEveryoneAndZerosAreLeftOut`, `TestMR032_HighlightsAuthorizationMatrix`, `TestMR032_SessionSummary`, `TestRN20_SessionSummaryHidesWhatTheSceneHid`, `TestMR032_SummaryCountsTreasureFoundInTheSession` (package `maps`, which marks and unmarks through `MapService`). E2E: `stage.spec.ts` (`@MR-032`), `session-summary.spec.ts` (`@MR-032`, `@RN-20`: "ao encerrar a sessão o mestre vê o resumo com a tabela por jogador e o jogador vê o cartão \"A sessão acabou\"; um teste rolado com a CD escondida não conta") and `gold.spec.ts` ("o resumo da sessão tem Mais tesouro encontrado...").

#### Related
- [MR-041](#mr-041-treasure-and-xp-by-gold): the treasure that feeds "Mais tesouro encontrado".
- The session summary appears at the end of the session (artboard E8-11, states 4 and 5). See [Architecture](../architecture.md#combat-highlights) and [Architecture](../architecture.md#session-summary).

### MR-033: Print the map with the grid

**As a** master, **I want** to print the current map, or save it as PDF, with the grid at table scale, **so that** I can play with miniatures.

- Priority: MVP
- Rules: RN-21
- Modules: maps

#### Acceptance criteria
- **Given** a map with a grid, **when** the master opens "Imprimir com a grade", **then** they see the screen "Imprimir o mapa" with the 2.54 cm (one inch) square and A4 paper, and the sheet count: 30 × 20 squares give 76.2 × 50.8 cm, on 9 A4 sheets.
- **Given** the print screen, **when** the master changes the square size (1 to 10 cm, with comma or dot) or the paper (A4, A3, A2, A1, Carta or Ofício), **then** the summary, the sheet preview and "As contas" (one row per paper, in both orientations) change at once; the orientation is the one that uses the fewest sheets, landscape on a tie. With 2 cm on A3, that is 3 sheets in portrait.
- **Given** a map larger than a page, **when** the master prints, **then** the map is split into sheets with a 1 cm margin and 1 cm overlap, with the grid aligned between them, each sheet carrying the label of where to paste it ("Página B2 · cole à direita da B1 e abaixo da A2") and a 5 cm ruler to check the scale.
- **Given** a size outside 1 to 10 cm, **when** the master types it, **then** the screen shows the error, leaves the preview empty and the "Imprimir" button dashed, doing nothing.
- **Given** a print of more than 16 sheets, **when** the master sets it up, **then** an amber warning names the paper that uses the fewest; above 36, the preview stops showing the labels. Nothing is blocked.
- **Given** a map without a grid, **when** the master looks at the map page, **then** the "Imprimir com a grade" button is disabled, with the reason written beside it.
- **Given** a map hidden from the players, **when** the master prints, **then** the whole map comes out: the print is the master's. The player never sees the button or the screen (the route answers "Só o mestre imprime o mapa.").
- **Given** the print, **then** only the image and the grid come out: points, marks and tokens are left out.

#### In the app
- Browser only: the grid and the image already come from the map, and nothing runs on the server. The screen is the route `/campaigns/:id/maps/:mapId/print` (`pages/maps/map-print`); the math lives in `print-math.ts`, with no DOM.
- What the printer gets is print CSS only (`@page` with the chosen paper size and a 1 cm margin, `@media print`). There is no server-side PDF and no extra dependency. See [Design](../design.md#print-the-map).
- The square size is chosen by the master, and so is the paper. "Ofício" is the Brazilian one (21.6 × 33 cm).
- The browser may shrink the page when printing, so the screen asks for 100% scale and no headers and footers, and each sheet carries the 5 cm ruler.
- Tests: Vitest `print-math.spec.ts` (the math checked in the design review, every paper in both orientations, the tie-break, the labels, the grid on each sheet), `map-print.spec.ts`, `map-head.spec.ts`. Playwright `map-print.spec.ts` (`@MR-033`): the master opens the print, changes to 2 cm and A3 and sees 3 sheets; a size outside 1 to 10 cm locks "Imprimir"; a map without a grid shows the reason; the player does not see the button and the route says it is the master's; on paper (`emulateMedia` for print and `page.pdf`) one sheet comes out per page, to scale, with no control. The screen passes axe and the alignment checks in `a11y.spec.ts` (`scanPrintScreens`).

#### Related
- [RN-21](rules.md).

### MR-034: Special movement

**As a** player, **I want** the app to handle jumping, difficult terrain and cover, **so that** the movement rules apply on screen as they do at the table.

- Priority: MVP
- Rules: RN-21
- Modules: play, rules

#### Acceptance criteria
- **Given** a character with Strength 16, **when** they jump, **then** the app shows the jump's reach in distance and height, calculated from Strength, and deducts it from movement.
- **Given** a square marked as difficult terrain, **when** the character enters it, **then** each square costs double the movement.
- **Given** a target behind cover marked on the map (half or three-quarters), **when** someone attacks it or it resists a Dexterity effect, **then** the server adds +2 or +5 to the AC or to the save, along the line between the two; behind a wall it cannot be targeted. The master can also mark the cover of a combatant for what the map does not show.
- **Given** the Dash, **when** the player uses it, **then** it keeps working together with difficult terrain.
- **Given** another creature in the way, **when** the character passes through its space, **then** the space of a non-enemy costs as difficult terrain, an enemy's cannot be crossed (only with two sizes of difference), and nobody ends the movement in another's space.
- **Given** an enemy next to the character, **when** the character leaves its reach without having used Disengage, **then** the app offers an opportunity attack to whoever controls the enemy, and the movement waits for the answer.

#### In the app
- **Movement on the server.** `MoveCombatant` charges the circle (the straight line, in tenths of a foot), difficult terrain, walls, other creatures (side, size, "Aliado"), flight and jumping (`jump`, the running start, the limits in `GetTurnOptions`). `GetMoveOptions` sends every reachable square and the reason for the refused ones. The reach of attacks and spells counts the squares between the centres, rounded down. Map cover and the master's mark enter the AC and the Dexterity saves, and full cover removes the target. Disengage turns on the turn's mark.
- **Jump.** Toren (Strength 16) jumps 16 ft with a running start and 8 ft standing, over the rubble; Brisa (Strength 8) cannot do the same (`TestMR034_JumpsAndTheRunningStart`). On `rules/combat.JumpLimits`: Strength 16 gives 16 ft of distance and 6 of height with a running start.
- **Map layers.** The master paints four layers on a map's grid: difficult terrain, wall, cover (half or three-quarters) and light (`PaintMapCells`, up to 400 squares per call, the last write wins, repeating changes nothing). `GetMapLayers` returns them packed in the `rules/grid` format. The master reads all four. The player of a map they can see, without fog, reads wall, terrain and cover, never the light. Changing the grid columns or the image erases the layers, and both changes are refused while there is a combat on the map.
- **Opportunity attack.** A move that leaves the reach of an enemy that sees the mover and has the reaction gives it an offer (`Encounter.opportunity_offers`). A long jump provokes too; the forced movement the master marks (`forced`) does not. The movement lands at once and the mover's turn waits (`OPPORTUNITY_PENDING`) until each offer is answered and its damage resolved. The reactor's controller attacks (`RollAttack` with `opportunity_offer_id`, without the reach check, with cover measured from the square the mover left), declines (`DeclineOpportunity`) or the master goes on without waiting (`SkipOpportunity`). At 0 HP the mover goes back to the square they left the reach from, if free. An offer nobody can answer anymore stops holding the turn. Undoing the move takes the offers along.
- **Screens.** The "Mover" page reads `GetMoveOptions` (the circle, each square's cost, the reason for a refusal, the warning of who may provoke and the question of a known trap) and has "Saltar" (distance and height, with the server's limits). Map layers are drawn by `shared/map-layers`. The target list of an attack or spell states the cover with its origin ("Meia cobertura (do mapa)", "Três quartos (marcada pelo mestre)") and leaves out who a wall covers entirely. The master's order shows the cover of each enemy against whoever has the turn and has "Marcar cobertura" (radios in place) and "Marcar como aliado". The opportunity attack has the player's warning, the master's card, the wait of whoever moved, "Esperando a reação do mestre" and "Seguir sem esperar". A short-sheet NPC has the field "Tamanho".
- **Map editor.** On the map page, on a computer, the master chooses "Pontos | Pintar". "Pintar" has the tools Terreno difícil, Parede, Cobertura (Meia, Três quartos) and Luz (Claro, Penumbra, Escuro), "Apagar", and a 1×1 or 3×3 brush.
  - Dragging paints square by square (none is skipped between two mouse moves), clicking paints one, Shift erases. The keyboard paints too: arrows move the brush cursor, Space paints, Esc leaves, and the arrows announce column and row quietly. On a tablet, one finger paints and two fingers move and zoom the map.
  - In "Pintar" the map is clean: no names, markers and tokens at 40%.
  - What is painted shows at once and goes to the server in batches (up to 400 squares, one batch at a time, in stroke order: the last write wins). "Camadas" says "Tudo salvo", "Salvando" or "Não salvou" (the reason is what the server answered; "Tentar de novo" only when the server did not answer, and a refusal discards what was waiting). Before reading what is already painted, or if the read fails, nothing is painted and "Tudo salvo" is never said. A stroke goes to the map where it was made, and leaving the page with strokes the server did not receive makes the editor ask.
  - It counts each layer ("4 quadrados · custa +1,5 m por quadrado", "2 quadrados de meia cobertura e 1 de três quartos", never the name of an object) and lets the master show or hide each one on their own map.
  - Without a grid there is nothing to paint: the tools are dashed with the reason written ("Defina a grade para pintar e ligar a névoa."), and "Definir a grade" is in the "Grade" panel. With a combat on the map painting works; "Mudar a grade" and "Trocar imagem" are off, with the reason said once ("Combate em andamento").
  - **Changing the grid, replacing the image and "Esquecer o que foi visto" ask in place** (focus on the title, "Voltar" first, a filled button saying what it erases; nothing is erased before the second click; the same question serves "Salvar as mudanças em …?", "Apagar …?" and "Desmarcar …?"). "Mudar a grade?" always opens, but carries the warning of what it erases, and only lets erase, when something is painted or seen and the size really changes (with fog on, the editor assumes the players have already seen something). "Trocar imagem" with nothing to lose goes straight to the picker.
  - The line "O que está desenhado na imagem, os jogadores veem." sits under the map state. On a phone there is no painting: a fixed notice ("Pintar só no computador"), the layers drawn with the legend, and the fog settings, which are editable.
- **Tests.**
  - Rules: `rules/grid` (`TestReachAgainstTheCave`, `TestMoveCostAgainstTheCave`, `TestCreaturesOnTheWay`, `TestCoverBetweenAgainstTheCave`, `TestLeavesReach`, `TestRange`, the layers and the line) and `rules/combat` (`TestJumpLimits`): the numbers of the cave in the drawings and those of Toren's jump (Strength 16) and Brisa's (Strength 10).
  - Server (`go test`, with the database, the drawings' cave as the map): `TestMR034_JumpsAndTheRunningStart`; `TestRN21_DifficultTerrainAndOtherCreaturesCostMore` (also covers the Dash, which doubles the speed before the cost); `TestMR034_CoverRaisesTheArmorClassOfTheTarget` (the crates, the column, a creature in the middle, the wall, the mark, the Dexterity save and the AC the player never receives); `TestRN21_TheCircleCostsTheExactLine`, `TestRN21_WallsColumnsAndSqueezesBlockAMove`, `TestRN21_GetMoveOptionsDrawsTheCircle`, `TestMR034_ReachIsCountedInSquaresBetweenCentresRoundedDown`, `TestMR034_TheMapsLayersDecideTheMove`.
  - Layers: `TestMR034_PaintingTheLayers`, `TestMR034_PaintingRefusals`, `TestMR034_AGridOrImageChangeClearsTheLayers`, `TestMR034_NoGridOrImageChangeDuringACombat`, `TestMR034_PlayersReadOnlyWhatTheyMay`, `TestMR034_LayerChangesReachTheRightStreams`, `TestHintGateMergesTheHintsOfAnInterval`.
  - Opportunity attack: `TestRN21_LeavingAnEnemysReachOffersAnAttack`, `TestRN21_WhoProvokesAnOpportunityAttack`, `TestMR034_GetMoveOptionsWarnsWhichSquaresProvoke`, `TestMR034_TheAnswersToAnOffer`, `TestMR034_TheMoversTurnWaits`, `TestMR034_TheWaitLastsUntilTheDamageSettles`, `TestMR034_TheCoverOfAnOpportunityAttackIsFromWhereTheMoverLeft`, `TestMR034_AForcedMoveIsTheMastersAndNeverProvokes`, `TestMR034_ZeroHitPointsSendsTheMoverBack`, `TestMR034_ZeroHitPointsStaysWhereTheLeftSquareIsTaken`, `TestMR034_OffersNobodyCanAnswerStopHoldingTheTurn`, `TestMR034_DecliningAndSkippingLeaveAnUndoableEntry`, `TestRN20_AHiddenReactorIsNeverNamedToAPlayer`, `TestRN20_ThePlayersWarningDoesNotDependOnAnNPCsReaction`.
  - E2E: `move.spec.ts` (`@MR-034`, `@RN-21`: the circle with rubble, the wall, the jump, cover and the mark, the offers, the cut movement, the ally); `map-editor.spec.ts` (`@MR-034`: painting walls, terrain and cover and erasing, the server keeps them; the 3×3 brush and the keyboard; the map without a grid; changing the grid and replacing the image asking; the combat on the map); `a11y.spec.ts` (every state, in both themes, desktop and phone). Vitest covers `move-plan`, `jump-plan`, `opportunity`, `cover`, `layers`, `paint-layers`, `paint-queue`, `paint-tools` and the editor components.

#### Related
- [RN-21](rules.md). See [Architecture](../architecture.md#grid-vision-and-presets) and [Architecture](../architecture.md#layers-fog-and-point-kinds).
- The rules chosen: the master paints difficult terrain, walls and cover on the map (half: low wall, crates; three-quarters: column, arrow slit), which the players see and can use; each square of difficult terrain entered costs 1.5 m more; other creatures and the opportunity attack as in the criteria above.

### MR-035: Traps

**As a** master, **I want** to place hidden traps on the map, with the DC to notice and to find, the trigger and the effect, **so that** I can surprise the table with the game's rules.

- Priority: MVP
- Rules: RN-10
- Modules: maps, play

#### Acceptance criteria
- **Given** a map, **when** the master places a trap with the DC to notice (passive Perception), the DC to find (Investigation), the trigger and the effect (damage, a saving throw), **then** only the master sees it.
- **Given** a hidden trap, **when** a character gets close and sees the square with passive Perception equal to or greater than the DC to notice, or searches and passes a Perception check (against the DC to notice) or an Investigation check (against the DC to find), **then** their player starts seeing the trap.
- **Given** a trap that fires, **when** the trigger happens, **then** the server applies the effect (damage, save) **and** the trap starts to show to the players.
- **Given** a trap not yet found or fired, **when** a player queries the map, **then** the server sends not even its position (RN-10).

#### In the app
- **Presets and content.** The eight example traps of the SRD are presets (`effects/traps.json`, `Content.TrapPresets()`) with the SRD severity table. Tests: `TestTrapPresetsOfTheSRD`, `TestTrapSeverityTables`, `TestLoadTrapsRefuses`.
- **A trap on the map.** `CreateMapPoint` and `UpdateMapPoint` accept the `TRAP` kind with the DC to notice (optional) and to find, an area from 1×1 to 4×4, the trigger (on entering or manual), the effect in parts (attack, damage that always hits, conditions, a save with what happens on failure and on success) and the state (armed, fired, disarmed), all checked with the same rules as the presets. Creating from a preset (`ContentService.ListTrapPresets`) copies the numbers and everything stays editable. A player receives only a trap revealed to their character (`RevealTrap`, which writes `trap_revealed` and warns only the chosen players), fired or revealed to all, and never the DCs, the effect or the preset (RN-10).
- **Noticing.** A player character, or a creature of theirs, that **ends a movement** (in a combat, or the token the master drops) within 3 m of an armed trap, seeing a square of the area, with the sheet's passive Perception (the creature sheet's, for a creature), minus 5 in dim light or in darkness seen by darkvision, equal to or greater than the DC to notice, comes to know the trap (`how = noticed`, event `trap_noticed`), and only their player receives the warning. A trap with no DC to notice is never noticed this way. The master reads "Quem notaria" in `GetTrapNoticers`: each character with passive Perception, the -5 of the light on the trap's squares as they see them now, whether they are on the map, within 3 m, can see and would notice.
- **Searching.** "Procurar armadilhas" (`SearchForTraps`): the player rolls Perception (against the DC to notice; a trap without that DC is never found this way) or Investigation (against the DC to find), with the app's d20 or a physical die (RN-18), against each armed trap within 3 m whose square the character sees. Passing reveals it only to that character (`searched`). The answer reads the same when there is nothing and when the check failed, and never carries a DC. If the map cannot be read afterwards to name what was found, the player is told how many traps were found and to look at the map. In a combat it is the SRD's Search action, only on the character's turn, and it spends the action (`ACTION_USED`, `NOT_YOUR_TURN`). A Perception search in dim light has disadvantage; blindsight is never lightly obscured.
- **Firing.** A trap **fires** on entering the area (a straight-line move stops at the first square of the area, a jump fires only where it lands, a token the master drops inside it fires, an NPC never does; when the master drags a character, only where the movement ends counts) or by the master's hand (`FireTrap`, which chooses who is caught; with no targets, the creatures in the area). A token fires a trap only on a map the players can see.
- **Effect.** The server rolls the attack, the damage and the save of each creature caught, with its bonus, like spell saves. Damage to an NPC or a creature lands at once, conditions go to the combatants, and damage to a player character **waits for the master** (RN-02), who applies it (`ApplyPendingDamage` in combat, `ApplyTrapDamage` outside it), changing the amount first if they want, or discards it. An NPC outside combat only gets a history line, and conditions become a reminder. The damage that waits survives the end of the combat and of the session. A fired trap stays visible to all ("Disparada"). The master disarms it after the table's thieves' tools check (`DisarmTrap`, event `trap_disarmed`), and a disarmed trap never fires.
- **Warnings.** `GetMoveOptions` marks the squares whose movement enters an armed trap the character knows (`known_trap_point_id`, `known_trap_name`), for the screen to ask "Isso entra no Fosso escondido. Mover assim mesmo?", and never one they do not know. A movement cut by an unknown trap only says it stopped, and the trap becomes public because it fired. "Desfazer" in combat undoes the whole firing (the trap armed again, HP and conditions back, pending damage erased), and only then the movement.
- **Design choices (not SRD rules).** The 3 m, measured between square centres; the trap's attack goes one by one to the creatures caught, in order; the damage of each part for each creature is one roll; a trap "Ao entrar na área" catches who is in the area, or only who entered if the effect has manual targets; an active check in dim light has no disadvantage (the master decides).
- **Master screen.** "Armadilhas do mapa" on the session page: one card per trap (Armada, Disparada às HH:MM or Desarmada; who knows it; the DCs, the trigger and the effect; "Quem notaria" with the server's numbers; "Revelar para…", "Disparar…" and "Desarmar"), the damage waiting for the master (the editable number, HP before and after, "Aplicar N de dano" or "Não aplicar"), also during a combat, and the "Registro". The server does not tell the distance in metres, only whether the character is within 3 m (`TrapNoticer.in_range`) and, in a separate field, whether their Perception would reach the DC (`passes_dc`): "Quem notaria" writes "Nota" or "Não nota" for who is close and "Se chegar a 3 m: nota" for who is far, comparing nothing in the browser. "Disparar…" with nobody marked fires for whoever the server finds there (the characters and their creatures with a token on the map), and the screen says so. If someone the master marked leaves the list before the click (the token is gone, the combat ended), the sheet names who left and waits for a new pick: an empty pick means "everyone in the area", never what the master meant. Each request has its own idempotency key (the same targets again are a retry; other targets are another request). The list of who may have been caught follows the trap's "Quem notaria": the dialog may open before the read arrives, says "Lendo quem está no mapa…" and completes when it arrives.
- **Player screen.** "Procurar armadilhas" (Perception or Investigation, in the app or with the physical die, with the second die when the server asks; in combat it is the Search action), the notice "Você notou uma armadilha.", the note "Você caiu na armadilha…" on their turn and the log of their own lines. The trap and the chest show on the map by the `MAP-LANGUAGE` marks, with a legend.
- **Editor, placing a trap.** "Armadilha" in the points bar places the point on the map and opens the panel, with the **eight SRD presets** as radios (each with a line: "Queda de 6 m, 2d6", "1 perfurante, 2d10 veneno · resistência de Constituição"; choosing one fills the form and everything stays editable) and "Começar do zero". The form has the name, the "Descrição para você", the "CD para notar (Percepção)" (empty: nobody notices on their own, as with the Poisoned Needle) and the "CD para achar (Investigação)", the notice "Não desenhe a armadilha na imagem: os jogadores veem a imagem.", the trigger area (1×1 to 4×4), the trigger (on entering or manual), the effect **in parts** the master adds and removes as text actions (Ataque, Dano que sempre acontece, Condição que sempre acontece, Teste de resistência with what happens to who fails and who passes), who is caught (the area or what the master chooses) and the state (Armada, Disparada, Desarmada). The SRD severity hint ("revés 10 a 11 · perigosa 12 a 15 · mortal 16 a 20") sits only under a save's DC and an attack's bonus, written as the server sends it (`ListTrapPresets`); the browser does not say whether the chosen DC is "perigosa". Each field says what is wrong after the first attempt to save (DC 1 to 30, dice d4 to d12 or a number 1 to 100, bonus 0 to 20). "Quem notaria" is the same table (`GetTrapNoticers`, `passes_dc`), all from the server. The three lights per character of the drawing (E9-02) do **not** exist, because the server answers one frame only: how the character sees now.
- **Tests.** Content and map: `TestMR035_PresetsFillTrapsAndLights`, `TestMR035_PointKindsAreValidated`, `TestMR035_UpdatingPointsOfTheNewKinds`, `TestMR035_RevealTrap`, `TestRN10_PlayersNeverReceiveTrapsLightsOrHiddenTreasure`. In play: `TestMR035_ThePassiveNotice`, `TestMR035_TheNoticeNeedsADCAndTheDCToBeMet`, `TestMR035_QuemNotaria`, `TestMR035_APassiveNoticeFollowsTheLight`, `TestMR035_ACreatureNoticesWithItsOwnEyesAndFiresTheTrap`, `TestMR035_Searching`, `TestMR035_APhysicalDieFollowsTheCampaign`, `TestMR035_SearchingInACombatCostsTheAction`, `TestMR035_AMoveStopsAtTheFirstSquareOfTheArea`, `TestMR035_AJumpFiresATrapOnlyWhereItLands`, `TestMR035_NPCsNeverFireATrapAndTheMasterPlacesOnlyWhereTheMoveEnds`, `TestMR035_TheMasterFiresATrapByHand`, `TestMR035_TheAttack`, `TestMR035_OutsideACombat`, `TestMR035_ATokenDroppedInTheArea`, `TestMR035_ADisarmedTrapNeverFires`, `TestMR035_TheWarningBeforeSteppingIn`, `TestMR035_TheEffectsArithmetic`, `TestRN10_NoPlayerResponseNamesAnUnrevealedTrap`, `TestRN10_AMoveStoppedByAnUnknownTrapReadsAsTheTrapFiring`. Hardening: `TestMR035_ANoticeAndOtherEventsDoNotBlockTheUndo`, `TestMR035_ADartIsItsOwnSave`, `TestMR035_TheMasterAddsCreaturesToAFiring`, `TestMR035_TheActivityOutsideACombat`, `TestMR035_ATokenFiresOnlyOnAMapThePlayersSee`, `TestMR035_ATrapDisarmedOrDeletedMeanwhileIsJustNotThere`, `TestMR035_ARetriedTrapMoveKeepsTheOffers`, `TestMR035_APerceptionSearchInDimLightHasDisadvantage`, `TestMR035_BlindsightIsNeverLightlyObscured`, `TestMR035_AnEndedCombatKeepsTheTrapDamageForTheMaster`, `TestMR035_AKeyReusedOnAnotherTrapDamageIsRefused`, `TestMR035_APassiveNoticeTellsNobodyElseAndTheMasterOnlyWithNoContent`. E2E: `traps.spec.ts` (`@MR-035`, `@RN-10`, `@RN-02`); Vitest specs of `core/traps` and `pages/live-session/traps`.

#### Related
- Comes from what the dungeon design (the old MR-010) foresaw: traps and chests.
- [MR-010](stories.md#mr-010-generate-dungeons), [MR-038](#mr-038-puzzles) (a wrong move can fire a map trap). See [Architecture](../architecture.md#layers-fog-and-point-kinds).

### MR-036: Fog of war by sight

**As a** player, **I want** to see on the map only what my character sees, **so that** exploration has the suspense of the table.

- Priority: MVP
- Rules: RN-10
- Modules: maps, play, rules

#### Acceptance criteria
- **Given** a map with light and dark areas the master marked, **when** the player opens it, **then** they see only what their character sees.
- **Given** a light source (a torch, a spell), **when** the master places it on the map, **then** the area around it becomes visible.
- **Given** a character with 18 m of Darkvision on the sheet, **when** they are in a dark area, **then** they see up to 18 m, in shades of grey.
- **Given** the master, **when** they open the map, **then** they see everything. What the player does not see never leaves the server (RN-10).
- **Given** a druid in wolf form, **when** they are in a dark room, **then** they see nothing new: the beast has no darkvision, and its senses apply in place of the character's (SRD).
- **Given** a player with a familiar on the same map, 30 m or less away, **when** they choose "Ver pelos olhos do familiar", **then** they see what the familiar sees, with its senses (the owl's darkvision), and the character is blind and deaf until the player stops; in combat it spends the action and ends at the start of the character's next turn.

#### In the app
- **Calculation.** `rules/vision` computes, from a map with walls, base and painted light and light sources, what each observer sees (bright, dim, grey with darkvision, wall seen), the union of several observers and the -5 penalty in dim light. `effects/lights.json` holds the SRD lights.
- **Settings and lights.** Each map with a grid can turn fog of war on (off by default; `SetMapFog`), with the base light (dark by default, dim or bright) and "Visão do grupo" (off). Turning fog off keeps the layers; removing the grid turns it off. The `Map` carries `fog_enabled` and `group_vision` also to the player; the base light only to the master. A `LIGHT` point (SRD preset or custom radii, 0 to 120 ft) never goes to a player. A light a character carries (`SetCarriedLight`, the player on their own character and the master on any) goes only to the master and the owner.
- **What each player sees.** On a fog map each player receives only what their character sees now or has seen, in every read (`GetMap`, `ListMaps`, `GetMapLayers`, and `GetMapVision`): no image (only the size; the image route refuses it too, and turning fog on copies the image if it serves something else), points only on squares seen or remembered, a player character's token always and an NPC's only on a square seen now, layers filtered and never the light. Each player sees through their own character (on the combatant's square while there is a combat), with the sheet's derived darkvision; "Visão do grupo" gives everyone the union. What was seen stays (`map_vision_memory`), without creatures, until the master asks "Esquecer o que foi visto" (`ForgetMapVision`) or changes the grid or the image. The master opens the map "as" a character (`as_character_id`). The stream sends `vision_changed` only to whom the view changed, and an NPC's movement only to whoever sees it. Nothing is remembered of a map the players cannot see.
- **Combat per player.** On a fog map during a combat, each player receives only the NPCs their character sees now (with "Visão do grupo", the group): out of the order, the turn ("Vez do mestre"), attack and spell targets, the log, the stream and the opportunity-attack offers, and `not_found` if the player names one. Player characters and their creatures never disappear. Movement is planned with the terrain the player knows (what they do not see is floor) and cut where something blocks it, naming no wall or creature; the `CombatMoved` hook runs after each movement. The log keeps who saw each line when it happened (`seen_by`). An opportunity attack's reactor must see the mover (an NPC, with the sheet's senses). The cover a player reads is that of what they know and see; an opportunity attack holds even if the NPC retreated into the dark; the view read before the transaction is checked (combat revision) and read again; the player's `revision` is per player; `DeleteMap` is refused during a combat.
- **The image in tiles per player.** On a fog map the player receives the image in 16 × 16-square tiles, assembled on the server only with the squares they saw or remember (opaque black elsewhere and no metadata), from `GET /images/maps/{mapa}/tiles/{tx}/{ty}`. Two images that differ only in squares the player does not know give them byte-identical tiles (proved by `TestRN10_TileOracle`); in exchange, at the fog edges 1 pixel is darkened in a PNG and up to a 16-pixel block in a JPEG. `GetMapVision` carries the index (`tiles_path`, `tile_squares`, `tiles` with each tile's revision), and a tile with no known square does not exist. There is one working copy of the image per map (up to 2,048 px, shrunk square by square, two maps in memory), one render at a time (`503` with `Retry-After` if waiting too long), a limit of cache misses per user (`429`) and a bounded tile cache; replacing the image or the grid and "Esquecer o que foi visto" discard them. See [Architecture](../architecture.md#per-player-image-tiles).
- **Wild Shape and the familiar's eyes.** A druid in Wild Shape has the beast's senses (`PartyVision`; a wolf has no darkvision: `TestMR036_AWolfSeesNothingInTheDark` reads `GetMapVision` of the wolf, what it had already seen remembered, and the vision back on taking its own form). `StartFamiliarSight` and `StopFamiliarSight` (`PlayService`) turn the familiar's eyes on and off: they need a familiar (the creature of origin `familiar`), the character and the familiar on the same map with a grid and 30 m or less apart (20 squares, at the tokens' position or, in combat, the combatants'). Outside combat it lasts until the player stops; in combat it spends the action and ends at the start of their next turn, and stopping earlier does not give the action back. The character is blind and deaf and the player sees only what the familiar sees (the character's own view leaves; other characters' views still count in "Visão do grupo"), with the conditions on the combatant, which the end removes, only those the sight gave. The sight ends on entering a combat. Fog puts the familiar as the player's observer, on its square, while it is within 30 m. Only the familiar sees for its owner: other creatures **show** (their token is a group token) but see for no one (`TestMR037_ACreatureDoesNotSeeForItsOwnerUnlessItIsTheFamiliarsEyes`; the justification is in [Architecture](../architecture.md#wild-shape-the-familiars-eyes-and-creature-tokens-mr-037-mr-036)).
- **Player and master screen (`shared/fog-map`).** The fog map in the session and in combat (tiles assembled in place, the shading of each state, the legend under the map, zoom and the group's sheets on the phone), "Carregando o mapa" with "parte N de M", the notice "Seu personagem não está neste mapa", the line "Luz que você carrega" and its sheet, the master's panel "Luz dos personagens" and "Ver como", and the familiar's eyes (the button in the line under the map, in the combat action list and in the sheet's creature card; the question, the banner and the turn's "Cego" line). Maps without fog stay as they were. The browser calculates no rule: it draws what the server sends and only counts squares ("76 quadrados vistos"). There is no "Ver como" of the combat order: the server does not send the master the order as a player sees it, and the screen does not pretend. The button "Ver pelos olhos" shows for any familiar; the 30 m distance is the server's alone.
- **Editor, fog, light and "Ver como".** "Névoa de guerra" is an editor panel, also on the phone: "Ligar a névoa" (needs the grid), the "Luz de base" (Claro, Penumbra, Escuro), "Visão do grupo" and "Esquecer o que foi visto", which asks in place. Each change goes at once (`SetMapFog`, one field per call). "Luz" is a point kind: the SRD sources the server lists as 52 px radios (name and radii in metres) and "Personalizada", with radii in metres (multiples of 1.5 m, up to 36 m, the server's limits) and the squares count below. **The reach is that of the radii, not of the lit squares:** no read gives the master which squares the light reaches (the master's view without "Ver como" is "everything seen"), so the map draws two rings (the bright light's, solid; the dim light's, dashed) and says walls cut light for real only with fog on, which "Ver como" shows. The point is the master's only: the player sees the light, never the point. "Ver como" appears in the editor with fog on, in the column beside the map: choosing a character swaps the map for the same `app-view-as-map` of the session (that player's tiles, tokens and layers) with the banner "Você está vendo o mapa como Toren" and "Voltar à sua vista"; the list counts the squares each one sees.
- **Tests.**
  - Rules: `TestVisionAgainstTheCave` (the cave drawings: Pensantus, Toren, Brisa and Sálvia in the dark, Toren and Brisa with the torch), `TestSensesBeyondDarkvision`, `TestLightIsBlockedByWalls`, `TestUnionOfViewers`, `TestPassivePenalty` and the time-budget benchmarks.
  - Settings and views: `TestMR036_FogSettings`, `TestMR036_CarriedLight`, `TestRN10_FogPlayersReceiveOnlyWhatTheirCharacterSees`, `TestMR036_FogViewsMatchTheCaveOracle` and `TestMR036_GroupVisionIsTheUnion` (the numbers of `cave-data.md`: Pensantus, Toren, Brisa and Sálvia, with and without the torch, and the union), `TestMR036_WhatWasSeenStays`, `TestMR036_VisionChangedReachesTheRightPlayers`, `TestMR036_SeeAsAPlayersCharacter`, `TestRN10_FogMapImageIsCopiedWhenShared`, `TestMR036_VisionOfAMapWithoutFog`, `TestCoalescedHintsQueueOnce`, `TestMR036_NothingIsRememberedOnAMapPlayersCannotSee`, `TestMR036_AClearBeatsALateRefresh`, `TestMR036_AColdStartTellsNobodyForNothing`, `TestMR036_ParentsAreNamedOnlyThroughWhatThePlayerKnows`, `TestMR036_AFogMapDuringACombat`, `TestMR036_PaintingTellsThePlayersWhoseLayersChanged`, `TestRN10_AFogMapsImageIsNeverReusedRaw`.
  - Combat: `TestRN10_FogCombatEachPlayerSeesOnlyTheirNPCs`, `TestRN10_FogCombatGroupVisionAndTheStream`, `TestRN10_FogCombatMovesPlanOnWhatThePlayerKnows`, `TestRN10_FogCombatTheLogKeepsWhoSawIt`, `TestRN10_FogCombatReactorsMustSeeTheMover`, plus the RN-20 ones (cover, retreat, stale view, `not_found` on every call, retry, deleting the map, a reactor with its own eyes, the Shield warning).
  - Tiles: `TestRN10_TilesCarryOnlyWhatThePlayerSaw`, `TestRN10_APlayerWhoSawNothingGetsNoTile`, `TestRN10_TileAuthorization`, `TestRN10_TileCaching`, `TestRN10_TilesAreInvalidated`, `TestRN10_TileOracle`, `TestTileBudget_*`.
  - Familiar and Wild Shape: `TestMR036_TheFamiliarsEyesGiveItsView` (the player sees the lit guardhouse through the owl, nobody else does, at 30 m the sight leaves and what was seen stays remembered), `TestMR036_FamiliarSightOutsideACombat`, `TestMR036_FamiliarSightInCombat`, `TestMR036_FamiliarSightKeepsAConditionTheCharacterHad`, `TestMR036_FamiliarSightNeedsTheFamiliarWithin30m`, `TestMR036_AWolfSeesNothingInTheDark`.
  - E2E: `fog.spec.ts` (`@MR-036`, `@RN-10`: Pensantus and Toren see different maps at the same instant and no player request downloads `/images/<id>`; what was seen stays darkened and without NPCs; the character outside the map; Toren's torch; the master's "Ver como"; Nanquim's eyes in and out of combat; the map without fog), `map-editor.spec.ts` (`@MR-036`, `@RN-10`: turning fog on, the base light, group vision and forgetting; what the master paints reaches the player only where their character sees; the Luz; "Ver como" Toren shows Toren's map) and `a11y.spec.ts` (the fog map and its states, in both themes, desktop and phone, and 1024 and 320 px for the session).

#### Related
- [MR-037](#mr-037-creatures-of-the-character) (the familiar, Wild Shape), [MR-034](#mr-034-special-movement) (walls and layers), [MR-035](#mr-035-traps) (lights and traps as points).
- Rules chosen: each player sees what their own character sees, and the master can turn on "Visão do grupo" on each map; player characters never disappear for the other players; what was seen stays darkened; the walls the master paints block sight and light, as in the official rules; an enemy the character does not see does not appear to them: attacking in the dark (even a Fireball in a suspect corner) is the master's call.
- See [Architecture](../architecture.md#layers-fog-and-point-kinds).

### MR-037: Creatures of the character

**As a** player, **I want** to control a creature of mine (the druid's Wild Shape, the undead of Animate Dead, a familiar), **so that** I can play it in combat with its own sheet.

- Priority: MVP
- Rules: RN-02, RN-20
- Modules: play, rules, characters

#### Acceptance criteria
- **Given** a druid with Wild Shape, **when** they use it, **then** the player gets the creature's sheet, taken from the SRD creatures, and acts with it in combat: the beast's AC, attacks, speeds and senses, the beast's HP separate, and no spells. The list of beasts follows their level (level 2: CR up to 1/4, no flying or swimming; 4: up to 1/2, no flying; 8: up to 1).
- **Given** a druid in beast form, **when** the beast falls to 0 HP, **then** they return to their own form and the leftover damage passes to their HP; "Voltar à forma normal" is a bonus action, and healing in the form heals the beast.
- **Given** the master, **when** they place a character's creature token on a map, **then** the whole group sees it (it is a group token, never hidden, even in fog).
- **Given** a character's creature, **when** its turn comes, **then** it enters the combat order, with its own action economy, and the player controls it.
- **Given** a character's creature, **when** it takes damage, **then** its HP is separate from the character's HP, and the master can correct it (RN-02).

#### In the app
- **Rules.** The 334 SRD 5.1 creatures are imported (`srd51/data/monsters.json`, Portuguese names `monster:<index>`), `MonsterDerived` (the creature sheet for combat, with the other speeds), the `summon` effect of Find Familiar (with the Pact of the Chain), Animate Dead and Conjure Animals, and the druid's Wild Shape (`wild_shape`, levels 2, 4 and 8, and the sheet combined as the SRD says). `ContentService.ListCreatures` and `GetCreature` serve the list and the sheet. Find Steed is out for now (it needs the rules for mounted combat).
- **Creatures of the character.** A creature belongs to the character (`character_creatures`: owner, SRD kind, a name of up to 40 characters, origin, what it can attack, the casting group, HP) and lasts until the player or the master dismisses it. `CharacterService`: `ListCharacterCreatures`, `GiveCreature` (the master gives any SRD creature), `RenameCreature`, `DismissCreature` and `AdjustCreatureHitPoints` (master, outside combat).
- **Summoning.** `PlayService.CastSummon` outside combat (Find Familiar as a ritual, **no slot**; Animate Dead, which spends the slot; Conjure Animals) and `CastSpell` with `summon` in combat (Conjure Animals: the creatures enter the order with **a single initiative**, and a total that ties with another combatant becomes a joint turn). A new familiar replaces the old one. As a house rule, Animate Dead creatures also roll one initiative (the SRD says that only of Conjure Animals), and the group uses the first creature's Dexterity bonus.
- **In combat.** A creature is a `creature` combatant (`CombatantKind.CREATURE`, `controlled_by_me`, `owner_character_id`), with its own turn and economy, which the owner player controls. The familiar does not attack. HP stays on it and is a number only for the owner and the master (RN-20). At 0 HP it is defeated and dismissed, and its HP goes back to the owner's list when the combat ends. `EndConcentration` dismisses the creatures of the casting (RN-22).
- **Wild Shape.** `CharacterService.ListWildShapeForms` lists the beasts of the level. `PlayService.AssumeWildShape` (in or out of combat; spends a use of Wild Shape and, in combat, the action) and `LeaveWildShape` (bonus action) change the form, which is part of the `vitals` (table `character_wild_shapes`). The druid fights with the beast's numbers (AC, attacks, speed, size, senses) and does not cast (`WILD_SHAPE_NO_SPELLS`). Damage lands on the beast's HP, so does healing, and when it falls what remains passes to the druid. The master's undo restores the form and the beast's HP. The beast's HP is a number only for the master and the druid's player (RN-20). The master can remove the form through the `vitals` correction (`wild_shape_hit_points_current`, 0 ends it). Events `wild_shape_started` and `wild_shape_ended` (the beast, the reason and the leftover damage).
- **Tokens.** The master places a creature's token on the exploration map with `PlaceMapToken` (`creature_id`; table `map_creature_tokens`): it is a group token. A creature's token follows its combatant when the fight ends. In a combat map the token is round with a dashed outline.
- **Sheet screen.** The "Criaturas" panel of the sheet (after "Combate" and "Magias"; only for whoever can have a creature: casts one of the three spells, is a druid with Wild Shape or already has one; for another player the server answers `not_found` and the panel does not exist, RN-20). One card per creature (the name the table gave, "Corvo · Miúdo · Familiar de Pensantus", AC, HP "1 de 1" and the speed in metres), "Ver a ficha do Nanquim" (the creature's sheet page, `/campaigns/:id/characters/:characterId/creatures/:creatureId`, in English for what is SRD, with the line that says why a familiar does not attack), "Renomear" (in place, up to 40 characters), "Dispensar" (asks in place, focus on "Voltar"; the player and the master dismiss any creature, a given one included (the server decides)) and, for the master, "Corrigir PV" outside combat. The summon sheet outside combat: Find Familiar as a ritual (the name, the 15 forms, the 4 of the Pact of the Chain, "Conjurar como ritual · 1 hora · sem gastar espaço"), Animate Dead (the slot decides how many, one kind only) and Conjure Animals (the four options with the slot's count and the animal). Only during a session (outside one the button is dashed and says "Agora não há sessão aberta."), and the server's refusal shows in the sheet itself, in words. The master gives the creature in the campaign's character list ("Dar uma criatura", search by Portuguese or English name, kind and CR across the 334 of the book) and dismisses from there; the player is warned in real time (`creatures_changed`). A creature given while the summon sheet is open is told after the sheet closes, and a failed refresh keeps the list and says "Não deu para atualizar as criaturas agora." with "Tentar de novo". The summon sheet reads `GetSummonOptions` (the spells the character casts, what each level brings, the slots, what a casting would send away), warns before casting what a casting dismisses and mixes creature kinds when the option counts several. See [Design](../design.md#character-creatures-mr-037).
- **Combat screen.**
  - When the player plays more than one combatant (the character and creatures), the turn bar gets tabs ("Sálvia · Já agiu · 13", "Lobos atrozes (2) · Sua vez · 10"; on a laptop, at the top of the page) and the page follows the turn to the tab of whoever acts; with no creatures there are no tabs.
  - The creatures' turn page ("Vez dos seus Lobos atrozes", "Depois de vocês: Nanquim") has a block per creature (image, name, owner's HP, the book's AC, "Ainda age"/"Encerrou", own action and movement, "Atacar" through the usual attack flow, "Mover o Lobo atroz 1" through the Mover page) and "Encerrar a parte dos Lobos" (which ends each one's part) or "Encerrar a vez do Nanquim". The familiar shows "O familiar não ataca" and the standard actions, and the Pact of the Chain's gets the line "Reação · Atacar".
  - Conjure Animals in combat reuses the out-of-combat summon sheet (options and beasts from `GetSummonOptions`), with the concentration line in the footer, the group's initiative d20 (the app's, or typed from a physical die) and the result ("Os 2 Lobos atrozes entram no combate com iniciativa 10 (um d20 para os dois: 8 + 2)…").
  - For the master, the order shows the creatures as dashed round tokens, "CA 14 · da Sálvia", the box of the joint turn with the group's name, the legend of the three shapes and the "Concentração" tag (solid, public: everyone sees who concentrates; the spell is on the line "Concentra em Conjurar Animais · 2 Lobos atrozes") and, only for whoever has creatures, "Perdeu a concentração" (asks in place, a notice with an icon, focus on "Voltar", "Dispensar os Lobos"; the server's undo brings the creatures back, so the question does not say it cannot be undone). An NPC concentrating on Web only has the line, without the question. The player is warned ("Você perdeu a concentração em Conjurar Animais. Os 2 Lobos atrozes sumiram."); the notice stays until they tap it, comes from the combat's data, holds only for that combat, and an undo that brings the creatures back removes it. Other players see the creatures only by the state word (RN-20).
  - Wild Shape is a line in the Action ("Transformar", "restam 2 de 2 usos · volta no descanso curto ou longo", "Sem usos", "Sem ação disponível"); the beast sheet comes from `ListWildShapeForms` (search by Portuguese or English name, the book's numbers, the cost before confirming); the beast's turn has the banner "Na forma de Lobo", the two HP pools, "Sem magias na forma de fera…", "Voltar à forma normal" (bonus action) and the book's traits. When the beast falls to 0 HP the notice "O Lobo caiu a 0 PV e você voltou à forma normal. 6 de dano passaram para você." stays until tapped. On the sheet, the "Criaturas" panel has the line "Transformar: Forma Selvagem" (only during a session).
  - What the server does not yet send (so the screen does not show it): the damage that passes from the beast to the druid as a number (the notice takes it from the difference between the character's HP before and after), the AC of a creature or beast (it comes from the book sheet) and the Portuguese names of the creatures' attacks (the screen translates the most common).
- **Names.** The Portuguese names of the creatures' attacks are `attack:<SRD name, lowercase with hyphens>` in `names_pt.json`; `TestAttackNamesPT` checks every attack of the 334 creatures.
- **Tests.**
  - Rules and content: `TestSnapshot`, `TestNamesPT`, `TestReferences`, `TestMonsterDerived`, `TestListCreatures`, `TestSummonOptions`, `TestCheckSummon`, `TestSummonLoaderRefuses`, `TestWildShapeForms`, `TestWildShapeDerived` (`backend/internal/rules`); `TestListAndGetCreatures` and `TestAuthorizationMatrix` (`backend/internal/characters`).
  - Creatures: `TestMR037_TheRitualFamiliarSpendsNoSlotAndANewOneReplacesTheOld`, `TestMR037_AnimateDeadAtTheThirdAndFifthCircles`, `TestMR037_ConjureAnimalsInCombat`, `TestMR037_TheMastersGiftRenameAndDismiss`, `TestMR037_ConcentrationEndingDismissesTheCastingsCreatures`, `TestMR037_UndoOfAConjuringTakesTheCreaturesAway`, `TestMR037_ACreatureAtZeroLeavesAndTheHitPointsGoBack`, `TestMR037_ExistingCreaturesJoinACombatAndAGroupSharesOneRoll`, `TestMR037_TheKindAudit`, `TestMR037_TheChainFamiliarAttacksOnlyWithItsReaction`, `TestRN20_CreatureHitPointsOnlyToOwnerAndMaster`, `TestMR037_SummonOptions` and the creature rows of `TestAuthorizationMatrix` (`characters` and `play`).
  - Wild Shape: `TestMR037_WildShapeBeastsByLevel`, `TestMR037_WildShapeInCombat`, `TestMR037_WildShapeDamageGoesToTheBeast`, `TestMR037_WildShapeExactDamageEndsTheFormWithNothingLeftOver`, `TestMR037_WildShapeUndoOfTheActions`, `TestMR037_WildShapeOutsideACombat`, `TestMR037_WildShapeLeavesOtherCombatantsAlone`, `TestMR037_ACreatureTokenFollowsItsCombatantWhenTheFightEnds`, `TestMR037_ACreatureDoesNotSeeForItsOwnerUnlessItIsTheFamiliarsEyes`.
  - E2E: `creatures.spec.ts` (`@MR-037`, `@RN-20`) and `creatures-combat.spec.ts` (`@MR-037`, `@RN-20`); the new screens are in `a11y.spec.ts`. In Vitest, the combat view helpers (`ownCombatant` never picks a creature), plus the specs of the creatures panel, the summon sheet and the combat blocks.

#### Related
- [MR-036](#mr-036-fog-of-war-by-sight) (the familiar's eyes), [RN-02](rules.md), [RN-20](rules.md), [RN-22](rules.md).
- See [Architecture](../architecture.md#creatures-summons-and-wild-shape-rules), [Architecture](../architecture.md#character-creatures-in-play-mr-037) and [Architecture](../architecture.md#wild-shape-the-familiars-eyes-and-creature-tokens-mr-037-mr-036).
- Rules chosen: Wild Shape (the damage left over when the beast falls to 0 HP passes to the druid, as in the SRD), Find Familiar (and the warlock's Pact of the Chain), Animate Dead and Conjure Animals are in, and the master can give any SRD creature to a character. Creatures summoned together roll one initiative and act together (the joint turn). Find Steed waits until after the MVP.

### MR-038: Puzzles

**As a** master, **I want** to create puzzles the players solve in the app, live in a scene, **so that** I can vary the pace of the session.

- Priority: MVP
- Rules: RN-10, RN-18, RN-27
- Modules: play, maps

#### Acceptance criteria
- **Given** I am the master of "Mirathel", **when** I create a puzzle of one of the six kinds ("Apagar as luzes", the combination lock, the rotating symbols, the riddle, the sequence to repeat or the cipher), **then** it is saved in the campaign and only I see it.
- **Given** a 5 by 5 "Apagar as luzes" puzzle, **when** I create it, **then** the server generates a start that has a solution and is never already solved, **and** I see how many presses are enough.
- **Given** an open session, **when** the master shows a puzzle, **then** the players see it and solve it live, all in the same state, with the clue the master wrote, **and** see who made the last move (the character's name).
- **Given** a puzzle shown, **when** two players play at the same time, **then** both moves count, and everyone sees the new state.
- **Given** a riddle, **when** a player types the answer, **then** the server compares it with the answers the master accepts, ignoring case and accents, **and** the player never receives the right answer.
- **Given** a sequence to repeat, **when** the master plays it, **then** the players see the sequence happen **and** then repeat it; the server checks the order.
- **Given** a cipher, **when** the players open it, **then** they see the encrypted message and decipher it with the key they found as a clue in the adventure; the server checks the deciphered message.
- **Given** a puzzle with hints, **when** the master releases the next hint, **then** the players see it; **when** a player passes a skill check against the DC the master set (Investigation, Arcana...), **then** they receive the next hint (RN-18: in the app or with the physical die).
- **Given** a split-information puzzle, **when** the players open it, **then** each sees only their part of the clue, the one the master gave them, **and** they must talk to solve it.
- **Given** a puzzle with consequences, **when** a wrong move happens (a wrong combination or answer), **then** what the master chose happens: a map trap fires (MR-035), a player's attempt is spent, or the limit of moves or time comes closer; at the end of the limit the puzzle stops and the master is warned.
- **Given** a puzzle shown, **when** the players solve it, **then** the server checks the solution (the player never receives the answer), the puzzle stops, the master is warned **and** what they chose in "Ao resolver" happens: only the notice (default), open a door (RN-26), reveal a map point or reveal a scene clue to whoever solved it (RN-27).
- **Given** a puzzle shown, **when** the master restarts or closes it, **then** the players see the same start again, or stop seeing it; "Gerar outro começo" ("Apagar as luzes" and rotating symbols) is a separate master action.

#### In the app
- **Server, first three kinds.** "Apagar as luzes", the combination lock and the rotating symbols, with the storage, live rounds, hints released by the master and "Ao resolver" (only notify, open a door, reveal a point, reveal a clue to whoever solved it). See [Architecture](../architecture.md#puzzles-mr-038).
- **Server, more kinds.** The riddle, the sequence, the cipher, the skill-check hint, split information and the consequences ("Ao errar": the trap, the attempt and the limit of moves or time) are three more pieces of `puzzleKind`, on the same API and tables. See [Architecture](../architecture.md#puzzles-mr-038) and [RN-27](rules.md).
- **Master, list and editor.** The "Quebra-cabeças" panel on the campaign page: the list with the four states and the empty state, "Editar" until the first "Mostrar", "Arquivar" asked on the row itself and "Mostrar os arquivados". The pages "Novo quebra-cabeça" and "Editar quebra-cabeça": the kind, the form of each kind, the clue, the hints and "Ao resolver" with the door, point or clue chosen among the campaign's maps and scenes, and the message. The design says "Apagar" in the list; the server only archives (and what was shown is session history), so the screen says "Arquivar".
- **Master, forms of the later kinds.**
  - The riddle with the text and the accepted answers (chips with a 44 px "×" and "Só você vê").
  - The sequence with the bells, tapped to put each step, "Apagar o último passo" and "Tocar para testar" (lights the steps on the master's screen only).
  - The cipher with the message, the shift or the keyword, and "Como os jogadores a veem", which **the server encrypts** (`PreviewPuzzleCipher`, after a pause), plus the scene clue that holds the key.
  - In **every** kind: "Dica por teste de perícia" (the skill among the 18 and the DC, both or neither), "Informação dividida" (up to 8 parts, each for a player character, none with two) and "Ao errar" (one way at a time: nothing, a map's trap, one attempt per player from 1 to 10, or the limit of moves and minutes; the trap and the attempt only on the three kinds that judge a move).
  - Each server refusal shows on the field it refers to (by the reason and the typed detail's field, never by the message) and the focus goes there; editing a puzzle opens with everything it had.
  - Bells go from 3 to 8 and steps from 3 to 12, as the server.
- **Master, in the session.** The panel "Quebra-cabeças" with "Mostrar aos jogadores" and "Ver ao vivo" (the list in the side column; the live card of the chosen puzzle, one at a time, in the main column, above the map): the panel as the players have it, the last move and who made it, how many are left at minimum, the hints, "Mostrar a próxima dica", "Gerar outro começo", "Recomeçar" and "Fechar" asked on the screen itself, and, when solved, who solved it, what the server did and the door opened on the map. Only for the master and with "Só você vê": the accepted answers, the sequence's steps or the message in the clear; the last move with what was typed or the bell struck, the attempts each player still has, the counters of moves and time, how many times the sequence was played, the trap that fired and who erred, who won a skill hint (with the roll) and "Tocar a sequência".
- **Player.** The player gets the notice "O mestre mostrou um quebra-cabeça" in the session and plays on the puzzle page (`?puzzle=ID`, inside the session page, in place of the panel, with the session notices on top: the trap noticed, the treasure found, the clue that arrived and the combat). One tap per light, the lock's wheels, the pillars with the links and the mural; and "Resolvido" with the master's text. The 7 × 7 panel fits in 320 × 568 px.
  - The riddle: the field, "Responder", "Não é isso." with icon and word, never saying how close, "Suas tentativas 2 de 3", and the dashed box with the reason when there are no more attempts.
  - The sequence is watched step by step at the server's pace (only the bells already revealed; the step number, the highlighted bell with its name written and spoken by a screen reader; no sound is needed), then repeated by any player, with "Errou o passo 4. A tentativa recomeçou; a Lia errou." and the server's "Passos certos 2 de 6".
  - The cipher: the card, the decoding table (a helper the app never checks), the message field and "Conferir".
  - "Tentar uma dica · Investigação": the DC never shows; in the app or by typing the physical die's d20, as RN-18 says; "Você conseguiu. Esta dica é só sua" or "Não deu desta vez."
  - Split information: the part that is only theirs, with the names of who has the others.
  - The counters ("Jogadas 7 de 10", "Tempo 4:48 de 5:00", which runs on its own from the server's deadline), "A armadilha disparou: Dardos envenenados." (only for who sees the point), and the neutral "parou".
- **What the browser knows.** The browser only draws what the server sends. A move goes with a new key and, if the answer does not arrive, it is repeated with the **same** key; the screen follows the answer (and the read the `puzzle_changed` hint triggers) only when the revision is greater. The browser never sees the next step of the sequence before the server reveals it, the answer, the unencrypted message, the key, the DC or another player's part. The server does not send another player's typed answer, so "resolveu às 21:34" does not say what was answered; the key clue's message is in the player's notes, with a button "Abrir as anotações", because the note does not carry the clue's ID. See [Design](../design.md#puzzles-mr-038-e10-06-and-e10-12).
- **Extensibility.** `PuzzleHost`, one panel per kind and the master and player panels were made to receive a new kind without rewriting anything.
- **Tests, server, first kinds.** `TestMR038_CreateEachKindAndReadItAsTheMaster` (the three kinds, master only, the start never solved and the minimum presses), `TestMR038_CreateChecksWhatTheMasterWrote` (each refusal with its reason), `TestMR038_ASeedGivesTheSameStart`, `TestMR038_EditUntilShownAndArchive`, `TestMR038_ShowPlayAndSolveEachKind` (everyone sees the same state and who played last; solved freezes), `TestMR038_TwoPlayersMovingAtOnce` (both moves count), `TestMR038_TwoMovesThatSolveAtOnce`, `TestMR038_AMoveWithTheSameKeyIsMadeOnce`, `TestMR038_ResetReseedAndClose`, `TestMR038_AnEditDropsThePreparedRun`, `TestMR038_TheMasterSolveMessage`, `TestMR038_TheHintIsThrottled`, `TestMR038_APuzzleNeverClosesTheUndoChain`, `TestMR038_HintsAreReleasedOneByOne`, `TestMR038_SolvingOpensADoor`, `TestMR038_SolvingRevealsAPoint`, `TestMR038_SolvingRevealsAClueToTheSolverOnly`, `TestRN10_PlayersNeverReceiveTheAnswer` and, in `rules/puzzle`, `TestLightsMinimumMatchesBruteForce`, `TestLightsKernelCases`, `TestPillarsMinimumAndLinks`.
- **Tests, server, later kinds.** `TestMR038_RiddleCreateAnswerAndSolve` and `TestMR038_RiddleChecksWhatTheMasterWrote` (create, show, the wrong answer, the right one without case or accents, the restart and each refusal), `TestMR038_SequencePlayedThenRepeatedWithAWrongStep` (play step by step without showing a step early, moves refused before and during, the wrong step, the whole sequence), `TestMR038_SequenceChecksWhatTheMasterWrote`, `TestMR038_APlayScheduleItsSteps`, `TestMR038_CipherRoundTripAndTheKeyAsAClue` (round trip, the key as a clue only who found it reads, the keyword), `TestMR038_CipherChecksWhatTheMasterWrote`, `TestMR038_AHintWonBySkillCheckIsOnlyThePlayers`, `TestMR038_AHintByAppDiceAndTheDiceMode` and `TestMR038_HintChecksWhatTheMasterWrote` (physical and app die, pass and fail, one attempt per hint, the campaign's dice mode, the DC that never goes out), `TestMR038_SplitInformationEachPlayerReadsOnlyTheirPart` and `TestMR038_SplitInformationChecksWhoGetsAPart` (the pending member gets nothing), `TestMR038_AWrongMoveFiresTheTrapOnce`, `TestMR038_OnWrongChecksWhatTheMasterWrote`, `TestMR038_AnAttemptIsSpentPerPlayer`, `TestMR038_AMovesLimitStopsThePuzzle`, `TestMR038_ATimeLimitStopsThePuzzle`, `TestMR038_ConcurrentWrongAnswersAreCountedExactly` (attempts and the limit counted exactly with simultaneous moves), `TestRN10_PlayersNeverReceiveWhatTheNewKindsHide` and, in `rules/puzzle`, `TestFold`, `TestMatches`, `TestCipherRoundTrip`, `TestSequenceStrike`, `TestSequencePlayback`.
- **Tests, web.** Vitest: `puzzle-draft.spec.ts` (each form becomes the request and comes back on edit, the refusals, skill and DC, the parts, "Ao errar" with one way only), `puzzle-play.spec.ts` (own wrong answer and not another's, "Tentar uma dica" in the app and with a physical die, the key that repeats, the sequence read again at each step with a fake clock), `puzzle-boards.spec.ts` (the light named "Luz na linha 2, coluna 3, acesa", the keyboard, each move of each kind), `puzzle-form.spec.ts`, `puzzles-panel.spec.ts`, `master-run.spec.ts`, `puzzle-errors.spec.ts`. E2E: `puzzles.spec.ts` (`@MR-038 @RN-10`: the lock that opens a door end to end with the player's JSON without the solution, the 7 × 7 panel at 320 × 568, the list, the pillars and the questions on the screen itself) and `puzzles-more.spec.ts` (`@MR-038 @RN-27 @RN-10 @RN-18`: the riddle wrong then right, with the player's JSON without the answers; the sequence seen and repeated, with a wrong step that fires the trap; the cipher solved with the key found as a scene clue; the skill hint with the physical die; split information, with each player's JSON holding only their part; the move limit that stops the puzzle; and the three fitting in 320 × 568). Screens and states are in `a11y.spec.ts`.

#### Related
- [MR-047](#mr-047-more-puzzles): the ideas left for after the MVP.
- [MR-035](#mr-035-traps) (the trap a wrong move can fire), [RN-10](rules.md), [RN-18](rules.md), [RN-26](rules.md), [RN-27](rules.md). See [Architecture](../architecture.md#puzzles-mr-038) and [Design](../design.md#puzzles-mr-038-e10-06-and-e10-12).
- The six kinds, the hints, "Ao resolver", the consequences, the skill-check hint and split information are all in the MVP, so the session does not get repetitive. The rotating symbols recall the pillars of Skyrim, with our own symbols.

### MR-039: AI-generated images for dungeons and scenes

**As a** game master, **I want** to generate an image from a dungeon, a map or a scene description (scene art, an isometric view of the map, or the map itself with texture, matching the grid) and ask for adjustments, **so that** I can show the table the place I am describing.

- Priority: MVP
- Rules: RN-10, RN-28
- Modules: maps (gallery)

#### Acceptance criteria
- **Given** a generated dungeon ([MR-010](#mr-010-generate-dungeons)) or a scene description, **when** the GM asks for an image with their own text and style, **then** the app generates the image and stores it in the campaign gallery, hidden from the players.
- **Given** a generated image, **when** the GM writes a new request ("darker", "with a bridge"), **then** the app edits it with knowledge of the scene, without starting over, **and** stores the new one next to the previous one.
- **Given** the campaign's monthly limit has been reached, **when** the GM asks for another image, **then** the app refuses and says why and when the limit resets.
- **Given** a map with a grid (generated or drawn), **when** the GM asks for "O mapa com textura" (the textured map), **then** the app generates the map itself seen from above, with floor and walls in place, cropped and fitted to match the grid, **and** the GM can set it as the map image without erasing the layers or what the players have already seen.
- **Given** a map, **when** the GM asks for "Vista isométrica" (isometric view), **then** the app generates the map art in isometric perspective from what the players see now, **and** it goes to the gallery as scene art. The reference drawing contains only the squares some character sees, only the creatures the players see, and the unrevealed secret door as a wall; other rooms and hidden enemies are left out. On a map without fog, the whole map goes, minus what is hidden from the players.
- **Given** a map with tokens, a combat or NPCs in the scene, **when** the GM asks for an image, **then** they choose which NPCs and enemies appear in it (gallery portraits go as references) **and** only those appear; they can also pick other gallery images as references. For an image made from a map, the choice offers only the NPCs and enemies the players see now.
- **Given** an image request, **when** the app sends it to the AI service, **then** only the map drawing, the room list (only for the textured map of a generated dungeon), the GM's text and the references they chose are sent, never personal data (RN-28).
- **Given** a map with a secret door not yet revealed, **when** the app builds the reference drawing for the textured map, **then** the door goes as a wall, **and** the image does not show the passage (RN-10, RN-26).
- **Given** scene art or an isometric view made from a map, **when** the app builds the reference drawing, **then** it carries only what the players see now (the squares, the creatures, the secret door as a wall), **and** the image does not show what they have not discovered yet. The goal is immersion: the image shows the scene as the group sees it.
- **Given** the service refuses a request or returns no image, **when** the GM waits for the image, **then** the app says so in Portuguese ("O serviço não gerou uma imagem") without spending the month's slot.

#### In the app
- **Service.** `ImageGenerationService`: `GetImageGenerationStatus`, `GenerateSceneImage`, `EditGeneratedImage`, `GetImageGeneration` (long poll), `CancelImageGeneration`, `ListImageEdits`, and, for images made from a map, `GetMapImageReference` (the small reference drawing and the NPCs the GM can mark, before asking), `GenerateMapImage` and `UseGeneratedImageAsMapImage` ("Usar como imagem do mapa"). The generator sits behind a small interface (`maps/images/gen`: Gemini over `net/http`, and a fake). Tables: `image_requests`, plus `gallery_images.generated` and `parent_image_id`. See [Architecture](../architecture.md#ai-generated-images-mr-039-rn-28-adr-0019) and [images made from a map](../architecture.md#images-made-from-a-map-mr-039-rn-28).
- **Three ways, all in the MVP.** Scene art, the isometric view of a map and the textured map that matches the grid. The model returns one of 10 fixed proportions (1:1, 3:2, 2:3, 3:4, 4:3, 4:5, 5:4, 9:16, 16:9 and 21:9, from 512 px to 4K). So the server sends the map drawing (floor and walls) in the proportion closest to the grid, padded with rock, crops the map out of the result and fits it to the grid size. The layers stay the truth of the game: if the image drifts a little from a wall, the GM generates again. Each request takes up to 10 object images and 4 character images as references.
- **The players' view.** Scene art and the isometric view of a map start from what the players see now: the union of what their characters see (the fog computation, without memory), the creatures they see and the unrevealed secret door as a wall; on a map without fog, the whole map minus what is hidden. The textured map starts from the whole map (the secret door as a wall), because fog already hides it from each player; it is padded with rock up to the closest model proportion, then cropped back and fitted to the map image size. The room list goes only for the textured map of a generated dungeon.
- **NPCs.** Only NPCs the players see can appear in scene art and isometric views (the server refuses the others, and the portrait of a map NPC they do not see, also in `object_image_ids`), with the portrait as reference. The textured map takes no creature and no character image (the image becomes the map background; a creature painted on it is not a map creature), so "Quem aparece na imagem" disappears for that way, and the portrait of any NPC on the map is refused as an object image there (`object_image_ids`). No NPC is offered while a combat runs on the map (combatants are not map tokens).
- **Limits.** A map whose image exceeds 16 megapixels (4,000 x 4,000 px, any grid: 200 x 400 squares fit) cannot become a textured map; the screen knows before asking (`texture_too_large`). The month limit is per campaign (default 20; see [Operations](../operations.md#generated-images-the-gemini-api)).
- **Whole-map images.** An image that shows the whole map (the textured map and every adjustment of it) is flagged by the server (`GalleryImage.shows_whole_map`, column `generated_kind` of `gallery_images`, inherited down the chain). The screen never offers it to the players with one tap, and in the gallery and the session image picker it asks first ("Esta imagem mostra o mapa inteiro, também o que os jogadores ainda não descobriram.", RN-10). "Usar como imagem do mapa" accepts an adjustment of a textured map (made at the map's size, with the same checks as the original); the adjustment asks for the proportion closest to the map image and is cut in the middle, never stretched. Migration 00161 backfills a chain of any depth.
- **Names.** The image gets a name the players understand: the GM's (`name` in the requests) or, if empty, the map's name and the way (only for a map the players see), the way and the day ("Arte da cena · 06/10"), or the adjusted image's name plus " (ajuste)". Never "Imagem N", never the request text, never the name of a hidden map or point (RN-10: the player the image is shown to reads the name).
- **Idempotency.** The idempotency key changes only after an answer or a change in the form, and the app retries once, with the same key, a lost answer.
- **Screens.** The "Gerar imagem" dialog (a bottom sheet on the phone), opened from the map, a scene point and the gallery: the three ways (those that do not fit there are dashed with the reason), the drawing the server builds from what the players see, "Quem aparece na imagem" with only the NPCs they see, gallery references, what goes to Google under each field, and the month's count; the wait with a long `GetImageGeneration`, "Cancelar" and "Parar de esperar"; the result with "Mostrar aos jogadores" (the [MR-028](#mr-028-show-an-image-to-the-players) flow), "Pedir um ajuste" and the chain, and "Usar como imagem do mapa" asked in place; failures by typed detail. The session's image picker ("Mostrar uma imagem aos jogadores") marks "Mapa inteiro". See [Design](../design.md#ai-generated-images-mr-039-e10-07).
- **Tests (Go).** `TestMR039_GenerateAndEditAScene`, `TestRN10_AGeneratedImageIsHiddenUntilShown`, `TestMR039_TheMonthlyCap`, `TestMR039_FailuresGiveTheSlotBack`, `TestMR039_OnlyTheMasterGenerates`, `TestRN28_NothingPersonalGoesToTheModel`, `TestMR039_TheSameKeyGeneratesOnce`, `TestMR039_CancelBeforeAndAfterTheRequestLeaves`, `TestMR039_TheLastSlotGoesToOneRequest`, `TestMR039_TheSameKeyAtTheSameTime`, `TestMR039_TheLongPoll`, `TestMR039_APictureThatArrivesAfterTheExpiryIsStoredOnce`, `TestMR039_AnUnsentStaleRequestRefunds`, `TestMR039_Shutdown`, `TestMR039_AParentDeletedDuringAnEdit`, `TestMR039_ARefusedKeyIsNotARefusal`, `TestMR039_RequestsInFlightCountAgainstTheGallery`, `TestMR039_TheReferencesAreShrunkAndTheRequestIsCapped`, `TestMR039_GenerationOffWithoutAKey`, `TestMR039_TheRequestIsChecked`, `TestMR039_AFullGalleryRefuses`. For the map ways: `TestMR039_ThePlayersViewOfTheCave`, `TestMR039_ASecretDoorIsAWallInThePlayersView`, `TestMR039_AMapWithoutFogIsShownWhole`, `TestMR039_NoNPCIsOfferedWhileACombatRuns`, `TestMR039_NobodyOnTheMap`, `TestMR039_TheSceneArtAndTheIsometricViewOfAMap`, `TestMR039_OnlyNPCsThePlayersSeeCanAppear`, `TestMR039_TheTexturedMapIsPaddedAndCroppedBack`, `TestMR039_ADungeonsTexturedMapSendsItsRooms`, `TestMR039_UseTheTexturedMapAsTheMapImage`, `TestMR039_UseIsRefusedWhenTheMapChanged`, `TestMR039_UseIsRefusedWhenTheWallsChanged`, `TestMR039_UseAfterAndRacingRedraw`, `TestMR039_UseGivesAFogMapACopyOfAPictureUsedElsewhere`, `TestMR039_TheMapRPCsAreTheMastersOnly`, `TestRN10_NothingOfTheMapPicturesReachesAPlayer`, `TestMR039_TheCapAndTheLongPollForMapKinds`, `TestMR039_AFailedMapRequestGivesTheSlotBack`, `TestMR039_TheSameKeyMakesOneMapPicture`, `TestMR039_TheTexturedMapTakesNoCharacters`, `TestMR039_AHiddenNPCsPortraitStaysOut`, `TestMR039_ASightThatSeesNothingIsNobody`, `TestMR039_AMapTooLargeForATextureIsSaidInAdvance`, `TestRN10_AMapWithoutFogDrawsNoHiddenToken`, `TestRN10_TheGalleryTellsWhichImagesShowTheWholeMap`, `TestMR039_UseAnEditOfATexturedMap`, `TestMR039_TheImageHasAMeaningfulName`, `TestMR039_AHiddenNPCsPortraitIsRefusedAsAnObject`, `TestMigrationsBackfillAChainOfEditsOfAnyDepth`. Pure packages: `TestCropByFractionsOfTheDrawing` and `TestFitToMapNeverStretches`/`TestCenterCrop` (`refimg`, `images`), `TestCropFitRefusesAnAnswerThatBreaksTheMemoryBudget` (`images`), and in `gen` (no database) `TestBuildBody`, `TestParseAnswer`, `TestGemini*`, `TestFake`; the key is never printed: `TestLoadImages`, `TestGeminiErrorsNeverCarryTheKey`.
- **Tests (browser).** `e2e/tests/images.spec.ts` (`@MR-039 @RN-28 @RN-10`: generate the scene art of a map, adjust, show, and the player sees it only afterwards; the textured map becomes the map image and the layers stay; only the NPCs the players see; the month limit; generation off; the session picker marks the whole map), plus Vitest `imagegen-form`, `imagegen-errors`, `imagegen-run`, `image-generate-dialog` and `image-picker-dialog`.

#### Related
- Uses the Gemini API (the image model, "Nano Banana"), with a Google AI Studio key, from the server, behind a small interface (ADR-0019, which replaces ADR-0014, which used Vertex AI). The model is configurable: `gemini-3.1-flash-image` by default. The new processor, Google (Gemini API), is in [Privacy](../privacy.md#what-generated-images-send-to-google-mr-039-rn-28); the key is a secret, and cost and limit are in [Operations](../operations.md#generated-images-the-gemini-api).
- Open: the per-campaign monthly limit (proposal: 20) is settled for good after measuring the cost with the real key (US$ 0.067 per 1K image on `gemini-3.1-flash-image`).

### MR-040: Level up from the sheet

**As a** player, when my character can level up, **I want** to edit the sheet to add only what the next level gives, **so that** I do not wait for the GM to apply the level for me.

- Priority: MVP
- Rules: RN-01, RN-12
- Modules: characters, rules

#### Acceptance criteria
- **Given** a character that "Pode subir de nível" (RN-12), **when** the player opens the locked sheet, **then** they can edit it; without that mark the sheet stays read-only (RN-01).
- **Given** the guided edit, **when** the player uses it, **then** it adds one class level and only that level's choices, such as hit points, skills, spells and the ability score increase.
- **Given** the guided edit, **when** the player tries to change anything else on the sheet, **then** the server refuses: the rest stays locked.
- **Given** an edit the D&D 5e rules do not allow, **when** the player submits it, **then** the rules engine refuses it.
- **Given** the hit die of a level, **when** the player rolls in the app, **then** the server rolls and stores the result, and rolling again returns the same one; in a campaign that forces physical dice, the app does not roll and the player types the result (RN-18).
- **Given** the player has leveled up, **when** the GM opens the character list, **then** they are notified and see "O que mudou": the player's choices at that level (ability, hit points and how, cantrips, spells, prepared), with the time. There is no approval and no veto: the GM keeps editing the sheet as always (RN-02).
- **Given** the player confirmed the level up, **when** the sheet is saved, **then** "Pode subir de nível" clears by itself.

#### In the app
- **Server.** `rules` has `LevelUpOptions` (what the class's next level gives), `ApplyLevelUp` (the sheet the choices make) and `CheckLevelUp` (refuses everything the level does not allow, with the field and a reason). `CharacterService` has `GetLevelUpOptions`, `PreviewLevelUp` (the "Resumo": the derived sheet from afterwards, made by the server), `RollLevelUpHitPoints`, `LevelUpCharacter` and `ListLevelUps` (the GM's "O que mudou"). Tables: `character_level_ups` (the record) and `character_level_up_rolls` (the stored die). See [Architecture](../architecture.md#leveling-up-from-the-sheet-mr-040) and [RN-01](rules.md).
- **Scope.** The server covers hit points, the ability score increase, spells (cantrips, known or spellbook, and prepared), the subclass and the options of the level's features. What stays out (multiclass, own subclass, the Warlock's invocations after level 2) the GM does in the editor. The full level-up screen stays in [MR-017](#mr-017-level-up), after the MVP. The GM never vetoes a level up: they are notified, see what changed and fix the sheet if they want.
- **The sheet.** On the locked sheet of a character that "Pode subir de nível", only the owner sees the block "Pensantus pode subir de nível" (the reason, "O mestre marcou “Chegar ao Vale Seco”." or "Você chegou a 2.700 XP.", and the filled button "Subir para o nível 4"). The button opens `/campaigns/:id/characters/:id/level-up`, with the steps Habilidades, Vida, Magias and Resumo. A step with no choice does not exist (Toren at level 5 has only Vida and Resumo); "Escolhas", for the subclass, feature options, skills and expertise, comes before Magias at the levels that give them. Nothing is stored until "Confirmar o nível 4"; "Cancelar" and "Voltar para a ficha" ask in place before discarding.
- **Numbers and dice.** The "O que muda" numbers come from `PreviewLevelUp` (the browser does not compute rules). The hit die uses the same roll picker as combat and the scene (RN-18). A stale revision (`aborted`) or an engine refusal shows in place, with the reason. "Ler a ficha de novo" keeps only the choices that still fit what the server now asks. The table's hit points rule decides the card (a table that allows only the average has no die). A roll made in the app stays only when it is the one the server kept for this class and level, and a typed roll only for the same level and a die it fits. Prepared spells and expertise are cut back to the new counts, so the screen never sends a pick the level no longer allows. The summary reads the spellcasting of the class being levelled, not the first class of the sheet.
- **After confirming.** The sheet shows the new level and "Pensantus subiu para o nível 4. O mestre foi avisado.", and the tag disappears. The GM sees "Subiu para o nível N" and a notice at the top of the campaign (for 24 hours, or until dismissed; the open campaign with a session reads again on `xp_changed`), and "O que mudou" opens the choices in place, with the time. See [Design](../design.md#level-up-mr-040).
- **Tests (Go).** `TestMR040_ThePlayerLevelsUpALockedSheet`, `TestMR040_OnlyWhenTheCharacterCanLevelUp`, `TestMR040_TheRestStaysLocked`, `TestLevelUpRefusals`, `TestMR040_TheRulesRefuseWhatTheLevelDoesNotGive`, `TestLevelUpRefusesBadSpells`, `TestLevelUpPensantus` (Wizard 3 to 4), `TestMR040_TheHitPointRollIsKept`, `TestMR040_ATypedPhysicalDie`, `TestMR040_TheMasterSeesWhatChanged`, `TestMR040_CanLevelUpClearsAfterwards` (in `progression`, by XP and by milestones), `TestLevelUpSweep` (the 12 classes from 1 to 20, with every SRD subclass: options and check agree), `TestLevelUpOptionsForTheSubclassLevel`, `TestLevelUpGainsTheEngineModels`, `TestLevelUpRefusesDuplicates`, `TestMR040_OneRollPerLevelNotPerClass`, `TestMR040_DuplicatesAreInvalid`, `TestMR040_AStoredSheetThatFailsToday`, `TestMR040_ListLevelUpsPages`, `TestMR040_StaleRevision`, `TestMR040_ADeadCharacterDoesNotLevelUp`, `TestMR040_AFighterWithNothingToChoose` (Toren) and the new rows of `TestAuthorizationMatrix`.
- **Tests (browser).** `e2e/tests/levelup.spec.ts` (`@MR-040 @RN-01 @RN-12`: "Pensantus sobe do Mago 3 para o 4…", with the GM seeing it; "Toren sobe do Guerreiro 4 para o 5…" with only Vida and Resumo and a die rolled in the app; "quem não pode subir de nível…": no button, and the route answers like a locked sheet), plus Vitest for the steps, choices, Constitution, die (average, rolled, typed, the dice rule), numbers, errors, the GM's notice and "O que mudou", the Bard (college and Expertise), the cleric (prepares from the class list), subclasses that give a cantrip and skill, Magical Secrets, and the sheet block; the screens are in `a11y.spec.ts` (`scanLevelUpScreens`).

#### Related
- [MR-017](#mr-017-level-up), [RN-01](rules.md), [RN-12](rules.md).

### MR-041: Treasure and XP by gold

**As a** game master, **I want** to place treasure on the map before the session and convert what the group found into XP, **so that** I can award XP by gold without doing the math on the spot.

- Priority: MVP
- Rules: RN-09
- Modules: maps, progression

#### Acceptance criteria
- **Given** a map, **when** the GM places a treasure (a chest, for example) with a value in gp, **then** the treasure is stored on the map, with the value in gp.
- **Given** a treasure the group found, **when** the GM marks it as found, **then** it counts as found and not yet converted.
- **Given** found treasures, **when** the GM uses "Voltar à cidade", **then** the app converts them into XP, 1 XP per gp (RN-09), split as the GM chooses, and each treasure is converted only once.

#### In the app
- **Treasure point.** A `TREASURE` point stores the value in gp (an integer, 0 to 1,000,000) and the description, hidden like any point. The GM marks it found by one or more characters (`MarkTreasureFound`, and `UnmarkTreasureFound` to take the mark off): from then on everyone who sees the map sees it, with the description, the value and who found it. With a session open, the treasure keeps the session (its summary counts it), and `treasure_found` and `treasure_unfound` enter the history with only IDs and gp; outside a session it counts in no summary. A treasure converted by an award has its value, mark and kind locked, and it cannot be deleted before being unmarked (the server refuses both, `MapBlocked`).
- **Editor.** "Tesouro" in the point bar places the point and opens the panel: the name, the "Descrição para os jogadores" (what is inside, which they read when the treasure is found) and the "Valor em ouro" (whole gp, 0 to 1,000,000). Under "Encontrado", "Marcar como encontrado" opens in place the same choice as the session (who found it, nobody marked, at least one) and says under the button that **marked outside a session the treasure enters no summary** (with a session open: "Marcado durante a Sessão N, o tesouro entra no resumo dela."). Once found it shows by whom and when, and "Desmarcar" asks in place with focus on "Voltar". Once converted to XP the fields are locked and "Desmarcar" and "Apagar ponto" become the dashed button that does not act, with the reason written. The "Pontos do mapa" list separates a trap and a chest in the same square.
- **Session page.** The GM sees "Tesouros do mapa": a card per treasure (hidden, found or converted), with what is inside (the GM's alone until found). "Marcar como encontrado" opens the form on the card: "Quem encontrou", the line of what enters the summary (the total in gp, which the server splits; the screen does no math). Then "Encontrado por Brisa às 21:40"; "Desmarcar" asks in place; once converted, the card shows the lock and the way to undo. The player sees the chest on the map (filled mark), the line "Baú de moedas, Tesouro, encontrado por Brisa" with the value and contents, and the notice "Brisa encontrou o Baú de moedas.".
- **"Voltar à cidade" (server).** `ProgressionService.ListTreasuresToConvert(campaign_id)` (`IDEMPOTENT`, GM only) lists the found treasures no award has converted, oldest to newest, with the map, name, gp, who found it, when, and whether it was found with a session open. It returns at most 100 (the oldest) and the `total`. "Voltar à cidade" is **an `AwardXP` in `GOLD` mode** with `treasure_point_ids` in place of `gold`: the server adds up the treasures' gp (1 XP per gp), splits among the chosen characters as in every award (equal shares rounded down; the remainder is lost and comes in `lost_xp`) and, in the same transaction, links each treasure to the award, locking the rows. Two awards at once for the same treasure convert it once; the other gets `TREASURE_ALREADY_CONVERTED`. Whoever receives is whoever the GM sends in `character_ids` (the screen marks every living character), regardless of who found it: "found by" is only for the highlight. "Dar XP por ouro" with typed gp stays as before.
- **History and privacy.** The response, the history (`XPAward.treasures`, GM only: the ID and gp of each treasure, which stay on the award even after it is undone; the player gets only `treasure_count` and the `gold`, RN-10) and the `xp_awarded` event (only IDs and gp) carry the converted treasures ("Voltar à cidade: 3 tesouros, 420 PO").
- **Refusals.** A treasure of another campaign or that is not a treasure (a scene point, for example): `not_found`. Not found yet: `TREASURE_NOT_FOUND_YET`. Already converted: `TREASURE_ALREADY_CONVERTED` (the detail carries the ID). More than 1,000,000 gp in total: `TREASURES_OVER_LIMIT`. No gp at all: `NOTHING_TO_GIVE`. `treasure_point_ids` together with `gold`, outside `GOLD` mode, repeated or with more than 100: `invalid_argument`. A campaign by enemies or by milestones: `MODE_NOT_ALLOWED` (its treasure stays on the map and in the summary, with no conversion).
- **Undo.** `UndoLastXPAward` undoes the conversion if it is the last award, and the treasures return to "found, not converted" in the same transaction (they can be converted again). After a newer award the conversion is no longer the last, and the treasures stay converted until that award is undone (an undo with the conversion's `expected_award_id` answers `aborted`): to correct earlier, correct by hand.
- **Campaign page, gold campaign.** The GM's "Experiência" panel gains the strip "Encontrado, ainda não convertido" ("3 tesouros · 420 PO", and a line per treasure: "Baú de moedas, 250 PO, de Brisa"; with none: "Nenhum tesouro esperando. Os que o grupo encontrar aparecem aqui.") and "Voltar à cidade" next to "Dar XP", both outlined at the same size (stacked, full width, on the phone). The strip lists at most 5 treasures and counts the rest ("e mais 2"). "Dar XP", in a gold campaign, opens with the same strip and the "Voltar à cidade" button, and "ou digite o ouro" separates the two ways; pressing "Voltar à cidade" there closes "Dar XP" and opens the conversion (one sheet at a time on the phone).
- **The dialog.** "Voltar à cidade" opens a 600 px dialog (bottom sheet on the phone): "Tesouros para converter" (all marked, each with the name, "Encontrado por Brisa às 21:40" and the gp; one found outside a session carries the tag "fora de uma sessão", and one line above the list says it counts in no session summary), "Quem recebe" (all the living marked) and the live written math: "420 PO em 3 tesouros = 420 XP", "420 XP ÷ 4 = 105 XP para cada" and "Sobra 0 XP." (or "Sobra 1 XP, que não vai para ninguém."). The footer, which does not scroll, has the math, "Para 3: Pensantus, Toren e Brisa" and the filled button that says the number ("Dar 105 XP para cada"). While the list is not read the dialog says "Lendo os tesouros encontrados..."; "Nenhum tesouro" only after a successful read (otherwise "Tentar de novo"). With no treasure or no character marked: "Marque pelo menos um tesouro e um personagem." and a dashed button. The screen's math is only the preview; the server's answer (`xp_each`, `lost_xp`) is the truth. With more than 100 treasures waiting: "Há mais N tesouros encontrados, que ficam para a próxima vez".
- **Errors and aftermath.** Errors stay in the dialog by the `XPBlocked` reason (`TREASURE_ALREADY_CONVERTED`, `TREASURE_NOT_FOUND_YET`, `TREASURES_OVER_LIMIT`, `MODE_NOT_ALLOWED`); the list is read again, what left disappears, the rest of the choice stays. After converting: "Voltar à cidade: Pensantus, Toren e Brisa receberam 140 XP cada. Os 3 tesouros foram convertidos.". The award's reason is "Voltar à cidade"; the history line is written from the treasures ("Voltar à cidade · 420 PO em 3 tesouros"; `treasure_count` and `gold`, the same for the player, who never sees which). "Desfazer", only on the last award, asks in place ("Os 3 tesouros (420 PO) voltam a “encontrado, não convertido”."); a "Voltar à cidade" that is no longer the last says, to the GM only, that the treasures are freed once the newer awards are undone. In a campaign by enemies there is no button: the strip exists only when a treasure was found, is called "Tesouro encontrado" and says "Esta campanha dá XP por inimigos, então o tesouro não vira XP. Ele aparece no resumo de cada sessão."; in a campaign by milestones there is no strip. The player sees only the history (the line, each one's XP), with no strip and no buttons.
- **Tests (Go).** `TestMR041_TreasureFound`, `TestMR035_PointKindsAreValidated` (the value), `TestRN10_PlayersNeverReceiveTrapsLightsOrHiddenTreasure`, `TestMR041_VoltarACidadeConvertsTreasuresIntoOneGoldAward` (420 gp in 3 treasures, 140 XP for each of 3, the history, the event, the repeated key), `TestMR041_ATreasureIsConvertedOnce` (a transaction holds the locked rows while two awards start: neither finishes before it lets go, then one wins), `TestMR041_ADoubleSubmitConvertsOnce`, `TestMR041_TheListStopsAtWhatOneConversionTakes`, `TestMR041_UndoFreesTheTreasures`, `TestMR041_VoltarACidadeRefusesWhatDoesNotFit`, `TestMR041_OnlyTheMasterReadsTheTreasuresToConvert`, and the `ListTreasuresToConvert` row of `TestAuthorizationMatrix`.
- **Tests (browser).** Vitest `town-sheet`, `treasure-strip`, `treasure`, `xp-errors`, `award-history`, `experience-panel`, `experience-store`, `xp-give-button`, `award-xp-sheet`, `treasure-card`, `treasure-point-panel`, `treasure-draft`. Playwright `gold.spec.ts` (`@MR-041 @MR-032`: three treasures become one award of 420 XP for Pensantus, the history line, the undo, what the player reads, the treasure converted in a race, the "Dar XP" of the page and of the live session, a campaign by enemies with the server's `MODE_NOT_ALLOWED` refusal, and the summary; splitting among several with a remainder is covered by Vitest `town-sheet` and `treasure`, which check the preview against the server's `TestSplitXP` numbers, because the suite has a single player (RN-03)), `traps.spec.ts` (`@MR-041`), `map-editor.spec.ts` (`@MR-041`: treasure outside a session, who found it, the player sees it only when found, "Desmarcar" and delete), and the screens in `a11y.spec.ts`.

#### Related
- The "Mais tesouro encontrado" highlight is in [MR-032](#mr-032-combat-highlights). The GM can also type the gp in "Dar XP por ouro" ([MR-016](#mr-016-award-xp)), which stays as an alternative. Traps and chests: [MR-035](#mr-035-traps). Gold only converts in a gold campaign; in the others the treasure counts in the session summary. See [Architecture](../architecture.md#layers-fog-and-point-kinds).

### MR-042: Bestiary

**As a** game master, **I want** to search the SRD creatures and put one in the combat, **so that** I can build an encounter without making a sheet for every enemy.

- Priority: MVP
- Rules: RN-20, RN-29
- Modules: rules, play, characters

#### Acceptance criteria
- **Given** I am the GM of "Mirathel", **when** I open the "Bestiário", **then** I see the 334 SRD creatures with a Portuguese name, and I filter by name, type, size and CR (ND).
- **Given** a creature of the bestiary, **when** I open it, **then** I see its sheet (AC, HP, speed, abilities, attacks and actions), with the SRD text in English.
- **Given** a combat being prepared, **when** I put three Goblins from the bestiary, **then** they enter as hidden NPCs, each with its own initiative (RN-19), average HP, or rolled if I ask, **and** the player sees only the word for their state (RN-20, RN-29).
- **Given** a combat with bestiary monsters, **when** it ends, **then** XP by enemies counts each one's CR, like an NPC's.
- **Given** a creature of the bestiary, **when** the GM chooses "Criar NPC", **then** the app makes a basic NPC sheet with its numbers, which the GM can rename and edit.

#### In the app
- **Reads.** `ContentService.ListCreatures` filters by **size** (`size`, the six of the SRD), by **minimum CR** (`min_cr`, next to `max_cr`; the minimum cannot exceed the maximum), by type, CR and name. Each row also brings AC and average HP ("Lobo · Wolf · SRD", "CA 13 · PV 11"). The search finds the Portuguese or the English name: "lobo" finds the Giant Wolf Spider by its Portuguese name, and "wolf" also finds the Werewolf's forms. `page_size` goes up to 400, so the bestiary brings all 334 at once. The player can read the bestiary (it is SRD rules) but the app shows it only to the GM. Portuguese type and size labels come from the server (`type_pt`, `size_pt`).
- **"Criar NPC".** `CharacterService.CreateNpcFromCreature` makes an NPC from a creature: GM only, kind **Minion (the default) or story NPC** (MINION and STORY: the basic sheet does not fit "Inimigo" and Boss, which have full sheets; the app offers only these two), with the GM's name and a basic sheet with AC, average HP, speed, abilities, initiative (the Dexterity modifier), size, CR, XP and up to three of the creature's attacks (weapon or spell, with the first damage; the other damage parts, like the fire on the dragon's bite, go in the sheet's description as "Mordida: +2d6 fogo."; saving throws and effects stay on the SRD sheet), with Portuguese names, and the `monster_key` that links to the SRD sheet. It is idempotent by the dialog's key. The NPC is a copy: the bestiary does not change and the NPC is edited like any other. Like every NPC, the player cannot list or read it (RN-04), and its token is born hidden (RN-10). NPCs made by "Criar NPC" follow the creature's multiattack.
- **"Pôr no combate".** `CombatService.AddMonsters` puts 1 to 10 monsters of a creature in a combat being prepared or running: the names ("Bandido 1" to "Bandido 3", or the GM's base name; with Bandidos already in the combat the server numbers on: "Bandido 4, Bandido 5 e Bandido 6"), average HP (the default) or rolled by the server, one per monster, hidden by default (the request may reveal them), and initiative rolled by the server for each, with the creature's Dexterity modifier. Each monster is a combat NPC linked to a hidden campaign NPC the app makes by itself: **one NPC per campaign and creature**, reused, never in the GM's list nor as a participant. On the map it starts without a square; in theatre of the mind no one has one. The player reads only the word for a revealed monster's state and nothing of the hidden ones; attacks go through `RollAttack` (the critical follows the table's rule); XP by enemies counts each one's CR. The idempotency key covers the whole request. The creature, the CR and the rolled HP are the GM's alone (a line of the log, also before the combat); the player only knows the name the GM gave.
- **Multiattack.** Multiattack follows SRD 5.1 (the snapshot's wrong numbers are corrected: Veteran 3, Shambling Mound ("Montículo Movediço") 2, Brown Bear 2, Gibbering Mouther ("Mandíbulas Tagarelas") 1); the app stores N attacks, any from the sheet, and the GM plays the mix.
- **Bestiary screen.** The "Bestiário" panel on the campaign page (GM only) leads to `/campaigns/:id/bestiary`: the 334 creatures with the Portuguese name and the SRD name in small type, type and size, CR and "CA 13 · PV 11", the search (both names; the hint under the list does not depend on the word), the type, size and CR filters (ranges or one level), the count ("5 de 334 criaturas"), the empty state ("Nenhuma criatura com “…”.") and the loading and error states. A row opens the sheet (`/campaigns/:id/bestiary/:criatura`) with the SRD text in English and the SRD credited. The creature icon follows the type (person, big figure, wolf, spider, paw, neutral glyph). A player has no entry point (RN-04); the page, if opened by address, says it is the GM's.
- **"Criar NPC" dialog.** The GM's name for the NPC, **Minion** or **NPC de história**, the numbers and attacks that go along (the server says them, `Creature.npc_attack_names`, by the same function that builds the sheet), and the confirmation ("NPC criado: Capitão bandido", the attacks the sheet received, the way to it). The creation key is born per dialog and covers all its attempts: a double tap makes one NPC and a retry after a lost answer says "Já foi criado." (the refusal of `idempotency_key` names the field in the `InvalidField` detail).
- **"Pôr no combate" sheet.** On each bestiary row and on the creature's page (the filled button of the "Usar esta criatura" panel; "Criar NPC" is outlined beside it). The sheet shows the combat the monsters enter ("Emboscada na ponte · em preparação" or "em andamento"; with no open combat the button becomes "Criar o combate e pôr", which starts one with the monsters and the whole group; with no session, it waits), "Quantos" (1 to 10, never more than fits in the 40 combatants), the preview "Entram como Bandido 1, Bandido 2 e Bandido 3.", the base name, HP ("Média (11)" or "Rolar") and "Escondidos no início" (on by default). The idempotency key is born per opened sheet and repeats on a retry with the same parameters; a changed choice is another key. What entered is announced over the list or on the sheet, with the way to the session. The 40 limit has a typed detail (`EncounterBlocked`, `TOO_MANY_COMBATANTS`) said in words.
- **In the session.** The GM's order shows the monster's CR ("NPC · ND 1/8 · CA 12"), the turn card carries "Ficha da criatura" (the `bestiary_creature_key` reaches only the GM), the log has the line "Você pôs 3 monstros no combate. Bandido 1: 9 PV (2d8 + 2: 3, 4); …" for the GM only, in a group "Antes do combate" when it came before the start, and the end-of-combat XP lists "Bandido 1 a 3 · ND 1/8 · 25 XP cada · 75 XP" before the split. See [Design](../design.md#monsters-in-combat-and-the-encounter-builder-mr-042-mr-043).
- **Tests (Go).** `TestListCreaturesBestiary` (`rules`); `TestListCreaturesBestiaryRequests`, `TestCreateNpcFromCreature`, `TestNpcSheetFromCreature`, `TestEveryCreatureMakesAnNpc` (`characters`); `TestMR042_ThreeBanditsJoinTheCombat`, `TestMR042_MonstersRollTheirHitPoints`, `TestMR042_AddMonstersRefusals`, `TestMR042_AMonstersCriticalFollowsTheTablesRule`, `TestRN20_PlayersNeverReceiveAMonstersNumbers`, `TestMR042_MonstersGiveXPByTheirChallengeRating`, `TestMR042_MonstersInTheatreHaveNoSquare`; the method rows in `TestAuthorizationMatrix`.
- **Tests (browser).** Vitest `bestiary-list`, `bestiary-creature`, `create-npc-sheet`, `bestiary-format`, `put-sheet`, `monsters` (`core/combat`), `combat-log`, `order-list`, `combat-xp`, and the panel in `campaign-detail`. Playwright `bestiary.spec.ts` (`@MR-042 @RN-04 @RN-10`) and `monsters.spec.ts` (`@MR-042 @RN-29 @RN-20 @RN-10`: three Bandidos at once, the CR and the GM's log, what the player receives, rolled and revealed, XP 3 x 25 = 75; the split by four, 18 each with 3 left, is Vitest's because the suite has one player), and the screens in `a11y.spec.ts`.

#### Related
- [MR-043](#mr-043-generate-encounters), [MR-044](#mr-044-generate-treasure), [RN-20](rules.md), [RN-29](rules.md). Custom monsters are not part of the MVP. The HP when putting in combat are the average, and the GM can roll.

### MR-043: Generate encounters

**As a** game master, **I want** to build an encounter knowing whether it is easy or deadly for my group, and generate one when I am out of ideas, **so that** I prepare the session faster.

- Priority: MVP
- Rules: RN-29
- Modules: rules, play, maps

#### Acceptance criteria
- **Given** the group of "Mirathel" (the campaign's living player characters, plus the NPCs I put in the group, at the level I say), **when** I build an encounter with bestiary creatures and quantities, **then** I see the total XP and the difficulty (low, moderate or high) against the group's budget, with the label "Guia de dificuldade do SRD 5.2.1 (regras de 2024)" and the warning that, with the 2014 monsters, the encounter tends to be a little easier.
- **Given** a built encounter, **when** I store it in a battle point on the map, **then** "Começar este combate" puts its monsters in the combat, as in [MR-042](#mr-042-bestiary).
- **Given** a difficulty and, if I want, a creature type, **when** I ask "Gerar encontro", **then** the app builds one with SRD creatures, a leader and a group, that never exceeds the budget nor brings a creature with CR above the group's lowest level plus 3 (the group's NPCs count, at the level the GM gave); "Gerar outro" makes a new one, and I can swap a creature.
- **Given** the same difficulty, the same options and the same seed, **when** I generate twice, **then** the result is the same.

#### In the app
- **Who is the group.** The campaign's living player characters, plus the NPCs the GM puts in the group at that moment of the story. Never a dead character or a pending member.
- **Service.** `EncounterService` (`proto/meurpg/play/v1/encounters.proto`, `play/encounters.go`), the GM's only (a player, a pending member and an outsider get `not_found`: the builder and the stored encounter are the GM's secret, RN-10). `EvaluateEncounter` measures creatures and quantities against the group (the living player characters, plus the `extra_party` with the level the GM gives) and returns the low, moderate and high budgets, the total XP, the band, how much it exceeds high, the maximum CR (the group's lowest level plus 3) and the warnings (above high, creature above the maximum CR, empty group, too many people for one combat). `GenerateEncounter` builds a leader and a group of one or two creatures from the SRD for the difficulty and type asked, and is the same for the same seed (without a seed the server draws one and returns it: "Gerar outro"). `ListEncounterSwaps` lists creatures of the same XP (and, with a type, of the same type) for "Trocar criatura". `SaveBattleEncounter`, `GetBattleEncounter`, `ClearBattleEncounter` and `ListBattleEncounters` store, read, remove and list a battle point's encounter (`battle_encounters`, migration `00160`); storing the same again changes nothing.
- **"Começar este combate".** `StartEncounter` has `monsters`, `monster_hit_points` and `monsters_hidden` (the app reads the point's encounter, lets the GM change it and sends it); the monsters enter in the same transaction as the start, by the same code as `AddMonsters`, with the same idempotency key, also in theatre of the mind. See [Architecture](../architecture.md#encounter-builder-mr-043).
- **Budget table.** `effects/encounter_budget.json` (content revision `fx.16`), the "XP Budget per Character" table of SRD 5.2.1 (CC BY 4.0), credited in `NOTICE` and on the "Créditos" page; SRD 5.1 has no difficulty table. Whether it fits the 2014 monsters is for the table to see after playing.
- **The page.** `campaigns/:id/encounters` (the "Encontros" panel of the campaign page, GM only; a player reads "Só o mestre monta encontros."). The group in chips (living player characters and the NPCs the GM puts in, with "Pôr um NPC no grupo": a campaign NPC or just a name, and the level from 1 to 20, with the budget that makes it requested from the server), the bar of the three bands with the total and the band in words ("Baixa", "Moderada", "Alta" and "Acima de alta", never "mortal"), the label "Guia de dificuldade do SRD 5.2.1 (regras de 2024)" with the Credits link and the 2014-monsters caveat, the maximum CR line and the warnings (above high, creature above the ceiling, empty group, too much for one combat, a creature the SRD lost). Creatures and quantities (1 to 40; the "−" at 1 removes the creature) with the search "Adicionar criatura" by Portuguese or SRD name.
- **The math is all the server's.** At each change `EvaluateEncounter` measures again (250 ms wait, one calculation at a time, an old answer discarded).
- **Generate, swap, store.** "Gerar encontro" (difficulty and type; the result comes with the seed in small type; "Gerar outro" draws another; the same seed gives the same encounter; "Usar este encontro" leads to the builder), "Trocar criatura" (the same XP, without the creatures the encounter already has; the quantity stays) and "Guardar no ponto de batalha" (the map, the point, "Já guarda um encontro", and the question in place before replacing). The opened builder from a point (`?point=`) brings its stored encounter into the draft, and "Tirar o encontro do ponto" (question in place, `ClearBattleEncounter`) deletes it.
- **In the session.** The current map's battle point that holds an encounter shows today's band and "Começar este combate", which opens "Iniciar combate" already filled (the point's name, the monsters in reading rows, Média or Rolar, and hidden); on confirming, `StartEncounter` goes with the monsters, the HP mode, `hidden`, the point (`map_point_id`) and one key. The "Com mapa / Sem mapa (teatro da mente)" choice of "Iniciar combate" applies here: `CombatClient.start` carries `mode` in the extras and, in theatre, the monsters enter without a square and the combat has no point. **From a battle point on a map with a grid the dialog opens on "Com mapa"**, and the table's "combat with map" rule does not turn it into "Sem mapa" (the GM can still choose). The confirmation of "Pôr no combate" says the names the server gave. The point card shows the "demais para um combate" warning the server already sent.
- **Tests (Go).** `TestMR043_TheBuilderMeasuresAnEncounterAgainstTheParty` (the design's numbers: 1,550 XP Moderate, with Orin 1,400 / 2,100 / 3,000, 2,900 above high by 300), `TestMR043_TheBuilderRefusals`, `TestMR043_GenerateIsDeterministicAndKeepsItsPromises`, `TestMR043_GenerateRefusals`, `TestMR043_SwapOptionsKeepTheTotal`, `TestMR043_SaveReadAndClearOnABattlePoint`, `TestMR043_BeginningThisCombatPutsTheSavedMonstersIn`, `TestMR043_BeginningThisCombatWorksInTheatreAndWithRolledHitPoints`, `TestMR043_StartingWithMonstersRefusals`, `TestMR043_AStartRetryIsCheckedAgainstTheRequest`, `TestMR043_AStartWithMonstersAndNpcParticipants`, `TestMR043_TheFortyCountsThePartysCreatures`, `TestMR043_AGeneratedEncounterNeverFillsTheCombat`, `TestMR043_ASavedEncounterWithAGoneCreature`, `TestMR043_TheSavedEncounterFollowsItsPoint`, `TestMR043_SavesAtOnceOnOnePoint`, `TestRN10_PlayersNeverGetTheBuilderOrTheSavedEncounter`, `TestMR043_TheBuilderAuthorizationMatrix`; the rules ones, with no database, in `rules/encounter` and `rules/encounters_test.go`.
- **Tests (browser).** Vitest `encounter-builder`, `budget-bar`, `generate-sheet`, `party-npc-sheet`, `save-sheet`, `battle-encounters`, `start-combat-saved`, `encounter-text`, `encounter-draft`. Playwright `monsters.spec.ts` (`@MR-043 @RN-29 @RN-10`: the builder against the group, the NPC in the group, the same seed twice, "Trocar", store and replace with the question, "Começar este combate" and what the player never receives) and the screens in `a11y.spec.ts`.

#### Related
- [MR-042](#mr-042-bestiary), [RN-29](rules.md). The 40-combatant ceiling is a typed server detail (`EncounterBlocked`, `TOO_MANY_COMBATANTS`, in `AddMonsters` and `StartEncounter`, counting also the group's creatures).

### MR-044: Generate treasure

**As a** game master, **I want** to generate the treasure of a creature or a lair, with coins, gems, art objects and magic items, **so that** I can put it on the map without inventing everything on the spot.

- Priority: MVP
- Rules: RN-09, RN-10
- Modules: rules, maps, progression, characters

#### Acceptance criteria
- **Given** the group level, **when** I ask for an "individual" or "lair" treasure, **then** the app generates the coins, gems and art objects (our own tables, in Portuguese) and the SRD 5.1 magic items, with the Portuguese name and the value in gp.
- **Given** a generated magic item, **when** I open it, **then** I see the rarity, the value with the label "valores do SRD 5.2.1 (regras de 2024)" and the SRD description in English.
- **Given** a generated treasure, **when** I put it on a map, **then** it becomes a hidden treasure point (RN-10) with the gold (coins, gems and art) in gp and the items in the description, which in a gold campaign becomes XP at "Voltar à cidade" ([MR-041](#mr-041-treasure-and-xp-by-gold)).
- **Given** the same request and the same seed, **when** I generate twice, **then** the result is the same.

#### In the app
- **Magic items.** The 362 SRD 5.1 magic items are in the rules content (`srd51/data/magic-items.json`, Portuguese names `item:<index>`, content revision fx.12), with `Content.MagicItems()`, `MagicItem(key)` and `MagicItemUnits(rarity)` (what a treasure draws: each loose item and one unit per family and rarity). See [Architecture](../architecture.md#magic-items).
- **Service.** `TreasureService` (module `maps`, GM only; a player, a pending member and a non-member get `not_found` for everything) generates the individual or lair treasure for a group level (by default the lowest level among the living characters) and a seed: the same request and seed give the same treasure. Coins (CP, SP, EP, GP and PP; 10 SP = 1 GP), gems and art objects come from our own tables, in Portuguese, made by our own criteria (gold targets tied to the SRD 5.2.1 values, two value scales of ours and names of ours), in four level bands (1 to 4, 5 to 10, 11 to 16 and 17 to 20). Magic items are the 362 of SRD 5.1, with a Portuguese name, and **an artifact never comes out**. The `Treasure` carries the `content_version`, and `PlaceTreasure` requires it: a seed only gives the same treasure within one content version. See [Architecture](../architecture.md#treasure-generator-mr-044).
- **Values.** Each item's value is the SRD 5.2.1 table ("Magic Item Rarities and Values", CC BY 4.0, p. 205, credited): common 100 gp, uncommon 400, rare 4,000, very rare 40,000, legendary 200,000, labelled "Valores do SRD 5.2.1 (regras de 2024)". **A consumable is worth half and a spell Scroll is worth the rarity's whole value** (the SRD 5.2.1 note does not halve the scroll, and ours are the 2014 ones, with SRD 5.1's rarity by spell level; the screen says so). A "Varia" (varies) family comes out as one variant, with its value. The artifact has no price and never comes out. SRD 5.1 has no values and no random treasure tables, so the tables of coins, gems and art are ours. `GetMagicItem` gives what "Ver descrição" shows (rarity, value, attunement and the SRD text in English).
- **"Pôr no mapa".** `PlaceTreasure` generates again on the server, from the mode, level and seed (never from a total from the app), and creates a **hidden** treasure point (RN-10) in the middle of a square, with the **gold** (coins, gems and art) as the value and everything in the treasure, in Portuguese, in the description; **magic items never become XP**. The call carries an idempotency key, which holds in the campaign and is stored with the request hash (`map_points.create_key` and `create_hash`, migrations `00163` and `00164`): the same key with another request is refused. In a gold campaign the point becomes XP at "Voltar à cidade" like any treasure (MR-041); the screen reads the campaign's XP mode from `GetCampaign`.
- **The page.** `/campaigns/:id/treasure` (`web/src/app/pages/treasure/`), GM only (a player reads "Só o mestre gera o tesouro da campanha."), with the entry "Gerar tesouro" in the "Mapas" panel of the campaign page. "Gerar tesouro" has the kind (Individual or De covil), the group level (a 1 to 20 counter, the `app-number-stepper`, which starts at the lowest living level from `GetTreasureParty` and can be changed; if reading the group fails the page says so, with "tente de novo") and the seed, which shows after generating; "Gerar outro" asks for another without a seed.
- **The result.** It draws what the server returned: coins with each stack's value in gp (and "10 PP valem 1 PO"), gems and art with values, magic items with the Portuguese name and the SRD's English one, the rarity, the value or "sem preço" and attunement, identical ones together ("2 ×"), the gold that becomes the point's (coins, gems and art) and the items' value apart, said as what does not become XP, and the line of what the campaign does with the gold (by enemies and by milestones, which does not become XP; by gold, which converts at "Voltar à cidade", MR-041). "Ver descrição" opens the item (rarity, value with "Valores do SRD 5.2.1 (regras de 2024)" and the "Créditos" link, half or the scroll's rule in words, the SRD text in English with `lang="en"`). The "Créditos" page names p. 205 (the item values) beside p. 201.
- **"Pôr no mapa" on the page.** Chooses the map and the square (on the computer, on the GM's map drawing, with arrow keys too; on the phone, the room of a generated dungeon, or the middle of the map; on the computer the dungeon's rooms are also radio buttons beside the map) and calls `PlaceTreasure` with the mode, level, seed, `content_version` and a key per request. The point is born hidden and the confirmation says so. After placing, "Abrir o mapa" is the main button and "Pôr no mapa" disappears for that treasure (a second click would make a second point and, in a gold campaign, double the XP); only "Gerar outro" brings the button back. Refusals have their own sentences: changed tables ("Gere de novo", with the button), map without a grid ("Escolha um mapa com grade"), square outside the grid and the limit of 200 points.
- **Tests (Go).** `TestMR044_GenerateAsTheMaster`, `TestMR044_PartyLevelComesFromTheLivingCharacters`, `TestTreasureAuthorizationMatrix`, `TestMR044_GetMagicItem`, `TestMR044_PlaceTreasureMakesAHiddenTreasurePoint`, `TestMR044_PlaceTreasureRefusals`, `TestMR044_PlaceTreasureIsIdempotent`, `TestRN10_APlayerNeverSeesAPlacedTreasureUntilRevealed` (reads what the player receives, in JSON and in the stream), `TestMR044_AGoldCampaignConvertsTheGoldOnly`; in `rules`, `TestTreasureIsDeterministic`, `TestTreasureInvariants`, `TestMagicItemValues`, `TestLoadTreasureRefuses` and `FuzzGenerateTreasure`.
- **Tests (browser).** `@MR-044 @MR-041 @RN-10` in `e2e/tests/treasure.spec.ts`.

#### Related
- [MR-041](#mr-041-treasure-and-xp-by-gold), [MR-042](#mr-042-bestiary), [MR-043](#mr-043-generate-encounters). Custom magic items are not part of the MVP.

### MR-045: Look up spells

**As a** player, **I want** to look up all the table's spells in the app, **so that** I can read what each one does before choosing and during the session.

- Priority: MVP
- Rules: RN-23
- Modules: characters, rules

#### Acceptance criteria
- **Given** I am a player of "Mirathel", **when** I open "Magias", **then** I see all the spells available at the table (the SRD's and the table's that the GM left on), with search by name and filters by class, spell level and school, **and** each one opens its full description.
- **Given** a spell the GM turned off in "Opções para os jogadores", **when** I look for it, **then** it does not appear.
- **Given** my sheet, **when** I choose spells, **then** I can only choose those on my class's list; on a multiclass sheet, those of each class, by the level in it.

#### In the app
- **Server.** `ContentService.ListSpells` lists the SRD's spells and the table's together, in Portuguese-name order: search by Portuguese or English name (accent- and case-insensitive), filters by class (including the list of a table class and the one-third caster's), level and school, and "only the ones I can learn" (the list of each sheet class up to the level it casts; in multiclass, each by its level). Only an active member reads (a pending one gets `not_found`), a player never receives an archived table spell, and the full description, with the target ("Só quem conjura", "Cone de 4,5 m"), is `GetSpellDetails`.
- **What the GM turns off.** A spell the GM turned off in "Opções para os jogadores" (an SRD one or the table's) does not appear in `ListSpells` nor in `GetSpellDetails` for a player (except, in the details, for whoever has the spell on their sheet), and the turned-off class disappears from `class_keys`; the GM reads them with the `off` mark. See [MR-025](#mr-025-register-table-content).
- **Live reload.** With a session open, when the GM turns a spell on or off (or writes a table spell), the list, the open spell and the class names update without reloading: the one turned off leaves the list and the card says "Esta magia não está disponível." (`content_changed`).
- **The "Magias" screen.** `/campaigns/:id/spells` (`web/src/app/pages/spells/`), for every active member, the GM too; the "Magias" panel of the campaign page leads to it. The app only asks and draws: the search, the filters (class, spell level, school), "Só as que posso aprender" (with the player's own character; the GM does not have the switch), the total ("73 magias") and the pages (`page_token`, "Mostrar mais") are `ListSpells`'s.
- **Layout.** At 1100 px or more there are three columns: the filters, the list and the description beside it. Below that the page is one column of up to 680 px, with the search and "Filtros (n)" (a sheet on the phone, a dialog from tablet up; from 768 px the two share a strip) and chips of what is on; the spell opens in place of the list, as a history step: "Voltar para Magias" and the browser's Back close the spell and bring back the same list, at the same height, with the same search and focus on the row that was open.
- **The description.** The shared piece `shared/spell-details` (`SpellBody`), with the "Alvo" line from `target.label_pt`. A table spell carries the tag "Da mesa", the fields "Ataque" and "Dano" and the GM's text in Portuguese, without the SRD mark; an SRD one carries the English text and "Créditos". An archived spell reaches only the GM, with "Arquivada".
- **States and link.** Loading (grey rows of the same height, no jump), empty ("Nenhuma magia com “zzz”." and "Limpar a busca"), error with "Tentar de novo", and basic sheet with the filter (a sentence, not an error, with "Ler todas as magias"). The filters (written when a search starts) and the open spell stay in the link (`?q=`, `classe`, `circulo`, `escola`, `minhas`, `magia`), and the page re-reads the link when history brings another. Cached answers (descriptions and class names) are valid by the table's content version: a GM edit shows without reloading. A spell that does not exist, or that the GM archived for the player, has its own sentence ("Esta magia não está disponível."), without "Tentar de novo".
- **Tests (Go).** `TestListSpellsSearch`, `TestListSpellsFilters`, `TestListSpellsLearnable`, `TestListSpellsHidden` (`rules`), `TestMR045_TheSpellsPage`, `TestMR045_OnlyTheSpellsACharacterCanLearn` and the method row in `TestAuthorizationMatrix` (`characters`).
- **Tests (browser).** Vitest (`spells-filter`, `spells-state`, `spells`, `spell-rows`, `spell-details-map`); Playwright `@MR-045` (`e2e/tests/spells.spec.ts`: the search "maos" and the "Cone de 4,5 m"; "Só as que posso aprender" of a level 1 Wizard with only cantrips and 1st-level spells; the archived spell out of the player's reach, on the screen and in the JSON, and with "Arquivada" for the GM), and the screens in `a11y.spec.ts`.

#### Related
- A part of [MR-020](#mr-020-look-up-the-rulebook), with only the spells. Letting the GM accept a spell outside the class list is for after the MVP ([MR-046](#mr-046-table-style-feature-by-feature)). Table content: [MR-025](#mr-025-register-table-content).

## Priority: MVP (prerequisite)

### MR-002: Generate an invite

**As a** master, **I want** to generate an invite link, **so that** players can join the campaign.

- Priority: MVP (prerequisite)
- Rules: RN-07
- Modules: campaigns

#### Acceptance criteria
- **Given** I am the master of "Mirathel", **when** I generate an invite, **then** I get a link valid for one person for 7 days **and** the server stores only the hash of the token.
- **Given** an invite that has not been used, **when** the master revokes it, **then** the link stops working **and** whoever already joined stays in the campaign.
- **Given** I am a player in "Mirathel", **when** I try to generate, list or revoke invites, **then** the server refuses.

The master can choose the number of uses and the validity of the invite; the implemented behaviour is RN-07.

#### In the app
- The master picks 1 to 20 uses (default 1) and a validity from 5 minutes to 30 days (default 7 days), and can revoke the invite.
- The "Convites" section of `/campaigns/:id` (`web/src/app/pages/campaign-detail/invites/`) is for the master only. It creates an invite (uses, and validity with the presets 1/7/30 days), shows the link once with a warning and a copy button, lists each invite on a row (uses, validity, whether it requires approval, and the state: Ativo, Usado, Expirado or Revogado) and revokes.
- Go tests: `TestMR002_MasterGetsASingleUseSevenDayInviteStoredAsAHash`, `TestMR002_RevokedInviteStopsWorking`, `TestMR002_OnlyTheMasterManagesInvites`.
- Playwright test: `o mestre gera um convite, vê o link uma vez e o revoga` (`@MR-002`, `e2e/tests/campaigns.spec.ts`).

#### Related
- RN-07 (see [Business rules](rules.md)).
- RN-15 (invite with approval) and [MR-024](#mr-024-approve-the-character-from-the-invite) extend this story: the master ticks "Exigir aprovação do mestre" on the invite, the player already creates the character through the invite, and the master approves.

### MR-005: Create NPCs

**As a** master, **I want** to create NPCs of each kind (enemy, boss, minion, story), with a full or a basic sheet depending on the kind.

- Priority: MVP (prerequisite)
- Rules: RN-04
- Modules: characters

#### Acceptance criteria
- **Given** I am the master of "Mirathel", **when** I create an enemy or a boss, **then** it has a full sheet; **when** I create a minion or a story NPC, **then** it has a basic sheet.
- **Given** I am a player in "Mirathel", **when** I open the campaign, **then** I see no NPC **and** the server refuses if I try to create one.
- **Given** an NPC created in "Mirathel", **when** the master opens another campaign, **then** the NPC does not appear there: using the same NPC in other campaigns is [MR-022](#mr-022-reuse-npcs).

#### In the app
- The master creates NPCs with `CharacterService.CreateCharacter`. An enemy and a boss get the full sheet (with the calculated values), a minion and a story NPC get the basic sheet, and the server refuses a sheet of the wrong kind.
- The NPC stays in the campaign where it was created, and its owner is the master.
- Go tests: `TestMR005_MasterCreatesNpcsOfEachKind` (first criterion) and `TestMR005_PlayersCannotSeeOrCreateNpcs` (second and third).
- Playwright tests: "o mestre cria um inimigo com ficha completa e um minion com ficha básica" and "o jogador não vê os NPCs da campanha".

## Priority: Later

Out of the MVP. They are planned for Etapa 11 of the [roadmap](../roadmap.md).

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
- Part of this story is in the MVP: the guided sheet edit, [MR-040](#mr-040-level-up-from-the-sheet). This story stays after the MVP as the complete level-up screen.
- The rules engine (rules as data) is a direct prerequisite. See [ADR-0008](../adr/0008-regras-dnd-conteudo-como-dados-motor-puro.md).

### MR-020: Look up the rulebook

**As a** master, **I want** to look up the rulebook with a smart search.

- Priority: Later
- Rules: —
- Modules: rules

Spells came earlier, in the MVP: [MR-045](#mr-045-look-up-spells).

### MR-021: Copy a character

**As a** player, **I want** to copy my character to another campaign, **so that** I can play both at the same time or a continuation.

- Priority: Later
- Rules: RN-03, RN-23
- Modules: characters

#### Acceptance criteria (proposal)
- **Given** a sheet that uses table content (a class, race or spell the master registered), **when** the player tries to copy it to another campaign, **then** the app refuses and says why: table content is valid only in its own campaign (RN-23, from MR-025).

#### Related
- RN-03 (see [Business rules](rules.md)).

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
Today, deleting the account of whoever created the campaign deletes the whole campaign (see [Privacy](../privacy.md#delete-the-account)). With more than one master, or after a hand-over, this changes: the campaign is deleted only when the last master leaves. See [ADR-0011](../adr/0011-autorizacao-papeis-por-campanha.md), as a proposal.

### MR-026: Propose a new race or class

**As a** player, **I want** to propose a race or class that does not exist in the app when I create the character, with the PDF or the link to the rules, **so that** the master can read it and decide.

- Priority: Later
- Rules: RN-15 (the same idea of approval as the invite)
- Modules: rules, characters

It comes after MR-025, and the player's proposal is valid only after the master approves it.

#### Acceptance criteria (proposal)
- **Given** I want to play a cook, a fan-made class, **when** I create the character and propose the class with the link to the PDF, **then** the master sees the request **and** the character waits for the decision.
- **Given** a request for a new class, **when** the master approves it and registers its rules (MR-025), **then** the character uses the class; **when** the master refuses, **then** the player picks another class.

#### Example
The player wants to play a cook, an unofficial class. They register the class when creating the character and add the link to the PDF (or the PDF) for the master to read, approve or refuse, and to register how its rules work.

### MR-027: Read the rules from a PDF

**As a** master, **I want** to upload the PDF with the rules and see the app register its classes, races and rules by itself, **so that** I do not type everything.

- Priority: Later
- Rules: —
- Modules: rules

It comes after the master's own registration (MR-025), and the master reviews everything before it takes effect.

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
- Who levels up: the player from the sheet (today, MR-040) or only the master.
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
