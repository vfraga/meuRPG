import { ChangeDetectionStrategy, Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MatDialog } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';
import { of } from 'rxjs';

import type { GetMapResponse } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { MapState } from '../../../core/maps/map-state';
import { mapMessage, mapResponse } from '../../../core/maps/maps-testing';
import { CombatLaunch } from './combat-launch';

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [CombatLaunch],
  template:
    '<app-combat-launch campaignId="camp-1" [state]="state" (started)="started.set($event.name)" />',
})
class Host {
  answer!: (r: GetMapResponse) => void;
  fail!: (e: unknown) => void;
  state = new MapState(
    () =>
      new Promise<GetMapResponse>((resolve, reject) => {
        this.answer = resolve;
        this.fail = reject;
      }),
  );
  started = signal('');
}

describe('CombatLaunch: "Iniciar combate" on the master\'s session (MR-013)', () => {
  let opened: { data: { map: { id: string; columns: number } | null } }[];

  async function setup() {
    opened = [];
    TestBed.configureTestingModule({
      providers: [
        {
          provide: MatDialog,
          useValue: {
            open: (_c: unknown, config: (typeof opened)[number]) => {
              opened.push(config);
              return { afterClosed: () => of(undefined) };
            },
          },
        },
      ],
    });
    const fixture = TestBed.createComponent(Host);
    const settle = async () => {
      for (let i = 0; i < 3; i++) {
        fixture.detectChanges();
        await fixture.whenStable();
      }
    };
    await settle();
    return { fixture, settle };
  }

  const button = (el: HTMLElement) =>
    Array.from(el.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Iniciar combate'),
    )!;

  it('waits while the current map is read, so the dialog never opens with "no map" for a session that has one', async () => {
    const { fixture, settle } = await setup();
    const host = fixture.componentInstance;
    const el = fixture.nativeElement as HTMLElement;
    void host.state.open('map-1');
    await settle();
    expect(el.textContent).toContain('Lendo o mapa atual…');
    expect(button(el).disabled).toBe(true);
    button(el).click();
    expect(opened).toHaveLength(0);

    host.answer(
      mapResponse(mapMessage('map-1', 'Estrada do Vale', { gridColumns: 20, gridRows: 14 })),
    );
    await settle();
    expect(el.textContent).toContain('Escolha quem luta e o app pede a iniciativa de todos.');
    expect(button(el).disabled).toBe(false);
    button(el).click();
    expect(opened).toHaveLength(1);
    expect(opened[0].data.map?.id).toBe('map-1');
    expect(opened[0].data.map?.columns).toBe(20);
  });

  it('opens at once when the session has no current map (the theatre of the mind)', async () => {
    const { fixture, settle } = await setup();
    const el = fixture.nativeElement as HTMLElement;
    await fixture.componentInstance.state.open(null);
    await settle();
    expect(el.textContent).toContain('Sem um mapa atual, o combate é no teatro da mente');
    button(el).click();
    expect(opened).toHaveLength(1);
    expect(opened[0].data.map).toBeNull();
  });

  it('does not start a combat in the theatre of the mind when the current map could not be read', async () => {
    const { fixture, settle } = await setup();
    const el = fixture.nativeElement as HTMLElement;
    const read = fixture.componentInstance.state.open('map-1');
    fixture.componentInstance.fail(new ConnectError('blip', Code.Unavailable));
    await read;
    await settle();
    expect(fixture.componentInstance.state.status()).toBe('error');
    expect(el.textContent).not.toContain('Sem um mapa atual');
    expect(el.textContent).toContain('Não deu para ler o mapa atual');
    expect(button(el).disabled).toBe(true);
    button(el).click();
    expect(opened).toHaveLength(0);
  });

  it('reads the map again with "Tentar de novo" and then starts on it', async () => {
    const { fixture, settle } = await setup();
    const host = fixture.componentInstance;
    const el = fixture.nativeElement as HTMLElement;
    const read = host.state.open('map-1');
    host.fail(new ConnectError('blip', Code.Unavailable));
    await read;
    await settle();
    Array.from(el.querySelectorAll('button'))
      .find((b) => b.textContent?.includes('Tentar de novo'))!
      .click();
    host.answer(
      mapResponse(mapMessage('map-1', 'Estrada do Vale', { gridColumns: 20, gridRows: 14 })),
    );
    await settle();
    expect(button(el).disabled).toBe(false);
    button(el).click();
    expect(opened[0].data.map?.id).toBe('map-1');
  });
});
