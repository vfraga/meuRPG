import { Injectable, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';
import { BehaviorSubject, of } from 'rxjs';

import { Campaign, Member, Role, XpMode } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { AuthService, AuthState } from '../../core/auth/auth.service';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import { GalleryClient } from '../../core/images/gallery-client';
import { RosterClient } from '../../core/maps/roster-client';
import { PuzzlesClient } from '../../core/puzzles/puzzles-client';
import { FakePuzzlesClient, lightsPuzzle } from '../../core/puzzles/puzzles-testing';
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

function campaign(id: string, name: string, myRole: Role): Campaign {
  return { id, name, myRole, xpMode: XpMode.ENEMIES, createdAt: undefined } as Campaign;
}

function member(userId: string, displayName: string, role: Role): Member {
  return { userId, displayName, role, joinedAt: undefined } as Member;
}

function activatedRouteFor(id: string) {
  return { paramMap: of(convertToParamMap({ id })) };
}

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => (resolve = r));
  return { promise, resolve };
}

/** Drains pending microtasks (the paramMap subscription and the
 * Promise.all().then() chain it kicks off both hop through a few) before
 * the next detectChanges() — more robust here than relying solely on
 * `fixture.whenStable()`, since the async work is not itself signal-driven
 * until the very end of the chain. */
