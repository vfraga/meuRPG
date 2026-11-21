import { signal } from '@angular/core';

import { toggledCapped } from '../../core/format/sets';
import type { ChoiceSelection, ChoiceTextEdit } from '../../shared/choice-groups/choice-groups';
import { applySelection, withChoiceText } from '../../shared/choice-groups/choice-picks';
import type { CharacterFormValue, ExtraClassValue, SpellOptionVm } from './character-editor.types';
import type { ClassBlock } from './class-blocks';
import {
  CUSTOM_BACKGROUND_PROFICIENCIES,
  CUSTOM_BACKGROUND_SKILLS,
  toggledExpertise,
  toggledSkill,
} from './skill-picks';

/**
 * What the person has picked on the page apart from the form's own controls: the skills, the custom background's
 * two skills and two tools or languages, the classes after the first, the expertise, the rolls, the feats and the
 * class and race choices. Plain state the editor builds (the draft, the pickers and the save all read it); the spells
 * picked in the "Magias" step are `SpellPicks`, next to it.
 */
export class EditorPicks {
  readonly selectedSkills = signal<ReadonlySet<string>>(new Set());
  /** The custom background's two granted skills (`CustomBackground.skills`, integrator fix, phase 2) — a separate,
   * capped-at-2 selection from `selectedSkills`, only shown when `background === 'custom'`. */
  readonly customBackgroundSkills = signal<ReadonlySet<string>>(new Set());
  /** The "Outro" background's two tools or languages (content keys), capped at 2 like its skills. */
  readonly customBackgroundProficiencies = signal<readonly string[]>([]);
  /** The classes after the first (multiclass at creation): one block each, see `ClassBlockFields`. */
  readonly extraClasses = signal<readonly ExtraClassValue[]>([]);
  /** The skills and the expertise the saved sheet held when the editor opened: their rows stay in the list for the session, so a
   * skill no list allows that is taken off can be ticked again and the focus stays where it was. */
  readonly heldAtOpen = signal<ReadonlySet<string>>(new Set());
  readonly expertiseHeldAtOpen = signal<ReadonlySet<string>>(new Set());
  /** Expertise doubles a skill's proficiency bonus — always a subset of `selectedSkills` (integrator fix, phase 2b). */
  readonly expertiseSkills = signal<ReadonlySet<string>>(new Set());
  /** One roll per level after the first, index 0 = level 2. Only sent when `hitPointsMethod` is "rolled" (integrator
   * fix, phase 2b). */
  readonly hitPointsRolls = signal<readonly number[]>([]);
  /** The picks of the class and race choices, in the order they were made, and their free texts (PM-05). */
  readonly featureChoiceKeys = signal<readonly string[]>([]);
  readonly featureChoiceText = signal<Readonly<Record<string, string>>>({});
  /** The sheet's feats and the ones taken in place of an improvement (MR-025): the master's "Talentos" section edits them. */
  readonly featKeys = signal<readonly string[]>([]);
  readonly featSlots = signal<Readonly<Record<string, string>>>({});
  /** Spells the sheet has that neither a pick of this form nor its subclass gave it (a race's or a feature's spell):
   * only known on an edit, from the server's own derived sheet, as it was when the sheet was opened. */
  readonly grantedSpells = signal<readonly SpellOptionVm[]>([]);
  /** The saved sheet has Palavras de Interrupção (a College of Lore bard): its "Perguntar" setting shows. Known on an edit. */
  readonly hasCuttingWords = signal(false);
  /** How many spells each casting class prepares, from the saved sheet's derived numbers (an edit only). */
  readonly preparedMaxByClass = signal<Readonly<Record<string, number>>>({});

  /** `grantedSkills` is what the race, the subrace and the background already give (the catalog's, read when needed). */
  constructor(private readonly grantedSkills: () => ReadonlyMap<string, string>) {}

  /** A reused page starts from nothing. */
  reset(): void {
    this.selectedSkills.set(new Set());
    this.heldAtOpen.set(new Set());
    this.expertiseHeldAtOpen.set(new Set());
    this.customBackgroundSkills.set(new Set());
    this.customBackgroundProficiencies.set([]);
    this.extraClasses.set([]);
    this.expertiseSkills.set(new Set());
    this.hitPointsRolls.set([]);
    this.featureChoiceKeys.set([]);
    this.featureChoiceText.set({});
    this.featKeys.set([]);
    this.featSlots.set({});
    this.grantedSpells.set([]);
    this.hasCuttingWords.set(false);
    this.preparedMaxByClass.set({});
  }

