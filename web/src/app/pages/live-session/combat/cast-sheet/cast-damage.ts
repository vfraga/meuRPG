import { type WritableSignal, computed, signal } from '@angular/core';

import type { PendingDamage } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { rollGroups } from '../../../../core/combat/cast-flow';
import { type DamageDie, type CombatClient } from '../../../../core/combat/combat-client';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { ActionKey } from '../../../../core/connect/idempotency';
import { showOfDice, type RollAnimator } from '../../../../shared/roll-overlay/roll-animator';

/** What `CastDamage` reads from the sheet it rolls for. */
export interface CastDamageContext {
  readonly api: Pick<CombatClient, 'rollDamage'>;
  readonly campaignId: string;
  readonly encounterId: string;
  /** The damages a closed sheet left unrolled: the sheet opens at them. */
  readonly resume: readonly PendingDamage[];
  readonly state: Pick<CombatState, 'apply'>;
  readonly animator: Pick<RollAnimator, 'play'>;
  /** The sheet's one request in the air, and its refusal. */
  readonly busy: WritableSignal<boolean>;
  readonly error: WritableSignal<string>;
}

/**
 * The damage of a cast that is still to roll: every damage and heal of the cast as it is now, the groups the player rolls
 * together, and the call that rolls one. Plain class the cast sheet builds; the keys (one for each damage, made once)
 * keep a tap repeated after a lost answer from rolling twice.
 */
export class CastDamage {
  /** Every damage and heal of the cast as it is now, by ID. */
  readonly pendings: WritableSignal<ReadonlyMap<string, PendingDamage>>;
  readonly groups = computed(() => rollGroups([...this.pendings().values()]));
  readonly owed = computed(() => this.groups().length > 0);
  /** One key per damage roll of a pending (its die): the same die again is a retry, another die a new request. */
  private readonly keys = new Map<string, ActionKey>();

  constructor(private readonly host: CastDamageContext) {
    this.pendings = signal(new Map(host.resume.map((p) => [p.id, p])));
  }

  /** The damages the cast made, as it told them. */
  set(pendings: readonly PendingDamage[]): void {
    this.pendings.set(new Map(pendings.map((p) => [p.id, p])));
  }

  /** "Rolar no app": every damage still owed is rolled, one call each. */
  async rollAll(): Promise<void> {
    for (const group of this.groups()) {
      if (!(await this.rollGroup(group, { inApp: true }))) {
        return;
      }
    }
  }

  /** The typed sum for the first group; true when it was rolled. */
  async rollTyped(sum: number): Promise<boolean> {
    const group = this.groups()[0];
    return group ? this.rollGroup(group, { sum }) : false;
  }

  async rollGroup(group: readonly PendingDamage[], die: DamageDie): Promise<boolean> {
    const p = group[0];
    if (this.host.busy()) {
      return false;
    }
    this.host.busy.set(true);
    this.host.error.set('');
    try {
      const res = await this.host.api.rollDamage(
        this.host.campaignId,
        this.host.encounterId,
        p.id,
        die,
        this.keyOf(p.id).keyFor(die),
      );
      this.host.state.apply(res.encounter);
      const next = new Map(this.pendings());
      for (const settled of [res.pending, ...res.cast]) {
        next.set(settled.id, settled);
      }
      this.pendings.set(next);
      const show = showOfDice('Dano', res.pending.roll, {
        withTotal: true,
        note: res.pending.damageTypePt || undefined,
      });
      if (show) {
        this.host.animator.play(show);
      }
      return true;
    } catch (err) {
      this.host.error.set(combatErrorMessage(err, 'rolar o dano'));
      return false;
    } finally {
      this.host.busy.set(false);
    }
  }

  private keyOf(id: string): ActionKey {
    let keys = this.keys.get(id);
    if (!keys) {
      keys = new ActionKey();
      this.keys.set(id, keys);
    }
    return keys;
  }
}
