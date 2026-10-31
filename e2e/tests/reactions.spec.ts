import { expect, type Browser, type BrowserContext, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { attackCombatSquares, beginAttackCombatRPC, combatRPC, getEncounterRPC, pensantusCasting, tableForCombat, type CombatTable } from './combat-support';
import { endOpenSessionRPC, openSessionPage } from './live-session-support';
import { callRPC, newSignedInContext, viaTargetList } from './support';
import { setTableRulesRPC } from './table-rules-support';
import { waitForEncounterRevision } from './revision-support';

// The reaction window (PM-04): an NPC's Escudo holds a player's turn and the player reads only "Esperando o mestre",
// a Contramágica asks the master about a player's cast, and a damage on a concentrating character asks its owner for
// the save. The table and the NPCs come through the API; what is under test is what each screen reads (RN-10, RN-20).

// The Mago joins at the start with the lowest face: its total differs from every other (beginAttackCombatRPC checks it)
// and it has a total of its own (23, 18, 5, 4 and 3 are distinct). It cannot be added to the running combat: the master cannot set its initiative then, and the app's
// roll could tie with the actor.
const playerFirst = { Pensantus: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4, Mago: 1 };

interface ReactionTable {
  m: Page;
  p: Page;
  table: CombatTable;
  campaignId: string;
  /** The label of the Mago, in plain sight next to Pensantus; empty when the table has none. */
  mage: string;
  done: () => Promise<void>;
}

/** A combat with Pensantus (casting), the Capitão and two Goblins, begun with Pensantus first; with `withMage`, the SRD Mago too. */
async function reactionTable(browser: Browser, name: string, withMage = false): Promise<ReactionTable> {
  const master: BrowserContext = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
  const player: BrowserContext = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 390, height: 844 } });
  const m = await master.newPage();
  const p = await player.newPage();
  await m.goto('/');
  await p.goto('/');
  const table = await tableForCombat(m, p, `${name} ${Date.now()}`, true, true, { sheet: pensantusCasting });
  const begun = await beginAttackCombatRPC(m, table, playerFirst, withMage ? { ...attackCombatSquares, Mago: [8, 8] } : undefined, [], withMage ? ['monster:mage'] : []);
  return {
    m,
    mage: begun.combatants.find((c) => c.label.startsWith('Mago'))?.label ?? '',
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

/** Pensantus attacks the Mago with Raio de Fogo, typing a 10 (16 against armor class 12: Escudo would make it 17). */
async function attackTheMage(p: Page, mage: string): Promise<ReturnType<Page['getByRole']>> {
  await p.getByRole('button', { name: 'Atacar com Raio de Fogo' }).click();
  const sheet = p.getByRole('dialog', { name: 'Atacar com Raio de Fogo' });
  await viaTargetList(p, sheet);
  await sheet.locator('label', { hasText: mage }).click();
  await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
  await sheet.getByLabel(/Role 1d20 para Raio de Fogo/).fill('10');
  await sheet.getByRole('button', { name: 'Confirmar 10' }).click();
  return sheet;
}

test(
  'o Escudo de um NPC segura o turno do jogador: ele lê só "Esperando o mestre", e o mestre responde pelo Mago',
  { tag: ['@PM-04', '@RN-10', '@RN-20'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const { m, p, campaignId, mage, done } = await reactionTable(browser, 'Escudo de NPC', true);
    try {
      await openSessionPage(m, campaignId);
      await openSessionPage(p, campaignId);
      const sheet = await attackTheMage(p, mage);

      // The player's screen: the wait and nothing of the NPC, its spell or its numbers.
      await expect(sheet.getByText('Esperando o mestre.').first()).toBeVisible();
      await expect(p.getByText(/pode conjurar|Contramágica|a sua reação/)).toHaveCount(0);
      await expect(p.getByRole('alertdialog')).toHaveCount(0);
      await sheet.getByRole('button', { name: 'Voltar à sua vez' }).click();
      const wait = p.getByRole('status').filter({ hasText: 'Esperando o mestre.' });
      await expect(wait).toContainText('O resultado do seu ataque sai quando ele responder.');
      await expect(p.getByRole('button', { name: 'Encerrar turno' })).toHaveAttribute('aria-disabled', 'true');
      await expect(wait).not.toContainText(/Escudo|Contramágica|Mago/);

      // The master's: the card of the Mago, what Escudo would change, and the turn that does not pass meanwhile.
      const card = m.getByRole('region', { name: mage });
      await expect(card.getByText(/17 contra 16 viraria erro/)).toBeVisible();
      await expect(m.getByRole('button', { name: 'Próximo turno' })).toBeDisabled();
      await expect(m.getByRole('button', { name: 'Rolar dano' })).toBeDisabled();
      await card.getByRole('button', { name: `Usar Escudo Arcano pelo ${mage}` }).click();

      // The hit became a miss, the wait is gone, and the turn goes on.
      await expect(wait).toHaveCount(0);
      await expect(p.getByRole('button', { name: 'Encerrar turno' })).not.toHaveAttribute('aria-disabled', 'true');
      await p.getByRole('button', { name: 'Abrir o registro do combate' }).click();
      await expect(p.getByRole('log', { name: 'Registro do combate' })).toContainText('Escudo Arcano');
    } finally {
      await done();
    }
  },
);

test(
  'a Contramágica de um NPC segura a conjuração do jogador, e o mestre responde sem o jogador ler o nome da magia',
  { tag: ['@PM-04', '@RN-10', '@RN-20'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const { m, p, campaignId, mage, done } = await reactionTable(browser, 'Contramágica', true);
    try {
      await openSessionPage(m, campaignId);
      await openSessionPage(p, campaignId);
      const enc = await getEncounterRPC(m, campaignId);
      const id = (label: string) => enc.combatants.find((c) => c.label === label)!.id;
      // Pensantus casts Mísseis Mágicos at Goblin 1: the cast waits for the Mago.
      const cast = await callRPC(p, 'meurpg.play.v1.CombatService/CastSpell', {
        campaignId,
        encounterId: enc.id,
        casterId: id('Pensantus'),
        spellKey: 'spell:magic-missile',
        slot: { level: 1 },
        targets: [{ combatantId: id('Goblin 1'), darts: 3 }],
        idempotencyKey: crypto.randomUUID(),
      });
      expect(cast.ok(), await cast.text()).toBeTruthy();

      // The caster's screen: the wait, and nothing of who can answer or of what was cast.
      const wait = p.getByRole('status').filter({ hasText: 'Esperando o mestre.' });
      await expect(wait).toBeVisible();
      await expect(wait).not.toContainText(/Contramágica|Mago|Mísseis/);
      await expect(p.getByRole('alertdialog')).toHaveCount(0);
      await expect(p.getByRole('button', { name: 'Encerrar turno' })).toHaveAttribute('aria-disabled', 'true');

      // The master's card of the Mago: who is casting (the master knows the spell: the player never reads it).
      const card = m.getByRole('region', { name: mage });
      await expect(card.getByText(/Pensantus/)).toBeVisible();
      await card.getByRole('button', { name: /Usar Contramágica/ }).click();
      await expect(wait).toHaveCount(0);
    } finally {
      await done();
    }
  },
);

// The DC is the master's unless the table's rule "Os jogadores veem a CD dos testes de resistência" is on (RN-24, RN-10).
for (const showDC of [false, true]) {
  test(
    `o dano em quem se concentra pede o teste ao dono: ele rola no app, e o ataque que deu o dano espera (${showDC ? 'com' : 'sem'} a CD)`,
    { tag: ['@PM-04', '@RN-22'] },
    async ({ browser }) => {
      test.setTimeout(240_000);
      const { m, p, campaignId, done } = await reactionTable(browser, 'Concentração');
      try {
        await setTableRulesRPC(m, campaignId, { showSaveDc: showDC });
        await openSessionPage(m, campaignId);
        await openSessionPage(p, campaignId);
        let enc = await getEncounterRPC(m, campaignId);
        const id = (label: string) => enc.combatants.find((c) => c.label === label)!.id;
        // Pensantus concentrates on Teia and ends his turn; the Capitão hits him.
        const cast = await callRPC(p, 'meurpg.play.v1.CombatService/CastSpell', {
          campaignId,
          encounterId: enc.id,
          casterId: id('Pensantus'),
          spellKey: 'spell:web',
          slot: { level: 2 },
          targets: [],
          idempotencyKey: crypto.randomUUID(),
        });
        expect(cast.ok(), await cast.text()).toBeTruthy();
        enc = await combatRPC(m, 'EndTurn', { campaignId, encounterId: enc.id, expectedCombatantId: id('Pensantus') });
        await waitForEncounterRevision([m, p], enc);
        const hit = await callRPC(m, 'meurpg.play.v1.CombatService/RollAttack', {
          campaignId,
          encounterId: enc.id,
          attackerId: id('Capitão Goblin'),
          attackKey: 'equipment:scimitar',
          targetId: id('Pensantus'),
          d20Face: 15,
          idempotencyKey: crypto.randomUUID(),
        });
        expect(hit.ok(), await hit.text()).toBeTruthy();
        const pending = (await hit.json()).pendingDamage.id as string;
        // Escudo asks first; he lets it pass.
        const shield = p.getByRole('alertdialog', { name: 'Você foi atingido: usar Escudo Arcano?' });
        await expect(shield).toBeVisible();
        await shield.getByRole('button', { name: 'Deixar passar' }).click();
        // The sheet closes only once AnswerReaction returns; until then the window is open and RollDamage is refused.
        await expect(shield).toHaveCount(0);
        const dmg = await callRPC(m, 'meurpg.play.v1.CombatService/RollDamage', { campaignId, encounterId: enc.id, pendingDamageId: pending, rollInApp: true, idempotencyKey: crypto.randomUUID() });
        expect(dmg.ok(), await dmg.text()).toBeTruthy();
        const apply = await callRPC(m, 'meurpg.play.v1.CombatService/ApplyPendingDamage', { campaignId, encounterId: enc.id, pendingDamageId: pending, idempotencyKey: crypto.randomUUID() });
        expect(apply.ok(), await apply.text()).toBeTruthy();

        // The owner's prompt, with the save the SRD asks for (Constituição, CD 10 or half the damage).
        const save = p.getByRole('alertdialog', { name: showDC ? /Concentração em risco: teste de resistência de Constituição contra CD \d+/ : /^Concentração em risco: teste de resistência de Constituição$/ });
        await expect(save).toBeVisible();
        await expect(save.getByText(/mantém a concentração em Teia/)).toBeVisible();
        await expect(save.getByRole('button', { name: 'Rolar no app' })).toBeVisible();
        await expect(save.getByRole('button', { name: 'Deixar o mestre rolar por mim' })).toBeVisible();
        // The turn does not pass while the save waits, for the master either.
        await expect(m.getByRole('button', { name: 'Próximo turno' })).toBeDisabled();
        await save.getByRole('button', { name: 'Rolar no app' }).click();
        await expect(save.getByText(/Você (manteve|perdeu) a concentração/)).toBeVisible();
        await save.getByRole('button', { name: 'Fechar' }).click();
        await expect(m.getByRole('button', { name: 'Próximo turno' })).toBeEnabled();
      } finally {
        await done();
      }
    },
  );
}
