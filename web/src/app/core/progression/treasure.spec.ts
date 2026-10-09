import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';

import {
  TreasureToConvertSchema,
  XPAwardMode,
  XPAwardSchema,
} from '../../../gen/meurpg/progression/v1/progression_pb';
import {
  awardTitle,
  foundLine,
  moreTreasures,
  foundWhen,
  stripHeadline,
  stripLine,
  totalPo,
  townCalc,
  townGivenText,
  townTitle,
  townUndoneText,
  treasureCount,
} from './treasure';

const nbsp = '\u00a0';
const NOW = new Date(2026, 9, 4, 23, 0);

function treasure(
  name: string,
  valuePo: number,
  finders: string[],
  at = new Date(2026, 9, 4, 21, 40),
  inSession = true,
) {
  return create(TreasureToConvertSchema, {
    pointId: name,
    name,
    valuePo,
    foundAt: timestampFromDate(at),
    foundBy: finders.map((f) => ({ characterId: f, characterName: f })),
    foundInSession: inSession,
  });
}

const CHEST = treasure('Baú de moedas', 250, ['Brisa']);
const PURSE = treasure('Bolsa do capitão', 120, ['Toren'], new Date(2026, 9, 4, 22, 2));
const IDOL = treasure('Ídolo de prata', 50, ['Pensantus', 'Sálvia'], new Date(2026, 9, 4, 22, 6));

describe('treasure texts (E9-09)', () => {
  it('counts treasures in the singular and the plural', () => {
    expect(treasureCount(1)).toBe('1\u00a0tesouro');
    expect(treasureCount(3)).toBe('3\u00a0tesouros');
    expect(treasureCount(1200)).toBe('1.200\u00a0tesouros');
  });

  it("sums the PO and writes the strip's headline and lines", () => {
    expect(totalPo([CHEST, PURSE, IDOL])).toBe(420);
    expect(stripHeadline([CHEST, PURSE, IDOL])).toBe(`3\u00a0tesouros · 420${nbsp}PO`);
    expect(stripLine(CHEST)).toBe(`Baú de moedas, 250${nbsp}PO, de Brisa`);
    expect(stripLine(IDOL)).toBe(`Ídolo de prata, 50${nbsp}PO, de Pensantus e Sálvia`);
  });

  it('says who found it and when: the clock for today, the day for an older find', () => {
    expect(foundLine(CHEST, NOW)).toBe(`Encontrado por Brisa às${nbsp}21:40`);
    const old = treasure('Anel', 10, ['Toren'], new Date(2026, 9, 2, 20, 48));
    expect(foundWhen(old.foundAt, NOW)).toBe(`em 02/10${nbsp}às${nbsp}20:48`);
    // A find whose finders were deleted still has a line.
    expect(foundLine(treasure('Anel', 10, []), NOW)).toBe(`Encontrado às${nbsp}21:40`);
  });

  describe('the calculation', () => {
    it('writes the sum, the division and what is left (420 PO between 4: nothing left)', () => {
      const calc = townCalc(3, 420, 4);
      expect(calc.sum).toBe(`420${nbsp}PO em 3\u00a0tesouros = 420${nbsp}XP`);
      expect(calc.big).toBe(`420${nbsp}XP ÷ 4 = 105${nbsp}XP para cada`);
      expect(calc.left).toBe(`Sobra 0${nbsp}XP.`);
      expect(calc.split).toEqual({ count: 4, each: 105, lost: 0 });
    });

    it('names the leftover: 370 PO between 3 is 123 each and 1 XP goes to nobody', () => {
      const calc = townCalc(2, 370, 3);
      expect(calc.big).toBe(`370${nbsp}XP ÷ 3 = 123${nbsp}XP para cada`);
      expect(calc.left).toBe(`Sobra 1${nbsp}XP, que não vai para ninguém.`);
    });

    it('rounds down, as the server does: 250 PO between 3 is 83 each, 1 left', () => {
      const calc = townCalc(1, 250, 3);
      expect(calc.split).toEqual({ count: 3, each: 83, lost: 1 });
      expect(calc.sum).toBe(`250${nbsp}PO em 1\u00a0tesouro = 250${nbsp}XP`);
    });

    it('gives nothing for nobody', () => {
      expect(townCalc(0, 0, 0).split.each).toBe(0);
    });
  });

  describe('the history', () => {
    const town = create(XPAwardSchema, {
      id: 'a',
      reason: 'Voltar à cidade',
      gold: 420,
      treasureCount: 3,
    });

    it('titles a "Voltar à cidade" award by its treasures, and any other by its reason', () => {
      expect(townTitle(3, 420)).toBe(`Voltar à cidade · 420${nbsp}PO em 3\u00a0tesouros`);
      expect(awardTitle(town)).toBe(`Voltar à cidade · 420${nbsp}PO em 3\u00a0tesouros`);
      expect(
        awardTitle(create(XPAwardSchema, { reason: 'Venda do cálice de prata', gold: 60 })),
      ).toBe('Venda do cálice de prata');
    });

    it('titles a milestone award that came without its text just "Marco"', () => {
      expect(
        awardTitle(
          create(XPAwardSchema, { mode: XPAwardMode.XP_AWARD_MODE_MILESTONE, undone: true }),
        ),
      ).toBe('Marco');
    });

    it('says what the master reads after giving and after undoing', () => {
      const shares = [
        { characterId: 'p', characterName: 'Pensantus', xp: 105 },
        { characterId: 't', characterName: 'Toren', xp: 105 },
      ];
      const award = create(XPAwardSchema, {
        id: 'a',
        reason: 'Voltar à cidade',
        gold: 420,
        treasureCount: 3,
        shares,
      });
      expect(townGivenText(award, 105, 0)).toBe(
        `Voltar à cidade: Pensantus e Toren receberam 105${nbsp}XP cada. Os 3\u00a0tesouros foram convertidos.`,
      );
      expect(townGivenText(award, 105, 1)).toContain(`Sobra 1${nbsp}XP, que não vai para ninguém.`);
      expect(townUndoneText(award)).toBe(
        'XP desfeito: os 3\u00a0tesouros voltaram a “encontrado, não convertido”.',
      );
      expect(townUndoneText(create(XPAwardSchema, { treasureCount: 1 }))).toBe(
        'XP desfeito: o tesouro voltou a “encontrado, não convertido”.',
      );
    });
  });

  describe("the preview agrees with the server's split (backend/internal/rules TestSplitXP, and its remainders)", () => {
    // Each row: PO total, receivers, what each gets and what is left: the numbers the Go tests give for
    // SplitXP (100/4 = 25, 100/3 = 33 r1, 7/1 = 7, 1/2 = 0 r1) plus ones with a remainder of 2 and 3.
    it.each([
      [100, 4, 25, 0],
      [100, 3, 33, 1],
      [7, 1, 7, 0],
      [1, 2, 0, 1],
      [170, 3, 56, 2],
      [419, 4, 104, 3],
      [370, 3, 123, 1],
    ])('%i PO between %i: %i each, %i left', (po, who, each, lost) => {
      const { split } = townCalc(2, po, who);
      expect(split).toEqual({ count: who, each, lost });
      expect(split.each * who + split.lost).toBe(po);
    });
  });

  it('agrees in number, with the count tied to its noun', () => {
    expect(moreTreasures(1, 'para depois')).toBe(
      `Há mais 1${nbsp}tesouro encontrado, que fica para depois.`,
    );
    expect(moreTreasures(29, 'para a próxima vez')).toBe(
      `Há mais 29${nbsp}tesouros encontrados, que ficam para a próxima vez.`,
    );
  });
});
