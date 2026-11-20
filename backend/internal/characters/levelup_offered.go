package characters

import (
	"slices"
	"strings"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// offeredChoices are the keys a level-up offers the player the character: the options of the features of the level (the
// class's, the subclasses the player may pick, the choices an earlier level left open and the new level's), and the options a
// swap may take: what GetLevelUpOptions lists after hideFromPlayer. It also holds, to place a pick of an option, the options the
// sheet has and the invocations a warlock may take in the place of one, which that list leaves out.
type offeredChoices struct {
	options map[string]bool
	swaps   map[string]bool
	// invocations are the Eldritch Invocations a warlock may take when it gives one up.
	invocations map[string]bool
	// owned are the options the character's sheet already has: their own picks (the Book of Ancient Secrets rituals)
	// are offered too.
	owned map[string]bool
	// late are the picks of the choices an earlier level left open.
	late map[string]bool
	// values are the picks of the choices the sheet after the level asks, with the options a player may read: where a
	// pick of the choice of an option is judged. scoped says they were worked out.
	values map[string]bool
	scoped bool
}

// offeredChoicesOf reads what the level offers this player.
func offeredChoicesOf(t levelUpTarget, m authz.Membership) (offeredChoices, error) {
	offer, lerr := rules.LevelUpOptions(t.build, t.classKey, viewContent(t.content, m))
	if lerr != nil {
		return offeredChoices{}, levelUpRulesError(lerr)
	}
	o := offeredChoices{options: map[string]bool{}, swaps: map[string]bool{}, invocations: map[string]bool{}, owned: map[string]bool{}, late: map[string]bool{}, values: map[string]bool{}}
	for _, k := range t.build.FeatureChoices {
		o.owned[k] = true
	}
	for _, n := range offer.InvocationSwaps {
		if !hiddenOption(t, n.Key) {
			o.invocations[n.Key] = true
		}
	}
	o.addFeatureChoices(t, offer.FeatureChoices)
	o.addSubclasses(t, offer.Subclasses)
	o.addChoiceGroups(t, offer.LateChoices)
	o.addLate(t, offer.LateChoices)
	o.addChoiceGroups(t, offer.NewChoices)
	o.addSwapPools(t, offer.PickSwaps)
	o.addSwapPools(t, offer.FeatureSwaps)
	return o, nil
}

// addSubclasses adds the options of the subclasses the player may pick: not one the table retired or switched off, unless the
// character already has it.
func (o offeredChoices) addSubclasses(t levelUpTarget, subs []rules.LevelUpSubclass) {
	for _, sub := range subs {
		if (sub.Archived || sub.Off) && !slices.ContainsFunc(t.build.Classes, func(c rules.ClassLevel) bool { return c.Subclass == sub.Key }) {
			continue
		}
		o.addFeatureChoices(t, sub.FeatureChoices)
	}
}

func (o offeredChoices) addSwapPools(t levelUpTarget, groups []rules.SwapGroup) {
	for _, g := range groups {
		for _, n := range g.Pool {
			if !hiddenOption(t, n.Key) {
				o.swaps[n.Key] = true
			}
		}
	}
}

func (o offeredChoices) addFeatureChoices(t levelUpTarget, choices []rules.LevelUpFeatureChoice) {
	for _, ch := range choices {
		for _, n := range ch.Options {
			o.addOption(t, n.Key)
		}
		for _, b := range ch.Blocked {
			o.addOption(t, b.Key)
		}
	}
}

// optionsOf are the options of the choices of the groups.
func optionsOf(groups []rules.ChoiceGroup) []rules.ChoiceOption {
	var out []rules.ChoiceOption
	for _, g := range groups {
		for _, ch := range g.Choices {
			out = append(out, ch.Options...)
		}
	}
	return out
}

func (o offeredChoices) addChoiceGroups(t levelUpTarget, groups []rules.ChoiceGroup) {
	for _, opt := range optionsOf(groups) {
		o.addOption(t, opt.Key)
		o.addOption(t, opt.Stored)
	}
}

// addLate adds the picks of the choices an earlier level left open.
func (o offeredChoices) addLate(t levelUpTarget, groups []rules.ChoiceGroup) {
	for _, opt := range optionsOf(groups) {
		for _, k := range []string{opt.Key, opt.Stored} {
			if k != "" && !hiddenOption(t, k) {
				o.late[k] = true
			}
		}
	}
}

// addScopedValues adds the picks that the choices of the sheet after the level offer a player. A level that cannot be applied
// leaves them out: the plan refuses it with its own answer.
func (o *offeredChoices) addScopedValues(t levelUpTarget, choices *charactersv1.LevelUpChoices) {
	after, err := rules.ApplyLevelUp(t.build, levelUpChoicesFromProto(t.classKey, choices, rules.LevelUpHitPoints{Average: true}), t.content)
	if err != nil {
		return
	}
	for _, opt := range optionsOf(t.content.Choices(after).Groups) {
		if !hiddenOption(t, opt.Key) {
			o.values[opt.Stored] = true
		}
	}
	o.scoped = true
}

func (o offeredChoices) addOption(t levelUpTarget, key string) {
	if key != "" && !hiddenOption(t, key) {
		o.options[key] = true
	}
}

// unofferedRefusal is the refusal of a request that sends a key the level does not offer this player, or nil: the answer to
// it is that refusal alone, with no sheet built from the key. The master reads the sheet as before.
func unofferedRefusal(t levelUpTarget, m authz.Membership, choices *charactersv1.LevelUpChoices) (*charactersv1.LevelUpRefusal, error) {
	if isMaster(m) {
		return nil, nil
	}
	if len(choices.GetFeatureChoiceKeys()) == 0 && len(choices.GetLateChoiceKeys()) == 0 && len(choices.GetSwapFeatureOptions()) == 0 && choices.GetSwapPick() == nil {
		return nil, nil
	}
	offered, err := offeredChoicesOf(t, m)
	if err != nil {
		return nil, err
	}
	if slices.ContainsFunc(choices.GetFeatureChoiceKeys(), func(k string) bool { _, _, ok := rules.SplitScopedChoice(k); return ok }) {
		offered.addScopedValues(t, choices)
	}
	if r := offered.unofferedPick(choices); r != nil {
		return r, nil
	}
	return offered.unofferedSwap(choices), nil
}

// unofferedPick is the refusal of the first pick or late pick the level does not offer, or nil.
func (o offeredChoices) unofferedPick(choices *charactersv1.LevelUpChoices) *charactersv1.LevelUpRefusal {
	for _, k := range choices.GetFeatureChoiceKeys() {
		if !o.offersKey(k, choices.GetSwapInvocationKey() != "") {
			return unoffered("choices.feature_choice_keys", charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_FEATURE_CHOICE)
		}
	}
	for _, k := range choices.GetLateChoiceKeys() {
		if !o.late[k] {
			return unoffered("choices.late_choice_keys", charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_FEATURE_CHOICE)
		}
	}
	return nil
}

// unofferedSwap is the refusal of the first swap that takes an option the level does not offer, or nil.
func (o offeredChoices) unofferedSwap(choices *charactersv1.LevelUpChoices) *charactersv1.LevelUpRefusal {
	for _, sw := range choices.GetSwapFeatureOptions() {
		if !o.swaps[sw.GetToKey()] {
			return unoffered("choices.swap_feature_options", charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SWAP_NOT_ALLOWED)
		}
	}
	if sw := choices.GetSwapPick(); sw != nil && sw.GetToKey() != "" && !o.swaps[sw.GetToKey()] {
		return unoffered("choices.swap_pick", charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SWAP_NOT_ALLOWED)
	}
	return nil
}

// unoffered is the refusal of a key the level does not offer: the field and the reason alone.
func unoffered(field string, reason charactersv1.LevelUpRefusalReason) *charactersv1.LevelUpRefusal {
	return &charactersv1.LevelUpRefusal{Field: field, Reason: reason}
}

// offersKey says the level offers the feature choice key k: an option, an invocation taken in the place of the one given
// up, a pick the sheet already has, or a pick of the choice of an option the level offers or the sheet has, which that choice
// offers this player.
func (o offeredChoices) offersKey(k string, swapsInvocation bool) bool {
	if o.options[k] || (swapsInvocation && o.invocations[k]) {
		return true
	}
	choiceKey, _, ok := rules.SplitScopedChoice(k)
	if !ok {
		return false
	}
	if o.owned[k] {
		return true
	}
	owner, _, _ := strings.Cut(choiceKey, "#")
	if !o.options[owner] && !o.owned[owner] && (!swapsInvocation || !o.invocations[owner]) {
		return false
	}
	return !o.scoped || o.values[k]
}
