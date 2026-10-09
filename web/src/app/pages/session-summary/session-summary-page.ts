import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  effect,
  inject,
  signal,
  untracked,
} from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';
import { map } from 'rxjs';

import { timestampDate } from '@bufbuild/protobuf/wkt';

import { Role } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type { SessionSummary } from '../../../gen/meurpg/play/v1/summary_pb';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import { describeConnectError } from '../../core/connect/connect-errors';
import {
  SessionSummaryClient,
  checksRatio,
  durationSeconds,
  formatDuration,
} from '../../core/play/session-summary';
import { setPageSubject } from '../../core/title/page-title';
import { formatSessionSpan } from '../../shared/session-time/session-time';
import { SessionStats } from '../../shared/session-summary/session-stats';
import { SessionSummaryBody } from '../../shared/session-summary/session-summary-body';
import {
  GameSessionSource,
  GameSessionVm,
} from '../campaign-detail/game-session/game-session-card.types';

const LOAD_ERROR = 'Não foi possível carregar o resumo da sessão. Tente de novo.';

/** A session number in the address: a whole number from 1. Anything else is a page that does not exist. */
const SESSION_NUMBER = /^[1-9]\d{0,8}$/;

interface Neighbour {
  readonly number: number;
  /** "Sessão 3", or "Sessão 4 em andamento" when that one is still open. */
  readonly label: string;
  readonly open: boolean;
}

type PageState =
  | { readonly status: 'loading' }
  /** Not a member (or no such campaign, or a number that is no number): the server's own `not_found`, said the same for all. */
  | { readonly status: 'denied' }
  | { readonly status: 'error'; readonly message: string }
  /** A member, but the campaign has no such session. */
  | { readonly status: 'missing'; readonly campaignName: string; readonly last: number | null }
  | { readonly status: 'open'; readonly campaignName: string }
  | {
      readonly status: 'ready';
      readonly campaignName: string;
      readonly isMaster: boolean;
      readonly summary: SessionSummary;
      readonly players: ReadonlyMap<string, string>;
      readonly previous: Neighbour | null;
      readonly next: Neighbour | null;
    };

