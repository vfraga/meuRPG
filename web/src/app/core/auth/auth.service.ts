import { Injectable, computed, inject, signal } from '@angular/core';
import { Code, ConnectError, createClient } from '@connectrpc/connect';
import { timestampDate } from '@bufbuild/protobuf/wkt';

import { IdentityService } from '../../../gen/meurpg/identity/v1/identity_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';

/**
 * The signed-in user, as far as the UI needs to know.
 *
 * `displayName` is the name the user typed in the app (`UpdateProfile`,
 * "Meu perfil"), never anything from the sign-in provider (identity.proto,
 * docs/privacy.md — no e-mail, name or photo from Google). It is `null`
 * until the user sets one, which every screen that reads it (the user menu,
 * in particular) treats as "no name chosen yet", not as an empty string.
 */
export interface AuthUser {
  readonly id: string;
  readonly displayName: string | null;
}

/** `User.display_name` is `""` when unset (protobuf's scalar default), never
 * absent; this turns that into `null` everywhere `AuthService` reads one. */
function displayNameOrNull(displayName: string | undefined): string | null {
  return displayName ? displayName : null;
}

/**
 * Where the app stands on "who is signed in", as a single value so a
 * template only ever renders one of these four cases — no separate
 * loading/error booleans that could disagree with each other.
 *
 * `unavailable` is deliberately distinct from `signed-out`: it means the
 * server (or its database) did not answer, not that there is no session.
 * Showing it as signed-out would be actively misleading for someone who is
 * in fact still signed in.
 */
export type AuthState =
  | { readonly status: 'unknown' }
  | { readonly status: 'signed-out' }
  | {
      readonly status: 'signed-in';
      readonly user: AuthUser;
      readonly sessionExpiresAt: Date | null;
    }
  | { readonly status: 'unavailable' };

/**
 * Session state for the whole app, backed by `IdentityService`.
 *
 * There is no separate "login" RPC to call: signing in is a browser
 * redirect to the server (see `signIn`), started fresh every time, and this
 * service only ever *reads* the result through `GetMe`. `refresh()` runs
 * once eagerly (see the constructor) — in practice as soon as anything in
 * the app shell injects this service, which happens at startup because the
 * user menu is always in the toolbar.
 */
@Injectable({ providedIn: 'root' })
export class AuthService {
  private readonly client = createClient(IdentityService, inject(CONNECT_TRANSPORT));

  private readonly stateSignal = signal<AuthState>({ status: 'unknown' });

  /** The current session state. Read this, never call GetMe directly. */
  readonly state = this.stateSignal.asReadonly();

  readonly isSignedIn = computed(() => this.stateSignal().status === 'signed-in');

  private refreshing: Promise<void> | null = null;

  constructor() {
    void this.refresh();
  }

  /**
   * Calls `GetMe` and updates `state` from the result. Safe to call again
   * later (e.g. to retry after `unavailable`, once a session's
   * `sessionExpiresAt` has passed, or when any call answered `unauthenticated`) — it always settles to one of the three
   * resolved states, never throws.
   */
  refresh(): Promise<void> {
    // Calls that arrive while one is in flight share it, so a burst of
    // `unauthenticated` answers asks `GetMe` once.
    this.refreshing ??= this.readMe().finally(() => {
      this.refreshing = null;
    });
    return this.refreshing;
  }

  private async readMe(): Promise<void> {
    try {
      const res = await this.client.getMe({});
      this.stateSignal.set({
        status: 'signed-in',
        user: { id: res.user?.id ?? '', displayName: displayNameOrNull(res.user?.displayName) },
        sessionExpiresAt: res.sessionExpiresAt ? timestampDate(res.sessionExpiresAt) : null,
      });
    } catch (err) {
      // Code.Unauthenticated is the one case GetMe documents as "there is
      // no session" (see identity.proto). Everything else — a network
      // failure, the database being down, a future error we didn't
      // anticipate — must not read as signed-out, so it falls back to
      // `unavailable`. ConnectError.from's own default (Code.Unknown) is
      // overridden here for exactly that reason: a plain thrown error
      // (e.g. fetch rejecting before it reaches the server) should land on
      // `unavailable` too, not on some third, unhandled bucket.
      const connectErr = ConnectError.from(err, Code.Unavailable);
      this.stateSignal.set(
        connectErr.code === Code.Unauthenticated
          ? { status: 'signed-out' }
          : { status: 'unavailable' },
      );
    }
  }

  /**
   * Sets the signed-in user's display name (`IdentityService.UpdateProfile`,
   * "Meu perfil"): 1 to 40 characters after trimming; an empty value clears
   * it. Throws the raw error on failure (e.g. `invalid_argument`), for the
   * caller to map with `describeConnectError` — this service only updates
   * `state` on success, so a failed save never shows a name that was not
   * actually saved.
   */
  async updateProfile(displayName: string): Promise<void> {
    const res = await this.client.updateProfile({ displayName });
    const current = this.stateSignal();
    if (current.status === 'signed-in') {
      this.stateSignal.set({
        ...current,
        user: { ...current.user, displayName: displayNameOrNull(res.user?.displayName) },
      });
    }
  }

  /**
   * How many other sessions of this user still work
   * (`IdentityService.CountOtherSessions`), for the "Sessões" section of
   * "Meu perfil". Throws the raw error on failure, for the caller to show.
   */
  async countOtherSessions(): Promise<number> {
    const res = await this.client.countOtherSessions({});
    return res.otherSessions;
  }

  /**
   * Ends every other session of this user (`SignOutOtherSessions`) and
   * returns how many ended. This session stays signed in, so `state` does
   * not change. Throws the raw error on failure.
   */
  async signOutOtherSessions(): Promise<number> {
    const res = await this.client.signOutOtherSessions({});
    return res.endedCount;
  }

  /**
   * Sends the browser to the server's sign-in flow with a full-page
   * navigation (this is a redirect dance with the OIDC provider, not
   * something the Angular router can do). `returnTo` must be a path on
   * this site: the server's `safeReturnTo` rejects anything else — a
   * malformed value here would only turn into the server's plain-text 400.
   *
   * The fragment is always dropped: it can hold a secret (the invite token in
   * `/invite#t=...`, which Angular's `router.url` still carries), and this
   * URL is a GET that ends up in the platform's request logs.
   */
  signIn(returnTo: string): void {
    const withoutFragment = returnTo.split('#', 1)[0];
    const path =
      withoutFragment.startsWith('/') && !withoutFragment.startsWith('//') ? withoutFragment : '/';
    window.location.assign(`/auth/login?return_to=${encodeURIComponent(path)}`);
  }

  /**
   * Ends the session on the server, then always returns to the home page
   * with a full navigation, so the next `AuthService` starts clean and
   * `GetMe` reports the real state. The navigation still happens even if
   * `SignOut` itself fails (e.g. the server is `unavailable`): the user
   * asked to leave, and a failed revoke on the server is not something a
   * client-side error message would let them fix anyway.
   */
  async signOut(): Promise<void> {
    try {
      await this.client.signOut({});
    } catch {
      // Best-effort — see the doc comment above.
    }
    window.location.assign('/');
  }
}
