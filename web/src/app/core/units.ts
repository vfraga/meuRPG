import { formatInt, joinDots, tight } from './format/text';

/**
 * Every distance the app writes comes from here (MR-013, E8-01): the table
 * measures in squares of 1,5 m (5 ft), so a distance is said in meters and in
 * squares together, "7,5 m · 5 quadrados", and the sheet, which is in feet, adds
 * them in parentheses, "(25 pés)". One place so the combat, the sheet and the
 * NPC form agree on the rounding (they used to have three copies). Pure
 * functions, no Angular: the text is tested without a DOM.
 */

/** One square is 5 ft, which is 1,5 m. */
export const SQUARE_FT = 5;
export const SQUARE_M = 1.5;

/** Meters for a distance in feet, at the table's rate and to one decimal place:
 * 25 ft is 7,5 m. Not the 0,3048 of the real foot. */
export function feetToMeters(feet: number): number {
  return Math.round(feet * 3) / 10;
}

/** The reverse, for a form that asks for meters: 9 m is 30 ft. Rounded to whole feet. */
export function metersToFeet(meters: number): number {
  return Math.round(meters / 0.3);
}

export function squaresToMeters(squares: number): number {
  return squares * SQUARE_M;
}

/** How many whole squares a distance in feet holds (movement left is spent a
 * square at a time, so 22 ft is 4 squares); never negative. */
export function reachSquares(feet: number): number {
  return Math.max(0, Math.floor(feet / SQUARE_FT));
}

/** Meters as the table says them: "1,5 m", "30 m", with a decimal comma and no useless zero. */
export function formatMeters(meters: number): string {
  const rounded = Math.round(meters * 10) / 10;
  return tight(`${String(rounded).replace('.', ',')} m`);
}

/** "5 quadrados", "1 quadrado". */
export function squaresText(squares: number): string {
  return tight(`${formatInt(squares)} ${squares === 1 ? 'quadrado' : 'quadrados'}`);
}

/** Meters as a bare number with the decimal comma ("4,5"), for a field whose unit is written beside it. */
export function metersNumber(feet: number): string {
  return String(feetToMeters(feet)).replace('.', ',');
}

/** A distance in feet as meters alone: "7,5 m". For a range, where the table
 * thinks in meters ("alcance 36 m"). */
export function metersText(feet: number): string {
  return formatMeters(feetToMeters(feet));
}

/** Meters with one decimal always ("9,0 m", "0,0 m"): what the movement says on
 * the turn and the "Mover" page (E9-05), where "9 m" and "9,0 m" would not read
 * as the same number from one screen to the next. */
export function metersFixed(feet: number): string {
  return tight(`${feetToMeters(feet).toFixed(1).replace('.', ',')} m`);
}

/** A distance in feet in the table's two units: "7,5 m · 5 quadrados". */
export function distanceText(feet: number): string {
  return joinDots([metersText(feet), squaresText(reachSquares(feet))]);
}

/** The same for a sentence ("Dá para andar até 7,5 m (5 quadrados)"). */
export function distanceInSentence(feet: number): string {
  return `${metersText(feet)} (${squaresText(reachSquares(feet))})`;
}

/** The squares and the feet, the half that follows the meters on the sheet:
 * "5 quadrados (25 pés)". */
export function squaresWithFeet(feet: number): string {
  return `${squaresText(reachSquares(feet))} ${tight(`(${feet} pés)`)}`;
}

/** The sheet's form, which is in feet: "7,5 m · 5 quadrados (25 pés)". */
export function distanceWithFeet(feet: number): string {
  return joinDots([metersText(feet), squaresWithFeet(feet)]);
}

/** The movement tile's second line: "5 quadrados livres". */
export function squaresFree(feet: number): string {
  const n = reachSquares(feet);
  return tight(`${n} ${n === 1 ? 'quadrado livre' : 'quadrados livres'}`);
}

/** "7,5 m (25 pés)": a distance with no squares (a sense's range on the sheet). */
export function metersWithFeet(feet: number): string {
  return `${metersText(feet)} ${tight(`(${feet} pés)`)}`;
}

/** Kilograms for a weight in pounds, at the Portuguese books' rate (1 lb = 0,5 kg), with a decimal comma and no
 * useless zero: 180 lb is "90 kg", 195 lb is "97,5 kg". */
export function poundsToKg(pounds: number): string {
  const kg = Math.round((pounds / 2) * 10) / 10;
  return `${String(kg).replace('.', ',')} kg`;
}

/** A weight in pounds in the app's two units, metric first and the book's unit after it: "90 kg (180 lb)". */
export function weightWithPounds(pounds: number): string {
  return `${poundsToKg(pounds)} (${pounds} lb)`.replace(/(\d) (kg|lb)/g, '$1\u00a0$2');
}
