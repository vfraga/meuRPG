import { Injectable, inject, signal } from '@angular/core';

import type { OptionSwitchEntry } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { TableContentClient } from './content-client';

/**
 * The master's switches (`ListOptionSwitches`: SRD and table, each with `off` and `hidden`) as the page that edits a class or a
 * subclass last read them, for "Como os jogadores veem": what the players do not receive is not named there. `unread` is set while the
 * last read failed, and the preview says so in one line.
 */
@Injectable()
export class SwitchesStore {
  readonly options = signal<readonly OptionSwitchEntry[]>([]);
  readonly unread = signal(false);

  private readonly client = inject(TableContentClient);

  /** Reads the list when `wanted` (the master editing a class or a subclass); a failure is `unread`, and nothing else stops. */
  async load(campaignId: string, wanted: boolean): Promise<void> {
    if (!wanted) {
      return;
    }
    try {
      this.options.set((await this.client.switches(campaignId)).options);
      this.unread.set(false);
    } catch {
      this.unread.set(true);
    }
  }
}
