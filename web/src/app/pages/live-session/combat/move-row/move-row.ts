import { ChangeDetectionStrategy, Component, computed, input, output } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { Combatant } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { MASTER_WAIT_NOTE_ID, isMasterWait } from '../../../../core/combat/reactions';
import { moveLine } from '../../../../core/combat/move-line';

/**
 * The movement row under the map (C4-03, C6-07): the walker, "Restam 9,0 m · 6 quadrados" with its bar, and on a phone the
 * "Mover" button (the one in the screen: from 1024px it is in the turn bar at the top, and the row says "arraste o seu token no
 * mapa" instead). The button is dashed with the reason while an answer is awaited (`locked`), and absent when there is nothing
 * left to move.
 */
@Component({
  selector: 'app-move-row',
  imports: [MatButtonModule, MatIconModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <mat-icon class="row__icon" aria-hidden="true">directions_walk</mat-icon>
    <div class="row__text" role="status">
      <span class="row__line"
        ><b>{{ line().lead }}</b
        >@if (line().rest) { · {{ line().rest }} }@if (drag()) { · arraste o seu token no mapa }</span
      >
      @if (!drag()) {
        <span class="row__bar" aria-hidden="true"><span class="row__fill" [style.width.%]="line().percent"></span></span>
      }
      @if (locked() && !masterWait()) {
        <span class="row__why" [id]="whyId"><mat-icon aria-hidden="true">block</mat-icon>{{ locked() }}</span>
      }
    </div>
    @if (button() && line().canMove) {
      <button
        matButton="outlined"
        type="button"
        class="row__btn"
        [class.row__btn--off]="!!locked()"
        [class.mr-button--off]="!!locked()"
        [disabled]="!!locked() || busy()"
        disabledInteractive
        [attr.aria-describedby]="locked() ? (masterWait() ? waitNoteId : whyId) : null"
        (click)="!locked() && move.emit()"
      >
        <mat-icon aria-hidden="true">open_with</mat-icon>Mover
      </button>
    }
  `,
  styleUrl: './move-row.scss',
})
export class MoveRow {
  readonly own = input.required<Combatant>();
  readonly grappled = input(false);
  /** Why nothing can be done now ("Esperando a reação do mestre"); `''` when free. */
  readonly locked = input('');
  readonly busy = input(false);
  /** The row has the "Mover" button (a phone); without it the movement is done by dragging the token on the map. */
  readonly button = input(true);
  /** The token can be dragged on the map: the row says so, and has no bar. */
  readonly drag = input(false);

  readonly move = output<void>();

  protected readonly line = computed(() => moveLine(this.own(), this.grappled()));
  protected readonly whyId = 'move-why';
  protected readonly waitNoteId = MASTER_WAIT_NOTE_ID;
  /** The wait is the master's: the turn card says it once, and the button points to it. */
  protected readonly masterWait = computed(() => isMasterWait(this.locked()));
}
