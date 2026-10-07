// Finding U13-03 (review/unit-13-web-live-rest.md): out-of-order stage answers for two different NPCs overwrite the newer stage with the older one.
import { SceneState } from './scene-state';
import { masterScene, stageNpc } from './scene-testing';
import { StageController, type StageApi } from './stage-controller';
import type { StageNpc } from '../../../gen/meurpg/play/v1/scene_pb';

describe('Review13 U13-03: stale stage answer overwrites a newer one', () => {
  function deferred() {
    let resolve: (s: readonly StageNpc[]) => void = () => undefined;
    const promise = new Promise<readonly StageNpc[]>((r) => (resolve = r));
    return { promise, resolve };
  }

  function setup() {
    const a = stageNpc('sa', 'Aldo', { master: true, characterId: 'a' });
    const b = stageNpc('sb', 'Bia', { master: true, characterId: 'b' });
    const pending = [deferred(), deferred()];
    let n = 0;
    const api: StageApi = {
      putOnStage: () => Promise.reject(new Error('unused')),
      takeOffStage: () => Promise.reject(new Error('unused')),
      setSpeaker: () => pending[n++].promise,
    };
    const state = new SceneState(
      () => Promise.resolve(masterScene([], [a, b])),
      () => true,
    );
    state.apply(masterScene([], [a, b]));
    const ctl = new StageController(
      api,
      state,
      () => 'c1',
      (id) => id,
    );
    return { a, b, pending, state, ctl };
  }

  it('control: answers in order leave the later state', async () => {
    const { a, b, pending, state, ctl } = setup();
    const first = ctl.speak('a', false);
    const second = ctl.speak('b', false);
    pending[0].resolve([{ ...a, speaking: true }, b]);
    await first;
    pending[1].resolve([
      { ...a, speaking: false },
      { ...b, speaking: true },
    ]);
    await second;
    expect(state.stage().map((n) => n.speaking)).toEqual([false, true]);
  });

  it('keeps the later server state when the earlier answer arrives last', async () => {
    const { a, b, pending, state, ctl } = setup();
    const first = ctl.speak('a', false);
    const second = ctl.speak('b', false);
    // Server applied A then B; B's answer arrives first, A's answer last.
    pending[1].resolve([
      { ...a, speaking: false },
      { ...b, speaking: true },
    ]);
    await second;
    pending[0].resolve([{ ...a, speaking: true }, b]);
    await first;
    expect(state.stage().map((n) => n.speaking)).toEqual([false, true]);
  });
});
