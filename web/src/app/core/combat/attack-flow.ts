import {
  AttackOutcome,
  type AttackRoll,
  CombatantSide,
  type HeldRollOffer,
  type PendingDamage,
  PendingDamageStatus,
  type TargetInReach,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { asksLucky } from '../held-rolls/held-roll-offer';
import { metersFixed, metersText } from '../units';
import { joinDots, tight } from '../format/text';
import { listing } from './cover';
import { diceNaturalMark, type NaturalMark } from './natural-mark';
import { stateWord } from './combat-view';

/**
 * The steps of the attack sheet (E6-07): Alvo, Rolar, Dano. The sheet's
 * logic is kept here as small pure functions, so the stepper, the
 * messages and the result lines are tested without a DOM.
 */

/** Where the attack is: choosing a target, rolling the d20, answering the question of a held roll (the halfling's Lucky
 * about a 1, then the Bardic Inspiration die), rolling the damage, or finished (a miss, or the damage rolled). */
export type AttackStage = 'target' | 'roll' | 'lucky' | 'inspire' | 'damage' | 'done';

export interface Step {
  readonly name: 'Alvo' | 'Rolar' | 'Dano';
  readonly state: 'done' | 'current' | 'todo';
}

const ORDER: readonly Step['name'][] = ['Alvo', 'Rolar', 'Dano'];
const AT: Record<AttackStage, number> = {
  target: 0,
  roll: 1,
  lucky: 1,
  inspire: 1,
  damage: 2,
  done: 3,
};

/** The stepper: the steps before the stage are done (a check), the stage's
 * own is current (`aria-current="step"`), the rest wait. */
export function steps(stage: AttackStage): Step[] {
  return ORDER.map((name, i) => ({
    name,
    state: i < AT[stage] ? 'done' : i === AT[stage] ? 'current' : 'todo',
  }));
}

/** Where the attack goes after the d20: a hit with damage to roll goes on
 * to Dano; a miss, or a hit that has no damage to roll, is finished. */
export function stageAfterRoll(
  outcome: AttackOutcome,
  pending: PendingDamage | undefined,
): AttackStage {
  const hit = outcome === AttackOutcome.HIT || outcome === AttackOutcome.CRITICAL_HIT;
  return hit && pending?.status === PendingDamageStatus.AWAITING_ROLL ? 'damage' : 'done';
}

/** Where the attack goes after the server answered a roll or a question: the Lucky question comes first, then the
 * Bardic Inspiration one; a roll a reaction holds is finished on this sheet; else the d20's result. */
export function stageOfAnswer(res: {
  readonly roll: AttackRoll;
  readonly pending?: PendingDamage;
  readonly held?: HeldRollOffer;
}): AttackStage {
  if (res.held) {
    return asksLucky(res.held) ? 'lucky' : 'inspire';
  }
  return res.roll.heldForReaction ? 'done' : stageAfterRoll(res.roll.outcome, res.pending);
}

/** A hit whose damage waits for the target's reaction (Escudo): the attacker has nothing to roll yet,
 * and the damage is not "none". */
export function awaitsReaction(pending: PendingDamage | null | undefined): boolean {
  return pending?.status === PendingDamageStatus.AWAITING_REACTION;
}

/** The word of the outcome pill. */
export function outcomeWord(outcome: AttackOutcome): string {
  switch (outcome) {
    case AttackOutcome.CRITICAL_HIT:
      return 'Crítico';
    case AttackOutcome.HIT:
      return 'Acertou';
    default:
      return 'Errou';
  }
}

/**
 * "1 natural" beside "Errou": a natural 1 on an attack roll always misses, whatever the modifiers (SRD 5.1,
 * "Rolling 1 or 20"), and the table should see that it was the die, not the armor class. `''` for any other roll.
 */
export function missMark(roll: Pick<AttackRoll, 'outcome' | 'd20'>): NaturalMark {
  const mark = roll.outcome === AttackOutcome.MISS ? diceNaturalMark(roll.d20) : '';
  return mark === '1 natural' ? mark : '';
}

export function isHit(outcome: AttackOutcome): boolean {
  return outcome === AttackOutcome.HIT || outcome === AttackOutcome.CRITICAL_HIT;
}

/** A target row of the first step. */
export interface TargetRow {
  readonly id: string;
  readonly label: string;
  /** "Ferido · a 7,5 m". */
  readonly sub: string;
  /** Why it can't be chosen: "Longe demais: alcance de 36 m". */
  readonly blocked: string;
  /** The cover it has against this attacker, with its source ("Meia cobertura
   * (do mapa)"), or `''`; and the pictogram that goes with it. */
  readonly cover: string;
  readonly coverMark: 'half' | 'three' | null;
}

/** The rows of the target list. `rangeFt` is the attack's reach, said when
 * a target is beyond it. */
export function targetRows(targets: readonly TargetInReach[], rangeFt: number): TargetRow[] {
  return targets.flatMap((t) => {
    const cover = listing(t);
    // Total cover from the map leaves the target out: the list never says what stands in the way.
    if (cover.kind === 'left-out') {
      return [];
    }
    const parts: string[] = [];
    const word = stateWord(t.state);
    if (word) {
      parts.push(word);
    }
    if (t.distanceFt !== undefined) {
      parts.push(`a ${metersFixed(t.distanceFt)}`);
    }
    return [
      {
        id: t.combatantId,
        label: t.label,
        sub: tight(joinDots(parts)),
        blocked:
          cover.kind === 'blocked'
            ? cover.text
            : t.tooFar
              ? tight(`Longe demais: alcance de ${metersText(rangeFt)}`)
              : '',
        cover: cover.kind === 'listed' ? cover.text : '',
        coverMark: cover.kind === 'listed' ? cover.mark : null,
      },
    ];
  });
}

/**
 * The creature an attack starts aimed at: the nearest one in reach, and the first of the list among equals; with none
 * in reach (or none measurable) the first of the list. The list itself keeps the server's order. Never a creature the
 * attack cannot reach, while another it can reach stands there (a sword's turn used to start on the first of the
 * order, 3 m away, with two others at 1,5 m).
 */
export function nearestInReach(targets: readonly TargetInReach[]): string {
  let best: TargetInReach | undefined;
  for (const t of targets) {
    if (t.tooFar || t.untargetable) {
      continue;
    }
    if (!best || (t.distanceFt ?? Infinity) < (best.distanceFt ?? Infinity)) {
      best = t;
    }
  }
  return (best ?? targets[0])?.combatantId ?? '';
}

/** Whether a creature of side `of` is the enemy of one of side `to`: the party against the enemies. An NPC marked "Aliado" is in the
 * party; no other pairing is hostile. */
export function isHostile(of: CombatantSide, to: CombatantSide): boolean {
  return (
    (of === CombatantSide.PARTY && to === CombatantSide.ENEMY) ||
    (of === CombatantSide.ENEMY && to === CombatantSide.PARTY)
  );
}

/**
 * The creature an attack starts aimed at: the nearest hostile creature in reach (a player's character, for an NPC); then the
 * nearest hostile out of reach; never an ally, unless no other creature exists, and then the nearest of the others
 * (`nearestInReach`). `isFoe` says whether the creature with that id is hostile to the attacker.
 */
export function defaultTarget(
  targets: readonly TargetInReach[],
  isFoe: (combatantId: string) => boolean,
): string {
  const foes = targets.filter((t) => isFoe(t.combatantId));
  const open = foes.filter((t) => !t.untargetable);
  const inReach = nearestInReach(open.filter((t) => !t.tooFar));
  if (inReach !== '') {
    return inReach;
  }
  const byDistance = (a: TargetInReach, b: TargetInReach) =>
    (a.distanceFt ?? Infinity) - (b.distanceFt ?? Infinity);
  const out = [...(open.length > 0 ? open : foes)].sort(byDistance)[0];
  return out?.combatantId ?? nearestInReach(targets);
}

/** The line about the target after the damage is rolled: "Goblin 2
 * derrotado", or the wait for the master when the target is a player's
 * character, or the target's new state word. */
export function targetAfter(
  label: string,
  targetIsPlayer: boolean,
  pending: PendingDamage,
  stateAfter: string,
): string {
  if (pending.targetDefeated) {
    return `${label} derrotado`;
  }
  if (targetIsPlayer || pending.status === PendingDamageStatus.ROLLED) {
    return 'Esperando o mestre aplicar o dano';
  }
  return stateAfter ? `${label}: ${stateAfter.toLowerCase()}` : label;
}

/** The target's hit points after a damage: temporary points go first, then
 * the hit points, never below 0 (RN-02). */
export function hitPointsAfter(current: number, temporary: number, amount: number): number {
  return Math.max(0, current - Math.max(0, amount - temporary));
}

/** "Toren: 26 de 31 PV, depois 21". */
export function hitPointsLine(label: string, current: number, max: number, after: number): string {
  return `${label}: ${current} de ${max} PV, depois ${after}`;
}

/** Whether a damage waits for something: a reaction, its roll (or the sum of one damage type after the master took an
 * extra out of its group), or the master's apply. The master's turn options also list, with the status APPLIED, the damage a
 * take-out could still undo: that one is done, it holds nothing back. */
export function waitsForSomething(p: PendingDamage): boolean {
  switch (p.status) {
    case PendingDamageStatus.AWAITING_REACTION:
    case PendingDamageStatus.AWAITING_ROLL:
    case PendingDamageStatus.AWAITING_GROUP_SUM:
    case PendingDamageStatus.ROLLED:
      return true;
    case PendingDamageStatus.APPLIED:
    case PendingDamageStatus.DISCARDED:
    case PendingDamageStatus.UNSPECIFIED:
      return false;
  }
}

/** The damages of the attacker's turn options that wait: what a screen counts, shows or blocks the next attack for. */
export function openPendingDamages(pendings: readonly PendingDamage[]): PendingDamage[] {
  return pendings.filter(waitsForSomething);
}

/** What the master still owes a hit: the damage waiting for "Aplicar", or
 * waiting to be rolled. `null` when nothing is pending. */
export function pendingNote(
  pendings: readonly PendingDamage[],
  attackerLabel: (p: PendingDamage) => string | undefined = () => undefined,
): string | null {
  const rolled = pendings.filter((p) => p.status === PendingDamageStatus.ROLLED);
  if (rolled.length > 0) {
    const sum = rolled.reduce((n, p) => n + p.amount, 0);
    return `Falta aplicar ${sum} de dano`;
  }
  if (pendings.some((p) => p.status === PendingDamageStatus.AWAITING_REACTION)) {
    return 'Falta a reação do alvo';
  }
  const roll = pendings.find((p) => p.status === PendingDamageStatus.AWAITING_ROLL);
  const sum = pendings.find((p) => p.status === PendingDamageStatus.AWAITING_GROUP_SUM);
  if (sum && !roll) {
    // A take-out asked the attacker for the sum of one damage type again: whose it is.
    const who = attackerLabel(sum);
    return who ? `Falta a soma de ${who}` : 'Falta rolar o dano';
  }
  return roll || sum ? 'Falta rolar o dano' : null;
}
