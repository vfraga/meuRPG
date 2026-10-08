import { DOCUMENT, Injectable, inject } from '@angular/core';

import { LiveSessionSourceLive } from '../live-session/live-session-source.live';
import { LiveStream } from '../live-session/live-stream';

/**
 * Listens to a campaign's live session for `xp_changed` (E7-10) and
 * `creatures_changed` (MR-037) and says so, so a page that shows XP or the
 * creatures (the character sheet, the campaign's list) reads again when the
 * master gives some or a creature arrives. It is the session page's own stream client (`LiveStream`, ADR-0005's
 * rules: backoff, closed when the tab is hidden, no reconnect when there is no
 * access) over `LiveSessionSourceLive`, not a copy of it. One stream at a time;
 * `follow(null)` closes it, which the page does when it goes away. Provided at
 * the sheet's route, so the generated play client stays in that lazy chunk.
 */
@Injectable()
export class XpWatcher {
  private readonly source = inject(LiveSessionSourceLive);
  private readonly document = inject(DOCUMENT);

  private stream: LiveStream | null = null;
  private campaignId: string | null = null;

  /** Follows `campaignId` (its open session), or stops with `null`. `onChange`
   * runs on every `xp_changed`, and on a reconnection (an event may have been missed);
   * `onCreatures`, when given, on every `creatures_changed` and on a reconnection too, and `onContent` on every
   * `content_changed` (the table's content moved: the editor and the level-up read their catalog again), and
   * `onForm` when a character's hit points or the combat changed (a Wild Shape form ends that way), with the
   * character of a vitals event, or `null` when it is the combat that changed. */
  follow(
    campaignId: string | null,
    onChange: () => void,
    onCreatures?: () => void,
    onContent?: () => void,
    onForm?: (characterId: string | null) => void,
  ): void {
    if (campaignId === this.campaignId) {
      return;
    }
    this.stream?.stop();
    this.stream = null;
    this.campaignId = campaignId;
    if (campaignId === null) {
      return;
    }
    let first = true;
    const stream: LiveStream = new LiveStream({
      open: (signal) => this.source.watch(campaignId, signal),
      classify: (err) => this.source.classifyError(err),
      document: this.document,
      handlers: {
        // The page reads on load itself: only a later `ready` (a reconnection, the tab back after a
        // while) reads again, both the XP and the creatures: an event may have been missed.
        onReady: () => {
          if (!first) {
            onChange();
            onCreatures?.();
            onContent?.();
            onForm?.(null);
          }
          first = false;
        },
        onVitals: (v) => onForm?.(v.characterId),
        onEncounterChanged: () => onForm?.(null),
        onXpChanged: onChange,
        onCreaturesChanged: onCreatures,
        onContentChanged: onContent,
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
}
