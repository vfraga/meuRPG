import type { LiveStreamHandlers } from './live-stream-handlers';
import type { LiveEventVm } from './live-session.types';

type Kind = LiveEventVm['kind'];
type Event<K extends Kind> = Extract<LiveEventVm, { kind: K }>;

/** The kinds `LiveStream` answers itself: `ready` moves the stream's state, `ended` closes it, a heartbeat only counts as a message. */
type OwnKind = 'ready' | 'ended' | 'heartbeat';

/** What each kind of message does to the page's handlers. A handler the page left out is skipped. */
const ROUTES: { [K in Exclude<Kind, OwnKind>]: (h: LiveStreamHandlers, e: Event<K>) => void } = {
  vitals: (h, e) => h.onVitals(e.vitals),
  currentMap: (h, e) => h.onCurrentMap?.(e.mapId),
  mapChanged: (h, e) => h.onMapChanged?.(e.mapId),
  tokenMoved: (h, e) => h.onTokenMoved?.(e),
  visionChanged: (h, e) => h.onVisionChanged?.(e.mapId),
  leftImages: (h) => h.onLeftImages?.(),
  shownImage: (h, e) => h.onShownImage?.(e.image),
  encounterChanged: (h, e) => h.onEncounterChanged?.(e),
  reactionWindowOpened: (h, e) => h.onReactionWindowOpened?.(e),
  reactionWindowClosed: (h, e) => h.onReactionWindowClosed?.(e),
  turnChanged: (h, e) => h.onTurnChanged?.(e),
  combatantMoved: (h, e) => h.onCombatantMoved?.(e),
  combatLogChanged: (h) => h.onCombatLogChanged?.(),
  xpChanged: (h) => h.onXpChanged?.(),
  // Both hints mean the same to the page: read the open scene again.
  sceneChanged: (h) => h.onSceneChanged?.(),
  sceneCheckRolled: (h) => h.onSceneChanged?.(),
  notesChanged: (h) => h.onNotesChanged?.(),
  stageChanged: (h) => h.onStageChanged?.(),
  trapNoticed: (h, e) => h.onTrapNoticed?.(e),
  creaturesChanged: (h) => h.onCreaturesChanged?.(),
  spellCastsChanged: (h) => h.onSpellCastsChanged?.(),
  groupCheckChanged: (h) => h.onGroupCheckChanged?.(),
  contentChanged: (h) => h.onContentChanged?.(),
  characterChanged: (h, e) => h.onCharacterChanged?.(e.characterId),
  // A revival is a change of that character first, then the revival itself.
  characterRevived: (h, e) => {
    h.onCharacterChanged?.(e.characterId);
    h.onCharacterRevived?.(e.characterId);
  },
  revivifyChanged: (h) => h.onRevivifyChanged?.(),
  callingYou: (h) => h.onCallingYou?.(),
  puzzleChanged: (h, e) => h.onPuzzleChanged?.(e.puzzleId),
};

/** Hands a message to the handler of its kind; a kind with none (a heartbeat, one this app does not know) does nothing. */
export function routeEvent(handlers: LiveStreamHandlers, event: LiveEventVm): void {
  const route = (
    ROUTES as Record<string, ((h: LiveStreamHandlers, e: LiveEventVm) => void) | undefined>
  )[event.kind];
  route?.(handlers, event);
}
