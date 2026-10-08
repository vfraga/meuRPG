import {
  Component,
  DOCUMENT,
  DestroyRef,
  Injector,
  computed,
  effect,
  inject,
  signal,
  untracked,
  viewChild,
  TemplateRef,
} from '@angular/core';
import { SpellCatalog } from '../../core/combat/spell-catalog';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';

import type { Map as MapMessage } from '../../../gen/meurpg/maps/v1/maps_pb';
import {
  type Encounter,
  EncounterMode,
  EncounterStatus,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { AuthService } from '../../core/auth/auth.service';
import { CombatClient } from '../../core/combat/combat-client';
import { CombatState } from '../../core/combat/combat-state';
import { mineTabs } from '../../core/combat/mine';
import { isTheatre } from '../../core/combat/theatre';
import type { ViewAsPerson } from '../../shared/fog-map/view-as-list';
import { FogMasterPanel } from './fog-tools/fog-master-panel';
import { FogPlayerTools } from './fog-tools/fog-player-tools';
import { FamiliarBand } from '../../shared/familiar-eyes/familiar-band';
import { FamiliarEyesClient } from '../../core/play/familiar-eyes';
import { FogView } from '../../core/maps/fog-view';
import { MapState } from '../../core/maps/map-state';
import { NotesClient } from '../../core/notes/notes-client';
import { NotesState } from '../../core/notes/notes-state';
import type { CluePlayer } from '../../core/maps/scene-clues';
import { XpChanges } from '../../core/progression/xp-changes';
import { PuzzleSessionState } from '../../core/puzzles/puzzle-session';
import { PuzzlesClient } from '../../core/puzzles/puzzles-client';
import { ToastQueue } from '../../core/traps/toast-queue';
import { TrapBoard } from '../../core/traps/trap-board';
import { foundToastTitle, TreasureWatch } from '../../core/traps/treasure-text';
import { firstLine } from '../../core/traps/trap-text';
import { TrapsClient } from '../../core/traps/traps-client';
import { focusWithRing } from '../../core/creatures/focus-ring';
import { LiveToast } from '../../shared/live-toast/live-toast';
import type { PickRow } from '../../shared/person-pick/person-pick';
import { MapsClient } from '../../core/maps/maps-client';
import { SceneClient } from '../../core/play/scene-client';
import { SceneState } from '../../core/play/scene-state';
import { setPageSubject } from '../../core/title/page-title';
import { openNotesSheet } from '../../shared/notes/notes-sheet';
import { OpenSessions } from '../../shell/live-notice/open-sessions';
import { SessionNotes } from '../../shell/session-notes/session-notes';
import { AdjustVitals } from './adjust-vitals/adjust-vitals';
import { AdjustVitalsData, AdjustVitalsResult } from './adjust-vitals/adjust-vitals.types';
import {
  CampaignInfoVm,
  LiveSessionSource,
  LiveSessionVm,
  PartyMemberInfoVm,
  PlayerSheetVm,
  ShownImageVm,
  VitalsVm,
} from './live-session.types';
import { ClueNotice } from './clue-notice/clue-notice';
import { BattleEncounters } from './battle-encounters/battle-encounters';
import { CombatLaunch } from './combat/combat-launch';
import { DiceDialog, DiceDialogData } from './combat/dice-dialog';
import { CombatView } from './combat/combat-view';
import { HighlightsCard } from './combat/combat-highlights/highlights-card';
import { LiveStream } from './live-stream';
import { PartyPanel } from './party-panel/party-panel';
import { PlayerVitals } from './player-vitals/player-vitals';
import { MasterLive } from './puzzles/master-live/master-live';
import { MasterPuzzles } from './puzzles/master-puzzles/master-puzzles';
import { PuzzleNotice } from './puzzles/puzzle-notice/puzzle-notice';
import { PuzzlePlayPage } from './puzzles/puzzle-play/puzzle-play';
import { SessionBlocked } from './session-blocked/session-blocked';
import { SessionEnded } from './session-ended/session-ended';
import { SessionHeader } from './session-header/session-header';
import { SessionMap } from './session-map/session-map';
import { SessionTokens } from './session-tokens/session-tokens';
import { SceneOpen } from './scene/scene-open/scene-open';
import { ScenePanel } from './scene/scene-panel/scene-panel';
import { ScenePlayer } from './scene/scene-player/scene-player';
import { LeftImagesBlock } from './left-images-block/left-images-block';
import { ShownImageBlock } from './shown-image-block/shown-image-block';
import { ShownImagePanel } from './shown-image-panel/shown-image-panel';
import { applySnapshot, applyVitals, partyRowSub } from './vitals';
import { FoundTreasures } from './treasure/found-treasures/found-treasures';
import { TreasurePanel } from './treasure/treasure-panel/treasure-panel';
import { TrapActivityList } from './traps/trap-activity/trap-activity';
import { TrapPanel } from './traps/trap-panel/trap-panel';
import { openTrapSearch } from './traps/trap-search-sheet/trap-search-sheet';

/**
 * Where the page stands. `live` covers reconnecting too: the numbers stay
 * on screen, readable, while the status line says "Reconectando…".
 */
type Phase = 'loading' | 'live' | 'no-access' | 'no-session' | 'ended' | 'error';

/**
 * The session page, `/campaigns/:id/session` (MR-011, MR-012, RN-02, RN-06,
 * RN-07; artboards E5-02 to E5-08).
 *
 * It asks for the campaign (its name, and whether the person is its
 * master), then opens the live stream (`LiveStream`: ADR-0005's client
 * rules). On every `ready` it reads the snapshot (`GetLiveSession`), and
 * applies each `vitals_changed` whose revision is newer. The player sees
 * their own character's vitals; the master sees the party, with "Ajustar".
 *
 * The map half (MR-012): the snapshot carries the current map and the
 * shown image (MR-028), and the stream's `current_map_changed`,
 * `map_changed`, `token_moved` and `shown_image_changed` keep them current
 * without a reload. `map_changed` reads the map again; if that answers
 * `not_found`, a player lost sight of it and gets the empty state.
 */
@Component({
  selector: 'app-live-session',
  imports: [
    MatButtonModule,
    MatIconModule,
    ClueNotice,
    FogMasterPanel,
    FogPlayerTools,
    FamiliarBand,
    BattleEncounters,
    CombatLaunch,
    CombatView,
    FoundTreasures,
    HighlightsCard,
    LiveToast,
    MasterLive,
    RouterLink,
    MasterPuzzles,
    PartyPanel,
    PuzzleNotice,
    PuzzlePlayPage,
    PlayerVitals,
    SessionBlocked,
    SessionEnded,
    SessionHeader,
    SessionMap,
    SessionTokens,
    SceneOpen,
    ScenePanel,
    ScenePlayer,
    TrapActivityList,
    TrapPanel,
    TreasurePanel,
    LeftImagesBlock,
    ShownImageBlock,
    ShownImagePanel,
  ],
  templateUrl: './live-session.html',
  styleUrl: './live-session.scss',
})
export class LiveSession {
  private readonly source = inject(LiveSessionSource);
  private readonly auth = inject(AuthService);
  private readonly xpChanges = inject(XpChanges);
  private readonly spellCatalog = inject(SpellCatalog);
  private readonly router = inject(Router);
  private readonly document = inject(DOCUMENT);
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);
  private readonly mapsApi = inject(MapsClient);
  private readonly combatApi = inject(CombatClient);
  private readonly sceneApi = inject(SceneClient);
  private readonly notesApi = inject(NotesClient);
  private readonly trapsApi = inject(TrapsClient);
  private readonly puzzlesApi = inject(PuzzlesClient);
  private readonly sessionNotes = inject(SessionNotes);
  private readonly openSessions = inject(OpenSessions);
  private readonly destroyRef = inject(DestroyRef);
  /** The adjust sheet needs this route's `LiveSessionSource`: MatDialog
   * and MatBottomSheet live in the root injector, which doesn't have it. */
  private readonly injector = inject(Injector);

  protected readonly campaignId = signal('');
  protected readonly phase = signal<Phase>('loading');
  protected readonly campaign = signal<CampaignInfoVm | null>(null);
  protected readonly session = signal<LiveSessionVm | null>(null);
  protected readonly vitals = signal<readonly VitalsVm[]>([]);
  protected readonly playerSheet = signal<PlayerSheetVm | null>(null);
  /** The armor class of the beast's book while the druid is a beast (the vitals show it in place of the druid's own). */
  protected readonly beastAc = signal<number | null>(null);
  protected readonly partyInfo = signal<ReadonlyMap<string, PartyMemberInfoVm>>(new Map());

  /** The session's current map, the image on show, and the map itself. */
  protected readonly currentMapId = signal<string | null>(null);
  protected readonly shownImage = signal<ShownImageVm | null>(null);
  /** The master's "Deixar com os jogadores" switch for the image on show. */
  protected readonly shownKeep = signal(false);
  /** The images the master left with the players (MR-028). */
  protected readonly leftImages = signal<readonly ShownImageVm[]>([]);
  /** What a player's screen reader hears when the master shows or stops. */
  protected readonly shownNotice = signal('');
  protected readonly campaignMaps = signal<readonly MapMessage[]>([]);
  protected readonly mapState = new MapState((mapId) => this.mapsApi.get(this.campaignId(), mapId));

  /** What traps did and the damage that waits (MR-035): the master's panel and the players' "Registro". */
  protected readonly trapBoard = new TrapBoard(
    this.trapsApi,
    this.mapsApi,
    () => this.campaignId(),
    () => this.currentMapId(),
    () => this.isMaster(),
  );
  /** The player's toasts: a trap noticed, a treasure found (8 s each). */
  protected readonly toasts = new ToastQueue();
  private readonly treasureWatch = new TreasureWatch();

  /** What the viewer sees of the current map when it has the fog of war on (MR-036): the vision with its tiles and
   * the layers, read again on `vision_changed` and `map_changed`. The combat map draws the same one. */
  protected readonly fog = new FogView(
    (mapId, as) => this.mapsApi.vision(this.campaignId(), mapId, as ?? ''),
    (mapId, as) => this.mapsApi.layers(this.campaignId(), mapId, as ?? ''),
    () => !this.isMaster(),
  );
  /** The current map's ID when it has the fog on and the session is live; the master reads it too (his own map is drawn by the same component). */
  private readonly fogMapId = computed(() => {
    const map = this.mapState.map();
    return map?.fogEnabled && this.phase() === 'live' ? map.id : null;
  });
  /** Goes up when what a player sees may have changed: the master's "Ver como" reads again (his stream never hears `vision_changed`). */
  protected readonly visionTick = signal(0);
  /** The character the master looks as ("Ver como"), the line about that view, and why it went back to "Todos". */
  protected readonly viewAs = signal<string | null>(null);
  protected readonly viewNote = signal('');
  protected readonly viewGone = signal('');
  /** Bumped when the stream says a character's creatures changed. */
  protected readonly creaturesTick = signal(0);
  protected readonly familiarNameNow = signal<string | null>(null);
  protected readonly seeingFamiliar = computed(() => !!this.vitals().at(0)?.familiarSight);
  private readonly familiarEyes = inject(FamiliarEyesClient);
  private visionTimer: ReturnType<typeof setTimeout> | undefined;
  protected readonly fogMaster = computed(
    () => this.isMaster() && !!this.mapState.map()?.fogEnabled,
  );
  protected readonly fogPlayer = computed(
    () => !this.isMaster() && !!this.mapState.map()?.fogEnabled,
  );
  protected readonly ownToken = computed(
    () => this.mapState.tokens().find((t) => t.mine && !t.creatureId) ?? null,
  );

  /** The session's combat (MR-013): read on every `ready` and after each
   * `encounter_changed`; `turn_changed` and `combatant_moved` apply in place. */
  protected readonly combat = new CombatState();
  /** The running combat has no map (RN-25): no fog, no traps, nothing that depends on place. */
  protected readonly inTheatre = computed(() => isTheatre(this.combat.shown()));

  /** The RP scene open in the session (MR-015): read on every `ready` and
   * after each `scene_changed` or `scene_check_rolled`. */
  protected readonly scene = new SceneState(
    () => this.sceneApi.get(this.campaignId()),
    () => this.isMaster(),
  );

  /** The puzzles of the session (MR-038, E10-06): the master's menu with each one's live state, or what the players are shown.
   * `puzzle_changed` reads one again; `ready` reads the list. */
  protected readonly puzzles = new PuzzleSessionState(
    this.puzzlesApi,
    () => this.campaignId(),
    () => this.isMaster(),
  );
  /** A combat is running (not ended): the player's open puzzle page keeps a notice for it, and says when it is their turn. */
  protected readonly combatRunning = computed(
    () => !this.isMaster() && this.combat.encounter()?.status === EncounterStatus.ACTIVE,
  );
  protected readonly yourTurn = computed(() => {
    const e = this.combat.encounter();
    return (
      !!e && e.status === EncounterStatus.ACTIVE && mineTabs(e).some((tab) => tab.state === 'turn')
    );
  });
  /** The master's puzzle panel is on the board when the campaign has puzzles (or when they could not be read). */
  protected readonly puzzlesShown = computed(
    () => this.puzzles.runs().length > 0 || this.puzzles.status() === 'error',
  );
  /** The puzzle a player has open (`?puzzle=ID`), in the place of the board; the master plays none. */
  protected readonly openPuzzle = signal<string | null>(null);

  /** The player's notes and the clues the master revealed (MR-030): read after
   * each `ready` and each `notes_changed`. The master has none. */
  protected readonly notes = new NotesState(this.notesApi, () => this.campaignId());

  /** The app bar's "Anotações" button, drawn by the bar (see `SessionNotes`). */
  protected readonly notesBar = viewChild<TemplateRef<unknown>>('notesBar');
  /** What a screen reader hears when the visible words are cut short on a narrow phone. */
  protected readonly notesAria = (fresh: number): string =>
    fresh > 0 ? `Anotações, ${fresh} ${fresh === 1 ? 'nova' : 'novas'}` : 'Anotações';
  protected readonly freshCount = computed(() => this.notes.fresh().length);

  /** The players' names by character, for "de Caio" on the session's highlights (master only). */
  protected readonly playerNames = computed(
    () =>
      new Map(
        [...this.partyInfo()].flatMap(([id, p]) =>
          p.playerName ? [[id, p.playerName] as const] : [],
        ),
      ),
  );

  protected readonly ownCharacterId = computed(() => this.vitals().at(0)?.characterId ?? '');
  protected readonly ownCharacterName = computed(() => this.vitals().at(0)?.name ?? '');
  /** The ended combat whose "Destaques" card this player closed (MR-032). */
  private readonly highlightsClosed = signal<string | null>(null);
  /** The players' "O combate acabou" card: the combat that ended, until they close it. */
  protected readonly highlightsFor = computed(() => {
    const e = this.combat.encounter();
    return !this.isMaster() &&
      e?.status === EncounterStatus.ENDED &&
      e.id !== this.highlightsClosed()
      ? e
      : null;
  });
  protected readonly highlightsSub = computed(() => {
    const e = this.highlightsFor();
    const rounds = Math.max(1, e?.round ?? 1);
    return e ? `${e.name} · ${rounds} ${rounds === 1 ? 'rodada' : 'rodadas'}` : '';
  });

  protected readonly stream = signal<LiveStream | null>(null);
  protected readonly connection = computed(() => this.stream()?.status() ?? 'connecting');
  protected readonly lastUpdate = computed(() => this.stream()?.lastMessageAt() ?? null);
  protected readonly isMaster = computed(() => this.campaign()?.isMaster ?? false);
  /** The player's own character: the only one the server sends them. */
  protected readonly ownVitals = computed(() => this.vitals()[0] ?? null);
  /** The master's "Ver como" list: the living player characters, with their players. */
  protected readonly viewAsPeople = computed<readonly ViewAsPerson[]>(() =>
    this.vitals().map((v) => ({
      id: v.characterId,
      name: v.name,
      sub: this.partyInfo().get(v.characterId)?.playerName ?? '',
    })),
  );
  /** The master's player characters, for who gets a clue (a deleted account has no player). */
  protected readonly cluePlayers = computed<readonly CluePlayer[]>(() =>
    this.vitals()
      .filter((v) => v.playerUserId !== '')
      .map((v) => ({
        id: v.characterId,
        name: v.name,
        playerName: this.partyInfo().get(v.characterId)?.playerName ?? '',
      })),
  );

  /** The living player characters as rows to pick from ("Quem encontrou"): the class and the player under the name. */
  protected readonly finderRows = computed<readonly PickRow[]>(() =>
    this.vitals()
      .filter((v) => v.playerUserId !== '')
      .map((v) => {
        const info = this.partyInfo().get(v.characterId);
        return {
          id: v.characterId,
          name: v.name,
          sub: [info?.classSummary ?? '', info?.playerName ? `de ${info.playerName}` : '']
            .filter(Boolean)
            .join(', '),
        };
      }),
  );
  /** The player's bonuses for "Procurar armadilhas" (the derived sheet). */
  protected readonly trapSkills = computed(() => this.playerSheet()?.skills ?? null);
  /** The player may search the map for traps: a character of theirs, on a map with a grid. */
  protected readonly searchable = computed(
    () =>
      !this.isMaster() && this.ownVitals() !== null && (this.mapState.map()?.gridColumns ?? 0) > 0,
  );

  /** The campaign of the current load; `null` once it turned out the page
   * can't show it (no access, a pending member, an error). */
  private campaignLoad: Promise<CampaignInfoVm | null> = Promise.resolve(null);
  private loadedSheetFor: string | null = null;
  private partyInfoIds = new Set<string>();
  private generation = 0;
  /** Moves with each read of the images left with the players, and with each one taken back here. */
  private leftSeq = 0;

  constructor() {
    // The tab's title carries the campaign's name once it is loaded.
    setPageSubject(() => this.campaign()?.name);
    inject(ActivatedRoute)
      .paramMap.pipe(takeUntilDestroyed())
      .subscribe((params) => {
        const id = params.get('id');
        if (id) {
          this.load(id);
        }
      });
    inject(ActivatedRoute)
      .queryParamMap.pipe(takeUntilDestroyed())
      .subscribe((params) => this.openPuzzle.set(params.get('puzzle')));
    this.destroyRef.onDestroy(() => {
      this.toasts.clear();
      clearTimeout(this.visionTimer);
      this.closeStream();
      this.sessionNotes.bar.set(null);
    });

    // A player's session page hands the app bar the "Anotações" button (the
    // bar only draws it); the master has no notes.
    effect(() => {
      const show = this.phase() === 'live' && !this.isMaster();
      const bar = this.notesBar();
      untracked(() => this.sessionNotes.bar.set(show ? (bar ?? null) : null));
    });

    // A treasure the master marks found while the player looks at the map: a toast (E9-09 4). The first read
    // of a map is the baseline, so a treasure found before they got here is not news.
    effect(() => {
      const map = this.mapState.status() === 'ready' ? this.mapState.map() : null;
      const points = this.mapState.points();
      untracked(() => {
        const fresh = this.treasureWatch.newlyFound(map?.id ?? null, points);
        if (!this.isMaster()) {
          for (const p of fresh) {
            this.toasts.push('inventory_2', foundToastTitle(p), firstLine(p.description));
          }
        }
      });
    });

    // A map with the fog of war on: the vision is read when it is the current one, and closed when it is not.
    // It depends on the map's ID and its fog flag, not on each reading of the map: a hint costs one read.
    effect(() => {
      const id = this.fogMapId();
      untracked(() => {
        // Another map: "Ver como" starts over at "Todos".
        this.viewAs.set(null);
        this.viewGone.set('');
        void this.fog.open(id);
      });
    });
    // The familiar a player looks through, for the name on the card "O que o Nanquim vê".
    effect(() => {
      const sight = this.ownVitals()?.familiarSight;
      const characterId = this.ownVitals()?.characterId ?? '';
      const campaignId = this.campaignId();
      untracked(() => {
        if (!sight || this.isMaster()) {
          this.familiarNameNow.set(null);
          return;
        }
        void this.familiarEyes
          .name(campaignId, characterId, sight.creatureId)
          .then((n) => this.familiarNameNow.set(n));
      });
    });

    // The beast's armor class from its book, read once for each beast the druid becomes.
    effect(() => {
      const key = this.ownVitals()?.wildShape?.beastKey ?? '';
      const campaignId = this.campaignId();
      untracked(() => {
        if (!key || this.isMaster()) {
          this.beastAc.set(null);
          return;
        }
        this.source.getCreatureArmorClass(campaignId, key).then(
          (ac) => this.beastAc.set(ac),
          () => this.beastAc.set(null),
        );
      });
    });

    // Waiting on the link before the master starts: the app's light poll
    // (RN-06) notices the new session, and the page opens it by itself.
    effect(() => {
      const id = this.campaignId();
      const listed = this.openSessions.liveCampaignIds().has(id);
      // Only a campaign that newly shows up in the list opens the page: a stale list still naming a session that
      // ended would otherwise load, find none, and load again without end.
      const appeared = this.listedCampaign.id === id && !this.listedCampaign.listed && listed;
      this.listedCampaign = { id, listed };
      if (this.phase() === 'no-session' && appeared) {
        untracked(() => this.load(id));
      }
    });
  }

  /** Whether the open-sessions list named the campaign the last time the waiting effect looked. */
  private listedCampaign: { id: string; listed: boolean } = { id: '', listed: false };
  /** Counts the stream's news of the current map and of the shown image: a snapshot that began before one of them
   * does not overwrite it. */
  private mapEvents = 0;
  private imageEvents = 0;

  private load(campaignId: string): void {
    const generation = ++this.generation;
    this.closeStream();
    this.campaignId.set(campaignId);
    this.phase.set('loading');
    this.campaign.set(null);
    this.session.set(null);
    this.vitals.set([]);
    this.playerSheet.set(null);
    this.partyInfo.set(new Map());
    this.currentMapId.set(null);
    this.shownImage.set(null);
    this.shownKeep.set(false);
    this.leftImages.set([]);
    this.shownNotice.set('');
    this.campaignMaps.set([]);
    void this.mapState.open(null);
    this.combat.clear();
    this.highlightsClosed.set(null);
    this.scene.clear();
    this.notes.clear();
    this.trapBoard.clear();
    this.puzzles.clear();
    this.toasts.clear();
    this.loadedSheetFor = null;
    this.partyInfoIds = new Set();

    // The campaign (its name, and whether the person is its master) and the
    // stream start together: at the table, every round trip is a wait.
    this.campaignLoad = this.source.getCampaign(campaignId).then(
      (campaign) => {
        if (generation !== this.generation) {
          return null;
        }
        if (campaign.awaitingApproval) {
          // A pending member isn't a member yet (RN-15): same page as a
          // stranger, without the campaign's name.
          this.closeStream();
          this.phase.set('no-access');
          return null;
        }
        this.campaign.set(campaign);
        return campaign;
      },
      (err: unknown) => {
        if (generation === this.generation) {
          this.closeStream();
          this.fail(err);
        }
        return null;
      },
    );
    this.openStream(campaignId, generation);
  }

  private openStream(campaignId: string, generation: number): void {
    const stream = new LiveStream({
      open: (signal) => this.source.watch(campaignId, signal),
      classify: (err) => this.source.classifyError(err),
      document: this.document,
      handlers: {
        onReady: () => {
          // A missed `xp_changed` while reconnecting leaves nothing stale.
          this.xpChanges.bump();
          // A `content_changed` missed while reconnecting leaves the spells the page read before out of date.
          this.spellCatalog.forget(campaignId);
          void this.trapBoard.refresh();
          void this.readSnapshot(campaignId, generation);
        },
        onContentChanged: () => this.spellCatalog.forget(campaignId),
        onVitals: (v) => {
          // Looking through a familiar's eyes, or coming back, changes what the player sees.
          const before =
            this.vitals().find((x) => x.characterId === v.characterId)?.familiarSight?.creatureId ??
            '';
          this.vitals.update((list) => applyVitals(list, v));
          if ((v.familiarSight?.creatureId ?? '') !== before) {
            this.scheduleVision();
          }
        },
        onCurrentMap: (mapId) => this.showMap(mapId),
        onMapChanged: (mapId) => this.mapChanged(mapId),
        onVisionChanged: (mapId) => {
          // The fog map's tokens, points and squares come from reads: read them again.
          if (mapId === this.currentMapId()) {
            void this.mapState.refresh();
            this.scheduleVision();
          }
        },
        onCreaturesChanged: () => this.creaturesTick.update((n) => n + 1),
        onPuzzleChanged: (id) => void this.puzzles.changed(id),
        onTokenMoved: (move) => {
          this.scheduleVision();
          void this.trapBoard.tokensMoved();
          // A token the page doesn't know (a missed `map_changed`): read again.
          if (!this.mapState.moveToken(move.mapId, move.characterId, move.xBp, move.yBp)) {
            void this.mapState.refresh();
          }
        },
        onEncounterChanged: (change) => {
          // A combat without a map has no fog and no traps (RN-25): the hint says the mode, and nothing of the map is read for it.
          const noMap = change.mode === EncounterMode.THEATRE || (!change.mode && this.inTheatre());
          // Any change of the combat may change who sees whom (revision 0 is a hint with no number): read the vision too.
          if (!noMap) {
            this.scheduleVision();
          }
          // Read again only when the news is newer than the copy on screen. A hint with no revision
          // (0) is the server telling a player "read again" without counting: an opportunity offer
          // made to them, or any change on a fog map, where each player has a revision of their own
          // (ADR-0007). It is always read.
          const current = this.combat.encounter();
          if (
            !current ||
            current.id !== change.encounterId ||
            change.revision === 0 ||
            change.revision > current.revision
          ) {
            void this.loadCombat(generation);
          }
          // A trap that fired in the combat leaves damage that waits for the master.
          if (!noMap) {
            void this.trapBoard.refreshDamages();
          }
        },
        onTurnChanged: (turn) => {
          if (!this.combat.applyTurn(turn)) {
            void this.loadCombat(generation);
          }
        },
        onCombatantMoved: (move) => {
          if (!this.inTheatre()) {
            this.scheduleVision();
            void this.trapBoard.tokensMoved();
          }
          if (!this.combat.applyMove(move)) {
            void this.loadCombat(generation);
          }
        },
        onCombatLogChanged: () => {
          this.combat.touchLog();
          if (!this.inTheatre()) {
            void this.trapBoard.refreshDamages();
          }
        },
        onTrapNoticed: (notice) => void this.trapNoticed(notice.mapId, notice.pointId),
        onXpChanged: () => {
          this.xpChanges.bump();
          // Converting a treasure to XP (and undoing it) changes its point on the map, with no map event.
          if (this.isMaster()) {
            void this.mapState.refresh();
          }
        },
        onSceneChanged: () => void this.scene.refresh(),
        onNotesChanged: () => {
          void this.notes.refresh(true);
          // A cipher's key is a clue in the notes: the open puzzle reads its run again to learn the player found it.
          void this.puzzles.refresh();
        },
        // The stage is part of the open scene: read it again (it names nobody).
        onStageChanged: () => void this.scene.refresh(),
        onShownImage: (image) => this.shownImageChanged(image),
        onLeftImages: () => void this.reloadLeftImages(),
        onEnded: () => this.ended(),
        onFatal: (kind) => {
          if (kind === 'signed-out') {
            this.auth.signIn(this.router.url);
          } else {
            this.phase.set('no-access');
          }
        },
      },
    });
    this.stream.set(stream);
    stream.start();
  }

  private async readSnapshot(campaignId: string, generation: number): Promise<void> {
    const mapEvents = this.mapEvents;
    const imageEvents = this.imageEvents;
    try {
      const [snapshot, campaign] = await Promise.all([
        this.source.getLiveSession(campaignId),
        this.campaignLoad,
      ]);
      if (generation !== this.generation || !campaign) {
        return;
      }
      this.session.set(snapshot.session);
      this.vitals.update((list) => applySnapshot(list, snapshot.vitals));
      // News that came while the snapshot was on its way is newer than it: the snapshot only fills in what no event told.
      const mapIsNews = mapEvents === this.mapEvents;
      if (mapIsNews) {
        this.currentMapId.set(snapshot.currentMapId);
      }
      if (imageEvents === this.imageEvents) {
        this.shownImage.set(snapshot.shownImage);
        this.shownKeep.set(snapshot.shownImageKeep);
      }
      void this.reloadLeftImages();
      void this.loadCombat(generation);
      void this.scene.refresh();
      void this.puzzles.refresh();
      if (!this.isMaster()) {
        // A clue that arrived while the stream was down counts as new too.
        void this.notes.refresh(true);
      }
      this.combat.touchLog(); // the log is read again too, after a reconnection
      // Each `ready` (a reconnection too) reads the map again: a missed event never leaves it stale.
      if (mapIsNews) {
        void this.mapState.open(snapshot.currentMapId);
      }
      // What a player sees on a fog map, and the creatures, follow the server's current state too.
      this.scheduleVision();
      this.creaturesTick.update((n) => n + 1);
      if (this.isMaster()) {
        void this.reloadMaps();
      }
      this.phase.set('live');
      this.stream()?.confirmHealthy();
      // Here already: the notice about this session has nothing to add.
      this.openSessions.dismiss(snapshot.session.sessionId);
      this.loadDetails(campaignId, generation);
    } catch (err) {
      if (generation !== this.generation) {
        return;
      }
      this.snapshotFailed(err);
    }
  }

  /** What a snapshot that could not be read means: some answers end the page, only the others are worth asking again. */
  private snapshotFailed(err: unknown): void {
    switch (this.source.classifyError(err)) {
      case 'no-session':
        this.ended();
        return;
      case 'no-access':
      case 'forbidden':
        // The server will not give this person the session, however many times it is asked.
        this.closeStream();
        this.phase.set('no-access');
        return;
      case 'signed-out':
        this.closeStream();
        this.auth.signIn(this.router.url);
        return;
      case 'invalid':
        // The request itself is wrong: asking again changes nothing, "Tentar de novo" starts over.
        this.closeStream();
        this.phase.set('error');
        return;
      default:
        // Try the whole thing again: the next `ready` reads a new one.
        this.stream()?.restart();
    }
  }

  /** The session's combat, as this person may see it (best effort: the
   * screen keeps the copy it has until the next event or `ready`). */
  private async loadCombat(generation: number): Promise<void> {
    const ticket = this.combat.beginRead();
    try {
      const encounter = await this.combatApi.get(this.campaignId());
      if (generation === this.generation && !this.combat.applyRead(ticket, encounter)) {
        // Dropped for a turn or a move that came while it was out: that read may be older than the event.
        if (this.combat.patchedSince(ticket)) {
          void this.loadCombat(generation);
        }
      }
    } catch {
      // The stream's next event, or reconnection, reads it again.
    }
  }

  /** "Fechar" on the players' "Destaques" card. */
  protected closeHighlights(): void {
    const e = this.highlightsFor();
    if (e) {
      this.highlightsClosed.set(e.id);
    }
  }

  /** "Iniciar combate" answered: the combat is on screen at once. */
  protected combatStarted(encounter: Encounter): void {
    this.combat.apply(encounter);
  }

  /** "Mudar" on the initiative screen: the player's dice choice (RN-18). */
  protected changeDice(): void {
    const campaign = this.campaign();
    if (!campaign) {
      return;
    }
    this.dialog
      .open<DiceDialog, DiceDialogData, boolean>(DiceDialog, {
        data: {
          campaignId: this.campaignId(),
          campaignName: campaign.name,
          mode: campaign.diceMode,
          preference: campaign.dicePreference,
        },
        width: '560px',
        maxWidth: 'calc(100vw - 32px)',
        autoFocus: 'first-heading',
      })
      .afterClosed()
      .subscribe(() => void this.refreshCampaign());
  }

  /** The campaign again, to pick up the dice choice the player just saved. */
  protected async refreshCampaign(): Promise<void> {
    const generation = this.generation;
    try {
      const campaign = await this.source.getCampaign(this.campaignId());
      if (generation === this.generation) {
        this.campaign.set(campaign);
      }
    } catch {
      // Best effort: the screen keeps the choice it had.
    }
  }

  /** What the vitals come with: the player's CA and class line, or the
   * master's class and player names per character. Best effort: the live
   * numbers don't wait for these. */
  private loadDetails(campaignId: string, generation: number): void {
    if (this.isMaster()) {
      const ids = this.vitals().map((v) => v.characterId);
      if (ids.every((id) => this.partyInfoIds.has(id))) {
        return;
      }
      ids.forEach((id) => this.partyInfoIds.add(id));
      this.source.getPartyInfo(campaignId).then(
        (info) => generation === this.generation && this.partyInfo.set(info),
        () => undefined,
      );
      return;
    }
    const own = this.ownVitals();
    if (!own || this.loadedSheetFor === own.characterId) {
      return;
    }
    this.loadedSheetFor = own.characterId;
    this.source.getPlayerSheet(campaignId, own.characterId).then(
      (sheet) => generation === this.generation && this.playerSheet.set(sheet),
      () => undefined,
    );
  }

  /** "Voltar aos seus olhos" worked: the band goes at once; the stream's newer vitals confirm it. */
  protected familiarStopped(characterId: string): void {
    this.vitals.update((list) =>
      list.map((v) => (v.characterId === characterId ? { ...v, familiarSight: null } : v)),
    );
    void this.mapState.refresh();
    this.scheduleVision();
  }

  /** What a player sees may have changed (a hint, a move, a turn): one read of the vision and its layers for a burst of them. The master's "Ver como" reads again too. */
  private scheduleVision(): void {
    if (this.visionTimer !== undefined) {
      return;
    }
    this.visionTimer = setTimeout(() => {
      this.visionTimer = undefined;
      void this.fog.refresh();
      this.visionTick.update((n) => n + 1);
    }, 120);
  }

  /** "Ver como": the character the master looks as; the line about that view clears until the next read. */
  protected setViewAs(id: string | null): void {
    this.viewAs.set(id);
    this.viewNote.set('');
    this.viewGone.set('');
  }

  /** The character he looked as died or left the campaign: back to "Todos", and say so. */
  protected viewAsGone(): void {
    this.viewAs.set(null);
    this.viewNote.set('');
    this.viewGone.set('Esse personagem morreu ou saiu da campanha. Voltamos para “Todos”.');
  }

  /** `current_map_changed`, or the master's own choice. */
  protected showMap(mapId: string | null): void {
    this.mapEvents++;
    this.currentMapId.set(mapId);
    void this.mapState.open(mapId);
    if (this.isMaster()) {
      void this.reloadMaps();
    }
  }

  private mapChanged(mapId: string): void {
    // The damage that waits covers the whole campaign (ListTrapDamages): a trap on another map may have fired.
    void this.trapBoard.refreshDamages();
    if (mapId === this.currentMapId()) {
      void this.mapState.refresh();
      // Something about the traps may have changed: a firing, a notice, a damage that waits.
      void this.trapBoard.refresh();
      this.scheduleVision();
    }
    if (this.isMaster()) {
      void this.reloadMaps();
    }
  }

  /** `trap_noticed` (E9-08 G): this player's character noticed a trap. The map is read again (the trap is on it now,
   * for them alone) and the toast names it. */
  private async trapNoticed(mapId: string, pointId: string): Promise<void> {
    if (mapId === this.currentMapId()) {
      await this.mapState.refresh();
    }
    const name = this.mapState.points().find((p) => p.id === pointId)?.name;
    this.toasts.push(
      'visibility',
      'Você notou uma armadilha.',
      name ? `${name}, no mapa.` : 'Ela já aparece no seu mapa.',
    );
    void this.trapBoard.refreshActivity();
  }

  /** "Procurar armadilhas": the player's search sheet (E9-08). */
  protected searchTraps(): void {
    const campaign = this.campaign();
    if (!campaign) {
      return;
    }
    const opener = this.document.activeElement as HTMLElement | null;
    openTrapSearch(this.dialog, this.bottomSheet, {
      campaignId: this.campaignId(),
      skills: this.trapSkills(),
      diceMode: campaign.diceMode,
      preference: campaign.dicePreference,
      state: this.mapState,
      inCombat: false,
    })
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe(() => {
        focusWithRing(opener);
        void this.trapBoard.refreshActivity();
      });
  }

  /** `shown_image_changed`: the block appears, changes or goes away. */
  private shownImageChanged(image: ShownImageVm | null): void {
    this.imageEvents++;
    if (!this.isMaster()) {
      const previous = this.shownImage();
      if (image) {
        this.shownNotice.set(`O mestre está mostrando ${image.name}.`);
      } else if (previous) {
        this.shownNotice.set('O mestre parou de mostrar a imagem.');
      }
    }
    // The switch is per image: a new image, or none, starts with it off.
    this.shownKeep.set(false);
    this.shownImage.set(image);
  }

  /** The master took an image back: it leaves the list now, and a read that began before this answer is dropped
   * (it may still carry the image). */
  protected takenBack(image: ShownImageVm): void {
    this.leftSeq++;
    this.leftImages.update((list) => list.filter((i) => i.id !== image.id));
  }

  /** The images left with the players, read again (best effort: the list
   * keeps what it had). */
  protected async reloadLeftImages(): Promise<void> {
    const generation = this.generation;
    // Only the latest read lands: replies come in any order, and an older one holds an older list.
    const seq = ++this.leftSeq;
    try {
      const images = await this.source.listLeftImages(this.campaignId());
      if (generation === this.generation && seq === this.leftSeq) {
        this.leftImages.set(images);
      }
    } catch {
      // Best effort.
    }
  }

  /** The master's select and the "Fundo de mapa escondido" tags read the list. */
  private async reloadMaps(): Promise<void> {
    const generation = this.generation;
    try {
      const maps = await this.mapsApi.list(this.campaignId());
      if (generation === this.generation) {
        this.campaignMaps.set(maps);
      }
    } catch {
      // Best effort: the select keeps what it had.
    }
  }

  private ended(): void {
    this.closeStream();
    // Without a snapshot, there was never a session on this page.
    this.phase.set(this.session() ? 'ended' : 'no-session');
    void this.openSessions.refresh();
  }

  private fail(err: unknown): void {
    switch (this.source.classifyError(err)) {
      case 'no-access':
        this.phase.set('no-access');
        return;
      case 'signed-out':
        this.auth.signIn(this.router.url);
        return;
      default:
        this.phase.set('error');
    }
  }

  private closeStream(): void {
    this.stream()?.stop();
    this.stream.set(null);
  }

  protected retry(): void {
    this.load(this.campaignId());
  }

  protected sessionEndedHere(): void {
    this.ended();
  }

  /** "Anotações": the bar's button, or the notice's "Abrir anotações". */
  protected openNotes(): void {
    openNotesSheet(this.dialog, this.bottomSheet, {
      state: this.notes,
      openScene: () => this.scene.scene()?.pointId ?? '',
    })
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe(() => {
        // Drawn twice (phone and wide): the one on screen takes the focus.
        const bars = Array.from(this.document.querySelectorAll<HTMLElement>('.notes-bar'));
        bars.find((b) => b.offsetParent !== null)?.focus();
      });
  }

  /** "Ajustar": a bottom sheet on a phone, a dialog from a tablet up, with
   * the same content (README-A). */
  protected adjust(vitals: VitalsVm): void {
    const info = this.partyInfo().get(vitals.characterId);
    const data: AdjustVitalsData = {
      campaignId: this.campaignId(),
      vitals,
      sub: partyRowSub(info),
      playerName: info?.playerName ?? null,
    };
    const onPhone = this.document.defaultView?.matchMedia('(max-width: 767.98px)').matches ?? false;
    const closed = onPhone
      ? this.bottomSheet
          .open<AdjustVitals, AdjustVitalsData, AdjustVitalsResult>(AdjustVitals, {
            data,
            injector: this.injector,
            ariaLabel: `Ajustar ${vitals.name}`,
            // The title first, so a stray Enter can't take 5 HP.
            autoFocus: 'first-heading',
          })
          .afterDismissed()
      : this.dialog
          .open<AdjustVitals, AdjustVitalsData, AdjustVitalsResult>(AdjustVitals, {
            data,
            injector: this.injector,
            width: '440px',
            maxWidth: 'calc(100vw - 32px)',
            ariaLabelledBy: 'adjust-title',
            autoFocus: 'first-heading',
          })
          .afterClosed();
    closed.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((result) => {
      if (result?.kind === 'saved') {
        this.vitals.update((list) => applyVitals(list, result.vitals));
      } else if (result?.kind === 'ended') {
        this.ended();
      }
    });
  }
}
