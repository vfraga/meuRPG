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
import { formatDayAt } from '../../../shared/session-time/session-time';
import { OpenSessions } from '../../../shell/live-notice/open-sessions';
import { GameSessionSource, GameSessionVm } from './game-session-card.types';

type CardState =
  | { status: 'loading' }
  | { status: 'ready'; session: GameSessionVm | null }
  | { status: 'error'; message: string };

type ActionState = { status: 'idle' } | { status: 'saving' } | { status: 'error'; message: string };

/** `copied`: the button says "Link copiado". `manual`: the Clipboard API
 * failed, so the link shows in a read-only field, selected. */
type CopyState = 'idle' | 'copied' | 'manual';

const MASTER_ONLY_MESSAGES = {
  [Code.PermissionDenied]: 'Só o mestre da campanha pode gerenciar sessões.',
  [Code.Unavailable]: 'Não foi possível falar com o servidor agora. Tente de novo em instantes.',
};

/**
 * The "Sessão" panel on `/campaigns/:id` (artboard E5-09).
 *
 * For the master: "Iniciar sessão" (RN-01: starting locks every player's
 * sheet; ending never unlocks them) or, while one is open, "Sessão 4 em
 * andamento, desde 30/09 às 20:05." with "Entrar na sessão" (the page's one
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
  private readonly openSessions = inject(OpenSessions);
  private readonly startKey = new ActionKey();
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  readonly campaignId = input.required<string>();
  /** The master manages sessions; a player only sees the open one. */
  readonly isMaster = input(true);

  protected readonly state = signal<CardState>({ status: 'loading' });
  protected readonly actionState = signal<ActionState>({ status: 'idle' });
  protected readonly confirmingEnd = signal(false);
  protected readonly copyState = signal<CopyState>('idle');
  /** The locked-sheet count from the last `StartGameSession` call in this
   * component's lifetime — cleared on reload, shown via
   * `lockedSheetCountLabel` right under the "em andamento" line (integrator
   * amendment, 29/09/2026; pt-BR singular/plural fixed, integrator
   * follow-up: "1 ficha travada." not "1 fichas travadas."). */
  protected readonly lastLockedSheetCount = signal<number | null>(null);
  protected readonly lockedSheetCountLabel = lockedSheetCountLabel;
  protected readonly formatDayAt = formatDayAt;
  protected readonly sessionLink = sessionLink;

  private copiedTimer: ReturnType<typeof setTimeout> | null = null;
  private loadSeq = 0;

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
          this.load();
        }
      });
    });
  }

  ngOnInit(): void {
    this.load();
  }

  ngOnDestroy(): void {
    if (this.copiedTimer !== null) {
      clearTimeout(this.copiedTimer);
    }
  }

  private load(): void {
    const mine = ++this.loadSeq;
    this.state.set({ status: 'loading' });
    this.lastLockedSheetCount.set(null);
    this.source.getCurrentSession(this.campaignId()).then(
      (session) => {
        if (mine === this.loadSeq) {
          this.state.set({ status: 'ready', session });
        }
      },
      (err: unknown) => {
        if (mine !== this.loadSeq) {
          return;
        }
        this.state.set({
          status: 'error',
          message: describeConnectError(err, MASTER_ONLY_MESSAGES),
        });
      },
    );
  }

  protected async startSession(): Promise<void> {
    this.actionState.set({ status: 'saving' });
    try {
      // A retry of the same start (a lost answer, a second tap) sends the same key and starts one session.
      const result = await this.source.startGameSession(
        this.campaignId(),
        this.startKey.keyFor(this.campaignId()),
      );
      this.startKey.renew();
      this.state.set({ status: 'ready', session: result.session });
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
      await this.source.endGameSession(this.campaignId(), gameSessionId);
      // A session that just ended is no longer "current" for this card.
      this.state.set({ status: 'ready', session: null });
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
