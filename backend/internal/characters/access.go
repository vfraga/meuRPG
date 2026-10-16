package characters

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The database's text values for kind and status (characters_kind_valid,
// characters_status_valid).
const (
	kindPlayer    = "player"
	kindMinion    = "minion"
	kindStory     = "story"
	statusActive  = "active"
	statusDead    = "dead"
	statusPending = "pending"
)

// Conversions between the database's kinds and the API's.
var (
	kindToDB = map[charactersv1.CharacterKind]string{
		charactersv1.CharacterKind_CHARACTER_KIND_PLAYER: kindPlayer,
		charactersv1.CharacterKind_CHARACTER_KIND_ENEMY:  "enemy",
		charactersv1.CharacterKind_CHARACTER_KIND_BOSS:   "boss",
		charactersv1.CharacterKind_CHARACTER_KIND_MINION: "minion",
		charactersv1.CharacterKind_CHARACTER_KIND_STORY:  "story",
	}
	kindFromDB = map[string]charactersv1.CharacterKind{
		kindPlayer: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER,
		"enemy":    charactersv1.CharacterKind_CHARACTER_KIND_ENEMY,
		"boss":     charactersv1.CharacterKind_CHARACTER_KIND_BOSS,
		"minion":   charactersv1.CharacterKind_CHARACTER_KIND_MINION,
		"story":    charactersv1.CharacterKind_CHARACTER_KIND_STORY,
	}
)

// characterState computes a character's state from its columns
// (docs/product/rules.md#character-lifecycle). NPCs are never locked
// nor dead, so they are always drafts.
func characterState(status string, sheetLockedAt *time.Time) charactersv1.CharacterState {
	switch {
	case status == statusDead:
		return charactersv1.CharacterState_CHARACTER_STATE_DEAD
	case status == statusPending:
		return charactersv1.CharacterState_CHARACTER_STATE_PENDING
	case sheetLockedAt != nil:
		return charactersv1.CharacterState_CHARACTER_STATE_LOCKED
	default:
		return charactersv1.CharacterState_CHARACTER_STATE_DRAFT
	}
}

// isMaster says whether the caller is the campaign's master. A pending
// member never is (authz makes them a player).
func isMaster(m authz.Membership) bool { return m.Role == authz.RoleMaster && !m.Pending }

// canSee says whether the caller may see a character of their campaign: the
// master sees every one; a player, only their own player characters; a
// pending member (RN-15, MR-024), only their own pending character, the one
// the allowance in package authz is about. Every other character is "not
// found" to them.
func canSee(m authz.Membership, kind, status string, playerUserID *string) bool {
	if isMaster(m) {
		return true
	}
	if m.Pending && status != statusPending {
		return false
	}
	return kind == kindPlayer && playerUserID != nil && *playerUserID == m.UserID
}

// newCharacterStatus is the status a character starts with: 'pending' when
// a pending member creates it, so it waits for the master's approval
// (RN-15); 'active' otherwise.
func newCharacterStatus(m authz.Membership) string {
	if m.Pending {
		return statusPending
	}
	return statusActive
}

// playerEditsSheet says whether the owning player may still change the
// sheet (RN-01): while it is a draft, or while it waits for the master's
// approval (RN-15, MR-024).
func playerEditsSheet(state charactersv1.CharacterState) bool {
	return state == charactersv1.CharacterState_CHARACTER_STATE_DRAFT ||
		state == charactersv1.CharacterState_CHARACTER_STATE_PENDING
}

// playerEditsStory says whether the owning player may change the story:
// whenever they may change the sheet, and afterwards while the master
// allows it (story_editing_allowed).
func playerEditsStory(state charactersv1.CharacterState, allowed bool) bool {
	return playerEditsSheet(state) || allowed
}

// blockedReason says why the owning player may not change the sheet now.
func blockedReason(state charactersv1.CharacterState) charactersv1.CharacterBlockedReason {
	if state == charactersv1.CharacterState_CHARACTER_STATE_DEAD {
		return charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CHARACTER_DEAD
	}
	return charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_SHEET_LOCKED
}

// characterToProto builds the Character a member sees: m is the caller,
// who may see the row (canSee), and playerDisplayName the owning player's
// display name. A full sheet gets its derived numbers, from content (the
// campaign's).
func (s *Service) characterToProto(content *rules.Content, row charactersdb.Character, m authz.Membership, playerDisplayName string) (*charactersv1.Character, error) {
	sheet, err := loadSheet(row.ID, row.Sheet)
	if err != nil {
		return nil, err
	}
	story, err := loadStory(row.ID, row.Story)
	if err != nil {
		return nil, err
	}
	state := characterState(row.Status, row.SheetLockedAt)
	master := isMaster(m)
	player := row.Kind == kindPlayer
	c := &charactersv1.Character{
		Id:                  row.ID,
		CampaignId:          deref(row.CampaignID),
		Kind:                kindFromDB[row.Kind],
		State:               state,
		Name:                row.Name,
		PlayerUserId:        deref(row.PlayerUserID),
		PlayerDisplayName:   playerDisplayName,
		Sheet:               sheet,
		Story:               story,
		Revision:            row.Revision,
		SheetLockedAt:       timestamp(row.SheetLockedAt),
		DiedAt:              diedAt(row),
		CreatedAt:           timestamppb.New(row.CreatedAt),
		UpdatedAt:           timestamppb.New(row.UpdatedAt),
		Reserved:            row.Reserved,
		StoryEditingAllowed: row.StoryEditingAllowed,
	}
	setPermissions(c, row, state, master)
	if player && mayReadReview(m, row) {
		c.RevivedAt = timestamp(row.RevivedAt)
		if state == charactersv1.CharacterState_CHARACTER_STATE_DEAD {
			c.DeathRound, c.DeathEncounterId = row.DeathRound, deref(row.DeathEncounterID)
		}
	}
	if master && player && state == charactersv1.CharacterState_CHARACTER_STATE_DEAD {
		c.RevivifyBlocked = row.RevivifyBlocked
	}
	if full := sheet.GetFull(); full != nil {
		build := buildOf(full)
		der := rules.Derive(build, content)
		c.Derived = derivedToProto(der)
		addChangedContent(c.Derived, der.Issues, content, build, full)
	}
	return c, nil
}

// setPermissions sets what the caller may do with the character: master says whether
// the caller is the campaign's master, state is the character's.
func setPermissions(c *charactersv1.Character, row charactersdb.Character, state charactersv1.CharacterState, master bool) {
	player := row.Kind == kindPlayer
	dead := state == charactersv1.CharacterState_CHARACTER_STATE_DEAD
	pending := state == charactersv1.CharacterState_CHARACTER_STATE_PENDING
	c.CanEdit = master || playerEditsSheet(state)
	c.CanEditStory = master || playerEditsStory(state, row.StoryEditingAllowed)
	c.CanAccessMasterNotes = master
	c.CanApprove = master && pending
	if !master || !player {
		return // what follows is the master's, on a player's character
	}
	c.CanMarkDead = !row.Reserved && !dead && !pending
	c.CanSetStoryEditing = !row.Reserved
	c.CanRequestChanges = pending
	c.CanRevive = dead
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func timestamp(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

// diedAt is when the character died, unset while it lives: a revived character keeps its
// died_at in the table as the master's history, but it is not dead.
func diedAt(row charactersdb.Character) *timestamppb.Timestamp {
	if row.Status != statusDead {
		return nil
	}
	return timestamp(row.DiedAt)
}
