import { ChangeDetectionStrategy, Component } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { create } from '@bufbuild/protobuf';

import {
  GetMapLayersResponseSchema,
  GetMapResponseSchema,
  MapSchema,
} from '../../../../gen/meurpg/maps/v1/maps_pb';
import { SceneChecks } from '../../../core/maps/scene-actions';
import { RosterClient } from '../../../core/maps/roster-client';
import { MapsClient } from '../../../core/maps/maps-client';
import { PuzzleAccessCheck } from '../../../core/puzzles/puzzle-access';
import { PuzzlesClient } from '../../../core/puzzles/puzzles-client';
import {
  FakePuzzlesClient,
  fakeChecks,
  fakeRoster,
  preview,
} from '../../../core/puzzles/puzzles-testing';
import { PuzzleForm } from './puzzle-form';

@Component({ template: 'campanha', changeDetection: ChangeDetectionStrategy.OnPush })
class Stub {}

describe('PuzzleForm start preview', { timeout: 20_000 }, () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('asks for a new start when the master comes back to lights', async () => {
    const api = new FakePuzzlesClient();
    api.previewResult = preview(
      Array.from({ length: 25 }, (_, i) => i < 17),
      4,
      7n,
    );
    TestBed.configureTestingModule({
      providers: [
        provideRouter([
          { path: 'campaigns/:id/puzzles/new', component: PuzzleForm },
          { path: 'campaigns/:id', component: Stub },
        ]),
        { provide: PuzzlesClient, useValue: api },
        {
          provide: MapsClient,
          useValue: {
            list: async () => [create(MapSchema, { id: 'm1', name: 'A capela' })],
            get: async () => create(GetMapResponseSchema, { points: [] }),
            layers: async () => create(GetMapLayersResponseSchema, {}),
          },
        },
        { provide: SceneChecks, useValue: fakeChecks },
        { provide: RosterClient, useValue: fakeRoster },
        {
          provide: PuzzleAccessCheck,
          useValue: { check: async () => ({ status: 'master', campaignName: 'Mirathel' }) },
        },
      ],
    });
    const harness = await RouterTestingHarness.create();
    await harness.navigateByUrl('/campaigns/camp-1/puzzles/new', PuzzleForm);
    const settle = async () => {
      for (let i = 0; i < 4; i++) {
        harness.detectChanges();
        await harness.fixture.whenStable();
        await vi.advanceTimersByTimeAsync(0);
      }
      harness.detectChanges();
    };
    const pause = async () => {
      await vi.advanceTimersByTimeAsync(300);
      await settle();
    };
    await settle();
    const el = harness.routeNativeElement as HTMLElement;
    await pause();
    const count = () => api.calls.filter((c) => c[0] === 'previewStart').length;
    const first = count();
    expect(first).toBeGreaterThan(0);

    el.querySelector<HTMLInputElement>('input[value="lock"]')!.click();
    await settle();
    el.querySelector<HTMLInputElement>('input[value="lights"]')!.click();
    await settle();
    await pause();

    expect(el.textContent).not.toContain('Sorteando um começo...');
    expect(count()).toBe(first + 1);
  });
});
