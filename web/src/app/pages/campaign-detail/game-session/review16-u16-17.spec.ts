// Finding U16-17 in review/unit-16-web-content-campaigns.md
import { Injectable, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import { OpenSessionVm, OpenSessions } from '../../../shell/live-notice/open-sessions';
import { GameSessionCard } from './game-session-card';
import { GameSessionSource, GameSessionVm } from './game-session-card.types';

@Injectable()
class FakeSource {
  current: GameSessionVm | null = null;
  getCurrentSession(): Promise<GameSessionVm | null> {
    return Promise.resolve(this.current);
  }
  startGameSession = vi.fn();
  endGameSession = vi.fn();
}

const gs: GameSessionVm = {
  id: 's1',
  sessionNumber: 2,
  startedAt: new Date('2026-09-29T12:00:00Z'),
  endedAt: null,
};
const open: OpenSessionVm = {
  sessionId: 's1',
  campaignId: 'camp-1',
  campaignName: 'C',
  sessionNumber: 2,
  startedAt: gs.startedAt,
  isMaster: false,
};

describe('Review16 U16-17: player card follows sessions that start/end after load', () => {
  const sessions = signal<readonly OpenSessionVm[]>([]);
  let source: FakeSource;

  beforeEach(() => {
    sessions.set([]);
    TestBed.configureTestingModule({
      imports: [GameSessionCard],
      providers: [
        provideRouter([]),
        { provide: GameSessionSource, useClass: FakeSource },
        { provide: OpenSessions, useValue: { sessions, refresh: () => Promise.resolve() } },
      ],
    });
    source = TestBed.inject(GameSessionSource) as unknown as FakeSource;
  });

  async function render() {
    const fixture = TestBed.createComponent(GameSessionCard);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('isMaster', false);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    return fixture;
  }

  async function settle(fixture: Awaited<ReturnType<typeof render>>) {
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
  }

  it('shows "Entrar na sessão" when the master starts a session while the player is on the page', async () => {
    const fixture = await render();
    expect((fixture.nativeElement as HTMLElement).textContent?.trim()).toBe('');

    source.current = gs;
    sessions.set([open]);
    await settle(fixture);

    expect((fixture.nativeElement as HTMLElement).textContent).toContain('Entrar na sessão');
  });

  it('drops "Entrar na sessão" when the master ends the session while the player is on the page', async () => {
    source.current = gs;
    sessions.set([open]);
    const fixture = await render();
    expect((fixture.nativeElement as HTMLElement).textContent).toContain('Entrar na sessão');

    source.current = null;
    sessions.set([]);
    await settle(fixture);

    expect((fixture.nativeElement as HTMLElement).textContent).not.toContain('Entrar na sessão');
  });
});
