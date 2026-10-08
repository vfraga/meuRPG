import { Injectable, signal } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';

import { OpenSessionVm, OpenSessions } from '../../../shell/live-notice/open-sessions';
import { GameSessionCard } from './game-session-card';
import {
  GameSessionSource,
  GameSessionVm,
  StartGameSessionResultVm,
} from './game-session-card.types';

@Injectable()
class FakeGameSessionSource {
  getCurrentSessionResult: Promise<GameSessionVm | null> = Promise.resolve(null);
  readonly startGameSession = vi.fn();
  readonly endGameSession = vi.fn();

  getCurrentSession(): Promise<GameSessionVm | null> {
    return this.getCurrentSessionResult;
  }
}

function session(sessionNumber: number, id = `sess-${sessionNumber}`): GameSessionVm {
  return { id, sessionNumber, startedAt: new Date('2026-09-29T12:00:00Z'), endedAt: null };
}

function startResult(sessionNumber: number, lockedSheetCount: number): StartGameSessionResultVm {
  return { session: session(sessionNumber), lockedSheetCount };
}

describe('GameSessionCard', () => {
  let fake: FakeGameSessionSource;
  const polled = signal<readonly OpenSessionVm[]>([]);
  const openSessions = { sessions: polled, refresh: vi.fn(() => Promise.resolve()) };
  const polledSession: OpenSessionVm = {
    sessionId: 'sess-2',
    campaignId: 'camp-1',
    campaignName: 'Mirathel',
    sessionNumber: 2,
    startedAt: new Date('2026-09-29T12:00:00Z'),
    isMaster: false,
  };

  beforeEach(() => {
    openSessions.refresh.mockClear();
    polled.set([]);
    TestBed.configureTestingModule({
      imports: [GameSessionCard],
      providers: [
        provideRouter([]),
        { provide: GameSessionSource, useClass: FakeGameSessionSource },
        { provide: OpenSessions, useValue: openSessions },
      ],
    });
    fake = TestBed.inject(GameSessionSource) as unknown as FakeGameSessionSource;
  });

  async function render(isMaster = true): Promise<{
    el: HTMLElement;
    fixture: ComponentFixture<GameSessionCard>;
  }> {
    const fixture = TestBed.createComponent(GameSessionCard);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('isMaster', isMaster);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    return { el: fixture.nativeElement as HTMLElement, fixture };
  }

  it('shows "Iniciar sessão" when no session is open', async () => {
    fake.getCurrentSessionResult = Promise.resolve(null);
    const { el } = await render();
    expect(el.textContent).toContain('Iniciar sessão');
    expect(el.textContent).not.toContain('Encerrar sessão');
  });

  it('shows "Sessão N em andamento" and "Encerrar sessão" when one is open', async () => {
    fake.getCurrentSessionResult = Promise.resolve(session(3));
    const { el } = await render();
    expect(el.textContent).toContain('Sessão 3 em andamento');
    expect(el.textContent).toContain('Encerrar sessão');
    expect(el.textContent).not.toContain('Iniciar sessão');
  });

  it('starts a session, switches to "em andamento", and shows the locked-sheet count', async () => {
    fake.getCurrentSessionResult = Promise.resolve(null);
    fake.startGameSession.mockResolvedValue(startResult(1, 4));
    const { el, fixture } = await render();

    const button = Array.from(el.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Iniciar sessão'),
    ) as HTMLButtonElement;
    button.click();
    await fixture.whenStable();
    fixture.detectChanges();

    expect(fake.startGameSession).toHaveBeenCalledWith('camp-1', expect.any(String));
    expect(el.textContent).toContain('Sessão 1 em andamento');
    expect(el.textContent).toContain('4 fichas travadas.');
  });

  it('says "1 ficha travada." — singular, not "1 fichas travadas." (integrator fix)', async () => {
    fake.getCurrentSessionResult = Promise.resolve(null);
    fake.startGameSession.mockResolvedValue(startResult(1, 1));
    const { el, fixture } = await render();

    const button = Array.from(el.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Iniciar sessão'),
    ) as HTMLButtonElement;
    button.click();
    await fixture.whenStable();
    fixture.detectChanges();

    expect(el.textContent).toContain('1 ficha travada.');
    expect(el.textContent).not.toContain('1 fichas travadas.');
  });

  it('says "0 fichas travadas." for zero — plural, still sensible', async () => {
    fake.getCurrentSessionResult = Promise.resolve(null);
    fake.startGameSession.mockResolvedValue(startResult(1, 0));
    const { el, fixture } = await render();

    const button = Array.from(el.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Iniciar sessão'),
    ) as HTMLButtonElement;
    button.click();
    await fixture.whenStable();
    fixture.detectChanges();

    expect(el.textContent).toContain('0 fichas travadas.');
  });

  it('shows a clear message when starting fails because one is already open', async () => {
    fake.getCurrentSessionResult = Promise.resolve(null);
    fake.startGameSession.mockRejectedValue(new ConnectError('open', Code.FailedPrecondition));
    const { el, fixture } = await render();

    const button = Array.from(el.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Iniciar sessão'),
    ) as HTMLButtonElement;
    button.click();
    await fixture.whenStable();
    fixture.detectChanges();

    expect(el.textContent).toContain('Já existe uma sessão em andamento nesta campanha.');
  });

  it('ends a session (by its id) after the in-place confirmation, and switches back to the "nenhuma sessão" state', async () => {
    fake.getCurrentSessionResult = Promise.resolve(session(2, 'sess-2'));
    fake.endGameSession.mockResolvedValue({ ...session(2, 'sess-2'), endedAt: new Date() });
    const { el, fixture } = await render();

    buttonNamed(el, 'Encerrar sessão').click();
    await fixture.whenStable();
    fixture.detectChanges();
    // Can't be undone: it asks first, in place (docs/design.md).
    expect(fake.endGameSession).not.toHaveBeenCalled();
    expect(el.textContent).toContain('Encerrar a sessão 2? Ela não reabre depois.');

    buttonNamed(el, 'Confirmar encerramento').click();
    await fixture.whenStable();
    fixture.detectChanges();

    expect(fake.endGameSession).toHaveBeenCalledWith('camp-1', 'sess-2');
    expect(el.textContent).toContain('Nenhuma sessão em andamento');
    // The app bar's "Ao vivo" link goes away now, not at the next poll.
    expect(openSessions.refresh).toHaveBeenCalled();
  });

  it('"Cancelar" goes back to "Encerrar sessão" without ending anything', async () => {
    fake.getCurrentSessionResult = Promise.resolve(session(2, 'sess-2'));
    const { el, fixture } = await render();

    buttonNamed(el, 'Encerrar sessão').click();
    await fixture.whenStable();
    fixture.detectChanges();
    buttonNamed(el, 'Cancelar').click();
    await fixture.whenStable();
    fixture.detectChanges();

    expect(fake.endGameSession).not.toHaveBeenCalled();
    expect(el.textContent).not.toContain('Confirmar encerramento');
    expect(el.textContent).toContain('Encerrar sessão');
  });

  it('links "Entrar na sessão" to the session page (E5-09)', async () => {
    fake.getCurrentSessionResult = Promise.resolve(session(4));
    const { el } = await render();
    const link = Array.from(el.querySelectorAll('a')).find((a) =>
      a.textContent?.includes('Entrar na sessão'),
    );
    expect(link?.getAttribute('href')).toBe('/campaigns/camp-1/session');
    expect(el.textContent).toContain('Ao vivo');
  });

  it('copies the session link and says "Link copiado"', async () => {
    fake.getCurrentSessionResult = Promise.resolve(session(4));
    const writeText = vi.fn(() => Promise.resolve());
    vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText } });
    try {
      const { el, fixture } = await render();
      buttonNamed(el, 'Copiar link da sessão').click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(writeText).toHaveBeenCalledWith(`${location.origin}/campaigns/camp-1/session`);
      expect(el.textContent).toContain('Link copiado');
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it('shows the link in a read-only field when the browser refuses to copy', async () => {
    fake.getCurrentSessionResult = Promise.resolve(session(4));
    vi.stubGlobal('navigator', {
      ...navigator,
      clipboard: { writeText: () => Promise.reject(new Error('denied')) },
    });
    try {
      const { el, fixture } = await render();
      buttonNamed(el, 'Copiar link da sessão').click();
      await fixture.whenStable();
      fixture.detectChanges();

      const field = el.querySelector<HTMLInputElement>('input[readonly]');
      expect(field?.value).toBe(`${location.origin}/campaigns/camp-1/session`);
      expect(el.textContent).not.toContain('Link copiado');
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it('shows a player the open session with "Entrar na sessão", and nothing to manage', async () => {
    fake.getCurrentSessionResult = Promise.resolve(session(3));
    const { el } = await render(false);
    expect(el.textContent).toContain('Sessão 3 em andamento');
    expect(el.textContent).toContain('Entrar na sessão');
    expect(el.textContent).not.toContain('Encerrar sessão');
    expect(el.textContent).not.toContain('Copiar link da sessão');
  });

  it('shows a player nothing at all while no session is open', async () => {
    fake.getCurrentSessionResult = Promise.resolve(null);
    const { el } = await render(false);
    expect(el.textContent?.trim()).toBe('');
    expect(el.classList).toContain('is-empty');
  });

  it('shows a player "Entrar na sessão" when the master starts a session while the page is open', async () => {
    const { el, fixture } = await render(false);
    expect(el.textContent?.trim()).toBe('');

    fake.getCurrentSessionResult = Promise.resolve(session(2));
    polled.set([polledSession]);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();

    expect(el.textContent).toContain('Entrar na sessão');
  });

  it('takes "Entrar na sessão" from a player when the master ends the session while the page is open', async () => {
    fake.getCurrentSessionResult = Promise.resolve(session(2));
    polled.set([polledSession]);
    const { el, fixture } = await render(false);
    expect(el.textContent).toContain('Entrar na sessão');

    fake.getCurrentSessionResult = Promise.resolve(null);
    polled.set([]);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();

    expect(el.textContent).not.toContain('Entrar na sessão');
  });

  it('sends the same idempotency key when the start is tried again after a failure', async () => {
    fake.getCurrentSessionResult = Promise.resolve(null);
    fake.startGameSession.mockRejectedValueOnce(new ConnectError('down', Code.Unavailable));
    fake.startGameSession.mockResolvedValue(startResult(1, 1));
    const { el, fixture } = await render();

    buttonNamed(el, 'Iniciar sessão').click();
    await fixture.whenStable();
    fixture.detectChanges();
    buttonNamed(el, 'Iniciar sessão').click();
    await fixture.whenStable();

    const [first, retry] = fake.startGameSession.mock.calls.map((c) => c[1] as string);
    expect(retry).toBe(first);
  });
});

function buttonNamed(el: HTMLElement, name: string): HTMLButtonElement {
  return Array.from(el.querySelectorAll('button')).find((b) =>
    b.textContent?.includes(name),
  ) as HTMLButtonElement;
}
