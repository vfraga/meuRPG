import { ApplicationRef, Injectable, signal } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { BehaviorSubject } from 'rxjs';

import { AuthService } from '../../core/auth/auth.service';
import { LightPresets } from '../../core/maps/light-presets';
import { MapsClient } from '../../core/maps/maps-client';
import { visionResponse } from '../../core/maps/vision-testing';
import { FamiliarEyesClient } from '../../core/play/familiar-eyes';
import { RosterClient } from '../../core/maps/roster-client';
import { ProgressionClient } from '../../core/progression/progression-client';
import { PuzzlesClient } from '../../core/puzzles/puzzles-client';
import { SceneChecks } from '../../core/maps/scene-actions';
import { FakePuzzlesClient, fakeChecks } from '../../core/puzzles/puzzles-testing';
import {
  FakeMapsClient,
  mapMessage,
  mapResponse,
  mapToken,
} from '../../core/maps/maps-testing';
import { SceneClient } from '../../core/play/scene-client';
import { SessionSummaryClient } from '../../core/play/session-summary';
import {
  FakeSceneClient,
} from '../../core/play/scene-testing';

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
  ShownImageVm,
} from './live-session.types';
import { pensantusVitals } from './testing';

class KindError extends Error {
  constructor(readonly kind: LiveErrorKind) {
    super(kind);
  }
}

/** A `LiveSessionSource` whose stream the test feeds by hand. */
@Injectable()
class FakeLiveSessionSource implements LiveSessionSource {
  campaign: CampaignInfoVm | Error = {
    name: 'Mirathel',
    isMaster: false,
    awaitingApproval: false,
    diceMode: 1,
    dicePreference: 1,
  };
  snapshot: LiveSnapshotVm | Error = {
    session: { sessionId: 's4', sessionNumber: 4, startedAt: new Date(2026, 8, 30, 20, 5) },
    vitals: [pensantusVitals()],
    currentMapId: null,
    shownImage: null,
    shownImageKeep: false,
  };
  sheet: PlayerSheetVm = {
    armorClass: 14,
    summary: 'Mago 3, Gnomo das Rochas',
    senses: ['Visão no escuro: 18 m'],
  };
  party = new Map<string, PartyMemberInfoVm>([
    ['pensantus', { classSummary: 'Mago 3', playerName: 'Vinicius' }],
    ['brisa', { classSummary: 'Ladina 3', playerName: 'Ana' }],
  ]);
  /** What the next `watch` call does: yields these, then fails or waits. */
  events: LiveEventVm[] = [{ kind: 'ready' }];
  failWith: Error | null = null;
  readonly endSession = vi.fn(() => Promise.resolve());
  readonly adjustVitals = vi.fn();
  readonly setCurrentMap = vi.fn((_c: string, mapId: string | null) => Promise.resolve(mapId));
  readonly setShownImage = vi.fn();
  /** The images left with the players, as the server lists them. */
  left: ShownImageVm[] = [];
  readonly listLeftImages = vi.fn(() => Promise.resolve(this.left));
  readonly takeBackLeftImage = vi.fn((_c: string, id: string) => {
    this.left = this.left.filter((i) => i.id !== id);
    return Promise.resolve();
  });

  getCampaign(): Promise<CampaignInfoVm> {
    return this.campaign instanceof Error
      ? Promise.reject(this.campaign)
      : Promise.resolve(this.campaign);
  }

  private later: ((e: LiveEventVm) => void) | null = null;

  /** Sends one more event on the open stream. */
  push(event: LiveEventVm): void {
    this.later?.(event);
  }

  async *watch(_campaignId: string, signal: AbortSignal): AsyncIterable<LiveEventVm> {
    for (const e of this.events) {
      yield e;
    }
    if (this.failWith) {
      throw this.failWith;
    }
    // Then stay open, taking `push`ed events, until the page closes it.
    for (;;) {
      const next = await new Promise<LiveEventVm>((resolve, reject) => {
        this.later = resolve;
        signal.addEventListener('abort', () => reject(new Error('aborted')));
      });
      yield next;
    }
  }

  getLiveSession(): Promise<LiveSnapshotVm> {
    return this.snapshot instanceof Error
      ? Promise.reject(this.snapshot)
      : Promise.resolve(this.snapshot);
  }

  getPlayerSheet(): Promise<PlayerSheetVm> {
    return Promise.resolve(this.sheet);
  }

