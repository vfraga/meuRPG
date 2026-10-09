package play

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
	"github.com/PuraFome/meuRPG/backend/internal/rules/zone"
)

// The calls that make and change zones (W7-Z). Every one that changes a zone carries an
// idempotency key; the master makes them all, and a caster moves their own zone when the
// spell lets them (Moonbeam). The calls answer with the zone; the combat is read again from
// `encounter_changed`, which is how a zone reaches the players that see it.

// maxZoneName is the longest name of a zone the master puts.
const maxZoneName = 80

// maxScenarioSquares is the biggest radius or side of a zone the master puts, in squares: 36 is
// the radius of the biggest fog a spell can leave (a Fog Cloud at the 9th level).
const maxScenarioSquares = 36

// maxZoneRounds is the longest a zone the master puts may last: a day is 14400 rounds, and
// the clock of a combat does not go that far.
const maxZoneRounds = 6000

// zoneNotFound is what anyone but the master (and the caster, for a zone they move) gets.
func zoneNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("zone not found"))
}

// zoneWrite is the common frame of the master's calls on a zone: the key, the combat, and the
// zone it is about, read in the change's transaction.
func (s *Service) zoneWrite(ctx context.Context, m authz.Membership, key string, hash *string, kind, encID string, do func(c *combatTx) (any, error)) (combatResult, error) {
	return s.write(ctx, combatWrite{m: m, key: key, hash: hash, kind: kind, encounterID: encID}, do)
}

// publishZoneChanged tells the table a zone changed: the master always; the players too when the
// zone is one they are told of or one that hides creatures from them, so that they read the
// combat again (the content-free hint that every change makes).
func (s *Service) publishZoneChanged(ctx context.Context, campaignID string, d *encounterData, tellPlayers bool) {
	if tellPlayers {
		s.publishEncounterChanged(ctx, campaignID, d.enc)
		return
	}
	s.hub.Publish(campaignID, live.Event{Audience: live.Audience{Master: true}, Message: encounterChangedMessage(d.enc)})
}

// tellsPlayers says a change of the zone reaches the players: they see it, or it hides creatures.
func (z zoneState) tellsPlayers() bool {
	return z.row.VisibleToPlayers || z.blocksSight() || len(z.row.Rules) > 0
}

// ListMapZones implements playv1connect.CombatServiceHandler.
func (s *Service) ListMapZones(
	ctx context.Context,
	req *connect.Request[playv1.ListMapZonesRequest],
) (*connect.Response[playv1.ListMapZonesResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}
	_, d, err := s.readEncounter(ctx, m.CampaignID, encID)
	if err != nil {
		return nil, err
	}
	v, err := s.viewerFor(ctx, m, d.enc, d.cs)
	if err != nil {
		return nil, s.dbError(ctx, "work out what the player sees", err)
	}
	zones, _, err := s.zonesView(ctx, d, v)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.ListMapZonesResponse{Zones: zones}), nil
}

// zonesView is the zones the viewer is told of, and, in a combat without a map, for a player the
// zones their character knows and whether it is inside each.
func (s *Service) zonesView(_ context.Context, d *encounterData, v combatViewer) ([]*playv1.MapZone, []*playv1.ZoneSelf, error) {
	theatre := isTheatre(d.enc)
	var zones []*playv1.MapZone
	var self []*playv1.ZoneSelf
	for _, z := range zoneStatesOf(d.zones) {
		if !z.seenBy(v, d.cs) {
			continue
		}
		if theatre && !v.master {
			in := false
			for _, c := range d.cs {
				if v.owns(c) && z.inside(c, true) {
					in = true
				}
			}
			self = append(self, &playv1.ZoneSelf{ZoneId: z.row.ID, SpellKey: z.row.SpellKey, Name: z.row.Name, SelfInside: in})
			continue
		}
		zones = append(zones, zoneProto(z, v, d.cs, d.enc.Round))
	}
	return zones, self, nil
}

