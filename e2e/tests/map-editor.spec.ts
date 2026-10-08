import { expect, test, type Browser, type Page } from '@playwright/test';

import { cavePoints, clickSquare, dragSquares, editorRoute, getMapRPC, layersOf, mapToPaint, surfaceOf } from './editor-support';
import { beginFogCombat, moveTo, sessionRoute, tableForFog, visionOf, type FogTable } from './fog-support';
import { endOpenSessionRPC } from './live-session-support';
import { tableForMaps } from './maps-support';
import { afterRender, boxOf, callRPC, newSignedInContext } from './support';

// The map editor on a computer (Etapa 9, slice 9.12: MR-034, MR-035, MR-036, MR-041, RN-10; E9-01 and E9-02): painting the layers, the
// fog's settings, the questions in place, the Luz, Armadilha and Tesouro points with their presets, and "Ver como". Setup goes through the
// API; every test makes its own campaign. The tests read what the server kept (`GetMapLayers`, `GetMap`, `GetMapVision`) and the words on
// the screen, never the pixels.

// Each test makes its own campaign, with the cave and its players through the API: the default 30 s is for tests that start from less.
test.describe.configure({ timeout: 120_000 });

const tools = (page: Page) => page.getByRole('group', { name: 'Ferramenta de pintura' });
// The tag, and under it the polite line a screen reader hears once: the first is the one a person sees.
const saved = (page: Page) => page.locator('app-layers-panel').getByText('Tudo salvo').first();

/** Opens the editor of a map on "Pintar". */
async function paintMode(page: Page, campaignId: string, mapId: string): Promise<void> {
  await page.goto(editorRoute(campaignId, mapId));
  await expect(page.getByRole('radio', { name: 'Pontos' })).toBeVisible();
  await page.getByRole('radio', { name: 'Pintar' }).click();
}

async function atMaps(browser: Browser, name: string, columns: number, body: (scene: { master: Page; player: Page; campaignId: string; mapId: string; map: { columns: number; rows: number }; characterId: string }) => Promise<void>): Promise<void> {
  const masterContext = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 1000 } });
  const playerContext = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 1000 } });
  try {
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    await Promise.all([master.goto('/'), player.goto('/')]);
    const table = await tableForMaps(master, player, `${name} ${Date.now()}`);
    const map = await mapToPaint(master, table.campaignId, 'A caverna do Vale Seco', columns);
    await body({ master, player, campaignId: table.campaignId, mapId: map.mapId, map, characterId: table.characterId });
  } finally {
    await masterContext.close();
    await playerContext.close();
  }
}

interface CaveScene {
  table: FogTable;
  mp: Page;
  ap: Page;
  bp: Page;
}

/** The cave at the table (the master, Pensantus's player and Toren's), with the fog as asked; the session is ended whatever happens. */
async function atCave(browser: Browser, name: string, options: { noFog?: boolean }, body: (scene: CaveScene) => Promise<void>): Promise<void> {
  const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 1000 } });
  const pensantus = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 1000 } });
  const toren = await newSignedInContext(browser, 'E-mail Não Verificado', { viewport: { width: 1280, height: 1000 } });
  const [mp, ap, bp] = await Promise.all([master.newPage(), pensantus.newPage(), toren.newPage()]);
  let campaignId = '';
  try {
    await Promise.all([mp.goto('/'), ap.goto('/'), bp.goto('/')]);
    const table = await tableForFog(mp, ap, bp, `${name} ${Date.now()}`, options);
    campaignId = table.campaignId;
    await body({ table, mp, ap, bp });
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(mp, campaignId);
    }
    await Promise.all([master.close(), pensantus.close(), toren.close()]);
  }
}

test('o mestre pinta paredes, terreno e cobertura, apaga com Shift, e o servidor guarda tudo @MR-034', async ({ browser }) => {
  await atMaps(browser, 'Pintar', 20, async ({ master, player, campaignId, mapId, map }) => {
    await paintMode(master, campaignId, mapId);
    await expect(master.locator('app-layers-panel')).toContainText('Terreno difícil');
    await expect(saved(master)).toBeVisible();

    // Walls: a drag of four squares, square by square, saved in a batch.
    await tools(master).getByRole('button', { name: 'Parede' }).click();
    await dragSquares(master, map, [5, 5], [8, 5]);
    await expect(master.locator('app-layers-panel')).toContainText('4 quadrados · bloqueia movimento, visão e luz');
    await expect(saved(master)).toBeVisible();
    expect((await layersOf(master, campaignId, mapId)).wall).toBe(4);

    // Difficult terrain: a click paints one square.
    await tools(master).getByRole('button', { name: 'Terreno difícil' }).click();
    await clickSquare(master, map, 10, 8);
    await expect(master.locator('app-layers-panel')).toContainText('1 quadrado · custa +1,5 m por quadrado');
    await expect(saved(master)).toBeVisible();

    // Cover: a second line asks for the degree; the list never names an object.
    await tools(master).getByRole('button', { name: 'Cobertura' }).click();
    await master.getByRole('radio', { name: 'Três quartos' }).click();
    await clickSquare(master, map, 2, 2);
    await master.getByRole('radio', { name: 'Meia' }).click();
    await clickSquare(master, map, 3, 2);
    await expect(master.locator('app-layers-panel')).toContainText('1 quadrado de meia cobertura e 1 de três quartos');
    await expect(saved(master)).toBeVisible();

    // Shift erases the chosen tool's layer; so does "Apagar".
    await tools(master).getByRole('button', { name: 'Parede' }).click();
    await master.keyboard.down('Shift');
    await clickSquare(master, map, 8, 5);
    await master.keyboard.up('Shift');
    await expect(master.locator('app-layers-panel')).toContainText('3 quadrados · bloqueia movimento, visão e luz');
    await tools(master).getByRole('button', { name: 'Apagar' }).click();
    await clickSquare(master, map, 7, 5);
    await expect(master.locator('app-layers-panel')).toContainText('2 quadrados · bloqueia movimento, visão e luz');
    await expect(saved(master)).toBeVisible();

    const bits = await layersOf(master, campaignId, mapId);
    expect(bits).toMatchObject({ wall: 2, terrain: 1, half: 1, threeQuarters: 1 });

    // It stays after a reload, and the player of a map they see receives the walls and the terrain, never the light.
    await master.reload();
    await master.getByRole('radio', { name: 'Pintar' }).click();
    await expect(master.locator('app-layers-panel')).toContainText('2 quadrados · bloqueia movimento, visão e luz');
    expect(await layersOf(player, campaignId, mapId)).toMatchObject({ wall: 2, terrain: 1, half: 1, threeQuarters: 1, light: 0 });
  });
});

