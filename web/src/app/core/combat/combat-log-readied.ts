import {
  type CombatLogEntry,
  CombatLogKind,
  type CombatLogReadied,
  ReadiedStatus,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { heldName } from './ready-held';
import { targetPhrase } from './ready-card';
import type { LogContext } from './combat-log-types';
import { ofSubject } from './combat-log-words';

/**
 * The lines of a readied action (#582, board 582-08). The server fills the payload (the trigger and the action) for the master and
 * the readier's player alone (RN-10): without it every line says the actor and the kind, and nothing more; a held spell is never
 * named to anyone else. A use reads as the board says it; a pass says so; a loss or a give-up says which action it was.
 */

/** "Espada longa no Zumbi 1", "Mover", "Esquivar": the held action as the line names it. */
function heldOf(r: CombatLogReadied, targetLabel: string): string {
  return heldName(
    { kind: r.kind, actionNamePt: r.actionNamePt, moveFt: 0 },
    targetPhrase(targetLabel),
  );
}

/** "Tor preparou uma ação" and, for who may read it, the action and the trigger. */
export function readiedText(e: CombatLogEntry): string {
  const r = e.readied;
  return r
    ? ` preparou uma ação: ${heldOf(r, e.targetLabel)}, “${r.triggerText}”`
    : ' preparou uma ação';
}

/** A use or a pass of the window. Without the payload the line cannot tell which: the others' entry carries neither the trigger nor the status. */
export function readiedFiredText(e: CombatLogEntry): string {
  const r = e.readiedFired;
  if (!r) {
    return ' respondeu à ação preparada';
  }
  if (r.status === ReadiedStatus.IGNORED) {
    return ` deixou passar a ação preparada${r.actionNamePt ? ` (${r.actionNamePt})` : ''}`;
  }
  return ` usou a ação preparada: ${heldOf(r, e.targetLabel)}, “${r.triggerText}”`;
}

/** The loss of the row: for who may read it, which action; a give-up says so. The others read "perdeu a ação preparada" for every loss. */
export function readiedLostLine(
  e: CombatLogEntry,
  ctx: LogContext,
): { readonly actor: string; readonly text: string } {
  const r = e.readiedLost;
  if (!r) {
    return { actor: e.actorLabel, text: ' perdeu a ação preparada' };
  }
  if (r.canceled) {
    return {
      actor: e.actorLabel,
      text: ` desistiu da ação preparada${r.actionNamePt ? ` (${r.actionNamePt})` : ''}`,
    };
  }
  return {
    actor: '',
    text: `A ${r.actionNamePt || 'ação'} preparada ${ofSubject(e.actorLabel, ctx)} se perdeu`,
  };
}

/**
 * Whether an attack's line is the use of a readied attack: the server writes the fired line of the same actor just before the
 * attack's own (`older` is the entry before it in time; a round lists the latest first). The attack carries `as_reaction` like an
 * opportunity attack does (its window is a catch window), and the wire has nothing else to tell them apart.
 */
export function followsReadiedFire(e: CombatLogEntry, older: CombatLogEntry | undefined): boolean {
  return (
    older?.kind === CombatLogKind.READIED_FIRED &&
    older.actorLabel === e.actorLabel &&
    older.readiedFired?.status !== ReadiedStatus.IGNORED
  );
}
