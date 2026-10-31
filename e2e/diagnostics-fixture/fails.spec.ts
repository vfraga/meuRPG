import { expect } from '@playwright/test';

import { test } from '../tests/diagnostics';

// Run by tests/diagnostics.spec.ts, never by the suite (playwright.config.ts only reads ../tests).
test('a click whose answer never arrives', async ({ page }) => {
  await page.route('http://diagnostics.test/never', () => {});
  await page.route('http://diagnostics.test/', (route) =>
    route.fulfill({ contentType: 'text/html; charset=utf-8', body: '<button onclick="fetch(\'/never\')">pedir</button><p id="done" hidden>pronto</p>' }),
  );
  await page.goto('http://diagnostics.test/');
  await page.getByRole('button', { name: 'pedir' }).click();
  await expect(page.locator('#done')).toBeVisible({ timeout: 1_500 });
});

test('a page that crashed in a context the test closes itself', async ({ browser }) => {
  const context = await browser.newContext();
  try {
    const page = await context.newPage();
    const crashed = page.waitForEvent('crash');
    await page.goto('chrome://crash').catch(() => undefined);
    await crashed;
    expect(page.isClosed()).toBe(true);
  } finally {
    await context.close();
  }
});
