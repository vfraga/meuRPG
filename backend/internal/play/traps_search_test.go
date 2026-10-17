package play

import (
	"encoding/json"
	"errors"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Characterization of SearchForTraps and searcherPlace (the search for traps, MR-035): the
// refusals with their codes and messages, who may call, where the search is made from, the
// roll modes and what the history keeps. These tests need the database.

const (
	searchBadSkill   = "skill must be PERCEPTION, INVESTIGATION or OTHER"
	searchBadOther   = "other_skill_key must be a skill of the SRD that is neither Perception nor Investigation"
	searchStrayKey   = "other_skill_key is only for a search with OTHER"
	searchNoCharMsg  = "you have no living character to search with"
	searchNotOnMap   = "your character is not on a map with a grid"
	searchNotNow     = "the combat has not begun: search when it is your turn"
	searchNoNumbers  = "your character has no numbers for this check"
	searchKeyReused  = "idempotency_key was already used for another change"
	searchTwoDiceMsg = "a Perception search in dim light has disadvantage: roll a second die"
)

// trapSearchCall calls SearchForTraps as u with a request the edit fills.
func (r *trapRig) trapSearchCall(t *testing.T, u *user, edit func(*playv1.SearchForTrapsRequest)) (*playv1.SearchForTrapsResponse, error) {
	t.Helper()
	req := &playv1.SearchForTrapsRequest{CampaignId: r.campaignID, IdempotencyKey: newKey()}
	edit(req)
	res, err := u.play.SearchForTraps(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// wantTrapSearchRefusal checks the code and the exact message of a refusal.
func wantTrapSearchRefusal(t *testing.T, what string, err error, code connect.Code, message string) {
	t.Helper()
	cerr, ok := errors.AsType[*connect.Error](err)
	if !ok {
		t.Errorf("%s error = %v, want %v: %q", what, err, code, message)
		return
	}
	if cerr.Code() != code || cerr.Message() != message {
		t.Errorf("%s error = %v: %q, want %v: %q", what, cerr.Code(), cerr.Message(), code, message)
	}
}

// trapSearchEvents are the payloads of the session's trap_searched events, oldest first.
func (r *trapRig) trapSearchEvents(t *testing.T) []actionEvent {
	t.Helper()
	rows, err := r.h.pool.Query(t.Context(), `SELECT payload FROM session_events WHERE kind = 'trap_searched' ORDER BY seq`)
	if err != nil {
		t.Fatalf("read the history: %v", err)
	}
	defer rows.Close()
	var out []actionEvent
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatalf("scan: %v", err)
		}
		var ev actionEvent
		if err := json.Unmarshal(raw, &ev); err != nil {
			t.Fatalf("payload %s: %v", raw, err)
		}
		out = append(out, ev)
	}
	return out
}

// execTrapSearchSQL runs a statement against the rig's database.
func (r *trapRig) execTrapSearchSQL(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := r.h.pool.Exec(t.Context(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

// giveTrapSearchEffect puts an effect of the given modifiers (JSON) on a character, out of a combat.
func (r *trapRig) giveTrapSearchEffect(t *testing.T, characterID, sourceKey, modifiers string) {
	t.Helper()
	r.execTrapSearchSQL(t, `INSERT INTO character_effects (campaign_id, character_id, group_id, source_key, source_kind, duration_kind, modifiers)
		VALUES ($1, $2, gen_random_uuid(), $3, 'spell', 'until_dismissed', $4::jsonb)`, r.campaignID, characterID, sourceKey, modifiers)
}

// trapSearchTyped is the edit of a typed search: the skill and the face of one die.
func trapSearchTyped(skill playv1.TrapSearchSkill, face int32) func(*playv1.SearchForTrapsRequest) {
	return func(q *playv1.SearchForTrapsRequest) {
		q.Skill = skill
		q.Roll = &playv1.SearchForTrapsRequest_D20Face{D20Face: face}
	}
}

// trapSearchFaces is the edit of a search with the faces of two dice.
func trapSearchFaces(skill playv1.TrapSearchSkill, faces ...int32) func(*playv1.SearchForTrapsRequest) {
	return func(q *playv1.SearchForTrapsRequest) {
		q.Skill = skill
		q.D20Faces = faces
	}
}

// trapSearchInApp is the edit of a search rolled in the app.
func trapSearchInApp(skill playv1.TrapSearchSkill) func(*playv1.SearchForTrapsRequest) {
	return func(q *playv1.SearchForTrapsRequest) {
		q.Skill = skill
		q.Roll = &playv1.SearchForTrapsRequest_RollInApp{RollInApp: true}
	}
}

// TestSearchForTrapsRefusesABadRequestWithItsMessage: the order of the checks is the role, the key,
// the skill, its key, the roll, the second die; each refusal is invalid_argument with its message.
func TestSearchForTrapsRefusesABadRequestWithItsMessage(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	r.place(t, r.pens.GetId(), 8, 7)
	typed := func(skill playv1.TrapSearchSkill, face int32, edit func(*playv1.SearchForTrapsRequest)) func(*playv1.SearchForTrapsRequest) {
		return func(q *playv1.SearchForTrapsRequest) {
			trapSearchTyped(skill, face)(q)
			if edit != nil {
				edit(q)
			}
		}
	}
	cases := []struct {
		name string
		edit func(*playv1.SearchForTrapsRequest)
		msg  string
	}{
		{"no key", func(q *playv1.SearchForTrapsRequest) {
			trapSearchTyped(investigation, 10)(q)
			q.IdempotencyKey = "not a uuid"
		}, "idempotency_key must be a UUID"},
		{"a bad key before a bad skill", func(q *playv1.SearchForTrapsRequest) { q.IdempotencyKey = "" }, "idempotency_key must be a UUID"},
		{"no skill", typed(playv1.TrapSearchSkill_TRAP_SEARCH_SKILL_UNSPECIFIED, 10, nil), searchBadSkill},
		{"no skill with a bad roll", typed(playv1.TrapSearchSkill_TRAP_SEARCH_SKILL_UNSPECIFIED, 0, nil), searchBadSkill},
		{"OTHER with no key", typed(other, 10, nil), searchBadOther},
		{"OTHER with Perception's key", typed(other, 10, func(q *playv1.SearchForTrapsRequest) { q.OtherSkillKey = "skill:perception" }), searchBadOther},
		{"OTHER with Investigation's key", typed(other, 10, func(q *playv1.SearchForTrapsRequest) { q.OtherSkillKey = "skill:investigation" }), searchBadOther},
		{"OTHER with an ability", typed(other, 10, func(q *playv1.SearchForTrapsRequest) { q.OtherSkillKey = "ability:int" }), searchBadOther},
		{"OTHER with an unknown skill", typed(other, 10, func(q *playv1.SearchForTrapsRequest) { q.OtherSkillKey = "skill:juggling" }), searchBadOther},
		{"OTHER with a bad key before a bad roll", typed(other, 0, func(q *playv1.SearchForTrapsRequest) { q.OtherSkillKey = "arcana" }), searchBadOther},
		{"a key with PERCEPTION", typed(perception, 10, func(q *playv1.SearchForTrapsRequest) { q.OtherSkillKey = arcanaKey }), searchStrayKey},
		{"a key with INVESTIGATION before a bad roll", typed(investigation, 0, func(q *playv1.SearchForTrapsRequest) { q.OtherSkillKey = arcanaKey }), searchStrayKey},
		{"roll_in_app false", func(q *playv1.SearchForTrapsRequest) {
			q.Skill = investigation
			q.Roll = &playv1.SearchForTrapsRequest_RollInApp{}
		}, "roll_in_app must be true"},
		{"d20_face 0", typed(investigation, 0, nil), "d20_face must be 1 to 20"},
		{"d20_face 21", typed(investigation, 21, nil), "d20_face must be 1 to 20"},
		{"no roll", func(q *playv1.SearchForTrapsRequest) { q.Skill = investigation }, "set roll_in_app, d20_face or d20_faces"},
		{"three faces", trapSearchFaces(investigation, 4, 5, 6), "set roll_in_app, d20_face or d20_faces"},
		{"no faces", trapSearchFaces(investigation), "set roll_in_app, d20_face or d20_faces"},
		{"a face of 0 in the faces", trapSearchFaces(investigation, 0, 5), "d20_faces must be 1 to 20 each"},
		{"a face of 21 in the faces", trapSearchFaces(perception, 5, 21), "d20_faces must be 1 to 20 each"},
		{"d20_face_2 of 0 on a typed Perception", typed(perception, 10, func(q *playv1.SearchForTrapsRequest) { q.D20Face_2 = proto.Int32(0) }), "d20_face_2 must be 1 to 20"},
		{"d20_face_2 of 21 on a typed Perception", typed(perception, 10, func(q *playv1.SearchForTrapsRequest) { q.D20Face_2 = proto.Int32(21) }), "d20_face_2 must be 1 to 20"},
	}
	for _, c := range cases {
		_, err := r.trapSearchCall(t, r.ana, c.edit)
		wantTrapSearchRefusal(t, c.name, err, connect.CodeInvalidArgument, c.msg)
	}
	if n := r.eventCount(t, "trap_searched"); n != 0 {
		t.Errorf("trap_searched events = %d, want the refusals to leave none", n)
	}

	// The second die is read only where it counts: a Perception search with a typed die. Anywhere else
	// an out-of-range value is not looked at.
	r.h.roller.queue(10, 11)
	if res, err := r.trapSearchCall(t, r.ana, func(q *playv1.SearchForTrapsRequest) {
		trapSearchInApp(perception)(q)
		q.D20Face_2 = proto.Int32(99)
	}); err != nil || res.GetRoll().GetTotal() != 10 || res.GetSecondRoll().GetTotal() != 11 {
		t.Errorf("a d20_face_2 of 99 in the app = %v, %v; want it ignored: the app's two rolls 10 and 11", res, err)
	}
	for name, edit := range map[string]func(*playv1.SearchForTrapsRequest){
		"Investigation": typed(investigation, 10, func(q *playv1.SearchForTrapsRequest) { q.D20Face_2 = proto.Int32(99) }),
		"another skill": typed(other, 10, func(q *playv1.SearchForTrapsRequest) { q.OtherSkillKey = arcanaKey; q.D20Face_2 = proto.Int32(99) }),
	} {
		r.h.roller.queue(10, 10)
		if res, err := r.trapSearchCall(t, r.ana, edit); err != nil || res.GetSecondRoll() != nil {
			t.Errorf("a d20_face_2 of 99 with %s = %v, %v; want it ignored: one roll", name, res, err)
		}
	}
}

// TestSearchForTrapsWhoMayCall: a member of the campaign who is a player; the master has no
// character to search with, and the others never reach the search.
func TestSearchForTrapsWhoMayCall(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	r.place(t, r.pens.GetId(), 8, 7)
	call := func(u *user) error {
		_, err := r.trapSearchCall(t, u, trapSearchTyped(investigation, 10))
		return err
	}
	wantCode(t, "SearchForTraps(anonymous)", call(r.h.anonymous()), connect.CodeUnauthenticated)
	wantCode(t, "SearchForTraps(outsider)", call(r.h.newUser("Fora")), connect.CodeNotFound)
	wantTrapSearchRefusal(t, "SearchForTraps(master)", call(r.master), connect.CodePermissionDenied, "only the campaign's player can do this")
	var err error
	// A player with no character reads the same refusal as the master.
	late := r.h.newUser("Lia")
	r.h.join(r.master, r.campaignID, late)
	err = call(late)
	wantSceneBlocked(t, err, playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_NO_CHARACTER)
	wantTrapSearchRefusal(t, "SearchForTraps(a player with no character)", err, connect.CodeFailedPrecondition, searchNoCharMsg)
	if n := r.eventCount(t, "trap_searched"); n != 0 {
		t.Errorf("trap_searched events = %d, want none", n)
	}
}

// TestSearchForTrapsNeedsTrapsASessionAndAPlace: the server with the traps off, the campaign with
// no open session, the character that is not on the map and the sheet with no numbers; and the
// order of those checks (the request first, then the traps, the character, the session).
func TestSearchForTrapsNeedsTrapsASessionAndAPlace(t *testing.T) {
	t.Parallel()
	t.Run("a server with the traps off", func(t *testing.T) {
		t.Parallel()
		c := newCave(t) // no TrapBook wired
		search := func(edit func(*playv1.SearchForTrapsRequest)) error {
			req := &playv1.SearchForTrapsRequest{CampaignId: c.campaignID, IdempotencyKey: newKey()}
			edit(req)
			_, err := c.ana.play.SearchForTraps(t.Context(), connect.NewRequest(req))
			return err
		}
		wantTrapSearchRefusal(t, "a valid search", search(trapSearchTyped(investigation, 10)), connect.CodeUnavailable, "traps are not available on this server")
		wantTrapSearchRefusal(t, "a bad request", search(trapSearchTyped(investigation, 0)), connect.CodeInvalidArgument, "d20_face must be 1 to 20")
	})
	t.Run("no open session", func(t *testing.T) {
		t.Parallel()
		r := newTrapRig(t)
		r.place(t, r.pens.GetId(), 8, 7)
		r.master.end(t, r.master.liveSession(t, r.campaignID).GetGameSession())
		_, err := r.trapSearchCall(t, r.ana, trapSearchTyped(investigation, 10))
		wantTrapSearchRefusal(t, "a search with no session", err, connect.CodeFailedPrecondition, "the campaign has no open game session")
	})
	t.Run("the character is not on the map", func(t *testing.T) {
		t.Parallel()
		r := newTrapRig(t)
		_, err := r.trapSearchCall(t, r.ana, trapSearchTyped(investigation, 10))
		wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TRAP_NOT_ON_MAP)
		wantTrapSearchRefusal(t, "a search off the map", err, connect.CodeFailedPrecondition, searchNotOnMap)
		if n := r.eventCount(t, "trap_searched"); n != 0 {
			t.Errorf("trap_searched events = %d, want none", n)
		}
	})
	t.Run("the character has no numbers", func(t *testing.T) {
		t.Parallel()
		r := newTrapRig(t)
		// A player's character always has a full sheet: Pensantus gets the sheet of a creature's basic one.
		minion := r.master.createCharacter(t, r.campaignID, charactersv1.CharacterKind_CHARACTER_KIND_MINION, "Lacaio")
		r.execTrapSearchSQL(t, `UPDATE characters SET sheet = (SELECT sheet FROM characters WHERE id = $1) WHERE id = $2`, minion.GetId(), r.pens.GetId())
		r.place(t, r.pens.GetId(), 8, 7)
		_, err := r.trapSearchCall(t, r.ana, trapSearchTyped(investigation, 10))
		wantSceneBlocked(t, err, playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_NO_CHARACTER)
		wantTrapSearchRefusal(t, "a search of a basic sheet", err, connect.CodeFailedPrecondition, searchNoNumbers)
	})
}

// TestSearchForTrapsInACombat: a combat that has not begun refuses the search, a character the
// combat does not hold searches from the map and pays nothing, and the search of a combatant spends
// the action, touches the combat and is written with the round and the combatant.
func TestSearchForTrapsInACombat(t *testing.T) {
	t.Parallel()
	t.Run("a combat that has not begun", func(t *testing.T) {
		t.Parallel()
		r := newTrapRig(t)
		r.setupUnplaced(t)
		_, err := r.trapSearchCall(t, r.caio, trapSearchTyped(investigation, 10))
		wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TRAP_SEARCH_NOT_NOW)
		wantTrapSearchRefusal(t, "a search before the combat begins", err, connect.CodeFailedPrecondition, searchNotNow)
		if n := r.eventCount(t, "trap_searched"); n != 0 {
			t.Errorf("trap_searched events = %d, want none", n)
		}
	})
	t.Run("not the turn and the action used", func(t *testing.T) {
		t.Parallel()
		r := newTrapRig(t)
		r.fight(t)
		_, err := r.trapSearchCall(t, r.ana, trapSearchTyped(investigation, 10))
		wantEncounterBlocked(t, err, reasonNotTurn)
		if _, err := r.trapSearchCall(t, r.caio, trapSearchTyped(investigation, 10)); err != nil {
			t.Fatalf("Toren's search error = %v", err)
		}
		_, err = r.trapSearchCall(t, r.caio, trapSearchTyped(investigation, 10))
		wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED)
		wantTrapSearchRefusal(t, "a second search", err, connect.CodeFailedPrecondition, "the action of this turn is used")
	})
	t.Run("the search of a combatant", func(t *testing.T) {
		t.Parallel()
		r := newTrapRig(t)
		pit := r.trap(t, "Fosso Dourado", 7, 7, func(s *mapsv1.TrapSpec) { s.NoticeDc = 20 })
		e := r.fight(t)
		before := r.get(t, r.master).GetRevision()
		res, err := r.trapSearchCall(t, r.caio, trapSearchTyped(perception, 20))
		if err != nil || !res.GetSpentAction() || len(res.GetFoundPointIds()) != 1 || res.GetFoundPointIds()[0] != pit.GetId() {
			t.Fatalf("Toren's search = %v, %v; want the pit found and the action spent", res, err)
		}
		if after := r.get(t, r.master).GetRevision(); after <= before {
			t.Errorf("the combat's revision after the search = %d, was %d; want it touched", after, before)
		}
		events := r.trapSearchEvents(t)
		if len(events) != 1 {
			t.Fatalf("trap_searched events = %d, want 1", len(events))
		}
		ev := events[0]
		if ev.Actor != r.id(t, "Toren") || ev.Round != e.GetRound() || ev.Key != searchPerception || !ev.OnTurn || len(ev.Found) != 1 || ev.D20 != 20 || ev.Total != 20 {
			t.Errorf("the event = %s, want Toren's, in round %d, on turn, perception, a 20", eventSummary(ev), e.GetRound())
		}
	})
	t.Run("a character the combat does not hold", func(t *testing.T) {
		t.Parallel()
		r := newTrapRig(t)
		needle := r.trap(t, "Agulha Rubra", 8, 8, func(s *mapsv1.TrapSpec) { s.NoticeDc, s.FindDc = 0, 13 })
		r.fight(t)
		late := r.h.newUser("Lia") // joins after the combat began: no combatant
		r.h.join(r.master, r.campaignID, late)
		rogue := late.rogue(t, r.campaignID, "Lia", 3)
		r.place(t, rogue.GetId(), 8, 7)
		res, err := r.trapSearchCall(t, late, trapSearchTyped(investigation, 20))
		if err != nil || res.GetSpentAction() || len(res.GetFoundPointIds()) != 1 || res.GetFoundPointIds()[0] != needle.GetId() {
			t.Fatalf("Lia's search = %v, %v; want the needle found from the map and no action spent", res, err)
		}
		if ev := r.trapSearchEvents(t)[0]; ev.Actor != "" || ev.OnTurn || ev.Round != 0 {
			t.Errorf("the event = %s, want no combatant, no round and not on turn", eventSummary(ev))
		}
	})
	t.Run("a combatant with no square", func(t *testing.T) {
		t.Parallel()
		r := newTrapRig(t)
		r.fight(t)
		r.execTrapSearchSQL(t, `UPDATE combatants SET grid_col = NULL, grid_row = NULL WHERE id = $1`, r.id(t, "Toren"))
		_, err := r.trapSearchCall(t, r.caio, trapSearchTyped(investigation, 10))
		wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TRAP_NOT_ON_MAP)
		wantTrapSearchRefusal(t, "a search with no square", err, connect.CodeFailedPrecondition, searchNotOnMap)
	})
	t.Run("an ended combat", func(t *testing.T) {
		t.Parallel()
		r := newTrapRig(t)
		needle := r.trap(t, "Agulha Rubra", 8, 8, func(s *mapsv1.TrapSpec) { s.NoticeDc, s.FindDc = 0, 13 })
		e := r.fight(t)
		if _, err := r.master.combat.EndEncounter(t.Context(), connect.NewRequest(&playv1.EndEncounterRequest{CampaignId: r.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey()})); err != nil {
			t.Fatalf("EndEncounter() error = %v", err)
		}
		r.place(t, r.pens.GetId(), 8, 7)
		res, err := r.trapSearchCall(t, r.ana, trapSearchTyped(investigation, 20))
		if err != nil || res.GetSpentAction() || len(res.GetFoundPointIds()) != 1 || res.GetFoundPointIds()[0] != needle.GetId() {
			t.Fatalf("Pensantus's search after the combat = %v, %v; want the needle found, no action spent", res, err)
		}
	})
}

