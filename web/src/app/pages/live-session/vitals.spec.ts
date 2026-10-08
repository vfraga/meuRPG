import { brisaVitals, pensantusVitals } from './testing';
import {
  applySnapshot,
  applyVitals,
  hitPointsPercent,
  hitPointsState,
  partyRowSub,
  usedWords,
  whoSeesTheChange,
} from './vitals';

describe('applySnapshot', () => {
  it('takes the snapshot as the list of characters', () => {
    const next = applySnapshot([pensantusVitals(), brisaVitals()], [brisaVitals()]);
    expect(next.map((v) => v.characterId)).toEqual(['brisa']);
  });

  it('keeps a change that arrived before the snapshot (larger revision)', () => {
    const early = pensantusVitals({ revision: 5, hitPointsCurrent: 3 });
    const next = applySnapshot([early], [pensantusVitals({ revision: 4 })]);
    expect(next[0].hitPointsCurrent).toBe(3);
  });

  it('takes the snapshot when it is as new or newer', () => {
    const next = applySnapshot(
      [pensantusVitals({ revision: 4, hitPointsCurrent: 3 })],
      [pensantusVitals({ revision: 4, hitPointsCurrent: 10 })],
    );
    expect(next[0].hitPointsCurrent).toBe(10);
  });
});

describe('applyVitals', () => {
  it('applies a change only if its revision is newer', () => {
    const list = [pensantusVitals({ revision: 3 })];
    expect(
      applyVitals(list, pensantusVitals({ revision: 4, hitPointsCurrent: 9 }))[0].hitPointsCurrent,
    ).toBe(9);
    expect(
      applyVitals(list, pensantusVitals({ revision: 3, hitPointsCurrent: 9 }))[0].hitPointsCurrent,
    ).toBe(17);
    expect(
      applyVitals(list, pensantusVitals({ revision: 2, hitPointsCurrent: 9 }))[0].hitPointsCurrent,
    ).toBe(17);
  });

  it("drops the familiar's sight when the same revision arrives without it", () => {
    const sight = { creatureId: 'nanquim', inCombat: true };
    const list = [pensantusVitals({ revision: 5, familiarSight: sight })];
    expect(
      applyVitals(list, pensantusVitals({ revision: 5, familiarSight: null }))[0].familiarSight,
    ).toBeNull();
    // The same revision with the sight still on changes nothing, and an older copy never takes it away.
    expect(
      applyVitals(list, pensantusVitals({ revision: 5, familiarSight: sight }))[0].familiarSight,
    ).toEqual(sight);
    expect(
      applyVitals(list, pensantusVitals({ revision: 4, familiarSight: null }))[0].familiarSight,
    ).toEqual(sight);
  });

  it('adds a character that was not on screen', () => {
    expect(applyVitals([pensantusVitals()], brisaVitals()).map((v) => v.name)).toEqual([
      'Pensantus',
      'Brisa',
    ]);
  });
});

describe('hit point words', () => {
  it('fills the bar by the share of the maximum', () => {
    expect(hitPointsPercent({ hitPointsCurrent: 12, hitPointsMax: 24 })).toBe(50);
    expect(hitPointsPercent({ hitPointsCurrent: 0, hitPointsMax: 0 })).toBe(0);
  });

  it('says "Abaixo da metade" when current × 2 < max, and "Inconsciente" at 0', () => {
    expect(hitPointsState({ hitPointsCurrent: 12, hitPointsMax: 24 })).toBeNull();
    expect(hitPointsState({ hitPointsCurrent: 11, hitPointsMax: 24 })).toEqual({
      label: 'Abaixo da metade',
      tone: 'pending',
    });
    expect(hitPointsState({ hitPointsCurrent: 0, hitPointsMax: 24 })).toEqual({
      label: 'Inconsciente',
      tone: 'danger',
    });
  });

  it('counts with "usados"', () => {
    expect(usedWords(1, 3)).toBe('1 de 3 usados');
  });
});

describe('party row words', () => {
  it('names the class and the player', () => {
    expect(partyRowSub({ classSummary: 'Ladina 3', playerName: 'Ana' })).toBe('Ladina 3, de Ana');
  });

  it('never shows an empty name', () => {
    expect(partyRowSub({ classSummary: 'Mago 3', playerName: null })).toBe('Mago 3');
    expect(whoSeesTheChange(null, 'Pensantus')).toBe(
      'Quem joga com Pensantus vê a mudança na hora.',
    );
    expect(whoSeesTheChange('Ana', 'Brisa')).toBe('Ana vê a mudança na hora.');
  });
});
