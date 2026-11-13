import { Component, input } from '@angular/core';

import {
  abilityLabel,
  formatModifier,
  skillProficiencyLabel,
} from '../../../core/characters/character-labels';
import { weightWithPounds } from '../../../core/units';
import { FullSheetVm } from '../character-sheet.types';
import { ABILITY_ABBREVIATIONS } from '../sheet-format';

/**
 * The paper sheet's second column: the proficiency bonus, "Testes de resistência" and
 * "Perícias" as proficiency rows (a dot, the bonus, the name and, for a
 * skill, the ability), then the three passives and the senses. The dot is
 * never the only signal: each row also says "proficiente" (or the level) to
 * a screen reader.
 */
@Component({
  selector: 'app-proficiency-column',
  templateUrl: './proficiency-column.html',
  styleUrl: './proficiency-column.scss',
})
export class ProficiencyColumn {
  readonly sheet = input.required<FullSheetVm>();

  protected readonly abilityLabel = abilityLabel;
  protected readonly abbreviation = ABILITY_ABBREVIATIONS;
  protected readonly formatModifier = formatModifier;
  protected readonly skillProficiencyLabel = skillProficiencyLabel;
  protected readonly weight = weightWithPounds;
}
