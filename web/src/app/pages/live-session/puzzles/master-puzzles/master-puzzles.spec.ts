import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';

import { PuzzleRunStatus } from '../../../../../gen/meurpg/play/v1/puzzles_pb';
import { textOf } from '../../../../core/format/text-testing';
import { MapsClient } from '../../../../core/maps/maps-client';
import { PuzzleSessionState } from '../../../../core/puzzles/puzzle-session';
import { PuzzlesClient } from '../../../../core/puzzles/puzzles-client';
import {
  FakePuzzlesClient,
  asClient,
  lightsPuzzle,
  lockPuzzle,
  masterRun,
  pillarsPuzzle,
} from '../../../../core/puzzles/puzzles-testing';
import { MasterPuzzles } from './master-puzzles';
import { nextTurn } from '../../../../../test-settle';

describe('MasterPuzzles (MR-038, E10-06 states 3 and 4)', () => {
  const a = lightsPuzzle('a', 'O selo da Capela');
  const b = lockPuzzle('b', 'O cofre do Refeitório');
  const c = pillarsPuzzle('c', 'Os pilares da Galeria');
  let api: FakePuzzlesClient;
  let state: PuzzleSessionState;

  async function render(runs: ReturnType<typeof masterRun>[]) {
    api = new FakePuzzlesClient();
    api.sessionResult = runs;
    state = new PuzzleSessionState(
      asClient(api),
      () => 'camp-1',
      () => true,
    );
    await state.refresh();
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: PuzzlesClient, useValue: api },
        {
          provide: MapsClient,
          useValue: { list: async () => [], get: async () => ({}), layers: async () => ({}) },
        },
      ],
    });
    const fixture = TestBed.createComponent(MasterPuzzles);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('state', state);
    const settle = async () => {
      for (let i = 0; i < 3; i++) {
        fixture.detectChanges();
        await fixture.whenStable();
        await nextTurn();
      }
      fixture.detectChanges();
    };
    await settle();
    return { el: fixture.nativeElement as HTMLElement, settle, fixture };
  }

  it('lists each puzzle with where it stands, and "Mostrar aos jogadores" on the ones not shown', async () => {
    const { el } = await render([
      masterRun(a, PuzzleRunStatus.SHOWN),
      masterRun(b, PuzzleRunStatus.NOT_SHOWN),
      masterRun(c, PuzzleRunStatus.NOT_SHOWN),
    ]);
    const rows = Array.from(el.querySelectorAll('.mr-list li')).map((r) => textOf(r));
    expect(rows[0]).toContain('O selo da Capela');
    expect(rows[0]).toContain('Mostrado agora');
    expect(rows[0]).not.toContain('Mostrar aos jogadores');
    expect(rows[1]).toContain('Não mostrado');
    expect(rows[1]).toContain('Mostrar aos jogadores');
    expect(rows[2]).toContain('Mostrar aos jogadores');
  });

  it('offers "Ver ao vivo" on what is shown or solved (the card itself is in the main column), and the first one is chosen', async () => {
    const { el, fixture } = await render([
      masterRun(a, PuzzleRunStatus.SHOWN),
      masterRun(b, PuzzleRunStatus.SOLVED),
      masterRun(c, PuzzleRunStatus.CLOSED),
    ]);
    expect(el.querySelector('app-master-run')).toBeNull();
    const live = Array.from(el.querySelectorAll('[aria-label^="Ver ao vivo"]'));
    expect(live.map((l) => l.getAttribute('aria-label'))).toEqual([
      'Ver ao vivo O selo da Capela',
      'Ver ao vivo O cofre do Refeitório',
    ]);
    expect(state.selectedId()).toBe('a');
    expect(live[0].getAttribute('aria-pressed')).toBe('true');
    expect(live[1].getAttribute('aria-pressed')).toBe('false');
    (live[1] as HTMLElement).click();
    fixture.detectChanges();
    expect(state.selectedId()).toBe('b');
    // A closed puzzle is a row again, and offers to show it again.
    expect(textOf(el.querySelectorAll('.mr-list li')[2])).toContain('Fechado');
    expect(textOf(el.querySelectorAll('.mr-list li')[2])).toContain('Mostrar de novo');
  });

  it('shows a puzzle on one tap: the answer opens its card and the row follows', async () => {
    const { el, settle } = await render([masterRun(b, PuzzleRunStatus.NOT_SHOWN)]);
    api.runResults.set('b', masterRun(b, PuzzleRunStatus.SHOWN));
    (
      el.querySelector('[aria-label="Mostrar aos jogadores O cofre do Refeitório"]') as HTMLElement
    ).click();
    await settle();
    expect(api.calls.find((c) => c[0] === 'show')).toEqual(['show', 'camp-1', 'b']);
    expect(state.selectedId()).toBe('b');
    expect(textOf(el.querySelector('.mr-list li'))).toContain('Mostrado agora');
    expect(el.querySelector('[aria-label="Ver ao vivo O cofre do Refeitório"]')).not.toBeNull();
  });

  it('says why it could not show it, by code', async () => {
    const { el, settle } = await render([masterRun(b, PuzzleRunStatus.NOT_SHOWN)]);
    api.failWith = new ConnectError('x', Code.Unavailable);
    (
      el.querySelector('[aria-label="Mostrar aos jogadores O cofre do Refeitório"]') as HTMLElement
    ).click();
    await settle();
    expect(el.querySelector('[role="alert"]')?.textContent).toContain(
      'Não deu para mostrar o quebra-cabeça',
    );
  });

  it('follows the stream: a change of a shown puzzle reads it again and the row follows', async () => {
    const { el, settle } = await render([masterRun(a, PuzzleRunStatus.SHOWN)]);
    api.runResults.set('a', masterRun(a, PuzzleRunStatus.SOLVED));
    await state.changed('a');
    await settle();
    expect(textOf(el.querySelector('.mr-list li'))).toContain('Resolvido');
  });

  it('says when the list could not be read', async () => {
    api = new FakePuzzlesClient();
    api.failWith = new Error('down');
    state = new PuzzleSessionState(
      asClient(api),
      () => 'camp-1',
      () => true,
    );
    await state.refresh();
    TestBed.configureTestingModule({
      providers: [provideRouter([]), { provide: PuzzlesClient, useValue: api }],
    });
    const fixture = TestBed.createComponent(MasterPuzzles);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('state', state);
    fixture.detectChanges();
    expect((fixture.nativeElement as HTMLElement).textContent).toContain(
      'Não deu para ler os quebra-cabeças.',
    );
  });
});
