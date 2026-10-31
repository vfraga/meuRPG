import { test as setup } from './diagnostics';

import { authStatePath, signIn } from './support';

// Playwright's standard "setup project" pattern: signs in once per test
// user this suite needs, and saves each signed-in `storageState` (cookies)
// to a gitignored file (`authStatePath`, `e2e/.gitignore`) that every other
// test reuses instead of hitting the real, rate-limited `/auth/login`
// again (`backend/internal/identity/login.go`'s 20-per-client burst).
// `playwright.config.ts`'s "chrome" project has `dependencies: ['setup']`,
// so these three run first, in the "setup" project, before anything else.
//
// "Mestre Teste" and "Jogador Teste" are the master and the player of nearly
// every spec; "E-mail Não Verificado" is the second player of the specs that
// need two people at one table (the fog of war: Pensantus and Toren see
// different maps at the same moment). `creatures.spec.ts` also signs in as it,
// for real and in a fresh context, because what that test proves is the
// sign-in of a third account; it never touches the state saved here, so both
// uses work side by side. Never sign the saved state out (`signOut` deletes
// the session on the server).

setup('autenticar como Mestre Teste', async ({ page }) => {
  await signIn(page, 'Mestre Teste', '/');
  await page.context().storageState({ path: authStatePath('Mestre Teste') });
});

setup('autenticar como Jogador Teste', async ({ page }) => {
  await signIn(page, 'Jogador Teste', '/');
  await page.context().storageState({ path: authStatePath('Jogador Teste') });
});

setup('autenticar como E-mail Não Verificado', async ({ page }) => {
  await signIn(page, 'E-mail Não Verificado', '/');
  await page.context().storageState({ path: authStatePath('E-mail Não Verificado') });
});