test('o pincel 3 × 3 pinta nove quadrados, e o teclado pinta quadrado a quadrado: setas, Espaço e Esc @MR-034', async ({ browser }) => {
  await atMaps(browser, 'Teclado', 20, async ({ master, campaignId, mapId, map }) => {
    await paintMode(master, campaignId, mapId);
    await tools(master).getByRole('button', { name: 'Terreno difícil' }).click();
    await master.getByRole('radio', { name: '3×3' }).click();
    await clickSquare(master, map, 10, 6);
    await expect(master.locator('app-layers-panel')).toContainText('9 quadrados');
    await expect(saved(master)).toBeVisible();
    await master.getByRole('radio', { name: '1×1' }).click();

    // The keyboard: the surface takes the focus, the arrows move the brush, Espaço paints, Esc goes back to the tool.
    await tools(master).getByRole('button', { name: 'Parede' }).click();
    const surface = surfaceOf(master);
    await surface.focus();
    await master.keyboard.press('ArrowRight');
    await master.keyboard.press('Space');
    await master.keyboard.press('ArrowRight');
    await master.keyboard.press('Space');
    await expect(master.locator('app-layers-panel')).toContainText('2 quadrados · bloqueia movimento, visão e luz');
    await master.keyboard.press('Escape');
    await expect(tools(master).getByRole('button', { name: 'Parede' })).toBeFocused();
    await expect(saved(master)).toBeVisible();
    expect(await layersOf(master, campaignId, mapId)).toMatchObject({ wall: 2, terrain: 9 });
  });
});

test('um mapa sem grade não tem o que pintar: as ferramentas dizem por quê, e "Definir a grade" as liga @MR-034 @MR-036', async ({ browser }) => {
  await atMaps(browser, 'Sem grade', 0, async ({ master, campaignId, mapId }) => {
    await paintMode(master, campaignId, mapId);
    await expect(master.getByText('Defina a grade para pintar e ligar a névoa.')).toBeVisible();
    const wall = tools(master).getByRole('button', { name: 'Parede' });
    await expect(wall).toHaveAttribute('aria-disabled', 'true');
    await expect(wall).toHaveAttribute('aria-describedby', 'bar-why');
    await expect(surfaceOf(master)).toHaveCount(0);
    const fog = master.getByRole('switch', { name: 'Ligar a névoa' });
    await expect(fog).toHaveAttribute('aria-disabled', 'true');
    await expect(master.getByText('Precisa da grade definida.')).toBeVisible();

    await master.getByLabel('Colunas da grade').fill('3');
    await expect(master.getByText('Use um número inteiro de 4 a 200.')).toBeVisible();
    await master.getByLabel('Colunas da grade').fill('20');
    await master.getByRole('button', { name: 'Definir a grade' }).click();
    await expect(master.getByText('20 × 13 quadrados')).toBeVisible();
    await expect(surfaceOf(master)).toBeVisible();
    await expect(wall).not.toHaveAttribute('aria-disabled', 'true');
    await expect(fog).not.toHaveAttribute('aria-disabled', 'true');
  });
});

