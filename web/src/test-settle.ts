import { vi } from 'vitest';

/** What a component fixture needs to be settled: `ComponentFixture` has both. */
interface Settleable {
  detectChanges(): void;
  whenStable(): Promise<unknown>;
}

/** Fake timers replace `setTimeout` with a function that carries its clock; the real one has none. */
function timeoutIsFaked(): boolean {
  return vi.isFakeTimers() && 'clock' in globalThis.setTimeout;
}

/**
 * One turn of the event loop, for a spec that has to let the mocked promises and the component's own
 * `setTimeout(…, 0)` run. It never waits on a clock a spec may not own: when the fake clock is on, the turn is
 * the fake clock advanced by 0 ms (a real `setTimeout` would never fire, and the test would sit until its
 * timeout); otherwise it is one real macrotask.
 *
 * That hang was the web coverage failure: with `--coverage` the afterEach of `test-setup.ts` runs for the first
 * spec file of each worker only, so a file that turned the fake clock on and did not turn it off left it on for
 * every file after it in that worker, and each of their `await new Promise((r) => setTimeout(r))` timed out.
 */
export function nextTurn(): Promise<void> {
  if (timeoutIsFaked()) {
    return vi.advanceTimersByTimeAsync(0).then(() => undefined);
  }
  return new Promise((resolve) => setTimeout(resolve, 0));
}

/**
 * Settles a component: renders, lets one turn run, waits for what the zone-less scheduler still holds, renders
 * again. `turns` is for a chain of awaits that each need the view drawn before the next one starts (render, click,
 * answer, render): one turn per link, not a guess to be raised when a test is flaky.
 */
export async function settle(fixture: Settleable, turns = 1): Promise<void> {
  for (let i = 0; i < turns; i++) {
    fixture.detectChanges();
    await nextTurn();
    await fixture.whenStable();
  }
  fixture.detectChanges();
}
