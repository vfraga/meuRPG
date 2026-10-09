import {
  Component,
  ElementRef,
  Injector,
  OnDestroy,
  OnInit,
  afterNextRender,
  computed,
  inject,
  input,
  effect,
  signal,
  untracked,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { RouterLink } from '@angular/router';
import { Code } from '@connectrpc/connect';

import { lockedSheetCountLabel } from '../../../core/characters/character-labels';
import { describeConnectError } from '../../../core/connect/connect-errors';
import { ActionKey } from '../../../core/connect/idempotency';
import { LivePill } from '../../../shared/live-pill/live-pill';
import { COPIED_FOR_MS, copyText, sessionLink } from '../../../shared/session-link/session-link';
import { formatSessionStart } from '../../../shared/session-time/session-time';
import { OpenSessions } from '../../../shell/live-notice/open-sessions';
import { CampaignSessions } from './campaign-sessions';
import {
  GameSessionSource,
  GameSessionVm,
  OpenChoiceKindVm,
  OpenChoicesVm,
} from './game-session-card.types';

type CardState =
  | { status: 'loading' }
  | { status: 'ready'; session: GameSessionVm | null }
  | { status: 'error'; message: string };

type ActionState = { status: 'idle' } | { status: 'saving' } | { status: 'error'; message: string };

/** `copied`: the button says "Link copiado". `manual`: the Clipboard API
 * failed, so the link shows in a read-only field, selected. */
type CopyState = 'idle' | 'copied' | 'manual';

/** What a sheet lacks, as "faltam 2 perícias". */
const OPEN_CHOICE_WORDS: Record<OpenChoiceKindVm, readonly [string, string]> = {
  skills: ['perícia', 'perícias'],
  cantrips: ['truque', 'truques'],
  spellsKnown: ['magia conhecida', 'magias conhecidas'],
  spellsPrepared: ['magia preparada', 'magias preparadas'],
};

const MASTER_ONLY_MESSAGES = {
  [Code.PermissionDenied]: 'Só o mestre da campanha pode gerenciar sessões.',
  [Code.Unavailable]: 'Não foi possível falar com o servidor agora. Tente de novo em instantes.',
};

/**
 * The "Sessão" panel on `/campaigns/:id` (artboard E5-09).
 *
 * For the master: "Iniciar sessão" (RN-01: starting locks every player's
 * sheet; ending never unlocks them) or, while one is open, "Sessão 4 em
 * andamento, desde qui., 8 de out., 19h05." with "Entrar na sessão" (the page's one
 * filled button), "Copiar link da sessão" (RN-07) and "Encerrar sessão",
 * which confirms in place (docs/design.md: what can't be undone confirms on
 * the screen itself).
 *
 * For a player: only while a session is open, the same sentence with
 * "Entrar na sessão", filled: it's what a player comes to this page for
 * then (the session notice doesn't show on this page; this panel says it
 * instead). Nothing otherwise.
 */
@Component({
  selector: 'app-game-session-card',
  imports: [LivePill, MatButtonModule, MatIconModule, RouterLink],
  templateUrl: './game-session-card.html',
  styleUrl: './game-session-card.scss',
  host: { '[class.is-empty]': 'isEmpty()' },
})
export class GameSessionCard implements OnInit, OnDestroy {
  private readonly source = inject(GameSessionSource);
  private readonly sessions = inject(CampaignSessions);
  private readonly openSessions = inject(OpenSessions);
  private readonly startKey = new ActionKey();
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  readonly campaignId = input.required<string>();
  /** The master manages sessions; a player only sees the open one. */
  readonly isMaster = input(true);

  /** The page's list of sessions (shared with "Sessões anteriores"), as this panel needs it: the open one. */
  protected readonly state = computed<CardState>(() => {
    const s = this.sessions.state();
    switch (s.status) {
      case 'loading':
        return { status: 'loading' };
      case 'error':
        return { status: 'error', message: describeConnectError(s.error, MASTER_ONLY_MESSAGES) };
      case 'ready':
        return { status: 'ready', session: this.sessions.open() };
    }
  });
  protected readonly actionState = signal<ActionState>({ status: 'idle' });
  protected readonly confirmingEnd = signal(false);
  /** The characters with choices open, asked about before the session starts; `null` while nothing is asked. */
  protected readonly confirmingStart = signal<readonly OpenChoicesVm[] | null>(null);
  protected readonly copyState = signal<CopyState>('idle');
  /** The locked-sheet count from the last `StartGameSession` call in this
   * component's lifetime — cleared on reload, shown via
   * `lockedSheetCountLabel` right under the "em andamento" line (integrator
   * amendment, 29/09/2026; pt-BR singular/plural fixed, integrator
   * follow-up: "1 ficha travada." not "1 fichas travadas."). */
  protected readonly lastLockedSheetCount = signal<number | null>(null);
  protected readonly lockedSheetCountLabel = lockedSheetCountLabel;
  protected readonly formatSessionStart = formatSessionStart;
  protected readonly sessionLink = sessionLink;

  private copiedTimer: ReturnType<typeof setTimeout> | null = null;

  /** The open session of this campaign in the app's poll of open sessions, empty when there is none. */
  private readonly openId = computed(
    () =>
      this.openSessions.sessions().find((o) => o.campaignId === this.campaignId())?.sessionId ?? '',
  );

  /** A player's panel with no open session renders nothing at all. */
  protected readonly isEmpty = computed(() => {
    const s = this.state();
    return !this.isMaster() && !(s.status === 'ready' && s.session);
  });

  constructor() {
    // The notice of a session that opens or ends is not shown on this page: this card is the page's announcement, so a
    // player's card reads again when the poll says a session started or ended. (The master's card changes by its own actions.)
    let first = true;
    effect(() => {
      this.openId();
      if (first) {
        first = false;
        return;
      }
      untracked(() => {
        if (!this.isMaster()) {
          void this.sessions.reload(this.campaignId(), true);
        }
      });
    });
  }

  ngOnInit(): void {
    void this.sessions.ensureLoaded(this.campaignId());
  }

  ngOnDestroy(): void {
    if (this.copiedTimer !== null) {
      clearTimeout(this.copiedTimer);
    }
  }

  /** "Ilaria: faltam 1 perícia e 2 truques". */
  protected openChoicesLine(c: OpenChoicesVm): string {
    const parts = c.choices.map((o) => {
      const [one, many] = OPEN_CHOICE_WORDS[o.kind];
      return `${o.missing} ${o.missing === 1 ? one : many}`;
    });
    return `${c.name}: faltam ${new Intl.ListFormat('pt-BR', { type: 'conjunction' }).format(parts)}`;
  }

  /** "Iniciar sessão": starting locks every living player's sheet as it is (RN-01), so the master is told first
   * which sheets still have skills or spells to choose, and decides. */
  protected async requestStart(): Promise<void> {
    this.actionState.set({ status: 'saving' });
    let open: readonly OpenChoicesVm[];
    try {
      open = await this.source.listOpenChoices(this.campaignId());
    } catch (err) {
      this.actionState.set({
        status: 'error',
        message: describeConnectError(err, MASTER_ONLY_MESSAGES),
      });
      return;
    }
    if (open.length === 0) {
      await this.startSession();
      return;
    }
    this.actionState.set({ status: 'idle' });
    this.confirmingStart.set(open);
    this.focusAfterRender('.js-confirm-start');
  }

  protected cancelStart(): void {
    this.confirmingStart.set(null);
    this.focusAfterRender('.js-start');
  }

  protected async startSession(): Promise<void> {
    this.confirmingStart.set(null);
    this.actionState.set({ status: 'saving' });
    try {
      // A retry of the same start (a lost answer, a second tap) sends the same key and starts one session.
      const result = await this.source.startGameSession(
        this.campaignId(),
        this.startKey.keyFor(this.campaignId()),
      );
      this.startKey.renew();
      this.sessions.put(result.session);
      this.lastLockedSheetCount.set(result.lockedSheetCount);
      this.actionState.set({ status: 'idle' });
      // The app bar's "Ao vivo" link appears now, not at the next poll.
      void this.openSessions.refresh();
    } catch (err) {
      this.actionState.set({
        status: 'error',
        message: describeConnectError(err, {
          ...MASTER_ONLY_MESSAGES,
          [Code.FailedPrecondition]: 'Já existe uma sessão em andamento nesta campanha.',
        }),
      });
    }
  }

  protected askToEnd(): void {
    this.confirmingEnd.set(true);
    this.focusAfterRender('.js-confirm-end');
  }

  protected cancelEnd(): void {
    this.confirmingEnd.set(false);
    this.focusAfterRender('.js-end');
  }

  protected async endSession(gameSessionId: string): Promise<void> {
    this.actionState.set({ status: 'saving' });
    try {
      // The ended session leaves this card and goes to the top of "Sessões anteriores".
      this.sessions.put(await this.source.endGameSession(this.campaignId(), gameSessionId));
      this.confirmingEnd.set(false);
      this.copyState.set('idle');
      this.lastLockedSheetCount.set(null);
      this.actionState.set({ status: 'idle' });
      void this.openSessions.refresh();
    } catch (err) {
      this.actionState.set({
        status: 'error',
        message: describeConnectError(err, {
          ...MASTER_ONLY_MESSAGES,
          [Code.FailedPrecondition]: 'Não há nenhuma sessão em andamento nesta campanha.',
        }),
      });
    }
  }

  /** "Copiar link da sessão": the Clipboard API, or else the link in a
   * read-only field, selected, for the person to copy by hand. */
  protected async copyLink(): Promise<void> {
    if (await copyText(sessionLink(this.campaignId()))) {
      this.copyState.set('copied');
      if (this.copiedTimer !== null) {
        clearTimeout(this.copiedTimer);
      }
      this.copiedTimer = setTimeout(() => {
        this.copiedTimer = null;
        if (this.copyState() === 'copied') {
          this.copyState.set('idle');
        }
      }, COPIED_FOR_MS);
      return;
    }
    this.copyState.set('manual');
    afterNextRender(
      () => {
        const field = this.host.nativeElement.querySelector<HTMLInputElement>('.js-link-field');
        field?.focus();
        field?.select();
      },
      { injector: this.injector },
    );
  }

  /** A confirmation replaces the button that asked for it, so the focus
   * moves to its replacement instead of falling back to the page. */
  private focusAfterRender(selector: string): void {
    afterNextRender(() => this.host.nativeElement.querySelector<HTMLElement>(selector)?.focus(), {
      injector: this.injector,
    });
  }
}
