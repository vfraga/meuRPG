import { expect, type Browser, type BrowserContext, type Locator, type Page } from '@playwright/test';

import { combatRPC, getEncounterRPC, passTurnsTo, pensantusCasting, startEncounterRPC, tableForCombat, type CombatTable, type Encounter } from './combat-support';
import { endOpenSessionRPC } from './live-session-support';
import { callRPC, characterRpcBody, createCharacterRPC, newSignedInContext, type CharacterBuild } from './support';

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

/** `CastSpell` of a zone spell at the square `origin` from `page`, whoever the page is: the master for an NPC, the player for his own. */
export async function castZoneRPC(
  page: Page,
  t: { campaignId: string; encounterId: string },
  casterId: string,
  spellKey: string,
  origin: { col: number; row: number },
  slotLevel = 2,
): Promise<void> {
  const res = await callRPC(page, 'meurpg.play.v1.CombatService/CastSpell', {
    campaignId: t.campaignId,
    encounterId: t.encounterId,
    casterId,
    spellKey,
    slot: { level: slotLevel },
    targets: [],
    origin,
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

/** The first zone the map draws: its footprint, to see it change when the zone moves. */
export function firstFootprint(page: Page): Locator {
  return page.locator('app-zone-layer .zl__fill').first();
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
}

/**
 * The table of the specs of the eight spells: Pensantus (the player's, standing where the table puts him) and one NPC druid
 * for each of `casters`, all with the eight spells prepared (a table's NPC may carry any prepared list), begun with the
 * casters first. Nobody has cast yet: the spec casts with `cast` on the caster's turn (`passTurnsTo` moves the combat).
 * `fog` turns the fog of the map on with a bright base light, so Darkness is what hides a square.
 */
export async function moreZonesTable(browser: Browser, name: string, casters: MoreCaster[], options: { width?: number; colorScheme?: 'light' | 'dark'; fog?: boolean } = {}): Promise<ZoneTable> {
  const width = options.width ?? 1280;
  const viewport = { width, height: width >= 768 ? 900 : 844 };
  const master: BrowserContext = await newSignedInContext(browser, 'Mestre Teste', { viewport, colorScheme: options.colorScheme ?? 'light' });
  const player: BrowserContext = await newSignedInContext(browser, 'Jogador Teste', { viewport, colorScheme: options.colorScheme ?? 'light' });
  const m = await master.newPage();
  const p = await player.newPage();
  await m.goto('/');
  await p.goto('/');
  const table = await tableForCombat(m, p, `${name} ${Date.now()}`, true, true, { sheet: pensantusCasting });
  const campaignId = table.campaignId;
  if (options.fog) {
    const fog = await callRPC(m, 'meurpg.maps.v1.MapService/SetMapFog', { campaignId, mapId: table.mapId, fogEnabled: true, baseLight: 'LIGHT_LEVEL_BRIGHT' });
    expect(fog.ok(), await fog.text()).toBeTruthy();
  }
  const ids = new Map<string, string>();
  for (const c of casters) {
    ids.set(c.label, await createDruidRPC(m, campaignId, c.label, MORE_ZONE_SPELLS));
  }
  let enc = await startEncounterRPC(
    m,
    table,
    casters.map((c) => ({ characterId: ids.get(c.label)!, count: 1, hidden: c.hidden ?? false })),
  );
  const faces: Record<string, number> = { Pensantus: 1, ...Object.fromEntries(casters.map((c) => [c.label, c.face])) };
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
