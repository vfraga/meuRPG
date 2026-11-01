import { TestBed } from '@angular/core/testing';

import { RestKind } from '../../../gen/meurpg/play/v1/resources_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';
import { ResourceClient } from './resources-client';

interface Calls {
  readonly preview: unknown[];
  readonly rest: unknown[];
  readonly spend: unknown[];
  readonly answers: unknown[];
}

function clientWith(calls: Calls): ResourceClient {
  TestBed.configureTestingModule({
    providers: [ResourceClient, { provide: CONNECT_TRANSPORT, useValue: {} }],
  });
  const client = TestBed.inject(ResourceClient);
  (client as unknown as { client: unknown }).client = {
    getRestPreview: (req: unknown) => {
      calls.preview.push(req);
      return Promise.resolve({ characters: [], longRestAlreadyTaken: false });
    },
    takeRest: (req: unknown) => {
      calls.rest.push(req);
      return Promise.resolve({ vitals: [{ characterId: 'c1' }] });
    },
    answerHeldRoll: (req: unknown) => {
      calls.answers.push(req);
      return Promise.resolve({});
    },
    spendHitDice: (req: unknown) => {
      calls.spend.push(req);
      return Promise.resolve({ face: 4, constitutionModifier: 2, healed: 6 });
    },
  };
  return client;
}

describe('ResourceClient', () => {
  let calls: Calls;
  beforeEach(() => (calls = { preview: [], rest: [], spend: [], answers: [] }));

  it('asks what a rest would give back, writing nothing', async () => {
    await clientWith(calls).restPreview('camp', RestKind.LONG);
    expect(calls.preview).toEqual([{ campaignId: 'camp', kind: RestKind.LONG }]);
  });

  it('takes a rest under the given key with the hit dice choices, and answers the new vitals', async () => {
    const vitals = await clientWith(calls).takeRest('camp', RestKind.LONG, 'key-1', [
      { characterId: 'c1', dice: [{ faces: 10, count: 2 }] },
    ]);
    expect(calls.rest).toEqual([
      {
        campaignId: 'camp',
        kind: RestKind.LONG,
        idempotencyKey: 'key-1',
        hitDiceChoices: [{ characterId: 'c1', dice: [{ faces: 10, count: 2 }] }],
        withoutFoodOrDrink: false,
      },
    ]);
    expect(vitals.map((v) => v.characterId)).toEqual(['c1']);
  });

  it('says a long rest had no food or drink, so no level of exhaustion comes off', async () => {
    await clientWith(calls).takeRest('camp', RestKind.LONG, 'key-3', [], true);
    expect(calls.rest[0]).toMatchObject({ kind: RestKind.LONG, withoutFoodOrDrink: true });
  });

  it('takes a short rest without choices', async () => {
    await clientWith(calls).takeRest('camp', RestKind.SHORT, 'key-2');
    expect(calls.rest[0]).toMatchObject({ kind: RestKind.SHORT, hitDiceChoices: [] });
  });

  it('spends a die the app rolls, or one whose face the player typed', async () => {
    const client = clientWith(calls);
    await client.spendHitDice('camp', 'c1', 8, { inApp: true }, 'k1');
    const typed = await client.spendHitDice('camp', 'c1', 8, { face: 5 }, 'k2');
    await client.spendHitDice('camp', 'c1', 8, { face: 5, songFace: 4 }, 'k3');
    expect(calls.spend).toEqual([
      {
        campaignId: 'camp',
        characterId: 'c1',
        faces: 8,
        idempotencyKey: 'k1',
        roll: { case: 'rollInApp', value: true },
        typedSongFace: 0,
      },
      {
        campaignId: 'camp',
        characterId: 'c1',
        faces: 8,
        idempotencyKey: 'k2',
        roll: { case: 'typedFace', value: 5 },
        typedSongFace: 0,
      },
      {
        campaignId: 'camp',
        characterId: 'c1',
        faces: 8,
        idempotencyKey: 'k3',
        roll: { case: 'typedFace', value: 5 },
        typedSongFace: 4,
      },
    ]);
    expect(typed.healed).toBe(6);
  });

  it('answers Stroke of Luck with use or keep and no roll', async () => {
    const client = clientWith(calls);
    await client.answerHeldRoll('camp', 'h1', { kind: 'stroke', use: true }, 'k1');
    await client.answerHeldRoll('camp', 'h1', { kind: 'stroke', use: false }, 'k2');
    const answers = calls.answers.map(
      (c) => (c as { answer: { case: string; value: unknown } }).answer,
    );
    expect(answers).toEqual([
      { case: 'strokeOfLuck', value: { use: true } },
      { case: 'strokeOfLuck', value: { use: false } },
    ]);
  });

  it('answers Indomitable with a typed face, the two typed faces of a pair, or the app', async () => {
    const client = clientWith(calls);
    await client.answerHeldRoll(
      'camp',
      'h1',
      { kind: 'indomitable', reroll: true, roll: { face: 12 } },
      'k1',
    );
    await client.answerHeldRoll(
      'camp',
      'h1',
      { kind: 'indomitable', reroll: true, roll: { faces: [4, 15] } },
      'k2',
    );
    await client.answerHeldRoll(
      'camp',
      'h1',
      { kind: 'indomitable', reroll: false, roll: null },
      'k3',
    );
    const answers = calls.answers.map((c) => (c as { answer: { value: unknown } }).answer.value);
    expect(answers).toEqual([
      { reroll: true, roll: { case: 'typedFace', value: 12 }, typedFaces: [] },
      { reroll: true, roll: { case: undefined }, typedFaces: [4, 15] },
      { reroll: false, roll: { case: undefined }, typedFaces: [] },
    ]);
  });
});
