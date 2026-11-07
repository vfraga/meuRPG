import {
  acAndHp,
  alignmentPt,
  capitalized,
  creatureKeyOf,
  creatureSlug,
  listWithE,
  npcSpeedFt,
  typeAndSize,
  typeAndSizeInWords,
} from './bestiary-format';
import { challengeBounds } from './creature-types';
import { ogre, summary } from './creatures-testing';

describe("the bestiary's words (MR-042)", () => {
  it('a row says the type and size, and CA and PV, as the artboard does', () => {
    const wolf = summary('monster:wolf', 'Lobo', {
      typePt: 'fera',
      sizePt: 'Médio',
      armorClass: 13,
      hitPoints: 11,
    });
    expect(typeAndSize(wolf).replace(/\u00a0/g, ' ')).toBe('Fera · Médio');
    expect(acAndHp(wolf).replace(/\u00a0/g, ' ')).toBe('CA 13 · PV 11');
    expect(capitalized('')).toBe('');
  });

  it('says the type and the size in words, the size agreeing with the type, and falls back to the row form', () => {
    const say = (typePt: string, sizePt: string) => typeAndSizeInWords({ typePt, sizePt });
    expect(say('fera', 'Médio')).toBe('fera média');
    expect(say('monstruosidade', 'Enorme')).toBe('monstruosidade enorme');
    expect(say('fada', 'Miúdo')).toBe('fada miúda');
    expect(say('morto-vivo', 'Médio')).toBe('morto-vivo médio');
    expect(say('dragão', 'Imenso')).toBe('dragão imenso');
    expect(say('Gigante', 'Grande')).toBe('gigante grande');
    expect(say('construto', 'Médio')).toBe('construto médio');
    expect(say('ínfero', 'Pequeno')).toBe('ínfero pequeno');
    expect(say('gosma', 'Grande')).toBe('gosma grande');
    expect(say('gosma', 'Médio')).toBe('gosma média');
    expect(say('enxame de feras miúdas', 'Médio').replace(/\u00a0/g, ' ')).toBe(
      'Enxame de feras miúdas · Médio',
    );
    expect(say('coisa', 'Médio').replace(/\u00a0/g, ' ')).toBe('Coisa · Médio');
    expect(say('fera', 'Colossal').replace(/\u00a0/g, ' ')).toBe('Fera · Colossal');
  });

  it("the alignment is Portuguese when the table knows it, and the book's English (marked) when not", () => {
    expect(alignmentPt('chaotic evil')).toEqual({ text: 'caótico e mau', english: false });
    expect(alignmentPt('Unaligned')).toEqual({ text: 'sem alinhamento', english: false });
    expect(alignmentPt('neutral good (50%) or neutral evil (50%)')).toEqual({
      text: 'neutral good (50%) or neutral evil (50%)',
      english: true,
    });
    expect(alignmentPt('')).toEqual({ text: '', english: false });
  });

  it('the route uses the key without its prefix', () => {
    expect(creatureSlug('monster:giant-wolf-spider')).toBe('giant-wolf-spider');
    expect(creatureKeyOf('ogre')).toBe('monster:ogre');
  });

  it("an NPC walks at the creature's speed, or at its best one when it does not walk", () => {
    expect(npcSpeedFt(ogre())).toBe(40);
    expect(npcSpeedFt(ogre({ speedWalkFt: 0, speedSwimFt: 60, speedFlyFt: 30 }))).toBe(60);
  });

  it('lists names with "e" before the last', () => {
    expect(listWithE([])).toBe('');
    expect(listWithE(['Cimitarra'])).toBe('Cimitarra');
    expect(listWithE(['Cimitarra', 'Adaga'])).toBe('Cimitarra e Adaga');
    expect(listWithE(['A', 'B', 'C'])).toBe('A, B e C');
  });

  it('the ND filter is one rating or a range', () => {
    expect(challengeBounds('')).toEqual({ minCr: '', maxCr: '' });
    expect(challengeBounds('1/4')).toEqual({ minCr: '1/4', maxCr: '1/4' });
    expect(challengeBounds('1-4')).toEqual({ minCr: '1', maxCr: '4' });
    expect(challengeBounds('11-30')).toEqual({ minCr: '11', maxCr: '30' });
  });
});
