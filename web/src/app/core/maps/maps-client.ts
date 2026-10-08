import { Injectable, inject } from '@angular/core';
import type { MessageInitShape } from '@bufbuild/protobuf';
import { createClient } from '@connectrpc/connect';

import {
  type GetMapLayersResponse,
  type GetMapResponse,
  type GetTrapNoticersResponse,
  type GetMapVisionResponse,
  type LightLevel,
  type LightSpecSchema,
  type MapLayer,
  type MapSquare,
  type Map as MapMessage,
  MapPointKind,
  MapService,
  type MapPoint,
  type MapToken,
  type SceneAction,
  SceneActionDirection,
  type SceneClue,
  type TrapSpecSchema,
} from '../../../gen/meurpg/maps/v1/maps_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';

/** What a point's editor saves in one call (`UpdateMapPoint`). */
export interface PointChanges {
  readonly kind?: MapPointKind;
  readonly name?: string;
  readonly description?: string;
  readonly xBp?: number;
  readonly yBp?: number;
  /** A SUBMAP point's target; `''` removes it. */
  readonly targetMapId?: string;
  readonly revealed?: boolean;
  /** "Ganchos e anotações" of a SCENE point (MR-029); `''` clears them. */
  readonly hooks?: string;
  /** "Mostrar a CD aos jogadores" of a SCENE point (RN-20). */
  readonly showDc?: boolean;
  /** A TRAP point's whole spec (E9-02): every field, an unspecified state keeps the current one. */
  readonly trap?: MessageInitShape<typeof TrapSpecSchema>;
  /** A LIGHT point's whole spec: a preset's key and radii, or the custom radii. */
  readonly light?: MessageInitShape<typeof LightSpecSchema>;
  /** A TREASURE's value in PO, 0 to 1.000.000. */
  readonly treasureValuePo?: number;
}

/**
 * Thin wrapper around the generated `MapService` client (MR-008, MR-009,
 * MR-012), in the same shape as `GalleryClient`. `providedIn: 'root'`
 * because the map pages, the campaign page's Mapas panel and the session
 * page all use it, and only lazy code imports this file. Callers map
 * errors (`not_found`, `permission_denied`, `aborted`…) to Portuguese
 * themselves. Tests replace it with `{ provide: MapsClient, useValue }`.
 */
@Injectable({ providedIn: 'root' })
export class MapsClient {
  private readonly client = createClient(MapService, inject(CONNECT_TRANSPORT));

  async list(campaignId: string): Promise<MapMessage[]> {
    return (await this.client.listMaps({ campaignId })).maps;
  }

  /** `GetMap`. `asCharacterId` is the master's "Ver como" (MR-036): the map exactly as that character's player gets it. */
  get(campaignId: string, mapId: string, asCharacterId = ''): Promise<GetMapResponse> {
    return this.client.getMap({ campaignId, mapId, asCharacterId });
  }

  /** `GetMapLayers`: the walls, the difficult terrain and the cover the caller
   * may read (packed; `decodeLayers` unpacks them). */
  layers(campaignId: string, mapId: string, asCharacterId = ''): Promise<GetMapLayersResponse> {
    return this.client.getMapLayers({ campaignId, mapId, asCharacterId });
  }

  /** `GetMapVision` (MR-036): what the caller sees of a map's squares, packed
   * (`decodeVision` unpacks them), and the tiles of the image. */
  vision(campaignId: string, mapId: string, asCharacterId = ''): Promise<GetMapVisionResponse> {
    return this.client.getMapVision({ campaignId, mapId, asCharacterId });
  }

  /** `SetCarriedLight`: the light a character carries (a preset's key, or '' for none). */
  async setCarriedLight(
    campaignId: string,
    mapId: string,
    characterId: string,
    lightKey: string,
  ): Promise<MapToken> {
    const res = await this.client.setCarriedLight({ campaignId, mapId, characterId, lightKey });
    return need(res.token, 'SetCarriedLight');
  }

