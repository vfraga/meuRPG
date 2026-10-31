import { expect, type Page } from '@playwright/test';

import { canvasPng, createMapRPC, placeTokenRPC, revealMapRPC, setCurrentMapRPC, tableForMaps, uploadImageRPC, type MapsTable } from './maps-support';
import { startSessionRPC } from './live-session-support';
import { waitForEncounterRevision } from './revision-support';
import { callRPC, characterRpcBody, createCharacterRPC, pensantus, type CharacterBuild } from './support';

// Setup for the combat specs (Etapa 6, MR-013), through the API: these tests
// prove the combat screens, not the campaign, character and map forms other
// specs already cover. Every test makes its own campaign.

export interface CombatTable extends MapsTable {
  sessionId: string;
  mapId: string;
  /** The Capitão Goblin (an enemy), the Goblin (a minion) and Velha Odra (story). */
  captainId: string;
  goblinId: string;
  odraId: string;
}

/**
 * A table ready for a combat: Pensantus (the player's, with a token), the
 * NPCs Capitão Goblin, Goblin and Velha Odra, an open session and a map as
 * its current map, with a 20-column grid when `grid` (the default). Master
 * and player pages must be signed in as each; neither navigates.
 */
export async function tableForCombat(
  masterPage: Page,
  playerPage: Page,
  name: string,
  grid = true,
  attacks = false,
  character: { build?: CharacterBuild; sheet?: Record<string, unknown> } = {},
): Promise<CombatTable> {
  const base = await tableForMaps(masterPage, playerPage, name, true, character.sheet ?? (attacks ? pensantusAttacks : {}), character.build);
  // With `attacks` the Capitão carries a scimitar and wears chain mail, so the master has something to roll.
  const captainBody = characterRpcBody('ENEMY', { ...pensantus, name: 'Capitão Goblin' }) as { sheet: { full: object } };
  if (attacks) {
    captainBody.sheet.full = { ...captainBody.sheet.full, weaponKeys: ['equipment:scimitar'], armorKey: 'equipment:chain-mail' };
  }
  const captain = await createCharacterRPC(masterPage, base.campaignId, captainBody);
  expect(captain.ok()).toBeTruthy();
  const odra = await createCharacterRPC(masterPage, base.campaignId, {
    kind: 'CHARACTER_KIND_STORY',
    name: 'Velha Odra',
    sheet: { basic: { hitPointsMax: 9, armorClass: 10, speedFt: 25, attackBonus: 0, damage: '1d4', description: '' } },
  });
  expect(odra.ok()).toBeTruthy();

  const sessionId = await startSessionRPC(masterPage, base.campaignId);
  await masterPage.goto('/');
  const image = await uploadImageRPC(masterPage, base.campaignId, 'Emboscada na estrada', await canvasPng(masterPage, 2000, 1400, 'Emboscada na estrada'));
  const mapId = await createMapRPC(masterPage, base.campaignId, 'Emboscada na estrada', image);
  await revealMapRPC(masterPage, base.campaignId, mapId);
  await setCurrentMapRPC(masterPage, base.campaignId, mapId);
  if (grid) {
    await setGridRPC(masterPage, base.campaignId, mapId, 20);
  }
  await placeTokenRPC(masterPage, base.campaignId, mapId, base.characterId, 2500, 5400);
  const captainId = (await captain.json()).character.id as string;
  await placeTokenRPC(masterPage, base.campaignId, mapId, captainId, 5750, 3900);
  return { ...base, sessionId, mapId, captainId, goblinId: base.npcId!, odraId: (await odra.json()).character.id as string };
}

/** What Pensantus carries in the attack specs: a dagger, Raio de Fogo and some
 * spells (the "Sua vez" groups need something to list). */
export const pensantusAttacks = {
  weaponKeys: ['equipment:dagger'],
  cantripKeys: ['spell:fire-bolt'],
  knownSpellKeys: ['spell:magic-missile', 'spell:sleep', 'spell:shield', 'spell:web', 'spell:misty-step'],
  preparedSpellKeys: ['spell:magic-missile', 'spell:sleep', 'spell:shield', 'spell:web', 'spell:misty-step'],
};

/** Where `beginAttackCombatRPC` puts the NPCs when the caller names no squares. */
export const attackCombatSquares: Record<string, [number, number]> = { 'Capitão Goblin': [11, 5], 'Goblin 1': [9, 9], 'Goblin 2': [14, 10] };

