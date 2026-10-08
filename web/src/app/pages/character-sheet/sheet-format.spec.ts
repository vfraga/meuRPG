import {
  coinEntries,
  formatDate,
  issueTitle,
  spellLimitsText,
  pactSlotRow,
  spellSlotRows,
  stateTagLabel,
} from './sheet-format';

describe('sheet-format', () => {
  it('formats a date as the day only, local time', () => {
    expect(formatDate(new Date(2026, 8, 29, 19, 55))).toBe('29/09/2026');
    expect(formatDate(new Date(2026, 0, 3))).toBe('03/01/2026');
  });

  it('names each state with one word', () => {
    expect(stateTagLabel('draft')).toBe('Rascunho');
    expect(stateTagLabel('pending')).toBe('Pendente');
    expect(stateTagLabel('locked')).toBe('Travada');
    expect(stateTagLabel('dead')).toBe('Morto');
  });

  it('titles an issue from its stable code, never from its message', () => {
    const issue = (code: string, field = '') => ({ code, field, message: 'qualquer texto' });
    expect(issueTitle(issue('armor_proficiency', 'full.armor_key'))).toBe(
      'Armadura sem proficiência.',
    );
    expect(issueTitle(issue('armor_proficiency', 'full.shield'))).toBe('Escudo sem proficiência.');
    expect(issueTitle(issue('spell_not_on_list', 'full.prepared_spell_keys[0]'))).toBe(
      'Magia fora do grimório.',
    );
    expect(issueTitle(issue('spell_not_on_list', 'full.cantrip_keys[1]'))).toBe(
      'Magia fora da lista.',
    );
    expect(issueTitle(issue('skill_count'))).toBe('Perícias a revisar.');
    expect(issueTitle(issue('missing', 'full.background_key'))).toBe('Falta uma escolha.');
    // A code this app does not know yet still gets a title.
    expect(issueTitle(issue('something_new'))).toBe('Pendência de regra.');
  });

  it('lists one slot row per level that has slots, with an accessible count', () => {
    expect(spellSlotRows([4, 0, 1])).toEqual([
      { level: 1, label: '1º nível', count: 4, countLabel: '4 espaços' },
      { level: 3, label: '3º nível', count: 1, countLabel: '1 espaço' },
    ]);
    expect(spellSlotRows([])).toEqual([]);
  });

  it("makes one row of a Warlock's pact slots, and none without them", () => {
    expect(pactSlotRow({ level: 2, count: 2 })).toEqual({
      level: 2,
      label: '2º nível',
      count: 2,
      countLabel: '2 espaços',
    });
    expect(pactSlotRow({ level: 1, count: 0 })).toBeNull();
    expect(pactSlotRow(null)).toBeNull();
  });

  it('says the class spell limits in one sentence', () => {
    const sc = { className: 'Mago', ability: 'int' as const, saveDc: 14, attackBonus: 6 };
    expect(spellLimitsText({ ...sc, cantripsKnown: 3, spellsPreparedMax: 7 })).toBe(
      'Até 3 truques e 7 magias preparadas.',
    );
    expect(spellLimitsText({ ...sc, cantripsKnown: 0, spellsPreparedMax: 1 })).toBe(
      'Até 1 magia preparada.',
    );
    expect(spellLimitsText({ ...sc, cantripsKnown: 1, spellsPreparedMax: 0 })).toBe(
      'Até 1 truque.',
    );
    expect(spellLimitsText({ ...sc, cantripsKnown: 0, spellsPreparedMax: 0 })).toBe('');
  });

  it('lists only the coins carried, platinum first', () => {
    expect(coinEntries({ cp: 0, sp: 0, ep: 0, gp: 0, pp: 0 })).toEqual([]);
    expect(
      coinEntries({ cp: 5, sp: 0, ep: 0, gp: 15, pp: 0 }).map(
        (c) => `${c.amount} ${c.abbreviation}`,
      ),
    ).toEqual(['15 PO', '5 PC']);
  });
});
