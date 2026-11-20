import { computed, signal } from '@angular/core';

import {
  AttackOutcome,
  type AttackRoll,
  type HeldRollOffer,
  type ManeuverDieOption,
  type PendingDamage,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { AnswerHeldRollResponse } from '../../../../../gen/meurpg/play/v1/resources_pb';
import { EffectRollKind } from '../../../../../gen/meurpg/play/v1/lasting_effects_pb';
import type {
  Attack,
  PowerAttackOption,
  RollDie,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import {
  awaitsReaction,
  openPendingDamages,
  stageAfterRoll,
  stageOfAnswer,
} from '../../../../core/combat/attack-flow';
import type { CombatClient } from '../../../../core/combat/combat-client';
import { attackResult } from '../../../../core/combat/combat-client-wire';
import type {
  AttackDie,
  AttackEffects,
  DamageDie,
  ModeChoice,
  PairDie,
} from '../../../../core/combat/combat-client-types';
import type { TypedEntry } from '../../../../core/combat/typed-fields';
import { ExtraDiceState } from '../../../../core/effects/extra-dice-state';
import { ActionKey } from '../../../../core/connect/idempotency';
import type { FavorSpend } from '../../../../core/play/favor-spend';
import type { HeldRollController } from '../../../../core/held-rolls/held-roll-controller';
import { type HeldAnswer, asksLucky } from '../../../../core/held-rolls/held-roll-offer';
import { dieFields, genericDieFields, rollDiceOf } from '../../../../core/effects/effects';
import { poolShortMessage, tradesOf } from '../../../../core/combat/power-attack-trade';
import { resourceBlocked } from '../../../../core/resources/resources-errors';
import { ResourceBlockedReason } from '../../../../../gen/meurpg/play/v1/resources_pb';
import { rerollShow } from '../../../../shared/held-roll-prompt/held-roll-shows';
import type { RollAnimator, RollShow } from '../../../../shared/roll-overlay/roll-animator';
import { runBusy } from '../../../../shared/sheet/run-busy';
import type { AttackDamage } from './attack-damage';
import { DamageRolls } from './damage-rolls';
import {
  EFFECT_DIE_FACES,
  attackShow,
  extraDiceProblem,
  inspirationShow,
  rollRefusal,
} from './attack-sheet-view';
import type { AttackState } from './attack-state';
import type { PowerTrade } from './power-trade';

/** The requests of the sheet that roll: the d20, the damage, and the follow-up of a hit that waited for a reaction.
 * Plain class the attack sheet builds. */
export class AttackRolls {
  /** The d4 the effects add (Bênção, Perdição) and the table die ticked (always the last), typed when the roll is from physical dice. */
  private readonly extra = new ExtraDiceState();
  /** The d4 the effects on the attacker add to the roll, typed when the roll is from physical dice. */
  readonly extraFields = this.extra.asked;
  /** What the sheet asks the player to type: the effects' d4, then the table die ticked. */
  readonly typedFields = this.extra.fields;
  readonly extraFaces = this.extra.faces;
  /** The table die ticked (Golpe Certeiro): `null` for none. */
  readonly maneuver = this.extra.maneuver;
  /** The weapon-attack table dice with the uses left; read again when a roll finds none left. */
  readonly maneuverDice = signal<readonly ManeuverDieOption[]>([]);
  /** The same by key, for the offer. */
  readonly maneuverUses = computed(() =>
    Object.fromEntries(
      this.maneuverDice().flatMap((o) =>
        o.die
          ? [
              [
                o.die.key,
                { resource: o.die.resource, left: o.usesLeft, resourceNamePt: o.resourceNamePt },
              ],
            ]
          : [],
      ),
    ),
  );
  /** One key per attack roll (target and die): the same values again are a retry, others a new attack. */
  private readonly attackKeys = new ActionKey();
  /** The damage of the hit: one key for each die, so the same die again is a retry and another die (typed after an app roll was lost) is a new request. */
  private readonly damageRolls: DamageRolls;

  constructor(
    private readonly s: AttackState,
    damage: AttackDamage,
    private readonly question: HeldRollController,
    private readonly host: {
      readonly api: CombatClient;
      readonly animator: Pick<RollAnimator, 'play'>;
      readonly attack: Attack;
      readonly name: string;
      readonly modeChoice: () => ModeChoice | undefined;
      /** The power attack trade ticked on this roll. */
      readonly trade: PowerTrade;
      /** The box of the master's Inspiração: ticked, the roll spends it and sends no mode of its own. */
      readonly fav?: FavorSpend;
    },
  ) {
    this.damageRolls = new DamageRolls(s, damage, {
      api: host.api,
      animator: host.animator,
      fail: (err) => this.fail(err),
    });
    this.maneuverDice.set(s.data.maneuverDice ?? []);
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
    const maneuverKey = this.extra.maneuverKey();
    const trade = this.host.trade.picked();
    if (extra === null) {
      return;
    }
    const { data } = this.s;
    await runBusy(
      this.s.busy,
      this.s.error,
      async () => {
        const spend = this.host.fav?.spend() ?? false;
        const mode = spend ? undefined : this.host.modeChoice();
        const res = await this.host.api.attacks.rollAttack(
          data.campaignId,
          data.encounterId,
          data.attackerId,
          this.host.attack.key,
          id,
          die,
          this.attackKeys.keyFor({
            id,
            die,
            mode,
            extra,
            spend: data.useExtraAction,
            maneuver: maneuverKey,
            power: trade?.key ?? '',
            favor: spend,
          }),
          data.asReaction ?? false,
          data.opportunity?.offerId ?? '',
          mode,
          data.catchWindowId ?? '',
          this.effectsOf(extra, maneuverKey, trade, spend),
        );
        data.state.apply(res.encounter);
        this.host.trade.made.set(trade ?? null);
        // The roll is made (or held for a question): the Inspiração is spent, so the box unticks and who has it is read again.
        this.host.fav?.spent();
        this.settle(res);
        this.play(attackShow(this.host.name, res));
      },
      (err) => this.fail(err),
    );
  }

  /** What rides on the attack besides the d20: the effects' dice, the table die, the power attack trade and the Inspiração. */
  private effectsOf(
    extra: readonly number[],
    maneuverKey: string,
    trade: ReturnType<PowerTrade['picked']>,
    spend: boolean,
  ): AttackEffects {
    const { data } = this.s;
    return {
      extraDieFaces: extra,
      useExtraAction: data.useExtraAction ?? false,
      ...(maneuverKey ? { maneuverKey } : {}),
      ...(trade ? { powerAttackKey: trade.key } : {}),
      legendaryOption: data.legendary?.option ?? '',
      ...(spend ? { spendFavor: true } : {}),
    };
  }

  /** The damage of the hit and the sums typed for it: `DamageRolls`, which the window of a readied attack rolls with too. */
  rollDamage(
    die: DamageDie | { readonly parts: true },
    typedParts?: readonly TypedEntry[],
  ): Promise<void> {
    return this.damageRolls.rollDamage(die, typedParts);
  }

  rollDamageParts(sums: number[]): Promise<void> {
    return this.damageRolls.rollDamageParts(sums);
  }

  rollAwaitedSum(sums: number[], typeKey: string): Promise<void> {
    return this.damageRolls.rollAwaitedSum(sums, typeKey);
  }

  /** What a roll answered: the result and, for a hit, the damage; or, for a held roll, the d20 and its question (Lucky
   * about a 1 first, then the Bardic Inspiration die): no result is told before the answer. */
  settle(res: {
    readonly roll: AttackRoll;
    readonly pending: PendingDamage | undefined;
    readonly held?: HeldRollOffer;
  }): void {
    this.s.roll.set(res.roll);
    this.s.pending.set(res.pending ?? null);
    this.question.ask(res.held);
    if (res.held) {
      this.s.data.onHeld?.(res.held.holdId);
    }
    this.s.typing.set(false);
    this.s.stage.set(stageOfAnswer(res));
  }

  /** The answer to the question of the held roll: the attack the path brought, resolved or still held for the next
   * question on the new roll; the d20 or the die the app rolled tumbles. */
  answered(res: AnswerHeldRollResponse, done: { readonly answer: HeldAnswer }): void {
    if (res.result.case !== 'attack') {
      throw new Error('AnswerHeldRoll answered without its attack');
    }
    const attack = attackResult(res.result.value);
    this.s.data.state.apply(attack.encounter);
    this.settle(attack);
    const { answer } = done;
    this.play(
      answer.kind === 'die'
        ? inspirationShow(attack.roll, answer.use, answer.roll)
        : rerollShow(attack.roll.d20, answer.kind === 'stroke' ? null : answer.roll),
    );
  }

  /** A roll a reaction window held is now a question of the combat: the sheet that waited for the window asks it. */
  askAfterWindow(held: HeldRollOffer | undefined): void {
    if (!held) {
      return;
    }
    this.question.ask(held);
    this.s.data.onHeld?.(held.holdId);
    this.s.stage.set(asksLucky(held) ? 'lucky' : 'inspire');
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
      const now = openPendingDamages(res.pendingDamages).find((p) => p.id === waiting.id);
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

  /** "Usar <dado>": the die ticked joins the roll; none ticked leaves it as it is. */
  useDie(key: string): void {
    this.s.poolNotice.set('');
    this.extra.maneuver.set(this.maneuverDice().find((o) => o.die?.key === key)?.die ?? null);
  }

  private fail(err: unknown): void {
    const refused = this.host.fav?.refused(err);
    if (refused !== null && refused !== undefined) {
      this.s.error.set(refused);
      return;
    }
    this.s.poolNotice.set('');
    const die = this.extra.maneuver();
    const trade = this.host.trade.picked();
    const noUses = resourceBlocked(err)?.reason === ResourceBlockedReason.NO_USES_LEFT;
    if (trade && noUses) {
      void this.explainNoUses(die, trade);
      return;
    }
    if (die && noUses) {
      // The sheet was older than the table: nothing was spent; the offer shows the uses as they are now.
      const pool = this.maneuverDice().find((o) => o.die?.key === die.key)?.resourceNamePt;
      this.extra.maneuver.set(null);
      this.s.error.set(`${die.namePt}: não há usos de ${pool || die.resource}. Nada foi gasto.`);
      void this.refreshUses();
      return;
    }
    const refusal = rollRefusal(err);
    if (refusal.more > 0) {
      this.extraFields.set(genericDieFields(refusal.more, EFFECT_DIE_FACES));
    }
    this.s.error.set(refusal.message);
  }

  /** The roll was refused for want of uses with a trade ticked: read the uses again and say what is short. Two things that
   * spend one pool, with a use left for one, ask to take one check off and keep both ticked; a trade that has none is
   * taken off and listed as the server lists it. Nothing was spent in either case. */
  private async explainNoUses(die: RollDie | null, trade: PowerAttackOption): Promise<void> {
    await this.refreshUses();
    const listed = this.host.trade.options().find((o) => o.key === trade.key);
    const dieLeft = this.maneuverDice().find((o) => o.die?.key === die?.key)?.usesLeft ?? 0;
    if (die && !listed?.disabled && dieLeft > 0) {
      const pool = this.maneuverDice().find((o) => o.die?.key === die.key)?.resourceNamePt ?? '';
      // The trade's own read of the pool is the one the roll was refused on; the die's is the fallback.
      this.s.poolNotice.set(
        poolShortMessage(
          listed?.resourceNamePt || pool,
          listed?.resourceNamePt ? listed.usesLeft : dieLeft,
        ),
      );
      return;
    }
    this.host.trade.pickedKey.set('');
    this.s.error.set(`${trade.name}: não há usos. Nada foi gasto.`);
  }

  /** Reads the attacker's table dice and the trades of the weapon again, so the offers say how many uses are left. */
  private async refreshUses(): Promise<void> {
    const { data } = this.s;
    try {
      const res = await this.host.api.moves.turnOptions(
        data.campaignId,
        data.encounterId,
        data.attackerId,
      );
      this.maneuverDice.set(res.maneuverDice);
      const option = res.options?.attacks.find((a) => a.attack?.key === this.host.attack.key);
      if (option) {
        this.host.trade.options.set(tradesOf(option));
      }
    } catch {
      // The offer keeps what it shows; the roll without the die still works.
    }
  }

  /** The faces of the d4 typed for a roll from physical dice; none for the app's dice, `null` while one is missing (the sheet says so). */
  private typedExtra(die: AttackDie | PairDie): readonly number[] | null {
    const fields = this.typedFields();
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
