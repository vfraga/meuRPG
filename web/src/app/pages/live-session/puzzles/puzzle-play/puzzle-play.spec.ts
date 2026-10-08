import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  PuzzleBlockedReason,
  PuzzleBlockedSchema,
  type PuzzleRun,
} from '../../../../../gen/meurpg/play/v1/puzzles_pb';
import { create } from '@bufbuild/protobuf';
import { textOf } from '../../../../core/format/text-testing';
import { PuzzleSessionState } from '../../../../core/puzzles/puzzle-session';
import { PuzzlesClient } from '../../../../core/puzzles/puzzles-client';
import { SceneChecks } from '../../../../core/maps/scene-actions';
import {
  FakePuzzlesClient,
  NOW,
  asClient,
  at,
  cipherPuzzle,
  fakeChecks,
  fromNow,
  hintAnswer,
  lightsPuzzle,
  lockPuzzle,
  pillarsPuzzle,
  playerRun,
  riddlePuzzle,
  sequencePuzzle,
} from '../../../../core/puzzles/puzzles-testing';
import { PuzzlePlayPage } from './puzzle-play';

@Component({
  imports: [PuzzlePlayPage],
  template: `<app-puzzle-play campaignId="camp-1" [puzzleId]="id" [session]="session" [state]="state" ownName="Toren" [diceMode]="diceMode" [dicePreference]="preference" [reconnecting]="reconnecting()" (openNotes)="notes = notes + 1" (diceModeStale)="stale = stale + 1" />`,
})
class Host {
  diceMode = 1;
  preference = 1;
  notes = 0;
  stale = 0;
  id = 'a';
  session = { sessionId: 's7', sessionNumber: 7, startedAt: new Date() };
  state!: PuzzleSessionState;
  reconnecting = signal(false);
}

const lit = (on: number[]) => Array.from({ length: 25 }, (_, i) => on.includes(i));

