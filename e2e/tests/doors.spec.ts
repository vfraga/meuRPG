import { expect, type Browser, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { getEncounterRPC, torenSheet, toren } from './combat-support';
import { clickSquare, editorRoute, mapToPaint } from './editor-support';
import { sessionRoute, tableForFog } from './fog-support';
import { endOpenSessionRPC, openSessionPage } from './live-session-support';
import { placeTokenRPC, tableForMaps } from './maps-support';
import { movingTable, tapSquare } from './move-support';
import { callRPC, newSignedInContext, openCombatLog } from './support';
import { openCombatPanel } from './combat-layout';

// The doors in the map editor and in the session (Etapa 10, slice 10.14a: MR-010, RN-26, RN-10; E10-05 7 to 11). The maps, the walls and
// the doors come through the API; what is under test is the screens: the "Porta" tool, a player walking into a closed and a locked door,
// the master's door sheet and the secret door. The tests read what the server kept and sent (`GetMapLayers`, the JSON a player gets) and
// the words on the screen, never the pixels.

test.describe.configure({ timeout: 180_000 });

const torenFirst = { Toren: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 };

/** A DoorState as the layer stores it. */
const CLOSED = 2;
const LOCKED = 3;
const SECRET = 5;
const OPEN = 1;

/** What `GetMapLayers` gives the caller, as the app's JSON: the doors layer (four bits a square) and the walls, read square by square. */
async function layers(page: Page, campaignId: string, mapId: string) {
  const res = await callRPC(page, 'meurpg.maps.v1.MapService/GetMapLayers', { campaignId, mapId });
  expect(res.ok(), await res.text()).toBeTruthy();
  const body = await res.json();
  const columns = (body.gridColumns ?? 0) as number;
  const doors = Buffer.from(body.doors ?? '', 'base64');
  const walls = Buffer.from(body.wall ?? '', 'base64');
  const door = (col: number, row: number) => {
    const n = row * columns + col;
    const byte = doors[n >> 1] ?? 0;
    return n & 1 ? byte >> 4 : byte & 15;
  };
  const wall = (col: number, row: number) => {
    const n = row * columns + col;
    return ((walls[n >> 3] ?? 0) >> (n & 7)) & 1;
  };
  /** Every door the response carries, as "col,row:state". */
  const all = () => {
    const rows = (body.gridRows ?? 0) as number;
    const out: string[] = [];
    for (let row = 0; row < rows; row++) {
      for (let col = 0; col < columns; col++) {
        if (door(col, row) !== 0) {
          out.push(`${col},${row}:${door(col, row)}`);
        }
      }
    }
    return out;
  };
  return { door, wall, all };
}

async function paintLayer(master: Page, table: { campaignId: string; mapId: string }, layer: 'MAP_LAYER_WALL' | 'MAP_LAYER_DOORS', value: number, squares: [number, number][]): Promise<void> {
  const res = await callRPC(master, 'meurpg.maps.v1.MapService/PaintMapCells', {
    campaignId: table.campaignId,
    mapId: table.mapId,
    layer,
    value,
    squares: squares.map(([col, row]) => ({ col, row })),
  });
  expect(res.ok(), await res.text()).toBeTruthy();
}

/** The legend of the map the page shows. */
const legend = (page: Page) => page.getByRole('list', { name: 'Legenda do mapa' });

test('o mestre põe portas com a ferramenta Porta, e o jogador só sabe o que pode saber @MR-010 @RN-26 @RN-10', async ({ browser }) => {
  const masterContext = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 1000 } });
  const playerContext = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 1000 } });
  try {
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    await Promise.all([master.goto('/'), player.goto('/')]);
    const table = await tableForMaps(master, player, `Portas ${Date.now()}`);
    // A 20 x 13 map: a wall down column 10 (rows 3 to 9), floor on both sides.
    const map = await mapToPaint(master, table.campaignId, 'O corredor das portas', 20);
    const target = { campaignId: table.campaignId, mapId: map.mapId };
    await paintLayer(master, target, 'MAP_LAYER_WALL', 1, [3, 4, 5, 6, 7, 8, 9].map((row) => [10, row] as [number, number]));
    // Pensantus stands on the wall square (10, 4): 10,5 of 20 columns and 4,5 of 13 rows, in the middle of that square.
    await placeTokenRPC(master, table.campaignId, map.mapId, table.characterId, Math.round((10.5 / 20) * 10000), Math.round((4.5 / 13) * 10000));

    await master.goto(editorRoute(table.campaignId, map.mapId));
    await master.getByRole('radio', { name: 'Pintar' }).click();
    const tools = master.getByRole('group', { name: 'Ferramenta de pintura' });
    await tools.getByRole('button', { name: 'Porta' }).click();
    await expect(master.getByRole('group', { name: 'Tipo de porta' })).toBeVisible();
    await expect(master.getByText('Toque num quadrado para pôr a porta do tipo escolhido.')).toBeVisible();
    const kinds = master.getByRole('group', { name: 'Tipo de porta' });

    // A closed door where the wall has floor on both sides: the gap opens and the door goes in.
    await clickSquare(master, map, 10, 6);
    await expect(master.locator('app-layers-panel')).toContainText('1 porta');
    await expect.poll(async () => (await layers(master, table.campaignId, map.mapId)).door(10, 6)).toBe(CLOSED);
    // A locked door, and a secret one.
    await kinds.getByRole('button', { name: 'Trancada' }).click();
    await clickSquare(master, map, 10, 7);
    await kinds.getByRole('button', { name: 'Secreta' }).click();
    await clickSquare(master, map, 10, 8);
    await expect(master.locator('app-layers-panel')).toContainText('3 portas · 1 trancada · 1 secreta');
    await expect.poll(async () => {
      const now = await layers(master, table.campaignId, map.mapId);
      return [now.door(10, 7), now.door(10, 8)];
    }).toEqual([LOCKED, SECRET]);
    // The legend names each mark; the padlock and the secret door say "só você vê".
    await expect(legend(master).getByText('Porta fechada')).toBeVisible();
    await expect(legend(master).getByText('Porta trancada (só você vê)')).toBeVisible();
    await expect(legend(master).getByText('Porta secreta (só você vê)')).toBeVisible();

    const mine = await layers(master, table.campaignId, map.mapId);
    expect([mine.door(10, 6), mine.door(10, 7), mine.door(10, 8)]).toEqual([CLOSED, LOCKED, SECRET]);
    // The gap was opened: the wall under each door is gone.
    expect([mine.wall(10, 6), mine.wall(10, 7), mine.wall(10, 8)]).toEqual([0, 0, 0]);

    // What the player gets (RN-10, read as the app's JSON): the locked door reads closed, the secret one is no door but a wall.
    const theirs = await layers(player, table.campaignId, map.mapId);
    expect([theirs.door(10, 6), theirs.door(10, 7), theirs.door(10, 8)]).toEqual([CLOSED, CLOSED, 0]);
    expect(theirs.wall(10, 8)).toBe(1);
    expect(theirs.all().join(' ')).not.toMatch(/:3|:5/);

    // A tap where no door fits says why, in place, and paints nothing.
    await clickSquare(master, map, 3, 3);
    await expect(master.getByRole('alert').filter({ hasText: 'uma porta precisa de chão dos dois lados' })).toBeVisible();

    // Someone stands where the door would go: the panel asks first.
    await kinds.getByRole('button', { name: 'Fechada' }).click();
    await clickSquare(master, map, 10, 4);
    await expect(master.getByText('Pôr a porta onde há alguém?')).toBeVisible();
    await expect(master.getByText('Pensantus está nesse quadrado')).toBeVisible();
    expect((await layers(master, table.campaignId, map.mapId)).door(10, 4)).toBe(0);
    await master.getByRole('button', { name: 'Voltar' }).click();
    await expect(master.getByText('Pôr a porta onde há alguém?')).toHaveCount(0);
    await clickSquare(master, map, 10, 4);
    await master.getByRole('button', { name: 'Pôr a porta' }).click();
    await expect(master.locator('app-layers-panel')).toContainText('4 portas');
    await expect.poll(async () => (await layers(master, table.campaignId, map.mapId)).door(10, 4)).toBe(CLOSED);
    expect((await layers(master, table.campaignId, map.mapId)).door(10, 4)).toBe(CLOSED);

    // "Tirar a porta" closes the gap again as a wall.
    await master.getByRole('button', { name: 'Tirar a porta' }).click();
    await clickSquare(master, map, 10, 6);
    await expect(master.locator('app-layers-panel')).toContainText('3 portas');
    await expect.poll(async () => {
      const now = await layers(master, table.campaignId, map.mapId);
      return [now.door(10, 6), now.wall(10, 6)];
    }).toEqual([0, 1]);
    const after = await layers(master, table.campaignId, map.mapId);
    expect([after.door(10, 6), after.wall(10, 6)]).toEqual([0, 1]);
  } finally {
    await masterContext.close();
    await playerContext.close();
  }
});

