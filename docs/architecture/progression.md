# Progression module: XP and milestones

The master gives XP to the party and records milestones, and the whole campaign reads the history and everyone's XP (MR-016, RN-09, RN-12).
The `progression` module owns the tables `xp_awards`, `xp_award_shares` and `xp_award_treasures` (see [Data](../data/README.md)) and the `ProgressionService` (`proto/meurpg/progression/v1/progression.proto`).
XP itself is a number on the sheet (`FullSheet.experience_points`), so `characters` is what changes it; `progression` asks through an interface.
The code is in `backend/internal/progression`.

## Who may call what

| Call | Master | Player | Non-member | Anonymous | Pending member (RN-15) |
| --- | --- | --- | --- | --- | --- |
| `AwardXP`, `MarkMilestone`, `UndoLastXPAward`, `ListTreasuresToConvert` | Yes | `permission_denied` | `not_found` | `unauthenticated` | `not_found` |
| `AddMilestone`, `UpdateMilestone`, `MoveMilestone`, `RemoveMilestone`, `MarkMilestoneReached`, `GiveMilestoneTo` | Yes | `permission_denied` | `not_found` | `unauthenticated` | `not_found` |
| `ListXPAwards`, `GetCampaignExperience` | Yes | Yes | `not_found` | `unauthenticated` | `not_found` |
| `ListMilestones` | Yes, all | Yes, only the reached ones | `not_found` | `unauthenticated` | `not_found` |

`TestAuthorizationMatrix` calls each method as each of these roles and fails if a new method is not in its table.

## Awarding XP

`AwardXP` takes one of three modes (RN-09), and the mode must fit the campaign's XP mode.

| Mode | The total | Accepted by |
| --- | --- | --- |
| `ENEMIES` | The sum of `xp_value` of the defeated NPCs of an **ended** combat; one award per combat unless undone | Enemies campaigns |
| `GOLD` | 1 XP per gp | Gold campaigns |
| `MANUAL` | The number the master typed | Enemies and gold campaigns |

A milestones campaign accepts none of them, only the milestone calls.
A mode the campaign does not take is `failed_precondition` with the `XPBlocked` detail and the reason `MODE_NOT_ALLOWED`, which carries the campaign's mode.
The other `XPBlocked` reasons are `ENCOUNTER_NOT_ENDED`, `ALREADY_AWARDED`, `NOTHING_TO_GIVE` (total 0, or too little to give each character 1 XP), `NOTHING_TO_UNDO` and `CHARACTER_NOT_ELIGIBLE` (not a living player character of the campaign: dead, pending, an NPC or from another campaign; the detail carries the ID).

