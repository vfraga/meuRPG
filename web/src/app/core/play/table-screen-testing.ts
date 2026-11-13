import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';

import {
  type GetTableScreenResponse,
  GetTableScreenResponseSchema,
  TableScreenKind,
  type TableScreenNotice,
  type TableScreenUnavailable,
  type WatchTableScreenResponse,
  WatchTableScreenResponseSchema,
} from '../../../gen/meurpg/play/v1/table_screen_pb';
import {
  GameSessionBlockedReason,
  GameSessionBlockedSchema,
} from '../../../gen/meurpg/play/v1/play_pb';
import type { TvClock } from './table-screen-state';

/** A `GetTableScreen` answer for specs: the idle screen of Mirathel's session 3, with the fields overridable. */
export function screenResponse(
  overrides: {
    stage?: object;
    choice?: { mirror: boolean; pin: TableScreenKind };
    choiceRevision?: number;
    notice?: TableScreenNotice;
    options?: { kind: TableScreenKind; available: boolean; unavailable?: TableScreenUnavailable }[];
    presence?: { open: boolean; lostAt?: Date };
  } = {},
): GetTableScreenResponse {
  return create(GetTableScreenResponseSchema, {
    choiceRevision: overrides.choiceRevision ?? 1,
    choice: overrides.choice ?? { mirror: true, pin: TableScreenKind.UNSPECIFIED },
    stage: overrides.stage ?? { kind: TableScreenKind.IDLE, body: { case: 'idle', value: {} } },
    notice: overrides.notice,
    campaignName: 'Mirathel',
    sessionNumber: 3,
    party: ['Tor', 'Brisa'],
    options: overrides.options ?? [],
    presence: overrides.presence && {
      open: overrides.presence.open,
      lostAt: overrides.presence.lostAt && timestampFromDate(overrides.presence.lostAt),
    },
  } as never);
}

/** One event of `WatchTableScreen`. */
export function tvWire(event: 'ready' | 'heartbeat' | 'changed' | 'sessionEnded', sessionSeq = 0n) {
  const value = event === 'ready' || event === 'heartbeat' ? { sessionSeq } : {};
  return create(WatchTableScreenResponseSchema, { event: { case: event, value } } as never);
}

/** The TV's clock, moved by hand: timers fire when `advance` passes them. */
export class FakeClock implements TvClock {
  private at: number;
  private timers: { due: number; fn: () => void }[] = [];

  constructor(start: Date) {
    this.at = start.getTime();
  }

  now(): Date {
    return new Date(this.at);
  }

  after(ms: number, fn: () => void): () => void {
    const timer = { due: this.at + ms, fn };
    this.timers.push(timer);
    return () => (this.timers = this.timers.filter((t) => t !== timer));
  }

  /** How many timers wait. */
  get pending(): number {
    return this.timers.length;
  }

  advance(ms: number): void {
    const end = this.at + ms;
    for (;;) {
      const next = this.timers.filter((t) => t.due <= end).sort((a, b) => a.due - b.due)[0];
      if (!next) {
        break;
      }
      this.timers = this.timers.filter((t) => t !== next);
      this.at = next.due;
      next.fn();
    }
    this.at = end;
  }
}

/** One `WatchTableScreen` call the spec drives by hand. */
export class FakeWatchCall {
  private readonly queue: (IteratorResult<WatchTableScreenResponse> | { error: unknown })[] = [];
  private wake: (() => void) | null = null;
  aborted = false;

  constructor(readonly signal: AbortSignal) {
    signal.addEventListener('abort', () => {
      this.aborted = true;
      this.fail(new Error('aborted'));
    });
  }

  push(event: Parameters<typeof tvWire>[0], sessionSeq = 0n): void {
    this.queue.push({ value: tvWire(event, sessionSeq), done: false });
    this.wake?.();
  }

  fail(error: unknown): void {
    this.queue.push({ error });
    this.wake?.();
  }

  async *events(): AsyncIterable<WatchTableScreenResponse> {
    for (;;) {
      while (this.queue.length === 0) {
        await new Promise<void>((resolve) => (this.wake = resolve));
      }
      const next = this.queue.shift()!;
      if ('error' in next) {
        throw next.error;
      }
      if (next.done) {
        return;
      }
      yield next.value;
    }
  }
}

/** `failed_precondition` with `NO_OPEN_SESSION`. */
export function noOpenSession(): ConnectError {
  return new ConnectError('blocked', Code.FailedPrecondition, undefined, [
    { desc: GameSessionBlockedSchema, value: { reason: GameSessionBlockedReason.NO_OPEN_SESSION } },
  ]);
}
