import { TestBed } from '@angular/core/testing';

import type { TvCombatantVm } from '../../../core/play/table-screen-view';
import { TV_ORDER_MAX } from './order-rows';
import { TvOrder } from './tv-order';

const who = (name: string, more: Partial<TvCombatantVm> = {}): TvCombatantVm => ({
  name,
  initial: name[0],
  stateWord: null,
  onTurn: false,
  isPlayer: true,
  ...more,
});

function render(order: TvCombatantVm[], round = 3, mastersTurn = false) {
  const fixture = TestBed.createComponent(TvOrder);
  fixture.componentRef.setInput('order', order);
  fixture.componentRef.setInput('round', round);
  fixture.componentRef.setInput('mastersTurn', mastersTurn);
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

describe('TvOrder', () => {
  it('names the round and puts the one on turn first, marked "Vez"', () => {
    const el = render([
      who('Brisa'),
      who('Tor', { onTurn: true }),
      who('Zumbi 1', { isPlayer: false, stateWord: 'Ferido' }),
    ]);
    expect(el.querySelector('h2')?.textContent).toContain('Ordem');
    expect(el.querySelector('.tv-order__round')?.textContent).toBe('Rodada 3');
    const names = [...el.querySelectorAll('.tv-order__name')].map((n) => n.textContent);
    expect(names).toEqual(['Tor', 'Zumbi 1', 'Brisa']);
    expect(el.querySelector('.tv-order__row--turn .tv-order__word')?.textContent).toBe('Vez');
    expect(el.querySelector('[aria-current="step"]')?.textContent).toContain('Tor');
    expect(el.querySelector('[data-testid="tv-round-line"]')?.textContent).toBe('Rodada 4 começa');
  });

  it('says one word of state, never a number, and none for a player who is up', () => {
    const el = render([
      who('Tor', { onTurn: true }),
      who('Wight', { isPlayer: false, stateWord: 'Muito ferido' }),
      who('Jabuti', { stateWord: 'Caído' }),
    ]);
    const words = [...el.querySelectorAll('.tv-order__word')].map((n) => n.textContent);
    expect(words).toEqual(['Vez', 'Muito ferido', 'Caído']);
    expect(el.textContent).not.toMatch(/\d+ ?PV|CA \d/);
  });

  it('draws NPCs as squares and a fallen one as out of the fight', () => {
    const el = render([
      who('Tor', { onTurn: true }),
      who('Zumbi', { isPlayer: false, stateWord: 'Derrotado' }),
    ]);
    const tokens = el.querySelectorAll('app-combatant-token');
    expect(tokens[1].classList.contains('tk--npc')).toBe(true);
    expect(tokens[1].classList.contains('tk--defeated')).toBe(true);
    expect(tokens[0].classList.contains('tk--defeated')).toBe(false);
  });

  it('says "Vez do mestre" when the turn is one the table does not see', () => {
    const el = render([who('Brisa'), who('Tor')], 2, true);
    expect(el.querySelector('[data-testid="tv-masters-turn"]')?.textContent).toContain(
      'Vez do mestre',
    );
    expect(el.querySelector('[data-testid="tv-round-line"]')).toBeNull();
  });

  it('keeps the room of "+N" empty while every row is drawn', () => {
    const el = render([who('Tor', { onTurn: true })]);
    expect(el.querySelector('.tv-order__more')?.textContent?.trim()).toBe('');
  });

  it('cuts a long order with "+N"', () => {
    const many = Array.from({ length: TV_ORDER_MAX + 4 }, (_, i) =>
      who(`P${i}`, { onTurn: i === 0 }),
    );
    const el = render(many);
    expect(el.querySelectorAll('.tv-order__name')).toHaveLength(TV_ORDER_MAX);
    expect(el.querySelector('.tv-order__more')?.textContent?.trim()).toBe('+4');
  });
});
