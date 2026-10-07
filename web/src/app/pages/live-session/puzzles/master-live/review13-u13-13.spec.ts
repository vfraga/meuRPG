// Finding U13-13 (review/unit-13-web-live-rest.md): MasterLive reuses one MasterRun when the selection moves between runs, so its local state leaks to the next puzzle.
import { TestBed } from '@angular/core/testing';

import { PuzzleRunStatus } from '../../../../../gen/meurpg/play/v1/puzzles_pb';
import { PuzzleSessionState } from '../../../../core/puzzles/puzzle-session';
import { PuzzlesClient } from '../../../../core/puzzles/puzzles-client';
import { MapsClient } from '../../../../core/maps/maps-client';
import {
  FakePuzzlesClient,
  asClient,
  lockPuzzle,
  masterRun,
} from '../../../../core/puzzles/puzzles-testing';
import { MasterLive } from './master-live';

describe('Review13 U13-13: MasterLive keeps the previous puzzle local state', () => {
  const a = lockPuzzle('a', 'Cofre A');
  const b = lockPuzzle('b', 'Cofre B');

  async function render() {
    const api = new FakePuzzlesClient();
    api.sessionResult = [masterRun(a, PuzzleRunStatus.SHOWN), masterRun(b, PuzzleRunStatus.SHOWN)];
    const state = new PuzzleSessionState(
      asClient(api),
      () => 'camp-1',
      () => true,
    );
    await state.refresh();
    TestBed.configureTestingModule({
      providers: [
        { provide: PuzzlesClient, useValue: api },
        { provide: MapsClient, useValue: {} },
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
    const el = fixture.nativeElement as HTMLElement;
    const button = (text: string) =>
      [...el.querySelectorAll<HTMLButtonElement>('button')].find((x) =>
        x.textContent?.includes(text),
      );
    await settle();
    return { el, state, settle, button };
  }

  it('control: the solution toggle works on the chosen puzzle', async () => {
    const { el, state, settle, button } = await render();
    state.select('a');
    await settle();
    expect(el.querySelector('app-lock-board[label="Solução da fechadura"]')).toBeNull();
    button('Mostrar a solução só para mim')!.click();
    await settle();
    expect(el.querySelector('h3')?.textContent).toBe('Cofre A');
    expect(el.querySelector('app-lock-board[label="Solução da fechadura"]')).not.toBeNull();
  });

  it('hides the solution of A when the master moves to B', async () => {
    const { el, state, settle, button } = await render();
    state.select('a');
    await settle();
    button('Mostrar a solução só para mim')!.click();
    await settle();
    state.select('b');
    await settle();
    expect(el.querySelector('h3')?.textContent).toBe('Cofre B');
    expect(el.querySelector('app-lock-board[label="Solução da fechadura"]')).toBeNull();
  });

  it('drops an open close question of A when the master moves to B', async () => {
    const { el, state, settle } = await render();
    state.select('a');
    await settle();
    el.querySelector<HTMLButtonElement>('[data-act="close"]')!.click();
    await settle();
    expect(el.textContent).toContain('Fechar “Cofre A”?');
    state.select('b');
    await settle();
    expect(el.querySelector('h3')?.textContent).toBe('Cofre B');
    expect(el.textContent).not.toContain('Fechar “Cofre B”?');
  });
});
