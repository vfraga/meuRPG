import { expect } from '@playwright/test';
import { test } from './diagnostics';

import {
  acceptInvite,
  authStatePath,
  callRPC,
  characterRpcBody,
  createCampaign,
  createCharacterRPC,
  createCharacterViaUI,
  gotoCampaignTab,
  newSignedInContext,
  pensantus,
  pensantusDerived,
} from './support';

// Etapa 4 (characters, rules): MR-003, MR-004 and MR-005, against the real
// character editor and sheet (web/src/app/pages/character-editor,
// character-sheet) and the real CharacterService/ContentService.
//
// None of these tests' own story is signing in, so every context here
// reuses a saved state (auth.setup.ts) instead of hitting the real,
// rate-limited /auth/login. The default page is the master's.
test.use({ storageState: authStatePath('Mestre Teste') });

test('o jogador entra pelo convite, cria o personagem e o mestre o vê na campanha', { tag: '@MR-003' }, async ({
  page,
  browser,
}) => {
  const campaignId = await createCampaign(page, `Personagens ${Date.now()}`);
  await page.getByRole('button', { name: 'Gerar convite' }).click();
  await expect(page.getByText('não será mostrado de novo')).toBeVisible();
  const link = (await page.locator('.invite-reveal__link').textContent())?.trim();

  const playerContext = await newSignedInContext(browser, 'Jogador Teste');
  try {
    const { page: joinedPage } = await acceptInvite(playerContext, link!);

    // No living character yet: the CTA offers creating one
    // (campaign-characters.html).
    await joinedPage.getByRole('link', { name: 'Criar meu personagem' }).click();
    await expect(joinedPage).toHaveURL(new RegExp(`/campaigns/${campaignId}/characters/new$`));

    const characterId = await createCharacterViaUI(joinedPage, campaignId, pensantus);
    expect(characterId).toBeTruthy();
    await expect(joinedPage.getByRole('heading', { level: 1 })).toContainText(pensantus.name);
  } finally {
    await playerContext.close();
  }

  // Back on the master's page: reload to see the new character.
  await page.reload();
  await expect(page.getByRole('heading', { name: 'Personagens dos jogadores' })).toBeVisible();
  // "Experiência" lists him too (Etapa 7): the group list is the one this proves.
  await expect(page.getByRole('region', { name: 'Personagens' }).getByText(pensantus.name)).toBeVisible();
});

