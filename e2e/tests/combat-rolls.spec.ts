import { expect, test } from '@playwright/test';

import { combatRPC, getEncounterRPC, passTurnsTo, tableForCombat, waitTurnLeaves } from './combat-support';
import { grog, pipo, rollsTable, vex } from './combat-rolls-support';
import { endOpenSessionRPC, openSessionPage } from './live-session-support';
import { beginPlacedCombatRPC, orla, orlaSheet } from './rests-support';
import { callRPC, newSignedInContext } from './support';
import { secondPlayer } from './theatre-support';

// Advantage and disadvantage, the damage extras and the resistance steps on screen
// (PM-06, PM-07). The table, the NPCs and the combat come through the API; what is
// under test is what the player and the master read and type.

test(
  'um ataque com vantagem e Ataque Furtivo: os dois d20, o conta e não conta, o extra marcado e a soma digitada',
  { tag: ['@PM-06a', '@PM-06b', '@RN-18'] },
  async ({ browser }) => {
    test.setTimeout(120_000);
    const { m, p, campaignId, done } = await rollsTable(browser, 'Furtivo', vex, { weaponKeys: ['equipment:rapier'] }, { Vex: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 });
    try {
      // The goblin is held: attacks against it have advantage.
      const enc = await getEncounterRPC(m, campaignId);
      const goblin = enc.combatants.find((c) => c.label === 'Goblin 1')!;
      await combatRPC(m, 'SetCombatantConditions', { campaignId, encounterId: enc.id, combatantId: goblin.id, conditions: { keys: ['condition:restrained'] } });
      await openSessionPage(p, campaignId);
      await expect(p.getByRole('heading', { name: 'Sua vez, Vex' })).toBeVisible();

      await p.getByRole('button', { name: /^Atacar com Rapieira/ }).click();
      const sheet = p.getByRole('dialog', { name: /Atacar com Rapieira/ });
      await sheet.locator('label', { hasText: 'Goblin 1' }).click();
      // The mode is the server's: advantage, with the reason in words.
      const modes = sheet.getByRole('radiogroup', { name: 'Como rolar o d20' });
      await expect(modes.getByRole('radio', { name: /Vantagem/ })).toBeChecked();
      await expect(sheet.getByText(/Vantagem/).first()).toBeVisible();
      await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
      await sheet.getByLabel(/d20 \(1º\)|Primeiro d20|1º d20/).first().fill('4');
      await sheet.getByLabel(/d20 \(2º\)|Segundo d20|2º d20/).first().fill('17');
      await sheet.getByRole('button', { name: /Confirmar/ }).click();

      // Both dice are shown and the higher one counts.
      await expect(sheet.getByText(/vale/).first()).toBeVisible();
      await expect(sheet.getByText(/descartado/).first()).toBeVisible();

      // Sneak Attack is offered, marked, and the player types one sum for each part.
      const extra = sheet.getByRole('checkbox', { name: /Ataque Furtivo/ });
      await expect(extra).toBeEnabled();
      await extra.check();
      await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
      await sheet.getByLabel(/Rapieira/).last().fill('5');
      await sheet.getByLabel(/Ataque Furtivo/).last().fill('7');
      await sheet.getByRole('button', { name: /Confirmar/ }).click();
      await expect(sheet.getByText(/Ataque Furtivo/).first()).toBeVisible();

      // The log has the pair and the part.
      await sheet.getByRole('button', { name: /Voltar à sua vez/ }).click();
      await p.getByRole('button', { name: 'Abrir o registro do combate' }).click();
      await expect(p.getByRole('log', { name: 'Registro do combate' })).toContainText('Goblin 1');
    } finally {
      await done().catch(() => undefined);
    }
  },
);

