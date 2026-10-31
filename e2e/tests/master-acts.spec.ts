import { expect, type Browser, type BrowserContext, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { adjustVitalsRPC, beginAttackCombatRPC, combatRPC, getEncounterRPC, passTurnsTo, pensantusCasting, tableForCombat, type CombatTable } from './combat-support';
import { endOpenSessionRPC, openSessionPage } from './live-session-support';
import { newSignedInContext } from './support';
import { waitForEncounterRevision } from './revision-support';

// The master acts for a player's character (MR-053, C8-01 to C8-03): "Agir por Pensantus" on the character's turn, the band and
// "Voltar ao mestre", the one end-of-turn button named for the character, and the death save that starts a downed character's
// turn, which the master cannot skip. The table and the NPCs come through the API; what is under test is what each screen reads.

const playerFirst = { Pensantus: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 };

interface ActsTable {
  m: Page;
  p: Page;
  table: CombatTable;
  campaignId: string;
  done: () => Promise<void>;
}

/** A combat with Pensantus (casting), the Capitão and two Goblins, begun with Pensantus first; the master at 1280, the player on a phone. */
async function actsTable(browser: Browser, name: string): Promise<ActsTable> {
  const master: BrowserContext = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
  const player: BrowserContext = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 390, height: 844 } });
  const m = await master.newPage();
  const p = await player.newPage();
  await m.goto('/');
  await p.goto('/');
  const table = await tableForCombat(m, p, `${name} ${Date.now()}`, true, true, { sheet: pensantusCasting });
  await beginAttackCombatRPC(m, table, playerFirst);
  return {
    m,
    p,
    table,
    campaignId: table.campaignId,
    done: async () => {
      await endOpenSessionRPC(m, table.campaignId);
      await master.close();
      await player.close();
    },
  };
}

test('o mestre age pelo Pensantus: a faixa, "Voltar ao mestre" e um só botão para encerrar o turno dele', { tag: ['@MR-053', '@RN-20'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const { m, p, campaignId, done } = await actsTable(browser, 'Agir por');
  try {
    await openSessionPage(m, campaignId);
    await openSessionPage(p, campaignId);
    await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();

    // On the player's turn the master's "Turno" has the character's card: one outlined "Agir por", the combat bar still "Próximo turno".
    await expect(m.getByRole('region', { name: 'Vez do Pensantus' })).toBeVisible();
    await expect(m.getByRole('button', { name: 'Próximo turno' })).toBeVisible();
    await expect(m.getByRole('button', { name: 'Mover' })).toHaveCount(0);

    // Acting: the band, the character's own tabs, one end-of-turn button for him and the "Mover" of the map's header.
    await m.getByRole('button', { name: 'Agir por Pensantus' }).click();
    await expect(m.getByText('Você está agindo por Pensantus')).toBeVisible();
    await expect(m.getByRole('button', { name: 'Atacar com Raio de Fogo' })).toBeVisible();
    await expect(m.getByRole('button', { name: 'Encerrar o turno de Pensantus' })).toHaveCount(1);
    await expect(m.getByRole('button', { name: 'Próximo turno' })).toHaveCount(0);
    await expect(m.getByRole('button', { name: 'Mover' })).toHaveCount(1);

    // "Voltar ao mestre" gives the rail back; the turn is still Pensantus's and the player still has it.
    await m.getByRole('button', { name: 'Voltar ao mestre' }).click();
    await expect(m.getByRole('button', { name: 'Agir por Pensantus' })).toBeVisible();
    await expect(m.getByRole('button', { name: 'Próximo turno' })).toBeVisible();
    await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();

    // The master ends the character's turn: the captain's comes.
    await m.getByRole('button', { name: 'Agir por Pensantus' }).click();
    await m.getByRole('button', { name: 'Encerrar o turno de Pensantus' }).click();
    await expect(m.getByText('Vez do Capitão Goblin').first()).toBeVisible();
    await expect(m.getByText('Você está agindo por Pensantus')).toHaveCount(0);
    await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toHaveCount(0);
  } finally {
    await done();
  }
});

test('com as leituras do combate lentas, o turno que chega ao personagem caído mostra o teste contra a morte', { tag: ['@MR-053', '@RN-03'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { m, table, campaignId, done } = await actsTable(browser, 'Caído com leitura lenta');
  try {
    await openSessionPage(m, campaignId);
    // Every read of the combat the page makes answers late, as on a busy table: the turn events that follow the drop to 0 arrive
    // before the reads they asked for, and a copy patched with them would hold the turn but not the hit points.
    await m.route(/CombatService\/GetEncounter/, async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 100));
      await route.continue();
    });
    await adjustVitalsRPC(m, campaignId, table.characterId, { hitPointsCurrent: 0 });
    let enc = await getEncounterRPC(m, campaignId);
    enc = await combatRPC(m, 'EndTurn', { campaignId, encounterId: enc.id, expectedCombatantId: enc.currentCombatantId });
    enc = await passTurnsTo(m, campaignId, 'Pensantus');
    expect(enc.combatants.find((c) => c.label === 'Pensantus')?.deathSaveDue).toBe(true);
    await expect(m.getByRole('heading', { name: 'Pensantus está caído: teste contra a morte' })).toBeVisible();
    await expect(m.getByRole('button', { name: 'Agir por Pensantus' })).toHaveCount(0);
  } finally {
    await done();
  }
});

