import { expect, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { campaignWithEmptyPlayer, method } from './table-rules-support';
import { callRPC, newSignedInContext } from './support';

test.describe.configure({ timeout: 120_000 });

// RN-33 (class and race choices): the "Escolhas" step of the editor. A player does not create a sheet with a choice
// open; the step shows what the race and the class ask, with the rule of each option, and the options that cannot be
// taken yet stay in the list, dotted, with the reason. Every test makes its own campaign through the API.

/** Opens a `mat-select` by its label and picks an option (keyboard, as `support.ts` does). */
async function pick(page: Page, label: string, option: string): Promise<void> {
  const control = page.getByRole('combobox', { name: label, exact: true });
  await control.focus();
  await control.press('Enter');
  await page.getByRole('option', { name: option, exact: true }).click();
  await expect(control).toHaveAttribute('aria-expanded', 'false');
}

/** The sheet's stored picks, as the server holds them. */
async function storedPicks(page: Page, campaignId: string): Promise<string[]> {
  const list = await callRPC(page, 'meurpg.characters.v1.CharacterService/GetCharacter', {
    campaignId,
    characterId: page.url().split('/').pop()!,
  });
  expect(list.ok(), await list.text()).toBeTruthy();
  return (await list.json()).character.sheet.full.featureChoiceKeys as string[];
}

test(
  'um draconato guerreiro escolhe o ancestral e o estilo de luta, e só então cria o personagem @RN-33',
  { tag: '@RN-33' },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Draconato ${Date.now()}`);

      await p.goto(`/campaigns/${campaignId}/characters/new`);
      await p.getByLabel('Nome do personagem', { exact: true }).fill('Tharn');
      await pick(p, 'Raça', 'Draconato');
      await pick(p, 'Classe', 'Guerreiro');
      await pick(p, 'Antecedente', 'Acólito');
      await p.getByRole('tab', { name: 'Habilidades' }).click();
      await method(p, 'Digitar');

      // The step appears once the race and the class are known, marked as pending in words.
      const tab = p.getByRole('tab', { name: /Escolhas/ });
      await expect(tab).toBeVisible();
      await expect(tab).toContainText('Escolha pendente');

      // Two choices are open: "Criar personagem" says so, stays reachable, and leads to the first one.
      const create = p.getByRole('button', { name: 'Criar personagem' });
      await expect(create).toHaveAttribute('aria-disabled', 'true');
      await expect(create).toHaveAccessibleDescription(/Faltam escolhas: .*Estilo de Luta/);
      // The Acolyte's two languages are open too, but they only warn: the sentence under the button does not name them.
      await expect(create).not.toHaveAccessibleDescription(/Idiomas/);
      await create.focus();
      await p.keyboard.press('Enter');
      await expect(p).toHaveURL(/characters\/new$/);
      await expect(p.getByRole('heading', { level: 3, name: /Ancestral Dracônico/ })).toBeFocused();
      await expect(p.getByText(/Escolhas feitas: 0 de 4/)).toBeVisible();

      const ancestry = p.getByRole('region', { name: /Ancestral Dracônico/ });
      await ancestry.getByRole('radio', { name: /Vermelho/ }).click();
      await expect(ancestry.getByRole('radio', { name: /Vermelho/ })).toBeChecked();
      const style = p.getByRole('region', { name: /Estilo de Luta/ });
      await style.getByRole('radio', { name: /Defesa/ }).click();
      await expect(p.getByText(/Escolhas feitas: 2 de 4/)).toBeVisible();

      // The two choices that stop the sheet are made: the button creates, and the step stays marked while the two
      // languages of the Acolyte are open.
      await expect(create).toHaveAttribute('aria-disabled', 'false');
      await expect(tab).toContainText('Escolha pendente');
      const languages = p.getByRole('region', { name: /^Idiomas/ });
      await languages.getByRole('checkbox', { name: /^Anão/ }).click();
      await languages.getByRole('checkbox', { name: /^Gnômico/ }).click();
      await expect(p.getByText(/Escolhas feitas: 4 de 4/)).toBeVisible();

      // Nothing left open: the marker is gone, and the button creates.
      await expect(tab).not.toContainText('Escolha pendente');
      await expect(create).toHaveAttribute('aria-disabled', 'false');
      await create.click();

      await expect(p).toHaveURL(/\/campaigns\/[^/]+\/characters\/(?!new$)[^/]+$/);
      const picks = JSON.stringify(await storedPicks(p, campaignId));
      expect(picks).toContain('red');
      expect(picks).toContain('defense');
    } finally {
      await master.close();
      await player.close();
    }
  },
);

/** True when the page sends no preview request (`PreviewChoices`, `PreviewCharacter`) for `ms`. */
function previewQuietFor(page: Page, ms: number): Promise<boolean> {
  return page.waitForRequest(/PreviewChoices|PreviewCharacter/, { timeout: ms }).then(
    () => false,
    () => true,
  );
}

/** Waits out the asks already on their way: true once the page has been quiet for `ms`, false if it never is in `rounds`. */
async function previewSettles(page: Page, ms: number, rounds = 3): Promise<boolean> {
  for (let i = 0; i < rounds; i++) {
    if (await previewQuietFor(page, ms)) {
      return true;
    }
  }
  return false;
}

test(
  'o editor não pergunta de novo ao servidor quando nada mudou: a resposta igual não dispara outra pergunta @RN-33',
  { tag: '@RN-33' },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Sem laco ${Date.now()}`);
      await p.goto(`/campaigns/${campaignId}/characters/new`);
      await p.getByLabel('Nome do personagem', { exact: true }).fill('Brenna');
      await pick(p, 'Raça', 'Humano');
      await pick(p, 'Classe', 'Guerreiro');
      await expect(p.getByRole('tab', { name: /Escolhas/ })).toBeVisible();
      // The draft's first round of asks settles (a pause and a call each), and with no input it asks no more.
      expect(await previewSettles(p, 1000)).toBe(true);
      expect(await previewQuietFor(p, 2500)).toBe(true);
    } finally {
      await master.close();
      await player.close();
    }
  },
);

