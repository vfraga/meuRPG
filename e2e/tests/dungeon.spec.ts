import { expect, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { bitAt, createDungeonRPC, dungeonRoute, layersOfDungeon, previewRPC, roomsRPC } from './dungeon-support';
import { editorRoute, getMapRPC } from './editor-support';
import { canvasPng, revealMapRPC, tableForMaps, uploadImageRPC } from './maps-support';
import { expectLoaded } from './loaded';
import {
  callRPC,
  gotoCampaignTab,
  newSignedInContext,
} from './support';

// "Gerar masmorra", the rooms list and "Redesenhar" (Etapa 10, slice 10.14b: MR-010, RN-26, RN-10; E10-05 1 to 6). The campaign
// comes through the API; what is under test is the master's page (the options, the server's preview, "Criar o mapa"), the rooms list
// and "Pôr uma cena nesta sala", "Redesenhar", and what a player never gets. The tests read what the server kept and the words on the
// screen, never the pixels.

test.describe.configure({ timeout: 240_000 });

const SEED = '48213';

/** The preview's drawing and its words, as the page shows them. */
const drawing = (page: Page) => page.getByRole('img', { name: /^Prévia da masmorra/ });

/** Types a seed and waits until the server's preview for it is on the screen. */
async function seedField(page: Page, value: string): Promise<void> {
  const answered = page.waitForResponse((r) => r.url().includes('DungeonService/PreviewDungeon') && (r.request().postData() ?? '').includes(`"${value}"`));
  await page.getByRole('textbox', { name: 'Semente' }).fill(value);
  await answered;
  await expect(page.locator('.pv__draw[aria-busy="false"]')).toBeVisible();
}

/** The shape of the walls in the preview: the same options and seed must give the same string (the server draws it). */
async function wallShape(page: Page): Promise<string> {
  return (await page.locator('app-dungeon-preview .dp__veil').getAttribute('d')) ?? '';
}

test('o mestre vê a prévia do servidor e a mesma semente dá a mesma prévia, e cria o mapa com as camadas e as portas @MR-010 @RN-26', async ({ browser }) => {
  const masterContext = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 1000 } });
  const playerContext = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 1000 } });
  try {
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    await Promise.all([master.goto('/'), player.goto('/')]);
    const table = await tableForMaps(master, player, `Masmorra ${Date.now()}`);

    // The campaign's maps lead to the page.
    await gotoCampaignTab(master, table.campaignId, 'Preparação');
    await master.getByRole('link', { name: 'Gerar masmorra' }).click();
    await expect(master.getByRole('heading', { level: 1, name: 'Gerar masmorra' })).toBeVisible();
    await expect(drawing(master)).toBeVisible();
    await expect(master.getByText('luz de base “Penumbra”')).toBeVisible();

    // The same seed, the same options: the same dungeon (the server's, never the browser's).
    await seedField(master, SEED);
    await expect(master.getByRole('textbox', { name: 'Semente' })).toHaveValue(SEED);
    await expect(drawing(master)).toHaveAttribute('aria-label', /31 × 21 quadrados/);
    const first = await wallShape(master);
    const firstWords = await drawing(master).getAttribute('aria-label');
    const another = master.waitForResponse((r) => r.url().includes('DungeonService/PreviewDungeon'));
    await master.getByRole('button', { name: 'Outra semente' }).click();
    await another;
    await expect(master.getByRole('textbox', { name: 'Semente' })).not.toHaveValue(SEED);
    await expect.poll(() => wallShape(master)).not.toBe(first);
    await seedField(master, SEED);
    await expect.poll(() => wallShape(master)).toBe(first);
    expect(await drawing(master).getAttribute('aria-label')).toBe(firstWords);
    // What the server says for the same request: the layout the page drew.
    const direct = await previewRPC(master, table.campaignId, { seed: SEED, options: { width: 31, height: 21, roomSideMin: 3, roomSideMax: 9, stairs: 2, deadendRemoval: 60 } });
    expect(direct.width).toBe(31);
    expect(direct.rooms.length).toBeGreaterThan(0);
    await expect(drawing(master)).toHaveAttribute('aria-label', new RegExp(`${direct.rooms.length} salas?`));

    // A refused option is said in place, and "Criar o mapa" says why it is off.
    await master.getByRole('radio', { name: 'Outro' }).click();
    await master.getByRole('textbox', { name: 'Quadrados no lado maior' }).fill('130');
    await expect(master.getByText('O tamanho vai de 21 a 121 quadrados.')).toBeVisible();
    await expect(master.getByText('Corrija o tamanho para ver a masmorra.')).toBeVisible();
    await expect(master.getByRole('button', { name: 'Criar o mapa' })).toBeDisabled();
    await master.getByRole('radio', { name: 'Pequena (31)' }).click();
    await expect(drawing(master)).toBeVisible();

    // Creating: the map opens in the editor, hidden, with the walls, the doors and the rooms list.
    await seedField(master, SEED);
    await expect(drawing(master)).toBeVisible();
    await master.getByRole('button', { name: 'Criar o mapa' }).click();
    await expect(master).toHaveURL(/\/maps\/[0-9a-f-]{36}$/);
    const mapId = master.url().split('/').pop()!;
    await expect(master.getByRole('heading', { level: 1, name: `Masmorra de ${table.campaignName}` })).toBeVisible();
    await expect(master.getByRole('heading', { name: 'Salas', exact: true })).toBeVisible();
    const legend = master.getByRole('list', { name: 'Legenda do mapa' }).first();
    await expect(legend.getByText('Parede')).toBeVisible();
    // The stairs are named for themselves (the point says so), never as "Submapa".
    const stairLegend = master.getByRole('list', { name: 'Legenda do mapa' }).filter({ hasText: 'Escada para cima' });
    await expect(stairLegend.getByText('Escada para baixo')).toBeVisible();
    await expect(stairLegend.getByText('Submapa')).toHaveCount(0);

    const layers = await layersOfDungeon(master, table.campaignId, mapId);
    expect(layers.columns).toBe(31);
    expect(layers.doors).toBeGreaterThan(0);
    const { map } = await getMapRPC(master, table.campaignId, mapId);
    expect(map.revealed ?? false).toBe(false);
    expect(map.fogEnabled).toBe(true);
    // The seed the page previewed is the dungeon's.
    const rooms = await roomsRPC(master, table.campaignId, mapId);
    expect(String(rooms.seed)).toBe(SEED);
    expect(rooms.imageIsGenerated).toBe(true);
  } finally {
    await Promise.all([masterContext.close(), playerContext.close()]);
  }
});

