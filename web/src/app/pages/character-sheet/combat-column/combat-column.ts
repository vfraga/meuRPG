import { Component, computed, input } from '@angular/core';

import {
  abilityLabel,
  formatModifier,
  splitArmorDescription,
} from '../../../core/characters/character-labels';
import { FullSheetVm } from '../character-sheet.types';
import { CombatStats } from '../combat-stats/combat-stats';
import { coinEntries, pactSlotRow, spellLimitsText, spellSlotRows } from '../sheet-format';

/**
 * The paper sheet's middle column: the combat numbers, "Ataques" (a real
 * table: the weapons carried, then the damage cantrips, in the server's
 * order), "Magias" (per class: ability, DC and attack; the slots as circles;
 * the cantrips and spells) and "Equipamento" (armour, shield, weapons,
 * items, then the coins).
 */
@Component({
  selector: 'app-combat-column',
  imports: [CombatStats],
  templateUrl: './combat-column.html',
  styleUrl: './combat-column.scss',
})
export class CombatColumn {
  readonly sheet = input.required<FullSheetVm>();

  protected readonly abilityLabel = abilityLabel;
  protected readonly formatModifier = formatModifier;
  protected readonly spellLimitsText = spellLimitsText;

  /** "Magias de mago" for a single caster class, as the paper sheet's
   * spellcasting page names its class; "Magias" for a multiclass. */
  protected readonly spellsTitle = computed(() => {
    const classes = this.sheet().spellcasting;
    return classes.length === 1 ? `Magias de ${classes[0].className.toLowerCase()}` : 'Magias';
  });
  protected readonly slotRows = computed(() => spellSlotRows(this.sheet().spellSlots));
  protected readonly pactRow = computed(() => pactSlotRow(this.sheet().pactSlots));
  protected readonly hasSaveAttack = computed(() => this.sheet().attacks.some((a) => a.saveDc > 0));

  /** The armour's own name, only when the stored sheet has armour: without
   * it, the AC description may name a feature (Unarmored Defense), not
   * something carried. */
  protected readonly armorName = computed(() => {
    const s = this.sheet();
    return s.wearsArmor ? splitArmorDescription(s.armorClassDescription).armorNamePt : null;
  });
  /** The weapons are the attacks of kind `weapon` (a damage cantrip is not
   * equipment): no separate request for `FullSheet.weapon_keys`' names. */
  protected readonly weaponNames = computed(() =>
    this.sheet()
      .attacks.filter((a) => a.kind === 'weapon')
      .map((a) => a.namePt),
  );
  protected readonly coins = computed(() => coinEntries(this.sheet().coins));

  protected range(count: number): number[] {
    return Array.from({ length: count }, (_, i) => i);
  }
}
