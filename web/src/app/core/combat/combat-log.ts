import {
  AttackOutcome,
  type CombatLogEntry,
  CombatLogKind,
  type CombatLogRound,
  PendingDamageStatus,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { article } from '../format/article';
import { logLine } from './combat-log-lines';
import { LogContext, LogGroup, LogLine, NO_CONTEXT } from './combat-log-types';

/**
 * The combat log as the screens read it (E6-11, E6-14, E6-15): the server
 * sends structured entries, and the sentence is written here, in
 * Portuguese, from what each kind carries. A kind this app doesn't know
 * (a newer server's) is skipped, never shown as an empty line.
 */

// The pieces live in the `combat-log-*` files; the screens and the specs read the log through this one.
export { article };
export { logLine } from './combat-log-lines';
export { monstersAddedText } from './combat-log-actions';
export { pairOf } from './combat-log-attack';
export type {
  LogContext,
  LogGroup,
  LogLine,
  LogRoll,
  PoolCard,
  PoolCardRow,
} from './combat-log-types';

/** What the master did for a player's character carries "(pelo mestre)" at the end of its line, for everyone who reads it. */
function byMaster(e: CombatLogEntry, line: LogLine | null): LogLine | null {
  return line && e.byMaster ? { ...line, text: `${line.text} (pelo mestre)` } : line;
}

/** The rounds as the log draws them: latest first, the current round
 * "em andamento". Every round after the first ends with "A rodada N
 * começou", the earliest line of its group. */
export function logGroups(
  rounds: readonly CombatLogRound[],
  currentRound: number,
  encounterName = '',
  ctx: LogContext = NO_CONTEXT,
): LogGroup[] {
  return rounds.map((r) => {
    const lines = r.entries
      .map((e, i) => byMaster(e, logLine(e, encounterName, ctx, r.entries[i + 1])))
      .filter((l): l is LogLine => l !== null);
    if (r.round > 1) {
      lines.push({
        id: `round-${r.round}`,
        icon: 'skip_next',
        actor: '',
        text: `A rodada ${r.round} começou`,
        hidden: false,
        undoable: false,
      });
    }
    return {
      round: r.round,
      // Round 0 is before the combat begins: the monsters the master put in while it was being set up.
      title: r.round === 0 ? 'Antes do combate' : `Rodada ${r.round}`,
      status:
        r.round === 0
          ? currentRound === 0
            ? 'em preparação'
            : 'encerrada'
          : r.round >= currentRound
            ? 'em andamento'
            : 'encerrada',
      lines,
    };
  });
}

/** The newest line of all, for the collapsed log. */
export function latestLine(groups: readonly LogGroup[]): LogLine | null {
  for (const g of groups) {
    const first = g.lines.find((l) => !l.id.startsWith('round-'));
    if (first) {
      return first;
    }
  }
  return null;
}

/** How many entries the log has, for "9 entradas" (the synthetic "A rodada
 * começou" lines don't count). */
export function entryCount(groups: readonly LogGroup[]): number {
  return groups.reduce((n, g) => n + g.lines.filter((l) => !l.id.startsWith('round-')).length, 0);
}

/** "da"/"do" by the name's article. */
function ofThe(name: string): string {
  return article(name) === 'a' ? 'da' : 'do';
}

/** " (5 de dano)", " (errou)" or nothing: what the attack to be undone did. */
function attackUndoDamage(e: CombatLogEntry): string {
  if (e.outcome === AttackOutcome.MISS) {
    return ' (errou)';
  }
  return e.damage &&
    e.damage.status !== PendingDamageStatus.AWAITING_ROLL &&
    e.damage.status !== PendingDamageStatus.AWAITING_GROUP_SUM
    ? ` (${e.damage.amount} de dano)`
    : '';
}

/** One label per kind of entry the master may undo; the other kinds read "a última ação". */
const UNDO_LABELS: ReadonlyMap<CombatLogKind, (e: CombatLogEntry) => string> = new Map([
  [
    CombatLogKind.ATTACK,
    (e) => {
      const target = e.targetLabel || 'alguém';
      const to = article(target) === 'a' ? 'à' : 'ao';
      return `o ataque ${ofThe(e.actorLabel)} ${e.actorLabel} ${to} ${target}${attackUndoDamage(e)}`;
    },
  ],
  [
    CombatLogKind.ACTION,
    (e) => `a ação de ${e.actorLabel}${e.keyNamePt ? ` (${e.keyNamePt})` : ''}`,
  ],
  [
    CombatLogKind.FEATURE_USED,
    (e) =>
      `o uso de ${e.featureUsed?.namePt || e.keyNamePt || 'uma característica'} ${ofThe(e.actorLabel)} ${e.actorLabel}`,
  ],
  [CombatLogKind.HIT_POINTS_ADJUSTED, (e) => `o ajuste de PV de ${e.actorLabel || e.targetLabel}`],
  [
    CombatLogKind.SPELL_CAST,
    (e) => `a magia ${e.keyNamePt || ''} ${ofThe(e.actorLabel)} ${e.actorLabel}`.replace('  ', ' '),
  ],
  [CombatLogKind.REACTION, (e) => `a reação ${ofThe(e.actorLabel)} ${e.actorLabel}`],
  [
    CombatLogKind.LEGENDARY_SPENT,
    (e) => `o gasto de ações lendárias ${ofThe(e.actorLabel)} ${e.actorLabel}`,
  ],
  [
    CombatLogKind.OPPORTUNITY_OFFERED,
    (e) => `a oferta de ataque de oportunidade a ${e.targetLabel || 'alguém'}`,
  ],
  [
    CombatLogKind.DEATH_SAVE,
    (e) => `o teste contra a morte ${ofThe(e.actorLabel)} ${e.actorLabel}`,
  ],
  [
    CombatLogKind.CONDITIONS_CHANGED,
    (e) => `a mudança de condições ${ofThe(e.targetLabel)} ${e.targetLabel}`,
  ],
]);

/** The entry the undo would take back, named for the confirmation:
 * "o ataque do Capitão Goblin ao Toren (5 de dano)". */
export function undoLabel(e: CombatLogEntry): string {
  return UNDO_LABELS.get(e.kind)?.(e) ?? 'a última ação';
}

/** The last entry the master may undo, from the groups' source rounds. */
export function undoableEntry(rounds: readonly CombatLogRound[]): CombatLogEntry | null {
  for (const r of rounds) {
    const e = r.entries.find((x) => x.undoable);
    if (e) {
      return e;
    }
  }
  return null;
}

/** The first `max` lines of the log, latest first, keeping the rounds they
 * belong to: the player's panel on a laptop shows a short stretch and
 * "Ver o registro todo" opens the rest. */
export function truncateGroups(groups: readonly LogGroup[], max: number): LogGroup[] {
  const out: LogGroup[] = [];
  let left = max;
  for (const g of groups) {
    if (left <= 0) {
      break;
    }
    const lines = g.lines.slice(0, left);
    left -= lines.length;
    out.push({ ...g, lines });
  }
  return out;
}
