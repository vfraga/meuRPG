import { Injectable, signal } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
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
import { FakeMapsClient, mapMessage, mapResponse } from '../../core/maps/maps-testing';
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
  ShownImageVm,
} from './live-session.types';
import { pensantusVitals } from './testing';

/** A `LiveSessionSource` whose stream the test feeds by hand and whose snapshot answers when the test says. */
@Injectable()
class DeferredSource implements LiveSessionSource {
  snapshot: LiveSnapshotVm = {
    session: { sessionId: 's4', sessionNumber: 4, startedAt: new Date(2026, 8, 30, 20, 5) },
    vitals: [pensantusVitals()],
    currentMapId: null,
    shownImage: null,
    shownImageKeep: false,
  };
  private answer: (() => void) | null = null;
  /** Answers the snapshot request that is in flight, with `snapshot` as it stands now. */
  resolveSnapshot(): void {
    this.answer?.();
  }
  readonly endSession = vi.fn(() => Promise.resolve());
  readonly adjustVitals = vi.fn();
  readonly setCurrentMap = vi.fn((_c: string, mapId: string | null) => Promise.resolve(mapId));
  readonly setShownImage = vi.fn();
  readonly listLeftImages = vi.fn(() => Promise.resolve([] as ShownImageVm[]));
  readonly takeBackLeftImage = vi.fn(() => Promise.resolve());

  getCampaign(): Promise<CampaignInfoVm> {
    return Promise.resolve({
      name: 'Mirathel',
      isMaster: false,
      awaitingApproval: false,
      diceMode: 1,
      dicePreference: 1,
    });
  }

  private later: ((e: LiveEventVm) => void) | null = null;
  push(event: LiveEventVm): void {
    this.later?.(event);
  }

  async *watch(_campaignId: string, signal: AbortSignal): AsyncIterable<LiveEventVm> {
    yield { kind: 'ready' };
    for (;;) {
      const next = await new Promise<LiveEventVm>((resolve, reject) => {
        this.later = resolve;
        signal.addEventListener('abort', () => reject(new Error('aborted')));
      });
      yield next;
    }
  }

  getLiveSession(): Promise<LiveSnapshotVm> {
    return new Promise((resolve) => {
      this.answer = () => resolve(this.snapshot);
    });
  }

  getPlayerSheet(): Promise<PlayerSheetVm> {
    return Promise.resolve({ armorClass: 14, summary: 'Mago 3', senses: [] });
  }
  getCreatureArmorClass(): Promise<number | null> {
    return Promise.resolve(13);
  }
  getPartyInfo(): Promise<ReadonlyMap<string, PartyMemberInfoVm>> {
    return Promise.resolve(new Map());
  }
  classifyError(): LiveErrorKind {
    return 'transient';
  }
}

// Finding U10-2, see review/unit-10-web-core-stream.md
describe('Review10 U10-2: an older snapshot answer overwrites newer stream events', () => {
  let source: DeferredSource;
  let maps: FakeMapsClient;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [LiveSession],
      providers: [
        provideRouter([]),
        { provide: LiveSessionSource, useClass: DeferredSource },
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
        { provide: SessionSummaryClient, useValue: { get: vi.fn() } },
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
            queryParamMap: new BehaviorSubject(convertToParamMap({})),
          },
        },
      ],
    });
    source = TestBed.inject(LiveSessionSource) as unknown as DeferredSource;
    maps = TestBed.inject(MapsClient) as unknown as FakeMapsClient;
  });

  async function settle(fixture: ComponentFixture<LiveSession>): Promise<HTMLElement> {
    fixture.detectChanges();
    for (let i = 0; i < 4; i++) {
      await fixture.whenStable();
      await new Promise((r) => setTimeout(r));
      fixture.detectChanges();
    }
    return fixture.nativeElement as HTMLElement;
  }

  // The component's state is protected; the test reads it to state what the player ends up with.
  const state = (fixture: ComponentFixture<LiveSession>) =>
    fixture.componentInstance as unknown as {
      shownImage(): ShownImageVm | null;
      currentMapId(): string | null;
    };

  it('does not show again an image the master withdrew while the snapshot was in flight', async () => {
    const image: ShownImageVm = {
      id: 'img-1',
      name: 'Capitão Goblin',
      width: 400,
      height: 500,
      url: '/images/img-1',
    };
    // The snapshot request was made while the image was still on show...
    source.snapshot = { ...source.snapshot, shownImage: image };
    const fixture = TestBed.createComponent(LiveSession);
    await settle(fixture); // `ready` arrived, the snapshot request is in flight (unanswered)

    // ...the master takes the image away; the stream says so before the answer comes.
    source.push({ kind: 'shownImage', image: null });
    await settle(fixture);
    // The older answer arrives.
    source.resolveSnapshot();
    const el = await settle(fixture);

    expect(state(fixture).shownImage()).toBeNull();
    expect(el.querySelector('#shown-title')).toBeNull();
    expect(el.querySelector('.block__name')).toBeNull();
  });

  it('stays on the map the master moved the table to while the snapshot was in flight', async () => {
    const a = mapMessage('map-a', 'Mapa A', { revealed: true, current: true });
    const b = mapMessage('map-b', 'Mapa B', { revealed: true, current: true });
    maps.maps = [a, b];
    maps.responses.set('map-a', mapResponse(a));
    maps.responses.set('map-b', mapResponse(b));
    source.snapshot = { ...source.snapshot, currentMapId: 'map-a' };

    const fixture = TestBed.createComponent(LiveSession);
    await settle(fixture); // snapshot in flight, still saying map A

    source.push({ kind: 'currentMap', mapId: 'map-b' });
    await settle(fixture);
    source.resolveSnapshot();
    const el = await settle(fixture);

    expect(state(fixture).currentMapId()).toBe('map-b');
    expect(el.querySelector('#session-map-heading')?.textContent).toContain('Mapa B');
  });
});
