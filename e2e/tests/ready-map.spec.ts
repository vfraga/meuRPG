import { expect } from '@playwright/test';
import { test } from './diagnostics';
import { getEncounterRPC } from './combat-support';
import { openSessionPage } from './live-session-support';
import { tapSquare } from './move-support';
import { fireReadied, masterQueue, moveWindow, pickFireballPoint, placeFireball, readyMapTable, releaseSheet, releaseTexts, spellWindow, undoFire } from './ready-map-support';
import { readiedOf } from './ready-support';

/**
 * A held spell or move released on the map (#857, #856, boards 857-01 to 857-09) through the real stack: the window sends the player to the
 * map (the cast sheet or the Mover page in release mode), "Voltar" comes back with the mark cleared, the master releases for the player with
 * the footer that says the player may still answer, the wait shows where the player is, and a trigger the master undid is told as the board says.
 */

/** The window of the table that is open for the readier, as the server sends it. */
async function windowOf(page: Parameters<typeof getEncounterRPC>[0], campaignId: string) {
  const enc = await getEncounterRPC(page, campaignId);
  return (enc.reactionWindows ?? [])[0];
}

test('o jogador solta a Bola de Fogo preparada no mapa: o ponto, quem está na área, a conjuração e a volta à sessão', { tag: ['@RN-31', '@RN-22', '@MR-014'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  const { m, p, campaignId, done } = await readyMapTable(browser, 'Soltar no mapa', 'fireball');
  try {
    await fireReadied(m);
    const win = spellWindow(p);
    await expect(win).toBeVisible();

    // The window: what it costs (the legend), what the map does, and the way to the map. The master's row waits for the readier.
    await expect(win.getByTestId('readied-legend')).toContainText('Sem espaço: o de 3º nível foi gasto ao preparar.');
    await expect(win.getByTestId('release-map-notice')).toBeVisible();
    await expect(win.getByTestId('release-to-map')).toHaveText(/Soltar no mapa/);
    await expect(win.getByTestId('readied-clock')).toContainText(/Se você não responder, passa sozinho em/);
    const queue = masterQueue(m);
    await expect(queue.getByTestId('readied-wait')).toContainText(/Esperando Pensantus · passa sozinho em/);
    await expect(queue.locator('.btns button')).toHaveText(['Deixar passar', 'Soltar por Pensantus']);

    // The map: the band with the trigger and the slot, the wait line, and the master's row that reads "No mapa".
    const sheet = await placeFireball(p);
    await expect(sheet.getByTestId('release-band')).toContainText('Preparada:');
    await expect(sheet.getByTestId('release-band')).toContainText(/Espaço de 3º\s+nível · gasto ao preparar/);
    await expect(sheet.getByTestId('release-wait')).toHaveText(releaseTexts.onMap);
    await expect(queue.getByTestId('readied-wait')).toContainText(/No mapa · passa sozinho em/);
    expect((await windowOf(m, campaignId))?.rolling).toBe(true);

    // The cast: nothing is spent twice (the slot went at Ready), the damage is rolled once and the sheet leaves with "Voltar à sessão".
    await expect(sheet.getByText('3 criaturas', { exact: true })).toBeVisible();
    await sheet.getByRole('button', { name: 'Conjurar Bola de Fogo' }).click();
    // The dialog is named by its title, which becomes "Bola de Fogo conjurada" with the result.
    const result = p.getByRole('dialog', { name: 'Bola de Fogo conjurada' });
    await expect(result.getByRole('heading', { name: 'Bola de Fogo conjurada' })).toBeVisible();
    await result.getByRole('button', { name: 'Digitar o resultado' }).click();
    await result.getByLabel(/Role 8d6/).fill('28');
    await result.getByRole('button', { name: 'Confirmar 28' }).click();
    await result.getByRole('button', { name: releaseTexts.done }).click();
    await expect(result).toHaveCount(0);

    const after = await getEncounterRPC(m, campaignId);
    expect(after.reactionWindows ?? []).toHaveLength(0);
    expect(after.combatants.find((c) => c.label === 'Pensantus')?.reactionUsed).toBe(true);
    expect(after.combatants.map(readiedOf).filter((r) => r !== undefined && r.status === 'READIED_STATUS_WAITING')).toHaveLength(0);
    expect(after.combatants.filter((c) => c.label.startsWith('Goblin')).every((g) => g.defeated)).toBe(true);
  } finally {
    await done();
  }
});

test('"Voltar" do mapa traz a janela de novo, tira a marca e deixa a reação livre; a espera de 2 min já começou', { tag: ['@RN-31', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { m, p, campaignId, done } = await readyMapTable(browser, 'Voltar do mapa', 'fireball');
  try {
    await fireReadied(m);
    await expect(spellWindow(p)).toBeVisible();
    await spellWindow(p).getByTestId('release-to-map').click();
    const sheet = releaseSheet(p);
    await expect(sheet).toBeVisible();
    // The way back is "Voltar" (the board draws the "?" and no close button).
    await expect(sheet.getByRole('button', { name: 'Fechar' })).toHaveCount(0);
    await expect.poll(async () => (await windowOf(m, campaignId))?.rolling).toBe(true);

    await sheet.getByRole('button', { name: releaseTexts.back, exact: true }).click();
    await expect(sheet).toHaveCount(0);
    await expect(spellWindow(p)).toBeVisible();
    // The mark is off, the master's row waits for the readier again, and nothing was spent.
    await expect.poll(async () => (await windowOf(m, campaignId))?.rolling ?? false).toBe(false);
    expect((await windowOf(m, campaignId))?.waitBeganAt).toBeTruthy();
    await expect(masterQueue(m).getByTestId('readied-wait')).toContainText(/Esperando Pensantus · passa sozinho em/);
    expect((await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Pensantus')?.reactionUsed ?? false).toBe(false);

    // The player lets it pass from the window: the reaction stays free and the window closes.
    await spellWindow(p).getByRole('button', { name: 'Deixar passar' }).click();
    await expect(spellWindow(p)).toHaveCount(0);
    expect((await getEncounterRPC(m, campaignId)).reactionWindows ?? []).toHaveLength(0);
  } finally {
    await done();
  }
});

test('com o mapa aberto num aparelho, o outro aparelho do jogador lê onde a mesa espera', { tag: ['@RN-31', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { m, p, player, campaignId, done } = await readyMapTable(browser, 'Outro aparelho', 'fireball');
  try {
    await fireReadied(m);
    await spellWindow(p).getByTestId('release-to-map').click();
    await expect(releaseSheet(p)).toBeVisible();

    const other = await player.newPage();
    await openSessionPage(other, campaignId);
    await expect(spellWindow(other)).toBeVisible();
    await expect(spellWindow(other).getByTestId('readied-clock')).toContainText(releaseTexts.otherDevice);
  } finally {
    await done();
  }
});

test('o mestre solta a Bola de Fogo por Pensantus: o rodapé diz que ele ainda pode responder, e "Voltar" não gasta nada', { tag: ['@RN-31', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { m, p, campaignId, done } = await readyMapTable(browser, 'Mestre solta', 'fireball', { width: 1280, height: 800, both: true });
  try {
    await fireReadied(m);
    await expect(spellWindow(p)).toBeVisible();
    await masterQueue(m).getByRole('button', { name: 'Soltar por Pensantus' }).click();
    const sheet = releaseSheet(m, true);
    await expect(sheet).toBeVisible();
    await expect(sheet.getByTestId('release-foot')).toHaveText(releaseTexts.masterFoot);
    await expect(sheet.getByRole('button', { name: 'Fechar' })).toHaveCount(0);
    // The board's header: the filled marker and a bold "Reação" before the rest, and the band under it.
    await expect(sheet.locator('.frame__sub b')).toHaveText('Reação');
    await expect(sheet.locator('.frame__sub')).toHaveText('Reação · a ação preparada de Pensantus · você responde por Pensantus');
    await expect(sheet.getByTestId('release-band')).toContainText('Preparada:');
    // The footer: "Voltar" with its back arrow, "Mudar o local" with none.
    await pickFireballPoint(sheet);
    await sheet.getByRole('application', { name: 'Mapa: escolha o ponto da Bola de Fogo' }).press('Enter');
    await expect(sheet.getByRole('heading', { name: /Quem está na área/ })).toBeFocused();
    // Opening the list does not scroll the band out from under the header.
    await expect.poll(() => sheet.locator('.frame__body').evaluate((el) => el.scrollTop)).toBe(0);
    await expect(sheet.getByTestId('release-band')).toBeInViewport({ ratio: 1 });
    await expect(sheet.getByRole('button', { name: 'Voltar', exact: true }).locator('mat-icon')).toHaveText('arrow_back');
    await expect(sheet.getByRole('button', { name: 'Mudar o local' }).locator('mat-icon')).toHaveCount(0);
    // The master does not mark the window: the player's screen still reads the plain clock.
    expect((await windowOf(m, campaignId))?.rolling ?? false).toBe(false);
    await sheet.getByRole('button', { name: releaseTexts.back, exact: true }).click();
    await expect(sheet).toHaveCount(0);
    await expect(spellWindow(p)).toBeVisible();
    expect((await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Pensantus')?.reactionUsed ?? false).toBe(false);
  } finally {
    await done();
  }
});

test('o movimento preparado: "Mover agora" abre o mapa, "Voltar" volta à janela e "Mover para cá" gasta só a reação', { tag: ['@RN-31', '@RN-22', '@RN-21'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  const { m, p, campaignId, done } = await readyMapTable(browser, 'Mover preparado', 'move');
  try {
    await fireReadied(m);
    const win = moveWindow(p);
    await expect(win).toBeVisible();
    await expect(win.getByTestId('release-map-notice')).toContainText('O mapa abre para você escolher o quadrado.');
    await expect(masterQueue(m).locator('.btns button')).toHaveText(['Deixar passar', 'Mover por Pensantus']);

    await win.getByTestId('release-to-map').click();
    await expect(p.getByRole('heading', { name: 'Mover Pensantus' })).toBeVisible();
    await expect(p.getByTestId('release-band')).toContainText('Ação preparada:');
    await expect(p.getByTestId('release-note')).toContainText('É o movimento preparado: gasta a ■ Reação');
    await expect(p.locator('.move__legend')).toContainText('Você alcança');
    await expect(masterQueue(m).getByTestId('readied-wait')).toContainText(/No mapa · passa sozinho em/);

    // "Voltar" is back at the window, mark cleared.
    await p.getByRole('button', { name: releaseTexts.back, exact: true }).click();
    await expect(win).toBeVisible();
    await expect.poll(async () => (await windowOf(m, campaignId))?.rolling ?? false).toBe(false);

    // The move: one square to the right of Pensantus; the reaction is spent and the readied action is done.
    const before = (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Pensantus')!;
    await win.getByTestId('release-to-map').click();
    await expect(p.getByRole('heading', { name: 'Mover Pensantus' })).toBeVisible();
    await tapSquare(p, (before.col ?? 0) + 2, before.row ?? 0);
    // The cost card holds the cost and the note of the readied move; the table's wait is read above the buttons.
    await expect(p.locator('app-move-status .status')).toContainText('É o movimento preparado: gasta a ■ Reação');
    await expect(p.getByTestId('release-wait')).toHaveText(releaseTexts.onMap);
    await p.getByRole('button', { name: 'Mover para cá' }).click();
    await expect.poll(async () => (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Pensantus')?.col).toBe((before.col ?? 0) + 2);
    const after = await getEncounterRPC(m, campaignId);
    expect(after.reactionWindows ?? []).toHaveLength(0);
    expect(after.combatants.find((c) => c.label === 'Pensantus')?.reactionUsed).toBe(true);
  } finally {
    await done();
  }
});

test('o mestre move por Pensantus: o rodapé da página de movimento e a volta pelo "Voltar"', { tag: ['@RN-31', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { m, p, done } = await readyMapTable(browser, 'Mestre move', 'move');
  try {
    await fireReadied(m);
    await expect(moveWindow(p)).toBeVisible();
    await masterQueue(m).getByRole('button', { name: 'Mover por Pensantus' }).click();
    await expect(m.getByTestId('release-band')).toContainText('Ação preparada de Pensantus:');
    await expect(m.getByTestId('release-foot')).toHaveText(releaseTexts.masterFoot);
    // The legend names the mover when the master moves for someone; the player's own page says "Você alcança".
    await expect(m.locator('.move__legend')).toContainText('Pensantus alcança');
    await m.getByRole('button', { name: releaseTexts.back, exact: true }).click();
    await expect(m.getByTestId('release-foot')).toHaveCount(0);
    await expect(moveWindow(p)).toBeVisible();
  } finally {
    await done();
  }
});

test('o mestre usa o movimento preparado pelo jogador: o movimento gasta a reação e fecha a janela', { tag: ['@RN-31', '@RN-22', '@RN-21'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  const { m, p, campaignId, done } = await readyMapTable(browser, 'Mestre usa o movimento', 'move');
  try {
    await fireReadied(m);
    await expect(moveWindow(p)).toBeVisible();
    const before = (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Pensantus')!;
    await masterQueue(m).getByRole('button', { name: 'Mover por Pensantus' }).click();
    await expect(m.getByTestId('release-band')).toContainText('Ação preparada de Pensantus:');
    await tapSquare(m, (before.col ?? 0) + 2, before.row ?? 0);
    await m.getByRole('button', { name: 'Mover para cá' }).click();
    await expect.poll(async () => (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Pensantus')?.col).toBe((before.col ?? 0) + 2);
    const after = await getEncounterRPC(m, campaignId);
    expect(after.reactionWindows ?? []).toHaveLength(0);
    expect(after.combatants.find((c) => c.label === 'Pensantus')?.reactionUsed).toBe(true);
  } finally {
    await done();
  }
});

test('o mestre arrasta Pensantus com o movimento preparado na janela: é um passo comum, a janela continua aberta e a reação livre', { tag: ['@RN-31', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  const { m, p, campaignId, done } = await readyMapTable(browser, 'Mestre arrasta', 'move');
  try {
    await fireReadied(m);
    await expect(moveWindow(p)).toBeVisible();
    const before = (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Pensantus')!;
    const map = m.getByRole('group', { name: /Mapa de batalha/ });
    const box = (await map.boundingBox())!;
    const from = (await m.locator(`.cm__tk[data-id="${before.id}"]`).boundingBox())!;
    await m.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
    await m.mouse.down();
    await m.mouse.move(box.x + (((before.col ?? 0) + 2.5) * box.width) / 20, box.y + (((before.row ?? 0) + 0.5) * box.height) / 14, { steps: 8 });
    await m.mouse.up();
    await expect.poll(async () => (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Pensantus')?.col).toBe((before.col ?? 0) + 2);
    const after = await getEncounterRPC(m, campaignId);
    expect(after.reactionWindows ?? []).toHaveLength(1);
    expect(after.combatants.find((c) => c.label === 'Pensantus')?.reactionUsed ?? false).toBe(false);
    await expect(moveWindow(p)).toBeVisible();
  } finally {
    await done();
  }
});

test('o movimento preparado barrado no primeiro quadrado: a página fica aberta com "Algo bloqueia o caminho", nada é gasto e outro quadrado vale', { tag: ['@RN-31', '@RN-22', '@RN-21'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  const { m, p, campaignId, encounter, done } = await readyMapTable(browser, 'Movimento barrado', 'move', { blocker: true });
  try {
    const at = encounter.combatants.find((c) => c.label === 'Pensantus')!;
    await fireReadied(m);
    const win = moveWindow(p);
    await expect(win).toBeVisible();
    await win.getByTestId('release-to-map').click();
    await expect(p.getByRole('heading', { name: 'Mover Pensantus' })).toBeVisible();
    await tapSquare(p, (at.col ?? 0) + 2, at.row ?? 0);
    await p.getByRole('button', { name: 'Mover para cá' }).click();
    await expect(p.locator('.mr-notice--danger')).toContainText('Algo bloqueia o caminho: nada foi gasto. Escolha outro quadrado.');
    await expect(p.getByRole('heading', { name: 'Mover Pensantus' })).toBeVisible();
    const kept = await getEncounterRPC(m, campaignId);
    expect(kept.reactionWindows ?? []).toHaveLength(1);
    expect(kept.combatants.find((c) => c.label === 'Pensantus')?.col).toBe(at.col);
    expect(kept.combatants.find((c) => c.label === 'Pensantus')?.reactionUsed ?? false).toBe(false);
    // Another square, clear of the blocker, is the use.
    await tapSquare(p, at.col ?? 0, (at.row ?? 0) - 2);
    await p.getByRole('button', { name: 'Mover para cá' }).click();
    await expect.poll(async () => (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Pensantus')?.row).toBe((at.row ?? 0) - 2);
    // Leaving the blocker's reach opens an opportunity attack of its own: the readied window is the one that is gone.
    const used = await getEncounterRPC(m, campaignId);
    expect((used.reactionWindows ?? []).filter((w) => w.kind === 'REACTION_KIND_READIED')).toHaveLength(0);
    expect(used.combatants.find((c) => c.label === 'Pensantus')?.reactionUsed).toBe(true);
  } finally {
    await done();
  }
});

test('o mestre desfaz o gatilho com o mapa aberto: a folha diz que a Bola de Fogo continua preparada e sai com "Voltar à sessão"', { tag: ['@RN-31', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  const { m, p, campaignId, done } = await readyMapTable(browser, 'Gatilho desfeito', 'fireball');
  try {
    await fireReadied(m);
    await spellWindow(p).getByTestId('release-to-map').click();
    const sheet = releaseSheet(p);
    await expect(sheet).toBeVisible();

    await undoFire(m, campaignId);

    const gone = sheet.getByTestId('release-gone');
    await expect(gone).toContainText(releaseTexts.undone);
    await expect(sheet.getByTestId('release-done')).toHaveText(releaseTexts.done);
    await sheet.getByTestId('release-done').click();
    await expect(sheet).toHaveCount(0);

    // Nothing was spent and the spell still waits for the next trigger.
    const after = await getEncounterRPC(m, campaignId);
    expect(after.combatants.find((c) => c.label === 'Pensantus')?.reactionUsed ?? false).toBe(false);
    expect(after.combatants.map(readiedOf).filter((r) => r !== undefined && r.status === 'READIED_STATUS_WAITING')).toHaveLength(1);
    await expect(p.getByRole('region', { name: /Ação preparada: Bola de Fogo/ })).toBeVisible();
  } finally {
    await done();
  }
});
