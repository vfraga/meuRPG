package leaktest

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The hint number (#735): game_sessions.hint_seq, which every write of a session adds 1 to, hidden or not, and which a stream
// carries on Ready and on every heartbeat (WatchGameSessionResponse.session_seq). The app reads the session again when a
// heartbeat is ahead of what it applied. The rows below prove the number is a counter and nothing else:
//
//   - every field of the messages that carry it, or that answer from it, has a leak row (hintFieldRows);
//   - only a member who may watch the stream is told a number (TestTheHintNumberIsToldOnlyToWhoMayWatch);
//   - a write made only to what is hidden moves the number and moves nothing else a player reads, and the move does not
//     depend on the content (TestAHiddenWriteMovesThePlayersNumberAndNothingElse, three worlds that differ in that write);
//   - ListCombatLog with since_seq answers a player as if the write they cannot see had not been made
//     (TestAPlayersSinceSeqAnswerIgnoresWhatTheyCannotSee).
//
// What the number depends on, exactly: it moves once for each write that had an effect, and not for one that changed nothing (a
// paint of what was painted, a trap already disarmed, a clue the player had), whatever the write held. A token a player's
// character drops on a map is one write whatever the map and the token (TestALandingMovesAThirdPlayersNumberOnceWhateverTheMapAndTheToken):
// what it sees from where it stands is part of that write, and moves the number no more whether or not it found a trap
// (TestADropThatNoticesATrapMovesAThirdPlayersNumberLikeOneThatDoesNot), so a third player's number says nothing of a find, of the map
// or of a hidden token. The notice after a combat move, in package play, is a write of its own that always touches, found or not.
//
// The counter-per-transaction rule (one bump whatever the transaction told) is TestTxBumpsOncePerTransactionWhateverItTold
// and TestTxBumpsOnceWhateverTheSizeOfWhatItTold, in package play.

// hintFieldRows says, for every field of the messages that carry the hint number or the combat log's own counters, who may
// read it and what the leak test searches there. TestEveryHintNumberFieldHasALeakRow fails for a field that is not here, so a
// field added to these messages is a decision made here, with its row.
var hintFieldRows = map[protoreflect.FullName]string{
	"meurpg.play.v1.WatchGameSessionResponse.session_seq":                   "a counter: moves on every write the session made, hidden or not; carries no content, never derived from it (TestAHiddenWriteMovesThePlayersNumberAndNothingElse); only a member who may watch the stream reads it (TestTheHintNumberIsToldOnlyToWhoMayWatch)",
	"meurpg.play.v1.WatchGameSessionResponse.Ready.game_session":            "the session being watched: the session's own fields, searched by the stream check of the matrix (checkStream)",
	"meurpg.play.v1.WatchGameSessionResponse.Ready.session_seq":             "same counter as WatchGameSessionResponse.session_seq, as the stream opened",
	"meurpg.play.v1.WatchGameSessionResponse.Heartbeat.session_seq":         "same counter, read from the database at the beat; the heartbeat carries nothing else (TestAHiddenWriteMovesThePlayersNumberAndNothingElse)",
	"meurpg.play.v1.WatchGameSessionResponse.CombatLogChanged.encounter_id": "the combat's id, which every member reads; no canary",
	"meurpg.play.v1.WatchGameSessionResponse.CombatLogChanged.full":         "a flag, set on the master's copy alone: a player's hint is the same whatever was undone (TestCombatLogChangedHintOfAPlayerNeverCarriesFull, package play)",
	"meurpg.play.v1.ListCombatLogRequest.campaign_id":                       "an input",
	"meurpg.play.v1.ListCombatLogRequest.encounter_id":                      "an input",
	"meurpg.play.v1.ListCombatLogRequest.since_seq":                         "an input: the caller's own number, and the answer to it is the answer the caller would get from the whole log (TestAPlayersSinceSeqAnswerIgnoresWhatTheyCannotSee)",
	"meurpg.play.v1.ListCombatLogResponse.rounds":                           "the lines the caller may read: the canaries of the matrix's ListCombatLog row",
	"meurpg.play.v1.ListCombatLogResponse.undoable_event_id":                "the master's alone (the matrix's master-only rule)",
	"meurpg.play.v1.ListCombatLogResponse.last_seq":                         "a counter of the events the caller may read; a write the caller cannot see leaves it where it was (TestAPlayersSinceSeqAnswerIgnoresWhatTheyCannotSee)",
	"meurpg.play.v1.ListCombatLogResponse.full":                             "a flag; a player's is set only for what the player may see (TestAPlayersSinceSeqAnswerIgnoresWhatTheyCannotSee)",
	"meurpg.play.v1.CombatLogEntry.seq":                                     "the entry's own event number: it follows the entry's visibility, an entry the caller may not read is not in the answer at all",
}

// hintMessages are the messages whose every field hintFieldRows lists; hintFields lists single fields of larger messages.
func hintMessages() []protoreflect.MessageDescriptor {
	watch := (&playv1.WatchGameSessionResponse{}).ProtoReflect().Descriptor()
	return []protoreflect.MessageDescriptor{
		watch.Messages().ByName("Ready"), watch.Messages().ByName("Heartbeat"), watch.Messages().ByName("CombatLogChanged"),
		(&playv1.ListCombatLogRequest{}).ProtoReflect().Descriptor(), (&playv1.ListCombatLogResponse{}).ProtoReflect().Descriptor(),
	}
}

func hintFields() []protoreflect.FieldDescriptor {
	return []protoreflect.FieldDescriptor{
		(&playv1.WatchGameSessionResponse{}).ProtoReflect().Descriptor().Fields().ByName("session_seq"),
		(&playv1.CombatLogEntry{}).ProtoReflect().Descriptor().Fields().ByName("seq"),
	}
}

// TestEveryHintNumberFieldHasALeakRow needs no database.
func TestEveryHintNumberFieldHasALeakRow(t *testing.T) {
	seen := map[protoreflect.FullName]bool{}
	check := func(f protoreflect.FieldDescriptor) {
		seen[f.FullName()] = true
		if strings.TrimSpace(hintFieldRows[f.FullName()]) == "" {
			t.Errorf("%s has no leak row: say who may read it and what the leak test searches there (hintFieldRows, seq_test.go)", f.FullName())
		}
	}
	for _, m := range hintMessages() {
		for i := range m.Fields().Len() {
			check(m.Fields().Get(i))
		}
	}
	for _, f := range hintFields() {
		check(f)
	}
	for name := range hintFieldRows {
		if !seen[name] {
			t.Errorf("hintFieldRows names %s, which is not a field of these messages any more", name)
		}
	}
}

// ===== What a stream tells, and to whom.

// number is the hint number of the first event of the stream: what Ready says.
func (sw *streamWatcher) number() int64 {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.events[0].GetReady().GetSessionSeq()
}

// heartbeats lists the numbers of the heartbeats the stream received after its first n events.
func (sw *streamWatcher) heartbeats(n int) []*playv1.WatchGameSessionResponse {
	var out []*playv1.WatchGameSessionResponse
	for _, ev := range sw.since(n) {
		if ev.GetHeartbeat() != nil {
			out = append(out, ev)
		}
	}
	return out
}

// beatAfter waits for the heartbeat of the stream that comes after its first n events and satisfies ok, and returns its number.
// The heartbeat reads the number from the database each time it beats, so the wait ends when a beat reads what the writes
// left. The failure after 20 seconds is the test's ceiling, not a delay.
func (sw *streamWatcher) beatAfter(t *testing.T, n int, ok func(int64) bool) int64 {
	t.Helper()
	deadline := time.After(20 * time.Second)
	for {
		for _, hb := range sw.heartbeats(n) {
			if ok(hb.GetHeartbeat().GetSessionSeq()) {
				return hb.GetHeartbeat().GetSessionSeq()
			}
		}
		select {
		case <-sw.more:
		case <-deadline:
			t.Fatalf("%s's stream never beat with a number that fits (heartbeats: %v)", sw.p.name, sw.heartbeats(n))
		}
	}
}

// ===== The number is told only to who may watch.

// TestTheHintNumberIsToldOnlyToWhoMayWatch: a stranger, a member of another campaign, a pending member and someone with no
// session get the refusal the stream gives, the same for all of them, and never a number; ListCombatLog refuses them with
// no number either. Control: the master and the players read their number, and a member of another campaign reads the
// number of his own campaign's stream, not ours.
func TestTheHintNumberIsToldOnlyToWhoMayWatch(t *testing.T) {
	w := newWorld(t)
	ctx := t.Context()
	other := w.newPerson("Otto")
	otherCampaign := must(other.campaigns.CreateCampaign(ctx, rq(&campaignsv1.CreateCampaignRequest{Name: "Outra", XpMode: campaignsv1.XpMode_XP_MODE_MILESTONES}))).GetCampaign().GetId()
	outsiders := []*person{w.stranger, other, w.pending, w.anonymous()}

	// The stream: a refusal on the first Receive, and no message before it.
	var codes []connect.Code
	for _, p := range outsiders {
		anonymous := p.id == ""
		stream, err := p.play.WatchGameSession(ctx, connect.NewRequest(&playv1.WatchGameSessionRequest{CampaignId: w.campaign}))
		if err == nil {
			if stream.Receive() {
				t.Errorf("%s opened the stream and got %v", p.name, stream.Msg())
			}
			err = stream.Err()
			_ = stream.Close()
		}
		if err == nil {
			t.Errorf("%s opened the stream of a campaign that is not theirs", p.name)
			continue
		}
		if anonymous { // no session at all: the sign-in's own refusal
			if connect.CodeOf(err) != connect.CodeUnauthenticated {
				t.Errorf("someone with no session got %v, want the sign-in's refusal", err)
			}
		} else {
			codes = append(codes, connect.CodeOf(err))
		}
		if strings.Contains(strings.ToLower(err.Error()), "seq") {
			t.Errorf("the refusal of %s names a number: %v", p.name, err)
		}
	}
	for _, c := range codes[1:] {
		if c != codes[0] {
			t.Errorf("the refusals of the stream have codes %v, want one code for every outsider who has a session", codes)
			break
		}
	}

	// The combat log: refused with no number in the answer.
	for _, p := range outsiders {
		r := p.call(playv1connect.CombatServiceListCombatLogProcedure, &playv1.ListCombatLogRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), SinceSeq: 1})
		if r.ok() {
			t.Errorf("%s read the combat log: %s", p.name, shorten(r.body))
			continue
		}
		if low := strings.ToLower(string(r.body)); strings.Contains(low, "seq") || strings.Contains(low, "full") {
			t.Errorf("the refusal of the combat log to %s names a counter: %s", p.name, r.body)
		}
	}

	// Controls: who may watch reads a number, and Otto's own stream tells him his campaign's, not ours.
	w.hints.Settle(w.t)
	ours := w.watchStream(w.ana).number()
	if ours == 0 {
		t.Error("Ana's stream says number 0 in a session that has had many writes")
	}
	for _, p := range []*person{w.master, w.caio} {
		if got := w.watchStream(p).number(); got != ours {
			t.Errorf("%s's stream says %d, Ana's says %d: one session has one number", p.name, got, ours)
		}
	}
	stream, err := other.play.WatchGameSession(ctx, connect.NewRequest(&playv1.WatchGameSessionRequest{CampaignId: otherCampaign}))
	if err == nil {
		if stream.Receive() {
			if got := stream.Msg().GetReady().GetSessionSeq(); got == ours {
				t.Errorf("Otto's stream of his own campaign says our number %d", got)
			}
		}
		err = stream.Err()
		_ = stream.Close()
	}
	// his campaign has no open session: whatever it answers, it answers nothing of ours
	_ = err
}

