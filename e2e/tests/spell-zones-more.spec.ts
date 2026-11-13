import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { getEncounterRPC, passTurnsTo, passTurnsToAndWait } from './combat-support';
import { openSessionPage } from './live-session-support';
import { callRPC } from './support';
import { combatRPC } from './combat-support';
import { castZoneRPC, moreZonesTable, expectOnMap, zoneCard, zonesPanel, zonesRPC } from './zones-support';

// The eight spells that keep an area (RN-37) on the two screens, past the three the first spec covers: what the master's and
// the player's cards read, the master's calls, the saves a player is asked and the log's lines. The table and the casts come
// through the API; what is under test is what each screen reads.

test(
  'Névoa Obscurecente e Escuridão: o cartão do mestre traz a decisão dele, o do jogador não',
  { tag: ['@RN-37'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const t = await moreZonesTable(
      browser,
      'Névoa e escuridão',
      [
        { label: 'Druida', at: [3, 2], face: 20 },
        { label: 'Mago', at: [3, 12], face: 19 },
      ],
      { fog: true },
    );
    try {
      await castZoneRPC(t.m, t, t.id('Druida'), 'spell:fog-cloud', { col: 12, row: 3 }, 1);
      await passTurnsTo(t.m, t.campaignId, 'Mago');
      await castZoneRPC(t.m, t, t.id('Mago'), 'spell:darkness', { col: 12, row: 10 }, 2);
      await openSessionPage(t.m, t.campaignId);
      await openSessionPage(t.p, t.campaignId);

      // The master's cards: the zone, and the one call each spell leaves to the table (RN-37).
      await expect(zonesPanel(t.m).getByRole('heading', { level: 2 })).toHaveText('Zonas ativas (2)');
      await expect(zoneCard(t.m, 'Névoa Obscurecente · Druida')).toContainText('Esfera de 6 m de raio');
      await expect(zoneCard(t.m, 'Névoa Obscurecente · Druida')).toContainText('Você decide: um vento forte dispersa a Névoa em 1 rodada.');
      await expect(zoneCard(t.m, 'Escuridão · Mago')).toContainText('Esfera de 4,5 m de raio');
      await expect(zoneCard(t.m, 'Escuridão · Mago')).toContainText('Você decide: a luz de uma magia de nível 2 ou menor que toque a Escuridão é dissipada.');
      for (const label of ['Névoa Obscurecente · Druida', 'Escuridão · Mago']) {
        await expectOnMap(t.m, label);
        await expectOnMap(t.p, label);
      }

      // The player reads the same two zones by name and area alone: no call of the master's.
      await expect(zonesPanel(t.p).getByRole('heading', { level: 2 })).toHaveText('Zonas ativas (2)');
      await expect(zonesPanel(t.p)).not.toContainText('Você decide');
      await expect(zoneCard(t.p, 'Escuridão · Mago')).toContainText('Esfera de 4,5 m de raio');
    } finally {
      await t.done();
    }
  },
);

test(
  'Área Escorregadia: o jogador que estava na área recebe o teste de Destreza, cai no chão e o registro diz o resultado',
  { tag: ['@RN-37'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const t = await moreZonesTable(browser, 'Área escorregadia', [{ label: 'Druida', at: [3, 2], face: 20 }]);
    try {
      const me = (await getEncounterRPC(t.m, t.campaignId)).combatants.find((c) => c.label === 'Pensantus')!;
      await openSessionPage(t.p, t.campaignId);
      await openSessionPage(t.m, t.campaignId);
      await castZoneRPC(t.m, t, t.id('Druida'), 'spell:grease', { col: me.col ?? 0, row: me.row ?? 0 }, 1);

      const sheet = t.p.getByRole('dialog', { name: 'Área Escorregadia de Druida' });
      await expect(sheet).toBeVisible();
      await expect(sheet).toContainText('Você estava na área quando a magia foi conjurada.');
      await expect(sheet).toContainText('Teste de resistência de Destreza. Se falhar, você cai no chão.');
      await expect(sheet).not.toContainText(/\bCD\b/);
      await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
      await sheet.getByLabel(/Resultado do d20/).fill('2');
      await sheet.getByRole('button', { name: 'Confirmar 2' }).click();
      await expect(sheet).toContainText('Falhou');
      await expect(sheet).toContainText('1d20 (2) + 3');
      await sheet.getByRole('button', { name: 'Fechar', exact: true }).last().click();
      await expect(t.p.getByText('Derrubado', { exact: true }).first()).toBeVisible();
      await t.p.getByRole('button', { name: /Registro do combate/ }).click();
      await expect(t.p.getByText('Pensantus estava na Área Escorregadia quando a magia foi conjurada e falhou no teste de Destreza')).toBeVisible();
    } finally {
      await t.done();
    }
  },
);

test(
  'Constrição: o teste é de Força, quem falha fica contido e o registro diz que falhou',
  { tag: ['@RN-37'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const t = await moreZonesTable(browser, 'Constrição', [{ label: 'Druida', at: [3, 2], face: 20 }]);
    try {
      const me = (await getEncounterRPC(t.m, t.campaignId)).combatants.find((c) => c.label === 'Pensantus')!;
      await openSessionPage(t.p, t.campaignId);
      await castZoneRPC(t.m, t, t.id('Druida'), 'spell:entangle', { col: me.col ?? 0, row: me.row ?? 0 }, 1);

      const sheet = t.p.getByRole('dialog', { name: 'Constrição de Druida' });
      await expect(sheet).toContainText('Teste de resistência de Força. Se falhar, você fica contido até se soltar.');
      await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
      await sheet.getByLabel(/Resultado do d20/).fill('2');
      await sheet.getByRole('button', { name: 'Confirmar 2' }).click();
      await expect(sheet).toContainText('Falhou');
      await sheet.getByRole('button', { name: 'Fechar', exact: true }).last().click();
      await expect(t.p.getByText('Preso pelas plantas', { exact: true }).first()).toBeVisible();
      await t.p.getByRole('button', { name: /Registro do combate/ }).click();
      await expect(t.p.getByText('falhou no teste de Força')).toBeVisible();
    } finally {
      await t.done();
    }
  },
);

test(
  'Névoa Fétida: quem começa o turno na nuvem faz o teste de Constituição; o mestre lê a decisão sobre quem não respira e o jogador não',
  { tag: ['@RN-37'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const t = await moreZonesTable(browser, 'Névoa fétida', [{ label: 'Druida', at: [3, 2], face: 20 }]);
    try {
      const me = (await getEncounterRPC(t.m, t.campaignId)).combatants.find((c) => c.label === 'Pensantus')!;
      await openSessionPage(t.p, t.campaignId);
      await openSessionPage(t.m, t.campaignId);
      await castZoneRPC(t.m, t, t.id('Druida'), 'spell:stinking-cloud', { col: me.col ?? 0, row: me.row ?? 0 }, 3);
      await expect(zoneCard(t.m, 'Névoa Fétida · Druida')).toContainText('Você decide: quem não precisa respirar passa no teste sem rolar.');
      await expectOnMap(t.p, 'Névoa Fétida · Druida');
      await expect(zoneCard(t.p, 'Névoa Fétida · Druida')).toBeVisible();
      await expect(zoneCard(t.p, 'Névoa Fétida · Druida')).not.toContainText('Você decide');
      await expect(t.p.getByRole('dialog')).toHaveCount(0);

      // The cloud asks at the start of the turn, not at the cast.
      await passTurnsToAndWait(t.m, t.campaignId, 'Pensantus', t.p);
      const sheet = t.p.getByRole('dialog', { name: 'Névoa Fétida de Druida' });
      await expect(sheet).toContainText('Teste de resistência de Constituição. Se falhar, você passa a ação deste turno enjoado e cambaleando.');
      await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
      await sheet.getByLabel(/Resultado do d20/).fill('2');
      await sheet.getByRole('button', { name: 'Confirmar 2' }).click();
      await expect(sheet).toContainText('Falhou');
      await sheet.getByRole('button', { name: 'Fechar', exact: true }).last().click();
      await t.p.getByRole('button', { name: /Registro do combate/ }).click();
      await expect(t.p.getByText('falhou no teste de Constituição').first()).toBeVisible();
    } finally {
      await t.done();
    }
  },
);

test(
  'Silêncio: quem está dentro fica surdo e a magia com componente verbal é recusada com o motivo',
  { tag: ['@RN-37'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const t = await moreZonesTable(browser, 'Silêncio', [
      { label: 'Druida', at: [3, 2], face: 20 },
      { label: 'Mago', at: [3, 12], face: 19 },
    ]);
    try {
      await openSessionPage(t.m, t.campaignId);
      await castZoneRPC(t.m, t, t.id('Druida'), 'spell:silence', { col: 3, row: 12 }, 2);
      await passTurnsTo(t.m, t.campaignId, 'Mago');
      await expectOnMap(t.m, 'Silêncio · Druida');
      await expect(t.m.getByText('Surdo', { exact: true }).first()).toBeVisible();

      const res = await callRPC(t.m, 'meurpg.play.v1.CombatService/CastSpell', {
        campaignId: t.campaignId,
        encounterId: t.encounterId,
        casterId: t.id('Mago'),
        spellKey: 'spell:fog-cloud',
        slot: { level: 1 },
        targets: [],
        origin: { col: 8, row: 8 },
        idempotencyKey: crypto.randomUUID(),
      });
      expect(res.status()).toBe(400);
      expect(await res.text()).toContain('a spell with a verbal component cannot be cast');
      expect(await zonesRPC(t.m, t.campaignId)).toHaveLength(1);
    } finally {
      await t.done();
    }
  },
);

test(
  'Crescer Espinhos: quem anda dentro da área deixa um dano pendente, e o jogador lê só o nome e a área',
  { tag: ['@RN-37'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const t = await moreZonesTable(browser, 'Crescer espinhos', [{ label: 'Druida', at: [3, 2], face: 20 }]);
    try {
      const me = (await getEncounterRPC(t.m, t.campaignId)).combatants.find((c) => c.label === 'Pensantus')!;
      await openSessionPage(t.m, t.campaignId);
      await openSessionPage(t.p, t.campaignId);
      await castZoneRPC(t.m, t, t.id('Druida'), 'spell:spike-growth', { col: (me.col ?? 0) + 4, row: me.row ?? 0 }, 2);
      await expect(zoneCard(t.m, 'Crescer Espinhos · Druida')).toContainText('terreno difícil');
      await expect(zoneCard(t.p, 'Crescer Espinhos · Druida')).toBeVisible();
      await expect(zoneCard(t.p, 'Crescer Espinhos · Druida')).not.toContainText('terreno difícil');
      await combatRPC(t.m, 'MoveCombatant', { campaignId: t.campaignId, encounterId: t.encounterId, combatantId: t.id('Pensantus'), col: (me.col ?? 0) + 4, row: me.row ?? 0 });
      await expect(t.m.getByText('Falta rolar o dano')).toBeVisible();
    } finally {
      await t.done();
    }
  },
);
