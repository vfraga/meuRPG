import {
  type Combatant,
  type Encounter,
  ReactionKind,
  type ReactionWindow,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { article } from '../format/article';
import { currentCombatant, isPlayer } from './combat-view';
import { mountOf } from './mount-view';

/**
 * The window of a rider whose mount was knocked prone or dropped to 0 hit points (#588, `ReactionKind.DISMOUNT`; SRD 5.1,
 * "Mounted Combat"): the rider uses its reaction to land on its feet next to the mount, or falls prone within 5 ft. The window
 * has no prompt of its own and the player's trigger does not name the mount, so the name comes from the rider's own row
 * (`mounted_on_id`, which the server sends only when the player sees both). Nothing passes this window by itself: letting it
 * pass is a fall, so it is not in `OPTIONAL_KINDS` and has no countdown. Pure functions, tested without a DOM.
 */

/** The Prone condition, as the server keys it. */
const PRONE = 'condition:prone';

/** Whether the window is a rider's dismount. */
export function isDismountWindow(w: ReactionWindow): boolean {
  return w.kind === ReactionKind.DISMOUNT;
}

/** What the rider's prompt says, ready to draw (board 588-07). */
export interface DismountView {
  readonly title: string;
  /** "Reação · desmontar · Rodada 4". */
  readonly subtitle: string;
  /** "O Cavalo de guerra caiu." (bold) */
  readonly lead: string;
  readonly question: string;
  /** What each answer does. */
  readonly note: string;
  readonly chips: readonly string[];
  /** Whose turn waits, and that the master may answer too. */
  readonly wait: string;
  /** The fall: the answer that keeps the reaction. */
  readonly declineLabel: string;
  readonly useLabel: string;
  readonly ariaLabel: string;
}

/** "A vez do Zumbi 1", "A vez de Tor", "A sua vez" (the reader's own combatant is `selfId`), or "A vez" when the one on turn is hidden from the reader. */
export function whoseTurn(e: Encounter, selfId: string): string {
  const on = currentCombatant(e);
  if (!on) {
    return 'A vez';
  }
  if (on.id === selfId) {
    return 'A sua vez';
  }
  return `A vez ${isPlayer(on) ? `de ${on.label}` : `d${article(on.label)} ${on.label}`}`;
}

export function dismountView(e: Encounter, w: ReactionWindow): DismountView {
  const rider = e.combatants.find((c) => c.id === w.reactorId);
  const mount = rider ? mountOf(e, rider) : undefined;
  const lead = mount
    ? `${article(mount.label) === 'a' ? 'A' : 'O'} ${mount.label} caiu.`
    : 'A sua montaria caiu.';
  const question = 'Usar a reação para desmontar de pé?';
  return {
    title: 'A montaria caiu',
    subtitle: `Reação · desmontar · Rodada ${e.round}`,
    lead,
    question,
    note: 'Com a reação, você desce de pé num quadrado ao lado da montaria. Sem ela, cai Derrubado a até 1,5 m.',
    chips: ['Gasta a sua reação', 'Volta no começo da sua vez'],
    wait: `${whoseTurn(e, w.reactorId)} espera a resposta. O mestre também pode responder.`,
    declineLabel: 'Cair derrubado',
    useLabel: 'Desmontar de pé',
    ariaLabel: `${lead.replace(/\.$/, '')}: ${question.charAt(0).toLowerCase()}${question.slice(1)}`,
  };
}

/**
 * What the rider reads of a window that left the combat without their answer, from their own row as it is now: still mounted
 * (the mount stood up or was healed), prone (the master let it pass, or the rider could not react any more), or on its feet
 * with the reaction spent (the master used it for them).
 */
export function dismountGoneText(own: Combatant | null): string {
  if (own?.mountedOnId) {
    return 'Não é mais preciso responder: você continua montado.';
  }
  if (own?.conditions.includes(PRONE)) {
    return 'A pergunta se fechou sem a sua resposta: você caiu Derrubado.';
  }
  if (own?.reactionUsed) {
    return 'O mestre respondeu por você: você desmontou de pé e gastou a reação.';
  }
  return 'A pergunta se fechou sem a sua resposta.';
}
