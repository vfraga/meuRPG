import { editedLine, whenText } from './document-format';

const now = new Date(2026, 9, 2, 9, 30);

describe('document format', () => {
  it('says today, yesterday, and the date', () => {
    expect(whenText(new Date(2026, 9, 2, 7, 5), now)).toBe('hoje às 07:05');
    expect(whenText(new Date(2026, 9, 1, 22, 10), now)).toBe('ontem às 22:10');
    expect(whenText(new Date(2026, 8, 28, 22, 10), now)).toBe('28/09 às 22:10');
    expect(whenText(new Date(2025, 11, 31, 22, 10), now)).toBe('31/12/2025 às 22:10');
  });

  it('writes who edited it, when it is known', () => {
    const at = new Date(2026, 9, 1, 22, 10);
    expect(editedLine(at, 'Samuel', now)).toBe('Editado por Samuel ontem às 22:10.');
    expect(editedLine(at, '', now)).toBe('Editado ontem às 22:10.');
    expect(editedLine(null, '', now)).toBe('Ainda não foi salvo.');
  });
});
