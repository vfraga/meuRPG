package play

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// A roll can wait for a question before it resolves: the halfling's Lucky about a 1, a Bardic
// Inspiration die about the roll. inspiration_holds keeps such a roll, of every kind, the weapon attack included
// (combat_inspiration.go, inspiration_held.go); the store of this file gives it the one shape the gate (held_gate.go)
// works on, heldRoll, and writes an answer back. roll_holds, where an older release kept the weapon attack, is empty
// and unused.

// heldKind is the kind of roll that waits; held_registry.go says what each one does.
type heldKind string

// The kinds of roll that wait, which are the values of inspiration_holds.kind. The first four are rolled out of a combat (the trap search and the hint try
// also by a character in one, when the die is its combatant's); spell_attack, effect_save and
// concentration_save only in a combat, by a player's character.
const (
	heldWeaponAttack      heldKind = "weapon_attack"
	heldSpellAttack       heldKind = "spell_attack"
	heldEffectSave        heldKind = "effect_save"
	heldConcentrationSave heldKind = "concentration_save"
	heldSceneCheck        heldKind = "scene_check"
	heldGroupCheck        heldKind = "group_check"
	heldTrapSearch        heldKind = "trap_search"
	heldHintTry           heldKind = "hint_try"
	heldDeathSave         heldKind = "death_save"
	heldHide              heldKind = "hide_check"
	heldCounterspell      heldKind = "counterspell_check"
	heldContest           heldKind = "contest_check"
	heldInitiative        heldKind = "initiative"
)

// checksTheAnswer says the answer to a roll of the kind is refused when its question went stale (its combat round
// is over) or its die is gone. A weapon attack answers by the attack's own checks.
func checksTheAnswer(kind heldKind) bool { return kind != heldWeaponAttack }

// checksTheAnswer is checksTheAnswer for a roll as found.
func (h heldRoll) checksTheAnswer() bool { return checksTheAnswer(h.Kind) }

// heldRoll is one held roll, as the gate works on it.
type heldRoll struct {
	ID, CampaignID, CharacterID, CombatantID, EncounterID, UserID string
	// Key is the key of the request that made the roll wait, without the "#window" a reaction
	// window's replay adds (Window says it did).
	Key                       string
	Kind                      heldKind
	Window                    bool
	Physical                  bool
	Request                   []byte
	Faces                     []int
	Counted                   int
	Modifier                  int
	Total                     int
	Round                     *int32
	LuckyIndex, LuckyTo       *int32
	LuckyAnswerKey, AnswerKey *string
	// Indomitable's question (held_indomitable.go): asked, whether it may say the save failed, its answer's key
	// and the d20 that counted before and after the new roll; and the Bardic Inspiration die answered while it
	// follows (its key, and the die the player added).
	IndomAsk, IndomFailed bool
	IndomAnswerKey        *string
	IndomFrom, IndomTo    *int32
	DieAnswerKey          *string
	DieUsed               *bonusUse
	// Stroke of Luck's question (held_stroke.go): asked, whether it may say the roll missed or failed, its answer's key
	// and the d20 that counted when the player used the feature.
	StrokeAsk, StrokeFailed bool
	StrokeAnswerKey         *string
	StrokeFrom              *int32
	// CreatedAt is when the roll was held.
	CreatedAt time.Time
}

// heldRoller is who rolls: the character, and its combatant in a combat. Combatant is nil out of
// one.
type heldRoller struct {
	CampaignID, CharacterID, UserID string
	Combatant                       *playdb.Combatant
	Round                           int32
}

// in is the roller as the state found it: its combatant and the round of its combat, for the character
// of a roll that does not know where it is.
func (r heldRoller) in(st rollerState) heldRoller {
	r.Combatant, r.Round = st.Die.Combatant, st.Die.Round
	return r
}

// holdClass is the kind of roll an RPC answers: the questions of an attack roll and of every other roll have
// their own RPCs, and a hold of the other class is not found there, whichever table it is kept in.
type holdClass int

const (
	anyHold holdClass = iota
	attackHold
	outsideHold
)

// admits says the RPC of the class answers a roll of the kind, in the combat the reference names (none
// when it names none).
func (c holdClass) admits(h heldRoll, encounter string) bool {
	switch c {
	case attackHold:
		return h.Kind == heldWeaponAttack && (encounter == "" || h.EncounterID == encounter)
	case outsideHold:
		return h.Kind != heldWeaponAttack
	}
	return true
}

// holdRef says which hold to look for. Encounter is for the weapon attacks, which must belong to it.
type holdRef struct {
	Class                   holdClass
	Campaign, Encounter, ID string
}

// heldStores is the store of the held rolls.
type heldStores struct{ st inspirationHoldStore }

func newHeldStores(q *playdb.Queries) heldStores { return heldStores{st: inspirationHoldStore{q}} }

