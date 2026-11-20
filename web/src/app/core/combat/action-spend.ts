import {
  type Combatant,
  CombatantKind,
  CombatantState,
} from '../../../gen/meurpg/play/v1/combat_pb';
import type { ActionOption, DiceFormula } from '../../../gen/meurpg/rules/v1/rules_pb';
import { stateWord } from './combat-view';
import { featureUsesText, isReactionHint, reasonText } from './combat-options';

/**
 * The cost of a table feature's action (MR-025): it spends `cost` uses of its resource (1 when unset) and, with a
 * `cost_max` above that, the player chooses how many ("quantos?"). Everything here is for DISPLAY: the range, the
 * preview and the "faltam usos" line. The server reads the number picked in the same transaction as the use and refuses one
 * outside the range, so a wrong preview never changes what is spent.
 */

/** The least a use spends: the cost, and never below one. */
function leastCost(o: ActionOption): number {
  return Math.max(o.action?.cost ?? 0, 1);
}

/** Whether the use asks "quantos?": `cost_max` above the least the action spends. */
export function asksHowMany(o: ActionOption): boolean {
  return (o.action?.costMax ?? 0) > leastCost(o);
}

/** The resource of the action cannot pay its least cost: the row is off ("faltam usos"). An action with no resource never is. */
export function notEnoughUses(o: ActionOption): boolean {
  return !!o.action?.resourceKey && o.usesLeft < leastCost(o);
}

/** The numbers "quantos?" offers: the least cost to the smaller of `cost_max` and the uses left. `null` for a fixed cost or when
 * the resource cannot pay the least. */
export function spendRange(o: ActionOption): { min: number; max: number } | null {
  if (!asksHowMany(o) || notEnoughUses(o)) {
    return null;
  }
  return { min: leastCost(o), max: Math.min(o.action?.costMax ?? 0, o.usesLeft) };
}

/** "3d6", "1d6 + 2": the dice as the sheet writes them (the same sides add their counts); empty for no dice. */
function diceText(count: number, sides: number, bonus: number): string {
  const dice = count > 0 && sides > 0 ? `${count}d${sides}` : '';
  if (bonus === 0) {
    return dice || '';
  }
  const flat = String(Math.abs(bonus));
  return dice ? `${dice} ${bonus < 0 ? '−' : '+'} ${flat}` : String(bonus);
}

/** The base heal plus `extra` times the heal of each point above the cost (the dice of both have the same sides). */
function healText(
  base: DiceFormula | undefined,
  perPoint: DiceFormula | undefined,
  extra: number,
): string {
  const sides = base?.sides || perPoint?.sides || 0;
  const count = (base?.count ?? 0) + (perPoint?.count ?? 0) * extra;
  return diceText(count, sides, (base?.bonus ?? 0) + (perPoint?.bonus ?? 0) * extra);
}

/** "uso" or "usos". */
function usesWord(n: number): string {
  return n === 1 ? 'uso' : 'usos';
}

/** What spending `n` will cost and give: "Gasta 3 · cura 3d6 · 4 PV temporários". */
export function spendPreview(o: ActionOption, n: number): string {
  const a = o.action;
  const extra = Math.max(0, n - leastCost(o));
  const heal = healText(a?.heal, a?.healPerPoint, extra);
  const temp = (a?.tempHp ?? 0) + (a?.tempHpPerPoint ?? 0) * extra;
  const gives = [heal ? `cura ${heal}` : '', temp > 0 ? `${temp} PV temporários` : ''].filter(
    (p) => p !== '',
  );
  return [gives.length > 0 ? `Gasta ${n}` : `Gasta ${n} ${usesWord(n)}`, ...gives].join(' · ');
}

/** The row's detail line for an action with a cost above one, or a range: "Gasta de 1 a 3 · restam 5". `''` for a plain
 * one-use action, whose line is the usual one. */
export function costDetail(o: ActionOption): string {
  const a = o.action;
  const min = leastCost(o);
  if (notEnoughUses(o)) {
    return `Faltam usos: gasta ${min}, restam ${o.usesLeft}`;
  }
  const range = spendRange(o);
  if (range) {
    return `Gasta de ${range.min} a ${range.max} · restam ${o.usesLeft}`;
  }
  return a?.resourceKey && min > 1 ? `Gasta ${min} · restam ${o.usesLeft}` : '';
}

/** Whether a feature row is off: the server turned it off (a reaction's hint is not a block), or the pool cannot pay the cost. */
export function featureIsOff(o: ActionOption): boolean {
  return (!o.enabled && !isReactionHint(o.reason)) || notEnoughUses(o);
}

/** Why the row is off: "Faltam usos" when the pool cannot pay the least cost and has some left, else the server's own words,
 * else "Faltam usos" for a pool that is empty without a reason from the server. */
export function featureWhy(o: ActionOption): string {
  const server = reasonText(o.reason);
  if (!notEnoughUses(o)) {
    return server;
  }
  return o.usesLeft > 0 ? 'Faltam usos' : server || 'Faltam usos';
}

/** The line under a feature's name: what a use costs and what is left ("Gasta de 1 a 3 · restam 5"), or "2 usos". */
export function featureLine(o: ActionOption): string {
  return costDetail(o) || featureUsesText(o.action?.resourceKey ?? '', o.usesLeft);
}

/** The action heals or gives temporary hit points: it asks who gets them. */
export function givesResult(o: ActionOption): boolean {
  const a = o.action;
  return !!a && (!!a.heal || a.tempHp > 0 || a.tempHpPerPoint > 0 || !!a.healPerPoint);
}

/** The use asks something first: how many points, who gets the result, or the dice of the heal. */
export function opensSheet(o: ActionOption): boolean {
  return asksHowMany(o) || givesResult(o);
}

/** The dice the heal rolls when `n` points are spent (the bonus once); `null` when the action rolls nothing. */
export function healDice(
  o: ActionOption,
  n: number,
): { count: number; sides: number; bonus: number } | null {
  const base = zeroDice(o.action?.heal);
  const per = zeroDice(o.action?.healPerPoint);
  const extra = Math.max(0, n - leastCost(o));
  const count = base.count + per.count * extra;
  const sides = base.sides || per.sides;
  return count > 0 && sides > 0 ? { count, sides, bonus: base.bonus + per.bonus * extra } : null;
}

/** A dice formula, with zeros for an unset one. */
function zeroDice(d: DiceFormula | undefined): { count: number; sides: number; bonus: number } {
  return { count: d?.count ?? 0, sides: d?.sides ?? 0, bonus: d?.bonus ?? 0 };
}

/** One creature the heal may go to. */
export interface AllyRow {
  readonly id: string;
  readonly label: string;
  /** "Ferido". */
  readonly sub: string;
}

/** The other player characters of the combat the caller sees (the server sends only those) that are alive and not hidden: who a
 * heal may go to besides the actor. The server checks the choice again and refuses a creature or a dead one. */
export function allyRows(combatants: readonly Combatant[], ownId: string): AllyRow[] {
  return combatants
    .filter(
      (c) =>
        c.kind === CombatantKind.PLAYER &&
        c.id !== ownId &&
        !c.hidden &&
        c.state !== CombatantState.DEAD &&
        c.state !== CombatantState.DEFEATED,
    )
    .map((c) => ({ id: c.id, label: c.label, sub: stateWord(c.state) }));
}
