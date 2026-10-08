import { Component, computed, inject, input, output, signal } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';

import { describeCharacterError } from '../../../core/characters/character-errors';
import { FictionNotice } from '../../../shared/fiction-notice/fiction-notice';
import { CharacterSheetSource, CharacterSheetVm, CharacterStoryVm } from '../character-sheet.types';

type SavingState = { status: 'idle' } | { status: 'saving' } | { status: 'error'; message: string };

interface StoryEntry {
  readonly label: string;
  readonly value: string;
}

const emptyStory: CharacterStoryVm = {
  personality: { traits: '', ideals: '', bonds: '', flaws: '' },
  appearance: { age: '', height: '', weight: '', eyes: '', skin: '', hair: '', description: '' },
  backstory: '',
  allies: '',
};

/**
 * "História" (plan amendment A3): the appearance as a small grid, then the
 * personality, the backstory and the allies, each shown only when filled
 * in. "Editar história" reads `canEditStory` alone, whatever the caller's
 * role; without it, the panel says the story is locked. Saving goes
 * through `UpdateCharacterStory` and hands the updated character back to
 * the page (`changed`).
 */
@Component({
  selector: 'app-story-panel',
  imports: [
    FictionNotice,
    MatButtonModule,
    MatFormFieldModule,
    MatInputModule,
    ReactiveFormsModule,
  ],
  templateUrl: './story-panel.html',
  styleUrl: './story-panel.scss',
})
export class StoryPanel {
  private readonly source = inject(CharacterSheetSource);
  private readonly fb = inject(FormBuilder);

  readonly vm = input.required<CharacterSheetVm>();
  readonly changed = output<CharacterSheetVm>();

  protected readonly editing = signal(false);
  protected readonly saveState = signal<SavingState>({ status: 'idle' });

  protected readonly story = computed(() => this.vm().story ?? emptyStory);
  protected readonly appearance = computed<StoryEntry[]>(() => {
    const a = this.story().appearance;
    return [
      { label: 'Idade', value: a.age },
      { label: 'Altura', value: a.height },
      { label: 'Peso', value: a.weight },
      { label: 'Olhos', value: a.eyes },
      { label: 'Pele', value: a.skin },
      { label: 'Cabelo', value: a.hair },
    ].filter((e) => e.value !== '');
  });
  protected readonly texts = computed<StoryEntry[]>(() => {
    const s = this.story();
    return [
      { label: 'Aparência', value: s.appearance.description },
      { label: 'Traços de personalidade', value: s.personality.traits },
      { label: 'Ideais', value: s.personality.ideals },
      { label: 'Vínculos', value: s.personality.bonds },
      { label: 'Fraquezas', value: s.personality.flaws },
      { label: 'Antecedentes', value: s.backstory },
      { label: 'Aliados', value: s.allies },
    ].filter((e) => e.value !== '');
  });
  protected readonly isEmpty = computed(
    () => this.appearance().length === 0 && this.texts().length === 0,
  );

  protected readonly form = this.fb.nonNullable.group({
    traits: ['', Validators.maxLength(1000)],
    ideals: ['', Validators.maxLength(1000)],
    bonds: ['', Validators.maxLength(1000)],
    flaws: ['', Validators.maxLength(1000)],
    age: ['', Validators.maxLength(40)],
    height: ['', Validators.maxLength(40)],
    weight: ['', Validators.maxLength(40)],
    eyes: ['', Validators.maxLength(40)],
    skin: ['', Validators.maxLength(40)],
    hair: ['', Validators.maxLength(40)],
    appearanceDescription: ['', Validators.maxLength(2000)],
    backstory: ['', Validators.maxLength(10000)],
    allies: ['', Validators.maxLength(2000)],
  });

  /** The revision the form was copied from: the server's lock must judge what the person actually read. */
  private editedRevision = 0;

  protected startEditing(): void {
    const s = this.story();
    this.form.setValue({
      traits: s.personality.traits,
      ideals: s.personality.ideals,
      bonds: s.personality.bonds,
      flaws: s.personality.flaws,
      age: s.appearance.age,
      height: s.appearance.height,
      weight: s.appearance.weight,
      eyes: s.appearance.eyes,
      skin: s.appearance.skin,
      hair: s.appearance.hair,
      appearanceDescription: s.appearance.description,
      backstory: s.backstory,
      allies: s.allies,
    });
    this.editedRevision = this.vm().revision;
    this.saveState.set({ status: 'idle' });
    this.editing.set(true);
  }

  protected cancel(): void {
    this.editing.set(false);
  }

  protected async save(): Promise<void> {
    if (this.form.invalid) {
      this.form.markAllAsTouched();
      return;
    }
    const vm = this.vm();
    const v = this.form.getRawValue();
    const story: CharacterStoryVm = {
      personality: { traits: v.traits, ideals: v.ideals, bonds: v.bonds, flaws: v.flaws },
      appearance: {
        age: v.age,
        height: v.height,
        weight: v.weight,
        eyes: v.eyes,
        skin: v.skin,
        hair: v.hair,
        description: v.appearanceDescription,
      },
      backstory: v.backstory,
      allies: v.allies,
    };
    this.saveState.set({ status: 'saving' });
    try {
      const updated = await this.source.updateCharacterStory(
        vm.campaignId,
        vm.id,
        this.editedRevision,
        story,
      );
      this.saveState.set({ status: 'idle' });
      this.editing.set(false);
      this.changed.emit(updated);
    } catch (err) {
      this.saveState.set({ status: 'error', message: describeCharacterError(err) });
    }
  }
}