// byID finds a roll of the class the reference names.
func (hs heldStores) byID(ctx context.Context, ref holdRef) (heldRoll, error) {
	h, err := hs.st.byID(ctx, ref)
	if err == nil && !ref.Class.admits(h, ref.Encounter) {
		return heldRoll{}, pgx.ErrNoRows
	}
	return h, err
}

// byIDOf finds the roll of a kind.
func (hs heldStores) byIDOf(ctx context.Context, ref holdRef) (heldRoll, error) {
	return hs.st.byID(ctx, ref)
}

func (hs heldStores) openOf(ctx context.Context, roller heldRoller) ([]heldRoll, error) {
	return hs.st.openOf(ctx, roller)
}

func (hs heldStores) insert(ctx context.Context, env gateEnv, h heldRoll) (heldRoll, error) {
	return hs.st.insert(ctx, env, h)
}

func (hs heldStores) answerLucky(ctx context.Context, before, after heldRoll, staysHeld bool) error {
	return hs.st.answerLucky(ctx, before, after, staysHeld)
}

func (hs heldStores) askIndomitable(ctx context.Context, h heldRoll) error {
	return hs.st.askIndomitable(ctx, h)
}

func (hs heldStores) answerIndomitable(ctx context.Context, h heldRoll) error {
	return hs.st.answerIndomitable(ctx, h)
}

func (hs heldStores) answer(ctx context.Context, h heldRoll, key string) error {
	return hs.st.answer(ctx, h, key)
}

func (hs heldStores) forget(ctx context.Context, h heldRoll) error { return hs.st.forget(ctx, h) }

func (hs heldStores) asked(ctx context.Context, env gateEnv, h heldRoll) error {
	return hs.st.asked(ctx, env, h)
}

// byKeyOf finds the roll of a kind the request with a key made. A reaction window's replay keeps its question under
// the key of the request it replays, which a retry of the request is not the author of.
func (hs heldStores) byKeyOf(ctx context.Context, kind heldKind, roller heldRoller, key string) (heldRoll, bool, error) {
	h, found, err := hs.st.byKey(ctx, roller, key)
	if err != nil || !found || h.Kind != kind || h.Window {
		return heldRoll{}, false, err
	}
	return h, true, nil
}

// heldOfInspirationHold is an inspiration_holds row in the shape of the gate.
func heldOfInspirationHold(row playdb.InspirationHold) heldRoll {
	return heldRoll{
		ID: row.ID, CampaignID: row.CampaignID, CharacterID: row.CharacterID, UserID: row.UserID,
		CombatantID: deref(row.CombatantID), EncounterID: deref(row.EncounterID), Window: row.WindowHold,
		Key: row.IdempotencyKey, Kind: heldKind(row.Kind), Physical: row.Physical, Request: row.Request,
		Faces: intsOf(row.Faces), Counted: int(row.Counted), Modifier: int(row.Modifier), Total: int(row.Total),
		Round: row.Round, LuckyIndex: row.LuckyIndex, LuckyTo: row.LuckyTo, LuckyAnswerKey: row.LuckyAnswerKey,
		AnswerKey: row.AnswerKey, CreatedAt: row.CreatedAt,
		IndomAsk: row.IndomitableAsk, IndomFailed: row.IndomitableFailed, IndomAnswerKey: row.IndomitableAnswerKey,
		IndomFrom: row.IndomitableFrom, IndomTo: row.IndomitableTo, DieAnswerKey: row.DieAnswerKey, DieUsed: dieUsedOf(row.DieUsed),
		StrokeAsk: row.StrokeAsk, StrokeFailed: row.StrokeFailed, StrokeAnswerKey: row.StrokeAnswerKey, StrokeFrom: row.StrokeFrom,
	}
}

// inspirationHoldStore keeps the held rolls, in inspiration_holds: scoped by the campaign, owned by the character
// (and by its combatant in a combat).
type inspirationHoldStore struct{ q *playdb.Queries }

