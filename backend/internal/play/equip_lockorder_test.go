package play

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The lock order of the gear paths. A combat write takes the session's row first (Service.tx)
// and then reaches the character's row (an event that names the character key-shares it). The
// gear paths of the characters module (SetEquipped, and a save of the sheet that changes the
// armor or the shield) run inside the character's own transaction; they must also take the
// session's row before the character's, or an equip and a combat write that touch the same
// character each hold what the other wants (40P01, which db.InTx retries only after the
// database's deadlock timeout, a second at least).

// lockSpy is the review host the characters service sees: play's, noting every deadlock a
// host call is aborted with (a retry hides it from the caller, so the call's error is read here).
type lockSpy struct {
	characters.ReviewHost
	deadlocks atomic.Int32
}

func (l *lockSpy) note(err error) {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "40P01" {
		l.deadlocks.Add(1)
	}
}

func (l *lockSpy) EquipInCombat(ctx context.Context, tx pgx.Tx, campaignID, characterID, actorUserID string, change characters.EquipChange, t sessiontell.Tell) error {
	err := l.ReviewHost.EquipInCombat(ctx, tx, campaignID, characterID, actorUserID, change, t)
	l.note(err)
	return err
}

func (l *lockSpy) GearChanged(ctx context.Context, tx pgx.Tx, campaignID, characterID string, t sessiontell.Tell) error {
	err := l.ReviewHost.GearChanged(ctx, tx, campaignID, characterID, t)
	l.note(err)
	return err
}

func (l *lockSpy) ArmorWorn(ctx context.Context, tx pgx.Tx, campaignID, characterID string, t sessiontell.Tell) error {
	err := l.ReviewHost.ArmorWorn(ctx, tx, campaignID, characterID, t)
	l.note(err)
	return err
}

func (l *lockSpy) ReviveInCombat(ctx context.Context, tx pgx.Tx, campaignID, characterID, actorUserID string, at time.Time) (string, error) {
	enc, err := l.ReviewHost.ReviveInCombat(ctx, tx, campaignID, characterID, actorUserID, at)
	l.note(err)
	return enc, err
}

// gearRace is one gear path to race against a combat write on the same character.
type gearRace struct {
	a           *armed
	characterID string
	act         func() error // the path under test, as its caller calls it
	verify      func(t *testing.T)
}

// sheetEdit saves the character's sheet changed by edit, as the master.
func (a *armed) sheetEdit(t *testing.T, c *charactersv1.Character, edit func(*charactersv1.FullSheet)) func() error {
	t.Helper()
	cur := a.ana.character(t, c)
	sheet := cur.GetSheet()
	edit(sheet.GetFull())
	return func() error {
		_, err := a.master.characters.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
			CampaignId: a.campaignID, CharacterId: cur.GetId(), Revision: cur.GetRevision(), Name: cur.GetName(), Sheet: sheet,
		}))
		return err
	}
}

func equipRace(t *testing.T) gearRace {
	t.Helper()
	a, e := equipFight(t)
	a.passTo(t, e, "Reator")
	return gearRace{
		a: a, characterID: a.pens.GetId(),
		act: func() error { _, err := a.swap(a.ana, a.pens, longswordKey, false); return err },
		verify: func(t *testing.T) {
			t.Helper()
			if got := a.carriedOf(t, a.pens); len(got) != 1 || got[0] != longswordKey {
				t.Errorf("carried = %v, want the longsword stowed", got)
			}
			a.wantTurn(t, "after the stow", true, false)
		},
	}
}

func shieldRace(t *testing.T) gearRace {
	t.Helper()
	a := newCastingParty(t)
	a.mageArmorOnPensantus(t)
	return gearRace{
		a: a, characterID: a.pens.GetId(),
		act: a.sheetEdit(t, a.pens, func(f *charactersv1.FullSheet) { f.Shield = true }),
		verify: func(t *testing.T) {
			t.Helper()
			if got := a.vitals(t, a.pens).GetArmorClassBase(); got != 17 {
				t.Errorf("the base armor class with the shield = %d, want 17", got)
			}
		},
	}
}

