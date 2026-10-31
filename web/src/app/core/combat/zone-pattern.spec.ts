import { SpellZoneVisibility } from '../../../gen/meurpg/play/v1/combat_pb';
import { spellZone } from './combat-testing';
import { ZONE_PATTERNS, zoneLegend, zonePattern } from './zone-pattern';
import { zoneView, type ZoneViewer } from './zones';

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
      zoneView(spellZone({ id: 'a' }), master),
      zoneView(spellZone({ id: 'b', spellKey: 'spell:web', spellNamePt: 'Teia' }), master),
      zoneView(spellZone({ id: 'c' }), master),
    ];
    expect(zoneLegend(views).map((i) => i.name)).toEqual(['Raio Lunar', 'Teia']);
  });

  it('says "só você vê" for a spell whose every zone is the master\'s alone', () => {
    const hidden = spellZone({
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

  it('is empty with no zone', () => {
    expect(zoneLegend([])).toEqual([]);
  });
});
