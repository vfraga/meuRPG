import { type WritableSignal, computed, signal } from '@angular/core';

import type {
  DiceRoll,
  Encounter,
  LuckyOffer,
  OutsideInspirationOffer,
  SpellCast,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { InspirationQuestion } from '../../../../core/resources/bardic-inspiration';
import { LuckyQuestion, luckyHeldLine } from '../../../../core/combat/lucky-question';
import type { CombatState } from '../../../../core/combat/combat-state';
import { ActionKey } from '../../../../core/connect/idempotency';
import type {
  InspirationRoll,
  LuckyRoll,
  ResourceClient,
} from '../../../../core/resources/resources-client';
import { classResourceErrorMessage } from '../../../../core/resources/resources-errors';
import { showOfDice, type RollAnimator } from '../../../../shared/roll-overlay/roll-animator';
import { luckyShow } from './cast-sheet-view';

/** What `CastQuestions` reads from the sheet it asks for. */
export interface CastQuestionsContext {
  /** Read when the player answers: a sheet that never asks does not build the client. */
  readonly resources: () => Pick<ResourceClient, 'answerOutsideLucky' | 'answerOutsideInspiration'>;
  readonly campaignId: string;
  readonly name: string;
  readonly state: Pick<CombatState, 'apply'>;
  readonly animator: Pick<RollAnimator, 'play'>;
  readonly busy: WritableSignal<boolean>;
  readonly error: WritableSignal<string>;
  readonly typing: WritableSignal<boolean>;
  /** The cast as the result step shows it. */
  readonly showCast: (encounter: Encounter, cast: SpellCast) => void;
}

/**
 * The questions a spell attack's d20 can be held for: the halfling's Lucky ("Tirou 1: rolar de novo?") first, then the
 * Bardic Inspiration die's. The d20 is rolled and kept on the server; the sheet shows the question, and the answer brings
 * the cast (or the next question on the new roll). Plain class the cast sheet builds.
 */
export class CastQuestions {
  /** The spell attack's d20 was rolled and kept: the character holds a Bardic Inspiration die, and the player decides
   * before the result is told. Nothing was spent yet. */
  readonly offer = signal<OutsideInspirationOffer | null>(null);
  /** The spell attack's d20 was rolled and kept because it shows a 1 and the caster is a halfling: the Lucky question
   * comes first, before the die's. Nothing was spent yet. */
  readonly luckyOffer = signal<LuckyOffer | null>(null);
  /** The held d20, above the Lucky question: the sheet has no result to show it in yet. */
  readonly luckyHeld = computed(() => luckyHeldLine(this.luckyOffer()));
  private readonly answerKey = new ActionKey();
  /** One key per Lucky answer (roll again this way, or keep the 1): the same answer again is a retry. */
  private readonly luckyKey = new ActionKey();

  constructor(private readonly host: CastQuestionsContext) {}

  /** A cast that answered a question instead of the cast (nothing was spent, the d20 is kept on the server): the sheet
   * shows it, Lucky's about a 1 first, else the Bardic Inspiration die's. False for any other error. */
  asks(err: unknown): boolean {
    const question =
      err instanceof LuckyQuestion || err instanceof InspirationQuestion ? err : null;
    if (!question) {
      return false;
    }
    this.host.state.apply(question.encounter);
    if (question instanceof LuckyQuestion) {
      this.luckyOffer.set(question.offer);
    } else {
      this.offer.set(question.offer);
    }
    this.host.typing.set(false);
    // The d20 alone, no total and no word: the result is told after the answer.
    const held = showOfDice(this.host.name, question.offer.d20);
    if (held) {
      this.host.animator.play(held);
    }
    return true;
  }

  /** "Rolar de novo" (rolled in the app, or the typed face) or "Ficar com o 1": the spell is cast now with the new d20
   * (or the 1), or, when the caster holds a Bardic Inspiration die, the die's question comes next, on the new roll. */
  async answerLucky(reroll: boolean, die: LuckyRoll | null): Promise<void> {
    const offer = this.luckyOffer();
    if (!offer || this.host.busy()) {
      return;
    }
    this.host.busy.set(true);
    this.host.error.set('');
    try {
      const res = await this.host
        .resources()
        .answerOutsideLucky(
          this.host.campaignId,
          offer.holdId,
          reroll,
          die,
          this.luckyKey.keyFor({ hold: offer.holdId, reroll, die }),
        );
      const settled = res.result.case === 'spellCast' ? res.result.value : null;
      if (!settled?.encounter || (!settled.cast && !settled.inspirationOffer)) {
        throw new Error('AnswerOutsideLucky answered without its cast');
      }
      this.luckyKey.renew();
      this.luckyOffer.set(null);
      this.host.typing.set(false);
      if (settled.inspirationOffer) {
        this.host.state.apply(settled.encounter);
        this.offer.set(settled.inspirationOffer);
        this.animateLucky(settled.inspirationOffer.d20, die);
        return;
      }
      this.host.showCast(settled.encounter, settled.cast!);
    } catch (err) {
      this.host.error.set(classResourceErrorMessage(err, 'responder ao Sortudo'));
    } finally {
      this.host.busy.set(false);
    }
  }

  private animateLucky(roll: DiceRoll | undefined, die: LuckyRoll | null): void {
    const show = luckyShow(roll, die);
    if (show) {
      this.host.animator.play(show);
    }
  }

  /** The answer about the die: the spell is cast now, with the d20 it rolled and the die when it was used. */
  async answerDie(use: boolean, die: InspirationRoll | null): Promise<void> {
    const offer = this.offer();
    if (!offer || this.host.busy()) {
      return;
    }
    this.host.busy.set(true);
    this.host.error.set('');
    try {
      const res = await this.host
        .resources()
        .answerOutsideInspiration(
          this.host.campaignId,
          offer.holdId,
          use,
          die,
          this.answerKey.keyFor({ hold: offer.holdId, use, die }),
        );
      if (
        res.result.case !== 'spellCast' ||
        !res.result.value.encounter ||
        !res.result.value.cast
      ) {
        throw new Error('AnswerOutsideInspiration answered without its cast');
      }
      this.offer.set(null);
      this.host.showCast(res.result.value.encounter, res.result.value.cast);
    } catch (err) {
      this.host.error.set(classResourceErrorMessage(err, 'responder sobre a Inspiração de Bardo'));
    } finally {
      this.host.busy.set(false);
    }
  }
}
