// Review12 finding U12-16: TurnOptionsState keeps the previous combatant's
// options while the next combatant's answer is pending or fails.
import type { GetTurnOptionsResponse } from '../../../gen/meurpg/play/v1/combat_pb';
import type { CombatClient } from './combat-client';
import { TurnOptionsState } from './turn-options-state';

describe('Review12 U12-16: TurnOptionsState keeps the previous combatant data', () => {
  const optionsA = { yourTurn: true, attacks: ['A-attack'] } as unknown as GetTurnOptionsResponse;

  it('does not show A options while B is being loaded', async () => {
    const state = new TurnOptionsState();
    const client = {
      turnOptions: (_c: string, _e: string, id: string) =>
        id === 'A' ? Promise.resolve(optionsA) : new Promise<GetTurnOptionsResponse>(() => {}),
    } as unknown as CombatClient;
    await state.load(client, 'c', 'e', 'A');
    expect(state.data()).toBe(optionsA);
    void state.load(client, 'c', 'e', 'B');
    expect(state.data()).not.toBe(optionsA);
  });

  it('does not show A options after the read for B fails', async () => {
    const state = new TurnOptionsState();
    const client = {
      turnOptions: (_c: string, _e: string, id: string) =>
        id === 'A' ? Promise.resolve(optionsA) : Promise.reject(new Error('down')),
    } as unknown as CombatClient;
    await state.load(client, 'c', 'e', 'A');
    await state.load(client, 'c', 'e', 'B');
    expect(state.data()).not.toBe(optionsA);
  });
});
