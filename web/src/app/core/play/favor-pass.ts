import { FavorBlockedReason } from '../../../gen/meurpg/play/v1/favor_pb';
import type { FavorPassVm, FavorRefundVm } from './favor-client';
import { favorBlocked, favorBlockedMessage } from './favor-errors';

// The words of the pass and of the refund question (RN-43, boards 581-04 to 581-07), and the state functions behind them: which
// refusal says what, what a button reads while its call is on its way, and what a reader is told when something closed under them.

/** What a call of the pass or of the refund question is doing: the pressed button reads it, and its pair is dashed (581-07, item 14). */
export type FavorWork = 'ask' | 'cancel' | 'accept' | 'refuse' | 'give' | 'keep';

/** The label of the pressed button while its call is on its way. */
export const FAVOR_WORK_LABEL: Readonly<Record<FavorWork, string>> = {
  ask: 'Pedindo ao mestre…',
  cancel: 'Cancelando…',
  accept: 'Aceitando…',
  refuse: 'Recusando…',
  give: 'Devolvendo…',
  keep: 'Mantendo…',
};

/** The chip of a character whose ask waits for the master (581-07, item 4). */
export const FAVOR_WAITING_CHIP = 'Inspiração · esperando o mestre';

/** The line of the dialog under the list: nothing moves until the master answers. */
export const FAVOR_PASS_NOTE =
  'Até o mestre responder, ela continua com você e não pode ser usada.';

/** The reason under the dashed spend box while the ask waits. */
export function favorPassPendingText(toName: string): string {
  return `Você pediu para passar a Inspiração a ${toName}. Cancele o pedido para usar.`;
}

/** "Brisa · o mestre aceita ou recusa": the subtitle of the dialog. */
export function favorPassSubtitle(fromName: string): string {
  return `${fromName} · o mestre aceita ou recusa`;
}

/** The giver's row on the sheet: "Para Tor" in bold and " · esperando o mestre" after it. */
export function favorPassRow(toName: string): { lead: string; rest: string } {
  return { lead: `Para ${toName}`, rest: ' · esperando o mestre' };
}

/** What the giver reads when the master accepted (581-07, item 5). */
export function favorAcceptedText(toName: string): string {
  return `O mestre aceitou: a sua Inspiração foi para ${toName}.`;
}

/** What the giver reads after every refusal, and never anything of the receiver (RN-43: the receiver's state is not the giver's to read). */
export const FAVOR_REFUSED_TEXT = 'O mestre recusou: a Inspiração continua com você.';

/** What the roller reads when the master gave the Inspiração back (581-07, item 2): never why. */
export const FAVOR_RETURNED_TEXT = 'O mestre devolveu a sua Inspiração.';

/** One line of the master's request cards, in pieces so that the bold part is never built from markup. */
export interface FavorPiece {
  readonly text: string;
  readonly bold: boolean;
}

/** "Brisa quer passar a Inspiração". */
export function favorPassTitle(fromName: string): string {
  return `${fromName} quer passar a Inspiração`;
}

/** "**Para Tor.** Se aceitar, ela sai de Brisa e vai para Tor." or, when the receiver has one, "**Para Pensantus.** Pensantus já tem
 * Inspiração: aceitar não é possível." */
export function favorPassBody(pass: FavorPassVm): readonly FavorPiece[] {
  return [
    { text: `Para ${pass.toName}.`, bold: true },
    {
      text: pass.receiverHas
        ? ` ${pass.toName} já tem Inspiração: aceitar não é possível.`
        : ` Se aceitar, ela sai de ${pass.fromName} e vai para ${pass.toName}.`,
      bold: false,
    },
  ];
}

/** "Jabuti usou a Inspiração (Ataque). A rolagem já tinha vantagem por algo que Jabuti não vê: **Alvo Impedido** (só você vê)." The
 * server names the roll by its kind and the hidden sources by their sentence; the card says what they are and that only the master
 * reads them. */
export function favorRefundBody(prompt: FavorRefundVm): readonly FavorPiece[] {
  const label = prompt.rollLabel ? ` (${prompt.rollLabel})` : '';
  const lead = `${prompt.characterName} usou a Inspiração${label}.`;
  const names = [...new Set(prompt.hidden.filter((s) => s.advantage).map((s) => s.name))];
  if (names.length === 0) {
    return [{ text: `${lead} A rolagem já tinha vantagem.`, bold: false }];
  }
  const pieces: FavorPiece[] = [
    {
      text: `${lead} A rolagem já tinha vantagem por algo que ${prompt.characterName} não vê: `,
      bold: false,
    },
  ];
  names.forEach((name, i) => {
    pieces.push({ text: name, bold: true });
    pieces.push({ text: i < names.length - 1 ? ', ' : ' (só você vê).', bold: false });
  });
  return pieces;
}

/** The small line under the refund question. */
export function favorRefundNote(characterName: string): string {
  return `Sem resposta, ela fica gasta. Se devolver, ${characterName} lê só que você devolveu.`;
}

