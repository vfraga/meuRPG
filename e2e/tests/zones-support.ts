import { expect, type Browser, type BrowserContext, type Locator, type Page } from '@playwright/test';

import { combatRPC, getEncounterRPC, passTurnsTo, pensantusCasting, startEncounterRPC, tableForCombat, type CombatTable, type Encounter } from './combat-support';
import { endOpenSessionRPC } from './live-session-support';
import { callRPC, characterRpcBody, createCharacterRPC, newSignedInContext, openActionTab, pensantus, type CharacterBuild } from './support';

// Setup for the spell zone specs (RN-37), through the API: a combat of Pensantus (the player's, who casts Teia) and two NPC
// druids who cast Raio Lunar, one in plain sight (Druida) and one hidden from the players (Sombra). The specs prove what
// the two screens read about the zones, not the cast sheet or the character forms other specs cover.

/** The Encounter's zone as the Connect JSON codec writes it (camelCase, zero values left out) and the specs read it. */
export interface ZoneJson {
  id: string;
  spellKey: string;
  ownerLabel: string;
  origin?: { col?: number; row?: number };
  squares?: { col?: number; row?: number }[];
  burningSquares?: { col?: number; row?: number }[];
  wallSide?: string;
  detailsHidden?: boolean;
  visibility?: string;
}

export interface ZoneTable {
  m: Page;
  p: Page;
  table: CombatTable;
  campaignId: string;
  encounterId: string;
  /** The id of the combatant with this label. */
  id: (label: string) => string;
  done: () => Promise<void>;
}

/** A level 5 druid for the NPCs that cast Raio Lunar (Wisdom 16: the same build the creature specs use for a druid). */
const druid = (name: string): CharacterBuild => ({
  name,
  raceKey: 'race:human',
  race: 'Humano',
  classKey: 'class:druid',
  class: 'Druida',
  level: 5,
  backgroundKey: 'background:acolyte',
  backgroundSkillKeys: [],
  backgroundSkills: [],
  extraSkillKeys: ['skill:nature', 'skill:perception'],
  extraSkills: [],
  scores: { for: 10, des: 12, con: 14, int: 10, sab: 16, car: 12 },
});

/** An NPC druid with Raio Lunar prepared, and its id. */
async function createDruidRPC(master: Page, campaignId: string, name: string, spells: string[] = ['spell:moonbeam']): Promise<string> {
  const body = characterRpcBody('BOSS', druid(name)) as { sheet: { full: object } };
  body.sheet.full = { ...body.sheet.full, preparedSpellKeys: spells };
  const res = await createCharacterRPC(master, campaignId, body);
  expect(res.ok(), await res.text()).toBeTruthy();
  return (await res.json()).character.id as string;
}

/** A level 9 wizard for the NPC that casts Wall of Fire (4th-level slots), Pensantus's build with another name. */
const archmage = (name: string): CharacterBuild => ({ ...pensantus, name, level: 9 });

/** An NPC wizard of level 9 with `spells` prepared, and its id. */
async function createWizardRPC(master: Page, campaignId: string, name: string, spells: string[]): Promise<string> {
  const body = characterRpcBody('BOSS', archmage(name)) as { sheet: { full: object } };
  body.sheet.full = { ...body.sheet.full, knownSpellKeys: spells, preparedSpellKeys: spells };
  const res = await createCharacterRPC(master, campaignId, body);
  expect(res.ok(), await res.text()).toBeTruthy();
  return (await res.json()).character.id as string;
}

/** The wall a Wall of Fire cast sends (`SpellWall`, as the Connect JSON codec writes it): a line or a ring and the side that burns. */
export interface WallJson {
  direction?: { dx: number; dy: number };
  ring?: boolean;
  side: 'WALL_SIDE_LEFT' | 'WALL_SIDE_RIGHT' | 'WALL_SIDE_INNER' | 'WALL_SIDE_OUTER';
  lengthFt?: number;
}

/** `CastSpell` of a zone spell at the square `origin` from `page`, whoever the page is: the master for an NPC, the player for his own. */
export async function castZoneRPC(
  page: Page,
  t: { campaignId: string; encounterId: string },
  casterId: string,
  spellKey: string,
  origin: { col: number; row: number },
  slotLevel = 2,
  wall?: WallJson,
): Promise<void> {
  const res = await callRPC(page, 'meurpg.play.v1.CombatService/CastSpell', {
    campaignId: t.campaignId,
    encounterId: t.encounterId,
    casterId,
    spellKey,
    slot: { level: slotLevel },
    targets: [],
    origin,
    ...(wall ? { wall } : {}),
    idempotencyKey: crypto.randomUUID(),
  });
  expect(res.ok(), await res.text()).toBeTruthy();
}

