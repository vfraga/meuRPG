import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

/** One choice of a `Segmented` group. */
export interface Segment<T extends string> {
  readonly value: T;
  readonly label: string;
  readonly icon?: string;
  /** The choice exists but cannot be taken now (the radio is disabled). */
  readonly disabled?: boolean;
}

let nextId = 0;

/**
 * "Andar | Saltar" and "Distância | Altura" (E9-06): a radio group drawn as
 * segments, 48 px tall on a phone and 44 px from the tablet up, the same width
 * each. Real radio buttons under the labels, so the arrow keys move the choice
 * and a screen reader hears "Andar, selecionado, 1 de 2"; the chosen one has a
 * check as well as the filled look (never colour alone). `appearance="track"` is the
 * compact switch of the conversation's composer: the options sit in one rounded,
 * ground-tinted track, 36 px (40 on a phone) high, and the chosen one is a raised
 * bold chip (bold and a border, so never colour alone).
 */
@Component({
  selector: 'app-segmented',
  imports: [MatIconModule],
  host: { '[class.seg--track]': "appearance() === 'track'" },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="seg" role="radiogroup" [attr.aria-label]="label()">
      @for (s of segments(); track s.value) {
        <label class="seg__item" [class.seg__item--on]="s.value === value()" [class.seg__item--off]="s.disabled">
          <input
            type="radio"
            class="mr-visually-hidden"
            [name]="name"
            [value]="s.value"
            [checked]="s.value === value()"
            [disabled]="s.disabled ?? false"
            (change)="choose.emit(s.value)"
          />
          @if (appearance() === 'buttons') {
            @if (s.value === value()) {
              <mat-icon aria-hidden="true">check</mat-icon>
            } @else if (s.icon) {
              <mat-icon aria-hidden="true">{{ s.icon }}</mat-icon>
            }
          }
          {{ s.label }}
        </label>
      }
    </div>
  `,
  styleUrl: './segmented.scss',
})
export class Segmented<T extends string> {
  readonly label = input.required<string>();
  readonly segments = input.required<readonly Segment<T>[]>();
  readonly value = input.required<T>();
  /** `buttons`: separate bordered segments; `track`: one compact rounded track. */
  readonly appearance = input<'buttons' | 'track'>('buttons');
  readonly choose = output<T>();
  protected readonly name = `seg-${nextId++}`;
}
