import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { getEncounterRPC, waitTurnLeaves } from './combat-support';
import { endOpenSessionRPC, openSessionPage, openSheetTab } from './live-session-support';
import { combatWithTwoDown, reviveRPC, tableWithDead } from './revivify-support';
import { newSignedInContext, openActionTab } from './support';

// RN-03 and RN-10 (Revivificar, PM-08d), against the real screens: the cleric casts on a dead ally in a combat and outside one,
// the player's list holds only what the spell reaches, the diamonds are ticked, and the master answers outside a combat. The table
// comes through the API; what is under test is the sheet and what each person reads.

test(
  'em combate a clériga revive o aliado ao lado: a lista só traz quem a magia alcança, os diamantes são obrigatórios e o resultado diz o que foi gasto',
  { tag: ['@RN-03', '@RN-10'] },
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
      const { table } = await combatWithTwoDown(m, p, 'Revivificar em combate');
      campaignId = table.campaignId;
      await openSessionPage(p, campaignId);
      await expect(p.getByRole('heading', { name: 'Sua vez, Ilaria' })).toBeVisible();

      // Choosing the spell opens its own sheet, with the caster and the three steps.
      await openActionTab(p, 'Magias');
      await p.getByRole('button', { name: 'Conjurar Revivificar' }).click();
      const sheet = p.getByRole('dialog', { name: 'Revivificar' });
      await expect(sheet.getByRole('heading', { name: 'Revivificar' })).toBeFocused();
      await expect(sheet).toContainText('toque');
      await expect(sheet).toContainText('Ilaria, Clérigo 5');
      await expect(sheet.getByRole('list', { name: 'Passos' })).toContainText('1 Alvo 2 Confirmar 3 Resultado');

      // Only the creature the spell reaches is listed, and nothing says why the other is not.
      const who = sheet.getByRole('radiogroup', { name: 'Quem morreu por perto' });
      await expect(who.getByRole('radio')).toHaveCount(1);
      await expect(who).toContainText('Goblin 1');
      await expect(who).toContainText('Pode ser revivido');
      await expect(sheet).toContainText('Só aparece quem a magia pode alcançar agora.');
      await expect(sheet).not.toContainText('Goblin 2');
      await expect(sheet).not.toContainText(/longe|escondid|bloquead/i);

      // The diamonds are a reminder that must be ticked: until then the cast is the dashed button and does nothing.
      await sheet.getByRole('button', { name: 'Próximo' }).click();
      await expect(sheet.getByRole('heading', { name: 'Revivificar em Goblin 1' })).toBeFocused();
      await expect(sheet).toContainText('Gasta um espaço de 3º nível (você tem 2) e a sua ação.');
      const cast = sheet.getByRole('button', { name: 'Conjurar Revivificar' });
      await expect(cast).toHaveAttribute('aria-disabled', 'true');
      await expect(sheet).toContainText('Marque que você tem os diamantes');
      await cast.dispatchEvent('click');
      await expect(sheet.getByRole('heading', { name: 'Revivificar em Goblin 1' })).toBeVisible();
      expect((await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Goblin 1')?.defeated).toBe(true);

      await sheet.getByText('Tenho os diamantes', { exact: true }).click();
      await expect(sheet.getByRole('checkbox', { name: /Tenho os diamantes/ })).toBeChecked();
      await expect(cast).not.toHaveAttribute('aria-disabled', 'true');
      await cast.click();

      // The result, as a status: who lives, what was spent, the diamonds; and the turn can end.
      const result = sheet.getByRole('status').filter({ hasText: 'Goblin 1 voltou à vida.' });
      await expect(result).toContainText('Está com 1 PV, acordado e sem testes contra a morte.');
      await expect(result).toContainText('Você gastou um espaço de 3º nível (restam 1 de 2) e a sua ação.');
      await expect(result).toContainText('Diamantes de 300 PO gastos, como você confirmou.');
      expect((await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Goblin 1')?.defeated).toBeFalsy();

      await sheet.getByRole('button', { name: 'Encerrar turno' }).click();
      await waitTurnLeaves(m, campaignId, 'Ilaria');
      await expect(sheet).toHaveCount(0);
    } finally {
      await endOpenSessionRPC(m, campaignId);
      await master.close();
      await player.close();
    }
  },
);

test(
  'fora de combate a clériga espera o mestre: ele diz que já passou e ela lê só "O mestre disse que não dá."; depois ele diz que faz menos de 1 minuto',
  { tag: ['@RN-03', '@RN-10'] },
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
      const table = await tableWithDead(m, p, 'Revivificar fora de combate', true);
      campaignId = table.campaignId;
      await openSessionPage(m, campaignId);
      await openSessionPage(p, campaignId);

      // The button is on the page of the player whose character has the spell, with no combat open.
      await openSheetTab(p);
      await p.getByRole('button', { name: 'Revivificar', exact: true }).click();
      const sheet = p.getByRole('dialog', { name: 'Revivificar' });
      await expect(sheet.getByRole('radiogroup', { name: 'Quem morreu por perto' })).toContainText('Toren');
      await expect(sheet).toContainText('O mestre confirma que faz menos de 1 minuto.');
      await expect(sheet).toContainText('Fora de combate o app não conta o tempo: ao conjurar, o mestre responde.');
      await sheet.getByRole('button', { name: 'Próximo' }).click();
      await expect(sheet).toContainText('Gasta um espaço de 3º nível (você tem 2).');
      await expect(sheet).not.toContainText('a sua ação');
      await sheet.getByText('Tenho os diamantes', { exact: true }).click();
      await expect(sheet.getByRole('checkbox', { name: /Tenho os diamantes/ })).toBeChecked();
      await sheet.getByRole('button', { name: 'Conjurar Revivificar' }).click();

      // Nothing is spent while it waits.
      await expect(sheet.getByRole('status').filter({ hasText: 'Esperando o mestre' })).toBeVisible();
      await expect(sheet.getByRole('button', { name: 'Encerrar turno' })).toHaveCount(0);

      // The master reads the question, and says it was longer: the caster reads only that.
      const ask = m.getByRole('alertdialog', { name: 'Ilaria quer conjurar Revivificar em Toren' });
      await expect(ask).toBeVisible();
      await ask.getByRole('button', { name: 'Já passou' }).click();
      const answer = sheet.getByRole('status').filter({ hasText: 'O mestre disse que não dá.' });
      await expect(answer).toBeVisible();
      await expect(sheet).not.toContainText('voltou à vida');
      await expect(sheet).not.toContainText('Diamantes de 300 PO gastos');
      await sheet.getByRole('button', { name: 'Voltar à sessão' }).click();
      await expect(sheet).toHaveCount(0);
    } finally {
      await endOpenSessionRPC(m, campaignId);
      await master.close();
      await player.close();
    }
  },
);

test(
  'o jogador do personagem morto lê "Você voltou à vida" quando o mestre o revive',
  { tag: ['@RN-03', '@RN-10'] },
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
      const table = await tableWithDead(m, p, 'Voltou à vida', false);
      campaignId = table.campaignId;
      await openSessionPage(p, campaignId);
      await openSheetTab(p);
      await expect(p.getByText('Você não tem um personagem vivo nesta campanha.')).toBeVisible();

      await reviveRPC(m, campaignId, table.deadId);

      // The notice is a polite status; without a log line that names a caster, it says only what happened.
      const notice = p.getByRole('status').filter({ hasText: 'Você voltou à vida.' });
      await expect(notice).toContainText('Está com 1 PV.');
      await expect(notice).not.toContainText('usou Revivificar');
      await expect(notice).toHaveAttribute('aria-live', 'polite');
    } finally {
      await endOpenSessionRPC(m, campaignId);
      await master.close();
      await player.close();
    }
  },
);
