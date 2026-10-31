import { expect, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { endOpenSessionRPC } from './live-session-support';
import { newSignedInContext, pensantus, showAllPicks } from './support';
import { classCard, markMilestoneRPC, passClassStep, tableForLevelUp, toren } from './levelup-support';

// MR-040 (the guided level-up: Habilidades, Vida, Magias, Resumo; the master sees "O que mudou"), RN-01 (the
// sheet stays locked: only the level's choices change), RN-12 (who can level up). The data comes through
// the API; the screens are what is under test. Every test makes its own campaign.

const sheetOf = (campaignId: string, characterId: string) => `/campaigns/${campaignId}/characters/${characterId}`;

/** Clicks the row of a pick list by the name on it. */
// `label.row`, not `.row`: the "O que muda até aqui" panel has `li.row` rows too (a saving throw that changes), and
// once the server's preview is in, `.first()` would pick one of them.
const row = (page: Page, name: string) => page.locator('.row__main, label.row').filter({ hasText: name }).first();

test(
  'Pensantus sobe do Mago 3 para o 4: habilidade, vida média, um truque, duas magias e duas para preparar; o mestre vê "O que mudou"',
  { tag: ['@MR-040', '@RN-01', '@RN-12'] },
  async ({ browser }) => {
    test.setTimeout(150_000);
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 800 } });
    const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 800 } });
    const m = await master.newPage();
    const p = await player.newPage();
    let campaignId = '';
    try {
      await m.goto('/');
      await p.goto('/');
      const table = await tableForLevelUp(m, p, `Subida de Pensantus ${Date.now()}`);
      campaignId = table.campaignId;

      // The locked sheet says why, and has the one filled button.
      await p.goto(sheetOf(campaignId, table.characterId));
      await expect(p.getByRole('heading', { name: 'Pensantus pode subir de nível' })).toBeVisible();
      await expect(p.getByText('O mestre marcou “Chegar ao Vale Seco”.')).toBeVisible();
      await p.getByRole('link', { name: 'Subir para o nível 4' }).click();

      // Habilidades: +2 in Inteligência, and what it changes, from the server.
      await expect(p.getByRole('heading', { name: 'Subir para o nível 4' })).toBeVisible();
      await passClassStep(p);
      await expect(p.getByText('Passo 2 de 5 · Habilidades')).toBeVisible();
      await expect(p.getByText('Falta escolher 1 habilidade.')).toBeVisible();
      await row(p, 'Inteligência').click();
      await expect(p.getByText('18 → 20')).toBeVisible();
      await expect(p.getByRole('heading', { name: 'O que muda com Inteligência 20' })).toBeVisible();
      await p.getByRole('button', { name: 'Próximo' }).click();

      // Vida: the average is already chosen.
      await expect(p.getByText('Passo 3 de 5 · Vida')).toBeVisible();
      await expect(p.getByText('Média: 4')).toBeVisible();
      await expect(p.getByText('4 + Constituição +3 · de 23 para 30')).toBeVisible();
      await p.getByRole('button', { name: 'Próximo' }).click();

      // Magias: a cantrip, two spells for the book, two to prepare. A missing choice blocks "Próximo".
      await expect(p.getByText('Passo 4 de 5 · Magias')).toBeVisible();
      await expect(p.getByText('Falta escolher 1 truque.')).toBeVisible();
      // "Próximo" is aria-disabled but still answers a tap: the focus goes to the first missing choice.
      await p.getByRole('button', { name: 'Próximo' }).click({ force: true });
      await expect(p.locator('#pick-cantrips').getByRole('radio').first()).toBeFocused();
      await expect(p.getByText('Passo 4 de 5 · Magias')).toBeVisible();
      await p.getByRole('button', { name: /Ver os outros \d+ truques/ }).click();
      await row(p, 'Prestidigitação').click();
      await p.getByLabel('Buscar magia').fill('nebuloso');
      await row(p, 'Passo Nebuloso').first().click();
      await p.getByLabel('Buscar magia').fill('reflexos');
      await row(p, 'Reflexos').first().click();
      await p.getByLabel('Buscar magia').fill('');
      await expect(p.getByText('Faltam preparar 2 magias.')).toBeVisible();
      const prepare = p.locator('#pick-prepared');
      await prepare.locator('.row__main').filter({ hasText: 'Passo Nebuloso' }).click();
      await prepare.locator('.row__main').filter({ hasText: 'Detectar Magia' }).click();
      await expect(prepare).toContainText('9 de 9');
      await p.getByRole('button', { name: 'Próximo' }).click();

      // Resumo: everything before → after, and the rest stays locked.
      await expect(p.getByText('Passo 5 de 5 · Resumo')).toBeVisible();
      const summary = p.getByRole('region', { name: 'O que muda', exact: true });
      await expect(summary).toContainText('Mago 3');
      await expect(summary).toContainText('Mago 4');
      await expect(summary).toContainText('23');
      await expect(summary).toContainText('30');
      await expect(summary).toContainText('CD das magias');
      await expect(p.getByText('O resto da ficha não muda e continua travado.')).toBeVisible();
      await p.getByRole('button', { name: 'Confirmar o nível 4' }).click();

      // The sheet: level 4, the new numbers, a status the player dismisses, and no tag any more.
      await expect(p).toHaveURL(new RegExp(`/characters/${table.characterId}$`));
      await expect(p.getByRole('status').filter({ hasText: 'Pensantus subiu para o nível 4. O mestre foi avisado.' })).toBeVisible();
      await expect(p.getByRole('definition').filter({ hasText: 'Mago 4' })).toBeVisible();
      await expect(p.locator('app-ability-medallions')).toContainText('20');
      await expect(p.getByText('Pode subir de nível')).toHaveCount(0);
      await expect(p.getByRole('link', { name: 'Subir para o nível 4' })).toHaveCount(0);
      await expect(p.getByLabel('Estado do personagem')).toContainText('Travada');
      await p.getByRole('button', { name: 'Dispensar o aviso' }).click();
      await expect(p.getByText('Pensantus subiu para o nível 4.')).toHaveCount(0);

      // The master is told, and sees what was chosen.
      await m.goto(`/campaigns/${campaignId}`);
      await expect(m.getByRole('status').filter({ hasText: 'Pensantus subiu para o nível 4.' })).toBeVisible();
      await m.getByRole('button', { name: 'O que mudou: Pensantus' }).click();
      const changes = m.getByRole('region', { name: 'O que Pensantus escolheu no nível 4' });
      await expect(changes).toContainText('+2 em Inteligência');
      await expect(changes).toContainText('Prestidigitação');
      await expect(changes).toContainText('Passo Nebuloso e Reflexos');
      await expect(changes).toContainText('Confirmado em');
      await expect(changes).toContainText('Nada fica à espera do seu OK');
      await expect(changes.getByRole('link', { name: 'Abrir a ficha' })).toBeVisible();
    } finally {
      await endOpenSessionRPC(m, campaignId);
      await master.close();
      await player.close();
    }
  },
);