/** What the master reads when the giver gave the ask up before the answer (581-07, item 15). */
export function favorCanceledText(fromName: string): string {
  return `${fromName} cancelou o pedido antes da sua resposta. Nada mudou.`;
}

/** What the master reads when a question was answered or closed on another screen. */
export const FAVOR_ANSWERED_ELSEWHERE = 'Esse pedido já foi respondido em outra tela.';

/** What the master reads when "Devolver a Inspiração" found the roller with one again. */
export function favorHasAgainText(characterName: string): string {
  return `${characterName} já tem Inspiração de novo: não há o que devolver. O pedido fechou.`;
}

/** What the master reads when the receiver cannot hold it now (the accept's `CANNOT_RECEIVE`, the give back's too). */
export const FAVOR_CANNOT_RECEIVE = 'Essa personagem não pode ficar com a Inspiração agora.';

/** A call that failed for a reason the server does not name (the network, an error). */
export const FAVOR_ANSWER_FAILED = 'Não foi possível responder agora. Tente de novo.';
export const FAVOR_ASK_FAILED = 'Não foi possível pedir ao mestre. Tente de novo.';
export const FAVOR_CANCEL_FAILED = 'Não foi possível cancelar o pedido. Tente de novo.';

/** A reader's line of the notices region (the giver's, the roller's, the master's). */
export interface FavorLine {
  readonly id: number;
  readonly text: string;
  /** `success`: something good reached the reader; `neutral`: a plain fact (a refusal, a card that closed, nothing changed). */
  readonly tone: 'success' | 'neutral';
  readonly icon: string;
}

/** What the answer to a pass or a question of the master came to. */
export interface FavorOutcome {
  /** The card is gone: the call (or the one before it) closed the question. A refusal that leaves it open has `false`. */
  readonly closes: boolean;
  /** The master reads this, in the notice. */
  readonly text: string;
  /** The Inspiração answered the "Devolver" with `ALREADY_HAS`: the question is still open on the server, so the screen keeps it spent. */
  readonly keep?: boolean;
}

/** What a refused answer to a pass says to the master, and whether the card stays (it stays on `CANNOT_RECEIVE` and `NOT_THE_HOLDER`
 * of an accept: the server leaves the ask pending so that it can be refused). */
export function passAnswerOutcome(err: unknown, pass: FavorPassVm): FavorOutcome {
  switch (favorBlocked(err)?.reason) {
    case FavorBlockedReason.PASS_CLOSED:
      return { closes: true, text: favorCanceledText(pass.fromName) };
    case FavorBlockedReason.CANNOT_RECEIVE:
      return { closes: false, text: FAVOR_CANNOT_RECEIVE };
    case FavorBlockedReason.NOT_THE_HOLDER:
      return {
        closes: false,
        text: `${pass.fromName} não está mais com a Inspiração. Recuse o pedido.`,
      };
    default:
      return { closes: false, text: FAVOR_ANSWER_FAILED };
  }
}

/** What a refused answer to the refund question says to the master. */
export function refundAnswerOutcome(err: unknown, prompt: FavorRefundVm): FavorOutcome {
  switch (favorBlocked(err)?.reason) {
    case FavorBlockedReason.PASS_CLOSED:
      return { closes: true, text: FAVOR_ANSWERED_ELSEWHERE };
    case FavorBlockedReason.ALREADY_HAS:
      return { closes: true, keep: true, text: favorHasAgainText(prompt.characterName) };
    case FavorBlockedReason.CANNOT_RECEIVE:
      return { closes: false, text: FAVOR_CANNOT_RECEIVE };
    default:
      return { closes: false, text: FAVOR_ANSWER_FAILED };
  }
}

/** What a refused ask says to the giver in the dialog, and whether what the screen knew is out of date (read again). */
export function askOutcome(err: unknown): { text: string; stale: boolean } {
  const reason = favorBlocked(err)?.reason;
  switch (reason) {
    case FavorBlockedReason.NO_SESSION:
      return { text: favorBlockedMessage(reason), stale: true };
    case FavorBlockedReason.PASS_PENDING:
      return {
        text: 'Você já pediu para passar a Inspiração. Espere o mestre ou cancele o pedido.',
        stale: true,
      };
    case FavorBlockedReason.NOT_THE_HOLDER:
      return { text: 'Você não está com a Inspiração agora.', stale: true };
    case FavorBlockedReason.NOT_A_PLAYER_CHARACTER:
      return { text: 'Essa personagem não está na mesa agora.', stale: false };
    case FavorBlockedReason.SELF:
      return { text: favorBlockedMessage(reason), stale: false };
    default:
      return { text: FAVOR_ASK_FAILED, stale: false };
  }
}

/** What a refused cancel says to the giver: the ask was answered meanwhile, so the screen reads again. */
export function cancelOutcome(err: unknown): { text: string; stale: boolean } {
  return favorBlocked(err)?.reason === FavorBlockedReason.PASS_CLOSED
    ? { text: 'Esse pedido já foi respondido.', stale: true }
    : { text: FAVOR_CANCEL_FAILED, stale: false };
}
