import type { DiceRoll } from '../../../gen/meurpg/play/v1/combat_pb';

/**
 * How the combat screens write dice (timeline.md, shared decision 3): the
 * dice, the faces in parentheses, the modifier and the total
 * (`1d20 (13) + 6 = 19`); a typed physical roll without the dice
 * (`16 + 5 = 21`, with the note "dado físico" next to it). Pure functions,
 * tested without a DOM.
 */

/** " + 6", " − 1" or nothing for 0. */
export function modifierText(modifier: number): string {
  if (modifier === 0) {
    return '';
  }
  return ` ${modifier < 0 ? '−' : '+'} ${Math.abs(modifier)}`;
}

/** The dice an effect added to a roll (Bênção, Perdição), written after the modifier: " + 1d4 (3)", and what they add up to.
 * `named` writes the source before each die (" + Golpe Certeiro 1d8 (5)", " + Bênção 1d4 (3)"): the attack sheet's result
 * names every die, effects included. */
export function extraDiceText(
  roll: Pick<DiceRoll, 'extraDice'>,
  named = false,
): { text: string; sum: number } {
  const dice = (roll.extraDice ?? []).filter((d) => d.face > 0);
  return {
    text: dice
      .map(
        (d) =>
          ` ${d.sign < 0 ? '−' : '+'} ${named && d.sourceNamePt ? `${d.sourceNamePt} ` : ''}1d${d.faces} (${d.face})`,
      )
      .join(''),
    sum: dice.reduce((sum, d) => sum + d.sign * d.face, 0),
  };
}

/** A roll as the formula the table reads, the total included; `named` is as in `extraDiceText`. */
export function rollFormula(roll: DiceRoll, named = false): string {
  // A d20 a feature raised says so (Talento Confiável): what came up, what it counted as, and the feature.
  const treated = treatedFormula(roll);
  if (treated) {
    return treated;
  }
  const mod = modifierText(roll.modifier);
  if (roll.diceCount === 0) {
    return `${roll.total}`;
  }
  const extra = extraDiceText(roll, named);
  if (roll.physical || roll.faces.length === 0) {
    // The player typed what the dice showed, not each die.
    return `${roll.total - roll.modifier - extra.sum}${mod}${extra.text} = ${roll.total}`;
  }
  return `${roll.diceCount}d${roll.diceSides} (${roll.faces.join(', ')})${mod}${extra.text} = ${roll.total}`;
}

// ---- a d20 that a feature made count as another number (Talento Confiável, PM-03b) ----

/** A d20 of 9 or lower counts as 10 (Talento Confiável, SRD 5.1). */
export const TREATED_FLOOR = 10;

/** The Portuguese name of the feature that changed a d20 (`DiceRoll.treated_as_source`, a content key). */
const TREATED_SOURCE_NAMES: Readonly<Record<string, string>> = {
  'feature:reliable-talent': 'Talento Confiável',
};

/** The name a roll's `treated_as_source` goes by on the screens: the server's `treated_as_source_name` (sent only to the
 * roller and the master), else the SRD feature the app knows by its key (Talento Confiável), else "uma característica". */
export function treatedSourceName(source: string, name = ''): string {
  return name || (TREATED_SOURCE_NAMES[source] ?? 'uma característica');
}

/** The d20 that came up and counted as another number: the roll's one face, or the face of the die that counted when two
 * were rolled (the lowest one that was raised). `null` for a roll no feature changed. */
function treatedFace(roll: DiceRoll): number | null {
  const counted = roll.treatedAs;
  if (counted === undefined || roll.faces.length === 0) {
    return null;
  }
  // Two dice (advantage or disadvantage): the one that counted, when it is the one that was raised.
  const picked = roll.faces.length === 2 ? roll.faces[roll.countedIndex] : undefined;
  if (picked !== undefined && picked < counted) {
    return picked;
  }
  const raised = roll.faces.filter((f) => f < counted);
  return raised.length > 0 ? Math.min(...raised) : roll.faces[0];
}

/** "d20: 6 → 10 (Talento Confiável) + 9 = 19": what came up, what it counted as and the feature that did it. `null` when no
 * feature changed the roll: a d20 of 14 is never marked. */
