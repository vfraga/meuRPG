import { Component, input, output } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

let nextId = 0;

/**
 * The editor's switch "Revelado aos jogadores" (README-B): not filled with
 * the accent, because only the primary button is. On: an `ink` track, a
 * `surface` handle and a check. Off: an outlined `control-line` track with
 * a small handle. A real `role="switch"` button, named by its label and
 * described by the hint.
 */
@Component({
  selector: 'app-reveal-switch',
  imports: [MatIconModule],
  template: `
    <div class="sw">
      <ng-content select="[lead]" />
      <span class="sw__text">
        <span class="sw__label" [id]="id + '-label'">{{ label() }}</span>
        <span class="sw__hint" [id]="id + '-hint'">{{ hint() }}</span>
      </span>
      <button
        type="button"
        role="switch"
        class="sw__track"
        [class.sw__track--on]="checked()"
        [class.sw__track--off]="disabled()"
        [attr.aria-disabled]="disabled() ? true : null"
        [attr.aria-checked]="checked()"
        [attr.aria-labelledby]="id + '-label'"
        [attr.aria-describedby]="id + '-hint'"
        (click)="!disabled() && checkedChange.emit(!checked())"
      >
        <span class="sw__handle">
          @if (checked()) {
            <mat-icon aria-hidden="true">check</mat-icon>
          }
        </span>
      </button>
    </div>
  `,
  styleUrl: './reveal-switch.scss',
})
export class RevealSwitch {
  /** Two switches on one page (the point's and the DC's) need their own ids. */
  protected readonly id = `sw-${nextId++}`;
  readonly label = input.required<string>();
  readonly hint = input('');
  /** Controlled: the switch only says what was asked; the parent decides (a refused save leaves it where it was). */
  readonly checked = input(false);
  /** The switch cannot be changed now (the reason is a sentence beside it): `aria-disabled`, still focusable. */
  readonly disabled = input(false);
  readonly checkedChange = output<boolean>();
}