// ===== A write only the master may see.

// hiddenWrite is a write made to what a player may not see. prepare makes what the write needs (a character, say) before the
// worlds are compared, and returns the write itself; the content differs by variant: a name that is long or short, a number of
// hit points, another hidden combatant. Each variant registers its content as a canary of its own.
type hiddenWrite struct {
	name       string
	masterLine bool // the master's log shows a line for it (the control that the write was made)
	// roster is true for the writes that change who is in the combat (AddCombatants, RemoveCombatant): they have always told every
	// player "encounter_changed", an event of an id and a mode, even when the combatant is hidden. That hint is older than the
	// hint number and is not what this test pins; it is the one event allowed, once, for these writes, and reported apart.
	roster bool
	// before is made in every world, the one without the write too, before anything is read or watched: a write the table sees
	// that the hidden one then joins (a firing the master adds a creature to). It leaves what prepare needs in w.notes.
	before  func(w *world)
	prepare func(w *world, variant int) func()
}

var hiddenWrites = []hiddenWrite{
	{"a hidden monster added", false, false, nil, func(w *world, variant int) func() {
		kind := "sq-m" // a combatant's name holds 30 characters at most: LEAKCANARY-, the kind and its number
		if variant == 1 {
			kind = "sq-monster-longer"
		}
		return func() {
			hidden := true
			w.scriptD20(ownFace)
			added := must(w.master.combat.AddMonsters(w.t.Context(), rq(&playv1.AddMonstersRequest{
				CampaignId: w.campaign, EncounterId: w.encounter.GetId(), IdempotencyKey: newKey(), CreatureKey: "monster:bandit", Count: 1, Name: w.secrets.marker(kind), Hidden: &hidden,
			})))
			w.requireOwnTotal(added.GetCombatantIds()[0])
		}
	}},
	{"a hit point correction of a hidden NPC", true, false, nil, func(w *world, variant int) func() {
		damage := w.secrets.number("seq-hidden-damage", []int64{4111, 4213}[variant])
		return func() {
			must(w.master.combat.AdjustCombatantHitPoints(w.t.Context(), rq(&playv1.AdjustCombatantHitPointsRequest{
				CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: w.combatant(w.hiddenNPC).GetId(), IdempotencyKey: newKey(),
				Change: &playv1.AdjustCombatantHitPointsRequest_Damage{Damage: int32(damage)},
			})))
		}
	}},
	{"a hidden NPC added as a reinforcement", false, true, nil, func(w *world, variant int) func() {
		npc, _ := w.npc("sq-reinforce-"+[]string{"a", "bcdefgh"}[variant], &charactersv1.BasicSheet{HitPointsMax: int32(4400 + variant), ArmorClass: 12, SpeedFt: 30})
		w.secrets.id("sq-reinforce", npc.GetId())
		return func() {
			hidden := true
			must(w.master.combat.AddCombatants(w.t.Context(), rq(&playv1.AddCombatantsRequest{
				CampaignId: w.campaign, EncounterId: w.encounter.GetId(), IdempotencyKey: newKey(),
				Participants: []*playv1.Participant{{CharacterId: npc.GetId(), Hidden: &hidden}},
			})))
		}
	}},
	{"a hidden combatant removed", false, true, nil, func(w *world, variant int) func() {
		who := []*charactersv1.Character{w.hiddenNPC, w.bandit}[variant]
		return func() {
			must(w.master.combat.RemoveCombatant(w.t.Context(), rq(&playv1.RemoveCombatantRequest{
				CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: w.combatant(who).GetId(), IdempotencyKey: newKey(),
			})))
		}
	}},
	{
		name:       "a hidden NPC added to a firing the table saw",
		masterLine: true,
		before: func(w *world) { // the master fires the trap at the boss: a line the players hold, whose firing the hidden NPC joins
			fired := must(w.master.play.FireTrap(w.t.Context(), rq(&playv1.FireTrapRequest{
				CampaignId: w.campaign, MapId: w.fogMap, PointId: w.pts["trap-guardroom"].GetId(), IdempotencyKey: newKey(),
				TargetIds: []string{w.combatantOf(w.encounter, w.boss.GetId()).GetId()},
			})))
			w.notes["firing"] = fired.GetFiring().GetId()
			firedTrapIsPublic(w) // the firing is the table's: its name and point are no secret from now on
		},
		prepare: func(w *world, variant int) func() {
			face := []int{17, 2}[variant]
			return func() { // the master adds the hidden NPC to that firing: an event only he hears joins the line
				w.scriptD20(face)
				must(w.master.play.FireTrap(w.t.Context(), rq(&playv1.FireTrapRequest{
					CampaignId: w.campaign, MapId: w.fogMap, PointId: w.pts["trap-guardroom"].GetId(), IdempotencyKey: newKey(), ExtendFiringId: w.notes["firing"],
					TargetIds: []string{w.combatantOf(w.encounter, w.hiddenNPC.GetId()).GetId()},
				})))
			}
		},
	},
	{
		// A trap the master fires by hand on a map no player sees (outside a combat: none runs there): the firing writes an event, so the
		// number moves once, and what the table is told is the master's alone. A player's stream and the twin's without the firing read
		// the same; the trap and what it did to the party are hidden whatever they are (the two variants fire two other traps).
		name: "a hidden trap fired outside a combat on a map the players do not see",
		before: func(w *world) {
			must(w.master.maps.SetMapGrid(w.t.Context(), rq(&mapsv1.SetMapGridRequest{CampaignId: w.campaign, MapId: w.hiddenMap, Columns: 12}))) // a map with no grid has no traps
			for i, name := range []string{"seq-fired-a", "seq-fired-bcdefgh"} {
				w.pts[name] = w.point(w.hiddenMap, mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, w.secrets.marker(name), w.secrets.marker(name+"-description"), 8+i, 8, func(r *mapsv1.CreateMapPointRequest) {
					r.Trap = w.trapSpec()
					r.Trap.Trigger = rulesv1.TrapTrigger_TRAP_TRIGGER_MANUAL
				})
				w.secrets.id("trap", w.pts[name].GetId())
			}
		},
		prepare: func(w *world, variant int) func() {
			point := w.pts[[]string{"seq-fired-a", "seq-fired-bcdefgh"}[variant]]
			return func() {
				must(w.master.play.FireTrap(w.t.Context(), rq(&playv1.FireTrapRequest{CampaignId: w.campaign, MapId: w.hiddenMap, PointId: point.GetId(), IdempotencyKey: newKey()})))
			}
		},
	},
	{
		// The master changes the table's rules while a combat runs with hidden combatants: the rules decide what the log shows (the
		// save DC, the death saves), so the master's log is read again from the start and the number moves once. A player hears
		// nothing that names a kind: their stream reads the same whichever rule was set.
		name: "the table's rules changed in a combat with hidden combatants",
		prepare: func(w *world, variant int) func() {
			return func() {
				rules := must(w.master.campaigns.GetTableRules(w.t.Context(), rq(&campaignsv1.GetTableRulesRequest{CampaignId: w.campaign}))).GetRules()
				rules.ShowSaveDc = variant == 1
				must(w.master.campaigns.SetTableRules(w.t.Context(), rq(&campaignsv1.SetTableRulesRequest{CampaignId: w.campaign, Rules: rules})))
			}
		},
	},
	{"a cover mark on a hidden NPC", false, false, nil, func(w *world, variant int) func() {
		cover := []playv1.CoverDegree{playv1.CoverDegree_COVER_DEGREE_HALF, playv1.CoverDegree_COVER_DEGREE_THREE_QUARTERS}[variant]
		return func() {
			must(w.master.combat.SetCombatantCover(w.t.Context(), rq(&playv1.SetCombatantCoverRequest{
				CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: w.combatant(w.hiddenNPC).GetId(), IdempotencyKey: newKey(), Cover: cover,
			})))
		}
	}},
	{"a condition of a hidden NPC", false, false, nil, func(w *world, variant int) func() {
		key := []string{"condition:poisoned", "condition:frightened"}[variant]
		return func() {
			must(w.master.combat.SetCombatantConditions(w.t.Context(), rq(&playv1.SetCombatantConditionsRequest{
				CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: w.combatant(w.hiddenNPC).GetId(), IdempotencyKey: newKey(), Conditions: &playv1.ConditionList{Keys: []string{key}},
			})))
		}
	}},
	// The writes of the maps module (#735 part 2, 4d maps). Each one is made to what no player sees (a map they cannot open, or a token
	// that is hidden): the master hears map_changed, the players hear nothing, and their number moves once, the same whatever the write held.
	{
		name: "a token placed on a map the players do not see",
		before: func(w *world) {
			for _, kind := range []string{"seq-token-a", "seq-token-bcdefgh"} {
				npc, _ := w.npc(kind, &charactersv1.BasicSheet{HitPointsMax: 12, ArmorClass: 12, SpeedFt: 30})
				w.notes[kind] = npc.GetId()
			}
		},
		prepare: func(w *world, variant int) func() {
			id := w.notes[[]string{"seq-token-a", "seq-token-bcdefgh"}[variant]]
			return func() { w.place(w.hiddenMap, id, 3+variant*5, 6+variant) }
		},
	},
	{
		name: "a token moved on a map the players do not see",
		before: func(w *world) {
			npc, _ := w.npc("seq-token-moved", &charactersv1.BasicSheet{HitPointsMax: 12, ArmorClass: 12, SpeedFt: 30})
			w.notes["moved"] = npc.GetId()
			w.place(w.hiddenMap, npc.GetId(), 2, 2)
		},
		prepare: func(w *world, variant int) func() {
			return func() { w.place(w.hiddenMap, w.notes["moved"], 5+variant*6, 4+variant*3) }
		},
	},
	{
		name: "the fog of a map the players do not see written",
		before: func(w *world) {
			must(w.master.maps.SetMapGrid(w.t.Context(), rq(&mapsv1.SetMapGridRequest{CampaignId: w.campaign, MapId: w.hiddenMap, Columns: 12})))
		},
		prepare: func(w *world, variant int) func() {
			light := []mapsv1.LightLevel{mapsv1.LightLevel_LIGHT_LEVEL_DIM, mapsv1.LightLevel_LIGHT_LEVEL_DARK}[variant]
			return func() {
				must(w.master.maps.SetMapFog(w.t.Context(), rq(&mapsv1.SetMapFogRequest{CampaignId: w.campaign, MapId: w.hiddenMap, FogEnabled: proto.Bool(true), BaseLight: &light})))
			}
		},
	},
	{
		name: "a wall painted on a map the players do not see",
		before: func(w *world) {
			must(w.master.maps.SetMapGrid(w.t.Context(), rq(&mapsv1.SetMapGridRequest{CampaignId: w.campaign, MapId: w.hiddenMap, Columns: 12})))
		},
		prepare: func(w *world, variant int) func() {
			return func() {
				w.paint(w.hiddenMap, mapsv1.MapLayer_MAP_LAYER_WALL, 1, [][2]int32{{int32(2 + variant), 3}, {int32(3 + variant*4), 3}})
			}
		},
	},
	{
		name: "a light painted on a map the players do not see",
		before: func(w *world) {
			must(w.master.maps.SetMapGrid(w.t.Context(), rq(&mapsv1.SetMapGridRequest{CampaignId: w.campaign, MapId: w.hiddenMap, Columns: 12})))
		},
		prepare: func(w *world, variant int) func() {
			return func() {
				w.paint(w.hiddenMap, mapsv1.MapLayer_MAP_LAYER_LIGHT, int32(2+variant), [][2]int32{{int32(2 + variant), 5}, {int32(3 + variant*4), 6}})
			}
		},
	},
	{
		name: "a locked door painted on a map the players do not see",
		before: func(w *world) {
			must(w.master.maps.SetMapGrid(w.t.Context(), rq(&mapsv1.SetMapGridRequest{CampaignId: w.campaign, MapId: w.hiddenMap, Columns: 12})))
		},
		prepare: func(w *world, variant int) func() {
			return func() {
				w.paint(w.hiddenMap, mapsv1.MapLayer_MAP_LAYER_DOORS, 3, [][2]int32{{int32(2 + variant), 6}, {int32(3 + variant*4), 7}})
			}
		},
	},
	{
		name: "a point created on a map the players do not see",
		prepare: func(w *world, variant int) func() {
			kind := []mapsv1.MapPointKind{mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, mapsv1.MapPointKind_MAP_POINT_KIND_BATTLE}[variant]
			name := w.secrets.marker([]string{"seq-point-a", "seq-point-bcdefgh"}[variant])
			return func() { w.point(w.hiddenMap, kind, name, name+"-description", 5, 5+variant, nil) }
		},
	},
	{
		name: "a hidden trap renamed on a map the players do not see",
		before: func(w *world) {
			must(w.master.maps.SetMapGrid(w.t.Context(), rq(&mapsv1.SetMapGridRequest{CampaignId: w.campaign, MapId: w.hiddenMap, Columns: 12})))
			w.pts["seq-trap-edit"] = w.point(w.hiddenMap, mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, w.secrets.marker("seq-trap-edit"), w.secrets.marker("seq-trap-edit-description"), 6, 6, func(r *mapsv1.CreateMapPointRequest) { r.Trap = w.trapSpec() })
			w.secrets.id("trap", w.pts["seq-trap-edit"].GetId())
		},
		prepare: func(w *world, variant int) func() {
			p := w.pts["seq-trap-edit"]
			name := w.secrets.marker([]string{"seq-trap-renamed-a", "seq-trap-renamed-bcdefgh"}[variant])
			return func() {
				must(w.master.maps.UpdateMapPoint(w.t.Context(), rq(&mapsv1.UpdateMapPointRequest{CampaignId: w.campaign, MapId: w.hiddenMap, PointId: p.GetId(), Name: &name})))
			}
		},
	},
	{
		name: "a trap disarmed on a map the players do not see",
		before: func(w *world) {
			must(w.master.maps.SetMapGrid(w.t.Context(), rq(&mapsv1.SetMapGridRequest{CampaignId: w.campaign, MapId: w.hiddenMap, Columns: 12})))
			for _, name := range []string{"seq-disarm-a", "seq-disarm-bcdefgh"} {
				w.pts[name] = w.point(w.hiddenMap, mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, w.secrets.marker(name), w.secrets.marker(name+"-description"), 7, 7, func(r *mapsv1.CreateMapPointRequest) { r.Trap = w.trapSpec() })
				w.secrets.id("trap", w.pts[name].GetId())
			}
		},
		prepare: func(w *world, variant int) func() {
			p := w.pts[[]string{"seq-disarm-a", "seq-disarm-bcdefgh"}[variant]]
			return func() {
				must(w.master.maps.DisarmTrap(w.t.Context(), rq(&mapsv1.DisarmTrapRequest{CampaignId: w.campaign, MapId: w.hiddenMap, PointId: p.GetId()})))
			}
		},
	},
	{
		name: "a treasure placed on a map the players do not see",
		before: func(w *world) {
			must(w.master.maps.SetMapGrid(w.t.Context(), rq(&mapsv1.SetMapGridRequest{CampaignId: w.campaign, MapId: w.hiddenMap, Columns: 12})))
		},
		prepare: func(w *world, variant int) func() {
			seed := uint64(w.secrets.number("seq-treasure-seed", []int64{6101, 6202}[variant]))
			name := w.secrets.marker([]string{"seq-treasure-a", "seq-treasure-bcdefgh"}[variant])
			return func() {
				rolled := must(w.master.treasure.GenerateTreasure(w.t.Context(), rq(&mapsv1.GenerateTreasureRequest{CampaignId: w.campaign, Mode: mapsv1.TreasureMode_TREASURE_MODE_HOARD, PartyLevel: proto.Int32(5), Seed: &seed}))).GetTreasure()
				must(w.master.treasure.PlaceTreasure(w.t.Context(), rq(&mapsv1.PlaceTreasureRequest{
					CampaignId: w.campaign, MapId: w.hiddenMap, Mode: mapsv1.TreasureMode_TREASURE_MODE_HOARD, PartyLevel: 5, Seed: &seed, Column: 4, Row: 4,
					Name: &name, IdempotencyKey: newKey(), ContentVersion: rolled.GetContentVersion(),
				})))
			}
		},
	},
}

