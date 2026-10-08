import { expect, type Page } from '@playwright/test';

import { combatRPC, getEncounterRPC, setGridRPC, startEncounterRPC, toren, torenSheet, type CombatTable, type Encounter } from './combat-support';
import { CAVE, CAVE_COLUMNS, caveImage, squareBp } from './fog-support';
import { endOpenSessionRPC, startSessionRPC } from './live-session-support';
import { createMapRPC, placeTokenRPC, revealMapRPC, setCurrentMapRPC, uploadImageRPC } from './maps-support';
import { boxOf, callRPC, characterRpcBody, createCharacterRPC, pensantus, type CharacterBuild } from './support';

// Setup for the specs of the creatures in combat and Wild Shape (Etapa 9, MR-037, RN-20), through the API:
// the cave "A caverna do Vale Seco" without fog, a druid (Sálvia) or Pensantus with his familiar, Toren as the
// other player, three goblins in the guard room, and the combat begun with the faces the test asks for.
// Every test makes its own campaign. The pages must be signed in as each person; none navigates.

/** Sálvia, a human Druid 5 who prepares Conjurar Animais and Curar Ferimentos, with a quarterstaff. */
export const salvia: CharacterBuild = {
  name: 'Sálvia',
  raceKey: 'race:human',
  race: 'Humano',
  classKey: 'class:druid',
  class: 'Druida',
  level: 5,
  backgroundKey: 'background:acolyte',
  backgroundSkillKeys: [],
  backgroundSkills: [],
  extraSkillKeys: ['skill:nature', 'skill:perception'],
  extraSkills: ['Natureza', 'Percepção'],
  scores: { for: 10, des: 14, con: 14, int: 10, sab: 16, car: 12 },
};
export const salviaSheet = { weaponKeys: ['equipment:quarterstaff'], preparedSpellKeys: ['spell:conjure-animals', 'spell:cure-wounds', 'spell:entangle'] };

export interface CreatureCombatTable {
  campaignId: string;
  sessionId: string;
  mapId: string;
  /** The first player's character (Sálvia, or Pensantus), Toren, and the NPCs. */
  heroId: string;
  torenId: string;
  goblin1Id: string;
  goblin2Id: string;
  captainId: string;
  /** Nanquim, Pensantus's raven, when `nanquim` was asked for. */
  familiarId?: string;
}

async function join(master: Page, player: Page, campaignId: string): Promise<void> {
  const invite = await callRPC(master, 'meurpg.campaigns.v1.CampaignService/CreateInvite', { campaignId, maxUses: 1, expiresIn: '3600s' });
  expect(invite.ok()).toBeTruthy();
  const accepted = await callRPC(player, 'meurpg.campaigns.v1.CampaignService/AcceptInvite', { token: (await invite.json()).token });
  expect(accepted.ok()).toBeTruthy();
}

async function minion(master: Page, campaignId: string, name: string): Promise<string> {
  const res = await createCharacterRPC(master, campaignId, {
    kind: 'CHARACTER_KIND_MINION',
    name,
    sheet: {
      basic: {
        hitPointsMax: 7,
        armorClass: 15,
        speedFt: 30,
        attackBonus: 4,
        damage: '1d6+2',
        description: '',
        attacks: [{ name: 'Cimitarra', attackBonus: 4, damageDiceCount: 1, damageDiceSides: 6, damageBonus: 2, damageType: 'DAMAGE_TYPE_SLASHING', rangeFt: 5 }],
      },
    },
  });
  expect(res.ok(), await res.text()).toBeTruthy();
  return (await res.json()).character.id as string;
}

export interface CreatureCombatOptions {
  /** The first player plays Pensantus (a familiar) instead of Sálvia. */
  hero?: 'salvia' | 'pensantus';
  /** Pensantus has Nanquim (Convocar Familiar, a ritual: no slot). */
  nanquim?: boolean;
}

/**
 * The cave at the table: a campaign with the first player's druid or wizard and Toren, three goblins, an open session
 * and the cave as the map (24 columns, no fog), and every token placed. The hero stands at the corridor mouth.
 */
