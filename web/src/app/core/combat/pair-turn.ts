import type { Combatant, Encounter } from '../../../gen/meurpg/play/v1/combat_pb';
import { isDown } from './combat-view';
import { acts, partToEnd } from './joint-turn';
import { isDismountWindow } from './reaction-dismount';
import { openWindows } from './reactions';

/**
 * "Encerrar turno" of a rider that directs its mount: one button for the two parts, one call each. Pure
 * functions over the combat as it is now.
 */

/** The mount whose part the rider's player ends with their own: it still acts, and the rider is on its feet. A rider at 0 hit
 * points leaves the mount to the master ("Caído: o mestre conduz o Cavalo de guerra."). `null` otherwise. */
export function rideToEnd(
  e: Encounter | null,
  ride: Combatant | null,
  rider: Combatant | null,
): Combatant | null {
  const mount = ride ? e?.combatants.find((c) => c.id === ride.id) : undefined;
  return e && mount && acts(e, mount) && !(rider && isDown(rider)) ? mount : null;
}

/**
 * The parts one "Encerrar turno" ends, in the order they are sent. Whoever rides nothing ends one part, as before
 * (`partToEnd`). A rider ends its own part first and then the mount's: the server holds the rider's part on what the rider
 * still owes (a damage to roll, the rage question, a death save), and a refusal there sends nothing for the mount, which
 * keeps its move. A part that no longer acts is left out, so a retry after the first call, or a part the master ended,
 * sends only what is left.
 */
export function turnEnds(e: Encounter, own: Combatant | null, ride: Combatant | null): string[] {
  if (!ride) {
    return [partToEnd(e, own)];
  }
  const mount = rideToEnd(e, ride, own);
  return [...(own && acts(e, own) ? [own.id] : []), ...(mount ? [mount.id] : [])];
}

/**
 * Whether the window that holds the mount's part is this rider's own question: the save to stay on the mount or the dismount
 * window. Any other pending reaction (an opportunity offer, a readied window, an effect save, the master's reaction) is not the
 * pair's to swallow: "Encerrar turno" says it.
 */
export function ownPairWindowOpen(e: Encounter, rider: Combatant): boolean {
  return (
    e.mountSave?.riderId === rider.id ||
    openWindows(e).some((w) => isDismountWindow(w) && w.reactorId === rider.id)
  );
}
