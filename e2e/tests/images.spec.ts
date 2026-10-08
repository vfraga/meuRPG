import { expect, test, type Browser, type Page } from '@playwright/test';

import { endOpenSessionRPC } from './live-session-support';
import { generateSceneRPC, generateTextureRPC, liveSessionAsPlayer, mapRoute, referenceRPC, statusRPC, tableForImages, type ImagesTable } from './images-support';
import { sessionRoute } from './fog-support';
import { afterRender, callRPC, newSignedInContext } from './support';

// Generated images on screen (Etapa 10, slice 10.16: MR-039, RN-28, RN-10; E10-07). The server runs with the fake image generator, so a
// picture comes in a moment and nothing leaves the machine. The table comes through the API; what is under test is the master's dialog
// (the ways, who appears, the wait, the result, the adjustment, "Usar como imagem do mapa", the errors) and what a player never gets: the
// tests read the JSON the player's session receives, never the pixels.

test.describe.configure({ timeout: 240_000 });

interface Scene {
  table: ImagesTable;
  mp: Page;
  ap: Page;
}

/** A table with the master and Pensantus's player; whatever happens, the session ends and the contexts close. */
async function atTable(browser: Browser, name: string, body: (scene: Scene) => Promise<void>): Promise<void> {
  const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
  const pensantus = await newSignedInContext(browser, 'Jogador Teste');
  const toren = await newSignedInContext(browser, 'E-mail Não Verificado');
  const [mp, ap, bp] = await Promise.all([master.newPage(), pensantus.newPage(), toren.newPage()]);
  let campaignId = '';
  try {
    await Promise.all([mp.goto('/'), ap.goto('/'), bp.goto('/')]);
    const table = await tableForImages(mp, ap, bp, name);
    campaignId = table.campaignId;
    await body({ table, mp, ap });
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(mp, campaignId).catch(() => undefined);
    }
    await Promise.all([master.close(), pensantus.close(), toren.close()]);
  }
}

const dialog = (page: Page) => page.getByRole('dialog');

async function openGenerate(mp: Page, table: ImagesTable): Promise<void> {
  await mp.goto(mapRoute(table));
  await mp.getByRole('button', { name: 'Gerar imagem com IA' }).click();
  await expect(dialog(mp).getByRole('heading', { level: 2, name: 'Gerar imagem' })).toBeVisible();
  // The drawing and the NPCs of the players' view have come.
  await expect(dialog(mp).getByText('Quem aparece na imagem')).toBeVisible();
}

