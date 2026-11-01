import { ChangeDetectionStrategy, Component, computed, input, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { MasterQuestion } from '../../../core/held-rolls/held-questions';
import {
  asksIndomitable,
  asksLucky,
  asksStroke,
  dieOf,
} from '../../../core/held-rolls/held-roll-offer';
import {
  d20Chip,
  d20Line,
  questionRule,
  questionTitle,
  questionWhat,
} from '../../../core/held-rolls/held-text';
import { MultiRoll, type RollField } from '../../../shared/multi-roll/multi-roll';
import { inspirationDieName, useLabel } from '../../../core/resources/bardic-inspiration';

/** Each question's own heading id: the panel and the rows of the order may both show several. */
let nextId = 0;

const D20_SIDES = 20;

/**
 * A question the master reads about someone else's roll, drawn as its own card (outside a combat) or inside the row of the
 * roller (`layout="row"`): the title, the status ("Brisa responde primeiro · você em 7 s"), what was rolled, the d20 and
 * the answers. The roller answers first; the buttons wait for the master's time and then answer for the roller
 * ("Rolar de novo" in the app or a face typed, "Ficar com o 1", or the die's). The question that ended is a polite
 * line. The seconds of the countdown are not announced: the status line changes only when the question changes.
 */
@Component({
  selector: 'app-held-question',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatButtonModule, MatIconModule, MultiRoll],
  templateUrl: './held-question.html',
  styleUrl: './held-question.scss',
})
export class HeldQuestion {
  readonly question = input.required<MasterQuestion>();
  readonly layout = input<'row' | 'card'>('card');
  /** "Digitar o resultado" is allowed (the campaign did not force the app dice). */
  readonly canType = input(true);
  /** Who the attack was against, when the page knows ("Goblin chefe"). */
  readonly target = input('');

  protected readonly titleId = `held-q-${nextId++}`;
  protected readonly typing = signal(false);
  protected readonly face = signal(Number.NaN);
  protected readonly offer = computed(() => this.question().controller.offer());
  protected readonly phase = computed(() => this.question().phase());
  protected readonly title = computed(() => {
    const offer = this.offer();
    return offer ? questionTitle(offer) : '';
  });
  protected readonly heading = computed(() =>
    this.layout() === 'card' ? `${this.question().roller} · ${this.title()}` : this.title(),
  );
  protected readonly what = computed(() => {
    const offer = this.offer();
    return offer ? questionWhat(offer, this.target()) : '';
  });
  protected readonly rule = computed(() => {
    const offer = this.offer();
    return offer ? questionRule(offer) : { lead: '', text: '' };
  });
  protected readonly chip = computed(() => {
    const offer = this.offer();
    return offer ? d20Chip(offer) : '';
  });
  protected readonly line = computed(() => {
    const offer = this.offer();
    return offer ? d20Line(offer) : '';
  });
  protected readonly lucky = computed(() => asksLucky(this.offer()));
  /** The fighter's question: roll the failed saving throw again, or keep it. */
  protected readonly indomitable = computed(() => asksIndomitable(this.offer()));
  /** The rogue's Stroke of Luck: use it or keep the roll; nothing is rolled, so there is no typed face. */
  protected readonly stroke = computed(() => asksStroke(this.offer()));
  /** The question rolls a d20 again (Lucky's or Indomitable's), not the Bardic Inspiration die. */
  protected readonly rerolls = computed(() => this.lucky() || this.indomitable());
  /** With advantage or disadvantage Indomitable's new roll takes both dice, and a typed one is both faces. */
  protected readonly pair = computed(
    () => this.indomitable() && (this.offer()?.d20?.faces.length ?? 1) > 1,
  );
  protected readonly pairFields: readonly RollField[] = [
    { key: 'd20-1', label: 'Primeiro d20', min: 1, max: 20 },
    { key: 'd20-2', label: 'Segundo d20', min: 1, max: 20 },
  ];
  /** The size of the die the typed face is for: the d20 of a reroll, or the Bardic Inspiration die. */
  protected readonly sides = computed(() =>
    this.rerolls() ? D20_SIDES : (dieOf(this.offer())?.sides ?? 0),
  );
  protected readonly dieName = computed(() => inspirationDieName(this.sides()));
  protected readonly useText = computed(() =>
    useLabel(this.sides(), dieOf(this.offer())?.fromName ?? ''),
  );
  protected readonly faceOk = computed(() => {
    const f = this.face();
    return Number.isInteger(f) && f >= 1 && f <= this.sides();
  });
  protected readonly waiting = computed(() => this.phase() !== 'ready');
  protected readonly icon = computed(() =>
    this.phase() === 'waiting' || this.phase() === 'too-early' ? 'schedule' : 'warning',
  );
  protected readonly noteIcon = computed(() => (this.phase() === 'done' ? 'check' : 'info'));
  protected readonly failure = computed(() => {
    const q = this.question();
    return q.controller.refusal() ? '' : q.controller.error();
  });

  protected useStroke(): void {
    void this.question().answer({ kind: 'stroke', use: true });
  }

  protected rollAgain(): void {
    const roll = { inApp: true } as const;
    void this.question().answer(
      this.lucky()
        ? { kind: 'lucky', reroll: true, roll }
        : this.indomitable()
          ? { kind: 'indomitable', reroll: true, roll }
          : { kind: 'die', use: true, roll },
    );
  }

  protected keep(): void {
    void this.question().answer(
      this.stroke()
        ? { kind: 'stroke', use: false }
        : this.lucky()
          ? { kind: 'lucky', reroll: false, roll: null }
          : this.indomitable()
            ? { kind: 'indomitable', reroll: false, roll: null }
            : { kind: 'die', use: false, roll: null },
    );
  }

  protected sendTyped(): void {
    if (!this.faceOk()) {
      return;
    }
    const roll = { face: this.face() };
    void this.question().answer(
      this.lucky()
        ? { kind: 'lucky', reroll: true, roll }
        : this.indomitable()
          ? { kind: 'indomitable', reroll: true, roll }
          : { kind: 'die', use: true, roll },
    );
    this.typing.set(false);
  }

  protected sendPair(faces: number[]): void {
    void this.question().answer({ kind: 'indomitable', reroll: true, roll: { faces } });
    this.typing.set(false);
  }

  protected onFace(event: Event): void {
    this.face.set((event.target as HTMLInputElement).valueAsNumber);
  }
}
