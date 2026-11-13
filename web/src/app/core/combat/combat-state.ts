import type { OwnHit } from './own-hit';
import { computed, signal } from '@angular/core';

import {
  type Combatant,
  CombatantState,
  type Encounter,
  EncounterStatus,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { isTheatre } from './theatre';

/** What `turn_changed` carries (play.proto). */
export interface TurnChange {
  readonly encounterId: string;
  readonly round: number;
  readonly currentCombatantId: string;
  readonly masterTurn: boolean;
  /** The combat's revision with this turn; 0 (or absent) when the server gives no number to compare (a player on a fog map). */
  readonly revision?: number;
}

/** What `combatant_moved` carries (play.proto). */
export interface CombatantMove {
  readonly encounterId: string;
  readonly combatantId: string;
  readonly col: number;
  readonly row: number;
}

/** The sentence of a window that closed by itself ("Queda Suave fechou. Você já usou a sua reação."). */
export interface ReactionNotice {
  readonly windowId: string;
  readonly text: string;
}

/**
 * A combatant as a turn that moves on finds it, until the combat is read again: its part is open and no death save is due (a
 * turn to a combatant who is down is never patched: the server decides what it owes at its start, see `applyTurn`), and one
 * that takes the turn starts it with a fresh economy (Action, bonus action, reaction and movement), which the stream's turn
 * event does not carry.
 */
function startOfTurn(c: Combatant, acts: boolean): Combatant {
  const fresh = acts && !c.defeated;
  const spent = c.actionUsed || c.bonusActionUsed || c.reactionUsed || c.movementUsedDft > 0;
  if (!c.turnPartEnded && !c.deathSaveDue && !(fresh && spent)) {
    return c;
  }
  return {
    ...c,
    turnPartEnded: false,
    deathSaveDue: false,
    ...(fresh
      ? {
          actionUsed: false,
          bonusActionUsed: false,
          reactionUsed: false,
          movementUsedDft: 0,
          movementLeftDft: c.speedDft,
          movementLeftFt: c.speedFt,
        }
      : {}),
  };
}

/** Who starts a fresh economy when the turn passes to `currentId`: it, and the creatures that share its initiative total (a joint turn). */
function takesTurn(e: Encounter, currentId: string, c: Combatant): boolean {
  const cur = e.combatants.find((x) => x.id === currentId);
  return (
    c.id === currentId ||
    (!!cur && c.kind === cur.kind && !!c.initiative && c.initiative === cur.initiative)
  );
}

/** Whether a combatant is down: at 0 hit points, or marked so. */
function isDown(c: Combatant): boolean {
  return (
    c.state === CombatantState.DOWN || (c.hitPointsCurrent !== undefined && c.hitPointsCurrent <= 0)
  );
}

/** What `CombatState.beginRead` hands out. */
export interface ReadTicket {
  readonly seq: number;
  readonly applied: number;
  readonly patched: number;
}

/**
 * The open session's combat on this screen: the encounter as the caller may
 * see it, with the small updates the page makes after its own calls and
 * after the stream's events. Pure TypeScript with signals, so its rules
 * (a read that started before a later read, or before an answer, never
 * replaces it; an answer with an older revision than the copy on screen is
 * dropped; `turn_changed` applies in place only as the very next revision and `combatant_moved` in place; a combatant the screen doesn't know
 * means "read the combat again") are tested without a DOM.
 */
export class CombatState {
  /** `null` while the session has had no combat. */
  readonly encounter = signal<Encounter | null>(null);
  /** The player is on the "Mover" page (E6-10): the session page leaves its
   * header out, since the page has its own one-line one. */
  readonly moving = signal(false);
  /** The player opened the full-screen map ("Ver mapa"): same one-line header. */
  readonly mapOpen = signal(false);
  /** Either full-page view is open. */
  readonly fullPage = computed(() => this.moving() || this.mapOpen());
  /** Goes up on every `combat_log_changed` and every (re)connection: the log
   * panel reads the log again whenever it changes. */
  readonly logTick = signal(0);
  /** The latest attack that hurt the player's own character, read from the combat log (the sheet says it as the attack's damage). */
  readonly ownHit = signal<OwnHit | null>(null);
  /** What a reaction window that closed by itself said (`reaction_window_closed`), kept in a status line until it is
   * dismissed, and gone with the combat. Nothing of it is stored in the browser. */
  readonly reactionNotice = signal<ReactionNotice | null>(null);
  /** The ended combat the person already left ("Voltar à sessão"). */
  private readonly dismissedId = signal<string | null>(null);

  /** The combat the page shows: running, or one that ended and wasn't left. */
  readonly shown = computed(() => {
    const e = this.encounter();
    return e && !(e.status === EncounterStatus.ENDED && e.id === this.dismissedId()) ? e : null;
  });

  /** Reads started, the last one applied, and the copies applied by other
   * means (answers to calls). Together they order the reads without the
   * encounter's `revision`: on a fog map it is a count of what the player
   * saw, which can be lower than the number of an earlier copy. */
  private readsStarted = 0;
  private lastReadApplied = 0;
  private applied = 0;
  /** `turn_changed` and `combatant_moved` applied in place: a read that began before one of them may not show it. */
  private patched = 0;

  /** A fresh copy that is not a read: an answer to a call, or the combat
   * being started. Of two copies of the same combat the larger revision
   * wins, so an answer that began before another change and arrives after a
   * read that showed it does not bring the older copy back; another combat
   * replaces it. The comparison is between numbers of the same kind: a read
   * (`applyRead`) sets the number on screen whatever it is, so after the fog
   * turns on the answers compare against the player's own count. */
  apply(next: Encounter | null): void {
    const current = this.encounter();
    if (next && current && next.id === current.id && next.revision < current.revision) {
      return;
    }
    this.applied++;
    this.encounter.set(next);
  }

  /** Call just before asking the server for the combat; give the ticket to
   * `applyRead` with the answer. */
  beginRead(): ReadTicket {
    return { seq: ++this.readsStarted, applied: this.applied, patched: this.patched };
  }

  /** The answer to a read. It is dropped (`false`) when a read started later
   * was applied already, or a copy was applied since this one started: that
   * copy is at least as new. A stream event applied in place since it started
   * drops it too (see `patchedSince`): the read may be older than the event. */
  applyRead(ticket: ReadTicket, next: Encounter | null): boolean {
    if (
      ticket.seq < this.lastReadApplied ||
      ticket.applied !== this.applied ||
      ticket.patched !== this.patched
    ) {
      return false;
    }
    this.lastReadApplied = ticket.seq;
    this.encounter.set(next);
    return true;
  }

  /** Whether an event was applied in place since the read began: the combat on screen is newer than what the read
   * may carry, so the page reads again (the next read begins after the event). */
  patchedSince(ticket: ReadTicket): boolean {
    return ticket.patched !== this.patched;
  }

  /** Whether anything replaced the combat on screen while the read was out: an event applied in place, or an answer to
   * a call. The read was dropped for it, but that answer may be older than a change the read was asked for (the
   * master revealed a monster while the player's own move was answered), so the page reads again: the next read
   * begins after both and shows the newest. A read dropped only because a later read was applied needs none. */
  changedSince(ticket: ReadTicket): boolean {
    return ticket.patched !== this.patched || ticket.applied !== this.applied;
  }

  private logFull = false;

  /** `combat_log_changed`, or a `ready`: read the log again, only what is new unless `full` (the log was rewritten, or who sees what changed). */
  touchLog(full = false): void {
    this.logFull ||= full;
    this.logTick.update((n) => n + 1);
  }

  /** Whether the next read of the log must be the whole log; asking clears it. */
  takeLogFull(): boolean {
    const full = this.logFull;
    this.logFull = false;
    return full;
  }

  /** The windows the stream announced (`reaction_window_opened`) in this page's life: one that was not announced came
   * with a read, after a reload, and its prompt says "Combate atualizado agora.". */
  private readonly announced = new Set<string>();

  noteReactionOpened(windowId: string): void {
    this.announced.add(windowId);
  }

  wasAnnounced(windowId: string): boolean {
    return this.announced.has(windowId);
  }

  /** `reaction_window_closed`: a window closed by itself; the sentence stays until `dismissReactionNotice`. */
  noteReactionClosed(windowId: string, text: string): void {
    this.reactionNotice.set(text ? { windowId, text } : null);
  }

  dismissReactionNotice(): void {
    this.reactionNotice.set(null);
  }

  clear(): void {
    this.applied++;
    this.reactionNotice.set(null);
    this.encounter.set(null);
    this.dismissedId.set(null);
    this.moving.set(false);
    this.mapOpen.set(false);
  }

  /** "Voltar à sessão" on the summary of an ended combat. */
  dismissEnded(): void {
    const e = this.encounter();
    if (e) {
      this.dismissedId.set(e.id);
    }
  }

  /**
   * `turn_changed`. `false` when the page must read the combat again: it is about a combat the screen doesn't have, it is
   * more than one revision ahead of the copy on screen (the copy lacks what the revisions in between changed, such as a
   * character's hit points, and a patched copy would claim a revision whose data it does not have), or it moves to a
   * combatant who is down, whose start of turn (the death save due) the server sets and the event does not carry.
   * It patches in place only the very next revision, or an event with no number (a player on a fog map), and only a turn that
   * moves on: a part that ended inside a joint turn is read again.
   */
  applyTurn(turn: TurnChange): boolean {
    const e = this.encounter();
    if (!e || e.id !== turn.encounterId) {
      return false;
    }
    const revision = turn.revision ?? 0;
    if (revision > 0 && revision <= e.revision) {
      return true; // an event no newer than the combat on screen: nothing to patch, nothing to read again
    }
    if (revision > 0 && revision !== e.revision + 1) {
      return false; // a gap: what the revisions in between changed is not in the copy
    }
    if (e.combatants.some((c) => c.id === turn.currentCombatantId && isDown(c))) {
      return false; // a down combatant's turn starts with a death save the server decides
    }
    // A part that ended inside a joint turn moves "current" to another member of the same group, in the same round. The
    // event does not say whose part ended, and a copy that is exactly one revision behind did not take the call that ended
    // it (the part was ended from another screen, or the answer is still on its way): patching only the current
    // combatant would leave the member who ended still acting. The combat is read again.
    if (turn.round === e.round && e.turnGroupIds.includes(turn.currentCombatantId)) {
      return turn.currentCombatantId === e.currentCombatantId;
    }
    this.patched++;
    this.encounter.set({
      ...e,
      // The event was the next revision and the copy took it whole: an older answer arriving now is dropped by `apply`.
      revision: Math.max(e.revision, revision),
      round: turn.round,
      currentCombatantId: turn.currentCombatantId,
      masterTurn: turn.masterTurn,
      // The group belongs to the turn that ended; until the combat is read again the turn is the current combatant's.
      turnGroupIds: [],
      combatants: e.combatants.map((c) => startOfTurn(c, takesTurn(e, turn.currentCombatantId, c))),
    });
    return true;
  }

  /** `combatant_moved`. `false` when the combatant is not known (a missed
   * `encounter_changed`): read the combat again. */
  applyMove(move: CombatantMove): boolean {
    const e = this.encounter();
    if (!e || e.id !== move.encounterId) {
      return false;
    }
    // A combat without a map has no squares (RN-25): what changed is the movement spent, which only the combat itself says.
    if (isTheatre(e)) {
      return false;
    }
    if (!e.combatants.some((c) => c.id === move.combatantId)) {
      return false;
    }
    this.patched++;
    this.encounter.set({
      ...e,
      combatants: e.combatants.map((c) =>
        c.id === move.combatantId ? { ...c, placed: true, col: move.col, row: move.row } : c,
      ),
    });
    return true;
  }
}
