import { expect, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { layersOf, mapToPaint } from './editor-support';
import { campaignWithEmptyPlayer, factor, masterCampaign, method, setTableRulesRPC, tableRulesOf, wallSquares } from './table-rules-support';
import { callRPC, characterRpcBody, createCharacterRPC, newSignedInContext, pensantus, fillChoicesStep } from './support';
import { endOpenSessionRPC } from './live-session-support';
import { passClassStep, tableForLevelUp, toren } from './levelup-support';
import { pickRadio } from './move-support';
import { awardXpRPC, tableForXp } from './xp-support';

test.describe.configure({ timeout: 120_000 });

// MR-025 (RN-24, RN-25, RN-09): the master's "Regras da mesa", the ways a player makes the scores of a new sheet,
// and the grid calibration of a map. Every test makes its own campaign through the API.

/** Opens a `mat-select` by its label and picks an option (keyboard, as `support.ts` does). */
async function pick(page: Page, label: string, option: string): Promise<void> {
  const control = page.getByRole('combobox', { name: label, exact: true });
  await control.focus();
  await control.press('Enter');
  await page.getByRole('option', { name: option, exact: true }).click();
  await expect(control).toHaveAttribute('aria-expanded', 'false');
}

/** The "Básico" step of a player who needs no skills: a human barbarian called `name`. */
async function fillBasics(page: Page, name: string): Promise<void> {
  await page.getByLabel('Nome do personagem', { exact: true }).fill(name);
  await pick(page, 'Raça', 'Humano');
  await pick(page, 'Classe', 'Bárbaro');
  await pick(page, 'Antecedente', 'Acólito');
}

test(
  'a mesa física preenche os três estilos de uma vez e as escolhas ficam salvas @RN-24',
  { tag: '@RN-24' },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    try {
      const page = await master.newPage();
      await page.goto('/');
      const campaignId = await masterCampaign(page, `Regras ${Date.now()}`);
      await page.goto(`/campaigns/${campaignId}/rules`);
      await expect(page.getByRole('heading', { level: 1, name: 'Regras da mesa' })).toBeVisible();
      await expect(page.getByText('Tudo salvo. Nenhuma mudança para salvar.')).toBeVisible();
      // The defaults are not one of the styles: dice are the players' choice, combat has a map, fog off.
      await expect(page.getByRole('radio', { name: /Personalizado/ })).toBeChecked();

      await pickRadio(page, /Mesa física/);
      await expect(page.getByText('Mesa física: o estilo preencheu 2 escolhas abaixo; ainda não foi salvo.')).toBeVisible();
      await expect(page.getByRole('radio', { name: /Começar sem mapa/ })).toBeChecked();
      await expect(page.getByRole('radio', { name: /Todos rolam os próprios dados/ })).toBeChecked();
      await expect(page.getByText('2 mudanças não salvas')).toBeVisible();
      // A hand edit takes the style back to "Personalizado".
      await pickRadio(page, /Começar com mapa/);
      await expect(page.getByText('Personalizado. Suas escolhas não batem mais com nenhum estilo.')).toBeVisible();
      await pickRadio(page, /Mesa física/);

      await pickRadio(page, /A média/);
      await page.getByRole('button', { name: 'Adicionar um lembrete' }).click();
      await page.getByLabel('Lembrete 1', { exact: true }).fill('Beber uma poção é uma ação bônus');
      await page.getByRole('button', { name: 'Salvar regras' }).click();
      await expect(page.getByText(/Regras salvas\./)).toBeVisible();

      await page.reload();
      await expect(page.getByRole('radio', { name: /Mesa física/ })).toBeChecked();
      await expect(page.getByRole('radio', { name: /Começar sem mapa/ })).toBeChecked();
      await expect(page.getByRole('radio', { name: /A média/ })).toBeChecked();
      await expect(page.getByRole('switch', { name: 'Névoa de guerra nos mapas novos' })).toHaveAttribute('aria-checked', 'false');
      await expect(page.getByLabel('Lembrete 1', { exact: true })).toHaveValue('Beber uma poção é uma ação bônus');
      // The server holds the same, and works the style out itself.
      const saved = await tableRulesOf(page, campaignId);
      expect(saved.style).toBe('TABLE_STYLE_MESA_FISICA');
      expect(saved.rules).toMatchObject({ diceMode: 'DICE_MODE_PHYSICAL', hitPoints: 'HIT_POINTS_RULE_AVERAGE', houseRules: ['Beber uma poção é uma ação bônus'] });
    } finally {
      await master.close();
    }
  },
);