test(
  'Toren sobe do Guerreiro 4 para o 5: só Vida e Resumo, e rolar o dado no app guarda o resultado',
  { tag: ['@MR-040', '@RN-12', '@RN-18'] },
  async ({ browser }) => {
    test.setTimeout(120_000);
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 390, height: 844 } });
    const m = await master.newPage();
    const p = await player.newPage();
    let campaignId = '';
    try {
      await m.goto('/');
      await p.goto('/');
      const table = await tableForLevelUp(m, p, `Subida de Toren ${Date.now()}`, { build: toren });
      campaignId = table.campaignId;
      await p.goto(sheetOf(campaignId, table.characterId));
      await p.getByRole('link', { name: 'Subir para o nível 5' }).click();

      await passClassStep(p);
      await expect(p.getByText('Passo 2 de 3 · Vida')).toBeVisible();
      await expect(p.getByText('Neste nível não há mais nada para escolher')).toBeVisible();
      await expect(p.getByText('Ataque Extra')).toBeVisible();
      await expect(p.getByText('Média: 6')).toBeVisible();

      await p.locator('.dice-choice__card').filter({ hasText: 'Rolar 1d10' }).click();
      await expect(p.getByText('Falta rolar o dado de vida.')).toBeVisible();
      await p.getByRole('button', { name: 'Rolar no app' }).click();
      await expect(p.getByText(/Rolado no app: \d+ no d10/)).toBeVisible();
      await p.getByRole('button', { name: 'Próximo' }).click();

      await expect(p.getByText('Passo 3 de 3 · Resumo')).toBeVisible();
      await expect(p.getByRole('region', { name: 'O que muda', exact: true })).toContainText('Dados de vida');
      await p.getByRole('button', { name: 'Confirmar o nível 5' }).click();
      await expect(p.getByRole('status').filter({ hasText: 'Toren subiu para o nível 5.' })).toBeVisible();
      await expect(p.getByRole('definition').filter({ hasText: 'Guerreiro 5' })).toBeVisible();

      // The master's list keeps the roll on record.
      await m.goto(`/campaigns/${campaignId}`);
      await m.getByRole('button', { name: 'O que mudou: Toren' }).click();
      await expect(m.getByRole('region', { name: 'O que Toren escolheu no nível 5' })).toContainText('Rolado no app');
    } finally {
      await endOpenSessionRPC(m, campaignId);
      await master.close();
      await player.close();
    }
  },
);

