package characters

import (
	"slices"
	"strings"
	"testing"
	"uuid"

	"connectrpc.com/connect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// More of the guided level-up frozen before the split: the table's switched-off and
// retired content in the options (what a player reads, and what the master reads),
// a new class the table closed, the preview's own shape check, and the errors from
// behind the calls (the progression rule, the dice setting, the die, a stored sheet
// that cannot be read).

// levelUpFighterSheet is a Fighter at level, with the Defense style, at xp.
func levelUpFighterSheet(level, xp int32, subclass string) *charactersv1.CharacterSheet {
	cl := &charactersv1.ClassLevel{ClassKey: "class:fighter", Level: level}
	if subclass != "" {
		cl.Subclass = &charactersv1.ClassLevel_SubclassKey{SubclassKey: subclass}
	}
	return &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores:        &rulesv1.AbilityScores{Strength: 15, Dexterity: 12, Constitution: 14, Intelligence: 13, Wisdom: 10, Charisma: 10},
		RaceKey:           "race:human",
		Background:        &charactersv1.FullSheet_BackgroundKey{BackgroundKey: "background:acolyte"},
		Languages:         testLanguages,
		Classes:           []*charactersv1.ClassLevel{cl},
		ArmorKey:          "equipment:chain-mail",
		WeaponKeys:        []string{"equipment:longsword"},
		FeatureChoiceKeys: []string{"feature:fighter-fighting-style-defense"},
		ExperiencePoints:  xp,
	}}}
}

