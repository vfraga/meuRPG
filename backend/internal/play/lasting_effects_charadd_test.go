package play

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The characterization of AddCharacterEffect (the master gives an effect to characters outside
// a combat): every refusal with its code and message, every kind of input and its outcome, who
// reads the effect, the event and what the streams are told. The expected values were read from
// the code as it was; a comment says "frozen" where the behavior looks wrong and is kept.

// charEdit changes the request of a case.
type charEdit = func(*playv1.AddCharacterEffectRequest)

// giveRaw sends the request as the master (the base one is Bless on Pensantus).
func (a *armed) giveRaw(t *testing.T, u *user, edit charEdit) (*playv1.AddCharacterEffectResponse, error) {
	t.Helper()
	req := &playv1.AddCharacterEffectRequest{CampaignId: a.campaignID, IdempotencyKey: newKey(), CatalogKey: blessKey, CharacterIds: []string{a.pens.GetId()}}
	if edit != nil {
		edit(req)
	}
	res, err := u.lasting.AddCharacterEffect(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func withKey(key string) charEdit {
	return func(r *playv1.AddCharacterEffectRequest) { r.CatalogKey = key }
}

func onCharacters(ids ...string) charEdit {
	return func(r *playv1.AddCharacterEffectRequest) { r.CharacterIds = ids }
}

func TestAddCharacterEffectRefusals(t *testing.T) { //nolint:tparallel // the cases share one party, in order
	t.Parallel()
	a := newEffectsParty(t)
	outsider := a.h.newUser("Fora")
	manyIDs := func() []string {
		var ids []string
		for range 11 {
			ids = append(ids, newKey())
		}
		return ids
	}
	rounds := playv1.EffectDurationKind_EFFECT_DURATION_KIND_ROUNDS
	cases := []struct {
		name string
		by   *user
		edit charEdit
		code connect.Code
		msg  string
	}{
		{"a player", a.caio, nil, connect.CodePermissionDenied, "only the campaign's master can do this"},
		{"someone outside the campaign", outsider, nil, connect.CodeNotFound, "campaign not found"},
		{"the key is not a UUID", a.master, func(r *playv1.AddCharacterEffectRequest) { r.IdempotencyKey = "nope" }, connect.CodeInvalidArgument, "idempotency_key must be a UUID"},
		{"no character", a.master, onCharacters(), connect.CodeInvalidArgument, "character_ids must have 1 to 10 characters"},
		{"eleven characters", a.master, func(r *playv1.AddCharacterEffectRequest) { r.CharacterIds = manyIDs() }, connect.CodeInvalidArgument, "character_ids must have 1 to 10 characters"},
		{"a character id that is not a UUID", a.master, onCharacters("toren"), connect.CodeInvalidArgument, "character_ids must be UUIDs"},
		{"a character twice", a.master, func(r *playv1.AddCharacterEffectRequest) {
			r.CharacterIds = []string{a.pens.GetId(), a.toren.GetId(), a.pens.GetId()}
		}, connect.CodeInvalidArgument, "character_ids repeats a character"},
		{"a label of 31 characters", a.master, func(r *playv1.AddCharacterEffectRequest) { r.PlayerLabel = strings.Repeat("ã", 31) }, connect.CodeInvalidArgument, "player_label must have at most 30 characters"},
		{"a timed effect with no seconds", a.master, func(r *playv1.AddCharacterEffectRequest) { r.DurationKind = rounds }, connect.CodeInvalidArgument, "seconds must be 1 to 86400"},
		{"a timed effect with negative seconds", a.master, func(r *playv1.AddCharacterEffectRequest) { r.DurationKind, r.Seconds = rounds, -5 }, connect.CodeInvalidArgument, "seconds must be 1 to 86400"},
		{"a timed effect over a day", a.master, func(r *playv1.AddCharacterEffectRequest) { r.DurationKind, r.Seconds = rounds, 86401 }, connect.CodeInvalidArgument, "seconds must be 1 to 86400"},
		{"until the start of a turn", a.master, func(r *playv1.AddCharacterEffectRequest) {
			r.DurationKind = playv1.EffectDurationKind_EFFECT_DURATION_KIND_UNTIL_START_OF_TURN_OF
		}, connect.CodeInvalidArgument, "duration_kind has no meaning outside a combat"},
		{"until the end of a turn", a.master, func(r *playv1.AddCharacterEffectRequest) {
			r.DurationKind = playv1.EffectDurationKind_EFFECT_DURATION_KIND_UNTIL_END_OF_TURN_OF
		}, connect.CodeInvalidArgument, "duration_kind has no meaning outside a combat"},
		{"a character that does not exist", a.master, onCharacters(newKey()), connect.CodeNotFound, "character not found"},
		{"an NPC", a.master, onCharacters(a.goblin.GetId()), connect.CodeNotFound, "character not found"},
		{"one known and one unknown", a.master, func(r *playv1.AddCharacterEffectRequest) {
			r.CharacterIds = []string{a.pens.GetId(), newKey()}
		}, connect.CodeNotFound, "character not found"},
		{"no catalog key", a.master, withKey(""), connect.CodeInvalidArgument, "catalog_key is not an effect the master may add"},
		{"a key of nothing", a.master, withKey("bogus"), connect.CodeInvalidArgument, "catalog_key is not an effect the master may add"},
		{"exhaustion", a.master, withKey("condition:exhaustion"), connect.CodeInvalidArgument, "catalog_key is not an effect the master may add"},
		{"a condition that does not exist", a.master, withKey("condition:nope"), connect.CodeInvalidArgument, "catalog_key is not an effect the master may add"},
		{"a spell with no effect", a.master, withKey(fireBolt), connect.CodeInvalidArgument, "catalog_key is not an effect the master may add"},
		{"a spell that does not exist", a.master, withKey("spell:nope"), connect.CodeInvalidArgument, "catalog_key is not an effect the master may add"},
		{"an effect of a feature", a.master, withKey("effect:lethargy"), connect.CodeInvalidArgument, "catalog_key is not an effect the master may add"},
		{"an effect that does not exist", a.master, withKey("effect:nope"), connect.CodeInvalidArgument, "catalog_key is not an effect the master may add"},
		{"Enhance Ability with no ability", a.master, withKey(enhanceAbilityKey), connect.CodeInvalidArgument, "ability_key must be one of str, dex, con, int, wis or cha for this spell"},
		{"Enhance Ability for luck", a.master, func(r *playv1.AddCharacterEffectRequest) { r.CatalogKey, r.AbilityKey = enhanceAbilityKey, "luck" }, connect.CodeInvalidArgument, "ability_key must be one of str, dex, con, int, wis or cha for this spell"},
		{"Mage Armor on a creature in armor", a.master, func(r *playv1.AddCharacterEffectRequest) {
			r.CatalogKey, r.CharacterIds = mageArmorKey, []string{a.toren.GetId()}
		}, connect.CodeFailedPrecondition, "the effect takes no hold of these characters"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := a.giveRaw(t, tc.by, tc.edit)
			wantRefusal(t, tc.name, err, tc.code, tc.msg)
		})
	}
	if rows := a.characterEffectRows(t); len(rows) != 0 {
		t.Errorf("the refusals left effects: %v", rows)
	}
	if events := a.effectEvents(t, nil); len(events) != 0 {
		t.Errorf("the refusals left events: %v", events)
	}
}

func TestAddCharacterEffectNeedsAnOpenSession(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, caio := h.newUser("Samuel"), h.newUser("Caio")
	campaignID := h.newCampaign(master, "Mirathel", caio)
	hero := caio.hero(t, campaignID, "Toren", "class:fighter", "race:human", 2, abilities(16, 13, 14, 10, 8), []string{battleaxe}, nil)
	_, err := master.lasting.AddCharacterEffect(t.Context(), connect.NewRequest(&playv1.AddCharacterEffectRequest{
		CampaignId: campaignID, IdempotencyKey: newKey(), CatalogKey: blessKey, CharacterIds: []string{hero.GetId()},
	}))
	wantBlocked(t, "AddCharacterEffect with no session", err, playv1.GameSessionBlockedReason_GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION)
	wantRefusal(t, "AddCharacterEffect with no session", err, connect.CodeFailedPrecondition, "the campaign has no open game session")
}

func TestAddCharacterEffectRefusesACharacterInACombatAndOnlyThatOne(t *testing.T) {
	t.Parallel()
	a := newEffectsParty(t)
	// Only Toren, Brisa and the goblin fight: Pensantus is outside the combat.
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.toren.GetId()}, {CharacterId: a.bri.GetId()}, {CharacterId: a.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 18, "Brisa": 1},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Brisa": {4, 4}},
	})
	_ = e
	_, err := a.giveRaw(t, a.master, onCharacters(a.pens.GetId(), a.toren.GetId()))
	wantRefusal(t, "a character in the combat", err, connect.CodeFailedPrecondition, "the character is in a combat: add the effect there")
	if rows := a.characterEffectRows(t); len(rows) != 0 {
		t.Errorf("the refusal left effects: %v", rows)
	}
	if _, err := a.giveRaw(t, a.master, onCharacters(a.pens.GetId())); err != nil {
		t.Fatalf("a character outside the combat: error = %v, want it given", err)
	}
	if got := len(a.characterEffectRows(t)); got != 1 {
		t.Errorf("rows = %d, want 1", got)
	}
}