test('o turno de um personagem caído começa pelo teste contra a morte e o mestre não o pula', { tag: ['@MR-053', '@RN-03'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { m, table, campaignId, done } = await actsTable(browser, 'Caído pelo mestre');
  try {
    await openSessionPage(m, campaignId);
    // He drops to 0 hit points during his own turn; the turn comes round again with the save due.
    await adjustVitalsRPC(m, campaignId, table.characterId, { hitPointsCurrent: 0 });
    let enc = await getEncounterRPC(m, campaignId);
    enc = await combatRPC(m, 'EndTurn', { campaignId, encounterId: enc.id, expectedCombatantId: enc.currentCombatantId });
    enc = await passTurnsTo(m, campaignId, 'Pensantus');
    await waitForEncounterRevision(m, enc);
    expect(enc.combatants.find((c) => c.label === 'Pensantus')?.deathSaveDue).toBe(true);

    await expect(m.getByRole('heading', { name: 'Pensantus está caído: teste contra a morte' })).toBeVisible();
    await expect(m.getByText('O turno de Pensantus só passa depois do teste.')).toBeVisible();
    await expect(m.getByRole('button', { name: 'Agir por Pensantus' })).toHaveCount(0);
    await expect(m.getByRole('button', { name: 'Passar sem rolar' })).toHaveCount(0);
    // The turn does not pass: the button is the dashed one and the bar says why.
    await expect(m.getByRole('button', { name: 'Próximo turno' })).toHaveAttribute('aria-disabled', 'true');
    await expect(m.getByText('Falta o teste contra a morte').first()).toBeVisible();

    // A typed d20 for the character (a physical die): the counts move and the turn can pass.
    await m.getByRole('button', { name: 'Digitar o resultado' }).click();
    await m.getByLabel(/Role 1d20 para o teste contra a morte/).fill('14');
    await m.getByRole('button', { name: 'Confirmar 14' }).click();
    await expect(m.getByRole('status').filter({ hasText: 'Teste contra a morte' }).first()).toBeVisible();
    await expect(m.getByRole('button', { name: 'Próximo turno' })).not.toHaveAttribute('aria-disabled', 'true');
    await m.getByRole('button', { name: 'Próximo turno' }).click();
    await expect(m.getByRole('heading', { name: 'Pensantus está caído: teste contra a morte' })).toHaveCount(0);
  } finally {
    await done();
  }
});
