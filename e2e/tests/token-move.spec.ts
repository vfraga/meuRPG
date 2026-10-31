import { expect, type Browser, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { getMapRPC } from './editor-support';
import { CAVE_COLUMNS, CAVE_ROWS, type FogTable, sessionRoute, squareBp, tableForFog } from './fog-support';
import { endOpenSessionRPC, showMapTool } from './live-session-support';
import { callRPC, newSignedInContext } from './support';

// MR-012 and RN-10, the master's side of moving tokens outside combat (B2), through the screens: "Mover Tor" with the pad and
// "Quadrados por clique", the stop before a square marked "Não dá para passar", "Pôr o grupo no mapa", and the switch "Os jogadores movem
// os próprios personagens". Setup goes through the API; every test makes its own campaign and the session is ended whatever happens.

test.describe.configure({ timeout: 180_000 });

interface Scene {
  table: FogTable;
  mp: Page;
  ap: Page;
  bp: Page;
}

/** The cave at the table with the fog off, Pensantus on the corridor's mouth (2, 7) and Toren away from it (3, 9). */
async function atCave(browser: Browser, name: string, body: (scene: Scene) => Promise<void>): Promise<void> {
  const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 1100 } });
  const pensantus = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 1000 } });
  const toren = await newSignedInContext(browser, 'E-mail Não Verificado', { viewport: { width: 1280, height: 1000 } });
  const [mp, ap, bp] = await Promise.all([master.newPage(), pensantus.newPage(), toren.newPage()]);
  let campaignId = '';
  try {
    await Promise.all([mp.goto('/'), ap.goto('/'), bp.goto('/')]);
    const table = await tableForFog(mp, ap, bp, `${name} ${Date.now()}`, { noFog: true, pensantusAt: { col: 2, row: 7 }, torenAt: { col: 3, row: 9 } });
    campaignId = table.campaignId;
    await body({ table, mp, ap, bp });
  } finally {
    await endOpenSessionRPC(mp, campaignId);
    await Promise.all([master.close(), pensantus.close(), toren.close()]);
  }
}

/** The master's "Tokens" tab under the map. */
async function openTokens(mp: Page, table: FogTable): Promise<void> {
  await mp.goto(sessionRoute(table.campaignId));
  await showMapTool(mp, 'Tokens');
  await expect(mp.getByRole('switch', { name: 'Os jogadores movem os próprios personagens' })).toBeVisible();
}

/** The square a character's token is on now, by the server. The server keeps the middle of the square in whole basis points
 * (it rounds 4687.5 down), so the test compares squares, not points. */
async function squareOf(page: Page, table: FogTable, characterId: string): Promise<{ col: number; row: number }> {
  const { tokens } = await getMapRPC(page, table.campaignId, table.mapId);
  const token = tokens.find((t) => t.characterId === characterId && !t.creatureId) as { xBp: number; yBp: number };
  return { col: Math.floor((token.xBp * CAVE_COLUMNS) / 10000), row: Math.floor((token.yBp * CAVE_ROWS) / 10000) };
}