// AddMapZone implements playv1connect.CombatServiceHandler.
func (s *Service) AddMapZone(
	ctx context.Context,
	req *connect.Request[playv1.AddMapZoneRequest],
) (*connect.Response[playv1.AddMapZoneResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}
	in, err := s.scenarioZoneOf(ctx, m.CampaignID, req.Msg)
	if err != nil {
		return nil, err
	}
	var made zoneState
	res, err := s.zoneWrite(ctx, m, key, idem.Hash(req.Msg), eventMapZoneAdded, encID, func(c *combatTx) (any, error) {
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		b := in
		if isTheatre(c.enc) {
			b.cells, b.origin = nil, nil
		} else {
			if req.Msg.GetOrigin() == nil || !inGrid(c.enc, req.Msg.GetOrigin().GetCol(), req.Msg.GetOrigin().GetRow()) {
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("origin must be a square of the grid"))
			}
			terrain, err := s.terrainOf(ctx, c.tx, m.CampaignID, c.enc)
			if err != nil {
				return nil, err
			}
			origin := grid.Square{Col: int(req.Msg.GetOrigin().GetCol()), Row: int(req.Msg.GetOrigin().GetRow())}
			b.origin = &origin
			b.cells = terrain.OpenArea(origin, scenarioShape(b.shape, b.sizeFt, origin), b.obscurity.BlocksSight())
		}
		z, err := s.insertZone(ctx, c, b)
		if err != nil {
			return nil, err
		}
		made = z
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		if err := s.syncZoneRules(ctx, c); err != nil {
			return nil, err
		}
		return actionEvent{Round: c.enc.Round, Secret: !z.tellsPlayers(), Zone: &zoneNote{ID: z.row.ID, What: "added"}}, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "put a zone", err)
	}
	var zoneID string
	if ev, err := resultEvent(res, actionEvent{}); err == nil && ev.Zone != nil {
		zoneID = ev.Zone.ID
	}
	return zoneAnswer(ctx, s, m, res, zoneID, made, func(d *encounterData, z zoneState) {
		s.publishZoneChanged(ctx, m.CampaignID, d, z.tellsPlayers())
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, z.tellsPlayers())
	}, func(z *playv1.MapZone) *playv1.AddMapZoneResponse { return &playv1.AddMapZoneResponse{Zone: z} })
}

// zoneAnswer finishes a call on a zone: it publishes and builds the answer from the zone as it
// stands. A retry (the closure never ran) finds the zone from the event's note.
func zoneAnswer[R any](ctx context.Context, s *Service, m authz.Membership, res combatResult, zoneID string, made zoneState, publish func(d *encounterData, z zoneState), build func(*playv1.MapZone) *R) (*connect.Response[R], error) {
	var z zoneState
	_, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		z = made
		if z.row.ID == "" {
			if rows, err := s.queries.ListAllMapZones(ctx, d.enc.ID); err == nil {
				if i := slices.IndexFunc(rows, func(r playdb.MapZone) bool { return r.ID == zoneID }); i >= 0 {
					z = zoneStateOf(rows[i])
				}
			}
		}
		publish(d, z)
	})
	if err != nil {
		return nil, err
	}
	if z.row.ID == "" {
		return connect.NewResponse(build(nil)), nil
	}
	cs, err := s.queries.ListCombatants(ctx, z.row.EncounterID)
	if err != nil {
		return nil, s.dbError(ctx, "list the combatants", err)
	}
	enc, err := s.queries.GetEncounterInSession(ctx, playdb.GetEncounterInSessionParams{GameSessionID: res.session.ID, ID: res.encounterID})
	if err != nil {
		return nil, s.dbError(ctx, "find the encounter", err)
	}
	return connect.NewResponse(build(zoneProto(z, viewerOf(m), cs, enc.Round))), nil
}

