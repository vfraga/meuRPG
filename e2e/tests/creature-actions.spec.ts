import { expect, type Browser, type Locator, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { combatRPC, passTurnsTo, tableForCombat, type Encounter } from './combat-support';
import { vex } from './combat-rolls-support';
import { endOpenSessionRPC, openSessionPage } from './live-session-support';
import { masterLog } from './combat-layout';
import { callRPC, newSignedInContext, openCombatLog } from './support';

// The master's creature actions that are no attack roll (a breath weapon, a claw's rider), as the master and the player read them.
// The table and the combat come through the API, in the mind's theatre (the master names the targets, no map to pick on); the d20s
// are typed, so the order of the turns never depends on a roll. A saving throw's outcome is random, so the specs assert that the
// line exists and what the player must never read (the stat block's text, the DC), not whether the target saved.

/** A combat with one monster of the SRD against Vex, the monster first in the order, and its turn open. */
async function monsterTurn(browser: Browser, name: string, creatureKey: string) {
  const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 1000 } });
  const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 390, height: 844 } });
  const m = await master.newPage();
  const p = await player.newPage();
  await m.goto('/');
  await p.goto('/');
  const table = await tableForCombat(m, p, `${name} ${Date.now()}`, false, false, { build: vex, sheet: { weaponKeys: ['equipment:rapier'] } });
  const campaignId = table.campaignId;
  const started = await callRPC(m, 'meurpg.play.v1.CombatService/StartEncounter', {
    campaignId,
    idempotencyKey: crypto.randomUUID(),
    name: 'Emboscada na estrada',
    participants: [],
    monsters: [{ creatureKey, count: 1 }],
    mode: 'ENCOUNTER_MODE_THEATRE',
  });
  expect(started.ok(), await started.text()).toBeTruthy();
  let enc = (await started.json()).encounter as Encounter;
  const monsterIds = new Set(enc.combatants.filter((c) => (c as { bestiaryCreatureKey?: string }).bestiaryCreatureKey).map((c) => c.id));
  for (const c of enc.combatants) {
    enc = await combatRPC(m, 'SubmitInitiative', { campaignId, encounterId: enc.id, combatantId: c.id, d20Face: monsterIds.has(c.id) ? 20 : 1 });
  }
  enc = await combatRPC(m, 'BeginCombat', { campaignId, encounterId: enc.id });
  const monster = enc.combatants.find((c) => monsterIds.has(c.id))!;
  // A monster starts hidden, and the players read nothing of a hidden creature's turn: show it, as the master does at the ambush.
  await combatRPC(m, 'SetCombatantHidden', { campaignId, encounterId: enc.id, combatantId: monster.id, hidden: false });
  await passTurnsTo(m, campaignId, monster.label);
  await openSessionPage(p, campaignId);
  await openSessionPage(m, campaignId);
  const card = m.getByRole('region', { name: new RegExp(`^(Ações|Vez) d[ao] ${monster.label}$`) });
  return {
    m,
    p,
    card,
    done: async () => {
      await endOpenSessionRPC(m, campaignId);
      await master.close();
      await player.close();
    },
  };
}

/** The text of the combat log, for the audience of the page (the player's sheet or the master's fold, opened first). */
const openLogs = new WeakMap<Page, Locator>();
async function logText(page: Page, master: boolean): Promise<string> {
  // Opened once per page: on a phone the log is a sheet, and its opener is gone while it is open.
  let log = openLogs.get(page);
  if (!log) {
    log = master ? await masterLog(page) : await openCombatLog(page);
    openLogs.set(page, log);
  }
  return (await log.textContent()) ?? '';
}

