import { SpellZoneVisibility, WallSide } from '../../../gen/meurpg/play/v1/combat_pb';
import { spellZone } from './combat-testing';
import { arrowTurn, BURNING_PATTERN, ZONE_PATTERNS, zoneLegend, zonePattern } from './zone-pattern';
import { zoneView, type ZoneViewer } from './zones';

const at = { squares: [{ col: 0, row: 0 }] };
const master: ZoneViewer = { master: true, combatants: [] };

describe('the pattern of a zone', () => {
  it('gives each of the six spells its own pattern, the same every time', () => {
    const keys = [
      'spell:moonbeam',
      'spell:web',
      'spell:cloudkill',
      'spell:spirit-guardians',
      'spell:grease',
      'spell:entangle',
    ];
    const picked = keys.map(zonePattern);
    expect(new Set(picked).size).toBe(6);
    expect(keys.map(zonePattern)).toEqual(picked);
  });

  const CATALOG = [
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

  it('gives every catalog spell its own pattern, drawing class and drawing', () => {
    const picked = CATALOG.map(zonePattern);
    expect(new Set(picked).size).toBe(CATALOG.length);
    const all = [...picked, BURNING_PATTERN].map((i) => ZONE_PATTERNS[i]);
    expect(new Set(all.map((t) => t.cls)).size).toBe(all.length);
    expect(new Set(all.map((t) => t.d)).size).toBe(all.length);
    expect(new Set(ZONE_PATTERNS.map((t) => t.cls)).size).toBe(ZONE_PATTERNS.length);
  });

  it('keeps the looks of the first six as they were', () => {
    const tint = (k: string) => ZONE_PATTERNS[zonePattern(k)];
    expect(tint('spell:moonbeam')).toMatchObject({
      cls: 'vlines',
      tint: 'ink',
      veil: { tone: 'surface', pct: 38 },
    });
    expect(tint('spell:spirit-guardians').tint).toBe('accent');
    expect(tint('spell:grease').cls).toBe('triangles');
  });

  it('veils the clouds, the Darkness and the wall as the board says', () => {
    const veil = (k: string) => ZONE_PATTERNS[zonePattern(k)].veil;
    expect(veil('spell:silence')).toEqual({ tone: 'surface', pct: 38 });
    expect(veil('spell:fog-cloud')).toEqual({ tone: 'surface', pct: 72 });
    expect(veil('spell:stinking-cloud')).toEqual({ tone: 'surface', pct: 72 });
    expect(veil('spell:darkness')).toEqual({ tone: 'ink', pct: 80 });
    expect(veil('spell:wall-of-fire')).toEqual({ tone: 'accent', pct: 72 });
    expect(ZONE_PATTERNS[zonePattern('spell:darkness')].tint).toBe('light');
    expect(ZONE_PATTERNS[zonePattern('spell:wall-of-fire')].tint).toBe('light');
  });

  it('draws the burning side with arrows, garnet and dashed', () => {
    expect(ZONE_PATTERNS[BURNING_PATTERN]).toMatchObject({
      cls: 'arrows',
      tint: 'accent',
      dashed: true,
    });
  });

  it('never hashes a table spell onto a look that veils', () => {
    for (const k of ['table:abc-123', 'table:x', 'spell:unknown', 'spell:fireball']) {
      expect(ZONE_PATTERNS[zonePattern(k)].hashable).toBe(true);
    }
  });

  it('only ever hashes a table spell onto the looks flagged hashable, none of which veils more than the first six did', () => {
    const hashable = ZONE_PATTERNS.map((t, i) => [t, i] as const).filter(([t]) => t.hashable);
    expect(hashable.length).toBeGreaterThan(0);
    for (const [t] of hashable) {
      expect(t.veil).toEqual({ tone: 'surface', pct: 38 });
      expect(t.dashed).toBe(false);
    }
    // the six spells the app began with are the hashable ones: a reorder of the table fails here
    const first = ['moonbeam', 'web', 'spirit-guardians', 'cloudkill', 'grease', 'entangle'];
    expect(new Set(first.map((k) => zonePattern(`spell:${k}`)))).toEqual(
      new Set(hashable.map(([, i]) => i)),
    );
  });

  describe('the turn of the arrows of a wall', () => {
    const run = (cols: number[], rows: number[]) => cols.map((c, i) => ({ col: c, row: rows[i] }));
    const east = run([2, 3, 4, 5, 6], [3, 3, 3, 3, 3]);

    it('points to the left or the right of the way the wall runs from its origin', () => {
      expect(arrowTurn(WallSide.LEFT, east, { col: 2, row: 3 })).toBe(270);
      expect(arrowTurn(WallSide.RIGHT, east, { col: 2, row: 3 })).toBe(90);
    });

    it('follows the way from the origin, not the order of the squares', () => {
      expect(arrowTurn(WallSide.LEFT, east, { col: 6, row: 3 })).toBe(90);
      expect(arrowTurn(WallSide.RIGHT, east, { col: 6, row: 3 })).toBe(270);
    });

    it('turns by 45 degrees for a diagonal wall', () => {
      const diagonal = run([2, 3, 4], [2, 3, 4]);
      expect(arrowTurn(WallSide.LEFT, diagonal, { col: 2, row: 2 })).toBe(315);
      expect(arrowTurn(WallSide.RIGHT, diagonal, { col: 2, row: 2 })).toBe(135);
    });

    it('still turns for a wall the fog cut to its far end', () => {
      expect(arrowTurn(WallSide.LEFT, run([5, 6], [3, 3]), { col: 5, row: 3 })).toBe(270);
    });

    it('does not guess for a wall of one square, or without an origin', () => {
      expect(arrowTurn(WallSide.LEFT, [{ col: 2, row: 3 }], { col: 2, row: 3 })).toBeNull();
      expect(arrowTurn(WallSide.LEFT, east, null)).toBeNull();
      expect(arrowTurn(WallSide.UNSPECIFIED, east, { col: 2, row: 3 })).toBeNull();
    });

    it('turns a ring on one fixed axis, right for outer and left for inner, whatever the squares', () => {
      const center = { col: 5, row: 5 };
      const full = run([5, 6, 7, 7, 7, 6, 5, 4, 3, 3, 3, 4], [3, 3, 3, 4, 5, 7, 7, 7, 7, 6, 5, 3]);
      expect(arrowTurn(WallSide.OUTER, full, center)).toBe(0);
      expect(arrowTurn(WallSide.INNER, full, center)).toBe(180);
      // a ring whose center is not seen: the origin is one of its squares, the axis stays
      expect(arrowTurn(WallSide.OUTER, full, full[0])).toBe(0);
      expect(arrowTurn(WallSide.INNER, full, full[0])).toBe(180);
    });
  });

  it('gives a table spell a pattern inside the palette', () => {
    const p = zonePattern('table:abc-123');
    expect(p).toBeGreaterThanOrEqual(0);
    expect(p).toBeLessThan(ZONE_PATTERNS.length);
    expect(zonePattern('table:abc-123')).toBe(p);
  });
});

describe('the legend of the zones on the map', () => {
  it('lists each spell once, in the order they were cast', () => {
    const views = [
      zoneView(spellZone({ ...at, id: 'a' }), master),
      zoneView(spellZone({ ...at, id: 'b', spellKey: 'spell:web', spellNamePt: 'Teia' }), master),
      zoneView(spellZone({ ...at, id: 'c' }), master),
    ];
    expect(zoneLegend(views).map((i) => i.name)).toEqual(['Raio Lunar', 'Teia']);
  });

  it('says "só você vê" for a spell whose every zone is the master\'s alone', () => {
    const hidden = spellZone({
      ...at,
      id: 'a',
      spellKey: 'spell:cloudkill',
      spellNamePt: 'Nuvem Mortal',
      visibility: SpellZoneVisibility.MASTER_ONLY,
    });
    const [item] = zoneLegend([zoneView(hidden, master)]);
    expect(item.masterOnly).toBe(true);
    const both = zoneLegend([
      zoneView(hidden, master),
      zoneView({ ...hidden, id: 'b', visibility: SpellZoneVisibility.EVERYONE }, master),
    ]);
    expect(both[0].masterOnly).toBe(false);
  });

  it("carries the letters of a spell's zones and adds the burning side of a wall under the wall's letter", () => {
    const wall = spellZone({
      ...at,
      id: 'w',
      spellKey: 'spell:wall-of-fire',
      spellNamePt: 'Muralha de Fogo',
      burningSquares: [{ col: 1, row: 1 }],
    });
    const views = [
      zoneView(spellZone({ ...at, id: 'a' }), master, 0),
      zoneView(wall, master, 1),
      zoneView(spellZone({ ...at, id: 'c' }), master, 2),
    ];
    const legend = zoneLegend(views);
    expect(legend.map((i) => [i.name, i.letters])).toEqual([
      ['Raio Lunar', ['A', 'C']],
      ['Muralha de Fogo', ['B']],
      ['Lado que queima', ['B']],
    ]);
  });

  it('marks the burning side master-only like its wall, and skips a zone with no square on the map', () => {
    const hiddenWall = spellZone({
      ...at,
      id: 'w',
      spellKey: 'spell:wall-of-fire',
      spellNamePt: 'Muralha de Fogo',
      visibility: SpellZoneVisibility.MASTER_ONLY,
      squares: [{ col: 0, row: 0 }],
      burningSquares: [{ col: 1, row: 1 }],
    });
    const ghost = spellZone({ ...at, id: 'g', squares: [] });
    const legend = zoneLegend([zoneView(ghost, master, 0), zoneView(hiddenWall, master, 1)]);
    expect(legend.map((i) => [i.name, i.letters, i.masterOnly])).toEqual([
      ['Muralha de Fogo', ['B'], true],
      ['Lado que queima', ['B'], true],
    ]);
  });

  it('is empty with no zone', () => {
    expect(zoneLegend([])).toEqual([]);
  });
});