test('o mestre gera a arte da cena de um mapa, pede um ajuste e a mostra, e o jogador só a vê depois de mostrada @MR-039 @RN-28 @RN-10', async ({ browser }) => {
  await atTable(browser, `Imagem ${Date.now()}`, async ({ table, mp, ap }) => {
    await openGenerate(mp, table);
    const d = dialog(mp);

    // The three ways, the drawing that goes along and what goes to Google, said under the fields.
    await expect(d.getByRole('radio', { name: 'Arte da cena' })).toHaveAttribute('aria-checked', 'true');
    await expect(d.getByText('A imagem parte do que os jogadores veem agora.')).toBeVisible();
    await expect(d.getByText('Não escreva nomes de pessoas.')).toBeVisible();
    // The note is in the fixed footer; under 360 px it ends the body instead (one of the two is drawn).
    await expect(d.locator('p.synth:visible')).toContainText('SynthID');
    await expect(d.getByText(/Restam\s20\sde\s20\simagens\sem\s\p{L}+\./u)).toBeVisible();

    // "Gerar imagem" waits for the text.
    await expect(d.getByRole('button', { name: 'Gerar imagem' })).toHaveAttribute('aria-disabled', 'true');
    await d.getByRole('checkbox', { name: /Capitão Goblin/ }).click();
    await d.getByRole('textbox', { name: 'Descreva o lugar' }).fill('Uma sala de guarda com tochas e caixotes');
    await expect(d.getByRole('button', { name: 'Gerar imagem' })).not.toHaveAttribute('aria-disabled', 'true');
    await d.getByRole('button', { name: 'Gerar imagem' }).click();

    // The picture comes into the gallery, hidden: "Imagem 1", saved, and nothing for the players.
    // The heading is the image's name (the players read the same), never its number.
    await expect(d.getByRole('heading', { level: 2, name: 'A caverna do Vale Seco · arte da cena' })).toBeVisible({ timeout: 60_000 });
    await expect(d.getByText('Guardada na galeria')).toBeVisible();
    await expect(d.getByText('Escondida dos jogadores até você mostrar.')).toBeVisible();
    await expect(d.getByText(/Gerada a partir do mapa · Pintura a óleo · 1 NPC/)).toBeVisible();
    const imageId = (await d.locator('.shot img').getAttribute('src'))!.replace('/images/', '');
    expect(await (await liveSessionAsPlayer(ap, table.campaignId)).shownImage).toBeUndefined();
    // The gallery is the master's: a player can neither list it nor fetch the file.
    const list = await callRPC(ap, 'meurpg.maps.v1.GalleryService/ListGalleryImages', { campaignId: table.campaignId });
    expect(list.ok()).toBe(false);
    expect((await ap.request.get(`/images/${imageId}`)).ok()).toBe(false);

    // The adjustment: a new picture beside this one, and the chain.
    await expect(d.getByRole('button', { name: 'Pedir o ajuste' })).toHaveAttribute('aria-disabled', 'true');
    await d.getByRole('textbox', { name: 'Pedir um ajuste' }).fill('mais escura, com uma ponte sobre o poço');
    await d.getByRole('button', { name: 'Pedir o ajuste' }).click();
    await expect(d.getByRole('heading', { level: 2, name: 'A caverna do Vale Seco · arte da cena (ajuste)' })).toBeVisible({ timeout: 60_000 });
    await expect(d.getByText('A cadeia de ajustes')).toBeVisible();
    await expect(d.locator('.chain__now')).toHaveText('Imagem 2');
    await expect(d.getByRole('button', { name: 'Imagem 1' })).toBeVisible();
    expect((await liveSessionAsPlayer(ap, table.campaignId)).shownImage).toBeUndefined();

    // "Mostrar aos jogadores": one touch; only then does the player's session carry the image.
    await d.getByRole('button', { name: 'Mostrar aos jogadores' }).click();
    await expect(d.getByText('Mostrada aos jogadores')).toBeVisible();
    // The players read a name the master can recognise, never "Imagem N": the map's and the way, and the adjustment.
    await expect.poll(async () => (await liveSessionAsPlayer(ap, table.campaignId)).shownImage?.name).toBe('A caverna do Vale Seco · arte da cena (ajuste)');
    const secondId = (await d.locator('.shot img').getAttribute('src'))!.replace('/images/', '');
    expect((await liveSessionAsPlayer(ap, table.campaignId)).shownImage.id).toBe(secondId);
  });
});

