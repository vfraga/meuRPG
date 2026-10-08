import { signal } from '@angular/core';

import type { GetTrapNoticersResponse } from '../../../gen/meurpg/maps/v1/maps_pb';
import { type TrapActivity, type TrapDamage } from '../../../gen/meurpg/play/v1/traps_pb';
import type { MapsClient } from '../maps/maps-client';
import type { TrapsClient } from './traps-client';

/**
 * What the session page knows about the traps beyond the map's own points (MR-035): what they
 * did outside a combat (`ListTrapActivity`: the master reads every firing, search and notice; a
 * player their own), the damage that waits for the master (`ListTrapDamages`, master only) and
 * "Quem notaria" for the trap whose card is open. Plain TypeScript with signals: the page reads it
 * again on `map_changed`, `encounter_changed`, `combat_log_changed`, a token that moved and every `ready`, and a
 * stale answer never overwrites a newer one. Best effort: a failed read keeps what is on screen.
 */
export class TrapBoard {
  readonly activity = signal<readonly TrapActivity[]>([]);
  readonly damages = signal<readonly TrapDamage[]>([]);
  readonly noticers = signal<ReadonlyMap<string, GetTrapNoticersResponse>>(new Map());
  /** The traps whose "Quem notaria" could not be read: the card says so and offers "Tentar de novo". */
  readonly noticersFailed = signal<ReadonlySet<string>>(new Set());
  readonly loaded = signal(false);

  private readonly noticersAsked = new Map<string, number>();
  private activityAsked = 0;
  private damagesAsked = 0;
  private readonly open = new Set<string>();
  private movedReading = false;
  private movedAgain = false;

  constructor(
    private readonly traps: TrapsClient,
    private readonly maps: MapsClient,
    private readonly campaignId: () => string,
    private readonly mapId: () => string | null,
    private readonly isMaster: () => boolean,
  ) {}

  /** The activity, the damages that wait and "Quem notaria" of every open card. */
  async refresh(): Promise<void> {
    await Promise.all([this.refreshActivity(), this.refreshDamages(), this.refreshNoticers()]);
  }

  async refreshActivity(): Promise<void> {
    const mine = ++this.activityAsked;
    try {
      const res = await this.traps.activity(this.campaignId());
      if (mine === this.activityAsked) {
        this.activity.set(res.activity);
        this.loaded.set(true);
      }
    } catch {
      // No open session, or the network: the next hint reads it again.
    }
  }

  async refreshDamages(): Promise<void> {
    if (!this.isMaster()) {
      return;
    }
    const mine = ++this.damagesAsked;
    try {
      const res = await this.traps.damages(this.campaignId());
      if (mine === this.damagesAsked) {
        this.damages.set(res.damages);
      }
    } catch {
      // Best effort.
    }
  }

  /** "Quem notaria" for a trap: asked when its card opens and again on every refresh while it is open. */
  async watchNoticers(pointId: string): Promise<void> {
    this.open.add(pointId);
    await this.loadNoticers(pointId);
  }

  unwatchNoticers(pointId: string): void {
    this.open.delete(pointId);
  }

  /** "Disparar…" needs a trap's "Quem notaria" even when its card is closed: read it while the dialog is open (the
   * dialog follows the answer as it arrives). The function returned stops, unless the card itself was watching. */
  watchWhileFiring(pointId: string): () => void {
    if (this.open.has(pointId)) {
      return () => undefined;
    }
    void this.watchNoticers(pointId);
    return () => this.unwatchNoticers(pointId);
  }

  /**
   * A token moved: who is near each open trap is not what it was. Moves come in bursts (a drag), so a read in flight
   * is followed by one more, never by one for each move.
   */
  async tokensMoved(): Promise<void> {
    if (this.movedReading) {
      this.movedAgain = true;
      return;
    }
    this.movedReading = true;
    try {
      do {
        this.movedAgain = false;
        await this.refreshNoticers();
      } while (this.movedAgain);
    } finally {
      this.movedReading = false;
    }
  }

  private async refreshNoticers(): Promise<void> {
    await Promise.all([...this.open].map((id) => this.loadNoticers(id)));
  }

  /** "Quem notaria" again, after a failed read ("Tentar de novo"). */
  async retryNoticers(pointId: string): Promise<void> {
    await this.loadNoticers(pointId);
  }

  private async loadNoticers(pointId: string): Promise<void> {
    const mapId = this.mapId();
    if (!mapId || !this.isMaster()) {
      return;
    }
    // A stale answer never overwrites a newer one: each read of a trap counts.
    const mine = (this.noticersAsked.get(pointId) ?? 0) + 1;
    this.noticersAsked.set(pointId, mine);
    try {
      const res = await this.maps.getTrapNoticers(this.campaignId(), mapId, pointId);
      if (this.noticersAsked.get(pointId) === mine && this.open.has(pointId)) {
        this.noticers.update((m) => new Map(m).set(pointId, res));
        this.noticersFailed.update((s) => new Set([...s].filter((id) => id !== pointId)));
      }
    } catch {
      if (this.noticersAsked.get(pointId) === mine) {
        this.noticersFailed.update((s) => new Set(s).add(pointId));
      }
    }
  }

  /** The latest firing of a trap in the activity (the one to add creatures to), or `null`. */
  firingOf(pointId: string): TrapActivity | null {
    const list = this.activity().filter((a) => a.firing?.pointId === pointId);
    return list.length > 0 ? list[list.length - 1] : null;
  }

  /** A damage was applied or discarded: gone from the list at once, read again in the background. */
  settled(id: string): void {
    this.damages.update((list) => list.filter((d) => d.id !== id));
    void this.refresh();
  }

  clear(): void {
    this.activityAsked++;
    this.damagesAsked++;
    this.activity.set([]);
    this.damages.set([]);
    this.noticers.set(new Map());
    this.noticersFailed.set(new Set());
    this.noticersAsked.clear();
    this.open.clear();
    this.loaded.set(false);
  }
}
