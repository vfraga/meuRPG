import {
  DOCUMENT,
  DestroyRef,
  Injectable,
  InjectionToken,
  effect,
  inject,
  signal,
  untracked,
} from '@angular/core';

import type { WatchTableScreenResponse } from '../../../gen/meurpg/play/v1/table_screen_pb';
import { LiveStream } from './live-stream';
import type { LiveEventVm } from './live-session.types';
import { TableScreenClient } from './table-screen-client';
import { classifyTvStreamError } from './table-screen-errors';
import { TvScreenState, type TvClock, systemClock } from './table-screen-state';
import { toTvScreen } from './table-screen-view';

/** The wait that gathers a burst of `changed` into one read. */
export const TV_DEBOUNCE_MS = 300;

/** The clock the TV's timers use; specs provide their own. */
export const TV_CLOCK = new InjectionToken<TvClock>('TvClock', {
  providedIn: 'root',
  factory: () => systemClock,
});

/** `WatchTableScreen` as the events `LiveStream` knows: `changed` is "read again", like a content hint. */
export async function* tvEvents(
  stream: AsyncIterable<WatchTableScreenResponse>,
): AsyncIterable<LiveEventVm> {
  for await (const res of stream) {
    switch (res.event.case) {
      case 'ready':
        yield { kind: 'ready', sessionSeq: Number(res.event.value.sessionSeq) };
        break;
      case 'heartbeat':
        yield { kind: 'heartbeat', sessionSeq: Number(res.event.value.sessionSeq) };
        break;
      case 'changed':
        yield { kind: 'contentChanged' };
        break;
      case 'sessionEnded':
        yield { kind: 'ended' };
        break;
      default:
        // A heartbeat, or an event of a newer server: proof the stream is alive.
        yield { kind: 'heartbeat' };
    }
  }
}

/**
 * Drives the TV tab: opens `WatchTableScreen` through `LiveStream` (its backoff, its dead-stream check, its pause
 * while the tab is hidden) and reads `GetTableScreen` after every `ready` and, gathered, after every `changed`.
 * Reads never overlap: a hint that comes during one makes exactly one more read after it. The last good read
 * stays while the stream reconnects; `state` says what to draw. One watcher per TV page
 * (`providers: [TableScreenWatcher]`).
 */
@Injectable()
export class TableScreenWatcher {
  private readonly client = inject(TableScreenClient);
  private readonly document = inject(DOCUMENT);
  private readonly clock = inject(TV_CLOCK);

  readonly state = new TvScreenState(this.clock, () => void this.read());

  private readonly stream = signal<LiveStream | null>(null);
  private campaignId = '';
  private again = false;
  private cancelDebounce: (() => void) | null = null;

  constructor() {
    // The stream's status is the connection: `reconnecting` starts the minute; `paused` (tab hidden) is not a loss.
    effect(() => {
      const status = this.stream()?.status();
      if (status === 'live' || status === 'paused') {
        untracked(() => this.state.connection(true));
      } else if (status === 'reconnecting') {
        untracked(() => this.state.connection(false));
      }
    });
    inject(DestroyRef).onDestroy(() => this.stop());
  }

  /** Opens the stream of the campaign's screen; its first `ready` reads the screen. */
  start(campaignId: string): void {
    this.stop();
    this.campaignId = campaignId;
    const stream = new LiveStream({
      open: (signal) => tvEvents(this.client.watch(campaignId, signal)),
      classify: classifyTvStreamError,
      document: this.document,
      handlers: {
        onReady: () => {
          this.dropDebounce();
          void this.read();
        },
        // A heartbeat ahead of the stream's number: a `changed` was lost, so the screen is read again, once.
        onBehind: () => this.read(),
        onVitals: () => undefined,
        onContentChanged: () => this.hint(),
        onEnded: () => this.state.ended(),
        onFatal: (kind) => (kind === 'signed-out' ? this.state.signedOut() : this.state.refuse()),
      },
    });
    this.stream.set(stream);
    stream.start();
  }

  stop(): void {
    this.stream()?.stop();
    this.stream.set(null);
    this.dropDebounce();
    this.state.dispose();
    this.campaignId = '';
  }

  private hint(): void {
    if (this.cancelDebounce) {
      return;
    }
    this.cancelDebounce = this.clock.after(TV_DEBOUNCE_MS, () => {
      this.cancelDebounce = null;
      void this.read();
    });
  }

  private inflight: Promise<boolean> | null = null;

  /** Reads the screen; resolves `true` when the read worked. A call that comes during a read joins it (one more read follows
   * it) and gets the outcome of the last read that ran. */
  private read(): Promise<boolean> {
    if (this.inflight) {
      this.again = true;
      return this.inflight;
    }
    const run = this.readUntilQuiet().finally(() => (this.inflight = null));
    this.inflight = run;
    return run;
  }

  private async readUntilQuiet(): Promise<boolean> {
    let ok = await this.readOnce();
    while (this.again) {
      this.again = false;
      ok = await this.readOnce();
    }
    return ok;
  }

  private async readOnce(): Promise<boolean> {
    const campaignId = this.campaignId;
    if (campaignId === '') {
      return false;
    }
    try {
      const res = await this.client.get(campaignId);
      if (campaignId === this.campaignId) {
        this.state.loaded(toTvScreen(res));
      }
      return true;
    } catch (err) {
      if (campaignId === this.campaignId) {
        this.failed(err);
      }
      return false;
    }
  }

  private failed(err: unknown): void {
    this.state.readFailed(err);
    const phase = this.state.phase().kind;
    // A refusal or the end closes the stream too: there is nothing more to watch.
    if (phase === 'no-access' || phase === 'ended' || phase === 'signed-out') {
      this.stream()?.stop();
    }
  }

  private dropDebounce(): void {
    this.cancelDebounce?.();
    this.cancelDebounce = null;
  }
}
