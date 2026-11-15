import {
  type Combatant,
  type CombatLogRound,
  CombatLogKind,
  type PendingDamage,
  PendingDamageStatus,
} from '../../../gen/meurpg/play/v1/combat_pb';
import type { DamagePart, DamagePartRoll } from '../../../gen/meurpg/play/v1/combat_rolls_pb';
import { article } from '../format/article';
import { isSpellRider } from './combat-log-actions';
import { initialPicks, inTypedGroup, isOffPart, partTotal } from './damage-parts';

/**
 * Taking an extra out of a typed group (#835): the master's removal of a part whose sum was typed or rolled as one for
 * its damage type undoes what the damage did and asks one new sum of that type. The wire carries no reason for the part
 * taken out; it is in the combat log's removal line, which only the master and the attacker's player read with a key.
 */

/** A part the master took out, as the log says it (the master's and the attacker's reading). */
export interface Removal {
  readonly attackerId: string;
  readonly targetId: string;
  readonly key: string;
  readonly name: string;
  readonly reason: string;
}

/**
 * The removal lines of the log that carry the part's key, newest first (the log lists the latest round and entry first). A
 * removal belongs to the latest attack of that attacker on that target: when a newer attack by the same one on the same
 * target is listed before it, the removal is of an earlier attack, and a damage that waits for its sum now must not read
 * that old reason.
 */
export function removalsOf(rounds: readonly CombatLogRound[]): Removal[] {
  const newer = new Set<string>();
  const out: Removal[] = [];
  for (const e of rounds.flatMap((r) => r.entries)) {
    const pair = `${e.actorId}>${e.targetId}`;
    if (e.kind === CombatLogKind.ATTACK) {
      newer.add(pair);
    } else if (e.kind === CombatLogKind.DAMAGE_PART_REMOVED && e.key !== '' && !newer.has(pair)) {
      out.push({
        attackerId: e.actorId,
        targetId: e.targetId,
        key: e.key,
        name: e.keyNamePt,
        reason: e.reason,
      });
    }
  }
  return out;
}

/** The save a spell's hit effect made the target roll on an attack, as the log says it (the master's reading names the condition). */
export interface SpellSave {
  readonly attackerId: string;
  readonly targetId: string;
  /** The spell's key, which the master's reading shares with the damage part the effect made. */
  readonly key: string;
  /** The condition the failed save gave ("Impedido"), empty when it was passed or none came with it. */
  readonly condition: string;
}

/** The saves of spell hit effects in the log, newest first. */
export function spellSavesOf(rounds: readonly CombatLogRound[]): SpellSave[] {
  return rounds
    .flatMap((r) => r.entries)
    .filter((e) => e.kind === CombatLogKind.HIT_RIDER && isSpellRider(e) && e.key !== '')
    .map((e) => ({
      attackerId: e.actorId,
      targetId: e.targetId,
      key: e.key,
      condition: e.hitRider?.conditionNamePt ?? '',
    }));
}

/** The removal that belongs to a damage: the same attacker and target, and a part the damage has. */
export function removalFor(p: PendingDamage, removals: readonly Removal[]): Removal | undefined {
  return removals.find(
    (r) =>
      r.attackerId === p.attackerId &&
      r.targetId === p.targetId &&
      p.parts.some((x) => x.key === r.key),
  );
}

function the(name: string): string {
  return `${article(name)} ${name}`;
}

/** A part the damage always has counts unless it is off (the part of a choosable extra counts by what is marked). */
function counts(part: DamagePart): boolean {
  return !isOffPart(part);
}

/** "2d6", or "1d8 + 1d6" when the faces differ, in the order the dice first appear. */
function diceText(parts: readonly DamagePart[]): string {
  const bySides = new Map<number, number>();
  for (const x of parts) {
    if (x.diceCount > 0) {
      bySides.set(x.diceSides, (bySides.get(x.diceSides) ?? 0) + x.diceCount);
    }
  }
  return [...bySides].map(([sides, count]) => `${count}d${sides}`).join(' + ');
}

