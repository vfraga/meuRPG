import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  computed,
  effect,
  inject,
  input,
  output,
  signal,
  untracked,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { RouterLink } from '@angular/router';
import { timestampDate } from '@bufbuild/protobuf/wkt';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { PuzzleKind } from '../../../../../gen/meurpg/play/v1/puzzles_pb';
import { SceneChecks } from '../../../../core/maps/scene-actions';
import { PuzzlePlay } from '../../../../core/puzzles/puzzle-play';
import { PuzzleSessionState } from '../../../../core/puzzles/puzzle-session';
import {
  clockOf,
  kindName,
  lastMoveParts,
  limitRows,
  litCount,
  litWords,
  pillarsChangedText,
  agoText,
  trapFired,
} from '../../../../core/puzzles/puzzle-format';
import { PuzzlesClient } from '../../../../core/puzzles/puzzles-client';
import { LivePill } from '../../../../shared/live-pill/live-pill';
import { LimitCounters } from '../../../../shared/puzzle-boards/limit-counters';
import { PillarsBoard } from '../../../../shared/puzzle-boards/pillars-board';
import { PuzzleHost } from '../../../../shared/puzzle-boards/puzzle-host';
import { sessionSince } from '../../../../shared/session-time/session-time';
import type { LiveSessionVm } from '../../live-session.types';
import { HintTryControl } from '../hint-try/hint-try';
import { MyPart } from '../my-part/my-part';

/** How long the dashed frame of "just changed" stays on what another person moved. */
const CHANGED_MS = 6000;

/**
 * A shown puzzle, played by a player (MR-038, RN-27, RN-10; E10-06 states 6 to 9): the master's clue, the board, what is
 * going on (who moved last, how many lights are lit), the hints the master released, and, once solved, "Resolvido" with the
 * master's own words. It sits in the session page's place (the session page keeps the one stream and says when the puzzle
 * changed; this page reads the run again then), and "Voltar para a sessão" is a link back.
 *
 * - **The board follows the server.** Every move goes with its own idempotency key and retries with the same one (`PuzzlePlay`).
 * - **Nothing is computed here.** The lights, the wheels and the pillars are what the server sent; the win is the server's; the
 *   page never reads or guesses a solution (it never receives one).
 * - A puzzle the master closed says so and leaves the way back; one solved or stopped is frozen (the board is `aria-disabled`).
 * - 7 × 7 keeps 48 px lights at 390 px, and at 320 px the board goes edge to edge with 44 px lights (`app-lights-board`).
 */
@Component({
  selector: 'app-puzzle-play',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [
    HintTryControl,
    LimitCounters,
    LivePill,
    MatButtonModule,
    MatIconModule,
    MatProgressSpinnerModule,
    MyPart,
    PillarsBoard,
    PuzzleHost,
    RouterLink,
  ],
  templateUrl: './puzzle-play.html',
  styleUrl: './puzzle-play.scss',
})
export class PuzzlePlayPage {
  private readonly api = inject(PuzzlesClient);
  private readonly destroyRef = inject(DestroyRef);
  private readonly checks = inject(SceneChecks);

  readonly campaignId = input.required<string>();
  readonly puzzleId = input.required<string>();
  readonly session = input.required<LiveSessionVm>();
  /** The session page's own list state: its `tick` says when this puzzle changed. */
  readonly state = input.required<PuzzleSessionState>();
  /** The player's character, so "Você tocou numa luz". */
  readonly ownName = input('');
  readonly reconnecting = input(false);
  /** How the table rolls its dice (RN-18) and the player's own choice: the try for a hint rolls the same way. */
  readonly diceMode = input<DiceMode>(DiceMode.PLAYERS_CHOOSE);
  readonly dicePreference = input<DicePreference>(DicePreference.APP);
  /** The server refused a try for a hint because the table rolls the other way: the session reads the campaign's dice mode again. */
  readonly diceModeStale = output<void>();
  /** "Abrir as anotações": the cipher's key, once found, is a clue in the player's notes. */
  readonly openNotes = output<void>();

