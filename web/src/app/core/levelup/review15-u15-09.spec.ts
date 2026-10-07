import { create } from '@bufbuild/protobuf';

import {
  Ability,
  DerivedClassSchema,
  SpellcastingSchema,
  type DerivedSheet,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { changeRows, type SummaryContext } from './levelup-summary';
import { pensantus } from './levelup-testing';

// Wizard 3 / Cleric 1 levelling Cleric. The server lists `spellcasting` in class order (Wizard first).
function multiclass(after: boolean): DerivedSheet {
  const sheet = pensantus(false); // Wizard 3 numbers: DC 14, +6, 3 cantrips, 7 prepared (unchanged)
  sheet.classes = [
    create(DerivedClassSchema, { classKey: 'class:wizard', namePt: 'Mago', level: 3 }),
    create(DerivedClassSchema, { classKey: 'class:cleric', namePt: 'Clérigo', level: after ? 2 : 1 }),
  ];
  sheet.spellcasting = [
    ...sheet.spellcasting,
    create(SpellcastingSchema, {
      classKey: 'class:cleric',
      classNamePt: 'Clérigo',
      ability: Ability.WISDOM,
      saveDc: after ? 13 : 12,
      attackBonus: after ? 5 : 4,
      cantripsKnown: after ? 4 : 3,
      preparedMax: after ? 5 : 4,
    }),
  ];
  return sheet;
}

describe('Review15 U15-9: the level-up summary reads the spellcasting of the class being levelled', () => {
  const ctx = {
    hpSub: '',
    cantrips: ['Chama Sagrada'],
    spells: [],
    prepared: ['Bênção'],
    spellbook: false,
    spellsMissing: 0,
    learnsSpells: false,
    // The class being levelled (options.classKey); the summary has to be told which one it is.
    classKey: 'class:cleric',
  } as SummaryContext;
  const rows = changeRows(multiclass(false), multiclass(true), ctx);
  const row = (key: string) => rows.find((r) => r.key === key);

  it("shows the Cleric's spell DC and attack going up", () => {
    expect(row('dc')).toMatchObject({ before: '12', after: '13' });
    expect(row('attack')).toMatchObject({ before: '+4', after: '+5' });
  });

  it("shows the Cleric's cantrips and prepared spells with their new names", () => {
    expect(row('cantrips')).toMatchObject({
      before: '3',
      after: '4',
      sub: 'Novo: Chama Sagrada',
    });
    expect(row('prepared')).toMatchObject({ before: '4', after: '5', sub: 'Nova: Bênção' });
  });
});
