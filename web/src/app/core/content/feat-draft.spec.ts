import { create } from '@bufbuild/protobuf';

import { TableFeatSchema } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { draftToFeat, emptyFeat, featPaths, featToDraft } from './feat-draft';
import { menu } from './content-testing-menu';

const stored = () =>
  create(TableFeatSchema, {
    namePt: 'Mestre das Cordas',
    descPt: ['Primeiro.', 'Segundo.'],
    prerequisite: {
      minimums: { strength: 13 },
      anyOf: { dexterity: 13, wisdom: 13 },
      proficiencyKey: 'proficiency:medium-armor',
      raceKey: 'race:human',
      level: 4,
    },
    effects: [{ type: 'ability_increase', count: 1, from: ['strength', 'dexterity'], value: '1' }],
  });

describe('the feat form (MR-025)', () => {
  it('sends the options and the named choice back when only the name changes', () => {
    const body = create(TableFeatSchema, {
      namePt: 'Resistente',
      descPt: ['Texto.'],
      options: [{ key: 'feature:opcao@mesa', namePt: 'Opção', descPt: ['Texto.'] }],
      effects: [
        { type: 'choice', choice: 'ability', count: 1, name: 'habilidade' },
        { type: 'choice', choice: 'feature', count: 1, set: 'fighting_style' },
        { type: 'modifier', target: 'score.str', mode: 'add', value: '1', cap: 20 },
      ],
    });
    const draft = featToDraft(body);
    draft.feature = { ...draft.feature, name: 'Muito resistente' };
    const sent = draftToFeat(draft, menu());
    expect(sent.namePt).toBe('Muito resistente');
    expect(sent.options).toEqual(body.options);
    expect(sent.effects).toEqual([
      { type: 'choice', choice: 'ability', count: 1, name: 'habilidade' },
      { type: 'choice', choice: 'feature', count: 1, set: 'fighting_style' },
      { type: 'modifier', target: 'score.str', mode: 'add', value: '1', cap: 20 },
    ]);
  });

  it('turns a stored feat into a draft and back to the same body', () => {
    const draft = featToDraft(stored());
    expect(draft.feature.text).toBe('Primeiro.\n\nSegundo.');
    expect(draft.minimums.strength).toBe('13');
    expect(draft.minimums.dexterity).toBe('');
    expect(draft.level).toBe('4');
    expect(draftToFeat(draft, menu())).toEqual({
      namePt: 'Mestre das Cordas',
      descPt: ['Primeiro.', 'Segundo.'],
      prerequisite: {
        minimums: { strength: 13 },
        anyOf: { dexterity: 13, wisdom: 13 },
        proficiencyKey: 'proficiency:medium-armor',
        spellcasting: false,
        raceKey: 'race:human',
        races: [],
        size: '',
        lacksFeatureKey: '',
        level: 4,
      },
      effects: [
        { type: 'ability_increase', count: 1, from: ['strength', 'dexterity'], value: '1' },
      ],
    });
  });

  it('keeps the races, the size and the feature to lack when only the name changes', () => {
    const body = create(TableFeatSchema, {
      namePt: 'Sangue antigo',
      prerequisite: {
        races: ['race:elf', 'subrace:high-elf'],
        size: 'Medium',
        lacksFeatureKey: 'feat:grappler',
      },
    });
    const draft = featToDraft(body);
    const renamed = { ...draft, feature: { ...draft.feature, name: 'Sangue novo' } };
    const sent = draftToFeat(renamed, menu());
    expect(sent.namePt).toBe('Sangue novo');
    expect(sent.prerequisite).toEqual({
      proficiencyKey: '',
      spellcasting: false,
      raceKey: '',
      races: ['race:elf', 'subrace:high-elf'],
      size: 'Medium',
      lacksFeatureKey: 'feat:grappler',
      level: 0,
    });
  });

  it('drops the kept races when the master types the single race', () => {
    const draft = featToDraft(
      create(TableFeatSchema, { prerequisite: { races: ['race:elf', 'race:dwarf'] } }),
    );
    expect(draftToFeat({ ...draft, raceKey: 'race:human' }, menu()).prerequisite?.races).toEqual(
      [],
    );
  });

  it('sends a prerequisite that asks nothing as empty, and keeps no key on the feat', () => {
    const init = draftToFeat(
      { ...emptyFeat(), feature: { ...emptyFeat().feature, name: '  Livre ' } },
      menu(),
    );
    expect(init.namePt).toBe('Livre');
    expect(init.prerequisite).toEqual({
      proficiencyKey: '',
      spellcasting: false,
      raceKey: '',
      races: [],
      size: '',
      lacksFeatureKey: '',
      level: 0,
    });
    expect(init.effects).toEqual([]);
  });

  it('lists the inputs the editor draws, with the fields the effect type reads', () => {
    const paths = featPaths(featToDraft(stored()), menu());
    expect(paths).toEqual(
      expect.arrayContaining([
        'table_feat.name_pt',
        'table_feat.prerequisite.minimums.strength',
        'table_feat.prerequisite.level',
        'table_feat.prerequisite.races',
        'table_feat.prerequisite.size',
        'table_feat.prerequisite.lacks_feature_key',
        'table_feat.effects[0].type',
        'table_feat.effects[0].from',
        'table_feat.effects[0].value',
      ]),
    );
  });
});