test('Toren anda até uma porta fechada em combate e ela se abre; o registro diz @MR-010 @RN-26', async ({ browser }) => {
  test.setTimeout(240_000);
  const { m, p, table, campaignId, done } = await movingTable(browser, 'Porta fechada', torenFirst, {
    character: { build: toren, sheet: torenSheet },
    paint: async (master, t) => {
      // A wall down column 8 with a closed door at (8, 7), on Toren's row.
      await paintLayer(master, t, 'MAP_LAYER_WALL', 1, [4, 5, 6, 8, 9, 10].map((row) => [8, row] as [number, number]));
      await paintLayer(master, t, 'MAP_LAYER_DOORS', CLOSED, [[8, 7]]);
    },
  });
  try {
    expect((await layers(p, campaignId, table.mapId)).door(8, 7)).toBe(CLOSED);
    await openSessionPage(p, campaignId);
    await openSessionPage(m, campaignId);
    await expect(p.getByRole('heading', { name: 'Sua vez, Toren' })).toBeVisible();
    await p.getByRole('button', { name: 'Mover', exact: true }).click();
    await expect(p.getByRole('heading', { name: 'Mover Toren' })).toBeVisible();
    await expect(legend(p).getByText('Porta fechada')).toBeVisible();
    await expect(legend(p).getByText('trancada')).toHaveCount(0);

    // Five squares straight, through the door: 7,5 m of the 9,0 m.
    await tapSquare(p, 10, 7);
    await expect(p.getByText('Mover 7,5 m', { exact: true })).toBeVisible();
    await p.getByRole('button', { name: 'Mover para cá' }).click();
    await expect(p.getByRole('heading', { name: 'Sua vez, Toren' })).toBeVisible();
    await expect.poll(async () => (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Toren')?.col).toBe(10);

    // The door is open for everyone, and the log says who opened it, to the master and to the player.
    expect((await layers(p, campaignId, table.mapId)).door(8, 7)).toBe(OPEN);
    expect((await layers(m, campaignId, table.mapId)).door(8, 7)).toBe(OPEN);
    // A toast says it at once, so the player need not open the log to know the move opened the door.
    await expect(p.getByText('Você abriu a porta.')).toBeVisible();
    // The player's log is a fold under the map (a sheet on a phone): open it first.
    await expect(await openCombatLog(p)).toContainText('abriu a porta');
    await openCombatPanel(m, 'Registro');
    await expect(m.getByText('abriu a porta').first()).toBeVisible();
    // The map says so too: the legend of the "Mover" page now names an open door (the combat's own map card has no legend).
    await p.getByRole('button', { name: 'Mover', exact: true }).click();
    await expect(p.getByRole('heading', { name: 'Mover Toren' })).toBeVisible();
    await expect(legend(p).getByText('Porta aberta')).toBeVisible();
  } finally {
    await done();
  }
});

test('uma porta trancada para o movimento antes dela; o mapa do jogador segue com "Porta fechada"; o mestre revela a porta secreta @MR-010 @RN-26 @RN-10', async ({ browser }) => {
  test.setTimeout(300_000);
  const { m, p, table, campaignId, done } = await movingTable(browser, 'Porta trancada', torenFirst, {
    character: { build: toren, sheet: torenSheet },
    paint: async (master, t) => {
      // North of Toren (5, 7): a wall along row 5 with a locked door at (5, 5). East of him: a wall down column 8 with a secret door at (8, 9).
      await paintLayer(master, t, 'MAP_LAYER_WALL', 1, [[3, 5], [4, 5], [6, 5], [7, 5], [8, 6], [8, 7], [8, 8], [8, 10]]);
      await paintLayer(master, t, 'MAP_LAYER_DOORS', LOCKED, [[5, 5]]);
      await paintLayer(master, t, 'MAP_LAYER_DOORS', SECRET, [[8, 9]]);
    },
  });
  try {
    // What the player is told before anything happens: the locked door reads closed, the secret door is a wall and no door.
    const before = await layers(p, campaignId, table.mapId);
    expect(before.door(5, 5)).toBe(CLOSED);
    expect(before.door(8, 9)).toBe(0);
    expect(before.wall(8, 9)).toBe(1);
    expect(before.all().join(' ')).not.toMatch(/:3|:5/);
    // The master has them as painted.
    const mine = await layers(m, campaignId, table.mapId);
    expect([mine.door(5, 5), mine.door(8, 9)]).toEqual([LOCKED, SECRET]);

    await openSessionPage(p, campaignId);
    await openSessionPage(m, campaignId);
    await expect(p.getByRole('heading', { name: 'Sua vez, Toren' })).toBeVisible();
    await p.getByRole('button', { name: 'Mover', exact: true }).click();
    await expect(p.getByRole('heading', { name: 'Mover Toren' })).toBeVisible();
    await expect(legend(p).getByText('Porta fechada')).toBeVisible();
    await expect(legend(p).getByText('secreta')).toHaveCount(0);

    // Straight north, through the locked door: the move stops before it, on the same page, and says why.
    await tapSquare(p, 5, 3);
    await p.getByRole('button', { name: 'Mover para cá' }).click();
    const notice = p.getByRole('status').filter({ hasText: 'A porta está trancada.' });
    await expect(notice).toBeVisible();
    await expect(notice).toContainText('Só o mestre a destranca.');
    await expect(p.getByRole('heading', { name: 'Mover Toren' })).toBeVisible();
    await expect.poll(async () => (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Toren')?.row).toBe(6);
    // The app does not remember the lock for the player: the door is still a closed one on their map, and nowhere says "trancada".
    expect((await layers(p, campaignId, table.mapId)).door(5, 5)).toBe(CLOSED);
    await expect(legend(p).getByText('Porta fechada')).toBeVisible();
    await expect(legend(p).getByText('trancada')).toHaveCount(0);
    await expect(m.getByRole('list', { name: 'Legenda do mapa' }).getByText('Porta trancada (só você vê)')).toBeVisible();
    await p.getByRole('button', { name: 'Cancelar' }).click();

    // The secret door: the master's map marks it, the player's has a wall there. The master reveals it from its sheet.
    const master = m.getByRole('list', { name: 'Legenda do mapa' });
    await expect(master.getByText('Porta secreta (só você vê)')).toBeVisible();
    await m.getByRole('button', { name: /^Porta secreta, coluna 9, linha 10/ }).click();
    const sheet = m.getByRole('dialog', { name: 'Porta' });
    await expect(sheet.getByRole('heading', { name: 'Porta secreta' })).toBeVisible();
    await sheet.getByRole('button', { name: 'Revelar a porta secreta' }).click();
    await expect(sheet.getByText('Os jogadores vão ver a porta. Revelar?')).toBeVisible();
    // Nothing is revealed before the second tap.
    expect((await layers(p, campaignId, table.mapId)).door(8, 9)).toBe(0);
    await sheet.getByRole('button', { name: 'Revelar', exact: true }).click();
    await expect(sheet).toHaveCount(0);

    const after = await layers(p, campaignId, table.mapId);
    expect(after.door(8, 9)).toBe(CLOSED);
    expect(after.wall(8, 9)).toBe(0);
    expect((await layers(m, campaignId, table.mapId)).door(8, 9)).toBe(CLOSED);
    await expect(m.getByRole('list', { name: 'Legenda do mapa' }).getByText('Porta secreta')).toHaveCount(0);

    // The master unlocks the other door from its sheet: the lock is the master's, and the player's map still says "Porta fechada".
    // Toren's "Vez" tag sits over that square and lets the tap through to the door.
    await m.getByRole('button', { name: /^Porta trancada, coluna 6, linha 6/ }).click();
    await expect(m.getByRole('radio', { name: /Trancada/ })).toHaveAttribute('aria-checked', 'true');
    await m.getByRole('radio', { name: /Aberta/ }).click();
    await expect(m.getByRole('radio', { name: /Aberta/ })).toHaveAttribute('aria-checked', 'true');
    await m.getByRole('button', { name: 'Pronto' }).click();
    expect((await layers(p, campaignId, table.mapId)).door(5, 5)).toBe(OPEN);
  } finally {
    await done();
  }
});

async function atCave(browser: Browser, name: string, body: (scene: { master: Page; player: Page; table: Awaited<ReturnType<typeof tableForFog>> }) => Promise<void>): Promise<void> {
  const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 1000 } });
  const pensantus = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 1000 } });
  const toren = await newSignedInContext(browser, 'E-mail Não Verificado', { viewport: { width: 1280, height: 1000 } });
  const [mp, ap, bp] = await Promise.all([master.newPage(), pensantus.newPage(), toren.newPage()]);
  let campaignId = '';
  try {
    await Promise.all([mp.goto('/'), ap.goto('/'), bp.goto('/')]);
    const table = await tableForFog(mp, ap, bp, `${name} ${Date.now()}`);
    campaignId = table.campaignId;
    await body({ master: mp, player: ap, table });
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(mp, campaignId);
    }
    await Promise.all([master.close(), pensantus.close(), toren.close()]);
  }
}

