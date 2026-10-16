import { TestBed } from '@angular/core/testing';
import { MatBottomSheetRef } from '@angular/material/bottom-sheet';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';
import { create } from '@bufbuild/protobuf';

import { XpMode } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { MapBlockedReason, MapBlockedSchema } from '../../../../gen/meurpg/maps/v1/maps_pb';
import {
  type Treasure,
  TreasureBlockedReason,
  TreasureBlockedSchema,
  TreasureInvalidFieldSchema,
} from '../../../../gen/meurpg/maps/v1/treasure_pb';
import { flat, isOff } from '../../../core/creatures/creatures-testing';
import { DungeonsClient } from '../../../core/maps/dungeons-client';
import { FakeDungeonsClient } from '../../../core/maps/dungeons-testing';
import { MapsClient } from '../../../core/maps/maps-client';
import { FakeMapsClient, mapMessage, mapResponse } from '../../../core/maps/maps-testing';
import { TreasureClient } from '../../../core/treasure/treasure-client';
import {
  FakeTreasureClient,
  sampleHoard,
  sampleIndividual,
} from '../../../core/treasure/treasure-testing';
import { PlaceSheet, type PlaceSheetData } from './place-sheet';
import { nextTurn } from '../../../../test-settle';

const plain = (s: string | undefined) => s?.replace(/\u00a0/g, ' ');

/** An `invalid_argument` the way the server sends it: it names the request field that broke the rule. */
function invalidField(field: string): ConnectError {
  return new ConnectError('x', Code.InvalidArgument, undefined, [
    { desc: TreasureInvalidFieldSchema, value: create(TreasureInvalidFieldSchema, { field }) },
  ]);
}

