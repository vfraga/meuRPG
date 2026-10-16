import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';

import {
  AreaPlacement,
  AreaTargetSchema,
  CombatantKind,
  PreviewSpellAreaResponseSchema,
  TargetInReachSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { SpellAreaShape } from '../../../gen/meurpg/rules/v1/rules_pb';
import type { AreaPick } from '../../core/combat/area-flow';
import { combatant } from '../../core/combat/combat-testing';
import { AreaMap } from './area-map';

// With --coverage test-setup.ts only hooks the first file of a worker, so the fake clock this file leaves on would
// reach the next file (a real setTimeout never fires on a fake clock).
afterEach(() => {
  vi.useRealTimers();
});

/** Each square is 20 px on the stubbed 240 × 160 map (12 × 8 squares). */
const SQ = 20;

const plain = (t: string | null | undefined) =>
  (t ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

const caster = combatant({
  id: 'p',
  label: 'Pensantus',
  kind: CombatantKind.PLAYER,
  mine: true,
  col: 2,
  row: 4,
});
const goblin = combatant({ id: 'g1', label: 'Goblin 1', col: 6, row: 4 });
const hidden = combatant({ id: 'g3', label: 'Goblin 3', col: 8, row: 2, hidden: true });

const fireball = { shape: SpellAreaShape.SPHERE, sizeFt: 20, widthFt: 0 };
const cone = { shape: SpellAreaShape.CONE, sizeFt: 15, widthFt: 0 };

function setup(
  over: {
    placement?: AreaPlacement;
    area?: typeof fireball;
    rangeFt?: number;
    master?: boolean;
    readOnly?: boolean;
    columns?: number;
  } = {},
) {
  const fixture = TestBed.createComponent(AreaMap);
  const ref = fixture.componentRef;
  ref.setInput('image', { url: 'map.png', width: 1200, height: 800 });
  ref.setInput('columns', over.columns ?? 12);
  ref.setInput('rows', 8);
  ref.setInput('combatants', [caster, goblin, hidden]);
  ref.setInput('casterId', 'p');
  ref.setInput(
    'spell',
    over.placement === AreaPlacement.DIRECTION ? 'Mãos Flamejantes' : 'Bola de Fogo',
  );
  ref.setInput('placement', over.placement ?? AreaPlacement.POINT);
  ref.setInput('area', over.area ?? fireball);
  ref.setInput('rangeFt', over.rangeFt ?? 150);
  ref.setInput('master', over.master ?? false);
  ref.setInput('readOnly', over.readOnly ?? false);
  ref.setInput('seen', [
    create(TargetInReachSchema, { combatantId: 'g1', label: 'Goblin 1', distanceFt: 20 }),
    create(TargetInReachSchema, { combatantId: 'p', label: 'Pensantus', distanceFt: 0 }),
  ]);
  const places: (AreaPick | null)[] = [];
  let confirms = 0;
  let cancels = 0;
  fixture.componentInstance.place.subscribe((p) => {
    places.push(p);
    ref.setInput('placed', p);
  });
  fixture.componentInstance.confirm.subscribe(() => confirms++);
  fixture.componentInstance.dismiss.subscribe(() => cancels++);
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const app = el.querySelector<HTMLElement>('[role="application"]')!;
  app.getBoundingClientRect = () =>
    ({ left: 0, top: 0, width: 12 * SQ, height: 8 * SQ, right: 240, bottom: 160 }) as DOMRect;
  const pointer = (type: string, col: number, row: number, kind = 'mouse', lift = 0) => {
    const ev = new MouseEvent(type, {
      bubbles: true,
      clientX: col * SQ + SQ / 2,
      clientY: row * SQ + SQ / 2 + lift,
      button: 0,
    });
    Object.defineProperty(ev, 'pointerType', { value: kind });
    Object.defineProperty(ev, 'pointerId', { value: 7 });
    app.dispatchEvent(ev);
    fixture.detectChanges();
  };
  const key = (k: string, shiftKey = false) => {
    app.dispatchEvent(new KeyboardEvent('keydown', { key: k, shiftKey, bubbles: true }));
    fixture.detectChanges();
  };
  return {
    fixture,
    el,
    app,
    places,
    pointer,
    key,
    confirms: () => confirms,
    cancels: () => cancels,
  };
}

const edge = (el: HTMLElement) => el.querySelector('.am__edge');
const cellCount = (el: HTMLElement) =>
  (el.querySelector('.am__fill')?.getAttribute('d') ?? '').split('M').length - 1;

describe('AreaMap: placing a point', () => {
  it('is a named, focusable application, and says how to place before anything is placed', () => {
    const { el, app, places } = setup();
    expect(app.getAttribute('aria-label')).toBe('Mapa: escolha o ponto da Bola de Fogo');
    expect(app.tabIndex).toBe(0);
    expect(plain(el.querySelector('.hint')?.textContent)).toBe(
      'Toque no mapa ou arraste para escolher o ponto. A esfera tem 6 m de raio (4 quadrados).',
    );
    expect(cellCount(el)).toBe(0);
    expect(places).toEqual([]);
  });

  it('draws the dashed template under the pointer without placing, and places it on a click', () => {
    const { el, places, pointer } = setup();
    pointer('pointermove', 6, 4);
    expect(edge(el)?.classList).toContain('am__edge--moving');
    expect(el.querySelector('.am__fill--on')).toBeNull();
    expect(places).toEqual([]);
    pointer('pointerup', 6, 4);
    expect(places).toEqual([{ kind: 'point', square: { col: 6, row: 4 } }]);
    expect(el.querySelector('.am__fill--on')).not.toBeNull();
    expect(el.querySelector('.am__diamond')).not.toBeNull();
    expect(plain(el.querySelector('.am__length')?.textContent)).toBe('6,0 m');
    expect(plain(el.querySelector('.hint')?.textContent)).toBe(
      'Ponto a 6,0 m de você. Toque de novo no mesmo ponto ou use “Confirmar local”.',
    );
  });

  it('confirms on a second tap on the same point', () => {
    const { places, pointer, confirms } = setup();
    pointer('pointerup', 6, 4);
    pointer('pointerup', 6, 4);
    expect(places).toHaveLength(1);
    expect(confirms()).toBe(1);
  });

  it('follows a finger 44 px above it, and places where the template is when the finger lifts', () => {
    const { el, places, pointer } = setup();
    pointer('pointerdown', 6, 4, 'touch', 44);
    pointer('pointermove', 7, 4, 'touch', 44);
    expect(el.querySelector('.am__finger')).not.toBeNull();
    expect(places).toEqual([]);
    pointer('pointerup', 7, 4, 'touch', 44);
    expect(places).toEqual([{ kind: 'point', square: { col: 7, row: 4 } }]);
    expect(el.querySelector('.am__finger')).toBeNull();
  });

  it("dims the squares beyond the range and refuses a player's tap there, saying why", () => {
    const { el, places, pointer } = setup({ rangeFt: 25 });
    expect(el.querySelector('.am__dim')).not.toBeNull();
    pointer('pointerup', 11, 4);
    expect(places).toEqual([null]);
    expect(plain(el.querySelector('[role="alert"] p')?.textContent)).toBe(
      'Fora do alcance. A Bola de Fogo vai até 7,5 m; esse ponto está a 13,5 m.',
    );
    expect(plain(el.textContent)).toContain('Fora do alcance (7,5 m)');
  });

  it('does not hold the master to the range: the dimming is only a hint', () => {
    const { el, places, pointer } = setup({ rangeFt: 25, master: true });
    expect(el.querySelector('.am__dim')).not.toBeNull();
    pointer('pointerup', 11, 4);
    expect(places).toEqual([{ kind: 'point', square: { col: 11, row: 4 } }]);
    expect(el.querySelector('[role="alert"]')).toBeNull();
    // The master sees the hidden one marked in the legend.
    expect(el.textContent).toContain('Escondido (só você vê)');
  });

  it("draws the server's squares and its point when a wall moved it, with an ✕ where the tap was", () => {
    const { fixture, el, pointer } = setup();
    pointer('pointerup', 9, 4);
    fixture.componentRef.setInput(
      'preview',
      create(PreviewSpellAreaResponseSchema, {
        origin: { col: 4, row: 4 },
        squares: [
          { col: 4, row: 4 },
          { col: 5, row: 4 },
        ],
        moved: true,
      }),
    );
    fixture.detectChanges();
    expect(cellCount(el)).toBe(2);
    const diamond = el.querySelector<HTMLElement>('.am__diamond')!;
    expect(diamond.style.left).toBe(`${(4.5 / 12) * 100}%`);
    const tapped = el.querySelector<HTMLElement>('.am__tapped')!;
    expect(tapped.style.left).toBe(`${(9.5 / 12) * 100}%`);
  });

  it('takes no tap and no key in step 2, where it only shows the area', () => {
    const { el, places, pointer, key } = setup({ readOnly: true });
    pointer('pointerup', 6, 4);
    key('Enter');
    expect(places).toEqual([]);
    expect(el.querySelector('.center__btn')).toBeNull();
    expect(el.querySelector('.hint')).toBeNull();
  });

  it('moves the placed point one square a touch with "Ajustar o ponto"', () => {
    const { el, places, pointer } = setup();
    pointer('pointerup', 6, 4);
    const right = el.querySelector<HTMLButtonElement>('[aria-label="Um quadrado para a direita"]')!;
    right.click();
    expect(places.at(-1)).toEqual({ kind: 'point', square: { col: 7, row: 4 } });
    expect(right.getBoundingClientRect).toBeDefined();
  });
});

describe('AreaMap: a cone, a line or a cube out of the caster', () => {
  it('takes the nearest of eight directions from a tap, and leaves the caster square out', () => {
    const { el, app, places, pointer } = setup({ placement: AreaPlacement.DIRECTION, area: cone });
    expect(app.getAttribute('aria-label')).toBe('Mapa: escolha a direção da Mãos Flamejantes');
    pointer('pointerup', 9, 5);
    expect(places).toEqual([{ kind: 'direction', direction: { dx: 1, dy: 0 } }]);
    expect(cellCount(el)).toBe(7);
    expect(el.querySelector('.am__fill')?.getAttribute('d')).not.toContain('M2 4h1');
    expect(el.querySelector('.am__length')).toBeNull();
    expect(plain(el.querySelector('.hint')?.textContent)).toContain('Direção: leste.');
  });

  it('turns the direction 45 degrees with the arrows, and Enter places then confirms', () => {
    const { places, key, confirms } = setup({ placement: AreaPlacement.DIRECTION, area: cone });
    key('ArrowRight');
    key('ArrowRight');
    key('Enter');
    expect(places).toEqual([{ kind: 'direction', direction: { dx: 1, dy: 1 } }]);
    key('Enter');
    expect(confirms()).toBe(1);
  });
});

describe('AreaMap: the keyboard', () => {
  it('moves the point a square with an arrow and five with Shift, from the caster', () => {
    const { places, key } = setup();
    key('ArrowRight');
    key('ArrowRight', true);
    key('Enter');
    expect(places).toEqual([{ kind: 'point', square: { col: 8, row: 4 } }]);
  });

  it('updates the "Ponto a … de você" hint as the point moves by keyboard, Shift included', () => {
    const { fixture, el, key } = setup();
    const hint = () => plain(el.querySelector('.hint')?.textContent);
    expect(hint()).toContain('Toque no mapa ou arraste');
    key('ArrowRight');
    fixture.detectChanges();
    expect(hint()).toContain('Ponto a 1,5 m de você');
    key('ArrowRight', true);
    fixture.detectChanges();
    expect(hint()).toContain('Ponto a 9,0 m de você');
  });

  it('cancels with Escape', () => {
    const { key, cancels } = setup();
    key('Escape');
    expect(cancels()).toBe(1);
  });

  it('announces a move after a pause, with the count only after a preview, and never a name', () => {
    vi.useFakeTimers();
    const { fixture, el, key } = setup();
    const status = () => plain(el.querySelector('p[role="status"]')?.textContent);
    key('ArrowRight');
    expect(status()).toBe('');
    vi.advanceTimersByTime(300);
    fixture.detectChanges();
    expect(status()).toBe('Ponto a 1,5 m de você');
    key('Enter');
    fixture.componentRef.setInput(
      'preview',
      create(PreviewSpellAreaResponseSchema, {
        origin: { col: 3, row: 4 },
        targets: [create(AreaTargetSchema, { combatantId: 'g1', label: 'Goblin 1' })],
      }),
    );
    fixture.detectChanges();
    vi.advanceTimersByTime(300);
    fixture.detectChanges();
    expect(status()).toBe('Ponto a 1,5 m de você, 1 criatura na área');
    expect(status()).not.toContain('Goblin');
  });

  it('opens "Centrar em…" with C: the creatures the caster sees, the caster and the arrows; a pick places there', async () => {
    const { fixture, el, places, key } = setup();
    key('c');
    await fixture.whenStable();
    const list = el.querySelector('[role="listbox"]')!;
    expect(list.getAttribute('aria-label')).toBe('Centrar em…');
    const options = Array.from(list.querySelectorAll('[role="option"]')).map((o) =>
      plain(
        `${o.querySelector('.center__name')?.textContent} ${o.querySelector('.center__detail')?.textContent ?? ''}`,
      ),
    );
    expect(options).toEqual([
      'Goblin 1 a 6,0 m',
      'Pensantus (você) aqui',
      'Um quadrado do mapa (use as setas)',
    ]);
    // The hidden Goblin 3 is not a creature the caster sees: it is never offered.
    expect(list.textContent).not.toContain('Goblin 3');
    list.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
    fixture.detectChanges();
    expect(places).toEqual([{ kind: 'point', square: { col: 6, row: 4 } }]);
    expect(el.querySelector('[role="listbox"]')).toBeNull();
  });

  it('points a direction spell at a creature from "Apontar para…", with no caster entry', async () => {
    const { fixture, el, places } = setup({ placement: AreaPlacement.DIRECTION, area: cone });
    el.querySelector<HTMLButtonElement>('.center__btn')!.click();
    fixture.detectChanges();
    await fixture.whenStable();
    const list = el.querySelector('[role="listbox"]')!;
    expect(list.getAttribute('aria-label')).toBe('Apontar para…');
    expect(list.textContent).not.toContain('(você)');
    (list.querySelector('[role="option"]') as HTMLElement).click();
    fixture.detectChanges();
    expect(places).toEqual([{ kind: 'direction', direction: { dx: 1, dy: 0 } }]);
  });
});

describe('AreaMap: the zoom of a big map', () => {
  const buttons = (el: HTMLElement) =>
    el.querySelector('[role="group"][aria-label="Zoom do mapa"]');

  it('offers no zoom on a map that fits, and no zoom to a read-only one', () => {
    expect(buttons(setup().el)).toBeNull();
    expect(buttons(setup({ columns: 60, readOnly: true }).el)).toBeNull();
  });

  it('zooms a map of 60 columns in and out by steps, the first one fitting the screen', () => {
    const { fixture, el } = setup({ columns: 60 });
    const group = buttons(el)!;
    expect(group).not.toBeNull();
    const scroller = () => el.querySelector('.scroller')!;
    const zoomIn = group.querySelector<HTMLButtonElement>('[aria-label="Aproximar o mapa"]')!;
    const zoomOut = group.querySelector<HTMLButtonElement>('[aria-label="Afastar o mapa"]')!;
    expect(scroller().classList.contains('scroller--zoomed')).toBe(false);
    expect(zoomOut.getAttribute('aria-disabled')).toBe('true');
    zoomIn.click();
    fixture.detectChanges();
    expect(scroller().classList.contains('scroller--zoomed')).toBe(true);
    expect(zoomOut.getAttribute('aria-disabled')).toBeNull();
    for (let i = 0; i < 5; i++) {
      zoomIn.click();
    }
    fixture.detectChanges();
    expect(zoomIn.getAttribute('aria-disabled')).toBe('true');
    for (let i = 0; i < 5; i++) {
      zoomOut.click();
    }
    fixture.detectChanges();
    expect(scroller().classList.contains('scroller--zoomed')).toBe(false);
  });
});
