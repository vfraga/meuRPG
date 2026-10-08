import { signal } from '@angular/core';
import { Code, ConnectError } from '@connectrpc/connect';

import type { MasterPuzzleRun, PuzzleSummary } from '../../../gen/meurpg/play/v1/puzzles_pb';
import type { PuzzlesClient } from './puzzles-client';

/** The calls the session's puzzle lists need; `PuzzlesClient` is one. */
export type SessionApi = Pick<PuzzlesClient, 'listSession' | 'masterRun' | 'listShown'>;

/**
 * The puzzles of the open session, as this person reads them (MR-038, E10-06): the master's menu (every puzzle that is
 * not archived, with where it stands: `runs`) or the players' list of what is shown (`shown`). The session page owns one
 * and feeds it from the stream: `puzzle_changed` carries only the puzzle's ID, so a change reads that puzzle again, and a
 * `ready` (a reconnection too) reads the list.
 *
 * A read that started before a newer write never wins: each puzzle's reads and writes take a number, and only the
 * latest-started one is applied.
 */
export class PuzzleSessionState {
  /** The master's menu, newest first as the server sends it. */
  readonly runs = signal<readonly MasterPuzzleRun[]>([]);
  /** What the players read: the shown puzzles, in the order they were shown. */
  readonly shown = signal<readonly PuzzleSummary[]>([]);
  /** `'idle'` until the first read, `'error'` when the list could not be read (the old list stays). */
  readonly status = signal<'idle' | 'ready' | 'error'>('idle');

  /**
   * One counter for each puzzle the stream said changed, and one for "any" (a full read, a reconnection): the player's open
   * puzzle reads its own run again when its `versionOf` goes up, since it is not part of the lists. Counters, not a "last changed"
   * mark, so two hints in one turn are both seen.
   */
  private readonly changes = signal<ReadonlyMap<string, number>>(new Map());
  private readonly anyChange = signal(0);
  /** The master's open live card (`null`: none), one at a time: the row's "Ver ao vivo". */
  readonly selectedId = signal<string | null>(null);

  private listSeq = 0;
  private idSeq = 0;
  private readonly applied = new Map<string, number>();

  constructor(
    private readonly api: SessionApi,
    private readonly campaignId: () => string,
    private readonly isMaster: () => boolean,
  ) {}

  clear(): void {
    this.listSeq++;
    this.idSeq++;
    this.applied.clear();
    this.runs.set([]);
    this.shown.set([]);
    this.selectedId.set(null);
    this.status.set('idle');
  }

  /** The whole list, after `ready` and after a change of a puzzle this screen does not know. */
  async refresh(): Promise<void> {
    this.bump('');
    await this.read();
  }

  private async read(): Promise<void> {
    const ticket = ++this.listSeq;
    const written = new Map(this.applied);
    try {
      if (this.isMaster()) {
        const listed = await this.api.listSession(this.campaignId());
        if (ticket !== this.listSeq) {
          return;
        }
        // A puzzle written (an action's answer, a `puzzle_changed` read) after this list began is newer than the list's copy.
        const runs = listed.map((r) => {
          const id = r.puzzle?.id ?? '';
          const current =
            this.applied.get(id) !== written.get(id)
              ? this.runs().find((c) => c.puzzle?.id === id)
              : undefined;
          return current ?? r;
        });
        this.runs.set(runs);
      } else {
        const shown = await this.api.listShown(this.campaignId());
        if (ticket !== this.listSeq) {
          return;
        }
        this.shown.set(shown);
      }
      this.status.set('ready');
    } catch (err) {
      if (ticket === this.listSeq && !isNoSession(err)) {
        this.status.set('error');
      }
    }
  }

  /** `puzzle_changed`: the master reads that puzzle; a player reads the list of what is shown (it is short). */
  async changed(puzzleId: string): Promise<void> {
    this.bump(puzzleId);
    if (!this.isMaster() || !this.runs().some((r) => r.puzzle?.id === puzzleId)) {
      await this.read();
      return;
    }
    const ticket = ++this.idSeq;
    this.applied.set(puzzleId, ticket);
    try {
      const run = await this.api.masterRun(this.campaignId(), puzzleId);
      // A read that started before a later action never wins; and a read older than what is on screen (by the run's own
      // revision) never puts it back.
      const had = this.runs().find((r) => r.puzzle?.id === puzzleId);
      const older = had?.run && run.run && run.run.revision < had.run.revision;
      if (this.applied.get(puzzleId) === ticket && !older) {
        this.put(run);
      }
    } catch {
      // The next hint, or `ready`, reads again.
    }
  }

  /** How many times the puzzle (or everything) changed since the page opened; 0 before the first. */
  versionOf(puzzleId: string): number {
    return this.anyChange() + (this.changes().get(puzzleId) ?? 0);
  }

  select(puzzleId: string | null): void {
    this.selectedId.set(puzzleId);
  }

  private bump(id: string): void {
    if (id === '') {
      this.anyChange.update((n) => n + 1);
      return;
    }
    this.changes.update((m) => new Map(m).set(id, (m.get(id) ?? 0) + 1));
  }

  /** A master action answered with the puzzle as it stands now. */
  replace(run: MasterPuzzleRun): void {
    const id = run.puzzle?.id ?? '';
    this.applied.set(id, ++this.idSeq);
    this.put(run);
  }

  private put(run: MasterPuzzleRun): void {
    const id = run.puzzle?.id ?? '';
    this.runs.update((list) => {
      const at = list.findIndex((r) => r.puzzle?.id === id);
      return at < 0 ? [run, ...list] : list.map((r, i) => (i === at ? run : r));
    });
  }
}

function isNoSession(err: unknown): boolean {
  // No open session: the page is leaving for "A sessão acabou"; the list has nothing to say.
  return ConnectError.from(err, Code.Unavailable).code === Code.FailedPrecondition;
}
