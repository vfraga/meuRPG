import { DOCUMENT, DestroyRef, Injector, inject } from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { ActivatedRoute, Router } from '@angular/router';

import { AuthService } from '../../core/auth/auth.service';
import { CastingClient } from '../../core/casting/casting-client';
import { CombatClient } from '../../core/combat/combat-client';
import { SpellCatalog } from '../../core/combat/spell-catalog';
import { TurnAlert } from '../../core/combat/turn-alert';
import { FreeDiceClient } from '../../core/free-dice/free-dice-client';
import { FreeDiceState } from '../../core/free-dice/free-dice-state';
import { ConversationClient } from '../../core/conversation/conversation-client';
import { MapsClient } from '../../core/maps/maps-client';
import { NotesClient } from '../../core/notes/notes-client';
import { FamiliarEyesClient } from '../../core/play/familiar-eyes';
import { LiveSessionSource } from '../../core/play/live-session.types';
import { OpenSessions } from '../../core/play/open-sessions';
import { SceneClient } from '../../core/play/scene-client';
import { TableScreenClient } from '../../core/play/table-screen-client';
import { TableScreenPanel } from '../../core/play/table-screen-panel';
import { XpChanges } from '../../core/progression/xp-changes';
import { PuzzlesClient } from '../../core/puzzles/puzzles-client';
import { TrapsClient } from '../../core/traps/traps-client';
import { RollAnimator } from '../../shared/roll-overlay/roll-animator';
import { SessionNotes } from '../../shell/session-notes/session-notes';
import { CombatState } from '../../core/combat/combat-state';
import { DarkWatch } from '../../core/maps/dark-watch';
import { FogView } from '../../core/maps/fog-view';
import { MapState } from '../../core/maps/map-state';
import { TokenMoveClient } from '../../core/maps/token-move-client';
import { TokenMover } from '../../core/maps/token-mover';
import { NotesState } from '../../core/notes/notes-state';
import { ConversationState } from '../../core/conversation/conversation-state';
import { SceneState } from '../../core/play/scene-state';
import { PuzzleSessionState } from '../../core/puzzles/puzzle-session';
import { ToastQueue } from '../../core/traps/toast-queue';
import { TrapBoard } from '../../core/traps/trap-board';
import { TreasureWatch } from '../../core/traps/treasure-text';
import type { LiveSessionState } from './live-session-state';
import { freeRollShow } from './free-dice/free-roll-show';

/** What the session page injects, handed to the plain classes it builds (they are not in the injection context). */
export interface LiveSessionServices {
  readonly source: LiveSessionSource;
  readonly auth: AuthService;
  readonly xpChanges: XpChanges;
  readonly spellCatalog: SpellCatalog;
  readonly router: Router;
  readonly route: ActivatedRoute;
  readonly document: Document;
  readonly dialog: MatDialog;
  readonly bottomSheet: MatBottomSheet;
  readonly mapsApi: MapsClient;
  readonly tokenMoveApi: TokenMoveClient;
  readonly combatApi: CombatClient;
  readonly castingApi: CastingClient;
  readonly sceneApi: SceneClient;
  readonly conversationApi: ConversationClient;
  readonly notesApi: NotesClient;
  readonly trapsApi: TrapsClient;
  readonly puzzlesApi: PuzzlesClient;
  readonly freeDiceApi: FreeDiceClient;
  readonly tableScreenApi: TableScreenClient;
  /** The dice overlay of the shell: the table's free rolls tumble on it. */
  readonly rollAnimator: RollAnimator;
  readonly sessionNotes: SessionNotes;
  readonly openSessions: OpenSessions;
  readonly destroyRef: DestroyRef;
  /** The adjust sheet needs this route's `LiveSessionSource`: MatDialog and MatBottomSheet live in the root injector,
   * which doesn't have it. */
  readonly injector: Injector;
  readonly turnAlert: TurnAlert;
  readonly familiarEyes: FamiliarEyesClient;
}

/** Reads every service of the page; call it in an injection context (a field initializer of the component). */
export function injectLiveSessionServices(): LiveSessionServices {
  return {
    source: inject(LiveSessionSource),
    auth: inject(AuthService),
    xpChanges: inject(XpChanges),
    spellCatalog: inject(SpellCatalog),
    router: inject(Router),
    route: inject(ActivatedRoute),
    document: inject(DOCUMENT),
    dialog: inject(MatDialog),
    bottomSheet: inject(MatBottomSheet),
    mapsApi: inject(MapsClient),
    tokenMoveApi: inject(TokenMoveClient),
    combatApi: inject(CombatClient),
    castingApi: inject(CastingClient),
    sceneApi: inject(SceneClient),
    conversationApi: inject(ConversationClient),
    notesApi: inject(NotesClient),
    trapsApi: inject(TrapsClient),
    puzzlesApi: inject(PuzzlesClient),
    freeDiceApi: inject(FreeDiceClient),
    tableScreenApi: inject(TableScreenClient),
    rollAnimator: inject(RollAnimator),
    sessionNotes: inject(SessionNotes),
    openSessions: inject(OpenSessions),
    destroyRef: inject(DestroyRef),
    injector: inject(Injector),
    turnAlert: inject(TurnAlert),
    familiarEyes: inject(FamiliarEyesClient),
  };
}

