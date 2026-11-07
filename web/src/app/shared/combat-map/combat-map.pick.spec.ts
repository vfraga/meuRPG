import { TestBed } from '@angular/core/testing';

import { combatant } from '../../core/combat/combat-testing';
import { CombatMap, type PickMark } from './combat-map';

const mark = (id: string, label: string, over: Partial<PickMark> = {}): PickMark => ({
  id,
  label,
  reason: '',
  note: '',
  count: 0,
  mode: 'target',
  ...over,
});

function map(inputs: Record<string, unknown>) {
  const fixture = TestBed.createComponent(CombatMap);
  const ref = fixture.componentRef;
  ref.setInput('image', { url: '/images/x', width: 2000, height: 1400 });
  ref.setInput('columns', 20);
  ref.setInput('rows', 14);
  ref.setInput('combatants', [
    combatant({ id: 'me', label: 'Pensantus', col: 3, row: 7 }),
    combatant({ id: 'g1', label: 'Goblin 1', col: 8, row: 7 }),
    combatant({ id: 'g3', label: 'Goblin 3', col: 18, row: 13 }),
  ]);
  for (const [k, v] of Object.entries(inputs)) {
    ref.setInput(k, v);
  }
  fixture.detectChanges();
  return { fixture, el: fixture.nativeElement as HTMLElement };
}

describe('CombatMap: choosing a target (C7-04)', () => {
  it('draws no button when nobody is choosing', () => {
    expect(map({}).el.querySelector('.cm__pick')).toBeNull();
  });

  it("names the master's offer candidates, and says which one is chosen", () => {
    const { el } = map({
      pick: [
        mark('g1', 'Goblin 1', { mode: 'offer', count: 1 }),
        mark('g3', 'Goblin 3', { mode: 'offer' }),
      ],
    });
    const buttons = [...el.querySelectorAll<HTMLButtonElement>('.cm__pick')];
    expect(buttons.map((b) => b.getAttribute('aria-label'))).toEqual([
      'Oferecer a Goblin 1',
      'Oferecer a Goblin 3',
    ]);
    expect(buttons.map((b) => b.getAttribute('aria-pressed'))).toEqual(['true', 'false']);
    expect(el.querySelectorAll('.cm__pick--offer').length).toBe(2);
  });

  it('makes a button of each creature offered, with its name, and nothing of the others', () => {
    const { el } = map({
      pick: [mark('g1', 'Goblin 1'), mark('g3', 'Goblin 3', { reason: 'cobertura total' })],
    });
    const buttons = [...el.querySelectorAll<HTMLButtonElement>('.cm__pick')];
    expect(buttons.map((b) => b.getAttribute('aria-label'))).toEqual([
      'Escolher Goblin 1 como alvo',
      'Goblin 3: cobertura total',
    ]);
    expect(buttons[0].getAttribute('aria-disabled')).toBeNull();
    expect(buttons[1].getAttribute('aria-disabled')).toBe('true');
    expect(el.querySelector('[data-pick="me"]')).toBeNull();
  });

  it('dims the token of a creature that cannot be chosen, and only that one', () => {
    const { el } = map({
      pick: [mark('g1', 'Goblin 1'), mark('g3', 'Goblin 3', { reason: 'fora de alcance' })],
    });
    expect(el.querySelector('[data-id="g3"]')?.classList.contains('cm__tk--dim')).toBe(true);
    expect(el.querySelector('[data-id="g1"]')?.classList.contains('cm__tk--dim')).toBe(false);
    expect(el.querySelector('[data-id="me"]')?.classList.contains('cm__tk--dim')).toBe(false);
  });

  it('tells a tap, valid or not, to the page', () => {
    const { fixture, el } = map({
      pick: [mark('g1', 'Goblin 1'), mark('g3', 'Goblin 3', { reason: 'fora de alcance' })],
    });
    const taps: string[] = [];
    fixture.componentInstance.pickTap.subscribe((id) => taps.push(id));
    el.querySelector<HTMLButtonElement>('[data-pick="g1"]')!.click();
    el.querySelector<HTMLButtonElement>('[data-pick="g3"]')!.click();
    expect(taps).toEqual(['g1', 'g3']);
  });

  it('writes "Alvo" on the chosen target, "×2" on a creature with two darts and the catch of a far one', () => {
    const one = map({
      pick: [mark('g1', 'Goblin 1', { count: 1, note: 'Longe: desvantagem' })],
    }).el;
    expect(one.querySelector('.cm__mark-n')?.textContent).toContain('Alvo');
    expect(one.querySelector('.cm__far')?.textContent).toBe('Longe: desvantagem');
    const darts = map({ pick: [mark('g1', 'Goblin 1', { count: 2, mode: 'darts' })] }).el;
    expect(darts.querySelector('.cm__mark-n')?.textContent?.replace(/\s+/g, '')).toBe('×2');
    expect(darts.querySelector('.cm__pick')?.getAttribute('aria-label')).toContain(
      'Pôr outro dardo',
    );
  });

  it('puts the catch chip below its token, above at the foot of the map, and a gutter around the map', () => {
    const { el } = map({ pick: [mark('g1', 'Goblin 1', { note: 'Longe: desvantagem' })] });
    expect(el.querySelector('.cm__far')?.className).not.toContain('cm__far--above');
    expect(el.querySelector('.cm--pick')).not.toBeNull();
  });

  it('shortens two catch chips to "Longe"', () => {
    const { fixture, el } = map({
      pick: [
        mark('g1', 'Goblin 1', { note: 'Longe: desvantagem' }),
        mark('g4', 'Goblin 4', { note: 'Longe: desvantagem' }),
      ],
    });
    fixture.componentRef.setInput('combatants', [
      combatant({ id: 'g1', label: 'Goblin 1', col: 8, row: 7 }),
      combatant({ id: 'g4', label: 'Goblin 4', col: 9, row: 7 }),
    ]);
    fixture.detectChanges();
    const chips = [...el.querySelectorAll('.cm__far')];
    expect(chips.map((c) => c.textContent)).toEqual(['Longe', 'Longe']);
    expect(el.querySelector('.cm__pick')?.getAttribute('aria-label')).toContain('desvantagem');
  });

  it('shows why a creature cannot be chosen, as a chip and in a polite status', () => {
    const { el } = map({
      pick: [mark('g3', 'Goblin 3', { reason: 'cobertura total' })],
      pickNote: { id: 'g3', text: 'Goblin 3: cobertura total' },
    });
    expect(el.querySelector('.cm__why')?.textContent).toContain('Goblin 3: cobertura total');
    expect(el.querySelector('[role="status"]')?.textContent).toBe('Goblin 3: cobertura total');
  });
});

