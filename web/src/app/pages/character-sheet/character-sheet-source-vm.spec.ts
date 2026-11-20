import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';

import {
  CharacterKind,
  CharacterSchema,
  CharacterState,
  FullSheetSchema,
  LevelUpReason,
  Review,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { DerivedSheetSchema } from '../../../gen/meurpg/rules/v1/rules_pb';
import { FullSheetVm } from './character-sheet.types';
import { toCharacterSheetVm } from './character-sheet-map';

/** The whole mapping on a sheet that has everything, and on one that has nothing but its required fields:
 * every "?? ''" and "? x : y" of `toCharacterSheetVm` is read on both sides. */
describe('toCharacterSheetVm on a bare and on a dense character', () => {
  const when = new Date(2026, 5, 1, 10, 30);

  function dense() {
    return create(CharacterSchema, {
      id: 'c1',
      campaignId: 'camp',
      kind: CharacterKind.BOSS,
      state: CharacterState.LOCKED,
      name: 'Fulano',
      playerDisplayName: 'Jogador',
      sheet: {
        content: {
          case: 'full',
          value: create(FullSheetSchema, {
            portraitImageId: 'img-1',
            armorKey: 'equipment:chain',
            shield: true,
            equipment: [
              { name: 'Corda', quantity: 0 },
              { name: 'Tocha', quantity: 3 },
            ],
            ammunition: [
              { key: 'equipment:arrow', namePt: 'Flechas', quantity: 15, spentLastCombat: 6 },
            ],
            coins: { copper: 1, silver: 2, electrum: 3, gold: 4, platinum: 5 },
            experiencePoints: 900,
            challengeRating: '2',
            xpValue: 450,
            customFeaturesText: 'texto livre',
          }),
        },
      },
      derived: create(DerivedSheetSchema, {
        raceNamePt: 'Anão',
        subraceNamePt: 'Anão da Montanha',
        backgroundNamePt: 'Soldado',
        classes: [
          { namePt: 'Guerreiro', level: 3 },
          { namePt: 'Mago', level: 2 },
        ],
        totalLevel: 5,
        nextLevelXp: 6500,
        breathWeapon: { textPt: 'Sopro de fogo' },
        pactMagic: { slotLevel: 2, count: 2 },
        itemModifiers: [
          {
            target: 'ac',
            value: 1,
            sourceKey: 'equipment:shield',
            sourceNamePt: 'Escudo',
            counts: false,
            reasonPt: 'Parado.',
          },
        ],
        proficiencies: { armor: ['Leve'], weapons: ['Simples'], tools: ['Ferreiro'] },
        spells: [
          {
            spell: { key: 'spell:light', namePt: 'Luz', level: 0, ritual: true, reaction: true },
            prepared: true,
          },
          { prepared: false },
        ],
        changedContent: [
          {
            key: 'k',
            namePt: 'Mudou',
            changedAt: timestampFromDate(when),
            messages: ['a'],
            codes: ['skill_count'],
          },
          { key: 'k2', namePt: 'Sem data', messages: [], codes: [] },
        ],
        features: [{ key: 'feature:wild-shape', namePt: 'Forma Selvagem' }],
      }),
      sheetLockedAt: timestampFromDate(when),
      diedAt: timestampFromDate(when),
      revivedAt: timestampFromDate(when),
      levelUpReason: LevelUpReason.MILESTONE,
      canLevelUp: true,
      review: { status: Review.AWAITING, reason: 'r', requestedAt: timestampFromDate(when) },
      reserved: true,
    });
  }

  it('reads every optional part a dense full sheet carries', () => {
    const vm = toCharacterSheetVm(dense());
    const sheet = vm.sheet as FullSheetVm;
    expect(vm).toMatchObject({
      characterKind: 'boss',
      state: 'locked',
      portraitImageId: 'img-1',
      playerDisplayName: 'Jogador',
      raceLabel: 'Anão da Montanha',
      classSummary: 'Guerreiro 3 / Mago 2',
      backgroundLabel: 'Soldado',
      experiencePoints: 900,
      totalLevel: 5,
      nextLevelXp: 6500,
      levelUpReason: 'milestone',
      challengeRating: '2',
      xpValue: 450,
      reserved: true,
    });
    expect(vm.sheetLockedAt).toEqual(when);
    expect(vm.diedAt).toEqual(when);
    expect(vm.revivedAt).toEqual(when);
    expect(vm.review).toMatchObject({ status: 'awaiting', reason: 'r', resubmittedAt: null });
    expect(sheet.coins).toEqual({ cp: 1, sp: 2, ep: 3, gp: 4, pp: 5 });
    expect(sheet.equipment).toEqual([
      { name: 'Corda', quantity: 1 },
      { name: 'Tocha', quantity: 3 },
    ]);
    expect(sheet.ammunition).toEqual([
      {
        key: 'equipment:arrow',
        namePt: 'Flechas',
        quantity: 15,
        spent: 6,
        back: 3,
        recovered: false,
      },
    ]);
    expect(sheet.proficiencies).toEqual(['Leve', 'Simples', 'Ferreiro']);
    expect(sheet.pactSlots).toEqual({ level: 2, count: 2 });
    expect(sheet.breathWeapon).toBe('Sopro de fogo');
    expect(sheet.hasWildShape).toBe(true);
    expect(sheet.itemModifiers).toEqual([
      {
        target: 'ac',
        value: 1,
        sourceKey: 'equipment:shield',
        sourceNamePt: 'Escudo',
        counts: false,
        reasonPt: 'Parado.',
      },
    ]);
    expect(sheet.spells).toEqual([
      {
        key: 'spell:light',
        namePt: 'Luz',
        level: 0,
        prepared: true,
        ritual: true,
        concentration: false,
        reaction: true,
      },
      {
        key: '',
        namePt: '',
        level: 0,
        prepared: false,
        ritual: false,
        concentration: false,
        reaction: false,
      },
    ]);
    expect(sheet.changedContent.map((c) => c.changedAt)).toEqual([when, null]);
    expect(sheet.changedContent.map((c) => c.codes)).toEqual([['skill_count'], []]);
  });

  it('falls back to empty texts, zeroes and nulls on a bare full sheet', () => {
    const bare = create(CharacterSchema, {
      sheet: { content: { case: 'full', value: create(FullSheetSchema) } },
      derived: create(DerivedSheetSchema),
    });
    const vm = toCharacterSheetVm(bare);
    const sheet = vm.sheet as FullSheetVm;
    expect(vm).toMatchObject({
      characterKind: 'player',
      state: 'draft',
      portraitImageId: '',
      playerDisplayName: null,
      raceLabel: '',
      classSummary: '',
      backgroundLabel: '',
      experiencePoints: 0,
      totalLevel: 0,
      levelUpReason: null,
      challengeRating: '',
      xpValue: 0,
      sheetLockedAt: null,
      diedAt: null,
      revivedAt: null,
      review: null,
    });
    expect(sheet.coins).toEqual({ cp: 0, sp: 0, ep: 0, gp: 0, pp: 0 });
    expect(sheet.pactSlots).toBeNull();
    expect(sheet.breathWeapon).toBe('');
    expect(sheet.proficiencies).toEqual([]);
    expect(sheet.hasWildShape).toBe(false);
    expect(sheet.spellSlots).toEqual([]);
  });

  it('shows an empty basic sheet for a character with no sheet at all, and for a full one with no derived sheet', () => {
    const none = toCharacterSheetVm(create(CharacterSchema));
    expect(none.sheet).toMatchObject({ kind: 'basic', hitPointsMax: 0, attacks: [] });
    expect(none).toMatchObject({
      portraitImageId: '',
      alignmentLabel: '',
      experiencePoints: null,
      challengeRating: '',
      xpValue: 0,
      raceLabel: '',
      classSummary: '',
    });
    const noDerived = toCharacterSheetVm(
      create(CharacterSchema, {
        sheet: { content: { case: 'full', value: create(FullSheetSchema) } },
      }),
    );
    expect(noDerived.sheet.kind).toBe('basic');
    expect(noDerived.experiencePoints).toBe(0);
  });

  it('keeps the legacy damage only when a basic sheet lists no attacks, and its rating and XP', () => {
    const basic = (attacks: { name: string }[]) =>
      toCharacterSheetVm(
        create(CharacterSchema, {
          sheet: {
            content: {
              case: 'basic',
              value: { damage: '1d4', challengeRating: '1/2', xpValue: 100, attacks },
            },
          },
        }),
      );
    const without = basic([]);
    expect(without.sheet).toMatchObject({ kind: 'basic', legacyDamage: '1d4' });
    expect(without.challengeRating).toBe('1/2');
    expect(without.xpValue).toBe(100);
    expect(basic([{ name: 'Garra' }]).sheet).toMatchObject({ legacyDamage: '' });
  });
});
