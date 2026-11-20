import type { CastingGrant, CastingSpell } from '../../../gen/meurpg/play/v1/casting_pb';
import { aboveOwnLevel } from '../combat/combat-grid';
import { abilityLabel } from '../characters/character-labels';
import type { AbilityKey } from '../characters/characters.types';
import type { CastWay } from './cast-out-flow';

/**
 * The ways to cast a spell that a granted_spell effect gives (MR-025), beside the class's own (a slot, a ritual). Each grant
 * is a way of its own: no slot, a use of its resource unless it is at will; "com espaço" when the grant also allows a slot;
 * a ritual-only grant is the ritual way. The server decides what is allowed; this only words it.
 */

/** A way to cast, as the sheet picks it: the class's (slot, ritual) or a grant's. */
export interface WayPick {
  /** "slot", "ritual", "grant:<source>" or "grant-slot:<source>". */
  readonly id: string;
  /** The cast the way makes: with a slot (or none for a cantrip and a grant's use), or a ritual. */
  readonly way: CastWay;
  /** The grant the cast goes through; `null` for the class's own. */
  readonly grant: CastingGrant | null;
  /** Whether the cast needs a slot (a leveled spell the class's way, or the grant's slot way). */
  readonly slot: boolean;
}

const CLASS_SLOT = 'slot';
const CLASS_RITUAL = 'ritual';

function grantPick(g: CastingGrant, withSlot: boolean): WayPick {
  return {
    id: `${withSlot ? 'grant-slot' : 'grant'}:${g.sourceKey}`,
    way: g.ritualOnly ? 'ritual' : 'slot',
    grant: g,
    slot: withSlot,
  };
}

/** Every way the spell can be cast, the class's first, then each grant (and its slot way when it allows one). */
export function wayPicksOf(s: CastingSpell): WayPick[] {
  const out: WayPick[] = [];
  if (s.canCast) {
    out.push({ id: CLASS_SLOT, way: 'slot', grant: null, slot: true });
  }
  if (s.ritualAllowed) {
    out.push({ id: CLASS_RITUAL, way: 'ritual', grant: null, slot: false });
  }
  for (const g of s.grants) {
    out.push(grantPick(g, false));
    if (g.alsoWithSlot && !g.ritualOnly) {
      out.push(grantPick(g, true));
    }
  }
  return out;
}

/** The way with this id, or `null`. */
export function pickOfWay(s: CastingSpell, id: string): WayPick | null {
  return wayPicksOf(s).find((p) => p.id === id) ?? null;
}

/** The way a spell opens with: the class's slot, else its ritual, else the first grant that works now. */
export function defaultWayId(s: CastingSpell): string {
  const picks = wayPicksOf(s);
  return (
    (picks.find((p) => p.grant === null) ?? picks.find((p) => p.grant?.ready) ?? picks[0])?.id ??
    CLASS_SLOT
  );
}

/** Whether some way works now: a slot or a ritual of the class, or a grant that is at will, has a use left or allows a free slot. */
export function castableNow(s: CastingSpell): boolean {
  return (
    s.canCast ||
    s.ritualAllowed ||
    s.grants.some((g) => g.ready || (g.alsoWithSlot && s.slots.length > 0))
  );
}

/** "À vontade", "1 uso restante", "2 usos restantes", "sem usos". */
export function grantUsesText(g: CastingGrant): string {
  if (g.atWill) {
    return 'À vontade';
  }
  if (g.usesLeft <= 0) {
    return 'sem usos';
  }
  return g.usesLeft === 1 ? '1 uso restante' : `${g.usesLeft} usos restantes`;
}

/** The numbers a grant gives the spell: "Carisma · CD 14 · ataque +6". */
export function grantNumbers(g: CastingGrant): string {
  const attack = g.attackBonus >= 0 ? `+${g.attackBonus}` : `−${Math.abs(g.attackBonus)}`;
  return `${abilityLabel(g.ability as AbilityKey)} · CD ${g.saveDc} · ataque ${attack}`;
}

/** The card of a grant's way ("Legado · À vontade"); a grant with no use left is off, unless it is cast with a slot. */
export function grantCard(
  p: WayPick,
  spellLevel: number,
): {
  id: string;
  title: string;
  detail: string;
  enabled: boolean;
  reason: string;
} {
  const g = p.grant;
  if (!g) {
    throw new Error('grantCard: the way has no grant');
  }
  const how = p.way === 'ritual' ? 'como ritual' : p.slot ? 'com espaço de magia' : '';
  const usable = p.slot || g.ready;
  return {
    id: p.id,
    title: [g.sourceNamePt, how].filter((x) => x !== '').join(' · '),
    detail: p.slot
      ? `${grantNumbers(g)} · não gasta usos`
      : `${grantUsesText(g)} · ${grantNumbers(g)}${aboveOwnLevel(g.castLevel, spellLevel)}`,
    enabled: usable,
    reason: usable ? '' : 'Sem usos restantes.',
  };
}
