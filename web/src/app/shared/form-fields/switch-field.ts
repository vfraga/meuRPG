import { Component, input, output } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

let nextId = 0;

/**
 * A switch with its words (the app's ink track and check, as "Névoa de guerra"): the label and, beside it, the state in a word
 * ("Ligado", "Desligado"), so it never depends on colour. A real `role="switch"` button, a 44 px target.
 */
@Component({
  selector: 'app-switch-field',
  imports: [MatIconModule],
  template: `
    <div class="row" [class.row--off]="disabled()">
      <button
        type="button"
        role="switch"
        class="switch"
        [class.switch--on]="checked()"
        [attr.aria-checked]="checked()"
        [disabled]="disabled()"
        [attr.aria-describedby]="describedby() || null"
        [attr.aria-labelledby]="labelId"
        [attr.data-field]="path() || null"
        (click)="toggled.emit(!checked())"
      >
        <span class="switch__handle">
          @if (checked()) {
            <mat-icon aria-hidden="true">check</mat-icon>
          }
        </span>
      </button>
      <span class="text">
        <span class="label" [id]="labelId">{{ label() }}</span>
        <span class="state">{{ checked() ? onWord() : offWord() }}</span>
      </span>
    </div>
  `,
  styles: `
    :host {
      display: block;
    }

    .row {
      display: flex;
      align-items: center;
      gap: var(--mr-space-3);
      min-height: 48px;
    }

    // A switch that cannot be turned yet: only the control is dimmed, so its words keep their contrast.
    .row--off .switch {
      opacity: 0.55;
      cursor: not-allowed;
    }

    .text {
      display: flex;
      align-items: baseline;
      flex-wrap: wrap;
      gap: 2px 10px;
    }

    .label {
      font-size: var(--mr-font-17);
      font-weight: 700;
    }

    .state {
      font-size: var(--mr-font-14);
      color: var(--mr-ink-muted);
    }

    .switch {
      flex: none;
      position: relative;
      box-sizing: border-box;
      width: 52px;
      height: 32px;
      padding: 0;
      border: 2px solid var(--mr-control-line);
      border-radius: 16px;
      background: var(--mr-surface);
      cursor: pointer;

      &--on {
        border-color: var(--mr-ink);
        background: var(--mr-ink);
      }

      // A 44 px target around the 32 px track (the 28 px padding box plus 8 px each side).
      &::before {
        content: '';
        position: absolute;
        inset: -8px -4px;
      }

      &:focus-visible {
        outline: 2px solid var(--mr-focus);
        outline-offset: 2px;
      }

      &:disabled {
        opacity: 0.5;
        cursor: not-allowed;
      }
    }

    .switch__handle {
      position: absolute;
      top: 50%;
      left: 4px;
      display: flex;
      align-items: center;
      justify-content: center;
      box-sizing: border-box;
      width: 16px;
      height: 16px;
      border-radius: 50%;
      background: var(--mr-control-line);
      transform: translateY(-50%);

      .mat-icon {
        width: 16px;
        height: 16px;
        font-size: var(--mr-font-16);
        color: var(--mr-ink);
      }

      .switch--on & {
        left: 24px;
        width: 24px;
        height: 24px;
        background: var(--mr-surface);
      }

      .switch--on & .mat-icon {
        width: 20px;
        height: 20px;
        font-size: var(--mr-font-20);
      }
    }
  `,
})
export class SwitchField {
  readonly label = input.required<string>();
  readonly checked = input.required<boolean>();
  readonly path = input('');
  /** The switch cannot be turned (what it needs is not chosen yet). */
  readonly disabled = input(false);
  /** The id of the element that explains the switch (why it is disabled). */
  readonly describedby = input('');
  /** The words beside the label: "Ligado"/"Desligado", or "Ligada"/"Desligada" where the noun is feminine. */
  readonly onWord = input('Ligado');
  readonly offWord = input('Desligado');
  readonly toggled = output<boolean>();
  protected readonly labelId = `sw-${nextId++}`;
}
