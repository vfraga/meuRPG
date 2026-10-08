import { Component, computed, effect, inject, signal } from '@angular/core';
import { Code, ConnectError } from '@connectrpc/connect';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { EncounterBlockedReason } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CombatClient, newKey } from '../../../../core/combat/combat-client';
import { combatErrorMessage, encounterBlocked } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { spendLabel, spendPlan, turnHas } from '../../../../core/combat/theatre';
import { SheetFrame } from '../sheet-frame/sheet-frame';
import { injectSheet } from '../sheet-host';
import { SpendStepper, startAmount } from './spend-stepper';

/** What the page hands the movement sheet. */
export interface SpendSheetData {
  readonly campaignId: string;
  readonly encounterId: string;
  /** Who spends: the player's character or one of their creatures. */
  readonly combatantId: string;
  /** The page's copy of the combat: the sheet reads what is left from it, and hands each answer back. */
  readonly state: CombatState;
}

/**
 * "Gastar movimento" (RN-25, E10-04 states 5 to 8): a combat without a map has no "Mover" page, so the player says how far they went. A
 * number in meters, from 1,5 m to 1,5 m, and what is left after it. The app does not check the path or the reach (the master does): the
 * server only holds the sum to the movement the turn has (`SpendMovement`, `TOO_FAR`). A bottom sheet on a phone and a dialog from a tablet
 * up (`openSheet`); the plus stops at what is left, "Depois restam" follows the amount in a live region, and "Cancelar" and the filled
 * "Gastar 6,0 m" are the same width side by side, even at 320 px. The idempotency key is made once per amount, so a double tap never spends twice.
 */
@Component({
  selector: 'app-spend-sheet',
  imports: [MatButtonModule, MatIconModule, SheetFrame, SpendStepper],
  template: `
    <app-sheet-frame title="Gastar movimento" [subtitle]="subtitle()" [phone]="inSheet" (closed)="cancel()">
      @if (error()) {
        <div class="mr-notice mr-notice--danger" role="alert">
          <mat-icon aria-hidden="true">error</mat-icon>
          <p>{{ error() }}</p>
        </div>
      }
      @if (who(); as c) {
        <div class="amount">
          <span class="amount__cap" id="spend-cap">Em passos de 1,5 m</span>
          <app-spend-stepper [who]="c" [ft]="ft()" [busy]="busy()" label="Quanto você gastou" (ftChange)="change($event)" />
        </div>
        <dl class="sum" aria-live="polite">
          <div class="sum__row"><dt>Já gastou</dt><dd>{{ plan().spent }}</dd></div>
          <div class="sum__row"><dt>Vai gastar</dt><dd>{{ plan().amount }}</dd></div>
          <div class="sum__row sum__row--after"><dt>Depois restam</dt><dd>{{ plan().after }}</dd></div>
          <span class="bar" aria-hidden="true"><span class="bar__fill" [style.width.%]="plan().afterPercent"></span></span>
        </dl>
        @if (!plan().canMore && plan().limitFt > 0) {
          <p class="limit" id="spend-limit">Só restam {{ plan().left }}.</p>
        }
        <p class="note">
          <mat-icon aria-hidden="true">info</mat-icon>
          <span>O app não confere o caminho nem o alcance. Combine com o mestre onde você ficou.</span>
        </p>
      } @else {
        <p class="note" role="status">Esse combatente não está mais no combate.</p>
      }
      <div foot class="actions">
        <button mat-stroked-button type="button" class="btn" (click)="cancel()">Cancelar</button>
        <button mat-flat-button type="button" class="btn btn--go" [disabled]="!can() || busy()" disabledInteractive [class.btn--off]="!can()" (click)="spend()">
          {{ label() }}
        </button>
      </div>
    </app-sheet-frame>
  `,
  styleUrl: './spend-sheet.scss',
})
export class SpendSheet {
  private readonly api = inject(CombatClient);
  private readonly sheet = injectSheet<SpendSheetData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');

  protected readonly who = computed(
    () =>
      this.data.state.encounter()?.combatants.find((c) => c.id === this.data.combatantId) ?? null,
  );
  protected readonly ft = signal(
    startAmount(
      this.data.state.encounter()?.combatants.find((c) => c.id === this.data.combatantId) ?? {
        movementLeftFt: 5,
      },
    ),
  );
  protected readonly plan = computed(() => {
    const c = this.who();
    return spendPlan(
      c ?? { speedDft: 1, movementLeftDft: 0, movementLeftFt: 0, movementUsedDft: 0 },
      this.ft(),
    );
  });
  protected readonly subtitle = computed(() => turnHas(this.plan().total));
  protected readonly can = computed(() => this.plan().ft > 0);
  protected readonly label = computed(() => spendLabel(this.plan()));
  /** Made again whenever the amount changes: a new spend, not a retry. */
  private key = newKey();

  protected change(ft: number): void {
    this.ft.set(ft);
    this.key = newKey();
    this.error.set('');
  }

  protected cancel(): void {
    this.sheet.close(false);
  }

  private async reread(err: unknown): Promise<void> {
    const code = ConnectError.from(err).code;
    if (code === Code.Aborted || code === Code.FailedPrecondition || code === Code.NotFound) {
      try {
        this.data.state.apply(await this.api.get(this.data.campaignId));
        this.ft.set(this.plan().ft);
      } catch {
        // The stream's next read brings it.
      }
    }
  }

  protected async spend(): Promise<void> {
    if (!this.can() || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.spendMovement(
        this.data.campaignId,
        this.data.encounterId,
        this.data.combatantId,
        this.plan().ft,
        this.key,
      );
      this.data.state.apply(res.encounter);
      this.sheet.close(true);
    } catch (err) {
      // A refusal that means the screen is stale reads the combat again first, so "a tela foi atualizada" is true and the numbers are the server's.
      await this.reread(err);
      const blocked = encounterBlocked(err);
      this.error.set(
        blocked?.reason === EncounterBlockedReason.TOO_FAR
          ? `Você só tem ${this.plan().left} neste turno. Escolha menos.`
          : combatErrorMessage(err, 'gastar o movimento'),
      );
    } finally {
      this.busy.set(false);
    }
  }
}
