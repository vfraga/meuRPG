import { timestampFromMs } from '@bufbuild/protobuf/wkt';
import { describe, expect, it } from 'vitest';

import { ReactionKind } from '../../../gen/meurpg/play/v1/combat_pb';
import { reactionWindow } from './combat-testing';
import {
  MAP_WAIT_SECONDS,
  OTHER_DEVICE_TEXT,
  ON_MAP_TEXT,
  readiedSecondsLeft,
  masterBarWait,
  masterRowWait,
  otherDeviceText,
  playerWaitText,
  waitBeganMs,
} from './readied-wait';

const T0 = Date.parse('2026-10-10T12:00:00Z');
const MS = 1000;

const win = (over: object = {}) =>
  reactionWindow({
    id: 'w1',
    kind: ReactionKind.READIED,
    reactorId: 'pen',
    reactorLabel: 'Pensantus',
    reactorIsPlayer: true,
    ...over,
  } as never);

describe('the wait of a readied window (#857)', () => {
  it('is 120 s long, once, counted from the wait the server says has run when the window was read', () => {
    expect(MAP_WAIT_SECONDS).toBe(120);
    const w = win({ waitBeganAt: timestampFromMs(T0), waitedMs: 5000n, rolling: true });
    expect(waitBeganMs(w)).toBe(T0);
    // read at 1000 (5 s of the wait had run), 12 s later: 17 s have run.
    expect(readiedSecondsLeft(w, 0, 1000 + 12 * MS, 1000, 1000 + 12 * MS)).toBe(103);
  });

  it("counts the wait with the server's waited_ms and the time since the read, never with this device's clock against wait_began_at", () => {
    // This device's clock is an hour ahead of the server's: wait_began_at is an hour in the past for it.
    const w = win({ waitBeganAt: timestampFromMs(T0), waitedMs: 30_000n, rolling: true });
    const readAt = T0 + 3_600_000;
    expect(readiedSecondsLeft(w, readAt, readAt, readAt, readAt)).toBe(90);
    expect(readiedSecondsLeft(w, readAt, readAt + 10 * MS, readAt, readAt + 10 * MS)).toBe(80);
  });

  it('goes on counting from the same start when the player is back (rolling cleared) and never restarts', () => {
    const marked = win({ waitBeganAt: timestampFromMs(T0), waitedMs: 40_000n, rolling: true });
    const back = win({ waitBeganAt: timestampFromMs(T0), waitedMs: 40_000n, rolling: false });
    expect(readiedSecondsLeft(marked, 0, 8 * MS, 0, 8 * MS)).toBe(72);
    expect(readiedSecondsLeft(back, 0, 8 * MS, 0, 8 * MS)).toBe(72);
  });

  it('is the 30 s of the window, counted from when it was first seen, before any mark', () => {
    const w = win();
    expect(waitBeganMs(w)).toBeNull();
    expect(readiedSecondsLeft(w, T0, T0 + 6 * MS, T0 + 20 * MS, T0 + 6 * MS)).toBe(24);
  });

  it('never goes below 0 and never above 120', () => {
    const w = win({ waitBeganAt: timestampFromMs(T0), waitedMs: 1000n });
    expect(readiedSecondsLeft(w, 0, 500 * MS, 0, 500 * MS)).toBe(0);
    expect(readiedSecondsLeft(w, 0, -30 * MS, 0, -30 * MS)).toBe(120);
  });

  it('rounds the seconds up, as the other clocks do', () => {
    const w = win({ waitBeganAt: timestampFromMs(T0), waitedMs: 0n });
    expect(readiedSecondsLeft(w, 0, 12_400, 0, 12_400)).toBe(108);
  });

  it('is said to the player in the window with the running count (857-09 card 8)', () => {
    expect(playerWaitText(24)).toBe('Se você não responder, passa sozinho em 24 s.');
    expect(playerWaitText(72)).toBe('Se você não responder, passa sozinho em 1 min 12 s.');
  });

  it('is said on the map as one wait of 2 minutes', () => {
    expect(ON_MAP_TEXT).toBe('A mesa espera você: até 2 min.');
  });

  it("is said on the player's other device when this one is not on the map", () => {
    expect(otherDeviceText(108)).toBe(
      'Você abriu o mapa em outro aparelho. Passa sozinho em 1 min 48 s.',
    );
    expect(OTHER_DEVICE_TEXT).toContain('outro aparelho');
  });

  it("says the master's row: waiting before the map, on the map after it (857-08)", () => {
    expect(masterRowWait('Pensantus', false, 24)).toEqual({
      onMap: false,
      text: 'Esperando Pensantus · passa sozinho em 24 s',
    });
    expect(masterRowWait('Pensantus', true, 108)).toEqual({
      onMap: true,
      text: 'No mapa · passa sozinho em 1 min 48 s',
    });
    expect(masterRowWait('Pensantus', false, 72).text).toBe(
      'Esperando Pensantus · passa sozinho em 1 min 12 s',
    );
  });

  it('says only who the master waits for when the window has no clock', () => {
    expect(masterRowWait('Tor', false, undefined)).toEqual({ onMap: false, text: 'Esperando Tor' });
  });

  it("says the master's band in one line (857-08 card 7)", () => {
    expect(masterBarWait('Pensantus', false, 24)).toBe(
      'Esperando a reação de Pensantus · passa sozinho em 24 s',
    );
    expect(masterBarWait('Pensantus', true, 108)).toBe(
      'Pensantus está no mapa · passa sozinho em 1 min 48 s',
    );
  });
});
