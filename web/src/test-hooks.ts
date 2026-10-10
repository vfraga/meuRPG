import { afterEach, beforeEach } from 'vitest';

/**
 * Registers the per-test hooks of `src/test-setup.ts` (the `setupFiles` of the `test` target in angular.json lists the
 * builder's own setup, and `vitest.config.ts` lists this file).
 *
 * Vitest runs a setup file again before every spec file, and that is what registers `beforeEach`/`afterEach` for the
 * file's tests. With `--coverage` the Angular builder's bundled setup files are not run again: they stay cached for the
 * worker, so only the first spec file of each worker got the hooks (measured on 07/10/2026: 9 runs for 381 files) and
 * the later ones kept the previous file's TestBed, fake clock and stubs, which hung their `await fixture.whenStable()` and
 * `setTimeout` waits until the 5 s test timeout. This file is loaded by Vite itself, not bundled, so Vitest runs it
 * again for every file in both runs, and it only reaches the bundled code through the global that `test-setup.ts` fills.
 */
type Hooks = { beforeEach(): void; afterEach(): void };
const hooks = (): Hooks =>
  (globalThis as Record<symbol, unknown>)[Symbol.for('meurpg.testHooks')] as Hooks;

beforeEach(() => hooks().beforeEach());
afterEach(() => hooks().afterEach());