test(
  'o mestre escolhe a Pensantus, manda andar para a direita e o movimento para antes do quadrado "Não dá para passar", com a razão e a distância',
  { tag: ['@MR-012', '@RN-10'] },
  async ({ browser }) => {
    await atCave(browser, 'Mover token', async ({ table, mp }) => {
      const marked = await callRPC(mp, 'meurpg.maps.v1.MapService/PaintMapCells', {
        campaignId: table.campaignId,
        mapId: table.mapId,
        layer: 'MAP_LAYER_NO_PASS',
        value: 1,
        squares: [{ col: 9, row: 7 }],
      });
      expect(marked.ok(), await marked.text()).toBeTruthy();
      await openTokens(mp, table);

      // Nothing is chosen: no panel. The box of a character opens "Mover Pensantus"; the button waits for a destination.
      await expect(mp.getByRole('heading', { name: /^Mover / })).toHaveCount(0);
      await mp.getByRole('checkbox', { name: 'Escolher Pensantus para mover' }).check();
      await expect(mp.getByRole('heading', { name: 'Mover Pensantus' })).toBeVisible();
      const go = mp.getByRole('button', { name: 'Mover Pensantus', exact: true });
      await expect(go).toHaveAttribute('aria-disabled', 'true');
      await expect(mp.getByText('Escolha o quadrado de destino.')).toBeVisible();

      // The pad and the steps set the destination; the summary says it in words and meters.
      await mp.getByRole('button', { name: 'Ir para a direita' }).click();
      for (let i = 0; i < 9; i++) {
        await mp.getByRole('button', { name: 'Mais um quadrado' }).click();
      }
      await expect(mp.getByText('Pensantus vai 10 quadrados para a direita')).toBeVisible();
      await expect(mp.getByText('Distância: 15 m')).toBeVisible();

      // The server walks it and says where it stopped: before the marked square, 6 squares (9 m) on.
      await go.click();
      await expect(mp.locator('.mr-notice').getByText('O movimento parou antes.', { exact: true })).toBeVisible();
      await expect(mp.locator('.mr-notice').getByText(/O próximo quadrado está marcado como “Não dá para passar”\. Pensantus andou 9\s+m\./)).toBeVisible();
      await expect(mp.getByText(/Pedido: 10\s+quadrados \(15\s+m\) · andou 6 \(9\s+m\)/)).toBeVisible();
      await expect(mp.getByText('9 m · parou antes')).toBeVisible();
      expect(await squareOf(mp, table, table.pensantusId)).toEqual({ col: 8, row: 7 });
    });
  },
);

test(
  'o mestre liga "Os jogadores movem os próprios personagens": o jogador move só o próprio token, e desligado o servidor recusa',
  { tag: ['@MR-012', '@RN-10'] },
  async ({ browser }) => {
    await atCave(browser, 'Jogadores movem', async ({ table, mp, ap }) => {
      const asPlayer = (characterId: string, col: number, row: number) =>
        callRPC(ap, 'meurpg.maps.v1.MapService/MoveMapTokens', { campaignId: table.campaignId, mapId: table.mapId, characterIds: [characterId], ...squareBp(col, row) });
      await openTokens(mp, table);
      const toggle = mp.getByRole('switch', { name: 'Os jogadores movem os próprios personagens' });
      await expect(toggle).toHaveAttribute('aria-checked', 'false');
      await expect(mp.getByText('Só o mestre move os tokens neste mapa.')).toBeVisible();
      expect((await asPlayer(table.pensantusId, 4, 7)).status()).toBe(403);

      await toggle.click();
      await expect(toggle).toHaveAttribute('aria-checked', 'true');
      await expect(mp.getByText('Cada jogador arrasta só o próprio token. O mestre continua movendo todos.')).toBeVisible();
      const own = await asPlayer(table.pensantusId, 4, 7);
      expect(own.ok(), await own.text()).toBeTruthy();
      expect(await squareOf(mp, table, table.pensantusId)).toEqual({ col: 4, row: 7 });
      // Not another character's token, and the master still moves everyone.
      expect((await asPlayer(table.torenId, 4, 9)).status()).toBe(403);

      await toggle.click();
      await expect(toggle).toHaveAttribute('aria-checked', 'false');
      expect((await asPlayer(table.pensantusId, 5, 7)).status()).toBe(403);
    });
  },
);

test(
  '"Pôr o grupo no mapa" abre a janela com quem vem, a formação e o quadrado do mapa, e o servidor põe os tokens em fila a partir dele',
  { tag: ['@MR-012'] },
  async ({ browser }) => {
    await atCave(browser, 'Pôr o grupo', async ({ table, mp }) => {
      await openTokens(mp, table);
      await mp.getByRole('button', { name: 'Pôr o grupo no mapa' }).click();
      const dialog = mp.getByRole('dialog', { name: 'Pôr o grupo no mapa' });
      await expect(dialog.getByText('Quem vem')).toBeVisible();
      await expect(dialog.getByText('2 de 2')).toBeVisible();
      await expect(dialog.getByRole('radio', { name: 'Em bloco' })).toHaveAttribute('aria-checked', 'true');
      await dialog.getByRole('radio', { name: 'Em fila' }).click();
      await expect(dialog.getByText('Lado a lado, à direita do primeiro.')).toBeVisible();

      // The corner of the line: a click on the picture, on the corridor's square (12, 7).
      const picture = dialog.locator('.pick');
      const box = (await picture.boundingBox())!;
      await picture.click({ position: { x: (12.5 / 24) * box.width, y: (7.5 / 16) * box.height } });
      await expect(dialog.getByText('Canto de cima à esquerda: coluna 13, linha 8.')).toBeAttached();
      await dialog.getByRole('button', { name: 'Pôr 2 no mapa' }).click();
      await expect(dialog).toBeHidden();
      await expect(mp.getByText('2 no mapa.')).toBeVisible();

      const first = await squareOf(mp, table, table.pensantusId);
      const second = await squareOf(mp, table, table.torenId);
      // The first of the list is the corner; the other is beside it, to the right.
      expect([first, second]).toEqual(expect.arrayContaining([{ col: 12, row: 7 }, { col: 13, row: 7 }]));
    });
  },
);

