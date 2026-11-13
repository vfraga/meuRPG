import {
  coinEntries,
  formatDate,
  formatWhen,
  grantedSpellTags,
  issueTitle,
  keepUnitsTogether,
  spellLimitsText,
  pactSlotRow,
  pendingTag,
  spellGroups,
  spellSlotRows,
  spellStateSummary,
  stateTagLabel,
} from './sheet-format';

describe('sheet-format', () => {
  it('words how a granted spell is cast: its cost and the waived material', () => {
    const g = {
      key: 'spell:bless',
      namePt: 'Bênção',
      level: 1,
      sourcePt: 'Graça de teste',
      atWill: false,
      resource: 'usos_da_graca',
      resourceNamePt: 'Usos da graça',
      alsoWithSlot: false,
      ritualOnly: false,
      countsAsKnown: false,
      noMaterialComponents: false,
      usesCost: 0,
      ability: null,
      abilityPending: false,
      saveDc: 0,
      attackBonus: 0,
    } as const;
    expect(grantedSpellTags(g)).toEqual(['Gasta um uso de Usos da graça']);
    expect(grantedSpellTags({ ...g, usesCost: 2, noMaterialComponents: true })).toEqual([
      'Gasta 2 usos de Usos da graça',
      'Sem material',
    ]);
    expect(grantedSpellTags({ ...g, atWill: true, resource: '' })).toEqual(['À vontade']);
  });

  it('keeps a number beside its unit, and only a unit', () => {
    expect(keepUnitsTogether('Uma luneta, um rolo de corda e 10 PO.')).toBe(
      'Uma luneta, um rolo de corda e 10\u00a0PO.',
    );
    expect(keepUnitsTogether('Corda de 15 m e 2 kg de ração, 3 pés de fio')).toBe(
      'Corda de 15\u00a0m e 2\u00a0kg de ração, 3\u00a0pés de fio',
    );
    expect(keepUnitsTogether('Carga de 90 kg (180 lb)')).toBe('Carga de 90\u00a0kg (180\u00a0lb)');
    // A word that only starts like a unit stays as it is.
    expect(keepUnitsTogether('5 mapas e 2 pontas, 10 POTES')).toBe('5 mapas e 2 pontas, 10 POTES');
  });

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

  it('titles every code the rules service sends', () => {
    const title = (code: string) => issueTitle({ code, field: '', message: 'x' });
    expect(
      [
        'spell_count',
        'spell_level',
        'expertise',
        'unknown_key',
        'subclass_level',
        'multiclass_prerequisite',
        'hit_point_rolls',
        'score_above_20',
        'level',
        'formula',
      ].map(title),
    ).toEqual([
      'Magias a revisar.',
      'Magias a revisar.',
      'Especialização a revisar.',
      'Escolha inválida.',
      'Subclasse cedo demais.',
      'Pré-requisito de multiclasse.',
      'Rolagens de pontos de vida.',
      'Habilidade acima de 20.',
      'Nível acima do máximo.',
      'Efeito ignorado.',
    ]);
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
    const limits = { cantripsKnown: 0, spellsPreparedMax: 0, spellsKnownMax: 0, spellbook: false };
    expect(spellLimitsText({ ...sc, ...limits, cantripsKnown: 3, spellsPreparedMax: 7 })).toBe(
      'Até 3 truques e 7 magias preparadas.',
    );
    expect(spellLimitsText({ ...sc, ...limits, spellsPreparedMax: 1 })).toBe(
      'Até 1 magia preparada.',
    );
    expect(spellLimitsText({ ...sc, ...limits, cantripsKnown: 1 })).toBe('Até 1 truque.');
    expect(spellLimitsText({ ...sc, ...limits })).toBe('');
  });

  it('calls the spells of a class that knows them "conhecidas", not "preparadas"', () => {
    const sorcerer = {
      className: 'Feiticeiro',
      ability: 'cha' as const,
      saveDc: 13,
      attackBonus: 5,
      cantripsKnown: 4,
      spellsPreparedMax: 0,
      spellsKnownMax: 5,
      spellbook: false,
    };
    expect(spellLimitsText(sorcerer)).toBe('Até 4 truques e 5 magias conhecidas.');
    expect(spellLimitsText({ ...sorcerer, cantripsKnown: 0, spellsKnownMax: 1 })).toBe(
      'Até 1 magia conhecida.',
    );
  });

  it('lists only the coins carried, platinum first', () => {
    expect(coinEntries({ cp: 0, sp: 0, ep: 0, gp: 0, pp: 0 })).toEqual([]);
    expect(
      coinEntries({ cp: 5, sp: 0, ep: 0, gp: 15, pp: 0 }).map(
        (c) => `${c.amount} ${c.abbreviation}`,
      ),
    ).toEqual(['15 PO', '5 PC']);
  });

  it('writes the day, the month and the hour of a review the way the sheet says it', () => {
    expect(formatWhen(new Date(2026, 9, 8, 21, 10))).toBe('8 de out., 21h10');
    expect(formatWhen(new Date(2026, 0, 31, 7, 5))).toBe('31 de jan., 7h05');
  });

  it("tags a pending character with where the master's review stands, in words and an icon", () => {
    const review = { reason: 'x', requestedAt: null, resubmittedAt: null };
    expect(pendingTag(null)).toEqual({ label: 'Pendente', icon: 'schedule' });
    expect(pendingTag({ ...review, status: 'awaiting' }).label).toBe('Pendente');
    expect(pendingTag({ ...review, status: 'changes_requested' })).toEqual({
      label: 'Pendente · ajustes pedidos',
      icon: 'edit',
    });
    expect(pendingTag({ ...review, status: 'resubmitted' })).toEqual({
      label: 'Pendente · reenviado',
      icon: 'task_alt',
    });
  });
});