/** The zones the master reads now. */
export async function zonesRPC(page: Page, campaignId: string): Promise<ZoneJson[]> {
  const enc = (await getEncounterRPC(page, campaignId)) as Encounter & { zones?: ZoneJson[] };
  return enc.zones ?? [];
}

/** The zone of `spellKey` cast by `caster`, as the master reads it. */
export async function zoneOf(master: Page, campaignId: string, spellKey: string, caster: string): Promise<ZoneJson> {
  const zone = (await zonesRPC(master, campaignId)).find((z) => z.spellKey === spellKey && z.ownerLabel === caster);
  expect(zone, `no ${spellKey} zone of ${caster}`).toBeTruthy();
  return zone!;
}

/**
 * The table: Pensantus (level 3, Teia prepared), Druida and Sombra (hidden) in a combat on the 20 x 14 grid, begun with the
 * druids first. Each druid casts Raio Lunar on its turn (the master plays them), then Pensantus, on his turn, casts Teia.
 * So the zones are, in the order they were cast: Raio Lunar of Druida, Raio Lunar of Sombra (a hidden caster: the master's
 * alone) and Teia of Pensantus. Both pages are `width` wide; neither navigates.
 */
export async function zoneTable(browser: Browser, name: string, width = 1280, colorScheme: 'light' | 'dark' = 'light'): Promise<ZoneTable> {
  const viewport = { width, height: width >= 768 ? 900 : 844 };
  const master: BrowserContext = await newSignedInContext(browser, 'Mestre Teste', { viewport, colorScheme });
  const player: BrowserContext = await newSignedInContext(browser, 'Jogador Teste', { viewport, colorScheme });
  const m = await master.newPage();
  const p = await player.newPage();
  await m.goto('/');
  await p.goto('/');
  const table = await tableForCombat(m, p, `${name} ${Date.now()}`, true, true, { sheet: pensantusCasting });
  const campaignId = table.campaignId;
  const druidId = await createDruidRPC(m, campaignId, 'Druida');
  const shadowId = await createDruidRPC(m, campaignId, 'Sombra');

  let enc = await startEncounterRPC(m, table, [
    { characterId: druidId, count: 1, hidden: false },
    { characterId: shadowId, count: 1, hidden: true },
  ]);
  const faces: Record<string, number> = { Druida: 20, Sombra: 19, Pensantus: 5 };
  for (const c of enc.combatants) {
    enc = await combatRPC(m, 'SubmitInitiative', { campaignId, encounterId: enc.id, combatantId: c.id, d20Face: faces[c.label] ?? 1 });
  }
  const at: Record<string, [number, number]> = { Druida: [3, 2], Sombra: [3, 12] };
  for (const [label, [col, row]] of Object.entries(at)) {
    const combatantId = enc.combatants.find((c) => c.label === label)!.id;
    enc = await combatRPC(m, 'MoveCombatant', { campaignId, encounterId: enc.id, combatantId, col, row });
  }
  enc = await combatRPC(m, 'BeginCombat', { campaignId, encounterId: enc.id });
  const t = { campaignId, encounterId: enc.id };
  const ids = new Map(enc.combatants.map((c) => [c.label, c.id]));
  const id = (label: string) => ids.get(label)!;

  // Druida's turn: Raio Lunar over open floor; then Sombra's; then Pensantus's own Teia, a few squares from where he stands.
  await castZoneRPC(m, t, id('Druida'), 'spell:moonbeam', { col: 12, row: 3 });
  await passTurnsTo(m, campaignId, 'Sombra');
  await castZoneRPC(m, t, id('Sombra'), 'spell:moonbeam', { col: 16, row: 11 });
  enc = await passTurnsTo(m, campaignId, 'Pensantus');
  const me = enc.combatants.find((c) => c.label === 'Pensantus')!;
  await castZoneRPC(p, t, id('Pensantus'), 'spell:web', { col: (me.col ?? 0) + 3, row: (me.row ?? 0) + 2 });

  return {
    m,
    p,
    table,
    campaignId,
    encounterId: enc.id,
    id,
    done: async () => {
      await endOpenSessionRPC(m, campaignId);
      await master.close();
      await player.close();
    },
  };
}

/** The "Zonas ativas" panel of a screen. */
export function zonesPanel(page: Page): Locator {
  return page.getByRole('region', { name: /^Zonas ativas/ });
}

/** The card of a zone in the panel, by its label ("Teia · Pensantus"). */
export function zoneCard(page: Page, label: string): Locator {
  return zonesPanel(page).locator('li.zp__card', { has: page.getByRole('heading', { level: 3, name: label }) });
}

/** The first zone the map draws: its footprint (the veil's outline), to see it change when the zone moves. */
export function firstFootprint(page: Page): Locator {
  return page.locator('app-zone-layer .zl__zone > .zl__veil').first();
}

