import { ChangeDetectionStrategy, Component, computed, effect, input, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { Combatant } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { ExtraActionOption } from '../../../../../gen/meurpg/play/v1/lasting_effects_pb';
import type { GearVm } from '../../../../core/characters/gear';
import {
  GEAR_RULE,
  type GearFlow,
  gearBlocked,
  gearCost,
  shieldBackAc,
  twoHandedWarnings,
} from '../../../../core/combat/gear-flow';
import { Fold } from '../../../../shared/fold/fold';
import { GearList } from '../../../../shared/gear-list/gear-list';

/**
 * "Sacar ou guardar": the player's gear under the attacks, folded. The header carries the armor class; the rule line
 * says what each change costs; the list is the sheet's (`GearList`, in its combat form: the cost is in the button, the
 * armor is not listed). The flow (`GearFlow`) asks the server and reads the gear again; the fold is not drawn while the
 * gear is not read, unless the read failed ("Tentar de novo").
 */
@Component({
  selector: 'app-gear-fold',
  imports: [Fold, GearList, MatButtonModule, MatIconModule],
  templateUrl: './gear-fold.html',
  styleUrl: './gear-fold.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class GearFold {
  readonly flow = input.required<GearFlow>();
  readonly own = input.required<Combatant>();
  /** The extra action an effect gives (Velocidade): it may be "Usar um objeto", and then it pays for a change. */
  readonly extra = input<ExtraActionOption | null>(null);
  /** Why the turn is held for something else; the rows have no buttons meanwhile. */
  readonly locked = input('');

  protected readonly rule = GEAR_RULE;
  /** The fold's open state, told to the flow: a closed fold reads nothing. */
  protected readonly opened = signal(true);

  constructor() {
    effect(() => this.flow().setOpen(this.opened()));
  }
  private readonly turn = computed(() => {
    const extra = this.extra();
    return {
      objectInteractionUsed: this.own().objectInteractionUsed,
      actionUsed: this.own().actionUsed,
      extraObject: !!extra?.available && extra.allowedActions.includes('use-an-object'),
    };
  });
  protected readonly costOf = computed(() => {
    const turn = this.turn();
    return (piece: GearVm) => gearCost(piece, turn);
  });
  protected readonly blockedOf = computed(() => {
    const turn = this.turn();
    return (piece: GearVm) => gearBlocked(piece, turn);
  });
  /** The armor is the sheet's: a combat does not dress it, and the rule line says so. */
  protected readonly pieces = computed(() =>
    this.flow()
      .gear()
      .filter((g) => g.kind !== 'armor'),
  );
  private readonly twoHanded = computed(() =>
    twoHandedWarnings(this.pieces(), this.flow().armorClass()),
  );
  protected readonly warnings = computed(() => this.twoHanded().map((w) => w.text));
  protected readonly warnedKeys = computed(() => this.twoHanded().map((w) => w.key));
  /** The header's armor class: "CA 18", or "CA 18 → 16" while the shield waits behind a two-handed weapon. */
  protected readonly acNote = computed(() => {
    const ac = this.flow().armorClass();
    if (ac === null) {
      return '';
    }
    const back = shieldBackAc(this.pieces(), this.flow().shieldIdle());
    return back === null ? `CA ${ac}` : `CA ${back} → ${ac}`;
  });
  protected readonly acLabel = computed(() => {
    const ac = this.flow().armorClass();
    const back = shieldBackAc(this.pieces(), this.flow().shieldIdle());
    if (ac === null) {
      return '';
    }
    return back === null
      ? `Classe de armadura ${ac}`
      : `Classe de armadura ${ac}, era ${back} com o escudo`;
  });
}
