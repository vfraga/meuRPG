import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { AwaitedSum, KeptRow, Removal } from '../../../../core/combat/group-take-out';

/**
 * What the player's sheet keeps in view while the master's take-out asks one sum again (#835): the notice, the parts of
 * the awaited group, the part taken out, and the sums that stay. Never the target's hit points.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-group-sum-parts',
  imports: [MatIconModule],
  template: `
    <div class="mr-notice mr-notice--neutral" role="status" data-testid="group-notice">
      <mat-icon aria-hidden="true">remove_circle_outline</mat-icon>
      <p>{{ notice() }}</p>
    </div>
    <dl class="parts" aria-label="O dano por parte">
      @for (x of awaited().parts; track x.key) {
        <div class="parts__row">
          <dt>{{ x.labelPt }}</dt>
          <dd>{{ x.diceCount }}d{{ x.diceSides }}{{ x.flat ? ' + ' + x.flat : '' }} de {{ x.damageTypePt }}</dd>
        </div>
      }
      @if (removal(); as r) {
        <div class="parts__row parts__row--gone">
          <dt>
            <s>{{ r.name }}</s>
          </dt>
          <dd>tirado</dd>
        </div>
      }
      @for (k of kept(); track k.label) {
        <div class="parts__row">
          <dt>{{ k.label }}</dt>
          <dd>{{ k.formula }} de {{ k.typePt }}: {{ k.total }} <small>já digitado, fica</small></dd>
        </div>
      }
    </dl>
  `,
  styles: `
    :host {
      display: flex;
      flex-direction: column;
      gap: 12px;
    }
    .parts {
      display: flex;
      flex-direction: column;
      margin: 0;
    }
    .parts__row {
      display: grid;
      grid-template-columns: minmax(0, 1fr) minmax(0, 2fr);
      gap: 12px;
      padding: 8px 0;
      border-top: 1px solid var(--mr-rule);
    }
    dt {
      font-weight: 700;
    }
    dd {
      margin: 0;
    }
    small {
      margin-left: 6px;
      font-size: var(--mr-font-13);
      color: var(--mr-ink-muted);
    }
    .parts__row--gone {
      color: var(--mr-ink-muted);
    }
  `,
})
export class GroupSumParts {
  readonly notice = input.required<string>();
  readonly awaited = input.required<AwaitedSum>();
  readonly kept = input<readonly KeptRow[]>([]);
  /** The part the master took out, when the log says which. */
  readonly removal = input<Removal | undefined>(undefined);
}
