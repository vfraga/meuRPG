import { create } from '@bufbuild/protobuf';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { By } from '@angular/platform-browser';

import { CharacterKind } from '../../../../gen/meurpg/characters/v1/characters_pb';
import {
  LightLevel,
  MapLayer,
  MapPointKind,
  MapPointSchema,
  TrapState,
} from '../../../../gen/meurpg/maps/v1/maps_pb';
import type { MapPoint } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { TrapTrigger } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { Code, ConnectError } from '@connectrpc/connect';
import type { GetDungeonRoomsResponse } from '../../../../gen/meurpg/maps/v1/dungeons_pb';
import { DungeonsClient } from '../../../core/maps/dungeons-client';
import { FakeDungeonsClient, roomsResponse } from '../../../core/maps/dungeons-testing';
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
import { visionResponse } from '../../../core/maps/vision-testing';
import { TrapPresets } from '../../../core/traps/trap-presets';
import { MapView } from '../../../shared/map-view/map-view';
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

describe('MapEditor', () => {
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
  const text = () => (el.textContent ?? '').replace(/\u00a0/g, ' ').replace(/\s+/g, ' ');
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

  describe('a generated dungeon (MR-010, E10-05 5 and 6)', () => {
    it('never asks about the rooms of a map the server does not say is a generated dungeon, and shows nothing of one', async () => {
      await setup();
      expect(dungeons.calls).toEqual([]);
      expect(el.querySelector('app-dungeon-rooms')).toBeNull();
      expect(el.querySelector('app-dungeon-image')).toBeNull();
    });

    it('lists the rooms beside the map, with the "Imagem" panel under it, on the map the server says is a dungeon', async () => {
      await setup(undefined, {}, {}, roomsResponse());
      expect(dungeons.calls).toEqual(['rooms map-1']);
      expect(el.querySelector('.layout__side app-dungeon-rooms')).toBeTruthy();
      expect(el.querySelector('.layout__side app-dungeon-image')).toBeTruthy();
      expect(text()).toContain('Sala 1');
      expect(text()).toContain('Porta com armadilha');
      expect(text()).toContain('Gerada pelo app');
    });

    it('outlines the chosen room on the map with the solid frame, and lets go', async () => {
      await setup(undefined, {}, {}, roomsResponse());
      expect(el.querySelector('app-editor-overlay svg.room')).toBeNull();
      el.querySelectorAll<HTMLButtonElement>('app-dungeon-rooms .room__head')[1]!.click();
      await settle();
      const rect = el.querySelector('app-editor-overlay svg.room rect')!;
      expect([
        rect.getAttribute('x'),
        rect.getAttribute('y'),
        rect.getAttribute('width'),
        rect.getAttribute('height'),
      ]).toEqual(['5', '1', '5', '3']);
      el.querySelectorAll<HTMLButtonElement>('app-dungeon-rooms .room__head')[1]!.click();
      await settle();
      expect(el.querySelector('app-editor-overlay svg.room')).toBeNull();
    });

    it('puts a scene on the map at once and reads the rooms again', async () => {
      await setup(undefined, {}, {}, roomsResponse());
      const before = dungeons.calls.length;
      Array.from(el.querySelectorAll<HTMLButtonElement>('app-dungeon-rooms button'))
        .find((b) => b.textContent?.trim() === 'Pôr uma cena nesta sala')!
        .click();
      await settle();
      await settle();
      expect(state.points().some((p) => p.name === 'Sala 1')).toBe(true);
      expect(dungeons.calls.slice(before)).toEqual(['placeScene map-1 1', 'rooms map-1']);
    });

    it('"Pintar" keeps the "Imagem" panel and drops the rooms list (the side column is the layers and the grid)', async () => {
      await setup(undefined, {}, {}, roomsResponse());
      radio('Pintar').click();
      await settle();
      expect(el.querySelector('app-dungeon-image')).toBeTruthy();
      expect(el.querySelector('app-dungeon-rooms')).toBeNull();
      expect(text()).toContain('Camadas');
    });

    it('"Redesenhar" puts the map with the new image in the page\'s state, and the layers are not read again', async () => {
      await setup(undefined, {}, {}, roomsResponse());
      const layersBefore = api.calls.filter((c) => c.startsWith('layers')).length;
      button('Redesenhar').click();
      await settle();
      button('Redesenhar', true).click();
      await settle();
      await settle();
      expect(dungeons.calls).toContain('redraw map-1');
      expect(state.map()?.revision).toBe(2);
      expect(api.calls.filter((c) => c.startsWith('layers')).length).toBe(layersBefore);
    });

    it('says so, with a way to try again, when the rooms cannot be read', async () => {
      await setup(undefined, {}, {}, roomsResponse());
      dungeons.failWith.set('rooms', new ConnectError('down', Code.Unavailable));
      (fixture.componentInstance as unknown as { reloadDungeon(): void }).reloadDungeon();
      await settle();
      expect(el.querySelector('[role="alert"]')?.textContent).toContain(
        'Não deu para ler as salas da masmorra.',
      );
      dungeons.failWith.clear();
      button('Tentar de novo').click();
      await settle();
      expect(el.querySelector('app-dungeon-rooms')).toBeTruthy();
    });

    it('goes to the chosen room on the map and back to the whole map when it is let go', async () => {
      await setup(undefined, {}, {}, roomsResponse());
      const view = fixture.debugElement.query(By.directive(MapView)).componentInstance as MapView;
      const focusOn = vi.spyOn(view, 'focusOn');
      const fit = vi.spyOn(view, 'fit');
      el.querySelectorAll<HTMLButtonElement>('app-dungeon-rooms .room__head')[1]!.click();
      await settle();
      expect(focusOn).toHaveBeenCalledTimes(1);
      el.querySelectorAll<HTMLButtonElement>('app-dungeon-rooms .room__head')[1]!.click();
      await settle();
      expect(fit).toHaveBeenCalled();
    });

    it('names the stairs for themselves in the legend and in the list ("Escada", never "Submapa")', async () => {
      await setup({ gridColumns: 11, gridRows: 9, fogEnabled: false }, {}, {}, roomsResponse());
      state.upsertPoint(
        mapPoint('stair-up', 'Escada para cima', { kind: MapPointKind.SUBMAP, stairs: 1 }),
      );
      await settle();
      const legend = (el.querySelector('.legend')?.textContent ?? '').replace(/\s+/g, ' ');
      expect(legend).toContain('Escada para cima');
      expect(legend).not.toContain('Submapa');
      const row = Array.from(el.querySelectorAll('.pl__row')).find((r) =>
        r.textContent?.includes('Escada para cima'),
      )!;
      expect(row.textContent).toContain('Escada');
      expect(row.textContent).not.toContain('Submapa');
    });
  });

  describe('dragging a token', () => {
    it("saves a creature's move by its creature id, and an owner's by the character id", async () => {
      await setup();
      state.upsertToken(mapToken('c-pensantus', 'Corvo', { creatureId: 'raven' }));
      const view = fixture.debugElement.query(By.directive(MapView)).componentInstance as MapView;
      view.moved.emit({ kind: 'token', id: 'raven', xBp: 3000, yBp: 3000 });
      view.moved.emit({ kind: 'token', id: 'c-pensantus', xBp: 7000, yBp: 7000 });
      await settle();
      expect(api.calls).toContain('placeToken map-1 creature:raven 3000 3000');
      expect(api.calls).toContain('placeToken map-1 c-pensantus 7000 7000');
    });
  });

  describe("a creature's token and its owner's", () => {
    // The creature's `character_id` is its owner's: the two share it, and the creature comes first in the list.
    async function withCreature() {
      await setup();
      state.removeToken('c-pensantus');
      state.upsertToken(mapToken('c-pensantus', 'Corvo', { creatureId: 'raven' }));
      state.upsertToken(mapToken('c-pensantus', 'Pensantus'));
      const view = fixture.debugElement.query(By.directive(MapView)).componentInstance as MapView;
      return view;
    }
    const removeButton = () =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        b.textContent?.includes('Remover do mapa'),
      )!;

    it('takes the owner off the map when the owner is the one selected, not the creature that shares its character id', async () => {
      const view = await withCreature();
      view.tokenSelect.emit('c-pensantus');
      await settle();
      expect(el.querySelector('#tk-title')?.textContent).toContain('Pensantus');
      removeButton().click();
      await settle();
      expect(api.calls).toContain('removeToken map-1 c-pensantus');
      expect(state.tokens().map((t) => t.name)).toEqual(['Corvo']);
    });

    it('takes a creature off the map by its creature id, and offers no hiding, which only a character has', async () => {
      const view = await withCreature();
      view.tokenSelect.emit('raven');
      await settle();
      expect(el.querySelector('#tk-title')?.textContent).toContain('Corvo');
      expect(text()).not.toContain('Esconder');
      removeButton().click();
      await settle();
      expect(api.calls).toContain('removeToken map-1 creature:raven');
      expect(state.tokens().map((t) => t.name)).toEqual(['Pensantus']);
    });
  });

  describe('modes', () => {
    it('opens on "Pontos": the list of points, with the three new kinds on the bar', async () => {
      await setup();
      expect(radio('Pontos').getAttribute('aria-checked')).toBe('true');
      expect(text()).toContain('Pontos do mapa');
      expect(text()).toContain('Fosso escondido');
      expect(text()).toContain('Baú de moedas');
      expect(text()).toContain('Tocha da guarita');
      expect(surface()).toBeUndefined();
    });

    it('"Pintar" shows the tools, "Camadas", "Grade" and "Névoa de guerra", and the surface that catches the brush', async () => {
      await setup();
      radio('Pintar').click();
      await settle();
      expect(button('Terreno difícil')).toBeTruthy();
      expect(text()).toContain('Camadas');
      expect(text()).toContain('Tudo salvo');
      expect(text()).toContain('Grade');
      expect(text()).toContain('Névoa de guerra');
      expect(surface()).toBeDefined();
      expect(text()).not.toContain('Pontos do mapa');
    });
  });

  describe('unsaved changes', () => {
    async function dirtyTrap(): Promise<void> {
      await setup();
      Array.from(el.querySelectorAll<HTMLElement>('button.pl__row'))
        .find((r) => r.textContent?.includes('Fosso escondido'))!
        .click();
      await settle();
      const field = Array.from(el.querySelectorAll('mat-form-field'))
        .find(
          (f) =>
            f.querySelector('mat-label')?.textContent?.trim() === 'CD para achar (Investigação)',
        )!
        .querySelector('input')!;
      field.value = '12';
      field.dispatchEvent(new Event('input'));
      await settle();
    }

    it('asks in place before going to "Pintar", and "Continuar editando" stays', async () => {
      await dirtyTrap();
      radio('Pintar').click();
      await settle();
      expect(text()).toContain('Salvar as mudanças em Fosso escondido?');
      expect(radio('Pontos').getAttribute('aria-checked')).toBe('true');
      button('Continuar editando').click();
      await settle();
      expect(text()).not.toContain('Salvar as mudanças em');
      expect(radio('Pontos').getAttribute('aria-checked')).toBe('true');
    });

    it('"Descartar mudanças" goes on to "Pintar"; "Salvar e continuar" saves first', async () => {
      await dirtyTrap();
      radio('Pintar').click();
      await settle();
      button('Descartar mudanças').click();
      await settle();
      expect(radio('Pintar').getAttribute('aria-checked')).toBe('true');
      expect(api.calls.some((c) => c.startsWith('updatePoint'))).toBe(false);
      radio('Pontos').click();
      await settle();
      Array.from(el.querySelectorAll<HTMLElement>('button.pl__row'))
        .find((r) => r.textContent?.includes('Fosso escondido'))!
        .click();
      await settle();
      const field = Array.from(el.querySelectorAll('mat-form-field'))
        .find(
          (f) =>
            f.querySelector('mat-label')?.textContent?.trim() === 'CD para achar (Investigação)',
        )!
        .querySelector('input')!;
      field.value = '12';
      field.dispatchEvent(new Event('input'));
      await settle();
      radio('Pintar').click();
      await settle();
      button('Salvar e continuar').click();
      await settle();
      await settle();
      expect(api.calls.some((c) => c.startsWith('updatePoint'))).toBe(true);
      expect(radio('Pintar').getAttribute('aria-checked')).toBe('true');
    });
  });

  describe('painting', () => {
    it('reads the painted layers once, and paints a stroke at once with "Salvando" until it reaches the server', async () => {
      await setup();
      expect(api.calls.filter((c) => c.startsWith('layers'))).toHaveLength(1);
      radio('Pintar').click();
      await settle();
      button('Parede').click();
      await settle();
      surface()!.stroke.emit({ centers: [{ col: 3, row: 4 }], erase: false });
      await frame();
      await settle();
      // On the screen before the server answers.
      expect(text()).toContain('1 quadrado · bloqueia movimento, visão e luz');
      expect(text()).toContain('Salvando');
      await flush();
      expect(api.paints).toEqual([
        { layer: MapLayer.WALL, value: 1, squares: [{ col: 3, row: 4 }] },
      ]);
      expect(text()).toContain('Tudo salvo');
    });

    it('a drag is one call with every square; the 3 × 3 brush paints nine', async () => {
      await setup();
      radio('Pintar').click();
      await settle();
      button('Terreno difícil').click();
      radio('3×3').click();
      await settle();
      surface()!.stroke.emit({ centers: [{ col: 5, row: 5 }], erase: false });
      surface()!.stroke.emit({ centers: [{ col: 6, row: 5 }], erase: false });
      surface()!.strokeEnd.emit();
      await flush();
      expect(api.paints).toHaveLength(1);
      expect(api.paints[0].layer).toBe(MapLayer.DIFFICULT_TERRAIN);
      expect(api.paints[0].squares).toHaveLength(12);
    });

    it('"Apagar" and Shift erase the chosen tool\'s layer with value 0', async () => {
      await setup();
      radio('Pintar').click();
      await settle();
      button('Parede').click();
      surface()!.stroke.emit({ centers: [{ col: 1, row: 1 }], erase: false });
      surface()!.stroke.emit({ centers: [{ col: 1, row: 1 }], erase: true });
      await flush();
      expect(api.paints.map((p) => `${p.layer}:${p.value}`)).toEqual([
        `${MapLayer.WALL}:1`,
        `${MapLayer.WALL}:0`,
      ]);
      expect(text()).toContain('nada pintado');
    });

    it('cover paints the chosen degree; the light, the chosen level', async () => {
      await setup();
      radio('Pintar').click();
      await settle();
      button('Cobertura').click();
      await settle();
      expect(text()).toContain('Graus de cobertura');
      expect(text()).toContain(
        '+2 na CA e nos testes de resistência de Destreza. Dá para passar por cima.',
      );
      radio('Três quartos').click();
      surface()!.stroke.emit({ centers: [{ col: 2, row: 2 }], erase: false });
      button('Luz').click();
      await settle();
      radio('Claro').click();
      surface()!.stroke.emit({ centers: [{ col: 3, row: 2 }], erase: false });
      await flush();
      expect(api.paints.map((p) => `${p.layer}:${p.value}`)).toEqual([
        `${MapLayer.COVER}:2`,
        `${MapLayer.LIGHT}:3`,
      ]);
    });

    it('says "Não salvou" when the server refuses, keeps what was painted and tries again', async () => {
      await setup();
      radio('Pintar').click();
      await settle();
      button('Parede').click();
      api.failWith = new Error('offline');
      surface()!.stroke.emit({ centers: [{ col: 0, row: 0 }], erase: false });
      await flush();
      expect(text()).toContain('Não salvou');
      expect(text()).toContain('1 quadrado · bloqueia');
      api.failWith = null;
      button('Tentar de novo').click();
      await flush();
      expect(text()).toContain('Tudo salvo');
      expect(api.paints).toHaveLength(1);
    });

    it('hides a layer on his own map with its switch, and sends nothing', async () => {
      await setup();
      radio('Pintar').click();
      await settle();
      button('Parede').click();
      surface()!.stroke.emit({ centers: [{ col: 0, row: 0 }], erase: false });
      await frame();
      await settle();
      expect(el.querySelectorAll('app-editor-overlay .sq--wall')).toHaveLength(1);
      const wall = Array.from(
        el.querySelectorAll<HTMLElement>('app-layers-panel [role="switch"]'),
      )[1];
      wall.click();
      await settle();
      expect(el.querySelectorAll('app-editor-overlay .sq--wall')).toHaveLength(0);
    });

    it('leaving "Pintar" sends what waits', async () => {
      await setup();
      radio('Pintar').click();
      await settle();
      button('Parede').click();
      surface()!.stroke.emit({ centers: [{ col: 0, row: 0 }], erase: false });
      radio('Pontos').click();
      await settle();
      await Promise.resolve();
      expect(api.paints).toHaveLength(1);
    });

    it('without a grid there is nothing to paint: no surface, the tools cannot act, and the reason is said', async () => {
      await setup({ gridColumns: 0, gridRows: 0 });
      radio('Pintar').click();
      await settle();
      expect(surface()).toBeUndefined();
      expect(text()).toContain('Defina a grade para pintar e ligar a névoa.');
      expect(el.querySelector('#bar-why')).not.toBeNull();
      expect(button('Parede').getAttribute('aria-disabled')).toBe('true');
      expect(text()).toContain('Este mapa ainda não tem grade.');
      expect(text()).toContain('Precisa da grade definida.');
    });

    it('while a combat runs says so, still paints, and turns the grid change off', async () => {
      await setup(undefined, { combatRunning: true });
      expect(text()).toContain(
        'Um combate está em andamento neste mapa. Dá para pintar e apagar; a grade e a imagem só mudam depois dele.',
      );
      radio('Pintar').click();
      await settle();
      expect(surface()).toBeDefined();
      expect(button('Mudar a grade').getAttribute('aria-disabled')).toBe('true');
      expect(text()).toContain('Há um combate neste mapa. Termine-o para mudar a grade.');
    });

    it('tells the page when a new grid or image would erase something', async () => {
      await setup({ gridColumns: 24, gridRows: 16, fogEnabled: true });
      const erases: boolean[] = [];
      fixture.componentInstance.erasesChange.subscribe((e) => erases.push(e));
      fixture.detectChanges();
      await settle();
      // The fog is on: the players may have seen something.
      expect(fixture.componentInstance['paint'].erases()).toBe(true);
    });
  });

  describe('the door tool', () => {
    /** A 24 x 16 map with one wall square at (9, 6), under Pensantus's token (40 % of each side), and one at (15, 2) with a closed door on it. */
    const wallAt = (...squares: [number, number][]) => {
      const bytes = new Uint8Array(48);
      for (const [col, row] of squares) {
        const n = row * 24 + col;
        bytes[n >> 3] |= 1 << (n & 7);
      }
      return bytes;
    };
    const tap = async (col: number, row: number) => {
      surface()!.stroke.emit({ centers: [{ col, row }], erase: false });
      await frame();
      await settle();
    };
    async function paintMode(
      layers: { wall?: Uint8Array; doors?: Uint8Array } = { wall: wallAt([9, 6], [15, 2]) },
    ) {
      await setup(undefined, undefined, layers);
      radio('Pintar').click();
      await settle();
      button('Porta').click();
      await settle();
    }

    it('is the fifth tool: a second line picks the kind, with "Tirar a porta", and the side panel says what a tap does', async () => {
      await paintMode();
      expect(button('Porta').getAttribute('aria-pressed')).toBe('true');
      expect(text()).toContain('Tipo de porta');
      for (const name of ['Fechada', 'Aberta', 'Trancada', 'Grade', 'Secreta', 'Tirar a porta']) {
        expect(button(name), name).toBeTruthy();
      }
      expect(button('Fechada').getAttribute('aria-pressed')).toBe('true');
      expect(text()).toContain('Toque num quadrado para pôr a porta do tipo escolhido.');
      expect(text()).toContain('Portas');
    });

    it('a tap on a wall with floor on both sides opens the gap and paints the chosen door: the wall goes first, then the door', async () => {
      await paintMode();
      button('Trancada').click();
      await settle();
      await tap(15, 2);
      await flush();
      expect(api.paints).toEqual([
        // The safe order (RN-10): the door first, then the wall goes, so no player sees a gap in between.
        { layer: MapLayer.DOORS, value: 3, squares: [{ col: 15, row: 2 }] },
        { layer: MapLayer.WALL, value: 0, squares: [{ col: 15, row: 2 }] },
      ]);
      expect(text()).toContain('1 porta · 1 trancada');
      // The master sees the padlock on the map and in the legend; nobody else gets a locked door.
      expect(el.querySelectorAll('.sq--door').length).toBe(1);
      expect(text()).toContain('Porta trancada (só você vê)');
    });

    it('a tap where no door fits says why, in place, and paints nothing', async () => {
      await paintMode();
      await tap(3, 3);
      await flush();
      expect(api.paints).toEqual([]);
      const alert = el.querySelector('[role="alert"]');
      expect(alert?.textContent).toContain(
        'Aqui não dá: uma porta precisa de chão dos dois lados.',
      );
    });

    it('"Tirar a porta" closes the gap again as a wall', async () => {
      await paintMode();
      await tap(15, 2);
      await flush();
      api.paints.length = 0;
      button('Tirar a porta').click();
      await settle();
      await tap(15, 2);
      await flush();
      expect(api.paints).toEqual([
        // Taking the door away: the wall comes back first.
        { layer: MapLayer.WALL, value: 1, squares: [{ col: 15, row: 2 }] },
        { layer: MapLayer.DOORS, value: 0, squares: [{ col: 15, row: 2 }] },
      ]);
    });

    it('asks first, in place, before a closed door goes where someone stands, and paints it only on "Pôr a porta"', async () => {
      await paintMode();
      await tap(9, 6);
      await flush();
      expect(api.paints).toEqual([]);
      expect(text()).toContain('Pôr a porta onde há alguém?');
      expect(text()).toContain('Pensantus está nesse quadrado');
      button('Voltar').click();
      await settle();
      expect(text()).not.toContain('Pôr a porta onde há alguém?');
      expect(api.paints).toEqual([]);
      await tap(9, 6);
      button('Pôr a porta').click();
      await flush();
      expect(api.paints.map((p) => `${p.layer}:${p.value}`)).toEqual([
        `${MapLayer.DOORS}:2`,
        `${MapLayer.WALL}:0`,
      ]);
    });

    it('"Tirar a porta" over someone asks too: the wall that comes back would hide them', async () => {
      const doors = new Uint8Array(192);
      // A closed door at (9, 6), under Pensantus's token: square 153 is the high nibble of byte 76.
      doors[76] = 0x20;
      await paintMode({ wall: wallAt([8, 6], [10, 6]), doors });
      button('Tirar a porta').click();
      await settle();
      await tap(9, 6);
      await flush();
      expect(api.paints).toEqual([]);
      expect(text()).toContain('Tirar a porta onde há alguém?');
      expect(text()).toContain('Com a parede de volta');
      button('Tirar a porta', true).click();
      await flush();
      expect(api.paints.map((p) => `${p.layer}:${p.value}`)).toEqual([
        `${MapLayer.WALL}:1`,
        `${MapLayer.DOORS}:0`,
      ]);
    });

    it('the door tool\'s row has no "Apagar" and no brush (one square at a time), and the cursor says "Aqui não dá" where a tap would be refused', async () => {
      await paintMode();
      expect(el.querySelector('#bar-brush')).toBeNull();
      expect(
        Array.from(el.querySelectorAll('[aria-label="Ferramenta de pintura"] button')).some((b) =>
          b.textContent?.trim().endsWith('Apagar'),
        ),
      ).toBe(false);
      surface()!.hover.emit({ col: 3, row: 3 });
      await settle();
      expect(el.querySelector('.cursor__tag')?.textContent?.trim()).toBe('Aqui não dá');
      expect(el.querySelector('.cursor .cursor__door')).toBeNull();
      surface()!.hover.emit({ col: 15, row: 2 });
      await settle();
      expect(el.querySelector('.cursor__tag')?.textContent?.trim()).toBe('Fechada');
      expect(el.querySelector('.cursor .cursor__door')).not.toBeNull();
    });

    it('does not ask for an open door or a grade: they do not hide anyone', async () => {
      await paintMode();
      button('Aberta').click();
      await settle();
      await tap(9, 6);
      await flush();
      expect(text()).not.toContain('Pôr a porta onde há alguém?');
      expect(api.paints.map((p) => `${p.layer}:${p.value}`)).toEqual([
        `${MapLayer.DOORS}:1`,
        `${MapLayer.WALL}:0`,
      ]);
    });

    it("a secret door is drawn with the wall's hatch for the master and named in the legend with the crossed eye", async () => {
      await paintMode();
      button('Secreta').click();
      await settle();
      await tap(15, 2);
      await flush();
      expect(text()).toContain('Porta secreta (só você vê)');
      expect(text()).toContain('1 porta · 1 secreta');
    });
  });

  describe('the door tool on a calibrated map', () => {
    // 8 x 8 rules' grid = 4 x 4 drawing squares x factor 2. A one-square wall along column 4, floor on both sides.
    const thinWall = () => {
      const bytes = new Uint8Array(8);
      for (let row = 0; row < 8; row++) {
        const n = row * 8 + 4;
        bytes[n >> 3] |= 1 << (n & 7);
      }
      return bytes;
    };
    const tapAt = async (col: number, row: number) => {
      surface()!.stroke.emit({ centers: [{ col, row }], erase: false });
      await frame();
      await settle();
    };
    async function paintCalibrated(doors?: Uint8Array) {
      await setup(
        {
          gridColumns: 8,
          gridRows: 8,
          drawnColumns: 4,
          drawnRows: 4,
          squareFactor: 2,
          fogEnabled: false,
        },
        undefined,
        { wall: thinWall(), doors },
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

    it('on a calibrated map a door tap paints and sends the whole block of the drawing square, as the server does', async () => {
      await paintCalibrated();
      await tapAt(4, 2);
      await flush();
      expect(doorSquares(api.paints)).toEqual(['4,2', '4,3', '5,2', '5,3']);
      expect(el.querySelectorAll('.sq--door').length).toBe(4);
    });

    it('on a calibrated map "Tirar a porta" on one square of a door block takes the whole block', async () => {
      const doors = new Uint8Array(32);
      for (const [c, r] of [
        [4, 2],
        [5, 2],
        [4, 3],
        [5, 3],
      ]) {
        const n = r * 8 + c;
        doors[n >> 1] |= 2 << (4 * (n & 1));
      }
      await paintCalibrated(doors);
      button('Tirar a porta').click();
      await settle();
      await tapAt(5, 3);
      await flush();
      expect(doorSquares(api.paints.filter((p) => p.value === 0))).toEqual([
        '4,2',
        '4,3',
        '5,2',
        '5,3',
      ]);
      expect(el.querySelectorAll('.sq--door').length).toBe(0);
    });
  });

  describe('leaving with strokes', () => {
    async function strokeWaiting(): Promise<void> {
      await setup();
      radio('Pintar').click();
      await settle();
      button('Parede').click();
      surface()!.stroke.emit({ centers: [{ col: 3, row: 4 }], erase: false });
      await frame();
    }

    it('lets him go at once when nothing waits', async () => {
      await setup();
      expect(await fixture.componentInstance.confirmLeave()).toBe(true);
    });

    it('sends what waits first, and goes when the server took it', async () => {
      await strokeWaiting();
      expect(await fixture.componentInstance.confirmLeave()).toBe(true);
      expect(api.paints).toHaveLength(1);
    });

    it('asks in place when the server does not take the strokes, and "Voltar" stays', async () => {
      await strokeWaiting();
      api.failWith = new ConnectError('down', Code.Unavailable);
      const answer = fixture.componentInstance.confirmLeave();
      await flush();
      expect(text()).toContain('Há traços que o servidor não recebeu');
      expect(text()).toContain('Sair e perder os traços');
      button('Voltar').click();
      expect(await answer).toBe(false);
      await settle();
      expect(text()).not.toContain('Há traços que o servidor não recebeu');
    });

    it('"Sair e perder os traços" leaves', async () => {
      await strokeWaiting();
      api.failWith = new ConnectError('down', Code.Unavailable);
      const answer = fixture.componentInstance.confirmLeave();
      await flush();
      button('Sair e perder os traços').click();
      expect(await answer).toBe(true);
    });

    it('the browser asks too while strokes wait, and not when none do', async () => {
      await strokeWaiting();
      const waiting = new Event('beforeunload', { cancelable: true });
      window.dispatchEvent(waiting);
      expect(waiting.defaultPrevented).toBe(true);
      await flush();
      const quiet = new Event('beforeunload', { cancelable: true });
      window.dispatchEvent(quiet);
      expect(quiet.defaultPrevented).toBe(false);
    });
  });

  describe('a grid change under strokes that wait', () => {
    it('drops them: they were made on squares the map no longer has', async () => {
      await setup();
      radio('Pintar').click();
      await settle();
      button('Parede').click();
      api.failWith = new ConnectError('down', Code.Unavailable);
      surface()!.stroke.emit({ centers: [{ col: 20, row: 14 }], erase: false });
      await frame();
      await flush();
      expect(text()).toContain('Não salvou');
      api.failWith = null;
      state.setMap({ ...state.map()!, gridColumns: 12, gridRows: 8, layersRevision: 2 });
      await flush();
      expect(text()).toContain('Tudo salvo');
      expect(api.paints).toEqual([]);
    });
  });

  describe('the screen in each mode', () => {
    it('"Pintar" shows a clean map: no labels, and the markers stand back', async () => {
      await setup();
      expect(el.querySelectorAll('.lbl').length).toBeGreaterThan(0);
      radio('Pintar').click();
      await settle();
      expect(el.querySelectorAll('.lbl')).toHaveLength(0);
      expect(el.querySelector('app-map-view .mv--faded')).not.toBeNull();
    });

    it('the points of one square share one label', async () => {
      await setup();
      // The fixtures stand on different squares: each point has its own label here.
      const labels = Array.from(el.querySelectorAll('.lbl'), (l) =>
        l.textContent?.replace(/\s+/g, ' ').trim(),
      );
      expect(labels.length).toBe(4);
      expect(labels.some((l) => l?.includes('Fosso escondido'))).toBe(true);
    });

    it("the legend names only what the map has, with the toolbar's light names", async () => {
      await setup();
      const legend = el.querySelector('app-map-legend')?.textContent ?? '';
      expect(legend).toContain('Cena de RP');
      expect(legend).not.toContain('Batalha');
      expect(legend).not.toContain('Submapa');
      radio('Pintar').click();
      await settle();
      button('Luz').click();
      await settle();
      radio('Penumbra').click();
      surface()!.stroke.emit({ centers: [{ col: 3, row: 2 }], erase: false });
      await frame();
      await settle();
      expect(el.querySelector('app-map-layers-legend')?.textContent).toContain('Penumbra');
      expect(text()).not.toContain('Luz em penumbra');
      expect(text()).toContain('Tokens');
    });

    it('"Graus de cobertura" is in the side column while Cobertura is chosen, and not under the map', async () => {
      await setup();
      radio('Pintar').click();
      await settle();
      button('Cobertura').click();
      await settle();
      expect(el.querySelector('aside app-cover-degrees')).not.toBeNull();
      expect(el.querySelector('.layout__map app-cover-degrees')).toBeNull();
    });

    it('without a grid the tools stay off with the reason beside them: none chosen, no brush, no "Tudo salvo"', async () => {
      await setup({ gridColumns: 0, gridRows: 0 });
      radio('Pintar').click();
      await settle();
      expect(el.querySelector('app-editor-bar #bar-why')).not.toBeNull();
      expect(el.querySelectorAll('app-editor-bar [aria-pressed="true"]')).toHaveLength(0);
      expect(radio('1×1').hasAttribute('disabled')).toBe(true);
      expect(text()).not.toContain('Tudo salvo');
    });

    it('"Ver como" comes first in the side column', async () => {
      await setup({ gridColumns: 24, gridRows: 16, fogEnabled: true, baseLight: LightLevel.DARK });
      api.visions.set(
        'c-toren',
        visionResponse(['B'.repeat(24), ...Array.from({ length: 15 }, () => '.'.repeat(24))]),
      );
      api.visions.set(
        'c-pensantus',
        visionResponse(['B'.repeat(24), ...Array.from({ length: 15 }, () => '.'.repeat(24))]),
      );
      await settle();
      const viewAs = el.querySelector('aside app-view-as-list')!;
      const list = el.querySelector('aside app-point-list')!;
      expect(viewAs.compareDocumentPosition(list) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    });

    it('one question pattern: the unsaved point asks with the title focused and the filled button last', async () => {
      await setup();
      Array.from(el.querySelectorAll<HTMLElement>('button.pl__row'))
        .find((r) => r.textContent?.includes('Fosso escondido'))!
        .click();
      await settle();
      const field = Array.from(el.querySelectorAll('mat-form-field'))
        .find(
          (f) =>
            f.querySelector('mat-label')?.textContent?.trim() === 'CD para achar (Investigação)',
        )!
        .querySelector('input')!;
      field.value = '12';
      field.dispatchEvent(new Event('input'));
      await settle();
      radio('Pintar').click();
      await settle();
      const ask = el.querySelector('app-map-ask')!;
      expect(ask.querySelector('h3')?.textContent).toContain(
        'Salvar as mudanças em Fosso escondido?',
      );
      expect(document.activeElement).toBe(ask.querySelector('h3'));
      const buttons = Array.from(ask.querySelectorAll('button'), (b) => b.textContent?.trim());
      expect(buttons[0]).toBe('Continuar editando');
      expect(buttons[1]).toBe('Salvar e continuar');
    });
  });

  describe('the new points', () => {
    function pick(name: string): void {
      const row = Array.from(el.querySelectorAll<HTMLElement>('button.pl__row')).find((r) =>
        r.textContent?.includes(name),
      )!;
      row.click();
    }

    it('opens the panel of each kind', async () => {
      await setup();
      pick('Fosso escondido');
      await settle();
      expect(el.querySelector('app-trap-point-panel')).not.toBeNull();
      expect(text()).toContain('Predefinições do SRD');
      fixture.componentInstance['requestSelect'](null);
      await settle();
      pick('Baú de moedas');
      await settle();
      expect(el.querySelector('app-treasure-point-panel')).not.toBeNull();
      fixture.componentInstance['requestSelect'](null);
      await settle();
      pick('Tocha da guarita');
      await settle();
      expect(el.querySelector('app-light-point-panel')).not.toBeNull();
      expect(text()).toContain('Tipo de luz');
      fixture.componentInstance['requestSelect'](null);
      await settle();
      pick('Taverna');
      await settle();
      expect(el.querySelector('app-point-panel')).not.toBeNull();
    });

    it('draws the radii of the selected light as two rings and names them in the legend', async () => {
      await setup();
      pick('Tocha da guarita');
      await settle();
      expect(el.querySelector('app-editor-overlay .reach__bright')).not.toBeNull();
      expect(el.querySelector('app-editor-overlay .reach__dim')).not.toBeNull();
      expect(text()).toContain('Alcance da luz clara');
      expect(text()).toContain('Alcance da penumbra');
    });

    it('creates a trap with a spec the server takes, a light from the torch preset and a treasure with no value', async () => {
      await setup();
      const view = fixture.debugElement.query(By.directive(MapView)).componentInstance as MapView;
      for (const [label, kind] of [
        ['Armadilha', MapPointKind.TRAP],
        ['Luz', MapPointKind.LIGHT],
        ['Tesouro', MapPointKind.TREASURE],
      ] as const) {
        button(label).click();
        view.emptyClick.emit({ xBp: 1000, yBp: 2000 });
        await settle();
        const call = api.calls.filter((c) => c.startsWith('createPoint')).at(-1)!;
        expect(call).toContain(`"kind":${kind}`);
        fixture.componentInstance['requestSelect'](null);
        fixture.componentInstance['dirty'].set(false);
        await settle();
      }
      expect(api.calls.filter((c) => c.startsWith('createPoint'))).toHaveLength(3);
    });

    it('a trap and a chest on one square stay two rows of the list', async () => {
      await setup();
      const names = Array.from(el.querySelectorAll('.pl__name'), (n) => n.textContent);
      expect(names).toContain('Fosso escondido');
      expect(names).toContain('Baú de moedas');
    });
  });

  describe('saving a point that is being moved', () => {
    it('keeps the dragged position when the answer to "Salvar ponto" was computed before the move committed', async () => {
      await setup();
      const updates: { resolve: (p: MapPoint) => void }[] = [];
      api.updatePoint = () => new Promise((resolve) => updates.push({ resolve }));
      Array.from(el.querySelectorAll<HTMLElement>('button.pl__row'))
        .find((r) => r.textContent?.includes('Fosso escondido'))!
        .click();
      await settle();
      const field = Array.from(el.querySelectorAll('mat-form-field'))
        .find(
          (f) =>
            f.querySelector('mat-label')?.textContent?.trim() === 'CD para achar (Investigação)',
        )!
        .querySelector('input')!;
      field.value = '12';
      field.dispatchEvent(new Event('input'));
      await settle();
      const comp = fixture.componentInstance as unknown as {
        onMoved(m: unknown): Promise<void>;
        save(): Promise<boolean>;
      };

      // The master drags the trap; the move request stays in flight.
      const moving = comp.onMoved({ kind: 'point', id: 'pit', xBp: 1000, yBp: 2000 });
      await settle();
      expect(state.points().find((p) => p.id === 'pit')!.xBp).toBe(1000);
      // Still in flight, "Salvar ponto" for the panel change; the server answers it with the old place first.
      const saving = comp.save();
      await settle();
      expect(updates).toHaveLength(2);
      updates[1].resolve(
        mapPoint('pit', 'Fosso escondido', { xBp: 4800, yBp: 5000, kind: MapPointKind.TRAP }),
      );
      await saving;
      updates[0].resolve(
        mapPoint('pit', 'Fosso escondido', { xBp: 1000, yBp: 2000, kind: MapPointKind.TRAP }),
      );
      await moving;
      await settle();

      const shown = state.points().find((p) => p.id === 'pit')!;
      expect([shown.xBp, shown.yBp]).toEqual([1000, 2000]);
    });
  });

  describe('"Ver como"', () => {
    it('is offered with the fog on, and shows that player\'s map with "Voltar à sua vista"', async () => {
      await setup({ gridColumns: 24, gridRows: 16, fogEnabled: true, baseLight: LightLevel.DARK });
      api.visions.set(
        'c-toren',
        visionResponse(['B'.repeat(24), ...Array.from({ length: 15 }, () => '.'.repeat(24))]),
      );
      api.visions.set(
        'c-pensantus',
        visionResponse(['B'.repeat(24), ...Array.from({ length: 15 }, () => '.'.repeat(24))]),
      );
      api.responses.set(
        'map-1@c-toren',
        mapResponse(
          mapMessage('map-1', 'A caverna do Vale Seco', {
            gridColumns: 24,
            gridRows: 16,
            fogEnabled: true,
          }),
          [],
          [mapToken('c-toren', 'Toren')],
        ),
      );
      await settle();
      expect(text()).toContain('Ver como');
      const row = Array.from(
        el.querySelectorAll<HTMLElement>('app-view-as-list [role="radio"]'),
      ).find((r) => r.textContent?.includes('Toren'))!;
      row.click();
      await settle();
      expect(text()).toContain('Você está vendo o mapa como Toren');
      expect(text()).toContain('Para voltar à sua vista, escolha “Todos”.');
      expect(button('Voltar à sua vista')).toBeTruthy();
      expect(el.querySelector('app-editor-bar')).toBeNull();
      button('Voltar à sua vista').click();
      await settle();
      expect(el.querySelector('app-editor-bar')).not.toBeNull();
      expect(text()).not.toContain('Você está vendo o mapa como');
    });

    it('is not offered without the fog', async () => {
      await setup();
      expect(text()).not.toContain('Ver como');
    });
  });
});
