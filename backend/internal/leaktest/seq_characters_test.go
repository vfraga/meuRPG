package leaktest

import (
	"slices"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	progressionv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1"
)

// The writes of a character that go through the funnel of package characters and progression (an XP award and its undo, a revival,
// a change of gear, the choices a player completes): each moves a third player's number once, whoever the character is, and says
// nothing else to her that depends on it. Ana, who owns none of what is written to (Toren, an NPC, Dan's character), is the
// third player of every twin below. Each test builds the twins in worlds of their own, with a write that differs in who it
// concerns (a character the table sees, one it does not), and compares what Ana's stream told her and how far her number went.

// seqTwin is what one world of a twin test read: how far Ana's number went from the write, and the kinds of the events each
// person's stream heard (the heartbeats and the end of the session left out), by the person's name.
type seqTwin struct {
	moved int64
	kinds map[string][]string
}

// seqTwinWorld builds a world, runs prepare (which may write, before anything is watched, and returns the write the twin is about)
// and watches the master, Ana and Caio while the write is made. The write was made when its call returned: a beat that begins after
// the next one reads what it left, which is how Ana's number is read, as the landing twins do.
func seqTwinWorld(t *testing.T, prepare func(w *world) func()) seqTwin {
	t.Helper()
	w := newWorld(t)
	write := prepare(w)
	w.hints.Settle(w.t)
	streams := map[*person]*streamWatcher{w.master: w.watchStream(w.master), w.ana: w.watchStream(w.ana), w.caio: w.watchStream(w.caio)}
	watch := streams[w.ana]
	start := watch.number()
	write()
	watch.beatAfter(t, watch.count(), func(int64) bool { return true })
	end := watch.beatAfter(t, watch.count(), func(int64) bool { return true })
	must(w.master.play.EndGameSession(t.Context(), rq(&playv1.EndGameSessionRequest{CampaignId: w.campaign, GameSessionId: w.session})))
	out := seqTwin{moved: end - start, kinds: map[string][]string{}}
	for p, sw := range streams {
		<-sw.done
		for _, ev := range sw.since(1) {
			if c := eventCase(ev); c != "heartbeat" && c != "session_ended" {
				out.kinds[p.name] = append(out.kinds[p.name], c)
			}
		}
	}
	return out
}

// requireSeqTwins fails unless every twin moved Ana's number by exactly one and told her the same events as the first.
func requireSeqTwins(t *testing.T, names []string, twins []seqTwin) {
	t.Helper()
	for i, tw := range twins {
		if tw.moved != 1 {
			t.Errorf("Ana's number moved by %d in the world of %s, want 1: the write is one write", tw.moved, names[i])
		}
		if !slices.Equal(tw.kinds["Ana"], twins[0].kinds["Ana"]) {
			t.Errorf("Ana heard %v in the world of %s and %v in the world of %s, want the same", tw.kinds["Ana"], names[i], twins[0].kinds["Ana"], names[0])
		}
	}
}

// ===== An XP award, and its undo.

// xpTargets are the characters an award of the twin goes to, by the twin's name.
var xpTargets = map[string]func(w *world) []string{
	"Toren, Caio's character": func(w *world) []string { return []string{w.toren.GetId()} },
	"Pensantus, Ana's own":    func(w *world) []string { return []string{w.pens.GetId()} },
	"both of them":            func(w *world) []string { return []string{w.toren.GetId(), w.pens.GetId()} },
}

var xpTwinNames = []string{"Toren, Caio's character", "Pensantus, Ana's own", "both of them"}