test('mudar a grade pergunta no lugar quando há o que apagar: "Voltar" não apaga nada, o segundo clique apaga @MR-034', async ({ browser }) => {
  await atMaps(browser, 'Mudar a grade', 20, async ({ master, campaignId, mapId, map }) => {
    await paintMode(master, campaignId, mapId);
    // With nothing painted or seen the grid changes with no warning.
    await master.getByRole('button', { name: 'Mudar a grade' }).click();
    await expect(master.getByRole('heading', { name: 'Mudar a grade?' })).toBeFocused();
    await expect(master.getByText('apaga o terreno, as paredes')).toHaveCount(0);
    await master.getByRole('button', { name: 'Voltar' }).click();
    await expect(master.getByRole('button', { name: 'Mudar a grade' })).toBeFocused();

    await tools(master).getByRole('button', { name: 'Parede' }).click();
    await dragSquares(master, map, [3, 3], [6, 3]);
    await expect(saved(master)).toBeVisible();

    await master.getByRole('button', { name: 'Mudar a grade' }).click();
    await expect(master.getByRole('heading', { name: 'Mudar a grade?' })).toBeFocused();
    await expect(master.getByText('Mudar a grade apaga o terreno, as paredes, a cobertura e a luz pintados, e o que os jogadores já viram.')).toBeVisible();
    await master.getByLabel('Colunas').fill('30');
    await expect(master.getByText('Linhas: 20, pela proporção da imagem. A grade ficaria com 30 × 20 quadrados.')).toBeVisible();
    // The first click asked; nothing is erased until the second.
    expect(await layersOf(master, campaignId, mapId)).toMatchObject({ wall: 4, columns: 20 });
    await master.getByRole('button', { name: 'Voltar' }).click();
    expect(await layersOf(master, campaignId, mapId)).toMatchObject({ wall: 4, columns: 20 });

    await master.getByRole('button', { name: 'Mudar a grade' }).click();
    await master.getByLabel('Colunas').fill('30');
    await master.getByRole('button', { name: 'Apagar e mudar a grade' }).click();
    await expect(master.getByText('30 × 20 quadrados')).toBeVisible();
    expect(await layersOf(master, campaignId, mapId)).toMatchObject({ wall: 0, columns: 30, rows: 20 });
    await expect(master.locator('app-layers-panel')).toContainText('nada pintado');
  });
});

test('trocar a imagem pergunta no lugar quando há o que apagar, e o texto avisa que os jogadores veem o que está na imagem @MR-034', async ({ browser }) => {
  await atMaps(browser, 'Imagem', 20, async ({ master, campaignId, mapId, map }) => {
    await paintMode(master, campaignId, mapId);
    await expect(master.getByText('O que está desenhado na imagem, os jogadores veem.')).toBeVisible();
    await tools(master).getByRole('button', { name: 'Parede' }).click();
    await clickSquare(master, map, 3, 3);
    await expect(saved(master)).toBeVisible();

    await master.getByRole('button', { name: 'Trocar imagem' }).click();
    await expect(master.getByRole('heading', { name: 'Trocar a imagem?' })).toBeFocused();
    await expect(master.getByText('Imagem agora: A caverna do Vale Seco')).toBeVisible();
    await expect(master.getByText('Os pontos e os tokens ficam.')).toBeVisible();
    await expect(master.getByRole('dialog')).toHaveCount(0);
    await master.getByRole('button', { name: 'Voltar' }).first().click();
    await expect(master.getByRole('button', { name: 'Trocar imagem' })).toBeFocused();
    expect((await layersOf(master, campaignId, mapId)).wall).toBe(1);
    // The second click goes on to the gallery's picker; nothing is erased until an image is chosen there.
    await master.getByRole('button', { name: 'Trocar imagem' }).click();
    await master.getByRole('button', { name: 'Apagar e trocar a imagem' }).click();
    await expect(master.getByRole('dialog', { name: 'Trocar a imagem do mapa' })).toBeVisible();
    expect((await layersOf(master, campaignId, mapId)).wall).toBe(1);
  });
});

test('as configurações da névoa: ligar, a luz de base, a visão do grupo e "Esquecer o que foi visto", que pergunta antes @MR-036', async ({ browser }) => {
  await atCave(browser, 'Névoa', { noFog: true }, async ({ table, mp }) => {
    await paintMode(mp, table.campaignId, table.mapId);
    const fog = mp.getByRole('switch', { name: 'Ligar a névoa' });
    await expect(fog).toHaveAttribute('aria-checked', 'false');
    await expect(mp.getByText('Sem névoa, não há o que esquecer.')).toBeVisible();
    await fog.click();
    await expect(fog).toHaveAttribute('aria-checked', 'true');
    expect((await getMapRPC(mp, table.campaignId, table.mapId)).map).toMatchObject({ fogEnabled: true });

    await mp.getByRole('radio', { name: 'Penumbra' }).click();
    await expect(mp.getByRole('radio', { name: 'Penumbra' })).toHaveAttribute('aria-checked', 'true');
    expect((await getMapRPC(mp, table.campaignId, table.mapId)).map).toMatchObject({ baseLight: 'LIGHT_LEVEL_DIM' });
    await mp.getByRole('radio', { name: 'Escuro' }).click();
    await expect(mp.getByRole('radio', { name: 'Escuro' })).toHaveAttribute('aria-checked', 'true');

    const group = mp.getByRole('switch', { name: 'Visão do grupo' });
    await group.click();
    await expect(group).toHaveAttribute('aria-checked', 'true');
    await expect(mp.getByText('Ligada: todos veem o que o grupo todo vê.')).toBeVisible();
    expect((await getMapRPC(mp, table.campaignId, table.mapId)).map).toMatchObject({ fogEnabled: true, groupVision: true });
    await group.click();

    // Pensantus walks, so there is something remembered; "Esquecer" asks first.
    await moveTo(mp, table, table.pensantusId, 10, 13);
    await moveTo(mp, table, table.pensantusId, 5, 8);
    const remembered = async () => (await visionOf(mp, table, table.pensantusId)).states.filter((s) => s === 5).length;
    await expect.poll(remembered).toBeGreaterThan(0);
    await mp.getByRole('button', { name: 'Esquecer o que foi visto' }).click();
    await expect(mp.getByRole('heading', { name: 'Esquecer o que foi visto?' })).toBeFocused();
    await expect(mp.getByText('Isso não dá para desfazer.')).toBeVisible();
    await mp.getByRole('button', { name: 'Voltar' }).click();
    await expect(mp.getByRole('button', { name: 'Esquecer o que foi visto' })).toBeFocused();
    expect(await remembered()).toBeGreaterThan(0);
    await mp.getByRole('button', { name: 'Esquecer o que foi visto' }).click();
    await mp.getByRole('button', { name: 'Esquecer o que foi visto' }).last().click();
    await expect(mp.getByText('os jogadores voltaram a ver só o que o personagem deles vê agora')).toBeVisible();
    expect(await remembered()).toBe(0);
  });
});

