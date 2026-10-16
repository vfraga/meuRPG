import { Component, inject } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import type { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { DicePreferencePanel } from '../../../shared/dice-preference-panel/dice-preference-panel';

export interface DiceDialogData {
  readonly campaignId: string;
  readonly campaignName: string;
  readonly mode: DiceMode;
  readonly preference: DicePreference;
}

/** "Mudar" on the initiative screen: the player's "Como você rola os
 * dados" (E6-18, RN-18) in a dialog, so they need not leave the session. */
@Component({
  selector: 'app-dice-dialog',
  imports: [DicePreferencePanel, MatButtonModule],
  template: `
    <div class="dlg" role="group" aria-label="Como você rola os dados">
      <app-dice-preference-panel
        [campaignId]="data.campaignId"
        [campaignName]="data.campaignName"
        [mode]="data.mode"
        [savedPreference]="data.preference"
      />
      <button mat-stroked-button type="button" class="close" (click)="ref.close(true)">Fechar</button>
    </div>
  `,
  styles: `
    .dlg {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-3);
      padding: var(--mr-space-4);
      background: var(--mr-surface);
    }

    .close {
      --mat-button-outlined-label-text-color: var(--mr-ink);
      align-self: flex-end;
    }
  `,
})
export class DiceDialog {
  protected readonly data = inject<DiceDialogData>(MAT_DIALOG_DATA);
  protected readonly ref = inject<MatDialogRef<DiceDialog, boolean>>(MatDialogRef);
}
