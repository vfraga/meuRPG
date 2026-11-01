package progression

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	progressionv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
)

// AwardXP and UndoLastXPAward: every refusal as the app reads it (code,
// message and XPBlocked detail) and which comes first, the rows and session
// events of each kind of award and of its undo, the retries of a key, what a
// failure of another module leaves, and the races inside the transaction.

const (
	modeManualAward  = progressionv1.XPAwardMode_XP_AWARD_MODE_MANUAL
	modeGoldAward    = progressionv1.XPAwardMode_XP_AWARD_MODE_GOLD
	modeEnemiesAward = progressionv1.XPAwardMode_XP_AWARD_MODE_ENEMIES
	// internalError is what the client reads of a failure it cannot fix.
	internalError = "internal: internal error"
)

// TestAwardXPRefusesABadRequestExactly: each check of the request before the
// database, in the order they run: the caller first, then the mode's fields,
// the treasures, the reason, the key and the characters.
func TestAwardXPRefusesABadRequestExactly(t *testing.T) {
	t.Parallel()
	rig := newFaultRig(t, enemies, 1)
	pc := rig.pcs[0].GetId()
	manual := func(r *progressionv1.AwardXPRequest) {
		r.Mode, r.Amount, r.CharacterIds = modeManualAward, 10, []string{pc}
	}
	many := func(n int, id func(i int) string) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = id(i)
		}
		return out
	}
	for _, c := range []struct {
		name string
		edit func(r *progressionv1.AwardXPRequest)
		want string
	}{
		{"enemies: an encounter that is not a UUID", func(r *progressionv1.AwardXPRequest) {
			r.Mode, r.EncounterId, r.Amount = modeEnemiesAward, "x", 0
		}, "not_found: encounter not found"},
		{"enemies: no encounter comes before the amount", func(r *progressionv1.AwardXPRequest) {
			r.Mode, r.EncounterId = modeEnemiesAward, ""
		}, "not_found: encounter not found"},
		{"enemies: an amount", func(r *progressionv1.AwardXPRequest) {
			r.Mode, r.EncounterId = modeEnemiesAward, newKey()
		}, "invalid_argument: amount and gold must be empty for an ENEMIES award"},
		{"enemies: gold", func(r *progressionv1.AwardXPRequest) {
			r.Mode, r.EncounterId, r.Amount, r.Gold = modeEnemiesAward, newKey(), 0, 5
		}, "invalid_argument: amount and gold must be empty for an ENEMIES award"},
		{"enemies: treasures", func(r *progressionv1.AwardXPRequest) {
			r.Mode, r.EncounterId, r.Amount, r.TreasurePointIds = modeEnemiesAward, newKey(), 0, []string{newKey()}
		}, "invalid_argument: treasure_point_ids must be empty unless the mode is GOLD"},
		{"manual: treasures come before the reason", func(r *progressionv1.AwardXPRequest) {
			r.TreasurePointIds, r.Reason = []string{"x"}, ""
		}, "invalid_argument: treasure_point_ids must be empty unless the mode is GOLD"},
		{
			"manual: an encounter", func(r *progressionv1.AwardXPRequest) { r.EncounterId = newKey() },
			"invalid_argument: encounter_id and gold must be empty for a MANUAL award",
		},
		{
			"manual: gold", func(r *progressionv1.AwardXPRequest) { r.Gold = 1 },
			"invalid_argument: encounter_id and gold must be empty for a MANUAL award",
		},
		{
			"manual: gold comes before the amount", func(r *progressionv1.AwardXPRequest) { r.Gold, r.Amount = 1, 0 },
			"invalid_argument: encounter_id and gold must be empty for a MANUAL award",
		},
		{
			"manual: no amount", func(r *progressionv1.AwardXPRequest) { r.Amount = 0 },
			"invalid_argument: amount must be 1 to 1000000",
		},
		{
			"manual: a negative amount", func(r *progressionv1.AwardXPRequest) { r.Amount = -1 },
			"invalid_argument: amount must be 1 to 1000000",
		},
		{
			"manual: one more than a sheet holds", func(r *progressionv1.AwardXPRequest) { r.Amount = 1_000_001 },
			"invalid_argument: amount must be 1 to 1000000",
		},
		{"gold: an encounter", func(r *progressionv1.AwardXPRequest) {
			r.Mode, r.Amount, r.EncounterId, r.Gold = modeGoldAward, 0, newKey(), 5
		}, "invalid_argument: encounter_id and amount must be empty for a GOLD award"},
		{
			"gold: an amount", func(r *progressionv1.AwardXPRequest) { r.Mode, r.Gold = modeGoldAward, 5 },
			"invalid_argument: encounter_id and amount must be empty for a GOLD award",
		},
		{"gold: treasures and gold", func(r *progressionv1.AwardXPRequest) {
			r.Mode, r.Amount, r.Gold, r.TreasurePointIds = modeGoldAward, 0, 5, []string{newKey()}
		}, "invalid_argument: gold must be empty when treasure_point_ids is set"},
		{"gold: 101 treasures", func(r *progressionv1.AwardXPRequest) {
			r.Mode, r.Amount, r.TreasurePointIds = modeGoldAward, 0, many(101, func(int) string { return "x" })
		}, "invalid_argument: request over its size limit [request refused field=treasure_point_ids reason=REQUEST_REFUSED_REASON_LIST_TOO_LONG]"}, // the request size limit of the field, the same 100, refuses it before the handler
		{"gold: a treasure that is not a UUID", func(r *progressionv1.AwardXPRequest) {
			r.Mode, r.Amount, r.TreasurePointIds = modeGoldAward, 0, []string{newKey(), "x"}
		}, "invalid_argument: treasure_point_ids must be UUIDs"},
		{"gold: a repeated treasure, in two spellings", func(r *progressionv1.AwardXPRequest) {
			id := newKey()
			r.Mode, r.Amount, r.TreasurePointIds = modeGoldAward, 0, []string{id, strings.ToUpper(id)}
		}, "invalid_argument: treasure_point_ids must not repeat a treasure"},
		{"gold: the treasures come before the reason", func(r *progressionv1.AwardXPRequest) {
			r.Mode, r.Amount, r.TreasurePointIds, r.Reason = modeGoldAward, 0, []string{"x"}, ""
		}, "invalid_argument: treasure_point_ids must be UUIDs"},
		{
			"gold: none", func(r *progressionv1.AwardXPRequest) { r.Mode, r.Amount = modeGoldAward, 0 },
			"invalid_argument: gold must be 1 to 1000000",
		},
		{
			"gold: negative", func(r *progressionv1.AwardXPRequest) { r.Mode, r.Amount, r.Gold = modeGoldAward, 0, -3 },
			"invalid_argument: gold must be 1 to 1000000",
		},
		{"gold: one more than a sheet holds", func(r *progressionv1.AwardXPRequest) {
			r.Mode, r.Amount, r.Gold = modeGoldAward, 0, 1_000_001
		}, "invalid_argument: gold must be 1 to 1000000"},
		{
			"no mode", func(r *progressionv1.AwardXPRequest) { r.Mode = progressionv1.XPAwardMode_XP_AWARD_MODE_UNSPECIFIED },
			"invalid_argument: mode must be ENEMIES, GOLD or MANUAL",
		},
		{
			"the milestone mode", func(r *progressionv1.AwardXPRequest) { r.Mode = progressionv1.XPAwardMode_XP_AWARD_MODE_MILESTONE },
			"invalid_argument: mode must be ENEMIES, GOLD or MANUAL",
		},
		{
			"a mode the server does not know", func(r *progressionv1.AwardXPRequest) { r.Mode = 99 },
			"invalid_argument: mode must be ENEMIES, GOLD or MANUAL",
		},
		{"the mode comes before the key and the characters", func(r *progressionv1.AwardXPRequest) {
			r.Mode, r.IdempotencyKey, r.CharacterIds = 0, "1", nil
		}, "invalid_argument: mode must be ENEMIES, GOLD or MANUAL"},
		{
			"no reason", func(r *progressionv1.AwardXPRequest) { r.Reason = "" },
			"invalid_argument: reason must not be empty",
		},
		{
			"a blank reason", func(r *progressionv1.AwardXPRequest) { r.Reason = "   " },
			"invalid_argument: reason must not be empty",
		},
		{
			"a reason of 121 characters", func(r *progressionv1.AwardXPRequest) { r.Reason = strings.Repeat("á", 121) },
			"invalid_argument: reason must be at most 120 characters, got 121",
		},
		{
			"a reason with a line break", func(r *progressionv1.AwardXPRequest) { r.Reason = "a\nb" },
			"invalid_argument: reason must not contain the character U+000A",
		},
		{
			"the reason comes before the key", func(r *progressionv1.AwardXPRequest) { r.Reason, r.IdempotencyKey = "", "1" },
			"invalid_argument: reason must not be empty",
		},
		{
			"a key that is not a UUID", func(r *progressionv1.AwardXPRequest) { r.IdempotencyKey = "1" },
			"invalid_argument: idempotency_key must be a UUID",
		},
		{
			"the key comes before the characters", func(r *progressionv1.AwardXPRequest) { r.IdempotencyKey, r.CharacterIds = "", nil },
			"invalid_argument: idempotency_key must be a UUID",
		},
		{
			"no characters", func(r *progressionv1.AwardXPRequest) { r.CharacterIds = nil },
			"invalid_argument: character_ids must have 1 to 40 characters",
		},
		{"41 characters, before they are read", func(r *progressionv1.AwardXPRequest) {
			r.CharacterIds = many(41, func(int) string { return "x" })
		}, "invalid_argument: character_ids must have 1 to 40 characters"},
		{
			"a character that is not a UUID", func(r *progressionv1.AwardXPRequest) { r.CharacterIds = []string{pc, "x"} },
			"invalid_argument: character_ids must be UUIDs",
		},
		{
			"a repeated character, in two spellings", func(r *progressionv1.AwardXPRequest) { r.CharacterIds = []string{pc, strings.ToUpper(pc)} },
			"invalid_argument: character_ids must not repeat a character",
		},
	} {
		_, err := rig.master.award(rig.campaign, func(r *progressionv1.AwardXPRequest) {
			manual(r)
			c.edit(r)
		})
		rig.wantAwardRefusal(t, c.name, err, c.want)
	}

	// Who may call comes before any check of the request.
	bad := func(r *progressionv1.AwardXPRequest) { r.Mode, r.IdempotencyKey = 0, "1" }
	_, err := rig.players[0].award(rig.campaign, bad)
	rig.wantAwardRefusal(t, "a player", err, "permission_denied: only the campaign's master can do this")
	_, err = rig.h.newUser("De fora").award(rig.campaign, bad)
	rig.wantAwardRefusal(t, "a non-member", err, "not_found: campaign not found")
	_, err = rig.master.award(newKey(), bad)
	rig.wantAwardRefusal(t, "a campaign that does not exist", err, "not_found: campaign not found")

	// The bounds themselves are given.
	if res, err := rig.master.award(rig.campaign, func(r *progressionv1.AwardXPRequest) {
		manual(r)
		r.Amount, r.Reason = 1_000_000, strings.Repeat("á", 120)
	}); err != nil || res.GetXpEach() != 1_000_000 || res.GetLostXp() != 0 {
		t.Errorf("AwardXP(1000000, a reason of 120) = %v, %v; want given", res, err)
	}
	if n := len(rig.master.history(t, rig.campaign)); n != 1 {
		t.Errorf("history has %d awards, want only the one given: a refusal writes nothing", n)
	}
}