/** The corner badges the map draws, one white diamond with a letter for each zone the screen reads. */
export function zoneBadges(page: Page): Locator {
  return zoneLayer(page).locator('.zl__badge');
}

/** The letter of a zone as its list card says it ("Zona A" is the picture's name): the key between the list and the map. */
export async function zoneLetter(page: Page, label: string): Promise<string> {
  const name = await zoneCard(page, label).getByRole('img', { name: /^Zona / }).getAttribute('aria-label');
  return (name ?? '').replace(/^Zona /, '');
}

/** The map shows the zone of this label: its letter's badge is on it (the map names nothing, the list and the legend do). */
export async function expectOnMap(page: Page, label: string): Promise<void> {
  const letter = await zoneLetter(page, label);
  expect(letter, `${label} has no letter in the list`).not.toBe('');
  const badge = zoneBadges(page).filter({ hasText: new RegExp(`^${letter}$`) });
  await expect(badge).toHaveCount(1);
  // The letter is painted above the white diamond, in the ink (a bare text node was hidden under the diamond's ::before).
  const letterEl = badge.locator('.zl__letter');
  await expect(letterEl).toHaveText(letter);
  await expect(letterEl).toHaveCSS('position', 'relative');
  await expect(letterEl).toHaveCSS('z-index', '1');
  await expect(letterEl).toHaveCSS('color', 'rgb(27, 34, 48)');
}

/**
 * A zone's "Mais ações" menu, open: the one button of the card is named after the zone and opens a menu with "Mover" (a zone
 * that moves by an action) and "Encerrar zona".
 */
export async function openZoneMenu(page: Page, label: string): Promise<Locator> {
  await zoneCard(page, label).getByRole('button', { name: `Mais ações de ${label}` }).click();
  const menu = page.getByRole('menu');
  await expect(menu).toBeVisible();
  return menu;
}

/** Chooses `action` in the zone's "Mais ações" menu. */
export async function zoneAction(page: Page, label: string, action: 'Mover' | 'Encerrar zona'): Promise<void> {
  const menu = await openZoneMenu(page, label);
  await menu.getByRole('menuitem', { name: action, exact: true }).click();
  await expect(menu).toHaveCount(0);
}

/** The zone layer of the map (decorative: it is `aria-hidden`, and the panel is the accessible list). */
export function zoneLayer(page: Page): Locator {
  return page.locator('app-zone-layer');
}

/** The master moves Pensantus onto the zone's origin (a forced move, as the master's), which catches him as one who came in. */
export async function enterZoneRPC(t: ZoneTable, zone: ZoneJson): Promise<void> {
  await combatRPC(t.m, 'MoveCombatant', {
    campaignId: t.campaignId,
    encounterId: t.encounterId,
    combatantId: t.id('Pensantus'),
    col: zone.origin?.col ?? 0,
    row: zone.origin?.row ?? 0,
    forced: true,
  });
}

/** The spells the eight more zone specs cast (RN-37), by the level of the slot each takes. */
export const MORE_ZONE_SPELLS = ['spell:fog-cloud', 'spell:darkness', 'spell:stinking-cloud', 'spell:grease', 'spell:entangle', 'spell:spike-growth', 'spell:silence'];

/** A caster of the table of the eight zone spells: its label, the square it stands on and its initiative. */
export interface MoreCaster {
  label: string;
  at: [number, number];
  face: number;
  hidden?: boolean;
  /** A level 9 wizard with these spells prepared instead of the druid with the eight spells. */
  wizard?: string[];
}

/**
 * The table of the specs of the eight spells: Pensantus (the player's, standing where the table puts him) and one NPC druid
 * for each of `casters`, all with the eight spells prepared (a table's NPC may carry any prepared list), begun with the
 * casters first. Nobody has cast yet: the spec casts with `cast` on the caster's turn (`passTurnsTo` moves the combat).
 * `fog` turns the fog of the map on with a bright base light, so Darkness is what hides a square.
 */
