import { computed, signal } from '@angular/core';
import { clone } from '@bufbuild/protobuf';

import {
  CombatLogEntrySchema,
  type CombatLogEntry,
  type CombatLogRound,
  type ListCombatLogResponse,
} from '../../../gen/meurpg/play/v1/combat_pb';
import type { CombatClient } from './combat-client';
import { undoableEntry } from './combat-log';

/** `since_seq` that asks for the whole log. */
const WHOLE_LOG = 0n;

/**
 * The combat log on this screen (`ListCombatLog`): read again after every
 * `combat_log_changed` and every reconnection (the page's `logTick`). It
 * asks only for what is new (`since_seq` = the `last_seq` it holds) and puts
 * the entries that come in place by id; an answer that is `full` replaces
 * everything. It keeps the rounds as the server sent them (the undo needs
 * the structured entry); the panel draws the groups from them.
 */
export class CombatLogState {
  readonly rounds = signal<readonly CombatLogRound[]>([]);
  /** The event `UndoLastAction` would take back; only the master gets it. */
  readonly undoableId = signal('');
  readonly loaded = signal(false);
  readonly undoable = computed(() => undoableEntry(this.rounds()));
  private asked = 0;
  /** The `last_seq` of what the page holds, and the combat it belongs to. */
  private lastSeq = WHOLE_LOG;
  private heldFor = '';
  /** A whole read was asked for and none has been applied yet (a later read may have replaced the one that carried it). */
  private needFull = false;

  async load(
    api: CombatClient,
    campaignId: string,
    encounterId: string,
    full = false,
  ): Promise<void> {
    const mine = ++this.asked;
    this.needFull ||= full;
    if (encounterId !== this.heldFor) {
      this.needFull = true;
    }
    const wholeLog = this.needFull || !this.loaded();
    try {
      const res = await api.turn.log(campaignId, encounterId, wholeLog ? WHOLE_LOG : this.lastSeq);
      if (mine === this.asked) {
        this.apply(res, wholeLog, encounterId);
      }
    } catch {
      // Best effort: the next `combat_log_changed` reads it again.
    }
  }

  private apply(res: ListCombatLogResponse, asked: boolean, encounterId: string): void {
    const whole = res.full || asked;
    this.rounds.set(whole ? res.rounds : mergeRounds(this.rounds(), res.rounds));
    // The flag of a held entry may be old: it is right only on the entry this answer sends.
    this.rounds.update((rounds) => withUndoable(rounds, res));
    this.undoableId.set(res.undoableEventId);
    this.lastSeq = res.lastSeq;
    this.heldFor = encounterId;
    this.needFull = false;
    this.loaded.set(true);
  }

  clear(): void {
    this.asked++;
    this.rounds.set([]);
    this.undoableId.set('');
    this.loaded.set(false);
    this.lastSeq = WHOLE_LOG;
    this.heldFor = '';
    this.needFull = false;
  }
}

/** The held rounds with the entries of a delta put in place. The server lists an entry where its first event stands (creation
 * order, latest first), so an entry whose id is held is replaced at the same place, in the same round, whatever its `seq` is now.
 * A new id is newer than every held entry (it was created after the answer the app holds), so it goes at the front of its round,
 * in the order the answer lists it, and a round the app does not hold yet is added in order (latest first). */
function mergeRounds(
  held: readonly CombatLogRound[],
  delta: readonly CombatLogRound[],
): CombatLogRound[] {
  const rounds = held.map((r) => ({ ...r, entries: [...r.entries] }));
  const holder = (id: string) => rounds.find((r) => r.entries.some((e) => e.id === id));
  for (const d of delta) {
    const fresh: CombatLogEntry[] = [];
    for (const entry of d.entries) {
      const home = holder(entry.id);
      if (home) {
        home.entries[home.entries.findIndex((e) => e.id === entry.id)] = entry;
      } else {
        fresh.push(entry);
      }
    }
    if (fresh.length === 0) {
      continue;
    }
    let round = rounds.find((r) => r.round === d.round);
    if (!round) {
      round = { ...d, entries: [] };
      const after = rounds.findIndex((r) => r.round < d.round);
      rounds.splice(after < 0 ? rounds.length : after, 0, round);
    }
    round.entries.unshift(...fresh);
  }
  return rounds as CombatLogRound[];
}

/** The rounds with `undoable` set on the entries this answer sent as undoable, and only if the answer names an undoable event. */
function withUndoable(
  rounds: readonly CombatLogRound[],
  res: ListCombatLogResponse,
): CombatLogRound[] {
  const sent = new Set<string>();
  if (res.undoableEventId !== '') {
    for (const r of res.rounds) {
      for (const e of r.entries) {
        if (e.undoable) {
          sent.add(e.id);
        }
      }
    }
  }
  return rounds.map((r) => ({
    ...r,
    entries: r.entries.map((e) => withFlag(e, sent.has(e.id))),
  })) as CombatLogRound[];
}

function withFlag(e: CombatLogEntry, undoable: boolean): CombatLogEntry {
  if (e.undoable === undoable) {
    return e;
  }
  const copy = clone(CombatLogEntrySchema, e);
  copy.undoable = undoable;
  return copy;
}