test(
  'quem não pode subir de nível não tem o botão, e a rota responde como a de uma ficha travada',
  { tag: ['@MR-040', '@RN-01', '@RN-12'] },
  async ({ browser }) => {
    test.setTimeout(90_000);
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    const m = await master.newPage();
    const p = await player.newPage();
    let campaignId = '';
    try {
      await m.goto('/');
      await p.goto('/');
      const table = await tableForLevelUp(m, p, `Sem marco ${Date.now()}`, { milestone: false });
      campaignId = table.campaignId;

      await p.goto(sheetOf(campaignId, table.characterId));
      await expect(p.getByLabel('Estado do personagem')).toContainText('Travada');
      await expect(p.getByRole('link', { name: /Subir para o nível/ })).toHaveCount(0);
      await expect(p.getByText('Pode subir de nível')).toHaveCount(0);

      await p.goto(`${sheetOf(campaignId, table.characterId)}/level-up`);
      await expect(p.getByRole('heading', { name: 'Ainda não dá para subir de nível' })).toBeVisible();
      await expect(p.getByText('Falta o mestre marcar um marco para este personagem')).toBeVisible();
      await expect(p.getByRole('link', { name: 'Voltar para a ficha' }).last()).toBeVisible();
      await expect(p.getByText(/Passo \d de \d/)).toHaveCount(0);

      // The master has no button either: the player levels up, the master edits the sheet.
      await m.goto(`${sheetOf(campaignId, table.characterId)}/level-up`);
      await expect(m.getByText('Quem sobe o nível é o jogador')).toBeVisible();

      // Once the master marks a milestone, the same route opens.
      await markMilestoneRPC(m, campaignId, 'Chegar ao Vale Seco', [table.characterId]);
      await p.goto(`${sheetOf(campaignId, table.characterId)}/level-up`);
      await expect(p.getByText('Passo 1 de 5 · Classe')).toBeVisible();
    } finally {
      await endOpenSessionRPC(m, campaignId);
      await master.close();
      await player.close();
    }
  },
);

test(
  'Corvina, Mago 3 e Clérigo 1, escolhe qual classe sobe: a primeira vem marcada, trocar depois de escolher pergunta, e o nível lê "o nível 2 de Clérigo" @MR-040 @RN-12',
  { tag: ['@MR-040', '@RN-12'] },
  async ({ browser }) => {
    test.setTimeout(150_000);
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 800 } });
    const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 800 } });
    const m = await master.newPage();
    const p = await player.newPage();
    let campaignId = '';
    try {
      await m.goto('/');
      await p.goto('/');
      const table = await tableForLevelUp(m, p, `Subida multiclasse ${Date.now()}`, {
        build: { ...pensantus, name: 'Corvina', level: 3, scores: { ...pensantus.scores, sab: 14 } },
        sheet: {
          classes: [
            { classKey: 'class:wizard', level: 3, subclassKey: 'subclass:evocation' },
            { classKey: 'class:cleric', level: 1, subclassKey: 'subclass:life' },
          ],
          preparedSpellKeys: ['spell:magic-missile', 'spell:shield', 'spell:mage-armor', 'spell:burning-hands', 'spell:sleep', 'spell:scorching-ray', 'spell:web', 'spell:bless', 'spell:cure-wounds'],
        },
      });
      campaignId = table.campaignId;
      await p.goto(`/campaigns/${campaignId}/characters/${table.characterId}/level-up`);

      // The class step opens the flow: the first class of the sheet is marked, and the server is read with no class.
      await expect(p.getByRole('heading', { name: 'Subir para o nível 5' })).toBeVisible();
      await expect(p.getByRole('heading', { name: 'Subir em qual classe?' })).toBeVisible();
      const mago = p.getByRole('radio', { name: /^Mago/ });
      const clerigo = p.getByRole('radio', { name: /^Clérigo/ });
      await expect(mago).toBeChecked();
      await expect(mago).toBeFocused();
      await expect(p.getByText('Corvina · Mago 3 → Mago 4')).toBeVisible();

      // Something chosen: the other class asks first, and "Continuar" keeps everything.
      await p.getByRole('button', { name: 'Próximo' }).click();
      await row(p, 'Inteligência').click();
      await p.getByRole('button', { name: 'Voltar' }).click();
      await p.locator('app-class-pick label.card').filter({ hasText: 'Clérigo' }).click();
      const ask = p.getByRole('group', { name: 'Trocar de classe?' });
      await expect(ask).toBeVisible();
      await expect(ask.getByRole('button', { name: 'Continuar com o Mago' })).toBeFocused();
      await ask.getByRole('button', { name: 'Continuar com o Mago' }).click();
      await expect(ask).toBeHidden();
      await expect(mago).toBeChecked();
      await expect(p.getByText('Corvina · Mago 3 → Mago 4')).toBeVisible();

      // "Trocar": the draft goes, the options of the Clérigo are read and the level lines name the class.
      await p.locator('app-class-pick label.card').filter({ hasText: 'Clérigo' }).click();
      await ask.getByRole('button', { name: 'Trocar para o Clérigo' }).click();
      await expect(clerigo).toBeChecked();
      await expect(p.getByText('Corvina · Clérigo 1 → Clérigo 2')).toBeVisible();
      await expect(p.getByRole('status').filter({ hasText: /^Clérigo escolhido\. O nível tem \d passos\./ })).toBeAttached();
      await p.getByRole('button', { name: 'Próximo' }).click();
      await expect(p.getByText(/Só o que o nível 2 de Clérigo dá fica aberto\./)).toBeVisible();
      await expect(p.getByRole('heading', { name: 'O que o nível 2 de Clérigo dá' })).toBeVisible();
      await expect(p.getByRole('heading', { name: 'Subir para o nível 5' })).toBeVisible();
    } finally {
      await endOpenSessionRPC(m, campaignId);
      await master.close();
      await player.close();
    }
  },
);

