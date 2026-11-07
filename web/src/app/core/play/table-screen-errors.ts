import { Code, ConnectError } from '@connectrpc/connect';

import { classifyLiveError } from './live-error';
import type { LiveErrorKind } from './live-session.types';

/**
 * What a failed `TableScreenService` call means:
 * - `no-access`: `permission_denied` or `not_found`, a player or a stranger opening the master's screen (the
 *   page says nothing of the campaign);
 * - `stale`: `aborted`, the choice changed since it was read;
 * - the rest as `classifyLiveError` reads them (`no-session` is `NO_OPEN_SESSION`).
 */
export type TableScreenErrorKind = Exclude<LiveErrorKind, 'forbidden'> | 'stale';

export function tableScreenErrorKind(err: unknown): TableScreenErrorKind {
  if (ConnectError.from(err, Code.Unavailable).code === Code.Aborted) {
    return 'stale';
  }
  const kind = classifyLiveError(err);
  return kind === 'forbidden' ? 'no-access' : kind;
}

/** For `LiveStream`: both refusals end the TV's stream for good, which `LiveStream` reads as `no-access`. */
export function classifyTvStreamError(err: unknown): LiveErrorKind {
  const kind = tableScreenErrorKind(err);
  return kind === 'stale' ? 'transient' : kind;
}