test('o que o mestre pinta chega ao jogador só onde o personagem dele enxerga @MR-036 @RN-10', async ({ browser }) => {
  await atCave(browser, 'Pintar e a névoa', {}, async ({ table, mp, ap }) => {
    const view = await visionOf(mp, table, table.pensantusId);
    const seeNow = (n: number) => view.states[n] === 2 || view.states[n] === 3 || view.states[n] === 4;
    // A floor square Pensantus sees now, and one he does not see at all, both clear of anything painted.
    const floor: number[] = [];
    for (let n = 0; n < 24 * 16; n++) {
      const row = Math.floor(n / 24);
      const col = n % 24;
      if (row >= 2 && col < 23 && ![':', '#', 'h', 'q'].includes(CAVE_AT(col, row))) {
        floor.push(n);
      }
    }
    const nearby = floor.find((n) => seeNow(n) && n % 24 > 5)!;
    const faraway = floor.find((n) => view.states[n] === 0)!;
    expect(nearby).toBeDefined();
    expect(faraway).toBeDefined();

    await ap.goto(sessionRoute(table.campaignId));
    await expect(ap.locator('app-fog-base').first()).toBeVisible();
    // What the player's map holds, as the server answers for him: the screen reads it again when the stream says the layers changed.
    const terrain = async () => (await layersOf(ap, table.campaignId, table.mapId)).terrain;
    const before = await terrain();
    const masterBefore = (await layersOf(mp, table.campaignId, table.mapId)).terrain;

    await paintMode(mp, table.campaignId, table.mapId);
    const map = { columns: 24, rows: 16 };
    await tools(mp).getByRole('button', { name: 'Terreno difícil' }).click();
    await clickSquare(mp, map, faraway % 24, Math.floor(faraway / 24));
    await expect(saved(mp)).toBeVisible();
    await clickSquare(mp, map, nearby % 24, Math.floor(nearby / 24));
    await expect(saved(mp)).toBeVisible();
    // The master has both.
    expect((await layersOf(mp, table.campaignId, table.mapId)).terrain).toBe(masterBefore + 2);
    // The player gets the one in sight and not the other, and his legend names the terrain.
    await expect.poll(terrain, { timeout: 15_000 }).toBe(before + 1);
    await expect(ap.getByText('Terreno difícil').first()).toBeVisible();
  });
});

/** The cave's character at a square (the rubble, the walls, the crates and the column are not plain floor). */
function CAVE_AT(col: number, row: number): string {
  return CAVE_ROWS[row]?.[col] ?? '#';
}
const CAVE_ROWS = [
  '########################',
  '########################',
  '################.......#',
  '################.......#',
  '################....q..#',
  '################.......#',
  '#......#########.......#',
  '...................h...#',
  '...................h...#',
  '#...::.#..######.......#',
  '#...::.#..##############',
  '######........##########',
  '######........##########',
  '######........##########',
  '######........##########',
  '########################',
];

test('com um combate no mapa, pintar vale e a grade e a imagem ficam desligadas, com o motivo escrito uma vez @MR-034', async ({ browser }) => {
  await atCave(browser, 'Combate e o editor', {}, async ({ table, mp }) => {
    await beginFogCombat(mp, table);
    await paintMode(mp, table.campaignId, table.mapId);
    await expect(mp.getByText('Um combate está em andamento neste mapa. Dá para pintar e apagar; a grade e a imagem só mudam depois dele.')).toBeVisible();
    const change = mp.getByRole('button', { name: 'Mudar a grade' });
    await expect(change).toHaveAttribute('aria-disabled', 'true');
    await expect(mp.getByText('Há um combate neste mapa. Termine-o para mudar a grade.')).toBeVisible();
    await change.click({ force: true });
    await afterRender(mp);
    await expect(mp.getByRole('heading', { name: 'Mudar a grade?' })).toHaveCount(0);
    const swap = mp.getByRole('button', { name: 'Trocar imagem' });
    await expect(swap).toHaveAttribute('aria-disabled', 'true');
    await expect(swap).toHaveAttribute('aria-describedby', 'combat-why');
    // The server refuses it too, by the typed reason.
    const refused = await callRPC(mp, 'meurpg.maps.v1.MapService/SetMapGrid', { campaignId: table.campaignId, mapId: table.mapId, columns: 20 });
    expect(refused.status()).toBe(400);

    // Painting still works: a wall the combat reads.
    const walls = (await layersOf(mp, table.campaignId, table.mapId)).wall;
    await tools(mp).getByRole('button', { name: 'Parede' }).click();
    await clickSquare(mp, { columns: 24, rows: 16 }, 17, 3);
    await expect(saved(mp)).toBeVisible();
    expect((await layersOf(mp, table.campaignId, table.mapId)).wall).toBe(walls + 1);
  });
});

