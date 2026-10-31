import { expect, test } from '@playwright/test';

import { combatRPC, endTurnOf, passTurnsTo, passTurnsToAndWait, startEncounterRPC, tableForCombat } from './combat-support';
import { grog, rollsTable, vex } from './combat-rolls-support';
import { alliedTable, closeSheet, contestRPC, idOf, typeD20 } from './contests-support';
import { endOpenSessionRPC, openSessionPage } from './live-session-support';
import { newSignedInContext, openActionTab, viaTargetList } from './support';
import { waitForEncounterRevision } from './revision-support';

test('o jogador agarra o goblin (o mestre responde), é agarrado por ele e escapa: nunca lê o total do NPC', { tag: ['@RN-34', '@RN-10', '@RN-20'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const { m, p, campaignId, done } = await rollsTable(browser, 'Agarrão', grog, {}, { Grog: 20, 'Goblin 1': 5, 'Capitão Goblin': 4, 'Goblin 2': 3 });
  try {
    await openSessionPage(p, campaignId);
    await expect(p.getByRole('heading', { name: 'Sua vez, Grog' })).toBeVisible();
    await openActionTab(p, 'Ataques');
    await p.getByRole('button', { name: /Agarrar: escolher o alvo/ }).click();
    const sheet = p.getByRole('dialog');
    await viaTargetList(p, sheet);
    await sheet.getByText('Tenho uma mão livre').click();
    await sheet.locator('label', { hasText: 'Goblin 1' }).click();
    await sheet.getByRole('button', { name: 'Rolar a disputa' }).click();
    await typeD20(sheet, 20);
    await expect(sheet.getByText(/Esperando o mestre/)).toBeVisible();
    // The master answers for the Goblin: the initiator's total is his to read.
    await openSessionPage(m, campaignId);
    const card = m.getByTestId('contest-card');
    await expect(card).toContainText('20 + 5 = 25');
    await card.locator('label', { hasText: 'Atletismo' }).click();
    await typeD20(card, 3);

    // The player reads only the win, never the Goblin's total (RN-20).
    await expect(sheet.getByText('Você venceu a disputa.')).toBeVisible();
    await expect(sheet).toContainText('Goblin 1');
    await expect(sheet).toContainText('Agarrado');
    await expect(sheet).not.toContainText('1d20 (3)');
    await closeSheet(sheet);
    const holds = m.getByTestId('grapples');
    await expect(holds).toContainText('Grog segura o Goblin 1');

    // The master lets the Goblin go; later the Goblin grapples Grog, who chooses the skill and rolls.
    await holds.getByRole('button', { name: 'Soltar Goblin 1' }).click();
    await expect(holds).toBeHidden();
    // The player's end of turn lands first: passing the turns before it would end Grog's turn twice.
    await endTurnOf(p, m, campaignId, 'Grog');
    const enc = await passTurnsToAndWait(m, campaignId, 'Goblin 1', [m, p]);
    await contestRPC(m, 'StartContest', {
      campaignId,
      encounterId: enc.id,
      initiatorId: idOf(enc, 'Goblin 1'),
      targetId: idOf(enc, 'Grog'),
      purpose: 'CONTEST_PURPOSE_GRAPPLE',
      roll: { d20Faces: { faces: [18] } },
    });
    const answer = p.getByRole('dialog');
    await expect(answer).toContainText('Goblin 1 tenta agarrar você');
    await answer.locator('label', { hasText: 'Acrobacia' }).click();
    await typeD20(answer, 2);
    await expect(answer.getByText(/Você perdeu a disputa/)).toBeVisible();
    await expect(answer).not.toContainText('18');
    await closeSheet(answer);

    // Grappled: on her turn the speed is 0 and "Escapar" is the first line of the action; the master answers for the Goblin.
    await passTurnsToAndWait(m, campaignId, 'Grog', [m, p]);
    await expect(p.getByRole('heading', { name: 'Sua vez, Grog' })).toBeVisible();
    await expect(p.getByText('Agarrado').first()).toBeVisible();
    await openActionTab(p, 'Ataques');
    await p.getByRole('button', { name: 'Escapar do agarrão' }).click();
    const escape = p.getByRole('dialog');
    await escape.locator('label', { hasText: 'Acrobacia' }).click();
    await typeD20(escape, 19);
    await expect(escape.getByText(/Esperando o mestre/)).toBeVisible();
    const against = m.getByTestId('contest-card');
    await expect(against).toContainText('Grog tenta escapar');
    await typeD20(against, 2);
    await expect(escape.getByText(/Você se soltou/)).toBeVisible();
    await expect(escape).not.toContainText('1d20 (2)');
    await closeSheet(escape);
    await expect(p.getByText('Agarrado')).toHaveCount(0);
  } finally {
    await done().catch(() => undefined);
  }
});

test('o jogador derruba o goblin com um empurrão e escapa de um agarrão de CD fixa sem ler a CD', { tag: ['@RN-34', '@RN-10', '@RN-20'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const { m, p, campaignId, done } = await rollsTable(browser, 'Empurrão', grog, {}, { Grog: 20, 'Goblin 1': 5, 'Capitão Goblin': 4, 'Goblin 2': 3 });
  try {
    await openSessionPage(p, campaignId);
    await expect(p.getByRole('heading', { name: 'Sua vez, Grog' })).toBeVisible();
    await openActionTab(p, 'Ataques');
    await p.getByRole('button', { name: /Empurrar: escolher o alvo/ }).click();
    const sheet = p.getByRole('dialog');
    await viaTargetList(p, sheet);
    // A shove needs no free hand (SRD 5.1): the sheet does not ask for one.
    await expect(sheet.getByText('Tenho uma mão livre')).toHaveCount(0);
    await sheet.locator('label', { hasText: 'Goblin 1' }).click();
    await sheet.getByRole('button', { name: 'Rolar a disputa' }).click();
    await typeD20(sheet, 20);
    await openSessionPage(m, campaignId);
    const card = m.getByTestId('contest-card');
    await card.locator('label', { hasText: 'Atletismo' }).click();
    await typeD20(card, 2);

    // The winner chooses what the shove does; the push is the server's square.
    await expect(sheet.getByText('Você venceu a disputa.')).toBeVisible();
    await sheet.locator('label', { hasText: 'Derrubar' }).click();
    await sheet.getByRole('button', { name: 'Confirmar' }).click();
    await expect(sheet.getByRole('button', { name: 'Confirmar' })).toBeHidden();
    await expect(sheet).not.toContainText('1d20 (2)');
    await closeSheet(sheet);

    // A grapple with a fixed escape DC (an attack of the creature): the player reads her own total and the result, never the DC.
    // The player's end of turn lands first: passing the turns before it would end Grog's turn twice.
    await endTurnOf(p, m, campaignId, 'Grog');
    const enc = await passTurnsTo(m, campaignId, 'Goblin 1');
    await contestRPC(m, 'StartContest', {
      campaignId,
      encounterId: enc.id,
      initiatorId: idOf(enc, 'Goblin 1'),
      targetId: idOf(enc, 'Grog'),
      purpose: 'CONTEST_PURPOSE_GRAPPLE',
      kind: 'CONTEST_KIND_ESCAPE_DC',
      escapeDc: 16,
    });
    await expect(m.getByTestId('grapples')).toContainText('CD de escape 16');
    await passTurnsToAndWait(m, campaignId, 'Grog', [m, p]);
    await openActionTab(p, 'Ataques');
    await p.getByRole('button', { name: 'Escapar do agarrão' }).click();
    const escape = p.getByRole('dialog');
    await escape.locator('label', { hasText: 'Atletismo' }).click();
    await typeD20(escape, 20);
    await expect(escape.getByText('Você escapou.')).toBeVisible();
    await expect(escape).not.toContainText('16');
    await expect(p.locator('body')).not.toContainText(/CD de escape/);
  } finally {
    await done().catch(() => undefined);
  }
});

test('a ladina se esconde com a Ação Ardilosa sem ler quem notou, e ajuda um aliado', { tag: ['@RN-34', '@RN-10', '@RN-20'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const { m, p, campaignId, done } = await alliedTable(browser, 'Esconder', vex, { weaponKeys: ['equipment:shortsword'] }, { Vex: 20, 'Goblin 1': 5, Tavo: 4, 'Goblin 2': 3 });
  try {
    await openSessionPage(p, campaignId);
    await expect(p.getByRole('heading', { name: 'Sua vez, Vex' })).toBeVisible();
    await openActionTab(p, 'Ladino');
    await p.getByRole('button', { name: 'Usar Ação Ardilosa: Esconder' }).click();
    const sheet = p.getByRole('dialog');
    await expect(sheet).toContainText(/Você não se esconde de quem [oa] vê claramente/);
    await typeD20(sheet, 20);
    await expect(sheet.getByText(/Esperando o mestre/)).toBeVisible();

    // The master sees the total against each passive Perception and decides; the player reads one sentence.
    await openSessionPage(m, campaignId);
    const card = m.getByTestId('hide-card');
    await expect(card).toContainText('Percepção passiva');
    await card.getByRole('button', { name: /^Aplicar/ }).click();
    await expect(sheet.getByText(/Você está escondid/)).toBeVisible();
    await expect(sheet).not.toContainText(/Percepção passiva|notou|nota /);
    await closeSheet(sheet);
    await expect(p.getByText(/Escondid/).first()).toBeVisible();
    await expect(p.locator('body')).not.toContainText('Percepção passiva');

    // Help: an ally and a task (the action).
    await openActionTab(p, 'Outras');
    await p.getByRole('button', { name: 'Ajudar', exact: true }).click();
    const help = p.getByRole('dialog');
    await help.locator('label', { hasText: 'Tavo' }).click();
    await help.getByRole('button', { name: 'Continuar' }).click();
    await help.locator('label', { hasText: 'Percepção' }).click();
    await help.getByRole('button', { name: /^Ajudar Tavo/ }).click();
    await expect(help.getByRole('button', { name: /^Ajudar Tavo/ })).toBeHidden();
    await expect(help).toContainText('Você ajudou Tavo. Ele terá vantagem no próximo teste de Percepção.');
    await closeSheet(help);
  } finally {
    await done().catch(() => undefined);
  }
});

test('o teste em grupo é encerrado pelo mestre sem mostrar a CD, e a surpresa marcada aparece só para quem a sofre', { tag: ['@RN-34', '@RN-10', '@RN-20'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
  const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 390, height: 844 } });
  const m = await master.newPage();
  const p = await player.newPage();
  await m.goto('/');
  await p.goto('/');
  const table = await tableForCombat(m, p, `Grupo ${Date.now()}`, true, true, { build: vex, sheet: { weaponKeys: ['equipment:shortsword'] } });
  const { campaignId } = table;
  try {
    await openSessionPage(m, campaignId);
    await openSessionPage(p, campaignId);

    // The master asks for a Stealth check with a DC he keeps to himself; the player rolls and waits.
    await m.getByRole('button', { name: 'Pedir teste em grupo' }).click();
    const ask = m.getByTestId('group-ask');
    await ask.getByLabel('Teste', { exact: true }).selectOption({ label: 'Furtividade' });
    await ask.getByLabel(/^CD \(/).fill('13');
    await ask.getByRole('button', { name: 'Pedir o teste' }).click();
    // The sheet opens by itself on the player's screen.
    const sheet = p.getByRole('dialog');
    // A party of one: "Todos" is one character, so it is a check of its own (a group needs two or more).
    await expect(sheet).toContainText('O mestre pede um teste de Furtividade.');
    await typeD20(sheet, 15);
    await expect(sheet.getByText(/Esperando o mestre/)).toBeVisible();
    await expect(sheet).not.toContainText(/CD/);

    const open = m.getByTestId('group-open');
    await expect(open).toContainText('1 de 1');
    await expect(open).not.toContainText('Veredito do grupo');
    await open.getByRole('button', { name: 'Encerrar o teste' }).click();
    await expect(sheet.getByText('O mestre encerrou o teste.')).toBeVisible();
    await expect(p.locator('body')).not.toContainText(/CD 13|CD: 13/);
    await closeSheet(sheet);

    // The combat opens: the master marks who is surprised in the start screen, the player reads only her own state.
    let enc = await startEncounterRPC(m, table, [{ characterId: table.goblinId, count: 2, hidden: false }]);
    await waitForEncounterRevision(m, enc);
    const card = m.getByTestId('surprise-card');
    await expect(card).toContainText('Quem está surpreso?');
    await card.locator('label:has(input[aria-label="Surpreso: Vex"])').click();
    await expect(card.getByLabel('Surpreso: Vex')).toBeChecked();
    for (const c of enc.combatants) {
      enc = await combatRPC(m, 'SubmitInitiative', { campaignId, encounterId: enc.id, combatantId: c.id, d20Face: c.label === 'Vex' ? 20 : 3 });
    }
    for (const [label, [col, row]] of Object.entries({ Vex: [8, 9], 'Goblin 1': [9, 9], 'Goblin 2': [14, 10] })) {
      enc = await combatRPC(m, 'MoveCombatant', { campaignId, encounterId: enc.id, combatantId: idOf(enc, label), col, row });
    }
    await combatRPC(m, 'BeginCombat', { campaignId, encounterId: enc.id });
    await expect(p.getByRole('heading', { name: 'Sua vez, Vex' })).toBeVisible();
    await expect(p.getByText(/Você está surpres[oa] neste turno/)).toBeVisible();
    await expect(p.getByRole('button', { name: 'Passar o turno' })).toBeVisible();
    for (const line of ['Atacar', 'Movimento', 'Ação bônus', 'Reação']) {
      await expect(p.getByText(line, { exact: true }).first()).toBeVisible();
    }
    await expect(p.getByText(/Surpresa/).first()).toBeVisible();
  } finally {
    await endOpenSessionRPC(m, campaignId).catch(() => undefined);
    await master.close();
    await player.close();
  }
});