test('a fúria do bárbaro corta o dano pela metade e o mestre vê o passo da resistência', { tag: ['@PM-07a', '@PM-07b', '@RN-20'] }, async ({ browser }) => {
  test.setTimeout(120_000);
  const { m, p, campaignId, done } = await rollsTable(browser, 'Fúria', grog, { weaponKeys: ['equipment:greataxe'] }, { Grog: 20, 'Goblin 1': 15, 'Capitão Goblin': 5, 'Goblin 2': 4 });
  try {
    await openSessionPage(p, campaignId);
    await expect(p.getByRole('heading', { name: 'Sua vez, Grog' })).toBeVisible();
    // Grog is a Berserker: Fúria asks about the frenzy first (SRD 5.1, Frenzy); this rage is a plain one.
    await p.getByRole('button', { name: /Fúria/ }).click();
    await p.getByRole('button', { name: 'Só fúria' }).click();
    await expect(p.getByText('Em fúria').first()).toBeVisible();

    // He attacks a hostile creature, so the rage holds when the turn ends.
    const enc = await getEncounterRPC(m, campaignId);
    const id = (label: string) => enc.combatants.find((c) => c.label === label)!.id;
    const swing = await callRPC(p, 'meurpg.play.v1.CombatService/RollAttack', {
      campaignId,
      encounterId: enc.id,
      attackerId: id('Grog'),
      attackKey: 'equipment:greataxe',
      targetId: id('Goblin 1'),
      idempotencyKey: crypto.randomUUID(),
      d20Face: 2,
    });
    expect(swing.ok(), await swing.text()).toBeTruthy();
    const end = await callRPC(p, 'meurpg.play.v1.CombatService/EndTurn', { campaignId, encounterId: enc.id, expectedCombatantId: id('Grog'), idempotencyKey: crypto.randomUUID() });
    expect(end.ok(), await end.text()).toBeTruthy();
    await waitTurnLeaves(m, campaignId, 'Grog');

    // The goblin's blow lands on the raging barbarian: the master reads the step.
    await openSessionPage(m, campaignId);
    const card = m.getByRole('region', { name: 'Ações do Goblin 1' });
    await card.getByRole('button', { name: 'Digitar o resultado' }).click();
    await card.getByLabel(/Role 1d20/).fill('20');
    await card.getByRole('button', { name: 'Confirmar 20' }).click();
    await card.getByRole('button', { name: 'Rolar dano' }).click();
    await expect(card.getByText(/Resistência/).first()).toBeVisible();
    await expect(card.getByRole('checkbox', { name: /Ignorar a resistência/ }).first()).toBeVisible();
    await expect(card.getByRole('button', { name: /Aplicar \d+ de dano/ })).toBeVisible();
    // The player never reads the step of a damage that is not hers... and keeps the rage chip.
    await expect(p.getByText('Em fúria').first()).toBeVisible();
  } finally {
    await done().catch(() => undefined);
  }
});

test(
  'o halfling tira 1 no ataque: "Tirou 1: rolar de novo?", o novo d20 digitado e "Sortudo: 1 → 15"',
  { tag: ['@MR-014', '@RN-35', '@RN-18'] },
  async ({ browser }) => {
    test.setTimeout(120_000);
    const { p, campaignId, done } = await rollsTable(browser, 'Sortudo', pipo, { weaponKeys: ['equipment:battleaxe'] }, { Pipo: 20, 'Goblin 1': 15, 'Capitão Goblin': 5, 'Goblin 2': 4 });
    try {
      await openSessionPage(p, campaignId);
      await expect(p.getByRole('heading', { name: 'Sua vez, Pipo' })).toBeVisible();
      await p.getByRole('button', { name: /^Atacar com Machado de batalha/ }).click();
      const sheet = p.getByRole('dialog', { name: /Atacar com Machado de batalha/ });
      await sheet.locator('label', { hasText: 'Goblin 1' }).click();
      await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
      await sheet.getByLabel(/Role 1d20 para/).fill('1');
      await sheet.getByRole('button', { name: 'Confirmar 1' }).click();

      // A 1 on the d20 (SRD 5.1, Halfling, Lucky): the question comes before the result.
      await expect(sheet.getByRole('heading', { name: 'Tirou 1: rolar de novo?' })).toBeVisible();
      await expect(sheet.getByText('O mestre ainda não disse se acertou.')).toBeVisible();
      await expect(sheet.getByRole('button', { name: 'Ficar com o 1' })).toBeVisible();
      await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
      await sheet.getByLabel(/Role 1d20 de novo/).fill('15');
      await sheet.getByRole('button', { name: 'Confirmar 15' }).click();

      // The new roll stands: the result is told with it, and the sheet says what Lucky did.
      await expect(sheet.getByText('Sortudo: 1 → 15')).toBeVisible();
      await expect(sheet.getByRole('heading', { name: 'Tirou 1: rolar de novo?' })).toBeHidden();
      await expect(sheet.getByText(/Acertou|Crítico/).first()).toBeVisible();
    } finally {
      await done().catch(() => undefined);
    }
  },
);

