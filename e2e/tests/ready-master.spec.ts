import { expect } from '@playwright/test';
import { test } from './diagnostics';
import { attackCombatSquares, beginAttackCombatRPC, getEncounterRPC, passTurnsToAndWait, pensantusCasting, tableForCombat } from './combat-support';
import { endSessionOf } from './feature-use-support';
import { goblinTrigger, readiedOf, readyAttackRPC } from './ready-support';
import { openSessionPage } from './live-session-support';
import { newSignedInContext } from './support';

/**
 * "Ações preparadas" (#582) through the real stack, the master's side: the row of whoever readied, "⏰ Preparada" in the order, "Disparar" off
 * on the readier's own turn with the reason in the row, and on from the next turn; the fire opens the readier's window, and the row says it.
 */
test('o mestre lê a ação preparada, só a dispara fora da vez de quem preparou e vê a janela aberta', { tag: ['@RN-31', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
  const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 900 } });
  const m = await master.newPage();
  const p = await player.newPage();
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Preparar mestre ${Date.now()}`, true, true, { sheet: pensantusCasting });
    campaignId = table.campaignId;
    const begun = await beginAttackCombatRPC(m, table, { Pensantus: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 }, attackCombatSquares);
    await openSessionPage(p, campaignId);
    await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
    await readyAttackRPC(p, campaignId, begun, 'Pensantus', 'Capitão Goblin');

    await openSessionPage(m, campaignId);
    const fold = m.locator('app-readied-fold');
    await expect(fold.getByRole('button', { name: 'Ações preparadas' })).toBeVisible();
    const row = fold.locator('li.row');
    await expect(row).toContainText('Pensantus · Adaga no Capitão Goblin');
    await expect(row).toContainText(`“${goblinTrigger}”`);
    await expect(m.locator('app-order-list').getByTestId('order-readied')).toHaveText(/Preparada/);

    // On the readier's own turn "Disparar" is off and the row says why.
    await expect(row).toContainText('É a vez de Pensantus: só dispara fora da vez dele.');
    await expect(row.getByRole('button', { name: 'Disparar a ação de Pensantus' })).toHaveAttribute('aria-disabled', 'true');

    // On the next turn it is on; the fire opens the readier's window and the row says so.
    await passTurnsToAndWait(m, campaignId, 'Capitão Goblin', m);
    const fire = row.getByRole('button', { name: 'Disparar a ação de Pensantus' });
    await expect(fire).toBeEnabled();
    await expect(row).not.toContainText('só dispara fora da vez dele');
    await fire.click();
    await expect(row).toContainText('A janela de Pensantus já está aberta: espere a resposta.');
    await expect(fire).toHaveAttribute('aria-disabled', 'true');
    const after = await getEncounterRPC(m, campaignId);
    expect(after.reactionWindows?.map((w) => w.kind)).toContain('REACTION_KIND_READIED');
    expect(after.combatants.map(readiedOf).filter((r) => r !== undefined)).toHaveLength(1);
  } finally {
    await endSessionOf(m, campaignId);
    await master.close();
    await player.close();
  }
});
