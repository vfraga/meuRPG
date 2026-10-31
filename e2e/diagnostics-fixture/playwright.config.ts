import { defineConfig } from '@playwright/test';

// The browser the suite drives (playwright.config.ts): the installed Chrome locally, Playwright's Chromium in CI.
const channel = process.env.E2E_BROWSER_CHANNEL ?? (process.env.CI ? '' : 'chrome');

// Tests that fail on purpose, run by tests/diagnostics.spec.ts to see what a failed test attaches. No stack, no
// global setup: the pages are made here. The caller names the output folder (--output): a run empties it, and
// this one runs inside the suite.
export default defineConfig({
  testDir: '.',
  testMatch: /fails\.spec\.ts/,
  retries: 0,
  workers: 1,
  timeout: 5_000,
  use: { channel: channel || undefined, actionTimeout: 2_000, trace: 'retain-on-failure', screenshot: 'only-on-failure' },
});
