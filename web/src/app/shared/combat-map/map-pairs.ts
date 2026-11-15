import { computed } from '@angular/core';

import type { Combatant } from '../../../gen/meurpg/play/v1/combat_pb';
import { conditionTags } from '../../core/combat/conditions';
import { ridingOn } from '../../core/combat/mount-view';

/**
 * A rider and the mount it sits on, as the map draws them (588-04): one token, the mount's, with the rider's disc on it, one
 * pick button for the two, and the rider's line in the map's text list. Only a pair the viewer sees both of is drawn so; the
 * server sends the link only then (RN-10).
 */
export class MapPairs {
  constructor(
    private readonly shown: () => readonly Combatant[],
    private readonly currentId: () => string,
  ) {}

  /** Each mount on the map with the rider that sits on it. */
  private readonly pairs = computed(() => {
    const shown = this.shown();
    const ids = new Set(shown.map((c) => c.id));
    const out = new Map<string, Combatant>();
    for (const c of shown) {
      if (c.mountedOnId && ids.has(c.mountedOnId)) {
        out.set(c.mountedOnId, c);
      }
    }
    return out;
  });

  /** What the map's visually hidden list says of each token: "Tor, montado no Cavalo de guerra, coluna 6, linha 6". */
  readonly listed = computed(() =>
    this.shown().map((c) => {
      const mount = this.rides(c) ? this.shown().find((m) => m.id === c.mountedOnId) : undefined;
      const on = mount ? `, ${ridingOn(mount)}` : '';
      const tags = c.conditions.length ? `, ${conditionTags(c).join(', ')}` : '';
      return { id: c.id, text: `${c.label}${on}, coluna ${c.col + 1}, linha ${c.row + 1}${tags}` };
    }),
  );

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

  /** The pick button of a pair names both: "Escolher Cavalo de guerra como alvo · leva Tor". */
  pickName(c: Combatant, name: string): string {
    const rider = this.riderOf(c);
    return rider ? `${name} · leva ${rider.label}` : name;
  }
}
