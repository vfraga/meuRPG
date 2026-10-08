import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { of } from 'rxjs';
import { create } from '@bufbuild/protobuf';

import { MapPointKind, MapPointSchema, TrapState } from '../../../../../gen/meurpg/maps/v1/maps_pb';
import { MapsClient } from '../../../../core/maps/maps-client';
import { MapState } from '../../../../core/maps/map-state';
import { TrapBoard } from '../../../../core/traps/trap-board';
import { CombatClient } from '../../../../core/combat/combat-client';
import { TrapsClient } from '../../../../core/traps/traps-client';
import { TrapPanel } from './trap-panel';

const trap = (id: string, name: string, state: TrapState) =>
  create(MapPointSchema, {
    id,
    kind: MapPointKind.TRAP,
    name,
    trap: { state, noticeDc: 15, findDc: 15, areaSize: 1 },
  });

describe('TrapPanel', () => {
  function setup(
    points: ReturnType<typeof trap>[],
    answer: { revealed?: ReturnType<typeof trap>; refuseDisarm?: boolean } = {},
  ) {
    const disarmed: string[] = [];
    const maps = {
      getTrapNoticers: async () => ({ noticeDc: 15, noticers: [] }),
      disarmTrap: async (_c: string, _m: string, id: string) => {
        if (answer.refuseDisarm) {
          throw new Error('refused');
        }
        disarmed.push(id);
        return trap(id, 'Fosso escondido', TrapState.DISARMED);
      },
    };
    const traps = {
      activity: async () => ({ activity: [] }),
      damages: async () => ({ damages: [] }),
    };
    const sheet = { open: () => ({ afterClosed: () => of(answer.revealed) }) };
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
    state.points.set(points);
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
    return { fixture, el: fixture.nativeElement as HTMLElement, disarmed, state, signal, answer };
  }

  it('draws nothing while the map has no trap and no damage waits', () => {
    expect(setup([]).el.querySelector('section')).toBeNull();
  });

  it('opens the first armed card and counts the armed ones', async () => {
    const { fixture, el } = setup([
      trap('a', 'Antiga', TrapState.DISARMED),
      trap('b', 'Fosso escondido', TrapState.ARMED),
      trap('c', 'Agulha', TrapState.ARMED),
    ]);
    await fixture.whenStable();
    fixture.detectChanges();
    expect(el.textContent).toContain('2 armadas');
    const open = Array.from(el.querySelectorAll('app-trap-card')).filter((c) =>
      c.textContent?.includes('Esconder detalhes'),
    );
    expect(open).toHaveLength(1);
    expect(open[0].textContent).toContain('Fosso escondido');
  });

  it('disarms through the server and shows the point as it came back', async () => {
    const { fixture, el, disarmed, state } = setup([trap('b', 'Fosso escondido', TrapState.ARMED)]);
    await fixture.whenStable();
    fixture.detectChanges();
    Array.from(el.querySelectorAll('button'))
      .find((b) => b.textContent?.includes('Desarmar'))!
      .click();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(disarmed).toEqual(['b']);
    expect(state.points()[0].trap?.state).toBe(TrapState.DISARMED);
    expect(el.textContent).toContain('Desarmada');
  });

  it("drops the failed disarm's alert once a later action on a trap works", async () => {
    const { fixture, el, answer } = setup([trap('b', 'Fosso escondido', TrapState.ARMED)], {
      refuseDisarm: true,
    });
    await fixture.whenStable();
    fixture.detectChanges();
    const button = (text: string) =>
      Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.includes(text))!;
    button('Desarmar').click();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(el.querySelector('[role="alert"]')).not.toBeNull();
    answer.revealed = trap('b', 'Fosso escondido', TrapState.ARMED);
    button('Revelar para').click();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(el.querySelector('[role="alert"]')).toBeNull();
  });
});
