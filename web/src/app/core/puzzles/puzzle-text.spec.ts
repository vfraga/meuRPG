import {
  cipherColumns,
  fold,
  hasCipherLetter,
  keywordLetters,
  keywordSwapsNothing,
} from './puzzle-text';

describe('the text rules of the puzzle forms', () => {
  it('folds as the server does: no capitals, accents or punctuation, one space between words', () => {
    expect(fold('  A Sombra! ')).toBe('a sombra');
    expect(fold('Escuridão')).toBe('escuridao');
    expect(fold('o tesouro, está... sob o altar?')).toBe('o tesouro esta sob o altar');
    expect(fold('!!!')).toBe('');
    expect(fold('Maçã 7')).toBe('maca 7');
  });

  it('folds exactly the accents the server folds, and keeps the other letters as they are', () => {
    expect(fold('ÁÀÂÃÄÅĀ ÉÈÊËĒ ÍÌÎÏĪ ÓÒÔÕÖØŌ ÚÙÛÜŪ ÇÑÝÿ ßÆŒ')).toBe(
      'aaaaaaa eeeee iiiii ooooooo uuuuu cnyy ssaeoe',
    );
    // Not in the server's list: the form must not call two such answers "the same".
    expect(fold('ě')).toBe('ě');
    expect(fold('ě')).not.toBe(fold('e'));
    expect(fold('š')).toBe('š');
  });

  it("needs a letter A to Z in a cipher's message", () => {
    expect(hasCipherLetter('O tesouro')).toBe(true);
    expect(hasCipherLetter('123 !!!')).toBe(false);
    expect(hasCipherLetter('çã')).toBe(true);
  });

  it('counts the distinct letters of a keyword, once folded', () => {
    expect(keywordLetters('Lua')).toEqual(['l', 'u', 'a']);
    expect(keywordLetters('Mistério!')).toEqual(['m', 'i', 's', 't', 'e', 'r', 'o']);
    expect(keywordLetters('aaa')).toEqual(['a']);
  });

  it('knows a keyword that swaps nothing', () => {
    expect(keywordSwapsNothing(keywordLetters('abc'))).toBe(true);
    expect(keywordSwapsNothing(keywordLetters('abd'))).toBe(false);
    expect(keywordSwapsNothing(keywordLetters('lua'))).toBe(false);
  });

  it("lists the decoding table's columns: the distinct letters of the ciphered letter, in order", () => {
    expect(cipherColumns('R WHVRXUR HVWD VRE R DOWDU')).toEqual([
      'D',
      'E',
      'H',
      'O',
      'R',
      'U',
      'V',
      'W',
      'X',
    ]);
    expect(cipherColumns('1, 2!')).toEqual([]);
  });
});

describe('fold against the Go Fold of the server', () => {
  // Expected values come from the server's Fold: Nd digits only, simple lower case per character.
  const cases: [string, string][] = [
    ['m²', 'm'],
    ['x³', 'x'],
    ['a²b', 'a b'],
    ['½', ''],
    ['Ⅳ', ''],
    ['ⅷ', ''],
    ['①', ''],
    ['İ', 'i'],
    ['İstanbul', 'istanbul'],
    ['ΑΣ', 'ασ'],
    ['ΟΔΥΣΣΕΥΣ', 'οδυσσευσ'],
    ['Σ', 'σ'],
    ['ǅ', 'ǆ'],
    ['ᾼ', 'ᾳ'],
    ['  A Sombra! ', 'a sombra'],
    ['Ação', 'acao'],
  ];
  for (const [input, expected] of cases) {
    it(`folds ${JSON.stringify(input)} to ${JSON.stringify(expected)}`, () => {
      expect(fold(input)).toBe(expected);
    });
  }
});
