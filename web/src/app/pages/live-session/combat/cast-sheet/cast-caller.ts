import type { Encounter, SpellCast } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { dartTargets } from '../../../../core/combat/cast-flow';
import { type CombatClient } from '../../../../core/combat/combat-client';
import {
  type AttackDie,
  type PairDie,
  type PoolDie,
} from '../../../../core/combat/combat-client-types';
import { newKey } from '../../../../core/connect/idempotency';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import { metamagicChoices } from '../../../../core/resources/metamagic';
import type { RollAnimator } from '../../../../shared/roll-overlay/roll-animator';
import type { CastArea } from './cast-area';
import type { CastChoices } from './cast-choices';
import type { CastDamage } from './cast-damage';
import type { CastQuestions } from './cast-questions';
import { castShow, castTargetsOf } from './cast-sheet-view';
import type { CastState } from './cast-state';

/** The request that casts the spell, and what the sheet does with its answer. Plain class the cast sheet builds. */
export class CastCaller {
  /** Made again when the choice changes: a new cast, not a retry. */
  private castKey = newKey();

  constructor(
    private readonly s: CastState,
    private readonly choices: CastChoices,
    private readonly area: CastArea,
    private readonly questions: CastQuestions,
    private readonly damage: CastDamage,
    private readonly host: {
      readonly api: CombatClient;
      readonly animator: Pick<RollAnimator, 'play'>;
    },
  ) {}

  /** A new slot, target or dart is a new cast, not a retry. */
  newCast(): void {
    this.castKey = newKey();
  }

  /** "Conjurar X" (a spell with no attack roll). A list that may hit nobody, with nobody ticked, asks in place first. */
  castNow(): Promise<void> {
    const r = this.choices.rule();
    if (
      !this.area.flow &&
      r.kind === 'multi' &&
      r.min === 0 &&
      this.s.chosen().length === 0 &&
      !this.s.listAsking()
    ) {
      this.s.listAsking.set(true);
      return Promise.resolve();
    }
    return this.cast(null);
  }

  /** "Conjurar mesmo assim" over an area with nobody the caster sees: a spell that rolls a pool goes on to its roll. */
  castAnyway(): Promise<void> {
    if (this.area.flow) {
      this.area.flow.nobodyAccepted.set(true);
      return this.choices.usesPicker() ? Promise.resolve() : this.cast(null);
    }
    return this.cast(null);
  }

  async cast(die: AttackDie | PoolDie | PairDie | null): Promise<void> {
    if (this.s.busy() || !this.choices.ready()) {
      return;
    }
    const { data } = this.s;
    await this.s.run(
      async () => {
        const slot = this.area.slotRef();
        const flow = this.area.flow;
        const area = flow ? flow.choice() : null;
        const roll = die ?? (this.choices.kind() === 'pool' ? { inApp: true as const } : null);
        const sculpted = this.choices.sculptedIds();
        // A placed area's key follows its request: the same slot, place and roll again is a retry of it.
        const key = flow
          ? this.area.key.keyFor([slot, area, roll, this.choices.damageType(), sculpted])
          : this.castKey;
        const picked = this.s.picked();
        const res = await this.host.api.casts.castSpell(
          data.campaignId,
          data.encounterId,
          data.casterId,
          data.spellKey,
          slot,
          castTargetsOf(
            !!flow,
            this.choices.rule(),
            () => dartTargets(this.s.dealt()),
            this.s.chosen(),
          ),
          // A pool is always rolled by someone: the server does it when the app rolls every die.
          roll,
          key,
          undefined,
          this.choices.damageType(),
          this.choices.kind() === 'attack' && picked !== null
            ? { mode: picked, reason: this.s.reason().trim() }
            : undefined,
          metamagicChoices(this.s.chosenMeta(), this.s.picks()),
          {
            ...(flow ? { area } : {}),
            ...(this.choices.asksAbility ? { abilityKey: this.s.ability() } : {}),
            ...(sculpted.length ? { sculptedIds: sculpted } : {}),
          },
        );
        this.show(res.encounter, res.cast);
      },
      (err) => {
        if (!this.questions.asks(err)) {
          this.s.error.set(combatErrorMessage(err, 'conjurar a magia'));
        }
      },
    );
  }

  /** The cast as the result step shows it. */
  show(encounter: Encounter, cast: SpellCast): void {
    this.s.data.state.apply(encounter);
    this.s.cast.set(cast);
    const shown = castShow(this.s.data.name, cast);
    if (shown) {
      this.host.animator.play(shown);
    }
    this.damage.set(cast.pendingDamages);
    this.s.typing.set(false);
  }
}