// TestGetLevelUpOptionsFrozenSwitchedOffContent: what a player and the master read
// when the master switched content off or retired it: a subclass, the class whose
// list a table subclass reuses, a race a feat asks for, and new classes.
func TestGetLevelUpOptionsFrozenSwitchedOffContent(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 2700)
	m := tb.master
	setRules(t, m, tb.campaign, func(r *campaignsv1.TableRules) { r.FeatsAllowed = true })
	ink := m.addEntry(t, tb.campaign, inkBlade(t, m.classDefaults(t, tb.campaign)))
	guardian := m.addEntry(t, tb.campaign, testClass("Guardião do Vale"))
	dwarfFeat := m.addEntry(t, tb.campaign, &rulesv1.TableFeat{
		NamePt: "Sangue de anão", DescPt: []string{"Só para anões."},
		Prerequisite: &rulesv1.FeatPrerequisite{RaceKey: "race:dwarf"},
	})
	// A feat that asks for the hidden dwarf or the visible elf and not to have a feat the master switches off.
	offFeat := m.addEntry(t, tb.campaign, &rulesv1.TableFeat{NamePt: "Dom escondido", DescPt: []string{"Escondido."}})
	pickyFeat := m.addEntry(t, tb.campaign, &rulesv1.TableFeat{
		NamePt: "Exigente", DescPt: []string{"Exigente."},
		Prerequisite: &rulesv1.FeatPrerequisite{Races: []string{"race:dwarf", "race:elf"}, LacksFeatureKey: offFeat.GetKey()},
	})
	// Toren, Fighter 2 going to 3, picks the subclass; Lia, Fighter 3 of the ink blade,
	// goes to 4 with the wizard's list.
	toren := tb.other.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Toren", levelUpFighterSheet(2, 900, ""))
	liaSheet := levelUpFighterSheet(3, 2700, ink.GetKey())
	liaSheet.GetFull().CantripKeys = []string{"spell:light", "spell:mage-hand"}
	liaSheet.GetFull().KnownSpellKeys = []string{"spell:shield", "spell:magic-missile", "spell:sleep"}
	liaPlayer := tb.h.newUser("Bri")
	tb.h.join(m, tb.campaign, liaPlayer)
	lia := liaPlayer.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Lia", liaSheet)
	labels := map[string]string{
		tb.pc.GetId(): "<pensantus>", toren.GetId(): "<toren>", lia.GetId(): "<lia>", tb.campaign: "<campaign>",
		ink.GetKey(): "<ink-blade>", guardian.GetKey(): "<guardian>", dwarfFeat.GetKey(): "<dwarf-feat>",
		offFeat.GetKey(): "<off-feat>", pickyFeat.GetKey(): "<picky-feat>",
	}

	// The master retires the guardian and switches off the champion, the wizard, the
	// dwarf and the rogue.
	if _, err := m.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: tb.campaign, Key: guardian.GetKey()})); err != nil {
		t.Fatalf("ArchiveTableEntry() error = %v", err)
	}
	m.setOff(t, tb.campaign, true, "subclass:champion", "class:wizard", "race:dwarf", "class:rogue", offFeat.GetKey())

	// The master's options are the golden; the player reads the same without what was switched
	// off or retired: no class that is closed, no list of a switched-off class, and no feat that asks
	// only for a race that is closed (RN-10).
	wantHiddenFromPlayer(t, tb, "toren", toren, tb.other, labels,
		`-classChoices[class:rogue] -classChoices[class:wizard] -subclasses[<ink-blade>].spellListClassKey -subclasses[subclass:champion]`)
	wantHiddenFromPlayer(t, tb, "lia", lia, liaPlayer, labels,
		`-classChoices[class:rogue] -classChoices[class:wizard] -feats[<dwarf-feat>] -feats[<off-feat>] -feats[<picky-feat>].lacksFeatureNamePt -feats[<picky-feat>].prerequisite.lacksFeatureKey -feats[<picky-feat>].prerequisite.races[1] -feats[<picky-feat>].unmet[0].keys[1] -spellListClassKey feats[<picky-feat>].lacksFeatureHidden=true feats[<picky-feat>].prerequisite.races[0]="race:elf" feats[<picky-feat>].unmet[0].keys[0]="race:elf"`)
	wantHiddenFromPlayer(t, tb, "pensantus", tb.pc, tb.owner, labels,
		`-classChoices[class:rogue] -feats[<dwarf-feat>] -feats[<off-feat>] -feats[<picky-feat>].lacksFeatureNamePt -feats[<picky-feat>].prerequisite.lacksFeatureKey -feats[<picky-feat>].prerequisite.races[1] -feats[<picky-feat>].unmet[0].keys[1] feats[<picky-feat>].lacksFeatureHidden=true feats[<picky-feat>].prerequisite.races[0]="race:elf" feats[<picky-feat>].unmet[0].keys[0]="race:elf"`)

	// A new class the table closed: retired for everyone, switched off for a player only.
	id := tb.pc.GetId()
	archived := levelUpRefused(&charactersv1.LevelUpRefusal{
		Field: "class_key", Reason: charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_ARCHIVED_CHOICE, ClassKey: guardian.GetKey(),
	})
	for _, u := range []*user{tb.owner, tb.master} {
		_, err := tb.optionsWith(u, &charactersv1.GetLevelUpOptionsRequest{CharacterId: id, ClassKey: guardian.GetKey()})
		wantLevelUpErr(t, "a retired class", err, archived)
	}
	_, err := tb.optionsWith(tb.owner, &charactersv1.GetLevelUpOptionsRequest{CharacterId: id, ClassKey: "class:rogue"})
	wantLevelUpErr(t, "a switched-off class", err, levelUpRefused(&charactersv1.LevelUpRefusal{
		Field: "class_key", Reason: charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SWITCHED_OFF_CHOICE, ClassKey: "class:rogue",
	}))
	if _, err := tb.optionsWith(tb.master, &charactersv1.GetLevelUpOptionsRequest{CharacterId: id, ClassKey: "class:rogue"}); err != nil {
		t.Errorf("the master's options for a switched-off class error = %v, want them", err)
	}
	_, err = tb.rollWith(tb.owner, &charactersv1.RollLevelUpHitPointsRequest{CharacterId: id, IdempotencyKey: uuid.New().String(), ClassKey: guardian.GetKey()})
	wantLevelUpErr(t, "a roll for a retired class", err, archived)
	ch := pensantusLevelUp()
	ch.ClassKey = "class:rogue"
	_, err = tb.levelUp(tb.owner, tb.pc, ch)
	wantLevelUpErr(t, "a level of a switched-off class", err, levelUpRefused(&charactersv1.LevelUpRefusal{
		Field: "class_key", Reason: charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SWITCHED_OFF_CHOICE, ClassKey: "class:rogue",
	}))
}

