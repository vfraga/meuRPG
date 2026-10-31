import { expect, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { endOpenSessionRPC } from './live-session-support';
import { archiveEntryRPC } from './spells-support';
import { newSignedInContext, showAllPicks, fillChoicesStep } from './support';
import { passClassStep } from './levelup-support';
import {
  changeGuardianSkillsRPC,
  createGuardianRPC,
  createInkBladeRPC,
  createInkBladeSubclassRPC,
  createOwlRaceRPC,
  createPlainSubclassRPC,
  createSheetRPC,
  emptyTable,
  icaroSheetBody,
  lockAndMilestone,
} from './table-sheet-support';

// MR-025 (the table's own content in the character editor: a class, a race, the "Outro" background and a
// sheet of several classes), RN-23 (a change applies at once and the sheet says what no longer fits) and MR-040
// (the level-up with a table class and with a third caster's subclass). The master's content comes through the
// API (the content editors are another slice); the player's screens are what is under test. Every test makes its
// own campaign.

// A click on something that is not there fails in 15 s instead of waiting for the test's whole timeout.
test.use({ actionTimeout: 15_000 });

const sheetOf = (campaignId: string, characterId: string) => `/campaigns/${campaignId}/characters/${characterId}`;

/** Opens a `mat-select` by its label with the keyboard (a compact grid can put the floating label over the click) and picks an option. */
async function pick(page: Page, label: string, option: string | RegExp): Promise<void> {
  const control = page.getByRole('combobox', { name: label, exact: true });
  await control.focus();
  await control.press('Enter');
  await page.getByRole('option', { name: option }).click();
  await expect(control).toHaveAttribute('aria-expanded', 'false');
}

/** The "Habilidades" step with the typed way ("Digitar"), the scores given. */
async function typeScores(page: Page, scores: Record<string, number>): Promise<void> {
  await page.getByRole('tab', { name: 'Habilidades' }).click();
  await page.locator('.seg__item').filter({ hasText: 'Digitar' }).click();
  for (const [label, value] of Object.entries(scores)) {
    await page.getByLabel(label, { exact: true }).fill(String(value));
  }
}

test(
  'Davi cria o Ícaro com a classe e a raça da mesa e o antecedente Outro',
  { tag: ['@MR-025', '@RN-23'] },
  async ({ browser }) => {
    test.setTimeout(150_000);
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
    const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 900 } });
    const m = await master.newPage();
    const p = await player.newPage();
    try {
      await m.goto('/');
      await p.goto('/');
      const { campaignId } = await emptyTable(m, p, `Ícaro ${Date.now()}`);
      await createGuardianRPC(m, campaignId);
      await createOwlRaceRPC(m, campaignId);

      await p.goto(`/campaigns/${campaignId}/characters/new`);
      await p.getByLabel('Nome do personagem', { exact: true }).fill('Ícaro');

      // The table's entries sit among the SRD's with the neutral "Da mesa" tag, in the lists and in the closed select.
      const race = p.getByRole('combobox', { name: 'Raça', exact: true });
      await race.focus();
      await race.press('Enter');
      await expect(p.getByRole('option', { name: /^Corujeiro/ })).toContainText('Da mesa');
      await expect(p.getByRole('option', { name: /^Gnomo$/ })).not.toContainText('Da mesa');
      await p.getByRole('option', { name: /^Corujeiro/ }).click();
      await expect(p.getByText('Dá +2 e +1 nas habilidades que você escolher')).toBeVisible();

      await pick(p, 'Classe', /^Guardião do Vale/);
      await expect(p.getByRole('combobox', { name: 'Classe', exact: true })).toContainText('Da mesa');
      await expect(p.getByText('O Guardião do Vale escolhe a subclasse no nível 3.')).toBeVisible();

      // "Outro": a name, two skills, two tools or languages, the feature and the equipment (SRD 5.1, question 82).
      await pick(p, 'Antecedente', 'Outro (personalizado)');
      await expect(p.getByRole('heading', { name: 'Personalizar um antecedente' })).toBeVisible();
      await p.getByLabel('Nome do antecedente', { exact: true }).fill('Batedor de torre');
      const bgSkills = p.locator('[aria-labelledby="background-skills-label"]');
      await bgSkills.getByRole('checkbox', { name: 'Percepção', exact: true }).check();
      await bgSkills.getByRole('checkbox', { name: 'Sobrevivência', exact: true }).check();
      const tools = p.getByRole('combobox', { name: 'Ferramentas ou idiomas (escolha 2)' });
      await tools.focus();
      await tools.press('Enter');
      await p.getByRole('option', { name: 'Ferramentas de ladrão' }).click();
      await p.getByRole('option', { name: 'Élfico' }).click();
      // A third is not offered once two are chosen.
      await expect(p.getByRole('option', { name: 'Anão' })).toBeDisabled();
      await p.keyboard.press('Escape');
      await expect(p.locator('mat-hint').filter({ hasText: '2 de 2 escolhidas' })).toBeVisible();
      await p.getByLabel('Nome da característica*', { exact: true }).fill('Olho no horizonte');
      await p.getByLabel('Texto da característica*', { exact: true }).fill('Você sempre acha o ponto mais alto de um lugar.');
      await p.getByLabel('Equipamento do antecedente*', { exact: true }).fill('Uma luneta, um rolo de corda e 10 PO.');

      await typeScores(p, { Força: 14, Destreza: 13, Constituição: 14, Inteligência: 10, Sabedoria: 15, Carisma: 8 });

      // The class's skill count and saving throws are the server's entry.
      await p.getByRole('tab', { name: 'Perícias' }).click();
      await expect(p.getByText('O Guardião do Vale escolhe 2 perícias ao começar e dá proficiência nos testes de resistência de Força e Sabedoria.')).toBeVisible();
      const skills = p.getByRole('group', { name: 'Perícias', exact: true });
      await skills.getByRole('checkbox', { name: 'Atletismo', exact: true }).check();
      await skills.getByRole('checkbox', { name: 'Natureza', exact: true }).check();

      // The race's +2 and +1 are two choices of "Escolhas", asked once the server has read the draft: the step is there, and the
      // sheet is not complete without them.
      await p.getByRole('tab', { name: /Escolhas/ }).click();
      await p.getByRole('region', { name: /^Bônus de \+2/ }).getByRole('checkbox', { name: /^Sabedoria/ }).click();
      await p.getByRole('region', { name: /^Bônus de \+1/ }).getByRole('checkbox', { name: /^Destreza/ }).click();
      await fillChoicesStep(p);

      await p.getByRole('button', { name: 'Criar personagem' }).click();
      await expect(p).toHaveURL(/\/campaigns\/[^/]+\/characters\/(?!new$)[^/]+$/);
      await expect(p.getByRole('heading', { name: 'Ícaro', level: 1 })).toBeVisible();
      // The scores of the sheet carry the picks: Sabedoria 15 + 1 of the race + 2 picked, Destreza 13 + 1 picked.
      const sheetText = (await p.locator('main').innerText()).replace(/\s+/g, ' ');
      expect(sheetText).toContain('Sabedoria +4 valor 18');
      expect(sheetText).toContain('Destreza +2 valor 14');
      await expect(p.getByText('Guardião do Vale 1').first()).toBeVisible();
      await expect(p.getByText('Corujeiro').first()).toBeVisible();
      // The background's own feature is on the sheet, as the player wrote it.
      await expect(p.getByText('Olho no horizonte')).toBeVisible();
    } finally {
      await master.close();
      await player.close();
    }
  },
);