test('o mestre põe uma cena na sala 1, e uma sala se escolhe na lista @MR-010 @RN-26', async ({ browser }) => {
  const masterContext = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 1000 } });
  const playerContext = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 1000 } });
  try {
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    await Promise.all([master.goto('/'), player.goto('/')]);
    const table = await tableForMaps(master, player, `Cena na sala ${Date.now()}`);
    const mapId = await createDungeonRPC(master, table.campaignId, 'A masmorra das salas', { seed: SEED, options: { width: 31, height: 21 } });
    await master.goto(editorRoute(table.campaignId, mapId));
    const list = master.getByRole('region', { name: 'Salas' });
    await expect(list.getByRole('heading', { name: 'Salas', exact: true })).toBeVisible();
    await expect(list.getByText('Só você vê')).toBeVisible();

    // A room chosen in the list is outlined on the map.
    const room1 = list.getByRole('button', { name: /^Sala 1/ });
    await room1.click();
    await expect(room1).toHaveAttribute('aria-pressed', 'true');
    await expect(master.locator('app-editor-overlay svg.room rect')).toHaveCount(1);
    await room1.click();
    await expect(master.locator('app-editor-overlay svg.room rect')).toHaveCount(0);

    // "Pôr uma cena nesta sala" makes the hidden scene point "Sala 1" in the middle of the room.
    const rooms = await roomsRPC(master, table.campaignId, mapId);
    const first = rooms.rooms.find((r: { id: number }) => r.id === 1) ?? rooms.rooms[0];
    await list.getByRole('button', { name: 'Pôr uma cena nesta sala' }).first().click();
    await expect(list.getByRole('status')).toContainText(`Cena “Sala ${first.id}” posta no mapa`);
    const { points } = await getMapRPC(master, table.campaignId, mapId);
    const scene = points.find((p) => p.name === `Sala ${first.id}`);
    expect(scene?.kind).toBe('MAP_POINT_KIND_SCENE');
    expect(scene?.revealed ?? false).toBe(false);
    // At the middle of the room: the square of its centre, in basis points.
    const cols = rooms.width as number;
    const rows = rooms.height as number;
    expect(Math.abs((scene!.xBp as number) - ((first.centerCol + 0.5) / cols) * 10000)).toBeLessThanOrEqual(1);
    expect(Math.abs((scene!.yBp as number) - ((first.centerRow + 0.5) / rows) * 10000)).toBeLessThanOrEqual(1);
    // And its marker is on the map at once.
    await expect(master.locator(`[data-point="${scene!.id}"]`)).toBeVisible();
    await expect(list.getByText('1 cena nesta sala')).toBeVisible();
    // The stairs are submap points with an arrow, named in the legend.
    expect(points.filter((p) => String(p.name).startsWith('Escada')).length).toBeGreaterThan(0);
  } finally {
    await Promise.all([masterContext.close(), playerContext.close()]);
  }
});

