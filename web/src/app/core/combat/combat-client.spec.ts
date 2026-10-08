import { TestBed } from '@angular/core/testing';
import { ConnectError, Code } from '@connectrpc/connect';

import { CONNECT_TRANSPORT } from '../connect/transport';
import { CombatClient } from './combat-client';

// A change the combat sends carries the key of its request: a retry after a lost answer reuses it (the server
// answers with what the first call did), other values take a new one, and a worked change frees the key.

interface Sent {
  readonly endTurn: { idempotencyKey: string; expectedCombatantId: string }[];
  readonly answers: (Error | null)[];
}

function clientWith(sent: Sent): CombatClient {
  TestBed.configureTestingModule({
    providers: [CombatClient, { provide: CONNECT_TRANSPORT, useValue: {} }],
  });
  const client = TestBed.inject(CombatClient);
  (client as unknown as { client: unknown }).client = {
    endTurn: (req: { idempotencyKey: string; expectedCombatantId: string }) => {
      sent.endTurn.push(req);
      const failure = sent.answers.shift();
      return failure ? Promise.reject(failure) : Promise.resolve({ encounter: { id: 'enc' } });
    },
  };
  return client;
}

describe('CombatClient keys', () => {
  it('sends the same key when the same change is tried again after a lost answer', async () => {
    const sent: Sent = { endTurn: [], answers: [new ConnectError('lost', Code.Unavailable), null] };
    const client = clientWith(sent);
    await expect(client.endTurn('c', 'e', 'a')).rejects.toBeInstanceOf(ConnectError);
    await client.endTurn('c', 'e', 'a');
    expect(sent.endTurn).toHaveLength(2);
    expect(sent.endTurn[1].idempotencyKey).toBe(sent.endTurn[0].idempotencyKey);
  });

  it('takes a new key for other values', async () => {
    const sent: Sent = { endTurn: [], answers: [new ConnectError('lost', Code.Unavailable), null] };
    const client = clientWith(sent);
    await expect(client.endTurn('c', 'e', 'a')).rejects.toBeInstanceOf(ConnectError);
    await client.endTurn('c', 'e', 'b');
    expect(sent.endTurn[1].idempotencyKey).not.toBe(sent.endTurn[0].idempotencyKey);
  });

  it('takes a new key for the same values once the change worked', async () => {
    const sent: Sent = { endTurn: [], answers: [] };
    const client = clientWith(sent);
    await client.endTurn('c', 'e', 'a');
    await client.endTurn('c', 'e', 'a');
    expect(sent.endTurn[1].idempotencyKey).not.toBe(sent.endTurn[0].idempotencyKey);
  });
});
