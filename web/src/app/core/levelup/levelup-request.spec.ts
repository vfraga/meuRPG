import { LevelUpHitPointsMethod } from '../../../gen/meurpg/characters/v1/characters_pb';
import { choicesRequest, type RequestSource } from './levelup-request';
import { fighterOptions } from './levelup-testing';

const source: RequestSource = {
  options: fighterOptions(),
  abilityIncrease: {},
  featKey: '',
  subclassKey: '',
  cantrips: new Set(),
  spells: new Set(),
  prepared: new Set(),
  features: new Set(),
  skills: new Set(),
  expertise: new Set(),
  instrument: '',
  lateDrafts: new Map(),
  newDrafts: new Map(),
};

describe('choicesRequest', () => {
  it('writes the empty level as the class, nothing else and the average', () => {
    expect(choicesRequest(source, null)).toEqual({
      classKey: 'class:fighter',
      abilityIncrease: {},
      featKey: '',
      subclassKey: '',
      cantripKeys: [],
      knownSpellKeys: [],
      preparedSpellKeys: [],
      featureChoiceKeys: [],
      lateChoiceKeys: [],
      featureChoiceText: {},
      skillProficiencyKeys: [],
      expertiseSkillKeys: [],
      instrumentKey: '',
      hitPoints: { method: LevelUpHitPointsMethod.AVERAGE },
    });
  });

  it('writes the abilities by their wire names, skipping the ones with no increase', () => {
    const request = choicesRequest(
      { ...source, abilityIncrease: { str: 1, dex: 0, cha: 1 } },
      null,
    );
    expect(request.abilityIncrease).toEqual({ strength: 1, charisma: 1 });
  });

  it('writes the picks as lists, and the die as rolled in the app or typed', () => {
    const request = choicesRequest(
      {
        ...source,
        subclassKey: 'sub:x',
        featKey: 'feat:y',
        cantrips: new Set(['c1']),
        spells: new Set(['s1', 's2']),
        prepared: new Set(['p1']),
        features: new Set(['f1']),
        skills: new Set(['k1']),
        expertise: new Set(['e1']),
        instrument: 'i1',
      },
      { kind: 'app', value: 5 },
    );
    expect(request).toMatchObject({
      subclassKey: 'sub:x',
      featKey: 'feat:y',
      cantripKeys: ['c1'],
      knownSpellKeys: ['s1', 's2'],
      preparedSpellKeys: ['p1'],
      featureChoiceKeys: ['f1'],
      skillProficiencyKeys: ['k1'],
      expertiseSkillKeys: ['e1'],
      instrumentKey: 'i1',
      hitPoints: { method: LevelUpHitPointsMethod.ROLLED_IN_APP, value: 5 },
    });
    expect(choicesRequest(source, { kind: 'physical', value: 3 }).hitPoints).toEqual({
      method: LevelUpHitPointsMethod.ROLLED_PHYSICAL,
      value: 3,
    });
  });

  it('writes the rule swap in swap_pick and the feature swaps in swap_feature_options, together', () => {
    const rule = { group: 'fighting_style', fromKey: 'a', toKey: 'b' };
    const feature = { group: 'feature:infusoes@mesa', fromKey: 'c', toKey: 'd' };
    const request = choicesRequest(
      { ...source, swapPick: rule, swapFeatureOptions: [feature] },
      null,
    );
    expect(request.swapPick).toEqual(rule);
    expect(request.swapFeatureOptions).toEqual([feature]);
  });

  it('leaves both swap fields out when nothing is swapped', () => {
    const request = choicesRequest({ ...source, swapFeatureOptions: [] }, null);
    expect('swapPick' in request).toBe(false);
    expect('swapFeatureOptions' in request).toBe(false);
  });
});
