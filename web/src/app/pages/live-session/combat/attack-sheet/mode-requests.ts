import { type WritableSignal, computed, signal } from '@angular/core';

import {
  type RollMode,
  type RollModeRequest,
  RollModeRequestStatus,
} from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import type { CombatClient } from '../../../../core/combat/combat-client';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { ActionKey } from '../../../../core/connect/idempotency';
import { currentRequest } from './attack-sheet-view';

/** What `ModeRequests` reads from the sheet it asks for. */
export interface ModeRequestsContext {
  readonly api: Pick<CombatClient, 'requestRollMode' | 'cancelRollModeRequest'>;
  readonly campaignId: string;
  readonly encounterId: string;
  readonly attackerId: string;
  readonly attackKey: string;
  readonly state: Pick<CombatState, 'apply' | 'encounter'>;
  readonly targetId: () => string | null;
  /** The mode the player settled on: their pick, else the server's suggestion. */
  readonly chosen: () => RollMode;
  readonly busy: WritableSignal<boolean>;
  readonly error: WritableSignal<string>;
}

/**
 * The mode of the d20 the player asks the master for ("Pedir ao mestre"): the mode picked and why, the request as the
 * combat shows it, and the call that sends it or takes it back. Plain class the attack sheet builds.
 */
export class ModeRequests {
  /** The mode the player picked; null follows the server's suggestion. */
  readonly picked = signal<RollMode | null>(null);
  readonly reason = signal('');
  /** The request this sheet sent to the master (its state is read from the combat). */
  readonly asked = signal<RollModeRequest | null>(null);
  /** The request of this attack as the combat shows it now (waiting, answered), or none. */
  readonly request = computed(() => currentRequest(this.asked(), this.host.state.encounter()));
  readonly answered = computed(() => {
    const r = this.request();
    return r?.status === RollModeRequestStatus.ANSWERED ? r : null;
  });
  /** One key per request to the master: the same mode and reason again is a retry. */
  private readonly keys = new ActionKey();

  constructor(private readonly host: ModeRequestsContext) {}

  /** A new target starts from its own suggestion; a request for the old one is taken back. */
  drop(): void {
    const sent = this.asked();
    this.picked.set(null);
    this.reason.set('');
    this.asked.set(null);
    if (sent && sent.status === RollModeRequestStatus.PENDING) {
      void this.host.api
        .cancelRollModeRequest(
          this.host.campaignId,
          this.host.encounterId,
          sent.id,
          this.keys.keyFor({ cancel: sent.id }),
        )
        .then((e) => this.host.state.apply(e))
        .catch(() => undefined);
    }
  }

  /** "Pedir ao mestre": the mode asked for and why; the sheet waits for the answer. */
  async ask(): Promise<void> {
    const id = this.host.targetId();
    if (id === null || this.host.busy()) {
      return;
    }
    this.host.busy.set(true);
    this.host.error.set('');
    try {
      const mode = this.host.chosen();
      const reason = this.reason().trim();
      const res = await this.host.api.requestRollMode(
        this.host.campaignId,
        this.host.encounterId,
        this.host.attackerId,
        this.host.attackKey,
        id,
        mode,
        reason,
        this.keys.keyFor({ id, mode, reason }),
      );
      this.host.state.apply(res.encounter);
      this.asked.set(res.request);
    } catch (err) {
      this.host.error.set(combatErrorMessage(err, 'pedir ao mestre'));
    } finally {
      this.host.busy.set(false);
    }
  }

  /** "Cancelar pedido": the player takes it back and picks again. */
  async cancel(): Promise<void> {
    const sent = this.asked();
    if (!sent || this.host.busy()) {
      return;
    }
    this.host.busy.set(true);
    this.host.error.set('');
    try {
      const e = await this.host.api.cancelRollModeRequest(
        this.host.campaignId,
        this.host.encounterId,
        sent.id,
        this.keys.keyFor({ cancel: sent.id }),
      );
      this.host.state.apply(e);
      this.asked.set(null);
      this.picked.set(null);
      this.reason.set('');
    } catch (err) {
      this.host.error.set(combatErrorMessage(err, 'cancelar o pedido'));
    } finally {
      this.host.busy.set(false);
    }
  }
}
