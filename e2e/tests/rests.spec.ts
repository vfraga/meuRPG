import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { endTurnOf, getEncounterRPC, passTurnsTo, tableForCombat, toren, torenSheet } from './combat-support';
import { tableForLevelUp } from './levelup-support';
import { endOpenSessionRPC, openSessionPage, openSheetTab } from './live-session-support';
import { setDiceModeRPC } from './puzzles-support';
import {
  aurora,
  auroraSheet,
  beginPlacedCombatRPC,
  createAllyRPC,
  createCreatureNpcRPC,
  damageCombatantRPC,
  spendForRestRPC,
} from './rests-support';
import { newSignedInContext, openActionTab } from './support';
import { masterLog } from './combat-layout';
import { waitForEncounterRevision } from './revision-support';

// Rests, hit dice and Lay on Hands on screen (Etapa 8, MR-012, MR-014, RN-02, RN-10): the master's short rest
// and what it gives back, the player's page changing with no reload, the hit die spent from the sheet, and the touch
// of a paladin that heals an ally and does nothing to an undead without ever saying why. The tables come through
// the API; what is under test is the screens and what each audience reads.

test(
  'o mestre faz o descanso curto: a confirmação lista o que volta, a ficha do jogador muda ao vivo e o jogador gasta um dado de vida',
  { tag: ['@MR-012', '@RN-02'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
    const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 390, height: 844 } });
    const m = await master.newPage();
    const p = await player.newPage();
    let campaignId = '';
    try {
      await m.goto('/');
      await p.goto('/');
      const table = await tableForLevelUp(m, p, `Descanso curto ${Date.now()}`, { build: toren, sheet: torenSheet, milestone: false });
      campaignId = table.campaignId;
      // Real dice at this table: the player types the face of the hit die, so the healing is the same on every run.
      await setDiceModeRPC(m, campaignId, 'DICE_MODE_PHYSICAL');
      // Toren (Fighter 5, Con +3, 46 hit points): 10 hit points left, 3 of 5 d10 spent, Second Wind and Action Surge used.
      await spendForRestRPC(m, campaignId, table.characterId);

      // The player has the session and the sheet open, and neither is reloaded from here on.
      const sheet = await player.newPage();
      await sheet.goto(`/campaigns/${campaignId}/characters/${table.characterId}`);
      const counter = (name: string) => sheet.getByRole('region', { name: 'Recursos' }).getByRole('listitem').filter({ hasText: name });
      await expect(counter('Retomar o Fôlego')).toContainText('0 de 1');
      await expect(counter('Surto de Ação')).toContainText('0 de 1');
      await openSessionPage(p, campaignId);
      await expect(p.locator('.hp__current')).toHaveText('10');

      // The master opens the rest card and reads what a short rest gives back before taking it.
      await openSessionPage(m, campaignId, 'grupo');
      const card = m.getByRole('region', { name: 'Descanso' });
      await card.getByRole('button', { name: 'Descanso curto' }).click();
      const question = card.getByRole('alertdialog', { name: 'Começar o descanso curto?' });
      await expect(question).toContainText('Pelo menos 1 hora');
      await expect(question).toContainText('Os jogadores podem gastar dados de vida para curar.');
      await expect(question).toContainText('Retomar o Fôlego: Toren 1 de 1');
      await expect(question).toContainText('Surto de Ação: Toren 1 de 1');
      await question.getByRole('button', { name: 'Descansar' }).click();
      await expect(card.getByText('Descanso curto feito.').first()).toBeVisible();
      await expect(question).toHaveCount(0);

      // The player's pages changed by themselves: the uses are back, the hit points are not (a short rest gives dice, not hit points).
      await expect(counter('Retomar o Fôlego')).toContainText('1 de 1');
      await expect(counter('Surto de Ação')).toContainText('1 de 1');
      await expect(p.locator('.hp__current')).toHaveText('10');

      // The player spends one d10: the face is typed, the line says the roll, the modifier and what came back.
      await openSheetTab(p);
      await p.getByRole('button', { name: 'Gastar dados de vida' }).click();
      const dice = p.getByRole('dialog', { name: 'Gastar dados de vida' });
      await expect(dice).toContainText('d10');
      await dice.getByLabel(/Role 1d10/).fill('6');
      await dice.getByRole('button', { name: 'Confirmar 6' }).click();
      await expect(dice.getByRole('list', { name: 'Dados gastos agora' })).toContainText('Rolou 6 + 3 = 9 · recuperou 9 PV');
      await dice.locator('button', { hasText: 'Fechar' }).click();
      await expect(p.locator('.hp__current')).toHaveText('19');
    } finally {
      await endOpenSessionRPC(m, campaignId);
      await master.close();
      await player.close();
    }
  },
);

