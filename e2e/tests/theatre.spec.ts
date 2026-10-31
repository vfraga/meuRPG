import { expect, test } from '@playwright/test';

import { type Encounter, adjustVitalsRPC, brisa, brisaSheet, combatRPC, getEncounterRPC, passTurnsTo } from './combat-support';
import { openSessionPage } from './live-session-support';
import { required } from './support';
import { setTableRulesRPC } from './table-rules-support';
import { beginTheatreRPC, secondPlayer, startTheatreRPC, theatreTable } from './theatre-support';
import { masterLog, openOrderRow } from './combat-layout';
import { waitForEncounterRevision } from './revision-support';

// Combat without a map on screen (Etapa 10, slice 10.13b, MR-025, RN-25, RN-24, RN-20, ADR-0017): the start dialog's mode, the
// master's and the player's screens with no grid, movement by number, the opportunity attack the master offers, the critical
// hint and the death saves the table hides. The table and the combat come through the API; what is under test is the screens
// and what each audience reads.

test(
  'um combate sem mapa de ponta a ponta: o mestre inicia, Toren gasta 6,0 m e vê que restam 3,0 m, ataca sem distância, e o mestre oferece o ataque de oportunidade',
  { tag: ['@MR-025', '@RN-25', '@RN-20', '@RN-24'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const t = await theatreTable(browser, 'Teatro da mente');
    const { m, p, table, campaignId } = t;
    try {
      // The table's rule "combate com mapa" off: the dialog offers "Sem mapa" first, and says why once.
      await setTableRulesRPC(m, campaignId, { combatStartsWithMap: false });
      await openSessionPage(m, campaignId);
      await m.getByRole('button', { name: 'Iniciar combate' }).click();
      const dialog = m.getByRole('dialog', { name: 'Iniciar combate' });
      await expect(dialog.getByRole('radio', { name: /Sem mapa \(teatro da mente\)/ })).toBeChecked();
      await expect(dialog.getByTestId('theatre-why')).toContainText('Sem mapa, o app não sabe onde ninguém está.');
      // No map to show and no grid to ask for.
      await expect(dialog.getByText('Mapa do combate')).toHaveCount(0);
      await expect(dialog.getByText('Esse mapa ainda não tem grade.')).toHaveCount(0);
      // "Com mapa" brings the map's part back, and the choice is the master's.
      await dialog.locator('label', { hasText: 'Com mapa' }).first().click();
      await expect(dialog.getByText('Mapa do combate')).toBeVisible();
      await dialog.locator('label', { hasText: 'Sem mapa' }).first().click();
      await dialog.getByRole('button', { name: 'Mais um Goblin', exact: true }).click();
      await dialog.getByRole('button', { name: 'Mais um Capitão Goblin' }).click();
      await dialog.getByRole('button', { name: 'Iniciar combate' }).click();
      await expect(m.getByText('Os NPCs rolaram sozinhos.')).toBeVisible();
      const started = await getEncounterRPC(m, campaignId);
      expect((started as { mode?: string }).mode).toBe('ENCOUNTER_MODE_THEATRE');
      // Nobody has a square: no "Sem quadrado no mapa" warning, no map card.
      await expect(m.getByText('Sem quadrado no mapa.')).toHaveCount(0);
      await expect(m.getByRole('button', { name: 'Ver mapa' })).toHaveCount(0);

      // The dialog starts the NPCs hidden (question 31): the master reveals them, as he would on screen.
      for (const c of started.combatants.filter((x) => x.kind === 'COMBATANT_KIND_NPC')) {
        await combatRPC(m, 'SetCombatantHidden', { campaignId, encounterId: started.id, combatantId: c.id, hidden: false });
      }
      await openSessionPage(p, campaignId);
      const begun = await beginTheatreRPC(m, table, started, { Toren: 20, 'Goblin': 12, 'Capitão Goblin': 10 });
      await waitForEncounterRevision([m, p], begun);

      // The master: the pill, the card of whoever is on turn with the movement he may spend, the cover panel, no map.
      await expect(m.getByText('Vez do Toren', { exact: true }).first()).toBeVisible();
      await expect(m.locator('app-theatre-pill')).toContainText('Teatro da mente');
      await expect(m.getByRole('heading', { name: 'Vez do Toren' })).toBeVisible();
      await expect(m.getByRole('heading', { name: 'Cobertura dos alvos' })).toBeVisible();
      await expect(m.getByText('Sem mapa: só o número')).toBeVisible();

      // Toren's phone: the panel in the map's place, "Gastar movimento" and not "Mover".
      await expect(p.getByRole('heading', { name: 'Combate sem mapa' })).toBeVisible();
      await expect(p.getByText('Sem mapa, o app não sabe onde ninguém está.')).toBeVisible();
      await expect(p.getByRole('button', { name: 'Mover', exact: true })).toHaveCount(0);
      await expect(p.getByRole('button', { name: 'Ver mapa' })).toHaveCount(0);
      await p.getByRole('button', { name: 'Gastar movimento' }).first().click();
      const sheet = p.getByRole('dialog', { name: 'Gastar movimento' }).or(p.getByRole('heading', { name: 'Gastar movimento' }));
      await expect(sheet.first()).toBeVisible();
      await expect(p.getByText('Você tem 9,0 m neste turno')).toBeVisible();
      // The steps are 1,5 m; the minus waits at one step.
      await expect(p.getByRole('button', { name: 'Menos 1,5 m' })).toHaveAttribute('aria-disabled', 'true');
      for (let i = 0; i < 3; i++) {
        await p.getByRole('button', { name: 'Mais 1,5 m' }).click();
      }
      await expect(p.getByLabel('Quanto gastar: 6,0 m')).toBeVisible();
      await expect(p.getByText('Depois restam')).toBeVisible();
      await expect(p.locator('.sum__row--after dd')).toHaveText('3,0 m');
      await p.getByRole('button', { name: 'Gastar 6,0 m' }).click();
      // The walker keeps the count of the metres left, with no map to show it on (MR-025).
      await expect(p.getByRole('listitem', { name: /^Movimento: 3,0\sm$/ }).first()).toBeVisible();
      // The server's number, as the JSON says it: 6 m = 20 ft = 200 tenths of a foot spent, no square.
      const after = await getEncounterRPC(p, campaignId);
      const toren = after.combatants.find((c) => c.label === 'Toren') as unknown as { movementUsedDft: number; movementLeftDft: number; placed?: boolean };
      expect(toren.movementUsedDft).toBe(200);
      expect(toren.movementLeftDft).toBe(100);
      expect(toren.placed ?? false).toBe(false);
      await expect(await masterLog(m)).toContainText('Toren gastou 6,0 m de movimento');
      await expect(await masterLog(m)).toContainText('sem mapa (teatro da mente)');

      // The attack: every enemy in sight is a target, with no distance and no "Longe demais".
      await p.getByRole('button', { name: 'Atacar com Espada longa' }).click();
      const attack = p.getByRole('dialog', { name: /Atacar com Espada longa/ }).or(p.getByRole('heading', { name: /Atacar com Espada longa/ }));
      await expect(attack.first()).toBeVisible();
      await expect(p.getByText('Escolha o alvo (quem você vê)')).toBeVisible();
      await expect(p.getByText('O mestre decide quem está ao alcance.')).toBeVisible();
      await expect(p.getByText('Longe demais')).toHaveCount(0);
      await expect(p.getByText(/\ba \d+,\d m\b/)).toHaveCount(0);
      await p.getByRole('radiogroup').getByText('Capitão Goblin', { exact: true }).click();
      await p.getByRole('button', { name: 'Rolar no app' }).click();
      await expect(p.getByText(/Acertou|Errou|Crítico/).first()).toBeVisible();
      await p.keyboard.press('Escape');

      // The master marks cover: four rows, and the word reaches the attacker's list.
      await m.getByRole('button', { name: 'Mudar a cobertura de Capitão Goblin' }).click();
      await expect(m.getByRole('heading', { name: 'Cobertura do Capitão Goblin' })).toBeVisible();
      await expect(m.getByRole('radio')).toHaveCount(4);
      await m.locator('label', { hasText: 'Meia cobertura' }).click();
      await m.getByRole('button', { name: 'Salvar cobertura' }).click();
      await expect(m.getByText('Meia cobertura: +2 na CA')).toBeVisible();

      // Toren's turn ends; on the Goblin's turn the master offers the opportunity attack to Toren.
      await combatRPC(m, 'EndTurn', { campaignId, encounterId: started.id, expectedCombatantId: (await getEncounterRPC(m, campaignId)).currentCombatantId, discardPendingDamage: true });
      await waitForEncounterRevision([m, p], await passTurnsTo(m, campaignId, 'Goblin'));
      await expect(m.getByRole('heading', { name: 'Vez do Goblin' })).toBeVisible();
      await m.getByRole('button', { name: 'Oferecer ataque de oportunidade' }).click();
      await expect(m.getByRole('heading', { name: 'Oferecer ataque de oportunidade' })).toBeVisible();
      await expect(m.getByText('Saiu do alcance de', { exact: true })).toBeVisible();
      await m.getByRole('button', { name: 'Oferecer a Toren' }).click();
      // The player's question, with no square and no armor class; the master's wait, with its two ways out.
      const prompt = p.getByRole('alertdialog', { name: 'Ataque de oportunidade' });
      await expect(prompt).toBeVisible();
      await expect(prompt).toContainText('O Goblin está saindo do seu alcance. Ataque de oportunidade?');
      await expect(m.getByRole('status').filter({ hasText: 'Esperando a resposta do' })).toBeVisible();
      await expect(await masterLog(m)).toContainText('Goblin saiu do alcance de Toren');
      // Withdrawn by mistake: the question leaves the phone, and nobody lost a reaction.
      await m.getByRole('button', { name: 'Retirar a oferta' }).click();
      await expect(prompt.getByText('retirou a oferta')).toBeVisible();
      await prompt.getByRole('button', { name: 'Fechar' }).click();
      await expect(prompt).toHaveCount(0);
      const withdrawn = await getEncounterRPC(m, campaignId);
      expect((withdrawn.combatants.find((c) => c.label === 'Toren') as unknown as { reactionUsed?: boolean }).reactionUsed ?? false).toBe(false);
      // Offered again, this time the player answers: "Não atacar".
      await m.getByRole('button', { name: 'Oferecer ataque de oportunidade' }).click();
      await m.getByRole('button', { name: 'Oferecer a Toren' }).click();
      await expect(prompt).toBeVisible();
      await prompt.getByRole('button', { name: 'Não atacar' }).click();
      await expect(prompt).toHaveCount(0);
      await expect(m.getByRole('status').filter({ hasText: 'Esperando a resposta do' })).toHaveCount(0);
    } finally {
      await t.done();
    }
  },
);

test(
  'o teste contra a morte escondido: o outro jogador lê só "Caída" e o aviso do registro, e o dono e o mestre veem as marcas',
  { tag: ['@RN-24', '@RN-20', '@MR-025'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const t = await theatreTable(browser, 'Mortes escondidas');
    const { m, p, table, campaignId } = t;
    const lia = await secondPlayer(browser, m, campaignId, brisa, brisaSheet);
    try {
      await setTableRulesRPC(m, campaignId, { combatStartsWithMap: false, deathSaves: 'DEATH_SAVE_VISIBILITY_OWNER_AND_MASTER' });
      const enc0 = await startTheatreRPC(m, table, [{ characterId: table.goblinId, count: 1, hidden: false }]);
      await beginTheatreRPC(m, table, enc0, { Brisa: 20, Toren: 15, 'Goblin': 5 });
      await openSessionPage(m, campaignId);
      await openSessionPage(p, campaignId);
      await openSessionPage(lia.page, campaignId);
      // Brisa falls with her turn on: she rolls her death saves.
      await adjustVitalsRPC(m, campaignId, lia.characterId, { hitPointsCurrent: 0 });
      let enc = await getEncounterRPC(m, campaignId);
      enc = await combatRPC(m, 'EndTurn', { campaignId, encounterId: enc.id, expectedCombatantId: enc.currentCombatantId });
      enc = await passTurnsTo(m, campaignId, 'Brisa');
      await waitForEncounterRevision([lia.page, p], enc);
      await lia.page.getByRole('button', { name: 'Digitar o resultado' }).click();
      await lia.page.getByLabel(/Role 1d20 para o teste contra a morte/).fill('4');
      await lia.page.getByRole('button', { name: 'Confirmar 4' }).click();
      await expect(lia.page.getByText('1 de 3').first()).toBeVisible();
      // The owner: the marks and the words that say who sees them.
      await expect(lia.page.getByTestId('death-private')).toContainText('Só você e o mestre');
      // The master: the same marks, with who else sees them.
      await openOrderRow(m, 'Brisa');
      await expect(m.getByText('Dono e mestre').first()).toBeVisible();
      // The other player: the state in words, no marks, no counts, and the reason in the log.
      await expect(p.getByText('Caída').first()).toBeVisible();
      await expect(p.getByText(/Sucessos|Falhas/)).toHaveCount(0);
      await expect(p.getByText(/\d de 3/)).toHaveCount(0);
      // The log (a sheet on the phone) says why her rolls are not in it, and has none of them.
      await p.getByRole('button', { name: 'Abrir o registro do combate' }).click();
      await expect(p.getByText('Esta mesa só deixa o dono e o mestre verem os testes contra a morte.')).toBeVisible();
      await expect(p.getByRole('log', { name: 'Registro do combate' })).not.toContainText('teste contra a morte');
      await p.keyboard.press('Escape');
      // What the server sent that player is the state word alone, as the app's JSON.
      const seen = (await getEncounterRPC(p, campaignId)).combatants.find((c) => c.label === 'Brisa')!;
      expect(seen.state).toBe('COMBATANT_STATE_DOWN');
      expect(seen.deathSuccesses ?? 0).toBe(0);
      expect(seen.deathFailures ?? 0).toBe(0);
      const own = (await getEncounterRPC(lia.page, campaignId)).combatants.find((c) => c.label === 'Brisa')!;
      expect(own.deathFailures ?? 0).toBe(1);

      // Three successes on her next turns make her stable: the result is public, the road to it is not.
      let rolled: Encounter | undefined;
      for (let i = 0; i < 3; i++) {
        const now = await getEncounterRPC(m, campaignId);
        await combatRPC(m, 'EndTurn', { campaignId, encounterId: now.id, expectedCombatantId: now.currentCombatantId, discardPendingDamage: true });
        await passTurnsTo(m, campaignId, 'Brisa');
        const turn = await getEncounterRPC(m, campaignId);
        rolled = await combatRPC(lia.page, 'RollDeathSave', { campaignId, encounterId: turn.id, combatantId: turn.combatants.find((c) => c.label === 'Brisa')!.id, d20Face: 15 });
      }
      await waitForEncounterRevision(p, required(rolled, 'the death save the loop rolled'));
      const stable = (await getEncounterRPC(p, campaignId)).combatants.find((c) => c.label === 'Brisa')!;
      expect(stable.state).toBe('COMBATANT_STATE_STABLE');
      await p.getByRole('button', { name: 'Abrir o registro do combate' }).click();
      // From a real server log: "Brisa estabilizou", with no roll, no outcome and no counts.
      await expect(p.getByRole('log', { name: 'Registro do combate' })).toContainText('Brisa estabilizou');
      await expect(p.getByRole('log', { name: 'Registro do combate' })).not.toContainText('teste contra a morte');
      await p.keyboard.press('Escape');
    } finally {
      await lia.close();
      await t.done();
    }
  },
);

test(
  'o crítico "máximo mais uma rolagem" com dados físicos: a dica diz o que rolar e o máximo aparece como parte fixa',
  { tag: ['@RN-24', '@RN-18', '@MR-025'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const t = await theatreTable(browser, 'Crítico');
    const { m, p, table, campaignId } = t;
    try {
      await setTableRulesRPC(m, campaignId, {
        combatStartsWithMap: false,
        diceMode: 'DICE_MODE_PHYSICAL',
        critical: 'CRITICAL_RULE_MAX_PLUS_ROLL',
      });
      const enc0 = await startTheatreRPC(m, table, [{ characterId: table.captainId, count: 1, hidden: false }]);
      await beginTheatreRPC(m, table, enc0, { Toren: 20, 'Capitão Goblin': 5 });
      await openSessionPage(p, campaignId);
      await p.getByRole('button', { name: 'Atacar com Espada longa' }).click();
      await p.getByRole('radiogroup').getByText('Capitão Goblin', { exact: true }).click();
      // Physical dice: the d20 is typed; a natural 20 is a critical hit.
      await p.getByLabel(/Role 1d20 para Espada longa/).fill('20');
      await p.getByRole('button', { name: /^Confirmar/ }).click();
      // This table is called "Crítico" too, and the session bar's back link says so (not on a phone): the first one shown is the roll's.
      await expect(p.getByText('Crítico').filter({ visible: true }).first()).toBeVisible();
      // The hint says what to roll under the table's rule, with the maximum as the fixed part (1d8: 8), and the browser does no dice math.
      await expect(p.getByText(/Acerto crítico: o máximo mais uma rolagem\. O máximo dos dados \(8\) já vale sem rolar; role 1d8 uma vez\./)).toBeVisible();
      await expect(p.getByText('Role 1d8 e digite só o que saiu, de 1 a 8. O app soma o resto.')).toBeVisible();
      await p.getByLabel(/Role 1d8 para o dano/).fill('5');
      // The total shown before it is sent is the one the server records: 5 typed + 8 (the critical's maximum) + 3 (the modifier) = 16.
      await expect(p.getByText('+ 8 do crítico + 3 de modificador')).toBeVisible();
      await expect(p.getByText('5 + 11 = 16')).toBeVisible();
      await p.getByRole('button', { name: /^Confirmar/ }).click();
      await expect(p.getByText('5 + 11 = 16 de dano cortante · dado físico')).toBeVisible();
    } finally {
      await t.done();
    }
  },
);
