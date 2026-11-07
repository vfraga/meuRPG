import { NgTemplateOutlet } from '@angular/common';
import { ChangeDetectionStrategy, Component, computed, input, output } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

let nextId = 0;

/** What the card is: a box that ticks, a round choice among several, or a line that is told and not asked. */
export type OptionCardKind = 'checkbox' | 'radio' | 'fixed';

/**
 * An option as a card (MR-025): the whole card is the 44 px target, a real input stays under it (visually hidden, still
 * focusable, so the keyboard and the screen reader get a checkbox or a radio) and the card draws the mark in the --mr-*
 * tokens. Ticked is the accent border and soft fill; one that may not be taken now stays grey and dashed with its reason
 * written; a `fixed` one is a line that applies by itself. Radios of one group share `name` and sit in a radiogroup.
 */
@Component({
  selector: 'app-option-card',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatIconModule, NgTemplateOutlet],
  template: `
    @if (kind() === 'fixed') {
      <div class="opt opt--on opt--fixed">
        <mat-icon class="opt__mark" aria-hidden="true">check_circle</mat-icon>
        <ng-container *ngTemplateOutlet="text" />
      </div>
    } @else {
      <label class="opt" [class.opt--on]="checked()" [class.opt--off]="!!why() || disabled()">
        <input
          class="mr-visually-hidden"
          [type]="kind()"
          [attr.name]="name() || null"
          [checked]="checked()"
          [disabled]="!!why() || disabled()"
          [attr.aria-describedby]="describedByIds()"
          (change)="chosen.emit()"
        />
        <span class="opt__box" [class.opt__box--round]="kind() === 'radio'" aria-hidden="true">
          @if (checked()) {
            @if (kind() === 'radio') {
              <span class="opt__dot"></span>
            } @else {
              <mat-icon>check</mat-icon>
            }
          }
        </span>
        <ng-container *ngTemplateOutlet="text" />
      </label>
    }
    <ng-template #text>
      <span class="opt__text">
        <span class="opt__head">
          <b>{{ title() }}</b>
          @if (cost()) {
            <span class="opt__cost">{{ cost() }}</span>
          }
        </span>
        @if (sub()) {
          <span class="opt__sub" [id]="subId">{{ sub() }}</span>
        }
        @if (why()) {
          <span class="opt__why"><mat-icon aria-hidden="true">block</mat-icon>{{ why() }}</span>
        }
      </span>
    </ng-template>
  `,
  styleUrl: './option-card.scss',
})
export class OptionCard {
  readonly kind = input<OptionCardKind>('checkbox');
  readonly title = input.required<string>();
  readonly checked = input(false);
  /** The radios of one group share a name. */
  readonly name = input('');
  readonly cost = input('');
  readonly sub = input('');
  /** Why it cannot be taken now; set, the card is grey and dashed and the input is off. */
  readonly why = input('');
  /** Off like `why` but with no reason written inside the card (the caller says it on a line of its own). */
  readonly disabled = input(false);
  /** The id of a line outside the card that says more about it ("Sem usos de ..."); read after the sub line. */
  readonly describedBy = input('');
  readonly chosen = output<void>();

  protected readonly subId = `option-card-${nextId++}`;
  protected readonly describedByIds = computed(
    () => [this.sub() ? this.subId : '', this.describedBy()].filter(Boolean).join(' ') || null,
  );
}