export async function tableForCreatureCombat(master: Page, heroPlayer: Page, torenPlayer: Page, name: string, options: CreatureCombatOptions = {}): Promise<CreatureCombatTable> {
  const created = await callRPC(master, 'meurpg.campaigns.v1.CampaignService/CreateCampaign', { name, xpMode: 'XP_MODE_ENEMIES' });
  expect(created.ok()).toBeTruthy();
  const campaignId = (await created.json()).campaign.id as string;

  await join(master, heroPlayer, campaignId);
  const druid = (options.hero ?? 'salvia') === 'salvia';
  const heroBody = characterRpcBody('PLAYER', druid ? salvia : pensantus) as { sheet: { full: object } };
  heroBody.sheet.full = {
    ...heroBody.sheet.full,
    ...(druid ? salviaSheet : { knownSpellKeys: ['spell:find-familiar'], preparedSpellKeys: ['spell:find-familiar'], cantripKeys: ['spell:fire-bolt'], weaponKeys: ['equipment:dagger'] }),
  };
  const hero = await createCharacterRPC(heroPlayer, campaignId, heroBody);
  expect(hero.ok(), await hero.text()).toBeTruthy();
  const heroId = (await hero.json()).character.id as string;

  await join(master, torenPlayer, campaignId);
  const torenBody = characterRpcBody('PLAYER', toren) as { sheet: { full: object } };
  torenBody.sheet.full = { ...torenBody.sheet.full, ...torenSheet };
  const torenRes = await createCharacterRPC(torenPlayer, campaignId, torenBody);
  expect(torenRes.ok(), await torenRes.text()).toBeTruthy();
  const torenId = (await torenRes.json()).character.id as string;

  const goblin1Id = await minion(master, campaignId, 'Goblin 1');
  const goblin2Id = await minion(master, campaignId, 'Goblin 2');
  const captain = await createCharacterRPC(master, campaignId, characterRpcBody('ENEMY', { ...pensantus, name: 'Capitão Goblin' }));
  expect(captain.ok(), await captain.text()).toBeTruthy();
  const captainId = (await captain.json()).character.id as string;

  // The session is open from here: a setup that fails after it still ends it, so no open session is left behind.
  try {
    const sessionId = await startSessionRPC(master, campaignId);
    await master.goto('/');
    const image = await uploadImageRPC(master, campaignId, 'A caverna do Vale Seco', await caveImage(master));
    const mapId = await createMapRPC(master, campaignId, 'A caverna do Vale Seco', image);
    await revealMapRPC(master, campaignId, mapId);
    await setCurrentMapRPC(master, campaignId, mapId);
    await setGridRPC(master, campaignId, mapId, CAVE_COLUMNS);
    const place = (id: string, col: number, row: number) => {
      const { xBp, yBp } = squareBp(col, row);
      return placeTokenRPC(master, campaignId, mapId, id, xBp, yBp);
    };
    await place(heroId, 5, 8);
    await place(torenId, 6, 7);
    for (const [id, col, row] of [[goblin1Id, 18, 5], [goblin2Id, 20, 7], [captainId, 21, 3]] as const) {
      await place(id, col, row);
      const shown = await callRPC(master, 'meurpg.maps.v1.MapService/SetMapTokenHidden', { campaignId, mapId, characterId: id, hidden: false });
      expect(shown.ok(), await shown.text()).toBeTruthy();
    }

    let familiarId: string | undefined;
    if (options.nanquim) {
      const cast = await callRPC(heroPlayer, 'meurpg.play.v1.PlayService/CastSummon', {
        campaignId,
        characterId: heroId,
        spellKey: 'spell:find-familiar',
        ritual: true,
        summon: { option: 0, creatureKeys: ['monster:raven'], names: ['Nanquim'] },
        idempotencyKey: crypto.randomUUID(),
      });
      expect(cast.ok(), await cast.text()).toBeTruthy();
      familiarId = (await cast.json()).creatureIds[0] as string;
      const { xBp, yBp } = squareBp(5, 9);
      const placed = await callRPC(master, 'meurpg.maps.v1.MapService/PlaceMapToken', { campaignId, mapId, creatureId: familiarId, xBp, yBp });
      expect(placed.ok(), await placed.text()).toBeTruthy();
    }
    return { campaignId, sessionId, mapId, heroId, torenId, goblin1Id, goblin2Id, captainId, familiarId };
  } catch (err) {
    await endOpenSessionRPC(master, campaignId).catch(() => undefined);
    throw err;
  }
}

/** A trap on a square of the cave (24 x 16), born hidden, that fires when someone enters it; a flat damage so the number is known. */
export async function trapAt(master: Page, campaignId: string, mapId: string, name: string, col: number, row: number, damage = '3'): Promise<void> {
  const res = await callRPC(master, 'meurpg.maps.v1.MapService/CreateMapPoint', {
    campaignId,
    mapId,
    kind: 'MAP_POINT_KIND_TRAP',
    name,
    description: 'No corredor',
    ...squareBp(col, row),
    trap: { noticeDc: 30, findDc: 5, areaSize: 1, trigger: 'TRAP_TRIGGER_ENTER', effect: { damage: [{ dice: damage, damageTypeKey: 'damage-type:bludgeoning' }] } },
  });
  expect(res.ok(), await res.text()).toBeTruthy();
}

