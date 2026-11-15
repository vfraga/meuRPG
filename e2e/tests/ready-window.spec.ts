import { expect, type Browser, type Page } from '@playwright/test';
import { test } from './diagnostics';
import { beginAttackCombatRPC, getEncounterRPC, passTurnsToAndWait, pensantusCasting, tableForCombat, type Encounter } from './combat-support';
import { openCombatPanel } from './combat-layout';
import { endSessionOf } from './feature-use-support';
import { goblinTrigger, readiedOf, readyAttackRPC, readySpellRPC } from './ready-support';
import { openSessionPage } from './live-session-support';
import { waitForEncounterRevision } from './revision-support';
import { newSignedInContext, openCombatLog } from './support';

/**
 * The windows and prompts of a readied action (#582, boards 582-05 to 582-08) through the real stack: the master fires from the fold, the readier's
 * prompt opens by itself with the trigger quoted and the creatures the attack reaches, the master's queue reads the same window in its own words,
 * and the answer ("Atacar" or "Deixar passar"), the loss at the readier's next turn and the window that closed without them are told as the board says.
 */

/** Pensantus next to the Capitão (so the dagger reaches him), Goblin 1 within the dagger's throw and Goblin 2 beyond it. */
const squares: Record<string, [number, number]> = { Pensantus: [1, 1], 'Capitão Goblin': [2, 1], 'Goblin 1': [9, 9], 'Goblin 2': [14, 10] };
const faces = { Pensantus: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 };

/** The table: Pensantus readied an attack on the Capitão, and the turn is the Capitão's, with both pages open on the session. */
async function readiedTable(browser: Browser, name: string) {
  const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
  const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 900 } });
  const m = await master.newPage();
  const p = await player.newPage();
  await m.goto('/');
  await p.goto('/');
  const table = await tableForCombat(m, p, `${name} ${Date.now()}`, true, true, { sheet: pensantusCasting });
  const begun = await beginAttackCombatRPC(m, table, faces, squares);
  await openSessionPage(p, table.campaignId);
  await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
  await waitForEncounterRevision(p, await readyAttackRPC(p, table.campaignId, begun, 'Pensantus', 'Capitão Goblin'));
  await openSessionPage(m, table.campaignId);
  await passTurnsToAndWait(m, table.campaignId, 'Capitão Goblin', [m, p]);
  return { master, player, m, p, campaignId: table.campaignId };
}

/** "Disparar" of the fold on the master's page. */
async function fire(m: Page): Promise<void> {
  const button = m.locator('app-readied-fold').getByRole('button', { name: 'Disparar a ação de Pensantus' });
  await expect(button).toBeEnabled();
  await button.click();
}

