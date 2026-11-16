import { combatant } from './combat-testing';
import { combatPool, ownPool, poolPercent, vitalsPool } from './form-pool';

describe('form pool (584-02, 584-04, 584-05)', () => {
  const druid = {
    hitPointsCurrent: 27,
    hitPointsMax: 27,
    wildShapeBeastKey: 'monster:wolf',
    wildShapeHitPointsCurrent: 7,
    wildShapeHitPointsMax: 11,
  };

  it("plays with the beast's pool in a form and with the character's own out of one", () => {
    expect(combatPool(combatant({ id: 's', label: 'Sálvia', ...druid }))).toEqual({
      current: 7,
      max: 11,
    });
    expect(
      combatPool(combatant({ id: 's', label: 'Sálvia', hitPointsCurrent: 27, hitPointsMax: 27 })),
    ).toEqual({
      current: 27,
      max: 27,
    });
  });

  it('has no pool for a viewer who has no numbers (RN-20)', () => {
    expect(combatPool(combatant({ id: 's', label: 'Sálvia' }))).toBeNull();
  });

  it("keeps the druid's own pool for the one line that says it, only in a form", () => {
    expect(ownPool(combatant({ id: 's', label: 'Sálvia', ...druid }))).toEqual({
      current: 27,
      max: 27,
    });
    expect(
      ownPool(combatant({ id: 's', label: 'Sálvia', hitPointsCurrent: 27, hitPointsMax: 27 })),
    ).toBeNull();
  });

  it("reads the form of a player's own vitals", () => {
    const v = { hitPointsCurrent: 27, hitPointsMax: 27 };
    expect(vitalsPool({ ...v, wildShape: { hitPointsCurrent: 7, hitPointsMax: 11 } })).toEqual({
      current: 7,
      max: 11,
    });
    expect(vitalsPool({ ...v, wildShape: null })).toEqual({ current: 27, max: 27 });
  });
});

describe('poolPercent', () => {
  it('is how full the pool is, capped at 100, and 0 with no pool or an empty one', () => {
    expect(poolPercent({ hitPointsMax: 20, hitPointsCurrent: 5 } as never)).toBe(25);
    expect(poolPercent({ hitPointsMax: 20, hitPointsCurrent: 30 } as never)).toBe(100);
    expect(poolPercent({ hitPointsMax: 0, hitPointsCurrent: 0 } as never)).toBe(0);
    expect(poolPercent({} as never)).toBe(0);
  });
});
