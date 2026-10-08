package characters

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// This file is the table's own content between its three shapes (MR-025, RN-23,
// ADR-0018): the typed messages of rules/v1/table_content.proto (what the editors
// send and read), the protojson stored in campaign_content.data, and the
// rules.Overlay the engine adds to the SRD. The messages map one to one onto the
// Overlay's fields, so this is renaming, with the one thing the server adds: the
// keys.

// Limits the server checks before the rules engine does (ADR-0018, section 9);
// the engine checks the 300 entries and the 60 features itself.
const (
	// MaxTableEntryBytes is the most protojson a table entry's data may take.
	MaxTableEntryBytes = 64 * 1024
)

// Reasons of a TableContentViolation that the server raises itself. The engine's
// are rules.Reason*.
const (
	reasonImmutable = "immutable"
	reasonSizeLimit = "size_limit"
)

// tableKinds describes the six kinds: the key prefix, the proto kind and the
// feature prefix of what the kind holds.
var tableKinds = []struct {
	prefix string
	kind   rulesv1.TableContentKind
}{
	{"class", rulesv1.TableContentKind_TABLE_CONTENT_KIND_CLASS},
	{"subclass", rulesv1.TableContentKind_TABLE_CONTENT_KIND_SUBCLASS},
	{"race", rulesv1.TableContentKind_TABLE_CONTENT_KIND_RACE},
	{"subrace", rulesv1.TableContentKind_TABLE_CONTENT_KIND_SUBRACE},
	{"background", rulesv1.TableContentKind_TABLE_CONTENT_KIND_BACKGROUND},
	{"spell", rulesv1.TableContentKind_TABLE_CONTENT_KIND_SPELL},
}

func kindPrefix(k rulesv1.TableContentKind) string {
	for _, tk := range tableKinds {
		if tk.kind == k {
			return tk.prefix
		}
	}
	return ""
}

func kindOfPrefix(prefix string) rulesv1.TableContentKind {
	for _, tk := range tableKinds {
		if tk.prefix == prefix {
			return tk.kind
		}
	}
	return rulesv1.TableContentKind_TABLE_CONTENT_KIND_UNSPECIFIED
}

// keyKind is the kind a table key is of ("class:x@mesa" is "class").
func keyKind(key string) string {
	prefix, _, _ := strings.Cut(key, ":")
	return prefix
}

// tableBody is the typed message of one entry. It is what the stored data is the
// protojson of.
type tableBody interface {
	proto.Message
	GetNamePt() string
}

// newBody makes an empty body of a kind.
func newBody(kind rulesv1.TableContentKind) tableBody {
	switch kind {
	case rulesv1.TableContentKind_TABLE_CONTENT_KIND_CLASS:
		return &rulesv1.TableClass{}
	case rulesv1.TableContentKind_TABLE_CONTENT_KIND_SUBCLASS:
		return &rulesv1.TableSubclass{}
	case rulesv1.TableContentKind_TABLE_CONTENT_KIND_RACE:
		return &rulesv1.TableRace{}
	case rulesv1.TableContentKind_TABLE_CONTENT_KIND_SUBRACE:
		return &rulesv1.TableSubrace{}
	case rulesv1.TableContentKind_TABLE_CONTENT_KIND_BACKGROUND:
		return &rulesv1.TableBackground{}
	case rulesv1.TableContentKind_TABLE_CONTENT_KIND_SPELL:
		return &rulesv1.TableSpell{}
	}
	return nil
}

// setBody puts a body in the oneof of an entry.
func setBody(e *rulesv1.TableEntry, b tableBody) {
	switch v := b.(type) {
	case *rulesv1.TableClass:
		e.Body = &rulesv1.TableEntry_TableClass{TableClass: v}
	case *rulesv1.TableSubclass:
		e.Body = &rulesv1.TableEntry_TableSubclass{TableSubclass: v}
	case *rulesv1.TableRace:
		e.Body = &rulesv1.TableEntry_TableRace{TableRace: v}
	case *rulesv1.TableSubrace:
		e.Body = &rulesv1.TableEntry_TableSubrace{TableSubrace: v}
	case *rulesv1.TableBackground:
		e.Body = &rulesv1.TableEntry_TableBackground{TableBackground: v}
	case *rulesv1.TableSpell:
		e.Body = &rulesv1.TableEntry_TableSpell{TableSpell: v}
	}
}

// bodyFields are the six oneof getters of a write request.
type bodyFields interface {
	GetTableClass() *rulesv1.TableClass
	GetTableSubclass() *rulesv1.TableSubclass
	GetTableRace() *rulesv1.TableRace
	GetTableSubrace() *rulesv1.TableSubrace
	GetTableBackground() *rulesv1.TableBackground
	GetTableSpell() *rulesv1.TableSpell
}

