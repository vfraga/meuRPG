import { damageRows } from './damage-rows';

const roll = (over: Record<string, unknown>) =>
  ({
    partKey: 'weapon',
    labelPt: 'Espada curta',
    diceCount: 1,
    diceSides: 6,
    faces: [],
    flat: 3,
    sum: 10,
    counted: true,
    damageTypeKey: 'damage-type:piercing',
    rerolled: [],
    ...over,
  }) as never;

const types = new Map([
  ['damage-type:piercing', 'perfurante'],
  ['damage-type:cold', 'frio'],
]);

describe('damageRows', () => {
  const group = [
    roll({}),
    roll({ partKey: 'sneak-attack', labelPt: 'Ataque Furtivo', diceCount: 3, flat: 0, sum: 0 }),
    roll({
      partKey: 'frost',
      labelPt: 'Geada no Punho',
      diceCount: 2,
      diceSides: 8,
      faces: [3, 3],
      flat: 0,
      sum: 6,
      damageTypeKey: 'damage-type:cold',
    }),
  ];

  it('heads a typed group with its dice, the sum typed for it and the fixed number, and lists its parts under it', () => {
    const rows = damageRows(group, types);
    expect(rows.map((r) => [r.kind, r.label])).toEqual([
      ['group', 'Perfurante'],
      ['child', 'Espada curta'],
      ['child', 'Ataque Furtivo'],
      ['part', 'Geada no Punho'],
    ]);
    expect(rows[0].formula).toBe('4d6 (10) + 3 · uma soma digitada');
    expect(rows[0].total).toBe(13);
    expect(rows[1].formula).toBe('1d6 + 3');
    expect(rows[2].formula).toBe('3d6');
  });

  it('leaves a part alone in its type as a plain row', () => {
    const rows = damageRows([group[0], group[2]], types);
    expect(rows.map((r) => r.kind)).toEqual(['part', 'part']);
  });
});
