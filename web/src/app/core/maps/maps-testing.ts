import { create } from '@bufbuild/protobuf';

import { CharacterKind } from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  type Map as MapMessage,
  MapPointKind,
  MapSchema,
  type MapPoint,
  MapPointSchema,
  type MapToken,
  MapTokenSchema,
  type GetMapLayersResponse,
  type GetMapResponse,
  type GetMapVisionResponse,
  type GetTrapNoticersResponse,
  type LightLevel,
  type MapLayer,
  type SceneAction,
  SceneActionSchema,
  type SceneClue,
  SceneClueSchema,
} from '../../../gen/meurpg/maps/v1/maps_pb';
import type { PointChanges } from './maps-client';

/**
 * Builders and a stand-in for the map specs (never imported by the app
 * itself, so never bundled): a `Map`, a `MapPoint` and a `MapToken` as the
 * server sends them, and a `MapsClient` that remembers its calls.
 */
export function mapMessage(
  id: string,
  name: string,
  partial: Partial<Omit<MapMessage, '$typeName'>> = {},
): MapMessage {
  return create(MapSchema, {
    id,
    campaignId: 'camp-1',
    name,
    image: {
      id: `img-${id}`,
      url: `/images/img-${id}`,
      thumbnailUrl: `/images/img-${id}/thumb`,
      width: 2400,
      height: 1600,
      name: `Imagem de ${name}`,
    },
    revealed: false,
    revision: 1,
    ...partial,
  });
}

export function mapPoint(
  id: string,
  name: string,
  partial: Partial<Omit<MapPoint, '$typeName'>> = {},
): MapPoint {
  return create(MapPointSchema, {
    id,
    mapId: 'map-1',
    kind: MapPointKind.SCENE,
    name,
    description: '',
    xBp: 5000,
    yBp: 5000,
    revealed: false,
    ...partial,
  });
}

export function mapToken(
  characterId: string,
  name: string,
  partial: Partial<Omit<MapToken, '$typeName'>> = {},
): MapToken {
  return create(MapTokenSchema, {
    mapId: 'map-1',
    characterId,
    name,
    kind: CharacterKind.PLAYER,
    xBp: 4000,
    yBp: 4000,
    hidden: false,
    ...partial,
  });
}

export function mapResponse(
  map: MapMessage,
  points: MapPoint[] = [],
  tokens: MapToken[] = [],
): GetMapResponse {
  return { $typeName: 'meurpg.maps.v1.GetMapResponse', map, points, tokens };
}

/** A `MapsClient` over in-memory maps: every call is recorded in `calls`. */
export class FakeMapsClient {
  maps: MapMessage[] = [];
  responses = new Map<string, GetMapResponse>();
  calls: string[] = [];
  failWith: unknown = null;

  private record(call: string, ...args: unknown[]): void {
    this.calls.push([call, ...args].join(' '));
    if (this.failWith) {
      throw this.failWith;
    }
  }

  async list(_campaignId: string): Promise<MapMessage[]> {
    this.record('list');
    return this.maps;
  }

  /** What `GetMapVision` answers, by the character the master reads as (`''` for the caller's own). */
  visions = new Map<string, GetMapVisionResponse>();
  /** The layers `GetMapLayers` answers. */
  layersResponse: GetMapLayersResponse | null = null;

  async vision(_c: string, mapId: string, asCharacterId = ''): Promise<GetMapVisionResponse> {
    this.record('vision', mapId, asCharacterId);
    const res = this.visions.get(asCharacterId);
    if (!res) {
      const { ConnectError, Code } = await import('@connectrpc/connect');
      throw new ConnectError('not found', Code.NotFound);
    }
    return res;
  }

  async layers(_c: string, mapId: string, asCharacterId = ''): Promise<GetMapLayersResponse> {
    this.record('layers', mapId, asCharacterId);
    return (
      this.layersResponse ??
      ({
        $typeName: 'meurpg.maps.v1.GetMapLayersResponse',
        gridColumns: 0,
        gridRows: 0,
        difficultTerrain: new Uint8Array(),
        wall: new Uint8Array(),
        cover: new Uint8Array(),
      } as unknown as GetMapLayersResponse)
    );
  }

  /** What `PaintMapCells` was asked: layer, value and the squares of each call, in order. */
  paints: { layer: MapLayer; value: number; squares: { col: number; row: number }[] }[] = [];

  async paint(
    _c: string,
    mapId: string,
    layer: MapLayer,
    value: number,
    squares: readonly { col: number; row: number }[],
  ): Promise<{ layersRevision: number; changed: number }> {
    this.record('paint', mapId, layer, value, squares.length);
    this.paints.push({ layer, value, squares: squares.map((s) => ({ col: s.col, row: s.row })) });
    return { layersRevision: this.paints.length, changed: squares.length };
  }

  /** The map the next `SetMapFog` and `SetMapGrid` answer with (set by the spec). */
  mapAfter: MapMessage | null = null;

