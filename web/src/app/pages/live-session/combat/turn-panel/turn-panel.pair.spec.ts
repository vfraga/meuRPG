import { TestBed } from '@angular/core/testing';

import { CombatantKind } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { TurnPanel } from './turn-panel';

describe('TurnPanel: the rider with the mount it directs (588-04, 588-05)', () => {
  it('is the rider\'s own turn, "Cavalgando o Cavalo de guerra", with no joint pill and one "Encerrar turno"', () => {
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
      'Cavalgando o Cavalo de guerra · em seguida: Zumbi 1',
    );
    // Boards 588-04 and 588-05: the token of the turn is the pair's, the mount with the rider's small disc on its corner.
    const pair = el.querySelector('.turn__banner app-pair-token')!;
    const [mount, rider] = Array.from(pair.querySelectorAll('app-combatant-token'));
    expect([mount.textContent?.trim(), rider.textContent?.trim()]).toEqual(['C', 'T']);
    expect(mount.classList).toContain('tk--npc');
    expect(mount.classList).toContain('tk--current');
    const disc = getComputedStyle(rider);
    expect([disc.position, disc.top, disc.right]).toEqual(['absolute', '-6px', '-6px']);
    expect(el.querySelectorAll('.turn__banner > app-combatant-token')).toHaveLength(0);
    expect(el.querySelector('app-joint-pill')).toBeNull();
    expect(el.querySelector('app-end-part')).toBeNull();
    expect(el.querySelector('app-end-turn')).not.toBeNull();
  });
});