// TestUndoLastXPAwardRefusesExactly: the caller, the key, the expected award,
// then what the campaign has; a retry of the key answers the award it undid,
// whatever happened since.
func TestUndoLastXPAwardRefusesExactly(t *testing.T) {
	t.Parallel()
	rig := newFaultRig(t, enemies, 1)
	pc := rig.pcs[0].GetId()

	_, err := rig.players[0].xp.UndoLastXPAward(t.Context(), connect.NewRequest(&progressionv1.UndoLastXPAwardRequest{CampaignId: rig.campaign, IdempotencyKey: "1"}))
	rig.wantAwardRefusal(t, "a player with a bad key", err, "permission_denied: only the campaign's master can do this")
	_, err = rig.undoWith("1", "x")
	rig.wantAwardRefusal(t, "the key comes before the expected award", err, "invalid_argument: idempotency_key must be a UUID")
	_, err = rig.undoWith(newKey(), "x")
	rig.wantAwardRefusal(t, "an expected award that is not a UUID", err, "invalid_argument: expected_award_id must be a UUID")
	_, err = rig.undoWith(newKey(), newKey())
	rig.wantAwardRefusal(t, "nothing to undo comes before the expected award", err,
		"failed_precondition: there is no award to undo [reason=XP_BLOCKED_REASON_NOTHING_TO_UNDO character= xp_mode=XP_MODE_UNSPECIFIED treasure=]")

	first := rig.master.manual(t, rig.campaign, 30, pc)
	rig.label(first.GetAward().GetId(), "first")
	second := rig.master.manual(t, rig.campaign, 20, pc)
	rig.label(second.GetAward().GetId(), "second")
	_, err = rig.undoWith(newKey(), first.GetAward().GetId())
	rig.wantAwardRefusal(t, "an expected award that is not the last", err, "aborted: another award is the last now")
	// The expected one, in capitals, is the same award.
	key := newKey()
	undone, err := rig.undoWith(key, strings.ToUpper(second.GetAward().GetId()))
	if err != nil || undone.GetId() != second.GetAward().GetId() {
		t.Fatalf("UndoLastXPAward(expected the last) = %v, %v; want the second undone", undone, err)
	}
	// A retry of the key, after another award, answers the award it undid and
	// takes nothing more; the expected award is not read again.
	third := rig.master.manual(t, rig.campaign, 5, pc)
	rig.label(third.GetAward().GetId(), "third")
	again, err := rig.undoWith(strings.ToUpper(key), first.GetAward().GetId())
	if err != nil || again.GetId() != second.GetAward().GetId() || !again.GetUndone() {
		t.Errorf("UndoLastXPAward(retry) = %v, %v; want the second award again", again, err)
	}
	if got := rig.master.xpOf(t, rig.pcs[0]); got != 35 {
		t.Errorf("sheet XP = %d, want 35: the retry took nothing back", got)
	}
	wantLines(t, "rows", []string{
		rig.awardRow(t, first.GetAward().GetId()), rig.awardRow(t, second.GetAward().GetId()), rig.awardRow(t, third.GetAward().GetId()),
	}, []string{
		`award <first> mode=manual reason="Pela ajuda ao ferreiro" given_by=<master> encounter= gold=null total=30 milestone= again=false hashed=true undone=false undone_by= undo_key=false; share <pensantus> xp=30 level_at_mark=null`,
		`award <second> mode=manual reason="Pela ajuda ao ferreiro" given_by=<master> encounter= gold=null total=20 milestone= again=false hashed=true undone=true undone_by=<master> undo_key=true; share <pensantus> xp=20 level_at_mark=null`,
		`award <third> mode=manual reason="Pela ajuda ao ferreiro" given_by=<master> encounter= gold=null total=5 milestone= again=false hashed=true undone=false undone_by= undo_key=false; share <pensantus> xp=5 level_at_mark=null`,
	})
}

