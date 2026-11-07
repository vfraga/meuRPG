import { type MessageInitShape, create } from '@bufbuild/protobuf';

import { DiceMode, DicePreference } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  SummonOptionSchema,
  SummonSlotSchema,
  SummonSpellOptionsSchema,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { CombatantSchema } from '../../../gen/meurpg/play/v1/combat_pb';
import { CreatureActionSchema, CreatureSchema } from '../../../gen/meurpg/rules/v1/rules_pb';
import type { SlotRow } from '../../core/combat/cast-flow';
import {
  concentrationLineOf,
  costLineOf,
  creatureKeysOf,
  diceFlagsOf,
  joinDotsOf,
  listLabelOf,
  missingOf,
  optionLabel,
  replacesNoteOf,
  resultLineOf,
  resultTitleOf,
  rowsOf,
  setupLineOf,
  slotId,
  slotRow,
  slotRowsOf,
  subtitleOf,
} from './summon-views';

/** The words keep a non-breaking space between a number and its noun; the specs read them as plain spaces. */
const plain = (text: string) => text.replace(/ /g, ' ');

const spell = (over: MessageInitShape<typeof SummonSpellOptionsSchema> = {}) =>
  create(SummonSpellOptionsSchema, {
    spellKey: 'spell:conjure-animals',
    namePt: 'Conjurar Animais',
    level: 3,
    castingTimePt: '1 ação',
    concentration: true,
    circles: [{ circle: 3 }, { circle: 5 }],
    ...over,
  });

const row = (over: Partial<SlotRow> = {}): SlotRow => ({
  level: 3,
  pact: false,
  free: 2,
  total: 3,
  used: 1,
  enabled: true,
  title: '3º nível',
  count: '2 livres de 3',
  ...over,
});

describe('summon views: the slots', () => {
  it('names a slot by its circle, with the pact apart', () => {
    expect(slotId(row())).toBe('3');
    expect(slotId(row({ pact: true }))).toBe('3p');
  });

  it('turns a server slot into a row: used, enabled, titled, counted', () => {
    const r = slotRow(create(SummonSlotSchema, { level: 3, pact: true, total: 2, free: 0 }));
    expect(r).toMatchObject({ level: 3, pact: true, used: 2, enabled: false });
    expect(plain(r.title)).toBe('3º nível (pacto)');
    expect(plain(slotRow(create(SummonSlotSchema, { level: 4, total: 1, free: 1 })).title)).toBe(
      '4º nível',
    );
  });

  it('lists the slots from the spell circle up, only the circles the server names, with slots', () => {
    const slots = [
      create(SummonSlotSchema, { level: 2, total: 4, free: 4 }),
      create(SummonSlotSchema, { level: 3, total: 2, free: 1 }),
      create(SummonSlotSchema, { level: 4, total: 1, free: 1 }),
      create(SummonSlotSchema, { level: 5, total: 0, free: 0 }),
      create(SummonSlotSchema, { level: 5, pact: true, total: 1, free: 1 }),
    ];
    expect(slotRowsOf(spell(), slots).map(slotId)).toEqual(['3', '5p']);
    expect(slotRowsOf(null, slots)).toEqual([]);
  });
});

