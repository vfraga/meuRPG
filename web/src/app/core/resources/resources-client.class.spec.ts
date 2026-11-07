import { TestBed } from '@angular/core/testing';

import { LayOnHandsCure } from '../../../gen/meurpg/play/v1/resources_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';
import { ResourceClient } from './resources-client';

function clientWith(sent: unknown[], answer: unknown = {}): ResourceClient {
  TestBed.configureTestingModule({
    providers: [ResourceClient, { provide: CONNECT_TRANSPORT, useValue: {} }],
  });
  const client = TestBed.inject(ResourceClient);
  const call = (req: unknown) => {
    sent.push(req);
    return Promise.resolve(answer);
  };
  (client as unknown as { client: unknown }).client = {
    useLayOnHands: call,
    createSpellSlot: call,
    convertSpellSlot: call,
    giveBardicInspiration: call,
    answerHeldRoll: call,
    listHeldRolls: call,
  };
  return client;
}

describe('ResourceClient: the class resource calls', () => {
  it('touches with an amount, or with a cure, under the key of the caller', async () => {
    const sent: unknown[] = [];
    const client = clientWith(sent);
    await client.useLayOnHands('c', 'e', 'a', 't', { amount: 8 }, 'k1');
    await client.useLayOnHands('c', 'e', 'a', 't', { cure: 'poison' }, 'k2');
    await client.useLayOnHands('c', 'e', 'a', 't', { cure: 'disease' }, 'k3');
    expect(sent).toEqual([
      {
        campaignId: 'c',
        encounterId: 'e',
        actorId: 'a',
        targetId: 't',
        idempotencyKey: 'k1',
        effect: { case: 'amount', value: 8 },
      },
      expect.objectContaining({ effect: { case: 'cure', value: LayOnHandsCure.POISON } }),
      expect.objectContaining({ effect: { case: 'cure', value: LayOnHandsCure.DISEASE } }),
    ]);
  });

  it('creates and converts a slot by its level', async () => {
    const sent: unknown[] = [];
    const client = clientWith(sent);
    await client.createSpellSlot('c', 'e', 'a', 2, 'k1');
    await client.convertSpellSlot('c', 'e', 'a', 3, 'k2');
    expect(sent).toEqual([
      { campaignId: 'c', encounterId: 'e', actorId: 'a', slotLevel: 2, idempotencyKey: 'k1' },
      { campaignId: 'c', encounterId: 'e', actorId: 'a', slotLevel: 3, idempotencyKey: 'k2' },
    ]);
  });

  it('gives the die to a creature', async () => {
    const sent: unknown[] = [];
    await clientWith(sent).giveBardicInspiration('c', 'e', 'a', 't', 'k');
    expect(sent).toEqual([
      { campaignId: 'c', encounterId: 'e', actorId: 'a', targetId: 't', idempotencyKey: 'k' },
    ]);
  });

  it('answers any held roll in one call: Lucky rolled again in the app or typed, the 1 kept, the die used or kept', async () => {
    const sent: unknown[] = [];
    const answer = { next: undefined };
    const client = clientWith(sent, answer);
    expect(
      await client.answerHeldRoll(
        'c',
        'h',
        { kind: 'lucky', reroll: true, roll: { inApp: true } },
        'k1',
      ),
    ).toBe(answer);
    await client.answerHeldRoll(
      'c',
      'h',
      { kind: 'lucky', reroll: true, roll: { face: 14 } },
      'k2',
    );
    await client.answerHeldRoll('c', 'h', { kind: 'lucky', reroll: false, roll: null }, 'k3');
    await client.answerHeldRoll('c', 'h', { kind: 'die', use: true, roll: { inApp: true } }, 'k4');
    await client.answerHeldRoll('c', 'h', { kind: 'die', use: true, roll: { face: 5 } }, 'k5');
    await client.answerHeldRoll('c', 'h', { kind: 'die', use: false, roll: null }, 'k6');
    expect(sent).toEqual([
      {
        campaignId: 'c',
        holdId: 'h',
        idempotencyKey: 'k1',
        answer: {
          case: 'lucky',
          value: { reroll: true, roll: { case: 'rollInApp', value: true } },
        },
      },
      expect.objectContaining({
        answer: { case: 'lucky', value: { reroll: true, roll: { case: 'typedFace', value: 14 } } },
      }),
      // Keeping sends no roll at all.
      expect.objectContaining({
        answer: { case: 'lucky', value: { reroll: false, roll: { case: undefined } } },
      }),
      expect.objectContaining({
        answer: { case: 'die', value: { use: true, roll: { case: 'rollInApp', value: true } } },
      }),
      expect.objectContaining({
        answer: { case: 'die', value: { use: true, roll: { case: 'typedFace', value: 5 } } },
      }),
      expect.objectContaining({
        answer: { case: 'die', value: { use: false, roll: { case: undefined } } },
      }),
    ]);
  });

  it('answers a reroll rule’s question with the boolean alone: the server rolls the new d20', async () => {
    const sent: unknown[] = [];
    const client = clientWith(sent);
    await client.answerHeldRoll('c', 'h', { kind: 'reroll', reroll: true, roll: null }, 'k1');
    await client.answerHeldRoll('c', 'h', { kind: 'reroll', reroll: false, roll: null }, 'k2');
    expect(sent).toEqual([
      expect.objectContaining({ answer: { case: 'reroll', value: { reroll: true } } }),
      expect.objectContaining({ answer: { case: 'reroll', value: { reroll: false } } }),
    ]);
  });

  it('lists the questions that wait for the campaign', async () => {
    const sent: unknown[] = [];
    await clientWith(sent).listHeldRolls('c');
    expect(sent).toEqual([{ campaignId: 'c' }]);
  });
});