// TestSearchForTrapsRollModes: the second die of a Perception search, typed or rolled, and what
// each mode counts and writes: exhaustion is a disadvantage on the check, an effect gives
// advantage, and the answer says which die counted.
func TestSearchForTrapsRollModes(t *testing.T) {
	t.Parallel()
	const hardDC = 16
	setup := func(t *testing.T) (*trapRig, *mapsv1.MapPoint) {
		t.Helper()
		r := newTrapRig(t)
		hole := r.trap(t, "Fosso Dourado", 9, 7, func(s *mapsv1.TrapSpec) { s.NoticeDc, s.FindDc = hardDC, hardDC })
		r.place(t, r.pens.GetId(), 8, 7) // Pensantus: Perception +0, Investigation +4
		return r, hole
	}
	t.Run("two typed dice of a Perception search", func(t *testing.T) {
		t.Parallel()
		r, _ := setup(t)
		res, err := r.trapSearchCall(t, r.ana, trapSearchFaces(perception, 15, 8))
		if err != nil || res.GetRoll().GetFaces()[0] != 15 || res.GetRoll().GetTotal() != 15 || res.GetSecondRoll().GetFaces()[0] != 8 || res.GetSecondRoll().GetTotal() != 8 || res.GetMode() != playv1.RollMode_ROLL_MODE_NORMAL {
			t.Fatalf("15 and 8 = %v, %v; want the rolls 15 and 8, normal mode", res, err)
		}
		ev := r.trapSearchEvents(t)[0]
		if ev.D20 != 15 || ev.D20B != 8 || ev.Total != 15 || ev.RollMode != "" || ev.Counted != 0 {
			t.Errorf("the event = %s, want d20 15, second d20 8, no mode", eventSummary(ev))
		}
	})
	t.Run("the second die is ignored by Investigation", func(t *testing.T) {
		t.Parallel()
		r, _ := setup(t)
		res, err := r.trapSearchCall(t, r.ana, trapSearchFaces(investigation, 15, 8))
		if err != nil || res.GetSecondRoll() != nil || res.GetRoll().GetFaces()[0] != 15 || res.GetRoll().GetTotal() != 19 {
			t.Fatalf("15 and 8 on Investigation = %v, %v; want the first die alone, 15 + 4", res, err)
		}
	})
	t.Run("the second die rolled in the app", func(t *testing.T) {
		t.Parallel()
		r, _ := setup(t)
		r.h.roller.queue(14, 6)
		res, err := r.trapSearchCall(t, r.ana, trapSearchInApp(perception))
		if err != nil || res.GetRoll().GetTotal() != 14 || res.GetSecondRoll().GetTotal() != 6 {
			t.Fatalf("the app's two rolls = %v, %v; want 14 and 6", res, err)
		}
	})
	t.Run("a typed die with the second one in its own field", func(t *testing.T) {
		t.Parallel()
		r, _ := setup(t)
		res, err := r.trapSearchCall(t, r.ana, func(q *playv1.SearchForTrapsRequest) {
			trapSearchTyped(perception, 12)(q)
			q.D20Face_2 = proto.Int32(5)
		})
		if err != nil || res.GetRoll().GetTotal() != 12 || res.GetSecondRoll().GetTotal() != 5 {
			t.Fatalf("12 and 5 = %v, %v; want the rolls 12 and 5", res, err)
		}
	})
	t.Run("exhaustion is a disadvantage on the check", func(t *testing.T) {
		t.Parallel()
		r, hole := setup(t)
		r.master.liveSession(t, r.campaignID)
		r.execTrapSearchSQL(t, `INSERT INTO character_vitals (character_id, hit_points_current, exhaustion_level, revision, updated_at)
			VALUES ($1, NULL, 1, 1, now()) ON CONFLICT (character_id) DO UPDATE SET exhaustion_level = 1`, r.pens.GetId())
		// Investigation: the lower of two dice counts (SRD 5.1), 4 + 4.
		r.h.roller.queue(15, 3)
		res, err := r.trapSearchCall(t, r.ana, trapSearchInApp(investigation))
		if err != nil || res.GetMode() != playv1.RollMode_ROLL_MODE_DISADVANTAGE || res.GetSecondRoll() != nil || len(res.GetSources()) == 0 ||
			len(res.GetRoll().GetFaces()) != 2 || res.GetRoll().GetFaces()[0] != 15 || res.GetRoll().GetFaces()[1] != 3 || res.GetRoll().GetTotal() != 7 || res.GetRoll().GetCountedIndex() != 1 {
			t.Fatalf("Investigation with exhaustion = %v, %v; want faces 15 and 3, the second counted, total 7, sources", res, err)
		}
		ev := r.trapSearchEvents(t)[0]
		if ev.RollMode != "disadvantage" || ev.Counted != 1 || ev.D20 != 15 || ev.D20B != 3 || ev.Total != 7 {
			t.Errorf("the event = %s, want disadvantage, the second die counted", eventSummary(ev))
		}
		// Perception: the same total counts for the lit squares and the dim ones.
		got, err := r.trapSearchCall(t, r.ana, trapSearchFaces(perception, 17, 18))
		if err != nil || got.GetMode() != playv1.RollMode_ROLL_MODE_DISADVANTAGE || got.GetRoll().GetTotal() != 17 || len(got.GetFoundPointIds()) != 1 || got.GetFoundPointIds()[0] != hole.GetId() {
			t.Fatalf("Perception 17 and 18 with exhaustion = %v, %v; want the lower (17) to count and find the pit", got, err)
		}
		// Typed with a single die the roll is refused: the mode needs two.
		_, err = r.trapSearchCall(t, r.ana, trapSearchTyped(investigation, 10))
		wantCode(t, "one typed die under disadvantage", err, connect.CodeInvalidArgument)
	})
	t.Run("an effect that gives advantage", func(t *testing.T) {
		t.Parallel()
		r, _ := setup(t)
		r.master.liveSession(t, r.campaignID)
		r.giveTrapSearchEffect(t, r.pens.GetId(), "spell:enhance-ability", `[{"kind":"check_advantage","abilities":["wis","int"]}]`)
		res, err := r.trapSearchCall(t, r.ana, trapSearchFaces(investigation, 5, 17))
		if err != nil || res.GetMode() != playv1.RollMode_ROLL_MODE_ADVANTAGE || res.GetRoll().GetTotal() != 21 || res.GetRoll().GetCountedIndex() != 1 || res.GetSecondRoll() != nil {
			t.Fatalf("Investigation 5 and 17 with advantage = %v, %v; want the higher (17 + 4) to count", res, err)
		}
		// A Perception search under advantage answers with the higher roll, and the second total, for the
		// dim squares, is the first die with the bonus (the light's disadvantage cancels the advantage).
		got, err := r.trapSearchCall(t, r.ana, trapSearchFaces(perception, 5, 17))
		if err != nil || got.GetMode() != playv1.RollMode_ROLL_MODE_ADVANTAGE || got.GetRoll().GetTotal() != 17 || got.GetSecondRoll() != nil {
			t.Fatalf("Perception 5 and 17 with advantage = %v, %v; want the higher (17) to count", got, err)
		}
		ev := r.trapSearchEvents(t)[1]
		if ev.RollMode != "advantage" || ev.Counted != 1 || ev.D20 != 5 || ev.D20B != 17 {
			t.Errorf("the event = %s, want advantage, the second die counted", eventSummary(ev))
		}
	})
}

