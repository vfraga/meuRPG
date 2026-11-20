import { Component, input, output } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import { CheckBox } from '../check-box/check-box';

/** One line of "Quem recebe": a character the master may include. */
export interface Recipient {
  readonly id: string;
  readonly name: string;
  /** "Mago 3 · de Vinicius", or "Mago 3 · 2.600 XP agora". */
  readonly sub: string;
  readonly checked: boolean;
  /** Cannot be checked (the character died). */
  readonly disabled?: boolean;
  /** What this one gets ("+116 XP"), "Não recebe" when unchecked, or empty
   * (a milestone has no number). */
  readonly amount: string;
  /** A state word beside the name ("Caído"), with its icon. */
  readonly tag?: { readonly label: string; readonly icon: string };
  /** A quiet line under the sub ("Está viva, então recebe a parte dela."). */
  readonly note?: string;
}

/**
 * "Quem recebe" (E7-06, E7-07, E7-08): the characters with a checkbox each
 * and, on the right, what each one gets. Shared by the end-of-combat block
 * and the "Dar XP" and "Registrar marco" sheets, so the three read the same.
 * The parent owns who is checked; this only says who was tapped. An
 * unchecked row says it in words ("Não recebe"), never only by the box.
 */
@Component({
  selector: 'app-xp-recipients',
  imports: [CheckBox, MatIconModule],
  template: `
    <ul class="list" [attr.aria-label]="label()">
      @for (r of rows(); track r.id) {
        <li class="row">
          <app-check-box
            [checked]="r.checked"
            [disabled]="r.disabled ?? false"
            [label]="'Marcar ' + r.name"
            (toggle)="toggle.emit(r.id)"
          />
          <span class="who">
            <span class="name">
              {{ r.name }}
              @if (r.tag; as tag) {
                <span class="state"><mat-icon aria-hidden="true">{{ tag.icon }}</mat-icon>{{ tag.label }}</span>
              }
            </span>
            <span class="sub">{{ r.sub }}</span>
            @if (r.note) {
              <span class="sub">{{ r.note }}</span>
            }
          </span>
          @if (r.amount) {
            <span class="amount" [class.amount--off]="!r.checked">{{ r.amount }}</span>
          }
        </li>
      }
    </ul>
  `,
  styleUrl: './xp-recipients.scss',
})
export class XpRecipients {
  readonly rows = input.required<readonly Recipient[]>();
  /** The group's name for a screen reader. */
  readonly label = input('Quem recebe');

  /** The id of the row the person tapped. */
  readonly toggle = output<string>();
}