test(
  'o jogador faz as habilidades por compra de pontos, e o servidor guarda o jeito @RN-24',
  { tag: '@RN-24' },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Pontos ${Date.now()}`);

      await p.goto(`/campaigns/${campaignId}/characters/new`);
      await fillBasics(p, 'Ícaro');
      await p.getByRole('tab', { name: 'Habilidades' }).click();
      await expect(p.getByRole('radio', { name: 'Padrão' })).toBeChecked();
      await method(p, 'Pontos');
      await expect(p.getByText('Restam 27 pontos')).toBeVisible();
      await expect(p.getByText('SRD 5.2.1 (regras de 2024)')).toBeVisible();

      // 15, 14, 13, 10, 10 and 8: 9 + 7 + 5 + 2 + 2 + 0 = 25 of 27.
      const buy = async (ability: string, to: number) => {
        for (let v = 8; v < to; v++) {
          await p.getByRole('button', { name: `Aumentar ${ability}`, exact: true }).click();
        }
      };
      await buy('Sabedoria', 15);
      await buy('Destreza', 14);
      await buy('Constituição', 13);
      await buy('Força', 10);
      await buy('Carisma', 10);
      await expect(p.getByText('Restam 2 pontos')).toBeVisible();
      await expect(p.getByText('25 de 27 gastos')).toBeVisible();
      // One more point on the 13 costs 2: it fits. On a 10 it fits too, but not on the 14 (it costs 2 and then 2 more).
      await expect(p.getByRole('button', { name: 'Diminuir Inteligência', exact: true })).toHaveAttribute('aria-disabled', 'true');

      await fillChoicesStep(p);

      await p.getByRole('button', { name: 'Criar personagem' }).click();
      await expect(p).toHaveURL(/\/campaigns\/[^/]+\/characters\/(?!new$)[^/]+$/);
      const characterId = p.url().split('/').pop()!;
      const res = await callRPC(p, 'meurpg.characters.v1.CharacterService/GetCharacter', { campaignId, characterId });
      expect(res.ok(), await res.text()).toBeTruthy();
      const sheet = (await res.json()).character.sheet.full;
      expect(sheet.abilityOrigin.method).toBe('ABILITY_METHOD_POINT_BUY');
      expect(sheet.baseScores).toMatchObject({ strength: 10, dexterity: 14, constitution: 13, intelligence: 8, wisdom: 15, charisma: 10 });
    } finally {
      await master.close();
      await player.close();
    }
  },
);

test(
  'os 4d6 vêm do servidor, e recarregar mostra os mesmos @RN-24',
  { tag: '@RN-24' },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Quatro d6 ${Date.now()}`);

      await p.goto(`/campaigns/${campaignId}/characters/new`);
      await fillBasics(p, 'Ícaro');
      await p.getByRole('tab', { name: 'Habilidades' }).click();
      await method(p, '4d6');
      await p.getByRole('button', { name: 'Rolar as habilidades' }).click();
      const chips = p.getByRole('group', { name: /^\d+: dados \d, \d, \d e \d; o \d foi descartado/ });
      await expect(chips).toHaveCount(6);
      await expect(p.getByText(/Rolados em \d\d\/\d\d \d\d:\d\d\. Rolar de novo mostra os mesmos\./)).toBeVisible();
      const first = await chips.evaluateAll((els) => els.map((e) => e.getAttribute('aria-label')!));

      // Reload: the server keeps the roll, so the same six come back and there is no button to roll again.
      await p.reload();
      await fillBasics(p, 'Ícaro');
      await p.getByRole('tab', { name: 'Habilidades' }).click();
      await method(p, '4d6');
      await expect(p.getByRole('button', { name: 'Rolar as habilidades' })).toHaveCount(0);
      await expect(chips).toHaveCount(6);
      expect(await chips.evaluateAll((els) => els.map((e) => e.getAttribute('aria-label')!))).toEqual(first);
      const stored = await callRPC(p, 'meurpg.characters.v1.CharacterService/GetAbilityRolls', { campaignId });
      expect((await stored.json()).rolls.sets).toHaveLength(6);

      // Placing them in order and creating the sheet: the server checks the scores against the stored roll.
      const abilities = ['Força', 'Destreza', 'Constituição', 'Inteligência', 'Sabedoria', 'Carisma'];
      for (const [i, ability] of abilities.entries()) {
        await p.getByLabel(ability, { exact: true }).selectOption({ index: i + 1 });
      }
      await fillChoicesStep(p);
      await p.getByRole('button', { name: 'Criar personagem' }).click();
      await expect(p).toHaveURL(/\/campaigns\/[^/]+\/characters\/(?!new$)[^/]+$/);
      const characterId = p.url().split('/').pop()!;
      const res = await callRPC(p, 'meurpg.characters.v1.CharacterService/GetCharacter', { campaignId, characterId });
      const sheet = (await res.json()).character.sheet.full;
      expect(sheet.abilityOrigin.method).toBe('ABILITY_METHOD_ROLLED_4D6');
      expect(sheet.abilityOrigin.rolls).toHaveLength(6);
    } finally {
      await master.close();
      await player.close();
    }
  },
);

