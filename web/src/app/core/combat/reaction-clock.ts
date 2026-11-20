/**
 * The clocks of the automatic pass, apart from the windows: how long an optional reaction waits, how long once its player is rolling
 * or on the map, and how a count is said. Shared by `reaction-autopass.ts` and `readied-wait.ts`.
 */

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

const SECONDS_PER_MINUTE = 60;

/** "1 min 52 s", or "24 s" under a minute. */
export function clockText(seconds: number): string {
  if (seconds < SECONDS_PER_MINUTE) {
    return `${seconds} s`;
  }
  const rest = seconds % SECONDS_PER_MINUTE;
  return `${Math.floor(seconds / SECONDS_PER_MINUTE)} min${rest > 0 ? ` ${rest} s` : ''}`;
}