describe('summon views: the lines of the sheet', () => {
  it('joins only the parts that are there', () => {
    expect(joinDotsOf(['a', '', 'b'])).toBe('a · b');
    expect(joinDotsOf(['', ''])).toBe('');
  });

  it('says the spell before the cast: ritual, casting time, concentration only in a combat', () => {
    const base = { spell: spell(), done: false, inCombat: false, ritual: false, slot: null };
    expect(plain(subtitleOf(base))).toBe('Magia de 3º nível · 1 ação');
    expect(plain(subtitleOf({ ...base, ritual: true, inCombat: true }))).toBe(
      'Magia de 3º nível · ritual · 1 ação · concentração',
    );
    expect(
      plain(subtitleOf({ ...base, inCombat: true, spell: spell({ concentration: false }) })),
    ).toBe('Magia de 3º nível · 1 ação');
    expect(subtitleOf({ ...base, spell: null })).toBe('');
  });

  it('says the spell after the cast: its name, the circle and the concentration', () => {
    const done = { spell: spell(), done: true, inCombat: true, ritual: false };
    expect(plain(subtitleOf({ ...done, slot: row() }))).toBe(
      'Conjurar Animais · 3º nível · concentração',
    );
    expect(plain(subtitleOf({ ...done, slot: null, spell: spell({ concentration: false }) }))).toBe(
      'Conjurar Animais',
    );
    expect(subtitleOf({ ...done, spell: null, slot: null })).toBe('');
  });

  it('says what losing the concentration does, by the kinds chosen so far', () => {
    const input = { spell: spell(), count: 2, kindTitles: [] as string[], concentrating: '' };
    expect(plain(concentrationLineOf(input).text)).toBe(
      'Se você perder a concentração, as 2 criaturas somem.',
    );
    expect(concentrationLineOf(input)).toMatchObject({ lead: 'Concentração.', ends: '' });
    expect(plain(concentrationLineOf({ ...input, count: 1 }).text)).toBe(
      'Se você perder a concentração, a criatura some.',
    );
    expect(plain(concentrationLineOf({ ...input, kindTitles: ['Lobo atroz'] }).text)).toBe(
      'Se você perder a concentração, os 2 Lobos atrozes somem.',
    );
    expect(plain(concentrationLineOf({ ...input, kindTitles: ['Águia gigante'] }).text)).toContain(
      'as 2 ',
    );
    expect(plain(concentrationLineOf({ ...input, kindTitles: ['Lobo', 'Gato'] }).text)).toContain(
      'as 2 criaturas',
    );
    expect(plain(concentrationLineOf({ ...input, concentrating: 'Teia' }).ends)).toBe(
      'Conjurar Animais encerra a concentração em Teia.',
    );
  });

  it('says the slot and the quantity folded into one line', () => {
    const option = create(SummonOptionSchema, { count: 2, type: 'beast', maxCr: '1' });
    expect(plain(setupLineOf(row(), option))).toBe('3º nível · 2 feras de ND 1 ou menos');
    expect(plain(setupLineOf(row({ pact: true }), null))).toBe('3º nível (pacto)');
    expect(plain(setupLineOf(null, create(SummonOptionSchema, { count: 1 })))).toBe('1 criatura');
    expect(setupLineOf(null, null)).toBe('');
  });

  it('labels an option: beasts, or any other kind', () => {
    expect(
      plain(optionLabel(create(SummonOptionSchema, { count: 1, type: 'beast', maxCr: '1/2' }))),
    ).toBe('1 fera de ND 1/2 ou menos');
    expect(
      plain(optionLabel(create(SummonOptionSchema, { count: 3, type: 'undead', maxCr: '1' }))),
    ).toBe('3 criaturas de ND 1 ou menos');
  });

  it('notes what a casting replaces, only with a spell', () => {
    expect(replacesNoteOf(null)).toBe('');
    expect(replacesNoteOf(spell())).toBe('');
  });

  it('labels the list by the spell and the quantity', () => {
    expect(listLabelOf(4, true, false, 0, 1)).toBe('Forma · 4 do livro');
    expect(listLabelOf(4, false, true, 1, 3)).toBe('Criaturas · 1 de 3');
    expect(listLabelOf(4, false, false, 0, 1)).toBe('Criatura');
  });

  it('says what the cast still needs, in one sentence', () => {
    const ready = {
      ritual: false,
      hasSlot: true,
      familiar: false,
      name: '',
      hasOption: true,
      several: false,
      count: 1,
      total: 1,
    };
    expect(missingOf(ready)).toBe('');
    expect(missingOf({ ...ready, hasSlot: false })).toBe('Escolha o espaço de magia.');
    expect(missingOf({ ...ready, ritual: true, hasSlot: false })).toBe('');
    expect(missingOf({ ...ready, total: 0 })).toBe('Escolha a criatura.');
    expect(plain(missingOf({ ...ready, total: 0, several: true, count: 3 }))).toBe(
      'Escolha mais 3 criaturas.',
    );
    expect(missingOf({ ...ready, familiar: true, name: ' ', total: 0 })).toBe(
      'Escolha a forma e dê um nome ao familiar.',
    );
    expect(plain(missingOf({ ...ready, total: 3, count: 1 }))).toBe('Tire 2 criaturas.');
    expect(
      plain(missingOf({ ...ready, hasSlot: false, total: 0, hasOption: false, count: 2 })),
    ).toBe('Escolha o espaço de magia.');
    expect(plain(missingOf({ ...ready, hasSlot: false, familiar: true, name: '', total: 0 }))).toBe(
      'Escolha o espaço de magia, escolha a forma e dê um nome ao familiar.',
    );
  });

  it('says what the cast costs: a ritual, or the slot', () => {
    expect(costLineOf(null, false, null, 1)).toBe('');
    expect(plain(costLineOf(spell({ castingTimePt: '1 hora' }), true, null, 1))).toBe(
      'Conjurar como ritual · 1 hora · sem gastar espaço',
    );
    expect(plain(costLineOf(spell(), false, row(), 1))).toBe(
      '1 ação · gasta um espaço de 3º nível',
    );
    expect(plain(costLineOf(spell(), false, row({ pact: true }), 3))).toBe(
      '3 criaturas · 1 ação · gasta um espaço de 3º nível (pacto)',
    );
    expect(plain(costLineOf(spell(), false, null, 1))).toBe('1 ação · gasta um espaço de magia');
    // A feature's summon spends a use and shows how many remain, with no slot in it.
    const feature = spell({
      grantedBy: 'feature:chamado@mesa',
      resourceKey: 'chamados',
      usesLeft: 2,
    });
    // The subtitle says the time and the use; the line says what it adds.
    expect(plain(costLineOf(feature, false, null, 2))).toBe('2 criaturas');
    expect(costLineOf(feature, false, null, 1)).toBe('');
    expect(
      plain(
        subtitleOf({
          spell: spell({ ...feature, grantedByLabelPt: 'Druida 3', usesLeft: 1 }),
          done: false,
          inCombat: false,
          ritual: false,
          slot: null,
        }),
      ),
    ).toBe('Druida 3 · 1 ação · gasta 1 uso (resta 1)');
    expect(
      plain(
        subtitleOf({ spell: feature, done: false, inCombat: false, ritual: false, slot: null }),
      ),
    ).toBe('Habilidade · 1 ação · gasta 1 uso (restam 2)');
  });
});

