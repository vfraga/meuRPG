import { Component, computed, input, output } from '@angular/core';

import { abilityLabel, formatModifier } from '../../../core/characters/character-labels';
import type { VitalsVm } from '../../../core/play/live-session.types';
import { SpellHelp } from '../../../shared/spell-details/spell-help';
import { FullSheetVm } from '../character-sheet.types';
import { CombatStats } from '../combat-stats/combat-stats';
import {
  grantedSpellLevel,
  grantedSpellNumbers,
  grantedSpellTags,
  pactSlotRow,
  spellGroups,
  spellLimitsText,
  spellSlotRows,
  spellStateSummary,
} from '../sheet-format';

/**
 * The paper sheet's middle column: the combat numbers, "Ataques" (a real
 * table: the weapons carried, then the damage cantrips, in the server's
 * order), "Magias" (per class: ability, DC and attack; the slots as circles;
 * the cantrips and spells); "Equipamento" is its own panel (`EquipmentPanel`).
 */
@Component({
  selector: 'app-combat-column',
  imports: [CombatStats, SpellHelp],
  templateUrl: './combat-column.html',
  styleUrl: './combat-column.scss',
})
export class CombatColumn {
  readonly sheet = input.required<FullSheetVm>();
  /** This character's live vitals while its campaign has an open session; `null` otherwise. */
  readonly vitals = input<VitalsVm | null>(null);

  /** Only to show what Ajuda adds. */
  protected readonly aidVitals = computed(() => {
    const v = this.vitals();
    return v && (v.hitPointsMaxBonus ?? 0) > 0 && !v.wildShape ? v : null;
  });

  /** The live slot rows (with what is spent) are on screen in the counters: the sheet's own circles, which only count the slots there are, are left out so no slot is drawn twice. */
  readonly liveSlots = input(false);

  protected readonly abilityLabel = abilityLabel;
  protected readonly formatModifier = formatModifier;
  protected readonly spellLimitsText = spellLimitsText;
  protected readonly grantedLevel = grantedSpellLevel;
  protected readonly grantedTags = grantedSpellTags;
  protected readonly grantedNumbers = grantedSpellNumbers;

  /** A tap on a spell (its name or its "?"): the key, so the sheet opens its description. */
  readonly spellOpened = output<string>();
  /** The spells by level, each with its tags and, for a class that prepares, whether it is prepared. */
  protected readonly spellGroupList = computed(() =>
    spellGroups(this.sheet().spells, this.sheet().spellcasting),
  );
  protected readonly stateSummary = computed(() => spellStateSummary(this.spellGroupList()));

  /** "Magias de mago" for a single caster class, as the paper sheet's
   * spellcasting page names its class; "Magias" for a multiclass. */
  protected readonly spellsTitle = computed(() => {
    const classes = this.sheet().spellcasting;
    return classes.length === 1 ? `Magias de ${classes[0].className.toLowerCase()}` : 'Magias';
  });
  protected readonly slotRows = computed(() => spellSlotRows(this.sheet().spellSlots));
  protected readonly pactRow = computed(() => pactSlotRow(this.sheet().pactSlots));
  protected readonly hasSaveAttack = computed(() => this.sheet().attacks.some((a) => a.saveDc > 0));

  protected range(count: number): number[] {
    return Array.from({ length: count }, (_, i) => i);
  }
}
