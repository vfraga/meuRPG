package characters

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
)

// The acceptance criteria of MR-040 (docs/product/stories.md): a character
// that can level up goes up on its locked sheet, through the guided level-up,
// and only gets what the level gives. Each test starts its own database.

// xpLevelUps is the progression module's rule for a campaign that counts XP
// (RN-12), without the module: the sheet's XP reaching the next level's.
type xpLevelUps struct{}

func (xpLevelUps) LevelUpReason(_ context.Context, _ pgx.Tx, _, _ string, level, xp, next int32) (charactersv1.LevelUpReason, error) {
	if level < 20 && next > 0 && xp >= next {
		return charactersv1.LevelUpReason_LEVEL_UP_REASON_XP, nil
	}
	return charactersv1.LevelUpReason_LEVEL_UP_REASON_UNSPECIFIED, nil
}

// testDiceRules is the campaign's dice setting, which a test changes.
type testDiceRules struct{ rule atomic.Int32 }

func (d *testDiceRules) LevelUpDice(context.Context, pgx.Tx, string, string) (charactersv1.LevelUpDiceRule, error) {
	return charactersv1.LevelUpDiceRule(d.rule.Load()), nil
}

// liveSpy records the campaigns that got the live hint.
type liveSpy struct {
	mu      sync.Mutex
	calls   []string
	content []string // the campaigns that got content_changed
	vitals  []string // the characters whose vitals were sent
}

func (l *liveSpy) PublishVitalsChanged(_ context.Context, _, characterID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.vitals = append(l.vitals, characterID)
}

func (l *liveSpy) PublishXPChanged(campaignID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, campaignID)
}

func (l *liveSpy) PublishContentChanged(campaignID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.content = append(l.content, campaignID)
}

func (l *liveSpy) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.calls)
}

// levelUpTable is a campaign with a master and two players, Pensantus (Wizard
// 3, 2,700 XP: level 4 is his) for the owner, and the sheets locked.
type levelUpTable struct {
	h              *harness
	master, owner  *user
	other          *user
	campaign       string
	pc             *charactersv1.Character
	dice           *testDiceRules
	live           *liveSpy
	hitDieFaces    *dice.Fixed
	pensantusSheet *charactersv1.CharacterSheet
}

// newLevelUpTable builds the table. faces are the hit die results the server
// will roll, in order.
func newLevelUpTable(t *testing.T, xp int32, faces ...int) *levelUpTable {
	t.Helper()
	tb := &levelUpTable{dice: &testDiceRules{}, live: &liveSpy{}, hitDieFaces: &dice.Fixed{Faces: faces}}
	tb.dice.rule.Store(int32(charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_PLAYER_CHOOSES))
	tb.h = newHarnessWith(t, func(c *Config) { c.Dice, c.Roller = tb.dice, tb.hitDieFaces })
	tb.h.svc.SetLevelUps(xpLevelUps{})
	tb.h.svc.SetLive(tb.live)
	tb.master, tb.owner, tb.other = tb.h.newUser("Samuel"), tb.h.newUser("Dona"), tb.h.newUser("Outra")
	tb.campaign = tb.h.newCampaign(tb.master, "Mirathel", tb.owner, tb.other)
	sheet := pensantusSheet()
	sheet.GetFull().ExperiencePoints = xp
	tb.pensantusSheet = sheet
	tb.pc = tb.owner.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", sheet)
	tb.h.lockSheets(tb.campaign)                        // the first game session started: RN-01
	tb.pc = tb.owner.get(t, tb.campaign, tb.pc.GetId()) // as the player sees it
	return tb
}

// pensantusLevelUp is the reference level 4: Int +2, a cantrip, two spells
// for the book (and prepared), the average hit points.
func pensantusLevelUp() *charactersv1.LevelUpChoices {
	return &charactersv1.LevelUpChoices{
		ClassKey:          "class:wizard",
		AbilityIncrease:   &rulesv1.AbilityScores{Intelligence: 2},
		CantripKeys:       []string{"spell:mage-hand"},
		KnownSpellKeys:    []string{"spell:misty-step", "spell:invisibility"},
		PreparedSpellKeys: []string{"spell:misty-step", "spell:invisibility"},
		HitPoints:         &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
	}
}

