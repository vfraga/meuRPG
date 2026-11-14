package characters

import (
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
)

// #580 S3 through the service: the pick of a daily choice is written on a locked sheet, by the owner and by the master,
// and the rests clear it; a fixed proficiency until the short rest is spent by it and given back by the long rest.

func dailyHarness(t *testing.T) (h *harness, master, player *user, campaign string, pc *charactersv1.Character, choice string) {
	t.Helper()
	h = newHarness(t)
	master, player = h.newUser("Samuel"), h.newUser("Dona")
	campaign = h.newCampaign(master, "Mirathel", player)
	feat := master.addEntry(t, campaign, &rulesv1.TableFeat{
		NamePt: "Repertório do Dia", DescPt: []string{"Texto."},
		Effects: []*rulesv1.TableEffect{
			{Type: "choice", Choice: "skill", Count: 1, DailyChoice: true, Duration: "long_rest"},
			{Type: "choice", Choice: "tool", Count: 1, DailyChoice: true, Duration: "short_rest"},
			{Type: "proficiency", Proficiency: "skill:perception", Duration: "short_rest"},
		},
	})
	sheet := pensantusSheet()
	sheet.GetFull().SkillProficiencyKeys = nil
	pc = player.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", sheet)
	// A player's feats come from the level-up; the master adds one to a sheet.
	sheet.GetFull().FeatKeys = []string{feat.GetKey()}
	var err error
	if pc, err = master.update(t, pc, "Pensantus", sheet); err != nil {
		t.Fatalf("UpdateCharacter(feat) error = %v", err)
	}
	return h, master, player, campaign, pc, feat.GetKey() + "#0"
}

func (u *user) pick(t *testing.T, campaign, character, choice, key string) error {
	t.Helper()
	_, err := u.api.SetDailyChoice(t.Context(), connect.NewRequest(&charactersv1.SetDailyChoiceRequest{CampaignId: campaign, CharacterId: character, ChoiceId: choice, Key: key}))
	return err
}

func skillOfSheet(c *charactersv1.Character, key string) *rulesv1.DerivedSkill {
	for _, s := range c.GetDerived().GetSkills() {
		if s.GetKey() == key {
			return s
		}
	}
	return nil
}

func takeRest(t *testing.T, h *harness, campaign string, kind playv1.RestKind) {
	t.Helper()
	err := db.InTx(t.Context(), h.pool, func(tx pgx.Tx) error {
		_, _, err := h.svc.TakeRest(t.Context(), tx, campaign, &playv1.TakeRestRequest{Kind: kind})
		return err
	})
	if err != nil {
		t.Fatalf("TakeRest(%v) error = %v", kind, err)
	}
}

func TestADailyChoiceIsPickedOnALockedSheetAndTheRestsClearIt(t *testing.T) {
	t.Parallel()
	h, master, player, campaign, pc, choice := dailyHarness(t)
	if h.lockSheets(campaign) != 1 {
		t.Fatal("expected the sheet to lock")
	}
	if got := player.get(t, campaign, pc.GetId()).GetDerived().GetDailyChoices(); len(got) != 2 || got[0].GetPicked() != "" {
		t.Fatalf("daily choices = %v, want two waiting", got)
	}

	if err := player.pick(t, campaign, pc.GetId(), choice, "skill:stealth"); err != nil {
		t.Fatalf("the owner's pick on a locked sheet: %v", err)
	}
	if s := skillOfSheet(player.get(t, campaign, pc.GetId()), "skill:stealth"); s.GetProficiency() != rulesv1.ProficiencyLevel_PROFICIENCY_LEVEL_PROFICIENT || s.GetUntilRest() != rulesv1.RestBound_REST_BOUND_LONG {
		t.Errorf("stealth after the pick = %v, want full proficiency until the long rest", s)
	}
	if err := master.pick(t, campaign, pc.GetId(), choice, "skill:arcana"); err != nil {
		t.Fatalf("the master's pick: %v", err)
	}
	for name, bad := range map[string]struct{ choice, key string }{
		"a key the choice does not offer": {choice, "skill:nada"},
		"a choice the sheet lacks":        {"feat:nada#0", "skill:stealth"},
	} {
		if err := player.pick(t, campaign, pc.GetId(), bad.choice, bad.key); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("%s: error = %v, want invalid_argument", name, err)
		}
	}
	if err := h.newUser("Intrusa").pick(t, campaign, pc.GetId(), choice, "skill:stealth"); err == nil {
		t.Error("a stranger picked for the sheet")
	}

	// A short rest spends the fixed proficiency but keeps the long-rest pick; the long rest ends the pick and gives it back.
	takeRest(t, h, campaign, playv1.RestKind_REST_KIND_SHORT)
	got := player.get(t, campaign, pc.GetId())
	if s := skillOfSheet(got, "skill:perception"); s.GetProficiency() != rulesv1.ProficiencyLevel_PROFICIENCY_LEVEL_NONE {
		t.Errorf("perception after the short rest = %v, want it spent", s)
	}
	if s := skillOfSheet(got, "skill:arcana"); s.GetUntilRest() != rulesv1.RestBound_REST_BOUND_LONG {
		t.Errorf("a pick until the long rest after a short rest = %v, want it kept", s)
	}
	takeRest(t, h, campaign, playv1.RestKind_REST_KIND_LONG)
	got = player.get(t, campaign, pc.GetId())
	if s := skillOfSheet(got, "skill:perception"); s.GetProficiency() != rulesv1.ProficiencyLevel_PROFICIENCY_LEVEL_PROFICIENT {
		t.Errorf("perception after the long rest = %v, want it back", s)
	}
	if s := skillOfSheet(got, "skill:arcana"); s.GetUntilRest() != rulesv1.RestBound_REST_BOUND_UNSPECIFIED || slices.ContainsFunc(got.GetDerived().GetDailyChoices(), func(c *rulesv1.DailyChoice) bool { return c.GetPicked() != "" }) {
		t.Errorf("after the long rest the pick should be gone: %v %v", s, got.GetDerived().GetDailyChoices())
	}
}