  protected readonly Kind = PuzzleKind;
  /** The puzzle's moves and run; `open` starts it over for another puzzle (going from `?puzzle=A` to `B` loads B). */
  protected readonly play = new PuzzlePlay(this.api, () => this.campaignId());
  /** The last answer was wrong (the riddle and the cipher): "Não é isso." until the next one. */
  protected readonly verdict = signal<'' | 'wrong'>('');
  /** The skills' names, to write "Investigação" from "skill:investigation". */
  private readonly skillNames = signal<ReadonlyMap<string, string>>(new Map());
  /** The plays of the sequence when the wrong bell on screen was struck: a new play ends that note. */
  private readonly playsAtWrong = signal<{ at: number; plays: number } | null>(null);
  protected readonly now = signal(new Date());

  protected readonly run = computed(() => this.play.run());
  protected readonly lights = computed(() => litCount(this.run()?.state));
  protected readonly last = computed(() => lastMoveParts(this.run()?.lastMove, this.ownName()));
  protected readonly lastAgo = computed(() => {
    const at = this.run()?.lastMove?.at;
    return at ? agoText(timestampDate(at), this.now()) : '';
  });
  /** What another person just changed, for a few seconds; your own moves are not outlined. */
  protected readonly changed = computed<readonly number[]>(() => {
    const last = this.run()?.lastMove;
    if (!last?.at || (this.ownName() !== '' && last.characterName === this.ownName())) {
      return [];
    }
    return this.now().getTime() - timestampDate(last.at).getTime() < CHANGED_MS ? last.changed : [];
  });
  protected readonly frozen = computed(() => !!this.run()?.solved || !!this.run()?.stopped);
  protected readonly since = computed(() => sessionSince(this.session().startedAt));
  protected readonly kind = computed(() => this.run()?.kind ?? PuzzleKind.UNSPECIFIED);
  protected readonly kindWord = computed(() => kindName(this.kind()));
  protected readonly solvedAt = computed(() => {
    const at = this.run()?.solvedAt;
    return at ? clockOf(timestampDate(at)) : '';
  });
  /** The words of a lock's wheels now, "Lua · Lua · Onda · Estrela". */
  protected readonly wheelWords = computed(() => {
    const run = this.run();
    const state = run?.state?.kind;
    return state?.case === 'lock'
      ? state.value.wheels.map((w) => run?.symbols[w]?.namePt ?? '').join(' · ')
      : '';
  });
  protected readonly faceList = computed(() =>
    (this.run()?.symbols ?? []).map((s) => ({ key: s.key, namePt: s.namePt })),
  );
  protected readonly instruction = computed(() => {
    switch (this.kind()) {
      case PuzzleKind.LIGHTS:
        return 'Toque numa luz: ela e as quatro vizinhas trocam. Apague todas.';
      case PuzzleKind.LOCK:
        return 'Gire as rodas até a combinação certa.';
      case PuzzleKind.CIPHER:
        return 'Decifre a carta e digite a mensagem. Qualquer um pode tentar.';
      case PuzzleKind.RIDDLE:
      case PuzzleKind.SEQUENCE:
        return '';
      default:
        return 'Gire os pilares até ficarem como o mural.';
    }
  });
  /** What the page says once it is solved, by kind. */
  protected readonly doneText = computed(() => {
    switch (this.kind()) {
      case PuzzleKind.LIGHTS:
        return 'O quebra-cabeça terminou. Todas as luzes estão apagadas.';
      case PuzzleKind.LOCK:
        return 'O quebra-cabeça terminou. A fechadura abriu.';
      case PuzzleKind.RIDDLE:
        return 'O quebra-cabeça terminou. O enigma foi respondido.';
      case PuzzleKind.SEQUENCE:
        return 'O quebra-cabeça terminou. Os sinos tocaram na ordem certa.';
      case PuzzleKind.CIPHER:
        return 'O quebra-cabeça terminou. A mensagem foi decifrada.';
      default:
        return 'O quebra-cabeça terminou. Os pilares combinam com o mural.';
    }
  });
  protected readonly mural = computed(() => this.run()?.mural?.pillars ?? []);
  protected readonly linked = computed(() => {
    const config = this.run()?.config?.kind;
    return config?.case === 'pillars' && config.value.links.some((l) => l.alsoTurns.length > 0);
  });
  protected readonly changedPillars = computed(() =>
    this.kind() === PuzzleKind.PILLARS
      ? pillarsChangedText(this.run()?.lastMove?.changed ?? [])
      : '',
  );
  /** The kinds that judge a typed answer or a bell: they stop being a board when solved or stopped, and say why nothing can be typed. */
  protected readonly judged = computed(
    () =>
      this.kind() === PuzzleKind.RIDDLE ||
      this.kind() === PuzzleKind.SEQUENCE ||
      this.kind() === PuzzleKind.CIPHER,
  );
  protected readonly hostMode = computed<'play' | 'view'>(() =>
    this.judged() && (this.frozen() || this.play.gone()) ? 'view' : 'play',
  );
  /** The counters of "Ao errar": attempts, moves, time (the time runs down by the clock, from the server's deadline). */
  protected readonly counters = computed(() => {
    const run = this.run();
    return run ? limitRows(run, this.now()) : [];
  });
  protected readonly outOfAttempts = computed(() =>
    this.counters().some((c) => c.key === 'attempts' && c.spent),
  );
  /** Why nothing can be typed now, for the riddle and the cipher. */
  protected readonly blocked = computed(() =>
    this.outOfAttempts() && !this.frozen() ? 'Você não tem mais tentativas.' : '',
  );
  /** The trap that the last wrong move fired ("Dardos envenenados"); only for a player who sees it (the server sends nothing else). */
  protected readonly trap = computed(() => trapFired(this.run()?.lastMove));
  /** "Errou o passo 4. A tentativa recomeçou..." for the sequence, until the master plays it again. */
  protected readonly sequenceNote = computed(() => {
    const last = this.run()?.lastMove;
    if (this.kind() !== PuzzleKind.SEQUENCE || !last?.wrong || this.frozen()) {
      return null;
    }
    const at = last.at ? timestampDate(last.at).getTime() : 0;
    const seen = this.playsAtWrong();
    if (seen && seen.at === at && (this.run()?.sequence?.plays ?? 0) > seen.plays) {
      return null;
    }
    const own = this.ownName() !== '' && last.characterName === this.ownName();
    return {
      lead: `Errou o passo ${last.step}.`,
      text: `A tentativa recomeçou; ${own ? 'você errou' : `${last.characterName} errou`}.`,
    };
  });
  /** Whether the counters stand in the "Como está" panel: the riddle and the cipher draw them beside their button. */
  protected readonly countersBelow = computed(
    () =>
      this.counters().length > 0 &&
      !(
        this.hostMode() === 'play' &&
        (this.kind() === PuzzleKind.RIDDLE || this.kind() === PuzzleKind.CIPHER)
      ),
  );
  /** The "Como está" panel has something to say: never an empty card (the riddle and the sequence say it all in their board). */
  protected readonly showInfo = computed(() => {
    const kind = this.kind();
    const own =
      kind === PuzzleKind.LIGHTS || kind === PuzzleKind.LOCK || kind === PuzzleKind.PILLARS;
    return (
      !!this.run()?.solved ||
      own ||
      this.instruction() !== '' ||
      !!this.last() ||
      this.countersBelow()
    );
  });
  /** The stopped notice names the limit that was spent, as the artboard's state 10 does ("O limite de jogadas chegou: 10 de 10."). */
  protected readonly stoppedWhy = computed(() => {
    const spent = this.counters().filter((c) => c.spent);
    if (!this.run()?.stopped || spent.length === 0) {
      return '';
    }
    const parts = spent.flatMap((c) =>
      c.key === 'moves'
        ? [`O limite de jogadas chegou: ${c.value}.`]
        : c.key === 'time'
          ? ['O tempo acabou.']
          : [],
    );
    return parts.join(' ');
  });
  /** "Investigação" in "Tentar uma dica · Investigação". */
  protected readonly hintSkill = computed(() => {
    const key = this.run()?.hintSkillKey ?? '';
    return this.skillNames().get(key) ?? 'perícia';
  });
  /** The solved line of the three kinds of 10.15b: who solved it and when (a typed answer is never sent to the others). */
  protected readonly solvedBy = computed(() => {
    const run = this.run();
    if (!run?.solved || !this.judged()) {
      return '';
    }
    return `${run.solvedByName || 'Alguém'} resolveu${this.solvedAt() ? ` às ${this.solvedAt()}` : ''}.`;
  });
  /** The first hint a player won alone (10.15b): everything before it is shared. */
  protected readonly sharedHints = computed(() => this.run()?.sharedHints ?? 0);
  /** What a screen reader hears after a move: who moved and, for the lights, how many are lit. */
  protected readonly announce = computed(() => {
    const last = this.last();
    if (!last) {
      return '';
    }
    const base = `${last.who} ${last.what}.`;
    return this.kind() === PuzzleKind.LIGHTS ? `${base} ${litWords(this.lights())}.` : base;
  });