// wantHiddenFromPlayer reads the character's level-up options as the master and as the
// player: the master's are the golden, and what the player's lack or change is want (a
// keyed list element that is gone is -list[key]).
func wantHiddenFromPlayer(t *testing.T, tb *levelUpTable, name string, character *charactersv1.Character, player *user, labels map[string]string, want string) {
	t.Helper()
	master, err := tb.options(tb.master, character)
	if err != nil {
		t.Fatalf("GetLevelUpOptions(%s, master) error = %v", name, err)
	}
	read, err := tb.options(player, character)
	if err != nil {
		t.Fatalf("GetLevelUpOptions(%s, player) error = %v", name, err)
	}
	levelUpGolden(t, "options_"+name+"_switched_off_master", master, labels)
	got := fieldsMoved(relabeled(protoFlatKeyed(t, master), labels), relabeled(protoFlatKeyed(t, read), labels))
	if got != want {
		t.Errorf("%s: what the player's options lack:\n got  %s\n want %s", name, got, want)
	}
}

// relabeled replaces each id in m's keys and values with its label, before
// fieldsMoved sorts them: sorted by the random ids, the order would change from
// run to run.
func relabeled(m, labels map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		for id, label := range labels {
			k = strings.ReplaceAll(k, id, label)
			v = strings.ReplaceAll(v, id, label)
		}
		out[k] = v
	}
	return out
}

// TestPreviewLevelUpFrozenShape: the preview checks the shape of the choices after the
// character, as invalid_argument, and refuses a dice setting as an error.
func TestPreviewLevelUpFrozenShape(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 2700)
	preview := func(u *user, characterID string, ch *charactersv1.LevelUpChoices) error {
		_, err := u.api.PreviewLevelUp(t.Context(), connect.NewRequest(&charactersv1.PreviewLevelUpRequest{CampaignId: tb.campaign, CharacterId: characterID, Choices: ch}))
		return err
	}
	repeated := pensantusLevelUp()
	repeated.CantripKeys = []string{"spell:light", "spell:light"}
	wantLevelUpErr(t, "no choices", preview(tb.owner, tb.pc.GetId(), nil), levelUpInvalid("choices", "is required"))
	wantLevelUpErr(t, "a repeat", preview(tb.owner, tb.pc.GetId(), repeated), levelUpInvalid("choices.cantrip_keys", "repeats an entry"))
	// The character comes first.
	wantLevelUpErr(t, "a repeat, another's character", preview(tb.other, tb.pc.GetId(), repeated), levelUpNotFound())
	// The master previews too (the guided level-up's checks of a player bind the master's
	// preview the same way); a preview without a method takes the average.
	noMethod := pensantusLevelUp()
	noMethod.HitPoints = nil
	res, err := tb.master.api.PreviewLevelUp(t.Context(), connect.NewRequest(&charactersv1.PreviewLevelUpRequest{CampaignId: tb.campaign, CharacterId: tb.pc.GetId(), Choices: noMethod}))
	if err != nil || res.Msg.GetRefusal() != nil || res.Msg.GetAfter().GetHitPointsMax() != 30 {
		t.Errorf("the master's preview without a method = %v, %v; want 30 hit points, no refusal", res, err)
	}
	tb.dice.rule.Store(int32(charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_FORCED_IN_APP))
	typed := pensantusLevelUp()
	typed.HitPoints = &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_ROLLED_PHYSICAL, Value: 3}
	wantLevelUpErr(t, "typed, in-app dice", preview(tb.owner, tb.pc.GetId(), typed),
		levelUpBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_DICE_FORCED_IN_APP, tb.pc.GetId(), levelUpNoMsg))
}

