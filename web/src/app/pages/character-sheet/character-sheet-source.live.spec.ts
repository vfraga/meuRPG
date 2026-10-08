import {
  Alignment,
  BasicSheet,
  DamageType,
  Character,
  CharacterKind,
  CharacterState,
  CharacterStory,
  FullSheet,
  LevelUpReason,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { CreatureSize, DerivedSheet } from '../../../gen/meurpg/rules/v1/rules_pb';
import { BasicSheetVm, FullSheetVm } from './character-sheet.types';
import { toCharacterSheetVm, toCharacterStoryInit, toStoryVm } from './character-sheet-source.live';

describe('story round-trips load → save unchanged (integrator fix, phase 2b)', () => {
  it('a fully populated CharacterStory survives toStoryVm → toCharacterStoryInit', () => {
    const loaded: CharacterStory = {
      $typeName: 'meurpg.characters.v1.CharacterStory',
      personality: {
        $typeName: 'meurpg.characters.v1.Personality',
        traits: 'Fala sozinho quando pensa.',
        ideals: 'Conhecimento acima de tudo.',
        bonds: 'Deve à sua guilda.',
        flaws: 'Curioso demais para o próprio bem.',
      },
      appearance: {
        $typeName: 'meurpg.characters.v1.Appearance',
        age: '112 anos',
        height: '1,05 m',
        weight: '20 kg',
        eyes: 'castanhos',
        skin: 'bronzeada',
        hair: 'grisalho, curto',
        description: 'Sempre com um livro debaixo do braço.',
      },
      backstory: 'Cresceu numa vila de gnomos nas colinas.',
      allies: 'A Guilda dos Arcanistas.',
    };

    const vm = toStoryVm(loaded);
    const wireInit = toCharacterStoryInit(vm);

    // Every field the server sent comes back exactly as it was — nothing
    // the story form doesn't show (there is nothing it doesn't show) gets
    // silently dropped or blanked.
    expect(wireInit).toEqual({
      personality: {
        traits: loaded.personality!.traits,
        ideals: loaded.personality!.ideals,
        bonds: loaded.personality!.bonds,
        flaws: loaded.personality!.flaws,
      },
      appearance: {
        age: loaded.appearance!.age,
        height: loaded.appearance!.height,
        weight: loaded.appearance!.weight,
        eyes: loaded.appearance!.eyes,
        skin: loaded.appearance!.skin,
        hair: loaded.appearance!.hair,
        description: loaded.appearance!.description,
      },
      backstory: loaded.backstory,
      allies: loaded.allies,
    });
  });

  it('an unset story maps to all-empty fields, never undefined', () => {
    const vm = toStoryVm(undefined);
    expect(vm).toEqual({
      personality: { traits: '', ideals: '', bonds: '', flaws: '' },
      appearance: {
        age: '',
        height: '',
        weight: '',
        eyes: '',
        skin: '',
        hair: '',
        description: '',
      },
      backstory: '',
      allies: '',
    });
  });
});

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

describe('the sheet header shows alignment and XP, read from the stored FullSheet (integrator follow-up)', () => {
  it('maps a chosen alignment to its Portuguese label', () => {
    const vm = toCharacterSheetVm(
      characterWithFullSheet(minimalFullSheet({ alignment: Alignment.CHAOTIC_GOOD })),
    );
    expect(vm.alignmentLabel).toBe('Caótico e bom');
  });

  it('maps an unset alignment to an empty label — "nothing when unset"', () => {
    const vm = toCharacterSheetVm(
      characterWithFullSheet(minimalFullSheet({ alignment: Alignment.UNSPECIFIED })),
    );
    expect(vm.alignmentLabel).toBe('');
  });

  it('reads experience points straight from the FullSheet, 0 included', () => {
    const vm = toCharacterSheetVm(
      characterWithFullSheet(minimalFullSheet({ experiencePoints: 0 })),
    );
    expect(vm.experiencePoints).toBe(0);

    const vmWithXp = toCharacterSheetVm(
      characterWithFullSheet(minimalFullSheet({ experiencePoints: 2700 })),
    );
    expect(vmWithXp.experiencePoints).toBe(2700);
  });

  it("maps a pending character and the master's can_approve (MR-024)", () => {
    const vm = toCharacterSheetVm({
      ...characterWithFullSheet(minimalFullSheet()),
      state: CharacterState.PENDING,
      canApprove: true,
    });
    expect(vm.state).toBe('pending');
    expect(vm.canApprove).toBe(true);
  });

  it('has no alignment or XP for a BasicSheet NPC', () => {
    const basic: BasicSheet = {
      $typeName: 'meurpg.characters.v1.BasicSheet',
      monsterKey: '',
      combatOnly: false,
      size: CreatureSize.UNSPECIFIED,
      hitPointsMax: 7,
      armorClass: 13,
      speedFt: 30,
      attackBonus: 0,
      damage: '',
      description: 'Um goblin arisco.',
      initiativeBonus: 2,
      attacks: [
        {
          $typeName: 'meurpg.characters.v1.BasicAttack',
          name: 'Cimitarra',
          attackBonus: 4,
          damageDiceCount: 1,
          damageDiceSides: 6,
          damageBonus: 2,
          damageType: DamageType.SLASHING,
          rangeFt: 0,
        },
      ],
      challengeRating: '',
      xpValue: 0,
      portraitImageId: '',
    };
    const character: Character = {
      ...characterWithFullSheet(minimalFullSheet()),
      kind: CharacterKind.MINION,
      sheet: {
        $typeName: 'meurpg.characters.v1.CharacterSheet',
        content: { case: 'basic', value: basic },
      },
      derived: undefined,
    };

    const vm = toCharacterSheetVm(character);
    expect(vm.alignmentLabel).toBe('');
    expect(vm.experiencePoints).toBeNull();
    const sheet = vm.sheet as BasicSheetVm;
    expect(sheet.initiativeBonus).toBe(2);
    expect(sheet.attacks).toEqual([
      {
        name: 'Cimitarra',
        attackBonus: 4,
        damageDiceCount: 1,
        damageDiceSides: 6,
        damageBonus: 2,
        damageType: 'slashing',
      },
    ]);
  });
});

describe('the sheet header names the race', () => {
  it('uses the subrace name when there is one, the race name otherwise', () => {
    const character = characterWithFullSheet(minimalFullSheet());
    const withSubrace = toCharacterSheetVm({
      ...character,
      derived: { ...minimalDerivedSheet(), raceNamePt: 'Gnomo', subraceNamePt: 'Gnomo das Rochas' },
    });
    expect(withSubrace.raceLabel).toBe('Gnomo das Rochas');
    const raceOnly = toCharacterSheetVm({
      ...character,
      derived: { ...minimalDerivedSheet(), raceNamePt: 'Humano', subraceNamePt: '' },
    });
    expect(raceOnly.raceLabel).toBe('Humano');
  });
});

describe('the sheet maps armor_class_description, features and hints (integrator fix)', () => {
  it("carries armor_class_description, each feature's source_pt, and every hint straight through", () => {
    const derived: DerivedSheet = {
      ...minimalDerivedSheet(),
      armorClassDescription: 'Armadura de couro + escudo',
      features: [
        {
          $typeName: 'meurpg.rules.v1.Feature',
          key: 'feature:arcane-recovery',
          name: 'Arcane Recovery',
          namePt: 'Recuperação Arcana',
          sourcePt: 'Mago 1',
          description: 'You have learned to regain some of your magical energy.',
        },
      ],
      hints: [
        {
          $typeName: 'meurpg.rules.v1.Hint',
          sourceKey: 'trait:gnome-cunning',
          text: 'Vantagem em testes de resistência de INT, SAB e CAR contra magia.',
        },
      ],
    };
    const character = characterWithFullSheet(
      minimalFullSheet({ armorKey: 'equipment:leather-armor', shield: true }),
    );
    const vm = toCharacterSheetVm({ ...character, derived });
    const sheet = vm.sheet as FullSheetVm;

    expect(sheet.armorClassDescription).toBe('Armadura de couro + escudo');
    // The armor and shield lines come from the stored choices, not from
    // parsing the description.
    expect(sheet.wearsArmor).toBe(true);
    expect(sheet.hasShield).toBe(true);
    const unarmored = toCharacterSheetVm(characterWithFullSheet(minimalFullSheet()));
    expect((unarmored.sheet as FullSheetVm).wearsArmor).toBe(false);
    expect((unarmored.sheet as FullSheetVm).hasShield).toBe(false);
    expect(sheet.features).toEqual([
      {
        name: 'Recuperação Arcana',
        sourcePt: 'Mago 1',
        description: 'You have learned to regain some of your magical energy.',
      },
    ]);
    expect(sheet.hints).toEqual([
      {
        sourceKey: 'trait:gnome-cunning',
        text: 'Vantagem em testes de resistência de INT, SAB e CAR contra magia.',
      },
    ]);
  });
});

describe("the sheet maps a Warlock's Pact Magic apart from the spell slots", () => {
  const withDerived = (over: Partial<DerivedSheet>) =>
    toCharacterSheetVm({
      ...characterWithFullSheet(minimalFullSheet()),
      derived: { ...minimalDerivedSheet(), ...over },
    }).sheet as FullSheetVm;

  it('carries the level and the count of the pact slots, with no spell slots', () => {
    const sheet = withDerived({
      spellSlots: [],
      pactMagic: { $typeName: 'meurpg.rules.v1.PactMagic', slotLevel: 1, count: 2 },
    });
    expect(sheet.pactSlots).toEqual({ level: 1, count: 2 });
    expect(sheet.spellSlots).toEqual([]);
  });

  it('keeps the pact slots out of the slots of the other classes of a multiclass', () => {
    const sheet = withDerived({
      spellSlots: [{ $typeName: 'meurpg.rules.v1.SpellSlots', level: 1, count: 3 }],
      pactMagic: { $typeName: 'meurpg.rules.v1.PactMagic', slotLevel: 2, count: 1 },
    });
    expect(sheet.spellSlots).toEqual([3]);
    expect(sheet.pactSlots).toEqual({ level: 2, count: 1 });
  });

  it('has no pact slots without the feature', () => {
    expect(withDerived({}).pactSlots).toBeNull();
  });
});
