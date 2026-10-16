import { DOCUMENT, Injectable, inject } from '@angular/core';

import { LiveSessionSourceLive } from './live-session-source.live';
import type { VitalsVm } from './live-session.types';
import { LiveStream } from './live-stream';

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
   * character of a vitals event, or `null` when it is the combat that changed. `onVitals`, when given, gets the live
   * numbers of the session's characters (the counters on the sheet): the snapshot on every `ready` (the master's has them
   * all, a player's only their own character) and each `vitals_changed` after it. `onCharacter` runs when the master asked for changes, its
   * player sent it again or it was revived (`character_changes_requested`, `character_resubmitted`,
   * `character_revived`), with that character. */
  follow(
    campaignId: string | null,
    onChange: () => void,
    onCreatures?: () => void,
    onContent?: () => void,
    onForm?: (characterId: string | null) => void,
    onVitals?: (vitals: VitalsVm) => void,
    onCharacter?: (characterId: string) => void,
  ): void {
    if (campaignId === this.campaignId) {
      // The same session, asked again (the page moved to another character of it, or read its sheet again): the live
      // numbers are read again, the stream stays.
      if (campaignId !== null && onVitals) {
        this.readVitals(campaignId, onVitals);
      }
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
          if (onVitals) {
            this.readVitals(campaignId, onVitals);
          }
          if (!first) {
            onChange();
            onCreatures?.();
            onContent?.();
            onForm?.(null);
          }
          first = false;
        },
        onVitals: (v) => {
          onVitals?.(v);
          onForm?.(v.characterId);
        },
        onEncounterChanged: () => onForm?.(null),
        onXpChanged: onChange,
        onCreaturesChanged: onCreatures,
        onContentChanged: onContent,
        onCharacterChanged: onCharacter,
        onEnded: () => this.stop(stream),
        onFatal: () => this.stop(stream),
      },
    });
    this.stream = stream;
    stream.start();
  }

  /** The snapshot is a read: a failed one leaves what is on screen, and the next event or reconnection reads again. */
  private readVitals(campaignId: string, onVitals: (vitals: VitalsVm) => void): void {
    this.source.getLiveSession(campaignId).then(
      (snapshot) => snapshot.vitals.forEach((v) => onVitals(v)),
      () => undefined,
    );
  }

  private stop(stream: LiveStream): void {
    stream.stop();
    if (this.stream === stream) {
      this.stream = null;
      this.campaignId = null;
    }
  }
}
