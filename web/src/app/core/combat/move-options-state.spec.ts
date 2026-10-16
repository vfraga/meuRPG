import { fakeCombatClient } from './combat-client-testing';
import { MoveOptionsState } from './move-options-state';

describe('MoveOptionsState', () => {
  function api() {
    const asked: { who: string; jump: unknown }[] = [];
    const answers: ((r: unknown) => void)[] = [];
    return {
      asked,
      answers,
      client: fakeCombatClient({
        moveOptions: (_c: string, _e: string, who: string, jump?: unknown) => {
          asked.push({ who, jump });
          return new Promise((resolve) => answers.push(resolve));
        },
      }),
    };
  }

  it("asks for the long jump's squares when a jump is given, and for the walk's otherwise", async () => {
    const fake = api();
    const state = new MoveOptionsState();
    const walk = state.load(fake.client, 'c', 'e', 't');
    fake.answers[0]({ id: 'walk' });
    await walk;
    const jump = state.load(fake.client, 'c', 'e', 't', { runningStart: true });
    fake.answers[1]({ id: 'jump' });
    await jump;
    expect(fake.asked).toEqual([
      { who: 't', jump: undefined },
      { who: 't', jump: { runningStart: true } },
    ]);
    expect(state.data()).toEqual({ id: 'jump' });
  });

  it("never reads a walk's answer as the jump's: the options are dropped when the way changes", async () => {
    const fake = api();
    const state = new MoveOptionsState();
    const walk = state.load(fake.client, 'c', 'e', 't');
    fake.answers[0]({ id: 'walk' });
    await walk;
    expect(state.data()).toEqual({ id: 'walk' });
    void state.load(fake.client, 'c', 'e', 't', { runningStart: false });
    expect(state.data()).toBeNull();
  });

  it('drops a slow answer that arrives after a newer question', async () => {
    const fake = api();
    const state = new MoveOptionsState();
    const slow = state.load(fake.client, 'c', 'e', 't');
    const fresh = state.load(fake.client, 'c', 'e', 't', { runningStart: true });
    fake.answers[1]({ id: 'jump' });
    await fresh;
    fake.answers[0]({ id: 'walk' });
    await slow;
    expect(state.data()).toEqual({ id: 'jump' });
  });
});
