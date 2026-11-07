import type {
  CombatLogDamage,
  CombatLogEffect,
  CombatLogSpellTarget,
  DiceRoll,
} from '../../../gen/meurpg/play/v1/combat_pb';
import type { CountedDie, DamagePartRoll } from '../../../gen/meurpg/play/v1/combat_rolls_pb';
import { modifierText, rollText, treatedFormula, treatedSourceName } from './combat-dice';
import { listNames } from './conditions';
import { hasRuleReroll, luckyText } from './lucky';

/**
 * The lines under a log sentence that tell the dice: a die a table feature's floor counted as another number (a
 * `roll_minimum`), a d20 a reroll rule rolled again, a d20 that counted as another number. The server sends these fields only
 * to whoever may read the roll (the roller's player and the master), so a line is written only when the field is there:
 * nothing here decides who reads what.
 */

/** "1 → 2, 7": the faces of a roll, the dice a floor raised as "what showed → what counted". */
export function markedFaces(faces: readonly number[], counted: readonly CountedDie[]): string {
  return faces
    .map((face, i) => {
      const c = counted.find((x) => x.index === i);
      return c ? `${c.face} → ${c.countedAs}` : String(face);
    })
    .join(', ');
}

/** "Espada curta 1d6 (4) + 2 = 6": the dice of one part of a damage, with the dice a floor raised marked. */
export function partLine(p: DamagePartRoll): string {
  const dice = p.diceCount > 0 ? `${p.diceCount}d${p.diceSides}` : '';
  const faces = p.faces.length > 0 ? ` (${markedFaces(p.faces, p.countedDice)})` : '';
  const flat = p.flat !== 0 ? ` ${p.flat > 0 ? '+' : '−'} ${Math.abs(p.flat)}` : '';
  const out = p.counted ? '' : ' — não conta';
  return `${p.labelPt} ${dice}${faces}${dice ? flat : flat.trim()} = ${p.sum + p.flat}${out}`.trim();
}

/** "Chama Insistente: o 1 contou como 2.": one sentence for each feature that raised a die. A feature with no name (retired,
 * switched off, or a roll the reader may not know) reads "uma característica". */
export function countedSources(dice: readonly CountedDie[]): string[] {
  const bySource = new Map<string, CountedDie[]>();
  for (const d of dice) {
    bySource.set(d.sourceKey, [...(bySource.get(d.sourceKey) ?? []), d]);
  }
  return [...bySource.values()].map((list) => {
    const what = listNames(list.map((d) => `o ${d.face} contou como ${d.countedAs}`));
    return `${treatedSourceName('', list[0].sourceName)}: ${what}.`;
  });
}

/** The dice of a damage that a floor raised, from its parts and from its own roll. */
export function countedOf(d: CombatLogDamage): CountedDie[] {
  return [...d.parts.flatMap((p) => p.countedDice), ...(d.roll?.countedDice ?? [])];
}

/** "2d10 (1 → 2, 7) = 9": the roll of a damage with no parts of its own (a spell's), when a floor raised one of its dice;
 * `[]` otherwise. */
export function rolledLine(d: CombatLogDamage): string[] {
  const roll = d.roll;
  if (d.parts.length > 0 || !roll || roll.countedDice.length === 0 || roll.faces.length === 0) {
    return [];
  }
  const faces = markedFaces(roll.faces, roll.countedDice);
  return [
    `${roll.diceCount}d${roll.diceSides} (${faces})${modifierText(roll.modifier)} = ${roll.total}`,
  ];
}

/** Everything under a sentence about the dice a floor raised in one damage: the line of each roll with a raised die marked,
 * then the sentence naming the feature. Nothing when no die was raised. */
export function countedNotes(d: CombatLogDamage | undefined): string[] {
  if (!d) {
    return [];
  }
  const parts = d.parts.filter((p) => p.countedDice.length > 0).map(partLine);
  return [...parts, ...rolledLine(d), ...countedSources(countedOf(d))];
}

/** The notes of every damage a spell's targets took (a spell with two damage types lists each). */
export function castCountedNotes(targets: readonly CombatLogSpellTarget[]): string[] {
  return targets.flatMap((t) => [t.damage, ...t.moreDamages].flatMap(countedNotes));
}

/** "Sorte Teimosa: 2 → 14 · 1d20 (14) + 5 = 19": a d20 a reroll rule rolled again, with the roll it made. The rerolls of
 * Lucky, Indomitable and Stroke of Luck keep their short line. */
export function rerollNote(roll: DiceRoll | undefined): string {
  const note = luckyText(roll);
  return note && hasRuleReroll(roll) && roll ? `${note} · ${rollText(roll)}` : note;
}

/** The d20 that counted as another number on a spell's saving throw, as a note: "d20: 3 → 8 (Vontade de Ferro) + 2 = 10". */
export function castTreatedNotes(targets: readonly CombatLogSpellTarget[]): string[] {
  return targets.flatMap((t) => {
    const line = t.save?.roll ? treatedFormula(t.save.roll) : null;
    return line ? [line] : [];
  });
}

/** The same for the saving throw against a lasting effect, which carries the face, the number it counted as and the total
 * apart. */
export function effectTreatedNote(fx: CombatLogEffect): string {
  if (fx.treatedAs === undefined || fx.d20 === undefined || fx.total === undefined) {
    return '';
  }
  const name = treatedSourceName(fx.treatedAsSource, fx.treatedAsSourceName);
  return `d20: ${fx.d20} → ${fx.treatedAs} (${name})${modifierText(fx.total - fx.treatedAs)} = ${fx.total}`;
}
