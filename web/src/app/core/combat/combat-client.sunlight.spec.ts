import { TestBed } from '@angular/core/testing';

import { SunlightScope } from '../../../gen/meurpg/play/v1/combat_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';
import { CombatClient } from './combat-client';

function fake() {
  const calls: Record<string, unknown>[] = [];
  TestBed.configureTestingModule({
    providers: [CombatClient, { provide: CONNECT_TRANSPORT, useValue: {} }],
  });
  const client = TestBed.inject(CombatClient);
  (client as unknown as { base: { client: unknown } }).base.client = {
    setSunlight: (req: Record<string, unknown>) => {
      calls.push(req);
      return Promise.resolve({ encounter: { id: 'enc' } });
    },
  };
  return { client, calls };
}

describe('CombatClient, the sunlight switch', () => {
  it('turns the sun on keeping the scope, with an idempotency key', async () => {
    const { client, calls } = fake();
    const enc = await client.combatants.setSunlight('c', 'e', { on: true });
    expect(enc.id).toBe('enc');
    expect(calls[0]).toMatchObject({
      campaignId: 'c',
      encounterId: 'e',
      on: true,
      scope: SunlightScope.UNSPECIFIED,
      addSquares: [],
      removeSquares: [],
    });
    expect(calls[0]['idempotencyKey']).toEqual(expect.any(String));
  });

  it('sends the scope and the squares it is given', async () => {
    const { client, calls } = fake();
    await client.combatants.setSunlight('c', 'e', {
      on: false,
      scope: SunlightScope.SQUARES,
      addSquares: [{ col: 1, row: 2 }],
      removeSquares: [{ col: 3, row: 4 }],
    });
    expect(calls[0]).toMatchObject({
      on: false,
      scope: SunlightScope.SQUARES,
      addSquares: [{ col: 1, row: 2 }],
      removeSquares: [{ col: 3, row: 4 }],
    });
  });

  it('resends the same key when the caller keeps it across a retry', async () => {
    const { client, calls } = fake();
    await client.combatants.setSunlight('c', 'e', { on: true }, 'k1');
    await client.combatants.setSunlight('c', 'e', { on: true }, 'k1');
    expect(calls.map((r) => r['idempotencyKey'])).toEqual(['k1', 'k1']);
  });
});
