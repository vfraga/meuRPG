// Finding U16-16 in review/unit-16-web-content-campaigns.md
import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { of } from 'rxjs';

import { type BestiaryAccess, BestiaryAccessCheck } from '../../../core/creatures/bestiary-access';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { FakeCreaturesClient } from '../../../core/creatures/creatures-testing';
import { FakeEncountersClient } from '../../../core/encounters/encounters-testing';
import { EncountersClient } from '../../../core/encounters/encounters-client';
import { RosterClient } from '../../../core/maps/roster-client';
import { EncounterBuilder } from './encounter-builder';

describe('Review16 U16-16: removing a party NPC by a stale chip index removes the wrong NPC', () => {
  beforeEach(() => vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] }));
  afterEach(() => vi.useRealTimers());

  it('clicking the first chip x twice before the measure lands removes only the first NPC', async () => {
    const api = new FakeEncountersClient();
    const creatures = new FakeCreaturesClient();
    const results: Record<string, unknown> = {};
    const access = { status: 'master', campaignName: 'C' } as unknown as BestiaryAccess;
    TestBed.configureTestingModule({
      providers: [
        provideRouter([{ path: 'campaigns/:id/encounters', component: EncounterBuilder }]),
        { provide: EncountersClient, useValue: api },
        { provide: CreaturesClient, useValue: creatures },
        { provide: BestiaryAccessCheck, useValue: { check: async () => access } },
        { provide: RosterClient, useValue: { list: async () => [] } },
        {
          provide: MatDialog,
          useValue: {
            open: (_c: unknown, config: { ariaLabelledBy: string }) => ({
              afterClosed: () => of(results[config.ariaLabelledBy]),
            }),
          },
        },
        { provide: MatBottomSheet, useValue: {} },
      ],
    });
    const harness = await RouterTestingHarness.create();
    await harness.navigateByUrl('/campaigns/camp-1/encounters', EncounterBuilder);
    const settle = async (ms = 0) => {
      for (let i = 0; i < 4; i++) {
        harness.detectChanges();
        await harness.fixture.whenStable();
        await vi.advanceTimersByTimeAsync(ms);
      }
      harness.detectChanges();
    };
    await settle(300);
    const el = harness.routeNativeElement as HTMLElement;
    const add = async (id: string, level: number) => {
      results['pn-t'] = { characterId: id, name: '', level, label: id };
      Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
        .find((b) => (b.textContent ?? '').includes('Pôr um NPC no grupo'))!
        .click();
      await settle(300);
    };
    await add('A', 3);
    await add('B', 5);
    await add('C', 7);
    expect(api.evaluateCalls.at(-1)?.party.map((p) => p.characterId)).toEqual(['A', 'B', 'C']);

    const xs = el.querySelectorAll<HTMLButtonElement>('.chip__x');
    expect(xs).toHaveLength(3);
    // Two quick clicks on A's chip, before the new measure lands: chips still show A, B, C.
    xs[0].click();
    xs[0].click();
    await settle(300);
    expect(api.evaluateCalls.at(-1)?.party.map((p) => p.characterId)).toEqual(['B', 'C']);
  });
});
