// Finding U14-5 in review/unit-14-web-maps.md
import { create } from '@bufbuild/protobuf';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { By } from '@angular/platform-browser';

import { CharacterKind } from '../../../../gen/meurpg/characters/v1/characters_pb';
import {
  MapLayer,
  MapPointKind,
  MapPointSchema,
  TrapState,
} from '../../../../gen/meurpg/maps/v1/maps_pb';
import { TrapTrigger } from '../../../../gen/meurpg/rules/v1/rules_pb';
import type { GetDungeonRoomsResponse } from '../../../../gen/meurpg/maps/v1/dungeons_pb';
import { DungeonsClient } from '../../../core/maps/dungeons-client';
import { FakeDungeonsClient } from '../../../core/maps/dungeons-testing';
import { LightPresets } from '../../../core/maps/light-presets';
import { MapState } from '../../../core/maps/map-state';
import { MapsClient } from '../../../core/maps/maps-client';
import {
  FakeMapsClient,
  mapMessage,
  mapPoint,
  mapResponse,
  mapToken,
} from '../../../core/maps/maps-testing';
import { RosterClient } from '../../../core/maps/roster-client';
import { TrapPresets } from '../../../core/traps/trap-presets';
import { PaintSurface } from '../paint-surface/paint-surface';
import { MapEditor } from './map-editor';

const roster = [
  {
    id: 'c-pensantus',
    name: 'Pensantus',
    kind: CharacterKind.PLAYER,
    playerUserId: 'u1',
    classSummary: 'Mago 3',
    raceName: 'Gnomo',
    playerName: 'Vinicius',
  },
  {
    id: 'c-toren',
    name: 'Toren',
    kind: CharacterKind.PLAYER,
    playerUserId: 'u2',
    classSummary: 'Guerreiro 5',
    raceName: 'Humano',
    playerName: 'Caio',
  },
];

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
const chest = create(MapPointSchema, {
  id: 'chest',
  mapId: 'map-1',
  kind: MapPointKind.TREASURE,
  name: 'Baú de moedas',
  xBp: 7000,
  yBp: 8000,
  treasureValuePo: 250,
});
const torch = create(MapPointSchema, {
  id: 'torch',
  mapId: 'map-1',
  kind: MapPointKind.LIGHT,
  name: 'Tocha da guarita',
  xBp: 8000,
  yBp: 3000,
  light: { presetKey: 'light:torch', brightFt: 20, dimFt: 20 },
});
const tavern = mapPoint('tavern', 'Taverna', { kind: MapPointKind.SCENE });

