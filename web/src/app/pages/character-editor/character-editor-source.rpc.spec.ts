import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  DiceMode,
  DicePreference,
  HitPointsRule,
  Role,
  XpMode,
} from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  AbilityMethod,
  AbilityOriginSchema,
  CharacterKind,
  CharacterSchema,
  CharacterState,
  FullSheetSchema,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { DerivedSheetSchema, SpellDetailsSchema } from '../../../gen/meurpg/rules/v1/rules_pb';
import { TableContentKind } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { CONNECT_TRANSPORT } from '../../core/connect/transport';
import type { CharacterFormValue } from './character-editor.types';
import { CharacterEditorSourceLive } from './character-editor-source.live';
import { toFormFullSheet } from './character-editor-sheet-map';

/** The source with its generated clients swapped for the server's answers a spec gives. */
interface Clients {
  characterClient?: unknown;
  contentClient?: unknown;
  campaignClient?: unknown;
  tableClient?: unknown;
}

function sourceWith(clients: Clients): CharacterEditorSourceLive {
  TestBed.resetTestingModule();
  TestBed.configureTestingModule({
    providers: [CharacterEditorSourceLive, { provide: CONNECT_TRANSPORT, useValue: {} }],
  });
  const source = TestBed.inject(CharacterEditorSourceLive);
  Object.assign(source as unknown as Clients, clients);
  return source;
}

const EMPTY_CONTENT = {
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
  levelXp: [],
};

describe('the catalog read, around the campaign and the feats', () => {
  const content = { listContent: () => Promise.resolve({ content: EMPTY_CONTENT }) };

  it('names the way the table earns XP, and leaves it out when the campaign says none', async () => {
    const read = async (xpMode: XpMode) =>
      (
        await sourceWith({
          contentClient: content,
          campaignClient: {
            getCampaign: () => Promise.resolve({ campaign: { myRole: Role.PLAYER, xpMode } }),
          },
        }).loadCatalog('camp')
      ).xpMode;
    expect(await read(XpMode.ENEMIES)).toBe('enemies');
    expect(await read(XpMode.GOLD)).toBe('gold');
    expect(await read(XpMode.MILESTONES)).toBe('milestones');
    expect(await read(XpMode.UNSPECIFIED)).toBeUndefined();
  });

  it('treats a refused or missing campaign as "not the master", and fails on any other error', async () => {
    const withError = (err: unknown) =>
      sourceWith({
        contentClient: content,
        campaignClient: { getCampaign: () => Promise.reject(err) },
      }).loadCatalog('camp');
    expect((await withError(new ConnectError('no', Code.PermissionDenied))).viewerIsMaster).toBe(
      false,
    );
    expect((await withError(new ConnectError('gone', Code.NotFound))).xpMode).toBeUndefined();
    await expect(withError(new ConnectError('down', Code.Unavailable))).rejects.toThrow('down');
    await expect(withError(new Error('boom'))).rejects.toThrow('boom');
  });

  it("reads the master's feats with the switches, and leaves them out when a read fails", async () => {
    const campaignClient = (rules: () => Promise<unknown>) => ({
      getCampaign: () => Promise.resolve({ campaign: { myRole: Role.MASTER } }),
      getTableRules: rules,
    });
    const switches = {
      listOptionSwitches: () =>
        Promise.resolve({
          options: [
            {
              key: 'feat:alert',
              namePt: 'Alerta',
              kind: TableContentKind.FEAT,
              archived: false,
              table: true,
              off: true,
            },
            {
              key: 'feat:old',
              namePt: 'Antigo',
              kind: TableContentKind.FEAT,
              archived: true,
              table: false,
              off: false,
            },
            {
              key: 'race:x',
              namePt: 'Raça',
              kind: TableContentKind.RACE,
              archived: false,
              table: false,
              off: false,
            },
          ],
        }),
    };
    const ok = await sourceWith({
      contentClient: content,
      tableClient: switches,
      campaignClient: campaignClient(() => Promise.resolve({ rules: { featsAllowed: true } })),
    }).loadCatalog('camp');
    expect(ok.viewerIsMaster).toBe(true);
    expect(ok.feats).toEqual({
      options: [{ key: 'feat:alert', namePt: 'Alerta', fromTable: true, off: true }],
      featsAllowed: true,
    });
    const noRules = await sourceWith({
      contentClient: content,
      tableClient: switches,
      campaignClient: campaignClient(() => Promise.resolve({})),
    }).loadCatalog('camp');
    expect(noRules.feats?.featsAllowed).toBe(false);
    const failed = await sourceWith({
      contentClient: content,
      tableClient: switches,
      campaignClient: campaignClient(() => Promise.reject(new Error('x'))),
    }).loadCatalog('camp');
    expect(failed.feats).toBeUndefined();
  });
});

