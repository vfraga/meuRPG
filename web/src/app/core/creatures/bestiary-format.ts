import type { Creature, CreatureSummary } from '../../../gen/meurpg/rules/v1/rules_pb';
import { joinDots, tieNumbers } from '../format/text';

/** "fera" → "Fera": the server sends the type in lower case, the list writes it as a label. */
export function capitalized(word: string): string {
  return word === '' ? word : word.charAt(0).toLocaleUpperCase('pt-BR') + word.slice(1);
}

/** "Fera · Médio": the type and the size, as a row of the bestiary says them. */
export function typeAndSize(s: Pick<CreatureSummary, 'typePt' | 'sizePt'>): string {
  return joinDots([capitalized(s.typePt), s.sizePt].filter((p) => p !== ''));
}

// The type names the server sends (creatureTypeNamePT in backend/internal/rules/creatures.go).
const FEMININE_TYPES = new Set(['fera', 'monstruosidade', 'aberração', 'planta', 'fada', 'gosma']);
const MASCULINE_TYPES = new Set([
  'morto-vivo',
  'humanoide',
  'dragão',
  'gigante',
  'construto',
  'celestial',
  'ínfero',
  'elemental',
]);
const FEMININE_SIZES: Readonly<Record<string, string>> = {
  miúdo: 'miúda',
  pequeno: 'pequena',
  médio: 'média',
  grande: 'grande',
  enorme: 'enorme',
  imenso: 'imensa',
};

/** "fera média", "morto-vivo médio": the type and the size in running text, the size agreeing with the type's gender.
 * A type or a size this build does not know falls back to the joined form of a list row ("Fera · Médio"). */
export function typeAndSizeInWords(s: Pick<CreatureSummary, 'typePt' | 'sizePt'>): string {
  const type = s.typePt.toLocaleLowerCase('pt-BR');
  const size = s.sizePt.toLocaleLowerCase('pt-BR');
  const feminine = FEMININE_TYPES.has(type);
  if (size === '' || (!feminine && !MASCULINE_TYPES.has(type)) || !(size in FEMININE_SIZES)) {
    return typeAndSize(s);
  }
  return `${type} ${feminine ? FEMININE_SIZES[size] : size}`;
}

/** "CA 13 · PV 11": the armor class and the average hit points of a row. */
export function acAndHp(s: Pick<CreatureSummary, 'armorClass' | 'hitPoints'>): string {
  return joinDots([tieNumbers(`CA ${s.armorClass}`), tieNumbers(`PV ${s.hitPoints}`)]);
}

const ALIGNMENT_PT: Record<string, string> = {
  unaligned: 'sem alinhamento',
  'any alignment': 'qualquer alinhamento',
  'any non-good alignment': 'qualquer alinhamento não bom',
  'any non-lawful alignment': 'qualquer alinhamento não leal',
  'any chaotic alignment': 'qualquer alinhamento caótico',
  'any evil alignment': 'qualquer alinhamento mau',
  'lawful good': 'leal e bom',
  'lawful neutral': 'leal e neutro',
  'lawful evil': 'leal e mau',
  'neutral good': 'neutro e bom',
  neutral: 'neutro',
  'neutral evil': 'neutro e mau',
  'chaotic good': 'caótico e bom',
  'chaotic neutral': 'caótico e neutro',
  'chaotic evil': 'caótico e mau',
};

/**
 * The SRD's alignment text ("chaotic evil") in Portuguese. The ones the table does not know
 * ("neutral good (50%) or neutral evil (50%)") stay the book's English, and `english` says so,
 * so the page marks them `lang="en"` (the same rule as the rest of the SRD's text).
 */
export function alignmentPt(alignment: string): {
  readonly text: string;
  readonly english: boolean;
} {
  const known = ALIGNMENT_PT[alignment.trim().toLowerCase()];
  return known
    ? { text: known, english: false }
    : { text: alignment.trim(), english: alignment.trim() !== '' };
}

/** The route segment of a creature ("monster:ogre" → "ogre") and back. */
export function creatureSlug(key: string): string {
  return key.replace(/^monster:/, '');
}
export function creatureKeyOf(slug: string): string {
  return `monster:${slug}`;
}

/** The speed an NPC made from the creature gets, in feet: the walking one, or the best other (the server's rule, `CreateNpcFromCreature`). */
export function npcSpeedFt(c: Creature): number {
  return c.speedWalkFt > 0
    ? c.speedWalkFt
    : Math.max(c.speedFlyFt, c.speedSwimFt, c.speedClimbFt, c.speedBurrowFt);
}

/** "Cimitarra e Adaga", "A, B e C". */
export function listWithE(items: readonly string[]): string {
  return items.length < 2
    ? (items[0] ?? '')
    : `${items.slice(0, -1).join(', ')} e ${items[items.length - 1]}`;
}
