import { type ReactionWindow, ReactionKind } from '../../../gen/meurpg/play/v1/combat_pb';
import { joinDots } from '../format/text';
import { ofThe } from './move-plan';
import { featureWindowGroup, featureWindowName, isFeatureWindow } from './reaction-feature';
import { reactionKindName } from './reaction-record';

/** The words of a window's queue card: the name of the reaction and the title and subtitle of the group it belongs to. Pure functions. */

/** The name of the reaction a window offers, as `names_pt.json` writes it. */
export function reactionName(w: ReactionWindow): string {
  return isFeatureWindow(w) ? featureWindowName(w) : reactionKindName(w.kind);
}

/** What the group's title says: the kind of action that provoked its windows. */
export function groupTitle(first: ReactionWindow): string {
  switch (first.kind) {
    case ReactionKind.COUNTERSPELL:
      return 'Reações a uma magia';
    case ReactionKind.FEATHER_FALL:
    case ReactionKind.SLOW_FALL:
      return 'Reações a uma queda';
    case ReactionKind.HELLISH_REBUKE:
      return 'Reações a um dano';
    case ReactionKind.FEATURE:
      return 'Reações a uma criatura';
    case ReactionKind.READIED:
      return 'Ação preparada';
    case ReactionKind.DISMOUNT:
      return 'Reações a uma montaria';
    default:
      return 'Reações a um ataque';
  }
}

/** What the group's second line says: the action that provoked it, and how the windows are answered. */
function triggerSubtitle(first: ReactionWindow): string {
  const t = first.trigger;
  if (!t) {
    return 'na ordem da iniciativa';
  }
  switch (first.kind) {
    case ReactionKind.COUNTERSPELL:
      return joinDots([`${t.actionNamePt} ${ofThe([t.actorLabel])}`, 'na ordem da iniciativa']);
    case ReactionKind.SHIELD:
    case ReactionKind.UNCANNY_DODGE:
    case ReactionKind.DEFLECT_MISSILES:
    case ReactionKind.MANEUVER_REDUCE:
      return joinDots([
        `Ataque ${ofThe([t.actorLabel])}${t.targetLabel ? ` contra ${t.targetLabel}` : ''}`,
        'acertou',
      ]);
    case ReactionKind.FEATHER_FALL:
    case ReactionKind.SLOW_FALL:
      return joinDots(['Queda num fosso', 'na ordem da iniciativa']);
    case ReactionKind.DISMOUNT:
      return joinDots(['Montaria derrubada ou morta', 'na ordem da iniciativa']);
    case ReactionKind.PROTECTION:
      return joinDots([
        `Ataque ${ofThe([t.actorLabel])}${t.targetLabel ? ` contra ${t.targetLabel}` : ''}`,
        'ainda não rolado',
      ]);
    case ReactionKind.FEATURE:
      return joinDots([featureWindowGroup(first), 'na ordem da iniciativa']);
    default:
      return joinDots([
        `${t.actionNamePt || reactionName(first)} ${ofThe([t.actorLabel])}`,
        'na ordem da iniciativa',
      ]);
  }
}

/** The second line of a group's card: a readied action's is the master's own fire; the others say what provoked them. */
export function groupSubtitle(first: ReactionWindow): string {
  return first.kind === ReactionKind.READIED ? 'disparada por você' : triggerSubtitle(first);
}
