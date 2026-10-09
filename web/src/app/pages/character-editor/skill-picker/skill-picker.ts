import { Component, computed, input, output } from '@angular/core';
import { MatCheckboxModule } from '@angular/material/checkbox';

import { SkillOptionVm } from '../character-editor.types';
import { abilityAbbreviation, countLabel } from '../editor-labels';

/**
 * The "Perícias" step: one row per skill, like the sheet's proficiency
 * rows — the proficiency checkbox (named by the skill alone), the ability
 * abbreviation, and "Especialização", which only a proficient skill can
 * take. The state and its rules stay in `CharacterEditor`
 * (`toggleSkill`/`toggleExpertise`); this only shows it and reports clicks.
 *
 * `proficient` holds the skills the player picked; `granted` the ones the race
 * or the background already give, with where each comes from. A granted skill
 * shows taken and locked, and is no pick: it is left out of the count, as the
 * server leaves it out.
 */
@Component({
  selector: 'app-skill-picker',
  imports: [MatCheckboxModule],
  templateUrl: './skill-picker.html',
  styleUrl: './skill-picker.scss',
})
export class SkillPicker {
  readonly skills = input.required<readonly SkillOptionVm[]>();
  readonly proficient = input.required<ReadonlySet<string>>();
  /** Skill key to its source in Portuguese ("da raça", "do antecedente"). */
  readonly granted = input<ReadonlyMap<string, string>>(new Map());
  readonly expertise = input.required<ReadonlySet<string>>();

  readonly toggleSkill = output<string>();
  readonly toggleExpertise = output<string>();

  protected readonly abilityAbbreviation = abilityAbbreviation;
  /** Two columns on a wide screen, read top to bottom like the sheet's
   * list (the first half, then the second); stacked on a phone. */
  protected readonly columns = computed(() => {
    const skills = this.skills();
    const half = Math.ceil(skills.length / 2);
    return [skills.slice(0, half), skills.slice(half)].filter((column) => column.length > 0);
  });
  protected isProficient(key: string): boolean {
    return this.proficient().has(key) || this.granted().has(key);
  }
  protected readonly summary = computed(() => {
    const granted = this.granted().size;
    const parts = [
      countLabel(
        this.proficient().size,
        'perícia marcada',
        'perícias marcadas',
        'Nenhuma perícia marcada',
      ),
    ];
    if (granted > 0) {
      parts.push(countLabel(granted, 'já concedida', 'já concedidas', ''));
    }
    const expertise = this.expertise().size;
    if (expertise > 0) {
      parts.push(`${expertise} com especialização`);
    }
    return parts.join(', ');
  });
}