function listOf(names: readonly string[]): string {
  return names.length > 1
    ? `${names.slice(0, -1).join(', ')} e ${names[names.length - 1]}`
    : (names[0] ?? '');
}

/** What the sum of the awaited damage type is made of: the parts left, their dice, the range and the fixed number. */
export interface AwaitedSum {
  readonly typeKey: string;
  readonly typePt: string;
  readonly parts: readonly DamagePart[];
  /** "Espada curta e Sinal da Presa, perfurante: 2d6". */
  readonly label: string;
  /** "De 2 a 12." */
  readonly hint: string;
  readonly min: number;
  readonly max: number;
  /** The fixed numbers of the parts, which the app adds. */
  readonly flat: number;
  /** "2d6". */
  readonly dice: string;
}

/** The sum a damage awaits (`AWAITING_GROUP_SUM`): `null` for a damage in any other state or with no dice left in the type. */
export function awaitedSumOf(p: PendingDamage): AwaitedSum | null {
  if (p.status !== PendingDamageStatus.AWAITING_GROUP_SUM || p.awaitedDamageTypeKey === '') {
    return null;
  }
  const picks = initialPicks(p.parts);
  const parts = p.parts.filter(
    (x) =>
      x.damageTypeKey === p.awaitedDamageTypeKey &&
      (x.choosable ? picks.some((k) => k.key === x.key) : counts(x)),
  );
  const dicePart = parts.filter((x) => x.diceCount > 0);
  if (dicePart.length === 0) {
    return null;
  }
  const min = dicePart.reduce((n, x) => n + x.diceCount, 0);
  const max = dicePart.reduce((n, x) => n + x.diceCount * x.diceSides, 0);
  const typePt = parts[0]?.damageTypePt ?? '';
  const dice = diceText(dicePart);
  return {
    typeKey: p.awaitedDamageTypeKey,
    typePt,
    parts,
    label: `${listOf(dicePart.map((x) => x.labelPt))}${typePt ? `, ${typePt}` : ''}: ${dice}`,
    hint: min === max ? `Deve ser ${min}.` : `De ${min} a ${max}.`,
    min,
    max,
    flat: parts.reduce((n, x) => n + x.flat, 0),
    dice,
  };
}

/** A sum that stays while the other is asked again: "Geada no Punho 2d8 de frio: 6". */
export interface KeptRow {
  readonly label: string;
  readonly formula: string;
  readonly typePt: string;
  readonly total: number;
  /** The app adds this part by itself (a hit effect, Golpe Pesado): the row says "automático". */
  readonly auto: boolean;
}

/** What the damage keeps of the other damage types: each counted line of `part_rolls`, with its fixed number. */
export function keptRowsOf(p: PendingDamage): KeptRow[] {
  return p.partRolls
    .filter((r) => r.counted)
    .map((r) => ({
      label: r.labelPt,
      formula: r.diceCount > 0 ? `${r.diceCount}d${r.diceSides}` : '',
      typePt: p.parts.find((x) => x.key === r.partKey)?.damageTypePt ?? '',
      total: partTotal(r),
      auto: p.parts.find((x) => x.key === r.partKey)?.auto ?? false,
    }));
}

/** "Dano: 4 + 3 + 6 de frio = 13": the sum typed, the fixed numbers and what is kept, and the total they make. */
export function sumFormula(
  awaited: AwaitedSum,
  kept: readonly KeptRow[],
  typed: number,
): { text: string; total: number } {
  let text = `${typed}`;
  if (awaited.flat !== 0) {
    text += ` ${awaited.flat < 0 ? '−' : '+'} ${Math.abs(awaited.flat)}`;
  }
  for (const k of kept) {
    text += ` + ${k.total}${k.typePt ? ` de ${k.typePt}` : ''}`;
  }
  const total = typed + awaited.flat + kept.reduce((n, k) => n + k.total, 0);
  return { text: `Dano: ${text} = ${total}`, total };
}