  /** `CreateMap`. `idempotencyKey`: one per create, sent again on a retry (see `ActionKey`). */
  async create(
    campaignId: string,
    name: string,
    imageId: string,
    idempotencyKey: string,
  ): Promise<MapMessage> {
    const res = await this.client.createMap({ campaignId, name, imageId, idempotencyKey });
    return need(res.map, 'CreateMap');
  }

  async update(
    campaignId: string,
    mapId: string,
    revision: number,
    changes: { name?: string; imageId?: string },
  ): Promise<MapMessage> {
    const res = await this.client.updateMap({ campaignId, mapId, revision, ...changes });
    return need(res.map, 'UpdateMap');
  }

  async delete(campaignId: string, mapId: string): Promise<void> {
    await this.client.deleteMap({ campaignId, mapId });
  }

  async setRevealed(campaignId: string, mapId: string, revealed: boolean): Promise<MapMessage> {
    const res = await this.client.setMapRevealed({ campaignId, mapId, revealed });
    return need(res.map, 'SetMapRevealed');
  }

  /** `SetMapGrid` (RN-21, RN-25): the squares of the DRAWING across the image's width, 4 to 200; 0 clears the
   * grid. `squareFactor` is required: how many squares of 1,5 m each is worth (1 to 20). A caller that changes only the
   * columns passes the map's own factor, or the server would read 0 as 1 and drop the calibration. The server
   * answers with the rows it worked out. */
  async setGrid(
    campaignId: string,
    mapId: string,
    columns: number,
    squareFactor: number,
  ): Promise<MapMessage> {
    const res = await this.client.setMapGrid({ campaignId, mapId, columns, squareFactor });
    return need(res.map, 'SetMapGrid');
  }

  /** `PaintMapCells` (MR-034): one value on up to 400 squares of one layer. Answers with how many changed. */
  async paint(
    campaignId: string,
    mapId: string,
    layer: MapLayer,
    value: number,
    squares: readonly MapSquare[] | readonly { col: number; row: number }[],
  ): Promise<{ layersRevision: number; changed: number }> {
    const res = await this.client.paintMapCells({
      campaignId,
      mapId,
      layer,
      value,
      squares: squares.map((s) => ({ col: s.col, row: s.row })),
    });
    return { layersRevision: res.layersRevision, changed: res.changed };
  }

  /** `SetMapFog` (MR-036): each field set replaces the current value. */
  async setFog(
    campaignId: string,
    mapId: string,
    changes: { fogEnabled?: boolean; baseLight?: LightLevel; groupVision?: boolean },
  ): Promise<MapMessage> {
    const res = await this.client.setMapFog({ campaignId, mapId, ...changes });
    return need(res.map, 'SetMapFog');
  }

  /** `ForgetMapVision`: "Esquecer o que foi visto". */
  async forgetVision(campaignId: string, mapId: string): Promise<void> {
    await this.client.forgetMapVision({ campaignId, mapId });
  }

  async createPoint(
    campaignId: string,
    mapId: string,
    point: {
      kind: MapPointKind;
      name: string;
      description: string;
      xBp: number;
      yBp: number;
      targetMapId?: string;
      /** A TRAP needs its spec, a LIGHT its light; a TREASURE may carry its value. */
      trap?: PointChanges['trap'];
      light?: PointChanges['light'];
      treasureValuePo?: number;
    },
    idempotencyKey: string,
  ): Promise<MapPoint> {
    const res = await this.client.createMapPoint({ campaignId, mapId, ...point, idempotencyKey });
    return need(res.point, 'CreateMapPoint');
  }

  async updatePoint(
    campaignId: string,
    mapId: string,
    pointId: string,
    changes: PointChanges,
  ): Promise<MapPoint> {
    const res = await this.client.updateMapPoint({ campaignId, mapId, pointId, ...changes });
    return need(res.point, 'UpdateMapPoint');
  }

  async deletePoint(campaignId: string, mapId: string, pointId: string): Promise<void> {
    await this.client.deleteMapPoint({ campaignId, mapId, pointId });
  }

