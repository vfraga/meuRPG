import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { castSheet, openCastOf } from './casting-support';
import { listCreaturesRPC, tableForCreatures } from './creatures-support';
import { openSessionPage } from './live-session-support';
import { newSignedInContext } from './support';

// MR-048 (casting outside a combat) and MR-037 (the character's creatures): Convocar Familiar from "Conjurar" has
// no target; the creature sheet picks the form and casts the ritual, and the familiar lands on the sheet.

test(
  'Pensantus abre Conjurar, escolhe Convocar Familiar, não vê alvo, escolhe a criatura e o familiar aparece em Criaturas (jogadora e mestre)',
  { tag: ['@MR-048', '@MR-037'] },
  async ({ browser }) => {
    test.setTimeout(180_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    try {
      await master.goto('/');
      await player.goto('/');
      const table = await tableForCreatures(master, player, `Conjurar Familiar ${Date.now()}`);

      await openSessionPage(player, table.campaignId);
      const sheet = await openCastOf(player, 'Convocar Familiar');
      await expect(sheet.getByText('traz uma criatura: ela não tem alvo')).toBeVisible();
      await expect(sheet.getByText('Quem recebe')).toHaveCount(0);
      await expect(sheet.getByText(/Quem (recebe|você toca)/)).toHaveCount(0);

      await sheet.getByRole('button', { name: 'Escolher a criatura' }).click();
      const summon = player.locator('mat-dialog-container[aria-labelledby="summon-t"]');
      await summon.getByLabel('Nome do familiar').fill('Nanquim');
      await summon.locator('label', { hasText: /Corvo/ }).click();
      await expect(summon.getByText('Conjurar como ritual · 1 hora · sem gastar espaço')).toBeVisible();
      await summon.getByRole('button', { name: 'Convocar o familiar' }).click();
      await expect(summon).toBeHidden();
      await expect(castSheet(player).getByText(/Nanquim chegou\./)).toBeVisible();

      const creatures = await listCreaturesRPC(player, table.campaignId, table.characterId);
      expect(creatures.map((c) => c.name)).toEqual(['Nanquim']);

      // On the sheet, under "Criaturas", for the player and for the master.
      for (const page of [player, master]) {
        await page.goto(`/campaigns/${table.campaignId}/characters/${table.characterId}`);
        const panel = page.locator('app-creatures-panel');
        await expect(panel.getByRole('heading', { name: 'Criaturas', level: 2 })).toBeVisible();
        await expect(panel.locator('app-creature-card').getByRole('heading', { name: 'Nanquim' })).toBeVisible();
        await expect(panel.locator('app-creature-card').getByText('Corvo · Miúdo · Familiar de Pensantus')).toBeVisible();
      }
    } finally {
      await masterContext.close();
      await playerContext.close();
    }
  },
);
