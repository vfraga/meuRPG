import { TableScreenKind, TvState } from '../../../gen/meurpg/play/v1/table_screen_pb';
import { create } from '@bufbuild/protobuf';
import { GetMapResponseSchema, MapTileSchema } from '../../../gen/meurpg/maps/v1/maps_pb';
import { mapMessage, mapToken } from '../../core/maps/map-builders-testing';
import { visionResponse } from '../../core/maps/vision-testing';
import { screenResponse } from '../../core/play/table-screen-testing';
import { type TvMapVm, toTvScreen } from '../../core/play/table-screen-view';

/** The pieces of a TV screen the specs of the page and of each stage share (never imported by the app). */

export const IDLE = { kind: TableScreenKind.IDLE, body: { case: 'idle', value: {} } };

/** A fog map as the TV gets it: no image, the tiles of the group's vision, the party on it. */
export function fogMapBody(name = 'Cripta do Corvo') {
  return {
    name,
    revision: 7,
    map: create(GetMapResponseSchema, {
      map: mapMessage('m1', name, { fogEnabled: true }),
      tokens: [mapToken('c1', 'Tor'), mapToken('c2', 'Brisa', { xBp: 6000 })],
    }),
    vision: visionResponse(['BBBB', 'BBBB'], {
      tilesPath: '/images/maps/m1/tiles/',
      tileSquares: 16,
      tiles: [create(MapTileSchema, { tx: 0, ty: 0, revision: 3 })],
    }),
  };
}

/** A map without fog: the picture as it is. */
export function plainMapBody(name = 'Taverna') {
  return {
    name,
    revision: 2,
    map: create(GetMapResponseSchema, {
      map: mapMessage('m2', name, { fogEnabled: false }),
      tokens: [mapToken('c1', 'Tor')],
    }),
  };
}

export const rollsOf = [
  { who: 'Brisa', expression: '2d6+3', total: 11, faces: [2, 2] },
  { who: 'Tor', expression: '1d20+2', total: 14, faces: [10] },
];

export function combatBody(map?: object, extra: object = {}) {
  return {
    case: 'combat',
    value: {
      round: 3,
      mastersTurn: false,
      log: ['Jabuti ataca o Zumbi 2: acertou, 5 de dano.', 'bilbo gastou 6,0 m de movimento.'],
      order: [
        { name: 'Brisa', initial: 'B', state: TvState.UNSPECIFIED, isPlayer: true },
        { name: 'Tor', initial: 'T', state: TvState.UNSPECIFIED, isPlayer: true, onTurn: true },
        { name: 'Zumbi 1', initial: 'Z1', state: TvState.HURT, isPlayer: false },
      ],
      map,
      ...extra,
    },
  };
}

/** A ready screen of Mirathel's session 3 with the stage given. */
export function readyScreen(stage?: { kind: TableScreenKind; body: object }, rolls: object[] = []) {
  const response = screenResponse({ stage: stage ? { ...stage, rolls } : undefined });
  return toTvScreen(response);
}

export function mapVm(partial: Partial<TvMapVm> = {}): TvMapVm {
  return {
    name: 'Cripta do Corvo',
    hiddenByFog: false,
    map: null,
    vision: null,
    revision: 1,
    ...partial,
  };
}