describe('PlaceSheet: "Pôr no mapa" (MR-044, RN-10, E10-10 states 4 and 6)', () => {
  let treasure: FakeTreasureClient;
  let maps: FakeMapsClient;
  let dungeons: FakeDungeonsClient;
  let close: ReturnType<typeof vi.fn>;

  async function setup(
    opts: { phone?: boolean; xpMode?: XpMode; prep?: () => void; treasure?: Treasure } = {},
  ) {
    treasure = new FakeTreasureClient();
    maps = new FakeMapsClient();
    dungeons = new FakeDungeonsClient();
    maps.maps = [
      mapMessage('map-1', 'Masmorra de Mirathel', {
        gridColumns: 11,
        gridRows: 9,
        current: true,
        generatedDungeon: true,
      }),
      mapMessage('map-2', 'A caverna', { gridColumns: 24, gridRows: 16 }),
      mapMessage('map-3', 'Mapa sem grade'),
    ];
    for (const m of maps.maps) {
      maps.responses.set(m.id, mapResponse(m));
    }
    opts.prep?.();
    close = vi.fn();
    const data: PlaceSheetData = {
      campaignId: 'camp-1',
      treasure: opts.treasure ?? sampleHoard(),
      xpMode: opts.xpMode ?? XpMode.ENEMIES,
      campaignName: 'Mirathel',
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: TreasureClient, useValue: treasure },
        { provide: MapsClient, useValue: maps },
        { provide: DungeonsClient, useValue: dungeons },
        { provide: MAT_DIALOG_DATA, useValue: data },
        opts.phone
          ? { provide: MatBottomSheetRef, useValue: { dismiss: close } }
          : { provide: MatDialogRef, useValue: { close } },
      ],
    });
    const fixture = TestBed.createComponent(PlaceSheet);
    const settle = async () => {
      for (let i = 0; i < 6; i++) {
        fixture.detectChanges();
        await fixture.whenStable();
        // The calls are chains of promises: let them run before the next look.
        await nextTurn();
      }
    };
    await settle();
    const el = fixture.nativeElement as HTMLElement;
    const button = (name: string) =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        flat(b)?.startsWith(name),
      )!;
    const pickMap = async (id: string) => {
      const select = el.querySelector<HTMLSelectElement>('select')!;
      select.selectedIndex = maps.maps.findIndex((m) => m.id === id);
      select.dispatchEvent(new Event('change'));
      await settle();
    };
    const arrow = async (key: string, shiftKey = false) => {
      el.querySelector('.pick')!.dispatchEvent(
        new KeyboardEvent('keydown', { key, shiftKey, bubbles: true, cancelable: true }),
      );
      await settle();
    };
    return { el, settle, button, pickMap, arrow };
  }

  it('opens on the map of the session, with the square in the middle of its first room, and says where in words', async () => {
    const { el } = await setup();
    expect(flat(el.querySelector('.frame__title'))).toBe('Pôr no mapa');
    expect(flat(el.querySelector('.frame__sub'))).toBe(
      'Cria um ponto de tesouro escondido no quadrado escolhido.',
    );
    expect(el.querySelector<HTMLSelectElement>('select')!.value).toBe('map-1');
    expect(flat(el.querySelector('.where'))).toBe('Onde Na Sala 1.');
    expect(el.querySelector('app-map-view')).not.toBeNull();
    expect(flat(el.querySelector('.what'))).toContain('Só você vê');
  });

  it('shows no coordinates on screen; a screen reader hears the chosen square', async () => {
    const { el } = await setup();
    expect(el.querySelector('.pick')?.getAttribute('aria-describedby')).toBe('place-map-help');
    expect(flat(el.querySelector('[role=status].mr-visually-hidden'))).toBe(
      'Quadrado escolhido: coluna 3, linha 3, na Sala 1.',
    );
  });

  it('the summary says the gold apart from the items, in words, and what the campaign does with it', async () => {
    const { el } = await setup();
    const what = plain(flat(el.querySelector('.what')))!;
    expect(what).toContain('517 PO em moedas, gemas e arte.');
    expect(what).toContain(
      'Na descrição: 1.200 PP, 340 PO, 2 × Ágata, Quartzo azul, Cálice de prata gravado, Poção de Cura, Capa Élfica, Varinha de Mísseis Mágicos, Anel de Proteção.',
    );
    expect(what).toContain('Mirathel dá XP por inimigos');
  });

  it('"Pôr no mapa" sends the treasure as generated (mode, level, seed, content version), the square and one key', async () => {
    const { button, arrow, settle } = await setup();
    await arrow('ArrowRight');
    button('Pôr no mapa').click();
    await settle();
    expect(treasure.placed).toHaveLength(1);
    expect(treasure.placed[0]).toMatchObject({
      campaignId: 'camp-1',
      mapId: 'map-1',
      mode: 2,
      partyLevel: 4,
      seed: 2209n,
      contentVersion: 'srd51@a8abc93b235c+fx.17',
      column: 3,
      row: 2,
      name: '',
    });
    expect(treasure.placed[0]!.idempotencyKey).toMatch(/^[0-9a-f-]{36}$/);
    expect(close).toHaveBeenCalledWith(
      expect.objectContaining({
        kind: 'placed',
        mapId: 'map-1',
        mapName: 'Masmorra de Mirathel',
        place: 'Sala 1',
        goldPo: 517,
        itemCount: 4,
      }),
    );
  });

  it('the arrow keys move the square inside the grid, Shift by five', async () => {
    const { button, arrow, settle } = await setup();
    await arrow('ArrowLeft', true);
    await arrow('ArrowUp', true);
    button('Pôr no mapa').click();
    await settle();
    expect(treasure.placed[0]).toMatchObject({ column: 0, row: 0 });
  });

  it('the name goes along when typed (trimmed), and the server names the point when it is empty', async () => {
    const { el, button, settle } = await setup();
    const input = el.querySelector<HTMLInputElement>('input[data-field=name]')!;
    input.value = '  Cofre da cripta ';
    input.dispatchEvent(new Event('input'));
    await settle();
    button('Pôr no mapa').click();
    await settle();
    expect(treasure.placed[0]!.name).toBe('Cofre da cripta');
    expect(el.querySelector('input[data-field=name]')?.getAttribute('maxlength')).toBe('80');
  });

  it('a retry of the same request sends the same key; a changed square or name makes another', async () => {
    const { button, arrow, settle } = await setup();
    treasure.failWith.set('place', new ConnectError('down', Code.Unavailable));
    button('Pôr no mapa').click();
    await settle();
    expect(close).not.toHaveBeenCalled();
    treasure.failWith.clear();
    button('Pôr no mapa').click();
    await settle();
    expect(treasure.placed).toHaveLength(2);
    expect(treasure.placed[1]!.idempotencyKey).toBe(treasure.placed[0]!.idempotencyKey);
    // Another request: another key.
    close.mockClear();
    treasure.failWith.set('place', new ConnectError('down', Code.Unavailable));
    await arrow('ArrowDown');
    button('Pôr no mapa').click();
    await settle();
    expect(treasure.placed[2]!.idempotencyKey).not.toBe(treasure.placed[1]!.idempotencyKey);
  });

  it('on a desktop the rooms are radios beside the map too: choosing one moves the square to its middle, and the map outlines the room', async () => {
    const { el, button, settle } = await setup();
    expect(Array.from(el.querySelectorAll('.rooms__grid .room')).map((r) => flat(r))).toEqual([
      'Sala 1',
      'Sala 2',
      'Sala 3',
    ]);
    el.querySelectorAll<HTMLInputElement>('.room input')[2]!.click();
    await settle();
    expect(flat(el.querySelector('.where'))).toBe('Onde Na Sala 3.');
    button('Pôr no mapa').click();
    await settle();
    expect(treasure.placed[0]).toMatchObject({ column: 5, row: 6 });
  });

  it('a square the server refuses reads the map again, so a changed grid is drawn, and says so', async () => {
    const { el, button, settle } = await setup();
    treasure.failWith.set('place', invalidField('column'));
    const before = maps.calls.filter((c) => c === 'get map-1').length;
    button('Pôr no mapa').click();
    await settle();
    expect(maps.calls.filter((c) => c === 'get map-1').length).toBe(before + 1);
    expect(el.querySelector('[role=alert]')?.textContent).toContain('fora da grade');
    expect(el.querySelector('[role=alert]')?.textContent).not.toContain('80 letras');
  });

  it('a refusal that is not about the square does not read the map again, and says what is wrong', async () => {
    const { el, button, settle } = await setup();
    treasure.failWith.set('place', invalidField('name'));
    const before = maps.calls.filter((c) => c === 'get map-1').length;
    button('Pôr no mapa').click();
    await settle();
    expect(maps.calls.filter((c) => c === 'get map-1').length).toBe(before);
    expect(el.querySelector('[role=alert]')?.textContent).toContain('1 a 80 caracteres');
    expect(el.querySelector('[role=alert]')?.textContent).not.toContain('fora da grade');
  });

  it('an individual treasure says its gold is coins only', async () => {
    const { el } = await setup({ treasure: sampleIndividual() });
    expect(flat(el.querySelector('.what__gold'))?.replace(/\u00a0/g, ' ')).toBe('33 PO em moedas.');
  });

  it('a double tap makes one request', async () => {
    const { button, settle } = await setup();
    const go = button('Pôr no mapa');
    go.click();
    go.click();
    await settle();
    expect(treasure.placed).toHaveLength(1);
  });

  it('a changed content version says "Gere de novo" and offers it; the button closes asking the page to generate again', async () => {
    const { el, button, settle } = await setup();
    treasure.failWith.set(
      'place',
      new ConnectError('x', Code.FailedPrecondition, undefined, [
        {
          desc: TreasureBlockedSchema,
          value: create(TreasureBlockedSchema, { reason: TreasureBlockedReason.CONTENT_CHANGED }),
        },
      ]),
    );
    button('Pôr no mapa').click();
    await settle();
    expect(el.querySelector('[role=alert]')?.textContent).toContain('Gere de novo');
    button('Gerar de novo').click();
    expect(close).toHaveBeenCalledWith({ kind: 'again' });
  });

  it('the 200-point cap and a square out of the grid have their words', async () => {
    const { el, button, settle } = await setup();
    treasure.failWith.set('place', new ConnectError('x', Code.ResourceExhausted));
    button('Pôr no mapa').click();
    await settle();
    expect(el.querySelector('[role=alert]')?.textContent).toContain('limite de 200 pontos');
    treasure.failWith.set('place', invalidField('column'));
    button('Pôr no mapa').click();
    await settle();
    expect(el.querySelector('[role=alert]')?.textContent).toContain('fora da grade');
  });

  it('the server\'s NO_GRID is said as "Escolha um mapa com grade"', async () => {
    const { el, button, settle } = await setup();
    treasure.failWith.set(
      'place',
      new ConnectError('x', Code.FailedPrecondition, undefined, [
        {
          desc: MapBlockedSchema,
          value: create(MapBlockedSchema, { reason: MapBlockedReason.NO_GRID }),
        },
      ]),
    );
    button('Pôr no mapa').click();
    await settle();
    expect(el.querySelector('[role=alert]')?.textContent).toContain('Escolha um mapa com grade');
  });

  it('a map without a grid says "Escolha um mapa com grade" and the button rests', async () => {
    const { el, button, pickMap } = await setup();
    await pickMap('map-3');
    expect(flat(el.querySelector('[data-testid=no-grid]'))).toContain('Escolha um mapa com grade');
    expect(isOff(button('Pôr no mapa'))).toBe(true);
    expect(el.querySelector('app-map-view')).toBeNull();
    expect(Array.from(el.querySelectorAll('option')).map((o) => o.textContent?.trim())).toContain(
      'Mapa sem grade · sem grade',
    );
  });

  it('a map that is not a dungeon starts in the middle and names no room', async () => {
    const { el, button, pickMap, settle } = await setup();
    await pickMap('map-2');
    expect(el.querySelector('.where')).toBeNull();
    button('Pôr no mapa').click();
    await settle();
    expect(treasure.placed[0]).toMatchObject({ mapId: 'map-2', column: 12, row: 8 });
    expect(close).toHaveBeenCalledWith(expect.objectContaining({ place: '' }));
  });

  it('with no map at all it says so and offers nothing to place', async () => {
    const { el, button } = await setup({ prep: () => (maps.maps = []) });
    expect(flat(el.querySelector('.mr-notice'))).toContain('ainda não tem mapa');
    expect(isOff(button('Pôr no mapa'))).toBe(true);
  });

  it('on a phone it picks a room, not a square: no map is drawn, and the point falls in the middle of the room', async () => {
    const { el, button, settle } = await setup({ phone: true });
    expect(el.querySelector('app-map-view')).toBeNull();
    expect(flat(el.querySelector('.frame__sub'))).toBe('Um ponto de tesouro escondido');
    expect(Array.from(el.querySelectorAll('.rooms__grid .room')).map((r) => flat(r))).toEqual([
      'Sala 1',
      'Sala 2',
      'Sala 3',
    ]);
    el.querySelectorAll<HTMLInputElement>('.room input')[2]!.click();
    await settle();
    expect(el.querySelector('.room--on')?.textContent).toContain('Sala 3');
    button('Pôr no mapa').click();
    await settle();
    expect(treasure.placed[0]).toMatchObject({ column: 5, row: 6 });
    expect(close).toHaveBeenCalledWith(expect.objectContaining({ place: 'Sala 3' }));
  });

  it('on a phone a map without rooms puts the point in its middle and says so', async () => {
    const { el, button, pickMap, settle } = await setup({ phone: true });
    await pickMap('map-2');
    expect(flat(el.querySelector('[data-testid=no-rooms]'))).toContain(
      'o ponto cai no meio do mapa',
    );
    button('Pôr no mapa').click();
    await settle();
    expect(treasure.placed[0]).toMatchObject({ column: 12, row: 8 });
  });

  it('"Cancelar" closes with nothing', async () => {
    const { button } = await setup();
    button('Cancelar').click();
    expect(close).toHaveBeenCalledWith(undefined);
  });
});
