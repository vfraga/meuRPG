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
import { LiveEventVm, LiveSessionSource, LiveSnapshotVm } from './live-session.types';
import { pensantusVitals } from './testing';

/** A source whose stream the test feeds by hand; every snapshot read is counted. */
@Injectable()
class FakeSource {
  snapshotReads = 0;
  snapshot: LiveSnapshotVm = {
    session: { sessionId: 's4', sessionNumber: 4, startedAt: new Date(2026, 8, 30, 20, 5) },
    vitals: [pensantusVitals({ revision: 5, familiarSight: { creatureId: 'nanquim', inCombat: true } })],
    currentMapId: null,
    shownImage: null,
    shownImageKeep: false,
  };
  private later: ((e: LiveEventVm) => void) | null = null;
  push(e: LiveEventVm): void {
    this.later?.(e);
  }
  getCampaign() {
    return Promise.resolve({
      name: 'Mirathel',
      isMaster: false,
      awaitingApproval: false,
      diceMode: 1,
      dicePreference: 1,
    });
  }
  async *watch(_c: string, signal: AbortSignal): AsyncIterable<LiveEventVm> {
    yield { kind: 'ready' };
    for (;;) {
      yield await new Promise<LiveEventVm>((resolve, reject) => {
        this.later = resolve;
        signal.addEventListener('abort', () => reject(new Error('aborted')));
      });
    }
  }
  getLiveSession() {
    this.snapshotReads++;
    return Promise.resolve(this.snapshot);
  }
  getPlayerSheet() {
    return Promise.resolve({ armorClass: 14, summary: 'Mago 3', senses: [] });
  }
  getCreatureArmorClass() {
    return Promise.resolve(13);
  }
  getPartyInfo() {
    return Promise.resolve(new Map());
  }
  classifyError() {
    return 'transient';
  }
}

describe('Review17 U17-7: a dismissed familiar leaves the page in familiar sight', () => {
  // Finding U17-7 (review/unit-17-contract.md)
  it('goes back to the character\'s own eyes when the familiar is dismissed in a combat', async () => {
    TestBed.configureTestingModule({
      imports: [LiveSession],
      providers: [
        provideRouter([]),
        { provide: LiveSessionSource, useClass: FakeSource },
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
          useValue: { liveCampaignIds: signal(new Set<string>()).asReadonly(), refresh: vi.fn(() => Promise.resolve()), dismiss: vi.fn() },
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
    const source = TestBed.inject(LiveSessionSource) as unknown as FakeSource;
    const fixture = TestBed.createComponent(LiveSession);
    const settle = async () => {
      for (let i = 0; i < 4; i++) {
        fixture.detectChanges();
        await fixture.whenStable();
        await new Promise((r) => setTimeout(r));
      }
      fixture.detectChanges();
    };
    await settle();
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelector('[data-testid="familiar-band"]')).not.toBeNull();
    const readsBefore = source.snapshotReads;

    // What the server really does on DismissCreature in a combat: creatures_changed, then
    // every character's vitals republished at the SAME stored revision (liveFamiliar only
    // hides the sight on read; nothing bumps character_vitals.revision), sight now empty.
    source.push({ kind: 'creaturesChanged' });
    await settle();
    source.push({ kind: 'vitals', vitals: pensantusVitals({ revision: 5, familiarSight: null }) });
    await settle();

    // creatures_changed does not re-read the snapshot...
    expect(source.snapshotReads).toBe(readsBefore);
    // ...so the page must have dropped the sight from the vitals alone, and does not.
    expect(el.querySelector('[data-testid="familiar-band"]')).toBeNull();
  });
});
