import { computed, signal } from '@angular/core';

import type { GetTurnOptionsResponse } from '../../../gen/meurpg/play/v1/combat_pb';
import type { CombatClient } from './combat-client';

/**
 * The options of one combatant on this screen (`GetTurnOptions`): the
 * player's own, or the master's current one. The page asks again whenever
 * the combat changes. An answer is shown when it is newer than the one on
 * screen, even if a newer question is already out: the screen never goes
 * back to an older economy, and never keeps an older one while a fresher
 * answer is in hand (dropping it kept the economy from before the last
 * action on screen until the next answer came).
 */
export class TurnOptionsState {
  readonly data = signal<GetTurnOptionsResponse | null>(null);
  /** The number of the last question asked, and of the answer on screen. */
  private readonly asked = signal(0);
  private readonly shown = signal(0);
  /**
   * A newer question is out than the answer on screen: the options may still be the ones from
   * before the last change of the combat (an action just spent). "Encerrar turno" waits for
   * them before it decides whether to ask.
   */
  readonly settling = computed(() => this.asked() > this.shown());
  private subject = '';

  async load(
    api: CombatClient,
    campaignId: string,
    encounterId: string,
    combatantId: string,
  ): Promise<void> {
    const mine = this.asked() + 1;
    this.asked.set(mine);
    // Another combatant's economy is never shown for this one while the answer comes, and no
    // answer asked for before belongs to this one.
    if (this.subject !== combatantId) {
      this.subject = combatantId;
      this.data.set(null);
      this.shown.set(mine - 1);
    }
    try {
      const res = await api.moves.turnOptions(campaignId, encounterId, combatantId);
      if (mine > this.shown()) {
        this.shown.set(mine);
        this.data.set(res);
      }
    } catch {
      // Best effort: the next change of the combat asks again. A failed question does not keep
      // the screen settling for ever: what is on screen stays.
      if (mine > this.shown()) {
        this.shown.set(mine);
      }
    }
  }

  clear(): void {
    const mine = this.asked() + 1;
    this.asked.set(mine);
    this.shown.set(mine);
    this.subject = '';
    this.data.set(null);
  }
}