// TestGiveRefusalsAndTheirOrder: the refusals the database decides in an
// enemies campaign, with their detail, and which one wins when a request has
// two faults. The refusals of the gold and milestones campaigns are in
// gold_test.go and milestones_test.go.
func TestGiveRefusalsAndTheirOrder(t *testing.T) {
	t.Parallel()
	rig := newFaultRig(t, enemies, 2)
	pens, toren := rig.pcs[0].GetId(), rig.pcs[1].GetId()
	npc := rig.master.minion(t, rig.campaign, "Goblin", 1)
	rig.label(npc.GetId(), "goblin")
	stranger := newKey()
	rig.label(stranger, "stranger")

	_, err := rig.master.award(rig.campaign, func(r *progressionv1.AwardXPRequest) {
		r.Mode, r.Gold, r.CharacterIds = modeGoldAward, 10, []string{stranger}
	})
	rig.wantAwardRefusal(t, "the campaign's mode comes before the party", err,
		"failed_precondition: this campaign's XP mode does not take this [reason=XP_BLOCKED_REASON_MODE_NOT_ALLOWED character= xp_mode=XP_MODE_ENEMIES treasure=]")
	_, err = rig.master.award(rig.campaign, func(r *progressionv1.AwardXPRequest) {
		r.Mode, r.Amount, r.CharacterIds = modeManualAward, 10, []string{pens, stranger, npc.GetId()}
	})
	rig.wantAwardRefusal(t, "the first character not in the party", err,
		"failed_precondition: only the campaign's living player characters can get XP [reason=XP_BLOCKED_REASON_CHARACTER_NOT_ELIGIBLE character=<stranger> xp_mode=XP_MODE_UNSPECIFIED treasure=]")
	_, err = rig.master.award(rig.campaign, func(r *progressionv1.AwardXPRequest) {
		r.Mode, r.Amount, r.CharacterIds = modeManualAward, 1, []string{pens, toren}
	})
	rig.wantAwardRefusal(t, "less than 1 XP each", err,
		"failed_precondition: there is no XP to give each character [reason=XP_BLOCKED_REASON_NOTHING_TO_GIVE character= xp_mode=XP_MODE_UNSPECIFIED treasure=]")

	open := rig.fight(t, npc, 2, 1, false)
	rig.label(open.GetId(), "open")
	_, err = rig.enemies(open.GetId(), pens, stranger)
	rig.wantAwardRefusal(t, "the party comes before the combat", err,
		"failed_precondition: only the campaign's living player characters can get XP [reason=XP_BLOCKED_REASON_CHARACTER_NOT_ELIGIBLE character=<stranger> xp_mode=XP_MODE_UNSPECIFIED treasure=]")
	_, err = rig.enemies(open.GetId(), pens)
	rig.wantAwardRefusal(t, "a combat not ended", err,
		"failed_precondition: the combat has not ended [reason=XP_BLOCKED_REASON_ENCOUNTER_NOT_ENDED character= xp_mode=XP_MODE_UNSPECIFIED treasure=]")
	_, err = rig.enemies(newKey(), pens)
	rig.wantAwardRefusal(t, "a combat that does not exist", err, "not_found: encounter not found")
	if _, err := rig.master.combat.EndEncounter(t.Context(), connect.NewRequest(&playv1.EndEncounterRequest{CampaignId: rig.campaign, EncounterId: open.GetId(), IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("EndEncounter() error = %v", err)
	}
	_, err = rig.enemies(open.GetId(), pens, toren)
	rig.wantAwardRefusal(t, "1 XP of defeated among two", err,
		"failed_precondition: there is no XP to give each character [reason=XP_BLOCKED_REASON_NOTHING_TO_GIVE character= xp_mode=XP_MODE_UNSPECIFIED treasure=]")
	res, err := rig.enemies(open.GetId(), pens)
	if err != nil || res.GetXpEach() != 1 {
		t.Fatalf("AwardXP(enemies, 1 XP to one) = %v, %v; want 1 each", res, err)
	}
	_, err = rig.enemies(open.GetId(), toren)
	rig.wantAwardRefusal(t, "the combat's XP given already", err,
		"failed_precondition: the combat's XP was already given [reason=XP_BLOCKED_REASON_ALREADY_AWARDED character= xp_mode=XP_MODE_UNSPECIFIED treasure=]")
	none := rig.fight(t, npc, 1, 0, true)
	_, err = rig.enemies(none.GetId(), pens)
	rig.wantAwardRefusal(t, "a combat with no one defeated", err,
		"failed_precondition: there is no XP to give each character [reason=XP_BLOCKED_REASON_NOTHING_TO_GIVE character= xp_mode=XP_MODE_UNSPECIFIED treasure=]")
}

// TestEachAwardKindAndItsUndo: with a session open, each kind of award writes
// its rows and its session event; a retry of its key changes nothing; its undo
// takes it back and writes its own event. Then the same with no session open:
// the rows only.
func TestEachAwardKindAndItsUndo(t *testing.T) {
	t.Parallel()
	t.Run("enemies and manual", func(t *testing.T) {
		t.Parallel()
		rig := newFaultRig(t, enemies, 3)
		pens, toren, brisa := rig.pcs[0].GetId(), rig.pcs[1].GetId(), rig.pcs[2].GetId()
		goblin := rig.master.minion(t, rig.campaign, "Goblin", 50)
		enc := rig.fight(t, goblin, 2, 2, true) // opens the session
		rig.label(enc.GetId(), "combat")

		key := newKey()
		send := func() (*progressionv1.AwardXPResponse, error) {
			return rig.awardWith(key, func(r *progressionv1.AwardXPRequest) {
				r.Mode, r.EncounterId, r.CharacterIds = modeEnemiesAward, enc.GetId(), []string{toren, pens, brisa}
			})
		}
		res, err := send()
		if err != nil {
			t.Fatalf("AwardXP(enemies) error = %v", err)
		}
		rig.label(res.GetAward().GetId(), "enemies")
		if res.GetXpEach() != 33 || res.GetLostXp() != 1 || res.GetAward().GetTotalXp() != 100 {
			t.Errorf("response = each %d, lost %d, total %d; want 33, 1, 100", res.GetXpEach(), res.GetLostXp(), res.GetAward().GetTotalXp())
		}
		if again, err := send(); err != nil || again.GetAward().GetId() != res.GetAward().GetId() || again.GetXpEach() != 33 || again.GetLostXp() != 1 {
			t.Errorf("AwardXP(retry) = %v, %v; want the same answer", again, err)
		}
		manual, err := rig.master.award(rig.campaign, func(r *progressionv1.AwardXPRequest) {
			r.Mode, r.Amount, r.Reason, r.CharacterIds = modeManualAward, 7, "Boa ideia", []string{brisa}
		})
		if err != nil {
			t.Fatalf("AwardXP(manual) error = %v", err)
		}
		rig.label(manual.GetAward().GetId(), "manual")
		wantLines(t, "rows", []string{rig.awardRow(t, res.GetAward().GetId()), rig.awardRow(t, manual.GetAward().GetId())}, []string{
			`award <enemies> mode=enemies reason="Pela ajuda ao ferreiro" given_by=<master> encounter=<combat> gold=null total=100 milestone= again=false hashed=true undone=false undone_by= undo_key=false; share <brisa> xp=33 level_at_mark=null; share <pensantus> xp=33 level_at_mark=null; share <toren> xp=33 level_at_mark=null`,
			`award <manual> mode=manual reason="Boa ideia" given_by=<master> encounter= gold=null total=7 milestone= again=false hashed=true undone=false undone_by= undo_key=false; share <brisa> xp=7 level_at_mark=null`,
		})
		// The award's event keeps the characters in the order sent.
		wantLines(t, "events", rig.takeEvents(t), []string{
			`xp_awarded by <master> {"award_id":"<enemies>","character_ids":["<toren>","<pensantus>","<brisa>"],"encounter_id":"<combat>","lost_xp":1,"mode":"enemies","total_xp":100,"xp_each":33}`,
			`xp_awarded by <master> {"award_id":"<manual>","character_ids":["<brisa>"],"mode":"manual","total_xp":7,"xp_each":7}`,
		})

		for _, undoing := range []string{"manual", "enemies"} {
			undoKey := newKey()
			undone, err := rig.undoWith(undoKey, "")
			if err != nil {
				t.Fatalf("UndoLastXPAward(%s) error = %v", undoing, err)
			}
			if got := rig.redact(undone.GetId()); got != "<"+undoing+">" {
				t.Errorf("undone %s, want <%s>", got, undoing)
			}
			if _, err := rig.undoWith(undoKey, ""); err != nil {
				t.Fatalf("UndoLastXPAward(%s, retry) error = %v", undoing, err)
			}
		}
		wantLines(t, "rows after the undos", []string{rig.awardRow(t, res.GetAward().GetId()), rig.awardRow(t, manual.GetAward().GetId())}, []string{
			`award <enemies> mode=enemies reason="Pela ajuda ao ferreiro" given_by=<master> encounter=<combat> gold=null total=100 milestone= again=false hashed=true undone=true undone_by=<master> undo_key=true; share <brisa> xp=33 level_at_mark=null; share <pensantus> xp=33 level_at_mark=null; share <toren> xp=33 level_at_mark=null`,
			`award <manual> mode=manual reason="Boa ideia" given_by=<master> encounter= gold=null total=7 milestone= again=false hashed=true undone=true undone_by=<master> undo_key=true; share <brisa> xp=7 level_at_mark=null`,
		})
		// The undo's event lists the characters in the order they were created.
		wantLines(t, "events of the undos", rig.takeEvents(t), []string{
			`xp_award_undone by <master> {"award_id":"<manual>","character_ids":["<brisa>"],"mode":"manual","total_xp":7}`,
			`xp_award_undone by <master> {"award_id":"<enemies>","character_ids":["<pensantus>","<toren>","<brisa>"],"encounter_id":"<combat>","mode":"enemies","total_xp":100}`,
		})
		for i, pc := range rig.pcs {
			if got := rig.master.xpOf(t, pc); got != 0 {
				t.Errorf("sheet %d XP = %d after the undos, want 0", i, got)
			}
		}
		// Undone, the combat's XP can be given again.
		if _, err := rig.enemies(enc.GetId(), pens); err != nil {
			t.Errorf("AwardXP(enemies after its undo) error = %v", err)
		}
	})
	t.Run("gold", func(t *testing.T) {
		t.Parallel()
		rig := newFaultRig(t, gold, 2)
		pens, toren := rig.pcs[0].GetId(), rig.pcs[1].GetId()
		first, second := rig.newMap(), rig.newMap()
		rig.label(first, "map1")
		rig.label(second, "map2")
		session := rig.startSession(t)
		bolsa := rig.treasure(t, first, "Bolsa", 120, &session, pens)
		rig.label(bolsa, "bolsa")
		idolo := rig.treasure(t, second, "Ídolo", 81, &session, toren)
		rig.label(idolo, "idolo")
		calice := rig.treasure(t, first, "Cálice", 0, &session, toren)
		rig.label(calice, "calice")

		typed, err := rig.master.award(rig.campaign, func(r *progressionv1.AwardXPRequest) {
			r.Mode, r.Gold, r.CharacterIds = modeGoldAward, 25, []string{pens, toren}
		})
		if err != nil {
			t.Fatalf("AwardXP(typed gold) error = %v", err)
		}
		rig.label(typed.GetAward().GetId(), "typed")
		if typed.GetXpEach() != 12 || typed.GetLostXp() != 1 || len(typed.GetTreasures()) != 0 {
			t.Errorf("typed gold = %v; want 12 each, 1 lost, no treasures", typed)
		}
		key := newKey()
		town, err := rig.backToTown(key, []string{toren, pens}, idolo, calice, bolsa)
		if err != nil {
			t.Fatalf("AwardXP(voltar à cidade) error = %v", err)
		}
		rig.label(town.GetAward().GetId(), "town")
		if town.GetXpEach() != 100 || town.GetLostXp() != 1 || town.GetAward().GetGold() != 201 || len(town.GetTreasures()) != 3 {
			t.Errorf("voltar à cidade = %v; want 100 each, 1 lost, 201 PO, 3 treasures", town)
		}
		if _, err := rig.backToTown(key, []string{pens, toren}, bolsa, idolo, calice); err != nil {
			t.Errorf("AwardXP(voltar à cidade, retry in another order) error = %v", err)
		}
		wantLines(t, "rows", []string{rig.awardRow(t, typed.GetAward().GetId()), rig.awardRow(t, town.GetAward().GetId())}, []string{
			`award <typed> mode=gold reason="Pela ajuda ao ferreiro" given_by=<master> encounter= gold=25 total=25 milestone= again=false hashed=true undone=false undone_by= undo_key=false; share <pensantus> xp=12 level_at_mark=null; share <toren> xp=12 level_at_mark=null`,
			`award <town> mode=gold reason="Voltar à cidade" given_by=<master> encounter= gold=201 total=201 milestone= again=false hashed=true undone=false undone_by= undo_key=false; share <pensantus> xp=100 level_at_mark=null; share <toren> xp=100 level_at_mark=null; treasure <bolsa> po=120; treasure <calice> po=0; treasure <idolo> po=81`,
		})
		// The award's event keeps the characters and the treasures in the order sent.
		wantLines(t, "events", rig.takeEvents(t), []string{
			`xp_awarded by <master> {"award_id":"<typed>","character_ids":["<pensantus>","<toren>"],"gold":25,"lost_xp":1,"mode":"gold","total_xp":25,"xp_each":12}`,
			`xp_awarded by <master> {"award_id":"<town>","character_ids":["<toren>","<pensantus>"],"gold":201,"lost_xp":1,"mode":"gold","total_xp":201,"treasures":[{"point_id":"<idolo>","value_po":81},{"point_id":"<calice>","value_po":0},{"point_id":"<bolsa>","value_po":120}],"xp_each":100}`,
		})

		for range 2 {
			if _, err := rig.undoWith(newKey(), ""); err != nil {
				t.Fatalf("UndoLastXPAward() error = %v", err)
			}
		}
		wantLines(t, "rows after the undos", []string{rig.awardRow(t, typed.GetAward().GetId()), rig.awardRow(t, town.GetAward().GetId())}, []string{
			`award <typed> mode=gold reason="Pela ajuda ao ferreiro" given_by=<master> encounter= gold=25 total=25 milestone= again=false hashed=true undone=true undone_by=<master> undo_key=true; share <pensantus> xp=12 level_at_mark=null; share <toren> xp=12 level_at_mark=null`,
			`award <town> mode=gold reason="Voltar à cidade" given_by=<master> encounter= gold=201 total=201 milestone= again=false hashed=true undone=true undone_by=<master> undo_key=true; share <pensantus> xp=100 level_at_mark=null; share <toren> xp=100 level_at_mark=null; treasure <bolsa> po=120; treasure <calice> po=0; treasure <idolo> po=81`,
		})
		// The undo's event has no xp_each, lost_xp or gold.
		wantLines(t, "events of the undos", rig.takeEvents(t), []string{
			`xp_award_undone by <master> {"award_id":"<town>","character_ids":["<pensantus>","<toren>"],"mode":"gold","total_xp":201,"treasures":[{"point_id":"<bolsa>","value_po":120},{"point_id":"<calice>","value_po":0},{"point_id":"<idolo>","value_po":81}]}`,
			`xp_award_undone by <master> {"award_id":"<typed>","character_ids":["<pensantus>","<toren>"],"mode":"gold","total_xp":25}`,
		})
		if left := rig.toConvert(t); len(left) != 3 {
			t.Errorf("%d treasures to convert after the undo, want the 3 back", len(left))
		}
	})
	t.Run("milestones", func(t *testing.T) {
		t.Parallel()
		rig := newFaultRig(t, milestones, 2)
		pens, toren := rig.pcs[0].GetId(), rig.pcs[1].GetId()
		rig.startSession(t)
		adHoc, err := rig.master.xp.MarkMilestone(t.Context(), connect.NewRequest(&progressionv1.MarkMilestoneRequest{
			CampaignId: rig.campaign, Reason: "Derrotaram o chefe", CharacterIds: []string{toren, pens}, IdempotencyKey: newKey(),
		}))
		if err != nil {
			t.Fatalf("MarkMilestone() error = %v", err)
		}
		rig.label(adHoc.Msg.GetAward().GetId(), "adhoc")
		planned := rig.master.plan(t, rig.campaign, "A ponte")
		rig.label(planned.GetId(), "ponte")
		reached := rig.master.reach(t, rig.campaign, planned.GetId(), pens)
		rig.label(reached.GetAward().GetId(), "reached")
		given, err := rig.master.giveMilestone(planned.GetId(), rig.campaign, toren)
		if err != nil {
			t.Fatalf("GiveMilestoneTo() error = %v", err)
		}
		rig.label(given.GetAward().GetId(), "given")
		ids := []string{adHoc.Msg.GetAward().GetId(), reached.GetAward().GetId(), given.GetAward().GetId()}
		rows := func() []string {
			var out []string
			for _, id := range ids {
				out = append(out, rig.awardRow(t, id))
			}
			return out
		}
		wantLines(t, "rows", rows(), []string{
			`award <adhoc> mode=milestone reason="Derrotaram o chefe" given_by=<master> encounter= gold=null total=0 milestone= again=false hashed=true undone=false undone_by= undo_key=false; share <pensantus> xp=0 level_at_mark=1; share <toren> xp=0 level_at_mark=1`,
			`award <reached> mode=milestone reason="A ponte" given_by=<master> encounter= gold=null total=0 milestone=<ponte> again=false hashed=true undone=false undone_by= undo_key=false; share <pensantus> xp=0 level_at_mark=1`,
			`award <given> mode=milestone reason="A ponte" given_by=<master> encounter= gold=null total=0 milestone=<ponte> again=true hashed=true undone=false undone_by= undo_key=false; share <toren> xp=0 level_at_mark=1`,
		})
		wantLines(t, "events", rig.takeEvents(t), []string{
			`milestone_marked by <master> {"award_id":"<adhoc>","character_ids":["<toren>","<pensantus>"],"mode":"milestone"}`,
			`milestone_marked by <master> {"award_id":"<reached>","character_ids":["<pensantus>"],"milestone_id":"<ponte>","mode":"milestone"}`,
			`milestone_marked by <master> {"award_id":"<given>","character_ids":["<toren>"],"milestone_id":"<ponte>","mode":"milestone"}`,
		})
		for range 3 {
			if _, err := rig.undoWith(newKey(), ""); err != nil {
				t.Fatalf("UndoLastXPAward() error = %v", err)
			}
		}
		wantLines(t, "rows after the undos", rows(), []string{
			`award <adhoc> mode=milestone reason="Derrotaram o chefe" given_by=<master> encounter= gold=null total=0 milestone= again=false hashed=true undone=true undone_by=<master> undo_key=true; share <pensantus> xp=0 level_at_mark=1; share <toren> xp=0 level_at_mark=1`,
			`award <reached> mode=milestone reason="A ponte" given_by=<master> encounter= gold=null total=0 milestone=<ponte> again=false hashed=true undone=true undone_by=<master> undo_key=true; share <pensantus> xp=0 level_at_mark=1`,
			`award <given> mode=milestone reason="A ponte" given_by=<master> encounter= gold=null total=0 milestone=<ponte> again=true hashed=true undone=true undone_by=<master> undo_key=true; share <toren> xp=0 level_at_mark=1`,
		})
		wantLines(t, "events of the undos", rig.takeEvents(t), []string{
			`xp_award_undone by <master> {"award_id":"<given>","character_ids":["<toren>"],"milestone_id":"<ponte>","mode":"milestone"}`,
			`xp_award_undone by <master> {"award_id":"<reached>","character_ids":["<pensantus>"],"milestone_id":"<ponte>","mode":"milestone"}`,
			`xp_award_undone by <master> {"award_id":"<adhoc>","character_ids":["<pensantus>","<toren>"],"mode":"milestone"}`,
		})
	})
	t.Run("no session open", func(t *testing.T) {
		t.Parallel()
		rig := newFaultRig(t, gold, 1)
		pens := rig.pcs[0].GetId()
		mapID := rig.newMap()
		rig.label(mapID, "map")
		bolsa := rig.treasure(t, mapID, "Bolsa", 120, nil, pens)
		rig.label(bolsa, "bolsa")
		town, err := rig.backToTown("", []string{pens}, bolsa)
		if err != nil {
			t.Fatalf("AwardXP(voltar à cidade) error = %v", err)
		}
		rig.label(town.GetAward().GetId(), "town")
		if _, err := rig.undoWith(newKey(), ""); err != nil {
			t.Fatalf("UndoLastXPAward() error = %v", err)
		}
		wantLines(t, "rows", []string{rig.awardRow(t, town.GetAward().GetId())}, []string{
			`award <town> mode=gold reason="Voltar à cidade" given_by=<master> encounter= gold=120 total=120 milestone= again=false hashed=true undone=true undone_by=<master> undo_key=true; share <pensantus> xp=120 level_at_mark=null; treasure <bolsa> po=120`,
		})
		wantLines(t, "events", rig.takeEvents(t), nil)
	})
}

// TestARetriedAwardKeyAnswersTheFirstAward: a retry is looked up before
// anything else is checked, so what changed since (a character that died, the
// campaign's XP mode) does not turn it into an error; one that raced with its
// own first request finds it inside the transaction.
func TestARetriedAwardKeyAnswersTheFirstAward(t *testing.T) {
	t.Parallel()
	rig := newFaultRig(t, enemies, 2)
	pens, toren := rig.pcs[0].GetId(), rig.pcs[1].GetId()
	rig.startSession(t)
	key := newKey()
	send := func(amount int32) (*progressionv1.AwardXPResponse, error) {
		return rig.awardWith(key, func(r *progressionv1.AwardXPRequest) {
			r.Mode, r.Amount, r.CharacterIds = modeManualAward, amount, []string{pens, toren}
		})
	}
	first, err := send(10)
	if err != nil {
		t.Fatalf("AwardXP() error = %v", err)
	}
	rig.label(first.GetAward().GetId(), "first")
	if _, err := rig.master.characters.MarkCharacterDead(t.Context(), connect.NewRequest(&charactersv1.MarkCharacterDeadRequest{CampaignId: rig.campaign, CharacterId: toren})); err != nil {
		t.Fatalf("MarkCharacterDead() error = %v", err)
	}
	if err := rig.master.setXPMode(rig.campaign, milestones, true); err != nil {
		t.Fatalf("SetCampaignXpMode() error = %v", err)
	}
	again, err := send(10)
	if err != nil || again.GetAward().GetId() != first.GetAward().GetId() || again.GetXpEach() != 5 {
		t.Errorf("AwardXP(retry after a death and a change of mode) = %v, %v; want the first award", again, err)
	}
	_, err = send(11)
	rig.wantAwardRefusal(t, "the key of another request, after a change of mode", err, "invalid_argument: idempotency_key was already used for another change")
	if err := rig.master.setXPMode(rig.campaign, enemies, true); err != nil {
		t.Fatalf("SetCampaignXpMode() error = %v", err)
	}
	rig.takeEvents(t)

	// The first request commits in the gap between the checks and the
	// transaction of its retry: the retry finds it inside, and answers it.
	raceKey := newKey()
	race := func(r *progressionv1.AwardXPRequest) {
		r.Mode, r.Amount, r.CharacterIds = modeManualAward, 8, []string{pens}
	}
	var inner *progressionv1.AwardXPResponse
	rig.h.beforeAward = func(context.Context) {
		var err error
		if inner, err = rig.awardWith(raceKey, race); err != nil {
			t.Errorf("AwardXP(inside the gap) error = %v", err)
		}
	}
	outer, err := rig.awardWith(raceKey, race)
	if err != nil || inner == nil || outer.GetAward().GetId() != inner.GetAward().GetId() {
		t.Fatalf("AwardXP(raced by its own first request) = %v, %v; want the first's award", outer, err)
	}
	rig.label(outer.GetAward().GetId(), "raced")
	// Another request with the key, in the same gap, is refused inside.
	otherKey := newKey()
	rig.h.beforeAward = func(context.Context) {
		if _, err := rig.awardWith(otherKey, func(r *progressionv1.AwardXPRequest) { race(r); r.Amount = 9 }); err != nil {
			t.Errorf("AwardXP(inside the gap) error = %v", err)
		}
	}
	_, err = rig.awardWith(otherKey, race)
	rig.wantAwardRefusal(t, "another request with the key, in the gap", err, "invalid_argument: idempotency_key was already used for another change")
	if got := rig.master.xpOf(t, rig.pcs[0]); got != 5+8+9 {
		t.Errorf("sheet XP = %d, want %d: each request given once", got, 5+8+9)
	}
	// Each request that wrote is logged once; the ones that found it are not.
	wantLines(t, "events of the races", rig.takeEvents(t), []string{
		`xp_awarded by <master> {"award_id":"<raced>","character_ids":["<pensantus>"],"mode":"manual","total_xp":8,"xp_each":8}`,
		`xp_awarded by <master> {"award_id":"<uuid1>","character_ids":["<pensantus>"],"mode":"manual","total_xp":9,"xp_each":9}`,
	})
}

// TestARetryOfAnAwardWithoutAHashIsComparedByWhatItLeft: an award written
// before the request's hash was kept is compared by its mode, its milestone,
// "Dar a mais alguém", its treasures (gold) and its characters (planned
// milestones).
func TestARetryOfAnAwardWithoutAHashIsComparedByWhatItLeft(t *testing.T) {
	t.Parallel()
	forget := func(t *testing.T, rig *faultRig, awardID string) {
		t.Helper()
		if _, err := rig.h.pool.Exec(t.Context(), `UPDATE xp_awards SET idempotency_hash = NULL WHERE id = $1`, awardID); err != nil {
			t.Fatalf("forget the hash: %v", err)
		}
	}
	const reused = "invalid_argument: idempotency_key was already used for another change"
	t.Run("manual", func(t *testing.T) {
		t.Parallel()
		rig := newFaultRig(t, enemies, 2)
		pens, toren := rig.pcs[0].GetId(), rig.pcs[1].GetId()
		key := newKey()
		send := func(mode progressionv1.XPAwardMode, amount int32, reason string, ids ...string) (*progressionv1.AwardXPResponse, error) {
			return rig.awardWith(key, func(r *progressionv1.AwardXPRequest) {
				r.Mode, r.Amount, r.Reason, r.CharacterIds = mode, amount, reason, ids
			})
		}
		first, err := send(modeManualAward, 10, "Ponte", pens)
		if err != nil {
			t.Fatalf("AwardXP() error = %v", err)
		}
		forget(t, rig, first.GetAward().GetId())
		// Finding: without the hash, a manual retry with another amount, reason
		// or characters is taken for the first request and answered with it.
		for _, c := range []struct {
			name   string
			amount int32
			reason string
			ids    []string
		}{{"the same", 10, "Ponte", []string{pens}}, {"another amount", 99, "Ponte", []string{pens}}, {"another reason", 10, "Outra", []string{pens}}, {"other characters", 10, "Ponte", []string{toren}}} {
			if res, err := send(modeManualAward, c.amount, c.reason, c.ids...); err != nil || res.GetAward().GetId() != first.GetAward().GetId() {
				t.Errorf("AwardXP(%s, no hash) = %v, %v; want the first award", c.name, res, err)
			}
		}
		_, err = rig.awardWith(key, func(r *progressionv1.AwardXPRequest) {
			r.Mode, r.EncounterId, r.CharacterIds = modeEnemiesAward, newKey(), []string{pens}
		})
		rig.wantAwardRefusal(t, "another mode", err, reused)
		_, err = rig.master.xp.MarkMilestone(t.Context(), connect.NewRequest(&progressionv1.MarkMilestoneRequest{
			CampaignId: rig.campaign, Reason: "Ponte", CharacterIds: []string{pens}, IdempotencyKey: key,
		}))
		rig.wantAwardRefusal(t, "a milestone with a manual award's key", err, reused)
		if got := rig.master.xpOf(t, rig.pcs[1]); got != 0 {
			t.Errorf("Toren's XP = %d, want 0", got)
		}
	})
	t.Run("gold", func(t *testing.T) {
		t.Parallel()
		rig := newFaultRig(t, gold, 1)
		pens := rig.pcs[0].GetId()
		mapID := rig.newMap()
		a := rig.treasure(t, mapID, "A", 10, nil, pens)
		b := rig.treasure(t, mapID, "B", 20, nil, pens)
		c := rig.treasure(t, mapID, "C", 30, nil, pens)
		typedKey, townKey := newKey(), newKey()
		typed, err := rig.awardWith(typedKey, func(r *progressionv1.AwardXPRequest) {
			r.Mode, r.Gold, r.CharacterIds = modeGoldAward, 40, []string{pens}
		})
		if err != nil {
			t.Fatalf("AwardXP(typed gold) error = %v", err)
		}
		town, err := rig.backToTown(townKey, []string{pens}, a, b)
		if err != nil {
			t.Fatalf("AwardXP(voltar à cidade) error = %v", err)
		}
		forget(t, rig, typed.GetAward().GetId())
		forget(t, rig, town.GetAward().GetId())
		if res, err := rig.awardWith(typedKey, func(r *progressionv1.AwardXPRequest) {
			r.Mode, r.Gold, r.CharacterIds = modeGoldAward, 41, []string{pens}
		}); err != nil || res.GetAward().GetId() != typed.GetAward().GetId() {
			t.Errorf("AwardXP(typed gold, other gold, no hash) = %v, %v; want the first award", res, err)
		}
		_, err = rig.backToTown(typedKey, []string{pens}, c)
		rig.wantAwardRefusal(t, "treasures with a typed gold's key", err, reused)
		_, err = rig.awardWith(townKey, func(r *progressionv1.AwardXPRequest) {
			r.Mode, r.Gold, r.CharacterIds = modeGoldAward, 30, []string{pens}
		})
		rig.wantAwardRefusal(t, "typed gold with a conversion's key", err, reused)
		_, err = rig.backToTown(townKey, []string{pens}, a)
		rig.wantAwardRefusal(t, "fewer treasures", err, reused)
		_, err = rig.backToTown(townKey, []string{pens}, a, c)
		rig.wantAwardRefusal(t, "other treasures", err, reused)
		if res, err := rig.backToTown(townKey, []string{pens}, b, a); err != nil || res.GetAward().GetId() != town.GetAward().GetId() {
			t.Errorf("AwardXP(the same treasures in another order, no hash) = %v, %v; want the first award", res, err)
		}
	})
	t.Run("milestones", func(t *testing.T) {
		t.Parallel()
		rig := newFaultRig(t, milestones, 2)
		pens, toren := rig.pcs[0].GetId(), rig.pcs[1].GetId()
		adHocKey := newKey()
		mark := func(reason string, ids ...string) (*progressionv1.XPAward, error) {
			res, err := rig.master.xp.MarkMilestone(t.Context(), connect.NewRequest(&progressionv1.MarkMilestoneRequest{
				CampaignId: rig.campaign, Reason: reason, CharacterIds: ids, IdempotencyKey: adHocKey,
			}))
			return res.Msg.GetAward(), err
		}
		adHoc, err := mark("Chefe", pens)
		if err != nil {
			t.Fatalf("MarkMilestone() error = %v", err)
		}
		forget(t, rig, adHoc.GetId())
		// Finding: an ad hoc milestone without the hash compares no characters
		// and no reason: other ones are the first request.
		if res, err := mark("Outro", toren); err != nil || res.GetId() != adHoc.GetId() {
			t.Errorf("MarkMilestone(other characters and reason, no hash) = %v, %v; want the first award", res, err)
		}

		ponte, rio := rig.master.plan(t, rig.campaign, "A ponte"), rig.master.plan(t, rig.campaign, "O rio")
		reachKey := newKey()
		reach := func(milestoneID string, ids ...string) error {
			_, err := rig.master.xp.MarkMilestoneReached(t.Context(), connect.NewRequest(&progressionv1.MarkMilestoneReachedRequest{
				CampaignId: rig.campaign, MilestoneId: milestoneID, CharacterIds: ids, IdempotencyKey: reachKey,
			}))
			return err
		}
		if err := reach(ponte.GetId(), pens, toren); err != nil {
			t.Fatalf("MarkMilestoneReached() error = %v", err)
		}
		var reachedID string
		if err := rig.h.pool.QueryRow(t.Context(), `SELECT id::TEXT FROM xp_awards WHERE idempotency_key = $1`, reachKey).Scan(&reachedID); err != nil {
			t.Fatalf("find the award: %v", err)
		}
		forget(t, rig, reachedID)
		if err := reach(ponte.GetId(), toren, pens); err != nil {
			t.Errorf("MarkMilestoneReached(the same, characters in another order, no hash) error = %v", err)
		}
		rig.wantAwardRefusal(t, "fewer characters", reach(ponte.GetId(), pens), reused)
		rig.wantAwardRefusal(t, "another milestone", reach(rio.GetId(), pens, toren), reused)
		_, err = rig.master.xp.GiveMilestoneTo(t.Context(), connect.NewRequest(&progressionv1.GiveMilestoneToRequest{
			CampaignId: rig.campaign, MilestoneId: ponte.GetId(), CharacterIds: []string{pens, toren}, IdempotencyKey: reachKey,
		}))
		rig.wantAwardRefusal(t, "a give with a first mark's key", err, reused)
		_, err = rig.master.xp.MarkMilestoneReached(t.Context(), connect.NewRequest(&progressionv1.MarkMilestoneReachedRequest{
			CampaignId: rig.campaign, MilestoneId: ponte.GetId(), CharacterIds: []string{pens}, IdempotencyKey: adHocKey,
		}))
		rig.wantAwardRefusal(t, "a planned milestone with an ad hoc one's key", err, reused)
	})
}

// TestAnAwardAndAnUndoWhenAnotherModuleFails: each call to another module
// that fails rolls the whole award or undo back and answers the client; a
// sheet that is no longer a player character's is refused when giving and
// skipped when undoing.
func TestAnAwardAndAnUndoWhenAnotherModuleFails(t *testing.T) {
	t.Parallel()
	rig := newFaultRig(t, gold, 2)
	pens, toren := rig.pcs[0].GetId(), rig.pcs[1].GetId()
	mapID := rig.newMap()
	rig.startSession(t)
	notFound := connect.NewError(connect.CodeNotFound, errInjected)
	bolsa := rig.treasure(t, mapID, "Bolsa", 40, nil, pens)
	rig.label(bolsa, "bolsa")
	town := func() error {
		_, err := rig.backToTown("", []string{pens, toren}, bolsa)
		return err
	}
	for _, c := range []struct {
		fault string
		err   error
		want  string
	}{
		{"xp_mode", errInjected, internalError},
		{"xp_mode_in_tx", errInjected, internalError},
		{"party", errInjected, internalError},
		{"lock_treasures", errInjected, internalError},
		{"add_experience", errInjected, internalError},
		// A sheet the party listed but the characters module no longer finds.
		{
			"add_experience", notFound,
			"failed_precondition: only the campaign's living player characters can get XP [reason=XP_BLOCKED_REASON_CHARACTER_NOT_ELIGIBLE character=<pensantus> xp_mode=XP_MODE_UNSPECIFIED treasure=]",
		},
		// The treasures' own errors go back unwrapped.
		{"mark_converted", errInjected, internalError},
		{"append_event", errInjected, internalError},
	} {
		rig.faults.arm(c.fault, c.err)
		rig.wantAwardRefusal(t, "AwardXP with "+c.fault+" failing", town(), c.want)
	}
	if n := len(rig.master.history(t, rig.campaign)); n != 0 {
		t.Fatalf("history has %d awards after the failures, want none", n)
	}
	if got := rig.master.xpOf(t, rig.pcs[0]); got != 0 {
		t.Errorf("sheet XP = %d after the failures, want 0", got)
	}
	if left := rig.toConvert(t); len(left) != 1 {
		t.Errorf("%d treasures to convert after the failures, want 1", len(left))
	}
	wantLines(t, "events after the failures", rig.takeEvents(t), nil)

	given, err := rig.backToTown("", []string{pens, toren}, bolsa)
	if err != nil {
		t.Fatalf("AwardXP(voltar à cidade) error = %v", err)
	}
	rig.label(given.GetAward().GetId(), "town")
	rig.takeEvents(t)
	for _, fault := range []string{"add_experience", "release", "append_event"} {
		rig.faults.arm(fault, errInjected)
		_, err := rig.undoWith(newKey(), "")
		rig.wantAwardRefusal(t, "UndoLastXPAward with "+fault+" failing", err, internalError)
	}
	if got := rig.master.xpOf(t, rig.pcs[0]); got != 20 {
		t.Errorf("sheet XP = %d after the failed undos, want 20", got)
	}
	// A share whose sheet is gone from the player characters is skipped: the
	// undo goes on with the others.
	rig.faults.arm("add_experience", notFound)
	if _, err := rig.undoWith(newKey(), ""); err != nil {
		t.Fatalf("UndoLastXPAward(a sheet not found) error = %v", err)
	}
	if a, b := rig.master.xpOf(t, rig.pcs[0]), rig.master.xpOf(t, rig.pcs[1]); a != 20 || b != 0 {
		t.Errorf("sheet XP = %d and %d after the undo, want 20 (not found, skipped) and 0", a, b)
	}
	wantLines(t, "events of the undo", rig.takeEvents(t), []string{
		`xp_award_undone by <master> {"award_id":"<town>","character_ids":["<pensantus>","<toren>"],"mode":"gold","total_xp":40,"treasures":[{"point_id":"<bolsa>","value_po":40}]}`,
	})

	// A combat that cannot be read: its error goes back unwrapped.
	en := newFaultRig(t, enemies, 1)
	goblin := en.master.minion(t, en.campaign, "Goblin", 50)
	enc := en.fight(t, goblin, 1, 1, true)
	en.faults.arm("encounter", errInjected)
	_, err = en.enemies(enc.GetId(), en.pcs[0].GetId())
	en.wantAwardRefusal(t, "AwardXP with the combat failing", err, internalError)
}

// TestAFailureOfAnotherModuleIsLogged: what the client reads of a failure it
// cannot fix says nothing, so the service logs it at ERROR for the operator.
func TestAFailureOfAnotherModuleIsLogged(t *testing.T) {
	t.Parallel()
	rig := newFaultRig(t, enemies, 1)
	rig.faults.arm("party", errInjected)
	_, err := rig.master.award(rig.campaign, func(r *progressionv1.AwardXPRequest) {
		r.Mode, r.Amount, r.CharacterIds = modeManualAward, 10, []string{rig.pcs[0].GetId()}
	})
	rig.wantAwardRefusal(t, "AwardXP with the party failing", err, internalError)
	rig.wantFailureLogged(t, "AwardXP")
}

// TestAnUndoSkipsWhatItHasNoXPToTakeBack: a share of 0 XP (the sheet was
// full) and a sheet that is no longer a player character's take nothing back,
// and the undo still goes through.
func TestAnUndoSkipsWhatItHasNoXPToTakeBack(t *testing.T) {
	t.Parallel()
	rig := newFaultRig(t, enemies, 2)
	pens, toren := rig.pcs[0].GetId(), rig.pcs[1].GetId()
	if _, err := rig.h.pool.Exec(t.Context(), `UPDATE characters SET sheet = jsonb_set(sheet, '{full,experience_points}', '1000000') WHERE id = $1`, toren); err != nil {
		t.Fatalf("fill Toren's sheet: %v", err)
	}
	res := rig.master.manual(t, rig.campaign, 20, pens, toren)
	rig.label(res.GetAward().GetId(), "full")
	if _, err := rig.h.pool.Exec(t.Context(), `UPDATE characters SET status = 'pending' WHERE id = $1`, pens); err != nil {
		t.Fatalf("make Pensantus pending: %v", err)
	}
	if _, err := rig.undoWith(newKey(), ""); err != nil {
		t.Fatalf("UndoLastXPAward() error = %v", err)
	}
	// Toren's share is what the full sheet gained: nothing.
	wantLines(t, "rows", []string{rig.awardRow(t, res.GetAward().GetId())}, []string{
		`award <full> mode=manual reason="Pela ajuda ao ferreiro" given_by=<master> encounter= gold=null total=20 milestone= again=false hashed=true undone=true undone_by=<master> undo_key=true; share <pensantus> xp=10 level_at_mark=null; share <toren> xp=0 level_at_mark=null`,
	})
	var pensXP, torenXP int32
	if err := rig.h.pool.QueryRow(t.Context(), `SELECT (sheet->'full'->>'experience_points')::INT4 FROM characters WHERE id = $1`, pens).Scan(&pensXP); err != nil {
		t.Fatalf("read Pensantus's XP: %v", err)
	}
	if err := rig.h.pool.QueryRow(t.Context(), `SELECT (sheet->'full'->>'experience_points')::INT4 FROM characters WHERE id = $1`, toren).Scan(&torenXP); err != nil {
		t.Fatalf("read Toren's XP: %v", err)
	}
	if pensXP != 10 || torenXP != 1_000_000 {
		t.Errorf("XP = %d and %d after the undo, want 10 (not a player character any more) and 1000000 (gained nothing)", pensXP, torenXP)
	}
}

// TestAnAwardThatLosesARaceInsideItsTransaction: what another request commits
// while an award is being given is judged inside the award's transaction: a
// change of the XP mode after the first check, and an award for the same
// combat, or with the same key, written right before this one.
func TestAnAwardThatLosesARaceInsideItsTransaction(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 3) // the racing request runs while the award's transaction holds a connection
	rig := newFaultRig(t, enemies, 2)
	pens, toren := rig.pcs[0].GetId(), rig.pcs[1].GetId()
	manual := func(r *progressionv1.AwardXPRequest) {
		r.Mode, r.Amount, r.CharacterIds = modeManualAward, 10, []string{pens}
	}

	rig.h.beforeAward = func(context.Context) {
		if err := rig.master.setXPMode(rig.campaign, milestones, true); err != nil {
			t.Errorf("SetCampaignXpMode() error = %v", err)
		}
	}
	_, err := rig.master.award(rig.campaign, manual)
	rig.wantAwardRefusal(t, "the mode changed after the first check", err,
		"failed_precondition: this campaign's XP mode does not take this [reason=XP_BLOCKED_REASON_MODE_NOT_ALLOWED character= xp_mode=XP_MODE_MILESTONES treasure=]")
	if err := rig.master.setXPMode(rig.campaign, enemies, true); err != nil {
		t.Fatalf("SetCampaignXpMode() error = %v", err)
	}

	goblin := rig.master.minion(t, rig.campaign, "Goblin", 50)
	enc := rig.fight(t, goblin, 2, 2, true)
	rig.faults.setBeforeWrite(func() {
		if _, err := rig.enemies(enc.GetId(), pens); err != nil {
			t.Errorf("AwardXP(the racing award) error = %v", err)
		}
	})
	_, err = rig.enemies(enc.GetId(), toren)
	// The insert conflicts with the racing write (40001), and the retry of the
	// transaction finds the combat's award: the unique index's own refusal
	// is the last guard, and CockroachDB never gets to it here.
	rig.wantAwardRefusal(t, "the combat's XP given right before", err,
		"failed_precondition: the combat's XP was already given [reason=XP_BLOCKED_REASON_ALREADY_AWARDED character= xp_mode=XP_MODE_UNSPECIFIED treasure=]")

	key := newKey()
	var first *progressionv1.AwardXPResponse
	rig.faults.setBeforeWrite(func() {
		var err error
		if first, err = rig.awardWith(key, manual); err != nil {
			t.Errorf("AwardXP(the racing first request) error = %v", err)
		}
	})
	res, err := rig.awardWith(key, manual)
	// The same way, the retry of the transaction finds the key: the answer is
	// the first request's award.
	rig.wantAwardRefusal(t, "the same key written right before", err, "ok")
	if first == nil || res.GetAward().GetId() != first.GetAward().GetId() {
		t.Errorf("AwardXP(raced by its first request) = %v; want the first's award", res)
	}
	if got := rig.master.xpOf(t, rig.pcs[1]); got != 0 {
		t.Errorf("Toren's XP = %d, want 0", got)
	}
	if got := rig.master.xpOf(t, rig.pcs[0]); got != 100+10 {
		t.Errorf("Pensantus's XP = %d, want %d: each award given once", got, 100+10)
	}
}
