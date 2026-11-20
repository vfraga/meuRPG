import { ChangeDetectionStrategy, Component, input } from '@angular/core';

import type { DamageDie } from '../../../../core/combat/combat-client-types';
import type { TypedEntry } from '../../../../core/combat/typed-fields';
import { MultiRoll } from '../../../../shared/multi-roll/multi-roll';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';
import { DamageParts } from '../damage-parts/damage-parts';
import { AttackDamage } from './attack-damage';
import { GroupSumParts } from './group-sum-parts';
import type { AttackState } from './attack-state';

/** What the damage step asks of whoever rolls: the app's dice, a typed sum, the sums of the parts, the one sum asked again. */
export interface DamageActions {
  rollDamage(
    die: DamageDie | { readonly parts: true },
    typedParts?: readonly TypedEntry[],
  ): Promise<void>;
  /** The sums typed for each damage type (or part that needs its own key), in the order of the fields. */
  rollDamageParts(sums: number[]): Promise<void>;
  /** The sum asked again after the master took an extra out of a group: that damage type alone. */
  rollAwaitedSum(sums: number[], typeKey: string): Promise<void>;
}

/**
 * The damage of a hit (E6-08): "Acertou: role o dano.", the dice of the damage with the attack sheet's own words ("Role 1d8 para o
 * dano", "Digite a soma dos dados, de 1 a 8. O app soma o modificador.", "Dano total") or, with parts, the extras and one field for
 * each damage type; the critical's groups and the sum asked again after the master took a part out. The attack sheet draws it at its
 * `damage` stage, and the window of a readied attack (#938) draws it for the hit its release rolled. The caller owns what is rolled
 * (`actions`), the request and where the answer goes.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-attack-damage-step',
  imports: [DamageParts, GroupSumParts, MultiRoll, RollPicker],
  templateUrl: './attack-damage-step.html',
  styleUrl: './attack-damage-step.scss',
})
export class AttackDamageStep {
  readonly state = input.required<AttackState>();
  readonly damage = input.required<AttackDamage>();
  readonly actions = input.required<DamageActions>();
  readonly canApp = input.required<boolean>();
  readonly canType = input.required<boolean>();
  readonly preferApp = input.required<boolean>();

  /** The two-way bindings of the pickers write the state's own signals. */
  protected get typing() {
    return this.state().typing;
  }
  protected get picks() {
    return this.state().picks;
  }
}
