import type { TableEffect } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { signed } from '../effects/effects';
import { RECHARGE_WORDS, type ReadRow } from './content-read-parts';

/** What a player reads of the power attack of an entry, and the effects the page still lists after it. */
export interface PowerAttackRead {
  /** "Ao atacar" and, when the trade spends a pool the entry defines, "Usos"; none without a power attack. */
  readonly rows: readonly ReadRow[];
  /** The entry's effects without the pool the "Usos" row already says, so a pool is read once. */
  readonly rest: readonly TableEffect[];
}

const WHOLE = /^\d+$/;

/** The words of the board for when the uses come back (the sentence of a line says "num"; the row says "no"). */
const BACK: Readonly<Record<string, string>> = {
  ...RECHARGE_WORDS,
  short_rest: 'voltam no descanso curto',
  long_rest: 'voltam no descanso longo',
};

/**
 * The rows of "Como os jogadores veem" for a power attack: "Ao atacar: −5 no ataque, +10 no dano" and "Usos: 3 · voltam no descanso curto".
 * Numbers outside the ranges (a stored effect the editor keeps as it is) draw nothing: they are not a trade a player can make.
 */
export function powerAttackRead(effects: readonly TableEffect[]): PowerAttackRead {
  const p = effects.find((e) => e.type === 'power_attack');
  if (!p || p.attackPenalty < 1 || p.damageBonus < 1) {
    return { rows: [], rest: effects };
  }
  const rows: ReadRow[] = [
    {
      label: 'Ao atacar',
      value: `${signed(-p.attackPenalty)} no ataque, ${signed(p.damageBonus)} no dano`,
    },
  ];
  const name = p.resource.trim();
  const pool =
    name === ''
      ? undefined
      : effects.find(
          (e) => e.type === 'resource' && e.resource.trim() === name && WHOLE.test(e.max),
        );
  if (pool) {
    const back = BACK[pool.recharge];
    rows.push({ label: 'Usos', value: back ? `${pool.max} · ${back}` : pool.max });
  }
  return { rows, rest: pool ? effects.filter((e) => e !== pool) : effects };
}
