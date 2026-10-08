import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';

import {
  GetMapLayersResponseSchema,
  GetMapResponseSchema,
  MapPointKind,
  MapPointSchema,
  MapSchema,
  SceneClueSchema,
} from '../../../gen/meurpg/maps/v1/maps_pb';
import { MapsClient } from '../maps/maps-client';
import { SolveTargets } from './solve-targets';

/** A 4 × 3 map: floor all round, a wall at (1,0) and (1,2), a closed door at (1,1) and an open one at (3,1). */
function layers() {
  const doors = new Uint8Array(6);
  const set = (n: number, state: number) => (doors[n >> 1] |= n & 1 ? state << 4 : state);
  set(1 * 4 + 1, 2);
  set(1 * 4 + 3, 1);
  const wall = new Uint8Array(2);
  wall[0] |= 1 << 1;
  wall[1] |= 1 << 1;
  return create(GetMapLayersResponseSchema, {
    gridColumns: 4,
    gridRows: 3,
    wall,
    difficultTerrain: new Uint8Array(2),
    cover: new Uint8Array(1),
    doors,
  });
}

describe('SolveTargets (the choices of "Ao resolver")', () => {
  let targets: SolveTargets;
  const calls: string[] = [];

  beforeEach(() => {
    calls.length = 0;
    const api = {
      list: async () => {
        calls.push('list');
        return [
          create(MapSchema, { id: 'm1', name: 'A capela' }),
          create(MapSchema, { id: 'm2', name: 'A cripta' }),
        ];
      },
      get: async (_c: string, mapId: string) => {
        calls.push(`get ${mapId}`);
        return create(GetMapResponseSchema, {
          points:
            mapId === 'm1'
              ? [
                  create(MapPointSchema, {
                    id: 'p1',
                    name: 'Altar',
                    kind: MapPointKind.SCENE,
                    clues: [create(SceneClueSchema, { id: 'c1', text: 'O sol nasce a leste.' })],
                  }),
                  create(MapPointSchema, {
                    id: 'p2',
                    name: 'Baú do Salão',
                    kind: MapPointKind.BATTLE,
                  }),
                ]
              : [],
        });
      },
      layers: async () => {
        calls.push('layers');
        return layers();
      },
    };
    TestBed.configureTestingModule({
      providers: [SolveTargets, { provide: MapsClient, useValue: api }],
    });
    targets = TestBed.inject(SolveTargets);
    targets.use('camp-1');
  });

  it("lists the campaign's maps once", async () => {
    expect(await targets.maps()).toEqual([
      { id: 'm1', name: 'A capela' },
      { id: 'm2', name: 'A cripta' },
    ]);
    await targets.maps();
    expect(calls.filter((c) => c === 'list').length).toBe(1);
  });

  it('names the doors of a map by their kind and their place, counted from 1', async () => {
    expect((await targets.doors('m1')).map((d) => d.label)).toEqual([
      'Fechada · (2, 2)',
      'Aberta · (4, 2)',
    ]);
    await targets.doors('m1');
    expect(calls.filter((c) => c === 'layers').length).toBe(1);
  });

  it('lists the points of a map', async () => {
    expect((await targets.points('m1')).map((p) => p.name)).toEqual(['Altar', 'Baú do Salão']);
  });

  it('lists the clues of the scenes of every map, with the scene that holds them', async () => {
    expect(await targets.clues()).toEqual([
      { id: 'c1', text: 'O sol nasce a leste.', pointName: 'Altar' },
    ]);
  });
});

describe('SolveTargets after a failed read', () => {
  function setup(api: Record<string, unknown>): SolveTargets {
    TestBed.configureTestingModule({
      providers: [SolveTargets, { provide: MapsClient, useValue: api }],
    });
    const t = TestBed.inject(SolveTargets);
    t.use('camp-1');
    return t;
  }

  it('maps() asks the server again after a failed ListMaps', async () => {
    let n = 0;
    const t = setup({
      list: async () => {
        if (++n === 1) {
          throw new Error('503');
        }
        return [create(MapSchema, { id: 'm1', name: 'A capela' })];
      },
    });
    await expect(t.maps()).rejects.toThrow('503');
    expect(await t.maps()).toEqual([{ id: 'm1', name: 'A capela' }]);
    expect(n).toBe(2);
  });

  it('points() asks the server again after a failed GetMap', async () => {
    let n = 0;
    const t = setup({
      get: async () => {
        if (++n === 1) {
          throw new Error('503');
        }
        return create(GetMapResponseSchema, {});
      },
    });
    await expect(t.points('m1')).rejects.toThrow('503');
    expect(await t.points('m1')).toEqual([]);
    expect(n).toBe(2);
  });

  it('doors() asks the server again after a failed GetMapLayers', async () => {
    let n = 0;
    const t = setup({
      layers: async () => {
        if (++n === 1) {
          throw new Error('503');
        }
        return create(GetMapLayersResponseSchema, {
          gridColumns: 1,
          gridRows: 1,
          wall: new Uint8Array(1),
          difficultTerrain: new Uint8Array(1),
          cover: new Uint8Array(1),
          doors: new Uint8Array(1),
        });
      },
    });
    await expect(t.doors('m1')).rejects.toThrow('503');
    expect(await t.doors('m1')).toEqual([]);
    expect(n).toBe(2);
  });
});
