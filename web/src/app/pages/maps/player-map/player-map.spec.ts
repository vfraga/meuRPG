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

describe('PlayerMap: a map with no fog', () => {
  async function render(): Promise<HTMLElement> {
    const api = new FakeMapsClient();
    const trap = mapPoint('t1', 'Fosso escondido', {
      kind: MapPointKind.TRAP,
      revealed: false,
      xBp: 3000,
      yBp: 3000,
      trap: { state: TrapState.TRIGGERED, areaSize: 2 } as never,
    });
    const chest = mapPoint('c1', 'Baú de moedas', {
      kind: MapPointKind.TREASURE,
      revealed: false,
      treasureFoundAt: { seconds: 1n, nanos: 0 } as never,
      xBp: 7000,
      yBp: 7000,
    });
    const state = new MapState(async () =>
      mapResponse(
        mapMessage('map-1', 'A caverna', { gridColumns: 24, gridRows: 16, fogEnabled: false }),
        [trap, chest],
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
    return fixture.nativeElement as HTMLElement;
  }

  it("draws a fired trap and a found treasure as the map's own marks (the area, the chest), not a bare name", async () => {
    const el = await render();
    expect(el.querySelector('app-map-pins .area')).not.toBeNull();
    expect(el.querySelector('app-map-pins .pin--found')).not.toBeNull();
    expect(el.querySelectorAll('app-map-marker')).toHaveLength(2);
    // The marker is only the hit area: the mark under it is drawn by the pins.
    expect(el.querySelectorAll('app-map-marker .pt__shape--pin')).toHaveLength(2);
  });

  it('names the marks in the legend, and draws the found chest solid, not "escondido"', async () => {
    const el = await render();
    expect(el.querySelector('app-map-pins-legend')?.textContent).toContain('Tesouro encontrado');
    expect(el.querySelector('app-map-pins .pin--found')?.classList).not.toContain('pin--hidden');
    expect(el.querySelector('.lbl--hidden')).toBeNull();
  });
});

describe('PlayerMap: the second lock on what a player may see (RN-10)', () => {
  /** Hidden things that slipped past the server: none of them may reach the pins, the list or the sheet. */
  async function render() {
    const api = new FakeMapsClient();
    const points = [
      mapPoint('p-scene', 'Taverna revelada', {
        kind: MapPointKind.SCENE,
        revealed: true,
        xBp: 1000,
        yBp: 1000,
      }),
      mapPoint('p-trap-hidden', 'Armadilha secreta', {
        kind: MapPointKind.TRAP,
        revealed: false,
        xBp: 3000,
        yBp: 3000,
        trap: { state: TrapState.ARMED, areaSize: 1 } as never,
      }),
      mapPoint('p-light', 'Luz do mestre', {
        kind: MapPointKind.LIGHT,
        revealed: false,
        xBp: 5000,
        yBp: 5000,
      }),
      mapPoint('p-treasure-hidden', 'Tesouro oculto', {
        kind: MapPointKind.TREASURE,
        revealed: false,
        xBp: 6000,
        yBp: 6000,
      }),
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

  it('draws pins only for the points the player knows', async () => {
    const el = (await render()).nativeElement as HTMLElement;
    const ids = [...el.querySelectorAll('app-map-pins [data-pin-of]')]
      .map((n) => n.getAttribute('data-pin-of'))
      .sort();
    expect(ids).toEqual(['p-trap-fired', 'p-trap-mine']);
  });

  it('lists only the points the player knows, by name', async () => {
    const el = (await render()).nativeElement as HTMLElement;
    const names = [...el.querySelectorAll('.list__name')].map((n) => n.textContent?.trim());
    expect(names).not.toContain('Armadilha secreta');
    expect(names).not.toContain('Luz do mestre');
    expect(names).not.toContain('Tesouro oculto');
    expect(names).toContain('Taverna revelada');
    expect(names).toContain('Fosso disparado');
    expect(names).toContain('Armadilha conhecida');
  });

  it('does not open the sheet of a hidden point', async () => {
    const fixture = await render();
    (fixture.componentInstance as unknown as { openPoint(id: string): void }).openPoint(
      'p-trap-hidden',
    );
    fixture.detectChanges();
    expect((fixture.nativeElement as HTMLElement).querySelector('app-point-sheet')).toBeNull();
  });
});
