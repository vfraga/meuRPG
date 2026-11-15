import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { MountRow } from '../../../../core/combat/mount-plan';

/**
 * The "Montaria" panel of the "Mover" page (RN-21): one row for each mount the server lists, with what mounting costs, or why
 * it cannot be done now. A refused row keeps its button where it is, dashed and off, so the list does not jump about.
 */
@Component({
  selector: 'app-mount-panel',
  imports: [MatButtonModule, MatIconModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <section class="mp" aria-labelledby="mount-panel-title">
      <header class="mp__head">
        <h2 id="mount-panel-title" class="mp__title"><mat-icon aria-hidden="true">pets</mat-icon>Montaria</h2>
        <span class="mp__rule">custa metade do deslocamento</span>
      </header>
      <ul class="mp__list">
        @for (row of rows(); track row.id) {
          <li class="mp__row" [class.mp__row--off]="row.refused">
            <span class="mp__disc" aria-hidden="true">{{ row.initial }}</span>
            <span class="mp__text">
              <strong class="mp__name">{{ row.name }}</strong>
              <span class="mp__line">
                @if (row.refused) {
                  <mat-icon aria-hidden="true">block</mat-icon>
                }{{ row.line }}
              </span>
            </span>
            <button
              matButton="outlined"
              type="button"
              class="mp__go"
              [class.mp__go--off]="row.refused"
              [disabled]="row.refused || busy()"
              disabledInteractive
              [attr.aria-label]="'Montar ' + row.name"
              (click)="mount.emit(row.id)"
            >
              <mat-icon aria-hidden="true">pets</mat-icon>{{ busy() ? 'Montando…' : 'Montar' }}
            </button>
          </li>
        }
      </ul>
    </section>
  `,
  styleUrl: './mount-panel.scss',
})
export class MountPanel {
  readonly rows = input.required<readonly MountRow[]>();
  readonly busy = input(false);
  readonly mount = output<string>();
}
