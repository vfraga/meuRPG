import { expect } from '@playwright/test';
import { test } from './diagnostics';

// The terms of use and the privacy policy are public: the Google consent screen links to them, so
// they must open with no session. The footer reaches them from every page, and the site asks search
// engines not to index it (robots.txt, the robots meta tag).

test('os termos e a privacidade abrem sem entrar, com a data e o aviso de que não houve revisão jurídica', { tag: '@legal' }, async ({ page }) => {
  await page.goto('/terms');
  await expect(page).toHaveTitle('Termos de uso · MeuRPG');
  await expect(page.getByRole('heading', { level: 1, name: 'Termos de uso' })).toBeVisible();
  await expect(page.getByText('Última atualização: 09/10/2026')).toBeVisible();
  await expect(page.getByText('Ainda não foram revisados por um advogado.')).toBeVisible();
  await expect(page.getByRole('heading', { level: 2, name: '1. O que é o MeuRPG' })).toBeVisible();
  await expect(page).toHaveURL(/\/terms$/);

  await page.goto('/privacy');
  await expect(page).toHaveTitle('Política de privacidade · MeuRPG');
  await expect(page.getByRole('heading', { level: 1, name: 'Política de privacidade' })).toBeVisible();
  await expect(page.getByText('Última atualização: 24/10/2026')).toBeVisible();
  await expect(page.getByText('Ainda não foi revisado por um advogado.')).toBeVisible();
  await expect(page.getByRole('heading', { level: 2, name: '3. Os dados da sua conta Google' })).toBeVisible();
  await expect(page).toHaveURL(/\/privacy$/);
});

test('o rodapé leva aos termos, à privacidade e aos créditos, e marca a página aberta', { tag: '@legal' }, async ({ page }) => {
  await page.goto('/');
  const footer = page.getByRole('navigation', { name: 'Rodapé' });
  await expect(page.getByRole('contentinfo')).toContainText(`© ${new Date().getFullYear()} MeuRPG`);

  await footer.getByRole('link', { name: 'Termos de uso' }).click();
  await expect(page).toHaveURL(/\/terms$/);
  await expect(footer.getByRole('link', { name: 'Termos de uso' })).toHaveAttribute('aria-current', 'page');

  await footer.getByRole('link', { name: 'Privacidade' }).click();
  await expect(page).toHaveURL(/\/privacy$/);
  await expect(footer.getByRole('link', { name: 'Privacidade' })).toHaveAttribute('aria-current', 'page');

  await footer.getByRole('link', { name: 'Créditos' }).click();
  await expect(page).toHaveURL(/\/credits$/);
});

test('o índice leva ao título da seção e o link com âncora abre na seção', { tag: '@legal' }, async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto('/privacy');
  await page.getByRole('navigation', { name: 'Nesta página' }).getByRole('link', { name: /Cookies/ }).click();
  await expect(page).toHaveURL(/\/privacy#cookies$/);
  await expect(page.getByRole('heading', { level: 2, name: '6. Cookies' })).toBeFocused();
  await expect(page.getByRole('heading', { level: 2, name: '6. Cookies' })).toBeInViewport();

  await page.goto('/terms#lei');
  await expect(page.getByRole('heading', { level: 2, name: '11. Lei aplicável' })).toBeInViewport();
});

test('o site pede aos buscadores que não o indexem e serve o ícone e o manifesto', { tag: '@legal' }, async ({ page, request }) => {
  await page.goto('/');
  await expect(page.locator('meta[name="robots"]')).toHaveAttribute('content', 'noindex, nofollow');

  const robots = await request.get('/robots.txt');
  expect(robots.ok()).toBeTruthy();
  expect(await robots.text()).toBe('User-agent: *\nDisallow: /\n');

  const manifest = await request.get('/site.webmanifest');
  expect(manifest.headers()['content-type']).toContain('application/manifest+json');
  expect((await manifest.json()).short_name).toBe('MeuRPG');

  for (const [path, type] of [
    ['/favicon.ico', 'image/x-icon'],
    ['/favicon.svg', 'image/svg+xml'],
    ['/apple-touch-icon-180.png', 'image/png'],
    ['/icon-192.png', 'image/png'],
    ['/icon-512.png', 'image/png'],
    ['/icon-maskable-512.png', 'image/png'],
  ]) {
    const response = await request.get(path);
    expect(response.ok(), path).toBeTruthy();
    expect(response.headers()['content-type'], path).toContain(type);
  }
});
