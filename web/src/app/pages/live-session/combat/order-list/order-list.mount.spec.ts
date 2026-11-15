import { TestBed } from '@angular/core/testing';

import { CombatantKind } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { OrderList } from './order-list';

describe('OrderList with a rider and its mount', () => {
  const text = (n: Element | null | undefined) => n?.textContent?.replace(/\s+/g, ' ').trim();
  function setup(mountKind: string) {
    const fixture = TestBed.createComponent(OrderList);
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        currentCombatantId: 'z',
        combatants: [
          combatant({
            id: 'tor',
            label: 'Tor',
            kind: CombatantKind.PLAYER,
            initiative: 17,
            mountedOnId: 'h',
          }),
          combatant({
            id: 'h',
            label: 'Cavalo de guerra',
            initiative: mountKind === 'independent' ? 9 : 17,
            riderId: 'tor',
            mountKind,
          }),
          combatant({ id: 'z', label: 'Zumbi', initiative: 12 }),
        ],
      }),
    );
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }

  it('boxes a controlled mount with its rider: "Montado", no hint, the mount indented and "Adestrada · leva Tor"', () => {
    const el = setup('controlled');
    const box = el.querySelector('app-order-group');
    expect(text(box?.querySelector('.head'))).toBe('linkMontadoiniciativa 17');
    expect(box?.querySelectorAll('.row')).toHaveLength(2);
    expect(text(box?.querySelector('.row--mount .row__sub'))).toContain('Adestrada · leva Tor');
  });

  it('leaves an independent mount in its own place, saying who it carries', () => {
    const el = setup('independent');
    expect(el.querySelector('app-order-group')).toBeNull();
    expect(text(el.querySelector('[data-combatant="h"] .row__sub'))).toContain(
      'Leva o Tor · independente',
    );
  });
});