describe('the character an edit opens', () => {
  function characterClient(character: ReturnType<typeof create<typeof CharacterSchema>>) {
    return { getCharacter: () => Promise.resolve({ character }) };
  }

  it('reads a full sheet: what the derived sheet adds, the origin of the scores and what the form edits', async () => {
    const full = create(FullSheetSchema, {
      cantripKeys: ['spell:a'],
      knownSpellKeys: ['spell:b'],
      preparedSpellKeys: ['spell:c'],
      abilityOrigin: create(AbilityOriginSchema, {
        method: AbilityMethod.ROLLED_4D6,
        rolls: [{ dice: [1, 2, 3, 4], total: 9 }],
        typed: true,
      }),
    });
    const character = create(CharacterSchema, {
      kind: CharacterKind.ENEMY,
      state: CharacterState.LOCKED,
      name: 'Orc',
      revision: 4,
      canEdit: false,
      reserved: true,
      sheet: { content: { case: 'full', value: full } },
      derived: create(DerivedSheetSchema, {
        spellcasting: [
          { classKey: 'class:wizard', preparedMax: 5 },
          { classKey: 'class:fighter', preparedMax: 0 },
        ],
        spells: [
          { spell: { key: 'spell:c', level: 1 }, prepared: true },
          { spell: { key: 'spell:granted', level: 2 }, prepared: true },
          { spell: { key: 'spell:cantrip', level: 0 }, prepared: true },
          { spell: { key: 'spell:unprepared', level: 1 }, prepared: false },
          { prepared: true },
        ],
        features: [{ key: 'feature:cutting-words' }],
      }),
    });
    const edit = await sourceWith({
      characterClient: characterClient(character),
    }).loadCharacterForEdit('camp', 'c1');
    expect(edit).toMatchObject({
      kind: 'enemy',
      revision: 4,
      blocked: 'sheet_locked',
      sheetLocked: true,
      reserved: true,
      preparedMax: { 'class:wizard': 5 },
      grantedSpellKeys: ['spell:granted'],
      featureKeys: ['feature:cutting-words'],
      abilityOrigin: {
        method: 'rolled_4d6',
        rolls: { sets: [{ dice: [1, 2, 3, 4], total: 9 }], typed: true, rolledAt: null },
      },
      basic: null,
    });
    expect(edit.full?.name).toBe('Orc');
  });

  it('says a dead character is blocked for good, and an editable one is not blocked', async () => {
    const dead = await sourceWith({
      characterClient: characterClient(
        create(CharacterSchema, { state: CharacterState.DEAD, canEdit: false }),
      ),
    }).loadCharacterForEdit('camp', 'c1');
    expect(dead).toMatchObject({ blocked: 'character_dead', sheetLocked: true, full: null });
    const open = await sourceWith({
      characterClient: characterClient(
        create(CharacterSchema, { state: CharacterState.DRAFT, canEdit: true }),
      ),
    }).loadCharacterForEdit('camp', 'c1');
    expect(open).toMatchObject({
      blocked: null,
      sheetLocked: false,
      preparedMax: {},
      grantedSpellKeys: [],
      featureKeys: [],
      abilityOrigin: null,
    });
  });

  it('keeps no rolls for an origin that was not rolled, and no origin for an unknown method', async () => {
    const origin = async (method: AbilityMethod) =>
      (
        await sourceWith({
          characterClient: characterClient(
            create(CharacterSchema, {
              sheet: {
                content: {
                  case: 'full',
                  value: create(FullSheetSchema, {
                    abilityOrigin: create(AbilityOriginSchema, { method }),
                  }),
                },
              },
            }),
          ),
        }).loadCharacterForEdit('camp', 'c1')
      ).abilityOrigin;
    expect(await origin(AbilityMethod.POINT_BUY)).toEqual({ method: 'point_buy', rolls: null });
    expect(await origin(AbilityMethod.UNSPECIFIED)).toBeNull();
  });

  it('reads a basic sheet', async () => {
    const edit = await sourceWith({
      characterClient: characterClient(
        create(CharacterSchema, {
          kind: CharacterKind.MINION,
          name: 'Goblin',
          sheet: { content: { case: 'basic', value: { hitPointsMax: 7 } } },
        }),
      ),
    }).loadCharacterForEdit('camp', 'c1');
    expect(edit.full).toBeNull();
    expect(edit.basic).toMatchObject({ name: 'Goblin', hitPointsMax: 7 });
  });
});

