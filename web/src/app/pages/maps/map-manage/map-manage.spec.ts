import { ComponentFixture, TestBed } from '@angular/core/testing';

import type { GetMapLayersResponse } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { MapState } from '../../../core/maps/map-state';
import { MapsClient } from '../../../core/maps/maps-client';
import { FakeMapsClient, mapMessage, mapResponse, mapToken } from '../../../core/maps/maps-testing';
import { RosterClient } from '../../../core/maps/roster-client';
import { MapManage } from './map-manage';

describe("MapManage: the master's map on a phone (E9-01 7)", () => {
  let fixture: ComponentFixture<MapManage>;
  let el: HTMLElement;
  let api: FakeMapsClient;

  async function setup(gridColumns: number) {
    api = new FakeMapsClient();
    api.layersResponse = {
      $typeName: 'meurpg.maps.v1.GetMapLayersResponse',
      gridColumns,
      gridRows: gridColumns > 0 ? 16 : 0,
      layersRevision: 1,
      // Square 0 is a wall, and squares 1 and 2 are difficult terrain.
      difficultTerrain: Uint8Array.of(0b0000_0110),
      wall: Uint8Array.of(0b0000_0001),
      cover: new Uint8Array(),
      light: new Uint8Array(),
      fogWithheld: false,
    } as never;
    const state = new MapState(async () =>
      mapResponse(
        mapMessage('map-1', 'A caverna do Vale Seco', {
          gridColumns,
          gridRows: gridColumns > 0 ? 16 : 0,
        }),
        [],
        [mapToken('c1', 'Pensantus')],
      ),
    );
    await state.open('map-1');
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: MapsClient, useValue: api },
        { provide: RosterClient, useValue: { list: () => Promise.resolve([]) } },
      ],
    });
    fixture = TestBed.createComponent(MapManage);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('state', state);
    fixture.detectChanges();
    el = fixture.nativeElement;
    await fixture.whenStable();
    await new Promise((r) => setTimeout(r));
    fixture.detectChanges();
    return state;
  }
  const text = () => (el.textContent ?? '').replace(/\s+/g, ' ');

  it('says the painting is for the computer, with a fixed notice, and has no paint tools', async () => {
    await setup(24);
    expect(text()).toContain('Pintar só no computador');
    expect(text()).toContain('No celular você vê as camadas e muda a névoa.');
    expect(el.querySelector('app-editor-bar')).toBeNull();
    expect(el.querySelector('app-paint-surface')).toBeNull();
    expect(
      Array.from(el.querySelectorAll('[role="radio"]')).some((r) =>
        r.textContent?.includes('Pintar'),
      ),
    ).toBe(false);
  });

  it('draws the layers the server has, and names them in the legend', async () => {
    await setup(24);
    expect(el.querySelectorAll('app-editor-overlay .sq--wall')).toHaveLength(1);
    expect(el.querySelectorAll('app-editor-overlay .sq--terrain')).toHaveLength(2);
    const legend = Array.from(el.querySelectorAll('app-map-layers-legend li'), (li) =>
      li.textContent?.trim(),
    );
    expect(legend).toEqual(['Parede', 'Terreno difícil']);
  });

  it('has the fog settings, editable', async () => {
    await setup(24);
    expect(text()).toContain('Névoa de guerra');
    const fog = el.querySelector('[role="switch"]') as HTMLElement;
    expect(fog.getAttribute('aria-disabled')).not.toBe('true');
  });

  it('reads no layers for a map without a grid, and the fog cannot be turned on', async () => {
    await setup(0);
    expect(api.calls.some((c) => c.startsWith('layers'))).toBe(false);
    expect(text()).toContain('Precisa da grade definida.');
  });

  it('draws the layers of the latest map change when an older answer comes last', async () => {
    const state = await setup(24);
    const pending: ((value: GetMapLayersResponse) => void)[] = [];
    api.layers = () =>
      new Promise((resolve) => {
        pending.push(resolve);
      });
    const changed = (layersRevision: number) =>
      state.setMap(
        mapMessage('map-1', 'A caverna do Vale Seco', {
          gridColumns: 24,
          gridRows: 16,
          layersRevision,
        }),
      );
    changed(2);
    fixture.detectChanges();
    changed(3);
    fixture.detectChanges();
    expect(pending).toHaveLength(2);
    const walls = (wall: number) =>
      ({ ...api.layersResponse, layersRevision: 3, wall: Uint8Array.of(wall) }) as never;
    // The newer answer (one wall) is back first, the older one (two walls) after it.
    pending[1]!(walls(0b0000_0001));
    await new Promise((r) => setTimeout(r));
    pending[0]!(walls(0b0000_0011));
    await new Promise((r) => setTimeout(r));
    fixture.detectChanges();
    expect(el.querySelectorAll('app-editor-overlay .sq--wall')).toHaveLength(1);
  });
});