  getCreatureArmorClass(): Promise<number | null> {
    return Promise.resolve(13);
  }
  getPartyInfo(): Promise<ReadonlyMap<string, PartyMemberInfoVm>> {
    return Promise.resolve(this.party);
  }

  classifyError(err: unknown): LiveErrorKind {
    return err instanceof KindError ? err.kind : 'transient';
  }
}

// Finding U10-6, see review/unit-10-web-core-stream.md
describe('Review10 U10-6: a reconnection does not re-read the fog vision', () => {
  let source: FakeLiveSessionSource;
  let maps: FakeMapsClient;
  const query = new BehaviorSubject(convertToParamMap({}));

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [LiveSession],
      providers: [
        provideRouter([]),
        { provide: LiveSessionSource, useClass: FakeLiveSessionSource },
        { provide: MapsClient, useClass: FakeMapsClient },
        { provide: LightPresets, useValue: { list: () => Promise.resolve([]) } },
        {
          provide: FamiliarEyesClient,
          useValue: { name: () => Promise.resolve('Nanquim'), start: vi.fn(), stop: vi.fn() },
        },
        { provide: ProgressionClient, useValue: { experience: vi.fn(), listAwards: vi.fn() } },
        { provide: RosterClient, useValue: { list: () => Promise.resolve([]) } },
        { provide: SceneClient, useValue: new FakeSceneClient() },
        { provide: PuzzlesClient, useValue: new FakePuzzlesClient() },
        { provide: SceneChecks, useValue: fakeChecks },
        { provide: SessionSummaryClient, useValue: { get: vi.fn().mockRejectedValue(new Error('x')) } },
        { provide: AuthService, useValue: { signIn: vi.fn(), state: signal({ status: 'signed-in' }) } },
        {
          provide: OpenSessions,
          useValue: {
            liveCampaignIds: signal<ReadonlySet<string>>(new Set()).asReadonly(),
            refresh: vi.fn(() => Promise.resolve()),
            dismiss: vi.fn(),
          },
        },
        {
          provide: ActivatedRoute,
          useValue: {
            paramMap: new BehaviorSubject(convertToParamMap({ id: 'mirathel' })),
            queryParamMap: query,
          },
        },
      ],
    });
    source = TestBed.inject(LiveSessionSource) as unknown as FakeLiveSessionSource;
    maps = TestBed.inject(MapsClient) as unknown as FakeMapsClient;
    const map = mapMessage('map-1', 'A caverna do Vale Seco', {
      revealed: true,
      current: true,
      fogEnabled: true,
      gridColumns: 4,
      gridRows: 4,
    });
    maps.maps = [map];
    maps.responses.set(
      'map-1',
      mapResponse(map, [], [mapToken('pensantus', 'Pensantus', { mine: true, xBp: 1250, yBp: 6250 })]),
    );
    maps.visions.set('', visionResponse(['....', 'gBd.', '..r.', '....']));
    source.snapshot = { ...(source.snapshot as LiveSnapshotVm), currentMapId: 'map-1' };
  });

  async function settle(fixture: ComponentFixture<LiveSession>): Promise<void> {
    fixture.detectChanges();
    for (let i = 0; i < 4; i++) {
      await fixture.whenStable();
      await new Promise((r) => setTimeout(r));
      fixture.detectChanges();
    }
    TestBed.inject(ApplicationRef).tick();
  }

  it('reads the vision again when the stream says ready a second time', async () => {
    const fixture = TestBed.createComponent(LiveSession);
    await settle(fixture);
    await new Promise((r) => setTimeout(r, 300));
    const visionCalls = () => maps.calls.filter((c) => c.startsWith('vision map-1')).length;
    const layerCalls = () => maps.calls.filter((c) => c.startsWith('layers map-1')).length;
    const getCalls = () => maps.calls.filter((c) => c.startsWith('get map-1')).length;
    expect(visionCalls()).toBeGreaterThan(0);
    const before = { vision: visionCalls(), layers: layerCalls(), gets: getCalls() };

    // What a reconnection delivers: a new `ready` on the same stream session.
    source.push({ kind: 'ready' });
    await settle(fixture);
    await new Promise((r) => setTimeout(r, 400));
    await settle(fixture);

    // The map itself is read again (the page's own comment promises it)...
    expect(getCalls()).toBe(before.gets + 1);
    // ...but a missed vision_changed would leave the fog stale unless the vision is read too.
    expect(visionCalls()).toBeGreaterThan(before.vision);
    expect(layerCalls()).toBeGreaterThan(before.layers);
  });
});