// seqSnapshot is what two members and the master read of a world after a hidden write, or without one.
type seqSnapshot struct {
	start, end int64             // the number before the write and after it (every stream agrees)
	players    map[string]string // each player's views (stream events after ready, logs) with ids and times replaced
	masterLog  string
}

var (
	uuidRE = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	timeRE = regexp.MustCompile(`\d{4}-\d\d-\d\dT[0-9:.]+Z`)
)

// normalized replaces the ids by their order of appearance and the times by "T": two worlds built the same way read the same
// text, though each has ids and times of its own.
func normalized(s string) string {
	ids := map[string]string{}
	s = uuidRE.ReplaceAllStringFunc(s, func(id string) string {
		if _, ok := ids[id]; !ok {
			ids[id] = fmt.Sprintf("id%d", len(ids))
		}
		return ids[id]
	})
	return timeRE.ReplaceAllString(s, "T")
}

// hiddenWriteWorld builds a world, makes the write (nil: none) and reads what the streams and the logs of both players say.
// Inside the world it also proves the rules that need no other world: the players heard no event, the heartbeat holds the
// number and nothing else, nothing a canary holds is in any answer, and every log answer, plain and from since_seq, is
// what it was before the write.
func hiddenWriteWorld(t *testing.T, row *hiddenWrite, variant int, made bool) seqSnapshot {
	t.Helper()
	write := row
	if !made {
		write = nil
	}
	w := newWorld(t)
	players := []*person{w.ana, w.caio}
	if row.before != nil {
		row.before(w)
	}
	// The world's setup ends with writes whose hints go out on a timer: let them go first, or they pass for the write's.
	w.hints.Settle(w.t)
	watchers := map[*person]*streamWatcher{w.master: w.watchStream(w.master), w.ana: w.watchStream(w.ana), w.caio: w.watchStream(w.caio)}
	snap := seqSnapshot{start: watchers[w.ana].number(), players: map[string]string{}}
	for p, sw := range watchers {
		if sw.number() != snap.start {
			t.Fatalf("%s's stream opened at number %d, Ana's at %d", p.name, sw.number(), snap.start)
		}
	}
	type views struct{ plain, since *playv1.ListCombatLogResponse }
	read := func(p *person, from int64) views {
		return views{plain: w.seqLog(p, 0), since: w.seqLog(p, from)}
	}
	var doWrite func()
	if write != nil {
		doWrite = write.prepare(w, variant)
	}
	base := map[*person]views{}
	for _, p := range players {
		first := w.seqLog(p, 0)
		base[p] = views{plain: first, since: w.seqLog(p, first.GetLastSeq())}
	}
	masterBefore := w.seqLog(w.master, 0)
	if n := len(masterBefore.GetRounds()[0].GetEntries()); n == 0 || masterBefore.GetRounds()[0].GetEntries()[0].GetSeq() == 0 {
		t.Fatalf("the master's entries carry no number (%d entries): the control is gone", n)
	}

	if write != nil {
		doWrite()
	}

	// The number: a heartbeat of every stream reads what the write left (one more than before, or the same without a write).
	want := snap.start
	if write != nil {
		want++
	}
	for _, sw := range watchers {
		snap.end = sw.beatAfter(t, 1, func(got int64) bool { return got == want })
		// and it stays there: a beat after that one reads the same (the end of the stream checks that none read more)
		sw.beatAfter(t, sw.count(), func(got int64) bool { return got == want })
	}
	if write != nil && write.masterLine {
		if masterAfter := w.seqLog(w.master, 0); proto.Equal(masterBefore, masterAfter) {
			t.Errorf("%s: the master's log did not change: the write was not made", write.name)
		}
	}
	for _, p := range players {
		after := read(p, base[p].plain.GetLastSeq())
		if !proto.Equal(base[p].plain, after.plain) {
			t.Errorf("a player read another whole log after a write only the master sees (%s):\n before %s\n after  %s", p.name, jsonOf(base[p].plain), jsonOf(after.plain))
		}
		if !proto.Equal(base[p].since, after.since) {
			t.Errorf("a player read another answer to since_seq after a write only the master sees (%s):\n before %s\n after  %s", p.name, jsonOf(base[p].since), jsonOf(after.since))
		}
		for _, v := range []*playv1.ListCombatLogResponse{after.plain, after.since} {
			requireNoEntrySeq(t, p.name, v)
			for _, f := range w.inspect(p, asReply(v), nil) {
				t.Errorf("%s: the combat log: %s", p.name, f)
			}
		}
		snap.players[p.name] = normalized(jsonOf(after.plain) + "\n" + jsonOf(after.since))
	}
	snap.masterLog = normalized(jsonOf(w.seqLog(w.master, 0)))

	// The streams until the session ends: a player heard no event, the master heard the write. Ending the session is a write too, and moves
	// the number once: a heartbeat that goes out after the call reads the number of the end, so the number of the heartbeats is
	// checked up to the event each stream had when the call was made.
	beforeEnd := map[*person]int{}
	for p, sw := range watchers {
		beforeEnd[p] = sw.count()
	}
	must(w.master.play.EndGameSession(t.Context(), rq(&playv1.EndGameSessionRequest{CampaignId: w.campaign, GameSessionId: w.session})))
	for p, sw := range watchers {
		select {
		case <-sw.done:
		case <-time.After(30 * time.Second):
			t.Fatalf("%s's stream did not end", p.name)
		}
		heard := 0
		for i, ev := range sw.since(1) {
			switch c := eventCase(ev); c {
			case "heartbeat":
				n := ev.GetHeartbeat().GetSessionSeq()
				if i+1 < beforeEnd[p] && n != snap.start && n != want {
					t.Errorf("%s: a heartbeat says %d, want %d or %d (one write, one bump)", p.name, n, snap.start, want)
				}
				if p != w.master && !proto.Equal(ev, &playv1.WatchGameSessionResponse{SessionSeq: n, Event: &playv1.WatchGameSessionResponse_Heartbeat_{Heartbeat: &playv1.WatchGameSessionResponse_Heartbeat{SessionSeq: n}}}) {
					t.Errorf("%s: a heartbeat holds more than the number: %s", p.name, jsonOf(ev))
				}
			case "session_ended":
			case "encounter_changed":
				if write == nil || !write.roster || p == w.master { // the roster hint of a roster write is allowed (hiddenWrite.roster)
					heard++
					if p != w.master {
						t.Errorf("%s heard %q after a write only the master sees: %s", p.name, c, shorten(mustJSON(ev)))
					}
				}
			default:
				heard++
				if p != w.master {
					t.Errorf("%s heard %q after a write only the master sees: %s", p.name, c, shorten(mustJSON(ev)))
				}
			}
			if p != w.master {
				body, _ := protojson.Marshal(ev)
				for _, f := range w.inspect(p, reply{status: 200, body: body, msg: ev}, nil) {
					t.Errorf("%s: event %s: %s", p.name, eventCase(ev), f)
				}
			}
		}
		if p == w.master && write != nil && heard == 0 {
			t.Errorf("%s: the master heard nothing of the write: the control is gone", write.name)
		}
	}
	return snap
}

