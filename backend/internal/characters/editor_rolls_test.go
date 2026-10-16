package characters

import (
	"testing"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
)

// The characterization of RollAbilityScores: who may roll, the refusals with their code,
// message and detail, the sets stored once and answered again, and the failures of the
// server.

// editorRolls renders an answer of RollAbilityScores, or its refusal.
func editorRolls(res *charactersv1.RollAbilityScoresResponse, err error) string {
	if err != nil {
		return editorRefusal(err)
	}
	return editorMessage(res.ProtoReflect())
}

func TestRollAbilityScoresEditorAnswers(t *testing.T) {
	t.Parallel()
	faces := []int{6, 5, 4, 1, 6, 6, 6, 6, 1, 1, 1, 1, 2, 3, 4, 5, 3, 3, 3, 3, 5, 5, 5, 2}
	et := newEditorTable(t, func(c *Config) { c.Roller = &dice.Fixed{Faces: faces} })
	typed := []int32{6, 5, 4, 1}
	other := []int32{6, 5, 4, 2}
	steps := []struct {
		name  string
		as    *user
		typed [][]int32
		want  string
	}{
		{"nobody signed in", et.h.anonymous(), nil, `unauthenticated: sign in to continue`},
		{"a stranger", et.visitor, nil, `not_found: campaign not found`},
		{"the master", et.master, nil, `permission_denied: only a player makes a sheet's ability scores; the master's NPCs are free`},
		{"five typed sets", et.dona, [][]int32{typed, typed, typed, typed, typed}, `invalid_argument: typed_dice an ability roll has six sets of dice | InvalidField{field="typed_dice"}`},
		{"a typed 7", et.dona, [][]int32{typed, typed, typed, typed, typed, {7, 1, 1, 1}}, `invalid_argument: typed_dice each die of an ability roll is 1 to 6 | InvalidField{field="typed_dice"}`},
		{"the app rolls for Dona", et.dona, nil, `RollAbilityScoresResponse{rolls=AbilityRolls{sets=[AbilityRollSet{dice=[6, 5, 4, 1] total=15}, AbilityRollSet{dice=[6, 6, 6, 6] total=18}, AbilityRollSet{dice=[1, 1, 1, 1] total=3}, AbilityRollSet{dice=[2, 3, 4, 5] total=12}, AbilityRollSet{dice=[3, 3, 3, 3] total=9}, AbilityRollSet{dice=[5, 5, 5, 2] total=15}] rolled_at=<time>}}`},
		{"Dona asks again", et.dona, nil, `RollAbilityScoresResponse{rolls=AbilityRolls{sets=[AbilityRollSet{dice=[6, 5, 4, 1] total=15}, AbilityRollSet{dice=[6, 6, 6, 6] total=18}, AbilityRollSet{dice=[1, 1, 1, 1] total=3}, AbilityRollSet{dice=[2, 3, 4, 5] total=12}, AbilityRollSet{dice=[3, 3, 3, 3] total=9}, AbilityRollSet{dice=[5, 5, 5, 2] total=15}] rolled_at=<time>} already_rolled=true}`},
		{"Dona types other dice", et.dona, [][]int32{typed, typed, typed, typed, typed, typed}, `failed_precondition: the ability scores are refused: ABILITY_SCORES_REFUSAL_REASON_ROLLS_ALREADY_STORED | AbilityScoresRefusal{reason=ABILITY_SCORES_REFUSAL_REASON_ROLLS_ALREADY_STORED method=ABILITY_METHOD_ROLLED_4D6}`},
		{"Caio types his dice", et.caio, [][]int32{typed, other, typed, typed, typed, typed}, `RollAbilityScoresResponse{rolls=AbilityRolls{sets=[AbilityRollSet{dice=[6, 5, 4, 1] total=15}, AbilityRollSet{dice=[6, 5, 4, 2] total=15}, AbilityRollSet{dice=[6, 5, 4, 1] total=15}, AbilityRollSet{dice=[6, 5, 4, 1] total=15}, AbilityRollSet{dice=[6, 5, 4, 1] total=15}, AbilityRollSet{dice=[6, 5, 4, 1] total=15}] typed=true rolled_at=<time>}}`},
		{"Caio sends them again", et.caio, [][]int32{typed, other, typed, typed, typed, typed}, `RollAbilityScoresResponse{rolls=AbilityRolls{sets=[AbilityRollSet{dice=[6, 5, 4, 1] total=15}, AbilityRollSet{dice=[6, 5, 4, 2] total=15}, AbilityRollSet{dice=[6, 5, 4, 1] total=15}, AbilityRollSet{dice=[6, 5, 4, 1] total=15}, AbilityRollSet{dice=[6, 5, 4, 1] total=15}, AbilityRollSet{dice=[6, 5, 4, 1] total=15}] typed=true rolled_at=<time>} already_rolled=true}`},
		{"the app has no faces left for Pia", et.pia, nil, `internal: internal error`},
	}
	for _, s := range steps {
		got := editorRolls(s.as.rollScores(et.campaign, s.typed...))
		if got != s.want {
			t.Errorf("%s:\n got  %s\n want %s", s.name, got, s.want)
		}
	}
	et.editorLogged(t, `"msg":"characters: cannot roll the ability scores"`, `roll the ability scores: roll a d6: dice: Fixed ran out of faces`)

	// The campaign's dice setting: forced in the app refuses typed dice, forced physical
	// refuses the app's roll. A table without the roll refuses both, first.
	other2 := et.h.newUser("Rui")
	et.h.join(et.master, et.campaign, other2)
	et.dice.rule.Store(int32(charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_FORCED_IN_APP))
	got := editorRolls(other2.rollScores(et.campaign, typed, typed, typed, typed, typed, typed))
	if want := `failed_precondition: the ability scores are refused: ABILITY_SCORES_REFUSAL_REASON_DICE_FORCED_IN_APP | AbilityScoresRefusal{reason=ABILITY_SCORES_REFUSAL_REASON_DICE_FORCED_IN_APP method=ABILITY_METHOD_ROLLED_4D6}`; got != want {
		t.Errorf("typed dice, forced in the app:\n got  %s\n want %s", got, want)
	}
	et.dice.rule.Store(int32(charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_FORCED_PHYSICAL))
	got = editorRolls(other2.rollScores(et.campaign))
	if want := `failed_precondition: the ability scores are refused: ABILITY_SCORES_REFUSAL_REASON_DICE_FORCED_PHYSICAL | AbilityScoresRefusal{reason=ABILITY_SCORES_REFUSAL_REASON_DICE_FORCED_PHYSICAL method=ABILITY_METHOD_ROLLED_4D6}`; got != want {
		t.Errorf("the app's roll, forced physical:\n got  %s\n want %s", got, want)
	}
	setRules(t, et.master, et.campaign, methods(true, true, false, true))
	for _, typedDice := range [][][]int32{nil, {typed, typed, typed, typed, typed, typed}} {
		got = editorRolls(et.dona.rollScores(et.campaign, typedDice...))
		if want := `failed_precondition: the ability scores are refused: ABILITY_SCORES_REFUSAL_REASON_METHOD_NOT_ALLOWED | AbilityScoresRefusal{reason=ABILITY_SCORES_REFUSAL_REASON_METHOD_NOT_ALLOWED method=ABILITY_METHOD_ROLLED_4D6}`; got != want {
			t.Errorf("a table without the roll (%d typed sets):\n got  %s\n want %s", len(typedDice), got, want)
		}
	}
	setRules(t, et.master, et.campaign, func(*campaignsv1.TableRules) {})
}

