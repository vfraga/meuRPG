import { ApplicationRef, Injectable, signal } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { BehaviorSubject, of } from 'rxjs';

import { AuthService } from '../../core/auth/auth.service';
import { TurnAlert } from '../../core/combat/turn-alert';
import { LightPresets } from '../../core/maps/light-presets';
import { MapsClient } from '../../core/maps/maps-client';
import { visionResponse } from '../../core/maps/vision-testing';
import { FamiliarEyesClient } from '../../core/play/familiar-eyes';
import { textOf } from '../../core/format/text-testing';
import { CastingClient } from '../../core/casting/casting-client';
import { FakeCastingClient } from '../../core/casting/casting-testing';
import { RosterClient } from '../../core/maps/roster-client';
import { ProgressionClient } from '../../core/progression/progression-client';
import { XpChanges } from '../../core/progression/xp-changes';
import { RevivifyClient } from '../../core/revivify/revivify-client';
import {
  type RevivifyRequest,
  RevivifyRequestSchema,
  RevivifyRequestStatus,
} from '../../../gen/meurpg/play/v1/revivify_pb';
import { PuzzlesClient } from '../../core/puzzles/puzzles-client';
import { SceneChecks } from '../../core/maps/scene-actions';
import {
  FakePuzzlesClient,
  fakeChecks,
  lightsPuzzle,
  masterRun,
  playerRun,
} from '../../core/puzzles/puzzles-testing';
import { PuzzleRunStatus } from '../../../gen/meurpg/play/v1/puzzles_pb';
import { create } from '@bufbuild/protobuf';
import {
  CastEffect,
  OutsideCastSchema,
  OutsideCastStatus,
  OutsideCastTargetSchema,
} from '../../../gen/meurpg/play/v1/casting_pb';
import {
  CombatantKind,
  CombatantState,
  type Encounter,
  EncounterMode,
  EncounterStatus,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from '../../core/combat/combat-testing';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { XpMode } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CharacterExperienceSchema,
  GetCampaignExperienceResponseSchema,
} from '../../../gen/meurpg/progression/v1/progression_pb';
import {
  FakeMapsClient,
  mapMessage,
  mapPoint,
  mapResponse,
  mapToken,
} from '../../core/maps/maps-testing';
import { SceneClient } from '../../core/play/scene-client';
import { ResourceClient } from '../../core/resources/resources-client';
import { restPreview, restPreviewResponse } from '../../core/resources/resources-testing';
import { CharacterVitalsSchema } from '../../../gen/meurpg/play/v1/play_pb';
import { CombatClient } from '../../core/combat/combat-client';
import { TrapsClient } from '../../core/traps/traps-client';
import { ContestClient } from '../../core/combat/contest-client';
import { FakeContestClient, groupCheck } from '../../core/combat/contest-testing';
import { CombatState } from '../../core/combat/combat-state';
import { SpellCatalog } from '../../core/combat/spell-catalog';
import { SessionSummaryClient } from '../../core/play/session-summary';
import { SessionSummarySchema } from '../../../gen/meurpg/play/v1/summary_pb';
import {
  FakeSceneClient,
  masterScene,
  playerScene,
  sceneRoll,
} from '../../core/play/scene-testing';
import {
  MapPointKind,
  SceneActionSchema,
  TreasureFinderSchema,
} from '../../../gen/meurpg/maps/v1/maps_pb';
import { OpenSessions } from '../../core/play/open-sessions';
import { CALLING_NOTICE_MS, LiveSession, mageArmorClassOf } from './live-session';
import {
  CampaignInfoVm,
  LiveErrorKind,
  LiveEventVm,
  LiveSessionSource,
  LiveSnapshotVm,
  PartyMemberInfoVm,
  PlayerSheetVm,
  ShownImageVm,
} from '../../core/play/live-session.types';
import { brisaVitals, pensantusVitals } from '../../core/play/live-testing';
import { nextTurn } from '../../../test-settle';

/**
 * The page's protected members, looked up where the page keeps them (its own, the state, the view, the sheets, the events
 * and the loader), so a test reads `page.vitals()` as the user of the page would think of it.
 */
function flat(fixture: ComponentFixture<LiveSession>): object {
  const page = fixture.componentInstance as unknown as Record<string, Record<string, unknown>>;
  const homes = [page, page['state'], page['view'], page['sheets'], page['events'], page['loader']];
  return new Proxy(
    {},
    {
      get(_target, key: string) {
        const home = homes.find((h) => key in h);
        const value = home?.[key];
        return typeof value === 'function' ? value.bind(home) : value;
      },
    },
  );
}

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

