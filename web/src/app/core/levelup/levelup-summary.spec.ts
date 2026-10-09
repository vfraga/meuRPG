import { create } from '@bufbuild/protobuf';

import {
  Ability,
  AttackSchema,
  CharacterSpellSchema,
  DerivedClassSchema,
  HitDiceSchema,
  SpellSchema,
  SpellcastingSchema,
  type DerivedSheet,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { changeRows, type SummaryContext } from './levelup-summary';
import { pensantus } from './levelup-testing';

const plain = (s: string) => s.replace(/\u00a0/g, ' ');

const ctx: SummaryContext = {
  classKey: 'class:wizard',
  hpSub: 'Média 4 + Constituição +3',
  cantrips: ['Prestidigitação'],
  spells: ['Passo Nebuloso', 'Reflexos'],
  prepared: ['Passo Nebuloso', 'Detectar Magia'],
  spellbook: true,
  spellsMissing: 0,
};

describe('changeRows: the summary of E8-15', () => {
  const rows = changeRows(pensantus(false), pensantus(true), ctx);
  const row = (key: string) => rows.find((r) => r.key === key);

  it('lists what moves, before → after, in the artboard order, and leaves out what does not', () => {
    expect(rows.map((r) => r.key)).toEqual([
      'level',
      'ability-int',
      'hp',
      'hit-dice',
      'dc',
      'attack',
      'cantrips',
      'spells',
      'slots-2',
      'prepared',
      'save-int',
      'skills-6>7',
      'passive-investigation',
    ]);
  });

  it('words each line', () => {
    expect(row('level')).toMatchObject({ before: 'Mago 3', after: 'Mago 4' });
    expect(row('ability-int')).toMatchObject({
      label: 'Inteligência',
      before: '18',
      after: '20',
      sub: 'Modificador +4 → +5',
    });
    expect(row('hp')).toMatchObject({
      before: '23',
      after: '30',
      sub: 'Média 4 + Constituição +3',
    });
    expect(row('hit-dice')).toMatchObject({ before: '3d6', after: '4d6' });
    expect(row('dc')).toMatchObject({ before: '14', after: '15' });
    expect(row('attack')).toMatchObject({ before: '+6', after: '+7' });
    expect(row('prepared')).toMatchObject({
      before: '7',
      after: '9',
      sub: 'Novas: Passo Nebuloso e Detectar Magia',
    });
    expect(row('cantrips')).toMatchObject({
      before: '3',
      after: '4',
      sub: 'Novo: Prestidigitação',
    });
    expect(row('slots-2')).toMatchObject({ label: 'Espaços de 2º nível', before: '2', after: '3' });
    expect(row('save-int')).toMatchObject({
      label: 'Teste de resistência de Inteligência',
      before: '+6',
      after: '+7',
    });
    expect(row('passive-investigation')).toMatchObject({
      label: 'Investigação passiva',
      before: '16',
      after: '17',
    });
  });

  it('groups the skills that move by the same amount', () => {
    expect(row('skills-6>7')).toMatchObject({
      label: 'Arcanismo e História',
      before: '+6',
      after: '+7',
    });
  });

  it('says how many spells of the book are still missing', () => {
    const half = changeRows(pensantus(false), pensantus(true), {
      ...ctx,
      spells: ['Passo Nebuloso'],
      spellsMissing: 1,
    });
    expect(plain(half.find((r) => r.key === 'spells')?.after ?? '')).toContain('(falta 1)');
  });
});

describe('changeRows: a table class (E10-02 state 7)', () => {
  const ctxTable: SummaryContext = {
    ...ctx,
    table: true,
    newFeatures: ['Estilo de luta', 'Conjuração'],
  };
  const rows = changeRows(pensantus(false), pensantus(true), ctxTable);
  const row = (key: string) => rows.find((r) => r.key === key);

  it('tags the slots that come from the class table with "Da mesa", and says where they come from', () => {
    expect(row('slots-2')).toMatchObject({
      table: true,
      sub: 'Da tabela da classe',
      before: '2',
      after: '3',
    });
  });

  it("tags only the slots: the hit points and the rest are the same rows as the SRD's", () => {
    expect(rows.filter((r) => r.table).map((r) => r.key)).toEqual(['slots-2']);
  });

  it('lists the features the level gives, by name, with their count, and no before', () => {
    expect(row('features')).toMatchObject({
      label: 'Novas características',
      before: '',
      after: '2',
      sub: expect.any(String),
    });
    expect(plain(row('features')?.sub ?? '')).toBe('Estilo de luta · Conjuração');
  });

  it('shows only the rows that change: an SRD class has no tag and no feature row unless there are features', () => {
    const srd = changeRows(pensantus(false), pensantus(true), ctx);
    expect(srd.some((r) => r.table)).toBe(false);
    expect(srd.some((r) => r.key === 'features')).toBe(false);
    expect(srd.find((r) => r.key === 'slots-2')?.sub).toBe('');
  });
});

describe('changeRows: a class that starts casting, and how it learns its spells (10.12b fix round 1)', () => {
  const none = (s: DerivedSheetLike) => s;
  type DerivedSheetLike = ReturnType<typeof pensantus>;
  const nonCasterBefore = () => {
    const before = pensantus(false);
    before.spellcasting = [];
    before.spellSlots = [];
    before.spells = [];
    return none(before);
  };

  it('shows a dash, not 0 or +0, before the class casts', () => {
    const rows = changeRows(nonCasterBefore(), pensantus(true), ctx);
    expect(rows.find((r) => r.key === 'dc')).toMatchObject({ before: '—', after: '15' });
    expect(rows.find((r) => r.key === 'attack')).toMatchObject({ before: '—', after: '+7' });
    expect(rows.find((r) => r.key === 'cantrips')).toMatchObject({ before: '—' });
    expect(rows.find((r) => r.key === 'prepared')).toMatchObject({ before: '—', after: '9' });
  });

  it('writes the hit dice of every class, joined by a plus', () => {
    const after = pensantus(true);
    after.hitDice = [...after.hitDice, create(HitDiceSchema, { faces: 10, count: 1 })];
    const rows = changeRows(pensantus(false), after, ctx);
    expect(rows.find((r) => r.key === 'hit-dice')).toMatchObject({
      before: '3d6',
      after: '4d6 + 1d10',
    });
  });

  it('does not show the spells a class does not learn, even when the level names some', () => {
    const rows = changeRows(pensantus(false), pensantus(true), { ...ctx, learnsSpells: false });
    expect(ctx.spells.length).toBeGreaterThan(0);
    expect(rows.some((r) => r.key === 'spells')).toBe(false);
  });

  it('counts the known spells from the spell list of the class that casts them, not from the class that levels up', () => {
    const known = (n: number) =>
      Array.from({ length: n }, (_, i) =>
        create(CharacterSpellSchema, {
          spell: create(SpellSchema, { key: `spell:s${i}`, level: 1, classKeys: ['class:wizard'] }),
        }),
      );
    const subclassCaster = (sheet: DerivedSheet, spells: number) => {
      sheet.spellcasting.forEach((c) => {
        c.classKey = 'class:fighter';
        c.spellsKnown = 0;
      });
      sheet.spells = known(spells);
      return sheet;
    };
    const rows = changeRows(
      subclassCaster(pensantus(false), 2),
      subclassCaster(pensantus(true), 3),
      { ...ctx, classKey: 'class:fighter', spellListClassKey: 'class:wizard' },
    );
    expect(rows.find((r) => r.key === 'spells')).toMatchObject({ before: '2', after: '3' });
  });

  it('a class that prepares from its list shows "Magias preparadas" and no "Magias conhecidas"', () => {
    const rows = changeRows(pensantus(false), pensantus(true), {
      ...ctx,
      learnsSpells: false,
      spells: [],
    });
    expect(rows.some((r) => r.key === 'spells')).toBe(false);
    expect(rows.some((r) => r.key === 'prepared')).toBe(true);
  });

  it('a class that learns its spells keeps "Magias conhecidas" (or the book), and a known-spells class has no prepared row', () => {
    expect(
      changeRows(pensantus(false), pensantus(true), { ...ctx, learnsSpells: true }).some(
        (r) => r.key === 'spells',
      ),
    ).toBe(true);
    const bard = pensantus(true);
    bard.spellcasting[0].preparedMax = 0;
    const rows = changeRows(pensantus(false), bard, { ...ctx, learnsSpells: true });
    expect(rows.some((r) => r.key === 'prepared')).toBe(false);
  });
});

describe('changeRows: a row with no gain is not shown', () => {
  it('has no "Truques" row for a class that has no cantrips: nothing before, 0 after', () => {
    const before = pensantus(false);
    before.spellcasting = [];
    before.spellSlots = [];
    before.spells = [];
    const after = pensantus(true);
    after.spellcasting[0].cantripsKnown = 0;
    const rows = changeRows(before, after, ctx);
    expect(rows.some((r) => r.key === 'cantrips')).toBe(false);
    expect(rows.some((r) => r.key === 'dc')).toBe(true);
  });

  it('keeps it when the class gets cantrips, and when it already had some', () => {
    const before = pensantus(false);
    before.spellcasting = [];
    expect(
      changeRows(before, pensantus(true), ctx).find((r) => r.key === 'cantrips'),
    ).toMatchObject({ before: '—', after: '4' });
    expect(
      changeRows(pensantus(false), pensantus(true), ctx).some((r) => r.key === 'cantrips'),
    ).toBe(true);
  });
});

// Wizard 3 / Cleric 1 levelling Cleric. The server lists `spellcasting` in class order (Wizard first).
function multiclass(after: boolean): DerivedSheet {
  const sheet = pensantus(false); // Wizard 3 numbers: DC 14, +6, 3 cantrips, 7 prepared (unchanged)
  sheet.classes = [
    create(DerivedClassSchema, { classKey: 'class:wizard', namePt: 'Mago', level: 3 }),
    create(DerivedClassSchema, {
      classKey: 'class:cleric',
      namePt: 'Clérigo',
      level: after ? 2 : 1,
    }),
  ];
  sheet.spellcasting = [
    ...sheet.spellcasting,
    create(SpellcastingSchema, {
      classKey: 'class:cleric',
      classNamePt: 'Clérigo',
      ability: Ability.WISDOM,
      saveDc: after ? 13 : 12,
      attackBonus: after ? 5 : 4,
      cantripsKnown: after ? 4 : 3,
      preparedMax: after ? 5 : 4,
    }),
  ];
  return sheet;
}

describe('changeRows: a multiclass sheet reads the spellcasting of the class being levelled', () => {
  const clericCtx: SummaryContext = {
    hpSub: '',
    cantrips: ['Chama Sagrada'],
    spells: [],
    prepared: ['Bênção'],
    spellbook: false,
    spellsMissing: 0,
    learnsSpells: false,
    classKey: 'class:cleric',
  };
  const rows = changeRows(multiclass(false), multiclass(true), clericCtx);
  const row = (key: string) => rows.find((r) => r.key === key);

  it("shows the Cleric's spell DC and attack going up", () => {
    expect(row('dc')).toMatchObject({ before: '12', after: '13' });
    expect(row('attack')).toMatchObject({ before: '+4', after: '+5' });
  });

  it("shows the Cleric's cantrips and prepared spells with their new names", () => {
    expect(row('cantrips')).toMatchObject({
      before: '3',
      after: '4',
      sub: 'Novo: Chama Sagrada',
    });
    expect(row('prepared')).toMatchObject({ before: '4', after: '5', sub: 'Nova: Bênção' });
  });
});

// A Bard with Magical Secrets: spells off the Bard list count among the ones the class knows.
describe('changeRows: the spells a class knows count the ones taken from another class list', () => {
  const spell = (key: string, lists: string[]) =>
    create(CharacterSpellSchema, {
      spell: create(SpellSchema, { key, level: 1, classKeys: lists }),
    });
  const bard = (after: boolean): DerivedSheet => {
    const sheet = pensantus(false);
    sheet.classes = [
      create(DerivedClassSchema, {
        classKey: 'class:bard',
        namePt: 'Bardo',
        level: after ? 10 : 9,
      }),
    ];
    sheet.spellcasting = [
      create(SpellcastingSchema, {
        classKey: 'class:bard',
        classNamePt: 'Bardo',
        ability: Ability.CHARISMA,
        saveDc: 16,
        attackBonus: 8,
        cantripsKnown: 4,
        spellsKnown: after ? 6 : 4,
      }),
    ];
    sheet.spells = [
      spell('spell:healing-word', ['class:bard', 'class:cleric']),
      spell('spell:vicious-mockery', ['class:bard']),
      spell('spell:thunderwave', ['class:bard', 'class:wizard']),
      spell('spell:cure-wounds', ['class:bard', 'class:cleric']),
    ];
    if (after) {
      // Magical Secrets: one of the Cleric list and one of the Wizard list, none of them on the Bard list.
      sheet.spells.push(spell('spell:spiritual-weapon', ['class:cleric']));
      sheet.spells.push(spell('spell:misty-step', ['class:wizard']));
    }
    return sheet;
  };
  const bardCtx: SummaryContext = {
    classKey: 'class:bard',
    hpSub: '',
    cantrips: [],
    spells: ['Arma Espiritual', 'Passo Nebuloso'],
    prepared: [],
    spellbook: false,
    spellsMissing: 0,
  };

  it('shows the known spells as the server counts them, the ones off the class list included', () => {
    const rows = changeRows(bard(false), bard(true), bardCtx);
    expect(rows.find((r) => r.key === 'spells')).toMatchObject({
      label: 'Magias conhecidas',
      before: '4',
      after: '6',
      sub: 'Novas: Arma Espiritual e Passo Nebuloso',
    });
  });

  it("counts a wizard's book from the spells on the sheet, since the server counts no fixed number for it (positive control)", () => {
    const before = pensantus(false);
    const after = pensantus(true);
    before.spells = [spell('spell:shield', ['class:wizard'])];
    after.spells = [
      spell('spell:shield', ['class:wizard']),
      spell('spell:misty-step', ['class:wizard']),
    ];
    const rows = changeRows(before, after, {
      ...bardCtx,
      classKey: 'class:wizard',
      spellbook: true,
    });
    expect(rows.find((r) => r.key === 'spells')).toMatchObject({ before: '1', after: '2' });
  });
});

describe('changeRows: the attacks a level moves', () => {
  const sword = (bonus: number, damage: string) =>
    create(AttackSchema, {
      key: 'equipment:longsword',
      namePt: 'Espada Longa',
      attackBonus: bonus,
      damage,
      damageTypePt: 'cortante',
    });
  const bolt = (damage: string) =>
    create(AttackSchema, {
      key: 'spell:fire-bolt',
      namePt: 'Raio de Fogo',
      attackBonus: 6,
      damage,
      damageTypePt: 'fogo',
    });

  it('lists a weapon whose to-hit and damage rise with the score, with its damage type', () => {
    const before = pensantus(false, { attacks: [sword(5, '1d8+3'), bolt('1d10')] });
    const after = pensantus(true, { attacks: [sword(7, '1d8+5'), bolt('1d10')] });

    const rows = changeRows(before, after, ctx).filter((r) => r.key.startsWith('attack-'));

    expect(rows).toEqual([
      {
        key: 'attack-equipment:longsword',
        label: 'Espada Longa',
        before: '+5 · 1d8+3',
        after: '+7 · 1d8+5',
        sub: 'cortante',
      },
    ]);
  });

  it('lists a cantrip that rolls more dice', () => {
    const before = pensantus(false, { attacks: [bolt('1d10')] });
    const after = pensantus(true, { attacks: [bolt('2d10')] });

    const rows = changeRows(before, after, ctx).filter((r) => r.key.startsWith('attack-'));

    expect(rows).toHaveLength(1);
    expect(rows[0]).toMatchObject({
      label: 'Raio de Fogo',
      before: '+6 · 1d10',
      after: '+6 · 2d10',
    });
  });

  it('leaves out an attack that does not change and one the sheet had no line for', () => {
    const before = pensantus(false, { attacks: [sword(5, '1d8+3')] });
    const after = pensantus(true, { attacks: [sword(5, '1d8+3'), bolt('1d10')] });

    expect(changeRows(before, after, ctx).some((r) => r.key.startsWith('attack-'))).toBe(false);
  });
});
