import { expect, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { getEncounterRPC } from './combat-support';
import { addEffectRPC, effectRow, effectsTable, effectsRPC, exhaustionOf, openPanel, setExhaustionRPC } from './effects-support';
import { endOpenSessionRPC, openSessionPage, startSessionRPC, tableWithPensantus } from './live-session-support';
import { newSignedInContext, boxOf } from './support';

// The master's side of the effects that last (W7-E, RN-22, RN-10): the panel "Efeitos em jogo" with the clock of the
// turns, the dialogs to add an effect, change how long it lasts and what the players see, "Encerrar" with the question
// for a concentration, the exhaustion controls, the long rest's "Sem comida ou bebida" and "Passar o tempo". The table and
// the effects come through the API; what is under test is what the master reads and does on the screen.

/** The dialog or the bottom sheet that opened with this title. */
const sheet = (page: Page, name: string | RegExp) => page.getByRole('dialog', { name });

test('o painel lista os efeitos e o relógio dos turnos, e a tabela é uma lista de linhas com "Encerrar" dentro', { tag: ['@W7-E', '@RN-22', '@RN-10'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const t = await effectsTable(browser, 'Painel de efeitos');
  try {
    const panel = await openPanel(t);
    await expect(panel.getByRole('heading', { level: 2, name: 'Efeitos em jogo' })).toBeVisible();
    await expect(panel).toContainText(/2 efeitos · rodada 1 · vez de Pensantus/);

    // The columns of the board, and a row for each effect, named after it.
    const header = panel.locator('.fx__cols');
    for (const column of ['Efeito', 'Em quem', 'Origem', 'Duração e fim', 'Jogadores veem']) {
      await expect(header).toContainText(column);
    }
    const bless = effectRow(panel, 'Bênção');
    await expect(bless).toContainText('Concentração');
    await expect(bless.locator('.fx__who')).toContainText(/(Pensantus, Goblin 1|Goblin 1, Pensantus) \(2 alvos\)/);
    await expect(bless.locator('.fx__origin')).toContainText('De Pensantus');
    await expect(bless.locator('.fx__line').first()).toContainText(/Restam \d+ rodadas?: acaba no turno de Pensantus/);
    const prone = effectRow(panel, 'Derrubado');
    await expect(prone.locator('.fx__who')).toContainText('Goblin 2');
    await expect(prone.locator('.fx__origin')).toContainText('Do mestre');

    // The clock follows the initiative from the turn that is running; the phone's subtitle is not on the table.
    await expect(panel.getByRole('heading', { level: 3, name: 'O relógio dos turnos, a partir da vez de Pensantus' })).toBeVisible();
    await expect(bless.locator('.fx__sub')).toBeHidden();

    // Keyboard: "Encerrar" is a button of the row, in the order of Tab after the row's own controls.
    const endBless = bless.getByRole('button', { name: /^Encerrar Bênção/ });
    await endBless.focus();
    await expect(endBless).toBeFocused();
    expect((await boxOf(endBless)).height).toBeGreaterThanOrEqual(44);
  } finally {
    await t.done();
  }
});

test('"Encerrar" numa concentração pede a confirmação em danger-outline com o foco em "Cancelar"; um efeito sem concentração encerra sem pergunta', { tag: ['@W7-E', '@RN-22', '@RN-10'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const t = await effectsTable(browser, 'Encerrar efeitos');
  try {
    const panel = await openPanel(t);
    // Derrubado has no concentration: it ends at once.
    await effectRow(panel, 'Derrubado').getByRole('button', { name: /^Encerrar/ }).first().click();
    await expect(panel.getByRole('heading', { level: 3, name: 'Derrubado', exact: true })).toHaveCount(0);

    const bless = effectRow(panel, 'Bênção');
    const end = bless.getByRole('button', { name: /^Encerrar Bênção/ });
    await end.click();
    const ask = panel.getByRole('alertdialog', { name: /Encerrar Bênção de Pensantus\?/ });
    await expect(ask).toContainText(/A concentração de Pensantus acaba e Bênção sai de (Pensantus e Goblin 1|Goblin 1 e Pensantus)\./);
    await expect(ask).toContainText('Isto não se desfaz.');
    await expect(ask.getByRole('button', { name: 'Cancelar' })).toBeFocused();

    // Esc and "Cancelar" give the question back and the focus to "Encerrar"; nothing ended.
    await t.m.keyboard.press('Escape');
    await expect(ask).toHaveCount(0);
    await expect(end).toBeFocused();
    await end.click();
    await ask.getByRole('button', { name: 'Cancelar' }).click();
    await expect(ask).toHaveCount(0);
    await expect(effectRow(panel, 'Bênção')).toBeVisible();

    // Confirmed, the whole casting goes, and it is gone from the combat the server holds.
    await end.click();
    await ask.getByRole('button', { name: 'Encerrar Bênção' }).click();
    await expect(panel.getByRole('heading', { level: 3, name: 'Bênção', exact: true })).toHaveCount(0);
    await expect(panel.getByText('Nenhum efeito em jogo agora.')).toBeVisible();
  } finally {
    await t.done();
  }
});

test('o mestre muda a duração, tira um alvo e muda o que os jogadores veem', { tag: ['@W7-E', '@RN-22', '@RN-10'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const t = await effectsTable(browser, 'Duração e visibilidade');
  try {
    const panel = await openPanel(t);
    const bless = effectRow(panel, 'Bênção');
    const before = (await bless.locator('.fx__line').first().innerText()).trim();

    // "Mudar a duração": three radios, the first marked, and a number of rounds from 1 to 600.
    await bless.getByRole('button', { name: /^Mudar a duração de Bênção/ }).click();
    let dialog = sheet(t.m, /^Mudar a duração de Bênção em (Pensantus, Goblin 1|Goblin 1, Pensantus)$/);
    await expect(dialog.getByRole('radio', { name: /^Mais rodadas/ })).toBeChecked();
    await expect(dialog).toContainText('De 1 a 600.');
    await dialog.getByRole('textbox', { name: 'Rodadas' }).fill('601');
    await expect(dialog.getByRole('button', { name: 'Salvar' })).toHaveAttribute('aria-disabled', 'true');
    await dialog.getByRole('textbox', { name: 'Rodadas' }).fill('3');
    await dialog.getByRole('button', { name: 'Salvar' }).click();
    await expect(dialog).toHaveCount(0);
    await expect.poll(async () => (await effectRow(panel, 'Bênção').locator('.fx__line').first().innerText()).trim()).not.toBe(before);

    // "Tirar de um alvo": Goblin 1 loses the Bênção, Pensantus keeps it.
    await effectRow(panel, 'Bênção').getByRole('button', { name: 'Tirar de um alvo' }).click();
    await effectRow(panel, 'Bênção').getByRole('button', { name: 'Tirar Goblin 1' }).click();
    await expect(effectRow(panel, 'Bênção').locator('.fx__who')).toHaveText(/^Em quem: Pensantus$/);
    await expect(effectRow(panel, 'Bênção').getByRole('button', { name: 'Tirar de um alvo' })).toHaveCount(0);

    // The visibility card: the switch, who sees and the label of up to 30 characters.
    await effectRow(panel, 'Derrubado').getByRole('button', { name: /Jogadores veem/ }).click();
    dialog = sheet(t.m, 'O que os jogadores veem de Derrubado em Goblin 2');
    const toggle = dialog.getByRole('switch', { name: 'Os jogadores veem este efeito' });
    await expect(toggle).toHaveAttribute('aria-checked', 'true');
    await expect(dialog.getByRole('radio', { name: /^Todos os jogadores/ })).toBeChecked();
    const label = dialog.getByLabel('O que aparece');
    await label.fill('x'.repeat(40));
    expect(await label.inputValue()).toHaveLength(30);
    await dialog.getByRole('radio', { name: /^Só o dono do alvo/ }).check();
    await dialog.getByRole('button', { name: 'Salvar' }).click();
    await expect(dialog).toHaveCount(0);
    await expect(effectRow(panel, 'Derrubado').locator('.fx__seedesk')).toContainText('só o dono do alvo');

    // Off: nothing reveals it, and the panel says "Não".
    await effectRow(panel, 'Derrubado').getByRole('button', { name: /Jogadores veem/ }).click();
    await dialog.getByRole('switch', { name: 'Os jogadores veem este efeito' }).click();
    await expect(dialog.getByRole('radio', { name: /^Todos os jogadores/ })).toHaveCount(0);
    await dialog.getByRole('button', { name: 'Salvar' }).click();
    await expect(dialog).toHaveCount(0);
    await expect(effectRow(panel, 'Derrubado').locator('.fx__seedesk')).toContainText('Não');
  } finally {
    await t.done();
  }
});

test('"Adicionar efeito": o catálogo, o alvo, quem conjurou e a duração; o efeito entra no painel', { tag: ['@W7-E', '@RN-22', '@RN-10'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const t = await effectsTable(browser, 'Adicionar efeito');
  try {
    const panel = await openPanel(t);
    await panel.getByRole('button', { name: 'Adicionar efeito' }).click();
    const dialog = sheet(t.m, 'Adicionar um efeito');
    await expect(dialog.getByRole('heading', { name: 'Adicionar um efeito' })).toBeVisible();
    await dialog.getByLabel('Em quem').selectOption({ label: 'Capitão Goblin' });
    await dialog.getByLabel('Efeito', { exact: true }).selectOption({ label: 'Velocidade' });
    await expect(dialog.getByLabel('Quem conjurou')).toBeVisible();
    await dialog.getByLabel('Quem conjurou').selectOption({ label: 'Pensantus' });
    await dialog.getByLabel('Duração', { exact: true }).selectOption({ label: 'Outro número de rodadas' });
    await dialog.getByRole('textbox', { name: 'Rodadas' }).fill('5');
    await dialog.getByRole('button', { name: 'Adicionar' }).click();
    await expect(dialog).toHaveCount(0);
    const haste = effectRow(panel, 'Velocidade');
    await expect(haste.locator('.fx__who')).toContainText('Capitão Goblin');
    await expect(haste.locator('.fx__origin')).toContainText('De Pensantus');
    await expect(haste).toContainText('Concentração');
  } finally {
    await t.done();
  }
});

test('a exaustão: níveis de 0 a 6, "Baixar 1 nível" e a confirmação do nível 6 com o foco em "Cancelar"', { tag: ['@W7-E', '@RN-22', '@RN-10'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const t = await effectsTable(browser, 'Exaustão');
  try {
    const panel = await openPanel(t);
    const open = async () => {
      await panel.getByRole('button', { name: 'Exaustão' }).click();
      const dialog = sheet(t.m, /^Exaustão/);
      await expect(dialog).toBeVisible();
      return dialog;
    };
    let dialog = await open();
    await dialog.getByLabel('Quem').selectOption({ label: 'Pensantus' });
    await expect(dialog.getByRole('radio')).toHaveCount(7);
    await expect(dialog).toContainText('baixar o nível não cura. O nível 6 é a morte.');
    await dialog.getByRole('radio', { name: /^Nível 3/ }).check();
    await dialog.getByRole('button', { name: 'Salvar' }).click();
    await expect(dialog).toHaveCount(0);
    await expect.poll(() => exhaustionOf(t.m, t.campaignId, 'Pensantus')).toBe(3);

    dialog = await open();
    await dialog.getByLabel('Quem').selectOption({ label: 'Pensantus' });
    await expect(dialog.getByRole('radio', { name: /^Nível 3/ })).toBeChecked();
    await dialog.getByRole('button', { name: 'Baixar 1 nível' }).click();
    await expect(dialog).toHaveCount(0);
    await expect.poll(() => exhaustionOf(t.m, t.campaignId, 'Pensantus')).toBe(2);

    // Level 6 does not kill by itself: it asks, and "Cancelar" has the focus.
    dialog = await open();
    await dialog.getByLabel('Quem').selectOption({ label: 'Pensantus' });
    await dialog.getByRole('radio', { name: /^Nível 6/ }).check();
    await dialog.getByRole('button', { name: 'Salvar' }).click();
    await expect(dialog.getByRole('heading', { name: 'Exaustão nível 6: Pensantus morre?' })).toBeVisible();
    await expect(dialog).toContainText('O nível 6 é a morte.');
    await expect(dialog).toContainText('Isto não se desfaz.');
    await expect(dialog.getByRole('button', { name: 'Cancelar' })).toBeFocused();
    await dialog.getByRole('button', { name: 'Cancelar' }).click();
    await expect(dialog.getByRole('heading', { name: 'Exaustão de Pensantus' })).toBeVisible();
    await dialog.getByRole('button', { name: 'Cancelar' }).click();
    await expect(dialog).toHaveCount(0);
    expect(await exhaustionOf(t.m, t.campaignId, 'Pensantus')).toBe(2);
  } finally {
    await t.done();
  }
});

test('a 390 px o efeito é uma linha compacta de "Mais", com os botões de 44 px e nada rola de lado', { tag: ['@W7-E', '@RN-22', '@RN-10'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const t = await effectsTable(browser, 'Efeitos no celular', 390);
  try {
    const panel = await openPanel(t);
    await expect(panel.locator('.fx__cols')).toBeHidden();
    const bless = effectRow(panel, 'Bênção');
    await expect(bless.locator('.fx__sub')).toContainText(/Em (Pensantus, Goblin 1|Goblin 1, Pensantus) · de Pensantus/);
    // C7-02: one line per effect (its name, the targets, the end), no frame; "Encerrar" and the others stay 44 px tall.
    await expect(bless.locator('.fx__who')).toBeVisible();
    for (const name of [/^Encerrar Bênção/, /^Mudar a duração de/]) {
      const box = await boxOf(bless.getByRole('button', { name }));
      expect(box.height).toBeGreaterThanOrEqual(44);
    }
    const overflow = await t.m.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow).toBeLessThanOrEqual(0);

    // The visibility card, in a sheet: the switch first, the label field below, the 30 characters named.
    await effectRow(panel, 'Derrubado').getByRole('button', { name: /Jogadores veem/ }).click();
    const dialog = sheet(t.m, 'O que os jogadores veem de Derrubado em Goblin 2');
    await expect(dialog.getByRole('switch', { name: 'Os jogadores veem este efeito' })).toBeVisible();
    await expect(dialog).toContainText('Até 30 caracteres.');
    await expect(dialog).toContainText('não ponha números nele');
    await dialog.getByRole('button', { name: 'Cancelar' }).click();
    await expect(dialog).toHaveCount(0);
  } finally {
    await t.done();
  }
});

test('fora do combate: o painel lista os efeitos dos personagens, "Passar o tempo" e o descanso longo com e sem comida', { tag: ['@W7-E', '@RN-22', '@RN-10'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
  const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 390, height: 844 } });
  const m = await master.newPage();
  const p = await player.newPage();
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableWithPensantus(m, p, `Efeitos fora do combate ${Date.now()}`);
    campaignId = table.campaignId;
    await startSessionRPC(m, campaignId);
    await setExhaustionRPC(m, campaignId, table.characterId, 2, 0);
    await openSessionPage(m, campaignId, 'grupo');

    const panel = m.getByRole('region', { name: 'Efeitos em jogo' });
    await expect(panel.getByText('Nenhum efeito nos personagens agora.')).toBeVisible();

    // "Passar o tempo": the four buttons pass the time at once and the answer says what ended; "Outro tempo" takes any seconds.
    const time = m.getByRole('group', { name: 'Passar o tempo' });
    await time.getByRole('button', { name: '10 min, passar 10 minutos' }).click();
    await expect(panel).toContainText('Passou 10 minutos. Nenhum efeito acabou.');
    await time.getByRole('button', { name: 'Outro tempo' }).click();
    await panel.getByLabel('Segundos').fill('0');
    await expect(panel.getByRole('button', { name: 'Passar', exact: true })).toHaveAttribute('aria-disabled', 'true');
    await panel.getByLabel('Segundos').fill('600');
    await panel.getByRole('button', { name: 'Passar', exact: true }).click();
    await expect(panel).toContainText('Passou 10 minutos. Nenhum efeito acabou.');

    // The long rest asks about food and drink, the switch is off, and one level of exhaustion comes off.
    const rest = m.getByRole('region', { name: 'Descanso' });
    await rest.getByRole('button', { name: 'Descanso longo' }).click();
    const question = rest.getByRole('alertdialog', { name: 'Começar o descanso longo?' });
    const food = question.getByRole('switch', { name: 'Sem comida ou bebida' });
    await expect(food).toHaveAttribute('aria-checked', 'false');
    await expect(question).toContainText('Com comida e bebida, o descanso longo baixa 1 nível de exaustão de cada personagem.');
    await question.getByRole('button', { name: 'Descansar' }).click();
    await expect(rest.getByText('Descanso longo feito.').first()).toBeVisible();
    await openSessionPage(m, campaignId, 'grupo');
    await panel.getByRole('button', { name: 'Exaustão' }).click();
    let dialog = sheet(m, /^Exaustão/);
    await expect(dialog.getByRole('radio', { name: /^Nível 1/ })).toBeChecked();
    await dialog.getByRole('button', { name: 'Cancelar' }).click();

    // Without food or drink, the next long rest takes nothing off.
    await rest.getByRole('button', { name: 'Descanso longo' }).click();
    await question.getByRole('switch', { name: 'Sem comida ou bebida' }).click();
    await expect(question.getByRole('switch', { name: 'Sem comida ou bebida' })).toHaveAttribute('aria-checked', 'true');
    await question.getByRole('button', { name: 'Descansar' }).click();
    await expect(rest.getByText('Descanso longo feito.').first()).toBeVisible();
    await openSessionPage(m, campaignId, 'grupo');
    await panel.getByRole('button', { name: 'Exaustão' }).click();
    dialog = sheet(m, /^Exaustão/);
    await expect(dialog.getByRole('radio', { name: /^Nível 1/ })).toBeChecked();
  } finally {
    await endOpenSessionRPC(m, campaignId);
    await master.close();
    await player.close();
  }
});

test('fora do combate: "Dar efeito" põe um efeito do catálogo no personagem, com a duração em tempo de jogo', { tag: ['@W7-E', '@RN-22', '@RN-10'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
  const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 390, height: 844 } });
  const m = await master.newPage();
  const p = await player.newPage();
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableWithPensantus(m, p, `Dar efeito ${Date.now()}`);
    campaignId = table.campaignId;
    await startSessionRPC(m, campaignId);
    await openSessionPage(m, campaignId, 'grupo');

    const panel = m.getByRole('region', { name: 'Efeitos em jogo' });
    await expect(panel.getByText('Nenhum efeito nos personagens agora.')).toBeVisible();
    await panel.getByRole('button', { name: 'Adicionar efeito' }).click();
    const dialog = sheet(m, 'Dar um efeito');
    await expect(dialog.getByRole('heading', { name: 'Dar um efeito' })).toBeVisible();
    // Nobody ticked yet: nothing to give.
    await expect(dialog.getByRole('button', { name: 'Dar efeito' })).toHaveAttribute('aria-disabled', 'true');
    await dialog.locator('label.check', { hasText: 'Pensantus' }).click();
    await expect(dialog.getByRole('checkbox', { name: 'Pensantus' })).toBeChecked();
    await dialog.getByLabel('Efeito', { exact: true }).selectOption({ label: 'Aprimorar Habilidade' });

    // Aprimorar Habilidade is cast for one ability: the six, each with its animal, and nothing goes without one.
    await expect(dialog.getByRole('radio')).toHaveCount(6 + 3);
    await expect(dialog).toContainText('Coruja');
    await expect(dialog.getByRole('button', { name: 'Dar efeito' })).toHaveAttribute('aria-disabled', 'true');
    await dialog.locator('label.row', { hasText: 'Sabedoria' }).click();
    await expect(dialog.getByRole('radio', { name: /^Sabedoria/ })).toBeChecked();
    await dialog.getByLabel('Duração', { exact: true }).selectOption({ label: 'Um tempo de jogo' });
    await dialog.getByLabel('Quanto tempo').selectOption({ label: '1 hora' });
    await expect(dialog).toContainText('Dura 1 hora de tempo de jogo.');
    await dialog.getByRole('button', { name: 'Dar efeito' }).click();
    await expect(dialog).toHaveCount(0);

    const card = panel.getByRole('listitem').filter({ hasText: 'Aprimorar Habilidade' });
    await expect(card).toContainText('Pensantus');
    await expect(card).toContainText(/1 hora|3600|60 minutos/);

    // The time the master passes is taken off the effect.
    await m.getByRole('group', { name: 'Passar o tempo' }).getByRole('button', { name: '10 min, passar 10 minutos' }).click();
    await expect(panel).toContainText('Passou 10 minutos. Nenhum efeito acabou.');
  } finally {
    await endOpenSessionRPC(m, campaignId);
    await master.close();
    await player.close();
  }
});

