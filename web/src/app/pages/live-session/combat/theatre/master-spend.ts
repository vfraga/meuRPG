import { Component, computed, effect, inject, input, signal, untracked } from '@angular/core';
import { Code, ConnectError } from '@connectrpc/connect';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import {
  type Combatant,
  EncounterBlockedReason,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { newKey } from '../../../../core/connect/idempotency';
import { combatErrorMessage, encounterBlocked } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { spendPlan } from '../../../../core/combat/theatre';
import { PHONE_QUERY, mediaQuery } from '../../../../shared/map-view/media-query';
import { SpendStepper, startAmount } from './spend-stepper';

/**
 * The master's "Gastar movimento" for an NPC (RN-25, E10-04 state 2): the movement the NPC has, as a bar, and a number to spend in steps of
 * 1,5 m. It is the master's word: the app checks no path, only that the sum never passes what the turn has (the server). The plus stops at
 * what is left; once nothing is left the button is dashed and says why. Inside the NPC's card, under its chips.
 */
@Component({
  selector: 'app-master-spend',
  imports: [MatButtonModule, MatIconModule, SpendStepper],
  template: `
    <section class="move" aria-labelledby="ms-title">
      <div class="move__head">
        <h3 class="move__title" id="ms-title">Movimento</h3>
        <span class="move__hint">Sem mapa: só o número</span>
      </div>
      <span class="bar" role="img" [attr.aria-label]="'Restam ' + plan().left + ' de ' + plan().total">
        <span class="bar__fill" [style.width.%]="plan().leftPercent"></span>
      </span>
      @if (subject().movementLeftFt > 0) {
        <div class="move__row">
          <span class="move__cap">Gastar, em passos de 1,5 m</span>
          <app-spend-stepper [who]="subject()" [ft]="ft()" [fill]="phone()" [busy]="busy()" [label]="'Quanto ' + subject().label + ' gastou'" (ftChange)="change($event)" />
          <button mat-stroked-button type="button" class="move__go" [disabled]="busy()" (click)="spend()">Gastar movimento</button>
        </div>
      } @else {
        <div class="move__row">
          <button mat-stroked-button type="button" class="move__go mr-button--off" disabled disabledInteractive aria-describedby="ms-why">Gastar movimento</button>
          <span class="move__why" id="ms-why">{{ subject().label }} já gastou todo o movimento.</span>
        </div>
      }
      @if (error()) {
        <p class="move__error" role="alert"><mat-icon aria-hidden="true">error</mat-icon>{{ error() }}</p>
      }
    </section>
  `,
  styleUrl: './master-spend.scss',
})
export class MasterSpend {
  private readonly api = inject(CombatClient);

  readonly campaignId = input.required<string>();
  readonly encounterId = input.required<string>();
  /** The NPC on turn. */
  readonly subject = input.required<Combatant>();
  readonly state = input.required<CombatState>();

  /** On a phone the stepper and the button have one width. */
  protected readonly phone = mediaQuery(PHONE_QUERY);
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  protected readonly ft = signal(5);
  private key = newKey();

  protected readonly plan = computed(() => spendPlan(this.subject(), this.ft()));

  constructor() {
    // Each NPC, and each turn, starts at one step; a refresh of the same turn keeps the number.
    let of = '';
    effect(() => {
      const s = this.subject();
      const id = `${s.id}:${this.state().encounter()?.round}`;
      if (id !== of) {
        of = id;
        untracked(() => {
          this.ft.set(startAmount(s));
          this.error.set('');
          this.key = newKey();
        });
      }
    });
  }

  /** A refusal that means the screen is stale reads the combat again before it says so. */
  private async reread(err: unknown): Promise<void> {
    const code = ConnectError.from(err).code;
    if (code === Code.Aborted || code === Code.FailedPrecondition || code === Code.NotFound) {
      try {
        this.state().apply(await this.api.turn.get(this.campaignId()));
        this.ft.set(this.plan().ft);
      } catch {
        // The stream's next read brings it.
      }
    }
  }

  protected change(ft: number): void {
    this.ft.set(ft);
    this.key = newKey();
    this.error.set('');
  }

  protected async spend(): Promise<void> {
    const plan = this.plan();
    if (plan.ft <= 0 || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.moves.spendMovement(
        this.campaignId(),
        this.encounterId(),
        this.subject().id,
        plan.ft,
        this.key,
      );
      this.state().apply(res.encounter);
      this.key = newKey();
      this.ft.set(startAmount({ movementLeftFt: Math.floor(res.movementLeftDft / 10) }));
    } catch (err) {
      await this.reread(err);
      const left = this.plan().left;
      const blocked = encounterBlocked(err);
      this.error.set(
        blocked?.reason === EncounterBlockedReason.TOO_FAR
          ? `Só restam ${left}. Escolha menos.`
          : combatErrorMessage(err, 'gastar o movimento'),
      );
    } finally {
      this.busy.set(false);
    }
  }
}
