import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import type { Transport } from '@connectrpc/connect';

import { GetMeResponseSchema } from '../../../gen/meurpg/identity/v1/identity_pb';
import { AuthService } from '../../core/auth/auth.service';
import { CONNECT_TRANSPORT } from '../../core/connect/transport';
import { OPEN_SESSIONS_FETCHER, OpenSessionVm, OpenSessions, POLL_INTERVAL_MS } from './open-sessions';

// Finding U10-1, see review/unit-10-web-core-stream.md
describe('Review10 U10-1: an unauthenticated answer of the poll is never acted on', () => {
  let getMeCalls: number;
  let fetch: ReturnType<typeof vi.fn<() => Promise<OpenSessionVm[]>>>;

  beforeEach(() => {
    vi.useFakeTimers();
    getMeCalls = 0;
    // GetMe answers OK the first time (the constructor); the server then
    // loses the session, so every later call is unauthenticated.
    const transport: Transport = {
      unary: (method) => {
        getMeCalls++;
        if (getMeCalls > 1) {
          return Promise.reject(new ConnectError('no session', Code.Unauthenticated)) as never;
        }
        return Promise.resolve({
          stream: false,
          service: method.parent,
          method,
          header: new Headers(),
          trailer: new Headers(),
          message: create(GetMeResponseSchema, { user: { id: 'u1', displayName: 'Ana' } }),
        }) as never;
      },
      stream(): never {
        throw new Error('no streams');
      },
    };
    fetch = vi.fn(() => Promise.reject(new ConnectError('no session', Code.Unauthenticated)));
    TestBed.configureTestingModule({
      providers: [
        { provide: CONNECT_TRANSPORT, useValue: transport },
        { provide: OPEN_SESSIONS_FETCHER, useValue: () => Promise.resolve(fetch) },
      ],
    });
  });

  afterEach(() => vi.useRealTimers());

  it('after an unauthenticated poll, the poll stops or the auth state becomes signed-out', async () => {
    const auth = TestBed.inject(AuthService);
    TestBed.inject(OpenSessions);
    await vi.advanceTimersByTimeAsync(0);
    TestBed.tick();
    await vi.advanceTimersByTimeAsync(0);
    expect(auth.state().status).toBe('signed-in');
    expect(fetch).toHaveBeenCalledTimes(1); // first poll: unauthenticated

    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 3);

    const pollStopped = fetch.mock.calls.length === 1;
    const signedOut = auth.state().status === 'signed-out';
    expect(
      pollStopped || signedOut,
      `poll calls=${fetch.mock.calls.length}, auth=${auth.state().status}, GetMe calls=${getMeCalls}`,
    ).toBe(true);
  });
});