// requireNoEntrySeq fails for an entry of a player's log that carries a number: the numbers of the entries a player reads would have
// gaps where an event they may not see sits. The master's entries carry theirs.
func requireNoEntrySeq(t *testing.T, who string, l *playv1.ListCombatLogResponse) {
	t.Helper()
	for _, r := range l.GetRounds() {
		for _, en := range r.GetEntries() {
			if en.GetSeq() != 0 {
				t.Errorf("%s's entry %s carries seq %d, want 0 (the master's alone)", who, en.GetId(), en.GetSeq())
			}
		}
	}
}

func (w *world) seqLog(p *person, since int64) *playv1.ListCombatLogResponse {
	w.t.Helper()
	return must(p.combat.ListCombatLog(w.t.Context(), rq(&playv1.ListCombatLogRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), SinceSeq: since})))
}

// TestAHiddenWriteMovesThePlayersNumberAndNothingElse: three worlds built the same way, with a write only the master sees
// (content A in one, content B in another, in the third none). A player's view of the world with the write and of the world
// without it differ in the number and in nothing else; the two worlds with the write read the same, down to the number, though
// the hidden content differs (a longer name, other hit points): the number is a counter of writes that had an effect, never a function of
// what they held. Inside each world the players heard no event, a heartbeat held the number alone, and the answers of the combat log are
// what they were before (hiddenWriteWorld).
func TestAHiddenWriteMovesThePlayersNumberAndNothingElse(t *testing.T) {
	// The world without the write is built the same way for every row that makes nothing before it (hiddenWrite.before): one
	// of them serves all of those rows, built by the first that asks.
	var (
		sharedOnce sync.Once
		shared     seqSnapshot
		sharedOK   bool
	)
	control := func(t *testing.T, hw *hiddenWrite) seqSnapshot {
		t.Helper()
		if hw.before != nil {
			return hiddenWriteWorld(t, hw, 0, false)
		}
		sharedOnce.Do(func() {
			shared = hiddenWriteWorld(t, hw, 0, false)
			sharedOK = true // not reached when the world's build stopped its test
		})
		if !sharedOK {
			t.Fatal("the world without a write, shared by the rows that make nothing before it, could not be built")
		}
		return shared
	}
	// The rows run in two groups at the same time (each row builds three worlds, and most of a world's time is the database's
	// copy of the schema, which waits for the server, not for this process); inside a group one row follows another.
	for g := range 2 {
		t.Run(fmt.Sprintf("group %d", g+1), func(t *testing.T) {
			t.Parallel()
			for i := g; i < len(hiddenWrites); i += 2 {
				hw := &hiddenWrites[i]
				t.Run(hw.name, func(t *testing.T) {
					a := hiddenWriteWorld(t, hw, 0, true)
					b := hiddenWriteWorld(t, hw, 1, true)
					none := control(t, hw)
					if a.start != b.start || a.start != none.start {
						t.Errorf("the worlds opened at numbers %d, %d and %d: they are built the same way", a.start, b.start, none.start)
					}
					if a.end != a.start+1 || b.end != b.start+1 || none.end != none.start {
						t.Errorf("the numbers after: %d, %d and %d (before: %d), want one bump for each write and none without", a.end, b.end, none.end, a.start)
					}
					for name := range a.players {
						if a.players[name] != b.players[name] {
							t.Errorf("%s read other things in two worlds whose hidden content differs:\n A %s\n B %s", name, shorten([]byte(a.players[name])), shorten([]byte(b.players[name])))
						}
						if a.players[name] != none.players[name] {
							t.Errorf("%s read other things in the world with the hidden write and the world without it:\n with    %s\n without %s", name, shorten([]byte(a.players[name])), shorten([]byte(none.players[name])))
						}
					}
					if hw.masterLine && (a.masterLog == b.masterLog || a.masterLog == none.masterLog) {
						t.Errorf("the master read the same log in worlds that differ in what he wrote: the control is gone")
					}
				})
			}
		})
	}
}

