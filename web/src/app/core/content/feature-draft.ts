import type { MessageInitShape } from '@bufbuild/protobuf';

import {
  type TableBackground,
  TableBackgroundSchema,
  type TableFeature,
  TableFeatureSchema,
  type TableRace,
  TableRaceSchema,
  type TableSubrace,
  TableSubraceSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import type { CatalogAbility } from './catalog';
import {
  type EffectDraft,
  type EffectMenuVm,
  draftToEffect,
  effectToDraft,
  newRowId,
  strictRangeFeet,
  rangeMeters,
  unreadableNumberPaths,
} from './effect-draft';
import { paragraphs } from './spell-draft';

/**
 * The race, subrace and background forms (MR-025, RN-23, E10-01 states 6 and 7), and the feature they share with the class
 * editors of 10.12: a name, a text and effects from the server's menu. The server makes the feature keys; a feature the
 * editor read keeps its key when it goes back, so the sheets that chose on it keep their choice.
 */

export type FeatureInit = MessageInitShape<typeof TableFeatureSchema>;
export type RaceInit = MessageInitShape<typeof TableRaceSchema>;
export type SubraceInit = MessageInitShape<typeof TableSubraceSchema>;
export type BackgroundInit = MessageInitShape<typeof TableBackgroundSchema>;

export interface FeatureDraft {
  /** A stable id the editor tracks the row by (never sent). */
  id: string;
  /** Empty for a new feature. */
  key: string;
  name: string;
  /** Paragraphs, blank line between them. */
  text: string;
  effects: EffectDraft[];
  /**
   * The feature's option list ("Opções para escolher"): options the player picks, each with a name, a text and effects. Only a
   * class or subclass feature has them; an option has none of its own. Their keys are made by the server and sent back on
   * every save, so a pick keeps its option.
   */
  options: FeatureDraft[];
}

export function emptyFeature(): FeatureDraft {
  return { id: newRowId(), key: '', name: '', text: '', effects: [], options: [] };
}

export function featureToDraft(f: TableFeature): FeatureDraft {
  const effects = f.effects.map((e) => {
    const draft = effectToDraft(e);
    // A note that only repeats the trait's own text is shown folded away (it is sent again as the first paragraph when empty).
    return draft.type === 'note' &&
      draft.textPt.trim() !== '' &&
      draft.textPt.trim() === f.descPt[0]?.trim()
      ? { ...draft, textPt: '' }
      : draft;
  });
  return {
    id: newRowId(),
    key: f.key,
    name: f.namePt,
    text: f.descPt.join('\n\n'),
    effects,
    options: f.options.map(featureToDraft),
  };
}

/** A feature with a name, or nothing: an unnamed row is not sent (the editor marks it before). */
export function draftToFeature(f: FeatureDraft, menu: EffectMenuVm): FeatureInit {
  const text = paragraphs(f.text);
  return {
    key: f.key,
    namePt: f.name.trim(),
    descPt: text,
    // An unnamed option is not sent, like an unnamed feature; a feature with no options sends none.
    ...optionsOf(f, menu),
    effects: f.effects.map((e) => {
      const effect = draftToEffect(e, menu);
      // A note's own text is the reminder the sheet shows, and it says what the trait says: left empty (folded under
      // "Mais opções"), it is the trait's first paragraph. Granting spells needs none.
      if (e.type === 'note' && !effect.textPt && (effect.spells?.length ?? 0) === 0 && text[0]) {
        effect.textPt = text[0];
      }
      return effect;
    }),
  };
}

/** The `AbilityScores` fields in the sheet's order. The names the screen shows come from the catalog (`CatalogVm.abilities`). */
export const ABILITY_FIELDS = [
  'strength',
  'dexterity',
  'constitution',
  'intelligence',
  'wisdom',
  'charisma',
] as const;

export type AbilityField = (typeof ABILITY_FIELDS)[number];
export type Bonuses = Record<AbilityField, number>;

export function noBonuses(): Bonuses {
  return { strength: 0, dexterity: 0, constitution: 0, intelligence: 0, wisdom: 0, charisma: 0 };
}

/** The bonuses of "Sabedoria +2, Destreza +1", in the sheet's order; "Nenhum" when there are none. */
export function bonusText(b: Bonuses, abilities: readonly CatalogAbility[]): string {
  const parts = abilities
    .filter((a) => b[a.field] !== 0)
    .map((a) => `${a.name} ${b[a.field] > 0 ? '+' : '−'}${Math.abs(b[a.field])}`);
  return parts.length > 0 ? parts.join(', ') : 'Nenhum';
}

/** What a bonus can be: the server's limit (`rules.maxRaceBonus`, 4 in either direction). */
export const BONUS_MIN = -4;
export const BONUS_MAX = 4;

export interface RaceDraft {
  name: string;
  size: string;
  speedM: string;
  darkvisionM: string;
  bonuses: Bonuses;
  /** "O jogador escolhe onde pôr os bônus": on when the race has bonuses to place. */
  choosing: boolean;
  /** "2, 1": the amounts the player places, each on a different ability; "2, 1 / 1, 1, 1" offers several sets, of which the player picks one. */
  choice: string;
  /** The sizes the player picks one from ("Small", "Medium"); empty is the race's own size. */
  sizes: string[];
  languages: string[];
  languageChoices: number;
  traits: FeatureDraft[];
}

export function emptyRace(): RaceDraft {
  return {
    name: '',
    size: 'Medium',
    speedM: '9',
    darkvisionM: '',
    bonuses: noBonuses(),
    choosing: false,
    choice: '2, 1',
    sizes: [],
    languages: [],
    languageChoices: 0,
    traits: [],
  };
}

export const SIZE_OPTIONS: readonly { value: string; label: string }[] = [
  { value: 'Tiny', label: 'Miúdo' },
  { value: 'Small', label: 'Pequeno' },
  { value: 'Medium', label: 'Médio' },
  { value: 'Large', label: 'Grande' },
];

function bonusesOf(b: TableRace['abilityBonuses']): Bonuses {
  return {
    strength: b?.strength ?? 0,
    dexterity: b?.dexterity ?? 0,
    constitution: b?.constitution ?? 0,
    intelligence: b?.intelligence ?? 0,
    wisdom: b?.wisdom ?? 0,
    charisma: b?.charisma ?? 0,
  };
}

export function raceToDraft(r: TableRace): RaceDraft {
  return {
    name: r.namePt,
    size: r.size || 'Medium',
    speedM: rangeMeters(r.speedFt),
    darkvisionM: rangeMeters(r.darkvisionFt),
    bonuses: bonusesOf(r.abilityBonuses),
    choosing: r.choiceBonuses.length > 0 || r.choiceBonusSets.length > 0,
    choice:
      r.choiceBonusSets.length > 0
        ? r.choiceBonusSets.map((s) => s.bonuses.join(', ')).join(' / ')
        : r.choiceBonuses.length > 0
          ? r.choiceBonuses.join(', ')
          : '2, 1',
    sizes: [...r.sizes],
    languages: [...r.languages],
    languageChoices: r.languageChoices,
    traits: r.traits.map(featureToDraft),
  };
}

/** "2, 1" → [2, 1]; what does not read as a number is left out (the server refuses an empty list for a choosing race). */
export function choiceAmounts(text: string): number[] {
  return text
    .split(/[,\s]+/)
    .map((t) => Number(t.replace('+', '')))
    .filter((n) => Number.isInteger(n) && n !== 0);
}

/** "2, 1 / 1, 1, 1" → [[2, 1], [1, 1, 1]]: the sets of bonuses the text offers, in the order written. A text with one set is the
 * race's single `choiceBonuses`; with more, its `choiceBonusSets`. */
export function choiceSets(text: string): number[][] {
  return text.split('/').map(choiceAmounts);
}

export function draftToRace(d: RaceDraft, menu: EffectMenuVm): RaceInit {
  const sets = d.choosing ? choiceSets(d.choice) : [];
  return {
    namePt: d.name.trim(),
    size: d.size,
    speedFt: strictRangeFeet(d.speedM),
    abilityBonuses: { ...d.bonuses },
    choiceBonuses: sets.length === 1 ? sets[0] : [],
    ...(sets.length > 1 ? { choiceBonusSets: sets.map((bonuses) => ({ bonuses })) } : {}),
    ...(d.sizes.length > 0 ? { sizes: [...d.sizes] } : {}),
    darkvisionFt: strictRangeFeet(d.darkvisionM),
    languages: [...d.languages],
    languageChoices: d.languageChoices,
    traits: d.traits.map((t) => draftToFeature(t, menu)),
  };
}

export interface SubraceDraft {
  name: string;
  raceKey: string;
  bonuses: Bonuses;
  traits: FeatureDraft[];
}

export function subraceToDraft(s: TableSubrace): SubraceDraft {
  return {
    name: s.namePt,
    raceKey: s.raceKey,
    bonuses: bonusesOf(s.abilityBonuses),
    traits: s.traits.map(featureToDraft),
  };
}

export function draftToSubrace(d: SubraceDraft, menu: EffectMenuVm): SubraceInit {
  return {
    namePt: d.name.trim(),
    raceKey: d.raceKey,
    abilityBonuses: { ...d.bonuses },
    traits: d.traits.map((t) => draftToFeature(t, menu)),
  };
}

export interface BackgroundDraft {
  name: string;
  skills: string[];
  tools: string[];
  languageChoices: number;
  equipment: string;
  feature: FeatureDraft;
}

export function emptyBackground(): BackgroundDraft {
  return {
    name: '',
    skills: ['', ''],
    tools: [],
    languageChoices: 0,
    equipment: '',
    feature: emptyFeature(),
  };
}

export function backgroundToDraft(b: TableBackground): BackgroundDraft {
  return {
    name: b.namePt,
    skills: [b.skills[0] ?? '', b.skills[1] ?? ''],
    tools: [...b.tools],
    languageChoices: b.languageChoices,
    equipment: b.equipmentPt,
    feature: b.feature ? featureToDraft(b.feature) : emptyFeature(),
  };
}

export function draftToBackground(d: BackgroundDraft, menu: EffectMenuVm): BackgroundInit {
  return {
    namePt: d.name.trim(),
    skills: d.skills.filter((s) => s !== ''),
    tools: [...d.tools],
    languageChoices: d.languageChoices,
    equipmentPt: d.equipment.trim(),
    feature: draftToFeature(d.feature, menu),
  };
}

/** The paths of the inputs one feature draws, at `base` ("table_class.levels[4].features[0]"): the feature, its name and text and, for
 * each effect, its type and the fields the menu says that type reads. */
export function featureOwnPaths(base: string, f: FeatureDraft, menu: EffectMenuVm): string[] {
  const out: string[] = [base, `${base}.name_pt`, `${base}.desc_pt`];
  f.options.forEach((o, k) => out.push(...featureOwnPaths(`${base}.options[${k}]`, o, menu)));
  f.effects.forEach((e, k) => {
    const at = `${base}.effects[${k}]`;
    out.push(at, `${at}.type`);
    for (const field of menu.fieldsOf(e.type)) {
      out.push(`${at}.${field.name}`);
    }
  });
  return out;
}

/** The paths of the number fields of one feature (and of its named options: an unnamed one is not sent) whose text is no whole number, at `base`. */
export function unreadableFeaturePaths(
  base: string,
  f: FeatureDraft,
  menu: EffectMenuVm,
): string[] {
  return [
    ...unreadableNumberPaths(`${base}.effects`, f.effects, menu),
    ...f.options.flatMap((o, k) =>
      o.name.trim() === '' ? [] : unreadableFeaturePaths(`${base}.options[${k}]`, o, menu),
    ),
  ];
}

/** The paths of the inputs a list of features draws. A violation at a path with no input lands on the nearest one above (`inputFor`). */
export function featurePaths(
  prefix: string,
  features: readonly FeatureDraft[],
  menu: EffectMenuVm,
): string[] {
  return features.flatMap((f, i) => featureOwnPaths(`${prefix}[${i}]`, f, menu));
}

/** The feature's options as the save sends them: only the named ones, and no field at all when there are none. */
function optionsOf(f: FeatureDraft, menu: EffectMenuVm): { options?: FeatureInit[] } {
  const options = f.options.filter((o) => o.name.trim() !== '').map((o) => draftToFeature(o, menu));
  return options.length > 0 ? { options } : {};
}
