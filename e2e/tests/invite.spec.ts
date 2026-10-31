import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { createCampaign, idpOrigin, newSignedInContext } from './support';

// MR-003 (entrar pelo convite): a second person, already signed in, opens
// the invite link and joins; the master sees them in the members list.
//
// Every context in this file is explicit about its own auth state — no
// file-level `test.use({ storageState })` here, on purpose: this file's
// second test needs one context that is genuinely signed out, and
// `browser.newContext()` inherits whatever `storageState` the running
// test is configured with (project `use` / `test.use`) unless the call
// overrides it — confirmed live: a file-level `test.use({storageState:
// authStatePath('Mestre Teste')})` here once made `browser.newContext()`
// (no args) come back already signed in as the master, breaking the
// signed-out guest flow below in a way that was easy to miss (the cookies
// were there; only the button never rendered). Every context below states
// its own state explicitly instead, so there is nothing to inherit.

test('jogador já logado abre o link do convite, entra na campanha e o mestre o vê nos membros', { tag: '@MR-003' }, async ({ browser }) => {
  // Both reuse a saved state (auth.setup.ts): this test's story is the
  // invite, not signing in.
  const masterContext = await newSignedInContext(browser, 'Mestre Teste');
  try {
    const page = await masterContext.newPage();
    const campaignId = await createCampaign(page, `Mesa conjunta ${Date.now()}`);
    await page.getByRole('button', { name: 'Gerar convite' }).click();
    await expect(page.getByText('não será mostrado de novo')).toBeVisible();
    const link = (await page.locator('.invite-reveal__link').textContent())?.trim();
    expect(link).toBeTruthy();

    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const playerPage = await playerContext.newPage();
      await playerPage.goto(link!);
      // AcceptInvite runs as soon as the page loads (already signed in), then
      // the app navigates to the campaign.
      await expect(playerPage).toHaveURL(new RegExp(`/campaigns/${campaignId}$`));
      await expect(playerPage.getByRole('heading', { level: 1 })).toContainText('Mesa conjunta');
    } finally {
      await playerContext.close();
    }

    // Back on the master's page: reload to see the new member. `exact:
    // true`: the campaign page's "Sessão" card (Etapa 4) also says "trava
    // a ficha dos jogadores", which a loose substring match against
    // "jogador" hits too.
    await page.reload();
    await expect(page.getByText('jogador', { exact: true })).toBeVisible();
  } finally {
    await masterContext.close();
  }
});

// The sign-in-through-invite path: the person opens the invite link signed
// out, clicks "Entrar para aceitar o convite", picks a devidp user, and lands
// on the campaign, joined. The page posts the token in the body of POST
// /auth/login (intent=campaign_invite); the server keeps only its hash in the
// login state and accepts the invite after the callback. The token must never
// appear in any URL the browser requests.
//
// The guest's sign-in below is this test's whole point, so it's the one
// real /auth/login hit here — the master's context still reuses its saved
// state (see the file-level note above for why that's a *separate*,
// explicit context here rather than the default `page` fixture).
test(
  'visitante sem sessão entra pelo convite, faz login e é adicionado à campanha automaticamente',
  { tag: '@MR-003' },
  async ({ browser }) => {
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    try {
      const page = await masterContext.newPage();
      const campaignId = await createCampaign(page, `Convite sem sessão ${Date.now()}`);
      await page.getByRole('button', { name: 'Gerar convite' }).click();
      await expect(page.getByText('não será mostrado de novo')).toBeVisible();
      const link = (await page.locator('.invite-reveal__link').textContent())?.trim();

      // No storageState at all: genuinely signed out, unlike masterContext
      // above.
      const guestContext = await browser.newContext();
      try {
        const guestPage = await guestContext.newPage();
        const token = new URL(link!).hash.replace(/^#t=/, '');
        expect(token.length).toBeGreaterThan(0);
        const requestedURLs: string[] = [];
        guestPage.on('request', (request) => requestedURLs.push(request.url()));

        await guestPage.goto(link!);
        await guestPage.getByRole('button', { name: 'Entrar para aceitar o convite' }).click();

        await expect(guestPage).toHaveURL((url) => url.origin === idpOrigin && url.pathname === '/authorize');
        await guestPage.getByRole('button', { name: 'Jogador Teste', exact: true }).click();

        await expect(guestPage).toHaveURL(new RegExp(`/campaigns/${campaignId}$`));
        // Playwright reports the URL without the fragment, so the invite page's
        // own URL does not count; any other appearance would be a leak.
        expect(requestedURLs.filter((url) => url.includes(token))).toEqual([]);
      } finally {
        await guestContext.close();
      }

      // Back on the master's page: the guest is now a member.
      await page.reload();
      await expect(page.getByText('jogador', { exact: true })).toBeVisible();
    } finally {
      await masterContext.close();
    }
  },
);
