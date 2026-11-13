import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { createEntryRPC, entryRoute, listEntriesJSON, raceBody, spellBody } from './content-support';
import { refuseEntries, targetLeftTheMenu } from './left-out-support';
import { campaignWithEmptyPlayer } from './table-rules-support';
import { boxOf, gotoCampaignTab, newSignedInContext } from './support';

test.describe.configure({ timeout: 120_000 });

// Issue #736: the entries the rules refuse are left out of the campaign and marked, and the master sees which and why. The
// refusal is said by the answer of ListTableEntries (see left-out-support.ts); the archive is the real one.

const modifierRace = (): Record<string, unknown> => ({
  ...raceBody('Corujeiro'),
  traits: [{ namePt: 'Olhos de caçador', descPt: ['Enxergam longe.'], effects: [{ type: 'modifier', target: 'critical_range', mode: 'set', value: '19', applies: 'both' }] }],
});

async function table(browser: import('@playwright/test').Browser) {
  const master = await newSignedInContext(browser, 'Mestre Teste');
  const player = await newSignedInContext(browser, 'Jogador Teste');
  const m = await master.newPage();
  const p = await player.newPage();
  await Promise.all([m.goto('/'), p.goto('/')]);
  const campaignId = await campaignWithEmptyPlayer(m, p, `Mesa dos recusados ${Date.now()}`);
  const spell = await createEntryRPC(m, campaignId, 'tableSpell', spellBody('Lâmina de Nanquim'));
  const race = await createEntryRPC(m, campaignId, 'tableRace', modifierRace());
  const done = await refuseEntries(m, [
    { key: spell, field: 'table_spell.damage[0].dice', reason: 'bad_formula' },
    { key: race, field: 'table_race.traits[0].effects[0].target', reason: 'bad_value' },
  ], {
    // The target the rules no longer have: the stored code is not in the menu.
    [race]: (e) => targetLeftTheMenu(e),
  });
  return { master, player, m, p, campaignId, spell, race, done };
}

test(
  'o mestre vê as entradas fora da campanha, corrige uma e remove a outra @MR-025',
  { tag: ['@MR-025', '@RN-23'] },
  async ({ browser }) => {
    const { master, player, m, campaignId, race, spell, done } = await table(browser);
    try {
      await m.goto(`/campaigns/${campaignId}/content`);
      const panel = m.getByRole('region', { name: '2 entradas fora da campanha' });
      await expect(panel).toBeVisible();
      await expect(panel.getByRole('link', { name: 'Corujeiro' })).toBeVisible();
      await expect(panel.getByRole('link', { name: 'Lâmina de Nanquim' })).toBeVisible();
      await expect(panel).toContainText('este alvo não está mais no menu de efeitos');
      await expect(panel).toContainText('Esta fórmula não funciona');

      // The master fixes the race: the editor says why, up top and at the field, and the target is not in the menu.
      await panel.getByRole('link', { name: 'Corrigir' }).first().click();
      await expect(m.getByRole('heading', { level: 1, name: 'Corujeiro' })).toBeVisible();
      await expect(m.getByText('Esta raça está fora da campanha.')).toBeVisible();
      await expect(m.locator('.mr-tag--danger', { hasText: 'Fora da campanha' })).toBeVisible();
      await expect(m.getByText('Este alvo não está mais no menu de efeitos. Escolha outro.')).toBeVisible();
      const target = m.getByLabel('O que muda');
      await expect(target).toHaveValue('alvo_antigo');
      await expect(target.locator('option:checked')).toHaveText('Um alvo que saiu do menu');

      // Remover archives the spell and clears it from the panel; the real archive is in the table.
      await m.goto(`/campaigns/${campaignId}/content`);
      await m.getByRole('button', { name: 'Mais ações de Lâmina de Nanquim' }).click();
      await m.getByRole('menuitem', { name: 'Remover' }).click();
      await m.getByRole('dialog').getByRole('button', { name: 'Remover' }).click();
      await expect(m.getByRole('region', { name: '1 entrada fora da campanha' })).toBeVisible();
      await expect(m.getByRole('link', { name: 'Lâmina de Nanquim' })).toHaveCount(0);
      expect((await listEntriesJSON(m, campaignId)).entries!.find((e) => e.key === spell)?.archived).toBe(true);
      await m.getByRole('button', { name: 'Mais ações de Corujeiro' }).click();
      await m.getByRole('menuitem', { name: 'Remover' }).click();
      await m.getByRole('dialog').getByRole('button', { name: 'Remover' }).click();
      await expect(m.getByRole('region', { name: /entrada.* fora da campanha/ })).toHaveCount(0);
      expect((await listEntriesJSON(m, campaignId)).entries!.find((e) => e.key === race)?.archived).toBe(true);
    } finally {
      await done(); // a read the page started is answered before the page closes
      await master.close();
      await player.close();
    }
  },
);

