import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { beginFogCombat, tableForFog } from './fog-support';
import { endOpenSessionRPC, openSessionPage } from './live-session-support';
import {
  addTvContent,
  beginCrowdedCombat,
  chooseRow,
  closeTvPanel,
  inspectOrder,
  ringRoom,
  sceneBoxes,
  twoOnStageRPC,
  expectPanelFits,
  pageOverflow,
  openTvPanel,
  openTvTab,
  putMapOnTable,
  putSceneOnTable,
  recordTraffic,
  rollForTableRPC,
  showImageRPC,
  storageOf,
  tableForTv,
  tvButton,
  tvRoute,
  tvTitle,
} from './table-screen-support';
import { callRPC, newSignedInContext } from './support';

// #648 "Tela da mesa": the master opens the TV tab from the live session and chooses what it shows; the TV is a tab signed in as the
// master that reads only what every player may read (RN-10). Setup goes through the API; the control, the TV tab and the requests the TV
// tab makes are what is under test.

const TABLE_SCREEN = '/meurpg.play.v1.TableScreenService/';
const GET_ME = '/meurpg.identity.v1.IdentityService/GetMe';

test(
  'o mestre abre a Tela da mesa pela barra da sessão: a TV espera, espelha o que ele mostra, fixa outro item, recusa o que não existe e guarda a escolha ao recarregar',
  { tag: '@RN-10' },
  async ({ browser }) => {
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    let campaignId = '';
    const master = await masterContext.newPage();
    try {
      const player = await playerContext.newPage();
      await Promise.all([master.goto('/'), player.goto('/')]);
      const table = await tableForTv(master, player, `Tela da mesa ${Date.now()}`);
      campaignId = table.campaignId;
      const content = await addTvContent(master, campaignId);
      await openSessionPage(master, campaignId);

      // The bar offers the panel; before any TV is open it says so and offers the link.
      await expect(tvButton(master)).toContainText('Tela da mesa');
      let panel = await openTvPanel(master);
      await expect(panel.getByTestId('tv-state')).toContainText('Fechada');
      await expect(panel.getByTestId('tv-closed')).toBeVisible();

      // The link opens the TV in a new tab of the master's browser: idle, because nothing is on the table.
      const tv = await openTvTab(masterContext, panel, campaignId);
      await expect(tv.getByRole('heading', { level: 2, name: 'Nada à mostra ainda' })).toBeVisible();
      await expect(tvTitle(tv)).toContainText('Sessão 1');
      await expect(panel.getByTestId('tv-state')).toContainText('Aberta');
      await expect(panel.getByTestId('tv-now')).toContainText('A tela de espera.');
      await expect(panel.getByTestId('tv-mirror').getByRole('radio')).toBeChecked();

      // Espelhar: the map the session has, then a roll for the table, the scene and the image, each over the one before.
      await putMapOnTable(master, campaignId, content.mapId);
      await expect(tvTitle(tv)).toHaveText('Estrada do Vale');
      await expect(tv.getByRole('region', { name: 'Mapa Estrada do Vale' })).toBeVisible();
      await rollForTableRPC(master, campaignId);
      await expect(tv.getByTestId('tv-roll-last')).toContainText('1d20');

      await putSceneOnTable(master, campaignId, content.sceneId);
      await expect(tvTitle(tv)).toHaveText('A carroça tombada');
      await expect(tv.getByText('Uma carroça de mercador tombada na estrada.')).toBeVisible();
      await expect(tv.getByRole('region', { name: 'Mapa Estrada do Vale' })).toHaveCount(0);

      await showImageRPC(master, campaignId, content.imageId);
      await expect(tvTitle(tv)).toHaveText('Capitão Goblin');
      await expect(tv.getByRole('img', { name: 'Capitão Goblin' })).toBeVisible();

      // The panel reads the table when it opens (the master's page is behind it): opened again, it says what the TV shows now.
      await closeTvPanel(master);
      panel = await openTvPanel(master);
      await expect(panel.getByTestId('tv-now')).toContainText('A imagem Capitão Goblin.');

      // Fixar: the map, while the image and the scene are still on the table. The bar says it is pinned.
      await chooseRow(panel, 'tv-pin-map');
      await expect(tvTitle(tv)).toHaveText('Estrada do Vale');
      await expect(tv.getByRole('img', { name: 'Capitão Goblin' })).toHaveCount(0);
      await expect(panel.getByTestId('tv-now')).toContainText('O mapa Estrada do Vale, fixa.');
      await expect(tvButton(master)).toContainText('Na TV: Mapa');
      await expect(panel.getByTestId('tv-mirror').getByRole('radio')).not.toBeChecked();

      // A pin with nothing behind it is dashed, says why, and cannot be chosen.
      const combatRow = panel.getByTestId('tv-pin-combat');
      await expect(combatRow).toContainText('Nenhum combate agora.');
      await expect(combatRow.getByRole('radio')).toBeDisabled();
      await expect(combatRow.locator('.row')).toHaveCSS('border-top-style', 'dashed');
      await expect(panel.getByTestId('tv-pin-scene').getByRole('radio')).toBeEnabled();

      // The choice is stored, not the tab's: the reloaded TV still shows the pinned map, not the image over it.
      await tv.reload();
      await expect(tvTitle(tv)).toHaveText('Estrada do Vale');
      await expect(tv.getByRole('img', { name: 'Capitão Goblin' })).toHaveCount(0);
      await expect(panel.getByTestId('tv-state')).toContainText('Aberta');

      // The pinned image ends: the TV falls back to the waiting screen and says why, in the TV and in the panel.
      await chooseRow(panel, 'tv-pin-image');
      await expect(tvTitle(tv)).toHaveText('Capitão Goblin');
      await showImageRPC(master, campaignId, '');
      await expect(tv.getByTestId('tv-notice')).toContainText('A imagem saiu da mesa.');
      await expect(tv.getByRole('heading', { level: 2, name: 'Nada à mostra ainda' })).toBeVisible();
      await closeTvPanel(master);
      panel = await openTvPanel(master);
      await expect(panel.getByTestId('tv-notice')).toContainText('A imagem saiu da mesa.');

      // Espelhar again follows the table: the scene is the first thing there.
      await chooseRow(panel, 'tv-mirror');
      await expect(tvTitle(tv)).toHaveText('A carroça tombada');
      await expect(tv.getByTestId('tv-notice')).toHaveCount(0);
    } finally {
      await endOpenSessionRPC(master, campaignId);
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test('um jogador que abre a Tela da mesa vê a recusa, sem nada da mesa', { tag: '@RN-10' }, async ({ browser }) => {
  const masterContext = await newSignedInContext(browser, 'Mestre Teste');
  const playerContext = await newSignedInContext(browser, 'Jogador Teste');
  let campaignId = '';
  const master = await masterContext.newPage();
  try {
    const player = await playerContext.newPage();
    await Promise.all([master.goto('/'), player.goto('/')]);
    const table = await tableForTv(master, player, `Tela da mesa recusada ${Date.now()}`);
    campaignId = table.campaignId;
    const content = await addTvContent(master, campaignId);
    await putMapOnTable(master, campaignId, content.mapId);

    // The master sees the table; the player gets the card of the refusal and nothing of the campaign.
    await player.goto(tvRoute(campaignId));
    await expect(player.getByText('Só o mestre abre a Tela da mesa.')).toBeVisible();
    await expect(player.getByText('Ela é a tela da sala; você joga pela sessão no seu aparelho.')).toBeVisible();
    await expect(player.getByText('Estrada do Vale')).toHaveCount(0);
    await expect(player.getByText('Pensantus')).toHaveCount(0);
    const asked = await callRPC(player, 'meurpg.play.v1.TableScreenService/GetTableScreen', { campaignId });
    expect(asked.status()).toBe(403);
  } finally {
    await endOpenSessionRPC(master, campaignId);
    await masterContext.close();
    await playerContext.close();
  }
});

test(
  'a aba da TV só fala com a Tela da mesa: nenhuma chamada aos dados da sessão do mestre, e os ladrilhos de mapa são os da TV',
  { tag: '@RN-10' },
  async ({ browser }) => {
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const pensantusContext = await newSignedInContext(browser, 'Jogador Teste');
    const torenContext = await newSignedInContext(browser, 'E-mail Não Verificado');
    let campaignId = '';
    const master = await masterContext.newPage();
    try {
      const pensantusPage = await pensantusContext.newPage();
      const torenPage = await torenContext.newPage();
      await Promise.all([master.goto('/'), pensantusPage.goto('/'), torenPage.goto('/')]);
      const table = await tableForFog(master, pensantusPage, torenPage, `Tela da mesa rede ${Date.now()}`);
      campaignId = table.campaignId;
      // With the fog on, the TV draws only what the group sees: "Visão do grupo" on.
      const fog = await callRPC(master, 'meurpg.maps.v1.MapService/SetMapFog', { campaignId, mapId: table.mapId, groupVision: true });
      expect(fog.ok(), await fog.text()).toBeTruthy();

      // The TV tab is recorded from its first request, through a map, a roll and a combat on that map.
      const tv = await masterContext.newPage();
      const traffic = recordTraffic(tv);
      await tv.goto(tvRoute(campaignId));
      const tiles = tv.locator('app-fog-map img[src$="as=tv"]');
      await expect(tiles.first()).toBeVisible();
      await rollForTableRPC(master, campaignId);
      await expect(tv.getByTestId('tv-roll-last')).toBeVisible();
      await beginFogCombat(master, table);
      await expect(tvTitle(tv)).toContainText('Combate');
      await expect(tv.getByRole('heading', { level: 2, name: 'Ordem' })).toBeVisible();
      await expect(tv.getByText('Rodada 1')).toBeVisible();
      await expect(tiles.first()).toBeVisible();

      const procedures = traffic.procedures();
      // The positive control: the recorder saw the TV's own calls.
      expect(procedures, 'the recorder did not see the TV read the table screen').toContain(`${TABLE_SCREEN}GetTableScreen`);
      expect(procedures, 'the recorder did not see the TV open its stream').toContain(`${TABLE_SCREEN}WatchTableScreen`);

      // The master's own session data, one by one so the failure names the call.
      for (const forbidden of ['GetLiveSession', 'GetEncounter', 'ListCombatLog', 'ListOpenGameSessions']) {
        const calls = procedures.filter((path) => path.endsWith(`/${forbidden}`));
        expect(calls, `the TV tab called ${forbidden}, a read of the master's session`).toEqual([]);
      }
      // Anything else outside TableScreenService (and GetMe, for the route's guard).
      const outside = [...new Set(procedures.filter((path) => !path.startsWith(TABLE_SCREEN) && path !== GET_ME))];
      expect(outside, `the TV tab called procedures outside TableScreenService: ${outside.join(', ')}`).toEqual([]);

      // The images: tiles of the TV, never the map's picture or a tile of the master's own view.
      const pictures = traffic.images().filter((path) => path.startsWith('/maps/') || path.startsWith('/images/'));
      expect(pictures.length, 'the TV drew no map tile').toBeGreaterThan(0);
      const notTv = pictures.filter((path) => !/\/tiles\//.test(path) || !path.endsWith('as=tv'));
      expect(notTv, `the TV tab asked for images that are not map tiles of the TV: ${notTv.join(', ')}`).toEqual([]);
    } finally {
      await endOpenSessionRPC(master, campaignId);
      await masterContext.close();
      await pensantusContext.close();
      await torenContext.close();
    }
  },
);

test(
  '"Copiar o link" põe só o endereço da Tela da mesa na área de transferência, sem consulta nem chave, e o painel não grava nada no navegador',
  { tag: '@RN-10' },
  async ({ browser }) => {
    const masterContext = await newSignedInContext(browser, 'Mestre Teste', { permissions: ['clipboard-read', 'clipboard-write'] });
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    let campaignId = '';
    const master = await masterContext.newPage();
    try {
      const player = await playerContext.newPage();
      await Promise.all([master.goto('/'), player.goto('/')]);
      const table = await tableForTv(master, player, `Tela da mesa link ${Date.now()}`);
      campaignId = table.campaignId;
      await openSessionPage(master, campaignId);
      const tv = await masterContext.newPage();
      await tv.goto(tvRoute(campaignId));
      await expect(tv.getByRole('heading', { level: 2, name: 'Nada à mostra ainda' })).toBeVisible();

      // Everything the browser keeps for the app before the panel is opened.
      const before = await storageOf(master);

      const panel = await openTvPanel(master);
      await expect(panel.getByTestId('tv-state')).toContainText('Aberta');
      await chooseRow(panel, 'tv-pin-idle');
      await expect(panel.getByTestId('tv-now')).toContainText('A tela de espera, fixa.');
      await chooseRow(panel, 'tv-mirror');
      await expect(panel.getByTestId('tv-now')).toContainText('A tela de espera.');
      await panel.locator('.js-copy').click();
      await expect(panel.locator('.js-copy')).toContainText('Link copiado');

      const origin = new URL(master.url()).origin;
      const copied = await master.evaluate(() => navigator.clipboard.readText());
      expect(copied).toBe(`${origin}${tvRoute(campaignId)}`);
      const url = new URL(copied);
      expect(url.search).toBe('');
      expect(url.hash).toBe('');
      expect(url.username + url.password).toBe('');

      // The panel kept nothing: no choice, no link, no token in either store.
      expect(await storageOf(master)).toBe(before);
    } finally {
      await endOpenSessionRPC(master, campaignId);
      await masterContext.close();
      await playerContext.close();
    }
  },
);

for (const size of [
  { width: 390, height: 844 },
  { width: 1280, height: 800 },
  { width: 1280, height: 720 },
]) {
  test(
    `o painel da Tela da mesa cabe numa tela sem rolar, em ${size.width}x${size.height}: fixo com linha tracejada e aviso, ⓘ aberto, falha e sem conexão`,
    { tag: '@RN-10' },
    async ({ browser }) => {
      const masterContext = await newSignedInContext(browser, 'Mestre Teste', { viewport: size });
      const playerContext = await newSignedInContext(browser, 'Jogador Teste');
      let campaignId = '';
      const master = await masterContext.newPage();
      try {
        const player = await playerContext.newPage();
        await Promise.all([master.goto('/'), player.goto('/')]);
        const table = await tableForTv(master, player, `Tela da mesa cabe ${Date.now()}`);
        campaignId = table.campaignId;
        const content = await addTvContent(master, campaignId);
        await putMapOnTable(master, campaignId, content.mapId);
        await openSessionPage(master, campaignId);
        const tv = await masterContext.newPage();
        await tv.goto(tvRoute(campaignId));
        await expect(tvTitle(tv)).toHaveText('Estrada do Vale');

        // Espelhar, with the dashed rows and their reasons.
        let panel = await openTvPanel(master);
        await expect(panel.getByTestId('tv-state')).toContainText('Aberta');
        await expect(panel.getByTestId('tv-pin-combat')).toContainText('Nenhum combate agora.');
        await expectPanelFits(panel, 'espelhando');

        // The longest: pinned to an image that has ended, so the notice stands over the rows.
        await showImageRPC(master, campaignId, content.imageId);
        await closeTvPanel(master);
        panel = await openTvPanel(master);
        await chooseRow(panel, 'tv-pin-image');
        await showImageRPC(master, campaignId, '');
        await expect(tv.getByTestId('tv-notice')).toContainText('A imagem saiu da mesa.');
        await closeTvPanel(master);
        panel = await openTvPanel(master);
        await expect(panel.getByTestId('tv-notice')).toContainText('A imagem saiu da mesa.');
        await expect(panel.getByTestId('tv-pin-image')).toContainText('Nenhuma imagem à mostra.');
        await expectPanelFits(panel, 'fixo, com o aviso e as linhas tracejadas');

        // The ⓘ of "Espelhar": the order of the mirror takes the rows' place.
        await panel.getByRole('button', { name: 'Como o Espelhar escolhe' }).click();
        await expect(panel.getByTestId('tv-order')).toBeVisible();
        await expectPanelFits(panel, 'com o ⓘ aberto');
        await panel.getByRole('button', { name: 'Como o Espelhar escolhe' }).click();

        // A choice that is not stored: the notice and "Tentar de novo".
        await master.route('**/*TableScreenService/SetTableScreen', (route) => route.abort());
        await panel.getByTestId('tv-mirror').locator('label').click();
        await expect(panel.getByTestId('tv-not-saved')).toBeVisible();
        await expectPanelFits(panel, 'com a escolha que não foi gravada');
        await master.unroute('**/*TableScreenService/SetTableScreen');
        await panel.locator('.js-retry').click();
        await expect(panel.getByTestId('tv-not-saved')).toHaveCount(0);

        // The TV tab goes away: "Sem conexão desde".
        await closeTvPanel(master);
        await tv.close();
        panel = await openTvPanel(master);
        await expect(panel.getByTestId('tv-lost')).toBeVisible();
        await expectPanelFits(panel, 'sem conexão');
      } finally {
        await endOpenSessionRPC(master, campaignId);
        await masterContext.close();
        await playerContext.close();
      }
    },
  );
}

for (const size of [
  // `down` is how far the page may scroll down: nowhere on a TV, as far as it likes on a phone.
  { width: 1280, height: 800, down: 0 },
  { width: 1280, height: 720, down: 0 },
  { width: 1600, height: 900, down: 0 },
  { width: 390, height: 844, down: Number.POSITIVE_INFINITY },
]) {
  test(
    `a TV cabe numa tela em ${size.width}x${size.height}: espera, mapa com rolagem e combate no mapa (no celular só sem rolagem lateral)`,
    { tag: '@RN-10' },
    async ({ browser }) => {
      const masterContext = await newSignedInContext(browser, 'Mestre Teste', { viewport: size });
      const pensantusContext = await newSignedInContext(browser, 'Jogador Teste');
      const torenContext = await newSignedInContext(browser, 'E-mail Não Verificado');
      let campaignId = '';
      const master = await masterContext.newPage();
      try {
        const pensantusPage = await pensantusContext.newPage();
        const torenPage = await torenContext.newPage();
        await Promise.all([master.goto('/'), pensantusPage.goto('/'), torenPage.goto('/')]);
        const table = await tableForFog(master, pensantusPage, torenPage, `TV cabe ${Date.now()}`);
        campaignId = table.campaignId;
        const fog = await callRPC(master, 'meurpg.maps.v1.MapService/SetMapFog', { campaignId, mapId: table.mapId, groupVision: true });
        expect(fog.ok(), await fog.text()).toBeTruthy();
        const tv = await masterContext.newPage();
        await tv.goto(tvRoute(campaignId));
        const tiles = tv.locator('app-fog-map img[src$="as=tv"]');
        const fits = async (screen: string): Promise<void> => {
          const over = await pageOverflow(tv);
          expect(over.width, `the TV scrolls sideways on ${screen} at ${size.width}x${size.height}`).toBeLessThanOrEqual(0);
          expect(over.height, `the TV scrolls down on ${screen} at ${size.width}x${size.height}`).toBeLessThanOrEqual(size.down);
        };

        // The waiting screen, pinned through the same call the panel makes.
        const read = await callRPC(master, 'meurpg.play.v1.TableScreenService/GetTableScreen', { campaignId });
        const revision = (await read.json()).choiceRevision ?? 0;
        const pinned = await callRPC(master, 'meurpg.play.v1.TableScreenService/SetTableScreen', {
          campaignId,
          choiceRevision: revision,
          choice: { mirror: false, pin: 'TABLE_SCREEN_KIND_IDLE' },
        });
        expect(pinned.ok(), await pinned.text()).toBeTruthy();
        await expect(tv.getByRole('heading', { level: 2, name: 'Nada à mostra ainda' })).toBeVisible();
        await fits('a espera');

        // The map with the rolls beside it.
        const mirrored = await callRPC(master, 'meurpg.play.v1.TableScreenService/SetTableScreen', {
          campaignId,
          choiceRevision: revision + 1,
          choice: { mirror: true, pin: 'TABLE_SCREEN_KIND_UNSPECIFIED' },
        });
        expect(mirrored.ok(), await mirrored.text()).toBeTruthy();
        await rollForTableRPC(master, campaignId);
        await rollForTableRPC(master, campaignId);
        await expect(tiles.first()).toBeVisible();
        await expect(tv.getByTestId('tv-roll-last')).toBeVisible();
        await fits('o mapa com a rolagem');

        // The combat on the map.
        await beginFogCombat(master, table);
        await expect(tv.getByRole('heading', { level: 2, name: 'Ordem' })).toBeVisible();
        await expect(tiles.first()).toBeVisible();
        await fits('o combate no mapa');
      } finally {
        await endOpenSessionRPC(master, campaignId);
        await masterContext.close();
        await pensantusContext.close();
        await torenContext.close();
      }
    },
  );
}

for (const size of [
  { width: 390, height: 844 },
  { width: 1280, height: 720 },
]) {
  test(`o painel da Tela da mesa cabe numa tela com o combate fixo, em ${size.width}x${size.height}`, { tag: '@RN-10' }, async ({ browser }) => {
    const masterContext = await newSignedInContext(browser, 'Mestre Teste', { viewport: size });
    const pensantusContext = await newSignedInContext(browser, 'Jogador Teste');
    const torenContext = await newSignedInContext(browser, 'E-mail Não Verificado');
    let campaignId = '';
    const master = await masterContext.newPage();
    try {
      const pensantusPage = await pensantusContext.newPage();
      const torenPage = await torenContext.newPage();
      await Promise.all([master.goto('/'), pensantusPage.goto('/'), torenPage.goto('/')]);
      const table = await tableForFog(master, pensantusPage, torenPage, `Painel combate ${Date.now()}`);
      campaignId = table.campaignId;
      await beginFogCombat(master, table);
      await openSessionPage(master, campaignId);
      const tv = await masterContext.newPage();
      await tv.goto(tvRoute(campaignId));
      await expect(tv.getByRole('heading', { level: 2, name: 'Ordem' })).toBeVisible();
      const panel = await openTvPanel(master);
      await expect(panel.getByTestId('tv-state')).toContainText('Aberta');
      await chooseRow(panel, 'tv-pin-combat');
      await expect(panel.getByTestId('tv-now')).toContainText('O combate, fixo: a ordem.');
      await expectPanelFits(panel, 'com o combate fixo');
    } finally {
      await endOpenSessionRPC(master, campaignId);
      await masterContext.close();
      await pensantusContext.close();
      await torenContext.close();
    }
  });
}

test('a TV em 1280x720 com a barra de atraso: a ordem só esconde as linhas que não cabem, e o "+N" conta só essas', { tag: '@RN-10' }, async ({ browser }) => {
  const masterContext = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 720 } });
  const pensantusContext = await newSignedInContext(browser, 'Jogador Teste');
  const torenContext = await newSignedInContext(browser, 'E-mail Não Verificado');
  let campaignId = '';
  const master = await masterContext.newPage();
  try {
    const pensantusPage = await pensantusContext.newPage();
    const torenPage = await torenContext.newPage();
    await Promise.all([master.goto('/'), pensantusPage.goto('/'), torenPage.goto('/')]);
    const table = await tableForFog(master, pensantusPage, torenPage, `TV ordem ${Date.now()}`, { noFog: true });
    campaignId = table.campaignId;
    await beginCrowdedCombat(master, table, 6);
    const tv = await masterContext.newPage();
    await tv.goto(tvRoute(campaignId));
    await expect(tv.getByRole('heading', { level: 2, name: 'Ordem' })).toBeVisible();

    // The late bar takes room from the stage: a read that fails, then a change that asks for one.
    await tv.route('**/*TableScreenService/GetTableScreen', (route) => route.abort());
    await rollForTableRPC(master, campaignId);
    await expect(tv.getByTestId('tv-late')).toBeVisible();

    const fit = await inspectOrder(tv);
    expect(fit.drawn, 'the order drew fewer than 2 rows').toBeGreaterThanOrEqual(2);
    expect(fit.lastInsideCard, 'the last drawn row goes past the bottom of the Ordem card').toBe(true);
    expect(fit.more, 'the table has 10 combatants and the room is short: the "+N" is expected').toMatch(/^\+\d+$/);
    expect(fit.hiddenWhileEmpty, 'rows are hidden while "+N" says nothing is left out').toBe(0);
    expect(fit.hiddenOneDoesNotFit, 'the first hidden row would have fit: the "+N" counts a row that could be drawn').toBe(true);
  } finally {
    await endOpenSessionRPC(master, campaignId);
    await masterContext.close();
    await pensantusContext.close();
    await torenContext.close();
  }
});

test('uma cena com dois NPCs em cena na TV: "Em cena" mostra os dois e "Fala agora" só no que fala; sem NPC o cartão do texto ocupa a largura toda', { tag: '@RN-10' }, async ({ browser }) => {
  const masterContext = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 720 } });
  const playerContext = await newSignedInContext(browser, 'Jogador Teste');
  let campaignId = '';
  const master = await masterContext.newPage();
  try {
    const player = await playerContext.newPage();
    await Promise.all([master.goto('/'), player.goto('/')]);
    const table = await tableForTv(master, player, `TV cena ${Date.now()}`);
    campaignId = table.campaignId;
    const content = await addTvContent(master, campaignId);
    await putMapOnTable(master, campaignId, content.mapId);
    const tv = await masterContext.newPage();
    await tv.goto(tvRoute(campaignId));
    await expect(tvTitle(tv)).toHaveText('Estrada do Vale');

    // No one on the stage: the text card has the whole width.
    await putSceneOnTable(master, campaignId, content.sceneId);
    await expect(tvTitle(tv)).toHaveText('A carroça tombada');
    await expect(tv.getByRole('heading', { level: 2, name: 'Em cena' })).toHaveCount(0);
    const alone = await sceneBoxes(tv);
    expect(alone.text, 'the text card is narrower than the scene with nobody on the stage').toBeGreaterThanOrEqual(alone.whole - 1);

    // Two on the stage, Mira speaking.
    await twoOnStageRPC(master, campaignId);
    const npcs = tv.locator('.tv-scene__npc');
    await expect(npcs).toHaveCount(2);
    await expect(npcs.nth(0)).toContainText('Mira');
    await expect(npcs.nth(1)).toContainText('Borin');
    await expect(tv.locator('.tv-scene__npc--speaking')).toHaveCount(1);
    await expect(tv.locator('.tv-scene__npc--speaking')).toContainText('Mira');
    await expect(tv.locator('.tv-scene__speaking')).toHaveCount(1);
    await expect(tv.locator('.tv-scene__speaking')).toHaveText(/Fala agora/);
    await expect(npcs.filter({ hasText: 'Borin' }).locator('.tv-scene__speaking')).toHaveCount(0);
    await expect(npcs.filter({ hasText: 'Mira' }).locator('.tv-scene__speaking')).toHaveCount(1);
    // The ring of the one speaking is whole: the list that clips has room for it on every side.
    const room = await ringRoom(tv);
    expect(Math.min(room.left, room.top, room.right, room.bottom), `the speaking ring is cut by the list: ${JSON.stringify(room)}`).toBeGreaterThanOrEqual(0);
    const shared = await sceneBoxes(tv);
    expect(shared.text, 'the text card still has the whole width with two NPCs beside it').toBeLessThan(shared.whole - 100);
  } finally {
    await endOpenSessionRPC(master, campaignId);
    await masterContext.close();
    await playerContext.close();
  }
});
