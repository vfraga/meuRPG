import { computed, signal } from '@angular/core';

import type {
  Combatant,
  Encounter,
  GetTurnOptionsResponse,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import type { MountOptions } from '../../../../gen/meurpg/rules/v1/rules_pb';
import type { CombatClient } from '../../../core/combat/combat-client';
import type { MoveResult } from '../../../core/combat/combat-client-types';
import type { Square } from '../../../core/combat/combat-grid';

/** What `MoveMount` reads from the screen the flow serves. */
export interface MoveMountContext {
  readonly api: Pick<CombatClient, 'moves'>;
  readonly campaignId: () => string;
  readonly encounter: () => Encounter | null;
  readonly own: () => Combatant | null;
  readonly options: () => GetTurnOptionsResponse | null;
  readonly creatureOptions: () => ReadonlyMap<string, GetTurnOptionsResponse>;
}

/** What `MoveMount` reads from the move flow it belongs to: who moves, and how a move is sent. */
export interface MoveMountHost {
  readonly mover: () => Combatant | null;
  readonly moverId: () => string;
  /** The refusal the page shows next to the map: a new try clears the last one. */
  readonly moveError: { set(text: string): void };
  /** Sends a move through the flow (busy, notes, applying the answer). */
  runMove(call: (e: Encounter) => Promise<MoveResult>): Promise<boolean>;
  /** The flow's answer to a refusal: the words go next to the map. */
  afterMove(ok: boolean): void;
}

/** The mount side of the "Mover" page's flow (RN-21): mounting, dismounting, and whose move a rider's is. */
export class MoveMount {
  /** "Desmontar aqui" found the square held by something the person does not see: the rider is still on the mount. */
  readonly blocked = signal(false);
  /** What the mover can do about mounts (`TurnOptions.mount`): the creature's own options, or the character's. */
  readonly options = computed<MountOptions | undefined>(() => {
    const id = this.host.moverId();
    return id && id !== this.ctx().own()?.id
      ? this.ctx().creatureOptions().get(id)?.options?.mount
      : this.ctx().options()?.options?.mount;
  });
  /** The combatant a mounted mover moves: its controlled mount. A rider on an independent mount moves on that mount's own turn. */
  readonly rideId = computed(() => {
    const who = this.host.mover();
    const mount = who?.mountedOnId
      ? this.ctx()
          .encounter()
          ?.combatants.find((c) => c.id === who.mountedOnId)
      : undefined;
    return mount?.mountKind === 'controlled' ? mount.id : '';
  });

  /** A page that closes or opens takes the warning of the last dismount with it. */
  reset(): void {
    this.blocked.set(false);
  }

  constructor(
    private readonly ctx: () => MoveMountContext,
    private readonly host: MoveMountHost,
  ) {}

  /** Who walks when this combatant's page confirms a move: the mount that carries it, or itself. */
  subjectOf(who: Combatant): string {
    return this.rideId() || who.id;
  }

  /** The walk of a combatant to a square: the mount carries its rider along (RN-21), so it is the mount's. */
  move(e: Encounter, who: Combatant, to: Square): Promise<MoveResult> {
    const { api, campaignId } = this.ctx();
    return api.moves.move(campaignId(), e.id, this.subjectOf(who), to.col, to.row);
  }

  /** "Montar" on a row of the "Montaria" panel. The page stays open: it is now the mount's. */
  async mountOn(mountId: string): Promise<void> {
    const own = this.host.mover();
    if (!own) {
      return;
    }
    this.blocked.set(false);
    this.host.moveError.set('');
    const ok = await this.host.runMove(async (e) => ({
      encounter: await this.ctx().api.moves.mount(this.ctx().campaignId(), e.id, own.id, mountId),
      stoppedEarly: false,
      lockedDoor: false,
      provoked: false,
    }));
    if (!ok) {
      this.host.afterMove(false);
    }
  }

  /** "Desmontar aqui": the rider lands on the square. Something unseen on it leaves the rider mounted and says so. */
  async dismountTo(to: Square): Promise<void> {
    const own = this.host.mover();
    if (!own) {
      return;
    }
    this.blocked.set(false);
    this.host.moveError.set('');
    let stopped = false;
    const ok = await this.host.runMove(async (e) => {
      const res = await this.ctx().api.moves.dismount(
        this.ctx().campaignId(),
        e.id,
        own.id,
        to.col,
        to.row,
      );
      stopped = res.stoppedEarly;
      return { encounter: res.encounter, stoppedEarly: false, lockedDoor: false, provoked: false };
    });
    if (ok) {
      this.blocked.set(stopped);
    } else {
      this.host.afterMove(false);
    }
  }
}

/** What stopped the master's drag short, in the page's notice (empty when nothing did). */
export function stoppedMessage(res: MoveResult): string {
  if (res.lockedDoor) {
    // The master walks through closed doors but a locked one stops them too (RN-26).
    return 'Uma porta trancada parou o movimento. Destranque a porta (toque nela no mapa) e mova de novo.';
  }
  // A barred or secret door stops it too: the token stands where it stopped.
  return res.stoppedEarly
    ? 'Uma grade levadiça ou uma porta secreta parou o movimento. Abra a porta (toque nela no mapa) e mova de novo.'
    : '';
}
