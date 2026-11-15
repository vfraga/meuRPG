import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { endOpenSessionRPC, openSessionPage } from './live-session-support';
import { callRPC } from './support';
import { aloneFlow, damageCard, otherPlayerLogFlow, pendingIdOf, rolledFlow, sneakHit, takeOutTable, twoPartsFlow } from './take-out-support';

// #835: the master takes an extra out of a typed group. The damage was applied to an NPC: the app undoes it, the roller
// types the sum of that damage type again, and the damage is applied anew. The table comes through the API; what is under test
// is what the master reads and the player is shown.

test(
  'tirar o Ataque Furtivo de um grupo digitado: o mestre vê o que se desfaz, espera a soma, e a jogadora digita só a soma de perfurante',
  { tag: ['@MR-012', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(180_000);
    const t = await takeOutTable(browser);
    const { m, p, campaignId } = t;
    try {
      await sneakHit(t);
      await openSessionPage(m, campaignId);
      const card = damageCard(m);
      await expect(card.getByText('Aplicado · 15')).toBeVisible();
      await expect(card.getByText('4d6')).toHaveCount(0);

      // Sneak Attack is the only extra; the rapier has no "Tirar".
      await expect(card.getByRole('button', { name: /^Tirar o / })).toHaveCount(1);
      await card.getByRole('button', { name: 'Tirar o Ataque Furtivo' }).click();
      const panel = card.getByRole('alertdialog', { name: 'Tirar o Ataque Furtivo do dano?' });
      await expect(panel.getByText(/Desfaz os 15 de dano: o Goblin 1 (volta a \d+ PV|recupera os PV que perdeu)\./)).toBeVisible();
      await expect(panel.getByText('Devolve o Ataque Furtivo deste turno.')).toBeVisible();
      await expect(panel.getByText(/Pede a Vex só a soma de perfurante \(1d8\)\./)).toBeVisible();

      // The reason is required.
      const field = panel.getByLabel('Motivo (aparece no registro)');
      await field.fill('x');
      await field.fill('');
      await expect(panel.getByText('Escreva o motivo, com até 120 caracteres.')).toBeVisible();
      await expect(panel.getByRole('button', { name: 'Tirar e pedir a soma' })).toHaveAttribute('aria-disabled', 'true');
      await field.fill('Sem aliado perto do alvo');
      // How often each page asks for the turn options from the take-out on (the master's call reads the log).
      const calls = { master: 0, player: 0 };
      const count = (who: 'master' | 'player') => (r: { url(): string }) => {
        calls[who] += Number(/GetTurnOptions/.test(r.url()));
      };
      m.on('request', count('master'));
      p.on('request', count('player'));
      await panel.getByRole('button', { name: 'Tirar e pedir a soma' }).click();

      // The damage waits for the sum of the piercing alone; the table goes on.
      await expect(card.getByText('Esperando Vex')).toBeVisible();
      await expect(card.getByText('tirado: “Sem aliado perto do alvo”')).toBeVisible();
      await expect(m.getByText('Falta a soma de Vex').first()).toBeVisible();

      // The player's card says only what she may type: the notice, the parts and one field, never the target's hit points.
      await expect(p.getByText('Falta digitar de novo a soma do dano do seu ataque.')).toBeVisible();
      await p.getByRole('button', { name: 'Digitar a soma' }).click();
      const sheet = p.getByRole('dialog').filter({ has: p.getByRole('heading', { name: 'Digitar o dano de novo' }) });
      await expect(sheet.getByText('O mestre tirou o Ataque Furtivo: “Sem aliado perto do alvo”. Digite de novo só a soma de perfurante.')).toBeVisible();
      await expect(sheet.getByText(/ PV/)).toHaveCount(0);
      await expect(sheet.getByLabel('Rapieira, perfurante: 1d8')).toHaveCount(1);
      await sheet.getByLabel('Rapieira, perfurante: 1d8').fill('1');
      await expect(sheet.getByText('Dano: 1 + 3 = 4')).toBeVisible();
      await sheet.getByRole('button', { name: 'Aplicar 4 de dano' }).click();

      // The damage lands again: nothing waits any more, and the part is out for good (it goes once), so there is no new "Tirar".
      await expect(m.getByText('Falta a soma de Vex')).toHaveCount(0);
      await expect(card.getByText('Esperando Vex')).toHaveCount(0);
      test.info().annotations.push({ type: 'GetTurnOptions', description: `master ${calls.master}, player ${calls.player}` });
      await expect(card.getByRole('button', { name: /^Tirar o / })).toHaveCount(0);
    } finally {
      await endOpenSessionRPC(m, campaignId).catch(() => undefined);
    }
  },
);

test(
  'tirar a parte depois de outra ação é recusado: nada muda e o servidor diz que já aconteceu outra coisa',
  { tag: ['@MR-012'] },
  async ({ browser }) => {
    test.setTimeout(120_000);
    const t = await takeOutTable(browser);
    const { m, campaignId } = t;
    try {
      await sneakHit(t);
      const pending = await pendingIdOf(t);
      // Anything the master does afterwards closes the undo chain: here, a correction of the captain's hit points.
      await callRPC(m, 'meurpg.play.v1.CombatService/AdjustCombatantHitPoints', {
        campaignId,
        encounterId: t.encounterId,
        combatantId: t.id('Goblin 1'),
        heal: 1,
        idempotencyKey: crypto.randomUUID(),
      });
      const res = await callRPC(m, 'meurpg.play.v1.CombatService/RemoveDamagePart', {
        campaignId,
        encounterId: t.encounterId,
        pendingDamageId: pending,
        partKey: 'sneak-attack',
        reason: 'tarde demais',
        idempotencyKey: crypto.randomUUID(),
      });
      expect(res.status()).toBe(400);
      expect(await res.text()).toMatch(/NOTHING_TO_UNDO|UNDO_TARGET_CHANGED|failed_precondition/);
      await openSessionPage(m, campaignId);
      await expect(damageCard(m).getByRole('button', { name: /^Tirar o / })).toHaveCount(0);
    } finally {
      await endOpenSessionRPC(m, campaignId).catch(() => undefined);
    }
  },
);

// The three other states of the board 835-04 (2, 3 and 4), each on its own table: the flows say what the master and the roller read.
test(
  'tirar uma parte sozinha no seu tipo: sai como antes, sem pedir uma nova soma',
  { tag: ['@MR-012'] },
  async ({ browser }) => {
    test.setTimeout(180_000);
    const t = await takeOutTable(browser, {}, { feats: ['radiant'] });
    try {
      await aloneFlow(t);
    } finally {
      await t.done().catch(() => undefined);
    }
  },
);

test(
  'tirar um extra de um grupo que continua com duas partes: o cartão pede uma soma das duas',
  { tag: ['@MR-012', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(180_000);
    const t = await takeOutTable(browser, {}, { feats: ['pierce'] });
    try {
      await twoPartsFlow(t);
    } finally {
      await t.done().catch(() => undefined);
    }
  },
);

test(
  'tirar um extra de um dano rolado e ainda não aplicado a um personagem: nada é desfeito',
  { tag: ['@MR-012'] },
  async ({ browser }) => {
    test.setTimeout(180_000);
    const t = await takeOutTable(browser, {}, { ally: true });
    try {
      await rolledFlow(t);
    } finally {
      await t.done().catch(() => undefined);
    }
  },
);

test(
  'tirar uma parte: outra jogadora lê no registro que o mestre tirou o Ataque Furtivo, sem o motivo',
  { tag: ['@MR-012'] },
  async ({ browser }) => {
    test.setTimeout(180_000);
    const t = await takeOutTable(browser, {}, { ally: true });
    try {
      await otherPlayerLogFlow(t);
    } finally {
      await t.done().catch(() => undefined);
    }
  },
);