test('um efeito posto a mão some do painel quando o servidor o encerra por outro caminho', { tag: ['@W7-E', '@RN-22', '@RN-10'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const t = await effectsTable(browser, 'Efeito encerrado por fora');
  try {
    const panel = await openPanel(t);
    const enc = await getEncounterRPC(t.m, t.campaignId);
    await addEffectRPC(t.m, t.campaignId, enc.id, 'condition:prone', [t.id('Capitão Goblin')]);
    // The panel reads again by itself when the combat changes: the new row comes with no reload.
    await expect(panel.locator('li.fx__row')).toHaveCount(3, { timeout: 30_000 });
    const list = (await effectsRPC(t.m, 'ListLastingEffects', { campaignId: t.campaignId, encounterId: enc.id })) as { effects: { id: string; sourceNamePt: string; targetLabels: string[] }[] };
    const mine = list.effects.find((e) => e.sourceNamePt === 'Derrubado' && e.targetLabels.includes('Capitão Goblin'))!;
    await effectsRPC(t.m, 'EndLastingEffect', { campaignId: t.campaignId, encounterId: enc.id, effectId: mine.id, scope: 'EFFECT_END_SCOPE_THIS' });
    await expect(panel.locator('li.fx__row')).toHaveCount(2, { timeout: 30_000 });
  } finally {
    await t.done();
  }
});
