import { pensantusVitals } from '../../../core/play/live-testing';
import { MAX_PIPS, rechargeLine, resourceBoxes, slotCounterRows, slotsFootnote } from './counters';

describe('rechargeLine', () => {
  it('says when the uses come back, in the board words', () => {
    expect(rechargeLine('short_rest')).toBe('Volta num descanso curto ou longo');
    expect(rechargeLine('long_rest')).toBe('Volta num descanso longo');
  });

  it('writes nothing for the kinds the app has no rule for', () => {
    expect(rechargeLine('dawn')).toBe('');
    expect(rechargeLine('none')).toBe('');
  });
});

describe('resourceBoxes', () => {
  it('counts what is left of what there is, with the pips', () => {
    const [rage] = resourceBoxes([
      { key: 'rage', namePt: 'Fúria', total: 3, used: 1, recharge: 'long_rest' },
    ]);
    expect(rage).toEqual({
      key: 'rage',
      name: 'Fúria',
      count: '2 de 3',
      pips: { left: 2, total: 3 },
      again: 'Volta num descanso longo',
    });
  });

  it('shows only the number above 6 uses, and the pips up to 6', () => {
    const boxes = resourceBoxes([
      { key: 'ki', namePt: 'Chi', total: MAX_PIPS, used: 0, recharge: 'short_rest' },
      { key: 'ki2', namePt: 'Chi demais', total: MAX_PIPS + 1, used: 0, recharge: 'short_rest' },
      { key: 'loh', namePt: 'Cura pelas Mãos', total: 25, used: 0, recharge: 'long_rest' },
    ]);
    expect(boxes.map((b) => b.pips !== null)).toEqual([true, false, false]);
    expect(boxes[2].count).toBe('25 de 25');
  });

  it('says "ilimitado" for the 99 uses of an unlimited resource, with no pips', () => {
    const [rage] = resourceBoxes([
      { key: 'rage', namePt: 'Fúria', total: 99, used: 0, recharge: 'long_rest' },
    ]);
    expect(rage.count).toBe('ilimitado');
    expect(rage.pips).toBeNull();
  });

  it('leaves out a resource with no uses, never shows an internal key, and has none for old copies', () => {
    expect(
      resourceBoxes([{ key: 'x', namePt: 'X', total: 0, used: 0, recharge: 'long_rest' }]),
    ).toEqual([]);
    expect(
      resourceBoxes([{ key: 'feature:odd', namePt: '', total: 1, used: 0, recharge: 'none' }])[0]
        .name,
    ).toBe('Recurso');
    expect(resourceBoxes(undefined)).toEqual([]);
  });

  it('never counts less than none', () => {
    expect(
      resourceBoxes([{ key: 'a', namePt: 'A', total: 2, used: 5, recharge: 'none' }])[0].count,
    ).toBe('0 de 2');
  });
});

describe('slotCounterRows', () => {
  it('says each level as "3 de 4", the free ones', () => {
    const rows = slotCounterRows(pensantusVitals());
    expect(rows.map((r) => [r.label, r.count, r.created])).toEqual([
      ['1º nível', '2 de 4', ''],
      ['2º nível', '2 de 2', ''],
    ]);
  });

  it('says "criado" next to a level that has a slot Flexible Casting made', () => {
    const rows = slotCounterRows({
      spellSlots: [
        { level: 1, total: 4, used: 1, created: 1 },
        { level: 2, total: 3, used: 0, created: 2 },
      ],
      pactSlots: null,
    });
    expect(rows.map((r) => r.created)).toEqual(['criado', '2 criados']);
  });

  it('keeps the pact slots as a row of their own, after the levels', () => {
    const rows = slotCounterRows({
      spellSlots: [{ level: 1, total: 2, used: 0 }],
      pactSlots: { slotLevel: 3, total: 2, used: 1 },
    });
    expect(rows[1]).toMatchObject({ key: 'pact', label: 'Pacto · 3º nível', count: '1 de 2' });
  });
});

describe('slotsFootnote', () => {
  const slots = [{ level: 1, total: 2, used: 0 }];
  const pact = { slotLevel: 1, total: 1, used: 0 };

  it('says the long rest, and the warlock only for a character that has pact slots', () => {
    expect(slotsFootnote({ spellSlots: slots, pactSlots: null })).toBe(
      'Voltam num descanso longo.',
    );
    expect(slotsFootnote({ spellSlots: slots, pactSlots: pact })).toBe(
      'Voltam num descanso longo. (O Bruxo recupera os espaços de pacto num descanso curto ou longo.)',
    );
  });

  it('says only the pact for a warlock with no other slots, and nothing without slots', () => {
    expect(slotsFootnote({ spellSlots: [], pactSlots: pact })).toBe(
      'Os espaços de pacto voltam num descanso curto ou longo.',
    );
    expect(slotsFootnote({ spellSlots: [], pactSlots: null })).toBe('');
  });
});
