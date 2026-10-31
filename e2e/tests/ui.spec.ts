import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { callRPC, idpOrigin } from './support';

// The sign-in flow as a person does it: through the app's own buttons. This
// file, login.spec.ts and invite.spec.ts's signed-out test are the only
// ones that sign in for real (auth.setup.ts's saved states cover every
// other spec — see support.ts's `signIn` doc comment) — deliberately, since
// signing in is the point here. The other login tests drive /auth/login
// directly and check the server's side; these check what the user sees.
test.describe('entrar e sair pela interface', () => {
  test('o mestre entra pelo botão "Entrar" e sai pelo "Sair" @MR-001', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('button', { name: 'Entrar' }).click();

    await expect(page).toHaveURL((url) => url.origin === idpOrigin && url.pathname === '/authorize');
    await page.getByRole('button', { name: 'Mestre Teste', exact: true }).click();

    await expect(page).toHaveURL('/');
    // "Minha conta" is the account menu's fallback label until a display
    // name is set (user-menu.ts) — true for "Mestre Teste" only as long as
    // no other spec ever sets one on this shared devidp account. No spec in
    // this suite does (character-*.spec.ts never touches /profile); keep it
    // that way, or give any test that needs a display name its own fresh
    // sign-in instead of reusing the shared saved state.
    await expect(page.getByText('Minha conta')).toBeVisible();

    await page.getByRole('button', { name: 'Sair' }).click();
    await expect(page.getByRole('button', { name: 'Entrar' })).toBeVisible();
    expect((await callRPC(page, 'meurpg.identity.v1.IdentityService/GetMe')).status()).toBe(401);
  });

  test('uma página protegida manda para o login e volta para ela depois', async ({ page }) => {
    await page.goto('/campaigns');

    await expect(page).toHaveURL((url) => url.origin === idpOrigin && url.pathname === '/authorize');
    await page.getByRole('button', { name: 'Jogador Teste', exact: true }).click();

    await expect(page).toHaveURL('/campaigns');
    await expect(page.getByRole('heading', { name: 'Minhas campanhas' })).toBeVisible();
  });
});