/**
 * Starts a combat with the Capitão and two Goblins in plain sight, rolls
 * everyone's initiative with the given d20 faces (by label; the master may
 * type them) and places them, then begins. Returns the combat on turn 1.
 *
 * A creature the combat needs besides those (an SRD creature key in `monsters`, in plain sight) joins at the start,
 * never later: the master can set an initiative only while the combat is in SETUP, and the app rolls the initiative
 * of a creature added to a running combat with `dice.Crypto`, where a tie with the actor is a joint turn. Give its
 * label a face in `faces`; the call fails when that creature's total equals another combatant's, so a tie cannot
 * reach the test by chance.
 */
export async function beginAttackCombatRPC(
  master: Page,
  table: CombatTable,
  faces: Record<string, number>,
  at: Record<string, [number, number]> = attackCombatSquares,
  hidden: string[] = [],
  monsters: string[] = [],
): Promise<Encounter> {
  let enc = monsters.length
    ? await startEncounterWithMonstersRPC(master, table, monsters)
    : await startEncounterRPC(master, table, [
        { characterId: table.captainId, count: 1, hidden: false },
        { characterId: table.goblinId, count: 2, hidden: false },
      ]);
  for (const c of enc.combatants) {
    enc = await combatRPC(master, 'SubmitInitiative', { campaignId: table.campaignId, encounterId: enc.id, combatantId: c.id, d20Face: faces[c.label] ?? 1 });
  }
  for (const added of enc.combatants.filter((c) => (c as { bestiaryCreatureKey?: string }).bestiaryCreatureKey)) {
    const same = enc.combatants.filter((c) => c.id !== added.id && c.initiative === added.initiative).map((c) => c.label);
    expect(same, `${added.label} total ${added.initiative} must differ from every other combatant's: give its label another face in faces`).toEqual([]);
  }
  for (const [label, [col, row]] of Object.entries(at)) {
    const id = enc.combatants.find((c) => c.label === label)!.id;
    enc = await combatRPC(master, 'MoveCombatant', { campaignId: table.campaignId, encounterId: enc.id, combatantId: id, col, row });
  }
  for (const label of hidden) {
    const id = enc.combatants.find((c) => c.label === label)!.id;
    enc = await combatRPC(master, 'SetCombatantHidden', { campaignId: table.campaignId, encounterId: enc.id, combatantId: id, hidden: true });
  }
  return combatRPC(master, 'BeginCombat', { campaignId: table.campaignId, encounterId: enc.id });
}

export async function setGridRPC(page: Page, campaignId: string, mapId: string, columns: number): Promise<void> {
  const res = await callRPC(page, 'meurpg.maps.v1.MapService/SetMapGrid', { campaignId, mapId, columns });
  expect(res.ok(), await res.text()).toBeTruthy();
}

/** `StartEncounter` for the whole party and the given NPC copies. */
export async function startEncounterRPC(
  page: Page,
  table: CombatTable,
  npcs: { characterId: string; count: number; hidden?: boolean }[],
  name = 'Emboscada na estrada',
): Promise<Encounter> {
  const res = await callRPC(page, 'meurpg.play.v1.CombatService/StartEncounter', {
    campaignId: table.campaignId,
    idempotencyKey: crypto.randomUUID(),
    name,
    participants: npcs,
  });
  expect(res.ok(), await res.text()).toBeTruthy();
  return (await res.json()).encounter as Encounter;
}

/** `StartEncounter` for the whole party, the Capitão, two Goblins and SRD creatures (`monsters`, one each), all in plain sight. */
async function startEncounterWithMonstersRPC(page: Page, table: CombatTable, monsters: string[]): Promise<Encounter> {
  const res = await callRPC(page, 'meurpg.play.v1.CombatService/StartEncounter', {
    campaignId: table.campaignId,
    idempotencyKey: crypto.randomUUID(),
    name: 'Emboscada na estrada',
    participants: [
      { characterId: table.captainId, count: 1, hidden: false },
      { characterId: table.goblinId, count: 2, hidden: false },
    ],
    monsters: monsters.map((creatureKey) => ({ creatureKey, count: 1 })),
    monstersHidden: false,
  });
  expect(res.ok(), await res.text()).toBeTruthy();
  return (await res.json()).encounter as Encounter;
}

export async function getEncounterRPC(page: Page, campaignId: string): Promise<Encounter> {
  const res = await callRPC(page, 'meurpg.play.v1.CombatService/GetEncounter', { campaignId });
  expect(res.ok(), await res.text()).toBeTruthy();
  return (await res.json()).encounter as Encounter;
}

/** Any write of `CombatService` that takes the encounter and an idempotency key. */
export async function combatRPC(page: Page, method: string, body: object): Promise<Encounter> {
  const res = await callRPC(page, `meurpg.play.v1.CombatService/${method}`, { idempotencyKey: crypto.randomUUID(), ...body });
  expect(res.ok(), await res.text()).toBeTruthy();
  return (await res.json()).encounter as Encounter;
}

