import { Code, ConnectError } from '@connectrpc/connect';

import { FavorBlockedReason, FavorBlockedSchema } from '../../../gen/meurpg/play/v1/favor_pb';
import type {
  FavorClient,
  FavorHolderVm,
  FavorPassVm,
  FavorReadVm,
  FavorRefundVm,
} from './favor-client';
import { noOpenSession } from './table-screen-testing';

export { noOpenSession };

/** A `failed_precondition` with the typed `FavorBlocked` detail the server sends. */
export function favorRefused(reason: FavorBlockedReason): ConnectError {
  return new ConnectError('blocked', Code.FailedPrecondition, undefined, [
    { desc: FavorBlockedSchema, value: { reason } },
  ]);
}

/** A pass as the read gives it, with the names the specs use (Brisa gives to Tor). */
export function favorPass(extra: Partial<FavorPassVm> = {}): FavorPassVm {
  return {
    id: 'pass-1',
    fromId: 'brisa',
    fromName: 'Brisa',
    toId: 'tor',
    toName: 'Tor',
    receiverHas: false,
    ...extra,
  };
}

/** A refund question as the read gives it (Jabuti's attack, a hidden advantage). */
export function favorRefund(extra: Partial<FavorRefundVm> = {}): FavorRefundVm {
  return {
    id: 'refund-1',
    characterId: 'jabuti',
    characterName: 'Jabuti',
    rollLabel: 'Ataque',
    hidden: [{ name: 'Alvo Impedido', advantage: true }],
    ...extra,
  };
}

/** A `FavorClient` the specs steer: what the read answers, and what the grant, the pass and the answers do. */
export class FakeFavorClient {
  /** What `read` answers; a function can throw to be a failed read. */
  holders: readonly FavorHolderVm[] | (() => readonly FavorHolderVm[]) = [];
  /** The asks and the questions the read gives. */
  passes: readonly FavorPassVm[] = [];
  refunds: readonly FavorRefundVm[] = [];
  /** What `grant` does: `null` is a grant that works. */
  grantError: unknown = null;
  /** What each call of the pass does: `null` works. */
  passError: unknown = null;
  cancelError: unknown = null;
  answerPassError: unknown = null;
  answerRefundError: unknown = null;
  /** What `pass` answers when it works. */
  passAnswer: FavorPassVm | null = null;
  readonly reads: string[] = [];
  readonly grants: { campaignId: string; characterId: string; key: string }[] = [];
  readonly asks: { campaignId: string; fromId: string; toId: string; key: string }[] = [];
  readonly cancels: { campaignId: string; passId: string; key: string }[] = [];
  readonly passAnswers: { campaignId: string; passId: string; accept: boolean; key: string }[] = [];
  readonly refundAnswers: {
    campaignId: string;
    promptId: string;
    giveBack: boolean;
    key: string;
  }[] = [];
  /** Calls that wait for the spec to let them go (an in-flight button): the resolver of each is here, newest last. */
  hold: 'none' | 'ask' | 'cancel' | 'answerPass' | 'answerRefund' = 'none';
  readonly release: (() => void)[] = [];

  read(campaignId: string): Promise<FavorReadVm> {
    this.reads.push(campaignId);
    try {
      const holders = typeof this.holders === 'function' ? this.holders() : this.holders;
      return Promise.resolve({ holders, passes: this.passes, refunds: this.refunds });
    } catch (err) {
      return Promise.reject(err);
    }
  }

  grant(campaignId: string, characterId: string, key: string): Promise<void> {
    this.grants.push({ campaignId, characterId, key });
    return this.grantError ? Promise.reject(this.grantError) : Promise.resolve();
  }

  async pass(
    campaignId: string,
    fromId: string,
    toId: string,
    key: string,
  ): Promise<FavorPassVm | null> {
    this.asks.push({ campaignId, fromId, toId, key });
    await this.waitIf('ask');
    if (this.passError) {
      throw this.passError;
    }
    return this.passAnswer;
  }

  async cancelPass(campaignId: string, passId: string, key: string): Promise<void> {
    this.cancels.push({ campaignId, passId, key });
    await this.waitIf('cancel');
    if (this.cancelError) {
      throw this.cancelError;
    }
  }

  async answerPass(
    campaignId: string,
    passId: string,
    accept: boolean,
    key: string,
  ): Promise<void> {
    this.passAnswers.push({ campaignId, passId, accept, key });
    await this.waitIf('answerPass');
    if (this.answerPassError) {
      throw this.answerPassError;
    }
  }

  async answerRefund(
    campaignId: string,
    promptId: string,
    giveBack: boolean,
    key: string,
  ): Promise<void> {
    this.refundAnswers.push({ campaignId, promptId, giveBack, key });
    await this.waitIf('answerRefund');
    if (this.answerRefundError) {
      throw this.answerRefundError;
    }
  }

  private waitIf(call: typeof this.hold): Promise<void> {
    return this.hold === call
      ? new Promise<void>((resolve) => this.release.push(resolve))
      : Promise.resolve();
  }

  asClient(): FavorClient {
    return this as unknown as FavorClient;
  }
}

/** A read that fails the way a down network does. */
export function failedRead(): never {
  throw new ConnectError('down', Code.Unavailable);
}

/** Puts a `FavorState` where a session is open and `holders` (id to name) have the Inspiração, with no read. */
export function openFavor(
  favor: {
    campaignId: { set(v: string): void };
    holders: { set(v: ReadonlyMap<string, string>): void };
  },
  holders: Record<string, string> = {},
): void {
  favor.campaignId.set('camp');
  favor.holders.set(new Map(Object.entries(holders)));
}
