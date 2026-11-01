import { create } from '@bufbuild/protobuf';

import {
  AttackRollSchema,
  DiceRollSchema,
  RollAttackResponseSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { answeredText, settledRoll } from './held-roll-result';
import { heldAnswer, heldDie, heldLucky, heldStroke } from './held-roll-testing';

/** The d20 as the server writes it once Lucky rolled the 1 again: 14 + 5 = 19. */
const rerolled = create(DiceRollSchema, {
  diceCount: 1,
  diceSides: 20,
  faces: [14],
  countedIndex: 0,
  modifier: 5,
  total: 19,
  rerolled: [{ index: 0, from: 1, to: 14, sourceKey: 'trait:lucky' }],
});

const attackAnswer = (d20 = rerolled) =>
  heldAnswer({
    result: {
      case: 'attack',
      value: create(RollAttackResponseSchema, {
        roll: create(AttackRollSchema, { d20 }),
      }),
    },
  });

describe('the roll a held answer settled', () => {
  it('reads the d20 where each path writes it', () => {
    expect(settledRoll(attackAnswer())?.rerolled).toHaveLength(1);
    expect(settledRoll(heldAnswer({ next: heldDie(14, 5, 8, {}, 1) }))?.rerolled).toHaveLength(1);
    expect(settledRoll(heldAnswer())).toBeUndefined();
  });
});

describe('what the master’s answer came to', () => {
  const reroll = { kind: 'lucky', reroll: true, roll: { inApp: true } } as const;

  it('says the new face and the total when Lucky rolled the d20 again', () => {
    expect(answeredText('Brisa', reroll, heldLucky(), attackAnswer())).toBe(
      'Você respondeu por Brisa: rolou de novo e tirou 14 (19 com o bônus).',
    );
  });

  it('says Stroke of Luck was used or the roll kept', () => {
    expect(answeredText('Brisa', { kind: 'stroke', use: true }, heldStroke(), attackAnswer())).toBe(
      'Você respondeu por Brisa: usou o Golpe de Sorte.',
    );
    expect(answeredText('Brisa', { kind: 'stroke', use: false }, heldStroke(), heldAnswer())).toBe(
      'Você respondeu por Brisa: manteve o resultado.',
    );
  });

  it('says the face typed with no bonus to add', () => {
    const plain = create(DiceRollSchema, { ...rerolled, modifier: 0, total: 14 });
    expect(answeredText('Brisa', reroll, heldLucky(), attackAnswer(plain))).toBe(
      'Você respondeu por Brisa: rolou de novo e tirou 14.',
    );
  });

  it('says the 1 was kept', () => {
    expect(
      answeredText(
        'Brisa',
        { kind: 'lucky', reroll: false, roll: null },
        heldLucky(),
        heldAnswer(),
      ),
    ).toBe('Você respondeu por Brisa: ficou com o 1.');
  });

  it('says the die was added or kept', () => {
    expect(
      answeredText(
        'Brisa',
        { kind: 'die', use: true, roll: { inApp: true } },
        heldDie(),
        heldAnswer(),
      ),
    ).toBe('Você respondeu por Brisa: somou o d8.');
    expect(
      answeredText('Brisa', { kind: 'die', use: false, roll: null }, heldDie(), heldAnswer()),
    ).toBe('Você respondeu por Brisa: guardou o dado.');
  });

  it('falls back on a plain line when the result carries no d20', () => {
    expect(answeredText('Brisa', reroll, heldLucky(), heldAnswer())).toBe(
      'Você respondeu por Brisa: rolou de novo.',
    );
  });
});
