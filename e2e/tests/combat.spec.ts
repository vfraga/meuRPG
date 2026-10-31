import { expect, type Browser, type BrowserContext, type Page } from '@playwright/test';
import { test } from './diagnostics';

import {
  adjustVitalsRPC,
  beginAttackCombatRPC,
  brisa,
  brisaAidSheet,
  brisaSheet,
  combatRPC,
  endTurnOf,
  getEncounterRPC,
  passTurnsTo,
  pensantusCasting,
  ragna,
  ragnaSheet,
  setGridRPC,
  tableForCombat,
  toren,
  torenSheet,
  type CombatTable,
  type Encounter,
} from './combat-support';
import type { CharacterBuild } from './support';
import { endOpenSessionRPC, openSessionPage, clickStartCombat, expectStartCombatOffered } from './live-session-support';
import { boxOf, callRPC, layoutSize, newSignedInContext, openActionTab, openSpentGroup, pensantus, viaTargetList } from './support';
import { placeTokenRPC } from './maps-support';
import { setTableRulesRPC } from './table-rules-support';
import { sq20, thirdPlayer, trapTable } from './trap-support';
import { masterLog, openCombatPanel, openOrderRow } from './combat-layout';
import { waitForEncounterRevision } from './revision-support';

// The combat on screen (Etapa 6, slice 6.5a, MR-013, RN-18 to RN-22): the
// master sets the grid and starts a combat, everybody rolls initiative, the
// turns go round, a player moves inside the reach, and the master ends it.
// The table and the NPCs come through the API; what is under test is the
// screens and what each audience sees (RN-10, RN-20).

test(
  'o mestre define a grade e inicia o combate; a iniciativa, os turnos e o fim aparecem para cada um',
  { tag: ['@MR-013', '@RN-19', '@RN-20', '@RN-21'] },
  async ({ browser }) => {
    test.setTimeout(180_000);
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
    const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 390, height: 844 } });
    const m = await master.newPage();
    const p = await player.newPage();
    let campaignId = '';
    try {
      await m.goto('/');
      await p.goto('/');
      const table = await tableForCombat(m, p, `Combate ${Date.now()}`, false);
      campaignId = table.campaignId;

      // A map without a grid: "Iniciar combate" sends the master to the grid page.
      await m.goto(`/campaigns/${campaignId}/session`);
      await clickStartCombat(m);
      await expect(m.getByText('Esse mapa ainda não tem grade.')).toBeVisible();
      // A map without a grid is no map for "Com mapa" (RN-25): that choice waits with its reason, and "Sem mapa" is the one chosen.
      await expect(m.getByRole('radio', { name: /^Com mapa/ })).toBeDisabled();
      await expect(m.getByRole('radio', { name: /Sem mapa \(teatro da mente\)/ })).toBeChecked();
      await m.getByRole('link', { name: 'Definir a grade' }).click();
      await expect(m.getByRole('heading', { name: 'Grade do mapa' })).toBeVisible();
      // Columns 3 is below the server's 4 to 200; 30 gives 21 rows for 2000 x 1400.
      await m.getByLabel('Quadrados de 1,5 m na largura').fill('3');
      await expect(m.getByText('Use um número inteiro de 4 a 200.')).toBeVisible();
      await expect(m.getByRole('button', { name: 'Salvar grade' })).toHaveAttribute('aria-disabled', 'true');
      await m.getByLabel('Quadrados de 1,5 m na largura').fill('20');
      await expect(m.getByText('20 × 14')).toBeVisible();
      await expect(m.getByText('30 m × 21 m')).toBeVisible();
      await m.getByRole('button', { name: 'Salvar grade' }).click();
      await expect(m).toHaveURL(/\/session$/);

      // Start with the party and three hidden goblins (and the captain).
      await clickStartCombat(m);
      await expect(m.getByRole('dialog', { name: 'Iniciar combate' })).toBeVisible();
      await expect(m.getByText('20 × 14 quadrados de 1,5 m')).toBeVisible();
      await expect(m.getByText('30 m × 21 m').first()).toBeVisible();
      for (let i = 0; i < 3; i++) {
        await m.getByRole('button', { name: 'Mais um Goblin', exact: true }).click();
      }
      await m.getByRole('button', { name: 'Mais um Capitão Goblin' }).click();
      await expect(m.getByText('Minion · vira Goblin 1, 2 e 3')).toBeVisible();
      await expect(m.getByText('Escondido no início').first()).toBeVisible();
      await m.getByRole('dialog').getByRole('button', { name: 'Iniciar combate' }).click();
      await expect(m.getByRole('heading', { level: 1, name: 'Sessão 1' })).toBeVisible();
      await expect(m.getByText('Os NPCs rolaram sozinhos.')).toBeVisible();
      await expect(m.getByRole('button', { name: 'Começar o combate' })).toHaveAttribute('aria-disabled', 'true');
      await expect(m.getByText('Falta a iniciativa de Pensantus.')).toBeVisible();

      // The player rolls in the app; the NPCs never show on their screen.
      await openSessionPage(p, campaignId);
      await expect(p.getByRole('heading', { name: 'Role a iniciativa' })).toBeVisible();
      await expect(p.getByText('Goblin')).toHaveCount(0);
      await p.getByRole('button', { name: 'Rolar no app' }).click();
      await expect(p.getByText('Esperando o mestre começar o combate')).toBeVisible();

      // Force a tie between two goblins, put the captain first, and break the tie on the screen.
      let enc = await getEncounterRPC(m, campaignId);
      const id = (label: string) => enc.combatants.find((c) => c.label === label)!.id;
      const face = async (label: string, d20Face: number) => {
        enc = await combatRPC(m, 'SubmitInitiative', { campaignId, encounterId: enc.id, combatantId: id(label), d20Face });
      };
      await face('Capitão Goblin', 20);
      // The player's roll stays what the app rolled, except that the master
      // may correct it: here, to make the order the same on every run.
      await face('Pensantus', 19);
      await face('Goblin 1', 7);
      await face('Goblin 2', 7);
      await face('Goblin 3', 3);
      const pen = enc.combatants.find((c) => c.label === 'Pensantus')!;
      const cap = enc.combatants.find((c) => c.label === 'Capitão Goblin')!;
      if (pen.initiative === cap.initiative) {
        // eslint-disable-next-line meurpg/rpc-revision-wait -- the player acts only after the master's own clicks, and the player's "Sua vez" heading asserted before that is the wait
        enc = await combatRPC(m, 'SetInitiativeOrder', { campaignId, encounterId: enc.id, combatantIds: [cap.id, pen.id] });
        await waitForEncounterRevision(m, enc);
      }
      await expect(m.getByText(/Empate em 7: Goblin \d e Goblin \d\. Escolha a ordem com as setas\./)).toBeVisible();
      // The first of the group can't go up: the arrow is a dashed outline that says so and does nothing.
      const firstUp = m.locator('[aria-label^="Subir Goblin"][aria-disabled="true"]');
      await expect(firstUp).toHaveCount(1);
      const first = ((await firstUp.getAttribute('aria-label')) ?? '').replace(/^Subir | na ordem$/g, '');
      await m.getByRole('button', { name: `Descer ${first} na ordem` }).click();
      await expect(m.getByText('Empate em 7: Goblin')).toHaveCount(0);
      await expect(m.getByText('Todos rolaram.')).toBeVisible();

      // Put the goblins on squares, then begin.
      for (const [label, col, row] of [['Goblin 1', 9, 9], ['Goblin 2', 14, 10], ['Goblin 3', 15, 4], ['Capitão Goblin', 11, 5]] as const) {
        enc = await combatRPC(m, 'MoveCombatant', { campaignId, encounterId: enc.id, combatantId: id(label), col, row });
      }
      await m.getByRole('button', { name: 'Começar o combate' }).click();
      await expect(m.getByRole('button', { name: 'Próximo turno' })).toBeVisible();
      await expect(m.getByText('Vez do Capitão Goblin').first()).toBeVisible();
      await expect(m.getByRole('region', { name: 'Combate', exact: true }).getByText('Rodada 1', { exact: true })).toBeVisible();

      // The captain is hidden: the player sees "Vez do mestre", never a goblin.
      await expect(p.getByRole('heading', { name: 'Vez do mestre' })).toBeVisible();
      await expect(p.getByText('Goblin')).toHaveCount(0);
      await expect(p.getByRole('listitem').filter({ hasText: 'Pensantus' }).first()).toBeVisible();

      // "Próximo turno": Pensantus, and the player is told.
      await m.getByRole('button', { name: 'Próximo turno' }).click();
      await expect(m.getByText('Vez do Pensantus').first()).toBeVisible();
      await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();

      // The player moves 2 right and 1 down (11,2 ft, 3,4 m: the straight line, RN-21) inside the circle, and is refused beyond it.
      await p.getByRole('button', { name: 'Mover' }).click();
      await expect(p.getByRole('heading', { name: 'Mover Pensantus' })).toBeVisible();
      const map = p.getByRole('group', { name: /Mapa de batalha/ });
      const box = await boxOf(map);
      const w = box.width / 20;
      const h = box.height / 14;
      const start = enc.combatants.find((c) => c.label === 'Pensantus')!;
      const own = (await getEncounterRPC(p, campaignId)).combatants.find((c) => c.mine)!;
      const at = (dc: number, dr: number) => ({ x: ((own.col ?? 0) + dc + 0.5) * w, y: ((own.row ?? 0) + dr + 0.5) * h });
      expect(start).toBeTruthy();
      await map.click({ position: at(8, 1) });
      await expect(p.getByRole('alert').filter({ hasText: 'Longe demais' })).toBeVisible();
      await expect(p.getByRole('button', { name: 'Mover para cá' })).toHaveAttribute('aria-disabled', 'true');
      await map.click({ position: at(2, 1) });
      await expect(p.getByRole('status').filter({ hasText: 'Mover 3,4 m' })).toContainText('Depois restam 4,1 m.');
      await p.getByRole('button', { name: 'Mover para cá' }).click();
      await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
      await expect.poll(async () => (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Pensantus')?.col).toBe((own.col ?? 0) + 2);

      // The master asks before ending, and the summary shows what happened.
      await m.getByRole('button', { name: 'Encerrar combate' }).click();
      await expect(m.getByRole('alertdialog', { name: 'Encerrar o combate?' })).toBeVisible();
      await expect(m.getByRole('button', { name: 'Cancelar' })).toBeFocused();
      await m.getByRole('button', { name: 'Encerrar combate' }).last().click();
      await expect(m.getByRole('heading', { name: 'Combate encerrado' })).toBeVisible();
      await expect(p.getByRole('heading', { name: 'O combate acabou' })).toBeVisible(); // the player's one end-of-combat card
      await m.getByRole('button', { name: 'Voltar à sessão' }).click();
      await expectStartCombatOffered(m);
    } finally {
      if (campaignId) {
        await endOpenSessionRPC(m, campaignId);
      }
      await master.close();
      await player.close();
    }
  },
);