// ===== A hidden event that joins a line the player holds.

// joinedRead is what two players read of a world where the master answered the window of a held cast, and the master's own number.
type joinedRead struct {
	start, end int64             // the hint number before the master's answer and after it
	players    map[string]string // each player's plain log, answer from the number they held before, and answer from the one they hold now
	lastSeq    map[string]int64  // each player's last_seq
	master     string            // the master's whole log, as text
}

// joinedHostWorld plays Ana's Sacred Flame at a dragon with three uses of Legendary Resistance in a world of its own. The dragon's
// save rolls face; the master passes the window, or spends a use (the use is a Secret event that joins the line of the cast, the
// roll of the save is Secret too). It reads what both players read before the answer and after it.
func joinedHostWorld(t *testing.T, face int, answer heldAnswer) joinedRead {
	t.Helper()
	w := newWorld(t)
	ctx := t.Context()
	pens, dragon := w.sacredFlameFight()
	w.scriptD20(face)
	cast := w.ana.call(playv1connect.CombatServiceCastSpellProcedure, &playv1.CastSpellRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CasterId: pens.GetId(), SpellKey: "spell:sacred-flame", IdempotencyKey: newKey(),
		Targets: []*playv1.SpellTarget{{CombatantId: dragon.GetId()}},
	})
	if !cast.ok() {
		t.Fatalf("Ana's cast: status %d %s", cast.status, cast.body)
	}
	w.hints.Settle(w.t)
	players := []*person{w.ana, w.caio}
	watchers := map[*person]*streamWatcher{w.master: w.watchStream(w.master), w.ana: w.watchStream(w.ana), w.caio: w.watchStream(w.caio)}
	out := joinedRead{start: watchers[w.ana].number(), players: map[string]string{}, lastSeq: map[string]int64{}}
	held := map[*person]int64{}
	for _, p := range players {
		held[p] = w.seqLog(p, 0).GetLastSeq() // the cast waits: the number a player holds before the answer
		w.reads(p)                            // the table reads the combat while it waits, as the rows of legendary_test.go do
	}
	var window *playv1.ReactionWindow
	for _, rw := range must(w.master.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter().GetReactionWindows() {
		if rw.GetKind() == playv1.ReactionKind_REACTION_KIND_LEGENDARY_RESISTANCE {
			window = rw
		}
	}
	if window == nil {
		t.Fatal("the master reads no Legendary Resistance window: the control is gone")
	}
	req := &playv1.AnswerReactionRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: window.GetId(), Answer: playv1.ReactionChoice_REACTION_CHOICE_PASS, IdempotencyKey: newKey()}
	switch answer {
	case removeDragon:
		must(w.master.combat.RemoveCombatant(ctx, rq(&playv1.RemoveCombatantRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: dragon.GetId(), IdempotencyKey: newKey()})))
	default:
		if answer == useTrait {
			req.Answer, req.CreatureIds = playv1.ReactionChoice_REACTION_CHOICE_USE, []string{dragon.GetId()}
		}
		if resp := w.master.call(playv1connect.CombatServiceAnswerReactionProcedure, req); !resp.ok() {
			t.Fatalf("the master's answer: status %d %s", resp.status, resp.body)
		}
	}
	out.end = watchers[w.ana].beatAfter(t, 1, func(got int64) bool { return got == out.start+1 })
	for _, p := range players {
		plain := w.seqLog(p, 0)
		fromHeld := w.seqLog(p, held[p])
		fromNow := w.seqLog(p, plain.GetLastSeq())
		for _, l := range []*playv1.ListCombatLogResponse{plain, fromHeld, fromNow} {
			requireNoEntrySeq(t, p.name, l)
			// The dragon is in sight of the table (the cast is at it): its name is no canary here. What the players never read is
			// the trait: the words of the use and of the window.
			for _, word := range []string{"legendary", "resistance", "resisted", "usesLeft", window.GetId()} {
				if contains(asReply(l).body, word) {
					t.Errorf("%s: the combat log holds %q, which is the master's alone", p.name, word)
				}
			}
		}
		out.players[p.name] = normalized(jsonOf(plain) + "\n" + jsonOf(fromHeld) + "\n" + jsonOf(fromNow))
		out.lastSeq[p.name] = plain.GetLastSeq()
		if fromHeld.GetLastSeq() != plain.GetLastSeq() || fromNow.GetLastSeq() != plain.GetLastSeq() {
			t.Errorf("%s: last_seq of the three answers is %d, %d and %d, want one number", p.name, plain.GetLastSeq(), fromHeld.GetLastSeq(), fromNow.GetLastSeq())
		}
		if len(fromNow.GetRounds()) != 0 || fromNow.GetFull() {
			t.Errorf("%s: asking from the number held now answers %s, want nothing new", p.name, jsonOf(fromNow))
		}
	}
	out.master = jsonOf(w.seqLog(w.master, 0))
	return out
}