test(
  'Rafa cria a Corvina, Maga 3 e Clériga 1, cada classe com a subclasse da mesa, e escolhe as magias de cada lista',
  { tag: ['@MR-025', '@RN-23'] },
  async ({ browser }) => {
    test.setTimeout(150_000);
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
    const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 900 } });
    const m = await master.newPage();
    const p = await player.newPage();
    try {
      await m.goto('/');
      await p.goto('/');
      const { campaignId } = await emptyTable(m, p, `Corvina ${Date.now()}`);
      await createPlainSubclassRPC(m, campaignId, 'class:wizard', 'Tradição da Tinta', 2);
      await createPlainSubclassRPC(m, campaignId, 'class:cleric', 'Domínio do Caminho', 1, [{ classLevel: 1, spellKey: 'spell:detect-magic' }]);
      await createInkBladeRPC(m, campaignId);

      await p.goto(`/campaigns/${campaignId}/characters/new`);
      await p.getByLabel('Nome do personagem', { exact: true }).fill('Corvina');
      await pick(p, 'Raça', 'Gnomo');
      await pick(p, 'Classe', 'Mago');
      await p.getByLabel('Nível', { exact: true }).fill('3');
      await pick(p, 'Subclasse', /^Tradição da Tinta/);
      await pick(p, 'Antecedente', 'Acólito');

      // One block per class, the total level read from them.
      await p.getByRole('button', { name: 'Adicionar classe' }).click();
      const blocks = p.locator('app-class-block');
      await expect(blocks).toHaveCount(2);
      await expect(blocks.nth(0).getByRole('heading', { name: 'Classe 1' })).toBeVisible();
      await expect(blocks.nth(1).getByRole('heading', { name: 'Classe 2' })).toBeVisible();
      const second = blocks.nth(1);
      const classSelect = second.getByRole('combobox', { name: 'Classe', exact: true });
      await classSelect.focus();
      await classSelect.press('Enter');
      await p.getByRole('option', { name: 'Clérigo' }).click();
      await second.getByLabel('Nível', { exact: true }).fill('1');
      const subclassSelect = second.getByRole('combobox', { name: 'Subclasse', exact: true });
      await subclassSelect.focus();
      await subclassSelect.press('Enter');
      // The domain of the table sits beside the SRD's, tagged.
      await expect(p.getByRole('option', { name: /^Domínio do Caminho/ })).toContainText('Da mesa');
      await expect(p.getByRole('option', { name: /^Domínio da Vida$/ })).not.toContainText('Da mesa');
      await p.getByRole('option', { name: /^Domínio do Caminho/ }).click();
      await expect(p.getByText('Nível total').locator('..')).toContainText('4');
      await expect(p.getByText('Os pré-requisitos de cada classe aparecem na ficha')).toBeVisible();

      await typeScores(p, { Força: 10, Destreza: 12, Constituição: 14, Inteligência: 15, Sabedoria: 14, Carisma: 8 });

      // The spell step: a section per class, each list up to the circle of that class level.
      await p.getByRole('tab', { name: 'Magias' }).click();
      await expect(p.getByRole('heading', { name: 'Mago · até o 2º nível de magia' })).toBeVisible();
      await expect(p.getByRole('heading', { name: 'Clérigo · até o 1º nível de magia' })).toBeVisible();
      const wizard = p.locator('.spell-section').filter({ hasText: 'Mago · até o 2º nível de magia' });
      const cleric = p.locator('.spell-section').filter({ hasText: 'Clérigo · até o 1º nível de magia' });
      // The master's own spell is on the wizard's list, with the others of its circle.
      await expect(wizard.getByRole('checkbox', { name: /^Lâmina de Nanquim/ }).first()).toBeVisible();
      await expect(wizard.getByRole('checkbox', { name: /^Bola de Fogo/ })).toHaveCount(0);
      await expect(cleric.getByRole('checkbox', { name: /^Bênção/ }).first()).toBeVisible();
      await expect(cleric.getByRole('checkbox', { name: /^Lâmina de Nanquim/ })).toHaveCount(0);
      await wizard.getByRole('group', { name: 'Magias conhecidas' }).getByRole('checkbox', { name: /^Lâmina de Nanquim/ }).check();
      await cleric.getByRole('group', { name: 'Magias preparadas' }).getByRole('checkbox', { name: /^Bênção/ }).check();

      // A spell no class of the sheet lists is greyed, with why and a link to "Magias".
      await cleric.getByPlaceholder('Buscar magia').fill('amizade');
      const out = cleric.locator('.picker__out');
      await expect(out).toContainText('Amizade Animal');
      await expect(out).toContainText('Fora da lista das suas classes');
      await expect(out.getByRole('link', { name: 'Ver em Magias' })).toHaveAttribute('href', `/campaigns/${campaignId}/spells`);
      await expect(out.getByRole('checkbox')).toHaveCount(0);

      await fillChoicesStep(p);

      await p.getByRole('button', { name: 'Criar personagem' }).click();
      await expect(p).toHaveURL(/\/campaigns\/[^/]+\/characters\/(?!new$)[^/]+$/);
      await expect(p.getByRole('heading', { name: 'Corvina', level: 1 })).toBeVisible();
      await expect(p.getByText('Mago 3').first()).toBeVisible();
      await expect(p.getByText('Clérigo 1').first()).toBeVisible();
    } finally {
      await master.close();
      await player.close();
    }
  },
);

