package rules

import (
	"slices"
	"strings"
	"time"
)

// The kinds of item an infusion record sits on.
const (
	// InfusionItemGear is a piece of the holder's sheet: the armor, the shield or a weapon, by its gear key.
	InfusionItemGear = "gear"
	// InfusionItemObject is a thing that is no piece of the sheet, by its own name.
	InfusionItemObject = "object"
)

// InfusionRecord is one live infusion of the character as holder (Build.Infusions). The holder is the character itself.
type InfusionRecord struct {
	ID string
	// ArtificerID is the character who made the infusion. In this version the holder is always the artificer; a record
	// whose artificer is not the build's character (Build.CharacterID) is not valid, so a sheet never judges another
	// character's infusion by its own features.
	ArtificerID string
	// InfusionKey is the key of the infusion option (Content.Infusion).
	InfusionKey string
	// ItemKind is InfusionItemGear or InfusionItemObject; ItemKey is the gear key, or the object's own key.
	ItemKind, ItemKey string
	// ObjectName is the name of an object, "" for gear.
	ObjectName string
	// Parameter is the value the infusion asks for when infused: the damage type of a resistance label.
	Parameter string
	// Attuned says the holder is attuned to the item.
	Attuned bool
	Since   time.Time
}

// InfusionStatus is what one record says on the sheet now: the infusion block's row (FullSheet.infusions).
type InfusionStatus struct {
	ID, InfusionKey string
	// NamePT is the infusion's name; ItemNamePT the piece's or the object's.
	NamePT, ItemNamePT string
	ItemKind, ItemKey  string
	Parameter          string
	// Effect is the part of the infusion the app computes (InfusionEffect*); "" for a key the content does not know.
	Effect string
	// Computed says the app handles the infusion (a number or a mark); false is a text only one, which the table applies.
	Computed bool
	// Valid says the record counts: the content knows the infusion, the character has its feature, picked it and still
	// meets its prerequisite, a gear piece is on the sheet with no older infusion, a manual bonus does not stand on the same
	// piece, and the host's limit has room for it. ReasonPT says why it does not.
	Valid    bool
	ReasonPT string
	// OverLimit says the record would count but for the host's limit: the oldest of the valid ones give way to the newest.
	OverLimit bool
	// Bonus is the size of the number the infusion gives at the class level, 0 when it gives none or the record is not
	// valid; Counts says the number holds now (the piece is equipped and, when asked, attuned).
	Bonus  int
	Counts bool
	// RequiresAttunement and Attuned are the infusion's ask and the record's state.
	RequiresAttunement, Attuned bool
	// HostKey is the feature that holds the infusion, "" for a key the content does not know.
	HostKey string
}

// InfusionHostStatus is a feature of the character that holds infusions: how many may be live and how many valid
// records are.
type InfusionHostStatus struct {
	HostKey    string
	ClassLevel int
	Limit      int
	Used       int
}

// The reasons a record is not valid now, in Portuguese (InfusionStatus.ReasonPT).
const (
	reasonInfusionUnknown = "A infusão não existe mais no conteúdo."
	reasonInfusionNoHost  = "A ficha não tem a característica desta infusão."
	reasonInfusionUnpick  = "O personagem não conhece esta infusão."
	reasonInfusionManual  = "A peça já tem um bônus mágico marcado."
	reasonInfusionLost    = "A infusão perdeu o pré-requisito."
	reasonInfusionNoPiece = "A peça não está na ficha."
	reasonInfusionOther   = "A infusão é de outro artífice, e esta ficha não a avalia."
	reasonInfusionTaken   = "A peça já tem outra infusão, mais antiga."
	reasonInfusionOver    = "A infusão está acima do limite."
)

// infusionLine is a record read against the content and the sheet.
type infusionLine struct {
	rec InfusionRecord
	in  TableInfusion
	// known says the content knows the infusion key; host is its feature and level the character's level in the
	// host's class.
	known bool
	host  string
	level int
	// valid says the record counts; reasonPT says why not.
	valid    bool
	reasonPT string
	// bonus is the number the infusion gives at the class level, 0 when it gives none.
	bonus int
	// onSheet says a gear record's piece is on the sheet.
	onSheet bool
}

// gear says the record sits on a gear key.
func (l infusionLine) gear() bool { return l.rec.ItemKind == InfusionItemGear }

