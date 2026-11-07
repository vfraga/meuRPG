import { Injectable, inject } from '@angular/core';
import { createClient } from '@connectrpc/connect';

import {
  type GetTableScreenResponse,
  type SetTableScreenResponse,
  TableScreenKind,
  TableScreenService,
  type WatchTableScreenResponse,
} from '../../../gen/meurpg/play/v1/table_screen_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';

/** What the master chose for the TV: follow the table, or pin one kind of screen. */
export interface TableScreenPick {
  readonly mirror: boolean;
  /** Read by the server only while `mirror` is off. */
  readonly pin: TableScreenKind;
}

/**
 * The only client of `TableScreenService`: the master's panel and the TV tab both go through it. Every call
 * is the master's (the server answers anyone else `permission_denied` or `not_found`). Callers map errors with
 * `tableScreenErrorKind`. `providedIn: 'root'`, imported only by lazy code.
 */
@Injectable({ providedIn: 'root' })
export class TableScreenClient {
  private readonly client = createClient(TableScreenService, inject(CONNECT_TRANSPORT));

  /** The choice, the options, the presence and what the TV shows now. */
  get(campaignId: string): Promise<GetTableScreenResponse> {
    return this.client.getTableScreen({ campaignId });
  }

  /** Stores the choice read at `choiceRevision`; a newer revision on the server is `aborted`. */
  set(
    campaignId: string,
    choiceRevision: number,
    pick: TableScreenPick,
  ): Promise<SetTableScreenResponse> {
    return this.client.setTableScreen({
      campaignId,
      choiceRevision,
      choice: { mirror: pick.mirror, pin: pick.pin },
    });
  }

  /** The TV's stream: carries nothing but "read again", until the session ends. */
  watch(campaignId: string, signal: AbortSignal): AsyncIterable<WatchTableScreenResponse> {
    return this.client.watchTableScreen({ campaignId }, { signal });
  }
}