/** The JSON shape of an `Encounter` (the Connect JSON codec: camelCase, and
 * zero values left out) that the specs read. */
export interface Encounter {
  id: string;
  name: string;
  status: string;
  round?: number;
  currentCombatantId?: string;
  masterTurn?: boolean;
  gridColumns: number;
  gridRows: number;
  combatants: Combatant[];
  revision: number;
  reactionPrompts?: { pendingDamageId: string; targetId: string }[];
  /** The members of the group on turn (a joint turn when more than one). */
  turnGroupIds?: string[];
}

export interface Combatant {
  id: string;
  label: string;
  kind: string;
  /** In a joint turn, this member's part already ended. */
  turnPartEnded?: boolean;
  mine?: boolean;
  hidden?: boolean;
  initiative?: number;
  placed?: boolean;
  col?: number;
  row?: number;
  movementLeftFt?: number;
  hitPointsCurrent?: number;
  defeated?: boolean;
  actionUsed?: boolean;
  state?: string;
  deathSuccesses?: number;
  deathFailures?: number;
  deathSaveDue?: boolean;
  conditions?: string[];
  conditionNamesPt?: string[];
  concentrationSpell?: string;
  armorClassBonus?: number;
}

/** What Pensantus carries in the casting specs: the attack specs' list plus
 * Mãos Flamejantes (a saving throw spell, the area one). */
export const pensantusCasting = {
  ...pensantusAttacks,
  knownSpellKeys: [...pensantusAttacks.knownSpellKeys, 'spell:burning-hands'],
  preparedSpellKeys: [...pensantusAttacks.preparedSpellKeys, 'spell:burning-hands'],
};

/** Toren, a human Fighter 5 (Extra Attack, Retomar o Fôlego, Surto de Ação)
 * with a longsword and chain mail, for the feature specs. */
export const toren: CharacterBuild = {
  name: 'Toren',
  raceKey: 'race:human',
  race: 'Humano',
  classKey: 'class:fighter',
  class: 'Guerreiro',
  subclassKey: 'subclass:champion',
  subclass: 'Campeão',
  level: 5,
  background: 'Soldado',
  backgroundSkillKeys: ['skill:athletics', 'skill:intimidation'],
  backgroundSkills: ['Atletismo', 'Intimidação'],
  extraSkillKeys: ['skill:perception', 'skill:survival'],
  extraSkills: ['Percepção', 'Sobrevivência'],
  scores: { for: 16, des: 12, con: 15, int: 10, sab: 13, car: 8 },
};
export const torenSheet = { weaponKeys: ['equipment:longsword'], armorKey: 'equipment:chain-mail' };

/** The master's correction of a player's vitals (hit points, slots used, resources used). */
export async function adjustVitalsRPC(page: Page, campaignId: string, characterId: string, change: object): Promise<void> {
  const res = await callRPC(page, 'meurpg.play.v1.PlayService/AdjustCharacterVitals', {
    campaignId,
    characterId,
    idempotencyKey: crypto.randomUUID(),
    ...change,
  });
  expect(res.ok(), await res.text()).toBeTruthy();
}

/** Passes the turn (as the master) until `label` is the one on turn. */
/** Waits until the server has passed the turn on from the combatant called `label`. A click on "Encerrar turno"
 * returns before the server answers, so an `EndTurn` sent right after it races the click's own (`aborted`). */
export async function waitTurnLeaves(master: Page, campaignId: string, label: string): Promise<void> {
  await expect.poll(() => turnIsWith(master, campaignId, label)).toBe(false);
}

/** Whether the combatant called `label` is (one of) the one(s) on turn now. */
async function turnIsWith(master: Page, campaignId: string, label: string): Promise<boolean> {
  const e = await getEncounterRPC(master, campaignId);
  const group = e.turnGroupIds?.length ? e.turnGroupIds : [e.currentCombatantId ?? ''];
  return group.some((id) => e.combatants.find((c) => c.id === id)?.label === label);
}

/**
 * The player taps "Encerrar turno" and waits until the server has really passed the turn on from `label`: the tap returns
 * before the call is answered, and a spec that read the turn at once would see the old one still there and go on to the
 * wrong screen.
 */
export async function endTurnOf(player: Page, master: Page, campaignId: string, label: string): Promise<void> {
  await player.getByRole('button', { name: 'Encerrar turno' }).click();
  // Right after an action, the screen may still hold the turn options read before it (they are read again a moment
  // after the action's answer) and ask "Ainda tem 1 ataque desta ação. Encerrar mesmo?": this helper ends the turn
  // anyway. A spec about that question taps the button itself.
  await expect
    .poll(async () => {
      const go = player.locator('.ask__go');
      if (await go.isVisible()) {
        await go.click();
      }
      return turnIsWith(master, campaignId, label);
    })
    .toBe(false);
}

