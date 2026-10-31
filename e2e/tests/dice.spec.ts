import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { tableWithPensantus } from './live-session-support';
import {
  gotoCampaignTab,
  newSignedInContext,
} from './support';

// RN-18 (MR-013, MR-014): how the campaign's players roll dice. The master
// sets it in "Dados" (E6-17); a player sees "Como você rola os dados"
// (E6-18) on the campaign page, as a choice or, when the master decided for
// everyone, as a locked note.
test(
  'o mestre decide como rolam; o jogador escolhe só quando a campanha deixa, e o mestre vê a escolha',
  { tag: '@RN-18' },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const masterPage = await master.newPage();
      const playerPage = await player.newPage();
      await masterPage.goto('/');
      await playerPage.goto('/');
      const { campaignId } = await tableWithPensantus(masterPage, playerPage, `Dados ${Date.now()}`);
      await gotoCampaignTab(masterPage, campaignId, 'Regras');
      await playerPage.goto(`/campaigns/${campaignId}`);

      // The default: players choose, and the player may pick. The master's panel only says it: the mode is edited in
      // "Regras da mesa" (one place, saved with the rest of the rules).
      const dados = masterPage.getByRole('region', { name: 'Dados' });
      await expect(dados.getByText('Cada jogador escolhe')).toBeVisible();
      await expect(dados.getByRole('radio')).toHaveCount(0);
      const como = playerPage.getByRole('region', { name: 'Como você rola os dados' });
      await expect(como.getByRole('radio', { name: /No app/ })).toBeChecked();

      // The master makes everyone roll in the app, on "Regras da mesa": the player gets the note.
      await masterPage.getByRole('link', { name: 'Mudar em Regras da mesa' }).click();
      await expect(masterPage.getByRole('heading', { level: 1, name: 'Regras da mesa' })).toBeVisible();
      await masterPage.getByRole('radiogroup', { name: 'Como os jogadores rolam' }).locator('label', { hasText: 'Todos rolam no app' }).click();
      await masterPage.getByRole('button', { name: 'Salvar regras' }).click();
      await expect(masterPage.getByText(/Regras salvas\./)).toBeVisible();
      await gotoCampaignTab(masterPage, campaignId, 'Regras');
      await expect(dados.getByText('Todos rolam no app')).toBeVisible();
      await playerPage.reload();
      await expect(como.getByText('O mestre decidiu: todos rolam no app.')).toBeVisible();
      await expect(como.getByRole('radio', { name: /Meus próprios dados/ })).toBeDisabled();
      await expect(como.getByRole('button', { name: 'Salvar escolha' })).toHaveCount(0);

      // Back to "Cada jogador escolhe": the player picks their own dice and
      // the master sees it in the list.
      await masterPage.goto(`/campaigns/${campaignId}/rules`);
      await masterPage.getByRole('radiogroup', { name: 'Como os jogadores rolam' }).locator('label', { hasText: 'Cada jogador escolhe' }).click();
      await masterPage.getByRole('button', { name: 'Salvar regras' }).click();
      await expect(masterPage.getByText(/Regras salvas\./)).toBeVisible();
      await playerPage.reload();
      await como.getByText('Meus próprios dados', { exact: true }).click();
      await como.getByRole('button', { name: 'Salvar escolha' }).click();
      await expect(como.getByText('Salvo.')).toBeVisible();

      await gotoCampaignTab(masterPage, campaignId, 'Regras');
      await expect(dados.getByRole('listitem').filter({ hasText: 'Sem nome no perfil' })).toContainText('Meus próprios dados');
    } finally {
      await master.close();
      await player.close();
    }
  },
);
