-- Every query names the campaign next to the character: a character ID of
-- another campaign matches no row, which the handlers answer as "not found".
-- campaign_id is nullable in the table (a deleted campaign sets it to NULL),
-- so the argument is cast to UUID to make it a plain string in Go.

-- name: CountCampaignCharacters :one
-- Every character of the campaign, of any kind and status (dead ones too),
-- for the cap on creating (RN-30). The NPCs the app makes for the creatures
-- of a combat (combat_only, RN-29) are not counted here: the master never lists
-- them and nothing deletes them. They have their own cap (CountCampaignCombatOnlyNpcs).
-- The index on campaign_id finds the rows.
SELECT count(*)::INT4 FROM characters
WHERE campaign_id = sqlc.arg(campaign_id)::UUID
  AND COALESCE(sheet->'basic'->>'combat_only', 'false') <> 'true';

-- name: CountCampaignCombatOnlyNpcs :one
-- The NPCs the app made for the creatures of a combat (combat_only, RN-29),
-- for their own cap (RN-30). The index on campaign_id finds the rows.
SELECT count(*)::INT4 FROM characters
WHERE campaign_id = sqlc.arg(campaign_id)::UUID
  AND COALESCE(sheet->'basic'->>'combat_only', 'false') = 'true';

-- name: InsertCharacter :one
-- status is 'active', or 'pending' for a character created by a pending
-- member (RN-15, MR-024). reserved is true for the character the master makes for a
-- player to claim (MR-049): no owner.
-- create_key (a UUID, unique in the campaign: characters_campaign_id_create_key_idx) and create_hash
-- are the idempotency key of CreateCharacter and the hash of its request; NULL when the call sent
-- no key. A retry reads the first character with GetCharacterByCreateKey.
INSERT INTO characters
    (campaign_id, kind, player_user_id, master_user_id, status, name, sheet, story, create_key, create_hash, reserved, created_at, updated_at)
VALUES (
    sqlc.arg(campaign_id)::UUID, sqlc.arg(kind), sqlc.narg(player_user_id), sqlc.narg(master_user_id),
    sqlc.arg(status), sqlc.arg(name), sqlc.arg(sheet), sqlc.arg(story), sqlc.narg(create_key)::UUID, sqlc.narg(create_hash),
    sqlc.arg(reserved)::BOOL, sqlc.arg(now), sqlc.arg(now)
)
ON CONFLICT (campaign_id, create_key) WHERE create_key IS NOT NULL DO NOTHING
RETURNING *;

-- name: InsertNpcFromCreature :one
-- The NPC of "Criar NPC" (MR-042). create_key is the dialog's idempotency key;
-- a second call with the same key inserts nothing (the unique index
-- characters_campaign_id_create_key_idx), and the caller reads the first one
-- with GetCharacterByCreateKey. kind and master_user_id as for any NPC.
INSERT INTO characters
    (campaign_id, kind, master_user_id, status, name, sheet, story, create_key, created_at, updated_at)
VALUES (
    sqlc.arg(campaign_id)::UUID, sqlc.arg(kind), sqlc.arg(master_user_id), 'active', sqlc.arg(name), sqlc.arg(sheet),
    sqlc.arg(story), sqlc.arg(create_key)::UUID, sqlc.arg(now), sqlc.arg(now)
)
ON CONFLICT (campaign_id, create_key) WHERE create_key IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetCharacterByCreateKey :one
SELECT * FROM characters
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND create_key = sqlc.arg(create_key)::UUID;

-- name: GetCharacter :one
SELECT * FROM characters
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND id = sqlc.arg(id);

-- name: GetCharacterForUpdate :one
-- FOR UPDATE locks the row until the transaction ends, so two edits of the
-- same character wait for each other instead of both reading the same
-- revision.
SELECT * FROM characters
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND id = sqlc.arg(id)
FOR UPDATE;

-- name: GetLivingPlayerCharacterID :one
-- The player's living character in the campaign, if any (RN-03). The
-- partial unique index characters_one_living_player_character allows at
-- most one.
SELECT id FROM characters
WHERE campaign_id = sqlc.arg(campaign_id)::UUID
  AND player_user_id = sqlc.arg(player_user_id)::UUID
  AND kind = 'player' AND status <> 'dead'
LIMIT 1;

-- name: ListCharacters :many
-- Without player_user_id, every character of the campaign (the master's
-- list); with it, only that player's characters. With status, only the
-- characters in that status (a pending member sees only their pending
-- character, RN-15). Players' characters come first, then NPCs, each group
-- oldest first. The story is left out: a list does not show it.
SELECT id, kind, status, name, player_user_id, sheet, sheet_locked_at, created_at, reserved, claimed_at
FROM characters
WHERE campaign_id = sqlc.arg(campaign_id)::UUID
  AND (
      sqlc.narg(player_user_id)::UUID IS NULL
      OR (kind = 'player' AND player_user_id = sqlc.narg(player_user_id)::UUID AND NOT reserved)
  )
  AND (sqlc.narg(status)::TEXT IS NULL OR status = sqlc.narg(status)::TEXT)
  -- The NPCs the app makes for the monsters of a combat (RN-29) are not the
  -- master's to list.
  AND COALESCE(sheet->'basic'->>'combat_only', 'false') <> 'true'
ORDER BY kind <> 'player', created_at, id;

-- name: UpdateCharacterSheet :one
-- revision in the WHERE clause is a second guard: the handler already
-- compared it under FOR UPDATE, so no row here means a stale revision.
UPDATE characters
SET name = sqlc.arg(name), sheet = sqlc.arg(sheet), revision = revision + 1, updated_at = sqlc.arg(now)
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND id = sqlc.arg(id) AND revision = sqlc.arg(revision)
RETURNING *;

