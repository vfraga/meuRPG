import { type MessageInitShape, create } from '@bufbuild/protobuf';

import {
  type SummonSpellOptions,
  SummonSpellOptionsSchema,
} from '../../../gen/meurpg/characters/v1/characters_pb';

/** Convocar Montaria for a paladin (#588): two forms, the pony too small to ride, the three types, and the notes "Como funciona" folds. */
export function steedSpell(
  over: MessageInitShape<typeof SummonSpellOptionsSchema> = {},
): SummonSpellOptions {
  return create(SummonSpellOptionsSchema, {
    spellKey: 'spell:find-steed',
    namePt: 'Convocar Montaria',
    level: 2,
    castingTimePt: '10 minutos',
    typeChoices: ['celestial', 'fey', 'fiend'],
    minInt: 6,
    notes: ['Vocês se falam por telepatia a até 1,5 km.'],
    circles: [
      {
        circle: 2,
        options: [
          {
            count: 1,
            forms: [
              { monsterKey: 'monster:warhorse', namePt: 'Cavalo de guerra' },
              { monsterKey: 'monster:pony', namePt: 'Pônei' },
            ],
            tooSmallForYou: ['monster:pony'],
          },
        ],
      },
    ],
    ...over,
  });
}