test(
  'um 19 digitado é recusado, na tela e pelo servidor, e um jeito desligado some @RN-24',
  { tag: '@RN-24' },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Digitar ${Date.now()}`);

      await p.goto(`/campaigns/${campaignId}/characters/new`);
      await fillBasics(p, 'Ícaro');
      await p.getByRole('tab', { name: 'Habilidades' }).click();
      await method(p, 'Digitar');
      await expect(p.getByText('de 3 a 18, antes do bônus da raça')).toBeVisible();
      await p.locator('input').and(p.getByLabel('Força', { exact: true })).fill('19');
      await fillChoicesStep(p);
      await p.getByRole('button', { name: 'Criar personagem' }).click();
      await expect(p.getByText(/digite valores de 3 a 18/)).toBeVisible();
      await expect(p).toHaveURL(/characters\/new$/);

      // The server refuses the same scores on its own, by the typed reason.
      const body = characterRpcBody('PLAYER', pensantus) as { sheet: { full: { baseScores: Record<string, number> } } };
      body.sheet.full.baseScores = { strength: 19, dexterity: 10, constitution: 10, intelligence: 10, wisdom: 10, charisma: 10 };
      const refused = await createCharacterRPC(p, campaignId, { ...body, abilityMethod: 'ABILITY_METHOD_TYPED' });
      expect(refused.status()).toBe(400);
      expect(JSON.stringify(await refused.json())).toContain('TYPED_OUT_OF_RANGE');

      // The master switches "Digitar" off: the player no longer sees it, and the server refuses it.
      await setTableRulesRPC(m, campaignId, { abilityMethods: { standardArray: true, pointBuy: true, rolled4d6: true, typed: false } });
      await p.reload();
      await p.getByRole('tab', { name: 'Habilidades' }).click();
      await expect(p.getByRole('radio', { name: 'Digitar' })).toHaveCount(0);
      await expect(p.getByRole('radio', { name: 'Padrão' })).toBeVisible();
      body.sheet.full.baseScores = { strength: 12, dexterity: 10, constitution: 10, intelligence: 10, wisdom: 10, charisma: 10 };
      const notAllowed = await createCharacterRPC(p, campaignId, { ...body, abilityMethod: 'ABILITY_METHOD_TYPED' });
      expect(notAllowed.status()).toBe(400);
      expect(JSON.stringify(await notAllowed.json())).toContain('METHOD_NOT_ALLOWED');
    } finally {
      await master.close();
      await player.close();
    }
  },
);

test(
  'mudar o modo de XP depois de dar XP pede a confirmação no lugar @RN-09',
  { tag: '@RN-09' },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const table = await tableForXp(m, p, `Modo de XP ${Date.now()}`);
      // Nobody has XP yet: a change would be made at once. Give some first.
      await awardXpRPC(m, table.campaignId, { mode: 'MANUAL', reason: 'A porta da torre', characterIds: [table.characterId], amount: 50 });

      await m.goto(`/campaigns/${table.campaignId}/rules`);
      await expect(m.getByRole('radio', { name: /Por inimigos/ })).toBeChecked();
      await pickRadio(m, /Por marcos/);
      // Picking only picks: the campaign still has the old mode until "Mudar para marcos".
      await expect(m.getByRole('button', { name: 'Mudar para marcos' })).toBeVisible();
      expect((await (await callRPC(m, 'meurpg.campaigns.v1.CampaignService/GetCampaign', { campaignId: table.campaignId })).json()).campaign.xpMode).toBe('XP_MODE_ENEMIES');
      await m.getByRole('button', { name: 'Mudar para marcos' }).click();
      await expect(m.getByRole('heading', { name: 'Mudar para “por marcos”?' })).toBeFocused();
      await expect(m.getByText('Já houve XP dado nesta campanha. Mudar vale só daqui para frente.')).toBeVisible();
      // Going back changes nothing.
      await m.getByRole('button', { name: 'Voltar', exact: true }).click();
      await expect(m.getByRole('radio', { name: /Por inimigos/ })).toBeChecked();

      await pickRadio(m, /Por marcos/);
      await m.getByRole('button', { name: 'Mudar para marcos' }).click();
      await m.getByRole('button', { name: 'Mudar para marcos' }).click();
      await expect(m.getByText(/Modo de XP mudado para por marcos, em \d\d\/\d\d\/\d{4}/)).toBeVisible();
      await m.reload();
      await expect(m.getByRole('radio', { name: /Por marcos/ })).toBeChecked();
      const campaign = await callRPC(m, 'meurpg.campaigns.v1.CampaignService/GetCampaign', { campaignId: table.campaignId });
      expect((await campaign.json()).campaign.xpMode).toBe('XP_MODE_MILESTONES');
    } finally {
      await master.close();
      await player.close();
    }
  },
);

test(
  'calibrar o mapa a 3 m guarda as paredes pintadas, escaladas, e uma mudança que não é múltiplo pergunta antes @RN-25',
  { tag: '@RN-25' },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 1000 } });
    try {
      const page = await master.newPage();
      await page.goto('/');
      const campaignId = await masterCampaign(page, `Calibração ${Date.now()}`);
      const map = await mapToPaint(page, campaignId, 'A torre em ruínas', 12);
      const painted = await callRPC(page, 'meurpg.maps.v1.MapService/PaintMapCells', {
        campaignId,
        mapId: map.mapId,
        layer: 'MAP_LAYER_WALL',
        value: 1,
        squares: wallSquares(),
      });
      expect(painted.ok(), await painted.text()).toBeTruthy();
      expect((await layersOf(page, campaignId, map.mapId)).wall).toBe(5);

      await page.goto(`/campaigns/${campaignId}/maps/${map.mapId}`);
      await page.getByRole('radio', { name: 'Pintar' }).click();
      const grid = page.getByRole('region', { name: 'Grade' });
      await expect(grid.getByText('12 × 8 quadrados')).toBeVisible();
      await grid.getByRole('button', { name: 'Calibrar o quadrado' }).click();
      await expect(page.getByRole('heading', { name: 'Cada quadrado deste desenho vale' })).toBeFocused();
      await factor(page, '3 m');
      await expect(page.getByText('o mapa terá 24 × 16 quadrados.')).toBeVisible();
      await expect(page.getByText(/cada quadrado pintado vira 2\s×\s2/)).toBeVisible();
      await page.getByRole('button', { name: 'Salvar a grade' }).click();

      // Nothing was asked: the walls stay, each one now 2 × 2.
      await expect(grid.getByText('12 × 8 quadrados do desenho')).toBeVisible();
      await expect(grid.getByText('cada um vale 3 m')).toBeVisible();
      await expect(grid.getByText('nas regras: 24 × 16 quadrados de 1,5 m')).toBeVisible();
      const scaled = await layersOf(page, campaignId, map.mapId);
      expect(scaled).toMatchObject({ columns: 24, rows: 16, wall: 20 });
      const got = await callRPC(page, 'meurpg.maps.v1.MapService/GetMap', { campaignId, mapId: map.mapId });
      expect((await got.json()).map).toMatchObject({ drawnColumns: 12, drawnRows: 8, squareFactor: 2, gridColumns: 24 });

      // 3 m to 4,5 m is not a multiple: it clears the layers, so the app asks first.
      await grid.getByRole('button', { name: 'Calibrar o quadrado' }).click();
      await factor(page, '4,5 m');
      await page.getByRole('button', { name: 'Salvar a grade' }).click();
      await expect(page.getByRole('heading', { name: 'Mudar a grade?' })).toBeFocused();
      expect((await layersOf(page, campaignId, map.mapId)).wall).toBe(20);
      await page.getByRole('button', { name: 'Voltar', exact: true }).click();
      await expect(page.getByRole('heading', { name: 'Cada quadrado deste desenho vale' })).toBeVisible();
      await page.getByRole('button', { name: 'Salvar a grade' }).click();
      await page.getByRole('button', { name: 'Apagar e mudar a grade' }).click();
      await expect(grid.getByText('cada um vale 4,5 m')).toBeVisible();
      expect(await layersOf(page, campaignId, map.mapId)).toMatchObject({ columns: 36, rows: 24, wall: 0 });
    } finally {
      await master.close();
    }
  },
);

test(
  'a mesa decide os PV do nível: com a regra fixa, o jogador não escolhe @RN-24',
  { tag: '@RN-24' },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    let campaignId = '';
    let m: Page | undefined;
    try {
      m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const table = await tableForLevelUp(m, p, `PV do nível ${Date.now()}`, { build: toren });
      campaignId = table.campaignId;
      const page = `/campaigns/${campaignId}/characters/${table.characterId}/level-up`;

      // The default: the player chooses, two cards.
      await p.goto(page);
      await passClassStep(p);
      await expect(p.getByText('Passo 2 de 3 · Vida')).toBeVisible();
      await expect(p.getByRole('radio', { name: /Média:/ })).toBeVisible();
      await expect(p.getByRole('radio', { name: /Rolar 1d10/ })).toBeVisible();

      // "Rolar": only the die, and the server refuses the average on its own.
      await setTableRulesRPC(m, campaignId, { hitPoints: 'HIT_POINTS_RULE_ROLL' });
      await p.goto(page);
      await passClassStep(p);
      await expect(p.getByText('A mesa pede que todos rolem o dado de vida. A média não é oferecida.')).toBeVisible();
      await expect(p.getByRole('radio', { name: /Média:/ })).toHaveCount(0);
      await expect(p.getByText('Falta rolar o dado de vida.')).toBeVisible();
      await p.getByRole('button', { name: /Rolar no app/ }).click();
      await expect(p.getByText(/Rolado no app: \d+ no d10/)).toBeVisible();

      // "A média": only the average, no die.
      await setTableRulesRPC(m, campaignId, { hitPoints: 'HIT_POINTS_RULE_AVERAGE' });
      await p.goto(page);
      await passClassStep(p);
      await expect(p.getByText('A mesa usa a média: todos recebem o valor médio do dado de vida. O dado não é oferecido.')).toBeVisible();
      await expect(p.getByRole('radio', { name: /Rolar 1d10/ })).toHaveCount(0);
      await expect(p.getByRole('button', { name: /Rolar no app/ })).toHaveCount(0);
      await expect(p.getByRole('button', { name: 'Próximo' })).toBeEnabled();
    } finally {
      if (m && campaignId) {
        await endOpenSessionRPC(m, campaignId);
      }
      await master.close();
      await player.close();
    }
  },
);

test(
  'o jogador lê as regras da mesa, sem controles, e o link das regras de mapas leva aos mapas @RN-24',
  { tag: '@RN-24' },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Leitura ${Date.now()}`);
      await setTableRulesRPC(m, campaignId, { hitPoints: 'HIT_POINTS_RULE_AVERAGE', houseRules: ['Beber uma poção é uma ação bônus'] });

      await p.goto(`/campaigns/${campaignId}`);
      await p.getByRole('link', { name: 'Ler as regras' }).click();
      await expect(p).toHaveURL(new RegExp(`/campaigns/${campaignId}/rules$`));
      await expect(p.getByRole('heading', { level: 1, name: 'Regras da mesa' })).toBeVisible();
      await expect(p.getByText('Só o mestre muda as regras da mesa.')).toBeVisible();
      await expect(p.getByText('Pontos de vida ao subir de nível', { exact: true })).toBeVisible();
      await expect(p.getByText('A média', { exact: true })).toBeVisible();
      await expect(p.getByText('Beber uma poção é uma ação bônus')).toBeVisible();
      await expect(p.getByRole('radio')).toHaveCount(0);
      await expect(p.getByRole('switch')).toHaveCount(0);
      await expect(p.getByRole('button', { name: 'Salvar regras' })).toHaveCount(0);

      // The master's page: "Abrir os mapas" opens the campaign at its maps.
      await m.goto(`/campaigns/${campaignId}/rules`);
      await expect(m.getByRole('link', { name: 'Abrir o conteúdo da mesa' })).toHaveCount(0);
      await m.getByRole('link', { name: 'Abrir os mapas' }).click();
      await expect(m).toHaveURL(new RegExp(`/campaigns/${campaignId}/prep#maps$`));
      await expect(m.getByRole('region', { name: 'Mapas' })).toBeInViewport();
    } finally {
      await master.close();
      await player.close();
    }
  },
);

