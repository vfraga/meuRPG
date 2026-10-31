import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { toren, torenSheet } from './combat-support';
import { liveSessionAsPlayer } from './images-support';
import { endOpenSessionRPC, openSessionPage } from './live-session-support';
import { canvasPng, uploadImageRPC } from './maps-support';
import { getOpenSceneRPC, openSceneRPC, setSceneImagesRPC, tableForScenes } from './scene-support';
import { createMiraRPC, getStageRPC, putOnStageRPC } from './stage-support';
import { callRPC, newSignedInContext } from './support';
import { secondPlayer } from './theatre-support';

// MR-015, MR-028 and MR-031 with RN-10, the master's side of who sees what in a scene, through the screens: the eye of a stage NPC
// and of a scene action, "Quem está na cena" with "Os jogadores podem entrar", and who sees a shown image. What each player
// reads is also checked through the API: a player never gets a hidden thing. Setup goes through the API; every test makes its own campaign.

test(
  'o mestre esconde a Mira e uma ação com o olho: o jogador não recebe nenhuma das duas, e quando o mestre as mostra de novo elas voltam',
  { tag: ['@MR-015', '@MR-031', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    const m = await masterContext.newPage();
    const p = await playerContext.newPage();
    let campaignId = '';
    try {
      await m.goto('/');
      const table = await tableForScenes(m, p, `Olho ${Date.now()}`, true);
      campaignId = table.campaignId;
      const miraId = await createMiraRPC(m, campaignId);
      await openSceneRPC(m, campaignId, table.cartId);
      await putOnStageRPC(m, campaignId, miraId);
      await openSessionPage(m, campaignId);
      await openSessionPage(p, campaignId);
      const stage = p.getByRole('group', { name: 'Em cena: Mira' });
      await expect(stage).toBeVisible();
      const row = m.locator('[data-stage-card]').filter({ hasText: 'Mira' });
      await expect(row).toContainText('Os jogadores veem');

      // The NPC's eye: hidden from the players, the speech button dashed and off.
      await m.getByRole('button', { name: 'Esconder Mira dos jogadores' }).click();
      await expect(row).toContainText('Escondida dos jogadores');
      await expect(row.getByRole('button', { name: /Dar a fala: Mira está escondida dos jogadores e não fala/ })).toHaveAttribute('aria-disabled', 'true');
      await expect(stage).toHaveCount(0);
      expect(await getStageRPC(p, campaignId)).toEqual([]);
      expect((await getStageRPC(m, campaignId)).map((n) => n.name)).toEqual(['Mira']);
      await m.getByRole('button', { name: 'Mostrar Mira aos jogadores' }).click();
      await expect(row).toContainText('Os jogadores veem');
      await expect(p.getByRole('group', { name: 'Em cena: Mira' })).toBeVisible();

      // The action's eye: "Escondida" for the master, no trace of it for the player.
      const actions = m.getByRole('region', { name: 'Ações e rolagens' });
      await actions.getByRole('button', { name: 'Esconder Acalmar os cavalos dos jogadores' }).click();
      await expect(actions.locator('.sa__item').filter({ hasText: 'Acalmar os cavalos' })).toContainText('Escondida');
      await expect(p.getByText('Acalmar os cavalos')).toHaveCount(0);
      expect(JSON.stringify(await getOpenSceneRPC(p, campaignId))).not.toContain('Acalmar os cavalos');
      expect(JSON.stringify(await getOpenSceneRPC(m, campaignId))).toContain('Acalmar os cavalos');
      await actions.getByRole('button', { name: 'Mostrar Acalmar os cavalos aos jogadores' }).click();
      await expect(p.getByText('Acalmar os cavalos').first()).toBeVisible();
    } finally {
      await endOpenSessionRPC(m, campaignId);
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'o mestre abre a cena só para a Pensantus: o Toren não a vê, e com "Os jogadores podem entrar" ele pode entrar e o mestre lê quem entrou',
  { tag: ['@MR-015', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    const m = await masterContext.newPage();
    const p = await playerContext.newPage();
    let campaignId = '';
    let second: Awaited<ReturnType<typeof secondPlayer>> | null = null;
    try {
      await m.goto('/');
      const table = await tableForScenes(m, p, `Quem está ${Date.now()}`, true);
      campaignId = table.campaignId;
      second = await secondPlayer(browser, m, campaignId, toren, torenSheet);
      await openSceneRPC(m, campaignId, table.cartId);
      await openSessionPage(m, campaignId);
      await openSessionPage(second.page, campaignId);
      await openSessionPage(p, campaignId);
      await expect(m.getByText(/^Aberta para os jogadores desde \d\d:\d\d$/)).toBeVisible();
      await expect(second.page.getByRole('region', { name: 'Cena: A carroça tombada' })).toBeVisible();

      // "Quem está na cena": the chooser starts with everyone; Toren is taken out.
      await m.getByRole('button', { name: 'Quem está na cena' }).click();
      const chooser = m.getByRole('dialog', { name: 'Quem está na cena?' });
      await expect(chooser.getByText('Toda a mesa está na cena.')).toBeVisible();
      await chooser.getByRole('checkbox', { name: /Toren/ }).uncheck();
      await expect(chooser.getByText('Pensantus está na cena.')).toBeVisible();
      await chooser.getByRole('button', { name: 'Pronto' }).click();
      await expect(chooser).toBeHidden();
      await expect(m.getByText(/^Aberta desde \d\d:\d\d para$/)).toBeVisible();
      await expect(m.getByRole('list', { name: 'Quem está na cena' })).toContainText('Pensantus');
      await expect(m.getByText('Toren não está nesta cena.')).toBeVisible();
      await expect(second.page.getByRole('region', { name: 'Cena: A carroça tombada' })).toHaveCount(0);
      expect(JSON.stringify(await getOpenSceneRPC(second.page, campaignId))).not.toContain('A carroça tombada');
      await expect(p.getByRole('region', { name: 'Cena: A carroça tombada' })).toBeVisible();

      // "Os jogadores podem entrar": off, Toren reads no scene; on, he reads only its name and may join.
      const mayJoin = m.getByRole('switch', { name: 'Os jogadores podem entrar' });
      await expect(mayJoin).toHaveAttribute('aria-checked', 'false');
      await mayJoin.click();
      await expect(mayJoin).toHaveAttribute('aria-checked', 'true');
      const seen = await getOpenSceneRPC(second.page, campaignId);
      expect(seen.scene).toMatchObject({ name: 'A carroça tombada', joinable: true });
      expect(JSON.stringify(seen)).not.toContain('Procurar pistas na carroça');
      const joined = await callRPC(second.page, 'meurpg.play.v1.PlayService/JoinScene', { campaignId });
      expect(joined.ok(), await joined.text()).toBeTruthy();
      // The line starts with its icon's ligature, so the words are matched without the start anchor.
      await expect(m.getByText(/Toren entrou na cena às \d\d:\d\d\.$/)).toBeVisible();
      await expect(m.getByRole('list', { name: 'Quem está na cena' })).toContainText('Toren');

      // A scene open to everyone again has no switch: nobody to let in.
      await m.getByRole('button', { name: 'Quem está na cena' }).click();
      const again = m.getByRole('dialog', { name: 'Quem está na cena?' });
      await again.getByRole('checkbox', { name: 'Todos' }).check();
      await again.getByRole('button', { name: 'Pronto' }).click();
      await expect(m.getByText(/^Aberta para os jogadores desde \d\d:\d\d$/)).toBeVisible();
      await expect(m.getByRole('switch', { name: 'Os jogadores podem entrar' })).toHaveCount(0);
    } finally {
      await endOpenSessionRPC(m, campaignId);
      await second?.close();
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'numa cena aberta só para a Pensantus o mestre mostra uma imagem para quem está na cena, amplia para a mesa toda e escolhe quem vê',
  { tag: ['@MR-015', '@MR-028', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    const m = await masterContext.newPage();
    const p = await playerContext.newPage();
    let campaignId = '';
    let second: Awaited<ReturnType<typeof secondPlayer>> | null = null;
    try {
      await m.goto('/');
      const table = await tableForScenes(m, p, `Imagem para quem ${Date.now()}`, false);
      campaignId = table.campaignId;
      const toren2 = await secondPlayer(browser, m, campaignId, toren, torenSheet);
      second = toren2;
      const hall = await uploadImageRPC(m, campaignId, 'Vista do salão', await canvasPng(m, 640, 400, 'Vista do salão', '#7a5a40'));
      const cellar = await uploadImageRPC(m, campaignId, 'O porão', await canvasPng(m, 640, 400, 'O porão', '#5a4b42'));
      await setSceneImagesRPC(m, table, table.cartId, [hall, cellar]);
      await openSceneRPC(m, campaignId, table.cartId);
      await openSessionPage(m, campaignId);
      await m.getByRole('button', { name: 'Quem está na cena' }).click();
      const who = m.getByRole('dialog', { name: 'Quem está na cena?' });
      await who.getByRole('checkbox', { name: /Toren/ }).uncheck();
      await who.getByRole('button', { name: 'Pronto' }).click();
      await expect(who).toBeHidden();
      const images = m.getByRole('region', { name: 'Imagens da cena' });
      const shownTo = async (page: typeof p) => (await liveSessionAsPlayer(page, campaignId)).shownImage?.id ?? '';

      // On a partial scene "Mostrar" is a menu: the default is those in the scene.
      await images.getByRole('button', { name: 'Mostrar Vista do salão para…' }).click();
      const menu = m.getByRole('menu');
      await expect(menu.getByText('Mostrar “Vista do salão” para')).toBeVisible();
      await expect(menu.getByRole('menuitem', { name: /Só quem está na cena/ })).toContainText('Pensantus');
      await expect(menu.getByRole('menuitem', { name: /Toda a mesa/ })).toContainText('Toren');
      await menu.getByRole('menuitem', { name: /Só quem está na cena/ }).click();
      await expect(images.getByText('Pensantus vê')).toBeVisible();
      expect(await shownTo(p)).toBe(hall);
      expect(await shownTo(toren2.page)).toBe('');
      expect((await toren2.page.request.get(`/images/${hall}`)).status()).toBe(404);

      // "Mostrar a toda a mesa": Toren gets it too.
      await images.getByRole('button', { name: 'Mostrar a toda a mesa' }).click();
      await expect(images.getByText('Toda a mesa vê')).toBeVisible();
      await expect.poll(() => shownTo(toren2.page)).toBe(hall);

      // "Mudar quem vê": the chooser opens with everyone; only Toren is kept, and Pensantus loses it at once.
      await images.getByRole('button', { name: 'Mudar quem vê' }).click();
      const chooser = m.getByRole('dialog', { name: 'Quem vê “Vista do salão”?' });
      await chooser.getByRole('checkbox', { name: /Pensantus/ }).uncheck();
      await expect(chooser.getByText('Toren vê a imagem.')).toBeVisible();
      await chooser.getByRole('button', { name: 'Pronto' }).click();
      await expect(images.getByText('Só Toren vê')).toBeVisible();
      await expect.poll(() => shownTo(p)).toBe('');
      expect(await shownTo(toren2.page)).toBe(hall);
    } finally {
      await endOpenSessionRPC(m, campaignId);
      await second?.close();
      await masterContext.close();
      await playerContext.close();
    }
  },
);
