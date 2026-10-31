import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { authStatePath, createCampaign, waitForCampaignList } from './support';

// MR-001 (criar campanha) and MR-002 (gerar convite), by the screen, as the
// master would use it. The underlying rules already have Go tests
// (TestMR001_*, TestMR002_*, backend/internal/campaigns); these prove the
// same criteria hold through the UI.
//
// Neither test's own story is signing in, so both reuse the master's saved
// state (auth.setup.ts) instead of a fresh /auth/login.
test.use({ storageState: authStatePath('Mestre Teste') });

test.describe('criar campanha', () => {
  test('o mestre cria uma campanha pela tela e a vê como mestre na lista', { tag: '@MR-001' }, async ({ page }) => {
    await page.goto('/campaigns');
    const name = `Mirathel ${Date.now()}`;

    await expect(page.getByRole('heading', { name: 'Minhas campanhas' })).toBeVisible();
    await waitForCampaignList(page);
    await page.getByLabel('Nome da campanha').fill(name);
    await page.getByRole('combobox', { name: 'Modo de XP' }).click();
    await page.getByRole('option', { name: 'Por inimigos derrotados' }).click();
    await page.getByRole('button', { name: 'Criar campanha' }).click();

    // MR-001's criterion: creating it makes the caller its master.
    await expect(page).toHaveURL(/\/campaigns\/[^/]+$/);
    await expect(page.getByRole('heading', { name, level: 1 })).toBeVisible();
    await expect(page.getByText('Você é mestre nesta campanha')).toBeVisible();

    // And MR-001's other half: it shows up in "Minhas campanhas", tagged
    // "Mestre".
    await page.goto('/campaigns');
    const item = page.getByRole('link', { name });
    await expect(item).toBeVisible();
    await expect(item).toContainText('Mestre');
  });
});

test.describe('convites', () => {
  test('o mestre gera um convite, vê o link uma vez e o revoga', { tag: '@MR-002' }, async ({ page }) => {
    await createCampaign(page, `Convites ${Date.now()}`);

    await expect(page.getByRole('heading', { name: 'Convites', level: 2 })).toBeVisible();
    // Nothing generated yet: no status text and no revoke button.
    await expect(page.getByRole('button', { name: 'Revogar' })).toHaveCount(0);

    await page.getByRole('button', { name: 'Gerar convite' }).click();

    // The link is shown exactly this once, with a clear warning.
    await expect(page.getByText('não será mostrado de novo')).toBeVisible();
    const linkText = (await page.locator('.invite-reveal__link').textContent())?.trim();
    expect(linkText).toMatch(/\/invite#t=.+/);

    // It shows up in the list, active.
    await expect(page.getByText('ativo')).toBeVisible();

    // Dismissing the one-time banner does not affect the list.
    await page.getByRole('button', { name: 'Ok, guardei o link' }).click();
    await expect(page.getByText('não será mostrado de novo')).toHaveCount(0);
    await expect(page.getByText('ativo')).toBeVisible();

    // Revoking stops it from working (the invite list reflects that
    // immediately) and there is nothing left to revoke.
    await page.getByRole('button', { name: 'Revogar' }).click();
    await expect(page.getByText('revogado')).toBeVisible();
    await expect(page.getByText('ativo')).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Revogar' })).toHaveCount(0);
  });
});