// scenarioZoneOf checks the request of a zone the master puts and works out the row it makes,
// but for the squares, which the transaction works out from the terrain.
func (s *Service) scenarioZoneOf(ctx context.Context, campaignID string, req *playv1.AddMapZoneRequest) (zoneBuild, error) {
	bad := func(msg string) error { return connect.NewError(connect.CodeInvalidArgument, errors.New(msg)) }
	name := strings.TrimSpace(req.GetName())
	if name == "" || utf8.RuneCountInString(name) > maxZoneName {
		return zoneBuild{}, bad(fmt.Sprintf("name must be 1 to %d characters", maxZoneName))
	}
	b := zoneBuild{name: name, visible: req.GetVisibleToPlayers(), anchored: true}
	switch req.GetShape() {
	case playv1.ZoneShape_ZONE_SHAPE_SPHERE:
		b.shape = zoneSphere
	case playv1.ZoneShape_ZONE_SHAPE_CUBE:
		b.shape = zoneCube
	case playv1.ZoneShape_ZONE_SHAPE_CYLINDER:
		b.shape = zoneCylinder
	default:
		return zoneBuild{}, bad("shape must be a sphere, a cube or a cylinder")
	}
	size := int(req.GetSizeSquares())
	if size < 1 || size > maxScenarioSquares {
		return zoneBuild{}, bad(fmt.Sprintf("size_squares must be 1 to %d", maxScenarioSquares))
	}
	b.sizeFt = size * grid.FeetPerSquare
	if d := req.GetDurationRounds(); d < 0 || d > maxZoneRounds {
		return zoneBuild{}, bad(fmt.Sprintf("duration_rounds must be 0 to %d", maxZoneRounds))
	}
	b.duration = req.GetDurationRounds()
	fx := req.GetEffects()
	switch fx.GetObscurity() {
	case playv1.ZoneObscurity_ZONE_OBSCURITY_UNSPECIFIED:
	case playv1.ZoneObscurity_ZONE_OBSCURITY_LIGHT:
		b.obscurity = zone.ObscureLight
	case playv1.ZoneObscurity_ZONE_OBSCURITY_HEAVY:
		b.obscurity = zone.ObscureHeavy
	case playv1.ZoneObscurity_ZONE_OBSCURITY_DARK:
		b.obscurity = zone.ObscureDark
	default:
		return zoneBuild{}, bad("obscurity must be none, light, heavy or dark")
	}
	b.difficult = fx.GetDifficult()
	if fx.GetDamages() {
		count, sides := int(fx.GetDamageCount()), int(fx.GetDamageSides())
		if count < 1 || count > 20 || sides < 2 || sides > 100 {
			return zoneBuild{}, bad("the damage needs 1 to 20 dice of 2 to 100 sides")
		}
		if !strings.HasPrefix(fx.GetDamageTypeKey(), "damage-type:") || s.namesFor(ctx, campaignID)(fx.GetDamageTypeKey()) == "" {
			return zoneBuild{}, bad("damage_type_key must be one of the SRD's damage types")
		}
		b.dmg = zoneDamage{count: count, sides: sides, kind: fx.GetDamageTypeKey()}
		tr := zone.Trigger{Damage: true, OnSuccess: zone.SuccessNone}
		if ab := fx.GetSaveAbility(); ab != "" {
			if !slices.Contains([]string{"str", "dex", "con", "int", "wis", "cha"}, ab) {
				return zoneBuild{}, bad("save_ability must be str, dex, con, int, wis or cha")
			}
			dc := int(fx.GetSaveDc())
			if dc < 1 || dc > 30 {
				return zoneBuild{}, bad("save_dc must be 1 to 30")
			}
			b.dc, tr.Save = dc, ab
			if fx.GetHalfOnSuccess() {
				tr.OnSuccess = zone.SuccessHalf
			}
		} else if fx.GetSaveDc() != 0 || fx.GetHalfOnSuccess() {
			return zoneBuild{}, bad("save_dc and half_on_success are for damage with a saving throw")
		}
		enter, start := tr, tr
		enter.Kind, start.Kind = zone.OnEnterFirstTimeOnATurn, zone.StartOfTurn
		b.triggers = []zone.Trigger{enter, start}
	} else if fx.GetSaveAbility() != "" || fx.GetSaveDc() != 0 || fx.GetDamageCount() != 0 || fx.GetDamageSides() != 0 || fx.GetDamageTypeKey() != "" || fx.GetHalfOnSuccess() {
		return zoneBuild{}, bad("the save and the damage are for a zone that damages")
	}
	return b, nil
}