test(
  'o Ícaro sobe do nível 1 para o 2 com a classe da mesa: a vida, o estilo de luta, as magias e o resumo com "Da mesa"',
  { tag: ['@MR-025', '@MR-040', '@RN-23'] },
  async ({ browser }) => {
    test.setTimeout(150_000);
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
    const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 900 } });
    const m = await master.newPage();
    const p = await player.newPage();
    let campaignId = '';
    try {
      await m.goto('/');
      await p.goto('/');
      const table = await emptyTable(m, p, `Subida do Ícaro ${Date.now()}`);
      campaignId = table.campaignId;
      const guardian = await createGuardianRPC(m, campaignId);
      const characterId = await createSheetRPC(p, campaignId, icaroSheetBody(guardian));
      await lockAndMilestone(m, campaignId, characterId);

      await p.goto(sheetOf(campaignId, characterId));
      await expect(p.getByRole('heading', { name: 'Ícaro pode subir de nível' })).toBeVisible();
      await p.getByRole('link', { name: 'Subir para o nível 2' }).click();
      await expect(p.getByRole('heading', { name: 'Subir para o nível 2' })).toBeVisible();
      await expect(p.getByText('Ícaro · Guardião do Vale 1 → Guardião do Vale 2')).toBeVisible();

      // Vida: the average, by the table's rule; the server gives the numbers.
      await passClassStep(p);
      await expect(p.getByText('Passo 2 de')).toBeVisible();
      await expect(p.getByText('Média: 6')).toBeVisible();
      await p.getByRole('button', { name: 'Próximo' }).click();

      // Escolhas: the fighting style the class offers at level 2.
      await expect(p.getByText(/Passo 3 de \d · Escolhas/)).toBeVisible();
      await expect(p.getByRole('heading', { name: 'Estilo de luta' })).toBeVisible();
      await p.locator('#pick-feature-0').getByRole('radio').first().click();
      await p.getByRole('button', { name: 'Próximo' }).click();

      // Magias: the half caster prepares from the druid's list, whose name the step says.
      await expect(p.getByText(/Passo 4 de \d · Magias/)).toBeVisible();
      const prepare = p.locator('#pick-prepared');
      await expect(prepare).toBeVisible();
      // The server says how many to prepare ("Faltam preparar 2 magias."): pick that many from the list.
      const reason = (await p.locator('#foot-reason').textContent()) ?? '';
      const howMany = Number(/(\d+)/.exec(reason)?.[1] ?? '1');
      expect(howMany).toBeGreaterThan(0);
      // By name, from the druid's list the guardian casts from.
      await showAllPicks(prepare);
      for (const name of ['Amizade Animal', 'Bom Fruto', 'Criar ou Destruir Água', 'Curar Ferimentos'].slice(0, howMany)) {
        await prepare.getByRole('checkbox', { name: new RegExp(`^${name}`) }).check();
      }
      await p.getByRole('button', { name: 'Próximo' }).click();

      // Resumo: only the rows that change, the slots tagged "Da mesa".
      await expect(p.getByText(/Passo 5 de 5 · Resumo/)).toBeVisible();
      const summary = p.getByRole('region', { name: 'O que muda', exact: true });
      await expect(summary).toContainText('Guardião do Vale 1');
      await expect(summary).toContainText('Guardião do Vale 2');
      const slots = summary.locator('li').filter({ hasText: 'Espaços de 1º nível' });
      await expect(slots).toContainText('Da mesa');
      await expect(slots).toContainText('Da tabela da classe');
      await expect(slots).toContainText('0');
      await expect(slots).toContainText('2');
      await expect(summary.locator('li').filter({ hasText: 'Novas características' })).toContainText('Estilo de luta');
      // A row that does not change is not there.
      await expect(summary).not.toContainText('Teste de resistência de Constituição');
      await p.getByRole('button', { name: 'Confirmar o nível 2' }).click();

      await expect(p).toHaveURL(sheetOf(campaignId, characterId));
      await expect(p.getByText('Ícaro subiu para o nível 2.')).toBeVisible();
    } finally {
      if (campaignId) await endOpenSessionRPC(m, campaignId);
      await master.close();
      await player.close();
    }
  },
);