// infusionLines reads the records of b against the content, in order. manualStands says a manual bonus on the piece of an
// item bonus infusion makes the record not valid (the manual one stands); a save that asks whether the piece may take a
// manual bonus passes false, because the bonus it asks about is the one in b. lost are the options the character no
// longer meets the prerequisite of: their records are not valid.
// The valid records beyond the host's limit are not valid either (limitInfusions).
// Two valid records on one gear key cannot be live (one live record per piece): the oldest stands and the other is not
// valid, the one that comes first in b.Infusions when they began together.
func (c *content) infusionLines(b Build, manualStands bool, lost map[string]*LostPrerequisite) []infusionLine {
	out := make([]infusionLine, 0, len(b.Infusions))
	for _, r := range b.Infusions {
		out = append(out, c.infusionLine(b, r, manualStands, lost))
	}
	oldest := map[string]int{}
	for i, l := range out {
		if !l.valid || !l.gear() {
			continue
		}
		j, taken := oldest[l.rec.ItemKey]
		switch {
		case !taken:
			oldest[l.rec.ItemKey] = i
		case l.rec.Since.Before(out[j].rec.Since):
			out[j].valid, out[j].reasonPT, out[j].bonus = false, reasonInfusionTaken, 0
			oldest[l.rec.ItemKey] = i
		default:
			out[i].valid, out[i].reasonPT, out[i].bonus = false, reasonInfusionTaken, 0
		}
	}
	c.limitInfusions(out)
	return out
}

// readInfusions reads the records of b the way the sheet does, with the options whose prerequisite the character lost
// (what Derive leaves out): every count reads the same validity as the sheet.
func (c *content) readInfusions(b Build, manualStands bool) []infusionLine {
	if len(b.Infusions) == 0 {
		return nil
	}
	_, lost, _ := c.settleLost(b)
	return c.infusionLines(b, manualStands, lost)
}

// limitInfusions makes the valid records beyond the limit of their host not valid: the host's limit at the artificer's class
// level, the newest keeping their place and the oldest giving way (since, then id, the order the write uses). Nothing ends.
func (c *content) limitInfusions(lines []infusionLine) {
	byHost := map[string][]int{}
	for i, l := range lines {
		if l.valid {
			byHost[l.host] = append(byHost[l.host], i)
		}
	}
	for host, at := range byHost {
		limit := c.infusions.rules[host].ActiveAt(lines[at[0]].level)
		if len(at) <= limit {
			continue
		}
		slices.SortFunc(at, func(x, y int) int {
			if d := lines[x].rec.Since.Compare(lines[y].rec.Since); d != 0 {
				return d
			}
			return strings.Compare(lines[x].rec.ID, lines[y].rec.ID)
		})
		for _, i := range at[:len(at)-limit] {
			lines[i].valid, lines[i].reasonPT, lines[i].bonus = false, reasonInfusionOver, 0
		}
	}
}

func (c *content) infusionLine(b Build, r InfusionRecord, manualStands bool, lost map[string]*LostPrerequisite) infusionLine {
	l := infusionLine{rec: r}
	in, ok := c.infusions.data[r.InfusionKey]
	if !ok {
		l.reasonPT = reasonInfusionUnknown
		return l
	}
	l.in, l.known, l.host = in, true, c.infusions.host[r.InfusionKey]
	_, hasRule := c.infusions.rules[l.host]
	level, owns := c.infusionHostLevel(b, l.host)
	l.level = level
	l.onSheet = l.gear() && b.listsGear(r.ItemKey)
	switch {
	case r.ArtificerID != b.CharacterID:
		l.reasonPT = reasonInfusionOther
	case !hasRule || !owns:
		l.reasonPT = reasonInfusionNoHost
	case !slices.Contains(b.FeatureChoices, r.InfusionKey):
		l.reasonPT = reasonInfusionUnpick
	case lost[r.InfusionKey] != nil:
		l.reasonPT = reasonInfusionLost
	case l.gear() && !l.onSheet:
		l.reasonPT = reasonInfusionNoPiece
	case manualStands && l.gear() && in.Effect == InfusionEffectItemBonus && b.hasItemBonus(r.ItemKey):
		l.reasonPT = reasonInfusionManual
	default:
		l.valid, l.bonus = true, in.BonusAt(level)
	}
	return l
}

