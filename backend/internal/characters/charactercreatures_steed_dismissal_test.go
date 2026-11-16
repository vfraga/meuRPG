package characters

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
)

// The dismissal of a steed of Find Steed, through the API and the play spy: the owner pays the action first, the master pays none, a refusal
// leaves the steed as it was, and the release of a dismissed steed tells its owner.

// steed makes the wolf a bonded steed of Find Steed, in the table, as a cast out of combat leaves it.
func (tb *creatureTable) steed(t *testing.T, name string) string {
	t.Helper()
	id := tb.wolf(t, name).GetId()
	if _, err := tb.h.pool.Exec(t.Context(), `UPDATE character_creatures SET source = 'find_steed', creature_type = 'celestial', bonded = true WHERE id = $1`, id); err != nil {
		t.Fatalf("make a steed: %v", err)
	}
	return id
}

func (tb *creatureTable) dismissAs(u *user, id string, how charactersv1.CreatureDismissal) error {
	_, err := u.api.DismissCreature(context.Background(), connect.NewRequest(&charactersv1.DismissCreatureRequest{CampaignId: tb.campaign, CreatureId: id, Dismissal: how}))
	return err
}

func (tb *creatureTable) bondOf(t *testing.T, id string) bool {
	t.Helper()
	var bonded bool
	if err := tb.h.pool.QueryRow(t.Context(), "SELECT bonded FROM character_creatures WHERE id = $1", id).Scan(&bonded); err != nil {
		t.Fatalf("read the bond: %v", err)
	}
	return bonded
}

func TestSteedDismissalAsksPlayForTheOwnersActionFirst(t *testing.T) {
	t.Parallel()
	tb := newCreatureTable(t)
	id := tb.steed(t, "Trovão")
	dismiss := charactersv1.CreatureDismissal_CREATURE_DISMISSAL_DISMISS
	if err := tb.dismissAs(tb.owner, id, dismiss); err != nil {
		t.Fatal(err)
	}
	wantCreatureCalls(t, "the owner dismisses the steed", tb.spy, "lock", "spend the action for "+id, fmt.Sprintf("leaving by %s: %s", tb.owner.id, id),
		fmt.Sprintf(`event creature_dismissed by %s {"character_id":%q,"creature_ids":[%q],"reason":"dismissed"}`, tb.owner.id, tb.pc.GetId(), id),
		"publish creatures of "+tb.owner.id)
	if _, err := tb.h.pool.Exec(t.Context(), `UPDATE character_creatures SET bonded = false WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	master := tb.steed(t, "Raio")
	if err := tb.dismissAs(tb.master, master, dismiss); err != nil {
		t.Fatal(err)
	}
	for _, c := range tb.spy.take() {
		if c == "spend the action for "+master {
			t.Error("the master paid an action for the dismissal")
		}
	}
}

func TestSteedDismissalRefusedByPlayLeavesTheSteedAndNeverLeavesTheCombat(t *testing.T) {
	t.Parallel()
	tb := newCreatureTable(t)
	id := tb.steed(t, "Trovão")
	tb.spy.spendFail = errors.New("not your turn")
	if err := tb.dismissAs(tb.owner, id, charactersv1.CreatureDismissal_CREATURE_DISMISSAL_DISMISS); err == nil {
		t.Fatal("the dismissal went through a refused spend")
	}
	wantCreatureCalls(t, "a refused spend", tb.spy, "lock", "spend the action for "+id)
	if got := tb.creatureRow(t, id); got != `hp 11/11 dismissed ""` || !tb.bondOf(t, id) {
		t.Errorf("after the refusal: %s, bonded %v", got, tb.bondOf(t, id))
	}
}

func TestReleaseOfADismissedSteedEndsTheBondAndTellsItsOwner(t *testing.T) {
	t.Parallel()
	tb := newCreatureTable(t)
	id := tb.steed(t, "Trovão")
	if err := tb.dismissAs(tb.owner, id, charactersv1.CreatureDismissal_CREATURE_DISMISSAL_DISMISS); err != nil {
		t.Fatal(err)
	}
	tb.spy.take()
	release := charactersv1.CreatureDismissal_CREATURE_DISMISSAL_RELEASE
	if err := tb.dismissAs(tb.owner, id, release); err != nil {
		t.Fatal(err)
	}
	wantCreatureCalls(t, "the release of a dismissed steed", tb.spy, "lock", "publish creatures of "+tb.owner.id)
	if tb.bondOf(t, id) {
		t.Error("the bond stayed")
	}
	if err := tb.dismissAs(tb.owner, id, release); err != nil {
		t.Fatal(err)
	}
	wantCreatureCalls(t, "a second release", tb.spy, "lock")
}

func TestReleaseOfAnyCreatureButABondedSteedIsRefused(t *testing.T) {
	t.Parallel()
	tb := newCreatureTable(t)
	live, gone := tb.wolf(t, "Presa").GetId(), tb.wolf(t, "Garra").GetId()
	if err := tb.dismiss(tb.owner, gone); err != nil {
		t.Fatal(err)
	}
	tb.spy.take()
	release := charactersv1.CreatureDismissal_CREATURE_DISMISSAL_RELEASE
	for name, id := range map[string]string{"a live wolf": live, "a dismissed wolf": gone} {
		if err := tb.dismissAs(tb.owner, id, release); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("release of %s = %v, want invalid_argument", name, err)
		}
	}
}

// The bond is the character's, not the spell's: a character that cannot cast Find Steed now (it is not prepared) still reads its bonded
// steed, live or dismissed, so the sheet can draw it and offer its release. The owner and the master read it; a release ends it.
func TestSummonOptionsCarryTheBondedSteedWithoutTheSpell(t *testing.T) {
	t.Parallel()
	tb := newCreatureTable(t)
	id := tb.steed(t, "Trovão")
	bondOf := func(u *user) *charactersv1.CharacterCreature {
		t.Helper()
		res, err := u.api.GetSummonOptions(t.Context(), connect.NewRequest(&charactersv1.GetSummonOptionsRequest{CampaignId: tb.campaign, CharacterId: tb.pc.GetId()}))
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range res.Msg.GetSpells() {
			if s.GetSpellKey() == "spell:find-steed" {
				t.Fatal("the character casts Find Steed: the test wants one that does not")
			}
		}
		return res.Msg.GetBondedSteed()
	}
	if got := bondOf(tb.owner); got.GetId() != id || !got.GetBonded() {
		t.Errorf("the live steed: got %q bonded %v, want %q", got.GetId(), got.GetBonded(), id)
	}
	if err := tb.dismissAs(tb.owner, id, charactersv1.CreatureDismissal_CREATURE_DISMISSAL_DISMISS); err != nil {
		t.Fatal(err)
	}
	for name, u := range map[string]*user{"the owner": tb.owner, "the master": tb.master} {
		if got := bondOf(u); got.GetId() != id || got.GetName() != "Trovão" || got.GetCreatureType() != "celestial" {
			t.Errorf("%s, the dismissed steed: got %q %q %q", name, got.GetId(), got.GetName(), got.GetCreatureType())
		}
	}
	if err := tb.dismissAs(tb.owner, id, charactersv1.CreatureDismissal_CREATURE_DISMISSAL_RELEASE); err != nil {
		t.Fatal(err)
	}
	if got := bondOf(tb.owner); got != nil {
		t.Errorf("after the release: got %q, want no bond", got.GetId())
	}
}