// bodyOf is the body a request carries, with the kind it gives, or nil.
func bodyOf(r bodyFields) (tableBody, rulesv1.TableContentKind) {
	switch {
	case r.GetTableClass() != nil:
		return r.GetTableClass(), rulesv1.TableContentKind_TABLE_CONTENT_KIND_CLASS
	case r.GetTableSubclass() != nil:
		return r.GetTableSubclass(), rulesv1.TableContentKind_TABLE_CONTENT_KIND_SUBCLASS
	case r.GetTableRace() != nil:
		return r.GetTableRace(), rulesv1.TableContentKind_TABLE_CONTENT_KIND_RACE
	case r.GetTableSubrace() != nil:
		return r.GetTableSubrace(), rulesv1.TableContentKind_TABLE_CONTENT_KIND_SUBRACE
	case r.GetTableBackground() != nil:
		return r.GetTableBackground(), rulesv1.TableContentKind_TABLE_CONTENT_KIND_BACKGROUND
	case r.GetTableSpell() != nil:
		return r.GetTableSpell(), rulesv1.TableContentKind_TABLE_CONTENT_KIND_SPELL
	}
	return nil, rulesv1.TableContentKind_TABLE_CONTENT_KIND_UNSPECIFIED
}

// bodyField is the name of the body's field in a request, for violations.
func bodyField(kind rulesv1.TableContentKind) string { return "table_" + kindPrefix(kind) }

// entryRow is a stored entry with its body decoded.
type entryRow struct {
	row  charactersdb.CampaignContent
	kind rulesv1.TableContentKind
	body tableBody
}

// decodeRow reads a stored entry. The error never carries the data, which holds
// what the master wrote.
func decodeRow(row charactersdb.CampaignContent) (entryRow, error) {
	kind := kindOfPrefix(row.Kind)
	body := newBody(kind)
	if body == nil {
		return entryRow{}, fmt.Errorf("%w: the kind of table entry %s", errCorruptDocument, row.ContentKey)
	}
	if err := loadJSON.Unmarshal(row.Data, body); err != nil {
		return entryRow{}, fmt.Errorf("%w: table entry %s", errCorruptDocument, row.ContentKey)
	}
	return entryRow{row: row, kind: kind, body: body}, nil
}

// overlayOf is the rules.Overlay of a campaign's stored entries at a revision.
func overlayOf(rows []charactersdb.CampaignContent, revision int) (rules.Overlay, error) {
	entries := make([]entryRow, 0, len(rows))
	for _, r := range rows {
		e, err := decodeRow(r)
		if err != nil {
			return rules.Overlay{}, err
		}
		entries = append(entries, e)
	}
	o, _ := overlayFromEntries(entries, revision)
	return o, nil
}

// overlayIndex says, for each kind's slice of an Overlay, which entry is at each
// index, so an OverlayError's "classes[2]" names an entry.
type overlayIndex map[string][]entryRow

// overlayFromEntries turns entries into the engine's Overlay, in the order
// given, and remembers which entry is at which index.
func overlayFromEntries(entries []entryRow, revision int) (rules.Overlay, overlayIndex) {
	o := rules.Overlay{Revision: revision}
	idx := overlayIndex{}
	for _, e := range entries {
		te := rules.TableEntry{
			Key: e.row.ContentKey, NamePT: e.row.NamePt, Archived: e.row.ArchivedAt != nil, Revision: int(e.row.Revision), ChangedAt: e.row.UpdatedAt,
		}
		switch b := e.body.(type) {
		case *rulesv1.TableClass:
			o.Classes = append(o.Classes, tableClassOf(te, b))
			idx["classes"] = append(idx["classes"], e)
		case *rulesv1.TableSubclass:
			o.Subclasses = append(o.Subclasses, tableSubclassOf(te, b))
			idx["subclasses"] = append(idx["subclasses"], e)
		case *rulesv1.TableRace:
			o.Races = append(o.Races, tableRaceOf(te, b))
			idx["races"] = append(idx["races"], e)
		case *rulesv1.TableSubrace:
			o.Subraces = append(o.Subraces, rules.TableSubrace{
				TableEntry: te, Race: b.GetRaceKey(), AbilityBonuses: tableScoresOf(b.GetAbilityBonuses()), Traits: tableFeaturesOf(b.GetTraits()),
			})
			idx["subraces"] = append(idx["subraces"], e)
		case *rulesv1.TableBackground:
			o.Backgrounds = append(o.Backgrounds, rules.TableBackground{
				TableEntry: te, Skills: b.GetSkills(), Tools: b.GetTools(), LanguageChoices: int(b.GetLanguageChoices()),
				EquipmentPT: b.GetEquipmentPt(), Feature: tableFeatureOf(b.GetFeature()),
			})
			idx["backgrounds"] = append(idx["backgrounds"], e)
		case *rulesv1.TableSpell:
			o.Spells = append(o.Spells, tableSpellOf(te, b))
			idx["spells"] = append(idx["spells"], e)
		}
	}
	return o, idx
}