test('o mapa com textura vira a imagem do mapa e as camadas e o que os jogadores viram continuam @MR-039 @RN-28 @RN-10', async ({ browser }) => {
  await atTable(browser, `Textura ${Date.now()}`, async ({ table, mp }) => {
    const layersOf = async () => {
      const res = await callRPC(mp, 'meurpg.maps.v1.MapService/GetMapLayers', { campaignId: table.campaignId, mapId: table.mapId });
      expect(res.ok(), await res.text()).toBeTruthy();
      const body = await res.json();
      return { wall: body.wall, difficult: body.difficultTerrain, cover: body.cover };
    };
    const imageOf = async () => {
      const res = await callRPC(mp, 'meurpg.maps.v1.MapService/GetMap', { campaignId: table.campaignId, mapId: table.mapId });
      expect(res.ok(), await res.text()).toBeTruthy();
      return (await res.json()).map.image.id as string;
    };
    const before = await layersOf();
    expect(before.wall).toBeTruthy();
    const imageBefore = await imageOf();

    await openGenerate(mp, table);
    const d = dialog(mp);
    await d.getByRole('radio', { name: 'O mapa com textura' }).click();
    // The textured map takes no creature, and no ratio: the server picks one from the grid.
    await expect(d.getByText('Quem aparece na imagem')).toHaveCount(0);
    await expect(d.getByRole('combobox', { name: 'Proporção' })).toHaveCount(0);
    await expect(d.getByText('ajustado à grade de 24 × 16 quadrados')).toBeVisible();
    await d.getByRole('textbox', { name: 'Descreva o lugar' }).fill('Uma caverna de pedra clara com musgo');
    await d.getByRole('button', { name: 'Gerar imagem' }).click();

    // The picture comes with the grid over it; "Usar como imagem do mapa" asks in place.
    await expect(d.getByRole('heading', { level: 2, name: 'A caverna do Vale Seco · mapa com textura' })).toBeVisible({ timeout: 60_000 });
    await expect(d.getByText('O mapa visto de cima, com a grade por cima (24 × 16 quadrados)')).toBeVisible();
    await expect(d.locator('svg.shot__grid path')).toHaveCount(1);
    await expect(d.getByRole('button', { name: 'Mostrar aos jogadores' })).toHaveCount(0);
    await expect(d.getByText(/Mostra o mapa inteiro, também o que os jogadores ainda não descobriram/)).toBeVisible();
    // An adjustment of it is a picture of the whole map too: no showing with one touch, and it can be used as the map's image.
    await d.getByRole('textbox', { name: 'Pedir um ajuste' }).fill('pedra mais clara');
    await d.getByRole('button', { name: 'Pedir o ajuste' }).click();
    await expect(d.getByText('A cadeia de ajustes')).toBeVisible({ timeout: 60_000 });
    await expect(d.getByRole('button', { name: 'Mostrar aos jogadores' })).toHaveCount(0);
    await expect(d.locator('svg.shot__grid path')).toHaveCount(1);
    await d.getByRole('button', { name: 'Usar como imagem do mapa' }).click();
    await expect(d.getByText('A grade, as camadas e o que os jogadores já viram continuam. Os jogadores veem a nova imagem.')).toBeVisible();
    // Nothing is used before the second press.
    expect(await imageOf()).toBe(imageBefore);
    await d.getByRole('button', { name: 'Usar como imagem do mapa' }).last().click();
    await expect(d.getByText(/Agora esta é a imagem do mapa/)).toBeVisible();

    // The map's image is the new one and the layers are the same bytes.
    expect(await imageOf()).not.toBe(imageBefore);
    expect(await layersOf()).toEqual(before);

    // Closing, the page shows the new image without a reload.
    await d.getByRole('button', { name: 'Fechar' }).click();
    await expect(dialog(mp)).toHaveCount(0);
    await expect(mp.locator('app-map-head')).toContainText('Imagem:');
  });
});

