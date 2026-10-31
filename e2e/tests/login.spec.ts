import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { day, getMe, loginCookie, sessionCookie, signIn, signOut } from './support';

// The game master's sign-in, through devidp standing in for Google. MR-001
// and every story after it start with "Dado que estou logado": these tests
// prove that step. The server-side rules behind them are covered in Go
// (backend/internal/identity); here they are checked in a real browser.

test.describe('login do mestre', () => {
  test('entra pelo provedor OIDC local e o GetMe devolve a conta', { tag: '@MR-001' }, async ({ page }) => {
    await signIn(page, 'Mestre Teste', '/');

    const res = await getMe(page);
    expect(res.status()).toBe(200);
    const me = await res.json();
    expect(me.user.id).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/);
    // The session lasts 30 days from the sign-in, never more.
    const left = Date.parse(me.sessionExpiresAt) - Date.now();
    expect(left).toBeGreaterThan(30 * day - 5 * 60_000);
    expect(left).toBeLessThanOrEqual(30 * day);
    // The verified e-mail is only a security contact: GetMe never returns it.
    expect(JSON.stringify(me)).not.toContain('@');
  });

  test('cada pessoa tem uma conta, a mesma em todo login', { tag: '@MR-001' }, async ({ browser }) => {
    const accountOf = async (user: 'Mestre Teste' | 'Jogador Teste') => {
      const context = await browser.newContext();
      try {
        const page = await context.newPage();
        await signIn(page, user);
        return (await (await getMe(page)).json()).user.id as string;
      } finally {
        await context.close();
      }
    };

    const mestre = await accountOf('Mestre Teste');
    expect(await accountOf('Mestre Teste')).toBe(mestre);
    expect(await accountOf('Jogador Teste')).not.toBe(mestre);
  });

  test('sair encerra a sessão', { tag: '@MR-001' }, async ({ page, context }) => {
    await signIn(page);
    expect((await getMe(page)).status()).toBe(200);

    expect((await signOut(page)).status()).toBe(200);
    expect(await context.cookies()).not.toContainEqual(expect.objectContaining({ name: sessionCookie }));

    const after = await getMe(page);
    expect(after.status()).toBe(401);
    expect(await after.json()).toMatchObject({ code: 'unauthenticated' });
  });

  test('o cookie de sessão tem as habilidades certos', async ({ page, context, baseURL }) => {
    const signedInAt = Date.now();
    await signIn(page);

    const cookies = await context.cookies(baseURL);
    const session = cookies.find((c) => c.name === sessionCookie);
    expect(session).toMatchObject({
      // No leading dot: a host-only cookie, which __Host- requires.
      domain: new URL(baseURL!).hostname,
      path: '/',
      secure: true,
      httpOnly: true,
      sameSite: 'Lax',
    });
    expect(session!.expires * 1000).toBeGreaterThan(signedInAt + 30 * day - 5 * 60_000);
    expect(session!.expires * 1000).toBeLessThanOrEqual(Date.now() + 30 * day + 1_000);
    // The login cookie is single use: the callback deleted it.
    expect(cookies.map((c) => c.name)).not.toContain(loginCookie);
    // HttpOnly: the page's JavaScript cannot read the session.
    expect(await page.evaluate(() => document.cookie)).not.toContain(sessionCookie);
    // But the browser itself sends it back, even over plain http: Chrome
    // treats http://localhost as a secure context, so a Secure (and
    // __Host-) cookie works there.
    const status = await page.evaluate(async () => {
      const res = await fetch('/meurpg.identity.v1.IdentityService/GetMe', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Connect-Protocol-Version': '1' },
        body: '{}',
      });
      return res.status;
    });
    expect(status).toBe(200);
  });

  test('return_to para outro site é recusado', async ({ page, baseURL }) => {
    for (const returnTo of ['https://evil.example/', '//evil.example', '/\\evil.example', 'javascript:alert(1)']) {
      const res = await page.request.get(`/auth/login?return_to=${encodeURIComponent(returnTo)}`, { maxRedirects: 0 });
      expect(res.status(), returnTo).toBe(400);
      expect(res.headers()['location'], returnTo).toBeUndefined();
    }

    // In the browser, too: no trip to the provider, nor anywhere else.
    const res = await page.goto(`/auth/login?return_to=${encodeURIComponent('//evil.example')}`);
    expect(res?.status()).toBe(400);
    expect(new URL(page.url()).origin).toBe(new URL(baseURL!).origin);
  });
});
