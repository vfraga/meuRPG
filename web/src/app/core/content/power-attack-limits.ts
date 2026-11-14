import type { EffectDraft } from './effect-draft.types';

/** The bounds of a `power_attack` (backend/internal/rules/powerattack.go): the penalty taken off the attack roll, the flat damage added to
 * a hit and how many weapon properties the filter may name. The effect menu says the numbers' ranges (`powerLimits` reads them); these
 * are what a stored effect is judged by before any menu is at hand (`keptOf`), and the fallback of a menu that does not say. */
export interface PowerAttackLimits {
  readonly penalty: { readonly min: number; readonly max: number };
  readonly damage: { readonly min: number; readonly max: number };
  readonly properties: number;
}

export const POWER_ATTACK_LIMITS: PowerAttackLimits = {
  penalty: { min: 1, max: 10 },
  damage: { min: 1, max: 20 },
  properties: 4,
};

/** The weapon kinds of a power_attack: any (also the empty field), melee or ranged. */
export const POWER_ATTACK_WEAPONS: readonly string[] = ['', 'any', 'melee', 'ranged'];

/** The limits the menu states for the type's two numbers; the fixed ones where it does not. */
export function powerLimits(menu: {
  fieldsOf(type: string): readonly { name: string; min: number; max: number }[];
}): PowerAttackLimits {
  const fields = menu.fieldsOf('power_attack');
  const range = (name: string, own: { min: number; max: number }) => {
    const f = fields.find((x) => x.name === name);
    return f && f.max > 0 ? { min: f.min, max: f.max } : own;
  };
  return {
    penalty: range('attack_penalty', POWER_ATTACK_LIMITS.penalty),
    damage: range('damage_bonus', POWER_ATTACK_LIMITS.damage),
    properties: POWER_ATTACK_LIMITS.properties,
  };
}

/** The sentence under a number outside its range ("De 1 a 10."). */
export function rangeText(r: { readonly min: number; readonly max: number }): string {
  return `De ${r.min} a ${r.max}.`;
}

/** The sentence under a filter that names too many properties. */
export function tooManyText(limits: PowerAttackLimits): string {
  return `Até ${limits.properties} propriedades.`;
}

/** What the editor says of a draft before the server does, by the field's proto name: a number outside its range (an empty one says
 * nothing, and one that reads as no number has its own sentence) and a filter with more properties than it may have. */
export function powerAttackIssues(
  d: Pick<EffectDraft, 'attackPenalty' | 'damageBonus' | 'weaponProperties' | 'typed'>,
  limits: PowerAttackLimits,
): Readonly<Record<string, string>> {
  const out: Record<string, string> = {};
  const outside = (name: string, n: number, r: { min: number; max: number }) => {
    const typedZero = n === 0 && d.typed?.[name] !== undefined;
    if (!Number.isNaN(n) && (n !== 0 || typedZero) && (n < r.min || n > r.max)) {
      out[name] = rangeText(r);
    }
  };
  outside('attack_penalty', d.attackPenalty, limits.penalty);
  outside('damage_bonus', d.damageBonus, limits.damage);
  if (d.weaponProperties.length > limits.properties) {
    out['weapon_properties'] = tooManyText(limits);
  }
  return out;
}

/** Whether a stored power_attack holds something the block cannot show: a number outside its range, a weapon kind that is not one of
 * the three, more properties than a filter may name or one twice. Such an effect is kept as stored (`keptOf`). */
export function outsideTheBlock(e: {
  readonly attackPenalty: number;
  readonly damageBonus: number;
  readonly weapon: string;
  readonly weaponProperties: readonly string[];
}): boolean {
  const l = POWER_ATTACK_LIMITS;
  return (
    e.attackPenalty < l.penalty.min ||
    e.attackPenalty > l.penalty.max ||
    e.damageBonus < l.damage.min ||
    e.damageBonus > l.damage.max ||
    !POWER_ATTACK_WEAPONS.includes(e.weapon) ||
    e.weaponProperties.length > l.properties ||
    new Set(e.weaponProperties).size !== e.weaponProperties.length
  );
}
