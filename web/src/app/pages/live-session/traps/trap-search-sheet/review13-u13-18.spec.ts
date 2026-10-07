// Finding U13-18 (review/unit-13-web-live-rest.md): a found trap is reported as "Você não encontrou nada." when the map refresh fails or the point is missing.
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { create } from '@bufbuild/protobuf';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { MapPointKind, MapPointSchema } from '../../../../../gen/meurpg/maps/v1/maps_pb';
import { SearchForTrapsResponseSchema } from '../../../../../gen/meurpg/play/v1/traps_pb';
import { TrapsClient } from '../../../../core/traps/traps-client';
import { TrapSearchSheet, type TrapSearchData } from './trap-search-sheet';

const roll = { diceCount: 1, diceSides: 20, faces: [13], modifier: 4, total: 17 };

describe('Review13 U13-18: found trap reported as nothing when the map refresh gives no point', () => {
  async function search(points: ReturnType<typeof create<typeof MapPointSchema>>[]) {
    const api = {
      search: async () =>
        create(SearchForTrapsResponseSchema, { roll, spentAction: true, foundPointIds: ['p1'] }),
    };
    const data: TrapSearchData = {
      campaignId: 'c',
      skills: { perception: 4, investigation: 4 },
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
      // MapState.refresh swallows a failed GetMap and keeps what is there: it resolves with the old points.
      state: {
        refresh: async () => undefined,
        points: () => points,
      } as unknown as TrapSearchData['state'],
      inCombat: true,
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: TrapsClient, useValue: api },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: vi.fn() } },
      ],
    });
    const fixture = TestBed.createComponent(TrapSearchSheet);
    fixture.detectChanges();
    await (
      fixture.componentInstance as unknown as { rollWith(d: unknown): Promise<void> }
    ).rollWith({ inApp: true });
    fixture.detectChanges();
    return (fixture.nativeElement as HTMLElement).textContent!;
  }

  it('control: with the point in the map, the trap is named', async () => {
    const text = await search([
      create(MapPointSchema, { id: 'p1', kind: MapPointKind.TRAP, name: 'Fosso' }),
    ]);
    expect(text).toContain('Você achou uma armadilha: Fosso.');
  });

  it('does not tell the player "nada" when the server said a trap was found', async () => {
    const text = await search([]);
    expect(text).not.toContain('Você não encontrou nada.');
  });
});