func (st inspirationHoldStore) byKey(ctx context.Context, roller heldRoller, key string) (heldRoll, bool, error) {
	row, err := st.q.GetInspirationHoldByKey(ctx, playdb.GetInspirationHoldByKeyParams{CampaignID: roller.CampaignID, IdempotencyKey: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return heldRoll{}, false, nil
	}
	if err != nil {
		return heldRoll{}, false, fmt.Errorf("find the held roll: %w", err)
	}
	return heldOfInspirationHold(row), true, nil
}

func (st inspirationHoldStore) byID(ctx context.Context, ref holdRef) (heldRoll, error) {
	row, err := st.q.GetInspirationHold(ctx, playdb.GetInspirationHoldParams{CampaignID: ref.Campaign, ID: ref.ID})
	if err != nil {
		return heldRoll{}, err //nolint:wrapcheck // the caller says what it was looking for
	}
	return heldOfInspirationHold(row), nil
}

// openOf lists the rolls the roller owns that wait: its combatant's, in a combat, and the character's that
// name no combatant (made out of a combat, or before the column was written). A combatant that is not
// backed by a character (a creature, an NPC's copy) owns only its own, whatever character_id it carries.
func (st inspirationHoldStore) openOf(ctx context.Context, roller heldRoller) ([]heldRoll, error) {
	owner := holdOwnerOf(roller)
	if owner.CombatantID == nil && owner.CharacterID == nil {
		return nil, nil
	}
	rows, err := st.q.ListOpenInspirationHolds(ctx, playdb.ListOpenInspirationHoldsParams(owner))
	if err != nil {
		return nil, fmt.Errorf("list the held rolls: %w", err)
	}
	out := make([]heldRoll, len(rows))
	for i, row := range rows {
		out[i] = heldOfInspirationHold(row)
	}
	return out, nil
}

// holdOwner is whom the holds of inspiration_holds belong to: the combatant, in its combat, and the character
// that backs it (or that rolls out of a combat). A nil field matches nothing.
type holdOwner struct{ EncounterID, CombatantID, CharacterID *string }

// holdOwnerOfCombatant is the owner of the holds of a combatant that a character backs.
func holdOwnerOfCombatant(c playdb.Combatant) holdOwner {
	r := heldRoller{Combatant: &c}
	if backedByCharacter(c) {
		r.CharacterID = c.CharacterID
	}
	return holdOwnerOf(r)
}

// holdOwnerOf is the owner of the roller's holds.
func holdOwnerOf(r heldRoller) holdOwner {
	var o holdOwner
	if r.Combatant != nil {
		o.EncounterID, o.CombatantID = &r.Combatant.EncounterID, &r.Combatant.ID
	}
	if r.CharacterID != "" {
		o.CharacterID = &r.CharacterID
	}
	return o
}

func (st inspirationHoldStore) insert(ctx context.Context, env gateEnv, h heldRoll) (heldRoll, error) {
	row, err := st.q.InsertInspirationHold(ctx, playdb.InsertInspirationHoldParams{
		CampaignID: h.CampaignID, CharacterID: h.CharacterID, UserID: h.UserID, Kind: string(h.Kind), IdempotencyKey: h.Key,
		Request: h.Request, Faces: faces32(h.Faces), Modifier: clamp32(h.Modifier, -heldBonusLimit, heldBonusLimit),
		Total: clamp32(h.Total, -heldBonusLimit, heldBonusLimit), Counted: clamp32(h.Counted, 0, 1), Physical: h.Physical,
		CreatedAt: env.now, Round: h.Round, LuckyIndex: h.LuckyIndex,
		CombatantID: nilIfEmpty(h.CombatantID), EncounterID: nilIfEmpty(h.EncounterID), WindowHold: h.Window && h.Kind == heldWeaponAttack,
		IndomitableAsk: h.IndomAsk, IndomitableFailed: h.IndomFailed, StrokeAsk: h.StrokeAsk, StrokeFailed: h.StrokeFailed,
	})
	if err != nil {
		return heldRoll{}, fmt.Errorf("hold the roll: %w", err)
	}
	return heldOfInspirationHold(row), nil
}

// heldBonusLimit bounds the modifier and the total a hold keeps: no sheet gets near it.
const heldBonusLimit = 1000

// answerLucky writes the d20 after the Lucky answer (the faces, the one that counts and the total), the
// answer's key and the new face. The roll is read from here by the die's question and by the settled
// roll alike, so it is written whether the die comes next or not.
func (st inspirationHoldStore) answerLucky(ctx context.Context, _, after heldRoll, _ bool) error {
	_, err := st.q.AnswerInspirationHoldLucky(ctx, playdb.AnswerInspirationHoldLuckyParams{
		ID: after.ID, Faces: faces32(after.Faces), Counted: clamp32(after.Counted, 0, 1),
		Total: clamp32(after.Total, -heldBonusLimit, heldBonusLimit), LuckyAnswerKey: after.LuckyAnswerKey, LuckyTo: after.LuckyTo,
	})
	return err //nolint:wrapcheck // the question wraps it
}

func (st inspirationHoldStore) answer(ctx context.Context, h heldRoll, key string) error {
	return st.q.AnswerInspirationHold(ctx, playdb.AnswerInspirationHoldParams{ID: h.ID, AnswerKey: &key}) //nolint:wrapcheck // the gate wraps it
}

func (st inspirationHoldStore) forget(ctx context.Context, h heldRoll) error {
	return st.q.DeleteInspirationHold(ctx, h.ID) //nolint:wrapcheck // the gate wraps it
}

// asked moves the combat's revision for the question of any roll held in a combat: the master and the roller's player
// read the combat again to see it (the combatant carries its question). A roll out of a combat has no combat to move.
func (inspirationHoldStore) asked(ctx context.Context, env gateEnv, _ heldRoll) error {
	if env.c == nil {
		return nil
	}
	enc, err := env.c.q.TouchEncounter(ctx, env.c.enc.ID)
	if err != nil {
		return fmt.Errorf("touch the encounter: %w", err)
	}
	env.c.enc = enc
	return nil
}