export function treatedFormula(roll: DiceRoll, bonusNote = ''): string | null {
  const face = treatedFace(roll);
  if (face === null || roll.treatedAs === undefined) {
    return null;
  }
  // "+ 9 (Acrobacia)": a page that names what the bonus is for says it after the number.
  const note = bonusNote && roll.modifier !== 0 ? ` (${bonusNote})` : '';
  return `d20: ${face} → ${roll.treatedAs} (${treatedSourceName(roll.treatedAsSource, roll.treatedAsSourceName)})${modifierText(roll.modifier)}${note} = ${roll.total}`;
}

/** "O d20 de 6 contou como 10: perícia com proficiência.": the sentence under the result, or `null` when nothing changed. */
export function treatedSentence(roll: DiceRoll): string | null {
  const face = treatedFace(roll);
  return face === null || roll.treatedAs === undefined
    ? null
    : `O d20 de ${face} contou como ${roll.treatedAs}: perícia com proficiência.`;
}

/** What a screen reader says for the same roll: "d20: 6, contou 10 por Talento Confiável, mais 9, total 19". */
export function treatedSpeech(roll: DiceRoll): string | null {
  const face = treatedFace(roll);
  if (face === null || roll.treatedAs === undefined) {
    return null;
  }
  const mod =
    roll.modifier === 0
      ? ''
      : `, ${roll.modifier < 0 ? 'menos' : 'mais'} ${Math.abs(roll.modifier)}`;
  return `d20: ${face}, contou ${roll.treatedAs} por ${treatedSourceName(roll.treatedAsSource, roll.treatedAsSourceName)}${mod}, total ${roll.total}`;
}

/** The typed d20's live total before it is sent, when the character's Talento Confiável will raise it: "6 → 10 (Talento
 * Confiável) + 9 = 19". The server owns the rule; this only shows the account the sheet already knows. */
export function treatedPreview(
  face: number,
  bonus: number,
  source: string,
  name = '',
): { readonly text: string; readonly total: number } | null {
  const to = TREATED_FLOOR;
  if (face >= to) {
    return null;
  }
  const total = to + bonus;
  return {
    text: `${face} → ${to} (${treatedSourceName(source, name)})${modifierText(bonus)} = ${total}`,
    total,
  };
}

/** A roll for a sentence: the app's `1d20 (14) = 14`, a typed one `16 + 5 = 21
 * · dado físico` (or just `1 · dado físico` for a bare d20: `1 = 1` reads as a typo). */
export function rollText(roll: DiceRoll): string {
  if (!roll.physical || roll.treatedAs !== undefined) {
    return rollFormula(roll);
  }
  return roll.modifier === 0 ? `${roll.total} · dado físico` : `${rollFormula(roll)} · dado físico`;
}

/** A d20 whose bonus is not carried, only its face and total: `1d20 (2) + 2 = 4`, `1d20 (4) − 1 = 3`. */
export function d20TotalText(face: number, total: number): string {
  return `1d20 (${face})${modifierText(total - face)} = ${total}`;
}

/** The typed d20 and its bonus, before it is sent: `16 + 5 = 21`. */
export function typedTotal(face: number, bonus: number): string {
  return `${face}${modifierText(bonus)} = ${face + bonus}`;
}

/** "7 de fogo", "5 de dano perfurante": the physical types are adjectives. */
export function damageWords(amount: number, typePt: string): string {
  if (!typePt) {
    return `${amount} de dano`;
  }
  const adjective = /^(cortante|perfurante|contundente)$/.test(typePt);
  return `${amount} de ${adjective ? `dano ${typePt}` : typePt}`;
}

/** The dice a feature adds to a critical hit, with the feature's name ("1d12 Crítico Brutal"): the last `count` dice of
 * the roll are theirs. */
export interface ExtraDice {
  readonly count: number;
  readonly name: string;
}

/** The extra dice a pending damage or a log damage carries, or `undefined` when it has none. */
export function extraDiceOf(d: {
  readonly extraDiceCount: number;
  readonly extraDiceNamePt: string;
}): ExtraDice | undefined {
  return d.extraDiceCount > 0 ? { count: d.extraDiceCount, name: d.extraDiceNamePt } : undefined;
}

