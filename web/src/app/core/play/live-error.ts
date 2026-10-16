import { Code, ConnectError } from '@connectrpc/connect';

import {
  GameSessionBlockedReason,
  GameSessionBlockedSchema,
} from '../../../gen/meurpg/play/v1/play_pb';
import type { LiveErrorKind } from './live-session.types';

/**
 * Maps a Connect error to what it means for the page, by code and by the
 * `GameSessionBlocked` detail, never by the message (play.proto lists which
 * method returns what).
 */
export function classifyLiveError(err: unknown): LiveErrorKind {
  const connectErr = ConnectError.from(err, Code.Unavailable);
  switch (connectErr.code) {
    case Code.NotFound:
      return 'no-access';
    case Code.Unauthenticated:
      return 'signed-out';
    case Code.InvalidArgument:
      return 'invalid';
    case Code.PermissionDenied:
      return 'forbidden';
    case Code.FailedPrecondition: {
      const [detail] = connectErr.findDetails(GameSessionBlockedSchema);
      return detail?.reason === GameSessionBlockedReason.NO_OPEN_SESSION
        ? 'no-session'
        : 'transient';
    }
    default:
      return 'transient';
  }
}
