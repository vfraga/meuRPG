import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { openSessionPage } from './live-session-support';
import {
  createLightsRPC,
  createLockRPC,
  createPillarsRPC,
  DOOR_OPEN,
  doorState,
  endTable,
  masterRunJson,
  moveRPC,
  pickPuzzleAction,
  playerRunJson,
  playerRunText,
  puzzleRoute,
  showPuzzleRPC,
  tableForPuzzles,
} from './puzzles-support';
import { gotoCampaignTab, newSignedInContext, openCampaignTab } from './support';

// Puzzles on screen (Etapa 10, slice 10.15a: MR-038, RN-27, RN-10; E10-06): "Apagar as luzes", the combination lock and the turning
// symbols, for the master (the list, the form, the live view, "Recomeçar" and "Fechar" asked in place) and for the players (the boards).
// The puzzles come through the API where the screen is not what is under test; the tests read what a player's response carries, as the
// app reads it, to prove the answer never leaves the server.

test.describe.configure({ timeout: 240_000 });

/** What a player's response must never carry (RN-10, RN-27): the answer, the fewest moves, the way to make them, "Ao resolver". */
const NEVER_IN_A_PLAYER_RESPONSE = ['"solution"', '"minimum"', '"path"', '"onSolve"', '"on_solve"', '"hintCheck"'];

async function twoPeople(browser: import('@playwright/test').Browser, viewport = { width: 1280, height: 1000 }, colorScheme: 'light' | 'dark' = 'light') {
  const masterContext = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 1000 }, colorScheme });
  const playerContext = await newSignedInContext(browser, 'Jogador Teste', { viewport, colorScheme });
  const master = await masterContext.newPage();
  const player = await playerContext.newPage();
  await Promise.all([master.goto('/'), player.goto('/')]);
  return { master, player, close: () => Promise.all([masterContext.close(), playerContext.close()]) };
}

