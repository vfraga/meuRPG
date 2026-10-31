import { expect, type Browser, type BrowserContext, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { beginAttackCombatRPC, tableForCombat } from './combat-support';
import { endOpenSessionRPC, openSessionPage } from './live-session-support';
import { newSignedInContext, openActionTab } from './support';

// The target of an attack or a spell is tapped on the map (C7-04 on a phone, C7-05 at 1280): "Atacar" and "Conjurar" bring the map into
// view, the creatures the server offers are ringed buttons, a tap names the target, "Confirmar" opens the same sheet at the roll, and
// "Escolher na lista" is the sheet's own list. Pensantus stands at column 6, row 8 of the 20-column road; the dagger reaches 6 m
// (18 m thrown, with disadvantage) and Raio de Fogo 36 m.

const PHONE = { width: 390, height: 844 };
const LAPTOP = { width: 1280, height: 900 };
const playerFirst = { Pensantus: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 };
// Goblin 2 is 65 ft from Pensantus: past the dagger's long range, within Raio de Fogo's.
const farGoblin: Record<string, [number, number]> = { 'Capitão Goblin': [11, 5], 'Goblin 1': [9, 9], 'Goblin 2': [19, 0] };

interface Acting {
  m: Page;
  p: Page;
  campaignId: string;
  done: () => Promise<void>;
}

/** A combat with Pensantus on turn, the Capitão and two Goblins on the map. */
async function acting(browser: Browser, name: string, viewport = PHONE, at?: Record<string, [number, number]>): Promise<Acting> {
  const master: BrowserContext = await newSignedInContext(browser, 'Mestre Teste', { viewport: LAPTOP });
  const player: BrowserContext = await newSignedInContext(browser, 'Jogador Teste', { viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  await m.goto('/');
  await p.goto('/');
  const table = await tableForCombat(m, p, `${name} ${Date.now()}`, true, true);
  await beginAttackCombatRPC(m, table, playerFirst, at);
  await openSessionPage(p, table.campaignId);
  await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
  return {
    m,
    p,
    campaignId: table.campaignId,
    done: async () => {
      await endOpenSessionRPC(m, table.campaignId);
      await master.close();
      await player.close();
    },
  };
}

const asking = (p: Page) => p.getByRole('region', { name: 'Escolha do alvo' });
const ring = (p: Page, who: string) => p.getByRole('button', { name: new RegExp(`^(Escolher|Pôr outro dardo em) ${who}`) });

test('o jogador toca o alvo no mapa: Adaga em Goblin 1, Confirmar abre a folha na rolagem e o ataque acontece', { tag: ['@MR-012', '@RN-18'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const { p, done } = await acting(browser, 'Alvo no mapa');
  try {
    await p.getByRole('button', { name: 'Atacar com Adaga' }).click();
    // The map comes into view with its ring, and the bar asks; the sheet is not open.
    await expect(asking(p)).toBeVisible();
    await expect(asking(p)).toContainText('Escolha o alvo · Adaga');
    await expect(asking(p)).toContainText('Toque num inimigo com anel.');
    await expect(p.getByRole('dialog')).toHaveCount(0);
    await expect(p.getByRole('button', { name: 'Confirmar' })).toHaveAttribute('aria-disabled', 'true');
    await expect(p.getByText('Escolhendo o alvo')).toBeVisible();
    await expect(p.locator('app-combat-map-card')).toBeInViewport();
    // Goblin 1 is within reach; Goblin 2 is far: the dagger throws that far, with disadvantage.
    await ring(p, 'Goblin 1').click();
    await expect(asking(p)).toContainText('Adaga em Goblin 1');
    await expect(asking(p)).toContainText('+5 para acertar');
    await expect(p.getByRole('button', { name: 'Confirmar' })).toBeEnabled();
    await p.getByRole('button', { name: 'Confirmar' }).click();
    // The sheet names the target before the roll, as it always did.
    const sheet = p.getByRole('dialog', { name: 'Atacar com Adaga' });
    await expect(sheet.locator('.chosen')).toContainText('Goblin 1');
    await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
    await sheet.getByLabel(/Role 1d20 para Adaga/).fill('20');
    await sheet.getByRole('button', { name: 'Confirmar 20' }).click();
    await expect(sheet.getByText(/Acertou|Crítico/).first()).toBeVisible();
  } finally {
    await done();
  }
});

test('o jogador conjura Raio de Fogo tocando no alvo do mapa', { tag: ['@MR-014', '@RN-18'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const { p, done } = await acting(browser, 'Raio de Fogo no mapa');
  try {
    await p.getByRole('button', { name: 'Atacar com Raio de Fogo' }).click();
    await expect(asking(p)).toContainText('Escolha o alvo · Raio de Fogo');
    await ring(p, 'Capitão Goblin').click();
    await ring(p, 'Goblin 2').click();
    await expect(asking(p)).toContainText('Raio de Fogo em Goblin 2');
    await p.getByRole('button', { name: 'Confirmar' }).click();
    const sheet = p.getByRole('dialog', { name: 'Atacar com Raio de Fogo' });
    await expect(sheet.locator('.chosen')).toContainText('Goblin 2');
    await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
    await sheet.getByLabel(/Role 1d20 para Raio de Fogo/).fill('20');
    await sheet.getByRole('button', { name: 'Confirmar 20' }).click();
    await expect(sheet.getByText(/Acertou|Crítico/).first()).toBeVisible();
  } finally {
    await done();
  }
});

test('Mísseis Mágicos: cada toque põe um dardo, Desfazer tira o último e Confirmar abre a folha com os dardos postos', { tag: ['@MR-014', '@RN-02'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const { p, done } = await acting(browser, 'Mísseis no mapa');
  try {
    await openActionTab(p, 'Magias');
    await p.getByRole('button', { name: 'Conjurar Mísseis Mágicos' }).click();
    await expect(asking(p)).toContainText('Mísseis Mágicos · 0 de 3 dardos');
    await expect(asking(p)).toContainText('Cada toque põe um dardo; um alvo pode levar vários.');
    await ring(p, 'Goblin 1').click();
    await ring(p, 'Goblin 1').click();
    await expect(asking(p)).toContainText('Mísseis Mágicos · 2 de 3 dardos');
    await expect(asking(p).getByRole('status')).toHaveText('2 de 3 dardos');
    await expect(p.getByRole('button', { name: 'Confirmar' })).toHaveAttribute('aria-disabled', 'true');
    await p.getByRole('button', { name: 'Desfazer' }).click();
    await expect(asking(p)).toContainText('1 de 3 dardos');
    await ring(p, 'Capitão Goblin').click();
    await ring(p, 'Capitão Goblin').click();
    await expect(asking(p)).toContainText('3 de 3 dardos');
    await expect(p.getByRole('button', { name: 'Confirmar' })).toBeEnabled();
    await p.getByRole('button', { name: 'Confirmar' }).click();
    const sheet = p.getByRole('dialog', { name: 'Conjurar Mísseis Mágicos' });
    await expect(sheet.getByText('3 de 3 dardos distribuídos.')).toBeVisible();
    await sheet.getByRole('button', { name: 'Conjurar Mísseis Mágicos' }).click();
    await expect(sheet.getByRole('heading', { name: 'Você conjurou Mísseis Mágicos' })).toBeVisible();
  } finally {
    await done();
  }
});

test('Cancelar volta à lista de ações, com o foco no botão que começou a escolha e nada aberto', { tag: ['@MR-012'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const { p, done } = await acting(browser, 'Cancelar alvo');
  try {
    const attack = p.getByRole('button', { name: 'Atacar com Adaga' });
    await attack.click();
    await expect(asking(p)).toBeVisible();
    await expect(asking(p).getByText('Escolha o alvo · Adaga')).toBeFocused();
    await p.getByRole('button', { name: 'Cancelar' }).click();
    await expect(asking(p)).toHaveCount(0);
    await expect(p.getByRole('dialog')).toHaveCount(0);
    await expect(attack).toBeFocused();
    // The turn bar is back, with its one "Encerrar turno".
    await expect(p.getByRole('button', { name: 'Encerrar turno' })).toBeVisible();
  } finally {
    await done();
  }
});

test('um alvo apagado diz por que não vale, ao toque: fora de alcance', { tag: ['@MR-012', '@RN-21'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const { p, done } = await acting(browser, 'Alvo fora de alcance', PHONE, farGoblin);
  try {
    await p.getByRole('button', { name: 'Atacar com Adaga' }).click();
    await expect(asking(p)).toBeVisible();
    // A dimmed one still has a name, with its reason, and a tap on it chooses nothing.
    const far = p.getByRole('button', { name: 'Goblin 2: fora de alcance' });
    // It is aria-disabled on purpose (it chooses nothing) yet it takes the tap that says why: the click is sent as it is.
    await far.dispatchEvent('click');
    // The reason is in the bar's hint line, and on the map too where a free place is left.
    await expect(asking(p)).toContainText('Goblin 2: fora de alcance');
    await expect(asking(p)).toContainText('Escolha o alvo · Adaga');
    await expect(p.getByRole('button', { name: 'Confirmar' })).toHaveAttribute('aria-disabled', 'true');
  } finally {
    await done();
  }
});

test('Escolher na lista abre a folha com a lista de alvos de sempre e o ataque se completa por ela', { tag: ['@MR-012', '@RN-18'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const { p, done } = await acting(browser, 'Alvo pela lista');
  try {
    await p.getByRole('button', { name: 'Atacar com Raio de Fogo' }).click();
    await asking(p).getByRole('button', { name: 'Escolher na lista' }).click();
    const sheet = p.getByRole('dialog', { name: 'Atacar com Raio de Fogo' });
    await expect(sheet.getByRole('heading', { name: 'Atacar com Raio de Fogo' })).toBeFocused();
    await expect(asking(p)).toHaveCount(0);
    await sheet.locator('label', { hasText: 'Goblin 1' }).click();
    await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
    await sheet.getByLabel(/Role 1d20 para Raio de Fogo/).fill('20');
    await sheet.getByRole('button', { name: 'Confirmar 20' }).click();
    await expect(sheet.getByText(/Acertou|Crítico/).first()).toBeVisible();
  } finally {
    await done();
  }
});

test('a 1280: o mapa ganha o destaque, o cartão da ação pendente fica no trilho e o alvo longe diz "Longe: desvantagem"', { tag: ['@MR-012', '@RN-21'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const { p, done } = await acting(browser, 'Alvo no trilho', LAPTOP);
  try {
    await p.getByRole('button', { name: 'Atacar com Adaga' }).click();
    const card = asking(p);
    await expect(card).toContainText('Escolha o alvo no mapa');
    await expect(card.getByRole('button', { name: 'Escolher na lista' })).toBeVisible();
    await expect(p.getByText('Toque no alvo')).toBeVisible();
    await expect(p.getByText('Escolhendo o alvo')).toBeVisible();
    // The turn bar keeps its one "Mover" and its one "Encerrar turno".
    await expect(p.getByRole('button', { name: 'Mover' })).toHaveCount(1);
    await expect(p.getByRole('button', { name: 'Encerrar turno' })).toHaveCount(1);
    // Goblin 2 is past the dagger's reach and within its long range: valid, with the catch said on the map.
    await expect(p.getByRole('button', { name: /Goblin 2.*Longe: desvantagem/ })).toBeVisible();
    await expect(p.locator('.cm__far').first()).toContainText('Longe');
    await ring(p, 'Goblin 1').click();
    await expect(card).toContainText('Adaga em Goblin 1');
    await card.getByRole('button', { name: 'Confirmar' }).click();
    await expect(p.getByRole('dialog', { name: 'Atacar com Adaga' }).locator('.chosen')).toContainText('Goblin 1');
  } finally {
    await done();
  }
});
