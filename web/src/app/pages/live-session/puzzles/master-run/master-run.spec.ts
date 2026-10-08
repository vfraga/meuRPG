import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  GetMapLayersResponseSchema,
  GetMapResponseSchema,
  MapSchema,
} from '../../../../../gen/meurpg/maps/v1/maps_pb';
import {
  type MasterPuzzleRun,
  PuzzleBlockedReason,
  PuzzleBlockedSchema,
  PuzzleRunStatus,
  PuzzleSolveAction,
  PuzzleSolveOutcome,
  PuzzleStopReason,
} from '../../../../../gen/meurpg/play/v1/puzzles_pb';
import { textOf } from '../../../../core/format/text-testing';
import { MapsClient } from '../../../../core/maps/maps-client';
import { PuzzlesClient } from '../../../../core/puzzles/puzzles-client';
import {
  NOW,
  FakePuzzlesClient,
  at,
  cipherPuzzle,
  lightsPuzzle,
  lockPuzzle,
  masterRun,
  pillarsPuzzle,
  playerRun,
  riddlePuzzle,
  sequencePuzzle,
} from '../../../../core/puzzles/puzzles-testing';
import { MasterRun } from './master-run';

@Component({
  imports: [MasterRun],
  template: `<app-master-run campaignId="camp-1" [run]="run()" [now]="now" (updated)="updates.push($event)" />`,
})
class Host {
  run = signal<MasterPuzzleRun>(masterRun(lightsPuzzle('a', 'x'), PuzzleRunStatus.SHOWN));
  now = NOW;
  updates: MasterPuzzleRun[] = [];
}

const lights = () =>
  lightsPuzzle('a', 'O selo da Capela', {
    hints: ['A luz do selo responde ao toque.', 'Cada toque troca cinco luzes de uma vez.'],
    onSolve: {
      action: PuzzleSolveAction.OPEN_DOOR,
      message: 'A porta da Capela se abriu.',
      target: { case: 'door', value: { mapId: 'm1', col: 1, row: 0 } },
    },
  });

/** The 5 x 5 board of the artboard: 12 lit, three touches left, one touch made by Lia 12 s ago. */
function liveLights(extra: Parameters<typeof masterRun>[2] = {}) {
  const puzzle = lights();
  const lit = Array.from({ length: 25 }, (_, i) =>
    [8, 11, 12, 13, 15, 16, 17, 20, 21, 22, 23, 24].includes(i),
  );
  const run = playerRun(puzzle, {
    state: { kind: { case: 'lights', value: { lit } } },
    lastMove: {
      characterName: 'Lia',
      move: { kind: { case: 'lights', value: { row: 0, col: 0 } } },
      changed: [0, 1, 5],
      at: at(12),
    },
  });
  return masterRun(puzzle, PuzzleRunStatus.SHOWN, {
    run,
    movesMade: 1,
    releasedHints: 1,
    minimum: {
      solvable: true,
      moves: 3,
      path: [
        { kind: { case: 'lights', value: { row: 2, col: 3 } } },
        { kind: { case: 'lights', value: { row: 3, col: 0 } } },
        { kind: { case: 'lights', value: { row: 4, col: 4 } } },
      ],
    },
    minimumFromStart: { solvable: true, moves: 4, path: [] },
    lastMove: run.lastMove,
    ...extra,
  });
}

