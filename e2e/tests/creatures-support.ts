import { expect, type Page } from '@playwright/test';

import { startSessionRPC } from './live-session-support';
import { tableForMaps, type MapsTable } from './maps-support';
import { callRPC, pensantus, type CharacterBuild } from './support';

// Setup for the creatures specs (Etapa 9, MR-037), through the API: these tests prove the
// creature screens, not the campaign and character forms other specs cover. Every test
// makes its own campaign.

export interface CreaturesTable extends MapsTable {
  /** Set when a session was started. */
  sessionId: string;
}

/** Pensantus at level 4 with Convocar Familiar in the spellbook and prepared. */
const wizardWithFamiliar = {
  cantripKeys: ['spell:fire-bolt'],
  knownSpellKeys: ['spell:find-familiar', 'spell:magic-missile', 'spell:shield', 'spell:sleep'],
  preparedSpellKeys: ['spell:find-familiar', 'spell:magic-missile', 'spell:shield', 'spell:sleep'],
};

/**
 * A table for the creatures: Pensantus (a level 4 wizard who has Convocar Familiar), and,
 * when `session` is true, an open session (a creature is cast during one). Master and
 * player pages must be signed in as each; neither navigates.
 */
export async function tableForCreatures(master: Page, player: Page, name: string, session = true): Promise<CreaturesTable> {
  const base = await tableForMaps(master, player, name, false, wizardWithFamiliar, { ...pensantus, level: 4 });
  const sessionId = session ? await startSessionRPC(master, base.campaignId) : '';
  return { ...base, sessionId };
}

/** The character's creatures as the app reads them (the owner's player and the master). */
export async function listCreaturesRPC(page: Page, campaignId: string, characterId: string): Promise<{ id: string; name: string; hitPointsCurrent?: number; hitPointsMax?: number; monsterKey: string; source: string; armorClass?: number }[]> {
  const res = await callRPC(page, 'meurpg.characters.v1.CharacterService/ListCharacterCreatures', { campaignId, characterId });
  expect(res.ok()).toBeTruthy();
  return ((await res.json()).creatures ?? []) as never;
}

/** The master gives the character a creature by its SRD key. */
export async function giveCreatureRPC(master: Page, campaignId: string, characterId: string, monsterKey: string, name = ''): Promise<string> {
  const res = await callRPC(master, 'meurpg.characters.v1.CharacterService/GiveCreature', { campaignId, characterId, monsterKey, name });
  expect(res.ok()).toBeTruthy();
  return (await res.json()).creature.id as string;
}

/** A druid (level 5, Conjurar Animais prepared) or a cleric (level 5, Animar Mortos prepared) with an open session. */
export async function tableForCaster(master: Page, player: Page, name: string, caster: 'druid' | 'cleric'): Promise<CreaturesTable> {
  const build: CharacterBuild = {
    name: caster === 'druid' ? 'Sálvia' : 'Irmã Clara',
    raceKey: 'race:human',
    race: 'Humano',
    classKey: caster === 'druid' ? 'class:druid' : 'class:cleric',
    class: caster === 'druid' ? 'Druida' : 'Clérigo',
    subclassKey: caster === 'cleric' ? 'subclass:life' : undefined,
    level: 5,
    backgroundKey: 'background:acolyte',
    backgroundSkillKeys: [],
    backgroundSkills: [],
    extraSkillKeys: caster === 'druid' ? ['skill:nature', 'skill:perception'] : ['skill:medicine', 'skill:persuasion'],
    extraSkills: [],
    scores: { for: 10, des: 12, con: 14, int: 10, sab: 16, car: 12 },
  };
  const spells = caster === 'druid' ? ['spell:conjure-animals', 'spell:cure-wounds'] : ['spell:animate-dead', 'spell:bless'];
  const base = await tableForMaps(master, player, name, false, { preparedSpellKeys: spells }, build);
  return { ...base, sessionId: await startSessionRPC(master, base.campaignId) };
}
