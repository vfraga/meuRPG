import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { createMapRPC, saveDocumentRPC, tableWithDocumentParts } from './document-support';
import { createPointRPC, setCurrentMapRPC } from './maps-support';
import {
  authStatePath,
  callRPC,
  gotoCampaignTab,
  newSignedInContext,
  openCampaignTab,
  startGameSession,
} from './support';

// MR-018, the campaign document (web/src/app/pages/campaign-document and the
// "Documento da campanha" panel of the campaign page), against the real
// CampaignDocumentService. Every test builds its own campaign through the
// API; signing in is never the point, so every context reuses a saved state
// (the default page is the master's).
test.use({ storageState: authStatePath('Mestre Teste') });

test(
  'o mestre escreve o documento com a imagem e os links, salva, recarrega, e os links abrem numa janela',
  { tag: '@MR-018' },
  async ({ page }) => {
    test.slow();
    await page.goto('/');
    const table = await tableWithDocumentParts(page, `Documento ${Date.now()}`);

    await gotoCampaignTab(page, table.campaignId, 'Preparação');
    const row = page.getByRole('link', { name: /^Documento da campanha/ });
    await expect(row.getByText('Ainda sem texto · só você lê')).toBeVisible();
    await row.click();

    await expect(page.getByRole('heading', { level: 1, name: `${table.campaignName} — preparação` })).toBeVisible();
    await expect(page.getByText('Só o mestre vê este documento.')).toBeVisible();
    await page.getByRole('button', { name: 'Editar documento' }).click();

    // A heading and a paragraph with bold text, through the toolbar.
    const text = page.getByRole('textbox', { name: 'Texto' });
    await expect(text).toBeFocused();
    await page.getByRole('button', { name: 'Título' }).click();
    await page.keyboard.type('A estrada de Mirathel');
    await page.keyboard.press('Enter');
    await page.keyboard.press('Enter');
    await page.keyboard.type('Quem passar num teste de ');
    await page.getByRole('button', { name: 'Negrito' }).click();
    await page.keyboard.type('Sabedoria');
    // Past the closing "**" (not End: on a Mac it scrolls instead of moving the caret).
    await page.keyboard.press('ArrowRight');
    await page.keyboard.press('ArrowRight');
    await page.keyboard.type(' vê as flechas.');
    await expect(text).toHaveValue(/^## A estrada de Mirathel\n\nQuem passar num teste de \*\*Sabedoria\*\* vê as flechas\./);
    await expect(page.getByText('Rascunho não salvo')).toBeVisible();

    // The live preview follows.
    const preview = page.getByRole('region', { name: 'Prévia' });
    await expect(preview.getByRole('heading', { level: 2, name: 'A estrada de Mirathel' })).toBeVisible();
    await expect(preview.locator('strong')).toHaveText('Sabedoria');

    // The image, the map link and the sheet link, from their dialogs: no ID is typed.
    await page.keyboard.press('Enter');
    await page.keyboard.press('Enter');
    await page.getByRole('button', { name: 'Imagem da galeria' }).click();
    const imageDialog = page.getByRole('dialog', { name: 'Imagem da galeria' });
    await imageDialog.getByRole('radio', { name: /Taverna do Javali/ }).click();
    await imageDialog.getByRole('button', { name: 'Inserir imagem' }).click();
    await expect(text).toHaveValue(/!\[Taverna do Javali\]\(image:[0-9a-f-]{36}\)/);

    await page.getByRole('button', { name: 'Link para mapa' }).click();
    await page.getByRole('dialog', { name: 'Link para mapa' }).getByRole('button', { name: /Mirathel e arredores/ }).click();
    await expect(text).toHaveValue(new RegExp(`\\[Mirathel e arredores\\]\\(map:${table.mapId}\\)`));
    await expect(text).toBeFocused();

    await page.keyboard.type(' e ');
    await page.getByRole('button', { name: 'Link para ficha' }).click();
    await page.getByRole('dialog', { name: 'Link para ficha' }).getByRole('button', { name: /Capitão Goblin/ }).click();
    await expect(text).toHaveValue(new RegExp(`\\[Capitão Goblin\\]\\(character:${table.npcId}\\)`));

    await page.getByRole('button', { name: 'Salvar documento' }).click();

    // Back to reading, saved.
    await expect(page.getByRole('button', { name: 'Editar documento' })).toBeVisible();
    await expect(page.getByText(/^Salvo às \d\d:\d\d$/)).toBeVisible();
    const check = async () => {
      const article = page.getByRole('article', { name: 'Texto do documento' });
      await expect(article.getByRole('heading', { level: 2, name: 'A estrada de Mirathel' })).toBeVisible();
      await expect(article.locator('strong')).toHaveText('Sabedoria');
      const figure = article.getByRole('figure');
      await expect(figure.getByRole('img', { name: 'Taverna do Javali' })).toBeVisible();
      await expect(figure.getByText('Taverna do Javali', { exact: true }).last()).toBeVisible();
      await expect(article.getByRole('button', { name: 'Mirathel e arredores' })).toBeVisible();
      await expect(article.getByRole('button', { name: 'Capitão Goblin' })).toBeVisible();
      // The Sumário lists the heading.
      await expect(page.getByRole('navigation', { name: 'Sumário' }).getByRole('button', { name: 'A estrada de Mirathel' })).toBeVisible();
    };
    await check();

    // The panel on the campaign page now says it was edited.
    await page.reload();
    await check();
    await page.getByRole('link', { name: 'Voltar para a campanha' }).click();
    await openCampaignTab(page, 'Preparação');
    await expect(page.getByRole('link', { name: /^Documento da campanha/ }).getByText(/^Editado hoje às \d\d:\d\d · só você lê$/)).toBeVisible();
    // Back to the document: the tab click made its own history entry, so "back" would land on "Mesa".
    await page.goto(`/campaigns/${table.campaignId}/document`);

    // The map link opens a dialog over the document (E5-29): the map with
    // every point (a hidden one marked "Escondido"), whether it is revealed
    // and the open session's current map, the legend and the editor link.
    await createPointRPC(page, table.campaignId, table.mapId, { kind: 'BATTLE', name: 'Emboscada na estrada', xBp: 4000, yBp: 6000, revealed: true });
    await createPointRPC(page, table.campaignId, table.mapId, { kind: 'SUBMAP', name: 'Covil dos goblins', xBp: 2000, yBp: 3000 });
    expect((await startGameSession(page, table.campaignId)).ok()).toBeTruthy();
    await setCurrentMapRPC(page, table.campaignId, table.mapId);
    await page.getByRole('button', { name: 'Mirathel e arredores' }).click();
    const mapDialog = page.getByRole('dialog', { name: 'Mirathel e arredores' });
    await expect(mapDialog).toBeVisible();
    await expect(mapDialog.getByText('Revelado aos jogadores. Mapa atual da Sessão 1.')).toBeVisible();
    const mapImage = mapDialog.getByRole('img', { name: 'Prévia do mapa Mirathel e arredores' });
    await expect(mapImage).toBeVisible();
    await expect.poll(() => mapImage.locator('img').evaluate((img: HTMLImageElement) => img.naturalWidth)).toBeGreaterThan(0);
    await expect(mapDialog.getByText('Emboscada na estrada', { exact: true })).toBeVisible();
    await expect(mapDialog.locator('.lbl__pill', { hasText: 'Covil dos goblins' })).toContainText('Escondido');
    await expect(mapDialog.getByRole('list', { name: 'Pontos deste mapa' })).toContainText('Covil dos goblins, Submapa, escondido');
    await expect(mapDialog.getByRole('list', { name: 'Legenda do mapa' })).toContainText('Escondido');
    await expect(mapDialog.getByRole('link', { name: 'Abrir no editor de mapas' })).toHaveAttribute('href', `/campaigns/${table.campaignId}/maps/${table.mapId}`);
    await page.keyboard.press('Escape');
    await expect(mapDialog).toBeHidden();
    await expect(page.getByRole('button', { name: 'Mirathel e arredores' })).toBeFocused();

    // The sheet link opens the sheet dialog.
    await page.getByRole('button', { name: 'Capitão Goblin' }).click();
    const sheetDialog = page.getByRole('dialog', { name: 'Capitão Goblin' });
    await expect(sheetDialog.getByText(/Mago 3/)).toBeVisible();
    await expect(sheetDialog.getByRole('link', { name: 'Abrir ficha' })).toHaveAttribute(
      'href',
      `/campaigns/${table.campaignId}/characters/${table.npcId}`,
    );
  },
);

test(
  'um jogador da campanha vê só "Só o mestre vê o documento da campanha", e a página da campanha não mostra o painel',
  { tag: '@MR-018' },
  async ({ page, browser }) => {
    await page.goto('/');
    const table = await tableWithDocumentParts(page, `Documento jogador ${Date.now()}`);
    await saveDocumentRPC(page, table.campaignId, '## Segredo\n\nO capitão trabalha para um contrabandista.', 0);

    const invite = await callRPC(page, 'meurpg.campaigns.v1.CampaignService/CreateInvite', {
      campaignId: table.campaignId,
      maxUses: 1,
      expiresIn: '3600s',
    });
    expect(invite.ok()).toBeTruthy();
    const context = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const player = await context.newPage();
      await player.goto('/');
      const accepted = await callRPC(player, 'meurpg.campaigns.v1.CampaignService/AcceptInvite', {
        token: (await invite.json()).token,
      });
      expect(accepted.ok()).toBeTruthy();

      await player.goto(`/campaigns/${table.campaignId}`);
      await expect(player.getByRole('heading', { level: 1, name: table.campaignName })).toBeVisible();
      await expect(player.getByRole('heading', { name: 'Documento da campanha' })).toHaveCount(0);

      await player.goto(`/campaigns/${table.campaignId}/document`);
      await expect(player.getByText('Só o mestre vê o documento da campanha.')).toBeVisible();
      await expect(player.getByText('O capitão trabalha')).toHaveCount(0);
      await expect(player.getByRole('button', { name: 'Editar documento' })).toHaveCount(0);
    } finally {
      await context.close();
    }
  },
);

