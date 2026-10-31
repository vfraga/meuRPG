import { expect, test } from '@playwright/test';

import { combatRPC, endTurnOf, getEncounterRPC, passTurnsTo, passTurnsToAndWait, type Encounter } from './combat-support';
import { beginCreatureCombat, hitAndApply, sessionRoute, tableForCreatureCombat, tapCaveSquare, trapAt } from './creatures-combat-support';
import { closeSheet } from './contests-support';
import { endOpenSessionRPC } from './live-session-support';
import { callRPC, newSignedInContext, openActionTab } from './support';
import { openOrderRow } from './combat-layout';

// MR-037 (a player's creatures in combat and the druid's Wild Shape) and RN-20 (a creature's hit points go only
// to its owner's player and the master; another player gets the state word). The setup goes through the API; the
// screens do what the acceptance criteria are about. Every test makes its own campaign.

const FACES = { Toren: 20, Sálvia: 13, 'Capitão Goblin': 5, 'Goblin 1': 4, 'Goblin 2': 4 };

async function tables(browser: import('@playwright/test').Browser, name: string, options: Parameters<typeof tableForCreatureCombat>[4] = {}) {
  const mc = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 1000 } });
  const pc = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 1000 } });
  const tc = await newSignedInContext(browser, 'E-mail Não Verificado', { viewport: { width: 1280, height: 1000 } });
  const m = await mc.newPage();
  const p = await pc.newPage();
  const t = await tc.newPage();
  await m.goto('/');
  await p.goto('/');
  await t.goto('/');
  let table: Awaited<ReturnType<typeof tableForCreatureCombat>>;
  try {
    // The session starts inside the helper's own try: a setup that fails after it ends the session there.
    table = await tableForCreatureCombat(m, p, t, `${name} ${Date.now()}`, options);
  } catch (err) {
    await mc.close();
    await pc.close();
    await tc.close();
    throw err;
  }
  const done = async () => {
    await endOpenSessionRPC(m, table.campaignId);
    await mc.close();
    await pc.close();
    await tc.close();
  };
  return { m, p, t, table, done };
}

