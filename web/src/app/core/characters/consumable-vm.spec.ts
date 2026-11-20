import { create } from '@bufbuild/protobuf';

import {
  ConsumableKind,
  SheetConsumableSchema,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { consumableOf, signedText } from './consumable-vm';

describe('consumableOf', () => {
  it('has none for an item the server sent no consumable for', () => {
    expect(consumableOf(undefined)).toBeUndefined();
  });

  it('reads the kind, the cost and the healing dice the server sent', () => {
    const c = consumableOf(
      create(SheetConsumableSchema, {
        kind: ConsumableKind.POTION,
        costsAction: true,
        healDiceCount: 2,
        healDiceSides: 4,
        healBonus: 2,
      }),
    );
    expect(c).toEqual({
      kind: 'potion',
      costsAction: true,
      heal: { count: 2, sides: 4, modifier: 2, text: '2d4 + 2' },
    });
  });

  it('writes the dice with no number added, and with a number taken off', () => {
    const dice = (bonus: number) =>
      consumableOf(
        create(SheetConsumableSchema, {
          kind: ConsumableKind.POTION,
          healDiceCount: 3,
          healDiceSides: 6,
          healBonus: bonus,
        }),
      )?.heal?.text;
    expect(dice(0)).toBe('3d6');
    expect(dice(-1)).toBe('3d6 − 1');
  });

  it('knows the oil, the scroll and any other kind, and has no dice when the use rolls nothing', () => {
    const kind = (k: ConsumableKind) => consumableOf(create(SheetConsumableSchema, { kind: k }));
    expect(kind(ConsumableKind.OIL)).toEqual({ kind: 'oil', costsAction: false, heal: null });
    expect(kind(ConsumableKind.SCROLL)?.kind).toBe('scroll');
    expect(kind(ConsumableKind.OTHER)?.kind).toBe('other');
    expect(kind(ConsumableKind.UNSPECIFIED)?.kind).toBe('other');
  });
});

describe('the lasting effect of a use', () => {
  it('reads the title and the length the server sent, and says nothing of its own', () => {
    const c = consumableOf(
      create(SheetConsumableSchema, {
        kind: ConsumableKind.POTION,
        costsAction: true,
        effectTitlePt: 'Resistência a fogo',
        effectDurationTextPt: 'por 1 hora',
      }),
    );
    expect(c?.lasting).toEqual({ title: 'Resistência a fogo', length: 'por 1 hora' });
  });

  it('has none when the server sent no title: a healing potion, an oil, a scroll', () => {
    const c = consumableOf(create(SheetConsumableSchema, { kind: ConsumableKind.POTION }));
    expect(c?.lasting).toBeUndefined();
  });
});

describe('signedText', () => {
  it('writes a number added with a plus and one taken off with a minus sign', () => {
    expect(signedText(2)).toBe('+ 2');
    expect(signedText(-1)).toBe('− 1');
  });
});