test('uma armadilha nasce de uma predefinição do SRD, e "Quem notaria" mostra os números do servidor @MR-035 @RN-10', async ({ browser }) => {
  await atCave(browser, 'Armadilhas', { noFog: true }, async ({ table, mp, ap }) => {
    await mp.goto(editorRoute(table.campaignId, table.mapId));
    await expect(mp.getByRole('radio', { name: 'Pontos' })).toBeVisible();

    // The pit: the kind, a click on the map, the preset fills the form.
    await mp.getByRole('group', { name: 'Adicionar ponto' }).getByRole('button', { name: 'Armadilha' }).click();
    const map = mp.getByRole('group', { name: /^Mapa / });
    const box = await boxOf(map);
    await mp.mouse.click(box.x + box.width * (11.5 / 24), box.y + box.height * (7.5 / 16));
    await expect(mp.getByRole('heading', { name: 'Predefinições do SRD' })).toBeVisible();
    await expect(mp.getByLabel('Nome', { exact: true })).toBeFocused();
    await mp.getByRole('radio', { name: /Fosso escondido/ }).click();
    await expect(mp.getByLabel('Nome', { exact: true })).toHaveValue('Fosso escondido');
    await expect(mp.getByLabel('CD para notar (Percepção)')).toHaveValue('15');
    await expect(mp.getByLabel('CD para achar (Investigação)')).toHaveValue('15');
    await expect(mp.getByRole('radio', { name: '2×2' })).toHaveAttribute('aria-checked', 'true');
    await expect(mp.getByText('Não desenhe a armadilha na imagem: os jogadores veem a imagem.')).toBeVisible();
    await expect(mp.getByRole('heading', { name: 'Dano que sempre acontece' })).toBeVisible();
    await mp.getByRole('button', { name: 'Salvar ponto' }).click();
    await expect(mp.getByText('Fosso escondido salvo.')).toBeAttached();

    const pit = (await getMapRPC(mp, table.campaignId, table.mapId)).points.find((p) => p.name === 'Fosso escondido') as { trap: Record<string, unknown>; revealed?: boolean };
    expect(pit.trap).toMatchObject({ presetKey: 'trap:hidden-pit', noticeDc: 15, findDc: 15, areaSize: 2, trigger: 'TRAP_TRIGGER_ENTER' });
    expect(pit.revealed ?? false).toBe(false);

    // "Quem notaria": each player character with the server's passive Perception, and the verdict the server worked out.
    const who = mp.getByRole('heading', { name: 'Quem notaria' });
    await expect(who).toBeVisible();
    await expect(mp.getByText('Percepção passiva contra a CD 15 para notar')).toBeVisible();
    for (const name of ['Pensantus', 'Toren']) {
      await expect(mp.locator('app-trap-noticers').getByText(name, { exact: true })).toBeVisible();
    }
    await expect(mp.locator('app-trap-noticers').getByText('Percepção passiva').first()).toBeVisible();

    // A trap does not exist for a player until it is found (RN-10): not in their map, not by name.
    const seen = await getMapRPC(ap, table.campaignId, table.mapId);
    expect(seen.points.map((p) => p.name)).not.toContain('Fosso escondido');

    // The needle: no DC to notice, so nobody notices it alone; the effect in parts, the trigger by hand.
    await mp.getByRole('group', { name: 'Adicionar ponto' }).getByRole('button', { name: 'Armadilha' }).click();
    await mp.mouse.click(box.x + box.width * (7.5 / 24), box.y + box.height * (13.5 / 16));
    // The new point opens its own panel (the server's first sample trap fills it); the old one is gone before the next click.
    await expect(mp.getByRole('heading', { name: 'Nova armadilha' })).toBeVisible();
    await mp.getByRole('radio', { name: /Agulha envenenada/ }).click();
    await expect(mp.getByLabel('CD para notar (Percepção)')).toHaveValue('');
    await expect(mp.getByLabel('CD para achar (Investigação)')).toHaveValue('20');
    await expect(mp.getByRole('radio', { name: 'Manual', exact: true }).first()).toHaveAttribute('aria-checked', 'true');
    await expect(mp.getByRole('heading', { name: 'Teste de resistência' })).toBeVisible();
    await expect(mp.getByText('CDs de resistência do SRD: revés 10 a 11 · perigosa 12 a 15 · mortal 16 a 20.')).toBeVisible();
    await mp.getByRole('button', { name: 'Salvar ponto' }).click();
    await expect(mp.getByText('Ninguém nota esta armadilha sozinho')).toBeVisible();
    const needle = (await getMapRPC(mp, table.campaignId, table.mapId)).points.find((p) => p.name === 'Agulha envenenada') as { trap: { effect: { damage: unknown[]; save: Record<string, unknown> } } };
    expect(needle.trap.effect.damage).toHaveLength(2);
    expect(needle.trap.effect.save).toMatchObject({ ability: 'ABILITY_CONSTITUTION', dc: 15 });

    // A part is removed with its own bin and added with a text action; a bad die is said in the field.
    // Each step waits for the form to settle: filling `.last()` before the new
    // part rendered would fill the old field and leave the new one empty (and in
    // error too).
    await mp.getByRole('button', { name: 'Remover a parte Dano que sempre acontece 2' }).click();
    await expect(mp.getByRole('button', { name: 'Remover a parte Dano que sempre acontece 2' })).toHaveCount(0);
    const damage = mp.getByLabel('Dano', { exact: true });
    const before = await damage.count();
    await mp.getByRole('button', { name: 'Dano que sempre acontece' }).last().click();
    await expect(damage).toHaveCount(before + 1);
    await damage.last().fill('2d20');
    await mp.getByRole('button', { name: 'Salvar ponto' }).click();
    await expect(mp.getByText('Use dados como 2d6 (de d4 a d12) ou um número de 1 a 100.')).toBeVisible();
  });
});