// TestRollAbilityScoresEditorWhenTheServerFails: a table rules read that fails, and stored
// sets that cannot be read, are the server's failures.
func TestRollAbilityScoresEditorWhenTheServerFails(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, func(c *Config) {
		c.Roller = &dice.Fixed{Faces: []int{6, 5, 4, 1, 6, 5, 4, 1, 6, 5, 4, 1, 6, 5, 4, 1, 6, 5, 4, 1, 6, 5, 4, 1}}
	})
	et.src.failInside.Store(true)
	got := editorRolls(et.dona.rollScores(et.campaign))
	if got != `internal: internal error` {
		t.Errorf("the rules read fails: %s", got)
	}
	et.editorLogged(t, `"msg":"characters: cannot roll the ability scores"`, `read the table rules: the content source is down`)
	et.src.failInside.Store(false)
	if _, err := et.dona.rollScores(et.campaign); err != nil {
		t.Fatalf("RollAbilityScores() error = %v", err)
	}
	if _, err := et.h.pool.Exec(t.Context(), `UPDATE character_ability_rolls SET sets = '{"a": 1}' WHERE user_id = $1`, et.dona.id); err != nil {
		t.Fatalf("corrupt the sets: %v", err)
	}
	typed := []int32{6, 5, 4, 1}
	for _, typedDice := range [][][]int32{nil, {typed, typed, typed, typed, typed, typed}} {
		got = editorRolls(et.dona.rollScores(et.campaign, typedDice...))
		if got != `internal: this character cannot be read right now` {
			t.Errorf("stored sets that cannot be read (%d typed sets): %s", len(typedDice), got)
		}
	}
	et.editorLogged(t, `"msg":"characters: cannot roll the ability scores"`, `a stored character document cannot be read`)
}
