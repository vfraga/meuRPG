import { Code, ConnectError } from '@connectrpc/connect';

import { describeConnectError } from '../../core/connect/connect-errors';

/** What a link's lookup answered, for the dialogs. */
export type LookupState<T> =
  | { readonly status: 'loading' }
  | { readonly status: 'ready'; readonly value: T }
  | { readonly status: 'gone' }
  | { readonly status: 'error'; readonly message: string };

/** `not_found` means the target was deleted (or is not the campaign's):
 * "(mapa apagado)"; anything else is an error to retry. */
export function lookupFailure<T>(err: unknown): LookupState<T> {
  if (ConnectError.from(err, Code.Unavailable).code === Code.NotFound) {
    return { status: 'gone' };
  }
  return { status: 'error', message: describeConnectError(err, {}) };
}

/** The copy of a failed save, by code. The server's own message is never
 * shown (it names the field and the rule, not the person's fix). */
export function saveErrorMessage(err: unknown): string {
  return describeConnectError(err, {
    [Code.InvalidArgument]:
      'O servidor não aceitou o texto. Ele passa de 200 KB, ou tem um caractere de controle que não dá para salvar. Encurte ou apague o trecho e salve de novo.',
    [Code.PermissionDenied]: 'Só o mestre da campanha edita o documento.',
    [Code.NotFound]: 'Essa campanha não existe mais, ou você não é membro dela.',
    [Code.Unauthenticated]:
      'Sua sessão expirou. Entre de novo em outra aba e volte a esta para salvar: entrar nesta aba recarrega a página e perde o texto.',
    [Code.Unavailable]:
      'Não deu para salvar: a conexão caiu. O texto continua aqui. Tente de novo.',
  });
}

export const CONFLICT_MESSAGE = 'Este documento mudou em outra aba ou em outro aparelho.';

export function isConflict(err: unknown): boolean {
  return ConnectError.from(err, Code.Unavailable).code === Code.Aborted;
}
