import { signal } from '@angular/core';

import type { ReactionAnswer } from '../../../../core/combat/combat-client-types';
import type { Encounter } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { AimNeed } from '../../../../core/combat/readied-aim';
import type { QueueRow } from '../../../../core/combat/reaction-master';
import { readiedOfWindow } from '../../../../core/combat/reaction-readied';

/**
 * What the master's list of creatures of each readied window asks, and what he picked (`app-readied-aim`): "Atacar por X" and
 * "Soltar por X" wait for a creature when the attack needs one (the server refuses an attack with none).
 */
export class ReadiedAimState {
  private readonly needs = signal<Readonly<Record<string, AimNeed>>>({});
  private readonly targets = signal<Readonly<Record<string, string | null>>>({});

  constructor(private readonly encounter: () => Encounter) {}

  held(row: Pick<QueueRow, 'window'>) {
    return readiedOfWindow(this.encounter(), row.window);
  }

  needOf(row: QueueRow): AimNeed {
    return this.needs()[row.window.id] ?? 'loading';
  }

  targetOf(row: QueueRow): string | null {
    return this.targets()[row.window.id] ?? null;
  }

  setNeed(row: QueueRow, need: AimNeed): void {
    this.needs.update((m) => ({ ...m, [row.window.id]: need }));
  }

  setTarget(row: QueueRow, target: string | null): void {
    this.targets.update((m) => ({ ...m, [row.window.id]: target }));
  }

  /** Whether the use waits for a creature to be picked. */
  off(row: QueueRow): boolean {
    if (!row.readied?.aims) {
      return false;
    }
    switch (this.needOf(row)) {
      case 'required':
        return this.targetOf(row) === null;
      case 'optional':
        return false;
      case 'failed':
        return (this.held(row)?.targetCombatantId ?? '') === '';
      default:
        return true;
    }
  }

  /** Whether the use button is off: not this window's turn, a use the queue cannot make, or a creature still to pick. */
  useOff(row: QueueRow): boolean {
    return !row.answerNow || !!row.readied?.useOff || this.off(row);
  }

  /** The use of a readied action: the creature picked and the d20 of the app for an attack or a spell (`creature_ids`, `roll`); `null` while it is off. */
  answerOf(row: QueueRow): ReactionAnswer | null {
    if (row.readied?.useOff || this.off(row)) {
      return null;
    }
    const target = this.targetOf(row);
    return row.readied?.aims
      ? { use: true, ...(target ? { creatureIds: [target] } : {}), die: { inApp: true } }
      : { use: true };
  }
}