describe('the table rules for the scores', () => {
  const rules = (over: Record<string, unknown> = {}) => ({
    rules: {
      abilityMethods: { standardArray: true, pointBuy: false, rolled4d6: false, typed: true },
      hitPoints: HitPointsRule.PLAYER_CHOOSES,
    },
    standardArray: [15, 14],
    pointBuyCosts: [0, 1],
    pointBuyMinScore: 8,
    pointBuyBudget: 27,
    typedMinScore: 3,
    typedMaxScore: 18,
    ...over,
  });
  const table = (
    campaign: Record<string, unknown> | undefined,
    tableRules: unknown,
    characterClient: unknown = {},
  ) =>
    sourceWith({
      campaignClient: {
        getCampaign: () => Promise.resolve({ campaign }),
        getTableRules: () => Promise.resolve(tableRules),
      },
      characterClient,
    }).loadAbilityTable('camp');

  it('has no table for the master, or when the campaign cannot be read', async () => {
    expect(await table({ myRole: Role.MASTER }, rules())).toBeNull();
    expect(await table(undefined, rules())).toBeNull();
  });

  it("reads a player's table without asking for rolls when the table does not roll", async () => {
    const vm = await table(
      {
        myRole: Role.PLAYER,
        diceMode: DiceMode.PLAYERS_CHOOSE,
        myDicePreference: DicePreference.APP,
      },
      rules(),
    );
    expect(vm).toEqual({
      standardArray: true,
      pointBuy: false,
      rolled4d6: false,
      typed: true,
      standardValues: [15, 14],
      pointBuyCosts: [0, 1],
      pointBuyMinScore: 8,
      pointBuyBudget: 27,
      typedMin: 3,
      typedMax: 18,
      hitPoints: 'player_chooses',
      physicalDice: false,
      diceForced: false,
      rolls: null,
    });
  });

  it('asks for the stored rolls when the table rolls, and reads the dice and the hit points rules', async () => {
    const getAbilityRolls = vi.fn(() =>
      Promise.resolve({
        rolls: {
          sets: [{ dice: [6, 5, 4, 3], total: 15 }],
          typed: false,
          rolledAt: timestampFromDate(new Date(2026, 1, 2)),
        },
      }),
    );
    const vm = await table(
      { myRole: Role.PLAYER, diceMode: DiceMode.PHYSICAL, myDicePreference: DicePreference.APP },
      rules({
        rules: { abilityMethods: { rolled4d6: true }, hitPoints: HitPointsRule.ROLL },
      }),
      { getAbilityRolls },
    );
    expect(getAbilityRolls).toHaveBeenCalled();
    expect(vm).toMatchObject({
      hitPoints: 'roll',
      physicalDice: true,
      diceForced: true,
      rolled4d6: true,
      rolls: {
        sets: [{ dice: [6, 5, 4, 3], total: 15 }],
        typed: false,
        rolledAt: new Date(2026, 1, 2),
      },
    });
    const average = await table(
      { myRole: Role.PLAYER, diceMode: DiceMode.PLAYERS_CHOOSE },
      rules({ rules: undefined }),
    );
    expect(average).toMatchObject({
      hitPoints: 'player_chooses',
      standardArray: true,
      pointBuy: true,
      rolled4d6: true,
      typed: true,
    });
    const averaged = await table(
      { myRole: Role.PLAYER, diceMode: DiceMode.PLAYERS_CHOOSE },
      rules({ rules: { abilityMethods: { rolled4d6: false }, hitPoints: HitPointsRule.AVERAGE } }),
    );
    expect(averaged?.hitPoints).toBe('average');
  });
});

