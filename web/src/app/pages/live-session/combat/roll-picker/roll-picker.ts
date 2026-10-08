import { NgTemplateOutlet } from '@angular/common';
import {
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
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { parseSum, typedTotal } from '../../../../core/combat/combat-dice';
import { tight } from '../../../../core/format/text';

let nextId = 0;

/**
 * The two ways to roll one die, inside the attack sheet, the master's card
 * and the damage step (E6-07, E6-08, RN-18): "Rolar no app" and "Digitar o
 * resultado". The player's saved choice is the filled button and the other a
 * text link; a forced mode (`canApp` or `canType` false) hides the other.
 * Typing is the physical roll: a field of 64px, the live total in a status
 * region, and "Confirmar 16" until the number is valid (a d20 takes 1 to 20,
 * a damage the sum of its dice, N to N times the faces: Q38). The page picks
 * the words with `label` and `hint`; the sheet swaps its title while the
 * person types (`typing`).
 */
@Component({
  selector: 'app-roll-picker',
  imports: [MatButtonModule, MatIconModule, NgTemplateOutlet],
  templateUrl: './roll-picker.html',
  styleUrl: './roll-picker.scss',
})
export class RollPicker {
  /** "Rolar no app" is allowed (the campaign did not force physical dice). */
  readonly canApp = input(true);
  /** "Digitar o resultado" is allowed (the campaign did not force the app). */
  readonly canType = input(true);
  /** The player's choice: the filled button is the app's, or the typed one. */
  readonly preferApp = input(true);
  /** The face range of what is typed: 1 to 20 for a d20, N to N×faces for damage. */
  readonly min = input(1);
  readonly max = input(20);
  /** Added to the typed number: the attack bonus, or the damage modifier. */
  readonly modifier = input(0);
  /** The field's label ("Role 1d20 para o Machado de batalha (+5)"). */
  readonly label = input.required<string>();
  /** The line under it ("Role o seu dado e digite o número que saiu (1 a 20)."). */
  readonly hint = input('');
  /** The caption of the total box ("Total do ataque"). */
  readonly totalNote = input('Total');
  /** The fixed parts added to what is typed, said in the field's place of the modifier ("+ 8 do crítico + 3 de modificador"). */
  readonly fixedText = input('');
  /** What the buttons are called when the way is a damage roll. */
  readonly appLabel = input('Rolar no app');
  /** The master's NPC card has no filled button of its own ("Próximo turno" owns it). */
  readonly outlined = input(false);
  readonly busy = input(false);
  /** The buttons stick to the bottom of the sheet that scrolls (the attack sheet). */
  readonly sticky = input(false);
  /** With `sticky`, the typed error still stands under the field instead of in the sticky
   * footer, so nothing in the footer covers the live total (the scene's roll sheet, E7-04). */
  readonly inlineError = input(false);
  /** The inline roll (the cast's damage, the death save): a label in the small
   * size, the field and the total tighter. The attack sheet keeps its E6-08 size. */
  readonly compact = input(false);
  /** The label stays for screen readers but is not drawn: a page that already says it in its own heading. */
  readonly labelHidden = input(false);
  /** The typed number is the whole answer (no bonus to add): its total box would only say it twice, so it stays for a screen reader alone. */
  readonly hideTotal = input(false);
  /** No empty result box while the number is not typed: only the status text for a screen reader. */
  readonly hideWait = input(false);
  /** Typing mode, two-way: the sheet reads it to change its title. */
  readonly typing = model(false);

  readonly app = output<void>();
  readonly typed = output<number>();

  private readonly injector = inject(Injector);
  private readonly field = viewChild<ElementRef<HTMLInputElement>>('field');
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  protected readonly id = `roll-picker-${nextId++}`;
  protected readonly text = signal('');
  protected readonly value = computed(() => parseSum(this.text(), this.min(), this.max()));
  protected readonly invalid = computed(() => this.text().trim() !== '' && this.value() === null);
  protected readonly showTyping = computed(() => this.typing() || !this.canApp());
  protected readonly errorText = computed(() =>
    tight(`Digite um número de ${this.min()} a ${this.max()}`),
  );
  protected readonly hintText = computed(() => tight(this.hint()));
  /** What a screen reader hears while the well shows only "—". */
  protected readonly waitText = computed(() =>
    tight(`O total aparece quando o número for de ${this.min()} a ${this.max()}.`),
  );
  protected readonly total = computed(() => {
    const v = this.value();
    return v === null ? null : { sum: v + this.modifier(), text: typedTotal(v, this.modifier()) };
  });
  protected readonly bonusText = computed(() => {
    if (this.fixedText()) {
      return this.fixedText();
    }
    const m = this.modifier();
    return m === 0
      ? ''
      : `${m < 0 ? '−' : '+'} ${Math.abs(m)} de ${this.max() === 20 ? 'bônus' : 'modificador'}`;
  });

  /** Back to the two buttons, with the field empty (after a roll went through). */
  reset(): void {
    this.typing.set(false);
    this.text.set('');
  }

  /** The typed number is dropped, the field stays open (what it was rolled for changed). */
  clear(): void {
    this.text.set('');
  }

  /** "Digitar o resultado": the field opens focused and in view, above the
   * buttons that stick to the bottom of a sheet on a short screen. */
  protected startTyping(): void {
    this.typing.set(true);
    afterNextRender(
      () => {
        // The whole typed form (label, field, error, "Confirmar") comes into view: at
        // the top of a sheet that scrolls, in the middle of the page otherwise.
        this.host.nativeElement.scrollIntoView({ block: this.sticky() ? 'start' : 'center' });
        this.field()?.nativeElement.focus({ preventScroll: true });
      },
      { injector: this.injector },
    );
  }

  protected onType(event: Event): void {
    this.text.set((event.target as HTMLInputElement).value);
  }

  protected confirm(): void {
    const v = this.value();
    if (v !== null && !this.busy()) {
      this.typed.emit(v);
    }
  }
}