// MoveMapZone implements playv1connect.CombatServiceHandler.
func (s *Service) MoveMapZone(
	ctx context.Context,
	req *connect.Request[playv1.MoveMapZoneRequest],
) (*connect.Response[playv1.MoveMapZoneResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}
	zoneID, err := parseCombatID(req.Msg.GetZoneId(), "zone")
	if err != nil {
		return nil, err
	}
	if req.Msg.GetOrigin() == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("origin is required"))
	}
	var made zoneState
	res, err := s.zoneWrite(ctx, m, key, idem.Hash(req.Msg), eventMapZoneChanged, encID, func(c *combatTx) (any, error) {
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		if isTheatre(c.enc) {
			return nil, errNeedsAMap()
		}
		row, err := c.q.GetMapZone(ctx, playdb.GetMapZoneParams{EncounterID: c.enc.ID, ID: zoneID})
		if err != nil {
			return nil, zoneNotFound()
		}
		z := zoneStateOf(row)
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		v := c.viewer(m, cs)
		var caster playdb.Combatant
		if row.CasterID != nil {
			if i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == *row.CasterID }); i >= 0 {
				caster = cs[i]
			}
		}
		spec, hasSpec := zone.Lookup(row.SpellKey)
		if !inGrid(c.enc, req.Msg.GetOrigin().GetCol(), req.Msg.GetOrigin().GetRow()) {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("origin must be a square of the grid"))
		}
		to := grid.Square{Col: int(req.Msg.GetOrigin().GetCol()), Row: int(req.Msg.GetOrigin().GetRow())}
		if !v.master {
			// The caster moves their own beam with an action, up to the spell's distance from where it is
			// now (SRD, Moonbeam: 60 ft in any direction; not the range of the cast).
			if caster.ID == "" || !v.owns(caster) || !hasSpec || !spec.CasterMoves || row.OriginCol == nil {
				return nil, zoneNotFound()
			}
			if err := s.mustActNow(ctx, c, caster); err != nil {
				return nil, err
			}
			if caster.ActionUsed {
				return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED, "the action of this turn is used")
			}
			from := grid.Square{Col: int(*row.OriginCol), Row: int(*row.OriginRow)}
			if dist := grid.RangeFt(from, to); dist > spec.MoveFt {
				return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_OUT_OF_REACH, "the zone moves less than that",
					func(b *playv1.EncounterBlocked) { b.MissingFt = clamp32(dist-spec.MoveFt, 0, math.MaxInt32) })
			}
			if err := c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
				ID: caster.ID, ActionUsed: true, BonusActionUsed: caster.BonusActionUsed, ReactionUsed: caster.ReactionUsed, Dashed: caster.Dashed,
			}); err != nil {
				return nil, fmt.Errorf("spend the action: %w", err)
			}
		} else if err := s.mustNotWaitForZone(ctx, c); err != nil {
			return nil, err
		}
		terrain, err := s.terrainOf(ctx, c.tx, m.CampaignID, c.enc)
		if err != nil {
			return nil, err
		}
		if err := s.moveZoneTo(ctx, c, cs, z, to, terrain, 0); err != nil {
			return nil, err
		}
		for _, zz := range c.zones {
			if zz.row.ID == z.row.ID {
				made = zz
			}
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		return actionEvent{Round: c.enc.Round, Secret: !z.tellsPlayers(), Zone: &zoneNote{ID: z.row.ID, What: "moved"}}, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "move a zone", err)
	}
	return zoneAnswer(ctx, s, m, res, zoneID, made, func(d *encounterData, z zoneState) {
		s.publishZoneChanged(ctx, m.CampaignID, d, z.tellsPlayers())
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, z.tellsPlayers())
	}, func(z *playv1.MapZone) *playv1.MoveMapZoneResponse { return &playv1.MoveMapZoneResponse{Zone: z} })
}

