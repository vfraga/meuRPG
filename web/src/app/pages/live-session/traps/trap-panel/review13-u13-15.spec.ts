// Finding U13-15 (review/unit-13-web-live-rest.md): aria-disabled (disabledInteractive) trap buttons still emit, and reveal()/fire() do not re-check busy or "everyone sees".
import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { create } from '@bufbuild/protobuf';
import { of } from 'rxjs';

import { MapPointKind, MapPointSchema, TrapState } from '../../../../../gen/meurpg/maps/v1/maps_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { MapState } from '../../../../core/maps/map-state';
import { MapsClient } from '../../../../core/maps/maps-client';
import { TrapBoard } from '../../../../core/traps/trap-board';
import { TrapsClient } from '../../../../core/traps/traps-client';
import { TrapPanel } from './trap-panel';

describe('Review13 U13-15: disabled trap buttons must not open sheets', () => {
  function setup(revealed: boolean) {
    const opened: string[] = [];
    const sheet = {
      open: (c: { name: string }) => (opened.push(c.name), { afterClosed: () => of(undefined) }),
    };
    let finishDisarm!: () => void;
    const pending = new Promise<void>((r) => (finishDisarm = r));
    const maps = {
      getTrapNoticers: async () => ({ noticeDc: 15, noticers: [] }),
      disarmTrap: async () => {
        await pending;
        return create(MapPointSchema, { id: 'b', kind: MapPointKind.TRAP, name: 'Fosso' });
      },
    };
    const traps = {
      activity: async () => ({ activity: [] }),
      damages: async () => ({ damages: [] }),
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: MapsClient, useValue: maps },
        { provide: TrapsClient, useValue: traps },
        { provide: CombatClient, useValue: {} },
        { provide: MatDialog, useValue: sheet },
        { provide: MatBottomSheet, useValue: sheet },
      ],
    });
    const state = new MapState(async () => ({}) as never);
    state.map.set({ id: 'm' } as never);
    state.points.set([
      create(MapPointSchema, {
        id: 'b',
        kind: MapPointKind.TRAP,
        name: 'Fosso',
        revealed,
        trap: { state: TrapState.ARMED, noticeDc: 15, findDc: 15, areaSize: 1 },
      }),
    ]);
    const board = new TrapBoard(
      traps as never,
      maps as never,
      () => 'c',
      () => 'm',
      () => true,
    );
    const fixture = TestBed.createComponent(TrapPanel);
    fixture.componentRef.setInput('campaignId', 'c');
    fixture.componentRef.setInput('state', state);
    fixture.componentRef.setInput('board', board);
    fixture.detectChanges();
    const button = (text: string) =>
      Array.from((fixture.nativeElement as HTMLElement).querySelectorAll('button')).find((b) =>
        b.textContent?.includes(text),
      )!;
    return { fixture, opened, button, finishDisarm };
  }

  it('control: an enabled "Revelar para…" opens the reveal sheet', async () => {
    const { fixture, opened, button } = setup(false);
    await fixture.whenStable();
    fixture.detectChanges();
    expect(button('Revelar para').getAttribute('aria-disabled')).not.toBe('true');
    button('Revelar para').click();
    expect(opened).toHaveLength(1);
  });

  it('does not open the reveal sheet when everyone already sees the trap', async () => {
    const { fixture, opened, button } = setup(true);
    await fixture.whenStable();
    fixture.detectChanges();
    expect(button('Revelar para').getAttribute('aria-disabled')).toBe('true');
    button('Revelar para').click();
    expect(opened).toEqual([]);
  });

  it('does not open the fire sheet while the disarm call is pending', async () => {
    const { fixture, opened, button, finishDisarm } = setup(false);
    await fixture.whenStable();
    fixture.detectChanges();
    button('Desarmar').click();
    fixture.detectChanges();
    expect(button('Disparar').getAttribute('aria-disabled')).toBe('true');
    button('Disparar').click();
    expect(opened).toEqual([]);
    finishDisarm();
    await fixture.whenStable();
  });
});