describe('summon views: the list, the keys and the dice', () => {
  const choice = (key: string, title: string) => ({ key, title, subtitle: '' });

  it('lists nothing without an option, the forms by name, and the beasts the search found', () => {
    const none = { option: null, beasts: null, beastBlocks: new Map(), forms: null };
    expect(rowsOf(none)).toEqual([]);
    const forms = create(SummonOptionSchema, {
      forms: [
        { monsterKey: 'monster:wolf', namePt: 'Lobo' },
        { monsterKey: 'monster:cat', namePt: 'Gato' },
      ],
    });
    expect(rowsOf({ ...none, option: forms }).map((r) => [r.key, r.title, r.subtitle])).toEqual([
      ['monster:cat', 'Gato', ''],
      ['monster:wolf', 'Lobo', ''],
    ]);
    const beasts = create(SummonOptionSchema, { type: 'beast', maxCr: '1' });
    expect(rowsOf({ ...none, option: beasts })).toEqual([]);
    expect(
      rowsOf({
        ...none,
        option: beasts,
        beasts: [{ key: 'monster:bat', namePt: 'Morcego' } as never],
      }).map((r) => [r.key, r.title, r.attacks]),
    ).toEqual([['monster:bat', 'Morcego', undefined]]);
  });

  it("a table feature's list puts the table's creature first and tells a book creature as a beast row does (ND, CA, PV, attacks)", () => {
    const forms = create(SummonOptionSchema, {
      forms: [
        { monsterKey: 'monster:wolf', namePt: 'Lobo' },
        { monsterKey: 'creature:lobo-do-vale@mesa', namePt: 'Lobo do Vale' },
      ],
    });
    const wolf = create(CreatureSchema, {
      summary: { key: 'monster:wolf', namePt: 'Lobo', sizePt: 'Médio', challengeRating: '1/4' },
      armorClass: 13,
      hitPoints: 11,
      speedWalkFt: 40,
      actions: [
        create(CreatureActionSchema, {
          name: 'Bite',
          namePt: 'Mordida',
          hasAttack: true,
          attackBonus: 4,
          damage: [{ dice: '2d4+2', damageTypePt: 'perfurante' }],
        }),
      ],
    });
    const rows = rowsOf({
      option: forms,
      beasts: null,
      beastBlocks: new Map(),
      forms: [wolf],
      bring: true,
    });
    expect(rows.map((r) => [r.title, r.table])).toEqual([
      ['Lobo do Vale', true],
      ['Lobo', false],
    ]);
    expect(plain(rows[1].subtitle)).toBe('Médio · ND 1/4 · 12 m · CA 13 · PV 11');
    expect(rows[1].attacks?.[0]).toMatchObject({ name: 'Mordida' });
  });

  it('marks the table creatures by the "@mesa" suffix, in the forms and in the beasts', () => {
    const none = { option: null, beasts: null, beastBlocks: new Map(), forms: null };
    const forms = create(SummonOptionSchema, {
      forms: [
        { monsterKey: 'creature:lobo-do-vale@mesa', namePt: 'Lobo do Vale' },
        { monsterKey: 'monster:wolf', namePt: 'Lobo' },
      ],
    });
    expect(rowsOf({ ...none, option: forms }).map((r) => [r.title, r.table])).toEqual([
      ['Lobo', false],
      ['Lobo do Vale', true],
    ]);
    const beasts = create(SummonOptionSchema, { type: 'beast', maxCr: '1' });
    expect(
      rowsOf({
        ...none,
        option: beasts,
        beasts: [{ key: 'creature:x@mesa', namePt: 'X' } as never],
      }).map((r) => r.table),
    ).toEqual([true]);
  });

  it("gives a table creature no attacks of the base's block: its numbers are the owner's, and the row says so", () => {
    const block = { actions: [{ hasAttack: true, namePt: 'Mordida', damage: [] }] } as never;
    const option = create(SummonOptionSchema, { type: 'beast', maxCr: '1' });
    const rows = rowsOf({
      option,
      forms: null,
      beasts: [
        { key: 'creature:x@mesa', namePt: 'X' },
        { key: 'monster:bat', namePt: 'Morcego' },
      ] as never,
      beastBlocks: new Map([
        ['creature:x@mesa', block],
        ['monster:bat', block],
      ]),
    });
    expect(rows.map((r) => [r.key, r.attacks === undefined])).toEqual([
      ['creature:x@mesa', true],
      ['monster:bat', false],
    ]);
  });

  it('repeats each kind as many times as it was chosen, in the list order', () => {
    const rows = [choice('a', 'A'), choice('b', 'B'), choice('c', 'C')];
    expect(creatureKeysOf(rows, { c: 1, a: 2 })).toEqual(['a', 'a', 'c']);
    expect(creatureKeysOf(rows, {})).toEqual([]);
  });

  it('reads how the initiative d20 is made: by the app, typed, or either', () => {
    expect(diceFlagsOf(undefined)).toEqual({ canApp: true, canType: false, typing: false });
    expect(diceFlagsOf({ diceMode: DiceMode.APP, preference: DicePreference.APP })).toEqual({
      canApp: true,
      canType: false,
      typing: false,
    });
    expect(diceFlagsOf({ diceMode: DiceMode.PHYSICAL, preference: DicePreference.APP })).toEqual({
      canApp: false,
      canType: true,
      typing: true,
    });
    const either = DiceMode.PLAYERS_CHOOSE;
    expect(diceFlagsOf({ diceMode: either, preference: DicePreference.PHYSICAL }).typing).toBe(
      true,
    );
    expect(diceFlagsOf({ diceMode: either, preference: DicePreference.APP }).typing).toBe(false);
  });
});