test(
  'o jogador abre a ficha no celular e vê as seções da ficha oficial com os valores calculados pelo servidor',
  { tag: '@MR-004' },
  async ({ browser }) => {
    // Chromium mobile viewport, not a real device: Safari rejects __Host-
    // cookies on http://localhost (CONTRIBUTING.md's e2e section). Neither
    // context signs in for real: both reuse the saved states, the mobile
    // one alongside its own viewport/isMobile options.
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste', {
      viewport: { width: 390, height: 844 },
      isMobile: true,
    });
    try {
      const masterPage = await masterContext.newPage();
      const campaignId = await createCampaign(masterPage, `Ficha mobile ${Date.now()}`);
      await masterPage.getByRole('button', { name: 'Gerar convite' }).click();
      const link = (await masterPage.locator('.invite-reveal__link').textContent())?.trim();

      const { page: joinedPage } = await acceptInvite(playerContext, link!);

      // Created through the API, not the editor: this test's criterion is
      // the sheet screen, already proven separately by the @MR-003 test.
      const created = await createCharacterRPC(joinedPage, campaignId, characterRpcBody('PLAYER', pensantus));
      expect(created.status()).toBe(200);
      const characterId = (await created.json()).character.id as string;

      await joinedPage.goto(`/campaigns/${campaignId}/characters/${characterId}`);

      // The official sheet's sections (character-sheet.html and its child
      // components), each an <h2>. "Habilidades" and "Combate" are for screen
      // readers (the medallions and the shield are their visible titles);
      // "Magias de mago" only shows for a caster, which Pensantus is.
      for (const section of ['Habilidades', 'Combate', 'Perícias', 'Magias', 'Equipamento', 'Características e traços', 'História']) {
        await expect(joinedPage.getByRole('heading', { level: 2, name: section })).toBeVisible();
      }

      // The numbers come from the server's DerivedSheet, never recalculated
      // in the browser: the ability medallion shows the modifier (the score
      // is in its pill), and the spell DC and attack are written on rules,
      // label under the value, like the paper sheet.
      await expect(joinedPage.locator('dt:text-is("Inteligência") + dd')).toContainText(pensantusDerived.intModifier);
      await expect(joinedPage.locator('dt:text-is("CD de magia") + dd')).toHaveText(
        String(pensantusDerived.spellSaveDc),
      );
      await expect(joinedPage.locator('dt:text-is("Ataque de magia") + dd')).toHaveText(
        pensantusDerived.spellAttackBonus,
      );
      await expect(joinedPage.locator('dt:text-is("Classe de Armadura") + dd')).toHaveText(
        String(pensantusDerived.armorClass),
      );
      await expect(joinedPage.locator('dt:text-is("Pontos de vida máximos") + dd')).toHaveText(
        String(pensantusDerived.hitPointsMax),
      );
    } finally {
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test('o mestre cria um inimigo com ficha completa e um minion com ficha básica', { tag: '@MR-005' }, async ({ page }) => {
  const campaignId = await createCampaign(page, `NPCs ${Date.now()}`);

  // The "Novo NPC" menu (campaign-npcs.html, on the "Preparação" tab) routes to
  // /campaigns/:id/npcs/new/:kind, one per NPC kind.
  await gotoCampaignTab(page, campaignId, 'Preparação');
  await page.getByRole('button', { name: 'Novo NPC' }).click();
  await page.getByRole('menuitem', { name: 'Inimigo' }).click();
  await expect(page).toHaveURL(new RegExp(`/campaigns/${campaignId}/npcs/new/enemy$`));

  // Inimigo (ENEMY): full sheet, the same stepper as a player's.
  const enemyId = await createCharacterViaUI(page, campaignId, { ...pensantus, name: 'Goblin Chefe' }, page.url());
  expect(enemyId).toBeTruthy();

  await gotoCampaignTab(page, campaignId, 'Preparação');
  await page.getByRole('button', { name: 'Novo NPC' }).click();
  await page.getByRole('menuitem', { name: 'Minion' }).click();
  await expect(page).toHaveURL(new RegExp(`/campaigns/${campaignId}/npcs/new/minion$`));

  // Minion: a flat basic-sheet form, no stepper (character-editor.html).
  await page.getByLabel('Nome do personagem').fill('Bandido');
  await page.getByLabel('Pontos de Vida (máximo)').fill('4');
  await page.getByLabel('Classe de Armadura').fill('12');
  // The speed is typed in metres, in steps of 1,5 m (no feet on screen).
  await page.getByLabel('Deslocamento (m)').fill('9');
  await page.getByLabel('Iniciativa').fill('+2');
  await page.getByRole('button', { name: 'Adicionar ataque' }).click();
  await page.getByLabel('Nome do ataque').fill('Cimitarra');
  await page.getByLabel('Bônus de ataque').fill('+4');
  await page.getByLabel('Dados do dano').fill('1d6');
  await page.getByLabel('Bônus do dano').fill('+2');
  // Opened from the keyboard: the empty field's floating label overlaps the click point (see support.ts).
  const damageType = page.getByRole('combobox', { name: 'Tipo de dano' });
  await damageType.focus();
  await damageType.press('Enter');
  await page.getByRole('option', { name: 'Cortante' }).click();
  await expect(page.getByText('Na ficha: Cimitarra, +4 para acertar, 1d6 + 2 de dano cortante')).toBeVisible();
  await page.getByRole('button', { name: 'Criar NPC' }).click();
  await expect(page).toHaveURL(/\/campaigns\/[^/]+\/characters\/[^/]+$/);

  // The sheet shows initiative and the attack as the master typed them.
  await expect(page.locator('dt:text-is("Iniciativa") + dd')).toHaveText('+2');
  await expect(page.getByText('Cimitarra +4 · 1d6 + 2 cortante')).toBeVisible();

  await gotoCampaignTab(page, campaignId, 'Preparação');
  // "NPCs" is the title of a panel of the "Preparação" tab (campaign-npcs.html).
  await expect(page.getByRole('heading', { name: 'NPCs', level: 2 })).toBeVisible();
  await expect(page.getByText('Goblin Chefe')).toBeVisible();
  await expect(page.getByText('Bandido')).toBeVisible();
});

test('o jogador não vê os NPCs da campanha', { tag: '@MR-005' }, async ({ page, browser }) => {
  const campaignId = await createCampaign(page, `NPCs escondidos ${Date.now()}`);
  const npc = await createCharacterRPC(page, campaignId, characterRpcBody('ENEMY', { ...pensantus, name: 'Vilão Secreto' }));
  const npcId = (await npc.json()).character.id as string;

  await page.getByRole('button', { name: 'Gerar convite' }).click();
  const link = (await page.locator('.invite-reveal__link').textContent())?.trim();

  const playerContext = await newSignedInContext(browser, 'Jogador Teste');
  try {
    const { page: joinedPage } = await acceptInvite(playerContext, link!);

    await expect(joinedPage.getByRole('heading', { name: 'NPCs', level: 3 })).toHaveCount(0);
    await expect(joinedPage.getByText('Vilão Secreto')).toHaveCount(0);

    // The RPC itself hides it too, exactly like a campaign the caller is
    // not a member of: `not_found`, never `permission_denied`, so no NPC id
    // leaks.
    const getRes = await callRPC(joinedPage, 'meurpg.characters.v1.CharacterService/GetCharacter', {
      campaignId,
      characterId: npcId,
    });
    expect(getRes.status()).toBe(404);
    expect(await getRes.json()).toMatchObject({ code: 'not_found' });
  } finally {
    await playerContext.close();
  }
});
