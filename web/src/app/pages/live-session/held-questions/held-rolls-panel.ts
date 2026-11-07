import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  computed,
  effect,
  inject,
  input,
  signal,
  untracked,
} from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import { DiceMode } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { HeldQuestions, type MasterQuestion } from '../../../core/held-rolls/held-questions';
import { asksReroll } from '../../../core/held-rolls/held-roll-offer';
import { pendingCountText, questionTitle } from '../../../core/held-rolls/held-text';
import { ResourceClient } from '../../../core/resources/resources-client';
import { PHONE_QUERY, mediaQuery } from '../../../shared/map-view/media-query';
import { HeldQuestion } from './held-question';

/**
 * "Perguntas pendentes" outside a combat (the master's session page): the rolls that wait for their roller's answer, which
 * the master may answer too after ten seconds. It lists what `ListHeldRolls` lists, and reads it again when `tick` moves
 * (the stream's `held_rolls_changed` hint, a reconnection) or the campaign changes; a read that comes back after a newer one
 * is ignored. A question disappears when the server stops listing it, leaving a polite line for a moment. On a phone one
 * question is open and the others are rows that open it. In a combat the questions are in the order, row by row.
 */
@Component({
  selector: 'app-held-rolls-panel',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [HeldQuestion, MatIconModule],
  templateUrl: './held-rolls-panel.html',
  styleUrl: './held-rolls-panel.scss',
})
export class HeldRollsPanel {
  private readonly api = inject(ResourceClient);

  readonly campaignId = input.required<string>();
  /** Moves when the stream says the questions changed, and after a reconnection. */
  readonly tick = input(0);
  readonly diceMode = input<DiceMode>(DiceMode.PLAYERS_CHOOSE);

  protected readonly questions = new HeldQuestions({
    send: (holdId, answer, key) => this.api.answerHeldRoll(this.campaignId(), holdId, answer, key),
  });
  protected readonly phone = mediaQuery(PHONE_QUERY);
  protected readonly canType = computed(() => this.diceMode() !== DiceMode.APP);
  protected readonly countText = computed(() => pendingCountText(this.questions.count()));
  /** The question the master chose to open on a phone. */
  private readonly chosen = signal('');
  private reads = 0;

  /** On a phone, the one open card: the one the master chose, else the first the master can answer, else the first. */
  protected readonly open = computed(() => {
    const live = this.questions.entries().filter((q) => !q.ended());
    return (
      live.find((q) => q.holdId === this.chosen()) ??
      live.find((q) => q.phase() === 'ready') ??
      live[0] ??
      null
    );
  });
  /** The others, as rows. */
  protected readonly rows = computed(() =>
    this.questions.entries().filter((q) => !q.ended() && q !== this.open()),
  );
  /** The ones that ended and still show what came of them. */
  protected readonly ended = computed(() => this.questions.entries().filter((q) => q.ended()));

  constructor() {
    effect(() => {
      this.tick();
      this.campaignId();
      untracked(() => void this.load());
    });
    inject(DestroyRef).onDestroy(() => {
      this.reads++;
      this.questions.dispose();
    });
  }

  protected rowTitle(q: MasterQuestion): string {
    const offer = q.controller.offer();
    return `${q.roller} · ${offer ? questionTitle(offer) : ''}`;
  }

  /** A reroll rule's question is a wait (R7's clock), the others warn once the master may answer. */
  protected foldIcon(q: MasterQuestion): string {
    return q.phase() === 'ready' && !asksReroll(q.controller.offer()) ? 'warning' : 'schedule';
  }

  protected choose(q: MasterQuestion): void {
    this.chosen.set(q.holdId);
  }

  private async load(): Promise<void> {
    const read = ++this.reads;
    try {
      const res = await this.api.listHeldRolls(this.campaignId());
      // A newer read was asked for while this one was on the way: its answer is the one to keep.
      if (read === this.reads) {
        this.questions.sync(res.offers);
      }
    } catch {
      // A hint that did not come back: the panel keeps what it shows and reads again on the next one.
    }
  }
}
