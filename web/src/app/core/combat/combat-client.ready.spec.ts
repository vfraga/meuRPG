import { TestBed } from '@angular/core/testing';

import { ReadiedKind } from '../../../gen/meurpg/play/v1/combat_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';
import { CombatClient } from './combat-client';

function fake(answer: Record<string, unknown>) {
  const calls: { name: string; req: Record<string, unknown> }[] = [];
  TestBed.configureTestingModule({
    providers: [CombatClient, { provide: CONNECT_TRANSPORT, useValue: {} }],
  });
  const client = TestBed.inject(CombatClient);
  const call = (name: string) => (req: Record<string, unknown>) => {
    calls.push({ name, req });
    return Promise.resolve(answer);
  };
  (client.ready as unknown as { client: unknown }).client = {
    readyAction: call('readyAction'),
    fireReadied: call('fireReadied'),
    cancelReadied: call('cancelReadied'),
  };
  return { client, calls };
}

describe('CombatClient, readying an action (#582)', () => {
  it('sends the readied action with the trigger, the kind, the keys and the idempotency key it is given', async () => {
    const { client, calls } = fake({ encounter: { id: 'enc' } });
    const enc = await client.ready.readyAction(
      {
        campaignId: 'c',
        encounterId: 'e',
        combatantId: 't',
        triggerText: 'Se a porta abrir',
        kind: ReadiedKind.SPELL,
        actionKey: '',
        spellKey: 'spell:magic-missile',
        slotLevel: 1,
        targetCombatantId: 'z1',
      },
      'k1',
    );
    expect(enc).toEqual({ id: 'enc' });
    expect(calls).toEqual([
      {
        name: 'readyAction',
        req: {
          campaignId: 'c',
          encounterId: 'e',
          combatantId: 't',
          triggerText: 'Se a porta abrir',
          kind: ReadiedKind.SPELL,
          actionKey: '',
          spellKey: 'spell:magic-missile',
          slotLevel: 1,
          targetCombatantId: 'z1',
          idempotencyKey: 'k1',
        },
      },
    ]);
  });

  it('fires and gives up a readied action by its id', async () => {
    const { client, calls } = fake({ encounter: { id: 'enc' } });
    await client.ready.fireReadied('c', 'e', 'r1', 'k2');
    await client.ready.cancelReadied('c', 'e', 'r1', 'k3');
    expect(calls).toEqual([
      {
        name: 'fireReadied',
        req: { campaignId: 'c', encounterId: 'e', readiedId: 'r1', idempotencyKey: 'k2' },
      },
      {
        name: 'cancelReadied',
        req: { campaignId: 'c', encounterId: 'e', readiedId: 'r1', idempotencyKey: 'k3' },
      },
    ]);
  });

  it('fails the call when the answer carries no combat', async () => {
    const { client } = fake({});
    await expect(client.ready.cancelReadied('c', 'e', 'r1', 'k')).rejects.toThrow();
  });
});
