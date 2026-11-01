import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  input,
  model,
  output,
  signal,
  untracked,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { HeldRollOffer } from '../../../gen/meurpg/play/v1/combat_pb';
import { LUCKY_TITLE, luckyText, luckyWhat } from '../../core/combat/lucky';
import {
  type HeldAnswer,
  asksIndomitable,
  asksStroke,
  asksLucky,
  dieOf,
  luckyHeldLine,
  strokeFailed,
} from '../../core/held-rolls/held-roll-offer';
import {
  INDOMITABLE_WHAT,
  STROKE_WHAT,
  indomitableTitle,
  strokeTitle,
} from '../../core/held-rolls/held-text';
import { inspirationDieName, promptTitle, useLabel } from '../../core/resources/bardic-inspiration';
import { MultiRoll, type RollField } from '../multi-roll/multi-roll';
import { RollPicker } from '../roll-picker/roll-picker';

/** Each prompt's own heading id: a sheet and the session panel may both ask on the page. */
let nextId = 0;

/**
 * How much of the held d20 the prompt draws, which is what each place showed before the question was one:
 * - `attack`: nothing (the attack sheet shows the d20 in its own result card), and the die is rolled with the roll
 *   picker;
 * - `sheet`: Lucky's question under "Seu d20: 1d20 (1) + 5 = 6. ..." (a sheet with no result to show it in yet);
 * - `card`: the d20's face and bonus over the question (the scene, the traps, the group check, the hint try, the session
 *   panel), and the die typed in a field of its own.
 */
export type HeldRollLook = 'attack' | 'sheet' | 'card';

/**
 * The question a roll that waits asks its roller (SRD 5.1: the creature decides after seeing the d20 and before the
 * master says whether the roll succeeds), the same wherever it comes: the halfling's Lucky, "Tirou 1: rolar de novo?"
 * ("Rolar de novo", "Digitar o resultado", "Ficar com o 1"), or the Bardic Inspiration die, "Usar a Inspiração de Bardo
 * (d8)?" ("Somar o d8 (Inspiração de Orla)", "Digitar o resultado", "Guardar o dado"). The typed face takes 1 to the die's
 * size, and the way of rolling the table allows (RN-18) is the host's `canApp` and `canType`. It says nothing about
 * whether the roll hit. The host answers with `answer` (a `HeldRollController` takes it).
 */
@Component({
  selector: 'app-held-roll-prompt',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatButtonModule, MatIconModule, MultiRoll, RollPicker],
  templateUrl: './held-roll-prompt.html',
  styleUrl: './held-roll-prompt.scss',
})
export class HeldRollPrompt {
  /** The question: the d20 with the die that shows the 1, or the die the roller holds. */
  readonly offer = input.required<HeldRollOffer>();
  readonly look = input<HeldRollLook>('card');
  readonly canApp = input(true);
  readonly canType = input(true);
  readonly preferApp = input(true);
  readonly busy = input(false);
  /** Typing mode, two-way: the sheet reads it to change its title. */
  readonly typing = model(false);
  /** The player's answer: roll again, keep, add the die, keep the die. */
  readonly answer = output<HeldAnswer>();

  protected readonly titleId = `held-t-${nextId++}`;
  protected readonly lucky = computed(() => asksLucky(this.offer()));
  protected readonly luckyTitle = LUCKY_TITLE;
  protected readonly luckyWhat = computed(() => luckyWhat(this.offer()));
  /** The rogue's question, after the result of an attack or a check: Stroke of Luck turns the miss into a hit or counts the
   * d20 as 20. Nothing is rolled, so there is no picker. */
  protected readonly stroke = computed(() => asksStroke(this.offer()));
  protected readonly strokeTitle = computed(() => strokeTitle(this.offer()));
  protected readonly strokeTold = computed(() => strokeFailed(this.offer()));
  protected readonly strokeWhat = STROKE_WHAT;
  /** The fighter's question, after the result of a saving throw: Indomitable rolls it again and the new roll stands. */
  protected readonly indomitable = computed(() => asksIndomitable(this.offer()));
  protected readonly indomitableTitle = computed(() => indomitableTitle(this.offer()));
  protected readonly indomitableWhat = INDOMITABLE_WHAT;
  /** With advantage or disadvantage the new roll takes both dice: a typed one is the two faces (`app-multi-roll`). */
  protected readonly indomitablePair = computed(() => (this.offer().d20?.faces.length ?? 1) > 1);
  protected readonly indomitableFields: readonly RollField[] = [
    { key: 'd20-1', label: 'Primeiro d20', min: 1, max: 20 },
    { key: 'd20-2', label: 'Segundo d20', min: 1, max: 20 },
  ];
  /** Which question of which roll: it changes when Lucky's answer brings the die's. */
  private readonly question = computed(
    () => `${this.offer().holdId}:${this.lucky()}:${this.indomitable()}:${this.stroke()}`,
  );
  protected readonly die = computed(() => dieOf(this.offer()) ?? { sides: 0, fromName: '' });
  protected readonly dieName = computed(() => inspirationDieName(this.die().sides));
  protected readonly dieTitle = computed(() => promptTitle(this.die().sides));
  /** "Somar o d8 (Inspiração de Orla)". */
  protected readonly useButton = computed(() => useLabel(this.die().sides, this.die().fromName));
  /** The face typed for the die in the card look. */
  protected readonly face = signal(Number.NaN);
  protected readonly faceOk = computed(() => {
    const f = this.face();
    return Number.isInteger(f) && f >= 1 && f <= this.die().sides;
  });
  /** The d20 as it came up: "1", or "1 e 13" for a roll with advantage or disadvantage. */
  protected readonly faces = computed(() => (this.offer().d20?.faces ?? []).join(' e '));
  /** The d20 that counts, for the die's question. */
  protected readonly counted = computed(() => {
    const roll = this.offer().d20;
    return roll?.faces[roll.countedIndex] ?? roll?.faces[0] ?? 0;
  });
  protected readonly bonusText = computed(() => {
    const m = this.offer().d20?.modifier ?? 0;
    return m === 0 ? '' : ` (${m > 0 ? '+' : '−'}${Math.abs(m)} de bônus)`;
  });
  protected readonly heldLine = computed(() => luckyHeldLine(this.offer()));
  /** "Sortudo: 1 → 14": the die's question comes after the halfling's Lucky rolled the d20 again. */
  protected readonly luckyDone = computed(() => luckyText(this.offer().d20));

  constructor() {
    // Lucky's answer moves the same prompt on to the die: the typing the player left open does not carry over.
    effect(() => {
      this.question();
      untracked(() => {
        this.typing.set(false);
        this.face.set(Number.NaN);
      });
    });
  }

  protected onFace(event: Event): void {
    this.face.set((event.target as HTMLInputElement).valueAsNumber);
  }
}
