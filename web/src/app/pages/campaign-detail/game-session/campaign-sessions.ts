import { Injectable, computed, inject, signal } from '@angular/core';

import { GameSessionSource, GameSessionVm } from './game-session-card.types';

export type CampaignSessionsState =
  | { readonly status: 'loading' }
  | { readonly status: 'ready'; readonly sessions: readonly GameSessionVm[] }
  | { readonly status: 'error'; readonly error: unknown };

/**
 * The campaign's game sessions, read once per page (`ListGameSessions`) and
 * shared by the panels that draw them: "Sessão" takes the open one, "Sessões
 * anteriores" the ended ones. Provided by `CampaignDetail`, so a second panel
 * never means a second call.
 *
 * The list is newest first, as the server sends it. Starting and ending a
 * session update it in place (`put`), so the ended one moves to the top of
 * "Sessões anteriores" without a reload.
 */
@Injectable()
export class CampaignSessions {
  private readonly source = inject(GameSessionSource);

  readonly state = signal<CampaignSessionsState>({ status: 'loading' });

  /** The open session, or `null` while there is none (or the list is not here yet). */
  readonly open = computed(() => {
    const s = this.state();
    return s.status === 'ready' ? (s.sessions.find((x) => !x.endedAt) ?? null) : null;
  });

  /** The ended sessions, newest first. */
  readonly ended = computed(() => {
    const s = this.state();
    return s.status === 'ready' ? s.sessions.filter((x) => x.endedAt) : [];
  });

  private campaignId = '';
  private seq = 0;
  private inFlight: Promise<void> | null = null;

  /** Reads the list unless this campaign's is already here or on its way. */
  ensureLoaded(campaignId: string): Promise<void> {
    if (campaignId === this.campaignId && (this.inFlight || this.state().status === 'ready')) {
      return this.inFlight ?? Promise.resolve();
    }
    return this.reload(campaignId);
  }

  /**
   * Reads the list again; an answer of an older read (or of another campaign)
   * is dropped. `quietly` keeps what is on screen while it reads (a refresh the
   * person did not ask for), instead of going back to "loading".
   */
  reload(campaignId: string = this.campaignId, quietly = false): Promise<void> {
    const mine = ++this.seq;
    const keep = quietly && campaignId === this.campaignId && this.state().status === 'ready';
    this.campaignId = campaignId;
    if (!keep) {
      this.state.set({ status: 'loading' });
    }
    const read = this.source.listSessions(campaignId).then(
      (sessions) => {
        if (mine === this.seq) {
          this.state.set({ status: 'ready', sessions });
        }
      },
      (error: unknown) => {
        if (mine === this.seq) {
          this.state.set({ status: 'error', error });
        }
      },
    );
    this.inFlight = read;
    void read.finally(() => {
      if (this.inFlight === read) {
        this.inFlight = null;
      }
    });
    return read;
  }

  /** A session this page just started or ended: it takes its place in the list. */
  put(session: GameSessionVm): void {
    const s = this.state();
    if (s.status !== 'ready') {
      return;
    }
    const sessions = [...s.sessions.filter((x) => x.id !== session.id), session].sort(
      (a, b) => b.sessionNumber - a.sessionNumber,
    );
    this.state.set({ status: 'ready', sessions });
  }
}