// SRD 5.1 "Multiclassing": Doran, Guerreiro 5 (Força 16, Destreza 14, Inteligência 13, Carisma 9), takes the
// Mago at level 1. The class step opens "Uma classe nova" with each class's prerequisite (the Bardo is closed
// with the reason), the Vida step says the die of the new class, the summary says why each number moved, and the
// footer asks in place before the class that does not come undone is added.
// A phone prepares its spells here the same way a desktop does.
for (const { width, height } of [
  { width: 1280, height: 800 },
  { width: 390, height: 844 },
]) {
  test(
    `Doran, Guerreiro 5, sobe em uma classe nova: o pré-requisito à vista, a pergunta antes de acrescentar o Mago 1 e a ficha com as duas classes, a ${width} px @MR-040 @RN-12`,
    { tag: ['@MR-040', '@RN-12'] },
    async ({ browser }) => {
      test.setTimeout(180_000);
      const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width, height } });
      const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width, height } });
      const m = await master.newPage();
      const p = await player.newPage();
      let campaignId = '';
      try {
        await m.goto('/');
        await p.goto('/');
        const doran = { ...toren, name: 'Doran', level: 5, scores: { for: 15, des: 13, con: 13, int: 12, sab: 10, car: 8 } };
        const table = await tableForLevelUp(m, p, `Subida em classe nova ${Date.now()}`, { build: doran });
        campaignId = table.campaignId;
        await p.goto(`/campaigns/${campaignId}/characters/${table.characterId}/level-up`);

        await expect(p.getByRole('heading', { name: 'Subir em qual classe?' })).toBeVisible();
        await expect(p.getByRole('radio', { name: /^Guerreiro/ })).toBeChecked();
        await classCard(p, /^Uma classe nova/).click();
        await expect(p.getByRole('heading', { name: 'Qual classe nova?' })).toBeVisible();
        await expect(p.getByText('Você cumpre o pré-requisito do Guerreiro')).toBeVisible();
        // Closed, in words: the Bardo asks for Carisma 13 and Doran has 9. It stays focusable and cannot be chosen.
        const bardo = p.getByRole('radio', { name: /^Bardo/ });
        await expect(bardo).toHaveAttribute('aria-disabled', 'true');
        await expect(classCard(p, /^Bardo/)).toContainText('Falta: Carisma 13 (você tem 9).');
        // It keeps the focus (aria-disabled, not disabled) and the keyboard cannot choose it either.
        await bardo.focus();
        await p.keyboard.press('Space');
        await expect(bardo).not.toBeChecked();
        // "Próximo" waits for the class.
        await expect(p.getByText('Escolha a classe nova para continuar.')).toBeVisible();
        await expect(p.getByText('Exige Inteligência 13. Você tem Inteligência 13.')).toBeVisible();
        await classCard(p, /^Mago/).click();
        await expect(p.getByText('Doran · Guerreiro 5 → Guerreiro 5 · Mago 1')).toBeVisible();
        await expect(p.getByText(/Passo 1 de 4 · Classe/)).toBeVisible();
        await p.getByRole('button', { name: 'Próximo' }).click();

        // Vida: the die of the new class, never the maximum.
        await expect(p.getByRole('heading', { name: 'Pontos de vida de Mago 1' })).toBeVisible();
        await expect(p.getByText('O nível 1 de Mago dá um d6, e o personagem vai ao nível total 6.')).toBeVisible();
        await expect(p.getByText('Média: 4')).toBeVisible();
        await p.getByRole('button', { name: 'Próximo' }).click();

        // Magias: the counts of the Mago's level 1, not the total level's.
        await expect(p.getByText('Passo 3 de 4 · Magias')).toBeVisible();
        const cantrips = p.locator('#pick-cantrips');
        await showAllPicks(cantrips);
        for (const name of ['Raio de Fogo', 'Mãos Mágicas', 'Ilusão Menor']) {
          await cantrips.getByRole('checkbox', { name: new RegExp(`^${name}`) }).check();
        }
        const spells = p.locator('#pick-spells');
        await showAllPicks(spells);
        for (const name of ['Mísseis Mágicos', 'Sono', 'Escudo Arcano', 'Armadura Arcana', 'Detectar Magia', 'Identificação']) {
          await spells.getByRole('checkbox', { name: new RegExp(`^${name}`) }).check();
        }
        const prepare = p.locator('#pick-prepared');
        // Six spells in the book, four rows shown: the button appears once the page has repainted the list.
        await showAllPicks(prepare, 2);
        for (const name of ['Mísseis Mágicos', 'Escudo Arcano']) {
          await prepare.getByRole('checkbox', { name: new RegExp(`^${name}`) }).check();
        }
        await p.getByRole('button', { name: 'Próximo' }).click();

        // Resumo: the numbers from the server, and why.
        await expect(p.getByText('Passo 4 de 4 · Resumo')).toBeVisible();
        const summary = p.getByRole('region', { name: 'O que muda', exact: true });
        await expect(summary).toContainText('Nível total');
        await expect(summary).toContainText('Guerreiro 5 · Mago 1');
        await expect(summary).toContainText('5d10 + 1d6');
        await expect(summary).toContainText('Ficam separados por tipo.');
        await expect(summary).toContainText('Pelo nível total (6): não muda.');
        await expect(summary).toContainText('Proficiências novas');
        // The book is six spells every time, not the count of a preview that has not seen the last pick.
        await expect(summary).toContainText(/Livro de magias[\s\S]*?—\s*para\s*→\s*6\b/);
        await expect(summary).toContainText('Só a primeira classe dá equipamento.');

        // The question in place: "Voltar" first, and nothing is saved until the class is confirmed.
        await p.getByRole('button', { name: 'Confirmar o nível 6' }).click();
        const question = p.getByRole('alertdialog', { name: 'Subir em Mago 1?' });
        await expect(question).toContainText('Isso acrescenta uma classe nova à ficha: Mago 1, e o nível total vai a 6. Não se desfaz.');
        await expect(question.getByRole('button', { name: 'Voltar' })).toBeFocused();
        await question.getByRole('button', { name: 'Voltar' }).click();
        await expect(question).toBeHidden();
        await p.getByRole('button', { name: 'Confirmar o nível 6' }).click();
        await p.getByRole('button', { name: 'Subir em Mago 1' }).click();

        await expect(p).toHaveURL(sheetOf(campaignId, table.characterId));
        await expect(p.getByText('Doran subiu para o nível 6.')).toBeVisible();
        await expect(p.getByText(/Guerreiro 5/).first()).toBeVisible();
        await expect(p.getByText(/Mago 1/).first()).toBeVisible();
        // Back on the route: nothing more to level up with this milestone.
        await p.goto(`${sheetOf(campaignId, table.characterId)}/level-up`);
        await expect(p.getByRole('heading', { name: 'Ainda não dá para subir de nível' })).toBeVisible();
      } finally {
        await endOpenSessionRPC(m, campaignId);
        await master.close();
        await player.close();
      }
    },
  );
}