test('os jogadores só veem os NPCs que enxergam: o escondido não está na lista @MR-039 @RN-28 @RN-10', async ({ browser }) => {
  await atTable(browser, `Quem aparece ${Date.now()}`, async ({ table, mp }) => {
    // What the server offers: the NPCs the players see now, never the hidden Goblin 2.
    const scene = await referenceRPC(mp, table, 'IMAGE_GENERATION_KIND_MAP_SCENE');
    const names = (scene.creatures as { name: string; characterId: string }[]).map((c) => c.name).sort();
    expect(names).toEqual(['Capitão Goblin', 'Goblin 1']);
    expect(scene.creatures.some((c: { characterId: string }) => c.characterId === table.hiddenId)).toBe(false);
    expect(scene.playersSeeSomething).toBe(true);
    // The textured map starts from the whole map and lists no creature.
    expect(((await referenceRPC(mp, table, 'IMAGE_GENERATION_KIND_TEXTURED_MAP')).creatures ?? []).length).toBe(0);

    // And the dialog lists only those, with the sentence on why another is not there.
    await openGenerate(mp, table);
    const d = dialog(mp);
    await expect(d.getByRole('checkbox')).toHaveCount(2);
    await expect(d.getByRole('checkbox', { name: /Goblin 1/ })).toBeVisible();
    await expect(d.getByRole('checkbox', { name: /Capitão Goblin/ })).toBeVisible();
    await expect(d.getByRole('checkbox', { name: /Goblin 2/ })).toHaveCount(0);
    await expect(d.getByText('0 de 2 marcados')).toBeVisible();
    await expect(d.getByText(/uma criatura que eles não veem não está na lista/)).toBeVisible();

    // The server refuses the hidden one if it is asked for anyway.
    const refused = await callRPC(mp, 'meurpg.maps.v1.ImageGenerationService/GenerateMapImage', {
      campaignId: table.campaignId,
      mapId: table.mapId,
      kind: 'IMAGE_GENERATION_KIND_MAP_SCENE',
      idempotencyKey: crypto.randomUUID(),
      prompt: 'Uma sala',
      npcCharacterIds: [table.hiddenId],
    });
    expect(refused.status()).toBe(400);
  });
});

test('o limite do mês: a conta mostra 0, o botão fica desligado e diz quando volta @MR-039 @RN-28', async ({ browser }) => {
  await atTable(browser, `Limite ${Date.now()}`, async ({ table, mp }) => {
    const limit = (await statusRPC(mp, table.campaignId)).monthlyLimit as number;
    for (let i = 0; i < limit; i++) {
      await generateSceneRPC(mp, table.campaignId, `Imagem de teste ${i + 1}`);
    }
    // A zero is left out of the JSON.
    expect((await statusRPC(mp, table.campaignId)).remaining ?? 0).toBe(0);

    await openGenerate(mp, table);
    const d = dialog(mp);
    await d.getByRole('textbox', { name: 'Descreva o lugar' }).fill('Mais uma imagem');
    await expect(d.getByText(/Restam\s0\sde\s\d+\simagens\sem\s\p{L}+\./u)).toBeVisible();
    await expect(d.getByText(/Você usou as \d+ imagens de \p{L}+\. Volta em 1º de \p{L}+\./u)).toBeVisible();
    await expect(d.getByRole('button', { name: 'Gerar imagem' })).toHaveAttribute('aria-disabled', 'true');
    // The server says the same when asked anyway: a typed refusal, and no slot moved.
    const asked = await callRPC(mp, 'meurpg.maps.v1.ImageGenerationService/GenerateSceneImage', { campaignId: table.campaignId, idempotencyKey: crypto.randomUUID(), prompt: 'Mais uma' });
    expect(asked.status()).toBe(400);
    expect(await asked.text()).toContain('IMAGE_GENERATION_BLOCKED_REASON_LIMIT_REACHED');
  });
});

test('com a geração desligada o botão fica ao lado da razão e nenhum diálogo abre @MR-039 @RN-28', async ({ browser }) => {
  const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
  try {
    const mp = await master.newPage();
    await mp.goto('/');
    const created = await callRPC(mp, 'meurpg.campaigns.v1.CampaignService/CreateCampaign', { name: `Desligada ${Date.now()}`, xpMode: 'XP_MODE_ENEMIES' });
    expect(created.ok()).toBeTruthy();
    const campaignId = (await created.json()).campaign.id as string;
    // The server of this stack has the fake generator on; the screen is judged against the answer a server without one gives.
    await mp.route('**/meurpg.maps.v1.ImageGenerationService/GetImageGenerationStatus', (route) =>
      route.fulfill({ contentType: 'application/json', body: JSON.stringify({ status: { enabled: false, monthlyLimit: 20, remaining: 20, month: '2026-10' } }) }),
    );
    await mp.goto(`/campaigns/${campaignId}/gallery`);
    const button = mp.getByRole('button', { name: 'Gerar imagem com IA' });
    await expect(button).toHaveAttribute('aria-disabled', 'true');
    await expect(mp.getByText('A geração de imagens não está ligada neste servidor.')).toBeVisible();
    // The button is dashed and does nothing: forced, since Playwright waits for an enabled one.
    await button.click({ force: true });
    await afterRender(mp);
    await expect(dialog(mp)).toHaveCount(0);
  } finally {
    await master.close();
  }
});

