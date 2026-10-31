import { expect, type Browser, type BrowserContext, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { beginAttackCombatRPC, getEncounterRPC, pensantusAttacks, tableForCombat, type CombatTable } from './combat-support';
import { endOpenSessionRPC, openSessionPage, openSheetTab } from './live-session-support';
import { callRPC, newSignedInContext, viaTargetList } from './support';

// The effects that last, on the player's screens (W7-E, RN-22): "Seus efeitos" and the exhaustion card on the live sheet,
// what a paralysing effect takes from the turn, the saving throw of the end of the turn, and the d4 of Bênção typed from a
// physical die. The table and the effects come through the API; what is under test is what the player reads (RN-10, RN-20).

const playerFirst = { Pensantus: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 };

interface EffectsTable {
  m: Page;
  p: Page;
  table: CombatTable;
  campaignId: string;
  encounterId: string;
  me: string;
  captain: string;
  done: () => Promise<void>;
}

/** A combat with Pensantus (the player's, first), the Capitão and two Goblins; the player is on a 390 px phone. */
async function effectsTable(browser: Browser, name: string, width = 390, hidden: string[] = []): Promise<EffectsTable> {
  const master: BrowserContext = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
  const player: BrowserContext = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width, height: 844 } });
  const m = await master.newPage();
  const p = await player.newPage();
  await m.goto('/');
  await p.goto('/');
  const table = await tableForCombat(m, p, `${name} ${Date.now()}`, true, true, { sheet: pensantusAttacks });
  const enc = await beginAttackCombatRPC(m, table, playerFirst, undefined, hidden);
  return {
    m,
    p,
    table,
    campaignId: table.campaignId,
    encounterId: enc.id,
    me: enc.combatants.find((c) => c.label === 'Pensantus')!.id,
    captain: enc.combatants.find((c) => c.label === 'Capitão Goblin')!.id,
    done: async () => {
      await endOpenSessionRPC(m, table.campaignId);
      await master.close();
      await player.close();
    },
  };
}

/** The master puts an effect of the catalog on Pensantus, visible to the players. */
async function addEffectRPC(t: EffectsTable, catalogKey: string, caster?: string): Promise<void> {
  const res = await callRPC(t.m, 'meurpg.play.v1.LastingEffectService/AddLastingEffect', {
    campaignId: t.campaignId,
    encounterId: t.encounterId,
    idempotencyKey: crypto.randomUUID(),
    targetIds: [t.me],
    catalogKey,
    ...(caster ? { casterId: caster } : {}),
    duration: { kind: 'EFFECT_DURATION_KIND_ROUNDS', rounds: 10 },
    playerVisible: true,
    audience: 'EFFECT_AUDIENCE_ALL',
  });
  expect(res.ok(), await res.text()).toBeTruthy();
}

/** The page does not scroll sideways: nothing is wider than the screen. Polled, like any layout read after a change. */
async function expectNoSideScroll(page: Page): Promise<void> {
  await expect
    .poll(() => page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth))
    .toBeLessThanOrEqual(0);
}

/**
 * Resizes the page and waits until the page itself has the new width. `setViewportSize` returns before the renderer has
 * laid the page out at the new size: in CI (run 38160388327) the first frame after a 390 → 320 resize was still painted
 * with the 390 px layout, and a bounding box read 12 to 42 ms later came from it.
 */
async function resizeTo(page: Page, width: number, height: number): Promise<void> {
  await page.setViewportSize({ width, height });
  await expect.poll(() => page.evaluate(() => document.documentElement.clientWidth)).toBe(width);
}