describe('the calls that carry the draft', () => {
  const formValue = (over: Partial<CharacterFormValue> = {}): CharacterFormValue => ({
    ...toFormFullSheet('Nome', create(FullSheetSchema)),
    ...over,
  });

  it('rolls the ability scores with the dice typed from a physical die', async () => {
    const rollAbilityScores = vi.fn(() =>
      Promise.resolve({ rolls: { sets: [{ dice: [1, 1, 1, 1], total: 3 }], typed: true } }),
    );
    const source = sourceWith({ characterClient: { rollAbilityScores } });
    const rolls = await source.rollAbilityScores('camp', [[1, 1, 1, 1]]);
    expect(rollAbilityScores).toHaveBeenCalledWith({
      campaignId: 'camp',
      typedDice: [{ dice: [1, 1, 1, 1] }],
    });
    expect(rolls).toEqual({
      sets: [{ dice: [1, 1, 1, 1], total: 3 }],
      typed: true,
      rolledAt: null,
    });
    await source.rollAbilityScores('camp');
    expect(rollAbilityScores).toHaveBeenLastCalledWith({ campaignId: 'camp', typedDice: [] });
  });

  it('previews the draft against the sheet loaded for the character, and reads what the server derives', async () => {
    const previewCharacter = vi.fn(() =>
      Promise.resolve({
        derived: create(DerivedSheetSchema, {
          hitPointsMax: 12,
          hitPointsFromEffects: 2,
          spellcasting: [
            { classKey: 'class:wizard', cantripsKnown: 3, spellsKnown: 0, preparedMax: 4 },
          ],
          issues: [
            { field: 'full.feat_keys[0]', message: 'falta o pré-requisito' },
            { field: 'full.feat_keys[5]', message: 'fora' },
            { field: 'full.armor_key', message: 'outra' },
          ],
        }),
      }),
    );
    const getCharacter = () =>
      Promise.resolve({
        character: create(CharacterSchema, {
          sheet: {
            content: { case: 'full', value: create(FullSheetSchema, { coins: { gold: 9 } }) },
          },
        }),
      });
    const source = sourceWith({ characterClient: { previewCharacter, getCharacter } });
    await source.loadCharacterForEdit('camp', 'c1');
    const vm = await source.previewCharacter({
      campaignId: 'camp',
      characterId: 'c1',
      kind: 'player',
      forPlayer: true,
      full: formValue({ featKeys: ['feat:alert'] }),
    });
    expect(vm).toEqual({
      hitPointsMax: 12,
      hitPointsFromEffects: 2,
      spellcasting: [
        { classKey: 'class:wizard', cantripsKnown: 3, spellsKnown: 0, preparedMax: 4 },
      ],
      featIssues: { 'feat:alert': 'falta o pré-requisito' },
    });
    const sent = (previewCharacter.mock.calls[0] as unknown[])[0] as {
      forPlayer?: boolean;
      sheet: { content: { value: { coins?: { gold: number } } } };
    };
    expect(sent.forPlayer).toBe(true);
    expect(sent.sheet.content.value.coins?.gold).toBe(9);

    const none = sourceWith({
      characterClient: { previewCharacter: () => Promise.resolve({}) },
    });
    expect(
      await none.previewCharacter({
        campaignId: 'camp',
        characterId: null,
        kind: 'player',
        full: formValue(),
      }),
    ).toEqual({ hitPointsMax: 0, hitPointsFromEffects: 0, spellcasting: [], featIssues: {} });
  });

  it('previews the choices of the draft', async () => {
    const previewChoices = vi.fn(() =>
      Promise.resolve({
        groups: [],
        done: 1,
        total: 2,
        notOffered: ['a'],
        spellSources: [],
        skillsAllowed: null,
        expertiseAllowed: null,
      }),
    );
    const vm = await sourceWith({ characterClient: { previewChoices } }).previewChoices({
      campaignId: 'camp',
      characterId: null,
      kind: 'player',
      full: formValue(),
    });
    expect(vm).toEqual({
      groups: [],
      done: 1,
      total: 2,
      notOffered: ['a'],
      spellSources: [],
      skillsAllowed: null,
      expertiseAllowed: null,
    });
  });

  it('carries the skills and the expertise the sources allow, and null while the server names none', async () => {
    const previewChoices = vi.fn(() =>
      Promise.resolve({
        groups: [],
        done: 0,
        total: 0,
        notOffered: [],
        spellSources: [],
        skillsAllowed: { any: false, keys: ['skill:arcana'] },
        expertiseAllowed: { any: false, keys: ['skill:arcana'] },
      }),
    );
    const vm = await sourceWith({ characterClient: { previewChoices } }).previewChoices({
      campaignId: 'camp',
      characterId: null,
      kind: 'player',
      full: formValue(),
    });
    expect(vm.skillsAllowed).toEqual({ any: false, keys: ['skill:arcana'] });
    expect(vm.expertiseAllowed).toEqual({ any: false, keys: ['skill:arcana'] });
  });

  it('creates a full sheet or a basic one, with the way the scores were made', async () => {
    const createCharacter = vi.fn(() => Promise.resolve({ character: { id: 'new' } }));
    const source = sourceWith({ characterClient: { createCharacter } });
    const full = await source.createCharacter({
      campaignId: 'camp',
      kind: 'player',
      full: formValue(),
      basic: null,
      abilityMethod: 'point_buy',
      forPlayer: true,
      idempotencyKey: 'k1',
    });
    expect(full).toEqual({ characterId: 'new' });
    const fullCall = (createCharacter.mock.calls[0] as unknown[])[0] as Record<string, unknown>;
    expect(fullCall).toMatchObject({
      name: 'Nome',
      forPlayer: true,
      abilityMethod: AbilityMethod.POINT_BUY,
      idempotencyKey: 'k1',
      kind: CharacterKind.PLAYER,
    });
    await source.createCharacter({
      campaignId: 'camp',
      kind: 'minion',
      full: null,
      basic: {
        name: 'Goblin',
        hitPointsMax: 7,
        armorClass: 13,
        speedFt: 30,
        initiativeBonus: 2,
        attacks: [],
        legacyDamage: '',
        legacyAttackBonus: 0,
        description: '',
        challengeRating: '',
        xpValue: 0,
        portraitImageId: '',
        size: 0,
      },
      idempotencyKey: 'k2',
    });
    const basicCall = (createCharacter.mock.calls[1] as unknown[])[0] as Record<string, unknown>;
    expect(basicCall).toMatchObject({ name: 'Goblin', kind: CharacterKind.MINION });
    expect(basicCall).not.toHaveProperty('forPlayer');
    expect(basicCall).not.toHaveProperty('abilityMethod');
    expect(basicCall['sheet']).toMatchObject({ content: { case: 'basic' } });
  });

  it('updates a sheet and keeps the answer as the base of the next save', async () => {
    const answer = create(CharacterSchema, {
      revision: 6,
      sheet: { content: { case: 'full', value: create(FullSheetSchema, { coins: { gold: 1 } }) } },
    });
    const updateCharacter = vi.fn(() => Promise.resolve({ character: answer }));
    const source = sourceWith({ characterClient: { updateCharacter } });
    const first = await source.updateCharacter({
      campaignId: 'camp',
      characterId: 'c1',
      revision: 5,
      name: 'Nome',
      full: formValue(),
      basic: null,
    });
    expect(first).toEqual({ revision: 6 });
    await source.updateCharacter({
      campaignId: 'camp',
      characterId: 'c1',
      revision: 6,
      name: 'Nome',
      full: formValue(),
      basic: null,
    });
    const second = (updateCharacter.mock.calls[1] as unknown[])[0] as {
      sheet: { content: { value: { coins?: { gold: number } } } };
    };
    expect(second.sheet.content.value.coins?.gold).toBe(1);
    // A basic answer replaces nothing and a basic save sends the basic sheet.
    const basicSource = sourceWith({
      characterClient: {
        updateCharacter: () =>
          Promise.resolve({
            character: create(CharacterSchema, {
              revision: 2,
              sheet: { content: { case: 'basic', value: {} } },
            }),
          }),
      },
    });
    expect(
      await basicSource.updateCharacter({
        campaignId: 'camp',
        characterId: 'c2',
        revision: 1,
        name: 'Goblin',
        full: null,
        basic: {
          name: 'Goblin',
          hitPointsMax: 1,
          armorClass: 1,
          speedFt: 1,
          initiativeBonus: 0,
          attacks: [],
          legacyDamage: '',
          legacyAttackBonus: 0,
          description: '',
          challengeRating: '',
          xpValue: 0,
          portraitImageId: '',
          size: 0,
        },
      }),
    ).toEqual({ revision: 2 });
  });

  it('reads the details of a spell', async () => {
    const getSpellDetails = vi.fn(() =>
      Promise.resolve({
        spell: create(SpellDetailsSchema, { spell: { key: 'spell:a', namePt: 'A' } }),
      }),
    );
    const vm = await sourceWith({ contentClient: { getSpellDetails } }).loadSpellDetails(
      'camp',
      'spell:a',
    );
    expect(getSpellDetails).toHaveBeenCalledWith({ campaignId: 'camp', spellKey: 'spell:a' });
    expect(vm.key).toBe('spell:a');
  });
});