  async setFog(
    _c: string,
    mapId: string,
    changes: { fogEnabled?: boolean; baseLight?: LightLevel; groupVision?: boolean },
  ): Promise<MapMessage> {
    this.record('setFog', mapId, JSON.stringify(changes));
    return (
      this.mapAfter ?? mapMessage(mapId, 'Mapa', { gridColumns: 24, gridRows: 16, ...changes })
    );
  }

  async forgetVision(_c: string, mapId: string): Promise<void> {
    this.record('forgetVision', mapId);
  }

  async setGrid(
    _c: string,
    mapId: string,
    columns: number,
    squareFactor: number,
  ): Promise<MapMessage> {
    // The factor is only written when it is not 1, so the calls of a map that was never calibrated read as before.
    this.record('setGrid', mapId, squareFactor === 1 ? columns : `${columns} x${squareFactor}`);
    const drawnRows = Math.round((columns * 1600) / 2400);
    return (
      this.mapAfter ??
      mapMessage(mapId, 'Mapa', {
        gridColumns: columns * squareFactor,
        gridRows: drawnRows * squareFactor,
        drawnColumns: columns,
        drawnRows,
        squareFactor,
      })
    );
  }

  /** What `GetTrapNoticers` answers. */
  noticers: GetTrapNoticersResponse | null = null;

  async getTrapNoticers(
    _c: string,
    mapId: string,
    pointId: string,
  ): Promise<GetTrapNoticersResponse> {
    this.record('getTrapNoticers', mapId, pointId);
    return (
      this.noticers ??
      ({
        $typeName: 'meurpg.maps.v1.GetTrapNoticersResponse',
        noticers: [],
        noticeDc: 0,
      } as GetTrapNoticersResponse)
    );
  }

  async markTreasureFound(
    _c: string,
    mapId: string,
    pointId: string,
    characterIds: readonly string[],
  ): Promise<MapPoint> {
    this.record('markTreasureFound', mapId, pointId, characterIds.join(','));
    return mapPoint(pointId, 'Baú', {
      kind: MapPointKind.TREASURE,
      treasureFoundAt: { seconds: 1n, nanos: 0, $typeName: 'google.protobuf.Timestamp' } as never,
    });
  }

  async unmarkTreasureFound(_c: string, mapId: string, pointId: string): Promise<MapPoint> {
    this.record('unmarkTreasureFound', mapId, pointId);
    return mapPoint(pointId, 'Baú', { kind: MapPointKind.TREASURE });
  }

  async createPoint(
    _c: string,
    mapId: string,
    point: { kind: MapPointKind; name: string; xBp: number; yBp: number },
  ): Promise<MapPoint> {
    this.record('createPoint', mapId, JSON.stringify(point));
    return mapPoint('new-point', point.name, { kind: point.kind, xBp: point.xBp, yBp: point.yBp });
  }

  async deletePoint(_c: string, mapId: string, pointId: string): Promise<void> {
    this.record('deletePoint', mapId, pointId);
  }

  async removeToken(
    _c: string,
    mapId: string,
    characterId: string,
    creatureId = '',
  ): Promise<void> {
    this.record('removeToken', mapId, creatureId ? `creature:${creatureId}` : characterId);
  }

  async setCarriedLight(
    _c: string,
    mapId: string,
    characterId: string,
    lightKey: string,
  ): Promise<MapToken> {
    this.record('setCarriedLight', mapId, characterId, lightKey);
    return mapToken(characterId, 'Token', { mapId, carriedLight: lightKey });
  }

  async get(_campaignId: string, mapId: string, asCharacterId = ''): Promise<GetMapResponse> {
    this.record('get', mapId, ...(asCharacterId ? [asCharacterId] : []));
    const res =
      this.responses.get(asCharacterId ? `${mapId}@${asCharacterId}` : mapId) ??
      this.responses.get(mapId);
    if (!res) {
      const { ConnectError, Code } = await import('@connectrpc/connect');
      throw new ConnectError('not found', Code.NotFound);
    }
    return res;
  }

  async create(_c: string, name: string, imageId: string): Promise<MapMessage> {
    this.record('create', name, imageId);
    return mapMessage('new-map', name);
  }

  async setPointRevealed(
    _c: string,
    mapId: string,
    pointId: string,
    revealed: boolean,
  ): Promise<MapPoint> {
    this.record('setPointRevealed', mapId, pointId, revealed);
    return mapPoint(pointId, 'Ponto', { revealed });
  }

  async setTokenHidden(
    _c: string,
    mapId: string,
    characterId: string,
    hidden: boolean,
  ): Promise<MapToken> {
    this.record('setTokenHidden', mapId, characterId, hidden);
    return mapToken(characterId, 'Token', { hidden });
  }

  async placeToken(
    _c: string,
    mapId: string,
    characterId: string,
    xBp: number,
    yBp: number,
    creatureId = '',
  ): Promise<MapToken> {
    this.record('placeToken', mapId, characterId || `creature:${creatureId}`, xBp, yBp);
    return mapToken(characterId, 'Token', { xBp, yBp, creatureId });
  }

