import { mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { expect, type Locator, type Page } from '@playwright/test';
import { callRPC } from './support';
import { pensantusCasting, type Combatant, type Encounter } from './combat-support';

/** Support of the "Preparar" screens (#582): the Ready sheet, the readied card and, for the master, the order row and the fold. */

/** The readied action as the JSON of a `Combatant` carries it (the master's and its owner's copy only). */
export interface ReadiedJson {
  id: string;
  triggerText: string;
  kind: string;
  actionKey?: string;
  actionNamePt?: string;
  targetCombatantId?: string;
  moveFt?: number;
  status: string;
}

/** The readied action of a combatant, or `undefined` for one that holds none (the server sends the waiting row only). */
export function readiedOf(c: Combatant): ReadiedJson | undefined {
  return (c as Combatant & { readied?: ReadiedJson }).readied;
}

/** A write of `ReadyService` (`ReadyAction`, `FireReadied`, `CancelReadied`), answered with the combat as the caller sees it. */
export async function readyRPC(page: Page, method: 'ReadyAction' | 'FireReadied' | 'CancelReadied', body: object): Promise<Encounter> {
  const res = await callRPC(page, `meurpg.play.v1.ReadyService/${method}`, { idempotencyKey: crypto.randomUUID(), ...body });
  expect(res.ok(), await res.text()).toBeTruthy();
  return (await res.json()).encounter as Encounter;
}

/**
 * What Pensantus carries in the sheet specs of the Ready action: the casting list (Mísseis Mágicos, Sono, Teia, Mãos Flamejantes, which the
 * window cannot release yet) plus Enfeitiçar Pessoa, a spell of one creature, which it can.
 */
export const pensantusReadyKit = {
  ...pensantusCasting,
  knownSpellKeys: [...pensantusCasting.knownSpellKeys, 'spell:charm-person'],
  preparedSpellKeys: [...pensantusCasting.preparedSpellKeys, 'spell:charm-person'],
};

/** Why Mover is off in the sheet, and why a spell of darts, several targets or a point is (the window answers with a creature only). */
export const moveOffText = 'Mover pela janela ainda não dá: por enquanto o mestre move a ficha.';
export const spellOffText = 'Soltar pela janela ainda não dá para esta magia: ela pede dardos, alvos ou um ponto no mapa.';

/** The trigger of the specs: the zombie of the boards, with the goblin of the table. */
export const goblinTrigger = 'Quando o goblin passar pela porta';

/** Opens the Ready sheet from the "Outras" tab of the player's turn and returns it. */
export async function openReadySheet(page: Page): Promise<Locator> {
  const other = page.getByRole('tabpanel', { name: /^Outras\b/ });
  await page.getByRole('tablist', { name: 'Tipos de ação' }).getByRole('tab', { name: /^Outras( \d+)?(,|$)/ }).click();
  await other.getByRole('button', { name: 'Preparar uma ação' }).click();
  const sheet = page.getByRole('dialog', { name: 'Preparar uma ação' });
  await expect(sheet).toBeVisible();
  return sheet;
}

/** The folder the screenshots go to when `E2E_SHOTS_DIR` names one; otherwise nothing is written. */
const shotsDir = process.env.E2E_SHOTS_DIR;

/** Saves the viewport as `582-<screen>-<width>-<claro|escuro>.png` when `E2E_SHOTS_DIR` is set; a normal run writes nothing. */
export async function shootReady(page: Page, screen: string, width: number, scheme: 'light' | 'dark'): Promise<void> {
  if (!shotsDir) {
    return;
  }
  mkdirSync(shotsDir, { recursive: true });
  await page.evaluate(() => document.fonts.ready);
  await page.screenshot({ path: join(shotsDir, `582-${screen}-${width}-${scheme === 'light' ? 'claro' : 'escuro'}.png`) });
}

/** The player's own call of `ReadyAction`: the held attack, with the trigger of the boards (the spec has no need to drive the sheet again). */
export async function readyAttackRPC(player: Page, campaignId: string, encounter: Encounter, who: string, target?: string): Promise<Encounter> {
  const id = (label: string): string => encounter.combatants.find((c) => c.label === label)!.id;
  return readyRPC(player, 'ReadyAction', {
    campaignId,
    encounterId: encounter.id,
    combatantId: id(who),
    triggerText: goblinTrigger,
    kind: 'READIED_KIND_ATTACK',
    actionKey: 'equipment:dagger',
    ...(target ? { targetCombatantId: id(target) } : {}),
  });
}

/** The player's own call of `ReadyAction` for a spell of one action held with a slot of its level (no target named when `target` is left out). */
export async function readySpellRPC(player: Page, campaignId: string, encounter: Encounter, who: string, spellKey: string, slotLevel: number, target?: string): Promise<Encounter> {
  const id = (label: string): string => encounter.combatants.find((c) => c.label === label)!.id;
  return readyRPC(player, 'ReadyAction', {
    campaignId,
    encounterId: encounter.id,
    combatantId: id(who),
    triggerText: goblinTrigger,
    kind: 'READIED_KIND_SPELL',
    spellKey,
    slotLevel,
    ...(target ? { targetCombatantId: id(target) } : {}),
  });
}