// infusionHostLevel is the character's level in the class of the host feature, and whether the character has the
// feature: the class is there at the feature's level and, for a subclass feature, with that subclass.
func (c *content) infusionHostLevel(b Build, host string) (int, bool) {
	f, ok := c.features[host]
	if !ok {
		return 0, false
	}
	level := classLevelIn(b.Classes, f.Class)
	if level < max(f.Level, 1) {
		return level, false
	}
	if f.Subclass != "" && !slices.ContainsFunc(b.Classes, func(cl ClassLevel) bool { return cl.Class == f.Class && cl.Subclass == f.Subclass }) {
		return level, false
	}
	return level, true
}

// infusesBonusOf says a record with an item bonus would be valid on the gear key if the key had no manual bonus: the key
// takes no manual bonus. The records are read as the sheet reads them (a manual bonus on another piece stands, so that
// piece's record takes no place of the limit), with the manual bonus of the key under test left out.
func (c *content) infusesBonusOf(b Build, key string) bool {
	b.ItemBonuses = slices.DeleteFunc(slices.Clone(b.ItemBonuses), func(ib ItemBonus) bool { return ib.Key == key })
	for _, l := range c.readInfusions(b, true) {
		if l.valid && l.gear() && l.rec.ItemKey == key && l.in.Effect == InfusionEffectItemBonus {
			return true
		}
	}
	return false
}

// attunedInfusions is how many valid records ask for attunement and are attuned.
func (c *content) attunedInfusions(b Build) int {
	n := 0
	for _, l := range c.readInfusions(b, true) {
		if l.valid && l.in.RequiresAttunement && l.rec.Attuned {
			n++
		}
	}
	return n
}

func (b Build) hasItemBonus(key string) bool {
	_, ok := b.itemBonus(key)
	return ok
}

// holds says the infusion's number holds as far as the record goes: the infusion asks no attunement or the holder is
// attuned to it.
func (l infusionLine) holds() bool { return !l.in.RequiresAttunement || l.rec.Attuned }

// applyInfusions puts the valid records in the deriver, once equip has left in x.b the equipped gear: the item bonus
// ones become bonuses of their piece, so the numbers come from the one path a manual bonus takes; the pieces with an
// infusion are named; and the best spell focus gives its bonus to the spell attack.
func (x *deriver) applyInfusions() {
	x.infused, x.bonusFrom = map[string]infusionLine{}, map[string]infusionLine{}
	for _, l := range x.lines {
		if !l.valid {
			continue
		}
		switch {
		case l.in.Effect == InfusionEffectSpellFocusBonus:
			x.addFocus(l)
		case l.gear():
			x.infuseGear(l)
		}
	}
}

// infuseGear names the piece of a valid gear record and, for an item bonus, gives the piece the bonus.
func (x *deriver) infuseGear(l infusionLine) {
	key := l.rec.ItemKey
	if _, taken := x.infused[key]; taken {
		return
	}
	x.infused[key] = l
	if l.in.Effect != InfusionEffectItemBonus || l.bonus < 1 {
		return
	}
	x.bonusFrom[key] = l
	x.b.ItemBonuses = append(slices.Clone(x.b.ItemBonuses), ItemBonus{Key: key, Bonus: l.bonus, RequiresAttunement: l.in.RequiresAttunement, Attuned: l.rec.Attuned})
}

// addFocus names the piece of a gear focus and lists the focus; the best one that counts sets the spell attack bonus.
func (x *deriver) addFocus(l infusionLine) {
	if l.gear() {
		x.infuseGear(l)
	}
	if l.bonus < 1 {
		return // below the first step: no number and no line
	}
	x.foci = append(x.foci, l)
	if x.focusCounts(l) && l.bonus > x.focusBonus {
		x.focusBonus = l.bonus
	}
}

// focusCounts says the focus can be used now: it is attuned when it asks, and a piece of gear is on the sheet and equipped.
func (x *deriver) focusCounts(l infusionLine) bool {
	if !l.holds() {
		return false
	}
	return !l.gear() || l.onSheet && !x.b.IsCarried(l.rec.ItemKey)
}

// infusionName is the name of the valid infusion on the gear key, or "".
func (x *deriver) infusionName(key string) string {
	l, ok := x.infused[key]
	if !ok {
		return ""
	}
	return x.infusionNamePT(l.rec.InfusionKey)
}

