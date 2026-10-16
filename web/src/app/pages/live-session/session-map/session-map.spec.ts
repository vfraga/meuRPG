import { TestBed } from '@angular/core/testing';
import { By } from '@angular/platform-browser';
import { provideRouter } from '@angular/router';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { MapState, tokenKey } from '../../../core/maps/map-state';
import { MapsClient } from '../../../core/maps/maps-client';
import { SceneClient } from '../../../core/play/scene-client';
import { mapMessage, mapPoint, mapResponse, mapToken } from '../../../core/maps/maps-testing';
import { create } from '@bufbuild/protobuf';

import { MapPointKind, MapRefSchema } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { MapView } from '../../../shared/map-view/map-view';
import { LiveSessionSource } from '../../../core/play/live-session.types';
import { SessionMap } from './session-map';

describe('SessionMap dragging a token', () => {
  const placeToken = vi.fn();
  const raven = mapToken('pens', 'Nanquim', { creatureId: 'raven', xBp: 2000, yBp: 2000 });
  const owner = mapToken('pens', 'Pensantus', { mine: true, xBp: 6000, yBp: 6000 });

  async function mount(tokens = [raven, owner]) {
    const state = new MapState(async () =>
      mapResponse(mapMessage('map-1', 'Torre', { revealed: true }), [], tokens),
    );
    await state.open('map-1');
    const fixture = TestBed.createComponent(SessionMap);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('state', state);
    fixture.componentRef.setInput('mapId', 'map-1');
    fixture.componentRef.setInput('isMaster', true);
    fixture.detectChanges();
    const view = fixture.debugElement.query(By.directive(MapView)).componentInstance as MapView;
    return { state, view, fixture };
  }

  const at = (state: MapState, key: string) =>
    state
      .tokens()
      .filter((t) => tokenKey(t) === key)
      .map((t) => [t.xBp, t.yBp]);

  beforeEach(() => {
    placeToken.mockReset();
    placeToken.mockImplementation(async (_c, _m, id, xBp, yBp) => mapToken(id, 'x', { xBp, yBp }));
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: MapsClient, useValue: { placeToken } },
        { provide: LiveSessionSource, useValue: {} },
        { provide: SceneClient, useValue: {} },
      ],
    });
  });

  it('saves the move of a character token by its character id', async () => {
    const { state, view } = await mount([owner]);
    view.moved.emit({ kind: 'token', id: 'pens', xBp: 7000, yBp: 7000 });
    await Promise.resolve();
    expect(placeToken).toHaveBeenCalledWith('camp-1', 'map-1', 'pens', 7000, 7000);
    expect(at(state, 'pens')).toEqual([[7000, 7000]]);
  });

  it('saves the move of a creature token by its creature id', async () => {
    const { view } = await mount();
    view.moved.emit({ kind: 'token', id: 'raven', xBp: 3000, yBp: 3000 });
    await Promise.resolve();
    expect(placeToken).toHaveBeenCalledTimes(1);
    expect(placeToken).toHaveBeenCalledWith('camp-1', 'map-1', '', 3000, 3000, 'raven');
  });

  it("moves the owner's token, not its creature's, when the owner is dragged", async () => {
    const { state, view } = await mount();
    view.moved.emit({ kind: 'token', id: 'pens', xBp: 7000, yBp: 7000 });
    await Promise.resolve();
    expect(at(state, 'raven')).toEqual([[2000, 2000]]);
    expect(at(state, 'pens')).toEqual([[7000, 7000]]);
  });

  it('drops the banner of a refused move once a later move is saved', async () => {
    placeToken.mockRejectedValueOnce(new Error('refused'));
    const { view, fixture } = await mount([owner]);
    const banner = () => (fixture.nativeElement as HTMLElement).querySelector('[role="alert"]');
    view.moved.emit({ kind: 'token', id: 'pens', xBp: 7000, yBp: 7000 });
    await fixture.whenStable();
    fixture.detectChanges();
    expect(banner()).not.toBeNull();
    view.moved.emit({ kind: 'token', id: 'pens', xBp: 7100, yBp: 7100 });
    await fixture.whenStable();
    fixture.detectChanges();
    expect(banner()).toBeNull();
  });
});

describe('SessionMap: "Ir para <mapa>" on a Submapa point', () => {
  const setCurrentMap = vi.fn();

  async function mount() {
    const stairs = mapPoint('p-sub', 'Torre', {
      kind: MapPointKind.SUBMAP,
      revealed: true,
      targetMap: create(MapRefSchema, { id: 'map-2', name: 'Interior da torre' }),
    });
    const noTarget = mapPoint('p-none', 'Escada', { kind: MapPointKind.SUBMAP, revealed: true });
    const state = new MapState(async () =>
      mapResponse(mapMessage('map-1', 'Torre', { revealed: true }), [stairs, noTarget], []),
    );
    await state.open('map-1');
    const fixture = TestBed.createComponent(SessionMap);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('state', state);
    fixture.componentRef.setInput('mapId', 'map-1');
    fixture.componentRef.setInput('isMaster', true);
    const changed: (string | null)[] = [];
    fixture.componentInstance.currentChanged.subscribe((m) => changed.push(m));
    fixture.detectChanges();
    return { fixture, changed, el: fixture.nativeElement as HTMLElement };
  }

  beforeEach(() => {
    setCurrentMap.mockReset();
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: MapsClient, useValue: {} },
        { provide: LiveSessionSource, useValue: { setCurrentMap } },
        { provide: SceneClient, useValue: {} },
      ],
    });
  });

  const goButtons = (el: HTMLElement) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('.row__go'));

  it('offers it only on a Submapa point that has a destination', async () => {
    const { el } = await mount();
    expect(goButtons(el)).toHaveLength(1);
    expect(goButtons(el)[0].textContent).toContain('Ir para Interior da torre');
    expect(goButtons(el)[0].getAttribute('aria-label')).toBe(
      'Levar a sessão para Interior da torre',
    );
  });

  it("changes the session's current map to the destination", async () => {
    setCurrentMap.mockResolvedValue('map-2');
    const { fixture, changed, el } = await mount();
    goButtons(el)[0].click();
    await fixture.whenStable();
    expect(setCurrentMap).toHaveBeenCalledWith('camp-1', 'map-2');
    expect(changed).toEqual(['map-2']);
  });

  it('says so when the session is over', async () => {
    const { ConnectError, Code } = await import('@connectrpc/connect');
    setCurrentMap.mockRejectedValue(new ConnectError('no', Code.FailedPrecondition));
    const { fixture, changed, el } = await mount();
    goButtons(el)[0].click();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(changed).toEqual([]);
    expect(el.textContent).toContain('A sessão acabou: o mapa só muda durante a sessão.');
  });
});