test('o mestre faz uma fechadura que abre uma porta e a mostra; o jogador a resolve, a porta abre no mapa e ele lê a mensagem @MR-038 @RN-27 @RN-10', async ({ browser }) => {
  const { master, player, close } = await twoPeople(browser);
  const table = await tableForPuzzles(master, player, `Fechadura ${Date.now()}`);
  try {
    // The master makes the lock on the form: the kind, the name, the solution (a wheel turned once), "Ao resolver" with a door.
    await master.goto(puzzleRoute(table.campaignId, 'puzzles', 'new'));
    await expect(master.getByRole('heading', { level: 1, name: 'Novo quebra-cabeça' })).toBeVisible();
    await master.getByRole('radio', { name: /^Fechadura de combinação/ }).check();
    await master.getByLabel('Nome', { exact: true }).fill('O cofre do Refeitório');
    await master.getByRole('group', { name: 'Solução da fechadura' }).getByRole('button', { name: 'Próximo símbolo: Roda 1' }).click();
    await master.getByLabel('Pista para os jogadores').fill('O fogo nasce antes da lua.');
    await master.getByRole('button', { name: 'Adicionar uma dica' }).click();
    await master.getByRole('textbox', { name: 'Dica 1' }).fill('A pista fala de três coisas da natureza.');
    await master.getByRole('radio', { name: 'Abrir uma porta' }).check();
    await master.getByLabel('Mapa', { exact: true }).selectOption({ label: 'A capela' });
    await master.getByLabel('Qual porta').selectOption({ label: 'Fechada · (11, 7)' });
    await master.getByLabel('Mensagem para os jogadores (opcional)').fill('A porta da Capela se abriu.');
    await master.getByRole('button', { name: 'Criar quebra-cabeça' }).click();
    await expect(master).toHaveURL(puzzleRoute(table.campaignId));
    await openCampaignTab(master, 'Preparação');
    await expect(master.getByRole('region', { name: 'Quebra-cabeças' }).getByText('O cofre do Refeitório')).toBeVisible();

    // The session: "Mostrar aos jogadores" and the live card.
    await openSessionPage(master, table.campaignId);
    await master.getByRole('button', { name: 'Mostrar aos jogadores O cofre do Refeitório' }).click();
    const card = master.getByRole('article', { name: 'O cofre do Refeitório' });
    await expect(card.getByText('Os jogadores veem')).toBeVisible();
    await expect(card.getByText('Dicas: 0 de 1 solta')).toBeVisible();

    // The player gets the notice and opens the puzzle.
    await openSessionPage(player, table.campaignId);
    await expect(player.getByText('O mestre mostrou um quebra-cabeça')).toBeVisible({ timeout: 30_000 });
    await player.getByRole('link', { name: 'Abrir o quebra-cabeça' }).click();
    await expect(player.getByRole('heading', { level: 1, name: 'O cofre do Refeitório' })).toBeVisible();
    await expect(player.getByText('“O fogo nasce antes da lua.”')).toBeVisible();

    // What the player's response carries (RN-10): the state and the clue, never the solution, the minimum or "Ao resolver".
    const url = new URL(player.url());
    const puzzleId = url.searchParams.get('puzzle')!;
    const text = await playerRunText(player, table.campaignId, puzzleId);
    for (const secret of NEVER_IN_A_PLAYER_RESPONSE) {
      expect(text, secret).not.toContain(secret);
    }
    expect(text).not.toContain(table.map.mapId);
    expect(text).not.toContain('A porta da Capela se abriu.');
    const run = (await playerRunJson(player, table.campaignId, puzzleId)).run as { state: { lock: { wheels: number[] } }; solved?: boolean };
    expect(run.state.lock.wheels).toEqual([0, 0, 0, 1]);

    // Two turns of the wheels solve it: the first wheel up, the last one back.
    await player.getByRole('button', { name: 'Próximo símbolo: Roda 1' }).click();
    await expect(player.getByRole('group', { name: 'Roda 1: Chama' })).toBeVisible();
    await expect(player.locator('.info__last')).toContainText('Você girou a 1ª roda');
    await expect(player.getByText('Resolvido', { exact: true })).toHaveCount(0);
    await player.getByRole('button', { name: 'Símbolo anterior: Roda 4' }).click();
    await expect(player.getByText('Resolvido', { exact: true })).toBeVisible();
    await expect(player.getByText('A porta da Capela se abriu.')).toBeVisible();
    await expect(player.getByText('O quebra-cabeça terminou. A fechadura abriu.')).toBeVisible();
    // Frozen: the arrows say so and do nothing.
    await expect(player.getByRole('button', { name: 'Próximo símbolo: Roda 2' })).toHaveAttribute('aria-disabled', 'true');

    // The door opened on the map, for the master and for the player; the master reads it on his card.
    await expect.poll(() => doorState(master, table.campaignId, table.map.mapId, table.door.col, table.door.row)).toBe(DOOR_OPEN);
    await expect.poll(() => doorState(player, table.campaignId, table.map.mapId, table.door.col, table.door.row)).toBe(DOOR_OPEN);
    await expect(card.getByText(/Jogador Teste|Pensantus/).first()).toBeVisible();
    await expect(card.getByText('Pensantus resolveu “O cofre do Refeitório”', { exact: false })).toBeVisible();
    await expect(card.getByText('Uma porta se abriu.')).toBeVisible();
    await expect(card.getByRole('img', { name: 'A porta aberta no mapa A capela, coluna 11, linha 7' })).toBeVisible();

    // The players' JSON now carries what happened, in the master's words, and still no answer.
    const after = await playerRunText(player, table.campaignId, puzzleId);
    expect(after).toContain('A porta da Capela se abriu.');
    for (const secret of NEVER_IN_A_PLAYER_RESPONSE) {
      expect(after, secret).not.toContain(secret);
    }
  } finally {
    await endTable(master, table.campaignId);
    await close();
  }
});

