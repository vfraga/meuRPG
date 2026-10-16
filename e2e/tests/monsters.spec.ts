import { expect, test, type Page } from '@playwright/test';

import { combatRPC, getEncounterRPC, startEncounterRPC, tableForCombat, type CombatTable, type Encounter } from './combat-support';
import { createPointRPC } from './maps-support';
import { callRPC, createCharacterRPC, newSignedInContext } from './support';
import { openSessionPage } from './live-session-support';

// MR-042 (monsters in the combat, "Pôr no combate"), MR-043 (the encounter builder: "Gerar encontro", "Trocar", the battle point,
// "Começar este combate"), RN-29 (the monsters of the bestiary), RN-20 (what a player never receives of a monster) and RN-10 (the builder
// and a saved encounter are the master's secret). Setup (campaign, player, map, session) goes through the API; every test makes its own
// campaign. The bestiary list itself is in bestiary.spec.ts.

/** The JSON of an encounter as a combatant row reads it, with the master-only fields (RN-29). */
type MonsterRow = Encounter['combatants'][number] & { challengeRating?: string; bestiaryCreatureKey?: string; hitPointsMax?: number; armorClass?: number };

const label = (c: { label: string }) => c.label;

/** The master's page on the bestiary row of a creature: its button "Pôr no combate: <nome>". */
async function openPutSheet(master: Page, campaignId: string, search: string, namePt: string) {
  await master.goto(`/campaigns/${campaignId}/bestiary`);
  await master.getByRole('searchbox', { name: 'Nome' }).fill(search);
  await expect(master.locator('.list__n')).toContainText('de 334 criaturas');
  await master.getByRole('button', { name: `Pôr no combate: ${namePt}` }).click();
  const sheet = master.getByRole('dialog', { name: 'Pôr no combate' });
  await expect(sheet).toBeVisible();
  return sheet;
}

/** Every combatant's d20 and the start of the combat, so the order and the turn exist. */
async function beginCombat(master: Page, table: CombatTable, enc: Encounter): Promise<Encounter> {
  let e = enc;
  for (const c of e.combatants) {
    e = await combatRPC(master, 'SubmitInitiative', { campaignId: table.campaignId, encounterId: e.id, combatantId: c.id, d20Face: 10 });
  }
  return combatRPC(master, 'BeginCombat', { campaignId: table.campaignId, encounterId: e.id });
}

