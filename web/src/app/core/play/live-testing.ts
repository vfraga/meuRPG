import { VitalsVm } from './live-session.types';

/** Pensantus at the table (Mago 3: 23 HP, 4 + 2 slots, 3d6), for specs. */
export function pensantusVitals(overrides: Partial<VitalsVm> = {}): VitalsVm {
  return {
    characterId: 'pensantus',
    name: 'Pensantus',
    playerUserId: 'vinicius',
    hitPointsCurrent: 17,
    hitPointsMax: 23,
    hitPointsTemporary: 0,
    spellSlots: [
      { level: 1, total: 4, used: 2 },
      { level: 2, total: 2, used: 0 },
    ],
    pactSlots: null,
    hitDice: '3d6',
    hitDiceSizes: [{ faces: 6, total: 3, used: 1 }],
    hitDiceTotal: 3,
    hitDiceUsed: 1,
    revision: 1,
    ...overrides,
  };
}

/** Brisa (Ladina 3: 24 HP, no slots, 3d8), for specs. */
export function brisaVitals(overrides: Partial<VitalsVm> = {}): VitalsVm {
  return {
    characterId: 'brisa',
    name: 'Brisa',
    playerUserId: 'ana',
    hitPointsCurrent: 9,
    hitPointsMax: 24,
    hitPointsTemporary: 5,
    spellSlots: [],
    pactSlots: null,
    hitDice: '3d8',
    hitDiceSizes: [{ faces: 8, total: 3, used: 0 }],
    hitDiceTotal: 3,
    hitDiceUsed: 0,
    revision: 2,
    ...overrides,
  };
}
