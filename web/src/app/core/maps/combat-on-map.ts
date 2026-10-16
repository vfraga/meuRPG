import { Injectable, inject } from '@angular/core';

import { EncounterStatus } from '../../../gen/meurpg/play/v1/combat_pb';
import { CombatClient } from '../combat/combat-client';

/**
 * Whether a combat that has not ended runs on a map (the editor's "Combate em andamento": the grid and the image
 * of that map cannot change, painting can). It reads the open session's latest combat (`GetEncounter`); no open
 * session, no combat, or a failed read all say "no": the server refuses the change anyway (`MapBlocked
 * COMBAT_RUNNING`), so the page only warns early. `providedIn: 'root'`, imported only by lazy code.
 */
@Injectable({ providedIn: 'root' })
export class CombatOnMap {
  private readonly combat = inject(CombatClient);

  async running(campaignId: string, mapId: string): Promise<boolean> {
    try {
      const encounter = await this.combat.turn.get(campaignId);
      return (
        encounter !== null &&
        encounter.mapId === mapId &&
        encounter.status !== EncounterStatus.ENDED
      );
    } catch {
      return false;
    }
  }
}
