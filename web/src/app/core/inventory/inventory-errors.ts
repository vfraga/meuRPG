import { Code, ConnectError } from '@connectrpc/connect';

import {
  ItemBlockedReason,
  ItemBlockedSchema,
} from '../../../gen/meurpg/characters/v1/inventory_service_pb';
import { describeConnectError } from '../connect/connect-errors';

/** What the person was doing, to finish "Não deu para …". */
export type ItemAction =
  | 'read'
  | 'give'
  | 'equip'
  | 'remove'
  | 'transfer'
  | 'coins'
  | 'request'
  | 'identify'
  | 'rest'
  | 'use'
  | 'scroll'
  | 'charges'
  | 'ammunition'
  | 'text';

const WHAT: Record<ItemAction, string> = {
  read: 'abrir o equipamento',
  give: 'adicionar o item',
  equip: 'vestir ou guardar o item',
  remove: 'tirar o item',
  transfer: 'dar o item',
  coins: 'salvar as moedas',
  request: 'marcar o pedido',
  identify: 'identificar o item',
  rest: 'confirmar o descanso',
  use: 'usar o item',
  scroll: 'ler o pergaminho',
  charges: 'gastar as cargas',
  ammunition: 'recuperar a munição',
  text: 'transformar o texto em itens',
};

/** The reason the server refused a call with, when it said one. */
export function itemBlocked(err: unknown): { reason: ItemBlockedReason; have: number } | null {
  const blocked = ConnectError.from(err, Code.Unavailable).findDetails(ItemBlockedSchema)[0];
  return blocked ? { reason: blocked.reason, have: blocked.have } : null;
}

/** The sentence for a refusal (RN-10: it never names what an unidentified item is). */
export function blockedText(reason: ItemBlockedReason, have = 0): string {
  switch (reason) {
    case ItemBlockedReason.SLOT_TAKEN:
      return 'Esse lugar do corpo já está ocupado. Guarde o outro item antes.';
    case ItemBlockedReason.NOT_EQUIPPABLE:
      return 'Esse item não se veste nem se empunha.';
    case ItemBlockedReason.NOT_ATTUNABLE:
      return 'Esse item não pede sintonia.';
    case ItemBlockedReason.ATTUNEMENT_FULL:
      return 'Você já tem 3 itens sintonizados. Encerre a sintonia de um para marcar outro.';
    case ItemBlockedReason.ATTUNEMENT_RESTRICTION:
      return 'Esse item só pode ser sintonizado por quem cumpre o que ele pede.';
    case ItemBlockedReason.COPY_ATTUNED:
      return 'Você já está sintonizado a um item igual a este.';
    case ItemBlockedReason.ALREADY_ATTUNED:
      return 'Você já está sintonizado a este item.';
    case ItemBlockedReason.NOT_ATTUNED:
      return 'Você não está sintonizado a este item.';
    case ItemBlockedReason.ITEM_CURSED:
      return 'Você não consegue se desfazer deste item. Ele está amaldiçoado. Só uma magia, como Remover Maldição, ou o mestre resolve.';
    case ItemBlockedReason.IN_COMBAT:
      return 'Há um combate em andamento. Isto espera o fim do combate.';
    case ItemBlockedReason.NOT_ENOUGH:
      return have > 0 ? `Só há ${have}.` : 'Não há o bastante.';
    case ItemBlockedReason.NOT_USABLE:
      return 'Esse item não se usa assim.';
    case ItemBlockedReason.ALREADY_IDENTIFIED:
      return 'Esse item já foi identificado.';
    case ItemBlockedReason.NOTHING_TO_ANSWER:
      return 'Não há pedido para responder.';
    case ItemBlockedReason.REQUEST_WAITING:
      return 'Já há um pedido deste item esperando o descanso curto.';
    case ItemBlockedReason.SCROLL_UNREADABLE:
      return 'Você não consegue ler este pergaminho. A magia não está na lista de nenhuma classe sua. O pergaminho continua com você.';
    case ItemBlockedReason.SCROLL_TOO_HIGH:
      return 'A magia é de nível maior que o seu maior espaço: faça o teste para conjurá-la.';
    case ItemBlockedReason.FULL:
      return 'A lista de itens está cheia. Tire um item antes.';
    case ItemBlockedReason.USE_IN_COMBAT:
      return 'Em combate isso é uma ação: use pela tela do combate.';
    case ItemBlockedReason.CAST_IT:
      return 'Esse item guarda uma magia: conjure pela ficha.';
    case ItemBlockedReason.CANNOT_RECEIVE:
      return 'Esse personagem não pode receber itens.';
    case ItemBlockedReason.DEAD:
      return 'Esse personagem está morto.';
    case ItemBlockedReason.ARMOR_IN_COMBAT:
      return 'Vestir ou tirar armadura não cabe num combate. Faça no fim do combate.';
    case ItemBlockedReason.NOTHING_SPENT:
      return 'Não há munição gasta para recuperar.';
    default:
      return 'Não dá para fazer isso agora.';
  }
}

/** The Portuguese message for a failed call of the inventory: what happened and how to fix it, by code and typed detail. */
export function itemErrorMessage(err: unknown, action: ItemAction): string {
  const connectErr = ConnectError.from(err, Code.Unavailable);
  if (connectErr.code === Code.FailedPrecondition) {
    const blocked = itemBlocked(connectErr);
    if (blocked) {
      return blockedText(blocked.reason, blocked.have);
    }
    return 'Não dá para fazer isso agora. Olhe a ficha e tente de novo.';
  }
  return describeConnectError(connectErr, {
    [Code.InvalidArgument]: `Não deu para ${WHAT[action]}: confira os dados e tente de novo.`,
    [Code.NotFound]: 'Esse item ou personagem não existe mais, ou você não o vê. Recarregue a página.',
    [Code.PermissionDenied]: 'Você não pode fazer isso.',
    [Code.Aborted]: 'Algo mudou enquanto você agia. Tente de novo.',
    [Code.Unavailable]: `Não deu para ${WHAT[action]}: o servidor não respondeu. Tente de novo.`,
  });
}
