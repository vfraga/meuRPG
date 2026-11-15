import type { Combatant } from '../../../gen/meurpg/play/v1/combat_pb';

/**
 * A rider and the mount it sits on, as the map draws them (588-04): one token, the mount's, with the rider's disc on it.
 * Only a pair the viewer sees both of is drawn so; the server sends the link only then (RN-10).
 */
export class MapPairs {
  constructor(
    private readonly shown: () => readonly Combatant[],
    private readonly currentId: () => string,
  ) {}

  private pairs(): Map<string, Combatant> {
    const shown = this.shown();
    const ids = new Set(shown.map((c) => c.id));
    const out = new Map<string, Combatant>();
    for (const c of shown) {
      if (c.mountedOnId && ids.has(c.mountedOnId)) {
        out.set(c.mountedOnId, c);
      }
    }
    return out;
  }

  /** The rider drawn on this mount's token. */
  riderOf(c: Combatant): Combatant | undefined {
    return this.pairs().get(c.id);
  }

  /** The mount carries the rider `id`: whoever moves the rider moves the token. */
  carries(c: Combatant, id: string | null): boolean {
    return id !== null && this.riderOf(c)?.id === id;
  }

  /** The combatant rides a mount that is on the map: it is drawn on the mount's token, not on its own. */
  rides(c: Combatant): boolean {
    return !!c.mountedOnId && this.pairs().get(c.mountedOnId)?.id === c.id;
  }

  /** The turn belongs to the token when it belongs to the rider on it. */
  isCurrent(c: Combatant): boolean {
    return c.id === this.currentId() || this.riderOf(c)?.id === this.currentId();
  }
}