test('o mestre pinta uma parede e redesenha: a imagem muda e as camadas ficam @MR-010 @RN-26', async ({ browser }) => {
  const masterContext = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 1000 } });
  const playerContext = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 1000 } });
  try {
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    await Promise.all([master.goto('/'), player.goto('/')]);
    const table = await tableForMaps(master, player, `Redesenhar ${Date.now()}`);
    const mapId = await createDungeonRPC(master, table.campaignId, 'A masmorra redesenhada', { seed: SEED, options: { width: 31, height: 21 } });
    const before = await getMapRPC(master, table.campaignId, mapId);
    const imageBefore = (before.map.image as { id: string }).id;
    const layersBefore = await layersOfDungeon(master, table.campaignId, mapId);

    // A wall in the middle of room 1's floor (it was floor).
    const rooms = await roomsRPC(master, table.campaignId, mapId);
    const room = rooms.rooms[0];
    const col = room.centerCol as number;
    const row = room.centerRow as number;
    expect(bitAt(layersBefore.wall, layersBefore.columns, col, row)).toBe(0);
    const painted = await callRPC(master, 'meurpg.maps.v1.MapService/PaintMapCells', { campaignId: table.campaignId, mapId, layer: 'MAP_LAYER_WALL', value: 1, squares: [{ col, row }] });
    expect(painted.ok(), await painted.text()).toBeTruthy();

    await master.goto(editorRoute(table.campaignId, mapId));
    const panel = master.getByRole('region', { name: 'Imagem' });
    await expect(panel.getByText('Gerada pelo app')).toBeVisible();
    await panel.getByRole('button', { name: 'Redesenhar' }).click();
    // Asked in place: nothing is drawn before the second click.
    await expect(master.getByRole('heading', { name: 'Redesenhar a imagem?' })).toBeVisible();
    expect((await getMapRPC(master, table.campaignId, mapId)).map.image).toMatchObject({ id: imageBefore });
    await master.getByRole('button', { name: 'Voltar' }).click();
    await expect(panel.getByRole('button', { name: 'Redesenhar' })).toBeFocused();
    await panel.getByRole('button', { name: 'Redesenhar' }).click();
    await master.getByRole('button', { name: 'Redesenhar', exact: true }).last().click();
    await expect(master.getByText('A imagem foi desenhada de novo.')).toBeVisible();

    const after = await getMapRPC(master, table.campaignId, mapId);
    expect((after.map.image as { id: string }).id).not.toBe(imageBefore);
    // The layers stay: the painted wall, and every door.
    const layersAfter = await layersOfDungeon(master, table.campaignId, mapId);
    expect(bitAt(layersAfter.wall, layersAfter.columns, col, row)).toBe(1);
    expect(layersAfter.doors).toBe(layersBefore.doors);

    // A map whose image the master replaced is no longer the dungeon's: the panel says so and offers no "Redesenhar".
    const other = await uploadImageRPC(master, table.campaignId, 'Outra imagem', await canvasPng(master, 1200, 800, 'Outra imagem'));
    const replaced = await callRPC(master, 'meurpg.maps.v1.MapService/UpdateMap', { campaignId: table.campaignId, mapId, revision: after.map.revision, imageId: other });
    expect(replaced.ok(), await replaced.text()).toBeTruthy();
    await master.goto(editorRoute(table.campaignId, mapId));
    await expect(panel.getByText('Imagem trocada')).toBeVisible();
    await expect(panel.getByRole('button', { name: 'Redesenhar' })).toHaveCount(0);
  } finally {
    await Promise.all([masterContext.close(), playerContext.close()]);
  }
});

