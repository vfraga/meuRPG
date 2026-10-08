import {
  DOCUMENT,
  DestroyRef,
  Injectable,
  InjectionToken,
  computed,
  effect,
  inject,
  signal,
} from '@angular/core';

import { Code, ConnectError } from '@connectrpc/connect';

import { AuthService } from '../../core/auth/auth.service';
import { CONNECT_TRANSPORT } from '../../core/connect/transport';

/** An open game session of one of the signed-in person's campaigns
 * (`PlayService.ListOpenGameSessions`), as the notice and the "Ao vivo"
 * pill need it. */
export interface OpenSessionVm {
  readonly sessionId: string;
  readonly campaignId: string;
  readonly campaignName: string;
  readonly sessionNumber: number;
  readonly startedAt: Date;
  /** The person is the campaign's master (they started it themself). */
  readonly isMaster: boolean;
}

/** One `ListOpenGameSessions` call, newest session first. */
export type OpenSessionsFetcher = () => Promise<OpenSessionVm[]>;

/**
 * Loads the fetcher. The default imports `open-sessions-client.ts` only when
 * the first poll runs: the generated `PlayService` client (and the
 * campaigns and rules messages it imports) stays out of the initial bundle,
 * which this service is part of. Tests provide a fake.
 */
export const OPEN_SESSIONS_FETCHER = new InjectionToken<() => Promise<OpenSessionsFetcher>>(
  'OPEN_SESSIONS_FETCHER',
  {
    providedIn: 'root',
    factory: () => {
      const transport = inject(CONNECT_TRANSPORT);
      return () =>
        import('./open-sessions-client').then((m) => m.createOpenSessionsFetcher(transport));
    },
  },
);

/** How often the app asks, while the tab is visible (RN-06, ADR-0005). */
export const POLL_INTERVAL_MS = 30_000;

/**
 * The open sessions of the signed-in person's campaigns, kept fresh by a
 * light poll (RN-06; docs/architecture.md#live-session): every 30 seconds
 * while the person is signed in and the tab is visible, once as soon as the
 * tab becomes visible again, and nothing while it is hidden. There is no
 * open connection: the live stream exists only on the session page.
 *
 * Which notices the person closed lives here too, in memory only: no Web
 * Storage (docs/privacy.md), so a reload may show a notice again.
 */
@Injectable({ providedIn: 'root' })
export class OpenSessions {
  private readonly auth = inject(AuthService);
  private readonly document = inject(DOCUMENT);
  private readonly loadFetcher = inject(OPEN_SESSIONS_FETCHER);

  private readonly sessionsSignal = signal<readonly OpenSessionVm[]>([]);
  private readonly dismissedSignal = signal<ReadonlySet<string>>(new Set());

  /** Newest first. Empty while signed out. */
  readonly sessions = this.sessionsSignal.asReadonly();
  /** Session IDs whose notice the person closed (or followed) in this tab. */
  readonly dismissed = this.dismissedSignal.asReadonly();

  /** Campaign IDs with an open session, for the "Sessão ao vivo" tag. */
  readonly liveCampaignIds = computed(
    () => new Set(this.sessionsSignal().map((s) => s.campaignId)),
  );

  private fetcher: Promise<OpenSessionsFetcher> | null = null;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private polling = false;
  private inFlight = false;

  constructor() {
    // Poll only while signed in.
    effect(() => {
      if (this.auth.state().status === 'signed-in') {
        this.start();
      } else {
        this.stop();
      }
    });

    const onVisibility = () => this.onVisibilityChange();
    this.document.addEventListener('visibilitychange', onVisibility);
    inject(DestroyRef).onDestroy(() => {
      this.document.removeEventListener('visibilitychange', onVisibility);
      this.stop();
    });
  }

  /** Asks now (and restarts the 30-second wait), e.g. right after the
   * master starts or ends a session, so the pill doesn't lag behind. */
  refresh(): Promise<void> {
    return this.polling ? this.poll() : Promise.resolve();
  }

  /** Hides the notice of this session for the rest of this tab's life. */
  dismiss(sessionId: string): void {
    this.dismissedSignal.update((ids) => new Set(ids).add(sessionId));
  }

  private start(): void {
    if (this.polling) {
      return;
    }
    this.polling = true;
    if (this.visible()) {
      void this.poll();
    }
  }

  private stop(): void {
    this.polling = false;
    this.clearTimer();
    this.sessionsSignal.set([]);
  }

  private onVisibilityChange(): void {
    if (!this.polling) {
      return;
    }
    if (this.visible()) {
      void this.poll();
    } else {
      this.clearTimer();
    }
  }

  private async poll(): Promise<void> {
    this.clearTimer();
    if (this.inFlight) {
      return;
    }
    this.inFlight = true;
    try {
      // A chunk that failed to load is loaded again on the next poll.
      this.fetcher ??= this.loadFetcher().catch((err: unknown) => {
        this.fetcher = null;
        throw err;
      });
      const fetch = await this.fetcher;
      const sessions = await fetch();
      if (this.polling) {
        this.sessionsSignal.set(sessions);
      }
    } catch (err) {
      // Keep what we had: a network hiccup shouldn't hide a live session.
      // A lost sign-in is the exception: AuthService reads it again, settles
      // to signed-out and the effect above stops the poll.
      if (ConnectError.from(err).code === Code.Unauthenticated) {
        void this.auth.refresh();
      }
    } finally {
      this.inFlight = false;
      if (this.polling && this.visible()) {
        this.timer = setTimeout(() => void this.poll(), POLL_INTERVAL_MS);
      }
    }
  }

  private visible(): boolean {
    return this.document.visibilityState !== 'hidden';
  }

  private clearTimer(): void {
    if (this.timer !== null) {
      clearTimeout(this.timer);
      this.timer = null;
    }
  }
}

/** `/campaigns/<id>` or `/campaigns/<id>/session`, from a router URL. */
function campaignPage(url: string): { campaignId: string; session: boolean } | null {
  const path = url.split(/[?#]/, 1)[0];
  const match = /^\/campaigns\/([^/]+)(\/session)?\/?$/.exec(path);
  return match ? { campaignId: match[1], session: match[2] !== undefined } : null;
}

/**
 * The sessions the notice announces on `url`, oldest first: every open
 * session the person plays in (a master started theirs, so it isn't news)
 * whose notice they haven't closed, except the one of the campaign whose
 * page they are on (its "Sessão" panel says it). None on a session page:
 * the person is at a table already, and the page is theirs (the app bar's
 * "Ao vivo" link still leads to any other open session).
 *
 * Oldest first, one notice per session: a session that starts later adds
 * its notice below the others, so a notice never changes or moves under
 * the person's finger while they reach for "Entrar na sessão".
 */
export function sessionsToAnnounce(
  sessions: readonly OpenSessionVm[],
  dismissed: ReadonlySet<string>,
  url: string,
): OpenSessionVm[] {
  const here = campaignPage(url);
  if (here?.session) {
    return [];
  }
  return sessions
    .filter((s) => !s.isMaster && !dismissed.has(s.sessionId) && here?.campaignId !== s.campaignId)
    .reverse();
}

/**
 * The session the app bar's "Ao vivo" link opens on `url`: the newest open
 * session of any of the person's campaigns, as master or player. None on a
 * session page: its own status line already carries the "Ao vivo" pill.
 */
export function sessionForLiveLink(
  sessions: readonly OpenSessionVm[],
  url: string,
): OpenSessionVm | null {
  return campaignPage(url)?.session ? null : (sessions[0] ?? null);
}
