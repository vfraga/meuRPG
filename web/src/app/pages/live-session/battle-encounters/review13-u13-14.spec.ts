// Finding U13-14 (review/unit-13-web-live-rest.md): begin(k) has no unknown-creature guard, so the disabledInteractive button still opens the dialog.
import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MatDialog } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';
import { of } from 'rxjs';

import { MapPointKind } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { isOff } from '../../../core/creatures/creatures-testing';
import { EncountersClient } from '../../../core/encounters/encounters-client';
import {
  FakeEncountersClient,
  artboardEncounter,
} from '../../../core/encounters/encounters-testing';
import { MapState } from '../../../core/maps/map-state';
import { mapMessage, mapPoint } from '../../../core/maps/maps-testing';
import { BattleEncounters } from './battle-encounters';

@Component({
  imports: [BattleEncounters],
  template:
    '<app-battle-encounters campaignId="camp-1" [state]="state" (started)="started.set($event.name)" />',
})
class Host {
  state = new MapState(async () => {
    throw new Error('not read');
  });
  started = signal('');
}

describe('Review13 U13-14: a lost-creature encounter cannot be started by clicking the dashed button', () => {
  async function setup(unknownKeys: string[]) {
    const api = new FakeEncountersClient();
    const art = artboardEncounter();
    api.kept = [{ mapPointId: 'pt-1', creatureCount: 13 }];
    api.battle.set('pt-1', { encounter: art.encounter, evaluation: art.evaluation, unknownKeys });
    const open = vi.fn(() => ({ afterClosed: () => of(undefined) }));
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: EncountersClient, useValue: api },
        { provide: MatDialog, useValue: { open } },
      ],
    });
    const fixture = TestBed.createComponent(Host);
    const state = fixture.componentInstance.state;
    state.map.set(mapMessage('map-1', 'Estrada do Vale', { gridColumns: 24, gridRows: 16 }));
    state.points.set([mapPoint('pt-1', 'Emboscada na ponte', { kind: MapPointKind.BATTLE })]);
    for (let i = 0; i < 5; i++) {
      fixture.detectChanges();
      await fixture.whenStable();
    }
    return { el: fixture.nativeElement as HTMLElement, open };
  }

  it('control: a known encounter opens the dialog on click', async () => {
    const { el, open } = await setup([]);
    el.querySelector<HTMLButtonElement>('.enc__go')!.click();
    expect(open).toHaveBeenCalledTimes(1);
  });

  it('does not open "Iniciar combate" when a creature left the SRD', async () => {
    const { el, open } = await setup(['monster:gone']);
    const go = el.querySelector<HTMLButtonElement>('.enc__go')!;
    expect(isOff(go)).toBe(true);
    go.click();
    expect(open).not.toHaveBeenCalled();
  });
});