test('da galeria: uma imagem gerada leva a etiqueta, e "Pedir um ajuste" abre o resultado com a cadeia @MR-039 @RN-28', async ({ browser }) => {
  await atTable(browser, `Galeria ${Date.now()}`, async ({ table, mp }) => {
    const first = await generateSceneRPC(mp, table.campaignId, 'Uma taverna à noite', 'Uma taverna à noite');
    const second = await callRPC(mp, 'meurpg.maps.v1.ImageGenerationService/EditGeneratedImage', {
      campaignId: table.campaignId,
      imageId: first,
      idempotencyKey: crypto.randomUUID(),
      instruction: 'com a lareira acesa',
    });
    expect(second.ok(), await second.text()).toBeTruthy();
    await expect.poll(async () => {
      const list = await callRPC(mp, 'meurpg.maps.v1.GalleryService/ListGalleryImages', { campaignId: table.campaignId });
      return ((await list.json()).images ?? []).filter((i: { generated?: boolean }) => i.generated).length;
    }).toBe(2);

    await mp.goto(`/campaigns/${table.campaignId}/gallery`);
    await expect(mp.getByText('Gerada por IA')).toHaveCount(2);
    await expect(mp.getByText('Editada de Uma taverna à noite')).toBeVisible();

    // The lightbox of a generated image offers the adjustment, which opens its result with the chain.
    await mp.getByRole('button', { name: 'Ver Uma taverna à noite (ajuste)' }).first().click();
    await mp.getByRole('button', { name: 'Pedir um ajuste' }).click();
    const d = dialog(mp);
    await expect(d.getByRole('heading', { level: 2, name: 'Uma taverna à noite (ajuste)' })).toBeVisible();
    await expect(d.getByText('A cadeia de ajustes')).toBeVisible();
    await expect(d.getByRole('button', { name: 'Mostrar aos jogadores' })).toBeVisible();
  });
});

test('da galeria: mostrar um mapa com textura pergunta antes, e só mostra na segunda vez @MR-039 @RN-10', async ({ browser }) => {
  await atTable(browser, `Mapa inteiro ${Date.now()}`, async ({ table, mp, ap }) => {
    const textureId = await generateTextureRPC(mp, table);
    await mp.goto(`/campaigns/${table.campaignId}/gallery`);
    await mp.getByRole('button', { name: /^Ver / }).first().click();
    await mp.getByRole('button', { name: 'Pedir um ajuste' }).click();
    const d = dialog(mp);
    await expect(d.getByText(/Mostra o mapa inteiro, também o que os jogadores ainda não descobriram/)).toBeVisible();
    await d.getByRole('button', { name: 'Mostrar aos jogadores' }).click();
    await expect(d.getByText('Esta imagem mostra o mapa inteiro, também o que os jogadores ainda não descobriram.')).toBeVisible();
    expect((await liveSessionAsPlayer(ap, table.campaignId)).shownImage).toBeUndefined();
    await d.getByRole('button', { name: 'Voltar' }).click();
    expect((await liveSessionAsPlayer(ap, table.campaignId)).shownImage).toBeUndefined();
    await d.getByRole('button', { name: 'Mostrar aos jogadores' }).click();
    await d.getByRole('button', { name: 'Mostrar mesmo assim' }).click();
    await expect.poll(async () => (await liveSessionAsPlayer(ap, table.campaignId)).shownImage?.id).toBe(textureId);
  });
});

