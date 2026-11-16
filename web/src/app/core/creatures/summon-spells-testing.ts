import { create } from '@bufbuild/protobuf';

import {
  type SummonSpellOptions,
  SummonSpellOptionsSchema,
} from '../../../gen/meurpg/characters/v1/characters_pb';

type Init<T> = Partial<Omit<T, '$typeName' | '$unknown'>>;

/** Convocar Familiar for a wizard: a ritual, no slot, one creature out of the given forms (key, name, none attack). */
export function familiarSpell(
  forms: [string, string][] = [
    ['monster:raven', 'Corvo'],
    ['monster:cat', 'Gato'],
    ['monster:bat', 'Morcego'],
  ],
  over: Init<SummonSpellOptions> = {},
): SummonSpellOptions {
  return create(SummonSpellOptionsSchema, {
    spellKey: 'spell:find-familiar',
    namePt: 'Convocar Familiar',
    level: 1,
    castingTimePt: '1 hora',
    ritual: true,
    canRitual: true,
    circles: [
      {
        circle: 1,
        options: [
          {
            count: 1,
            attack: 1,
            forms: forms.map(([monsterKey, namePt]) => ({ monsterKey, namePt, attack: 1 })),
          },
        ],
      },
    ],
    ...over,
  });
}

/** Animar Mortos for a cleric with slots of the 3rd and the 5th circle: 1 and 5 undead. */
export function undeadSpell(over: Init<SummonSpellOptions> = {}): SummonSpellOptions {
  const forms = [
    { monsterKey: 'monster:skeleton', namePt: 'Esqueleto', attack: 3 },
    { monsterKey: 'monster:zombie', namePt: 'Zumbi', attack: 3 },
  ];
  return create(SummonSpellOptionsSchema, {
    spellKey: 'spell:animate-dead',
    namePt: 'Animar Mortos',
    level: 3,
    castingTimePt: '1 minuto',
    canCastWithSlot: true,
    circles: [
      { circle: 3, options: [{ count: 1, forms }] },
      { circle: 5, options: [{ count: 5, forms }] },
    ],
    ...over,
  });
}

/** Conjurar Animais for a druid: four options by challenge rating, at the 3rd circle. */
export function beastSpell(over: Init<SummonSpellOptions> = {}): SummonSpellOptions {
  return create(SummonSpellOptionsSchema, {
    spellKey: 'spell:conjure-animals',
    namePt: 'Conjurar Animais',
    level: 3,
    castingTimePt: '1 ação',
    concentration: true,
    canCastWithSlot: true,
    circles: [
      {
        circle: 3,
        options: [
          { count: 1, type: 'beast', maxCr: '2', attack: 3 },
          { count: 2, type: 'beast', maxCr: '1', attack: 3 },
          { count: 4, type: 'beast', maxCr: '1/2', attack: 3 },
          { count: 8, type: 'beast', maxCr: '1/4', attack: 3 },
        ],
      },
    ],
    ...over,
  });
}
