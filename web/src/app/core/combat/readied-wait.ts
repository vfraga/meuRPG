import { timestampMs } from '@bufbuild/protobuf/wkt';

import type { ReactionWindow } from '../../../gen/meurpg/play/v1/combat_pb';
import { AUTO_PASS_SECONDS, ROLLING_WAIT_SECONDS, clockText } from './reaction-clock';

/**
 * The wait of a readied action's window while its player is on the map (#857, #856, boards 857-08 and 857-09): the 30 s of the window
 * give way, at the first entry on the map, to one wait of 2 minutes. The server records when it began (`ReactionWindow.wait_began_at`,
 * once; no later mark or clear changes it) and says how long it has run (`waited_ms`); the master's screen passes the window when the 120 s end. Pure functions.
 */

/** The one wait the player gets once on the map, in seconds. */
export const MAP_WAIT_SECONDS: number = ROLLING_WAIT_SECONDS;

const MS_PER_SECOND = 1000;

/** What the player reads under the map: the table waits once, 2 minutes, from the first entry. */
export const ON_MAP_TEXT = 'A mesa espera você: até 2 min.';

/** The prefix of the line a second device reads while the player is on the map on another one. */
export const OTHER_DEVICE_TEXT = 'Você abriu o mapa em outro aparelho.';

/** When the 120 s began, in ms, or `null` before the player's first entry on the map. */
export function waitBeganMs(w: ReactionWindow): number | null {
  return w.waitBeganAt ? timestampMs(w.waitBeganAt) : null;
}

function clamp(seconds: number, max: number): number {
  return Math.min(max, Math.max(0, seconds));
}

/**
 * The seconds left before a readied window passes by itself (rounded up, held between 0 and the wait). Once the player marked the
 * map, the 120 s count from the wait the server says has run (`ReactionWindow.waited_ms`, worked out with its own clock) plus the
 * time since this screen read the window, on a monotonic clock (`readAt` and `mono`, from `performance.now()`): neither the device's clock
 * set against `wait_began_at` nor a step of the wall clock mid-wait moves the 120 s. Before the mark it is the 30 s of the window from when this
 * screen first saw it (`seenAt`).
 */
export function readiedSecondsLeft(
  w: ReactionWindow,
  seenAt: number,
  now: number,
  readAt: number,
  mono: number,
): number {
  if (waitBeganMs(w) !== null) {
    const waited = Number(w.waitedMs) + (mono - readAt);
    return clamp(Math.ceil(MAP_WAIT_SECONDS - waited / MS_PER_SECOND), MAP_WAIT_SECONDS);
  }
  return clamp(Math.ceil(AUTO_PASS_SECONDS - (now - seenAt) / MS_PER_SECOND), AUTO_PASS_SECONDS);
}

/** What the player reads in the window: "Se você não responder, passa sozinho em 1 min 12 s.". */
export function playerWaitText(seconds: number): string {
  return `Se você não responder, passa sozinho em ${clockText(seconds)}.`;
}

/** What the player's other device reads in its window while the map is open on another: "Você abriu o mapa em outro aparelho. Passa sozinho em 1 min 48 s.". */
export function otherDeviceText(seconds: number): string {
  return `${OTHER_DEVICE_TEXT} Passa sozinho em ${clockText(seconds)}.`;
}

/** The wait line of the master's row: where the player is and the time left; `seconds` is `undefined` for a window with no clock (it waits for another). */
export function masterRowWait(
  reactor: string,
  onMap: boolean,
  seconds: number | undefined,
): { readonly onMap: boolean; readonly text: string } {
  const lead = onMap ? 'No mapa' : `Esperando ${reactor}`;
  return {
    onMap,
    text: seconds === undefined ? lead : `${lead} · passa sozinho em ${clockText(seconds)}`,
  };
}

/** The master's band above the order, in one line. */
export function masterBarWait(reactor: string, onMap: boolean, seconds: number): string {
  const lead = onMap ? `${reactor} está no mapa` : `Esperando a reação de ${reactor}`;
  return `${lead} · passa sozinho em ${clockText(seconds)}`;
}