test(
  'Sálvia conjura 2 Lobos atrozes em combate com um d20 só, eles agem juntos, Toren só vê o estado deles e perder a concentração os dispensa',
  { tag: ['@MR-037', '@RN-20'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const { m, p, t, table, done } = await tables(browser, 'Lobos');
    try {
      const campaignId = table.campaignId;
      await beginCreatureCombat(m, table, { Sálvia: p, Toren: t }, FACES, { 'Capitão Goblin': [21, 3], 'Goblin 1': [6, 6], 'Goblin 2': [20, 7] });
      await passTurnsTo(m, campaignId, 'Sálvia');
      await p.goto(sessionRoute(campaignId));
      await expect(p.getByRole('heading', { name: 'Sua vez, Sálvia' })).toBeVisible();
      // Alone, she has no tabs.
      await expect(p.getByRole('tablist', { name: 'O que você joga' })).toHaveCount(0);

      // The sheet: the slot, how many and which, what losing the concentration does, and one d20 for the group.
      await openActionTab(p, 'Magias');
      await p.getByRole('button', { name: 'Conjurar Conjurar Animais' }).click();
      // The dialog's name is its title, which changes to "Lobos atrozes conjurados" with the result.
    const sheet = p.getByRole('dialog');
      await expect(sheet.getByText('Escolha a criatura.')).toBeVisible();
      await sheet.getByText('2 feras de ND 1 ou menos').click();
      await sheet.getByRole('button', { name: 'Escolher Lobo atroz' }).click();
      await sheet.getByRole('button', { name: 'Mais Lobo atroz' }).click();
      await expect(sheet.getByText('Se você perder a concentração, os 2 Lobos atrozes somem.')).toBeVisible();
      await sheet.getByRole('button', { name: 'Digitar o d20 de um dado físico' }).click();
      await sheet.getByLabel('Role 1d20 para a iniciativa das criaturas').fill('8');
      await sheet.getByRole('button', { name: 'Conjurar Animais', exact: true }).click();
      await expect(sheet.getByRole('heading', { name: 'Lobos atrozes conjurados' })).toBeVisible();
      await expect(sheet.getByText('Os 2 Lobos atrozes entram no combate com iniciativa 10 (um d20 para os dois: 8 + 2). Eles agem juntos, depois da Sálvia.')).toBeVisible();
      await expect(sheet.getByText('CA 14 · PV 37 de 37 · 15 m')).toHaveCount(2);
      await sheet.getByRole('button', { name: 'Fechar' }).last().click();

      // Now she plays three combatants: the tabs, and the character's is the one open.
      const tabs = p.getByRole('tablist', { name: 'O que você joga' });
      await expect(tabs.getByRole('tab', { name: /Sálvia/ })).toHaveAttribute('aria-selected', 'true');
      await expect(tabs.getByRole('tab', { name: /Lobos atrozes \(2\)/ })).toContainText('Espera · vez 10');
      await endTurnOf(p, m, campaignId, 'Sálvia');

      // The wolves' turn: the page follows the turn to their tab, one block each, one joint turn.
      await expect(p.getByRole('heading', { name: 'Vez dos seus Lobos atrozes' })).toBeVisible();
      await expect(tabs.getByRole('tab', { name: /Lobos atrozes/ })).toHaveAttribute('aria-selected', 'true');
      await expect(tabs.getByRole('tab', { name: /Sálvia/ })).toContainText('Já agiu');
      const blocks = p.locator('app-creature-block');
      await expect(blocks).toHaveCount(2);
      await expect(blocks.first()).toContainText('PV 37 de 37 · CA 14');

      // Toren sees the group by name and the state words, never a number of theirs (RN-20).
      await t.goto(sessionRoute(campaignId));
      await expect(t.getByRole('heading', { name: 'Vez dos Lobos atrozes da Sálvia' })).toBeVisible();
      // The screen says no number of theirs either: not the hit points, the armor class, nor what they can do.
      const page = t.locator('main');
      for (const text of [/37 de 37/, /PV 37/, /CA 14/, /Atacar com/, /Mover o Lobo/, /Ainda tem/, /Mordida/]) {
        await expect(page.getByText(text)).toHaveCount(0);
      }
      await expect(t.getByRole('button', { name: /Lobo atroz/ })).toHaveCount(0);
      const seen = await getEncounterRPC(t, campaignId);
      const wolfForToren = seen.combatants.find((c) => c.label === 'Lobo atroz 1') as unknown as Record<string, unknown>;
      expect(wolfForToren).toBeTruthy();
      for (const field of ['hitPointsCurrent', 'hitPointsMax', 'armorClass', 'creatureAttack', 'summonGroupId', 'movementLeftDft', 'creatureId', 'initiativeFace', 'initiativeBonus']) {
        expect(wolfForToren[field], field).toBeUndefined();
      }
      expect(wolfForToren['state']).toBe('COMBATANT_STATE_UNHURT');
      expect(wolfForToren['ownerCharacterId']).toBeTruthy();

      // Lobo atroz 1 moves, Lobo atroz 2 attacks the goblin next to it: each has an action and a movement of its own.
      await p.getByRole('button', { name: 'Mover o Lobo atroz 1' }).first().click();
      await expect(p.getByRole('heading', { name: 'Mover Lobo atroz 1' })).toBeVisible();
      // A trap on the square it walks to: the wolf is caught on its own part, and its block says so (9.14's note, for a creature).
      await trapAt(m, campaignId, table.mapId, 'Fosso escondido', 4, 8);
      await tapCaveSquare(p, 4, 8);
      await p.getByRole('button', { name: 'Mover para cá' }).click();
      await expect(p.getByRole('heading', { name: 'Vez dos seus Lobos atrozes' })).toBeVisible();
      await expect(blocks.first()).toContainText('O Lobo atroz 1 caiu na armadilha Fosso escondido.');
      await expect(blocks.first().locator('.tile').nth(1)).not.toContainText('15,0 m de 15,0 m');
      await expect(blocks.nth(1).locator('.tile').nth(1)).toContainText('15,0 m');

      await p.getByRole('button', { name: 'Atacar com Mordida: Lobo atroz 2' }).click();
      // The dialog's name is its title, which changes while a number is typed: the page has one dialog open.
      const attack = p.getByRole('dialog');
      await attack.locator('label', { hasText: 'Goblin 1' }).click();
      // A physical die: 18 + 5 hits the goblin's 15.
      await attack.getByRole('button', { name: 'Digitar o resultado' }).click();
      // Pack Tactics: an ally of the wolf is next to the goblin, so the roll has advantage and takes two d20.
      await attack.getByLabel('Primeiro d20').fill('18');
      await attack.getByLabel('Segundo d20').fill('3');
      await attack.getByRole('button', { name: /^Confirmar/ }).click();
      await expect(attack.getByText('Acertou: role o dano.')).toBeVisible();
      // The sheet is closed before the damage: the creature says it is owed, and it blocks its turn.
      await attack.getByRole('button', { name: 'Fechar' }).first().click();
      await expect(blocks.nth(1)).toContainText('Falta rolar o dano do ataque do Lobo atroz 2.');
      await blocks.nth(1).getByRole('button', { name: 'Rolar o dano: Lobo atroz 2' }).click();
      const damage = p.getByRole('dialog');
      await damage.getByRole('button', { name: 'Digitar o resultado' }).click();
      await damage.getByLabel(/Role 2d6/).fill('7');
      // The bite's +3 is added by the app, and the button says the total that is sent: 7 + 3.
      await expect(damage.getByText('7 + 3 = 10')).toBeVisible();
      await damage.getByRole('button', { name: 'Confirmar 10' }).click();
      await expect(damage.getByText(/Goblin 1/).first()).toBeVisible();
      await damage.getByRole('button', { name: 'Voltar à sua vez' }).click();
      await expect(blocks.nth(1)).not.toContainText('Falta rolar o dano');
      await expect(blocks.nth(1).locator('.tile').first()).toContainText('Usada');
      await expect(blocks.first().locator('.tile').first()).toContainText('Disponível');

      // One button ends both parts: it asks first, says what is left, "Voltar" has the focus and the tabs wait.
      await p.getByRole('button', { name: 'Encerrar a parte dos Lobos' }).click();
      const ask = p.getByRole('alertdialog');
      await expect(ask.getByRole('button', { name: 'Voltar' })).toBeFocused();
      await expect(ask).toContainText('Os 2 Lobos ainda têm ação e movimento.');
      await expect(p.locator('app-mine-tabs').first()).toHaveAttribute('inert', '');
      await ask.getByRole('button', { name: 'Encerrar a parte dos Lobos' }).click();
      await expect.poll(async () => {
        const e = await getEncounterRPC(m, campaignId);
        return e.combatants.find((c) => c.id === e.currentCombatantId)?.label ?? '';
      }).not.toMatch(/Lobo atroz/);

      // The master's order: the wolves in a box named for them, and Sálvia's concentration with its question.
      await m.goto(sessionRoute(campaignId));
      const order = m.getByRole('region', { name: 'Ordem de iniciativa' });
      // A row opens in place: the concentration, the form and the beast's pool are in the opened row.
      await openOrderRow(m, 'Sálvia');
      await expect(order.getByText('Lobos atrozes da Sálvia ·')).toBeVisible();
      await expect(order.getByText('CA 14 · da Sálvia')).toHaveCount(2);
      await expect(order.getByText('Concentração: Conjurar Animais · 2 Lobos atrozes')).toBeVisible();
      await order.getByRole('button', { name: 'Perdeu a concentração' }).click();
      const question = order.getByRole('alertdialog', { name: 'A Sálvia perdeu a concentração?' });
      await expect(question.getByRole('button', { name: 'Voltar' })).toBeFocused();
      await expect(question).toContainText('Conjurar Animais acaba e os 2 Lobos atrozes somem do combate, da ordem e do mapa.');
      await expect(question).not.toContainText('Isso não se desfaz');
      await question.getByRole('button', { name: 'Dispensar os Lobos' }).click();
      await expect(order.getByText('Lobo atroz 1')).toHaveCount(0);

      // Her phone says so and stays until touched; the tabs are gone.
      const lost = p.getByTestId('concentration-lost');
      await expect(lost).toContainText('Você perdeu a concentração em Conjurar Animais. Os 2 Lobos atrozes sumiram.');
      await expect(p.getByRole('tablist', { name: 'O que você joga' })).toHaveCount(0);
      await lost.getByRole('button', { name: 'Entendi' }).click();
      await expect(lost).toHaveCount(0);
    } finally {
      await done();
    }
  },
);

test(
  'Sálvia vira Lobo, leva dano além dos PV da fera (o mestre aplica), o que sobra passa para ela, e ela é avisada; depois vira de novo e volta pelo botão',
  { tag: ['@MR-037', '@RN-02', '@RN-20'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const { m, p, t, table, done } = await tables(browser, 'Forma Selvagem');
    try {
      const campaignId = table.campaignId;
      await beginCreatureCombat(m, table, { Sálvia: p, Toren: t }, FACES);
      await passTurnsTo(m, campaignId, 'Sálvia');
      await p.goto(sessionRoute(campaignId));
      await expect(p.getByRole('heading', { name: 'Sua vez, Sálvia' })).toBeVisible();

      // The line of the Ação, in the class's tab (Druida): its uses and what it costs.
      await openActionTab(p, 'Druida');
      const row = p.locator('app-action-row', { hasText: 'Forma Selvagem' });
      await expect(row).toContainText('Vire uma fera · restam 2 de 2 usos · volta no descanso curto ou longo');
      await row.getByRole('button', { name: 'Transformar: Forma Selvagem' }).click();

      // The beasts her level allows, from the server, in order; the chosen one opens its numbers and the cost.
      const sheet = p.getByRole('dialog', { name: 'Forma Selvagem' });
      await expect(sheet.getByText('Feras de ND até 1/2, sem voo.')).toBeVisible();
      await expect(sheet.getByText(/feras que o seu nível permite/)).toBeVisible();
      await expect(sheet.getByText('Escolha uma fera.')).toBeVisible();
      await sheet.getByLabel('Buscar fera').fill('wolf');
      await expect(sheet.locator('label', { hasText: '(Wolf)' })).toHaveCount(1);
      await sheet.locator('label', { hasText: '(Wolf)' }).click();
      await expect(sheet.getByText('Gasta a ação e 1 uso de Forma Selvagem (restará 1).')).toBeVisible();
      await expect(sheet.locator('.row__sub').first()).toContainText('CA 13');
      await sheet.getByRole('button', { name: 'Virar Lobo' }).click();

      // As a wolf: the band, the two reserves, no spells, the bite, and the way back as a bonus action.
      await expect(p.getByText('Na forma de Lobo').first()).toBeVisible();
      await expect(p.locator('app-wild-pools')).toContainText('PV do Lobo');
      await expect(p.locator('app-wild-pools')).toContainText('11 de 11');
      await expect(p.locator('app-wild-pools')).toContainText('PV da Sálvia');
      await openActionTab(p, 'Magias');
      await expect(p.getByText('Sem magias na forma de fera. Volte à forma normal para conjurar.')).toBeVisible();
      await expect(p.getByRole('button', { name: /Conjurar/ })).toHaveCount(0);
      // The Forma Selvagem line is gone from the class's tab, which is "Habilidades" now: the way back is there.
      await openActionTab(p, 'Habilidades');
      await expect(p.getByRole('button', { name: 'Voltar à forma normal' })).toBeVisible();
      await expect(p.getByText('Traços do Lobo')).toBeVisible();

      // Toren and the master read that she is a wolf; only the master (and she) read the wolf's hit points (RN-20).
      await t.goto(sessionRoute(campaignId));
      await expect(t.getByText('Na forma de Lobo').first()).toBeVisible();
      const asWolf = (await getEncounterRPC(t, campaignId)).combatants.find((c) => c.label === 'Sálvia') as unknown as Record<string, unknown>;
      expect(asWolf['wildShapeBeastNamePt']).toBe('Lobo');
      for (const field of ['wildShapeHitPointsCurrent', 'wildShapeHitPointsMax']) {
        expect(asWolf[field], field).toBeUndefined();
      }
      await expect(t.getByText(/PV do Lobo|11 de 11/)).toHaveCount(0);
      await m.goto(sessionRoute(campaignId));
      await openOrderRow(m, 'Sálvia');
      await expect(m.getByRole('region', { name: 'Ordem de iniciativa' }).getByText('Na forma de Lobo').first()).toBeVisible();
      await expect(m.getByRole('region', { name: 'Ordem de iniciativa' }).locator('app-beast-pool', { hasText: '11 de 11' })).toBeVisible();

      // Damage past the beast's hit points: the master applies it; the beast falls, 19 carry over, and she is told.
      await hitAndApply(m, campaignId, 'Goblin 1', 'Sálvia', 30);
      const notice = p.getByTestId('form-ended');
      await expect(notice).toContainText('O Lobo caiu a 0 PV e você voltou à forma normal.');
      await expect(notice).toContainText('19 de dano passaram para você.');
      await expect(p.getByText('Na forma de Lobo')).toHaveCount(0);
      // Toren never read the numbers.
      await t.goto(sessionRoute(campaignId));
      await expect(t.getByText(/de dano passaram/)).toHaveCount(0);
      await notice.getByRole('button', { name: 'Entendi' }).click();
      await expect(notice).toHaveCount(0);

      // Next round her action is free again: back through the bonus action, with no notice for what she did herself.
      await endTurnOf(p, m, campaignId, 'Sálvia');
      await passTurnsToAndWait(m, campaignId, 'Sálvia', [m, p]);
      await expect(p.getByRole('heading', { name: 'Sua vez, Sálvia' })).toBeVisible();
      await openActionTab(p, 'Druida');
      await p.getByRole('button', { name: 'Transformar: Forma Selvagem' }).click();
      const again = p.getByRole('dialog', { name: 'Forma Selvagem' });
      await again.getByLabel('Buscar fera').fill('wolf');
      await again.locator('label', { hasText: '(Wolf)' }).click();
      await again.getByRole('button', { name: 'Virar Lobo' }).click();
      await expect(p.getByText('Na forma de Lobo').first()).toBeVisible();
      await openActionTab(p, 'Habilidades');
      await p.getByRole('button', { name: 'Voltar à forma normal' }).click();
      await expect(p.getByText('Na forma de Lobo')).toHaveCount(0);
      await expect(p.getByTestId('form-ended')).toHaveCount(0);
    } finally {
      await done();
    }
  },
);

test(
  'o Nanquim de Pensantus tem a própria vez: voa, usa as ações padrão e não ataca',
  { tag: ['@MR-037', '@RN-20'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const { m, p, t, table, done } = await tables(browser, 'Familiar', { hero: 'pensantus', nanquim: true });
    try {
      const campaignId = table.campaignId;
      // Pensantus 20, Nanquim 12 (the raven rolls its own), then the rest.
      await beginCreatureCombat(m, table, { Pensantus: p, Toren: t, Nanquim: p }, { Pensantus: 20, Toren: 15, Nanquim: 9, 'Capitão Goblin': 2, 'Goblin 1': 2, 'Goblin 2': 2 });
      await passTurnsTo(m, campaignId, 'Nanquim');
      await p.goto(sessionRoute(campaignId));
      await expect(p.getByRole('heading', { name: 'Vez do seu Nanquim' })).toBeVisible();
      const tabs = p.getByRole('tablist', { name: 'O que você joga' });
      await expect(tabs.getByRole('tab', { name: /Nanquim/ })).toHaveAttribute('aria-selected', 'true');
      const block = p.locator('app-creature-block');
      await expect(block).toContainText('PV 1 de 1 · CA 12');
      await expect(block.getByText('O familiar não ataca.')).toBeVisible();
      await expect(block.getByRole('button', { name: /^Atacar/ })).toHaveCount(0);
      for (const name of ['Ajudar', 'Disparada', 'Desengajar', 'Esquivar', 'Esconder', 'Procurar']) {
        await expect(block.locator('.std__btn', { hasText: name })).toHaveCount(1);
      }
      await expect(block.getByRole('button', { name: 'Mover o Nanquim' })).toBeVisible();
      await expect(block.locator('.tile').nth(1)).toContainText('voo');

      // A familiar can't attack, but it can take the Help action (SRD 5.1, Find Familiar): the same sheet as a
      // character's, and it spends the raven's own action, not Pensantus's.
      await block.getByRole('button', { name: 'Ajudar: Nanquim' }).click();
      const help = p.getByRole('dialog');
      await help.locator('label', { hasText: 'Toren' }).click();
      await help.getByRole('button', { name: 'Continuar' }).click();
      await help.locator('label', { hasText: 'Percepção' }).click();
      await help.getByRole('button', { name: /^Ajudar Toren/ }).click();
      await expect(help).toContainText('Você ajudou Toren. Ele terá vantagem no próximo teste de Percepção.');
      await closeSheet(help);
      await expect(block.locator('.tile').first()).toContainText('Usada');
      await p.getByRole('button', { name: 'Encerrar a vez do Nanquim' }).click();
      await expect.poll(async () => {
        const e = await getEncounterRPC(m, campaignId);
        return e.combatants.find((c) => c.id === e.currentCombatantId)?.label ?? '';
      }).not.toBe('Nanquim');
    } finally {
      await done();
    }
  },
);
