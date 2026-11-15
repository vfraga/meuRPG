import { mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { expect, type Locator, type Page } from '@playwright/test';

import { brisa, combatRPC, getEncounterRPC } from './combat-support';
import { rollsTable, vex, type RollsTable } from './combat-rolls-support';
import { openSessionPage } from './live-session-support';
import { createEntryRPC, giveFeatRPC } from './content-support';
import { callRPC, openCombatLog, viaTargetList } from './support';

// The table of the take-out specs (#835): Vex, a rogue with a rapier and Sneak Attack, against a held goblin. The rapier and
// Sneak Attack are both piercing, so the player types ONE sum for the type: a typed group.

export const FACES = { Vex: 20, 'Goblin 1': 5, 'Goblin 2': 4, Brisa: 3 };

export interface TakeOutTable extends RollsTable {
  encounterId: string;
  /** Combatant ids by label. */
  id: (label: string) => string;
}

/** The table's extras of Vex: each is a feat that adds one die to every melee hit (an "extra_damage" effect), of its own damage type. */
const FEATS = {
  /** A second piercing extra: with the rapier and Sneak Attack the piercing group has three parts. */
  pierce: { namePt: 'Ferroada do Vale', damageType: 'damage-type:piercing', diceSides: 6 },
  /** A radiant extra alone in its type: taking it out is the plain take-out, with no new sum. */
  radiant: { namePt: 'Brilho do Vale', damageType: 'damage-type:radiant', diceSides: 4 },
} as const;

export interface TakeOutOptions {
  /** The extras Vex has besides Sneak Attack. */
  feats?: readonly (keyof typeof FEATS)[];
  /** A second player's character beside Vex, for a hit on a player's character. */
  ally?: boolean;
}

export async function takeOutTable(
  browser: Parameters<typeof rollsTable>[0],
  look: Parameters<typeof rollsTable>[5] = {},
  options: TakeOutOptions = {},
): Promise<TakeOutTable> {
  const t = await rollsTable(browser, 'Tirar parte', vex, { weaponKeys: ['equipment:rapier'] }, FACES, look, {
    ally: options.ally ? { build: brisa, col: 8, row: 8 } : undefined,
  });
  // The extras are table feats given to Vex before the first hit; the character is read again at each attack.
  for (const name of options.feats ?? []) {
    const f = FEATS[name];
    const key = await createEntryRPC(t.m, t.campaignId, 'tableFeat', {
      namePt: f.namePt,
      descPt: ['Um dado a mais em cada golpe de perto.'],
      effects: [{ type: 'extra_damage', applies: 'any_target', dice: '1', diceSides: f.diceSides, damageType: f.damageType, weapon: 'melee', frequency: 'each_hit' }],
    });
    await giveFeatRPC(t.m, t.campaignId, t.table.characterId, key);
  }
  const enc = await getEncounterRPC(t.m, t.campaignId);
  // The goblin is held: Vex attacks with advantage, which is what makes Sneak Attack hold.
  for (const label of options.ally ? ['Goblin 1', 'Brisa'] : ['Goblin 1']) {
    const held = enc.combatants.find((c) => c.label === label)!;
    await combatRPC(t.m, 'SetCombatantConditions', { campaignId: t.campaignId, encounterId: enc.id, combatantId: held.id, conditions: { keys: ['condition:restrained'] } });
  }
  return { ...t, encounterId: enc.id, id: (label) => enc.combatants.find((c) => c.label === label)!.id };
}

/** What one of Vex's hits marks and types (the sheet of the attack). */
export interface HitPlan {
  /** The target's label in the list of the sheet. */
  target?: string;
  /** The extras to mark by name (Sneak Attack and the feats), and those to unmark (Sneak Attack is marked on its own with advantage). */
  extras?: readonly string[];
  without?: readonly string[];
  /** The label of the field of the typed sum, and what is typed in it. */
  field?: string;
  sum?: string;
  /** Several fields, one for each damage type, when the sums are not one group's: label and what is typed. */
  fields?: readonly (readonly [string, string])[];
  /** The button that sends the damage: "Aplicar 15 de dano" for an NPC, what the sheet says for a player's character. */
  send?: string | RegExp;
  keepSheet?: boolean;
}

/** Vex hits with advantage (the first d20 is a 4, the second a 17), marks the extras and types ONE sum for the group of a damage type. */
export async function hitWith(t: TakeOutTable, plan: HitPlan = {}): Promise<Locator> {
  const { p, campaignId } = t;
  await openSessionPage(p, campaignId);
  await expect(p.getByRole('heading', { name: 'Sua vez, Vex' })).toBeVisible();
  await p.getByRole('button', { name: /^Atacar com Rapieira/ }).click();
  const sheet = p.getByRole('dialog');
  await viaTargetList(p, sheet);
  await sheet.locator('label', { hasText: plan.target ?? 'Goblin 1' }).click();
  await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
  await sheet.getByLabel(/d20 \(1º\)|Primeiro d20|1º d20/).first().fill('4');
  await sheet.getByLabel(/d20 \(2º\)|Segundo d20|2º d20/).first().fill('17');
  await sheet.getByRole('button', { name: /Confirmar/ }).click();
  for (const extra of plan.extras ?? ['Ataque Furtivo']) {
    await sheet.getByRole('checkbox', { name: new RegExp(extra) }).check();
  }
  for (const extra of plan.without ?? []) {
    await sheet.getByRole('checkbox', { name: new RegExp(extra) }).uncheck();
  }
  await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
  for (const [label, value] of plan.fields ?? [[plan.field ?? 'Soma dos dados: 1d8 + 2d6', plan.sum ?? '12']]) {
    await sheet.getByLabel(label, { exact: true }).fill(value);
  }
  await sheet.getByRole('button', { name: plan.send ?? 'Aplicar 15 de dano' }).click();
  if (!plan.keepSheet) {
    await sheet.getByRole('button', { name: /Voltar à sua vez/ }).click();
  }
  return sheet;
}

/** Vex hits Goblin 1 with advantage, marks Sneak Attack and types ONE sum (12) for the piercing; the damage is applied (15). */
export async function sneakHit(t: TakeOutTable, keepSheet = false): Promise<Locator> {
  const sheet = await hitWith(t, { keepSheet: true });
  await expect(sheet.getByText(/Ataque Furtivo/).first()).toBeVisible();
  if (!keepSheet) {
    await sheet.getByRole('button', { name: /Voltar à sua vez/ }).click();
  }
  return sheet;
}

/** The master takes Sneak Attack out of the damage by the API (the screens are shown by the other specs). */
export async function takeOutRPC(t: TakeOutTable, reason: string): Promise<void> {
  const pending = await pendingIdOf(t);
  const res = await callRPC(t.m, 'meurpg.play.v1.CombatService/RemoveDamagePart', {
    campaignId: t.campaignId,
    encounterId: t.encounterId,
    pendingDamageId: pending,
    partKey: 'sneak-attack',
    reason,
    idempotencyKey: crypto.randomUUID(),
  });
  expect(res.ok(), await res.text()).toBeTruthy();
}

/** The id of the damage the master's options list for Vex's hit. */
export async function pendingIdOf(t: TakeOutTable): Promise<string> {
  const res = await callRPC(t.m, 'meurpg.play.v1.CombatService/GetTurnOptions', { campaignId: t.campaignId, encounterId: t.encounterId, combatantId: t.id('Vex') });
  expect(res.ok(), await res.text()).toBeTruthy();
  const list = (await res.json()).pendingDamages as { id: string }[];
  expect(list.length).toBeGreaterThan(0);
  return list[0].id;
}

/** The master's card of Vex's turn: the damage with "Tirar". */
export const damageCard = (m: Page) => m.getByRole('region', { name: /^Dano de Vex$/ });

const shotsDir = process.env.E2E_SHOTS_DIR;

/** The device scale of the shots (2) when `E2E_SHOTS_DIR` is set. */
export const takeOutShotScale = shotsDir ? { deviceScaleFactor: 2 } : {};

/** Saves the viewport as `835-web-<screen>-<width>-<light|dark>.png` when `E2E_SHOTS_DIR` is set, scrolled to `at` first. */
export async function shootTakeOut(page: Page, screen: string, width: number, scheme: 'light' | 'dark', at?: Locator): Promise<void> {
  if (!shotsDir) {
    return;
  }
  mkdirSync(shotsDir, { recursive: true });
  if (at) {
    await at.evaluate((el) => {
      el.style.scrollMarginTop = '140px';
      el.scrollIntoView({ block: el.getBoundingClientRect().height < innerHeight * 0.8 ? 'center' : 'start' });
    });
  }
  await page.screenshot({ path: join(shotsDir, `835-web-${screen}-${width}-${scheme}.png`) });
}

/** What the screen must hold at every width: no sideways scroll and every button of `scope` at least 44px tall and wide enough to tap. */
export async function expectFits(page: Page, scope: Locator, screen: string): Promise<void> {
  const wide = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(wide, `${screen}: the page scrolls sideways by ${wide}px`).toBeLessThanOrEqual(0);
  const small: string[] = [];
  for (const b of await scope.getByRole('button').all()) {
    const box = await b.boundingBox();
    const name = (await b.innerText()).replace(/\s+/g, ' ').trim();
    if (box && (box.height < 44 || box.width < 44)) {
      small.push(`${name} ${Math.round(box.width)}x${Math.round(box.height)}`);
    }
  }
  expect(small, `${screen}: touch targets under 44px`).toEqual([]);
}

/** The button is dashed (the app's disabled pattern), not greyed out. */
export async function expectDashed(button: Locator): Promise<void> {
  await expect(button).toHaveCSS('border-top-style', 'dashed');
}

/** What a state flow may do at each screen: save a shot (the a11y rows) and scan it. Both are optional: the flow spec only reads. */
export interface View {
  shoot?: (page: Page, screen: string, at?: Locator) => Promise<void>;
  scan?: (page: Page, screen: string) => Promise<void>;
}

async function see(v: View, page: Page, screen: string, label: string, at?: Locator): Promise<void> {
  await v.scan?.(page, label);
  await v.shoot?.(page, screen, at);
}

/** The master opens the panel of "Tirar" on a part of the damage card and writes a reason. */
async function openPanel(m: Page, part: string, title: string, reason: string) {
  const card = damageCard(m);
  await card.getByRole('button', { name: `Tirar ${part}` }).click();
  const panel = card.getByRole('alertdialog', { name: title });
  await panel.getByLabel('Motivo (aparece no registro)').fill(reason);
  return { card, panel };
}

/** State 3: the group keeps two parts after the take-out, so the card of the one who rolled asks ONE sum of both. */
export async function twoPartsFlow(t: TakeOutTable, v: View = {}): Promise<void> {
  const { m, p, campaignId } = t;
  await hitWith(t, { extras: ['Ataque Furtivo', 'Ferroada do Vale'], field: 'Soma dos dados: 1d8 + 2d6 + 1d6', sum: '14', send: 'Aplicar 17 de dano' });
  await openSessionPage(m, campaignId);
  const { card, panel } = await openPanel(m, 'o Ataque Furtivo', 'Tirar o Ataque Furtivo do dano?', 'Sem aliado perto do alvo');
  await expect(panel.getByText(/Pede a Vex só a soma de perfurante \(1d8 \+ 1d6\)\./)).toBeVisible();
  await see(v, m, 'two-parts-confirm', 'Tirar parte, o grupo continua com duas partes, a confirmação', panel);
  await panel.getByRole('button', { name: 'Tirar e pedir a soma' }).click();
  await expect(card.getByText('Esperando Vex')).toBeVisible();
  await p.getByRole('button', { name: 'Digitar a soma' }).click();
  const sheet = p.getByRole('dialog').filter({ has: p.getByRole('heading', { name: 'Digitar o dano de novo' }) });
  const field = sheet.getByLabel('Rapieira e Ferroada do Vale, perfurante: 1d8 + 1d6', { exact: true });
  await expect(field).toBeVisible();
  await expect(field).toHaveAccessibleDescription(/De 2 a 14\./);
  await expect(sheet.getByText(/ PV/)).toHaveCount(0);
  await see(v, p, 'two-parts-roller', 'Tirar parte, a folha com a soma das duas partes', sheet);
  await field.fill('9');
  await expect(sheet.getByText('Dano: 9 + 3 = 12')).toBeVisible();
  await sheet.getByRole('button', { name: 'Aplicar 12 de dano' }).click();
  await expect(card.getByText('Esperando Vex')).toHaveCount(0);
}

/** State 2: a part alone in its damage type goes out as before ("Tirar o X? O dano cai de A para B."), with no new sum. */
export async function aloneFlow(t: TakeOutTable, v: View = {}): Promise<void> {
  const { m, p, campaignId } = t;
  await hitWith(t, { extras: ['Brilho do Vale'], without: ['Ataque Furtivo'], fields: [['Rapieira, perfurante: 1d8', '5'], ['Brilho do Vale, radiante: 1d4', '3']], send: 'Aplicar 11 de dano' });
  await openSessionPage(m, campaignId);
  const card = damageCard(m);
  await expect(card.getByText('Aplicado · 11')).toBeVisible();
  await see(v, m, 'alone-master', 'Tirar parte, uma parte sozinha no seu tipo, a carta do mestre', card);
  const { panel } = await openPanel(m, 'o Brilho do Vale', 'Tirar o Brilho do Vale? O dano cai de 11 para 8.', 'Sem brilho hoje');
  await expect(panel.getByRole('list')).toHaveCount(0);
  await see(v, m, 'alone-confirm', 'Tirar parte, uma parte sozinha, a confirmação', panel);
  await panel.getByRole('button', { name: 'Tirar', exact: true }).click();
  // No new sum: the card of the roller says nothing to type, and nothing waits on the master's bar.
  await expect(panel).toHaveCount(0);
  await expect(card.getByText('Esperando Vex')).toHaveCount(0);
  await expect(m.getByText('Falta a soma de Vex')).toHaveCount(0);
  await expect(p.getByText('Falta digitar de novo a soma do dano do seu ataque.')).toHaveCount(0);
}

/** State 4: the damage was rolled on a player's character and not applied yet, so nothing is undone. */
export async function rolledFlow(t: TakeOutTable, v: View = {}): Promise<void> {
  const { m, p, campaignId } = t;
  await hitWith(t, { target: 'Brisa', send: /Aplicar|Enviar|Mandar/ });
  await openSessionPage(m, campaignId);
  const { panel } = await openPanel(m, 'o Ataque Furtivo', 'Tirar o Ataque Furtivo do dano?', 'Sem aliado perto do alvo');
  await expect(panel.getByText(/O dano ainda não foi aplicado: nada é desfeito\. Vex digita de novo só a soma de perfurante, e o mestre aplica como sempre\./)).toBeVisible();
  await expect(panel.getByRole('list')).toHaveCount(0);
  await see(v, m, 'rolled-confirm', 'Tirar parte, rolado e ainda não aplicado, a confirmação', panel);
  await panel.getByRole('button', { name: 'Tirar e pedir a soma' }).click();
  await expect(damageCard(m).getByText('Esperando Vex')).toBeVisible();
  await expect(p.getByText('Falta digitar de novo a soma do dano do seu ataque.')).toBeVisible();
}

/** The log of another player (not the master, not the attacker's): the line says what the master took out and never why. */
export async function otherPlayerLogFlow(t: TakeOutTable, v: View = {}): Promise<void> {
  const { ally, campaignId } = t;
  const reason = 'Sem aliado perto do alvo';
  await sneakHit(t);
  await takeOutRPC(t, reason);
  const other = ally!.page;
  await openSessionPage(other, campaignId);
  const log = await openCombatLog(other);
  const line = log.getByText('O mestre tirou o Ataque Furtivo', { exact: true });
  await expect(line).toBeVisible();
  // The reason is only for the master and the attacker's player: nowhere on this page, not even hidden.
  await expect(other.getByText(/Sem aliado perto do alvo/)).toHaveCount(0);
  expect(await other.content()).not.toContain(reason);
  // The attack line is back to waiting for the damage, as the server sends it.
  await expect(log.getByText(/Vex ataca o Goblin 1 com a Rapieira: acertou, falta rolar o dano/)).toBeVisible();
  await see(v, other, 'other-player-log', 'Tirar parte, o registro de outra jogadora', log);
}
