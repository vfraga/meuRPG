import { Code, ConnectError } from '@connectrpc/connect';

import {
  CharacterBlockedReason,
  CharacterBlockedSchema,
  type ItemRecharge,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import type { ConsumableKind } from '../characters/consumable-vm';
import { kindWords, type TurnBlock } from '../characters/consumables';
import { blockedMessage, encounterBlocked, sessionClosed } from '../combat/combat-errors';
import { EncounterBlockedReason } from '../../../gen/meurpg/play/v1/combat_pb';
import { noChargesWithRecharge } from '../characters/recharge-text';
import { articleOf } from '../characters/charges';
import { describeCharacterError } from '../characters/character-errors';
import {
  ChargesBlockedReason,
  type ChargesBlocked,
  ChargesBlockedSchema,
} from '../../../gen/meurpg/play/v1/charges_pb';

/** The typed detail of a `failed_precondition` from a charges or ammunition call, or `null`. Never read from the message. */
export function chargesBlocked(err: unknown): ChargesBlocked | null {
  const connectErr = ConnectError.from(err, Code.Unavailable);
  if (connectErr.code !== Code.FailedPrecondition) {
    return null;
  }
  return connectErr.findDetails(ChargesBlockedSchema)[0] ?? null;
}

/** What each refusal says, in words the master or the player can act on. */
export function chargesBlockedMessage(blocked: ChargesBlocked): string {
  switch (blocked.reason) {
    case ChargesBlockedReason.REGAIN_FACE_NEEDED:
      return `Informe a face do d${blocked.needed} que conta as cargas devolvidas.`;
    case ChargesBlockedReason.COMBAT_RUNNING:
      return 'O combate ainda não acabou: recolha a munição depois dele.';
    case ChargesBlockedReason.ALREADY_RECOVERED:
      return 'A munição desse combate já foi recolhida.';
    default:
      return 'As cargas ou a munição não podem ser usadas agora.';
  }
}

/** The item an error is about: the words of the refusal name it and say how many charges it holds. */
export interface ChargesErrorItem {
  readonly itemName: string;
  readonly left: number;
  /** The item's recharge rules, when the sheet has them: "Sem cargas" then says when the item comes back. */
  readonly recharge?: ItemRecharge;
}

const NOT_GOT_THROUGH = new Set<Code>([
  Code.Unavailable,
  Code.Unknown,
  Code.Internal,
  Code.DeadlineExceeded,
]);

/** The call did not reach the server or was cut off: nothing changed, so there is nothing to read again. */
export function chargesDidNotGetThrough(err: unknown): boolean {
  return NOT_GOT_THROUGH.has(ConnectError.from(err, Code.Unavailable).code);
}

/**
 * What a refused or failed charges call says on the sheet, by the typed reason and never by the server's message: the
 * `ChargesBlocked` sentences, then the item's own refusals (not enough charges, none, destroyed) with the item named,
 * "Nada mudou" for a call that did not get through (`doing` says what the call was: "gastar a carga", "salvar", "amanhecer"), and the character sentences for the rest.
 */
export function chargesErrorMessage(
  err: unknown,
  item: ChargesErrorItem,
  doing = 'gastar a carga',
): string {
  const blocked = chargesBlocked(err);
  if (blocked) {
    return chargesBlockedMessage(blocked);
  }
  const connectErr = ConnectError.from(err, Code.Unavailable);
  if (connectErr.code === Code.FailedPrecondition) {
    const reason = connectErr.findDetails(CharacterBlockedSchema)[0]?.reason;
    switch (reason) {
      case CharacterBlockedReason.CHARGES_NOT_ENOUGH:
        return `Cargas a menos. ${articleOf(item.itemName)} ${item.itemName} só tem ${item.left} ${item.left === 1 ? 'carga' : 'cargas'}.`;
      case CharacterBlockedReason.ITEM_NO_CHARGES:
        return item.recharge
          ? noChargesWithRecharge(item.itemName, item.recharge)
          : `Sem cargas. ${articleOf(item.itemName)} ${item.itemName} não tem cargas agora.`;
      case CharacterBlockedReason.ITEM_DESTROYED:
        return `Item destruído. ${articleOf(item.itemName)} ${item.itemName} não tem mais uso.`;
    }
  }
  if (chargesDidNotGetThrough(err)) {
    return `Não foi possível ${doing}. Nada mudou. Tente de novo.`;
  }
  return describeCharacterError(err);
}

/** Who and what a refused or failed use of a consumable is about, for the words of its refusals. */
export interface ConsumableErrorContext {
  readonly kind: ConsumableKind;
  /** The name of the character that holds the item: the refusals about "this character" say who. */
  readonly userName: string;
  /** The receiver the call named, when it was a gift: the refusals that are about them say their name. */
  readonly targetName?: string;
  /** The receiver's id, to tell whose death `CHARACTER_DEAD` is. */
  readonly targetId?: string;
  /** The character the master acts for: "Só na vez de Tor". Absent for the player's own use. */
  readonly actingFor?: string;
  /** The call carried the faces of typed dice: only then can an `invalid_argument` be about them. */
  readonly typedDice?: boolean;
}

/** What a refusal is about, for what the sheet does next: the receiver (read the list again), the item, or neither. */
export type RefusalCause = 'target-dead' | 'target-away' | 'item-gone' | 'other';

/**
 * A refusal or failure of a use as the sheet shows it: a bold lead, the rest of the sentence, and how it looks. A turn or an
 * action the character does not have is a ban line (the look of a folded group's reason); every other refusal is a red notice.
 */
export interface ConsumableRefusal {
  readonly lead: string;
  readonly text: string;
  readonly look: 'danger' | 'ban';
  /** The call never reached the server: the sheet offers to send it again. */
  readonly retry: boolean;
  readonly cause: RefusalCause;
  /** The turn or the action is what keeps the use: the sheet shows it as its ban line and offers nothing under it. */
  readonly block?: TurnBlock;
}

function refusal(
  lead: string,
  text: string,
  more: Partial<Pick<ConsumableRefusal, 'look' | 'retry' | 'cause' | 'block'>> = {},
): ConsumableRefusal {
  return { lead, text, look: 'danger', retry: false, cause: 'other', ...more };
}

/** What keeps a use from the Action, in two parts: a bold lead and the rest of the sentence. */
export interface TurnBlockParts {
  readonly lead: string;
  readonly text: string;
}

/** The line of a turn that does not allow the use, the one the server's refusals say too: "Só na sua vez. Beber ou dar gasta a ação.", "Ação já usada. Você já usou a ação desta vez.". */
export function turnBlockParts(block: TurnBlock, actingFor: string | null): TurnBlockParts {
  return block === 'turn'
    ? {
        lead: `Só na ${actingFor ? `vez de ${actingFor}` : 'sua vez'}.`,
        text: 'Beber ou dar gasta a ação.',
      }
    : { lead: 'Ação já usada.', text: `${actingFor ?? 'Você'} já usou a ação desta vez.` };
}

/** The line of a turn that does not allow the use, as one sentence. */
export function turnBlockMessage(block: TurnBlock, actingFor: string | null): string {
  const { lead, text } = turnBlockParts(block, actingFor);
  return `${lead} ${text}`;
}

/** What a refusal of the combat says: the turn, the action, the receiver out of reach, down or dead, the item gone, and the reasons the combat has sentences for. */
function combatRefusal(
  encounter: NonNullable<ReturnType<typeof encounterBlocked>>,
  ctx: ConsumableErrorContext,
): ConsumableRefusal {
  const words = kindWords(ctx.kind);
  const target = ctx.targetName ?? 'Quem ia receber';
  switch (encounter.reason) {
    case EncounterBlockedReason.NOT_YOUR_TURN:
    case EncounterBlockedReason.ACTION_USED: {
      const block = encounter.reason === EncounterBlockedReason.NOT_YOUR_TURN ? 'turn' : 'action';
      const { lead, text } = turnBlockParts(block, ctx.actingFor ?? null);
      return refusal(lead, text, { look: 'ban', block });
    }
    case EncounterBlockedReason.COMBATANT_DOWN:
      return refusal('Quem está caído não age.', `Mas ainda pode receber ${words.the}.`);
    // A creature at three failed death saves still reads "Caída" in the list of a player, and the server refuses it.
    case EncounterBlockedReason.TARGET_DEAD:
      return refusal(
        `${target} morreu.`,
        `Não dá para dar ${words.the} a quem morreu. Nada foi gasto.`,
        { cause: 'target-dead' },
      );
    case EncounterBlockedReason.TARGET_NOT_ADJACENT:
      return refusal(
        'Longe demais.',
        `${target} não está mais ${ctx.actingFor ? `ao lado de ${ctx.actingFor}` : 'ao seu lado'}. Nada foi gasto.`,
        { cause: 'target-away' },
      );
    case EncounterBlockedReason.ITEM_GONE:
      return goneSheetRefusal(ctx.kind);
  }
  return refusal('Não foi possível.', blockedMessage(encounter));
}

/** The refusal of an item that is gone and whose sheet was read again: it was used, and the sheet is up to date. */
export function goneSheetRefusal(kind: ConsumableKind): ConsumableRefusal {
  return refusal(`${kindWords(kind).goneUsed}.`, 'A ficha foi atualizada.', { cause: 'item-gone' });
}

/**
 * The refusal of an item that is gone while the page could not read the sheet again (or the read was overtaken): the lead says
 * the item was used, and the rest does not say the sheet was updated, because it was not. It offers to read it again.
 */
export function unreadSheetRefusal(kind: ConsumableKind): ConsumableRefusal {
  return refusal(`${kindWords(kind).goneUsed}.`, 'Não conseguimos reler a ficha.', {
    retry: true,
    cause: 'item-gone',
  });
}

/** Whether the sheet must be read again after a failed use: everything but a call that never reached the server. */
export function consumableNeedsReload(err: unknown): boolean {
  return ConnectError.from(err, Code.Unavailable).code !== Code.Unavailable;
}

/**
 * What a refused or failed use of a consumable says in the "Usar" sheet, by the typed reason and never by the server's
 * message: a bold lead and the sentence. The refusals that spend nothing say so; a call that was cut off or failed on the
 * server says "Não sabemos se a unidade foi usada: confira a ficha", since it may have spent the unit; one that never left says nothing
 * was spent and can be sent again. The turn and the action of a combat are ban lines.
 */
export function consumableRefusal(err: unknown, ctx: ConsumableErrorContext): ConsumableRefusal {
  const encounter = encounterBlocked(err);
  if (encounter) {
    return combatRefusal(encounter, ctx);
  }
  if (sessionClosed(err)) {
    return refusal('Sem sessão aberta.', 'Só dá para usar durante a sessão. Nada foi gasto.');
  }
  const connectErr = ConnectError.from(err, Code.Unavailable);
  return characterDeadRefusal(connectErr, ctx) ?? codeRefusal(connectErr, ctx, err);
}

/** A dead user or receiver: the one whose death the server names. */
function characterDeadRefusal(
  connectErr: ConnectError,
  ctx: ConsumableErrorContext,
): ConsumableRefusal | null {
  if (connectErr.code !== Code.FailedPrecondition) {
    return null;
  }
  const blocked = connectErr.findDetails(CharacterBlockedSchema)[0];
  if (blocked?.reason !== CharacterBlockedReason.CHARACTER_DEAD) {
    return null;
  }
  const forTarget = !!ctx.targetId && blocked.characterId === ctx.targetId;
  const who = forTarget ? (ctx.targetName ?? 'Quem ia receber') : ctx.userName;
  return refusal(`${who} morreu.`, 'Quem morreu não usa nem recebe itens. Nada foi gasto.', {
    cause: forTarget ? 'target-dead' : 'other',
  });
}

/** What the call's own code says, when no typed reason does. */
function codeRefusal(
  connectErr: ConnectError,
  ctx: ConsumableErrorContext,
  err: unknown,
): ConsumableRefusal {
  const words = kindWords(ctx.kind);
  switch (connectErr.code) {
    case Code.InvalidArgument:
      return ctx.typedDice
        ? refusal(
            'Dados inválidos.',
            'Cada dado vai de 1 até o número de faces dele. Nada foi gasto.',
          )
        : refusal('Pedido inválido.', `Não deu para usar ${words.the}. Nada foi gasto.`);
    case Code.NotFound:
      return ctx.targetName
        ? refusal('Fora da mesa.', `${ctx.targetName} não está mais na mesa. Nada foi gasto.`, {
            cause: 'target-away',
          })
        : refusal('Fora da mesa.', `${ctx.userName} não está mais na mesa. Nada foi gasto.`);
    case Code.Unavailable:
      return refusal('Sem conexão.', `Não deu para usar ${words.the}. Nada foi gasto.`, {
        retry: true,
      });
    case Code.Unknown:
    case Code.Internal:
    case Code.DeadlineExceeded:
      return refusal('Não sabemos se a unidade foi usada:', 'confira a ficha.');
  }
  return refusal('Não foi possível.', describeCharacterError(err));
}

/**
 * What a failed read of who may receive the item says, by the typed reason: the item is gone, the character is dead or no
 * longer at the table, and for anything else that the list could not be read (and can be read again). It never speaks of
 * typed dice or of a receiver, which the list read has none of.
 */
export function consumableTargetsRefusal(
  err: unknown,
  ctx: ConsumableErrorContext,
): ConsumableRefusal {
  const words = kindWords(ctx.kind);
  if (encounterBlocked(err)?.reason === EncounterBlockedReason.ITEM_GONE) {
    return goneSheetRefusal(ctx.kind);
  }
  const connectErr = ConnectError.from(err, Code.Unavailable);
  if (
    connectErr.code === Code.FailedPrecondition &&
    connectErr.findDetails(CharacterBlockedSchema)[0]?.reason ===
      CharacterBlockedReason.CHARACTER_DEAD
  ) {
    return refusal(`${ctx.userName} morreu.`, 'Quem morreu não usa nem recebe itens.');
  }
  if (connectErr.code === Code.NotFound) {
    return refusal('Fora da mesa.', `${ctx.userName} não está mais na mesa.`);
  }
  return refusal('Sem a lista.', `Não deu para ver quem pode receber ${words.the}.`, {
    retry: true,
  });
}
