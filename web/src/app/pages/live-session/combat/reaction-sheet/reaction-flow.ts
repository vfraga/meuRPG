import { type WritableSignal, signal } from '@angular/core';

import {
  type ConcentrationSaveResult,
  EncounterBlockedReason,
  type LuckyOffer,
  type OutsideInspirationOffer,
  type ReactionResult,
  type ReactionWindow,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { SlotRow } from '../../../../core/combat/cast-flow';
import { type CombatClient } from '../../../../core/combat/combat-client';
import {
  type ConcentrationAnswer,
  type ReactionAnswer,
} from '../../../../core/combat/combat-client-types';
import { combatErrorMessage, encounterBlocked } from '../../../../core/combat/combat-errors';
import { luckyReroll } from '../../../../core/combat/lucky-question';
import { isOptionalReaction } from '../../../../core/combat/reaction-autopass';
import type { SlotAfter } from '../../../../core/combat/reactions';
import { ActionKey } from '../../../../core/connect/idempotency';
import { ExtraDiceState } from '../../../../core/effects/extra-dice-state';
import type {
  InspirationRoll,
  LuckyRoll,
  ResourceClient,
} from '../../../../core/resources/resources-client';
import { classResourceErrorMessage } from '../../../../core/resources/resources-errors';
import {
  type RollAnimator,
  type RollShow,
  showOfConcentration,
  showOfDie,
  showOfReaction,
} from '../../../../shared/roll-overlay/roll-animator';
import type { ReactionSheetData, ReactionSheetResult } from './reaction-sheet-data';
import { slotAfter } from './reaction-sheet-view';

/** The three steps of the sheet: the question, the die, the answer. */
export type Step = 'ask' | 'roll' | 'result';

/** What the answer did, once the server told it. */
export type Answered =
  | { readonly kind: 'reaction'; readonly result: ReactionResult }
  | { readonly kind: 'concentration'; readonly result: ConcentrationSaveResult };

/** What the flow reads from the sheet that builds it. */
export interface ReactionFlowHost {
  readonly data: ReactionSheetData;
  readonly api: CombatClient;
  /** Read when the player answers about the die: a sheet that never asks does not build the client. */
  readonly resources: () => ResourceClient;
  readonly animator: RollAnimator;
  readonly close: (result?: ReactionSheetResult) => void;
  /** The window being asked: the first one, then the second step the answer opened. */
  readonly asked: () => ReactionWindow;
  readonly chosen: () => SlotRow | null;
}

/**
 * The calls of the reaction sheet and what they leave: the answer, the concentration save, the questions about the
 * Lucky and the Bardic Inspiration die, the monk's second step. The sheet keeps what the question collects and draws;
 * this keeps the step it is on, what is in the air and what came back.
 */
export class ReactionFlow {
  /** The sheet opened on the monk's second step (after a reload): it starts at "Devolver?". */
  readonly secondStep: boolean;
  readonly step: WritableSignal<Step>;
  readonly busy = signal(false);
  readonly error = signal('');
  readonly typingSave = signal(false);
  /** The concentration save's d20 was rolled and kept: the character holds a Bardic Inspiration die, and the player
   * decides before the result is told. The window stays open. */
  readonly offer = signal<OutsideInspirationOffer | null>(null);
  /** The concentration save's d20 was rolled and kept because it shows a 1 and the character is a halfling: the Lucky
   * question ("Tirou 1: rolar de novo?") comes first, before the die's. The window stays open. */
  readonly luckyOffer = signal<LuckyOffer | null>(null);
  /** The d4 an effect adds to the typed saving throw (Bênção, Perdição): asked when the server says the roll takes them. */
  readonly extra = new ExtraDiceState();
  /** What the answer did, once the server told it. */
  readonly answered = signal<Answered | null>(null);
  readonly afterSlot = signal<SlotAfter | null>(null);
  /** The monk's second step: the window that asks "Devolver?". */
  readonly nextWindowId: WritableSignal<string>;
  /** When the player moved to the die (ms), after the server was told; `null` before. */
  readonly rollingAt = signal<number | null>(null);
  /** One key per answer (its slot, its die): a repeated tap is a retry, another answer is a new request. */
  private readonly keys = new ActionKey();
  private readonly dieKey = new ActionKey();
  /** One key per Lucky answer (roll again this way, or keep the 1): the same answer again is a retry. */
  private readonly luckyKey = new ActionKey();

  constructor(private readonly host: ReactionFlowHost) {
    this.secondStep = host.data.window.prompt.case === 'deflectThrow';
    this.step = signal<Step>(this.secondStep ? 'result' : 'ask');
    this.nextWindowId = signal(this.secondStep ? host.data.window.id : '');
  }

  /**
   * The player moves to the die: the server is told (the master's clock would pass this reaction at 30 s, in the
   * middle of the roll) and the longer wait counts from now. Telling is best effort: if it fails, the roll goes on
   * with the 30 s clock, as before.
   */
  goRoll(): void {
    this.step.set('roll');
    const w = this.host.asked();
    if (this.rollingAt() !== null || !isOptionalReaction(w)) {
      return;
    }
    this.rollingAt.set(Date.now());
    void this.tellRolling(w.id);
  }

  private async tellRolling(windowId: string): Promise<void> {
    try {
      const e = await this.host.api.attacks.markReactionRolling(
        this.host.data.campaignId,
        this.host.data.encounterId,
        windowId,
      );
      this.host.data.state.apply(e);
    } catch {
      // The window may have been answered meanwhile; the stream shows the truth.
      this.rollingAt.set(null);
    }
  }

  async send(answer: ReactionAnswer): Promise<void> {
    const w = this.host.asked();
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.host.api.attacks.answerReaction(
        this.host.data.campaignId,
        this.host.data.encounterId,
        w.id,
        answer,
        this.keys.keyFor({ window: w.id, answer }),
      );
      this.host.data.state.apply(res.encounter);
      if (!answer.use) {
        this.host.close();
        return;
      }
      if (res.result) {
        this.afterSlot.set(slotAfter(answer, this.host.chosen()));
        this.nextWindowId.set(res.result.nextWindowId);
        this.answered.set({ kind: 'reaction', result: res.result });
        // The dice its result sheet shows, tumbling (a physical die is never animated).
        this.play(() => showOfReaction(res.result));
      }
      this.step.set('result');
      this.typingSave.set(false);
    } catch (err) {
      await this.failed(err, answer, 'usar a reação');
    } finally {
      this.busy.set(false);
    }
  }

  async concentrate(how: ConcentrationAnswer): Promise<void> {
    if (this.busy()) {
      return;
    }
    const w = this.host.asked();
    const extra = this.extra.take(how.kind === 'typed');
    if (extra === null) {
      this.error.set(this.extra.missingText());
      return;
    }
    const sent: ConcentrationAnswer =
      how.kind === 'typed' && extra.length > 0 ? { ...how, extra } : how;
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.host.api.attacks.resolveConcentrationSave(
        this.host.data.campaignId,
        this.host.data.encounterId,
        w.id,
        sent,
        this.keys.keyFor({ window: w.id, how: sent }),
      );
      this.host.data.state.apply(res.encounter);
      if (res.lucky || res.offer) {
        // Nothing was written: the window stays open and the question comes (Lucky's first).
        this.luckyOffer.set(res.lucky ?? null);
        this.offer.set(res.offer ?? null);
        this.typingSave.set(false);
        return;
      }
      if (!res.result) {
        // Left to the master: nothing more to read here.
        this.host.close();
        return;
      }
      this.showSave(res.result);
    } catch (err) {
      const more = this.extra.fromRefusal(err);
      if (more) {
        this.error.set(more);
        return;
      }
      await this.failed(err, null, 'resolver o teste de concentração');
    } finally {
      this.busy.set(false);
    }
  }

  private showSave(result: ConcentrationSaveResult): void {
    this.answered.set({ kind: 'concentration', result });
    this.play(() => showOfConcentration(result));
    this.step.set('result');
    this.typingSave.set(false);
  }

  /** "Rolar de novo" (rolled in the app, or the typed face) or "Ficar com o 1": the save is settled now with the new d20
   * (or the 1), or, when the character holds a Bardic Inspiration die, the die's question comes next, on the new roll. */
  async answerLucky(reroll: boolean, die: LuckyRoll | null): Promise<void> {
    const offer = this.luckyOffer();
    if (!offer || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.host
        .resources()
        .answerOutsideLucky(
          this.host.data.campaignId,
          offer.holdId,
          reroll,
          die,
          this.luckyKey.keyFor({ hold: offer.holdId, reroll, die }),
        );
      const settled = res.result.case === 'concentrationSave' ? res.result.value : null;
      if (!settled?.encounter || (!settled.result && !settled.inspirationOffer)) {
        throw new Error('AnswerOutsideLucky answered without its concentration save');
      }
      this.host.data.state.apply(settled.encounter);
      this.luckyKey.renew();
      this.luckyOffer.set(null);
      if (settled.inspirationOffer) {
        this.offer.set(settled.inspirationOffer);
        this.tumbleReroll(die, settled.inspirationOffer);
        return;
      }
      this.showSave(settled.result!);
    } catch (err) {
      this.error.set(classResourceErrorMessage(err, 'responder ao Sortudo'));
    } finally {
      this.busy.set(false);
    }
  }

  /** The d20 the app rolled again, tumbling: the new face alone, "1 → 14" (the die's question shows the rest). */
  private tumbleReroll(die: LuckyRoll | null, offer: OutsideInspirationOffer): void {
    const again = die && 'inApp' in die ? luckyReroll(offer.d20) : undefined;
    if (again) {
      this.play(() =>
        showOfDie('Sortudo', offer.d20?.diceSides ?? 0, again.to, `${again.from} → ${again.to}`),
      );
    }
  }

  /** The answer about the die: the save is settled now, with the d20 it rolled and the die when it was used. */
  async answerDie(use: boolean, die: InspirationRoll | null): Promise<void> {
    const offer = this.offer();
    if (!offer || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.host
        .resources()
        .answerOutsideInspiration(
          this.host.data.campaignId,
          offer.holdId,
          use,
          die,
          this.dieKey.keyFor({ hold: offer.holdId, use, die }),
        );
      const settled = res.result.case === 'concentrationSave' ? res.result.value : null;
      if (!settled?.encounter || !settled.result) {
        throw new Error('AnswerOutsideInspiration answered without its concentration save');
      }
      this.host.data.state.apply(settled.encounter);
      this.offer.set(null);
      this.showSave(settled.result);
    } catch (err) {
      this.error.set(classResourceErrorMessage(err, 'responder sobre a Inspiração de Bardo'));
    } finally {
      this.busy.set(false);
    }
  }

  /** The dice tumble; whatever happens here never turns an answered reaction into an error. */
  private play(make: () => RollShow | null): void {
    try {
      const show = make();
      if (show) {
        this.host.animator.play(show);
      }
    } catch {
      // The animation is a nicety: the result is already on the sheet.
    }
  }

  /** A refusal: the die Counterspell needed asks for itself; an answer out of order reads the combat again. */
  private async failed(err: unknown, answer: ReactionAnswer | null, what: string): Promise<void> {
    const blocked = encounterBlocked(err);
    if (blocked?.reason === EncounterBlockedReason.REACTION_NEEDS_ROLL && answer && !answer.die) {
      this.goRoll();
      return;
    }
    this.error.set(combatErrorMessage(err, what));
    if (blocked?.reason === EncounterBlockedReason.NOT_YOUR_TURN_TO_ANSWER) {
      try {
        const fresh = await this.host.api.turn.get(this.host.data.campaignId);
        if (fresh) {
          this.host.data.state.apply(fresh);
        }
      } catch {
        // The stream's next event reads it again.
      }
    }
  }

  back(): void {
    this.step.set('ask');
    this.error.set('');
  }

  /** "Devolver (1 de chi)": the page opens the attack, which names this window. */
  throwBack(): void {
    const id = this.nextWindowId();
    if (id) {
      this.host.close({ throwWindowId: id });
    }
  }

  /** "Guardar a flecha": the second step is passed. */
  async keepMissile(): Promise<void> {
    const id = this.nextWindowId();
    if (!id || this.busy()) {
      this.host.close();
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const answer: ReactionAnswer = { use: false };
      this.host.data.state.apply(
        (
          await this.host.api.attacks.answerReaction(
            this.host.data.campaignId,
            this.host.data.encounterId,
            id,
            answer,
            this.keys.keyFor({ window: id, answer }),
          )
        ).encounter,
      );
      this.host.close();
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'guardar a flecha'));
    } finally {
      this.busy.set(false);
    }
  }
}
