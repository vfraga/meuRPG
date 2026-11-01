import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  inject,
  input,
  model,
  output,
  signal,
  viewChildren,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { parseSum } from '../../core/combat/combat-dice';

let nextId = 0;

/** One number of physical dice: the field's name, and the least and most it can be. */
export interface RollField {
  readonly key: string;
  readonly label: string;
  readonly min: number;
  readonly max: number;
}

/**
 * Rolling more than one number at once: the two d20 of advantage or
 * disadvantage, or the damage's several groups of dice. "Rolar no app" or
 * "Digitar o resultado"; typing opens one labelled field for each number, in
 * the order the dice are rolled, a live total and "Confirmar" once every one is
 * valid. The same two ways as the single-die picker, for more than one die.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-multi-roll',
  imports: [MatButtonModule, MatIconModule],
  templateUrl: './multi-roll.html',
  styleUrl: './multi-roll.scss',
})
export class MultiRoll {
  readonly fields = input.required<readonly RollField[]>();
  readonly canApp = input(true);
  readonly canType = input(true);
  readonly preferApp = input(true);
  /** Added to the typed numbers for the live total (the attack bonus, or the damage's fixed part). */
  readonly modifier = input(0);
  /** "Total do ataque". Without it the live total is not drawn (a field alone is the answer). */
  readonly totalNote = input('');
  /** How the numbers make the total: added up (damage), or the higher or lower one counts (the d20 pair). */
  readonly combine = input<'sum' | 'higher' | 'lower'>('sum');
  readonly appLabel = input('Rolar no app');
  readonly hint = input('');
  readonly busy = input(false);
  readonly typing = model(false);

  readonly app = output<void>();
  readonly typed = output<number[]>();

  private readonly injector = inject(Injector);
  private readonly inputs = viewChildren<ElementRef<HTMLInputElement>>('field');
  protected readonly id = `multi-roll-${nextId++}`;
  protected readonly texts = signal<Partial<Record<string, string>>>({});
  protected readonly showTyping = computed(() => this.typing() || !this.canApp());
  protected readonly values = computed(() =>
    this.fields().map((f) => parseSum(this.texts()[f.key] ?? '', f.min, f.max)),
  );
  protected readonly complete = computed(() => this.values().every((v) => v !== null));
  protected readonly invalid = computed(() =>
    this.fields().map(
      (f, i) => (this.texts()[f.key] ?? '').trim() !== '' && this.values()[i] === null,
    ),
  );
  protected readonly anyInvalid = computed(() => this.invalid().some(Boolean));
  protected readonly total = computed(() => {
    if (!this.complete()) {
      return null;
    }
    const v = this.values() as number[];
    const base =
      this.combine() === 'higher'
        ? Math.max(...v)
        : this.combine() === 'lower'
          ? Math.min(...v)
          : v.reduce((a, n) => a + n, 0);
    return base + this.modifier();
  });

  /** Back to the buttons with the fields empty (the roll went through). */
  reset(): void {
    this.typing.set(false);
    this.texts.set({});
  }

  protected startTyping(): void {
    this.typing.set(true);
    afterNextRender(() => this.inputs()[0]?.nativeElement.focus(), { injector: this.injector });
  }

  protected onType(key: string, event: Event): void {
    this.texts.update((t) => ({ ...t, [key]: (event.target as HTMLInputElement).value }));
  }

  protected rangeText(f: RollField): string {
    return f.min === f.max ? `${f.min}` : `${f.min} a ${f.max}`;
  }

  protected confirm(): void {
    if (this.complete() && !this.busy()) {
      this.typed.emit(this.values() as number[]);
    }
  }
}
