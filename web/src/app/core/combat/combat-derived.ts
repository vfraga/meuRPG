import {
  type Combatant,
  CombatLogKind,
  type CombatLogRound,
  CombatantKind,
  CombatantState,
  CoverDegree,
  CoverSource,
  type Encounter,
  type GetMoveOptionsResponse,
  type GetTurnOptionsResponse,
  PendingDamageStatus,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { type FallNote, fallNote } from '../traps/trap-log';
import type { Square } from './combat-grid';
import { ownCreatures } from './mine';
import { isDead, isDown } from './combat-view';
import { reachingAttacks } from './opportunity';

/** What the combat screen derives from the combat, the options and the log, as pure functions: each of the screen's
 * `computed`s that needed more than a line of logic is one of these, so it is tested without a component. */

/** The cover each combatant has against whoever has the turn, from the targets of the attacks and spells in the options. */
export function coverAgainst(
  opts: GetTurnOptionsResponse | null | undefined,
): Map<string, { cover: CoverDegree; source: CoverSource }> {
  const out = new Map<string, { cover: CoverDegree; source: CoverSource }>();
  for (const t of [
    ...(opts?.attackTargets ?? []).flatMap((a) => a.targets),
    ...(opts?.spellTargets ?? []).flatMap((s) => s.targets),
  ]) {
    if (
      !out.has(t.combatantId) &&
      t.cover !== CoverDegree.NONE &&
      t.cover !== CoverDegree.UNSPECIFIED
    ) {
      out.set(t.combatantId, { cover: t.cover, source: t.coverSource });
    }
  }
  return out;
}

/** A reactor an opportunity offer is about and the square the mover left, for the map (`OfferMark`). */
export interface OfferSquares {
  readonly reactor: Square;
  readonly left: Square | null;
}

/** The reactors drawn on the map for whoever answers: the placed ones, with the square the mover left when the offer is for them. */
export function offerMarks(e: Encounter): OfferSquares[] {
  return e.opportunityOffers.flatMap((o) => {
    const r = e.combatants.find((c) => c.id === o.reactorId && c.placed);
    return r
      ? [
          {
            reactor: { col: r.col, row: r.row },
            left: o.forYou ? { col: o.leftCol, row: o.leftRow } : null,
          },
        ]
      : [];
  });
}

/** The reach the master's map draws, as the server said it (`Reach`): from whoever is on turn, when it is placed and on. */
export function masterReach(
  who: Combatant | null,
  on: boolean,
  options: GetMoveOptionsResponse | null,
): { origin: Square; leftDft: number; squares: Square[] } | null {
  return on && who?.placed && options
    ? {
        origin: { col: who.col, row: who.row },
        leftDft: options.movementLeftDft,
        squares: options.reachable.map((q) => ({ col: q.col, row: q.row })),
      }
    : null;
}

/** The characters at three failures: the master is asked to confirm each death. */
export function dyingOf(e: Encounter | null): Combatant[] {
  return (e?.combatants ?? []).filter((c) => c.state === CombatantState.DYING);
}

/** The downed player characters whose death save is due this turn: the master rolls it when the player is away. */
export function deathDueOf(e: Encounter | null): Combatant[] {
  return (e?.combatants ?? []).filter(
    (c) => c.kind === CombatantKind.PLAYER && c.state === CombatantState.DOWN && c.deathSaveDue,
  );
}

/** The melee attacks an opportunity attack can use: off turn, with the reaction free, someone in reach and the character
 * on its feet. `opts` is the player's `GetTurnOptions`. */
export function opportunityAttacks(
  own: Combatant | null,
  opts: GetTurnOptionsResponse | null,
  canReact: boolean,
): { key: string; name: string }[] {
  if (!canReact || !own || own.reactionUsed || isDown(own) || isDead(own) || !opts?.options) {
    return [];
  }
  return reachingAttacks(opts);
}

/** The player's hit whose damage is still to roll (the sheet was closed). */
export function pendingRollOf(opts: GetTurnOptionsResponse | null, ownId: string | undefined) {
  return (
    opts?.pendingDamages.find(
      (p) => p.status === PendingDamageStatus.AWAITING_ROLL && p.attackerId === ownId,
    ) ?? null
  );
}

/** The newest attack of the one on turn was stopped by the target's Escudo (from the log). */
export function stoppedByReaction(rounds: readonly CombatLogRound[], actorId: string | undefined) {
  const entry = rounds
    .flatMap((r) => r.entries)
    .find((x) => x.kind === CombatLogKind.ATTACK && x.actorId === actorId);
  return !!entry?.stoppedByReaction;
}

/** The first trap of the round's log that caught the combatant. */
function trapNoteIn(
  round: CombatLogRound | undefined,
  combatantId: string,
  who = '',
): FallNote | null {
  for (const entry of round?.entries ?? []) {
    if (entry.kind === CombatLogKind.TRAP_TRIGGERED && entry.trap) {
      const note = fallNote(entry.trap, combatantId, who);
      if (note) {
        return note;
      }
    }
  }
  return null;
}

/** What a trap did to this player's character this round, from the combat log. */
export function trapNote(
  e: Encounter | null,
  own: Combatant | null,
  rounds: readonly CombatLogRound[],
): FallNote | null {
  if (!e || !own) {
    return null;
  }
  return trapNoteIn(
    rounds.find((r) => r.round === e.round),
    own.id,
  );
}

/** The same note for each of the player's creatures that a trap caught (a wolf or Nanquim that walked into one on its own part). */
export function creatureTrapNotes(
  e: Encounter | null,
  master: boolean,
  rounds: readonly CombatLogRound[],
): Map<string, FallNote> {
  const notes = new Map<string, FallNote>();
  if (!e || master) {
    return notes;
  }
  const round = rounds.find((r) => r.round === e.round);
  for (const c of ownCreatures(e)) {
    const note = trapNoteIn(round, c.id, c.label);
    if (note) {
      notes.set(c.id, note);
    }
  }
  return notes;
}

/** The d20 the app just rolled for a combatant's initiative, and the line the initiative card tells it: "15 + 4 = 19", "15 − 1 = 14",
 * "15 = 15" with no modifier. `null` while any of the three numbers is missing. */
export function initiativeLine(c: Combatant | undefined): { face: number; line: string } | null {
  if (
    c?.initiativeFace === undefined ||
    c.initiativeBonus === undefined ||
    c.initiative === undefined
  ) {
    return null;
  }
  const mod =
    c.initiativeBonus === 0
      ? ''
      : ` ${c.initiativeBonus < 0 ? '\u2212' : '+'} ${Math.abs(c.initiativeBonus)}`;
  return { face: c.initiativeFace, line: `${c.initiativeFace}${mod} = ${c.initiative}` };
}

/** What the player reads after a feature spent the action: Action Surge gives the action back, frenzy says so, any other is "Usou X.". */
export function featureNote(name: string, resourceKey: string, frenzy: boolean): string {
  if (frenzy) {
    return `${name}: você entrou em frenesi.`;
  }
  return resourceKey === 'action_surge' ? `${name}: você tem outra ação.` : `Usou ${name}.`;
}
