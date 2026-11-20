import type { CombatClient } from '../../../../core/combat/combat-client';
import type { DamageDie } from '../../../../core/combat/combat-client-types';
import { type TypedEntry, typedEntries } from '../../../../core/combat/typed-fields';
import { ActionKey } from '../../../../core/connect/idempotency';
import type { RollAnimator } from '../../../../shared/roll-overlay/roll-animator';
import { runBusy } from '../../../../shared/sheet/run-busy';
import type { AttackDamage } from './attack-damage';
import type { DamageActions } from './attack-damage-step';
import { damageShow } from './attack-sheet-view';
import type { AttackState } from './attack-state';

/**
 * The request of the damage of a hit (`RollDamage`) and what its answer does to the state the damage step reads: the combat goes to the
 * page's copy, the damage rolled is kept and the stage is done. One key for each way of rolling it, so a tap repeated after a lost answer
 * never rolls twice and a typed sum after a lost app roll is a new request. The attack sheet and the window of a readied attack (#938)
 * both roll their damage here; each says what a refusal reads like (`fail`).
 */
export class DamageRolls implements DamageActions {
  private readonly keys = new ActionKey();

  constructor(
    private readonly s: AttackState,
    private readonly damage: AttackDamage,
    private readonly host: {
      readonly api: Pick<CombatClient, 'attacks'>;
      readonly animator: Pick<RollAnimator, 'play'>;
      /** A refusal of the roll: the message the screen shows. */
      readonly fail: (err: unknown) => void;
    },
  ) {}

  async rollDamage(
    die: DamageDie | { readonly parts: true },
    typedParts?: readonly TypedEntry[],
  ): Promise<void> {
    const p = this.s.pending();
    if (!p || this.s.busy()) {
      return;
    }
    const { data } = this.s;
    await runBusy(
      this.s.busy,
      this.s.error,
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
          this.keys.keyFor({ die, choice }),
          choice,
        );
        data.state.apply(res.encounter);
        this.s.damage.set(res.pending);
        const show = damageShow(res.pending);
        if (show) {
          this.host.animator.play(show);
        }
        this.s.typing.set(false);
        this.s.stage.set('done');
      },
      (err) => this.host.fail(err),
    );
  }

  /** The sums typed for each damage type (or part that needs its own key), in the order of the fields. */
  rollDamageParts(sums: number[]): Promise<void> {
    return this.rollDamage({ parts: true }, typedEntries(this.damage.typed(), sums));
  }

  /** The sum asked again after the master took an extra out of a group: that damage type alone. */
  rollAwaitedSum(sums: number[], typeKey: string): Promise<void> {
    return this.rollDamage({ parts: true }, [{ damageType: typeKey, sum: sums[0] }]);
  }
}
