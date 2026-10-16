import { computed, signal } from '@angular/core';

import type { CombatLogRound } from '../../../gen/meurpg/play/v1/combat_pb';
import type { CombatClient } from './combat-client';
import { undoableEntry } from './combat-log';

/**
 * The combat log on this screen (`ListCombatLog`): read again after every
 * `combat_log_changed` and every reconnection (the page's `logTick`), the
 * latest answer wins. It keeps the rounds as the server sent them (the undo
 * needs the structured entry); the panel draws the groups from them.
 */
export class CombatLogState {
  readonly rounds = signal<readonly CombatLogRound[]>([]);
  /** The event `UndoLastAction` would take back; only the master gets it. */
  readonly undoableId = signal('');
  readonly loaded = signal(false);
  readonly undoable = computed(() => undoableEntry(this.rounds()));
  private asked = 0;

  async load(api: CombatClient, campaignId: string, encounterId: string): Promise<void> {
    const mine = ++this.asked;
    try {
      const res = await api.turn.log(campaignId, encounterId);
      if (mine === this.asked) {
        this.rounds.set(res.rounds);
        this.undoableId.set(res.undoableEventId);
        this.loaded.set(true);
      }
    } catch {
      // Best effort: the next `combat_log_changed` reads it again.
    }
  }

  clear(): void {
    this.asked++;
    this.rounds.set([]);
    this.undoableId.set('');
    this.loaded.set(false);
  }
}
