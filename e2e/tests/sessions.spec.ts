import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { callRPC, signIn } from './support';

// "Sair dos outros dispositivos" (ASVS 5.0 V7.4.3). The account "Sessões
// Teste" exists only for this: ending every other session of an account
// would break every other spec sharing it, so no saved state of it exists
// and nothing else signs in as it.
test.describe('sessões', () => {
  test('sair dos outros dispositivos termina a outra sessão e mantém esta', async ({ browser }) => {
    const laptop = await browser.newContext();
    const phone = await browser.newContext();
    try {
      const laptopPage = await laptop.newPage();
      const phonePage = await phone.newPage();
      await signIn(laptopPage, 'Sessões Teste', '/');
      await signIn(phonePage, 'Sessões Teste', '/');

      // The laptop sees the phone. (A regex, not "1": a local rerun leaves the sessions of
      // earlier runs behind; a fresh database, as in CI, has exactly one. The exact
      // wording is covered by profile.spec.ts.)
      await laptopPage.goto('/profile');
      await expect(laptopPage.getByText(/conectado em \d+ outros? dispositivos?\./)).toBeVisible();

      // Asking is not enough: the confirmation comes first, on the page.
      await laptopPage.getByRole('button', { name: 'Sair dos outros dispositivos' }).click();
      await expect(laptopPage.getByText('Não dá para desfazer.')).toBeVisible();
      await laptopPage.getByRole('button', { name: 'Confirmar saída' }).click();
      await expect(laptopPage.getByRole('status').filter({ hasText: /Pronto: \d+ dispositivos? (foi|foram) desconectados?\./ })).toBeVisible();

      // The laptop is still signed in; the phone is signed out for real.
      expect((await callRPC(laptopPage, 'meurpg.identity.v1.IdentityService/GetMe')).ok()).toBeTruthy();
      expect((await callRPC(phonePage, 'meurpg.identity.v1.IdentityService/GetMe')).status()).toBe(401);
      // The app sends the phone through sign-in again. (devidp remembers its own
      // login for an hour, so it may bring the phone straight back, signed in with
      // a NEW session: what matters is that the old cookie was refused.)
      const signInStarted = phonePage.waitForRequest((req) => new URL(req.url()).pathname === '/auth/login');
      await phonePage.goto('/profile');
      await signInStarted;
    } finally {
      await laptop.close();
      await phone.close();
    }
  });
});