describe('a sheet the form reads with parts missing', () => {
  it('falls back to the defaults of an empty message', () => {
    const form = toFormFullSheet('Sem nada', create(FullSheetSchema));
    expect(form).toMatchObject({
      name: 'Sem nada',
      className: '',
      level: 1,
      subclassName: '',
      customSubclassName: '',
      background: '',
      customBackgroundName: '',
      customBackgroundSkills: null,
      customBackgroundProficiencies: [],
      customBackgroundFeatureName: '',
      customBackgroundFeatureText: '',
      customBackgroundEquipment: '',
      extraClasses: [],
      abilities: { str: 10, dex: 10, con: 10, int: 10, wis: 10, cha: 10 },
      extraAbilityBonuses: { str: 0, dex: 0, con: 0, int: 0, wis: 0, cha: 0 },
      hitPointsMethod: 'average',
      hitPointsRolls: [],
      alignment: '',
      cuttingWordsAsk: 'only-attacks',
    });
  });

  it('reads a custom background with one skill as no pair, and a class with a custom subclass', () => {
    const form = toFormFullSheet(
      'Custom',
      create(FullSheetSchema, {
        background: {
          case: 'customBackground',
          value: {
            name: 'Eremita',
            skillKeys: ['skill:a'],
            proficiencyKeys: ['p'],
            equipment: 'e',
          },
        },
        classes: [
          { classKey: 'class:a', level: 2, subclass: { case: 'customSubclassName', value: 'Meu' } },
          { classKey: 'class:b', level: 1, subclass: { case: 'subclassKey', value: 'sub:b' } },
        ],
      }),
    );
    expect(form).toMatchObject({
      background: 'custom',
      customBackgroundName: 'Eremita',
      customBackgroundSkills: null,
      customBackgroundProficiencies: ['p'],
      customBackgroundEquipment: 'e',
      className: 'class:a',
      subclassName: '',
      customSubclassName: 'Meu',
      extraClasses: [
        { classKey: 'class:b', level: 1, subclassKey: 'sub:b', customSubclassName: '' },
      ],
    });
  });
});