test(
  'a paladina cura uma aliada com a Cura pelas Mãos e, num morto-vivo, só lê "Sem efeito", sem o tipo da criatura',
  { tag: ['@MR-014', '@RN-02', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
    const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 390, height: 844 } });
    const m = await master.newPage();
    const p = await player.newPage();
    let campaignId = '';
    try {
      await m.goto('/');
      await p.goto('/');
      const table = await tableForCombat(m, p, `Cura pelas Mãos ${Date.now()}`, true, false, { build: aurora, sheet: auroraSheet });
      campaignId = table.campaignId;
      const brisaId = await createAllyRPC(m, campaignId, 'Brisa', 20);
      const skeletonId = await createCreatureNpcRPC(m, campaignId, 'monster:skeleton', 'Esqueleto');
      // Aurora (Paladin 3: 15 points) stands between Brisa and the Esqueleto, both within the touch.
      await beginPlacedCombatRPC(m, table, [brisaId, skeletonId], { Aurora: 20, Brisa: 15, Esqueleto: 5 }, { Aurora: [8, 9], Brisa: [7, 9], Esqueleto: [9, 9] });
      const hitPoints = async (label: string) => (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === label)!.hitPointsCurrent;
      await damageCombatantRPC(m, campaignId, 'Brisa', 12);
      await damageCombatantRPC(m, campaignId, 'Esqueleto', 3);
      expect(await hitPoints('Brisa')).toBe(8);

      await openSessionPage(m, campaignId, 'grupo');
      await openSessionPage(p, campaignId);
      await expect(p.getByRole('heading', { name: 'Sua vez, Aurora' })).toBeVisible();
      const groups = p.getByRole('region', { name: 'O que você pode fazer' });

      // The touch on the ally: five points, "Curou", and the pool is 10 of 15.
      await openActionTab(p, 'Paladino');
      await groups.getByRole('button', { name: 'Usar Cura pelas Mãos' }).click();
      let dialog = p.getByRole('dialog', { name: 'Cura pelas Mãos' });
      await expect(dialog).toContainText('Reserva: 15 de 15 pontos');
      await dialog.locator('label', { hasText: 'Brisa' }).click();
      await dialog.getByRole('button', { name: 'Curar 5 PV em Brisa' }).click();
      await expect(dialog.locator('.pill')).toContainText('Curou');
      await expect(dialog).toContainText('restam 10 de 15');
      await dialog.locator('button', { hasText: 'Fechar' }).click();
      await expect(dialog).toHaveCount(0);
      await expect.poll(() => hitPoints('Brisa')).toBe(13);
      await expect(await masterLog(m)).toContainText('Aurora curou 5 PV de Brisa com a Cura pelas Mãos (5 pontos)');

      // The next round: the pool shows what is left, and the touch on the undead spends the points and does nothing.
      await endTurnOf(p, m, campaignId, 'Aurora');
      await waitForEncounterRevision([m, p], await passTurnsTo(m, campaignId, 'Aurora'));
      await expect(p.getByRole('heading', { name: 'Sua vez, Aurora' })).toBeVisible();
      await openActionTab(p, 'Paladino');
      await groups.getByRole('button', { name: 'Usar Cura pelas Mãos' }).click();
      dialog = p.getByRole('dialog', { name: 'Cura pelas Mãos' });
      await expect(dialog).toContainText('Reserva: 10 de 15 pontos');
      const skeletonBefore = await hitPoints('Esqueleto');
      await dialog.locator('label', { hasText: 'Esqueleto' }).click();
      await dialog.getByRole('button', { name: 'Curar 5 PV em Esqueleto' }).click();
      await expect(dialog.locator('.pill')).toHaveText(/Sem efeito/);
      await expect(dialog).toContainText('Você tocou o Esqueleto e gastou 5 pontos da reserva (restam 5 de 15). Nada acontece.');
      // Not a word about what the Esqueleto is, on the player's screen.
      await expect(dialog).not.toContainText(/morto-vivo|constructo|construto|criatura|imune/i);
      await dialog.locator('button', { hasText: 'Fechar' }).click();
      expect(await hitPoints('Esqueleto')).toBe(skeletonBefore);
      // The master's log is the only place that says why.
      await expect(await masterLog(m)).toContainText('Aurora tocou o Esqueleto com a Cura pelas Mãos (5 pontos): sem efeito; o toque não agiu');
      await expect(p.getByText(/o toque não agiu/)).toHaveCount(0);
    } finally {
      await endOpenSessionRPC(m, campaignId);
      await master.close();
      await player.close();
    }
  },
);
