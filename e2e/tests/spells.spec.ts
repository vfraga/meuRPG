import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { archiveEntryRPC, createInkBladeRPC, listSpellsRPC, tableForSpells } from './spells-support';
import { callRPC, newSignedInContext } from './support';

// MR-045 (the players' "Magias" page, slice 10.11b), RN-23 (what players see) and RN-10's habit: the
// page asks the server and draws what it answers. Setup goes through the API; every test makes its own
// campaign.

test(
  'o jogador procura "maos" e acha Mãos Flamejantes com o alvo "Cone de 4,5 m"',
  { tag: ['@MR-045'] },
  async ({ browser }) => {
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const master = await masterContext.newPage();
      const player = await playerContext.newPage();
      await master.goto('/');
      await player.goto('/');
      const table = await tableForSpells(master, player, `Mirathel ${Date.now()}`);

      // The way in: the campaign page has the panel, for a player too.
      await player.goto(`/campaigns/${table.campaignId}`);
      await expect(player.getByRole('heading', { name: 'Magias', level: 2 })).toBeVisible();
      await player.getByRole('link', { name: 'Abrir as magias' }).click();
      await expect(player.getByRole('heading', { name: 'Magias', level: 1 })).toBeVisible();

      const server = await listSpellsRPC(player, table.campaignId, { query: 'maos' });
      expect(server.spells.map((s) => s.namePt)).toContain('Mãos Flamejantes');
      await player.getByRole('searchbox', { name: 'Buscar pelo nome' }).fill('maos');
      await expect(player.locator('.list__n')).toContainText(`${server.total} ${server.total === 1 ? 'magia' : 'magias'}`);
      await expect(player.locator('button.row .row__name')).toHaveText(server.spells.map((s) => s.namePt));

      await player.locator('button.row', { hasText: 'Mãos Flamejantes' }).click();
      const facts = player.locator('.spell__facts');
      await expect(player.locator('#spell-card-title')).toHaveText('Mãos Flamejantes');
      await expect(facts.locator('.spell__fact', { hasText: 'Alvo' })).toContainText('Cone de 4,5 m');
      await expect(player.getByText('Texto do SRD 5.1; a tradução para o português é nossa.')).toBeVisible();
      // An SRD spell's text is our Portuguese translation; "Ver em inglês" flips to the SRD's own, marked lang=en.
      await expect(player.locator('.spell__prose .srd[lang=en]')).toHaveCount(0);
      await player.getByRole('button', { name: 'Ver em inglês' }).click();
      await expect(player.locator('.spell__prose .srd[lang=en]').first()).toBeVisible();
      await expect(player.getByText('(em inglês)')).toBeVisible();
      await player.getByRole('button', { name: 'Ver em português' }).click();
      await expect(player.locator('.spell__prose .srd[lang=en]')).toHaveCount(0);

      // Nothing matches: the word is in the sentence, and the way out is a button.
      await player.getByRole('searchbox', { name: 'Buscar pelo nome' }).fill('zzzz');
      await expect(player.getByText('Nenhuma magia com “zzzz”.')).toBeVisible();
      await player.getByRole('button', { name: 'Limpar a busca' }).click();
      await expect(player.locator('button.row').first()).toBeVisible();
    } finally {
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  '"Só as que posso aprender" de um Mago de nível 1 lista só truques e magias do 1º nível da lista do Mago',
  { tag: ['@MR-045'] },
  async ({ browser }) => {
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const master = await masterContext.newPage();
      const player = await playerContext.newPage();
      await master.goto('/');
      await player.goto('/');
      const table = await tableForSpells(master, player, `Mirathel ${Date.now()}`);

      // The truth, from another source: the whole catalog's wizard spells up to the 1st circle.
      const content = await callRPC(player, 'meurpg.rules.v1.ContentService/ListContent', { campaignId: table.campaignId });
      const catalog = ((await content.json()).content.spells ?? []) as { level?: number; classKeys?: string[] }[];
      const expected = catalog.filter((s) => (s.classKeys ?? []).includes('class:wizard') && (s.level ?? 0) <= 1).length;
      expect(expected).toBeGreaterThan(20);

      await player.goto(`/campaigns/${table.campaignId}/spells`);
      // At 1280 px the filters are in the panel.
      await player.getByRole('switch', { name: 'Só as que posso aprender' }).click();
      await expect(player.getByRole('switch', { name: 'Só as que posso aprender' })).toHaveAttribute('aria-checked', 'true');
      await expect(player.locator('.list__n')).toContainText(`${expected} magias`);

      // Page through every row: only truques and 1º nível.
      const rows = player.locator('button.row');
      const more = player.getByRole('button', { name: 'Mostrar mais' });
      while (await more.isVisible()) {
        const before = await rows.count();
        await more.click();
        await expect.poll(() => rows.count()).toBeGreaterThan(before);
      }
      await expect(player.locator('button.row')).toHaveCount(expected);
      const subs = await player.locator('button.row .row__sub').allTextContents();
      expect(subs.every((s) => /^(Truque|1º nível)/.test(s.trim()))).toBe(true);
      expect(subs.some((s) => s.startsWith('Truque'))).toBe(true);
      expect(subs.some((s) => s.startsWith('1º nível'))).toBe(true);

      // The server agrees with the screen.
      const mine = await listSpellsRPC(player, table.campaignId, { characterId: table.characterId });
      expect(mine.total).toBe(expected);
      expect(player.url()).toContain('mine=1');
    } finally {
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'uma magia da mesa que o mestre arquivou some para o jogador (na tela e no JSON) e aparece com "Arquivada" para o mestre',
  { tag: ['@MR-045', '@RN-23'] },
  async ({ browser }) => {
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const master = await masterContext.newPage();
      const player = await playerContext.newPage();
      await master.goto('/');
      await player.goto('/');
      const table = await tableForSpells(master, player, `Mirathel ${Date.now()}`);
      const key = await createInkBladeRPC(master, table.campaignId);

      // Live: the player sees it, with "Da mesa", and reads the master's own text in Portuguese.
      await player.goto(`/campaigns/${table.campaignId}/spells?q=nanquim`);
      const row = player.locator('button.row', { hasText: 'Lâmina de Nanquim' });
      await expect(row).toBeVisible();
      await expect(row).toContainText('Da mesa');
      await row.click();
      await expect(player.locator('#spell-card-title')).toHaveText('Lâmina de Nanquim');
      await expect(player.getByText('Um risco de tinta negra corta o ar e rasga o alvo.')).toBeVisible();
      await expect(player.locator('.spell__fact', { hasText: 'Alvo' })).toContainText('Uma criatura');
      await expect(player.locator('.spell__fact', { hasText: 'Dano' })).toContainText('2d8 necrótico');
      await expect(player.getByText('Texto do SRD 5.1; a tradução para o português é nossa.')).toHaveCount(0);
      await expect(player.getByRole('button', { name: 'Ver em inglês' })).toHaveCount(0);

      await archiveEntryRPC(master, table.campaignId, key);

      // Archived: gone for the player, on the screen and in the JSON.
      expect((await listSpellsRPC(player, table.campaignId, { query: 'nanquim' })).spells).toEqual([]);
      // The spell's key (not a slug of its name) is what must be missing from every answer.
      expect(JSON.stringify(await listSpellsRPC(player, table.campaignId))).not.toContain(key);
      expect(JSON.stringify(await listSpellsRPC(player, table.campaignId))).not.toContain('Nanquim');
      await player.goto(`/campaigns/${table.campaignId}/spells?q=nanquim`);
      await expect(player.getByText('Nenhuma magia com “nanquim”.')).toBeVisible();
      await expect(player.locator('button.row')).toHaveCount(0);

      // The master still reads it, marked "Arquivada".
      const asMaster = await listSpellsRPC(master, table.campaignId, { query: 'nanquim' });
      expect(asMaster.spells.map((s) => [s.key, s.archived])).toEqual([[key, true]]);
      await master.goto(`/campaigns/${table.campaignId}/spells?q=nanquim`);
      const archived = master.locator('button.row', { hasText: 'Lâmina de Nanquim' });
      await expect(archived).toContainText('Arquivada');
      await expect(archived).toContainText('Da mesa');
    } finally {
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'no celular, a magia é um passo do histórico: o Voltar do navegador fecha a magia e traz a mesma lista, com a mesma busca',
  { tag: ['@MR-045'] },
  async ({ browser }) => {
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 390, height: 844 } });
    try {
      const master = await masterContext.newPage();
      const player = await playerContext.newPage();
      await master.goto('/');
      await player.goto('/');
      const table = await tableForSpells(master, player, `Mirathel ${Date.now()}`);
      await player.goto(`/campaigns/${table.campaignId}/spells?q=escudo`);
      const rows = player.locator('button.row');
      await expect(rows.first()).toBeVisible();
      await rows.first().click();
      await expect(player.locator('#spell-card-title')).toHaveText('Escudo Arcano');
      await expect(rows).toHaveCount(0);
      expect(player.url()).toContain('spell=');

      // The browser's Back closes the spell: the same search, the row that was open has the focus.
      await player.goBack();
      await expect(rows.first()).toBeVisible();
      expect(player.url()).not.toContain('spell=');
      expect(player.url()).toContain('q=escudo');
      await expect(rows.first()).toBeFocused();

      // "Voltar para Magias" does the same, and Forward opens the spell again.
      await rows.first().click();
      await player.getByRole('button', { name: 'Voltar para Magias' }).click();
      await expect(rows.first()).toBeVisible();
      await expect(player.getByRole('searchbox', { name: 'Buscar pelo nome' })).toHaveValue('escudo');
      await player.goForward();
      await expect(player.locator('#spell-card-title')).toHaveText('Escudo Arcano');
    } finally {
      await masterContext.close();
      await playerContext.close();
    }
  },
);
