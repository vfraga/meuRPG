package play

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
)

// What traps did outside a combat, as each member may read it (MR-035, RN-10): the
// firings and the searches of the open session that have no combat to put them in the
// log. The master gets all of it; a player only the lines of their own characters, with
// their own dice and "passou" or "falhou", never a DC, and never a trap their
// characters do not know (a trap that fired is public; a search line names only what
// that search found for its own character).

// creatureLabels names the creatures a firing caught outside a combat (their owner's free text, read now and never
// kept in the event: docs/privacy.md), by creature ID.
func (s *Service) creatureLabels(ctx context.Context, campaignID string, caught []trapCaughtEvent) (map[string]string, error) {
	var owners []string
	for _, cc := range caught {
		if creatureOf(cc) {
			owners = append(owners, cc.Character)
		}
	}
	out := map[string]string{}
	if len(owners) == 0 {
		return out, nil
	}
	var found []link.Creature
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		found, err = s.roster.CharacterCreatures(ctx, tx, campaignID, slices.Compact(slices.Sorted(slices.Values(owners))))
		return err
	})
	for _, c := range found {
		out[c.ID] = c.Name
	}
	return out, err
}

// creatureOf says a caught target is a creature of a character (a creature token
// dropped into a trap outside a combat), not a character: it is caught under its
// owner's character ID.
func creatureOf(cc trapCaughtEvent) bool { return cc.Character != "" && cc.Character != cc.Target }

// activityLine is an event of the activity, with the firings that extend it merged in.
type activityLine struct {
	id        string
	at        time.Time
	character string // the searcher's character
	ev        actionEvent
	notice    *noticeLine // set for a passive notice
}

// noticeLine is a passive notice: the trap and the characters that noticed it.
type noticeLine struct {
	point      string
	characters []string
}

