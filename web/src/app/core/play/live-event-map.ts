import { timestampDate } from '@bufbuild/protobuf/wkt';

import type { WatchGameSessionResponse } from '../../../gen/meurpg/play/v1/play_pb';
import { toFreeRollVm } from '../free-dice/free-roll';
import type { LiveEventVm } from './live-session.types';
import { toShownImageVm, toVitalsVm } from './live-vitals-map';

type WireEvent = NonNullable<WatchGameSessionResponse['event']> & { case: string };

/** One translator per case of the wire event, so a case added to the proto fails to compile until it is mapped here.
 * `null` skips the event (a `vitals_changed` with no body says nothing). */
type Translators = {
  [E in WireEvent as E['case']]: (value: E['value']) => LiveEventVm | null;
};

const signal =
  (kind: 'ready' | 'heartbeat' | 'ended' | 'leftImages' | 'combatLogChanged' | 'xpChanged') =>
  (): LiveEventVm => ({ kind });

const TRANSLATORS: Translators = {
  ready: signal('ready'),
  heartbeat: signal('heartbeat'),
  hiddenHitPending: signal('heartbeat'),
  vitalsChanged: (v) => (v.vitals ? { kind: 'vitals', vitals: toVitalsVm(v.vitals) } : null),
  sessionEnded: signal('ended'),
  currentMapChanged: (v) => ({ kind: 'currentMap', mapId: v.mapId || null }),
  mapChanged: (v) => ({ kind: 'mapChanged', mapId: v.mapId }),
  tokenMoved: (v) => ({
    kind: 'tokenMoved',
    mapId: v.mapId,
    characterId: v.characterId,
    xBp: v.xBp,
    yBp: v.yBp,
  }),
  visionChanged: (v) => ({ kind: 'visionChanged', mapId: v.mapId }),
  shownImageChanged: (v) => ({ kind: 'shownImage', image: toShownImageVm(v.image) }),
  leftImagesChanged: signal('leftImages'),
  encounterChanged: (v) => ({
    kind: 'encounterChanged',
    encounterId: v.encounterId,
    revision: v.revision,
    // The combat's mode (RN-25): a combat without a map has no fog to read and no trap to refresh.
    mode: v.mode,
  }),
  reactionWindowOpened: (v) => ({
    kind: 'reactionWindowOpened',
    encounterId: v.encounterId,
    windowId: v.windowId,
  }),
  reactionWindowClosed: (v) => ({
    kind: 'reactionWindowClosed',
    encounterId: v.encounterId,
    windowId: v.windowId,
    closedByItself: v.closedByItself,
    text: v.textPt,
  }),
  turnChanged: (v) => ({
    kind: 'turnChanged',
    encounterId: v.encounterId,
    round: v.round,
    currentCombatantId: v.currentCombatantId,
    masterTurn: v.masterTurn,
    revision: v.revision,
  }),
  combatantMoved: (v) => ({
    kind: 'combatantMoved',
    encounterId: v.encounterId,
    combatantId: v.combatantId,
    col: v.col,
    row: v.row,
  }),
  combatLogChanged: signal('combatLogChanged'),
  xpChanged: signal('xpChanged'),
  sceneChanged: () => ({ kind: 'sceneChanged' }),
  sceneCheckRolled: () => ({ kind: 'sceneCheckRolled' }),
  notesChanged: () => ({ kind: 'notesChanged' }),
  stageChanged: () => ({ kind: 'stageChanged' }),
  trapNoticed: (v) => ({ kind: 'trapNoticed', mapId: v.mapId, pointId: v.pointId }),
  creaturesChanged: () => ({ kind: 'creaturesChanged' }),
  spellCastsChanged: () => ({ kind: 'spellCastsChanged' }),
  groupCheckChanged: () => ({ kind: 'groupCheckChanged' }),
  contentChanged: () => ({ kind: 'contentChanged' }),
  characterChangesRequested: (v) => ({ kind: 'characterChanged', characterId: v.characterId }),
  characterResubmitted: (v) => ({ kind: 'characterChanged', characterId: v.characterId }),
  characterRevived: (v) => ({ kind: 'characterRevived', characterId: v.characterId }),
  revivifyChanged: () => ({ kind: 'revivifyChanged' }),
  callingYou: () => ({ kind: 'callingYou' }),
  heldRollsChanged: () => ({ kind: 'heldRollsChanged' }),
  // A message with no roll tells nothing, like a `vitals_changed` with no body.
  freeRollRolled: (v) => (v.roll ? { kind: 'freeRollRolled', roll: toFreeRollVm(v.roll) } : null),
  puzzleChanged: (v) => ({ kind: 'puzzleChanged', puzzleId: v.puzzleId }),
  // A line with no body says nothing.
  sessionMessageAdded: (v) => (v.message ? { kind: 'sessionMessage', message: v.message } : null),
  attackHit: (v) => ({
    kind: 'attackHit',
    id: v.id,
    actorLabel: v.actorLabel,
    amount: v.amount,
    damageTypePt: v.damageTypePt,
  }),
  masterTyping: (v) => ({ kind: 'masterTyping', expiresInSeconds: v.expiresInSeconds }),
  tableScreenChanged: (v) => ({
    kind: 'tableScreenChanged',
    open: v.open,
    lostAt: v.lostAt ? timestampDate(v.lostAt) : null,
    choiceRevision: v.choiceRevision,
    presenceKnown: v.presenceKnown,
  }),
};

/** The view-model event for a wire event; `null` when it carries nothing to tell. */
export function toLiveEvent(event: WatchGameSessionResponse['event']): LiveEventVm | null {
  const translate = event.case
    ? (TRANSLATORS[event.case] as ((value: unknown) => LiveEventVm | null) | undefined)
    : undefined;
  // A newer server's event this app doesn't know yet: still proof that the stream is alive.
  return translate ? translate(event.value) : { kind: 'heartbeat' };
}