test('o jogador toca uma luz e o painel de 7 × 7 cabe em 320 × 568 sem rolar de lado @MR-038 @RN-10', async ({ browser }) => {
  const { master, player, close } = await twoPeople(browser, { width: 320, height: 568 }, 'dark');
  const table = await tableForPuzzles(master, player, `Luzes ${Date.now()}`);
  try {
    const puzzleId = await createLightsRPC(master, table.campaignId, 'Os candelabros da cripta', 7, { clue: 'Os candelabros guardam a cripta.' });
    await showPuzzleRPC(master, table.campaignId, puzzleId);
    await player.goto(`/campaigns/${table.campaignId}/session?puzzle=${puzzleId}`);
    await expect(player.getByRole('heading', { level: 1, name: 'Os candelabros da cripta' })).toBeVisible({ timeout: 30_000 });
    const lights = player.getByRole('button', { name: /^Luz na linha \d, coluna \d, (acesa|apagada)$/ });
    await expect(lights).toHaveCount(49);

    // Fits: no sideways scroll, and every light is at least 44 px (the lights touch and the board bleeds 6 px from the edge at 320).
    const fit = await player.evaluate(() => {
      const buttons = Array.from(document.querySelectorAll<HTMLElement>('app-lights-board button'));
      const boxes = buttons.map((b) => b.getBoundingClientRect());
      return {
        scrolls: document.documentElement.scrollWidth > document.documentElement.clientWidth,
        smallest: Math.min(...boxes.map((r) => Math.min(r.width, r.height))),
        left: Math.min(...boxes.map((r) => r.left)),
        right: Math.max(...boxes.map((r) => r.right)),
        bottom: Math.max(...boxes.map((r) => r.bottom)),
        width: document.documentElement.clientWidth,
        height: window.innerHeight,
      };
    });
    expect(fit.scrolls).toBe(false);
    expect(fit.smallest).toBeGreaterThanOrEqual(44);
    expect(fit.left).toBeGreaterThanOrEqual(0);
    expect(fit.right).toBeLessThanOrEqual(fit.width);
    // The whole board is on the first screen (E10-06 state 9): the lead, the status line and the clue make room for it.
    expect(fit.bottom).toBeLessThanOrEqual(fit.height);
    await expect(player.getByText(/^Luzes acesas:/)).toBeInViewport();

    // A tap goes to the server and the light follows its answer: the pressed light turns over.
    const first = lights.first();
    const before = await first.getAttribute('aria-label');
    await first.click();
    await expect(first).not.toHaveAttribute('aria-label', before!);
    // The server kept it; a read as the app reads it agrees.
    const state = ((await playerRunJson(player, table.campaignId, puzzleId)).run as { state: { lights: { lit?: boolean[] } } }).state.lights.lit ?? [];
    expect(state.length).toBe(49);
    expect(before).toContain(state[0] ? 'apagada' : 'acesa');
  } finally {
    await endTable(master, table.campaignId);
    await close();
  }
});

