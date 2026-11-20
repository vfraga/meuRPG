import { expect, type Browser, type Page } from '@playwright/test';
import { test } from './diagnostics';
import { beginAttackCombatRPC, getEncounterRPC, passTurnsToAndWait, pensantusCasting, tableForCombat, type Encounter } from './combat-support';
import { openCombatPanel } from './combat-layout';
import { endSessionOf } from './feature-use-support';
import { goblinTrigger, pensantusReadyKit, readiedOf, readyAttackRPC, readySpellRPC } from './ready-support';
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
    // "Deixar passar" has the focus at open. Whether the browser draws a ring on a focus a script sets is its own call (a window opened by the master's word has had no pointer), so only the focus is asserted.
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

/** The table where Pensantus readied a spell of the kit with a slot of its level, naming `target` if any, and the master fired it: the readier's prompt is open. */
async function heldSpellTable(browser: Browser, name: string, spellKey: string, slotLevel: number, target?: string) {
  const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
  const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 900 } });
  const m = await master.newPage();
  const p = await player.newPage();
  await m.goto('/');
  await p.goto('/');
  const table = await tableForCombat(m, p, `${name} ${Date.now()}`, true, true, { sheet: pensantusReadyKit });
  const campaignId = table.campaignId;
  const begun = await beginAttackCombatRPC(m, table, faces, squares);
  await openSessionPage(p, campaignId);
  await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
  await waitForEncounterRevision(p, await readySpellRPC(p, campaignId, begun, 'Pensantus', spellKey, slotLevel, target));
  await openSessionPage(m, campaignId);
  await passTurnsToAndWait(m, campaignId, 'Capitão Goblin', [m, p]);
  await fire(m);
  return { master, player, m, p, campaignId };
}

/** Mísseis Mágicos at the 1st level, the Capitão named at Ready. */
const missilesTable = (browser: Browser, name: string) => heldSpellTable(browser, name, 'spell:magic-missile', 1, 'Capitão Goblin');

