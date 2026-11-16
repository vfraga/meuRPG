import {
  ChangeDetectionStrategy,
  Component,
  afterNextRender,
  input,
  output,
  viewChild,
} from '@angular/core';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatSelect, MatSelectModule } from '@angular/material/select';

import type { MountKind, MountMarkView } from '../../../../core/combat/mount-mark';
import { SaddleIcon } from '../../../../shared/saddle-icon/saddle-icon';

/**
 * The master's "Montaria" select in an opened row of the order (#588, boards 588-08 and 588-09): "Mais" › "Montaria…" opens it,
 * as "Marcar cobertura" opens its picker. The three marks are "Nenhuma", "Adestrada" and "Independente", each with its line under
 * the name; the current one is the picked one, and what it means stays under the select, naming the rider it carries. A pick is
 * sent at once (`SetCombatantMount`). While the mount carries a rider "Nenhuma" is off and the reason is written; with no mark
 * yet, a note says what marking does. While a call is in the air the options are off and the select keeps the focus. It has no button of its own: "Montaria…" closes it again. A select, not three segments:
 * the column is 288 px wide.
 */
@Component({
  selector: 'app-row-mount',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatFormFieldModule, MatSelectModule, SaddleIcon],
  template: `
    <mat-form-field appearance="outline" class="mount__field" subscriptSizing="dynamic">
      <mat-label>Montaria</mat-label>
      <mat-select
        [value]="view().current"
        [attr.aria-busy]="busy()"
        [aria-label]="'Montaria: ' + label()"
        aria-describedby="mount-hint mount-why"
        (valueChange)="pick.emit($event)"
      >
        <mat-select-trigger>
          @if (view().current) {
            <app-saddle-icon class="mount__icon" />
          }
          {{ currentLabel() }}
        </mat-select-trigger>
        @for (o of view().options; track o.kind) {
          <mat-option [value]="o.kind" [disabled]="o.disabled || busy()" class="mount__opt">
            <b class="opt__t">{{ o.label }}</b>
            <span class="opt__d">{{ o.hint }}</span>
          </mat-option>
        }
      </mat-select>
    </mat-form-field>
    <p class="mount__hint" id="mount-hint">{{ view().hint }}</p>
    @if (view().why) {
      <p class="mount__why" id="mount-why">{{ view().why }}</p>
    }
    @if (view().note) {
      <p class="mount__note">{{ view().note }}</p>
    }
  `,
  styles: `
    :host {
      display: flex;
      flex-direction: column;
      gap: 6px;
      min-width: 0;
    }
    .mount__field {
      width: 100%;
    }
    .mount__icon {
      margin-inline-end: 8px;
    }
    .mount__hint,
    .mount__why {
      margin: 0;
      font-size: var(--mr-font-13);
      font-weight: 600;
      line-height: 18px;
      color: var(--mr-ink-muted);
      overflow-wrap: anywhere;
    }
    .mount__note {
      margin: 0;
      padding-top: 6px;
      border-top: 1px solid var(--mr-line);
      font-size: var(--mr-font-13);
      line-height: 18px;
      color: var(--mr-ink-muted);
    }
    // Two lines an option (588-09): the name, and what the mark means under it.
    .mount__opt {
      height: auto;
      min-height: 48px;
      padding-block: 6px;
    }
    .opt__t,
    .opt__d {
      display: block;
    }
    .opt__d {
      font-size: var(--mr-font-13);
      line-height: 18px;
      color: var(--mr-ink-muted);
      white-space: normal;
    }
  `,
})
export class RowMount {
  readonly view = input.required<MountMarkView>();
  /** The combatant, for the select's name. */
  readonly label = input.required<string>();
  readonly busy = input(false);
  readonly pick = output<MountKind>();
  private readonly select = viewChild.required(MatSelect);

  constructor() {
    // Opened from "Mais" › "Montaria…": the focus goes into the select, as it goes into the cover picker.
    afterNextRender(() => this.select().focus());
  }

  protected currentLabel(): string {
    const v = this.view();
    return v.options.find((o) => o.kind === v.current)?.label ?? '';
  }
}
