import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { createRouterTransport } from '@connectrpc/connect';

import {
  CharacterSchema,
  CharacterService,
  FullSheetSchema,
  GetCharacterResponseSchema,
  UpdateCharacterResponseSchema,
  type UpdateCharacterRequest,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  AttackSchema,
  DerivedSheetSchema,
  GearEntrySchema,
  GearKind,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { GearClient } from '../../core/characters/gear';
import { CONNECT_TRANSPORT } from '../../core/connect/transport';
import { marksOf, sheetGear, weaponMarksOf } from './character-sheet-marks';
import { CharacterSheetSourceLive } from './character-sheet-source.live';

const gear = [
  create(GearEntrySchema, {
    key: 'equipment:dagger',
    namePt: 'Adaga',
    kind: GearKind.WEAPON,
    equipped: true,
    count: 1,
  }),
  create(GearEntrySchema, {
    key: 'equipment:mace',
    namePt: 'Maça',
    kind: GearKind.WEAPON,
    count: 1,
  }),
  create(GearEntrySchema, {
    key: 'equipment:shield',
    namePt: 'Escudo',
    kind: GearKind.SHIELD,
    equipped: true,
    count: 1,
  }),
];
const derived = create(DerivedSheetSchema, {
  gear,
  attacks: [create(AttackSchema, { key: 'equipment:dagger', silvered: true, magical: true })],
});
const full = create(FullSheetSchema, {
  weaponMarks: [
    { weaponKey: 'equipment:mace', adamantine: true },
    { weaponKey: 'equipment:dagger', silvered: false },
  ],
});

describe('the marks of the weapons on the sheet', () => {
  it('reads the three marks of an attack or of a mark', () => {
    expect(marksOf({ silvered: true, adamantine: false, magical: true })).toEqual({
      silvered: true,
      adamantine: false,
      magical: true,
    });
    expect(weaponMarksOf(full)).toEqual([
      { weaponKey: 'equipment:mace', silvered: false, adamantine: true, magical: false },
      { weaponKey: 'equipment:dagger', silvered: false, adamantine: false, magical: false },
    ]);
  });

  it('gives an equipped weapon the marks of its attack (the engine’s: the sheet’s or a table rule’s), a carried one the sheet’s mark, armor and shield none', () => {
    const rows = sheetGear(derived, full);
    expect(rows.find((g) => g.key === 'equipment:dagger')!.marks).toEqual({
      silvered: true,
      adamantine: false,
      magical: true,
    });
    expect(rows.find((g) => g.key === 'equipment:mace')!.marks).toEqual({
      silvered: false,
      adamantine: true,
      magical: false,
    });
    expect(rows.find((g) => g.key === 'equipment:shield')!.marks).toBeUndefined();
  });

  it('leaves a weapon with neither an attack nor a mark without marks', () => {
    const rows = sheetGear(derived, create(FullSheetSchema, {}));
    expect(rows.find((g) => g.key === 'equipment:mace')!.marks).toBeUndefined();
  });
});

describe('the combat’s read of the gear carries the marks too', () => {
  it('maps them the same way as the sheet', async () => {
    const transport = createRouterTransport(({ service }) => {
      service(CharacterService, {
        getCharacter: () =>
          create(GetCharacterResponseSchema, {
            character: create(CharacterSchema, {
              revision: 4,
              derived,
              sheet: { content: { case: 'full', value: full } },
            }),
          }),
      });
    });
    TestBed.configureTestingModule({
      providers: [{ provide: CONNECT_TRANSPORT, useValue: transport }],
    });
    const read = await TestBed.inject(GearClient).read('c1', 'ch1');
    expect(read.gear.map((g) => [g.key, g.marks])).toEqual([
      ['equipment:dagger', { silvered: true, adamantine: false, magical: true }],
      ['equipment:mace', { silvered: false, adamantine: true, magical: false }],
      ['equipment:shield', undefined],
    ]);
  });
});

describe('saving the marks goes over the whole sheet and its revision', () => {
  it('sends the full sheet with only the marks changed, the revision read and the name', async () => {
    const sent: UpdateCharacterRequest[] = [];
    const base = create(FullSheetSchema, {
      armorKey: 'equipment:chain',
      shield: true,
      equipment: [{ name: 'Corda', quantity: 1 }],
    });
    const transport = createRouterTransport(({ service }) => {
      service(CharacterService, {
        updateCharacter: (req) => {
          sent.push(req);
          return create(UpdateCharacterResponseSchema, {
            character: create(CharacterSchema, { id: 'ch1', revision: 8, name: 'Fulano' }),
          });
        },
      });
    });
    TestBed.configureTestingModule({
      providers: [{ provide: CONNECT_TRANSPORT, useValue: transport }, CharacterSheetSourceLive],
    });
    await TestBed.inject(CharacterSheetSourceLive).setWeaponMarks('c1', 'ch1', 7, 'Fulano', base, [
      { weaponKey: 'equipment:dagger', silvered: true, adamantine: false, magical: false },
    ]);
    expect(sent).toHaveLength(1);
    expect(sent[0].revision).toBe(7);
    expect(sent[0].name).toBe('Fulano');
    const content = sent[0].sheet!.content;
    expect(content.case).toBe('full');
    const value = content.value as typeof base;
    expect(value.armorKey).toBe('equipment:chain');
    expect(value.shield).toBe(true);
    expect(value.equipment.map((e) => e.name)).toEqual(['Corda']);
    expect(value.weaponMarks.map((m) => [m.weaponKey, m.silvered])).toEqual([
      ['equipment:dagger', true],
    ]);
    expect(base.weaponMarks).toEqual([]); // the sheet on screen is not changed by the call
  });
});
