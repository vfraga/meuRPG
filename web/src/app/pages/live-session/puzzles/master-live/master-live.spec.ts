import { TestBed } from '@angular/core/testing';

import { PuzzleRunStatus } from '../../../../../gen/meurpg/play/v1/puzzles_pb';
import { PuzzleSessionState } from '../../../../core/puzzles/puzzle-session';
import { PuzzlesClient } from '../../../../core/puzzles/puzzles-client';
import {
  FakePuzzlesClient,
  asClient,
  lightsPuzzle,
  lockPuzzle,
  masterRun,
} from '../../../../core/puzzles/puzzles-testing';
import { MasterLive } from './master-live';

describe('MasterLive (the open live card, in the main column)', () => {
  const a = lightsPuzzle('a', 'O selo da Capela');
  const b = lockPuzzle('b', 'O cofre do Refeitório');

  async function render(puzzles = [a, b]) {
    const api = new FakePuzzlesClient();
    api.sessionResult = puzzles.map((p) => masterRun(p, PuzzleRunStatus.SHOWN));
    const state = new PuzzleSessionState(
      asClient(api),
      () => 'camp-1',
      () => true,
    );
    await state.refresh();
    TestBed.configureTestingModule({
      providers: [
        { provide: PuzzlesClient, useValue: api },
        { provide: (await import('../../../../core/maps/maps-client')).MapsClient, useValue: {} },
      ],
    });
    const fixture = TestBed.createComponent(MasterLive);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('state', state);
    const settle = async () => {
      fixture.detectChanges();
      await fixture.whenStable();
      fixture.detectChanges();
    };
    await settle();
    return { el: fixture.nativeElement as HTMLElement, state, settle };
  }

  it('shows nothing until a puzzle is chosen, and only the chosen one, one at a time', async () => {
    const { el, state, settle } = await render();
    expect(el.querySelector('app-master-run')).toBeNull();
    state.select('a');
    await settle();
    expect(el.querySelectorAll('app-master-run')).toHaveLength(1);
    expect(el.querySelector('h3')?.textContent).toBe('O selo da Capela');
    state.select('b');
    await settle();
    expect(el.querySelectorAll('app-master-run')).toHaveLength(1);
    expect(el.querySelector('h3')?.textContent).toBe('O cofre do Refeitório');
  });

  it('lets a closed puzzle go back to being a row', async () => {
    const { el, state, settle } = await render();
    state.select('a');
    await settle();
    state.replace(masterRun(a, PuzzleRunStatus.CLOSED));
    await settle();
    expect(el.querySelector('app-master-run')).toBeNull();
  });

  describe('switching between two puzzles', () => {
    const vault = lockPuzzle('c', 'Cofre A');
    const other = lockPuzzle('d', 'Cofre B');
    const solution = 'app-lock-board[label="Solução da fechadura"]';

    it('hides the solution the master showed for one puzzle when another is chosen', async () => {
      const { el, state, settle } = await render([vault, other]);
      state.select('c');
      await settle();
      expect(el.querySelector(solution)).toBeNull();
      [...el.querySelectorAll<HTMLButtonElement>('button')]
        .find((x) => x.textContent?.includes('Mostrar a solução só para mim'))!
        .click();
      await settle();
      expect(el.querySelector('h3')?.textContent).toBe('Cofre A');
      expect(el.querySelector(solution)).not.toBeNull();
      state.select('d');
      await settle();
      expect(el.querySelector('h3')?.textContent).toBe('Cofre B');
      expect(el.querySelector(solution)).toBeNull();
    });

    it('drops the open close question of one puzzle when another is chosen', async () => {
      const { el, state, settle } = await render([vault, other]);
      state.select('c');
      await settle();
      el.querySelector<HTMLButtonElement>('[data-act="close"]')!.click();
      await settle();
      expect(el.textContent).toContain('Fechar “Cofre A”?');
      state.select('d');
      await settle();
      expect(el.querySelector('h3')?.textContent).toBe('Cofre B');
      expect(el.textContent).not.toContain('Fechar “Cofre B”?');
    });
  });
});