test(
  'o mestre corrige o alvo, salva, e a raça volta para a campanha @MR-025',
  { tag: ['@MR-025', '@RN-23'] },
  async ({ browser }) => {
    const { master, player, m, campaignId, race, done } = await table(browser);
    try {
      await m.goto(entryRoute(campaignId, race));
      const target = m.getByLabel('O que muda');
      await expect(target).toHaveValue('alvo_antigo');
      await target.selectOption({ label: 'Margem de acerto crítico' });
      await m.getByRole('button', { name: 'Salvar raça' }).click();
      await expect(m.getByText('Corujeiro voltou para a campanha.')).toBeVisible();
      await expect(m.locator('.mr-tag--danger', { hasText: 'Fora da campanha' })).toHaveCount(0);
      await expect(m.getByText('Esta raça está fora da campanha.')).toHaveCount(0);
      // The list lost the race and kept the spell.
      await m.goto(`/campaigns/${campaignId}/content`);
      await expect(m.getByRole('region', { name: '1 entrada fora da campanha' })).toBeVisible();
    } finally {
      await done(); // a read the page started is answered before the page closes
      await master.close();
      await player.close();
    }
  },
);

test(
  'a campanha mostra o aviso ao mestre e leva à lista das entradas fora @MR-025',
  { tag: ['@MR-025', '@RN-23'] },
  async ({ browser }) => {
    const { master, player, m, campaignId, done } = await table(browser);
    try {
      await gotoCampaignTab(m, campaignId, 'Regras');
      const content = m.getByRole('region', { name: 'Conteúdo da mesa' });
      await expect(content).toContainText('2 entradas do conteúdo da mesa estão fora da campanha.');
      await content.getByRole('link', { name: 'Ver o conteúdo da mesa' }).click();
      await expect(m.getByRole('region', { name: '2 entradas fora da campanha' })).toBeVisible();

      // That a player never gets `left_out`, its count, a path or the name of a retired entry is not checked here: the refusal is
      // seeded in the master's page only, so a player's screen would be empty for a reason that proves nothing. The leak test does it
      // with a table that really has refused entries: backend/internal/leaktest, TestLeftOutIsTheMastersAlone.
    } finally {
      await done(); // a read the page started is answered before the page closes
      await master.close();
      await player.close();
    }
  },
);

test(
  'no celular a lista fora da campanha cabe numa tela e remove sem o editor @MR-025',
  { tag: ['@MR-025', '@RN-23'] },
  async ({ browser }) => {
    const { master, player, m, campaignId, spell, done } = await table(browser);
    try {
      await m.setViewportSize({ width: 390, height: 844 });
      await m.goto(`/campaigns/${campaignId}/content`);
      const panel = m.getByRole('region', { name: '2 entradas fora da campanha' });
      await expect(panel).toBeVisible();
      await expect(panel.getByText('Para corrigir, abra a entrada no computador.')).toBeVisible();
      await expect(panel.getByRole('link', { name: 'Corrigir' })).toHaveCount(0);
      expect(await m.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
      const box = await boxOf(panel);
      expect(box.y + box.height).toBeLessThanOrEqual(844);

      await m.getByRole('button', { name: 'Mais ações de Lâmina de Nanquim' }).click();
      await expect(m.getByRole('menuitem', { name: 'Exportar para corrigir num pacote' })).toHaveCount(0);
      await m.getByRole('menuitem', { name: 'Remover' }).click();
      await m.getByRole('dialog').getByRole('button', { name: 'Remover' }).click();
      await expect(m.getByRole('region', { name: '1 entrada fora da campanha' })).toBeVisible();
      expect((await listEntriesJSON(m, campaignId)).entries!.find((e) => e.key === spell)?.archived).toBe(true);
    } finally {
      await done(); // a read the page started is answered before the page closes
      await master.close();
      await player.close();
    }
  },
);