func armorRace(t *testing.T) gearRace {
	t.Helper()
	a := newCastingParty(t)
	a.mageArmorOnPensantus(t)
	return gearRace{
		a: a, characterID: a.pens.GetId(),
		act: a.sheetEdit(t, a.pens, func(f *charactersv1.FullSheet) { f.ArmorKey = leatherArmorKey }),
		verify: func(t *testing.T) {
			t.Helper()
			if _, ok := a.armorEffectSeconds(t, a.pens.GetId()); ok {
				t.Error("Mage Armor stayed on a character that donned armor")
			}
		},
	}
}

// TestGearPathsTakeTheSessionBeforeTheCharacter holds a combat write at the session's lock and has
// it write an event that names the character, with a gear path of the same character started
// meanwhile. In the old order the gear path holds the character and waits for the session while
// the combat write waits for the character: a deadlock, which the database breaks after its
// timeout. In the right order the gear path holds nothing while it waits, the combat write
// finishes first, and the gear path runs after it with no deadlock and no lost write.
func TestGearPathsTakeTheSessionBeforeTheCharacter(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		make func(*testing.T) gearRace
	}{
		{"SetEquipped in a combat", equipRace},
		{"a shield put on", shieldRace},
		{"armor donned", armorRace},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runGearRace(t, tt.make(t))
		})
	}
}

func reviveRace(t *testing.T) gearRace {
	t.Helper()
	a := newCastingParty(t)
	a.execSQL(t, `UPDATE characters SET status = 'dead', died_at = now() WHERE id = $1`, a.toren.GetId())
	return gearRace{
		a: a, characterID: a.toren.GetId(),
		act: func() error {
			_, err := a.master.characters.ReviveCharacter(t.Context(), connect.NewRequest(&charactersv1.ReviveCharacterRequest{
				CampaignId: a.campaignID, CharacterId: a.toren.GetId(), IdempotencyKey: newKey(),
			}))
			return err
		},
		verify: func(t *testing.T) {
			t.Helper()
			if got := a.vitals(t, a.toren).GetHitPointsCurrent(); got != 1 {
				t.Errorf("the revived character's hit points = %d, want 1", got)
			}
		},
	}
}

// TestReviveTakesTheSessionBeforeTheCharacter is the same race for ReviveCharacter: it locks the
// character, writes it back to life, and then reaches play (ReviveInCombat), which takes the session's row.
func TestReviveTakesTheSessionBeforeTheCharacter(t *testing.T) {
	t.Parallel()
	runGearRace(t, reviveRace(t))
}

// choicesRace is CompleteCharacterChoices of a fighter whose fighting style is open (the master took the pick out of its sheet).
// The call writes the sheet and then the master's history line of the open session, which takes the session's row: the character first
// and the session second is the reverse of every write of the funnel.
func choicesRace(t *testing.T) gearRace {
	t.Helper()
	const style = "feature:fighter-fighting-style"
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: &rulesv1.AbilityScores{Strength: 15, Dexterity: 14, Constitution: 13, Intelligence: 10, Wisdom: 12, Charisma: 8},
		RaceKey:    "race:human", Classes: []*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: 1}},
		Background: &charactersv1.FullSheet_BackgroundKey{BackgroundKey: "background:acolyte"},
		Languages:  []string{"Anão", "Élfico", "Gnômico"}, FeatureChoiceKeys: []string{"feature:fighter-fighting-style-defense"},
	}}}
	a := newArmedWith(t, func(a *armed) {
		made, err := a.ana.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
			CampaignId: a.campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Ardis", Sheet: sheet,
		}))
		if err != nil {
			t.Fatalf("CreateCharacter(Ardis) error = %v", err)
		}
		// The way a sheet written before the choices were asked has its style open: the pick is taken out of the stored sheet.
		a.execSQL(t, `UPDATE characters SET sheet = sheet #- '{full,feature_choice_keys}' WHERE id = $1`, made.Msg.GetCharacter().GetId())
		a.toren = a.ana.character(t, made.Msg.GetCharacter())
	})
	return gearRace{
		a: a, characterID: a.toren.GetId(),
		act: func() error {
			_, err := a.ana.characters.CompleteCharacterChoices(t.Context(), connect.NewRequest(&charactersv1.CompleteCharacterChoicesRequest{
				CampaignId: a.campaignID, CharacterId: a.toren.GetId(), ExpectedRevision: a.toren.GetRevision(),
				Picks: []*charactersv1.ChoicePick{{ChoiceKey: style, OptionKeys: []string{"feature:fighter-fighting-style-archery"}}},
			}))
			return err
		},
		verify: func(t *testing.T) {
			t.Helper()
			if got := a.ana.character(t, a.toren).GetSheet().GetFull().GetFeatureChoiceKeys(); !slices.Contains(got, "feature:fighter-fighting-style-archery") {
				t.Errorf("the sheet's picks = %v, want the style chosen", got)
			}
		},
	}
}