test(
  'o jogador vê "O mestre move os tokens agora." e nenhum controle; quando o mestre libera ele move a Pensantus, o movimento para antes do quadrado "Não dá para passar", e quando o mestre trava o painel fecha',
  { tag: ['@MR-012', '@RN-10'] },
  async ({ browser }) => {
    await atCave(browser, 'Jogador move', async ({ table, mp, ap }) => {
      const switchTo = async (enabled: boolean) => {
        const res = await callRPC(mp, 'meurpg.maps.v1.MapService/SetMapPlayersMove', { campaignId: table.campaignId, mapId: table.mapId, enabled });
        expect(res.ok(), await res.text()).toBeTruthy();
      };
      const marked = await callRPC(mp, 'meurpg.maps.v1.MapService/PaintMapCells', {
        campaignId: table.campaignId,
        mapId: table.mapId,
        layer: 'MAP_LAYER_NO_PASS',
        value: 1,
        squares: [{ col: 9, row: 7 }],
      });
      expect(marked.ok(), await marked.text()).toBeTruthy();
      await ap.goto(sessionRoute(table.campaignId));

      // The master's switch is off (the map's default): one line, no control.
      await expect(ap.getByText('O mestre move os tokens agora.')).toBeVisible();
      await expect(ap.getByRole('button', { name: 'Mover Pensantus' })).toHaveCount(0);

      // The master frees the moving while the player looks: the notice, then the line and the button.
      await switchTo(true);
      await expect(ap.getByText('O mestre liberou: agora você move Pensantus.').first()).toBeVisible();
      const open = ap.getByRole('button', { name: 'Mover Pensantus', exact: true });
      await expect(open).toBeVisible();
      await open.click();
      await expect(ap.getByRole('button', { name: 'Ir para a direita' })).toBeVisible();
      await expect(ap.getByText('Quadrados por clique')).toBeVisible();
      await expect(ap.getByText('Escolha a direção')).toBeVisible();

      // The plan, then the send: the server walks it and stops before the marked square.
      await ap.getByRole('button', { name: 'Ir para a direita' }).click();
      for (let i = 0; i < 9; i++) {
        await ap.getByRole('button', { name: 'Mais um quadrado' }).click();
      }
      await expect(ap.getByText('Pensantus vai 10 quadrados para a direita')).toBeVisible();
      await ap.getByRole('button', { name: 'Mover Pensantus', exact: true }).click();
      await expect(ap.locator('.mr-notice').getByText('O movimento parou antes.', { exact: true })).toBeVisible();
      await expect(ap.locator('.mr-notice').getByText(/O próximo quadrado está marcado como “Não dá para passar”\. Pensantus andou 6 \(9\s+m\)\./)).toBeVisible();
      expect(await squareOf(mp, table, table.pensantusId)).toEqual({ col: 8, row: 7 });

      // The master takes the switch back: the notice, and the panel goes away without moving anybody.
      await switchTo(false);
      await expect(ap.getByText('O mestre voltou a mover os tokens.').first()).toBeVisible();
      await expect(ap.getByRole('button', { name: 'Ir para a direita' })).toHaveCount(0);
      await expect(ap.getByRole('button', { name: 'Mover Pensantus' })).toHaveCount(0);
      expect(await squareOf(mp, table, table.pensantusId)).toEqual({ col: 8, row: 7 });
    });
  },
);
