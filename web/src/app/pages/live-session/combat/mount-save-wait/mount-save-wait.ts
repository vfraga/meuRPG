import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { MountSavePrompt } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { MountSaveWait } from '../../../../core/combat/mount-save';

/**
 * "Esperando o teste de Tor" (#588, board 588-08): what the master reads while a rider's save to stay mounted holds the table. It
 * says the save, its DC and the cause, and who is answering; "Rolar por Tor" opens the save's sheet for the master, who may always
 * roll it (the rider's player has it open on their own screen). The turn does not pass until the save is rolled.
 */
@Component({
  selector: 'app-mount-save-wait',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatButtonModule, MatIconModule],
  template: `
    @if (wait(); as w) {
      <section class="mr-notice mr-notice--warning card" aria-labelledby="mount-save-wait-t" data-testid="mount-save-wait">
        <mat-icon aria-hidden="true">hourglass_top</mat-icon>
        <div class="card__body">
          <h3 class="card__title" id="mount-save-wait-t">{{ w.title }}</h3>
          <p class="card__detail">{{ w.detail }}</p>
          <button matButton="outlined" type="button" class="card__btn" [disabled]="busy()" disabledInteractive (click)="roll.emit(w.prompt)">
            {{ w.rollLabel }}
          </button>
        </div>
      </section>
    }
  `,
  styles: `
    :host {
      display: block;
      min-width: 0;
    }
    .card__body {
      display: flex;
      flex-direction: column;
      align-items: flex-start;
      gap: var(--mr-space-2);
      min-width: 0;
    }
    .card__title {
      margin: 0;
      font-size: var(--mr-font-16);
      font-weight: 700;
      line-height: 22px;
    }
    .card__detail {
      margin: 0;
      font-size: var(--mr-font-14);
      line-height: 20px;
      overflow-wrap: anywhere;
    }
    .card__btn {
      --mat-button-outlined-container-height: 44px;
      --mat-button-outlined-label-text-color: var(--mr-ink);
      white-space: normal;
    }
  `,
})
export class MountSaveWaitCard {
  readonly wait = input.required<MountSaveWait | null>();
  readonly busy = input(false);
  readonly roll = output<MountSavePrompt>();
}
