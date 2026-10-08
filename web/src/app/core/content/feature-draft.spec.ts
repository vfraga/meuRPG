import { create } from '@bufbuild/protobuf';

import {
  TableBackgroundSchema,
  TableSubraceSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { abilities, menu, feature } from './content-testing';
import {
  backgroundToDraft,
  bonusText,
  choiceAmounts,
  draftToBackground,
  draftToRace,
  draftToSubrace,
  emptyBackground,
  emptyFeature,
  emptyRace,
  featurePaths,
  featureToDraft,
  draftToFeature,
  noBonuses,
  subraceToDraft,
} from './feature-draft';

describe('the race, subrace and background forms (E10-01 states 6 and 7)', () => {
  const m = menu();

  it('sends a race: metres to feet, the bonuses, the languages, and the choice bonuses only when the player places them', () => {
    const d = {
      ...emptyRace(),
      name: ' Corujeiro ',
      speedM: '9',
      darkvisionM: '18',
      bonuses: { ...noBonuses(), wisdom: 2, dexterity: 1 },
      languages: ['language:common'],
      languageChoices: 1,
    };
    expect(draftToRace(d, m)).toMatchObject({
      namePt: 'Corujeiro',
      size: 'Medium',
      speedFt: 30,
      darkvisionFt: 60,
      abilityBonuses: { wisdom: 2, dexterity: 1, strength: 0 },
      choiceBonuses: [],
      languages: ['language:common'],
      languageChoices: 1,
    });
    expect(draftToRace({ ...d, choosing: true, choice: '2, 1' }, m)).toMatchObject({
      choiceBonuses: [2, 1],
    });
    expect(choiceAmounts('+2 e 1')).toEqual([2, 1]);
    expect(bonusText(d.bonuses, abilities)).toBe('Destreza +1, Sabedoria +2');
    expect(bonusText(noBonuses(), abilities)).toBe('Nenhum');
  });

  it('keeps the key of a feature the editor read, and leaves a new one with none', () => {
    const f = featureToDraft(feature('Planar', [{ type: 'note', textPt: 'Cai devagar.' }]));
    expect(f.key).toBe('feature:Planar');
    expect(draftToFeature(f, m)).toEqual({
      key: 'feature:Planar',
      namePt: 'Planar',
      descPt: ['Texto.'],
      effects: [{ type: 'note', textPt: 'Cai devagar.' }],
    });
    expect(draftToFeature({ ...emptyFeature(), name: 'Novo' }, m)).toEqual({
      key: '',
      namePt: 'Novo',
      descPt: [],
      effects: [],
    });
  });

  it("folds away a note that only repeats the trait's text, and sends it again when sent", () => {
    const f = featureToDraft({ ...feature('Planar', [{ type: 'note', textPt: 'Texto.' }]) });
    expect(f.effects[0].textPt).toBe('');
    expect(draftToFeature(f, m).effects).toEqual([{ type: 'note', textPt: 'Texto.' }]);
    const own = featureToDraft(feature('Planar', [{ type: 'note', textPt: 'Cai devagar.' }]));
    expect(own.effects[0].textPt).toBe('Cai devagar.');
  });

  it('sends a subrace with its race, which never changes', () => {
    const stored = create(TableSubraceSchema, {
      namePt: 'Da Colina',
      raceKey: 'race:corujeiro@mesa',
      abilityBonuses: { constitution: 1 },
    });
    const body = draftToSubrace(subraceToDraft(stored), m);
    expect(body).toMatchObject({
      namePt: 'Da Colina',
      raceKey: 'race:corujeiro@mesa',
      abilityBonuses: { constitution: 1 },
      traits: [],
    });
  });

  it('sends a background: two skills, tools, the equipment and the one feature', () => {
    const stored = create(TableBackgroundSchema, {
      namePt: 'Cartógrafo do Vale',
      skills: ['skill:arcana', 'skill:history'],
      tools: ['proficiency:cartographers-tools'],
      languageChoices: 1,
      equipmentPt: 'Um estojo de mapas',
      feature: feature('Mapas na memória', [{ type: 'note', textPt: 'Lembra.' }]),
    });
    const d = backgroundToDraft(stored);
    expect(draftToBackground(d, m)).toEqual({
      namePt: 'Cartógrafo do Vale',
      skills: ['skill:arcana', 'skill:history'],
      tools: ['proficiency:cartographers-tools'],
      languageChoices: 1,
      equipmentPt: 'Um estojo de mapas',
      feature: {
        key: 'feature:Mapas na memória',
        namePt: 'Mapas na memória',
        descPt: ['Texto.'],
        effects: [{ type: 'note', textPt: 'Lembra.' }],
      },
    });
    // Two skills are asked for; one left empty is not sent as a blank.
    expect(
      (
        draftToBackground({ ...emptyBackground(), name: 'X', skills: ['skill:arcana', ''] }, m) as {
          skills: string[];
        }
      ).skills,
    ).toEqual(['skill:arcana']);
  });

  it("lists the paths a list of features draws inputs for: its fields and each effect's fields from the menu", () => {
    const f = featureToDraft(
      feature('Olhos', [{ type: 'proficiency', proficiency: 'skill:perception' }]),
    );
    const paths = featurePaths('table_race.traits', [f], m);
    expect(paths).toContain('table_race.traits[0].name_pt');
    expect(paths).toContain('table_race.traits[0].effects[0].proficiency');
    expect(paths).toContain('table_race.traits[0].effects[0].level');
    // A field the type does not read has no input: a refusal there lands on the effect.
    expect(paths).not.toContain('table_race.traits[0].effects[0].range_ft');
  });

  it('does not turn a speed or darkvision it cannot read into 0, which means "none"', () => {
    for (const text of ['18 m', '18m', '1.000,5', 'abc', '-3']) {
      const race = draftToRace({ ...emptyRace(), name: 'X', speedM: text, darkvisionM: text }, m);
      expect(race.speedFt, `speedM "${text}"`).toBeNaN();
      expect(race.darkvisionFt, `darkvisionM "${text}"`).toBeNaN();
    }
    const read = draftToRace({ ...emptyRace(), name: 'X', speedM: '9', darkvisionM: '4,5' }, m);
    expect([read.speedFt, read.darkvisionFt]).toEqual([30, 15]);
    const none = draftToRace({ ...emptyRace(), name: 'X', speedM: '', darkvisionM: '0' }, m);
    expect([none.speedFt, none.darkvisionFt]).toEqual([0, 0]);
  });
});
