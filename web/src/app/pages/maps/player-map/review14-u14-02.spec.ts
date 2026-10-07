// Finding U14-2 in review/unit-14-web-maps.md
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import { MapPointKind, TrapState } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { MapState } from '../../../core/maps/map-state';
import { MapsClient } from '../../../core/maps/maps-client';
import {
  FakeMapsClient,
  mapMessage,
  mapPoint,
  mapResponse,
  mapToken,
} from '../../../core/maps/maps-testing';
import { PlayerMap } from './player-map';

describe('Review14 U14-2: PlayerMap pins, list and sheet ignore the second lock', () => {
  async function render() {
    const api = new FakeMapsClient();
    const points = [
      mapPoint('p-scene', 'Taverna revelada', { kind: MapPointKind.SCENE, revealed: true, xBp: 1000, yBp: 1000 }),
      mapPoint('p-trap-hidden', 'Armadilha secreta', {
        kind: MapPointKind.TRAP,
        revealed: false,
        xBp: 3000,
        yBp: 3000,
        trap: { state: TrapState.ARMED, areaSize: 1 } as never,
      }),
      mapPoint('p-light', 'Luz do mestre', { kind: MapPointKind.LIGHT, revealed: false, xBp: 5000, yBp: 5000 }),
      mapPoint('p-treasure-hidden', 'Tesouro oculto', { kind: MapPointKind.TREASURE, revealed: false, xBp: 6000, yBp: 6000 }),
      // Legitimately known: must stay drawn.
      mapPoint('p-trap-fired', 'Fosso disparado', {
        kind: MapPointKind.TRAP,
        revealed: false,
        xBp: 8000,
        yBp: 8000,
        trap: { state: TrapState.TRIGGERED, areaSize: 1 } as never,
      }),
      mapPoint('p-trap-mine', 'Armadilha conhecida', {
        kind: MapPointKind.TRAP,
        revealed: false,
        xBp: 9000,
        yBp: 2000,
        trapRevealedTo: ['c-1'] as never,
        trap: { state: TrapState.ARMED, areaSize: 1 } as never,
      }),
    ];
    const state = new MapState(async () =>
      mapResponse(
        mapMessage('map-1', 'A caverna', { gridColumns: 24, gridRows: 16, fogEnabled: false }),
        points,
        [mapToken('c-1', 'Pensantus', { mine: true })],
      ),
    );
    await state.open('map-1');
    TestBed.configureTestingModule({
      providers: [provideRouter([]), { provide: MapsClient, useValue: api }],
    });
    const fixture = TestBed.createComponent(PlayerMap);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('state', state);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    return fixture;
  }

  it('draws pins only for known points', async () => {
    const el = (await render()).nativeElement as HTMLElement;
    const ids = [...el.querySelectorAll('app-map-pins [data-pin-of]')].map((n) => n.getAttribute('data-pin-of')).sort();
    expect(ids).toEqual(['p-trap-fired', 'p-trap-mine']);
  });

  it('lists only known points by name', async () => {
    const el = (await render()).nativeElement as HTMLElement;
    const names = [...el.querySelectorAll('.list__name')].map((n) => n.textContent?.trim());
    expect(names).not.toContain('Armadilha secreta');
    expect(names).not.toContain('Luz do mestre');
    expect(names).not.toContain('Tesouro oculto');
    expect(names).toContain('Taverna revelada');
    expect(names).toContain('Fosso disparado');
    expect(names).toContain('Armadilha conhecida');
  });

  it('does not open the sheet for a hidden point', async () => {
    const fixture = await render();
    (fixture.componentInstance as unknown as { openPoint(id: string): void }).openPoint('p-trap-hidden');
    fixture.detectChanges();
    expect((fixture.nativeElement as HTMLElement).querySelector('app-point-sheet')).toBeNull();
  });
});