// TestTheMastersAnswerToAHeldCastReadsTheSameToThePlayersWhateverItWas: the master answers the window of a held cast at a dragon, in worlds
// that differ only in what he does and in what the dragon rolled. A Secret event joins the line of the cast when he spends a use of
// Legendary Resistance (with the roll as its content) and when he passes; the line the players read is the same cast either way.
//   - he answers: a use spent after a roll of 1, a use spent after a roll of 2, the save passed;
//   - he takes the dragon out of the combat while the cast waits: after a roll of 1 (a use to offer), after a roll of 20 (none).
//
// In each set each player's whole log, the answer from the number they held and last_seq are the same, and no answer holds a word
// of the trait. The hint number moves by one in each world (one write). The master's log tells the worlds apart (the control).
func TestTheMastersAnswerToAHeldCastReadsTheSameToThePlayersWhateverItWas(t *testing.T) {
	runs := []struct {
		name   string
		face   int
		answer heldAnswer
	}{
		{"a use spent after a roll of 1", 1, useTrait},
		{"a use spent after a roll of 2", 2, useTrait},
		{"the save passed", 20, passWindow},
		{"the dragon removed after a roll of 1", 1, removeDragon},
		{"the dragon removed after a roll of 20", 20, removeDragon},
	}
	reads := make([]joinedRead, len(runs))
	for i, r := range runs {
		t.Run(r.name, func(t *testing.T) { reads[i] = joinedHostWorld(t, r.face, r.answer) })
		if reads[i].end != reads[i].start+1 || reads[i].start != reads[0].start {
			t.Errorf("%s: the number moved from %d to %d (the first world opened at %d), want one bump for the answer", r.name, reads[i].start, reads[i].end, reads[0].start)
		}
	}
	for _, set := range [][]int{{0, 1, 2}, {3, 4}} {
		first := reads[set[0]]
		for _, i := range set[1:] {
			for name := range first.players {
				if first.players[name] != reads[i].players[name] {
					t.Errorf("%s reads other things after %q and after %q:\n %s\n %s", name, runs[set[0]].name, runs[i].name, shorten([]byte(first.players[name])), shorten([]byte(reads[i].players[name])))
				}
				if first.lastSeq[name] != reads[i].lastSeq[name] {
					t.Errorf("%s's last_seq is %d after %q and %d after %q: an event they do not hear of moved their number", name, first.lastSeq[name], runs[set[0]].name, reads[i].lastSeq[name], runs[i].name)
				}
			}
		}
	}
	if !contains([]byte(reads[0].master), `"resisted":true`) || contains([]byte(reads[2].master), `"resisted":true`) {
		t.Error("the master's log does not tell the use spent from the save passed: the control is gone")
	}
}

// ===== since_seq.

// TestAPlayersSinceSeqAnswerIgnoresWhatTheyCannotSee: the answer to ListCombatLog(since_seq = X) of a player who may not see a
// write made after X is the answer without it (proto.Equal: entries, last_seq, full), for the writes below, and the master's
// answer moves (the control). A player who sees the write (Ana, who sees the dragon of the fogged action) reads it.
func TestAPlayersSinceSeqAnswerIgnoresWhatTheyCannotSee(t *testing.T) {
	w := newWorld(t)
	ctx := t.Context()
	e := w.encounter
	m := w.master
	hid := w.combatant(w.hiddenNPC)
	var dragon *playv1.Combatant // in Ana's sight, not in Caio's: the master left it shown
	for _, c := range e.GetCombatants() {
		if c.GetBestiaryCreatureKey() == dragonKey {
			dragon = c
		}
	}
	if dragon == nil {
		t.Fatal("the fixture has no dragon")
	}
	toren := w.combatant(w.toren)

	// asked takes the answers every member reads from the numbers they held before the write.
	type held map[*person]int64
	hold := func() held {
		h := held{}
		for _, p := range []*person{w.master, w.ana, w.caio} {
			h[p] = w.seqLog(p, 0).GetLastSeq()
		}
		return h
	}
	ask := func(h held) map[*person]*playv1.ListCombatLogResponse {
		out := map[*person]*playv1.ListCombatLogResponse{}
		for p, n := range h {
			out[p] = w.seqLog(p, n)
		}
		return out
	}
	// step makes a write and checks who read it. unmoved lists the people whose answer must be what it was.
	step := func(name string, unmoved []*person, moved []*person, write func()) {
		t.Helper()
		h := hold()
		before := ask(h)
		write()
		after := ask(h)
		for _, p := range unmoved {
			requireNoEntrySeq(t, p.name, after[p])
			if !proto.Equal(before[p], after[p]) {
				t.Errorf("%s: %s's answer to since_seq=%d moved:\n before %s\n after  %s", name, p.name, h[p], jsonOf(before[p]), jsonOf(after[p]))
			}
			for _, f := range w.inspect(p, asReply(after[p]), nil) {
				t.Errorf("%s: %s: %s", name, p.name, f)
			}
		}
		for _, p := range moved {
			if proto.Equal(before[p], after[p]) {
				t.Errorf("%s: %s's answer to since_seq=%d did not move: the control is gone (%s)", name, p.name, h[p], jsonOf(after[p]))
			}
		}
	}

	step("a hit point correction of a hidden NPC", []*person{w.ana, w.caio}, []*person{m}, func() {
		const damage = 4155
		w.secrets.number("seq-since-damage", damage)
		must(m.combat.AdjustCombatantHitPoints(ctx, rq(&playv1.AdjustCombatantHitPointsRequest{
			CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: hid.GetId(), IdempotencyKey: newKey(),
			Change: &playv1.AdjustCombatantHitPointsRequest_Damage{Damage: damage},
		})))
	})
	// A hidden NPC that no line the players hold names is shown and hidden: they read nothing new.
	toggle := func(hidden bool) func() {
		return func() {
			must(m.combat.SetCombatantHidden(ctx, rq(&playv1.SetCombatantHiddenRequest{CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: hid.GetId(), IdempotencyKey: newKey(), Hidden: hidden})))
		}
	}
	step("a hidden NPC shown, no line of it held", []*person{w.ana, w.caio}, []*person{m}, toggle(false))
	step("a hidden NPC hidden again", []*person{w.ana, w.caio}, []*person{m}, toggle(true))

	// The dragon in Ana's sight, not in Caio's, attacks Toren, out of turn, and the master takes the action back.
	w.hints.Settle(w.t)
	var attack *playv1.RollAttackResponse
	step("a fogged action", []*person{w.caio}, []*person{m, w.ana}, func() {
		opts := must(m.combat.GetTurnOptions(ctx, rq(&playv1.GetTurnOptionsRequest{CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: dragon.GetId()})))
		key := ""
		for _, a := range opts.GetOptions().GetAttacks() {
			if a.GetAttack().GetKey() != "" && a.GetAttack().GetSaveDc() == 0 {
				key = a.GetAttack().GetKey()
				break
			}
		}
		if key == "" {
			w.t.Fatal("the dragon has no attack to make")
		}
		attack = must(m.combat.RollAttack(ctx, rq(&playv1.RollAttackRequest{
			CampaignId: w.campaign, EncounterId: e.GetId(), AttackerId: dragon.GetId(), AttackKey: key, TargetId: toren.GetId(), IdempotencyKey: newKey(),
			D20Faces: []int32{19, 18}, AsReaction: true,
		})))
	})
	if attack == nil {
		t.Fatal("the dragon's attack was not made")
	}
	step("the fogged action taken back", []*person{w.caio}, []*person{m, w.ana}, func() {
		log := w.seqLog(m, 0)
		must(m.combat.UndoLastAction(ctx, rq(&playv1.UndoLastActionRequest{CampaignId: w.campaign, EncounterId: e.GetId(), ExpectedEventId: log.GetUndoableEventId(), IdempotencyKey: newKey()})))
	})
}

