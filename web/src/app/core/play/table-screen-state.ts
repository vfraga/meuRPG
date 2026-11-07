import { computed, signal } from '@angular/core';

import { clockText } from './table-screen-labels';
import { tableScreenErrorKind } from './table-screen-errors';
import { type TvScreenVm } from './table-screen-view';

/** The time source of the TV: the real one in the app, a hand-driven one in specs. */
export interface TvClock {
  now(): Date;
  /** Runs `fn` after `ms`; the returned function cancels it. */
  after(ms: number, fn: () => void): () => void;
}

export const systemClock: TvClock = {
  now: () => new Date(),
  after: (ms, fn) => {
    const timer = setTimeout(fn, ms);
    return () => clearTimeout(timer);
  },
};

/** After a failed first read the TV asks again. */
export const TV_RETRY_MS = 10_000;
/** A screen that lost its connection keeps showing the last good read this long, with a warning. */
export const TV_STALE_MS = 60_000;

/** A connection that just dropped is not shown as late for this long: the stream's own 10-minute cap reconnects in about a second. */
export const TV_LATE_GRACE_MS = 3000;

/** What the TV page draws: one case per state of the board. */
export type TvPhase =
  | { readonly kind: 'loading'; readonly text: string }
  | { readonly kind: 'error'; readonly title: string; readonly detail: string }
  | {
      readonly kind: 'ready';
      readonly screen: TvScreenVm;
      /** The warning of a connection lost less than a minute ago; `null` while connected. */
      readonly late: string | null;
    }
  | {
      readonly kind: 'stale';
      readonly title: string;
      readonly detail: string;
      /** The names of the last good read, for the header; empty when nothing was read. */
      readonly campaignName: string;
      readonly sessionLabel: string;
    }
  | {
      readonly kind: 'ended';
      readonly title: string;
      readonly farewell: string;
      readonly hint: string;
      /** The names of the last good read, for the header; empty when nothing was read. */
      readonly campaignName: string;
      readonly sessionLabel: string;
      /** The hour the end was heard, "23:40"; empty when unknown. */
      readonly endedAt: string;
    }
  | { readonly kind: 'no-access'; readonly title: string; readonly detail: string }
  /** The sign-in is gone: the page sends the viewer to the login. */
  | { readonly kind: 'signed-out'; readonly title: string; readonly detail: string };

/** One way the screen can be behind (the stream down, or reads failing): when it began, whether it is worth a warning
 * yet, and whether it lasted a minute. */
class Outage {
  readonly since = signal<Date | null>(null);
  readonly late = signal(false);
  readonly stale = signal(false);
  private cancels: (() => void)[] = [];

  constructor(private readonly clock: TvClock) {}

  begin(): void {
    if (this.since() !== null) {
      return;
    }
    this.since.set(this.clock.now());
    this.cancels = [
      this.clock.after(TV_LATE_GRACE_MS, () => this.late.set(true)),
      this.clock.after(TV_STALE_MS, () => {
        this.late.set(true);
        this.stale.set(true);
      }),
    ];
  }

  end(): void {
    this.drop();
    this.since.set(null);
    this.late.set(false);
    this.stale.set(false);
  }

  /** Cancels the timers, leaving the state as it is. */
  drop(): void {
    this.cancels.forEach((cancel) => cancel());
    this.cancels = [];
  }
}

/**
 * The TV's lifecycle as signals: loading, the screen with an optional "late" warning, the hidden screen after a
 * minute without a connection, the end of the session and the refusal. The inputs are the watcher's: a good
 * read, a failed read, the connection coming and going, the end. The 10 s retry and the minute are the
 * clock's, so a spec drives them without timers.
 */
export class TvScreenState {
  private readonly screen = signal<TvScreenVm | null>(null);
  private readonly readFailing = signal(false);
  private readonly connectionOutage: Outage;
  private readonly readOutage: Outage;
  private readonly signedOutNow = signal(false);
  private readonly endedAt = signal<Date | null>(null);
  private readonly ending = signal(false);
  private readonly refused = signal(false);
  private cancelRetry: (() => void) | null = null;

  /** `readAgain` is asked after a failed first read, once `TV_RETRY_MS` has passed. */
  constructor(
    private readonly clock: TvClock,
    private readonly readAgain: () => void,
  ) {
    this.connectionOutage = new Outage(clock);
    this.readOutage = new Outage(clock);
  }

