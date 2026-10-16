import { ChangeDetectionStrategy, Component, computed, inject, input, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { Encounter } from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  type RollModeRequest,
  RollMode,
  RollModeRequestStatus,
} from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { requestLine } from '../../../../core/combat/roll-mode';
import { ActionKey } from '../../../../core/connect/idempotency';

/**
 * The master's queue of roll mode requests: a player asked for a mode better
 * than the app's suggestion ("Pedido de Vantagem: Toren → Goblin (Espada curta) —
 * “estou nas costas dele”"). "Aprovar" gives the mode asked for, "Recusar" the
 * suggested one, "Desvantagem" the worst for the roller. The answer goes to the
 * player's sheet, which rolls with it. Nothing is drawn while the queue is empty.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-roll-mode-queue',
  imports: [MatButtonModule, MatIconModule],
  template: `
    @if (pending().length) {
      <section class="queue" aria-label="Pedidos de modo de rolagem">
        @for (r of pending(); track r.id) {
          <div class="card" role="group" [attr.aria-label]="'Pedido de ' + r.attackNamePt">
            <p class="card__text">
              <mat-icon aria-hidden="true">front_hand</mat-icon>
              <span>{{ line(r) }}</span>
            </p>
            <div class="card__buttons">
              <button mat-flat-button type="button" [disabled]="busy()" (click)="answer(r, r.requestedMode)">Aprovar</button>
              <button mat-stroked-button type="button" [disabled]="busy()" (click)="answer(r, r.suggestedMode)">Recusar</button>
              <button mat-stroked-button type="button" [disabled]="busy()" (click)="answer(r, disadvantage)">Desvantagem</button>
            </div>
          </div>
        }
        @if (error()) {
          <div class="mr-notice mr-notice--danger" role="alert">
            <mat-icon aria-hidden="true">error</mat-icon>
            <p>{{ error() }}</p>
          </div>
        }
      </section>
    }
  `,
  styleUrl: './roll-mode-queue.scss',
})
export class RollModeQueue {
  private readonly api = inject(CombatClient);
  readonly encounter = input.required<Encounter>();
  readonly campaignId = input.required<string>();
  readonly state = input.required<CombatState>();

  protected readonly busy = signal(false);
  protected readonly error = signal('');
  protected readonly disadvantage = RollMode.DISADVANTAGE;
  private readonly keys = new ActionKey();

  protected readonly pending = computed(() =>
    (this.encounter().rollModeRequests ?? []).filter(
      (r) => r.status === RollModeRequestStatus.PENDING,
    ),
  );

  protected line(r: RollModeRequest): string {
    const combatants = this.encounter().combatants;
    return requestLine(r, (id) => combatants.find((c) => c.id === id)?.label ?? 'Alguém');
  }

  protected async answer(r: RollModeRequest, mode: RollMode): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const encounter = await this.api.attacks.answerRollModeRequest(
        this.campaignId(),
        this.encounter().id,
        r.id,
        mode,
        this.keys.keyFor({ id: r.id, mode }),
      );
      this.state().apply(encounter);
      this.keys.renew();
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'responder ao pedido'));
    } finally {
      this.busy.set(false);
    }
  }
}