test(
  'o Guerreiro escolhe a subclasse que conjura no nível 3 e ganha as magias da lista do Mago na subida de nível',
  { tag: ['@MR-025', '@MR-040', '@RN-23'] },
  async ({ browser }) => {
    test.setTimeout(150_000);
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
    const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 900 } });
    const m = await master.newPage();
    const p = await player.newPage();
    let campaignId = '';
    try {
      await m.goto('/');
      await p.goto('/');
      const table = await emptyTable(m, p, `Lâmina de Tinta ${Date.now()}`);
      campaignId = table.campaignId;
      await createInkBladeSubclassRPC(m, campaignId);
      await createInkBladeRPC(m, campaignId);
      const sheet = icaroSheetBody('class:fighter');
      sheet.name = 'Rúnico';
      (sheet.sheet as any).full.classes = [{ classKey: 'class:fighter', level: 2 }];
      (sheet.sheet as any).full.experiencePoints = 900;
      (sheet.sheet as any).full.skillProficiencyKeys = ['skill:athletics', 'skill:perception'];
      const characterId = await createSheetRPC(p, campaignId, sheet);
      await lockAndMilestone(m, campaignId, characterId);

      await p.goto(sheetOf(campaignId, characterId));
      await p.getByRole('link', { name: 'Subir para o nível 3' }).click();
      await expect(p.getByRole('heading', { name: 'Subir para o nível 3' })).toBeVisible();
      // Before the subclass is chosen the fighter casts nothing: Classe, Vida, Escolhas, Resumo.
      await passClassStep(p);
      await expect(p.getByText(/Passo 2 de 4/)).toBeVisible();
      await p.getByRole('button', { name: 'Próximo' }).click();
      await expect(p.getByRole('heading', { name: 'Subclasse' })).toBeVisible();
      await p.locator('#pick-subclass').getByRole('radio', { name: /Lâmina de Tinta/ }).click();
      // Picking it makes the Magias step appear: the step count follows.
      await expect(p.getByText(/Passo 3 de 5/)).toBeVisible();
      await p.getByRole('button', { name: 'Próximo' }).click();

      await expect(p.getByText(/Passo 4 de 5 · Magias/)).toBeVisible();
      // The cantrips and the spells come from the wizard's list, not the fighter's.
      await expect(p.getByText(/truques de Mago/)).toBeVisible();
      await expect(p.getByText(/Escolha 3 magias de 1º nível para aprender/)).toBeVisible();
      const cantrips = p.locator('#pick-cantrips');
      await showAllPicks(cantrips);
      await cantrips.getByRole('checkbox', { name: /^Luz/ }).check();
      await cantrips.getByRole('checkbox', { name: /^Ilusão Menor/ }).check();
      const spells = p.locator('#pick-spells');
      await showAllPicks(spells);
      // The master's own spell is on the wizard's list, so the fighter's subclass can learn it.
      await p.getByLabel('Buscar magia').fill('nanquim');
      await spells.getByRole('checkbox', { name: /Lâmina de Nanquim/ }).check();
      await p.getByLabel('Buscar magia').fill('');
      await spells.getByRole('checkbox', { name: /^Alarme/ }).check();
      await spells.getByRole('checkbox', { name: /^Armadura Arcana/ }).check();
      await p.getByRole('button', { name: 'Próximo' }).click();

      await expect(p.getByText(/Passo 5 de 5 · Resumo/)).toBeVisible();
      await p.getByRole('button', { name: 'Confirmar o nível 3' }).click();
      await expect(p).toHaveURL(sheetOf(campaignId, characterId));
      await expect(p.getByText('Rúnico subiu para o nível 3.')).toBeVisible();
      await expect(p.getByText('Lâmina de Nanquim').first()).toBeVisible();
      // The sheet casts with the subclass's own ability and the proficiency bonus (the fighter's Intelligence 10 and +2).
      const casting = p.getByRole('heading', { name: /Magias/ }).locator('xpath=ancestor::*[contains(@class, "mr-panel")][1]');
      await expect(casting.getByText('Habilidade').locator('..')).toContainText('Inteligência');
      await expect(casting.getByText('CD de magia').locator('..')).toContainText('10');
      await expect(casting.getByText('Ataque de magia').locator('..')).toContainText('+2');
    } finally {
      if (campaignId) await endOpenSessionRPC(m, campaignId);
      await master.close();
      await player.close();
    }
  },
);

