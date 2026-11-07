import type { TvCombatantVm } from '../../../core/play/table-screen-view';
import { orderRows } from './order-rows';

const who = (name: string, onTurn = false): TvCombatantVm => ({
  name,
  initial: name[0],
  stateWord: null,
  onTurn,
  isPlayer: true,
});
const names = (rows: ReturnType<typeof orderRows>['rows']) =>
  rows.map((r) => (r.kind === 'who' ? r.who.name : `--${r.label}`));

describe('orderRows', () => {
  it('puts the one on turn first and marks where the next round starts', () => {
    const { rows, more } = orderRows([who('Brisa'), who('Tor', true), who('Wight')], 3);
    expect(names(rows)).toEqual(['Tor', 'Wight', '--Rodada 4 começa', 'Brisa']);
    expect(more).toBe(0);
  });

  it('keeps the order when the first is on turn, with no divider', () => {
    expect(names(orderRows([who('Tor', true), who('Brisa')], 1).rows)).toEqual(['Tor', 'Brisa']);
  });

  it("keeps the server's order on the master's turn", () => {
    expect(names(orderRows([who('Brisa'), who('Tor')], 2).rows)).toEqual(['Brisa', 'Tor']);
  });

  it('cuts a long order with the count of those left out', () => {
    const many = Array.from({ length: 14 }, (_, i) => who(`P${i}`, i === 0));
    const { rows, more } = orderRows(many, 1, 10);
    expect(rows).toHaveLength(10);
    expect(more).toBe(4);
  });

  it('counts the divider apart from the cut', () => {
    const many = Array.from({ length: 12 }, (_, i) => who(`P${i}`, i === 3));
    const { rows, more } = orderRows(many, 1, 10);
    expect(rows.filter((r) => r.kind === 'who')).toHaveLength(10);
    expect(more).toBe(2);
  });
});
