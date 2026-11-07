import { TestBed } from '@angular/core/testing';

import type { TvCombatantVm, TvMapVm } from '../../../core/play/table-screen-view';
import { fogMapBody, rollsOf } from '../table-screen-testing';
import { TV_LOG_LINES, TvCombat } from './tv-combat';

const order: TvCombatantVm[] = [
  { name: 'Brisa', initial: 'B', stateWord: null, onTurn: false, isPlayer: true },
  { name: 'Tor', initial: 'T', stateWord: null, onTurn: true, isPlayer: true },
];
const log = Array.from({ length: 6 }, (_, i) => `Linha ${i + 1}.`);

function render(inputs: Record<string, unknown>) {
  const fixture = TestBed.createComponent(TvCombat);
  fixture.componentRef.setInput('order', order);
  fixture.componentRef.setInput('round', 3);
  fixture.componentRef.setInput('log', log);
  Object.entries(inputs).forEach(([k, v]) => fixture.componentRef.setInput(k, v));
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

const fogVm = (): TvMapVm => {
  const body = fogMapBody();
  return { name: body.name, hiddenByFog: false, map: body.map, vision: body.vision, revision: 1 };
};

describe('TvCombat', () => {
  it('without a map is the order beside the whole log, newest first', () => {
    const el = render({ map: null });
    expect(el.querySelector('app-tv-map')).toBeNull();
    expect(el.querySelector('.tv-combat--map')).toBeNull();
    expect(el.querySelector('app-tv-order')).not.toBeNull();
    const lines = [...el.querySelectorAll('.tv-combat__line')].map((l) => l.textContent);
    expect(lines).toEqual(log.slice(0, TV_LOG_LINES));
    expect(el.querySelector('#tv-log-title')?.textContent).toContain('Último no combate');
  });

  it('with a map is the map, the order and the log', () => {
    const el = render({ map: fogVm() });
    expect(el.querySelector('.tv-combat--map')).not.toBeNull();
    expect(el.querySelector('app-tv-map app-fog-map')).not.toBeNull();
    expect(el.querySelectorAll('app-tv-order')).toHaveLength(1);
    expect(el.querySelectorAll('.tv-combat__line')).toHaveLength(log.length);
  });

  it('with the fog hiding the map is the card, the order and the log', () => {
    const el = render({
      map: { name: '', hiddenByFog: true, map: null, vision: null, revision: 0 },
    });
    expect(el.querySelector('app-tv-fog-card')).not.toBeNull();
    expect(el.querySelector('app-tv-order')).not.toBeNull();
    expect(el.querySelector('.tv-combat__line')).not.toBeNull();
  });

  it('says "Vez do mestre" on the master’s turn', () => {
    const el = render({ map: null, mastersTurn: true });
    expect(el.querySelector('[data-testid="tv-masters-turn"]')).not.toBeNull();
  });

  it('adds the free rolls, and draws no log card without lines', () => {
    const el = render({ map: null, rolls: rollsOf, log: [] });
    expect(el.querySelector('app-tv-rolls section')).not.toBeNull();
    expect(el.querySelector('.tv-combat__log')).toBeNull();
  });
});
