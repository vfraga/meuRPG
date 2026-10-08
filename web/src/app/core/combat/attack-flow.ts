import {
  AttackOutcome,
  type PendingDamage,
  PendingDamageStatus,
  type TargetInReach,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { metersFixed, metersText } from '../units';
import { joinDots, tight } from '../format/text';
import { listing } from './cover';
import { stateWord } from './combat-view';

/**
 * The steps of the attack sheet (E6-07): Alvo, Rolar, Dano. The sheet's
 * logic is kept here as small pure functions, so the stepper, the
 * messages and the result lines are tested without a DOM.
 */

/** Where the attack is: choosing a target, rolling the d20, rolling the
 * damage, or finished (a miss, or the damage rolled). */
export type AttackStage = 'target' | 'roll' | 'damage' | 'done';

export interface Step {
  readonly name: 'Alvo' | 'Rolar' | 'Dano';
  readonly state: 'done' | 'current' | 'todo';
}

const ORDER: readonly Step['name'][] = ['Alvo', 'Rolar', 'Dano'];
const AT: Record<AttackStage, number> = { target: 0, roll: 1, damage: 2, done: 3 };

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

/** What the master still owes a hit: the damage waiting for "Aplicar", or
 * waiting to be rolled. `null` when nothing is pending. */
export function pendingNote(pendings: readonly PendingDamage[]): string | null {
  const rolled = pendings.filter((p) => p.status === PendingDamageStatus.ROLLED);
  if (rolled.length > 0) {
    const sum = rolled.reduce((n, p) => n + p.amount, 0);
    return `Falta aplicar ${sum} de dano`;
  }
  if (pendings.some((p) => p.status === PendingDamageStatus.AWAITING_REACTION)) {
    return 'Falta a reação do alvo (Escudo)';
  }
  return pendings.some((p) => p.status === PendingDamageStatus.AWAITING_ROLL)
    ? 'Falta rolar o dano'
    : null;
}

/** The damages that still stand in the way of passing the turn. */
export function openDamages(pendings: readonly PendingDamage[]): PendingDamage[] {
  return pendings.filter(
    (p) =>
      p.status === PendingDamageStatus.ROLLED ||
      p.status === PendingDamageStatus.AWAITING_ROLL ||
      p.status === PendingDamageStatus.AWAITING_REACTION,
  );
}
