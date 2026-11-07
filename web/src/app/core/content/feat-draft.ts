import type { MessageInitShape } from '@bufbuild/protobuf';

import type { AbilityScores } from '../../../gen/meurpg/rules/v1/rules_pb';
import {
  type TableFeat,
  type TableFeatSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import {
  type EffectMenuVm,
  draftToEffect,
  effectToDraft,
  unreadableNumberPaths,
} from './effect-draft';
import {
  type FeatureDraft,
  ABILITY_FIELDS,
  type AbilityField,
  emptyFeature,
  featureOwnPaths,
  featureToDraft,
  optionsInit,
  unreadableFeaturePaths,
} from './feature-draft';
import { paragraphs } from './spell-draft';

/**
 * The feat form (MR-025): a name, a text, what the feat asks of a character and its effects, which come from the same
 * menu as a feature's (plus `ability_increase`, only a feat's). The feature editor holds the name, the text and the
 * effects (`feature`, with no key: a feat has none); the prerequisite fields are kept as text so a half-typed number
 * is never rewritten. Whether a character meets the prerequisite is the server's.
 */

/** The most options a feat has (`MaxFeatureOptions`); the server refuses more. */
export const MAX_FEAT_OPTIONS = 60;

export type FeatInit = MessageInitShape<typeof TableFeatSchema>;

/** The score asked of each ability, as typed ("13"); empty asks nothing. */
export type Scores = Record<AbilityField, string>;

export function noScores(): Scores {
  return {
    strength: '',
    dexterity: '',
    constitution: '',
    intelligence: '',
    wisdom: '',
    charisma: '',
  };
}

export interface FeatDraft {
  feature: FeatureDraft;
  minimums: Scores;
  anyOf: Scores;
  proficiencyKey: string;
  spellcasting: boolean;
  raceKey: string;
  /** Any of several races or sub-races; empty when the single `raceKey` is set (the server refuses both). */
  races: string[];
  /** "Tiny", "Small", "Medium" or "Large", as the server writes it; empty is any size. */
  size: string;
  /** The one feature, trait or feat the character must not have; empty asks nothing. */
  lacksFeatureKey: string;
  /** As typed; empty asks nothing. */
  level: string;
  /**
   * The feat's own option list, in order. An option the server made keeps its key; a new one has none and the server
   * makes it. An unnamed option is not sent.
   */
  options: FeatureDraft[];
}

export function emptyFeat(): FeatDraft {
  return {
    feature: emptyFeature(),
    minimums: noScores(),
    anyOf: noScores(),
    proficiencyKey: '',
    spellcasting: false,
    raceKey: '',
    races: [],
    size: '',
    lacksFeatureKey: '',
    level: '',
    options: [],
  };
}

function scoresOf(s: AbilityScores | undefined): Scores {
  const out = noScores();
  for (const a of ABILITY_FIELDS) {
    const v = s?.[a] ?? 0;
    out[a] = v > 0 ? String(v) : '';
  }
  return out;
}

/** The race list, the size and the feature to lack of a saved prerequisite. */
function whoOf(
  p: TableFeat['prerequisite'],
): Pick<FeatDraft, 'races' | 'size' | 'lacksFeatureKey'> {
  return {
    races: [...(p?.races ?? [])],
    size: p?.size ?? '',
    lacksFeatureKey: p?.lacksFeatureKey ?? '',
  };
}

export function featToDraft(f: TableFeat): FeatDraft {
  const p = f.prerequisite;
  return {
    feature: {
      ...emptyFeature(),
      name: f.namePt,
      text: f.descPt.join('\n\n'),
      effects: f.effects.map(effectToDraft),
    },
    minimums: scoresOf(p?.minimums),
    anyOf: scoresOf(p?.anyOf),
    proficiencyKey: p?.proficiencyKey ?? '',
    spellcasting: p?.spellcasting ?? false,
    raceKey: p?.raceKey ?? '',
    ...whoOf(p),
    level: p && p.level > 0 ? String(p.level) : '',
    options: f.options.map(featureToDraft),
  };
}

/** The abilities with a score typed; an unreadable text is NaN, which the server refuses at that field. */
function scoresInit(s: Scores): Partial<Record<AbilityField, number>> {
  const out: Partial<Record<AbilityField, number>> = {};
  for (const a of ABILITY_FIELDS) {
    const t = s[a].trim();
    if (t !== '') {
      out[a] = Number(t);
    }
  }
  return out;
}

export function draftToFeat(d: FeatDraft, menu: EffectMenuVm): FeatInit {
  const minimums = scoresInit(d.minimums);
  const anyOf = scoresInit(d.anyOf);
  const level = d.level.trim();
  return {
    namePt: d.feature.name.trim(),
    descPt: paragraphs(d.feature.text),
    prerequisite: {
      ...(Object.keys(minimums).length > 0 ? { minimums } : {}),
      ...(Object.keys(anyOf).length > 0 ? { anyOf } : {}),
      proficiencyKey: d.proficiencyKey,
      spellcasting: d.spellcasting,
      raceKey: d.raceKey,
      // A feat with raceKey sends no list (the server refuses both).
      races: d.raceKey ? [] : d.races,
      size: d.size,
      lacksFeatureKey: d.lacksFeatureKey,
      level: level === '' ? 0 : Number(level),
    },
    effects: d.feature.effects.map((e) => draftToEffect(e, menu)),
    ...optionsInit(d.options, menu),
  };
}

/** The paths of the inputs the feat editor draws (a violation at another path lands on the nearest one above). */
export function featPaths(d: FeatDraft, menu: EffectMenuVm): string[] {
  const p = 'table_feat';
  const out = [
    `${p}.name_pt`,
    `${p}.desc_pt`,
    `${p}.prerequisite`,
    `${p}.prerequisite.minimums`,
    `${p}.prerequisite.any_of`,
    `${p}.prerequisite.proficiency_key`,
    `${p}.prerequisite.race_key`,
    `${p}.prerequisite.races`,
    `${p}.prerequisite.size`,
    `${p}.prerequisite.lacks_feature_key`,
    `${p}.prerequisite.level`,
    `${p}.prerequisite.spellcasting`,
    `${p}.effects`,
    `${p}.options`,
  ];
  for (const a of ABILITY_FIELDS) {
    out.push(`${p}.prerequisite.minimums.${a}`, `${p}.prerequisite.any_of.${a}`);
  }
  d.races.forEach((_, i) => out.push(`${p}.prerequisite.races[${i}]`));
  d.feature.effects.forEach((e, k) => {
    const at = `${p}.effects[${k}]`;
    out.push(at, `${at}.type`);
    for (const field of menu.fieldsOf(e.type)) {
      out.push(`${at}.${field.name}`);
    }
  });
  d.options.forEach((o, k) => out.push(...featureOwnPaths(`${p}.options[${k}]`, o, menu)));
  return out;
}

/** The paths of the feat's number fields whose text is no whole number. */
export function unreadableFeatPaths(d: FeatDraft, menu: EffectMenuVm): string[] {
  return [
    ...unreadableNumberPaths('table_feat.effects', d.feature.effects, menu),
    ...d.options.flatMap((o, k) =>
      o.name.trim() === '' ? [] : unreadableFeaturePaths(`table_feat.options[${k}]`, o, menu),
    ),
  ];
}
