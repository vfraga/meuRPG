import {
  distanceInSentence,
  distanceText,
  distanceWithFeet,
  feetToMeters,
  formatMeters,
  metersText,
  metersToFeet,
  metersWithFeet,
  reachSquares,
  squaresFree,
  squaresText,
  poundsToKg,
  squaresToMeters,
  weightWithPounds,
} from './units';

/** The text ties numbers to their units with no-break spaces; the tests read it with plain ones. */
const plain = (s: string) => s.replace(/\u00a0/g, ' ');

describe('units: feet, meters and squares (1 quadrado = 1,5 m = 5 pés)', () => {
  it('turns feet into meters at the table rate, to one decimal', () => {
    expect(feetToMeters(5)).toBe(1.5);
    expect(feetToMeters(25)).toBe(7.5);
    expect(feetToMeters(30)).toBe(9);
    expect(feetToMeters(35)).toBe(10.5);
    expect(feetToMeters(0)).toBe(0);
    // An odd foot count: 7 ft is 2,1 m, not a float artefact.
    expect(feetToMeters(7)).toBe(2.1);
  });

  it('turns meters back into feet, and squares into meters', () => {
    expect(metersToFeet(9)).toBe(30);
    expect(metersToFeet(1.5)).toBe(5);
    expect(squaresToMeters(3)).toBe(4.5);
  });

  it('counts whole squares and never fewer than none', () => {
    expect(reachSquares(25)).toBe(5);
    expect(reachSquares(22)).toBe(4);
    expect(reachSquares(4)).toBe(0);
    expect(reachSquares(0)).toBe(0);
    expect(reachSquares(-5)).toBe(0);
  });

  it('says meters with a decimal comma and no useless zero', () => {
    expect(plain(formatMeters(1.5))).toBe('1,5 m');
    expect(plain(formatMeters(30))).toBe('30 m');
    expect(plain(formatMeters(0))).toBe('0 m');
    expect(plain(metersText(25))).toBe('7,5 m');
  });

  it('says squares in the singular for one', () => {
    expect(plain(squaresText(1))).toBe('1 quadrado');
    expect(plain(squaresText(5))).toBe('5 quadrados');
    expect(plain(squaresText(0))).toBe('0 quadrados');
  });

  it('writes a distance in both units: 5, 25, 30 and 35 ft, none, and an odd count', () => {
    expect(plain(distanceText(5))).toBe('1,5 m · 1 quadrado');
    expect(plain(distanceText(25))).toBe('7,5 m · 5 quadrados');
    expect(plain(distanceText(30))).toBe('9 m · 6 quadrados');
    expect(plain(distanceText(35))).toBe('10,5 m · 7 quadrados');
    expect(plain(distanceText(0))).toBe('0 m · 0 quadrados');
    // 22 ft is 6,6 m, but only 4 whole squares can be walked.
    expect(plain(distanceText(22))).toBe('6,6 m · 4 quadrados');
  });

  it('adds the feet for the sheet, which is in feet', () => {
    expect(plain(distanceWithFeet(25))).toBe('7,5 m · 5 quadrados (25 pés)');
    expect(plain(distanceWithFeet(30))).toBe('9 m · 6 quadrados (30 pés)');
    expect(plain(metersWithFeet(60))).toBe('18 m (60 pés)');
  });

  it('has the sentence and tile forms', () => {
    expect(plain(distanceInSentence(25))).toBe('7,5 m (5 quadrados)');
    expect(plain(squaresFree(25))).toBe('5 quadrados livres');
    expect(plain(squaresFree(5))).toBe('1 quadrado livre');
    expect(plain(squaresFree(0))).toBe('0 quadrados livres');
  });

  it('never lets a number come apart from its unit, nor a line start with the dot', () => {
    const text = distanceWithFeet(25);
    expect(text).toContain('7,5 m ·');
    expect(text).toContain('5 quadrados');
    expect(text).toContain('25 pés');
  });
});

describe('units: weight (1 lb = 0,5 kg)', () => {
  it('turns pounds into kilograms at the books rate, with a decimal comma', () => {
    expect(poundsToKg(180)).toBe('90 kg');
    expect(poundsToKg(195)).toBe('97,5 kg');
    expect(poundsToKg(0)).toBe('0 kg');
  });

  it('writes metric first with the book unit in parentheses, never breaking a number from its unit', () => {
    expect(weightWithPounds(180)).toBe('90\u00a0kg (180\u00a0lb)');
    expect(plain(weightWithPounds(195))).toBe('97,5 kg (195 lb)');
  });
});
