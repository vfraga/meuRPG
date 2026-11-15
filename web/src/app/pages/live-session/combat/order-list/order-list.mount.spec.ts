import { TestBed } from '@angular/core/testing';

import { type Combatant, CombatantKind } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { OrderList } from './order-list';

describe('OrderList with a rider and its mount', () => {
  const text = (n: Element | null | undefined) => n?.textContent?.replace(/\s+/g, ' ').trim();
  const tor = combatant({
    id: 'tor',
    label: 'Tor',
    kind: CombatantKind.PLAYER,
    initiative: 17,
    mountedOnId: 'h',
  });
  const horse = (mountKind: string, initiative = 17) =>
    combatant({ id: 'h', label: 'Cavalo de guerra', initiative, riderId: 'tor', mountKind });
  const zombie = combatant({ id: 'z', label: 'Zumbi', initiative: 12 });

  function render(combatants: Combatant[]) {
    const fixture = TestBed.createComponent(OrderList);
    fixture.componentRef.setInput('encounter', encounter({ currentCombatantId: 'z', combatants }));
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }
  const setup = (mountKind: string) =>
    render([tor, horse(mountKind, mountKind === 'independent' ? 9 : 17), zombie]);
  const indent = (el: HTMLElement, id: string) =>
    getComputedStyle(el.querySelector(`[data-combatant="${id}"] .row__head`)!).paddingLeft;

  it('boxes a controlled mount with its rider: "Montado · iniciativa 17", no hint, "Adestrada · leva Tor" with the link', () => {
    const el = setup('controlled');
    const box = el.querySelector('app-order-group');
    expect(text(box?.querySelector('.head'))).toBe('linkMontado· iniciativa 17');
    expect(box?.querySelectorAll('.row')).toHaveLength(2);
    const line = box?.querySelector('.row--mount .row__ride');
    expect(text(line)).toBe('linkAdestrada · leva Tor');
    expect(line?.querySelector('mat-icon')?.getAttribute('aria-hidden')).toBe('true');
  });

  it('indents the mount under its rider, in the rendered row, past the box padding of the master list', () => {
    const el = setup('controlled');
    expect(indent(el, 'h')).toBe('22px');
    expect(indent(el, 'tor')).not.toBe('22px');
  });

  it('leaves an independent mount in its own place, saying who it carries', () => {
    const el = setup('independent');
    expect(el.querySelector('app-order-group')).toBeNull();
    expect(text(el.querySelector('[data-combatant="h"] .row__sub'))).toContain(
      'Leva o Tor · independente',
    );
  });

  it('keeps the pair inside a tie under "Turno conjunto", the mount indented under its rider', () => {
    const brisa = combatant({
      id: 'b',
      label: 'Brisa',
      kind: CombatantKind.PLAYER,
      initiative: 17,
    });
    const el = render([brisa, tor, horse('controlled'), zombie]);
    const box = el.querySelector('app-order-group');
    expect(text(box?.querySelector('.head'))).toContain('Turno conjunto');
    expect(box?.querySelectorAll('.row')).toHaveLength(3);
    expect(indent(el, 'h')).toBe('22px');
  });

  it('does not indent an independent mount that only ties with its rider, and keeps its line', () => {
    const el = render([tor, horse('independent'), zombie]);
    expect(el.querySelector('app-order-group')).not.toBeNull();
    expect(el.querySelector('.row--mount')).toBeNull();
    expect(text(el.querySelector('[data-combatant="h"] .row__sub'))).toContain(
      'Leva o Tor · independente',
    );
  });
});