test(
  'quem não é membro vê "Campanha não encontrada"',
  { tag: '@MR-018' },
  async ({ page, browser }) => {
    await page.goto('/');
    const table = await tableWithDocumentParts(page, `Documento alheio ${Date.now()}`);
    const context = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const stranger = await context.newPage();
      await stranger.goto(`/campaigns/${table.campaignId}/document`);
      await expect(stranger.getByRole('heading', { level: 1, name: 'Campanha não encontrada' })).toBeVisible();
    } finally {
      await context.close();
    }
  },
);

test(
  'duas abas: a que ficou para trás recebe o aviso de conflito e mantém o rascunho até recarregar',
  { tag: '@MR-018' },
  async ({ page, context }) => {
    await page.goto('/');
    const table = await tableWithDocumentParts(page, `Documento conflito ${Date.now()}`);
    const url = `/campaigns/${table.campaignId}/document`;

    // Chrome shows one tab at a time and does not render the hidden ones, so a
    // hidden tab's scroll-into-view (part of every click) can stall for many
    // seconds on a busy machine. Like a person switching tabs, bring each tab
    // to the front before acting on it.
    const tabB = await context.newPage();
    await page.bringToFront();
    await page.goto(url);
    await tabB.bringToFront();
    await tabB.goto(url);
    await tabB.getByRole('button', { name: 'Editar documento' }).click();
    await page.bringToFront();
    await page.getByRole('button', { name: 'Editar documento' }).click();

    // Tab A saves first.
    await page.getByRole('textbox', { name: 'Texto' }).fill('Texto da aba A');
    await page.getByRole('button', { name: 'Salvar documento' }).click();
    await expect(page.getByText('Texto da aba A', { exact: true })).toBeVisible();

    // Tab B, still on revision 0, saves a different text.
    await tabB.bringToFront();
    const textB = tabB.getByRole('textbox', { name: 'Texto' });
    await textB.fill('Texto da aba B, que ainda não foi salvo');
    await tabB.getByRole('button', { name: 'Salvar documento' }).click();
    await expect(tabB.getByText('Este documento mudou em outra aba ou em outro aparelho.')).toBeVisible();
    // The draft is still there, and saving again is not offered.
    await expect(textB).toHaveValue('Texto da aba B, que ainda não foi salvo');
    await expect(tabB.getByRole('button', { name: 'Salvar documento' })).toBeDisabled();

    // "Recarregar" is the person's choice: then the saved text replaces the draft.
    await tabB.getByRole('button', { name: 'Recarregar' }).click();
    await expect(textB).toHaveValue('Texto da aba A');
    await expect(tabB.getByText('Este documento mudou em outra aba ou em outro aparelho.')).toBeHidden();
  },
);

