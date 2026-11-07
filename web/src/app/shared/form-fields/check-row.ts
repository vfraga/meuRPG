import { Component, input, output } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

/**
 * A checkbox with its words beside it, the whole row a 44 px target: a real checkbox under a 24 px box that shows a check
 * (the accent only when it is checked, and the check itself says so). `<ng-content>` is anything after the words, such as the
 * "Da mesa" tag.
 */
@Component({
  selector: 'app-check-row',
  imports: [MatIconModule],
  template: `
    <label class="row" [class.row--invalid]="invalid()">
      <input type="checkbox" class="mr-visually-hidden" [checked]="checked()" [disabled]="disabled()" [attr.aria-invalid]="ariaInvalid() || null" [attr.aria-describedby]="describedby() || null" [attr.data-field]="path() || null" (change)="toggled.emit(!checked())" />
      <span class="box" aria-hidden="true"><mat-icon>check</mat-icon></span>
      <span class="text">
        <span class="label" [class.label--strong]="sub() !== ''">{{ label() }}</span>
        @if (sub() !== '') {
          <span class="sub">{{ sub() }}</span>
        }
      </span>
      <ng-content />
    </label>
  `,
  styles: `
    :host {
      display: block;
    }

    .row {
      display: flex;
      align-items: center;
      gap: var(--mr-space-3);
      min-height: 44px;
      cursor: pointer;
    }

    .box {
      flex: none;
      display: flex;
      align-items: center;
      justify-content: center;
      box-sizing: border-box;
      width: 24px;
      height: 24px;
      border: 2px solid var(--mr-control-line);
      border-radius: 5px;
      background: var(--mr-surface);
      color: transparent;

      .mat-icon {
        width: 18px;
        height: 18px;
        font-size: var(--mr-font-18);
      }
    }

    input:checked + .box {
      border-color: var(--mr-accent);
      background: var(--mr-accent);
      color: var(--mr-on-accent);
    }

    input:focus-visible + .box {
      outline: 2px solid var(--mr-focus);
      outline-offset: 2px;
    }

    .text {
      display: flex;
      flex-direction: column;
      min-width: 0;
      padding: var(--mr-space-1) 0;
    }

    .label {
      font-size: var(--mr-font-17);

      &--strong {
        font-weight: 700;
      }
    }

    .sub {
      font-size: var(--mr-font-14);
      color: var(--mr-ink-muted);
    }

    // An invalid row has its words and box in the danger colour.
    .row--invalid {
      .label {
        color: var(--mr-danger-ink);
      }

      .box,
      input:checked + .box {
        border-color: var(--mr-danger-ink);
      }

      input:checked + .box {
        background: var(--mr-danger-ink);
      }
    }
  `,
})
export class CheckRow {
  readonly label = input.required<string>();
  readonly checked = input.required<boolean>();
  readonly disabled = input(false);
  /** One muted line under the words (a trait's effect in a line); the words then turn bold. */
  readonly sub = input('');
  /** The row is in error: its words and box take the danger colour. */
  readonly invalid = input(false);
  /** Tells assistive technology the checkbox is in error. */
  readonly ariaInvalid = input(false);
  /** The id of the element that explains the row. */
  readonly describedby = input('');
  readonly path = input('');
  readonly toggled = output<boolean>();
}
