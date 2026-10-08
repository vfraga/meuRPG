import { signal } from '@angular/core';
import type { MessageInitShape } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import type { DiceRoll } from '../../../gen/meurpg/play/v1/combat_pb';
import {
  PuzzleBlockedReason,
  type PuzzleMoveSchema,
  type PuzzleRun,
  type TryPuzzleHintResponse,
} from '../../../gen/meurpg/play/v1/puzzles_pb';
import { newKey } from '../connect/idempotency';
import { isTransient, puzzleBlocked, puzzleErrorMessage } from './puzzle-errors';
import type { HintDie, MoveAnswer } from './puzzles-client';

/** What the player's page needs from the client; `PuzzlesClient` is one. */
export interface PlayApi {
  run(campaignId: string, puzzleId: string): Promise<PuzzleRun>;
  move(
    campaignId: string,
    puzzleId: string,
    move: MessageInitShape<typeof PuzzleMoveSchema>,
    idempotencyKey: string,
  ): Promise<MoveAnswer>;
  tryHint(
    campaignId: string,
    puzzleId: string,
    die: HintDie,
    idempotencyKey: string,
  ): Promise<TryPuzzleHintResponse>;
}

/** What a move came to, for the page that sent it: a typed answer or a deciphered message is judged, not applied. */
export interface MoveVerdict {
  /** The move reached the server and was judged. */
  readonly sent: boolean;
  /** The judged move was wrong (the player's own: a wrong answer, a wrong bell). */
  readonly wrong: boolean;
  readonly solved: boolean;
}

/** The player's own try for a hint by a skill check, as the page says it ("Você conseguiu." / "Não deu desta vez."). */
export interface HintTry {
  readonly passed: boolean;
  /** The player's own roll: the d20 and the total with the bonus. Never the DC. */
  readonly roll: DiceRoll | undefined;
}

/** Calls `fn` after `ms` and returns what cancels it: the sequence's reveal reads the run again at each step. */
export type Schedule = (ms: number, fn: () => void) => () => void;

const timeoutSchedule: Schedule = (ms, fn) => {
  const id = setTimeout(fn, ms);
  return () => clearTimeout(id);
};

/** A little after the server's `next_in_ms`, so the read finds the step revealed, never the one before it. */
export const REVEAL_SLACK_MS = 60;

/** Waits before the 1st, 2nd and 3rd retry of a move whose answer never came. */
export const RETRY_WAITS_MS: readonly number[] = [400, 1200, 3000];

/**
 * The player's open puzzle (MR-038, RN-27): the run as the server last said it, and the moves.
 *
 * - **The board follows the server.** A move changes nothing on screen until the server answers; the answer (or a read
 *   after `puzzle_changed`) is applied only when its `revision` is larger than the one on screen, so a late answer never
 *   puts the board back.
 * - **Each move has its own idempotency key**, made when the player acts, and a failed try that never got an answer is
 *   sent again with the *same* key (up to three times): the server plays a key once, so a retry after a lost answer
 *   never touches the board twice. Moves are relative and commute, so two taps in a row are sent at once.
 * - **A refusal reads again.** A `failed_precondition` (solved, stopped, no attempts) or a `not_found` (the master closed
 *   it) says the screen was stale: the message is shown and the run is read again.
 *
 * Plain TypeScript with signals, so the timing is tested with a fake `wait`.
 */
export class PuzzlePlay {
  /** The run on screen, or `null` before the first read and after the master closed it. */
  readonly run = signal<PuzzleRun | null>(null);
  /** The master closed the puzzle, or it never was one the player may read. */
  readonly gone = signal(false);
  /** Something to say about the last move or read, in words, or `''`. */
  readonly message = signal('');
  /** Moves sent and not answered yet. */
  readonly pending = signal(0);
  /** A try for a hint is in flight. */
  readonly hintBusy = signal(false);
  /** The last try for a hint, until the next one or the next change of the puzzle. */
  readonly hintTry = signal<HintTry | null>(null);
  /** What to say when the first read of the run failed and nothing is on screen, or `''`; "Tentar de novo" reads again. */
  readonly loadError = signal('');
  /** The server refused a try because the table rolls its dice the other way: the page reads the campaign's dice mode again. */
  readonly diceModeStale = signal(0);

  constructor(
    private readonly api: PlayApi,
    private readonly campaignId: () => string,
    private readonly wait: (ms: number) => Promise<void> = (ms) =>
      new Promise((resolve) => setTimeout(resolve, ms)),
    private readonly makeKey: () => string = newKey,
    private readonly schedule: Schedule = timeoutSchedule,
  ) {}

  private cancelReveal: (() => void) | null = null;
  /** The page left: a read that is still on its way never starts a timer after it. */
  private disposed = false;
  /** Bell taps wait for the one before: the server may commit two moves in flight in either order, and a retry swaps them for sure. */
  private bellQueue: Promise<unknown> = Promise.resolve();
  /** Counts the wrong bells: taps made before the player knew a bell was wrong are dropped, not judged again. */
  private bellEpoch = 0;
  /** Counts the puzzles opened: work that began for an earlier one (a tap, a hint, a read) never touches the one on screen. */
  private openSeq = 0;

