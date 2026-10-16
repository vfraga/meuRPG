import { DestroyRef, computed, effect, inject, signal, untracked } from '@angular/core';
import { Code, ConnectError } from '@connectrpc/connect';

import type {
  Encounter,
  OpportunityOffer,
  ReactionWindow,
} from '../../../gen/meurpg/play/v1/combat_pb';
import type { CombatClient } from './combat-client';
import type { CombatState } from './combat-state';
import { barText, offersToAnswer, reactorIsMasters } from './opportunity';
import { revealBarText } from './hidden-reveal';
import { AutoPassClock, autoPassKey, autoPassText } from './reaction-autopass';
import { openWindows } from './reactions';
import { windowsBarText } from './reaction-master';

/** How often the master's screen looks at the clock of the automatic pass of a player's optional reaction. */
const AUTO_PASS_TICK_MS = 1000;

/** The failures of the pass that are the network's, not a refusal: the pass may be sent again. */
const RETRYABLE: ReadonlySet<Code> = new Set([
  Code.Unavailable,
  Code.DeadlineExceeded,
  Code.Unknown,
  Code.Internal,
  Code.Aborted,
]);

/** What `MasterAutoPass` reads from the screen it serves. */
export interface AutoPassContext {
  readonly api: Pick<CombatClient, 'answerReaction' | 'skipOpportunity'>;
  readonly campaignId: () => string;
  readonly encounter: () => Encounter | null;
  readonly isMaster: () => boolean;
  readonly state: () => CombatState;
}

/** The offers of an opportunity attack that a player's character answers, which pass after 30 s like any optional reaction. */
export function autoPassOffers(e: Encounter): OpportunityOffer[] {
  return offersToAnswer(e).filter((o) => !reactorIsMasters(e, o));
}

/**
 * The master's bar: "Esperando a sua resposta: escondidas atingidas", or "Esperando a sua reação: Goblin 2", with the time
 * left of the automatic pass when a player's optional reaction has a clock (`left`, by window id).
 */
export function masterWaitNote(
  e: Encounter,
  left: Readonly<Record<string, number>>,
  playerName: (characterId: string) => string,
): string {
  const other = revealBarText(e) || barText(e, playerName);
  if (other) {
    return other;
  }
  const soonest = Math.min(...Object.values(left));
  const text = windowsBarText(e);
  return text && Number.isFinite(soonest)
    ? `${text} · ${autoPassText(soonest).toLowerCase()}`
    : text;
}

/**
 * The master's screen passes by itself the optional reactions and the opportunity attacks a player leaves unanswered
 * (`AUTO_PASS_SECONDS`): a second-by-second clock runs while any is waiting, and each one that is due is sent with a key of
 * its own (two tabs, or a retry, never answer twice). Call it in an injection context (a field initializer): it watches the
 * combat with an effect and stops its timer with the screen.
 */
export class MasterAutoPass {
  private readonly windowsClock = new AutoPassClock();
  /** The same clock for the opportunity attacks a player's character may take and has not answered. */
  private readonly offersClock = new AutoPassClock();
  private readonly now = signal(Date.now());
  private timer: ReturnType<typeof setInterval> | undefined;

  /** The seconds left of each player's optional reaction the master's screen passes by itself, by window id. */
  readonly windowsLeft = computed<Readonly<Record<string, number>>>(() =>
    this.secondsLeft(this.windowsClock, (e) => openWindows(e).map((w) => w.id)),
  );
  /** The seconds left of each such offer the master's screen passes by itself, by offer id. */
  readonly offersLeft = computed<Readonly<Record<string, number>>>(() =>
    this.secondsLeft(this.offersClock, (e) => autoPassOffers(e).map((o) => o.id)),
  );

  constructor(private readonly ctx: AutoPassContext) {
    inject(DestroyRef).onDestroy(() => clearInterval(this.timer));
    effect(() => {
      const e = this.ctx.encounter();
      const master = this.ctx.isMaster();
      untracked(() =>
        this.watch(master && e ? openWindows(e) : [], master && e ? autoPassOffers(e) : []),
      );
    });
  }

  private secondsLeft(clock: AutoPassClock, idsOf: (e: Encounter) => string[]) {
    const now = this.now();
    const e = this.ctx.encounter();
    const left: Record<string, number> = {};
    if (!e || !this.ctx.isMaster()) {
      return left;
    }
    for (const id of idsOf(e)) {
      const s = clock.secondsLeft(id, now);
      if (s !== null) {
        left[id] = s;
      }
    }
    return left;
  }

  /** Starts (or stops) the second-by-second clock of the automatic pass for the windows the master's screen waits on. */
  private watch(windows: readonly ReactionWindow[], offers: readonly OpportunityOffer[]): void {
    const now = Date.now();
    this.windowsClock.sync(windows, now);
    this.offersClock.syncIds(
      offers.map((o) => o.id),
      now,
    );
    this.now.set(now);
    const running = this.timer !== undefined;
    if (this.windowsClock.soonest(now) === null && this.offersClock.soonest(now) === null) {
      clearInterval(this.timer);
      this.timer = undefined;
    } else if (!running) {
      this.timer = setInterval(() => this.tick(), AUTO_PASS_TICK_MS);
    }
  }

  private tick(): void {
    const now = Date.now();
    this.now.set(now);
    const e = this.ctx.encounter();
    if (!e) {
      return;
    }
    for (const id of this.windowsClock.due(now)) {
      void this.passWindow(e.id, id);
    }
    for (const id of this.offersClock.due(now)) {
      void this.passOffer(e.id, id);
    }
  }

  /** The same "Seguir sem esperar" the master taps, with a key of its own; a refusal (answered meanwhile) is ignored quietly. */
  private async passOffer(encounterId: string, offerId: string): Promise<void> {
    try {
      const res = await this.ctx.api.skipOpportunity(
        this.ctx.campaignId(),
        encounterId,
        offerId,
        autoPassKey(offerId),
      );
      this.ctx.state().apply(res);
    } catch (err) {
      if (!(err instanceof ConnectError) || RETRYABLE.has(err.code)) {
        this.offersClock.release(offerId);
      }
    }
  }

  /** The same "Deixar passar" the master taps, with a key of its own; a refusal (answered meanwhile) is ignored quietly. */
  private async passWindow(encounterId: string, windowId: string): Promise<void> {
    try {
      const res = await this.ctx.api.answerReaction(
        this.ctx.campaignId(),
        encounterId,
        windowId,
        { use: false },
        autoPassKey(windowId),
      );
      this.ctx.state().apply(res.encounter);
    } catch (err) {
      if (!(err instanceof ConnectError) || RETRYABLE.has(err.code)) {
        this.windowsClock.release(windowId);
      }
    }
  }
}
