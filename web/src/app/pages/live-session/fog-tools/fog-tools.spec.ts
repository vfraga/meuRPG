import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import {
  CharacterCreatureSchema,
  CreatureSource,
} from '../../../../gen/meurpg/characters/v1/characters_pb';
import { textOf } from '../../../core/format/text-testing';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { lightRadii, LightPresets } from '../../../core/maps/light-presets';
import { MapState } from '../../../core/maps/map-state';
import { MapsClient } from '../../../core/maps/maps-client';
import { FakeMapsClient, mapMessage, mapResponse, mapToken } from '../../../core/maps/maps-testing';
import { FamiliarRow } from './familiar-row';
import { FogPlayerTools } from './fog-player-tools';

const raven = create(CharacterCreatureSchema, {
  id: 'nanquim',
  characterId: 'pensantus',
  monsterKey: 'monster:raven',
  monsterNamePt: 'Corvo',
  name: 'Nanquim',
  source: CreatureSource.FAMILIAR,
});
const wolf = create(CharacterCreatureSchema, {
  id: 'lobo',
  characterId: 'pensantus',
  monsterKey: 'monster:wolf',
  monsterNamePt: 'Lobo',
  name: 'Lobo',
  source: CreatureSource.CONJURE_ANIMALS,
});

describe("the familiar's row (E9-04)", () => {
  const list = vi.fn();
  beforeEach(() => {
    list.mockReset();
    TestBed.configureTestingModule({
      providers: [{ provide: CreaturesClient, useValue: { list } }],
    });
  });

  function create_(inputs: Record<string, unknown> = {}) {
    const fixture = TestBed.createComponent(FamiliarRow);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('characterId', 'pensantus');
    fixture.componentRef.setInput('characterName', 'Pensantus');
    for (const [k, v] of Object.entries(inputs)) {
      fixture.componentRef.setInput(k, v);
    }
    fixture.detectChanges();
    return fixture;
  }

  it('shows the creature\'s picture, its name, "Corvo · familiar de Pensantus" and the button, with no distance', async () => {
    list.mockResolvedValue([wolf, raven]);
    const fixture = create_();
    await fixture.whenStable();
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelector('app-creature-art')).not.toBeNull();
    expect(textOf(el.querySelector('.fr__text'))).toBe('Nanquim Corvo · familiar de Pensantus');
    expect(textOf(el.querySelector('app-familiar-eyes-button'))).toBe('visibility Ver pelos olhos');
    expect(textOf(el)).not.toMatch(/\d+ ?m\b/);
  });

  it('has no row for a character without a familiar, for a refused list, and while it already looks through its eyes', async () => {
    list.mockResolvedValue([wolf]);
    const none = create_();
    await none.whenStable();
    none.detectChanges();
    expect((none.nativeElement as HTMLElement).querySelector('.fr')).toBeNull();
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [{ provide: CreaturesClient, useValue: { list } }],
    });
    list.mockRejectedValue(new Error('not found'));
    const refused = create_();
    await refused.whenStable();
    refused.detectChanges();
    expect((refused.nativeElement as HTMLElement).querySelector('.fr')).toBeNull();
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [{ provide: CreaturesClient, useValue: { list } }],
    });
    list.mockResolvedValue([raven]);
    const seeing = create_({ seeing: true });
    await seeing.whenStable();
    seeing.detectChanges();
    expect((seeing.nativeElement as HTMLElement).querySelector('.fr')).toBeNull();
  });

  it("does not show the last character's familiar while another character's list is on its way", async () => {
    list.mockResolvedValue([raven]);
    const fixture = create_();
    await fixture.whenStable();
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelector('.fr')).not.toBeNull();
    list.mockReturnValue(new Promise(() => undefined));
    fixture.componentRef.setInput('characterId', 'brisa');
    fixture.detectChanges();
    expect(el.querySelector('.fr')).toBeNull();
  });

  it('reads the list again when the stream says the creatures changed', async () => {
    list.mockResolvedValue([]);
    const fixture = create_();
    await fixture.whenStable();
    list.mockResolvedValue([raven]);
    fixture.componentRef.setInput('reload', 1);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    expect((fixture.nativeElement as HTMLElement).querySelector('.fr')).not.toBeNull();
  });
});

describe("the player's fog tools: the light confirmation is a toast over the page", () => {
  const api = new FakeMapsClient();

  beforeEach(() => {
    vi.useFakeTimers();
    TestBed.configureTestingModule({
      providers: [
        { provide: MapsClient, useValue: api },
        {
          provide: LightPresets,
          useValue: {
            list: () =>
              Promise.resolve([{ key: 'light:torch', name: 'Tocha', radii: lightRadii(20, 20) }]),
          },
        },
        { provide: CreaturesClient, useValue: { list: () => Promise.resolve([]) } },
      ],
    });
  });
  afterEach(() => vi.useRealTimers());

  it('says what the light did for six seconds, takes no room in the layout, and puts the new token on the map', () => {
    const state = new MapState(async () => mapResponse(mapMessage('m1', 'M'), [], []));
    state.apply(
      mapResponse(mapMessage('m1', 'M'), [], [mapToken('toren', 'Toren', { mine: true })]),
    );
    const fixture = TestBed.createComponent(FogPlayerTools);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('state', state);
    fixture.componentRef.setInput('mapId', 'm1');
    fixture.componentRef.setInput('own', state.tokens()[0]);
    fixture.detectChanges();
    const tools = fixture.componentInstance as unknown as {
      lightChanged(c: {
        token: ReturnType<typeof mapToken>;
        option: { key: string; name: string; radii: string } | null;
      }): void;
    };
    tools.lightChanged({
      token: mapToken('toren', 'Toren', { mapId: 'm1', mine: true, carriedLight: 'light:torch' }),
      option: { key: 'light:torch', name: 'Tocha', radii: '6 m claro + 6 m de penumbra' },
    });
    fixture.detectChanges();
    const toast = (fixture.nativeElement as HTMLElement).querySelector('.toast')!;
    expect(toast.getAttribute('role')).toBe('status');
    expect(textOf(toast)).toBe('lightbulb Você acendeu a tocha: 6 m claro + 6 m de penumbra.');
    expect(state.tokens()[0].carriedLight).toBe('light:torch');
    vi.advanceTimersByTime(6000);
    fixture.detectChanges();
    expect((fixture.nativeElement as HTMLElement).querySelector('.toast')).toBeNull();
  });
});