  /** Stops the timer that reads the sequence again (the page is leaving). */
  dispose(): void {
    this.disposed = true;
    this.stopReveal();
  }

  private stopReveal(): void {
    this.cancelReveal?.();
    this.cancelReveal = null;
  }

  /** Opens a puzzle: the first read. */
  async open(puzzleId: string): Promise<void> {
    this.disposed = false;
    this.stopReveal();
    this.openSeq++;
    this.bellQueue = Promise.resolve();
    this.bellEpoch++;
    this.pending.set(0);
    this.hintBusy.set(false);
    this.run.set(null);
    this.gone.set(false);
    this.loadError.set('');
    this.message.set('');
    this.hintTry.set(null);
    this.pendingId = puzzleId;
    await this.refresh();
  }

  private pendingId = '';

  /** Reads the run again (after `puzzle_changed`, or when the page was away). */
  async refresh(): Promise<void> {
    const id = this.pendingId;
    if (id === '') {
      return;
    }
    try {
      const next = await this.api.run(this.campaignId(), id);
      if (id !== this.pendingId) {
        return;
      }
      this.apply(next);
      this.gone.set(false);
      this.loadError.set('');
    } catch (err) {
      if (id !== this.pendingId) {
        return;
      }
      if (ConnectError.from(err, Code.Unavailable).code === Code.NotFound) {
        this.gone.set(true);
        return;
      }
      // Any other failure keeps what is on screen: the next hint or "ready" reads again. With nothing on screen, the page says so.
      if (this.run() === null) {
        this.loadError.set(puzzleErrorMessage(err, 'abrir o quebra-cabeça', 'player'));
      }
    }
  }

  /** Applies a run if it is newer than the one on screen (the same puzzle) or the first of another. Returns whether it did. */
  apply(next: PuzzleRun): boolean {
    // A late answer for the puzzle that was open before is not this one's.
    if (this.pendingId !== '' && next.puzzleId !== this.pendingId) {
      return false;
    }
    const current = this.run();
    if (
      current &&
      current.puzzleId === next.puzzleId &&
      next.revision <= current.revision &&
      !movedOn(current, next)
    ) {
      return false;
    }
    // What a player tried for a hint is about the hint they were at: once it moves on (the master released one, the player may try again),
    // or the puzzle is over, the line goes.
    if (
      current &&
      (next.hints.length !== current.hints.length ||
        (next.canTryHint && !current.canTryHint) ||
        next.solved ||
        next.stopped)
    ) {
      this.hintTry.set(null);
    }
    this.run.set(next);
    this.watchReveal(next);
    return true;
  }

  /** While a play runs, read again when the server says the next step is shown (the stream's hint says it too; this is the safety net). */
  private watchReveal(run: PuzzleRun): void {
    this.stopReveal();
    const playback = run.sequence;
    if (!this.disposed && playback?.playing && playback.nextInMs > 0) {
      this.cancelReveal = this.schedule(
        playback.nextInMs + REVEAL_SLACK_MS,
        () => void this.refresh(),
      );
    }
  }

  /**
   * Makes one move. Never rejects: a failure becomes `message`. The verdict says whether a typed answer or a bell was judged wrong:
   * the server's answer carries the verdict of this very move (also on a replay), never read from the run's last move, which may be
   * another player's.
   *
   * Bell taps go one at a time, in the order they were made, each after the answer to the one before; `pending` counts the taps still
   * waiting too, so the screen can say so. A wrong bell drops the taps queued behind it (they were made for a sequence that started over).
   */
  move(move: MessageInitShape<typeof PuzzleMoveSchema>): Promise<MoveVerdict> {
    if (move.kind?.case !== 'sequence') {
      return this.send(move);
    }
    const epoch = this.bellEpoch;
    const seq = this.openSeq;
    this.pending.update((n) => n + 1);
    const turn = this.bellQueue.then(async () => {
      if (seq !== this.openSeq) {
        // Made for the puzzle that was open before: opening this one already cleared the count.
        return { sent: false, wrong: false, solved: false } satisfies MoveVerdict;
      }
      if (epoch !== this.bellEpoch) {
        this.pending.update((n) => n - 1);
        return { sent: false, wrong: false, solved: false } satisfies MoveVerdict;
      }
      const verdict = await this.send(move, true);
      if (verdict.wrong && seq === this.openSeq) {
        this.bellEpoch++;
      }
      return verdict;
    });
    this.bellQueue = turn;
    return turn;
  }

