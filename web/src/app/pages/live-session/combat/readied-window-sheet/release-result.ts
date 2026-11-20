import { create } from '@bufbuild/protobuf';
import { type Signal, computed } from '@angular/core';

import type { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type {
  AttackRoll,
  CombatantState,
  PendingDamage,
  SpellCast,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { AttackSchema } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { awaitsReaction, stageAfterRoll } from '../../../../core/combat/attack-flow';
import { castRows, effectSentence } from '../../../../core/combat/cast-flow';
import type { CombatClient } from '../../../../core/combat/combat-client';
import type { CombatState } from '../../../../core/combat/combat-state';
import { isPlayer } from '../../../../core/combat/combat-view';
import { initialPicks } from '../../../../core/combat/damage-parts';
import { poolRollText } from '../../../../core/combat/hp-effects';
import { d20Formula } from '../../../../core/combat/roll-mode';
import type { RollAnimator } from '../../../../shared/roll-overlay/roll-animator';
import { AttackDamage } from '../attack-sheet/attack-damage';
import { AttackState } from '../attack-sheet/attack-state';
import { DamageRolls } from '../attack-sheet/damage-rolls';
import { outcomeOf, rollRefusal } from '../attack-sheet/attack-sheet-view';

/** What the release of a readied action rolled, as `AnswerReaction` answers it (#938). */
export interface ReleaseRolled {
  readonly attackRoll: AttackRoll | undefined;
  readonly pendingDamage: PendingDamage | undefined;
  readonly cast: SpellCast | undefined;
}

/** What the result of a release is made from. */
export interface ReleaseResultInput {
  readonly campaignId: string;
  readonly encounterId: string;
  readonly round: number;
  /** The readier. */
  readonly attackerId: string;
  /** The key and the name of the held attack or spell. */
  readonly actionKey: string;
  readonly name: string;
  readonly targetLabel: string;
  readonly rolled: ReleaseRolled;
  readonly diceMode: DiceMode;
  readonly preference: DicePreference;
  readonly state: CombatState;
  readonly canApp: boolean;
  readonly api: Pick<CombatClient, 'attacks'>;
  readonly animator: Pick<RollAnimator, 'play'>;
}

/**
 * What the window of a readied action shows once its answer rolled something (#938, boards 938-04 and 938-06): the done steps of the
 * attack sheet ("Alvo", "Rolar" with its pill) and, for a hit, the damage it asks right there with the attack sheet's own damage step,
 * rolled with `RollDamage` on the pending damage the hit opened; or the cast of a pool spell with the cast sheet's own result. The
 * attack sheet's state and damage parts are reused as they are: this only builds them from the answer, which is what the sheet that
 * made the attack would have had in hand.
 */
export class ReleaseResult {
  readonly state: AttackState;
  readonly damage: AttackDamage;
  readonly rolls: DamageRolls;
  readonly cast: SpellCast | undefined;
  readonly targetLabel: Signal<string>;
  readonly outcome = computed(() => outcomeOf(this.state.roll(), false));
  readonly d20Formula = computed(() => {
    const d20 = this.state.roll()?.d20;
    return d20 ? d20Formula(d20, true) : '';
  });
  /** The hit's damage waits for the target's reaction (Escudo): it is not a miss. */
  readonly waiting = computed(() => awaitsReaction(this.state.pending()));
  private readonly labels = computed(() => {
    const out = new Map<string, { label: string; state: CombatantState }>();
    for (const c of this.input.state.encounter()?.combatants ?? []) {
      out.set(c.id, { label: c.label, state: c.state });
    }
    return out;
  });
  readonly castRows = computed(() =>
    this.cast ? castRows(this.cast, new Map(), this.labels()) : [],
  );
  readonly castSentence = computed(() => {
    const e = this.input.state.encounter();
    return this.cast
      ? effectSentence(this.cast, this.labels(), (id) => {
          const c = e?.combatants.find((x) => x.id === id);
          return !!c && !isPlayer(c);
        })
      : '';
  });
  readonly castPool = computed(() => (this.cast?.poolRoll ? poolRollText(this.cast.poolRoll) : ''));

  constructor(private readonly input: ReleaseResultInput) {
    const { rolled } = input;
    this.cast = rolled.cast;
    this.targetLabel = computed(() => input.targetLabel);
    this.state = new AttackState({
      campaignId: input.campaignId,
      encounterId: input.encounterId,
      attackerId: input.attackerId,
      round: input.round,
      attack: create(AttackSchema, { key: input.actionKey, namePt: input.name }),
      targets: [],
      diceMode: input.diceMode,
      preference: input.preference,
      state: input.state,
    });
    this.state.roll.set(rolled.attackRoll ?? null);
    this.state.pending.set(rolled.pendingDamage ?? null);
    this.state.stage.set(
      rolled.attackRoll ? stageAfterRoll(rolled.attackRoll.outcome, rolled.pendingDamage) : 'done',
    );
    // The extras that come marked are the starting choice of the damage.
    this.state.picks.set(initialPicks(rolled.pendingDamage?.parts ?? []));
    this.damage = new AttackDamage(
      this.state,
      input.name,
      input.canApp,
      computed(() => this.outcome()?.crit),
      this.targetLabel,
    );
    this.rolls = new DamageRolls(this.state, this.damage, {
      api: input.api,
      animator: input.animator,
      fail: (err) => this.state.error.set(rollRefusal(err).message),
    });
  }

  /** The hit asks its damage right here. */
  readonly asksDamage = computed(() => this.state.stage() === 'damage' && !!this.damage.dice());
  /** Nothing more to roll: a miss, a damage rolled, a hit waiting for a reaction or a cast. */
  readonly finished = computed(() => !this.asksDamage());
}