/** A rolled damage with the critical's own dice apart from the feature's: `2d12 (7, 11) + 1d12 Crítico Brutal (4) + 3
 * = 25`. The faces come in the server's order (the critical's first, the extra ones last), and the part of the modifier
 * that is the maximum of a critical that came without rolling stands apart ("12 (máximo) + ..."). `null` when the roll
 * cannot be split: typed dice have no faces. */
export function splitFormula(roll: DiceRoll, extra: ExtraDice, criticalMax = 0): string | null {
  const own = roll.diceCount - extra.count;
  if (extra.count <= 0 || roll.physical || own < 1 || roll.faces.length !== roll.diceCount) {
    return null;
  }
  const parts: string[] = [];
  if (criticalMax > 0) {
    parts.push(`${criticalMax} (máximo)`);
  }
  parts.push(`${own}d${roll.diceSides} (${roll.faces.slice(0, own).join(', ')})`);
  parts.push(
    `${extra.count}d${roll.diceSides} ${extra.name} (${roll.faces.slice(own).join(', ')})`,
  );
  return `${parts.join(' + ')}${modifierText(roll.modifier - criticalMax)} = ${roll.total}`;
}

/** The same split for dice rolled on the table: only the sum was typed, so the groups come from the counts
 * (`3d12 (2d12 + 1d12 Crítico Brutal) = 22 + 3 = 25`), and the maximum of a critical that came without rolling stands
 * apart. `null` when the roll is not a typed one or has no own dice. */
export function physicalSplitFormula(
  roll: DiceRoll,
  extra: ExtraDice,
  criticalMax = 0,
): string | null {
  const own = roll.diceCount - extra.count;
  if (extra.count <= 0 || !roll.physical || own < 1) {
    return null;
  }
  const sides = roll.diceSides;
  const groups = `${roll.diceCount}d${sides} (${own}d${sides} + ${extra.count}d${sides} ${extra.name})`;
  const bonus = roll.modifier - criticalMax;
  const sum = roll.total - roll.modifier;
  const max = criticalMax > 0 ? `${criticalMax} (máximo) + ` : '';
  return `${max}${groups} = ${sum}${modifierText(bonus)} = ${roll.total}`;
}

/** The damage line: `1d10 (7) = 7 de fogo`, `2d6 (5, 4) + 2 = 11 de dano`; with the extra dice of a critical hit the
 * groups are split (`2d12 (7, 11) + 1d12 Crítico Brutal (4) + 3 = 25 de dano cortante`). */
export function damageFormula(
  roll: DiceRoll,
  typePt: string,
  extra?: ExtraDice,
  criticalMax = 0,
): string {
  const words = damageWords(roll.total, typePt);
  const split = extra ? splitFormula(roll, extra, criticalMax) : null;
  if (split) {
    return `${split.replace(/ = \d+$/, '')} = ${words}`;
  }
  const shown = rollFormula(roll).replace(/ = \d+$/, '');
  // A flat number, or a typed sum with no bonus, has no formula to show.
  return roll.diceCount === 0 || shown === `${roll.total}` ? words : `${shown} = ${words}`;
}

/** The dice a damage is made of, as the field asks for it: "2d6". */
export function diceName(count: number, sides: number): string {
  return `${count}d${sides}`;
}

/** What a typed sum of physical dice may be (Q38): from one per die to every
 * die at its face. */
export function sumRange(count: number, sides: number): { min: number; max: number } {
  return { min: count, max: count * sides };
}

/** The face typed for a physical d20: a whole number from 1 to 20, or `null`. */
export function parseFace(text: string): number | null {
  return parseWhole(text, 1, 20);
}

/** A typed sum inside its range, or `null`. */
export function parseSum(text: string, min: number, max: number): number | null {
  return parseWhole(text, min, max);
}

function parseWhole(text: string, min: number, max: number): number | null {
  const value = text.trim();
  if (!/^\d{1,4}$/.test(value)) {
    return null;
  }
  const n = Number(value);
  return n >= min && n <= max ? n : null;
}
