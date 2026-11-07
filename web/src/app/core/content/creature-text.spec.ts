import { create } from '@bufbuild/protobuf';

import { CreatureSchema } from '../../../gen/meurpg/rules/v1/rules_pb';
import {
  TableClassFeatureSchema,
  TableClassSchema,
  TableContentKind,
  TableCreatureSchema,
  TableEntrySchema,
  TableSpellSchema,
  type TableEntry,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { creatureToDraft, emptyAttack, emptyCreature } from './creature-draft';
import { carriersOf } from './creature-carriers';
import { creaturePreview, diceWithBonus } from './creature-preview';
import { attackSummary, formulaEcho, formulaWords, traitLine } from './creature-text';

const tight = (t: string) => t.replace(/[  ]/g, ' ');
const nameOf = (k: string) => ({ 'damage-type:bludgeoning': 'contundente' })[k] ?? k;

describe('the echo under a formula', () => {
  it('writes the functions and the operators in words', () => {
    expect(formulaWords('4 + 4 * level()')).toBe('4 + 4 × nível do dono');
    expect(formulaWords('13+prof()')).toBe('13 + bônus de proficiência');
    expect(formulaWords('mod("wis") + prof()')).toBe(
      'modificador de Sabedoria + bônus de proficiência',
    );
    expect(formulaWords("mod('cha')")).toBe('modificador de Carisma');
    expect(formulaWords('score("str") - 10')).toBe('valor de Força − 10');
    expect(formulaWords('slot() * 2')).toBe('círculo da magia × 2');
    expect(formulaWords('floor(level() / 2)')).toBe('floor(nível do dono ÷ 2)');
    expect(formulaWords('classLevel("ranger") + speedwalk()')).toBe(
      'nível de ranger + deslocamento a pé',
    );
    expect(formulaWords('mod("xyz")')).toBe('mod("xyz")');
    expect(formulaWords('  ')).toBe('');
  });

  it('ends with the range the server clamps to, and says nothing for an empty formula', () => {
    expect(formulaEcho('4 + 4 * level()', 'de 1 a 9.999')).toBe(
      '= 4 + 4 × nível do dono · de 1 a 9.999',
    );
    expect(formulaEcho('', 'de 1 a 40')).toBe('');
  });
});

describe('the lines of a collapsed attack and trait', () => {
  it('says the shape, the dice, the damage type and whose numbers the attack uses', () => {
    const a = {
      ...emptyAttack(),
      diceCount: '2',
      diceSides: '6',
      damageType: 'damage-type:bludgeoning',
    };
    expect(tight(attackSummary(a, nameOf))).toBe(
      'Corpo a corpo, 1,5 m · 2d6 contundente · acerto e dano da criatura',
    );
    expect(tight(attackSummary({ ...a, reachM: '3', toHit: 'prof()' }, nameOf))).toBe(
      'Corpo a corpo, 3 m · 2d6 contundente · acerto bônus de proficiência, dano da criatura',
    );
    expect(
      tight(
        attackSummary(
          { ...a, melee: false, rangeM: '9', longRangeM: '18', damageBonus: '1' },
          nameOf,
        ),
      ),
    ).toBe('À distância, 9 m e 18 m · 2d6 contundente · acerto da criatura, dano 1');
  });

  it('puts a trait text on one line', () => {
    expect(traitLine('Primeira.\n\n  Segunda.')).toBe('Primeira. Segunda.');
  });
});

describe('the dice with the creature damage bonus', () => {
  it('adds it to the dice, and a flat damage stops at 0', () => {
    expect(diceWithBonus('2d4+2', 1)).toBe('2d4 + 3');
    expect(diceWithBonus('1d6', 0)).toBe('1d6');
    expect(diceWithBonus('1d6', -2)).toBe('1d6 − 2');
    expect(diceWithBonus('2d6-1', 1)).toBe('2d6');
    expect(diceWithBonus('1', -5)).toBe('0');
    expect(diceWithBonus('2d6+MOD', 3)).toBe('2d6 + MOD');
  });
});

describe('"Como fica na mesa"', () => {
  const base = create(CreatureSchema, {
    summary: {
      key: 'monster:wolf',
      namePt: 'Lobo',
      typePt: 'fera',
      sizePt: 'Médio',
      challengeRating: '1/4',
    },
    armorClass: 13,
    hitPoints: 11,
    speedWalkFt: 40,
    actions: [
      {
        name: 'Bite',
        namePt: 'Mordida',
        hasAttack: true,
        attackBonus: 4,
        damage: [{ dice: '2d4+2', damageTypePt: 'perfurante' }],
      },
      { name: 'Howl', text: 'x' },
    ],
  });

  it('writes the numbers, with the formula of an owner beside them, and the attacks without an owner', () => {
    const d = {
      ...creatureToDraft(
        create(TableCreatureSchema, {
          namePt: 'Lobo do Vale',
          baseKey: 'monster:wolf',
          damageBonus: 1,
          hitPointsFormula: '4 + 4 * level()',
          armorClassFormula: '13 + prof()',
          attacks: [
            {
              namePt: 'Investida do Vale',
              melee: true,
              diceCount: 2,
              diceSides: 6,
              damageType: 'damage-type:bludgeoning',
            },
          ],
          traits: [{ namePt: 'Elo', textPt: 'Texto.' }],
        }),
      ),
    };
    const p = creaturePreview(d, base, 'Lobo', nameOf);
    expect(p.name).toBe('Lobo do Vale');
    expect(tight(p.line)).toBe('Fera · Médio · base Lobo · CA 13 · PV 11 · 12 m');
    expect(p.rows.map((r) => [r.label, tight(r.value)])).toEqual([
      ['CA', '13 · com dono, 13 + bônus de proficiência'],
      ['PV', '11 · com dono, 4 + 4 × nível do dono'],
      ['Deslocamento', '12 m'],
      ['Bônus no dano', '+1'],
    ]);
    expect(p.actions.map((a) => [a.name, a.origin, tight(a.detail)])).toEqual([
      ['Mordida', 'da base', '+4 · 2d4 + 3 perfurante'],
      ['Investida do Vale', 'da mesa', '+0 · 2d6 + 1 contundente'],
    ]);
    expect(p.traits).toEqual(['Elo']);
    expect(p.ownerNote).toBe('Com dono, o acerto e o dano seguem as fórmulas.');
  });

  it('uses the fixed numbers over the base and says nothing of an owner when no formula is set', () => {
    const d = { ...emptyCreature(), baseKey: 'monster:wolf', hitPoints: '40', armorClass: '17' };
    const p = creaturePreview(d, base, 'Lobo', nameOf);
    expect(p.name).toBe('Nova criatura');
    expect(p.rows.slice(0, 2).map((r) => [r.label, tight(r.value)])).toEqual([
      ['CA', '17'],
      ['PV', '40'],
    ]);
    expect(p.ownerNote).toBe('');
  });

  it('draws what it can before the base is read', () => {
    const p = creaturePreview(
      { ...emptyCreature(), baseKey: 'monster:wolf' },
      null,
      'wolf',
      nameOf,
    );
    expect(p.rows).toEqual([]);
    expect(p.line).toBe('base wolf');
    expect(p.actions).toEqual([]);
  });
});

describe('who brings a creature', () => {
  const key = 'creature:lobo-do-vale@mesa';
  const dressed = (kind: TableContentKind, name: string, body: TableEntry['body']) =>
    create(TableEntrySchema, { key: `${name}@mesa`, kind, namePt: name, body });

  it('finds the spells, class features and class features of a level that list it, with where they come from', () => {
    const entries = [
      dressed(TableContentKind.SPELL, 'Chamado da Matilha', {
        case: 'tableSpell',
        value: create(TableSpellSchema, {
          level: 3,
          summon: { creatureKeys: [key, 'monster:wolf'] },
        }),
      }),
      dressed(TableContentKind.SPELL, 'Outra', {
        case: 'tableSpell',
        value: create(TableSpellSchema, { level: 1, summon: { creatureKeys: ['monster:wolf'] } }),
      }),
      dressed(TableContentKind.CLASS_FEATURE, 'Companheiro do Vale', {
        case: 'tableClassFeature',
        value: create(TableClassFeatureSchema, { level: 3, summon: { creatureKeys: [key] } }),
      }),
      dressed(TableContentKind.CLASS, 'Guardião do Vale', {
        case: 'tableClass',
        value: create(TableClassSchema, {
          levels: [
            { features: [] },
            {
              features: [
                { namePt: 'Chamado', summon: { creatureKeys: [key] } },
                { namePt: 'Nada' },
              ],
            },
          ],
        }),
      }),
    ];
    expect(carriersOf(entries, key).map((c) => [c.name, c.detail])).toEqual([
      ['Chamado da Matilha', 'magia da mesa, 3º círculo'],
      ['Companheiro do Vale', 'característica da classe, nível 3'],
      ['Chamado', 'Guardião do Vale, nível 2'],
    ]);
    expect(carriersOf(entries, '')).toEqual([]);
    expect(carriersOf(entries, 'creature:outra@mesa')).toEqual([]);
  });
});
