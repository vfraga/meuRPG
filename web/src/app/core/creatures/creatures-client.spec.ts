import { TestBed } from '@angular/core/testing';

import { CreatureDismissal } from '../../../gen/meurpg/characters/v1/characters_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';
import { CreaturesClient } from './creatures-client';

/** The real client with its two services replaced by recorders: what goes on the wire for a bonded steed. */
describe('CreaturesClient, the requests of a bonded steed', () => {
  function withRequests() {
    TestBed.configureTestingModule({
      providers: [CreaturesClient, { provide: CONNECT_TRANSPORT, useValue: {} }],
    });
    const client = TestBed.inject(CreaturesClient);
    const dismissals: Record<string, unknown>[] = [];
    const casts: Record<string, unknown>[] = [];
    const inner = client as unknown as { characters: unknown; play: unknown };
    inner.characters = {
      dismissCreature: (req: Record<string, unknown>) => {
        dismissals.push(req);
        return Promise.resolve({});
      },
    };
    inner.play = {
      castSummon: (req: Record<string, unknown>) => {
        casts.push(req);
        return Promise.resolve({ creatureIds: ['steed-1'], dismissedCreatureIds: ['old-1'] });
      },
    };
    return { client, dismissals, casts };
  }

  it('sends how a steed goes as `dismissal`: DISMISS keeps the bond, RELEASE ends it; a plain dismissal sends none', async () => {
    const { client, dismissals } = withRequests();
    await client.dismiss('camp-1', 'steed-1', 'dismiss');
    await client.dismiss('camp-1', 'steed-1', 'release');
    await client.dismiss('camp-1', 'cr-1');
    expect(dismissals).toEqual([
      { campaignId: 'camp-1', creatureId: 'steed-1', dismissal: CreatureDismissal.DISMISS },
      { campaignId: 'camp-1', creatureId: 'steed-1', dismissal: CreatureDismissal.RELEASE },
      { campaignId: 'camp-1', creatureId: 'cr-1' },
    ]);
  });

  it("sends the steed's type picked on the first cast as `summon.creature_type`, and none for any other summon", async () => {
    const { client, casts } = withRequests();
    const cast = {
      campaignId: 'camp-1',
      characterId: 'char-1',
      spellKey: 'spell:find-steed',
      ritual: false,
      slot: { level: 2, pact: false },
      idempotencyKey: 'k',
    };
    const res = await client.castSummon({
      ...cast,
      summon: { option: 0, creatureKeys: ['monster:warhorse'], names: [], creatureType: 'fey' },
    });
    await client.castSummon({
      ...cast,
      summon: { option: 0, creatureKeys: ['monster:wolf'], names: [] },
    });
    expect(casts[0]['summon']).toEqual({
      option: 0,
      creatureKeys: ['monster:warhorse'],
      names: [],
      creatureType: 'fey',
    });
    expect(casts[1]['summon']).toEqual({ option: 0, creatureKeys: ['monster:wolf'], names: [] });
    expect(res).toEqual({ creatureIds: ['steed-1'], replacedIds: ['old-1'] });
  });
});
