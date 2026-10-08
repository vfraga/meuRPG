import { Component, inject, signal } from '@angular/core';
import { toObservable } from '@angular/core/rxjs-interop';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { Router } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';
import { filter, take } from 'rxjs';

import { AuthService, type AuthState } from '../../core/auth/auth.service';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import { describeAcceptInviteError } from '../../core/campaigns/invite-errors';

type State =
  | { status: 'no-token' }
  | { status: 'waiting-for-session' }
  | { status: 'signed-out' }
  | { status: 'accepting' }
  | { status: 'error'; message: string; retry: 'session' | 'accept' | null };
// There is no "accepted" state to render: on success, accept() navigates
// away to the campaign before there is anything left to show here.

/** A failure the same invite may get past on another try: the server or the network, not the invite. */
function isTransient(err: unknown): boolean {
  switch (ConnectError.from(err, Code.Unavailable).code) {
    case Code.Unavailable:
    case Code.DeadlineExceeded:
    case Code.ResourceExhausted:
    case Code.Aborted:
    case Code.Internal:
    case Code.Unknown:
      return true;
    default:
      return false;
  }
}

const TOKEN_PATTERN = /^#t=(.+)$/;

/**
 * "/invite" (public, MR-003): reads the invite token from the URL
 * fragment, strips the fragment immediately, and either accepts the invite
 * (signed in) or offers the sign-in-through-invite path (signed out).
 *
 * The token never becomes a route param, a query param or anything else
 * that could end up in `history`, a log line or a `Referer` header
 * (ADR-0009, docs/privacy.md) — it lives only in `token`, a private
 * field on this component, for as long as the page is open.
 */
@Component({
  selector: 'app-invite-accept',
  imports: [MatButtonModule, MatIconModule, MatProgressSpinnerModule],
  templateUrl: './invite-accept.html',
  styleUrl: './invite-accept.scss',
})
export class InviteAccept {
  private readonly auth = inject(AuthService);
  private readonly campaigns = inject(CampaignsService);
  private readonly router = inject(Router);

  /** The invite token, read once from `location.hash` in the constructor
   * and kept only in memory — never in Web Storage, a cookie, or the URL
   * (see the no-Web-Storage rule, docs/privacy.md). */
  private readonly token: string | null;

  protected readonly state = signal<State>({ status: 'waiting-for-session' });

  constructor() {
    const match = TOKEN_PATTERN.exec(window.location.hash);
    let token: string | null = null;
    try {
      token = match ? decodeURIComponent(match[1]) : null;
    } catch {
      // A malformed percent-escape is no token at all: the link is invalid.
    }
    this.token = token;

    // Strip the fragment right away: the token must not linger in the
    // visible URL, in `history`, or in anything (a screenshot, a shared
    // link, another script reading `location` later) that could leak it.
    // `replaceState` — not `pushState` — so this never becomes its own
    // history entry.
    history.replaceState(null, '', window.location.pathname + window.location.search);

    if (!this.token) {
      this.state.set({ status: 'no-token' });
      return;
    }

    toObservable(this.auth.state)
      .pipe(
        filter((s) => s.status !== 'unknown'),
        take(1),
      )
      .subscribe((s) => this.onSession(s));
  }

  private onSession(s: AuthState): void {
    if (s.status === 'signed-in') {
      void this.accept();
    } else if (s.status === 'signed-out') {
      this.state.set({ status: 'signed-out' });
    } else {
      this.state.set({
        status: 'error',
        message: 'Não foi possível confirmar sua sessão agora. Tente de novo em instantes.',
        retry: 'session',
      });
    }
  }

  /** The page's "Tentar de novo": the fragment is gone from the URL, so a reload could not do it; the token in memory can. */
  protected async retry(): Promise<void> {
    const s = this.state();
    if (s.status !== 'error' || s.retry === null) {
      return;
    }
    if (s.retry === 'accept') {
      await this.accept();
      return;
    }
    this.state.set({ status: 'waiting-for-session' });
    await this.auth.refresh();
    this.onSession(this.auth.state());
  }

  private async accept(): Promise<void> {
    this.state.set({ status: 'accepting' });
    try {
      const res = await this.campaigns.acceptInvite(this.token!);
      const id = res.campaign?.id;
      if (id && res.campaign?.awaitingApproval && !res.alreadyMember) {
        // An invite with approval (RN-15, MR-024): the new pending member
        // goes straight to creating the character the master will approve.
        await this.router.navigate(['/campaigns', id, 'characters', 'new']);
      } else if (id) {
        // A fresh join or `already_member` (pending or not): the campaign
        // page is where they go. For someone still pending, it shows
        // "esperando a aprovação do mestre" and their character.
        await this.router.navigate(['/campaigns', id]);
      }
    } catch (err) {
      this.state.set({
        status: 'error',
        message: describeAcceptInviteError(err),
        retry: isTransient(err) ? 'accept' : null,
      });
    }
  }

  /**
   * Signs in through the server, carrying the invite along so it gets
   * accepted right after (the backend contract: a same-origin POST to
   * `/auth/login` with `intent=campaign_invite` and `intent_payload=<token>`
   * — see docs/architecture.md#web-app-web). A hidden form, not `fetch`:
   * signing in is a full-page OIDC redirect dance, exactly like
   * `AuthService.signIn`, and the CSP's `form-action 'self'` allows a
   * same-origin POST like this one.
   */
  protected signInToAccept(): void {
    const form = document.createElement('form');
    form.method = 'post';
    form.action = '/auth/login';
    form.hidden = true;

    const fields: Record<string, string> = {
      return_to: '/campaigns',
      intent: 'campaign_invite',
      intent_payload: this.token ?? '',
    };
    for (const [name, value] of Object.entries(fields)) {
      const field = document.createElement('input');
      field.type = 'hidden';
      field.name = name;
      field.value = value;
      form.appendChild(field);
    }

    document.body.appendChild(form);
    form.submit();
  }
}
