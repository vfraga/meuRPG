import type { ZoneView } from './zones';

/**
 * How each spell's zone looks on the map: a line pattern (never colour alone) and one of two inks of the map's own tokens.
 * Each pattern is one tile of 0.2 x 0.2 grid squares drawn as a stroked path, so the map and the legend's swatch draw the same
 * thing from the same data.
 */
export interface ZonePatternTile {
  readonly name: string;
  /** Stroked with round caps: a dot is a zero-length line. */
  readonly d: string;
  /** Which of the map's two inks draws it: the dark ink or the garnet accent. */
  readonly tint: 'ink' | 'accent';
}

export const ZONE_TILE = 0.2;

export const ZONE_PATTERNS: readonly ZonePatternTile[] = [
  { name: 'listras verticais', d: 'M0.1 0V0.2', tint: 'ink' },
  { name: 'pontos', d: 'M0.1 0.1h0.001', tint: 'ink' },
  {
    name: 'listras inclinadas',
    d: 'M0 0.2L0.2 0M-0.05 0.05L0.05 -0.05M0.15 0.25L0.25 0.15',
    tint: 'accent',
  },
  { name: 'ondas', d: 'M0 0.1Q0.05 0 0.1 0.1T0.2 0.1', tint: 'ink' },
  { name: 'triângulos', d: 'M0.04 0.15L0.1 0.04L0.16 0.15Z', tint: 'accent' },
  { name: 'listras horizontais', d: 'M0 0.1H0.2', tint: 'accent' },
];

/** The spells the app keeps, each with a pattern of its own so the six never share one. */
const KNOWN: Readonly<Record<string, number>> = {
  'spell:moonbeam': 0,
  'spell:web': 1,
  'spell:spirit-guardians': 2,
  'spell:cloudkill': 3,
  'spell:grease': 4,
  'spell:entangle': 5,
};

const HASH_BASE = 31;

/** The pattern of a spell's zone: fixed for the six spells of the catalog, a hash of the key for a table spell. */
export function zonePattern(spellKey: string): number {
  const known = KNOWN[spellKey];
  if (known !== undefined) {
    return known;
  }
  let h = 0;
  for (const ch of spellKey) {
    h = (h * HASH_BASE + ch.charCodeAt(0)) >>> 0;
  }
  return h % ZONE_PATTERNS.length;
}

export interface ZoneLegendItem {
  readonly key: string;
  readonly name: string;
  readonly pattern: number;
  /** Every zone of this spell on the map is the master's alone. */
  readonly masterOnly: boolean;
}

/** One legend entry per spell that has a zone on the map, in the order they were cast. */
export function zoneLegend(views: readonly ZoneView[]): ZoneLegendItem[] {
  const items = new Map<string, ZoneLegendItem>();
  for (const v of views) {
    const seen = items.get(v.spellKey);
    const hidden = v.masterOnly !== '';
    items.set(v.spellKey, {
      key: v.spellKey,
      name: v.spellName,
      pattern: zonePattern(v.spellKey),
      masterOnly: seen ? seen.masterOnly && hidden : hidden,
    });
  }
  return [...items.values()];
}
