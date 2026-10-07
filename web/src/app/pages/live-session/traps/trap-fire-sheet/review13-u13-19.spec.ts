// Finding U13-19 (review/unit-13-web-live-rest.md): the fire sheet keeps one idempotency key per sheet, so a retry with other targets reuses it.
import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { create } from '@bufbuild/protobuf';
import { ConnectError, Code } from '@connectrpc/connect';

import { MapPointKind, MapPointSchema } from '../../../../../gen/meurpg/maps/v1/maps_pb';
import { TrapsClient } from '../../../../core/traps/traps-client';
import { TrapFireSheet, type TrapFireData } from './trap-fire-sheet';

describe('Review13 U13-19: fire sheet key follows the request', () => {
  function setup() {
    const calls: unknown[][] = [];
    const api = {
      fire: async (...a: unknown[]) => {
        calls.push(a);
        if (calls.length === 1) {
          throw new ConnectError('lost', Code.Unavailable);
        }
        return { firing: { caught: [] } };
      },
    };
    const data: TrapFireData = {
      campaignId: 'c',
      mapId: 'm',
      point: create(MapPointSchema, { id: 'x', kind: MapPointKind.TRAP, name: 'Fosso' }),
      targets: signal([
        { id: 't', name: 'Toren', sub: '' },
        { id: 'b', name: 'Brisa', sub: '' },
      ]),
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
    const go = async () => {
      el.querySelector<HTMLButtonElement>('.pf__go')!.click();
      await fixture.whenStable();
      fixture.detectChanges();
    };
    const toggle = (i: number) => {
      el.querySelectorAll<HTMLInputElement>('input[type=checkbox]')[i].click();
      fixture.detectChanges();
    };
    return { calls, go, toggle };
  }

  it('control: the same request retried keeps its key', async () => {
    const { calls, go, toggle } = setup();
    toggle(0);
    await go();
    await go();
    expect(calls.length).toBe(2);
    expect(calls[1][3]).toEqual(['t']);
    expect(calls[1][4]).toBe(calls[0][4]);
  });

  it('a retry with other targets is another request and needs a new key', async () => {
    const { calls, go, toggle } = setup();
    toggle(0);
    await go();
    toggle(0);
    toggle(1);
    await go();
    expect(calls.length).toBe(2);
    expect(calls[0][3]).toEqual(['t']);
    expect(calls[1][3]).toEqual(['b']);
    expect(calls[1][4]).not.toBe(calls[0][4]);
  });
});
