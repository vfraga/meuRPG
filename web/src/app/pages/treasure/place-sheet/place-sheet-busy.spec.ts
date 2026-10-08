import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { XpMode } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { flat } from '../../../core/creatures/creatures-testing';
import { DungeonsClient } from '../../../core/maps/dungeons-client';
import { FakeDungeonsClient } from '../../../core/maps/dungeons-testing';
import { MapsClient } from '../../../core/maps/maps-client';
import { FakeMapsClient, mapMessage, mapResponse } from '../../../core/maps/maps-testing';
import { TreasureClient } from '../../../core/treasure/treasure-client';
import { FakeTreasureClient, sampleHoard } from '../../../core/treasure/treasure-testing';
import { PlaceSheet, type PlaceSheetData } from './place-sheet';

describe('PlaceSheet while the request runs', () => {
  it('refuses to dismiss the sheet while PlaceTreasure is in flight (else the result is lost and a retry uses a new key)', async () => {
    const treasure = new FakeTreasureClient();
    const maps = new FakeMapsClient();
    maps.maps = [mapMessage('map-1', 'Masmorra', { gridColumns: 11, gridRows: 9, current: true })];
    maps.responses.set('map-1', mapResponse(maps.maps[0]));
    let release!: () => void;
    treasure.gate = new Promise<void>((r) => (release = r));
    const close = vi.fn();
    const data: PlaceSheetData = {
      campaignId: 'camp-1',
      treasure: sampleHoard(),
      xpMode: XpMode.ENEMIES,
      campaignName: 'Mirathel',
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: TreasureClient, useValue: treasure },
        { provide: MapsClient, useValue: maps },
        { provide: DungeonsClient, useValue: new FakeDungeonsClient() },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close } },
      ],
    });
    const fixture = TestBed.createComponent(PlaceSheet);
    const settle = async () => {
      for (let i = 0; i < 6; i++) {
        fixture.detectChanges();
        await fixture.whenStable();
        await new Promise<void>((resolve) => setTimeout(resolve, 0));
      }
    };
    await settle();
    const el = fixture.nativeElement as HTMLElement;
    Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => flat(b)?.startsWith('Pôr no mapa'))!
      .click();
    await settle();
    expect(treasure.placed).toHaveLength(1); // the request is running (busy)

    // Esc / backdrop / the X of the frame: all dismiss the sheet; the X emits the frame's `closed`.
    el.querySelector<HTMLButtonElement>('.frame__close')!.click();
    await settle();

    expect(
      close,
      'the sheet was dismissed while busy; the later result goes nowhere',
    ).not.toHaveBeenCalled();
    release();
  });
});