func TestAddCharacterEffectOutcomes(t *testing.T) { //nolint:tparallel // the cases share one party, in order
	t.Parallel()
	a := newEffectsParty(t)
	rounds := playv1.EffectDurationKind_EFFECT_DURATION_KIND_ROUNDS
	cases := []struct {
		name   string
		target *charactersv1.Character // Pensantus when nil
		edit   charEdit
		want   string // the rows, who reads them, and how many ids came back
	}{
		{name: "a condition that only its owner reads", want: "{\"audience\": \"owner\", \"character_id\": \"pens\", \"condition_keys\": [\"condition:poisoned\"], \"duration_kind\": \"until_dismissed\", \"player_visible\": true, \"source_key\": \"condition:poisoned\", \"source_kind\": \"master\"}\nseen 1/1/0\nids 1", edit: withKey("condition:poisoned")},
		{name: "a condition everyone reads", want: "{\"audience\": \"all\", \"character_id\": \"pens\", \"condition_keys\": [\"condition:blinded\"], \"duration_kind\": \"until_dismissed\", \"player_visible\": true, \"source_key\": \"condition:blinded\", \"source_kind\": \"master\"}\nseen 1/1/1\nids 1", edit: withKey("condition:blinded")},
		{name: "a condition for everyone against its default", want: "{\"audience\": \"all\", \"character_id\": \"pens\", \"condition_keys\": [\"condition:poisoned\"], \"duration_kind\": \"until_dismissed\", \"player_visible\": true, \"source_key\": \"condition:poisoned\", \"source_kind\": \"master\"}\nseen 1/1/0\nids 1", edit: func(r *playv1.AddCharacterEffectRequest) {
			r.CatalogKey, r.Audience = "condition:poisoned", playv1.EffectAudience_EFFECT_AUDIENCE_ALL
		}},
		{name: "a condition for its owner against its default", want: "{\"audience\": \"owner\", \"character_id\": \"pens\", \"condition_keys\": [\"condition:blinded\"], \"duration_kind\": \"until_dismissed\", \"player_visible\": true, \"source_key\": \"condition:blinded\", \"source_kind\": \"master\"}\nseen 1/1/0\nids 1", edit: func(r *playv1.AddCharacterEffectRequest) {
			r.CatalogKey, r.Audience = "condition:blinded", playv1.EffectAudience_EFFECT_AUDIENCE_OWNER
		}},
		// Finding fix/lasting-effects-audience: an unknown audience fails closed, hidden from the players and kept as owner.
		{name: "an audience the server does not know is the master's alone", want: "{\"audience\": \"owner\", \"character_id\": \"pens\", \"condition_keys\": [\"condition:poisoned\"], \"duration_kind\": \"until_dismissed\", \"player_visible\": false, \"source_key\": \"condition:poisoned\", \"source_kind\": \"master\"}\nseen 1/0/0\nids 1", edit: func(r *playv1.AddCharacterEffectRequest) {
			r.CatalogKey, r.Audience = "condition:poisoned", playv1.EffectAudience(99)
		}},
		{name: "hidden from the players", want: "{\"audience\": \"all\", \"character_id\": \"pens\", \"condition_keys\": [\"condition:blinded\"], \"duration_kind\": \"until_dismissed\", \"player_visible\": false, \"source_key\": \"condition:blinded\", \"source_kind\": \"master\"}\nseen 1/0/0\nids 1", edit: func(r *playv1.AddCharacterEffectRequest) {
			r.CatalogKey, r.PlayerVisible = "condition:blinded", new(false)
		}},
		{name: "shown to the players, said so", want: "{\"audience\": \"all\", \"character_id\": \"pens\", \"condition_keys\": [\"condition:blinded\"], \"duration_kind\": \"until_dismissed\", \"player_visible\": true, \"source_key\": \"condition:blinded\", \"source_kind\": \"master\"}\nseen 1/1/1\nids 1", edit: func(r *playv1.AddCharacterEffectRequest) {
			r.CatalogKey, r.PlayerVisible = "condition:blinded", new(true)
		}},
		{name: "a label of 30 characters", want: "{\"audience\": \"all\", \"character_id\": \"pens\", \"condition_keys\": [\"condition:blinded\"], \"duration_kind\": \"until_dismissed\", \"player_label\": \"ãããããããããããããããããããããããããããããã\", \"player_visible\": true, \"source_key\": \"condition:blinded\", \"source_kind\": \"master\"}\nseen 1/1/1\nids 1", edit: func(r *playv1.AddCharacterEffectRequest) {
			r.CatalogKey, r.PlayerLabel = "condition:blinded", strings.Repeat("ã", 30)
		}},
		{name: "a long rest keeps no seconds", want: "{\"audience\": \"all\", \"character_id\": \"pens\", \"condition_keys\": [\"condition:prone\"], \"duration_kind\": \"long_rest\", \"player_visible\": true, \"source_key\": \"condition:prone\", \"source_kind\": \"master\"}\nseen 1/1/1\nids 1", edit: func(r *playv1.AddCharacterEffectRequest) {
			r.CatalogKey, r.DurationKind, r.Seconds = "condition:prone", playv1.EffectDurationKind_EFFECT_DURATION_KIND_LONG_REST, 77
		}},
		{name: "until dismissed keeps no seconds", want: "{\"audience\": \"all\", \"character_id\": \"pens\", \"condition_keys\": [\"condition:prone\"], \"duration_kind\": \"until_dismissed\", \"player_visible\": true, \"source_key\": \"condition:prone\", \"source_kind\": \"master\"}\nseen 1/1/1\nids 1", edit: func(r *playv1.AddCharacterEffectRequest) {
			r.CatalogKey, r.DurationKind, r.Seconds = "condition:prone", playv1.EffectDurationKind_EFFECT_DURATION_KIND_UNTIL_DISMISSED, 77
		}},
		{name: "a concentration is until dismissed", want: "{\"audience\": \"all\", \"character_id\": \"pens\", \"condition_keys\": [\"condition:prone\"], \"duration_kind\": \"until_dismissed\", \"player_visible\": true, \"source_key\": \"condition:prone\", \"source_kind\": \"master\"}\nseen 1/1/1\nids 1", edit: func(r *playv1.AddCharacterEffectRequest) {
			r.CatalogKey, r.DurationKind, r.Seconds = "condition:prone", playv1.EffectDurationKind_EFFECT_DURATION_KIND_CONCENTRATION, 5
		}},
		// Finding fix/lasting-effects-outside-combat (b): seconds with no kind read as the clock of game time.
		{name: "seconds with no kind are a timed effect", want: "{\"audience\": \"all\", \"character_id\": \"pens\", \"condition_keys\": [\"condition:prone\"], \"duration_kind\": \"rounds\", \"player_visible\": true, \"seconds_left\": 600, \"source_key\": \"condition:prone\", \"source_kind\": \"master\"}\nseen 1/1/1\nids 1", edit: func(r *playv1.AddCharacterEffectRequest) {
			r.CatalogKey, r.Seconds = "condition:prone", 600
		}},
		{name: "a kind the server does not know is no kind (frozen: looks wrong because the kind is silently taken for the default)", want: "{\"audience\": \"all\", \"character_id\": \"pens\", \"condition_keys\": [\"condition:prone\"], \"duration_kind\": \"until_dismissed\", \"player_visible\": true, \"source_key\": \"condition:prone\", \"source_kind\": \"master\"}\nseen 1/1/1\nids 1", edit: func(r *playv1.AddCharacterEffectRequest) {
			r.CatalogKey, r.DurationKind, r.Seconds = "condition:prone", playv1.EffectDurationKind(99), 600
		}},
		{name: "a timed condition", want: "{\"audience\": \"all\", \"character_id\": \"pens\", \"condition_keys\": [\"condition:prone\"], \"duration_kind\": \"rounds\", \"player_visible\": true, \"seconds_left\": 600, \"source_key\": \"condition:prone\", \"source_kind\": \"master\"}\nseen 1/1/1\nids 1", edit: func(r *playv1.AddCharacterEffectRequest) {
			r.CatalogKey, r.DurationKind, r.Seconds = "condition:prone", rounds, 600
		}},
		{name: "a timed condition of one second", want: "{\"audience\": \"all\", \"character_id\": \"pens\", \"condition_keys\": [\"condition:prone\"], \"duration_kind\": \"rounds\", \"player_visible\": true, \"seconds_left\": 1, \"source_key\": \"condition:prone\", \"source_kind\": \"master\"}\nseen 1/1/1\nids 1", edit: func(r *playv1.AddCharacterEffectRequest) {
			r.CatalogKey, r.DurationKind, r.Seconds = "condition:prone", rounds, 1
		}},
		{name: "a timed condition of a day", want: "{\"audience\": \"all\", \"character_id\": \"pens\", \"condition_keys\": [\"condition:prone\"], \"duration_kind\": \"rounds\", \"player_visible\": true, \"seconds_left\": 86400, \"source_key\": \"condition:prone\", \"source_kind\": \"master\"}\nseen 1/1/1\nids 1", edit: func(r *playv1.AddCharacterEffectRequest) {
			r.CatalogKey, r.DurationKind, r.Seconds = "condition:prone", rounds, 86400
		}},
		{name: "Bless takes its own minute", want: "{\"audience\": \"owner\", \"character_id\": \"pens\", \"condition_keys\": [], \"duration_kind\": \"rounds\", \"mods\": \"roll_die\", \"player_visible\": true, \"seconds_left\": 60, \"source_key\": \"spell:bless\", \"source_kind\": \"spell\"}\nseen 1/1/0\nids 1", edit: withKey(blessKey)},
		{name: "Bless for an hour", want: "{\"audience\": \"owner\", \"character_id\": \"pens\", \"condition_keys\": [], \"duration_kind\": \"rounds\", \"mods\": \"roll_die\", \"player_visible\": true, \"seconds_left\": 3600, \"source_key\": \"spell:bless\", \"source_kind\": \"spell\"}\nseen 1/1/0\nids 1", edit: func(r *playv1.AddCharacterEffectRequest) {
			r.CatalogKey, r.DurationKind, r.Seconds = blessKey, rounds, 3600
		}},
		{name: "Bless until dismissed", want: "{\"audience\": \"owner\", \"character_id\": \"pens\", \"condition_keys\": [], \"duration_kind\": \"until_dismissed\", \"mods\": \"roll_die\", \"player_visible\": true, \"source_key\": \"spell:bless\", \"source_kind\": \"spell\"}\nseen 1/1/0\nids 1", edit: func(r *playv1.AddCharacterEffectRequest) {
			r.CatalogKey, r.DurationKind = blessKey, playv1.EffectDurationKind_EFFECT_DURATION_KIND_UNTIL_DISMISSED
		}},
		{name: "Bless until a long rest", want: "{\"audience\": \"owner\", \"character_id\": \"pens\", \"condition_keys\": [], \"duration_kind\": \"long_rest\", \"mods\": \"roll_die\", \"player_visible\": true, \"source_key\": \"spell:bless\", \"source_kind\": \"spell\"}\nseen 1/1/0\nids 1", edit: func(r *playv1.AddCharacterEffectRequest) {
			r.CatalogKey, r.DurationKind = blessKey, playv1.EffectDurationKind_EFFECT_DURATION_KIND_LONG_REST
		}},
		{name: "Bless ignores the ability", want: "{\"audience\": \"owner\", \"character_id\": \"pens\", \"condition_keys\": [], \"duration_kind\": \"rounds\", \"mods\": \"roll_die\", \"player_visible\": true, \"seconds_left\": 60, \"source_key\": \"spell:bless\", \"source_kind\": \"spell\"}\nseen 1/1/0\nids 1", edit: func(r *playv1.AddCharacterEffectRequest) { r.CatalogKey, r.AbilityKey = blessKey, "dex" }},
		{name: "Bless for everyone against its default", want: "{\"audience\": \"all\", \"character_id\": \"pens\", \"condition_keys\": [], \"duration_kind\": \"rounds\", \"mods\": \"roll_die\", \"player_visible\": true, \"seconds_left\": 60, \"source_key\": \"spell:bless\", \"source_kind\": \"spell\"}\nseen 1/1/1\nids 1", edit: func(r *playv1.AddCharacterEffectRequest) {
			r.CatalogKey, r.Audience = blessKey, playv1.EffectAudience_EFFECT_AUDIENCE_ALL
		}},
		{name: "Bless hidden", want: "{\"audience\": \"owner\", \"character_id\": \"pens\", \"condition_keys\": [], \"duration_kind\": \"rounds\", \"mods\": \"roll_die\", \"player_visible\": false, \"seconds_left\": 60, \"source_key\": \"spell:bless\", \"source_kind\": \"spell\"}\nseen 1/0/0\nids 1", edit: func(r *playv1.AddCharacterEffectRequest) {
			r.CatalogKey, r.PlayerVisible = blessKey, new(false)
		}},
		{name: "Mage Armor works the armor class out", want: "{\"audience\": \"all\", \"character_id\": \"pens\", \"condition_keys\": [], \"duration_kind\": \"rounds\", \"mods\": \"base_ac:15\", \"player_visible\": true, \"seconds_left\": 28800, \"source_key\": \"spell:mage-armor\", \"source_kind\": \"spell\"}\nseen 1/1/1\nids 1", edit: withKey(mageArmorKey)},
		{name: "Guidance", want: "{\"audience\": \"owner\", \"character_id\": \"pens\", \"condition_keys\": [], \"duration_kind\": \"rounds\", \"mods\": \"roll_die\", \"player_visible\": true, \"seconds_left\": 60, \"source_key\": \"spell:guidance\", \"source_kind\": \"spell\"}\nseen 1/1/0\nids 1", edit: withKey(guidanceKey)},
		{name: "Longstrider", want: "{\"audience\": \"owner\", \"character_id\": \"pens\", \"condition_keys\": [], \"duration_kind\": \"rounds\", \"mods\": \"speed_add:10\", \"player_visible\": true, \"seconds_left\": 3600, \"source_key\": \"spell:longstrider\", \"source_kind\": \"spell\"}\nseen 1/1/0\nids 1", edit: withKey(longstriderKey)},
		{name: "Heroism has no caster to take the modifier from", want: "{\"audience\": \"owner\", \"character_id\": \"pens\", \"condition_keys\": [], \"duration_kind\": \"rounds\", \"mods\": \"condition_immunity,turn_temp_hp\", \"player_visible\": true, \"seconds_left\": 60, \"source_key\": \"spell:heroism\", \"source_kind\": \"spell\"}\nseen 1/1/0\nids 1", edit: withKey(heroismKey)},
		{name: "Enhance Ability for dexterity", want: "{\"audience\": \"owner\", \"character_id\": \"pens\", \"condition_keys\": [], \"duration_kind\": \"rounds\", \"mods\": \"check_advantage[\\\"dex\\\"]\", \"player_visible\": true, \"seconds_left\": 3600, \"source_key\": \"spell:enhance-ability\", \"source_kind\": \"spell\"}\nseen 1/1/0\nids 1", edit: func(r *playv1.AddCharacterEffectRequest) { r.CatalogKey, r.AbilityKey = enhanceAbilityKey, "dex" }},
		{name: "Vicious Mockery has no time of its own", want: "{\"audience\": \"owner\", \"character_id\": \"pens\", \"condition_keys\": [], \"duration_kind\": \"until_dismissed\", \"mods\": \"attack_disadvantage\", \"player_visible\": true, \"source_key\": \"spell:vicious-mockery\", \"source_kind\": \"spell\"}\nseen 1/1/0\nids 1", edit: withKey("spell:vicious-mockery")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a.clearEffects(t, nil)
			target := tc.target
			if target == nil {
				target = a.pens
			}
			res, err := a.giveRaw(t, a.master, func(r *playv1.AddCharacterEffectRequest) {
				r.CharacterIds = []string{target.GetId()}
				tc.edit(r)
			})
			if err != nil {
				t.Fatalf("error = %v, want it given", err)
			}
			rows := a.characterEffectRows(t)
			got := strings.Join(rows, "\n") + "\nseen " + a.seenBy(t, a.ana, a.caio, target.GetId()) + "\nids " + fmt.Sprint(len(res.GetEffectIds()))
			if got != tc.want {
				t.Errorf("GOLDEN\t%s\t%q", tc.name, got)
			}
		})
	}
}