test('uma magia de dardos preparada é solta com os dardos repartidos pelo jogador', { tag: ['@RN-31', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { master, player, m, p, campaignId } = await missilesTable(browser, 'Preparar dardos solta');
  try {
    const prompt = p.getByRole('alertdialog', { name: 'Soltar Mísseis Mágicos' });
    await expect(prompt).toBeVisible();
    // The darts are shared out as the cast screen shares them: the release waits until all three are placed.
    await expect(prompt.getByText('Divida os 3 dardos entre os alvos')).toBeVisible();
    await expect(prompt.getByText('Distribua todos os dardos: 0 de 3.')).toBeVisible();
    const release = prompt.getByRole('button', { name: 'Soltar agora' });
    await expect(release).toHaveAttribute('aria-disabled', 'true');
    await prompt.getByRole('button', { name: 'Pôr um dardo em Capitão Goblin' }).click();
    await prompt.getByRole('button', { name: 'Pôr um dardo em Capitão Goblin' }).click();
    await prompt.getByRole('button', { name: 'Pôr um dardo em Goblin 1' }).click();
    await expect(prompt.getByText('3 de 3 dardos distribuídos.')).toBeVisible();
    await expect(release).not.toHaveAttribute('aria-disabled', 'true');
    await release.click();
    await expect(prompt).toHaveCount(0);
    const after = await getEncounterRPC(p, campaignId);
    expect(after.reactionWindows ?? []).toHaveLength(0);
    expect(after.combatants.find((c) => c.label === 'Pensantus')?.reactionUsed).toBe(true);
    await waitForEncounterRevision(p, after);
    const log = await openCombatLog(p);
    await expect(log).toContainText('Mísseis Mágicos');
  } finally {
    await endSessionOf(m, campaignId);
    await master.close();
    await player.close();
  }
});

test('deixar passar uma magia de dardos preparada não gera o aviso de perda', { tag: ['@RN-31', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { master, player, m, p, campaignId } = await missilesTable(browser, 'Preparar dardos passa');
  try {
    const prompt = p.getByRole('alertdialog', { name: 'Soltar Mísseis Mágicos' });
    await expect(prompt).toBeVisible();
    await prompt.getByRole('button', { name: 'Deixar passar' }).click();
    await expect(prompt).toHaveCount(0);
    const after = await getEncounterRPC(p, campaignId);
    expect(after.combatants.find((c) => c.label === 'Pensantus')?.reactionUsed ?? false).toBe(false);
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

test('o seletor de dardos, só com o teclado, mantém o foco dentro do aviso quando o último dardo é posto e quando o último é tirado', { tag: ['@RN-31', '@RN-22', '@a11y'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { master, player, m, p, campaignId } = await missilesTable(browser, 'Preparar dardos teclado');
  try {
    const prompt = p.getByRole('alertdialog', { name: 'Soltar Mísseis Mágicos' });
    await expect(prompt).toBeVisible();
    const add = prompt.getByRole('button', { name: 'Pôr um dardo em Capitão Goblin' });
    const take = prompt.getByRole('button', { name: 'Tirar um dardo de Capitão Goblin' });
    /** Where the focus is after the page drew twice: inside the prompt, on a control, never on the page behind it (the body). */
    const focusNow = async () => {
      await p.evaluate(() => new Promise<void>((done) => requestAnimationFrame(() => requestAnimationFrame(() => done()))));
      return prompt.evaluate((el) => {
        const on = document.activeElement;
        return { inside: !!on && on !== document.body && el.contains(on), label: on?.getAttribute('aria-label') ?? on?.tagName ?? '' };
      });
    };

    // Three darts with Enter from the "+" of the Capitão: after the third the "+" has no dart left to place, and the focus stays on it.
    await add.focus();
    for (let i = 1; i <= 3; i++) {
      await p.keyboard.press('Enter');
      await expect(prompt.getByText(i === 3 ? '3 de 3 dardos distribuídos.' : `Distribua todos os dardos: ${i} de 3.`)).toBeVisible();
      expect(await focusNow(), `depois do dardo ${i}`).toEqual({ inside: true, label: 'Pôr um dardo em Capitão Goblin' });
    }
    await expect(add).toHaveAttribute('aria-disabled', 'true');
    // Pressed again at the limit it does nothing: no fourth dart.
    await p.keyboard.press('Enter');
    await expect(prompt.getByText('3 de 3 dardos distribuídos.')).toBeVisible();

    // And back down with Enter from the "−": after the last dart is taken the "−" is off, and the focus stays on it.
    await take.focus();
    for (let i = 2; i >= 0; i--) {
      await p.keyboard.press('Enter');
      await expect(prompt.getByText(`Distribua todos os dardos: ${i} de 3.`)).toBeVisible();
      expect(await focusNow(), `ao tirar até ${i}`).toEqual({ inside: true, label: 'Tirar um dardo de Capitão Goblin' });
    }
    await expect(take).toHaveAttribute('aria-disabled', 'true');
    await prompt.getByRole('button', { name: 'Deixar passar' }).click();
    await expect(prompt).toHaveCount(0);
  } finally {
    await endSessionOf(m, campaignId);
    await master.close();
    await player.close();
  }
});

test('uma magia de uma criatura preparada é solta na criatura que o jogador escolhe da lista', { tag: ['@RN-31', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { master, player, m, p, campaignId } = await heldSpellTable(browser, 'Preparar criatura solta', 'spell:charm-person', 1);
  try {
    const prompt = p.getByRole('alertdialog', { name: 'Soltar Enfeitiçar Pessoa' });
    await expect(prompt).toBeVisible();
    // No creature was named at Ready: the list asks for one and the release waits.
    await expect(prompt.getByText('Escolha o alvo (só quem você vê)')).toBeVisible();
    await expect(prompt.getByRole('radio', { name: /Capitão Goblin/ })).not.toBeChecked();
    const release = prompt.getByRole('button', { name: 'Soltar agora' });
    await expect(release).toHaveAttribute('aria-disabled', 'true');
    await prompt.locator('label', { hasText: 'Capitão Goblin' }).click();
    await expect(prompt.getByRole('radio', { name: /Capitão Goblin/ })).toBeChecked();
    // The first touch on the list told the table the player is choosing: their prompt says so, and the master's clock now counts the longer wait.
    await expect(prompt).toContainText(/Você está escolhendo: a mesa espera 1 min \d+ s\. Depois disso, passa sozinho\./);
    const queue = m.locator('app-reaction-queue[part="rest"]');
    await expect
      .poll(
        async () => {
          const left = /passa sozinho em (?:(\d+) min)?\s*(?:(\d+) s)?/.exec(await queue.innerText());
          return Number(left?.[1] ?? 0) * 60 + Number(left?.[2] ?? 0);
        },
        { message: 'os segundos do relógio do mestre' },
      )
      .toBeGreaterThan(30);
    await release.click();
    await expect(prompt).toHaveCount(0);
    const after = await getEncounterRPC(p, campaignId);
    expect(after.reactionWindows ?? []).toHaveLength(0);
    expect(after.combatants.find((c) => c.label === 'Pensantus')?.reactionUsed).toBe(true);
    await waitForEncounterRevision(p, after);
    await expect(p.getByTestId('readied-lost')).toHaveCount(0);
    const log = await openCombatLog(p);
    await expect(log).toContainText('Enfeitiçar Pessoa');
  } finally {
    await endSessionOf(m, campaignId);
    await master.close();
    await player.close();
  }
});