test('o sopro do cão infernal: o mestre usa a ação, o resultado aparece no cartão e o jogador lê a salvaguarda dele sem o texto do SRD nem a CD', { tag: ['@MR-012', '@RN-20'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const { p, card, done } = await monsterTurn(browser, 'Sopro', 'monster:hell-hound');
  try {
    const use = card.getByRole('button', { name: /^Usar Sopro de fogo/ });
    await expect(use).toBeVisible();
    await expect(card.getByRole('group', { name: /Quem a ação atinge/ })).toBeVisible();
    // The card names the action and its usage in Portuguese: "Sopro de fogo", "Recarga 5–6", no "Salvaguarda" and no English name.
    const before = (await card.innerText()).replace(/\s+/g, ' ');
    expect(before).toContain('Sopro de fogo');
    expect(before).toContain('Recarga 5–6');
    expect(before).not.toContain('Fire Breath');
    expect(before).not.toMatch(/Salvaguarda/i);
    await use.click();
    const result = card.getByTestId('creature-action-result');
    await expect(result).toBeVisible();
    // The save is random: the line says it passed or failed, and the master reads the roll against the DC.
    await expect(result).toContainText(/Passou|Falhou|Falha automática/);
    await expect(result).toContainText('Teste de resistência');
    expect((await card.innerText()).replace(/\s+/g, ' ')).not.toMatch(/Salvaguarda|Fire Breath/i);
    // The action is spent.
    await expect(use).toBeDisabled();
    // The player's log carries the action and the save, never the stat block's words or the DC.
    const playerLog = await openCombatLog(p);
    await expect(playerLog).toContainText(/usou Sopro de fogo/);
    const log = (await playerLog.textContent()) ?? '';
    expect(log).toMatch(/Vex (falhou|resistiu)/);
    expect(log).not.toMatch(/Fire Breath|Salvaguarda/i);
    expect(log).not.toContain('Texto do SRD');
    expect(log).not.toMatch(/\bCD\b/);
  } finally {
    await done().catch(() => undefined);
  }
});

test('a mordida da carniçal acerta e oferece o efeito das garras: o mestre aplica e a oferta some', { tag: ['@MR-012'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const { m, card, done } = await monsterTurn(browser, 'Garras', 'monster:ghoul');
  try {
    // A typed hit (a d20 of 15), the damage rolled and applied; the claw then offers its rider.
    await card.locator('label', { hasText: 'Garras' }).click();
    const claws = card.getByRole('button', { name: 'Digitar o resultado' });
    await claws.click();
    await card.getByLabel(/Role 1d20/).fill('15');
    await card.getByRole('button', { name: 'Confirmar 15' }).click();
    await card.getByRole('button', { name: 'Rolar dano' }).click();
    await card.getByRole('button', { name: /Aplicar \d+ de dano/ }).click();
    const offer = m.getByTestId('hit-rider').first();
    await expect(offer).toBeVisible();
    await offer.getByRole('button', { name: /^Aplicar / }).click();
    await expect(offer).toBeHidden();
    await expect.poll(() => logText(m, true)).toMatch(/usou Garras/);
  } finally {
    await done().catch(() => undefined);
  }
});


test('a mordida da aranha: o dano de veneno do efeito chega ao registro do mestre e ao do jogador alvo', { tag: ['@MR-012', '@RN-20'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const { m, p, card, done } = await monsterTurn(browser, 'Veneno', 'monster:giant-spider');
  try {
    // A typed hit (a d20 of 15), the bite's damage rolled and applied; the bite then offers its poison rider.
    await card.locator('label', { hasText: 'Mordida' }).click();
    await card.getByRole('button', { name: 'Digitar o resultado' }).click();
    await card.getByLabel(/Role 1d20/).fill('15');
    await card.getByRole('button', { name: 'Confirmar 15' }).click();
    await card.getByRole('button', { name: 'Rolar dano' }).click();
    await card.getByRole('button', { name: /Aplicar \d+ de dano/ }).click();
    const offer = m.getByTestId('hit-rider').first();
    await expect(offer).toBeVisible();
    await offer.getByRole('button', { name: /^Aplicar / }).click();
    await expect(offer).toBeHidden();
    // The save is random, the poison damage is not: it is rolled whether the save passed (half) or failed. The rider's damage is a
    // line of its own on the log, awaiting the roll, then rolled, then applied; the master and the target's player read each stage.
    await expect.poll(() => logText(m, true)).toMatch(/usou Mordida em Vex: Vex (passou|falhou) no teste.*falta rolar o dano/);
    await expect.poll(() => logText(p, false)).toMatch(/usou Mordida em Vex: Vex (passou|falhou) no teste.*falta rolar o dano/);
    await card.getByRole('button', { name: 'Rolar dano' }).click();
    const rolled = /usou Mordida em Vex: Vex (passou|falhou) no teste \([^)]*\)[^)]*\), \d+ de dano/;
    await expect.poll(() => logText(m, true)).toMatch(rolled);
    expect(await logText(m, true)).toMatch(/contra\sCD\s11/); // only the master reads the DC
    await expect.poll(() => logText(p, false)).toMatch(rolled);
    await card.getByRole('button', { name: /Aplicar \d+ de dano/ }).click();
    await expect.poll(() => logText(m, true)).not.toMatch(/esperando o mestre aplicar/);
    for (const text of [await logText(m, true), await logText(p, false)]) {
      expect(text).toMatch(rolled);
      expect(text).not.toContain('falta rolar o dano');
      expect(text).not.toMatch(/Poison|poison|Bite|Salvaguarda/);
    }
    // The player never reads the DC.
    expect(await logText(p, false)).not.toMatch(/\bCD\b/);
  } finally {
    await done().catch(() => undefined);
  }
});