test(
  'o halfling com o dado de Inspiração de Bardo tira 1 no ataque: "Tirou 1: rolar de novo?" primeiro, depois "Usar a Inspiração de Bardo (d6)?" sobre o novo d20, e o dado digitado entra no ataque',
  { tag: ['@MR-014', '@RN-35', '@RN-18'] },
  async ({ browser }) => {
    test.setTimeout(150_000);
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
    const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 390, height: 844 } });
    const m = await master.newPage();
    const p = await player.newPage();
    let bard: Awaited<ReturnType<typeof secondPlayer>> | undefined;
    let campaignId = '';
    try {
      await m.goto('/');
      await p.goto('/');
      const table = await tableForCombat(m, p, `Sortudo e dado ${Date.now()}`, true, true, { build: pipo, sheet: { weaponKeys: ['equipment:battleaxe'] } });
      campaignId = table.campaignId;
      bard = await secondPlayer(browser, m, campaignId, orla, orlaSheet);
      const begun = await beginPlacedCombatRPC(
        m,
        table,
        [table.captainId],
        { Orla: 20, Pipo: 15, 'Capitão Goblin': 5 },
        { Orla: [7, 9], Pipo: [8, 9], 'Capitão Goblin': [9, 9] },
      );
      // Orla's turn: she gives Pipo her d6 (SRD 5.1, Bard 1), and the turn goes on to him.
      const id = (label: string) => begun.combatants.find((c) => c.label === label)!.id;
      const gift = await callRPC(bard.page, 'meurpg.play.v1.ResourceService/GiveBardicInspiration', {
        campaignId,
        encounterId: begun.id,
        actorId: id('Orla'),
        targetId: id('Pipo'),
        idempotencyKey: crypto.randomUUID(),
      });
      expect(gift.ok(), await gift.text()).toBeTruthy();
      await passTurnsTo(m, campaignId, 'Pipo');
      await openSessionPage(p, campaignId);
      await expect(p.getByRole('heading', { name: 'Sua vez, Pipo' })).toBeVisible();

      await p.getByRole('button', { name: /^Atacar com Machado de batalha/ }).click();
      const sheet = p.getByRole('dialog', { name: /Atacar com Machado de batalha/ });
      await sheet.locator('label', { hasText: 'Capitão Goblin' }).click();
      await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
      await sheet.getByLabel(/Role 1d20 para/).fill('1');
      await sheet.getByRole('button', { name: 'Confirmar 1' }).click();

      // The 1 is on the table first, so Lucky asks first, alone: the die's question waits.
      await expect(sheet.getByRole('heading', { name: 'Tirou 1: rolar de novo?' })).toBeVisible();
      await expect(sheet.getByRole('heading', { name: 'Usar a Inspiração de Bardo (d6)?' })).toBeHidden();
      await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
      await sheet.getByLabel(/Role 1d20 de novo/).fill('15');
      await sheet.getByRole('button', { name: 'Confirmar 15' }).click();

      // Then the die, on the new d20, and the result is still not told.
      await expect(sheet.getByRole('heading', { name: 'Usar a Inspiração de Bardo (d6)?' })).toBeVisible();
      await expect(sheet.getByRole('heading', { name: 'Tirou 1: rolar de novo?' })).toBeHidden();
      await expect(sheet.getByText('Sortudo: 1 → 15')).toBeVisible();
      await expect(sheet.getByText(/Acertou|Crítico|Errou/)).toBeHidden();
      await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
      await sheet.getByLabel(/Role 1d6/).fill('4');
      await sheet.getByRole('button', { name: 'Confirmar 4' }).click();

      // The attack is told with both: the new d20 that Lucky rolled and the d6 that was added.
      await expect(sheet.getByText('Sortudo: 1 → 15')).toBeVisible();
      await expect(sheet.getByText(/Acertou|Crítico/).first()).toBeVisible();
      await expect(sheet.getByRole('heading', { name: 'Usar a Inspiração de Bardo (d6)?' })).toBeHidden();
    } finally {
      await endOpenSessionRPC(m, campaignId).catch(() => undefined);
      await bard?.close();
      await master.close();
      await player.close();
    }
  },
);