test(
  'o mestre muda as perícias da classe e a ficha do Ícaro diz o que não combina mais, até ser corrigido',
  { tag: ['@MR-025', '@RN-23'] },
  async ({ browser }) => {
    test.setTimeout(150_000);
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
    const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 320, height: 568 } });
    const m = await master.newPage();
    const p = await player.newPage();
    try {
      await m.goto('/');
      await p.goto('/');
      const { campaignId } = await emptyTable(m, p, `A classe mudou ${Date.now()}`);
      // The class gives 3 skills at first, and Ícaro has 3.
      const guardian = await createGuardianRPC(m, campaignId, { skillChoose: 3 });
      const body = icaroSheetBody(guardian);
      (body.sheet as any).full.skillProficiencyKeys = ['skill:athletics', 'skill:nature', 'skill:stealth'];
      const characterId = await createSheetRPC(p, campaignId, body);

      await p.goto(sheetOf(campaignId, characterId));
      await expect(p.getByRole('heading', { name: 'Ícaro', level: 1 })).toBeVisible();
      await expect(p.getByText('A classe mudou.')).toHaveCount(0);

      // The master changes it: it is live at once.
      await changeGuardianSkillsRPC(m, campaignId, guardian, 2);
      await p.reload();
      const notice = p.locator('app-changed-content');
      await expect(notice).toContainText('A classe mudou.');
      await expect(notice).toContainText('O mestre mudou a classe Guardião do Vale em ');
      await expect(notice).toContainText('Guardião do Vale agora dá 2 perícias no nível 1; esta ficha tem 3.');
      // At 320 px nothing scrolls sideways.
      expect(await p.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);

      // "Ver o que mudou" opens the sheet with the same sentence, who fixes it, and "Fechar".
      await p.getByRole('button', { name: 'Ver o que mudou' }).click();
      const sheet = p.locator('app-changed-content-sheet');
      await expect(sheet.getByRole('heading', { name: 'O que mudou: Guardião do Vale' })).toBeVisible();
      await expect(sheet).toContainText('Guardião do Vale agora dá 2 perícias no nível 1; esta ficha tem 3.');
      await expect(sheet).toContainText('Quem ajusta: você, na ficha.');
      await expect(sheet).toContainText('Perícias da ficha: ');
      await sheet.getByRole('button', { name: 'Fechar', exact: true }).last().click();
      await expect(sheet).toHaveCount(0);

      // The master sees it too.
      await m.goto(sheetOf(campaignId, characterId));
      await expect(m.locator('app-changed-content')).toContainText('A classe mudou.');

      // Ícaro's owner fixes the sheet (the notice says "você, na ficha" for a sheet nobody locked): one skill less, and
      // the notice goes by itself, the numbers matching again. The class is not touched.
      await p.goto(`${sheetOf(campaignId, characterId)}/edit`);
      await p.getByRole('tab', { name: 'Perícias' }).click();
      await p.getByRole('group', { name: 'Perícias', exact: true }).getByRole('checkbox', { name: 'Furtividade', exact: true }).first().uncheck();
      await p.getByRole('button', { name: 'Salvar ficha' }).click();
      await expect(p).toHaveURL(sheetOf(campaignId, characterId));
      await expect(p.getByRole('heading', { name: 'Ícaro', level: 1 })).toBeVisible();
      await expect(p.getByText('A classe mudou.')).toHaveCount(0);
    } finally {
      await master.close();
      await player.close();
    }
  },
);