function flush(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

describe('CampaignDetail', () => {
  let fake: FakeCampaignsService;
  const experience = vi.fn();
  const listAwards = vi.fn();
  let puzzles: FakePuzzlesClient;

  function configure(id = 'camp-1', route: unknown = activatedRouteFor(id)): void {
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
        { provide: ActivatedRoute, useValue: route },
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

  it('shows the campaign name as the only h1, and members with display names', async () => {
    configure();
    fake.getCampaignResult = Promise.resolve({
      campaign: campaign('camp-1', 'Mirathel', Role.MASTER),
    });
    fake.listMembersResult = Promise.resolve({
      members: [member('u1', 'Vinicius', Role.MASTER), member('u2', '', Role.PLAYER)],
    });

    const el = await render();
    const headings = el.querySelectorAll('h1');
    expect(headings.length).toBe(1);
    expect(headings[0].textContent).toContain('Mirathel');
    expect(el.textContent).toContain('Você é mestre nesta campanha. XP por inimigos derrotados.');
    expect(el.textContent).toContain('Vinicius');
    // Never an e-mail, and never a bare "Sem nome": the fallback says the role.
    expect(el.textContent).toContain('Jogador sem nome');
    expect(el.textContent).not.toContain('Sem nome');
    expect(el.textContent).not.toContain('@');
  });

  it('marks the viewer\'s own row, and points them to "Meu perfil" while they have no name', async () => {
    configure();
    fake.getCampaignResult = Promise.resolve({
      campaign: campaign('camp-1', 'Mirathel', Role.MASTER),
    });
    fake.listMembersResult = Promise.resolve({
      members: [member('u1', '', Role.MASTER), member('u2', 'Vinicius', Role.PLAYER)],
    });

    const el = await render();
    const rows = Array.from(el.querySelectorAll('section[aria-labelledby="members-heading"] li'));
    expect(rows[0].textContent).toContain('Mestre sem nome');
    expect(rows[0].textContent).toContain('(você)');
    expect(rows[0].querySelector('a')?.getAttribute('href')).toBe('/profile');
    // Somebody else's row: no "(você)" and no link to the viewer's profile.
    expect(rows[1].textContent).not.toContain('(você)');
    expect(rows[1].querySelector('a')).toBeNull();
  });

  it('shows "campanha não encontrada" for a not_found response, and never reveals why', async () => {
    configure();
    // ListMembers is never asked: the page loads the campaign first.
    fake.getCampaignResult = Promise.reject(new ConnectError('no such campaign', Code.NotFound));

    const el = await render();
    expect(el.querySelector('h1')?.textContent).toContain('não encontrada');
  });

  it('shows the Convites section only for the master', async () => {
    configure();
    fake.getCampaignResult = Promise.resolve({
      campaign: campaign('camp-1', 'Mirathel', Role.MASTER),
    });
    fake.listMembersResult = Promise.resolve({ members: [] });

    const el = await render();
    expect(el.textContent).toContain('Convites');
  });

  it('hides the Convites section for a player', async () => {
    configure();
    fake.getCampaignResult = Promise.resolve({
      campaign: campaign('camp-1', 'Mirathel', Role.PLAYER),
    });
    fake.listMembersResult = Promise.resolve({ members: [] });

    const el = await render();
    expect(el.textContent).not.toContain('Convites');
  });

  it('shows the "Personagens" section for everyone, and the "Sessão" card only for the master', async () => {
    configure();
    fake.getCampaignResult = Promise.resolve({
      campaign: campaign('camp-1', 'Mirathel', Role.MASTER),
    });
    fake.listMembersResult = Promise.resolve({ members: [] });

    const el = await render();
    expect(el.textContent).toContain('Personagens');
    expect(el.textContent).toContain('Sessão');
  });

  it('hides the "Sessão" card for a player', async () => {
    configure();
    fake.getCampaignResult = Promise.resolve({
      campaign: campaign('camp-1', 'Mirathel', Role.PLAYER),
    });
    fake.listMembersResult = Promise.resolve({ members: [] });

    const el = await render();
    expect(el.textContent).toContain('Personagens');
    expect(el.textContent).not.toContain('Iniciar sessão');
  });

  it('shows the "Bestiário" panel only for the master (MR-042: the SRD is public, the app shows the bestiary to the master)', async () => {
    configure();
    fake.getCampaignResult = Promise.resolve({
      campaign: campaign('camp-1', 'Mirathel', Role.MASTER),
    });
    fake.listMembersResult = Promise.resolve({ members: [] });
    const master = await render();
    const link = Array.from(master.querySelectorAll('a')).find((a) =>
      a.textContent?.includes('Abrir o bestiário'),
    );
    expect(link?.getAttribute('href')).toBe('/campaigns/camp-1/bestiary');

    TestBed.resetTestingModule();
    configure();
    fake.getCampaignResult = Promise.resolve({
      campaign: campaign('camp-1', 'Mirathel', Role.PLAYER),
    });
    fake.listMembersResult = Promise.resolve({ members: [] });
    const player = await render();
    expect(player.textContent).not.toContain('Bestiário');
    expect(player.querySelector('a[href$="/bestiary"]')).toBeNull();
  });

  it('shows the "Encontros" panel only for the master (MR-043: the builder and what it keeps are his secret, RN-10)', async () => {
    configure();
    fake.getCampaignResult = Promise.resolve({
      campaign: campaign('camp-1', 'Mirathel', Role.MASTER),
    });
    fake.listMembersResult = Promise.resolve({ members: [] });
    const master = await render();
    const link = Array.from(master.querySelectorAll('a')).find((a) =>
      a.textContent?.includes('Montar um encontro'),
    );
    expect(link?.getAttribute('href')).toBe('/campaigns/camp-1/encounters');

    TestBed.resetTestingModule();
    configure();
    fake.getCampaignResult = Promise.resolve({
      campaign: campaign('camp-1', 'Mirathel', Role.PLAYER),
    });
    fake.listMembersResult = Promise.resolve({ members: [] });
    const player = await render();
    expect(player.textContent).not.toContain('Encontros');
    expect(player.querySelector('a[href$="/encounters"]')).toBeNull();
  });

  it('shows the "Quebra-cabeças" panel only for the master (MR-038: the answers live there)', async () => {
    configure();
    puzzles.listResult = [lightsPuzzle('a', 'O selo da Capela')];
    fake.getCampaignResult = Promise.resolve({
      campaign: campaign('camp-1', 'Mirathel', Role.MASTER),
    });
    fake.listMembersResult = Promise.resolve({ members: [] });
    const master = await render();
    expect(master.textContent).toContain('O selo da Capela');
    expect(master.querySelector('a[href="/campaigns/camp-1/puzzles/new"]')).not.toBeNull();

    TestBed.resetTestingModule();
    configure();
    fake.getCampaignResult = Promise.resolve({
      campaign: campaign('camp-1', 'Mirathel', Role.PLAYER),
    });
    fake.listMembersResult = Promise.resolve({ members: [] });
    const player = await render();
    expect(player.textContent).not.toContain('Quebra-cabeças');
    expect(puzzles.calls).toEqual([]);
  });

  it('shows the "Galeria" panel only for the master (MR-019)', async () => {
    configure();
    fake.getCampaignResult = Promise.resolve({
      campaign: campaign('camp-1', 'Mirathel', Role.MASTER),
    });
    fake.listMembersResult = Promise.resolve({ members: [] });
    expect((await render()).textContent).toContain('Abrir galeria');

    TestBed.resetTestingModule();
    configure();
    fake.getCampaignResult = Promise.resolve({
      campaign: campaign('camp-1', 'Mirathel', Role.PLAYER),
    });
    fake.listMembersResult = Promise.resolve({ members: [] });
    const el = await render();
    expect(el.textContent).not.toContain('Galeria');
    expect((TestBed.inject(GalleryClient) as unknown as FakeGalleryClient).calls).toEqual([]);
  });

  it('gives everyone the "Regras da mesa" panel: the master opens them, a player reads them (MR-025)', async () => {
    configure();
    fake.getCampaignResult = Promise.resolve({
      campaign: campaign('camp-1', 'Mirathel', Role.MASTER),
    });
    fake.listMembersResult = Promise.resolve({ members: [] });
    const master = await render();
    expect(master.textContent).toContain('Regras da mesa');
    expect(
      Array.from(master.querySelectorAll('a[href="/campaigns/camp-1/rules"]')).map((a) =>
        a.textContent?.trim(),
      ),
    ).toContain('Abrir as regras');

    TestBed.resetTestingModule();
    configure();
    fake.getCampaignResult = Promise.resolve({
      campaign: campaign('camp-1', 'Mirathel', Role.PLAYER),
    });
    fake.listMembersResult = Promise.resolve({ members: [] });
    const player = await render();
    expect(
      Array.from(player.querySelectorAll('a[href="/campaigns/camp-1/rules"]')).map((a) =>
        a.textContent?.trim(),
      ),
    ).toContain('Ler as regras');
  });

  it('a pending member sees the wait banner and their character, never the members (MR-024)', async () => {
    configure();
    fake.getCampaignResult = Promise.resolve({
      campaign: {
        ...campaign('camp-1', 'Mirathel', Role.PLAYER),
        awaitingApproval: true,
        diceMode: 1,
        dicePreference: 1,
      },
    });
    const listMembers = vi.spyOn(fake, 'listMembers');

    const el = await render();
    expect(el.querySelector('h1')?.textContent).toContain('Mirathel');
    expect(el.textContent).toContain('Esperando a aprovação do mestre');
    expect(el.textContent).toContain('Personagens');
    expect(el.textContent).not.toContain('Membros');
    expect(el.textContent).not.toContain('Convites');
    expect(listMembers).not.toHaveBeenCalled();
  });

  it('shows the campaign it was reused for when the answer of the one it left lands last', async () => {
    const pending = new Map<string, ReturnType<typeof deferred<{ campaign: Campaign }>>>();
    const params$ = new BehaviorSubject(convertToParamMap({ id: 'camp-a' }));
    configure('camp-a', { paramMap: params$ });
    fake.getCampaign = ((id: string) => {
      const d = deferred<{ campaign: Campaign }>();
      pending.set(id, d);
      return d.promise;
    }) as unknown as typeof fake.getCampaign;

    const fixture = TestBed.createComponent(CampaignDetail);
    fixture.detectChanges();
    await flush();
    // Navigate to B before A answers; B answers first (a player there), then the slow A (the master there).
    params$.next(convertToParamMap({ id: 'camp-b' }));
    await flush();
    pending.get('camp-b')!.resolve({ campaign: campaign('camp-b', 'Campanha B', Role.PLAYER) });
    await flush();
    pending.get('camp-a')!.resolve({ campaign: campaign('camp-a', 'Campanha A', Role.MASTER) });
    await flush();
    fixture.detectChanges();

    const h1 = (fixture.nativeElement as HTMLElement).querySelector('h1')?.textContent ?? '';
    expect(h1).toContain('Campanha B');
    expect(h1).not.toContain('Campanha A');
    expect(experience).toHaveBeenCalledWith('camp-b');
    expect(experience).not.toHaveBeenCalledWith('camp-a');
  });

  it('shows a generic error message for a non-not_found failure', async () => {
    configure();
    fake.getCampaignResult = Promise.reject(new ConnectError('down', Code.Unavailable));

    const el = await render();
    expect(el.querySelector('h1')?.textContent).not.toContain('não encontrada');
    expect(el.textContent).toContain('Tente de novo');
  });

  it('tells a person whose login session ended to sign in again, not to retry', async () => {
    configure();
    fake.getCampaignResult = Promise.reject(new ConnectError('', Code.Unauthenticated));

    const el = await render();
    const text = el.textContent ?? '';
    expect(text).not.toContain('Tente de novo');
    expect(text).toContain('Entre de novo');
  });

  describe('"Experiência" (MR-016, E7-09)', () => {
    const asRole = (role: Role) => {
      configure();
      fake.getCampaignResult = Promise.resolve({ campaign: campaign('camp-1', 'Mirathel', role) });
      fake.listMembersResult = Promise.resolve({ members: [member('u1', 'Samuel', role)] });
    };

    it('shows every member the panel, loaded once with the history', async () => {
      asRole(Role.PLAYER);
      const el = await render();
      await flush();
      expect(el.querySelector('app-experience-panel h2')?.textContent).toBe('Experiência');
      expect(experience).toHaveBeenCalledTimes(1);
      expect(listAwards).toHaveBeenCalledTimes(1);
    });

    it('puts it right after "Sessão", and gives only the master "Dar XP"', async () => {
      asRole(Role.MASTER);
      const el = await render();
      await flush();
      const column = el.querySelector('.campaign-layout__column')!;
      const order = Array.from(column.children).map((c) => c.tagName.toLowerCase());
      expect(order.indexOf('app-experience-panel')).toBe(
        order.indexOf('app-game-session-card') + 1,
      );
      expect(
        Array.from(el.querySelectorAll('app-experience-panel button')).some(
          (b) => b.textContent?.trim() === 'Dar XP',
        ),
      ).toBe(true);
    });

    it('tags who can level up in the group list too (RN-12)', async () => {
      asRole(Role.MASTER);
      const source = TestBed.inject(
        CampaignCharactersSource,
      ) as unknown as FakeCampaignCharactersSource;
      source.listCharactersResult = Promise.resolve({
        playerCharacters: [
          {
            id: 'c1',
            name: 'Pensantus',
            kind: 'player',
            state: 'locked',
            classSummary: 'Mago 3',
            playerDisplayName: 'Vinicius',
          },
        ],
        npcs: [],
        hasLivingCharacter: true,
      });
      const el = await render();
      await flush();
      const fixtureEl = el.querySelector('app-campaign-characters')!;
      expect(fixtureEl.querySelector('app-level-up-tag')?.textContent).toContain(
        'Pode subir de nível',
      );
    });

    it('does not ask a pending member for the XP, which the server would refuse', async () => {
      configure();
      fake.getCampaignResult = Promise.resolve({
        campaign: {
          ...campaign('camp-1', 'Mirathel', Role.PLAYER),
          awaitingApproval: true,
          diceMode: 1,
          dicePreference: 1,
        },
      });
      await render();
      expect(experience).not.toHaveBeenCalled();
    });
  });
});
