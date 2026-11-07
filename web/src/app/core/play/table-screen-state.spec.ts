import { Code, ConnectError } from '@connectrpc/connect';

import { FakeClock, noOpenSession, screenResponse } from './table-screen-testing';
import { TV_LATE_GRACE_MS, TV_RETRY_MS, TV_STALE_MS, TvScreenState } from './table-screen-state';
import { toTvScreen } from './table-screen-view';

const START = new Date(2026, 0, 5, 21, 10);
const SCREEN = toTvScreen(screenResponse());

describe('TvScreenState', () => {
  let clock: FakeClock;
  let reads: number;
  let state: TvScreenState;

  beforeEach(() => {
    clock = new FakeClock(START);
    reads = 0;
    state = new TvScreenState(clock, () => reads++);
  });

  it('starts loading and shows the screen once read', () => {
    expect(state.phase()).toEqual({ kind: 'loading', text: 'Carregando a mesa…' });
    state.loaded(SCREEN);
    expect(state.phase()).toEqual({ kind: 'ready', screen: SCREEN, late: null });
  });

  it('says the error and reads again in 10 s, until a read works', () => {
    state.readFailed(new ConnectError('down', Code.Unavailable));
    expect(state.phase()).toMatchObject({
      kind: 'error',
      title: 'Não deu para abrir a Tela da mesa.',
    });
    clock.advance(TV_RETRY_MS - 1);
    expect(reads).toBe(0);
    clock.advance(1);
    expect(reads).toBe(1);
    state.readFailed(new ConnectError('down', Code.Unavailable));
    clock.advance(TV_RETRY_MS);
    expect(reads).toBe(2);
    state.loaded(SCREEN);
    expect(state.phase().kind).toBe('ready');
    expect(clock.pending).toBe(0);
  });

  it('marks a shown screen late when a later read fails, reads again every 10 s, and hides it after a minute', () => {
    state.loaded(SCREEN);
    state.readFailed(new ConnectError('down', Code.Unavailable));
    clock.advance(TV_LATE_GRACE_MS);
    expect(state.phase()).toMatchObject({ kind: 'ready', screen: SCREEN });
    expect((state.phase() as { late: string }).late).toContain('atrasada desde 21:10');
    clock.advance(TV_RETRY_MS - TV_LATE_GRACE_MS);
    expect(reads).toBe(1);
    state.readFailed(new ConnectError('down', Code.Unavailable));
    clock.advance(TV_RETRY_MS);
    expect(reads).toBe(2);
    clock.advance(TV_STALE_MS);
    expect(state.phase().kind).toBe('stale');
  });

  it('is on time again once a read works', () => {
    state.loaded(SCREEN);
    state.readFailed(new ConnectError('down', Code.Unavailable));
    clock.advance(TV_RETRY_MS);
    state.loaded(SCREEN);
    expect(state.phase()).toEqual({ kind: 'ready', screen: SCREEN, late: null });
    expect(clock.pending).toBe(0);
  });

  it('does not flash the warning for a reconnection that takes a second', () => {
    state.loaded(SCREEN);
    state.connection(false);
    clock.advance(1000);
    expect(state.phase()).toEqual({ kind: 'ready', screen: SCREEN, late: null });
    state.connection(true);
    clock.advance(TV_STALE_MS);
    expect(state.phase().kind).toBe('ready');
  });

  it('shows a lost sign-in, and never retries the read', () => {
    state.readFailed(new ConnectError('who', Code.Unauthenticated));
    expect(state.phase()).toMatchObject({ kind: 'signed-out' });
    expect(clock.pending).toBe(0);
    clock.advance(TV_RETRY_MS * 3);
    expect(reads).toBe(0);
  });

  it('warns with the hour a connection was lost, then hides the screen after a minute, then comes back', () => {
    state.loaded(SCREEN);
    clock.advance(1000);
    state.connection(false);
    clock.advance(TV_LATE_GRACE_MS);
    expect(state.phase()).toEqual({
      kind: 'ready',
      screen: SCREEN,
      late: 'Esta tela pode estar atrasada desde 21:10. Ela continua assim por até 1 minuto.',
    });
    clock.advance(TV_STALE_MS - TV_LATE_GRACE_MS - 1);
    expect(state.phase().kind).toBe('ready');
    clock.advance(1);
    expect(state.phase()).toEqual({
      kind: 'stale',
      title: 'Sem conexão desde 21:10',
      detail: 'Esta tela escondeu o que mostrava. Ela volta sozinha quando a conexão voltar.',
      campaignName: 'Mirathel',
      sessionLabel: 'Sessão 3',
    });
    state.connection(true);
    expect(state.phase()).toEqual({ kind: 'ready', screen: SCREEN, late: null });
  });

  it('counts the minute from the first loss, not from each reconnecting try', () => {
    state.loaded(SCREEN);
    state.connection(false);
    clock.advance(TV_STALE_MS / 2);
    state.connection(false);
    clock.advance(TV_STALE_MS / 2);
    expect(state.phase().kind).toBe('stale');
  });

  it('starts the minute again after a connection that came back', () => {
    state.loaded(SCREEN);
    state.connection(false);
    clock.advance(TV_STALE_MS / 2);
    state.connection(true);
    state.connection(false);
    clock.advance(TV_STALE_MS / 2);
    expect(state.phase().kind).toBe('ready');
    clock.advance(TV_STALE_MS / 2);
    expect(state.phase().kind).toBe('stale');
  });

  it('says the session ended, with the hour and the campaign', () => {
    state.loaded(SCREEN);
    clock.advance(60_000);
    state.ended();
    expect(state.phase()).toEqual({
      kind: 'ended',
      title: 'A sessão 3 terminou às 21:11.',
      farewell: 'Até a próxima sessão de Mirathel.',
      hint: 'O mestre pode fechar esta aba.',
      campaignName: 'Mirathel',
      sessionLabel: 'Sessão 3',
      endedAt: '21:11',
    });
  });

  it('an ended session that was never read names no campaign', () => {
    state.readFailed(noOpenSession());
    expect(state.phase()).toMatchObject({
      kind: 'ended',
      title: 'Não há sessão aberta.',
      farewell: '',
      campaignName: '',
      sessionLabel: '',
      endedAt: expect.any(String),
    });
  });

  it('refuses a tab that is not the master, naming nothing', () => {
    state.loaded(SCREEN);
    state.readFailed(new ConnectError('no', Code.PermissionDenied));
    expect(state.phase()).toEqual({
      kind: 'no-access',
      title: 'Só o mestre abre a Tela da mesa.',
      detail: 'Ela é a tela da sala; você joga pela sessão no seu aparelho.',
    });
  });

  it('refuses on not found too, and stops every timer', () => {
    state.connection(false);
    state.readFailed(new ConnectError('no', Code.NotFound));
    expect(state.phase().kind).toBe('no-access');
    expect(clock.pending).toBe(0);
  });

  it('is stale after a minute without a connection even before the first read', () => {
    state.connection(false);
    clock.advance(TV_STALE_MS);
    expect(state.phase().kind).toBe('stale');
  });
});
