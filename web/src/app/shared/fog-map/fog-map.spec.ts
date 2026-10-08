import { TestBed } from '@angular/core/testing';
import { textOf } from '../../core/format/text-testing';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { CharacterKind } from '../../../gen/meurpg/characters/v1/characters_pb';
import { decodeVision, type Vision } from '../../core/maps/vision';
import { visionResponse } from '../../core/maps/vision-testing';
import { mapToken } from '../../core/maps/maps-testing';
import type { MapLayers } from '../../core/maps/layers';
import { create as protoCreate } from '@bufbuild/protobuf';
import { MapPointKind, MapPointSchema, TrapState } from '../../../gen/meurpg/maps/v1/maps_pb';
import { FogMap } from './fog-map';

const plain = textOf;

const pensantus = mapToken('p', 'Pensantus', { mine: true, xBp: 2000, yBp: 2000 });
const toren = mapToken('t', 'Toren', { xBp: 3000, yBp: 2000 });
const goblin = mapToken('g', 'Goblin 2', { kind: CharacterKind.MINION, xBp: 8000, yBp: 2000 });
const lurker = mapToken('l', 'Goblin 3', { kind: CharacterKind.MINION, xBp: 3000, yBp: 3000 });
const nanquim = mapToken('p', 'Nanquim', {
  creatureId: 'c1',
  kind: CharacterKind.UNSPECIFIED,
  xBp: 2500,
  yBp: 2500,
});

const layers: MapLayers = {
  columns: 4,
  rows: 4,
  walls: [{ col: 0, row: 0 }],
  terrain: [],
  half: [{ col: 1, row: 1 }],
  threeQuarters: [],
};

function tiled(partial: Parameters<typeof visionResponse>[1] = {}): Vision {
  return decodeVision(
    visionResponse(['....', '.gB.', '.dr.', '....'], {
      tilesPath: '/images/maps/m1/tiles/',
      tileSquares: 2,
      tiles: [
        { $typeName: 'meurpg.maps.v1.MapTile', tx: 0, ty: 0, revision: 3 },
        { $typeName: 'meurpg.maps.v1.MapTile', tx: 1, ty: 1, revision: 1 },
      ],
      ...partial,
    }),
  );
}

