import { TestBed } from '@angular/core/testing';

import { combatant } from '../../core/combat/combat-testing';
import { CombatMap } from './combat-map';

function map(inputs: Record<string, unknown> = {}) {
  const fixture = TestBed.createComponent(CombatMap);
  const ref = fixture.componentRef;
  ref.setInput('image', { url: '/images/x', width: 2000, height: 1400 });
  ref.setInput('columns', 20);
  ref.setInput('rows', 14);
  ref.setInput('combatants', [
    combatant({ id: 'tor', label: 'Tor', col: 5, row: 5, mountedOnId: 'h' }),
    combatant({ id: 'h', label: 'Cavalo de guerra', col: 5, row: 5, riderId: 'tor' }),
  ]);
  for (const [k, v] of Object.entries(inputs)) {
    ref.setInput(k, v);
  }
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

describe('CombatMap: a rider on its mount (588-04)', () => {
  it('draws one token for the pair, the mount with the rider on it and the saddle', () => {
    const el = map();
    expect(el.querySelectorAll('.cm__tk')).toHaveLength(1);
    expect(el.querySelector('.cm__tk')?.getAttribute('data-id')).toBe('h');
    expect(el.querySelector('.cm__rider')?.textContent).toContain('T');
    expect(el.querySelector('app-saddle-icon')).not.toBeNull();
  });

  it('gives the pair the turn when the rider has it, and lets the rider be moved by the mount token', () => {
    const el = map({ currentId: 'tor', ownMoveId: 'tor' });
    const tk = el.querySelector('.cm__tk')!;
    expect(tk.classList).toContain('cm__tk--current');
    expect(tk.classList).toContain('cm__tk--movable');
    expect(tk.textContent).toContain('Vez');
  });

  it('draws a rider alone when its mount is not on the map for the viewer', () => {
    const el = map({
      combatants: [combatant({ id: 'tor', label: 'Tor', col: 5, row: 5, mountedOnId: '' })],
    });
    expect(el.querySelectorAll('.cm__tk')).toHaveLength(1);
    expect(el.querySelector('.cm__rider')).toBeNull();
  });

  it("puts the hidden seal of a pair top left on the master's map, clear of the rider's disc", () => {
    const el = map({
      isMaster: true,
      combatants: [
        combatant({ id: 'tor', label: 'Tor', col: 5, row: 5, mountedOnId: 'h' }),
        combatant({
          id: 'h',
          label: 'Cavalo de guerra',
          col: 5,
          row: 5,
          riderId: 'tor',
          hidden: true,
        }),
      ],
    });
    const token = el.querySelector('.cm__tk > app-combatant-token')!;
    expect(token.classList).toContain('tk--seal-left');
    const seal = token.querySelector('.tk__badge')!;
    expect(getComputedStyle(seal).left).toBe('-6px');
    expect(getComputedStyle(seal).right).toBe('auto');
  });

  it('draws one pick button for the pair, at the mount, naming both', () => {
    const mark = (id: string, label: string) =>
      ({ id, label, reason: '', note: '', count: 0, mode: 'target' }) as const;
    const el = map({ pick: [mark('tor', 'Tor'), mark('h', 'Cavalo de guerra')] });
    const buttons = el.querySelectorAll('.cm__pick');
    expect(buttons).toHaveLength(1);
    expect(buttons[0].getAttribute('data-pick')).toBe('h');
    expect(buttons[0].getAttribute('aria-label')).toBe(
      'Escolher Cavalo de guerra como alvo · leva Tor',
    );
  });

  it("keeps the rider's button when the mount is not one to pick", () => {
    const el = map({
      pick: [{ id: 'tor', label: 'Tor', reason: '', note: '', count: 0, mode: 'target' }],
    });
    expect([...el.querySelectorAll('.cm__pick')].map((b) => b.getAttribute('data-pick'))).toEqual([
      'tor',
    ]);
  });

  it("says the pair in the map's text list", () => {
    const items = [...map().querySelectorAll('ul.mr-visually-hidden li')].map((li) =>
      li.textContent?.trim(),
    );
    expect(items).toContain('Tor, montado no Cavalo de guerra, coluna 6, linha 6');
    expect(items).toContain('Cavalo de guerra, coluna 6, linha 6');
  });
});