// xpWorld is a world where the master awards XP by hand (the campaign counts by enemies, which a manual award fits) to the targets;
// with undo, the award is made before the watch and the write is the master taking it back.
func xpWorld(t *testing.T, targets string, undo bool) seqTwin {
	t.Helper()
	return seqTwinWorld(t, func(w *world) func() {
		must(w.master.campaigns.SetCampaignXpMode(t.Context(), rq(&campaignsv1.SetCampaignXpModeRequest{CampaignId: w.campaign, XpMode: campaignsv1.XpMode_XP_MODE_ENEMIES, Confirm: true})))
		ids := xpTargets[targets](w)
		award := func() string {
			res := must(w.master.xp.AwardXP(t.Context(), rq(&progressionv1.AwardXPRequest{
				CampaignId: w.campaign, Mode: progressionv1.XPAwardMode_XP_AWARD_MODE_MANUAL, Amount: 100, Reason: "Teste", CharacterIds: ids, IdempotencyKey: newKey(),
			})))
			if len(res.GetAward().GetShares()) != len(ids) {
				t.Fatalf("the award = %s, want a share for each of %d characters", protojson.Format(res), len(ids))
			}
			return res.GetAward().GetId()
		}
		if !undo {
			return func() { award() }
		}
		id := award()
		return func() {
			res := must(w.master.xp.UndoLastXPAward(t.Context(), rq(&progressionv1.UndoLastXPAwardRequest{CampaignId: w.campaign, IdempotencyKey: newKey(), ExpectedAwardId: id})))
			if res.GetAward().GetId() != id {
				t.Fatalf("the undo took back %s, want %s", res.GetAward().GetId(), id)
			}
		}
	})
}

// xpTwins runs the three twins of xpWorld and checks what they share. Control: every member hears xp_changed, which says nothing
// of whom the XP went to (a hint of no content), in every twin, so the twins cannot pass because the write made no sound.
func xpTwins(t *testing.T, undo bool) {
	t.Helper()
	twins := make([]seqTwin, len(xpTwinNames))
	for i, name := range xpTwinNames {
		twins[i] = xpWorld(t, name, undo)
		for who, kinds := range twins[i].kinds {
			if !slices.Contains(kinds, "xp_changed") {
				t.Errorf("%s heard %v in the world of %s: xp_changed is told to every member, the control is gone", who, kinds, name)
			}
		}
	}
	requireSeqTwins(t, xpTwinNames, twins)
}

// TestAnXPAwardMovesAThirdPlayersNumberOnceWhoeverItGoesTo: a manual award of XP is one write that moves the number once, whether
// it went to Caio's character (twin: Toren), to Ana's own, or to both, so Ana, who is told only the hint of no content, reads the
// same number and the same events whoever the award was for.
func TestAnXPAwardMovesAThirdPlayersNumberOnceWhoeverItGoesTo(t *testing.T) {
	xpTwins(t, false)
}

// TestTheUndoOfAnXPAwardMovesAThirdPlayersNumberOnceWhoeverItWentTo: taking back the last award is one write that moves the number
// once, whoever the award had gone to, and tells Ana the same as the award did.
func TestTheUndoOfAnXPAwardMovesAThirdPlayersNumberOnceWhoeverItWentTo(t *testing.T) {
	xpTwins(t, true)
}

// ===== A revival.

// reviveWorld is a world where a dead character comes back by the master's hand with no combat running (a combat would tell the
// whole table of its own combatant, which is not what this twin is about): Toren, whose owner Caio is at the table, or the character
// of Dan, a member who watches nothing. The API allows no character the table cannot see: a reserved character cannot be marked dead
// (RESERVED), an NPC does not die and a character awaiting approval cannot either, so Dan's is the one furthest from Ana's table.
func reviveWorld(t *testing.T, dans bool) seqTwin {
	t.Helper()
	return seqTwinWorld(t, func(w *world) func() {
		must(w.master.combat.EndEncounter(t.Context(), rq(&playv1.EndEncounterRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), IdempotencyKey: newKey()})))
		who := w.toren
		if dans {
			dan := w.newPerson("Dan")
			w.join(false, dan)
			who = w.pc(dan, "Dan, o ausente", "race:human")
		}
		must(w.master.characters.MarkCharacterDead(t.Context(), rq(&charactersv1.MarkCharacterDeadRequest{CampaignId: w.campaign, CharacterId: who.GetId()})))
		return func() {
			res := must(w.master.characters.ReviveCharacter(t.Context(), rq(&charactersv1.ReviveCharacterRequest{CampaignId: w.campaign, CharacterId: who.GetId(), IdempotencyKey: newKey()})))
			if res.GetCharacter().GetDiedAt() != nil {
				t.Fatalf("the revived character = %s, want it alive", protojson.Format(res))
			}
		}
	})
}

