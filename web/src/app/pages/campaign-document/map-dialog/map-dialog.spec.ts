import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import { MapPointKind, TrapState } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { mapPoint } from '../../../core/maps/maps-testing';
import {
  DocumentLinks,
  type MapView,
  type OpenSessionMap,
} from '../../../shared/campaign-document/document-clients';
import { DocumentMapDialog } from './map-dialog';

const IMAGE = { url: '/images/img-1', width: 2400, height: 1600 };

class FakeLinks {
  map: MapView = {
    id: 'map-1',
    name: 'Mirathel e arredores',
    revealed: true,
    image: IMAGE,
    gridColumns: 24,
    gridRows: 16,
    points: [
      mapPoint('p1', 'Emboscada na estrada', { revealed: true, kind: 1 }),
      mapPoint('p2', 'Covil dos goblins', { revealed: false, kind: 2 }),
    ],
  };
  session: OpenSessionMap | null = { sessionNumber: 4, mapId: 'map-1' };
  getMap = () => Promise.resolve(this.map);
  openSessionMap = () => Promise.resolve(this.session);
}

describe('DocumentMapDialog (E5-29)', () => {
  let links: FakeLinks;

  beforeEach(() => {
    links = new FakeLinks();
    TestBed.configureTestingModule({
      imports: [DocumentMapDialog],
      providers: [provideRouter([]), { provide: DocumentLinks, useValue: links }],
    });
  });

  async function render(): Promise<HTMLElement> {
    const fixture = TestBed.createComponent(DocumentMapDialog);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('mapId', 'map-1');
    fixture.componentRef.setInput('text', 'Mirathel e arredores');
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }

  it('says the map is revealed and the open session is on it', async () => {
    const el = await render();
    expect(el.querySelector('.doc-dialog__sub')?.textContent?.trim()).toBe(
      'visibilityRevelado aos jogadores. Mapa atual da Sessão 4.',
    );
  });

  it('leaves the session out when the open session is on another map, or there is none', async () => {
    links.session = { sessionNumber: 4, mapId: 'map-2' };
    let el = await render();
    expect(el.querySelector('.doc-dialog__sub span')?.textContent).toBe('Revelado aos jogadores.');

    TestBed.resetTestingModule();
    links = new FakeLinks();
    links.session = null;
    links.map = { ...links.map, revealed: false };
    TestBed.configureTestingModule({
      imports: [DocumentMapDialog],
      providers: [provideRouter([]), { provide: DocumentLinks, useValue: links }],
    });
    el = await render();
    expect(el.querySelector('.doc-dialog__sub span')?.textContent).toBe('Escondido dos jogadores.');
    expect(el.querySelector('.doc-dialog__sub mat-icon')?.textContent).toBe('visibility_off');
  });

  it("draws a trap and a found treasure with the map's own marks, not a floating name: the pins, and a marker that is only a hit area", async () => {
    links.map = {
      ...links.map,
      points: [
        mapPoint('t1', 'Fosso escondido', {
          kind: MapPointKind.TRAP,
          revealed: true,
          xBp: 3000,
          yBp: 3000,
          trap: { state: TrapState.ARMED, areaSize: 2 } as never,
        }),
        mapPoint('c1', 'Baú de moedas', {
          kind: MapPointKind.TREASURE,
          revealed: false,
          treasureFoundAt: { seconds: 1n, nanos: 0 } as never,
          xBp: 7000,
          yBp: 7000,
        }),
      ],
    };
    const el = await render();
    expect(el.querySelector('app-map-pins .area')).not.toBeNull();
    expect(el.querySelector('app-map-pins .pin--found')).not.toBeNull();
    expect(el.querySelector('app-map-pins-legend')?.textContent).toContain('Tesouro encontrado');
    // The marker draws no icon of its own on top of the pin.
    expect(el.querySelectorAll('app-map-marker .pt__shape--pin')).toHaveLength(2);
  });

  it('draws every point, hidden ones too, and names them for a screen reader', async () => {
    const el = await render();
    expect(el.querySelectorAll('app-map-marker').length).toBe(2);
    const list = el.querySelector('ul[aria-label="Pontos deste mapa"]');
    expect(list?.textContent).toContain('Emboscada na estrada, Batalha');
    expect(list?.textContent).toContain('Covil dos goblins, Submapa, escondido');
    // Read-only: the points are not buttons here.
    expect(el.querySelectorAll('app-map-marker button').length).toBe(0);
  });

  it('shows the legend with the points, and always the editor link', async () => {
    let el = await render();
    expect(el.querySelector('app-map-legend')).not.toBeNull();
    expect(el.querySelector('a[href="/campaigns/camp-1/maps/map-1"]')?.textContent).toContain(
      'Abrir no editor de mapas',
    );

    TestBed.resetTestingModule();
    links = new FakeLinks();
    links.map = { ...links.map, points: [] };
    TestBed.configureTestingModule({
      imports: [DocumentMapDialog],
      providers: [provideRouter([]), { provide: DocumentLinks, useValue: links }],
    });
    el = await render();
    expect(el.querySelector('app-map-legend')).toBeNull();
    expect(el.querySelector('ul[aria-label="Pontos deste mapa"]')).toBeNull();
  });
});
