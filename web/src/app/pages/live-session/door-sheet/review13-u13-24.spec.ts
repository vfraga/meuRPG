// Finding U13-24 (review/unit-13-web-live-rest.md): DoorSheet caches the door state at open time in `now`, so after a player opens the door, tapping the checked "Fechada" returns early and paints nothing.
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { MapLayer } from '../../../../gen/meurpg/maps/v1/maps_pb';
import type { DoorSquare } from '../../../core/maps/layers';
import { MapsClient } from '../../../core/maps/maps-client';
import { FakeMapsClient } from '../../../core/maps/maps-testing';
import { DoorSheet, type DoorSheetData } from './door-sheet';

const plain = (t: string | null | undefined) =>
  (t ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

describe('Review13 U13-24: door sheet keeps a stale door state and swallows the master tap', () => {
  function setup(door: DoorSquare) {
    const api = new FakeMapsClient();
    const data: DoorSheetData = { campaignId: 'camp-1', mapId: 'map-1', door, wall: false };
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: MapsClient, useValue: api },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: () => undefined } },
      ],
    });
    const fixture = TestBed.createComponent(DoorSheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const press = async (name: string) => {
      Array.from(el.querySelectorAll('button'))
        .find((b) => plain(b.textContent).includes(name))!
        .click();
      fixture.detectChanges();
      await new Promise((r) => setTimeout(r));
      await fixture.whenStable();
      fixture.detectChanges();
    };
    return { api, press };
  }

  it('control: tapping a different choice paints it', async () => {
    const { api, press } = setup({ col: 4, row: 2, state: 2, axis: 'h' });
    await press('Trancada');
    expect(api.paints).toEqual([
      { layer: MapLayer.DOORS, value: 3, squares: [{ col: 4, row: 2 }] },
    ]);
  });

  it('after a player opens the door (state 1), the master tapping "Fechada" closes it', async () => {
    const door: DoorSquare = { col: 4, row: 2, state: 2, axis: 'h' };
    const { api, press } = setup(door);
    // The door opens elsewhere (a player's move, another tab). The sheet has no other live channel than its data.
    (door as { state: number }).state = 1;
    await press('Fechada');
    expect(api.paints).toEqual([
      { layer: MapLayer.DOORS, value: 2, squares: [{ col: 4, row: 2 }] },
    ]);
  });
});