test(
  '"Seus efeitos" e a exaustão aparecem na ficha do jogador, com o nível, sem CD e sem rolar de lado, no celular',
  { tag: ['@W7-E', '@RN-22', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const t = await effectsTable(browser, 'Seus efeitos');
    try {
      await addEffectRPC(t, 'spell:bless');
      const set = await callRPC(t.m, 'meurpg.play.v1.LastingEffectService/SetExhaustion', {
        campaignId: t.campaignId,
        idempotencyKey: crypto.randomUUID(),
        characterId: t.table.characterId,
        level: 4,
        expectedLevel: 0,
      });
      expect(set.ok(), await set.text()).toBeTruthy();
      await openSessionPage(t.p, t.campaignId);
      const sheet = await openSheetTab(t.p);

      // The card of the effect: its name, whose it is, the label and the clock.
      await expect(t.p.getByRole('heading', { name: 'Seus efeitos' })).toBeVisible();
      const card = t.p.getByRole('group', { name: 'Bênção' });
      await expect(card).toBeVisible();
      await expect(card).toContainText('+1d4 em ataques e resistências');
      await expect(card).toContainText(/Restam \d+ rodadas/);
      await expect(t.p.getByText(/\bCD\b/)).toHaveCount(0);

      // The exhaustion: the label in the header and a card with the lines up to the level.
      // The combat's own compact card (hidden under "Ficha") has the same label: the one in "Ficha" is the one read.
      await expect(sheet.locator('app-effect-pill', { hasText: 'Exaustão 4' })).toBeVisible();
      const exhaustion = t.p.getByRole('group', { name: 'Nível 4' });
      await expect(exhaustion).toContainText('Definida pelo mestre. Cada nível soma aos de baixo.');
      await expect(exhaustion).toContainText('PV máximos pela metade');
      await expect(exhaustion).not.toContainText('Deslocamento 0');
      await expectNoSideScroll(t.p);

      // At 320 px the cards stack and nothing scrolls sideways.
      await resizeTo(t.p, 320, 568);
      await expect(card).toBeVisible();
      await expectNoSideScroll(t.p);
    } finally {
      await t.done();
    }
  },
);

test(
  'um efeito que paralisa tira a ação do turno, e o teste do fim do turno abre sozinho, sem a CD, com três respostas',
  { tag: ['@W7-E', '@RN-22', '@RN-20'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const t = await effectsTable(browser, 'Teste do fim do turno');
    try {
      await addEffectRPC(t, 'spell:hold-person', t.captain);
      await openSessionPage(t.p, t.campaignId);

      // The turn: the server's sentence, with no cause for the paralysis; the Capitão is on the player's order, so the card names him.
      const note = t.p.getByTestId('effect-note');
      await expect(note).toContainText(/Você está Paralisad[oa]\./);
      await expect(note).not.toContainText('Capitão');
      await expect(t.p.getByText('Você não pode agir.').first()).toBeVisible();
      await expect(t.p.getByRole('group', { name: 'Imobilizar Pessoa' })).toContainText('De Capitão Goblin');

      // Nothing of the turn is available, so the shapes of the Ação, the Bônus and the Reação say "indisponível" and ending the turn does not ask.
      await expect(t.p.getByRole('listitem', { name: 'Ação indisponível', exact: true })).toBeVisible();
      await expect(t.p.getByRole('listitem', { name: 'Ação disponível', exact: true })).toHaveCount(0);

      // Ending the turn opens the saving throw: the ability, the modifier and the three answers, never a DC.
      await t.p.getByRole('button', { name: 'Encerrar turno' }).click();
      await expect(t.p.getByText('Ainda tem ação disponível. Encerrar mesmo?')).toHaveCount(0);
      const sheet = t.p.getByRole('dialog', { name: 'Teste de resistência do fim do turno' });
      await expect(sheet.getByRole('heading', { name: 'Fim do seu turno' })).toBeVisible();
      await expect(sheet).toContainText('Teste de resistência de Sabedoria.');
      await expect(sheet).toContainText('Seu modificador:');
      await expect(sheet).not.toContainText(/\bCD\b/);
      const app = sheet.getByRole('button', { name: 'Rolar no app' });
      const typed = sheet.getByRole('button', { name: 'Digitar o resultado' });
      const hand = sheet.getByRole('button', { name: 'Deixar o mestre rolar por mim' });
      for (const b of [app, typed, hand]) {
        await expect(b).toBeVisible();
      }
      await expect(sheet.getByRole('heading', { name: 'Fim do seu turno' })).toBeFocused();

      // At 320 px the three answers stack, full width, and nothing scrolls sideways.
      await resizeTo(t.p, 320, 568);
      const boxes = [await app.boundingBox(), await typed.boundingBox(), await hand.boundingBox()];
      expect(boxes.every((b) => b !== null && b.x >= 0 && b.x + b.width <= 320)).toBeTruthy();
      expect(boxes[1]!.y).toBeGreaterThan(boxes[0]!.y);
      expect(boxes[2]!.y).toBeGreaterThan(boxes[1]!.y);
      await expectNoSideScroll(t.p);

      // Pensantus is a Rock Gnome: Gnome Cunning gives advantage on a Wisdom save against magic, and the effect is a
      // spell's, so the save asks for two d20 (SRD 5.1). A 20 typed from a physical die passes: the effect ends and the
      // sheet says so.
      await typed.click();
      await sheet.getByLabel('Primeiro d20').fill('20');
      await sheet.getByLabel('Segundo d20').fill('3');
      await sheet.getByRole('button', { name: /Confirmar/ }).click();
      await expect(sheet).toContainText('Passou');
      await expect(sheet).toContainText('Imobilizar Pessoa acabou.');
      await sheet.getByRole('button', { name: 'Fechar', exact: true }).last().click();
      await expect(t.p.getByRole('group', { name: 'Imobilizar Pessoa' })).toHaveCount(0);
    } finally {
      await t.done();
    }
  },
);

test(
  'a Bênção pede o d4 ao lado do d20 com dados físicos e o resultado mostra o dado somado',
  { tag: ['@W7-E', '@RN-22', '@RN-18'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const t = await effectsTable(browser, 'Bênção em dados físicos');
    try {
      await addEffectRPC(t, 'spell:bless');
      await openSessionPage(t.p, t.campaignId);
      await t.p.getByRole('button', { name: 'Atacar com Raio de Fogo' }).click();
      const sheet = t.p.getByRole('dialog', { name: 'Atacar com Raio de Fogo' });
      await viaTargetList(t.p, sheet);
      await sheet.locator('label', { hasText: 'Capitão Goblin' }).click();
      await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();

      // The d4 of Bênção has its own field, named and with what it does; the roll is not sent without it.
      const d4 = sheet.getByLabel('Resultado do d4 (Bênção)');
      await expect(d4).toBeVisible();
      await expect(sheet).toContainText('Bênção soma 1d4 a esta jogada. Role um d4 além do d20.');
      await sheet.getByLabel(/Role 1d20 para Raio de Fogo/).fill('14');
      await sheet.getByRole('button', { name: 'Confirmar 14' }).click();
      await expect(sheet.getByRole('alert')).toContainText('Digite o resultado do d4 antes de confirmar.');
      await expectNoSideScroll(t.p);

      await d4.fill('3');
      await sheet.getByRole('button', { name: 'Confirmar 14' }).click();
      await expect(sheet).toContainText('+ 1d4 (3)');
      await expect(sheet).toContainText('Bênção');
    } finally {
      await t.done();
    }
  },
);

test(
  'a ordem lista a condição uma vez e nenhuma frase do jogador nomeia quem conjurou o efeito',
  { tag: ['@W7-E', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const t = await effectsTable(browser, 'Rótulos na ordem', 390, ['Capitão Goblin']);
    try {
      await addEffectRPC(t, 'spell:hold-person', t.captain);
      await openSessionPage(t.p, t.campaignId);
      // The Capitão is hidden from the players: the card never names him.
      await expect(t.p.getByRole('group', { name: 'Imobilizar Pessoa' })).toContainText('De alguém que você não vê');
      const enc = await getEncounterRPC(t.m, t.campaignId);
      expect(enc.combatants.find((c) => c.id === t.me)?.conditions ?? []).not.toHaveLength(0);
      // The order lists the condition once, and no sentence names the caster.
      await expect(t.p.getByText('Paralisado', { exact: true }).first()).toBeVisible();
      await expect(t.p.getByText(/Imobilizar Pessoa de|do Capitão/)).toHaveCount(0);
    } finally {
      await t.done();
    }
  },
);
