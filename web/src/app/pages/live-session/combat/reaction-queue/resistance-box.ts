import { ChangeDetectionStrategy, Component, input, model } from '@angular/core';

import type { ResistanceBox as ResistanceBoxModel } from '../../../../core/combat/legendary-resistance-card';

/**
 * "Se falhar, usar Resistência Lendária" (#557, boards 04, 09): the box in the save the master rolls for an NPC that has
 * the trait and a use left. Unticked by default; the parent draws it only when `resistanceBox` gives one. The row is the
 * label of a real checkbox.
 */
@Component({
  selector: 'app-resistance-box',
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './resistance-box.html',
  styleUrl: './resistance-box.scss',
})
export class ResistanceBox {
  readonly box = input.required<ResistanceBoxModel>();
  readonly checked = model(false);
  readonly disabled = input(false);
}