  /** The saved sheet's picks (an edit). */
  load(full: CharacterFormValue): void {
    this.selectedSkills.set(new Set(full.skillProficiencies));
    this.heldAtOpen.set(new Set(full.skillProficiencies));
    this.expertiseHeldAtOpen.set(new Set(full.expertiseSkillKeys));
    this.customBackgroundSkills.set(new Set(full.customBackgroundSkills ?? []));
    this.customBackgroundProficiencies.set(full.customBackgroundProficiencies);
    this.extraClasses.set(full.extraClasses);
    this.expertiseSkills.set(new Set(full.expertiseSkillKeys));
    this.hitPointsRolls.set(full.hitPointsRolls);
    this.featureChoiceKeys.set(full.featureChoiceKeys);
    this.featureChoiceText.set(full.featureChoiceText);
    this.featKeys.set(full.featKeys ?? []);
    this.featSlots.set(full.featSlots ?? {});
  }

  readonly toggleSkill = (key: string): void => {
    const next = toggledSkill(
      this.selectedSkills(),
      this.expertiseSkills(),
      this.grantedSkills(),
      key,
    );
    if (!next) {
      return;
    }
    if (next.expertise) {
      this.expertiseSkills.set(next.expertise);
    }
    this.selectedSkills.set(next.skills);
  };

  /** Toggles expertise on a proficient skill (doubles its proficiency
   * bonus — Bard, Rogue). A no-op on a skill that is not proficient yet:
   * the checkbox is disabled for those in the template. */
  readonly toggleExpertise = (key: string): void => {
    const next = toggledExpertise(
      this.selectedSkills(),
      this.grantedSkills(),
      this.expertiseSkills(),
      key,
    );
    if (next) {
      this.expertiseSkills.set(next);
    }
  };

  /** Toggles one of the custom background's two granted skills. Caps at 2
   * (`CustomBackground.skill_keys`: "at most 2"): a third click while two
   * are already chosen does nothing, so the player unchecks one first. */
  readonly toggleCustomBackgroundSkill = (key: string): void => {
    this.customBackgroundSkills.set(
      toggledCapped(this.customBackgroundSkills(), key, CUSTOM_BACKGROUND_SKILLS),
    );
  };

  /** Picks or drops one of the "Outro" background's two tools or languages; a third does nothing, like the skills. */
  readonly setCustomProficiencies = (keys: readonly string[]): void => {
    this.customBackgroundProficiencies.set(keys.slice(0, CUSTOM_BACKGROUND_PROFICIENCIES));
  };

  /** "Adicionar classe": a new block at level 1 with nothing chosen. The server checks the multiclass
   * prerequisites of every class and shows what the sheet does not meet as issues on the sheet. */
  readonly addClass = (): void => {
    this.extraClasses.update((list) => [
      ...list,
      { classKey: '', level: 1, subclassKey: '', customSubclassName: '' },
    ]);
  };

  /** `index` counts the blocks on screen, the form's own first: block 1 is the first extra class. */
  readonly removeClass = (index: number): void => {
    this.extraClasses.update((list) => list.filter((_, i) => i !== index - 1));
  };

  /** A block after the first changed (`index` counts the extra classes from 0). */
  changeExtraClass(index: number, change: Partial<ClassBlock>): void {
    this.extraClasses.update((list) => list.map((c, i) => (i === index ? { ...c, ...change } : c)));
  }

  /** The master gives the sheet a feat (MR-025). */
  readonly addFeat = (key: string): void => {
    this.featKeys.update((keys) => (keys.includes(key) ? keys : [...keys, key]));
  };

  /** The master takes a feat off; the slot of an improvement it replaced goes with it (the server drops it too). */
  readonly removeFeat = (key: string): void => {
    this.featKeys.update((keys) => keys.filter((k) => k !== key));
    this.featSlots.update((slots) =>
      Object.fromEntries(Object.entries(slots).filter(([k]) => k !== key)),
    );
  };

  /** A pick on the "Escolhas" step: the stored keys follow it, and the server reads the draft again. */
  readonly pickChoice = (pick: ChoiceSelection): void => {
    this.featureChoiceKeys.update((keys) => applySelection(keys, pick.choice, pick.optionKeys));
  };

  readonly editChoiceText = (edit: ChoiceTextEdit): void => {
    this.featureChoiceText.update((texts) =>
      withChoiceText(texts, edit.choice.key, edit.n, edit.text),
    );
  };
}
