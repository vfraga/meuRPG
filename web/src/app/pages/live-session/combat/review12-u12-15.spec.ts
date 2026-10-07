// Finding U12-15: a failed read of the session's current map (MapState 'error') leaves map() null, so
// "Iniciar combate" opens the dialog with map:null and the combat defaults to the theatre of the mind (RN-25, fixed once started).
import { ChangeDetectionStrategy, Component } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MatDialog } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';
import { of } from 'rxjs';

import type { GetMapResponse } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { MapState } from '../../../core/maps/map-state';
import { CombatLaunch } from './combat-launch';

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [CombatLaunch],
  template: '<app-combat-launch campaignId="camp-1" [state]="state" />',
})
class Host {
  state = new MapState(() =>
    Promise.reject<GetMapResponse>(new ConnectError('blip', Code.Unavailable)),
  );
}

describe('Review12 U12-15: start combat after a failed current-map read', () => {
  it('does not offer a theatre combat for a session whose map failed to load', async () => {
    const opened: { data: { map: unknown } }[] = [];
    TestBed.configureTestingModule({
      providers: [
        {
          provide: MatDialog,
          useValue: {
            open: (_c: unknown, config: { data: { map: unknown } }) => {
              opened.push(config);
              return { afterClosed: () => of(undefined) };
            },
          },
        },
      ],
    });
    const fixture = TestBed.createComponent(Host);
    const el = fixture.nativeElement as HTMLElement;
    await fixture.componentInstance.state.open('map-1');
    for (let i = 0; i < 3; i++) {
      fixture.detectChanges();
      await fixture.whenStable();
    }
    expect(fixture.componentInstance.state.status()).toBe('error');
    const btn = Array.from(el.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Iniciar combate'),
    )!;
    btn.click();
    const offeredTheatre = opened.some((o) => o.data.map === null);
    expect(offeredTheatre || !btn.disabled ? 'opens with map:null' : 'blocked').toBe('blocked');
    expect(el.textContent).not.toContain('Sem um mapa atual');
  });
});