// ===== A drop that notices a trap, and one that does not.

// landing says where a landing happens and what the landing token is, in a world of landingWorld.
type landing struct {
	// unseenMap lands the token on a map the players do not see (neither revealed nor current), where no trap is.
	unseenMap bool
	// hiddenToken has the master hide Toren's token before it lands.
	hiddenToken bool
	// noticeDC is the DC to notice of the armed trap beside the landing square on the map the players see (0: never noticed by passing).
	noticeDC int32
	// how is the write that lands the token: "" is PlaceMapToken, "move" is MoveMapTokens and "party" is PutPartyOnMap. A move and a
	// party put run with no combat on the map, as a landing that fires a trap does (the combat of the world is ended).
	how string
	// fires lands the token on the square of the trap, so that the landing fires it.
	fires bool
	// npc lands an NPC's token (which notices and fires nothing) in place of Toren's.
	npc bool
}

// noticeWorld is landingWorld for a token that lands on the map the players see, visible, beside a trap with the given DC to notice.
func noticeWorld(t *testing.T, noticeDC int32) (ana, caio []string, moved int64) {
	t.Helper()
	return landingWorld(t, landing{noticeDC: noticeDC})
}

// landingWorld builds a world, adds an armed trap on the map the players see, has the master drop Toren (Caio's character) as l says
// (the token placed once before, so the landing is a move), and reads what Ana, a third player, was told: the kinds of her events (the
// heartbeats left out) and how far her number went from the landing. It returns what Caio heard as well.
func landingWorld(t *testing.T, l landing) (ana, caio []string, moved int64) {
	t.Helper()
	w := newWorld(t)
	w.pts["seq-notice-trap"] = w.point(w.fogMap, mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, w.secrets.marker("seq-notice-trap"), w.secrets.marker("seq-notice-trap-description"), 7, 13, func(r *mapsv1.CreateMapPointRequest) {
		r.Trap = w.trapSpec()
		r.Trap.NoticeDc = l.noticeDC
	})
	w.secrets.id("trap", w.pts["seq-notice-trap"].GetId())
	onMap := w.fogMap
	if l.unseenMap {
		onMap = w.map2
	}
	if l.fires || l.how != "" {
		must(w.master.combat.EndEncounter(t.Context(), rq(&playv1.EndEncounterRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), IdempotencyKey: newKey()})))
	}
	who := w.toren.GetId()
	if l.npc {
		who = w.stage2.GetId()
	}
	col := 8
	if l.fires {
		col = 7 // the trap's own square
	}
	w.place(onMap, who, 20, 2) // far from the trap: the first placement notices nothing
	if l.hiddenToken {
		must(w.master.maps.SetMapTokenHidden(t.Context(), rq(&mapsv1.SetMapTokenHiddenRequest{CampaignId: w.campaign, MapId: onMap, CharacterId: who, Hidden: true})))
	}
	w.hints.Settle(w.t)
	streams := map[*person]*streamWatcher{w.ana: w.watchStream(w.ana), w.caio: w.watchStream(w.caio)}
	start := streams[w.ana].number()
	x, y := at(col, 13)
	switch l.how {
	case "move":
		must(w.master.maps.MoveMapTokens(t.Context(), rq(&mapsv1.MoveMapTokensRequest{CampaignId: w.campaign, MapId: onMap, CharacterIds: []string{who}, XBp: x, YBp: y})))
	case "party":
		must(w.master.maps.PutPartyOnMap(t.Context(), rq(&mapsv1.PutPartyOnMapRequest{CampaignId: w.campaign, MapId: onMap, CharacterIds: []string{who}, XBp: x, YBp: y})))
	default:
		w.place(onMap, who, col, 13)
	}
	// The landing was made when place returned: a beat that begins after the next one reads what it left.
	watch := streams[w.ana]
	watch.beatAfter(t, watch.count(), func(int64) bool { return true })
	end := watch.beatAfter(t, watch.count(), func(int64) bool { return true })
	must(w.master.play.EndGameSession(t.Context(), rq(&playv1.EndGameSessionRequest{CampaignId: w.campaign, GameSessionId: w.session})))
	kinds := map[*person][]string{}
	for p, sw := range streams {
		<-sw.done
		for _, ev := range sw.since(1) {
			if c := eventCase(ev); c != "heartbeat" && c != "session_ended" {
				kinds[p] = append(kinds[p], c)
			}
		}
	}
	return kinds[w.ana], kinds[w.caio], end - start
}

// TestALandingMovesAThirdPlayersNumberOnceWhateverTheMapAndTheToken: a player's character that lands moves the session's number once,
// whether the map is one the players see or not (twins: the map the players see with a trap Toren notices, and a map they do not see)
// and whether the token is hidden or visible (twins: the same landing of a hidden token and of a visible one), so a third player's number
// says nothing of where the token landed or of what the master hid. Ana, who sees none of it, is the third player.
func TestALandingMovesAThirdPlayersNumberOnceWhateverTheMapAndTheToken(t *testing.T) {
	_, _, seen := landingWorld(t, landing{noticeDC: 1})
	_, _, unseen := landingWorld(t, landing{unseenMap: true, noticeDC: 1})
	_, _, hidden := landingWorld(t, landing{hiddenToken: true})
	_, _, visible := landingWorld(t, landing{})
	if seen != unseen {
		t.Errorf("Ana's number moved by %d where the map was seen and by %d where it was not, want the same", seen, unseen)
	}
	if hidden != visible {
		t.Errorf("Ana's number moved by %d where the token was hidden and by %d where it was visible, want the same", hidden, visible)
	}
	_, _, npc := landingWorld(t, landing{npc: true, noticeDC: 1})
	_, _, moves := landingWorld(t, landing{how: "move", noticeDC: 1})
	_, _, party := landingWorld(t, landing{how: "party", noticeDC: 1})
	_, _, movesUnseen := landingWorld(t, landing{how: "move", unseenMap: true, noticeDC: 1})
	if moves != movesUnseen {
		t.Errorf("Ana's number moved by %d for a move where the map was seen and by %d where it was not, want the same", moves, movesUnseen)
	}
	for name, moved := range map[string]int64{"seen": seen, "unseen": unseen, "hidden": hidden, "visible": visible, "an NPC's": npc, "a move": moves, "a party put": party, "a move on an unseen map": movesUnseen} {
		if moved != 1 {
			t.Errorf("Ana's number moved by %d for the landing in the %s world, want 1: a landing is one write", moved, name)
		}
	}
}

// TestADropThatNoticesATrapMovesAThirdPlayersNumberLikeOneThatDoesNot: a token dropped beside a trap is one write, whose number moves
// whether or not a trap was noticed (and whether or not the map holds one), so a player who
// hears nothing of it reads the same number and the same events in the world where Toren noticed the trap (a DC he reaches) and in
// the one where he could not (a trap with no DC to notice). Control: only Toren's player hears trap_noticed.
func TestADropThatNoticesATrapMovesAThirdPlayersNumberLikeOneThatDoesNot(t *testing.T) {
	anaA, caioA, movedA := noticeWorld(t, 1)
	anaB, caioB, movedB := noticeWorld(t, 0)
	if !slices.Contains(caioA, "trap_noticed") || slices.Contains(caioB, "trap_noticed") {
		t.Fatalf("Caio heard %v where the trap was noticed and %v where it was not: the control is gone", caioA, caioB)
	}
	if slices.Contains(anaA, "trap_noticed") {
		t.Errorf("Ana heard %v: the hint of a trap is its finder's alone", anaA)
	}
	if movedA != movedB {
		t.Errorf("Ana's number moved by %d where the trap was noticed and by %d where it was not, want the same", movedA, movedB)
	}
	if !slices.Equal(anaA, anaB) {
		t.Errorf("Ana heard %v where the trap was noticed and %v where it was not, want the same", anaA, anaB)
	}
}

