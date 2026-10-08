import {
  Alignment,
  BasicSheet,
  DamageType,
  FullSheet,
  HitPointsMethod,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  CharacterEditorSourceLive,
  mergeFullSheetInit,
  toBasicSheetInit,
  toFormBasicSheet,
  toFormFullSheet,
  toFullSheetInit,
} from './character-editor-source.live';
import { TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';

import { Role } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';

import { CONNECT_TRANSPORT } from '../../core/connect/transport';
import { CreatureSize } from '../../../gen/meurpg/rules/v1/rules_pb';

/**
 * A fully populated `FullSheet` — every field set, including the two the
 * editor form has no UI for (`coins`, `feature_choice_keys`) — for the
 * integrator's no-data-loss fix (phase 2b): loading it, then saving without
 * touching anything, must send back exactly what was loaded.
 *
 * One class only: the MVP editor's own, already-documented scope limit
 * (one class; more is multiclassing) is not what this test is about — it
 * covers everything the editor's fields actually claim to edit.
 */
function fullyPopulatedFullSheet(): FullSheet {
  return {
    $typeName: 'meurpg.characters.v1.FullSheet',
    baseScores: {
      $typeName: 'meurpg.rules.v1.AbilityScores',
      strength: 8,
      dexterity: 14,
      constitution: 16,
      intelligence: 18,
      wisdom: 12,
      charisma: 10,
    },
    raceKey: 'race:gnome',
    subraceKey: 'subrace:rock-gnome',
    classes: [
      {
        $typeName: 'meurpg.characters.v1.ClassLevel',
        classKey: 'class:wizard',
        level: 3,
        subclass: { case: 'subclassKey', value: 'subclass:evocation' },
      },
    ],
    background: {
      case: 'backgroundKey',
      value: 'background:acolyte',
    },
    skillProficiencyKeys: ['skill:arcana', 'skill:history', 'skill:investigation'],
    expertiseSkillKeys: ['skill:arcana'],
    extraAbilityBonuses: {
      $typeName: 'meurpg.rules.v1.AbilityScores',
      strength: 0,
      dexterity: 0,
      constitution: 1,
      intelligence: 2,
      wisdom: 0,
      charisma: 0,
    },
    hitPoints: {
      $typeName: 'meurpg.characters.v1.HitPoints',
      method: HitPointsMethod.ROLLED,
      rolls: [4, 6, 2],
    },
    armorKey: '',
    shield: false,
    weaponKeys: ['equipment:quarterstaff', 'equipment:dagger'],
    cantripKeys: ['spell:fire-bolt', 'spell:ray-of-frost', 'spell:minor-illusion'],
    knownSpellKeys: [
      'spell:magic-missile',
      'spell:burning-hands',
      'spell:shield',
      'spell:mage-armor',
    ],
    preparedSpellKeys: ['spell:magic-missile', 'spell:shield', 'spell:mage-armor'],
    equipment: [
      { $typeName: 'meurpg.characters.v1.Item', name: 'Grimório', quantity: 1 },
      { $typeName: 'meurpg.characters.v1.Item', name: 'Tocha', quantity: 3 },
    ],
    // No UI collects this — the field this test exists to protect.
    coins: {
      $typeName: 'meurpg.characters.v1.Coins',
      copper: 5,
      silver: 12,
      electrum: 0,
      gold: 30,
      platinum: 1,
    },
    languages: ['Anão', 'Élfico'],
    toolProficiencies: ['Ferramentas de cartógrafo'],
    experiencePoints: 2700,
    alignment: Alignment.NEUTRAL_GOOD,
    customFeaturesText: 'Sabe um truque de cartas que sempre erra.',
    // No UI collects this either — the plan §4 gap this test also protects.
    featureChoiceKeys: ['feature:fighter-fighting-style-defense'],
    // Nor this: the NPC's challenge rating and the XP it gives (Etapa 7); the
    // editor must not drop them when it saves.
    challengeRating: '2',
    xpValue: 450,
    portraitImageId: '',
    contentRevision: 0,
    knownIssues: [],
    contentBaselines: {},
  };
}

describe('FullSheet round-trips load → save unchanged (integrator fix, phase 2b)', () => {
  it('a fully populated FullSheet survives toFormFullSheet → mergeFullSheetInit, coins and feature_choice_keys included', () => {
    const loaded = fullyPopulatedFullSheet();

    const form = toFormFullSheet('Pensantus', loaded);
    const merged = mergeFullSheetInit(loaded, form);

    expect(merged).toEqual({
      baseScores: {
        strength: loaded.baseScores!.strength,
        dexterity: loaded.baseScores!.dexterity,
        constitution: loaded.baseScores!.constitution,
        intelligence: loaded.baseScores!.intelligence,
        wisdom: loaded.baseScores!.wisdom,
        charisma: loaded.baseScores!.charisma,
      },
      raceKey: loaded.raceKey,
      subraceKey: loaded.subraceKey,
      classes: [
        {
          classKey: loaded.classes[0].classKey,
          level: loaded.classes[0].level,
          subclass: loaded.classes[0].subclass,
        },
      ],
      background: loaded.background,
      skillProficiencyKeys: loaded.skillProficiencyKeys,
      expertiseSkillKeys: loaded.expertiseSkillKeys,
      extraAbilityBonuses: {
        strength: loaded.extraAbilityBonuses!.strength,
        dexterity: loaded.extraAbilityBonuses!.dexterity,
        constitution: loaded.extraAbilityBonuses!.constitution,
        intelligence: loaded.extraAbilityBonuses!.intelligence,
        wisdom: loaded.extraAbilityBonuses!.wisdom,
        charisma: loaded.extraAbilityBonuses!.charisma,
      },
      hitPoints: { method: loaded.hitPoints!.method, rolls: loaded.hitPoints!.rolls },
      armorKey: loaded.armorKey,
      shield: loaded.shield,
      weaponKeys: loaded.weaponKeys,
      cantripKeys: loaded.cantripKeys,
      knownSpellKeys: loaded.knownSpellKeys,
      preparedSpellKeys: loaded.preparedSpellKeys,
      equipment: [
        { name: 'Grimório', quantity: 1 },
        { name: 'Tocha', quantity: 3 },
      ],
      languages: loaded.languages,
      toolProficiencies: loaded.toolProficiencies,
      experiencePoints: loaded.experiencePoints,
      alignment: loaded.alignment,
      customFeaturesText: loaded.customFeaturesText,
      // The whole point: fields the form has no UI for come back exactly
      // as loaded, not wiped to a zero/empty default.
      coins: loaded.coins,
      featureChoiceKeys: loaded.featureChoiceKeys,
      challengeRating: loaded.challengeRating,
      xpValue: loaded.xpValue,
      portraitImageId: loaded.portraitImageId,
      contentRevision: loaded.contentRevision,
      knownIssues: loaded.knownIssues,
      contentBaselines: loaded.contentBaselines,
    });
  });

  it('a custom background with exactly two granted skills round-trips too', () => {
    const loaded: FullSheet = {
      ...fullyPopulatedFullSheet(),
      background: {
        case: 'customBackground',
        value: {
          $typeName: 'meurpg.characters.v1.CustomBackground',
          name: 'Sábio',
          skillKeys: ['skill:arcana', 'skill:history'],
          proficiencyKeys: ['proficiency:thieves-tools', 'language:elvish'],
          featureName: 'Pesquisador',
          featureText: 'Sabe a quem perguntar.',
          equipment: 'Um tinteiro.',
        },
      },
    };

    const form = toFormFullSheet('Pensantus', loaded);
    expect(form.background).toBe('custom');
    expect(form.customBackgroundName).toBe('Sábio');
    expect(form.customBackgroundSkills).toEqual(['skill:arcana', 'skill:history']);
    // The editor shows every field of the "Outro" background now (slice 10.12b).
    expect(form.customBackgroundProficiencies).toEqual([
      'proficiency:thieves-tools',
      'language:elvish',
    ]);
    expect(form.customBackgroundFeatureName).toBe('Pesquisador');
    expect(form.customBackgroundFeatureText).toBe('Sabe a quem perguntar.');
    expect(form.customBackgroundEquipment).toBe('Um tinteiro.');

    const merged = mergeFullSheetInit(loaded, form);
    // `background` is always rebuilt from the form (there is no "not shown
    // by the form" part of it to preserve), so it comes back as a plain
    // init shape, not the branded message `loaded` carried.
    expect(merged.background).toEqual({
      case: 'customBackground',
      // What the form does not show survives the save: the tools or languages, the
      // feature and the equipment the server holds.
      value: {
        name: 'Sábio',
        skillKeys: ['skill:arcana', 'skill:history'],
        proficiencyKeys: ['proficiency:thieves-tools', 'language:elvish'],
        featureName: 'Pesquisador',
        featureText: 'Sabe a quem perguntar.',
        equipment: 'Um tinteiro.',
      },
    });
  });

  it('a sheet of several classes round-trips: the first in the form fields, the others in extraClasses, in order', () => {
    const loaded: FullSheet = {
      ...fullyPopulatedFullSheet(),
      classes: [
        {
          $typeName: 'meurpg.characters.v1.ClassLevel',
          classKey: 'class:wizard',
          level: 3,
          subclass: { case: 'subclassKey', value: 'subclass:ink@mesa' },
        },
        {
          $typeName: 'meurpg.characters.v1.ClassLevel',
          classKey: 'class:cleric',
          level: 1,
          subclass: { case: 'subclassKey', value: 'subclass:path@mesa' },
        },
        {
          $typeName: 'meurpg.characters.v1.ClassLevel',
          classKey: 'class:fighter',
          level: 2,
          subclass: { case: 'customSubclassName', value: 'Duelista' },
        },
      ],
    };
    const form = toFormFullSheet('Corvina', loaded);
    expect(form).toMatchObject({
      className: 'class:wizard',
      level: 3,
      subclassName: 'subclass:ink@mesa',
    });
    expect(form.extraClasses).toEqual([
      {
        classKey: 'class:cleric',
        level: 1,
        subclassKey: 'subclass:path@mesa',
        customSubclassName: '',
      },
      { classKey: 'class:fighter', level: 2, subclassKey: '', customSubclassName: 'Duelista' },
    ]);
    // Saving it unchanged sends every class back, never only the first.
    expect(mergeFullSheetInit(loaded, form).classes).toEqual([
      {
        classKey: 'class:wizard',
        level: 3,
        subclass: { case: 'subclassKey', value: 'subclass:ink@mesa' },
      },
      {
        classKey: 'class:cleric',
        level: 1,
        subclass: { case: 'subclassKey', value: 'subclass:path@mesa' },
      },
      {
        classKey: 'class:fighter',
        level: 2,
        subclass: { case: 'customSubclassName', value: 'Duelista' },
      },
    ]);
    // A block with no class chosen never reaches the wire.
    expect(
      toFullSheetInit({
        ...form,
        extraClasses: [
          ...form.extraClasses,
          { classKey: '', level: 1, subclassKey: '', customSubclassName: '' },
        ],
      }).classes,
    ).toHaveLength(3);
  });

  it('CreateCharacter (no loaded message) is exactly toFullSheetInit — nothing to merge yet', () => {
    const form = toFormFullSheet('Pensantus', fullyPopulatedFullSheet());
    expect(mergeFullSheetInit(undefined, form)).toEqual(toFullSheetInit(form));
  });

  it('sends no hit points rolls on the wire for the "average" method, even if some were typed', () => {
    const loaded = fullyPopulatedFullSheet(); // hitPointsMethod: 'rolled' by fixture
    const form = toFormFullSheet('Pensantus', loaded);

    const stillRolled = toFullSheetInit(form);
    expect(stillRolled.hitPoints).toEqual({ method: loaded.hitPoints!.method, rolls: [4, 6, 2] });

    const switchedToAverage = toFullSheetInit({ ...form, hitPointsMethod: 'average' });
    expect(switchedToAverage.hitPoints.rolls).toEqual([]);
  });
});

describe('the catalog the editor reads (slice 10.12b)', () => {
  const key = (k: string) => k.endsWith('@mesa');

  it("marks an entry of the table by its key, carries the class numbers and a third caster's subclass, and a table class reuses another list", async () => {
    TestBed.configureTestingModule({
      providers: [CharacterEditorSourceLive, { provide: CONNECT_TRANSPORT, useValue: {} }],
    });
    const source = TestBed.inject(CharacterEditorSourceLive);
    // The generated client is a field of the source: the spec gives it the server's answer.
    (source as unknown as { contentClient: unknown }).contentClient = {
      listContent: () =>
        Promise.resolve({
          content: {
            races: [
              {
                key: 'race:gnome',
                namePt: 'Gnomo',
                abilityBonuses: { constitution: 0 },
                choiceBonuses: [],
                archived: false,
              },
              {
                key: 'race:corujeiro@mesa',
                namePt: 'Corujeiro',
                abilityBonuses: undefined,
                choiceBonuses: [2, 1],
                archived: true,
              },
            ],
            subraces: [],
            classes: [
              {
                key: 'class:guardiao@mesa',
                namePt: 'Guardião do Vale',
                hitDie: 10,
                savingThrows: [1, 5],
                skillChoice: { count: 2 },
                subclassLevel: 3,
                spellcasting: {
                  preparation: 2,
                  firstLevel: 2,
                  maxSpellLevelByLevel: [0, 1],
                  listClassKey: 'class:druid',
                },
              },
              {
                key: 'class:fighter',
                namePt: 'Guerreiro',
                hitDie: 10,
                savingThrows: [],
                subclassLevel: 3,
              },
            ],
            subclasses: [
              {
                key: 'subclass:ink@mesa',
                namePt: 'Lâmina de Tinta',
                classKey: 'class:fighter',
                alwaysPrepared: [{ spellKey: 'spell:shield', classLevel: 3 }],
                spellcasting: {
                  preparation: 1,
                  listClassKey: 'class:wizard',
                  firstLevel: 3,
                  maxSpellLevelByLevel: [0, 0, 1],
                },
              },
              {
                key: 'subclass:champion',
                namePt: 'Campeão',
                classKey: 'class:fighter',
                alwaysPrepared: [],
              },
            ],
            backgrounds: [
              {
                key: 'background:cartografo@mesa',
                namePt: 'Cartógrafo do Vale',
                equipmentPt: 'Uma luneta',
              },
            ],
            skills: [],
            armor: [],
            weapons: [],
            spells: [
              {
                key: 'spell:ink-blade@mesa',
                namePt: 'Lâmina de Nanquim',
                level: 1,
                classKeys: ['class:wizard'],
                archived: false,
                off: false,
              },
            ],
            proficiencies: [
              { key: 'proficiency:smiths-tools', namePt: 'Ferramentas de ferreiro', kind: 3 - 2 },
              { key: 'proficiency:thieves-tools', namePt: 'Ferramentas de ladrão', kind: 7 },
              { key: 'proficiency:light-armor', namePt: 'Armaduras leves', kind: 2 },
            ],
            languages: [{ key: 'language:elvish', namePt: 'Élfico', kind: 6 }],
            challengeRatings: [],
          },
        }),
    };
    (source as unknown as { campaignClient: unknown }).campaignClient = {
      getCampaign: () => Promise.resolve({ campaign: { myRole: Role.PLAYER } }),
    };
    const catalog = await source.loadCatalog('camp-1');

    expect(catalog.races.map((r) => [r.key, r.fromTable, r.archived, r.choiceBonuses])).toEqual([
      ['race:gnome', false, false, []],
      ['race:corujeiro@mesa', true, true, [2, 1]],
    ]);
    const guardian = catalog.classes[0];
    expect(guardian).toMatchObject({
      fromTable: true,
      skillChoose: 2,
      savingThrows: ['str', 'wis'],
      spellListClassKey: 'class:druid',
      isCaster: true,
      spellcastingFirstLevel: 2,
    });
    // A class that casts nothing has no list.
    expect(catalog.classes[1]).toMatchObject({
      isCaster: false,
      spellListClassKey: '',
      skillChoose: 0,
    });
    const [ink, champion] = catalog.classes[1].subclasses;
    (source as unknown as { campaignClient: unknown }).campaignClient = {
      getCampaign: () => Promise.resolve({ campaign: { myRole: Role.MASTER } }),
    };
    expect((await source.loadCatalog('camp-1')).viewerIsMaster).toBe(true);
    expect(ink).toMatchObject({
      fromTable: true,
      casting: {
        preparation: 'known',
        listClassKey: 'class:wizard',
        firstLevel: 3,
        maxSpellLevelByLevel: [0, 0, 1],
      },
    });
    expect(champion).toMatchObject({ fromTable: false, casting: null, alwaysPrepared: [] });
    expect(ink.alwaysPrepared).toEqual([{ spellKey: 'spell:shield', classLevel: 3 }]);
    expect(catalog.backgrounds[0]).toMatchObject({ fromTable: true, equipmentPt: 'Uma luneta' });
    expect(catalog.spells[0].fromTable).toBe(true);
    expect(key('x@mesa')).toBe(true);
    // The tools and the languages of the "Outro" background are the catalog's own, by kind: a kit counts as a tool, armour never.
    expect(catalog.toolsAndLanguages).toEqual([
      { key: 'proficiency:smiths-tools', namePt: 'Ferramentas de ferreiro', kind: 'tool' },
      { key: 'proficiency:thieves-tools', namePt: 'Ferramentas de ladrão', kind: 'tool' },
      { key: 'language:elvish', namePt: 'Élfico', kind: 'language' },
    ]);
    expect(catalog.viewerIsMaster).toBe(false);
  });
  describe('who is reading', () => {
    const emptyContent = {
      races: [],
      subraces: [],
      classes: [],
      subclasses: [],
      backgrounds: [],
      skills: [],
      armor: [],
      weapons: [],
      spells: [],
      proficiencies: [],
      languages: [],
      challengeRatings: [],
    };

    function sourceWith(getCampaign: () => Promise<unknown>): CharacterEditorSourceLive {
      TestBed.configureTestingModule({
        providers: [CharacterEditorSourceLive, { provide: CONNECT_TRANSPORT, useValue: {} }],
      });
      const source = TestBed.inject(CharacterEditorSourceLive);
      (source as unknown as { contentClient: unknown }).contentClient = {
        listContent: () => Promise.resolve({ content: emptyContent }),
      };
      (source as unknown as { campaignClient: unknown }).campaignClient = { getCampaign };
      return source;
    }

    it('reads the master from the campaign', async () => {
      const source = sourceWith(() => Promise.resolve({ campaign: { myRole: Role.MASTER } }));
      expect((await source.loadCatalog('camp-1')).viewerIsMaster).toBe(true);
    });

    it('fails the load when the role cannot be read, instead of showing a master the player view', async () => {
      const source = sourceWith(() => Promise.reject(new Error('transient')));
      await expect(source.loadCatalog('camp-1')).rejects.toThrow('transient');
    });

    it('takes a refusal of the campaign as "not the master"', async () => {
      const source = sourceWith(() =>
        Promise.reject(new ConnectError('no', Code.PermissionDenied)),
      );
      expect((await source.loadCatalog('camp-1')).viewerIsMaster).toBe(false);
    });
  });
});

describe('a basic sheet through the editor', () => {
  it('reads and writes initiative and attacks, with the damage type and the reach', () => {
    const basic: BasicSheet = {
      $typeName: 'meurpg.characters.v1.BasicSheet',
      monsterKey: '',
      combatOnly: false,
      size: CreatureSize.UNSPECIFIED,
      hitPointsMax: 7,
      armorClass: 15,
      speedFt: 30,
      attackBonus: 0,
      damage: '',
      description: 'Pequeno.',
      initiativeBonus: 2,
      challengeRating: '1/4',
      xpValue: 50,
      portraitImageId: '',
      attacks: [
        {
          $typeName: 'meurpg.characters.v1.BasicAttack',
          name: 'Arco curto',
          attackBonus: 4,
          damageDiceCount: 1,
          damageDiceSides: 6,
          damageBonus: 2,
          damageType: DamageType.PIERCING,
          rangeFt: 80,
        },
      ],
    };

    const form = toFormBasicSheet('Goblin', basic);
    expect(form.attacks[0].damageType).toBe('piercing');
    const init = toBasicSheetInit(form);
    expect(init.attacks[0]).toEqual({
      name: 'Arco curto',
      attackBonus: 4,
      damageDiceCount: 1,
      damageDiceSides: 6,
      damageBonus: 2,
      damageType: DamageType.PIERCING,
      rangeFt: 80,
    });
    expect(init.initiativeBonus).toBe(2);
  });

  it('carries the ND and the XP the minion gives through the form, so a save never wipes them (E7-11)', () => {
    const basic = {
      $typeName: 'meurpg.characters.v1.BasicSheet' as const,
      monsterKey: '',
      combatOnly: false,
      hitPointsMax: 7,
      armorClass: 15,
      speedFt: 30,
      attackBonus: 0,
      damage: '',
      description: '',
      initiativeBonus: 0,
      attacks: [],
      challengeRating: '1/4',
      xpValue: 50,
      portraitImageId: '6f1c2d3e-0000-4000-8000-000000000001',
      size: CreatureSize.UNSPECIFIED,
    };
    const form = toFormBasicSheet('Goblin', basic);
    expect(form).toMatchObject({ challengeRating: '1/4', xpValue: 50 });
    expect(toBasicSheetInit(form)).toMatchObject({ challengeRating: '1/4', xpValue: 50 });
    // The portrait (MR-031) survives the save too.
    expect(toBasicSheetInit(form).portraitImageId).toBe('6f1c2d3e-0000-4000-8000-000000000001');
    // The size (MR-034) survives the save too, though no control shows it yet.
    const large = toFormBasicSheet('Ogro', { ...basic, size: CreatureSize.LARGE });
    expect(toBasicSheetInit(large).size).toBe(CreatureSize.LARGE);

    // What the master typed beyond the table survives too.
    expect(toBasicSheetInit({ ...form, challengeRating: '1/2', xpValue: 70 })).toMatchObject({
      challengeRating: '1/2',
      xpValue: 70,
    });
    expect(toBasicSheetInit({ ...form, challengeRating: '', xpValue: 0 })).toMatchObject({
      challengeRating: '',
      xpValue: 0,
    });
  });

  it('sends the old damage text back unchanged while there are no attacks', () => {
    const form = toFormBasicSheet('Goblin', {
      $typeName: 'meurpg.characters.v1.BasicSheet',
      monsterKey: '',
      combatOnly: false,
      size: CreatureSize.UNSPECIFIED,
      hitPointsMax: 7,
      armorClass: 15,
      speedFt: 30,
      attackBonus: 3,
      damage: 'mordida venenosa',
      description: '',
      initiativeBonus: 0,
      attacks: [],
      challengeRating: '',
      xpValue: 0,
      portraitImageId: '',
    });
    expect(form.legacyDamage).toBe('mordida venenosa');
    expect(toBasicSheetInit(form)).toMatchObject({ damage: 'mordida venenosa', attackBonus: 3 });
  });
});
