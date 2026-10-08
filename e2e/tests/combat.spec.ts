import { expect, test, type Browser, type BrowserContext, type Page } from '@playwright/test';

import {
  adjustVitalsRPC,
  beginAttackCombatRPC,
  brisa,
  brisaSheet,
  combatRPC,
  endTurnOf,
  getEncounterRPC,
  passTurnsTo,
  pensantusCasting,
  setGridRPC,
  tableForCombat,
  toren,
  torenSheet,
  type CombatTable,
  type Encounter,
} from './combat-support';
import type { CharacterBuild } from './support';
import { endOpenSessionRPC, openSessionPage } from './live-session-support';
import { boxOf, callRPC, layoutSize, newSignedInContext } from './support';

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
      await m.getByRole('button', { name: 'Iniciar combate' }).click();
      await expect(m.getByText('Esse mapa ainda não tem grade.')).toBeVisible();
      // A map without a grid is no map for "Com mapa" (RN-25): that choice waits with its reason, and "Sem mapa" is the one chosen.
      await expect(m.getByRole('radio', { name: /^Com mapa/ })).toBeDisabled();
      await expect(m.getByRole('radio', { name: /Sem mapa \(teatro da mente\)/ })).toBeChecked();
      await m.getByRole('link', { name: 'Definir a grade' }).click();
      await expect(m.getByRole('heading', { name: 'Grade do mapa' })).toBeVisible();
      // Columns 4 is below the screen's 5 to 60; 30 gives 21 rows for 2000 x 1400.
      await m.getByLabel('Quadrados de 1,5 m na largura').fill('4');
      await expect(m.getByText('Use um número inteiro de 5 a 60.')).toBeVisible();
      await expect(m.getByRole('button', { name: 'Salvar grade' })).toHaveAttribute('aria-disabled', 'true');
      await m.getByLabel('Quadrados de 1,5 m na largura').fill('20');
      await expect(m.getByText('20 × 14')).toBeVisible();
      await expect(m.getByText('30 m × 21 m')).toBeVisible();
      await m.getByRole('button', { name: 'Salvar grade' }).click();
      await expect(m).toHaveURL(/\/session$/);

      // Start with the party and three hidden goblins (and the captain).
      await m.getByRole('button', { name: 'Iniciar combate' }).click();
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
        await combatRPC(m, 'SetInitiativeOrder', { campaignId, encounterId: enc.id, combatantIds: [cap.id, pen.id] });
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
      await expect(m.getByText('Vez do Capitão Goblin')).toBeVisible();
      await expect(m.getByRole('region', { name: 'Combate', exact: true }).getByText('Rodada 1', { exact: true })).toBeVisible();

      // The captain is hidden: the player sees "Vez do mestre", never a goblin.
      await expect(p.getByRole('heading', { name: 'Vez do mestre' })).toBeVisible();
      await expect(p.getByText('Goblin')).toHaveCount(0);
      await expect(p.getByRole('listitem').filter({ hasText: 'Pensantus' }).first()).toBeVisible();

      // "Próximo turno": Pensantus, and the player is told.
      await m.getByRole('button', { name: 'Próximo turno' }).click();
      await expect(m.getByText('Vez do Pensantus')).toBeVisible();
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
      await expect(p.getByRole('heading', { name: 'Combate encerrado' })).toBeVisible();
      await m.getByRole('button', { name: 'Voltar à sessão' }).click();
      await expect(m.getByRole('button', { name: 'Iniciar combate' })).toBeVisible();
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
): Promise<ActingTable> {
  const master: BrowserContext = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
  const player: BrowserContext = await newSignedInContext(browser, 'Jogador Teste', { viewport: phone });
  const m = await master.newPage();
  const p = await player.newPage();
  await m.goto('/');
  await p.goto('/');
  const table = await tableForCombat(m, p, `${name} ${Date.now()}`, true, true, character);
  await beginAttackCombatRPC(m, table, faces, undefined, hidden);
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
    // The groups: the cantrip with its pill, the spells, the standard actions (not Atacar or Conjurar), the movement.
    const groups = p.getByRole('region', { name: 'O que você pode fazer' });
    await expect(groups.getByRole('heading', { name: 'Ação', exact: true })).toBeVisible();
    await expect(groups.getByText('+6 para acertar · 1d10 de fogo · alcance 36 m')).toBeVisible();
    await expect(groups.getByRole('button', { name: 'Disparada' })).toBeVisible();
    await expect(groups.getByRole('button', { name: 'Conjurar uma magia' })).toHaveCount(0);
    await expect(groups.getByRole('heading', { name: 'Movimento' })).toBeVisible();
    // No armor class on the player's screen.
    await expect(p.getByText(/contra CA/)).toHaveCount(0);

    await p.getByRole('button', { name: 'Atacar com Raio de Fogo' }).click();
    const sheet = p.getByRole('dialog', { name: 'Atacar com Raio de Fogo' });
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
    await expect(p.getByText('Ação já usada').first()).toBeVisible();
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
    await p.getByRole('button', { name: 'Encerrar turno' }).last().click();
    await expect(p.getByRole('alertdialog', { name: /Ainda tem ação disponível/ })).toBeVisible();
    await expect(p.getByRole('button', { name: 'Voltar' })).toBeFocused();
    await p.getByRole('button', { name: 'Voltar' }).click();
    expect((await getEncounterRPC(m, campaignId)).currentCombatantId).toBeTruthy();
    const before = (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.mine === undefined && c.label === 'Pensantus');
    expect(before?.movementLeftFt).toBe(25);

    // The Disparada spends the action and doubles the movement left.
    await p.getByRole('button', { name: 'Disparada' }).click();
    await expect(p.getByText('Ação já usada').first()).toBeVisible();
    await expect(p.getByText('Restam 15,0 m')).toBeVisible();
    // The speed already includes the Dash: the total is the doubled one, never doubled again.
    await expect(p.getByText('de 15,0 m').first()).toBeVisible();
    await expect(p.getByText('de 30,0 m')).toHaveCount(0);
    const after = (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Pensantus');
    expect(after?.movementLeftFt).toBe(50);
    await expect(p.getByRole('log', { name: 'Registro do combate' })).toHaveCount(0);

    // The action is used and the bonus action still free: still no filled button, but no question either.
    await p.getByRole('button', { name: 'Encerrar turno' }).last().click();
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
    await expect(m.getByRole('heading', { name: 'Ações do Capitão Goblin' })).toBeVisible();
    const card = m.getByRole('region', { name: 'Ações do Capitão Goblin' });
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
    await expect(m.getByRole('heading', { name: 'Ações do Capitão Goblin' })).toBeVisible();

    // Applying changes the player's hit points, and the log says so.
    await card.getByRole('button', { name: /Aplicar \d+ de dano/ }).click();
    await expect.poll(async () => (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Pensantus')?.hitPointsCurrent).toBeLessThan(23);
    await expect(m.getByRole('log', { name: 'Registro do combate' })).toContainText('Capitão Goblin ataca');

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
    await combatRPC(m, 'RollAttack', {
      campaignId,
      encounterId: enc.id,
      attackerId: enc.combatants.find((c) => c.label === 'Goblin 1')!.id,
      attackKey: 'basic:0',
      targetId: enc.combatants.find((c) => c.label === 'Pensantus')!.id,
      d20Face: 3,
    });
    await expect(m.getByRole('log', { name: 'Registro do combate' })).toContainText('Goblin 1');
    await expect(m.getByRole('log', { name: 'Registro do combate' })).toContainText('Só o mestre vê');
    await p.getByRole('button', { name: 'Abrir o registro do combate' }).click();
    await expect(p.getByRole('log', { name: 'Registro do combate' })).toBeVisible();
    await expect(p.getByRole('log', { name: 'Registro do combate' })).not.toContainText('Goblin 1');
    await expect(p.getByRole('log', { name: 'Registro do combate' })).not.toContainText('Só o mestre vê');
    await p.keyboard.press('Escape');

    // "Dano/Cura" on Goblin 2: 3 of damage, then a heal back.
    await m.getByRole('button', { name: 'Dano ou cura em Goblin 2' }).click();
    const dialog = m.getByRole('dialog', { name: 'Dano ou cura em Goblin 2' });
    await expect(dialog.getByRole('heading', { name: 'Dano ou cura em Goblin 2' })).toBeFocused();
    await expect(dialog.getByText('Agora: 7 de 7 PV')).toBeVisible();
    await dialog.getByLabel('Dano sofrido').fill('3');
    await expect(dialog.getByText('Depois: 4 de 7 PV')).toBeVisible();
    await dialog.getByRole('button', { name: 'Salvar ajuste' }).click();
    await expect(dialog).toHaveCount(0);
    await expect.poll(async () => (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.id === goblin2.id)?.hitPointsCurrent).toBe(4);
    await expect(m.getByRole('log', { name: 'Registro do combate' })).toContainText('Goblin 2 perdeu 3 PV, por ajuste do mestre, agora com 4 PV');
    // The player sees only the state word of the NPC, never the numbers.
    await expect(p.getByText('4 de 7')).toHaveCount(0);

    await m.getByRole('button', { name: 'Dano ou cura em Goblin 2' }).click();
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
    const card = m.getByRole('region', { name: 'Ações do Capitão Goblin' });
    const roll = async (face: string) => {
      await card.getByRole('button', { name: 'Digitar o resultado' }).click();
      await card.getByLabel(/Role 1d20 para Cimitarra/).fill(face);
      await card.getByRole('button', { name: `Confirmar ${face}` }).click();
    };

    // A hit that is not critical on a character who can cast Escudo waits for the reaction.
    await roll('15');
    await expect(card.getByText('Esperando a reação do Pensantus.')).toBeVisible();
    await expect(card.getByText('Ele pode conjurar Escudo Arcano (+5 na CA). O jogador decide sem ver o total; você pode responder por ele.')).toBeVisible();
    await expect(card.getByRole('button', { name: 'Rolar dano' })).toHaveAttribute('aria-disabled', 'true');
    await expect(card.getByText('Espere a reação do Pensantus.')).toBeVisible();
    // Passing the turn asks, as with a damage to apply.
    await m.getByRole('button', { name: 'Próximo turno' }).click();
    await expect(m.getByRole('alertdialog', { name: /Há dano sem aplicar/ })).toBeVisible();
    await m.getByRole('button', { name: 'Voltar' }).click();
    // The answers have the same size.
    const use = await layoutSize(card.getByRole('button', { name: 'Usar Escudo Arcano por ele' }));
    const skip = await layoutSize(card.getByRole('button', { name: 'Seguir sem Escudo Arcano' }));
    expect(use.height).toBe(skip.height);

    // Without Escudo the hit goes on to "Rolar dano" and its apply or discard.
    await card.getByRole('button', { name: 'Seguir sem Escudo Arcano' }).click();
    await card.getByRole('button', { name: 'Rolar dano' }).click();
    await expect(card.getByRole('button', { name: /Aplicar \d+ de dano/ })).toBeVisible();
    await card.getByRole('button', { name: 'Não aplicar' }).click();
    await card.getByRole('button', { name: 'Descartar' }).click();

    // Another attack: 11 + 3 = 14 reaches 13, but not 18: Escudo (+5 na CA) stops it.
    await roll('11');
    await expect(card.getByText('Esperando a reação do Pensantus.')).toBeVisible();
    await card.getByRole('button', { name: 'Usar Escudo Arcano por ele' }).click();
    await expect(card.locator('.pill', { hasText: 'Errou: o Escudo Arcano segurou' })).toBeVisible();
    await expect(card.getByRole('button', { name: /Rolar dano|Aplicar/ })).toHaveCount(0);
    await expect(m.getByRole('log', { name: 'Registro do combate' })).toContainText('o Escudo Arcano segurou');
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
    await p.getByRole('button', { name: 'Encerrar turno' }).last().click();
    const back = await layoutSize(p.getByRole('button', { name: 'Voltar' }));
    const end = await layoutSize(p.getByRole('button', { name: 'Encerrar turno' }).last());
    expect(back.height).toBe(end.height);
    expect(end.height).toBeLessThanOrEqual(48);
    expect(await p.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    await p.getByRole('button', { name: 'Voltar' }).click();

    // The physical roll's error is in view, above the sticky footer, not under it.
    await p.getByRole('button', { name: 'Atacar com Raio de Fogo' }).click();
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
    await p.getByRole('button', { name: 'Conjurar Mísseis Mágicos' }).click();
    const sheet = p.getByRole('dialog', { name: 'Conjurar Mísseis Mágicos' });
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
    await expect(p.getByText('Ação já usada').first()).toBeVisible();
    await p.getByRole('button', { name: 'Abrir o registro do combate' }).click();
    await expect(p.getByRole('log', { name: 'Registro do combate' })).toContainText('Pensantus conjura Mísseis Mágicos (1º nível): 2 dardos no Capitão Goblin');
  } finally {
    await done();
  }
});

test('uma magia de resistência em dois goblins rola o dano uma vez para a conjuração toda', { tag: ['@MR-014'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const { m, p, campaignId, done } = await actingTable(browser, 'Resistência', playerFirst, [], undefined, casting);
  try {
    await openSessionPage(p, campaignId);
    await p.getByRole('button', { name: 'Conjurar Mãos Flamejantes' }).click();
    const sheet = p.getByRole('dialog', { name: 'Conjurar Mãos Flamejantes' });
    await expect(sheet.getByText('Quem a magia atinge (pode ser ninguém)')).toBeVisible();
    await sheet.locator('label', { hasText: 'Goblin 1' }).click();
    await sheet.locator('label', { hasText: 'Goblin 2' }).click();
    await sheet.getByRole('button', { name: 'Conjurar Mãos Flamejantes' }).click();
    // The server rolls each save: the player reads the outcome and the DC.
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
    await expect(m.getByRole('log', { name: 'Registro do combate' })).toContainText('Pensantus rola o teste contra a morte: 14 · dado físico, sucesso (1 sucesso, 0 falhas)');

    // Round 3: a natural 1 counts two failures; round 4: a 5 is the third.
    enc = await passTurnsTo(m, campaignId, 'Pensantus');
    await p.getByRole('button', { name: 'Digitar o resultado' }).click();
    await p.getByLabel(/Role 1d20 para o teste contra a morte/).fill('1');
    await p.getByRole('button', { name: 'Confirmar 1' }).click();
    await expect(p.getByText(/Teste contra a morte: 1 · dado físico\. Falha: um 1 conta duas falhas\./)).toBeVisible();
    await endTurnOf(p, m, campaignId, 'Pensantus');
    enc = await passTurnsTo(m, campaignId, 'Pensantus');
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
    await row.first().getByRole('button', { name: 'Confirmar a morte' }).click();
    await m.getByRole('alertdialog').getByRole('button', { name: 'Confirmar a morte' }).click();
    await expect(m.getByText('Morto').first()).toBeVisible();
    await expect(m.getByRole('log', { name: 'Registro do combate' })).toContainText('Pensantus morreu');
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
    const card = m.getByRole('region', { name: 'Ações do Capitão Goblin' });
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
    await expect(prompt.getByRole('button', { name: 'Não usar' })).toBeFocused();
    await expect(prompt.getByRole('radio', { name: /1º nível/ })).toBeChecked();
    // No way out without an answer; the master's card waits.
    await p.keyboard.press('Escape');
    await expect(prompt).toBeVisible();
    await expect(card.getByText('Esperando a reação do Pensantus.')).toBeVisible();
    const no = await layoutSize(prompt.getByRole('button', { name: 'Não usar' }));
    const yes = await layoutSize(prompt.getByRole('button', { name: 'Conjurar Escudo Arcano' }));
    expect(yes.height).toBe(no.height);
    expect(yes.width).toBe(no.width);
    await prompt.getByRole('button', { name: 'Conjurar Escudo Arcano' }).click();
    await expect(prompt.getByText('O Escudo Arcano segurou o ataque do Capitão Goblin.')).toBeVisible();
    await expect(prompt.getByText('Espaços de 1º nível: 3 livres de 4')).toBeVisible();
    await prompt.getByRole('button', { name: 'Fechar' }).click();
    // The master's card turned the same hit into a miss, without a reload.
    await expect(card.locator('.pill', { hasText: 'Errou: o Escudo Arcano segurou' })).toBeVisible();
    await expect(card.getByText('Esperando a reação do Pensantus.')).toHaveCount(0);
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
    const card = m.getByRole('region', { name: 'Ações do Capitão Goblin' });
    await card.getByRole('button', { name: 'Digitar o resultado' }).click();
    await card.getByLabel(/Role 1d20 para Cimitarra/).fill('11');
    await card.getByRole('button', { name: 'Confirmar 11' }).click();
    const prompt = p.getByRole('alertdialog', { name: 'Você foi atingido: usar Escudo Arcano?' });
    await expect(prompt).toBeVisible();
    await card.getByRole('button', { name: 'Seguir sem Escudo Arcano' }).click();
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
    await again.getByRole('button', { name: 'Não usar' }).click();
    await expect(again).toHaveCount(0);
    await expect(card.getByRole('button', { name: 'Rolar dano' })).toBeVisible();
    await expect(card.getByText('Esperando a reação do Pensantus.')).toHaveCount(0);
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
    await m.getByRole('button', { name: 'Mais ações para Goblin 1' }).click();
    await m.getByRole('menuitem', { name: 'Condições…' }).click();
    const dialog = m.getByRole('dialog', { name: 'Condições de Goblin 1' });
    await expect(dialog.getByRole('heading', { name: 'Condições de Goblin 1' })).toBeFocused();
    await expect(dialog.getByRole('checkbox')).toHaveCount(15);
    await expect(dialog.getByText('Derrubado')).toBeVisible(); // prone is "Derrubado"; "Caído" is the 0-hit-point state
    await expect(dialog.getByRole('button', { name: 'Salvar condições' })).toHaveAttribute('aria-disabled', 'true');
    await dialog.getByRole('checkbox', { name: 'Envenenado' }).check();
    await dialog.getByRole('checkbox', { name: 'Derrubado' }).check();
    await dialog.getByRole('button', { name: 'Salvar condições' }).click();
    await expect(dialog).toHaveCount(0);
    // In the master's order, as tags under the name; on the player's strip (first one and "+1"); in the map's text list.
    await expect(m.getByRole('list', { name: 'Condições de Goblin 1' })).toContainText('Envenenado');
    await expect(m.getByRole('list', { name: 'Condições de Goblin 1' })).toContainText('Derrubado');
    await expect(p.getByRole('list', { name: 'Condições de Goblin 1' })).toContainText('Derrubado');
    await expect(p.getByRole('list', { name: 'Condições de Goblin 1' })).toContainText('+1');
    await expect(p.getByText('Goblin 1, coluna 10, linha 10, Derrubado, Envenenado')).toBeAttached();
    await p.getByRole('button', { name: 'Abrir o registro do combate' }).click();
    await expect(p.getByRole('log', { name: 'Registro do combate' })).toContainText('Goblin 1 ficou Derrubado e Envenenado');
    await p.keyboard.press('Escape');

    // Pensantus casts Teia, which asks for concentration; he sees it and may end it himself.
    await p.getByRole('button', { name: 'Conjurar Teia' }).click();
    const sheet = p.getByRole('dialog', { name: 'Conjurar Teia' });
    await sheet.getByRole('button', { name: 'Conjurar Teia' }).click();
    await expect(sheet.getByText('Você está concentrado em Teia.')).toBeVisible();
    await expect(sheet.getByText('Sua ação foi usada.')).toBeVisible();
    await sheet.getByRole('button', { name: 'Voltar à sua vez' }).click();
    await expect(p.getByText('Concentrado em Teia', { exact: true })).toBeVisible();
    await expect(m.getByText('Concentra em Teia')).toBeVisible();
    // The master sees it in the dialog too, with the same action.
    await m.getByRole('button', { name: 'Mais ações para Pensantus' }).click();
    await m.getByRole('menuitem', { name: 'Condições…' }).click();
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
    await p.getByRole('button', { name: 'Conjurar Teia' }).click();
    await p.getByRole('dialog').getByRole('button', { name: 'Conjurar Teia' }).click();
    await p.getByRole('button', { name: 'Voltar à sua vez' }).click();
    await p.getByRole('button', { name: 'Encerrar turno' }).last().click();
    const card = m.getByRole('region', { name: 'Ações do Capitão Goblin' });
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
    await expect(m.getByRole('log', { name: 'Registro do combate' })).toContainText('1 de dano (o dado deu');
    await expect(m.getByRole('log', { name: 'Registro do combate' })).toContainText('Teste de Constituição, CD 10, para manter a concentração');
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
    await expect(groups.getByText('2 usos').or(groups.getByText('1 uso')).first()).toBeVisible();
    // The first attack spends the action, but not the second one of the Attack action.
    // A natural 1 always misses: the attack is spent and no damage is left to roll.
    const attack = async () => {
      await groups.getByRole('button', { name: /^Atacar com Espada/ }).click();
      const sheet = p.getByRole('dialog', { name: /Atacar com Espada/ });
      await sheet.locator('label', { hasText: 'Goblin 1' }).click();
      await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
      await sheet.getByLabel(/Role 1d20/).fill('1');
      await sheet.getByRole('button', { name: 'Confirmar 1' }).click();
      return sheet;
    };
    let sheet = await attack();
    await expect(sheet.getByText('Você ainda tem 1 ataque desta ação.')).toBeVisible();
    await sheet.getByRole('button', { name: 'Voltar à sua vez' }).click();
    await expect(groups.getByText('1 ataque restante').first()).toBeVisible();
    await expect(groups.getByRole('button', { name: /^Atacar com Espada/ })).toBeEnabled();
    sheet = await attack();
    await expect(sheet.getByText('Sua ação foi usada.')).toBeVisible();
    await sheet.getByRole('button', { name: 'Voltar à sua vez' }).click();
    // Both attacks made: the row keeps its place with the reason.
    await expect(groups.getByText('Ataques desta ação já usados').first()).toBeVisible();
    await expect(groups.getByRole('button', { name: /^Atacar com Espada/ })).toHaveAttribute('aria-disabled', 'true');

    // Surto de Ação: another action, with its attacks.
    await groups.getByRole('button', { name: 'Usar Surto de Ação' }).click();
    await expect(groups.getByText('Surto de Ação: você tem outra ação.')).toBeVisible();
    await expect(groups.getByRole('button', { name: /^Atacar com Espada/ })).toBeEnabled();

    // Retomar o Fôlego rolls its d10 (typed here) and heals; the use is spent.
    await groups.getByRole('button', { name: 'Usar Retomar o Fôlego' }).click();
    const wind = p.getByRole('dialog', { name: 'Retomar o Fôlego' });
    await wind.getByRole('button', { name: 'Digitar o resultado' }).click();
    await wind.getByLabel(/Role 1d10/).fill('7');
    await wind.getByRole('button', { name: 'Confirmar 7' }).click();
    await expect(wind.getByText(/\d+ PV recuperados/)).toBeVisible();
    await expect(wind.getByText('Sua ação bônus foi usada.')).toBeVisible();
    await wind.getByRole('button', { name: 'Voltar à sua vez' }).click();
    await expect.poll(async () => (await vitalsOf(m, campaignId, 'Toren')).hitPointsCurrent).toBeGreaterThan(20);
    await expect(groups.getByText('Sem usos: volta num descanso curto')).toBeVisible();
    await expect(groups.getByRole('button', { name: 'Usar Retomar o Fôlego' })).toHaveAttribute('aria-disabled', 'true');
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
    await p.getByRole('button', { name: /Ataque de oportunidade/ }).click();
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
    await expect(p.getByText('Sua reação: Usada.')).toBeVisible();
    await expect(p.getByRole('button', { name: /Ataque de oportunidade/ })).toHaveCount(0);
    await expect(m.getByRole('log', { name: 'Registro do combate' })).toContainText('Pensantus ataca o Goblin 1 com a Adaga (ataque de oportunidade): errou');
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
    await p.getByRole('button', { name: 'Conjurar Curar Ferimentos' }).click();
    const sheet = p.getByRole('dialog', { name: 'Conjurar Curar Ferimentos' });
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


test('o mestre passa a vez de quem está caído com o teste por rolar; o jogador não', { tag: ['@MR-014', '@RN-03'] }, async ({ browser }) => {
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
    // The player can't pass it before rolling (no button, and the server refuses)...
    await expect(p.getByRole('button', { name: 'Encerrar turno' })).toHaveCount(0);
    const refused = await callRPC(p, 'meurpg.play.v1.CombatService/EndTurn', { campaignId, encounterId: enc.id, idempotencyKey: crypto.randomUUID(), expectedCombatantId: enc.currentCombatantId });
    expect(refused.ok()).toBe(false);
    // ...the master can, and the turn moves on.
    enc = await combatRPC(m, 'EndTurn', { campaignId, encounterId: enc.id, expectedCombatantId: enc.currentCombatantId });
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

test('a distância sai em metros e em quadrados: o quadro Movimento, a barra do turno, o cartão do mestre e a ficha', { tag: ['@MR-013'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const { m, p, table, campaignId, done } = await actingTable(browser, 'Quadrados', playerFirst);
  try {
    await openSessionPage(p, campaignId);
    await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
    // The Movimento tile: the meters, and the squares under the bar.
    const tiles = p.getByRole('list', { name: 'O que você tem neste turno' });
    await expect(tiles.getByText('7,5 m de 7,5 m')).toBeVisible();
    await expect(tiles.getByText('5 quadrados livres')).toBeVisible();
    // The turn bar: "Mover 7,5 m · 5 quadrados" in a cell of its own, inside the bar.
    const bar = p.locator('app-turn-bar');
    await expect(bar.getByText('Mover 7,5 m · 5 quadrados')).toBeVisible();
    expect(await bar.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
    // The "Movimento" group says it in a sentence.
    await expect(p.getByText('Você ainda não andou. Dá para andar até 7,5 m (5 quadrados).')).toBeVisible();

    // The master's card for the captain (a gnome too): the Deslocamento in both units, and the Movimento chip.
    await openSessionPage(m, campaignId);
    await passTurnsTo(m, campaignId, 'Capitão Goblin');
    const card = m.getByRole('region', { name: /Ações do Capitão Goblin|Vez do Capitão Goblin/ });
    await expect(card.locator('.stat', { hasText: 'Deslocamento' })).toContainText('7,5 m · 5 quadrados');
    await expect(card.locator('.chip', { hasText: 'Movimento' })).toContainText('7,5 m · 5 quadrados');

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
    const names = groups.locator('app-action-row:has(app-spell-help) .row__name');
    await expect(names).toHaveText(['Raio de Fogo', 'Mísseis Mágicos', 'Sono', 'Passo Nebuloso', 'Teia', 'Escudo Arcano']);
    const shield = groups.locator('app-action-row', { hasText: 'Escudo Arcano' });
    await expect(shield.getByText('Só fora da sua vez')).toBeVisible();
    await expect(shield.getByRole('button', { name: 'Conjurar Escudo Arcano' })).toHaveAttribute('aria-disabled', 'true');
    await expect(shield.getByText('Reação', { exact: true })).toBeVisible();

    // The "?" opens the spell's description in the session, and "Fechar" gives the focus back to it.
    const help = groups.getByRole('button', { name: 'Detalhes de Sono' });
    await help.click();
    const details = p.getByRole('dialog', { name: 'Descrição de Sono' });
    await expect(details.getByRole('heading', { name: 'Sono' })).toBeVisible();
    await expect(details.getByText('Tempo de conjuração')).toBeVisible();
    await expect(details.getByText('This spell sends creatures into a magical slumber.')).toBeVisible();
    await expect(details.getByRole('button', { name: 'Fechar' }).last()).toBeInViewport({ ratio: 1 });
    await details.getByRole('button', { name: 'Fechar' }).last().click();
    await expect(details).toHaveCount(0);
    await expect(help).toBeFocused();

    // With no free slot the 1st and 2nd circles say "Sem espaço", and the slot rows above are the explanation.
    await adjustVitalsRPC(m, campaignId, table.characterId, { spellSlotsUsed: [{ level: 1, used: 4 }, { level: 2, used: 2 }] });
    await openSessionPage(p, campaignId);
    await expect(groups.getByText('0 livres de 4')).toBeVisible();
    await expect(groups.getByText('0 livres de 2')).toBeVisible();
    await expect(groups.locator('.row__why', { hasText: /^\s*block\s*Sem espaço\s*$/ })).toHaveCount(4);
    await expect(groups.getByText(/Sem espaço de/)).toHaveCount(0);
    // Nothing can be cast now: the order is by circle, then by name, and Escudo Arcano is among them.
    await expect(names).toHaveText(['Raio de Fogo', 'Escudo Arcano', 'Mísseis Mágicos', 'Sono', 'Passo Nebuloso', 'Teia']);
  } finally {
    await done();
  }
});

test('Sono em dois goblins e no Capitão: o mestre vê o total e os PV, o jogador só vê quem adormeceu', { tag: ['@MR-014', '@RN-20'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  const { m, p, campaignId, done } = await actingTable(browser, 'Sono', playerFirst);
  try {
    await openSessionPage(m, campaignId);
    await openSessionPage(p, campaignId);
    await p.getByRole('button', { name: 'Conjurar Sono' }).click();
    const sheet = p.getByRole('dialog', { name: 'Conjurar Sono' });
    await expect(sheet.getByRole('heading', { name: 'Conjurar Sono' })).toBeFocused();
    await expect(sheet.getByText('Ação · alcance 27 m · 5d8 de pontos de vida')).toBeVisible();
    // The caster says who is in the area, by name: no hit points on the list.
    await expect(sheet.getByText('Quem está na área da magia')).toBeVisible();
    await expect(sheet.getByText('O mestre confere quem está na área. Você não vê os pontos de vida dos inimigos.')).toBeVisible();
    await expect(sheet.getByText(/\bPV\b/)).toHaveCount(0);
    await sheet.locator('label', { hasText: 'Goblin 1' }).click();
    await sheet.locator('label', { hasText: 'Goblin 2' }).click();
    await sheet.locator('label', { hasText: 'Capitão Goblin' }).click();

    // The "?" in the header opens the description over the sheet; "Fechar" comes back to the choices.
    await sheet.getByRole('button', { name: 'Detalhes de Sono' }).click();
    const details = p.getByRole('dialog', { name: 'Descrição de Sono' });
    await expect(details.getByText('This spell sends creatures into a magical slumber.')).toBeVisible();
    await details.getByRole('button', { name: 'Fechar' }).last().click();
    await expect(details).toHaveCount(0);
    await expect(sheet.getByRole('checkbox', { name: /Goblin 1/ })).toBeChecked();

    // The pool is typed from the physical dice: 20 reaches both goblins (7 + 7), not the captain.
    await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
    await sheet.getByLabel(/Role 5d8/).fill('41');
    await expect(sheet.getByText('Digite um número de 5 a 40')).toBeVisible();
    await sheet.getByLabel(/Role 5d8/).fill('20');
    await sheet.getByRole('button', { name: 'Confirmar 20' }).click();

    // The player: who fell asleep, who was not affected, the caster's own roll, and no number of an enemy's.
    await expect(sheet.getByRole('heading', { name: 'Sono conjurado' })).toBeVisible();
    await expect(sheet.getByText('O Goblin 1 adormeceu. O Goblin 2 adormeceu. O Capitão Goblin não foi afetado.')).toBeVisible();
    await expect(sheet.getByText('Sua rolagem')).toBeVisible();
    await expect(sheet.getByText('5d8 = 20 · dado físico')).toBeVisible();
    await expect(sheet.getByText(/\bPV\b|restam|restantes/)).toHaveCount(0);
    await sheet.getByRole('button', { name: 'Voltar à sua vez' }).click();
    await p.getByRole('button', { name: 'Abrir o registro do combate' }).click();
    const playerLog = p.getByRole('log', { name: 'Registro do combate' });
    await expect(playerLog).toContainText('Pensantus conjura Sono: o Goblin 1 adormece. O Goblin 2 adormece. O Capitão Goblin não foi afetado.');
    await expect(playerLog).not.toContainText('PV');

    // The master: the pool, each creature from the lowest hit points up with the total that is left.
    const masterLog = m.getByRole('log', { name: 'Registro do combate' });
    await expect(masterLog).toContainText('Pensantus conjura Sono (1º nível)');
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
