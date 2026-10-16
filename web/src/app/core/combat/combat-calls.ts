import { signal } from '@angular/core';
import { Code, ConnectError } from '@connectrpc/connect';

import type { Encounter } from '../../../gen/meurpg/play/v1/combat_pb';
import type { CombatClient } from './combat-client';
import { combatErrorMessage } from './combat-errors';
import type { CombatState } from './combat-state';

/** What `CombatCalls` reads from the screen it serves. */
export interface CombatCallsContext {
  readonly api: Pick<CombatClient, 'get'>;
  readonly campaignId: () => string;
  /** The combat as the screen shows it, or `null` before it is read. */
  readonly encounter: () => Encounter | null;
  readonly state: () => CombatState;
}

/** The codes after which what the screen shows may be stale, so the combat is read again. */
const STALE_CODES: ReadonlySet<Code> = new Set([
  Code.Aborted,
  Code.FailedPrecondition,
  Code.NotFound,
  Code.Unavailable,
  Code.DeadlineExceeded,
]);

/**
 * The calls of the combat screen that answer with the combat: one at a time (`busy`), the answer applied at once, a refusal
 * said in Portuguese (`error`), and the combat read again when the refusal means the screen is stale. The screen and the
 * parts it is made of (moving, the sheets) share this state, so a tap in one part waits for a call in another.
 */
export class CombatCalls {
  readonly busy = signal(false);
  readonly error = signal('');

  constructor(private readonly ctx: CombatCallsContext) {}

  /** Runs a call that answers with the combat: applies it, or says why not. A refusal that means the screen is stale reads the
   * combat again. `false` when it was not made (no combat, or another call in the air) or failed. */
  async run(call: (e: Encounter) => Promise<Encounter>): Promise<boolean> {
    const e = this.ctx.encounter();
    if (!e || this.busy()) {
      return false;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      this.ctx.state().apply(await call(e));
      return true;
    } catch (err) {
      this.error.set(combatErrorMessage(err));
      await this.refreshAfter(err);
      return false;
    } finally {
      this.busy.set(false);
    }
  }

  /** Reads the combat again after a failure that may mean the screen is stale; a failed read is left to the stream's next `ready`. */
  async refreshAfter(err: unknown): Promise<void> {
    if (!STALE_CODES.has(ConnectError.from(err).code)) {
      return;
    }
    const state = this.ctx.state();
    const ticket = state.beginRead();
    try {
      state.applyRead(ticket, await this.ctx.api.get(this.ctx.campaignId()));
    } catch {
      // The stream's next `ready` reads it again.
    }
  }
}