test('o mestre dispara, o jogador lê o gatilho e ataca, e o mestre lê a mesma janela com as palavras dele', { tag: ['@RN-31', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { master, player, m, p, campaignId } = await readiedTable(browser, 'Preparar janela');
  try {
    await fire(m);

    // The readier's prompt opens by itself: the trigger quoted, the creature named at Ready chosen, the one out of reach written in its card.
    const prompt = p.getByRole('alertdialog', { name: 'Ação preparada: atacar com Adaga' });
    await expect(prompt).toBeVisible();
    await expect(prompt).toContainText(`O mestre disse que o gatilho aconteceu: “${goblinTrigger}”. Atacar com a Adaga agora?`);
    await expect(prompt).toContainText('Em quem · 2 criaturas ao alcance');
    await expect(prompt.getByRole('radio', { name: /Capitão Goblin/ })).toBeChecked();
    await expect(prompt.locator('label', { hasText: 'Goblin 2' })).toContainText('Fora de alcance: até 18 m');
    await expect(prompt.getByRole('button', { name: 'Deixar passar' })).toBeFocused();
    await expect(prompt).toContainText(/Se você não responder, passa sozinho em \d+ s\./);

    // The master reads the same window in his queue: what is held and the trigger, who the table waits for, "Deixar passar" first.
    const queue = m.locator('app-reaction-queue[part="rest"]');
    await expect(queue).toContainText(`Ação preparada: Adaga no Capitão Goblin · “${goblinTrigger}”`);
    await expect(queue).toContainText(/Esperando Pensantus · passa sozinho em \d+ s/);
    await expect(queue.locator('.btns button')).toHaveText(['Deixar passar', 'Atacar por Pensantus']);

    // The attack is the reaction: the prompt closes, the reaction is spent and the log tells the use, then the attack.
    await prompt.getByRole('button', { name: 'Atacar o Capitão Goblin' }).click();
    await expect(prompt).toHaveCount(0);
    const after = await getEncounterRPC(p, campaignId);
    expect(after.reactionWindows ?? []).toHaveLength(0);
    expect(after.combatants.find((c) => c.label === 'Pensantus')?.reactionUsed).toBe(true);
    expect(after.combatants.map(readiedOf).filter((r) => r !== undefined)).toHaveLength(0);
    await waitForEncounterRevision(p, after);
    const log = await openCombatLog(p);
    // The server's line carries the held action and the trigger, no target (the attack's own line follows it).
    await expect(log).toContainText(`Pensantus usou a ação preparada: Adaga, “${goblinTrigger}”`);
    // The readied attack is a reaction but not an opportunity attack: its own line does not say so.
    await expect(log).toContainText('Pensantus ataca o Capitão Goblin com a Adaga');
    await expect(log).not.toContainText('(ataque de oportunidade)');
  } finally {
    await endSessionOf(m, campaignId);
    await master.close();
    await player.close();
  }
});

test('deixar passar mantém a reação, e uma janela que o mestre passou diz isso ao jogador', { tag: ['@RN-31', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { master, player, m, p, campaignId } = await readiedTable(browser, 'Preparar passar');
  try {
    await fire(m);
    const prompt = p.getByRole('alertdialog', { name: 'Ação preparada: atacar com Adaga' });
    await expect(prompt).toBeVisible();
    // The master lets it pass from his queue while the readier still reads the prompt: it says so and only closes.
    await m.locator('app-reaction-queue[part="rest"]').getByRole('button', { name: 'Deixar passar' }).click();
    // Nothing on the wire says who closed it (the clock ran for a few seconds only): the text does not claim the 30 s, and the focus goes to Fechar.
    await expect(prompt).toContainText('A janela fechou.');
    await expect(prompt).not.toContainText('Passou sozinho');
    await expect(prompt.getByRole('button', { name: 'Deixar passar' })).toHaveCount(0);
    await expect(prompt.getByRole('button', { name: 'Fechar' })).toBeFocused();
    await prompt.getByRole('button', { name: 'Fechar' }).click();
    await expect(prompt).toHaveCount(0);
    const after = await getEncounterRPC(p, campaignId);
    expect(after.combatants.find((c) => c.label === 'Pensantus')?.reactionUsed ?? false).toBe(false);
    expect(after.combatants.map(readiedOf).filter((r) => r !== undefined)).toHaveLength(0);
    await waitForEncounterRevision(p, after);
    await openCombatPanel(m, 'Registro');
    await expect(m.getByRole('log', { name: 'Registro do combate' })).toContainText('Pensantus deixou passar a ação preparada (Adaga)');
  } finally {
    await endSessionOf(m, campaignId);
    await master.close();
    await player.close();
  }
});

test('o gatilho que não veio perde a ação preparada e o jogador lê o aviso na vez dele', { tag: ['@RN-31', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { master, player, m, p, campaignId } = await readiedTable(browser, 'Preparar perdida');
  try {
    // The trigger never comes: the turns go round and the readier's own turn starts first.
    const enc: Encounter = await passTurnsToAndWait(m, campaignId, 'Pensantus', [m, p]);
    await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
    const notice = p.getByTestId('readied-lost');
    await expect(notice).toContainText('A sua ação preparada se perdeu.');
    await expect(notice).toContainText('A ação gasta não volta.');
    expect(enc.combatants.map(readiedOf).filter((r) => r !== undefined && r.status === 'READIED_STATUS_WAITING')).toHaveLength(0);
    await notice.getByRole('button', { name: 'Entendi' }).click();
    await expect(notice).toHaveCount(0);
    const log = await openCombatLog(p);
    await expect(log).toContainText('A Adaga preparada de Pensantus se perdeu');
  } finally {
    await endSessionOf(m, campaignId);
    await master.close();
    await player.close();
  }
});

test('o jogador deixa passar pelo próprio aviso: a reação continua livre e a janela fecha', { tag: ['@RN-31', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { master, player, m, p, campaignId } = await readiedTable(browser, 'Preparar jogador passa');
  try {
    await fire(m);
    const prompt = p.getByRole('alertdialog', { name: 'Ação preparada: atacar com Adaga' });
    await expect(prompt).toBeVisible();
    await prompt.getByRole('button', { name: 'Deixar passar' }).click();
    await expect(prompt).toHaveCount(0);
    const after = await getEncounterRPC(p, campaignId);
    expect(after.reactionWindows ?? []).toHaveLength(0);
    expect(after.combatants.find((c) => c.label === 'Pensantus')?.reactionUsed ?? false).toBe(false);
    await waitForEncounterRevision(p, after);
    const log = await openCombatLog(p);
    await expect(log).toContainText('Pensantus deixou passar a ação preparada (Adaga)');
  } finally {
    await endSessionOf(m, campaignId);
    await master.close();
    await player.close();
  }
});

test('um ataque de magia preparado é solto pelo jogador, na criatura que ele escolhe', { tag: ['@RN-31', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
  const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 900 } });
  const m = await master.newPage();
  const p = await player.newPage();
  await m.goto('/');
  await p.goto('/');
  const table = await tableForCombat(m, p, `Preparar magia ${Date.now()}`, true, true, { sheet: pensantusCasting });
  const campaignId = table.campaignId;
  try {
    const begun = await beginAttackCombatRPC(m, table, faces, squares);
    await openSessionPage(p, campaignId);
    await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
    await waitForEncounterRevision(p, await readySpellRPC(p, campaignId, begun, 'Pensantus', 'spell:fire-bolt', 0, 'Capitão Goblin'));
    await openSessionPage(m, campaignId);
    await passTurnsToAndWait(m, campaignId, 'Capitão Goblin', [m, p]);
    await fire(m);

    // A cantrip that attacks is held as a spell and released as an attack: the creatures it reaches, the one named at Ready chosen.
    const prompt = p.getByRole('alertdialog', { name: 'Soltar Raio de Fogo' });
    await expect(prompt).toBeVisible();
    await expect(prompt).toContainText('ao alcance');
    await expect(prompt.getByRole('radio', { name: /Capitão Goblin/ })).toBeChecked();
    await expect(prompt.locator('.pair button')).toHaveText(['Deixar passar', 'Soltar no Capitão Goblin']);
    await prompt.getByRole('button', { name: 'Soltar no Capitão Goblin' }).click();
    await expect(prompt).toHaveCount(0);
    const after = await getEncounterRPC(p, campaignId);
    expect(after.reactionWindows ?? []).toHaveLength(0);
    expect(after.combatants.find((c) => c.label === 'Pensantus')?.reactionUsed).toBe(true);
    await waitForEncounterRevision(p, after);
    await expect(p.getByTestId('readied-lost')).toHaveCount(0);
    const log = await openCombatLog(p);
    await expect(log).not.toContainText('(ataque de oportunidade)');
  } finally {
    await endSessionOf(m, campaignId);
    await master.close();
    await player.close();
  }
});

test('uma magia de dardos preparada é recusada na soltura, e deixá-la passar não gera o aviso de perda', { tag: ['@RN-31', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
  const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 900 } });
  const m = await master.newPage();
  const p = await player.newPage();
  await m.goto('/');
  await p.goto('/');
  const table = await tableForCombat(m, p, `Preparar magia passa ${Date.now()}`, true, true, { sheet: pensantusCasting });
  const campaignId = table.campaignId;
  try {
    const begun = await beginAttackCombatRPC(m, table, faces, squares);
    await openSessionPage(p, campaignId);
    await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
    await waitForEncounterRevision(p, await readySpellRPC(p, campaignId, begun, 'Pensantus', 'spell:magic-missile', 1, 'Capitão Goblin'));
    await openSessionPage(m, campaignId);
    await passTurnsToAndWait(m, campaignId, 'Capitão Goblin', [m, p]);
    await fire(m);
    const prompt = p.getByRole('alertdialog', { name: 'Soltar Mísseis Mágicos' });
    // Magic Missile shares darts among its targets, which AnswerReaction does not carry: the server refuses the release (and nothing is
    // spent), the prompt says what it needs and the readier can still let it pass.
    await prompt.getByRole('button', { name: 'Soltar no Capitão Goblin' }).click();
    await expect(prompt.getByRole('alert')).toContainText('Não deu para soltar a magia por aqui: ela pede dardos ou um ponto no mapa');
    expect((await getEncounterRPC(p, campaignId)).combatants.find((c) => c.label === 'Pensantus')?.reactionUsed ?? false).toBe(false);
    await prompt.getByRole('button', { name: 'Deixar passar' }).click();
    await expect(prompt).toHaveCount(0);
    const after = await getEncounterRPC(p, campaignId);
    await waitForEncounterRevision(p, after);
    const log = await openCombatLog(p);
    await expect(log).toContainText('se perdeu');
    // The readier chose it: the loss is in the log, but nothing interrupts the table with a notice.
    await expect(p.getByTestId('readied-lost')).toHaveCount(0);
  } finally {
    await endSessionOf(m, campaignId);
    await master.close();
    await player.close();
  }
});