func (tb *levelUpTable) levelUp(u *user, c *charactersv1.Character, ch *charactersv1.LevelUpChoices) (*charactersv1.Character, error) {
	res, err := u.api.LevelUpCharacter(tb.h.t.Context(), connect.NewRequest(&charactersv1.LevelUpCharacterRequest{
		CampaignId: tb.campaign, CharacterId: c.GetId(), Revision: c.GetRevision(), Choices: ch,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetCharacter(), nil
}

func (tb *levelUpTable) options(u *user, c *charactersv1.Character) (*charactersv1.LevelUpOptions, error) {
	res, err := u.api.GetLevelUpOptions(tb.h.t.Context(), connect.NewRequest(&charactersv1.GetLevelUpOptionsRequest{CampaignId: tb.campaign, CharacterId: c.GetId()}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetOptions(), nil
}

func (tb *levelUpTable) preview(u *user, c *charactersv1.Character, ch *charactersv1.LevelUpChoices) (*charactersv1.PreviewLevelUpResponse, error) {
	res, err := u.api.PreviewLevelUp(tb.h.t.Context(), connect.NewRequest(&charactersv1.PreviewLevelUpRequest{CampaignId: tb.campaign, CharacterId: c.GetId(), Choices: ch}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (tb *levelUpTable) roll(u *user, c *charactersv1.Character) (*charactersv1.RollLevelUpHitPointsResponse, error) {
	res, err := u.api.RollLevelUpHitPoints(tb.h.t.Context(), connect.NewRequest(&charactersv1.RollLevelUpHitPointsRequest{
		CampaignId: tb.campaign, CharacterId: c.GetId(), IdempotencyKey: uuid.New().String(),
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (tb *levelUpTable) levelUps(u *user, characterID string) ([]*charactersv1.LevelUp, error) {
	res, err := u.api.ListLevelUps(tb.h.t.Context(), connect.NewRequest(&charactersv1.ListLevelUpsRequest{CampaignId: tb.campaign, CharacterId: characterID}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetLevelUps(), nil
}

// refusal returns the LevelUpRefusal detail of a failed_precondition.
func refusal(t *testing.T, call string, err error) *charactersv1.LevelUpRefusal {
	t.Helper()
	wantCode(t, call, err, connect.CodeFailedPrecondition)
	ce, _ := errors.AsType[*connect.Error](err)
	for _, d := range ce.Details() {
		if v, derr := d.Value(); derr == nil {
			if r, ok := v.(*charactersv1.LevelUpRefusal); ok {
				return r
			}
		}
	}
	t.Fatalf("%s error %v has no LevelUpRefusal detail", call, err)
	return nil
}

// TestMR040_ThePlayerLevelsUpALockedSheet: Pensantus, Wizard 3 with the XP for
// level 4, goes up on a locked sheet: the owner reads the options, previews
// the summary, and confirms; the sheet has the new level, and can_level_up
// goes away by itself.
func TestMR040_ThePlayerLevelsUpALockedSheet(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 2700)
	pc := tb.pc
	if pc.GetState() != charactersv1.CharacterState_CHARACTER_STATE_LOCKED || !pc.GetCanLevelUp() || pc.GetCanEdit() {
		t.Fatalf("Pensantus: state %v, can_level_up %v, can_edit %v; want a locked sheet that can level up", pc.GetState(), pc.GetCanLevelUp(), pc.GetCanEdit())
	}
	// The lock is real: the owner cannot edit the sheet any other way.
	_, err := tb.owner.update(t, pc, pc.GetName(), pc.GetSheet())
	if blocked(t, "UpdateCharacter", err).GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_SHEET_LOCKED {
		t.Fatal("the owner edits a locked sheet")
	}

	o, err := tb.options(tb.owner, pc)
	if err != nil {
		t.Fatalf("GetLevelUpOptions() error = %v", err)
	}
	if o.GetClassKey() != "class:wizard" || o.GetFromLevel() != 3 || o.GetToLevel() != 4 || o.GetHitDie() != 6 || o.GetHitPointAverage() != 4 ||
		!o.GetAbilityScoreImprovement() || o.GetCantrips() != 1 || o.GetSpells() != 2 || o.GetSpellsKind() != charactersv1.LevelUpSpellsKind_LEVEL_UP_SPELLS_KIND_SPELLBOOK ||
		!o.GetPrepares() || o.GetMaxSpellLevel() != 2 || o.GetPreparedMax() != 7 || o.GetSubclassDue() {
		t.Errorf("options = %v", o)
	}
	if o.GetDiceRule() != charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_PLAYER_CHOOSES || o.GetKeptHitPointRoll() != 0 ||
		o.GetSpellSlotsBefore()[1] != 2 || o.GetSpellSlotsAfter()[1] != 3 || len(o.GetNewFeatures()) != 1 {
		t.Errorf("options (automatic part) = %v", o)
	}

	// The summary: the numbers that change, from the server.
	p, err := tb.preview(tb.owner, pc, pensantusLevelUp())
	if err != nil {
		t.Fatalf("PreviewLevelUp() error = %v", err)
	}
	if p.GetRefusal() != nil || p.GetAfter().GetTotalLevel() != 4 || p.GetAfter().GetHitPointsMax() != 30 {
		t.Errorf("preview = %v, want level 4, 30 hit points, no refusal", p)
	}
	if sc := p.GetAfter().GetSpellcasting()[0]; sc.GetSaveDc() != 15 || sc.GetPreparedMax() != 9 {
		t.Errorf("preview spellcasting = %v, want DC 15 and 9 prepared", sc)
	}
	if got := tb.master.get(t, tb.campaign, pc.GetId()); got.GetRevision() != pc.GetRevision() {
		t.Error("a preview changed the sheet")
	}

	up, err := tb.levelUp(tb.owner, pc, pensantusLevelUp())
	if err != nil {
		t.Fatalf("LevelUpCharacter() error = %v", err)
	}
	d := up.GetDerived()
	if up.GetState() != charactersv1.CharacterState_CHARACTER_STATE_LOCKED || up.GetRevision() != pc.GetRevision()+1 {
		t.Errorf("state %v, revision %d; want still locked, revision %d", up.GetState(), up.GetRevision(), pc.GetRevision()+1)
	}
	full := up.GetSheet().GetFull()
	if d.GetTotalLevel() != 4 || d.GetHitPointsMax() != 30 || full.GetClasses()[0].GetLevel() != 4 || full.GetExtraAbilityBonuses().GetIntelligence() != 2 ||
		len(full.GetCantripKeys()) != 4 || len(full.GetKnownSpellKeys()) != 12 || len(full.GetPreparedSpellKeys()) != 9 {
		t.Errorf("sheet after = level %d, %d HP, class level %d, Int bonus %d, %d cantrips, %d in the book, %d prepared",
			d.GetTotalLevel(), d.GetHitPointsMax(), full.GetClasses()[0].GetLevel(), full.GetExtraAbilityBonuses().GetIntelligence(),
			len(full.GetCantripKeys()), len(full.GetKnownSpellKeys()), len(full.GetPreparedSpellKeys()))
	}
	if len(d.GetIssues()) != 0 {
		t.Errorf("issues after = %v", d.GetIssues())
	}
	if up.GetCanLevelUp() {
		t.Error("can_level_up stays after the level it could take")
	}
	if tb.live.count() != 1 {
		t.Errorf("live hints = %d, want 1", tb.live.count())
	}
	// The player cannot take a second level on the same revision.
	_, err = tb.levelUp(tb.owner, pc, pensantusLevelUp())
	wantCode(t, "LevelUpCharacter(again)", err, connect.CodeAborted)
}

// TestMR040_OnlyWhenTheCharacterCanLevelUp: every call is refused for a
// character that cannot level up now, for the owner and for the master.
func TestMR040_OnlyWhenTheCharacterCanLevelUp(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 900) // level 3, 2,700 XP for the next
	pc := tb.pc
	if pc.GetCanLevelUp() {
		t.Fatal("Pensantus can level up with 900 XP")
	}
	cannot := charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CANNOT_LEVEL_UP
	for _, u := range []*user{tb.owner, tb.master} {
		_, err := tb.options(u, pc)
		if blocked(t, "GetLevelUpOptions", err).GetReason() != cannot {
			t.Errorf("GetLevelUpOptions reason is not CANNOT_LEVEL_UP")
		}
		_, err = tb.preview(u, pc, pensantusLevelUp())
		if blocked(t, "PreviewLevelUp", err).GetReason() != cannot {
			t.Errorf("PreviewLevelUp reason is not CANNOT_LEVEL_UP")
		}
	}
	_, err := tb.roll(tb.owner, pc)
	if blocked(t, "RollLevelUpHitPoints", err).GetReason() != cannot {
		t.Error("RollLevelUpHitPoints reason is not CANNOT_LEVEL_UP")
	}
	_, err = tb.levelUp(tb.owner, pc, pensantusLevelUp())
	if blocked(t, "LevelUpCharacter", err).GetReason() != cannot {
		t.Error("LevelUpCharacter reason is not CANNOT_LEVEL_UP")
	}

	// An NPC never levels up this way.
	npc := tb.master.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_BOSS, "Strahd", enemySheet())
	_, err = tb.options(tb.master, npc)
	if blocked(t, "GetLevelUpOptions(NPC)", err).GetReason() != cannot {
		t.Error("an NPC can level up")
	}

	// The master gives the XP (a locked sheet is still the master's): now it can.
	full := proto.CloneOf(pc.GetSheet())
	full.GetFull().ExperiencePoints = 2700
	pc, err = tb.master.update(t, pc, pc.GetName(), full)
	if err != nil {
		t.Fatalf("UpdateCharacter() error = %v", err)
	}
	if !pc.GetCanLevelUp() {
		t.Fatal("Pensantus cannot level up with 2,700 XP")
	}
	if _, err := tb.options(tb.owner, pc); err != nil {
		t.Errorf("GetLevelUpOptions() error = %v", err)
	}
	// A player who is not the owner cannot even see the character.
	_, err = tb.options(tb.other, pc)
	wantCode(t, "GetLevelUpOptions(other player)", err, connect.CodeNotFound)
	// A class the character does not have.
	_, err = tb.owner.api.GetLevelUpOptions(t.Context(), connect.NewRequest(&charactersv1.GetLevelUpOptionsRequest{
		CampaignId: tb.campaign, CharacterId: pc.GetId(), ClassKey: "class:fighter",
	}))
	wantCode(t, "GetLevelUpOptions(a new class)", err, connect.CodeInvalidArgument)
}

// TestMR040_TheRestStaysLocked: the level-up changes the class, the
// ability bonuses, the hit points and the new lists, and nothing else. The
// client never sends the sheet, and a choice that would reach for something
// else is refused.
func TestMR040_TheRestStaysLocked(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 2700)
	pc := tb.pc

	// Reaching for what the level does not give (each is a different field)
	// leaves the sheet as it was.
	for name, mod := range map[string]func(ch *charactersv1.LevelUpChoices) charactersv1.LevelUpRefusalReason{
		"a skill": func(ch *charactersv1.LevelUpChoices) charactersv1.LevelUpRefusalReason {
			ch.SkillProficiencyKeys = []string{"skill:stealth"}
			return charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SKILLS
		},
		"expertise": func(ch *charactersv1.LevelUpChoices) charactersv1.LevelUpRefusalReason {
			ch.ExpertiseSkillKeys = []string{"skill:arcana"}
			return charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_EXPERTISE
		},
		"a feature option": func(ch *charactersv1.LevelUpChoices) charactersv1.LevelUpRefusalReason {
			ch.FeatureChoiceKeys = []string{"feature:fighter-fighting-style-defense"}
			return charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_FEATURE_CHOICE
		},
	} {
		ch := pensantusLevelUp()
		want := mod(ch)
		_, err := tb.levelUp(tb.owner, pc, ch)
		if got := refusal(t, name, err).GetReason(); got != want {
			t.Errorf("%s: reason = %v, want %v", name, got, want)
		}
	}
	if got := tb.owner.get(t, tb.campaign, pc.GetId()); !proto.Equal(got, pc) {
		t.Error("a refused level-up changed the character")
	}

	// A subclass that is not the character's (it has its own, chosen at level 2).
	ch := pensantusLevelUp()
	ch.SubclassKey = "subclass:life"
	_, err := tb.levelUp(tb.owner, pc, ch)
	wantCode(t, "a subclass of another class", err, connect.CodeInvalidArgument)

	// A good level-up leaves everything else byte for byte as it was.
	up, err := tb.levelUp(tb.owner, pc, pensantusLevelUp())
	if err != nil {
		t.Fatalf("LevelUpCharacter() error = %v", err)
	}
	was, now := proto.CloneOf(pc.GetSheet().GetFull()), proto.CloneOf(up.GetSheet().GetFull())
	for _, f := range []*charactersv1.FullSheet{was, now} {
		f.Classes, f.ExtraAbilityBonuses, f.HitPoints = nil, nil, nil
		f.CantripKeys, f.KnownSpellKeys, f.PreparedSpellKeys = nil, nil, nil
	}
	if !proto.Equal(was, now) {
		t.Errorf("the level-up changed more than it should:\nbefore %v\nafter  %v", was, now)
	}
	if up.GetName() != pc.GetName() || !proto.Equal(up.GetStory(), pc.GetStory()) {
		t.Error("the name or the story changed")
	}
	// The sheet is still locked for the owner.
	_, err = tb.owner.update(t, up, up.GetName(), up.GetSheet())
	wantCode(t, "UpdateCharacter(after the level-up)", err, connect.CodeFailedPrecondition)
	// The master still edits it, as always.
	if _, err := tb.master.update(t, up, up.GetName(), up.GetSheet()); err != nil {
		t.Errorf("the master's UpdateCharacter error = %v", err)
	}
}

// TestMR040_TheRulesRefuseWhatTheLevelDoesNotGive: choices that break a rule of
// the level are `failed_precondition` with the field and a reason code, and
// the sheet is not saved.
func TestMR040_TheRulesRefuseWhatTheLevelDoesNotGive(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 2700)
	pc := tb.pc
	reason := charactersv1.LevelUpRefusalReason_value
	tests := []struct {
		name   string
		mod    func(ch *charactersv1.LevelUpChoices)
		reason string
		field  string
		code   string
	}{
		{"+1 in one ability", func(ch *charactersv1.LevelUpChoices) { ch.AbilityIncrease = &rulesv1.AbilityScores{Intelligence: 1} }, "ABILITY_SHAPE", "full.extra_ability_bonuses", ""},
		{"+3 in total", func(ch *charactersv1.LevelUpChoices) {
			ch.AbilityIncrease = &rulesv1.AbilityScores{Intelligence: 2, Wisdom: 1}
		}, "ABILITY_SHAPE", "full.extra_ability_bonuses", ""},
		{"no new cantrip", func(ch *charactersv1.LevelUpChoices) { ch.CantripKeys = nil }, "CANTRIPS", "full.cantrip_keys", ""},
		{"one spell short", func(ch *charactersv1.LevelUpChoices) { ch.KnownSpellKeys = ch.KnownSpellKeys[:1] }, "SPELLS", "full.known_spell_keys", ""},
		{"a cleric spell in the book", func(ch *charactersv1.LevelUpChoices) {
			ch.KnownSpellKeys, ch.PreparedSpellKeys = []string{"spell:misty-step", "spell:aid"}, nil
		}, "SPELLS", "full.known_spell_keys", ""},
		{"a 3rd-circle spell", func(ch *charactersv1.LevelUpChoices) {
			ch.KnownSpellKeys, ch.PreparedSpellKeys = []string{"spell:misty-step", "spell:fireball"}, nil
		}, "SHEET_ISSUE", "full.known_spell_keys[11]", "spell_level"},
		{"too many prepared", func(ch *charactersv1.LevelUpChoices) {
			ch.PreparedSpellKeys = append(ch.PreparedSpellKeys, "spell:find-familiar", "spell:detect-magic", "spell:comprehend-languages")
		}, "SHEET_ISSUE", "full.prepared_spell_keys", "spell_count"},
		{"a typed die above the die", func(ch *charactersv1.LevelUpChoices) {
			ch.HitPoints = &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_ROLLED_PHYSICAL, Value: 7}
		}, "HIT_POINTS", "choices.hit_points.value", ""},
		{"a typed die of zero", func(ch *charactersv1.LevelUpChoices) {
			ch.HitPoints = &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_ROLLED_PHYSICAL, Value: 0}
		}, "HIT_POINTS", "choices.hit_points.value", ""},
		{"an in-app roll never made", func(ch *charactersv1.LevelUpChoices) {
			ch.HitPoints = &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_ROLLED_IN_APP}
		}, "HIT_POINT_ROLL_MISSING", "choices.hit_points", ""},
	}
	for _, tt := range tests {
		ch := pensantusLevelUp()
		tt.mod(ch)
		_, err := tb.levelUp(tb.owner, pc, ch)
		r := refusal(t, tt.name, err)
		if r.GetReason() != charactersv1.LevelUpRefusalReason(reason["LEVEL_UP_REFUSAL_REASON_"+tt.reason]) || r.GetField() != tt.field || r.GetIssueCode() != tt.code {
			t.Errorf("%s: refusal = %v, want %s on %s (%s)", tt.name, r, tt.reason, tt.field, tt.code)
		}
		// The preview says the same, as data.
		p, err := tb.preview(tb.owner, pc, ch)
		if err != nil || p.GetRefusal().GetReason() != r.GetReason() {
			t.Errorf("%s: preview = %v, %v; want the same refusal", tt.name, p.GetRefusal(), err)
		}
	}

	// Malformed input is invalid_argument.
	for name, mod := range map[string]func(ch *charactersv1.LevelUpChoices){
		"no hit points method": func(ch *charactersv1.LevelUpChoices) { ch.HitPoints = nil },
		"an unknown spell":     func(ch *charactersv1.LevelUpChoices) { ch.CantripKeys = []string{"spell:not-a-spell"} },
		"a spell twice": func(ch *charactersv1.LevelUpChoices) {
			ch.KnownSpellKeys = []string{"spell:misty-step", "spell:misty-step"}
		},
		"a list too long": func(ch *charactersv1.LevelUpChoices) {
			ch.CantripKeys = slices.Repeat([]string{"spell:light"}, 31)
		},
		"a class that is not there": func(ch *charactersv1.LevelUpChoices) { ch.ClassKey = "class:fighter" },
		"an unknown method": func(ch *charactersv1.LevelUpChoices) {
			ch.HitPoints = &charactersv1.LevelUpHitPoints{Method: 99}
		},
	} {
		ch := pensantusLevelUp()
		mod(ch)
		_, err := tb.levelUp(tb.owner, pc, ch)
		wantCode(t, name, err, connect.CodeInvalidArgument)
	}
	_, err := tb.owner.api.LevelUpCharacter(t.Context(), connect.NewRequest(&charactersv1.LevelUpCharacterRequest{
		CampaignId: tb.campaign, CharacterId: pc.GetId(), Choices: pensantusLevelUp(),
	}))
	wantCode(t, "no revision", err, connect.CodeInvalidArgument)

	if got := tb.owner.get(t, tb.campaign, pc.GetId()); !proto.Equal(got, pc) {
		t.Error("a refused level-up changed the character")
	}
	if tb.live.count() != 0 || len(tb.live.vitals) != 0 {
		t.Error("a refused level-up sent the live hint")
	}
}

// TestMR040_ALevelUpSendsTheNewVitalsToTheOpenSession: the new maximum and
// current hit points go out as the character's vitals once the level-up is
// committed, so an open session shows them without reading again; a refused
// level-up sends none.
func TestMR040_ALevelUpSendsTheNewVitalsToTheOpenSession(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 2700)
	bad := pensantusLevelUp()
	bad.ClassKey = "class:fighter"
	if _, err := tb.levelUp(tb.owner, tb.pc, bad); err == nil {
		t.Fatal("LevelUpCharacter(another class) = nil, want a refusal")
	}
	if len(tb.live.vitals) != 0 {
		t.Fatalf("vitals sent by a refused level-up = %v, want none", tb.live.vitals)
	}
	if _, err := tb.levelUp(tb.owner, tb.pc, pensantusLevelUp()); err != nil {
		t.Fatalf("LevelUpCharacter() error = %v", err)
	}
	if got := tb.live.vitals; !slices.Equal(got, []string{tb.pc.GetId()}) {
		t.Errorf("vitals sent = %v, want the leveled character once", got)
	}
}

// TestMR040_TheHitPointRollIsKept: the server rolls the hit die once for the
// level and returns the same roll afterwards (RN-18); the level-up takes it,
// and the campaign's dice setting decides what else is allowed.
func TestMR040_TheHitPointRollIsKept(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 2700, 6) // the server has only one face to give
	pc := tb.pc
	first, err := tb.roll(tb.owner, pc)
	if err != nil {
		t.Fatalf("RollLevelUpHitPoints() error = %v", err)
	}
	if first.GetDie() != 6 || first.GetValue() != 6 || first.GetAlreadyRolled() {
		t.Errorf("first roll = %v, want a d6 showing 6", first)
	}
	second, err := tb.roll(tb.owner, pc) // another key: still not a reroll
	if err != nil || second.GetValue() != 6 || !second.GetAlreadyRolled() {
		t.Errorf("second roll = %v, %v; want the same 6 again", second, err)
	}
	if o, err := tb.options(tb.owner, pc); err != nil || o.GetKeptHitPointRoll() != 6 {
		t.Errorf("options = %v, %v; want the kept roll 6", o.GetKeptHitPointRoll(), err)
	}
	if _, err := tb.roll(tb.master, pc); err == nil || connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("the master's RollLevelUpHitPoints error = %v, want permission_denied", err)
	}
	_, err = tb.owner.api.RollLevelUpHitPoints(t.Context(), connect.NewRequest(&charactersv1.RollLevelUpHitPointsRequest{CampaignId: tb.campaign, CharacterId: pc.GetId(), IdempotencyKey: "x"}))
	wantCode(t, "RollLevelUpHitPoints(bad key)", err, connect.CodeInvalidArgument)

	// Forced physical dice: no roll in the app, neither a new one nor the kept.
	tb.dice.rule.Store(int32(charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_FORCED_PHYSICAL))
	_, err = tb.roll(tb.owner, pc)
	if blocked(t, "RollLevelUpHitPoints(physical)", err).GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_DICE_FORCED_PHYSICAL {
		t.Error("a roll in the app was allowed with physical dice")
	}
	ch := pensantusLevelUp()
	ch.HitPoints = &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_ROLLED_IN_APP}
	_, err = tb.levelUp(tb.owner, pc, ch)
	if blocked(t, "LevelUpCharacter(in app, physical)", err).GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_DICE_FORCED_PHYSICAL {
		t.Error("the app's roll was taken with physical dice")
	}
	// Forced in-app: a typed result is refused.
	tb.dice.rule.Store(int32(charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_FORCED_IN_APP))
	ch.HitPoints = &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_ROLLED_PHYSICAL, Value: 3}
	_, err = tb.levelUp(tb.owner, pc, ch)
	if blocked(t, "LevelUpCharacter(typed, in app)", err).GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_DICE_FORCED_IN_APP {
		t.Error("a typed result was taken when everybody rolls in the app")
	}

	// The roll is taken: 6 on a d6 and Con +3 is 9 for the level, so 32.
	tb.dice.rule.Store(int32(charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_PLAYER_CHOOSES))
	ch.HitPoints = &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_ROLLED_IN_APP}
	up, err := tb.levelUp(tb.owner, pc, ch)
	if err != nil {
		t.Fatalf("LevelUpCharacter(in app) error = %v", err)
	}
	hp := up.GetSheet().GetFull().GetHitPoints()
	if up.GetDerived().GetHitPointsMax() != 32 || hp.GetMethod() != charactersv1.HitPointsMethod_HIT_POINTS_METHOD_ROLLED || !slices.Equal(hp.GetRolls(), []int32{4, 4, 6}) {
		t.Errorf("hit points = %d, %v; want 32 with rolls [4 4 6]", up.GetDerived().GetHitPointsMax(), hp)
	}
	levelUps, err := tb.levelUps(tb.master, "")
	if err != nil || len(levelUps) != 1 {
		t.Fatalf("ListLevelUps() = %v, %v", levelUps, err)
	}
	if hpr := levelUps[0].GetChoices().GetHitPoints(); hpr.GetMethod() != charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_ROLLED_IN_APP || hpr.GetValue() != 6 {
		t.Errorf("the record's hit points = %v, want the in-app roll of 6", hpr)
	}
	// The used roll is gone from the table.
	var n int
	if err := tb.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM character_level_up_rolls`).Scan(&n); err != nil || n != 0 {
		t.Errorf("%d kept rolls after the level-up, %v; want 0", n, err)
	}
}

// TestMR040_ATypedPhysicalDie: the player types the result of a physical die.
func TestMR040_ATypedPhysicalDie(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 2700)
	tb.dice.rule.Store(int32(charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_FORCED_PHYSICAL))
	ch := pensantusLevelUp()
	ch.HitPoints = &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_ROLLED_PHYSICAL, Value: 2}
	up, err := tb.levelUp(tb.owner, tb.pc, ch)
	if err != nil {
		t.Fatalf("LevelUpCharacter() error = %v", err)
	}
	// 2 + Con +3 = 5 for the level: 23 + 5.
	if got := up.GetDerived().GetHitPointsMax(); got != 28 {
		t.Errorf("hit points = %d, want 28", got)
	}
}

// TestMR040_TheMasterSeesWhatChanged: the master is told by the live hint and
// reads the player's choices with the time, per character; there is no
// approval, and the master edits the sheet as always.
func TestMR040_TheMasterSeesWhatChanged(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 2700)
	pc := tb.pc
	if got, err := tb.levelUps(tb.master, ""); err != nil || len(got) != 0 {
		t.Fatalf("ListLevelUps() before = %v, %v", got, err)
	}
	before := tb.h.clock.Now()
	if _, err := tb.levelUp(tb.owner, pc, pensantusLevelUp()); err != nil {
		t.Fatalf("LevelUpCharacter() error = %v", err)
	}
	if tb.live.count() != 1 || tb.live.calls[0] != tb.campaign {
		t.Errorf("live hints = %v, want one for the campaign", tb.live.calls)
	}
	got, err := tb.levelUps(tb.master, "")
	if err != nil || len(got) != 1 {
		t.Fatalf("ListLevelUps() = %v, %v", got, err)
	}
	l := got[0]
	c := l.GetChoices()
	if l.GetCharacterId() != pc.GetId() || l.GetCharacterName() != "Pensantus" || l.GetPlayerDisplayName() != "Dona" ||
		l.GetClassKey() != "class:wizard" || l.GetClassNamePt() != "Mago" || l.GetFromLevel() != 3 || l.GetToLevel() != 4 {
		t.Errorf("level-up = %v", l)
	}
	if c.GetAbilityIncrease().GetIntelligence() != 2 || !slices.Equal(c.GetCantripKeys(), []string{"spell:mage-hand"}) ||
		!slices.Equal(c.GetKnownSpellKeys(), []string{"spell:misty-step", "spell:invisibility"}) ||
		!slices.Equal(c.GetPreparedSpellKeys(), []string{"spell:misty-step", "spell:invisibility"}) {
		t.Errorf("choices = %v", c)
	}
	if hp := c.GetHitPoints(); hp.GetMethod() != charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE || hp.GetValue() != 4 {
		t.Errorf("hit points = %v, want the average, 4", hp)
	}
	if l.GetNamesPt()["spell:misty-step"] == "" || l.GetNamesPt()["class:wizard"] != "Mago" {
		t.Errorf("names = %v", l.GetNamesPt())
	}
	if l.GetCreatedAt().AsTime().Before(before) {
		t.Errorf("created at %v, before the call", l.GetCreatedAt().AsTime())
	}
	// Per character, and only the master reads.
	if only, err := tb.levelUps(tb.master, pc.GetId()); err != nil || len(only) != 1 {
		t.Errorf("ListLevelUps(character) = %v, %v", only, err)
	}
	if none, err := tb.levelUps(tb.master, "6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e0ff"); err != nil || len(none) != 0 {
		t.Errorf("ListLevelUps(another character) = %v, %v", none, err)
	}
	_, err = tb.levelUps(tb.owner, "")
	wantCode(t, "ListLevelUps(owner)", err, connect.CodePermissionDenied)
	// No approval and no veto: the master edits the sheet as always.
	up := tb.master.get(t, tb.campaign, pc.GetId())
	if _, err := tb.master.update(t, up, "Pensantus, o Sábio", up.GetSheet()); err != nil {
		t.Errorf("the master's UpdateCharacter error = %v", err)
	}
}

// TestMR040_StaleRevision: a level-up made on an old copy of the sheet is
// `aborted`, and changes nothing.
func TestMR040_StaleRevision(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 2700)
	old := tb.pc
	// The master changes the sheet (the revision goes up) before the player confirms.
	if _, err := tb.master.update(t, old, "Pensantus, o Sábio", old.GetSheet()); err != nil {
		t.Fatalf("UpdateCharacter() error = %v", err)
	}
	_, err := tb.levelUp(tb.owner, old, pensantusLevelUp())
	wantCode(t, "LevelUpCharacter(stale)", err, connect.CodeAborted)
	now := tb.master.get(t, tb.campaign, old.GetId())
	if now.GetSheet().GetFull().GetClasses()[0].GetLevel() != 3 || tb.live.count() != 0 {
		t.Error("a stale level-up changed the sheet")
	}
	// With the current revision it goes through.
	if _, err := tb.levelUp(tb.owner, now, pensantusLevelUp()); err != nil {
		t.Errorf("LevelUpCharacter(current) error = %v", err)
	}
}

// TestMR040_ADeadCharacterDoesNotLevelUp: the character died: everything is
// refused with CHARACTER_DEAD.
func TestMR040_ADeadCharacterDoesNotLevelUp(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 2700)
	dead := tb.master.markDead(t, tb.pc)
	reason := charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CHARACTER_DEAD
	_, err := tb.options(tb.owner, dead)
	if blocked(t, "GetLevelUpOptions", err).GetReason() != reason {
		t.Error("GetLevelUpOptions did not say the character is dead")
	}
	_, err = tb.roll(tb.owner, dead)
	if blocked(t, "RollLevelUpHitPoints", err).GetReason() != reason {
		t.Error("RollLevelUpHitPoints did not say the character is dead")
	}
	_, err = tb.levelUp(tb.owner, dead, pensantusLevelUp())
	if blocked(t, "LevelUpCharacter", err).GetReason() != reason {
		t.Error("LevelUpCharacter did not say the character is dead")
	}
}

// TestMR040_AFighterWithNothingToChoose: Toren at Fighter 4 to 5 has only the
// hit points to decide (the guided flow skips every other step).
func TestMR040_AFighterWithNothingToChoose(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 0)
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores:        &rulesv1.AbilityScores{Strength: 15, Dexterity: 12, Constitution: 14, Intelligence: 8, Wisdom: 10, Charisma: 10},
		RaceKey:           "race:human",
		Background:        &charactersv1.FullSheet_BackgroundKey{BackgroundKey: "background:acolyte"},
		Classes:           []*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: 4, Subclass: &charactersv1.ClassLevel_SubclassKey{SubclassKey: "subclass:champion"}}},
		ArmorKey:          "equipment:chain-mail",
		WeaponKeys:        []string{"equipment:longsword"},
		FeatureChoiceKeys: []string{"feature:fighter-fighting-style-defense"},
		ExperiencePoints:  6500, // level 5
	}}}
	toren := tb.other.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Toren", sheet)
	tb.h.lockSheets(tb.campaign)
	toren = tb.master.get(t, tb.campaign, toren.GetId())
	o, err := tb.options(tb.other, toren)
	if err != nil {
		t.Fatalf("GetLevelUpOptions() error = %v", err)
	}
	if o.GetAbilityScoreImprovement() || o.GetSubclassDue() || o.GetCantrips() != 0 || o.GetSpells() != 0 || o.GetPrepares() || len(o.GetFeatureChoices()) != 0 ||
		o.GetSkillChoices() != 0 || o.GetExpertiseChoices() != 0 || o.GetHitDie() != 10 {
		t.Errorf("Fighter 4 to 5 = %v, want only the hit points", o)
	}
	up, err := tb.levelUp(tb.other, toren, &charactersv1.LevelUpChoices{
		ClassKey: "class:fighter", HitPoints: &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
	})
	if err != nil {
		t.Fatalf("LevelUpCharacter() error = %v", err)
	}
	if up.GetDerived().GetTotalLevel() != 5 || up.GetDerived().GetProficiencyBonus() != 3 {
		t.Errorf("Toren = level %d, proficiency +%d, want level 5, +3", up.GetDerived().GetTotalLevel(), up.GetDerived().GetProficiencyBonus())
	}
}

// TestMR040_OneRollPerLevelNotPerClass: a multiclass character rolls once for
// the level; the roll serves only the class that rolled it.
func TestMR040_OneRollPerLevelNotPerClass(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 0, 5)
	sheet := pensantusSheet()
	sheet.GetFull().ExperiencePoints = 6500 // level 5
	sheet.GetFull().Classes = append(sheet.GetFull().Classes, &charactersv1.ClassLevel{ClassKey: "class:fighter", Level: 1})
	pc := tb.other.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Multi", sheet)
	pc = tb.other.get(t, tb.campaign, pc.GetId())
	rollAs := func(class string) (*charactersv1.RollLevelUpHitPointsResponse, error) {
		res, err := tb.other.api.RollLevelUpHitPoints(t.Context(), connect.NewRequest(&charactersv1.RollLevelUpHitPointsRequest{
			CampaignId: tb.campaign, CharacterId: pc.GetId(), ClassKey: class, IdempotencyKey: uuid.New().String(),
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	first, err := rollAs("class:wizard")
	if err != nil || first.GetDie() != 6 || first.GetValue() != 5 {
		t.Fatalf("first roll = %v, %v; want a d6 showing 5", first, err)
	}
	_, err = rollAs("class:fighter")
	if r := refusal(t, "the other class's roll", err); r.GetReason() != charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_HIT_POINT_ROLL_OTHER_CLASS {
		t.Errorf("refusal = %v, want HIT_POINT_ROLL_OTHER_CLASS", r)
	}
	if again, err := rollAs("class:wizard"); err != nil || !again.GetAlreadyRolled() || again.GetValue() != 5 {
		t.Errorf("the same class again = %v, %v; want the kept 5", again, err)
	}
	// The options say who rolled, and the fighter's level-up cannot take it.
	res, err := tb.other.api.GetLevelUpOptions(t.Context(), connect.NewRequest(&charactersv1.GetLevelUpOptionsRequest{CampaignId: tb.campaign, CharacterId: pc.GetId(), ClassKey: "class:fighter"}))
	if err != nil || res.Msg.GetOptions().GetKeptHitPointRoll() != 5 || res.Msg.GetOptions().GetKeptHitPointRollClassKey() != "class:wizard" {
		t.Errorf("options = %v, %v; want the wizard's kept 5", res.Msg.GetOptions(), err)
	}
	_, err = tb.levelUp(tb.other, pc, &charactersv1.LevelUpChoices{
		ClassKey: "class:fighter", HitPoints: &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_ROLLED_IN_APP},
	})
	if r := refusal(t, "LevelUpCharacter(other class's roll)", err); r.GetReason() != charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_HIT_POINT_ROLL_OTHER_CLASS {
		t.Errorf("refusal = %v, want HIT_POINT_ROLL_OTHER_CLASS", r)
	}
}

// TestMR040_DuplicatesAreInvalid: a key twice in a list of the choices is
// malformed, and one the sheet already has is refused by the sheet's own check.
func TestMR040_DuplicatesAreInvalid(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 2700)
	for name, mod := range map[string]func(ch *charactersv1.LevelUpChoices){
		"a spell twice":        func(ch *charactersv1.LevelUpChoices) { ch.KnownSpellKeys = []string{"spell:blur", "spell:blur"} },
		"a spell it has":       func(ch *charactersv1.LevelUpChoices) { ch.KnownSpellKeys = []string{"spell:blur", "spell:shield"} },
		"a prepared one twice": func(ch *charactersv1.LevelUpChoices) { ch.PreparedSpellKeys = []string{"spell:blur", "spell:blur"} },
		"a cantrip it has":     func(ch *charactersv1.LevelUpChoices) { ch.CantripKeys = []string{"spell:fire-bolt"} },
	} {
		ch := pensantusLevelUp()
		mod(ch)
		_, err := tb.levelUp(tb.owner, tb.pc, ch)
		wantCode(t, name, err, connect.CodeInvalidArgument)
	}
}

// TestMR040_AStoredSheetThatFailsToday: a sheet that does not pass today's
// rules by itself is the master's to fix: a typed refusal, not a bare
// invalid_argument.
func TestMR040_AStoredSheetThatFailsToday(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 2700)
	if _, err := tb.h.pool.Exec(t.Context(), `UPDATE characters SET sheet = jsonb_set(sheet, '{full,race_key}', '"race:gone"') WHERE id = $1`, tb.pc.GetId()); err != nil {
		t.Fatalf("break the sheet: %v", err)
	}
	for name, call := range map[string]func() error{
		"GetLevelUpOptions": func() error { _, err := tb.options(tb.owner, tb.pc); return err },
		"LevelUpCharacter":  func() error { _, err := tb.levelUp(tb.owner, tb.pc, pensantusLevelUp()); return err },
	} {
		r := refusal(t, name, call())
		if r.GetReason() != charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SHEET_NEEDS_MASTER || r.GetField() != "full.race_key" {
			t.Errorf("%s: refusal = %v, want SHEET_NEEDS_MASTER on full.race_key", name, r)
		}
	}
}

// TestMR040_ListLevelUpsPages: the list comes newest first, a page at a time.
func TestMR040_ListLevelUpsPages(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 2700)
	var want []string
	for range 5 {
		var id string
		err := tb.h.pool.QueryRow(t.Context(), `
			INSERT INTO character_level_ups (campaign_id, character_id, class_key, from_level, to_level, hp_method, hp_value, choices, created_at)
			VALUES ($1, $2, 'class:wizard', 3, 4, 'average', 4, '{}', $3) RETURNING id`,
			tb.campaign, tb.pc.GetId(), tb.h.clock.Now()).Scan(&id)
		if err != nil {
			t.Fatalf("insert a level-up: %v", err)
		}
		want = append([]string{id}, want...)
	}
	var got []string
	token := ""
	for pages := 0; ; pages++ {
		res, err := tb.master.api.ListLevelUps(t.Context(), connect.NewRequest(&charactersv1.ListLevelUpsRequest{CampaignId: tb.campaign, PageSize: 2, PageToken: token}))
		if err != nil {
			t.Fatalf("ListLevelUps(page %d) error = %v", pages, err)
		}
		for _, l := range res.Msg.GetLevelUps() {
			got = append(got, l.GetId())
		}
		if token = res.Msg.GetNextPageToken(); token == "" {
			break
		}
		if pages > 5 {
			t.Fatal("the list never ends")
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("pages = %v, want %v", got, want)
	}
	for name, req := range map[string]*charactersv1.ListLevelUpsRequest{
		"a bad token":  {CampaignId: tb.campaign, PageToken: "nope"},
		"a page of 51": {CampaignId: tb.campaign, PageSize: 51},
	} {
		_, err := tb.master.api.ListLevelUps(t.Context(), connect.NewRequest(req))
		wantCode(t, name, err, connect.CodeInvalidArgument)
	}
}

// storedHitPoints is the character's stored current hit points: nil when never
// set (full), and whether the vitals row exists.
func (tb *levelUpTable) storedHitPoints(id string) (current *int32, found bool) {
	tb.h.t.Helper()
	err := tb.h.pool.QueryRow(tb.h.t.Context(), "SELECT hit_points_current FROM character_vitals WHERE character_id = $1", id).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false
	}
	if err != nil {
		tb.h.t.Fatalf("read the vitals: %v", err)
	}
	return current, true
}

// setHitPoints writes the character's stored current hit points (nil: never set).
func (tb *levelUpTable) setHitPoints(id string, current *int32) {
	tb.h.t.Helper()
	if _, err := tb.h.pool.Exec(tb.h.t.Context(),
		`INSERT INTO character_vitals (character_id, hit_points_current, revision, updated_at) VALUES ($1, $2, 1, now())
		 ON CONFLICT (character_id) DO UPDATE SET hit_points_current = excluded.hit_points_current`, id, current); err != nil {
		tb.h.t.Fatalf("write the vitals: %v", err)
	}
}

// MR-040, RN-12: the level-up raises the current hit points by what the maximum
// gained: a wound stays a wound, a full character stays full, and a character
// whose hit points were never set stays full without a number being written.
func TestMR040_TheCurrentHitPointsRiseWithTheMaximum(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		current *int32 // before; the maximum is 23 at level 3 and 30 at level 4
		want    *int32
	}{
		{"wounded", new(int32(20)), new(int32(27))},
		{"at the maximum", new(int32(23)), new(int32(30))},
		{"dying", new(int32(0)), new(int32(7))},
		{"never set", nil, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			tb := newLevelUpTable(t, 2700)
			if tb.pc.GetDerived().GetHitPointsMax() != 23 {
				t.Fatalf("maximum before = %d, want 23", tb.pc.GetDerived().GetHitPointsMax())
			}
			tb.setHitPoints(tb.pc.GetId(), c.current)
			up, err := tb.levelUp(tb.owner, tb.pc, pensantusLevelUp())
			if err != nil {
				t.Fatalf("LevelUpCharacter() error = %v", err)
			}
			if up.GetDerived().GetHitPointsMax() != 30 {
				t.Fatalf("maximum after = %d, want 30", up.GetDerived().GetHitPointsMax())
			}
			got, _ := tb.storedHitPoints(tb.pc.GetId())
			if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
				t.Errorf("stored current hit points = %v, want %v", deref32(got), deref32(c.want))
			}
		})
	}
}

// deref32 prints a nullable number.
func deref32(n *int32) string {
	if n == nil {
		return "unset"
	}
	return strconv.Itoa(int(*n))
}

// A level-up of a character with no vitals row writes none: it is full.
func TestMR040_ACharacterWithoutVitalsGetsNone(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 2700)
	if _, err := tb.levelUp(tb.owner, tb.pc, pensantusLevelUp()); err != nil {
		t.Fatalf("LevelUpCharacter() error = %v", err)
	}
	if _, found := tb.storedHitPoints(tb.pc.GetId()); found {
		t.Error("a level-up created a vitals row for a character that was full")
	}
}

// The master's edit of the sheet moves the current hit points the same way: a
// higher maximum lifts a wound's number, a lower one never leaves the current
// above it.
func TestMR040_TheMastersEditMovesTheCurrentHitPointsWithTheMaximum(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 0)
	id := tb.pc.GetId()
	edit := func(constitution int32) *charactersv1.Character {
		t.Helper()
		full := proto.Clone(tb.pc.GetSheet()).(*charactersv1.CharacterSheet)
		full.GetFull().BaseScores.Constitution = constitution
		cur := tb.master.get(t, tb.campaign, id)
		out, err := tb.master.update(t, cur, cur.GetName(), full)
		if err != nil {
			t.Fatalf("master's UpdateCharacter() error = %v", err)
		}
		return out
	}
	start := tb.pc.GetDerived().GetHitPointsMax()

	// Wounded: 6 below the maximum. A higher Constitution lifts both.
	tb.setHitPoints(id, new(start-6))
	raised := edit(18) // Con 15 -> 18: +1 per level
	gain := raised.GetDerived().GetHitPointsMax() - start
	if gain <= 0 {
		t.Fatalf("the maximum went %d -> %d, want a rise", start, raised.GetDerived().GetHitPointsMax())
	}
	if got, _ := tb.storedHitPoints(id); got == nil || *got != start-6+gain {
		t.Errorf("wounded, after a rise of %d: current = %v, want %d", gain, deref32(got), start-6+gain)
	}

	// At the maximum, a lower maximum pulls the current down with it.
	tb.setHitPoints(id, new(raised.GetDerived().GetHitPointsMax()))
	lowered := edit(8)
	if lowered.GetDerived().GetHitPointsMax() >= raised.GetDerived().GetHitPointsMax() {
		t.Fatalf("the maximum did not fall: %d", lowered.GetDerived().GetHitPointsMax())
	}
	if got, _ := tb.storedHitPoints(id); got == nil || *got != lowered.GetDerived().GetHitPointsMax() {
		t.Errorf("at the maximum, after a fall: current = %v, want %d", deref32(got), lowered.GetDerived().GetHitPointsMax())
	}

	// A wound below the lower maximum is left alone (the fall is not charged to it).
	tb.setHitPoints(id, new(int32(3)))
	edit(6)
	if got, _ := tb.storedHitPoints(id); got == nil || *got != 3 {
		t.Errorf("a wound below the fallen maximum: current = %v, want 3", deref32(got))
	}

	// Never set stays never set.
	tb.setHitPoints(id, nil)
	edit(15)
	if got, _ := tb.storedHitPoints(id); got != nil {
		t.Errorf("never set: current = %v, want unset", deref32(got))
	}
}

// afterQuery runs hook once, right after the first statement whose text
// contains match finishes. The statements the hook itself makes pass.
type afterQuery struct {
	match string
	hook  func()
	fired atomic.Bool
}

type queryTextKey struct{}

func (*afterQuery) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, queryTextKey{}, data.SQL)
}

func (a *afterQuery) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if sql, _ := ctx.Value(queryTextKey{}).(string); strings.Contains(sql, a.match) && a.fired.CompareAndSwap(false, true) {
		// From a goroutine of its own: a call that reaches this service again, or a
		// write through another pool, from inside a transaction's closure would look
		// like a nested acquisition.
		done := make(chan struct{})
		go func() {
			defer close(done)
			a.hook()
		}()
		<-done
	}
}

// hookAfterQuery makes the service read through a pool of its own whose
// statements run hook once, after the first one that contains match: what the
// hook commits lands exactly between two statements of a read.
func (h *harness) hookAfterQuery(match string, hook func()) {
	h.t.Helper()
	pool, err := db.NewPoolWith(h.t.Context(), h.pool.Config().ConnString(), func(cfg *pgxpool.Config) {
		cfg.MaxConns = 2
		cfg.ConnConfig.Tracer = &afterQuery{match: match, hook: hook}
	})
	if err != nil {
		h.t.Fatalf("NewPoolWith() error = %v", err)
	}
	h.t.Cleanup(pool.Close)
	h.svc.pool, h.svc.queries = pool, charactersdb.New(pool)
}

// The level-up options are one moment: a player who levels up while the page
// is being read never makes it offer the level just taken with its kept hit
// point roll gone.
func TestMR040_TheOptionsAreOneSnapshotWhileThePlayerLevelsUp(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 2700, 6)
	if _, err := tb.roll(tb.owner, tb.pc); err != nil {
		t.Fatalf("RollLevelUpHitPoints() error = %v", err)
	}
	ch := pensantusLevelUp()
	ch.HitPoints = &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_ROLLED_IN_APP}
	tb.h.hookAfterQuery("FROM characters", func() {
		if _, err := tb.levelUp(tb.owner, tb.pc, ch); err != nil {
			t.Errorf("LevelUpCharacter() from the hook error = %v", err)
		}
	})
	o, err := tb.options(tb.owner, tb.pc)
	// As it was: the level and the roll the player had. As it became: no level left to take.
	if err == nil && o.GetKeptHitPointRoll() != 6 {
		t.Errorf("GetLevelUpOptions(): level %d to %d with the kept roll %d; want the options as they were, with the roll 6, or a refusal", o.GetFromLevel(), o.GetToLevel(), o.GetKeptHitPointRoll())
	}

	// Positive control: the hook did level the character up.
	if got := tb.owner.get(t, tb.campaign, tb.pc.GetId()); got.GetDerived().GetTotalLevel() != 4 {
		t.Errorf("level after the hook = %d, want 4", got.GetDerived().GetTotalLevel())
	}
}

// The same for the preview: the roll the plan keeps and the character come
// from one moment, so a level-up that commits between them never makes the
// preview refuse a roll as missing.
func TestMR040_ThePreviewIsOneSnapshotWhileThePlayerLevelsUp(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 2700, 6)
	if _, err := tb.roll(tb.owner, tb.pc); err != nil {
		t.Fatalf("RollLevelUpHitPoints() error = %v", err)
	}
	ch := pensantusLevelUp()
	ch.HitPoints = &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_ROLLED_IN_APP}
	tb.h.hookAfterQuery("FROM characters", func() {
		if _, err := tb.levelUp(tb.owner, tb.pc, ch); err != nil {
			t.Errorf("LevelUpCharacter() from the hook error = %v", err)
		}
	})
	p, err := tb.preview(tb.owner, tb.pc, ch)
	if err == nil && p.GetRefusal() != nil {
		t.Errorf("PreviewLevelUp() refused %v; want the preview as it was, with the roll kept, or an error", p.GetRefusal())
	}
	if got := tb.owner.get(t, tb.campaign, tb.pc.GetId()); got.GetDerived().GetTotalLevel() != 4 {
		t.Errorf("level after the hook = %d, want 4", got.GetDerived().GetTotalLevel())
	}
}
