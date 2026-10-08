import type { Injector } from '@angular/core';
import { Code, ConnectError } from '@connectrpc/connect';
import type { Transport, UnaryRequest, UnaryResponse } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { isRateLimited } from './connect-errors';
import {
  rateLimitInterceptor,
  sessionEndedInterceptor,
  UNARY_DEADLINE_MS,
  withUnaryDeadline,
} from './transport';

describe('rateLimitInterceptor', () => {
  const call = (fail: unknown) =>
    rateLimitInterceptor(async () => {
      throw fail;
    })({} as UnaryRequest) as Promise<UnaryResponse>;

  it('turns a rate-limited answer into unavailable, so no screen shows a "limit reached" wording', async () => {
    const limited = new ConnectError(
      'too many requests',
      Code.ResourceExhausted,
      new Headers({ 'Retry-After': '4' }),
    );
    const err = (await call(limited).catch((e: unknown) => e)) as ConnectError;
    expect(err.code).toBe(Code.Unavailable);
    expect(err.rawMessage).toContain('Espere 4 segundos');
    expect(isRateLimited(err)).toBe(true);
  });

  it('leaves every other failure as it is', async () => {
    const full = new ConnectError('gallery full', Code.ResourceExhausted);
    await expect(call(full)).rejects.toBe(full);
    const network = new TypeError('Failed to fetch');
    await expect(call(network)).rejects.toBe(network);
  });
});

describe('sessionEndedInterceptor', () => {
  const refresh = vi.fn();
  const injector = { get: () => ({ refresh }) } as unknown as Injector;
  const call = (method: string, fail: unknown) =>
    sessionEndedInterceptor(injector)(async () => {
      throw fail;
    })({ method: { name: method } } as UnaryRequest) as Promise<UnaryResponse>;

  beforeEach(() => refresh.mockReset());

  it('has the auth state read again when any call answers unauthenticated', async () => {
    const ended = new ConnectError('no session', Code.Unauthenticated);
    await expect(call('ListCampaigns', ended)).rejects.toBe(ended);
    expect(refresh).toHaveBeenCalledTimes(1);
  });

  it('never refreshes on GetMe or SignOut, which would loop', async () => {
    const ended = new ConnectError('no session', Code.Unauthenticated);
    await expect(call('GetMe', ended)).rejects.toBe(ended);
    await expect(call('SignOut', ended)).rejects.toBe(ended);
    expect(refresh).not.toHaveBeenCalled();
  });

  it('leaves every other failure alone', async () => {
    const down = new ConnectError('down', Code.Unavailable);
    await expect(call('ListCampaigns', down)).rejects.toBe(down);
    expect(refresh).not.toHaveBeenCalled();
  });
});

/** A transport that only records the timeout each call was given. */
function recorder() {
  const unary = vi.fn().mockResolvedValue({});
  const stream = vi.fn().mockResolvedValue({});
  return { unary, stream, transport: { unary, stream } as unknown as Transport };
}

// The method, signal, headers and input do not matter here: the wrapper only
// touches the third argument, the timeout.
const method = {} as never;

describe('withUnaryDeadline', () => {
  it('gives a unary call that has no deadline the default one', async () => {
    const { unary, transport } = recorder();
    await withUnaryDeadline(transport, UNARY_DEADLINE_MS).unary(
      method,
      undefined,
      undefined,
      undefined,
      {},
    );
    expect(unary.mock.calls[0][2]).toBe(UNARY_DEADLINE_MS);
  });

  it('keeps the deadline a call set for itself', async () => {
    const { unary, transport } = recorder();
    await withUnaryDeadline(transport, UNARY_DEADLINE_MS).unary(
      method,
      undefined,
      5_000,
      undefined,
      {},
    );
    expect(unary.mock.calls[0][2]).toBe(5_000);
  });

  it('never puts a deadline on a stream, which is meant to stay open', async () => {
    const { stream, transport } = recorder();
    await withUnaryDeadline(transport, UNARY_DEADLINE_MS).stream(
      method,
      undefined,
      undefined,
      undefined,
      (async function* () {})(),
    );
    expect(stream.mock.calls[0][2]).toBeUndefined();
  });

  it('stays above the longest long poll of the server (25 s)', () => {
    expect(UNARY_DEADLINE_MS).toBeGreaterThan(25_000);
  });
});
