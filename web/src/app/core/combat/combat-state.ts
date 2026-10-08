import { computed, signal } from '@angular/core';

import { type Encounter, EncounterStatus } from '../../../gen/meurpg/play/v1/combat_pb';
import { isTheatre } from './theatre';

/** What `turn_changed` carries (play.proto). */
export interface TurnChange {
  readonly encounterId: string;
  readonly round: number;
  readonly currentCombatantId: string;
  readonly masterTurn: boolean;
}

/** What `combatant_moved` carries (play.proto). */
export interface CombatantMove {
  readonly encounterId: string;
  readonly combatantId: string;
  readonly col: number;
  readonly row: number;
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
 * dropped; `turn_changed` and `combatant_moved` apply in place; a combatant the screen doesn't know
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

  /** `combat_log_changed`, or a `ready`: read the log again. */
  touchLog(): void {
    this.logTick.update((n) => n + 1);
  }

  clear(): void {
    this.applied++;
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

  /** `turn_changed`. `false` when it is about a combat the screen doesn't
   * have: read it again. */
  applyTurn(turn: TurnChange): boolean {
    const e = this.encounter();
    if (!e || e.id !== turn.encounterId) {
      return false;
    }
    // A part that ended inside a joint turn moves "current" to another member of the same group, in the same
    // round: the group and who already ended their part stay. Only a turn that moves on starts over.
    this.patched++;
    const sameGroup = turn.round === e.round && e.turnGroupIds.includes(turn.currentCombatantId);
    this.encounter.set({
      ...e,
      round: turn.round,
      currentCombatantId: turn.currentCombatantId,
      masterTurn: turn.masterTurn,
      ...(sameGroup
        ? {}
        : {
            // The group belongs to the turn that ended; until the combat is read again the turn is the current combatant's.
            turnGroupIds: [],
            combatants: e.combatants.map((c) =>
              c.turnPartEnded ? { ...c, turnPartEnded: false } : c,
            ),
          }),
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
