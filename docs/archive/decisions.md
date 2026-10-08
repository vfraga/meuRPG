# Decision history

Who decided what, when, and under which question. This is the history of the MeuRPG product, architecture, data, operations and design decisions, kept out of the current docs on purpose: the docs say what the system does and why, this file says when and by whom it was decided. For the delivery history (Etapas, PRs, migrations, slices), see [etapas.md](etapas.md).

Conventions used below:

- Question numbers are the rows of the progress document ("MeuRPG — Como está o trabalho"). Samuel is the product owner and answered questions 1 to 65 (the first batches on 29/09, 02/10 and 03/10/2026); Vinicius answered 66 to 87 on his behalf (04/10, 05/10 and 07/10/2026).
- "Etapa" is a delivery stage of the [roadmap](../roadmap.md); "slice" ("fatia" in the original notes) is a numbered piece of an Etapa (for example 10.4b); PR numbers are those of `PuraFome/meuRPG`.
- ADRs are the private architecture decision records in `docs/adr/` (not published).
- Portuguese terms in quotes are UI strings or the names of the original sources.

## Contents

1. [Questions answered](#questions-answered): by date, 29/09 to 07/10/2026 (questions 1 to 87).
2. [Roadmap and scope decisions](#roadmap-and-scope-decisions): what the MVP is and how it grew.
3. [Decisions by document](#decisions-by-document): the decisions that used to sit in the product, privacy, architecture, data, operations and design docs.
4. [Open at the time of archiving](#open-at-the-time-of-archiving).

## Questions answered

Nothing here is open. Each answer from Samuel became a rule marked "Decided". Links point to the current product docs ([rules](../product/rules.md), [stories](../product/stories.md), [glossary](../product/glossary.md)). Question numbers are the rows of the progress document. Stage ("Etapa") and slice ("fatia") numbers are the roadmap's; PR numbers are those of `PuraFome/meuRPG`.

### Answered on 29/09/2026

- **Product name and domain.** "our pure rpg" is a provisional name; the domain is chosen only when the name is final. See [ADR-0008](../adr/0008-regras-dnd-conteudo-como-dados-motor-puro.md).
- **RN-02 (HP correction by hand).** Yes, the master can correct HP and spell slots at any moment of the session; the master has the final word. See [RN-02](../product/rules.md).
- **RN-05 (master who is also a player in another campaign).** Yes, decided; the backend already supported it. See [RN-05](../product/rules.md).
- **MR-015 (who chooses the scene actions).** In the RP scene, the master chooses the possible actions; in combat, the system decides and shows the actions, by the D&D rules. See [MR-015](../product/stories.md#mr-015-rp-scene-actions) and [ADR-0008](../adr/0008-regras-dnd-conteudo-como-dados-motor-puro.md).
- **RN-09 (XP by gold).** 1 XP per 1 gold piece (GP), as in the old editions. See [RN-09](../product/rules.md).
- **RN-03 (one character per campaign, or another when the first dies).** The player only creates a new character in the same campaign when the current one dies; the dead character is not deleted, it stays in the system. See [RN-03](../product/rules.md).
- **RN-07 (invite uses and validity).** Default of 1 use and 7 days; the master chooses from 1 to 20 uses and from 5 minutes to 30 days, and can revoke. The behaviour already implemented became the rule. See [RN-07](../product/rules.md) and [MR-002](../product/stories.md#mr-002-generate-an-invite).
- **RN-06 (is the on-screen notification enough, or is push needed?).** The on-screen notification is enough, for whoever has the app open; no browser push notification in the MVP. See [RN-06](../product/rules.md).
- **RN-08 (does DOCX continue? what display format?).** No DOCX: only editable PDF, in the D&D Beyond format or the Portuguese sheet. The import itself is left for after the MVP. See [RN-08](../product/rules.md) and [MR-007](../product/stories.md#mr-007-import-a-sheet-from-pdf).
- **MR-018 and MR-019 (MVP or Later?).** MVP, in Etapa 5 of the roadmap, next to the maps. See the [roadmap](../roadmap.md) and [stories](../product/stories.md#mr-018-campaign-document).
- **Does any data from the old app need to come to the new system?** The old database can be deleted. Only the characters are imported into the new database, in a one-time import. See the [data model](../data.md) and the [roadmap](../roadmap.md).
- **Classes and races the table uses today (ADR-0008).** All 12 classes and 9 base races of D&D 5e, with plans to add the official expansions and some community-made content. The SRD 5.1 covers the 12 classes and 9 base races, but only one subclass per class and a limited set of subraces and backgrounds; the rest (other subclasses, the Sage background, expansions and community content) comes in as content registered by the table. See [ADR-0008](../adr/0008-regras-dnd-conteudo-como-dados-motor-puro.md).
- **Characters of someone who deletes the account.** The characters of a player who deletes the account stay linked to the master and are not deleted. A master who deletes the account has 30 days to come back with the same account before everything is deleted. See [RN-16](../product/rules.md) and [Privacy](../privacy.md#delete-the-account).
- **How long do we keep an unused account?** Each person chooses in their own profile; default of 1 year. See [RN-16](../product/rules.md).
- **More than one master.** Yes: a campaign can have more than one master, and a master can hand the campaign over to another. See [RN-13](../product/rules.md) and [MR-023](../product/stories.md#mr-023-hand-over-or-share-the-campaign).
- **Does creating a campaign require a Google account?** Confirmed for the MVP: only a full master account creates a campaign, and today only the Google account is full. See [RN-14](../product/rules.md).
- **Invite with approval.** Yes, with an addition: through the invite the player already creates the character, and the master approves or refuses that character for the campaign. See [RN-15](../product/rules.md) and [MR-024](../product/stories.md#mr-024-approve-the-character-from-the-invite).
- **MR-002, proposed criteria.** Accepted by Samuel, together with the invite defaults (1 use, 7 days) already implemented. See [MR-002](../product/stories.md#mr-002-generate-an-invite).
- **Rules as data and Expr (ADR-0008).** Accepted: the rules become data, the SRD 5.1 ships with the app, what is not in it the table registers, and the formulas run in the Expr library (version 1.17.7 or newer), with the built-in functions turned off and a size limit. See [ADR-0008](../adr/0008-regras-dnd-conteudo-como-dados-motor-puro.md).
- **Priority of MR-023 and MR-024.** MR-024 (approve the character from the invite) is in the MVP, in Etapa 4; MR-023 (hand over or share the campaign) is after the MVP, in Etapa 11. See the [roadmap](../roadmap.md).
- **Player password without Google (RN-17).** The player joins without a password. When the first 30-day session expires, the app requires a password or a link to Google to continue (ADR-0009, option 3). See [RN-17](../product/rules.md).
- **Character of someone who deletes the account (RN-16).** The proposed handling was accepted: the character becomes the master's, with no link to the deleted account; the deletion screen warns and lets the person delete the character too; the free text that stays is not filtered. The master's 30-day wait uses the "delete on" mark on the account, checked at login. See [Privacy](../privacy.md#the-tension-between-keeping-the-character-and-erasing-the-identity).
- **Minors under 18.** The MVP is only for people over 18, by self-declaration. A minor who is already at the table plays without an account of their own until there is a flow with the guardians reviewed by a lawyer. See [Privacy](../privacy.md#minors).
- **Controller, data protection officer and channel.** Samuel is the controller and Vinicius is the data protection officer (encarregado). An e-mail address used only for this receives requests until the domain exists. See [Privacy](../privacy.md).
- **Opening to other tables or charging.** Not in the MVP: the app is only for our table. The decision comes back before opening. See [Privacy](../privacy.md).
- **CockroachDB plan and backups.** The current plan stays, with the backups in São Paulo, kept for at most 30 days. See [Operations](../operations.md).
- **Minors at the table today.** There are none (answered by Vinicius on 29/09/2026).
- **Character created after the first session (RN-01).** Stays editable until the next session starts (answered by Vinicius on 29/09/2026). See [RN-01](../product/rules.md).
- **Descriptive text after the lock (RN-01).** The player edits the character's story (personality, appearance, history, allies) while the sheet is a draft. After the lock, only when the master releases it, character by character, until the next session starts or until the master locks again. The calculated numbers are never editable (answered by Vinicius on 29/09/2026). See [RN-01](../product/rules.md).
- **Acceptance criteria of MR-005.** Accepted as proposed (answered by Vinicius on 29/09/2026). See [MR-005](../product/stories.md#mr-005-create-npcs).
- **RN-11 (master notes).** Decided: only the master reads and edits the master notes (answered by Vinicius on 29/09/2026). See [RN-11](../product/rules.md).
- **SRD text.** Names appear in Portuguese, with the official terminology of the Brazilian editions of the Player's Handbook and the Dungeon Master's Guide (aligned on 07/10/2026; the exception is what Samuel decided, such as "Derrubado" for prone, question 43, and WotC proper names that the SRD 5.1 does not carry, which follow the SRD's name); descriptions stay in English for now. Later we will look for a Portuguese translation with a suitable licence (answered by Vinicius on 29/09/2026).
- **CockroachDB plan.** It is the legacy Unlimited plan, contracted before the 2024 licence change. Changing plan loses Unlimited; whether to migrate to Cloud SQL or another product is under evaluation (answered by Vinicius on 29/09/2026). See [Operations](../operations.md).
- **Content registered by the table.** Applies per campaign, because different campaigns use different materials (answered by Vinicius on 29/09/2026). See [ADR-0008](../adr/0008-regras-dnd-conteudo-como-dados-motor-puro.md).
- **Old app database.** It is shut down, with its backups, right after the one-time import of the characters, in Etapa 11 (answered by Vinicius on 29/09/2026). See the [roadmap](../roadmap.md).
- **Physical dice or app dice (RN-18).** Both: the master chooses whether to allow it; if allowed, each player chooses between the app's dice and physical ones (decided by Samuel on 29/09/2026). See [RN-18](../product/rules.md).
- **Content that is not in the SRD.** Samuel's idea (29/09/2026): the master sends the rules PDF and the app registers the classes, races and rules by itself; when it cannot, the master registers them, and the player can propose a new race or class (with the PDF or the link) for the master to approve. It became the stories [MR-025](../product/stories.md#mr-025-register-table-content), [MR-026](../product/stories.md#mr-026-propose-a-new-race-or-class) and [MR-027](../product/stories.md#mr-027-read-the-rules-from-a-pdf). The priority was decided on 02/10/2026: after the MVP. On 03/10/2026, Vinicius put MR-025 in the MVP (Etapa 10); MR-026 and MR-027 stay in Etapa 11.

### Answered on 02/10/2026

Brought by Vinicius. Samuel accepted questions 33 to 42 as we proposed them. The numbers are those of the rows of the progress document.

- **Question 20: priority of the table content (MR-025 and MR-026).** Both went to after the MVP: first the registration by the master ([MR-025](../product/stories.md#mr-025-register-table-content)), then the player's proposal ([MR-026](../product/stories.md#mr-026-propose-a-new-race-or-class)), which only counts after the master approves. On 03/10/2026, Vinicius put MR-025 in the MVP (Etapa 10); MR-026 stays in Etapa 11. See the [roadmap](../roadmap.md).
- **Question 21: read the rules from a PDF (MR-027).** Yes, but after the registration by the master (MR-025), and the master reviews everything before it counts. It stays in Etapa 11. See [MR-027](../product/stories.md#mr-027-read-the-rules-from-a-pdf).
- **Question 22: does the app keep the PDF?** The PDF is kept only while it is processed and is deleted right after; a short time to live (TTL) on the file guarantees deletion even if processing fails. See [MR-027](../product/stories.md#mr-027-read-the-rules-from-a-pdf) and [Privacy](../privacy.md).
- **Question 23: does the player leave when the master refuses the invite character?** Yes: the refused character and the pending participation are deleted, and the master sends a new invite if they want. Already implemented. See [RN-15](../product/rules.md) and [MR-024](../product/stories.md#mr-024-approve-the-character-from-the-invite).
- **Question 24: the pending player who never creates the character.** The master now sees who is pending without a character, with a button to remove them, and the pending participation is deleted automatically after 30 days without a character. Decided and done, on the server and on the screen. See [MR-024](../product/stories.md#mr-024-approve-the-character-from-the-invite).
- **Question 25: ordinary invite for someone pending.** The pending player becomes a member at once, because the ordinary invite does not ask for approval and counts as the master's approval. Decided and done on 02/10/2026. See [RN-15](../product/rules.md).
- **Question 27: does the player see the campaign document (MR-018)?** No: in the MVP, only the master. Already implemented. See [MR-018](../product/stories.md#mr-018-campaign-document).
- **Question 28: in the session, does the player see the other characters' HP?** No: each player sees their own, and the master sees everyone's. Already implemented. See [RN-02](../product/rules.md).
- **Question 29: can a battle or RP-scene point exist before combat and scenes?** Yes, and it already works that way: the point has a name and a description, and battle and scene points open the combat and the scene when those stages arrive. See [MR-008](../product/stories.md#mr-008-points-of-interest).
- **Question 30: gallery limits (MR-019).** JPEG, PNG or WebP, up to 10 MB per image, up to 300 images and 500 MB per campaign; the photo metadata (EXIF, location) is removed on upload. Already implemented. See [MR-019](../product/stories.md#mr-019-image-gallery).
- **Question 31: NPC on the map without the players seeing?** Yes: the NPC token is born hidden, and the master reveals it when they want. Already implemented. See [MR-009](../product/stories.md#mr-009-map-without-spoilers).
- **Question 32: does the player keep access to the image after the master stops showing it (MR-028)?** The default stays: the image disappears from the players' screen and they lose access when the master stops showing it. In addition, the master gets a control to leave the image with the players when needed. The control has existed since Etapa 6: the "Deixar com os jogadores" switch, and the "Imagens que o mestre deixou" list on their session page. See [MR-028](../product/stories.md#mr-028-show-an-image-to-the-players).
- **Question 33: initiative of several identical NPCs.** Each NPC rolls its own initiative; the group does not roll together. Decided and done in Etapa 6. See [RN-19](../product/rules.md).
- **Question 34: does the player see the enemies' HP?** Not as a number: they see a word, Ileso (unhurt), Ferido (hurt), Muito ferido (half or less) or Derrotado (defeated). Decided and done in Etapa 6. See [RN-20](../product/rules.md).
- **Question 35: does the player see the NPCs' AC and rolls?** No: they see whether they hit or missed and the damage they take. Decided and done in Etapa 6. See [RN-20](../product/rules.md).
- **Question 36: grid square size.** Each square is worth 1.5 m, and so is the diagonal step (the SRD rule). Decided and done in Etapa 6. See [RN-21](../product/rules.md). **Changed on 03/10/2026:** Vinicius decided that movement is one level (the distance is the straight line between the centres of the squares), in place of this answer; it changes in Etapa 9. The 1.5 m square stays.
- **Question 37: movement limit.** The app stops the player from walking more than the turn's movement (it warns and does not allow it); the master moves anyone anywhere. Decided and done in Etapa 6. See [RN-21](../product/rules.md).
- **Question 38: physical dice.** The player types the sum of the dice, and the app adds the modifier. Decided and done in Etapa 6. See [RN-18](../product/rules.md).
- **Question 39: the third failure on the death save.** The character only dies when the master confirms; then RN-03 applies (stays dead, and the player can create another). Decided and done in Etapa 6. See [RN-03](../product/rules.md).
- **Question 40: the rolls of character creation (abilities and HP).** They roll in the browser and are not recorded: the sheet stays editable until the first session, and the master reviews. Decided and implemented (02/10/2026, Etapa 6). See [MR-004](../product/stories.md#mr-004-character-sheet-in-the-pdf-format).
- **Question 41: how to roll the abilities.** The app offers rolling 4d6 dropping the lowest, six times, with the player assigning the values, and also the standard array (15, 14, 13, 12, 10, 8); whoever prefers keeps typing. Decided and implemented (02/10/2026, Etapa 6). See [MR-004](../product/stories.md#mr-004-character-sheet-in-the-pdf-format).
- **Question 42: conditions and concentration.** In the MVP, the app only marks and reminds (it reminds the concentration check when the character takes damage), without applying the effects by itself; the master decides. Decided and done in Etapa 6. See [RN-22](../product/rules.md).

### Answered on 03/10/2026

Samuel answered questions 43 to 65, and Vinicius decided where each change goes in the roadmap. What he accepted as we proposed only changes the text of the documents; what changes what exists or what is designed was done in Etapas 8 and 9, and each question says where.

- **Question 43: the name of the condition "prone".** "Derrubado", so it is not confused with "Caído" (the state at 0 HP). Already implemented. See [RN-22](../product/rules.md) and the [glossary](../product/glossary.md).
- **Question 44: the NPC's XP, by CR or typed.** Both ways are valid, and the master chooses: the CR is optional, and the XP field accepts any number. Already implemented. See [RN-09](../product/rules.md) and [MR-016](../product/stories.md#mr-016-award-xp).
- **Question 45: who receives the XP.** By enemies, the master decides at the end of combat how to split it (the list of who receives). Already implemented. By milestones, the master defines beforehand the moments at which they give a full level (the planned milestones). Decided and done in Etapa 8 (slice 8.10, PR #88). See [RN-09](../product/rules.md) and [MR-016](../product/stories.md#mr-016-award-xp).
- **Question 46: the division that does not come out even.** Rounds down. Already implemented. See [RN-09](../product/rules.md).
- **Question 47: XP by gold.** The gold comes from treasures and chests placed beforehand on the map; when the group goes back to town, what was found becomes XP, 1 per GP. Typing the GP keeps working. Decided; server done in Etapa 9 (slice 9.11) and screens done in 9.18. See [RN-09](../product/rules.md) and [MR-041](../product/stories.md#mr-041-treasure-and-xp-by-gold).
- **Question 48: level up before the full screen.** While the character "Pode subir de nível" (by milestone or by XP), the player edits the sheet, only to add what the next level gives; the rest stays locked. Decided, before the MVP (Etapa 8); the server (slice 8.13) and the screen (slice 8.15, without master veto, as question 66 decided) are done. The full level-up screen is after the MVP. See [RN-01](../product/rules.md), [RN-12](../product/rules.md), [MR-040](../product/stories.md#mr-040-level-up-from-the-sheet) and [MR-017](../product/stories.md#mr-017-level-up).
- **Question 49: the level-up notice.** The master and the player see it. Already implemented. See [RN-12](../product/rules.md).
- **Question 50: who sees the XP history.** Everyone in the campaign. Already implemented. See [RN-09](../product/rules.md) and [MR-016](../product/stories.md#mr-016-award-xp).
- **Question 51: what the RP scenes test.** Skill checks, ability checks and saving throws; spells and features after the MVP. Already implemented. See [MR-015](../product/stories.md#mr-015-rp-scene-actions).
- **Question 52: does the player see the DC?** The master chooses, per scene, whether the players see the DC and whether they passed; the default stays hidden. Decided; the server (slice 8.9) and the screens (slice 8.14) are done. See [RN-20](../product/rules.md) and [MR-015](../product/stories.md#mr-015-rp-scene-actions).
- **Question 53: who opens the scene.** The master opens it, and a revealed point also opens it. Already implemented. See [MR-015](../product/stories.md#mr-015-rp-scene-actions).
- **Question 54: the log of the scene rolls.** It exists, as it is. Already implemented. See [MR-015](../product/stories.md#mr-015-rp-scene-actions).
- **Question 55: the attempts.** It is not one per opening of the scene: the master sets the limit per action and per player, and can give a player one more attempt. Decided; the server is done (slice 8.9, Etapa 8: 1 by default, from 1 to 5 or unlimited, reset when the scene is reopened) and so are the screens (slice 8.14). See [RN-20](../product/rules.md) and [MR-015](../product/stories.md#mr-015-rp-scene-actions).
- **Question 56: joint attack.** The group includes the players, and the combatants of the group act in the same turn. Decided and done in Etapa 8 (slice 8.11, 04/10/2026), with the NPC-only group visible only to the master. See [MR-013](../product/stories.md#mr-013-turn-order).
- **Question 57: Shield in the spell list.** It stays among the unavailable ones on the character's own turn. Already implemented, on the server (slice 8.1) and on the screen (slice 8.4, PR #86). See [MR-014](../product/stories.md#mr-014-your-turn).
- **Question 58: the spells that read HP.** The six of the SRD (Sleep, Colour Spray, Power Word: Stun and Kill, Spare the Dying and Heal), without Dobre pelos Mortos (Fold of the Dead). Already implemented, on the server (slice 8.1) and on the screen (slice 8.4, PR #86). See [MR-014](../product/stories.md#mr-014-your-turn).
- **Question 59: who the clue goes to.** Only to whom the master chooses, with nobody marked beforehand; the players share what they discovered at the table, in roleplay. Already implemented, on the server and on the screen (slice 8.5, PR #89). See [MR-029](../product/stories.md#mr-029-scene-hooks-and-clues).
- **Question 60: does the master read the players' notes?** Never. Already implemented. See [MR-030](../product/stories.md#mr-030-player-notes).
- **Question 61: what a discovered scene is.** The revealed or opened scene, for the whole group. Already implemented. See [MR-030](../product/stories.md#mr-030-player-notes).
- **Question 62: the NPC portrait.** It stays on the NPC's sheet, and a PNG with a transparent background has to work, keeping the transparency. Decided and done in Etapa 8 (slice 8.6, PR #87). See [MR-031](../product/stories.md#mr-031-npcs-in-the-scene).
- **Question 63: what the player does in the scene.** The scene is entirely the master's; any scene point opens it (already implemented). The player only sees the NPCs and can tap one to see it larger, nothing more (done in slice 8.6, PR #87). See [MR-031](../product/stories.md#mr-031-npcs-in-the-scene).
- **Question 64: the highlights.** The combat ones, as they are (already implemented, on the server and on the screen, slice 8.6), plus "Mais tesouro encontrado" (done on the server in slice 9.11, together with MR-041, and on the screen in slice 9.18) and "mais testes passados fora do combate" (scene checks with a DC, only from scenes that showed the DC), this one done on the server (`GetSessionSummary`, slice 8.9) and on the screen (slice 8.14): the summary appears when the master ends the session. See [MR-032](../product/stories.md#mr-032-combat-highlights).
- **Question 65: print the map.** The master chooses the square size and the paper size (A4, A3, A2, Letter...). Decided and done in Etapa 8 (slice 8.7). See [MR-033](../product/stories.md#mr-033-print-the-map-with-the-grid).

### Answered on 04/10/2026

Vinicius answered for Samuel: 66 as we proposed, and 67 to 75, from Etapa 9, accepting our suggestions, with four additions (69 to 72). Everything was done in Etapa 9, and each question says in which slice and PR.

- **Question 66: does the master veto a level up from the sheet?** No. The master is notified ("Subiu para o nível N") and sees "O que mudou" in the character list, and corrects whatever they want on the sheet, as always (RN-02). A veto would need a new state, "waiting for the master", which does not exist. Decided and already done this way in Etapa 8 (slices 8.13 and 8.15). See [MR-040](../product/stories.md#mr-040-level-up-from-the-sheet).
- **Question 67: fog of war.** Each player sees only what their own character (and their creatures) can see; the master can turn on "Visão do grupo" on each map, and the players' characters never disappear for the other players. What was already seen stays on the map, darkened. Decided and done in Etapa 9: on the server, each character's vision (slices 9.4, 9.5 and 9.7; PRs #107, #109 and #113), and on the screens, each player's map and the master's "Ver como" (slice 9.13, #118) and the fog in the map editor (slice 9.12, #123). See [MR-036](../product/stories.md#mr-036-fog-of-war-by-sight).
- **Question 68: the walls.** The master paints the walls on the grid, and they block sight, light and movement, as in the official rules. On a map without walls, sight depends only on light. Decided and done in Etapa 9: the walls on the server (slices 9.2 and 9.3; PRs #97 and #103) and painted in the map editor (slice 9.12, #123). See [MR-036](../product/stories.md#mr-036-fog-of-war-by-sight).
- **Question 69: difficult terrain and other creatures.** Each difficult-terrain square the character enters costs 1.5 m more. And, by the official rules: moving through the space of a creature that is not an enemy costs as difficult terrain; through an enemy's it is not possible, unless there are two sizes of difference; nobody ends movement in another's space; and leaving an enemy's reach provokes an opportunity attack, which the app offers to whoever can attack (except after Disengage). Decided and done in Etapa 9: on the server (slice 9.6, PR #104) and on the movement screen (slice 9.15, #115). See [RN-21](../product/rules.md) and [MR-034](../product/stories.md#mr-034-special-movement).
- **Question 70: cover.** It is marked on the map (a column, a low wall, crates), so players and enemies can use it: the app computes the cover of each attack from the line between the two (half +2, three-quarters +5; behind a wall, nobody aims). The master also marks a combatant's cover for what the map does not show, and that mark disappears when the combatant moves. Decided and done in Etapa 9: on the server (slice 9.6, PR #104), on the combat screen (slice 9.15, #115) and painted in the map editor (slice 9.12, #123). See [MR-034](../product/stories.md#mr-034-special-movement).
- **Question 71: noticing and finding a trap.** Whoever comes within 3 m and sees the square notices it if their passive Perception is equal to or greater than the DC to notice (-5 in dim light), and only that player starts seeing it. When searching, the player chooses a Perception check (against the DC to notice, as in the book) or an Investigation check (against the DC to find). Decided and done in Etapa 9: on the server (slice 9.8, PR #110, and the notice to the player, #119) and on the session (slice 9.14, #119) and map editor (slice 9.12, #123) screens. See [MR-035](../product/stories.md#mr-035-traps).
- **Question 72: attacking someone you cannot see.** The app does not show the enemy the character cannot see; the player tells the master where they attack (even a Fireball in a suspicious corner), and the master resolves it. Decided. See [MR-036](../product/stories.md#mr-036-fog-of-war-by-sight).
- **Question 73: a trap's damage.** On a character, it waits for the master to apply it, like the damage of attacks and spells in combat, also outside combat; the master can change the number. Decided and done in Etapa 9: on the server (slice 9.8, PR #110) and on the session screen (slice 9.14, #119). See [MR-035](../product/stories.md#mr-035-traps).
- **Question 74: the character's creatures.** Wild Shape, Find Familiar (and the warlock's Pact of the Chain), Animate Dead and Conjure Animals come in, and the master can give any SRD creature to a character. Find Steed stays for after the MVP (it needs mounted combat rules). Decided and done in Etapa 9: the server of summoned and given creatures (slice 9.9, PR #106) and Wild Shape's (slice 9.10, #112), and the screens, on the sheet (slice 9.16, #117) and in combat (slice 9.17, #122). See [MR-037](../product/stories.md#mr-037-creatures-of-the-character).
- **Question 75: "Voltar à cidade" (back to town).** The master chooses the treasures found (all ticked) and who receives (all living characters ticked); the total becomes XP, 1 per GP, split and rounded down, in a single award that can be undone. Only in a campaign by gold; in the others, the treasure counts in the session summary ("Mais tesouro encontrado"). Decided and done in Etapa 9: the server (slice 9.11, PR #105) and the screens (slice 9.18, #116). See [MR-041](../product/stories.md#mr-041-treasure-and-xp-by-gold).

### Answered on 05/10/2026

Vinicius answered for Samuel all the questions of Etapa 10: six as we suggested, others with additions (82, 85, which he widened the same day, and 86), and 76, 79 and 83 after detailing the ideas.

- **Question 77: HP on level up.** The player chooses between rolling and the average; the master can leave only one of the two. Accepted as we suggested. See [RN-24](../product/rules.md).
- **Question 78: the ways to make the abilities.** All four are valid: the standard array, point buy with 27 points, 4d6 dropping the lowest (rolled and kept by the server) and typing. Accepted; 4d6 was already the tables' habit, and the SRD 5.2.1 only put it in an open text. See [RN-24](../product/rules.md).
- **Question 80: does a change in the table content apply to sheets that are already locked?** Yes, at once; the sheet shows "A classe mudou" when something falls outside the rules. Accepted. See [RN-23](../product/rules.md).
- **Question 81: the HP of a monster put in combat.** The average; the master can roll. Accepted. See [MR-042](../product/stories.md#mr-042-bestiary).
- **Question 82: the free-text background and subclass.** Both stay, with an addition: a background is not just a description. The free-text background follows the SRD 5.1 rule "Personalizar um antecedente" (customizing a background): besides the name, two skills (already so), two tools or languages in total, a feature (the text of whoever creates it) and the equipment. Done on the server in slice 10.2 (06/10/2026); the screen of the "Básico" step is 10.12b. See [MR-025](../product/stories.md#mr-025-register-table-content).
- **Question 84: the 3 m grid.** A map drawn with 3 m squares is calibrated and counts four 1.5 m squares in each; there is no 1 m square. Accepted. The 1.5 m (5 ft) squares, as in the Player's Handbook, have been the default since Etapa 6; what is new is the drawing with larger squares. See [RN-25](../product/rules.md).
- **Question 85: the AI-generated image.** In the MVP, all three ways: the scene art, the isometric view of a map and the map itself with texture, matching the grid (Vinicius corrected this the same day: the isometric view and the map on the grid are in the MVP, not later). The master chooses which NPCs and enemies appear, and can give other references from the gallery (the combat map, the portraits). On 06/10/2026, Vinicius added: the isometric view and the scene art made from a map show only what the players see now (the other rooms and the hidden enemies are left out), so the image increases immersion; the textured map still starts from the whole map, which the fog hides from each player. See [MR-039](../product/stories.md#mr-039-ai-generated-images-for-dungeons-and-scenes).
- **Question 86: who is the group of an encounter.** The living player characters of the campaign, and the master can add to the group the NPCs that accompany the characters at that moment in the story, with the level they say. See [MR-043](../product/stories.md#mr-043-generate-encounters).
- **Question 76: the table style and house rules.** Accepted as we suggested, for the MVP: at the top of "Regras da mesa", three ready-made styles — "Tudo no app" (dice in the app, combat with a map, fog on in new maps), "Mesa física" (physical dice, combat without a map by default, fog off) and "Teatro da mente" (each player chooses the dice, combat without a map, no fog) — and "Personalizado"; a style only fills in the defaults, which stay editable, and the house rules go on (RN-24). The idea comes from different masters (the narrator, the physical-table one, the tactical one, the beginner, the improviser, the table without phones): the app should take away the biggest pain of each. The rest was kept for after the MVP in [MR-046](../product/stories.md#mr-046-table-style-feature-by-feature). See [RN-24](../product/rules.md).
- **Question 79: what solving a puzzle gives.** Accepted as we suggested — the master is notified, and "Ao resolver" can open a door, reveal a map point or reveal a clue to whoever solved it —, with the hints the master releases and, so the MVP session is not repetitive, these further ideas already in the MVP: consequences (a wrong move triggers a trap, uses up an attempt or counts toward a limit of moves or time), a hint earned with a skill check, split information (each player sees a part of the clue) and three new types: the riddle, the sequence to repeat and the cipher. The rest was kept for after the MVP in [MR-047](../product/stories.md#mr-047-more-puzzles). See [MR-038](../product/stories.md#mr-038-puzzles) and [RN-27](../product/rules.md).
- **Question 83: what the players see of the table content.** (a) Every playable option: the master has the "Opções para os jogadores" screen, with a switch per class, subclass, race, subrace, background and spell (from the SRD and from the table); (b) a spell lookup in the app, "Magias", with all the spells that are on, but on the sheet each person only chooses from their own class's list, also in multiclass (the master accepting a spell outside the list is left for after the MVP); (c) everything, the numbers and the effects; (d) what is off never appears to the players; (e) when an entry changes, only the owners of the sheets that use it are notified. See [RN-23](../product/rules.md) and [MR-045](../product/stories.md#mr-045-look-up-spells). The server for the switches and the live hint is done (slice 10.1d); the screen is 10.11c.

### Answered on 07/10/2026

The full code review of 07/10 raised one product question, which Vinicius answered for Samuel, and two choices about AI images, which Vinicius decided.

- **Question 87: the name of a trap the master fires by hand.** It stays public: once the master fires a trap, every player reads its name in the combat log, even a player whose character does not see the square, as at a physical table, where everyone hears the trap go off. The map still hides the square. This was already the behaviour, written into RN-10 by PR #226. See [RN-10](../product/rules.md).
- **The monthly image slot of a failed request.** The slot comes back only when the call was certainly not billed: the model refused, the key was refused (401 or 403), or the answer had no image. A timeout, a connection cut after the request left and an unreadable answer keep the slot spent, because Gemini may have billed them. A panic in our own code still gives the slot back: it is a bug of the server, and the server's daily cap still bounds the cost. See [RN-28](../product/rules.md) and [Operations](../operations.md#generated-images-the-gemini-api).
- **"Redesenhar" with a full gallery.** A redraw of a generated dungeon's map is allowed with the gallery full when the old image goes away with it (it is not shown, not a portrait and not used elsewhere), because the gallery does not grow; it is still refused when the old image stays. See [Architecture](../architecture.md#generated-dungeon-maps).


## Roadmap and scope decisions

These decisions used to open and close the roadmap page.

- **29/09/2026, no gradual migration from the old app** (decided by Samuel). The new backend is built from scratch and covers every MVP story on its own. The old Angular app (`src/`) stays only as a reference until it leaves the repository in a separate PR; the old NestJS server (`server/`) will be removed (see [legacy-app.md](../legacy-app.md)).
- **29/09/2026, MR-023 and MR-024.** Both came from Samuel's answers of that day. The same day he decided the priority: MR-024 (approve the invite's character) goes into the MVP, in Etapa 4; MR-023 (hand over or split the campaign) goes after the MVP, in Etapa 11.
- **29/09/2026, the "ficha de papel" look** (decided by Vinicius, see [design.md](../design.md)). Between Etapas 4 and 5 the screens of Etapas 1 to 4 were redesigned, and from then on every new screen is designed and reviewed before the PR.
- **29/09/2026, MR-025, MR-026 and MR-027** (the content the table registers, including from a PDF) came from an idea of Samuel's. On 02/10/2026 they were placed after the MVP: first the registration by the master (MR-025), then the player's proposal (MR-026, always with the master's approval) and the PDF reading (MR-027, with the master reviewing everything). On 03/10/2026 only MR-025 went into the MVP (Etapa 10); MR-026 and MR-027 stay in Etapa 11.
- **03/10/2026, the MVP grew.** After a survey of other masters, Vinicius decided that Etapa 7 ends as approved and that Etapas 8, 9 and 10 enter the MVP. His decisions win over Samuel's earlier answers where they differ: circle movement (RN-21, question 36), MR-025 (table rules) and MR-010 (dungeons, now the generator) move into the MVP. The reason: it is what those masters said they need at the table. After Etapa 7 come the table, deep combat and map, and content and generation. The MVP is done at the end of Etapa 10. AI-generated images (MR-039) were first planned on Gemini in Vertex AI, in our Google Cloud project; on 05/10/2026, in the Etapa 10 plan, Vinicius switched to the Gemini API with a Google AI Studio key. Samuel is told in the progress document; it is not a new question for him.
- **03/10/2026, MR-040** (level up from the sheet) sits before the MVP in its own slice after the Etapa 8 screens; the full level-up screen stays after the MVP (MR-017).
- **05/10/2026, Etapa 10 planned** by Vinicius. Only three tables of SRD 5.2.1 (the 2024 rules) enter, credited and labelled on screen: the XP budget of encounters, the values of magic items, and the standard array and point buy of abilities (same as 2014); the rest stays on SRD 5.1. Out of scope: copying content between campaigns (MR-026), own monsters and magic items, feats outside the SRD, flanking, the hex grid, multi-floor dungeons and character images.
- **30/09/2026, MR-028** (show an image to the players) entered the MVP in Etapa 5 at Vinicius's request.
- **Etapa 11 (after the MVP).** The order: PDF sheet import (MR-007), the full level-up screen (MR-017; the guided sheet edit, MR-040, comes before the MVP), the rules book (MR-020), copy a character (MR-021), reuse NPCs (MR-022), hand over or split the campaign (MR-023), then the player's proposal (MR-026) and PDF reading of rules (MR-027). Kept for after the MVP: table style feature by feature (MR-046) and more puzzles (MR-047). Two cleanup tasks with no story: import only the characters from the old database and decommission it (decided by Samuel on 29/09/2026, see [data.md](../data.md) and [privacy.md](../privacy.md#the-legacy-app-database)), and remove `server/` (NestJS) and then `src/` (Angular) from the repository.
- There are no dates in the roadmap: the pace depends on each person's free time.

## Decisions by document

### Product: vision

- **Product vision, principle 5** — No gradual migration from the legacy app: the new backend is built from scratch, story by story. Decided 29/09/2026.

### Product: stories MR-001 to MR-016

- **MR-001 / RN-30** — Campaign cap per account (default 10) and creators' e-mail allow-list recorded as story criteria and tests. Dated 07/10/2026.
- **MR-003 / RN-03** — A player creates a new character in this campaign only when the current one dies; the dead character stays in the system. Answered 29/09/2026.
- **MR-003 / RN-17** — Login of a player without Google (per-table handle): joins without a password; when the first 30-day session expires, must set a password or link Google (ADR-0009, option 3). Was listed as pending in the open-questions doc.
- **MR-003 / RN-15** — Invite requiring approval: player joins as pending member, creates the character, which is born pending until the master approves or refuses. Implemented 29/09/2026.
- **MR-004** — The sheet is calculated by rules-as-data with formulas in the Expr language (ADR-0008). Samuel accepted this design 29/09/2026.
- **MR-004** — The character editor rolls abilities (4d6 drop lowest, six times, player distributes) and hit points, plus the standard array (15, 14, 13, 12, 10, 8); whoever prefers keeps typing. Rolls run in the browser, are not recorded, the sheet stays editable until the first session and the master reviews; these rolls are not part of RN-18. Requested by Samuel 01/10/2026 (questions 40 and 41), implemented 02/10/2026 (Etapa 6).
- **MR-006** — The last two acceptance criteria (character created after the first session locks at the next one; story locked, master releases it for one session) were answered by Vinicius 29/09/2026.
- **MR-006** — The edit address of a locked sheet shows the lock before any form (`Character.can_edit`). Since 03/10/2026.
- **MR-008** — The battle point and the scene point exist before combat (Etapa 6) and RP scenes (Etapa 7), with name and description only; until then opening them only shows the point's card. Decided 02/10/2026 (question 29 of the progress doc). Opening a submap point shows the point's card first, then the submap (design decision 30/09/2026). Rename/delete map: design E6-27, 03/10/2026.
- **MR-011 / RN-06** — The on-screen notification, for whoever has the app open, is enough for the MVP; no browser push notification. Answered 29/09/2026.
- **MR-011 / RN-07** — Invites default to 1 use and 7 days; master picks 1 to 20 uses and 5 minutes to 30 days, and can revoke. Answered 29/09/2026.
- **MR-012 / RN-20** — In combat the player sees enemies by a word (Ileso, Ferido, Muito ferido, Derrotado), not by HP. Decided 02/10/2026.
- **MR-012 / RN-02** — The master can correct HP and spell slots by hand during the session; the master has the final word. Answered 29/09/2026.
- **MR-013 / RN-19, RN-20, RN-21** — Each NPC rolls its own initiative; the player sees enemy state by a word, never HP or AC; each grid square is 1.5 m, diagonals included, and the app does not let the player exceed the turn's movement. Decided 02/10/2026.
- **MR-013** — The available speed comes from the rules engine (rules as data, ADR-0008). Samuel accepted 29/09/2026.
- **MR-013** — The turn group mixes NPCs and players (same initiative total side by side), and players can also attack together. Decided by Samuel 03/10/2026 (question 56); implemented 04/10/2026 (Etapa 8, slice 8.11). Movement in squares shown 03/10/2026 (slice 8.4).
- **MR-014** — Spell order on the player's turn, with Shield among the unavailable ones on the character's own turn. Decided by Samuel 03/10/2026 (question 57).
- **MR-014** — The six spells that read HP (Sono, Leque Cromático, Palavra de Poder: Atordoar e Matar, Estabilizar/Poupar os Moribundos, Cura Completa), without Toll the Dead (not in SRD 5.1). Decided by Samuel 03/10/2026 (question 58).
- **MR-014 / RN-18** — Dice: master chooses how the campaign rolls (each player chooses, all in the app, all with own dice); with physical dice the player types the sum and the app adds the modifier. Combat rolls followed this from slice 6.4a (attack d20 and damage) and 6.4b (spell-attack d20, healing, Second Wind d10, death save).
- **MR-014 / RN-20** — The player sees whether they hit or missed and the damage, not AC or the NPC's roll. Decided 02/10/2026 (done in slice 6.4a).
- **MR-014 / RN-22, RN-03** — Conditions and concentration are only marked and reminded; the master decides the effects (RN-22). On the third death-save failure the character dies only when the master confirms (RN-03). Decided 02/10/2026 (done in slice 6.4b).
- **MR-014** — Conditions: the 15 of the SRD; "Derrubado" is `prone` (question 43).
- **MR-014** — Which actions, bonus actions, reactions and resources the system knows come from the rules engine (rules as data). Samuel accepted 29/09/2026.
- **MR-015** — In the RP scene the master chooses the possible actions and the player sees what they can do with their own bonus; in combat the system decides and shows the actions by D&D rules. Answered 29/09/2026.
- **MR-015** — Scenes have skill checks, ability checks and saving throws (spells and abilities after the MVP); the master opens the scene, and a revealed point also opens it; the scene log exists. Decided by Samuel 03/10/2026 (questions 51 to 55).
- **MR-015** — "Mostrar a CD aos jogadores" is per scene, off by default (question 52). Attempts per player per action: 1 by default, another number or no limit (question 55); this replaces Etapa 7's "one roll per action while the scene is open". Server done in slice 8.9, screens in slice 8.14.
- **MR-015** — The scene log shows the master every roll (question 54). Any scene point opens even with no actions (question 63, changed in Etapa 8).
- **MR-016 / RN-09** — In the gold mode, 1 XP per 1 gold piece (PO), as in older editions. Answered 29/09/2026.
- **MR-016** — Questions 44 to 50 (NPC ND, who splits, rounding, gold, milestone, level-up notice, who sees the history): decided by Samuel 03/10/2026. Accepted as built: ND or typed XP, the master decides who receives, round down, the notice for master and player, history for everyone.
- **MR-016** — Planned milestones (question 45, design E8-14): done in Etapa 8, slice 8.10, PR #88, 04/10/2026. XP from treasure (MR-041): Etapa 9, PRs #105 and #116 (question 47).
- **MR-016** — The rules engine gives what each character gains on leveling up (rules as data, ADR-0008). Samuel accepted 29/09/2026.

### Product: stories MR-018 to MR-031

- **MR-018** — Carried over from the legacy app and confirmed in the MVP in Etapa 5 of the roadmap, next to the maps. Samuel confirmed 29/09/2026.
- **MR-018 (acceptance criteria)** — In the MVP only the game master sees the campaign document. Accepted 02/10/2026 (progress-doc question 27).
- **MR-019** — Confirmed in the MVP in Etapa 5 next to the maps. Samuel confirmed 29/09/2026.
- **MR-019 (limits)** — JPEG, PNG or WebP, up to 10 MB per image, 300 images and 500 MB per campaign. Accepted 02/10/2026 (question 30).
- **MR-024 (criteria)** — Each invite chooses whether it requires approval, and a refusal deletes the character and the membership. Accepted 02/10/2026 (questions 23, 24, 25). The last two criteria (pending player without a character; plain invite for a pending player) were implemented on the server 02/10/2026; the game master screen for the first of them was built in Etapa 6.
- **MR-024** — Done 29/09/2026 (Etapa 4): backend, screen and tests.
- **MR-028 (priority)** — Added to the MVP at Vinicius's request on 30/09/2026, in Etapa 5.
- **MR-028 (criteria)** — Accepted 02/10/2026 (question 32); the last criterion ("Deixar com os jogadores") was designed and built in Etapa 6.
- **MR-028 (question 32)** — Samuel answered 02/10/2026: the default stays (the image disappears from the players' screen and they lose access when the game master stops showing it), and the game master gets a control to leave the image with the players ("Deixar com os jogadores", Etapa 6). Automatically keeping every image already shown in a per-player "chest" of handouts stays out and would be a new story. The list appears only on the session page; there is no design for it on the campaign page for the player, left for a decision by Vinicius.
- **MR-025 (priority)** — Priority decided 02/10/2026 (question 20): the game master's registration comes first, before MR-026 and MR-027. On 03/10/2026 Vinicius put the story in the MVP, in Etapa 10 (it was "Depois"), and widened it: own spells, the campaign grid (size, or none), dice rules beyond RN-18, and house rules. MR-026 and MR-027 stay after the MVP. On 05/10/2026, in the Etapa 10 plan, Vinicius decided the story enters whole: classes and subclasses, races and subraces, backgrounds, spells, house and dice rules, and the grid.
- **MR-025 (criterion: content per campaign)** — Content is per campaign. Decided 29/09/2026.
- **MR-025 (criterion: multiclass at creation)** — Decided 06/10/2026 with the approval of the Etapa 10 designs.
- **MR-025 (criterion: area spell)** — Added 06/10/2026.
- **MR-025 (criterion: "A classe mudou")** — Question 80, answered 05/10/2026 (Samuel): the sheet shows the new number, also locked, and what fell outside the rules is a notice that never blocks another edit.
- **MR-025 (criterion: "Opções para os jogadores")** — Question 83, answered 05/10/2026: players do not see switched-off options; what is on they see in full.
- **MR-025 (criterion: "Outro" background)** — Question 82, answered 05/10/2026: the free-text background and subclass ("Outro") stay, and the background gets the SRD 5.1 choices (two skills, two tools or languages, a feature, equipment).
- **MR-025 (questions 76, 77, 78, 80, 82, 83, 84)** — Answered 05/10/2026: 77, 78, 80, 82 and 84 in the answered-questions document; 76 (the table style) and 83 (what players see) the same day.
- **MR-025 (out of scope)** — Out of the MVP: copying content to another campaign (MR-026), own monsters and magic items, feats outside the SRD, flanking, the hexagonal grid.
- **MR-025 (delivery)** — Engine (`Content.With`) slice 10.1b, 06/10/2026; storage/serving slice 10.1c; option switches server 10.1d; switches screen 10.11c; table-rules server 10.4a; table rules in combat 10.4b; combat without a grid on the server 10.5b; spells and "Outro" background server 10.2; table-rules screens 10.13a; combat-without-grid screens 10.13b; content screens (list, spell/race/background editors) 10.11; class and subclass editors 10.12; classes/subclasses end-to-end server 10.3; character editor and level-up with table content 10.12b. All 06/10/2026 (the Etapa view is in [etapas.md](etapas.md)).
- **MR-010 (priority)** — In the MVP, Etapa 10, since 03/10/2026 (it was "Depois", as "Desenhar masmorras").
- **MR-010 (criterion: base light)** — The base light of a generated map is bright ("Clara"), which the game master can change to dim or dark. Decided 06/10/2026 with the approval of the Etapa 10 designs.
- **MR-010 (clean room, ADR-0015)** — The donjon `dungeon.pl` generator is CC BY-NC 3.0, incompatible with Apache 2.0; one agent read the program and wrote a behaviour specification in our own words (05/10/2026), a different agent checked it contains no donjon code, data or text, and a third, who never read the program, implemented the generator from it. The specification and origin note went in the generator PR.
- **MR-010 (delivery)** — Door layer on the server slice 10.6c (05/10/2026); generator 10.6b (05/10/2026); dungeon-to-map on the server 10.6d (06/10/2026); doors on screen 10.14a; generator screens 10.14b (06/10/2026; designs E10-05). AI-generated image is slice 10.16.
- **MR-029** — Server slice 8.2 and screens slice 8.5 (designs E8-04, E8-05). Samuel decided questions 59 and 63 on 03/10/2026.
- **MR-029 (question 59)** — Only the players the game master chooses receive a clue, and the screen marks nobody in advance; each player recounts what they found at the table, in roleplay, without sharing it through the app. Samuel decided 03/10/2026. A revealed clue cannot be hidden again.
- **MR-029 (question 63, changed)** — Any scene point opens, even without actions; the `NO_ACTIONS` reason is no longer sent. Samuel decided 03/10/2026.
- **MR-030** — Server slice 8.2 and screens slice 8.5 (designs E8-06, E8-07). Samuel decided questions 60 and 61 on 03/10/2026.
- **MR-030 (questions 60, 61)** — The game master never reads players' notes, and "discovered scene" is a scene revealed or opened, for the whole group. Samuel decided 03/10/2026.
- **MR-030 (limits)** — The limits of 2,000 characters per note and 300 notes per player are a proposal from the Etapa 8 plan that Samuel did not change.
- **MR-031** — Server slice 8.3 (Etapa 8) and screens slice 8.6. Samuel decided questions 62 and 63 on 03/10/2026. Design E8-08 to E8-10 stands as approved.
- **MR-031 (question 62)** — The portrait is the NPC sheet's `portrait_image_id`; a PNG with a transparent background keeps its transparency and the stage draws it without a box.
- **MR-031 (question 63)** — A player taps an NPC on the stage and sees it larger, with name and portrait only; the scene belongs entirely to the game master.

### Product: stories MR-032 to MR-038

- **MR-032 (highlights)** — Five combat highlight categories (Most damage dealt, Most healing, Tank, Finishing blow, Critical hits) plus the two session-level ones, "Mais testes passados fora do combate" and "Mais tesouro encontrado". Samuel accepted them 03/10/2026, question 64.
- **MR-032 (request)** — Vinicius's request ("which player dealt the most damage, healed most, tanked more, etc.") was clarified 03/10/2026: highlights are per player, with the five categories above (question 64). Samuel added "most treasure found" and "most checks passed outside combat" on 03/10/2026, which need more than one combat; the session summary appears at the end of the session (artboard E8-11, states 4 and 5) and is `GetSessionSummary`.
- **MR-033 (print)** — The master chooses the square size and the paper size; only the image and the grid are printed; only the master prints; "Ofício" is the Brazilian one (21.6 x 33 cm). Samuel decided 03/10/2026, question 65.
- **MR-034 (special movement)** — The master paints difficult terrain, walls and cover on the map (half: low wall, crates; three-quarters: column, arrow slit), which players see and use; each difficult-terrain square entered costs 1.5 m more; other creatures and the opportunity attack as in the criteria, following the official rules. Vinicius decided, answering for Samuel, 04/10/2026, questions 68 to 70.
- **MR-035 (traps)** — A character notices a trap when within 3 m and seeing the square, with passive Perception equal to or greater than the DC (-5 in dim light), and only that player starts seeing it; when searching, the player chooses Perception or Investigation; damage to a player character waits for the master to apply it, as in combat. Vinicius decided, answering for Samuel, 04/10/2026, questions 71 and 73.
- **MR-036 (fog)** — Each player sees what their own character sees; the master may turn on "Visão do grupo" per map; player characters never disappear for other players; what was seen stays darkened; walls the master paints block sight and light as in the official rules; an enemy the character does not see does not appear to them, and attacking in the dark (even a Fireball in a suspect corner) is the master's call. Vinicius decided, answering for Samuel, 04/10/2026, questions 67, 68 and 72.
- **MR-037 (creatures)** — Wild Shape (leftover damage passes to the druid when the beast falls to 0 HP, as in the SRD), Find Familiar (and the warlock's Pact of the Chain), Animate Dead and Conjure Animals are in; the master may give any SRD creature to a character; Find Steed is left for after the MVP (needs mounted-combat rules); creatures summoned together roll one initiative and act together (joint turn). Vinicius decided, answering for Samuel, 04/10/2026, question 74.
- **MR-037 (house rule)** — Animate Dead creatures also roll a single initiative (the SRD says it only for Conjure Animals), and the group uses the first creature's Dexterity bonus. An engineering choice stated in the story, 04/10/2026.
- **MR-037 (delivery dates)** — Rules slice 9.1 and server slices 9.9 and 9.10 on 04/10/2026; sheet screen (9.16, design E9-10) and combat screen (9.17, designs E9-11 and E9-12) on 05/10/2026.
- **MR-038 (puzzles)** — The first three kinds (Apagar as luzes, combination lock, rotating symbols, which recall Skyrim's pillars with our own symbols) were decided by Vinicius 05/10/2026. Question 79, answered 05/10/2026, added to the MVP the hints, "Ao resolver", the consequences, the skill-check hint, split information, the riddle, the sequence and the cipher, so the session does not get repetitive. The other ideas went to after the MVP (MR-047).
- **MR-038 (delivery dates)** — Server slices 10.7a and 10.7b and screen slices 10.15a (design E10-06, nine states) and 10.15b (design E10-12, eleven states) on 06/10/2026.
- **MR-035 (design choices, SRD gaps)** — Our own choices without an SRD rule, stated in the story: the 3 m measured between square centres; the trap attack goes one by one to the creatures caught; each part's damage per creature is one roll; "Ao entrar na área" catches who is in the area (or only who entered if targets are manual); an active check in dim light has no disadvantage (the master decides). Slice 9.8, 04/10/2026.
- **MR-035 (screen vs. design)** — The three lights per character of design E9-02 do not exist because the server answers a single frame (how the character sees now). Slice 9.12/9.14.
- **MR-036 (editor vs. design)** — The design E9 said custom light radii up to 60 m in 0.5 m steps; the server limits are multiples of 1.5 m up to 36 m, and the screen follows the server. Slice 9.12.
- **MR-036 (no "Ver como" of the combat order)** — The server does not send the master the order as a player sees it, so the screen does not pretend. Slice 9.13.
- **MR-032 (session summary delivery)** — Server slices 8.3 and 8.9, combat screens 8.6, session summary screen 8.14, "Mais tesouro encontrado" server 9.11 and screen 9.18 (design E9-09).
- **MR-033 (delivery)** — Slice 8.7 of Etapa 8, browser only.
- **MR-034 / MR-035 / MR-036 (delivery)** — Etapa 9: calculations in slice 9.2; layers, settings and trap points on the server in 9.3 (PR #103); fog per player in 9.4 (#107); tiles in 9.5 (#109); movement in 9.6, opportunity attacks in 9.6b; fog in combat in 9.7; traps in play in 9.8 (#110); Wild Shape and the familiar's eyes in 9.10; editor screens in 9.12 (#123); fog screens in 9.13 (#118); trap/treasure session screens in 9.14; movement screens in 9.15 and 9.15b.

### Product: stories MR-039 to MR-045

- **MR-039 (provider)** — ADR-0019 (Gemini API with a Google AI Studio key, behind a small interface) replaces ADR-0014, which used Vertex AI. ADR-0014 dated 03/10/2026. The model `gemini-2.5-flash-image` was switched off by Google on 02/10/2026, so the default is `gemini-3.1-flash-image`.
- **MR-039 (three ways)** — The three ways (scene art, isometric view of a map, textured map matching the grid) are all in the MVP. Question 85, answered and corrected by Vinicius 05/10/2026.
- **MR-039 (references)** — Up to 10 object images and 4 character images per request, per the Gemini documentation checked 05/10/2026.
- **MR-039 (players' view)** — Scene art and isometric view made from a map start only from what the players see now; the textured map starts from the whole map because fog already hides it. Vinicius decided 06/10/2026, so the image adds immersion instead of spoiling discovery. The reference drawing carries only seen squares, seen creatures and the unrevealed secret door as a wall (06/10/2026).
- **MR-039 (monthly limit)** — The per-campaign monthly limit (proposal: 20) stays a proposal until the cost is measured with the real key (US$ 0.067 per 1K image on `gemini-3.1-flash-image`, measured 05/10/2026).
- **MR-039 (design deviations, E10-07)** — The server wins over the design: the textured map takes no NPC and no character image ("Quem aparece na imagem" disappears for it), and no NPC is offered while a combat runs on the map. Slice 10.8b.
- **MR-039 (fix rounds, slice 10.16, 06/10/2026)** — Round 1: whole-map flag (`shows_whole_map`, `generated_kind`), meaningful image names, hidden-NPC portrait refused in `object_image_ids`, idempotency key changes only after an answer or form change. Round 2: session image picker marks "Mapa inteiro", adjustment is center-cropped to the map proportion, migration 00161 backfills chains.
- **MR-040 (scope and place in the roadmap)** — Level up from the sheet is a pre-MVP slice after the Etapa 8 screens. Samuel answered 03/10/2026 (question 48); Vinicius decided the roadmap place the same day. The server covers hit points, ability increase, spells, subclass and feature options; multiclass, own subclass and the Warlock's invocations after level 2 stay with the GM in the editor.
- **MR-040 (no GM veto)** — The GM does not veto a level up: they are notified, see what changed and correct the sheet if they want (RN-02). Vinicius decided 04/10/2026, question 66, as the screen already did.
- **MR-040 (delivery)** — Server in slice 8.13; screen in slice 8.15 (design E8-15).
- **MR-041 (scope)** — Treasure and XP by gold belong to Etapa 9, with traps and chests (MR-035). Samuel answered 03/10/2026 (question 47); Vinicius decided the roadmap place the same day.
- **MR-041 (details)** — The GM marks who found the treasure (one or more characters); "Voltar à cidade" converts the chosen treasures (all marked) for the chosen characters (all living marked), 1 XP per gp, split and rounded down, in a single award that can be undone; only in a gold campaign, and in the others the treasure counts in the session summary. Vinicius decided, answering for Samuel, 04/10/2026, question 75.
- **MR-041 (delivery)** — Treasure point on the map: slice 9.3. Session screens: slice 9.14 (#119). Editor: slice 9.12. "Voltar à cidade" server: slice 9.11. Conversion screens: slice 9.18 (#116, design E9-09).
- **MR-042 (origin)** — Added 05/10/2026 in the Etapa 10 plan: Vinicius asked for the three donjon tools (bestiary, encounters, treasure). They do not read donjon's code: the monsters come from SRD 5.1 and the rest is ours or the SRD's.
- **MR-042 (HP when putting in combat)** — The average, and the GM can roll. Question 81, accepted 05/10/2026. Custom monsters stay out of the MVP.
- **MR-042 (delivery)** — Slice 10.9a (05/10/2026): bestiary reads and "Criar NPC", server only. Slice 10.9b (06/10/2026): "Pôr no combate", server; round 1 review (one NPC per campaign and creature; multiattack fixed to SRD 5.1 as content fx.15: Veteran 3, Shambling Mound 2, Brown Bear 2, Gibbering Mouther 1; NPCs already made follow the creature's multiattack). Slice 10.17a (06/10/2026): bestiary screen and "Criar NPC". Slice 10.17b (06/10/2026): monsters in combat, on screen. Content fx.13 brought the Portuguese attack names.
- **MR-043 (origin)** — Added 05/10/2026, like MR-042.
- **MR-043 (who is the group)** — The living player characters of the campaign, plus the NPCs the GM puts in the group at that moment of the story. Question 86, answered 05/10/2026.
- **MR-043 (budget table)** — The "XP Budget per Character" table of SRD 5.2.1 (CC BY 4.0), credited in `NOTICE` and on the "Créditos" page; SRD 5.1 has no difficulty table. Whether it fits the 2014 monsters, the table sees after playing. Vinicius, 05/10/2026.
- **MR-043 (delivery)** — Slice 10.9c (06/10/2026): server (`EncounterService`, content `fx.16`, migration 00160). Slice 10.17b (06/10/2026): the screen. Slice 10.13b: the "Com mapa / Sem mapa" choice of "Iniciar combate". Design deviations: the "Gerar encontro" sheet puts "Usar este encontro" first in document order; "Pôr um NPC no grupo" does not write the math "Orin soma 150, 225 e 400" (a subtraction the server does not answer) and gained "Só um nome"; "Pôr no combate" gained the "Nome" field and the button says "Pôr 3 no combate" (the plural of the name is a guess).
- **MR-044 (origin)** — Added 05/10/2026, together with MR-042 and MR-043.
- **MR-044 (scroll value)** — A spell Scroll is worth the rarity's whole value, while other consumables are worth half. Engineering decision 06/10/2026: SRD 5.1 scrolls take their rarity from the spell level, and the SRD 5.2.1 note leaves the scroll out of the halving.
- **MR-044 (scope)** — Custom magic items stay out of the MVP. SRD 5.1 has no values or random treasure tables, so the coin, gem and art tables are ours.
- **MR-044 (delivery)** — Slice 10.10a (05/10/2026): the 362 magic items in the rules content (fx.12). Slice 10.10b (06/10/2026): server (`TreasureService`, content fx.17, migrations 00163 and 00164). Slice 10.17c (06/10/2026, design E10-10): the screen.
- **MR-045 (origin)** — Added 05/10/2026 (question 83): a part of MR-020, with only the spells, brought forward to the MVP.
- **MR-045 (scope)** — The GM accepting a spell outside the class list is for after the MVP (MR-046).
- **MR-045 (delivery)** — Slice 10.1d (06/10/2026): what the GM turns off. Slice 10.2 (06/10/2026): `ListSpells` server. Slice 10.11b (06/10/2026, design E10-11, states 1 to 3): the "Magias" screen. Slice 10.11c (06/10/2026): live reload on `content_changed`.

### Product: prerequisite and later stories

- **MR-002 / MR-005 / MR-007 / MR-017 / MR-020 / MR-021 / MR-022 / MR-023 / MR-026 / MR-027 / MR-046 / MR-047, "Priority" sections** — MR-002 and MR-005 were set as MVP prerequisites (no earlier implementation to reuse, since the old app is discontinued, but other MVP stories depend on them). Decided 29/09/2026. The "Later" stories (outside the MVP) were assigned to Etapa 11 of the roadmap.
- **MR-002 acceptance criteria** — Accepted by Samuel on 29/09/2026 together with the backend; the master loves being able to choose the number of uses and the validity of the invite, and the implemented behaviour became RN-07.
- **MR-002 (backend and screen)** — Backend ready on 29/09/2026; the "Convites" screen ready on 29/09/2026.
- **MR-002 / RN-15 / MR-024** — Invite with approval (RN-15) and MR-024 extend MR-002; implemented 29/09/2026.
- **RN-07** — Decided on 29/09/2026.
- **MR-005 acceptance criteria** — Proposed by us and accepted; Vinicius answered 29/09/2026.
- **MR-005 (backend and screen)** — Backend ready on 29/09/2026 (Etapa 4); the screen came with the Etapa 4 screens PR.
- **MR-007 / RN-08** — Samuel answered 29/09/2026: no DOCX; only an editable PDF, in the D&D Beyond format or the Portuguese sheet.
- **MR-017 / MR-040** — Samuel decided on 03/10/2026 (question 48) that a part of level up comes before the MVP: the guided sheet edit, MR-040, in Etapa 8. MR-017 stays after the MVP as the complete level-up screen.
- **MR-017 / rules engine** — Rules as data (the rules engine) accepted by Samuel on 29/09/2026 (ADR-0008).
- **MR-020 / MR-045** — Spells came earlier, into the MVP, as MR-045. Decided 05/10/2026, question 83.
- **MR-021 / RN-23** — The refusal to copy a sheet that uses table content came from MR-025, 05/10/2026.
- **MR-021 / RN-03** — Answered 29/09/2026.
- **MR-022 / RN-23** — The refusal to take an NPC with table content to another campaign came from MR-025, 05/10/2026.
- **MR-026 priority** — Decided 02/10/2026 (question 20): comes after MR-025 (Etapa 11), and the player's proposal is valid only after the master approves. The example (a cook, an unofficial class) is Samuel's.
- **MR-027 priority** — Decided 02/10/2026 (question 21): yes, but after the master's registration (MR-025), and the master reviews everything before it takes effect (Etapa 11).
- **MR-027 file deletion** — The uploaded PDF is deleted when processing ends or fails, with a short TTL on the stored file as a guarantee. Question 22, 02/10/2026.
- **MR-046** — New on 05/10/2026 (question 76): the MVP gets the three ready-made styles of the "Regras da mesa" page (RN-24); the rest of the ideas (from the personas of question 76) are kept in the story.
- **MR-047** — New on 05/10/2026 (question 79): the MVP already has six puzzle types, hints, the hint by skill check, split information, consequences and "Ao resolver" (MR-038); the rest of the ideas are kept in the story.
- **MR-023 / ADR-0011** — ADR-0011 is referenced "as a proposal" for the multi-master deletion rule.

### Product: rules RN-01 to RN-12

- **RN-01 (exception: guided level-up)** — while the character "Pode subir de nível", the player may edit the sheet only to add the next level (MR-040, before the MVP). Samuel answered 03/10/2026, question 48. Delivered in Etapa 8 (slice 8.13 server, 8.15 screen).
- **RN-01 (created-later character, story lock)** — a character created later stays editable until the next session; the story has its own lock released by the master. Vinicius answered 29/09/2026. Status: Decidido.
- **RN-01 (delivery)** — sheet lock and `CharacterBlocked` delivered in Etapa 4.
- **RN-01 (source anomaly)** — the old RN-01 cell had a paragraph about trap damage (MR-035, Etapa 9 slice 9.8) spliced into the middle of a word ("favoritos"); it was moved to RN-02 in the new text.
- **RN-02** — the master has the final word on HP and slots. Samuel decided 29/09/2026. Master correction implemented in Etapa 5; automatic calculations arrived with combat (Etapa 6, on main since 03/10/2026; `rules/combat` slice 6.1 on 02/10/2026; combat damage 6.4a; spells, resources and healing 6.4b). Trap damage: Etapa 9 slice 9.8; creatures: slice 9.9 (MR-037).
- **RN-02 (question 28)** — a player sees only their own character's numbers, not the others'; the master sees everyone's. Answered 02/10/2026.
- **RN-03** — one character per campaign; a dead character is kept; copy for another campaign. Samuel decided 29/09/2026. Implemented in Etapa 4 (except the copy, MR-021).
- **RN-03 (question 39)** — the third failed death save does not kill alone; the master confirms (`ConfirmDeath`, same effect as `MarkCharacterDead`). Decided 02/10/2026, delivered in slice 6.4b; screens in slice 6.5c.
- **RN-04** — reusable NPCs. Samuel decided 29/09/2026; Etapa 4 model: owner `characters.master_user_id`, one campaign.
- **RN-05** — roles per campaign (the backend already supported it; became a closed rule). Samuel decided 29/09/2026.
- **RN-06** — session start notice, no browser push in the MVP. Samuel decided 29/09/2026; server part delivered in Etapa 5.
- **RN-07** — an invite is not the session link. Samuel decided 29/09/2026.
- **RN-08** — import format: D&D Beyond or Portuguese sheet, both as fillable PDF, no DOCX. Samuel decided 29/09/2026.
- **RN-09 (XP by gold)** — decided by Samuel 29/09/2026.
- **RN-09 (questions 44-47)** — by enemies, the master chooses CR or typed XP and decides at the end of the combat who receives (44, 45); the split rounds down and the server reports the loss (46); XP from treasure converts when the group returns to town (47); milestones are planned beforehand (45). Samuel answered 03/10/2026.
- **RN-09 (questions 44-50)** — CR optional and the XP field accepts any number (44); the list of characters sent by the master divides (45); rounding down (46); the milestone mark lasts until the sheet's level rises, the level-up notice (49); everyone in the campaign sees the history (50). Samuel answered 03/10/2026.
- **RN-09 (RN-24)** — the XP mode can change after the campaign is created (`SetCampaignXpMode`, `XpModeChangeBlocked`). Etapa 10 slice 10.4a; RN-23 to RN-29 decided by Vinicius 05/10/2026, defaults adjusted by questions 76-86 answered 05/10/2026.
- **RN-09 (MR-044)** — the generated treasure counts only gold, magic items never become XP. Etapa 10 slice 10.10b.
- **RN-09 (delivery)** — XP tables and formulas: Etapa 7 slice 7.1; XP awarded (`progression`): slice 7.2; screens: slice 7.4; XP from treasure ("Voltar à cidade"): Etapa 9 slice 9.11 server, 9.18 screen; planned milestones: Etapa 8 slice 8.10.
- **RN-10 (question 32)** — images are served only while the player sees them and the browser asks again on every use (accepted 02/10/2026); "Deixar com os jogadores" also decided in question 32.
- **RN-10 (question 62)** — the NPC portrait is downloadable only while the NPC is on the stage of the open scene. Samuel decided 03/10/2026 (Etapa 8, MR-031).
- **RN-10 (question 61)** — a scene is "discovered" when the master reveals the point or opens it in a session. Samuel decided 03/10/2026 (MR-029, MR-030).
- **RN-10 (question 63)** — any scene point opens, even without actions; the scene belongs to the master and the server no longer refuses. Samuel decided 03/10/2026. NPCs in the scene delivered in slice 8.6 (MR-031).
- **RN-10 (D5, D6, D9; questions 67-75)** — traps, treasures, lights, layers and fog per player. Accepted 04/10/2026 (Etapa 9 slices 9.3, 9.4, 9.5, 9.7, 9.8, 9.11; Etapa 10 slices 10.6d and 10.10b; systematic leak test `TestLeakMatrix` in Etapa 10).
- **RN-10 (delivery)** — Etapa 5 delivered the base with the maps (MR-008, MR-009); Etapa 7 the RP scenes (MR-015). Status: Decidido.
- **RN-11** — only the master reads and edits master notes. Vinicius answered 29/09/2026.
- **RN-12 (question 48)** — before the MVP the player may also level up through the guided sheet edit allowed by RN-01; the full MR-017 screen stays after the MVP. Samuel decided 03/10/2026.
- **RN-12 (question 49)** — who sees the level-up notice: the master and the player. Samuel decided 03/10/2026.
- **RN-12 (delivery)** — `next_level_xp` slice 7.1; `can_level_up` / `level_up_reason` slice 7.2; screens slice 7.4; guided level-up slice 8.13; level-up screen slice 8.15 (Etapa 8).

### Product: rules RN-13 to RN-19 and flows

- **RN-13** — Several masters per campaign and handing a campaign over. Samuel decided 29/09/2026. ADR-0011 was "as proposal"; the old consequence note (deleting the creator's account deleted the whole campaign) was replaced by "deleted only when the last master leaves".
- **RN-14** — Creating a campaign needs a full master account (Google only in the MVP); confirms ADR-0009's proposal for the MVP. Samuel decided 29/09/2026.
- **RN-15** — Invite with approval; pending character approved/rejected by the master. Samuel decided 29/09/2026.
- **RN-15** — The invite chooses whether it requires approval (`requires_approval`), a rejection deletes the character and the pending membership, the 30-day expiry of a pending membership without a character (question 24) and the ordinary invite approving the pending player's character (question 25). Samuel decided 02/10/2026; implemented in the server the same day.
- **RN-15** — Implemented in Etapa 4 (MR-024); the master's screen for pending members came in Etapa 6 (PR #51). The pending member's access to `GetTableRules`/`GetAbilityRolls`/`RollAbilityScores` is refused with `not_found` per RN-24 (06/10/2026).
- **RN-16** — Account deletion: players' characters stay with the master, master has 30 days to return, inactivity default 1 year. Treatment accepted by Samuel 29/09/2026. The character side landed in the database in Etapa 4 (`ON DELETE SET NULL`, `CASCADE`, TTL).
- **RN-17** — Player login without Google: per-table handle (master nickname + player nickname), no password at first, password or Google link after the first 30-day session. Samuel decided 29/09/2026, including the password (ADR-0009 option 3).
- **RN-18** — Physical or app dice: master chooses whether players choose. Samuel decided 29/09/2026. "When not allowed, everyone uses the type the master picks" was our proposal.
- **RN-18** — The player types the sum of the dice, without the modifier (question 38). Samuel decided 02/10/2026.
- **RN-18** — Setting and preference (`dice_mode`, `dice_preference`) built in Etapa 6 slice 6.2; combat rolls from slice 6.4a; attack screens slice 6.5b; spells and the rest slice 6.4b; the 4d6 abilities of a new sheet joined in Etapa 10 slice 10.4a (RN-24); RP scene rolls in Etapa 7 (screens 7.5).
- **RN-18** — "Each player chooses" means a choice at every roll; only a master-forced mode bars the other (`WRONG_DICE_MODE`). Decided 03/10/2026.
- **RN-19** — Each identical NPC rolls its own initiative. Samuel decided 02/10/2026 (question 33). Screens came in slice 6.5.
- **RN-19** — Shared turn for combatants with the same total (players and NPCs). Samuel decided 04/10/2026 (question 56), Etapa 8.
- **"Discussões que podem mudar estas regras" (section dropped)** — nothing open. The 28/09/2026 conversation about classes and races closed on 29/09/2026: the table uses all base D&D 5e classes and races, and Samuel accepted the rules as data, with formulas in the Expr library (ADR-0008). The automatic HP and spell-slot calculation (part of RN-02), MR-004, MR-013, MR-014, MR-015, MR-016 and MR-017 use that engine.
- **RN-13..RN-19 "Situação" column** — all marked "Decidido pelo Samuel" (RN-13, 14, 16, 17, 18 on 29/09/2026; RN-15 on 29/09/2026 plus 02/10/2026 details; RN-19 on 02/10/2026, question 33).

### Product: rule RN-20

- **RN-20 (state word for NPC HP, hidden combatants)** — decided by Samuel on 02/10/2026 (questions 34 and 35).
- **RN-20 / MR-015 (RP scenes)** — Samuel confirmed on 03/10/2026 (questions 51 to 55) what was built in Etapa 7 (actions chosen by the master, the scene opens in the session for everyone, the log belongs to the master and each player sees their own) and changed two points.
- **RN-20 / MR-015 (DC per scene)** — Samuel answered "leave both options, for the master to decide" on 03/10/2026, question 52: the point has the "Mostrar a CD aos jogadores" switch, off by default. Implemented on the server in slice 8.9 and on screen in slice 8.14.
- **RN-20 / MR-015 (attempts)** — Samuel answered on 03/10/2026, question 55: "the master must be able to define this attempt limit per player and per roll type; the system must always prioritise customisation for the master". Result: a per-action limit (`max_attempts`) and "Dar mais uma tentativa".
- **RN-20 / MR-014 (HP-reading spells)** — decided on 03/10/2026, question 58 (resolved by the server with real HP, player sees only words).
- **RN-20 / MR-029 (clues)** — Samuel decided on 03/10/2026, question 59: the master picks recipients, the screen pre-selects nobody, players agree at the table what to share.
- **RN-20 / MR-029, MR-030 (hooks, clues, notes)** — Samuel decided on 03/10/2026, question 60.
- **RN-20 / MR-031 (NPCs on stage)** — Samuel decided on 03/10/2026, questions 62 and 63.
- **RN-20 / MR-032 (combat highlights)** — Samuel decided on 03/10/2026, question 64.
- **RN-20 / MR-032 (planned milestone reached then undone stays in the XP history)** — accepted limit, slice 8.10; based on question 50 (everyone sees the history) and ADR-0007.
- **RN-20 / MR-037 (character's creatures)** — question 74, decided by Vinicius on 04/10/2026.
- **RN-20 / MR-036 (combat on a fog map)** — design D6 and questions 67 and 72, accepted on 04/10/2026.
- **RN-20 (joint turn)** — delivered in Etapa 8 on 04/10/2026.
- **RN-20 (what is not hidden: tie in a mixed group, numbered copies, combat revision)** — accepted by the design for now.

### Product: rules RN-21 and RN-22

- **RN-21 (grid movement, base rule)** — Each square is 1.5 m and movement is a circle (straight line between centres). Samuel decided 02/10/2026 (questions 36 and 37).
- **RN-21 (the circle)** — The "circle" model for movement was decided by Vinicius on 03/10/2026, in place of the answer to question 36.
- **RN-21 (difficult terrain, walls, other creatures, opportunity attack, cover on the map)** — Official rules apply: difficult terrain +1.5 m per square, painted walls block movement and sight, passing through a non-enemy costs as difficult terrain, an enemy's space only with two sizes of difference, nobody ends a move in another's space, leaving an enemy's reach provokes an opportunity attack (not after Disengage), cover half +2 / three-quarters +5 computed per attack. Vinicius, answering for Samuel, 04/10/2026 (questions 68 to 70).
- **RN-22 (conditions and concentration)** — MVP only marks conditions and concentration and reminds the concentration check; effects are not applied automatically. Samuel answered 02/10/2026 (question 42).
- **RN-22 (condition name)** — `condition:prone` is called "Derrubado". Samuel answered 03/10/2026 (question 43).

### Product: rules RN-23 to RN-25

- **RN-23** — Table content (class, subclass, race, subrace, background, spell per campaign) decided. Vinicius decided 05/10/2026.
- **RN-23** — A content change takes effect on locked sheets too (question 80). Answered 05/10/2026.
- **RN-23** — What players see: every playable option whole, with a master switch per option (question 83). Answered 05/10/2026.
- **RN-23** — The target and area of table spells decided. Vinicius decided 06/10/2026.
- **RN-23** — A table area spell in combat works like the SRD's: the caster picks the creatures, no area drawn on the map. Decided 06/10/2026.
- **RN-23** — The "Outro" background follows the SRD 5.1 "Customizing a Background" rule (question 82). Answered 06/10/2026.
- **RN-24** — Table rules decided. Vinicius decided 05/10/2026.
- **RN-24** — The table style (Tudo no app / Mesa física / Teatro da mente / Personalizado) (question 76). Answered 05/10/2026.
- **RN-24** — Level-up hit points rule: roll, average or player chooses (question 77). Answered 05/10/2026.
- **RN-24** — Ways of making abilities, chosen by the master (question 78). Answered 05/10/2026.
- **RN-24** — Draft edit of the player's base values is checked against the recorded way (`ability_origin`). Decided 06/10/2026.
- **RN-24** — Manual ability bonuses may not get around the way (`EXTRA_BONUSES`). Decided 06/10/2026.
- **RN-25** — The grid calibration and combat without a grid decided. Vinicius decided 05/10/2026.
- **RN-25** — Grid calibration (question 84) accepted 05/10/2026.

### Product: rules RN-26 to RN-30

- **RN-26** — Doors as a map layer (none/open/closed/locked/portcullis/secret) was an engineering decision of the Etapa 10 plan. Vinicius decided 05/10/2026.
- **RN-27** — The first three puzzle types (lights, combination lock, rotating symbols); with question 79 (answered the same day): "Ao resolver", hints, skill-check hints, split information, consequences, riddle, sequence and cipher. Vinicius decided 05/10/2026, question 79.
- **RN-27** — The limits and the way the sequence is played (1.2 s per step etc.) are engineering choices of the 10.7b slice (design E10-12), not questions for Samuel. Engineering, 05/10/2026.
- **RN-28** — Using the Gemini API with an AI Studio key. Vinicius decided 05/10/2026.
- **RN-28** — The three modes (scene art, isometric view, textured map) and the choice of enemies, answered and corrected 05/10/2026, question 85. Vinicius answered 05/10/2026, question 85.
- **RN-28** — The isometric view and the scene art made from a map show only what the players see now. Vinicius decided 06/10/2026.
- **RN-29** — Monsters in combat as hidden NPCs, with RN-20 unchanged. Vinicius decided 05/10/2026.
- **RN-29** — Average hit points for monsters (question 81) accepted. Vinicius answered 05/10/2026, question 81.
- **RN-30** — Status "Proposta" (security audit of 07/10/2026, items S3 and S4): the numbers (10 campaigns per account, 100 images per day, the `CAMPAIGN_CREATORS` list) are ours; Samuel confirms or changes them. Still open. Origin: security audit 07/10/2026.

### Product: glossary

- **Glossary: Attempts (of a scene action) / MR-015** — Attempts per scene action: 1 by default, 1 to 5 or unlimited, master can grant one more. Samuel decided 03/10/2026, question 55. Server built in slice 8.9, screens in 8.14.
- **Glossary: Show the DC to players / MR-015, RN-20** — Per-scene switch, off by default; the master always sees the DC. Samuel decided 03/10/2026, question 52. Server built in slice 8.9, screens in 8.14.
- **Glossary: Session summary / MR-032** — What the master and the players see in the summary (question 64).
- **Glossary: Treasure / RN-09, MR-041** — Treasure as a map point converted to XP. Samuel decided 03/10/2026; details by Vinicius 04/10/2026, question 75. Map stores and marks treasure (slice 9.3); XP conversion on the server (slice 9.11, MR-041) and on the screen (slice 9.18).
- **Glossary: Return to town / MR-041, RN-09** — One gold-mode award, 1 XP per gp, equal split rounded down among chosen living characters, button next to "Dar XP" with the arithmetic written. Vinicius decided 04/10/2026, question 75. Server slice 9.11, screen slice 9.18.
- **Glossary: Prone (Derrubado)** — The SRD condition `condition:prone` is called "Derrubado", not "Caído", to avoid confusion with a character at 0 HP. Samuel decided 03/10/2026, question 43.
- **Glossary: Opportunity attack / RN-21** — The server makes an offer to whoever controls the enemy; the move lands at once and the mover's turn waits. Decided in question 69 (Vinicius, 04/10/2026).
- **Glossary: Campaign document / MR-018** — In the MVP only the master reads and edits the campaign document. Decided 02/10/2026, question 27.
- **Glossary: Ability (habilidade)** — Term changed from "atributo" to the official "habilidade" of the Brazilian editions; "aumento de atributo" became the feature "Incremento no Valor de Habilidade". Vinicius decided 07/10/2026.
- **Glossary: Spell level (nível da magia)** — Term changed from "nível"/"círculo" to the official "nível da magia". Vinicius decided 07/10/2026.
- **Glossary: Saving throw (teste de resistência)** — Term changed from "salvaguarda" to the official "teste de resistência". Vinicius decided 07/10/2026.
- **Glossary: Official names** — Portuguese names aligned to the Brazilian editions of the Player's Handbook and Dungeon Master's Guide on 07/10/2026, content revision `fx.18`. Samuel's decisions win when there is one ("Derrubado"); the general terms (habilidade, nível da magia, teste de resistência) were Vinicius's decision of 07/10/2026.
- **Glossary: Custom background** — Free-text background follows the SRD 5.1 rule "Customizing a Background", question 82; built in slice 10.2.
- **Glossary: Token / MR-008** — Player tokens are born visible, NPC tokens hidden. Decided 02/10/2026, question 31.

- **Glossary: Shown image (MR-028)** — A shown image disappears from the players' screens when the GM stops showing it, unless the GM left it with them. Samuel answered 02/10/2026, question 32.
- **Glossary: Joint turn (MR-013, RN-19)** — Combatants with the same initiative total, players and NPCs, play a single joint turn, replacing the old "joint attack" reminder. Samuel decided 03/10/2026, question 56.
- **Glossary: Trap (MR-035)** — Trap visibility to players (only what was noticed, found, revealed or triggered; never DCs or effect) and the related trap rules. Decided in questions 71 and 73 (04/10/2026, Vinicius for 67-75). Delivery: map storage and who knows the trap in slice 9.3; in-play behaviour (noticed, found, triggered, disarmed) in slice 9.8.
- **Glossary: Fog of war (MR-036)** — Fog-of-war behaviour (per-player view, GM sees all, group vision option) decided in questions 67 and 68.
- **Glossary: Group vision (MR-036)** — Group vision is a per-map GM option, off by default. Decided in question 67.
- **Glossary: Cover (MR-034)** — Cover rules (marked on the map, computed by the server, higher of map and manual cover applies, degrees do not add up). Decided in question 70.
- **Glossary: Difficult terrain (MR-034)** — Movement-cost rules (flying ignores map rubble, creature spaces count). Decided in question 69.
- **Glossary: Side (MR-034)** — A GM can mark an NPC "Aliado" so it fights on the party's side. Decided in question 69.
- **Glossary: Character creature (MR-037)** — Character creatures act in combat with their own sheet from the SRD creatures; the Find Mount spell is left out. Decided in question 74 (04/10/2026, Vinicius for 67-75).
- **Glossary: Planned milestone (RN-09, MR-016)** — Planned milestones in milestone campaigns. Samuel decided 03/10/2026; delivered in slice 8.10.
- **Glossary: Can level up (RN-12)** — The full level-up screen was originally left for after the MVP, with player-guided sheet editing coming before it; the latter is now what "Level up from the sheet" does.
- **Glossary: Level up from the sheet (RN-01, RN-12, MR-040)** — Guided editing of the locked sheet before the MVP. Samuel decided 03/10/2026. Server delivered in slice 8.13 (Etapa 8), screen in slice 8.15.
- **Glossary: What changed (MR-040)** — No veto by the GM on level-ups; the GM is notified, reads and corrects. Decided by Vinicius 04/10/2026 (question 66), propagated in #95.
- **Glossary: Table content (rules engine)** — The `Content.With` engine exists since slice 10.1b; storing and editing came in later Etapa 10 slices.
- **Glossary: Table style (RN-24)** — Server delivered in Etapa 10, slice 10.4a.
- **Glossary: Options for the players (RN-23)** — Server (`SetOptionSwitches`, `ListOptionSwitches`) delivered in Etapa 10, slice 10.1d; screen in slice 10.11c.
- **Glossary: Spells (reference) (MR-045)** — Server `ListSpells` delivered in slice 10.2; screen in slice 10.11.
- **Glossary: Table rules (RN-24)** — Server delivered in Etapa 10, slice 10.4a; critical hit and death saves stored then and applied in combat in slice 10.4b.
- **Glossary: Theatre of the mind (RN-25, ADR-0017)** — Server delivered in Etapa 10, slice 10.5b; screen in slice 10.13b.
- **Glossary: Door (RN-26)** — Server delivered in slice 10.6c; screens (editor, session, combat) in slice 10.14a.
- **Glossary: Puzzle (MR-038, RN-27)** — Server delivered in slice 10.7a (first three types) and 10.7b (riddle, sequence, cipher, skill hint, split information, consequences). Mural design E10-06.
- **Glossary: Bestiary (MR-042, RN-29)** — "Criar NPC" delivered in slice 10.17a; "Pôr no combate" server in slice 10.9b, screen in slice 10.17b.
- **Glossary: XP budget (MR-043)** — Delivered on the server in slice 10.9c and on screen in slice 10.17b (the encounter builder).
- **Glossary: Individual and hoard treasure (MR-044)** — Server delivered in slice 10.10b, screen in slice 10.17c.
- **Glossary: SRD 5.2.1 values** — The Spell Scroll is worth the full value (not halved as a consumable): an engineering decision of 06/10/2026.
- **Glossary: Generated dungeon (MR-010)** — Server delivered in slice 10.6d; screens in slice 10.14b.
- **Glossary: Generated image (MR-039)** — Was "planned for Etapa 10" in the old glossary.
- **Glossary: Area spell** — A table-content spell may have an area (06/10/2026).

### Privacy

- **Privacy / controller and DPO** — Samuel is the controller, Vinicius is the data protection officer (encarregado), and the contact channel is a dedicated e-mail address until the domain exists. Samuel decided 29/09/2026.
- **Privacy / RN-17 player login** — Players do not need to give an e-mail or real name; they sign in with an anonymous login (master's nickname plus the player's nickname), without a Google account. Samuel decided 29/09/2026.
- **Privacy / minors** — The MVP is for people aged 18 or older, by self-declaration. Samuel decided 29/09/2026.
- **Privacy / scope of the MVP** — Only our own table in the MVP: not open to other tables, no charging; opening it brings the decision back (processing-agent size class, GDPR analysis). Samuel decided 29/09/2026.
- **Privacy / ADR-0010** — "Privacy by design, LGPD as the base and GDPR as the yardstick" is still a proposal until Samuel accepts it (stage info, kept in the doc as a plain statement).
- **Privacy / inventory, RN-17 rows** — Handle, password, re-entry link and identity-log rows belong to the Google-less player login. Samuel decided 29/09/2026; not implemented yet (kept in the doc as current stage).
- **Privacy / inventory, pending membership** — A pending membership with no character is erased 30 days after joining by the database TTL on `pending_expires_at`. Decided and implemented 02/10/2026, question 24.
- **Privacy / inventory, XP history (MR-016)** — The XP history is visible to the whole campaign, master and players. Samuel answered 03/10/2026, question 50.
- **Privacy / no browser storage** — No `localStorage`, `sessionStorage`, IndexedDB, JS-written cookie or Worker holding a token or personal data, without exception. Vinicius's rule.
- **Privacy / legacy app** — `users.name`, `characters.email` and `sheet.playerName` are not migrated to the new schema. Decided 29/09/2026.
- **Privacy / invite accepted through sign-in** — The invite token travels in the body of `POST /auth/login` and only its hash is stored in the sign-in state; no `localStorage`, `sessionStorage` or service worker. Vinicius decided 29/09/2026.
- **Privacy / MR-018 campaign document** — The document is readable only by the master (players get `permission_denied`, non-members and pending members `not_found`). Decided 02/10/2026, question 27.
- **Privacy / MR-028 shown image** — The player downloads a shown image only while it is shown (afterwards no access) or while the master leaves it with the players. Answered 02/10/2026, question 32. The check "left images store only IDs and the master removes each one" was done in Etapa 6.
- **Privacy / ExportMyData and deletion preview** — They come in the privacy PR with the `PrivacyService`. Decided 29/09/2026.
- **Privacy / delivery notes removed** — Etapa 4 (screens warning "é ficção"), Etapa 5 (live session), Etapa 6 (left images check), Etapa 7 (RP scene rolls, XP events), Etapa 8 (clue reveals, planned milestones fatia 8.10), Etapa 9 (traps, treasure, light, fog `seen_by`), Etapa 10 (puzzles, table content), fatia 6.4a/6.4b (combat rolls in `session_events`), fatia 10.9c (saved encounters) were labels of when each row/bullet was added.

- **Privacy / Delete the account (RN-16)** — Player deletion is immediate with characters kept for the master; master accounts wait 30 days, restorable by signing in with the same Google account. Samuel decided 29/09/2026.
- **Privacy / Delete the account** — The "delete at" mark on the account, checked at sign-in, for the master's 30-day hold (instead of deleting the row at once), with session events storing only IDs. Samuel accepted the design 29/09/2026.
- **Privacy / Tension between keeping the character and erasing the identity** — The four-step treatment (character passes to the master, warning before confirming, option to delete instead, free text not filtered). Samuel accepted 29/09/2026.
- **Privacy / Processors (Render, GitHub Pages)** — `server/` of the legacy app will be removed from the repository. Samuel decided 29/09/2026.
- **Privacy / The legacy app database** — The legacy app's CockroachDB can be deleted, after a one-off import of only the characters. Samuel decided 29/09/2026.
- **Privacy / Minors** — Question from Samuel (29/09/2026): what is needed under LGPD and GDPR if players or masters are under 18. Answered the same day in the progress doc with the ADR-0010 (section 8) recommendation: MVP for over-18s by self-declaration, minors play without an account, lawyer + ECA Digital assessment + age check before opening to the public. Samuel accepted 29/09/2026; Vinicius confirmed the same day that there are no minors at the table today.
- **Privacy / To be defined (CockroachDB plan)** — Keep the current legacy Unlimited plan; migration to Cloud SQL or another product under evaluation. Samuel decided 29/09/2026.
- **Privacy / To be defined (MR-027, rule PDFs read with AI)** — The PDF is not stored; it exists only while being processed and is deleted right after, with a short TTL on the stored file as a safety net. Decided 02/10/2026, question 22.
- **Privacy / To be defined (MR-039, AI images from a map)** — Use the Gemini API (Google AI Studio key, paid-services terms) instead of Vertex AI. Vinicius decided 05/10/2026. The reference images may include the map of the scene and the NPC and enemy portraits (question 85). The planned item was planned for Etapa 10, slice 10.8; it is now built and described in "What generated images send to Google".
- **Privacy / Image shown to players (MR-028, RN-10)** — The player downloads the image only while it is shown (question 32, answered 02/10/2026: afterwards, no access) or while the master leaves it with the players ("Deixar com os jogadores" switch). The check requested in the privacy text was done in Etapa 6.
- **Privacy / Minors (delivery note)** — "Characters already follow this design in the database" was delivered in Etapa 4 (history removed from the text, behaviour kept).

### Architecture

The decisions that the former architecture page carried: who decided what, when, and the incidents behind some rules. The current page states the resulting design.

- Transactions and the pool: the "no read through the pool inside a transaction" rule came from a CI deadlock (PR #120): `MoveCombatant` read map terrain through the pool inside the transaction and, with four other requests, stayed stuck for 176 s.
- Contracts: idempotency levels by request kind and AIP naming were decided by Vinicius on 29/09/2026. The `idempotency_key` mechanism for every create call comes from the 07/10/2026 audit (finding F7, AIP-155).
- Identity: the player sign-in without Google (RN-17) was decided by Samuel on 29/09/2026 (per-table handle, no e-mail; ADR-0009), not implemented. The NestJS legacy server's removal was decided by Samuel on 29/09/2026.
- Hosting: server and database in the same provider/region was confirmed by Samuel.
- Invites: default of one use and 7 days is RN-07 (decided 29/09/2026). Approval invites (RN-15, MR-024) were implemented in Etapa 4. Accepting an invite through sign-in with nothing in the browser was decided by Vinicius on 29/09/2026 (ADR-0009). `identity.SignedIn` as a capability (no exported function acts as an arbitrary user) was decided by Vinicius on 29/09/2026; `ContextWithSession` was removed for the same reason.
- Session: the "Sign out of other devices" revocation (ASVS 5.0 V7.4.3) leaves open streams alive up to 60 s (finding S12 of the 07/10/2026 security audit; accepted, because the member only receives what they could already see, RN-10).
- Pending member: the `pendingMayCall` list gained the two 4d6 calls (`GetAbilityRolls`, `RollAbilityScores`) on 06/10/2026, because they are part of creating the character. Pruning of a pending member who never creates a character (30 days) is question 24.
- Campaign document: "master only" read access is question 27 (answered 02/10/2026) because the document holds spoilers.
- Creatures/summons: Find Steed (Convocar Montaria) is out of the MVP (question 74). Conjure Animals' single shared initiative is SRD rule; applying the same to Animate Dead is our decision. HP of a monster as the creature's average by default (question 81). Which creatures see through their owner (only the familiar, only while its eyes are on) was decided in slice 9.10 with the rationale kept in the architecture doc.
- Slice 9.10 review round: the "druid in Wild Shape" form is a separate table (`character_wild_shapes`) after a first design with columns in the vitals row hung `TestShieldTurnsAHitIntoAMiss` for minutes (read waiting for its own transaction's write).
- ADR notes taken out of the architecture doc: ADR-0011 (who may do what) gains the creature rows (master and owning player read, rename and dismiss; only the master gives and corrects HP; another player gets `not_found`; AssumeWildShape/LeaveWildShape/StartFamiliarSight/StopFamiliarSight are owner and master only, placing/removing a creature token is master only). ADR-0007 (events) gains the payloads of `creature_summoned` (`character_id`, `creature_ids`, `monster_keys`, `source`, `ritual`, `summon_roll` in combat), `creature_dismissed` (`character_id`, `creature_ids`, `reason`: owner/master/defeated/concentration), `wild_shape_started`, `wild_shape_ended` (written before the damage event and not closing undo), `familiar_sight` (ending by turn or by will is not undoable).
- Multiattack counts: the 5e-database snapshot is wrong for some creatures; corrections live in `effects/corrections.json` (content revision fx.15). Attack names in Portuguese: revision 11 brought 23 names, 13 the missing ones.
- Opportunity attacks: provoking/offers design resolved by question 69. Spell order on the turn options: question 57.
- Dungeon generator: clean room (ADR-0015); the specification and origin note live with the generator's PR as comments; a third person audited it (private ADR). Trap search rules (disadvantage in poor light, two d20): question 71.
- ADR-0018 (05/10/2026) moved the table content (`ContentService`) from `campaigns` (ADR-0008) to `characters`; the revision lives in `campaign_content_state` in `characters`, not in `campaigns.content_revision` as the ADR first said. The `campaign_id` in content requests was decided by Samuel on 29/09/2026 (content is per campaign).
- "A classe mudou" is question 80 (E10-02 state 8); players read table entries in full, with numbers and effects (question 83); the "Outro" background follows SRD 5.1 "Customizing a Background" (question 82); level-up without approval or veto from the master (question 66, decided by Vinicius 04/10/2026); who may level up (question 48).
- RP scenes: questions 51 to 55 (Samuel, 03/10/2026): the action types, showing the DC per scene (52), opening any scene point even without actions (63, changed in Etapa 8), the attempt limit per action and player (55). Clues/discoveries/hooks: questions 59 to 61; the NPC portrait on the sheet: question 62. "No hide-again for clues" and "the clue row keeps a copy of the text" are decisions of MR-029.
- Left-images list (MR-028): question 32 (02/10/2026). Live vitals visibility: question 28 (02/10/2026): the player sees only their own vitals.
- Stream: `max-instances = 1` while fan-out is in memory (ADR-0005); a shared channel replaces the hub if one instance is not enough.
- Combat: the Shield reaction design is "decision 5 of the timeline"; "Caído"/"Morrendo" words follow question 39 (the app never says someone died before the master confirms); spells that read HP is question 58; combat highlights and session summary are question 64 (Samuel, 03/10/2026). Death saves: the first save is at the start of the next turn if the character fell during their own turn (SRD 5.1).
- "Mais tesouro encontrado": the summary reads live treasure data (not events) because re-marking who found it writes no event; the cost (editing a treasure later changes an old summary; converted treasure locked) was accepted.
- XP: question 50 (progression design), 45 (the screen proposes who gets XP; the server does not decide who fought), 46 (split rounds down, remainder lost), 48 (milestone level-up), 75 ("Voltar à cidade"), 66 (no master veto on level-ups).
- Images: the rule that a player downloads an image only while they see it was decided on 30/09/2026 when MR-028 was integrated (before, any member downloaded any campaign image by ID). The player-visible view for AI scene art is ADR-0019 (06/10/2026). Notes: the master does not read player notes (question 60); scene labels/discoveries questions 59 to 61.
- Gemini docs were checked on 06/10/2026 (limits, Interactions API, `Api-Revision: 2026-05-20`).
- Maps: 200 maps per campaign and 200 points per map are proposals (like the gallery quota). Scene point types (battle, sub-map, scene) and the scene's actions came from question 29 (02/10/2026). Light/fog design decisions D2, D5, D6, D7, D8 of the Etapa 9 plan; fog tiles ADR-0016. "A trap that triggered stays public forever" is a design decision of slice 9.3.
- Generated dungeons: fog on with bright base light, so a textured image still hides the room behind a secret door (MR-010, decided 06/10/2026). The 2 s generation deadline is the specification's proposal (§6). Treasure: the tables and names are ours (no book or site); the Spell Scroll is not halved (engineering decision of 06/10/2026); the screen uses the party's lowest living level (design E10-10); the daily gold target ratios (half / one twentieth of a typical-rarity item) are our choice tied to SRD 5.2.1 values.
- Encounter builder: question 86 (the party and extra_party NPCs), monsters sent in the StartEncounter request instead of the server reading the point (three reasons kept in the architecture doc), generator written clean-room (ADR-0015). Theatre of the mind: ADR-0017, RN-25.


### Data model

- **29/09/2026, schema from zero.** The new schema starts from zero: `00001_init` is empty and each module creates its own tables as it is built. There is no gradual migration of data from the old app.
- **29/09/2026, Samuel: the characters are the one exception.** The old app's database can be deleted after a single, one-off import of only the characters (for example Pensantus) into the new database. It is not a schema migration and not a tool kept in the repository: a one-time, hand-reviewed task tracked in the roadmap.
- **29/09/2026, Samuel: the old `server/` (NestJS) leaves the repository.** The old-app comparison table in the data page records what changed, so removing the code loses no context.
- **Etapa 4: character state.** A `status` column (`active`, `dead` or `pending`) next to `sheet_locked_at`; a dead character changes state, never row (RN-03); a pending one (RN-15, MR-024) is deleted when the master refuses it.
- **Question 24 (RN-15): a pending member without a character disappears after 30 days.** Implemented as `campaign_members.pending_expires_at` with a row-level TTL.
- **Question 27 (02/10/2026): the campaign document is read and written by the master only** (MR-018).
- **Question 31 (02/10/2026): every NPC is born hidden on the map and in combat**; the master reveals when he wants.
- **Question 46: XP is split among the characters and rounded down.** The remainder is lost, and the split line says so.
- **Question 48: the "Pode subir de nível" mark** lasts while the sheet's level does not exceed `level_at_mark`; it is compared on read and nothing is written when the master raises the level.
- **Question 51 (accepted by Samuel on 03/10/2026): at most 20 actions per scene point.**
- **Question 52: the "Mostrar a CD aos jogadores" switch** (`map_points.show_dc`), off by default.
- **Question 55 (decided by Samuel on 03/10/2026): attempts per player** (`scene_actions.max_attempts`, default 1, 0 for unlimited).
- **Proposal: 200 maps per campaign and 200 points per map** (and the gallery quota of 300 images and 500 MiB), like the gallery quota, checked in the `INSERT` transaction.
- **RN-13 (more than one master), still a proposal (ADR-0011):** changes `campaigns.created_by` from a plain `CASCADE` to a rule that first checks whether another master remains.
- **07/10/2026 audit, decision D-02:** the event types became the reference table `session_event_kinds` (migrations `00168` to `00170`), instead of a `CHECK` rewritten in ten migrations.
- **07/10/2026 audit, finding F7:** an idempotency key and request hash on every RPC that creates a row (`00178` to `00180`, plus `00179` for `characters.create_hash`).
- **A column on `characters` for the content revision was tried and dropped** (Etapa 10): the table has a row-level TTL, and any repeated `ALTER` rewrites the TTL expression, which `TestMigrationsAreSafeToRerun` sees as a schema change. The revision lives inside the sheet document instead.

### Operations

- **29/09/2026, Samuel: the database plan.** CockroachDB on Google Cloud, in the same region, on Samuel's current plan (the legacy Unlimited plan, bought before the 2024 licensing change). Changing plan loses Unlimited. Backups stay in São Paulo, at most 30 days, to meet the deletion deadline.
- **Proposal: 100 generated images per day** as the Google quota and as `IMAGE_DAILY_LIMIT`, and **20 per campaign per month** (`IMAGE_MONTHLY_LIMIT`), from the estimate of 3 to 6 images per 4-hour session and 4 to 5 sessions a month (US$ 1.34 per campaign per month at most). The per-session numbers are an estimate to be replaced after measuring with the real key.
- **The production ELK is Samuel's decision**, with Cloud Logging (free at the current volume, 30 days) as the alternative.
- **Prices checked:** Cloud Storage on 30/09/2026; the Gemini price page on 06/10/2026.
- **The upload-slot memory figure was corrected on 06/10/2026** (230 MiB measured earlier, 178 MiB after the thumbnail and reference started scaling in strips).

### Design

- **29/09/2026, Vinicius: the visual direction.** "Ficha de papel" is direction A of the three explored on 29/09/2026 (the three directions are in the second Claude Design link); Vinicius chose it. The visual system exists in Claude Design with the components drawn.
- **A screen is drawn in Claude Design and approved by Vinicius before the code** (a small fix inside the system needs no design).
- **PR #56 shipped misalignments** visible in its own screenshots (a floating label measured before it existed, leaving words over a magnifier icon); that is why every piece is reviewed up close at 2x, and why the alignment checks of `e2e/tests/layout.ts` exist (02/10/2026).
- **Question 63:** the reason "Sem ações" was removed from the scene picker; every scene opens, even without actions.
- **Question 59:** nobody starts checked in "Quem recebe a pista".
- **Question 82:** the "Outro" background lets the player customize the name, two skills, two tools or languages, a feature and the equipment (SRD 5.1).
- **07/10/2026: three terms were renamed in the UI text:** "atributo" became "habilidade", "círculo" became "nível" (da magia), and "salvaguarda" became "teste de resistência", to follow the official D&D translation.
- **Deviations from the Claude Design drawings** (where the app differs from a design, and why) are kept in the design page next to each screen as "What differs from the design"; the implementing slice decided them and they were not recorded separately.


## Open at the time of archiving

Nothing: no question was open. New questions go to the progress document ("MeuRPG — Como está o trabalho"), not to this page. The only item still marked as a proposal is RN-30 (abuse limits from the 07/10/2026 security audit: 10 campaigns per account, 100 images per day, the `CAMPAIGN_CREATORS` list), which Samuel confirms or changes; and the multi-master deletion rule of ADR-0011.
