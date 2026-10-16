import { signal } from '@angular/core';

import {
  AttackOutcome,
  type AttackRoll,
  type InspirationOffer,
  type LuckyOffer,
  type PendingDamage,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { EffectRollKind } from '../../../../../gen/meurpg/play/v1/lasting_effects_pb';
import type { Attack } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { awaitsReaction, stageAfterRoll, stageOfAnswer } from '../../../../core/combat/attack-flow';
import type { CombatClient } from '../../../../core/combat/combat-client';
import type {
  AttackDie,
  DamageDie,
  ModeChoice,
  PairDie,
} from '../../../../core/combat/combat-client-types';
import { ActionKey } from '../../../../core/connect/idempotency';
import {
  type ExtraDieField,
  dieFields,
  genericDieFields,
  rollDiceOf,
} from '../../../../core/effects/effects';
import type { RollAnimator, RollShow } from '../../../../shared/roll-overlay/roll-animator';
import type { AttackDamage } from './attack-damage';
import type { AttackQuestions } from './attack-questions';
import {
  EFFECT_DIE_FACES,
  attackShow,
  damageShow,
  extraDiceProblem,
  rollRefusal,
} from './attack-sheet-view';
import type { AttackState } from './attack-state';

/** The requests of the sheet that roll: the d20, the damage, and the follow-up of a hit that waited for a reaction.
 * Plain class the attack sheet builds. */
export class AttackRolls {
  /** The d4 the effects on the attacker add to the roll (Bênção, Perdição), typed when the roll is from physical dice. */
  readonly extraFields = signal<readonly ExtraDieField[]>([]);
  readonly extraFaces = signal<readonly (number | null)[]>([]);
  /** One key per attack roll (target and die): the same values again are a retry, others a new attack. */
  private readonly attackKeys = new ActionKey();
  /** One key per damage roll: the same die again is a retry, another die (typed after an app roll was lost) is a new request. */
  private readonly damageKeys = new ActionKey();

  constructor(
    private readonly s: AttackState,
    private readonly damage: AttackDamage,
    private readonly questions: AttackQuestions,
    private readonly host: {
      readonly api: CombatClient;
      readonly animator: Pick<RollAnimator, 'play'>;
      readonly attack: Attack;
      readonly name: string;
      readonly modeChoice: () => ModeChoice | undefined;
    },
  ) {
    this.extraFields.set(
      dieFields(
        rollDiceOf(
          s.data.state.encounter()?.combatants.find((c) => c.id === s.data.attackerId)?.effects,
          EffectRollKind.ATTACK,
        ),
      ),
    );
  }

  async rollAttack(die: AttackDie | PairDie): Promise<void> {
    const id = this.s.targetId();
    if (id === null || this.s.busy()) {
      return;
    }
    const extra = this.typedExtra(die);
    if (extra === null) {
      return;
    }
    const { data } = this.s;
    await this.s.run(
      async () => {
        const mode = this.host.modeChoice();
        const res = await this.host.api.attacks.rollAttack(
          data.campaignId,
          data.encounterId,
          data.attackerId,
          this.host.attack.key,
          id,
          die,
          this.attackKeys.keyFor({ id, die, mode, extra, spend: data.useExtraAction }),
          data.asReaction ?? false,
          data.opportunity?.offerId ?? '',
          mode,
          data.catchWindowId ?? '',
          { extraDieFaces: extra, useExtraAction: data.useExtraAction ?? false },
        );
        data.state.apply(res.encounter);
        this.settle(res);
        this.play(attackShow(this.host.name, res));
      },
      (err) => this.fail(err),
    );
  }

  async rollDamage(
    die: DamageDie | { readonly parts: true },
    typedParts?: { partKey: string; sum: number }[],
  ): Promise<void> {
    const p = this.s.pending();
    if (!p || this.s.busy()) {
      return;
    }
    const { data } = this.s;
    await this.s.run(
      async () => {
        // With parts the answer is the whole choice, the extras marked (even none) and the sums typed.
        const choice =
          this.damage.parts().length > 0
            ? { extras: this.s.picks(), ...(typedParts ? { typedParts } : {}) }
            : undefined;
        const res = await this.host.api.attacks.rollDamage(
          data.campaignId,
          data.encounterId,
          p.id,
          die,
          this.damageKeys.keyFor({ die, choice }),
          choice,
        );
        data.state.apply(res.encounter);
        this.s.damage.set(res.pending);
        this.play(damageShow(res.pending));
        this.s.typing.set(false);
        this.s.stage.set('done');
      },
      (err) => this.fail(err),
    );
  }

  /** The sums typed for each group of dice, in the order of the fields. */
  rollDamageParts(sums: number[]): Promise<void> {
    return this.rollDamage(
      { parts: true },
      this.damage.fields().map((f, i) => ({ partKey: f.key, sum: sums[i] })),
    );
  }

  /** What a roll answered: the result and, for a hit, the damage; or, for a held roll, the d20 and its question (Lucky
   * about a 1 first, then the Bardic Inspiration die): no result is told before the answer. */
  settle(res: {
    readonly roll: AttackRoll;
    readonly pending: PendingDamage | undefined;
    readonly offer?: InspirationOffer;
    readonly lucky?: LuckyOffer;
  }): void {
    this.s.roll.set(res.roll);
    this.s.pending.set(res.pending ?? null);
    this.questions.offer.set(res.offer ?? null);
    this.questions.luckyOffer.set(res.lucky ?? null);
    const held = res.lucky ?? res.offer;
    if (held) {
      this.s.data.onHeld?.(held.holdId);
    }
    this.s.typing.set(false);
    this.s.stage.set(stageOfAnswer(res));
  }

  /** Reads the attacker's pending damages again: the hit that waited for a reaction is now a damage to roll, or gone
   * (the reaction stopped it: a miss). The player learns only that: never the armor class or the NPC's numbers (RN-10). */
  async followReaction(): Promise<void> {
    const s = this.s;
    const waiting = s.pending();
    if (!waiting) {
      return;
    }
    try {
      const res = await this.host.api.moves.turnOptions(
        s.data.campaignId,
        s.data.encounterId,
        s.data.attackerId,
      );
      const now = res.pendingDamages.find((p) => p.id === waiting.id);
      if (s.pending()?.id !== waiting.id || awaitsReaction(now)) {
        return;
      }
      if (now) {
        s.pending.set(now);
        s.stage.set(stageAfterRoll(s.roll()?.outcome ?? AttackOutcome.HIT, now));
        return;
      }
      const r = s.roll();
      if (r) {
        // A protobuf-es message is a plain object: the same roll, now a miss.
        s.roll.set({ ...r, outcome: AttackOutcome.MISS });
      }
      s.pending.set(null);
      s.stage.set('done');
    } catch {
      // The sheet keeps what it shows; the next change of the combat asks again.
    }
  }

  private play(show: RollShow | null): void {
    if (show) {
      this.host.animator.play(show);
    }
  }

  private fail(err: unknown): void {
    const refusal = rollRefusal(err);
    if (refusal.more > 0) {
      this.extraFields.set(genericDieFields(refusal.more, EFFECT_DIE_FACES));
    }
    this.s.error.set(refusal.message);
  }

  /** The faces of the d4 typed for a roll from physical dice; none for the app's dice, `null` while one is missing (the sheet says so). */
  private typedExtra(die: AttackDie | PairDie): readonly number[] | null {
    const fields = this.extraFields();
    if ('inApp' in die || fields.length === 0) {
      return [];
    }
    const problem = extraDiceProblem(fields, this.extraFaces());
    if (problem) {
      this.s.error.set(problem);
      return null;
    }
    return this.extraFaces() as readonly number[];
  }
}