export async function passTurnsTo(master: Page, campaignId: string, label: string): Promise<Encounter> {
  let enc = await getEncounterRPC(master, campaignId);
  for (let i = 0; i < 12; i++) {
    // NPC initiative is rolled, so a tie can make a joint turn: the label may be
    // any member still acting, and each member's part is ended on its own (ending
    // a part that already ended is `aborted`).
    const group = enc.turnGroupIds?.length ? enc.turnGroupIds : [enc.currentCombatantId ?? ''];
    const acting = group
      .map((id) => enc.combatants.find((c) => c.id === id))
      .filter((c): c is Combatant => !!c && !c.turnPartEnded);
    if (acting.some((c) => c.label === label)) {
      return enc;
    }
    enc = await combatRPC(master, 'EndTurn', {
      campaignId,
      encounterId: enc.id,
      expectedCombatantId: acting[0]?.id ?? enc.currentCombatantId,
      discardPendingDamage: true,
    });
  }
  throw new Error(`the turn never reached ${label}`);
}

/**
 * `passTurnsTo`, then waits until `pages` show the change: the page the next action uses (both, `[m, p]`, if both act).
 * `pages` are on the session; a page outside it throws.
 * Use it whenever a page acts next; `passTurnsTo` alone is for a spec that reads the state over RPC or looks at the moment before the page sees the change.
 */
export async function passTurnsToAndWait(master: Page, campaignId: string, label: string, pages: Page | Page[]): Promise<Encounter> {
  const enc = await passTurnsTo(master, campaignId, label);
  await waitForEncounterRevision(pages, enc);
  return enc;
}

/** Brisa, a human Cleric 3 of the Life domain, who prepares Curar Ferimentos. */
export const brisa: CharacterBuild = {
  name: 'Brisa',
  raceKey: 'race:human',
  race: 'Humano',
  classKey: 'class:cleric',
  class: 'Clérigo',
  subclassKey: 'subclass:life',
  subclass: 'Domínio da Vida',
  level: 3,
  background: 'Acólita',
  backgroundSkillKeys: ['skill:arcana', 'skill:nature'],
  backgroundSkills: ['Arcanismo', 'Natureza'],
  extraSkillKeys: ['skill:insight', 'skill:medicine'],
  extraSkills: ['Intuição', 'Medicina'],
  scores: { for: 12, des: 12, con: 14, int: 10, sab: 16, car: 12 },
};
/** Ragna, a human Barbarian 9 with a greataxe: Crítico Brutal (one extra weapon die on a melee critical hit). */
export const ragna: CharacterBuild = {
  name: 'Ragna',
  raceKey: 'race:human',
  race: 'Humano',
  classKey: 'class:barbarian',
  class: 'Bárbaro',
  level: 9,
  background: 'Soldada',
  backgroundSkillKeys: ['skill:athletics', 'skill:intimidation'],
  backgroundSkills: ['Atletismo', 'Intimidação'],
  extraSkillKeys: ['skill:perception', 'skill:survival'],
  extraSkills: ['Percepção', 'Sobrevivência'],
  scores: { for: 16, des: 13, con: 14, int: 10, sab: 10, car: 8 },
};
export const ragnaSheet = { weaponKeys: ['equipment:greataxe'] };

/** Dalila, a human Rogue 11 with expertise in four skills: Talento Confiável counts a d20 of 9 or lower as 10 on them. */
export const dalila: CharacterBuild = {
  name: 'Dalila',
  raceKey: 'race:human',
  race: 'Humano',
  classKey: 'class:rogue',
  class: 'Ladino',
  level: 11,
  background: 'Criminosa',
  backgroundSkillKeys: ['skill:deception', 'skill:sleight-of-hand'],
  backgroundSkills: ['Enganação', 'Prestidigitação'],
  extraSkillKeys: ['skill:acrobatics', 'skill:investigation', 'skill:perception', 'skill:stealth'],
  extraSkills: ['Acrobacia', 'Investigação', 'Percepção', 'Furtividade'],
  scores: { for: 10, des: 16, con: 14, int: 14, sab: 12, car: 8 },
};
export const dalilaSheet = { expertiseSkillKeys: ['skill:acrobatics', 'skill:investigation', 'skill:perception', 'skill:stealth'] };

export const brisaSheet = {
  weaponKeys: ['equipment:mace'],
  preparedSpellKeys: ['spell:cure-wounds'],
};
/** Brisa with Ajuda prepared (a 2nd-level spell: she has two such slots at level 3). */
export const brisaAidSheet = { ...brisaSheet, preparedSpellKeys: ['spell:cure-wounds', 'spell:aid'] };
