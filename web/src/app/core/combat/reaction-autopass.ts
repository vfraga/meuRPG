import { ReactionKind, type ReactionWindow } from '../../../gen/meurpg/play/v1/combat_pb';
import { isOpen } from './reactions';

/**
 * How long a player's optional reaction waits before it passes by itself.
 * Vinicius, 10/10: an unanswered optional reaction passes after 30 s.
 */
export const AUTO_PASS_SECONDS = 30;

/**
 * How long an optional reaction waits once its player said they are rolling the die (`ReactionWindow.rolling`):
 * a physical die takes a while to be found, rolled and typed. Still bounded, so a player who walked away does not
 * hold the table forever; after it, the window passes by itself like any other, and the master can pass it by hand
 * sooner ("Deixar passar").
 */
export const ROLLING_WAIT_SECONDS = 120;

const MS_PER_SECOND = 1000;
const SECONDS_PER_MINUTE = 60;

/** `seconds` held between 0 and `max`. */
function clamp(seconds: number, max: number): number {
  return Math.min(max, Math.max(0, seconds));
}

/**
 * The reactions a player may leave unanswered: using one is a choice, so not answering is the same as "Deixar passar".
 * A readied action fired by the master (READIED) is one too: its row is then ignored, and the reaction stays.
 * So is a reaction feature whose trigger happened (FEATURE): the player uses it or lets it pass. A hidden creature's window
 * comes with `unanswered` from the start, so it is the master's and nothing passes it.
 * While a readied attack's roll is held for other reactions, the server sends its window with `answerNow` false: it does not pass.
 * A rider's dismount as its mount falls (DISMOUNT) is not here: leaving it unanswered would be a fall prone, so it waits for the
 * rider's player or the master (#588).
 */
const OPTIONAL_KINDS: ReadonlySet<ReactionKind> = new Set([
  ReactionKind.SHIELD,
  ReactionKind.UNCANNY_DODGE,
  ReactionKind.HELLISH_REBUKE,
  ReactionKind.COUNTERSPELL,
  ReactionKind.CUTTING_WORDS,
  ReactionKind.DEFLECT_MISSILES,
  ReactionKind.MANEUVER_REDUCE,
  ReactionKind.PROTECTION,
  ReactionKind.FEATHER_FALL,
  ReactionKind.SLOW_FALL,
  ReactionKind.READIED,
  ReactionKind.FEATURE,
]);

/**
 * Whether the window is an optional reaction: never a saving throw (concentration, an effect's, the Repreensão Infernal
 * second step), a contest or the master's check, which are answered with other calls or have to be rolled. (An
 * opportunity attack is optional too, but it is not a window: it has its own clock, over the offers.)
 */
export function isOptionalReaction(w: ReactionWindow): boolean {
  return OPTIONAL_KINDS.has(w.kind) && !w.secondStep && isOpen(w);
}

/** A window the master's screen passes after `AUTO_PASS_SECONDS`: an optional reaction of a player whose turn to answer it is. */
export function autoPasses(w: ReactionWindow): boolean {
  // A window that came to the master (`unanswered`) waits for his answer: the server handed it over, nothing passes it by itself.
  return isOptionalReaction(w) && w.reactorIsPlayer && w.answerNow && !w.unanswered;
}

/** Where a UUID keeps its version and variant digits ("xxxxxxxx-xxxx-Vxxx-Nxxx-xxxxxxxxxxxx"). */
const UUID_VERSION_AT = 14;
const UUID_VARIANT_AT = 19;
const HEX_MAX = 15;
const HEX = 16;

/**
 * The idempotency key of the automatic pass of a window: two tabs, or a retry, never answer twice. The server takes
 * only a UUID as a key, so it is the window's own id with every hex digit flipped, but the version and variant ones:
 * another UUID, the same on every screen.
 */
export function autoPassKey(windowId: string): string {
  return windowId
    .toLowerCase()
    .replace(/[0-9a-f]/g, (d, at: number) =>
      at === UUID_VERSION_AT || at === UUID_VARIANT_AT
        ? d
        : (HEX_MAX - parseInt(d, HEX)).toString(HEX),
    );
}

/**
 * The clock of the automatic pass: when this screen first saw each window waiting for its player's answer. In memory
 * only; a reload starts the 30 s again.
 */
export class AutoPassClock {
  private readonly seen = new Map<string, number>();
  /** When this screen first saw each window marked as being rolled: the longer wait counts from here. */
  private readonly rollingSince = new Map<string, number>();
  private readonly sent = new Set<string>();