test(
  'a classe arquivada continua na ficha de quem a tem, que edita sem perder nada, e o mestre não a recebe como escolha nova',
  { tag: ['@MR-025', '@RN-23'] },
  async ({ browser }) => {
    test.setTimeout(150_000);
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
    const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 900 } });
    const m = await master.newPage();
    const p = await player.newPage();
    try {
      await m.goto('/');
      await p.goto('/');
      const { campaignId } = await emptyTable(m, p, `Arquivada ${Date.now()}`);
      const guardian = await createGuardianRPC(m, campaignId);
      const body = icaroSheetBody(guardian);
      (body.sheet as any).full.classes = [{ classKey: guardian, level: 3 }];
      const characterId = await createSheetRPC(p, campaignId, body);
      await archiveEntryRPC(m, campaignId, guardian);

      // The owner edits: the class is still there, tagged, the Magias step stays, and the save goes through.
      await p.goto(`${sheetOf(campaignId, characterId)}/edit`);
      await expect(p.getByRole('combobox', { name: 'Classe', exact: true })).toContainText('Guardião do Vale');
      await expect(p.getByRole('combobox', { name: 'Classe', exact: true })).toContainText('Arquivada');
      await expect(p.getByRole('tab', { name: 'Magias' })).toBeVisible();
      await p.getByRole('button', { name: 'Salvar ficha' }).click();
      await expect(p).toHaveURL(sheetOf(campaignId, characterId));
      await expect(p.getByText('Guardião do Vale 3').first()).toBeVisible();

      // The master making a new NPC is never offered it as a new choice (the server would refuse it).
      await m.goto(`/campaigns/${campaignId}/npcs/new/enemy`);
      const classSelect = m.getByRole('combobox', { name: 'Classe', exact: true });
      await classSelect.focus();
      await classSelect.press('Enter');
      await expect(m.getByRole('option', { name: 'Mago', exact: true })).toBeVisible();
      await expect(m.getByRole('option', { name: /^Guardião do Vale/ })).toHaveCount(0);
    } finally {
      await master.close();
      await player.close();
    }
  },
);
