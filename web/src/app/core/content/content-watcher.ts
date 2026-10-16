import { DOCUMENT, DestroyRef, Injectable, effect, inject, untracked } from '@angular/core';

import { LiveSessionSourceLive } from '../play/live-session-source.live';
import { LiveStream } from '../play/live-stream';
import { OpenSessions } from '../play/open-sessions';

/** The wait that gathers a burst of hints into one read: the server already lets one through every 250 ms. */
export const CONTENT_DEBOUNCE_MS = 300;

/**
 * Listens to a campaign's live session for `content_changed` (RN-23, RN-10) and says so, so a page that shows the table's
 * content (the content list and its options, "Magias", the character editor, the level-up, an open sheet) reads it again with
 * its own role. The hint carries nothing: the page asks the server, which answers for who is asking. It is the session page's
 * own stream client (`LiveStream`, ADR-0005's rules: backoff, closed when the tab is hidden, no reconnect without access), like
 * the sheet's `XpWatcher`, one stream per page (`providers: [ContentWatcher, LiveSessionSourceLive]`). Hints that come close
 * together are one read (`CONTENT_DEBOUNCE_MS`), and a later `ready` (a reconnection, the tab back) reads once, because a hint
 * may have been missed. Without an open session nobody sends a hint, so there is no stream: the page reads on load.
 */
@Injectable()
export class ContentWatcher {
  private readonly source = inject(LiveSessionSourceLive);
  private readonly document = inject(DOCUMENT);
  private readonly openSessions = inject(OpenSessions, { optional: true });
  private readonly destroyRef = inject(DestroyRef);

  private stream: LiveStream | null = null;
  private campaignId: string | null = null;
  private timer: ReturnType<typeof setTimeout> | null = null;

  constructor() {
    this.destroyRef.onDestroy(() => {
      this.follow(null, () => undefined);
      this.clearTimer();
    });
  }

  /** Follows the campaign while it has an open session (call it in a constructor: it makes an effect). */
  whileLive(campaignId: () => string, onChange: () => void): void {
    let firstRun = true;
    effect(() => {
      const id = campaignId();
      const live =
        id !== '' && (this.openSessions?.sessions().some((o) => o.campaignId === id) ?? false);
      // A session that was already open when the page came reads once, with the page's own load (the first `ready` asks
      // nothing). One that opens while the page is up connects up to 30 s later (the poll of open sessions): its first `ready`
      // reads, because the content may have changed in between.
      const readOnFirstReady = !firstRun;
      firstRun = false;
      untracked(() => this.follow(live ? id : null, onChange, readOnFirstReady));
    });
  }

  /** Follows `campaignId`'s session, or stops with `null`. The first `ready` is the page's own load, unless `readOnFirstReady`. */
  follow(campaignId: string | null, onChange: () => void, readOnFirstReady = false): void {
    if (campaignId === this.campaignId) {
      return;
    }
    this.stream?.stop();
    this.stream = null;
    this.clearTimer();
    this.campaignId = campaignId;
    if (campaignId === null) {
      return;
    }
    let first = !readOnFirstReady;
    const hint = () => {
      this.clearTimer();
      this.timer = setTimeout(() => {
        this.timer = null;
        onChange();
      }, CONTENT_DEBOUNCE_MS);
    };
    const stream = new LiveStream({
      open: (signal) => this.source.watch(campaignId, signal),
      classify: (err) => this.source.classifyError(err),
      document: this.document,
      handlers: {
        // The page reads on load itself: only a later `ready` reads again.
        onReady: () => {
          if (!first) {
            hint();
          }
          first = false;
        },
        onVitals: () => undefined,
        onContentChanged: hint,
        onEnded: () => this.stop(stream),
        onFatal: () => this.stop(stream),
      },
    });
    this.stream = stream;
    stream.start();
  }

  private stop(stream: LiveStream): void {
    stream.stop();
    if (this.stream === stream) {
      this.stream = null;
      this.campaignId = null;
    }
  }

  private clearTimer(): void {
    if (this.timer) {
      clearTimeout(this.timer);
      this.timer = null;
    }
  }
}