// TestARevivalMovesAThirdPlayersNumberOnceWhoseCharacterItBringsBack: bringing a dead character back is one write that moves the
// number once, whether it is Toren (twin: the character of Dan, a member who is not at the table), so Ana reads the same number and
// hears the same events in both worlds. Control: the master hears the revival in both, Caio only for his own character, and Ana never.
func TestARevivalMovesAThirdPlayersNumberOnceWhoseCharacterItBringsBack(t *testing.T) {
	names := []string{"Toren, whose owner is at the table", "Dan's character"}
	toren, dans := reviveWorld(t, false), reviveWorld(t, true)
	requireSeqTwins(t, names, []seqTwin{toren, dans})
	if !slices.Contains(toren.kinds["Mestre"], "character_revived") || !slices.Contains(dans.kinds["Mestre"], "character_revived") {
		t.Errorf("the master heard %v and %v, want character_revived in both: the control is gone", toren.kinds["Mestre"], dans.kinds["Mestre"])
	}
	if !slices.Contains(toren.kinds["Caio"], "character_revived") || slices.Contains(dans.kinds["Caio"], "character_revived") {
		t.Errorf("Caio heard %v for his own character and %v for Dan's, want character_revived for the first alone", toren.kinds["Caio"], dans.kinds["Caio"])
	}
	for name, tw := range map[string]seqTwin{"Toren": toren, "Dan's character": dans} {
		if slices.Contains(tw.kinds["Ana"], "character_revived") {
			t.Errorf("Ana heard %v of the revival of %s: the hint is its owner's and the master's alone", tw.kinds["Ana"], name)
		}
	}
}

// ===== A change of gear.

// gearKey is the weapon every character of a gear twin carries and the write stows.
const gearKey = "equipment:dagger"

// gearSheet is a fighter that carries a dagger in hand.
func gearSheet(w *world) *charactersv1.CharacterSheet {
	sheet := w.fullSheet("class:fighter", "race:human", 3)
	sheet.GetFull().WeaponKeys = []string{gearKey}
	return sheet
}

// gearWorld is a world where the master stows a dagger of a character, with no combat running (a combat holds the gear swap of a
// player's character and tells the table of it, which is not the write of a sheet this twin is about): of an enemy no player
// reads (who "hidden"), of Toren, a player's character the table sees ("toren"), or of Pensantus, Ana's own ("pens"). The master is
// the one who stows, which is what the API allows of an NPC: only the master edits one. A story NPC and a minion have a basic
// sheet, which has no gear (the API refuses), so the enemy is the one NPC a gear change can concern.
func gearWorld(t *testing.T, who string) seqTwin {
	t.Helper()
	return seqTwinWorld(t, func(w *world) func() {
		ctx := t.Context()
		must(w.master.combat.EndEncounter(ctx, rq(&playv1.EndEncounterRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), IdempotencyKey: newKey()})))
		var id string
		if who == "hidden" {
			id = must(w.master.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
				CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, Name: "Guarda " + who, Sheet: gearSheet(w),
			}))).GetCharacter().GetId()
		} else {
			pc := w.toren
			if who == "pens" {
				pc = w.pens
			}
			must(w.master.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
				CampaignId: w.campaign, CharacterId: pc.GetId(), Revision: pc.GetRevision(), Name: pc.GetName(), Sheet: gearSheet(w),
			})))
			id = pc.GetId()
		}
		return func() {
			res := must(w.master.characters.SetEquipped(ctx, rq(&charactersv1.SetEquippedRequest{CampaignId: w.campaign, CharacterId: id, Key: gearKey, Equipped: false})))
			if !slices.Contains(res.GetCarriedKeys(), gearKey) {
				t.Fatalf("the gear = %s, want the dagger stowed (the write was made)", protojson.Format(res))
			}
		}
	})
}

