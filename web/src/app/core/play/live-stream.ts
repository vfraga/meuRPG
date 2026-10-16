import { signal } from '@angular/core';

import { LiveErrorKind, LiveEventVm } from './live-session.types';
import { routeEvent } from './live-stream-dispatch';
import type { LiveStreamHandlers } from './live-stream-handlers';

/**
 * Where the stream stands:
 * - `connecting`: the first connection, before `ready`;
 * - `live`: `ready` arrived and messages keep coming;
 * - `reconnecting`: it dropped and a new try is scheduled or running;
 * - `paused`: the tab is hidden, so the stream is closed on purpose and
 *   reopens when the tab is visible again (ADR-0005's cost guard);
 * - `closed`: over for good (the session ended, no access, signed out, or
 *   the page left).
 */
export type StreamStatus = 'connecting' | 'live' | 'reconnecting' | 'paused' | 'closed';

export type { LiveStreamHandlers } from './live-stream-handlers';

export interface LiveStreamOptions {
  open(signal: AbortSignal): AsyncIterable<LiveEventVm>;
  classify(err: unknown): LiveErrorKind;
  handlers: LiveStreamHandlers;
  document: Document;
  /** For tests; `Math.random` by default. */
  random?: () => number;
}

/** No message for this long (the server sends a heartbeat every 25 s):
 * the stream is dead, even if the connection looks open. */
export const DEAD_STREAM_MS = 60_000;
/** Hidden this long, the tab closes its stream. */
export const HIDDEN_CLOSE_MS = 2 * 60_000;
/** Backoff: 1 s, 2 s, 4 s … up to 30 s. */
export const BACKOFF_MAX_MS = 30_000;

/** The wait before reconnection attempt `attempt` (0 first): doubling from
 * 1 s up to 30 s, each with ±20 % jitter so a table of phones doesn't
 * reconnect in lockstep after a server restart. */
export function backoffDelay(attempt: number, random: () => number = Math.random): number {
  const base = Math.min(BACKOFF_MAX_MS, 1000 * 2 ** attempt);
  return Math.min(BACKOFF_MAX_MS, Math.round(base * (0.8 + 0.4 * random())));
}

/**
 * The live session's stream, with ADR-0005's client rules
 * (docs/architecture.md#live-session):
 *
 * - `ready` first; the page reads the snapshot on each one;
 * - reconnects after any end that is not final, with backoff and jitter,
 *   and only while the tab is visible;
 * - closes itself after 2 minutes hidden, and reopens (with a new
 *   snapshot) when the tab is visible again;
 * - 60 seconds without any message is a dead stream: close and reconnect;
 * - `not_found` and `unauthenticated` end it without reconnecting;
 *   `NO_OPEN_SESSION` and `session_ended` end it as "the session ended".
 *
 * Plain TypeScript with signals, so its timing is tested with fake timers.
 */
export class LiveStream {
  readonly status = signal<StreamStatus>('connecting');
  /** When the last message (any, heartbeats too) arrived. */
  readonly lastMessageAt = signal<Date | null>(null);

  private readonly random: () => number;
  private controller: AbortController | null = null;
  private attempt = 0;
  private restarts = 0;
  private retryTimer: ReturnType<typeof setTimeout> | null = null;
  private deadTimer: ReturnType<typeof setTimeout> | null = null;
  private hiddenTimer: ReturnType<typeof setTimeout> | null = null;
  private readonly onVisibility = () => this.visibilityChanged();

  constructor(private readonly options: LiveStreamOptions) {
    this.random = options.random ?? Math.random;
  }

  start(): void {
    this.options.document.addEventListener('visibilitychange', this.onVisibility);
    if (this.hidden()) {
      // Opened in a background tab: wait until it is looked at.
      this.status.set('paused');
      return;
    }
    void this.connect();
  }

  /** Closes for good (the page is leaving, or the session is over). */
  stop(): void {
    this.status.set('closed');
    this.options.document.removeEventListener('visibilitychange', this.onVisibility);
    this.clearTimers();
    this.abortCurrent();
  }

  /** Drops the current stream and connects again after the backoff: the
   * page asks this when a snapshot failed, so the next `ready` reads a new
   * one. Each restart in a row waits longer (a `ready` alone doesn't reset
   * the wait here, or a failing snapshot would retry every second), until
   * the page says a snapshot worked (`confirmHealthy`). */
  restart(): void {
    if (this.status() === 'closed') {
      return;
    }
    this.restarts++;
    this.attempt = Math.max(this.attempt, this.restarts);
    this.abortCurrent();
    this.scheduleReconnect();
  }

