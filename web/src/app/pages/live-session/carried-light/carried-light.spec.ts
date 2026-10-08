import { TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { textOf as text } from '../../../core/format/text-testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { CharacterKind } from '../../../../gen/meurpg/characters/v1/characters_pb';
import { lightRadii, type LightOption, LightPresets } from '../../../core/maps/light-presets';
import { MapState } from '../../../core/maps/map-state';
import { MapsClient } from '../../../core/maps/maps-client';
import { FakeMapsClient, mapMessage, mapResponse, mapToken } from '../../../core/maps/maps-testing';
import { CarriedLight } from './carried-light';
import { type CarriedLightData, CarriedLightSheet } from './carried-light-sheet';
import { LightPanel } from './light-panel';

const options: LightOption[] = [
  { key: 'light:torch', name: 'Tocha', radii: lightRadii(20, 20) },
  { key: 'light:hooded-lantern', name: 'Lanterna coberta', radii: lightRadii(30, 30) },
  { key: 'light:light-spell', name: 'Luz', radii: lightRadii(20, 20) },
];
const presets = {
  list: vi.fn(async () => [
    ...options,
    { key: 'light:candle', name: 'Vela', radii: '1,5 m claro + 1,5 m de penumbra' },
  ]),
};

describe('the sheet "Luz que você carrega" (E9-04)', () => {
  const api = new FakeMapsClient();
  const close = vi.fn();
  const changed = vi.fn();

  function setup(current = '') {
    const data: CarriedLightData = {
      campaignId: 'c1',
      mapId: 'm1',
      characterId: 'toren',
      characterName: 'Toren',
      current,
      options,
      changed,
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: MapsClient, useValue: api },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close } },
      ],
    });
    const fixture = TestBed.createComponent(CarriedLightSheet);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }
  const radios = (el: HTMLElement) =>
    Array.from(el.querySelectorAll<HTMLInputElement>('input[type=radio]'));

  beforeEach(() => {
    api.calls = [];
    api.failWith = null;
    close.mockReset();
    changed.mockReset();
  });

  it('lists Nenhuma, Tocha, Lanterna coberta and Luz with their radii in meters, the one carried now checked', () => {
    const { el } = setup('light:torch');
    expect(Array.from(el.querySelectorAll('.opt'), (o) => text(o))).toEqual([
      'Nenhuma Sem luz: só enxerga o que já está iluminado',
      'Tocha 6 m claro + 6 m de penumbra',
      'Lanterna coberta 9 m claro + 9 m de penumbra',
      'Luz 6 m claro + 6 m de penumbra',
    ]);
    expect(radios(el).map((r) => r.checked)).toEqual([false, true, false, false]);
    expect(text(el.querySelector('h2'))).toBe('Luz que você carrega');
  });

  it('applies a choice at once, with no "Salvar", and tells the page what it did', async () => {
    const { fixture, el } = setup();
    radios(el)[1].click();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(api.calls).toEqual(['setCarriedLight m1 toren light:torch']);
    expect(changed).toHaveBeenCalledWith(
      expect.objectContaining({ carriedLight: 'light:torch' }),
      options[0],
    );
    expect(radios(el)[1].checked).toBe(true);
    expect(close).not.toHaveBeenCalled();
    expect(Array.from(el.querySelectorAll('button'), (b) => text(b))).toContain('Pronto');
  });

  it('puts the light out with "Nenhuma"', async () => {
    const { fixture, el } = setup('light:torch');
    radios(el)[0].click();
    await fixture.whenStable();
    expect(api.calls).toEqual(['setCarriedLight m1 toren ']);
    expect(changed).toHaveBeenCalledWith(expect.anything(), null);
  });

  it('keeps the choice it had and says why when the server refuses', async () => {
    const { fixture, el } = setup();
    api.failWith = new ConnectError('x', Code.PermissionDenied);
    radios(el)[1].click();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(text(el.querySelector('[role="alert"]'))).toContain(
      'Você só muda a luz do seu próprio personagem.',
    );
    expect(radios(el)[0].checked).toBe(true);
    expect(changed).not.toHaveBeenCalled();
  });

  it('closes with "Pronto" and with the ×', () => {
    const { el } = setup();
    Array.from(el.querySelectorAll('button'))
      .find((b) => text(b) === 'Pronto')!
      .click();
    expect(close).toHaveBeenCalledTimes(1);
    el.querySelector<HTMLButtonElement>('button[aria-label="Fechar"]')!.click();
    expect(close).toHaveBeenCalledTimes(2);
  });
});

