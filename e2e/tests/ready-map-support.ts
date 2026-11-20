import { expect, type Browser, type BrowserContext, type Locator, type Page } from '@playwright/test';
import { beginAttackCombatRPC, combatRPC, getEncounterRPC, passTurnsToAndWait, pensantusCasting, tableForCombat, type Encounter } from './combat-support';
import { featShotScale } from './feat-options-support';
import { endOpenSessionRPC, openSessionPage } from './live-session-support';
import { goblinTrigger, readyRPC, readySpellRPC } from './ready-support';
import { waitForEncounterRevision } from './revision-support';
import { callRPC, newSignedInContext, pensantus } from './support';

/**
 * Support of the screens that release a readied spell or move on the map (#857, #856, boards 857-01 to 857-09): a table where Pensantus
 * holds a Bola de Fogo or a move and the master has fired it, with both screens open on the session.
 */

/** Pensantus at level 5 with Bola de Fogo (a 3rd-level slot) beside the casting list of the other specs. */
export const fireballKit = {
  build: { ...pensantus, level: 5 },
  sheet: {
    ...pensantusCasting,
    knownSpellKeys: [...pensantusCasting.knownSpellKeys, 'spell:fireball'],
    preparedSpellKeys: [...pensantusCasting.preparedSpellKeys, 'spell:fireball'],
  },
};

/** The squares: Pensantus well in range of the three goblins, who stand close together so the ball reaches all of them. */
export const mapSquares: Record<string, [number, number]> = { Pensantus: [3, 6], 'Capitão Goblin': [12, 3], 'Goblin 1': [12, 7], 'Goblin 2': [13, 8] };
export const mapFaces = { Pensantus: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 };

/** What the player's page and the master's page say of the window, in the words of the boards. */
export const releaseTexts = {
  spellDialog: 'Soltar Bola de Fogo',
  spellButton: 'Soltar no mapa',
  moveDialog: 'Ação preparada: mover',
  moveButton: 'Mover agora',
  back: 'Voltar',
  done: 'Voltar à sessão',
  onMap: 'A mesa espera você: até 2 min.',
  masterFoot: 'Pensantus ainda pode responder no próprio aparelho: vale a primeira resposta.',
  undone: 'O mestre desfez o gatilho: a Bola de Fogo continua preparada.',
  otherDevice: /Você abriu o mapa em outro aparelho\. Passa sozinho em/,
} as const;

/** What the player holds: the Bola de Fogo (3rd-level slot), or a move of the speed it has. */
export type Held = 'fireball' | 'move' | 'direction' | 'wall';

/** The held spells of the other pickers: the cone of Mãos Flamejantes (a direction) and the Muralha de Fogo (a wall, 4th-level slot, so a level 9 caster). */
export const heldSpell = {
  direction: { key: 'spell:burning-hands', level: 1, name: 'Mãos Flamejantes' },
  wall: { key: 'spell:wall-of-fire', level: 4, name: 'Muralha de Fogo' },
} as const;

/** The caster of the table: Pensantus at level 5 with the Bola de Fogo, or at level 9 with the wall. */
function kitOf(held: Held): typeof fireballKit {
  if (held !== 'wall') {
    return fireballKit;
  }
  const spells = [...pensantusCasting.preparedSpellKeys, heldSpell.wall.key];
  return { build: { ...pensantus, level: 9 }, sheet: { ...pensantusCasting, knownSpellKeys: spells, preparedSpellKeys: spells } };
}

export interface ReadyMapTable {
  master: BrowserContext;
  player: BrowserContext;
  m: Page;
  p: Page;
  campaignId: string;
  /** The combat as it was read when the turn went to the Capitão, with Pensantus's readied action waiting. */
  encounter: Encounter;
  done: () => Promise<void>;
}

/** The table: Pensantus readied `held` on his turn, and the turn is the Capitão's. Nothing is fired yet. */
export async function readyMapTable(
  browser: Browser,
  name: string,
  held: Held,
  view: { width?: number; height?: number; colorScheme?: 'light' | 'dark'; both?: boolean; blocker?: boolean } = {},
): Promise<ReadyMapTable> {
  const viewport = { width: view.width ?? 1280, height: view.height ?? 900 };
  const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: view.both ? viewport : { width: 1280, height: 900 }, colorScheme: view.colorScheme, ...featShotScale });
  const player = await newSignedInContext(browser, 'Jogador Teste', { viewport, colorScheme: view.colorScheme, ...featShotScale });
  const m = await master.newPage();
  const p = await player.newPage();
  await m.goto('/');
  await p.goto('/');
  const table = await tableForCombat(m, p, `${name} ${Date.now()}`, true, true, kitOf(held));
  // `blocker`: a goblin the player cannot see stands on the square next to Pensantus, on the way east.
  const at = view.blocker ? { ...mapSquares, 'Goblin 2': [mapSquares['Pensantus'][0] + 1, mapSquares['Pensantus'][1]] as [number, number] } : mapSquares;
  const begun = await beginAttackCombatRPC(m, table, mapFaces, at, view.blocker ? ['Goblin 2'] : []);
  await openSessionPage(p, table.campaignId);
  await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
  const readied = await holdOn(p, table.campaignId, begun, held);
  await waitForEncounterRevision(p, readied);
  await openSessionPage(m, table.campaignId);
  const encounter = await passTurnsToAndWait(m, table.campaignId, 'Capitão Goblin', [m, p]);
  return {
    master,
    player,
    m,
    p,
    campaignId: table.campaignId,
    encounter,
    done: async () => {
      await endOpenSessionRPC(m, table.campaignId);
      await master.close();
      await player.close();
    },
  };
}