// TestGuidedLevelUpFrozenFaults: what each call answers and logs when what is behind it
// fails: the progression rule, the dice setting, the server's die, and a stored sheet
// that is not a sheet.
func TestGuidedLevelUpFrozenFaults(t *testing.T) {
	t.Parallel()
	tb, logs, faults := newLevelUpFrozenTable(t, 2700) // no faces: the server's die fails
	internal := levelUpErrWant{code: connect.CodeInternal, msg: "internal error"}
	app := pensantusLevelUp()
	app.HitPoints = &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_ROLLED_IN_APP}
	phys := pensantusLevelUp()
	phys.HitPoints = &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_ROLLED_PHYSICAL, Value: 3}

	_, err := tb.roll(tb.owner, tb.pc)
	wantLevelUpErr(t, "the die fails", err, internal)

	faults.diceFails.Store(true)
	_, err = tb.options(tb.owner, tb.pc)
	wantLevelUpErr(t, "options, the dice setting fails", err, internal)
	_, err = tb.roll(tb.owner, tb.pc)
	wantLevelUpErr(t, "roll, the dice setting fails", err, internal)
	_, err = tb.levelUp(tb.owner, tb.pc, app)
	wantLevelUpErr(t, "in the app, the dice setting fails", err, internal)
	_, err = tb.levelUp(tb.owner, tb.pc, phys)
	wantLevelUpErr(t, "typed, the dice setting fails", err, internal)
	// The average never reads the dice setting.
	if _, err := tb.preview(tb.owner, tb.pc, pensantusLevelUp()); err != nil {
		t.Errorf("a preview with the average, the dice setting failing: %v", err)
	}
	faults.diceFails.Store(false)

	faults.ruleFails.Store(true)
	_, err = tb.options(tb.owner, tb.pc)
	wantLevelUpErr(t, "options, the rule fails", err, internal)
	_, err = tb.preview(tb.owner, tb.pc, pensantusLevelUp())
	wantLevelUpErr(t, "preview, the rule fails", err, internal)
	_, err = tb.levelUp(tb.owner, tb.pc, pensantusLevelUp())
	wantLevelUpErr(t, "level up, the rule fails", err, internal)
	faults.ruleFails.Store(false)

	wantLines := []string{
		"ERROR characters: cannot roll the level up hit points code=internal",
		"ERROR characters: cannot read the level up options code=internal",
		"ERROR characters: cannot roll the level up hit points code=internal",
		"ERROR characters: cannot level up a character code=internal",
		"ERROR characters: cannot level up a character code=internal",
		"ERROR characters: cannot read the level up options code=internal",
		"ERROR characters: cannot preview a level up code=internal",
		"ERROR characters: cannot level up a character code=internal",
	}
	if got := logs.errorLines(); !slices.Equal(got, wantLines) {
		t.Errorf("error lines =\n%q\nwant\n%q", got, wantLines)
	}

	// A stored sheet that is not a sheet: internal, with the fixed message, for every call.
	if _, err := tb.h.pool.Exec(t.Context(), `UPDATE characters SET sheet = '{"full": 3}' WHERE id = $1`, tb.pc.GetId()); err != nil {
		t.Fatalf("break the sheet: %v", err)
	}
	corrupt := levelUpErrWant{code: connect.CodeInternal, msg: "this character cannot be read right now"}
	_, err = tb.options(tb.owner, tb.pc)
	wantLevelUpErr(t, "options, a corrupt sheet", err, corrupt)
	_, err = tb.preview(tb.owner, tb.pc, pensantusLevelUp())
	wantLevelUpErr(t, "preview, a corrupt sheet", err, corrupt)
	_, err = tb.roll(tb.owner, tb.pc)
	wantLevelUpErr(t, "roll, a corrupt sheet", err, corrupt)
	_, err = tb.levelUp(tb.owner, tb.pc, pensantusLevelUp())
	wantLevelUpErr(t, "level up, a corrupt sheet", err, corrupt)
}
