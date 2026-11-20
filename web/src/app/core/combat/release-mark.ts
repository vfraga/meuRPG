import { signal } from '@angular/core';

import { type Encounter, ReactionBusy } from '../../../gen/meurpg/play/v1/combat_pb';
import type { CombatClient } from './combat-client';

/** The window the mark is about. */
export interface ReleaseMarkIds {
  readonly campaignId: string;
  readonly encounterId: string;
  readonly windowId: string;
}

/**
 * The reactor's mark of being on the map for a readied spell or move (#857, #856): set on entering the map (the cast sheet's map
 * step, the Mover page) and cleared with the same call (`clear`) on "Voltar". The server dates the one wait of 120 s from the first
 * mark and no later mark or clear restarts it; this only says where the player is. Telling is best effort: the map opens and
 * closes whatever the network says, and the stream shows the truth. Only the reactor's player makes it (the master is refused).
 */
export class ReleaseMark {
  /**
   * The clear of each window still in the air, whichever screen sent it: a new entry on the same window (the player went back and
   * opened the map again at once) sets its mark only after that clear landed, or the late clear would erase the newer mark.
   */
  private static readonly clearing = new Map<string, Promise<void>>();

  /** The mark is set, as far as this screen knows. */
  readonly on = signal(false);
  private finished = false;
  /** The window was seen rolling while the mark was on: a later "not rolling" is a reopen, not a window the mark never reached. */
  private seenRolling = false;
  /** The mark in the air: a "Voltar" tapped before it lands waits for it, then clears it. */
  private pending: Promise<void> | null = null;

  constructor(
    private readonly api: Pick<CombatClient['attacks'], 'markReactionRolling'>,
    private readonly ids: ReleaseMarkIds,
    private readonly apply: (e: Encounter) => void,
    /** What the player is busy with while marked: left out for the map (the first use of the mark), `DICE` for the typed roll of a release. */
    private readonly busy: ReactionBusy = ReactionBusy.UNSPECIFIED,
  ) {}

  /** On entering the map: marks the window, once. */
  enter(): Promise<void> {
    if (this.on() || this.finished) {
      return Promise.resolve();
    }
    if (this.pending) {
      return this.pending;
    }
    const clearing = ReleaseMark.clearing.get(this.ids.windowId);
    const marking = clearing ? clearing.then(() => this.mark()) : this.mark();
    this.pending = marking.finally(() => (this.pending = null));
    return this.pending;
  }

  private async mark(): Promise<void> {
    try {
      this.apply(await this.call(false));
      this.on.set(true);
    } catch {
      // The window may be gone already; the stream shows it.
    }
  }

  /** On "Voltar": takes the mark off, when it was set. Resolves once the combat the server answers is applied, so the window shows what is true. */
  leave(): Promise<void> {
    const run = (async () => {
      await this.pending;
      if (!this.on() || this.finished) {
        return;
      }
      this.on.set(false);
      this.seenRolling = false;
      await this.clear();
    })();
    ReleaseMark.clearing.set(this.ids.windowId, run);
    void run.finally(() => {
      if (ReleaseMark.clearing.get(this.ids.windowId) === run) {
        ReleaseMark.clearing.delete(this.ids.windowId);
      }
    });
    return run;
  }

  private async clear(): Promise<void> {
    try {
      this.apply(await this.call(true));
    } catch {
      // The window may have closed meanwhile.
    }
  }

  /**
   * The window as the combat has it, told on every change. An undo of the trigger reopens a READIED window clean (the server clears
   * `rolling`), so a player still on the map marks again, once, or the master reads "not on the map".
   */
  observe(w: { readonly rolling: boolean } | undefined): void {
    if (!w || !this.on() || this.finished) {
      return;
    }
    if (w.rolling) {
      this.seenRolling = true;
      return;
    }
    if (this.seenRolling) {
      this.seenRolling = false;
      void this.mark();
    }
  }

  /** The release went through: the window is closed, and there is nothing to clear. */
  finish(): void {
    this.finished = true;
    this.seenRolling = false;
    this.on.set(false);
  }

  private call(clear: boolean): Promise<Encounter> {
    const { campaignId, encounterId, windowId } = this.ids;
    // The clear names nothing, and the map's mark keeps the four arguments it always had.
    return clear || this.busy === ReactionBusy.UNSPECIFIED
      ? this.api.markReactionRolling(campaignId, encounterId, windowId, clear)
      : this.api.markReactionRolling(campaignId, encounterId, windowId, false, this.busy);
  }
}