/** What the take-out panel of one part says: the question, the list of what it undoes and the button. */
export interface TakeOutPlan {
  /** The part sits in a typed group: taking it out undoes the damage and asks a new sum. */
  readonly group: boolean;
  readonly title: string;
  /** What taking it out undoes, then the sum it asks (an applied group). */
  readonly lines: readonly string[];
  /** A group damage that was only rolled: nothing to undo, and who types again. */
  readonly notice: string;
  readonly confirm: string;
}

const ONCE_PER_TURN: ReadonlySet<string> = new Set(['sneak-attack', 'colossus-slayer']);

function hitPointsLine(p: PendingDamage, target: Combatant | undefined): string {
  const n = p.appliedAmount ?? p.amountAfterSteps ?? p.amount;
  const who = target ? the(target.label) : 'o alvo';
  const now = target?.hitPointsCurrent;
  const max = target?.hitPointsMax;
  const known =
    now !== undefined &&
    max !== undefined &&
    now > 0 &&
    (target?.hitPointsTemporary ?? 0) === 0 &&
    !target?.defeated;
  return known
    ? `Desfaz os ${n} de dano: ${who} volta a ${Math.min(max, now + n)} PV.`
    : `Desfaz os ${n} de dano: ${who} recupera os PV que perdeu.`;
}

/** What undoing a hit effect's part says: its save and the condition it gave when the log has them, else the effect in general. */
function effectLine(r: DamagePartRoll, p: PendingDamage, saves: readonly SpellSave[]): string {
  const save = saves.find(
    (x) => x.key === r.partKey && x.attackerId === p.attackerId && x.targetId === p.targetId,
  );
  if (!save) {
    return `Desfaz o efeito de ${r.labelPt} no alvo; ele vale de novo com a nova soma.`;
  }
  return `Desfaz o teste ${article(r.labelPt) === 'a' ? 'da' : 'do'} ${r.labelPt}${save.condition ? ` (${save.condition})` : ''}.`;
}

/** The panel of "Tirar" for one part of a rolled or applied damage. */
export function takeOutPlan(
  roll: DamagePartRoll,
  p: PendingDamage,
  attacker: Combatant | undefined,
  target: Combatant | undefined,
  saves: readonly SpellSave[] = [],
): TakeOutPlan {
  const rolls = p.partRolls;
  if (!inTypedGroup(roll, rolls)) {
    return {
      group: false,
      title: `Tirar ${the(roll.labelPt)}? O dano cai de ${p.amount} para ${Math.max(0, p.amount - partTotal(roll))}.`,
      lines: [],
      notice: '',
      confirm: 'Tirar',
    };
  }
  const typePt = p.parts.find((x) => x.key === roll.partKey)?.damageTypePt ?? '';
  const left = p.parts.filter(
    (x) =>
      x.key !== roll.partKey &&
      x.damageTypeKey === roll.damageTypeKey &&
      x.diceCount > 0 &&
      (x.choosable ? x.selected && x.available : counts(x)),
  );
  const who = attacker?.label ?? 'o jogador';
  const title = `Tirar ${the(roll.labelPt)} do dano?`;
  if (p.status !== PendingDamageStatus.APPLIED) {
    return {
      group: true,
      title,
      lines: [],
      notice: `O dano ainda não foi aplicado: nada é desfeito. ${who} digita de novo só a soma de ${typePt}, e o mestre aplica como sempre.`,
      confirm: 'Tirar e pedir a soma',
    };
  }
  const lines = [hitPointsLine(p, target)];
  for (const r of rolls) {
    if (r.counted && r.hitEffect && r.partKey !== roll.partKey) {
      lines.push(effectLine(r, p, saves));
    }
  }
  if (ONCE_PER_TURN.has(roll.partKey)) {
    lines.push(`Devolve ${the(roll.labelPt)} deste turno.`);
  }
  lines.push(`Pede a ${who} só a soma de ${typePt} (${diceText(left)}).`);
  return { group: true, title, lines, notice: '', confirm: 'Tirar e pedir a soma' };
}