describe('FogMap', () => {
  beforeEach(() => TestBed.configureTestingModule({ imports: [FogMap] }));
  afterEach(() => vi.unstubAllGlobals());

  function create(inputs: Record<string, unknown> = {}) {
    const fixture = TestBed.createComponent(FogMap);
    fixture.componentRef.setInput('imageWidth', 960);
    fixture.componentRef.setInput('imageHeight', 640);
    fixture.componentRef.setInput('mapName', 'A caverna do Vale Seco');
    fixture.componentRef.setInput('vision', tiled());
    fixture.componentRef.setInput('layers', layers);
    fixture.componentRef.setInput('tokens', [pensantus, toren, goblin]);
    fixture.componentRef.setInput('viewer', { name: 'Pensantus', own: true });
    for (const [k, v] of Object.entries(inputs)) {
      fixture.componentRef.setInput(k, v);
    }
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  const settle = (fixture: ReturnType<typeof create>['fixture']) => {
    (fixture.nativeElement as HTMLElement)
      .querySelectorAll<HTMLImageElement>('.fb__tile')
      .forEach((t) => t.dispatchEvent(new Event('load')));
    fixture.detectChanges();
  };

  it('says "Carregando o mapa" once, as a status, with "parte N de M", until the tiles are in; then the notice goes', () => {
    const { fixture, el } = create();
    const notice = el.querySelector('[data-testid="fog-loading"]')!;
    expect(notice.getAttribute('role')).toBe('status');
    expect(plain(notice)).toContain('Carregando o mapa');
    expect(plain(notice)).toContain('Chegando a parte 1 de 2');
    el.querySelector<HTMLImageElement>('.fb__tile')!.dispatchEvent(new Event('load'));
    fixture.detectChanges();
    expect(plain(el.querySelector('[data-testid="fog-loading"]'))).toContain(
      'Chegando a parte 2 de 2',
    );
    settle(fixture);
    expect(el.querySelector('[data-testid="fog-loading"]')).toBeNull();
  });

  it('draws the party over a place whose tile is still on its way, and an NPC only once its tile has arrived', () => {
    const { fixture, el } = create({ tokens: [pensantus, toren, goblin, lurker] });
    const names = () =>
      Array.from(el.querySelectorAll('ul[aria-label="No mapa"] li'), (li) => plain(li));
    // Nothing has arrived: the party is on the map (it never vanishes); Goblin 3 waits for its tile; Goblin 2 is on a square with no tile.
    expect(names()).toEqual(['Goblin 2', 'Toren', 'Pensantus (você)']);
    settle(fixture);
    expect(names()).toEqual(['Goblin 2', 'Goblin 3', 'Toren', 'Pensantus (você)']);
  });

  it("draws the viewer's own token last, on top of its neighbours", () => {
    const { el } = create({ tokens: [pensantus, toren, goblin] });
    const names = Array.from(el.querySelectorAll('ul[aria-label="No mapa"] li'), (li) => plain(li));
    expect(names.at(-1)).toBe('Pensantus (você)');
  });

  it('says "Carregando o mapa" for the first load only: a tile that comes later brings back neither the notice nor the stripes', () => {
    const { fixture, el } = create();
    settle(fixture);
    expect(el.querySelector('[data-testid="fog-loading"]')).toBeNull();
    // The player sees more: a new tile appears.
    fixture.componentRef.setInput(
      'vision',
      tiled({
        revision: 2,
        tiles: [
          { $typeName: 'meurpg.maps.v1.MapTile', tx: 0, ty: 0, revision: 3 },
          { $typeName: 'meurpg.maps.v1.MapTile', tx: 1, ty: 1, revision: 1 },
          { $typeName: 'meurpg.maps.v1.MapTile', tx: 1, ty: 0, revision: 1 },
        ],
      }),
    );
    fixture.detectChanges();
    expect(el.querySelector('[data-testid="fog-loading"]')).toBeNull();
    expect(el.querySelectorAll('[data-pending]').length).toBe(0);
  });

  it('while the vision is read, the place is a still striped frame, with no map and no token yet', () => {
    const { el } = create({ vision: null, status: 'loading' });
    expect(el.querySelector('.fm__wait')).not.toBeNull();
    expect(el.querySelector('app-map-view')).toBeNull();
    expect(plain(el.querySelector('[data-testid="fog-loading"]'))).toContain('Carregando o mapa');
  });

  it('keeps the loading notice off when the screen has its own banner', () => {
    const { el } = create({ notices: false });
    expect(el.querySelector('[data-testid="fog-loading"]')).toBeNull();
  });

  it('says in words that the character is not on the map, and what is still seen', () => {
    const { el } = create({
      vision: tiled({ characterOnMap: false }),
      viewer: { name: 'Brisa', own: true },
    });
    const notice = el.querySelector('[data-testid="fog-off-map"]')!;
    expect(notice.getAttribute('role')).toBe('status');
    expect(plain(notice)).toBe(
      'location_off Brisa fora do mapa Seu personagem não está neste mapa. Você vê só o que já tinha visto.',
    );
  });

  it('says it of another character when the master reads as them', () => {
    const { el } = create({
      vision: tiled({ characterOnMap: false }),
      viewer: { name: 'Brisa', own: false },
    });
    expect(plain(el.querySelector('[data-testid="fog-off-map"]'))).toContain(
      'O personagem não está neste mapa: o jogador vê só o que já tinha visto.',
    );
  });

  it('names every state the map draws, in the order of MAP-LANGUAGE.md, then the layers and the tokens', () => {
    const { fixture, el } = create({ tokens: [pensantus, toren, goblin, nanquim] });
    settle(fixture);
    const items = Array.from(el.querySelectorAll('.mr-legend li'), (li) => plain(li));
    expect(items).toEqual([
      'Visto',
      'Penumbra',
      'No escuro, em cinza',
      'Já visto',
      'Não visto',
      'Parede',
      'Meia cobertura',
      'P Você',
      'T Companheiro',
      'G2 Inimigo à vista',
      'N Criatura',
    ]);
  });

  it('names the master\'s view by the character, never "Você": his chips and legend say "Toren"', () => {
    const asToren = mapToken('t', 'Toren', { mine: true, xBp: 3000, yBp: 2000 });
    const { fixture, el } = create({
      tokens: [asToren, pensantus === asToren ? toren : mapToken('p', 'Pensantus')],
      viewer: { name: 'Toren', own: false },
    });
    settle(fixture);
    expect(Array.from(el.querySelectorAll('.mr-legend li'), (li) => plain(li))).toContain(
      'T Toren',
    );
    expect(
      Array.from(el.querySelectorAll('ul[aria-label="No mapa"] li'), (li) => plain(li)),
    ).not.toContain('Toren (você)');
    expect(plain(el)).not.toContain('(você)');
  });

  it('has no "No escuro, em cinza" for a viewer who sees none (no darkvision): the legend follows the squares', () => {
    const { el } = create({ vision: decodeVision(visionResponse(['BB', 'dd', '..'])) });
    expect(Array.from(el.querySelectorAll('.mr-legend li'), (li) => plain(li))).not.toContain(
      'No escuro, em cinza',
    );
  });

  it('says the server is away with the app\'s "Reconectando…", and that a map has no grid', () => {
    const offline = create({ vision: null, status: 'error', error: 'offline' });
    expect(plain(offline.el.querySelector('[data-testid="fog-offline"]'))).toContain(
      'Reconectando…',
    );
    const grid = create({ vision: null, status: 'error', error: 'no-grid' });
    expect(plain(grid.el)).toContain('Este mapa não tem grade.');
  });

  it('shows a small box, not a black map, for a character off the map who never saw anything', () => {
    const { el } = create({ vision: tiled({ characterOnMap: false, ...{} }), tokens: [] });
    expect(el.querySelector('.fm__none')).toBeNull();
    const none = create({
      vision: decodeVision(visionResponse(['....', '....'], { characterOnMap: false })),
      tokens: [],
    });
    expect(none.el.querySelector('.fm__none')).not.toBeNull();
    expect(none.el.querySelector('app-map-view')).toBeNull();
    expect(plain(none.el.querySelector('[data-testid="fog-nothing"]'))).toContain(
      'Nada novo para ver',
    );
  });

  it('names only what is on the map: a map with nothing remembered has no "Já visto"', () => {
    const { el } = create({
      vision: decodeVision(visionResponse(['BB', 'dd'])),
      layers: null,
      tokens: [pensantus],
    });
    expect(Array.from(el.querySelectorAll('.mr-legend li'), (li) => plain(li))).toEqual([
      'Visto',
      'Penumbra',
      'P Você',
    ]);
  });

  it('writes what the viewer can use, not a count: senses, light, enemies in sight, what grey and remembered mean', () => {
    const { fixture, el } = create({ senses: ['Visão no escuro: 18 m'], carried: 'tocha' });
    settle(fixture);
    const card = plain(el.querySelector('[data-testid="fog-caption"]'));
    expect(card).toContain(
      'Você vê Visão no escuro: 18 m Luz que você carrega: tocha. Inimigos à vista: Goblin 2.',
    );
    expect(card).toContain('Em cinza: visto no escuro, pela visão no escuro.');
    expect(card).toContain('O que você já viu fica escurecido e sem inimigos');
    expect(card).not.toMatch(/\d+ de \d+ quadrados/);
  });

  it('is "O que o Nanquim vê" while the player looks through the familiar, with no senses of the character', () => {
    const { fixture, el } = create({ senses: ['Visão no escuro: 18 m'], familiar: 'Nanquim' });
    settle(fixture);
    const card = plain(el.querySelector('[data-testid="fog-caption"]'));
    expect(card).toContain('O que o Nanquim vê');
    expect(card).not.toContain('Visão no escuro: 18 m');
  });

  it('draws the party with its own marks: the viewer with the garnet ring, an NPC as a square, a creature with a dashed ring', () => {
    const { fixture, el } = create({ tokens: [pensantus, toren, goblin, nanquim] });
    settle(fixture);
    expect(el.querySelectorAll('app-map-token.tk--mine').length).toBe(1);
    expect(el.querySelectorAll('app-map-token.tk--npc').length).toBe(1);
    expect(el.querySelectorAll('app-map-token.tk--creature').length).toBe(1);
    expect(plain(el.querySelector('app-map-token.tk--npc'))).toBe('G2');
  });

  it('has the zoom buttons, 44 px, named, that start disabled at the whole map', () => {
    const { el } = create();
    const buttons = Array.from(el.querySelectorAll<HTMLButtonElement>('.fm__zoom button'));
    expect(buttons.map((b) => b.getAttribute('aria-label'))).toEqual([
      'Reduzir',
      'Ampliar',
      'Ajustar à tela',
    ]);
    expect(buttons.map((b) => b.disabled)).toEqual([true, false, true]);
  });

  it('on a phone, puts the party in chips above the map, and a tap takes the view to that character', () => {
    vi.stubGlobal('matchMedia', (query: string) => ({
      matches: true,
      media: query,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
    }));
    const { fixture, el } = create({ tokens: [pensantus, toren, goblin, nanquim] });
    settle(fixture);
    const chips = Array.from(el.querySelectorAll<HTMLButtonElement>('.fm__chip'));
    expect(chips.map((c) => plain(c))).toEqual(['P Pensantus (você)', 'T Toren']);
    expect(plain(el.querySelector('.fm__hint'))).toBe('Dois dedos para ampliar');
    const focus = vi.spyOn(fixture.componentInstance['view']()!, 'focusOn');
    chips[1].click();
    expect(focus).toHaveBeenCalledWith({ xBp: 3000, yBp: 2000 });
  });

  it('on a phone, puts the master\'s "Ver como" label above the map, in the flow; on a computer the band says it', () => {
    const computer = create({ badge: 'Vendo como Toren (Caio)' });
    expect(computer.el.querySelector('.fm__badge')).toBeNull();
    vi.stubGlobal('matchMedia', (query: string) => ({
      matches: true,
      media: query,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
    }));
    const phone = create({ badge: 'Vendo como Toren (Caio)' });
    expect(plain(phone.el.querySelector('.fm__badge'))).toBe('visibility Vendo como Toren (Caio)');
  });

  it('opens a phone zoomed in on the party (2x), once', () => {
    vi.stubGlobal('matchMedia', (query: string) => ({
      matches: true,
      media: query,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
    }));
    const { fixture } = create();
    expect(fixture.componentInstance['startAt']()).toEqual({ xBp: 2500, yBp: 2000 });
  });
  it('hosts the trap and chest marks over the map only when asked, drawing what the read sent (a remembered one darkened)', () => {
    const trap = protoCreate(MapPointSchema, {
      id: 'a',
      kind: MapPointKind.TRAP,
      xBp: 4000,
      yBp: 4000,
      trap: { state: TrapState.ARMED, areaSize: 1 },
      revealed: true,
    });
    const old = protoCreate(MapPointSchema, {
      id: 'b',
      kind: MapPointKind.TREASURE,
      xBp: 6000,
      yBp: 6000,
      revealed: true,
      remembered: true,
    });
    const without = create({ points: [trap, old] });
    expect(without.el.querySelector('app-map-pins')).toBeNull();
    const withPins = create({ points: [trap, old], pins: true });
    expect(withPins.el.querySelectorAll('app-map-pins .area')).toHaveLength(1);
    expect(withPins.el.querySelectorAll('app-map-pins .pin--remembered')).toHaveLength(1);
    const none = create({ points: [], pins: true });
    expect(none.el.querySelectorAll('app-map-pins .area, app-map-pins .pin')).toHaveLength(0);
  });
});