func (x *deriver) infusionNamePT(infusion string) string {
	if n := x.c.namePT(infusion); n != "" {
		return n
	}
	return "Infusão que a ficha não conhece"
}

// repeats says the weapon key makes its own ammunition: a valid repeating infusion on it, attuned when it asks.
func (x *deriver) repeats(key string) bool {
	l, ok := x.infused[key]
	return ok && l.in.Effect == InfusionEffectRepeatingAmmunition && l.holds()
}

// focusLines are the item modifier lines of the spell focus infusions: the one that counts says so, the others say why not.
func (x *deriver) focusLines() []ItemModifier {
	var out []ItemModifier
	used := false
	for _, l := range x.foci {
		name := l.rec.ObjectName
		if l.gear() {
			name = x.c.namePT(l.rec.ItemKey)
		}
		if name == "" {
			name = l.rec.ItemKey
		}
		reason, on := x.focusReason(l)
		if on && (l.bonus < x.focusBonus || used) {
			reason, on = "Outro foco vale mais.", false
		}
		used = used || on
		out = append(out, ItemModifier{
			Target: SpellAttackTarget, Value: l.bonus, SourceKey: l.rec.ItemKey, SourceNamePT: name, Counts: on, ReasonPT: reason,
			InfusionNamePT: x.infusionNamePT(l.rec.InfusionKey), InfusionID: l.rec.ID,
		})
	}
	return out
}

// focusReason is why a focus does not count, or "" and true when it does.
func (x *deriver) focusReason(l infusionLine) (string, bool) {
	switch {
	case l.gear() && x.b.IsCarried(l.rec.ItemKey):
		return "Carregado, não equipado.", false
	case !l.holds():
		return "Pede sintonização.", false
	case len(x.d.Spellcasting) == 0:
		return "Sem conjuração.", false
	}
	return "", true
}

// SpellAttackTarget is the target of an item modifier line that adds to the spell attack.
const SpellAttackTarget = "spell_attack"

// infusionStatuses fills Derived.Infusions and Derived.InfusionHosts once the item modifiers are listed: a record
// counts when one of its lines does.
func (x *deriver) infusionStatuses() {
	counts := map[string]bool{}
	for _, m := range x.d.ItemModifiers {
		if m.InfusionID != "" && m.Counts {
			counts[m.InfusionID] = true
		}
	}
	for _, l := range x.lines {
		x.d.Infusions = append(x.d.Infusions, x.statusOf(l, counts[l.rec.ID]))
	}
	for _, host := range sortedKeys(x.c.infusions.rules) {
		if level, owns := x.c.infusionHostLevel(x.original, host); owns {
			x.d.InfusionHosts = append(x.d.InfusionHosts, InfusionHostStatus{HostKey: host, ClassLevel: level, Limit: x.c.infusions.rules[host].ActiveAt(level), Used: x.usedOf(host)})
		}
	}
}

// usedOf is how many valid records the host holds.
func (x *deriver) usedOf(host string) int {
	n := 0
	for _, l := range x.lines {
		if l.valid && l.host == host {
			n++
		}
	}
	return n
}

func (x *deriver) statusOf(l infusionLine, counts bool) InfusionStatus {
	s := InfusionStatus{
		ID: l.rec.ID, InfusionKey: l.rec.InfusionKey, NamePT: x.infusionNamePT(l.rec.InfusionKey), ItemKind: l.rec.ItemKind, ItemKey: l.rec.ItemKey,
		Parameter: l.rec.Parameter, Valid: l.valid, ReasonPT: l.reasonPT, OverLimit: l.reasonPT == reasonInfusionOver, Counts: counts, Attuned: l.rec.Attuned,
		HostKey: l.host, ItemNamePT: l.rec.ObjectName,
	}
	if l.gear() {
		s.ItemNamePT = x.c.namePT(l.rec.ItemKey)
		if s.ItemNamePT == "" {
			s.ItemNamePT = l.rec.ItemKey
		}
	}
	if l.known {
		s.Effect, s.Computed, s.RequiresAttunement = l.in.Effect, l.in.Effect != InfusionEffectNone, l.in.RequiresAttunement
	}
	if l.valid {
		s.Bonus = l.bonus
	}
	return s
}
