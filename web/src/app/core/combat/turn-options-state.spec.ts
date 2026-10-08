import type { GetTurnOptionsResponse } from '../../../gen/meurpg/play/v1/combat_pb';
import type { CombatClient } from './combat-client';
import { TurnOptionsState } from './turn-options-state';

function api(answers: Promise<GetTurnOptionsResponse>[]): CombatClient {
  let i = 0;
  return { turnOptions: () => answers[i++] } as unknown as CombatClient;
}

describe('TurnOptionsState', () => {
  it('keeps the answer to the newest question', async () => {
    const state = new TurnOptionsState();
    let slow!: (r: GetTurnOptionsResponse) => void;
    const first = new Promise<GetTurnOptionsResponse>((resolve) => (slow = resolve));
    const fast = Promise.resolve({ yourTurn: true } as GetTurnOptionsResponse);
    const client = api([first, fast]);
    const a = state.load(client, 'c', 'e', 'x');
    await state.load(client, 'c', 'e', 'x');
    slow({ yourTurn: false } as GetTurnOptionsResponse);
    await a;
    expect(state.data()?.yourTurn).toBe(true);
  });

  it('keeps what it had when a question fails, and forgets on clear', async () => {
    const state = new TurnOptionsState();
    await state.load(
      api([Promise.resolve({ yourTurn: true } as GetTurnOptionsResponse)]),
      'c',
      'e',
      'x',
    );
    await state.load(api([Promise.reject(new Error('down'))]), 'c', 'e', 'x');
    expect(state.data()?.yourTurn).toBe(true);
    state.clear();
    expect(state.data()).toBeNull();
  });

  it("does not show the last combatant's options while the next one's come", async () => {
    const state = new TurnOptionsState();
    const optionsA = { yourTurn: true } as GetTurnOptionsResponse;
    const client = {
      turnOptions: (_c: string, _e: string, id: string) =>
        id === 'A' ? Promise.resolve(optionsA) : new Promise<GetTurnOptionsResponse>(() => {}),
    } as unknown as CombatClient;
    await state.load(client, 'c', 'e', 'A');
    expect(state.data()).toBe(optionsA);
    void state.load(client, 'c', 'e', 'B');
    expect(state.data()).toBeNull();
  });

  it("does not keep the last combatant's options when the next one's read fails", async () => {
    const state = new TurnOptionsState();
    await state.load(
      api([Promise.resolve({ yourTurn: true } as GetTurnOptionsResponse)]),
      'c',
      'e',
      'A',
    );
    await state.load(api([Promise.reject(new Error('down'))]), 'c', 'e', 'B');
    expect(state.data()).toBeNull();
  });
});
