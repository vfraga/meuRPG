import { AbstractControl } from '@angular/forms';

import { abilityLabel, formatModifier } from '../../core/characters/character-labels';
import { ABILITY_KEYS, AbilityKey } from '../../core/characters/characters.types';
import { AbilityScoresInput } from './character-editor.types';
import { upFirst } from '../../core/format/text';

/**
 * Display-only helpers for the character editor: labels, and the "what is
 * still wrong" summary a failed submit shows. None of this is a D&D rule —
 * the browser never computes one (every modifier, CA and PV comes from the
 * server); these only name and format what the person typed.
 */

/** The editor's steps, in order. `escolhas` only exists when the race or a class asks a choice, `magias` for a caster class. */
export type EditorStepKey =
  'basico' | 'atributos' | 'escolhas' | 'pericias' | 'magias' | 'equipamento';

export const EDITOR_STEP_LABELS: Record<EditorStepKey, string> = {
  basico: 'Básico',
  atributos: 'Habilidades',
  escolhas: 'Escolhas',
  pericias: 'Perícias',
  magias: 'Magias',
  equipamento: 'Equipamento',
};

/** The three-letter abbreviation the official sheet prints next to each
 * skill (For, Des, Con, Int, Sab, Car). */
const ABILITY_ABBREVIATIONS: Record<AbilityKey, string> = {
  str: 'For',
  dex: 'Des',
  con: 'Con',
  int: 'Int',
  wis: 'Sab',
  cha: 'Car',
};

export function abilityAbbreviation(key: AbilityKey): string {
  return ABILITY_ABBREVIATIONS[key];
}

/** "Constituição +1, Inteligência +2": the manual bonuses that are not zero,
 * in sheet order, so the collapsed "Bônus manuais" section still says what
 * it holds. Empty when every bonus is zero (or not a number yet). */
export function describeBonusesInUse(bonuses: Partial<AbilityScoresInput>): string {
  return ABILITY_KEYS.filter((key) => {
    const value = bonuses[key];
    return typeof value === 'number' && Number.isFinite(value) && value !== 0;
  })
    .map((key) => `${abilityLabel(key)} ${formatModifier(bonuses[key] as number)}`)
    .join(', ');
}

/** "Nenhuma perícia marcada", "1 perícia marcada", "3 perícias marcadas". */
export function countLabel(count: number, singular: string, plural: string, none: string): string {
  if (count === 0) {
    return none;
  }
  return count === 1 ? `1 ${singular}` : `${count} ${plural}`;
}

/** One validated control of a form, with the step it sits on (full sheet
 * only) and its label as the screen shows it. */
export interface EditorField {
  readonly path: string;
  readonly label: string;
  readonly step: EditorStepKey | null;
}

const ABILITY_FIELDS: readonly EditorField[] = ABILITY_KEYS.map((key) => ({
  path: `abilities.${key}`,
  label: abilityLabel(key),
  step: 'atributos' as const,
}));

const BONUS_FIELDS: readonly EditorField[] = ABILITY_KEYS.map((key) => ({
  path: `extraAbilityBonuses.${key}`,
  label: `bônus manual de ${abilityLabel(key)}`,
  step: 'atributos' as const,
}));

/** Every validated control of the full sheet, in the order the steps show
 * them. */
export const FULL_SHEET_FIELDS: readonly EditorField[] = [
  { path: 'name', label: 'Nome do personagem', step: 'basico' },
  { path: 'level', label: 'Nível', step: 'basico' },
  { path: 'experiencePoints', label: 'Pontos de experiência', step: 'basico' },
  { path: 'xpValue', label: 'XP ao derrotar', step: 'basico' },
  { path: 'className', label: 'Classe', step: 'basico' },
  { path: 'subclassName', label: 'Subclasse', step: 'basico' },
  { path: 'race', label: 'Raça', step: 'basico' },
  { path: 'subrace', label: 'Sub-raça', step: 'basico' },
  { path: 'background', label: 'Antecedente', step: 'basico' },
  { path: 'customBackgroundName', label: 'Nome do antecedente', step: 'basico' },
  ...ABILITY_FIELDS,
  ...BONUS_FIELDS,
  { path: 'equipmentText', label: 'Itens de equipamento', step: 'equipamento' },
  { path: 'languagesText', label: 'Idiomas', step: 'equipamento' },
  { path: 'toolProficienciesText', label: 'Proficiências em ferramentas', step: 'equipamento' },
  { path: 'customFeaturesText', label: 'Características personalizadas', step: 'equipamento' },
];

/** Not a control: "Rolar 4d6" or "Conjunto padrão" with a result still to
 * place. The page adds it to the invalid fields while that is so. */
export const UNPLACED_RESULTS_FIELD: EditorField = {
  path: 'abilities',
  label: 'coloque cada resultado numa habilidade',
  step: 'atributos',
};

/** The fields of `fields` whose control in `form` is invalid right now. */
export function invalidFields(
  form: AbstractControl,
  fields: readonly EditorField[],
): EditorField[] {
  return fields.filter((field) => form.get(field.path)?.invalid ?? false);
}

/**
 * The second sentence of the "fix these first" notice: the invalid fields'
 * labels, grouped by step on a full sheet ("Básico: Nome do personagem,
 * Raça. Habilidades: Força."), or just listed on the short form.
 */
export function describeInvalidFields(fields: readonly EditorField[]): string {
  const groups = new Map<EditorStepKey | null, string[]>();
  for (const field of fields) {
    const labels = groups.get(field.step) ?? [];
    labels.push(field.label);
    groups.set(field.step, labels);
  }
  return Array.from(groups.entries())
    .map(([step, labels]) => {
      const list = labels.join(', ');
      return step ? `${EDITOR_STEP_LABELS[step]}: ${list}.` : `${upFirst(list)}.`;
    })
    .join(' ');
}