// EndMapZone implements playv1connect.CombatServiceHandler.
func (s *Service) EndMapZone(
	ctx context.Context,
	req *connect.Request[playv1.EndMapZoneRequest],
) (*connect.Response[playv1.EndMapZoneResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}
	zoneID, err := parseCombatID(req.Msg.GetZoneId(), "zone")
	if err != nil {
		return nil, err
	}
	res, err := s.zoneWrite(ctx, m, key, idem.Hash(req.Msg), eventMapZoneChanged, encID, func(c *combatTx) (any, error) {
		var tell bool
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		row, err := c.q.GetMapZone(ctx, playdb.GetMapZoneParams{EncounterID: c.enc.ID, ID: zoneID})
		if err != nil {
			return nil, zoneNotFound()
		}
		z := zoneStateOf(row)
		tell = z.tellsPlayers()
		// A zone of a concentration ends the caster's concentration with it, and what else the
		// concentration held (SRD, "Concentration"): the same transaction.
		if err := s.endSpell(ctx, c, z, whatEnded); err != nil {
			return nil, err
		}
		if err := s.syncZoneRules(ctx, c); err != nil {
			return nil, err
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		return actionEvent{Round: c.enc.Round, Secret: !tell, Zone: &zoneNote{ID: zoneID, What: "ended"}}, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "end a zone", err)
	}
	if _, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishZoneChanged(ctx, m.CampaignID, d, true)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, true)
		for _, c := range d.cs {
			if c.UserID != nil && c.ConcentrationSpell == nil {
				s.publishCreaturesChanged(m.CampaignID, *c.UserID)
			}
		}
	}); err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.EndMapZoneResponse{}), nil
}

// windFor is the wind a request names.
func windFor(w playv1.ZoneWind) (zone.Wind, bool) {
	switch w {
	case playv1.ZoneWind_ZONE_WIND_MODERATE:
		return zone.WindModerate, true
	case playv1.ZoneWind_ZONE_WIND_STRONG:
		return zone.WindStrong, true
	}
	return "", false
}

// DisperseMapZone implements playv1connect.CombatServiceHandler.
func (s *Service) DisperseMapZone(
	ctx context.Context,
	req *connect.Request[playv1.DisperseMapZoneRequest],
) (*connect.Response[playv1.DisperseMapZoneResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}
	zoneID, err := parseCombatID(req.Msg.GetZoneId(), "zone")
	if err != nil {
		return nil, err
	}
	wind, ok := windFor(req.Msg.GetWind())
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("wind must be moderate or strong"))
	}
	var endsIn int32
	var made zoneState
	res, err := s.zoneWrite(ctx, m, key, idem.Hash(req.Msg), eventMapZoneChanged, encID, func(c *combatTx) (any, error) {
		endsIn, made = 0, zoneState{}
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		row, err := c.q.GetMapZone(ctx, playdb.GetMapZoneParams{EncounterID: c.enc.ID, ID: zoneID})
		if err != nil {
			return nil, zoneNotFound()
		}
		z := zoneStateOf(row)
		spec, ok := zone.Lookup(row.SpellKey)
		rounds, disperses := spec.Dispersal[wind]
		if !ok || !disperses {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("that wind does not disperse this zone"))
		}
		note := &zoneNote{ID: zoneID, What: "dispersed"}
		if rounds == 0 {
			if err := s.endSpell(ctx, c, z, whatDispersed); err != nil {
				return nil, err
			}
		} else {
			when := c.enc.Round + clamp32(rounds, 0, 1000)
			if row.DisperseRound != nil && *row.DisperseRound <= when {
				when = *row.DisperseRound // a stronger wind already told it to go sooner
			}
			if err := c.q.SetMapZoneDisperseRound(ctx, playdb.SetMapZoneDisperseRoundParams{ID: zoneID, DisperseRound: &when}); err != nil {
				return nil, fmt.Errorf("set the round a wind disperses the zone: %w", err)
			}
			endsIn, note.Moved = when, clamp32(rounds, 0, 1000)
			if err := c.reloadZones(ctx); err != nil {
				return nil, err
			}
			for _, zz := range c.zones {
				if zz.row.ID == zoneID {
					made = zz
				}
			}
		}
		if err := s.syncZoneRules(ctx, c); err != nil {
			return nil, err
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		return actionEvent{Round: c.enc.Round, Secret: !z.tellsPlayers(), Zone: note}, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "disperse a zone", err)
	}
	if _, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishZoneChanged(ctx, m.CampaignID, d, true)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, true)
	}); err != nil {
		return nil, err
	}
	out := &playv1.DisperseMapZoneResponse{EndsInRound: endsIn}
	if made.row.ID != "" {
		cs, err := s.queries.ListCombatants(ctx, made.row.EncounterID)
		if err != nil {
			return nil, s.dbError(ctx, "list the combatants", err)
		}
		out.Zone = zoneProto(made, viewerOf(m), cs, made.row.CastRound)
	}
	return connect.NewResponse(out), nil
}