test('na sessão, fora de combate, o mestre abre, fecha e tranca uma porta e o jogador vê a mudança na hora @MR-010 @RN-26 @RN-10', async ({ browser }) => {
  await atCave(browser, 'Portas na sessão', async ({ master, player, table }) => {
    // The wall between the rubble room and the east, at (7, 9), has floor on both sides: a closed door goes in it. Pensantus (5, 8) sees it.
    const target = { campaignId: table.campaignId, mapId: table.mapId };
    await paintLayer(master, target, 'MAP_LAYER_WALL', 0, [[7, 9]]);
    await paintLayer(master, target, 'MAP_LAYER_DOORS', CLOSED, [[7, 9]]);
    await Promise.all([master.goto(sessionRoute(table.campaignId)), player.goto(sessionRoute(table.campaignId))]);
    await expect(player.getByRole('list', { name: 'Legenda do mapa' }).getByText('Porta fechada')).toBeVisible();

    // The master's door button: its name says the kind and the place; the sheet has three choices and a "Pronto".
    await master.getByRole('button', { name: /^Porta fechada, coluna 8, linha 10/ }).click();
    const sheet = master.getByRole('dialog', { name: 'Porta' });
    await expect(sheet.getByRole('heading', { name: 'Porta' })).toBeVisible();
    await expect(sheet.getByRole('radio')).toHaveCount(3);
    await expect(sheet.getByRole('radio', { name: /Fechada/ })).toHaveAttribute('aria-checked', 'true');

    // Open: it reaches the player's map at once, through the stream.
    await sheet.getByRole('radio', { name: /Aberta/ }).click();
    await expect(sheet.getByRole('radio', { name: /Aberta/ })).toHaveAttribute('aria-checked', 'true');
    await expect(player.getByRole('list', { name: 'Legenda do mapa' }).getByText('Porta aberta')).toBeVisible();
    expect((await layers(player, table.campaignId, table.mapId)).door(7, 9)).toBe(OPEN);

    // Locked: the master sees the padlock; the player's map says "Porta fechada" and never "trancada" (RN-26, RN-10).
    await sheet.getByRole('radio', { name: /Trancada/ }).click();
    await expect(sheet.getByRole('radio', { name: /Trancada/ })).toHaveAttribute('aria-checked', 'true');
    await sheet.getByRole('button', { name: 'Pronto' }).click();
    await expect(sheet).toHaveCount(0);
    await expect(master.getByRole('list', { name: 'Legenda do mapa' }).getByText('Porta trancada (só você vê)')).toBeVisible();
    await expect(player.getByRole('list', { name: 'Legenda do mapa' }).getByText('Porta fechada')).toBeVisible();
    await expect(player.getByText('trancada')).toHaveCount(0);
    const theirs = await layers(player, table.campaignId, table.mapId);
    expect(theirs.door(7, 9)).toBe(CLOSED);
    expect(theirs.all().join(' ')).not.toMatch(/:3|:5/);
  });
});
