import { formatInt, formatXp, joinDots, tight, upFirst } from './text';

describe('tight', () => {
  it('ties "1 hora", "10 minutos" and "3 criaturas" so a line never ends on the number', () => {
    expect(tight('ritual de 1 hora · dispensa 3 criaturas em 10 minutos')).toBe(
      'ritual de\u00a01\u00a0hora · dispensa 3\u00a0criaturas em 10\u00a0minutos',
    );
  });

  it('keeps a number, its unit and the word before it together', () => {
    expect(tight('+5 para acertar · alcance 6 m')).toBe('+5 para acertar · alcance\u00a06\u00a0m');
    expect(tight('Dá para andar mais 4,5 m (3 quadrados)')).toBe(
      'Dá para andar mais\u00a04,5\u00a0m (3\u00a0quadrados)',
    );
    expect(tight('Digite um número de 1 a 20')).toBe('Digite um número de\u00a01\u00a0a\u00a020');
    expect(tight('Restam 7,5 m de 15 m')).toBe('Restam\u00a07,5\u00a0m de\u00a015\u00a0m');
  });
});

describe('joinDots', () => {
  it('keeps the dot with the word before it, so a wrapped line starts with a word', () => {
    expect(joinDots(['+5 para acertar', '1d4 + 3 perfurante', 'alcance 6 m'])).toBe(
      '+5 para acertar\u00a0· 1d4 + 3 perfurante\u00a0· alcance 6 m',
    );
    expect(joinDots(['só um'])).toBe('só um');
  });
});

describe('tight with XP and gold', () => {
  it('ties the number to its unit', () => {
    expect(tight('Faltam 334 XP')).toBe('Faltam\u00a0334\u00a0XP');
    expect(tight('120 PO são 120 XP')).toBe('120\u00a0PO são 120\u00a0XP');
  });
});

describe('formatInt and formatXp', () => {
  it('groups the thousands with a dot and keeps the unit with its number', () => {
    expect(formatInt(0)).toBe('0');
    expect(formatInt(999)).toBe('999');
    expect(formatInt(2716)).toBe('2.716');
    expect(formatInt(1000000)).toBe('1.000.000');
    expect(formatXp(2700)).toBe('2.700\u00a0XP');
    expect(formatXp(0)).toBe('0\u00a0XP');
  });
});

describe('tight with treasures (E9-09)', () => {
  it('ties a count to "tesouro" and "tesouros"', () => {
    expect(tight('3 tesouros · 420 PO')).toBe('3\u00a0tesouros · 420\u00a0PO');
    expect(tight('1 tesouro')).toBe('1\u00a0tesouro');
  });
});

describe('upFirst', () => {
  it('raises the first letter and leaves the rest', () => {
    expect(upFirst('o Hobgoblin')).toBe('O Hobgoblin');
    expect(upFirst('ágil')).toBe('Ágil');
  });

  it('leaves an empty text and a text that already starts up alone', () => {
    expect(upFirst('')).toBe('');
    expect(upFirst('Brisa')).toBe('Brisa');
  });
});
