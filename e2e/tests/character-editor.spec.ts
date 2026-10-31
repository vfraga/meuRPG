import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { authStatePath, createCampaign, fillChoicesStep } from './support';

// MR-004: the character editor offers what the character can have at its
// level (web/src/app/pages/character-editor). Not a sign-in test, so it
// reuses the master's saved session (auth.setup.ts).
test.use({ storageState: authStatePath('Mestre Teste') });

test(
  'o editor da ficha mostra só as magias do nível e deixa tirar a subclasse',
  { tag: '@MR-004' },
  async ({ page }) => {
    const campaignId = await createCampaign(page, `Editor ${Date.now()}`);
    await page.goto(`/campaigns/${campaignId}/characters/new`);

    // A level-1 bard (the default level is 1).
    await page.getByLabel('Nome do personagem', { exact: true }).fill('Lira');
    // Same keyboard route as support.ts's selectMatOption, which is not
    // exported: the panel's options only exist while it is open.
    const classSelect = page.getByRole('combobox', { name: 'Classe', exact: true });
    await classSelect.focus();
    await classSelect.press('Enter');
    await page.getByRole('option', { name: 'Bardo', exact: true }).click();
    await expect(page.getByLabel('Nível', { exact: true })).toHaveValue('1');

    // "Subclasse" waits for the level the class chooses it at (3 for the bard); there it can go back to none.
    await page.getByLabel('Nível', { exact: true }).fill('3');
    const subclassSelect = page.getByRole('combobox', { name: 'Subclasse', exact: true });
    await subclassSelect.focus();
    await subclassSelect.press('Enter');
    await expect(page.getByRole('option').first()).toHaveText('Nenhuma');
    await page.keyboard.press('Escape');
    // Back at level 1 the field is shut again, and says when the class chooses one.
    await page.getByLabel('Nível', { exact: true }).fill('1');
    await expect(page.getByText('O Bardo escolhe a subclasse no nível 3.')).toBeVisible();

    // "Magias conhecidas": only 1st-circle spells at level 1, circle then name.
    await page.getByRole('tab', { name: 'Magias' }).click();
    const known = page.getByRole('group', { name: 'Magias conhecidas', exact: true });
    await expect(known.getByRole('checkbox', { name: /^Amizade Animal/ })).toBeVisible();
    await expect(known.getByRole('checkbox', { name: /^Animar Objetos/ })).toHaveCount(0);

    const labels = await known.locator('mat-checkbox').allInnerTexts();
    const parsed = labels.map((text) => {
      const m = text.trim().match(/^(.*?)\s*\((\d+)º nível/);
      return { name: m![1], level: Number(m![2]) };
    });
    expect(parsed.length).toBeGreaterThan(5);
    expect(parsed.every((s) => s.level === 1)).toBe(true);
    const names = parsed.map((s) => s.name);
    expect(names).toEqual([...names].sort((a, b) => a.localeCompare(b, 'pt-BR')));
  },
);

// MR-004 (E6-20 to E6-23): creation rolls happen in the browser and the "?"
// reads a spell. Each test opens its own campaign and the player's editor.
async function openEditorAs(page: import('@playwright/test').Page, className: string) {
  const campaignId = await createCampaign(page, `Rolagens ${Date.now()}`);
  await page.goto(`/campaigns/${campaignId}/characters/new`);
  await page.getByLabel('Nome do personagem', { exact: true }).fill('Zézinho');
  const classSelect = page.getByRole('combobox', { name: 'Classe', exact: true });
  await classSelect.focus();
  await classSelect.press('Enter');
  await page.getByRole('option', { name: className, exact: true }).click();
}

const ABILITIES = ['Força', 'Destreza', 'Constituição', 'Inteligência', 'Sabedoria', 'Carisma'];

test(
  'rolar 4d6 dá seis resultados e todos podem ser colocados nas habilidades',
  { tag: '@MR-004' },
  async ({ page }) => {
    await openEditorAs(page, 'Bárbaro');
    await page.getByRole('tab', { name: 'Habilidades' }).click();
    await page.getByRole('radio', { name: /Rolar 4d6/ }).check();

    // Six results, each with its four dice and the discarded one named.
    const chips = page.getByRole('group', {
      name: /^\d+: dados \d, \d, \d e \d; o \d foi descartado/,
    });
    await expect(chips).toHaveCount(6);
    const totals: number[] = [];
    for (const label of await chips.evaluateAll((els) =>
      els.map((e) => e.getAttribute('aria-label')!),
    )) {
      totals.push(Number(label.split(':')[0]));
    }
    expect(totals).toEqual([...totals].sort((a, b) => b - a));
    totals.forEach((t) => {
      expect(t).toBeGreaterThanOrEqual(3);
      expect(t).toBeLessThanOrEqual(18);
    });

    // Saving waits until every result has an ability.
    await expect(page.getByText('Faltam 6 habilidades')).toBeVisible();
    await fillChoicesStep(page);
    await page.getByRole('button', { name: 'Criar personagem' }).click();
    await expect(page.getByText(/coloque cada resultado numa habilidade/)).toBeVisible();

    // The failed save took us to the first step with a problem: back to this one.
    await page.getByRole('tab', { name: 'Habilidades' }).click();
    // Result i goes to ability i (option 0 is "Escolher").
    for (const [i, ability] of ABILITIES.entries()) {
      await page.getByLabel(ability, { exact: true }).selectOption({ index: i + 1 });
    }
    await expect(page.getByText('Os seis resultados estão colocados.')).toBeVisible();
    const placed: number[] = [];
    for (const ability of ABILITIES) {
      const text = await page
        .getByLabel(ability, { exact: true })
        .evaluate((s: HTMLSelectElement) => s.selectedOptions[0].textContent!.trim());
      placed.push(Number(text));
    }
    expect(placed).toEqual(totals);
    expect(placed.reduce((a, b) => a + b, 0)).toBe(totals.reduce((a, b) => a + b, 0));
    await expect(page.getByRole('group', { name: /\. Em Força\.$/ })).toHaveCount(1);
  },
);

test(
  'o conjunto padrão pode ser colocado e trocado entre habilidades',
  { tag: '@MR-004' },
  async ({ page }) => {
    await openEditorAs(page, 'Bárbaro');
    await page.getByRole('tab', { name: 'Habilidades' }).click();
    await page.getByRole('radio', { name: /Conjunto padrão/ }).check();

    const chips = page.getByRole('group', { name: /^\d+\. (Livre|Em )/ });
    await expect(chips).toHaveCount(6);
    await expect(page.getByRole('button', { name: 'Rolar de novo' })).toHaveCount(0);

    for (const [i, ability] of ABILITIES.entries()) {
      await page.getByLabel(ability, { exact: true }).selectOption({ index: i + 1 });
    }
    await expect(page.getByLabel('Força', { exact: true })).toHaveValue('0');
    // Força takes Destreza's 14: the two swap, so the six stay the standard array.
    await page.getByLabel('Força', { exact: true }).selectOption({ index: 2 });
    const shown = await Promise.all(
      ABILITIES.map((a) =>
        page
          .getByLabel(a, { exact: true })
          .evaluate((s: HTMLSelectElement) => Number(s.selectedOptions[0].textContent!.trim())),
      ),
    );
    expect(shown[0]).toBe(14);
    expect(shown[1]).toBe(15);
    expect([...shown].sort((a, b) => b - a)).toEqual([15, 14, 13, 12, 10, 8]);
  },
);

test(
  'os pontos de vida rolados preenchem todos os níveis que faltam',
  { tag: '@MR-004' },
  async ({ page }) => {
    await openEditorAs(page, 'Bárbaro');
    await page.getByLabel('Nível', { exact: true }).fill('3');
    await page.getByRole('tab', { name: 'Habilidades' }).click();
    await page.getByRole('radio', { name: /Rolado/ }).check();

    const level2 = page.getByLabel('Nível 2 (1d12)', { exact: true });
    const level3 = page.getByLabel('Nível 3 (1d12)', { exact: true });
    await expect(level2).toHaveValue('');
    await page.getByRole('button', { name: 'Rolar os níveis que faltam' }).click();
    for (const field of [level2, level3]) {
      // The rolls land a moment after the click: wait for them, or an empty field reads as 0.
      await expect(field).not.toHaveValue('');
      const value = Number(await field.inputValue());
      expect(value).toBeGreaterThanOrEqual(1);
      expect(value).toBeLessThanOrEqual(12);
    }
    await expect(page.getByRole('button', { name: 'Rolar os níveis que faltam' })).toBeDisabled();
    await expect(page.getByText(/PV máximos até agora/)).toBeVisible();
  },
);

// The "Pontos de vida" box adds what the server derives for the draft, so the maximum it shows with every roll
// typed is the one the saved sheet has: a Hill Dwarf gets one more hit point per level (Dwarven Toughness).
test(
  'o anão da colina vê no quadro de pontos de vida o mesmo máximo que a ficha salva',
  { tag: '@MR-004' },
  async ({ page }) => {
    const campaignId = await createCampaign(page, `Anão ${Date.now()}`);
    await page.goto(`/campaigns/${campaignId}/npcs/new/enemy`);
    const pick = async (field: string, option: string) => {
      const select = page.getByRole('combobox', { name: field, exact: true });
      await select.focus();
      await select.press('Enter');
      await page.getByRole('option', { name: option, exact: true }).click();
    };

    await page.getByLabel('Nome do personagem', { exact: true }).fill('Gimli');
    await pick('Raça', 'Anão');
    await pick('Sub-raça', 'Anão da Colina');
    await pick('Classe', 'Guerreiro');
    await page.getByLabel('Nível', { exact: true }).fill('3');
    await pick('Antecedente', 'Acólito');
    await page.getByRole('tab', { name: 'Habilidades' }).click();
    await page.locator('input').and(page.getByLabel('Constituição', { exact: true })).fill('14');
    await page.getByRole('radio', { name: /Rolado/ }).check();
    await page.getByLabel('Nível 2 (1d10)', { exact: true }).fill('6');
    await page.getByLabel('Nível 3 (1d10)', { exact: true }).fill('4');

    // Constitution 14 + 2 (the dwarf) = 16, modifier +3: 13 + 9 + 7 from the dice and Dwarven Toughness's 3.
    const box = page.getByRole('status', { name: 'Pontos de vida até agora' });
    await expect(box.getByText('+3 PV de raça, classe ou característica')).toBeVisible();
    await expect(box.locator('.hp__sum strong')).toHaveText('32');
    await expect(box.getByText(/É uma prévia/)).toHaveCount(0);

    await page.getByRole('tab', { name: 'Perícias' }).click();
    const skills = page.getByRole('group', { name: 'Perícias', exact: true });
    await skills.getByRole('checkbox', { name: 'Atletismo', exact: true }).check();
    await skills.getByRole('checkbox', { name: 'Percepção', exact: true }).check();
    await page.getByRole('button', { name: 'Criar NPC' }).click();
    await expect(page).toHaveURL(/\/campaigns\/[^/]+\/characters\/(?!new$)[^/]+$/);
    await expect(page.locator('dt:text-is("Pontos de vida máximos") + dd')).toHaveText('32');
  },
);

test(
  'o "?" ao lado da magia abre a descrição, com o texto do SRD em português e o original em inglês',
  { tag: '@MR-004' },
  async ({ page }) => {
    await openEditorAs(page, 'Bardo');
    await page.getByRole('tab', { name: 'Magias' }).click();
    const help = page.getByRole('button', { name: 'Descrição de Amizade Animal' });
    await help.click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'Amizade Animal' })).toBeVisible();
    await expect(dialog.getByText('1º nível · Encantamento')).toBeVisible();
    for (const label of ['Tempo de conjuração', 'Alcance', 'Componentes', 'Duração']) {
      await expect(dialog.getByText(label, { exact: true })).toBeVisible();
    }
    await expect(dialog.getByText('1 ação')).toBeVisible();
    await expect(dialog.getByText('Texto do SRD 5.1', { exact: true })).toBeVisible();
    await expect(dialog.locator('.spell__prose .srd[lang="en"]')).toHaveCount(0);
    await dialog.getByRole('button', { name: 'Ver em inglês' }).click();
    await expect(dialog.getByText('Texto do SRD 5.1 (em inglês)')).toBeVisible();
    await expect(dialog.locator('.spell__prose .srd[lang="en"]').first()).toContainText('beast');

    // The X and the button at the bottom are both "Fechar".
    await dialog.getByRole('button', { name: 'Fechar', exact: true }).last().click();
    await expect(dialog).toHaveCount(0);
    await expect(help).toBeFocused();
  },
);