test(
  'sair da edição com texto não salvo pergunta antes, e "Descartar mudanças" confirma na própria tela',
  { tag: '@MR-018' },
  async ({ page }) => {
    await page.goto('/');
    const table = await tableWithDocumentParts(page, `Documento sair ${Date.now()}`);
    await page.goto(`/campaigns/${table.campaignId}/document`);
    await page.getByRole('button', { name: 'Editar documento' }).click();
    await page.getByRole('textbox', { name: 'Texto' }).fill('Rascunho que não vai ser salvo');

    // Leaving through a link asks first; "Continuar editando" stays.
    await page.getByRole('link', { name: 'Voltar para a campanha' }).click();
    const ask = page.getByRole('dialog', { name: 'Sair sem salvar?' });
    await expect(ask).toBeVisible();
    await ask.getByRole('button', { name: 'Continuar editando' }).click();
    await expect(page).toHaveURL(new RegExp(`/campaigns/${table.campaignId}/document$`));
    await expect(page.getByRole('textbox', { name: 'Texto' })).toHaveValue('Rascunho que não vai ser salvo');

    // "Descartar mudanças" confirms in place, then returns to reading.
    await page.getByRole('button', { name: 'Descartar mudanças' }).click();
    await expect(page.getByText('Descartar as mudanças? Não dá para desfazer.')).toBeVisible();
    await page.getByRole('button', { name: 'Continuar editando' }).click();
    await expect(page.getByRole('textbox', { name: 'Texto' })).toBeVisible();
    await page.getByRole('button', { name: 'Descartar mudanças' }).click();
    await page.getByRole('button', { name: 'Descartar mudanças' }).click();
    await expect(page.getByRole('button', { name: 'Editar documento' })).toBeVisible();
    await expect(page.getByText('O documento ainda está vazio.')).toBeVisible();
  },
);

