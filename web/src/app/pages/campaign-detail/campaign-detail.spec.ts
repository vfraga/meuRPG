import { Injectable, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter, Router } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { Code, ConnectError } from '@connectrpc/connect';

import { Campaign, Member, Role, XpMode } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { AuthService, AuthState } from '../../core/auth/auth.service';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import { TableRulesClient, draftFromRules } from '../../core/campaigns/table-rules';
import { TableContentClient } from '../../core/content/content-client';
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
import { MesaTab } from './tabs/mesa-tab';
import { PrepTab } from './tabs/prep-tab';
import { SetupTab } from './tabs/setup-tab';
import {
  CampaignCharactersSource,
  CampaignCharactersVm,
} from './characters/campaign-characters.types';
import { GameSessionSource, GameSessionVm } from '../../core/play/game-session-card.types';
import { nextTurn } from '../../../test-settle';
import { endPhone, startPhone } from './phone-testing';

/** `CampaignCharacters` and `CampaignNpcs` (through the page's `CampaignRoster`) and `GameSessionCard`
 * (the master's "Sessão" strip) are children of this page and of its tabs: they need a provider or Angular DI throws. */
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
  openChoices() {
    return Promise.resolve([]);
  }
}

@Injectable()
class FakeGameSessionSource {
  getCurrentSessionResult: Promise<GameSessionVm | null> = Promise.resolve(null);
  listSessions(): Promise<readonly GameSessionVm[]> {
    return this.getCurrentSessionResult.then((open) => (open ? [open] : []));
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
describe('CampaignDetail', () => {
  let fake: FakeCampaignsService;
  const experience = vi.fn();
  const listAwards = vi.fn();
  let puzzles: FakePuzzlesClient;

  /** The page and its three tabs, as `campaign-detail.routes.ts` lays them out (without the lazy loading). */
  const routes = [
    {
      path: 'campaigns/:id',
      component: CampaignDetail,
      children: [
        { path: '', pathMatch: 'full' as const, component: MesaTab },
        { path: 'prep', component: PrepTab },
        { path: 'setup', component: SetupTab },
      ],
    },
  ];

  function configure(): void {
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
      providers: [
        provideRouter(routes),
        { provide: CampaignsService, useClass: FakeCampaignsService },
        { provide: CampaignCharactersSource, useClass: FakeCampaignCharactersSource },
        { provide: GameSessionSource, useClass: FakeGameSessionSource },
        { provide: GalleryClient, useClass: FakeGalleryClient },
        { provide: AuthService, useClass: FakeAuthService },
        { provide: ProgressionClient, useValue: { experience, listAwards } },
        { provide: RosterClient, useValue: { list: () => Promise.resolve([]) } },
        { provide: PuzzlesClient, useValue: puzzles },
        {
          provide: TableRulesClient,
          useValue: { get: () => Promise.resolve({ saved: draftFromRules(undefined) }) },
        },
        {
          provide: TableContentClient,
          useValue: {
            list: () =>
              Promise.resolve({ entries: [], tableRevision: 0, contentVersion: '', leftOut: [] }),
          },
        },
      ],
    });
    fake = TestBed.inject(CampaignsService) as unknown as FakeCampaignsService;
  }

  let harness: RouterTestingHarness;

  /** Opens the campaign page at a tab (`''` is "Mesa", then `/prep` and `/setup`) and lets what it asks for arrive. */
  async function render(tab = '', id = 'camp-1'): Promise<HTMLElement> {
    harness = await RouterTestingHarness.create();
    await harness.navigateByUrl(`/campaigns/${id}${tab}`, CampaignDetail);
    await settleAll();
    return harness.routeNativeElement as HTMLElement;
  }

  async function settleAll(): Promise<void> {
    await nextTurn();
    await harness.fixture.whenStable();
    harness.fixture.detectChanges();
    await nextTurn();
    harness.fixture.detectChanges();
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
    // Never an e-mail, and never a bare "Sem nome": the fallback says where the name is missing.
    expect(el.textContent).toContain('Sem nome no perfil');
    expect(el.textContent).not.toContain('Jogador sem nome');
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
    expect(rows[0].textContent).toContain('Sem nome no perfil');
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
    const master = await render('/prep');
    const link = Array.from(master.querySelectorAll('a')).find((a) =>
      a.textContent?.includes('Bestiário'),
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
    const master = await render('/prep');
    const link = Array.from(master.querySelectorAll('a')).find((a) =>
      a.textContent?.includes('Montar encontro'),
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
    const master = await render('/prep');
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
    expect((await render('/prep')).textContent).toContain('Enviar imagem');

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
    const master = await render('/setup');
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

  it('gives only the master the "Exportar campanha" link, to the export page (MR-050)', async () => {
    configure();
    fake.getCampaignResult = Promise.resolve({
      campaign: campaign('camp-1', 'Mirathel', Role.MASTER),
    });
    fake.listMembersResult = Promise.resolve({ members: [] });
    const master = await render('/setup');
    expect(
      Array.from(master.querySelectorAll('a[href="/campaigns/camp-1/export"]')).map((a) =>
        a.textContent?.replace('download', '').trim(),
      ),
    ).toEqual(['Exportar campanha']);

    TestBed.resetTestingModule();
    configure();
    fake.getCampaignResult = Promise.resolve({
      campaign: campaign('camp-1', 'Mirathel', Role.PLAYER),
    });
    fake.listMembersResult = Promise.resolve({ members: [] });
    const player = await render();
    expect(player.querySelector('a[href="/campaigns/camp-1/export"]')).toBeNull();
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
    configure();
    fake.getCampaign = ((id: string) => {
      const d = deferred<{ campaign: Campaign }>();
      pending.set(id, d);
      return d.promise;
    }) as unknown as typeof fake.getCampaign;

    harness = await RouterTestingHarness.create();
    await harness.navigateByUrl('/campaigns/camp-a', CampaignDetail);
    await nextTurn();
    // Navigate to B before A answers; B answers first (a player there), then the slow A (the master there).
    await harness.navigateByUrl('/campaigns/camp-b', CampaignDetail);
    await nextTurn();
    pending.get('camp-b')!.resolve({ campaign: campaign('camp-b', 'Campanha B', Role.PLAYER) });
    await nextTurn();
    pending.get('camp-a')!.resolve({ campaign: campaign('camp-a', 'Campanha A', Role.MASTER) });
    await settleAll();

    const h1 = harness.routeNativeElement?.querySelector('h1')?.textContent ?? '';
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
      await nextTurn();
      expect(el.querySelector('app-experience-panel h2')?.textContent?.trim()).toBe('Experiência');
      expect(experience).toHaveBeenCalledTimes(1);
      expect(listAwards).toHaveBeenCalledTimes(1);
    });

    it('puts it between the players and "Sessões anteriores" on "Mesa", and gives only the master "Dar XP"', async () => {
      asRole(Role.MASTER);
      const el = await render();
      await nextTurn();
      const column = el.querySelector('.tab-layout__column')!;
      expect(Array.from(column.children, (c) => c.tagName.toLowerCase())).toEqual([
        'app-campaign-characters',
        'app-experience-panel',
        'app-past-sessions-panel',
      ]);
      expect(
        Array.from(el.querySelectorAll('app-experience-panel button')).some(
          (b) => b.textContent?.replace('star', '').trim() === 'Dar XP',
        ),
      ).toBe(true);
    });

    const columnOrder = (el: HTMLElement) =>
      Array.from(el.querySelector('.campaign-layout__column')!.children).map((c) =>
        c.tagName.toLowerCase(),
      );

    it('puts a player\'s characters right after "Sessão", before the XP and the dice', async () => {
      asRole(Role.PLAYER);
      const el = await render();
      await nextTurn();
      const order = columnOrder(el);
      expect(order.indexOf('app-campaign-characters')).toBe(
        order.indexOf('app-game-session-card') + 1,
      );
      expect(order.indexOf('app-campaign-characters')).toBeLessThan(
        order.indexOf('app-experience-panel'),
      );
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
      await nextTurn();
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

  describe("the master's tabs: Mesa, Preparação, Regras", () => {
    const asMaster = (pending = 0, role = Role.MASTER) => {
      configure();
      fake.getCampaignResult = Promise.resolve({ campaign: campaign('camp-1', 'Mirathel', role) });
      fake.listMembersResult = Promise.resolve({ members: [member('u1', 'Samuel', role)] });
      const source = TestBed.inject(
        CampaignCharactersSource,
      ) as unknown as FakeCampaignCharactersSource;
      const player = (id: string, name: string, state: 'locked' | 'pending') => ({
        id,
        name,
        kind: 'player' as const,
        state,
        classSummary: 'Mago 3',
        playerDisplayName: 'Ana',
      });
      source.listCharactersResult = Promise.resolve({
        playerCharacters: [
          player('c1', 'Pensantus', 'locked'),
          ...Array.from({ length: pending }, (_, i) => player(`p${i}`, `Brisa ${i}`, 'pending')),
        ],
        npcs: [
          {
            id: 'n1',
            name: 'Bartolo',
            kind: 'story' as const,
            state: 'draft' as const,
            classSummary: '',
            playerDisplayName: null,
          },
        ],
        hasLivingCharacter: true,
      });
    };

    const tabs = (el: HTMLElement) =>
      Array.from(el.querySelectorAll<HTMLAnchorElement>('nav[aria-label="Seções da campanha"] a'));

    it('has the three tabs as links in a nav named "Seções da campanha", each with an icon', async () => {
      asMaster();
      const el = await render();
      expect(tabs(el).map((a) => a.getAttribute('href'))).toEqual([
        '/campaigns/camp-1',
        '/campaigns/camp-1/prep',
        '/campaigns/camp-1/setup',
      ]);
      expect(tabs(el).map((a) => a.querySelector('mat-icon')?.textContent)).toEqual([
        'group',
        'map',
        'checklist',
      ]);
      expect(
        tabs(el).map((a) => a.textContent?.replace(/[a-z_]+(?=Mesa|Prep|Regras)/, '').trim()),
      ).toEqual(['Mesa', 'Preparação', 'Regras']);
    });

    it('marks the open tab with aria-current="page", and only that one', async () => {
      asMaster();
      for (const [path, index] of [
        ['', 0],
        ['/prep', 1],
        ['/setup', 2],
      ] as const) {
        TestBed.resetTestingModule();
        asMaster();
        const el = await render(path);
        expect(tabs(el).map((a) => a.getAttribute('aria-current'))).toEqual(
          [0, 1, 2].map((i) => (i === index ? 'page' : null)),
        );
      }
    });

    it('counts the characters that wait on "Mesa", in words for a screen reader, and shows no count at 0', async () => {
      asMaster(1);
      const el = await render();
      const badge = tabs(el)[0].querySelector('.tab__count')!;
      expect(badge.textContent).toContain('1');
      expect(badge.textContent).toContain('esperando aprovação');
      expect(badge.querySelectorAll('.mr-visually-hidden').length).toBeGreaterThan(0);
      expect(tabs(el)[1].querySelector('.tab__count')).toBeNull();

      TestBed.resetTestingModule();
      asMaster(0);
      const none = await render();
      expect(tabs(none)[0].querySelector('.tab__count')).toBeNull();
    });

    it('keeps the count on the other tabs: it comes from the page, not from "Mesa"', async () => {
      asMaster(2);
      const el = await render('/prep');
      expect(tabs(el)[0].querySelector('.tab__count')?.textContent).toContain('2');
    });

    it('keeps the focus on the tab link after a tab change, and draws the other tab', async () => {
      asMaster();
      const el = await render();
      expect(el.textContent).toContain('Personagens dos jogadores');
      const prep = tabs(el)[1];
      prep.click();
      await settleAll();
      expect(TestBed.inject(Router).url).toBe('/campaigns/camp-1/prep');
      expect(document.activeElement).toBe(prep);
      expect(el.textContent).toContain('Mapas');
      expect(el.textContent).not.toContain('Personagens dos jogadores');
    });

    it('draws the session strip above the tabs on every tab', async () => {
      for (const path of ['', '/prep', '/setup']) {
        TestBed.resetTestingModule();
        asMaster();
        const el = await render(path);
        const strip = el.querySelector('app-game-session-card')!;
        const nav = el.querySelector('nav[aria-label="Seções da campanha"]')!;
        expect(strip.textContent).toContain('Iniciar sessão');
        expect(strip.compareDocumentPosition(nav) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
      }
    });

    it('splits the characters: the players on "Mesa", the NPCs and "Novo NPC" on "Preparação"', async () => {
      asMaster();
      const mesa = await render();
      expect(mesa.querySelector('app-campaign-characters')?.textContent).toContain('Pensantus');
      expect(mesa.querySelector('app-campaign-npcs')).toBeNull();
      expect(mesa.textContent).not.toContain('Bartolo');
      expect(mesa.textContent).not.toContain('Novo NPC');

      TestBed.resetTestingModule();
      asMaster();
      const prep = await render('/prep');
      expect(prep.querySelector('app-campaign-characters')).toBeNull();
      expect(prep.querySelector('app-campaign-npcs')?.textContent).toContain('Bartolo');
      expect(prep.querySelector('app-campaign-npcs')?.textContent).toContain('Novo NPC');
      expect(prep.textContent).not.toContain('Pensantus');
    });

    it('puts on each tab what the brief names for it', async () => {
      const titles = async (path: string) => {
        TestBed.resetTestingModule();
        asMaster();
        const el = await render(path);
        return Array.from(
          el.querySelectorAll('app-mesa-tab h2, app-prep-tab h2, app-setup-tab h2'),
          (h) => h.textContent?.trim(),
        );
      };
      expect(await titles('')).toEqual(
        expect.arrayContaining(['Experiência', 'Sessões anteriores', 'Membros', 'Convites']),
      );
      expect(await titles('/prep')).toEqual(
        expect.arrayContaining([
          'Mapas',
          'NPCs',
          'Encontros',
          'Quebra-cabeças',
          'Galeria',
          'Documento da campanha',
        ]),
      );
      // The bestiary is one row that is a link.
      TestBed.resetTestingModule();
      asMaster();
      const prep = await render('/prep');
      expect(prep.querySelector('app-bestiary-panel a')?.textContent).toContain('Bestiário');
      expect(await titles('/setup')).toEqual(
        expect.arrayContaining([
          'Dados',
          'Regras da mesa',
          'Conteúdo da mesa',
          'Magias',
          'Pacote da campanha',
        ]),
      );
    });

    it('puts the three folded rows last on a phone, in the order of the board: Membros, Convites, Sessões anteriores', async () => {
      startPhone();
      try {
        asMaster();
        const el = await render();
        const column = el.querySelector('.tab-layout__column')!;
        const order = Array.from(column.children, (c) => c.tagName.toLowerCase());
        expect(order.slice(-3)).toEqual([
          'app-campaign-members',
          'app-campaign-invites',
          'app-past-sessions-panel',
        ]);
        expect(el.querySelectorAll('.tab-layout__column')).toHaveLength(1);
      } finally {
        endPhone();
      }
    });

    it('gives a player one page, with no tabs', async () => {
      asMaster(0, Role.PLAYER);
      const el = await render();
      expect(el.querySelector('nav[aria-label="Seções da campanha"]')).toBeNull();
      expect(el.querySelector('router-outlet')).toBeNull();
    });

    it('sends an old link to the maps ("#maps") to "Preparação"', async () => {
      asMaster();
      harness = await RouterTestingHarness.create();
      await harness.navigateByUrl('/campaigns/camp-1#maps', CampaignDetail);
      await settleAll();
      expect(TestBed.inject(Router).url).toBe('/campaigns/camp-1/prep#maps');
    });
  });
});
