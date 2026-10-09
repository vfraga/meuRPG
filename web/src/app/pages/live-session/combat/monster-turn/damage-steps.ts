import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

import type { CreatureActionResult } from '../../../../../gen/meurpg/play/v1/creatures_pb';
import { rollFormula } from '../../../../core/combat/combat-dice';
import { partRows } from '../../../../core/combat/creature-turn';

/**
 * The damage of a monster's attack, one block for each part the server rolled (W7-M): the roll,
 * then a row for each step of the target's resistance, vulnerability or immunity to the type, and
 * what lands. A player's character shows its steps to the master and to its own player; an NPC's
 * are the master's alone: the server leaves `steps` empty for anyone else (RN-20), so a part
 * with none shows only what it rolled.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-damage-steps',
  template: `
    @for (p of parts(); track p.id) {
      <section class="part" [attr.aria-label]="'Dano ' + p.type">
        <p class="part__roll">
          <span>{{ p.type }}</span>
          <span class="part__formula">{{ p.formula }}</span>
        </p>
        @for (row of p.rows; track $index) {
          <p class="part__row">
            <span>{{ row.label }}</span>
            <span class="part__value">{{ row.value }}</span>
          </p>
        }
        <p class="part__total"><span>Cai</span><span class="part__value">{{ p.amount }}</span></p>
      </section>
    }
  `,
  styles: `
    :host {
      display: block;
    }

    .part {
      padding: 8px 0;
      border-top: 1px solid var(--mr-line);
    }

    .part p {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      margin: 0;
      font-size: 15px;
    }

    .part__roll {
      font-weight: 700;
    }

    .part__formula,
    .part__row {
      color: var(--mr-ink-muted);
    }

    .part__total,
    .part__value {
      font-family: var(--mr-font-display);
      font-weight: 700;
    }
  `,
})
export class DamageSteps {
  readonly result = input.required<CreatureActionResult>();

  protected readonly parts = computed(() =>
    this.result().damageParts.map((p) => ({
      id: p.pendingDamageId,
      type: p.damageTypePt,
      formula: p.rolled ? rollFormula(p.rolled) : '',
      rows: partRows(p).slice(1),
      amount: p.amount,
    })),
  );
}
