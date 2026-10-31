import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { endOpenSessionRPC, openSessionPage, startSessionRPC } from './live-session-support';
import { canvasPng, tableForMaps, uploadImageRPC } from './maps-support';
import { newSignedInContext } from './support';

// MR-028: the master shows a gallery image to the players during a session.
// Setup goes through the API; the master's and the player's open session
// pages are what is under test.

test(
  'o mestre mostra uma imagem da galeria: a página do jogador mostra "O mestre está mostrando" com a imagem e o nome, e some quando o mestre para',
  { tag: '@MR-028' },
  async ({ browser }) => {
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    let campaignId = '';
    const master = await masterContext.newPage();
    try {
      const player = await playerContext.newPage();
      await master.goto('/');
      const table = await tableForMaps(master, player, `Imagem mostrada ${Date.now()}`);
      campaignId = table.campaignId;
      await uploadImageRPC(master, campaignId, 'Capitão Goblin', await canvasPng(master, 400, 500, 'Capitão Goblin', '#3a3a2a'));
      await uploadImageRPC(master, campaignId, 'Taverna do Javali', await canvasPng(master, 800, 500, 'Taverna'));
      await startSessionRPC(master, campaignId);

      await openSessionPage(player, campaignId);
      await openSessionPage(master, campaignId);
      await expect(player.getByText('O mestre está mostrando')).toHaveCount(0);

      // The master's panel starts empty; the picker's filled button is "Mostrar aos jogadores".
      await expect(master.getByText('Nenhuma imagem à mostra.')).toBeVisible();
      await master.getByRole('button', { name: 'Mostrar imagem' }).click();
      const dialog = master.getByRole('dialog', { name: 'Mostrar uma imagem aos jogadores' });
      await dialog.getByRole('button', { name: 'Mostrar aos jogadores' }).click();
      await expect(dialog.getByText('Escolha uma imagem para mostrar.')).toBeVisible();
      await dialog.getByRole('radio', { name: /Capitão Goblin/ }).click();
      await dialog.getByRole('button', { name: 'Mostrar aos jogadores' }).click();
      await expect(dialog).toBeHidden();
      await expect(master.getByText('Capitão Goblin está na tela dos jogadores.')).toBeAttached();
      await expect(master.getByRole('button', { name: 'Parar de mostrar' })).toBeFocused();

      // The player's open page shows it, with the name as caption.
      const block = player.getByRole('region', { name: 'O mestre está mostrando' });
      await expect(block).toBeVisible();
      await expect(block.getByRole('img', { name: 'Capitão Goblin' })).toBeVisible();
      await expect(block.getByText('Capitão Goblin', { exact: true })).toBeVisible();
      await expect(player.getByText('O mestre está mostrando Capitão Goblin.')).toBeAttached();

      // Swapping changes it in place.
      await master.getByRole('button', { name: 'Trocar imagem' }).click();
      const swap = master.getByRole('dialog', { name: 'Mostrar uma imagem aos jogadores' });
      await expect(swap.getByText('À mostra agora')).toBeVisible();
      await swap.getByRole('radio', { name: /Taverna do Javali/ }).click();
      await swap.getByRole('button', { name: 'Mostrar aos jogadores' }).click();
      await expect(block.getByRole('img', { name: 'Taverna do Javali' })).toBeVisible();

      // Stopping takes it off the player's screen, at once for the master.
      await master.getByRole('button', { name: 'Parar de mostrar' }).click();
      await expect(master.getByText('Imagem retirada da tela dos jogadores.')).toBeAttached();
      await expect(master.getByRole('button', { name: 'Mostrar imagem' })).toBeFocused();
      await expect(player.getByRole('region', { name: 'O mestre está mostrando' })).toHaveCount(0);
      await expect(player.getByText('O mestre parou de mostrar a imagem.')).toBeAttached();
    } finally {
      await endOpenSessionRPC(master, campaignId);
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'o mestre deixa a imagem com os jogadores: ela continua na página do jogador depois de parar, abre em tela cheia, e some quando o mestre a tira',
  { tag: '@MR-028' },
  async ({ browser }) => {
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    let campaignId = '';
    const master = await masterContext.newPage();
    try {
      const player = await playerContext.newPage();
      await master.goto('/');
      const table = await tableForMaps(master, player, `Imagem deixada ${Date.now()}`);
      campaignId = table.campaignId;
      const imageId = await uploadImageRPC(master, campaignId, 'Planta da torre', await canvasPng(master, 800, 600, 'Torre', '#5b4834'));
      await startSessionRPC(master, campaignId);
      await openSessionPage(player, campaignId);
      await openSessionPage(master, campaignId);

      // Show it with "Deixar com os jogadores" on (off by default).
      await master.getByRole('button', { name: 'Mostrar imagem' }).click();
      const dialog = master.getByRole('dialog', { name: 'Mostrar uma imagem aos jogadores' });
      await dialog.getByRole('radio', { name: /Planta da torre/ }).click();
      await dialog.getByRole('button', { name: 'Mostrar aos jogadores' }).click();
      const keep = master.getByRole('switch', { name: 'Deixar com os jogadores' });
      await expect(keep).toHaveAttribute('aria-checked', 'false');
      await keep.click();
      await expect(keep).toHaveAttribute('aria-checked', 'true');
      await expect(master.getByText('Ligado')).toBeVisible();
      await expect(player.getByRole('region', { name: 'Imagens que o mestre deixou' })).toHaveCount(0);

      // Stopping leaves it with the players.
      await master.getByRole('button', { name: 'Parar de mostrar' }).click();
      await expect(master.getByText('Planta da torre continua com os jogadores.')).toBeAttached();
      await expect(master.getByRole('region', { name: 'Imagem para os jogadores' }).getByRole('heading', { name: 'Deixadas com os jogadores' })).toBeVisible();
      const left = player.getByRole('region', { name: 'Imagens que o mestre deixou' });
      await expect(left).toBeVisible();
      await expect(player.getByRole('region', { name: 'O mestre está mostrando' })).toHaveCount(0);
      await expect(left.getByText('Planta da torre', { exact: true })).toBeVisible();

      // The player opens it full screen, and the file is served to them.
      await left.getByRole('button', { name: 'Ver Planta da torre em tela cheia' }).click();
      const viewer = player.getByRole('dialog');
      await expect(viewer.getByRole('img', { name: 'Planta da torre' })).toBeVisible();
      await viewer.getByRole('button', { name: 'Fechar' }).click();
      expect((await player.request.get(`/images/${imageId}`)).status()).toBe(200);

      // "Tirar": it leaves the player's page without a reload, and its URL answers 404.
      await master.getByRole('button', { name: 'Tirar Planta da torre dos jogadores' }).click();
      await expect(master.getByText('Planta da torre foi tirada.')).toBeAttached();
      await expect(master.getByRole('heading', { name: 'Deixadas com os jogadores' })).toHaveCount(0);
      await expect(master.getByRole('button', { name: 'Mostrar imagem' })).toBeFocused();
      await expect(left).toHaveCount(0);
      expect((await player.request.get(`/images/${imageId}`)).status()).toBe(404);
      expect((await player.request.get(`/images/${imageId}/thumb`)).status()).toBe(404);
      expect((await master.request.get(`/images/${imageId}`)).status()).toBe(200);
    } finally {
      await endOpenSessionRPC(master, campaignId);
      await masterContext.close();
      await playerContext.close();
    }
  },
);
