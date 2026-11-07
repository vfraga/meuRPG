import type { FeatChoice } from './effect-choices';

/**
 * A formula as a person reads it aloud: `8 + prof() + mod("@habilidade")` is "8 + bônus de proficiência + modificador da escolha
 * Habilidade". Only the functions the editor writes or explains are put into words; anything else stays as written.
 */

export interface ReadingPart {
  text: string;
  /** The part that names a choice of the feat. */
  strong: boolean;
}

const ABILITY_PT: Readonly<Record<string, string>> = {
  str: 'Força',
  dex: 'Destreza',
  con: 'Constituição',
  int: 'Inteligência',
  wis: 'Sabedoria',
  cha: 'Carisma',
};

const FUNCTION_PT: Readonly<Record<string, string>> = { mod: 'modificador', score: 'valor' };

const TOKEN = /\b(mod|score)\(\s*"([^"]*)"\s*\)|\bprof\(\s*\)|\blevel\(\s*\)/g;

/** What one `mod("…")` or `score("…")` says: of a choice by its label, of an ability by its name. */
function readCall(fn: string, arg: string, choices: readonly FeatChoice[]): ReadingPart {
  const what = FUNCTION_PT[fn];
  if (arg.startsWith('@')) {
    const label = choices.find((c) => c.name === arg.slice(1))?.label;
    return { text: `${what} da escolha ${label ?? arg}`, strong: true };
  }
  return { text: `${what} de ${ABILITY_PT[arg] ?? arg}`, strong: false };
}

export function formulaReading(text: string, choices: readonly FeatChoice[]): ReadingPart[] {
  const parts: ReadingPart[] = [];
  const plain = (s: string): void => {
    const spaced = s.replace(/\s+/g, ' ');
    if (spaced !== '') {
      parts.push({ text: spaced, strong: false });
    }
  };
  let at = 0;
  for (const m of text.trim().matchAll(TOKEN)) {
    const between = text.trim().slice(at, m.index);
    if (between.trim() === '' && parts.length > 0) {
      // Two calls with nothing between them are read as a sum, as the editor writes them.
      parts.push({ text: ' + ', strong: false });
    } else {
      plain(between);
    }
    if (m[1]) {
      parts.push(readCall(m[1], m[2], choices));
    } else {
      parts.push({
        text: m[0].startsWith('prof') ? 'bônus de proficiência' : 'nível',
        strong: false,
      });
    }
    at = m.index + m[0].length;
  }
  plain(text.trim().slice(at));
  return parts;
}

/** Whether the text points at a choice of the feat (`@name`), the only formulas the editor reads out. */
export function pointsAtChoice(text: string): boolean {
  return /@[a-z0-9_]+/.test(text);
}