test(
  'um link para um mapa ou uma ficha apagados vira texto com "(mapa apagado)", e uma imagem apagada mostra "Imagem apagada"',
  { tag: '@MR-018' },
  async ({ page }) => {
    await page.goto('/');
    const table = await tableWithDocumentParts(page, `Documento apagado ${Date.now()}`);
    const lonelyImage = table.imageId;
    // A second map, to delete: the first one keeps the gallery image in use.
    const doomed = await createMapRPC(page, table.campaignId, 'Torre perdida', table.imageId);
    await saveDocumentRPC(
      page,
      table.campaignId,
      `Veja [Torre perdida](map:${doomed}) e [Mirathel](map:${table.mapId}).\n\n![Taverna](image:${lonelyImage})\n\n![Fantasma](image:00000000-0000-4000-8000-000000000000)\n\n[x](javascript:alert(1)) <script>alert(1)</script>`,
      0,
    );
    const deleted = await callRPC(page, 'meurpg.maps.v1.MapService/DeleteMap', { campaignId: table.campaignId, mapId: doomed });
    expect(deleted.ok()).toBeTruthy();

    await page.goto(`/campaigns/${table.campaignId}/document`);
    const article = page.getByRole('article', { name: 'Texto do documento' });
    await expect(article.getByText('Torre perdida (mapa apagado)')).toBeVisible();
    await expect(article.getByRole('button', { name: 'Mirathel' })).toBeVisible();
    await expect(article.getByRole('figure')).toHaveCount(1);
    await expect(article.getByText('Imagem apagada')).toBeVisible();
    // Raw HTML and a javascript: link stay as plain text.
    await expect(article.getByText('[x](javascript:alert(1)) <script>alert(1)</script>')).toBeVisible();
    await expect(article.getByRole('link')).toHaveCount(0);
  },
);
