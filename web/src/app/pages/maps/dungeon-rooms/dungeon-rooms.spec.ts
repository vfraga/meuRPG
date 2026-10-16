import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';

import { DungeonsClient } from '../../../core/maps/dungeons-client';
import { FakeDungeonsClient, roomsResponse } from '../../../core/maps/dungeons-testing';
import { MapState } from '../../../core/maps/map-state';
import { mapMessage, mapResponse } from '../../../core/maps/maps-testing';
import { DungeonRooms, type RoomOutline } from './dungeon-rooms';
import { nextTurn } from '../../../../test-settle';

describe('DungeonRooms (E10-05 5)', () => {
  let fixture: ComponentFixture<DungeonRooms>;
  let el: HTMLElement;
  let api: FakeDungeonsClient;
  let state: MapState;
  let outlines: (RoomOutline | null)[];
  let changed: number;

  async function setup(info = roomsResponse()) {
    api = new FakeDungeonsClient();
    outlines = [];
    changed = 0;
    state = new MapState(async () => mapResponse(mapMessage('dungeon-1', 'Masmorra'), [], []));
    await state.open('dungeon-1');
    TestBed.configureTestingModule({ providers: [{ provide: DungeonsClient, useValue: api }] });
    fixture = TestBed.createComponent(DungeonRooms);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('mapId', 'dungeon-1');
    fixture.componentRef.setInput('info', info);
    fixture.componentRef.setInput('state', state);
    fixture.componentInstance.outline.subscribe((o) => outlines.push(o));
    fixture.componentInstance.changed.subscribe(() => changed++);
    fixture.detectChanges();
    el = fixture.nativeElement;
    await fixture.whenStable();
  }

  const text = (e: Element = el) => (e.textContent ?? '').replace(/\s+/g, ' ');
  const rooms = () => Array.from(el.querySelectorAll('li.room'));
  const settle = async () => {
    fixture.detectChanges();
    await nextTurn();
    await fixture.whenStable();
    fixture.detectChanges();
  };

  it("says it is the master's list and that the image carries no room numbers", async () => {
    await setup();
    expect(text()).toContain('Só você vê');
    expect(text()).toContain('A imagem não leva números de sala.');
    expect(el.querySelector('h2')?.textContent).toBe('Salas');
  });

  it('lists each room with its number and size in meters', async () => {
    await setup();
    expect(rooms()).toHaveLength(3);
    expect(text(rooms()[0]!)).toContain('Sala 1');
    expect(text(rooms()[0]!).replace(/\u00a0/g, ' ')).toContain('4,5 × 4,5 m');
    expect(text(rooms()[1]!).replace(/\u00a0/g, ' ')).toContain('7,5 × 4,5 m');
  });

  it('keeps its live region in the page, hidden while empty (not display: none)', async () => {
    await setup();
    const region = el.querySelector('.rooms__done')!;
    expect(region.getAttribute('role')).toBe('status');
    expect(region.classList).toContain('mr-visually-hidden');
    expect(region.textContent).toBe('');
  });

  it('lists the exits with their true door kinds, the secret door too', async () => {
    await setup();
    expect(text(rooms()[0]!)).toContain('Porta trancada ao sul, para um corredor');
    expect(text(rooms()[0]!)).toContain('Passagem ao leste, para a sala 2');
    expect(text(rooms()[1]!)).toContain('Porta secreta ao sul, para um corredor');
    expect(text(rooms()[2]!)).toContain('Porta fechada ao norte, para um corredor');
  });

  it('says a trapped door once, in the exits, next to the door that has the trap ("Porta com armadilha")', async () => {
    await setup();
    const trapped = rooms()[0]!.querySelectorAll('.room__exits li .room__trap');
    expect(trapped).toHaveLength(1);
    expect(trapped[0]!.closest('li')?.textContent).toContain(
      'Porta trancada ao sul, para um corredor',
    );
    expect(trapped[0]!.textContent).toContain('Porta com armadilha');
    expect(trapped[0]!.querySelector('mat-icon')?.textContent).toBe('warning');
    expect(el.querySelectorAll('.room__trap')).toHaveLength(1);
    expect(text(rooms()[1]!)).not.toContain('armadilha');
  });

  it('marks the room behind a secret door, and the stairs on a floor, the entrance first', async () => {
    await setup();
    expect(text(rooms()[1]!)).toContain('Atrás de uma porta secreta');
    expect(text(rooms()[0]!)).not.toContain('Atrás de uma porta secreta');
    expect(text(rooms()[0]!)).toContain('Escada para cima: a entrada');
    expect(text(rooms()[1]!)).toContain('Escada para baixo');
    expect(text(rooms()[2]!)).not.toContain('Escada');
    expect(rooms()[0]!.querySelector('app-stair-mark mat-icon')?.textContent).toBe('arrow_upward');
  });

  it('lists a stair outside every room, at the end of a corridor, and names the entrance', async () => {
    const info = roomsResponse();
    // The sample's up stair at (1, 1) is on room 1's floor; drop room 1 from the list and it stands in a corridor.
    await setup(roomsResponse({ rooms: info.rooms.slice(1) }));
    expect(text(el.querySelector('.rooms__stairs')!)).toContain('Escada para cima: a entrada');
    expect(text(el.querySelector('.rooms__stairs')!)).toContain('num corredor, coluna 2, linha 2');
    expect(el.querySelectorAll('.stairrow')).toHaveLength(1);
    expect(el.querySelector('.rooms__apart')?.textContent).toBe('Fora das salas');
  });

  it('counts the scenes already in a room', async () => {
    await setup();
    expect(rooms()[1]!.querySelector('.room__scenes')?.textContent).toBe('1 cena nesta sala');
    expect(rooms()[0]!.querySelector('.room__scenes')).toBeNull();
  });

  it('outlines the chosen room on the map (its floor) and lets go on the second choice', async () => {
    await setup();
    const head = rooms()[1]!.querySelector<HTMLButtonElement>('.room__head')!;
    head.click();
    await settle();
    expect(head.getAttribute('aria-pressed')).toBe('true');
    expect(rooms()[1]!.classList).toContain('room--on');
    expect(outlines.at(-1)).toEqual({ x: 5, y: 1, width: 5, height: 3 });
    // Another room takes over; the same one again lets go.
    rooms()[0]!.querySelector<HTMLButtonElement>('.room__head')!.click();
    await settle();
    expect(rooms()[1]!.classList).not.toContain('room--on');
    expect(outlines.at(-1)).toEqual({ x: 1, y: 1, width: 3, height: 3 });
    rooms()[0]!.querySelector<HTMLButtonElement>('.room__head')!.click();
    await settle();
    expect(outlines.at(-1)).toBeNull();
  });

  describe('"Pôr uma cena nesta sala"', () => {
    const place = (i: number) =>
      Array.from(rooms()[i]!.querySelectorAll('button')).find(
        (b) => b.textContent?.trim() === 'Pôr uma cena nesta sala',
      )!;

    it("puts the scene on the server's answer, shows it on the map at once and says so", async () => {
      await setup();
      place(0).click();
      await settle();
      expect(api.calls).toContain('placeScene dungeon-1 1');
      expect(state.points().map((p) => p.name)).toEqual(['Sala 1']);
      expect(el.querySelector('.rooms__done')?.textContent).toBe(
        'Cena “Sala 1” posta no mapa, escondida dos jogadores.',
      );
      // The list is read again (the room now counts its scene).
      expect(changed).toBe(1);
    });

    it('can be repeated: each call makes a point', async () => {
      await setup();
      place(2).click();
      await settle();
      place(2).click();
      await settle();
      expect(api.calls.filter((c) => c.startsWith('placeScene'))).toHaveLength(2);
    });

    it('says the map is full (200 points) on the room, with the reason', async () => {
      await setup();
      api.failWith.set('placeScene', new ConnectError('full', Code.ResourceExhausted));
      place(1).click();
      await settle();
      expect(rooms()[1]!.querySelector('[role="alert"]')?.textContent).toContain('200 pontos');
      expect(rooms()[0]!.querySelector('[role="alert"]')).toBeNull();
      expect(state.points()).toEqual([]);
      expect(changed).toBe(0);
    });
  });
});
