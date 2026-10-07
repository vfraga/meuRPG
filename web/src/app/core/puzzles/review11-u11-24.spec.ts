// Finding U11-24: SolveTargets caches a rejected promise (mapsPromise, layerCache, mapCache), so one failed read is never retried.
import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';

import { GetMapLayersResponseSchema, GetMapResponseSchema, MapSchema } from '../../../gen/meurpg/maps/v1/maps_pb';
import { MapsClient } from '../maps/maps-client';
import { SolveTargets } from './solve-targets';

describe('Review11 U11-24: SolveTargets caches rejected reads', () => {
  function setup(api: Record<string, unknown>): SolveTargets {
    TestBed.configureTestingModule({ providers: [SolveTargets, { provide: MapsClient, useValue: api }] });
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