describe('summon views: the result of a casting in a combat', () => {
  const creature = (over: MessageInitShape<typeof CombatantSchema> = {}) =>
    create(CombatantSchema, {
      monsterKey: 'monster:wolf',
      monsterNamePt: 'Lobo atroz',
      label: 'Lobo atroz 1',
      ...over,
    });

  it('titles the group in its gender: one creature, a kind, or a mix', () => {
    expect(resultTitleOf([creature()])).toBe('Lobo atroz 1 conjurado');
    expect(resultTitleOf([creature({ label: 'Águia 1', monsterNamePt: 'Águia' })])).toBe(
      'Águia 1 conjurada',
    );
    expect(resultTitleOf([creature(), creature({ label: 'Lobo atroz 2' })])).toBe(
      'Lobos atrozes conjurados',
    );
    expect(resultTitleOf([creature(), creature({ monsterKey: 'monster:cat' })])).toBe(
      'Criaturas conjuradas',
    );
    expect(resultTitleOf([creature({ monsterNamePt: '' }), creature({ monsterNamePt: '' })])).toBe(
      'Criaturas conjurados',
    );
  });

  it('writes the line of a creature: armor class, hit points, speed', () => {
    const c = creature({ hitPointsCurrent: 30, hitPointsMax: 37, speedFt: 50 });
    expect(plain(resultLineOf(c, 14))).toBe('CA 14 · PV 30 de 37 · 15 m');
    expect(plain(resultLineOf(c, undefined))).toBe('PV 30 de 37 · 15 m');
    expect(plain(resultLineOf(creature({ speedFt: 50 }), 14))).toBe('CA 14 · 15 m');
  });
});
