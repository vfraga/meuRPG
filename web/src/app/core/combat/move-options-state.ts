import { signal } from '@angular/core';

import type { GetMoveOptionsResponse } from '../../../gen/meurpg/play/v1/combat_pb';
import type { CombatClient } from './combat-client';

/**
 * Where one combatant can go (`GetMoveOptions`), as the page last read it: the
 * player's own on their turn, or any combatant for the master's "Mostrar o
 * alcance". Asked again whenever the combat changes; a slow answer that arrives
 * after a newer question is dropped, so the page never draws an older circle.
 */
export class MoveOptionsState {
  readonly data = signal<GetMoveOptionsResponse | null>(null);
  /** The read failed (the page says so): not a reason to block the move, the server still decides. */
  readonly failed = signal(false);
  private asked = 0;
  private subject = '';

  async load(
    api: CombatClient,
    campaignId: string,
    encounterId: string,
    combatantId: string,
  ): Promise<void> {
    const mine = ++this.asked;
    // Another combatant's circle is never drawn for this one while the answer comes.
    if (this.subject !== combatantId) {
      this.subject = combatantId;
      this.data.set(null);
      this.failed.set(false);
    }
    try {
      const res = await api.moveOptions(campaignId, encounterId, combatantId);
      if (mine === this.asked) {
        this.data.set(res);
        this.failed.set(false);
      }
    } catch {
      // The last good answer stays: the page draws it, and the server still judges the move.
      if (mine === this.asked) {
        this.failed.set(true);
      }
    }
  }

  clear(): void {
    this.asked++;
    this.subject = '';
    this.data.set(null);
    this.failed.set(false);
  }
}