describe('spellGroups', () => {
  const spell = (key: string, level: number, over: object = {}) => ({
    key,
    namePt: key,
    level,
    prepared: true,
    ritual: false,
    concentration: false,
    reaction: false,
    ...over,
  });
  const caster = (over: object) => ({
    className: 'Mago',
    ability: 'int' as const,
    saveDc: 13,
    attackBonus: 5,
    cantripsKnown: 3,
    spellsPreparedMax: 0,
    spellsKnownMax: 0,
    spellbook: false,
    ...over,
  });

  it('groups by level with cantrips first and tags in a fixed order', () => {
    const groups = spellGroups(
      [
        spell('b', 2),
        spell('a', 0),
        spell('c', 1, { reaction: true, concentration: true, ritual: true }),
      ],
      [caster({ spellsKnownMax: 5 })],
    );
    expect(groups.map((g) => g.label)).toEqual(['Truques', '1º nível', '2º nível']);
    expect(groups[1].rows[0].tags).toEqual(['Ritual', 'Concentração', 'Reação']);
    // A class that knows its spells has no preparation to say.
    expect(groups.flatMap((g) => g.rows.map((r) => r.state))).toEqual(['', '', '']);
    expect(spellStateSummary(groups)).toBe('');
  });

  it('tells prepared from the spellbook, and from known for a class without a book', () => {
    const spells = [spell('a', 1), spell('b', 1, { prepared: false }), spell('c', 0)];
    const book = spellGroups(spells, [caster({ spellsPreparedMax: 4, spellbook: true })]);
    expect(book[1].rows.map((r) => r.state)).toEqual(['Preparada', 'No grimório']);
    expect(book[0].rows[0].state).toBe('');
    expect(spellStateSummary(book)).toBe('1 preparada e 1 só no grimório.');
    const cleric = spellGroups(spells, [caster({ spellsPreparedMax: 4 })]);
    expect(cleric[1].rows.map((r) => r.state)).toEqual(['Preparada', 'Conhecida']);
    expect(spellStateSummary(cleric)).toBe('1 preparada e 1 só conhecida.');
  });
});
