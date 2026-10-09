import { Component, computed, effect, inject, input, signal, untracked } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { RouterLink } from '@angular/router';

import type { SessionSummary } from '../../../../gen/meurpg/play/v1/summary_pb';
import { combatErrorMessage } from '../../../core/combat/combat-errors';
import {
  SessionSummaryClient,
  checksRatio,
  durationSeconds,
  formatDuration,
  sessionSpan,
} from '../../../core/play/session-summary';
import { SessionStats } from '../../../shared/session-summary/session-stats';
import { SessionSummaryBody } from '../../../shared/session-summary/session-summary-body';
import { SessionBlocked } from '../session-blocked/session-blocked';

type Load = 'loading' | 'ready' | 'failed';

/**
 * What the session page shows once the session has ended (MR-032, E8-11 states
 * 4 and 5, question 64): it reads `GetSessionSummary` for the session that
 * just ended and draws it with `SessionSummaryBody`, the same body the kept
 * page (`/campaigns/:id/sessions/:n`) uses.
 *
 * - **The master** lands on "Sessão encerrada": the duration, the combats, the
 *   scenes opened and "Testes passados fora do combate N de M"; then "Resumo da
 *   sessão" with the session's highlights (the three of the combats, and "Mais
 *   testes passados fora do combate") and the per-player table of the checks
 *   passed of the ones tried, which only the master gets. The caption says only
 *   scenes that showed their DC count (RN-20). "Voltar à campanha" is the only
 *   button.
 * - **A player** gets the card "A sessão acabou" (the shared card of the combat's
 *   ending): the winners with their numbers, no "de N", and "Seu resultado,
 *   Pensantus". It replaces a combat card still open, because the page is no
 *   longer the live one. "Fechar" or the ✕ leaves the plain "A sessão acabou"
 *   notice (and "Voltar para a campanha").
 *
 * "Mais tesouro encontrado" (MR-041, E9-09): the master has a block of its own, one
 * row for everyone who found treasure (or the line that none was), and the player's
 * card has it as one more tile and in "Seu resultado". If the summary cannot be read, the page says so with "Tentar de novo" under the
 * plain notice: the summary is a bonus, never a screen the person needs. The card fades in over
 * 200ms, never under `prefers-reduced-motion`.
 */
@Component({
  selector: 'app-session-ended',
  imports: [
    MatButtonModule,
    MatIconModule,
    RouterLink,
    SessionBlocked,
    SessionStats,
    SessionSummaryBody,
  ],
  templateUrl: './session-ended.html',
  styleUrl: './session-ended.scss',
})
export class SessionEnded {
  private readonly api = inject(SessionSummaryClient);

  readonly campaignId = input.required<string>();
  readonly campaignName = input('');
  readonly sessionId = input.required<string>();
  readonly sessionNumber = input(0);
  readonly isMaster = input(false);
  /** The players' names by character, for "de Caio" on the master's tiles. */
  readonly players = input<ReadonlyMap<string, string>>(new Map());
  /** The reader's own character: marks "Você" and names "Seu resultado". */
  readonly characterId = input('');
  readonly characterName = input('');

  protected readonly load = signal<Load>('loading');
  protected readonly summary = signal<SessionSummary | null>(null);
  /** The player closed the card. */
  protected readonly closed = signal(false);

  protected readonly duration = computed(() => {
    const s = this.summary();
    return s ? formatDuration(durationSeconds(s)) : '';
  });
  protected readonly span = computed(() => {
    const s = this.summary();
    return s ? sessionSpan(s) : '';
  });
  protected readonly checks = computed(() => {
    const s = this.summary();
    return s ? checksRatio(s.checksPassed, s.checksTried) : '';
  });
  protected readonly combats = computed(() => this.summary()?.combats ?? 0);
  protected readonly scenes = computed(() => this.summary()?.scenesOpened ?? 0);
  protected readonly error = signal('');

  constructor() {
    effect(() => {
      const campaignId = this.campaignId();
      const sessionId = this.sessionId();
      untracked(() => {
        // Another session's card starts open.
        this.closed.set(false);
        void this.read(campaignId, sessionId);
      });
    });
  }

  /** The read in flight; a late answer of an older one is dropped. */
  private generation = 0;

  protected retry(): void {
    void this.read(this.campaignId(), this.sessionId());
  }

  private async read(campaignId: string, sessionId: string): Promise<void> {
    const generation = ++this.generation;
    this.load.set('loading');
    this.error.set('');
    try {
      const summary = await this.api.get(campaignId, sessionId);
      if (generation === this.generation) {
        this.summary.set(summary);
        this.load.set('ready');
      }
    } catch (err) {
      if (generation === this.generation) {
        this.error.set(combatErrorMessage(err, 'carregar o resumo'));
        this.load.set('failed');
      }
    }
  }
}
