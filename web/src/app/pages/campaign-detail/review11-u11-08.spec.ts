// Finding U11-08: CampaignDetail.load() has no sequence guard, so a slow
// response for campaign A that lands after campaign B's overwrites the page.
import { Injectable, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { BehaviorSubject } from 'rxjs';

import { Campaign, Member, Role, XpMode } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  GetCampaignExperienceResponseSchema,
  ListXPAwardsResponseSchema,
} from '../../../gen/meurpg/progression/v1/progression_pb';
import { AuthService, AuthState } from '../../core/auth/auth.service';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import { GalleryClient } from '../../core/images/gallery-client';
import { FakeGalleryClient } from '../../core/images/gallery-testing';
import { RosterClient } from '../../core/maps/roster-client';
import { ProgressionClient } from '../../core/progression/progression-client';
import { PuzzlesClient } from '../../core/puzzles/puzzles-client';
import { FakePuzzlesClient } from '../../core/puzzles/puzzles-testing';
import { ExperienceStore } from '../../core/progression/experience-store';
import { CampaignDetail } from './campaign-detail';
import { CampaignCharactersSource } from './characters/campaign-characters.types';
import { GameSessionSource } from './game-session/game-session-card.types';

type CampaignRes = { campaign: Campaign | undefined };

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => (resolve = r));
  return { promise, resolve };
}

function campaign(id: string, name: string, myRole: Role): Campaign {
  return { id, name, myRole, xpMode: XpMode.ENEMIES, createdAt: undefined } as Campaign;
}

@Injectable()
class FakeCampaignCharactersSource {
  listCharacters() {
    return Promise.resolve({ playerCharacters: [], npcs: [], hasLivingCharacter: false });
  }
}

@Injectable()
class FakeGameSessionSource {
  getCurrentSession() {
    return Promise.resolve(null);
  }
}

class FakeAuthService {
  readonly state = signal<AuthState>({
    status: 'signed-in',
    user: { id: 'u1', displayName: null },
    sessionExpiresAt: null,
  });
}

const flush = () => new Promise<void>((r) => setTimeout(r, 0));

describe('Review11 U11-08: campaign switch applies a stale response', () => {
  it('keeps showing B (and loading B experience) when A resolves after B', async () => {
    const pending = new Map<string, ReturnType<typeof deferred<CampaignRes>>>();
    const fakeCampaigns = {
      getCampaign: (id: string) => {
        const d = deferred<CampaignRes>();
        pending.set(id, d);
        return d.promise;
      },
      listMembers: () => Promise.resolve({ members: [] as Member[] }),
      listInvites: () => Promise.resolve({ invites: [] }),
      listPendingMembers: () => Promise.resolve({ members: [] }),
    };
    const params$ = new BehaviorSubject(convertToParamMap({ id: 'A' }));
    const experience = vi
      .fn()
      .mockResolvedValue(create(GetCampaignExperienceResponseSchema, { xpMode: XpMode.ENEMIES }));
    const listAwards = vi.fn().mockResolvedValue(create(ListXPAwardsResponseSchema, {}));

    TestBed.configureTestingModule({
      imports: [CampaignDetail],
      providers: [
        { provide: CampaignsService, useValue: fakeCampaigns },
        { provide: ActivatedRoute, useValue: { paramMap: params$ } },
        { provide: CampaignCharactersSource, useClass: FakeCampaignCharactersSource },
        { provide: GameSessionSource, useClass: FakeGameSessionSource },
        { provide: GalleryClient, useClass: FakeGalleryClient },
        { provide: AuthService, useClass: FakeAuthService },
        { provide: ProgressionClient, useValue: { experience, listAwards } },
        { provide: RosterClient, useValue: { list: () => Promise.resolve([]) } },
        { provide: PuzzlesClient, useValue: new FakePuzzlesClient() },
      ],
    });
    // ExperienceStore is provided at component level, so spy on the prototype.
    const loadSpy = vi.spyOn(ExperienceStore.prototype, 'load').mockResolvedValue(undefined as never);

    const fixture = TestBed.createComponent(CampaignDetail);
    fixture.detectChanges();
    await flush();

    // Navigate A -> B before A answers.
    params$.next(convertToParamMap({ id: 'B' }));
    await flush();

    // B answers first (viewer is a player there), then the slow A (viewer is master).
    pending.get('B')!.resolve({ campaign: campaign('B', 'Campanha B', Role.PLAYER) });
    await flush();
    pending.get('A')!.resolve({ campaign: campaign('A', 'Campanha A', Role.MASTER) });
    await flush();
    fixture.detectChanges();

    const h1 = (fixture.nativeElement as HTMLElement).querySelector('h1')?.textContent ?? '';
    expect(h1).toContain('Campanha B');
    expect(h1).not.toContain('Campanha A');
    const lastCall = loadSpy.mock.calls[loadSpy.mock.calls.length - 1];
    expect(lastCall).toEqual(['B', true, false]);
  });
});
