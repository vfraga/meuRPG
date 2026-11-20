import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { ReleaseBand } from '../../../../core/combat/release-text';

/**
 * The band under the header of a readied action released on the map (#857, #856; boards 857-02, 857-06, 857-07): the alarm icon, what
 * was prepared in bold with the trigger in quotes, and what it costs under it. The cast sheet, the master's area dialog and the Mover
 * page show the same one.
 */
@Component({
  selector: 'app-release-band',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatIconModule],
  template: `
    <div class="rel" data-testid="release-band">
      <mat-icon aria-hidden="true">alarm</mat-icon>
      <p>
        <b>{{ band().lead }}</b> {{ band().quote }}<br />
        <span class="rel__detail">{{ band().detail }}</span>
      </p>
    </div>
  `,
  styles: `
    :host {
      display: block;
    }
    .rel {
      display: flex;
      gap: var(--mr-space-3);
      align-items: flex-start;
      padding: 10px 14px;
      border-radius: var(--mr-radius-md);
      border: 1px solid var(--mr-line);
      background: var(--mr-surface);
      color: var(--mr-ink);
    }
    .rel mat-icon {
      flex: none;
      width: 20px;
      height: 20px;
      font-size: var(--mr-font-20);
      color: var(--mr-accent-text);
    }
    .rel p {
      margin: 0;
      font-size: var(--mr-font-15);
      line-height: 21px;
      overflow-wrap: anywhere;
    }
    .rel__detail {
      color: var(--mr-ink-muted);
      font-size: var(--mr-font-14);
    }
  `,
})
export class ReleaseBandView {
  readonly band = input.required<ReleaseBand>();
}
