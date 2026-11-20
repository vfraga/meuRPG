import { SpellRangeKind, type SpellDetails } from '../../../gen/meurpg/rules/v1/rules_pb';
import { joinDots, tight } from '../format/text';
import { damageDice } from './cast-flow';
import { poolDice } from './hp-effects';
import { metersText } from '../units';

/**
 * The line under a spell's name in "O que você pode fazer" (E8-02): where it
 * reaches and what it rolls, from the same details the cast sheet's subtitle reads:
 * "Alcance 27 m · 5d8 PV de criaturas", "Alcance 36 m · 8d6 de fogo". Empty until
 * the details are read; a cantrip's dice are the caster's (`Attack.spellDice`). Nothing is written by hand per spell.
 * `level` is the level the cast scales at (the spell's own when omitted): a grant's cast level when the grant is the way.
 */
export function spellSummary(
  details: SpellDetails | null | undefined,
  cantripDice = '',
  level = details?.spell?.level ?? 0,
): string {
  if (!details) {
    return '';
  }
  const range = rangePart(details);
  const effect = effectPart(details, level, cantripDice);
  return tight(joinDots([range, effect].filter((part) => part !== '')));
}

/** "Alcance 27 m", "Toque" or "Pessoal"; empty for a range with none of them. */
function rangePart(details: SpellDetails): string {
  const range = details.range;
  if (range?.kind === SpellRangeKind.RANGED && range.distanceFt > 0) {
    return `Alcance ${metersText(range.distanceFt)}`;
  }
  if (range?.kind === SpellRangeKind.TOUCH) {
    return 'Toque';
  }
  return range?.kind === SpellRangeKind.SELF ? 'Pessoal' : '';
}

/** What the spell rolls: the pool of hit points, the damage dice with their type or the healing. */
function effectPart(details: SpellDetails, level: number, cantripDice: string): string {
  const pool = poolDice(details, level);
  if (pool) {
    return `${pool.count}d${pool.sides} PV de criaturas`;
  }
  const dice = damageDice(details, level, cantripDice);
  if (dice) {
    const type = details.damage[0]?.damageTypePt ?? '';
    return type ? `${dice} de ${type}` : dice;
  }
  const heal = details.healBySlotLevel[level];
  return heal ? `cura ${heal.replace('MOD', 'mod.')}` : '';
}
