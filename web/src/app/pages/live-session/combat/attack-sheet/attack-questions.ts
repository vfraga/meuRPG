import { type WritableSignal, signal } from '@angular/core';

import type {
  AttackRoll,
  InspirationOffer,
  LuckyOffer,
  PendingDamage,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { AttackStage } from '../../../../core/combat/attack-flow';
import type { CombatState } from '../../../../core/combat/combat-state';
import { ActionKey } from '../../../../core/connect/idempotency';
import type {
  InspirationRoll,
  LuckyRoll,
  ResourceClient,
} from '../../../../core/resources/resources-client';
import { classResourceErrorMessage } from '../../../../core/resources/resources-errors';
import type { RollAnimator, RollShow } from '../../../../shared/roll-overlay/roll-animator';
import { inspirationShow, luckyRerollShow } from './attack-sheet-view';

/** What `AttackQuestions` reads from the sheet it asks for. */
export interface AttackQuestionsContext {
  readonly resources: Pick<ResourceClient, 'answerLucky' | 'answerBardicInspiration'>;
  readonly campaignId: string;
  readonly encounterId: string;
  readonly state: Pick<CombatState, 'apply'>;
  readonly animator: Pick<RollAnimator, 'play'>;
  readonly busy: WritableSignal<boolean>;
  readonly error: WritableSignal<string>;
  readonly stage: WritableSignal<AttackStage>;
  /** The page learns a roll of this sheet was held for a question, so it does not open the question a second time. */
  readonly onHeld: ((holdId: string) => void) | undefined;
  /** Takes what an answer brought (the roll, the damage, or the next question). */
  readonly settle: (res: {
    readonly roll: AttackRoll;
    readonly pending: PendingDamage | undefined;
    readonly offer?: InspirationOffer;
    readonly lucky?: LuckyOffer;
  }) => void;
}

/**
 * The questions a held attack roll asks the attacker: the halfling's Lucky about a 1 (asked first) and the Bardic
 * Inspiration die. The d20 is rolled and kept on the server; the sheet shows the question, and the answer finishes the
 * attack. Plain class the attack sheet builds.
 */
export class AttackQuestions {
  /** The Bardic Inspiration question of a held roll: the d20 is rolled, the result waits for the answer. */
  readonly offer: WritableSignal<InspirationOffer | null>;
  /** The Lucky question of a held roll whose d20 shows a 1: asked before the die's. */
  readonly luckyOffer: WritableSignal<LuckyOffer | null>;
  /** One key per answer about the die (use it with this die, or keep it): the same answer again is a retry. */
  private readonly answerKeys = new ActionKey();
  /** One key per Lucky answer (roll again this way, or keep the 1): the same answer again is a retry. */
  private readonly luckyKeys = new ActionKey();

  constructor(
    private readonly host: AttackQuestionsContext,
    start: { readonly offer: InspirationOffer | null; readonly lucky: LuckyOffer | null },
  ) {
    this.offer = signal(start.offer);
    this.luckyOffer = signal(start.lucky);
  }

  /** The question the combat now carries for the roll a reaction window held: Lucky first, then the die's. */
  askAfterWindow(lucky: LuckyOffer | undefined, offer: InspirationOffer | undefined): void {
    const held = lucky ?? offer;
    if (!held) {
      return;
    }
    this.luckyOffer.set(lucky ?? null);
    this.offer.set(offer ?? null);
    this.host.onHeld?.(held.holdId);
    this.host.stage.set(lucky ? 'lucky' : 'inspire');
  }

  /** "Rolar de novo" (rolled in the app, or the typed face) or "Ficar com o 1": the answer resolves the held attack, or
   * moves on to the Bardic Inspiration question on the new roll when the attacker holds a die. */
  async answerLucky(reroll: boolean, die: LuckyRoll | null): Promise<void> {
    const offer = this.luckyOffer();
    if (!offer || this.host.busy()) {
      return;
    }
    this.host.busy.set(true);
    this.host.error.set('');
    try {
      const res = await this.host.resources.answerLucky(
        this.host.campaignId,
        this.host.encounterId,
        offer.holdId,
        reroll,
        die,
        this.luckyKeys.keyFor({ holdId: offer.holdId, reroll, die }),
      );
      this.luckyKeys.renew();
      this.host.state.apply(res.encounter);
      this.host.settle(res);
      this.play(luckyRerollShow(res.roll, die));
    } catch (err) {
      this.host.error.set(classResourceErrorMessage(err, 'responder ao Sortudo'));
    } finally {
      this.host.busy.set(false);
    }
  }

  /** "Somar o d8" (rolled in the app, or the typed face) or "Guardar o dado": the answer finishes the held attack. */
  async answerInspiration(use: boolean, die: InspirationRoll | null): Promise<void> {
    const offer = this.offer();
    if (!offer || this.host.busy()) {
      return;
    }
    this.host.busy.set(true);
    this.host.error.set('');
    try {
      const res = await this.host.resources.answerBardicInspiration(
        this.host.campaignId,
        this.host.encounterId,
        offer.holdId,
        use,
        die,
        this.answerKeys.keyFor({ holdId: offer.holdId, use, die }),
      );
      this.answerKeys.renew();
      this.host.state.apply(res.encounter);
      this.host.settle(res);
      this.play(inspirationShow(res.roll, use, die));
    } catch (err) {
      this.host.error.set(classResourceErrorMessage(err, 'responder à Inspiração de Bardo'));
    } finally {
      this.host.busy.set(false);
    }
  }

  private play(show: RollShow | null): void {
    if (show) {
      this.host.animator.play(show);
    }
  }
}
