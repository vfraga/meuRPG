import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';

import { MapPointKind, MapPointSchema } from '../../../../../gen/meurpg/maps/v1/maps_pb';
import { MapsClient } from '../../../../core/maps/maps-client';
import { MapState } from '../../../../core/maps/map-state';
import { TreasurePanel } from './treasure-panel';

const treasure = (found: boolean) =>
  create(MapPointSchema, {
    id: 't1',
    mapId: 'map-A',
    kind: MapPointKind.TREASURE,
    name: 'Baú do fosso',
    description: 'Segredo do mestre',
    treasureValuePo: 50,
    treasureFoundAt: found ? timestampFromDate(new Date()) : undefined,
  });

describe('TreasurePanel', () => {
  function setup() {
    let resolve!: (p: ReturnType<typeof treasure>) => void;
    const calls: string[] = [];
    const maps = {
      markTreasureFound: (_c: string, mapId: string) => {
        calls.push(mapId);
        return new Promise<ReturnType<typeof treasure>>((r) => (resolve = r));
      },
    };
    TestBed.configureTestingModule({ providers: [{ provide: MapsClient, useValue: maps }] });
    const state = new MapState(async (id) => ({ map: { id }, points: [], tokens: [] }) as never);
    state.map.set({ id: 'map-A' } as never);
    state.points.set([treasure(false)]);
    const fixture = TestBed.createComponent(TreasurePanel);
    fixture.componentRef.setInput('campaignId', 'c');
    fixture.componentRef.setInput('state', state);
    fixture.componentRef.setInput('people', [{ id: 'ch1', name: 'Brisa' }]);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const click = (pick: (b: HTMLButtonElement) => boolean) => {
      Array.from(el.querySelectorAll('button')).find(pick)!.click();
      fixture.detectChanges();
    };
    click((b) => b.getAttribute('aria-label')?.includes('como encontrado') ?? false);
    el.querySelector<HTMLInputElement>('input[type=checkbox]')!.click();
    fixture.detectChanges();
    click((b) => b.textContent?.includes('Marcar como encontrado') ?? false);
    return { fixture, state, calls, answer: (p: ReturnType<typeof treasure>) => resolve(p) };
  }

  it('puts the answer on its own map when the map did not change', async () => {
    const { fixture, state, calls, answer } = setup();
    expect(calls).toEqual(['map-A']);
    answer(treasure(true));
    await fixture.whenStable();
    expect(state.points()).toHaveLength(1);
    expect(state.points()[0].treasureFoundAt).toBeDefined();
  });

  it('does not add the treasure of the old map to the new one when the master switched maps before the answer', async () => {
    const { fixture, state, calls, answer } = setup();
    expect(calls).toEqual(['map-A']);
    await state.open('map-B');
    expect(state.map()?.id).toBe('map-B');
    answer(treasure(true));
    await fixture.whenStable();
    expect(state.points().filter((p) => p.mapId === 'map-A')).toEqual([]);
  });
});
