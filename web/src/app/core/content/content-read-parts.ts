import type {
  TableEffect,
  TableFeature,
  TableRace,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import type { Ability } from '../../../gen/meurpg/rules/v1/rules_pb';
import { tight } from '../format/text';
import { feetToMeters, formatMeters } from '../units';
import type { CatalogAbility } from './catalog';
import { ABILITY_FIELDS, type Bonuses, noBonuses } from './feature-draft';
import { summonLine } from './summon-names';

/**
 * What a player reads of an entry (MR-025, question 83: "every option that is on, in full, with the numbers and the
 * effects"), and what the master reads where the app has no editor yet (classes and subclasses, 10.12). Pure: the page
 * hands over the entry and a way to name a key, and gets rows and sections of plain Portuguese back. Nothing here changes
 * what the server sent; a field the entry does not carry is not drawn. The pieces every kind shares live here.
 */

export interface ReadRow {
  readonly label: string;
  readonly value: string;
}

export interface ReadItem {
  readonly title: string;
  readonly text: string;
}

export interface ReadSection {
  /** Empty for a card with no heading of its own (then `label` names it for a screen reader). */
  readonly title: string;
  readonly label?: string;
  readonly items: readonly ReadItem[];
  /** A small line under the items. */
  readonly note?: string;
}

export interface EntryRead {
  readonly rows: readonly ReadRow[];
  /** A small line under the rows (what only the master reads). */
  readonly rowsNote?: string;
  readonly sections: readonly ReadSection[];
  /** Text of the entry itself (a spell's description, a subclass's). */
  readonly text: readonly string[];
}

const RECHARGE_WORDS: Readonly<Record<string, string>> = {
  short_rest: 'voltam num descanso curto',
  long_rest: 'voltam num descanso longo',
  dawn: 'voltam ao amanhecer',
  none: 'não voltam sozinhos',
  initiative: 'um uso volta ao rolar iniciativa, se não restar nenhum',
  restore_one_on_short_rest: 'um descanso curto devolve um uso e um longo devolve todos',
  restore_one_on_long_rest: 'um descanso longo devolve um uso',
};

const SENSE_WORDS: Readonly<Record<string, string>> = {
  darkvision: 'Visão no escuro',
  blindsight: 'Visão às cegas',
  tremorsense: 'Sentido sísmico',
  truesight: 'Visão verdadeira',
  'devils-sight': 'Visão do diabo',
};

export type NameOf = (key: string) => string;

/** A sentence ends with a stop: "Enxergam longe" becomes "Enxergam longe." (a text that already ends with one is left). */
export function ensureStop(text: string): string {
  const t = text.trim();
  return t === '' || /[.!?…:;]$/.test(t) ? t : `${t}.`;
}

/** One effect in a line, for the ones the player can read without the master's menu; '' for the others (their text carries them). */
export function effectLine(e: TableEffect, nameOf: NameOf): string {
  switch (e.type) {
    case 'proficiency': {
      const what =
        e.proficiency.startsWith('skill:') && e.proficiency !== 'skill:*'
          ? nameOf(e.proficiency)
          : '';
      return what ? `Proficiência em ${what}` : '';
    }
    case 'sense':
      return e.rangeFt > 0
        ? tight(`${SENSE_WORDS[e.sense] ?? e.sense} ${formatMeters(feetToMeters(e.rangeFt))}`)
        : '';
    case 'resource':
      // Only a plain number says well in a line; a formula (prof()) is left to the feature's own text.
      return /^\d+$/.test(e.max)
        ? `${e.max} ${e.max === '1' ? 'uso' : 'usos'}, ${RECHARGE_WORDS[e.recharge] ?? ''}`.replace(
            /, $/,
            '',
          )
        : '';
    case 'extra_attack':
      return e.count > 0 ? `Ataque extra: ${e.count} ataques` : '';
    default:
      return '';
  }
}

/** A feature as the player reads it: its text, and the effects it has that a line can say. */
export function featureItem(f: TableFeature, nameOf: NameOf, prefix = ''): ReadItem {
  const said = f.descPt.join(' ').toLowerCase();
  // An effect's line is added only when the trait's own text does not already say it.
  const lines = f.effects
    .map((e) => effectLine(e, nameOf))
    .filter((l) => l !== '' && !said.includes(l.toLowerCase()));
  const text = [...f.descPt, ...lines, summonLine(f.summon, nameOf)]
    .filter((l) => l !== '')
    .map(ensureStop)
    .join(' ');
  return { title: `${prefix}${f.namePt}`, text };
}

export function bonusesOf(b: TableRace['abilityBonuses']): Bonuses {
  const out = noBonuses();
  for (const a of ABILITY_FIELDS) {
    out[a] = b?.[a] ?? 0;
  }
  return out;
}

/** The abilities with their names, from the catalog (`ability:strength`...). */
export function abilitiesOf(nameOf: NameOf): CatalogAbility[] {
  return ABILITY_FIELDS.map((field, i) => ({
    field,
    ability: (i + 1) as Ability,
    name: nameOf(`ability:${field}`),
  }));
}

export function metres(ft: number): string {
  return formatMeters(feetToMeters(ft));
}

export function names(keys: readonly string[], nameOf: NameOf): string {
  return keys.map(nameOf).join(', ');
}
