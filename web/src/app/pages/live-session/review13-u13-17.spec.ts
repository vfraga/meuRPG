// Finding U13-17 (review/unit-13-web-live-rest.md): the treasure card stays stale after the XP changes (convert to XP or undo), because the page only bumps XpChanges and never re-reads the map.
import { ApplicationRef, Injectable, signal } from '@angular/core';
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
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { XpMode } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CharacterExperienceSchema,
  GetCampaignExperienceResponseSchema,
} from '../../../gen/meurpg/progression/v1/progression_pb';
import { FakeMapsClient, mapMessage, mapPoint, mapResponse } from '../../core/maps/maps-testing';
import { MapPointKind, TreasureFinderSchema } from '../../../gen/meurpg/maps/v1/maps_pb';
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

describe('Review13 U13-17: treasure card after an XP change', () => {
  let source: FakeLiveSessionSource;
  /** What the master's "Dar XP" reads (MR-016): the party's XP and how the campaign levels. */
  const xpExperience = vi.fn();
  let scenes: FakeSceneClient;
  let puzzles: FakePuzzlesClient;
  /** The query string of the page's address: `?puzzle=ID` opens a puzzle for a player. */
  const query = new BehaviorSubject(convertToParamMap({}));
  /** The summary of the ended session (MR-032); by default it cannot be read, so the page shows the plain notice. */
  const summary = vi.fn();
  const signIn = vi.fn();
  const liveCampaignIds = signal<ReadonlySet<string>>(new Set());
  const openSessions = {
    liveCampaignIds: liveCampaignIds.asReadonly(),
    refresh: vi.fn(() => Promise.resolve()),
    dismiss: vi.fn(),
  };

  beforeEach(() => {
    xpExperience.mockReset().mockResolvedValue(
      create(GetCampaignExperienceResponseSchema, {
        xpMode: XpMode.ENEMIES,
        characters: [
          create(CharacterExperienceSchema, {
            characterId: 'pensantus',
            name: 'Pensantus',
            level: 3,
            experiencePoints: 2600,
            nextLevelXp: 2700,
          }),
        ],
      }),
    );
    signIn.mockClear();
    openSessions.dismiss.mockClear();
    liveCampaignIds.set(new Set());
    summary.mockReset().mockRejectedValue(new Error('no summary'));
    scenes = new FakeSceneClient();
    puzzles = new FakePuzzlesClient();
    query.next(convertToParamMap({}));
    TestBed.configureTestingModule({
      imports: [LiveSession],
      providers: [
        provideRouter([]),
        { provide: LiveSessionSource, useClass: FakeLiveSessionSource },
        { provide: MapsClient, useClass: FakeMapsClient },
        {
          provide: LightPresets,
          useValue: {
            list: () =>
              Promise.resolve([
                { key: 'light:torch', name: 'Tocha', radii: '6 m claro + 6 m de penumbra' },
              ]),
          },
        },
        {
          provide: FamiliarEyesClient,
          useValue: { name: () => Promise.resolve('Nanquim'), start: vi.fn(), stop: vi.fn() },
        },
        { provide: ProgressionClient, useValue: { experience: xpExperience, listAwards: vi.fn() } },
        { provide: RosterClient, useValue: { list: () => Promise.resolve([]) } },
        { provide: SceneClient, useValue: scenes },
        { provide: PuzzlesClient, useValue: puzzles },
        { provide: SceneChecks, useValue: fakeChecks },
        { provide: SessionSummaryClient, useValue: { get: summary } },
        { provide: AuthService, useValue: { signIn, state: signal({ status: 'signed-in' }) } },
        { provide: OpenSessions, useValue: openSessions },
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

  const treasure = (converted: boolean) =>
    mapPoint('t1', 'Baú de moedas', {
      kind: MapPointKind.TREASURE,
      treasureValuePo: 250,
      treasureFoundAt: timestampFromDate(new Date(2026, 9, 4, 21, 40)),
      treasureFoundBy: [
        create(TreasureFinderSchema, { characterId: 'brisa', characterName: 'Brisa' }),
      ],
      treasureConverted: converted,
    });

  async function setup() {
    source.campaign = {
      name: 'Mirathel',
      isMaster: true,
      awaitingApproval: false,
      diceMode: 1,
      dicePreference: 1,
    };
    source.snapshot = { ...(source.snapshot as LiveSnapshotVm), currentMapId: 'map-1' };
    const maps = TestBed.inject(MapsClient) as unknown as FakeMapsClient;
    const map = mapMessage('map-1', 'Cripta', { revealed: true, current: true });
    maps.maps = [map];
    maps.responses.set('map-1', mapResponse(map, [treasure(false)]));
    const el = await settle(TestBed.createComponent(LiveSession));
    return { el, maps, map };
  }

  const card = (el: HTMLElement) => el.querySelector('app-treasure-card')?.textContent ?? '';

  it('control: a found treasure shows "Desmarcar" and not the lock', async () => {
    const { el } = await setup();
    expect(card(el)).toContain('Desmarcar');
    expect(card(el)).not.toContain('Convertido em XP');
  });

  it('shows the treasure as converted after xp_changed when the server now has it converted', async () => {
    const { el, maps, map } = await setup();
    maps.responses.set('map-1', mapResponse(map, [treasure(true)]));
    source.push({ kind: 'xpChanged' });
    await new Promise((r) => setTimeout(r));
    await new Promise((r) => setTimeout(r));
    TestBed.inject(ApplicationRef).tick();
    expect(card(el)).toContain('Convertido em XP');
    expect(card(el)).not.toContain('Desmarcar');
  });
});
