import {
  Component,
  ElementRef,
  Injector,
  OnDestroy,
  afterNextRender,
  computed,
  inject,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatMenuModule } from '@angular/material/menu';
import { RouterLink } from '@angular/router';

import { WIDE_BAR_QUERY } from './wide-bar';
import { PHONE_QUERY, mediaQuery } from '../../../shared/map-view/media-query';
import { LivePill } from '../../../shared/live-pill/live-pill';
import { COPIED_FOR_MS, copyText, sessionLink } from '../../../shared/session-link/session-link';
import { formatClock, sessionSinceShort } from '../../../shared/session-time/session-time';
import { LiveSessionSource, LiveSessionVm } from '../../../core/play/live-session.types';
import { StreamStatus } from '../../../core/play/live-stream';

type EndState = 'idle' | 'confirming' | 'saving' | 'error';

/**
 * The session bar of every live screen, master and player: one row of 56 px with the back link to the campaign (its name
 * on a computer, the arrow alone on a phone), "Sessão 4", the "Ao vivo" pill with "desde 20:05" (the day too, "desde 30/09 às 20:05",
 * when it started on another day) and "Dados" (MR-052: normal, "Rolando…" while a roll is on its way, off while the stream
 * reconnects). While the stream is down the status turns into the notice "Reconectando… Última atualização às 21:14."
 * (E5-08) under the bar, with no spinner, so nothing moves.
 *
 * The master also gets "Iniciar combate" outside a combat (the page opens the dialog; on a phone it is the menu's first item) and
 * the "Sessão" menu with "Copiar link da sessão" (RN-07) and "Encerrar sessão", which asks in place under the bar with
 * "Cancelar" focused.
 */
@Component({
  selector: 'app-session-header',
  imports: [LivePill, MatButtonModule, MatIconModule, MatMenuModule, RouterLink],
  templateUrl: './session-header.html',
  styleUrl: './session-header.scss',
})
export class SessionHeader implements OnDestroy {
  private readonly source = inject(LiveSessionSource);
  private readonly injector = inject(Injector);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  protected readonly phone = mediaQuery(PHONE_QUERY);
  /** From 1024 px the master's "Tela da mesa" is a button in the bar; below it, a line of the menu. */
  protected readonly wide = mediaQuery(WIDE_BAR_QUERY);

  readonly campaignId = input.required<string>();
  readonly campaignName = input.required<string>();
  readonly session = input.required<LiveSessionVm>();
  readonly isMaster = input(false);
  readonly connection = input<StreamStatus>('live');
  readonly lastUpdate = input<Date | null>(null);
  /** A free roll is on its way (MR-052): the "Dados" button waits. */
  readonly rolling = input(false);
  /** A combat is on: the master's "Iniciar combate" is not offered. */
  readonly inCombat = input(false);
  /** The current map is still being read (or could not be): "Iniciar combate" waits for it. */
  readonly startOff = input(false);

  /** The bar button's words and whether the TV is pinned (`barButtonLabel`). */
  readonly tvLabel = input<{ text: string; pinned: boolean }>({
    text: 'Tela da mesa',
    pinned: false,
  });
  /** The second line of the menu's "Tela da mesa…" (`menuLine`). */
  readonly tvMenuLine = input('Nenhuma tela aberta');
  /** A TV tab is connected: the bar button takes the accent look. */
  readonly tvOpen = input(false);
  /** The TV has no connection: the bar button turns to the warning look. */
  readonly tvLost = input(false);

  /** The master ended the session from here. */
  readonly ended = output<void>();
  /** "Dados": the person wants the free dice roller. */
  readonly openDice = output<void>();
  /** "Tela da mesa": the master wants the panel of the TV tab. */
  readonly openTableScreen = output<void>();
  /** "Iniciar combate": the master wants the dialog that starts a combat. */
  readonly startCombat = output<void>();

  protected readonly since = computed(() => sessionSinceShort(this.session().startedAt));
  protected readonly link = computed(() => sessionLink(this.campaignId()));
  protected readonly reconnecting = computed(() => {
    const status = this.connection();
    return status === 'reconnecting' || status === 'paused';
  });
  /** The dice wait for a roll in the air, and for the stream: with no connection there is no table to roll for. */
  protected readonly diceOff = computed(() => this.rolling() || this.reconnecting());
  protected readonly lastUpdateLabel = computed(() => {
    const at = this.lastUpdate();
    return at ? `Última atualização às ${formatClock(at)}.` : '';
  });

  protected readonly copied = signal(false);
  /** The clipboard refused: the link is shown in a field, selected, to copy by hand. */
  protected readonly linkShown = signal(false);
  protected readonly endState = signal<EndState>('idle');

  private readonly field = viewChild<ElementRef<HTMLInputElement>>('linkField');
  private copiedTimer: ReturnType<typeof setTimeout> | null = null;

  ngOnDestroy(): void {
    if (this.copiedTimer !== null) {
      clearTimeout(this.copiedTimer);
    }
  }

  /** The Clipboard API, or else the link selected in its field. */
  protected async copyLink(): Promise<void> {
    if (await copyText(this.link())) {
      this.copied.set(true);
      if (this.copiedTimer !== null) {
        clearTimeout(this.copiedTimer);
      }
      this.copiedTimer = setTimeout(() => {
        this.copiedTimer = null;
        this.copied.set(false);
      }, COPIED_FOR_MS);
      return;
    }
    this.linkShown.set(true);
    afterNextRender(
      () => {
        const field = this.field()?.nativeElement;
        field?.focus();
        field?.select();
      },
      { injector: this.injector },
    );
  }

  protected openDiceNow(): void {
    if (!this.diceOff()) {
      this.openDice.emit();
    }
  }

  protected askToEnd(): void {
    this.endState.set('confirming');
    this.focusAfterRender('.js-cancel-end');
  }

  protected cancelEnd(): void {
    // The request is already on its way: it cannot be called back, so the confirmation stays until it answers.
    if (this.endState() === 'saving') {
      return;
    }
    this.endState.set('idle');
    this.focusAfterRender('.bar__menu');
  }

  protected async endSession(): Promise<void> {
    this.endState.set('saving');
    try {
      await this.source.endSession(this.campaignId(), this.session().sessionId);
      this.endState.set('idle');
      this.ended.emit();
    } catch (err) {
      // Already over (another tab ended it): that's what was asked for.
      if (this.source.classifyError(err) === 'no-session') {
        this.endState.set('idle');
        this.ended.emit();
        return;
      }
      this.endState.set('error');
    }
  }

  private focusAfterRender(selector: string): void {
    afterNextRender(() => this.host.nativeElement.querySelector<HTMLElement>(selector)?.focus(), {
      injector: this.injector,
    });
  }
}
