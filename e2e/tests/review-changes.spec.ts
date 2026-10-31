import { expect, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { authStatePath, callRPC, characterRpcBody, createCharacterRPC, newSignedInContext, pensantus } from './support';

// RN-15 / MR-024 ("Pedir ajustes"), against the real screens: the master sends a
// pending character back with a reason, the player reads it, edits the sheet and
// sends it again, and the master approves. Both users reuse the states
// auth.setup.ts saved. The default page is the master's.
test.use({ storageState: authStatePath('Mestre Teste') });

const reason = 'O antecedente não bate com a história. Também falta escolher o equipamento.';

/** A campaign whose player already created a pending character (through the API: the editor is another story). */
async function pendingCharacter(master: Page, player: Page): Promise<{ campaignId: string; characterId: string }> {
  await master.goto('/');
  const created = await callRPC(master, 'meurpg.campaigns.v1.CampaignService/CreateCampaign', {
    name: `Pedir ajustes ${Date.now()}`,
    xpMode: 'XP_MODE_ENEMIES',
  });
  const campaignId = (await created.json()).campaign.id as string;
  const invite = await callRPC(master, 'meurpg.campaigns.v1.CampaignService/CreateInvite', {
    campaignId,
    maxUses: 1,
    expiresIn: '86400s',
    requiresApproval: true,
  });
  const { token } = await invite.json();
  await player.goto('/');
  const accepted = await callRPC(player, 'meurpg.campaigns.v1.CampaignService/AcceptInvite', { token });
  expect(accepted.ok()).toBeTruthy();
  const character = await createCharacterRPC(player, campaignId, characterRpcBody('PLAYER', pensantus));
  expect(character.status()).toBe(200);
  return { campaignId, characterId: (await character.json()).character.id as string };
}

test(
  'o mestre pede ajustes, o jogador lê o motivo, edita e envia de novo, e o mestre aprova',
  { tag: ['@MR-024', '@RN-15'] },
  async ({ page, browser }) => {
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const playerPage = await playerContext.newPage();
      const { campaignId, characterId } = await pendingCharacter(page, playerPage);
      const sheet = `/campaigns/${campaignId}/characters/${characterId}`;
      const state = (p: Page) => p.getByRole('list', { name: 'Estado do personagem' });

      // The master opens the form in place, and the reason is required.
      await page.goto(sheet);
      await expect(page.getByRole('button', { name: 'Aprovar personagem' })).toBeVisible();
      await page.getByRole('button', { name: 'Pedir ajustes', exact: true }).click();
      await expect(page.getByRole('heading', { name: `Pedir ajustes em ${pensantus.name}` })).toBeVisible();
      await expect(page.getByLabel(/^O que .* precisa ajustar$/)).toBeFocused();
      await page.getByRole('button', { name: 'Enviar pedido' }).click();
      await expect(page.getByRole('alert').filter({ hasText: 'O pedido não vai sem motivo.' })).toBeVisible();
      await expect(page.getByLabel(/^O que .* precisa ajustar$/)).toBeFocused();

      // Sent: the tag, the quoted reason, and the three buttons.
      await page.getByLabel(/^O que .* precisa ajustar$/).fill(reason);
      await page.getByRole('button', { name: 'Enviar pedido' }).click();
      await expect(state(page)).toContainText('Pendente · ajustes pedidos');
      await expect(page.getByRole('status').filter({ hasText: 'Pedido de ajustes enviado a' })).toBeAttached();
      await expect(page.getByRole('button', { name: 'Pedir ajustes de novo' })).toBeFocused();
      await expect(page.getByText(`“${reason}”`)).toBeVisible();
      await expect(page.getByRole('button', { name: 'Aprovar personagem' })).toBeVisible();
      await expect(page.getByRole('button', { name: 'Recusar personagem' })).toBeVisible();

      // The player reads the reason in place of the waiting strip, and nothing was sent for them.
      await playerPage.goto(sheet);
      await expect(playerPage.getByRole('alert').filter({ hasText: 'O mestre pediu ajustes.' })).toBeVisible();
      await expect(playerPage.getByText(`“${reason}”`)).toBeVisible();
      await expect(playerPage.getByText('Esperando a aprovação do mestre')).toHaveCount(0);

      // Editing the sheet never sends it again by itself.
      await playerPage.getByRole('link', { name: 'Editar ficha' }).click();
      await playerPage.getByRole('tab', { name: /Equipamento/ }).click();
      await playerPage.getByLabel('Itens de equipamento', { exact: true }).fill('Grimório\nAdaga');
      await playerPage.getByRole('button', { name: 'Salvar ficha' }).click();
      await expect(playerPage).toHaveURL(new RegExp(`/characters/${characterId}$`));
      await expect(playerPage.getByText('Grimório', { exact: true })).toBeVisible();
      await expect(playerPage.getByRole('button', { name: 'Enviar de novo' })).toBeVisible();

      // Sending again: the waiting strip returns with the confirmation, and the reason is gone from their screen.
      await playerPage.getByRole('button', { name: 'Enviar de novo' }).click();
      await expect(playerPage.getByRole('status').filter({ hasText: 'Ficha enviada de novo.' })).toBeVisible();
      await expect(playerPage.getByText('Esperando a aprovação do mestre')).toBeVisible();
      await expect(playerPage.getByRole('button', { name: 'Enviar de novo' })).toHaveCount(0);
      await expect(playerPage.getByText(reason)).toHaveCount(0);

      // The master sees it came back, with the request quoted, and approves.
      await page.reload();
      await expect(state(page)).toContainText('Pendente · reenviado');
      await expect(page.getByText(`“${reason}”`)).toBeVisible();
      await page.getByRole('button', { name: 'Aprovar personagem' }).click();
      await expect(state(page)).toContainText('Rascunho');
      await expect(page.getByRole('button', { name: 'Aprovar personagem' })).toHaveCount(0);

      await playerPage.reload();
      await expect(state(playerPage)).toContainText('Rascunho');
      await expect(playerPage.getByRole('alert').filter({ hasText: 'O mestre pediu ajustes.' })).toHaveCount(0);
    } finally {
      await playerContext.close();
    }
  },
);

test(
  'recusar continua apagando o personagem, com a confirmação que abre em "Cancelar"',
  { tag: ['@MR-024', '@RN-15'] },
  async ({ page, browser }) => {
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const playerPage = await playerContext.newPage();
      const { campaignId, characterId } = await pendingCharacter(page, playerPage);

      await page.goto(`/campaigns/${campaignId}/characters/${characterId}`);
      await page.getByRole('button', { name: 'Recusar personagem' }).click();
      await expect(page.getByRole('heading', { name: `Recusar ${pensantus.name}?` })).toBeVisible();
      await expect(page.getByRole('button', { name: 'Cancelar' })).toBeFocused();
      await page.getByRole('button', { name: 'Confirmar recusa' }).click();
      await expect(page).toHaveURL(new RegExp(`/campaigns/${campaignId}$`));

      const gone = await callRPC(playerPage, 'meurpg.characters.v1.CharacterService/GetCharacter', { campaignId, characterId });
      expect(gone.status()).toBe(404);
    } finally {
      await playerContext.close();
    }
  },
);
