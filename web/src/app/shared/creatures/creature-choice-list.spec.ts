import { TestBed } from '@angular/core/testing';

import { type ChoiceRow, CreatureChoiceList } from './creature-choice-list';

function render(rows: readonly ChoiceRow[]): HTMLElement {
  const fixture = TestBed.createComponent(CreatureChoiceList);
  fixture.componentRef.setInput('rows', rows);
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

describe('CreatureChoiceList table creatures', () => {
  const rows: readonly ChoiceRow[] = [
    { key: 'creature:lobo@mesa', title: 'Lobo do Vale', subtitle: 'Médio · 12 PV', table: true },
    { key: 'monster:wolf', title: 'Lobo', subtitle: 'Médio · ND 1/4' },
  ];

  it('tags a table row "Da mesa" and says whose numbers it gets, in place of its own', () => {
    const [table, book] = Array.from(render(rows).querySelectorAll('.row'));
    expect(table.querySelector('app-table-mark')?.textContent).toContain('Da mesa');
    expect(table.querySelector('.row__sub')?.textContent?.trim()).toBe(
      'Os números vêm da sua ficha quando ele chega.',
    );
    expect(table.textContent).not.toContain('12 PV');
    expect(book.querySelector('app-table-mark')).toBeNull();
    expect(book.querySelector('.row__sub')?.textContent?.trim()).toBe('Médio · ND 1/4');
  });
});

describe('CreatureChoiceList, a row with a warning (#588, board 588-21)', () => {
  const rows: readonly ChoiceRow[] = [
    {
      key: 'monster:pony',
      title: 'Pônei',
      subtitle: 'Médio · 11 PV',
      warning: 'Pequeno demais para você montar',
    },
    { key: 'monster:warhorse', title: 'Cavalo de guerra', subtitle: 'Grande · 19 PV' },
  ];

  it('says the warning under the numbers, with an icon, and the row stays pickable', () => {
    const [pony, horse] = Array.from(render(rows).querySelectorAll('.row'));
    const warn = pony.querySelector('.row__warn');
    expect(warn?.textContent).toContain('Pequeno demais para você montar');
    expect(warn?.querySelector('mat-icon')?.getAttribute('aria-hidden')).toBe('true');
    expect(pony.querySelector('.row__sub')?.textContent?.trim()).toBe('Médio · 11 PV');
    expect(pony.querySelector<HTMLInputElement>('input[type=radio]')?.disabled).toBe(false);
    expect(horse.querySelector('.row__warn')).toBeNull();
  });
});
