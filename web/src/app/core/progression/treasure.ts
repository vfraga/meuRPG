import { timestampDate } from '@bufbuild/protobuf/wkt';

import {
  type TreasureToConvert,
  type XPAward,
  XPAwardMode,
} from '../../../gen/meurpg/progression/v1/progression_pb';
import { formatClock, formatDayAt } from '../../shared/session-time/session-time';
import { formatInt, tight } from '../format/text';
import { nameList } from './xp-labels';
import { type Split, eachLine, splitXp } from './xp-math';

/** "1 tesouro", "3 tesouros". */
export function treasureCount(n: number): string {
  return tight(n === 1 ? '1 tesouro' : `${formatInt(n)} tesouros`);
}

/** "Há mais 29 tesouros encontrados, que ficam para a próxima vez." (agreeing in number). */
export function moreTreasures(n: number, when: string): string {
  return `Há mais ${treasureCount(n)} ${n === 1 ? 'encontrado, que fica' : 'encontrados, que ficam'} ${when}.`;
}

/** "420 PO": the number tied to its unit. */
export function po(n: number): string {
  return tight(`${formatInt(n)} PO`);
}

/** The PO of the treasures together: what the server will sum too (1 XP each). */
export function totalPo(list: readonly Pick<TreasureToConvert, 'valuePo'>[]): number {
  return list.reduce((sum, t) => sum + t.valuePo, 0);
}

/** "3 tesouros · 420 PO": the strip's big line. */
export function stripHeadline(list: readonly TreasureToConvert[]): string {
  return `${treasureCount(list.length)} · ${po(totalPo(list))}`;
}

/** "Baú de moedas, 250 PO, de Brisa": one treasure's line in the strip. */
export function stripLine(t: TreasureToConvert): string {
  const who = nameList(t.foundBy.map((f) => f.characterName));
  return tight(`${t.name}, ${formatInt(t.valuePo)} PO${who ? `, de ${who}` : ''}`);
}

/** "às 21:40" for a find of today, "em 02/10 às 21:40" for an older one. */
export function foundWhen(foundAt: TreasureToConvert['foundAt'], now: Date = new Date()): string {
  if (!foundAt) {
    return '';
  }
  const date = timestampDate(foundAt);
  const today =
    date.getFullYear() === now.getFullYear() &&
    date.getMonth() === now.getMonth() &&
    date.getDate() === now.getDate();
  // The hour never leaves its "às" (`tight` does not know the accent).
  return today ? `às\u00a0${formatClock(date)}` : `em ${formatDayAt(date)}`;
}

/** "Encontrado por Brisa às 21:40" (who and when; either may be missing). */
export function foundLine(t: TreasureToConvert, now: Date = new Date()): string {
  const who = nameList(t.foundBy.map((f) => f.characterName));
  return ['Encontrado', who ? `por ${who}` : '', foundWhen(t.foundAt, now)]
    .filter(Boolean)
    .join(' ');
}

/** What the dialog's calculation box says for the treasures and the characters
 * checked now. The server sums and divides again; this is the line the master
 * reads before pressing the filled button. */
export interface TownCalc {
  /** "420 PO em 3 tesouros = 420 XP". */
  readonly sum: string;
  /** "420 XP ÷ 4 = 105 XP para cada". */
  readonly big: string;
  /** "Sobra 0 XP." or "Sobra 1 XP, que não vai para ninguém." */
  readonly left: string;
  readonly split: Split;
}

export function townCalc(treasures: number, poTotal: number, characters: number): TownCalc {
  const split = splitXp(poTotal, characters);
  const left =
    split.lost === 0
      ? 'Sobra 0 XP.'
      : `Sobra ${formatInt(split.lost)} XP, que não vai para ninguém.`;
  return {
    sum: tight(
      `${formatInt(poTotal)} PO em ${treasureCount(treasures)} = ${formatInt(poTotal)} XP`,
    ),
    big: tight(`${formatInt(poTotal)} XP ÷ ${characters} = ${eachLine(split)}`),
    left: tight(left),
    split,
  };
}

/** The history line of a "Voltar à cidade" award: "Voltar à cidade · 420 PO em
 * 3 tesouros" (the award's `gold` is the PO of its treasures). */
export function townTitle(count: number, poTotal: number): string {
  return tight(`Voltar à cidade · ${formatInt(poTotal)} PO em ${treasureCount(count)}`);
}

/** The title of a history line: what the master wrote, except for "Voltar à
 * cidade", whose line says the treasures and their PO (the same for everyone:
 * a player reads the count and the total, never which treasures). A player gets
 * no text for a milestone that was undone and is planned again: the line says
 * only "Marco". */
export function awardTitle(award: XPAward): string {
  if (award.treasureCount > 0) {
    return townTitle(award.treasureCount, award.gold);
  }
  return award.reason === '' && award.mode === XPAwardMode.XP_AWARD_MODE_MILESTONE
    ? 'Marco'
    : award.reason;
}

/** What the master reads right after "Voltar à cidade": "Voltar à cidade:
 * Pensantus, Toren e Brisa receberam 105 XP cada. Os 3 tesouros foram
 * convertidos." */
export function townGivenText(award: XPAward, xpEach: number, lostXp: number): string {
  const names = nameList(award.shares.map((s) => s.characterName));
  const lost = lostXp === 0 ? '' : ` Sobra ${formatInt(lostXp)} XP, que não vai para ninguém.`;
  const done =
    award.treasureCount === 1
      ? 'O tesouro foi convertido.'
      : `Os ${formatInt(award.treasureCount)} tesouros foram convertidos.`;
  return tight(
    `Voltar à cidade: ${names} ${award.shares.length > 1 ? 'receberam' : 'recebeu'} ${formatInt(xpEach)} XP ${award.shares.length > 1 ? 'cada' : ''}`.trim() +
      `. ${done}${lost}`,
  );
}

/** Said after undoing a "Voltar à cidade": the treasures are free again. */
export function townUndoneText(award: XPAward): string {
  return award.treasureCount === 1
    ? 'XP desfeito: o tesouro voltou a “encontrado, não convertido”.'
    : `XP desfeito: os ${treasureCount(award.treasureCount)} voltaram a “encontrado, não convertido”.`;
}
