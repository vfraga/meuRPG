import { expect, test } from '@playwright/test';

import { combatRPC, getEncounterRPC, torenSheet, toren, pensantusCasting } from './combat-support';
import { openSessionPage } from './live-session-support';
import { boxOf, layoutSize } from './support';
import { movingTable, paintRPC, pickRadio, tapSquare } from './move-support';

// Movement, jumps, cover, "Aliado" and the opportunity attacks on screen (Etapa 9,
// slice 9.15, MR-034, RN-21, questions 68 to 70). The table, the layers and the
// combat come through the API; what is under test is the screens: what the
// player reads from the server's reach, what the master answers, and what each
// of them is never told.

const torenFirst = { Toren: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 };
const pensantusFirst = { Pensantus: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 };

test(
  'o jogador move pelo círculo: o entulho custa 1,5 m a mais, a parede é recusada e depois ele salta',
  { tag: ['@MR-034', '@RN-21'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const { m, p, table, campaignId, done } = await movingTable(browser, 'Círculo', torenFirst, {
      character: { build: toren, sheet: torenSheet },
      paint: async (master, t) => {
        await paintRPC(master, t, 'MAP_LAYER_DIFFICULT_TERRAIN', 1, [[7, 7]]);
        await paintRPC(master, t, 'MAP_LAYER_WALL', 1, [[5, 5]]);
      },
    });
    try {
      await openSessionPage(p, campaignId);
      await openSessionPage(m, campaignId);
      await expect(p.getByRole('heading', { name: 'Sua vez, Toren' })).toBeVisible();
      await p.getByRole('button', { name: 'Mover', exact: true }).click();
      await expect(p.getByRole('heading', { name: 'Mover Toren' })).toBeVisible();
      await expect(p.getByText('Restam 9,0 m de 9,0 m (6 quadrados de 1,5 m). Toque num quadrado destacado.')).toBeVisible();

      // The legend names what the map has, never an object: the layers the server sent, then the reach.
      const legend = p.getByRole('list', { name: 'Legenda do mapa' });
      await expect(legend.getByText('Parede')).toBeVisible();
      await expect(legend.getByText('Terreno difícil')).toBeVisible();
      await expect(legend.getByText('Você alcança')).toBeVisible();
      await expect(legend.getByText('Alcance de 9,0 m')).toBeVisible();

      // Two squares straight are 3,0 m; with one square of rubble in the way, the destination, 1,5 m more.
      await tapSquare(p, 6, 7);
      await expect(p.getByText('Mover 1,5 m', { exact: true })).toBeVisible();
      await tapSquare(p, 7, 7);
      await expect(p.getByText('Mover 4,5 m', { exact: true })).toBeVisible();
      await expect(p.getByText('Depois restam 4,5 m.')).toBeVisible();
      // A wall square inside the circle is refused by the server's reason, naming nothing else.
      await tapSquare(p, 5, 5);
      await expect(p.getByRole('alert').filter({ hasText: 'Sem caminho reto' })).toContainText('Uma parede bloqueia esse caminho');
      await expect(p.getByRole('button', { name: 'Mover para cá' })).toHaveAttribute('aria-disabled', 'true');
      // Beyond the circle: too far, and it does not make up a number.
      await tapSquare(p, 19, 7);
      await expect(p.getByRole('alert').filter({ hasText: 'Longe demais' })).toContainText('custa mais do que os 9,0 m que você tem');

      await tapSquare(p, 7, 7);
      await p.getByRole('button', { name: 'Mover para cá' }).click();
      await expect(p.getByRole('heading', { name: 'Sua vez, Toren' })).toBeVisible();
      await expect.poll(async () => (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Toren')?.col).toBe(7);
      await expect(p.getByText('4,5 m de 9,0 m').first()).toBeVisible();

      // Saltar: the limits come from Força 17 (a human's +1), and the 10 ft on foot just before give the running start.
      await p.getByRole('button', { name: 'Mover', exact: true }).click();
      await pickRadio(p, 'Saltar');
      await expect(p.getByRole('heading', { name: 'Saltar Toren' })).toBeVisible();
      await expect(p.getByText('5,1 m com corrida · 2,6 m parado')).toBeVisible();
      await expect(p.getByText('1,8 m com corrida · 0,9 m parado')).toBeVisible();
      await expect(p.getByText('Você andou pelo menos 3,0 m a pé antes de saltar.')).toBeVisible();
      // Too far for the limit: the page does not offer it, and says the limit (the button is off).
      await tapSquare(p, 14, 7);
      await expect(p.getByRole('alert').filter({ hasText: 'Seu salto vai até 5,1 m' })).toBeVisible();
      await expect(p.getByRole('button', { name: 'Saltar para cá' })).toHaveAttribute('aria-disabled', 'true');
      // Inside the limit but beyond the movement left (4,5 m): says what it has.
      await tapSquare(p, 10, 8);
      await expect(p.getByRole('alert').filter({ hasText: 'Você só tem 4,5 m de movimento' })).toBeVisible();
      await tapSquare(p, 9, 7);
      await expect(p.getByText('Saltar 3,0 m', { exact: true })).toBeVisible();
      await p.getByRole('button', { name: 'Saltar para cá' }).click();
      await expect(p.getByRole('heading', { name: 'Sua vez, Toren' })).toBeVisible();
      await expect.poll(async () => (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Toren')?.col).toBe(9);
      await expect(p.getByText('Você saltou 3,0 m.')).toBeVisible();
      await expect(m.getByRole('log', { name: 'Registro do combate' })).toContainText('Toren');
      void table;
    } finally {
      await done();
    }
  },
);

test(
  'a cobertura sai do mapa: o jogador a vê na lista de alvos, a parede tira o alvo, e o mestre marca outra',
  { tag: ['@MR-034', '@RN-21'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    // Pensantus at (5, 7). The Capitão stands behind a half-cover square on the same row, Goblin 1 behind a wall in a column,
    // and Goblin 2 in the open.
    const { m, p, table, campaignId, done } = await movingTable(browser, 'Cobertura', pensantusFirst, {
      character: { sheet: pensantusCasting },
      at: { 'Capitão Goblin': [9, 7], 'Goblin 1': [5, 3], 'Goblin 2': [9, 11] },
      paint: async (master, t) => {
        await paintRPC(master, t, 'MAP_LAYER_COVER', 1, [[7, 7]]);
        await paintRPC(master, t, 'MAP_LAYER_WALL', 1, [[5, 5]]);
      },
    });
    try {
      await openSessionPage(p, campaignId);
      await openSessionPage(m, campaignId);
      await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();

      // The master's order says each enemy's cover against whoever has the turn, with the source, never an object.
      const order = m.getByRole('region', { name: 'Ordem de iniciativa' });
      await expect(order.getByText('Meia cobertura (do mapa) contra o Pensantus')).toBeVisible();
      await expect(order.getByText('Cobertura total (do mapa) contra o Pensantus')).toBeVisible();

      // The player's target list: the cover with its source, the walled goblin left out.
      await p.getByRole('button', { name: 'Atacar com Raio de Fogo' }).click();
      const sheet = p.getByRole('dialog', { name: 'Atacar com Raio de Fogo' });
      const capitao = sheet.locator('label', { hasText: 'Capitão Goblin' });
      await expect(capitao).toContainText('Meia cobertura (do mapa)');
      await expect(sheet.locator('label', { hasText: 'Goblin 2' })).not.toContainText('cobertura');
      await expect(sheet.locator('label', { hasText: 'Goblin 1' })).toHaveCount(0);
      await expect(sheet).not.toContainText('Cobertura total');
      await sheet.getByRole('button', { name: 'Fechar' }).click();

      // The master marks the Capitão's cover by hand: three quarters, and Goblin 2 total. Applies at once.
      await order.getByRole('button', { name: 'Marcar cobertura de Capitão Goblin' }).click();
      const mark = order.getByRole('radiogroup', { name: 'Cobertura marcada de Capitão Goblin' });
      await pickRadio(mark, 'Três quartos');
      // The master's list says it in the cover line (no pill): the mark is the larger cover now.
      await expect(order.getByText('Três quartos (marcada pelo mestre) contra o Pensantus')).toBeVisible();
      await order.getByRole('button', { name: 'Fechar' }).click();
      await expect(order.getByRole('button', { name: 'Marcar cobertura de Capitão Goblin' })).toBeFocused();
      await order.getByRole('button', { name: 'Mais ações para Goblin 2' }).click();
      await m.getByRole('menuitem', { name: 'Marcar cobertura…' }).click();
      await pickRadio(order.getByRole('radiogroup', { name: 'Cobertura marcada de Goblin 2' }), 'Cobertura total');
      await order.getByRole('button', { name: 'Fechar' }).click();

      // The player reads the mark as a tag, and Goblin 2 is off the list with the reason. The sheet
      // is a copy of the targets at the moment it opens, so it is opened again until the marks arrive.
      await expect(p.getByText('Três quartos · marcada pelo mestre')).toBeVisible();
      await expect(async () => {
        if (await p.getByRole('dialog').isVisible()) {
          await p.keyboard.press('Escape');
        }
        await p.getByRole('button', { name: 'Atacar com Raio de Fogo' }).click();
        const open = p.getByRole('dialog');
        await expect(open.locator('label', { hasText: 'Goblin 2' })).toContainText('Cobertura total (marcada pelo mestre): não pode ser alvo', { timeout: 2_000 });
      }).toPass({ timeout: 30_000 });
      const again = p.getByRole('dialog');
      await expect(again.locator('label', { hasText: 'Capitão Goblin' })).toContainText('Três quartos (marcada pelo mestre)');
      await expect(again.locator('label', { hasText: 'Goblin 2' }).locator('input')).toBeDisabled();
      // The player never gets an armor class, with or without cover.
      await expect(again).not.toContainText(/CA \d/);
      void table;
    } finally {
      await done();
    }
  },
);

const adjacent = { 'Capitão Goblin': [15, 3], 'Goblin 1': [6, 7], 'Goblin 2': [15, 11] } satisfies Record<string, [number, number]>;

test(
  'o goblin sai do alcance do Pensantus: o jogador responde ao aviso e ataca, e o mestre vê a espera e pode seguir sem esperar',
  { tag: ['@MR-034', '@RN-21'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const goblinFirst = { 'Goblin 1': 20, Pensantus: 15, 'Capitão Goblin': 10, 'Goblin 2': 4 };
    const { m, p, campaignId, done } = await movingTable(browser, 'Oportunidade do jogador', goblinFirst, {
      at: adjacent,
      character: { sheet: pensantusCasting },
    });
    try {
      await openSessionPage(p, campaignId);
      await openSessionPage(m, campaignId);
      await expect(m.getByText('Vez do Goblin 1')).toBeVisible();
      // The master drags the goblin on turn out of the reach: a move like any, which provokes.
      const start = await getEncounterRPC(m, campaignId);
      const g1 = start.combatants.find((c) => c.label === 'Goblin 1')!;
      await combatRPC(m, 'MoveCombatant', { campaignId, encounterId: start.id, combatantId: g1.id, col: 9, row: 7 });

      // The player's `alertdialog`, like Escudo's: the safe answer has the focus, and no armor class.
      const prompt = p.getByRole('alertdialog', { name: 'Ataque de oportunidade' });
      await expect(prompt.getByText('O Goblin 1 está saindo do seu alcance. Ataque de oportunidade?')).toBeVisible();
      await expect(prompt.getByText('Gasta a sua reação.')).toBeVisible();
      await expect(prompt.getByRole('button', { name: 'Não atacar' })).toBeFocused();
      await expect(prompt).not.toContainText(/CA \d/);
      // The answers are stacked at one width and one height, with the weapon's numbers above them.
      await expect(prompt.getByText('Espada longa').or(prompt.getByText('Adaga')).first()).toBeVisible();
      const no = await layoutSize(prompt.getByRole('button', { name: 'Não atacar' }));
      const yes = await layoutSize(prompt.getByRole('button', { name: 'Atacar com Adaga' }));
      expect(Math.abs(no.width - yes.width)).toBeLessThanOrEqual(1);
      expect(Math.abs(no.height - yes.height)).toBeLessThanOrEqual(1);
      // The master reads that the goblin's move waits for a player, and can go on without them.
      const card = m.getByRole('group', { name: 'Ataque de oportunidade de Pensantus' });
      await expect(card.getByText('Esperando a reação do jogador de Pensantus')).toBeVisible();
      await expect(m.getByText('Esperando a reação do jogador de Pensantus').first()).toBeVisible();

      // "Atacar com Adaga" hands over to the attack sheet with the goblin as the target.
      await prompt.getByRole('button', { name: 'Atacar com Adaga' }).click();
      const sheet = p.getByRole('dialog');
      await expect(sheet.getByText('Goblin 1').first()).toBeVisible();
      await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
      await sheet.getByLabel(/Role 1d20/).fill('1');
      await sheet.getByRole('button', { name: 'Confirmar 1' }).click();
      await expect(sheet.getByText('Sua reação foi usada.')).toBeVisible();
      await sheet.getByRole('button', { name: 'Fechar' }).last().click();
      await expect(m.getByRole('log', { name: 'Registro do combate' })).toContainText('Pensantus ataca o Goblin 1 com a Adaga (ataque de oportunidade): errou');
      await expect(card).toHaveCount(0);
    } finally {
      await done();
    }
  },
);

test(
  'o mestre pula a espera de um jogador que não responde, e o aviso do jogador diz que o mestre respondeu',
  { tag: ['@MR-034', '@RN-21'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const goblinFirst = { 'Goblin 1': 20, Pensantus: 15, 'Capitão Goblin': 10, 'Goblin 2': 4 };
    const { m, p, campaignId, done } = await movingTable(browser, 'Seguir sem esperar', goblinFirst, { at: adjacent });
    try {
      await openSessionPage(p, campaignId);
      await openSessionPage(m, campaignId);
      await expect(m.getByText('Vez do Goblin 1')).toBeVisible();
      const start = await getEncounterRPC(m, campaignId);
      await combatRPC(m, 'MoveCombatant', { campaignId, encounterId: start.id, combatantId: start.combatants.find((c) => c.label === 'Goblin 1')!.id, col: 9, row: 7 });
      const prompt = p.getByRole('alertdialog', { name: 'Ataque de oportunidade' });
      await expect(prompt).toBeVisible();
      const card = m.getByRole('group', { name: 'Ataque de oportunidade de Pensantus' });
      await expect(card.getByText('Se você seguir sem esperar, o Pensantus não ataca e continua com a reação.')).toBeVisible();
      await card.getByRole('button', { name: 'Seguir sem esperar' }).click();
      await expect(card).toHaveCount(0);
      // The player's prompt says the master answered, and only closes.
      await expect(prompt.getByText('O mestre respondeu por você')).toBeVisible();
      await prompt.getByRole('button', { name: 'Fechar' }).click();
      await expect(prompt).toHaveCount(0);
    } finally {
      await done();
    }
  },
);

test(
  'o Pensantus sai do alcance do goblin: o aviso antes de mover, a espera do turno e a resposta do mestre',
  { tag: ['@MR-034', '@RN-21'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const { m, p, campaignId, done } = await movingTable(browser, 'Oportunidade do mestre', pensantusFirst, { at: adjacent, character: { sheet: pensantusCasting } });
    try {
      await openSessionPage(p, campaignId);
      await openSessionPage(m, campaignId);
      await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
      await p.getByRole('button', { name: 'Mover', exact: true }).click();
      await tapSquare(p, 3, 7);
      // A warning, since the server says "pode": it names the goblin the player sees, and offers Desengajar.
      await expect(p.getByText('Sair do alcance do Goblin 1 pode provocar um ataque de oportunidade.')).toBeVisible();
      await expect(p.getByRole('button', { name: 'Desengajar (gasta a ação)' })).toBeVisible();
      await p.getByRole('button', { name: 'Mover para cá' }).click();

      // The move landed; the player's turn waits, and every action says why instead of failing on click.
      await expect(p.getByRole('status').filter({ hasText: 'Esperando a reação do mestre.' })).toContainText('O Goblin 1 pode fazer um ataque de oportunidade.');
      await expect(p.getByRole('button', { name: 'Mover', exact: true })).toHaveAttribute('aria-disabled', 'true');
      await expect(p.getByRole('button', { name: 'Atacar com Raio de Fogo' })).toHaveAttribute('aria-disabled', 'true');

      // The master's prompt: the safe answer has the focus; the turn does not pass meanwhile.
      const card = m.getByRole('group', { name: 'Ataque de oportunidade de Goblin 1' });
      await expect(card.getByText('O Pensantus saiu do alcance do Goblin 1.')).toBeVisible();
      await expect(card.getByText('Goblin 1 ataca o Pensantus?')).toBeVisible();
      await expect(card.getByText('Cimitarra +4 · 1d6 + 2 cortante · gasta a reação dele')).toBeVisible();
      await expect(card.getByRole('button', { name: 'Não atacar' })).toBeFocused();
      await expect(m.getByText('Esperando a sua reação: Goblin 1')).toBeVisible();
      await expect(m.getByRole('button', { name: 'Próximo turno' })).toHaveAttribute('aria-disabled', 'true');

      // "Atacar com Cimitarra": the master rolls it (typed here), and the wait ends with the answer.
      await card.getByRole('button', { name: 'Atacar com Cimitarra' }).click();
      const sheet = m.getByRole('dialog');
      await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
      await sheet.getByLabel(/Role 1d20/).fill('1');
      await sheet.getByRole('button', { name: 'Confirmar 1' }).click();
      await expect(sheet.getByText('A reação dele foi usada.')).toBeVisible();
      await sheet.getByRole('button', { name: 'Fechar' }).last().click();
      await expect(card).toHaveCount(0);
      await expect(p.getByText('Esperando a reação do mestre.')).toHaveCount(0);
      await expect(p.getByRole('button', { name: 'Mover', exact: true })).not.toHaveAttribute('aria-disabled', 'true');
      await expect(m.getByRole('log', { name: 'Registro do combate' })).toContainText('Goblin 1 ataca o Pensantus com a Cimitarra (ataque de oportunidade): errou');
    } finally {
      await done();
    }
  },
);

test(
  'o mestre diz "Não atacar" e a vez do jogador segue',
  { tag: ['@MR-034', '@RN-21'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const { m, p, campaignId, done } = await movingTable(browser, 'Não atacar', pensantusFirst, { at: adjacent });
    try {
      await openSessionPage(p, campaignId);
      await openSessionPage(m, campaignId);
      await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
      await p.getByRole('button', { name: 'Mover', exact: true }).click();
      await tapSquare(p, 3, 7);
      await p.getByRole('button', { name: 'Mover para cá' }).click();
      const card = m.getByRole('group', { name: 'Ataque de oportunidade de Goblin 1' });
      await card.getByRole('button', { name: 'Não atacar' }).click();
      await expect(card).toHaveCount(0);
      await expect(p.getByText('Esperando a reação do mestre.')).toHaveCount(0);
      await expect(p.getByRole('button', { name: 'Mover', exact: true })).not.toHaveAttribute('aria-disabled', 'true');
    } finally {
      await done();
    }
  },
);

test(
  'com Desengajar nenhum movimento do turno provoca: o aviso some e o turno diz que o jogador está desengajado',
  { tag: ['@MR-034', '@RN-21'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const { m, p, campaignId, done } = await movingTable(browser, 'Desengajar', pensantusFirst, { at: adjacent });
    try {
      await openSessionPage(p, campaignId);
      await openSessionPage(m, campaignId);
      await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
      await p.getByRole('button', { name: 'Mover', exact: true }).click();
      await tapSquare(p, 3, 7);
      await expect(p.getByText('Sair do alcance do Goblin 1 pode provocar um ataque de oportunidade.')).toBeVisible();
      await p.getByRole('button', { name: 'Desengajar (gasta a ação)' }).click();
      // The page stays open and reads the options again: nothing provokes now.
      await expect(p.getByText('Sair do alcance do Goblin 1')).toHaveCount(0);
      await expect(p.getByText('Mover 3,0 m', { exact: true })).toBeVisible();
      await p.getByRole('button', { name: 'Mover para cá' }).click();
      await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
      await expect(p.getByText('Você usou Desengajar. Seus movimentos deste turno não provocam ataque de oportunidade.')).toBeVisible();
      await expect(p.getByText('Desengajado').first()).toBeVisible();
      // No offer, no waiting: the master has no prompt, and the player's turn goes on.
      await expect(m.getByRole('group', { name: /Ataque de oportunidade/ })).toHaveCount(0);
      await expect(p.getByText('Esperando a reação')).toHaveCount(0);
      await expect(p.getByRole('button', { name: 'Mover', exact: true })).not.toHaveAttribute('aria-disabled', 'true');
    } finally {
      await done();
    }
  },
);

test(
  'um goblin escondido no caminho corta o movimento: o jogador lê só "algo bloqueou o caminho"',
  { tag: ['@MR-034', '@RN-10', '@RN-21'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const { m, p, campaignId, done } = await movingTable(browser, 'Movimento cortado', pensantusFirst, {
      at: { 'Capitão Goblin': [15, 3], 'Goblin 1': [7, 7], 'Goblin 2': [15, 11] },
      hidden: ['Goblin 1'],
    });
    try {
      await openSessionPage(p, campaignId);
      await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
      await p.getByRole('button', { name: 'Mover', exact: true }).click();
      await tapSquare(p, 9, 7);
      await expect(p.getByText('Mover 6,0 m', { exact: true })).toBeVisible();
      await p.getByRole('button', { name: 'Mover para cá' }).click();
      await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
      const note = p.getByRole('status').filter({ hasText: 'Você parou antes: algo bloqueou o caminho.' });
      await expect(note).toBeVisible();
      await expect(note).not.toContainText('Goblin');
      await expect.poll(async () => (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Pensantus')?.col).toBe(6);
      // The player never learned there was a goblin there.
      await expect(p.getByText('Goblin 1')).toHaveCount(0);
    } finally {
      await done();
    }
  },
);

test(
  'o mestre marca um NPC como aliado: a etiqueta aparece para todos',
  { tag: ['@MR-034', '@RN-21'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const { m, p, campaignId, done } = await movingTable(browser, 'Aliado', pensantusFirst, { at: adjacent });
    try {
      await openSessionPage(p, campaignId);
      await openSessionPage(m, campaignId);
      const order = m.getByRole('region', { name: 'Ordem de iniciativa' });
      await order.getByRole('button', { name: 'Mais ações para Goblin 2' }).click();
      await m.getByRole('menuitem', { name: 'Marcar como aliado' }).click();
      await expect(order.getByText('Aliado')).toBeVisible();
      await expect(order.getByRole('list', { name: 'Condições de Goblin 2' })).toHaveCount(0);
      await expect(p.getByText('Aliado').first()).toBeVisible();
      await order.getByRole('button', { name: 'Mais ações para Goblin 2' }).click();
      await m.getByRole('menuitem', { name: 'Voltar a ser inimigo' }).click();
      await expect(order.getByText('Aliado')).toHaveCount(0);
    } finally {
      await done();
    }
  },
);

test(
  'o jogador arrasta o token no mapa: a página "Mover" abre naquele quadrado, com os avisos, e nada anda sozinho',
  { tag: ['@MR-034', '@RN-21'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const { m, p, campaignId, done } = await movingTable(browser, 'Arrastar', pensantusFirst, { at: adjacent });
    try {
      await openSessionPage(p, campaignId);
      await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
      const map = p.getByRole('group', { name: /Mapa de batalha/ });
      const box = await boxOf(map);
      const token = p.locator('.cm__tk--movable').first();
      const from = await boxOf(token);
      await p.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
      await p.mouse.down();
      // Two squares to the left of the token (it starts on column 5): out of the goblin's reach.
      await p.mouse.move(box.x + (3.5 * box.width) / 20, box.y + (7.5 * box.height) / 14, { steps: 8 });
      await p.mouse.up();
      await expect(p.getByRole('heading', { name: 'Mover Pensantus' })).toBeVisible();
      await expect(p.getByText('Mover 3,0 m', { exact: true })).toBeVisible();
      await expect(p.getByText('Sair do alcance do Goblin 1 pode provocar um ataque de oportunidade.')).toBeVisible();
      // Nothing moved: the token is still where it was until "Mover para cá".
      expect((await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Pensantus')?.col).toBe(5);
      await p.getByRole('button', { name: 'Mover para cá' }).click();
      await expect.poll(async () => (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Pensantus')?.col).toBe(3);
    } finally {
      await done();
    }
  },
);
