import { TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';

import { EncounterBlockedReason } from '../../../gen/meurpg/play/v1/combat_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';
import { CombatClient } from './combat-client';
import { blockedMessage } from './combat-errors';

interface Sent {
  answer: { campaignId: string; combatantId: string; endRage: boolean; idempotencyKey: string }[];
  end: { combatantId: string; idempotencyKey: string }[];
  fail: Error[];
}

function clientWith(sent: Sent): CombatClient {
  TestBed.configureTestingModule({
    providers: [CombatClient, { provide: CONNECT_TRANSPORT, useValue: {} }],
  });
  const client = TestBed.inject(CombatClient);
  const reply = () => {
    const failure = sent.fail.shift();
    return failure ? Promise.reject(failure) : Promise.resolve({ encounter: { id: 'enc' } });
  };
  (client as unknown as { base: { client: unknown } }).base.client = {
    answerRageEnd: (req: Sent['answer'][number]) => {
      sent.answer.push(req);
      return reply();
    },
    endRage: (req: Sent['end'][number]) => {
      sent.end.push(req);
      return reply();
    },
  };
  return client;
}

describe('the rage calls', () => {
  it('sends the answer to the rage question with the combatant and the choice', async () => {
    const sent: Sent = { answer: [], end: [], fail: [] };
    const client = clientWith(sent);
    await client.turn.answerRageEnd('c', 'e', 't', false);
    await client.turn.answerRageEnd('c', 'e', 't', true);
    expect(sent.answer.map((a) => [a.combatantId, a.endRage])).toEqual([
      ['t', false],
      ['t', true],
    ]);
  });

  it('keeps the key of "Encerrar fúria" across a retry after a lost answer', async () => {
    const sent: Sent = { answer: [], end: [], fail: [new ConnectError('lost', Code.Unavailable)] };
    const client = clientWith(sent);
    await expect(client.turn.endRage('c', 'e', 't')).rejects.toBeInstanceOf(ConnectError);
    await client.turn.endRage('c', 'e', 't');
    expect(sent.end[1].idempotencyKey).toBe(sent.end[0].idempotencyKey);
  });

  it('refuses spellcasting while raging in plain Portuguese', () => {
    expect(blockedMessage({ reason: EncounterBlockedReason.RAGING_CANNOT_CAST } as never)).toBe(
      'Você está em fúria: não pode lançar magias.',
    );
    expect(blockedMessage({ reason: EncounterBlockedReason.NOT_RAGING } as never)).toMatch(
      /não está em fúria/,
    );
  });
});
