package characters

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The characters that fight (MR-013). Package play declares what it needs
// (its CombatRoster interface) and this Service implements it with
// CombatParty and CombatCharacters, like the vitals: the numbers of a
// combatant (initiative bonus, speed, an NPC's hit points) come from the
// sheet, so they are read here, where the sheets are. Package play never
// touches the characters table. The methods take no caller: they run after
// play's own authorization check, and the master's notes never go through
// them (RN-11).

// CombatParty returns the campaign's living, active player characters,
// oldest first: the party that fights by default. Not NPCs, not the dead,
// not a character waiting for approval.
func (s *Service) CombatParty(ctx context.Context, tx pgx.Tx, campaignID string) ([]link.Character, error) {
	rows, err := s.queriesIn(tx).ListCombatParty(ctx, campaignID)
	if err != nil {
		return nil, s.dbError(ctx, "list the party", err)
	}
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	out := make([]link.Character, 0, len(rows))
	for _, row := range rows {
		c, err := combatCharacter(content, row.ID, row.Kind, row.Name, row.PlayerUserID, row.Sheet, row.WildShapeBeast, row.ActiveStates)
		if err != nil {
			return nil, s.dbError(ctx, "list the party", err)
		}
		out = append(out, c)
	}
	return out, nil
}

// PartyLevels returns the campaign's living, active player characters, oldest
// first, with the total level of each (the sum of its class levels): the party an
// encounter is measured against (MR-043). Not NPCs, not the dead, not a
// character waiting for approval. It implements play.CombatRoster.
func (s *Service) PartyLevels(ctx context.Context, tx pgx.Tx, campaignID string) ([]link.PartyMember, error) {
	rows, err := s.queriesIn(tx).ListCombatParty(ctx, campaignID)
	if err != nil {
		return nil, s.dbError(ctx, "list the party", err)
	}
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	out := make([]link.PartyMember, 0, len(rows))
	for _, row := range rows {
		sheet, err := loadSheet(row.ID, row.Sheet)
		if err != nil {
			return nil, s.dbError(ctx, "list the party", err)
		}
		if sheet.GetFull() == nil {
			continue // never: a player character has a full sheet
		}
		// The sheet's own level, not the beast's of a druid in Wild Shape: no beast here.
		out = append(out, link.PartyMember{ID: row.ID, Name: row.Name, Level: rules.Derive(buildOf(sheet.GetFull()), content).TotalLevel})
	}
	return out, nil
}

// RulesContent is the rules content of the campaign (the SRD, plus what the table
// added), read in tx (nil outside one). Package play reads the SRD's creatures and
// the encounter budget through it. It implements play.CombatRoster.
func (s *Service) RulesContent(ctx context.Context, tx pgx.Tx, campaignID string) (*rules.Content, error) {
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return nil, wrap("read rules content", err)
	}
	return content, nil
}

// SessionCharacters returns those of ids that are characters of the campaign,
// whatever their status (a dead one included), with their name and player
// only. It implements play.CombatRoster.
func (s *Service) SessionCharacters(ctx context.Context, tx pgx.Tx, campaignID string, ids []string) ([]link.Character, error) {
	valid := make([]string, 0, len(ids))
	for _, id := range ids {
		if id, ok := uid.Canonical(id); ok {
			valid = append(valid, id)
		}
	}
	if len(valid) == 0 {
		return nil, nil
	}
	rows, err := s.queriesIn(tx).ListSessionCharacters(ctx, charactersdb.ListSessionCharactersParams{CampaignID: campaignID, Ids: valid})
	if err != nil {
		return nil, s.dbError(ctx, "list the characters of a session", err)
	}
	out := make([]link.Character, 0, len(rows))
	for _, r := range rows {
		out = append(out, link.Character{ID: r.ID, Name: r.Name, Player: r.Kind == "player", PlayerUserID: conv.OrZero(r.PlayerUserID), Reserved: r.Reserved})
	}
	return out, nil
}

