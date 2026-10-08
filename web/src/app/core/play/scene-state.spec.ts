import type { OpenSceneInfo } from '../../../gen/meurpg/play/v1/scene_pb';
import { create } from '@bufbuild/protobuf';

import { SceneClueSchema } from '../../../gen/meurpg/maps/v1/maps_pb';
import { SceneState } from './scene-state';
import { masterScene, playerScene, sceneRoll, stageNpc } from './scene-testing';

describe('SceneState', () => {
  function stateWith(isMaster: boolean, ...answers: (OpenSceneInfo | null | Error)[]): SceneState {
    let i = 0;
    return new SceneState(
      () => {
        const next = answers[Math.min(i++, answers.length - 1)];
        return next instanceof Error ? Promise.reject(next) : Promise.resolve(next);
      },
      () => isMaster,
    );
  }

  it('says nothing for the first read (the page was just opened)', async () => {
    const state = stateWith(false, playerScene());
    await state.refresh();
    expect(state.scene()?.name).toBe('A carroça tombada');
    expect(state.notice()).toBe('');
  });

  it('tells a player when the master opens a scene and when it closes', async () => {
    const state = stateWith(false, null, playerScene(), null);
    await state.refresh();
    await state.refresh();
    expect(state.notice()).toBe('O mestre abriu uma cena: A carroça tombada.');
    await state.refresh();
    expect(state.notice()).toBe('O mestre fechou a cena.');
    expect(state.scene()).toBeNull();
  });

  it("does not tell a player about another player's roll or their own new row", async () => {
    const state = stateWith(
      false,
      playerScene(),
      playerScene([sceneRoll('r1', 'a1', 'Pensantus', 17)]),
    );
    await state.refresh();
    await state.refresh();
    expect(state.notice()).toBe('');
  });

  it('reads each new roll aloud to the master, oldest of the news first', async () => {
    const toren = sceneRoll('r1', 'a2', 'Toren', 7, { passed: false });
    const pens = sceneRoll('r2', 'a1', 'Pensantus', 17, { passed: true });
    const state = stateWith(true, masterScene(), masterScene([toren]), masterScene([pens, toren]));
    await state.refresh();
    await state.refresh();
    expect(state.notice()).toBe('Toren: Seguir os rastros dos goblins, 7, não passou');
    await state.refresh();
    expect(state.notice()).toBe('Pensantus: Procurar pistas na carroça, 17, passou');
  });

  it('keeps what it had when a read fails', async () => {
    const state = stateWith(false, playerScene(), new Error('network'));
    await state.refresh();
    await state.refresh();
    expect(state.scene()?.name).toBe('A carroça tombada');
  });

  it('never lets a late answer overwrite a newer one', async () => {
    let release: (s: OpenSceneInfo | null) => void = () => undefined;
    const slow = new Promise<OpenSceneInfo | null>((r) => (release = r));
    let calls = 0;
    const state = new SceneState(
      () => (calls++ === 0 ? slow : Promise.resolve(null)),
      () => false,
    );
    const first = state.refresh();
    await state.refresh();
    release(playerScene());
    await first;
    expect(state.scene()).toBeNull();
  });

  it("marks where focus goes after the master's own open and close", () => {
    const state = stateWith(true, null);
    state.openedHere(masterScene());
    expect(state.focusNext()).toBe('title');
    state.focusNext.set(null);
    state.closedHere();
    expect(state.focusNext()).toBe('open');
    expect(state.scene()).toBeNull();
    state.clear();
    expect(state.focusNext()).toBeNull();
  });

  describe("a read in flight when the master's own answer lands", () => {
    const before = create(SceneClueSchema, { id: 'k1', text: 'Uma pista' });
    const after = create(SceneClueSchema, {
      id: 'k1',
      text: 'Uma pista',
      revealedTo: [{ characterId: 'b' }],
    });

    /** The first read answers at once; each later one waits for the test to answer it. */
    function setup() {
      const pending: Array<(scene: OpenSceneInfo) => void> = [];
      let calls = 0;
      const state = new SceneState(
        () =>
          calls++ === 0
            ? Promise.resolve(masterScene([], [], { clues: [before] }))
            : new Promise((r) => pending.push(r)),
        () => true,
      );
      return { state, pending };
    }

    it('lands when no answer came meanwhile', async () => {
      const { state, pending } = setup();
      await state.refresh();
      const read = state.refresh();
      pending[0](masterScene([sceneRoll('r1', 'a2', 'Toren', 7)], [], { clues: [before] }));
      await read;
      expect(state.scene()?.rolls).toHaveLength(1);
      expect(pending).toHaveLength(1);
    });

    it('does not undo a clue the master just revealed: the older read is dropped', async () => {
      const { state, pending } = setup();
      await state.refresh();
      const stale = state.refresh();
      state.clueRevealed(after);
      expect(state.scene()?.clues[0].revealedTo).toHaveLength(1);
      pending[0](masterScene([], [], { clues: [before] }));
      await Promise.resolve();
      expect(state.scene()?.clues[0].revealedTo).toHaveLength(1);
      pending[1](masterScene([], [], { clues: [after] }));
      await stale;
      expect(state.scene()?.clues[0].revealedTo).toHaveLength(1);
    });

    it('still shows what the dropped read carried, by reading again (a roll made meanwhile)', async () => {
      const { state, pending } = setup();
      await state.refresh();
      const read = state.refresh(); // scene_check_rolled
      state.clueRevealed(after);
      pending[0](masterScene([], [], { clues: [before] }));
      await Promise.resolve();
      pending[1](masterScene([sceneRoll('r1', 'a2', 'Toren', 7)], [], { clues: [after] }));
      await read;
      expect(state.scene()?.clues[0].revealedTo).toHaveLength(1);
      expect(state.scene()?.rolls).toHaveLength(1);
      expect(state.notice()).toContain('Toren');
    });
  });

  it('tells a player when the master gives another attempt, not the master', async () => {
    const before = playerScene();
    const granted = playerScene([], [], {
      actions: before.actions.map((a) => (a.id === 'a5' ? { ...a, attemptsLeft: 2 } : a)),
    });
    const player = stateWith(false, before, granted);
    await player.refresh();
    await player.refresh();
    expect(player.notice()).toBe(
      'O mestre deu mais uma tentativa em Resistir ao cheiro de fumaça.',
    );
    const master = stateWith(true, masterScene(), masterScene());
    await master.refresh();
    await master.refresh();
    expect(master.notice()).toBe('');
  });

  it('says nothing about attempts when the scene was closed and opened again (the counts start over)', async () => {
    const before = playerScene([], [], {
      actions: playerScene().actions.map((a) => ({ ...a, attemptsLeft: 0 })),
    });
    const reopened = playerScene([], [], { openedAt: { seconds: 1791000000n, nanos: 0 } as never });
    const player = stateWith(false, before, reopened);
    await player.refresh();
    await player.refresh();
    expect(player.notice()).toBe('O mestre abriu uma cena: A carroça tombada.');
  });

  describe('the stage (MR-031)', () => {
    const mira = stageNpc('s1', 'Mira');
    const capitao = stageNpc('s2', 'Capitão Goblin');

    it("is the open scene's list, and empty with no scene", async () => {
      const state = stateWith(false, playerScene([], [mira, capitao]));
      expect(state.stage()).toEqual([]);
      await state.refresh();
      expect(state.stage().map((n) => n.name)).toEqual(['Mira', 'Capitão Goblin']);
    });

    it('tells a player who came in, who left and who speaks, but not the first read', async () => {
      const state = stateWith(
        false,
        playerScene([], [mira]),
        playerScene([], [mira, capitao]),
        playerScene([], [mira, { ...capitao, speaking: true }]),
        playerScene([], [{ ...capitao, speaking: true }]),
      );
      await state.refresh();
      expect(state.notice()).toBe('');
      await state.refresh();
      expect(state.notice()).toBe('Capitão Goblin entrou na cena.');
      await state.refresh();
      expect(state.notice()).toBe('Capitão Goblin fala.');
      await state.refresh();
      expect(state.notice()).toBe('Mira saiu da cena.');
    });

    it('does not read the stage aloud to the master, who moved it', async () => {
      const state = stateWith(true, masterScene([], [mira]), masterScene([], [mira, capitao]));
      await state.refresh();
      await state.refresh();
      expect(state.notice()).toBe('');
    });

    it("applies the master's own answer at once, and drops a read that was still on its way", async () => {
      const pending: Array<(s: OpenSceneInfo | null) => void> = [];
      let calls = 0;
      const state = new SceneState(
        () =>
          calls++ === 0
            ? Promise.resolve(masterScene())
            : new Promise<OpenSceneInfo | null>((r) => pending.push(r)),
        () => true,
      );
      await state.refresh();
      const late = state.refresh();
      state.setStage([mira]);
      expect(state.stage().map((n) => n.name)).toEqual(['Mira']);
      pending[0](masterScene());
      await Promise.resolve();
      expect(state.stage().map((n) => n.name)).toEqual(['Mira']);
      pending[1](masterScene([], [mira]));
      await late;
      expect(state.stage().map((n) => n.name)).toEqual(['Mira']);
    });

    it('ignores a stage answer when no scene is open', () => {
      const state = stateWith(true, null);
      state.setStage([mira]);
      expect(state.scene()).toBeNull();
      expect(state.stage()).toEqual([]);
    });
  });
});
