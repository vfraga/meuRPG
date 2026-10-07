import {
  Alignment,
  Character,
  CharacterKind,
  CharacterState,
  FullSheet,
  LevelUpReason,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { DerivedSheet } from '../../../gen/meurpg/rules/v1/rules_pb';
import { FullSheetVm } from './character-sheet.types';
import { toCharacterSheetVm } from './character-sheet-source.live';

/** Just enough to be a valid `DerivedSheet` — every array present, no
 * values these tests care about (alignment and XP come from `FullSheet`,
 * not `DerivedSheet` — that is the whole point of the follow-up). */
function minimalDerivedSheet(): DerivedSheet {
  return {
    $typeName: 'meurpg.rules.v1.DerivedSheet',
    contentVersion: 'srd51@test',
    raceNamePt: 'Gnomo da Rocha',
    subraceNamePt: '',
    backgroundNamePt: 'Sábio',
    classes: [],
    totalLevel: 3,
    proficiencyBonus: 2,
    abilities: [],
    savingThrows: [],
    skills: [],
    passivePerception: 10,
    passiveInvestigation: 10,
    passiveInsight: 10,
    initiative: 0,
    armorClass: 10,
    armorClassDescription: '',
    hitPointsMax: 10,
    hitDice: [],
    speedWalkFt: 25,
    senses: [],
    spellcasting: [],
    spellSlots: [],
    spells: [],
    attacks: [],
    features: [],
    languages: [],
    hints: [],
    issues: [],
    resources: [],
    actions: [],
    standardActions: [],
    nextLevelXp: 2700,
    speedFlyFt: 0,
    speedSwimFt: 0,
    speedClimbFt: 0,
    speedBurrowFt: 0,
    hover: false,
    saveActions: [],
    changedContent: [],
    backgroundEquipmentPt: '',
  };
}

function minimalFullSheet(overrides: Partial<FullSheet> = {}): FullSheet {
  return {
    $typeName: 'meurpg.characters.v1.FullSheet',
    baseScores: undefined,
    raceKey: 'race:gnome',
    subraceKey: 'subrace:rock-gnome',
    classes: [],
    background: { case: 'backgroundKey', value: 'background:acolyte' },
    skillProficiencyKeys: [],
    expertiseSkillKeys: [],
    extraAbilityBonuses: undefined,
    hitPoints: undefined,
    armorKey: '',
    shield: false,
    weaponKeys: [],
    cantripKeys: [],
    knownSpellKeys: [],
    preparedSpellKeys: [],
    equipment: [],
    coins: undefined,
    languages: [],
    toolProficiencies: [],
    experiencePoints: 0,
    alignment: Alignment.UNSPECIFIED,
    customFeaturesText: '',
    featureChoiceKeys: [],
    challengeRating: '',
    portraitImageId: '',
    contentRevision: 0,
    knownIssues: [],
    contentBaselines: {},
    xpValue: 0,
    ...overrides,
  };
}

function characterWithFullSheet(full: FullSheet): Character {
  return {
    $typeName: 'meurpg.characters.v1.Character',
    id: 'char-1',
    campaignId: 'camp-1',
    kind: CharacterKind.PLAYER,
    state: CharacterState.DRAFT,
    name: 'Pensantus',
    playerUserId: 'user-1',
    playerDisplayName: 'Vinicius',
    sheet: {
      $typeName: 'meurpg.characters.v1.CharacterSheet',
      content: { case: 'full', value: full },
    },
    story: undefined,
    derived: minimalDerivedSheet(),
    revision: 1,
    sheetLockedAt: undefined,
    diedAt: undefined,
    createdAt: undefined,
    updatedAt: undefined,
    canEdit: true,
    canEditStory: true,
    canMarkDead: false,
    canAccessMasterNotes: false,
    storyEditingAllowed: false,
    canSetStoryEditing: false,
    canApprove: false,
    canLevelUp: false,
    levelUpReason: LevelUpReason.UNSPECIFIED,
  };
}


// Finding U17-1 (review/unit-17-contract.md)
describe('Review17 U17-1: Warlock pact slots are missing from the character sheet', () => {
  it('exposes derived.pactMagic (a Warlock has spellSlots = [] and only pact slots) in the sheet view-model', () => {
    const derived: DerivedSheet = {
      ...minimalDerivedSheet(),
      spellSlots: [],
      pactMagic: { $typeName: 'meurpg.rules.v1.PactMagic', slotLevel: 1, count: 2 },
    };
    const vm = toCharacterSheetVm({ ...characterWithFullSheet(minimalFullSheet()), derived });
    const sheet = vm.sheet as FullSheetVm;

    // Some view-model field mentioning "pact" must carry the 2 slots of level 1.
    const pactKeys = Object.keys(sheet).filter((k) => /pact/i.test(k));
    expect(pactKeys.length).toBeGreaterThan(0);
    const dump = JSON.stringify(pactKeys.map((k) => (sheet as unknown as Record<string, unknown>)[k]));
    expect(dump).toMatch(/2/);
  });
});
