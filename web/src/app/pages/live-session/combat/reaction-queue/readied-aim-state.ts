import { signal } from '@angular/core';

import type { ReactionAnswer } from '../../../../core/combat/combat-client-types';
import type { Encounter, ReactionWindow } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { AimNeed } from '../../../../core/combat/readied-aim';
import type { QueueRow } from '../../../../core/combat/reaction-master';
import { ReadiedKind } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { masterRowWait } from '../../../../core/combat/readied-wait';
import { readiedOfWindow } from '../../../../core/combat/reaction-readied';
import { readiedRefusalText } from '../../../../core/combat/readied-refusal';
import {
  NO_PICK,
  type ReleasePick,
  type ReleasePlan,
  aimedUse,
  releaseComplete,
} from '../../../../core/combat/readied-release';

/** "Soltar por X" and "Mover por X" open the map for the master (#857, #856): the window, and the plan of a held spell (`null` for a move). */
export interface ReleaseRequest {
  readonly window: ReactionWindow;
  readonly plan: ReleasePlan | null;
}

/**
 * What the master's list of creatures of each readied window asks, and what he picked (`app-readied-aim`): "Atacar por X" and
 * "Soltar por X" wait for a creature when the attack needs one (the server refuses an attack with none).
 */
export class ReadiedAimState {
  private readonly needs = signal<Readonly<Record<string, AimNeed>>>({});
  private readonly targets = signal<Readonly<Record<string, string | null>>>({});
  private readonly picks = signal<Readonly<Record<string, ReleasePick>>>({});
  private readonly plans = signal<Readonly<Record<string, ReleasePlan | null>>>({});

  constructor(
    private readonly encounter: () => Encounter,
    /** The prefix of the ids of the queue's rows, for what a row's buttons point to. */
    private readonly uid = '',
  ) {}

  /** What a row's buttons point to: the reason "use" is off, the wait another window holds, or the line that says who the table waits for. */
  describedBy(row: QueueRow): string | null {
    const id = this.uid + row.window.id;
    if (row.status) {
      return `${id}-why`;
    }
    return row.readied ? `${id}-wait` : null;
  }

  /** The wait line of a readied row: where the player is and the time left (`undefined` for a window with no clock). */
  waitOf(row: QueueRow, seconds: number | undefined) {
    // The mark is also set by a player who only chooses targets in the window: "No mapa" is for a held action that is released on the map.
    return masterRowWait(
      row.window.reactorLabel,
      row.window.rolling && this.mapRelease(row) !== null,
      seconds,
    );
  }

  held(row: Pick<QueueRow, 'window'>) {
    return readiedOfWindow(this.encounter(), row.window);
  }

  /** The words of a refused answer of the row's window: its held action, and the darts when the held spell splits them. */
  refusalText(err: unknown, row: Pick<QueueRow, 'window'>, doing: string): string {
    return readiedRefusalText(err, this.held(row), doing, this.planOf(row));
  }

  needOf(row: QueueRow): AimNeed {
    return this.needs()[row.window.id] ?? 'loading';
  }

  targetOf(row: QueueRow): string | null {
    return this.targets()[row.window.id] ?? null;
  }

  pickOf(row: QueueRow): ReleasePick {
    return this.picks()[row.window.id] ?? NO_PICK;
  }

  planOf(row: Pick<QueueRow, 'window'>): ReleasePlan | null {
    return this.plans()[row.window.id] ?? null;
  }

  setPick(row: QueueRow, pick: ReleasePick): void {
    this.picks.update((m) => ({ ...m, [row.window.id]: pick }));
  }

  setPlan(row: QueueRow, plan: ReleasePlan | null): void {
    this.plans.update((m) => ({ ...m, [row.window.id]: plan }));
  }

  setNeed(row: QueueRow, need: AimNeed): void {
    this.needs.update((m) => ({ ...m, [row.window.id]: need }));
  }

  setTarget(row: QueueRow, target: string | null): void {
    this.targets.update((m) => ({ ...m, [row.window.id]: target }));
  }

  /** A held move, or a held spell the options place on the map: its use opens the map (`plan` is `null` for a move); `null` for any other. */
  mapRelease(row: QueueRow): { readonly plan: ReleasePlan | null } | null {
    const kind = this.held(row)?.kind;
    if (kind === ReadiedKind.MOVE) {
      return { plan: null };
    }
    const plan = this.planOf(row);
    return kind === ReadiedKind.SPELL && plan?.onMap ? { plan } : null;
  }

  /** Whether the use waits for a creature to be picked. */
  off(row: QueueRow): boolean {
    if (!row.readied?.aims) {
      return false;
    }
    switch (this.needOf(row)) {
      case 'required':
        return this.targetOf(row) === null;
      case 'optional': {
        const plan = this.planOf(row);
        return !!plan && !releaseComplete(plan, this.pickOf(row));
      }
      case 'failed':
        return (this.held(row)?.targetCombatantId ?? '') === '';
      default:
        return true;
    }
  }

  /** Whether the use button is off: not this window's turn, a use the queue cannot make, or a creature still to pick. */
  useOff(row: QueueRow): boolean {
    return !row.answerNow || this.off(row);
  }

  /** The use of a readied action: the creature picked or the held spell's targets, and the d20 of the app (`creature_ids` or `targets`, `roll`); `null` while it is off. */
  answerOf(row: QueueRow): ReactionAnswer | null {
    if (this.off(row)) {
      return null;
    }
    return row.readied?.aims
      ? aimedUse(this.planOf(row), this.pickOf(row), this.targetOf(row))
      : { use: true };
  }
}
