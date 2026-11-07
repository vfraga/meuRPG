import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { LimitedAction } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { RechargeRoll } from '../../../../core/combat/combat-derived';
import { rechargeFaces } from '../../../../core/combat/creature-action';

/**
 * "Recarga no começo do turno: rolou 5 no d6. Sopro de fogo recarregou." The d6 the server rolled when the creature's turn began
 * for each Recharge action it had spent (the combat log's line, which only the master and the creature's controller read). It sits
 * under the action chips, above the attacks, so the master reads why an action is spent before the rest of the card. Green when
 * every action rolled came back; neutral otherwise, with each action's own result.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-recharge-notice',
  imports: [MatIconModule],
  template: `
    @if (shown(); as r) {
      <p
        class="mr-notice recharge"
        [class.mr-notice--success]="r.allBack"
        [class.mr-notice--neutral]="!r.allBack"
        role="status"
        data-testid="recharge-rolled"
      >
        <mat-icon aria-hidden="true">casino</mat-icon>
        <span
          ><b>Recarga no começo do turno:</b> {{ r.text }}</span
        >
      </p>
    }
  `,
  styles: `
    :host {
      display: block;
    }
    .recharge {
      margin: 0;
      padding: 8px 12px;
      font-size: var(--mr-font-14);
    }
  `,
})
export class RechargeNotice {
  readonly rolls = input<readonly RechargeRoll[]>([]);
  readonly limited = input<readonly LimitedAction[]>([]);

  protected readonly shown = computed(() => {
    const rolls = this.rolls();
    if (rolls.length === 0) {
      return null;
    }
    const text = rolls
      .map((r) => {
        const copy = this.limited().find((l) => (l.namePt || l.name) === r.action);
        const faces = copy ? rechargeFaces(copy.usage) : '';
        const need = r.recharged || !faces ? '' : ` (precisa de ${faces})`;
        return `rolou ${r.face} no d6. ${r.action} ${r.recharged ? 'recarregou' : 'não recarregou'}${need}.`;
      })
      .join(' ');
    return { allBack: rolls.every((r) => r.recharged), text };
  });
}
