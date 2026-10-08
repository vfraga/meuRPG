import { brisaVitals, pensantusVitals } from '../testing';
import { changeBetween, draftFrom, sameChange } from './adjust-vitals.types';

describe('changeBetween', () => {
  it('is null when nothing changed', () => {
    const v = pensantusVitals();
    expect(changeBetween(v, draftFrom(v))).toBeNull();
  });

  it('sends only what changed, as absolute values', () => {
    const v = pensantusVitals();
    const draft = { ...draftFrom(v), hitPointsCurrent: 12, slotsUsed: { 1: 3, 2: 0 } };
    expect(changeBetween(v, draft)).toEqual({
      hitPointsCurrent: 12,
      spellSlotsUsed: [{ level: 1, used: 3 }],
    });
  });

  it('covers temporary HP, hit dice and pact slots', () => {
    const v = brisaVitals({ pactSlots: { slotLevel: 2, total: 2, used: 0 } });
    const draft = { ...draftFrom(v), hitPointsTemporary: 0, hitDiceUsed: 2, pactSlotsUsed: 1 };
    expect(changeBetween(v, draft)).toEqual({
      hitPointsTemporary: 0,
      pactSlotsUsed: 1,
      hitDiceUsed: 2,
    });
  });
});

describe('sameChange', () => {
  it('tells a retry (same numbers) from a new correction', () => {
    expect(sameChange({ hitPointsCurrent: 12 }, { hitPointsCurrent: 12 })).toBe(true);
    expect(sameChange({ hitPointsCurrent: 12 }, { hitPointsCurrent: 11 })).toBe(false);
  });
});

describe('changeBetween for resources and a beast form', () => {
  const wild = pensantusVitals({
    resources: [
      { key: 'wild_shape', namePt: 'Forma Selvagem', total: 2, used: 2, recharge: 'short_rest' },
      { key: 'second_wind', namePt: 'Retomar o Fôlego', total: 1, used: 1, recharge: 'short_rest' },
    ],
    wildShape: {
      beastKey: 'monster:wolf',
      beastNamePt: 'Lobo',
      hitPointsCurrent: 9,
      hitPointsMax: 11,
    },
  });

  it('draws its draft from the resources and the beast', () => {
    expect(draftFrom(wild)).toMatchObject({
      resourcesUsed: { wild_shape: 2, second_wind: 1 },
      wildShapeHitPoints: 9,
    });
    expect(draftFrom(pensantusVitals()).wildShapeHitPoints).toBeNull();
  });

  it('is null when no resource and no beast number moved', () => {
    expect(changeBetween(wild, draftFrom(wild))).toBeNull();
  });

  it('gives back one resource use and leaves the other resources alone', () => {
    const draft = { ...draftFrom(wild), resourcesUsed: { wild_shape: 1, second_wind: 1 } };
    expect(changeBetween(wild, draft)).toEqual({ resourcesUsed: [{ key: 'wild_shape', used: 1 }] });
  });

  it("sends the beast's hit points apart from the character's own", () => {
    const draft = { ...draftFrom(wild), wildShapeHitPoints: 0 };
    expect(changeBetween(wild, draft)).toEqual({ wildShapeHitPointsCurrent: 0 });
  });

  it('never sends beast hit points for a character in their own shape', () => {
    const v = pensantusVitals();
    expect(changeBetween(v, { ...draftFrom(v), wildShapeHitPoints: 5 })).toBeNull();
  });
});
