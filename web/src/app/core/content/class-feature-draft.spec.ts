import { create } from '@bufbuild/protobuf';

import { TableClassFeatureSchema } from '../../../gen/meurpg/rules/v1/table_content_pb';
import {
  classFeaturePaths,
  classFeatureToDraft,
  draftToClassFeature,
  emptyClassFeature,
  unreadableClassFeaturePaths,
} from './class-feature-draft';
import { menu } from './content-testing-menu';

const stored = () =>
  create(TableClassFeatureSchema, {
    namePt: 'Rastro do Corvo',
    descPt: ['Primeiro.', 'Segundo.'],
    classKey: 'class:ranger',
    level: 1,
    replacesFeatureKey: 'feature:favored-enemy-1-type',
    effects: [{ type: 'note', textPt: 'Segue o rastro de um corvo.' }],
  });

describe('the class feature form (MR-025)', () => {
  it('turns a stored feature into a draft and back to the same body', () => {
    const draft = classFeatureToDraft(stored());
    expect(draft.feature.text).toBe('Primeiro.\n\nSegundo.');
    expect(draft.level).toBe('1');
    expect(draftToClassFeature(draft, menu())).toEqual({
      namePt: 'Rastro do Corvo',
      descPt: ['Primeiro.', 'Segundo.'],
      classKey: 'class:ranger',
      level: 1,
      replacesFeatureKey: 'feature:favored-enemy-1-type',
      effects: [{ type: 'note', textPt: 'Segue o rastro de um corvo.' }],
    });
  });

  it('sends an empty level as 0 and a feature that replaces nothing as empty', () => {
    const init = draftToClassFeature(emptyClassFeature(), menu());
    expect(init.level).toBe(0);
    expect(init.replacesFeatureKey).toBe('');
    expect(init.effects).toEqual([]);
  });

  it('lists the inputs the editor draws, with the fields of each effect type', () => {
    const paths = classFeaturePaths(classFeatureToDraft(stored()), menu());
    expect(paths).toContain('table_class_feature.class_key');
    expect(paths).toContain('table_class_feature.level');
    expect(paths).toContain('table_class_feature.replaces_feature_key');
    expect(paths).toContain('table_class_feature.effects[0].text_pt');
  });

  it('keeps the summon of a class feature through a load and a save, and draws its inputs', () => {
    const withSummon = create(TableClassFeatureSchema, {
      namePt: 'Companheiro do Vale',
      classKey: 'class:ranger',
      level: 3,
      effects: [{ type: 'resource', resource: 'companheiro', max: '1' }],
      summon: {
        creatureKeys: ['creature:lobo@mesa', 'monster:wolf'],
        count: 'prof()',
        resourceKey: 'companheiro',
        resourceCost: 1,
        maxLive: 2,
        turnMode: 2,
      },
    });
    const draft = classFeatureToDraft(withSummon);
    expect(draft.feature.summon?.count).toBe('prof()');
    expect(draftToClassFeature(draft, menu()).summon).toEqual(withSummon.summon);
    expect(classFeaturePaths(draft, menu())).toContain('table_class_feature.summon.creature_keys');
    expect(unreadableClassFeaturePaths(draft, menu())).toEqual([]);
    expect(
      unreadableClassFeaturePaths(
        {
          ...draft,
          feature: { ...draft.feature, summon: { ...draft.feature.summon!, resourceKey: '' } },
        },
        menu(),
      ),
    ).toEqual(['table_class_feature.summon.resource_key']);
  });

  it('sends no summon when the class feature has none', () => {
    expect(draftToClassFeature(classFeatureToDraft(stored()), menu())).not.toHaveProperty('summon');
  });
});
