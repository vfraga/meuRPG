import {
  LevelUpHitPointsMethod,
  type LevelUpOptions,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { ABILITY_KEYS, type AbilityKey } from '../characters/characters.types';
import type { LevelUpChoicesInit } from './levelup-client';
import { type ChoiceDrafts, storedKeysOf, textsOf } from './levelup-choices';
import type { Rolled } from './levelup-draft-types';

const WIRE_ABILITY: Record<AbilityKey, string> = {
  str: 'strength',
  dex: 'dexterity',
  con: 'constitution',
  int: 'intelligence',
  wis: 'wisdom',
  cha: 'charisma',
};

/** What the request to the server is written from: the picks of the draft, as plain values. */
export interface RequestSource {
  readonly options: LevelUpOptions;
  readonly abilityIncrease: Partial<Record<AbilityKey, number>>;
  readonly featKey: string;
  readonly subclassKey: string;
  readonly cantrips: ReadonlySet<string>;
  readonly spells: ReadonlySet<string>;
  readonly prepared: ReadonlySet<string>;
  readonly features: ReadonlySet<string>;
  readonly skills: ReadonlySet<string>;
  readonly expertise: ReadonlySet<string>;
  readonly instrument: string;
  /** The swap of a pick at an Ability Score Improvement, once whole. */
  readonly swapPick?: LevelUpChoicesInit['swapPick'];
  /** The swaps of the options of table features, one for each group, once whole. */
  readonly swapFeatureOptions?: LevelUpChoicesInit['swapFeatureOptions'];
  readonly lateDrafts: ChoiceDrafts;
  readonly newDrafts: ChoiceDrafts;
}

/** The hit points as the server takes them: the die that was rolled, or the average. */
function hitPointsOf(hp: Rolled | null): LevelUpChoicesInit['hitPoints'] {
  if (!hp) {
    return { method: LevelUpHitPointsMethod.AVERAGE };
  }
  return {
    method:
      hp.kind === 'app'
        ? LevelUpHitPointsMethod.ROLLED_IN_APP
        : LevelUpHitPointsMethod.ROLLED_PHYSICAL,
    value: hp.value,
  };
}

/** The `LevelUpChoices` to send for the picks so far, with the die `hp` rolled (`null` for the average). */
export function choicesRequest(d: RequestSource, hp: Rolled | null): LevelUpChoicesInit {
  const o = d.options;
  return {
    classKey: o.classKey,
    abilityIncrease: Object.fromEntries(
      ABILITY_KEYS.filter((k) => d.abilityIncrease[k]).map((k) => [
        WIRE_ABILITY[k],
        d.abilityIncrease[k],
      ]),
    ),
    featKey: d.featKey,
    subclassKey: d.subclassKey,
    cantripKeys: [...d.cantrips],
    knownSpellKeys: [...d.spells],
    preparedSpellKeys: [...d.prepared],
    featureChoiceKeys: [...d.features, ...storedKeysOf(o.newChoices, d.newDrafts)],
    lateChoiceKeys: storedKeysOf(o.lateChoices, d.lateDrafts),
    featureChoiceText: {
      ...textsOf(o.lateChoices, d.lateDrafts),
      ...textsOf(o.newChoices, d.newDrafts),
    },
    skillProficiencyKeys: [...d.skills],
    expertiseSkillKeys: [...d.expertise],
    instrumentKey: d.instrument,
    ...(d.swapPick ? { swapPick: d.swapPick } : {}),
    ...(d.swapFeatureOptions?.length ? { swapFeatureOptions: d.swapFeatureOptions } : {}),
    hitPoints: hitPointsOf(hp),
  };
}