test('um jogador que digita os dados rola a iniciativa com o número do dado físico', { tag: ['@MR-013', '@RN-18'] }, async ({ browser }) => {
  test.setTimeout(120_000);
  const master = await newSignedInContext(browser, 'Mestre Teste');
  const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 390, height: 844 } });
  const m = await master.newPage();
  const p = await player.newPage();
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Dados físicos ${Date.now()}`);
    campaignId = table.campaignId;
    const pref = await p.request.post('/meurpg.campaigns.v1.CampaignService/SetMyDicePreference', {
      data: { campaignId, preference: 'DICE_PREFERENCE_PHYSICAL' },
      headers: { 'Connect-Protocol-Version': '1' },
    });
    expect(pref.ok()).toBeTruthy();
    await combatRPC(m, 'StartEncounter', {
      campaignId,
      name: 'Dados',
      participants: [{ characterId: table.goblinId, count: 1 }],
    });
    await openSessionPage(p, campaignId);
    // While the campaign lets each player choose, both ways are offered (RN-18); the saved choice only
    // decides which one is the filled button.
    await expect(p.getByRole('button', { name: 'Rolar no app' })).toBeVisible();
    await p.getByRole('button', { name: 'Digitar o resultado' }).click();
    await p.getByLabel(/Role 1d20 para a iniciativa/).fill('99');
    await expect(p.getByText('Digite um número de 1 a 20')).toBeVisible();
    await p.getByLabel(/Role 1d20 para a iniciativa/).fill('12');
    await expect(p.getByText('12 + 3 = 15 · dado físico')).toBeVisible();
    await p.getByRole('button', { name: 'Confirmar 12' }).click();
    await expect(p.getByText('Esperando o mestre começar o combate')).toBeVisible();
    await expect(p.getByText('1d20 (12) + 3 = 15')).toBeVisible();
    const enc = await getEncounterRPC(m, campaignId);
    expect(enc.combatants.find((c: Encounter['combatants'][number]) => c.label === 'Pensantus')?.initiative).toBe(15);
    await setGridRPC(m, campaignId, table.mapId, 20);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
});

// Acting in a combat (Etapa 6, slice 6.5b, MR-012, MR-014, RN-18 to RN-20): the
// player's "Sua vez" groups and attack sheet, the master's card with the armor
// class and the damage to apply, undo, "Dano/Cura" and the log. Pensantus
// carries a dagger and Raio de Fogo; the Capitão a scimitar and chain mail. The
// d20 is typed where the test needs a sure hit; the app's own roll is checked
// for what it shows either way.

interface ActingTable {
  m: Page;
  p: Page;
  table: CombatTable;
  campaignId: string;
  done: () => Promise<void>;
}

/** A combat with Pensantus, the Capitão and two Goblins, begun with the given d20 faces. */
async function actingTable(
  browser: Browser,
  name: string,
  faces: Record<string, number>,
  hidden: string[] = [],
  phone = { width: 390, height: 844 },
  character: { build?: CharacterBuild; sheet?: Record<string, unknown> } = {},
  at?: Record<string, [number, number]>,
): Promise<ActingTable> {
  const master: BrowserContext = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
  const player: BrowserContext = await newSignedInContext(browser, 'Jogador Teste', { viewport: phone });
  const m = await master.newPage();
  const p = await player.newPage();
  await m.goto('/');
  await p.goto('/');
  const table = await tableForCombat(m, p, `${name} ${Date.now()}`, true, true, character);
  await beginAttackCombatRPC(m, table, faces, at, hidden);
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

const playerFirst = { Pensantus: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 };
const captainFirst = { Pensantus: 15, 'Capitão Goblin': 20, 'Goblin 1': 5, 'Goblin 2': 4 };

test('o jogador ataca com dados físicos: digita o d20 e a soma do dano, e o goblin é derrotado', { tag: ['@MR-012', '@MR-014', '@RN-18', '@RN-20'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const { m, p, campaignId, done } = await actingTable(browser, 'Ataque físico', playerFirst);
  try {
    await openSessionPage(p, campaignId);
    await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
    // The tabs: "Ataques" is shown first, with the cantrip; the standard actions (not Atacar or Conjurar) are in "Outras"; the movement is "Mover".
    const groups = p.getByRole('region', { name: 'O que você pode fazer' });
    await expect(groups.getByRole('tab', { name: /^Ataques/ })).toHaveAttribute('aria-selected', 'true');
    await expect(groups.getByText('+6 para acertar · 1d10 de fogo · alcance 36 m')).toBeVisible();
    const other = await openActionTab(p, 'Outras');
    await expect(other.getByRole('button', { name: 'Disparada' })).toBeVisible();
    await expect(groups.getByRole('button', { name: 'Conjurar uma magia' })).toHaveCount(0);
    await expect(p.getByRole('button', { name: 'Mover' })).toBeVisible();
    await openActionTab(p, 'Ataques');
    // No armor class on the player's screen.
    await expect(p.getByText(/contra CA/)).toHaveCount(0);

    await p.getByRole('button', { name: 'Atacar com Raio de Fogo' }).click();
    const sheet = p.getByRole('dialog', { name: 'Atacar com Raio de Fogo' });
    await viaTargetList(p, sheet);
    await expect(sheet).toBeVisible();
    await expect(sheet.getByRole('heading', { name: 'Atacar com Raio de Fogo' })).toBeFocused();
    await expect(sheet.getByText('Ferido')).toHaveCount(0);
    await sheet.locator('label', { hasText: 'Goblin 1' }).click();
    await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
    await expect(sheet.getByRole('heading', { name: 'Digite o resultado do dado' })).toBeVisible();
    await sheet.getByLabel(/Role 1d20 para Raio de Fogo/).fill('27');
    await expect(sheet.getByText('Digite um número de 1 a 20')).toBeVisible();
    await expect(sheet.getByRole('button', { name: 'Confirmar' })).toHaveAttribute('aria-disabled', 'true');
    await sheet.getByLabel(/Role 1d20 para Raio de Fogo/).fill('20');
    await expect(sheet.getByText('20 + 6 = 26 · dado físico')).toBeVisible();
    await sheet.getByRole('button', { name: 'Confirmar 20' }).click();
    // A natural 20 is a critical hit: the dice double.
    await expect(sheet.locator('.pill', { hasText: 'Crítico' })).toBeVisible();
    // With the app's dice the server rolls them: the rule's line comes only once the player types a physical roll.
    await expect(sheet.getByText('Acerto crítico: o dano segue a regra da mesa.')).toBeVisible();
    await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
    await expect(sheet.getByText('Acerto crítico: role os dados duas vezes (2d10 no total).')).toBeVisible();
    await sheet.getByLabel(/Role 2d10/).fill('21');
    await expect(sheet.getByText('Digite um número de 2 a 20')).toBeVisible();
    await sheet.getByLabel(/Role 2d10/).fill('12');
    await sheet.getByRole('button', { name: 'Confirmar 12' }).click();
    await expect(sheet.getByText('Goblin 1 derrotado')).toBeVisible();
    await expect(sheet.getByText('12 de fogo')).toBeVisible();
    await expect(sheet.getByText('Sua ação foi usada. Truque: nenhum espaço de magia foi gasto.')).toBeVisible();
    await expect(sheet.getByRole('button', { name: 'Voltar à sua vez' })).toBeFocused();
    await sheet.getByRole('button', { name: 'Voltar à sua vez' }).click();

    // The goblin is defeated for everyone, the action is used, and "Encerrar turno" does not ask.
    const enc = await getEncounterRPC(m, campaignId);
    expect(enc.combatants.find((c) => c.label === 'Goblin 1')?.defeated).toBe(true);
    await openActionTab(p, 'Ataques');
    await expect((await openSpentGroup(p, 'Ação')).getByText('Ação já usada').first()).toBeVisible();
    // The log: his sentence, with the damage and the defeat.
    await p.getByRole('button', { name: 'Abrir o registro do combate' }).click();
    await expect(p.getByRole('log', { name: 'Registro do combate' })).toContainText('Pensantus atira no Goblin 1 com o Raio de Fogo: crítico, 12 de dano. Goblin 1 derrotado');
  } finally {
    await done();
  }
});

test('o jogador rola o ataque no app: o resultado mostra a conta e nunca a CA', { tag: ['@MR-012', '@MR-014', '@RN-20'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const { p, campaignId, done } = await actingTable(browser, 'Ataque no app', playerFirst);
  try {
    await openSessionPage(p, campaignId);
    await p.getByRole('button', { name: 'Atacar com Raio de Fogo' }).click();
    const sheet = p.getByRole('dialog', { name: 'Atacar com Raio de Fogo' });
    await viaTargetList(p, sheet);
    await expect(sheet.getByRole('radio', { name: /Capitão Goblin/ })).toBeEnabled();
    await sheet.locator('label', { hasText: 'Capitão Goblin' }).click();
    await sheet.getByRole('button', { name: 'Rolar no app' }).click();
    await expect(sheet.getByText(/1d20 \(\d+\) \+ 6 = \d+/)).toBeVisible();
    await expect(sheet.getByText(/Acertou|Crítico|Errou/).first()).toBeVisible();
    // The hit decides the next step: wait for either one before choosing, since isVisible() doesn't wait.
    const rollDamage = sheet.getByRole('button', { name: /Rolar \dd10 no app/ });
    const missed = sheet.getByText('Sem dano: o ataque errou.');
    await expect(rollDamage.or(missed)).toBeVisible();
    if (await rollDamage.isVisible()) {
      await sheet.getByRole('button', { name: /Rolar \dd10 no app/ }).click();
      await expect(sheet.getByText(/\d+d10 \([\d, ]+\) = \d+ de fogo/)).toBeVisible();
    } else {
      await expect(sheet.getByText('Sem dano: o ataque errou.')).toBeVisible();
    }
    await expect(sheet.getByText(/CA/)).toHaveCount(0);
    await sheet.getByRole('button', { name: 'Voltar à sua vez' }).click();
  } finally {
    await done();
  }
});

test('o jogador encerra o turno: pergunta com a ação livre, e a Disparada dobra o movimento', { tag: ['@MR-014', '@RN-21'] }, async ({ browser }) => {
  test.setTimeout(120_000);
  const { m, p, campaignId, done } = await actingTable(browser, 'Turno do jogador', playerFirst);
  try {
    await openSessionPage(p, campaignId);
    await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
    // "Encerrar turno" is an outline while the action is free, and asks before ending.
    await p.getByRole('button', { name: 'Encerrar turno' }).click();
    await expect(p.getByRole('alertdialog', { name: /Ainda tem ação disponível/ })).toBeVisible();
    await expect(p.getByRole('button', { name: 'Voltar' })).toBeFocused();
    await p.getByRole('button', { name: 'Voltar' }).click();
    expect((await getEncounterRPC(m, campaignId)).currentCombatantId).toBeTruthy();
    const before = (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.mine === undefined && c.label === 'Pensantus');
    expect(before?.movementLeftFt).toBe(25);

    // The Disparada spends the action and doubles the movement left.
    await openActionTab(p, 'Outras');
    await p.getByRole('button', { name: 'Disparada' }).click();
    await expect((await openSpentGroup(p, 'Ação')).getByText('Ação já usada').first()).toBeVisible();
    await expect(p.getByText('Restam 15,0 m')).toBeVisible();
    // The speed already includes the Dash: the total is the doubled one, never doubled again.
    await expect(p.getByText('Restam 30,0 m')).toHaveCount(0);
    const after = (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Pensantus');
    expect(after?.movementLeftFt).toBe(50);
    await expect(p.getByRole('log', { name: 'Registro do combate' })).toHaveCount(0);

    // The action is used and the bonus action still free: still no filled button, but no question either.
    await p.getByRole('button', { name: 'Encerrar turno' }).click();
    await expect(p.getByRole('heading', { name: 'Vez do Capitão Goblin' })).toBeVisible();
  } finally {
    await done();
  }
});

test('o goblin do mestre ataca: ele vê a CA, aplica o dano (os PV do jogador mudam), descarta outro e desfaz', { tag: ['@MR-012', '@MR-014', '@RN-02', '@RN-20'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const { m, p, campaignId, done } = await actingTable(browser, 'Ataque do mestre', captainFirst);
  try {
    await openSessionPage(m, campaignId);
    await openSessionPage(p, campaignId);
    await expect(m.getByRole('heading', { name: /^(Ações do|Vez do) Capitão Goblin$/ })).toBeVisible();
    const card = m.getByRole('region', { name: /^(Ações do|Vez do) Capitão Goblin$/ });
    await expect(card.getByText('Pontos de vida')).toBeVisible();
    await expect(card.getByText('CA', { exact: true })).toBeVisible();
    await expect(m.getByText('CA 13').first()).toBeVisible(); // Pensantus, in the order
    await expect(p.getByText(/CA \d+/)).toHaveCount(0); // never on the player's screen

    // The first attack: the master types a 20 (a sure hit) and sees the armor class it met.
    await card.getByRole('button', { name: 'Digitar o resultado' }).click();
    await card.getByLabel(/Role 1d20 para Cimitarra/).fill('20');
    await card.getByRole('button', { name: 'Confirmar 20' }).click();
    await expect(card.getByText(/contra CA 13 d[ao] Pensantus/)).toBeVisible();
    await card.getByRole('button', { name: 'Rolar dano' }).click();
    await expect(card.getByRole('button', { name: /Aplicar \d+ de dano/ })).toBeVisible();
    await expect(card.getByText(/Pensantus: 23 de 23 PV, depois \d+/)).toBeVisible();
    await expect(m.getByText(/Falta aplicar \d+ de dano/).first()).toBeVisible();

    // Passing the turn with a damage waiting asks first; "Voltar" keeps the turn.
    await m.getByRole('button', { name: 'Próximo turno' }).click();
    await expect(m.getByRole('alertdialog', { name: /Há dano sem aplicar/ })).toBeVisible();
    await expect(m.getByRole('button', { name: 'Voltar' })).toBeFocused();
    await m.getByRole('button', { name: 'Voltar' }).click();
    await expect(m.getByRole('heading', { name: /^(Ações do|Vez do) Capitão Goblin$/ })).toBeVisible();

    // Applying changes the player's hit points, and the log says so.
    await card.getByRole('button', { name: /Aplicar \d+ de dano/ }).click();
    await expect.poll(async () => (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Pensantus')?.hitPointsCurrent).toBeLessThan(23);
    await expect(await masterLog(m)).toContainText('Capitão Goblin ataca');

    // A second attack of the same turn (the master may attack again): the damage is discarded, after a question.
    await card.getByRole('button', { name: 'Digitar o resultado' }).click();
    await card.getByLabel(/Role 1d20 para Cimitarra/).fill('20');
    await card.getByRole('button', { name: 'Confirmar 20' }).click();
    await card.getByRole('button', { name: 'Rolar dano' }).click();
    await card.getByRole('button', { name: 'Não aplicar' }).click();
    await expect(card.getByText(/Descartar o dano de \d+\?/)).toBeVisible();
    await expect(card.getByRole('button', { name: 'Voltar' })).toBeFocused();
    await card.getByRole('button', { name: 'Descartar' }).click();
    const hp = (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Pensantus')?.hitPointsCurrent;
    expect(hp).toBeLessThan(23);

    // Undo takes back the last action only, named in the question; the damage discarded comes back to be decided.
    await openCombatPanel(m, 'Registro');
    await m.getByRole('button', { name: 'Desfazer última ação' }).first().click();
    await expect(m.getByText(/Desfazer o ataque do Capitão Goblin ao Pensantus/).or(m.getByText(/Desfazer o ataque do Capitão Goblin à Pensantus/))).toBeVisible();
    await expect(m.getByRole('button', { name: 'Voltar' }).last()).toBeFocused();
    await m.getByRole('button', { name: 'Desfazer', exact: true }).click();
    await expect(m.getByRole('alertdialog', { name: 'Desfazer a última ação' })).toHaveCount(0);
  } finally {
    await done();
  }
});

test('o mestre ajusta os PV de um NPC em "Dano/Cura", e o jogador não vê um goblin escondido no registro', { tag: ['@MR-012', '@RN-02', '@RN-20'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const { m, p, campaignId, table, done } = await actingTable(browser, 'Dano e cura', { Pensantus: 15, 'Capitão Goblin': 12, 'Goblin 1': 20, 'Goblin 2': 4 }, ['Goblin 1']);
  try {
    await openSessionPage(m, campaignId);
    await openSessionPage(p, campaignId);
    const enc = await getEncounterRPC(m, campaignId);
    const goblin2 = enc.combatants.find((c) => c.label === 'Goblin 2')!;

    // The hidden Goblin 1 is on turn: the player sees "Vez do mestre", and its attack never reaches their log.
    await expect(p.getByRole('heading', { name: 'Vez do mestre' })).toBeVisible();
    const rolled = await combatRPC(m, 'RollAttack', {
      campaignId,
      encounterId: enc.id,
      attackerId: enc.combatants.find((c) => c.label === 'Goblin 1')!.id,
      attackKey: 'basic:0',
      targetId: enc.combatants.find((c) => c.label === 'Pensantus')!.id,
      d20Face: 3,
    });
    await waitForEncounterRevision([m, p], rolled);
    await expect(await masterLog(m)).toContainText('Goblin 1');
    await expect(await masterLog(m)).toContainText('Só o mestre vê');
    await p.getByRole('button', { name: 'Abrir o registro do combate' }).click();
    await expect(p.getByRole('log', { name: 'Registro do combate' })).toBeVisible();
    await expect(p.getByRole('log', { name: 'Registro do combate' })).not.toContainText('Goblin 1');
    await expect(p.getByRole('log', { name: 'Registro do combate' })).not.toContainText('Só o mestre vê');
    await p.keyboard.press('Escape');

    // "Dano/Cura" on Goblin 2: 3 of damage, then a heal back.
    await openOrderRow(m, 'Goblin 2');
    await m.getByRole('button', { name: 'Dano/Cura: Goblin 2' }).click();
    const dialog = m.getByRole('dialog', { name: 'Dano ou cura em Goblin 2' });
    await expect(dialog.getByRole('heading', { name: 'Dano ou cura em Goblin 2' })).toBeFocused();
    await expect(dialog.getByText('Agora: 7 de 7 PV')).toBeVisible();
    await dialog.getByLabel('Dano sofrido').fill('3');
    await expect(dialog.getByText('Depois: 4 de 7 PV')).toBeVisible();
    await dialog.getByRole('button', { name: 'Salvar ajuste' }).click();
    await expect(dialog).toHaveCount(0);
    await expect.poll(async () => (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.id === goblin2.id)?.hitPointsCurrent).toBe(4);
    await expect(await masterLog(m)).toContainText('Goblin 2 perdeu 3 PV, por ajuste do mestre, agora com 4 PV');
    // The player sees only the state word of the NPC, never the numbers.
    await expect(p.getByText('4 de 7')).toHaveCount(0);

    await openOrderRow(m, 'Goblin 2');
    await m.getByRole('button', { name: 'Dano/Cura: Goblin 2' }).click();
    await m.getByRole('dialog').locator('label', { hasText: /^\s*Cura\s*$/ }).click();
    await m.getByLabel('PV curados').fill('9');
    await expect(m.getByText('Depois: 7 de 7 PV')).toBeVisible();
    await m.getByRole('button', { name: 'Salvar ajuste' }).click();
    await expect.poll(async () => (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.id === goblin2.id)?.hitPointsCurrent).toBe(7);
    expect(table.campaignId).toBe(campaignId);
  } finally {
    await done();
  }
});

test('o jogador pode conjurar Escudo: o mestre responde por ele, o acerto vira erro, ou segue sem Escudo', { tag: ['@MR-012', '@MR-014', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const { m, p, campaignId, done } = await actingTable(browser, 'Escudo', captainFirst);
  try {
    await openSessionPage(m, campaignId);
    await openSessionPage(p, campaignId);
    const card = m.getByRole('region', { name: /^(Ações do|Vez do) Capitão Goblin$/ });
    const roll = async (face: string) => {
      await card.getByRole('button', { name: 'Digitar o resultado' }).click();
      await card.getByLabel(/Role 1d20 para Cimitarra/).fill(face);
      await card.getByRole('button', { name: `Confirmar ${face}` }).click();
    };

    // A hit that is not critical on a character who can cast Escudo waits for the reaction.
    await roll('15');
    const queue = m.getByRole('region', { name: 'Reações a um ataque' });
    await expect(queue.getByText('Escudo Arcano · o ataque atingiu Pensantus')).toBeVisible();
    await expect(card.getByText('Espere a reação de Pensantus.')).toBeVisible();
    await expect(card.getByRole('button', { name: 'Rolar dano' })).toBeDisabled();
    // The turn waits for the reaction: the pass is not offered, and says why.
    await expect(m.getByRole('button', { name: 'Próximo turno' })).toBeDisabled();
    await expect(m.getByText('Esperando a reação de Pensantus')).toBeVisible();
    // The answers have the same size.
    const use = await layoutSize(queue.getByRole('button', { name: 'Usar pelo jogador' }));
    const skip = await layoutSize(queue.getByRole('button', { name: 'Deixar passar pelo jogador' }));
    expect(use.height).toBe(skip.height);

    // Without Escudo the hit goes on to "Rolar dano" and its apply or discard.
    await queue.getByRole('button', { name: 'Deixar passar pelo jogador' }).click();
    await card.getByRole('button', { name: 'Rolar dano' }).click();
    await expect(card.getByRole('button', { name: /Aplicar \d+ de dano/ })).toBeVisible();
    await card.getByRole('button', { name: 'Não aplicar' }).click();
    await card.getByRole('button', { name: 'Descartar' }).click();

    // Another attack: 11 + 3 = 14 reaches 13, but not 18: Escudo (+5 na CA) stops it.
    await roll('11');
    await expect(card.getByText('Espere a reação de Pensantus.')).toBeVisible();
    await queue.getByRole('button', { name: 'Usar pelo jogador' }).click();
    await expect(card.locator('.pill', { hasText: 'Errou: o Escudo Arcano segurou' })).toBeVisible();
    await expect(card.getByRole('button', { name: /Rolar dano|Aplicar/ })).toHaveCount(0);
    await expect(await masterLog(m)).toContainText('o Escudo Arcano segurou');
    const pens = (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Pensantus');
    expect(pens?.hitPointsCurrent).toBe(23);
  } finally {
    await done();
  }
});

test('numa tela de 320 × 568: a pergunta de encerrar cabe numa linha por botão, o erro do dado físico fica à vista e o rodapé tem fundo', { tag: ['@MR-014', '@a11y'] }, async ({ browser }) => {
  test.setTimeout(120_000);
  const { p, campaignId, done } = await actingTable(browser, 'Tela pequena', playerFirst, [], { width: 320, height: 568 });
  try {
    await openSessionPage(p, campaignId);
    // The two answers keep one line each: the same height, or two full rows of the same height.
    await p.getByRole('button', { name: 'Encerrar turno' }).click();
    const back = await layoutSize(p.getByRole('button', { name: 'Voltar' }));
    const end = await layoutSize(p.getByRole('button', { name: 'Encerrar turno' }));
    expect(back.height).toBe(end.height);
    expect(end.height).toBeLessThanOrEqual(48);
    expect(await p.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    await p.getByRole('button', { name: 'Voltar' }).click();

    // The physical roll's error is in view, above the sticky footer, not under it.
    await p.getByRole('button', { name: 'Atacar com Raio de Fogo' }).click();
    await viaTargetList(p, p.getByRole('dialog'));
    await p.locator('label', { hasText: 'Goblin 1' }).click();
    await p.getByRole('button', { name: 'Digitar o resultado' }).click();
    await p.getByLabel(/Role 1d20/).fill('27');
    const alert = p.getByRole('alert').filter({ hasText: 'Digite um número de 1 a 20' });
    await expect(alert).toBeInViewport({ ratio: 1 });
    const confirm = await boxOf(p.getByRole('button', { name: 'Confirmar' }));
    const err = await boxOf(alert);
    expect(err.y + err.height).toBeLessThanOrEqual(confirm.y);

    // The result's footer has its own band: "Voltar à sua vez" is whole and in view.
    await p.getByLabel(/Role 1d20/).fill('20');
    await p.getByRole('button', { name: 'Confirmar 20' }).click();
    await p.getByRole('button', { name: 'Digitar o resultado' }).click();
    await p.getByLabel(/Role 2d10/).fill('12');
    await p.getByRole('button', { name: 'Confirmar 12' }).click();
    await expect(p.getByRole('button', { name: 'Voltar à sua vez' })).toBeInViewport({ ratio: 1 });
  } finally {
    await done();
  }
});

// Casting, the fallen, reactions, conditions and class features (Etapa 6, slice
// 6.5c, MR-012, MR-014, RN-02, RN-03, RN-22): the player's cast sheet, the death
// saves and the master's confirmation, Escudo answered on both screens,
// the conditions and the concentration, another amount of damage, Extra Attack,
// Retomar o Fôlego and the opportunity attack.

const casting = { sheet: pensantusCasting };
const vitalsOf = async (m: Page, campaignId: string, label: string) =>
  (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === label)!;

test('o jogador conjura Mísseis Mágicos repartindo os dardos: o espaço some, o dano cai nos alvos e o aviso do último espaço aparece', { tag: ['@MR-014', '@RN-02'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const { m, p, table, campaignId, done } = await actingTable(browser, 'Mísseis', playerFirst, [], undefined, casting);
  try {
    // Three of four 1st-level slots and both 2nd-level ones used: this cast takes the last slot there is.
    await adjustVitalsRPC(m, campaignId, table.characterId, { spellSlotsUsed: [{ level: 1, used: 3 }, { level: 2, used: 2 }] });
    await openSessionPage(p, campaignId);
    await openActionTab(p, 'Magias');
    await p.getByRole('button', { name: 'Conjurar Mísseis Mágicos' }).click();
    const sheet = p.getByRole('dialog', { name: 'Conjurar Mísseis Mágicos' });
    await viaTargetList(p, sheet);
    await expect(sheet.getByRole('heading', { name: 'Conjurar Mísseis Mágicos' })).toBeFocused();
    await expect(sheet.getByRole('radio', { name: /1º nível/ })).toBeChecked();
    await expect(sheet.getByText('1 livre de 4')).toBeVisible();
    await expect(sheet.getByRole('radio', { name: /2º nível/ })).toBeDisabled();
    await expect(sheet.getByText('Sem espaço livre')).toBeVisible();
    await expect(sheet.getByText('É o seu último espaço de 1º nível: depois dele, o Escudo Arcano fica sem espaço.')).toBeVisible();
    // The darts: the button waits until all three are placed.
    const cast = sheet.getByRole('button', { name: 'Conjurar Mísseis Mágicos' });
    await expect(cast).toHaveAttribute('aria-disabled', 'true');
    await expect(sheet.getByText('Distribua todos os dardos: 0 de 3.')).toBeVisible();
    const more = (who: string) => sheet.getByRole('button', { name: `Pôr um dardo em ${who}` });
    await more('Capitão Goblin').click();
    await more('Capitão Goblin').click();
    await more('Goblin 1').click();
    await expect(sheet.getByText('3 de 3 dardos distribuídos.')).toBeVisible();
    await expect(more('Goblin 2')).toBeDisabled();
    await cast.click();
    // Spent at the cast, before any damage is rolled.
    await expect(sheet.getByRole('heading', { name: 'Você conjurou Mísseis Mágicos' })).toBeVisible();
    await expect(sheet.getByText('Espaços de 1º nível: 0 livres de 4')).toBeVisible();
    await expect(sheet.getByText('Escudo Arcano indisponível: sem espaço de 1º nível.')).toBeVisible();
    await expect(sheet.getByText('Sua ação foi usada.')).toBeVisible();
    await sheet.getByRole('button', { name: 'Rolar o dano no app' }).click();
    await expect(sheet.getByText(/Dardo 1: 1d4 \(\d\) \+ 1 = \d/).first()).toBeVisible();
    await expect(sheet.getByText(/Dardo 2:/)).toBeVisible();
    await expect(sheet.getByText(/\d+ de energia/).first()).toBeVisible();
    await expect(sheet.getByRole('button', { name: 'Voltar à sua vez' })).toBeFocused();
    await sheet.getByRole('button', { name: 'Voltar à sua vez' }).click();
    // The goblins took the damage; the log says it.
    expect((await vitalsOf(m, campaignId, 'Capitão Goblin')).hitPointsCurrent).toBeLessThan(23);
    await openActionTab(p, 'Magias');
    await expect((await openSpentGroup(p, 'Ação')).getByText('Ação já usada').first()).toBeVisible();
    await p.getByRole('button', { name: 'Abrir o registro do combate' }).click();
    await expect(p.getByRole('log', { name: 'Registro do combate' })).toContainText('Pensantus conjura Mísseis Mágicos (1º nível): 2 dardos no Capitão Goblin');
  } finally {
    await done();
  }
});

// Pensantus's token stands at column 6, row 8 of the 20-column road (the square of his map token, 25 % and 54 %).
const besidePensantus = { 'Capitão Goblin': [11, 2], 'Goblin 1': [6, 7], 'Goblin 2': [7, 7] } as Record<string, [number, number]>;

test('um cone sai de quem conjura: "Apontar para…" dá a direção, a lista diz quem está na área e o dano é rolado uma vez', { tag: ['@MR-014'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const { m, p, campaignId, done } = await actingTable(browser, 'Resistência', playerFirst, [], undefined, casting, besidePensantus);
  try {
    await openSessionPage(p, campaignId);
    await openActionTab(p, 'Magias');
    await p.getByRole('button', { name: 'Conjurar Mãos Flamejantes' }).click();
    const sheet = p.getByRole('dialog', { name: 'Conjurar Mãos Flamejantes' });
    // Step 1: the direction, from the list of the creatures he sees (the same place as a tap).
    await expect(sheet.getByText('Para onde?')).toBeVisible();
    await expect(sheet.getByRole('application', { name: 'Mapa: escolha a direção da Mãos Flamejantes' })).toBeFocused();
    await expect(sheet.getByRole('button', { name: 'Confirmar direção' })).toHaveAttribute('aria-disabled', 'true');
    await sheet.getByRole('button', { name: 'Apontar para…' }).click();
    await sheet.getByRole('listbox', { name: 'Apontar para…' }).getByRole('option', { name: /Goblin 1/ }).click();
    await sheet.getByRole('button', { name: 'Confirmar direção' }).click();
    // Step 2: who the server says is inside (the caster's square is not).
    await expect(sheet.getByRole('heading', { name: 'Quem está na área' })).toBeFocused();
    await expect(sheet.locator('app-area-list li')).toHaveCount(2);
    await expect(sheet.getByText('2 criaturas', { exact: true })).toBeVisible();
    await sheet.getByRole('button', { name: 'Conjurar Mãos Flamejantes' }).click();
    // The server rolls each save: the player reads the outcome and the DC.
    await expect(sheet.getByRole('heading', { name: /Mãos Flamejantes conjurad/ })).toBeVisible();
    await expect(sheet.getByText(/Falhou|Resistiu/).first()).toBeVisible();
    await expect(sheet.getByText('CD 14').first()).toBeVisible(); // an NPC's dice stay with the master (RN-20)
    await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
    await sheet.getByLabel(/Role 3d6/).fill('10');
    await sheet.getByRole('button', { name: 'Confirmar 10' }).click();
    await expect(sheet.getByRole('button', { name: 'Voltar à sua vez' })).toBeVisible();
    await sheet.getByRole('button', { name: 'Voltar à sua vez' }).click();
    const goblins = (await getEncounterRPC(m, campaignId)).combatants.filter((c) => c.label.startsWith('Goblin'));
    // One roll of 10 for both: a goblin that failed took 10 (and fell at 7 hit points), one that saved took half.
    expect(goblins.some((g) => g.defeated || (g.hitPointsCurrent ?? 7) < 7)).toBe(true);
  } finally {
    await done();
  }
});

test('caído: o teste contra a morte (sucesso), e com três falhas só o mestre vê "Morrendo" e confirma a morte', { tag: ['@MR-014', '@RN-03'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { m, p, table, campaignId, done } = await actingTable(browser, 'Caído', playerFirst);
  try {
    await openSessionPage(m, campaignId);
    await openSessionPage(p, campaignId);
    // He drops to 0 hit points during his own turn: nothing is owed until the next one.
    await adjustVitalsRPC(m, campaignId, table.characterId, { hitPointsCurrent: 0 });
    let enc = await getEncounterRPC(m, campaignId);
    enc = await combatRPC(m, 'EndTurn', { campaignId, encounterId: enc.id, expectedCombatantId: enc.currentCombatantId });
    enc = await passTurnsTo(m, campaignId, 'Pensantus');
    await waitForEncounterRevision(p, enc);
    expect(enc.combatants.find((c) => c.label === 'Pensantus')?.deathSaveDue).toBe(true);

    // Round 2: the hero says so, the marks are empty, and he can't end the turn before rolling.
    await expect(p.getByRole('heading', { name: 'Pensantus está caído' })).toBeVisible();
    await expect(p.getByText('É a sua vez, mas com 0 de 23 pontos de vida você não age. Role o teste contra a morte.')).toBeVisible();
    await expect(p.getByText('0 de 3')).toHaveCount(2);
    await expect(p.getByRole('button', { name: 'Encerrar turno' })).toHaveCount(0);
    await expect(p.getByRole('button', { name: 'Atacar com Raio de Fogo' })).toHaveCount(0);
    await expect(p.getByText('Enquanto estiver caído')).toBeVisible();
    await p.getByRole('button', { name: 'Digitar o resultado' }).click();
    await p.getByLabel(/Role 1d20 para o teste contra a morte/).fill('14');
    await p.getByRole('button', { name: 'Confirmar 14' }).click();
    await expect(p.getByRole('status').filter({ hasText: 'Teste contra a morte: 14 · dado físico. Sucesso.' })).toBeVisible();
    await expect(p.getByText('1 de 3').first()).toBeVisible();
    // The save is done: the one filled button is "Encerrar turno".
    await endTurnOf(p, m, campaignId, 'Pensantus');
    // The master's log has the roll; the other players would read only the outcome.
    await expect(await masterLog(m)).toContainText('Pensantus rola o teste contra a morte: 14 · dado físico, sucesso (1 sucesso, 0 falhas)');

    // Round 3: a natural 1 counts two failures; round 4: a 5 is the third.
    enc = await passTurnsTo(m, campaignId, 'Pensantus');
    await waitForEncounterRevision([m, p], enc);
    await p.getByRole('button', { name: 'Digitar o resultado' }).click();
    await p.getByLabel(/Role 1d20 para o teste contra a morte/).fill('1');
    await p.getByRole('button', { name: 'Confirmar 1' }).click();
    await expect(p.getByText(/Teste contra a morte: 1 · dado físico\. Falha: um 1 conta duas falhas\./)).toBeVisible();
    await endTurnOf(p, m, campaignId, 'Pensantus');
    enc = await passTurnsTo(m, campaignId, 'Pensantus');
    await waitForEncounterRevision([m, p], enc);
    await p.getByRole('button', { name: 'Digitar o resultado' }).click();
    await p.getByLabel(/Role 1d20 para o teste contra a morte/).fill('5');
    await p.getByRole('button', { name: 'Confirmar 5' }).click();
    await expect(p.getByText(/Falha\. Três falhas\./)).toBeVisible();
    // The player never reads "Morrendo": the counts are all there is.
    await expect(p.getByText('3 de 3').first()).toBeVisible();
    await expect(p.getByText('Morrendo')).toHaveCount(0);
    await p.getByRole('button', { name: 'Encerrar turno' }).click();
    // Off turn, with three failures: the master decides (never "Morrendo").
    await expect(p.getByText('Pensantus está caído, com três falhas: o mestre decide.')).toBeVisible();

    // The master is asked, in place, with the focus on the safe button.
    const row = m.getByRole('listitem').filter({ hasText: 'Pensantus' }).filter({ hasText: 'Morrendo' });
    await expect(row.first()).toContainText('3 falhas');
    const question = m.getByRole('alertdialog', { name: /Confirmar a morte do Pensantus|Confirmar a morte da Pensantus/ });
    await expect(question).toContainText('falhou três vezes no teste contra a morte');
    await expect(question.getByRole('button', { name: 'Ainda não' })).toBeFocused();
    await question.getByRole('button', { name: 'Ainda não' }).click();
    await expect(question).toHaveCount(0);
    await openOrderRow(m, 'Pensantus');
    await row.first().getByRole('button', { name: 'Confirmar a morte' }).click();
    await m.getByRole('alertdialog').getByRole('button', { name: 'Confirmar a morte' }).click();
    await expect(m.getByText('Morto').first()).toBeVisible();
    await expect(await masterLog(m)).toContainText('Pensantus morreu');
    enc = await getEncounterRPC(m, campaignId);
    expect(enc.combatants.find((c) => c.label === 'Pensantus')?.defeated).toBe(true);
  } finally {
    await done();
  }
});

test('o Escudo: o jogador decide num aviso, o cartão do mestre troca ao vivo, e o mestre responde por ele se o jogador não responde', { tag: ['@MR-014', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { m, p, campaignId, done } = await actingTable(browser, 'Escudo do jogador', captainFirst, [], undefined, casting);
  try {
    await openSessionPage(m, campaignId);
    await openSessionPage(p, campaignId);
    const card = m.getByRole('region', { name: /^(Ações do|Vez do) Capitão Goblin$/ });
    const roll = async (face: string) => {
      await card.getByRole('button', { name: 'Digitar o resultado' }).click();
      await card.getByLabel(/Role 1d20 para Cimitarra/).fill(face);
      await card.getByRole('button', { name: `Confirmar ${face}` }).click();
    };

    // 1. A hit that Escudo can stop: the prompt opens by itself on the player's screen, focus on "Não usar".
    await roll('11'); // 11 + 3 = 14 reaches 13, but not 18
    const prompt = p.getByRole('alertdialog', { name: 'Você foi atingido: usar Escudo Arcano?' });
    await expect(prompt).toBeVisible();
    await expect(prompt.getByRole('heading', { name: 'Você foi atingido' })).toBeVisible();
    await expect(prompt.getByRole('button', { name: 'Deixar passar' })).toBeFocused();
    await expect(prompt.getByRole('radio', { name: /1º nível/ })).toBeChecked();
    // The master's card waits for the answer.
    await expect(card.getByText('Espere a reação de Pensantus.')).toBeVisible();
    const no = await layoutSize(prompt.getByRole('button', { name: 'Deixar passar' }));
    const yes = await layoutSize(prompt.getByRole('button', { name: 'Conjurar Escudo Arcano' }));
    expect(yes.height).toBe(no.height);
    expect(yes.width).toBe(no.width);
    await prompt.getByRole('button', { name: 'Conjurar Escudo Arcano' }).click();
    await expect(prompt.getByText('O Escudo Arcano segurou o ataque do Capitão Goblin.')).toBeVisible();
    await expect(prompt.getByText('Espaços de 1º nível: 3 livres de 4')).toBeVisible();
    await prompt.getByRole('button', { name: 'Fechar' }).click();
    // The master's card turned the same hit into a miss, without a reload.
    await expect(card.locator('.pill', { hasText: 'Errou: o Escudo Arcano segurou' })).toBeVisible();
    await expect(card.getByText('Espere a reação de Pensantus.')).toHaveCount(0);
    expect((await vitalsOf(m, campaignId, 'Pensantus')).armorClassBonus).toBe(5);
    await p.getByRole('button', { name: 'Abrir o registro do combate' }).click();
    await expect(p.getByRole('log', { name: 'Registro do combate' })).toContainText('Pensantus conjura Escudo Arcano (1º nível), com a reação');
    await p.keyboard.press('Escape');

    // 2. The reaction is spent for this turn: the next hit goes straight on, no prompt.
    await roll('20'); // a critical hit is not stopped by Escudo and never asks
    await expect(card.getByRole('button', { name: 'Rolar dano' })).toBeVisible();
    await expect(p.getByRole('alertdialog')).toHaveCount(0);
  } finally {
    await done();
  }
});

test('o Escudo: o mestre responde pelo jogador enquanto o aviso está aberto, e o aviso diz isso', { tag: ['@MR-014', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { m, p, campaignId, done } = await actingTable(browser, 'Escudo do mestre', captainFirst, [], undefined, casting);
  try {
    await openSessionPage(m, campaignId);
    await openSessionPage(p, campaignId);
    const card = m.getByRole('region', { name: /^(Ações do|Vez do) Capitão Goblin$/ });
    await card.getByRole('button', { name: 'Digitar o resultado' }).click();
    await card.getByLabel(/Role 1d20 para Cimitarra/).fill('11');
    await card.getByRole('button', { name: 'Confirmar 11' }).click();
    const prompt = p.getByRole('alertdialog', { name: 'Você foi atingido: usar Escudo Arcano?' });
    await expect(prompt).toBeVisible();
    await m.getByRole('region', { name: 'Reações a um ataque' }).getByRole('button', { name: 'Deixar passar pelo jogador' }).click();
    await expect(prompt.getByText('O mestre respondeu por você')).toBeVisible();
    await prompt.getByRole('button', { name: 'Fechar' }).click();
    await expect(card.getByRole('button', { name: 'Rolar dano' })).toBeVisible();
    // The player's own refusal: "Não usar" lets the next hit go on to its damage.
    await card.getByRole('button', { name: 'Rolar dano' }).click();
    await card.getByRole('button', { name: 'Não aplicar' }).click();
    await card.getByRole('button', { name: 'Descartar' }).click();
    await card.getByRole('button', { name: 'Digitar o resultado' }).click();
    await card.getByLabel(/Role 1d20 para Cimitarra/).fill('11');
    await card.getByRole('button', { name: 'Confirmar 11' }).click();
    const again = p.getByRole('alertdialog', { name: 'Você foi atingido: usar Escudo Arcano?' });
    await expect(again).toBeVisible();
    await again.getByRole('button', { name: 'Deixar passar' }).click();
    await expect(again).toHaveCount(0);
    await expect(card.getByRole('button', { name: 'Rolar dano' })).toBeVisible();
    await expect(card.getByText('Espere a reação de Pensantus.')).toHaveCount(0);
  } finally {
    await done();
  }
});

test('condições e concentração: o mestre marca no menu, o jogador vê as etiquetas e encerra a própria concentração', { tag: ['@MR-014', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { m, p, campaignId, done } = await actingTable(browser, 'Condições', playerFirst, [], undefined, casting);
  try {
    await openSessionPage(m, campaignId);
    await openSessionPage(p, campaignId);
    // The master marks two conditions on Goblin 1 from its ⋮ menu.
    await openOrderRow(m, 'Goblin 1');
    await m.getByRole('button', { name: 'Mais ações para Goblin 1' }).click();
    await m.getByRole('menuitem', { name: 'Mudar condições' }).click();
    const dialog = m.getByRole('dialog', { name: 'Condições de Goblin 1' });
    await expect(dialog.getByRole('heading', { name: 'Condições de Goblin 1' })).toBeFocused();
    await expect(dialog.getByRole('checkbox')).toHaveCount(15);
    await expect(dialog.getByText('Derrubado')).toBeVisible(); // prone is "Derrubado"; "Caído" is the 0-hit-point state
    await expect(dialog.getByRole('button', { name: 'Salvar condições' })).toHaveAttribute('aria-disabled', 'true');
    await dialog.getByRole('checkbox', { name: 'Envenenado' }).check();
    await dialog.getByRole('checkbox', { name: 'Derrubado' }).check();
    await dialog.getByRole('button', { name: 'Salvar condições' }).click();
    await expect(dialog).toHaveCount(0);
    // In the master's order, as tags under the name. Envenenado has no outward sign, so it is the master's (and its owner's) alone (RN-10):
    // the player's strip, the map's text list and the log read Derrubado only, with no "+1" and no Envenenado anywhere.
    await expect(m.getByRole('list', { name: 'Condições de Goblin 1' })).toContainText('Envenenado');
    await expect(m.getByRole('list', { name: 'Condições de Goblin 1' })).toContainText('Derrubado');
    await expect(p.getByRole('list', { name: 'Condições de Goblin 1' })).toHaveText(/^\s*Derrubado\s*$/);
    await expect(p.getByText('Goblin 1, coluna 10, linha 10, Derrubado')).toBeAttached();
    await p.getByRole('button', { name: 'Abrir o registro do combate' }).click();
    await expect(p.getByRole('log', { name: 'Registro do combate' })).toContainText('Goblin 1 ficou Derrubado');
    await expect(p.getByText('Envenenado')).toHaveCount(0);
    await p.keyboard.press('Escape');

    // Pensantus casts Teia, which asks for concentration; he sees it and may end it himself.
    await openActionTab(p, 'Magias');
    await p.getByRole('button', { name: 'Conjurar Teia' }).click();
    const sheet = p.getByRole('dialog', { name: 'Conjurar Teia' });
    await sheet.getByRole('button', { name: 'Conjurar Teia' }).click();
    // Nobody is ticked: PM-02b asks in place first, and the spell still spends the slot and the action.
    await expect(sheet.getByText('Ninguém está marcado.')).toBeVisible();
    await sheet.getByRole('button', { name: 'Conjurar mesmo assim' }).click();
    await expect(sheet.getByText('Você está concentrado em Teia.')).toBeVisible();
    await expect(sheet.getByText('Sua ação foi usada.')).toBeVisible();
    await sheet.getByRole('button', { name: 'Voltar à sua vez' }).click();
    await expect(p.getByText('Concentrado em Teia', { exact: true })).toBeVisible();
    await openOrderRow(m, 'Pensantus');
    await expect(m.getByText('Concentração: Teia')).toBeVisible();
    // The master sees it in the dialog too, with the same action.
    await openOrderRow(m, 'Pensantus');
    await m.getByRole('button', { name: 'Mais ações para Pensantus' }).click();
    await m.getByRole('menuitem', { name: 'Mudar condições' }).click();
    await expect(m.getByRole('dialog', { name: 'Condições de Pensantus' }).getByText('Concentrado em')).toBeVisible();
    await m.getByRole('button', { name: 'Cancelar' }).click();
    await p.getByRole('button', { name: 'Encerrar concentração' }).click();
    await expect(p.getByText('Concentrado em Teia', { exact: true })).toHaveCount(0);
    expect((await vitalsOf(m, campaignId, 'Pensantus')).concentrationSpell ?? '').toBe('');
  } finally {
    await done();
  }
});

test('o mestre aplica outro valor de dano, e o lembrete da concentração mostra a CD do teste de Constituição', { tag: ['@MR-012', '@RN-02', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { m, p, campaignId, done } = await actingTable(browser, 'Outro valor', playerFirst, [], undefined, casting);
  try {
    await openSessionPage(m, campaignId);
    await openSessionPage(p, campaignId);
    // Pensantus concentrates on Teia, then the captain hits him with a critical (Escudo never stops it).
    await openActionTab(p, 'Magias');
    await p.getByRole('button', { name: 'Conjurar Teia' }).click();
    await p.getByRole('dialog').getByRole('button', { name: 'Conjurar Teia' }).click();
    await p.getByRole('dialog').getByRole('button', { name: 'Conjurar mesmo assim' }).click(); // nobody ticked: PM-02b asks in place
    await p.getByRole('button', { name: 'Voltar à sua vez' }).click();
    await p.getByRole('button', { name: 'Encerrar turno' }).click();
    const card = m.getByRole('region', { name: /^(Ações do|Vez do) Capitão Goblin$/ });
    await card.getByRole('button', { name: 'Digitar o resultado' }).click();
    await card.getByLabel(/Role 1d20 para Cimitarra/).fill('20');
    await card.getByRole('button', { name: 'Confirmar 20' }).click();
    await card.getByRole('button', { name: 'Rolar dano' }).click();
    await expect(card.getByRole('button', { name: /Aplicar \d+ de dano/ })).toBeVisible();
    // "Aplicar outro valor": a field with the rolled amount, "Aplicar N de dano" and "Voltar" under it.
    await card.getByRole('button', { name: 'Aplicar outro valor' }).click();
    const field = card.getByLabel('Dano a aplicar');
    await expect(field).toBeFocused();
    await expect(card.getByText(/O dado deu \d+\. Por exemplo, metade por resistência\./)).toBeVisible();
    // The field takes four digits at most, so the error shows for what isn't a number.
    await field.fill('abc');
    await expect(card.getByText('Digite um número de 0 a 9.999.')).toBeVisible();
    await field.fill('1');
    await expect(card.getByRole('button', { name: 'Aplicar 1 de dano' })).toBeVisible();
    await card.getByRole('button', { name: 'Voltar' }).click();
    await expect(card.getByRole('button', { name: 'Aplicar outro valor' })).toBeFocused();
    await card.getByRole('button', { name: 'Aplicar outro valor' }).click();
    await card.getByLabel('Dano a aplicar').fill('1');
    await card.getByRole('button', { name: 'Aplicar 1 de dano' }).click();
    await expect(card.getByText(/1 de dano aplicado \(o dado deu \d+\)\. Pensantus: 22 de 23 PV\./).first()).toBeVisible();
    await expect(card.getByText('Pensantus está concentrado em Teia. Teste de Constituição, CD 10.')).toBeVisible();
    expect((await vitalsOf(m, campaignId, 'Pensantus')).hitPointsCurrent).toBe(22);
    // The log keeps both numbers for the master, and the reminder.
    await expect(await masterLog(m)).toContainText('1 de dano (o dado deu');
    await expect(await masterLog(m)).toContainText('Teste de Constituição, CD 10, para manter a concentração');
  } finally {
    await done();
  }
});

test('guerreiro 5: o Ataque Extra deixa dois ataques por ação, Retomar o Fôlego rola e cura, e o Surto de Ação devolve a ação', { tag: ['@MR-014', '@RN-02'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { m, p, table, campaignId, done } = await actingTable(browser, 'Guerreiro', { Toren: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 }, [], undefined, { build: toren, sheet: torenSheet });
  try {
    await adjustVitalsRPC(m, campaignId, table.characterId, { hitPointsCurrent: 20 });
    // Toren stands next to Goblin 1: a sword reaches 1,5 m.
    const start = await getEncounterRPC(m, campaignId);
    await combatRPC(m, 'MoveCombatant', { campaignId, encounterId: start.id, combatantId: start.combatants.find((c) => c.label === 'Toren')!.id, col: 8, row: 9 });
    await openSessionPage(p, campaignId);
    const groups = p.getByRole('region', { name: 'O que você pode fazer' });
    await openActionTab(p, 'Guerreiro');
    await expect(groups.getByText('2 usos').or(groups.getByText('1 uso')).first()).toBeVisible();
    // The first attack spends the action, but not the second one of the Attack action.
    // A natural 1 always misses: the attack is spent and no damage is left to roll.
    const attack = async () => {
      await openActionTab(p, 'Ataques');
      await groups.getByRole('button', { name: /^Atacar com Espada/ }).click();
      const sheet = p.getByRole('dialog', { name: /Atacar com Espada/ });
      await viaTargetList(p, sheet);
      await sheet.locator('label', { hasText: 'Goblin 1' }).click();
      await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
      await sheet.getByLabel(/Role 1d20/).fill('1');
      await sheet.getByRole('button', { name: 'Confirmar 1' }).click();
      return sheet;
    };
    let sheet = await attack();
    await expect(sheet.getByText('Você ainda tem 1 ataque desta ação.')).toBeVisible();
    await sheet.getByRole('button', { name: 'Voltar à sua vez' }).click();
    // The Action stays a filled circle while an attack is left: the bar says how many.
    await expect(p.locator('app-turn-bar').getByLabel('Ação, 1 ataque restante')).toBeVisible();
    await openActionTab(p, 'Ataques');
    await expect(groups.getByRole('button', { name: /^Atacar com Espada/ })).toBeEnabled();
    sheet = await attack();
    await expect(sheet.getByText('Sua ação foi usada.')).toBeVisible();
    await sheet.getByRole('button', { name: 'Voltar à sua vez' }).click();
    // Both attacks made: the Action is spent, and the row is in its folded line with the reason.
    await openActionTab(p, 'Ataques');
    const spentAttacks = await openSpentGroup(p, 'Ação');
    await expect(spentAttacks.getByText('Ataques desta ação já usados').first()).toBeVisible();
    await expect(spentAttacks.getByRole('button', { name: /^Atacar com Espada/ })).toHaveAttribute('aria-disabled', 'true');

    // Surto de Ação (in the Guerreiro tab): another action, with its attacks.
    await openActionTab(p, 'Guerreiro');
    await groups.getByRole('button', { name: 'Usar Surto de Ação' }).click();
    await expect(groups.getByText('Surto de Ação: você tem outra ação.')).toBeVisible();
    await openActionTab(p, 'Ataques');
    await expect(groups.getByRole('button', { name: /^Atacar com Espada/ })).toBeEnabled();

    // Retomar o Fôlego rolls its d10 (typed here) and heals; the use is spent.
    await openActionTab(p, 'Guerreiro');
    await groups.getByRole('button', { name: 'Usar Retomar o Fôlego' }).click();
    const wind = p.getByRole('dialog', { name: 'Retomar o Fôlego' });
    await wind.getByRole('button', { name: 'Digitar o resultado' }).click();
    await wind.getByLabel(/Role 1d10/).fill('7');
    await wind.getByRole('button', { name: 'Confirmar 7' }).click();
    await expect(wind.getByText(/\d+ PV recuperados/)).toBeVisible();
    await expect(wind.getByText('Sua ação bônus foi usada.')).toBeVisible();
    await wind.getByRole('button', { name: 'Voltar à sua vez' }).click();
    await expect.poll(async () => (await vitalsOf(m, campaignId, 'Toren')).hitPointsCurrent).toBeGreaterThan(20);
    // The Bonus action is spent: Retomar o Fôlego is in the folded "Ação bônus · Usada" of the tab.
    await openActionTab(p, 'Guerreiro');
    const spentBonus = await openSpentGroup(p, 'Ação bônus');
    // Second Wind is a bonus action and the bonus action is spent: that is its reason (the button's description).
    const second = spentBonus.getByRole('button', { name: 'Usar Retomar o Fôlego' });
    await expect(second).toHaveAttribute('aria-disabled', 'true');
    await expect(second).toHaveAccessibleDescription(/Ação bônus já usada/);
    // Surto de Ação costs no economy, so it is never in a folded line: its row says it has no uses left.
    const surge = groups.getByRole('button', { name: 'Usar Surto de Ação' });
    await expect(surge).toHaveAttribute('aria-disabled', 'true');
    await expect(surge).toHaveAccessibleDescription(/Sem usos: volta num descanso curto/);
  } finally {
    await done();
  }
});

test('fora da vez: o ataque de oportunidade gasta a reação, só com ataques corpo a corpo', { tag: ['@MR-014', '@RN-21'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { m, p, campaignId, done } = await actingTable(browser, 'Oportunidade', captainFirst, [], undefined, casting);
  try {
    // Pensantus stands next to Goblin 1: a dagger is a melee weapon (thrown or not), so it can make the opportunity attack.
    const start = await getEncounterRPC(m, campaignId);
    await combatRPC(m, 'MoveCombatant', { campaignId, encounterId: start.id, combatantId: start.combatants.find((c) => c.label === 'Pensantus')!.id, col: 8, row: 9 });
    await openSessionPage(m, campaignId);
    await openSessionPage(p, campaignId);
    await expect(p.getByText('Sua reação: Disponível.')).toBeVisible();
    await p.getByRole('button', { name: /^Ataque de oportunidade (com |\()Adaga\)?$/ }).click();
    const sheet = p.getByRole('dialog', { name: 'Ataque de oportunidade com Adaga' });
    // A melee attack: "corpo a corpo", not the dagger's thrown range.
    await expect(sheet.getByText(/^Reação ·.*corpo a corpo$/)).toBeVisible();
    // Only the melee attack: no Raio de Fogo; and Goblin 2 is too far for the dagger.
    await expect(sheet.getByRole('heading', { name: 'Atacar com Adaga' })).toBeVisible();
    await expect(sheet.getByText('Longe demais').first()).toBeVisible(); // the captain and Goblin 2 are out of reach
    await sheet.locator('label', { hasText: 'Goblin 1' }).click();
    await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
    await sheet.getByLabel(/Role 1d20/).fill('1');
    await sheet.getByRole('button', { name: 'Confirmar 1' }).click();
    await expect(sheet.getByText('Sua reação foi usada.')).toBeVisible();
    await sheet.getByRole('button', { name: 'Voltar à sua vez' }).click();
    // The reaction is spent: the line says so, and the action is gone.
    await expect(p.getByText('Sua reação: Usada até o começo do seu próximo turno.')).toBeVisible();
    await expect(p.getByRole('button', { name: /Ataque de oportunidade/ })).toHaveCount(0);
    await expect(await masterLog(m)).toContainText('Pensantus ataca o Goblin 1 com a Adaga (ataque de oportunidade): errou');
  } finally {
    await done();
  }
});

test('a clériga conjura Curar Ferimentos em si mesma: a cura rola e vale na hora', { tag: ['@MR-014', '@RN-02'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { m, p, table, campaignId, done } = await actingTable(browser, 'Cura', { Brisa: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 }, [], undefined, { build: brisa, sheet: brisaSheet });
  try {
    await adjustVitalsRPC(m, campaignId, table.characterId, { hitPointsCurrent: 4 });
    await openSessionPage(p, campaignId);
    await openActionTab(p, 'Magias');
    await p.getByRole('button', { name: 'Conjurar Curar Ferimentos' }).click();
    const sheet = p.getByRole('dialog', { name: 'Conjurar Curar Ferimentos' });
    await viaTargetList(p, sheet);
    // A heal may be aimed at oneself, and only a heal.
    await sheet.locator('label', { hasText: 'Brisa (você)' }).click();
    await sheet.getByRole('button', { name: 'Conjurar Curar Ferimentos' }).click();
    await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
    await sheet.getByLabel(/Role 1d8/).fill('5');
    await sheet.getByRole('button', { name: 'Confirmar 5' }).click();
    await expect(sheet.getByText(/\d+ de cura/).first()).toBeVisible();
    await sheet.getByRole('button', { name: 'Voltar à sua vez' }).click();
    // The heal applied at once, up to the maximum.
    expect((await vitalsOf(m, campaignId, 'Brisa')).hitPointsCurrent).toBeGreaterThan(4);
  } finally {
    await done();
  }
});


test('a vez de quem está caído não passa sem o teste contra a morte, nem para o mestre', { tag: ['@MR-014', '@MR-053', '@RN-03'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { m, p, table, campaignId, done } = await actingTable(browser, 'Passar a vez caído', playerFirst);
  try {
    await adjustVitalsRPC(m, campaignId, table.characterId, { hitPointsCurrent: 0 });
    await openSessionPage(p, campaignId);
    let enc = await getEncounterRPC(m, campaignId);
    // Dropping to 0 on his own turn owes nothing yet: the master's correction raised the revision, so "Caído" is already there.
    await expect(p.getByRole('heading', { name: 'Pensantus está caído' })).toBeVisible();
    enc = await combatRPC(m, 'EndTurn', { campaignId, encounterId: enc.id, expectedCombatantId: enc.currentCombatantId });
    enc = await passTurnsTo(m, campaignId, 'Pensantus');
    expect(enc.combatants.find((c) => c.label === 'Pensantus')?.deathSaveDue).toBe(true);
    // Nobody can pass it before the save (SRD 5.1): the player has no button, and the server refuses both of them.
    await expect(p.getByRole('button', { name: 'Encerrar turno' })).toHaveCount(0);
    const turn = { campaignId, encounterId: enc.id, expectedCombatantId: enc.currentCombatantId };
    const refusedPlayer = await callRPC(p, 'meurpg.play.v1.CombatService/EndTurn', { ...turn, idempotencyKey: crypto.randomUUID() });
    expect(refusedPlayer.ok()).toBe(false);
    const refusedMaster = await callRPC(m, 'meurpg.play.v1.CombatService/EndTurn', { ...turn, idempotencyKey: crypto.randomUUID() });
    expect(refusedMaster.ok()).toBe(false);
    // The save made (the master rolls it for the character), the turn passes.
    await combatRPC(m, 'RollDeathSave', { campaignId, encounterId: enc.id, combatantId: enc.currentCombatantId, d20Face: 14 });
    enc = await combatRPC(m, 'EndTurn', turn);
    expect(enc.combatants.find((c) => c.id === enc.currentCombatantId)?.label).not.toBe('Pensantus');
  } finally {
    await done();
  }
});

// Squares everywhere, spells in the session and the spells that read hit points
// (Etapa 8, slice 8.4, MR-013, MR-014, RN-20): the table counts in squares of
// 1,5 m, every spell has its "?" and the server's order, and Sono tells the master
// the whole account and a player only who fell asleep. Pensantus carries Mísseis
// Mágicos, Sono, Escudo Arcano, Teia and Passo Nebuloso (`pensantusAttacks`), a
// gnome with 25 ft of speed (7,5 m, 5 squares); the Goblins have 7 hit points.

test('a distância sai em metros e em quadrados: a linha do movimento, a barra do turno, o cartão do mestre e a ficha', { tag: ['@MR-013'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const { m, p, table, campaignId, done } = await actingTable(browser, 'Quadrados', playerFirst);
  try {
    await openSessionPage(p, campaignId);
    await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
    // The movement row under the map: the meters and the squares, beside "Mover".
    await expect(p.getByRole('status').filter({ hasText: 'Restam 7,5 m' })).toContainText('Restam 7,5 m · 5 quadrados');
    // The sticky turn bar: the walker with "7,5 m" among what is left, inside the bar.
    const bar = p.locator('app-turn-bar');
    await expect(bar.getByText('7,5 m')).toBeVisible();
    expect(await bar.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);

    // The master's card for the captain (a gnome too, in chain mail without the Strength 13 it asks for, so 10 ft
    // slower: 15 ft): the Deslocamento in both units, and the Movimento chip.
    await openSessionPage(m, campaignId);
    await passTurnsTo(m, campaignId, 'Capitão Goblin');
    const card = m.getByRole('region', { name: /^(Ações do|Vez do) Capitão Goblin$/ });
    await expect(card.locator('.stat', { hasText: 'Deslocamento' })).toContainText('4,5 m · 3 quadrados');
    await expect(card.locator('.chip', { hasText: 'Movimento' })).toContainText('4,5 m · 3 quadrados');

    // The sheet: one line, in a box of its own, with the feet in parentheses.
    await p.goto(`/campaigns/${campaignId}/characters/${table.characterId}`);
    await expect(p.getByText('7,5 m · 5 quadrados (25 pés)')).toBeVisible();
  } finally {
    await done();
  }
});

test('as magias vêm na ordem do servidor, cada uma com o "?", e o Escudo na sua vez diz "Só fora da sua vez"', { tag: ['@MR-014'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const { m, p, table, campaignId, done } = await actingTable(browser, 'Magias com o ponto de interrogação', playerFirst);
  try {
    await openSessionPage(p, campaignId);
    const groups = p.getByRole('region', { name: 'O que você pode fazer' });
    // What can be cast now first, then by circle and by name; Escudo Arcano, a reaction, with the ones that can't.
    // The cantrip that attacks is in "Ataques", with its "?"; the spells are in "Magias" (their description is the cast sheet's "?", below).
    const attacks = await openActionTab(p, 'Ataques');
    await expect(attacks.locator('app-action-row:has(app-spell-help) .row__name')).toHaveText(['Raio de Fogo']);
    const spells = await openActionTab(p, 'Magias');
    const names = spells.locator('app-action-row .row__name');
    await expect(names).toHaveText(['Mísseis Mágicos', 'Sono', 'Passo Nebuloso', 'Teia', 'Escudo Arcano']);
    const shield = groups.locator('app-action-row', { hasText: 'Escudo Arcano' });
    await expect(shield.getByText('Só fora da sua vez')).toBeVisible();
    await expect(shield.getByRole('button', { name: 'Conjurar Escudo Arcano' })).toHaveAttribute('aria-disabled', 'true');
    await expect(shield.getByText('Reação', { exact: true })).toBeVisible();

    // A spell row has no "?": the description opens from the cast sheet.
    await expect(spells.locator('app-spell-help')).toHaveCount(0);

    // With no free slot the 1st and 2nd circles say "Sem espaço", and the slot rows above are the explanation.
    await adjustVitalsRPC(m, campaignId, table.characterId, { spellSlotsUsed: [{ level: 1, used: 4 }, { level: 2, used: 2 }] });
    await openSessionPage(p, campaignId);
    await openActionTab(p, 'Magias');
    // The slots line of the Magias tab (from 1024px) or the turn card's circles (a phone): both name each circle.
    const slot = (words: RegExp) => p.getByRole('listitem', { name: words }).or(p.getByRole('img', { name: words })).first();
    await expect(slot(/0 livres de 4/)).toBeVisible();
    await expect(slot(/0 livres de 2/)).toBeVisible();
    await expect(groups.locator('.row__why', { hasText: /^\s*block\s*Sem espaço\s*$/ })).toHaveCount(4);
    await expect(groups.getByText(/Sem espaço de/)).toHaveCount(0);
    // Nothing can be cast now: the order is by circle, then by name, and Escudo Arcano is among them.
    await openActionTab(p, 'Magias');
    await expect(names).toHaveText(['Escudo Arcano', 'Mísseis Mágicos', 'Sono', 'Passo Nebuloso', 'Teia']);
  } finally {
    await done();
  }
});

// Three of them around Goblin 1, out of Pensantus's reach: Sono centered there catches the three.
const aroundGoblin1 = { 'Capitão Goblin': [9, 10], 'Goblin 1': [9, 9], 'Goblin 2': [10, 9] } as Record<string, [number, number]>;

test('Sono em dois goblins e no Capitão: o mestre vê o total e os PV, o jogador só vê quem adormeceu', { tag: ['@MR-014', '@RN-20'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { m, p, campaignId, done } = await actingTable(browser, 'Sono', playerFirst, [], undefined, {}, aroundGoblin1);
  try {
    await openSessionPage(m, campaignId);
    await openSessionPage(p, campaignId);
    await openActionTab(p, 'Magias');
    await p.getByRole('button', { name: 'Conjurar Sono' }).click();
    const sheet = p.getByRole('dialog', { name: 'Conjurar Sono' });
    // On a map the sphere is placed: the focus starts on the map, and "Centrar em…" places it on a creature he sees.
    await expect(sheet.getByRole('application', { name: 'Mapa: escolha o ponto da Sono' })).toBeFocused();
    await expect(sheet.getByText('Ação · alcance 27 m · esfera de 6 m de raio · 5d8 de pontos de vida')).toBeVisible();
    await sheet.getByRole('button', { name: 'Centrar em…' }).click();
    await sheet.getByRole('listbox', { name: 'Centrar em…' }).getByRole('option', { name: /Goblin 1/ }).click();
    await sheet.getByRole('button', { name: 'Confirmar local' }).click();
    // Who the server says is inside, by name and state word: no hit points on the list.
    await expect(sheet.getByRole('heading', { name: 'Quem está na área' })).toBeFocused();
    await expect(sheet.locator('app-area-list li')).toHaveCount(3);
    await expect(sheet.getByText(/\bPV\b/)).toHaveCount(0);

    // The "?" in the header opens the description over the sheet; "Fechar" comes back to the choices.
    await sheet.getByRole('button', { name: 'Detalhes de Sono' }).click();
    const details = p.getByRole('dialog', { name: 'Descrição de Sono' });
    await expect(details.getByText('Esta magia coloca criaturas em um sono mágico.')).toBeVisible();
    await details.getByRole('button', { name: 'Fechar' }).last().click();
    await expect(details).toHaveCount(0);
    await expect(sheet.locator('app-area-list li')).toHaveCount(3);

    // The pool is typed from the physical dice: 20 reaches both goblins (7 + 7), not the captain.
    await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
    await sheet.getByLabel(/Role 5d8/).fill('41');
    await expect(sheet.getByText('Digite um número de 5 a 40')).toBeVisible();
    await sheet.getByLabel(/Role 5d8/).fill('20');
    await sheet.getByRole('button', { name: 'Confirmar 20' }).click();

    // The player: who fell asleep, who was not affected, the caster's own roll, and no number of an enemy's.
    await expect(sheet.getByRole('heading', { name: 'Sono conjurado' })).toBeVisible();
    await expect(sheet.getByText('O Capitão Goblin não foi afetado. O Goblin 1 adormeceu. O Goblin 2 adormeceu.')).toBeVisible();
    await expect(sheet.getByText('Sua rolagem')).toBeVisible();
    await expect(sheet.getByText('5d8 = 20 · dado físico')).toBeVisible();
    await expect(sheet.getByText(/\bPV\b|restam|restantes/)).toHaveCount(0);
    await sheet.getByRole('button', { name: 'Voltar à sua vez' }).click();
    await p.getByRole('button', { name: 'Abrir o registro do combate' }).click();
    const playerLog = p.getByRole('log', { name: 'Registro do combate' });
    await expect(playerLog).toContainText('Pensantus conjura Sono: o Capitão Goblin não foi afetado. O Goblin 1 adormece. O Goblin 2 adormece.');
    await expect(playerLog).not.toContainText('PV');

    // The master: the pool, each creature from the lowest hit points up with the total that is left.
    const log = await masterLog(m);
    await expect(log).toContainText('Pensantus conjura Sono (1º nível)');
    const card = m.locator('app-pool-card');
    await expect(card).toContainText('5d8 = 20');
    await expect(card.locator('.row', { hasText: 'Goblin 1' })).toContainText('7 PV');
    await expect(card.locator('.row', { hasText: 'Goblin 1' })).toContainText('20 − 7 = 13 restam');
    await expect(card.locator('.row', { hasText: 'Goblin 1' })).toContainText('Adormeceu · Inconsciente');
    await expect(card.locator('.row', { hasText: 'Goblin 2' })).toContainText('13 − 7 = 6 restam');
    await expect(card.locator('.row', { hasText: 'Capitão Goblin' })).toContainText('é mais que 6 restantes');
    await expect(card.locator('.row', { hasText: 'Capitão Goblin' })).toContainText('Não afetado');
    await card.getByRole('button', { name: 'Mudar as condições do Goblin 1' }).click();
    await expect(m.getByRole('dialog', { name: 'Condições de Goblin 1' })).toBeVisible();
    // The sleepers carry the condition on both screens; the hit points did not change.
    const enc = await getEncounterRPC(m, campaignId);
    expect(enc.combatants.find((c) => c.label === 'Goblin 1')?.conditions).toContain('condition:unconscious');
    expect(enc.combatants.find((c) => c.label === 'Capitão Goblin')?.conditions ?? []).not.toContain('condition:unconscious');
  } finally {
    await done();
  }
});

const mira: CharacterBuild = {
  name: 'Mira',
  raceKey: 'race:human',
  race: 'Humano',
  classKey: 'class:monk',
  class: 'Monge',
  level: 3,
  background: 'Soldado',
  backgroundSkillKeys: ['skill:athletics', 'skill:intimidation'],
  backgroundSkills: ['Atletismo', 'Intimidação'],
  extraSkillKeys: ['skill:acrobatics', 'skill:stealth'],
  extraSkills: ['Acrobacia', 'Furtividade'],
  scores: { for: 12, des: 17, con: 14, int: 10, sab: 15, car: 8 },
};

test('monge 3: o golpe das Artes Marciais e a Rajada de Golpes aparecem como ataques de ação bônus, e a Rajada espera o ataque da ação', { tag: ['@MR-014'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { m, p, campaignId, done } = await actingTable(browser, 'Monge', { Mira: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 }, [], undefined, { build: mira });
  try {
    const start = await getEncounterRPC(m, campaignId);
    await combatRPC(m, 'MoveCombatant', { campaignId, encounterId: start.id, combatantId: start.combatants.find((c) => c.label === 'Mira')!.id, col: 8, row: 9 });
    await openSessionPage(p, campaignId);
    const groups = p.getByRole('region', { name: 'O que você pode fazer' });
    // A natural 1 always misses: the attack is spent and no damage is left to roll.
    const strike = async () => {
      await openActionTab(p, 'Ataques');
      await groups.getByRole('button', { name: /^Atacar com Golpe desarmado/ }).click();
      const sheet = p.getByRole('dialog', { name: /Atacar com Golpe desarmado/ });
      await viaTargetList(p, sheet);
      await sheet.locator('label', { hasText: 'Goblin 1' }).click();
      // Each attack sheet opens fresh, with the app's dice offered first (as the Raio de Fogo test above).
      await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
      await sheet.getByLabel(/Role 1d20/).fill('1');
      await sheet.getByRole('button', { name: 'Confirmar 1' }).click();
      return sheet;
    };

    // Before the Attack action, Rajada de Golpes is off and says why.
    await openActionTab(p, 'Chi');
    await expect(groups.getByText('Só depois de atacar com a ação')).toBeVisible();
    await expect(groups.getByRole('button', { name: 'Usar Rajada de Golpes' })).toHaveAttribute('aria-disabled', 'true');

    let sheet = await strike();
    await expect(sheet.getByText('Sua ação foi usada.')).toBeVisible();
    await sheet.getByRole('button', { name: 'Voltar à sua vez' }).click();
    // The action is spent, yet the strike stays on offer as a bonus action attack.
    await openActionTab(p, 'Ataques');
    await expect(groups.getByText('Golpe desarmado das Artes Marciais')).toBeVisible();
    await expect(groups.getByRole('button', { name: /^Atacar com Golpe desarmado/ })).toBeEnabled();

    await openActionTab(p, 'Chi');
    await groups.getByRole('button', { name: 'Usar Rajada de Golpes' }).click();
    // The counter is the note of the bonus action attack, in the attacks' tab.
    await openActionTab(p, 'Ataques');
    await expect(groups.getByText('Rajada de Golpes: 2 golpes restantes')).toBeVisible();
    sheet = await strike();
    await expect(sheet.getByText('Rajada de Golpes: 1 golpe restante.')).toBeVisible();
    await sheet.getByRole('button', { name: 'Voltar à sua vez' }).click();
    await expect(groups.getByText('Rajada de Golpes: 1 golpe restante')).toBeVisible();
    sheet = await strike();
    await expect(sheet.getByText('Rajada de Golpes: acabaram os golpes.')).toBeVisible();
  } finally {
    await done();
  }
});

// Pensantus at level 5, with Bola de Fogo (a 3rd-level slot).
const fireballCaster = {
  build: { ...pensantus, level: 5 },
  sheet: {
    ...pensantusCasting,
    knownSpellKeys: [...pensantusCasting.knownSpellKeys, 'spell:fireball'],
    preparedSpellKeys: [...pensantusCasting.preparedSpellKeys, 'spell:fireball'],
  },
};
const aroundGoblin1Far = { 'Capitão Goblin': [12, 3], 'Goblin 1': [12, 7], 'Goblin 2': [13, 8] } as Record<string, [number, number]>;

test('a Bola de Fogo é posta num ponto do mapa: o ponto, quem está na área, a conjuração e o dano rolado uma vez', { tag: ['@MR-014', '@RN-10'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { m, p, campaignId, done } = await actingTable(browser, 'Bola de Fogo', playerFirst, [], undefined, fireballCaster, aroundGoblin1Far);
  try {
    await openSessionPage(p, campaignId);
    await openActionTab(p, 'Magias');
    await p.getByRole('button', { name: 'Conjurar Bola de Fogo' }).click();
    const sheet = p.getByRole('dialog', { name: 'Conjurar Bola de Fogo' });
    await expect(sheet.getByText('Onde ela explode')).toBeVisible();
    const map = sheet.getByRole('application', { name: 'Mapa: escolha o ponto da Bola de Fogo' });
    await expect(map).toBeFocused();
    await expect(sheet.getByText('Toque no mapa para escolher o ponto.')).toBeVisible();
    // The keyboard: C opens "Centrar em…"; a creature places the point on its square, Enter on the map confirms.
    await map.press('c');
    await sheet.getByRole('listbox', { name: 'Centrar em…' }).getByRole('option', { name: /Goblin 1/ }).click();
    await expect(sheet.getByText(/Ponto a\s+10,5\s+m de você/)).toBeVisible();
    await map.press('Enter');
    await expect(sheet.getByRole('heading', { name: 'Quem está na área' })).toBeFocused();
    await expect(sheet.getByText('3 criaturas', { exact: true })).toBeVisible();
    await expect(sheet.getByText(/no teste de Destreza|Sem cobertura/).first()).toBeVisible();
    // "Mudar o local" goes back with the point; nothing was spent.
    await sheet.getByRole('button', { name: 'Mudar o local' }).click();
    await expect(sheet.getByRole('button', { name: 'Confirmar local' })).not.toHaveAttribute('aria-disabled', 'true');
    await sheet.getByRole('button', { name: 'Confirmar local' }).click();
    await sheet.getByRole('button', { name: 'Conjurar Bola de Fogo' }).click();
    await expect(sheet.getByRole('heading', { name: 'Bola de Fogo conjurada' })).toBeVisible();
    await expect(sheet.getByText(/3º\s+nível · 3 criaturas · teste de resistência de Destreza/)).toBeVisible();
    await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
    await sheet.getByLabel(/Role 8d6/).fill('28');
    await sheet.getByRole('button', { name: 'Confirmar 28' }).click();
    await sheet.getByRole('button', { name: 'Voltar à sua vez' }).click();
    // One roll for all three: each took 28 or 14, and the goblins fell.
    const goblins = (await getEncounterRPC(m, campaignId)).combatants.filter((c) => c.label.startsWith('Goblin'));
    expect(goblins.every((g) => g.defeated)).toBe(true);
  } finally {
    await done();
  }
});

test('"Perguntar a cada vez": a Bola de Fogo segura o turno, o mestre vê a área e responde, e o jogador só lê "Esperando o mestre"', { tag: ['@MR-014', '@RN-10', '@RN-24'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  const { m, p, campaignId, done } = await actingTable(browser, 'Perguntar', playerFirst, ['Goblin 2'], undefined, fireballCaster, aroundGoblin1Far);
  try {
    await setTableRulesRPC(m, campaignId, { hiddenAreaHits: 'HIDDEN_AREA_HIT_RULE_ASK' });
    await openSessionPage(p, campaignId);
    await openSessionPage(m, campaignId);
    await openActionTab(p, 'Magias');
    await p.getByRole('button', { name: 'Conjurar Bola de Fogo' }).click();
    const sheet = p.getByRole('dialog', { name: 'Conjurar Bola de Fogo' });
    const map = sheet.getByRole('application', { name: 'Mapa: escolha o ponto da Bola de Fogo' });
    await map.press('c');
    await sheet.getByRole('listbox', { name: 'Centrar em…' }).getByRole('option', { name: /Goblin 1/ }).click();
    await map.press('Enter');
    // The hidden Goblin 2 is in the area, and the player is told neither its name nor a count that includes it.
    await expect(sheet.getByRole('heading', { name: 'Quem está na área' })).toBeFocused();
    await expect(sheet.getByText(/Goblin 2/)).toHaveCount(0);
    await sheet.getByRole('button', { name: 'Conjurar Bola de Fogo' }).click();
    await expect(sheet.getByRole('heading', { name: 'Bola de Fogo conjurada' })).toBeVisible();
    await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
    await sheet.getByLabel(/Role 8d6/).fill('28');
    await sheet.getByRole('button', { name: 'Confirmar 28' }).click();
    await sheet.getByRole('button', { name: 'Voltar à sua vez' }).click();

    // The player: the turn waits for the master, and nothing says why.
    await expect(p.getByText('Esperando o mestre').first()).toBeVisible();
    await expect(p.getByText(/escondid|Goblin 2/i)).toHaveCount(0);

    // The master: the question, the area of the last spell on his map and the wait in the bar.
    const card = m.getByRole('group', { name: /Bola de Fogo atingiu 1 criatura escondida/ });
    await expect(card).toBeVisible();
    await expect(card.getByText('Goblin 2')).toBeVisible();
    await expect(m.locator('app-area-overlay')).toBeVisible();
    await expect(m.getByText('Área da última magia')).toBeVisible();
    await expect(m.getByText('Esperando a sua resposta: escondidas atingidas')).toBeVisible();

    // After a reload the question comes back from the combat, and a line says the screen was rebuilt.
    await m.reload();
    await expect(m.getByRole('group', { name: /Bola de Fogo atingiu 1 criatura escondida/ })).toBeVisible();
    await expect(m.getByRole('status').filter({ hasText: 'Combate atualizado agora.' })).toBeVisible();
    await expect(m.locator('app-area-overlay')).toBeVisible();

    await m.getByRole('button', { name: 'Revelar', exact: true }).click();
    await expect(m.getByRole('group', { name: /Bola de Fogo atingiu/ })).toHaveCount(0);
    await expect(p.getByText('Esperando o mestre')).toHaveCount(0);
  } finally {
    await done();
  }
});

test('Ajuda sobre quem está a 0 PV: acorda, a ficha mostra o efeito, e o mestre encerra pelo menu da linha com a pergunta', { tag: ['@MR-014', '@RN-02'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  const t = await trapTable(browser, 'Ajuda');
  const { m, p, table, campaignId } = t;
  const third = await thirdPlayer(browser, t, brisa, brisaAidSheet);
  try {
    const square = sq20(4, 7);
    await placeTokenRPC(m, campaignId, table.mapId, third.characterId, square.xBp, square.yBp);
    await beginAttackCombatRPC(m, table, { Brisa: 20, Pensantus: 15, 'Capitão Goblin': 10, 'Goblin 1': 5, 'Goblin 2': 4 });
    await adjustVitalsRPC(m, campaignId, table.characterId, { hitPointsCurrent: 0 });
    await openSessionPage(p, campaignId);
    await openSessionPage(m, campaignId);

    // Before: at 0, the two pills of the board, with the counts of the combatant's death saves.
    const pills = p.getByRole('status');
    await expect(pills.getByText('Inconsciente', { exact: true })).toBeVisible();
    await expect(pills.getByText('Testes contra a morte: 0 sucessos, 0 falhas')).toBeVisible();

    // Brisa (the third player) casts Ajuda with a 2nd-level slot on Pensantus.
    const enc = await getEncounterRPC(m, campaignId);
    const id = (label: string) => enc.combatants.find((c) => c.label === label)!.id;
    const cast = await callRPC(third.page, 'meurpg.play.v1.CombatService/CastSpell', {
      campaignId,
      encounterId: enc.id,
      casterId: id('Brisa'),
      spellKey: 'spell:aid',
      slot: { level: 2 },
      targets: [{ combatantId: id('Pensantus') }],
      idempotencyKey: crypto.randomUUID(),
    });
    expect(cast.ok(), await cast.text()).toBeTruthy();

    // After: awake with 5 of 28, the sheet's maximum small, the tag, the amber banner and the green pill.
    await expect(p.getByText('+5 de Ajuda', { exact: true })).toBeVisible();
    await expect(p.getByText('máximo 23 da ficha')).toBeVisible();
    await expect(pills.getByText('Ajuda: +5 nos PV até o mestre encerrar ou um descanso longo')).toBeVisible();
    await expect(pills.getByText('Acordado · testes contra a morte zerados')).toBeVisible();
    await expect(pills.getByText('Inconsciente', { exact: true })).toHaveCount(0);

    // The master's row says it on one line, even at 1280 px.
    const order = m.getByRole('region', { name: 'Ordem de iniciativa' });
    await openOrderRow(m, 'Pensantus');
    const chip = order.locator('.row__effect--aid');
    await expect(chip).toContainText('Ajuda +5 PV');
    expect((await layoutSize(chip)).height).toBeLessThan(30);

    // The sheet page, outside the session's screen, draws the same numbers while the session is live.
    await p.goto(`/campaigns/${campaignId}/characters/${table.characterId}`);
    const stats = p.locator('app-combat-stats');
    await expect(stats.getByText('5 de 28')).toBeVisible({ timeout: 30_000 });
    await expect(stats.getByText('máximo 23 da ficha')).toBeVisible();
    await expect(stats.getByText('+5 de Ajuda', { exact: true })).toBeVisible();
    await expect(stats.getByText('Ajuda: +5 nos PV até o mestre encerrar ou um descanso longo')).toBeVisible();

    // The master ends it from the row's menu: each item has its icon, the question says the account, and the confirm ends it.
    await openOrderRow(m, 'Pensantus');
    await order.getByRole('button', { name: 'Mais ações para Pensantus' }).click();
    await expect(m.getByRole('menuitem', { name: 'Encerrar Ajuda em Pensantus' })).toBeVisible();
    await expect(m.getByRole('menuitem', { name: 'Mudar condições' })).toBeVisible();
    await m.getByRole('menuitem', { name: 'Encerrar Ajuda em Pensantus' }).click();
    const ask = m.getByRole('alertdialog', { name: /Encerrar a Ajuda d[oa] Pensantus/ });
    await expect(ask).toContainText('O máximo de PV volta a 23. Os PV atuais ficam em 5');
    await expect(ask.getByRole('button', { name: 'Cancelar' })).toBeFocused();
    await ask.getByRole('button', { name: 'Encerrar Ajuda' }).click();
    await expect(ask).toBeHidden();
    await expect(order.locator('.row__effect--aid')).toHaveCount(0);

    // The player's sheet goes back to the old words.
    await expect(stats.getByText('Os atuais aparecem na sessão')).toBeVisible();
    expect((await vitalsOf(m, campaignId, 'Pensantus')).hitPointsCurrent).toBe(5);
  } finally {
    await third.close();
    await t.done();
  }
});

test('Crítico Brutal com dados no app: a conta antes de rolar, a linha do resultado e o registro dizem o nome e os dados de cada grupo', { tag: ['@MR-012', '@MR-014'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { p, campaignId, done } = await actingTable(browser, 'Brutal app', { Ragna: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 }, [], undefined, { build: ragna, sheet: ragnaSheet }, { 'Capitão Goblin': [11, 5], 'Goblin 1': [6, 7], 'Goblin 2': [14, 10] });
  try {
    await openSessionPage(p, campaignId);
    await p.getByRole('button', { name: 'Atacar com Machado grande' }).click();
    const sheet = p.getByRole('dialog', { name: 'Atacar com Machado grande' });
    await viaTargetList(p, sheet);
    await sheet.locator('label', { hasText: 'Goblin 1' }).click();
    await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
    await sheet.getByLabel(/Role 1d20 para Machado grande/).fill('20');
    await sheet.getByRole('button', { name: 'Confirmar 20' }).click();
    await expect(sheet.locator('.pill', { hasText: 'Crítico' })).toBeVisible();

    // Before rolling: the whole sum and which part is the critical's and which the feature's.
    await expect(sheet.getByText('Dano do crítico')).toBeVisible();
    await expect(sheet.getByText('2d12 + 1d12 + 3', { exact: true })).toBeVisible();
    await expect(sheet.getByText('2d12 do crítico (dados dobrados) e 1d12 do Crítico Brutal (nível 9), mais 3 de modificador, de cortante.')).toBeVisible();
    await sheet.getByRole('button', { name: 'Rolar dano no app' }).click();
    await expect(sheet.getByText(/2d12 \(\d+, \d+\) \+ 1d12 Crítico Brutal \(\d+\) \+ 3 = \d+ de dano cortante/)).toBeVisible();
    await sheet.getByRole('button', { name: 'Voltar à sua vez' }).click();

    await p.getByRole('button', { name: 'Abrir o registro do combate' }).click();
    await expect(p.getByRole('log', { name: 'Registro do combate' })).toContainText(/dano 2d12 \(\d+, \d+\) \+ 1d12 Crítico Brutal \(\d+\) \+ 3 = \d+ de cortante/);
  } finally {
    await done();
  }
});

test('Crítico Brutal com dados físicos: o jogador rola três dados, o botão lê o total e o registro diz os grupos', { tag: ['@MR-012', '@MR-014', '@RN-18'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { p, campaignId, done } = await actingTable(browser, 'Brutal físico', { Ragna: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 }, [], undefined, { build: ragna, sheet: ragnaSheet }, { 'Capitão Goblin': [11, 5], 'Goblin 1': [6, 7], 'Goblin 2': [14, 10] });
  try {
    await openSessionPage(p, campaignId);
    await p.getByRole('button', { name: 'Atacar com Machado grande' }).click();
    const sheet = p.getByRole('dialog', { name: 'Atacar com Machado grande' });
    await viaTargetList(p, sheet);
    await sheet.locator('label', { hasText: 'Goblin 1' }).click();
    await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
    await sheet.getByLabel(/Role 1d20 para Machado grande/).fill('20');
    await sheet.getByRole('button', { name: 'Confirmar 20' }).click();
    await expect(sheet.locator('.pill', { hasText: 'Crítico' })).toBeVisible();

    await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
    await expect(sheet.getByText('Role 3d12 para o Machado grande (+3)')).toBeVisible();
    await expect(sheet.getByText('2d12 do crítico e 1d12 do Crítico Brutal. Role os três dados e digite a soma (3 a 36).')).toBeVisible();
    await sheet.getByLabel(/Role 3d12/).fill('22');
    await expect(sheet.getByText('22 (3d12) + 3 = 25')).toBeVisible();
    // The button reads the total, not the sum of the dice typed.
    await sheet.getByRole('button', { name: 'Confirmar 25' }).click();
    await sheet.getByRole('button', { name: 'Voltar à sua vez' }).click();

    await p.getByRole('button', { name: 'Abrir o registro do combate' }).click();
    await expect(p.getByRole('log', { name: 'Registro do combate' })).toContainText('dano 3d12 (2d12 + 1d12 Crítico Brutal) = 22 + 3 = 25 de cortante, dados físicos');
  } finally {
    await done();
  }
});
