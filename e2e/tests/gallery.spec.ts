import { expect } from '@playwright/test';
import { test } from './diagnostics';

import {
  canvasJpeg,
  galleryCard,
  imageIdOf,
  newCampaign,
  uploadThroughPicker,
  withExifGps,
} from './gallery-support';
import {
  acceptInvite,
  authStatePath,
  callRPC,
  gotoCampaignTab,
  newSignedInContext,
} from './support';

// MR-019, the gallery (web/src/app/pages/gallery, the Galeria panel of the
// campaign page), against the real upload route and GalleryService.
//
// Every image is built inside the test (a canvas JPEG, plus an EXIF block
// with a GPS position where that is the point), and every test makes its
// own campaign. Signing in is never the point here, so every context
// reuses a saved state; the default page is the master's.
test.use({ storageState: authStatePath('Mestre Teste') });

test(
  'o mestre envia um JPEG com EXIF e GPS: a imagem aparece na galeria, e o arquivo guardado não tem o EXIF',
  { tag: '@MR-019' },
  async ({ page }) => {
    await page.goto('/');
    const campaignId = await newCampaign(page, `Galeria EXIF ${Date.now()}`);
    const photo = withExifGps(await canvasJpeg(page));
    // The file really carries the metadata the server must drop.
    expect(photo.includes(Buffer.from('Exif\0\0'))).toBe(true);

    await page.goto(`/campaigns/${campaignId}/gallery`);
    await expect(page.getByRole('heading', { name: 'Nenhuma imagem ainda' })).toBeVisible();
    await expect(page.getByText('Use imagens do jogo. Não envie fotos de pessoas sem a autorização delas.')).toBeVisible();

    await uploadThroughPicker(page, [{ name: 'foto-da-mesa.jpg', mimeType: 'image/jpeg', buffer: photo }]);

    const card = galleryCard(page, 'Foto da mesa');
    await expect(card).toBeVisible();
    await expect(card.getByText('64 × 48 px')).toBeVisible();
    await expect(page.getByText(/1 imagem · \d+\sKB de 500\sMB/)).toBeVisible();

    // The stored image, downloaded with the master's cookies, has no EXIF
    // block at all (and so no GPS position).
    const id = await imageIdOf(card);
    const stored = await page.request.get(`/images/${id}`);
    expect(stored.status()).toBe(200);
    expect(stored.headers()['content-type']).toBe('image/jpeg');
    const bytes = await stored.body();
    expect(bytes.subarray(0, 2).equals(Buffer.from([0xff, 0xd8]))).toBe(true);
    expect(bytes.includes(Buffer.from('Exif'))).toBe(false);

    // It survives a reload: it's on the server, not just on the screen.
    await page.reload();
    await expect(galleryCard(page, 'Foto da mesa')).toBeVisible();
  },
);

test(
  'um arquivo recusado mostra o erro em português e deixa a galeria como estava',
  { tag: '@MR-019' },
  async ({ page }) => {
    await page.goto('/');
    const campaignId = await newCampaign(page, `Galeria recusa ${Date.now()}`);
    await page.goto(`/campaigns/${campaignId}/gallery`);
    await expect(page.getByRole('heading', { name: 'Nenhuma imagem ainda' })).toBeVisible();

    // A text file named .png: the browser calls it image/png, so only the
    // server, which reads the bytes, can refuse it. And a GIF, which the
    // app refuses before sending.
    await uploadThroughPicker(page, [
      { name: 'anotacoes.png', mimeType: 'image/png', buffer: Buffer.from('Isto é um texto, não uma imagem.\n') },
      { name: 'mapa-antigo.gif', mimeType: 'image/gif', buffer: Buffer.from('GIF89a\x01\x00\x01\x00\x00\x00\x00;', 'binary') },
    ]);

    const refused = page.getByRole('alert');
    await expect(
      refused.filter({ hasText: 'Não deu para enviar anotacoes.png. Esse arquivo não é uma imagem JPEG, PNG ou WebP.' }),
    ).toBeVisible();
    await expect(
      refused.filter({ hasText: 'Não deu para enviar mapa-antigo.gif. Esse arquivo não é uma imagem JPEG, PNG ou WebP.' }),
    ).toBeVisible();

    // Still empty, on the screen and on the server.
    await expect(page.getByRole('heading', { name: 'Nenhuma imagem ainda' })).toBeVisible();
    const listed = await callRPC(page, 'meurpg.maps.v1.GalleryService/ListGalleryImages', { campaignId });
    expect(listed.ok()).toBeTruthy();
    expect((await listed.json()).images ?? []).toEqual([]);
  },
);