  readonly phase = computed<TvPhase>(() => {
    const screen = this.screen();
    if (this.signedOutNow()) {
      return {
        kind: 'signed-out',
        title: 'Entre para abrir a Tela da mesa.',
        detail: 'Sua entrada expirou. Entre de novo para ver a tela.',
      };
    }
    if (this.refused()) {
      return {
        kind: 'no-access',
        title: 'Só o mestre abre a Tela da mesa.',
        detail: 'Ela é a tela da sala; você joga pela sessão no seu aparelho.',
      };
    }
    if (this.ending()) {
      return this.endedPhase(screen);
    }
    const outages = [this.connectionOutage, this.readOutage];
    const lostAt = earliest(outages.filter((o) => o.stale()).map((o) => o.since()));
    const lateAt = earliest(outages.filter((o) => o.late()).map((o) => o.since()));
    if (lostAt) {
      return {
        kind: 'stale',
        title: `Sem conexão desde ${clockText(lostAt)}`,
        detail: 'Esta tela escondeu o que mostrava. Ela volta sozinha quando a conexão voltar.',
        campaignName: screen?.campaignName ?? '',
        sessionLabel: screen?.sessionLabel ?? '',
      };
    }
    if (screen) {
      return {
        kind: 'ready',
        screen,
        late: lateAt
          ? `Esta tela pode estar atrasada desde ${clockText(lateAt)}. Ela continua assim por até 1 minuto.`
          : null,
      };
    }
    if (this.readFailing()) {
      return {
        kind: 'error',
        title: 'Não deu para abrir a Tela da mesa.',
        detail:
          'Tentando de novo em 10 s. Se não voltar, o mestre abre a tela outra vez pela sessão.',
      };
    }
    return { kind: 'loading', text: 'Carregando a mesa…' };
  });

  private endedPhase(screen: TvScreenVm | null): TvPhase {
    const at = this.endedAt();
    return {
      kind: 'ended',
      title: screen
        ? `A sessão ${screen.sessionNumber} terminou${at ? ` às ${clockText(at)}` : ''}.`
        : 'Não há sessão aberta.',
      farewell: screen ? `Até a próxima sessão de ${screen.campaignName}.` : '',
      hint: 'O mestre pode fechar esta aba.',
      campaignName: screen?.campaignName ?? '',
      sessionLabel: screen?.sessionLabel ?? '',
      endedAt: at ? clockText(at) : '',
    };
  }

  /** A good read: the screen to draw. */
  loaded(screen: TvScreenVm): void {
    this.screen.set(screen);
    this.readFailing.set(false);
    this.readOutage.end();
    this.dropRetry();
  }

  /** A failed read. A shown screen stays, marked late (and hidden after a minute); without one, the board's error.
   * Either way a new read comes in `TV_RETRY_MS`. Refusals, a lost sign-in and the end are final. */
  readFailed(err: unknown): void {
    const kind = tableScreenErrorKind(err);
    if (kind === 'no-access') {
      this.refuse();
    } else if (kind === 'signed-out') {
      this.signedOut();
    } else if (kind === 'no-session') {
      this.ended();
    } else {
      if (this.screen()) {
        this.readOutage.begin();
      } else {
        this.readFailing.set(true);
      }
      this.dropRetry();
      this.cancelRetry = this.clock.after(TV_RETRY_MS, () => {
        this.cancelRetry = null;
        this.readAgain();
      });
    }
  }

  /** The stream is connected or not. A minute without it hides the screen. */
  connection(connected: boolean): void {
    if (connected) {
      this.connectionOutage.end();
    } else {
      this.connectionOutage.begin();
    }
  }

  /** The session is over. */
  ended(): void {
    this.endedAt.set(this.clock.now());
    this.ending.set(true);
    this.settle();
  }

  /** The sign-in is gone. */
  signedOut(): void {
    this.signedOutNow.set(true);
    this.settle();
  }

  /** `permission_denied`, `not_found`: this tab may not see the table. */
  refuse(): void {
    this.refused.set(true);
    this.settle();
  }

  /** Cancels the timers (the page is leaving). */
  dispose(): void {
    this.settle();
  }

  private settle(): void {
    this.dropRetry();
    this.connectionOutage.drop();
    this.readOutage.drop();
  }

  private dropRetry(): void {
    this.cancelRetry?.();
    this.cancelRetry = null;
  }
}

function earliest(dates: readonly (Date | null)[]): Date | null {
  return dates.reduce<Date | null>((a, b) => (a && b ? (a < b ? a : b) : (a ?? b)), null);
}
