import { expect, type Browser, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { getEncounterRPC } from './combat-support';
import { beginFogCombat, moveTo, sessionRoute, tableForFog, visionOf, type FogOptions, type FogTable } from './fog-support';
import { endOpenSessionRPC, showMapTool } from './live-session-support';
import { callRPC, newSignedInContext, openActionTab } from './support';
import { openCombatPanel } from './combat-layout';

// The fog of war on screen (Etapa 9, MR-036, RN-10, RN-20; E9-03 and E9-04). The tests read the shading
// (`data-shaded` and `data-seen` on the picture: how many squares of each state), the tokens the map draws
// and the words on the screen, never the pixels of the image.

interface Shading {
  dim: number;
  grey: number;
  remembered: number;
  unseen: number;
}

/** The squares of each state on the map the page draws. */
async function shading(page: Page): Promise<Shading> {
  await expect(page.locator('app-fog-base').first()).toHaveAttribute('data-shaded', /./);
  const raw = (await page.locator('app-fog-base').first().getAttribute('data-shaded')) ?? '';
  const read = (key: string) => Number(new RegExp(`${key}:(\\d+)`).exec(raw)?.[1] ?? NaN);
  return { dim: read('dim'), grey: read('grey'), remembered: read('remembered'), unseen: read('unseen') };
}

/** The names of the tokens the map draws (its list for assistive tech). */
async function tokens(page: Page): Promise<string[]> {
  return (await page.locator('app-fog-map ul[aria-label="No mapa"] li').allTextContents()).map((n) => n.trim());
}

/** Waits for the last tile: the "Carregando o mapa" notice goes. */
async function loaded(page: Page): Promise<void> {
  await expect(page.locator('app-fog-base').first()).toBeVisible();
  await expect(page.getByTestId('fog-loading')).toHaveCount(0);
}

/** How many squares the viewer sees now: the picture carries the count of the states (the card tells what the character can use, not a number). */
async function seen(page: Page): Promise<number> {
  return Number(await page.locator('app-fog-base').first().getAttribute('data-seen'));
}

/** What a player's page asked the server for: the pathnames of every request. */
function watch(page: Page): string[] {
  const paths: string[] = [];
  page.on('request', (r) => paths.push(new URL(r.url()).pathname));
  return paths;
}

/** A request for a whole image (`/images/<id>`), which a player on a fog map never makes: only tiles (`/images/maps/<id>/tiles/…`). */
const wholeImage = (path: string) => /^\/images\/(?!maps\/)/.test(path);
const tile = (path: string) => /^\/images\/maps\/[^/]+\/tiles\/\d+\/\d+$/.test(path);

interface Scene {
  table: FogTable;
  mp: Page;
  ap: Page;
  bp: Page;
}

/**
 * A table of the cave with the master, Pensantus's player and Toren's: runs `body` and, whatever happens, ends the session and closes the
 * contexts (a test that fails never leaves its session open).
 */
async function atTable(browser: Browser, options: FogOptions, name: string, body: (scene: Scene) => Promise<void>): Promise<void> {
  const master = await newSignedInContext(browser, 'Mestre Teste');
  const pensantus = await newSignedInContext(browser, 'Jogador Teste');
  const toren = await newSignedInContext(browser, 'E-mail Não Verificado');
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

test('Pensantus e Toren veem mapas diferentes no mesmo instante, e nenhum baixa a imagem inteira @MR-036 @RN-10', async ({ browser }) => {
  await atTable(browser, {}, 'Mapas diferentes', async ({ table, ap, bp }) => {
    const asked = { a: watch(ap), b: watch(bp) };
    await Promise.all([ap.goto(sessionRoute(table.campaignId)), bp.goto(sessionRoute(table.campaignId))]);
    await Promise.all([loaded(ap), loaded(bp)]);

    // Pensantus has darkvision (18 m): the cave near him is seen in grey; Toren has none and sees only the lit room.
    const a = await shading(ap);
    const b = await shading(bp);
    expect(a.grey).toBeGreaterThan(30);
    expect(b.grey).toBeLessThan(3);
    expect(a.unseen).toBeLessThan(b.unseen);
    expect(await seen(ap)).toBeGreaterThan(await seen(bp));

    // The legend names only what each map shows: Toren's has no "No escuro, em cinza".
    await expect(ap.locator('.mr-legend').getByText('No escuro, em cinza', { exact: true })).toBeVisible();
    await expect(bp.locator('.mr-legend').getByText('No escuro, em cinza', { exact: true })).toHaveCount(0);
    // The card says what the character can use: Pensantus's darkvision, and who is in sight; no count of squares.
    await expect(ap.getByTestId('fog-caption')).toContainText('Visão no escuro: 18 m');
    await expect(ap.getByTestId('fog-caption')).toContainText('Inimigos à vista: Goblin 2.');
    expect(await ap.getByTestId('fog-caption').textContent()).not.toMatch(/\d+ de \d+ quadrados/);

    // The enemies in the dark room are not on either map; the one in the torch's light is, for both.
    for (const page of [ap, bp]) {
      const names = await tokens(page);
      expect(names).toContain('Goblin 2');
      expect(names).not.toContain('Goblin 1');
      expect(names).not.toContain('Capitão Goblin');
    }
    // The party is on every map, even on a black square.
    expect(await tokens(bp)).toEqual(expect.arrayContaining(['Pensantus', 'Toren (você)']));

    // The server's reads agree with what is drawn.
    const va = await visionOf(ap, table);
    const vb = await visionOf(bp, table);
    expect(va.states.filter((s) => s === 2).length).toBe(a.grey);
    expect(vb.states.filter((s) => s === 2).length).toBe(b.grey);

    // No request of a player fetched the whole image: only the tiles of their own view.
    expect([...asked.a, ...asked.b].filter(wholeImage)).toEqual([]);
    expect(asked.a.some(tile)).toBe(true);
    expect(asked.b.some(tile)).toBe(true);
  });
});

test('depois de andar, o que já foi visto continua, escurecido, com as marcas e sem inimigos @MR-036 @RN-10', async ({ browser }) => {
  await atTable(browser, {}, 'Memória', async ({ table, mp, ap }) => {
    await ap.goto(sessionRoute(table.campaignId));
    await loaded(ap);
    expect(await tokens(ap)).toContain('Goblin 2');
    expect((await shading(ap)).remembered).toBe(0);

    // He walks to the chest chamber: the corridor goes out of sight, and the goblin with it.
    await moveTo(mp, table, table.pensantusId, 10, 13);
    await expect.poll(async () => (await shading(ap)).remembered).toBeGreaterThan(0);
    await loaded(ap);
    // The tokens come from their own read, after the shading's: wait for it. A goblin that stayed would fail here too.
    await expect.poll(() => tokens(ap)).not.toContain('Goblin 2');
    expect(await tokens(ap)).toContain('Pensantus (você)');
    await expect(ap.locator('.mr-legend').getByText('Já visto', { exact: true })).toBeVisible();
    // The walls he remembers keep their marks, darkened: they are drawn again over the shading.
    await expect(ap.locator('app-map-layers.fb__mem .sq--wall').first()).toBeAttached();
    await expect(ap.getByTestId('fog-caption')).toContainText('O que você já viu fica escurecido e sem inimigos');

    // The master clears the memory: the remembered part goes back to black.
    const forgot = await callRPC(mp, 'meurpg.maps.v1.MapService/ForgetMapVision', { campaignId: table.campaignId, mapId: table.mapId });
    expect(forgot.ok()).toBeTruthy();
    await expect.poll(async () => (await shading(ap)).remembered).toBe(0);
  });
});

test('quem tem o personagem fora do mapa lê o aviso e o que fazer; a caixa é pequena @MR-036', async ({ browser }) => {
  await atTable(browser, { torenOffMap: true }, 'Fora do mapa', async ({ table, bp }) => {
    await bp.goto(sessionRoute(table.campaignId));
    const notice = bp.getByTestId('fog-off-map');
    await expect(notice).toBeVisible();
    await expect(notice).toContainText('Toren fora do mapa');
    await expect(notice).toContainText('Seu personagem não está neste mapa.');
    await expect(notice).toHaveAttribute('role', 'status');
    await expect(bp.getByTestId('fog-nothing')).toContainText('Nada novo para ver');
    // Nothing was ever seen: a small box, not a black map.
    await expect(bp.locator('.fm__none')).toBeVisible();
    await expect(bp.locator('app-fog-base')).toHaveCount(0);
  });
});

test('Toren acende a tocha e passa a ver mais; a confirmação não empurra o mapa @MR-036 @RN-10', async ({ browser }) => {
  await atTable(browser, {}, 'Tocha', async ({ table, bp }) => {
    await bp.goto(sessionRoute(table.campaignId));
    await loaded(bp);
    const before = await seen(bp);
    const row = bp.getByRole('button', { name: /Luz que você carrega/ });
    await expect(row).toContainText('Nenhuma');
    // Where the map sits in the page (not in the window: closing the sheet gives the focus back to the row, which may scroll).
    const mapTop = () => bp.locator('app-fog-base').first().evaluate((el) => Math.round(el.getBoundingClientRect().top + window.scrollY));
    const before_ = await mapTop();

    await row.click();
    const sheet = bp.getByRole('dialog', { name: 'Luz que você carrega' });
    await expect(sheet).toBeVisible();
    await expect(sheet.getByRole('radio')).toHaveCount(4);
    await sheet.getByText('Tocha', { exact: true }).click();
    await sheet.getByRole('button', { name: 'Pronto' }).click();
    await expect(sheet).toHaveCount(0);
    // The page says what it did, for six seconds, over the page: the map stays where it was.
    const toast = bp.getByRole('status').filter({ hasText: 'Você acendeu a tocha: 6 m claro + 6 m de penumbra.' });
    await expect(toast).toBeVisible();
    expect(await mapTop()).toBe(before_);

    await expect(row).toContainText('Tocha');
    await expect.poll(() => seen(bp)).toBeGreaterThan(before);
    await expect(bp.getByTestId('fog-caption')).toContainText('Luz que você carrega: tocha.');
    await expect(toast).toHaveCount(0, { timeout: 9000 });
  });
});

test('o mestre vê o mapa como o Toren vê, ao lado do mapa, e volta com "Todos" ou com "Voltar ao seu mapa" @MR-036 @RN-10', async ({ browser }) => {
  await atTable(browser, {}, 'Ver como', async ({ table, mp, bp }) => {
    const asked = watch(mp);
    await mp.goto(sessionRoute(table.campaignId));
    // His own map is the same component as a player's, with the whole image and every square seen.
    await expect(mp.locator('app-fog-map')).toHaveCount(1);
    await loaded(mp);
    expect((await shading(mp)).unseen).toBe(0);
    expect(asked.some(wholeImage)).toBe(true);
    // "Ver como" is in the tab "Névoa e luz" under the map, with the lights.
    await showMapTool(mp, 'Névoa e luz');
    const list = mp.getByRole('radiogroup', { name: 'Ver como' });
    await expect(mp.locator('#map-tool-panel-nevoa')).toContainText('Luz dos personagens');
    await expect(list.getByRole('radio')).toHaveCount(3);
    await expect(list.getByRole('radio', { name: /Todos/ })).toHaveAttribute('aria-checked', 'true');
    await expect(list.getByRole('radio', { name: /Pensantus/ })).toContainText(/\d+\s+quadrados vistos/);
    // No "(Jogador sem nome)" in the list.
    await expect(list).not.toContainText('Jogador sem nome');

    // Keyboard: the arrows move, Enter chooses.
    await list.getByRole('radio', { name: /Todos/ }).focus();
    await mp.keyboard.press('ArrowDown');
    await mp.keyboard.press('ArrowDown');
    await expect(list.getByRole('radio', { name: /Toren/ })).toBeFocused();
    await expect(list.getByRole('radio', { name: /Toren/ })).toHaveAttribute('aria-checked', 'false');
    await mp.keyboard.press('Enter');
    await expect(list.getByRole('radio', { name: /Toren/ })).toHaveAttribute('aria-checked', 'true');

    await expect(mp.getByText('Você está vendo o mapa como Toren.', { exact: false })).toBeVisible();
    await loaded(mp);
    // Still one map component, now Toren's: his tiles, his tokens, and no "você" anywhere.
    await expect(mp.locator('app-fog-map')).toHaveCount(1);
    await bp.goto(sessionRoute(table.campaignId));
    await loaded(bp);
    expect(await shading(mp)).toEqual(await shading(bp));
    expect((await tokens(mp)).map((n) => n.replace(' (você)', '')).sort()).toEqual((await tokens(bp)).map((n) => n.replace(' (você)', '')).sort());
    expect((await tokens(mp)).join()).not.toContain('(você)');
    expect(asked.some((u) => /\/tiles\/\d+\/\d+$/.test(u))).toBe(true);

    await mp.getByRole('button', { name: 'Voltar ao seu mapa' }).click();
    await expect(list.getByRole('radio', { name: /Todos/ })).toHaveAttribute('aria-checked', 'true');
    await list.getByRole('radio', { name: /Toren/ }).click();
    await list.getByRole('radio', { name: /Todos/ }).click();
    await expect(mp.getByText('Você está vendo o mapa como Toren.', { exact: false })).toHaveCount(0);
  });
});

test('"Ver como" um personagem que morreu volta para "Todos" e diz por quê @MR-036', async ({ browser }) => {
  await atTable(browser, {}, 'Ver como morto', async ({ table, mp }) => {
    await mp.goto(sessionRoute(table.campaignId));
    await showMapTool(mp, 'Névoa e luz');
    const list = mp.getByRole('radiogroup', { name: 'Ver como' });
    await list.getByRole('radio', { name: /Toren/ }).click();
    await expect(mp.getByText('Você está vendo o mapa como Toren.', { exact: false })).toBeVisible();
    const dead = await callRPC(mp, 'meurpg.characters.v1.CharacterService/MarkCharacterDead', { campaignId: table.campaignId, characterId: table.torenId });
    expect(dead.ok(), await dead.text()).toBeTruthy();
    // The master's stream says nothing of it: the next move of anything makes the page read again.
    await moveTo(mp, table, table.goblin1Id, 18, 6);
    await expect(mp.getByText('Esse personagem morreu ou saiu da campanha.', { exact: false })).toBeVisible();
    await expect(list.getByRole('radio', { name: /Todos/ })).toHaveAttribute('aria-checked', 'true');
  });
});

test('Pensantus vê pelos olhos do Nanquim pela tela, fora do combate, e volta aos seus olhos @MR-036', async ({ browser }) => {
  await atTable(browser, { familiar: { col: 15, row: 8 } }, 'Olhos do Nanquim', async ({ table, ap, bp }) => {
    await ap.goto(sessionRoute(table.campaignId));
    await loaded(ap);
    const own = await seen(ap);
    // The familiar's row under the legend: name, "Corvo · familiar de Pensantus" and the button; no distance.
    const row = ap.getByRole('region', { name: 'Seu familiar Nanquim' });
    await expect(row).toContainText('Corvo · familiar de Pensantus');
    await row.getByRole('button', { name: 'Ver pelos olhos' }).click();
    const question = ap.getByRole('dialog', { name: 'Ver pelos olhos do Nanquim?' });
    await expect(question).toContainText('Pensantus fica cego e surdo');
    await expect(question).toContainText('Dura até você voltar aos seus olhos.');
    await question.getByRole('button', { name: 'Ver pelos olhos' }).click();

    const band = ap.getByTestId('familiar-band');
    await expect(band).toBeVisible();
    await expect(band).toContainText('Você está vendo pelos olhos do Nanquim.');
    await expect(band).toContainText('Pensantus está cego e surdo.');
    await expect(band).toHaveAttribute('role', 'status');
    // The row goes while the band is there; the card is "O que o Nanquim vê"; the others see no band.
    await expect(row).toHaveCount(0);
    await expect(ap.getByTestId('fog-caption')).toContainText('O que o Nanquim vê');
    await expect(bp.getByTestId('familiar-band')).toHaveCount(0);
    await expect.poll(() => seen(ap)).not.toBe(own);

    await ap.getByRole('button', { name: 'Voltar aos seus olhos' }).click();
    await expect(band).toHaveCount(0);
    await expect.poll(() => seen(ap)).toBe(own);
    await expect(row).toBeVisible();
  });
});

test('o servidor recusa ver pelos olhos de um familiar a mais de 30 m, e a tela diz o motivo @MR-036', async ({ browser }) => {
  await atTable(browser, { familiar: { col: 22, row: 8 }, pensantusAt: { col: 0, row: 7 } }, 'Longe demais', async ({ table, ap }) => {
    await ap.goto(sessionRoute(table.campaignId));
    await loaded(ap);
    // The button is there (the browser never measures); the refusal comes from the server, by its reason.
    await ap.getByRole('region', { name: 'Seu familiar Nanquim' }).getByRole('button', { name: 'Ver pelos olhos' }).click();
    const question = ap.getByRole('dialog', { name: 'Ver pelos olhos do Nanquim?' });
    await question.getByRole('button', { name: 'Ver pelos olhos' }).click();
    await expect(question.getByRole('alert')).toContainText('Nanquim está a mais de 30 m de você');
    await expect(ap.getByTestId('familiar-band')).toHaveCount(0);
    await question.getByRole('button', { name: 'Cancelar' }).click();
    await expect(question).toHaveCount(0);
  });
});

test('na ficha, o cartão do familiar também tem "Ver pelos olhos" durante a sessão @MR-036 @MR-037', async ({ browser }) => {
  await atTable(browser, { familiar: { col: 15, row: 8 } }, 'Ficha do familiar', async ({ table, ap }) => {
    await ap.goto(`/campaigns/${table.campaignId}/characters/${table.pensantusId}`);
    const card = ap.locator('app-creature-card', { hasText: 'Nanquim' });
    await expect(card).toBeVisible();
    await card.getByRole('button', { name: 'Ver pelos olhos' }).click();
    await expect(ap.getByRole('dialog', { name: 'Ver pelos olhos do Nanquim?' })).toBeVisible();
    await ap.getByRole('dialog', { name: 'Ver pelos olhos do Nanquim?' }).getByRole('button', { name: 'Cancelar' }).click();
  });
});

test('no combate, o mapa tem a mesma névoa e os controles continuam; ver pelos olhos gasta a ação pela tela @MR-036 @RN-20', async ({ browser }) => {
  await atTable(browser, { familiar: { col: 15, row: 8 } }, 'Combate na névoa', async ({ table, mp, ap, bp }) => {
    const fetched = watch(ap);
    await beginFogCombat(mp, table);

    await ap.goto(sessionRoute(table.campaignId));
    await expect(ap.getByRole('heading', { name: /Sua vez, Pensantus/ })).toBeVisible();
    // The combat map is the same fog: the tiles and the shading (the player's map card has no legend of its own).
    await loaded(ap);
    expect((await shading(ap)).grey).toBeGreaterThan(0);
    await expect(ap.getByTestId('familiar-blind')).toHaveCount(0);
    // The light stays in reach during the combat, and so does the master's "Ver como".
    await expect(ap.getByRole('button', { name: /Luz que você carrega/ })).toBeVisible();
    await mp.goto(sessionRoute(table.campaignId));
    await openCombatPanel(mp, 'Névoa e luz');
    await expect(mp.getByRole('radiogroup', { name: 'Ver como' })).toBeVisible();
    await expect(mp.getByRole('heading', { name: 'Luz dos personagens' })).toBeVisible();

    // The action, in "Outras": "Ver pelos olhos do Nanquim", tagged Ação, with what it lasts.
    await openActionTab(ap, 'Outras');
    const action = ap.locator('app-action-row', { hasText: 'Ver pelos olhos do Nanquim' });
    await expect(action).toContainText('Ação');
    await expect(action).toContainText('Dura até o começo da sua próxima vez, ou até você voltar.');
    await action.getByRole('button', { name: 'Ver pelos olhos do Nanquim' }).click();
    const question = ap.getByRole('dialog', { name: 'Ver pelos olhos do Nanquim?' });
    await expect(question).toContainText('gasta a sua ação');
    await question.getByRole('button', { name: 'Ver pelos olhos' }).click();

    const band = ap.getByTestId('familiar-band');
    await expect(band).toContainText('Até o começo da sua próxima vez.');
    await expect(band).not.toContainText('rodada');
    await expect(ap.getByTestId('familiar-blind')).toHaveText(/Cego: para atacar, fale com o mestre\./);
    // The action was spent, and the others see only the conditions, in words.
    const now = await getEncounterRPC(mp, table.campaignId);
    const me = now.combatants.find((c) => c.label === 'Pensantus')!;
    expect(me.actionUsed).toBe(true);
    expect(me.conditions ?? []).toEqual(expect.arrayContaining(['condition:blinded', 'condition:deafened']));
    await bp.goto(sessionRoute(table.campaignId));
    await expect(bp.getByTestId('familiar-band')).toHaveCount(0);
    await expect(ap.getByRole('button', { name: 'Voltar aos seus olhos' })).toBeVisible();

    await ap.getByRole('button', { name: 'Voltar aos seus olhos' }).click();
    await expect(band).toHaveCount(0);
    await expect(ap.getByTestId('familiar-blind')).toHaveCount(0);

    // The "Mover" page draws the same fog, and no player request fetched the whole image, in the combat or on that page.
    await ap.getByRole('button', { name: 'Mover', exact: true }).click();
    await expect(ap.getByRole('heading', { name: 'Mover Pensantus' })).toBeVisible();
    await loaded(ap);
    expect((await shading(ap)).grey).toBeGreaterThan(0);
    expect(fetched.filter(wholeImage)).toEqual([]);
    expect(fetched.some(tile)).toBe(true);
  });
});

test('"Mapas revelados": o jogador abre o mapa com a mesma névoa e nenhum pedido baixa a imagem inteira @MR-036 @RN-10', async ({ browser }) => {
  await atTable(browser, {}, 'Mapas revelados', async ({ table, ap }) => {
    const fetched = watch(ap);
    await ap.goto(`/campaigns/${table.campaignId}/maps/${table.mapId}`);
    await expect(ap.locator('app-fog-map')).toBeVisible();
    await loaded(ap);
    expect((await shading(ap)).grey).toBeGreaterThan(0);
    expect(await tokens(ap)).toContain('Pensantus (você)');
    expect(fetched.filter(wholeImage)).toEqual([]);
    expect(fetched.some(tile)).toBe(true);
  });
});

test('um mapa sem névoa continua como era: a prévia, com a imagem inteira @MR-036', async ({ browser }) => {
  await atTable(browser, { noFog: true }, 'Sem névoa', async ({ table, ap }) => {
    const fetched = watch(ap);
    await ap.goto(sessionRoute(table.campaignId));
    await expect(ap.getByRole('img', { name: /Prévia do mapa A caverna do Vale Seco/ })).toBeVisible();
    await expect(ap.locator('app-fog-map')).toHaveCount(0);
    await expect(ap.getByRole('link', { name: 'Ver mapa' })).toBeVisible();
    expect(fetched.some(wholeImage)).toBe(true);
  });
});