// A save never writes the picks: the client's copy of them is dropped and the stored ones kept.
func TestASaveKeepsTheStoredDailyPicks(t *testing.T) {
	t.Parallel()
	_, master, player, campaign, pc, choice := dailyHarness(t)
	if err := player.pick(t, campaign, pc.GetId(), choice, "skill:stealth"); err != nil {
		t.Fatal(err)
	}
	cur := master.get(t, campaign, pc.GetId())
	sheet := cur.GetSheet()
	sheet.GetFull().DailyPicks = []*charactersv1.DailyPick{{ChoiceId: choice, Key: "skill:arcana", UntilRest: rulesv1.RestBound_REST_BOUND_LONG}}
	sheet.GetFull().ShortRested = true
	if _, err := master.update(t, cur, "Pensantus", sheet); err != nil {
		t.Fatalf("UpdateCharacter() error = %v", err)
	}
	got := master.get(t, campaign, pc.GetId())
	if got.GetSheet().GetFull().GetShortRested() || len(got.GetSheet().GetFull().GetDailyPicks()) != 1 || got.GetSheet().GetFull().GetDailyPicks()[0].GetKey() != "skill:stealth" {
		t.Errorf("the saved sheet's picks = %v (short rested %v), want the stored stealth pick and no mark", got.GetSheet().GetFull().GetDailyPicks(), got.GetSheet().GetFull().GetShortRested())
	}
}

func sheetPicksOf(c *charactersv1.Character) []*charactersv1.DailyPick {
	return c.GetSheet().GetFull().GetDailyPicks()
}

// A rest that read the sheets before a pick was made must not take the pick back: the rest locks the sheet and reads it again.
func TestARestDoesNotOverwriteAPickMadeAfterItReadTheSheets(t *testing.T) {
	t.Parallel()
	h, _, player, campaign, pc, choice := dailyHarness(t)
	var stale restRow
	if err := db.InTx(t.Context(), h.pool, func(tx pgx.Tx) error {
		rows, err := h.svc.restRows(t.Context(), tx, campaign)
		if len(rows) == 1 {
			stale = rows[0]
		}
		return err
	}); err != nil || stale.row.ID == "" {
		t.Fatalf("restRows() = %v, %v", stale.row.ID, err)
	}
	if err := player.pick(t, campaign, pc.GetId(), choice, "skill:stealth"); err != nil {
		t.Fatal(err)
	}
	if err := db.InTx(t.Context(), h.pool, func(tx pgx.Tx) error {
		return h.svc.endRestPicks(t.Context(), h.svc.queries.WithTx(tx), stale, false)
	}); err != nil {
		t.Fatalf("endRestPicks() error = %v", err)
	}
	got := player.get(t, campaign, pc.GetId())
	if p := sheetPicksOf(got); len(p) != 1 || p[0].GetKey() != "skill:stealth" {
		t.Errorf("picks after a short rest that read the sheets before the pick = %v, want the stealth pick kept", p)
	}
	if !got.GetSheet().GetFull().GetShortRested() {
		t.Errorf("the short rest left no mark on a sheet with a short-rest proficiency")
	}
}

