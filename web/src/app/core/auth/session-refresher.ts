import { InjectionToken } from '@angular/core';

/**
 * What the transport needs from the auth state when a call learns the login session ended. A token of its own,
 * provided by the app (`appConfig`: `useExisting: AuthService`), because `AuthService` injects the transport and
 * the transport importing `AuthService` back was an import cycle. The provider is `useExisting`, so the service
 * is still created lazily, on the first `unauthenticated` answer, as before.
 */
export interface SessionRefresher {
  refresh(): Promise<void>;
}

export const SESSION_REFRESHER = new InjectionToken<SessionRefresher>('SESSION_REFRESHER');
