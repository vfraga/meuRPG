import { create } from '@bufbuild/protobuf';

import { TableEffectSchema } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { powerAttackRead } from './power-attack-rows';

const power = (over: object = {}) =>
  create(TableEffectSchema, { type: 'power_attack', attackPenalty: 5, damageBonus: 10, ...over });
const pool = (over: object = {}) =>
  create(TableEffectSchema, {
    type: 'resource',
    resource: 'folego_do_atirador',
    max: '3',
    recharge: 'short_rest',
    ...over,
  });

describe('powerAttackRead: the rows a player reads of a power attack (the board of #586)', () => {
  it('says what the trade does to the roll and to the damage, with the true minus', () => {
    expect(powerAttackRead([power()]).rows).toEqual([
      { label: 'Ao atacar', value: '−5 no ataque, +10 no dano' },
    ]);
    expect(powerAttackRead([power({ attackPenalty: 1, damageBonus: 20 })]).rows[0].value).toBe(
      '−1 no ataque, +20 no dano',
    );
  });

  it('has no row for an entry without a power attack, and leaves its effects as they are', () => {
    const effects = [pool()];
    const read = powerAttackRead(effects);
    expect(read.rows).toEqual([]);
    expect(read.rest).toEqual(effects);
  });

  it('adds the uses of the pool the trade spends, and takes that pool off the effects the page lists', () => {
    const spent = pool();
    const other = pool({ resource: 'outro', max: '1', recharge: 'long_rest' });
    const read = powerAttackRead([power({ resource: 'folego_do_atirador' }), spent, other]);
    expect(read.rows).toEqual([
      { label: 'Ao atacar', value: '−5 no ataque, +10 no dano' },
      { label: 'Usos', value: '3 · voltam no descanso curto' },
    ]);
    expect(read.rest).toEqual([read.rest[0], other]);
    expect(read.rest).not.toContain(spent);
  });

  it('says when the uses come back in the words of the board', () => {
    const usos = (recharge: string) =>
      powerAttackRead([power({ resource: 'x' }), pool({ resource: 'x', recharge })]).rows.find(
        (r) => r.label === 'Usos',
      )?.value;
    expect(usos('long_rest')).toBe('3 · voltam no descanso longo');
    expect(usos('dawn')).toBe('3 · voltam ao amanhecer');
  });

  it('adds no uses row for a resource that is not a plain number of uses or is not defined here', () => {
    expect(
      powerAttackRead([power({ resource: 'x' }), pool({ resource: 'x', max: 'prof()' })]).rows,
    ).toHaveLength(1);
    expect(powerAttackRead([power({ resource: 'da_classe' })]).rows).toHaveLength(1);
  });

  it('draws nothing for numbers outside the ranges (the editor keeps those as stored)', () => {
    expect(powerAttackRead([power({ attackPenalty: 0 })]).rows).toEqual([]);
    expect(powerAttackRead([power({ damageBonus: 0 })]).rows).toEqual([]);
  });
});
