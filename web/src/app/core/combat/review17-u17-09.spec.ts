import { CombatantKind } from '../../../gen/meurpg/play/v1/combat_pb';
import { CombatState } from './combat-state';
import { combatant, encounter } from './combat-testing';
import { turnBanner } from './combat-view';
import { acts, turnMembers } from './joint-turn';

const P = CombatantKind.PLAYER;

// Finding U17-9 (review/unit-17-contract.md)
describe('Review17 U17-9: turn_changed leaves the previous turn group in place', () => {
  const brisa = combatant({ id: 'b', label: 'Brisa', kind: P, initiative: 19 });
  const toren = combatant({ id: 't', label: 'Toren', kind: P, initiative: 19 });
  const pens = combatant({ id: 'p', label: 'Pensantus', kind: P, initiative: 14, mine: true });

  function stateOnJointTurn(): CombatState {
    const state = new CombatState();
    state.apply(
      encounter({
        combatants: [brisa, toren, pens],
        currentCombatantId: 'b',
        turnGroupIds: ['b', 't'],
        revision: 5,
      }),
    );
    return state;
  }

  it('turn members follow currentCombatantId after applyTurn', () => {
    const state = stateOnJointTurn();
    expect(state.applyTurn({ encounterId: 'enc', round: 2, currentCombatantId: 'p', masterTurn: false })).toBe(true);
    const e = state.encounter()!;
    expect(turnMembers(e).map((c) => c.id)).toEqual(['p']);
  });

  it('the next player acts and the banner names them, before any reload', () => {
    const state = stateOnJointTurn();
    state.applyTurn({ encounterId: 'enc', round: 2, currentCombatantId: 'p', masterTurn: false });
    const e = state.encounter()!;
    expect(acts(e, pens)).toBe(true);
    expect(acts(e, brisa)).toBe(false);
    expect(turnBanner(e).title).toBe('Vez de Pensantus');
  });
});
