package play

import (
	"context"
	"slices"

	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// What a pool spell (Sleep, Color Spray) asks of the creatures it may touch, and what it leaves on the ones it reached (SRD 5.1, RN-20,
// RN-22). The rules are rules/combat's (PoolSkip); here are the facts read from the sheets and the stat blocks, the effect put on the
// creatures the pool reached and the end of Sleep on the creature that takes damage.

const (
	charmed = "condition:charmed"
	blinded = "condition:blinded"
)

// poolWhy is the words a skipped creature's cast tells the master, and whether the creature's own player reads them too.
type poolWhy struct {
	why   string
	owner bool
}

// poolCreatures are the creatures of the pool in the order of the targets, each with what stops the spell on it, and the words that
// say why for the master.
func (s *Service) poolCreatures(ctx context.Context, c *combatTx, k *hpCast) ([]combat.HPCreature, []poolWhy, error) {
	targs := make([]playdb.Combatant, len(k.targets))
	for i, t := range k.targets {
		targs[i] = t.c
	}
	sheets, err := c.sheetImmunities(ctx, targs)
	if err != nil {
		return nil, nil, err
	}
	states, err := s.readStates(ctx, c.tx, c.enc.ID)
	if err != nil {
		return nil, nil, err
	}
	creatures, whys := make([]combat.HPCreature, len(k.targets)), make([]poolWhy, len(k.targets))
	for i, t := range k.targets {
		traits, mods := sheets[t.c.ID], effectModsOf(states, t.c.ID)
		immune := func(condition string) bool {
			return combat.ImmuneTo(mods, condition) || sheetImmuneTo(traits, condition)
		}
		skip := combat.PoolSkip(k.spell, combat.PoolFacts{
			CreatureType: traits.CreatureType, CharmImmune: immune(charmed), FeyAncestry: traits.FeyAncestry != "",
			CannotSee: slices.Contains(t.c.Conditions, blinded) || immune(blinded), // a creature that cannot be blinded has no eyes to see with
		})
		creatures[i] = combat.HPCreature{HP: t.hp, Unconscious: slices.Contains(t.c.Conditions, unconscious), Skip: skip}
		whys[i] = poolWhyOf(skip, traits)
	}
	return creatures, whys, nil
}

// poolWhyOf is the master's reason for a skip; Fey Ancestry is the trait's own words, which the creature's player reads too.
func poolWhyOf(skip string, traits link.Traits) poolWhy {
	switch skip {
	case combat.PoolSkipUndead:
		return poolWhy{why: whySleepUndead}
	case combat.PoolSkipCharmImmune:
		return poolWhy{why: whySleepCharmImmune}
	case combat.PoolSkipFeyAncestry:
		return poolWhy{why: traits.FeyAncestry, owner: true}
	case combat.PoolSkipCannotSee:
		return poolWhy{why: whyCannotSee}
	}
	return poolWhy{}
}

// putPoolEffects puts the effect of a pool spell (a lasting effect: unconscious for a minute, blind until the end of the caster's next
// turn) on the creatures the pool reached. A druid in a beast form that falls asleep is itself again (SRD).
func (f *castFlow) putPoolEffects(ctx context.Context) error {
	content, err := f.s.contentOf(ctx, f.c)
	if err != nil {
		return err
	}
	def, ok := content.CombatSpellEffect(f.sp.Key)
	if !ok {
		return nil
	}
	if _, err := f.s.putLastingOnTargets(ctx, f.c, f.cs, f.sp, f.slotLevel, f.caster, f.targs, f.abilityKey, def, &f.out.made); err != nil {
		return err
	}
	for i, t := range f.targs {
		if f.out.made.Hits[i].Fx == fxAffected {
			if err := f.s.endFormIfAsleep(ctx, f.c, t, def.Conditions); err != nil {
				return err
			}
		}
	}
	return nil
}

// endEffectsOnDamage ends the effects of a target that end when it takes damage (Sleep: "until ... the creature takes damage"), and
// only the damaged target's: the others the same spell holds sleep on.
func (s *Service) endEffectsOnDamage(ctx context.Context, c *combatTx, target playdb.Combatant, d damageLanded) error {
	if d.hit.Amount <= 0 {
		return nil
	}
	rows, err := c.q.ListLastingEffects(ctx, c.enc.ID)
	if err != nil {
		return err
	}
	content, err := s.contentOf(ctx, c)
	if err != nil {
		return err
	}
	ending := slices.DeleteFunc(rows, func(st playdb.CombatantState) bool {
		def, ok := s.effectDef(content, conv.OrZero(st.SourceKey))
		return st.CombatantID != target.ID || !ok || !def.EndsOnDamage
	})
	if len(ending) == 0 {
		return nil
	}
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return err
	}
	return s.endEffectRows(ctx, c, cs, ending, endDamaged)
}
