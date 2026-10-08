import { expect, test, type Page } from '@playwright/test';

import { endOpenSessionRPC, openSessionPage, startSessionRPC } from './live-session-support';
import {
  canvasPng,
  createMapRPC,
  createPointRPC,
  placeTokenRPC,
  revealMapRPC,
  tableForMaps,
  uploadImageRPC,
} from './maps-support';
import { boxOf, callRPC, newSignedInContext } from './support';

// MR-008 (maps and points of interest), MR-009 (what the players see) and
// MR-012 (the session's map), through the screens. Setup (campaigns, images,
// maps, points, tokens) goes through the API; every test makes its own
// campaign. Signing in is never the point, so every context reuses a saved
// state.

/** Clicks the map at a fraction of its box (an empty spot of the image). */
async function clickMap(page: Page, x: number, y: number): Promise<void> {
  const map = page.getByRole('group', { name: /^Mapa / });
  // The editor's bar and header are above the map: bring it on screen first, or the click lands below the window.
  await map.evaluate((el) => el.scrollIntoView({ block: 'center' }));
  const box = await boxOf(map);
  await page.mouse.click(box.x + box.width * x, box.y + box.height * y);
}

test(
  'o mestre cria um mapa de uma imagem da galeria, põe uma batalha, um submapa e uma cena, e o jogador abre o submapa pela ficha do ponto',
  { tag: '@MR-008' },
  async ({ browser }) => {
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const master = await masterContext.newPage();
      const player = await playerContext.newPage();
      await master.goto('/');
      const table = await tableForMaps(master, player, `Mapas ${Date.now()}`);
      await uploadImageRPC(master, table.campaignId, 'Mapa de Mirathel', await canvasPng(master, 1200, 800, 'Mirathel'));
      const towerImage = await uploadImageRPC(master, table.campaignId, 'Planta da torre', await canvasPng(master, 800, 800, 'Torre', '#5b4834'));
      const towerId = await createMapRPC(master, table.campaignId, 'Torre de Mirathel', towerImage);
      await revealMapRPC(master, table.campaignId, towerId);

      // The new map form: a name and a tile of the gallery.
      await master.goto(`/campaigns/${table.campaignId}`);
      await master.getByRole('link', { name: 'Novo mapa' }).click();
      await expect(master.getByRole('heading', { name: 'Novo mapa', level: 1 })).toBeVisible();
      await master.getByRole('button', { name: 'Criar mapa' }).click();
      await expect(master.getByText('Dê um nome ao mapa.')).toBeVisible();
      await expect(master.getByText('Escolha uma imagem para o mapa.')).toBeVisible();
      await master.getByLabel('Nome').fill('Mirathel e arredores');
      await master.getByRole('radio', { name: /Mapa de Mirathel/ }).click();
      await master.getByRole('button', { name: 'Criar mapa' }).click();
      await expect(master).toHaveURL(/\/maps\/[^/]+$/);
      await expect(master.getByRole('heading', { name: 'Mirathel e arredores', level: 1 })).toBeVisible();
      await expect(master.getByText('Escondido dos jogadores')).toBeVisible();
      const mapUrl = master.url();

      // A battle: pick the kind, click the map, name it.
      await master.getByRole('button', { name: 'Batalha' }).click();
      await clickMap(master, 0.3, 0.6);
      await expect(master.getByRole('button', { name: 'Nova batalha, Batalha, escondido' })).toBeVisible();
      await expect(master.getByLabel('Nome', { exact: true })).toBeFocused();
      await master.getByLabel('Nome', { exact: true }).fill('Emboscada na estrada');
      await master.getByRole('button', { name: 'Salvar ponto' }).click();
      await expect(master.getByRole('button', { name: 'Emboscada na estrada, Batalha, escondido' })).toBeVisible();

      // A submap that leads to the tower, revealed.
      await master.getByRole('button', { name: 'Submapa', exact: true }).first().click();
      await clickMap(master, 0.7, 0.3);
      await expect(master.getByRole('button', { name: 'Novo submapa, Submapa, escondido' })).toBeVisible();
      await master.getByLabel('Nome', { exact: true }).fill('Torre de Mirathel');
      await master.getByLabel('Leva para').selectOption({ label: 'Torre de Mirathel' });
      await master.getByLabel('Descrição para os jogadores').fill('Uma torre antiga na colina.');
      await master.getByRole('switch', { name: 'Revelado aos jogadores' }).click();
      await master.getByRole('button', { name: 'Salvar ponto' }).click();
      await expect(master.getByRole('button', { name: 'Torre de Mirathel, Submapa', exact: true })).toBeVisible();

      // A scene.
      await master.getByRole('button', { name: 'Cena de RP', exact: true }).first().click();
      await clickMap(master, 0.5, 0.45);
      await expect(master.getByRole('button', { name: 'Nova cena, Cena de RP, escondido' })).toBeVisible();
      await master.getByLabel('Nome', { exact: true }).fill('Taverna do Javali');
      await master.getByRole('button', { name: 'Salvar ponto' }).click();
      await expect(master.getByRole('button', { name: 'Taverna do Javali, Cena de RP, escondido' })).toBeVisible();

      // The points are on the server: they survive a reload.
      await master.reload();
      await expect(master.getByRole('button', { name: 'Emboscada na estrada, Batalha, escondido' })).toBeVisible();
      await expect(master.getByRole('button', { name: 'Taverna do Javali, Cena de RP, escondido' })).toBeVisible();

      // Reveal the map; the player opens the submap point's sheet, then the target.
      await master.getByRole('button', { name: 'Revelar o mapa aos jogadores' }).click();
      await expect(master.getByRole('button', { name: 'Esconder o mapa dos jogadores' })).toBeVisible();

      await player.goto(mapUrl);
      await expect(player.getByRole('heading', { name: 'Mirathel e arredores', level: 1 })).toBeVisible();
      await player.getByRole('button', { name: 'Torre de Mirathel, Submapa' }).first().click();
      const sheet = player.getByRole('region', { name: 'Ponto Torre de Mirathel' });
      await expect(sheet.getByRole('heading', { name: 'Torre de Mirathel' })).toBeFocused();
      await expect(sheet.getByText('Uma torre antiga na colina.')).toBeVisible();
      // The sheet comes first: still on the same map.
      await expect(player).toHaveURL(mapUrl);
      await sheet.getByRole('button', { name: 'Abrir Torre de Mirathel' }).click();
      await expect(player.getByRole('heading', { name: 'Torre de Mirathel', level: 1 })).toBeVisible();
      await expect(player.getByRole('navigation', { name: 'Caminho do mapa' }).getByRole('link', { name: 'Mirathel e arredores' })).toBeVisible();
    } finally {
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'o jogador só vê o ponto revelado: a resposta de GetMap não traz o ponto escondido, nem o id, nem o nome',
  { tag: '@MR-009' },
  async ({ browser }) => {
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const master = await masterContext.newPage();
      const player = await playerContext.newPage();
      await master.goto('/');
      const table = await tableForMaps(master, player, `Mapas revelados ${Date.now()}`);
      const image = await uploadImageRPC(master, table.campaignId, 'Mapa', await canvasPng(master, 1200, 800, 'Mapa'));
      const mapId = await createMapRPC(master, table.campaignId, 'Vale dos Ecos', image);
      await revealMapRPC(master, table.campaignId, mapId);
      await createPointRPC(master, table.campaignId, mapId, { kind: 'SCENE', name: 'Ponte velha', description: 'Ranger.', xBp: 3000, yBp: 4000, revealed: true });
      const hiddenId = await createPointRPC(master, table.campaignId, mapId, { kind: 'BATTLE', name: 'Covil secreto', description: 'Só o mestre sabe.', xBp: 7000, yBp: 6000 });

      // The page's own GetMap response, read while the page is still on it.
      const [response] = await Promise.all([
        player.waitForResponse((res) => res.url().includes('meurpg.maps.v1.MapService/GetMap')),
        player.goto(`/campaigns/${table.campaignId}/maps/${mapId}`),
      ]);
      const body = await response.text();
      expect(body).toContain('Ponte velha');
      expect(body).not.toContain(hiddenId);
      expect(body).not.toContain('Covil secreto');
      expect(body).not.toContain('Só o mestre sabe');

      await expect(player.getByRole('button', { name: 'Ponte velha, Cena de RP' })).toBeVisible();
      await expect(player.getByRole('button', { name: /Covil secreto/ })).toHaveCount(0);
      await expect(player.getByText('Covil secreto')).toHaveCount(0);
      // "Pontos deste mapa" lists the same one point.
      await expect(player.getByRole('region', { name: 'Pontos deste mapa' }).getByRole('button')).toHaveCount(1);

      // The master sees both, the hidden one marked.
      await master.goto(`/campaigns/${table.campaignId}/maps/${mapId}`);
      await expect(master.getByRole('button', { name: 'Covil secreto, Batalha, escondido' })).toBeVisible();

      // A hidden map is "not found" for the player, like one that does not exist.
      const hiddenMap = await createMapRPC(master, table.campaignId, 'Mapa escondido', image);
      await player.goto(`/campaigns/${table.campaignId}/maps/${hiddenMap}`);
      await expect(player.getByRole('heading', { name: 'Mapa não encontrado' })).toBeVisible();
      const direct = await callRPC(player, 'meurpg.maps.v1.MapService/GetMap', { campaignId: table.campaignId, mapId: hiddenMap });
      expect(direct.status()).toBe(404);
    } finally {
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'o mestre escolhe o mapa da sessão e move um token: a página aberta do jogador mostra o mapa e a nova posição sem recarregar',
  { tag: '@MR-012' },
  async ({ browser }) => {
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    let campaignId = '';
    const master = await masterContext.newPage();
    try {
      const player = await playerContext.newPage();
      await master.goto('/');
      const table = await tableForMaps(master, player, `Mapa da sessão ${Date.now()}`);
      campaignId = table.campaignId;
      const image = await uploadImageRPC(master, campaignId, 'Mapa', await canvasPng(master, 1200, 800, 'Mapa'));
      const mapId = await createMapRPC(master, campaignId, 'Mirathel e arredores', image);
      await createPointRPC(master, campaignId, mapId, { kind: 'SCENE', name: 'Taverna do Javali', xBp: 5500, yBp: 4800, revealed: true });
      await placeTokenRPC(master, campaignId, mapId, table.characterId, 5200, 5400);
      await startSessionRPC(master, campaignId);

      await openSessionPage(player, campaignId);
      await expect(player.getByText('O mestre ainda não escolheu um mapa.')).toBeVisible();
      await openSessionPage(master, campaignId);

      // The master chooses the current map (which also reveals it).
      await master.getByLabel('Mapa atual').selectOption({ label: 'Mirathel e arredores (escondido)' });
      await expect(player.getByRole('heading', { name: 'Mirathel e arredores', level: 2 })).toBeVisible();
      const token = player.locator('app-map-view app-map-token', { has: player.locator('.tk__disc', { hasText: 'P' }) });
      await expect(token).toHaveAttribute('style', /left: 52%/);

      // The master moves the visible token (arrow keys, Shift: 5 %).
      const masterToken = master.getByRole('button', { name: 'Pensantus, visível' });
      await masterToken.focus();
      await master.keyboard.press('Shift+ArrowRight');
      await master.keyboard.press('Shift+ArrowRight');
      await expect(token).toHaveAttribute('style', /left: 62%/);
      await expect(token).toHaveAttribute('style', /top: 54%/);

      // Nothing was reloaded, and the move is on the server.
      const read = await callRPC(master, 'meurpg.maps.v1.MapService/GetMap', { campaignId, mapId });
      const moved = (await read.json()).tokens.find((t: { characterId: string }) => t.characterId === table.characterId);
      expect(moved.xBp).toBe(6200);
    } finally {
      await endOpenSessionRPC(master, campaignId);
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'o mestre renomeia o mapa pelo cabeçalho e o apaga depois de confirmar ali mesmo',
  { tag: '@MR-008' },
  async ({ browser }) => {
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const master = await masterContext.newPage();
      const player = await playerContext.newPage();
      await master.goto('/');
      const table = await tableForMaps(master, player, `Renomear mapa ${Date.now()}`);
      const image = await uploadImageRPC(master, table.campaignId, 'Mapa de Mirathel', await canvasPng(master, 1200, 800, 'Mirathel'));
      const mapId = await createMapRPC(master, table.campaignId, 'Mirathel e arredores', image);
      await createPointRPC(master, table.campaignId, mapId, { kind: 'BATTLE', name: 'Emboscada na estrada', xBp: 4000, yBp: 6000 });

      await master.goto(`/campaigns/${table.campaignId}/maps/${mapId}`);
      await expect(master.getByRole('heading', { name: 'Mirathel e arredores', level: 1 })).toBeVisible();

      // Renomear: the title becomes the field; Esc gives it back unchanged.
      await master.getByRole('button', { name: 'Renomear' }).click();
      const field = master.getByLabel('Nome do mapa');
      await expect(field).toBeFocused();
      await expect(field).toHaveValue('Mirathel e arredores');
      await field.press('Escape');
      await expect(master.getByRole('heading', { name: 'Mirathel e arredores', level: 1 })).toBeVisible();
      await expect(master.getByRole('button', { name: 'Renomear' })).toBeFocused();

      await master.getByRole('button', { name: 'Renomear' }).click();
      await master.getByLabel('Nome do mapa').fill('Arredores de Mirathel');
      await master.getByRole('button', { name: 'Salvar nome' }).click();
      await expect(master.getByRole('heading', { name: 'Arredores de Mirathel', level: 1 })).toBeVisible();
      await master.reload();
      await expect(master.getByRole('heading', { name: 'Arredores de Mirathel', level: 1 })).toBeVisible();

      // Apagar mapa: asks in place, focus on "Cancelar"; "Cancelar" keeps it.
      await master.getByRole('button', { name: 'Apagar mapa' }).click();
      const question = master.getByRole('group', { name: /Apagar Arredores de Mirathel\?/ });
      await expect(question).toContainText('O ponto dele vai junto; a imagem continua na galeria. Não dá para desfazer.');
      await expect(question.getByRole('button', { name: 'Cancelar' })).toBeFocused();
      await question.getByRole('button', { name: 'Cancelar' }).click();
      await expect(question).toBeHidden();

      await master.getByRole('button', { name: 'Apagar mapa' }).click();
      await master.getByRole('group', { name: /Apagar Arredores de Mirathel\?/ }).getByRole('button', { name: 'Apagar mapa' }).click();
      await expect(master).toHaveURL(new RegExp(`/campaigns/${table.campaignId}$`));
      const res = await callRPC(master, 'meurpg.maps.v1.MapService/GetMap', { campaignId: table.campaignId, mapId });
      expect(res.status()).toBe(404);
    } finally {
      await masterContext.close();
      await playerContext.close();
    }
  },
);