// ===== A search whose d20 is kept for a question, and one that is not.

// heldSearchWorld builds a world and has Caio's character search for traps with a d20 of 20 (Investigation). With held, the character
// holds a Bardic Inspiration die, so the roll is kept for the question about it and the search writes only the held roll; without it,
// the search is made. It reads what Ana, a third player, was told: the kinds of her events (the heartbeats left out) and how far her
// number went. It returns what the master and Caio heard as well.
func heldSearchWorld(t *testing.T, held, inCombat bool) (ana, master, caio []string, moved int64) {
	t.Helper()
	w := newWorld(t)
	die := `INSERT INTO character_inspiration (character_id, campaign_id, sides, seconds_left) VALUES ($1, $2, 8, 600)`
	args := []any{w.toren.GetId(), w.campaign}
	if inCombat {
		die = `UPDATE combatants SET inspiration_sides = 8, inspiration_expires_round = (SELECT round FROM encounters WHERE id = combatants.encounter_id) + 50 WHERE id = $1`
		args = []any{w.combatant(w.toren).GetId()}
	} else {
		must(w.master.combat.EndEncounter(t.Context(), rq(&playv1.EndEncounterRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), IdempotencyKey: newKey()})))
		w.place(w.fogMap, w.toren.GetId(), 8, 13)
	}
	if held {
		if _, err := w.pool.Exec(t.Context(), die, args...); err != nil {
			t.Fatalf("give the die: %v", err)
		}
	}
	w.hints.Settle(w.t)
	streams := map[*person]*streamWatcher{w.master: w.watchStream(w.master), w.ana: w.watchStream(w.ana), w.caio: w.watchStream(w.caio)}
	start := streams[w.ana].number()
	res := must(w.caio.play.SearchForTraps(t.Context(), rq(&playv1.SearchForTrapsRequest{
		CampaignId: w.campaign, Skill: playv1.TrapSearchSkill_TRAP_SEARCH_SKILL_INVESTIGATION, IdempotencyKey: newKey(),
		Roll: &playv1.SearchForTrapsRequest_D20Face{D20Face: 20},
	})))
	if (res.GetHeldRollOffer() != nil) != held {
		t.Fatalf("the search = %s, want the roll held: %v", protojson.Format(res), held)
	}
	watch := streams[w.ana]
	watch.beatAfter(t, watch.count(), func(int64) bool { return true })
	end := watch.beatAfter(t, watch.count(), func(int64) bool { return true })
	must(w.master.play.EndGameSession(t.Context(), rq(&playv1.EndGameSessionRequest{CampaignId: w.campaign, GameSessionId: w.session})))
	kinds := map[*person][]string{}
	for p, sw := range streams {
		<-sw.done
		for _, ev := range sw.since(1) {
			if c := eventCase(ev); c != "heartbeat" && c != "session_ended" {
				kinds[p] = append(kinds[p], c)
			}
		}
	}
	return kinds[w.ana], kinds[w.master], kinds[w.caio], end - start
}

// TestAHeldSearchMovesAThirdPlayersNumberLikeOneThatIsNot: a search whose d20 waits for a die's question is one write for the held
// roll, and one for the search that is made, each moving the number once, so a player who hears nothing of either reads the same
// number and the same events in the world where the roll was kept and in the one where it was not. Control: only the master and
// the searcher hear that the questions changed.
func TestAHeldSearchMovesAThirdPlayersNumberLikeOneThatIsNot(t *testing.T) {
	for _, inCombat := range []bool{false, true} {
		t.Run(map[bool]string{false: "out of a combat", true: "the Search action of a combat"}[inCombat], func(t *testing.T) {
			heldSearchTwins(t, inCombat)
		})
	}
}

func heldSearchTwins(t *testing.T, inCombat bool) {
	t.Helper()
	anaH, masterH, caioH, movedH := heldSearchWorld(t, true, inCombat)
	anaN, masterN, caioN, movedN := heldSearchWorld(t, false, inCombat)
	for who, got := range map[string][]string{"the master": masterH, "Caio": caioH} {
		if !slices.Contains(got, "held_rolls_changed") {
			t.Errorf("%s heard %v where the roll was kept: the control is gone", who, got)
		}
	}
	if slices.Contains(masterN, "held_rolls_changed") || slices.Contains(caioN, "held_rolls_changed") {
		t.Errorf("the master heard %v and Caio %v where the roll was not kept: no question changed", masterN, caioN)
	}
	if slices.Contains(anaH, "held_rolls_changed") {
		t.Errorf("Ana heard %v: the hint of a question is its roller's and the master's alone", anaH)
	}
	if movedH != movedN {
		t.Errorf("Ana's number moved by %d where the roll was kept and by %d where it was not, want the same", movedH, movedN)
	}
	if !slices.Equal(anaH, anaN) {
		t.Errorf("Ana heard %v where the roll was kept and %v where it was not, want the same", anaH, anaN)
	}
}

// TestALandingThatFiresATrapMovesAThirdPlayersNumberByTheLandingAndTheFiring: a token dropped on the square of an armed trap fires it, in a write of
// play's own, and is not also noticed, so the number moves by the landing (once) and by the firing, and the landing adds nothing for a
// find. The same landing where the trap is never noticed by passing (no DC) moves it the same.
func TestALandingThatFiresATrapMovesAThirdPlayersNumberByTheLandingAndTheFiring(t *testing.T) {
	_, _, noticeable := landingWorld(t, landing{fires: true, noticeDC: 1})
	_, _, unnoticeable := landingWorld(t, landing{fires: true, noticeDC: 0})
	if noticeable != unnoticeable {
		t.Errorf("Ana's number moved by %d where the trap could be noticed and by %d where it could not, want the same", noticeable, unnoticeable)
	}
	if noticeable != 2 {
		t.Errorf("Ana's number moved by %d for a landing that fires, want 2: the landing, then the firing", noticeable)
	}
}

// takeBackWorld builds a world where the master left an image with Ana alone (or with Caio too, when withCaio), takes it back, and reads
// what Caio's stream was told: how far his number went, and the kinds of his events (the heartbeats left out).
func takeBackWorld(t *testing.T, withCaio bool) (moved int64, kinds []string) {
	t.Helper()
	w := newWorld(t)
	image := w.newShown("seq-take-back", w.ana, w.caio)
	aud := chosen(w.ana)
	if withCaio {
		aud = chosen(w.ana, w.caio)
	}
	w.showImage(image, true, aud)
	must(w.master.play.SetShownImage(t.Context(), rq(&playv1.SetShownImageRequest{CampaignId: w.campaign})))
	w.hints.Settle(w.t)
	watch := w.watchStream(w.caio)
	start := watch.number()
	must(w.master.play.TakeBackLeftImage(t.Context(), rq(&playv1.TakeBackLeftImageRequest{CampaignId: w.campaign, ImageId: image.id})))
	// The take-back was made when the call returned: a beat that begins after the next one reads what it left.
	watch.beatAfter(t, watch.count(), func(int64) bool { return true })
	end := watch.beatAfter(t, watch.count(), func(int64) bool { return true })
	for _, ev := range watch.since(1) {
		if c := eventCase(ev); c != "heartbeat" {
			kinds = append(kinds, c)
			if ev.GetSessionSeq() != start+1 {
				t.Errorf("Caio heard %s with number %d, want %d, the take-back's", c, ev.GetSessionSeq(), start+1)
			}
		}
	}
	return end - start, kinds
}

// TestATakeBackMovesAThirdPlayersNumberOnceWhetherItConcernedThemOrNot: taking back an image left on the table is one write that moves
// the number once, so Caio, who the image was not left with, reads the same number as Caio, who it was left with, and hears nothing
// but his own list's hint in the world where it concerned him (control). The number is all he learns of the take-back.
func TestATakeBackMovesAThirdPlayersNumberOnceWhetherItConcernedThemOrNot(t *testing.T) {
	movedNot, heardNot := takeBackWorld(t, false)
	movedWith, heardWith := takeBackWorld(t, true)
	if movedNot != 1 || movedWith != 1 {
		t.Errorf("Caio's number moved by %d where the image was not his and by %d where it was, want 1 for both: a take-back is one write", movedNot, movedWith)
	}
	if len(heardNot) != 0 {
		t.Errorf("Caio heard %v of the take-back of an image left with Ana alone, want nothing", heardNot)
	}
	if len(heardWith) != 1 || heardWith[0] != "left_images_changed" {
		t.Errorf("Caio heard %v of the take-back of an image left with him, want left_images_changed alone: the control is gone", heardWith)
	}
}
