import { InjectionToken, Injector, inject } from '@angular/core';
import { Code, ConnectError } from '@connectrpc/connect';
import type { Interceptor, Transport } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';

import { AuthService } from '../auth/auth.service';
import { isRateLimited, rateLimitedMessage } from './connect-errors';

/**
 * A call the server turned away for asking too often (`resource_exhausted`
 * with a `Retry-After` header) becomes `unavailable`, with the wait in its
 * message. Every screen already treats `unavailable` as "try again in a
 * moment" and never as a final refusal, so none of them shows the copy of a
 * full gallery or of a campaign at its limit (also `resource_exhausted`) for
 * what is only "slow down". Nothing retries on its own: a person asks again,
 * and the live stream, which does retry, backs off (live-stream.ts).
 * `isRateLimited` still recognises the converted error, by the same header.
 */
export const rateLimitInterceptor: Interceptor = (next) => async (req) => {
  try {
    return await next(req);
  } catch (err) {
    const e = ConnectError.from(err);
    if (e.code === Code.ResourceExhausted && isRateLimited(e)) {
      throw new ConnectError(rateLimitedMessage(e), Code.Unavailable, e.metadata, undefined, e);
    }
    throw err;
  }
};

/**
 * A unary call answered `unauthenticated`: the login session ended (signed
 * out in another tab, "Sair dos outros dispositivos", expiry). `AuthService`
 * reads `GetMe` again so the app bar, the guards and the live-session poll
 * see `signed-out`. `GetMe` and `SignOut` are left out (`GetMe` is the
 * reader itself: refreshing on its answer would loop). `AuthService` comes
 * from the injector lazily because it injects this transport.
 */
export function sessionEndedInterceptor(injector: Injector): Interceptor {
  return (next) => async (req) => {
    try {
      return await next(req);
    } catch (err) {
      const e = ConnectError.from(err);
      const isReader = req.method.name === 'GetMe' || req.method.name === 'SignOut';
      if (e.code === Code.Unauthenticated && !isReader) {
        void injector.get(AuthService).refresh();
      }
      throw err;
    }
  };
}

/**
 * The Connect transport every generated client uses to reach the backend.
 *
 * `baseUrl: '/'` relies on the app being served from the same origin as the
 * API (see docs/architecture.md#web-app-web): in production the Go server
 * serves both, and in dev `proxy.conf.json` forwards RPC paths to it. There
 * is deliberately no separate "API URL" to configure per environment.
 *
 * `fetch` is overridden only to force `credentials: 'same-origin'`: the
 * session lives in the `__Host-meurpg_session` cookie (see identity.proto),
 * so every RPC needs it sent. Same-origin `fetch` already defaults to that
 * per the Fetch spec, but the session cookie is exactly the kind of thing
 * that must not depend on a runtime default — this makes it explicit and
 * pins it against ever becoming cross-origin by accident.
 *
 * Every unary call also carries `Connect-Protocol-Version: 1` already,
 * unconditionally, from `@connectrpc/connect`'s own request-header code —
 * nothing to add here for that (see docs/architecture.md#csrf).
 *
 * Unary calls also get a deadline (`UNARY_DEADLINE_MS`), so a request that
 * hangs ends in a `deadline_exceeded` error the screens already handle
 * instead of a spinner that never stops. Streams (the live session) are
 * left alone: they are meant to stay open.
 */
/**
 * The deadline of every unary call that does not set its own: 60 s. The longest
 * legitimate call is the image generation's long poll (25 s on the server,
 * `maxLongPoll`), so this stays above it with room for a slow network.
 */
export const UNARY_DEADLINE_MS = 60_000;

/**
 * Wraps a transport so every unary call carries a deadline. The deadline goes
 * out as the `Connect-Timeout-Ms` header (the server cancels its work too) and
 * aborts the `fetch` here. A call that passes its own `timeoutMs` keeps it.
 * `stream` is passed through untouched.
 */
export function withUnaryDeadline(inner: Transport, deadlineMs: number): Transport {
  return {
    unary: (method, signal, timeoutMs, header, input, contextValues) =>
      inner.unary(method, signal, timeoutMs ?? deadlineMs, header, input, contextValues),
    stream: (method, signal, timeoutMs, header, input, contextValues) =>
      inner.stream(method, signal, timeoutMs, header, input, contextValues),
  };
}

export const CONNECT_TRANSPORT = new InjectionToken<Transport>('CONNECT_TRANSPORT', {
  providedIn: 'root',
  factory: () => {
    const injector = inject(Injector);
    return withUnaryDeadline(
      createConnectTransport({
        baseUrl: '/',
        interceptors: [rateLimitInterceptor, sessionEndedInterceptor(injector)],
        fetch: (input, init) => fetch(input, { ...init, credentials: 'same-origin' }),
      }),
      UNARY_DEADLINE_MS,
    );
  },
});