// CombatCharacters returns those of ids that may fight in a combat of the
// campaign: its living characters, players' or NPCs, oldest first. IDs that
// are not UUIDs, or name any other character, are left out.
func (s *Service) CombatCharacters(ctx context.Context, tx pgx.Tx, campaignID string, ids []string) ([]link.Character, error) {
	valid := make([]string, 0, len(ids))
	for _, id := range ids {
		if id, ok := uid.Canonical(id); ok {
			valid = append(valid, id)
		}
	}
	if len(valid) == 0 {
		return nil, nil
	}
	rows, err := s.queriesIn(tx).ListCombatCharacters(ctx, charactersdb.ListCombatCharactersParams{CampaignID: campaignID, Ids: valid})
	if err != nil {
		return nil, s.dbError(ctx, "list the characters of a combat", err)
	}
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	out := make([]link.Character, 0, len(rows))
	for _, row := range rows {
		c, err := combatCharacter(content, row.ID, row.Kind, row.Name, row.PlayerUserID, row.Sheet, row.WildShapeBeast, row.ActiveStates)
		if err != nil {
			return nil, s.dbError(ctx, "list the characters of a combat", err)
		}
		out = append(out, c)
	}
	return out, nil
}

// combatCharacter reads the numbers a combatant starts with from a stored
// sheet: derived by the rules for a full sheet (a player, an enemy, a boss),
// as written for a basic one (a minion, a story NPC). A druid in Wild Shape
// (beast) starts, and keeps, with the beast's speed, size and jumps (MR-037).
func combatCharacter(content *rules.Content, id, kind, name string, playerUserID *string, doc []byte, beast *string, active []byte) (link.Character, error) {
	sheet, err := loadSheet(id, doc)
	if err != nil {
		return link.Character{}, err
	}
	c := link.Character{ID: id, Name: name, Player: kind == "player", PlayerUserID: conv.OrZero(playerUserID)}
	switch {
	case sheet.GetFull() != nil:
		d := derive(content, sheet.GetFull(), beast, active)
		c.InitiativeBonus, c.SpeedFt, c.HitPointsMax = d.Initiative, d.SpeedWalkFt, max(d.HitPointsMax, 0)
		c.SpeedFlyFt, c.Size = d.SpeedFlyFt, raceSizeOf(content, d, sheet.GetFull().GetRaceKey())
		if beast != nil && *beast != "" {
			c.Size = beastSize(content, *beast)
		}
		c.CastsInBeastForm = content.CastsInBeastForm(d)
		c.WildShapeCost = 1
		if d.WildShape != nil {
			c.WildShapeCost = conv.Int32(d.WildShape.Cost)
		}
		c.ArmorUntrained = armorTrainingOf(d, kind, beast).Any()
		jumps := combat.JumpLimits(d)
		c.JumpLongDFt, c.JumpHighDFt, c.JumpLongBonusDFt = jumps.LongRunning, jumps.HighRunning, jumps.LongRunningBonus
		if !c.Player {
			c.XPValue = int(sheet.GetFull().GetXpValue())
			c.PortraitImageID = sheet.GetFull().GetPortraitImageId()
		}
	case sheet.GetBasic() != nil:
		b := sheet.GetBasic()
		c.InitiativeBonus, c.SpeedFt, c.HitPointsMax = int(b.GetInitiativeBonus()), int(b.GetSpeedFt()), int(b.GetHitPointsMax())
		c.XPValue = int(b.GetXpValue())
		c.PortraitImageID = b.GetPortraitImageId()
		c.Size = sizeKey(b.GetSize())
		c.CombatOnly, c.MonsterKey, c.ChallengeRating = b.GetCombatOnly(), b.GetMonsterKey(), b.GetChallengeRating()
	default:
		return link.Character{}, fmt.Errorf("%w: the sheet of character %s has no content", errCorruptDocument, id)
	}
	return c, nil
}

// raceSizeOf is the size of a character, as a link.Character.Size: the one the sheet picked among
// the race's sizes, or the race's own.
func raceSizeOf(content *rules.Content, d rules.Derived, raceKey string) string {
	if d.Size != "" {
		return strings.ToLower(d.Size)
	}
	return raceSize(content, raceKey)
}

// raceSize is the size of a race, as a link.Character.Size: "medium" for a race
// the content does not know.
func raceSize(content *rules.Content, raceKey string) string {
	for _, r := range content.Catalog().Races {
		if r.Key == raceKey {
			return strings.ToLower(r.Size)
		}
	}
	return "medium"
}

// sizeKey is a basic sheet's Tamanho as a link.Character.Size: "medium" when the
// sheet says none.
func sizeKey(z rulesv1.CreatureSize) string {
	if z == rulesv1.CreatureSize_CREATURE_SIZE_UNSPECIFIED {
		return "medium"
	}
	return strings.ToLower(strings.TrimPrefix(z.String(), "CREATURE_SIZE_"))
}
