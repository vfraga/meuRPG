import { expect } from '@playwright/test';
import { test } from './diagnostics';
import { attackCombatSquares, beginAttackCombatRPC, combatRPC, getEncounterRPC, tableForCombat } from './combat-support';
import { endSessionOf } from './feature-use-support';
import { goblinTrigger, openReadySheet, pensantusReadyKit, readiedOf } from './ready-support';
import { openSessionPage } from './live-session-support';
import { waitForEncounterRevision } from './revision-support';
import { newSignedInContext, openSpentGroup } from './support';

/**
 * "Preparar" (#582) through the real stack, the player's side: the Ready action of "Outras" opens its sheet, the trigger is counted and
 * refused over 200 characters before anything is sent, the call holds the action and the card of the held action sits under the economy row
 * with the trigger quoted. "Desistir da ação preparada", in the card's own menu, gives it up: the action spent does not come back.
 */
test('um jogador prepara um ataque, lê o cartão e desiste dele na própria vez', { tag: ['@RN-31', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
  const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 900 } });
  const m = await master.newPage();
  const p = await player.newPage();
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Preparar ${Date.now()}`, true, true, { sheet: pensantusReadyKit });
    campaignId = table.campaignId;
    await beginAttackCombatRPC(m, table, { Pensantus: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 }, attackCombatSquares);

    await openSessionPage(p, campaignId);
    await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
    const sheet = await openReadySheet(p);
    await expect(sheet.getByText('Pensantus · fica guardada até o gatilho')).toBeVisible();
    await expect(sheet.getByRole('button', { name: 'Preparar', exact: true })).toBeDisabled();

    // The trigger is counted, and over 200 characters the field says so before anything is sent.
    const field = sheet.getByLabel('Quando dispara, nas suas palavras');
    await field.fill('a'.repeat(231));
    await expect(sheet.getByText('231/200')).toBeVisible();
    await expect(sheet.getByText('Até 200 caracteres.')).toBeVisible();
    await expect(sheet.getByRole('button', { name: 'Preparar', exact: true })).toBeDisabled();

    await sheet.locator('label', { hasText: 'Adaga' }).click();
    await sheet.getByLabel('Em quem (opcional)').selectOption({ label: 'Capitão Goblin' });
    await field.fill(goblinTrigger);
    await expect(sheet.getByText(`${goblinTrigger.length}/200`)).toBeVisible();
    await sheet.getByRole('button', { name: 'Preparar', exact: true }).click();
    await expect(sheet).toHaveCount(0);

    // The card sits under the economy row with the trigger quoted, and the server holds the row.
    const card = p.getByRole('region', { name: 'Ação preparada: Adaga no Capitão Goblin' });
    await expect(card).toBeVisible();
    await expect(card.getByText(`“${goblinTrigger}”`)).toBeVisible();
    await expect(card.getByText('Gasta a reação quando o mestre disparar · se perde no começo da sua vez')).toBeVisible();
    const held = (await getEncounterRPC(m, campaignId)).combatants.map(readiedOf).find((r) => r !== undefined);
    expect(held, 'the master reads the readied action of Pensantus').toMatchObject({ triggerText: goblinTrigger, kind: 'READIED_KIND_ATTACK', actionKey: 'equipment:dagger' });

    // The action is spent: "Preparar" waits in the folded Action, off.
    const spentGroup = await openSpentGroup(p, 'Ação');
    await expect(spentGroup.getByRole('button', { name: 'Preparar uma ação' })).toBeDisabled();

    // "Desistir" is in the card's menu; the action does not come back.
    await card.getByRole('button', { name: 'Mais ações da ação preparada' }).click();
    await p.getByRole('menuitem', { name: /Desistir da ação preparada/ }).click();
    await expect(card).toHaveCount(0);
    const after = await getEncounterRPC(m, campaignId);
    expect(after.combatants.map(readiedOf).filter((r) => r !== undefined)).toEqual([]);
    expect(after.combatants.find((c) => c.label === 'Pensantus')?.actionUsed).toBe(true);
  } finally {
    await endSessionOf(m, campaignId);
    await master.close();
    await player.close();
  }
});

/**
 * A spell is held with its slot (boards 582-02): the sheet lists the spells of 1 action with the level, writes the reason in the card of the
 * others (Escudo Arcano is a reaction), warns that the held spell is the concentration, and the card marks it. The server's refusal, when the
 * action was spent meanwhile, is written in the red band of the sheet with what was typed kept.
 */
test('um jogador prepara uma magia com o espaço e o servidor recusa quando a ação já foi usada', { tag: ['@RN-31', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
  const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 900 } });
  const m = await master.newPage();
  const p = await player.newPage();
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Preparar magia ${Date.now()}`, true, true, { sheet: pensantusReadyKit });
    campaignId = table.campaignId;
    const begun = await beginAttackCombatRPC(m, table, { Pensantus: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 }, attackCombatSquares);

    await openSessionPage(p, campaignId);
    await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
    const sheet = await openReadySheet(p);
    // Mover is on in a combat with a map (#856): what it does is written, and the trigger is all it needs to be held.
    await sheet.getByRole('button', { name: 'Mover', exact: true }).click();
    await expect(sheet.getByText(/Você anda até\s.+ quando o mestre disparar\./)).toBeVisible();
    await sheet.getByLabel('Quando dispara, nas suas palavras').fill('Quando o goblin recuar, corro até a porta');
    await expect(sheet.getByRole('button', { name: 'Preparar', exact: true })).toBeEnabled();
    await sheet.getByRole('button', { name: 'Magia', exact: true }).click();
    const shield = sheet.locator('label', { hasText: 'Escudo Arcano' });
    await expect(shield).toContainText('É uma reação: só uma magia de 1 ação pode ser preparada.');
    // The window releases every spell on the map (#857): a sleep, a cone and the darts are all listed on, none with a reason to be off.
    for (const name of ['Sono', 'Mãos Flamejantes', 'Mísseis Mágicos']) {
      await expect(sheet.locator('label', { hasText: name })).not.toContainText('Soltar pela janela');
    }
    await expect(sheet.getByRole('radio', { name: /Sono/ })).toBeEnabled();
    // A spell of one creature stays on: the level to hold it with, and the warning that it is the concentration.
    const charm = sheet.locator('label', { hasText: 'Enfeitiçar Pessoa' });
    await expect(charm).not.toContainText('Soltar pela janela');
    await charm.click();
    await expect(sheet.getByRole('group', { name: 'Com que espaço' })).toBeVisible();
    await expect(sheet.getByText(/Ação e espaço de 1º\s+nível/)).toBeVisible();
    const field = sheet.getByLabel('Quando dispara, nas suas palavras');
    await field.fill('Se o goblin sair da sombra do pilar');
    const go = sheet.getByRole('button', { name: 'Preparar Enfeitiçar Pessoa' });
    await expect(go).toBeEnabled();

    // The action is spent meanwhile: the server refuses, the sheet says why and keeps what was typed.
    await waitForEncounterRevision(p, await combatRPC(m, 'TakeAction', { campaignId, encounterId: begun.id, combatantId: begun.combatants.find((c) => c.label === 'Pensantus')!.id, actionKey: 'standard:dodge' }));
    await go.click();
    await expect(sheet.getByRole('alert')).toContainText('Sua ação já foi usada neste turno.');
    await expect(field).toHaveValue('Se o goblin sair da sombra do pilar');
    await expect(sheet).toBeVisible();
    await sheet.getByRole('button', { name: 'Cancelar' }).click();
    await expect(sheet).toHaveCount(0);
    expect((await getEncounterRPC(m, campaignId)).combatants.map(readiedOf).filter((r) => r !== undefined)).toEqual([]);
  } finally {
    await endSessionOf(m, campaignId);
    await master.close();
    await player.close();
  }
});
