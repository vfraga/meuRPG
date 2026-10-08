import { Code, ConnectError } from '@connectrpc/connect';

/** What every call says when the login session is gone: no retry fixes it, signing in again does. */
export const SESSION_ENDED = 'Sua sessão acabou. Entre de novo para continuar.';

/** What every call says when the server could not tell whether the change was saved (`unknown`): a blind retry could repeat it. */
export const OUTCOME_UNKNOWN =
  'Não deu para confirmar se a alteração foi salva. Confira na tela antes de tentar de novo.';

const GENERIC = 'Não foi possível falar com o servidor agora. Tente de novo em instantes.';

/**
 * Turns whatever a Connect call rejected with into a Portuguese message a
 * screen can show as-is.
 *
 * `messages` supplies the wording for the codes this call can meaningfully
 * fail with (see each `.proto` service comment for the list). A plain network
 * failure, which `ConnectError.from` cannot tell apart from "the server is
 * down", is `unavailable` and takes `messages[Code.Unavailable]` if given. A
 * code the screen did not word says what is true of every call: `unknown` (an
 * ambiguous commit) that the change must be checked, any other that the server
 * could not be reached or failed. The screen's `unavailable` wording is not
 * borrowed for them: it often names a cause (images off, a service down) that
 * a different failure does not share.
 */
export function describeConnectError(
  err: unknown,
  messages: Partial<Record<Code, string>>,
): string {
  const connectErr = ConnectError.from(err, Code.Unavailable);
  // "Slow down" is the same for every call: no screen's own wording applies.
  if (isRateLimited(connectErr)) {
    return rateLimitedMessage(connectErr);
  }
  const own = messages[connectErr.code];
  if (own !== undefined) {
    return own;
  }
  switch (connectErr.code) {
    case Code.Unauthenticated:
      return SESSION_ENDED;
    case Code.Unknown:
      return OUTCOME_UNKNOWN;
    case Code.Unavailable:
      return messages[Code.Unavailable] ?? GENERIC;
    default:
      return GENERIC;
  }
}

/**
 * Whether the server turned the call away for asking too often (the per-user
 * and per-address limits, docs/architecture.md#abuse-limits). It tells
 * by the `Retry-After` header, which no other refusal carries: a full gallery
 * or a campaign at its limit is also `resource_exhausted`, and waiting does
 * not fix those.
 */
export function isRateLimited(err: unknown): boolean {
  const e = ConnectError.from(err, Code.Unavailable);
  return (
    (e.code === Code.ResourceExhausted || e.code === Code.Unavailable) &&
    e.metadata.has('Retry-After')
  );
}

/** "Muitas ações em pouco tempo. Espere 3 segundos e tente de novo." */
export function rateLimitedMessage(err: unknown): string {
  const secs = Number(ConnectError.from(err, Code.Unavailable).metadata.get('Retry-After'));
  const wait =
    Number.isFinite(secs) && secs >= 1
      ? `Espere ${Math.ceil(secs)} ${Math.ceil(secs) === 1 ? 'segundo' : 'segundos'}`
      : 'Espere alguns segundos';
  return `Muitas ações em pouco tempo. ${wait} e tente de novo.`;
}