// TestAGearChangeMovesAThirdPlayersNumberOnceWhoseGearItIs: stowing a weapon is one write that moves the number once, whether the
// sheet is a hidden enemy's, a player's character the table sees (Toren) or Ana's own, so Ana reads the same number and hears the
// same events whatever the gear belonged to. The gear is the master's write in every twin, and outside a combat no event tells of
// it: the control that the write was made is the dagger in the answer, which the twin checks.
func TestAGearChangeMovesAThirdPlayersNumberOnceWhoseGearItIs(t *testing.T) {
	names := []string{"a hidden enemy", "Toren, a player's character", "Pensantus, Ana's own"}
	twins := []seqTwin{gearWorld(t, "hidden"), gearWorld(t, "toren"), gearWorld(t, "pens")}
	requireSeqTwins(t, names, twins)
}

// ===== The choices a character completes.

// choicesWorld is a world where a choice is completed: by Caio for Toren ("toren"), or by the master for an enemy no player reads
// ("hidden"). A player cannot complete the choices of a character that is not hers (the API answers "not found", as for a character
// she cannot see), so the one character no player may ask for is the master's alone to complete: that is the hidden twin of the
// choice a player completes for her own. The choice is the human's extra language, which the master takes out of the sheet after the
// character is made, through a client with no interceptor: the tests' own client makes the open choices of every sheet it saves
// (platform/choicetest), as the editor does, so a sheet written before the choices existed, which is what has one open, is saved raw.
func choicesWorld(t *testing.T, who string) seqTwin {
	t.Helper()
	return seqTwinWorld(t, func(w *world) func() {
		ctx := t.Context()
		raw := charactersv1connect.NewCharacterServiceClient(w.master.client, w.server.URL)
		by, c := w.caio, w.toren
		if who == "hidden" {
			by = w.master
			c = must(raw.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
				CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, Name: "Guarda " + who, Sheet: w.fullSheet("class:wizard", "race:human", 1),
			}))).GetCharacter()
		}
		open := proto.CloneOf(c.GetSheet())
		open.GetFull().FeatureChoiceKeys = nil
		c = must(raw.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
			CampaignId: w.campaign, CharacterId: c.GetId(), Revision: c.GetRevision(), Name: c.GetName(), Sheet: open,
		}))).GetCharacter()
		return func() {
			res := must(by.characters.CompleteCharacterChoices(ctx, rq(&charactersv1.CompleteCharacterChoicesRequest{
				CampaignId: w.campaign, CharacterId: c.GetId(), ExpectedRevision: c.GetRevision(),
				Picks: []*charactersv1.ChoicePick{{ChoiceKey: "race:human#language", OptionKeys: []string{"language:abyssal"}}},
			})))
			if got := res.GetCharacter().GetRevision(); got != c.GetRevision()+1 {
				t.Fatalf("the revision = %d, want %d: the choice was made", got, c.GetRevision()+1)
			}
		}
	})
}

// TestCompletingChoicesMovesAThirdPlayersNumberOnceWhoseCharacterItIs: completing the choices a sheet left open is one write that
// moves the number once, whether the character is Toren (completed by Caio) or an enemy no player may
// read (completed by the master, the only one who may), so Ana reads the same number and hears the same events. Control: every member
// hears xp_changed, the hint of no content a completed choice always tells, in every twin.
func TestCompletingChoicesMovesAThirdPlayersNumberOnceWhoseCharacterItIs(t *testing.T) {
	names := []string{"Toren, completed by Caio", "a hidden enemy, completed by the master"}
	twins := []seqTwin{choicesWorld(t, "toren"), choicesWorld(t, "hidden")}
	for i, tw := range twins {
		for who, kinds := range tw.kinds {
			if !slices.Contains(kinds, "xp_changed") {
				t.Errorf("%s heard %v in the world of %s: xp_changed is told to every member, the control is gone", who, kinds, names[i])
			}
		}
	}
	requireSeqTwins(t, names, twins)
}