  private async send(
    move: MessageInitShape<typeof PuzzleMoveSchema>,
    counted = false,
  ): Promise<MoveVerdict> {
    const run = this.run();
    if (!run || run.solved || run.stopped) {
      if (counted) {
        this.pending.update((n) => n - 1);
      }
      return { sent: false, wrong: false, solved: false };
    }
    const kind = move.kind?.case;
    // A typed answer spends an attempt: a second one made before the first is judged (Enter held, a double tap) is not sent.
    if ((kind === 'riddle' || kind === 'cipher') && this.pending() > 0) {
      return { sent: false, wrong: false, solved: false };
    }
    const seq = this.openSeq;
    const key = this.makeKey();
    if (!counted) {
      this.pending.update((n) => n + 1);
    }
    this.message.set('');
    try {
      const answer = await this.sendWithRetry(run.puzzleId, move, key);
      if (seq !== this.openSeq) {
        return { sent: false, wrong: false, solved: false };
      }
      this.apply(answer.run);
      return {
        sent: true,
        wrong: answer.wrong && !answer.run.solved,
        solved: answer.solvedByThisMove || answer.run.solved,
      };
    } catch (err) {
      if (seq !== this.openSeq) {
        return { sent: false, wrong: false, solved: false };
      }
      this.message.set(puzzleErrorMessage(err, 'fazer essa jogada', 'player'));
      const blocked = puzzleBlocked(err);
      const notFound = ConnectError.from(err, Code.Unavailable).code === Code.NotFound;
      if (blocked || notFound) {
        await this.refresh();
      }
      return { sent: false, wrong: false, solved: false };
    } finally {
      if (seq === this.openSeq) {
        this.pending.update((n) => n - 1);
      }
    }
  }

  /**
   * "Tentar uma dica": rolls the puzzle's skill check for the next hint, in the app or from the face of a real die. A pass puts the
   * hint in `run.hints` (this player's alone); the answer is the run as this player reads it, and their own roll. The DC never
   * reaches here. The same key goes with a retry, so a lost answer never rolls twice.
   */
  async tryHint(die: HintDie): Promise<void> {
    const run = this.run();
    if (!run || !run.canTryHint || this.hintBusy() || this.gone()) {
      return;
    }
    const seq = this.openSeq;
    const key = this.makeKey();
    this.hintBusy.set(true);
    this.message.set('');
    try {
      const answer = await this.sendHintWithRetry(run.puzzleId, die, key);
      if (seq !== this.openSeq) {
        return;
      }
      this.apply(answer.run ?? run);
      this.hintTry.set({ passed: answer.passed, roll: answer.roll });
    } catch (err) {
      if (seq !== this.openSeq) {
        return;
      }
      this.message.set(puzzleErrorMessage(err, 'tentar a dica', 'player'));
      if (puzzleBlocked(err)?.reason === PuzzleBlockedReason.WRONG_DICE_MODE) {
        // The table's dice mode changed under the page: read it again, so the way to roll on screen is the one the server wants.
        this.diceModeStale.update((n) => n + 1);
      }
      if (puzzleBlocked(err) || ConnectError.from(err, Code.Unavailable).code === Code.NotFound) {
        await this.refresh();
      }
    } finally {
      if (seq === this.openSeq) {
        this.hintBusy.set(false);
      }
    }
  }

  private async sendHintWithRetry(
    puzzleId: string,
    die: HintDie,
    key: string,
  ): Promise<TryPuzzleHintResponse> {
    for (let attempt = 0; ; attempt++) {
      try {
        return await this.api.tryHint(this.campaignId(), puzzleId, die, key);
      } catch (err) {
        const waitMs = RETRY_WAITS_MS[attempt];
        if (waitMs === undefined || !isTransient(err)) {
          throw err;
        }
        await this.wait(waitMs);
      }
    }
  }

  private async sendWithRetry(
    puzzleId: string,
    move: MessageInitShape<typeof PuzzleMoveSchema>,
    key: string,
  ): Promise<MoveAnswer> {
    for (let attempt = 0; ; attempt++) {
      try {
        return await this.api.move(this.campaignId(), puzzleId, move, key);
      } catch (err) {
        const waitMs = RETRY_WAITS_MS[attempt];
        if (waitMs === undefined || !isTransient(err)) {
          throw err;
        }
        await this.wait(waitMs);
      }
    }
  }
}

/**
 * Whether a read of the same revision is further along than the one on screen. A play of the sequence reveals a step, a time limit
 * stops the puzzle, a clue reaches the notes: none changes the revision, so the page takes the newer of two reads of one revision by what only moves forward.
 */
function movedOn(current: PuzzleRun, next: PuzzleRun): boolean {
  if (next.revision < current.revision) {
    return false;
  }
  if (next.stopped && !current.stopped) {
    return true;
  }
  // The player found the key of the cipher in the adventure: a clue that reached their notes, which moves no revision either.
  if (next.keyClueId !== '' && current.keyClueId === '') {
    return true;
  }
  const a = current.sequence;
  const b = next.sequence;
  if (!a || !b) {
    return false;
  }
  if (b.plays !== a.plays) {
    return b.plays > a.plays;
  }
  // The same play: it only goes forward, and once it ended (not playing, nothing shown) a late read from the middle of it never brings it back.
  if (!a.playing) {
    return false;
  }
  return !b.playing || b.shown.length > a.shown.length;
}