test(
  'com dados físicos, os dados só são guardados depois da pergunta, e a regra de PV da mesa vale na criação @RN-24',
  { tag: '@RN-24' },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Dados físicos ${Date.now()}`);
      await setTableRulesRPC(m, campaignId, { diceMode: 'DICE_MODE_PHYSICAL', hitPoints: 'HIT_POINTS_RULE_ROLL' });

      await p.goto(`/campaigns/${campaignId}/characters/new`);
      await fillBasics(p, 'Ícaro');
      await p.getByRole('tab', { name: 'Habilidades' }).click();
      // The hit points follow the table: only "Rolado", no choice.
      await expect(p.getByText('A mesa pede que os pontos de vida dos níveis acima do 1º sejam rolados')).toBeVisible();
      await expect(p.getByRole('radio', { name: /Média/ })).toHaveCount(0);

      await method(p, '4d6');
      const typed = [[6, 5, 5, 2], [5, 5, 4, 1], [5, 4, 4, 3], [4, 4, 4, 2], [4, 3, 3, 2], [3, 3, 2, 1]];
      for (const [i, row] of typed.entries()) {
        for (const [j, die] of row.entries()) {
          await p.getByLabel(`Rolagem ${i + 1}, dado ${j + 1}`, { exact: true }).fill(String(die));
        }
      }
      await p.getByRole('button', { name: 'Guardar os dados' }).click();
      await expect(p.getByRole('heading', { name: 'Guardar estas rolagens?' })).toBeFocused();
      await expect(p.getByText('Rolagem 1: 6, 5, 5, 2')).toBeVisible();
      // Nothing is stored before the confirmation.
      const before = await callRPC(p, 'meurpg.characters.v1.CharacterService/GetAbilityRolls', { campaignId });
      expect((await before.json()).rolls).toBeUndefined();
      await p.getByRole('button', { name: 'Voltar', exact: true }).click();
      await expect(p.getByLabel('Rolagem 1, dado 1', { exact: true })).toHaveValue('6');
      await p.getByRole('button', { name: 'Guardar os dados' }).click();
      await p.getByRole('button', { name: 'Guardar as rolagens' }).click();
      await expect(p.getByText(/Dados digitados em/)).toBeVisible();
      const after = await callRPC(p, 'meurpg.characters.v1.CharacterService/GetAbilityRolls', { campaignId });
      expect((await after.json()).rolls.sets).toHaveLength(6);
    } finally {
      await master.close();
      await player.close();
    }
  },
);

test(
  '"Reações dos inimigos": o mestre muda para "Sempre", salva, e o jogador lê a regra em palavras @PM-04',
  { tag: '@PM-04' },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Reações ${Date.now()}`);
      await m.goto(`/campaigns/${campaignId}/rules`);
      const group = m.getByRole('radiogroup', { name: 'Reações dos inimigos' });
      await expect(group.getByRole('radio', { name: /Só quando um inimigo pode reagir/ })).toBeChecked();
      await pickRadio(group, /^Sempre/);
      await m.getByRole('button', { name: 'Salvar regras' }).click();
      await expect.poll(async () => (await tableRulesOf(m, campaignId)).rules.enemyReactions).toBe('ENEMY_REACTIONS_RULE_ALWAYS');

      await p.goto(`/campaigns/${campaignId}/rules`);
      await expect(p.getByText('Reações dos inimigos', { exact: true })).toBeVisible();
      await expect(p.getByText('Sempre', { exact: true })).toBeVisible();
      await expect(p.getByRole('radio')).toHaveCount(0);
    } finally {
      await master.close();
      await player.close();
    }
  },
);
