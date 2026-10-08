import { ChangeDetectionStrategy, Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';

import { ABILITY_KEYS } from '../../../core/characters/characters.types';
import { StoryPanel } from './story-panel';
import {
  CharacterSheetSource,
  CharacterSheetVm,
  CharacterStoryVm,
  FullSheetVm,
} from '../character-sheet.types';

function emptyStory(): CharacterStoryVm {
  return {
    personality: { traits: '', ideals: '', bonds: '', flaws: '' },
    appearance: { age: '', height: '', weight: '', eyes: '', skin: '', hair: '', description: '' },
    backstory: '',
    allies: '',
  };
}

function fullSheet(overrides: Partial<FullSheetVm> = {}): FullSheetVm {
  return {
    kind: 'full',
    abilities: ABILITY_KEYS.map((key) => ({ key, score: 10, modifier: 0 })),
    proficiencyBonus: 2,
    savingThrows: [],
    skills: [],
    passivePerception: 10,
    passiveInvestigation: 10,
    passiveInsight: 10,
    initiative: 0,
    armorClass: 10,
    armorClassDescription: 'Sem armadura',
    wearsArmor: false,
    hasShield: false,
    hitPointsMax: 10,
    hitDice: '1d6',
    speedWalkFt: 25,
    senses: [],
    attacks: [],
    spellcasting: [],
    spellSlots: [],
    cantripNames: [],
    spellNames: [],
    features: [],
    languages: [],
    proficiencies: [],
    equipment: [],
    coins: { cp: 0, sp: 0, ep: 0, gp: 0, pp: 0 },
    customFeaturesText: '',
    issues: [],
    changedContent: [],
    hints: [],
    hasWildShape: false,
    pactSlots: null,
    contentVersion: 'srd51@test',
    ...overrides,
  };
}

function vm(overrides: Partial<CharacterSheetVm> = {}): CharacterSheetVm {
  return {
    id: 'char-1',
    campaignId: 'camp-1',
    characterKind: 'player',
    name: 'Pensantus',
    portraitImageId: '',
    state: 'draft',
    canEdit: true,
    sheetLockedAt: null,
    diedAt: null,
    revision: 1,
    sheet: fullSheet(),
    story: emptyStory(),
    canEditStory: true,
    storyEditingAllowed: false,
    canToggleStoryEditing: false,
    canMarkDead: false,
    canAccessMasterNotes: false,
    canApprove: false,
    isMaster: false,
    playerDisplayName: 'Vinicius',
    raceLabel: 'Gnomo da Rocha',
    classSummary: 'Mago 3',
    backgroundLabel: 'Sábio',
    alignmentLabel: 'Neutro e bom',
    experiencePoints: 2700,
    totalLevel: 3,
    nextLevelXp: 2700,
    canLevelUp: false,
    levelUpReason: null,
    challengeRating: '',
    xpValue: 0,
    ...overrides,
  };
}

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [StoryPanel],
  template: '<app-story-panel [vm]="vm()" />',
})
class Host {
  readonly vm = signal<CharacterSheetVm>(vm());
}

describe('StoryPanel: the revision a save is judged against', () => {
  async function editAndSave(reloaded: CharacterSheetVm | null) {
    const calls: Array<{ revision: number; backstory: string }> = [];
    const fake = {
      updateCharacterStory: (
        _c: string,
        _id: string,
        revision: number,
        story: CharacterStoryVm,
      ) => {
        calls.push({ revision, backstory: story.backstory });
        return Promise.reject(new Error('stale'));
      },
    };
    TestBed.configureTestingModule({
      imports: [Host],
      providers: [{ provide: CharacterSheetSource, useValue: fake }],
    });
    const fixture = TestBed.createComponent(Host);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const btn = (t: string) =>
      Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.trim() === t)!;

    fixture.componentInstance.vm.set(vm({ revision: 4 }));
    fixture.detectChanges();
    btn('Editar história').click();
    fixture.detectChanges();
    const area = Array.from(el.querySelectorAll('mat-form-field'))
      .find((f) => f.textContent?.includes('Antecedentes'))!
      .querySelector('textarea')!;
    area.value = 'Minha versão.';
    area.dispatchEvent(new Event('input'));

    if (reloaded) {
      fixture.componentInstance.vm.set(reloaded);
      fixture.detectChanges();
    }
    btn('Salvar história').click();
    await fixture.whenStable();
    return calls;
  }

  it('sends the revision the form was copied from, not one a background reload brought meanwhile', async () => {
    // Another writer saved revision 5 with their own text while the person typed.
    const calls = await editAndSave(
      vm({ revision: 5, story: { ...emptyStory(), backstory: 'Texto do outro autor.' } }),
    );
    expect(calls).toEqual([{ revision: 4, backstory: 'Minha versão.' }]);
  });

  it('sends the revision on screen when nothing changed meanwhile', async () => {
    const calls = await editAndSave(null);
    expect(calls).toEqual([{ revision: 4, backstory: 'Minha versão.' }]);
  });
});
