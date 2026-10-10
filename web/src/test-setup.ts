import { ɵgetCleanupHook as getCleanupHook } from '@angular/core/testing';
import { vi } from 'vitest';

/**
 * The per-test reset that `src/test-hooks.ts` registers for every spec file, bundled with the app code so it shares the
 * app's TestBed (`@angular/core/testing` is bundled, not external). This file only publishes the two functions on a
 * global; it registers no hook itself (see `src/test-hooks.ts` for why).
 *
 * The Angular unit-test builder runs Vitest without isolation between spec files (`isolate: false`), so a global that one
 * file stubs (`vi.stubGlobal('matchMedia', …)`) stays in place for the next file the same worker runs: a test that passes
 * alone fails after another one, depending on which files share a worker. Every test ends with the stubbed globals
 * restored, with the real clock back and with the TestBed reset, so no spec depends on the order the files run in.
 *
 * jsdom has no `Element.prototype.scrollIntoView`, which components call (a question brought into view, the field a
 * refusal points at). Every test starts with a fresh no-op mock of it: a spec never depends on another file having
 * defined it first, and a mock one test changes never reaches the next. The same goes for `window.scrollTo`, which jsdom
 * only answers with a "Not implemented" line that buried the real output of a run.
 */
const before = getCleanupHook(false);
const after = getCleanupHook(true);

export const testHooks = {
  beforeEach(): void {
    before();
    Element.prototype.scrollIntoView = vi.fn();
    window.scrollTo = vi.fn();
  },
  afterEach(): void {
    vi.unstubAllGlobals();
    // A fake clock one test turns on (vi.useFakeTimers) never reaches the next file either.
    vi.useRealTimers();
    after();
  },
};

(globalThis as Record<symbol, unknown>)[Symbol.for('meurpg.testHooks')] = testHooks;