  /** Looks at the open windows at `now` (ms): forgets the gone ones, starts the clock of the new ones. */
  sync(windows: readonly ReactionWindow[], now: number): void {
    const waiting = windows.filter(autoPasses);
    this.syncIds(
      waiting.map((w) => w.id),
      now,
    );
    for (const w of waiting) {
      if (w.rolling && !this.rollingSince.has(w.id)) {
        this.rollingSince.set(w.id, now);
      }
    }
  }

  /** The same for any list of ids waiting for a player (an opportunity attack's offer): the ones gone are forgotten. */
  syncIds(ids: Iterable<string>, now: number): void {
    const waiting = new Set(ids);
    for (const id of [...this.seen.keys()]) {
      if (!waiting.has(id)) {
        this.seen.delete(id);
        this.rollingSince.delete(id);
      }
    }
    for (const id of waiting) {
      if (!this.seen.has(id)) {
        this.seen.set(id, now);
      }
    }
  }

  /** The seconds left of a window (rounded up, never below 0), or `null` for one with no clock. */
  secondsLeft(windowId: string, now: number): number | null {
    const since = this.seen.get(windowId);
    if (since === undefined) {
      return null;
    }
    // A window its player is rolling waits longer, counted from when this screen saw the mark (never shorter than
    // what was left: the 30 s may be nearly up when the mark arrives).
    const rolling = this.rollingSince.get(windowId);
    if (rolling !== undefined) {
      return clamp(
        Math.ceil(ROLLING_WAIT_SECONDS - (now - rolling) / MS_PER_SECOND),
        ROLLING_WAIT_SECONDS,
      );
    }
    return Math.max(0, Math.ceil(AUTO_PASS_SECONDS - (now - since) / MS_PER_SECOND));
  }

  /** Whether the window's player said they are rolling (the master's card reads "Rolando…"). */
  isRolling(windowId: string): boolean {
    return this.rollingSince.has(windowId);
  }

  /** The windows whose 30 s are up and were not passed yet: each is returned once. */
  due(now: number): string[] {
    const ids = [...this.seen.keys()].filter(
      (id) => !this.sent.has(id) && this.secondsLeft(id, now) === 0,
    );
    for (const id of ids) {
      this.sent.add(id);
    }
    return ids;
  }

  /** A pass that failed for a reason that is not a refusal (the network): its window may be sent again. */
  release(windowId: string): void {
    this.sent.delete(windowId);
  }

  /** The least seconds left among the windows with a clock, or `null` when none. */
  soonest(now: number): number | null {
    const left = [...this.seen.keys()].map((id) => this.secondsLeft(id, now) ?? Infinity);
    return left.length === 0 ? null : Math.min(...left);
  }
}

/** "Passa sozinho em 24 s". */
export function autoPassText(seconds: number): string {
  return `Passa sozinho em ${seconds} s`;
}

/** What the player reads on their own prompt: "Se você não responder, passa sozinho em 24 s.". */
export function playerAutoPassText(seconds: number): string {
  return `Se você não responder, passa sozinho em ${seconds} s.`;
}

/**
 * The seconds left on the player's own prompt, counted from when it opened (`openedAt`, ms). The player's client never
 * answers by itself: the master's screen sends the pass.
 */
export function playerSecondsLeft(openedAt: number, now: number): number {
  return Math.max(0, Math.ceil(AUTO_PASS_SECONDS - (now - openedAt) / MS_PER_SECOND));
}

/** The master's card while the player rolls: "Rolando… passa sozinho em 1 min 52 s". */
export function rollingText(seconds: number): string {
  return `Rolando… ${autoPassText(seconds).toLowerCase()}`;
}

/** "1 min 52 s", or "24 s" under a minute. */
export function clockText(seconds: number): string {
  if (seconds < SECONDS_PER_MINUTE) {
    return `${seconds} s`;
  }
  const rest = seconds % SECONDS_PER_MINUTE;
  return `${Math.floor(seconds / SECONDS_PER_MINUTE)} min${rest > 0 ? ` ${rest} s` : ''}`;
}

/** What the player reads once they are rolling, in both steps: how long the table waits for the die. */
export function playerRollingText(seconds: number): string {
  return `Você está rolando: a mesa espera ${clockText(seconds)}. Depois disso a reação passa sozinha.`;
}

/** The seconds left of the longer wait, counted from when the player said they were rolling (`startedAt`, ms). */
export function playerRollingSecondsLeft(startedAt: number, now: number): number {
  // The screen's clock ticks once a second, so `now` can be a hair older than `startedAt`: never above the wait.
  return clamp(
    Math.ceil(ROLLING_WAIT_SECONDS - (now - startedAt) / MS_PER_SECOND),
    ROLLING_WAIT_SECONDS,
  );
}
