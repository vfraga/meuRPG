import { create } from '@bufbuild/protobuf';

import { CreatureSchema } from '../../../gen/meurpg/rules/v1/rules_pb';
import {
  TableContentKind,
  TableCreatureSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import type { Carrier } from './creature-carriers';
import { carriersNote, readCreatureForMaster } from './creature-phone-read';

const nameOf = (k: string): string =>
  ({ 'damage-type:bludgeoning': 'Concussão', 'damage-type:radiant': 'Radiante' })[k] ?? k;
const carrier = (name: string): Carrier => ({
  key: name,
  kind: TableContentKind.SPELL,
  name,
  detail: '',
});

/** The texts tie numbers with no-break spaces: the specs write plain ones. */
const plain = <T extends { label?: string; value?: string; text?: string }>(xs: readonly T[]) =>
  xs.map((x) => ({
    ...x,
    ...(x.value ? { value: x.value.replace(/\u00a0/g, ' ') } : {}),
    ...(x.text ? { text: x.text.replace(/\u00a0/g, ' ') } : {}),
  }));

const base = create(CreatureSchema, {
  summary: { namePt: 'Zumbi', typePt: 'morto-vivo', sizePt: 'Médio' },
  hitPoints: 22,
  armorClass: 8,
  immunities: [{ types: [{ namePt: 'veneno' }] }],
  actions: [
    {
      name: 'Slam',
      namePt: 'Pancada',
      hasAttack: true,
      attackBonus: 3,
      melee: true,
      reachFt: 5,
      damage: [{ dice: '1d6+1', damageTypePt: 'concussão' }],
    },
  ],
});

describe('readCreatureForMaster', () => {
  it('writes the rows of the board: base, numbers with the owner, attack bonus', () => {
    const c = create(TableCreatureSchema, {
      baseKey: 'monster:zombie',
      hitPoints: 11,
      hitPointsFormula: '4 + 4 * level()',
      armorClassFormula: '13 + prof()',
      damageBonusFormula: 'prof()',
      attackBonusFormula: 'mod("wis") + prof()',
    });
    const r = readCreatureForMaster(c, base, 'Zumbi', nameOf, []);
    expect(plain(r.rows)).toEqual([
      { label: 'Base', value: 'Zumbi (SRD), morto-vivo médio' },
      { label: 'PV', value: '11 · com dono, 4 + 4 × nível' },
      { label: 'CA', value: '8 · com dono, 13 + proficiência' },
      { label: 'Bônus no dano', value: 'com dono, proficiência' },
      { label: 'Bônus de ataque', value: 'com dono, modificador de Sabedoria + proficiência' },
      { label: 'Imunidade', value: 'veneno (da base)' },
    ]);
  });

  it('says only the name of the base until its stat block comes, and omits what is empty', () => {
    const r = readCreatureForMaster(create(TableCreatureSchema), null, 'Lobo', nameOf, []);
    expect(r.rows).toEqual([{ label: 'Base', value: 'Lobo (SRD)' }]);
    expect(r.sections).toEqual([]);
    expect(r.rowsNote).toBe('');
  });

  it('lists the damage lines with where each comes from, and says only the master reads them', () => {
    const c = create(TableCreatureSchema, {
      damageMods: [
        { kind: 'resist', damageTypes: ['damage-type:bludgeoning'], qualifier: 'nonmagical' },
        { kind: 'vulnerable', damageTypes: ['damage-type:radiant'] },
      ],
    });
    const r = readCreatureForMaster(c, base, 'Zumbi', nameOf, []);
    expect(r.rows.filter((x) => !['Base', 'PV', 'CA'].includes(x.label))).toEqual([
      { label: 'Resistência', value: 'concussão de ataque não mágico (da mesa)' },
      { label: 'Vulnerabilidade', value: 'radiante (da mesa)' },
      { label: 'Imunidade', value: 'veneno (da base)' },
    ]);
    expect(r.rowsNote).toBe('Só você muda estas linhas. Quem tem a criatura as lê na ficha dela.');
  });

  it("puts the traits and the attacks (the base's too) in one card with who brings the creature", () => {
    const c = create(TableCreatureSchema, {
      traits: [{ namePt: 'Elo com o guardião', textPt: 'Entende o dono.' }],
      attacks: [
        {
          namePt: 'Investida do Vale',
          melee: true,
          diceCount: 2,
          diceSides: 6,
          damageType: 'damage-type:bludgeoning',
        },
        { namePt: 'Uivo', rangeFt: 30, longRangeFt: 60, diceCount: 1, diceSides: 4 },
      ],
    });
    const r = readCreatureForMaster(c, base, 'Zumbi', nameOf, [
      carrier('A'),
      carrier('B'),
      carrier('A'),
    ]);
    expect(r.sections).toHaveLength(1);
    expect(plain(r.sections[0].items)).toEqual([
      { title: 'Elo com o guardião', text: 'Entende o dono.' },
      { title: 'Pancada (da base)', text: 'Corpo a corpo, 1,5 m · +3 · 1d6 + 1 concussão.' },
      { title: 'Investida do Vale', text: 'Corpo a corpo, 1,5 m · 2d6 concussão.' },
      { title: 'Uivo', text: 'À distância, 9 m e 18 m · 1d4.' },
    ]);
    expect(r.sections[0].note).toBe('Trazida por A e B.');
  });
});

describe('carriersNote', () => {
  it('names the entries, and is empty for none', () => {
    expect(carriersNote([])).toBe('');
    expect(carriersNote([carrier('A')])).toBe('Trazida por A.');
    expect(carriersNote([carrier('A'), carrier('B'), carrier('C')])).toBe('Trazida por A, B e C.');
  });
});