/** The state holders the page builds around its services: each one is a plain class with its own tests. */
export class LiveSessionParts {
  readonly mapState: MapState;
  /** The master's moving of tokens outside combat (B2): the selection and the move the map and the "Tokens" tab share. */
  readonly mover: TokenMover;
  /** What traps did and the damage that waits (MR-035): the master's panel and the players' "Registro". */
  readonly trapBoard: TrapBoard;
  /** The player's toasts: a trap noticed, a treasure found (8 s each). */
  readonly toasts = new ToastQueue();
  readonly treasureWatch = new TreasureWatch();
  /** What the viewer sees of the current map when it has the fog of war on (MR-036): the vision with its tiles and
   * the layers, read again on `vision_changed` and `map_changed`. The combat map draws the same one. */
  readonly fog: FogView;
  /** Who sees nothing where they stand on the current map (B3, master only): read again with the vision hints; a player's
   * page never reads it (the target is `null`). */
  readonly dark: DarkWatch;
  /** The session's combat (MR-013): read on every `ready` and after each
   * `encounter_changed`; `turn_changed` and `combatant_moved` apply in place. */
  readonly combat = new CombatState();
  /** The RP scene open in the session (MR-015): read on every `ready` and
   * after each `scene_changed` or `scene_check_rolled`. */
  readonly scene: SceneState;
  /** The conversation of the scene (MR-051, RN-36): read on the first `ready`, its gap read after each reconnection;
   * `session_message_added` and `master_typing` apply in place. */
  readonly conversation: ConversationState;
  /** The puzzles of the session (MR-038, E10-06): the master's menu with each one's live state, or what the players are shown.
   * `puzzle_changed` reads one again; `ready` reads the list. */
  readonly puzzles: PuzzleSessionState;
  /** The player's notes and the clues the master revealed (MR-030): read after
   * each `ready` and each `notes_changed`. The master has none. */
  readonly notes: NotesState;
  /** The free dice roller (MR-052): the form, the result, the last rolls and the table's log. */
  readonly freeDice: FreeDiceState;
  /** The master's table screen: the choice, the TV's presence and the labels of the bar's button. */
  readonly tableScreen: TableScreenPanel;

  constructor(services: LiveSessionServices, state: LiveSessionState) {
    const { mapsApi } = services;
    this.mapState = new MapState((mapId) => mapsApi.get(state.campaignId(), mapId));
    this.mover = new TokenMover(
      services.tokenMoveApi,
      this.mapState,
      () => state.campaignId(),
      () => state.isMaster(),
    );
    this.trapBoard = new TrapBoard(
      services.trapsApi,
      mapsApi,
      () => state.campaignId(),
      () => state.currentMapId(),
      () => state.isMaster(),
    );
    this.fog = new FogView(
      (mapId, as) => mapsApi.vision(state.campaignId(), mapId, as ?? ''),
      (mapId, as) => mapsApi.layers(state.campaignId(), mapId, as ?? ''),
      () => !state.isMaster(),
    );
    this.dark = new DarkWatch(
      (mapId) => mapsApi.charactersInTheDark(state.campaignId(), mapId),
      () => {
        const map = this.mapState.map();
        return state.isMaster() && map?.fogEnabled ? map.id : null;
      },
      () => this.mapState.tokens(),
    );
    this.scene = new SceneState(
      () => services.sceneApi.get(state.campaignId()),
      () => state.isMaster(),
    );
    this.conversation = new ConversationState(
      services.conversationApi,
      () => state.campaignId(),
      () => {
        const me = services.auth.state();
        return me.status === 'signed-in' ? me.user.id : '';
      },
    );
    this.puzzles = new PuzzleSessionState(
      services.puzzlesApi,
      () => state.campaignId(),
      () => state.isMaster(),
    );
    this.notes = new NotesState(services.notesApi, () => state.campaignId());
    this.tableScreen = new TableScreenPanel(services.tableScreenApi, () => state.campaignId());
    this.freeDice = new FreeDiceState(services.freeDiceApi, {
      campaignId: () => state.campaignId(),
      isMaster: () => state.isMaster(),
      show: (roll) => {
        const show = freeRollShow(roll);
        if (show) {
          services.rollAnimator.play(show);
        }
      },
    });
  }
}