test('um ponto de Luz usa uma fonte do SRD ou os raios do mestre, e o jogador nunca recebe o ponto @MR-036 @RN-10', async ({ browser }) => {
  await atCave(browser, 'Luz', { noFog: true }, async ({ table, mp, ap }) => {
    await mp.goto(editorRoute(table.campaignId, table.mapId));
    await mp.getByRole('group', { name: 'Adicionar ponto' }).getByRole('button', { name: 'Luz', exact: true }).click();
    const box = await boxOf(mp.getByRole('group', { name: /^Mapa / }));
    await mp.mouse.click(box.x + box.width * (13.5 / 24), box.y + box.height * (12.5 / 16));
    await expect(mp.getByLabel('Nome', { exact: true })).toBeFocused();
    await mp.getByLabel('Nome', { exact: true }).fill('Brasa do altar');
    // The sources of the SRD (and the others the server lists) are radios; a new light starts as the first one the server lists.
    const presets = (await (await callRPC(mp, 'meurpg.rules.v1.ContentService/ListLightPresets', { campaignId: table.campaignId })).json()).presets as { namePt: string }[];
    expect(presets.length).toBeGreaterThan(2);
    await expect(mp.getByRole('radio', { name: new RegExp(presets[0].namePt) }).first()).toHaveAttribute('aria-checked', 'true');
    await mp.getByRole('radio', { name: /Tocha/ }).click();
    await expect(mp.getByRole('radio', { name: /Tocha/ })).toHaveAttribute('aria-checked', 'true');
    await expect(mp.getByText('Raios: 4 quadrados de luz clara e mais 4 de penumbra, até 8 quadrados no total.')).toBeVisible();
    // The reach is drawn as two rings (the radii, never the lit squares) and the legend names them, with the honest note about the walls.
    await expect(mp.getByText('Alcance da luz clara')).toBeVisible();
    await expect(mp.getByText('Alcance da penumbra')).toBeVisible();
    await expect(mp.getByText('As paredes cortam a luz; o alcance de verdade aparece em “Ver como”.')).toBeVisible();

    await mp.getByRole('radio', { name: /Personalizada/ }).click();
    await mp.getByLabel('Luz clara até').fill('4');
    await mp.getByLabel('Mais penumbra até').fill('4,5');
    await mp.getByRole('button', { name: 'Salvar ponto' }).click();
    await expect(mp.getByText('Use múltiplos de 1,5 m (um quadrado), de 0 a 36 m.')).toBeVisible();
    await mp.getByLabel('Luz clara até').fill('4,5');
    await mp.getByRole('button', { name: 'Salvar ponto' }).click();
    await expect(mp.getByText('Brasa do altar salvo.')).toBeAttached();
    const light = (await getMapRPC(mp, table.campaignId, table.mapId)).points.find((p) => p.name === 'Brasa do altar') as { light: Record<string, unknown> };
    expect(light.light).toMatchObject({ brightFt: 15, dimFt: 15 });
    expect(light.light.presetKey ?? '').toBe('');

    await mp.getByRole('radio', { name: /Luz do Dia/ }).click();
    await mp.getByRole('button', { name: 'Salvar ponto' }).click();
    const dayLight = async () => ((await getMapRPC(mp, table.campaignId, table.mapId)).points.find((p) => p.name === 'Brasa do altar') as { light: Record<string, unknown> }).light;
    await expect.poll(dayLight).toMatchObject({ presetKey: 'light:daylight', brightFt: 60, dimFt: 60 });

    // The point is the master's: the player's map has no light point at all.
    expect((await getMapRPC(ap, table.campaignId, table.mapId)).points.map((p) => p.name)).not.toContain('Brasa do altar');
  });
});

