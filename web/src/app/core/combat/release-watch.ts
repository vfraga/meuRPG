import {
  DestroyRef,
  type EffectRef,
  computed,
  effect,
  inject,
  signal,
  untracked,
} from '@angular/core';

import {
  CombatantState,
  type Encounter,
  type ReactionWindow,
  ReadiedStatus,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { readiedSecondsLeft, waitBeganMs } from './readied-wait';
import { type GoneView, releaseGoneView } from './release-text';

/** Which readied spell or move the release is for. */
export interface ReleaseWatchIds {
  readonly windowId: string;
  readonly readiedId: string;
  readonly reactorId: string;
  /** The held spell's name; `''` for a move. */
  readonly spellName: string;
  readonly move: boolean;
}

/** How often the clock of the wait redraws, in ms. */
const TICK_MS = 1000;

/**
 * What a release (the cast sheet's map, the Mover page) reads of its READIED window while it is open (#857, #856): whether it is
 * still in the combat, the seconds left of the wait and, once it went away, what happened (the master undid the trigger, the reaction
 * was used, the time ran out). The map goes away with the window and the one button left is "Voltar à sessão". Build it in an
 * injection context: it keeps an effect and a one-second timer, both stopped with the screen.
 */
export class ReleaseWatch {
  private readonly now = signal(Date.now());
  /** The monotonic clock (`performance.now()`) of the 120 s: a step of the wall clock mid-wait does not move it. */
  private readonly mono = signal(performance.now());
  private readonly seenAt = Date.now();
  private last: ReactionWindow | undefined;
  /** When `last` was read, on the monotonic clock: the 120 s count from the server's `waited_ms` of that read plus the time since (#857). */
  private readAt = this.mono();
  private readonly goneAt = signal<number | null>(null);
  private readonly goneMono = signal(0);
  private readonly frozen = signal<GoneView | null>(null);

  /** The window as the combat has it now. */
  readonly window = computed(() =>
    this.encounter()?.reactionWindows.find((w) => w.id === this.ids.windowId),
  );
  readonly gone = computed(() => !this.window());
  /** The seconds left before the master's screen passes the window: the 120 s once the player was on the map, else the 30 s. */
  readonly secondsLeft = computed(() =>
    readiedSecondsLeft(
      this.window() ?? this.last ?? ({} as ReactionWindow),
      this.seenAt,
      this.goneAt() ?? this.now(),
      this.readAt,
      this.goneAt() === null ? this.mono() : this.goneMono(),
    ),
  );
  /** The player opened the map while the window was open: its wait is the 120 s, not the 30 s. */
  readonly waitedOnMap = signal(false);
  /** What happened to the window; `null` while it is open. It is said once, when the window is first seen gone. */
  readonly view = computed(() => this.frozen());

  constructor(
    private readonly encounter: () => Encounter | null,
    private readonly ids: ReleaseWatchIds,
    /** Told the window on every change while it is open (the player's mark reads a reopen from it). */
    private readonly onWindow?: (w: ReactionWindow) => void,
  ) {
    this.timer = setInterval(() => {
      this.now.set(Date.now());
      this.mono.set(performance.now());
    }, TICK_MS);
    inject(DestroyRef).onDestroy(() => this.destroy());
    const first = this.window();
    if (first) {
      this.see(first);
    }
    this.watching = effect(() => {
      const w = this.window();
      const e = this.encounter();
      untracked(() => (w ? this.see(w) : this.settle(e)));
    });
  }

  private readonly timer: ReturnType<typeof setInterval>;
  private readonly watching: EffectRef;

  /** Stops the clock and the watch: the release is over. Built in a longer-lived injector (the combat view's), it would run until that one goes. */
  destroy(): void {
    clearInterval(this.timer);
    this.watching.destroy();
  }

  private see(w: ReactionWindow): void {
    if (this.last !== w) {
      this.last = w;
      // The clock of the redraws catches up to the read, so the time since it is never negative.
      this.readAt = performance.now();
      this.mono.set(this.readAt);
      this.now.set(Date.now());
    }
    this.waitedOnMap.set(waitBeganMs(w) !== null);
    this.onWindow?.(w);
  }

  private settle(e: Encounter | null): void {
    if (this.goneAt() !== null || !e) {
      return;
    }
    const at = Date.now();
    const mono = performance.now();
    this.goneMono.set(mono);
    this.goneAt.set(at);
    const who = e.combatants.find((c) => c.id === this.ids.reactorId);
    const held = who?.readied;
    this.frozen.set(
      releaseGoneView({
        spell: this.ids.spellName,
        move: this.ids.move,
        stillWaiting: held?.id === this.ids.readiedId && held.status === ReadiedStatus.WAITING,
        reactionUsed: who?.reactionUsed ?? false,
        down: who?.state === CombatantState.DOWN,
        timedOut:
          !!this.last && readiedSecondsLeft(this.last, this.seenAt, at, this.readAt, mono) === 0,
        afterMap: this.waitedOnMap(),
      }),
    );
  }
}
