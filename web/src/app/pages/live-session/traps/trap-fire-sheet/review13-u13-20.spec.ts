// Finding U13-20 (review/unit-13-web-live-rest.md): a ticked person who leaves the target list silently turns "fire for Toren" into "fire for whoever is in the area" (targetIds []).
import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { create } from '@bufbuild/protobuf';

import { MapPointKind, MapPointSchema } from '../../../../../gen/meurpg/maps/v1/maps_pb';
import { TrapsClient } from '../../../../core/traps/traps-client';
import { TrapFireSheet, type TrapFireData } from './trap-fire-sheet';

describe('Review13 U13-20: a picked target that vanishes must not become "the whole area"', () => {
  function setup() {
    const calls: unknown[][] = [];
    const api = { fire: async (...a: unknown[]) => (calls.push(a), { firing: { caught: [] } }) };
    const rows = signal([
      { id: 't', name: 'Toren', sub: 'Perto' },
      { id: 'b', name: 'Brisa', sub: 'Longe' },
    ]);
    const data: TrapFireData = {
      campaignId: 'c',
      mapId: 'm',
      point: create(MapPointSchema, { id: 'x', kind: MapPointKind.TRAP, name: 'Fosso' }),
      targets: rows,
      extendFiringId: '',
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: TrapsClient, useValue: api },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: vi.fn() } },
      ],
    });
    const fixture = TestBed.createComponent(TrapFireSheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    el.querySelectorAll<HTMLInputElement>('input[type=checkbox]')[0].click();
    fixture.detectChanges();
    return { fixture, el, calls, rows };
  }

  it('control: Toren ticked and still listed fires for Toren', async () => {
    const { fixture, el, calls } = setup();
    el.querySelector<HTMLButtonElement>('.pf__go')!.click();
    await fixture.whenStable();
    expect(calls[0][3]).toEqual(['t']);
  });

  it('does not send an empty target list (= everyone in the area) after the ticked person left', async () => {
    const { fixture, el, calls, rows } = setup();
    rows.set([{ id: 'b', name: 'Brisa', sub: 'Longe' }]);
    fixture.detectChanges();
    el.querySelector<HTMLButtonElement>('.pf__go')!.click();
    await fixture.whenStable();
    const sent = calls.map((c) => c[3]);
    expect(sent).not.toContainEqual([]);
  });
});
