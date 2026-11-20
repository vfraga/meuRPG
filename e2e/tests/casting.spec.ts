import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { adjustVitalsRPC } from './combat-support';
import { castSheet, openCastOf, pickChoice, pickSlotRadio, pickTargetOf, tableForCasting, tableForHealing } from './casting-support';
import { endOpenSessionRPC, openSessionPage } from './live-session-support';
import { newSignedInContext } from './support';

// MR-048 (casting outside a combat), RN-32 (what a cast does and who reads it) and RN-10 (the master reads every
// cast, a player what they may). Setup goes through the API; every test makes its own campaign.

test(
  'a jogadora conjura Armadura Arcana em si, vê a CA e encerra a magia depois de confirmar',
  { tag: ['@MR-048', '@RN-32'] },
  async ({ browser }) => {
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    let campaignId = '';
    const master = await masterContext.newPage();
    try {
      const player = await playerContext.newPage();
      await master.goto('/');
      await player.goto('/');
      const table = await tableForCasting(master, player, `Armadura ${Date.now()}`);
      campaignId = table.campaignId;

      await openSessionPage(player, campaignId);
      const sheet = await openCastOf(player, 'Armadura Arcana');
      await pickSlotRadio(sheet, '1º nível');
      await pickTargetOf(sheet, 'Pensantus');
      await sheet.getByRole('button', { name: 'Conjurar Armadura Arcana em Pensantus' }).click();
      await expect(sheet.getByText(/CA \d+ \(13 \+ Destreza\)/)).toBeVisible();
      await sheet.getByRole('button', { name: 'Fechar' }).last().click();

      // The spell lasts: "dura 8 horas" is game time, and the note says what ends it.
      const panel = player.locator('app-casting-panel');
      await expect(panel.getByText('Magias ativas')).toBeVisible();
      await expect(panel.getByText('dura 8 horas')).toBeVisible();
      await expect(panel.getByText(/descanso longo do mestre/)).toBeVisible();

      // "Encerrar" asks first; Cancelar has the focus.
      await panel.getByRole('button', { name: 'Encerrar' }).click();
      const confirm = player.locator('app-cast-confirm-sheet');
      await expect(confirm.getByText('Encerrar Armadura Arcana de Pensantus?')).toBeVisible();
      await expect(confirm.getByRole('button', { name: 'Cancelar' })).toBeFocused();
      await confirm.getByRole('button', { name: 'Encerrar', exact: true }).click();
      await expect(panel.getByText('Magias ativas')).toBeHidden();
    } finally {
      await endOpenSessionRPC(master, campaignId);
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'um ritual de 11 minutos fica conjurando até o mestre concluir a conjuração',
  { tag: ['@MR-048', '@RN-32'] },
  async ({ browser }) => {
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    let campaignId = '';
    const master = await masterContext.newPage();
    try {
      const player = await playerContext.newPage();
      await master.goto('/');
      await player.goto('/');
      const table = await tableForCasting(master, player, `Ritual ${Date.now()}`);
      campaignId = table.campaignId;

      await openSessionPage(player, campaignId);
      await openSessionPage(master, campaignId, 'grupo');
      const sheet = await openCastOf(player, 'Alarme');
      await pickChoice(sheet, 'Como ritual');
      await expect(sheet.getByText('1 minuto + 10 = 11 minutos')).toBeVisible();
      await expect(sheet.locator('app-slot-picker')).toHaveCount(0);
      await sheet.getByRole('button', { name: 'Começar o ritual' }).click();
      await expect(sheet.getByText(/Um ritual não gasta espaço de magia/)).toBeVisible();
      await sheet.getByRole('button', { name: 'Fechar' }).last().click();

      const mine = player.locator('app-casting-panel');
      await expect(mine.getByText('Esperando o mestre concluir')).toBeVisible();
      await expect(mine.getByRole('button', { name: 'Concluir conjuração' })).toHaveCount(0);

      // The master sees the queue and completes the cast; the player's panel follows on its own.
      const queue = master.locator('app-casting-panel');
      await expect(queue.getByText(/conjura Alarme como ritual\./)).toBeVisible();
      await queue.getByRole('button', { name: 'Concluir conjuração' }).click();
      await expect(mine.getByText('Magias ativas')).toBeVisible();
      await expect(mine.getByText('dura 8 horas')).toBeVisible();
      await expect(castSheet(player)).toHaveCount(0);
    } finally {
      await endOpenSessionRPC(master, campaignId);
      await masterContext.close();
      await playerContext.close();
    }
  },
);

// #944: the dice a physical-dice table rolls follow the level of the slot, not the spell's own.
test(
  'numa mesa de dados físicos, Curar Ferimentos com um espaço de 3º nível pede 3d8 e aplica a soma digitada',
  { tag: ['@MR-048', '@RN-32'] },
  async ({ browser }) => {
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    let campaignId = '';
    const master = await masterContext.newPage();
    try {
      const player = await playerContext.newPage();
      await master.goto('/');
      await player.goto('/');
      const table = await tableForHealing(master, player, `Cura ${Date.now()}`);
      campaignId = table.campaignId;
      await adjustVitalsRPC(master, campaignId, table.characterId, { hitPointsCurrent: 1 });

      await openSessionPage(player, campaignId);
      const sheet = await openCastOf(player, 'Curar Ferimentos');
      await pickSlotRadio(sheet, '1º nível');
      await expect(sheet.getByText('Role 1d8 para Curar Ferimentos: some os dados')).toBeVisible();
      await expect(sheet.getByText('Digite a soma dos dados, de 1 a 8.')).toBeVisible();
      await pickSlotRadio(sheet, '3º nível');
      await expect(sheet.getByText('Role 3d8 para Curar Ferimentos: some os dados')).toBeVisible();
      await expect(sheet.getByText('Digite a soma dos dados, de 3 a 24.')).toBeVisible();
      await pickTargetOf(sheet, 'Brisa');

      // 20 is above the 1d8 maximum, so only the 3d8 of the 3rd-level slot accepts it. Healed: 20 + Wisdom 3 + Disciple of Life (2 + 3).
      await sheet.getByLabel('Role 3d8 para Curar Ferimentos: some os dados').fill('20');
      await sheet.getByRole('button', { name: 'Confirmar' }).click();
      await expect(sheet.getByText('Brisa recuperou 28 PV.')).toBeVisible();
    } finally {
      await endOpenSessionRPC(master, campaignId);
      await masterContext.close();
      await playerContext.close();
    }
  },
);
