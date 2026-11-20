import { afterEach, beforeEach, vi } from 'vitest';
import { noShadowedScrollIntoView } from './test-setup-guard';

/**
 * Runs before every spec file (the `setupFiles` of the `test` target in angular.json).
 *
 * The Angular unit-test builder runs Vitest without isolation between spec files (`isolate: false`), so a global that one
 * file stubs (`vi.stubGlobal('matchMedia', …)`) stays in place for the next file the same worker runs: a test that passes
 * alone fails after another one, depending on which files share a worker. Every test ends with the stubbed globals
 * restored, so no spec depends on the order the files run in.
 *
 * jsdom has no `Element.prototype.scrollIntoView`, which components call (a question brought into view, the field a
 * refusal points at). Every test starts with a fresh no-op mock of it: a spec never depends on another file having
 * defined it first, and a mock one test changes never reaches the next. The same goes for `window.scrollTo`, which jsdom
 * only answers with a "Not implemented" line that buried the real output of a run.
 *
 * With `--coverage` this file runs once per worker instead of once per spec file (measured on 07/10/2026: 9 runs for 381
 * files, so these hooks reached 358 of 4,757 tests), and `test-setup.spec.ts` fails. That is why CI decides with a plain
 * `npm test` and runs the coverage only for its report (see `.github/workflows/web.yml`).
 */
beforeEach(() => {
  Element.prototype.scrollIntoView = vi.fn();
  window.scrollTo = vi.fn();
});

afterEach(() => {
  noShadowedScrollIntoView();
  vi.unstubAllGlobals();
  // A fake clock one test turns on (vi.useFakeTimers) never reaches the next file either.
  vi.useRealTimers();
});
