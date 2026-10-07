// Finding U16-06 in review/unit-16-web-content-campaigns.md
import { Injectable, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';
import { of } from 'rxjs';

import { Campaign, Member, XpMode } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { AuthService, AuthState } from '../../core/auth/auth.service';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import { GalleryClient } from '../../core/images/gallery-client';
import { RosterClient } from '../../core/maps/roster-client';
import { PuzzlesClient } from '../../core/puzzles/puzzles-client';
import { FakePuzzlesClient } from '../../core/puzzles/puzzles-testing';
import { ProgressionClient } from '../../core/progression/progression-client';
import { create } from '@bufbuild/protobuf';
import {
  CharacterExperienceSchema,
  GetCampaignExperienceResponseSchema,
  ListXPAwardsResponseSchema,
} from '../../../gen/meurpg/progression/v1/progression_pb';
import { FakeGalleryClient } from '../../core/images/gallery-testing';
import { CampaignDetail } from './campaign-detail';
import {
  CampaignCharactersSource,
  CampaignCharactersVm,
} from './characters/campaign-characters.types';
import { GameSessionSource, GameSessionVm } from './game-session/game-session-card.types';

/** `CampaignCharacters` (the "Personagens" section) and `GameSessionCard`
 * (the master's "Sessão" card) are children of this page too — see
 * `campaign-detail.html`. Both need a provider or Angular DI throws. */
@Injectable()
class FakeCampaignCharactersSource {
  listCharactersResult: Promise<CampaignCharactersVm> = Promise.resolve({
    playerCharacters: [],
    npcs: [],
    hasLivingCharacter: false,
  });
  listCharacters(): Promise<CampaignCharactersVm> {
    return this.listCharactersResult;
  }
}

@Injectable()
class FakeGameSessionSource {
  getCurrentSessionResult: Promise<GameSessionVm | null> = Promise.resolve(null);
  getCurrentSession(): Promise<GameSessionVm | null> {
    return this.getCurrentSessionResult;
  }
}

/** Covers both CampaignDetail's own calls and CampaignInvites' (the master
 * section it renders as a child), so this fake needs every method both use. */
@Injectable()
class FakeCampaignsService {
  getCampaignResult: Promise<{ campaign: Campaign | undefined }> = Promise.resolve({
    campaign: undefined,
  });
  listMembersResult: Promise<{ members: Member[] }> = Promise.resolve({ members: [] });
  listInvitesResult: Promise<{ invites: [] }> = Promise.resolve({ invites: [] });

  getCampaign(): Promise<{ campaign: Campaign | undefined }> {
    return this.getCampaignResult;
  }
  listMembers(): Promise<{ members: Member[] }> {
    return this.listMembersResult;
  }
  listInvites(): Promise<{ invites: [] }> {
    return this.listInvitesResult;
  }
  listPendingMembers(): Promise<{ members: [] }> {
    return Promise.resolve({ members: [] });
  }
}

/** Who is signed in: `u1` unless a test says otherwise. */
class FakeAuthService {
  readonly state = signal<AuthState>({
    status: 'signed-in',
    user: { id: 'u1', displayName: null },
    sessionExpiresAt: null,
  });
}

function activatedRouteFor(id: string) {
  return { paramMap: of(convertToParamMap({ id })) };
}

/** Drains pending microtasks (the paramMap subscription and the
 * Promise.all().then() chain it kicks off both hop through a few) before
 * the next detectChanges() — more robust here than relying solely on
 * `fixture.whenStable()`, since the async work is not itself signal-driven
 * until the very end of the chain. */
function flush(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

describe('Review16 U16-06: ended session reads as server down on campaign detail', () => {
  let fake: FakeCampaignsService;
  const experience = vi.fn();
  const listAwards = vi.fn();
  let puzzles: FakePuzzlesClient;

  function configure(id = 'camp-1'): void {
    experience.mockReset().mockResolvedValue(
      create(GetCampaignExperienceResponseSchema, {
        xpMode: XpMode.ENEMIES,
        characters: [
          create(CharacterExperienceSchema, {
            characterId: 'c1',
            name: 'Pensantus',
            level: 3,
            experiencePoints: 2716,
            nextLevelXp: 2700,
            canLevelUp: true,
          }),
        ],
      }),
    );
    listAwards.mockReset().mockResolvedValue(create(ListXPAwardsResponseSchema, {}));
    puzzles = new FakePuzzlesClient();
    TestBed.configureTestingModule({
      imports: [CampaignDetail],
      providers: [
        { provide: CampaignsService, useClass: FakeCampaignsService },
        { provide: ActivatedRoute, useValue: activatedRouteFor(id) },
        { provide: CampaignCharactersSource, useClass: FakeCampaignCharactersSource },
        { provide: GameSessionSource, useClass: FakeGameSessionSource },
        { provide: GalleryClient, useClass: FakeGalleryClient },
        { provide: AuthService, useClass: FakeAuthService },
        { provide: ProgressionClient, useValue: { experience, listAwards } },
        { provide: RosterClient, useValue: { list: () => Promise.resolve([]) } },
        { provide: PuzzlesClient, useValue: puzzles },
      ],
    });
    fake = TestBed.inject(CampaignsService) as unknown as FakeCampaignsService;
  }

  async function render(): Promise<HTMLElement> {
    const fixture = TestBed.createComponent(CampaignDetail);
    fixture.detectChanges();
    await flush();
    await fixture.whenStable();
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }

  it('does not tell a person whose session ended to just try again', async () => {
    configure();
    fake.getCampaignResult = Promise.reject(new ConnectError('', Code.Unauthenticated));

    const el = await render();
    const text = el.textContent ?? '';
    expect(text).not.toContain('Tente de novo');
    expect(text.toLowerCase()).toMatch(/entr(e|ar)|sess[aã]o/);
  });
});
