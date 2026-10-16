import { Component, input, output } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { SlotRow } from '../../core/combat/cast-flow';
import { SlotDots } from '../slot-dots/slot-dots';

let nextId = 0;

/**
 * "Espaço de magia a gastar" (E6-09, E6-28): one radio card for each circle
 * the spell can be cast with, as the paper sheet draws the slots (a free ring,
 * a used disc crossed out) and "1 livre de 4". A circle with no free slot is
 * listed, dashed and disabled, with "Sem espaço livre" on its own line. Native
 * radios under the cards, so the arrows and screen readers work as in any
 * radio group; the chosen card has the 2px accent frame and a check.
 */
@Component({
  selector: 'app-slot-picker',
  imports: [MatIconModule, SlotDots],
  template: `
    <span class="cap" [id]="id + '-l'">{{ label() }}</span>
    <div class="rows" role="radiogroup" [attr.aria-labelledby]="id + '-l'">
      @for (r of rows(); track r.level + (r.pact ? 'p' : '')) {
        <label class="row" [class.row--off]="!r.enabled">
          <input
            type="radio"
            class="mr-visually-hidden"
            [name]="id"
            [checked]="isChosen(r)"
            [disabled]="!r.enabled"
            (change)="pick.emit(r)"
          />
          <span class="row__text">
            <b class="row__title">{{ r.title }}</b>
            <span class="row__count">
              @if (r.total !== null) {
                <app-slot-dots [total]="r.total" [used]="r.used" size="small" />
              }
              {{ r.count }}
            </span>
            @if (!r.enabled) {
              <span class="row__why"><mat-icon aria-hidden="true">block</mat-icon>Sem espaço livre</span>
            }
          </span>
          @if (isChosen(r)) {
            <mat-icon class="row__check" aria-hidden="true">check_circle</mat-icon>
          } @else if (r.enabled) {
            <mat-icon class="row__empty" aria-hidden="true">radio_button_unchecked</mat-icon>
          }
        </label>
      }
    </div>
  `,
  styleUrl: './slot-picker.scss',
})
export class SlotPicker {
  readonly rows = input.required<readonly SlotRow[]>();
  readonly chosen = input<SlotRow | null>(null);
  readonly label = input('Espaço de magia a gastar');
  readonly pick = output<SlotRow>();

  protected readonly id = `slot-picker-${nextId++}`;

  protected isChosen(r: SlotRow): boolean {
    const c = this.chosen();
    return !!c && c.level === r.level && c.pact === r.pact;
  }
}
