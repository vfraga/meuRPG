import { type Attack, AttackKind, Recharge } from '../../../gen/meurpg/rules/v1/rules_pb';
import type { AttackOption, PowerAttackOption } from '../../../gen/meurpg/rules/v1/rules_pb';
import { type RollMode } from '../../../gen/meurpg/play/v1/combat_rolls_pb';
import { signed } from '../effects/effects';
import { rechargeText } from './combat-errors';
import { damageText, gripDamage } from './combat-options';
import { d20Count } from './roll-mode';

/** The trades the server lists on a weapon line (a "power_attack" effect of the character's features): none for a
 * spell, a creature or an NPC, which the server leaves empty, and none for a line with no option. */
export function tradesOf(option: AttackOption | undefined): readonly PowerAttackOption[] {
  return option?.attack?.kind === AttackKind.WEAPON ? (option.powerAttacks ?? []) : [];
}

/** "−5 no ataque, +10 no dano": what the trade takes off the roll and adds to a hit, the signs from the app's `signed`. */
function numbers(o: PowerAttackOption): string {
  return `${signed(-o.penalty)} no ataque, ${signed(o.damageBonus)} no dano`;
}

/** The line of a weapon's row: "Golpe Pesado: −5 no ataque, +10 no dano". */
/** The row lines of every trade a weapon lists. */
export const tradeLinesOf = (o: AttackOption): string[] => tradesOf(o).map(tradeRowLine);

export function tradeRowLine(o: PowerAttackOption): string {
  return `${o.name}: ${numbers(o)}`;
}

/** The line under "Usar Golpe Pesado": the numbers, and that the damage needs a hit. A trade that spends a pool adds which
 * one ("gasta 1 de Fôlego do Atirador") and, when the roll leaves some, how many uses are left after it ("resta 2"). */
export function tradeOfferSub(o: PowerAttackOption): string {
  const base = `${numbers(o)} se acertar`;
  if (!o.resourceNamePt) {
    return base;
  }
  const after = o.usesLeft - 1;
  return `${base} · gasta 1 de ${o.resourceNamePt}${after > 0 ? ` · resta ${after}` : ''}`;
}

/** When the pool comes back, as the Ação lines say it ("volta no descanso curto ou longo": a long rest gives the uses of a
 * short-rest pool back too); the other recharges read as in `rechargeText`. */
export function restoredText(recharge: Recharge): string {
  switch (recharge) {
    case Recharge.SHORT_REST:
      return 'volta no descanso curto ou longo';
    case Recharge.LONG_REST:
      return 'volta no descanso longo';
    default:
      return rechargeText(recharge);
  }
}

/** Why a trade is off: with its pool named, "Sem usos de Fôlego do Atirador: volta no descanso curto ou longo." (without the
 * recharge when the server sent none); otherwise the reason the server wrote. Empty for a trade that is on. */
export function tradeWhy(o: PowerAttackOption): string {
  if (!o.disabled) {
    return '';
  }
  if (!o.resourceNamePt) {
    return o.disabledReasonPt || 'Sem usos.';
  }
  const back = o.recharge === Recharge.UNSPECIFIED ? '' : `: ${restoredText(o.recharge)}`;
  return `Sem usos de ${o.resourceNamePt}${back}.`;
}

/** The attack bonus the roll is made with: the weapon's, less the penalty of the trade picked. */
export function tradedBonus(bonus: number, picked: PowerAttackOption | undefined): number {
  return bonus - (picked?.penalty ?? 0);
}

/** "1d20 + 6", "2d20 − 3": the dice the mode rolls and the modifier. */
function attackText(mode: RollMode, bonus: number): string {
  const dice = `${d20Count(mode)}d20`;
  return bonus === 0 ? dice : `${dice} ${signed(bonus).replace(/^(.)/, '$1 ')}`;
}

/** "2d6 + 3" with the numbers spaced and the true minus; the dice and the flat number it ends with. */
function damageParts(damage: string): { readonly dice: string; readonly flat: number } {
  const m = /^(.*?)(?:\s*([+-])\s*(\d+))?$/.exec(damage.trim());
  const dice = m?.[1] ?? damage;
  return { dice, flat: m?.[3] ? Number(m[3]) * (m[2] === '-' ? -1 : 1) : 0 };
}

function withFlat(dice: string, flat: number): string {
  return flat === 0 ? dice : `${dice} ${signed(flat).replace(/^(.)/, '$1 ')}`;
}

/** The box that shows what the trade changes before the roll: the attack and the damage as they were and as they are. */
export function tradeBox(
  attack: Attack,
  picked: PowerAttackOption,
  mode: RollMode,
): {
  readonly attackBefore: string;
  readonly attackAfter: string;
  readonly damageBefore: string;
  readonly damageAfter: string;
} {
  const { dice, flat } = damageParts(gripDamage(attack));
  const type = attack.damageTypePt ? ` ${attack.damageTypePt}` : '';
  return {
    attackBefore: attackText(mode, attack.attackBonus),
    attackAfter: attackText(mode, tradedBonus(attack.attackBonus, picked)),
    damageBefore: damageText({ ...attack, damageTypePt: '' } as Attack),
    damageAfter: `${withFlat(dice, flat + picked.damageBonus)}${type}, se acertar`,
  };
}

/** The roll button with the trade on: "Rolar 1d20 + 1 no app", the dice of the mode and the new modifier. */
export function tradeRollLabel(mode: RollMode, bonus: number): string {
  return `Rolar ${attackText(mode, bonus)} no app`;
}

/** How the roll came out, for the line of the trade: the result told, or not yet (a held roll). */
export type TradeOutcome = 'hit' | 'miss' | 'unknown';

/** The line under the roll: "Golpe Pesado: −5 no ataque", and, for a miss, that the +10 does not come in. */
export function tradeRollLine(o: PowerAttackOption, outcome: TradeOutcome): string {
  const base = `${o.name}: ${signed(-o.penalty)} no ataque`;
  return outcome === 'miss' ? `${base} · sem dano, o ${signed(o.damageBonus)} não entra` : base;
}

/** Two things marked spend the one pool and it has one use left for them: the refusal says which pool and asks to unmark one. */
export function poolShortMessage(pool: string, left: number | null): string {
  const of = pool ? ` de ${pool}` : ' uso';
  const rest = left === null ? '' : `, e ${left === 1 ? 'resta 1' : `restam ${left}`}`;
  return `Sem usos suficientes: cada um gasta 1${of}${rest}. Desmarque um.`;
}

/** The key the server gives the damage part of a power attack. */
export const TRADE_PART_KEY = 'power_attack';

/** The damage part is the flat damage of a power attack: automatic, never doubled, only on a hit. */
export function isTradePart(p: { readonly key: string }): boolean {
  return p.key === TRADE_PART_KEY;
}

/** "+10": the part's flat number, written with its sign (it has no dice). */
export function tradePartFormula(p: { readonly flat: number }): string {
  return signed(p.flat);
}

/** The line under the part: why it is there, and that a critical hit leaves it alone. */
export function tradePartNote(p: { readonly flat: number }): string {
  return `Só porque acertou; num crítico, os dados dobram e o ${signed(p.flat)} não.`;
}