  constructor() {
    // Another puzzle in the address: open it, and forget the one before (a late answer of that one is ignored).
    effect(() => {
      const id = this.puzzleId();
      untracked(() => void this.play.open(id));
    });
    // The session page says "this puzzle changed" (the stream hint, or a reconnection): read the run again. The version is a counter
    // for this puzzle, so two hints in one turn are both seen.
    let seen = 0;
    effect(() => {
      const version = this.state().versionOf(this.puzzleId());
      untracked(() => {
        if (version > seen) {
          void this.play.refresh();
        }
        seen = version;
      });
    });
    // Another puzzle in the address: the old "Não é isso." goes.
    effect(() => {
      this.puzzleId();
      untracked(() => this.verdict.set(''));
    });
    // A wrong bell stays on screen until the master plays the sequence again: the plays when it was first seen are the mark.
    effect(() => {
      const run = this.run();
      const last = run?.lastMove;
      untracked(() => {
        if (last?.wrong && last.at) {
          const at = timestampDate(last.at).getTime();
          if (this.playsAtWrong()?.at !== at) {
            this.playsAtWrong.set({ at, plays: run?.sequence?.plays ?? 0 });
          }
        }
      });
    });
    // The skills' names (once per campaign), for the hint button.
    effect(() => {
      const campaign = this.campaignId();
      void untracked(() =>
        this.checks.skills(campaign).then(
          (skills) => this.skillNames.set(new Map(skills.map((s) => [s.key, s.label]))),
          () => undefined,
        ),
      );
    });
    // The table's dice mode changed under the page: the way to roll on screen is the campaign's, read again.
    let staleSeen = 0;
    effect(() => {
      const n = this.play.diceModeStale();
      untracked(() => {
        if (n > staleSeen) {
          this.diceModeStale.emit();
        }
        staleSeen = n;
      });
    });
    const timer = setInterval(() => this.now.set(new Date()), 1000);
    this.destroyRef.onDestroy(() => {
      clearInterval(timer);
      this.play.dispose();
    });
  }

  protected async onMove(move: Parameters<PuzzlePlay['move']>[0]): Promise<void> {
    const kind = move.kind?.case;
    const verdict = await this.play.move(move);
    if (kind === 'riddle' || kind === 'cipher') {
      this.verdict.set(verdict.wrong ? 'wrong' : '');
    }
  }
}