describe('the row "Luz que você carrega"', () => {
  beforeEach(() =>
    TestBed.configureTestingModule({ providers: [{ provide: LightPresets, useValue: presets }] }),
  );

  function create(carried: string) {
    const fixture = TestBed.createComponent(CarriedLight);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('mapId', 'm1');
    fixture.componentRef.setInput(
      'token',
      mapToken('toren', 'Toren', { mine: true, carriedLight: carried }),
    );
    fixture.detectChanges();
    return fixture;
  }

  it('says "Nenhuma" and offers "Mudar", in one button of at least 56 px', async () => {
    const fixture = create('');
    await fixture.whenStable();
    fixture.detectChanges();
    const row = (fixture.nativeElement as HTMLElement).querySelector<HTMLButtonElement>('.row')!;
    expect(text(row)).toBe('lightbulb Luz que você carrega Nenhuma Mudar');
    expect(row.disabled).toBe(false);
  });

  it('says the name of what is carried', async () => {
    const fixture = create('light:hooded-lantern');
    await fixture.whenStable();
    fixture.detectChanges();
    expect(text((fixture.nativeElement as HTMLElement).querySelector('.row__value'))).toBe(
      'Lanterna coberta',
    );
  });
});

describe('the master\'s "Luz dos personagens"', () => {
  const api = new FakeMapsClient();

  function setup() {
    TestBed.configureTestingModule({
      providers: [
        { provide: MapsClient, useValue: api },
        { provide: LightPresets, useValue: presets },
      ],
    });
    const state = new MapState(async () => mapResponse(mapMessage('m1', 'A caverna'), [], []));
    state.apply(
      mapResponse(
        mapMessage('m1', 'A caverna'),
        [],
        [
          mapToken('pensantus', 'Pensantus'),
          mapToken('toren', 'Toren', { carriedLight: 'light:torch' }),
          mapToken('goblin', 'Goblin 1', { kind: CharacterKind.MINION }),
          mapToken('nanquim', 'Nanquim', { creatureId: 'c1', kind: CharacterKind.UNSPECIFIED }),
        ],
      ),
    );
    const fixture = TestBed.createComponent(LightPanel);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('state', state);
    fixture.componentRef.setInput(
      'info',
      new Map([['toren', { classSummary: 'Guerreiro 5', playerName: 'Caio' }]]),
    );
    fixture.detectChanges();
    return { fixture, state, el: fixture.nativeElement as HTMLElement };
  }

  beforeEach(() => {
    api.calls = [];
    api.failWith = null;
  });

  it('has one select for each player character on the map, with what they carry, and none for NPCs or creatures', async () => {
    const { fixture, el } = setup();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(Array.from(el.querySelectorAll('.row__name'), (n) => text(n))).toEqual([
      'Pensantus',
      'Toren',
    ]);
    const selects = Array.from(el.querySelectorAll<HTMLSelectElement>('select'));
    expect(selects.map((s) => s.value)).toEqual(['', 'light:torch']);
    expect(Array.from(selects[0].options, (o) => text(o))).toEqual([
      'Nenhuma',
      'Tocha',
      'Lanterna coberta',
      'Luz',
    ]);
    expect(selects[1].getAttribute('aria-label')).toBe('Luz de Toren');
    expect(text(el.querySelectorAll('.row__sub')[1])).toBe('Caio');
  });

  it("sets the light of anyone at once, puts the new token on the map's state, and says what it did", async () => {
    const { fixture, state, el } = setup();
    await fixture.whenStable();
    fixture.detectChanges();
    const select = el.querySelectorAll<HTMLSelectElement>('select')[0];
    select.value = 'light:hooded-lantern';
    select.dispatchEvent(new Event('change'));
    await fixture.whenStable();
    fixture.detectChanges();
    expect(api.calls).toEqual(['setCarriedLight m1 pensantus light:hooded-lantern']);
    expect(state.tokens().find((t) => t.characterId === 'pensantus')?.carriedLight).toBe(
      'light:hooded-lantern',
    );
    expect(text(el.querySelector('.line'))).toContain(
      'Pensantus carrega lanterna coberta (9 m claro + 9 m de penumbra). Muda na hora no mapa de todos que enxergam esse lugar.',
    );
  });

  it('takes the line of the last change off the screen when the next one is refused', async () => {
    const { fixture, el } = setup();
    await fixture.whenStable();
    fixture.detectChanges();
    const select = el.querySelectorAll<HTMLSelectElement>('select')[0];
    select.value = 'light:hooded-lantern';
    select.dispatchEvent(new Event('change'));
    await fixture.whenStable();
    fixture.detectChanges();
    expect(text(el.querySelector('.line'))).toContain('Pensantus carrega');
    api.failWith = new ConnectError('x', Code.PermissionDenied);
    select.value = '';
    select.dispatchEvent(new Event('change'));
    await fixture.whenStable();
    fixture.detectChanges();
    expect(el.querySelector('[role="alert"]')).not.toBeNull();
    expect(el.querySelector<HTMLElement>('.line')?.hidden).toBe(true);
  });
});
