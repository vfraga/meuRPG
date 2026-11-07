import type { MessageInitShape } from '@bufbuild/protobuf';

import {
  type TableOption,
  type TableOptionSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import {
  type EffectMenuVm,
  draftToEffect,
  effectToDraft,
  unreadableNumberPaths,
} from './effect-draft';
import { type FeatureDraft, emptyFeature } from './feature-draft';
import { paragraphs } from './spell-draft';

/**
 * The table option form (MR-025): an option the master adds to one of the SRD's option sets (an invocation, a Metamagic option, a
 * fighting style, a Pact Boon...). Like a feat it is a name, a text and effects (kept in `feature`, with no key), plus the set it
 * joins, chosen once, and what it asks of a character. The level is kept as typed so a half-typed number is never rewritten;
 * whether a character qualifies is the server's, which lists the option blocked with what it lacks.
 */

/** The set whose options join the option list of a class or subclass feature of the table. */
export const TABLE_FEATURE_SET = 'table_feature';

export type OptionInit = MessageInitShape<typeof TableOptionSchema>;

export interface OptionDraft {
  feature: FeatureDraft;
  /** The set's word ("invocation"); empty until the master picks one. It never changes after the first save. */
  set: string;
  /** The key of the table feature whose option list the option joins; only the set "table_feature" has one. Kept as it came so an edit does not drop it. */
  offeredBy: string;
  /** A class and a level in it, both or neither. */
  classKey: string;
  classLevel: string;
  /** Options (the SRD's or the table's) the character must have picked. */
  requiresOptions: string[];
  /** A Pact Boon the character must have. */
  requiresPact: string;
  /** A spell the character must know. */
  requiresSpell: string;
}

export function emptyOption(set = ''): OptionDraft {
  return {
    feature: emptyFeature(),
    set,
    offeredBy: '',
    classKey: '',
    classLevel: '',
    requiresOptions: [],
    requiresPact: '',
    requiresSpell: '',
  };
}

export function optionToDraft(o: TableOption): OptionDraft {
  const p = o.prerequisite;
  return {
    feature: {
      ...emptyFeature(),
      name: o.namePt,
      text: o.descPt.join('\n\n'),
      effects: o.effects.map(effectToDraft),
    },
    set: o.set,
    offeredBy: o.offeredBy,
    classKey: p?.classLevel?.classKey ?? '',
    classLevel: p?.classLevel && p.classLevel.level > 0 ? String(p.classLevel.level) : '',
    requiresOptions: [...(p?.requiresOptionKeys ?? [])],
    requiresPact: p?.requiresPact ?? '',
    requiresSpell: p?.requiresSpellKey ?? '',
  };
}

export function draftToOption(d: OptionDraft, menu: EffectMenuVm): OptionInit {
  const level = d.classLevel.trim();
  const classLevel =
    d.classKey !== '' || level !== ''
      ? { classKey: d.classKey, level: level === '' ? 0 : Number(level) }
      : undefined;
  return {
    namePt: d.feature.name.trim(),
    descPt: paragraphs(d.feature.text),
    set: d.set,
    offeredBy: d.set === TABLE_FEATURE_SET ? d.offeredBy : '',
    prerequisite: {
      ...(classLevel ? { classLevel } : {}),
      requiresOptionKeys: [...d.requiresOptions],
      requiresPact: d.requiresPact,
      requiresSpellKey: d.requiresSpell,
    },
    effects: d.feature.effects.map((e) => draftToEffect(e, menu)),
  };
}

/** The paths of the inputs the option editor draws (a violation at another path lands on the nearest one above). */
export function optionPaths(d: OptionDraft, menu: EffectMenuVm): string[] {
  const p = 'table_option';
  const out = [
    `${p}.name_pt`,
    `${p}.desc_pt`,
    `${p}.set`,
    ...(d.set === TABLE_FEATURE_SET ? [OFFERED_BY_PATH] : []),
    `${p}.prerequisite`,
    `${p}.prerequisite.class_level`,
    `${p}.prerequisite.class_level.class_key`,
    `${p}.prerequisite.class_level.level`,
    `${p}.prerequisite.requires_option_keys`,
    `${p}.prerequisite.requires_pact`,
    `${p}.prerequisite.requires_spell_key`,
    `${p}.effects`,
  ];
  d.feature.effects.forEach((e, k) => {
    const at = `${p}.effects[${k}]`;
    out.push(at, `${at}.type`);
    for (const field of menu.fieldsOf(e.type)) {
      out.push(`${at}.${field.name}`);
    }
  });
  return out;
}

/** The SRD features that offer each set, by the set's word (the server's `optionSetDefs`): the options of these features are the
 * SRD's options of the set, and what a table option may require. */
export const OFFERERS: Readonly<Record<string, readonly string[]>> = {
  invocation: ['feature:eldritch-invocations'],
  pact_boon: ['feature:pact-boon'],
  metamagic: ['feature:metamagic-1', 'feature:metamagic-2', 'feature:metamagic-3'],
  fighting_style: [
    'feature:fighter-fighting-style',
    'feature:additional-fighting-style',
    'feature:paladin-fighting-style',
    'feature:ranger-fighting-style',
  ],
  hunters_prey: ['feature:hunters-prey'],
  defensive_tactics: ['feature:defensive-tactics'],
  multiattack: ['feature:multiattack'],
  superior_hunters_defense: ['feature:superior-hunters-defense'],
};

/** The path of the input that asks for the offering feature. */
export const OFFERED_BY_PATH = 'table_option.offered_by';

/** The path of the note that stands in for that input when nothing can be chosen (its select is disabled). */
export const OFFERED_BY_HELP_PATH = 'table_option.offered_by.help';

/** The set asks for an offering feature and none is chosen. */
export function missingOffering(d: OptionDraft): boolean {
  return d.set === TABLE_FEATURE_SET && d.offeredBy === '';
}

/** The paths of the option's number fields whose text is no whole number. */
export function unreadableOptionPaths(d: OptionDraft, menu: EffectMenuVm): string[] {
  return unreadableNumberPaths('table_option.effects', d.feature.effects, menu);
}
