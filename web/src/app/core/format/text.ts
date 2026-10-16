/**
 * How the screens write numbers and detail lines, shared by the combat and
 * the XP screens (it used to live in `combat-grid.ts`). Plain functions, no
 * Angular: the text is tested without a DOM.
 */

/** Joins the segments of a detail line with " · ": a no-break space before the
 * dot and a normal one after it, so a wrapped line ends with the dot and the
 * next starts with a word. */
export function joinDots(parts: readonly string[]): string {
  return parts.join(' · ');
}

/** Keeps a number, its unit and the words that lead to it on one line
 * ("alcance 36 m", "mais 4,5 m", "de 1 a 20", "2.716 XP", "120 PO") with
 * no-break spaces, so a line never ends on "alcance" or begins with "m". */
export function tight(text: string): string {
  return text
    .replace(/(\d) a (\d)/g, '$1 a $2')
    .replace(/(\d) (m|XP|PO|quadrados?|pés|horas?|minutos?|criaturas?|tesouros?)\b/g, '$1 $2')
    .replace(/\b(alcance|de|das|às|até|mais|restam|Restam|faltam|Faltam|a) (?=\d)/g, '$1 ');
}

/** A name or a word with its number ("Goblin 1", "Sessão 1", "Rodada 2") as one block, so a line
 * never breaks between them. Apart from `tight()`, which stays for numbers and their units. */
export function tieNumbers(text: string): string {
  return text.replace(/(\p{Lu}\p{L}*) (?=\d)/gu, '$1 ');
}

/** "2.716": pt-BR thousands separator, done by hand so the result never
 * depends on the runtime's locale data. Whole numbers only. */
export function formatInt(n: number): string {
  return String(Math.trunc(n)).replace(/\B(?=(\d{3})+(?!\d))/g, '.');
}

/** "2.700 XP", the number and its unit tied with a no-break space. */
export function formatXp(xp: number): string {
  return `${formatInt(xp)} XP`;
}

/** Ties a one-letter word to the next one ("A carroça", "O vau"), so a title never
 * breaks after "A" and leaves it alone at the end of a line. */
export function tieShortWords(text: string): string {
  return text.replace(/(^|\s)(\p{L}) (?=\S)/gu, '$1$2\u00a0');
}

/** The first letter up: "o Hobgoblin" is "O Hobgoblin". */
export function upFirst(text: string): string {
  return `${text.charAt(0).toUpperCase()}${text.slice(1)}`;
}