// levelUpRace is LevelUpCharacter of a wizard with the XP of the next level, a sheet that is locked when the session opens.
// Like the other writes of the sheet, the level-up writes the character and then the master's history line of the open session,
// so the session's row has to come first.
func levelUpRace(t *testing.T) gearRace {
	t.Helper()
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: &rulesv1.AbilityScores{Strength: 12, Dexterity: 16, Constitution: 15, Intelligence: 16, Wisdom: 13, Charisma: 12},
		RaceKey:    "race:gnome", SubraceKey: "subrace:rock-gnome",
		Classes: []*charactersv1.ClassLevel{{
			ClassKey: "class:wizard", Level: 3, Subclass: &charactersv1.ClassLevel_SubclassKey{SubclassKey: "subclass:evocation"},
		}},
		Background: &charactersv1.FullSheet_CustomBackground{CustomBackground: &charactersv1.CustomBackground{
			Name: "Sábio", SkillKeys: []string{"skill:arcana", "skill:history"},
			ProficiencyKeys: []string{"language:draconic", "language:elvish"},
			FeatureName:     "Pesquisador", FeatureText: "Quando você não sabe uma informação, sabe a quem perguntar.",
			Equipment: "Um tinteiro, uma pena e roupas comuns.",
		}},
		SkillProficiencyKeys: []string{"skill:investigation", "skill:insight"},
		WeaponKeys:           []string{"equipment:quarterstaff"},
		CantripKeys:          []string{"spell:fire-bolt", "spell:ray-of-frost", "spell:minor-illusion"},
		KnownSpellKeys: []string{
			"spell:magic-missile", "spell:burning-hands", "spell:shield", "spell:mage-armor", "spell:sleep",
			"spell:find-familiar", "spell:detect-magic", "spell:comprehend-languages", "spell:scorching-ray", "spell:web",
		},
		PreparedSpellKeys: []string{
			"spell:magic-missile", "spell:burning-hands", "spell:shield", "spell:mage-armor", "spell:sleep", "spell:scorching-ray", "spell:web",
		},
		Equipment:        []*charactersv1.Item{{Name: "Grimório", Quantity: 1}},
		Languages:        []string{"Dracônico"},
		ExperiencePoints: 2700, // the XP of level 4
		Alignment:        charactersv1.Alignment_ALIGNMENT_NEUTRAL_GOOD,
	}}}
	a := newArmedWith(t, func(a *armed) {
		made, err := a.ana.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
			CampaignId: a.campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Pensantus", Sheet: sheet,
		}))
		if err != nil {
			t.Fatalf("CreateCharacter(Pensantus) error = %v", err)
		}
		a.toren = a.ana.character(t, made.Msg.GetCharacter())
	})
	return gearRace{
		a: a, characterID: a.toren.GetId(),
		act: func() error {
			_, err := a.ana.characters.LevelUpCharacter(t.Context(), connect.NewRequest(&charactersv1.LevelUpCharacterRequest{
				CampaignId: a.campaignID, CharacterId: a.toren.GetId(), Revision: a.toren.GetRevision(),
				Choices: &charactersv1.LevelUpChoices{
					ClassKey:          "class:wizard",
					AbilityIncrease:   &rulesv1.AbilityScores{Intelligence: 2},
					CantripKeys:       []string{"spell:mage-hand"},
					KnownSpellKeys:    []string{"spell:misty-step", "spell:invisibility"},
					PreparedSpellKeys: []string{"spell:misty-step", "spell:invisibility"},
					HitPoints:         &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
				},
			}))
			return err
		},
		verify: func(t *testing.T) {
			t.Helper()
			if got := a.ana.character(t, a.toren).GetDerived().GetTotalLevel(); got != 4 {
				t.Errorf("the level = %d, want 4", got)
			}
		},
	}
}

// TestLevelUpTakesTheSessionBeforeTheCharacter is the same race for LevelUpCharacter.
func TestLevelUpTakesTheSessionBeforeTheCharacter(t *testing.T) {
	t.Parallel()
	runGearRace(t, levelUpRace(t))
}

