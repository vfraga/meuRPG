import { describe, expect, it } from 'vitest';

import type { FeatChoice } from './effect-choices';
import { formulaReading, pointsAtChoice } from './formula-reading';

const ability = (name: string, label: string): FeatChoice => ({
  kind: 'ability',
  name,
  number: 1,
  ofKind: 1,
  label,
  index: 0,
});

const sentence = (text: string, choices: readonly FeatChoice[] = []): string =>
  formulaReading(text, choices)
    .map((p) => p.text)
    .join('');

describe('formulaReading', () => {
  it('reads the proficiency bonus and the modifier of a choice, as the board says it', () => {
    const parts = formulaReading('8 + prof() + mod("@habilidade")', [
      ability('habilidade', 'Habilidade'),
    ]);
    expect(parts.map((p) => p.text).join('')).toBe(
      '8 + bônus de proficiência + modificador da escolha Habilidade',
    );
    expect(parts.filter((p) => p.strong).map((p) => p.text)).toEqual([
      'modificador da escolha Habilidade',
    ]);
  });

  it('reads the score of a choice with the numbered label of a feat with two', () => {
    expect(sentence('score("@b")', [ability('b', 'Habilidade (2ª)')])).toBe(
      'valor da escolha Habilidade (2ª)',
    );
  });

  it('reads an ability by its name and the level', () => {
    expect(sentence('mod("dex") + level()')).toBe('modificador de Destreza + nível');
  });

  it('keeps what it does not know and collapses the spaces', () => {
    expect(sentence('  1   +  classLevel("fighter")  ')).toBe('1 + classLevel("fighter")');
  });

  it('names a choice the feat no longer has by the token', () => {
    expect(sentence('mod("@sumiu")')).toBe('modificador da escolha @sumiu');
  });

  it('reads two calls with nothing between them as a sum', () => {
    expect(sentence('mod("@a")score("@a")', [ability('a', 'Habilidade')])).toBe(
      'modificador da escolha Habilidade + valor da escolha Habilidade',
    );
  });

  it('reads nothing for an empty formula', () => {
    expect(formulaReading('   ', [])).toEqual([]);
  });

  it('says whether a text points at a choice', () => {
    expect(pointsAtChoice('mod("@a")')).toBe(true);
    expect(pointsAtChoice('prof() + 8')).toBe(false);
  });
});