test(
  'o jogador da campanha vê que só o mestre vê a galeria, e a página da campanha não mostra a Galeria para ele',
  { tag: '@MR-019' },
  async ({ page, browser }) => {
    await page.goto('/');
    const campaignId = await newCampaign(page, `Galeria jogador ${Date.now()}`);
    const invite = await callRPC(page, 'meurpg.campaigns.v1.CampaignService/CreateInvite', {
      campaignId,
      maxUses: 1,
      expiresIn: '3600s',
    });
    expect(invite.ok()).toBeTruthy();
    const token = (await invite.json()).token as string;

    // The master sees the panel on the campaign page.
    await gotoCampaignTab(page, campaignId, 'Preparação');
    await expect(page.getByRole('heading', { name: 'Galeria', exact: true, level: 2 })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Enviar imagem' })).toBeVisible();

    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const link = new URL(`/invite#t=${token}`, page.url()).toString();
      const { page: player } = await acceptInvite(playerContext, link);
      await expect(player.getByRole('heading', { level: 1 })).toBeVisible();
      await expect(player.getByRole('heading', { name: 'Membros' })).toBeVisible();
      await expect(player.getByRole('heading', { name: 'Galeria', exact: true })).toHaveCount(0);
      await expect(player.getByRole('link', { name: 'Enviar imagem' })).toHaveCount(0);

      await player.goto(`/campaigns/${campaignId}/gallery`);
      await expect(player.getByText('Só o mestre vê a galeria da campanha.')).toBeVisible();
      await expect(player.getByRole('button', { name: 'Enviar imagem' })).toHaveCount(0);

      // And the server refuses the list itself (gallery.proto).
      const listed = await callRPC(player, 'meurpg.maps.v1.GalleryService/ListGalleryImages', { campaignId });
      expect(listed.status()).toBe(403);
      expect((await listed.json()).code).toBe('permission_denied');
    } finally {
      await playerContext.close();
    }
  },
);

test(
  'o mestre renomeia uma imagem e a apaga, confirmando na própria tela',
  { tag: '@MR-019' },
  async ({ page }) => {
    await page.goto('/');
    const campaignId = await newCampaign(page, `Galeria renomear ${Date.now()}`);
    const jpeg = await canvasJpeg(page);
    await page.goto(`/campaigns/${campaignId}/gallery`);
    await uploadThroughPicker(page, [{ name: 'IMG_2031.jpg', mimeType: 'image/jpeg', buffer: jpeg }]);
    await expect(galleryCard(page, 'IMG 2031')).toBeVisible(); // the name reads like a name: "_" becomes a space

    // Rename in place.
    await galleryCard(page, 'IMG 2031').getByRole('button', { name: 'Renomear IMG 2031' }).click();
    const field = page.getByRole('textbox', { name: 'Nome da imagem' });
    await expect(field).toBeFocused();
    await field.fill('Taverna do Javali');
    await page.getByRole('button', { name: 'Salvar nome' }).click();
    const card = galleryCard(page, 'Taverna do Javali');
    await expect(card).toBeVisible();
    await page.reload();
    await expect(card).toBeVisible();

    // Delete: the first click only asks, on the card itself.
    await card.getByRole('button', { name: 'Apagar Taverna do Javali' }).click();
    await expect(card.getByText('Apagar Taverna do Javali? Não dá para desfazer.')).toBeVisible();
    const confirm = card.getByRole('button', { name: 'Apagar imagem' });
    await expect(confirm).toBeFocused();
    await card.getByRole('button', { name: 'Cancelar' }).click();
    await expect(card.getByText('Não dá para desfazer.')).toHaveCount(0);

    await card.getByRole('button', { name: 'Apagar Taverna do Javali' }).click();
    await card.getByRole('button', { name: 'Apagar imagem' }).click();
    await expect(card).toHaveCount(0);
    await expect(page.getByRole('heading', { name: 'Nenhuma imagem ainda' })).toBeVisible();
    await page.reload();
    await expect(page.getByRole('heading', { name: 'Nenhuma imagem ainda' })).toBeVisible();
  },
);

test(
  'a imagem abre em tela cheia com anterior e próxima, e Esc devolve o foco ao cartão',
  { tag: '@MR-019' },
  async ({ page }) => {
    await page.goto('/');
    const campaignId = await newCampaign(page, `Galeria ver ${Date.now()}`);
    const first = await canvasJpeg(page, '#5b4834');
    const second = await canvasJpeg(page, '#9e2b3b');
    await page.goto(`/campaigns/${campaignId}/gallery`);
    await uploadThroughPicker(page, [
      { name: 'Planta da torre.jpg', mimeType: 'image/jpeg', buffer: first },
      { name: 'Capitão Goblin.jpg', mimeType: 'image/jpeg', buffer: second },
    ]);
    await expect(galleryCard(page, 'Capitão Goblin')).toBeVisible();
    await expect(galleryCard(page, 'Planta da torre')).toBeVisible();

    // Newest first: Capitão Goblin, then Planta da torre.
    const thumb = galleryCard(page, 'Planta da torre').getByRole('button', { name: 'Ver Planta da torre' }).first();
    await thumb.click();
    const dialog = page.getByRole('dialog', { name: 'Planta da torre' });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText('2 de 2')).toBeVisible();
    const id = await imageIdOf(galleryCard(page, 'Planta da torre'));
    await expect(dialog.getByRole('img', { name: 'Planta da torre' })).toHaveAttribute('src', `/images/${id}`);

    await page.keyboard.press('ArrowRight');
    await expect(page.getByRole('dialog', { name: 'Capitão Goblin' })).toBeVisible();
    await expect(page.getByText('1 de 2')).toBeVisible();
    await page.getByRole('button', { name: 'Imagem anterior' }).click();
    await expect(dialog).toBeVisible();

    await page.keyboard.press('Escape');
    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(thumb).toBeFocused();
  },
);
