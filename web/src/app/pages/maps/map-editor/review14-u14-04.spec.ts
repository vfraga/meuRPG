import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';

import {
  MapPointKind,
  MapPointSchema,
  TrapState,
} from '../../../../gen/meurpg/maps/v1/maps_pb';
import type { MapPoint } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { TrapTrigger } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { DungeonsClient } from '../../../core/maps/dungeons-client';
import { FakeDungeonsClient } from '../../../core/maps/dungeons-testing';
import { LightPresets } from '../../../core/maps/light-presets';
import { MapState } from '../../../core/maps/map-state';
import { MapsClient } from '../../../core/maps/maps-client';
import { FakeMapsClient, mapMessage, mapPoint, mapResponse } from '../../../core/maps/maps-testing';
import { RosterClient } from '../../../core/maps/roster-client';
import { TrapPresets } from '../../../core/traps/trap-presets';
import { MapEditor } from './map-editor';

const pit = create(MapPointSchema, {
  id: 'pit',
  mapId: 'map-1',
  kind: MapPointKind.TRAP,
  name: 'Fosso escondido',
  xBp: 4800,
  yBp: 5000,
  trap: {
    presetKey: '',
    noticeDc: 15,
    findDc: 15,
    areaSize: 2,
    trigger: TrapTrigger.ENTER,
    state: TrapState.ARMED,
  },
});

// Finding U14-4 in review/unit-14-web-maps.md
class DeferredMapsClient extends FakeMapsClient {
  readonly updates: { resolve: (p: MapPoint) => void; changes: Record<string, unknown> }[] = [];
  override updatePoint(
    _c: string,
    mapId: string,
    pointId: string,
    changes: Parameters<FakeMapsClient['updatePoint']>[3],
  ): Promise<MapPoint> {
    void mapId; void pointId;
    return new Promise((resolve) =>
      this.updates.push({ resolve, changes: changes as Record<string, unknown> }),
    );
  }
}

describe('Review14 U14-4: Salvar ponto answer overwrites a move still in flight', () => {
  it('keeps the dragged position when the save answer (old position) arrives before the move commits', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
    const api = new DeferredMapsClient();
    api.layersResponse = {
      $typeName: 'meurpg.maps.v1.GetMapLayersResponse',
      gridColumns: 24,
      gridRows: 16,
      layersRevision: 1,
      difficultTerrain: new Uint8Array(),
      wall: new Uint8Array(),
      cover: new Uint8Array(),
      light: new Uint8Array(),
      doors: new Uint8Array(),
      fogWithheld: false,
    };
    const state = new MapState(async () =>
      mapResponse(
        mapMessage('map-1', 'Caverna', { gridColumns: 24, gridRows: 16, fogEnabled: false }),
        [pit],
        [],
      ),
    );
    await state.open('map-1');
    TestBed.configureTestingModule({
      providers: [
        { provide: MapsClient, useValue: api },
        { provide: DungeonsClient, useValue: new FakeDungeonsClient() },
        { provide: RosterClient, useValue: { list: () => Promise.resolve([]) } },
        { provide: LightPresets, useValue: { list: () => Promise.resolve([]) } },
        { provide: TrapPresets, useValue: { list: () => Promise.resolve({ presets: [], severities: [] }) } },
      ],
    });
    const fixture = TestBed.createComponent(MapEditor);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('state', state);
    fixture.componentRef.setInput('combatRunning', false);
    fixture.componentRef.setInput('sessionNumber', null);
    fixture.detectChanges();
    const el: HTMLElement = fixture.nativeElement;
    const settle = async () => {
      fixture.detectChanges();
      await vi.advanceTimersByTimeAsync(0);
      await fixture.whenStable();
      fixture.detectChanges();
    };
    await settle();
    Array.from(el.querySelectorAll<HTMLElement>('button.pl__row'))
      .find((r) => r.textContent?.includes('Fosso escondido'))!
      .click();
    await settle();
    const field = Array.from(el.querySelectorAll('mat-form-field'))
      .find((f) => f.querySelector('mat-label')?.textContent?.trim() === 'CD para achar (Investigação)')!
      .querySelector('input')!;
    field.value = '12';
    field.dispatchEvent(new Event('input'));
    await settle();

    const comp = fixture.componentInstance as unknown as {
      onMoved(m: unknown): Promise<void>;
      save(): Promise<boolean>;
    };
    // 1. the master drags the trap; the move request stays in flight.
    const moving = comp.onMoved({ kind: 'point', id: 'pit', xBp: 1000, yBp: 2000 });
    await settle();
    expect(state.points().find((p) => p.id === 'pit')!.xBp).toBe(1000);
    // 2. still in flight, the master presses "Salvar ponto" for the panel change.
    const saving = comp.save();
    await settle();
    expect(api.updates.length).toBe(2);
    // 3. the server answered the save before the move committed: old position.
    api.updates[1]!.resolve(mapPoint('pit', 'Fosso escondido', { xBp: 4800, yBp: 5000, kind: MapPointKind.TRAP }));
    await saving;
    // 4. then the move commits.
    api.updates[0]!.resolve(mapPoint('pit', 'Fosso escondido', { xBp: 1000, yBp: 2000, kind: MapPointKind.TRAP }));
    await moving;
    await settle();
    // The server finally holds (1000, 2000); the screen must agree.
    const shown = state.points().find((p) => p.id === 'pit')!;
    expect([shown.xBp, shown.yBp]).toEqual([1000, 2000]);
    vi.useRealTimers();
  });
});
