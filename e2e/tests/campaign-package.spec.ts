import fs from 'node:fs';

import { expect, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { exportThroughScreen, joinAsPlayer, tableForPackage, zipWithManifest } from './campaign-package-support';
import { expectLoaded } from './loaded';
import { authStatePath, callRPC, gotoCampaignTab, newSignedInContext, waitForCampaignList, openCampaignTab } from './support';

// MR-050, the campaign package (web/src/app/pages/campaign-export and
// campaigns/campaign-import), against the real CampaignPackageService. Every
// test builds its own campaign through the API; signing in is never the
// point, so every context reuses a saved state (the default page is the
// master's).
test.use({ storageState: authStatePath('Mestre Teste') });

const packageInput = (page: Page) => page.getByLabel('Arquivo do pacote da campanha');
const countOf = (page: Page, label: string) => page.locator('.counts__row', { has: page.getByText(label, { exact: true }) }).locator('dd');

test(
  'um jogador da campanha não tem o link de exportar, a página diz que só o mestre exporta, e o JSON dele não traz a exportação',
  { tag: ['@MR-050', '@RN-10'] },
  async ({ page, browser }) => {
    await page.goto('/');
    const table = await tableForPackage(page, `Pacote jogador ${Date.now()}`);
    await exportThroughScreen(page, table.campaignId);

    const context = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const player = await context.newPage();
      await player.goto('/');
      await joinAsPlayer(page, player, table.campaignId);

      await player.goto(`/campaigns/${table.campaignId}`);
      await expect(player.getByRole('heading', { level: 1, name: table.campaignName })).toBeVisible();
      await expectLoaded(player);
      await expect(player.getByRole('heading', { name: 'Pacote da campanha' })).toHaveCount(0);
      await expect(player.getByRole('link', { name: 'Exportar campanha' })).toHaveCount(0);

      await player.goto(`/campaigns/${table.campaignId}/export`);
      await expect(player.getByText('Só o mestre exporta a campanha.')).toBeVisible();
      await expect(player.getByRole('button', { name: 'Exportar campanha' })).toHaveCount(0);
      await expect(player.getByRole('link', { name: 'Baixar de novo' })).toHaveCount(0);

      // The server says the same: no export of the master's reaches the player.
      const exported = await callRPC(player, 'meurpg.campaignpackage.v1.CampaignPackageService/GetCampaignExport', {
        campaignId: table.campaignId,
      });
      expect(exported.ok()).toBe(false);
      const body = await exported.text();
      expect(body).not.toContain('.meurpg.zip');
      expect(body).not.toContain('downloadPath');
    } finally {
      await context.close();
    }
  },
);

