import { type Signal, type WritableSignal, computed, signal } from '@angular/core';

import type { HeldRollOffer } from '../../../gen/meurpg/play/v1/combat_pb';
import { HeldRollController, type HeldRollHost } from './held-roll-controller';
import { HELD_ROLL_ANSWERED_TEXT } from './held-roll-errors';
import { type HeldAnswer, asksReroll } from './held-roll-offer';
import { answeredText } from './held-roll-result';
import { askingText } from './held-text';

/** How long a question that closed (answered by the roller, by the master, or by someone else first) stays on screen, so it
 * is read and announced once, in milliseconds. */
export const NOTE_LINGER_MS = 6000;
/** What the master reads when a reroll rule's question is gone: his screen cannot tell the roller's answer from an undo or a
 * stale question, so the line says only that it no longer applies. */
export const REROLL_GONE_TEXT = 'Essa pergunta não vale mais.';
const TICK_MS = 1000;
const MS_PER_SECOND = 1000;
/** When the server refuses the master as too early although the local clock says it is time (the clocks differ), the
 * buttons wait this long before trying again, in milliseconds. */
const RETRY_MS = 2000;

/** Where a question of the master stands:
 * - `waiting`: the roller answers first (the first ten seconds);
 * - `ready`: the roller did not answer and the master may;
 * - `answering`: the master's answer is on the way;
 * - `too-early`: the server refused the master's answer; the countdown runs again;
 * - `done`, `closed`, `lost`: it ended (the master answered, the roller did, someone else answered first). */
export type QuestionPhase =
  'waiting' | 'ready' | 'answering' | 'too-early' | 'done' | 'closed' | 'lost';

/** What the questions need from the page that holds them. */
export interface HeldQuestionsDeps {
  /** Sends `AnswerHeldRoll` (a function: a page that never asks does not build the client). */
  readonly send: HeldRollHost['send'];
  /** The time in milliseconds; injectable for the specs. */
  readonly clock?: () => number;
  readonly newKey?: () => string;
}

interface QuestionContext {
  readonly now: Signal<number>;
  readonly clock: () => number;
  readonly send: HeldRollHost['send'];
  readonly newKey?: () => string;
  /** The question changed in a way that may start the clock. */
  readonly changed: () => void;
}

/**
 * One question the master reads about someone else's roll: the roller answers first and the master may answer for them
 * after `MASTER_WAIT_SECONDS`. It wraps a `HeldRollController` (with `master` set) and says where the question stands
 * (`phase`) and the words of its status line, from the clock the owner ticks.
 */
export class MasterQuestion {
  readonly holdId: string;
  readonly combatantId: string;
  readonly roller: string;
  readonly controller: HeldRollController;
  /** What the question came to once it ended: "Brisa respondeu.". */
  readonly note = signal('');
  private readonly ending = signal<'done' | 'closed' | 'lost' | null>(null);
  private readonly endedAtMs = signal(0);
  private readonly answering = signal<HeldAnswer | null>(null);
  private readonly retryAt = signal(0);

  /** Seconds until the master may answer: what is left of the roller's time, or of the pause after a refusal. */
  readonly seconds = computed(() => {
    const now = this.ctx.now();
    const retry = Math.ceil((this.retryAt() - now) / MS_PER_SECOND);
    return Math.max(this.controller.waitSeconds(now), retry, 0);
  });
  readonly ended = computed(() => this.ending() !== null);
  readonly endedAt = this.endedAtMs.asReadonly();
  readonly phase = computed<QuestionPhase>(() => {
    const end = this.ending();
    if (end) {
      return end;
    }
    if (this.controller.busy()) {
      return 'answering';
    }
    if (this.seconds() > 0) {
      return this.controller.refusal() === 'too-early' ? 'too-early' : 'waiting';
    }
    return 'ready';
  });
  /** "Brisa responde primeiro" / "Brisa não respondeu": the status line's words (without the countdown). */
  readonly lead = computed(() => {
    const phase = this.phase();
    if (phase === 'waiting' || phase === 'too-early') {
      return `${this.roller} responde primeiro`;
    }
    return phase === 'ready' ? `${this.roller} não respondeu` : '';
  });
  /** "você em 7 s": the countdown beside it, `''` when nothing counts. */
  readonly countdown = computed(() => {
    const phase = this.phase();
    return phase === 'waiting' || phase === 'too-early' ? `você em ${this.seconds()} s` : '';
  });
  /** The first words of the early refusal: "Ainda é a vez de Brisa responder."; the countdown is `alertCountdown`. */
  readonly alertLead = computed(() => `Ainda é a vez de ${this.roller} responder.`);
  readonly alertCountdown = computed(() => `Você pode responder em ${this.seconds()} s.`);
  /** What the answer on the way is doing: "Rolando o d20 de novo…". */
  readonly asking = computed(() => {
    const answer = this.answering();
    const offer = this.controller.offer();
    return answer && offer ? askingText(answer, offer) : '';
  });

