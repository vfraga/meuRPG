import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  inject,
  input,
  output,
  signal,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { timestampDate } from '@bufbuild/protobuf/wkt';

import {
  type MasterPuzzleRun,
  PuzzleBlockedReason,
  PuzzleKind,
  PuzzleRunStatus,
  PuzzleSolveAction,
  PuzzleStopReason,
} from '../../../../../gen/meurpg/play/v1/puzzles_pb';
import { focusWithRing } from '../../../../core/creatures/focus-ring';
import { joinDots } from '../../../../core/format/text';
import {
  clockOf,
  kindIcon,
  kindName,
  lastMoveParts,
  limitRows,
  litCount,
  litWords,
  moveWord,
  agoText,
  outcomeText,
  trapFired,
} from '../../../../core/puzzles/puzzle-format';
import {
  isTransient,
  puzzleBlocked,
  puzzleErrorMessage,
} from '../../../../core/puzzles/puzzle-errors';
import { PuzzlesClient } from '../../../../core/puzzles/puzzles-client';
import { LockBoard } from '../../../../shared/puzzle-boards/lock-board';
import { PillarsBoard } from '../../../../shared/puzzle-boards/pillars-board';
import { PuzzleHost } from '../../../../shared/puzzle-boards/puzzle-host';
import { SequenceStrip } from '../../../../shared/puzzle-boards/sequence-strip';
import { LimitCounters } from '../../../../shared/puzzle-boards/limit-counters';
import { SecretPill } from '../../../../shared/puzzle-boards/secret-pill';
import { MapAsk } from '../../../maps/map-ask/map-ask';
import { DoorCrop } from '../door-crop/door-crop';

type Ask = 'reset' | 'close';
type Action = 'hint' | 'reseed' | 'reset' | 'close' | 'play';

/**
 * The master's live view of one shown puzzle (MR-038, E10-06 states 3 to 5): the same board the players have, static, in the same
 * order for every kind: what it is, whether the players see it, the state, the last move and by whom, what only the master
 * knows (always with "Só você vê") and the buttons.
 *
 * - **What the master adds** is read from the server's answer: the fewest moves left (`minimum`, and from the start), the lock's
 *   solution and the pillars' shortest turns, and the lights a shortest way presses (garnet frames: "Toque que resolve"). "Esconder
 *   os toques" and "Mostrar a solução só para mim" are only this screen's; nothing is computed here.
 * - **"Recomeçar" and "Fechar" ask in place** (E10-06 state 5): the question takes the buttons' place, the focus goes to its
 *   title, "Voltar" brings the buttons back and the focus to the one that asked; nothing happens before the second tap.
 * - **Solved** (state 4): who solved it and when, what the server did ("Uma porta se abriu.") and the door on its map.
 * - **The riddle, the sequence and the cipher** (10.15b, E10-12 state 5) add what only the master knows, each with "Só você vê": the accepted
 *   answers, the sequence's steps, the plain message. He also reads the answer typed or the bell struck in the last move, each player's
 *   attempts left, the counters of the limits, the trap that fired, and starts a play of the sequence ("Tocar a sequência").
 * - It reports each answer to the session page's list (`updated`); the stream then keeps it current.
 */
@Component({
  selector: 'app-master-run',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [
    DoorCrop,
    LimitCounters,
    LockBoard,
    MapAsk,
    MatButtonModule,
    MatIconModule,
    PillarsBoard,
    PuzzleHost,
    SecretPill,
    SequenceStrip,
  ],
  templateUrl: './master-run.html',
  styleUrl: './master-run.scss',
})
export class MasterRun {
  private readonly api = inject(PuzzlesClient);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  readonly campaignId = input.required<string>();
  readonly run = input.required<MasterPuzzleRun>();
  /** The clock the "há 12 s" counts from; the panel ticks it. */
  readonly now = input.required<Date>();
  /** The answer of an action: the puzzle as it stands. */
  readonly updated = output<MasterPuzzleRun>();

  protected readonly Kind = PuzzleKind;
  protected readonly ask = signal<Ask | null>(null);
  protected readonly busy = signal<Action | null>(null);
  protected readonly notice = signal('');
  /** "Esconder os toques" and "Mostrar a solução só para mim": this screen's alone. */
  protected readonly ringsHidden = signal(false);
  protected readonly solutionShown = signal(false);

