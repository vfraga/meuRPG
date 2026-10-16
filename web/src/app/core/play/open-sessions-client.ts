import { timestampDate } from '@bufbuild/protobuf/wkt';
import { createClient, type Transport } from '@connectrpc/connect';

import { Role } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { PlayService } from '../../../gen/meurpg/play/v1/play_pb';
import type { OpenSessionVm, OpenSessionsFetcher } from './open-sessions.types';

/**
 * `ListOpenGameSessions` over the generated `PlayService` client. Loaded
 * with a dynamic `import()` by `OpenSessions` (see `OPEN_SESSIONS_FETCHER`),
 * never statically from the shell, so the generated code stays in a lazy
 * chunk.
 */
export function createOpenSessionsFetcher(transport: Transport): OpenSessionsFetcher {
  const client = createClient(PlayService, transport);
  return async () => {
    const res = await client.listOpenGameSessions({});
    const sessions: OpenSessionVm[] = [];
    for (const open of res.openGameSessions) {
      const gs = open.gameSession;
      if (!gs) {
        continue;
      }
      sessions.push({
        sessionId: gs.id,
        campaignId: gs.campaignId,
        campaignName: open.campaignName,
        sessionNumber: gs.sessionNumber,
        startedAt: gs.startedAt ? timestampDate(gs.startedAt) : new Date(0),
        isMaster: open.myRole === Role.MASTER,
      });
    }
    // The server already sends the newest first; sorting keeps the notice
    // right even if that ever changes.
    return sessions.sort((a, b) => b.startedAt.getTime() - a.startedAt.getTime());
  };
}