describe('LiveSession', () => {
  let source: FakeLiveSessionSource;
  /** The casts of Revivify outside a combat that wait for the master. */
  const revivifyList = vi.fn<() => Promise<RevivifyRequest[]>>();
  /** What the master's "Dar XP" reads (MR-016): the party's XP and how the campaign levels. */
  const xpExperience = vi.fn();
  let scenes: FakeSceneClient;
  let puzzles: FakePuzzlesClient;
  /** The group check of the player's page (W7-X). */
  let contests: FakeContestClient;
  /** The query string of the page's address: `?puzzle=ID` opens a puzzle for a player. */
  const query = new BehaviorSubject(convertToParamMap({}));
  /** The summary of the ended session (MR-032); by default it cannot be read, so the page shows the plain notice. */
  const summary = vi.fn();
  const signIn = vi.fn();
  /** The master's rests (PM-07b 9). */
  const resources = { restPreview: vi.fn(), takeRest: vi.fn() };
  const liveCampaignIds = signal<ReadonlySet<string>>(new Set());
  const openSessions = {
    liveCampaignIds: liveCampaignIds.asReadonly(),
    refresh: vi.fn(() => Promise.resolve()),
    dismiss: vi.fn(),
  };

  let casting: FakeCastingClient;

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
    revivifyList.mockReset().mockResolvedValue([]);
    signIn.mockClear();
    resources.restPreview.mockReset();
    resources.takeRest.mockReset();
    openSessions.dismiss.mockClear();
    liveCampaignIds.set(new Set());
    summary.mockReset().mockRejectedValue(new Error('no summary'));
    scenes = new FakeSceneClient();
    casting = new FakeCastingClient();
    puzzles = new FakePuzzlesClient();
    contests = new FakeContestClient();
    contests.group = null;
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
        { provide: CastingClient, useValue: casting },
        { provide: SceneClient, useValue: scenes },
        { provide: ContestClient, useValue: contests.as() },
        { provide: RevivifyClient, useValue: { list: revivifyList, confirmTime: vi.fn() } },
        { provide: ResourceClient, useValue: resources },
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
      await nextTurn();
      fixture.detectChanges();
    }
    return fixture.nativeElement as HTMLElement;
  }

  function render(): Promise<HTMLElement> {
    return settle(TestBed.createComponent(LiveSession));
  }

  function button(el: HTMLElement, name: string): HTMLButtonElement {
    return Array.from(el.querySelectorAll('button')).find((b) =>
      b.textContent?.includes(name),
    ) as HTMLButtonElement;
  }

  it("shows a player their own character's vitals, with the CA from the sheet (E5-02)", async () => {
    const el = await render();
    expect(el.querySelector('h1')?.textContent).toContain('Sessão 4');
    expect(el.textContent).toContain('Mirathel');
    expect(el.textContent).toContain('Ao vivo');
    expect(el.textContent).toContain('Em andamento desde 30/09 às 20:05');
    expect(el.querySelector('.hp__current')?.textContent).toBe('17');
    expect(el.textContent).toContain('de 23');
    expect(el.querySelector('.shield__number')?.textContent).toBe('14');
    expect(el.textContent).toContain('Mago 3, Gnomo das Rochas');
    expect(el.textContent).toContain('Restam 2 de 3d6');
    expect(el.querySelector('[aria-label="1º nível: 2 livres de 4"]')).not.toBeNull();
    expect(el.textContent).toContain('O mestre ainda não escolheu um mapa.');
    expect(el.textContent).not.toContain('Ajustar');
    // Here already: the notice about this session is spent.
    expect(openSessions.dismiss).toHaveBeenCalledWith('s4');
  });

  describe('XP (MR-016)', () => {
    const asMaster = () => {
      source.campaign = {
        name: 'Mirathel',
        isMaster: true,
        awaitingApproval: false,
        diceMode: 1,
        dicePreference: 1,
      };
    };

    it('gives the master "Dar XP" in the party panel', async () => {
      asMaster();
      const master = await render();
      expect(master.querySelector('app-party-panel app-xp-give-button')).not.toBeNull();
      expect(button(master, 'Dar XP')).toBeTruthy();
    });

    it('says "Registrar marco" instead in a campaign that levels by milestones', async () => {
      asMaster();
      xpExperience.mockResolvedValue(
        create(GetCampaignExperienceResponseSchema, { xpMode: XpMode.MILESTONES, characters: [] }),
      );
      const el = await render();
      expect(button(el, 'Registrar marco')).toBeTruthy();
      expect(button(el, 'Dar XP')).toBeUndefined();
    });

    it('does not read the XP for a player (only the master has "Dar XP" here)', async () => {
      await render();
      expect(xpExperience).not.toHaveBeenCalled();
    });

    it('lets everything that depends on XP know when the stream says it changed, and ignores what it does not know', async () => {
      asMaster();
      await render();
      const changes = TestBed.inject(XpChanges);
      const before = changes.version();
      xpExperience.mockClear();

      source.push({ kind: 'xpChanged' });
      await nextTurn();
      expect(changes.version()).toBe(before + 1);
      // The party panel's "Dar XP" reads the characters again.
      await nextTurn();
      expect(xpExperience).toHaveBeenCalledTimes(1);
    });
  });

  describe('the group check (W7-X)', () => {
    const reads = () => contests.calls.filter((c) => c === 'groupCheck').length;

    it('reads the group check again when the stream says it changed, and shows the player’s wait', async () => {
      contests.group = groupCheck({ youRoll: false, members: [{ answered: true } as never] });
      const el = await render();
      const first = reads();
      expect(first).toBeGreaterThanOrEqual(1);
      expect(el.querySelector('app-group-check-card')?.textContent).toContain('Esperando o mestre');
      source.push({ kind: 'groupCheckChanged' });
      await nextTurn();
      await nextTurn();
      expect(reads()).toBe(first + 1);
    });

    it('shows nothing when the session has no group check', async () => {
      const el = await render();
      expect(el.querySelector('app-group-check-card')?.textContent?.trim()).toBe('');
    });
  });

  describe('puzzles (MR-038, E10-06)', () => {
    const selo = lightsPuzzle('a', 'O selo da Capela');

    it('tells a player the master showed a puzzle, with the way in, above the board', async () => {
      puzzles.shownResult = [summary(selo)];
      const el = await render();
      expect(el.querySelector('app-puzzle-notice')?.textContent).toContain(
        'O mestre mostrou um quebra-cabeça',
      );
      expect(el.querySelector('app-puzzle-notice a')?.textContent).toContain(
        'Abrir o quebra-cabeça',
      );
      expect(el.querySelector('app-master-puzzles')).toBeNull();
    });

    it('says nothing when the master shows nothing', async () => {
      const el = await render();
      expect(el.querySelector('app-puzzle-notice section')).toBeNull();
    });

    it('reads the list again when the stream says a puzzle changed: the notice appears live', async () => {
      const fixture = TestBed.createComponent(LiveSession);
      const el = await settle(fixture);
      expect(el.querySelector('app-puzzle-notice section')).toBeNull();
      puzzles.shownResult = [summary(selo)];
      source.push({ kind: 'puzzleChanged', puzzleId: 'a' });
      await settle(fixture);
      expect(el.querySelector('app-puzzle-notice section')).not.toBeNull();
    });

    it("opens the puzzle in the board's place when the address names it, with the way back to the session", async () => {
      puzzles.shownResult = [summary(selo)];
      puzzles.playerRunResult = playerRun(selo, { clue: 'Só o selo apagado abre o caminho.' });
      query.next(convertToParamMap({ puzzle: 'a' }));
      const el = await render();
      expect(el.querySelector('app-puzzle-play h1')?.textContent).toBe('O selo da Capela');
      expect(el.querySelector('app-puzzle-play a.back')?.textContent).toContain(
        'Voltar para a sessão',
      );
      // The session's own header and board are not drawn under it.
      expect(el.querySelector('app-session-header')).toBeNull();
      expect(el.querySelector('app-puzzle-notice')).toBeNull();
      // The session's own notices stay above the puzzle.
      expect(
        el.querySelector('app-puzzle-play')?.previousElementSibling?.tagName.toLowerCase(),
      ).toBeDefined();
      expect(el.querySelector('app-live-toast')).not.toBeNull();
      expect(el.querySelector('app-clue-notice')).not.toBeNull();
    });

    it('gives the master the panel with each puzzle and where it stands, and a live card for the shown one', async () => {
      source.campaign = {
        name: 'Mirathel',
        isMaster: true,
        awaitingApproval: false,
        diceMode: 1,
        dicePreference: 1,
      };
      puzzles.sessionResult = [
        masterRun(selo, PuzzleRunStatus.SHOWN),
        masterRun(lightsPuzzle('b', 'O cofre'), PuzzleRunStatus.NOT_SHOWN),
      ];
      const el = await render();
      const panel = el.querySelector('app-master-puzzles')!;
      expect(panel.textContent).toContain('O selo da Capela');
      expect(panel.textContent).toContain('Mostrar aos jogadores');
      // The live card is in the main column, not in the rail: one at a time, the shown one chosen.
      expect(panel.querySelector('app-master-run')).toBeNull();
      expect(el.querySelector('.board__left app-master-live app-master-run h3')?.textContent).toBe(
        'O selo da Capela',
      );
      // A master never gets the player's way in.
      expect(el.querySelector('app-puzzle-notice')).toBeNull();
    });

    it('leaves the board without a puzzle panel when the campaign has no puzzles', async () => {
      source.campaign = {
        name: 'Mirathel',
        isMaster: true,
        awaitingApproval: false,
        diceMode: 1,
        dicePreference: 1,
      };
      const el = await render();
      expect(el.querySelector('app-master-puzzles')).toBeNull();
    });
  });

  it('applies a vitals change from the stream without a reload', async () => {
    source.events = [
      { kind: 'ready' },
      { kind: 'vitals', vitals: pensantusVitals({ revision: 2, hitPointsCurrent: 12 }) },
    ];
    const el = await render();
    expect(el.querySelector('.hp__current')?.textContent).toBe('12');
  });

  it('shows the master the party with "Ajustar" per character, and the session link (E5-04)', async () => {
    source.campaign = {
      name: 'Mirathel',
      isMaster: true,
      awaitingApproval: false,
      diceMode: 1,
      dicePreference: 1,
    };
    source.snapshot = {
      session: { sessionId: 's4', sessionNumber: 4, startedAt: new Date(2026, 8, 30, 20, 5) },
      vitals: [pensantusVitals(), brisaVitals()],
      currentMapId: null,
      shownImage: null,
      shownImageKeep: false,
    };
    const el = await render();
    expect(el.textContent).toContain('Em andamento desde 30/09 às 20:05');
    expect(el.querySelector('[aria-label="Ajustar Pensantus"]')).not.toBeNull();
    expect(el.querySelector('[aria-label="Ajustar Brisa"]')).not.toBeNull();
    expect(el.textContent).toContain('Ladina 3, de Ana');
    expect(el.textContent).toContain('+5 temporários');
    expect(el.textContent).toContain('Abaixo da metade');
    expect(el.querySelector<HTMLInputElement>('#session-link')?.value).toBe(
      `${location.origin}/campaigns/mirathel/session`,
    );
    expect(el.textContent).toContain('Copiar link da sessão');
    expect(el.textContent).toContain('Encerrar sessão');
  });

  describe("the master's rests (PM-07b 9)", () => {
    const asMaster = () => {
      source.campaign = {
        name: 'Mirathel',
        isMaster: true,
        awaitingApproval: false,
        diceMode: 1,
        dicePreference: 1,
      };
      source.snapshot = {
        session: { sessionId: 's4', sessionNumber: 4, startedAt: new Date(2026, 8, 30, 20, 5) },
        vitals: [pensantusVitals(), brisaVitals()],
        currentMapId: null,
        shownImage: null,
        shownImageKeep: false,
      };
    };

    it('gives the master the "Descanso" card with the two rests', async () => {
      asMaster();
      const el = await render();
      expect(el.querySelector('app-rest-card h2')?.textContent).toBe('Descanso');
      expect(button(el, 'Descanso curto')).toBeTruthy();
      expect(button(el, 'Descanso longo')).toBeTruthy();
    });

    it('never gives a player the rests', async () => {
      const el = await render();
      expect(el.querySelector('app-rest-card')).toBeNull();
      expect(el.textContent).not.toContain('Descanso longo');
    });

    it('gives the player "Gastar dados de vida" on their card', async () => {
      const el = await render();
      expect(button(el, 'Gastar dados de vida')).toBeTruthy();
    });

    it("does not put the player's button on the master's party rows", async () => {
      asMaster();
      const el = await render();
      expect(button(el, 'Gastar dados de vida')).toBeUndefined();
    });

    it('shows the party as the rest left it, without waiting for the stream', async () => {
      asMaster();
      resources.restPreview.mockResolvedValue(
        restPreviewResponse([restPreview({ characterId: 'pensantus', name: 'Pensantus' })]),
      );
      resources.takeRest.mockResolvedValue([
        create(CharacterVitalsSchema, {
          characterId: 'pensantus',
          name: 'Pensantus',
          hitPointsCurrent: 23,
          hitPointsMax: 23,
          revision: 9,
        }),
      ]);
      const fixture = TestBed.createComponent(LiveSession);
      const el = await settle(fixture);
      expect(el.querySelector('.member__current')?.textContent).toBe('17');
      button(el, 'Descanso longo').click();
      await settle(fixture);
      button(el, 'Descansar').click();
      await settle(fixture);
      expect(el.querySelector('.member__current')?.textContent).toBe('23');
      expect(el.textContent).toContain('Descanso longo feito.');
    });
  });

  it('says "Peça um convite ao mestre", without the name, to a non-member', async () => {
    source.campaign = new KindError('no-access');
    const el = await render();
    expect(el.querySelector('h1')?.textContent).toContain('Peça um convite ao mestre');
    expect(el.textContent).not.toContain('Mirathel');
  });

  it('stops asking, and says there is no access, when the server forbids the session snapshot', async () => {
    source.snapshot = new KindError('forbidden') as never;
    const getLiveSession = vi.spyOn(source, 'getLiveSession');
    const el = await render();
    expect(el.querySelector('h1')?.textContent).toContain('Peça um convite ao mestre');
    expect(getLiveSession).toHaveBeenCalledTimes(1);
  });

  it('stops asking, and offers "Tentar de novo", when the server calls the session snapshot request invalid', async () => {
    source.snapshot = new KindError('invalid') as never;
    const getLiveSession = vi.spyOn(source, 'getLiveSession');
    const el = await render();
    expect(el.textContent).toContain('Não foi possível abrir a sessão');
    expect(button(el, 'Tentar de novo')).toBeDefined();
    expect(getLiveSession).toHaveBeenCalledTimes(1);
  });

  it("forgets the spells it read when the table's content changes, and again on a reconnection", async () => {
    const forget = vi.spyOn(TestBed.inject(SpellCatalog), 'forget');
    const fixture = TestBed.createComponent(LiveSession);
    await settle(fixture);
    forget.mockClear();
    source.push({ kind: 'contentChanged' });
    await settle(fixture);
    expect(forget).toHaveBeenCalledWith('mirathel');
    forget.mockClear();
    source.push({ kind: 'ready' });
    await settle(fixture);
    expect(forget).toHaveBeenCalledWith('mirathel');
  });

  it("reads the player's sheet again when the table's content changes", async () => {
    const fixture = TestBed.createComponent(LiveSession);
    await settle(fixture);
    const page = flat(fixture) as unknown as {
      playerSheet(): PlayerSheetVm | null;
    };
    expect(page.playerSheet()?.armorClass).toBe(14);
    source.sheet = { ...source.sheet, armorClass: 16 };
    source.push({ kind: 'contentChanged' });
    await settle(fixture);
    expect(page.playerSheet()?.armorClass).toBe(16);
  });

  it('shows the armor class the combat uses while an Armadura Arcana lasts on the character', async () => {
    const mage = (status: OutsideCastStatus) =>
      create(OutsideCastSchema, {
        spellKey: 'spell:mage-armor',
        status,
        targets: [
          create(OutsideCastTargetSchema, {
            characterId: 'pensantus',
            effect: CastEffect.ARMOR_CLASS,
            armorClass: 16,
          }),
        ],
      });
    casting.active = [mage(OutsideCastStatus.ACTIVE)];
    const fixture = TestBed.createComponent(LiveSession);
    await settle(fixture);
    const page = flat(fixture) as unknown as { ownSheet(): PlayerSheetVm | null };
    expect(page.ownSheet()?.armorClass).toBe(16);
    casting.active = [mage(OutsideCastStatus.ENDED)];
    source.push({ kind: 'spellCastsChanged' });
    await settle(fixture);
    expect(page.ownSheet()?.armorClass).toBe(14);
  });

  it('says the same to a pending member (RN-15)', async () => {
    source.campaign = {
      name: 'Mirathel',
      isMaster: false,
      awaitingApproval: true,
      diceMode: 1,
      dicePreference: 1,
    };
    const el = await render();
    expect(el.querySelector('h1')?.textContent).toContain('Peça um convite ao mestre');
    expect(el.textContent).not.toContain('Mirathel');
  });

  it('says "Nenhuma sessão em andamento", with the name, when none is open', async () => {
    source.events = [];
    source.failWith = new KindError('no-session');
    const el = await render();
    expect(el.querySelector('h1')?.textContent).toContain('Nenhuma sessão em andamento');
    expect(el.textContent).toContain('Mirathel');
    expect(el.textContent).toContain('Voltar para a campanha');
  });

  it('shows "Sessão 4 encerrada" when the stream says the session ended', async () => {
    const fixture = TestBed.createComponent(LiveSession);
    const el = await settle(fixture);
    expect(el.querySelector('h1')?.textContent).toContain('Sessão 4');

    source.push({ kind: 'ended' });
    await settle(fixture);
    expect(el.querySelector('h1')?.textContent).toContain('Sessão 4 encerrada');
    expect(el.textContent).toContain('A sessão acabou.');
    expect(openSessions.refresh).toHaveBeenCalled();
  });

  it('reads the summary of the session that ended and shows the player the card "A sessão acabou" (MR-032)', async () => {
    summary.mockResolvedValue(
      create(SessionSummarySchema, { duration: { seconds: 3600n, nanos: 0 } }),
    );
    const fixture = TestBed.createComponent(LiveSession);
    const el = await settle(fixture);
    source.push({ kind: 'ended' });
    await settle(fixture);
    expect(summary).toHaveBeenCalledWith('mirathel', 's4');
    expect(el.querySelector('h2.card__title')?.textContent).toBe('A sessão acabou');
    expect(el.querySelector('app-session-blocked')).toBeNull();
  });

  it("names a dead character's player's own result by the character, which has no vitals any more", async () => {
    source.snapshot = { ...(source.snapshot as LiveSnapshotVm), vitals: [] };
    const fixture = TestBed.createComponent(LiveSession);
    await settle(fixture);
    const page = flat(fixture) as unknown as {
      combat: { apply(e: Encounter): void };
      ownCharacterName(): string;
      highlightsCharacterId(): string;
    };
    expect(page.ownCharacterName()).toBe('');
    page.combat.apply(
      encounter({
        status: EncounterStatus.ENDED,
        combatants: [
          combatant({
            id: 'pen',
            label: 'Pensantus',
            kind: CombatantKind.PLAYER,
            characterId: 'pensantus',
            mine: true,
            state: CombatantState.DEAD,
            defeated: true,
          }),
        ],
      }),
    );
    expect(page.ownCharacterName()).toBe('Pensantus');
    expect(page.highlightsCharacterId()).toBe('pensantus');
  });

  it('lands the master on "Sessão encerrada" after confirming the end (MR-032)', async () => {
    source.campaign = {
      name: 'Mirathel',
      isMaster: true,
      awaitingApproval: false,
      diceMode: 1,
      dicePreference: 1,
    };
    summary.mockResolvedValue(
      create(SessionSummarySchema, {
        combats: 1,
        scenesOpened: 3,
        checksPassed: 9,
        checksTried: 12,
      }),
    );
    const fixture = TestBed.createComponent(LiveSession);
    const el = await settle(fixture);
    button(el, 'Encerrar sessão').click();
    await settle(fixture);
    button(el, 'Confirmar encerramento').click();
    await settle(fixture);
    expect(el.querySelector('#se-title')?.textContent).toBe('Sessão encerrada');
    expect(
      Array.from(el.querySelectorAll('.stat dd'), (d) => d.textContent?.replace(/\u00a0/g, ' ')),
    ).toEqual(['menos de 1 min', '1', '3', '9 de 12']);
    expect(el.querySelectorAll('button')).toHaveLength(0);
    expect(el.querySelector('.end__leave')?.textContent).toContain('Voltar à campanha');
  });

  it('the master ends the session after confirming in place', async () => {
    source.campaign = {
      name: 'Mirathel',
      isMaster: true,
      awaitingApproval: false,
      diceMode: 1,
      dicePreference: 1,
    };
    const fixture = TestBed.createComponent(LiveSession);
    const el = await settle(fixture);

    button(el, 'Encerrar sessão').click();
    await settle(fixture);
    expect(source.endSession).not.toHaveBeenCalled();
    button(el, 'Confirmar encerramento').click();
    await settle(fixture);

    expect(source.endSession).toHaveBeenCalledWith('mirathel', 's4');
    expect(el.querySelector('h1')?.textContent).toContain('Sessão 4 encerrada');
  });

  it('opens the session by itself when the master starts it (the RN-06 poll sees it)', async () => {
    source.events = [];
    source.failWith = new KindError('no-session');
    const fixture = TestBed.createComponent(LiveSession);
    const el = await settle(fixture);
    expect(el.querySelector('h1')?.textContent).toContain('Nenhuma sessão em andamento');

    source.events = [{ kind: 'ready' }];
    source.failWith = null;
    liveCampaignIds.set(new Set(['mirathel']));
    await settle(fixture);
    expect(el.querySelector('h1')?.textContent).toContain('Sessão 4');
    expect(el.querySelector('.hp__current')?.textContent).toBe('17');
  });

  it('sends a signed-out person to sign in and back', async () => {
    source.events = [];
    source.failWith = new KindError('signed-out');
    await render();
    expect(signIn).toHaveBeenCalled();
  });

  describe('the map and the image on show (MR-012, MR-028)', () => {
    let maps: FakeMapsClient;

    beforeEach(() => {
      maps = TestBed.inject(MapsClient) as unknown as FakeMapsClient;
      const map = mapMessage('map-1', 'Mirathel e arredores', { revealed: true, current: true });
      maps.responses.set(
        'map-1',
        mapResponse(
          map,
          [mapPoint('p1', 'Taverna do Javali', { revealed: true })],
          [mapToken('pensantus', 'Pensantus', { mine: true, xBp: 5200, yBp: 5400 })],
        ),
      );
      maps.maps = [map];
      source.snapshot = {
        ...(source.snapshot as LiveSnapshotVm),
        currentMapId: 'map-1',
      };
    });

    it('draws the current map from the snapshot, with the party and a link to the full map', async () => {
      const el = await render();
      expect(el.querySelector('#session-map-heading')?.textContent).toContain(
        'Mirathel e arredores',
      );
      expect(maps.calls).toContain('get map-1');
      expect(
        el.querySelector('[role="img"][aria-label="Prévia do mapa Mirathel e arredores"]'),
      ).not.toBeNull();
      expect(el.textContent).toContain('Pensantus');
      expect(el.textContent).toContain('(você)');
      expect(el.textContent).toContain('Ver mapa');
      expect(el.textContent).not.toContain('O mestre ainda não escolheu um mapa.');
    });

    it('moves a token from the stream without reading the map again', async () => {
      const el = await render();
      const gets = maps.calls.filter((c) => c.startsWith('get')).length;
      source.push({
        kind: 'tokenMoved',
        mapId: 'map-1',
        characterId: 'pensantus',
        xBp: 6200,
        yBp: 5400,
      });
      await nextTurn();
      TestBed.inject(ApplicationRef).tick();
      expect(el.querySelector('app-map-token')?.getAttribute('style')).toContain('left: 62%');
      expect(maps.calls.filter((c) => c.startsWith('get')).length).toBe(gets);
    });

    it('reads the map again on map_changed, and leaves it when the player lost sight of it', async () => {
      const el = await render();
      const gets = maps.calls.filter((c) => c.startsWith('get')).length;
      source.push({ kind: 'mapChanged', mapId: 'map-1' });
      await nextTurn();
      expect(maps.calls.filter((c) => c.startsWith('get')).length).toBe(gets + 1);

      maps.responses.delete('map-1'); // not_found: the map is hidden from them now
      source.push({ kind: 'mapChanged', mapId: 'map-1' });
      await nextTurn();
      TestBed.inject(ApplicationRef).tick();
      expect(el.textContent).toContain('O mestre ainda não escolheu um mapa.');
    });

    it('follows current_map_changed to another map, or to none', async () => {
      const el = await render();
      const other = mapMessage('map-2', 'Torre de Mirathel', { revealed: true, current: true });
      maps.responses.set('map-2', mapResponse(other));
      source.push({ kind: 'currentMap', mapId: 'map-2' });
      await nextTurn();
      TestBed.inject(ApplicationRef).tick();
      expect(el.querySelector('#session-map-heading')?.textContent).toContain('Torre de Mirathel');
      source.push({ kind: 'currentMap', mapId: null });
      await nextTurn();
      TestBed.inject(ApplicationRef).tick();
      expect(el.textContent).toContain('O mestre ainda não escolheu um mapa.');
    });

    it('shows the player the image on show, announces it, and takes it away when it stops', async () => {
      const el = await render();
      expect(el.textContent).not.toContain('O mestre está mostrando');
      const image = {
        id: 'img-1',
        name: 'Capitão Goblin',
        width: 400,
        height: 500,
        url: '/images/img-1',
      };
      source.push({ kind: 'shownImage', image });
      await nextTurn();
      TestBed.inject(ApplicationRef).tick();
      expect(el.querySelector('#shown-title')?.textContent).toContain('O mestre está mostrando');
      expect(el.querySelector('.block__name')?.textContent).toContain('Capitão Goblin');
      expect(el.querySelector('img[alt="Capitão Goblin"]')?.getAttribute('src')).toBe(
        '/images/img-1',
      );
      expect(el.querySelector('[role="status"]:not(.status)')).not.toBeNull();
      expect(el.textContent).toContain('O mestre está mostrando Capitão Goblin.');

      source.push({ kind: 'shownImage', image: null });
      await nextTurn();
      TestBed.inject(ApplicationRef).tick();
      expect(el.textContent).toContain('O mestre parou de mostrar a imagem.');
    });

    it('shows the player the images the master left, live, with no announcement (E6-25b)', async () => {
      const tower = {
        id: 'img-2',
        name: 'Planta da torre',
        width: 800,
        height: 600,
        url: '/images/img-2',
      };
      source.left = [tower];
      const el = await render();
      expect(el.querySelector('#left-title')).not.toBeNull();
      expect(el.querySelector('.row__name')?.textContent).toContain('Planta da torre');
      expect(el.querySelector('.row__thumb')?.getAttribute('src')).toBe('/images/img-2/thumb');
      expect(
        el.querySelector('button[aria-label="Ver Planta da torre em tela cheia"]'),
      ).not.toBeNull();
      expect(el.querySelector('.block__note')?.textContent).toContain(
        'Ficam aqui até o mestre tirar.',
      );

      source.left = [];
      source.push({ kind: 'leftImages' });
      await nextTurn();
      TestBed.inject(ApplicationRef).tick();
      expect(el.querySelector('#left-title')).toBeNull();
      expect(el.querySelector('[role="status"]:not(.status)')?.textContent?.trim() ?? '').toBe('');
    });

    describe('two reads of the images left with the players in flight', () => {
      const tower = {
        id: 'img-2',
        name: 'Planta da torre',
        width: 800,
        height: 600,
        url: '/images/img-2',
      };

      function deferred() {
        let resolve!: (v: ShownImageVm[]) => void;
        const promise = new Promise<ShownImageVm[]>((r) => (resolve = r));
        return { promise, resolve };
      }

      /** Two reads start (the master took the image back in between); they answer in the order given. */
      async function twoReads(newerFirst: boolean) {
        source.left = [tower];
        const el = await render();
        expect(el.querySelector('#left-title')).not.toBeNull();
        const older = deferred();
        const newer = deferred();
        source.listLeftImages
          .mockReset()
          .mockReturnValueOnce(older.promise)
          .mockReturnValueOnce(newer.promise);
        source.push({ kind: 'leftImages' });
        await nextTurn();
        source.push({ kind: 'leftImages' });
        await nextTurn();
        expect(source.listLeftImages).toHaveBeenCalledTimes(2);
        const answers = [() => older.resolve([tower]), () => newer.resolve([])];
        for (const answer of newerFirst ? answers.reverse() : answers) {
          answer();
          await nextTurn();
        }
        TestBed.inject(ApplicationRef).tick();
        return el;
      }

      it('leaves a taken-back image hidden when the replies arrive in order', async () => {
        const el = await twoReads(false);
        expect(el.querySelector('#left-title')).toBeNull();
      });

      it('keeps a taken-back image hidden when the older reply arrives last', async () => {
        const el = await twoReads(true);
        expect(el.querySelector('#left-title')).toBeNull();
      });
    });

    it('draws the block without announcing it when it comes with the snapshot (a reload)', async () => {
      source.snapshot = {
        ...(source.snapshot as LiveSnapshotVm),
        shownImage: { id: 'img-1', name: 'Carta', width: 800, height: 500, url: '/images/img-1' },
      };
      const el = await render();
      expect(el.querySelector('.block__name')?.textContent).toContain('Carta');
      expect(el.textContent).not.toContain('O mestre está mostrando Carta.');
    });
  });

  describe('RP scene (MR-015, E7-02, E7-03)', () => {
    async function tick(): Promise<void> {
      await nextTurn();
      TestBed.inject(ApplicationRef).tick();
    }

    it('draws a scene that was already open with the snapshot, with no announcement and no focus move', async () => {
      scenes.scene = playerScene();
      const el = await render();
      expect(el.querySelector('#sc-title')?.textContent).toBe('Cena: A\u00a0carroça tombada');
      expect(el.textContent).not.toContain('O mestre abriu uma cena: A carroça tombada.');
      expect(document.activeElement).not.toBe(el.querySelector('#sc-title'));
      expect(scenes.calls).toContain('get');
    });

    it('shows nothing extra while no scene is open', async () => {
      const el = await render();
      expect(el.querySelector('app-scene-player')?.textContent?.trim()).toBe('');
      expect(el.textContent).not.toContain('Nenhuma cena');
    });

    it('reads the scene again on scene_changed, announces the title, and says when the master closes it', async () => {
      const el = await render();
      scenes.scene = playerScene();
      source.push({ kind: 'sceneChanged' });
      await tick();
      await tick();
      expect(el.querySelector('#sc-title')?.textContent).toBe('Cena: A\u00a0carroça tombada');
      expect(el.textContent).toContain('O mestre abriu uma cena: A carroça tombada.');
      // The page does not take focus from where the player is.
      expect(document.activeElement).not.toBe(el.querySelector('#sc-title'));

      scenes.scene = null;
      source.push({ kind: 'sceneChanged' });
      await tick();
      await tick();
      expect(el.querySelector('#sc-title')).toBeNull();
      expect(el.textContent).toContain('O mestre fechou a cena.');
    });

    it("turns the player's row into the result when scene_check_rolled arrives for their own roll", async () => {
      scenes.scene = playerScene();
      const el = await render();
      expect(el.querySelectorAll('.sc__roll')).toHaveLength(5);
      scenes.scene = playerScene([sceneRoll('r1', 'a1', 'Pensantus', 17)], [], {
        actions: playerScene().actions.map((a) => (a.id === 'a1' ? { ...a, attemptsLeft: 0 } : a)),
      });
      source.push({ kind: 'sceneCheckRolled' });
      await tick();
      await tick();
      expect(el.querySelectorAll('.sc__roll')).toHaveLength(4);
      expect(el.querySelector('.sc__done')?.textContent).toContain('Rolada');
    });

    describe('the master', () => {
      beforeEach(() => {
        source.campaign = {
          name: 'Mirathel',
          isMaster: true,
          awaitingApproval: false,
          diceMode: 1,
          dicePreference: 1,
        };
        source.snapshot = {
          session: { sessionId: 's4', sessionNumber: 4, startedAt: new Date(2026, 8, 30, 20, 5) },
          vitals: [pensantusVitals(), brisaVitals()],
          currentMapId: 'map-1',
          shownImage: null,
          shownImageKeep: false,
        };
        const maps = TestBed.inject(MapsClient) as unknown as FakeMapsClient;
        maps.responses.set(
          'map-1',
          mapResponse(mapMessage('map-1', 'Estrada do Vale', { revealed: true, current: true }), [
            mapPoint('p1', 'A carroça tombada'),
          ]),
        );
      });

      it('has "Cena de RP" with "Abrir cena" while none is open, and no open-scene block', async () => {
        const el = await render();
        expect(el.querySelector('app-scene-panel h2')?.textContent).toBe('Cena de RP');
        expect(el.textContent).toContain('Nenhuma cena aberta.');
        expect(el.querySelector('app-scene-open')).toBeNull();
        expect(el.querySelector('.board--scene-open')).toBeNull();
      });

      it('moves the open scene to the top of the map column and takes "Cena de RP" out of the right one', async () => {
        scenes.scene = masterScene([sceneRoll('r1', 'a2', 'Toren', 7, { passed: false })]);
        const el = await render();
        expect(el.querySelector('app-scene-panel')).toBeNull();
        expect(el.querySelector('.board--scene-open')).not.toBeNull();
        const left = el.querySelector('.board__left')!;
        expect(left.firstElementChild?.tagName.toLowerCase()).toBe('app-scene-open');
        expect(left.querySelector('app-session-map')).not.toBeNull();
        expect(el.querySelector('#so-title')?.textContent).toBe('Cena: A\u00a0carroça tombada');
        expect(el.textContent).toContain('Não passou');
      });

      it('reads the rolls again on scene_check_rolled and reads the new one aloud', async () => {
        scenes.scene = masterScene();
        const el = await render();
        expect(el.textContent).toContain('Ninguém rolou ainda.');
        scenes.scene = masterScene([sceneRoll('r1', 'a2', 'Toren', 7, { passed: false })]);
        source.push({ kind: 'sceneCheckRolled' });
        await tick();
        await tick();
        expect(el.textContent).toContain('Toren: Seguir os rastros dos goblins, 7, não passou');
        expect(el.querySelectorAll('app-scene-roll-line')).toHaveLength(1);
      });

      it('opens a scene from its point in "Pontos do mapa", and the list says it is the open one', async () => {
        const maps = TestBed.inject(MapsClient) as unknown as FakeMapsClient;
        maps.responses.set(
          'map-1',
          mapResponse(mapMessage('map-1', 'Estrada do Vale', { revealed: true, current: true }), [
            mapPoint('p1', 'A carroça tombada', {
              revealed: true,
              sceneActions: [create(SceneActionSchema, { id: 'a1', key: 'skill:arcana' })],
            }),
          ]),
        );
        scenes.scene = masterScene();
        const el = await render();
        // A scene is already open: the point says so.
        expect(el.querySelector('.row__open')?.textContent).toContain('Cena aberta agora');
        scenes.scene = null;
        source.push({ kind: 'sceneChanged' });
        await tick();
        await tick();
        const open = el.querySelector<HTMLButtonElement>(
          'button[aria-label="Abrir cena A carroça tombada"]',
        )!;
        scenes.scene = masterScene();
        open.click();
        await tick();
        await tick();
        expect(scenes.calls).toContain('open p1');
        expect(el.querySelector('app-scene-open')).not.toBeNull();
        expect(document.activeElement).toBe(el.querySelector('#so-title'));
      });

      it('brings "Cena de RP" back when the scene closes from another tab', async () => {
        scenes.scene = masterScene();
        const el = await render();
        scenes.scene = null;
        source.push({ kind: 'sceneChanged' });
        await tick();
        await tick();
        expect(el.querySelector('app-scene-open')).toBeNull();
        expect(el.querySelector('app-scene-panel')).not.toBeNull();
      });
    });
  });

  describe('the snapshot against the stream', () => {
    const imageA: ShownImageVm = {
      id: 'img-1',
      name: 'Carta',
      width: 800,
      height: 500,
      url: '/images/img-1',
    };
    // The component's state is protected; the test reads it to state what the player ends up with.
    const state = (fixture: ComponentFixture<LiveSession>) =>
      flat(fixture) as unknown as {
        shownImage(): ShownImageVm | null;
        currentMapId(): string | null;
      };

    /** Holds the snapshot read until the test lets it go. */
    function deferSnapshot(): (snapshot: LiveSnapshotVm) => void {
      let release!: (snapshot: LiveSnapshotVm) => void;
      const pending = new Promise<LiveSnapshotVm>((resolve) => (release = resolve));
      source.getLiveSession = () => pending;
      return release;
    }

    /** With reduced motion the shown-image block leaves at once (no fade timer). */
    function reducedMotion(): void {
      vi.stubGlobal('matchMedia', (query: string) => ({
        matches: query.includes('reduce'),
        media: query,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
      }));
    }

    afterEach(() => vi.unstubAllGlobals());

    it('shows an image the snapshot carries and drops it on the stop event after it', async () => {
      reducedMotion();
      const release = deferSnapshot();
      const fixture = TestBed.createComponent(LiveSession);
      const el = fixture.nativeElement as HTMLElement;
      await settle(fixture);
      release({ ...(source.snapshot as LiveSnapshotVm), shownImage: imageA });
      await settle(fixture);
      expect(el.querySelector('.block__name')?.textContent).toContain('Carta');
      source.push({ kind: 'shownImage', image: null });
      await settle(fixture);
      expect(el.querySelector('.block__name')).toBeNull();
    });

    it('does not show again an image the master withdrew while the snapshot was in flight', async () => {
      reducedMotion();
      const release = deferSnapshot();
      const fixture = TestBed.createComponent(LiveSession);
      const el = fixture.nativeElement as HTMLElement;
      await settle(fixture); // `ready` came; the snapshot is still being read
      source.push({ kind: 'shownImage', image: null });
      await settle(fixture);
      // The snapshot, read before the stop, now answers with the image.
      release({ ...(source.snapshot as LiveSnapshotVm), shownImage: imageA });
      await settle(fixture);
      expect(el.querySelector('h1')?.textContent).toContain('Sessão 4');
      expect(state(fixture).shownImage()).toBeNull();
      expect(el.querySelector('.block__name')).toBeNull();
    });

    it('stays on the map the master moved the table to while the snapshot was in flight', async () => {
      const maps = TestBed.inject(MapsClient) as unknown as FakeMapsClient;
      const a = mapMessage('map-a', 'Mapa A', { revealed: true, current: true });
      const b = mapMessage('map-b', 'Mapa B', { revealed: true, current: true });
      maps.maps = [a, b];
      maps.responses.set('map-a', mapResponse(a));
      maps.responses.set('map-b', mapResponse(b));
      const release = deferSnapshot();
      const fixture = TestBed.createComponent(LiveSession);
      await settle(fixture);
      source.push({ kind: 'currentMap', mapId: 'map-b' });
      await settle(fixture);
      release({ ...(source.snapshot as LiveSnapshotVm), currentMapId: 'map-a' });
      const el = await settle(fixture);
      expect(state(fixture).currentMapId()).toBe('map-b');
      expect(el.querySelector('#session-map-heading')?.textContent).toContain('Mapa B');
    });
  });

  describe('waiting for a session that is not open', () => {
    it('does not open the stream again and again while the list still names the campaign', async () => {
      source.events = [];
      source.failWith = new KindError('no-session');
      const watch = vi.spyOn(source, 'watch');
      const getCampaign = vi.spyOn(source, 'getCampaign');
      liveCampaignIds.set(new Set(['mirathel'])); // stale: the session already ended
      const fixture = TestBed.createComponent(LiveSession);
      for (let i = 0; i < 20; i++) {
        await settle(fixture);
        TestBed.tick();
      }
      expect(watch.mock.calls.length).toBeLessThanOrEqual(2);
      expect(getCampaign.mock.calls.length).toBeLessThanOrEqual(2);
    });

    it('opens by itself when the campaign shows up in the list again after it left', async () => {
      source.events = [];
      source.failWith = new KindError('no-session');
      liveCampaignIds.set(new Set(['mirathel']));
      const fixture = TestBed.createComponent(LiveSession);
      const el = await settle(fixture);
      expect(el.querySelector('h1')?.textContent).toContain('Nenhuma sessão em andamento');

      liveCampaignIds.set(new Set());
      await settle(fixture);
      source.events = [{ kind: 'ready' }];
      source.failWith = null;
      liveCampaignIds.set(new Set(['mirathel']));
      await settle(fixture);
      expect(el.querySelector('h1')?.textContent).toContain('Sessão 4');
    });
  });

  describe('the treasure card after an XP change', () => {
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
    const card = (el: HTMLElement) => el.querySelector('app-treasure-card')?.textContent ?? '';

    it('shows the treasure as converted after xp_changed when the server now has it converted', async () => {
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
      const fixture = TestBed.createComponent(LiveSession);
      const el = await settle(fixture);
      expect(card(el)).toContain('Desmarcar');
      expect(card(el)).not.toContain('Convertido em XP');

      maps.responses.set('map-1', mapResponse(map, [treasure(true)]));
      source.push({ kind: 'xpChanged' });
      await settle(fixture);
      expect(card(el)).toContain('Convertido em XP');
      expect(card(el)).not.toContain('Desmarcar');
    });
  });

  describe('the fog of war (MR-036, RN-10, E9-03)', () => {
    let maps: FakeMapsClient;
    const tick = async () => {
      await nextTurn();
      TestBed.inject(ApplicationRef).tick();
    };
    const tile = (tx: number, ty: number, revision: number) => ({
      $typeName: 'meurpg.maps.v1.MapTile' as const,
      tx,
      ty,
      revision,
    });

    beforeEach(() => {
      maps = TestBed.inject(MapsClient) as unknown as FakeMapsClient;
      const map = mapMessage('map-1', 'A caverna do Vale Seco', {
        revealed: true,
        current: true,
        fogEnabled: true,
        gridColumns: 4,
        gridRows: 4,
        // A player gets no picture of a fog map: only its size.
        image: {
          $typeName: 'meurpg.maps.v1.MapImage',
          id: '',
          url: '',
          thumbnailUrl: '',
          width: 960,
          height: 640,
          name: '',
        },
        imageWithheld: true,
      });
      maps.maps = [map];
      maps.responses.set(
        'map-1',
        mapResponse(
          map,
          [],
          [
            mapToken('pensantus', 'Pensantus', { mine: true, xBp: 1250, yBp: 6250 }),
            mapToken('brisa', 'Brisa', { xBp: 3750, yBp: 6250 }),
          ],
        ),
      );
      maps.visions.set(
        '',
        visionResponse(['....', 'gBd.', '..r.', '....'], {
          tilesPath: '/images/maps/map-1/tiles/',
          tileSquares: 16,
          tiles: [tile(0, 0, 4)],
        }),
      );
      source.snapshot = { ...(source.snapshot as LiveSnapshotVm), currentMapId: 'map-1' };
    });

    it('gives a player the fog map in place: the vision, the layers and the tiles are read, and the image never is', async () => {
      const el = await render();
      expect(maps.calls).toContain('vision map-1 ');
      expect(maps.calls).toContain('layers map-1 ');
      expect(el.querySelector('app-fog-map')).not.toBeNull();
      expect(el.querySelector('app-fog-base')?.getAttribute('data-tiles')).toBe('1');
      const sources = Array.from(el.querySelectorAll('img'), (i) => i.getAttribute('src') ?? '');
      expect(sources).toEqual(['/images/maps/map-1/tiles/0/0?r=4']);
      // The old preview and its "Ver mapa" are not on a fog map.
      expect(el.querySelector('app-map-view')?.getAttribute('ng-reflect-mode')).not.toBe('preview');
      expect(el.textContent).not.toContain('Ver mapa');
      // Until the tile arrives the card waits; then it says what the character can use, not how many squares.
      expect(el.querySelector('.fm__caption')).toBeNull();
      el.querySelector('.fb__tile')!.dispatchEvent(new Event('load'));
      await tick();
      const card = textOf(el.querySelector('.fm__caption'));
      expect(card).toContain('Você vê');
      expect(card).toContain('Visão no escuro: 18 m');
      expect(card).toContain('Nenhum inimigo à vista.');
      expect(card).not.toMatch(/\d+ de \d+ quadrados/);
    });

    it('offers the player the row "Luz que você carrega" for their own character', async () => {
      const el = await render();
      expect(textOf(el.querySelector('app-carried-light'))).toBe(
        'lightbulb Luz que você carrega Nenhuma Mudar',
      );
    });

    it('reads the vision and the map again on vision_changed, and only for the current map', async () => {
      await render();
      const reads = () => maps.calls.filter((c) => c.startsWith('vision map-1 ')).length;
      const gets = () => maps.calls.filter((c) => c.startsWith('get map-1')).length;
      // Opening the page schedules one read of the vision; let it land so the counts below are only the hints'.
      await vi.waitFor(() => expect(reads()).toBe(2));
      const before = {
        reads: reads(),
        gets: gets(),
        layers: maps.calls.filter((c) => c.startsWith('layers map-1 ')).length,
      };
      // The page waits 120 ms before it reads, to join a burst of hints: a fake clock, so the burst is one on any machine.
      vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
      try {
        source.push({ kind: 'visionChanged', mapId: 'other-map' });
        await vi.advanceTimersByTimeAsync(1000);
        expect(reads()).toBe(before.reads);
        source.push({ kind: 'visionChanged', mapId: 'map-1' });
        source.push({
          kind: 'tokenMoved',
          mapId: 'map-1',
          characterId: 'pensantus',
          xBp: 6200,
          yBp: 5400,
        });
        source.push({ kind: 'visionChanged', mapId: 'map-1' });
        await vi.advanceTimersByTimeAsync(1000);
      } finally {
        vi.useRealTimers();
      }
      await vi.waitFor(async () => {
        await tick();
        expect(reads()).toBeGreaterThan(before.reads);
      });
      // Every hint costs one read of the vision (and the layers); a burst of them is one.
      expect(reads()).toBe(before.reads + 1);
      expect(maps.calls.filter((c) => c.startsWith('layers map-1 ')).length).toBe(
        before.layers + 1,
      );
      // The map itself is read per hint, joining one already on its way: one or two reads, never none and never one for
      // the other map's hint.
      expect(gets()).toBeGreaterThanOrEqual(before.gets + 1);
      expect(gets()).toBeLessThanOrEqual(before.gets + 2);
    });

    it('reads the combat again on vision_changed during a combat, so an NPC that just came into sight arrives', async () => {
      const get = vi.spyOn(TestBed.inject(CombatClient).turn, 'get').mockResolvedValue(null);
      const fixture = TestBed.createComponent(LiveSession);
      await settle(fixture);
      const page = flat(fixture) as unknown as { combat: CombatState };
      get.mockClear();
      // No combat on screen: a hint about the vision reads the map only.
      source.push({ kind: 'visionChanged', mapId: 'map-1' });
      await settle(fixture);
      expect(get).not.toHaveBeenCalled();
      page.combat.apply(encounter({ id: 'e1', combatants: [] }));
      source.push({ kind: 'visionChanged', mapId: 'other-map' });
      await settle(fixture);
      expect(get).not.toHaveBeenCalled();
      source.push({ kind: 'visionChanged', mapId: 'map-1' });
      await settle(fixture);
      expect(get).toHaveBeenCalledTimes(1);
    });

    it('says in words that the character is not on the map', async () => {
      maps.visions.set(
        '',
        visionResponse(['....', '..r.', '....', '....'], { characterOnMap: false }),
      );
      const el = await render();
      expect(textOf(el.querySelector('[data-testid="fog-off-map"]'))).toContain(
        'Seu personagem não está neste mapa.',
      );
    });

    it('keeps a map without fog as it was: the preview, with the image', async () => {
      const plain = mapMessage('map-1', 'Mirathel e arredores', { revealed: true, current: true });
      maps.responses.set(
        'map-1',
        mapResponse(plain, [], [mapToken('pensantus', 'Pensantus', { mine: true })]),
      );
      const el = await render();
      expect(el.querySelector('app-fog-map')).toBeNull();
      expect(maps.calls.some((c) => c.startsWith('vision'))).toBe(false);
      expect(
        el.querySelector('[role="img"][aria-label="Prévia do mapa Mirathel e arredores"]'),
      ).not.toBeNull();
    });

    it("shows the band while the player looks through the familiar's eyes, with its name and the way back", async () => {
      source.snapshot = {
        ...(source.snapshot as LiveSnapshotVm),
        vitals: [pensantusVitals({ familiarSight: { creatureId: 'nanquim', inCombat: false } })],
      };
      const el = await render();
      expect(textOf(el.querySelector('[data-testid="familiar-band"]'))).toBe(
        'visibility Você está vendo pelos olhos do Nanquim. Pensantus está cego e surdo.',
      );
      expect(button(el, 'Voltar aos seus olhos')).toBeTruthy();
    });

    describe('the master', () => {
      beforeEach(() => {
        source.campaign = {
          name: 'Mirathel',
          isMaster: true,
          awaitingApproval: false,
          diceMode: 1,
          dicePreference: 1,
        };
        source.snapshot = {
          ...(source.snapshot as LiveSnapshotVm),
          vitals: [pensantusVitals(), brisaVitals()],
        };
        maps.responses.set(
          'map-1',
          mapResponse(
            maps.maps[0],
            [],
            [
              mapToken('pensantus', 'Pensantus'),
              mapToken('brisa', 'Brisa'),
              mapToken('goblin', 'Goblin 1', { kind: 3 }),
            ],
          ),
        );
        maps.responses.set(
          'map-1@brisa',
          mapResponse(
            maps.maps[0],
            [],
            [mapToken('pensantus', 'Pensantus'), mapToken('brisa', 'Brisa', { mine: true })],
          ),
        );
        maps.visions.set('pensantus', visionResponse(['BBBB', '....', '....', '....']));
        maps.visions.set(
          'brisa',
          visionResponse(['dd..', '....', '....', '....'], {
            tilesPath: '/images/maps/map-1/tiles/',
            tileSquares: 16,
            tiles: [tile(0, 0, 2)],
          }),
        );
      });

      it('keeps his own map whole, and lists "Ver como" with the squares each character sees', async () => {
        const el = await render();
        // The counts come with the vision read, a little after the page opens: wait for them.
        await vi.waitFor(async () => {
          await tick();
          expect(textOf(el.querySelector('app-view-as-list'))).toContain(
            'Brisa Ana 2 quadrados vistos',
          );
        });
        // The same map component as the players', with the whole image, and his tokens to drag.
        expect(el.querySelector('app-fog-map')).not.toBeNull();
        expect(el.querySelector('app-fog-base img.fb__img')).not.toBeNull();
        expect(el.querySelector('app-view-as-map')).toBeNull();
        const rows = Array.from(el.querySelectorAll('app-view-as-list [role="radio"]'), (r) =>
          textOf(r),
        );
        expect(rows).toEqual([
          'Todos Sem névoa: o seu mapa de mestre',
          'Pensantus Vinicius 4 quadrados vistos',
          'Brisa Ana 2 quadrados vistos',
        ]);
        // Next to the map: the panel is the first block of the right column.
        expect(el.querySelector('.board__fog app-view-as-list')).not.toBeNull();
      });

      it('shows the map as one character sees it, with the band and the badge, and comes back with "Todos"', async () => {
        const el = await render();
        await vi.waitFor(async () => {
          await tick();
          expect(el.querySelector('app-view-as-list [data-character="brisa"]')).not.toBeNull();
        });
        (
          el.querySelector('app-view-as-list [data-character="brisa"]') as HTMLButtonElement
        ).click();
        await tick();
        await tick();
        await tick();
        expect(maps.calls).toContain('get map-1 brisa');
        expect(maps.calls).toContain('vision map-1 brisa');
        expect(maps.calls).toContain('layers map-1 brisa');
        expect(textOf(el.querySelector('app-view-as-map .band'))).toBe(
          'visibility Você está vendo o mapa como Brisa . Para voltar ao seu mapa, escolha “Todos”. Voltar ao seu mapa',
        );
        expect(el.querySelector('app-fog-base')?.querySelector('img')?.getAttribute('src')).toBe(
          '/images/maps/map-1/tiles/0/0?r=2&as=brisa',
        );
        expect(el.querySelector('app-map-view [data-item]')).toBeNull();

        // "Voltar ao seu mapa" is the way back; so is choosing "Todos".
        (el.querySelector('app-view-as-map .band__back') as HTMLButtonElement).click();
        await tick();
        expect(el.querySelector('app-view-as-map')).toBeNull();
        expect(
          el.querySelector('app-view-as-list [role="radio"][aria-checked="true"]')?.textContent,
        ).toContain('Todos');
      });

      it('has "Luz dos personagens" with a select for each player character on the map', async () => {
        const el = await render();
        expect(
          Array.from(el.querySelectorAll('app-light-panel .row__name'), (n) => textOf(n)),
        ).toEqual(['Pensantus', 'Brisa']);
      });
    });

    it('reads the vision and its layers again when the stream says ready a second time', async () => {
      const fixture = TestBed.createComponent(LiveSession);
      await settle(fixture);
      const count = (prefix: string) => maps.calls.filter((c) => c.startsWith(prefix)).length;
      await vi.waitFor(() => expect(count('vision map-1')).toBeGreaterThan(0));
      const before = {
        vision: count('vision map-1'),
        layers: count('layers map-1'),
        gets: count('get map-1'),
      };

      // What a reconnection delivers: a new `ready` on the same stream.
      source.push({ kind: 'ready' });
      await settle(fixture);
      await vi.waitFor(async () => {
        await settle(fixture);
        expect(count('vision map-1')).toBeGreaterThan(before.vision);
        expect(count('layers map-1')).toBeGreaterThan(before.layers);
      });
      expect(count('get map-1')).toBe(before.gets + 1);
    });
  });
  describe('Revivify outside a combat and a character that lives again', () => {
    const asMaster = () => {
      source.campaign = {
        name: 'Mirathel',
        isMaster: true,
        awaitingApproval: false,
        diceMode: 1,
        dicePreference: 1,
      };
    };
    const asked = create(RevivifyRequestSchema, {
      id: 'r1',
      status: RevivifyRequestStatus.PENDING,
      casterName: 'Ilaria',
      targetName: 'Toren',
    });

    it('asks the master on the first load, and again when the stream says the casts changed', async () => {
      asMaster();
      const el = await render();
      expect(revivifyList).toHaveBeenCalled();
      expect(el.querySelector('app-revivify-ask [role="alertdialog"]')).toBeNull();

      revivifyList.mockResolvedValue([asked]);
      const reads = revivifyList.mock.calls.length;
      source.push({ kind: 'revivifyChanged' });
      await nextTurn();
      await nextTurn();

      expect(revivifyList.mock.calls.length).toBeGreaterThan(reads);
    });

    it('shows the question to the master and never to a player', async () => {
      revivifyList.mockResolvedValue([asked]);
      const player = await render();
      expect(player.querySelector('app-revivify-ask')).toBeNull();
      expect(revivifyList).not.toHaveBeenCalled();
    });

    it('reads the vitals again when a character lives again', async () => {
      asMaster();
      await render();
      const reads = vi.spyOn(source, 'getLiveSession');
      source.push({ kind: 'characterRevived', characterId: 'brisa' });
      await nextTurn();
      await nextTurn();

      expect(reads).toHaveBeenCalled();
    });

    it('shows a player "Você voltou à vida" without moving the focus, and the master nothing', async () => {
      const el = await render();
      const notice = el.querySelector('app-revived-notice [role="status"]');
      expect(notice?.getAttribute('aria-live')).toBe('polite');
      expect(notice?.textContent?.trim()).toBe('');
      const focused = document.activeElement;

      source.push({ kind: 'characterRevived', characterId: 'pensantus' });
      await nextTurn();
      await nextTurn();
      TestBed.inject(ApplicationRef).tick();

      const said = (el.querySelector('app-revived-notice [role="status"]')?.textContent ?? '')
        .replace(/\s+/g, ' ')
        .trim();
      expect(said).toContain('Você voltou à vida. Está com 1 PV.');
      expect(said).not.toContain('iniciativa');
      expect(document.activeElement).toBe(focused);
    });

    it('shows the master no notice when a character lives again', async () => {
      asMaster();
      const el = await render();
      source.push({ kind: 'characterRevived', characterId: 'brisa' });
      await nextTurn();
      await nextTurn();
      expect(el.querySelector('app-revived-notice')).toBeNull();
    });
  });

  describe('the master calling (Chamar a atenção)', () => {
    const asMaster = () => {
      source.campaign = {
        name: 'Mirathel',
        isMaster: true,
        awaitingApproval: false,
        diceMode: 1,
        dicePreference: 1,
      };
    };
    const settle = async () => {
      await nextTurn();
      await nextTurn();
      TestBed.inject(ApplicationRef).tick();
    };

    it('alerts a player and shows "O mestre está chamando você." until it is dismissed', async () => {
      const nudge = vi.spyOn(TestBed.inject(TurnAlert), 'nudge');
      const el = await render();
      const live = el.querySelector('app-calling-notice [role="status"]');
      expect(live?.getAttribute('aria-live')).toBe('polite');
      expect(live?.textContent?.trim()).toBe('');

      source.push({ kind: 'callingYou' });
      await settle();

      expect(nudge).toHaveBeenCalledWith('master');
      expect(el.querySelector('app-calling-notice')?.textContent).toContain(
        'O mestre está chamando você.',
      );
      el.querySelector<HTMLButtonElement>('app-calling-notice button.close')!.click();
      await settle();
      expect(el.querySelector('app-calling-notice')?.textContent).not.toContain('chamando você');
    });

    it('goes away by itself after a few seconds', async () => {
      const el = await render();
      vi.useFakeTimers();
      try {
        source.push({ kind: 'callingYou' });
        await vi.advanceTimersByTimeAsync(0);
        TestBed.inject(ApplicationRef).tick();
        expect(el.querySelector('app-calling-notice')?.textContent).toContain('chamando você');
        await vi.advanceTimersByTimeAsync(CALLING_NOTICE_MS);
        TestBed.inject(ApplicationRef).tick();
        expect(el.querySelector('app-calling-notice')?.textContent).not.toContain('chamando você');
      } finally {
        vi.useRealTimers();
      }
    });

    it('never alerts the master', async () => {
      asMaster();
      const nudge = vi.spyOn(TestBed.inject(TurnAlert), 'nudge');
      const el = await render();
      source.push({ kind: 'callingYou' });
      await settle();
      expect(nudge).not.toHaveBeenCalled();
      expect(el.querySelector('app-calling-notice')).toBeNull();
    });
  });

  describe('the Revivificar button outside a combat', () => {
    it('is on the page of the player whose sheet has the spell ready, and opens the sheet for that character', async () => {
      source.sheet = { ...source.sheet, classes: 'Clérigo 5', revivify: true };
      const dialog = TestBed.inject(MatDialog);
      const open = vi
        .spyOn(dialog, 'open')
        .mockReturnValue({ afterClosed: () => of(undefined) } as never);
      const el = await render();

      button(el, 'Revivificar').click();

      expect(open).toHaveBeenCalledTimes(1);
      const config = open.mock.calls[0][1] as { data: Record<string, unknown> };
      expect(config.data).toMatchObject({
        campaignId: 'mirathel',
        casterName: 'Pensantus',
        classes: 'Clérigo 5',
        combat: null,
        casterCharacterId: 'pensantus',
      });
    });

    it('is not there for a character without the spell', async () => {
      const el = await render();
      expect(button(el, 'Revivificar')).toBeUndefined();
    });

    it('is not there for the master, even when the sheet has the spell', async () => {
      source.sheet = { ...source.sheet, revivify: true };
      source.campaign = {
        name: 'Mirathel',
        isMaster: true,
        awaitingApproval: false,
        diceMode: 1,
        dicePreference: 1,
      };
      const el = await render();
      expect(button(el, 'Revivificar')).toBeUndefined();
    });
  });
  describe('the reaction windows on the stream (PM-04)', () => {
    it('reads the combat again when a window opens, and keeps the sentence of one that closed by itself', async () => {
      const get = vi.spyOn(TestBed.inject(CombatClient).turn, 'get').mockResolvedValue(null);
      const fixture = TestBed.createComponent(LiveSession);
      await settle(fixture);
      const state = (flat(fixture) as unknown as { combat: CombatState }).combat;
      get.mockClear();

      source.push({ kind: 'reactionWindowOpened', encounterId: 'e1', windowId: 'w1' });
      await settle(fixture);
      expect(get).toHaveBeenCalledTimes(1);
      expect(state.wasAnnounced('w1')).toBe(true);

      source.push({
        kind: 'reactionWindowClosed',
        encounterId: 'e1',
        windowId: 'w1',
        closedByItself: true,
        text: 'Queda Suave fechou. Você já usou a sua reação.',
      });
      await settle(fixture);
      expect(state.reactionNotice()?.text).toBe('Queda Suave fechou. Você já usou a sua reação.');
      expect(get).toHaveBeenCalledTimes(2);
    });

    it('a window that was answered says nothing, and the combat is read again', async () => {
      const get = vi.spyOn(TestBed.inject(CombatClient).turn, 'get').mockResolvedValue(null);
      const fixture = TestBed.createComponent(LiveSession);
      await settle(fixture);
      const state = (flat(fixture) as unknown as { combat: CombatState }).combat;
      get.mockClear();
      source.push({
        kind: 'reactionWindowClosed',
        encounterId: 'e1',
        windowId: 'w1',
        closedByItself: false,
        text: '',
      });
      await settle(fixture);
      expect(state.reactionNotice()).toBeNull();
      expect(get).toHaveBeenCalledTimes(1);
    });
  });

  describe('the combat news on the stream (MR-013)', () => {
    const asMaster = () => {
      source.campaign = {
        name: 'Mirathel',
        isMaster: true,
        awaitingApproval: false,
        diceMode: 1,
        dicePreference: 1,
      };
    };
    const running = (over: Partial<Encounter> = {}) =>
      encounter({
        id: 'e1',
        revision: 5,
        currentCombatantId: 'a',
        combatants: [
          combatant({ id: 'a', label: 'Aldo', col: 1, row: 1 }),
          combatant({ id: 'b', label: 'Bia', col: 2, row: 2 }),
        ],
        ...over,
      });
    /** The page with the combat already read: later reads are counted from here. */
    async function withCombat(shown: Encounter | null = running()) {
      const get = vi.spyOn(TestBed.inject(CombatClient).turn, 'get').mockResolvedValue(shown);
      const fixture = TestBed.createComponent(LiveSession);
      await settle(fixture);
      const page = flat(fixture) as unknown as { combat: CombatState };
      get.mockClear();
      return { fixture, get, state: page.combat };
    }

    it('reads the combat again only when the news is newer than the copy on screen', async () => {
      const { fixture, get } = await withCombat();
      const hint = async (change: Partial<LiveEventVm & { kind: 'encounterChanged' }>) => {
        source.push({
          kind: 'encounterChanged',
          encounterId: 'e1',
          revision: 5,
          ...change,
        } as LiveEventVm);
        await settle(fixture);
      };
      await hint({ revision: 3 });
      expect(get).not.toHaveBeenCalled();
      await hint({ revision: 5 });
      expect(get).not.toHaveBeenCalled();
      await hint({ revision: 6 });
      expect(get).toHaveBeenCalledTimes(1);
      await hint({ revision: 0 });
      expect(get).toHaveBeenCalledTimes(2);
      await hint({ encounterId: 'other', revision: 1 });
      expect(get).toHaveBeenCalledTimes(3);
    });

    it('reads a combat that arrives while the screen has none', async () => {
      const { fixture, get } = await withCombat(null);
      source.push({ kind: 'encounterChanged', encounterId: 'e1', revision: 1 });
      await settle(fixture);
      expect(get).toHaveBeenCalledTimes(1);
    });

    it('asks the master for the damages that wait on a combat with a map, and not on a theatre', async () => {
      asMaster();
      const damages = vi.spyOn(TestBed.inject(TrapsClient), 'damages');
      const { fixture } = await withCombat();
      damages.mockClear();
      source.push({ kind: 'encounterChanged', encounterId: 'e1', revision: 9 });
      await settle(fixture);
      expect(damages).toHaveBeenCalledTimes(1);
      source.push({
        kind: 'encounterChanged',
        encounterId: 'e1',
        revision: 10,
        mode: EncounterMode.THEATRE,
      });
      await settle(fixture);
      expect(damages).toHaveBeenCalledTimes(1);
    });

    it('moves the turn in place, and reads the combat when it is not the one on screen', async () => {
      const { fixture, get, state } = await withCombat();
      source.push({
        kind: 'turnChanged',
        encounterId: 'e1',
        round: 3,
        currentCombatantId: 'b',
        masterTurn: false,
        revision: 6,
      });
      await settle(fixture);
      expect(state.encounter()?.currentCombatantId).toBe('b');
      expect(state.encounter()?.round).toBe(3);
      expect(get).not.toHaveBeenCalled();
      source.push({
        kind: 'turnChanged',
        encounterId: 'other',
        round: 1,
        currentCombatantId: 'x',
        masterTurn: false,
      });
      await settle(fixture);
      expect(get).toHaveBeenCalledTimes(1);
    });

    it('moves a combatant in place, and reads the combat for one it does not know', async () => {
      const { fixture, get, state } = await withCombat();
      source.push({ kind: 'combatantMoved', encounterId: 'e1', combatantId: 'b', col: 7, row: 8 });
      await settle(fixture);
      const moved = state.encounter()?.combatants.find((c) => c.id === 'b');
      expect([moved?.col, moved?.row]).toEqual([7, 8]);
      expect(get).not.toHaveBeenCalled();
      source.push({ kind: 'combatantMoved', encounterId: 'e1', combatantId: 'zz', col: 1, row: 1 });
      await settle(fixture);
      expect(get).toHaveBeenCalledTimes(1);
    });

    it('reads a combat without a map again on a move, and leaves the traps alone', async () => {
      const { fixture, get } = await withCombat(running({ mode: EncounterMode.THEATRE }));
      source.push({ kind: 'combatantMoved', encounterId: 'e1', combatantId: 'b', col: 7, row: 8 });
      await settle(fixture);
      expect(get).toHaveBeenCalledTimes(1);
    });

    it("reads the log again on combat_log_changed, and the master's damages with it", async () => {
      asMaster();
      const damages = vi.spyOn(TestBed.inject(TrapsClient), 'damages');
      const { fixture, state } = await withCombat();
      damages.mockClear();
      const before = state.logTick();
      source.push({ kind: 'combatLogChanged' });
      await settle(fixture);
      expect(state.logTick()).toBe(before + 1);
      expect(damages).toHaveBeenCalledTimes(1);
    });

    it('retries a combat read that failed, once, a moment later', async () => {
      vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
      try {
        const get = vi
          .spyOn(TestBed.inject(CombatClient).turn, 'get')
          .mockRejectedValueOnce(new Error('network'))
          .mockResolvedValue(running());
        const fixture = TestBed.createComponent(LiveSession);
        await settle(fixture);
        expect(get).toHaveBeenCalledTimes(1);
        await vi.advanceTimersByTimeAsync(1600);
        expect(get).toHaveBeenCalledTimes(2);
        await vi.advanceTimersByTimeAsync(5000);
        expect(get).toHaveBeenCalledTimes(2);
      } finally {
        vi.useRealTimers();
      }
    });
  });

  describe('the rest of the stream', () => {
    const asMaster = () => {
      source.campaign = {
        name: 'Mirathel',
        isMaster: true,
        awaitingApproval: false,
        diceMode: 1,
        dicePreference: 1,
      };
    };

    it('tells the player a trap was noticed, by the name on the map or in words when it is not there', async () => {
      const activity = vi.spyOn(TestBed.inject(TrapsClient), 'activity');
      const fixture = TestBed.createComponent(LiveSession);
      const el = await settle(fixture);
      activity.mockClear();
      source.push({ kind: 'trapNoticed', mapId: 'other', pointId: 'nowhere' });
      await settle(fixture);
      expect(textOf(el)).toContain('Você notou uma armadilha.');
      expect(textOf(el)).toContain('Ela já aparece no seu mapa.');
      expect(activity).toHaveBeenCalledTimes(1);
    });

    it('reads the notes and the puzzles on notes_changed, and the scene on stage_changed', async () => {
      const fixture = TestBed.createComponent(LiveSession);
      await settle(fixture);
      const before = puzzles.calls.length;
      source.push({ kind: 'notesChanged' });
      await settle(fixture);
      expect(puzzles.calls.length).toBeGreaterThan(before);
      scenes.calls.length = 0;
      source.push({ kind: 'stageChanged' });
      await settle(fixture);
      expect(scenes.calls.length).toBeGreaterThan(0);
    });

    it('bumps the creatures when the stream says they changed', async () => {
      const fixture = TestBed.createComponent(LiveSession);
      await settle(fixture);
      const page = flat(fixture) as unknown as { creaturesTick(): number };
      const before = page.creaturesTick();
      source.push({ kind: 'creaturesChanged' });
      await settle(fixture);
      expect(page.creaturesTick()).toBe(before + 1);
    });

    it('leaves the blocked page when the stream fails for good with a refusal', async () => {
      source.failWith = new KindError('forbidden');
      const el = await render();
      expect(textOf(el)).toContain('Voltar para a campanha');
      expect(textOf(el)).not.toContain('Ao vivo');
    });

    it('ends the page when the snapshot says there is no session', async () => {
      source.snapshot = new KindError('no-session');
      const el = await render();
      expect(textOf(el)).toContain('Nenhuma sessão em andamento');
    });

    it('sends a signed-out snapshot to sign in', async () => {
      source.snapshot = new KindError('signed-out');
      await render();
      expect(signIn).toHaveBeenCalled();
    });

    it('starts over on a transient snapshot failure: the stream is restarted', async () => {
      const read = vi.spyOn(source, 'getLiveSession');
      source.snapshot = new KindError('transient');
      const fixture = TestBed.createComponent(LiveSession);
      await settle(fixture);
      expect(read.mock.calls.length).toBeGreaterThanOrEqual(1);
    });

    it('shows "Não foi possível abrir a sessão" for a campaign that fails, and "Tentar de novo" starts over', async () => {
      source.campaign = new Error('down');
      const fixture = TestBed.createComponent(LiveSession);
      const el = await settle(fixture);
      expect(textOf(el)).toContain('Não foi possível abrir a sessão');
      source.campaign = {
        name: 'Mirathel',
        isMaster: false,
        awaitingApproval: false,
        diceMode: 1,
        dicePreference: 1,
      };
      button(el, 'Tentar de novo').click();
      await settle(fixture);
      expect(textOf(el)).toContain('Sessão 4');
    });

    it('signs a person out of a campaign that answers signed-out', async () => {
      source.campaign = new KindError('signed-out');
      await render();
      expect(signIn).toHaveBeenCalled();
    });

    it('says "sem acesso" for a campaign the server hides', async () => {
      source.campaign = new KindError('no-access');
      const el = await render();
      expect(textOf(el)).toContain('Peça um convite ao mestre');
    });

    it('asks the master for the class and player names once for the same party', async () => {
      asMaster();
      const info = vi.spyOn(source, 'getPartyInfo');
      const fixture = TestBed.createComponent(LiveSession);
      await settle(fixture);
      expect(info).toHaveBeenCalledTimes(1);
      source.push({ kind: 'ready' });
      await settle(fixture);
      expect(info).toHaveBeenCalledTimes(1);
    });

    it('lists the maps again for the master when the map changes, or the current one does', async () => {
      asMaster();
      const maps = TestBed.inject(MapsClient) as unknown as FakeMapsClient;
      const list = vi.spyOn(maps, 'list');
      const fixture = TestBed.createComponent(LiveSession);
      await settle(fixture);
      list.mockClear();
      source.push({ kind: 'mapChanged', mapId: 'other' });
      await settle(fixture);
      expect(list).toHaveBeenCalledTimes(1);
      source.push({ kind: 'currentMap', mapId: null });
      await settle(fixture);
      expect(list).toHaveBeenCalledTimes(2);
    });

    it('takes the familiar band away at once when the way back to the own eyes works', async () => {
      const fixture = TestBed.createComponent(LiveSession);
      await settle(fixture);
      const page = flat(fixture) as unknown as {
        vitals(): readonly { familiarSight: unknown }[];
        familiarStopped(id: string): void;
      };
      source.push({
        kind: 'vitals',
        vitals: {
          ...pensantusVitals(),
          familiarSight: { creatureId: 'f1', inCombat: false },
          revision: 99,
        },
      });
      await settle(fixture);
      expect(page.vitals()[0].familiarSight).not.toBeNull();
      page.familiarStopped('pensantus');
      expect(page.vitals()[0].familiarSight).toBeNull();
    });

    it('drops "Ver como" back to Todos and says why when the character is gone', async () => {
      asMaster();
      const fixture = TestBed.createComponent(LiveSession);
      await settle(fixture);
      const page = flat(fixture) as unknown as {
        viewAs(): string | null;
        viewGone(): string;
        setViewAs(id: string | null): void;
        viewAsGone(): void;
      };
      page.setViewAs('pensantus');
      expect(page.viewAs()).toBe('pensantus');
      page.viewAsGone();
      expect(page.viewAs()).toBeNull();
      expect(page.viewGone()).toContain('morreu ou saiu da campanha');
      page.setViewAs(null);
      expect(page.viewGone()).toBe('');
    });

    it("closes the players' highlights card and the summary under it", async () => {
      const fixture = TestBed.createComponent(LiveSession);
      await settle(fixture);
      const page = flat(fixture) as unknown as {
        combat: CombatState;
        highlightsFor(): Encounter | null;
        closeHighlights(): void;
      };
      page.closeHighlights();
      page.combat.apply(encounter({ id: 'done', status: EncounterStatus.ENDED, combatants: [] }));
      await settle(fixture);
      expect(page.highlightsFor()?.id).toBe('done');
      page.closeHighlights();
      expect(page.highlightsFor()).toBeNull();
      expect(page.combat.shown()).toBeNull();
    });

    it('shows a combat the master started at once', async () => {
      asMaster();
      const fixture = TestBed.createComponent(LiveSession);
      await settle(fixture);
      const page = flat(fixture) as unknown as {
        combat: CombatState;
        combatStarted(e: Encounter): void;
      };
      page.combatStarted(encounter({ id: 'new' }));
      expect(page.combat.encounter()?.id).toBe('new');
    });

    it('words the ended combat for the highlights: rounds and defeated enemies', async () => {
      const fixture = TestBed.createComponent(LiveSession);
      await settle(fixture);
      const page = flat(fixture) as unknown as {
        combat: CombatState;
        highlightsSub(): string;
      };
      expect(page.highlightsSub()).toBe('');
      page.combat.apply(
        encounter({
          id: 'done',
          name: 'Emboscada',
          round: 1,
          status: EncounterStatus.ENDED,
          combatants: [
            combatant({ id: 'p', label: 'P', kind: CombatantKind.PLAYER }),
            combatant({ id: 'g1', label: 'G1', defeated: true }),
            combatant({ id: 'g2', label: 'G2' }),
          ],
        }),
      );
      expect(page.highlightsSub()).toBe('Emboscada · 1 rodada · 1 de 2 derrotados');
      page.combat.apply(
        encounter({
          id: 'done2',
          name: 'Briga',
          round: 3,
          status: EncounterStatus.ENDED,
          combatants: [],
        }),
      );
      expect(page.highlightsSub()).toBe('Briga · 3 rodadas');
    });

    it('names who cast Revivificar on a character that lives again, from the combat log', async () => {
      const combatApi = TestBed.inject(CombatClient);
      vi.spyOn(combatApi.turn, 'get').mockResolvedValue(encounter({ id: 'e1', combatants: [] }));
      const log = vi.spyOn(combatApi.turn, 'log').mockResolvedValue({ rounds: [] } as never);
      const fixture = TestBed.createComponent(LiveSession);
      await settle(fixture);
      source.push({ kind: 'characterRevived', characterId: 'pensantus' });
      await settle(fixture);
      expect(log).toHaveBeenCalled();
      const page = flat(fixture) as unknown as {
        revivedCaster(): string;
        revivedId(): string;
      };
      expect(page.revivedId()).toBe('pensantus');
      expect(page.revivedCaster()).toBe('');
      log.mockRejectedValue(new Error('x'));
      source.push({ kind: 'characterRevived', characterId: 'pensantus' });
      await settle(fixture);
      expect(page.revivedCaster()).toBe('');
    });
  });

  describe('the sheets the page opens', () => {
    const asMaster = () => {
      source.campaign = {
        name: 'Mirathel',
        isMaster: true,
        awaitingApproval: false,
        diceMode: 1,
        dicePreference: 1,
      };
    };
    type Page = {
      adjust(v: unknown): void;
      changeDice(): void;
      openNotes(): void;
      searchTraps(): void;
      openRevivify(): void;
      takeVitals(v: readonly unknown[]): void;
      vitals(): readonly { hitPointsCurrent: number }[];
      campaign(): { name: string } | null;
    };
    const pageOf = (fixture: ComponentFixture<LiveSession>) => flat(fixture) as unknown as Page;
    /** The test browser has no `matchMedia`: say whether the screen is a phone. */
    const phone = (matches: boolean) => {
      const list = {
        matches,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
      };
      window.matchMedia = (() => list) as unknown as typeof window.matchMedia;
    };
    beforeEach(() => phone(false));
    afterEach(() => {
      delete (window as unknown as Record<string, unknown>)['matchMedia'];
    });

    it('opens "Ajustar" in a dialog and takes the numbers the sheet saved', async () => {
      asMaster();
      const dialog = TestBed.inject(MatDialog);
      const saved = { ...pensantusVitals(), hitPointsCurrent: 5, revision: 50 };
      const open = vi
        .spyOn(dialog, 'open')
        .mockReturnValue({ afterClosed: () => of({ kind: 'saved', vitals: saved }) } as never);
      const fixture = TestBed.createComponent(LiveSession);
      await settle(fixture);
      pageOf(fixture).adjust(pensantusVitals());
      await settle(fixture);
      expect(open).toHaveBeenCalledTimes(1);
      expect((open.mock.calls[0][1] as { data: { playerName: string } }).data.playerName).toBe(
        'Vinicius',
      );
      expect(pageOf(fixture).vitals()[0].hitPointsCurrent).toBe(5);
    });

    it('ends the page when the "Ajustar" sheet says the session ended', async () => {
      asMaster();
      vi.spyOn(TestBed.inject(MatDialog), 'open').mockReturnValue({
        afterClosed: () => of({ kind: 'ended' }),
      } as never);
      const fixture = TestBed.createComponent(LiveSession);
      await settle(fixture);
      pageOf(fixture).adjust(pensantusVitals());
      const el = await settle(fixture);
      expect(textOf(el)).toContain('Sessão 4 encerrada');
    });

    it('opens "Ajustar" in a bottom sheet on a phone, and leaves the numbers alone when it is dismissed', async () => {
      asMaster();
      const sheet = TestBed.inject(MatBottomSheet);
      const open = vi
        .spyOn(sheet, 'open')
        .mockReturnValue({ afterDismissed: () => of(undefined) } as never);
      const fixture = TestBed.createComponent(LiveSession);
      await settle(fixture);
      phone(true);
      pageOf(fixture).adjust(pensantusVitals());
      expect(open).toHaveBeenCalledTimes(1);
      expect((open.mock.calls[0][1] as { ariaLabel: string }).ariaLabel).toBe('Ajustar Pensantus');
      expect(pageOf(fixture).vitals()[0].hitPointsCurrent).toBe(17);
    });

    it('reads the campaign again after the dice dialog closes', async () => {
      const dialog = TestBed.inject(MatDialog);
      const open = vi
        .spyOn(dialog, 'open')
        .mockReturnValue({ afterClosed: () => of(true) } as never);
      const read = vi.spyOn(source, 'getCampaign');
      const fixture = TestBed.createComponent(LiveSession);
      await settle(fixture);
      read.mockClear();
      pageOf(fixture).changeDice();
      await settle(fixture);
      expect(open).toHaveBeenCalledTimes(1);
      expect((open.mock.calls[0][1] as { data: { campaignName: string } }).data.campaignName).toBe(
        'Mirathel',
      );
      expect(read).toHaveBeenCalledTimes(1);
    });

    it('opens the notes sheet', async () => {
      const dialog = TestBed.inject(MatDialog);
      const open = vi
        .spyOn(dialog, 'open')
        .mockReturnValue({ afterClosed: () => of(undefined) } as never);
      const fixture = TestBed.createComponent(LiveSession);
      await settle(fixture);
      pageOf(fixture).openNotes();
      await settle(fixture);
      expect(open).toHaveBeenCalledTimes(1);
    });

    it('opens "Procurar armadilhas" and reads the activity again when it closes', async () => {
      const dialog = TestBed.inject(MatDialog);
      const open = vi
        .spyOn(dialog, 'open')
        .mockReturnValue({ afterClosed: () => of(true) } as never);
      const activity = vi.spyOn(TestBed.inject(TrapsClient), 'activity');
      const fixture = TestBed.createComponent(LiveSession);
      await settle(fixture);
      activity.mockClear();
      pageOf(fixture).searchTraps();
      await settle(fixture);
      expect(open).toHaveBeenCalledTimes(1);
      expect((open.mock.calls[0][1] as { data: { inCombat: boolean } }).data.inCombat).toBe(false);
      expect(activity).toHaveBeenCalledTimes(1);
    });

    it('does nothing for the sheets before the campaign is known', async () => {
      source.campaign = new Error('down');
      const dialog = TestBed.inject(MatDialog);
      const open = vi.spyOn(dialog, 'open');
      const fixture = TestBed.createComponent(LiveSession);
      await settle(fixture);
      pageOf(fixture).changeDice();
      pageOf(fixture).searchTraps();
      pageOf(fixture).openRevivify();
      expect(open).not.toHaveBeenCalled();
    });

    it('takes the numbers a rest answered with', async () => {
      const fixture = TestBed.createComponent(LiveSession);
      await settle(fixture);
      pageOf(fixture).takeVitals([{ ...pensantusVitals(), hitPointsCurrent: 23, revision: 60 }]);
      expect(pageOf(fixture).vitals()[0].hitPointsCurrent).toBe(23);
    });
  });

  describe('mageArmorClassOf', () => {
    const cast = (over: Record<string, unknown>) =>
      create(OutsideCastSchema, {
        spellKey: 'spell:mage-armor',
        status: OutsideCastStatus.ACTIVE,
        targets: [
          create(OutsideCastTargetSchema, {
            characterId: 'pensantus',
            effect: CastEffect.ARMOR_CLASS,
            armorClass: 13,
          }),
        ],
        ...over,
      });

    it('is the best armor class a lasting Armadura Arcana gives the character, 0 without one', () => {
      const other = create(OutsideCastTargetSchema, {
        characterId: 'brisa',
        effect: CastEffect.ARMOR_CLASS,
        armorClass: 19,
      });
      const bigger = create(OutsideCastTargetSchema, {
        characterId: 'pensantus',
        effect: CastEffect.ARMOR_CLASS,
        armorClass: 15,
      });
      const healing = create(OutsideCastTargetSchema, {
        characterId: 'pensantus',
        effect: CastEffect.HEAL,
        armorClass: 20,
      });
      expect(mageArmorClassOf([], 'pensantus')).toBe(0);
      expect(mageArmorClassOf([cast({})], 'pensantus')).toBe(13);
      expect(mageArmorClassOf([cast({}), cast({ targets: [bigger] })], 'pensantus')).toBe(15);
      expect(mageArmorClassOf([cast({ targets: [other, healing] })], 'pensantus')).toBe(0);
      expect(mageArmorClassOf([cast({ spellKey: 'spell:shield' })], 'pensantus')).toBe(0);
      expect(mageArmorClassOf([cast({ status: OutsideCastStatus.ENDED })], 'pensantus')).toBe(0);
    });
  });
});