test('um tesouro marcado como encontrado fora de uma sessão: quem encontrou, a linha do resumo e o "Desmarcar" no lugar @MR-041', async ({ browser }) => {
  await atMaps(browser, 'Tesouro', 20, async ({ master, player, campaignId, mapId, characterId }) => {
    await master.goto(editorRoute(campaignId, mapId));
    await master.getByRole('group', { name: 'Adicionar ponto' }).getByRole('button', { name: 'Tesouro' }).click();
    const box = await boxOf(master.getByRole('group', { name: /^Mapa / }));
    await master.mouse.click(box.x + box.width * 0.4, box.y + box.height * 0.6);
    await expect(master.getByLabel('Nome', { exact: true })).toBeFocused();
    await master.getByLabel('Nome', { exact: true }).fill('Baú de moedas');
    await master.getByLabel('Descrição para os jogadores').fill('250 PO e uma adaga de prata.');
    await master.getByLabel('Valor em ouro').fill('250');
    await master.getByRole('button', { name: 'Salvar ponto' }).click();
    await expect(master.getByText('Baú de moedas salvo.')).toBeAttached();
    await expect(master.getByText('Não encontrado')).toBeVisible();
    await expect(master.getByText('Os jogadores não o veem, nem na lista.')).toBeVisible();

    // The player sees nothing of it, not its value.
    expect((await getMapRPC(player, campaignId, mapId)).points.map((p) => p.name)).not.toContain('Baú de moedas');

    await expect(master.getByText('Marcado fora de uma sessão, o tesouro não entra em resumo nenhum.')).toBeVisible();
    await master.getByRole('button', { name: 'Marcar como encontrado' }).click();
    await expect(master.getByRole('group', { name: 'Quem encontrou Baú de moedas?' })).toBeVisible();
    await expect(master.locator('app-person-pick input[type=checkbox]:checked')).toHaveCount(0);
    // Nobody is checked at first: the filled button is the dashed one that cannot act.
    await master.getByRole('group', { name: 'Quem encontrou Baú de moedas?' }).locator('label', { hasText: 'Pensantus' }).click();
    await expect(master.getByText('No resumo da sessão: Pensantus · 250 PO')).toBeVisible();
    await master.getByRole('button', { name: 'Marcar como encontrado' }).last().click();
    await expect(master.getByText(/Encontrado por Pensantus/)).toBeVisible();
    await expect(master.getByText('Todos veem o tesouro, o que há dentro e o valor.')).toBeVisible();

    // Now the players see it with its value, and the master cannot delete it before unmarking.
    const found = (await getMapRPC(player, campaignId, mapId)).points.find((p) => p.name === 'Baú de moedas') as { treasureValuePo: number; treasureFoundBy: { characterId: string }[] };
    expect(found.treasureValuePo).toBe(250);
    expect(found.treasureFoundBy.map((f) => f.characterId)).toEqual([characterId]);
    await expect(master.getByRole('button', { name: 'Apagar ponto' })).toHaveAttribute('aria-disabled', 'true');
    await expect(master.getByText('Este tesouro foi encontrado. Desmarque antes de apagar.')).toBeVisible();

    // "Desmarcar" asks in place, with the focus on the question.
    await master.getByRole('button', { name: 'Desmarcar' }).click();
    await expect(master.getByRole('heading', { name: /^Desmarcar Baú de moedas/ })).toBeFocused();
    await expect(master.getByRole('group', { name: /^Desmarcar / }).getByRole('button', { name: 'Voltar' })).toBeVisible();
    await master.getByRole('group', { name: /^Desmarcar / }).getByRole('button', { name: 'Voltar' }).click();
    await expect(master.getByText(/Encontrado por Pensantus/)).toBeVisible();
    await master.getByRole('button', { name: 'Desmarcar' }).click();
    await master.getByRole('group', { name: /^Desmarcar / }).getByRole('button', { name: 'Desmarcar' }).click();
    await expect(master.getByText('Não encontrado')).toBeVisible();
    expect((await getMapRPC(player, campaignId, mapId)).points.map((p) => p.name)).not.toContain('Baú de moedas');
    // Unmarked, it can go; deleting asks in place.
    await master.getByRole('button', { name: 'Apagar ponto' }).click();
    await expect(master.getByRole('heading', { name: 'Apagar Baú de moedas?' })).toBeFocused();
    await expect(master.getByText('Não dá para desfazer.')).toBeVisible();
    await master.getByRole('button', { name: 'Apagar ponto' }).first().click();
    await expect(master.getByText('Baú de moedas foi apagado.')).toBeAttached();
  });
});

test('"Ver como" mostra o mapa de Toren, e "Voltar à sua vista" traz o mapa do mestre de volta @MR-036 @RN-10', async ({ browser }) => {
  await atCave(browser, 'Ver como no editor', {}, async ({ table, mp, bp }) => {
    await mp.goto(editorRoute(table.campaignId, table.mapId));
    await expect(mp.getByRole('heading', { name: 'Ver como' })).toBeVisible();
    // The list counts, from the server's states, how many squares each one sees.
    const torenSees = (await visionOf(mp, table, table.torenId)).states.filter((s) => s >= 1 && s <= 4).length;
    await expect(mp.locator('app-view-as-list').getByText(`${torenSees} quadrados vistos`)).toBeVisible();

    await mp.getByRole('radio', { name: /Toren/ }).click();
    await expect(mp.getByText('Você está vendo o mapa como Toren')).toBeVisible();
    await expect(mp.getByText('Para voltar à sua vista, escolha “Todos”.')).toBeVisible();
    // What is on the screen is Toren's map: the same number of squares seen as the server's, and his own player's map says the same.
    await expect(mp.locator('app-fog-base').first()).toHaveAttribute('data-seen', String(torenSees));
    await bp.goto(sessionRoute(table.campaignId));
    await expect(bp.locator('app-fog-base').first()).toHaveAttribute('data-seen', String(torenSees));
    // The editor's bar is out of the way: nothing here can change the map.
    await expect(mp.getByRole('toolbar', { name: 'Ferramentas do mapa' })).toHaveCount(0);

    await mp.getByRole('button', { name: 'Voltar à sua vista' }).click();
    await expect(mp.getByRole('toolbar', { name: 'Ferramentas do mapa' })).toBeVisible();
    await expect(mp.getByText('Você está vendo o mapa como')).toHaveCount(0);
  });
});