describe('PuzzlePlayPage (MR-038, RN-27, RN-10; E10-06 states 6 to 9)', () => {
  let api: FakePuzzlesClient;
  let host: Host;

  // The page reads the real clock ("agora há pouco", the frame on what just changed) and the runs are dated from the
  // fixture's NOW: the clock is NOW, or the tests depend on the hour they run at (they failed after 21:12 on 06/10).
  // Only Date is faked: the page's own timers stay real.
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(NOW);
  });

  async function render(run: PuzzleRun, id = 'a', setup: (h: Host) => void = () => undefined) {
    api = new FakePuzzlesClient();
    api.playerRunResult = run;
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: PuzzlesClient, useValue: api },
        { provide: SceneChecks, useValue: fakeChecks },
      ],
    });
    const fixture = TestBed.createComponent(Host);
    host = fixture.componentInstance;
    host.id = id;
    setup(host);
    host.state = new PuzzleSessionState(
      asClient(api),
      () => 'camp-1',
      () => false,
    );
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
    return { el: fixture.nativeElement as HTMLElement, settle, fixture };
  }

  afterEach(() => document.body.replaceChildren());

  const lights = (partial: Parameters<typeof playerRun>[1] = {}) =>
    playerRun(lightsPuzzle('a', 'O selo da Capela'), {
      clue: 'Só o selo apagado abre o caminho.',
      hints: ['A luz do selo responde ao toque.'],
      state: { kind: { case: 'lights', value: { lit: lit([8, 11, 12]) } } },
      ...partial,
    });

  it('opens with the title, the session, the live status and the way back to the session', async () => {
    const { el } = await render(lights());
    expect(el.querySelector('h1')?.textContent).toBe('O selo da Capela');
    expect(textOf(el)).toContain('Quebra-cabeça da Sessão 7');
    expect(el.querySelector('app-live-pill')?.textContent).toContain('Ao vivo');
    const back = el.querySelector('a.back')!;
    expect(back.textContent).toContain('Voltar para a sessão');
    expect(back.getAttribute('href')).toBe('/campaigns/camp-1/session');
  });

  it("reads the master's clue as a quote, the hints released, and how many lights are lit", async () => {
    const { el } = await render(lights());
    expect(textOf(el.querySelector('.clue'))).toContain(
      'A pista do mestre “Só o selo apagado abre o caminho.”',
    );
    expect(textOf(el.querySelector('.hints'))).toContain(
      'Dicas soltas pelo mestre 1 A luz do selo responde ao toque.',
    );
    expect(textOf(el.querySelector('.info'))).toContain(
      'Toque numa luz: ela e as quatro vizinhas trocam. Apague todas.',
    );
    expect(textOf(el.querySelector('.info'))).toContain('Luzes acesas: 3');
  });

  it('plays: a tap on a light goes to the server with a key, and the board follows what the server answers', async () => {
    const { el, settle } = await render(lights());
    api.moveResult = () => ({
      run: lights({
        revision: 2,
        state: { kind: { case: 'lights', value: { lit: lit([0, 1, 5]) } } },
        lastMove: {
          characterName: 'Toren',
          move: { kind: { case: 'lights', value: { row: 0, col: 0 } } },
          changed: [0, 1, 5],
          at: at(0),
        },
      }),
      replayed: false,
      solvedByThisMove: false,
      wrong: false,
    });
    (el.querySelector('[aria-label="Luz na linha 1, coluna 1, apagada"]') as HTMLElement).click();
    await settle();
    const call = api.calls.find((c) => c[0] === 'move')!;
    expect(call[2]).toBe('a');
    expect(call[3]).toEqual({ kind: { case: 'lights', value: { row: 0, col: 0 } } });
    expect(typeof call[4]).toBe('string');
    expect(el.querySelector('[aria-label="Luz na linha 1, coluna 1, acesa"]')).not.toBeNull();
    expect(textOf(el.querySelector('.info'))).toContain('Luzes acesas: 3');
    // Your own move is not outlined as "just changed by someone else", but a screen reader hears it.
    expect(el.querySelectorAll('.cell--changed')).toHaveLength(0);
    expect(el.querySelector('[role="status"][aria-live="polite"]')?.textContent).toContain(
      'Você tocou numa luz. 3 acesas.',
    );
  });

  it('outlines what another person just changed, and says who and when', async () => {
    const { el } = await render(
      lights({
        lastMove: {
          characterName: 'Lia',
          move: { kind: { case: 'lights', value: { row: 1, col: 1 } } },
          changed: [6, 1, 5, 7, 11],
          at: at(1),
        },
      }),
    );
    expect(el.querySelectorAll('.cell--changed')).toHaveLength(5);
    expect(textOf(el.querySelector('.info__last'))).toContain('Lia tocou numa luz agora há pouco.');
  });

  it('reads the run again when the session page says this puzzle changed', async () => {
    const { el, settle } = await render(lights());
    api.playerRunResult = lights({
      revision: 5,
      hints: ['A luz do selo responde ao toque.', 'Cada toque troca cinco luzes de uma vez.'],
    });
    await host.state.changed('a');
    await settle();
    expect(el.querySelectorAll('.hints__list li')).toHaveLength(2);
  });

  it('is solved: "Resolvido", the board stops, and the master\'s own words with the time', async () => {
    const { el } = await render(
      lights({
        solved: true,
        solvedAt: at(0),
        solvedByName: 'Brisa',
        solvedMessage: 'A porta da Capela se abriu.',
        state: { kind: { case: 'lights', value: { lit: lit([]) } } },
      }),
    );
    expect(el.querySelector('.mr-notice--success')?.textContent).toContain('Resolvido');
    expect(textOf(el.querySelector('.info'))).toContain(
      'O quebra-cabeça terminou. Todas as luzes estão apagadas.',
    );
    expect(textOf(el.querySelector('.info__solved'))).toContain('A porta da Capela se abriu.');
    const cell = el.querySelector('button.cell') as HTMLButtonElement;
    expect(cell.getAttribute('aria-disabled')).toBe('true');
    cell.click();
    expect(api.calls.some((c) => c[0] === 'move')).toBe(false);
    // The page never says what the master chose to do (RN-10).
    expect(textOf(el)).not.toMatch(/Ao resolver|solução|mínimo/);
  });

  it('is stopped by a limit: the neutral line, and nothing moves', async () => {
    const { el } = await render(
      lights({
        stopped: true,
        stoppedMessage:
          'O quebra-cabeça parou. Ninguém joga mais até o mestre recomeçar ou fechar.',
      }),
    );
    expect(el.querySelector('.mr-notice--neutral')?.textContent).toContain(
      'O quebra-cabeça parou.',
    );
    (el.querySelector('button.cell') as HTMLElement).click();
    expect(api.calls.some((c) => c[0] === 'move')).toBe(false);
  });

  it('says a refusal in words and keeps the board', async () => {
    const { el, settle } = await render(lights());
    api.moveResult = () => {
      throw new ConnectError('x', Code.PermissionDenied);
    };
    (el.querySelector('button.cell') as HTMLElement).click();
    await settle();
    expect(el.querySelector('[role="alert"]')?.textContent).toContain(
      'Você não pode fazer isso agora.',
    );
    expect(el.querySelectorAll('button.cell')).toHaveLength(25);
  });

  it('says the master closed it, and leaves the way back', async () => {
    const { el, settle } = await render(lights());
    api.failWith = new ConnectError('x', Code.NotFound);
    await host.state.changed('a');
    await settle();
    expect(el.querySelector('.mr-notice--neutral')?.textContent).toContain(
      'O mestre fechou o quebra-cabeça.',
    );
    expect(el.querySelector('a.back')).not.toBeNull();
  });

  it('says there is nothing to open when the puzzle was never shown', async () => {
    api = new FakePuzzlesClient();
    api.failWith = new ConnectError('x', Code.NotFound);
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: PuzzlesClient, useValue: api },
        { provide: SceneChecks, useValue: fakeChecks },
      ],
    });
    const fixture = TestBed.createComponent(Host);
    fixture.componentInstance.state = new PuzzleSessionState(
      asClient(api),
      () => 'camp-1',
      () => false,
    );
    fixture.detectChanges();
    for (let i = 0; i < 3; i++) {
      await fixture.whenStable();
      fixture.detectChanges();
    }
    expect((fixture.nativeElement as HTMLElement).textContent).toContain(
      'O mestre não está mostrando este quebra-cabeça.',
    );
  });

  it('says "Reconectando…" instead of "Ao vivo" while the stream is down', async () => {
    const { el, settle } = await render(lights());
    host.reconnecting.set(true);
    await settle();
    expect(el.querySelector('.head__status')?.textContent).toContain('Reconectando…');
    expect(el.querySelector('app-live-pill')).toBeNull();
  });

  describe('the lock', () => {
    const lock = () =>
      playerRun(lockPuzzle('a', 'O cofre do Refeitório'), {
        clue: 'O fogo nasce antes da lua, e a raiz vê tudo.',
        state: { kind: { case: 'lock', value: { wheels: [0, 0, 2, 5] } } },
        lastMove: {
          characterName: 'Toren',
          move: { kind: { case: 'lock', value: { wheel: 2, delta: 1 } } },
          changed: [2],
          at: at(40),
        },
      });

    it('reads the wheels in words, never the solution, and turns a wheel with an arrow', async () => {
      const { el, settle } = await render(lock());
      expect(textOf(el.querySelector('.info'))).toContain(
        'As rodas agora: Lua · Lua · Onda · Estrela',
      );
      expect(textOf(el.querySelector('.info'))).toContain('Você girou a 3ª roda');
      expect(textOf(el.querySelector('.info'))).toContain(
        'Quando todas as rodas estiverem certas, o mestre é avisado.',
      );
      expect(el.querySelector('[aria-label="Roda 3: Onda"]')).not.toBeNull();
      api.moveResult = () => ({
        run: lock(),
        replayed: false,
        solvedByThisMove: false,
        wrong: false,
      });
      (el.querySelector('[aria-label="Próximo símbolo: Roda 2"]') as HTMLElement).click();
      await settle();
      expect(api.calls.find((c) => c[0] === 'move')![3]).toEqual({
        kind: { case: 'lock', value: { wheel: 1, delta: 1 } },
      });
      expect(el.querySelector('[aria-label="Solução da fechadura"]')).toBeNull();
    });
  });

  describe('the pillars', () => {
    const pillars = () =>
      playerRun(pillarsPuzzle('a', 'Os pilares da Galeria'), {
        clue: 'Os pilares obedecem ao mural.',
        mural: { pillars: [1, 0, 3, 2] },
        state: { kind: { case: 'pillars', value: { pillars: [3, 0, 2, 1] } } },
        lastMove: {
          characterName: 'Lia',
          move: { kind: { case: 'pillars', value: { pillar: 0, delta: 1 } } },
          changed: [0, 1],
          at: at(12),
        },
      });

    it('shows the mural to copy, the rule of the links, and who turned what', async () => {
      const { el, settle } = await render(pillars());
      expect(textOf(el.querySelector('.mural'))).toContain('O mural');
      expect(textOf(el.querySelector('.mural'))).toContain('Lobo Corvo Coruja Serpente');
      expect(textOf(el.querySelector('.mural'))).toContain('Deixe os pilares como o mural.');
      expect(textOf(el.querySelector('.info'))).toContain(
        'Girar um pilar gira também o da esquerda e o da direita.',
      );
      expect(textOf(el.querySelector('.info__last'))).toContain('Lia girou o pilar 1');
      expect(textOf(el.querySelector('.info__last'))).toContain('Os pilares 1 e 2 mudaram.');
      api.moveResult = () => ({
        run: pillars(),
        replayed: false,
        solvedByThisMove: false,
        wrong: false,
      });
      (el.querySelector('[aria-label="Girar o pilar 4"]') as HTMLElement).click();
      await settle();
      expect(api.calls.find((c) => c[0] === 'move')![3]).toEqual({
        kind: { case: 'pillars', value: { pillar: 3, delta: 1 } },
      });
    });
  });

  it('draws a 7 × 7 board with all 49 lights', async () => {
    const puzzle = lightsPuzzle('a', 'Os candelabros da cripta', {
      config: { kind: { case: 'lights', value: { size: 7 } } },
    });
    const { el } = await render(
      playerRun(puzzle, {
        state: {
          kind: {
            case: 'lights',
            value: { lit: Array.from({ length: 49 }, (_, i) => i % 3 === 0) },
          },
        },
      }),
    );
    expect(el.querySelectorAll('button.cell')).toHaveLength(49);
    expect(el.querySelector('[role="group"]')?.getAttribute('aria-label')).toBe(
      'Painel de luzes, 7 por 7',
    );
  });

  const typeInto = (field: Element | null, value: string) => {
    (field as HTMLInputElement).value = value;
    field!.dispatchEvent(new Event('input'));
  };
  const submit = (el: HTMLElement) => el.querySelector('form')!.dispatchEvent(new Event('submit'));
  const wrongMove = (name: string, extra: object = {}) =>
    ({ characterName: name, wrong: true, changed: [], at: at(0), ...extra }) as never;

  describe('the riddle (E10-12 state 6)', () => {
    const riddle = (partial: Parameters<typeof playerRun>[1] = {}) =>
      playerRun(riddlePuzzle('a', 'A porta da Cripta pergunta'), {
        clue: 'Procure no chão da Cripta.',
        ...partial,
      });
    const counted = {
      attemptsPerPlayer: 3,
      attemptsLeft: 3,
      maxMoves: 0,
      movesMade: 0,
      timeLimitSeconds: 0,
      secondsLeft: 0,
    };

    it('reads the riddle and the clue, and sends an answer; a wrong one says "Não é isso." and the attempts left', async () => {
      const { el, settle } = await render(riddle({ limits: counted }));
      expect(textOf(el.querySelector('.board-card'))).toContain('Moro embaixo de cada passo seu');
      expect(textOf(el.querySelector('.clue'))).toContain('Procure no chão da Cripta.');
      expect(textOf(el.querySelector('app-limit-counters'))).toContain('Suas tentativas 3 de 3');
      api.moveResult = () => ({
        run: riddle({
          revision: 2,
          limits: { ...counted, attemptsLeft: 2 },
          lastMove: wrongMove('Toren'),
        }),
        replayed: false,
        solvedByThisMove: false,
        wrong: true,
      });
      typeInto(el.querySelector('input[name="answer"]'), 'escuridão');
      submit(el);
      await settle();
      expect(api.calls.find((c) => c[0] === 'move')![3]).toEqual({
        kind: { case: 'riddle', value: { answer: 'escuridão' } },
      });
      expect(el.querySelector('.mr-notice--danger')?.textContent).toContain(
        'Não é isso. Tente outra resposta.',
      );
      expect(textOf(el.querySelector('app-limit-counters'))).toContain('Suas tentativas 2 de 3');
      // The typed answer stays in the field, to be changed.
      expect((el.querySelector('input[name="answer"]') as HTMLInputElement).value).toBe(
        'escuridão',
      );
    });

    it("does not call another player's wrong answer its own", async () => {
      const { el, settle } = await render(riddle());
      api.moveResult = () => ({
        run: riddle({ revision: 2, lastMove: wrongMove('Lia') }),
        replayed: false,
        solvedByThisMove: false,
        wrong: false,
      });
      typeInto(el.querySelector('input[name="answer"]'), 'x');
      submit(el);
      await settle();
      expect(el.querySelector('.board-card .mr-notice--danger')).toBeNull();
    });

    it('is solved: the field goes, who solved it and when, and "o mestre decide"; the answer is never in the page', async () => {
      const { el } = await render(riddle({ solved: true, solvedByName: 'Brisa', solvedAt: at(0) }));
      expect(el.querySelector('input[name="answer"]')).toBeNull();
      expect(el.querySelector('.mr-notice--success')?.textContent).toContain(
        'Resolvido. Brisa resolveu às',
      );
      expect(textOf(el.querySelector('.info'))).toContain(
        'O quebra-cabeça terminou. O enigma foi respondido.',
      );
      expect(textOf(el.querySelector('.info'))).toContain('O mestre decide o que acontece agora.');
      expect(textOf(el)).not.toMatch(/sombra/);
    });

    it('has no attempts left: a dashed box with the reason, and the counters still there', async () => {
      const { el } = await render(riddle({ limits: { ...counted, attemptsLeft: 0 } }));
      expect(el.querySelector('input[name="answer"]')).toBeNull();
      expect(el.querySelector('.blocked')?.textContent).toContain('Você não tem mais tentativas.');
      expect(textOf(el.querySelector('app-limit-counters'))).toContain(
        'Suas tentativas 0 de 3 · acabou',
      );
    });

    it('is stopped by a limit: the neutral line, the counters, and no field', async () => {
      const { el } = await render(
        riddle({
          stopped: true,
          stoppedMessage:
            'O quebra-cabeça parou. Ninguém joga mais até o mestre recomeçar ou fechar.',
          limits: {
            attemptsPerPlayer: 0,
            attemptsLeft: 0,
            maxMoves: 10,
            movesMade: 10,
            timeLimitSeconds: 300,
            secondsLeft: 12,
            deadline: fromNow(12),
          },
        }),
      );
      expect(el.querySelector('.mr-notice--neutral')?.textContent).toContain(
        'O quebra-cabeça parou.',
      );
      expect(el.querySelector('input[name="answer"]')).toBeNull();
      expect(textOf(el.querySelector('app-limit-counters'))).toContain('Jogadas 10 de 10 · acabou');
      expect(textOf(el.querySelector('app-limit-counters'))).toMatch(/Tempo 0:1[0-2] de 5:00/);
    });

    it('says the trap that fired, by the name the master gave it, and that the master says what happens', async () => {
      const { el } = await render(
        riddle({ lastMove: wrongMove('Lia', { trapName: 'Dardos envenenados' }) }),
      );
      const notice = el.querySelector('.mr-notice--warning')!;
      expect(notice.textContent).toContain('A armadilha disparou: Dardos envenenados.');
      expect(notice.textContent).toContain('O mestre diz o que acontece com quem estava na sala.');
    });

    it('says nothing of a trap the server did not name (a player who does not see it)', async () => {
      const { el } = await render(riddle({ lastMove: wrongMove('Lia') }));
      expect(el.querySelector('.mr-notice--warning')).toBeNull();
    });

    it("lets the time run down on its own, from the server's deadline", async () => {
      const { el } = await render(
        riddle({
          limits: {
            attemptsPerPlayer: 0,
            attemptsLeft: 0,
            maxMoves: 0,
            movesMade: 0,
            timeLimitSeconds: 300,
            secondsLeft: 288,
            deadline: fromNow(288),
          },
        }),
      );
      expect(textOf(el.querySelector('app-limit-counters'))).toMatch(/Tempo 4:[45]\d de 5:00/);
    });
  });

  describe('the sequence (E10-12 state 7)', () => {
    const seq = (playback: object, partial: Parameters<typeof playerRun>[1] = {}) =>
      playerRun(sequencePuzzle('a', 'Os sinos do Salão do trono'), {
        sequence: {
          totalSteps: 6,
          plays: 1,
          playing: false,
          shown: [],
          stepMs: 1200,
          nextInMs: 0,
          ...playback,
        },
        ...partial,
      });

    it('watches it play: only the bells the server revealed, the step number, and no bell to tap', async () => {
      const { el } = await render(seq({ playing: true, shown: [0, 1, 3], nextInMs: 900 }));
      expect(textOf(el.querySelector('.board-card'))).toContain(
        'O mestre está tocando os sinos. Observe e escute: passo 3 de 6.',
      );
      expect(el.querySelectorAll('app-sequence-strip li:not(.slot--empty)')).toHaveLength(3);
      expect(el.querySelector('.big__name')?.textContent).toBe('Sino pequeno');
      expect(el.querySelectorAll('app-bells-board button')).toHaveLength(0);
      // Nothing about the steps to come: not in the page, not in the JSON the server sent.
      expect(textOf(el)).not.toContain('Passo 4: Sino');
    });

    it('repeats it: any bell is a move, and the right steps come from the server', async () => {
      const { el, settle } = await render(
        seq({ plays: 2 }, { state: { kind: { case: 'sequence', value: { progress: 2 } } } }),
      );
      expect(textOf(el.querySelector('.board-card'))).toContain('Agora é com vocês.');
      expect(textOf(el.querySelector('.board-card'))).toContain('Passos certos 2 de 6');
      api.moveResult = () => ({
        run: seq(
          { plays: 2 },
          { revision: 2, state: { kind: { case: 'sequence', value: { progress: 3 } } } },
        ),
        replayed: false,
        solvedByThisMove: false,
        wrong: false,
      });
      (el.querySelector('[aria-label="Sino alto"]') as HTMLElement).click();
      await settle();
      expect(api.calls.find((c) => c[0] === 'move')![3]).toEqual({
        kind: { case: 'sequence', value: { bell: 1 } },
      });
      expect(textOf(el.querySelector('.board-card'))).toContain('Passos certos 3 de 6');
    });

    it('says which step was wrong and who erred, until the master plays it again', async () => {
      const wrong = seq({ plays: 2 }, { lastMove: wrongMove('Lia', { step: 4 }) });
      const { el, settle } = await render(wrong);
      expect(el.querySelector('.board-card .mr-notice--danger')?.textContent).toContain(
        'Errou o passo 4. A tentativa recomeçou; Lia errou.',
      );
      // The master plays it again: the old note goes.
      api.playerRunResult = seq(
        { plays: 3 },
        { revision: 5, lastMove: wrongMove('Lia', { step: 4, at: wrong.lastMove!.at }) },
      );
      await host.state.changed('a');
      await settle();
      expect(el.querySelector('.board-card .mr-notice--danger')).toBeNull();
    });

    it('says "você errou" when the wrong bell was the player\'s own', async () => {
      const { el } = await render(seq({ plays: 2 }, { lastMove: wrongMove('Toren', { step: 2 }) }));
      expect(el.querySelector('.board-card .mr-notice--danger')?.textContent).toContain(
        'Errou o passo 2. A tentativa recomeçou; você errou.',
      );
    });

    it('says the master has not played it yet and offers no bell to tap', async () => {
      const { el } = await render(seq({ plays: 0 }));
      expect(textOf(el.querySelector('.board-card'))).toContain(
        'O mestre ainda não tocou os sinos.',
      );
    });

    it('is solved: the bells go and the page says so', async () => {
      const { el } = await render(
        seq({ plays: 2 }, { solved: true, solvedByName: 'Lia', solvedAt: at(0) }),
      );
      expect(el.querySelectorAll('app-bells-board button')).toHaveLength(0);
      expect(textOf(el.querySelector('.info'))).toContain('Os sinos tocaram na ordem certa.');
    });
  });

  describe('the cipher (E10-12 state 8)', () => {
    const cipher = (partial: Parameters<typeof playerRun>[1] = {}) =>
      playerRun(cipherPuzzle('a', 'A carta do Capitão'), partial);

    it('draws the letter and the decoding table, and only "Conferir" talks to the server', async () => {
      const { el, settle } = await render(
        cipher({
          limits: {
            attemptsPerPlayer: 0,
            attemptsLeft: 0,
            maxMoves: 10,
            movesMade: 7,
            timeLimitSeconds: 0,
            secondsLeft: 0,
          },
        }),
      );
      expect(el.querySelector('.cipher')?.textContent).toBe('R WHVRXUR HVWD VRE R DOWDU');
      expect(el.querySelectorAll('.col__bet')).toHaveLength(9);
      expect(textOf(el.querySelector('app-limit-counters'))).toContain('Jogadas 7 de 10');
      typeInto(el.querySelector('.col__bet'), 'a');
      await settle();
      expect(api.calls.some((c) => c[0] === 'move')).toBe(false);
      api.moveResult = () => ({
        run: cipher({ revision: 2, lastMove: wrongMove('Toren') }),
        replayed: false,
        solvedByThisMove: false,
        wrong: true,
      });
      typeInto(el.querySelector('textarea'), 'o tesouro esta sobre o altar');
      submit(el);
      await settle();
      expect(api.calls.find((c) => c[0] === 'move')![3]).toEqual({
        kind: { case: 'cipher', value: { text: 'o tesouro esta sobre o altar' } },
      });
      expect(el.querySelector('.mr-notice--danger')?.textContent).toContain(
        'Não é isso. Confira as letras da tabela.',
      );
    });

    it('says the key has not been found yet', async () => {
      const lost = await render(cipher({ hasKeyClue: true }));
      expect(textOf(lost.el.querySelector('.key'))).toContain(
        'A chave da cifra Ainda não acharam a chave.',
      );
    });

    it('sends the player to their notes once the key was found', async () => {
      const found = await render(cipher({ hasKeyClue: true, keyClueId: 'k1' }));
      expect(textOf(found.el.querySelector('.key'))).toContain('Pista achada na aventura');
      expect(textOf(found.el.querySelector('.key'))).toContain('Ela está nas suas anotações.');
      (
        Array.from(found.el.querySelectorAll('.key button')).find((b) =>
          b.textContent?.includes('Abrir as anotações'),
        ) as HTMLElement
      ).click();
      expect(host.notes).toBe(1);
    });

    it('has no key panel when the master linked no clue', async () => {
      const { el } = await render(cipher());
      expect(el.querySelector('.key')).toBeNull();
    });
  });

  describe('a hint by a skill check (E10-12 states 9 and 9b)', () => {
    const base = (partial: Parameters<typeof playerRun>[1] = {}) =>
      playerRun(riddlePuzzle('a', 'A porta da Cripta pergunta'), {
        hintByCheck: true,
        hintSkillKey: 'skill:investigation',
        canTryHint: true,
        ...partial,
      });

    it('names the skill on the button and never a DC', async () => {
      const { el } = await render(base());
      const go = Array.from(el.querySelectorAll('button')).find((b) =>
        b.textContent?.includes('Tentar uma dica'),
      )!;
      expect(go.textContent?.trim()).toContain('Tentar uma dica · Investigação');
      expect(textOf(el)).not.toMatch(/\bCD\b/);
      expect(textOf(el.querySelector('.hints'))).toContain(
        'A rolagem é no app. Com dado físico, você digita o resultado.',
      );
    });

    it("rolls in the app, and a pass puts the hint in the list as only the player's own", async () => {
      const { el, settle } = await render(base({ hints: [], sharedHints: 0 }));
      api.hintResult = () =>
        hintAnswer(
          base({
            revision: 2,
            hints: ['Pense no que acompanha você ao meio-dia.'],
            sharedHints: 0,
          }),
          true,
          17,
        );
      (
        Array.from(el.querySelectorAll('button')).find((b) =>
          b.textContent?.includes('Tentar uma dica'),
        ) as HTMLElement
      ).click();
      await settle();
      expect(api.calls.find((c) => c[0] === 'tryHint')![3]).toEqual({ inApp: true });
      expect(el.querySelector('.hints .mr-notice--success')?.textContent).toContain(
        'Você conseguiu. Esta dica é só sua; se quiser, conte aos outros.',
      );
      expect(textOf(el.querySelector('.hints__list'))).toContain(
        'Pense no que acompanha você ao meio-dia. Esta dica é só sua.',
      );
    });

    it('says a fail the neutral way, with no number to beat, and offers no second try for the same hint', async () => {
      const { el, settle } = await render(base());
      api.hintResult = () => hintAnswer(base({ revision: 2, canTryHint: false }), false, 9);
      (
        Array.from(el.querySelectorAll('button')).find((b) =>
          b.textContent?.includes('Tentar uma dica'),
        ) as HTMLElement
      ).click();
      await settle();
      const notice = el.querySelector('.hints .mr-notice--neutral')!;
      expect(notice.textContent).toContain(
        'Não deu desta vez. Outro jogador pode tentar, ou o mestre solta uma dica.',
      );
      expect(textOf(notice)).not.toMatch(/\bCD\b/);
      expect(
        Array.from(el.querySelectorAll('.hints button')).some((b) =>
          b.textContent?.includes('Tentar uma dica'),
        ),
      ).toBe(false);
    });

    it('types the d20 of a real die when the table rolls physical dice, without the bonus', async () => {
      const { el, settle } = await render(base(), 'a', (h) => (h.diceMode = 3));
      expect(textOf(el.querySelector('.hints'))).toContain(
        'A mesa usa dados físicos: você digita o resultado.',
      );
      (
        Array.from(el.querySelectorAll('button')).find((b) =>
          b.textContent?.includes('Tentar uma dica'),
        ) as HTMLElement
      ).click();
      await settle();
      expect(textOf(el.querySelector('.hints'))).toContain(
        'Teste de Investigação. Role o seu d20 e digite o dado, sem o bônus.',
      );
      api.hintResult = () => hintAnswer(base({ revision: 2, canTryHint: false }), true, 18);
      typeInto(el.querySelector('.hints input'), '15');
      await settle();
      (
        Array.from(el.querySelectorAll('.hints button')).find((b) =>
          b.textContent?.includes('Confirmar 15'),
        ) as HTMLElement
      ).click();
      await settle();
      expect(api.calls.find((c) => c[0] === 'tryHint')![3]).toEqual({ face: 15 });
    });

    it('offers "Digitar o resultado" as a link when the table lets the player choose and they prefer the app', async () => {
      const { el, settle } = await render(base());
      const link = Array.from(el.querySelectorAll('.hints button')).find((b) =>
        b.textContent?.includes('Digitar o resultado'),
      ) as HTMLElement;
      expect(link).toBeTruthy();
      link.click();
      await settle();
      expect(el.querySelector('.hints input')).not.toBeNull();
    });

    it('does not offer a try when the server did not say the player may', async () => {
      const { el } = await render(base({ canTryHint: false }));
      expect(
        Array.from(el.querySelectorAll('button')).some((b) =>
          b.textContent?.includes('Tentar uma dica'),
        ),
      ).toBe(false);
    });

    it('has no check at all when the master set none', async () => {
      const { el } = await render(riddlePuzzle('a', 'x') && playerRun(riddlePuzzle('a', 'x')));
      expect(el.querySelector('app-hint-try')).toBeNull();
    });
  });

  describe('the split information (E10-12 state 9)', () => {
    it("reads only the player's own part, labelled as theirs, and the names of who else has one", async () => {
      const { el } = await render(
        playerRun(riddlePuzzle('a', 'x'), {
          myPart: '…os tambores ecoam três vezes antes de a porta ceder.',
          partHolders: ['Brisa', 'Sálvia'],
        }),
      );
      const mine = el.querySelector('app-my-part .mine')!;
      expect(textOf(mine)).toContain(
        'A sua parte da pista “…os tambores ecoam três vezes antes de a porta ceder.” visibility_off Só você vê esta parte.',
      );
      const others = el.querySelector('app-my-part .others')!;
      expect(
        Array.from(others.querySelectorAll('.others__name')).map((n) => n.textContent),
      ).toEqual(['Brisa', 'Sálvia']);
      expect(textOf(others)).toContain(
        'Vocês precisam conversar para juntar as partes. O texto delas não aparece aqui.',
      );
    });

    it('shows nothing when there is no part and nobody has one', async () => {
      const { el } = await render(playerRun(riddlePuzzle('a', 'x')));
      expect(el.querySelector('app-my-part section')).toBeNull();
    });

    it('shows who has a part when the player has none of their own', async () => {
      const { el } = await render(playerRun(riddlePuzzle('a', 'x'), { partHolders: ['Brisa'] }));
      expect(el.querySelector('app-my-part .mine')).toBeNull();
      expect(el.querySelector('app-my-part .others')?.textContent).toContain('Brisa');
    });
  });

  describe('slice 10.15b, fix round 1', () => {
    const seqRun = (partial: Parameters<typeof playerRun>[1] = {}) =>
      playerRun(sequencePuzzle('a', 'Os sinos'), {
        sequence: { totalSteps: 6, plays: 1, playing: false, shown: [], stepMs: 1200, nextInMs: 0 },
        ...partial,
      });

    const riddleRun = (partial: Parameters<typeof playerRun>[1] = {}) =>
      playerRun(riddlePuzzle('a', 'A porta da Cripta pergunta'), partial);

    it('shows the taps still waiting, and the bells go one at a time', async () => {
      const { el, settle } = await render(seqRun());
      const release: (() => void)[] = [];
      api.moveResult = () =>
        new Promise((resolve) =>
          release.push(() =>
            resolve({
              run: seqRun({ revision: 2 }),
              replayed: false,
              solvedByThisMove: false,
              wrong: false,
            }),
          ),
        );
      (el.querySelector('[aria-label="Sino redondo"]') as HTMLElement).click();
      (el.querySelector('[aria-label="Sino alto"]') as HTMLElement).click();
      await settle();
      expect(api.calls.filter((c) => c[0] === 'move')).toHaveLength(1);
      expect(textOf(el.querySelector('.board-card'))).toContain('2 toques a enviar');
      release[0]();
      await settle();
      expect(api.calls.filter((c) => c[0] === 'move')).toHaveLength(2);
    });

    it('shows the bells dashed with the reason when the player has no attempts left', async () => {
      const { el } = await render(
        seqRun({
          limits: {
            attemptsPerPlayer: 2,
            attemptsLeft: 0,
            maxMoves: 0,
            movesMade: 0,
            timeLimitSeconds: 0,
            secondsLeft: 0,
          },
        }),
      );
      expect(el.querySelector('.board-card .blocked')?.textContent).toContain(
        'Você não tem mais tentativas.',
      );
      expect(el.querySelector('app-bells-board button')?.getAttribute('aria-disabled')).toBe(
        'true',
      );
      expect(textOf(el.querySelector('app-limit-counters'))).toContain(
        'Suas tentativas 0 de 2 · acabou',
      );
    });

    it('names the limit that stopped it, as the artboard does', async () => {
      const { el } = await render(
        riddleRun({
          stopped: true,
          stoppedMessage: 'O quebra-cabeça parou. O mestre decide o que acontece agora.',
          limits: {
            attemptsPerPlayer: 0,
            attemptsLeft: 0,
            maxMoves: 10,
            movesMade: 10,
            timeLimitSeconds: 300,
            secondsLeft: 100,
            deadline: fromNow(100),
          },
        }),
      );
      const notice = el.querySelector('.mr-notice--neutral')!;
      expect(notice.textContent).toContain('O limite de jogadas chegou: 10 de 10.');
      // The clock stands still where the server read it.
      expect(textOf(el.querySelector('app-limit-counters'))).toContain('Tempo 1:40 de 5:00');
      expect(textOf(el.querySelector('app-limit-counters'))).not.toContain(
        'Tempo 1:40 de 5:00 · acabou',
      );
    });

    it('says the time ran out when that stopped it', async () => {
      const { el } = await render(
        riddleRun({
          stopped: true,
          stoppedMessage: 'O quebra-cabeça parou.',
          limits: {
            attemptsPerPlayer: 0,
            attemptsLeft: 0,
            maxMoves: 0,
            movesMade: 0,
            timeLimitSeconds: 300,
            secondsLeft: 0,
            deadline: fromNow(-5),
          },
        }),
      );
      expect(el.querySelector('.mr-notice--neutral')?.textContent).toContain('O tempo acabou.');
    });

    it('tells the player their own total on a failed try, and never the DC', async () => {
      const { el, settle } = await render(
        playerRun(riddlePuzzle('a', 'A porta'), {
          hintByCheck: true,
          hintSkillKey: 'skill:investigation',
          canTryHint: true,
        }),
      );
      api.hintResult = () =>
        hintAnswer(
          playerRun(riddlePuzzle('a', 'A porta'), {
            revision: 2,
            hintByCheck: true,
            hintSkillKey: 'skill:investigation',
            canTryHint: false,
          }),
          false,
          9,
        );
      (
        Array.from(el.querySelectorAll('button')).find((b) =>
          b.textContent?.includes('Tentar uma dica'),
        ) as HTMLElement
      ).click();
      await settle();
      const notice = el.querySelector('.hints .mr-notice--neutral')!;
      expect(textOf(notice)).toContain(
        'Você tirou 9. Não deu desta vez. Outro jogador pode tentar',
      );
      expect(textOf(notice)).not.toMatch(/\bCD\b/);
    });

    it('asks the session to read the dice mode again when the server says the table rolls the other way', async () => {
      const { el, settle } = await render(
        playerRun(riddlePuzzle('a', 'A porta'), {
          hintByCheck: true,
          hintSkillKey: 'skill:investigation',
          canTryHint: true,
        }),
      );
      api.hintResult = () => {
        throw new ConnectError('x', Code.FailedPrecondition, undefined, [
          {
            desc: PuzzleBlockedSchema,
            value: create(PuzzleBlockedSchema, { reason: PuzzleBlockedReason.WRONG_DICE_MODE }),
          },
        ]);
      };
      (
        Array.from(el.querySelectorAll('button')).find((b) =>
          b.textContent?.includes('Tentar uma dica'),
        ) as HTMLElement
      ).click();
      await settle();
      expect(host.stale).toBe(1);
    });

    it('does not say the typed d20 twice', async () => {
      const { el, settle } = await render(
        playerRun(riddlePuzzle('a', 'A porta'), {
          hintByCheck: true,
          hintSkillKey: 'skill:investigation',
          canTryHint: true,
        }),
        'a',
        (h) => (h.diceMode = 3),
      );
      (
        Array.from(el.querySelectorAll('button')).find((b) =>
          b.textContent?.includes('Tentar uma dica'),
        ) as HTMLElement
      ).click();
      await settle();
      typeInto(el.querySelector('.hints input'), '15');
      await settle();
      expect(el.querySelector('.hints .type__sum')?.classList).toContain('mr-visually-hidden');
    });
  });
});