// TestCompleteChoicesTakeTheSessionBeforeTheCharacter is the same race for CompleteCharacterChoices: it locks the character, writes
// its sheet, and then appends the history line of the session (AppendEvent), which takes the session's row.
func TestCompleteChoicesTakeTheSessionBeforeTheCharacter(t *testing.T) {
	t.Parallel()
	runGearRace(t, choicesRace(t))
}

// waitForSessionLock waits, on a condition and not on a clock, until a statement waits for a
// session's row lock.
func waitForSessionLock(t *testing.T, observer *pgxpool.Pool, done <-chan error) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		n, err := dbtest.WaitingStatements(t.Context(), observer, `%FROM game_sessions%FOR UPDATE%`)
		if err != nil {
			t.Fatalf("look for the waiting gear path: %v", err)
		}
		if n > 0 {
			return
		}
		select {
		case err := <-done:
			t.Fatalf("the gear path returned (%v) while the session was locked", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("the gear path never waited for the session's lock")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func runGearRace(t *testing.T, g gearRace) {
	t.Helper()
	ctx := t.Context()
	a := g.a
	spy := &lockSpy{ReviewHost: a.h.svc}
	a.h.chars.SetReviewHost(spy)

	var sessionID string
	if err := a.h.pool.QueryRow(ctx, `SELECT id FROM game_sessions WHERE campaign_id = $1 AND ended_at IS NULL`, a.campaignID).Scan(&sessionID); err != nil {
		t.Fatalf("read the open session: %v", err)
	}
	side := dbtest.SideConnection(t, a.h.pool)
	observer := dbtest.SideConnection(t, a.h.pool)

	// The combat write: the funnel takes the session first; the write then names the character.
	held, gate := make(chan struct{}), make(chan struct{})
	var open sync.Once
	release := func() { open.Do(func() { close(gate) }) }
	t.Cleanup(release) // a failed check must not leave the combat write holding the session's row
	var attempts atomic.Int32
	combat := make(chan error, 1)
	go func() {
		combat <- a.h.svc.txOn(ctx, side, sessionID, func(tx pgx.Tx, h *hints) error {
			if attempts.Add(1) == 1 {
				close(held)
				<-gate
			}
			q := a.h.svc.queries.WithTx(tx)
			seq, err := q.NextSessionEventSeq(ctx, sessionID)
			if err != nil {
				return err
			}
			if _, err := insertSessionEvent(ctx, q, playdb.InsertSessionEventParams{
				GameSessionID: sessionID, Seq: seq, Kind: eventHitPointsAdjusted, CharacterID: &g.characterID,
				Payload: []byte(`{}`), CreatedAt: a.h.clock.Now(),
			}); err != nil {
				return err
			}
			h.touch()
			return nil
		})
	}()
	<-held

	gear := make(chan error, 1)
	go func() { gear <- g.act() }()
	waitForSessionLock(t, observer, gear)

	// The gear path waits for the session: it must not hold the character meanwhile.
	probe, err := observer.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if _, err := probe.Exec(ctx, `SELECT id FROM characters WHERE id = $1 FOR UPDATE NOWAIT`, g.characterID); err != nil {
		t.Errorf("the character is locked while the gear path waits for the session: %v", err)
	}
	_ = probe.Rollback(ctx)

	release()
	for _, r := range []struct {
		what string
		ch   chan error
	}{{"the combat write", combat}, {"the gear path", gear}} {
		select {
		case err := <-r.ch:
			if err != nil {
				t.Errorf("%s error = %v", r.what, err)
			}
		case <-time.After(30 * time.Second):
			t.Fatalf("%s never finished", r.what)
		}
	}
	if n := spy.deadlocks.Load(); n != 0 {
		t.Errorf("the gear path was aborted by %d deadlock(s) (40P01)", n)
	}
	if n := attempts.Load(); n != 1 {
		t.Errorf("the combat write ran %d times, want 1 (it was a deadlock victim)", n)
	}
	var events int
	if err := a.h.pool.QueryRow(ctx, `SELECT count(*) FROM session_events WHERE game_session_id = $1 AND kind = $2 AND character_id = $3`, sessionID, eventHitPointsAdjusted, g.characterID).Scan(&events); err != nil || events != 1 {
		t.Errorf("the combat write's events = %d (%v), want 1", events, err)
	}
	g.verify(t)
}
