import { signal } from '@angular/core';

import type { GetTurnOptionsResponse } from '../../../gen/meurpg/play/v1/combat_pb';
import type { CombatClient } from './combat-client';

/**
 * The options of one combatant on this screen (`GetTurnOptions`): the
 * player's own, or the master's current one. The page asks again whenever
 * the combat changes; a slow answer that arrives after a newer question was
 * sent is dropped, so the screen never goes back to an older economy.
 */
export class TurnOptionsState {
  readonly data = signal<GetTurnOptionsResponse | null>(null);
  private asked = 0;
  private subject = '';

  async load(
    api: CombatClient,
    campaignId: string,
    encounterId: string,
    combatantId: string,
  ): Promise<void> {
    const mine = ++this.asked;
    // Another combatant's economy is never shown for this one while the answer comes.
    if (this.subject !== combatantId) {
      this.subject = combatantId;
      this.data.set(null);
    }
    try {
      const res = await api.turnOptions(campaignId, encounterId, combatantId);
      if (mine === this.asked) {
        this.data.set(res);
      }
    } catch {
      // Best effort: the next change of the combat asks again.
    }
  }

  clear(): void {
    this.asked++;
    this.subject = '';
    this.data.set(null);
  }
}
