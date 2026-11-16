// What the Inspiração asks of the master inside his combat (RN-43, boards 581-05 and 581-06): the cards at the top of what waits, and
// the count on the tab that holds them.

import { TestBed } from '@angular/core/testing';

import { favorPass, favorRefund } from '../../../core/play/favor-testing';
import { FavorState } from '../../../core/play/favor-state';
import { combatant, encounter } from '../../../core/combat/combat-testing';
import { CombatantKind, CombatantSide } from '../../../../gen/meurpg/play/v1/combat_pb';
import { mountCombatView } from './combat-view-testing';

const plain = (t: string | null | undefined) => (t ?? '').replace(/\s+/g, ' ').trim();

const enc = () =>
  encounter({
    revision: 1,
    currentCombatantId: 'g',
    combatants: [
      combatant({
        id: 'b',
        label: 'Brisa',
        kind: CombatantKind.PLAYER,
        side: CombatantSide.PARTY,
        characterId: 'brisa-c',
        initiative: 18,
      }),
      combatant({ id: 'g', label: 'Goblin chefe', initiative: 15 }),
    ],
  });

describe('CombatView: what the Inspiração asks of the master', () => {
  it('shows the cards at the top of what waits, and counts them on the tab', async () => {
    const { fixture, el, view } = mountCombatView({
      enc: enc(),
      master: true,
      matches: ['(min-width: 1024px)'],
    });
    await fixture.whenStable();
    fixture.detectChanges();
    expect(el.querySelector('app-favor-requests .fr')).toBeNull();

    const favor = TestBed.inject(FavorState);
    favor.passes.set([favorPass()]);
    favor.refunds.set([favorRefund()]);
    fixture.detectChanges();
    const cards = el.querySelectorAll('app-favor-requests article');
    expect(cards).toHaveLength(2);
    expect(plain(cards[0].querySelector('h3')?.textContent)).toBe('Brisa quer passar a Inspiração');
    expect(plain(el.querySelector('app-favor-requests #fr-h')?.textContent)).toContain(
      'Para você responder',
    );
    const turn = view.layout.railTabs()[0];
    expect(turn).toMatchObject({ label: 'Turno', badge: '2', words: '2 esperam você' });
  });

  it('shows no card to a player of the combat', async () => {
    const { fixture, el } = mountCombatView({ enc: enc(), master: false });
    await fixture.whenStable();
    TestBed.inject(FavorState).passes.set([favorPass()]);
    fixture.detectChanges();
    expect(el.querySelector('app-favor-requests')).toBeNull();
  });
});