-- name: ClearPortraits :execrows
-- The master deleted a gallery image: the NPCs that had it as their portrait
-- lose it (MR-031). The revision goes up, so a stale editor is told. Only the
-- sheets that have it are touched.
UPDATE characters
SET sheet = (sheet #- '{full,portrait_image_id}') #- '{basic,portrait_image_id}',
    revision = revision + 1, updated_at = sqlc.arg(now)
WHERE campaign_id = sqlc.arg(campaign_id)::UUID
  AND (sheet -> 'full' ->> 'portrait_image_id' = sqlc.arg(image_id)::TEXT
       OR sheet -> 'basic' ->> 'portrait_image_id' = sqlc.arg(image_id)::TEXT);

-- name: PortraitInUse :one
-- Whether any NPC of the campaign has the gallery image as its portrait (the
-- maps module copies a fog map's image when something else uses it, MR-036).
SELECT EXISTS (
    SELECT 1 FROM characters
    WHERE campaign_id = sqlc.arg(campaign_id)::UUID
      AND (sheet -> 'full' ->> 'portrait_image_id' = sqlc.arg(image_id)::TEXT
           OR sheet -> 'basic' ->> 'portrait_image_id' = sqlc.arg(image_id)::TEXT)
);

-- name: UpdateCharacterStory :one
-- As UpdateCharacterSheet: the story and the sheet share one revision.
UPDATE characters
SET story = sqlc.arg(story), revision = revision + 1, updated_at = sqlc.arg(now)
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND id = sqlc.arg(id) AND revision = sqlc.arg(revision)
RETURNING *;

-- name: SetStoryEditing :one
-- The master's permission for the player to edit the story. It changes
-- neither the revision nor updated_at, which follow the content.
UPDATE characters
SET story_editing_allowed = sqlc.arg(allowed)
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND id = sqlc.arg(id)
RETURNING *;

-- name: MarkCharacterDead :one
-- Marking a dead character again keeps the first died_at, so the call is
-- idempotent. A character that lived again (died_at stays as the history, status
-- 'active') gets a new death: died_at is set again and the switch of Revivify
-- starts off, since it belongs to one death. death_round and death_encounter_id are the
-- combat the master confirmed it in, NULL for a death marked outside a combat. It
-- changes neither the revision nor updated_at.
UPDATE characters
SET status = 'dead',
    died_at = CASE WHEN status = 'dead' THEN died_at ELSE sqlc.arg(now)::TIMESTAMPTZ END,
    death_round = CASE WHEN status = 'dead' THEN death_round ELSE sqlc.narg(death_round)::INT4 END,
    death_encounter_id = CASE WHEN status = 'dead' THEN death_encounter_id ELSE sqlc.narg(death_encounter_id)::UUID END,
    revivify_blocked = CASE WHEN status = 'dead' THEN revivify_blocked ELSE false END
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND id = sqlc.arg(id)
RETURNING *;

-- name: ReviveCharacter :one
-- The master's Reviver, or Revivify: a dead character lives again (RN-03). It goes back to
-- 'active' and keeps the sheet's lock as it was, since a death never touched it. died_at
-- stays as the history. revive_key and revive_hash are the idempotency key of the call and
-- the hash of its request (NULL for the spell, which has its own). It changes neither the
-- revision nor updated_at.
UPDATE characters
SET status = 'active', revived_at = sqlc.arg(now)::TIMESTAMPTZ, death_round = NULL, death_encounter_id = NULL,
    revivify_blocked = false, revive_key = sqlc.narg(revive_key), revive_hash = sqlc.narg(revive_hash)
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND id = sqlc.arg(id) AND status = 'dead'
RETURNING *;

-- name: SetCharacterRevivifyBlocked :one
-- The master's switch "Revivificar não funciona nesta morte", for a dead character.
UPDATE characters
SET revivify_blocked = sqlc.arg(blocked)
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND id = sqlc.arg(id) AND kind = 'player' AND status = 'dead'
RETURNING *;

-- name: ListDeadPlayerCharacters :many
-- The dead player characters of the campaign, newest death first: the targets Revivify
-- offers outside a combat, and the master's "Reviver". The master's switch comes along.
SELECT id, name, player_user_id, died_at, revivify_blocked
FROM characters
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND kind = 'player' AND status = 'dead'
ORDER BY died_at DESC, id;

-- name: ApproveCharacter :one
-- The master approved a pending character (RN-15, MR-024): it becomes an
-- ordinary living character, a draft until the next game session starts.
-- Like death, approval changes neither the revision nor updated_at.
UPDATE characters
SET status = 'active'
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND id = sqlc.arg(id) AND status = 'pending'
RETURNING *;

-- name: GetCharacterReview :one
-- The master's request for changes on a pending character, if there is one.
SELECT * FROM character_reviews WHERE character_id = sqlc.arg(character_id)::UUID;

-- name: UpsertCharacterReviewRequest :one
-- RequestCharacterChanges: the first request makes the row; asking again replaces the
-- reason and the time and starts over (CHANGES_REQUESTED, not resubmitted). request_key
-- and request_hash are the call's idempotency key (scoped) and request hash.
INSERT INTO character_reviews
    (character_id, campaign_id, player_user_id, status, reason, requested_at, request_key, request_hash)
VALUES (
    sqlc.arg(character_id)::UUID, sqlc.arg(campaign_id)::UUID, sqlc.arg(player_user_id)::UUID,
    'changes_requested', sqlc.arg(reason), sqlc.arg(now), sqlc.narg(request_key), sqlc.narg(request_hash)
)
ON CONFLICT (character_id) DO UPDATE SET
    status = 'changes_requested', reason = excluded.reason, requested_at = excluded.requested_at,
    resubmitted_at = NULL, request_key = excluded.request_key, request_hash = excluded.request_hash,
    resubmit_key = NULL, resubmit_hash = NULL
RETURNING *;

-- name: MarkCharacterReviewResubmitted :one
-- ResubmitCharacter: only an open request can be answered. No row means there is none.
UPDATE character_reviews
SET status = 'resubmitted', resubmitted_at = sqlc.arg(now), resubmit_key = sqlc.narg(resubmit_key), resubmit_hash = sqlc.narg(resubmit_hash)
WHERE character_id = sqlc.arg(character_id)::UUID AND status = 'changes_requested'
RETURNING *;

-- name: DeleteCharacterReview :exec
-- The reason is deleted as soon as the master settles the character (approval, rejection).
-- Deleting the character, the campaign or the player's account deletes it by itself
-- (ON DELETE CASCADE).
DELETE FROM character_reviews WHERE character_id = sqlc.arg(character_id)::UUID;

-- name: DeleteCharacterReviewsOfPlayer :exec
-- An ordinary invite promotes a pending member and counts as the master's approval
-- (ApprovePendingCharacterOfPlayer): the reason goes with the pending state.
DELETE FROM character_reviews
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND player_user_id = sqlc.arg(player_user_id)::UUID;

-- name: ListCharacterReviewStatuses :many
-- The status of each open review of the campaign, for the master's list ("Ajustes pedidos")
-- and the owner's own pending character. No reason leaves the table here.
SELECT character_id, status FROM character_reviews WHERE campaign_id = sqlc.arg(campaign_id)::UUID;

-- name: ApprovePendingCharacterOfPlayer :execrows
-- An ordinary invite promotes a pending member (RN-15): that counts as
-- the master's approval, so their character waiting for approval, if any,
-- is approved with exactly the effect of ApproveCharacter above, found by
-- player instead of by ID. A player has at most one living character
-- (RN-03), so at most one row matches; none matches when they have not
-- created it yet.
UPDATE characters
SET status = 'active'
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND player_user_id = sqlc.arg(player_user_id)
  AND kind = 'player' AND status = 'pending';

-- name: DeletePendingCharacter :execrows
-- The master rejected a pending character (RN-15, MR-024): it never became
-- part of the campaign, so it is deleted, story and all; the master's notes
-- about it go too (character_master_notes cascades). status = 'pending' in
-- the WHERE clause means no other character is ever deleted here: outside
-- account deletion, characters change status, never row (RN-03).
DELETE FROM characters
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND id = sqlc.arg(id) AND status = 'pending';

-- name: LockSheets :execrows
-- RN-01: when a game session starts, the sheets of the campaign's living
-- player characters that are still drafts lock. A character waiting for
-- approval (MR-024) does not lock yet, and NPCs never lock. A reserved character
-- (MR-049) has no player to lock out: it locks with the sessions that start after
-- its player claims it.
UPDATE characters
SET sheet_locked_at = sqlc.arg(now)::TIMESTAMPTZ
WHERE campaign_id = sqlc.arg(campaign_id)::UUID
  AND kind = 'player' AND status = 'active' AND sheet_locked_at IS NULL
  AND NOT reserved;

-- name: EndStoryEditing :execrows
-- When a game session starts, every permission to edit a story ends.
UPDATE characters
SET story_editing_allowed = false
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND story_editing_allowed;

-- name: CharacterIsInCampaign :one
SELECT EXISTS (
    SELECT 1 FROM characters
    WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND id = sqlc.arg(id)
);

-- name: GetMasterNotes :one
SELECT notes, updated_at FROM character_master_notes
WHERE campaign_id = $1 AND character_id = $2;

-- name: UpsertMasterNotes :one
INSERT INTO character_master_notes (campaign_id, character_id, notes, updated_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (campaign_id, character_id)
DO UPDATE SET notes = excluded.notes, updated_at = excluded.updated_at
RETURNING notes, updated_at;

-- name: DeleteMasterNotes :exec
-- Empty notes are not stored (RN-11 keeps only what is needed).
DELETE FROM character_master_notes
WHERE campaign_id = $1 AND character_id = $2;

-- name: ListVitals :many
-- The vitals of the campaign's living, active player characters (RN-02),
-- oldest first: the party at the table. A character without a
-- character_vitals row has fresh vitals, so the columns from it may be NULL.
-- The sheet comes along because the maximums are derived from it. The Wild Shape
-- form and the familiar's sight come along too (MR-037, MR-036).
SELECT c.id, c.name, c.player_user_id, c.sheet,
       v.hit_points_current, v.hit_points_temporary, v.spell_slots_used,
       v.pact_slots_used, v.hit_dice_used, v.hit_dice_used_by_die, v.spell_slots_created, v.resources_used, v.revision, v.updated_at,
       ws.beast AS wild_shape_beast, ws.hp AS wild_shape_hp, v.familiar_sight_creature_id, v.familiar_sight_in_combat,
       v.familiar_sight_conditions, v.hit_points_max_bonus, v.exhaustion_level, v.armor_class_base, c.active_states
FROM characters AS c
LEFT JOIN character_vitals AS v ON v.character_id = c.id
LEFT JOIN character_wild_shapes AS ws ON ws.character_id = c.id
WHERE c.campaign_id = sqlc.arg(campaign_id)::UUID
  AND c.kind = 'player' AND c.status = 'active' AND NOT c.reserved
ORDER BY c.created_at, c.id;

-- name: GetVitals :one
-- ListVitals for one character. No row means the character is not a
-- living, active player character of the campaign.
SELECT c.id, c.name, c.player_user_id, c.sheet,
       v.hit_points_current, v.hit_points_temporary, v.spell_slots_used,
       v.pact_slots_used, v.hit_dice_used, v.hit_dice_used_by_die, v.spell_slots_created, v.resources_used, v.revision, v.updated_at,
       ws.beast AS wild_shape_beast, ws.hp AS wild_shape_hp, v.familiar_sight_creature_id, v.familiar_sight_in_combat,
       v.familiar_sight_conditions, v.hit_points_max_bonus, v.exhaustion_level, v.armor_class_base, c.active_states
FROM characters AS c
LEFT JOIN character_vitals AS v ON v.character_id = c.id
LEFT JOIN character_wild_shapes AS ws ON ws.character_id = c.id
WHERE c.campaign_id = sqlc.arg(campaign_id)::UUID AND c.id = sqlc.arg(id)
  AND c.kind = 'player' AND c.status = 'active' AND NOT c.reserved;

-- name: GetVitalsWithDead :one
-- GetVitals that also answers for a player character that died: the page of a
-- dead character's player still reads its turn options and its log (a combat
-- that ran keeps them), and nothing here lets the character act again.
SELECT c.id, c.name, c.player_user_id, c.sheet,
       v.hit_points_current, v.hit_points_temporary, v.spell_slots_used,
       v.pact_slots_used, v.hit_dice_used, v.hit_dice_used_by_die, v.spell_slots_created, v.resources_used, v.revision, v.updated_at,
       ws.beast AS wild_shape_beast, ws.hp AS wild_shape_hp, v.familiar_sight_creature_id, v.familiar_sight_in_combat,
       v.familiar_sight_conditions, v.hit_points_max_bonus, v.exhaustion_level, v.armor_class_base, c.active_states
FROM characters AS c
LEFT JOIN character_vitals AS v ON v.character_id = c.id
LEFT JOIN character_wild_shapes AS ws ON ws.character_id = c.id
WHERE c.campaign_id = sqlc.arg(campaign_id)::UUID AND c.id = sqlc.arg(id)
  AND c.kind = 'player' AND c.status IN ('active', 'dead') AND NOT c.reserved;

-- name: UpsertVitals :one
-- Saves a character's vitals: the first save creates the row with revision
-- 1, and every later one adds 1. A NULL hit_points_current is "never set":
-- full hit points, whatever the maximum is.
INSERT INTO character_vitals
    (character_id, hit_points_current, hit_points_temporary, spell_slots_used,
     pact_slots_used, hit_dice_used, hit_dice_used_by_die, spell_slots_created, resources_used, revision, updated_at)
VALUES (
    sqlc.arg(character_id), sqlc.narg(hit_points_current), sqlc.arg(hit_points_temporary),
    sqlc.arg(spell_slots_used)::INT4[], sqlc.arg(pact_slots_used), sqlc.arg(hit_dice_used),
    sqlc.arg(hit_dice_used_by_die)::JSONB, sqlc.arg(spell_slots_created)::INT4[],
    sqlc.arg(resources_used)::JSONB, 1, sqlc.arg(now)
)
ON CONFLICT (character_id) DO UPDATE SET
    hit_points_current = excluded.hit_points_current,
    hit_points_temporary = excluded.hit_points_temporary,
    spell_slots_used = excluded.spell_slots_used,
    pact_slots_used = excluded.pact_slots_used,
    hit_dice_used = excluded.hit_dice_used,
    hit_dice_used_by_die = excluded.hit_dice_used_by_die,
    spell_slots_created = excluded.spell_slots_created,
    resources_used = excluded.resources_used,
    revision = character_vitals.revision + 1,
    updated_at = excluded.updated_at
RETURNING revision, updated_at;

-- name: SetVitalsExhaustion :one
-- Sets a character's level of exhaustion (SRD, Conditions: Exhaustion) and, when the new
-- hit point maximum cut the hit points, the hit points: the first call creates the row, as
-- UpsertVitals does. A NULL hit_points_current stays "never set" (full), so only a cut
-- writes it.
INSERT INTO character_vitals
    (character_id, hit_points_current, exhaustion_level, revision, updated_at)
VALUES (sqlc.arg(character_id), sqlc.narg(hit_points_current), sqlc.arg(exhaustion_level), 1, sqlc.arg(now))
ON CONFLICT (character_id) DO UPDATE SET
    hit_points_current = COALESCE(excluded.hit_points_current, character_vitals.hit_points_current),
    exhaustion_level = excluded.exhaustion_level,
    revision = character_vitals.revision + 1,
    updated_at = excluded.updated_at
RETURNING revision, updated_at;

-- name: SetVitalsArmorBase :one
-- Sets the base armor class an effect that lasts gives a character (NULL: none): the first
-- call creates the row, as UpsertVitals does, and every change bumps the revision.
INSERT INTO character_vitals
    (character_id, armor_class_base, revision, updated_at)
VALUES (sqlc.arg(character_id), sqlc.narg(armor_class_base), 1, sqlc.arg(now))
ON CONFLICT (character_id) DO UPDATE SET
    armor_class_base = excluded.armor_class_base,
    revision = character_vitals.revision + 1,
    updated_at = excluded.updated_at
RETURNING revision, updated_at;

-- name: ListMapCharacters :many
-- Those of the given characters that may stand on a map of the campaign as
-- tokens (package maps): its living characters, the players' (active: not
-- dead, not waiting for approval) and the NPCs. Players' characters first,
-- then NPCs, each group oldest first, as ListCharacters. No sheet, no story.
SELECT id, kind, name, player_user_id FROM characters
WHERE campaign_id = sqlc.arg(campaign_id)::UUID
  AND id = ANY(sqlc.arg(ids)::UUID[])
  AND status = 'active' AND NOT reserved
ORDER BY kind <> 'player', created_at, id;

-- name: ListNpcPortraits :many
-- The gallery image of the portrait of each of the given NPCs of the campaign
-- (living, active ones), "" for an NPC without one. A full sheet keeps it under
-- 'full', a basic one under 'basic' (package maps asks, to send an NPC's portrait
-- to the image model as a character reference, MR-039).
SELECT id, COALESCE(NULLIF(sheet -> 'full' ->> 'portrait_image_id', ''), NULLIF(sheet -> 'basic' ->> 'portrait_image_id', ''), '')::TEXT AS portrait_image_id
FROM characters
WHERE campaign_id = sqlc.arg(campaign_id)::UUID
  AND id = ANY(sqlc.arg(ids)::UUID[])
  AND kind <> 'player'
  AND status = 'active';

-- name: ListCombatParty :many
-- The campaign's living, active player characters, oldest first: the party
-- that fights (package play). The sheet comes along for the numbers a
-- combatant starts with (initiative, speed).
-- The beast of a druid in Wild Shape comes along (MR-037): the combatant starts
-- with the beast's speed and size.
SELECT c.id, c.kind, c.name, c.player_user_id, c.sheet, ws.beast AS wild_shape_beast, c.active_states
FROM characters AS c
LEFT JOIN character_wild_shapes AS ws ON ws.character_id = c.id
WHERE c.campaign_id = sqlc.arg(campaign_id)::UUID
  AND c.kind = 'player' AND c.status = 'active' AND NOT c.reserved
ORDER BY c.created_at, c.id;

-- name: ListCombatCharacters :many
-- Those of the given characters that may fight in a combat of the campaign:
-- its living characters, players' and NPCs, oldest first (package play).
-- The beast of a druid in Wild Shape comes along, as in ListCombatParty.
SELECT c.id, c.kind, c.name, c.player_user_id, c.sheet, ws.beast AS wild_shape_beast, c.active_states
FROM characters AS c
LEFT JOIN character_wild_shapes AS ws ON ws.character_id = c.id
WHERE c.campaign_id = sqlc.arg(campaign_id)::UUID
  AND c.id = ANY(sqlc.arg(ids)::UUID[])
  AND c.status = 'active' AND NOT c.reserved
ORDER BY c.created_at, c.id;

-- name: ListCombatCharactersWithDead :many
-- ListCombatCharacters that also returns a character that died, for the reads
-- of a combat that ran (a dead character's sheet names the attacks in the log,
-- and its player's page reads the turn options). Nothing that starts a combat
-- or lets a character act uses it.
SELECT c.id, c.kind, c.name, c.player_user_id, c.sheet, ws.beast AS wild_shape_beast, c.active_states
FROM characters AS c
LEFT JOIN character_wild_shapes AS ws ON ws.character_id = c.id
WHERE c.campaign_id = sqlc.arg(campaign_id)::UUID
  AND c.id = ANY(sqlc.arg(ids)::UUID[])
  AND c.status IN ('active', 'dead') AND NOT c.reserved
ORDER BY c.created_at, c.id;

-- name: ListSessionCharacters :many
-- Those of the given characters of the campaign, whatever their status: the
-- session summary names a character that died or left during the session
-- (package play). reserved says the master gave the character back to the reserve
-- since: a player's reads leave it out (RN-10).
SELECT id, kind, name, player_user_id, reserved FROM characters
WHERE campaign_id = sqlc.arg(campaign_id)::UUID
  AND id = ANY(sqlc.arg(ids)::UUID[]);

-- name: ListCharacterNames :many
-- The names of the given characters of the campaign, whatever their kind or
-- status (package progression names the characters of an award, even one that
-- died since).
SELECT id, name FROM characters
WHERE campaign_id = sqlc.arg(campaign_id)::UUID
  AND id = ANY(sqlc.arg(ids)::UUID[]);

-- name: GetLevelUpRoll :one
-- The hit die the server rolled for the character's next level (MR-040): one
-- per character and target total level (not per class: a multiclass
-- character cannot roll twice for a level), kept until the level-up uses it.
SELECT class_key, die, value FROM character_level_up_rolls
WHERE character_id = sqlc.arg(character_id)::UUID AND to_level = sqlc.arg(to_level);

-- name: InsertLevelUpRoll :execrows
-- Keeps a roll. A second roll for the same level does nothing (0 rows), so the
-- first one stands: asking again never rerolls.
INSERT INTO character_level_up_rolls (character_id, class_key, to_level, die, value, created_at)
VALUES (sqlc.arg(character_id)::UUID, sqlc.arg(class_key), sqlc.arg(to_level), sqlc.arg(die), sqlc.arg(value), sqlc.arg(now))
ON CONFLICT (character_id, to_level) DO NOTHING;

-- name: DeleteLevelUpRoll :exec
-- The level-up used the roll; its value goes on in character_level_ups.
DELETE FROM character_level_up_rolls
WHERE character_id = sqlc.arg(character_id)::UUID AND to_level = sqlc.arg(to_level);

-- name: InsertLevelUp :one
-- The record of a guided level-up (MR-040), written in the transaction that
-- saves the sheet.
INSERT INTO character_level_ups
    (campaign_id, character_id, class_key, from_level, to_level, hp_method, hp_value, choices, created_at)
VALUES (
    sqlc.arg(campaign_id)::UUID, sqlc.arg(character_id)::UUID, sqlc.arg(class_key), sqlc.arg(from_level),
    sqlc.arg(to_level), sqlc.arg(hp_method), sqlc.arg(hp_value), sqlc.arg(choices), sqlc.arg(created_at)
)
RETURNING *;

-- name: ListLevelUps :many
-- A campaign's level-ups, newest first, for the master's "O que mudou". With
-- character_id, only that character's. The character's name is the current
-- one, and its player the character's own.
SELECT l.id, l.character_id, l.class_key, l.from_level, l.to_level, l.choices, l.created_at,
       c.name AS character_name, c.player_user_id
FROM character_level_ups l
JOIN characters c ON c.id = l.character_id
WHERE l.campaign_id = sqlc.arg(campaign_id)::UUID
  AND (sqlc.narg(character_id)::UUID IS NULL OR l.character_id = sqlc.narg(character_id)::UUID)
  AND (sqlc.narg(before_created_at)::TIMESTAMPTZ IS NULL
       OR (l.created_at, l.id) < (sqlc.narg(before_created_at)::TIMESTAMPTZ, sqlc.narg(before_id)::UUID))
ORDER BY l.created_at DESC, l.id DESC
LIMIT sqlc.arg(max_rows);

-- A character's creatures (MR-037). A creature is "live" until it is
-- dismissed; a dismissed one is kept (dismissed_at, dismissed_reason) so an
-- undo can bring it back.

-- name: InsertCharacterCreature :one
INSERT INTO character_creatures
    (campaign_id, character_id, monster_key, name, source, attack, summon_group_id, concentration_cast_id, hp_current, hp_max, created_at, create_key, create_hash)
VALUES (
    sqlc.arg(campaign_id)::UUID, sqlc.arg(character_id)::UUID, sqlc.arg(monster_key), sqlc.arg(name), sqlc.arg(source),
    sqlc.arg(attack), sqlc.arg(summon_group_id)::UUID, sqlc.narg(concentration_cast_id)::UUID, sqlc.arg(hp_current), sqlc.arg(hp_max),
    sqlc.arg(created_at), sqlc.narg(create_key), sqlc.narg(create_hash)
)
RETURNING *;
-- A creature is made inside the owner's locked transaction (GiveCreature locks the owner first), so a
-- retry reads the first one with GetCharacterCreatureByCreateKey before it inserts, and never races it;
-- the unique index (character_creatures_create_key_idx) is the net under that.

-- name: SetCharacterCreatureCreateKey :exec
-- The idempotency key of GiveCreature and the hash of its request, set on the creature the summon made
-- (a summon makes the creature the same way whoever asks, so the key is written after it).
UPDATE character_creatures
SET create_key = sqlc.arg(create_key), create_hash = sqlc.arg(create_hash)
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND id = sqlc.arg(id)::UUID;

-- name: GetCharacterCreatureByCreateKey :one
-- The creature a GiveCreature with this idempotency key made, if any (the key carries the campaign's ID).
SELECT * FROM character_creatures WHERE create_key = $1;

-- name: GetCharacterCreature :one
-- One creature of the campaign, live or not, with its owner's player.
SELECT sqlc.embed(cc), c.player_user_id
FROM character_creatures cc
JOIN characters c ON c.id = cc.character_id
WHERE cc.campaign_id = sqlc.arg(campaign_id)::UUID AND cc.id = sqlc.arg(id)::UUID;

-- name: GetCharacterCreatureForUpdate :one
-- The same, locking the creature's row until the transaction ends.
SELECT sqlc.embed(cc), c.player_user_id
FROM character_creatures cc
JOIN characters c ON c.id = cc.character_id
WHERE cc.campaign_id = sqlc.arg(campaign_id)::UUID AND cc.id = sqlc.arg(id)::UUID
FOR UPDATE OF cc;

-- name: ListLiveCreaturesOfCharacters :many
-- The live creatures of the given characters of the campaign, oldest first.
SELECT sqlc.embed(cc), c.player_user_id
FROM character_creatures cc
JOIN characters c ON c.id = cc.character_id
WHERE cc.campaign_id = sqlc.arg(campaign_id)::UUID
  AND cc.character_id = ANY(sqlc.arg(character_ids)::UUID[])
  AND cc.dismissed_at IS NULL
ORDER BY cc.created_at, cc.id;

-- name: ListCreaturesByIDs :many
-- The creatures of the campaign with the given IDs, live or not.
SELECT sqlc.embed(cc), c.player_user_id
FROM character_creatures cc
JOIN characters c ON c.id = cc.character_id
WHERE cc.campaign_id = sqlc.arg(campaign_id)::UUID AND cc.id = ANY(sqlc.arg(ids)::UUID[])
ORDER BY cc.created_at, cc.id;

-- name: ListLiveCreaturesOnConcentration :many
-- The live creatures of the character that last only while it concentrates.
SELECT sqlc.embed(cc), c.player_user_id
FROM character_creatures cc
JOIN characters c ON c.id = cc.character_id
WHERE cc.campaign_id = sqlc.arg(campaign_id)::UUID
  AND cc.character_id = sqlc.arg(character_id)::UUID
  AND cc.concentration_cast_id IS NOT NULL
  AND cc.dismissed_at IS NULL
ORDER BY cc.created_at, cc.id;

-- name: ListLiveFamiliars :many
-- The character's live familiars: one at a time (the SRD), so a new one
-- dismisses these.
SELECT sqlc.embed(cc), c.player_user_id
FROM character_creatures cc
JOIN characters c ON c.id = cc.character_id
WHERE cc.campaign_id = sqlc.arg(campaign_id)::UUID
  AND cc.character_id = sqlc.arg(character_id)::UUID
  AND cc.source = 'familiar'
  AND cc.dismissed_at IS NULL
ORDER BY cc.created_at, cc.id;

-- name: CountLiveCreaturesOfCharacter :one
SELECT count(*)::INT4 FROM character_creatures
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND character_id = sqlc.arg(character_id)::UUID AND dismissed_at IS NULL;

-- name: DismissCreatures :many
-- Sends the live ones of these creatures away, with the reason; a creature
-- dismissed because it was defeated is at 0 hit points. Returns the ones it
-- dismissed.
UPDATE character_creatures
SET dismissed_at = sqlc.arg(at)::TIMESTAMPTZ, dismissed_reason = sqlc.arg(reason)::TEXT,
    hp_current = CASE WHEN sqlc.arg(reason)::TEXT = 'defeated' THEN 0 ELSE hp_current END
WHERE campaign_id = sqlc.arg(campaign_id)::UUID
  AND id = ANY(sqlc.arg(ids)::UUID[])
  AND dismissed_at IS NULL
RETURNING id;

-- name: ReviveCreatures :many
-- Brings back the creatures dismissed for this reason (an undo): the ones
-- defeated that an undo healed, the ones a master's undo of a casting or of the
-- end of a concentration gives back. Returns the ones it revived.
UPDATE character_creatures
SET dismissed_at = NULL, dismissed_reason = NULL
WHERE campaign_id = sqlc.arg(campaign_id)::UUID
  AND id = ANY(sqlc.arg(ids)::UUID[])
  AND dismissed_reason = sqlc.arg(reason)::TEXT
RETURNING id;

-- name: DeleteCreatures :exec
-- Takes the creatures a casting made away for good (the master's undo of it).
DELETE FROM character_creatures
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND id = ANY(sqlc.arg(ids)::UUID[]);

-- name: SetCharacterCreatureName :exec
UPDATE character_creatures SET name = sqlc.arg(name)
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND id = sqlc.arg(id)::UUID;

-- name: SetCharacterCreatureHitPoints :exec
-- The creature's hit points: the master's correction, or a combat's write-back.
UPDATE character_creatures SET hp_current = sqlc.arg(hp_current)
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND id = sqlc.arg(id)::UUID;

-- name: SetWildShape :exec
-- A druid's Wild Shape form (MR-037): the beast and its current hit points
-- (1 or more). The caller also bumps the vitals' revision (TouchVitals).
INSERT INTO character_wild_shapes (character_id, beast, hp, updated_at)
VALUES (sqlc.arg(character_id), sqlc.arg(beast), sqlc.arg(hp), sqlc.arg(now))
ON CONFLICT (character_id) DO UPDATE SET beast = excluded.beast, hp = excluded.hp, updated_at = excluded.updated_at;

-- name: ClearWildShape :exec
-- The druid is itself again.
DELETE FROM character_wild_shapes WHERE character_id = $1;

-- name: SetActiveStates :exec
-- The temporary states that are on for a character (a table feature's state effect): the whole
-- list, as JSON. The caller also bumps the vitals' revision (TouchVitals).
UPDATE characters SET active_states = sqlc.arg(states)::JSONB WHERE id = sqlc.arg(id);

-- name: CarryHitPoints :exec
-- The sheet's maximum hit points changed from old_max to new_max (a level-up,
-- an edit): a character whose current hit points are set gains what the maximum
-- gained (gain, zero when it fell), so a wound stays a wound, and never ends
-- above the new maximum. Both maximums are the sheet's, and Aid's bonus (the
-- character's hit_points_max_bonus) rides on top of each. NULL is "full" and stays so. The revision moves only
-- when the number does.
UPDATE character_vitals
SET hit_points_current = LEAST(LEAST(hit_points_current, sqlc.arg(old_max)::INT4 + hit_points_max_bonus) + sqlc.arg(gain)::INT4, sqlc.arg(new_max)::INT4 + hit_points_max_bonus),
    revision = revision + 1,
    updated_at = sqlc.arg(now)
WHERE character_id = sqlc.arg(character_id)
  AND hit_points_current IS NOT NULL
  AND hit_points_current <> LEAST(LEAST(hit_points_current, sqlc.arg(old_max)::INT4 + hit_points_max_bonus) + sqlc.arg(gain)::INT4, sqlc.arg(new_max)::INT4 + hit_points_max_bonus);

-- name: SetVitalsMaxBonus :one
-- Aid's bonus to a character's maximum hit points (hit_points_max_bonus) and the
-- current hit points that go with it: the first write creates the vitals row, as
-- TouchVitals does (hit_points_current is only for that insert, and a NULL "full"
-- stays full of the new maximum). A NULL hit_points_current argument leaves the
-- stored current hit points alone.
INSERT INTO character_vitals (character_id, hit_points_current, hit_points_max_bonus, revision, updated_at)
VALUES (sqlc.arg(character_id), sqlc.narg(hit_points_current), sqlc.arg(hit_points_max_bonus), 1, sqlc.arg(now))
ON CONFLICT (character_id) DO UPDATE SET
    hit_points_max_bonus = excluded.hit_points_max_bonus,
    hit_points_current = COALESCE(excluded.hit_points_current, character_vitals.hit_points_current),
    revision = character_vitals.revision + 1,
    updated_at = excluded.updated_at
RETURNING revision, updated_at;

-- name: TouchVitals :one
-- Bumps a character's vitals revision for a change made on a table of its own (the
-- Wild Shape form): the first write creates the vitals row, as UpsertVitals does
-- (hit_points_current is only for that insert).
INSERT INTO character_vitals (character_id, hit_points_current, revision, updated_at)
VALUES (sqlc.arg(character_id), sqlc.arg(hit_points_current), 1, sqlc.arg(now))
ON CONFLICT (character_id) DO UPDATE SET
    revision = character_vitals.revision + 1,
    updated_at = excluded.updated_at
RETURNING revision, updated_at;

-- name: SetFamiliarSight :one
-- "Ver pelos olhos do familiar" (MR-036): the familiar the player looks through
-- (NULL for none), whether it started in a combat and the conditions it gave the
-- combatant. The first write creates the vitals row, as TouchVitals does.
INSERT INTO character_vitals (character_id, familiar_sight_creature_id, familiar_sight_in_combat, familiar_sight_conditions, revision, updated_at)
VALUES (sqlc.arg(character_id), sqlc.narg(creature_id), sqlc.arg(in_combat), sqlc.arg(conditions)::TEXT[], 1, sqlc.arg(now))
ON CONFLICT (character_id) DO UPDATE SET
    familiar_sight_creature_id = excluded.familiar_sight_creature_id,
    familiar_sight_in_combat = excluded.familiar_sight_in_combat,
    familiar_sight_conditions = excluded.familiar_sight_conditions,
    revision = character_vitals.revision + 1,
    updated_at = excluded.updated_at
RETURNING revision, updated_at;

-- name: ListPartyVision :many
-- The campaign's active and dead player characters, oldest first, with what the
-- fog needs to know of how each one sees (package maps): the sheet, the beast of
-- a Wild Shape form, and the familiar the player looks through, if it is still with
-- the character (MR-036, MR-037). A dead character is listed (is_dead): its
-- player keeps seeing what the living party sees.
SELECT c.id, c.player_user_id, c.sheet, ws.beast AS wild_shape_beast, c.active_states,
       cc.id AS familiar_id, cc.monster_key AS familiar_monster_key,
       (c.status = 'dead')::BOOL AS is_dead
FROM characters AS c
LEFT JOIN character_wild_shapes AS ws ON ws.character_id = c.id
LEFT JOIN character_vitals AS v ON v.character_id = c.id
LEFT JOIN character_creatures AS cc ON cc.id = v.familiar_sight_creature_id AND cc.dismissed_at IS NULL
WHERE c.campaign_id = sqlc.arg(campaign_id)::UUID
  AND c.kind = 'player' AND c.status IN ('active', 'dead') AND NOT c.reserved
ORDER BY c.created_at, c.id;

-- name: ListMapCreatures :many
-- The live creatures of the given IDs that belong to a living player's character
-- of the campaign: the ones that may have a token on a map (package maps). Oldest
-- first.
SELECT cc.id, cc.character_id, cc.name, cc.monster_key, c.player_user_id
FROM character_creatures AS cc
JOIN characters AS c ON c.id = cc.character_id
WHERE cc.campaign_id = sqlc.arg(campaign_id)::UUID
  AND cc.id = ANY(sqlc.arg(ids)::UUID[])
  AND cc.dismissed_at IS NULL
  AND c.status = 'active'
ORDER BY cc.created_at, cc.id;

-- name: GetAbilityRolls :one
-- The six sets of 4d6 stored for the player's next new character in the
-- campaign (RN-24): the same sets come back until a sheet uses them.
SELECT sets, source, rolled_at FROM character_ability_rolls
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND user_id = sqlc.arg(user_id)::UUID;

-- name: InsertAbilityRolls :execrows
-- Stores the sets. A second insert for the same player and campaign does
-- nothing (0 rows), so the first sets stand: asking again never rerolls.
INSERT INTO character_ability_rolls (campaign_id, user_id, sets, source, rolled_at)
VALUES (sqlc.arg(campaign_id)::UUID, sqlc.arg(user_id)::UUID, sqlc.arg(sets)::JSONB, sqlc.arg(source), sqlc.arg(now))
ON CONFLICT (campaign_id, user_id) DO NOTHING;

-- name: DeleteAbilityRolls :exec
-- A sheet used the sets (CreateCharacter): the next character gets new ones.
DELETE FROM character_ability_rolls
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND user_id = sqlc.arg(user_id)::UUID;

-- The table's own content (MR-025, RN-23, ADR-0018): campaign_content, one row
-- per entry, and campaign_content_state, the campaign's content revision.

-- name: GetContentRevision :one
-- The campaign's content revision. No row is revision 0 (no content of its own).
-- Read in the caller's transaction: it orders a content write against a sheet
-- write, and the cache of the live content is keyed by it.
SELECT revision FROM campaign_content_state WHERE campaign_id = sqlc.arg(campaign_id)::UUID;

-- name: BumpContentRevision :one
-- Every content write starts here: the revision goes up by one (the row is made
-- by the first write), and the row is held until the transaction ends, so two
-- writes of one campaign run one after the other.
INSERT INTO campaign_content_state (campaign_id, revision, updated_at)
VALUES (sqlc.arg(campaign_id)::UUID, 1, sqlc.arg(now))
ON CONFLICT (campaign_id) DO UPDATE
SET revision = campaign_content_state.revision + 1, updated_at = EXCLUDED.updated_at
RETURNING revision;

-- name: ListCampaignContent :many
-- Every entry of the campaign, archived ones included, sorted by key.
SELECT * FROM campaign_content
WHERE campaign_id = sqlc.arg(campaign_id)::UUID
ORDER BY content_key;

-- name: GetCampaignContent :one
SELECT * FROM campaign_content
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND content_key = sqlc.arg(content_key);

-- name: InsertCampaignContent :one
INSERT INTO campaign_content
    (campaign_id, content_key, kind, name_pt, data, revision, created_at, updated_at)
VALUES (
    sqlc.arg(campaign_id)::UUID, sqlc.arg(content_key), sqlc.arg(kind), sqlc.arg(name_pt), sqlc.arg(data),
    sqlc.arg(revision), sqlc.arg(now), sqlc.arg(now)
)
RETURNING *;

-- name: InsertCampaignContentWithKey :one
-- InsertCampaignContent for CreateTableEntry with an idempotency key: create_key and create_hash are
-- kept with the entry. The master's write bumps the content revision first, in the same transaction,
-- so two calls with the same key take turns; ON CONFLICT is the net under that.
INSERT INTO campaign_content
    (campaign_id, content_key, kind, name_pt, data, revision, create_key, create_hash, created_at, updated_at)
VALUES (
    sqlc.arg(campaign_id)::UUID, sqlc.arg(content_key), sqlc.arg(kind), sqlc.arg(name_pt), sqlc.arg(data),
    sqlc.arg(revision), sqlc.arg(create_key), sqlc.arg(create_hash), sqlc.arg(now), sqlc.arg(now)
)
ON CONFLICT (create_key) WHERE create_key IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetCampaignContentByCreateKey :one
-- The entry a CreateTableEntry with this idempotency key made, if any (the key carries the campaign's ID).
SELECT * FROM campaign_content WHERE create_key = $1;

-- name: GetCampaignContentImport :one
-- The answer of the ImportTableContent that carried this idempotency key (the campaign's ID and
-- the key), with the hash of its request.
SELECT create_hash, response FROM campaign_content_imports
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND create_key = sqlc.arg(create_key);

-- name: InsertCampaignContentImport :exec
-- Keeps the key, the request hash and the answer of an applied import. The content revision is held
-- by the transaction (BumpContentRevision), so two imports with one key take turns; ON CONFLICT is
-- the net under that.
INSERT INTO campaign_content_imports (campaign_id, create_key, create_hash, response, created_at)
VALUES (sqlc.arg(campaign_id)::UUID, sqlc.arg(create_key), sqlc.arg(create_hash), sqlc.arg(response), sqlc.arg(now))
ON CONFLICT (campaign_id, create_key) DO NOTHING;

-- name: UpdateCampaignContent :one
-- The body and the name; the key, the kind and the archive mark stay.
UPDATE campaign_content
SET name_pt = sqlc.arg(name_pt), data = sqlc.arg(data), revision = sqlc.arg(revision), updated_at = sqlc.arg(now)
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND content_key = sqlc.arg(content_key)
RETURNING *;

-- name: SetCampaignContentArchived :one
-- archived true retires the entry, false brings it back. Neither is a change of
-- the entry: its revision and updated_at stay (a sheet that uses it is not told it
-- changed). The campaign's revision still goes up, for the cache (BumpContentRevision).
UPDATE campaign_content
SET archived_at = CASE WHEN sqlc.arg(archived)::BOOL THEN sqlc.arg(now)::TIMESTAMPTZ ELSE NULL END
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND content_key = sqlc.arg(content_key)
RETURNING *;

-- name: ListContentOff :many
-- The keys the master switched off for the players ("Opções para os jogadores"),
-- sorted. Read in the caller's transaction, with the revision.
SELECT content_key FROM campaign_content_off
WHERE campaign_id = sqlc.arg(campaign_id)::UUID
ORDER BY content_key;

-- name: InsertContentOff :exec
-- Switches the options off (the ones already off stay as they are).
INSERT INTO campaign_content_off (campaign_id, content_key, created_at)
SELECT sqlc.arg(campaign_id)::UUID, k, sqlc.arg(now)::TIMESTAMPTZ
FROM unnest(sqlc.arg(content_keys)::TEXT[]) AS k
ON CONFLICT (campaign_id, content_key) DO NOTHING;

-- name: DeleteContentOff :exec
-- Switches the options back on.
DELETE FROM campaign_content_off
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND content_key = ANY(sqlc.arg(content_keys)::TEXT[]);

-- name: ListCampaignSheets :many
-- Every character of the campaign with the sheet: who uses which table entry,
-- and who has issues after a change. Players' characters first, then NPCs.
SELECT id, kind, name, player_user_id, sheet
FROM characters
WHERE campaign_id = sqlc.arg(campaign_id)::UUID
ORDER BY kind <> 'player', created_at, id;

-- The campaign package (MR-050).

-- name: ListCharactersForPackage :many
-- Every character of the campaign with its sheet and story, oldest first, for an export.
SELECT id, kind, status, name, sheet, story FROM characters
WHERE campaign_id = sqlc.arg(campaign_id)::UUID
ORDER BY created_at, id;

-- name: ListMasterNotesOfCampaign :many
SELECT character_id, notes FROM character_master_notes
WHERE campaign_id = sqlc.arg(campaign_id)::UUID;

-- name: InsertImportedNpc :exec
-- An NPC made from a package: the id is chosen before, and the master owns it.
INSERT INTO characters (id, campaign_id, kind, master_user_id, status, name, sheet, story, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(campaign_id)::UUID, sqlc.arg(kind), sqlc.arg(master_user_id), 'active', sqlc.arg(name), sqlc.arg(sheet),
        sqlc.arg(story), sqlc.arg(now), sqlc.arg(now));

-- name: InsertImportedCampaignContent :exec
INSERT INTO campaign_content (campaign_id, content_key, kind, name_pt, data, revision, archived_at, created_at, updated_at)
VALUES (sqlc.arg(campaign_id)::UUID, sqlc.arg(content_key), sqlc.arg(kind), sqlc.arg(name_pt), sqlc.arg(data), sqlc.arg(revision),
        sqlc.narg(archived_at), sqlc.arg(now), sqlc.arg(now));

-- name: InsertImportedReservedCharacter :exec
-- A player's character made from a package (MR-050): reserved (no owner, invisible to the
-- players until one claims it, MR-049), with the id chosen before.
INSERT INTO characters (id, campaign_id, kind, status, name, sheet, story, reserved, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(campaign_id)::UUID, 'player', 'active', sqlc.arg(name), sqlc.arg(sheet), sqlc.arg(story), true, sqlc.arg(now), sqlc.arg(now));
