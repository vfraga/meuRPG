import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';

import {
  AreaPlacement,
  CombatantKind,
  WallSide,
  PreviewSpellAreaResponseSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { SpellAreaShape } from '../../../gen/meurpg/rules/v1/rules_pb';
import type { AreaPick } from '../../core/combat/area-flow';
import { combatant } from '../../core/combat/combat-testing';
import type { Square } from '../../core/combat/combat-grid';
import type { Direction } from '../../core/combat/spell-area';
import { type WallChoice, newWall } from '../../core/combat/spell-wall';
import { AreaMap } from './area-map';

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

const laid = create(PreviewSpellAreaResponseSchema, {
  origin: { col: 4, row: 2 },
  squares: [
    { col: 4, row: 2 },
    { col: 5, row: 2 },
    { col: 6, row: 2 },
  ],
  burningSquares: [
    { col: 4, row: 3 },
    { col: 5, row: 3 },
  ],
  lengthFt: 15,
});

const picked = { ...newWall(false), side: WallSide.RIGHT };

function setup(wall: WallChoice | null = newWall(false), over: { master?: boolean } = {}) {
  const fixture = TestBed.createComponent(AreaMap);
  const ref = fixture.componentRef;
  ref.setInput('image', { url: 'map.png', width: 1200, height: 800 });
  ref.setInput('columns', 12);
  ref.setInput('rows', 8);
  ref.setInput('combatants', [caster]);
  ref.setInput('casterId', 'p');
  ref.setInput('spell', 'Muralha de Fogo');
  ref.setInput('placement', AreaPlacement.POINT);
  ref.setInput('area', { shape: SpellAreaShape.LINE, sizeFt: 60, widthFt: 5 });
  ref.setInput('rangeFt', 120);
  ref.setInput('master', over.master ?? false);
  ref.setInput('wall', wall);
  const places: (AreaPick | null)[] = [];
  const aimed: { square: Square; direction: Direction }[] = [];
  fixture.componentInstance.place.subscribe((p) => {
    places.push(p);
    ref.setInput('placed', p);
  });
  fixture.componentInstance.aimed.subscribe((a) => aimed.push(a));
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const app = el.querySelector<HTMLElement>('[role="application"]')!;
  app.getBoundingClientRect = () =>
    ({ left: 0, top: 0, width: 12 * SQ, height: 8 * SQ, right: 240, bottom: 160 }) as DOMRect;
  const pointer = (type: string, col: number, row: number, kind = 'mouse') => {
    const ev = new MouseEvent(type, {
      bubbles: true,
      clientX: col * SQ + SQ / 2,
      clientY: row * SQ + SQ / 2,
      button: 0,
    });
    Object.defineProperty(ev, 'pointerType', { value: kind });
    Object.defineProperty(ev, 'pointerId', { value: 7 });
    app.dispatchEvent(ev);
    fixture.detectChanges();
  };
  return { fixture, ref, el, places, aimed, pointer };
}

describe('AreaMap: a wall', () => {
  it('says how to lay a line, and a ring, and names the map by its start or its center', () => {
    const line = setup();
    expect(plain(line.el.querySelector('.hint')?.textContent)).toContain(
      'Toque no começo e arraste para onde vai',
    );
    expect(line.el.querySelector('[role="application"]')?.getAttribute('aria-label')).toBe(
      'Mapa: escolha o começo da Muralha de Fogo',
    );
    const ring = setup(newWall(true));
    expect(plain(ring.el.querySelector('.hint')?.textContent)).toContain('Toque no centro do anel');
    expect(ring.el.querySelector('[role="application"]')?.getAttribute('aria-label')).toBe(
      'Mapa: escolha o centro da Muralha de Fogo',
    );
  });

  it('draws what the server laid: the wall in bricks and the burning side in arrows, from its squares', () => {
    const { ref, el, fixture } = setup(picked);
    ref.setInput('placed', { kind: 'point', square: { col: 4, row: 2 } });
    fixture.detectChanges();
    expect(el.querySelector('app-zone-layer svg')).toBeNull();
    ref.setInput('preview', laid);
    fixture.detectChanges();
    const layer = el.querySelector('app-zone-layer')!;
    const paths = Array.from(layer.querySelectorAll('path.zl__pat')).map((p) =>
      p.getAttribute('d'),
    );
    expect(paths).toHaveLength(2);
    expect(layer.querySelector('.zl__burn')).not.toBeNull();
    expect(layer.querySelectorAll('.zl__badge')).toHaveLength(0);
    // The browser's own template is not drawn over the server's wall.
    expect(el.querySelector('.am__fill')?.getAttribute('d') ?? '').toBe('');
  });

  it('draws the wall but not the burning side until the side is picked', () => {
    const { ref, el, fixture } = setup();
    ref.setInput('placed', { kind: 'point', square: { col: 4, row: 2 } });
    ref.setInput('preview', laid);
    fixture.detectChanges();
    expect(el.querySelector('app-zone-layer svg')).not.toBeNull();
    expect(el.querySelector('app-zone-layer .zl__burn')).toBeNull();
    expect(el.querySelectorAll('app-zone-layer path.zl__pat')).toHaveLength(1);
  });

  it('marks the start (or the center) with a square, not the diamond, and names it in the legend', () => {
    const { ref, el, fixture } = setup(picked);
    ref.setInput('placed', { kind: 'point', square: { col: 4, row: 2 } });
    ref.setInput('preview', laid);
    fixture.detectChanges();
    expect(el.querySelector('.am__start')).not.toBeNull();
    expect(el.querySelector('.am__diamond')).toBeNull();
    const legend = plain(el.querySelector('.mr-legend')?.textContent);
    expect(legend).toContain('Muralha');
    expect(legend).toContain('Lado que queima');
    expect(legend).toContain('Começo');
    const ring = setup(newWall(true));
    expect(plain(ring.el.querySelector('.mr-legend')?.textContent)).toContain('Centro');
  });

  it('a tap places the start; a drag to another square places it and gives the direction (one of eight)', () => {
    const { places, aimed, pointer } = setup();
    pointer('pointerdown', 3, 4);
    pointer('pointerup', 3, 4);
    expect(places).toEqual([{ kind: 'point', square: { col: 3, row: 4 } }]);
    expect(aimed).toEqual([]);
    pointer('pointerdown', 3, 4, 'touch');
    pointer('pointermove', 7, 4, 'touch');
    pointer('pointerup', 7, 1, 'touch');
    expect(aimed).toEqual([{ square: { col: 3, row: 4 }, direction: { dx: 1, dy: -1 } }]);
  });

  it('a drag on a ring is only a tap on its center, and has no direction', () => {
    const { places, aimed, pointer } = setup(newWall(true));
    pointer('pointerdown', 5, 5);
    pointer('pointerup', 8, 5);
    expect(aimed).toEqual([]);
    expect(places).toEqual([{ kind: 'point', square: { col: 8, row: 5 } }]);
  });

  it('a drag that starts out of reach places nothing and says why, but not for the master', () => {
    const far = setup();
    far.ref.setInput('rangeFt', 10);
    far.fixture.detectChanges();
    far.pointer('pointerdown', 10, 4);
    far.pointer('pointerup', 11, 4);
    expect(far.aimed).toEqual([]);
    expect(far.places).toEqual([null]);
    expect(far.el.querySelector('[role="alert"]')).not.toBeNull();
    const master = setup(newWall(false), { master: true });
    master.ref.setInput('rangeFt', 10);
    master.fixture.detectChanges();
    master.pointer('pointerdown', 10, 4);
    master.pointer('pointerup', 11, 4);
    expect(master.aimed).toHaveLength(1);
  });

  it('draws nothing of a wall for any other spell', () => {
    const { el } = setup(null);
    expect(el.querySelector('app-zone-layer')).toBeNull();
    expect(plain(el.querySelector('.mr-legend')?.textContent)).toContain('Área da magia');
  });

  it('keeps the start on the placed square when the mouse passes over another one', () => {
    const { ref, el, fixture, pointer } = setup();
    ref.setInput('placed', { kind: 'point', square: { col: 4, row: 2 } });
    ref.setInput('preview', laid);
    fixture.detectChanges();
    pointer('pointermove', 9, 6);
    const start = el.querySelector<HTMLElement>('.am__start')!;
    expect(start.style.left).toBe('37.5%');
    expect(el.querySelector('app-zone-layer svg')).not.toBeNull();
  });

  it('keeps the last wall drawn while the next answer is on its way, and drops it with the start', () => {
    const { ref, el, fixture } = setup(picked);
    ref.setInput('placed', { kind: 'point', square: { col: 4, row: 2 } });
    ref.setInput('preview', laid);
    fixture.detectChanges();
    ref.setInput('preview', null);
    fixture.detectChanges();
    expect(el.querySelectorAll('app-zone-layer path.zl__pat')).toHaveLength(2);
    ref.setInput('placed', null);
    fixture.detectChanges();
    expect(el.querySelector('app-zone-layer svg')).toBeNull();
  });

  it('says the direction of a drag to a screen reader', () => {
    vi.useFakeTimers();
    const { el, fixture, pointer } = setup();
    pointer('pointerdown', 3, 4);
    pointer('pointerup', 7, 4);
    vi.advanceTimersByTime(300);
    fixture.detectChanges();
    expect(plain(el.querySelector('p[role="status"]')?.textContent)).toContain('direção leste');
  });

  // Focusing the map when the sheet opens must not scroll the sheet's body to it: that hid the steps and the slot row above the map
  // (the body opened scrolled by 148 px at 390 px, #640).
  it('takes the focus when it opens without scrolling anything to it', async () => {
    const focus = vi.spyOn(HTMLElement.prototype, 'focus');
    const fixture = TestBed.createComponent(AreaMap);
    const ref = fixture.componentRef;
    ref.setInput('image', { url: 'map.png', width: 1200, height: 800 });
    ref.setInput('columns', 12);
    ref.setInput('rows', 8);
    ref.setInput('combatants', [caster]);
    ref.setInput('casterId', 'p');
    ref.setInput('spell', 'Muralha de Fogo');
    ref.setInput('placement', AreaPlacement.POINT);
    ref.setInput('area', { shape: SpellAreaShape.LINE, sizeFt: 60, widthFt: 5 });
    ref.setInput('autofocus', true);
    fixture.detectChanges();
    await fixture.whenStable();
    const app = fixture.nativeElement.querySelector('[role="application"]');
    const call = focus.mock.calls.findIndex((_, i) => focus.mock.contexts[i] === app);
    expect(call).toBeGreaterThanOrEqual(0);
    expect(focus.mock.calls[call][0]).toEqual({ preventScroll: true });
    focus.mockRestore();
  });
});
