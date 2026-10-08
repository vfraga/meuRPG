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
import { RouterLink } from '@angular/router';

import { LivePill } from '../../../shared/live-pill/live-pill';
import { COPIED_FOR_MS, copyText, sessionLink } from '../../../shared/session-link/session-link';
import { formatClock, sessionSince } from '../../../shared/session-time/session-time';
import { LiveSessionSource, LiveSessionVm } from '../live-session.types';
import { StreamStatus } from '../live-stream';

type EndState = 'idle' | 'confirming' | 'saving' | 'error';

/**
 * The session page's title block (README-A): "← Voltar para a campanha",
 * "Sessão 4", the campaign's name, and the status line: the "Ao vivo" pill
 * with "Em andamento desde 20:05" (the day too, "30/09 às 20:05", when it
 * started on another day), for the master and the players alike. While the stream is down it turns into "Reconectando… Última
 * atualização às 21:14." (E5-08), with no spinner, so nothing moves.
 *
 * The master also gets the session link in a read-only field, "Copiar link
 * da sessão" (RN-07) and "Encerrar sessão", which confirms in place.
 */
@Component({
  selector: 'app-session-header',
  imports: [LivePill, MatButtonModule, MatIconModule, RouterLink],
  templateUrl: './session-header.html',
  styleUrl: './session-header.scss',
})
export class SessionHeader implements OnDestroy {
  private readonly source = inject(LiveSessionSource);
  private readonly injector = inject(Injector);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);

  readonly campaignId = input.required<string>();
  readonly campaignName = input.required<string>();
  readonly session = input.required<LiveSessionVm>();
  readonly isMaster = input(false);
  readonly connection = input<StreamStatus>('live');
  readonly lastUpdate = input<Date | null>(null);

  /** The master ended the session from here. */
  readonly ended = output<void>();

  protected readonly since = computed(() => sessionSince(this.session().startedAt));
  protected readonly link = computed(() => sessionLink(this.campaignId()));
  protected readonly reconnecting = computed(() => {
    const status = this.connection();
    return status === 'reconnecting' || status === 'paused';
  });
  protected readonly lastUpdateLabel = computed(() => {
    const at = this.lastUpdate();
    return at ? `Última atualização às ${formatClock(at)}.` : '';
  });

  protected readonly copied = signal(false);
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
    const field = this.field()?.nativeElement;
    field?.focus();
    field?.select();
  }

  protected askToEnd(): void {
    this.endState.set('confirming');
    this.focusAfterRender('.js-confirm-end');
  }

  protected cancelEnd(): void {
    // The request is already on its way: it cannot be called back, so the confirmation stays until it answers.
    if (this.endState() === 'saving') {
      return;
    }
    this.endState.set('idle');
    this.focusAfterRender('.js-end');
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
