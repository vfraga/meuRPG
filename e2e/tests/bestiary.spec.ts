import { expect, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { canvasPng, createMapRPC, placeTokenRPC, revealMapRPC, tableForMaps, uploadImageRPC } from './maps-support';
import {
  callRPC,
  gotoCampaignTab,
  newSignedInContext,
} from './support';

// MR-042 (the bestiary and "Criar NPC", slice 10.17a), RN-04 (a player never sees the NPCs) and RN-10
// (an NPC's token is born hidden). Setup (campaign, player, map) goes through the API; every test makes
// its own campaign. "Pôr no combate" (slice 10.17b) is in monsters.spec.ts.

/** What the server says `ListCreatures` finds for a search: the count the page must show. */
async function serverCount(page: Page, campaignId: string, query: string): Promise<{ total: number; names: string[] }> {
  const res = await callRPC(page, 'meurpg.rules.v1.ContentService/ListCreatures', { campaignId, query, pageSize: 400 });
  expect(res.ok(), await res.text()).toBeTruthy();
  const body = await res.json();
  return { total: body.total as number, names: ((body.creatures ?? []) as { namePt: string }[]).map((c) => c.namePt) };
}

test(
  'o mestre procura "lobo", abre o Ogro, cria o "Capitão bandido" como Minion, e o NPC chega com os números do Ogro e o token escondido; o jogador não tem o bestiário',
  { tag: ['@MR-042', '@RN-04', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const master = await masterContext.newPage();
      const player = await playerContext.newPage();
      await master.goto('/');
      await player.goto('/');
      const table = await tableForMaps(master, player, `Mirathel ${Date.now()}`);
      const campaignId = table.campaignId;

      // The way in: the campaign page has the panel, for the master.
      await gotoCampaignTab(master, campaignId, 'Preparação');
      await master.getByRole('link', { name: /^Bestiário/ }).click();
      await expect(master.getByRole('heading', { name: 'Bestiário', level: 1 })).toBeVisible();
      await expect(master.locator('.mr-page-lead')).toContainText('334 criaturas do SRD 5.1 · Mirathel');
      await expect(master.locator('.list__n')).toHaveText('334 de 334 criaturas');
      // Every row has "Pôr no combate" beside the link that opens the stat block (slice 10.17b).
      await expect(master.getByRole('button', { name: /^Pôr no combate/ })).toHaveCount(334);

      // "lobo": the count is the server's, the rows say the Portuguese and the SRD's names.
      const lobo = await serverCount(master, campaignId, 'lobo');
      expect(lobo.total).toBe(5);
      await master.getByRole('searchbox', { name: 'Nome' }).fill('lobo');
      await expect(master.locator('.list__n')).toHaveText(`${lobo.total} de 334 criaturas`);
      await expect(master.locator('.row__pt')).toHaveText(lobo.names);
      const wolf = master.locator('.row', { hasText: 'Wolf · SRD' }).first();
      await expect(wolf.locator('.row__pt')).toHaveText('Lobo');
      await expect(wolf.locator('.row__kind')).toHaveText('Fera · Médio');
      await expect(wolf.locator('.row__nd')).toHaveText('ND 1/4');
      await expect(master.getByText('A busca vale para o nome em português e para o nome do SRD, em inglês.')).toBeVisible();
      // The SRD's name finds the Werewolf's three forms too.
      const wolfSearch = await serverCount(master, campaignId, 'wolf');
      expect(wolfSearch.total).toBe(7);
      await master.getByRole('searchbox', { name: 'Nome' }).fill('wolf');
      await expect(master.locator('.list__n')).toHaveText('7 de 334 criaturas');

      // An empty search says so, with the typed word, and gives the way out (the SRD has "Wyrmling", so "wyrm" is not empty).
      await master.getByRole('searchbox', { name: 'Nome' }).fill('xyzzy');
      await expect(master.getByText('Nenhuma criatura com “xyzzy”.')).toBeVisible();
      await expect(master.locator('.list__n')).toHaveText('0 de 334 criaturas');
      await master.getByRole('button', { name: 'Limpar a busca' }).click();
      await expect(master.locator('.list__n')).toHaveText('334 de 334 criaturas');

      // The filters: size and ND go to the server.
      await master.locator('select[name=size]').selectOption('huge');
      await master.locator('select[name=cr]').selectOption('1-4');
      const huge = await callRPC(master, 'meurpg.rules.v1.ContentService/ListCreatures', { campaignId, size: 'CREATURE_SIZE_HUGE', minCr: '1', maxCr: '4', pageSize: 400 });
      const hugeTotal = (await huge.json()).total as number;
      await expect(master.locator('.list__n')).toHaveText(`${hugeTotal} de 334 criaturas`);
      await master.getByRole('button', { name: 'Limpar filtros' }).click();

      // The Ogre: the stat block, the SRD's text in English.
      await master.getByRole('searchbox', { name: 'Nome' }).fill('ogro');
      // The app's result, not a race with it: wait for the server's count of the search. (A tap before the
      // typing pause ends is covered by the Vitest spec of the list.)
      const ogro = await serverCount(master, campaignId, 'ogro');
      await expect(master.locator('.list__n')).toHaveText(`${ogro.total} de 334 criaturas`);
      await master.locator('.row', { hasText: 'Ogre · SRD' }).first().locator('.row__link').click();
      await expect(master.getByRole('heading', { name: 'Ogro', level: 1 })).toBeVisible();
      // Each number in its own tile, with the note that says where it comes from.
      const tile = (label: string) => master.locator('.tile').filter({ has: master.locator('.tile__l', { hasText: new RegExp(`^${label}$`) }) });
      await expect(tile('CA').locator('.tile__v')).toHaveText('11');
      await expect(tile('CA').locator('.tile__n')).toHaveText('gibão de peles');
      await expect(tile('PV').locator('.tile__v')).toHaveText('59');
      await expect(tile('Deslocamento').locator('.tile__v')).toHaveText('12 m');
      await expect(tile('Deslocamento').locator('.tile__n')).toHaveText('40 pés');
      await expect(tile('Nível de desafio').locator('.tile__v')).toHaveText('2');
      await expect(tile('Nível de desafio').locator('.tile__n')).toHaveText('450 XP');
      await expect(master.getByText('Os textos abaixo são do livro de regras (SRD 5.1), em inglês.')).toBeVisible();
      await expect(master.locator('.entry[lang=en]', { hasText: 'Greatclub' })).toBeVisible();
      await expect(master.getByRole('button', { name: 'Pôr no combate' })).toHaveCount(1);
      // Back keeps the search.
      await master.getByRole('link', { name: 'Voltar ao Bestiário' }).first().click();
      await expect(master.getByRole('searchbox', { name: 'Nome' })).toHaveValue('ogro');
      await master.locator('.row', { hasText: 'Ogre · SRD' }).first().locator('.row__link').click();

      // "Criar NPC": the name, Minion, the numbers that come from the creature.
      await master.getByRole('button', { name: 'Criar NPC' }).click();
      const dialog = master.getByRole('dialog', { name: 'Criar NPC' });
      await expect(dialog.getByLabel('Nome do NPC')).toHaveValue('Ogro');
      await expect(dialog.getByRole('radio')).toHaveCount(2);
      await expect(dialog.getByRole('radio', { name: 'Minion' })).toBeChecked();
      await expect(dialog.getByText('NPC de história', { exact: true })).toBeVisible();
      await expect(dialog.locator('.num', { hasText: 'Força' })).toContainText('19');
      await dialog.getByLabel('Nome do NPC').fill('Capitão bandido');
      await dialog.getByRole('button', { name: 'Criar NPC' }).click();
      await expect(dialog).toBeHidden();
      const done = master.locator('.made');
      await expect(done).toContainText('NPC criado: Capitão bandido. Já está na lista de NPCs.');
      // The confirmation lists the attacks the NPC really got.
      await expect(done).toContainText('Ataques da ficha: Clava grande e Azagaia.');

      // The NPC is in the campaign's list with the creature's numbers (a Minion, a basic sheet).
      await done.getByRole('link', { name: 'Abrir a ficha' }).click();
      await expect(master.getByRole('heading', { name: 'Capitão bandido', level: 1 })).toBeVisible();
      // The numbers in their places on the sheet: the armor class, the speed and the hit points.
      await expect(master.locator('.shield', { hasText: 'Classe de Armadura' }).locator('.shield__number')).toHaveText('11');
      await expect(master.locator('.box--speed .box__value')).toContainText('12 m');
      await expect(master.locator('.hp', { hasText: 'Pontos de vida máximos' }).locator('.hp__value')).toHaveText('59');
      await expect(master.locator('.attacks__name')).toHaveText([/^Clava grande /, /^Azagaia /]);
      const npcId = master.url().split('/').pop()!;
      const got = await callRPC(master, 'meurpg.characters.v1.CharacterService/GetCharacter', { campaignId, characterId: npcId });
      const npc = (await got.json()).character;
      expect(npc.kind).toBe('CHARACTER_KIND_MINION');
      expect(npc.sheet.basic).toMatchObject({ hitPointsMax: 59, armorClass: 11, speedFt: 40, monsterKey: 'monster:ogre' });
      expect(npc.sheet.basic.abilityScores).toMatchObject({ strength: 19, constitution: 16 });
      // The dialog's line and the stat block's `npcAttackNames` say what the NPC got (one function on the server).
      expect((npc.sheet.basic.attacks as { name: string }[]).map((a) => a.name)).toEqual(['Clava grande', 'Azagaia']);
      const block = await callRPC(master, 'meurpg.rules.v1.ContentService/GetCreature', { campaignId, key: 'monster:ogre' });
      expect((await block.json()).creature.npcAttackNames).toEqual(['Clava grande', 'Azagaia']);
      await gotoCampaignTab(master, campaignId, 'Preparação');
      const npcs = master.getByRole('region', { name: 'NPCs' });
      await expect(npcs.getByText('Capitão bandido')).toBeVisible();
      await expect(npcs.getByText('Minion')).toBeVisible();

      // RN-10: its token is born hidden, and the player's map never carries it.
      const image = await uploadImageRPC(master, campaignId, 'Ponte', await canvasPng(master, 1200, 800, 'Ponte'));
      const mapId = await createMapRPC(master, campaignId, 'Ponte de Mirathel', image);
      await revealMapRPC(master, campaignId, mapId);
      await placeTokenRPC(master, campaignId, mapId, npcId, 5000, 5000);
      const asMaster = await (await callRPC(master, 'meurpg.maps.v1.MapService/GetMap', { campaignId, mapId })).json();
      expect(asMaster.tokens).toHaveLength(1);
      expect(asMaster.tokens[0].hidden).toBe(true);
      const asPlayer = await (await callRPC(player, 'meurpg.maps.v1.MapService/GetMap', { campaignId, mapId })).json();
      expect(asPlayer.tokens ?? []).toHaveLength(0);

      // RN-04: the player has no entry point and no NPC list; the page says it is the master's.
      await player.goto(`/campaigns/${campaignId}`);
      await expect(player.getByRole('heading', { name: 'Personagens', level: 3 }).first()).toBeVisible();
      await expect(player.getByText('Bestiário')).toHaveCount(0);
      await expect(player.getByRole('link', { name: /bestiário/i })).toHaveCount(0);
      await expect(player.getByText('Capitão bandido')).toHaveCount(0);
      await expect(player.getByRole('heading', { name: 'NPCs' })).toHaveCount(0);
      await player.goto(`/campaigns/${campaignId}/bestiary`);
      await expect(player.getByText('Só o mestre usa o bestiário da campanha.')).toBeVisible();
      await expect(player.getByRole('searchbox')).toHaveCount(0);
      const list = await callRPC(player, 'meurpg.characters.v1.CharacterService/ListCharacters', { campaignId });
      expect(JSON.stringify(await list.json())).not.toContain('Capitão bandido');
      const make = await callRPC(player, 'meurpg.characters.v1.CharacterService/CreateNpcFromCreature', {
        campaignId,
        creatureKey: 'monster:ogre',
        name: 'Intruso',
        kind: 'CHARACTER_KIND_MINION',
        idempotencyKey: crypto.randomUUID(),
      });
      expect(make.status()).toBe(403);
    } finally {
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'dar o mesmo toque duas vezes em "Criar NPC" faz um NPC só, e o "NPC de história" também sai da ficha do bestiário',
  { tag: ['@MR-042'] },
  async ({ browser }) => {
    test.setTimeout(120_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const master = await masterContext.newPage();
      const player = await playerContext.newPage();
      await master.goto('/');
      await player.goto('/');
      const table = await tableForMaps(master, player, `Bestiário dois toques ${Date.now()}`);
      await master.goto(`/campaigns/${table.campaignId}/bestiary/goblin`);
      await expect(master.getByRole('heading', { name: 'Goblin', level: 1 })).toBeVisible();
      await master.getByRole('button', { name: 'Criar NPC' }).click();
      const dialog = master.getByRole('dialog', { name: 'Criar NPC' });
      await dialog.getByLabel('Nome do NPC').fill('Vigia goblin');
      await dialog.getByText('NPC de história', { exact: true }).click();
      // Every request the dialog sends carries one key: count them, and see that the server made one NPC.
      const keys: string[] = [];
      await master.route('**/meurpg.characters.v1.CharacterService/CreateNpcFromCreature', (route) => {
        keys.push(JSON.parse(route.request().postData() ?? '{}').idempotencyKey);
        return route.continue();
      });
      const go = dialog.getByRole('button', { name: 'Criar NPC' });
      await go.dblclick();
      await expect(master.locator('.made')).toContainText('NPC criado: Vigia goblin.');
      expect(keys.length).toBeGreaterThanOrEqual(1);
      expect(new Set(keys).size).toBe(1);
      const list = await callRPC(master, 'meurpg.characters.v1.CharacterService/ListCharacters', { campaignId: table.campaignId });
      const made = ((await list.json()).characters as { name: string; kind: string }[]).filter((c) => c.name === 'Vigia goblin');
      expect(made).toHaveLength(1);
      expect(made[0].kind).toBe('CHARACTER_KIND_STORY');
      // The same key sent again, by hand, answers that NPC and makes no second one.
      const again = await callRPC(master, 'meurpg.characters.v1.CharacterService/CreateNpcFromCreature', {
        campaignId: table.campaignId, creatureKey: 'monster:goblin', name: 'Vigia goblin', kind: 'CHARACTER_KIND_STORY', idempotencyKey: keys[0],
      });
      expect(again.ok()).toBeTruthy();
      const after = await callRPC(master, 'meurpg.characters.v1.CharacterService/ListCharacters', { campaignId: table.campaignId });
      expect(((await after.json()).characters as { name: string }[]).filter((c) => c.name === 'Vigia goblin')).toHaveLength(1);
    } finally {
      await masterContext.close();
      await playerContext.close();
    }
  },
);
