import { Injectable, inject } from '@angular/core';

import { MapPointKind } from '../../../gen/meurpg/maps/v1/maps_pb';
import type {
  GetMapLayersResponse,
  GetMapResponse,
  Map as MapMessage,
} from '../../../gen/meurpg/maps/v1/maps_pb';
import { type DoorKind, decodeLayers } from '../maps/layers';
import { MapsClient } from '../maps/maps-client';

/** What "Abrir uma porta" can name (the words come from the map's own legend). */
const DOOR_WORDS: Readonly<Record<DoorKind, string>> = {
  1: 'Aberta',
  2: 'Fechada',
  3: 'Trancada',
  4: 'Grade',
  5: 'Secreta',
};

export interface MapChoice {
  readonly id: string;
  readonly name: string;
}

export interface DoorChoice {
  readonly col: number;
  readonly row: number;
  /** "Fechada · (5, 3)": the kind and the place (column, row; they count from 1 on screen). The door's picture sits under the select. */
  readonly label: string;
}

export interface PointChoice {
  readonly id: string;
  readonly name: string;
}

export interface ClueChoice {
  readonly id: string;
  readonly text: string;
  /** The scene the clue belongs to. */
  readonly pointName: string;
}

export interface TrapChoice {
  readonly mapId: string;
  readonly mapName: string;
  readonly pointId: string;
  /** The name the master gave the trap point. */
  readonly name: string;
}

/** The reads `SolveTargets` needs; `MapsClient` is one. */
export type TargetsApi = Pick<MapsClient, 'list' | 'get' | 'layers'>;

/**
 * What the master picks "Ao resolver" targets from (E10-06 state 2): the campaign's maps, a map's doors, a map's points and
 * the clues of the scenes. Everything comes from reads that already exist (`ListMaps`, `GetMapLayers`, `GetMap`); a map is
 * read once and kept for the form's lifetime. Plain data out, no signals: the form holds the state.
 */
@Injectable()
export class SolveTargets {
  private readonly api: TargetsApi = inject(MapsClient);
  private readonly layerCache = new Map<string, Promise<readonly DoorChoice[]>>();
  private readonly mapCache = new Map<string, Promise<GetMapResponse>>();
  private campaign = '';
  private mapsPromise: Promise<MapMessage[]> | null = null;

  /** Call once with the campaign before anything else. */
  use(campaignId: string): void {
    if (campaignId !== this.campaign) {
      this.layerCache.clear();
      this.mapCache.clear();
      this.mapsPromise = null;
    }
    this.campaign = campaignId;
  }

  maps(): Promise<MapChoice[]> {
    if (!this.mapsPromise) {
      const read = this.api.list(this.campaign);
      this.mapsPromise = read;
      // A failed read is not kept: the next call asks the server again.
      read.catch(() => {
        if (this.mapsPromise === read) {
          this.mapsPromise = null;
        }
      });
    }
    return this.mapsPromise.then((maps) => maps.map((m) => ({ id: m.id, name: m.name })));
  }

  doors(mapId: string): Promise<readonly DoorChoice[]> {
    let doors = this.layerCache.get(mapId);
    if (!doors) {
      const read = this.api.layers(this.campaign, mapId).then(doorsOf);
      doors = read;
      this.layerCache.set(mapId, read);
      read.catch(() => {
        if (this.layerCache.get(mapId) === read) {
          this.layerCache.delete(mapId);
        }
      });
    }
    return doors;
  }

  async points(mapId: string): Promise<PointChoice[]> {
    const { points } = await this.read(mapId);
    return points.map((p) => ({ id: p.id, name: p.name || 'Ponto sem nome' }));
  }

  /** Every clue of every scene point of every map of the campaign, scene by scene (the maps are read together). */
  async clues(): Promise<ClueChoice[]> {
    const maps = await this.maps();
    const reads = await Promise.all(maps.map((m) => this.read(m.id)));
    return reads.flatMap(({ points }) =>
      points
        .filter((point) => point.kind === MapPointKind.SCENE)
        .flatMap((point) =>
          point.clues.map((clue) => ({
            id: clue.id,
            text: clue.text,
            pointName: point.name || 'Cena sem nome',
          })),
        ),
    );
  }

  /** Every trap point of every map of the campaign ("Ao errar" fires one): "Dardos envenenados · Salão do trono". */
  async traps(): Promise<TrapChoice[]> {
    const maps = await this.maps();
    const reads = await Promise.all(maps.map((m) => this.read(m.id)));
    return reads.flatMap(({ points }, i) =>
      points
        .filter((point) => point.kind === MapPointKind.TRAP)
        .map((point) => ({
          mapId: maps[i].id,
          mapName: maps[i].name,
          pointId: point.id,
          name: point.name || 'Armadilha sem nome',
        })),
    );
  }

  private read(mapId: string): Promise<GetMapResponse> {
    let read = this.mapCache.get(mapId);
    if (!read) {
      const fetched = this.api.get(this.campaign, mapId);
      read = fetched;
      this.mapCache.set(mapId, fetched);
      fetched.catch(() => {
        if (this.mapCache.get(mapId) === fetched) {
          this.mapCache.delete(mapId);
        }
      });
    }
    return read;
  }
}

/** The doors of a map's layers, in reading order. */
export function doorsOf(response: GetMapLayersResponse): DoorChoice[] {
  const layers = decodeLayers({ ...response, doors: response.doors });
  return (layers.doors ?? []).map((d) => ({
    col: d.col,
    row: d.row,
    label: `${DOOR_WORDS[d.state]} · (${d.col + 1}, ${d.row + 1})`,
  }));
}
