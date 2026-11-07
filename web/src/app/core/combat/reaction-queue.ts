import { type Encounter } from '../../../gen/meurpg/play/v1/combat_pb';
import { legendaryCards, type LegendaryCard, windowsAbove } from './legendary-card';
import { type ReactionCard, reactionCards } from './reaction-master';
import { openWindows } from './reactions';

/**
 * The master's "Esperando você" queue (#557): the cards he must answer, one at a time, and the legendary cards after them.
 * Exactly one card is "Agora" (the first the master answers); the other blocking ones and the legendary cards read
 * "Depois". A card that waits on a player is not counted: it is not his to answer. Pure functions, no DOM.
 */
export interface QueueItem {
  readonly card: ReactionCard;
  /** `null` for a card that waits on a player. */
  readonly badge: 'Agora' | 'Depois' | null;
  /** "Depois da janela acima.", "Depois das duas janelas acima.", or `''`. */
  readonly waiting: string;
}

export interface MasterQueue {
  /** "Esperando você". */
  readonly title: string;
  /** How many blocking cards are the master's to answer. */
  readonly count: number;
  /** "Esperando você (2)", or `''` when nothing blocks the table. */
  readonly header: string;
  /** "trava a mesa" for one window, "uma de cada vez" for several. */
  readonly hint: string;
  readonly items: readonly QueueItem[];
  /** After the windows; "Opcional" when nothing blocks the table. */
  readonly legendary: readonly LegendaryCard[];
}

/** Whether the master has to answer the card (a player's own question is the player's). */
export function isMasterTurn(card: ReactionCard): boolean {
  switch (card.type) {
    case 'queue':
      return card.rows.some((r) => !r.isPlayer || r.window.unanswered);
    case 'concentration':
      return !card.player;
    default:
      return true;
  }
}

function waitingNote(position: number): string {
  return position <= 0 ? '' : `Depois ${windowsAbove(position)}.`;
}

/** Whether the card is the one to answer now: its window (or a row's) has `answerNow`. */
function answersNow(c: ReactionCard): boolean {
  switch (c.type) {
    case 'queue':
      return c.rows.some((r) => r.window.answerNow);
    case 'single':
    case 'unanswered':
      return c.row.window.answerNow;
    default:
      return c.window.answerNow;
  }
}

/** The master's queue of the combat. */
export function masterQueue(e: Encounter): MasterQueue {
  const cards = reactionCards(e);
  const mine = cards.filter(isMasterTurn);
  const first = mine.find(answersNow) ?? mine[0];
  const ordered = first ? [first, ...mine.filter((c) => c !== first)] : [];
  const items: QueueItem[] = [...ordered, ...cards.filter((c) => !ordered.includes(c))].map(
    (card) => {
      const at = ordered.indexOf(card);
      return {
        card,
        badge: at < 0 ? null : at === 0 ? 'Agora' : 'Depois',
        waiting: at < 0 ? '' : waitingNote(at),
      };
    },
  );
  return {
    title: 'Esperando você',
    count: mine.length,
    header: mine.length > 0 ? `Esperando você (${mine.length})` : '',
    hint: mine.length > 1 ? 'uma de cada vez' : 'trava a mesa',
    items,
    legendary: legendaryCards(e, { ahead: mine.length, playerWaits: openWindows(e).length > 0 }),
  };
}