/** The session page's route. */
export function sessionRoute(campaignId: string): string {
  return `/campaigns/${campaignId}/session`;
}

/**
 * Starts the combat of the party and the three goblins and rolls everyone's initiative by label (the master types the
 * NPCs' faces, each player their own, as a physical d20 each), places the goblins and begins. Players' pages by label.
 */
export async function beginCreatureCombat(
  master: Page,
  table: CreatureCombatTable,
  players: Record<string, Page>,
  faces: Record<string, number>,
  at: Record<string, [number, number]> = { 'Capitão Goblin': [21, 3], 'Goblin 1': [18, 5], 'Goblin 2': [20, 7] },
): Promise<Encounter> {
  const combatTable = { campaignId: table.campaignId, captainId: table.captainId, goblinId: table.goblin1Id } as unknown as CombatTable;
  let enc = await startEncounterRPC(master, combatTable, [
    { characterId: table.captainId, count: 1, hidden: false },
    { characterId: table.goblin1Id, count: 1, hidden: false },
    { characterId: table.goblin2Id, count: 1, hidden: false },
  ]);
  for (const c of enc.combatants) {
    const page = players[c.label] ?? master;
    const res = await callRPC(page, 'meurpg.play.v1.CombatService/SubmitInitiative', {
      campaignId: table.campaignId,
      encounterId: enc.id,
      combatantId: c.id,
      idempotencyKey: crypto.randomUUID(),
      d20Face: faces[c.label] ?? 2,
    });
    expect(res.ok(), await res.text()).toBeTruthy();
    enc = (await res.json()).encounter as Encounter;
  }
  for (const [label, [col, row]] of Object.entries(at)) {
    const id = enc.combatants.find((c) => c.label === label)!.id;
    enc = await combatRPC(master, 'MoveCombatant', { campaignId: table.campaignId, encounterId: enc.id, combatantId: id, col, row });
  }
  return combatRPC(master, 'BeginCombat', { campaignId: table.campaignId, encounterId: enc.id });
}

/** The rows of the cave, for a spec that wants to pick a floor square. */
export const floorAt = (col: number, row: number): boolean => CAVE[row]?.[col] === '.';

/** Taps a square of the cave's 24 x 16 grid on the battle map ("Mover" page). */
export async function tapCaveSquare(page: Page, col: number, row: number): Promise<void> {
  const map = page.getByRole('group', { name: /Mapa de batalha/ });
  await expect(map).toBeVisible();
  const box = await boxOf(map);
  await map.click({ position: { x: ((col + 0.5) * box.width) / 24, y: ((row + 0.5) * box.height) / 16 } });
}

/** Passes the master's turns until nobody of `labels` acts any more (the NPCs' turns), or the turn reaches `target`. */
export { passTurnsTo } from './combat-support';

/**
 * The master has Goblin 1 hit `targetLabel` (as a reaction, so it works off its turn) and applies `amount` of damage: the
 * damage of a player's character waits for the master, who may type another number (RN-02).
 */
export async function hitAndApply(master: Page, campaignId: string, attackerLabel: string, targetLabel: string, amount: number): Promise<void> {
  const enc = await getEncounterRPC(master, campaignId);
  const id = (label: string) => enc.combatants.find((c) => c.label === label)!.id;
  const hit = await callRPC(master, 'meurpg.play.v1.CombatService/RollAttack', {
    campaignId,
    encounterId: enc.id,
    attackerId: id(attackerLabel),
    attackKey: 'basic:0',
    targetId: id(targetLabel),
    idempotencyKey: crypto.randomUUID(),
    d20Face: 20,
    asReaction: true,
  });
  expect(hit.ok(), await hit.text()).toBeTruthy();
  const pending = (await hit.json()).pendingDamage as { id: string };
  const rolled = await callRPC(master, 'meurpg.play.v1.CombatService/RollDamage', {
    campaignId,
    encounterId: enc.id,
    pendingDamageId: pending.id,
    idempotencyKey: crypto.randomUUID(),
    typedSum: 3,
  });
  expect(rolled.ok(), await rolled.text()).toBeTruthy();
  const applied = await callRPC(master, 'meurpg.play.v1.CombatService/ApplyPendingDamage', {
    campaignId,
    encounterId: enc.id,
    pendingDamageId: pending.id,
    idempotencyKey: crypto.randomUUID(),
    amount,
  });
  expect(applied.ok(), await applied.text()).toBeTruthy();
}