// ListTrapActivity implements playv1connect.PlayServiceHandler.
func (s *Service) ListTrapActivity(
	ctx context.Context,
	req *connect.Request[playv1.ListTrapActivityRequest],
) (*connect.Response[playv1.ListTrapActivityResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	session, err := s.openSession(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListTrapEventsOfSession(ctx, session.ID)
	if err != nil {
		return nil, s.dbError(ctx, "list the trap events", err)
	}
	slices.Reverse(rows) // the query returns the newest 500, newest first
	var lines []*activityLine
	byID := map[string]*activityLine{}
	for _, r := range rows {
		var ev actionEvent
		if err := json.Unmarshal(r.Payload, &ev); err != nil {
			continue // never: this package wrote it
		}
		if r.Kind == eventTrapTriggered && ev.Trap != nil && ev.Trap.ExtendsID != "" {
			if host := byID[ev.Trap.ExtendsID]; host != nil && host.ev.Trap != nil {
				host.ev.Trap.Caught = append(host.ev.Trap.Caught, ev.Trap.Caught...) // the creatures it added
			}
			continue
		}
		l := &activityLine{id: r.ID, at: r.CreatedAt, character: deref(r.CharacterID), ev: ev}
		if r.Kind == eventTrapNoticed {
			// The maps module wrote it: the trap and the characters that noticed it.
			var n struct {
				PointID      string   `json:"point_id"`
				CharacterIDs []string `json:"character_ids"`
			}
			if err := json.Unmarshal(r.Payload, &n); err != nil || len(n.CharacterIDs) == 0 {
				continue // never: the maps module wrote it
			}
			l.notice = &noticeLine{point: n.PointID, characters: n.CharacterIDs}
		} else if r.Kind == eventTrapSearched {
			l.ev.Key = ev.Key
		} else if ev.Trap == nil {
			continue
		}
		lines = append(lines, l)
		byID[r.ID] = l
	}

	// Who the caller plays: a player reads only the lines of their own characters.
	master := m.Role == authz.RoleMaster
	owned := map[string]bool{}
	if !master {
		party, err := s.roster.CombatParty(ctx, nil, m.CampaignID)
		if err != nil {
			return nil, s.dbError(ctx, "read the party", err)
		}
		for _, c := range party {
			if c.PlayerUserID != "" && c.PlayerUserID == m.UserID {
				owned[c.ID] = true
			}
		}
	}

	var pointIDs, characterIDs, damageIDs, foundIDs, ownerIDs []string
	for _, l := range lines {
		if l.notice != nil {
			pointIDs = append(pointIDs, l.notice.point)
			characterIDs = append(characterIDs, l.notice.characters...)
		} else if l.ev.Trap != nil {
			pointIDs = append(pointIDs, l.ev.Trap.PointID)
			for _, cc := range l.ev.Trap.Caught {
				if creatureOf(cc) {
					ownerIDs = append(ownerIDs, cc.Character) // its name is its owner's to give: read below
				} else {
					characterIDs = append(characterIDs, cc.Target)
				}
				for _, d := range cc.Damages {
					if d.Pending != "" {
						damageIDs = append(damageIDs, d.Pending)
					}
				}
			}
		} else {
			characterIDs = append(characterIDs, l.character)
			foundIDs = append(foundIDs, l.ev.Found...)
		}
	}
	trapNames := map[string]string{}
	if s.traps != nil && len(pointIDs)+len(foundIDs) > 0 {
		all := slices.Compact(slices.Sorted(slices.Values(append(slices.Clone(pointIDs), foundIDs...))))
		if trapNames, err = s.traps.TrapNames(ctx, m.CampaignID, all); err != nil {
			return nil, s.dbError(ctx, "read the traps' names", err)
		}
	}
	names := map[string]string{}
	if len(characterIDs) > 0 {
		chars, err := s.roster.SessionCharacters(ctx, nil, m.CampaignID, slices.Compact(slices.Sorted(slices.Values(characterIDs))))
		if err != nil {
			return nil, s.dbError(ctx, "read the characters' names", err)
		}
		for _, c := range chars {
			names[c.ID] = c.Name
		}
	}
	// A creature dropped into a trap is caught under its owner's character: its name is
	// the owner's free text, read now and never kept in the event (docs/privacy.md).
	creatureNames := map[string]string{}
	if len(ownerIDs) > 0 {
		var found []link.Creature
		err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
			var err error
			found, err = s.roster.CharacterCreatures(ctx, tx, m.CampaignID, slices.Compact(slices.Sorted(slices.Values(ownerIDs))))
			return err
		})
		if err != nil {
			return nil, s.dbError(ctx, "read the creatures' names", err)
		}
		for _, c := range found {
			creatureNames[c.ID] = c.Name
		}
	}
	statuses := map[string]playv1.PendingDamageStatus{}
	applied := map[string]int32{}
	if len(damageIDs) > 0 {
		rows, err := s.queries.ListTrapDamageStatuses(ctx, damageIDs)
		if err != nil {
			return nil, s.dbError(ctx, "read the trap damages", err)
		}
		for _, r := range rows {
			statuses[r.ID] = map[string]playv1.PendingDamageStatus{
				"rolled": playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_ROLLED, "applied": playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED,
				"discarded": playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_DISCARDED,
			}[r.Status]
			if r.AppliedAmount != nil {
				applied[r.ID] = *r.AppliedAmount
			}
		}
	}

	res := &playv1.ListTrapActivityResponse{}
	for _, l := range lines {
		out := &playv1.TrapActivity{Id: l.id, At: timestamppb.New(l.at)}
		if nl := l.notice; nl != nil {
			// RN-10: a player reads only the notices of their own characters; the line
			// names the trap because they know it now, and carries no DC.
			notice := &playv1.TrapNotice{PointId: nl.point, TrapName: trapNames[nl.point]}
			for _, id := range nl.characters {
				if master || owned[id] {
					notice.CharacterIds = append(notice.CharacterIds, id)
					notice.CharacterNames = append(notice.CharacterNames, names[id])
				}
			}
			if len(notice.CharacterIds) == 0 {
				continue
			}
			out.Notice = notice
		} else if tr := l.ev.Trap; tr != nil {
			shown := *tr
			shown.Caught = nil
			ownedCreatures := map[string]bool{} // the creatures of the characters the viewer plays
			for _, cc := range tr.Caught {
				if creatureOf(cc) && owned[cc.Character] {
					ownedCreatures[cc.Target] = true
				}
			}
			for _, cc := range tr.Caught {
				if master || owned[cc.Target] || (creatureOf(cc) && owned[cc.Character]) {
					shown.Caught = append(shown.Caught, cc)
				}
			}
			if len(shown.Caught) == 0 && !master {
				continue
			}
			out.Firing = firingProto(&shown, l.id, trapNames[tr.PointID], trapView{
				master: master,
				owns:   func(id string) bool { return owned[id] || ownedCreatures[id] },
				label: func(id string) string {
					if n, ok := creatureNames[id]; ok {
						return n
					}
					return names[id]
				},
				status: func(id string) (playv1.PendingDamageStatus, bool) { st, ok := statuses[id]; return st, ok },
				amount: func(id string) (int32, bool) { a, ok := applied[id]; return a, ok },
			})
		} else {
			if !master && !owned[l.character] {
				continue
			}
			sr := &playv1.TrapSearchResult{
				CharacterId: l.character, CharacterName: names[l.character], FoundPointIds: l.ev.Found,
				Roll: diceRoll(1, 20, []int32{l.ev.D20}, l.ev.Modifier, l.ev.Total, l.ev.Physical),
			}
			if l.ev.Key == searchPerception {
				sr.Skill = playv1.TrapSearchSkill_TRAP_SEARCH_SKILL_PERCEPTION
			} else {
				sr.Skill = playv1.TrapSearchSkill_TRAP_SEARCH_SKILL_INVESTIGATION
			}
			if l.ev.D20B != 0 {
				sr.SecondRoll = diceRoll(1, 20, []int32{l.ev.D20B}, l.ev.Modifier, l.ev.D20B+l.ev.Modifier, l.ev.Physical)
			}
			for _, id := range l.ev.Found {
				sr.FoundNames = append(sr.FoundNames, trapNames[id])
			}
			out.Search = sr
		}
		res.Activity = append(res.Activity, out)
	}
	return connect.NewResponse(res), nil
}