  /** The page read a snapshot: the next restart waits the shortest time. */
  confirmHealthy(): void {
    this.restarts = 0;
  }

  private async connect(): Promise<void> {
    this.clearRetry();
    if (this.status() === 'closed') {
      return;
    }
    if (this.hidden()) {
      this.status.set('paused');
      return;
    }
    const controller = new AbortController();
    this.controller = controller;
    this.armDeadTimer();
    try {
      for await (const event of this.options.open(controller.signal)) {
        if (controller !== this.controller) {
          return; // replaced: a newer connection owns the page now
        }
        this.lastMessageAt.set(new Date());
        this.armDeadTimer();
        if (this.receive(event) === 'over') {
          return;
        }
      }
      if (controller !== this.controller) {
        return;
      }
      // Ended without an error (the 10-minute cap, a server restart).
      this.controller = null;
      this.scheduleReconnect();
    } catch (err) {
      if (controller !== this.controller) {
        return; // we aborted it ourselves (dead stream, hidden tab, stop)
      }
      this.controller = null;
      this.failed(err);
    }
  }

  /** What one message does: `ready` makes the stream live and asks the page for a snapshot, `ended` closes it for good
   * (and the loop with it), anything else goes to the page's handler for its kind. */
  private receive(event: LiveEventVm): 'more' | 'over' {
    if (event.kind === 'ready') {
      this.attempt = 0;
      this.status.set('live');
      this.options.handlers.onReady();
    } else if (event.kind === 'ended') {
      this.stop();
      this.options.handlers.onEnded();
      return 'over';
    } else {
      routeEvent(this.options.handlers, event);
    }
    return 'more';
  }

  /** The connection failed on its own: a fatal error ends the stream, the session being over too, and any other
   * error is tried again after the backoff. */
  private failed(err: unknown): void {
    switch (this.options.classify(err)) {
      case 'no-access':
        this.stop();
        this.options.handlers.onFatal('no-access');
        return;
      case 'signed-out':
        this.stop();
        this.options.handlers.onFatal('signed-out');
        return;
      case 'no-session':
        this.stop();
        this.options.handlers.onEnded();
        return;
      default:
        this.scheduleReconnect();
    }
  }

  private scheduleReconnect(): void {
    this.clearDeadTimer();
    if (this.status() === 'closed') {
      return;
    }
    if (this.hidden()) {
      this.status.set('paused');
      return;
    }
    this.status.set('reconnecting');
    const delay = backoffDelay(this.attempt, this.random);
    this.attempt++;
    this.clearRetry();
    this.retryTimer = setTimeout(() => void this.connect(), delay);
  }

  private visibilityChanged(): void {
    if (this.status() === 'closed') {
      return;
    }
    if (this.hidden()) {
      // Keep the stream for 2 minutes (a quick look at another app), then
      // close it: a forgotten tab holds nothing open.
      this.clearHiddenTimer();
      this.hiddenTimer = setTimeout(() => {
        this.hiddenTimer = null;
        this.abortCurrent();
        this.clearRetry();
        this.clearDeadTimer();
        this.status.set('paused');
      }, HIDDEN_CLOSE_MS);
      return;
    }
    this.clearHiddenTimer();
    if (this.status() === 'paused') {
      // Back in view: connect now, not after a backoff.
      this.attempt = 0;
      this.status.set('reconnecting');
      void this.connect();
    }
  }

  private armDeadTimer(): void {
    this.clearDeadTimer();
    this.deadTimer = setTimeout(() => {
      this.deadTimer = null;
      this.abortCurrent();
      this.scheduleReconnect();
    }, DEAD_STREAM_MS);
  }

  private abortCurrent(): void {
    const controller = this.controller;
    this.controller = null;
    controller?.abort();
  }

  private hidden(): boolean {
    return this.options.document.visibilityState === 'hidden';
  }

  private clearTimers(): void {
    this.clearRetry();
    this.clearDeadTimer();
    this.clearHiddenTimer();
  }

  private clearRetry(): void {
    if (this.retryTimer !== null) {
      clearTimeout(this.retryTimer);
      this.retryTimer = null;
    }
  }

  private clearDeadTimer(): void {
    if (this.deadTimer !== null) {
      clearTimeout(this.deadTimer);
      this.deadTimer = null;
    }
  }

  private clearHiddenTimer(): void {
    if (this.hiddenTimer !== null) {
      clearTimeout(this.hiddenTimer);
      this.hiddenTimer = null;
    }
  }
}