  async updatePoint(
    _c: string,
    mapId: string,
    pointId: string,
    changes: PointChanges,
  ): Promise<MapPoint> {
    this.record('updatePoint', mapId, pointId, JSON.stringify(changes));
    return mapPoint(pointId, changes.name ?? 'Ponto', { ...changes } as never);
  }

  /** The scene actions of the point the specs work on, as the server keeps them. */
  sceneActions: SceneAction[] = [];

  async addSceneAction(
    _c: string,
    _m: string,
    pointId: string,
    action: { key: string; name: string; dc: number },
  ): Promise<readonly SceneAction[]> {
    this.record('addSceneAction', pointId, JSON.stringify(action));
    if (this.sceneActions.length >= 20) {
      const { ConnectError, Code } = await import('@connectrpc/connect');
      throw new ConnectError('limit', Code.ResourceExhausted);
    }
    this.sceneActions = [
      ...this.sceneActions,
      create(SceneActionSchema, {
        id: `n${this.sceneActions.length + 1}`,
        checkName: action.key,
        ...action,
      }),
    ];
    return this.sceneActions;
  }

  async moveSceneAction(
    _c: string,
    _m: string,
    pointId: string,
    actionId: string,
    direction: 'up' | 'down',
  ): Promise<readonly SceneAction[]> {
    this.record('moveSceneAction', pointId, actionId, direction);
    const list = [...this.sceneActions];
    const i = list.findIndex((a) => a.id === actionId);
    const j = direction === 'up' ? i - 1 : i + 1;
    if (j >= 0 && j < list.length) {
      [list[i], list[j]] = [list[j], list[i]];
    }
    this.sceneActions = list;
    return list;
  }

  async setSceneActionAttempts(
    _c: string,
    _m: string,
    pointId: string,
    actionId: string,
    maxAttempts: number,
  ): Promise<readonly SceneAction[]> {
    this.record('setSceneActionAttempts', pointId, actionId, String(maxAttempts));
    this.sceneActions = this.sceneActions.map((a) =>
      a.id === actionId ? { ...a, maxAttempts } : a,
    );
    return this.sceneActions;
  }

  async removeSceneAction(
    _c: string,
    _m: string,
    pointId: string,
    actionId: string,
  ): Promise<readonly SceneAction[]> {
    this.record('removeSceneAction', pointId, actionId);
    this.sceneActions = this.sceneActions.filter((a) => a.id !== actionId);
    return this.sceneActions;
  }

  /** The clues of the point the specs work on, as the server keeps them. */
  sceneClues: SceneClue[] = [];
  /** What the next reveal answers with (set by the spec). */
  revealed: SceneClue | null = null;

  async addSceneClue(
    _c: string,
    _m: string,
    pointId: string,
    text: string,
  ): Promise<readonly SceneClue[]> {
    this.record('addSceneClue', pointId, text);
    if (this.sceneClues.length >= 30) {
      const { ConnectError, Code } = await import('@connectrpc/connect');
      throw new ConnectError('limit', Code.ResourceExhausted);
    }
    this.sceneClues = [
      ...this.sceneClues,
      create(SceneClueSchema, { id: `k${this.sceneClues.length + 1}`, text }),
    ];
    return this.sceneClues;
  }

  async updateSceneClue(
    _c: string,
    _m: string,
    pointId: string,
    clueId: string,
    text: string,
  ): Promise<readonly SceneClue[]> {
    this.record('updateSceneClue', pointId, clueId, text);
    this.sceneClues = this.sceneClues.map((c) => (c.id === clueId ? { ...c, text } : c));
    return this.sceneClues;
  }

  async moveSceneClue(
    _c: string,
    _m: string,
    pointId: string,
    clueId: string,
    direction: 'up' | 'down',
  ): Promise<readonly SceneClue[]> {
    this.record('moveSceneClue', pointId, clueId, direction);
    const list = [...this.sceneClues];
    const i = list.findIndex((c) => c.id === clueId);
    const j = direction === 'up' ? i - 1 : i + 1;
    if (j >= 0 && j < list.length) {
      [list[i], list[j]] = [list[j], list[i]];
    }
    this.sceneClues = list;
    return list;
  }

  async removeSceneClue(
    _c: string,
    _m: string,
    pointId: string,
    clueId: string,
  ): Promise<readonly SceneClue[]> {
    this.record('removeSceneClue', pointId, clueId);
    this.sceneClues = this.sceneClues.filter((c) => c.id !== clueId);
    return this.sceneClues;
  }

  async revealSceneClue(
    _c: string,
    clueId: string,
    characterIds: readonly string[],
  ): Promise<SceneClue> {
    this.record('revealSceneClue', clueId, characterIds.join(','));
    return this.revealed ?? create(SceneClueSchema, { id: clueId, text: 'x' });
  }
}
