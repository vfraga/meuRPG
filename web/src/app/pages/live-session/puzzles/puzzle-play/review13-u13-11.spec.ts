// Finding U13-11 (review/unit-13-web-live-rest.md): after the master closes a puzzle the page still offers a live "Tentar uma dica" that the server answers with NotFound.
import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';

import { PuzzleSessionState } from '../../../../core/puzzles/puzzle-session';
import { PuzzlesClient } from '../../../../core/puzzles/puzzles-client';
import { SceneChecks } from '../../../../core/maps/scene-actions';
import {
  FakePuzzlesClient,
  NOW,
  asClient,
  fakeChecks,
  playerRun,
  riddlePuzzle,
} from '../../../../core/puzzles/puzzles-testing';
import { PuzzlePlayPage } from './puzzle-play';

@Component({
  imports: [PuzzlePlayPage],
  template: `<app-puzzle-play campaignId="camp-1" puzzleId="a" [session]="session" [state]="state" ownName="Toren" [diceMode]="1" [dicePreference]="1" [reconnecting]="reconnecting()" />`,
})
class Host {
  session = { sessionId: 's7', sessionNumber: 7, startedAt: new Date() };
  state!: PuzzleSessionState;
  reconnecting = signal(false);
}

describe('Review13 U13-11: a closed puzzle still offers the hint try', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(NOW);
  });
  afterEach(() => document.body.replaceChildren());

  async function setup() {
    const api = new FakePuzzlesClient();
    api.playerRunResult = playerRun(riddlePuzzle('a', 'A porta da Cripta pergunta'), {
      hintByCheck: true,
      hintSkillKey: 'skill:investigation',
      canTryHint: true,
    });
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: PuzzlesClient, useValue: api },
        { provide: SceneChecks, useValue: fakeChecks },
      ],
    });
    const fixture = TestBed.createComponent(Host);
    const state = new PuzzleSessionState(
      asClient(api),
      () => 'camp-1',
      () => false,
    );
    fixture.componentInstance.state = state;
    document.body.append(fixture.nativeElement);
    const settle = async () => {
      for (let i = 0; i < 3; i++) {
        fixture.detectChanges();
        await fixture.whenStable();
        await new Promise((r) => setTimeout(r));
      }
      fixture.detectChanges();
    };
    await settle();
    const el = fixture.nativeElement as HTMLElement;
    const tryButton = () =>
      Array.from(el.querySelectorAll('button')).find((b) =>
        b.textContent?.includes('Tentar uma dica'),
      );
    return { api, state, el, settle, tryButton };
  }

  it('control: an open puzzle offers the hint try, and tapping it calls the server', async () => {
    const { api, el, settle, tryButton } = await setup();
    expect(el.querySelector('.mr-notice--neutral')).toBeNull();
    expect(tryButton()).toBeTruthy();
    tryButton()!.click();
    await settle();
    expect(api.calls.some((c) => c[0] === 'tryHint')).toBe(true);
  });

  it('after the master closed it, the hint try is not offered', async () => {
    const { api, state, el, settle, tryButton } = await setup();
    api.failWith = new ConnectError('x', Code.NotFound);
    await state.changed('a');
    await settle();
    expect(el.textContent).toContain('O mestre fechou o quebra-cabeça.');
    expect(tryButton()).toBeUndefined();
    expect(api.calls.some((c) => c[0] === 'tryHint')).toBe(false);
  });
});
