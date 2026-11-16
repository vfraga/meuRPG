import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatSelectModule } from '@angular/material/select';

import type { MountKind, MountMarkOption, MountMarkView } from '../../../../core/combat/mount-mark';
import { SaddleIcon } from '../../../../shared/saddle-icon/saddle-icon';

/**
 * The master's "Montaria" select in an opened row of the order (#588, boards 588-08 and 588-09): "Mais" › "Montaria…" opens it,
 * as "Marcar cobertura" opens its picker. The three marks are "Nenhuma", "Adestrada" and "Independente", each with its line; the
 * current one is the picked one, and what it means stays under the select, naming the rider it carries. A pick is sent at once
 * (`SetCombatantMount`). While the mount carries a rider "Nenhuma" is off and the reason is written. A select, not three
 * segments: the column is 288 px wide.
 */
@Component({
  selector: 'app-row-mount',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatButtonModule, MatFormFieldModule, MatSelectModule, SaddleIcon],
  template: `
    <mat-form-field appearance="outline" class="mount__field" subscriptSizing="dynamic">
      <mat-label>Montaria</mat-label>
      <mat-select
        [value]="view().current"
        [disabled]="busy()"
        [aria-label]="'Montaria: ' + label()"
        aria-describedby="mount-hint mount-why"
        (valueChange)="pick.emit($event)"
      >
        <mat-select-trigger><app-saddle-icon class="mount__icon" />{{ currentLabel() }}</mat-select-trigger>
        @for (o of view().options; track o.kind) {
          <mat-option [value]="o.kind" [disabled]="o.disabled">{{ line(o) }}</mat-option>
        }
      </mat-select>
    </mat-form-field>
    <p class="mount__hint" id="mount-hint">{{ view().hint }}</p>
    @if (view().why) {
      <p class="mount__why" id="mount-why">{{ view().why }}</p>
    }
    <button matButton type="button" class="mount__close" (click)="closed.emit()">Fechar</button>
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
    .mount__close {
      align-self: flex-start;
      margin-inline-start: -12px;
    }
  `,
})
export class RowMount {
  readonly view = input.required<MountMarkView>();
  /** The combatant, for the select's name. */
  readonly label = input.required<string>();
  readonly busy = input(false);
  readonly pick = output<MountKind>();
  readonly closed = output<void>();

  protected currentLabel(): string {
    const v = this.view();
    return v.options.find((o) => o.kind === v.current)?.label ?? '';
  }

  /** "Adestrada: age logo depois de quem monta e só usa Disparada, Desengajar e Esquivar." */
  protected line(o: MountMarkOption): string {
    return `${o.label}: ${o.hint.charAt(0).toLowerCase()}${o.hint.slice(1)}`;
  }
}
