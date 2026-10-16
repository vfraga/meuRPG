import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

import { CountPips } from '../../../shared/count-pips/count-pips';
import type { VitalsVm } from '../../../core/play/live-session.types';
import { resourceBoxes, slotCounterRows, slotsFootnote } from './counters';

/**
 * "Recursos" and "Espaços de magia" on the character sheet (PM-07b 7): a box of 92 px for each resource with its name,
 * "2 de 3", the pips (up to 6 uses; above that only the number) and when it comes back; then a row for each spell
 * level with its pips and "3 de 4" ("criado" beside the count of a slot Flexible Casting made), the pact slots as
 * their own row, and when they come back. Read-only: nothing here takes focus (the master adjusts from the
 * character's card on the session page). The numbers are the live ones, which the sheet page keeps current while the
 * campaign has an open session. A section with nothing to show is not drawn.
 */
@Component({
  selector: 'app-resource-counters',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [CountPips],
  templateUrl: './resource-counters.html',
  styleUrl: './resource-counters.scss',
})
export class ResourceCounters {
  readonly vitals = input.required<VitalsVm>();

  protected readonly boxes = computed(() => resourceBoxes(this.vitals().resources));
  protected readonly slots = computed(() => slotCounterRows(this.vitals()));
  protected readonly footnote = computed(() => slotsFootnote(this.vitals()));
}