test('o mapa com os pontos novos: a lista separa a armadilha e o baú do mesmo quadrado, e a legenda dá nome a cada marca @MR-035 @MR-041', async ({ browser }) => {
  await atCave(browser, 'Pontos novos', { noFog: true }, async ({ table, mp }) => {
    await cavePoints(mp, table);
    await mp.goto(editorRoute(table.campaignId, table.mapId));
    const list = mp.getByRole('region', { name: 'Pontos do mapa' });
    for (const name of ['Tocha da guarita', 'Fosso escondido', 'Agulha envenenada', 'Baú com agulha', 'Baú de moedas', 'Brasa do altar']) {
      await expect(list.getByRole('button', { name: new RegExp(name) })).toBeVisible();
    }
    await expect(list.getByRole('button', { name: /Agulha envenenada/ })).toContainText('Armada');
    await expect(list.getByRole('button', { name: /Baú de moedas/ })).toContainText('Não encontrado');
    const legend = mp.locator('app-map-pins-legend');
    await expect(legend).toContainText('Armadilha (só você vê)');
    await expect(legend).toContainText('Tesouro (escondido)');
    await expect(legend).toContainText('Luz (só você vê)');
    // The list picks the point: the panel of its kind.
    await list.getByRole('button', { name: /Baú com agulha/ }).click();
    await expect(mp.getByRole('heading', { name: 'Baú com agulha' })).toBeVisible();
    await expect(mp.getByText('Tesouro · escondido')).toBeVisible();
  });
});

test('os pontos do mesmo quadrado dividem um nome, a legenda diz só o que o mapa tem, e "Pintar" mostra o mapa limpo @MR-035 @MR-041', async ({ browser }) => {
  await atCave(browser, 'Nomes e legenda', {}, async ({ table, mp }) => {
    await cavePoints(mp, table);
    await mp.goto(editorRoute(table.campaignId, table.mapId));
    const map = mp.getByRole('group', { name: /^Mapa / });
    // The needle and its chest share a square: one name for both, not two on top of each other.
    await expect(map.getByText('Agulha envenenada · Baú com agulha')).toBeVisible();
    await expect(map.getByText('Agulha envenenada', { exact: true })).toHaveCount(0);
    // The legend names only what the map has: no Batalha, no Submapa, and the tokens as the map draws them.
    const legend = mp.locator('app-map-legend');
    await expect(legend).not.toContainText('Batalha');
    await expect(legend).not.toContainText('Submapa');
    await expect(legend).toContainText('Goblin 1');
    await expect(mp.getByText('Luz clara', { exact: true })).toHaveCount(0);

    // Painting: the names go away, so the brush and the painted light are never under a pill.
    await mp.getByRole('radio', { name: 'Pintar' }).click();
    await expect(surfaceOf(mp)).toBeVisible();
    await expect(map.getByText('Agulha envenenada · Baú com agulha')).toHaveCount(0);
    await expect(map.getByText('Fosso escondido')).toHaveCount(0);
    // The light's three levels carry the toolbar's names everywhere.
    await tools(mp).getByRole('button', { name: 'Luz' }).click();
    await mp.getByRole('radiogroup', { name: 'Luz pintada' }).getByRole('radio', { name: 'Penumbra' }).click();
    await clickSquare(mp, { columns: 24, rows: 16 }, 3, 8);
    await expect(saved(mp)).toBeVisible();
    await expect(mp.locator('app-map-layers-legend')).toContainText('Penumbra');
    await expect(mp.getByText('Luz em penumbra')).toHaveCount(0);
  });
});

test('sair com traços que o servidor não recebeu pergunta no lugar; o mestre fica ou perde os traços @MR-034', async ({ browser }) => {
  await atMaps(browser, 'Sair com traços', 20, async ({ master, campaignId, mapId, map }) => {
    await paintMode(master, campaignId, mapId);
    await tools(master).getByRole('button', { name: 'Parede' }).click();
    // The server stops answering: the stroke stays on the screen, "Não salvou" says so, with a way to try again.
    await master.route('**/*PaintMapCells', (r) => r.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'unavailable', message: 'down' }) }));
    await clickSquare(master, map, 4, 4);
    await expect(master.locator('app-layers-panel').getByText('Não salvou')).toBeVisible();
    await expect(master.getByRole('button', { name: 'Tentar de novo' })).toBeVisible();

    // Leaving through the app's own link asks first, in place; "Voltar" stays.
    await master.getByRole('link', { name: 'Minhas campanhas' }).first().click();
    await expect(master.getByRole('heading', { name: 'Há traços que o servidor não recebeu' })).toBeFocused();
    await master.getByRole('button', { name: 'Voltar' }).click();
    await expect(master.getByRole('heading', { name: 'Há traços que o servidor não recebeu' })).toHaveCount(0);

    // The server answers again: "Tentar de novo" sends the stroke, and now the page leaves without a question.
    await master.unroute('**/*PaintMapCells');
    await master.getByRole('button', { name: 'Tentar de novo' }).click();
    await expect(saved(master)).toBeVisible();
    expect((await layersOf(master, campaignId, mapId)).wall).toBe(1);
    await master.getByRole('link', { name: 'Minhas campanhas' }).first().click();
    await expect(master).toHaveURL(/\/campaigns$/);
  });
});
