// The master's "Aparar: perguntar a cada golpe" switch in the row menu: only an NPC with a Parry has it.
import { TestBed } from '@angular/core/testing';

import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { OrderList } from './order-list';
import { openAllRows } from './order-list-testing';

describe('the Parry switch in the row menu', () => {
  const flat = (n: Element | null | undefined) => n?.textContent?.replace(/\s+/g, ' ').trim();
  const captain = (ask?: boolean) =>
    combatant({
      id: 'cap',
      label: 'Capitão bandido',
      ...(ask === undefined ? {} : { parryAsk: ask }),
    } as never);
  const goblin = combatant({ id: 'gob', label: 'Goblin 1' });

  function mount(cap = captain(true)) {
    const fixture = TestBed.createComponent(OrderList);
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants: [cap, goblin], currentCombatantId: 'gob' }),
    );
    const sent: { id: string; ask: boolean }[] = [];
    fixture.componentInstance.parryAsk.subscribe((c) => sent.push(c));
    fixture.detectChanges();
    openAllRows(fixture);
    return { fixture, sent };
  }
  function menuOf(fixture: ReturnType<typeof mount>['fixture'], name: string) {
    (fixture.nativeElement as HTMLElement)
      .querySelector<HTMLButtonElement>(`button[aria-label="Mais ações para ${name}"]`)!
      .click();
    fixture.detectChanges();
    return Array.from(document.querySelectorAll<HTMLElement>('.mat-mdc-menu-item'));
  }
  const close = (fixture: ReturnType<typeof mount>['fixture']) => {
    document.body.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    fixture.detectChanges();
  };

  it('is a checkbox of the menu that is checked while it asks, and turns it off', () => {
    const { fixture, sent } = mount(captain(true));
    const item = menuOf(fixture, 'Capitão bandido').find((i) => flat(i)?.includes('Aparar'))!;
    expect(flat(item)).toBe('check_boxAparar: perguntar a cada golpe');
    expect(item.getAttribute('role')).toBe('menuitemcheckbox');
    expect(item.getAttribute('aria-checked')).toBe('true');
    item.click();
    expect(sent).toEqual([{ id: 'cap', ask: false }]);
  });

  it('names the same setting, unchecked, while the server decides, and turns it on', () => {
    const { fixture, sent } = mount(captain(false));
    const item = menuOf(fixture, 'Capitão bandido').find((i) => flat(i)?.includes('Aparar'))!;
    expect(flat(item)).toBe('check_box_outline_blankAparar: perguntar a cada golpe');
    expect(item.getAttribute('role')).toBe('menuitemcheckbox');
    expect(item.getAttribute('aria-checked')).toBe('false');
    item.click();
    expect(sent).toEqual([{ id: 'cap', ask: true }]);
  });

  it('is one entry, between the cover mark and the rest of the menu', () => {
    const { fixture } = mount(captain(true));
    const items = menuOf(fixture, 'Capitão bandido').map((i) => flat(i));
    expect(items.filter((i) => i?.includes('Aparar'))).toHaveLength(1);
    const at = items.findIndex((i) => i?.includes('Aparar'));
    expect(items[at - 1]).toContain('Marcar cobertura');
  });

  it('is not in the menu of an NPC without a Parry, nor of a player', () => {
    const { fixture } = mount(captain());
    expect(menuOf(fixture, 'Capitão bandido').some((i) => flat(i)?.includes('Aparar'))).toBe(false);
    close(fixture);
    expect(menuOf(fixture, 'Goblin 1').some((i) => flat(i)?.includes('Aparar'))).toBe(false);
  });
});
