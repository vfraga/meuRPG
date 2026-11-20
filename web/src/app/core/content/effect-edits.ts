import type { EffectDraft } from './effect-draft.types';

/** The effect after the rolls it covers were checked: a damage type holds only on damage alone, so checking anything else drops it. */
export function withRolls(effect: EffectDraft, rolls: string[]): EffectDraft {
  const only = rolls.length === 1 && rolls[0] === 'damage';
  return { ...effect, rolls, damageType: only ? effect.damageType : '' };
}

/** The effect after its resource was picked from the list; taking it away drops the cost that went with it. */
export function withResource(effect: EffectDraft, resource: string): EffectDraft {
  return { ...effect, resource, cost: resource === '' ? '' : effect.cost };
}
