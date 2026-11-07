import type { MessageInitShape } from '@bufbuild/protobuf';

import {
  type TableClassFeature,
  type TableClassFeatureSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import {
  type EffectMenuVm,
  draftToEffect,
  effectToDraft,
  unreadableNumberPaths,
} from './effect-draft';
import { type FeatureDraft, emptyFeature } from './feature-draft';
import { paragraphs } from './spell-draft';
import { draftToSummon, summonBlockedPaths, summonPaths, summonToDraft } from './summon-draft';

/**
 * The table class feature form (MR-025): a feature the master adds to a class at a class level, which may take the place of one
 * the class already has. A name, a text and effects (kept in `feature`, with no key), plus the class, the level and the feature
 * it replaces. The level is kept as typed so a half-typed number is never rewritten; whether the class has the replaced feature
 * is the server's to say.
 */

export type ClassFeatureInit = MessageInitShape<typeof TableClassFeatureSchema>;

export interface ClassFeatureDraft {
  feature: FeatureDraft;
  /** The class that has it, SRD or table; empty until the master picks one. It never changes after the first save. */
  classKey: string;
  level: string;
  /** The key of the class feature it replaces; empty replaces nothing. */
  replaces: string;
}

export function emptyClassFeature(): ClassFeatureDraft {
  return { feature: emptyFeature(), classKey: '', level: '', replaces: '' };
}

export function classFeatureToDraft(f: TableClassFeature): ClassFeatureDraft {
  return {
    feature: {
      ...emptyFeature(),
      name: f.namePt,
      text: f.descPt.join('\n\n'),
      effects: f.effects.map(effectToDraft),
      summon: f.summon ? summonToDraft(f.summon) : null,
    },
    classKey: f.classKey,
    level: f.level > 0 ? String(f.level) : '',
    replaces: f.replacesFeatureKey,
  };
}

export function draftToClassFeature(d: ClassFeatureDraft, menu: EffectMenuVm): ClassFeatureInit {
  const level = d.level.trim();
  return {
    namePt: d.feature.name.trim(),
    descPt: paragraphs(d.feature.text),
    classKey: d.classKey,
    level: level === '' ? 0 : Number(level),
    replacesFeatureKey: d.replaces.trim(),
    effects: d.feature.effects.map((e) => draftToEffect(e, menu)),
    ...(d.feature.summon ? { summon: draftToSummon(d.feature.summon) } : {}),
  };
}

/** The paths of the inputs the class feature editor draws (a violation at another path lands on the nearest one above). */
export function classFeaturePaths(d: ClassFeatureDraft, menu: EffectMenuVm): string[] {
  const p = 'table_class_feature';
  const out = [
    `${p}.name_pt`,
    `${p}.desc_pt`,
    `${p}.class_key`,
    `${p}.level`,
    `${p}.replaces_feature_key`,
    `${p}.effects`,
  ];
  d.feature.effects.forEach((e, k) => {
    const at = `${p}.effects[${k}]`;
    out.push(at, `${at}.type`);
    for (const field of menu.fieldsOf(e.type)) {
      out.push(`${at}.${field.name}`);
    }
  });
  if (d.feature.summon) {
    out.push(...summonPaths(p));
  }
  return out;
}

/** The paths of the class feature's number fields whose text is no whole number. */
export function unreadableClassFeaturePaths(d: ClassFeatureDraft, menu: EffectMenuVm): string[] {
  return [
    ...unreadableNumberPaths('table_class_feature.effects', d.feature.effects, menu),
    ...summonBlockedPaths('table_class_feature', d.feature),
  ];
}