test(
  'três Bandidos do bestiário entram num combate em preparação: nomes, PV médios, escondidos; o mestre lê o ND e o registro, o jogador só a palavra do estado e nada do escondido',
  { tag: ['@MR-042', '@RN-29', '@RN-20', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const master = await masterContext.newPage();
      const player = await playerContext.newPage();
      await master.goto('/');
      await player.goto('/');
      const table = await tableForCombat(master, player, `Mirathel ${Date.now()}`);
      const campaignId = table.campaignId;
      // A combat being set up, with the Goblin of the table in it.
      let enc = await startEncounterRPC(master, table, [{ characterId: table.goblinId, count: 1, hidden: false }]);

      // "Pôr no combate" from the bestiary: three, the average, hidden.
      const sheet = await openPutSheet(master, campaignId, 'bandit', 'Bandido');
      await expect(sheet.locator('.readonly')).toHaveText('Emboscada na estrada · em preparação');
      await expect(sheet.getByRole('switch', { name: 'Escondidos no início' })).toHaveAttribute('aria-checked', 'true');
      await expect(sheet.getByRole('radio', { name: 'Média (11)' })).toBeChecked();
      await sheet.getByRole('button', { name: 'Mais um Bandido' }).click();
      await sheet.getByRole('button', { name: 'Mais um Bandido' }).click();
      await expect(sheet.locator('.count__names')).toHaveText('Entram como Bandido 1, Bandido 2 e Bandido 3.');
      await sheet.getByRole('button', { name: 'Pôr 3 no combate' }).click();
      await expect(sheet).toBeHidden();
      const done = master.locator('.put-done');
      await expect(done).toContainText('Entraram no combate: Bandido 1, Bandido 2 e Bandido 3.');
      await expect(done).toContainText('Estão escondidos: só você os vê até revelar.');

      // The master reads the creature, the ND and the hit points of each (the average, 11), all hidden.
      enc = await getEncounterRPC(master, campaignId);
      const bandits = (enc.combatants as MonsterRow[]).filter((c) => c.label.startsWith('Bandido'));
      expect(bandits.map(label).sort()).toEqual(['Bandido 1', 'Bandido 2', 'Bandido 3']);
      for (const b of bandits) {
        expect(b.hidden).toBe(true);
        expect(b.hitPointsCurrent).toBe(11);
        expect(b.hitPointsMax).toBe(11);
        expect(b.challengeRating).toBe('1/8');
        expect(b.bestiaryCreatureKey).toBe('monster:bandit');
      }
      // The master's log says what was put in, with the hit points; round 0 is before the combat.
      const log = await callRPC(master, 'meurpg.play.v1.CombatService/ListCombatLog', { campaignId, encounterId: enc.id });
      expect(log.ok(), await log.text()).toBeTruthy();
      const masterLog = JSON.stringify(await log.json());
      expect(masterLog).toContain('COMBAT_LOG_KIND_MONSTERS_ADDED');
      expect(masterLog).toContain('Bandido 1');

      // The player knows nothing of them: not in the combat, not in the log (RN-20).
      const playerEnc = JSON.stringify(await getEncounterRPC(player, campaignId));
      for (const secret of ['Bandido', 'bandit', '1/8', 'bestiaryCreatureKey', 'challengeRating']) {
        expect(playerEnc, `o jogador não recebe "${secret}"`).not.toContain(secret);
      }
      const playerLog = await callRPC(player, 'meurpg.play.v1.CombatService/ListCombatLog', { campaignId, encounterId: enc.id });
      const playerLogText = await playerLog.text();
      expect(playerLogText).not.toContain('MONSTERS_ADDED');
      expect(playerLogText).not.toContain('Bandido');

      // The combat begins; the master reveals Bandido 2 only.
      enc = await beginCombat(master, table, await getEncounterRPC(master, campaignId));
      const second = enc.combatants.find((c) => c.label === 'Bandido 2')!;
      await combatRPC(master, 'SetCombatantHidden', { campaignId, encounterId: enc.id, combatantId: second.id, hidden: false });

      // The master's order: the ND beside the armor class, the hidden one marked.
      await openSessionPage(master, campaignId);
      const order = master.getByRole('region', { name: 'Ordem de iniciativa' });
      await expect(order.locator('.row', { hasText: 'Bandido 3' })).toContainText('ND 1/8');
      await expect(order.locator('.row', { hasText: 'Bandido 3' })).toContainText('Escondido');
      await expect(order.locator('.row', { hasText: 'Bandido 2' })).toContainText('Revelado');

      // The player's JSON: Bandido 2 by its name and the word of its state, no numbers; nothing at all of Bandido 1 and 3.
      const revealed = await getEncounterRPC(player, campaignId);
      const seen = revealed.combatants.find((c) => c.label === 'Bandido 2') as MonsterRow | undefined;
      expect(seen?.state).toBe('COMBATANT_STATE_UNHURT');
      expect(seen?.hitPointsCurrent).toBeUndefined();
      expect(seen?.hitPointsMax).toBeUndefined();
      expect(seen?.armorClass).toBeUndefined();
      expect(seen?.challengeRating).toBeUndefined();
      expect(seen?.bestiaryCreatureKey).toBeUndefined();
      const revealedText = JSON.stringify(revealed);
      expect(revealedText).not.toContain('Bandido 1');
      expect(revealedText).not.toContain('Bandido 3');
      expect(revealedText).not.toContain('bandit');

      // The player's screen: the word of the state, and nothing of the one still hidden.
      await openSessionPage(player, campaignId);
      await expect(player.getByText('Bandido 2').first()).toBeVisible();
      await expect(player.getByText('Ileso').first()).toBeVisible();
      await expect(player.getByText('Bandido 1')).toHaveCount(0);
      await expect(player.getByText('Bandido 3')).toHaveCount(0);
      await expect(player.getByText('1/8')).toHaveCount(0);
    } finally {
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'o combate em andamento recebe monstros rolados e revelados, e o XP por inimigos conta o ND de cada Bandido: 3 × 25 = 75',
  { tag: ['@MR-042', '@RN-29'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const master = await masterContext.newPage();
      const player = await playerContext.newPage();
      await master.goto('/');
      await player.goto('/');
      const table = await tableForCombat(master, player, `Bandidos ${Date.now()}`);
      const campaignId = table.campaignId;
      let enc = await startEncounterRPC(master, table, [{ characterId: table.goblinId, count: 1, hidden: false }]);
      enc = await beginCombat(master, table, enc);

      // Into a combat under way, rolled and revealed (the sheet's variants).
      const sheet = await openPutSheet(master, campaignId, 'bandit', 'Bandido');
      await expect(sheet.locator('.readonly')).toHaveText('Emboscada na estrada · em andamento');
      await sheet.getByRole('button', { name: 'Mais um Bandido' }).click();
      await sheet.getByRole('button', { name: 'Mais um Bandido' }).click();
      await sheet.locator('.seg__item', { hasText: 'Rolar' }).click();
      await sheet.getByRole('switch', { name: 'Escondidos no início' }).click();
      await sheet.getByRole('button', { name: 'Pôr 3 no combate' }).click();
      await expect(master.locator('.put-done')).toContainText('Os jogadores já os veem pelo estado.');
      enc = await getEncounterRPC(master, campaignId);
      const bandits = (enc.combatants as MonsterRow[]).filter((c) => c.label.startsWith('Bandido'));
      expect(bandits).toHaveLength(3);
      for (const b of bandits) {
        expect(b.hidden).toBeFalsy();
        // Rolled 2d8 + 2: between 4 and 18.
        expect(b.hitPointsMax).toBeGreaterThanOrEqual(4);
        expect(b.hitPointsMax).toBeLessThanOrEqual(18);
      }
      // The rolled dice are in the master's log line.
      const log = await callRPC(master, 'meurpg.play.v1.CombatService/ListCombatLog', { campaignId, encounterId: enc.id });
      const logText = JSON.stringify(await log.json());
      expect(logText).toContain('COMBAT_LOG_KIND_MONSTERS_ADDED');
      expect(logText).toContain('"rolled":true');

      // They fall, the master ends the combat, and the XP counts each Bandido's ND.
      for (const b of bandits) {
        enc = await combatRPC(master, 'AdjustCombatantHitPoints', { campaignId, encounterId: enc.id, combatantId: b.id, damage: 999 });
      }
      const goblin = enc.combatants.find((c) => c.label === 'Goblin')!;
      await combatRPC(master, 'AdjustCombatantHitPoints', { campaignId, encounterId: enc.id, combatantId: goblin.id, damage: 999 });
      await combatRPC(master, 'EndEncounter', { campaignId, encounterId: enc.id });

      await master.goto(`/campaigns/${campaignId}/session`);
      await expect(master.getByRole('heading', { name: 'Combate encerrado' })).toBeVisible();
      const block = master.getByRole('region', { name: 'Experiência do combate' });
      const kind = block.locator('.kind', { hasText: 'Bandido 1 a 3' });
      await expect(kind).toContainText('ND 1/8 · 25 XP cada');
      await expect(kind.locator('.kind__n')).toHaveText('75 XP');
      // The Goblin of the table is worth its own XP too: the total is the sum of the rows.
      await expect(block).toContainText('Total dos 4 derrotados');
    } finally {
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'o montador de encontros mede o encontro contra o grupo, com os números do servidor; um NPC no grupo muda o orçamento; "Gerar encontro" repete com a mesma semente e "Trocar" mantém o XP',
  { tag: ['@MR-043', '@RN-29'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const master = await masterContext.newPage();
      const player = await playerContext.newPage();
      await master.goto('/');
      await player.goto('/');
      const table = await tableForCombat(master, player, `Mirathel ${Date.now()}`);
      const campaignId = table.campaignId;
      const evaluate = async (entries: { creatureKey: string; count: number }[], extraParty: object[] = []) => {
        const res = await callRPC(master, 'meurpg.play.v1.EncounterService/EvaluateEncounter', { campaignId, entries, extraParty });
        expect(res.ok(), await res.text()).toBeTruthy();
        return (await res.json()).evaluation as { budget: { low: number; moderate: number; high: number }; totalXp: number; band: string; maxCr: string; lowestLevel: number };
      };
      const word: Record<string, string> = { ENCOUNTER_BAND_LOW: 'Baixa', ENCOUNTER_BAND_MODERATE: 'Moderada', ENCOUNTER_BAND_HIGH: 'Alta', ENCOUNTER_BAND_ABOVE_HIGH: 'Acima de alta' };
      const nf = (n: number) => n.toLocaleString('pt-BR');

      await master.goto(`/campaigns/${campaignId}/encounters`);
      await expect(master.getByRole('heading', { name: 'Encontros', level: 1 })).toBeVisible();
      await expect(master.locator('.chip', { hasText: 'Pensantus' })).toBeVisible();
      await expect(master.getByText('Guia de dificuldade do SRD 5.2.1 (regras de 2024)').first()).toBeVisible();
      await expect(master.getByText('Com os monstros de 2014, o encontro tende a ficar um pouco mais fácil.').first()).toBeVisible();
      await expect(master.locator('.guide a').first()).toHaveAttribute('href', '/credits');

      // Four Goblins by the search: the band and the numbers are the server's.
      const add = async (search: string, namePt: string) => {
        const box = master.getByRole('combobox', { name: 'Adicionar criatura' });
        await box.fill(search);
        await master.getByRole('option').filter({ has: master.locator('.pick__pt', { hasText: new RegExp(`^${namePt}$`) }) }).first().click();
      };
      await add('goblin', 'Goblin');
      for (let i = 0; i < 3; i++) {
        await master.getByRole('button', { name: 'Mais um Goblin' }).click();
      }
      const four = await evaluate([{ creatureKey: 'monster:goblin', count: 4 }]);
      expect(four.totalXp).toBe(200);
      const limit = { ENCOUNTER_BAND_LOW: four.budget.low, ENCOUNTER_BAND_MODERATE: four.budget.moderate, ENCOUNTER_BAND_HIGH: four.budget.high, ENCOUNTER_BAND_ABOVE_HIGH: four.budget.high }[four.band]!;
      await expect(master.locator('.head-xp')).toHaveText(
        four.band === 'ENCOUNTER_BAND_ABOVE_HIGH' ? `Acima de alta: 200 de ${nf(limit)} XP` : `${word[four.band]} · 200 de ${nf(limit)} XP`,
      );
      await expect(master.locator('.band--on')).toContainText(word[four.band]);
      await expect(master.locator('.total__n')).toHaveText('200 XP');
      await expect(master.getByText('mortal')).toHaveCount(0);

      // An NPC in the party with a level changes the budget, asked of the server.
      const orin = await createCharacterRPC(master, campaignId, {
        kind: 'CHARACTER_KIND_STORY',
        name: 'Orin, o guia',
        sheet: { basic: { hitPointsMax: 9, armorClass: 10, speedFt: 25, attackBonus: 0, damage: '1d4', description: '' } },
      });
      expect(orin.ok(), await orin.text()).toBeTruthy();
      const orinId = (await orin.json()).character.id as string;
      await master.getByRole('button', { name: 'Pôr um NPC no grupo' }).click();
      const npc = master.getByRole('dialog', { name: 'Pôr um NPC no grupo' });
      await npc.getByRole('radio', { name: /Orin, o guia/ }).check({ force: true });
      const withOrin = await evaluate([{ creatureKey: 'monster:goblin', count: 4 }], [{ characterId: orinId, name: '', level: 3 }]);
      await expect(npc.locator('.budget__n')).toContainText(`Alta ${nf(withOrin.budget.high)}`);
      await npc.getByRole('button', { name: 'Pôr no grupo' }).click();
      await expect(master.locator('.chip', { hasText: 'Orin, o guia' })).toContainText('NPC · nível 3');
      await expect(master.locator('.bar__budget').last()).toHaveText(nf(withOrin.budget.high));
      await master.getByRole('button', { name: 'Tirar Orin, o guia do grupo' }).click();
      await expect(master.locator('.chip', { hasText: 'Orin, o guia' })).toHaveCount(0);

      // "Gerar encontro": Moderada, Humanoide. The seed on screen gives the same encounter by the API, twice.
      await master.getByRole('button', { name: 'Gerar encontro' }).click();
      const gen = master.getByRole('dialog', { name: 'Gerar encontro' });
      // Changing the type draws again while the first draw is still on screen: wait for the humanoid draw to land and
      // the result to stop being busy, or the seed and the lines can come from two different draws.
      const humanoidDrawn = master.waitForResponse(
        (r) => r.url().includes('/meurpg.play.v1.EncounterService/GenerateEncounter') && (r.request().postData() ?? '').includes('humanoid'),
      );
      await gen.locator('select[name=type]').selectOption('humanoid');
      await humanoidDrawn;
      await expect(gen.locator('section.res')).toHaveAttribute('aria-busy', 'false');
      await expect(gen.locator('.res__seed')).toContainText('Semente');
      const seed = Number((await gen.locator('.res__seed').innerText()).replace(/\D/g, ''));
      expect(seed).toBeGreaterThan(0);
      const shown = (await gen.locator('.line__pt').allInnerTexts()).map((t) => t.trim());
      const drawn = async () => {
        const res = await callRPC(master, 'meurpg.play.v1.EncounterService/GenerateEncounter', { campaignId, band: 'ENCOUNTER_BAND_MODERATE', creatureType: 'humanoid', seed });
        expect(res.ok(), await res.text()).toBeTruthy();
        return (await res.json()) as { seed: number; evaluation: { lines: { count: number; creature: { namePt: string } }[]; totalXp: number } };
      };
      const first = await drawn();
      const second = await drawn();
      expect(second).toEqual(first);
      expect(first.evaluation.lines.map((l) => `${l.count} × ${l.creature.namePt}`)).toEqual(shown);

      // "Gerar outro" draws a new seed.
      await gen.getByRole('button', { name: 'Gerar outro' }).click();
      await expect(gen.locator('.res__seed')).not.toHaveText(`Semente ${seed}`);

      // "Trocar criatura": the same count of another creature with the same XP; the total does not move.
      const before = await gen.locator('.head').innerText();
      const line = gen.locator('.line').first();
      const was = (await line.locator('.line__pt').innerText()).trim();
      const count = was.split(' × ')[0];
      await line.getByRole('button', { name: /^Trocar criatura/ }).click();
      const swap = gen.locator('.swap');
      await expect(swap.locator('.swap__t')).toHaveText(`Trocar ${was}`);
      const options = swap.locator('.swap__opt');
      // The options come from the server after the panel opens: wait for the first one, never count too early.
      await expect(options.first()).toBeVisible();
      const chosen = (await options.first().locator('.swap__n').innerText()).trim();
      await options.first().click();
      await swap.getByRole('button', { name: `Trocar por ${chosen}` }).click();
      await expect(gen.locator('.swap')).toHaveCount(0);
      await expect(gen.locator('.line').first().locator('.line__pt')).toHaveText(`${count} × ${chosen}`);
      await expect(gen.locator('.head')).toHaveText(before);
      await expect(gen.locator('.res__seed')).toHaveText('Trocado à mão');

      // "Usar este encontro" puts it in the builder.
      await gen.getByRole('button', { name: 'Usar este encontro' }).click();
      await expect(gen).toBeHidden();
      await expect(master.locator('.row').first().locator('.row__pt')).toHaveText(chosen);
      await expect(master.locator('.head-xp')).toContainText(' XP');
      await expect(master.getByText('Semente')).toHaveCount(0);
    } finally {
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'o encontro guardado num ponto de batalha: "Guardar" pergunta antes de trocar, "Começar este combate" põe os monstros, e o jogador nunca recebe o encontro guardado',
  { tag: ['@MR-043', '@RN-29', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const master = await masterContext.newPage();
      const player = await playerContext.newPage();
      await master.goto('/');
      await player.goto('/');
      const table = await tableForCombat(master, player, `Mirathel ${Date.now()}`);
      const campaignId = table.campaignId;
      const pointId = await createPointRPC(master, campaignId, table.mapId, { kind: 'BATTLE', name: 'Emboscada na ponte', xBp: 3000, yBp: 3000, revealed: true });
      await createPointRPC(master, campaignId, table.mapId, { kind: 'BATTLE', name: 'Ruínas do forte', xBp: 7000, yBp: 3000, revealed: true });

      // The builder: two Goblins and a Bandido, kept on the point.
      await master.goto(`/campaigns/${campaignId}/encounters`);
      const add = async (search: string, namePt: string) => {
        await master.getByRole('combobox', { name: 'Adicionar criatura' }).fill(search);
        await master.getByRole('option').filter({ has: master.locator('.pick__pt', { hasText: new RegExp(`^${namePt}$`) }) }).first().click();
      };
      await add('goblin', 'Goblin');
      await master.getByRole('button', { name: 'Mais um Goblin' }).click();
      await add('bandit', 'Bandido');
      await expect(master.locator('.lines__n')).toHaveText('3 criaturas');
      await expect(master.locator('.total__n')).toHaveText('125 XP');
      await master.getByRole('button', { name: 'Guardar no ponto de batalha' }).click();
      const save = master.getByRole('dialog', { name: 'Guardar no ponto de batalha' });
      await expect(save.locator('select[name=map]')).toHaveValue(table.mapId);
      await expect(save.locator('.pt')).toHaveCount(2);
      await expect(save.locator('.pt', { hasText: 'Emboscada na ponte' })).toContainText('Sem encontro ainda');
      await expect(save.locator('.priv')).toContainText('Só você vê');
      await save.locator('.pt', { hasText: 'Emboscada na ponte' }).click();
      await save.getByRole('button', { name: 'Guardar no ponto de batalha' }).click();
      await expect(save).toBeHidden();
      await expect(master.locator('.mr-notice--success')).toContainText('Encontro guardado em “Emboscada na ponte”.');

      // Keeping again on the same point asks first, in place.
      await master.getByRole('button', { name: 'Mais um Bandido' }).click();
      await master.getByRole('button', { name: 'Guardar no ponto de batalha' }).click();
      const again = master.getByRole('dialog', { name: 'Guardar no ponto de batalha' });
      await expect(again.locator('.pt', { hasText: 'Emboscada na ponte' })).toContainText('Já guarda um encontro (3 criaturas)');
      await again.locator('.pt', { hasText: 'Emboscada na ponte' }).click();
      await again.getByRole('button', { name: 'Guardar no ponto de batalha' }).click();
      await expect(again.getByText('Trocar o encontro guardado?')).toBeVisible();
      await expect(again.getByRole('button', { name: 'Voltar' })).toBeFocused();
      await again.getByRole('button', { name: 'Voltar' }).click();
      await expect(again.getByText('Trocar o encontro guardado?')).toHaveCount(0);
      await again.getByRole('button', { name: 'Cancelar' }).click();
      await expect(again).toHaveCount(0);
      // Nothing was replaced: the point still keeps the three creatures.
      const kept = await callRPC(master, 'meurpg.play.v1.EncounterService/GetBattleEncounter', { campaignId, mapPointId: pointId });
      expect(JSON.stringify(await kept.json())).toContain('"count":2');

      // The session: the card of the point, and "Começar este combate" opens "Iniciar combate" filled.
      await openSessionPage(master, campaignId);
      const card = master.locator('.enc', { hasText: 'Emboscada na ponte' });
      await expect(card.locator('.enc__pill')).toContainText('Encontro guardado');
      await expect(card.locator('.enc__band')).toContainText(' XP');
      await expect(card.locator('.enc__guide')).toContainText('Guia de dificuldade do SRD 5.2.1 (regras de 2024)');
      await expect(card.locator('.enc__row')).toHaveCount(2);
      await card.getByRole('button', { name: 'Começar este combate' }).click();
      const start = master.getByRole('dialog', { name: 'Iniciar combate' });
      await expect(start.getByLabel('Nome do combate')).toHaveValue('Emboscada na ponte');
      await expect(start.locator('.mon__row')).toHaveCount(2);
      await expect(start.getByRole('switch', { name: 'Escondidos no início' })).toHaveAttribute('aria-checked', 'true');
      // From a battle point on a map with a grid the dialog opens on "Com mapa".
      await expect(start.getByRole('radio', { name: /^Com mapa/ })).toBeChecked();
      await start.getByRole('button', { name: 'Iniciar combate' }).click();
      await expect(start).toBeHidden();

      // The monsters are in the combat: numbered, hidden, from the creature, and the combat knows its point.
      const enc = await getEncounterRPC(master, campaignId);
      const names = (enc.combatants as MonsterRow[]).filter((c) => c.bestiaryCreatureKey).map(label).sort();
      expect(names).toEqual(['Bandido', 'Goblin 1', 'Goblin 2']);
      expect((enc.combatants as MonsterRow[]).filter((c) => c.bestiaryCreatureKey).every((c) => c.hidden)).toBe(true);
      expect((enc as unknown as { mapPointId?: string }).mapPointId).toBe(pointId);

      // A player never gets the builder or the saved encounter (RN-10): not found, and the point carries no trace of it.
      for (const [method, body] of [
        ['GetBattleEncounter', { campaignId, mapPointId: pointId }],
        ['ListBattleEncounters', { campaignId, mapId: table.mapId }],
        ['EvaluateEncounter', { campaignId, entries: [] }],
        ['ListEncounterSwaps', { campaignId, creatureKey: 'monster:goblin' }],
        ['ClearBattleEncounter', { campaignId, mapPointId: pointId }],
        ['GenerateEncounter', { campaignId, band: 'ENCOUNTER_BAND_MODERATE' }],
        ['SaveBattleEncounter', { campaignId, mapPointId: pointId, encounter: { monsters: [{ creatureKey: 'monster:goblin', count: 1 }] } }],
      ] as const) {
        const res = await callRPC(player, `meurpg.play.v1.EncounterService/${method}`, body);
        expect(res.status(), `${method} para o jogador`).toBe(404);
      }
      const map = await callRPC(player, 'meurpg.maps.v1.MapService/GetMap', { campaignId, mapId: table.mapId });
      expect(map.ok(), await map.text()).toBeTruthy();
      const mapText = JSON.stringify(await map.json());
      expect(mapText).toContain('Emboscada na ponte');
      expect(mapText.toLowerCase()).not.toContain('encounter');
      expect(mapText).not.toContain('goblin');
      // The page tells a player it is the master's.
      await player.goto(`/campaigns/${campaignId}/encounters`);
      await expect(player.getByText('Só o mestre monta encontros.')).toBeVisible();
    } finally {
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'o montador abre o encontro guardado do ponto, tira o encontro do ponto, e "Começar este combate" também vale "Sem mapa"',
  { tag: ['@MR-043', '@RN-29', '@RN-25'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const master = await masterContext.newPage();
      const player = await playerContext.newPage();
      await master.goto('/');
      await player.goto('/');
      const table = await tableForCombat(master, player, `Mirathel ${Date.now()}`);
      const campaignId = table.campaignId;
      const pointId = await createPointRPC(master, campaignId, table.mapId, { kind: 'BATTLE', name: 'Ruínas do forte', xBp: 7000, yBp: 3000, revealed: true });
      const saved = await callRPC(master, 'meurpg.play.v1.EncounterService/SaveBattleEncounter', {
        campaignId,
        mapPointId: pointId,
        encounter: { monsters: [{ creatureKey: 'monster:goblin', count: 2 }, { creatureKey: 'monster:bandit', count: 1 }] },
      });
      expect(saved.ok(), await saved.text()).toBeTruthy();

      // The editor's link brings the point's encounter into the builder, where it can be taken off the point (asked in place).
      await master.goto(`/campaigns/${campaignId}/encounters?map=${table.mapId}&point=${pointId}`);
      await expect(master.locator('.lines__n')).toHaveText('3 criaturas');
      await master.getByRole('button', { name: 'Tirar o encontro do ponto' }).click();
      await expect(master.getByText('Tirar o encontro do ponto?')).toBeVisible();
      await expect(master.getByRole('button', { name: 'Voltar' })).toBeFocused();
      await master.getByRole('button', { name: 'Voltar' }).click();
      await expect(master.getByText('Tirar o encontro do ponto?')).toHaveCount(0);
      const stillThere = await callRPC(master, 'meurpg.play.v1.EncounterService/GetBattleEncounter', { campaignId, mapPointId: pointId });
      expect(JSON.stringify(await stillThere.json())).toContain('monster:goblin');
      await master.getByRole('button', { name: 'Tirar o encontro do ponto' }).click();
      await master.getByRole('button', { name: 'Tirar o encontro' }).click();
      await expect(master.locator('.mr-notice--success')).toContainText('O ponto não guarda mais um encontro.');
      const gone = await callRPC(master, 'meurpg.play.v1.EncounterService/GetBattleEncounter', { campaignId, mapPointId: pointId });
      expect(JSON.stringify(await gone.json())).not.toContain('monster:goblin');

      // Put it back and start it "Sem mapa": the monsters come in with no squares and the combat has no point (RN-25).
      const again = await callRPC(master, 'meurpg.play.v1.EncounterService/SaveBattleEncounter', {
        campaignId,
        mapPointId: pointId,
        encounter: { monsters: [{ creatureKey: 'monster:goblin', count: 2 }] },
      });
      expect(again.ok(), await again.text()).toBeTruthy();
      await openSessionPage(master, campaignId);
      await master.locator('.enc', { hasText: 'Ruínas do forte' }).getByRole('button', { name: 'Começar este combate' }).click();
      const start = master.getByRole('dialog', { name: 'Iniciar combate' });
      await start.getByText('Sem mapa (teatro da mente)', { exact: true }).click();
      await start.getByRole('button', { name: 'Iniciar combate' }).click();
      await expect(start).toBeHidden();
      const enc = await getEncounterRPC(master, campaignId);
      expect((enc as unknown as { mode?: string }).mode).toBe('ENCOUNTER_MODE_THEATRE');
      expect((enc.combatants as MonsterRow[]).filter((c) => c.bestiaryCreatureKey).map(label).sort()).toEqual(['Goblin 1', 'Goblin 2']);
      expect((enc.combatants as MonsterRow[]).every((c) => !c.placed)).toBe(true);
      expect((enc as unknown as { mapPointId?: string }).mapPointId ?? '').toBe('');
    } finally {
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'sem combate aberto, "Pôr no combate" pergunta Com mapa ou Sem mapa, e "Iniciar combate" põe um monstro do bestiário sem sair do diálogo',
  { tag: ['@MR-042', '@MR-013', '@RN-25'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const master = await masterContext.newPage();
      const player = await playerContext.newPage();
      await master.goto('/');
      await player.goto('/');
      const table = await tableForCombat(master, player, `Mirathel ${Date.now()}`);
      const campaignId = table.campaignId;

      // From the bestiary: the map has a grid, so "Com mapa" is the default; the master picks the theatre of the mind.
      const sheet = await openPutSheet(master, campaignId, 'bandit', 'Bandido');
      await expect(sheet.getByRole('radio', { name: /Com mapa/ })).toBeChecked();
      await sheet.getByText('Sem mapa (teatro da mente)', { exact: true }).click();
      await expect(sheet.getByTestId('theatre-why')).toBeVisible();
      await sheet.getByRole('button', { name: 'Criar o combate e pôr' }).click();
      await expect(sheet).toBeHidden();
      const started = await getEncounterRPC(master, campaignId);
      expect((started as unknown as { mode?: string }).mode).toBe('ENCOUNTER_MODE_THEATRE');
      expect((started.combatants as MonsterRow[]).filter((c) => c.bestiaryCreatureKey).map(label)).toEqual(['Bandido']);
      await combatRPC(master, 'EndEncounter', { campaignId, encounterId: started.id });

      // From "Iniciar combate": the creature is searched and counted inside the dialog, which stays open.
      await openSessionPage(master, campaignId);
      await master.getByRole('button', { name: 'Voltar à sessão' }).click();
      await master.getByRole('button', { name: 'Iniciar combate' }).click();
      const dialog = master.getByRole('dialog', { name: 'Iniciar combate' });
      await expect(dialog.getByRole('link', { name: /bestiário/i })).toHaveCount(0);
      const pickBox = dialog.getByRole('combobox', { name: 'Adicionar criatura' });
      await pickBox.click();
      await pickBox.pressSequentially('bandit');
      await dialog.getByRole('option', { name: /Bandido/ }).first().click();
      await dialog.getByRole('button', { name: 'Mais um Bandido' }).click();
      await expect(dialog.locator('.pick__row')).toContainText('Bandido');
      await expect(dialog).toBeVisible();
      await dialog.getByText('Sem mapa (teatro da mente)', { exact: true }).click();
      await dialog.getByRole('button', { name: 'Iniciar combate' }).click();
      await expect(dialog).toBeHidden();
      const enc = await getEncounterRPC(master, campaignId);
      expect((enc as unknown as { mode?: string }).mode).toBe('ENCOUNTER_MODE_THEATRE');
      expect((enc.combatants as MonsterRow[]).filter((c) => c.bestiaryCreatureKey).map(label).sort()).toEqual(['Bandido 1', 'Bandido 2']);
    } finally {
      await masterContext.close();
      await playerContext.close();
    }
  },
);
