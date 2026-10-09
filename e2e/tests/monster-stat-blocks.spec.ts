import { expect, test, type Page } from '@playwright/test';

import { combatRPC, getEncounterRPC, tableForCombat } from './combat-support';
import { openSessionPage } from './live-session-support';
import { callRPC, newSignedInContext } from './support';
import { DRAGON, dragonFight } from './monster-stat-support';

// W7-M: a monster fights with its whole SRD stat block. The master's turn card opens the sheet beside the actions, an attack and a
// saving throw are made whole by the server, Fire Breath waits for its recharge, a legendary action is offered at the end of another
// creature's turn, and a player never gets any of it (RN-10, RN-20). The setup goes through the API; the combat is without a map
// (RN-25), so a monster has no square to be out of reach of.

/** The master's card of the monster on turn. */
const sheetOf = (master: Page) => master.getByRole('region', { name: 'Ficha do monstro' });
const actionsOf = (master: Page) => master.getByRole('region', { name: 'Ações', exact: true });

test(
  'o monstro com a ficha inteira: o mestre lê a ficha e usa a mordida e o Sopro de Fogo; o jogador não recebe nada da ficha',
  { tag: ['@W7-M', '@RN-10', '@RN-20'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const master = await masterContext.newPage();
      const player = await playerContext.newPage();
      await master.goto('/');
      await player.goto('/');
      const table = await tableForCombat(master, player, `Dragão ${Date.now()}`);
      const campaignId = table.campaignId;
      await dragonFight(master, table);

      // The sheet beside the actions, from the SRD block: armor class, hit points, immunity, the legendary actions.
      await openSessionPage(master, campaignId);
      const sheet = sheetOf(master);
      await expect(sheet).toContainText(DRAGON);
      await expect(sheet).toContainText('256 de 256');
      await expect(sheet).toContainText('Imunidades a dano');
      await expect(sheet).toContainText('fogo');
      await expect(sheet).toContainText('Resistência Lendária (3/dia).');
      await expect(sheet).toContainText('Ações lendárias (3 por rodada)');
      const actions = actionsOf(master);
      for (const name of ['Ataque múltiplo', 'Mordida', 'Garra', 'Cauda', 'Presença Aterradora', 'Sopro de Fogo']) {
        await expect(actions.getByRole('heading', { name })).toBeVisible();
      }
      await expect(actions).toContainText('Recarga 5–6 · Disponível');

      // An attack needs one target: the button is grey and says why.
      const bite = actions.locator('article', { has: master.getByRole('heading', { name: 'Mordida' }) });
      await expect(bite.getByRole('button', { name: 'Atacar' })).toHaveAttribute('aria-disabled', 'true');
      await expect(bite).toContainText('Escolha um alvo.');
      await actions.getByRole('combobox', { name: 'Alvos' }).click();
      await master.getByRole('option', { name: 'Pensantus' }).click();
      await master.keyboard.press('Escape');
      // Fire Breath: the saving throw is the server's; the action then waits for its recharge, and the turn's one action is spent.
      const breath = actions.locator('article', { has: master.getByRole('heading', { name: 'Sopro de Fogo' }) });
      await expect(breath).toContainText('Cone de 18 m');
      await expect(breath).toContainText('metade se passar');
      await breath.getByRole('button', { name: 'Soprar' }).click();
      await expect(actions.getByRole('status').filter({ hasText: 'Sopro de Fogo' })).toBeVisible();
      await expect(breath).toContainText('Recarga 5–6 · Recarregando');
      await expect(breath.getByRole('button', { name: 'Soprar' })).toHaveAttribute('aria-disabled', 'true');
      await expect(bite).toContainText('A ação deste turno já foi usada.');

      // RN-10, RN-20: the player's combat holds nothing of the stat block.
      const playerEnc = JSON.stringify(await getEncounterRPC(player, campaignId));
      for (const secret of ['creatureSheet', 'legendary', 'resistanceLeft', 'monster:adult-red-dragon', 'rechargeRoll']) {
        expect(playerEnc, `o jogador não recebe "${secret}"`).not.toContain(secret);
      }
      const turn = await callRPC(player, 'meurpg.play.v1.CreatureService/GetCreatureTurn', {
        campaignId,
        encounterId: (await getEncounterRPC(master, campaignId)).id,
        combatantId: (await getEncounterRPC(master, campaignId)).combatants.find((c) => c.label === DRAGON)!.id,
      });
      expect(turn.ok()).toBeFalsy();
      await openSessionPage(player, campaignId);
      await expect(player.getByText('Ficha do monstro')).toHaveCount(0);
      await expect(player.getByText('Resistência Lendária')).toHaveCount(0);
    } finally {
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'a ação lendária é oferecida no fim do turno de outra criatura, uma por oferta, só ao mestre',
  { tag: ['@W7-M', '@RN-10', '@RN-20'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const master = await masterContext.newPage();
      const player = await playerContext.newPage();
      await master.goto('/');
      await player.goto('/');
      const table = await tableForCombat(master, player, `Lendária ${Date.now()}`);
      const campaignId = table.campaignId;
      let enc = await dragonFight(master, table);

      // The dragon's own turn ends: no offer, for the end of its own turn is not another creature's.
      enc = await combatRPC(master, 'EndTurn', { campaignId, encounterId: enc.id, expectedCombatantId: enc.currentCombatantId, discardPendingDamage: true });
      await openSessionPage(master, campaignId);
      await expect(master.getByText('pode usar uma ação lendária')).toHaveCount(0);
      // Pensantus's turn ends: the offer opens for the dragon.
      await combatRPC(master, 'EndTurn', { campaignId, encounterId: enc.id, expectedCombatantId: enc.currentCombatantId, discardPendingDamage: true });
      await openSessionPage(master, campaignId);
      const offer = master.getByRole('group', { name: `Ação lendária de ${DRAGON}` });
      await expect(offer).toContainText('Fim da vez de');
      await expect(offer).toContainText('3 de 3');
      await expect(offer.getByRole('button', { name: 'Deixar passar' })).toBeVisible();
      expect(JSON.stringify(await getEncounterRPC(player, campaignId))).not.toContain('legendaryOffers');
      await openSessionPage(player, campaignId);
      await expect(player.getByText('ação lendária')).toHaveCount(0);

      // One for each offer: Detectar closes it.
      await offer.getByRole('button', { name: 'Usar (1)' }).first().click();
      await expect(master.getByRole('group', { name: `Ação lendária de ${DRAGON}` })).toHaveCount(0);
    } finally {
      await masterContext.close();
      await playerContext.close();
    }
  },
);