describe('CombatMap: a tap picks the creature whose square is under the pointer', () => {
  // 9 px squares on a 40 x 30 map: the 44 px buttons of neighbouring swords reach over each other.
  function small() {
    const { fixture, el } = map({
      columns: 40,
      rows: 30,
      combatants: [
        combatant({ id: 's1', label: 'Espada voadora 1', col: 20, row: 12 }),
        combatant({ id: 's2', label: 'Espada voadora 2', col: 21, row: 12 }),
        combatant({ id: 'arm', label: 'Armadura animada', col: 21, row: 13, size: 4 }),
      ],
      pick: [
        mark('s1', 'Espada voadora 1'),
        mark('s2', 'Espada voadora 2'),
        mark('arm', 'Armadura animada'),
      ],
    });
    const inner = fixture.componentInstance as unknown as { layer: { unit: () => number } };
    inner.layer.unit = () => 9;
    const surface = el.querySelector<HTMLElement>('.cm__surface')!;
    surface.getBoundingClientRect = () => ({ left: 0, top: 0, width: 360, height: 270 }) as DOMRect;
    const taps: string[] = [];
    fixture.componentInstance.pickTap.subscribe((id) => taps.push(id));
    const click = (id: string, x: number, y: number, detail = 1) =>
      el
        .querySelector<HTMLButtonElement>(`[data-pick="${id}"]`)!
        .dispatchEvent(new MouseEvent('click', { bubbles: true, clientX: x, clientY: y, detail }));
    return { taps, click };
  }

  it('sends the tap on a sword to that sword even when the button of its neighbour is on top', () => {
    const { taps, click } = small();
    // Square 21,12 is x 189-198, y 108-117: the button of the armor (the last one) is over it, and so is sword 1's.
    click('arm', 193, 112);
    click('s1', 193, 112);
    expect(taps).toEqual(['s2', 's2']);
  });

  it('sends a tap on a square of the big creature to it', () => {
    const { taps, click } = small();
    click('s2', 21 * 9 + 5, 14 * 9 + 5);
    expect(taps).toEqual(['arm']);
  });

  it('keeps the button of the keyboard: Enter on a button picks that creature', () => {
    const { taps, click } = small();
    click('s1', 0, 0, 0);
    expect(taps).toEqual(['s1']);
  });
});
