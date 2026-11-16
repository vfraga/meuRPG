import { NgTemplateOutlet } from '@angular/common';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { RailPanel, SessionRail } from '../../shared/session-rail/session-rail';
import { Fold } from '../../shared/fold/fold';
import { TabList } from '../../shared/tab-list/tab-list';
import { MasterPanel } from './master-board/master-panel';
import { MapTab } from './combat/map-tabs/map-tab';
import { SessionTabs } from './conversation/session-tabs';
import { UnreadNotice } from './conversation/unread-notice';
import { ClueNotice } from './clue-notice/clue-notice';
import { FreeRollLog } from './free-dice/free-roll-log';
import { FogMasterPanel } from './fog-tools/fog-master-panel';
import { FogPlayerTools } from './fog-tools/fog-player-tools';
import { FamiliarBand } from '../../shared/familiar-eyes/familiar-band';
import { BattleEncounters } from './battle-encounters/battle-encounters';
import { CombatLaunchNotice } from './combat/combat-launch';
import { CastingPanel } from './casting/casting-panel';
import { BardicBlock } from './inspiration/bardic-block';
import { CombatView } from './combat/combat-view';
import { FoundTreasures } from './treasure/found-treasures/found-treasures';
import { GroupCheckCard } from './group-check/group-check-card';
import { GroupCheckMaster } from './group-check/group-check-master';
import { HeldRollsPanel } from './held-questions/held-rolls-panel';
import { HighlightsCard } from './combat/combat-highlights/highlights-card';
import { LiveToast } from '../../shared/live-toast/live-toast';
import { MasterLive } from './puzzles/master-live/master-live';
import { RouterLink } from '@angular/router';
import { MasterPuzzles } from './puzzles/master-puzzles/master-puzzles';
import { PartyChargesPanel } from './party-charges/party-charges';
import { PartyPanel } from './party-panel/party-panel';
import { RestCard } from './rest-card/rest-card';
import { CharacterEffectsPanel } from './effects/character-effects-panel';
import { PuzzleNotice } from './puzzles/puzzle-notice/puzzle-notice';
import { PuzzlePlayPage } from './puzzles/puzzle-play/puzzle-play';
import { PlayerVitals } from './player-vitals/player-vitals';
import { RevivifyAsk } from './revivify-ask/revivify-ask';
import { RevivedNotice } from './revived-notice/revived-notice';
import { CallingNotice } from './calling-notice/calling-notice';
import { FavorLines } from '../../shared/favor-lines/favor-lines';
import { FavorRequests } from '../../shared/favor-requests/favor-requests';
import { FavorNotice } from './favor-notice/favor-notice';
import { SessionBlocked } from './session-blocked/session-blocked';
import { SessionEnded } from './session-ended/session-ended';
import { SessionHeader } from './session-header/session-header';
import { SessionMap } from './session-map/session-map';
import { SessionTokens } from './session-tokens/session-tokens';
import { SceneOpen } from './scene/scene-open/scene-open';
import { ScenePanel } from './scene/scene-panel/scene-panel';
import { ScenePlayer } from './scene/scene-player/scene-player';
import { TrapActivityList } from './traps/trap-activity/trap-activity';
import { TrapPanel } from './traps/trap-panel/trap-panel';
import { TreasurePanel } from './treasure/treasure-panel/treasure-panel';
import { LeftImagesBlock } from './left-images-block/left-images-block';
import { ShownImageBlock } from './shown-image-block/shown-image-block';
import { ShownImagePanel } from './shown-image-panel/shown-image-panel';

/** The components and directives the session page's template uses. */
export const LIVE_SESSION_IMPORTS = [
  RailPanel,
  SessionRail,
  Fold,
  TabList,
  MasterPanel,
  MatButtonModule,
  MatIconModule,
  ClueNotice,
  FreeRollLog,
  SessionTabs,
  NgTemplateOutlet,
  MapTab,
  UnreadNotice,
  FogMasterPanel,
  FogPlayerTools,
  FamiliarBand,
  BattleEncounters,
  CombatLaunchNotice,
  CastingPanel,
  BardicBlock,
  CombatView,
  FoundTreasures,
  GroupCheckCard,
  GroupCheckMaster,
  HeldRollsPanel,
  HighlightsCard,
  LiveToast,
  MasterLive,
  RouterLink,
  MasterPuzzles,
  PartyPanel,
  PartyChargesPanel,
  RestCard,
  CharacterEffectsPanel,
  PuzzleNotice,
  PuzzlePlayPage,
  PlayerVitals,
  RevivifyAsk,
  RevivedNotice,
  CallingNotice,
  FavorLines,
  FavorNotice,
  FavorRequests,
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
];
