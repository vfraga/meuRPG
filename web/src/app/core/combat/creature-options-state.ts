import { signal } from '@angular/core';

import type { GetTurnOptionsResponse } from '../../../gen/meurpg/play/v1/combat_pb';
import type { CombatClient } from './combat-client';

/**
 * The turn options of each creature the player plays (`GetTurnOptions` on a creature: its attacks with their
 * numbers, its standard actions, its movement; the owner's player and the master only). Like `TurnOptionsState`,
 * but for several at a time: the page asks again whenever the combat changes, a slow answer that arrives after a
 * newer question was sent is dropped, and a creature whose read failed keeps what it had.
 */
export class CreatureOptionsState {
  readonly data = signal<ReadonlyMap<string, GetTurnOptionsResponse>>(new Map());
  private asked = 0;

  async load(
    api: CombatClient,
    campaignId: string,
    encounterId: string,
    ids: readonly string[],
  ): Promise<void> {
    const mine = ++this.asked;
    const results = await Promise.allSettled(
      ids.map((id) => api.moves.turnOptions(campaignId, encounterId, id)),
    );
    if (mine !== this.asked) {
      return;
    }
    const before = this.data();
    const next = new Map<string, GetTurnOptionsResponse>();
    results.forEach((r, i) => {
      const value = r.status === 'fulfilled' ? r.value : before.get(ids[i]);
      if (value) {
        next.set(ids[i], value);
      }
    });
    this.data.set(next);
  }

  clear(): void {
    this.asked++;
    this.data.set(new Map());
  }
}