/** The player's own call that holds `held`, with the trigger of the boards. */
export async function holdOn(player: Page, campaignId: string, encounter: Encounter, held: Held): Promise<Encounter> {
  if (held === 'fireball') {
    return readySpellRPC(player, campaignId, encounter, 'Pensantus', 'spell:fireball', 3);
  }
  if (held === 'direction' || held === 'wall') {
    return readySpellRPC(player, campaignId, encounter, 'Pensantus', heldSpell[held].key, heldSpell[held].level);
  }
  return readyRPC(player, 'ReadyAction', {
    campaignId,
    encounterId: encounter.id,
    combatantId: encounter.combatants.find((c) => c.label === 'Pensantus')!.id,
    triggerText: goblinTrigger,
    kind: 'READIED_KIND_MOVE',
  });
}

/** "Disparar" of the fold on the master's page: the window opens for the readier. */
export async function fireReadied(m: Page): Promise<void> {
  const button = m.locator('app-readied-fold').getByRole('button', { name: 'Disparar a ação de Pensantus' });
  await expect(button).toBeEnabled();
  await button.click();
}

/**
 * The master undoes the fire with the server's own call: it names the fire as the last action (`undoable_event_id` of `ListCombatLog`)
 * and takes the window back, the readied row waiting again. The log has no line for a fire whose window is open (the window tells it), so
 * the log's "Desfazer última ação" has no row to show for it: the screen has no button for this undo, only the call.
 */
export async function undoFire(m: Page, campaignId: string): Promise<void> {
  const { id } = await getEncounterRPC(m, campaignId);
  const log = await callRPC(m, 'meurpg.play.v1.CombatService/ListCombatLog', { campaignId, encounterId: id });
  expect(log.ok(), await log.text()).toBeTruthy();
  const fired = (await log.json()).undoableEventId as string;
  expect(fired).toBeTruthy();
  await waitForEncounterRevision(m, await combatRPC(m, 'UndoLastAction', { campaignId, encounterId: id, expectedEventId: fired }));
}

/** The window of the held Bola de Fogo on the player's page, once the master fired. */
export function spellWindow(p: Page, name = 'Bola de Fogo'): Locator {
  return p.getByRole('alertdialog', { name: `Soltar ${name}` });
}

/** The window of the held move on the player's page. */
export function moveWindow(p: Page): Locator {
  return p.getByRole('alertdialog', { name: releaseTexts.moveDialog });
}

/** The cast sheet of the release (the player's) or the master's area dialog: the dialog titled "Soltar Bola de Fogo" (+ " por Pensantus" for the master). */
export function releaseSheet(page: Page, master = false, name = 'Bola de Fogo'): Locator {
  return page.getByRole('dialog', { name: master ? `Soltar ${name} por Pensantus` : `Soltar ${name}` });
}

/** The master's row of the readied window in the queue: it stays in the "rest" part however long the window waits (it is never a late question). */
export function masterQueue(m: Page): Locator {
  return m.locator('app-reaction-queue[part="rest"]');
}

/** Picks the point on the creature "Goblin 1" with the keyboard on the open release sheet ("C" opens "Centrar em…"), and waits until the sheet reads the point: "Confirmar local" is then filled. */
export async function pickFireballPoint(sheet: Locator): Promise<void> {
  const map = sheet.getByRole('application', { name: 'Mapa: escolha o ponto da Bola de Fogo' });
  await expect(map).toBeFocused();
  await map.press('c');
  await sheet.getByRole('listbox', { name: 'Centrar em…' }).getByRole('option', { name: /Goblin 1/ }).click();
  // Enter confirms the point the map holds: it is read once the pick has been placed, as a table sees it.
  await expect(sheet.getByText('Toque de novo no mesmo ponto')).toBeVisible();
}

/** Opens the map from the player's window and picks the point on the creature "Goblin 1"; stops at the "Quem está na área" step. */
export async function placeFireball(p: Page): Promise<Locator> {
  await spellWindow(p).getByTestId('release-to-map').click();
  const sheet = releaseSheet(p);
  await pickFireballPoint(sheet);
  await sheet.getByRole('application', { name: 'Mapa: escolha o ponto da Bola de Fogo' }).press('Enter');
  await expect(sheet.getByRole('heading', { name: 'Quem está na área' })).toBeFocused();
  return sheet;
}

/** The page scrolled so the legend (and, on a phone, the nudge buttons) under the map is in the shot, as the boards draw them: the end of the page on a phone, the legend at the foot of the window from 1024 px. */
export async function scrollToLegend(page: Page): Promise<void> {
  await page.evaluate(() => {
    const el = document.scrollingElement ?? document.documentElement;
    if (window.innerWidth < 1024) {
      el.scrollTop = el.scrollHeight;
      return;
    }
    document.querySelector('app-map-layers-legend')?.scrollIntoView({ block: 'end' });
  });
}
