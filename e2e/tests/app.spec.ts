import { expect } from '@playwright/test';
import { test } from './diagnostics';

test('o app carrega e mostra a versão do servidor', { tag: '@smoke' }, async ({ page }) => {
  await page.goto('/');

  await expect(page.getByText('Servidor conectado')).toBeVisible();
  // compose.yaml builds the image with VERSION=dev; CI can pass another one.
  await expect(page.locator('dt:text-is("Versão") + dd')).toHaveText(process.env.E2E_SERVER_VERSION ?? 'dev');
});