var abilityFromProto = func() map[rulesv1.Ability]rules.Ability {
	m := map[rulesv1.Ability]rules.Ability{}
	for a, p := range abilityToProto {
		m[p] = a
	}
	return m
}()

func tableAbilitiesOf(in []rulesv1.Ability) []rules.Ability {
	var out []rules.Ability
	for _, a := range in {
		out = append(out, abilityFromProto[a]) // an unknown one is "": the engine refuses it
	}
	return out
}

// tableScoresOf drops the zeros: an ability the message does not give is not in the map.
func tableScoresOf(s *rulesv1.AbilityScores) map[rules.Ability]int {
	if s == nil {
		return nil
	}
	out := map[rules.Ability]int{}
	for a, n := range abilityMap(s) {
		if n != 0 {
			out[a] = n
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func tableClassOf(te rules.TableEntry, b *rulesv1.TableClass) rules.TableClass {
	c := rules.TableClass{
		TableEntry: te, HitDie: int(b.GetHitDie()), SavingThrows: tableAbilitiesOf(b.GetSavingThrows()),
		SkillChoose: int(b.GetSkillChoose()), SkillFrom: b.GetSkillFrom(), Proficiencies: b.GetProficiencies(),
		Minimums: tableScoresOf(b.GetMinimums()), AnyOf: tableScoresOf(b.GetAnyOf()),
		MulticlassProficiencies: b.GetMulticlassProficiencies(), MulticlassSkillChoose: int(b.GetMulticlassSkillChoose()),
		SubclassLevel: int(b.GetSubclassLevel()),
	}
	for _, l := range b.GetAsiLevels() {
		c.ASILevels = append(c.ASILevels, int(l))
	}
	if cs := b.GetCasting(); cs != nil {
		c.Casting = *tableCastingOf(cs)
	}
	for _, l := range b.GetLevels() {
		c.Levels = append(c.Levels, rules.TableClassLevel{
			ProfBonus: int(l.GetProfBonus()), Features: tableFeaturesOf(l.GetFeatures()),
			TableLevel: tableLevelOf(l.GetCantripsKnown(), l.GetSpellsKnown(), l.GetSlots()),
		})
	}
	return c
}

func tableCastingOf(c *rulesv1.TableCasting) *rules.TableCasting {
	return &rules.TableCasting{
		Kind: c.GetKind(), Ability: abilityFromProto[c.GetAbility()], Preparation: c.GetPreparation(), ListFrom: c.GetListFrom(),
		PreparedMax: c.GetPreparedMax(), StartLevel: int(c.GetStartLevel()), Ritual: c.GetRitual(),
	}
}

// tableLevelOf is a row's casting columns. Slots has nine entries or none; the
// engine refuses a row whose slots are not nine counts, so a different length
// is padded or cut here and caught by slotsLength first.
func tableLevelOf(cantrips, spells int32, slots []int32) rules.TableLevel {
	l := rules.TableLevel{CantripsKnown: int(cantrips), SpellsKnown: int(spells)}
	for i := 0; i < len(slots) && i < len(l.Slots); i++ {
		l.Slots[i] = int(slots[i])
	}
	return l
}

func tableSubclassOf(te rules.TableEntry, b *rulesv1.TableSubclass) rules.TableSubclass {
	s := rules.TableSubclass{TableEntry: te, Class: b.GetClassKey(), Level: int(b.GetLevel()), DescPT: b.GetDescPt()}
	for _, l := range b.GetLevels() {
		s.Levels = append(s.Levels, rules.TableSubclassLevel{
			Level: int(l.GetLevel()), Features: tableFeaturesOf(l.GetFeatures()),
			TableLevel: tableLevelOf(l.GetCantripsKnown(), l.GetSpellsKnown(), l.GetSlots()),
		})
	}
	if c := b.GetCasting(); c != nil {
		s.Casting = tableCastingOf(c)
	}
	for _, ap := range b.GetAlwaysPrepared() {
		s.AlwaysPrepared = append(s.AlwaysPrepared, rules.TableAlwaysPrepared{ClassLevel: int(ap.GetClassLevel()), Spell: ap.GetSpellKey()})
	}
	return s
}

func tableRaceOf(te rules.TableEntry, b *rulesv1.TableRace) rules.TableRace {
	r := rules.TableRace{
		TableEntry: te, Size: b.GetSize(), SpeedFt: int(b.GetSpeedFt()), AbilityBonuses: tableScoresOf(b.GetAbilityBonuses()),
		DarkvisionFt: int(b.GetDarkvisionFt()), Languages: b.GetLanguages(), LanguageChoices: int(b.GetLanguageChoices()),
		Traits: tableFeaturesOf(b.GetTraits()),
	}
	for _, n := range b.GetChoiceBonuses() {
		r.ChoiceBonuses = append(r.ChoiceBonuses, int(n))
	}
	return r
}

func tableFeaturesOf(in []*rulesv1.TableFeature) []rules.TableFeature {
	var out []rules.TableFeature
	for _, f := range in {
		out = append(out, tableFeatureOf(f))
	}
	return out
}

func tableFeatureOf(f *rulesv1.TableFeature) rules.TableFeature {
	out := rules.TableFeature{Key: f.GetKey(), NamePT: f.GetNamePt(), DescPT: f.GetDescPt()}
	for _, e := range f.GetEffects() {
		out.Effects = append(out.Effects, rules.Effect{
			Type: e.GetType(), Target: e.GetTarget(), Mode: e.GetMode(), Value: e.GetValue(), When: e.GetWhen(), Tags: e.GetTags(),
			Proficiency: e.GetProficiency(), Level: e.GetLevel(), Roll: e.GetRoll(), Targets: e.GetTargets(),
			Sense: e.GetSense(), RangeFt: int(e.GetRangeFt()), Resource: e.GetResource(), Max: e.GetMax(), Recharge: e.GetRecharge(),
			Choice: e.GetChoice(), Count: int(e.GetCount()), From: e.GetFrom(), Economy: e.GetEconomy(),
			Spells: e.GetSpells(), TextPT: e.GetTextPt(),
		})
	}
	return out
}

var (
	castUnitOf = map[rulesv1.CastingTimeUnit]string{
		rulesv1.CastingTimeUnit_CASTING_TIME_UNIT_ACTION:       rules.CastAction,
		rulesv1.CastingTimeUnit_CASTING_TIME_UNIT_BONUS_ACTION: rules.CastBonusAction,
		rulesv1.CastingTimeUnit_CASTING_TIME_UNIT_REACTION:     rules.CastReaction,
		rulesv1.CastingTimeUnit_CASTING_TIME_UNIT_MINUTE:       rules.CastMinute,
		rulesv1.CastingTimeUnit_CASTING_TIME_UNIT_HOUR:         rules.CastHour,
	}
	rangeKindOf = map[rulesv1.SpellRangeKind]string{
		rulesv1.SpellRangeKind_SPELL_RANGE_KIND_SELF:      rules.RangeSelf,
		rulesv1.SpellRangeKind_SPELL_RANGE_KIND_TOUCH:     rules.RangeTouch,
		rulesv1.SpellRangeKind_SPELL_RANGE_KIND_RANGED:    rules.RangeRanged,
		rulesv1.SpellRangeKind_SPELL_RANGE_KIND_SIGHT:     rules.RangeSight,
		rulesv1.SpellRangeKind_SPELL_RANGE_KIND_UNLIMITED: rules.RangeUnlimited,
		rulesv1.SpellRangeKind_SPELL_RANGE_KIND_SPECIAL:   rules.RangeSpecial, // the engine refuses it
	}
	durationKindOf = map[rulesv1.SpellDurationKind]string{
		rulesv1.SpellDurationKind_SPELL_DURATION_KIND_INSTANTANEOUS:   rules.DurationInstantaneous,
		rulesv1.SpellDurationKind_SPELL_DURATION_KIND_TIMED:           rules.DurationTimed,
		rulesv1.SpellDurationKind_SPELL_DURATION_KIND_UNTIL_DISPELLED: rules.DurationUntilDispelled,
		rulesv1.SpellDurationKind_SPELL_DURATION_KIND_SPECIAL:         rules.DurationSpecial, // the engine refuses it
	}
	durationUnitOf = map[rulesv1.SpellDurationUnit]string{
		rulesv1.SpellDurationUnit_SPELL_DURATION_UNIT_ROUND:  rules.DurationRound,
		rulesv1.SpellDurationUnit_SPELL_DURATION_UNIT_MINUTE: rules.DurationMinute,
		rulesv1.SpellDurationUnit_SPELL_DURATION_UNIT_HOUR:   rules.DurationHour,
		rulesv1.SpellDurationUnit_SPELL_DURATION_UNIT_DAY:    rules.DurationDay,
	}
	targetKindOf = map[rulesv1.TableSpellTargetKind]string{
		rulesv1.TableSpellTargetKind_TABLE_SPELL_TARGET_KIND_CREATURE:  rules.TargetCreature,
		rulesv1.TableSpellTargetKind_TABLE_SPELL_TARGET_KIND_CREATURES: rules.TargetCreatures,
		rulesv1.TableSpellTargetKind_TABLE_SPELL_TARGET_KIND_AREA:      rules.TargetArea,
		rulesv1.TableSpellTargetKind_TABLE_SPELL_TARGET_KIND_SELF:      rules.TargetSelf,
	}
	shapeOf = map[rulesv1.TableAreaShape]string{
		rulesv1.TableAreaShape_TABLE_AREA_SHAPE_CONE:     rules.ShapeCone,
		rulesv1.TableAreaShape_TABLE_AREA_SHAPE_CUBE:     rules.ShapeCube,
		rulesv1.TableAreaShape_TABLE_AREA_SHAPE_CYLINDER: rules.ShapeCylinder,
		rulesv1.TableAreaShape_TABLE_AREA_SHAPE_LINE:     rules.ShapeLine,
		rulesv1.TableAreaShape_TABLE_AREA_SHAPE_SPHERE:   rules.ShapeSphere,
	}
	saveSuccessOf = map[rulesv1.SpellSaveSuccess]string{
		rulesv1.SpellSaveSuccess_SPELL_SAVE_SUCCESS_NONE:  "none",
		rulesv1.SpellSaveSuccess_SPELL_SAVE_SUCCESS_HALF:  "half",
		rulesv1.SpellSaveSuccess_SPELL_SAVE_SUCCESS_OTHER: "other", // the engine refuses it
	}
)

func tableSpellOf(te rules.TableEntry, b *rulesv1.TableSpell) rules.TableSpell {
	t := b.GetTarget()
	s := rules.TableSpell{
		TableEntry: te, Level: int(b.GetLevel()), School: b.GetSchoolKey(),
		CastingTime: rules.TableCastingTime{
			Unit: castUnitOf[b.GetCastingTime().GetUnit()], Amount: int(b.GetCastingTime().GetAmount()), TriggerPT: b.GetCastingTime().GetTriggerPt(),
		},
		Range: rules.TableRange{Kind: rangeKindOf[b.GetRange().GetKind()], DistanceFt: int(b.GetRange().GetDistanceFt())},
		Duration: rules.TableDuration{
			Kind: durationKindOf[b.GetDuration().GetKind()], Amount: int(b.GetDuration().GetAmount()),
			Unit: durationUnitOf[b.GetDuration().GetUnit()], UpTo: b.GetDuration().GetUpTo(),
		},
		Components: rules.TableComponents{
			Verbal: b.GetComponents().GetVerbal(), Somatic: b.GetComponents().GetSomatic(),
			Material: b.GetComponents().GetMaterial(), MaterialPT: b.GetComponents().GetMaterialPt(),
		},
		Concentration: b.GetConcentration(), Ritual: b.GetRitual(), Classes: b.GetClassKeys(),
		DescPT: b.GetDescPt(), HigherLevelPT: b.GetHigherLevelPt(),
		Target: rules.SpellTarget{
			Kind: targetKindOf[t.GetKind()], Count: int(t.GetCount()), PerSlotLevel: int(t.GetPerSlotLevel()),
			Shape: shapeOf[t.GetShape()], SizeFt: int(t.GetSizeFt()),
		},
		Attack: b.GetAttack(),
	}
	if sv := b.GetSave(); sv != nil {
		s.Save = &rules.SpellSave{Ability: abilityFromProto[sv.GetAbility()], OnSuccess: saveSuccessOf[sv.GetOnSuccess()]}
	}
	for _, d := range b.GetDamage() {
		s.Damage = append(s.Damage, rules.TableSpellDamage{
			Type: d.GetDamageTypeKey(), Dice: d.GetDice(), PerSlotLevel: d.GetPerSlotLevel(), PerTier: d.GetPerTier(),
		})
	}
	if h := b.GetHeal(); h != nil {
		s.Heal = &rules.TableSpellHeal{Dice: h.GetDice(), PerSlotLevel: h.GetPerSlotLevel(), AddsModifier: h.GetAddsModifier()}
	}
	return s
}

// slotsLength refuses a slots list that is not nine counts or none: the engine's
// row has nine, and padding a short list would hide a typo.
func slotsLength(field string, slots []int32) *rulesv1.TableContentViolation {
	if n := len(slots); n != 0 && n != 9 {
		return &rulesv1.TableContentViolation{
			Field: field, Reason: rules.ReasonTable, Message: fmt.Sprintf("the spell slots are nine counts (1st to 9th circle) or none, not %d", n),
		}
	}
	return nil
}

// checkShape checks what the messages cannot say for themselves and the engine
// never sees: the lengths of the slots lists.
func checkShape(body tableBody) []*rulesv1.TableContentViolation {
	var out []*rulesv1.TableContentViolation
	add := func(v *rulesv1.TableContentViolation) {
		if v != nil {
			out = append(out, v)
		}
	}
	switch b := body.(type) {
	case *rulesv1.TableClass:
		for i, l := range b.GetLevels() {
			add(slotsLength(fmt.Sprintf("table_class.levels[%d].slots", i), l.GetSlots()))
		}
	case *rulesv1.TableSubclass:
		for i, l := range b.GetLevels() {
			add(slotsLength(fmt.Sprintf("table_subclass.levels[%d].slots", i), l.GetSlots()))
		}
	}
	return out
}

// storedData is the protojson that goes in campaign_content.data.
func storedData(body tableBody) ([]byte, error) {
	return storeJSON.Marshal(body)
}

// entryToProto is the TableEntry of a stored entry. inUse is the number of
// characters using it, or -1 when the caller may not see it.
func entryToProto(e entryRow, inUse int) *rulesv1.TableEntry {
	out := &rulesv1.TableEntry{
		Key: e.row.ContentKey, Kind: e.kind, NamePt: e.row.NamePt, Archived: e.row.ArchivedAt != nil,
		Revision: e.row.Revision, CreatedAt: timestamppb.New(e.row.CreatedAt), UpdatedAt: timestamppb.New(e.row.UpdatedAt),
		ArchivedAt: timestamp(e.row.ArchivedAt),
	}
	if inUse > 0 {
		out.CharactersUsing = i32(inUse)
	}
	setBody(out, e.body)
	return out
}

// violationsOf turns what With returned into the violations of a refusal. entry is
// the key of the entry being written ("" for the whole content): an error in it is
// reported at the body's own field names ("table_class.levels[4]..."), and one in
// another entry names that entry's key (a write can break another entry, such as a
// spell list a class reuses).
func violationsOf(err error, idx overlayIndex, entry string) []*rulesv1.TableContentViolation {
	oe, ok := asOverlayError(err)
	if !ok {
		return []*rulesv1.TableContentViolation{{Reason: rules.ReasonOverlay, Message: err.Error()}}
	}
	// An entry with several things wrong comes back with every one of them (MR-025):
	// the editor marks each field at once.
	var out []*rulesv1.TableContentViolation
	for _, o := range oe.Violations() {
		out = append(out, violationOf(o, idx, entry))
	}
	return out
}

// violationOf is one overlay error as the API's violation: the path of the field
// in the request's body, and the key when the entry is not the one being written.
func violationOf(oe *rules.OverlayError, idx overlayIndex, entry string) *rulesv1.TableContentViolation {
	v := &rulesv1.TableContentViolation{Reason: oe.Reason, Message: oe.Message}
	if v.Reason == "" {
		v.Reason = rules.ReasonValue
	}
	m := overlayPath.FindStringSubmatch(oe.Field)
	if m == nil {
		v.Field = oe.Field
		return v
	}
	n, _ := strconv.Atoi(m[2])
	list := idx[m[1]]
	if n < 0 || n >= len(list) {
		v.Field = oe.Field
		return v
	}
	e := list[n]
	v.Field = "table_" + kindPrefix(e.kind) + m[3]
	if e.row.ContentKey != entry {
		v.Key = e.row.ContentKey
	}
	return v
}

// overlayPath splits "classes[2].levels[4]" into the kind's list, the index and
// the rest.
var overlayPath = regexp.MustCompile(`^(classes|subclasses|races|subraces|backgrounds|spells)\[(\d+)\](.*)$`)

// slugify makes the slug of a key from a Portuguese name: lower case, without
// accents, the rest of it a-z0-9 joined by hyphens, at most rules.MaxSlugLength.
func slugify(name string) string {
	var b strings.Builder
	hyphen := true // no leading hyphen
	for _, r := range strings.ToLower(name) {
		if plain, ok := accents[r]; ok {
			r = plain
		}
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			hyphen = false
		case !hyphen:
			b.WriteByte('-')
			hyphen = true
		}
	}
	return strings.Trim(truncate(b.String(), rules.MaxSlugLength), "-")
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

var accents = func() map[rune]rune {
	m := map[rune]rune{}
	for plain, with := range map[rune]string{
		'a': "áàâãäå", 'e': "éèêë", 'i': "íìîï", 'o': "óòôõö", 'u': "úùûü", 'c': "ç", 'n': "ñ", 'y': "ýÿ",
	} {
		for _, r := range with {
			m[r] = plain
		}
	}
	return m
}()

// reservedStemWords are the words a slug may not have (rules.checkTableKey):
// joined, they stop being the engine's words.
var reservedStemWords = []string{"magical-secrets", "wild-shape", "ability-score-improvement"}

// safeSlug makes a slug the engine accepts: not empty, without a reserved word
// and not ending in "spellcasting".
func safeSlug(slug, fallback string) string {
	if slug == "" {
		slug = fallback
	}
	for _, w := range reservedStemWords {
		slug = strings.ReplaceAll(slug, w, strings.ReplaceAll(w, "-", ""))
	}
	if slug == "spellcasting" || strings.HasSuffix(slug, "-spellcasting") {
		slug += "-x"
	}
	return slug
}

// uniqueSlug is slug, or slug-2, slug-3... the first the taken set does not
// have, within the engine's length limit. It adds the result to taken.
func uniqueSlug(slug string, taken map[string]bool) string {
	candidate := slug
	for n := 2; taken[candidate]; n++ {
		suffix := "-" + strconv.Itoa(n)
		candidate = strings.TrimRight(truncate(slug, rules.MaxSlugLength-len(suffix)), "-") + suffix
	}
	taken[candidate] = true
	return candidate
}

// entryKey is the key of a new entry: "<kind>:<slug>@mesa", the slug made from
// the name and unique among the campaign's keys (archived entries included).
func entryKey(kind rulesv1.TableContentKind, name string, rows []charactersdb.CampaignContent) string {
	prefix := kindPrefix(kind)
	taken := map[string]bool{}
	for _, r := range rows {
		if keyKind(r.ContentKey) == prefix {
			taken[slugOfStoredKey(r.ContentKey)] = true
		}
	}
	return prefix + ":" + uniqueSlug(safeSlug(slugify(name), prefix), taken) + "@mesa"
}

func asOverlayError(err error) (*rules.OverlayError, bool) {
	var oe *rules.OverlayError
	return oe, errors.As(err, &oe)
}

func slugOfStoredKey(key string) string {
	_, rest, _ := strings.Cut(key, ":")
	return strings.TrimSuffix(rest, "@mesa")
}

// checkFeatureCount refuses a body with more features than the rules engine
// allows (rules.MaxTableFeatures per class or subclass, rules.MaxTraits per
// race or subrace), before any work is done on them: a body inside the request
// limit can hold far more than that. The engine checks again, with the entry's
// path, when the entry is compiled.
func checkFeatureCount(kind rulesv1.TableContentKind, body tableBody) []*rulesv1.TableContentViolation {
	limit, what := rules.MaxTableFeatures, "features"
	switch body.(type) {
	case *rulesv1.TableRace, *rulesv1.TableSubrace:
		limit, what = rules.MaxTraits, "traits"
	}
	n := 0
	for _, g := range featureGroups(body) {
		n += len(g.features)
	}
	if n <= limit {
		return nil
	}
	return []*rulesv1.TableContentViolation{{
		Field: bodyField(kind), Reason: rules.ReasonLimit,
		Message: fmt.Sprintf("%d %s; the limit is %d", n, what, limit),
	}}
}

// featureKeys gives every feature of a body its key (ADR-0018, section 1): a
// feature that has one keeps it, if it is one the entry had; a new one gets a key
// made from its name; an empty one at a level where the stored entry has a feature
// of the same name takes that one's key (an editor that did not send it back must
// not lose the choices sheets made). old is the stored body, or nil for a new
// entry. It returns a violation for a key the entry never had, which would let a
// request claim another entry's feature.
func featureKeys(entryKey string, body, old tableBody) []*rulesv1.TableContentViolation {
	a := &keyAssigner{entryKey: entryKey, taken: map[string]bool{}, owned: map[string]bool{}, slugs: map[string]bool{}, byName: map[string]string{}}
	a.collectOld(old)
	var out []*rulesv1.TableContentViolation
	a.violations = &out
	switch b := body.(type) {
	case *rulesv1.TableClass:
		for i, l := range b.GetLevels() {
			a.assign(fmt.Sprintf("table_class.levels[%d]", i), "feature:", featureStem("class", entryKey), i+1, l.GetFeatures())
		}
	case *rulesv1.TableSubclass:
		for i, l := range b.GetLevels() {
			a.assign(fmt.Sprintf("table_subclass.levels[%d]", i), "feature:", featureStem("subclass", entryKey), int(l.GetLevel()), l.GetFeatures())
		}
	case *rulesv1.TableRace:
		a.assign("table_race", "trait:", featureStem("race", entryKey), 0, b.GetTraits())
	case *rulesv1.TableSubrace:
		a.assign("table_subrace", "trait:", featureStem("subrace", entryKey), 0, b.GetTraits())
	case *rulesv1.TableBackground:
		if f := b.GetFeature(); f != nil {
			a.assign("table_background", "background-feature:", featureStem("background", entryKey), 0, []*rulesv1.TableFeature{f})
		}
	}
	return out
}

// featureStem is the start of the keys of an entry's features: the kind, a short
// part of the entry's slug, a short hash of the whole entry key and "--" (which a
// slug never has, so the stem and the feature's own slug cannot run together). The
// hash is what keeps two entries whose long names share a beginning from making
// the same feature keys, and the short slug leaves room for the feature's name
// inside the engine's 60 characters.
func featureStem(kind, entryKey string) string {
	sum := sha256.Sum256([]byte(entryKey))
	slug := strings.TrimRight(truncate(slugOfStoredKey(entryKey), 16), "-")
	return kind + "-" + slug + "-" + hex.EncodeToString(sum[:3]) + "--"
}

type keyAssigner struct {
	entryKey string
	taken    map[string]bool // keys in use by this entry's features
	owned    map[string]bool // keys the stored entry has
	// slugs holds the slug of every key in taken and owned, kept as keys are
	// added, so a new feature's slug is checked without a pass over them all.
	slugs      map[string]bool
	byName     map[string]string
	violations *[]*rulesv1.TableContentViolation
}

// featuresOfBody lists the (level, features) groups of a body, in order.
func featureGroups(body tableBody) []struct {
	level    int
	features []*rulesv1.TableFeature
} {
	type group = struct {
		level    int
		features []*rulesv1.TableFeature
	}
	var out []group
	switch b := body.(type) {
	case *rulesv1.TableClass:
		for i, l := range b.GetLevels() {
			out = append(out, group{i + 1, l.GetFeatures()})
		}
	case *rulesv1.TableSubclass:
		for _, l := range b.GetLevels() {
			out = append(out, group{int(l.GetLevel()), l.GetFeatures()})
		}
	case *rulesv1.TableRace:
		out = append(out, group{0, b.GetTraits()})
	case *rulesv1.TableSubrace:
		out = append(out, group{0, b.GetTraits()})
	case *rulesv1.TableBackground:
		if f := b.GetFeature(); f != nil {
			out = append(out, group{0, []*rulesv1.TableFeature{f}})
		}
	}
	return out
}

func (a *keyAssigner) collectOld(old tableBody) {
	if old == nil {
		return
	}
	for _, g := range featureGroups(old) {
		for _, f := range g.features {
			a.owned[f.GetKey()] = true
			a.slugs[slugOfStoredKey(f.GetKey())] = true
			a.byName[strconv.Itoa(g.level)+"\x00"+f.GetNamePt()] = f.GetKey()
		}
	}
}

// take marks key as in use by the entry's features.
func (a *keyAssigner) take(key string) {
	a.taken[key] = true
	a.slugs[slugOfStoredKey(key)] = true
}

// assign sets the keys of one group of features. Keys already on the features
// are claimed first, so a new one never takes a slug an existing one has.
func (a *keyAssigner) assign(path, prefix, stem string, level int, features []*rulesv1.TableFeature) {
	for i, f := range features {
		if f.GetKey() == "" {
			continue
		}
		if !a.owned[f.GetKey()] {
			*a.violations = append(*a.violations, &rulesv1.TableContentViolation{
				Field: fmt.Sprintf("%s.features[%d].key", path, i), Reason: reasonImmutable,
				Message: "a feature key is made by the server; send back only the ones the entry has",
			})
			continue
		}
		a.take(f.GetKey())
	}
	for _, f := range features {
		if f.GetKey() != "" && a.owned[f.GetKey()] {
			continue
		}
		if f.GetKey() == "" {
			if k := a.byName[strconv.Itoa(level)+"\x00"+f.GetNamePt()]; k != "" && !a.taken[k] {
				f.Key = k
				a.take(k)
				continue
			}
			slug := slugify(f.GetNamePt())
			full := strings.Trim(truncate(stem+safeSlug(slug, "feature"), rules.MaxSlugLength), "-")
			f.Key = prefix + uniqueSlug(safeSlug(full, "feature"), a.slugs) + "@mesa" // marks the slug in a.slugs
			a.take(f.Key)
		}
	}
}

// parentHidden says whether an entry's required parent (a subclass's class, a
// subrace's race) is hidden from the players: retired or switched off.
func parentHidden(e entryRow, hidden func(string) bool) bool {
	switch b := e.body.(type) {
	case *rulesv1.TableSubclass:
		return hidden(b.GetClassKey())
	case *rulesv1.TableSubrace:
		return hidden(b.GetRaceKey())
	}
	return false
}

// forPlayer is a copy of an entry for a player (RN-23): no hidden key (retired or
// switched off, the SRD's too) in any reference of its body (the classes of a
// spell, the list a casting reads from, the spells an effect grants or a subclass
// always prepares).
func forPlayer(e entryRow, hidden func(string) bool) entryRow {
	if hidden == nil {
		return e // nothing is hidden: the entry goes as it is
	}
	body := proto.Clone(e.body).(tableBody)
	dropEffects := func(fs []*rulesv1.TableFeature) {
		for _, f := range fs {
			for _, ef := range f.GetEffects() {
				ef.Spells = slices.DeleteFunc(ef.Spells, func(k string) bool { return hidden(k) })
				ef.From = slices.DeleteFunc(ef.From, func(k string) bool { return hidden(k) })
			}
		}
	}
	casting := func(c *rulesv1.TableCasting) {
		if c != nil && hidden(c.GetListFrom()) {
			c.ListFrom = ""
		}
	}
	switch b := body.(type) {
	case *rulesv1.TableClass:
		casting(b.GetCasting())
		for _, l := range b.GetLevels() {
			dropEffects(l.GetFeatures())
		}
	case *rulesv1.TableSubclass:
		casting(b.GetCasting())
		b.AlwaysPrepared = slices.DeleteFunc(b.AlwaysPrepared, func(ap *rulesv1.TableAlwaysPrepared) bool { return hidden(ap.GetSpellKey()) })
		for _, l := range b.GetLevels() {
			dropEffects(l.GetFeatures())
		}
	case *rulesv1.TableRace:
		dropEffects(b.GetTraits())
	case *rulesv1.TableSubrace:
		dropEffects(b.GetTraits())
	case *rulesv1.TableBackground:
		if b.GetFeature() != nil {
			dropEffects([]*rulesv1.TableFeature{b.GetFeature()})
		}
	case *rulesv1.TableSpell:
		b.ClassKeys = slices.DeleteFunc(b.ClassKeys, func(k string) bool { return hidden(k) })
	}
	e.body = body
	return e
}