  protected readonly puzzle = computed(() => this.run().puzzle);
  protected readonly player = computed(() => this.run().run);
  protected readonly kind = computed(() => this.puzzle()?.kind ?? PuzzleKind.UNSPECIFIED);
  protected readonly name = computed(() => this.puzzle()?.name ?? '');
  protected readonly kindWord = computed(() => kindName(this.kind()));
  protected readonly icon = computed(() => kindIcon(this.kind()));
  protected readonly solved = computed(() => this.run().status === PuzzleRunStatus.SOLVED);
  protected readonly closed = computed(() => this.run().status === PuzzleRunStatus.CLOSED);
  protected readonly stopped = computed(() => !!this.player()?.stopped);
  protected readonly frozen = computed(() => this.solved() || this.stopped());

  protected readonly status = computed(() => {
    if (this.solved()) {
      return { icon: 'check', word: 'Resolvido', tone: 'success' };
    }
    if (this.stopped()) {
      return { icon: 'pause_circle', word: 'Parou', tone: 'pending' };
    }
    if (this.closed()) {
      return { icon: 'visibility_off', word: 'Fechado', tone: '' };
    }
    return { icon: 'visibility', word: 'Os jogadores veem', tone: 'success' };
  });

  protected readonly solvedLine = computed(() => {
    const p = this.player();
    const at = p?.solvedAt ? ` às ${clockOf(timestampDate(p.solvedAt))}` : '';
    return `${p?.solvedByName || 'Alguém'} resolveu “${this.name()}”${at}.`;
  });
  protected readonly stoppedLine = computed(() =>
    this.run().stopReason === PuzzleStopReason.TIME
      ? 'O tempo acabou: ninguém joga mais até você recomeçar ou fechar.'
      : 'O limite de jogadas foi atingido: ninguém joga mais até você recomeçar ou fechar.',
  );
  protected readonly outcome = computed(() => outcomeText(this.run().outcome));
  protected readonly doorTarget = computed(() => {
    const on = this.puzzle()?.onSolve;
    return this.solved() && on?.action === PuzzleSolveAction.OPEN_DOOR && on.target.case === 'door'
      ? on.target.value
      : null;
  });
  /** "Ao resolver" in words, for the meta line. */
  protected readonly onSolveWords = computed(() => {
    switch (this.puzzle()?.onSolve?.action) {
      case PuzzleSolveAction.OPEN_DOOR:
        return 'abrir uma porta';
      case PuzzleSolveAction.REVEAL_POINT:
        return 'revelar um ponto do mapa';
      case PuzzleSolveAction.REVEAL_CLUE:
        return 'revelar uma pista';
      default:
        return 'só avisar você';
    }
  });

  // What the players read, from the same message they get.
  protected readonly changed = computed<readonly number[]>(() => {
    const last = this.player()?.lastMove;
    if (!last?.at) {
      return [];
    }
    return this.now().getTime() - timestampDate(last.at).getTime() < 6000 ? last.changed : [];
  });
  protected readonly lit = computed(() => litCount(this.player()?.state));
  protected readonly litText = computed(() => litWords(this.lit()));
  protected readonly last = computed(() => lastMoveParts(this.run().lastMove));
  protected readonly lastAgo = computed(() => {
    const at = this.run().lastMove?.at;
    return at ? agoText(timestampDate(at), this.now()) : '';
  });
  protected readonly moves = computed(() => this.run().movesMade);
  protected readonly movesWord = computed(() => moveWord(this.kind(), this.moves()));

