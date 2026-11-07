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
