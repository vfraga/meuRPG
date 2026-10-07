import { Injectable, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { BehaviorSubject } from 'rxjs';

import { AuthService } from '../../core/auth/auth.service';
import { LightPresets } from '../../core/maps/light-presets';
import { MapsClient } from '../../core/maps/maps-client';
import { FamiliarEyesClient } from '../../core/play/familiar-eyes';
import { RosterClient } from '../../core/maps/roster-client';
import { ProgressionClient } from '../../core/progression/progression-client';
import { PuzzlesClient } from '../../core/puzzles/puzzles-client';
import { SceneChecks } from '../../core/maps/scene-actions';
import { FakePuzzlesClient, fakeChecks } from '../../core/puzzles/puzzles-testing';
import { FakeMapsClient } from '../../core/maps/maps-testing';
import { SceneClient } from '../../core/play/scene-client';
import { SessionSummaryClient } from '../../core/play/session-summary';
import { FakeSceneClient } from '../../core/play/scene-testing';
import { OpenSessions } from '../../shell/live-notice/open-sessions';
import { LiveSession } from './live-session';
import {
  CampaignInfoVm,
  LiveErrorKind,
  LiveEventVm,
  LiveSessionSource,
  LiveSnapshotVm,
  PartyMemberInfoVm,
  PlayerSheetVm,
} from './live-session.types';

class KindError extends Error {
  constructor(readonly kind: LiveErrorKind) {
    super(kind);
  }
}

/** A source whose stream always answers NO_OPEN_SESSION, counting every call. */
@Injectable()
class NoSessionSource implements LiveSessionSource {
  readonly watchCalls = vi.fn();
  readonly getCampaignCalls = vi.fn();
  readonly endSession = vi.fn();
  readonly adjustVitals = vi.fn();
  readonly setCurrentMap = vi.fn();
  readonly setShownImage = vi.fn();
  readonly listLeftImages = vi.fn(() => Promise.resolve([]));
  readonly takeBackLeftImage = vi.fn();

  getCampaign(): Promise<CampaignInfoVm> {
    this.getCampaignCalls();
    return Promise.resolve({
      name: 'Mirathel',
      isMaster: false,
      awaitingApproval: false,
      diceMode: 1,
      dicePreference: 1,
    });
  }
  // eslint-disable-next-line require-yield
  async *watch(_campaignId: string, _signal: AbortSignal): AsyncIterable<LiveEventVm> {
    this.watchCalls();
    if (this.watchCalls.mock.calls.length > 200) {
      // Safety valve so a runaway loop ends the test with a count, not a hang.
      await new Promise<never>(() => undefined);
    }
    throw new KindError('no-session');
  }
  getLiveSession(): Promise<LiveSnapshotVm> {
    return Promise.reject(new KindError('no-session'));
  }
  getPlayerSheet(): Promise<PlayerSheetVm> {
    return Promise.reject(new Error('unused'));
  }
  getCreatureArmorClass(): Promise<number | null> {
    return Promise.resolve(null);
  }
  getPartyInfo(): Promise<ReadonlyMap<string, PartyMemberInfoVm>> {
    return Promise.resolve(new Map());
  }
  classifyError(err: unknown): LiveErrorKind {
    return err instanceof KindError ? err.kind : 'transient';
  }
}

// Finding U10-4, see review/unit-10-web-core-stream.md
describe('Review10 U10-4: no-session page reloads in a loop on a stale open-sessions list', () => {
  const refresh = vi.fn(() => Promise.resolve());
  const openSessions = {
    // Stale: still lists the campaign although the server says no session is open.
    liveCampaignIds: signal<ReadonlySet<string>>(new Set(['mirathel'])).asReadonly(),
    refresh,
    dismiss: vi.fn(),
  };

  beforeEach(() => {
    refresh.mockClear();
    TestBed.configureTestingModule({
      imports: [LiveSession],
      providers: [
        provideRouter([]),
        { provide: LiveSessionSource, useClass: NoSessionSource },
        { provide: MapsClient, useClass: FakeMapsClient },
        { provide: LightPresets, useValue: { list: () => Promise.resolve([]) } },
        {
          provide: FamiliarEyesClient,
          useValue: { name: () => Promise.resolve('x'), start: vi.fn(), stop: vi.fn() },
        },
        {
          provide: ProgressionClient,
          useValue: { experience: vi.fn(), listAwards: vi.fn() },
        },
        { provide: RosterClient, useValue: { list: () => Promise.resolve([]) } },
        { provide: SceneClient, useValue: new FakeSceneClient() },
        { provide: PuzzlesClient, useValue: new FakePuzzlesClient() },
        { provide: SceneChecks, useValue: fakeChecks },
        { provide: SessionSummaryClient, useValue: { get: vi.fn().mockRejectedValue(new Error()) } },
        { provide: AuthService, useValue: { signIn: vi.fn(), state: signal({ status: 'signed-in' }) } },
        { provide: OpenSessions, useValue: openSessions },
        {
          provide: ActivatedRoute,
          useValue: {
            paramMap: new BehaviorSubject(convertToParamMap({ id: 'mirathel' })),
            queryParamMap: new BehaviorSubject(convertToParamMap({})),
          },
        },
      ],
    });
  });

  it('does not call watch/getCampaign again and again while the list stays stale', async () => {
    const source = TestBed.inject(LiveSessionSource) as unknown as NoSessionSource;
    const fixture = TestBed.createComponent(LiveSession);
    fixture.detectChanges();
    for (let i = 0; i < 20; i++) {
      await fixture.whenStable();
      await new Promise((r) => setTimeout(r));
      fixture.detectChanges();
      TestBed.tick();
    }
    const watches = source.watchCalls.mock.calls.length;
    const campaigns = source.getCampaignCalls.mock.calls.length;
    expect(
      { watches, campaigns },
      `watch called ${watches}x, getCampaign ${campaigns}x in 20 rounds`,
    ).toSatisfy((c: { watches: number; campaigns: number }) => c.watches <= 2 && c.campaigns <= 2);
  });
});