// TestSearchForTrapsEffectDiceAndSources: a die an effect adds to the check is rolled with the
// search, added to the bonus and spent; the answer shows where it came from.
func TestSearchForTrapsEffectDiceAndSources(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	r.trap(t, "Fosso Dourado", 9, 7, func(s *mapsv1.TrapSpec) { s.NoticeDc, s.FindDc = 0, 14 })
	r.place(t, r.pens.GetId(), 8, 7)
	r.master.liveSession(t, r.campaignID)
	r.giveTrapSearchEffect(t, r.pens.GetId(), "spell:guidance", `[{"kind":"roll_die","die":4,"sign":1,"applies_to":["check"],"once":true}]`)
	r.h.roller.queue(3, 9) // the d4 is rolled first, then the d20
	res, err := r.trapSearchCall(t, r.ana, trapSearchInApp(investigation))
	if err != nil {
		t.Fatalf("SearchForTraps() error = %v", err)
	}
	// 9 + 4 (Intelligence) + 3 (the d4) = 16 clears DC 14; the modifier of the answer holds the die.
	if res.GetRoll().GetTotal() != 16 || res.GetRoll().GetModifier() != 7 || len(res.GetFoundPointIds()) != 1 || len(res.GetBonusDice()) != 0 ||
		len(res.GetSources()) != 1 || res.GetSources()[0].GetKind() != playv1.AdvantageSourceKind_ADVANTAGE_SOURCE_KIND_EFFECT_DIE {
		t.Errorf("the search = %v, want the roll 9 + 7 = 16, the pit found and the die in the sources", res)
	}
	var left int
	if err := r.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM character_effects WHERE character_id = $1`, r.pens.GetId()).Scan(&left); err != nil || left != 0 {
		t.Errorf("effects left on Pensantus = %d (%v), want the once-die spent", left, err)
	}
	ev := r.trapSearchEvents(t)[0]
	if ev.Modifier != 7 || ev.Total != 16 || ev.D20 != 9 {
		t.Errorf("the event = %s, want d20 9, modifier 7, total 16", eventSummary(ev))
	}
}

// TestSearchForTrapsRetries: the retry of a search returns the first answer and changes nothing; a key
// of another change or another request is refused.
func TestSearchForTrapsRetries(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	needle := r.trap(t, "Agulha Rubra", 8, 8, func(s *mapsv1.TrapSpec) { s.NoticeDc, s.FindDc = 0, 13 })
	r.place(t, r.pens.GetId(), 8, 7)
	r.master.liveSession(t, r.campaignID)
	r.execTrapSearchSQL(t, `INSERT INTO character_vitals (character_id, hit_points_current, exhaustion_level, revision, updated_at)
		VALUES ($1, NULL, 1, 1, now()) ON CONFLICT (character_id) DO UPDATE SET exhaustion_level = 1`, r.pens.GetId())
	key := newKey()
	call := func(face1, face2 int32, skill playv1.TrapSearchSkill) (*playv1.SearchForTrapsResponse, error) {
		return r.trapSearchCall(t, r.ana, func(q *playv1.SearchForTrapsRequest) {
			trapSearchFaces(skill, face1, face2)(q)
			q.IdempotencyKey = key
		})
	}
	first, err := call(15, 18, investigation)
	if err != nil || len(first.GetFoundPointIds()) != 1 || first.GetFoundPointIds()[0] != needle.GetId() || len(first.GetSources()) == 0 {
		t.Fatalf("the first search = %v, %v; want the needle found and the sources of the roll", first, err)
	}
	again, err := call(15, 18, investigation)
	if err != nil {
		t.Fatalf("the retry error = %v", err)
	}
	// Finding: the retry does not carry the Sources of the roll (traps.go: shown is only worked out
	// by the first call), so it answers with fewer fields than the first.
	if len(again.GetSources()) != 0 {
		t.Errorf("the retry has %d sources, frozen as none", len(again.GetSources()))
	}
	again.Sources, first.Sources = nil, nil
	if !proto.Equal(first, again) {
		t.Errorf("the retry = %v, want the first answer %v", again, first)
	}
	if n := r.eventCount(t, "trap_searched"); n != 1 {
		t.Errorf("trap_searched events = %d, want 1", n)
	}
	_, err = call(15, 18, perception)
	wantTrapSearchRefusal(t, "the key with another skill", err, connect.CodeInvalidArgument, searchKeyReused)
	_, err = call(14, 18, investigation)
	wantTrapSearchRefusal(t, "the key with another roll", err, connect.CodeInvalidArgument, searchKeyReused)
	if n := r.eventCount(t, "trap_searched"); n != 1 {
		t.Errorf("trap_searched events = %d, want the refusals to add none", n)
	}
}

// TestSearchForTrapsKeyOfAnotherChange: a key an event of another kind already holds is refused.
func TestSearchForTrapsKeyOfAnotherChange(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	trap := r.trap(t, "Mola", 12, 7, pit("1d6"), func(s *mapsv1.TrapSpec) { s.Trigger = rulesv1.TrapTrigger_TRAP_TRIGGER_MANUAL })
	r.place(t, r.pens.GetId(), 8, 7)
	r.place(t, r.toren.GetId(), 12, 7)
	key := newKey()
	if _, err := r.fireWithKey(t, key, trap); err != nil {
		t.Fatalf("FireTrap() error = %v", err)
	}
	_, err := r.trapSearchCall(t, r.ana, func(q *playv1.SearchForTrapsRequest) {
		trapSearchTyped(investigation, 10)(q)
		q.IdempotencyKey = key
	})
	wantTrapSearchRefusal(t, "a search with the key of a firing", err, connect.CodeInvalidArgument, searchKeyReused)
}

// TestSearchForTrapsAnswerShapes: what each viewer reads of a search, and what the history keeps (ids and
// numbers, never a DC); the master reads the search in the activity, the other players do not.
func TestSearchForTrapsAnswerShapes(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	pit := r.trap(t, "Fosso Dourado", 9, 7, func(s *mapsv1.TrapSpec) { s.NoticeDc, s.FindDc = 12, 12 })
	r.place(t, r.pens.GetId(), 8, 7)
	r.place(t, r.toren.GetId(), 3, 3)
	res, err := r.trapSearchCall(t, r.ana, trapSearchTyped(investigation, 11))
	if err != nil || len(res.GetFoundPointIds()) != 1 || res.GetFoundPointIds()[0] != pit.GetId() || res.GetRoll().GetModifier() != 4 || res.GetRoll().GetTotal() != 15 || res.GetSpentAction() {
		t.Fatalf("the search = %v, %v; want the pit found, 11 + 4", res, err)
	}
	ev := r.trapSearchEvents(t)[0]
	if ev.Key != searchInvestigation || ev.D20 != 11 || ev.Modifier != 4 || ev.Total != 15 || ev.Physical != true || len(ev.Found) != 1 || ev.Found[0] != pit.GetId() || ev.Actor != "" {
		t.Errorf("the event = %s, want a typed (physical) investigation, 11 + 4, the pit", eventSummary(ev))
	}
	var searches int
	for _, act := range r.activity(t, r.master) {
		if act.GetSearch() != nil {
			searches++
			if got := act.GetSearch().GetFoundPointIds(); len(got) != 1 || got[0] != pit.GetId() {
				t.Errorf("the master's activity of the search = %v, want the pit", act.GetSearch())
			}
		}
	}
	if searches != 1 {
		t.Errorf("the master reads %d searches in the activity, want 1", searches)
	}
	if rev := r.revealedTo(t, pit.GetId()); len(rev) != 1 {
		t.Errorf("revealed to %v, want only the searcher", rev)
	}
}

// eventSummary is the fields of a search's event that a failure message needs.
func eventSummary(ev actionEvent) string {
	b, _ := json.Marshal(ev)
	return string(b)
}
