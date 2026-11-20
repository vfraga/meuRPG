import {
  ChangeDetectionStrategy,
  Component,
  afterNextRender,
  input,
  output,
  viewChild,
} from '@angular/core';
import { MatIconModule } from '@angular/material/icon';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatSelect, MatSelectModule } from '@angular/material/select';

import type { MountKind, MountMarkView } from '../../../../core/combat/mount-mark';
import { SaddleIcon } from '../../../../shared/saddle-icon/saddle-icon';

/**
 * The master's "Montaria" select in an opened row of the order: "Mais" › "Montaria…" opens it,
 * as "Marcar cobertura" opens its picker. The three marks are "Nenhuma", "Adestrada" and "Independente", each with its line under
 * the name; the current one is the picked one, and what it means stays under the select, naming the rider it carries. A pick is
 * sent at once (`SetCombatantMount`). While the mount carries a rider "Nenhuma" is off and the reason is written; with no mark
 * yet, a note says what marking does. While a call is in the air the options are off and the select keeps the focus. It has no button of its own: "Montaria…" closes it again. A select, not three segments:
 * the column is 288 px wide.
 */
@Component({
  selector: 'app-row-mount',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatFormFieldModule, MatIconModule, MatSelectModule, SaddleIcon],
  template: `
    @if (view().note) {
      <p class="mount__note">{{ view().note }}</p>
    }
    <mat-form-field appearance="outline" class="mount__field" subscriptSizing="dynamic">
      <mat-label>Montaria</mat-label>
      <mat-select
        [value]="view().current"
        [attr.aria-busy]="busy()"
        [aria-label]="'Montaria: ' + label()"
        aria-describedby="mount-hint mount-why"
        [hideSingleSelectionIndicator]="true"
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
            <span class="opt__check">
              @if (o.kind === view().current) {
                <mat-icon aria-hidden="true">check</mat-icon>
              }
            </span>
            <span class="opt__body">
              <b class="opt__t">{{ o.label }}</b>
              <span class="opt__d">{{ o.hint }}</span>
            </span>
          </mat-option>
        }
      </mat-select>
    </mat-form-field>
    <p class="mount__hint" id="mount-hint">{{ view().hint }}</p>
    @if (view().why) {
      <p class="mount__why" id="mount-why">{{ view().why }}</p>
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
    // Above the field: the open list lies over what is under it, and the note must stay in view.
    .mount__note {
      margin: 0;
      padding-bottom: 6px;
      border-bottom: 1px solid var(--mr-line);
      font-size: var(--mr-font-13);
      line-height: 18px;
      color: var(--mr-ink-muted);
    }
    // Two lines an option: the name, and what the mark means under it.
    .mount__opt {
      height: auto;
      min-height: 48px;
      padding-block: 6px;
    }
    .mount__opt .mdc-list-item__primary-text {
      display: flex;
      align-items: flex-start;
      gap: 8px;
    }
    // The chosen option: the check on the left, the name in the accent.
    .opt__check {
      flex: none;
      width: 20px;
      height: 20px;
      color: var(--mr-accent-text);
    }
    .opt__check .mat-icon {
      width: 20px;
      height: 20px;
      font-size: 20px;
    }
    .mount__opt.mdc-list-item--selected .opt__t {
      color: var(--mr-accent-text);
    }
    .opt__body {
      min-width: 0;
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
