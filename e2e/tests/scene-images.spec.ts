import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { endOpenSessionRPC, openSessionPage } from './live-session-support';
import { canvasPng, uploadImageRPC } from './maps-support';
import { getOpenSceneRPC, openSceneRPC, tableForScenes } from './scene-support';
import { callRPC, newSignedInContext } from './support';

// MR-015 (the images of an RP scene, "Imagens da cena"), through the screens:
// the master attaches gallery images to a scene point and orders them, opens the
// scene in a session and shows one to the players with the gallery's own action
// (MR-019). The list is the master's: a player never receives it (RN-10), and
// sees an image only when it is shown. Setup goes through the API.

test(
  'o mestre põe duas imagens da galeria na cena, muda a ordem, abre a cena e mostra uma ao jogador, que só vê a que foi mostrada',
  { tag: ['@MR-015', '@MR-019', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(120_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    await master.goto('/');
    const table = await tableForScenes(master, player, `Imagens da cena ${Date.now()}`, false);
    const campaignId = table.campaignId;
    try {
      const first = await uploadImageRPC(master, campaignId, 'Vista da carroça', await canvasPng(master, 640, 400, 'Vista da carroça', '#4b6b8a'));
      const second = await uploadImageRPC(master, campaignId, 'Rastros na lama', await canvasPng(master, 640, 400, 'Rastros na lama', '#6b8a4b'));

      // The editor: the section is there, empty, and says the players never see the list.
      await master.goto(`/campaigns/${campaignId}/maps/${table.mapId}`);
      await master.getByRole('button', { name: /^A carroça tombada, Cena de RP/ }).click();
      await expect(master.getByRole('heading', { name: 'Imagens da cena' })).toBeVisible();
      await expect(master.getByText('Nenhuma imagem ainda.')).toBeVisible();

      // "Escolher da galeria": two pictures, each put on the scene by the dialog.
      for (const name of ['Vista da carroça', 'Rastros na lama']) {
        await master.getByRole('button', { name: 'Escolher da galeria' }).click();
        const picker = master.getByRole('dialog', { name: 'Escolher da galeria' });
        await picker.getByRole('radio', { name: new RegExp(name) }).click();
        await picker.getByRole('button', { name: 'Pôr na cena' }).click();
        await expect(picker).toBeHidden();
        await expect(master.getByText(`Imagem ${name === 'Vista da carroça' ? 1 : 2}: ${name}`)).toBeVisible();
      }
      await expect(master.getByText('2 de 8', { exact: true })).toBeVisible();
      // Already on the scene: the picker no longer offers it.
      await master.getByRole('button', { name: 'Escolher da galeria' }).click();
      const again = master.getByRole('dialog', { name: 'Escolher da galeria' });
      await expect(again.getByRole('radio', { name: /Vista da carroça/ })).toHaveCount(0);
      await again.getByRole('button', { name: 'Cancelar' }).click();

      // Reorder: the second goes up, with focus staying on the same button; it survives a reload.
      await master.getByRole('button', { name: /^Subir a imagem 2, Rastros na lama/ }).click();
      await expect(master.getByRole('button', { name: /^Subir a imagem 1, Rastros na lama/ })).toBeFocused();
      await master.reload();
      await master.getByRole('button', { name: /^A carroça tombada, Cena de RP/ }).click();
      await expect(master.locator('.si__row').first()).toContainText('Rastros na lama');
      await expect(master.locator('.si__row').nth(1)).toContainText('Vista da carroça');

      // The session: open the scene; the master's card has both pictures.
      await openSessionPage(master, campaignId);
      await openSceneRPC(master, campaignId, table.cartId);
      const images = master.getByRole('region', { name: 'Imagens da cena' });
      await expect(images).toBeVisible();
      const tiles = images.locator('.sm__tile:not(.sm__tile--add)');
      await expect(tiles).toHaveCount(2);
      await expect(tiles.first()).toContainText('Rastros na lama');
      // The dashed "Da galeria" tile ends the row.
      await expect(images.getByRole('button', { name: 'Da galeria' })).toBeVisible();

      // The player: the scene is open, but nothing says which pictures it holds.
      await openSessionPage(player, campaignId);
      await expect(player.getByRole('region', { name: 'Cena: A carroça tombada' })).toBeVisible();
      await expect(player.getByText('Rastros na lama')).toHaveCount(0);
      await expect(player.getByRole('region', { name: 'O mestre está mostrando' })).toHaveCount(0);
      const playerScene = await getOpenSceneRPC(player, campaignId);
      expect(JSON.stringify(playerScene)).not.toContain(first);
      expect(JSON.stringify(playerScene)).not.toContain(second);
      const masterScene = await getOpenSceneRPC(master, campaignId);
      expect(JSON.stringify(masterScene)).toContain(first);
      expect(JSON.stringify(masterScene)).toContain(second);
      const playerMap = await callRPC(player, 'meurpg.maps.v1.MapService/GetMap', { campaignId, mapId: table.mapId });
      expect(playerMap.ok(), await playerMap.text()).toBeTruthy();
      expect(await playerMap.text()).not.toContain(first);

      // "Mostrar aos jogadores": the gallery's own action; the shown one is marked.
      await images.getByRole('button', { name: 'Mostrar Rastros na lama aos jogadores' }).click();
      await expect(tiles.first()).toContainText('À mostra');
      // The scene's image row keeps the one "Parar de mostrar" (the thumbnail only says "À mostra").
      await expect(master.getByRole('button', { name: 'Parar de mostrar', exact: true })).toBeVisible();
      const shown = player.getByRole('region', { name: 'O mestre está mostrando' });
      await expect(shown).toBeVisible();
      await expect(shown.getByRole('img', { name: 'Rastros na lama' })).toBeVisible();
      // The other picture is still the master's.
      expect((await player.request.get(`/images/${second}`)).ok()).toBe(true);
      expect((await player.request.get(`/images/${first}`)).ok()).toBe(false);

      // Taking one off the scene keeps it in the gallery.
      await master.goto(`/campaigns/${campaignId}/maps/${table.mapId}`);
      await master.getByRole('button', { name: /^A carroça tombada, Cena de RP/ }).click();
      await master.getByRole('button', { name: /^Tirar a imagem 2 da cena, Vista da carroça/ }).click();
      await expect(master.locator('.si__row')).toHaveCount(1);
      const list = await callRPC(master, 'meurpg.maps.v1.GalleryService/ListGalleryImages', { campaignId });
      expect(await list.text()).toContain(first);
    } finally {
      await endOpenSessionRPC(master, campaignId);
      await masterContext.close();
      await playerContext.close();
    }
  },
);
