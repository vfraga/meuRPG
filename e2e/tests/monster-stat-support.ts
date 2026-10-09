import { expect, type Page } from '@playwright/test';

import { type CombatTable, type Encounter } from './combat-support';
import { callRPC } from './support';
import { beginTheatreRPC } from './theatre-support';

export const DRAGON = 'Dragão vermelho adulto';

/** A combat without a map with an adult red dragon (revealed), the party and a Goblin: the dragon first on the order and the Goblin last,
 * so the turn that ends before the Goblin's is another creature's and the dragon is offered a legendary action (with only the dragon
 * and the party, the dragon's own turn would follow at once and close the offer). */
export async function dragonFight(master: Page, table: CombatTable): Promise<Encounter> {
  const res = await callRPC(master, 'meurpg.play.v1.CombatService/StartEncounter', {
    campaignId: table.campaignId,
    idempotencyKey: crypto.randomUUID(),
    name: 'O covil do dragão',
    participants: [{ characterId: table.goblinId, count: 1, hidden: false }],
    monsters: [{ creatureKey: 'monster:adult-red-dragon', count: 1 }],
    monstersHidden: false,
    mode: 'ENCOUNTER_MODE_THEATRE',
  });
  expect(res.ok(), await res.text()).toBeTruthy();
  const started = (await res.json()).encounter as Encounter;
  return beginTheatreRPC(master, table, started, { [DRAGON]: 20, Pensantus: 10, Goblin: 1 });
}
