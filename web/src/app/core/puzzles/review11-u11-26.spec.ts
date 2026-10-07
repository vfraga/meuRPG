// Finding U11-26: fold() in puzzle-text.ts differs from the server's Fold (backend/internal/rules/puzzle/answers.go:42)
// for characters outside the plain Latin set (No/Nl numbers, dotted capital I, final sigma). Expected values were
// computed by running the Go Fold function (go 1.24) on each input.
import { describe, expect, it } from 'vitest';
import { fold } from './puzzle-text';

describe('Review11 U11-26: fold() matches the server Fold', () => {
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
    it(`fold(${JSON.stringify(input)}) === ${JSON.stringify(expected)}`, () => {
      expect(fold(input)).toBe(expected);
    });
  }
});