- **The split.** `rules.SplitXP(total, how many)` rounds down (100 for 3 gives 33 each, and 1 is lost: the response carries `lost_xp`). The receivers are the list the master sends (`character_ids`, 1 to 40, no repeats): the server does not decide who fought, the screen proposes.
- **One transaction.** The award, its shares, each share added to `experience_points` (through `characters.AddExperience`, the `progression.Party` interface) and the session event are written together, inside play's funnel when it is connected (`SetSessionHost`): the open session's row is locked first, then the sheets, and the session's number moves once for an award or its undo that the open session logged. The sheet lock (RN-01) does not prevent it: it is a master's act. XP stays between 0 and 1,000,000 and the sheet's revision rises.
- **Idempotency.** Every write carries `idempotency_key`. The key lives on the award (and the undo's in `undo_key`), not in `session_events`, because an award exists with or without an open session. The same key changes and publishes nothing and returns what the first call did; the key of another change is `invalid_argument`.
- **Undo (ADR-0007).** `UndoLastXPAward` undoes only the last award that was not undone. The optional `expected_award_id` gives `aborted` if another award is already the last. Undo subtracts each share from the sheets (never below 0) or turns the marks off, and fills `undone_at` and `undone_by`. The award stays in the history with the label "Desfeito"; to undo an earlier one, undo the newer ones first.

## "Voltar à cidade" (MR-041)

Turning found treasure into XP is a `GOLD` award with `treasure_point_ids`, with no new kind of award or event.
`progression` never reads `maps` tables: it asks through `progression.Treasures`, which `maps.Treasures` implements (`backend/internal/maps/treasure_convert.go`, wired in `cmd/api`).

- **`ListTreasuresToConvert`** (master only, `IDEMPOTENT`) lists the treasures found and not converted, at most 100, oldest first (what one conversion accepts); `total` says how many exist. The finders' names come from `characters`, by ID.
- **Inside the award's transaction:** `LockForConversion` locks the points' rows (`SELECT … FOR UPDATE`) and `progression` checks each one: `not_found` if it is not the campaign's treasure, then `TREASURE_NOT_FOUND_YET`, `TREASURE_ALREADY_CONVERTED` and `TREASURES_OVER_LIMIT` (more than 1,000,000 gp in all). It stores the award with the summed gold, the shares and, in `xp_award_treasures`, the treasures; `MarkConverted` fills `map_points.treasure_converted_award_id`. Two simultaneous conversions wait for each other on the lock, and the second finds the treasure converted.
- **Undo** calls `Release`, which clears the link in the same transaction. Only the last award is undone, so a treasure converted by an award that is no longer the last stays converted; the master fixes it by hand.
- **What each reader gets (RN-10).** The history gives a player only `treasure_count` and `gold`; the `treasures` list (ID and gp of each) is the master's and survives undo. The `xp_awarded` and `xp_award_undone` events carry the same list, with no names. No API hands the raw event to a player.
- **After the commit** (never inside the transaction, so a retried or refused award publishes nothing), `Treasures.PublishChanged` sends `map_changed` once per map whose treasures were converted or freed, only if this call made the change. The converted state is the master's alone, so the event goes to the master only and carries the map's ID and nothing else.
- In an enemies or milestones campaign the mode is refused (`MODE_NOT_ALLOWED`); its treasure only shows on the map and in the summary.

## Milestones (RN-12)

In a milestones campaign the master writes the milestones beforehand and marks them reached.

- **`MarkMilestone`** stores a `milestone` award with shares of 0 XP and each character's total level at that moment (`level_at_mark`). Every award reads the living party inside its own transaction, so a character that died or levelled a moment before is judged as it is when the award is written.
- **Planned milestones (MR-016).** `planned_milestones` holds up to 100 per campaign, 1 to 120 characters each, in the master's order. `AddMilestone`, `UpdateMilestone`, `MoveMilestone` and `RemoveMilestone` make one change per call, each answering with the whole list; if that read fails after the commit, the answer is still a success (the failure is logged) with the written milestone (`Add`, `Update`) and an empty list, and the app reads the list again. Only a still-planned milestone changes (else `MILESTONE_ALREADY_REACHED`); the 101st is `resource_exhausted`.
- **Reaching one.** `MarkMilestoneReached(milestone_id, character_ids, idempotency_key)` and `GiveMilestoneTo` reuse `MarkMilestone`'s write: the same `milestone` award, with the milestone's text as the reason and `xp_awards.milestone_id` pointing to it. The milestone row is locked (`FOR UPDATE`), so two at once take turns. **Reached** is derived: having an award linked to it that was not undone.
- **Refusals.** All milestone writes check the campaign's mode (`MODE_NOT_ALLOWED`). Marking again is `MILESTONE_ALREADY_REACHED`; `GiveMilestoneTo` on a planned milestone is `MILESTONE_NOT_REACHED`, and on a character who already has it `CHARACTER_ALREADY_MARKED`. A milestone that ever had an award, even an undone one, is never removed (`MILESTONE_HAS_HISTORY`): deleting it would clear awards' `milestone_id`, and history is not rewritten. `ListMilestones` carries that fact as `has_history`.
- **A reused key must be the same request:** the same kind (a mark or "Dar a mais alguém", stored in `xp_awards.milestone_again`), milestone and characters, else `invalid_argument` and nothing is given.
- **`AddMilestone`'s key** is kept with a hash of the campaign and the cleaned text (trimmed, as stored), so a retry that differs only by the whitespace around the text is the same request.
- **`MoveMilestone`'s key** creates no row, so it lives in `planned_milestone_moves` (campaign, the scoped key, the hash of the request minus the key). Inside the move's transaction, after the list is locked, the same key and request answer with the list as it is, writing and publishing nothing; the key of another request (another milestone or direction) is `invalid_argument`. A call without a key is not deduplicated.
- **Undo** stays the last award's: with no live mark the milestone goes back to planned; undoing a "Dar a mais alguém" mark removes only that one.
- **Reading.** `ListMilestones` (any member) gives the master everything, in order, and the player **only the reached ones** (RN-20: the server does not build the planned ones for them). Each carries its live marks (`XPAward`: who, when, `can_undo` only for the master) and, last, the marks registered outside the list (`off_list`, whose ID is the award's). An XP-counting campaign answers `MODE_NOT_ALLOWED`.

## Reading the history and "Pode subir de nível"

- **`ListXPAwards`** pages from newest to oldest (`page_size` 1 to 50, default 20; opaque `page_token`) with who gave (display name), the mode, the reason, the combat's name, the shares (character name and XP), whether undone, and `can_undo` (only for the master, on the last). `play` is asked for the combats' names only when an award in the page has a combat.
- **`GetCampaignExperience`** gives each living player character's level, XP, the next level's XP, `can_level_up` and the reason (`XP` or `MILESTONE`).
- **On the sheet,** `GetCharacter` carries `can_level_up` and `level_up_reason`, only for the master and the owner, of a living player character. The rule is `levelUpReason` in `progression`: in an enemies or gold campaign, the sheet's XP reaching `DerivedSheet.next_level_xp`; in a milestones campaign, a milestone mark whose level has not yet been passed (the read compares; nothing is stored when the master levels up). Nobody levels up from level 20. `characters` asks `progression` through `characters.LevelUps` (wired in `cmd/api` with `SetLevelUps`).

## The live session

With an open session, each award, undo and milestone becomes a line in `session_events` (`xp_awarded`, `xp_award_undone`, `milestone_marked`) through the `progression.SessionLog` interface that `play` implements.
The payload has IDs and numbers only: neither the reason nor any name (the reason stays in `xp_awards`).
After the commit, `xp_changed` goes out, a contentless notice (field 14 of `WatchGameSessionResponse`) to everyone in the session; the app reads the history and XP again where it can.
Without an open session, XP is given all the same and nothing is stored in `session_events` or published.

## The NPC's XP (RN-20)

`challenge_rating` and `xp_value` are NPC sheet fields (`FullSheet` of enemy and boss, `BasicSheet` of minion).
The server checks the rating against the `rules` list and the XP from 0 to 1,000,000.
A player's sheet refuses both with `invalid_argument`; nothing is silently lost.
On entering combat, the NPC carries `xp_value` to `combatants.xp_value`, and only the master receives `Combatant.xp_value`.

## How the modules connect

`progression` declares these interfaces; the types that cross live in `progression/link`, and `cmd/api` wires everything.

| Interface | Implemented by | Used for |
| --- | --- | --- |
| `Party` | `characters` | The living party, names, `AddExperience` |
| `Combats`, `SessionLog` | `play` | The campaign's ended combat and its names; appending the event and `PublishXPChanged` |
| `Campaigns` | `campaigns` | `CampaignXPMode` |
| `Profiles` | `identity` | Display names |
| `Treasures` | `maps` | Converting treasure ("Voltar à cidade") |

In the other direction, `progression` implements `AwardedXP` for `campaigns` (how many awards stand and how much XP they gave), which the XP-mode change asks (RN-09), and `LevelUps` for `characters`.
No package imports another's code.

## In the app

The client is `web/src/app/core/progression`.

- `ProgressionClient` has one method per `ProgressionService` call and is loaded only by lazy code. The idempotency key is born once per action and goes again on a repeat.
- Errors become Portuguese by the code and the `XPBlocked` detail (`xp-errors.ts`), never by the message.
- `ExperienceStore` reads a campaign's XP and history. The campaign page ("Experiência" panel, the "Pode subir de nível" label), the session's "Dar XP" and the end of combat share it; a read overtaken by a newer one is discarded. Only for the master, in an XP-counting campaign, it also reads the treasures waiting (`load(id, withHistory, withTreasures)`), again on each `refresh`.
- `MilestonesStore` keeps the milestone list; `pages/campaign-detail/milestones` is the panel (planned, reached, characters, the "Marcar como alcançado" sheet). The player's campaign page does not listen to the stream, so it updates on reopening the tab.
- In the live session, `xp_changed` arrives through the same `LiveStream` and adds 1 to `XpChanges`; everything that depends on XP on the page reads again. The character sheet listens through `XpWatcher`, only while the campaign has an open session; without one, it reads on opening.
- The split the screen shows (`xp-math.ts`) and the gold summed for "Voltar à cidade" (`treasure.ts`) are only previews. What counts is the server's response (`xp_each`, `lost_xp`). The conversion is an `AwardXP` of mode `GOLD` with `treasure_point_ids` (the `town-sheet` dialog), the reason "Voltar à cidade" and one idempotency key per choice. No rule is born in the browser.
- The reason text and "O que aconteceu" carry the fiction notice ([Privacy](../privacy.md)).