test('o mestre faz "Apagar as luzes" no formulário, edita, e arquiva e desarquiva na lista @MR-038 @RN-27', async ({ browser }) => {
  const { master, player, close } = await twoPeople(browser);
  const table = await tableForPuzzles(master, player, `Lista ${Date.now()}`);
  try {
    await master.goto(puzzleRoute(table.campaignId, 'puzzles', 'new'));
    await master.getByLabel('Nome', { exact: true }).fill('O selo da Capela');
    // The start is the server's: the count of lights and the fewest touches arrive before the puzzle can be saved.
    await expect(master.getByText(/\d+ acesas?, \d+ apagadas?\./)).toBeVisible({ timeout: 30_000 });
    await expect(master.getByText(/Dá para resolver em \d+ toques?\./)).toBeVisible();
    await expect(master.locator('app-lights-board button')).toHaveCount(0);
    await master.getByRole('button', { name: 'Criar quebra-cabeça' }).click();
    await expect(master).toHaveURL(puzzleRoute(table.campaignId));
    await openCampaignTab(master, 'Preparação');
    const panel = master.getByRole('region', { name: 'Quebra-cabeças' });
    await expect(panel.getByText('O selo da Capela')).toBeVisible();
    await expect(panel.getByText('Apagar as luzes · 5 × 5')).toBeVisible();
    await expect(panel.getByText('Não mostrado')).toBeVisible();

    // Edit before it is shown.
    // The row is the link to the editor ("Editar" is not a button of the row: the board's row has a chevron).
    await panel.getByRole('link', { name: /^O selo da Capela/ }).click();
    await expect(master.getByRole('heading', { level: 1, name: 'Editar quebra-cabeça' })).toBeVisible();
    await expect(master.getByLabel('Nome', { exact: true })).toHaveValue('O selo da Capela');
    await master.getByLabel('Nome', { exact: true }).fill('O selo da Cripta');
    await master.getByRole('button', { name: 'Salvar quebra-cabeça' }).click();
    // Saving goes back to the campaign page ("Mesa"); the list is on "Preparação".
    await expect(master).toHaveURL(puzzleRoute(table.campaignId));
    await openCampaignTab(master, 'Preparação');
    await expect(panel.getByText('O selo da Cripta')).toBeVisible();

    // Archive asks in place, and nothing is archived before the second tap.
    await pickPuzzleAction(master, 'O selo da Cripta', 'Arquivar');
    await expect(panel.getByRole('heading', { name: 'Arquivar “O selo da Cripta”?' })).toBeFocused();
    await panel.getByRole('button', { name: 'Voltar' }).click();
    await expect(panel.getByRole('button', { name: 'Mais ações de O selo da Cripta' })).toBeFocused();
    await pickPuzzleAction(master, 'O selo da Cripta', 'Arquivar');
    await panel.getByRole('button', { name: 'Arquivar', exact: true }).click();
    await expect(panel.getByText('“O selo da Cripta” foi arquivado.')).toBeVisible();
    await expect(panel.getByText('Todos os quebra-cabeças estão arquivados.')).toBeVisible();
    await panel.getByRole('button', { name: 'Mostrar os arquivados' }).click();
    await pickPuzzleAction(master, 'O selo da Cripta', 'Desarquivar');
    await expect(panel.getByText('“O selo da Cripta” voltou para a lista.')).toBeVisible();

    // Once shown, it cannot be edited: the list has no "Editar" and the page says why.
    const id = (await (await master.request.post('/meurpg.play.v1.PuzzleService/ListPuzzles', { data: { campaignId: table.campaignId }, headers: { 'Connect-Protocol-Version': '1' } })).json()).puzzles[0].id as string;
    await showPuzzleRPC(master, table.campaignId, id);
    await gotoCampaignTab(master, table.campaignId, 'Preparação');
    await expect(master.getByRole('region', { name: 'Quebra-cabeças' }).getByText('Já mostrado')).toBeVisible();
    await expect(master.getByRole('link', { name: 'Editar O selo da Cripta' })).toHaveCount(0);
    await master.goto(puzzleRoute(table.campaignId, 'puzzles', id, 'edit'));
    await expect(master.getByText('já foi mostrado numa sessão e não pode mais ser editado')).toBeVisible();
  } finally {
    await endTable(master, table.campaignId);
    await close();
  }
});

test('os pilares: o jogador lê o mural e a regra, gira, e o mestre vê quem girou @MR-038 @RN-27 @RN-10', async ({ browser }) => {
  const { master, player, close } = await twoPeople(browser);
  const table = await tableForPuzzles(master, player, `Pilares ${Date.now()}`);
  try {
    const puzzleId = await createPillarsRPC(master, table.campaignId, 'Os pilares da Galeria', { clue: 'Os pilares obedecem ao mural.' });
    await openSessionPage(master, table.campaignId);
    await openSessionPage(player, table.campaignId);
    await master.getByRole('button', { name: 'Mostrar aos jogadores Os pilares da Galeria' }).click();
    const card = master.getByRole('article', { name: 'Os pilares da Galeria' });
    await expect(card.getByText('Os pilares agora')).toBeVisible();
    await expect(card.getByText('O mural', { exact: true })).toBeVisible();
    await expect(card.getByText(/Faltam, no mínimo, \d+ giros?/)).toBeVisible();

    await player.getByRole('link', { name: 'Abrir o quebra-cabeça' }).click();
    await expect(player.getByRole('heading', { level: 1, name: 'Os pilares da Galeria' })).toBeVisible();
    // The mural and the rule of the links are the players' (the artboard's state 7); the solution of a lock never would be.
    await expect(player.getByRole('group', { name: 'O mural' })).toBeVisible();
    await expect(player.getByText('Girar um pilar gira também o da esquerda e o da direita.')).toBeVisible();
    const text = await playerRunText(player, table.campaignId, puzzleId);
    expect(text).toContain('"mural"');
    for (const secret of NEVER_IN_A_PLAYER_RESPONSE) {
      expect(text, secret).not.toContain(secret);
    }

    const pillar1 = player.getByRole('group', { name: 'Pilares', exact: true }).getByRole('group', { name: /^Pilar 1:/ });
    const before = await pillar1.getAttribute('aria-label');
    await player.getByRole('button', { name: 'Girar o pilar 1' }).click();
    await expect(pillar1).not.toHaveAttribute('aria-label', before!);
    await expect(player.locator('.info__last')).toContainText('Você girou o pilar 1');
    await expect(card.getByText('Última jogada: Pensantus girou o pilar 1')).toBeVisible();
    // Another move through the API, as the same player: the page follows the stream and reads the run again.
    await moveRPC(player, table.campaignId, puzzleId, { pillars: { pillar: 3, delta: 1 } });
    await expect(player.locator('.info__last')).toContainText('Você girou o pilar 4');
  } finally {
    await endTable(master, table.campaignId);
    await close();
  }
});