  async setPointRevealed(
    campaignId: string,
    mapId: string,
    pointId: string,
    revealed: boolean,
  ): Promise<MapPoint> {
    const res = await this.client.setMapPointRevealed({ campaignId, mapId, pointId, revealed });
    return need(res.point, 'SetMapPointRevealed');
  }

  /** Places or moves a token: a character's (`characterId`) or a creature's (`creatureId`, with an empty `characterId`). */
  async placeToken(
    campaignId: string,
    mapId: string,
    characterId: string,
    xBp: number,
    yBp: number,
    creatureId = '',
  ): Promise<MapToken> {
    const res = await this.client.placeMapToken({
      campaignId,
      mapId,
      characterId,
      xBp,
      yBp,
      creatureId,
    });
    return need(res.token, 'PlaceMapToken');
  }

  async setTokenHidden(
    campaignId: string,
    mapId: string,
    characterId: string,
    hidden: boolean,
  ): Promise<MapToken> {
    const res = await this.client.setMapTokenHidden({ campaignId, mapId, characterId, hidden });
    return need(res.token, 'SetMapTokenHidden');
  }

  /** Takes a token off the map: a character's by `characterId`, or a creature's by `creatureId` (its `character_id` is its owner's,
   * so sending it would take the owner's token off instead). */
  async removeToken(
    campaignId: string,
    mapId: string,
    characterId: string,
    creatureId = '',
  ): Promise<void> {
    await this.client.removeMapToken(
      creatureId === ''
        ? { campaignId, mapId, characterId }
        : { campaignId, mapId, characterId: '', creatureId },
    );
  }

  /** `RevealTrap` (MR-035): the trap goes to the chosen characters' players, or to
   * everyone. Answers with the point as the master sees it. */
  async revealTrap(
    campaignId: string,
    mapId: string,
    pointId: string,
    to: { readonly characterIds: readonly string[] } | { readonly all: true },
  ): Promise<MapPoint> {
    const res = await this.client.revealTrap({
      campaignId,
      mapId,
      pointId,
      ...('all' in to ? { all: true } : { characterIds: [...to.characterIds] }),
    });
    return need(res.point, 'RevealTrap');
  }

  /** `GetTrapNoticers`: "Quem notaria", worked out by the server. */
  getTrapNoticers(
    campaignId: string,
    mapId: string,
    pointId: string,
  ): Promise<GetTrapNoticersResponse> {
    return this.client.getTrapNoticers({ campaignId, mapId, pointId });
  }

  /** `DisarmTrap`: "Desarmada", after the table resolved the check. */
  async disarmTrap(campaignId: string, mapId: string, pointId: string): Promise<MapPoint> {
    const res = await this.client.disarmTrap({ campaignId, mapId, pointId });
    return need(res.point, 'DisarmTrap');
  }

  /** `MarkTreasureFound` (MR-041): by one or more characters. */
  async markTreasureFound(
    campaignId: string,
    mapId: string,
    pointId: string,
    characterIds: readonly string[],
  ): Promise<MapPoint> {
    const res = await this.client.markTreasureFound({
      campaignId,
      mapId,
      pointId,
      characterIds: [...characterIds],
    });
    return need(res.point, 'MarkTreasureFound');
  }

  /** `UnmarkTreasureFound`: back to hidden (refused once it was turned into XP). */
  async unmarkTreasureFound(campaignId: string, mapId: string, pointId: string): Promise<MapPoint> {
    const res = await this.client.unmarkTreasureFound({ campaignId, mapId, pointId });
    return need(res.point, 'UnmarkTreasureFound');
  }

  /** `AddSceneAction` (MR-015): one more check on a SCENE point, saved at
   * once. `dc` 0 means none. Answers with the point's actions as they are now. */
  async addSceneAction(
    campaignId: string,
    mapId: string,
    pointId: string,
    action: { key: string; name: string; dc: number },
    idempotencyKey: string,
  ): Promise<readonly SceneAction[]> {
    const res = await this.client.addSceneAction({
      campaignId,
      mapId,
      pointId,
      ...action,
      idempotencyKey,
    });
    return res.actions;
  }

