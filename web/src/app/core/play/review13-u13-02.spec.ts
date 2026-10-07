// Finding U13-02 (review/unit-13-web-live-rest.md): a local answer bumps `generation` and drops an in-flight authoritative read that carries a roll the answer lacks.
import type { OpenSceneInfo } from '../../../gen/meurpg/play/v1/scene_pb';
import { create } from '@bufbuild/protobuf';

import { SceneClueSchema } from '../../../gen/meurpg/maps/v1/maps_pb';
import { SceneState } from './scene-state';
import { masterScene, sceneRoll } from './scene-testing';

describe('Review13 U13-02: local answer drops an in-flight read with a new roll', () => {
  const before = create(SceneClueSchema, { id: 'k1', text: 'Uma pista' });
  const after = create(SceneClueSchema, {
    id: 'k1',
    text: 'Uma pista',
    revealedTo: [{ characterId: 'b' }],
  });

  function setup() {
    let release!: (scene: OpenSceneInfo) => void;
    let calls = 0;
    const state = new SceneState(
      () =>
        calls++ === 0
          ? Promise.resolve(masterScene([], [], { clues: [before] }))
          : new Promise((r) => (release = r)),
      () => true,
    );
    return { state, release: (s: OpenSceneInfo) => release(s) };
  }

  it('control: without a local answer the roll shows', async () => {
    const { state, release } = setup();
    await state.refresh();
    const read = state.refresh();
    release(masterScene([sceneRoll('r1', 'a2', 'Toren', 7)], [], { clues: [before] }));
    await read;
    expect(state.scene()?.rolls).toHaveLength(1);
  });

  it('shows the roll that came in the read even if a clue answer landed meanwhile', async () => {
    const { state, release } = setup();
    await state.refresh();
    const read = state.refresh(); // scene_check_rolled
    state.clueRevealed(after); // master's own reveal answered meanwhile
    release(masterScene([sceneRoll('r1', 'a2', 'Toren', 7)], [], { clues: [after] }));
    await read;
    expect(state.scene()?.clues[0].revealedTo).toHaveLength(1);
    expect(state.scene()?.rolls).toHaveLength(1);
  });
});
