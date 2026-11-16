package characters

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The steed of Find Steed (Convocar Montaria) is the character's one bonded creature: dismissing it or losing it keeps the
// row, and a new casting brings the same row back at full hit points.

// checkBondedSteed refuses a casting of Find Steed that picks another form or another type than the steed the character is
// bonded to: the bond fixes both.
func (s *Service) checkBondedSteed(ctx context.Context, tx pgx.Tx, campaignID, characterID, spellKey string, made []link.SummonedForm) error {
	if spellKey != "spell:find-steed" || len(made) != 1 {
		return nil
	}
	bonded, err := s.bondedSteedOf(ctx, tx, campaignID, characterID)
	if err != nil || bonded == nil {
		return err
	}
	if bonded.MonsterKey != made[0].MonsterKey {
		return fmt.Errorf("%w: the steed is bonded to another form", rules.ErrSummonCreature)
	}
	if bonded.CreatureType != made[0].CreatureType {
		return fmt.Errorf("%w: the steed is bonded to another type", rules.ErrSummonType)
	}
	return nil
}

// bondedSteedOf is the character's bonded creature, live or dismissed; nil for none.
func (s *Service) bondedSteedOf(ctx context.Context, tx pgx.Tx, campaignID, characterID string) (*charactersdb.CharacterCreature, error) {
	row, err := s.queriesIn(tx).GetBondedCreature(ctx, charactersdb.GetBondedCreatureParams{CampaignID: campaignID, CharacterID: characterID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, wrap("read the bonded steed", err)
	}
	return &row.CharacterCreature, nil
}

// reviveBondedSteed brings the character's bonded steed back at full hit points, live in the roster, and returns it as the
// creature the casting made; nil when the character has none (the casting makes a new one).
func (s *Service) reviveBondedSteed(ctx context.Context, q *charactersdb.Queries, content *rules.Content, sm link.Summon) ([]link.Creature, error) {
	row, err := q.GetBondedCreatureForUpdate(ctx, charactersdb.GetBondedCreatureForUpdateParams{CampaignID: sm.CampaignID, CharacterID: sm.CharacterID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, wrap("read the bonded steed", err)
	}
	if row.CharacterCreature.DismissedAt != nil { // a dismissed steed comes back as one more live creature; a live one adds none
		if err := checkCreatureRoom(ctx, q, link.Summon{CampaignID: sm.CampaignID, CharacterID: sm.CharacterID, Creatures: make([]link.CreatureSpec, 1)}); err != nil {
			return nil, err
		}
	}
	if _, err := q.ReviveBondedCreature(ctx, charactersdb.ReviveBondedCreatureParams{CampaignID: sm.CampaignID, ID: row.CharacterCreature.ID}); err != nil {
		return nil, wrap("bring the steed back", err)
	}
	back, err := q.GetCharacterCreature(ctx, charactersdb.GetCharacterCreatureParams{CampaignID: sm.CampaignID, ID: row.CharacterCreature.ID})
	if err != nil {
		return nil, wrap("read the steed that came back", err)
	}
	return []link.Creature{creatureOf(content, viewOfRow(back))}, nil
}

// steedOptions adds what the sheet offers for a spell that brings a steed: the types to pick from, the Intelligence floor and the
// steed the character is bonded to, which a casting brings back.
func (s *Service) steedOptions(ctx context.Context, q *charactersdb.Queries, content *rules.Content, campaignID, characterID string, own rules.SummonChoices, out *charactersv1.SummonSpellOptions) error {
	if len(own.Options) == 0 || len(own.Options[0].TypeChoices) == 0 {
		return nil
	}
	out.TypeChoices, out.MinInt = own.Options[0].TypeChoices, i32(own.Options[0].MinInt)
	out.Notes = steedNotes()
	bonded, err := s.bondedSteedProto(ctx, q, content, campaignID, characterID)
	out.BondedSteed = bonded
	return err
}

// bondedSteedProto is the character's bonded steed as the API sends it, live or dismissed; nil for none.
func (s *Service) bondedSteedProto(ctx context.Context, q *charactersdb.Queries, content *rules.Content, campaignID, characterID string) (*charactersv1.CharacterCreature, error) {
	row, err := q.GetBondedCreature(ctx, charactersdb.GetBondedCreatureParams{CampaignID: campaignID, CharacterID: characterID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, wrap("read the bonded steed", err)
	}
	return characterCreatureToProto(content, row.CharacterCreature), nil
}

// steedNotes are the rules of Find Steed the server only says (SRD 5.1): the bond, where the steed appears and what it understands.
func steedNotes() []string {
	return []string{
		"Enquanto estiver a até 1,6 km do corcel, você pode se comunicar com ele telepaticamente.",
		"O corcel aparece em um espaço desocupado ao alcance da magia.",
		"O corcel entende o seu idioma e responde a você nele.",
	}
}