// The server keeps one import upload per account at a time (a new file drops the first one), so the tests that import
// run one after the other, in this order. The a11y scans import as other accounts (a11y.spec.ts, scanPackage).
test.describe('importar um pacote', () => {
  test.describe.configure({ mode: 'serial' });

  test(
    'o mestre exporta a campanha, baixa o pacote, importa o arquivo e abre a campanha nova com o mapa',
    { tag: '@MR-050' },
    async ({ page }, testInfo) => {
      test.slow();
      await page.goto('/');
      const table = await tableForPackage(page, `Pacote ${Date.now()}`);

      // The campaign page links to the export.
      await gotoCampaignTab(page, table.campaignId, 'Regras');
      const panel = page.getByRole('region', { name: 'Pacote da campanha' });
      await panel.getByRole('link', { name: 'Exportar campanha' }).click();
      await expect(page).toHaveURL(`/campaigns/${table.campaignId}/export`);

      await exportThroughScreen(page, table.campaignId);
      const last = page.getByRole('region', { name: 'Último pacote' });
      await expect(last.getByText(/\.meurpg\.zip/)).toBeVisible();

      // The download: a zip, named .meurpg.zip.
      const downloading = page.waitForEvent('download');
      await last.getByRole('link', { name: 'Baixar de novo' }).click();
      const download = await downloading;
      expect(download.suggestedFilename()).toMatch(/\.meurpg\.zip$/);
      const file = testInfo.outputPath(download.suggestedFilename());
      await download.saveAs(file);
      expect(fs.readFileSync(file).subarray(0, 2).toString('latin1')).toBe('PK');

      // Back on the campaigns list: "Importar campanha", then the file.
      await page.goto('/campaigns');
      await waitForCampaignList(page);
      await page.getByRole('link', { name: 'Importar campanha' }).click();
      await expect(page).toHaveURL('/campaigns/import');
      await expect(page.getByRole('heading', { level: 2, name: 'Importar campanha' })).toBeVisible();
      await packageInput(page).setInputFiles(file);

      const preview = page.getByRole('heading', { level: 2, name: 'Prévia do pacote' });
      await expect(preview).toBeVisible({ timeout: 60_000 });
      await expect(page.getByRole('progressbar')).toHaveCount(0);
      await expect(page.getByText(table.campaignName)).toBeVisible();
      await expect(countOf(page, 'Mapas')).toHaveText('1');
      await expect(countOf(page, 'Imagens')).toHaveText(/^1 \(/);
      await expect(countOf(page, 'NPCs e criaturas')).toHaveText('1');
      await expect(countOf(page, 'Cenas')).toHaveText('1');
      await expect(page.getByText('Tudo pode ser criado.')).toBeVisible();

      await page.getByRole('button', { name: 'Criar campanha' }).click();
      await expect(page.getByRole('heading', { level: 2, name: 'Campanha criada' })).toBeVisible({ timeout: 60_000 });
      await page.getByRole('link', { name: 'Abrir a campanha' }).click();

      await expect(page).toHaveURL(/\/campaigns\/[^/]+$/);
      expect(page.url()).not.toContain(table.campaignId);
      await expect(page.getByRole('heading', { level: 1 })).toContainText(table.campaignName);
      // The imported campaign opens on "Mesa"; its maps are on "Preparação".
      await openCampaignTab(page, 'Preparação');
      await expect(page.getByRole('region', { name: 'Mapas' }).getByText(table.mapName).first()).toBeVisible();
    },
  );

  test('um pacote de versão mais nova é recusado em palavras, sem "Criar campanha"', { tag: '@MR-050' }, async ({ page }) => {
    await page.goto('/campaigns/import');
    await expect(page.getByRole('heading', { level: 2, name: 'Importar campanha' })).toBeVisible();
    await packageInput(page).setInputFiles({
      name: 'futuro.meurpg.zip',
      mimeType: 'application/zip',
      buffer: zipWithManifest({ format_version: 2 }),
    });

    await expect(
      page.getByText('Este pacote é de uma versão mais nova do MeuRPG, e este servidor ainda não sabe abri-lo.'),
    ).toBeVisible({ timeout: 60_000 });
    await expect(page.getByRole('button', { name: 'Criar campanha' })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Escolher outro arquivo' })).toBeVisible();
  });


  test('um arquivo que não é zip recebe a recusa do app, e dá para escolher outro', { tag: '@MR-050' }, async ({ page }) => {
    await page.goto('/campaigns/import');
    await packageInput(page).setInputFiles({
      name: 'notas.txt',
      mimeType: 'text/plain',
      buffer: Buffer.from('isto não é um pacote'),
    });

    await expect(
      page.getByRole('alert').filter({ hasText: 'Esse arquivo não parece um pacote de campanha. Ele precisa terminar em .meurpg.zip (ou .zip).' }),
    ).toBeVisible();
    await expect(page.getByRole('button', { name: 'Criar campanha' })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Escolher o arquivo' })).toBeVisible();

    // A zip name with something else inside is judged by the server.
    await packageInput(page).setInputFiles({
      name: 'falso.meurpg.zip',
      mimeType: 'application/zip',
      buffer: Buffer.from('isto também não é um zip, só tem o nome'),
    });
    await expect(page.getByRole('heading', { level: 2, name: 'Prévia do pacote' })).toBeVisible({ timeout: 60_000 });
    await expect(page.getByRole('strong').filter({ hasText: 'Este pacote não pode ser criado.' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Criar campanha' })).toHaveCount(0);
  });


  test('no celular de 320 × 568 as duas páginas cabem sem rolagem horizontal', { tag: '@MR-050' }, async ({ browser }) => {
    const context = await browser.newContext({
      storageState: authStatePath('Mestre Teste'),
      viewport: { width: 320, height: 568 },
    });
    try {
      const page = await context.newPage();
      await page.goto('/');
      const table = await tableForPackage(page, `Pacote celular ${Date.now()}`);
      const overflow = () => page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);

      await exportThroughScreen(page, table.campaignId);
      await expect(page.getByRole('link', { name: 'Baixar de novo' })).toBeVisible();
      expect(await overflow()).toBeLessThanOrEqual(0);

      await page.goto('/campaigns/import');
      await expect(page.getByRole('button', { name: 'Escolher o arquivo' })).toBeVisible();
      expect(await overflow()).toBeLessThanOrEqual(0);

      await packageInput(page).setInputFiles({
        name: 'futuro.meurpg.zip',
        mimeType: 'application/zip',
        buffer: zipWithManifest({ format_version: 2 }),
      });
      await expect(page.getByRole('strong').filter({ hasText: 'Este pacote é de uma versão mais nova do MeuRPG' })).toBeVisible({ timeout: 60_000 });
      expect(await overflow()).toBeLessThanOrEqual(0);
    } finally {
      await context.close();
    }
  });
});
