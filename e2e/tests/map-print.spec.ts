import { expect, test } from '@playwright/test';

import { pdfPages, printRoute, summary, tableForPrinting } from './print-support';
import { afterRender, boxOf, newSignedInContext } from './support';

// MR-033: print the map with its grid, to scale. The master chooses the
// square's size and the paper; the page counts the sheets; the browser's
// print layout has one tile per page and none of the controls. Setup goes
// through the API; the screen is what is under test.

// Each test builds its own campaign, image and map before the screen: on a
// busy machine that alone takes a while.
test.describe.configure({ timeout: 90_000 });

test(
  'o mestre abre a impressão de um mapa com grade, muda para 2 cm e A3 e vê 3 folhas em retrato',
  { tag: '@MR-033' },
  async ({ browser }) => {
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const master = await masterContext.newPage();
      const player = await playerContext.newPage();
      await master.goto('/');
      const table = await tableForPrinting(master, player, `Impressão ${Date.now()}`);

      // The entry is on the map page, beside the title.
      await master.goto(`/campaigns/${table.campaignId}/maps/${table.gridMapId}`);
      await master.getByRole('link', { name: 'Imprimir com a grade' }).click();
      await expect(master).toHaveURL(new RegExp(`/maps/${table.gridMapId}/print$`));
      await expect(master.getByRole('heading', { name: 'Imprimir o mapa', level: 1 })).toBeVisible();

      // The defaults: one inch per square, A4: 30 x 20 x 2,54 cm on 9 sheets.
      const field = master.getByLabel('Tamanho do quadrado');
      await expect(field).toHaveValue('2,54');
      await expect(summary(master)).toContainText('76,2 × 50,8 cm');
      await expect(summary(master)).toContainText('em 9 folhas A4');
      await expect(master.getByText('Paisagem', { exact: true }).first()).toBeVisible();
      await expect(master.getByRole('button', { name: 'Voltar a 2,54 cm' })).toHaveCount(0);

      // 2 cm on A3: 60 x 40 cm, 3 sheets in portrait (4 in landscape).
      await field.fill('2');
      await master.getByRole('radio', { name: /A3/ }).check();
      await expect(field).toHaveValue('2');
      await expect(summary(master)).toContainText('60,0 × 40,0 cm');
      await expect(summary(master)).toContainText('em 3 folhas A3');
      await expect(master.getByText('Retrato', { exact: true }).first()).toBeVisible();
      await expect(master.getByText('Folhas A1 a A3 · 3 no total · retrato')).toBeVisible();

      // "As contas": every paper, both ways, the smaller one marked.
      const row = master.getByRole('row', { name: /^A3/ });
      await expect(row).toContainText('2 × 2 = 4 folhas');
      await expect(row).toContainText('3 × 1 = 3 folhas');
      await expect(row.getByText('A menor')).toHaveCount(1);

      // The way back to the default.
      await master.getByRole('button', { name: 'Voltar a 2,54 cm' }).click();
      await expect(field).toHaveValue('2,54');
      await expect(summary(master)).toContainText('em 4 folhas A3');
    } finally {
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test('uma medida fora de 1 a 10 cm trava o Imprimir e o app mostra o motivo', { tag: '@MR-033' }, async ({ browser }) => {
  const masterContext = await newSignedInContext(browser, 'Mestre Teste');
  const playerContext = await newSignedInContext(browser, 'Jogador Teste');
  try {
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    await master.goto('/');
    const table = await tableForPrinting(master, player, `Impressão medida ${Date.now()}`);
    await master.goto(printRoute(table.campaignId, table.gridMapId));

    const field = master.getByLabel('Tamanho do quadrado');
    const printButton = master.getByRole('button', { name: 'Imprimir', exact: true });
    await expect(printButton).not.toHaveAttribute('aria-disabled', 'true');

    for (const bad of ['0,5', '11', '', 'abc']) {
      await field.fill(bad);
      await expect(master.getByRole('alert').filter({ hasText: 'Use um tamanho de 1 a 10 cm.' })).toBeVisible();
      await expect(printButton).toHaveAttribute('aria-disabled', 'true');
      await expect(master.getByText('A prévia volta quando o tamanho do quadrado for válido.')).toBeVisible();
      await expect(summary(master)).toContainText('— × — cm');
    }

    // A click while it is blocked opens nothing: the page stays as it was.
    let printed = false;
    await master.exposeFunction('markPrinted', () => (printed = true));
    await master.evaluate(() => {
      window.print = () => void (window as unknown as { markPrinted(): void }).markPrinted();
    });
    await printButton.click({ force: true });
    await afterRender(master);
    expect(printed).toBe(false);

    // A comma or a dot, shown as typed; "Imprimir" works again and prints.
    await field.fill('2.5');
    await expect(field).toHaveValue('2.5');
    await expect(printButton).not.toHaveAttribute('aria-disabled', 'true');
    await printButton.click();
    await expect.poll(() => printed).toBe(true);
  } finally {
    await masterContext.close();
    await playerContext.close();
  }
});

test('um mapa sem grade não imprime: o botão diz por quê e a tela de impressão também', { tag: '@MR-033' }, async ({ browser }) => {
  const masterContext = await newSignedInContext(browser, 'Mestre Teste');
  const playerContext = await newSignedInContext(browser, 'Jogador Teste');
  try {
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    await master.goto('/');
    const table = await tableForPrinting(master, player, `Impressão sem grade ${Date.now()}`);

    await master.goto(`/campaigns/${table.campaignId}/maps/${table.plainMapId}`);
    await expect(master.getByRole('heading', { name: 'Sem grade', level: 1 })).toBeVisible();
    const entry = master.getByRole('button', { name: 'Imprimir com a grade' });
    await expect(entry).toHaveAttribute('aria-disabled', 'true');
    await expect(master.getByText('Defina a grade do mapa para imprimir em escala')).toBeVisible();
    await expect(master.getByRole('link', { name: 'Imprimir com a grade' })).toHaveCount(0);

    // The route itself, opened by hand, says the same and offers the grid.
    await master.goto(printRoute(table.campaignId, table.plainMapId));
    await expect(master.getByText('Este mapa ainda não tem grade.')).toBeVisible();
    await expect(master.getByRole('link', { name: 'Definir a grade' })).toBeVisible();
    await expect(master.getByRole('button', { name: 'Imprimir', exact: true })).toHaveCount(0);
  } finally {
    await masterContext.close();
    await playerContext.close();
  }
});

test('o jogador não vê o botão de imprimir e a tela de impressão responde que é só do mestre', { tag: '@MR-033' }, async ({ browser }) => {
  const masterContext = await newSignedInContext(browser, 'Mestre Teste');
  const playerContext = await newSignedInContext(browser, 'Jogador Teste');
  try {
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    await master.goto('/');
    const table = await tableForPrinting(master, player, `Impressão jogador ${Date.now()}`);

    // A map the master has not shown: the player gets the same "not found"
    // as the map page itself gives.
    await player.goto(printRoute(table.campaignId, table.gridMapId));
    await expect(player.getByText('Esse mapa não existe.')).toBeVisible();
    await expect(player.getByLabel('Tamanho do quadrado')).toHaveCount(0);

    // Shown to the players: the route is still the master's.
    const shown = await master.request.post('/meurpg.maps.v1.MapService/SetMapRevealed', {
      headers: { 'Connect-Protocol-Version': '1' },
      data: { campaignId: table.campaignId, mapId: table.gridMapId, revealed: true },
    });
    expect(shown.ok()).toBeTruthy();
    await player.goto(printRoute(table.campaignId, table.gridMapId));
    await expect(player.getByText('Só o mestre imprime o mapa.')).toBeVisible();
    await expect(player.getByLabel('Tamanho do quadrado')).toHaveCount(0);
    await expect(player.getByRole('button', { name: 'Imprimir', exact: true })).toHaveCount(0);

    // The player's viewer has no print entry either.
    await player.goto(`/campaigns/${table.campaignId}/maps/${table.gridMapId}`);
    await expect(player.getByRole('heading', { name: 'Estrada do Vale', level: 1 })).toBeVisible();
    await expect(player.getByText(/Imprimir/)).toHaveCount(0);
  } finally {
    await masterContext.close();
    await playerContext.close();
  }
});

test('no papel sai uma folha por página, com a grade em escala, e nenhum controle', { tag: '@MR-033' }, async ({ browser }) => {
  const masterContext = await newSignedInContext(browser, 'Mestre Teste');
  const playerContext = await newSignedInContext(browser, 'Jogador Teste');
  try {
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    await master.goto('/');
    const table = await tableForPrinting(master, player, `Impressão papel ${Date.now()}`);
    await master.goto(printRoute(table.campaignId, table.gridMapId));
    await expect(summary(master)).toContainText('em 9 folhas A4');

    // The sheets are not in the page until the browser is about to print
    // (a big map is thousands of them); `beforeprint` is what it fires.
    const sheets = master.locator('app-print-sheets .sheet');
    await expect(sheets).toHaveCount(0);
    await master.evaluate(() => window.dispatchEvent(new Event('beforeprint')));
    await expect(sheets).toHaveCount(9);
    // Hidden on screen, shown on paper.
    await expect(sheets.first()).toBeHidden();
    await master.emulateMedia({ media: 'print' });
    await expect(sheets.first()).toBeVisible();
    await expect(master.getByLabel('Tamanho do quadrado')).toBeHidden();
    await expect(master.getByRole('button', { name: 'Imprimir', exact: true })).toBeHidden();
    await expect(master.getByRole('banner')).toBeHidden();

    // A4 in landscape, 1 cm margins: each sheet prints 27,7 x 19,0 cm.
    const cm = 96 / 2.54;
    const box = await boxOf(sheets.nth(4));
    expect(box.width / cm).toBeCloseTo(27.7, 1);
    expect(box.height / cm).toBeCloseTo(19, 1);
    await expect(sheets.nth(4).locator('.label')).toHaveText('Página B2 · cole à direita da B1 e abaixo da A2');
    await expect(sheets.nth(0).locator('.label')).toContainText('Página A1');
    await expect(sheets.nth(4).getByText('5 cm · confira a escala')).toBeVisible();
    // The ruler is exactly 5 cm; the grid is drawn at 2,54 cm (a line each).
    const ruler = await boxOf(sheets.nth(4).locator('.ruler__bar'));
    expect(ruler.width / cm).toBeCloseTo(5, 1);
    const image = await boxOf(sheets.nth(0).locator('img'));
    expect(image.width / cm).toBeCloseTo(76.2, 1);
    expect(image.height / cm).toBeCloseTo(50.8, 1);
    // The image is the master's, one URL for every sheet; no points or tokens.
    const sources = await sheets.locator('img').evaluateAll((els) => els.map((e) => (e as HTMLImageElement).getAttribute('src')));
    expect(new Set(sources).size).toBe(1);
    await expect(master.locator('app-print-sheets').getByRole('button')).toHaveCount(0);

    // And what the browser makes of it: one page per sheet, in the paper's size.
    const pdf = await master.pdf({ preferCSSPageSize: true });
    expect(pdfPages(pdf)).toBe(9);
    expect(pdf.toString('latin1')).toMatch(/MediaBox \[0 0 841\.\d+ 59[45]\.\d+\]/);
    await master.evaluate(() => window.dispatchEvent(new Event('afterprint')));
    await expect(sheets).toHaveCount(0);
  } finally {
    await masterContext.close();
    await playerContext.close();
  }
});
