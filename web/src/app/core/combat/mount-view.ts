import type { Combatant, Encounter } from '../../../gen/meurpg/play/v1/combat_pb';
import { article } from './combat-log';

/**
 * A rider and its mount in the order and on the map (RN-21, RN-10). The server sends `mounted_on_id` and `rider_id` only to
 * someone who sees both rows, so every link read here is one the viewer may read; the mark (`mount_kind`) is the master's, the
 * owner's and the rider's player's, and is empty for anyone else. Pure TypeScript, tested without a DOM.
 */

/** The mount the combatant sits on, when the viewer sees the link. */
export function mountOf(e: Encounter, c: Combatant): Combatant | undefined {
  return c.mountedOnId ? e.combatants.find((x) => x.id === c.mountedOnId) : undefined;
}

/** The combatant that sits on the mount, when the viewer sees the link. */
export function riderOn(e: Encounter, c: Combatant): Combatant | undefined {
  return e.combatants.find((x) => x.mountedOnId === c.id);
}

/** The rider and its mount are the pair `a`, `b` (in either order). */
export function isPair(a: Combatant, b: Combatant): boolean {
  return a.mountedOnId === b.id || b.mountedOnId === a.id;
}

/**
 * Whether the order boxes the two together. The master knows the mark, so only a controlled mount shares its rider's box; a
 * player is not told the mark of a mount that is not theirs, and reads a mount that follows its rider in the order as the pair.
 */
export function boxedPair(a: Combatant, b: Combatant, master: boolean): boolean {
  if (!isPair(a, b)) {
    return false;
  }
  const mount = a.mountedOnId === b.id ? b : a;
  // A mark the viewer reads decides; an empty one, for a player, is a mark they are not told.
  return mount.mountKind === 'controlled' || (!master && mount.mountKind === '');
}

const the = (name: string): string => `${article(name) === 'a' ? 'a' : 'o'} ${name}`;
const onThe = (name: string): string => `${article(name) === 'a' ? 'na' : 'no'} ${name}`;

/**
 * What a row of the order says first about the link: "Montaria de Tor" under a pair's box (the master reads "Adestrada · leva
 * Tor"), "Montado no Esqueleto" and "Leva o Wight · independente" for a mount that keeps its own turn. Empty for a combatant
 * that is not part of a pair. `boxed` says the row is inside the pair's box, where the rider needs no line.
 */
export function mountLine(e: Encounter, c: Combatant, master: boolean, boxed: boolean): string {
  const mount = mountOf(e, c);
  if (mount) {
    return boxed ? '' : mountedOn(mount);
  }
  const rider = riderOn(e, c);
  if (!rider) {
    return '';
  }
  const kind = c.mountKind;
  if (boxed) {
    return master ? `Adestrada · leva ${rider.label}` : `Montaria de ${rider.label}`;
  }
  return `Leva ${the(rider.label)}${kind === 'independent' || master ? ' · independente' : ''}`;
}

/** `mountLine` for a combatant in the order, working out whether the order boxes it with its partner. */
export function mountLineOf(e: Encounter, c: Combatant, master: boolean): string {
  const partner = mountOf(e, c) ?? riderOn(e, c);
  return mountLine(e, c, master, partner !== undefined && boxedPair(c, partner, master));
}

/** "Montado no Cavalo de guerra": the rider's line, in the order and under "Sua vez, Tor" (588-04). */
export function mountedOn(mount: Combatant): string {
  return `Montado ${onThe(mount.label)}`;
}