  // What only the master knows.
  protected readonly minimum = computed(() => this.run().minimum);
  protected readonly fromStart = computed(() => this.run().minimumFromStart?.moves ?? 0);
  protected readonly minWord = computed(() => moveWord(this.kind(), this.minimum()?.moves ?? 0));
  /** The squares a shortest way presses: the garnet frames. */
  protected readonly rings = computed<readonly number[]>(() => {
    const size = (() => {
      const config = this.puzzle()?.config?.kind;
      return config?.case === 'lights' ? config.value.size : 0;
    })();
    if (
      this.kind() !== PuzzleKind.LIGHTS ||
      this.ringsHidden() ||
      this.frozen() ||
      this.closed() ||
      size === 0
    ) {
      return [];
    }
    return (this.minimum()?.path ?? []).flatMap((m) =>
      m.kind.case === 'lights' ? [m.kind.value.row * size + m.kind.value.col] : [],
    );
  });
  protected readonly solutionWheels = computed(() => {
    const solution = this.puzzle()?.solution?.kind;
    return solution?.case === 'lock' ? solution.value.wheels : [];
  });
  protected readonly faces = computed(() =>
    (this.puzzle()?.symbols ?? []).map((s) => ({ key: s.key, namePt: s.namePt })),
  );
  /** "Pilar 1 × 2 · Pilar 3 × 1": the shortest turns, as the server lists them. */
  protected readonly turns = computed(() => {
    const counts = new Map<number, number>();
    for (const m of this.minimum()?.path ?? []) {
      if (m.kind.case === 'pillars') {
        counts.set(m.kind.value.pillar, (counts.get(m.kind.value.pillar) ?? 0) + 1);
      }
    }
    return joinDots(
      [...counts]
        .sort((a, b) => a[0] - b[0])
        .map(([pillar, n]) => `Pilar\u00a0${pillar + 1}\u00a0×\u00a0${n}`),
    );
  });
  protected readonly muralPillars = computed(() => this.player()?.mural?.pillars ?? []);
  protected readonly hintCount = computed(() => this.puzzle()?.hints.length ?? 0);
  protected readonly released = computed(() => this.run().releasedHints);
  protected readonly hintsLeft = computed(() => this.hintCount() - this.released());
  protected readonly canReseed = computed(
    () => this.kind() === PuzzleKind.LIGHTS || this.kind() === PuzzleKind.PILLARS,
  );
  protected readonly clue = computed(() => this.puzzle()?.clue ?? '');

  /** "Pista: “…” · Dicas: 1 de 2 soltas · Ao resolver: abrir uma porta." with the dots tied to the word before them (a line never starts on "·"). */
  protected readonly meta = computed(() => {
    const hints =
      this.hintCount() > 0
        ? [
            `Dicas: ${this.released()} de ${this.hintCount()} ${this.hintCount() === 1 ? 'solta' : 'soltas'}`,
          ]
        : [];
    return `${joinDots([...(this.clue() ? [`Pista: “${this.clue()}”`] : []), ...hints, `Ao resolver: ${this.onSolveWords()}`])}.`;
  });

  /** The tries for a hint by a skill check, newest last: who rolled, what, and whether it passed (the last three). */
  protected readonly diceLines = computed(() =>
    this.run()
      .hintTries.slice(-3)
      .map((t) => {
        const face = t.roll?.faces[0];
        const total = t.roll?.total ?? 0;
        const how = face !== undefined && face !== total ? `${total} (d20: ${face})` : `${total}`;
        return `${t.characterName} rolou ${how}${t.roll?.physical ? ' (dado físico)' : ''} para a dica ${t.hint}: ${t.passed ? 'passou' : 'não passou'}.`;
      }),
  );

