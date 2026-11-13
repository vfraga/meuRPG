import { timestampFromDate } from '@bufbuild/protobuf/wkt';

import {
  Alignment,
  BasicSheet,
  DamageType,
  Character,
  CharacterKind,
  CharacterState,
  CharacterStory,
  CuttingWordsAsk,
  FullSheet,
  LevelUpReason,
  Review,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  Attack,
  AttackKind,
  CreatureSize,
  DerivedSheet,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { BasicSheetVm, FullSheetVm } from './character-sheet.types';
import { toCharacterSheetVm, toCharacterStoryInit, toStoryVm } from './character-sheet-map';

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
    carryCapacityLb: 150,
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
    states: [],
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
    hitPointsFromEffects: 0,
    resistances: [],
    shieldIdle: false,
    gear: [],
    itemModifiers: [],
    size: '',
    breathesWater: false,
    grantedSpells: [],
    rollDice: [],
    rollMinimums: [],
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
    carriedKeys: [],
    weaponMarks: [],
    itemBonuses: [],
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
    featKeys: [],
    featSlots: {},
    challengeRating: '',
    portraitImageId: '',
    contentRevision: 0,
    knownIssues: [],
    contentBaselines: {},
    featureChoiceText: {},
    cuttingWordsAsk: CuttingWordsAsk.UNSPECIFIED,
    xpValue: 0,
    size: '',
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
    canRequestChanges: false,
    canResubmit: false,
    canRevive: false,
    deathEncounterId: '',
    revivifyBlocked: false,
    canLevelUp: false,
    levelUpReason: LevelUpReason.UNSPECIFIED,
    reserved: false,
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

describe('the sheet shows the other speeds and the water breathing', () => {
  const character = () => characterWithFullSheet(minimalFullSheet());
  const extrasOf = (derived: Partial<DerivedSheet>) => {
    const vm = toCharacterSheetVm({
      ...character(),
      derived: { ...minimalDerivedSheet(), ...derived },
    });
    if (vm.sheet.kind !== 'full') throw new Error('expected a full sheet');
    return vm.sheet.speedExtras;
  };

  it('lists each speed above 0 in meters and feet, in a fixed order, then "Respira na água"', () => {
    const lines = extrasOf({ speedSwimFt: 30, speedFlyFt: 10, breathesWater: true }).map((l) =>
      l.replace(/\s/g, ' '),
    );
    expect(lines).toEqual(['Voo 3 m (10 pés)', 'Natação 9 m (30 pés)', 'Respira na água']);
  });

  it('has nothing for a character with only a walking speed', () => {
    expect(extrasOf({})).toEqual([]);
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
          descriptionPt: 'Você aprendeu a recuperar parte da sua energia mágica.',
          descriptionPtMissing: false,
          descriptionPtOnly: false,
          summaryPt: '',
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
        descriptionPt: 'Você aprendeu a recuperar parte da sua energia mágica.',
        descriptionPtMissing: false,
        descriptionPtOnly: false,
        summaryPt: '',
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

describe("the sheet maps the background equipment and a versatile weapon's two-handed damage", () => {
  it('carries background_equipment_pt and versatile_damage straight through', () => {
    const derived: DerivedSheet = {
      ...minimalDerivedSheet(),
      backgroundEquipmentPt: 'Uma capa e 10 po',
      attacks: [
        {
          $typeName: 'meurpg.rules.v1.Attack',
          key: 'equipment:quarterstaff',
          namePt: 'Bordão',
          kind: AttackKind.WEAPON,
          attackBonus: 2,
          damage: '1d6',
          damageTypePt: 'concussão',
          versatileDamage: '1d8',
          saveDc: 0,
        } as Attack,
      ],
    };
    const sheet = toCharacterSheetVm({
      ...characterWithFullSheet(minimalFullSheet()),
      derived,
    }).sheet as FullSheetVm;
    expect(sheet.backgroundEquipment).toBe('Uma capa e 10 po');
    expect(sheet.attacks[0].versatileDamage).toBe('1d8');
    const bare = toCharacterSheetVm(characterWithFullSheet(minimalFullSheet()))
      .sheet as FullSheetVm;
    expect(bare.backgroundEquipment).toBe('');
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

  it('carries what the spell list needs: level, prepared, and the ritual, concentration and reaction tags', () => {
    const entry = (key: string, level: number, prepared: boolean, over: object = {}) => ({
      $typeName: 'meurpg.rules.v1.CharacterSpell' as const,
      prepared,
      spell: {
        $typeName: 'meurpg.rules.v1.Spell' as const,
        key,
        name: key,
        namePt: key,
        level,
        schoolKey: '',
        schoolNamePt: '',
        classKeys: [],
        ritual: false,
        concentration: false,
        archived: false,
        off: false,
        reaction: false,
        ...over,
      },
    });
    const sheet = withDerived({
      spells: [
        entry('spell:shield', 1, true, { reaction: true }),
        entry('spell:detect-magic', 1, false, { ritual: true, concentration: true }),
      ],
    });
    expect(sheet.spells).toEqual([
      expect.objectContaining({
        key: 'spell:shield',
        level: 1,
        prepared: true,
        reaction: true,
        ritual: false,
      }),
      expect.objectContaining({
        key: 'spell:detect-magic',
        prepared: false,
        ritual: true,
        concentration: true,
        reaction: false,
      }),
    ]);
  });

  it('keeps what a class prepares apart from what a class knows', () => {
    const casting = (over: Record<string, number | boolean>) => ({
      $typeName: 'meurpg.rules.v1.Spellcasting' as const,
      classKey: 'class:x',
      classNamePt: 'Classe',
      ability: 0,
      saveDc: 13,
      attackBonus: 5,
      cantripsKnown: 4,
      preparedMax: 0,
      spellsKnown: 0,
      spellbook: false,
      ...over,
    });
    const [wizard, sorcerer] = withDerived({
      spellcasting: [casting({ preparedMax: 7, spellbook: true }), casting({ spellsKnown: 5 })],
    }).spellcasting;
    expect(wizard).toMatchObject({ spellsPreparedMax: 7, spellsKnownMax: 0, spellbook: true });
    expect(sorcerer).toMatchObject({ spellsPreparedMax: 0, spellsKnownMax: 5 });
  });
});

describe('the master review of a pending character, read from Character.review', () => {
  const base = characterWithFullSheet(minimalFullSheet({}));

  it('maps the status, the reason and the dates', () => {
    const when = new Date(2026, 9, 8, 21, 10);
    const vm = toCharacterSheetVm({
      ...base,
      canRequestChanges: true,
      review: {
        $typeName: 'meurpg.characters.v1.CharacterReview',
        status: Review.CHANGES_REQUESTED,
        reason: 'Falta o equipamento.',
        requestedAt: timestampFromDate(when),
        resubmittedAt: undefined,
      },
    });
    expect(vm.canRequestChanges).toBe(true);
    expect(vm.review).toEqual({
      status: 'changes_requested',
      reason: 'Falta o equipamento.',
      requestedAt: when,
      resubmittedAt: null,
    });
  });

  it('has no review for anyone who may not read it', () => {
    expect(toCharacterSheetVm({ ...base, review: undefined }).review).toBeNull();
  });
});

describe('the revival of a dead character, read from Character', () => {
  const base = characterWithFullSheet(minimalFullSheet({}));

  it('maps who may revive and when it last happened', () => {
    const when = new Date(2026, 9, 9, 10, 30);
    const vm = toCharacterSheetVm({
      ...base,
      state: CharacterState.DEAD,
      canRevive: true,
      revivedAt: timestampFromDate(when),
    });
    expect(vm.canRevive).toBe(true);
    expect(vm.revivedAt).toEqual(when);
  });

  it('has no revival for a character that never came back', () => {
    const vm = toCharacterSheetVm({ ...base, revivedAt: undefined });
    expect(vm.canRevive).toBe(false);
    expect(vm.revivedAt).toBeNull();
  });
});

describe('the sheet maps the breath weapon, the resistances and the one-line rule of a pick (PM-05)', () => {
  const resistance = (damageTypeNamePt: string) => ({
    $typeName: 'meurpg.rules.v1.Resistance' as const,
    damageType: `damage-type:${damageTypeNamePt}`,
    damageTypeNamePt,
    sourceKey: 'trait:x',
  });

  it('carries the breath weapon text, the resistances by name and the summary of a feature', () => {
    const derived: DerivedSheet = {
      ...minimalDerivedSheet(),
      breathWeapon: {
        $typeName: 'meurpg.rules.v1.BreathWeapon',
        textPt: 'Sopro em cone. Dano de 2d6 de fogo.',
      } as DerivedSheet['breathWeapon'],
      resistances: [resistance('fogo'), resistance('veneno')],
      features: [
        {
          $typeName: 'meurpg.rules.v1.Feature',
          key: 'feature:fighting-style-defense',
          name: 'Defense',
          namePt: 'Defesa',
          sourcePt: 'Guerreiro 1',
          description: '+1 AC.',
          descriptionPt: '',
          descriptionPtMissing: true,
          descriptionPtOnly: false,
          summaryPt: '+1 na CA com armadura.',
        },
      ],
    };

    const sheet = toCharacterSheetVm({
      ...characterWithFullSheet(minimalFullSheet()),
      derived,
    }).sheet as FullSheetVm;

    expect(sheet.breathWeapon).toBe('Sopro em cone. Dano de 2d6 de fogo.');
    expect(sheet.resistances).toEqual(['Fogo', 'Veneno']);
    expect(sheet.features[0].summaryPt).toBe('+1 na CA com armadura.');
  });

  it('has no breath weapon and no resistance by default', () => {
    const sheet = toCharacterSheetVm(characterWithFullSheet(minimalFullSheet()))
      .sheet as FullSheetVm;

    expect(sheet.breathWeapon).toBe('');
    expect(sheet.resistances).toEqual([]);
  });
});

describe('the sheet lists the feats a character took with the features (MR-025)', () => {
  it('shows a feat as a feature whose source is "Talento"', () => {
    const derived: DerivedSheet = {
      ...minimalDerivedSheet(),
      features: [
        {
          $typeName: 'meurpg.rules.v1.Feature',
          key: 'feat:atleta@mesa',
          name: '',
          namePt: 'Atleta',
          sourcePt: 'Talento',
          description: 'Você corre e escala melhor.',
          descriptionPt: 'Você corre e escala melhor.',
          descriptionPtMissing: false,
          descriptionPtOnly: true,
          summaryPt: '',
        },
      ],
    };
    const character = characterWithFullSheet(minimalFullSheet({ featKeys: ['feat:atleta@mesa'] }));
    const sheet = toCharacterSheetVm({ ...character, derived }).sheet as FullSheetVm;
    expect(sheet.features).toEqual([
      {
        name: 'Atleta',
        sourcePt: 'Talento',
        description: 'Você corre e escala melhor.',
        descriptionPt: 'Você corre e escala melhor.',
        descriptionPtMissing: false,
        descriptionPtOnly: true,
        summaryPt: '',
      },
    ]);
  });
});