test('na sessão: o seletor de "Mostrar imagem" marca o mapa inteiro e pergunta antes de mostrá-lo @MR-039 @RN-10', async ({ browser }) => {
  await atTable(browser, `Sessão ${Date.now()}`, async ({ table, mp, ap }) => {
    const textureId = await generateTextureRPC(mp, table);
    await ap.goto(sessionRoute(table.campaignId));
    await mp.goto(sessionRoute(table.campaignId));
    await mp.getByRole('button', { name: 'Mostrar imagem' }).click();
    const picker = mp.getByRole('dialog', { name: 'Mostrar uma imagem aos jogadores' });
    // The picture of the whole map is tagged in the grid.
    const tile = picker.getByRole('radio', { name: /mapa com textura/ });
    await expect(tile).toContainText('Mapa inteiro');
    await tile.click();
    await picker.getByRole('button', { name: 'Mostrar aos jogadores' }).click();
    await expect(picker.getByText('Esta imagem mostra o mapa inteiro, também o que os jogadores ainda não descobriram.')).toBeVisible();
    await expect(picker.getByRole('button', { name: 'Voltar' })).toBeFocused();
    expect((await liveSessionAsPlayer(ap, table.campaignId)).shownImage).toBeUndefined();
    // "Voltar" shows nothing.
    await picker.getByRole('button', { name: 'Voltar' }).click();
    await expect(picker.getByText('Mostrar o mapa inteiro?')).toHaveCount(0);
    expect((await liveSessionAsPlayer(ap, table.campaignId)).shownImage).toBeUndefined();
    await expect(ap.getByRole('region', { name: 'O mestre está mostrando' })).toHaveCount(0);
    // "Mostrar mesmo assim" shows it: the player's session and screen carry it.
    await picker.getByRole('button', { name: 'Mostrar aos jogadores' }).click();
    await picker.getByRole('button', { name: 'Mostrar mesmo assim' }).click();
    await expect(picker).toBeHidden();
    await expect.poll(async () => (await liveSessionAsPlayer(ap, table.campaignId)).shownImage?.id).toBe(textureId);
    await expect(ap.getByRole('region', { name: 'O mestre está mostrando' })).toBeVisible();
  });
});

test('no documento: inserir uma imagem do mapa inteiro pergunta antes, porque os jogadores leem o documento @MR-039 @RN-10', async ({ browser }) => {
  await atTable(browser, `Documento ${Date.now()}`, async ({ table, mp }) => {
    const textureId = await generateTextureRPC(mp, table);
    await mp.goto(`/campaigns/${table.campaignId}/document`);
    await mp.getByRole('button', { name: 'Editar documento' }).click();
    const text = mp.getByRole('textbox', { name: 'Texto' });
    await mp.getByRole('button', { name: 'Imagem da galeria' }).click();
    const picker = mp.getByRole('dialog', { name: 'Imagem da galeria' });
    await picker.getByRole('radio', { name: /mapa com textura/ }).click();
    await picker.getByRole('button', { name: 'Inserir imagem' }).click();
    await expect(picker.getByText('Inserir o mapa inteiro?')).toBeVisible();
    await expect(picker.getByText(/Esta imagem mostra o mapa inteiro, também o que os jogadores ainda não descobriram/)).toBeVisible();
    await expect(picker.getByRole('button', { name: 'Voltar' })).toBeFocused();
    await expect(text).not.toHaveValue(new RegExp(textureId));
    await picker.getByRole('button', { name: 'Voltar' }).click();
    await expect(picker.getByText('Inserir o mapa inteiro?')).toHaveCount(0);
    await picker.getByRole('button', { name: 'Inserir imagem' }).click();
    await picker.getByRole('button', { name: 'Inserir mesmo assim' }).click();
    await expect(text).toHaveValue(new RegExp(`image:${textureId}`));
  });
});
