import { Component, computed, effect, inject, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { Combatant } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { hitPointsAfter } from '../../../../core/combat/attack-flow';
import { CombatClient, type HpAdjust } from '../../../../core/combat/combat-client';
import { ActionKey } from '../../../../core/connect/idempotency';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { VitalsStepper } from '../../vitals-stepper/vitals-stepper';
import { injectSheet } from '../sheet-host';

export interface AdjustNpcData {
  readonly campaignId: string;
  readonly encounterId: string;
  readonly combatant: Combatant;
  readonly state: CombatState;
}

type Mode = 'damage' | 'heal' | 'exact';

const MODES: readonly { value: Mode; label: string }[] = [
  { value: 'damage', label: 'Dano' },
  { value: 'heal', label: 'Cura' },
  { value: 'exact', label: 'Valor exato' },
];

/** What "Dano/Cura" may type: 0 to 9.999 (the server's limit), and temporary
 * hit points from 0 to 999. */
export const MAX_AMOUNT = 9999;
export const MAX_TEMPORARY = 999;

/**
 * "Dano/Cura" on an NPC (MR-012, E5-05's "Ajustar"): the master's hand on its
 * hit points. One of damage, heal or an exact value, and, apart or together,
 * new temporary hit points; the line under says what the PV will be. A
 * defeated NPC healed above 0 comes back into the turn (the server does it).
 * One idempotency key per set of numbers: a retry after a failure repeats it.
 */
@Component({
  selector: 'app-adjust-npc',
  imports: [MatButtonModule, MatIconModule, VitalsStepper],
  templateUrl: './adjust-npc.html',
  styleUrl: './adjust-npc.scss',
})
export class AdjustNpc {
  private readonly api = inject(CombatClient);
  private readonly sheet = injectSheet<AdjustNpcData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly c = this.data.combatant;
  /** The combatant as the combat has it now: the numbers move while the sheet is open. */
  private readonly current = computed(
    () => this.data.state.encounter()?.combatants.find((x) => x.id === this.c.id) ?? this.c,
  );
  protected readonly hp = computed(() => this.current().hitPointsCurrent ?? 0);
  protected readonly max = computed(() => this.current().hitPointsMax ?? 0);
  protected readonly temp0 = computed(() => this.current().hitPointsTemporary ?? 0);
  protected readonly modes = MODES;
  protected readonly maxAmount = MAX_AMOUNT;
  protected readonly maxTemporary = MAX_TEMPORARY;

  protected readonly mode = signal<Mode>('damage');
  protected readonly amount = signal(0);
  protected readonly temporary = signal(this.c.hitPointsTemporary ?? 0);
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');
  private readonly key = new ActionKey();

  protected readonly amountMax = computed(() =>
    this.mode() === 'exact' ? this.max() : MAX_AMOUNT,
  );
  protected readonly amountLabel = computed(() =>
    this.mode() === 'damage' ? 'Dano sofrido' : this.mode() === 'heal' ? 'PV curados' : 'PV exatos',
  );
  protected readonly valid = computed(() => {
    const whole = (n: number, max: number) => Number.isInteger(n) && n >= 0 && n <= max;
    return whole(this.amount(), this.amountMax()) && whole(this.temporary(), MAX_TEMPORARY);
  });
  /** What the call sends: only what changes the NPC. `null`: nothing to change. */
  protected readonly adjust = computed<HpAdjust | null>(() => {
    const kind = this.mode();
    const change =
      kind === 'exact'
        ? this.amount() === this.hp()
          ? undefined
          : { kind, value: this.amount() }
        : this.amount() > 0
          ? { kind, value: this.amount() }
          : undefined;
    const temporary = this.temporary() !== this.temp0() ? this.temporary() : undefined;
    return change || temporary !== undefined ? { change, temporary } : null;
  });
  protected readonly after = computed(() => {
    const a = this.amount();
    switch (this.mode()) {
      case 'damage':
        return hitPointsAfter(this.hp(), this.temp0(), a);
      case 'heal':
        return Math.min(this.max(), this.hp() + a);
      default:
        return a;
    }
  });
  protected readonly preview = computed(() =>
    this.adjust()?.change
      ? `Depois: ${this.after()} de ${this.max()} PV`
      : `Agora: ${this.hp()} de ${this.max()} PV`,
  );

  protected readonly amountStep = (step: number) => (step < 0 ? `Tirar ${-step}` : `Somar ${step}`);
  protected readonly tempStep = (step: number) =>
    step < 0 ? 'Tirar 1 PV temporário' : 'Somar 1 PV temporário';

  constructor() {
    // A request in the air cannot be dismissed (Esc, the backdrop, ✕, Cancelar): its answer is always shown.
    effect(() => this.sheet.lock(this.busy()));
  }

  protected async save(): Promise<void> {
    const adjust = this.adjust();
    if (this.busy() || !this.valid()) {
      return;
    }
    if (!adjust) {
      this.sheet.close(false);
      return;
    }
    // New numbers are a new correction; the same numbers again are a retry.
    const key = this.key.keyFor([this.c.id, adjust]);
    this.busy.set(true);
    this.error.set('');
    try {
      this.data.state.apply(
        await this.api.adjustHitPoints(
          this.data.campaignId,
          this.data.encounterId,
          this.c.id,
          adjust,
          key,
        ),
      );
      this.sheet.close(true);
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'ajustar os PV'));
    } finally {
      this.busy.set(false);
    }
  }

  protected close(): void {
    if (this.busy()) {
      return;
    }
    this.sheet.close(false);
  }
}
