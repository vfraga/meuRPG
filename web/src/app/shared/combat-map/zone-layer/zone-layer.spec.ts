import { TestBed } from '@angular/core/testing';

import { SpellZoneVisibility, WallSide } from '../../../../gen/meurpg/play/v1/combat_pb';
import { spellZone } from '../../../core/combat/combat-testing';
import { zoneView, type ZoneView } from '../../../core/combat/zones';
import { ZoneLayer } from './zone-layer';

const square = (col: number, row: number) => ({ col, row });
function view(over: Parameters<typeof spellZone>[0]): ZoneView {
  return zoneView(
    spellZone({ squares: [square(4, 3), square(5, 3), square(4, 4), square(5, 4)], ...over }),
    { master: true, combatants: [] },
  );
}

function mount(zones: ZoneView[]) {
  TestBed.resetTestingModule();
  const fixture = TestBed.createComponent(ZoneLayer);
  fixture.componentRef.setInput('columns', 20);
  fixture.componentRef.setInput('rows', 10);
  fixture.componentRef.setInput('zones', zones);
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

describe('ZoneLayer', () => {
  it('draws nothing without a zone', () => {
    expect(mount([]).querySelector('svg')).toBeNull();
  });

  it('is decorative: the panel list is the accessible version', () => {
    const el = mount([view({ id: 'a' })]);
    expect(el.getAttribute('aria-hidden')).toBe('true');
  });

  it('draws the footprint of each zone from its squares, with a pattern and an outline', () => {
    const el = mount([
      view({ id: 'a' }),
      view({ id: 'b', spellKey: 'spell:web', spellNamePt: 'Teia', squares: [square(9, 9)] }),
    ]);
    const zones = el.querySelectorAll('g.zl__zone');
    expect(zones.length).toBe(2);
    const first = zones[0];
    expect(first.querySelector('.zl__veil')?.getAttribute('d')).toMatch(/^M/);
    expect(first.querySelector('.zl__pat')?.getAttribute('fill')).toMatch(/^url\(#.+\)$/);
    expect(el.querySelector('.zl__outline .zl__edge')?.getAttribute('d')).toMatch(/^M/);
    // two spells, two patterns
    expect(zones[1].querySelector('.zl__pat')?.getAttribute('fill')).not.toBe(
      first.querySelector('.zl__pat')?.getAttribute('fill'),
    );
  });

  it('carries no floating name on the map: each zone has a letter badge', () => {
    const el = mount([
      view({ id: 'a' }),
      { ...view({ id: 'b', spellKey: 'spell:web' }), letter: 'B' },
    ]);
    expect(el.querySelector('.zl__label')).toBeNull();
    const badges = [...el.querySelectorAll('.zl__badge')].map((b) => b.textContent?.trim());
    expect(badges).toEqual(['A', 'B']);
  });

  // The diamond is a positioned ::before: a letter left as bare text in the badge is painted under it (a white diamond with
  // no letter, #640). The letter is its own positioned element, above the diamond, in the ink token on the diamond's surface.
  it('puts each badge letter in its own element, painted above the diamond, in the ink on white', () => {
    const el = mount([
      view({ id: 'a' }),
      { ...view({ id: 'b', spellKey: 'spell:web' }), letter: 'B' },
    ]);
    const letters = [...el.querySelectorAll<HTMLElement>('.zl__badge > .zl__letter')];
    expect(letters.map((l) => l.textContent)).toEqual(['A', 'B']);
    for (const letter of letters) {
      const style = getComputedStyle(letter);
      expect(style.position).toBe('relative');
      expect(Number(style.zIndex)).toBeGreaterThanOrEqual(1);
    }
    const css = [...document.querySelectorAll('style')]
      .map((s) => s.textContent ?? '')
      .join('\n')
      .replace(/\s+/g, ' ');
    expect(css).toMatch(/\.zl__letter[^{]*\{[^}]*color: var\(--mr-map-token-ink\)/);
    expect(css).toMatch(
      /\.zl__badge[^{]*::before[^{]*\{[^}]*background: var\(--mr-map-token-surface\)/,
    );
  });

  it('draws the zone of a hidden caster dashed, with its badge dashed too', () => {
    const el = mount([view({ id: 'a', visibility: SpellZoneVisibility.MASTER_ONLY })]);
    expect(el.querySelector('g.zl__outline.zl__zone--dashed')).not.toBeNull();
    expect(el.querySelector('.zl__badge--dashed')).not.toBeNull();
  });

  it('puts the badge at the footprint corner, clamped to the map box', () => {
    const badge = mount([view({ id: 'a' })]).querySelector<HTMLElement>('.zl__badge')!;
    expect(badge.style.left).toContain('clamp(');
    expect(badge.style.left).toMatch(/^clamp\(.+, [\d.]+%, calc\(100% - .+\)\)$/);
    // inside the corner square (4,3), not centred on the corner: right of 4 and below 3 on a 20 x 10 map
    expect(Number(/([\d.]+)%/.exec(badge.style.left)?.[1])).toBeGreaterThan(20);
    expect(Number(/([\d.]+)%/.exec(badge.style.top)?.[1])).toBeGreaterThan(30);
  });

  it('keeps every badge inside the map for a zone at each corner and for a single square at (0,0)', () => {
    const at = (col: number, row: number, w: number, h: number) => {
      const squares = [];
      for (let c = 0; c < w; c++) {
        for (let r = 0; r < h; r++) {
          squares.push(square(col + c, row + r));
        }
      }
      return squares;
    };
    const zones = [
      view({ id: 'a', squares: at(0, 0, 3, 3) }),
      view({ id: 'b', squares: at(17, 0, 3, 3) }),
      view({ id: 'c', squares: at(0, 7, 3, 3) }),
      view({ id: 'd', squares: at(17, 7, 3, 3) }),
      view({ id: 'e', squares: [square(0, 0)] }),
    ];
    for (const b of mount(zones).querySelectorAll<HTMLElement>('.zl__badge')) {
      for (const v of [b.style.left, b.style.top]) {
        const pct = Number(/([\d.]+)%/.exec(v)?.[1]);
        expect(pct).toBeGreaterThanOrEqual(0);
        expect(pct).toBeLessThanOrEqual(100);
        expect(v).toMatch(/^clamp\(.+, calc\(100% - .+\)\)$/);
      }
    }
  });

  it('draws the veil under the pattern of a cloud, of the Darkness and of the wall', () => {
    for (const key of [
      'spell:fog-cloud',
      'spell:stinking-cloud',
      'spell:darkness',
      'spell:wall-of-fire',
    ]) {
      const g = mount([view({ id: 'a', spellKey: key })]).querySelector('g.zl__zone')!;
      const veil = g.querySelector('.zl__veil')!;
      expect(veil.getAttribute('d')).toMatch(/^M/);
      const kids = [...g.children];
      expect(kids.indexOf(veil)).toBeLessThan(kids.indexOf(g.querySelector('.zl__pat')!));
    }
    const dark = mount([view({ id: 'a', spellKey: 'spell:darkness' })]).querySelector('.zl__veil')!;
    expect(dark.getAttribute('fill-opacity')).toBe('0.8');
    expect(dark.classList.contains('zl__veil--ink')).toBe(true);
    const fog = mount([view({ id: 'a', spellKey: 'spell:fog-cloud' })]).querySelector('.zl__veil')!;
    expect(fog.getAttribute('fill-opacity')).toBe('0.72');
  });

  it('draws the burning side of a wall as its own layer from burningSquares, dashed', () => {
    const el = mount([
      view({
        id: 'w',
        spellKey: 'spell:wall-of-fire',
        squares: [square(4, 3), square(5, 3)],
        origin: square(4, 3),
        wallSide: WallSide.LEFT,
        burningSquares: [square(4, 2), square(5, 2)],
      }),
    ]);
    const burn = el.querySelector('g.zl__burn')!;
    expect(burn).not.toBeNull();
    expect(burn.querySelector('.zl__veil')?.getAttribute('d')).toMatch(/^M/);
    expect(burn.querySelector('.zl__edge')?.classList.contains('zl__edge--dashed')).toBe(true);
    expect(burn.querySelector('.zl__pat')?.getAttribute('fill')).toMatch(/^url\(#.+\)$/);
    // the burning side carries the wall's letter, not a badge of its own
    expect(el.querySelectorAll('.zl__badge').length).toBe(1);
  });

  it('turns the arrows with the wall side the server sent', () => {
    const el = mount([
      view({
        id: 'w',
        spellKey: 'spell:wall-of-fire',
        squares: [square(2, 3), square(3, 3), square(4, 3)],
        origin: square(2, 3),
        wallSide: WallSide.LEFT,
        burningSquares: [square(2, 2)],
      }),
    ]);
    const pattern = el.querySelector('pattern[id$="-b-w"]')!;
    expect(pattern.getAttribute('patternTransform')).toBe('rotate(270)');
  });

  it('draws both patterns, split on the diagonal, where two zones lie over each other, and keeps both outlines', () => {
    const el = mount([
      view({ id: 'a', squares: [square(1, 1), square(2, 1)] }),
      view({ id: 'b', spellKey: 'spell:web', squares: [square(2, 1), square(3, 1)] }),
    ]);
    const overlap = el.querySelector('g.zl__overlap')!;
    expect(overlap.querySelectorAll('.zl__pat').length).toBe(2);
    const fills = [...overlap.querySelectorAll('.zl__pat')].map((p) => p.getAttribute('fill'));
    expect(new Set(fills).size).toBe(2);
    expect(overlap.querySelector('.zl__pat')?.getAttribute('d')).toBe('M2 1H3L2 2Z');
    expect(el.querySelectorAll('g.zl__outline .zl__edge').length).toBe(2);
  });

  it("leaves the shared squares out of each zone's base fill, so only the triangles paint them", () => {
    const el = mount([
      view({ id: 'a', squares: [square(1, 1), square(2, 1)] }),
      view({ id: 'b', spellKey: 'spell:web', squares: [square(2, 1), square(3, 1)] }),
    ]);
    const [a, b] = [...el.querySelectorAll('g.zl__zone')].map((g) =>
      g.querySelector('.zl__veil')!.getAttribute('d'),
    );
    expect(a).toBe('M1 1h1v1h-1z');
    expect(b).toBe('M3 1h1v1h-1z');
    // the outline stays whole
    expect(el.querySelectorAll('g.zl__outline .zl__edge').length).toBe(2);
  });

  it("draws the burning side with the dashed edge and no arrows when the wall's way is unknown", () => {
    const el = mount([
      view({
        id: 'w',
        spellKey: 'spell:wall-of-fire',
        squares: [square(2, 3)],
        origin: square(2, 3),
        wallSide: WallSide.LEFT,
        burningSquares: [square(2, 2)],
      }),
    ]);
    const burn = el.querySelector('g.zl__burn')!;
    expect(burn.querySelector('.zl__edge--dashed')).not.toBeNull();
    expect(burn.querySelector('.zl__pat')).toBeNull();
    expect(el.querySelector('pattern[id$="-b-w"]')).toBeNull();
  });

  it('draws no overlap layer for zones that do not touch', () => {
    expect(mount([view({ id: 'a' })]).querySelector('g.zl__overlap')).toBeNull();
  });

  it('draws no burning layer for a zone that is no wall', () => {
    expect(mount([view({ id: 'a' })]).querySelector('g.zl__burn')).toBeNull();
  });

  it('gives each spell of the catalog a pattern of its own on the map', () => {
    const keys = [
      'spell:moonbeam',
      'spell:web',
      'spell:spirit-guardians',
      'spell:cloudkill',
      'spell:grease',
      'spell:entangle',
      'spell:silence',
      'spell:spike-growth',
      'spell:fog-cloud',
      'spell:stinking-cloud',
      'spell:darkness',
      'spell:wall-of-fire',
    ];
    const el = mount(keys.map((k, i) => view({ id: `z${i}`, spellKey: k })));
    const fills = [...el.querySelectorAll('.zl__zone .zl__pat')].map((p) => p.getAttribute('fill'));
    expect(new Set(fills).size).toBe(keys.length);
  });

  it('skips a zone with no squares', () => {
    expect(mount([view({ id: 'a', squares: [] })]).querySelector('g.zl__zone')).toBeNull();
  });
});