// The rest clears by what the effect lasts today: the master changes a long-rest choice to a short-rest one.
func TestARestClearsAPickByTheEffectsDurationToday(t *testing.T) {
	t.Parallel()
	h, master, player, campaign, pc, choice := dailyHarness(t)
	if err := player.pick(t, campaign, pc.GetId(), choice, "skill:stealth"); err != nil {
		t.Fatal(err)
	}
	key, _, _ := strings.Cut(choice, "#")
	entry := master.entries(t, campaign)[key]
	body := proto.Clone(bodyMessage(entry)).(*rulesv1.TableFeat)
	body.Effects[0].Duration = "short_rest"
	if _, err := master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, key, entry.GetRevision(), body))); err != nil {
		t.Fatalf("UpdateTableEntry() error = %v", err)
	}
	takeRest(t, h, campaign, playv1.RestKind_REST_KIND_SHORT)
	if p := sheetPicksOf(player.get(t, campaign, pc.GetId())); len(p) != 0 {
		t.Errorf("picks after a short rest = %v, want the pick cleared: the choice now lasts until a short rest", p)
	}
}

// A short rest clears a pick that lasts until a short rest and keeps one that lasts until a long rest; the mark is only set
// for a sheet that has a proficiency until a short rest.
func TestAShortRestClearsAShortRestPickAndMarksOnlyASheetThatHasOne(t *testing.T) {
	t.Parallel()
	h, master, player, campaign, pc, choice := dailyHarness(t)
	key, _, _ := strings.Cut(choice, "#")
	other := h.newUser("Outra")
	h.join(master, campaign, other)
	if err := player.pick(t, campaign, pc.GetId(), key+"#1", "proficiency:carpenters-tools"); err != nil {
		t.Fatal(err)
	}
	if err := player.pick(t, campaign, pc.GetId(), choice, "skill:stealth"); err != nil {
		t.Fatal(err)
	}
	plain := other.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Sem feat", pensantusSheet())
	takeRest(t, h, campaign, playv1.RestKind_REST_KIND_SHORT)
	got := player.get(t, campaign, pc.GetId())
	if p := sheetPicksOf(got); len(p) != 1 || p[0].GetKey() != "skill:stealth" {
		t.Errorf("picks after a short rest = %v, want only the long-rest pick", p)
	}
	if !got.GetSheet().GetFull().GetShortRested() {
		t.Error("the sheet with a short-rest proficiency has no mark after the short rest")
	}
	if other.get(t, campaign, plain.GetId()).GetSheet().GetFull().GetShortRested() {
		t.Error("a sheet with no short-rest proficiency was marked")
	}
}

// Picking what the choice already holds changes nothing: the revision stays, and the key is judged as any other.
func TestPickingTheSameKeyAgainIsANoOpAndAKeyThatIsNoToolIsRefusedAlike(t *testing.T) {
	t.Parallel()
	_, _, player, campaign, pc, choice := dailyHarness(t)
	if err := player.pick(t, campaign, pc.GetId(), choice, "skill:stealth"); err != nil {
		t.Fatal(err)
	}
	rev := player.get(t, campaign, pc.GetId()).GetRevision()
	if err := player.pick(t, campaign, pc.GetId(), choice, "skill:stealth"); err != nil {
		t.Fatal(err)
	}
	if now := player.get(t, campaign, pc.GetId()).GetRevision(); now != rev {
		t.Errorf("revision %d after picking the same key again, want %d", now, rev)
	}
	// There is no table content of the tool kind, so nothing a master hid can be told apart: a proficiency that is no tool and
	// a key that does not exist are refused with the same answer.
	toolChoice := strings.TrimSuffix(choice, "#0") + "#1"
	a := player.pick(t, campaign, pc.GetId(), toolChoice, "proficiency:light-armor")
	b := player.pick(t, campaign, pc.GetId(), toolChoice, "proficiency:nada")
	if a == nil || b == nil || a.Error() != b.Error() || connect.CodeOf(a) != connect.CodeInvalidArgument {
		t.Errorf("refusals = %v and %v, want the same invalid_argument", a, b)
	}
}

// A dead character takes no pick, and a basic sheet has no daily choices.
func TestADailyPickOnADeadCharacterOrABasicSheetIsRefused(t *testing.T) {
	t.Parallel()
	_, master, player, campaign, pc, choice := dailyHarness(t)
	npc := master.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_MINION, "Capanga", basicSheet())
	if err := master.pick(t, campaign, npc.GetId(), choice, "skill:stealth"); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a pick on a basic sheet: error = %v, want invalid_argument", err)
	}
	master.markDead(t, pc)
	if err := player.pick(t, campaign, pc.GetId(), choice, "skill:stealth"); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("a pick on a dead character: error = %v, want failed_precondition", err)
	}
}
