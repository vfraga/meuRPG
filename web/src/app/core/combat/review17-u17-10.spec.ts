import { CombatState } from './combat-state';
import { encounter } from './combat-testing';

// Finding U17-10 (review/unit-17-contract.md)
describe('Review17 U17-10: a fog player revision lower than the last global revision freezes the combat screen', () => {
  it('after the master turns the fog on, the player copy (revision = visible events) replaces the one at the global revision', () => {
    const state = new CombatState();
    // No fog: the player reads the encounter's global revision (40 changes so far).
    state.apply(encounter({ revision: 40, round: 2 }));
    // Fog switched on: the server answers GetEncounter with visibleRevision (events the
    // player could see, backend combat_view.go:462), here 3, and the round moved on.
    state.apply(encounter({ revision: 3, round: 3 }));
    expect(state.encounter()?.round).toBe(3);
  });
});
