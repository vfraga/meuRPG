import { create } from '@bufbuild/protobuf';

import {
  TableScreenKind,
  TableScreenNotice,
  TableScreenStageSchema,
  TvState,
} from '../../../gen/meurpg/play/v1/table_screen_pb';
import { screenResponse } from './table-screen-testing';
import { toTvScreen } from './table-screen-view';

function stageOf(kind: TableScreenKind, body: object, rolls: object[] = []) {
  return create(TableScreenStageSchema, { kind, body, rolls } as never);
}

describe('toTvScreen', () => {
  it('has the header and the party for the idle screen', () => {
    const vm = toTvScreen(screenResponse());
    expect(vm).toMatchObject({
      campaignName: 'Mirathel',
      sessionNumber: 3,
      sessionLabel: 'Sessão 3',
      stage: { kind: 'idle', party: ['Tor', 'Brisa'] },
      rolls: [],
    });
  });

  it('reads an image', () => {
    const stage = stageOf(TableScreenKind.IMAGE, {
      case: 'image',
      value: { name: 'A carta', url: '/i/1', width: 800, height: 600 },
    });
    expect(toTvScreen(screenResponse({ stage })).stage).toEqual({
      kind: 'image',
      name: 'A carta',
      url: '/i/1',
      width: 800,
      height: 600,
    });
  });

  it('reads a scene with who speaks', () => {
    const stage = stageOf(TableScreenKind.SCENE, {
      case: 'scene',
      value: {
        name: 'A taverna',
        description: 'Cheira a fumaça.',
        stage: [{ name: 'Javali', speaking: true }, { name: 'Bardo' }],
      },
    });
    expect(toTvScreen(screenResponse({ stage })).stage).toEqual({
      kind: 'scene',
      name: 'A taverna',
      description: 'Cheira a fumaça.',
      stage: [
        { name: 'Javali', speaking: true },
        { name: 'Bardo', speaking: false },
      ],
    });
  });

  it('reads a map, with and without the fog hiding it', () => {
    const shown = stageOf(TableScreenKind.MAP, {
      case: 'map',
      value: { name: 'Cripta', revision: 7, map: { map: { name: 'Cripta' } }, vision: {} },
    });
    const vm = toTvScreen(screenResponse({ stage: shown })).stage;
    expect(vm).toMatchObject({
      kind: 'map',
      map: { name: 'Cripta', hiddenByFog: false, revision: 7 },
    });
    expect(vm.kind === 'map' && vm.map.map).not.toBeNull();

    const hidden = stageOf(TableScreenKind.MAP, {
      case: 'map',
      value: { name: 'Cripta', hiddenByFog: true },
    });
    const fog = toTvScreen(screenResponse({ stage: hidden })).stage;
    expect(fog).toMatchObject({ kind: 'map', map: { hiddenByFog: true, map: null, vision: null } });
  });

  it('reads a combat: state words, the turn, the log and the map', () => {
    const stage = stageOf(TableScreenKind.COMBAT, {
      case: 'combat',
      value: {
        round: 3,
        mastersTurn: true,
        log: ['Tor ataca.'],
        order: [
          { name: 'Tor', initial: 'T', onTurn: true, isPlayer: true },
          { name: 'Zumbi', initial: 'Z', state: TvState.VERY_HURT },
          { name: 'Wight', initial: 'W', state: TvState.UNHURT },
          { name: 'Lobo', initial: 'L', state: TvState.DEFEATED },
          { name: 'Brisa', initial: 'B', state: TvState.FALLEN, isPlayer: true },
          { name: 'Rato', initial: 'R', state: TvState.HURT },
        ],
        map: { name: 'Cripta', hiddenByFog: true },
      },
    });
    const vm = toTvScreen(screenResponse({ stage })).stage;
    expect(vm.kind).toBe('combat');
    if (vm.kind !== 'combat') {
      return;
    }
    expect(vm.round).toBe(3);
    expect(vm.mastersTurn).toBe(true);
    expect(vm.log).toEqual(['Tor ataca.']);
    expect(vm.order.map((o) => o.stateWord)).toEqual([
      null,
      'Muito ferido',
      'Ileso',
      'Derrotado',
      'Caído',
      'Ferido',
    ]);
    expect(vm.order[0]).toMatchObject({ onTurn: true, isPlayer: true, initial: 'T' });
    expect(vm.map?.hiddenByFog).toBe(true);
  });

  it('has no map for a combat the players do not see the map of', () => {
    const stage = stageOf(TableScreenKind.COMBAT, { case: 'combat', value: { round: 1 } });
    const vm = toTvScreen(screenResponse({ stage })).stage;
    expect(vm).toMatchObject({ kind: 'combat', map: null, order: [] });
  });

  it('carries the free rolls of the table', () => {
    const stage = stageOf(TableScreenKind.IDLE, { case: 'idle', value: {} }, [
      { who: 'Brisa', expression: '2d6+3', total: 11, faces: [4, 4] },
    ]);
    expect(toTvScreen(screenResponse({ stage })).rolls).toEqual([
      { who: 'Brisa', expression: '2d6+3', total: 11, faces: [4, 4] },
    ]);
  });

  describe('the notice', () => {
    const idle = { kind: TableScreenKind.IDLE, body: { case: 'idle', value: {} } };

    it('is empty without one', () => {
      expect(toTvScreen(screenResponse()).notice).toBe('');
    });

    it('says why a pinned item is gone, with the pinned kind', () => {
      const vm = toTvScreen(
        screenResponse({
          stage: idle,
          notice: TableScreenNotice.PIN_ENDED,
          choice: { mirror: false, pin: TableScreenKind.COMBAT },
        }),
      );
      expect(vm.notice).toBe('O combate acabou. A TV foi para a tela de espera.');
    });

    it('says a scene meant for some players is not on the TV', () => {
      const vm = toTvScreen(screenResponse({ notice: TableScreenNotice.SCENE_NOT_FOR_ALL }));
      expect(vm.notice).toBe(
        'A cena está aberta só para alguns jogadores, então a TV não a mostra.',
      );
    });

    it('leaves out the fog notice when the fog card already says it, in a map or a combat', () => {
      const hidden = stageOf(TableScreenKind.MAP, {
        case: 'map',
        value: { hiddenByFog: true },
      });
      const combat = stageOf(TableScreenKind.COMBAT, {
        case: 'combat',
        value: { round: 1, map: { hiddenByFog: true } },
      });
      for (const stage of [hidden, combat]) {
        const vm = toTvScreen(
          screenResponse({ stage, notice: TableScreenNotice.MAP_NEEDS_GROUP_VISION }),
        );
        expect(vm.notice).toBe('');
      }
    });

    it('keeps the fog notice when no card says it', () => {
      const vm = toTvScreen(screenResponse({ notice: TableScreenNotice.MAP_NEEDS_GROUP_VISION }));
      expect(vm.notice).toContain('O mapa tem névoa');
    });
  });

  it('falls to the waiting screen for a body it does not know', () => {
    const stage = stageOf(TableScreenKind.MAP, {});
    expect(toTvScreen(screenResponse({ stage })).stage.kind).toBe('idle');
  });
});