// groups counts the distinct groups among the effects of the source key.
func (a *armed) groups(t *testing.T, key string) int {
	t.Helper()
	var n int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT count(DISTINCT group_id) FROM character_effects WHERE campaign_id = $1 AND source_key = $2`, a.campaignID, key).Scan(&n); err != nil {
		t.Fatalf("count the groups: %v", err)
	}
	return n
}

// The effect given to several characters is one group with a row for each; the same spell
// again replaces the old one of the character it is put on and no other; a retry of the request
// answers the same ids and writes nothing; a key spent on another change is refused; a
// character the effect takes no hold of (Mage Armor on armor) is left out; and the streams are
// told once for each change, each player what is theirs.
func TestAddCharacterEffectGroupsReplacesRetriesAndTellsTheStreams(t *testing.T) {
	t.Parallel()
	a := newEffectsParty(t)
	h := a.listen(t)
	all := []string{a.pens.GetId(), a.bri.GetId(), a.toren.GetId()}
	key := newKey()
	first := func(r *playv1.AddCharacterEffectRequest) { r.IdempotencyKey, r.CharacterIds = key, all }

	res, err := a.giveRaw(t, a.master, first)
	if err != nil {
		t.Fatalf("Bless on three: error = %v", err)
	}
	var steps []string
	step := func(name, got string) { steps = append(steps, name+": "+got) }
	step("three ids", fmt.Sprintf("%d", len(res.GetEffectIds())))
	step("rows", strings.Join(a.characterEffectRows(t), "\n"))
	step("groups", fmt.Sprintf("%d", a.groups(t, blessKey)))
	step("events", strings.Join(a.effectEvents(t, nil), "\n"))
	step("heard", a.heard(t, h))

	again, err := a.giveRaw(t, a.master, first)
	if err != nil {
		t.Fatalf("the same request again: error = %v", err)
	}
	step("retry answers the same ids", fmt.Sprintf("%v", slices.Equal(again.GetEffectIds(), res.GetEffectIds())))
	step("retry rows", fmt.Sprintf("%d rows, %d events", len(a.characterEffectRows(t)), len(a.effectEvents(t, nil))))
	// The streams are told again, with the vitals of the characters the first call may have changed: it may have died before it told them.
	step("retry heard", a.heard(t, h))

	_, err = a.giveRaw(t, a.master, func(r *playv1.AddCharacterEffectRequest) {
		r.IdempotencyKey, r.CharacterIds, r.CatalogKey = key, all, "condition:blinded"
	})
	wantRefusal(t, "the key of another request", err, connect.CodeInvalidArgument, "idempotency_key was already used for another change")
	_, err = a.giveRaw(t, a.master, func(r *playv1.AddCharacterEffectRequest) {
		r.IdempotencyKey, r.CharacterIds = key, []string{a.pens.GetId()}
	})
	wantRefusal(t, "the key of the same kind with other characters", err, connect.CodeInvalidArgument, "idempotency_key was already used for another change")

	spent := newKey()
	a.advanceGameTimeKeyed(t, spent, 6)
	_, err = a.giveRaw(t, a.master, func(r *playv1.AddCharacterEffectRequest) { r.IdempotencyKey = spent })
	wantRefusal(t, "the key of another kind of change", err, connect.CodeInvalidArgument, "idempotency_key was already used for another change")
	step("refusals left", fmt.Sprintf("%d rows, %d events", len(a.characterEffectRows(t)), len(a.effectEvents(t, nil))))
	step("refusals heard", a.heard(t, h))

	// Bless again on Pensantus alone: her old one goes, the other two stay.
	if _, err := a.giveRaw(t, a.master, onCharacters(a.pens.GetId())); err != nil {
		t.Fatalf("Bless again: error = %v", err)
	}
	step("replaced rows", strings.Join(a.characterEffectRows(t), "\n"))
	step("replaced groups", fmt.Sprintf("%d", a.groups(t, blessKey)))
	step("replaced heard", a.heard(t, h))

	// Mage Armor on the three: Toren wears armor and is left out.
	armor, err := a.giveRaw(t, a.master, func(r *playv1.AddCharacterEffectRequest) { r.CatalogKey, r.CharacterIds = mageArmorKey, all })
	if err != nil {
		t.Fatalf("Mage Armor on three: error = %v", err)
	}
	step("mage armor ids", fmt.Sprintf("%d", len(armor.GetEffectIds())))
	step("mage armor rows", strings.Join(a.characterEffectRows(t), "\n"))
	step("mage armor events", strings.Join(a.effectEvents(t, nil), "\n"))
	step("mage armor heard", a.heard(t, h))

	for _, s := range steps {
		name, got, _ := strings.Cut(s, ": ")
		if want, ok := groupsGolden[name]; !ok || got != want {
			t.Errorf("GOLDEN\t%s\t%q", name, got)
		}
	}
}

// groupsGolden is what each step of the test above saw, as read from the code before it was
// split into phases.
var groupsGolden = map[string]string{
	"rows":                       "{\"audience\": \"owner\", \"character_id\": \"bri\", \"condition_keys\": [], \"duration_kind\": \"rounds\", \"mods\": \"roll_die\", \"player_visible\": true, \"seconds_left\": 60, \"source_key\": \"spell:bless\", \"source_kind\": \"spell\"}\n{\"audience\": \"owner\", \"character_id\": \"pens\", \"condition_keys\": [], \"duration_kind\": \"rounds\", \"mods\": \"roll_die\", \"player_visible\": true, \"seconds_left\": 60, \"source_key\": \"spell:bless\", \"source_kind\": \"spell\"}\n{\"audience\": \"owner\", \"character_id\": \"toren\", \"condition_keys\": [], \"duration_kind\": \"rounds\", \"mods\": \"roll_die\", \"player_visible\": true, \"seconds_left\": 60, \"source_key\": \"spell:bless\", \"source_kind\": \"spell\"}",
	"retry heard":                "master[SpellCastsChanged,VitalsChanged,VitalsChanged,VitalsChanged] caio[SpellCastsChanged,VitalsChanged] ana[SpellCastsChanged,VitalsChanged] bia[SpellCastsChanged,VitalsChanged]",
	"replaced rows":              "{\"audience\": \"owner\", \"character_id\": \"bri\", \"condition_keys\": [], \"duration_kind\": \"rounds\", \"mods\": \"roll_die\", \"player_visible\": true, \"seconds_left\": 54, \"source_key\": \"spell:bless\", \"source_kind\": \"spell\"}\n{\"audience\": \"owner\", \"character_id\": \"pens\", \"condition_keys\": [], \"duration_kind\": \"rounds\", \"mods\": \"roll_die\", \"player_visible\": true, \"seconds_left\": 60, \"source_key\": \"spell:bless\", \"source_kind\": \"spell\"}\n{\"audience\": \"owner\", \"character_id\": \"toren\", \"condition_keys\": [], \"duration_kind\": \"rounds\", \"mods\": \"roll_die\", \"player_visible\": true, \"seconds_left\": 54, \"source_key\": \"spell:bless\", \"source_kind\": \"spell\"}",
	"mage armor rows":            "{\"audience\": \"all\", \"character_id\": \"bri\", \"condition_keys\": [], \"duration_kind\": \"rounds\", \"mods\": \"base_ac:16\", \"player_visible\": true, \"seconds_left\": 28800, \"source_key\": \"spell:mage-armor\", \"source_kind\": \"spell\"}\n{\"audience\": \"all\", \"character_id\": \"pens\", \"condition_keys\": [], \"duration_kind\": \"rounds\", \"mods\": \"base_ac:15\", \"player_visible\": true, \"seconds_left\": 28800, \"source_key\": \"spell:mage-armor\", \"source_kind\": \"spell\"}\n{\"audience\": \"owner\", \"character_id\": \"bri\", \"condition_keys\": [], \"duration_kind\": \"rounds\", \"mods\": \"roll_die\", \"player_visible\": true, \"seconds_left\": 54, \"source_key\": \"spell:bless\", \"source_kind\": \"spell\"}\n{\"audience\": \"owner\", \"character_id\": \"pens\", \"condition_keys\": [], \"duration_kind\": \"rounds\", \"mods\": \"roll_die\", \"player_visible\": true, \"seconds_left\": 60, \"source_key\": \"spell:bless\", \"source_kind\": \"spell\"}\n{\"audience\": \"owner\", \"character_id\": \"toren\", \"condition_keys\": [], \"duration_kind\": \"rounds\", \"mods\": \"roll_die\", \"player_visible\": true, \"seconds_left\": 54, \"source_key\": \"spell:bless\", \"source_kind\": \"spell\"}",
	"three ids":                  "3",
	"groups":                     "1",
	"events":                     "{\"effects\": [\"<id1>\", \"<id2>\", \"<id3>\"], \"key\": \"spell:bless\", \"targets\": [\"pens\", \"bri\", \"toren\"]} key=true hash=true enc=false char=false actor=true",
	"heard":                      "master[SpellCastsChanged] caio[SpellCastsChanged] ana[SpellCastsChanged] bia[SpellCastsChanged]",
	"retry answers the same ids": "true",
	"retry rows":                 "3 rows, 1 events",
	"refusals left":              "3 rows, 1 events",
	"refusals heard":             "master[SpellCastsChanged] caio[SpellCastsChanged] ana[SpellCastsChanged] bia[SpellCastsChanged]",
	"replaced groups":            "2",
	"replaced heard":             "master[SpellCastsChanged] caio[SpellCastsChanged] ana[SpellCastsChanged] bia[SpellCastsChanged]",
	"mage armor ids":             "2",
	"mage armor events":          "{\"effects\": [\"<id1>\", \"<id2>\", \"<id3>\"], \"key\": \"spell:bless\", \"targets\": [\"pens\", \"bri\", \"toren\"]} key=true hash=true enc=false char=false actor=true\n{\"effects\": [\"<id1>\"], \"key\": \"spell:bless\", \"targets\": [\"pens\"]} key=true hash=true enc=false char=false actor=true\n{\"effects\": [\"<id1>\", \"<id2>\"], \"key\": \"spell:mage-armor\", \"targets\": [\"pens\", \"bri\"]} key=true hash=true enc=false char=false actor=true",
	"mage armor heard":           "master[SpellCastsChanged,VitalsChanged,VitalsChanged] caio[SpellCastsChanged] ana[SpellCastsChanged,VitalsChanged] bia[SpellCastsChanged,VitalsChanged]",
}

// An effect that has no numeric modifiers (Hold Person, Web, Faerie Fire, the burning web) is
// valid outside a combat as it is in one: its conditions alone are the effect.
// Finding fix/lasting-effects-outside-combat (a): the modifiers were written as JSON null, which
// the table refuses, so the master got an internal error and nothing was written.
func TestAddCharacterEffectOfAnEffectWithNoModifiersIsAdded(t *testing.T) {
	t.Parallel()
	a := newEffectsParty(t)
	keys := []string{holdPerson, webSpell, "spell:faerie-fire", "effect:web-fire"}
	for _, key := range keys {
		res, err := a.giveRaw(t, a.master, withKey(key))
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if len(res.GetEffectIds()) != 1 {
			t.Errorf("%s: effect ids = %v, want 1", key, res.GetEffectIds())
		}
	}
	if rows := a.characterEffectRows(t); len(rows) != len(keys) {
		t.Errorf("rows = %d, want %d: %v", len(rows), len(keys), rows)
	}
}