test('o jogador lê o mapa revelado e nunca recebe a lista das salas @MR-010 @RN-10', async ({ browser }) => {
  const masterContext = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 1000 } });
  const playerContext = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 1000 } });
  try {
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    await Promise.all([master.goto('/'), player.goto('/')]);
    const table = await tableForMaps(master, player, `Jogador ${Date.now()}`);
    const mapId = await createDungeonRPC(master, table.campaignId, 'A masmorra do jogador', { seed: SEED, options: { width: 31, height: 21 } });
    await revealMapRPC(master, table.campaignId, mapId);

    // Every call of the service is the master's: the player gets `not_found`, as from a map that does not exist.
    for (const [method, body] of [
      ['GetDungeonRooms', { campaignId: table.campaignId, mapId }],
      ['PlaceDungeonScene', { campaignId: table.campaignId, mapId, roomId: 1 }],
      ['RedrawDungeonMap', { campaignId: table.campaignId, mapId }],
      ['PreviewDungeon', { campaignId: table.campaignId }],
    ] as const) {
      const res = await callRPC(player, `meurpg.maps.v1.DungeonService/${method}`, body);
      expect(res.status(), method).toBe(404);
      expect((await res.json()).code, method).toBe('not_found');
    }

    // What the player reads of the map has no seed, no room and no true door kind.
    const read = await callRPC(player, 'meurpg.maps.v1.MapService/GetMap', { campaignId: table.campaignId, mapId });
    expect(read.ok(), await read.text()).toBeTruthy();
    const json = await read.text();
    expect(json).not.toContain(SEED);
    expect(json).not.toMatch(/scenePointIds|imageIsGenerated|centerCol/);

    // The player's page: the map, and none of the master's panels. The screen never even asks the dungeon service.
    const asked: string[] = [];
    player.on('request', (r) => {
      if (r.url().includes('DungeonService/')) {
        asked.push(r.url());
      }
    });
    await player.goto(editorRoute(table.campaignId, mapId));
    await expect(player.getByRole('heading', { level: 1, name: 'A masmorra do jogador' })).toBeVisible();
    await expect(player.locator('app-player-map')).toBeVisible();
    await expectLoaded(player);
    expect(asked).toEqual([]);
    await expect(player.getByRole('heading', { name: 'Salas', exact: true })).toHaveCount(0);
    await expect(player.getByRole('button', { name: 'Redesenhar' })).toHaveCount(0);
    await expect(player.getByText('Pôr uma cena nesta sala')).toHaveCount(0);
  } finally {
    await Promise.all([masterContext.close(), playerContext.close()]);
  }
});