test('"Recomeçar" e "Fechar" perguntam na própria tela, e fechar tira o quebra-cabeça dos jogadores @MR-038 @RN-27', async ({ browser }) => {
  const { master, player, close } = await twoPeople(browser);
  const table = await tableForPuzzles(master, player, `Perguntas ${Date.now()}`);
  try {
    const puzzleId = await createLightsRPC(master, table.campaignId, 'O selo da Capela', 5);
    await showPuzzleRPC(master, table.campaignId, puzzleId);
    await openSessionPage(player, table.campaignId);
    await expect(player.getByText('O mestre mostrou um quebra-cabeça')).toBeVisible({ timeout: 30_000 });
    await openSessionPage(master, table.campaignId);
    const card = master.getByRole('article', { name: 'O selo da Capela' });
    await expect(card.getByText('Os jogadores veem')).toBeVisible();

    // The question takes the buttons' place, with the focus on its title; "Voltar" brings the buttons back and the focus to the one that asked.
    await card.getByRole('button', { name: 'Recomeçar' }).click();
    await expect(card.getByRole('heading', { name: 'Recomeçar “O selo da Capela”?' })).toBeFocused();
    await expect(card.getByText('Quem estiver jogando vê o começo de novo na hora.')).toBeVisible();
    await expect(card.getByRole('button', { name: 'Gerar outro começo' })).toHaveCount(0);
    await card.getByRole('button', { name: 'Voltar' }).click();
    await expect(card.getByRole('button', { name: 'Recomeçar' })).toBeFocused();

    // Recomeçar, confirmed: the board goes back to the start it had, for everyone, and the last move goes.
    const lit = async () => JSON.stringify(((await playerRunJson(player, table.campaignId, puzzleId)).run as { state: { lights: { lit?: boolean[] } } }).state.lights.lit);
    const start = await lit();
    await moveRPC(player, table.campaignId, puzzleId, { lights: { row: 2, col: 2 } });
    await expect.poll(lit).not.toBe(start);
    await card.getByRole('button', { name: 'Recomeçar' }).click();
    await card.locator('app-map-ask').getByRole('button', { name: 'Recomeçar', exact: true }).click();
    await expect.poll(lit).toBe(start);
    await expect(card.getByRole('button', { name: 'Recomeçar' })).toBeFocused();

    // Fechar: the second tap closes it, the players lose it, and it is a row that can be shown again.
    await card.getByRole('button', { name: 'Fechar' }).click();
    await expect(card.getByRole('heading', { name: 'Fechar “O selo da Capela”?' })).toBeFocused();
    await card.getByRole('button', { name: 'Fechar', exact: true }).last().click();
    await expect(master.getByRole('article', { name: 'O selo da Capela' })).toHaveCount(0);
    await expect(master.getByRole('button', { name: 'Mostrar de novo O selo da Capela' })).toBeVisible();
    await expect(player.getByText('O mestre mostrou um quebra-cabeça')).toHaveCount(0, { timeout: 30_000 });
    // A closed puzzle is "not found" to a player, as one that does not exist.
    const read = await player.request.post('/meurpg.play.v1.PuzzleService/GetPuzzleRun', { data: { campaignId: table.campaignId, puzzleId }, headers: { 'Connect-Protocol-Version': '1' } });
    expect(read.status()).toBe(404);

    // Shown again, it opens at its start.
    await master.getByRole('button', { name: 'Mostrar de novo O selo da Capela' }).click();
    await expect(master.getByRole('article', { name: 'O selo da Capela' }).getByText('Os jogadores veem')).toBeVisible();
    const run = (await masterRunJson(master, table.campaignId, puzzleId)).run as { status: string };
    expect(run.status).toBe('PUZZLE_RUN_STATUS_SHOWN');
  } finally {
    await endTable(master, table.campaignId);
    await close();
  }
});
