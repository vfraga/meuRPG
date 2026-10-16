import { Component, computed, input, output } from '@angular/core';

import {
  abilityLabel,
  formatModifier,
  splitArmorDescription,
} from '../../../core/characters/character-labels';
import type { VitalsVm } from '../../../core/play/live-session.types';
import { SpellHelp } from '../../../shared/spell-details/spell-help';
import { FullSheetVm } from '../character-sheet.types';
import { CombatStats } from '../combat-stats/combat-stats';
import {
  coinEntries,
  keepUnitsTogether,
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
 * the cantrips and spells) and "Equipamento" (armour, shield, weapons,
 * items, the background's equipment as text, then the coins).
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

  /** The armour's own name, only when the stored sheet has armour: without
   * it, the AC description may name a feature (Unarmored Defense), not
   * something carried. */
  protected readonly armorName = computed(() => {
    const s = this.sheet();
    return s.wearsArmor ? splitArmorDescription(s.armorClassDescription).armorNamePt : null;
  });
  /** The weapons are the attacks of kind `weapon` (a damage cantrip is not
   * equipment, and neither is the unarmed strike every character has): no
   * separate request for `FullSheet.weapon_keys`' names. */
  protected readonly weaponNames = computed(() =>
    this.sheet()
      .attacks.filter((a) => a.kind === 'weapon' && a.key !== 'attack:unarmed-strike')
      .map((a) => a.namePt),
  );
  /** The background's equipment text, with each number kept beside its unit. */
  protected readonly backgroundEquipment = computed(() =>
    keepUnitsTogether(this.sheet().backgroundEquipment),
  );
  protected readonly coins = computed(() => coinEntries(this.sheet().coins));

  protected range(count: number): number[] {
    return Array.from({ length: count }, (_, i) => i);
  }
}