// SetMapZoneVisible implements playv1connect.CombatServiceHandler.
func (s *Service) SetMapZoneVisible(
	ctx context.Context,
	req *connect.Request[playv1.SetMapZoneVisibleRequest],
) (*connect.Response[playv1.SetMapZoneVisibleResponse], error) {
	m, key, encID, zoneID, err := zoneMasterCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId(), req.Msg.GetZoneId())
	if err != nil {
		return nil, err
	}
	var made zoneState
	res, err := s.zoneWrite(ctx, m, key, idem.Hash(req.Msg), eventMapZoneChanged, encID, func(c *combatTx) (any, error) {
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		row, err := c.q.GetMapZone(ctx, playdb.GetMapZoneParams{EncounterID: c.enc.ID, ID: zoneID})
		if err != nil {
			return nil, zoneNotFound()
		}
		if err := c.q.SetMapZoneVisible(ctx, playdb.SetMapZoneVisibleParams{ID: zoneID, VisibleToPlayers: req.Msg.GetVisibleToPlayers()}); err != nil {
			return nil, fmt.Errorf("set whether the players see the zone: %w", err)
		}
		row.VisibleToPlayers = req.Msg.GetVisibleToPlayers()
		made = zoneStateOf(row)
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		return actionEvent{Round: c.enc.Round, Secret: true, Zone: &zoneNote{ID: zoneID, What: "changed"}}, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "set whether the players see a zone", err)
	}
	return zoneAnswer(ctx, s, m, res, zoneID, made, func(d *encounterData, z zoneState) {
		s.publishZoneChanged(ctx, m.CampaignID, d, true)
	}, func(z *playv1.MapZone) *playv1.SetMapZoneVisibleResponse {
		return &playv1.SetMapZoneVisibleResponse{Zone: z}
	})
}

// SetMapZoneKnown implements playv1connect.CombatServiceHandler.
func (s *Service) SetMapZoneKnown(
	ctx context.Context,
	req *connect.Request[playv1.SetMapZoneKnownRequest],
) (*connect.Response[playv1.SetMapZoneKnownResponse], error) {
	m, key, encID, zoneID, err := zoneMasterCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId(), req.Msg.GetZoneId())
	if err != nil {
		return nil, err
	}
	who, err := parseCombatID(req.Msg.GetCombatantId(), "combatant")
	if err != nil {
		return nil, err
	}
	var made zoneState
	res, err := s.zoneWrite(ctx, m, key, idem.Hash(req.Msg), eventMapZoneChanged, encID, func(c *combatTx) (any, error) {
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		row, err := c.q.GetMapZone(ctx, playdb.GetMapZoneParams{EncounterID: c.enc.ID, ID: zoneID})
		if err != nil {
			return nil, zoneNotFound()
		}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		if _, err := findCombatant(cs, who, viewerOf(m)); err != nil {
			return nil, err
		}
		known := slices.Clone(row.KnownBy)
		switch has := slices.Contains(known, who); {
		case req.Msg.GetKnown() && !has:
			if len(known) >= 40 {
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("the zone is known by as many creatures as it may"))
			}
			known = append(known, who)
		case !req.Msg.GetKnown() && has:
			known = slices.DeleteFunc(known, func(id string) bool { return id == who })
		}
		if err := c.q.SetMapZoneKnownBy(ctx, playdb.SetMapZoneKnownByParams{ID: zoneID, KnownBy: nonNil(known)}); err != nil {
			return nil, fmt.Errorf("set who knows the zone: %w", err)
		}
		row.KnownBy = known
		made = zoneStateOf(row)
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		return actionEvent{Round: c.enc.Round, Secret: true, Zone: &zoneNote{ID: zoneID, What: "changed"}}, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "set who knows a zone", err)
	}
	return zoneAnswer(ctx, s, m, res, zoneID, made, func(d *encounterData, z zoneState) {
		s.publishZoneChanged(ctx, m.CampaignID, d, true)
	}, func(z *playv1.MapZone) *playv1.SetMapZoneKnownResponse {
		return &playv1.SetMapZoneKnownResponse{Zone: z}
	})
}

