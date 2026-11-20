import { create } from '@bufbuild/protobuf';

import { TableFeatureSchema } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { featureItem } from './content-read-parts';
import { catalog } from './content-testing';

const cat = catalog();
const names = (k: string): string =>
  ({ 'feature:golpe@mesa': 'Golpe do Corvo' })[k] ?? cat.nameOf(k);

// "Como os jogadores veem" and the players' page read one function (#580, board 580-03): the "Pede" line of an option is
// worded by `optionAsksPhrase` and ends up in the item of the option.
describe('the options of a feature in what a player reads (#580, board 580-03)', () => {
  const feature = create(TableFeatureSchema, {
    namePt: 'Truques de lâmina',
    descPt: ['Escolha um truque.'],
    options: [
      { namePt: 'Golpe do Corvo', descPt: ['Soma o dado ao dano.'] },
      {
        namePt: 'Asa Partida',
        descPt: ['Soma o dado ao ataque.'],
        prerequisite: {
          classLevel: { classKey: 'class:paladin', level: 7 },
          requiresOptionKeys: ['feature:golpe@mesa'],
        },
      },
    ],
  });

  it('gives an option that asks the sentence "Pede Paladino 7 e Golpe do Corvo", and one that asks nothing none', () => {
    const [free, asking] = featureItem(feature, names).options ?? [];
    expect(free.asks).toBeUndefined();
    expect(asking.asks).toBe('Pede Paladino 7 e Golpe do Corvo');
    expect(asking.text).toBe('Soma o dado ao ataque.');
  });
});