describe('MasterRun (MR-038, E10-06 states 3 to 5)', () => {
  let api: FakePuzzlesClient;

  async function render(
    run: MasterPuzzleRun,
    prep: (a: FakePuzzlesClient) => void = () => undefined,
  ) {
    api = new FakePuzzlesClient();
    prep(api);
    const maps = {
      list: async () => [create(MapSchema, { id: 'm1', name: 'A capela' })],
      get: async () =>
        create(GetMapResponseSchema, {
          map: create(MapSchema, { id: 'm1', name: 'A capela', gridColumns: 2, gridRows: 2 }),
        }),
      layers: async () =>
        create(GetMapLayersResponseSchema, {
          gridColumns: 2,
          gridRows: 2,
          wall: new Uint8Array(1),
          difficultTerrain: new Uint8Array(1),
          cover: new Uint8Array(1),
        }),
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: PuzzlesClient, useValue: api },
        { provide: MapsClient, useValue: maps },
      ],
    });
    const fixture = TestBed.createComponent(Host);
    fixture.componentInstance.run.set(run);
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
    return {
      el: fixture.nativeElement as HTMLElement,
      settle,
      host: fixture.componentInstance,
      fixture,
    };
  }

  const button = (el: HTMLElement, text: string) =>
    Array.from(el.querySelectorAll('button')).find((b) =>
      b.textContent?.trim().includes(text),
    ) as HTMLButtonElement;

  afterEach(() => document.body.replaceChildren());

  describe('Apagar as luzes', () => {
    it('shows the board as the players have it, the last move and who made it, and what only the master knows', async () => {
      const { el } = await render(liveLights());
      expect(el.querySelector('h3')?.textContent).toBe('O selo da Capela');
      expect(textOf(el.querySelector('.rc__tags'))).toBe(
        'lightbulb Apagar as luzes visibility Os jogadores veem',
      );
      expect(el.querySelectorAll('app-lights-board [role="img"]')).toHaveLength(25);
      const text = textOf(el.querySelector('.facts'));
      expect(text).toContain('Última jogada: Lia tocou numa luz, há 12 s.');
      expect(text).toContain('12 acesas · 1 toque até agora');
      expect(text).toContain('Faltam, no mínimo, 3 toques (4 desde o começo)');
      expect(text).toContain('Dicas: 1 de 2 soltas · Ao resolver: abrir uma porta');
      // Only the master gets the rings, with their pill; Lia's move is older than 6 s, so nothing is dashed any more.
      expect(el.querySelectorAll('.cell--hint')).toHaveLength(3);
      expect(el.querySelectorAll('.cell--changed')).toHaveLength(0);
      expect(textOf(el.querySelector('.facts__legend'))).toBe(
        'Toque que resolve visibility_off Só você vê',
      );
    });

    it('lets him hide the rings (only this screen) and bring them back', async () => {
      const { el, settle } = await render(liveLights());
      // One label, "Mostrar os toques", pressed while the rings are on.
      expect(button(el, 'Mostrar os toques').getAttribute('aria-pressed')).toBe('true');
      button(el, 'Mostrar os toques').click();
      await settle();
      expect(el.querySelectorAll('.cell--hint')).toHaveLength(0);
      expect(el.querySelector('.facts__legend')).toBeNull();
      expect(button(el, 'Mostrar os toques').getAttribute('aria-pressed')).toBe('false');
      expect(api.calls).toEqual([]);
    });

    it('says when the board cannot be solved from here', async () => {
      const { el } = await render(liveLights({ minimum: { solvable: false, moves: 0, path: [] } }));
      expect(el.querySelector('.facts')?.textContent).toContain(
        'Desse jeito não dá para chegar lá: gere outro começo.',
      );
    });

    it('releases the next hint and passes the answer on; with none left the button says so', async () => {
      const next = liveLights({ releasedHints: 2 });
      const { el, settle, host } = await render(liveLights(), (a) => a.runResults.set('a', next));
      button(el, 'Mostrar a próxima dica').click();
      await settle();
      expect(api.calls[0]).toEqual(['releaseHint', 'camp-1', 'a', 1]);
      expect(host.updates).toEqual([next]);
      host.run.set(next);
      await settle();
      const done = button(el, 'Todas as dicas foram soltas');
      expect(done.classList).toContain('mr-button--off');
      done.click();
      expect(api.calls).toHaveLength(1);
    });

    it('has no hint button at all when the puzzle has no hints', async () => {
      const bare = liveLights();
      const { el } = await render(
        masterRun(lightsPuzzle('a', 'x'), PuzzleRunStatus.SHOWN, {
          run: bare.run,
          minimum: bare.minimum,
        }),
      );
      expect(button(el, 'Mostrar a próxima dica')).toBeUndefined();
    });

    it('draws another start with one tap (it is not a question) and passes the answer on', async () => {
      const { el, settle, host } = await render(liveLights(), (a) =>
        a.runResults.set('a', liveLights({ movesMade: 0 })),
      );
      button(el, 'Gerar outro começo').click();
      await settle();
      expect(api.calls[0]).toEqual(['reseed', 'camp-1', 'a', 1]);
      expect(host.updates).toHaveLength(1);
    });

    it('asks "Recomeçar" in place: the question takes the buttons, "Voltar" gives them back and the focus', async () => {
      const { el, settle } = await render(liveLights());
      button(el, 'Recomeçar').click();
      await settle();
      const ask = el.querySelector('app-map-ask')!;
      expect(ask.querySelector('h3')?.textContent).toBe('Recomeçar “O selo da Capela”?');
      expect(ask.textContent).toContain(
        'O painel volta ao mesmo começo que ele tinha, para todos os jogadores.',
      );
      expect(ask.textContent).toContain('Quem estiver jogando vê o começo de novo na hora.');
      expect(document.activeElement).toBe(ask.querySelector('h3'));
      expect(el.querySelector('.acts')).toBeNull();
      expect(api.calls).toEqual([]);
      button(ask as HTMLElement, 'Voltar').click();
      await settle();
      expect(el.querySelector('app-map-ask')).toBeNull();
      expect(document.activeElement).toBe(button(el, 'Recomeçar'));
      expect(api.calls).toEqual([]);
    });

    it('recomeça on the second tap, and "Fechar" asks its own question and closes', async () => {
      const { el, settle, host } = await render(liveLights(), (a) => {
        a.runResults.set('a', liveLights({ movesMade: 0 }));
      });
      button(el, 'Recomeçar').click();
      await settle();
      button(el.querySelector('app-map-ask') as HTMLElement, 'Recomeçar').click();
      await settle();
      expect(api.calls[0]).toEqual(['reset', 'camp-1', 'a', 1]);
      expect(host.updates).toHaveLength(1);
      expect(el.querySelector('app-map-ask')).toBeNull();
      button(el, 'Fechar').click();
      await settle();
      expect(el.querySelector('app-map-ask h3')?.textContent).toBe('Fechar “O selo da Capela”?');
      expect(el.querySelector('app-map-ask')?.textContent).toContain(
        'Se você mostrá-lo de novo, ele volta ao começo.',
      );
      button(el.querySelector('app-map-ask') as HTMLElement, 'Fechar').click();
      await settle();
      expect(api.calls[1]).toEqual(['close', 'camp-1', 'a']);
    });

    it('reads the puzzle again when the answer to a hint is lost, so a second tap is not a second hint', async () => {
      const done = liveLights({ releasedHints: 1 });
      const { el, settle, host } = await render(liveLights(), (a) => {
        a.runResults.set('a', done);
        a.releaseHint = () => Promise.reject(new ConnectError('x', Code.Unavailable));
      });
      button(el, 'Mostrar a próxima dica').click();
      await settle();
      expect(api.calls).toEqual([['masterRun', 'camp-1', 'a']]);
      expect(host.updates).toEqual([done]);
      expect(el.querySelector('[role="alert"]')?.textContent).toContain('confira antes de tentar');
    });

    it('keeps the plain refusal when the card cannot be read either', async () => {
      const { el, settle, host } = await render(liveLights(), (a) => {
        a.releaseHint = () => Promise.reject(new ConnectError('x', Code.Unavailable));
        a.masterRun = () => Promise.reject(new ConnectError('x', Code.Unavailable));
      });
      button(el, 'Mostrar a próxima dica').click();
      await settle();
      expect(host.updates).toEqual([]);
      expect(el.querySelector('[role="alert"]')?.textContent).toContain('o servidor não respondeu');
    });

    it('sends the revision of the run it shows on the hint, the new start, the restart and the sequence', async () => {
      const shown = (revision: number) =>
        liveLights({ run: { ...liveLights().run!, revision } as never });
      const { el, settle } = await render(shown(7), (a) => a.runResults.set('a', shown(8)));
      button(el, 'Mostrar a próxima dica').click();
      await settle();
      button(el, 'Gerar outro começo').click();
      await settle();
      button(el, 'Recomeçar').click();
      await settle();
      button(el, 'Recomeçar').click();
      await settle();
      expect(api.calls.map((c) => [c[0], c[3]])).toEqual([
        ['releaseHint', 7],
        ['reseed', 7],
        ['reset', 7],
      ]);
    });

    it('reads the run again and says it changed, without retrying, when the revision is stale', async () => {
      const now = liveLights({ releasedHints: 1 });
      const stale = new ConnectError('x', Code.FailedPrecondition, undefined, [
        {
          desc: PuzzleBlockedSchema,
          value: create(PuzzleBlockedSchema, { reason: PuzzleBlockedReason.STALE_REVISION }),
        },
      ]);
      const { el, settle, host } = await render(liveLights(), (a) => {
        a.runResults.set('a', now);
        a.releaseHint = vi.fn(() => Promise.reject(stale));
      });
      button(el, 'Mostrar a próxima dica').click();
      await settle();
      expect(api.calls).toEqual([['masterRun', 'camp-1', 'a']]);
      expect(host.updates).toEqual([now]);
      expect(el.querySelector('[role="alert"]')?.textContent).toContain(
        'mudou desde que você olhou',
      );
    });

    it('sends the new revision when the master chooses again after the re-read', async () => {
      const stale = new ConnectError('x', Code.FailedPrecondition, undefined, [
        {
          desc: PuzzleBlockedSchema,
          value: create(PuzzleBlockedSchema, { reason: PuzzleBlockedReason.STALE_REVISION }),
        },
      ]);
      const newer = liveLights({ run: { ...liveLights().run!, revision: 5 } as never });
      const { el, settle, host } = await render(liveLights(), (a) => {
        a.runResults.set('a', newer);
        a.releaseHint = vi.fn().mockRejectedValueOnce(stale).mockResolvedValue(newer) as never;
      });
      button(el, 'Mostrar a próxima dica').click();
      await settle();
      host.run.set(host.updates[0]);
      await settle();
      button(el, 'Mostrar a próxima dica').click();
      await settle();
      const calls = (api.releaseHint as unknown as ReturnType<typeof vi.fn>).mock.calls;
      expect(calls.map((c) => c[2])).toEqual([1, 5]);
    });

    it('says why an action was refused and leaves the card as it is', async () => {
      const { el, settle } = await render(liveLights(), (a) => {
        a.failWith = new ConnectError('x', Code.FailedPrecondition, undefined, [
          {
            desc: PuzzleBlockedSchema,
            value: create(PuzzleBlockedSchema, { reason: PuzzleBlockedReason.NO_MORE_HINTS }),
          },
        ]);
      });
      button(el, 'Mostrar a próxima dica').click();
      await settle();
      expect(el.querySelector('[role="alert"]')?.textContent).toContain(
        'Não há mais dicas para soltar.',
      );
    });

    it('is solved: who solved it, what the server did and the door on its map, with the buttons still there', async () => {
      const solved = liveLights({
        status: PuzzleRunStatus.SOLVED,
        outcome: PuzzleSolveOutcome.DOOR_OPENED,
        run: {
          ...liveLights().run!,
          solved: true,
          solvedByName: 'Brisa',
          solvedAt: at(60),
          solvedMessage: 'A porta da Capela se abriu.',
        },
      });
      const { el } = await render(solved);
      expect(el.querySelector('.mr-notice--success')?.textContent).toContain(
        'Brisa resolveu “O selo da Capela” às',
      );
      expect(textOf(el.querySelector('.rc__tags'))).toContain('check Resolvido');
      expect(textOf(el.querySelector('.outcome'))).toContain('Uma porta se abriu.');
      expect(textOf(el.querySelector('.outcome'))).toContain(
        'Os jogadores leram: “A porta da Capela se abriu.”',
      );
      // No dice line unless something was rolled.
      expect(el.textContent).not.toContain('rolou');
      expect(el.textContent).not.toContain('O app não rolou dados');
      expect(el.querySelector('app-door-crop')).not.toBeNull();
      // Frozen: no hint ring, no "Faltam" line, but "Recomeçar" and "Fechar" stay.
      expect(el.querySelectorAll('.cell--hint')).toHaveLength(0);
      expect(el.querySelector('.facts')?.textContent).not.toContain('Faltam');
      expect(button(el, 'Recomeçar')).toBeTruthy();
      expect(button(el, 'Fechar')).toBeTruthy();
    });

    it('has no "Mostrar a solução" once solved, and no "Dicas" line for a puzzle without hints', async () => {
      const done = masterRun(lockPuzzle('b', 'O cofre'), PuzzleRunStatus.SOLVED, {
        run: playerRun(lockPuzzle('b', 'O cofre'), { solved: true }),
      });
      const { el } = await render(done);
      expect(button(el, 'Mostrar a solução só para mim')).toBeUndefined();
      expect(textOf(el.querySelector('.facts'))).not.toContain('Dicas:');
    });

    it('says what was rolled when a hint was won by a skill check, and only then', async () => {
      const tried = liveLights({
        hintTries: [
          {
            characterName: 'Lia',
            hint: 2,
            passed: true,
            roll: { diceCount: 1, diceSides: 20, faces: [11], modifier: 3, total: 14 },
          },
        ],
      });
      const { el } = await render(tried);
      expect(textOf(el.querySelector('.facts'))).toContain(
        'Lia rolou 14 (d20: 11) para a dica 2: passou.',
      );
    });

    it('says when a limit stopped it, by the reason', async () => {
      const stopped = liveLights({
        stopReason: PuzzleStopReason.TIME,
        run: { ...liveLights().run!, stopped: true },
      });
      const { el } = await render(stopped);
      expect(el.querySelector('.mr-notice--warning')?.textContent).toContain('O tempo acabou');
      expect(textOf(el.querySelector('.rc__tags'))).toContain('Parou');
    });
  });

  describe('Fechadura de combinação', () => {
    const lock = () => {
      const puzzle = lockPuzzle('b', 'O cofre do Refeitório', {
        clue: 'O fogo nasce antes da lua, e a raiz vê tudo.',
        hints: ['A pista fala de três coisas da natureza.'],
      });
      const run = playerRun(puzzle, {
        lastMove: {
          characterName: 'Toren',
          move: { kind: { case: 'lock', value: { wheel: 2, delta: 1 } } },
          changed: [2],
          at: at(40),
        },
      });
      return masterRun(puzzle, PuzzleRunStatus.SHOWN, {
        run,
        lastMove: run.lastMove,
        releasedHints: 1,
        movesMade: 3,
      });
    };

    it('keeps the solution hidden until he asks, always with "Só você vê", and hides it again', async () => {
      const { el, settle } = await render(lock());
      expect(textOf(el.querySelector('.part'))).toContain('As rodas agora');
      expect(textOf(el.querySelector('.part'))).toContain('Lua Lua Onda Estrela');
      expect(el.textContent).not.toContain('A solução');
      button(el, 'Mostrar a solução só para mim').click();
      await settle();
      const solution = Array.from(el.querySelectorAll('.part')).at(-1)!;
      expect(textOf(solution)).toContain('A solução');
      expect(textOf(solution)).toContain('Só você vê');
      expect(textOf(solution)).toContain('Chama Lua Raiz Estrela');
      expect(button(el, 'Mostrar a solução só para mim').getAttribute('aria-pressed')).toBe('true');
      button(el, 'Mostrar a solução só para mim').click();
      await settle();
      expect(el.textContent).not.toContain('A solução');
      expect(api.calls).toEqual([]);
    });

    it('says the last move, the clue and the hints; has no "Gerar outro começo" (the start is his)', async () => {
      const { el } = await render(lock());
      const text = textOf(el.querySelector('.facts'));
      expect(text).toContain('Última jogada: Toren girou a 3ª roda, há 40 s.');
      expect(text).toContain('Pista: “O fogo nasce antes da lua, e a raiz vê tudo.”');
      expect(text).toContain('Dicas: 1 de 1 solta');
      expect(button(el, 'Gerar outro começo')).toBeUndefined();
    });
  });

  describe('Símbolos giratórios', () => {
    const pillars = () => {
      const puzzle = pillarsPuzzle('c', 'Os pilares da Galeria');
      const run = playerRun(puzzle, {
        mural: { pillars: [1, 0, 3, 2] },
        lastMove: {
          characterName: 'Lia',
          move: { kind: { case: 'pillars', value: { pillar: 0, delta: 1 } } },
          changed: [0, 1],
          at: at(12),
        },
      });
      return masterRun(puzzle, PuzzleRunStatus.SHOWN, {
        run,
        lastMove: run.lastMove,
        movesMade: 1,
        minimum: {
          solvable: true,
          moves: 7,
          path: [
            { kind: { case: 'pillars', value: { pillar: 0, delta: 1 } } },
            { kind: { case: 'pillars', value: { pillar: 0, delta: 1 } } },
            { kind: { case: 'pillars', value: { pillar: 2, delta: 1 } } },
          ],
        },
        minimumFromStart: { solvable: true, moves: 8, path: [] },
      });
    };

    it('shows the pillars now and the mural, the fewest turns left and the way, only when asked', async () => {
      const { el, settle } = await render(pillars());
      const parts = Array.from(el.querySelectorAll('.part')).map((p) => textOf(p));
      expect(parts[0]).toContain('Os pilares agora');
      expect(parts[1]).toContain('O mural');
      expect(parts[1]).toContain('Lobo Corvo Coruja Serpente');
      expect(textOf(el.querySelector('.facts'))).toContain(
        'Faltam, no mínimo, 7 giros (8 desde o começo)',
      );
      expect(el.textContent).not.toContain('Giros que resolvem');
      button(el, 'Mostrar a solução só para mim').click();
      await settle();
      expect(textOf(el.querySelector('.facts'))).toContain(
        'Giros que resolvem: Pilar 1 × 2 · Pilar 3 × 1 visibility_off Só você vê',
      );
      expect(button(el, 'Gerar outro começo')).toBeTruthy();
    });
  });

  describe('the tries for a hint by a skill check', () => {
    it('lists who rolled, what, and whether it passed: the last three, newest last', async () => {
      const roll = (face: number, total: number) => ({
        diceCount: 1,
        diceSides: 20,
        faces: [face],
        modifier: total - face,
        total,
      });
      const { el } = await render(
        liveLights({
          hintTries: [
            { characterName: 'Toren', hint: 1, passed: false, roll: roll(4, 7) },
            { characterName: 'Brisa', hint: 1, passed: true, roll: roll(16, 19) },
            { characterName: 'Lia', hint: 2, passed: false, roll: roll(2, 5) },
            { characterName: 'Sálvia', hint: 2, passed: true, roll: roll(18, 18) },
          ],
        }),
      );
      const lines = Array.from(el.querySelectorAll('.facts__line--quiet')).map((l) => textOf(l));
      expect(lines).toContain('Brisa rolou 19 (d20: 16) para a dica 1: passou.');
      expect(lines).toContain('Lia rolou 5 (d20: 2) para a dica 2: não passou.');
      expect(lines).toContain('Sálvia rolou 18 para a dica 2: passou.');
      expect(lines.join(' ')).not.toContain('Toren rolou');
      // The DC is the master's own: it is on his puzzle and never on this line.
      expect(el.textContent).not.toMatch(/\bCD\b/);
    });
  });

  describe('Enigma (E10-12 state 5)', () => {
    const riddle = (
      extra: Parameters<typeof masterRun>[2] = {},
      onWrong: object = { attemptsPerPlayer: 3 },
    ) => {
      const puzzle = riddlePuzzle('r', 'A porta da Cripta pergunta', { onWrong });
      const run = playerRun(puzzle, {
        limits: {
          attemptsPerPlayer: 3,
          attemptsLeft: 3,
          maxMoves: 0,
          movesMade: 0,
          timeLimitSeconds: 0,
          secondsLeft: 0,
        },
        lastMove: {
          characterName: 'Toren',
          move: { kind: { case: 'riddle', value: { answer: 'escuridão' } } },
          wrong: true,
          changed: [],
          at: at(8),
        },
      });
      return masterRun(puzzle, PuzzleRunStatus.SHOWN, {
        run,
        lastMove: run.lastMove,
        movesMade: 1,
        attempts: [
          { characterName: 'Toren', wrong: 1, left: 2 },
          { characterName: 'Brisa', wrong: 0, left: 3 },
          { characterName: 'Sálvia', wrong: 0, left: 3 },
        ],
        ...extra,
      });
    };

    it('shows the riddle, the accepted answers with "Só você vê", the answer typed and each one\'s attempts', async () => {
      const { el } = await render(riddle());
      expect(textOf(el.querySelector('.rc__tags'))).toContain('Enigma');
      expect(el.querySelector('.rc__body')?.textContent).toContain(
        'Moro embaixo de cada passo seu',
      );
      const answers = el.querySelector('.answers')!;
      expect(Array.from(answers.querySelectorAll('li')).map((l) => l.textContent)).toEqual([
        'sombra',
        'a sombra',
      ]);
      expect(textOf(answers.closest('.part'))).toContain(
        'Respostas aceitas visibility_off Só você vê',
      );
      expect(textOf(el.querySelector('.facts'))).toContain(
        'Última jogada: Toren tentou “escuridão”: errou, há 8 s. Tentativas de Toren: 2 de 3.',
      );
      expect(textOf(el.querySelector('.facts'))).toContain(
        'Toren 2 de 3 · Brisa 3 de 3 · Sálvia 3 de 3',
      );
      // The master does not play: no field, no "Responder".
      expect(el.querySelector('input, form')).toBeNull();
    });

    it('is solved: who solved it, and the buttons stay', async () => {
      const done = riddle({ status: PuzzleRunStatus.SOLVED });
      const { el } = await render({
        ...done,
        run: { ...done.run!, solved: true, solvedByName: 'Brisa', solvedAt: at(60) },
      } as MasterPuzzleRun);
      expect(el.querySelector('.mr-notice--success')?.textContent).toContain(
        'Brisa resolveu “A porta da Cripta pergunta” às',
      );
    });
  });

  describe('Sequência (E10-12 state 5)', () => {
    const sequence = (extra: Parameters<typeof masterRun>[2] = {}, runExtra: object = {}) => {
      const puzzle = sequencePuzzle('s', 'Os sinos do Salão do trono', {
        onWrong: { trap: { mapId: 'm1', pointId: 't1' } },
      });
      const run = playerRun(puzzle, {
        sequence: { totalSteps: 6, plays: 2, playing: false, shown: [], stepMs: 1200, nextInMs: 0 },
        lastMove: {
          characterName: 'Lia',
          move: { kind: { case: 'sequence', value: { bell: 2 } } },
          wrong: true,
          step: 4,
          changed: [],
          at: at(8),
          trapName: 'Dardos envenenados',
        },
        ...runExtra,
      });
      return masterRun(puzzle, PuzzleRunStatus.SHOWN, {
        run,
        lastMove: run.lastMove,
        movesMade: 5,
        ...extra,
      });
    };

    it('shows the whole sequence with "Só você vê", the step that was wrong, the plays and the trap that fired', async () => {
      const { el } = await render(sequence());
      const strip = Array.from(el.querySelectorAll('app-sequence-strip li')).map((i) =>
        i.getAttribute('aria-label'),
      );
      expect(strip).toEqual([
        'Passo 1: Sino redondo',
        'Passo 2: Sino alto',
        'Passo 3: Sino pequeno',
        'Passo 4: Sino redondo',
        'Passo 5: Sino largo',
        'Passo 6: Sino alto',
      ]);
      expect(textOf(el.querySelector('.part'))).toContain('A sequência visibility_off Só você vê');
      expect(textOf(el.querySelector('.facts'))).toContain(
        'Última jogada: Lia errou no passo 4. A tentativa recomeçou, há 8 s.',
      );
      expect(textOf(el.querySelector('.facts'))).toContain(
        'Os jogadores já viram a sequência tocar 2 vezes.',
      );
      expect(textOf(el.querySelector('.mr-notice--warning'))).toContain(
        'A armadilha disparou: Dardos envenenados. Foi o erro de Lia.',
      );
    });

    it('tells a second firing of the same trap from the first by the last move beside it', async () => {
      const { el, settle, host } = await render(sequence());
      const facts = () => textOf(el.querySelector('.facts'));
      const notice = () => textOf(el.querySelector('.mr-notice--warning'));
      expect(facts()).toContain('há 8 s');
      const again = sequence(
        {},
        {
          lastMove: {
            characterName: 'Lia',
            move: { kind: { case: 'sequence', value: { bell: 2 } } },
            wrong: true,
            step: 4,
            changed: [],
            at: at(2),
            trapName: 'Dardos envenenados',
          },
        },
      );
      const before = notice();
      host.run.set(again);
      await settle();
      expect(notice()).toBe(before);
      expect(facts()).toContain('agora há pouco');
      expect(facts()).not.toContain('há 8 s');
    });

    it('plays it for the players with "Tocar a sequência", and the answer is the puzzle as it stands', async () => {
      const played = sequence();
      const { el, settle, host } = await render(played, (a) =>
        a.runResults.set('s', {
          ...played,
          run: { ...played.run!, sequence: { ...played.run!.sequence!, plays: 3 } },
        } as MasterPuzzleRun),
      );
      button(el, 'Tocar a sequência').click();
      await settle();
      expect(api.calls.find((c) => c[0] === 'playSequence')).toEqual([
        'playSequence',
        'camp-1',
        's',
        1,
      ]);
      expect(host.updates[0].run?.sequence?.plays).toBe(3);
    });

    it('says when it has not been played', async () => {
      const never = await render(
        sequence(
          {},
          {
            sequence: {
              totalSteps: 6,
              plays: 0,
              playing: false,
              shown: [],
              stepMs: 1200,
              nextInMs: 0,
            },
            lastMove: undefined,
          },
        ),
      );
      expect(textOf(never.el.querySelector('.facts'))).toContain(
        'Os jogadores ainda não viram a sequência tocar',
      );
    });

    it('says when it is playing now', async () => {
      const now = await render(
        sequence(
          {},
          {
            sequence: {
              totalSteps: 6,
              plays: 1,
              playing: true,
              shown: [0, 1],
              stepMs: 1200,
              nextInMs: 800,
            },
          },
        ),
      );
      expect(textOf(now.el.querySelector('.facts'))).toContain(
        'Os jogadores estão vendo a sequência tocar agora.',
      );
    });

    it('cannot play a stopped or solved sequence: the button is off and says nothing happens', async () => {
      const stopped = sequence({ stopReason: PuzzleStopReason.MOVES }, { stopped: true });
      const { el } = await render(stopped);
      const play = button(el, 'Tocar a sequência');
      expect(play.getAttribute('aria-disabled') ?? String(play.disabled)).toMatch(/true/);
      play.click();
      expect(api.calls.some((c) => c[0] === 'playSequence')).toBe(false);
    });

    it('says the refusal in words when the server refuses to play it', async () => {
      const { el, settle } = await render(sequence(), (a) => {
        a.failWith = new ConnectError('x', Code.FailedPrecondition, undefined, [
          {
            desc: PuzzleBlockedSchema,
            value: create(PuzzleBlockedSchema, { reason: PuzzleBlockedReason.STOPPED }),
          },
        ]);
      });
      button(el, 'Tocar a sequência').click();
      await settle();
      expect(el.querySelector('[role="alert"]')?.textContent).toContain('O quebra-cabeça parou');
    });
  });

  describe('Cifra (E10-12 state 5)', () => {
    const cipher = () => {
      const puzzle = cipherPuzzle('c', 'A carta do Capitão', {
        onWrong: { maxMoves: 10, timeLimitSeconds: 300 },
      });
      const run = playerRun(puzzle, {
        limits: {
          attemptsPerPlayer: 0,
          attemptsLeft: 0,
          maxMoves: 10,
          movesMade: 7,
          timeLimitSeconds: 300,
          secondsLeft: 192,
          deadline: timestampAt(192),
        },
        lastMove: {
          characterName: 'Sálvia',
          move: { kind: { case: 'cipher', value: { text: 'o tesouro esta sobre o altar' } } },
          wrong: true,
          changed: [],
          at: at(5),
        },
      });
      return masterRun(puzzle, PuzzleRunStatus.SHOWN, {
        run,
        lastMove: run.lastMove,
        movesMade: 7,
      });
    };

    it('shows the letter as the players read it, the plain message with "Só você vê", the message typed, and the counters', async () => {
      const { el } = await render(cipher());
      expect(el.querySelector('.cipher')?.textContent).toBe('R WHVRXUR HVWD VRE R DOWDU');
      expect(textOf(el.querySelector('.part__label--plain'))).toContain(
        'A mensagem: O tesouro está sob o altar visibility_off Só você vê',
      );
      expect(textOf(el.querySelector('.facts'))).toContain(
        'Última jogada: Sálvia digitou “o tesouro esta sobre o altar”: errou, há 5 s.',
      );
      expect(textOf(el.querySelector('app-limit-counters'))).toContain('Jogadas 7 de 10');
      expect(textOf(el.querySelector('app-limit-counters'))).toContain('Tempo 3:12 de 5:00');
    });

    it('says a limit stopped it by its reason', async () => {
      const stopped = {
        ...cipher(),
        stopReason: PuzzleStopReason.MOVES,
        run: { ...cipher().run!, stopped: true },
      } as MasterPuzzleRun;
      const { el } = await render(stopped);
      expect(el.querySelector('.mr-notice--warning')?.textContent).toContain(
        'O limite de jogadas foi atingido',
      );
    });
  });
});

/** A timestamp `seconds` after the spec's clock, for a deadline. */
function timestampAt(seconds: number) {
  return at(-seconds);
}
