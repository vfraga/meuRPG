import { ChangeDetectionStrategy, Component, computed, input, output } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';
import { MatMenuModule } from '@angular/material/menu';

import type { Combatant, ReadiedAction } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { readiedCardView } from '../../../../core/combat/ready-card';

/**
 * The readied action under the economy row (#582, board 582-03): the held action, the trigger quoted and where it stands, in any
 * sub-tab of the actions. On the readier's own turn it has a "⋮" with "Desistir da ação preparada" (`CancelReadied`; the action
 * spent does not come back); off the turn it only reads, because who decides the trigger happened is the master. Nothing here is
 * derived for anyone else: the server sends the row to its owner and the master only (RN-10).
 */
@Component({
  selector: 'app-readied-card',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatIconModule, MatMenuModule],
  template: `
    <section class="card" [attr.aria-label]="'Ação preparada: ' + view().title" data-testid="readied-card">
      <div class="card__head">
        <mat-icon class="card__icon" aria-hidden="true">alarm</mat-icon>
        <h3 class="card__title">{{ view().title }}</h3>
        @if (view().concentration) {
          <span class="card__chip"><mat-icon aria-hidden="true">adjust</mat-icon>Concentração</span>
        }
        @if (ownTurn()) {
          <button
            type="button"
            class="card__more"
            aria-label="Mais ações da ação preparada"
            [matMenuTriggerFor]="menu"
            [disabled]="busy()"
          >
            <mat-icon aria-hidden="true">more_vert</mat-icon>
          </button>
          <mat-menu #menu="matMenu" xPosition="before">
            <button mat-menu-item type="button" (click)="giveUp.emit()">
              <mat-icon aria-hidden="true">close</mat-icon>
              <span class="item">Desistir da ação preparada<small class="item__sub">A ação gasta não volta.</small></span>
            </button>
          </mat-menu>
        }
      </div>
      <p class="card__quote">{{ view().quote }}</p>
      <p class="card__state">
        <mat-icon aria-hidden="true">{{ ownTurn() ? 'square' : 'hourglass_empty' }}</mat-icon>{{ view().state }}
      </p>
    </section>
  `,
  styleUrl: './readied-card.scss',
})
export class ReadiedCard {
  readonly readied = input.required<ReadiedAction>();
  readonly combatants = input<readonly Pick<Combatant, 'id' | 'label'>[]>([]);
  /** The readier is on turn: it may give the action up. */
  readonly ownTurn = input(false);
  readonly reactionUsed = input(false);
  readonly busy = input(false);
  /** "Desistir da ação preparada". */
  readonly giveUp = output<void>();

  protected readonly view = computed(() =>
    readiedCardView(this.readied(), this.combatants(), {
      ownTurn: this.ownTurn(),
      reactionUsed: this.reactionUsed(),
    }),
  );
}
