import { createClient } from '@connectrpc/connect';
import { CombatService } from '../../../gen/meurpg/play/v1/combat_pb';
import { newKey } from '../connect/idempotency';
import { CONNECT_TRANSPORT } from '../connect/transport';
import { inject } from '@angular/core';

/** What the parts of the combat client share: the generated client and the keyed sender. */
export class CombatClientBase {
  readonly client = createClient(CombatService, inject(CONNECT_TRANSPORT));
  /** The key of each change still waiting for its answer, by the request it carries. */
  private readonly sending = new Map<string, string>();

  /**
   * Sends a change under the key of its request: the same request again (a second tap, or a try after a lost
   * answer) keeps the key, so the server answers with what the first call did; other values are another change
   * with a new key. Once the change worked the next one, even with the same values (two equal hits), is new.
   * A `given` key is the caller's, kept across its own retries.
   */
  async keyed<T>(
    what: readonly unknown[],
    call: (key: string) => Promise<T>,
    given?: string,
  ): Promise<T> {
    if (given !== undefined) {
      return call(given);
    }
    const print = JSON.stringify(what, (_name, value: unknown) =>
      typeof value === 'bigint' ? value.toString() : value,
    );
    let key = this.sending.get(print);
    if (key === undefined) {
      key = newKey();
      this.sending.set(print, key);
    }
    const res = await call(key);
    this.sending.delete(print);
    return res;
  }
}
