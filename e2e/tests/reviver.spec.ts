import { expect, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { authStatePath, callRPC, characterRpcBody, createCharacterRPC, newSignedInContext, pensantus } from './support';

// RN-03 ("Reviver"), against the real screens: the master brings a dead player character back from its page, and is
// refused, with the way to the other character, while the player has another living one. The default page is the master's.
test.use({ storageState: authStatePath('Mestre Teste') });

/** A campaign whose player has a character the master marked dead (through the API: the death is another story). */
async function deadCharacter(master: Page, player: Page): Promise<{ campaignId: string; characterId: string }> {
  await master.goto('/');
  const created = await callRPC(master, 'meurpg.campaigns.v1.CampaignService/CreateCampaign', {
    name: `Reviver ${Date.now()}`,
    xpMode: 'XP_MODE_ENEMIES',
  });
  const campaignId = (await created.json()).campaign.id as string;
  const invite = await callRPC(master, 'meurpg.campaigns.v1.CampaignService/CreateInvite', {
    campaignId,
    maxUses: 1,
    expiresIn: '86400s',
    requiresApproval: false,
  });
  const { token } = await invite.json();
  await player.goto('/');
  const accepted = await callRPC(player, 'meurpg.campaigns.v1.CampaignService/AcceptInvite', { token });
  expect(accepted.ok()).toBeTruthy();
  const character = await createCharacterRPC(player, campaignId, characterRpcBody('PLAYER', pensantus));
  expect(character.status()).toBe(200);
  const characterId = (await character.json()).character.id as string;
  const dead = await callRPC(master, 'meurpg.characters.v1.CharacterService/MarkCharacterDead', { campaignId, characterId });
  expect(dead.status()).toBe(200);
  return { campaignId, characterId };
}

test(
  'o mestre revive um personagem morto pela ficha: a pergunta abre em "Reviver", Escape cancela e a ficha volta a ser a de um vivo',
  { tag: ['@RN-03'] },
  async ({ page, browser }) => {
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const playerPage = await playerContext.newPage();
      const { campaignId, characterId } = await deadCharacter(page, playerPage);
      const sheet = `/campaigns/${campaignId}/characters/${characterId}`;
      const state = (p: Page) => p.getByRole('list', { name: 'Estado do personagem' });

      // The master sees the dead page whole, with "Reviver" where "Marcar como morto" would be.
      await page.goto(sheet);
      await expect(state(page)).toContainText('Morto');
      await expect(page.getByText('A ficha fica guardada.')).toBeVisible();
      await expect(page.getByRole('button', { name: 'Marcar como morto' })).toHaveCount(0);

      // The question opens in place with the focus on the outlined button, and Escape puts the focus back.
      await page.getByRole('button', { name: 'Reviver', exact: true }).click();
      const question = page.getByRole('alertdialog', { name: `Reviver ${pensantus.name}?` });
      await expect(question).toContainText('volta com 1 PV, sem a condição Inconsciente');
      await expect(page.getByRole('button', { name: `Reviver ${pensantus.name}` })).toBeFocused();
      await page.keyboard.press('Escape');
      await expect(question).toHaveCount(0);
      await expect(page.getByRole('button', { name: 'Reviver', exact: true })).toBeFocused();
      await expect(state(page)).toContainText('Morto');

      // Confirmed: the character lives, and the page is the one of the living.
      await page.getByRole('button', { name: 'Reviver', exact: true }).click();
      await page.getByRole('button', { name: `Reviver ${pensantus.name}` }).click();
      await expect(page.getByRole('status').filter({ hasText: `${pensantus.name} voltou à vida` })).toBeAttached();
      await expect(state(page)).not.toContainText('Morto');
      await expect(page.getByText('A ficha fica guardada.')).toHaveCount(0);
      await expect(page.getByRole('button', { name: 'Reviver', exact: true })).toHaveCount(0);
      await expect(page.getByRole('button', { name: 'Marcar como morto' })).toBeVisible();

      // The player's page is the living one too, without "E agora?".
      await playerPage.goto(sheet);
      await expect(state(playerPage)).not.toContainText('Morto');
      await expect(playerPage.getByRole('heading', { name: 'E agora?' })).toHaveCount(0);
    } finally {
      await playerContext.close();
    }
  },
);

test(
  'o mestre não revive enquanto o jogador tem outro personagem vivo, e o aviso leva até ele',
  { tag: ['@RN-03'] },
  async ({ page, browser }) => {
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const playerPage = await playerContext.newPage();
      const { campaignId, characterId } = await deadCharacter(page, playerPage);
      const sheet = `/campaigns/${campaignId}/characters/${characterId}`;

      // The player's page of the dead one offers a new character; the player makes it.
      await playerPage.goto(sheet);
      await expect(playerPage.getByRole('heading', { name: 'E agora?' })).toBeVisible();
      await expect(playerPage.getByRole('link', { name: 'Criar um novo personagem' })).toBeVisible();
      const replacement = await createCharacterRPC(playerPage, campaignId, characterRpcBody('PLAYER', { ...pensantus, name: 'Personagem Novo' }));
      expect(replacement.status()).toBe(200);
      const livingId = (await replacement.json()).character.id as string;
      await playerPage.reload();
      await expect(playerPage.getByRole('heading', { name: 'E agora?' })).toHaveCount(0);

      // The master is refused with nothing changed: the card names the other character and links to it.
      await page.goto(sheet);
      await page.getByRole('button', { name: 'Reviver', exact: true }).click();
      await page.getByRole('button', { name: `Reviver ${pensantus.name}` }).click();
      const refusal = page.getByRole('alert').filter({ hasText: 'já tem outro personagem vivo' });
      await expect(refusal).toContainText('Personagem Novo');
      await expect(refusal.getByRole('link', { name: 'Abrir Personagem Novo' })).toHaveAttribute('href', `/campaigns/${campaignId}/characters/${livingId}`);
      await refusal.getByRole('button', { name: 'Cancelar' }).click();
      await expect(refusal).toHaveCount(0);
      await expect(page.getByRole('list', { name: 'Estado do personagem' })).toContainText('Morto');

      // The link goes to the living character's page.
      await page.getByRole('button', { name: 'Reviver', exact: true }).click();
      await page.getByRole('button', { name: `Reviver ${pensantus.name}` }).click();
      await page.getByRole('link', { name: 'Abrir Personagem Novo' }).click();
      await expect(page).toHaveURL(new RegExp(`/characters/${livingId}$`));
      await expect(page.getByRole('heading', { level: 1, name: 'Personagem Novo' })).toBeVisible();
    } finally {
      await playerContext.close();
    }
  },
);