describe('Review14 U14-5: the door tool paints one rules square on a calibrated map', () => {
  let fixture: ComponentFixture<MapEditor>;
  let el: HTMLElement;
  let api: FakeMapsClient;
  let dungeons: FakeDungeonsClient;
  let state: MapState;

  async function setup(
    mapPartial: Parameters<typeof mapMessage>[2] = {
      gridColumns: 24,
      gridRows: 16,
      fogEnabled: false,
    },
    inputs: { combatRunning?: boolean; sessionNumber?: number | null } = {},
    layers: { wall?: Uint8Array; doors?: Uint8Array } = {},
    dungeonRooms: GetDungeonRoomsResponse | null = null,
  ) {
    api = new FakeMapsClient();
    // An ordinary map: the generator did not make it, so `GetDungeonRooms` is `not_found` and the editor shows nothing of a dungeon.
    dungeons = new FakeDungeonsClient();
    dungeons.roomsAnswer = dungeonRooms;
    const map = mapMessage('map-1', 'A caverna do Vale Seco', {
      ...mapPartial,
      generatedDungeon: dungeonRooms !== null,
    });
    api.layersResponse = {
      $typeName: 'meurpg.maps.v1.GetMapLayersResponse',
      gridColumns: mapPartial.gridColumns ?? 0,
      gridRows: mapPartial.gridRows ?? 0,
      layersRevision: 1,
      difficultTerrain: new Uint8Array(),
      wall: new Uint8Array(),
      cover: new Uint8Array(),
      light: new Uint8Array(),
      doors: new Uint8Array(),
      fogWithheld: false,
      ...layers,
    };
    state = new MapState(async () =>
      mapResponse(map, [tavern, pit, chest, torch], [mapToken('c-pensantus', 'Pensantus')]),
    );
    await state.open('map-1');
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: MapsClient, useValue: api },
        { provide: DungeonsClient, useValue: dungeons },
        { provide: RosterClient, useValue: { list: () => Promise.resolve(roster) } },
        {
          provide: LightPresets,
          useValue: {
            list: () =>
              Promise.resolve([
                {
                  key: 'light:torch',
                  name: 'Tocha',
                  radii: '6 m claro + 6 m de penumbra',
                  brightFt: 20,
                  dimFt: 20,
                },
              ]),
          },
        },
        {
          provide: TrapPresets,
          useValue: { list: () => Promise.resolve({ presets: [], severities: [] }) },
        },
      ],
    });
    fixture = TestBed.createComponent(MapEditor);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('state', state);
    fixture.componentRef.setInput('combatRunning', inputs.combatRunning ?? false);
    fixture.componentRef.setInput('sessionNumber', inputs.sessionNumber ?? null);
    fixture.detectChanges();
    el = fixture.nativeElement;
    await settle();
  }
  // Fake `setTimeout`: the paint queue (150 ms) and the counts read (250 ms) are waited for by moving the
  // clock, not by sleeping. `requestAnimationFrame` stays real (see `frame`).
  beforeEach(() => vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] }));
  afterEach(() => vi.useRealTimers());
  const settle = async () => {
    fixture.detectChanges();
    await vi.advanceTimersByTimeAsync(0);
    await fixture.whenStable();
    fixture.detectChanges();
  };
  const button = (t: string, last = false) => {
    const all = Array.from(el.querySelectorAll<HTMLElement>('button')).filter((b) =>
      b.textContent?.trim().endsWith(t),
    );
    return (last ? all[all.length - 1] : all[0])!;
  };
  const radio = (t: string) =>
    Array.from(el.querySelectorAll<HTMLElement>('[role="radio"]')).find((b) =>
      b.textContent?.trim().endsWith(t),
    )!;
  const surface = () =>
    fixture.debugElement.query(By.directive(PaintSurface))?.componentInstance as
      PaintSurface | undefined;
  /** The painted squares are drawn once a frame: let one go by. */
  const frame = () => new Promise((r) => requestAnimationFrame(() => r(null)));
  // The paint queue and the counts run on the fake clock; the painted squares are drawn on a real animation frame,
  // so wait for one too (with the clock alone, a fast machine checked the text before any frame was drawn).
  const flush = async () => {
    await vi.advanceTimersByTimeAsync(400);
    await frame();
    await settle();
  };

  // 8 x 8 rules' grid = 4 x 4 drawing squares x factor 2. A one-square wall along column 4, floor on both sides.
  const wall = () => {
    const bytes = new Uint8Array(8);
    for (let row = 0; row < 8; row++) {
      const n = row * 8 + 4;
      bytes[n >> 3] |= 1 << (n & 7);
    }
    return bytes;
  };
  const tap = async (col: number, row: number) => {
    surface()!.stroke.emit({ centers: [{ col, row }], erase: false });
    await frame();
    await settle();
  };
  async function paintMode(doors?: Uint8Array) {
    await setup(
      { gridColumns: 8, gridRows: 8, drawnColumns: 4, drawnRows: 4, squareFactor: 2, fogEnabled: false },
      undefined,
      { wall: wall(), doors },
    );
    radio('Pintar').click();
    await settle();
    button('Porta').click();
    await settle();
  }
  const doorSquares = (calls: typeof api.paints) =>
    calls
      .filter((c) => c.layer === MapLayer.DOORS)
      .flatMap((c) => c.squares.map((q) => `${q.col},${q.row}`))
      .sort();

  it('a door tap paints and sends the whole 2 x 2 block, as the server does (layers.go: a door is a whole square of the drawing)', async () => {
    await paintMode();
    await tap(4, 2);
    await flush();
    // The server paints bc,br = (4,2) .. (5,3). What the editor sends / shows must be that block.
    expect(doorSquares(api.paints)).toEqual(['4,2', '4,3', '5,2', '5,3']);
    // The master's local copy of the door layer: the whole block, not one small square.
    expect(el.querySelectorAll('.sq--door').length).toBe(4);
  });

  it('"Tirar a porta" on one square of a door block also takes the whole block', async () => {
    const doors = new Uint8Array(32);
    for (const [c, r] of [[4, 2], [5, 2], [4, 3], [5, 3]]) {
      const n = r * 8 + c;
      doors[n >> 1] |= 2 << (4 * (n & 1));
    }
    await paintMode(doors);
    button('Tirar a porta').click();
    await settle();
    await tap(5, 3);
    await flush();
    expect(doorSquares(api.paints.filter((p) => p.value === 0))).toEqual(['4,2', '4,3', '5,2', '5,3']);
    expect(el.querySelectorAll('.sq--door').length).toBe(0);
  });
});
