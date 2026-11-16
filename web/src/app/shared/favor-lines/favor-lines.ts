import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import { FavorState } from '../../core/play/favor-state';

/**
 * What the Inspiração's pass and refund tell their reader, one line each (RN-43, board 581-07, states 2, 5, 6 and 15): the giver's
 * "O mestre aceitou: a sua Inspiração foi para Tor.", the receiver's "Brisa passou a Inspiração para você.", the roller's "O mestre
 * devolveu a sua Inspiração." and the master's notices of a card that closed before the answer. A polite status that does not move the
 * focus; the region is always in the page, so the text that appears in it is announced. Each line has a 44 px "Dispensar o aviso".
 * Nobody else reads anything (RN-10): the lines come from `FavorState`, which holds only what the server told this reader.
 */
@Component({
  selector: 'app-favor-lines',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatIconModule],
  template: `
    <div class="live" role="status" aria-live="polite">
      @for (line of favor.lines(); track line.id) {
        <div
          class="mr-notice"
          [class.mr-notice--success]="line.tone === 'success'"
          [class.mr-notice--neutral]="line.tone === 'neutral'"
          data-testid="favor-line"
        >
          <mat-icon
            [class.mr-icon-filled]="line.icon === 'star'"
            aria-hidden="true"
            >{{ line.icon }}</mat-icon
          >
          <p class="text">{{ line.text }}</p>
          <button
            type="button"
            class="close"
            aria-label="Dispensar o aviso"
            (click)="favor.dismissLine(line.id)"
          >
            <mat-icon aria-hidden="true">close</mat-icon>
          </button>
        </div>
      }
    </div>
  `,
  styles: `
    :host {
      display: block;
    }

    .live:empty {
      display: none;
    }

    .live {
      display: grid;
      gap: var(--mr-space-2);
    }

    .text {
      flex: 1;
      min-width: 0;
      margin: 0;
    }

    .close {
      display: inline-flex;
      flex: none;
      align-items: center;
      justify-content: center;
      width: 44px;
      height: 44px;
      margin: -12px -14px -12px 0;
      border: 0;
      background: transparent;
      color: inherit;
      cursor: pointer;
    }
  `,
})
export class FavorLines {
  protected readonly favor = inject(FavorState);
}
