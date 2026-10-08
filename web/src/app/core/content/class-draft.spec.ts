import { create } from '@bufbuild/protobuf';

import { Ability } from '../../../gen/meurpg/rules/v1/rules_pb';
import {
  TableClassSchema,
  TableSubclassSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import {
  type ClassDraft,
  castingOfKind,
  classFeatureBase,
  classFeaturePaths,
  classToDraft,
  draftToClass,
  draftToSubclass,
  emptyCasting,
  emptyClass,
  emptySubclass,
  flattenAlwaysPrepared,
  gridColumns,
  newLevelFeatureId,
  rowsEdited,
  rowsOfTable,
  sameRows,
  slotsText,
  sortedFeatures,
  subclassFeatureBase,
  subclassLevels,
  subclassRowBase,
  subclassToDraft,
  tableFor,
} from './class-draft';
import { classDefaults, menu } from './content-testing';
import { emptyEffect } from './effect-draft';
import { emptyFeature } from './feature-draft';

const defaults = classDefaults();
const m = menu();

function feature(
  level: number,
  name: string,
  effects: ClassDraft['features'][number]['feature']['effects'] = [],
) {
  return { id: newLevelFeatureId(), level, feature: { ...emptyFeature(), name, effects } };
}

describe('the 20-level table starts from the server (E10-02 state 2)', () => {
  it('pastes the rows of each way of casting, as the server sent them', () => {
    for (const [kind, prep] of [
      ['', 'prepared'],
      ['full', 'prepared'],
      ['full', 'known'],
      ['half', 'prepared'],
      ['half', 'known'],
      ['pact', 'known'],
      ['third', 'known'],
    ] as const) {
      const table = tableFor(defaults, kind, prep)!;
      const rows = rowsOfTable(table, defaults);
      expect(rows).toHaveLength(20);
      table.rows.forEach((r, i) => {
        expect(rows[i].profBonus).toBe(r.profBonus);
        expect(rows[i].cantrips).toBe(r.cantripsKnown);
        expect(rows[i].spells).toBe(r.spellsKnown);
        expect(rows[i].slots).toEqual(r.slots.length > 0 ? [...r.slots] : Array<number>(9).fill(0));
      });
    }
  });

  it("starts a new class from the table of a class that does not cast, with the server's ASI and subclass levels", () => {
    const d = emptyClass(defaults);
    expect(d.casting.kind).toBe('');
    expect(d.rows[0].profBonus).toBe(2);
    expect(d.rows[16].profBonus).toBe(6);
    expect(d.asiLevels).toEqual([4, 8, 12, 16, 19]);
    expect(d.subclassLevel).toBe(3);
    expect(d.rows.every((r) => r.slots.every((n) => n === 0))).toBe(true);
  });

  it('a kind brings its start level and its way of preparing; the half caster has nothing at level 1', () => {
    const c = castingOfKind('half', emptyCasting(''), defaults);
    expect(c).toMatchObject({ kind: 'half', preparation: 'prepared', startLevel: 2 });
    const rows = rowsOfTable(tableFor(defaults, 'half', 'prepared'), defaults);
    expect(rows[0].slots.every((n) => n === 0)).toBe(true);
    expect(rows[1].slots[0]).toBe(2);
  });

  it('knows whether the master edited the table (the question before it is replaced)', () => {
    const casting = castingOfKind('half', emptyCasting(''), defaults);
    const rows = rowsOfTable(tableFor(defaults, 'half', 'prepared'), defaults);
    expect(rowsEdited(rows, casting, defaults)).toBe(false);
    const edited = rows.map((r, i) =>
      i === 4 ? { ...r, slots: r.slots.map((n, k) => (k === 1 ? 3 : n)) } : r,
    );
    expect(rowsEdited(edited, casting, defaults)).toBe(true);
    expect(sameRows(rows, rowsOfTable(tableFor(defaults, 'half', 'prepared'), defaults))).toBe(
      true,
    );
  });

  it('a stored third caster is not "edited": its rows carry no proficiency bonus and its levels before the start are empty', () => {
    const stored = create(TableSubclassSchema, {
      namePt: 'Tradição da Tinta',
      classKey: 'class:fighter',
      casting: {
        kind: 'third',
        ability: Ability.INTELLIGENCE,
        preparation: 'known',
        listFrom: 'class:wizard',
        startLevel: 3,
      },
      levels: tableFor(defaults, 'third', 'known')!
        .rows.slice(2)
        .map((r, i) => ({
          level: i + 3,
          cantripsKnown: r.cantripsKnown,
          spellsKnown: r.spellsKnown,
          slots: [...r.slots],
        })),
    });
    const d = subclassToDraft(stored, defaults);
    expect(rowsEdited(d.rows, d.casting, defaults)).toBe(false);
    const edited = d.rows.map((r, i) => (i === 3 ? { ...r, cantrips: 4 } : r));
    expect(rowsEdited(edited, d.casting, defaults)).toBe(true);
  });

  it('shows the circles the kind uses: 5 for the half caster, 9 for the full one, and none without casting', () => {
    const half = castingOfKind('half', emptyCasting(''), defaults);
    const rows = rowsOfTable(tableFor(defaults, 'half', 'prepared'), defaults);
    expect(gridColumns(half, rows, defaults, false)).toMatchObject({
      casts: true,
      cantrips: true,
      spells: false,
    });
    const known = gridColumns(
      castingOfKind('half', { ...half, preparation: 'known' }, defaults),
      rows,
      defaults,
      false,
    );
    expect(known.spells).toBe(true);
    expect(gridColumns(emptyCasting(''), rows, defaults, false).casts).toBe(false);
    expect(gridColumns(half, rows, defaults, true).circles).toBe(9);
  });

  it('says the slots of a row in words', () => {
    expect(
      slotsText(
        { profBonus: 3, cantrips: 0, spells: 0, slots: [4, 2, 0, 0, 0, 0, 0, 0, 0] },
        false,
      ),
    ).toBe('4 de 1º, 2 de 2º');
    expect(
      slotsText({ profBonus: 3, cantrips: 0, spells: 0, slots: [0, 0, 2, 0, 0, 0, 0, 0, 0] }, true),
    ).toBe('2 de 3º (pacto)');
    expect(
      slotsText({ profBonus: 2, cantrips: 0, spells: 0, slots: Array<number>(9).fill(0) }, false),
    ).toBe('');
  });
});

describe('the class request (E10-02 states 1 to 3)', () => {
  function guardiao(): ClassDraft {
    const d = emptyClass(defaults);
    const casting = {
      ...castingOfKind('half', d.casting, defaults),
      ability: Ability.WISDOM,
      listFrom: 'class:druid',
    };
    return {
      ...d,
      name: ' Guardião do Vale ',
      hitDie: 10,
      saves: [Ability.STRENGTH, Ability.WISDOM],
      skillFrom: ['skill:athletics', 'skill:perception'],
      proficiencies: ['proficiency:light-armor'],
      minimums: { ...d.minimums, wisdom: 13 },
      casting,
      rows: rowsOfTable(tableFor(defaults, 'half', 'prepared'), defaults),
    };
  }

  it('sends the name trimmed, the saves, the skills, the multiclass minimums and the casting', () => {
    const body = draftToClass(guardiao(), m);
    expect(body).toMatchObject({
      namePt: 'Guardião do Vale',
      hitDie: 10,
      savingThrows: [Ability.STRENGTH, Ability.WISDOM],
      skillChoose: 2,
      skillFrom: ['skill:athletics', 'skill:perception'],
      proficiencies: ['proficiency:light-armor'],
      minimums: { wisdom: 13 },
      subclassLevel: 3,
      asiLevels: [4, 8, 12, 16, 19],
      casting: {
        kind: 'half',
        ability: Ability.WISDOM,
        preparation: 'prepared',
        listFrom: 'class:druid',
        startLevel: 2,
      },
    });
    expect(body.anyOf).toBeUndefined();
    expect(body.levels).toHaveLength(20);
  });

  it('sends a class that does not cast with no casting and no slots', () => {
    const body = draftToClass({ ...emptyClass(defaults), name: 'Cronista' }, m);
    expect(body.casting).toBeUndefined();
    expect(body.levels!.every((l) => (l.slots ?? []).length === 0)).toBe(true);
    expect(body.levels![0].profBonus).toBe(2);
  });

  it('puts a grid edit in its row, and only there', () => {
    const d = guardiao();
    const rows = d.rows.map((r, i) =>
      i === 4 ? { ...r, slots: r.slots.map((n, k) => (k === 2 ? 1 : n)) } : r,
    );
    const body = draftToClass({ ...d, rows }, m);
    expect(body.levels![4].slots).toEqual([4, 2, 1, 0, 0, 0, 0, 0, 0]);
    expect(body.levels![5].slots).toEqual(d.rows[5].slots);
    expect(body.levels![0].slots).toEqual([]);
  });

  it('puts each feature under its level, in the order of its level, with only the fields of the chosen effect type', () => {
    const d = guardiao();
    const features = [
      feature(1, 'Vigília', [
        {
          ...emptyEffect('sense'),
          sense: 'darkvision',
          rangeM: '18',
          target: 'speed.walk',
          value: '5',
        },
      ]),
      feature(1, 'Passo firme'),
      feature(5, 'Ataque extra'),
    ];
    const body = draftToClass({ ...d, features }, m);
    expect(body.levels![0].features!.map((f) => f.namePt)).toEqual(['Vigília', 'Passo firme']);
    expect(body.levels![4].features!.map((f) => f.namePt)).toEqual(['Ataque extra']);
    // A sense sends its sense and range, never the modifier fields the draft still holds.
    expect(body.levels![0].features![0].effects![0]).toEqual({
      type: 'sense',
      sense: 'darkvision',
      rangeFt: 60,
    });
  });

  it('sorts the features by level and keeps the order inside a level', () => {
    const a = feature(5, 'B');
    const b = feature(1, 'A');
    const c = feature(5, 'C');
    expect(sortedFeatures([a, b, c]).map((f) => f.feature.name)).toEqual(['A', 'B', 'C']);
  });

  it('reads a stored class back into the same request (nothing the editor does not show is lost)', () => {
    const body = draftToClass(guardiao(), m);
    const stored = create(TableClassSchema, {
      ...(body as object),
      multiclassProficiencies: ['proficiency:shields'],
      multiclassSkillChoose: 1,
      anyOf: { strength: 13, dexterity: 13 },
    });
    const back = draftToClass(classToDraft(stored, defaults), m);
    expect(back).toMatchObject({
      multiclassProficiencies: ['proficiency:shields'],
      multiclassSkillChoose: 1,
      anyOf: { strength: 13, dexterity: 13 },
      minimums: { wisdom: 13 },
      casting: { kind: 'half', startLevel: 2 },
    });
    expect(back.levels).toEqual(body.levels);
  });

  it("takes the server's start level for a casting stored with 0", () => {
    const stored = create(TableClassSchema, {
      namePt: 'X',
      casting: { kind: 'half', ability: Ability.WISDOM, preparation: 'prepared' },
    });
    expect(classToDraft(stored, defaults).casting.startLevel).toBe(2);
  });
});

describe('the paths of a refusal (E10-02 state 3)', () => {
  it("names a feature by its level and its place among that level's features", () => {
    const features = [feature(1, 'A'), feature(1, 'B'), feature(5, 'C')];
    expect(classFeatureBase(features, 0)).toBe('table_class.levels[0].features[0]');
    expect(classFeatureBase(features, 1)).toBe('table_class.levels[0].features[1]');
    expect(classFeatureBase(features, 2)).toBe('table_class.levels[4].features[0]');
  });

  it('lists the inputs of a feature and of the fields of its effect', () => {
    const e = emptyEffect('sense');
    const paths = classFeaturePaths([feature(4, 'Olhar', [e])], m);
    expect(paths).toEqual(
      expect.arrayContaining([
        'table_class.levels[3].features[0]',
        'table_class.levels[3].features[0].name_pt',
        'table_class.levels[3].features[0].effects[0].type',
        'table_class.levels[3].features[0].effects[0].sense',
        'table_class.levels[3].features[0].effects[0].range_ft',
      ]),
    );
  });
});

describe('the subclass request (E10-02 state 4)', () => {
  it('sends a third caster with its rows from the level casting starts, the casting columns from the server', () => {
    const casting = {
      ...castingOfKind('third', { ...emptyCasting('third'), preparation: 'known' }, defaults),
      ability: Ability.INTELLIGENCE,
      listFrom: 'class:wizard',
    };
    const d = {
      ...emptySubclass('class:fighter'),
      name: 'Tradição da Tinta',
      conjures: true,
      casting,
      rows: rowsOfTable(tableFor(defaults, 'third', 'known'), defaults),
    };
    const body = draftToSubclass(d, m);
    expect(body.casting).toMatchObject({
      kind: 'third',
      ability: Ability.INTELLIGENCE,
      listFrom: 'class:wizard',
      preparation: 'known',
      startLevel: 3,
    });
    expect(body.levels).toHaveLength(18);
    expect(body.levels![0]).toMatchObject({
      level: 3,
      cantripsKnown: 2,
      spellsKnown: 3,
      slots: [2, 0, 0, 0, 0, 0, 0, 0, 0],
    });
    expect(body.levels![17].level).toBe(20);
    expect(subclassLevels(d)[0]).toBe(3);
    expect(subclassRowBase(d, 3)).toBe('table_subclass.levels[0]');
    expect(subclassRowBase(d, 2)).toBe('');
  });

  it('a subclass that does not cast sends only the levels that have a feature, and no casting', () => {
    const d = {
      ...emptySubclass('class:wizard'),
      name: 'Tradição',
      features: [feature(6, 'Tinta viva'), feature(2, 'Traço arcano')],
    };
    const body = draftToSubclass(d, m);
    expect(body.casting).toBeUndefined();
    expect(body.levels!.map((l) => l.level)).toEqual([2, 6]);
    expect(body.levels![0].features![0].namePt).toBe('Traço arcano');
    expect(subclassFeatureBase({ ...d, features: sortedFeatures(d.features) }, 1)).toBe(
      'table_subclass.levels[1].features[0]',
    );
  });

  it('groups the always-prepared spells by class level, and sends them as the server lists them', () => {
    const d = {
      ...emptySubclass('class:cleric'),
      name: 'Domínio do Caminho',
      alwaysPrepared: [
        { level: 1, spells: ['spell:longstrider', 'spell:detect-magic'] },
        { level: 3, spells: ['spell:misty-step'] },
      ],
    };
    expect(draftToSubclass(d, m).alwaysPrepared).toEqual([
      { classLevel: 1, spellKey: 'spell:longstrider' },
      { classLevel: 1, spellKey: 'spell:detect-magic' },
      { classLevel: 3, spellKey: 'spell:misty-step' },
    ]);
    expect(flattenAlwaysPrepared(d.alwaysPrepared)).toHaveLength(3);
    const stored = create(TableSubclassSchema, {
      namePt: 'Domínio do Caminho',
      classKey: 'class:cleric',
      alwaysPrepared: flattenAlwaysPrepared(d.alwaysPrepared),
    });
    expect(subclassToDraft(stored, defaults).alwaysPrepared).toEqual(d.alwaysPrepared);
  });

  it("reads a stored third caster back, the 0 start level taken from the server's table", () => {
    const stored = create(TableSubclassSchema, {
      namePt: 'Tradição da Tinta',
      classKey: 'class:fighter',
      casting: {
        kind: 'third',
        ability: Ability.INTELLIGENCE,
        preparation: 'known',
        listFrom: 'class:wizard',
      },
      levels: [{ level: 3, cantripsKnown: 2, spellsKnown: 3, slots: [2] }],
      descPt: ['Uma tradição.', 'Outra.'],
    });
    const d = subclassToDraft(stored, defaults);
    expect(d.conjures).toBe(true);
    expect(d.casting.startLevel).toBe(3);
    expect(d.rows[2]).toMatchObject({ cantrips: 2, spells: 3 });
    expect(d.text).toBe('Uma tradição.\n\nOutra.');
    expect(draftToSubclass(d, m).descPt).toEqual(['Uma tradição.', 'Outra.']);
  });
});