// SetZoneMember implements playv1connect.CombatServiceHandler.
func (s *Service) SetZoneMember(
	ctx context.Context,
	req *connect.Request[playv1.SetZoneMemberRequest],
) (*connect.Response[playv1.SetZoneMemberResponse], error) {
	m, key, encID, zoneID, err := zoneMasterCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId(), req.Msg.GetZoneId())
	if err != nil {
		return nil, err
	}
	who, err := parseCombatID(req.Msg.GetCombatantId(), "combatant")
	if err != nil {
		return nil, err
	}
	var made zoneState
	res, err := s.zoneWrite(ctx, m, key, idem.Hash(req.Msg), eventMapZoneChanged, encID, func(c *combatTx) (any, error) {
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		if !isTheatre(c.enc) {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_THEATRE_ONLY, "who is inside a zone is the master's word only in a combat without a map")
		}
		row, err := c.q.GetMapZone(ctx, playdb.GetMapZoneParams{EncounterID: c.enc.ID, ID: zoneID})
		if err != nil {
			return nil, zoneNotFound()
		}
		z := zoneStateOf(row)
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == who })
		if i < 0 {
			return nil, errCombatantNotFound()
		}
		target := cs[i]
		if req.Msg.GetInside() && !z.affects(target) {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("the zone does not affect that creature"))
		}
		if err := s.mustNotWaitForZone(ctx, c); err != nil {
			return nil, err
		}
		members := slices.Clone(row.Members)
		was := slices.Contains(members, who)
		switch {
		case req.Msg.GetInside() && !was:
			if len(members) >= 40 {
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("the zone has as many creatures inside as it may"))
			}
			members = append(members, who)
		case !req.Msg.GetInside() && was:
			members = slices.DeleteFunc(members, func(id string) bool { return id == who })
		}
		if err := c.q.SetMapZoneMembers(ctx, playdb.SetMapZoneMembersParams{ID: zoneID, Members: nonNil(members)}); err != nil {
			return nil, fmt.Errorf("set who is inside the zone: %w", err)
		}
		row.Members = members
		if err := c.reloadZones(ctx); err != nil {
			return nil, err
		}
		z = zoneStateOf(row)
		made = z
		if req.Msg.GetInside() && !was {
			// The creature enters: the zone's triggers run as they do on a map. The ones marked in the
			// round of the cast were in the area when the spell appeared, and get the cast's trigger.
			kinds := []zone.TriggerKind{zone.OnEnter, zone.OnEnterFirstTimeOnATurn}
			if _, ok := z.trigger(zone.AtCast); ok && row.CastRound == c.enc.Round {
				kinds = []zone.TriggerKind{zone.AtCast} // marked in the round of the cast: it was there when the spell appeared
			}
			for _, k := range kinds {
				if err := s.fireZone(ctx, c, z, target, fireOpts{kind: k}); err != nil {
					return nil, err
				}
			}
		}
		if err := s.syncZoneRules(ctx, c); err != nil {
			return nil, err
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		return actionEvent{Round: c.enc.Round, Secret: true, Zone: &zoneNote{ID: zoneID, What: "changed"}}, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "mark who is inside a zone", err)
	}
	return zoneAnswer(ctx, s, m, res, zoneID, made, func(d *encounterData, z zoneState) {
		s.publishZoneChanged(ctx, m.CampaignID, d, true)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, true)
	}, func(z *playv1.MapZone) *playv1.SetZoneMemberResponse { return &playv1.SetZoneMemberResponse{Zone: z} })
}

// zoneMasterCall parses the ids every call of the master's on a zone carries.
func zoneMasterCall(ctx context.Context, campaignID, rawKey, rawEnc, rawZone string) (m authz.Membership, key, encID, zoneID string, err error) {
	if m, err = authz.RequireCampaignRole(ctx, campaignID, authz.RoleMaster); err != nil {
		return
	}
	if key, err = parseKey(rawKey); err != nil {
		return
	}
	if encID, err = parseCombatID(rawEnc, "encounter"); err != nil {
		return
	}
	zoneID, err = parseCombatID(rawZone, "zone")
	return
}