  constructor(
    offer: HeldRollOffer,
    private readonly ctx: QuestionContext,
  ) {
    this.holdId = offer.holdId;
    this.combatantId = offer.combatantId;
    this.roller = offer.rollerLabel || 'Quem rolou';
    this.controller = new HeldRollController({
      send: ctx.send,
      settle: (res, done) => {
        if (!res.next) {
          this.end('done', answeredText(this.roller, done.answer, done.asked, res));
        }
      },
      master: true,
      now: ctx.clock,
      newKey: ctx.newKey,
    });
    this.controller.ask(offer);
  }

  /** The master's answer for the roller. */
  async answer(answer: HeldAnswer): Promise<void> {
    if (this.controller.busy()) {
      return;
    }
    this.answering.set(answer);
    await this.controller.answer(answer);
    this.answering.set(null);
    const refusal = this.controller.refusal();
    if (refusal === 'answered') {
      this.end('lost', HELD_ROLL_ANSWERED_TEXT);
    } else if (refusal === 'too-early') {
      this.retryAt.set(this.ctx.clock() + RETRY_MS);
    }
    this.ctx.changed();
  }

  /** The server lists the question again: a new question of the same roll replaces the old one. */
  refresh(offer: HeldRollOffer): void {
    const current = this.controller.offer();
    if (
      !this.ended() &&
      !this.controller.busy() &&
      current?.question.case !== offer.question.case
    ) {
      this.controller.ask(offer);
    }
  }

  /** The server stopped listing the question while it waited: the roller answered first. */
  gone(): void {
    if (!this.ended() && !this.controller.busy()) {
      const offer = this.controller.offer();
      this.end('closed', asksReroll(offer) ? REROLL_GONE_TEXT : `${this.roller} respondeu.`);
    }
  }

  private end(kind: 'done' | 'closed' | 'lost', note: string): void {
    this.note.set(note);
    this.endedAtMs.set(this.ctx.clock());
    this.ending.set(kind);
  }
}

/**
 * The questions the master sees, kept in step with what the page reads (the combat's, or `ListHeldRolls`'s): `sync` takes
 * the questions now listed, keeps the ones it knows (with their countdown and refusals), adds the new ones and closes the
 * ones that went away. A one-second clock (`now`) runs only while a question waits for its time or a closed one is still
 * on screen. Plain class a page or a panel builds; `dispose` stops the clock.
 */
export class HeldQuestions {
  /** The time the countdowns read, moved once a second while something counts. */
  readonly now: WritableSignal<number>;
  private readonly list = signal<readonly MasterQuestion[]>([]);
  readonly entries = this.list.asReadonly();
  /** How many questions wait for an answer (the closed ones that linger do not count). */
  readonly count = computed(() => this.list().filter((q) => !q.ended()).length);
  private readonly clock: () => number;
  private readonly context: QuestionContext;
  private timer: ReturnType<typeof setInterval> | null = null;

  constructor(deps: HeldQuestionsDeps) {
    this.clock = deps.clock ?? Date.now;
    this.now = signal(this.clock());
    this.context = {
      now: this.now,
      clock: this.clock,
      send: deps.send,
      newKey: deps.newKey,
      changed: () => this.schedule(),
    };
  }

  /** The question of the combatant, or `null`. */
  forCombatant(combatantId: string): MasterQuestion | null {
    return combatantId ? (this.list().find((q) => q.combatantId === combatantId) ?? null) : null;
  }

  /** Takes the questions the page reads now. */
  sync(offers: readonly HeldRollOffer[]): void {
    this.now.set(this.clock());
    const listed = new Map(offers.map((o) => [o.holdId, o]));
    const next: MasterQuestion[] = [];
    for (const known of this.list()) {
      const offer = listed.get(known.holdId);
      listed.delete(known.holdId);
      if (offer) {
        known.refresh(offer);
      } else {
        known.gone();
      }
      next.push(known);
    }
    for (const offer of listed.values()) {
      next.push(new MasterQuestion(offer, this.context));
    }
    this.list.set(this.pruned(next));
    this.schedule();
  }

  dispose(): void {
    this.stop();
  }

  private pruned(list: readonly MasterQuestion[]): readonly MasterQuestion[] {
    const now = this.now();
    return list.filter((q) => !q.ended() || now - q.endedAt() <= NOTE_LINGER_MS);
  }

  /** Runs the clock while it has something to move, and stops it when it has not. */
  private schedule(): void {
    const needed = this.list().some((q) => q.ended() || q.seconds() > 0);
    if (needed && this.timer === null) {
      this.timer = setInterval(() => this.tick(), TICK_MS);
    } else if (!needed) {
      this.stop();
    }
  }

  private tick(): void {
    this.now.set(this.clock());
    this.list.set(this.pruned(this.list()));
    this.schedule();
  }

  private stop(): void {
    if (this.timer !== null) {
      clearInterval(this.timer);
      this.timer = null;
    }
  }
}