test(
  'dois idiomas iguais escolhidos em sequência, antes de o servidor responder, não passam: o repetido nem é oferecido @RN-33',
  { tag: '@RN-33' },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Repetido ${Date.now()}`);

      await p.goto(`/campaigns/${campaignId}/characters/new`);
      await p.getByLabel('Nome do personagem', { exact: true }).fill('Brenna');
      await pick(p, 'Raça', 'Humano');
      await pick(p, 'Classe', 'Guerreiro');
      await pick(p, 'Antecedente', 'Acólito');
      await p.getByRole('tab', { name: 'Habilidades' }).click();
      await method(p, 'Digitar');
      await p.getByRole('tab', { name: /Escolhas/ }).click();
      await expect(p.getByText(/Escolhas feitas: 0 de 4/)).toBeVisible();

      // The server's answers are held: what the page knows is only what the player did.
      let release: () => void = () => undefined;
      const held = new Promise<void>((resolve) => (release = resolve));
      await p.route(/PreviewChoices/, async (route) => {
        await held;
        await route.continue();
      });

      const extra = p.getByRole('region', { name: /^Idioma(?!s)/ });
      const acolyte = p.getByRole('region', { name: /^Idiomas/ });
      await extra.getByRole('radio', { name: /^Abissal/ }).click();
      // The language the race took is not offered to the background, before it can be clicked.
      const repeated = acolyte.getByRole('checkbox', { name: /^Abissal/ });
      await expect(repeated).toHaveAttribute('aria-disabled', 'true');
      await expect(repeated).toContainText('Você já escolheu esta opção.');
      // A click on the disabled option does nothing (the event, since a disabled control takes no pointer click).
      await repeated.dispatchEvent('click');
      await expect(repeated).not.toBeChecked();
      await acolyte.getByRole('checkbox', { name: /^Anão/ }).click();
      // And the other way round: what the background took is not offered to the race.
      await expect(extra.getByRole('radio', { name: /^Anão/ })).toHaveAttribute('aria-disabled', 'true');
      release();
      await p.getByRole('region', { name: /Estilo de Luta/ }).getByRole('radio', { name: /Defesa/ }).click();
      await acolyte.getByRole('checkbox', { name: /^Celestial/ }).click();
      await expect(p.getByText(/Escolhas feitas: 4 de 4/)).toBeVisible();

      await p.getByRole('button', { name: 'Criar personagem' }).click();
      await expect(p).toHaveURL(/\/campaigns\/[^/]+\/characters\/(?!new$)[^/]+$/);
      expect(await storedPicks(p, campaignId)).toEqual(
        expect.arrayContaining([
          'race:human#language=language:abyssal',
          'background:acolyte#language=language:dwarvish',
          'background:acolyte#language=language:celestial',
        ]),
      );
    } finally {
      await master.close();
      await player.close();
    }
  },
);

test(
  'um humano escolhe o idioma extra e os dois do Acólito no passo Escolhas, e a ficha lista os três @RN-33',
  { tag: '@RN-33' },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Idiomas ${Date.now()}`);

      await p.goto(`/campaigns/${campaignId}/characters/new`);
      await p.getByLabel('Nome do personagem', { exact: true }).fill('Brenna');
      await pick(p, 'Raça', 'Humano');
      await pick(p, 'Classe', 'Guerreiro');
      await pick(p, 'Antecedente', 'Acólito');
      await p.getByRole('tab', { name: 'Habilidades' }).click();
      await method(p, 'Digitar');

      // The Fighting Style stops the sheet; the languages only warn, and the sentence under the button says so.
      const create = p.getByRole('button', { name: 'Criar personagem' });
      await expect(create).toHaveAccessibleDescription(/Falta uma escolha: Estilo de Luta/);
      await expect(create).not.toHaveAccessibleDescription(/Idioma/);

      await p.getByRole('tab', { name: /Escolhas/ }).click();
      await expect(p.getByText(/Escolhas feitas: 0 de 4/)).toBeVisible();
      await p.getByRole('region', { name: /Estilo de Luta/ }).getByRole('radio', { name: /Defesa/ }).click();

      // The human's extra language: a pick-one list from the catalog's languages, the one the human speaks dotted.
      const extra = p.getByRole('region', { name: /^Idioma(?!s)/ });
      await expect(extra.getByRole('radio', { name: /^Comum/ })).toHaveAttribute('aria-disabled', 'true');
      await extra.getByRole('radio', { name: /^Anão/ }).click();
      await expect(extra.getByRole('radio', { name: /^Anão/ })).toBeChecked();

      // The Acolyte's two: the language the first pick took is dotted here, and the third pick is refused.
      const acolyte = p.getByRole('region', { name: /^Idiomas/ });
      await expect(acolyte.getByRole('checkbox', { name: /^Anão/ })).toHaveAttribute('aria-disabled', 'true');
      await acolyte.getByRole('checkbox', { name: /^Élfico/ }).click();
      await acolyte.getByRole('checkbox', { name: /^Gigante/ }).click();
      await expect(acolyte.getByRole('checkbox', { name: /^Gnômico/ })).toHaveAttribute('aria-disabled', 'true');
      await expect(p.getByText(/Escolhas feitas: 4 de 4/)).toBeVisible();

      await create.click();
      await expect(p).toHaveURL(/\/campaigns\/[^/]+\/characters\/(?!new$)[^/]+$/);
      const picks = await storedPicks(p, campaignId);
      expect(picks).toEqual(
        expect.arrayContaining([
          'race:human#language=language:dwarvish',
          'background:acolyte#language=language:elvish',
          'background:acolyte#language=language:giant',
        ]),
      );

      // The sheet lists the race's language and the three picks.
      const line = p.locator('dl.more > div').filter({ has: p.locator('dt', { hasText: 'Idiomas' }) }).locator('dd');
      for (const name of ['Comum', 'Anão', 'Élfico', 'Gigante']) {
        await expect(line).toContainText(name);
      }
    } finally {
      await master.close();
      await player.close();
    }
  },
);