  /** `UpdateSceneAction`: how many attempts each player has at the action
   * (1 to 5, 0 for unlimited), saved at once. Answers with the point's actions. */
  async setSceneActionAttempts(
    campaignId: string,
    mapId: string,
    pointId: string,
    actionId: string,
    maxAttempts: number,
  ): Promise<readonly SceneAction[]> {
    const res = await this.client.updateSceneAction({
      campaignId,
      mapId,
      pointId,
      actionId,
      maxAttempts,
    });
    return res.actions;
  }

  /** `MoveSceneAction`: one place up or down. */
  async moveSceneAction(
    campaignId: string,
    mapId: string,
    pointId: string,
    actionId: string,
    direction: 'up' | 'down',
  ): Promise<readonly SceneAction[]> {
    const res = await this.client.moveSceneAction({
      campaignId,
      mapId,
      pointId,
      actionId,
      direction: direction === 'up' ? SceneActionDirection.UP : SceneActionDirection.DOWN,
    });
    return res.actions;
  }

  /** `RemoveSceneAction`: no confirmation (the rolls made stay in the log). */
  async removeSceneAction(
    campaignId: string,
    mapId: string,
    pointId: string,
    actionId: string,
  ): Promise<readonly SceneAction[]> {
    const res = await this.client.removeSceneAction({ campaignId, mapId, pointId, actionId });
    return res.actions;
  }
  /** `AddSceneClue` (MR-029): one more clue on a SCENE point, saved at once.
   * Answers with the point's clues as they are now, the new one last. */
  async addSceneClue(
    campaignId: string,
    mapId: string,
    pointId: string,
    text: string,
    idempotencyKey: string,
  ): Promise<readonly SceneClue[]> {
    const res = await this.client.addSceneClue({
      campaignId,
      mapId,
      pointId,
      text,
      idempotencyKey,
    });
    return res.clues;
  }

  /** `UpdateSceneClue`: the new text of one clue (what players already got keeps the old). */
  async updateSceneClue(
    campaignId: string,
    mapId: string,
    pointId: string,
    clueId: string,
    text: string,
  ): Promise<readonly SceneClue[]> {
    const res = await this.client.updateSceneClue({ campaignId, mapId, pointId, clueId, text });
    return res.clues;
  }

  /** `MoveSceneClue`: one place up or down. */
  async moveSceneClue(
    campaignId: string,
    mapId: string,
    pointId: string,
    clueId: string,
    direction: 'up' | 'down',
  ): Promise<readonly SceneClue[]> {
    const res = await this.client.moveSceneClue({
      campaignId,
      mapId,
      pointId,
      clueId,
      direction: direction === 'up' ? SceneActionDirection.UP : SceneActionDirection.DOWN,
    });
    return res.clues;
  }

  /** `RemoveSceneClue`: the page asks first; players who got the clue keep it. */
  async removeSceneClue(
    campaignId: string,
    mapId: string,
    pointId: string,
    clueId: string,
  ): Promise<readonly SceneClue[]> {
    const res = await this.client.removeSceneClue({ campaignId, mapId, pointId, clueId });
    return res.clues;
  }

  /** `RevealSceneClue` (MR-029): gives the clue to the players of these
   * characters. Answers with the clue and everyone who has it now. */
  async revealSceneClue(
    campaignId: string,
    clueId: string,
    characterIds: readonly string[],
  ): Promise<SceneClue> {
    const res = await this.client.revealSceneClue({
      campaignId,
      clueId,
      characterIds: [...characterIds],
    });
    return need(res.clue, 'RevealSceneClue');
  }
}

function need<T>(value: T | undefined, call: string): T {
  if (value === undefined) {
    throw new Error(`${call} answered without its result`);
  }
  return value;
}
