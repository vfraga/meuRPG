import { TestBed } from '@angular/core/testing';

import { CONNECT_TRANSPORT } from '../connect/transport';
import { CombatClient } from './combat-client';
import { create } from '@bufbuild/protobuf';
import {
  AnswerReactionResponseSchema,
  AttackOutcome,
  ReactionBusy,
} from '../../../gen/meurpg/play/v1/combat_pb';

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
  (client as unknown as { base: { client: unknown } }).base.client = {
    answerReaction: call('answerReaction'),
    markReactionRolling: call('markReactionRolling'),
  };
  return { client, calls };
}

const enc = { id: 'enc' };

describe('CombatClient, the roll of a readied release (#938)', () => {
  it('sends one typed d20, the pair of d20, the pool sum and the faces an effect adds, each in its own field', async () => {
    const { client, calls } = fake({ encounter: enc });
    await client.attacks.answerReaction('c', 'e', 'w', { use: true, die: { typed: 14 } }, 'k');
    await client.attacks.answerReaction(
      'c',
      'e',
      'w',
      { use: true, die: { d20Faces: [4, 17] } },
      'k',
    );
    await client.attacks.answerReaction('c', 'e', 'w', { use: true, die: { poolSum: 23 } }, 'k');
    await client.attacks.answerReaction(
      'c',
      'e',
      'w',
      { use: true, die: { typed: 9 }, extraDieFaces: [3] },
      'k',
    );
    const [one, pair, pool, extra] = calls.map((c) => c.req);
    expect(one['roll']).toEqual({ case: 'typed', value: 14 });
    expect(one['d20Faces']).toEqual([]);
    expect(pair['d20Faces']).toEqual([4, 17]);
    expect(pair['roll']).toEqual({ case: undefined });
    expect(pool['roll']).toEqual({ case: 'poolSum', value: 23 });
    expect(extra['extraDieFaces']).toEqual([3]);
    expect(one['extraDieFaces']).toEqual([]);
  });

  it('hands back what the release rolled: the attack roll, the damage it opened and the cast', async () => {
    const res = create(AnswerReactionResponseSchema, {
      encounter: enc as never,
      attackRoll: { outcome: AttackOutcome.HIT },
      pendingDamage: { id: 'pd1' },
      cast: { castId: 'cast1' },
    } as never);
    const { client } = fake(res as never);
    const out = await client.attacks.answerReaction(
      'c',
      'e',
      'w',
      { use: true, die: { typed: 14 } },
      'k',
    );
    expect(out.attackRoll?.outcome).toBe(AttackOutcome.HIT);
    expect(out.pendingDamage?.id).toBe('pd1');
    expect(out.cast?.castId).toBe('cast1');
  });

  it('marks the window with what the player is busy with, and says nothing of it for the map', async () => {
    const { client, calls } = fake({ encounter: enc });
    await client.attacks.markReactionRolling('c', 'e', 'w', false, ReactionBusy.DICE);
    await client.attacks.markReactionRolling('c', 'e', 'w');
    expect(calls[0].req['busy']).toBe(ReactionBusy.DICE);
    expect(calls[1].req['busy']).toBe(ReactionBusy.UNSPECIFIED);
  });
});