test(
  'um bruxo de nível 5 escolhe a dádiva e três invocações, e a que ainda não pode ser escolhida fica na lista com o motivo @RN-33',
  { tag: '@RN-33' },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Bruxo ${Date.now()}`);

      await p.goto(`/campaigns/${campaignId}/characters/new`);
      await p.getByLabel('Nome do personagem', { exact: true }).fill('Mirela');
      await pick(p, 'Raça', 'Humano');
      await pick(p, 'Classe', 'Bruxo');
      await p.getByLabel('Nível', { exact: true }).fill('5');
      await pick(p, 'Subclasse', 'O Corruptor');
      await pick(p, 'Antecedente', 'Acólito');
      await p.getByRole('tab', { name: 'Habilidades' }).click();
      await method(p, 'Digitar');

      await p.getByRole('tab', { name: /Escolhas/ }).click();
      const pact = p.getByRole('region', { name: /Dádiva do Pacto/ });
      await pact.getByRole('radio', { name: /Pacto da Lâmina/ }).click();

      // Three invocations at level 5. One of them asks for a level the warlock has not reached: it stays in the
      // list, dotted (aria-disabled, still focusable), and its card says why.
      const invocations = p.getByRole('region', { name: /Invocações Místicas/ });
      const blocked = invocations.getByRole('checkbox', { name: /Passo Ascendente/ });
      await expect(blocked).toHaveAttribute('aria-disabled', 'true');
      await blocked.focus();
      await expect(blocked).toBeFocused();
      await expect(blocked).toContainText('9');
      await p.keyboard.press('Space');
      await expect(blocked).toHaveAttribute('aria-checked', 'false');

      for (const name of [/Armadura de Sombras/, /Idioma Bestial/, /Visão Diabólica/]) {
        await invocations.getByRole('checkbox', { name }).click();
      }
      await expect(p.getByRole('status').filter({ hasText: 'Já escolheu 3.' })).toBeVisible();

      // The human's language and the Acolyte's two are choices of the step too.
      await p.getByRole('region', { name: /^Idioma(?!s)/ }).getByRole('radio', { name: /^Anão/ }).click();
      const languages = p.getByRole('region', { name: /^Idiomas/ });
      await languages.getByRole('checkbox', { name: /^Élfico/ }).click();
      await languages.getByRole('checkbox', { name: /^Gnômico/ }).click();
      await expect(p.getByRole('tab', { name: /Escolhas/ })).not.toContainText('Escolha pendente');

      await p.getByRole('button', { name: 'Criar personagem' }).click();
      await expect(p).toHaveURL(/\/campaigns\/[^/]+\/characters\/(?!new$)[^/]+$/);
      const picks = JSON.stringify(await storedPicks(p, campaignId));
      for (const key of ['armor-of-shadows', 'beast-speech', 'devils-sight', 'blade']) {
        expect(picks).toContain(key);
      }
      expect(picks).not.toContain('ascendant-step');
    } finally {
      await master.close();
      await player.close();
    }
  },
);