  // The riddle, the sequence and the cipher: what only the master knows.
  protected readonly answers = computed(() => {
    const solution = this.puzzle()?.solution?.kind;
    return solution?.case === 'riddle' ? solution.value.answers : [];
  });
  protected readonly sequenceSteps = computed(() => {
    const solution = this.puzzle()?.solution?.kind;
    return solution?.case === 'sequence' ? solution.value.steps : [];
  });
  protected readonly plays = computed(() => this.player()?.sequence?.plays ?? 0);
  protected readonly playing = computed(() => !!this.player()?.sequence?.playing);
  protected readonly cipherMessage = computed(() => {
    const solution = this.puzzle()?.solution?.kind;
    return solution?.case === 'cipher' ? solution.value.message : '';
  });
  /** "Toren: 2 de 3": the player of the last move, with their attempts left (a puzzle with attempts only). */
  protected readonly lastAttempts = computed(() => {
    const who = this.run().lastMove?.characterName;
    const row = this.run().attempts.find((a) => a.characterName === who);
    const per = this.puzzle()?.onWrong?.attemptsPerPlayer ?? 0;
    return row && per > 0 ? `Tentativas de ${who}: ${row.left} de ${per}.` : '';
  });
  /** Every player's attempts, "Brisa 3 de 3 · Pensantus 3 de 3". */
  protected readonly attemptsLine = computed(() => {
    const per = this.puzzle()?.onWrong?.attemptsPerPlayer ?? 0;
    return per > 0
      ? joinDots(
          this.run().attempts.map((a) => `${a.characterName}\u00a0${a.left}\u00a0de\u00a0${per}`),
        )
      : '';
  });
  /** The counters of the limits: moves and time (the attempts are said player by player). */
  protected readonly counters = computed(() => {
    const r = this.player();
    return r ? limitRows(r, this.now()).filter((row) => row.key !== 'attempts') : [];
  });
  /** The moves counter says it already ("Jogadas 7 de 10"): the plain count under it would say the same twice. */
  protected readonly movesCounted = computed(() => this.counters().some((c) => c.key === 'moves'));
  protected readonly trap = computed(() => trapFired(this.run().lastMove));
  protected readonly trapWho = computed(() => this.run().lastMove?.characterName ?? '');
  protected readonly judged = computed(
    () =>
      this.kind() === PuzzleKind.RIDDLE ||
      this.kind() === PuzzleKind.SEQUENCE ||
      this.kind() === PuzzleKind.CIPHER,
  );

  protected readonly id = computed(() => `mr-${this.puzzle()?.id ?? ''}`);

  protected askFor(what: Ask): void {
    this.notice.set('');
    this.ask.set(what);
  }

  protected back(what: Ask): void {
    this.ask.set(null);
    afterNextRender(
      () =>
        focusWithRing(this.host.nativeElement.querySelector<HTMLElement>(`[data-act="${what}"]`)),
      { injector: this.injector },
    );
  }

  /**
   * An action whose answer was lost may have worked: the card shows the puzzle as the server has it, so a
   * second tap is not a second hint, or another reset, nobody saw coming.
   */
  private async readAgain(id: string): Promise<boolean> {
    try {
      this.updated.emit(await this.api.masterRun(this.campaignId(), id));
      return true;
    } catch {
      // Still no answer: the notice says the action did not go through, and the card stays as it was.
      return false;
    }
  }

  protected async act(action: Action): Promise<void> {
    const id = this.puzzle()?.id;
    if (!id || this.busy()) {
      return;
    }
    this.busy.set(action);
    this.notice.set('');
    try {
      const campaign = this.campaignId();
      // The revision of the run this card shows: a run that moved since (a player, another tab) refuses the action.
      const seen = this.player()?.revision ?? 0;
      const next =
        action === 'hint'
          ? await this.api.releaseHint(campaign, id, seen)
          : action === 'play'
            ? await this.api.playSequence(campaign, id, seen)
            : action === 'reseed'
              ? await this.api.reseed(campaign, id, seen)
              : action === 'reset'
                ? await this.api.reset(campaign, id, seen)
                : await this.api.close(campaign, id);
      this.ask.set(null);
      if (action === 'reset' || action === 'reseed') {
        this.solutionShown.set(false);
      }
      this.updated.emit(next);
      if (action === 'reset' || action === 'close') {
        afterNextRender(
          () =>
            focusWithRing(
              this.host.nativeElement.querySelector<HTMLElement>(`[data-act="${action}"]`),
            ),
          { injector: this.injector },
        );
      }
    } catch (err) {
      this.ask.set(null);
      const what =
        action === 'hint'
          ? 'soltar a dica'
          : action === 'play'
            ? 'tocar a sequência'
            : action === 'close'
              ? 'fechar o quebra-cabeça'
              : 'recomeçar o quebra-cabeça';
      // A run that moved since the card was drawn: show it as it is now; the master chooses again, nothing is retried.
      const stale = puzzleBlocked(err)?.reason === PuzzleBlockedReason.STALE_REVISION;
      if (stale) {
        await this.readAgain(id);
      }
      this.notice.set(
        !stale && isTransient(err) && (await this.readAgain(id))
          ? `O servidor não respondeu a tempo ao pedido de ${what}. O quebra-cabeça abaixo está como ele ficou: confira antes de tentar de novo.`
          : puzzleErrorMessage(err, what),
      );
    } finally {
      this.busy.set(null);
    }
  }
}