/**
 * "/campaigns/:id/sessions/:number" (PM-01): the summary of an ended session,
 * kept for anyone in the campaign to reopen after the moment it ended.
 *
 * The number is the session's `session_number`, not its id. The page reads
 * `ListGameSessions` to find the id and the neighbouring sessions, then
 * `GetSessionSummary` (which draws what the server sent this reader and nothing
 * more: RN-20), and, for the master, the players' names. The server answers
 * `not_found` both for a number that does not exist and for someone who is not a
 * member; the page tells them apart only by what it knows itself: a member whose
 * list lacks the number reads "Não há Sessão 9", anyone else gets one generic
 * page with no campaign name in it.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-session-summary-page',
  imports: [MatButtonModule, MatIconModule, RouterLink, SessionStats, SessionSummaryBody],
  templateUrl: './session-summary-page.html',
  styleUrl: './session-summary-page.scss',
})
export class SessionSummaryPage {
  private readonly campaigns = inject(CampaignsService);
  private readonly sessions = inject(GameSessionSource);
  private readonly api = inject(SessionSummaryClient);
  private readonly route = inject(ActivatedRoute);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  protected readonly campaignId = toSignal(
    this.route.paramMap.pipe(map((p) => p.get('id') ?? '')),
    { initialValue: '' },
  );
  private readonly rawNumber = toSignal(
    this.route.paramMap.pipe(map((p) => p.get('number') ?? '')),
    { initialValue: '' },
  );
  /** The session's number, or 0 when the address holds no number. */
  protected readonly number = computed(() => {
    const raw = this.rawNumber();
    return SESSION_NUMBER.test(raw) ? Number(raw) : 0;
  });

  protected readonly state = signal<PageState>({ status: 'loading' });

  protected readonly ready = computed(() => {
    const s = this.state();
    return s.status === 'ready' ? s : null;
  });
  /** The campaign's name once known: while the rest still loads, and on the pages that name it. */
  protected readonly campaignName = computed(() => {
    const s = this.state();
    return s.status === 'ready' || s.status === 'missing' || s.status === 'open'
      ? s.campaignName
      : this.knownName();
  });
  /** What `GetCampaign` said, a member's own read: it never names a campaign to someone who is not in it. */
  private readonly knownName = signal('');
  /** "qui., 24 de set., 19h12 às 22h30". */
  protected readonly when = computed(() => {
    const s = this.ready()?.summary;
    return s?.startedAt && s.endedAt
      ? formatSessionSpan(timestampDate(s.startedAt), timestampDate(s.endedAt))
      : '';
  });
  protected readonly duration = computed(() => {
    const s = this.ready()?.summary;
    return s ? formatDuration(durationSeconds(s)) : '';
  });
  protected readonly checks = computed(() => {
    const s = this.ready()?.summary;
    return s ? checksRatio(s.checksPassed, s.checksTried) : '';
  });
  protected readonly ownCharacterId = computed(
    () => this.ready()?.summary.mine?.highlights?.characterId ?? '',
  );
  protected readonly ownCharacterName = computed(
    () => this.ready()?.summary.mine?.highlights?.name ?? '',
  );
  protected readonly loadError = computed(() => {
    const s = this.state();
    return s.status === 'error' ? s.message : '';
  });

  /** The read in flight; the answer of an older one (the address changed meanwhile) is dropped. */
  private generation = 0;

  constructor() {
    // "Sessão 2 · Mirathel": the tab names the session as soon as the address does, the campaign when it is known.
    setPageSubject(() => {
      if (this.state().status === 'denied') {
        return 'Página não encontrada';
      }
      const n = this.number();
      if (n === 0) {
        return null;
      }
      const campaign = this.campaignName();
      return campaign ? `Sessão ${n} · ${campaign}` : `Sessão ${n}`;
    });
    effect(() => {
      const id = this.campaignId();
      const n = this.number();
      untracked(() => void this.read(id, n));
    });
  }

  protected retry(): void {
    void this.read(this.campaignId(), this.number());
  }

  private async read(campaignId: string, number: number): Promise<void> {
    const generation = ++this.generation;
    const current = () => generation === this.generation;
    this.state.set({ status: 'loading' });
    this.knownName.set('');
    if (number === 0) {
      this.state.set({ status: 'denied' });
      return;
    }
    try {
      const campaignRead = this.campaigns.getCampaign(campaignId);
      void campaignRead.then(
        (res) => current() && this.knownName.set(res.campaign?.name ?? ''),
        () => undefined,
      );
      const [campaign, list] = await Promise.all([
        campaignRead,
        this.sessions.listSessions(campaignId),
      ]);
      if (!current()) {
        return;
      }
      const name = campaign.campaign?.name ?? '';
      const session = list.find((s) => s.sessionNumber === number);
      if (!session) {
        const last = list.find((s) => s.endedAt);
        this.state.set({
          status: 'missing',
          campaignName: name,
          last: last?.sessionNumber ?? null,
        });
        return;
      }
      if (!session.endedAt) {
        this.state.set({ status: 'open', campaignName: name });
        return;
      }
      const isMaster = campaign.campaign?.myRole === Role.MASTER;
      const summary = await this.api.get(campaignId, session.id);
      if (!current()) {
        return;
      }
      const players = isMaster
        ? await this.api.playerNames(campaignId).catch(() => new Map<string, string>())
        : new Map<string, string>();
      if (!current()) {
        return;
      }
      this.state.set({
        status: 'ready',
        campaignName: name,
        isMaster,
        summary,
        players,
        ...neighbours(list, number),
      });
      this.focusAfterRender('.js-title');
    } catch (err) {
      if (current()) {
        this.fail(err);
      }
    }
  }

  /** What a refused or failed read says: `not_found` is the generic page, the others have their own. */
  private fail(err: unknown): void {
    const code = ConnectError.from(err, Code.Unavailable).code;
    if (code === Code.NotFound) {
      this.state.set({ status: 'denied' });
    } else if (code === Code.FailedPrecondition) {
      // The session is still open (it was ended on another screen a moment ago, or not yet).
      this.state.set({ status: 'open', campaignName: this.campaignName() });
    } else {
      this.state.set({
        status: 'error',
        message: describeConnectError(err, { [Code.Unavailable]: LOAD_ERROR }),
      });
      this.focusAfterRender('.js-retry');
    }
  }

  /** The page changes under the person's hands (a neighbour, a retry): the focus follows to where the new content starts. */
  private focusAfterRender(selector: string): void {
    afterNextRender(() => this.host.nativeElement.querySelector<HTMLElement>(selector)?.focus(), {
      injector: this.injector,
    });
  }
}

/** The sessions on either side of `number`, by number: "Sessão 1" and "Sessão 3". The next one may still be open. */
function neighbours(
  list: readonly GameSessionVm[],
  number: number,
): { previous: Neighbour | null; next: Neighbour | null } {
  const before = list
    .filter((s) => s.sessionNumber < number)
    .sort((a, b) => b.sessionNumber - a.sessionNumber)[0];
  const after = list
    .filter((s) => s.sessionNumber > number)
    .sort((a, b) => a.sessionNumber - b.sessionNumber)[0];
  const of = (s: GameSessionVm | undefined): Neighbour | null =>
    s
      ? {
          number: s.sessionNumber,
          open: !s.endedAt,
          label: s.endedAt ? `Sessão ${s.sessionNumber}` : `Sessão ${s.sessionNumber} em andamento`,
        }
      : null;
  return { previous: of(before), next: of(after) };
}
