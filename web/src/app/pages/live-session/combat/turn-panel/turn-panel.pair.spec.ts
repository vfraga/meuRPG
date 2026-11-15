import { TestBed } from '@angular/core/testing';

import { CombatantKind } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { TurnPanel } from './turn-panel';

describe('TurnPanel: the rider with the mount it directs (588-04, 588-05)', () => {
  it('is the rider\'s own turn, "Montado no Cavalo de guerra", with no joint pill and one "Encerrar turno"', () => {
    const fixture = TestBed.createComponent(TurnPanel);
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        currentCombatantId: 'tor',
        turnGroupIds: ['tor', 'h'],
        combatants: [
          combatant({
            id: 'tor',
            label: 'Tor',
            kind: CombatantKind.PLAYER,
            mine: true,
            initiative: 17,
            mountedOnId: 'h',
          }),
          combatant({
            id: 'h',
            label: 'Cavalo de guerra',
            initiative: 17,
            riderId: 'tor',
            mountKind: 'controlled',
          }),
          combatant({ id: 'z', label: 'Zumbi 1', initiative: 12 }),
        ],
      }),
    );
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelector('.turn__title')?.textContent).toBe('Sua vez, Tor');
    expect(el.querySelector('.turn__sub')?.textContent?.replace(/\s+/g, ' ')).toContain(
      'Montado no Cavalo de guerra · em seguida: Zumbi 1',
    );
    expect(el.querySelector('app-joint-pill')).toBeNull();
    expect(el.querySelector('app-end-part')).toBeNull();
    expect(el.querySelector('app-end-turn')).not.toBeNull();
  });
});
