import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  inject,
  input,
  output,
} from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

/** One choice of a `ChoiceRow`. */
export interface Choice<T> {
  readonly value: T;
  readonly label: string;
}

/**
 * One choice among a few, drawn as the "Gerar masmorra" page does (E10-05 1): separate chips that wrap (`chips`, the size and the shape) or
 * one bordered strip (`strip`, the corridors and the doors). It is a radio group: Tab reaches the chosen one, the arrow keys move the
 * choice (and the focus) and wrap around, and the chosen one is soft garnet, bold and checked, never colour alone. Every target is at
 * least 44 px high (48 px on a phone).
 */
@Component({
  selector: 'app-choice-row',
  imports: [MatIconModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="row" [class.row--strip]="variant() === 'strip'" [class.row--sym]="symmetric()" role="radiogroup" [attr.aria-label]="label()" [attr.aria-describedby]="describedBy()" (keydown)="onKey($event)">
      @for (c of choices(); track c.label; let i = $index) {
        <button type="button" role="radio" class="row__item" [class.row__item--on]="isOn(c)" [attr.aria-checked]="isOn(c)" [tabindex]="isStop(i) ? 0 : -1" [disabled]="disabled()" (click)="pick(c)">
          <mat-icon class="chk" [class.chk--off]="!isOn(c)" aria-hidden="true">check</mat-icon>{{ c.label }}
        </button>
      }
    </div>
  `,
  styleUrl: './choice-row.scss',
})
export class ChoiceRow<T> {
  readonly label = input.required<string>();
  readonly choices = input.required<readonly Choice<T>[]>();
  readonly value = input.required<T | null>();
  readonly variant = input<'chips' | 'strip'>('chips');
  readonly disabled = input(false);
  /** Chips with the same room on both sides of the words (the check's on the left, a spacer on the right), so the words are centred whether or not the chip is the chosen one. */
  readonly symmetric = input(false);
  readonly describedBy = input<string | null>(null);
  readonly valueChange = output<T>();

  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);

  protected isOn(c: Choice<T>): boolean {
    return c.value === this.value();
  }

  /** The tab stop: the chosen one, or the first when none is chosen. */
  protected isStop(index: number): boolean {
    const chosen = this.choices().findIndex((c) => this.isOn(c));
    return index === (chosen < 0 ? 0 : chosen);
  }

  protected pick(c: Choice<T>): void {
    if (!this.disabled() && !this.isOn(c)) {
      this.valueChange.emit(c.value);
    }
  }

  protected onKey(event: KeyboardEvent): void {
    const step =
      event.key === 'ArrowRight' || event.key === 'ArrowDown'
        ? 1
        : event.key === 'ArrowLeft' || event.key === 'ArrowUp'
          ? -1
          : 0;
    if (step === 0 || this.disabled()) {
      return;
    }
    event.preventDefault();
    const choices = this.choices();
    const at = Math.max(
      0,
      choices.findIndex((c) => this.isOn(c)),
    );
    const next = choices[(at + step + choices.length) % choices.length];
    if (next) {
      this.valueChange.emit(next.value);
      // The chosen one holds the tab stop once the view updates; move the focus there.
      setTimeout(() => {
        const radios = this.host.nativeElement.querySelectorAll<HTMLElement>('[role="radio"]');
        radios[(at + step + choices.length) % choices.length]?.focus();
      });
    }
  }
}