test('o mestre escolhe em quem o ataque preparado, sem alvo no gatilho, é feito por um jogador', { tag: ['@RN-31', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
  const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 900 } });
  const m = await master.newPage();
  const p = await player.newPage();
  await m.goto('/');
  await p.goto('/');
  const table = await tableForCombat(m, p, `Preparar sem alvo ${Date.now()}`, true, true, { sheet: pensantusCasting });
  const campaignId = table.campaignId;
  try {
    const begun = await beginAttackCombatRPC(m, table, faces, squares);
    await openSessionPage(p, campaignId);
    await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
    await waitForEncounterRevision(p, await readyAttackRPC(p, campaignId, begun, 'Pensantus'));
    await openSessionPage(m, campaignId);
    await passTurnsToAndWait(m, campaignId, 'Capitão Goblin', [m, p]);
    await fire(m);
    const queue = m.locator('app-reaction-queue[part="rest"]');
    // No creature was named: the use waits for the master to pick one (the server refuses an attack with none).
    await expect(queue.getByRole('radio', { name: /Capitão Goblin/ })).toBeVisible();
    const use = queue.getByRole('button', { name: 'Atacar por Pensantus' });
    await expect(use).toHaveClass(/btn--off/);
    await queue.locator('label', { hasText: 'Capitão Goblin' }).click();
    await expect(use).not.toHaveClass(/btn--off/);
    await use.click();
    await expect(queue.locator('.row')).toHaveCount(0);
    const after = await getEncounterRPC(m, campaignId);
    expect(after.reactionWindows ?? []).toHaveLength(0);
    expect(after.combatants.find((c) => c.label === 'Pensantus')?.reactionUsed).toBe(true);
    // The readier's prompt, still open, says the window closed (the reaction was used).
    await expect(p.getByRole('alertdialog', { name: 'Ação preparada: atacar com Adaga' })).toContainText('A janela fechou e a sua reação foi usada.');
  } finally {
    await endSessionOf(m, campaignId);
    await master.close();
    await player.close();
  }
});