export async function moreZonesTable(browser: Browser, name: string, casters: MoreCaster[], options: { width?: number; colorScheme?: 'light' | 'dark'; fog?: boolean; player?: { build?: CharacterBuild; sheet?: Record<string, unknown> }; playerFace?: number } = {}): Promise<ZoneTable> {
  const width = options.width ?? 1280;
  const viewport = { width, height: width >= 768 ? 900 : 844 };
  const master: BrowserContext = await newSignedInContext(browser, 'Mestre Teste', { viewport, colorScheme: options.colorScheme ?? 'light' });
  const player: BrowserContext = await newSignedInContext(browser, 'Jogador Teste', { viewport, colorScheme: options.colorScheme ?? 'light' });
  const m = await master.newPage();
  const p = await player.newPage();
  await m.goto('/');
  await p.goto('/');
  const table = await tableForCombat(m, p, `${name} ${Date.now()}`, true, true, options.player ?? { sheet: pensantusCasting });
  const campaignId = table.campaignId;
  if (options.fog) {
    const fog = await callRPC(m, 'meurpg.maps.v1.MapService/SetMapFog', { campaignId, mapId: table.mapId, fogEnabled: true, baseLight: 'LIGHT_LEVEL_BRIGHT' });
    expect(fog.ok(), await fog.text()).toBeTruthy();
  }
  const ids = new Map<string, string>();
  for (const c of casters) {
    ids.set(c.label, c.wizard ? await createWizardRPC(m, campaignId, c.label, c.wizard) : await createDruidRPC(m, campaignId, c.label, MORE_ZONE_SPELLS));
  }
  let enc = await startEncounterRPC(
    m,
    table,
    casters.map((c) => ({ characterId: ids.get(c.label)!, count: 1, hidden: c.hidden ?? false })),
  );
  const faces: Record<string, number> = { Pensantus: options.playerFace ?? 1, ...Object.fromEntries(casters.map((c) => [c.label, c.face])) };
  for (const c of enc.combatants) {
    enc = await combatRPC(m, 'SubmitInitiative', { campaignId, encounterId: enc.id, combatantId: c.id, d20Face: faces[c.label] ?? 1 });
  }
  for (const c of casters) {
    const combatantId = enc.combatants.find((x) => x.label === c.label)!.id;
    enc = await combatRPC(m, 'MoveCombatant', { campaignId, encounterId: enc.id, combatantId, col: c.at[0], row: c.at[1] });
  }
  enc = await combatRPC(m, 'BeginCombat', { campaignId, encounterId: enc.id });
  const combatants = new Map(enc.combatants.map((c) => [c.label, c.id]));
  return {
    m,
    p,
    table,
    campaignId,
    encounterId: enc.id,
    id: (label: string) => combatants.get(label)!,
    done: async () => {
      await endOpenSessionRPC(m, campaignId);
      await master.close();
      await player.close();
    },
  };
}

/** Wall of Fire, the spell the wall specs cast. */
export const WALL_SPELL = 'spell:wall-of-fire';

/**
 * The table of the Wall of Fire specs: Pensantus as a level 9 wizard who prepared Wall of Fire (the player's, first in the
 * order, so his turn is the first) and Mago, an NPC wizard of level 9 with it too, in plain sight at (8, 5), west of the walls the specs lay. The map is
 * the 20 x 14 grid; `fog` turns the fog on with a bright base light, so only a wall (or Darkness) hides a square.
 */
export async function wallTable(browser: Browser, name: string, options: { width?: number; colorScheme?: 'light' | 'dark'; fog?: boolean } = {}): Promise<ZoneTable> {
  const spells = [...pensantusCasting.preparedSpellKeys, WALL_SPELL];
  return moreZonesTable(browser, name, [{ label: 'Mago', at: [8, 5], face: 10, wizard: [WALL_SPELL] }], {
    ...options,
    playerFace: 20,
    player: { build: { ...pensantus, level: 9 }, sheet: { ...pensantusCasting, knownSpellKeys: spells, preparedSpellKeys: spells } },
  });
}

/** The cast sheet of Wall of Fire, open on the player's page: "Conjurar Muralha de Fogo", with the map of "Onde fica" focused. */
export async function openWallSheet(page: Page): Promise<{ sheet: Locator; map: Locator }> {
  await openActionTab(page, 'Magias');
  await page.getByRole('button', { name: 'Conjurar Muralha de Fogo' }).click();
  const sheet = page.getByRole('dialog', { name: 'Conjurar Muralha de Fogo' });
  const map = sheet.getByRole('application', { name: /^Mapa: escolha o (começo|centro) da Muralha de Fogo$/ });
  await expect(map).toBeFocused();
  return { sheet, map };
}

/** A tap on a square of the "Onde fica" map (20 x 14 squares, the map fills its box): the start of a line, the centre of a ring. */
export async function tapWallSquare(map: Locator, col: number, row: number): Promise<void> {
  const size = await map.evaluate((el) => ({ width: el.clientWidth, height: el.clientHeight }));
  await map.click({ position: { x: ((col + 0.5) * size.width) / 20, y: ((row + 0.5) * size.height) / 14 } });
}

/** The server's preview of the wall has come: its squares are drawn and "Calculando onde a muralha fica…" is gone. */
export async function expectWallCalculated(sheet: Locator): Promise<void> {
  await expect(sheet.locator('.zl__zone').first()).toBeVisible();
  await expect(sheet.getByRole('status').filter({ hasText: 'Calculando onde a muralha fica…' })).toHaveCount(0);
}

/** The "Próximo: quem está nela" button of the step. */
export function nextStep(sheet: Locator): Locator {
  return sheet.getByRole('button', { name: 'Próximo: quem está nela' });
}
