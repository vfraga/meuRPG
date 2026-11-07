import { expect, type Page } from '@playwright/test';

import { createEntryRPC, listEntriesJSON, raceBody } from './content-support';
import { setSwitchesRPC } from './content-options-support';
import { toren } from './levelup-support';
import { setTableRulesRPC } from './table-rules-support';
import type { CharacterBuild } from './support';

// Setup for the a11y rows of the race, subrace and feat screens of the editor (MR-025, RN-23): a table race whose
// trait asks the player to pick one of two options, a subrace that replaces one trait and the bonuses of its race, and feats that ask
// for races, a size and a feature the character must not have. Everything comes through the API, so a row proves its screen and not
// the editors; invented names, as the other content specs have.

/** The race Corujeiro with the trait "Sentidos da noite": the player picks "Olhos de coruja" or "Passo de pena", once. */
export function nightRaceBody(namePt = 'Corujeiro'): Record<string, unknown> {
  const body = raceBody(namePt);
  const traits = body.traits as Record<string, unknown>[];
  traits.push({
    namePt: 'Sentidos da noite',
    descPt: ['O corujeiro nasce afinado com a noite. Escolha um ao criar o personagem.'],
    options: [
      { namePt: 'Olhos de coruja', descPt: ['Enxerga no escuro como na penumbra.'], effects: [{ type: 'sense', sense: 'darkvision', rangeFt: 120 }] },
      { namePt: 'Passo de pena', descPt: ['Você anda sem fazer barulho, nem nas folhas secas.'], effects: [{ type: 'proficiency', proficiency: 'skill:stealth' }] },
    ],
  });
  return body;
}

export interface NightRace {
  /** The race entry's key. */
  race: string;
  /** The keys of its traits, in order: "Olhos de caçador", "Planar", "Sentidos da noite". */
  traits: string[];
}

/** Creates the race of `nightRaceBody` and reads back the keys the server made for its traits. */
export async function createNightRaceRPC(master: Page, campaignId: string, namePt = 'Corujeiro'): Promise<NightRace> {
  const race = await createEntryRPC(master, campaignId, 'tableRace', nightRaceBody(namePt));
  const entry = (await listEntriesJSON(master, campaignId)).entries!.find((e) => e.key === race)!;
  const traits = (entry.tableRace.traits as { key: string }[]).map((t) => t.key);
  expect(traits).toHaveLength(3);
  return { race, traits };
}

/** "Corujeiro das Neves": +2 Constituição and +1 Sabedoria in place of the race's bonuses, one trait of its own, "Olhos de caçador" replaced. */
export async function createSnowSubraceRPC(master: Page, campaignId: string, parent: NightRace): Promise<string> {
  return createEntryRPC(master, campaignId, 'tableSubrace', {
    namePt: 'Corujeiro das Neves',
    raceKey: parent.race,
    abilityBonuses: { constitution: 2, wisdom: 1 },
    replacesAbilityBonuses: true,
    replacesTraitKeys: [parent.traits[0]],
    traits: [{ namePt: 'Pele de geada', descPt: ['Resistência a dano de frio.'], effects: [{ type: 'note', textPt: 'Resistência a dano de frio.' }] }],
  });
}

export interface FeatTable {
  /** "Pequeno e Escorregadio": Destreza 13, Gnomo, Halfling or Corujeiro das Cinzas, Pequeno, and not Agarrador. */
  smallAndSlippery: string;
  /** "Passo Silencioso": Destreza 13 and not "Dom Oculto", which the master switched off. */
  quietStep: string;
  /** "Mão Leve": not Retomar o Fôlego (the feature of every Guerreiro). */
  lightHand: string;
}

/**
 * The feats of the boards, in a campaign with "Talentos" on: one that asks for races (one of them switched off), a size and a feature the
 * character must not have; one whose condition names a feat the master switched off (the player reads "uma condição que a mesa não
 * mostra"); one that asks the character not to have a feature that every Guerreiro has.
 */
export async function createFeatsRPC(master: Page, campaignId: string): Promise<FeatTable> {
  await setTableRulesRPC(master, campaignId, { featsAllowed: true });
  const ashes = await createEntryRPC(master, campaignId, 'tableRace', raceBody('Corujeiro das Cinzas'));
  const smallAndSlippery = await createEntryRPC(master, campaignId, 'tableFeat', {
    namePt: 'Pequeno e Escorregadio',
    descPt: ['Você escapa por onde ninguém passa: sai de uma criatura que o agarra com o deslocamento normal.'],
    prerequisite: { minimums: { dexterity: 13 }, races: ['race:gnome', 'race:halfling', ashes], size: 'Small', lacksFeatureKey: 'feat:grappler' },
  });
  const hidden = await createEntryRPC(master, campaignId, 'tableFeat', { namePt: 'Dom Oculto', descPt: ['Escondido da lista.'] });
  const quietStep = await createEntryRPC(master, campaignId, 'tableFeat', {
    namePt: 'Passo Silencioso',
    descPt: ['Você anda sem fazer barulho.'],
    prerequisite: { minimums: { dexterity: 13 }, lacksFeatureKey: hidden },
  });
  const lightHand = await createEntryRPC(master, campaignId, 'tableFeat', {
    namePt: 'Mão Leve',
    descPt: ['Seus dedos são rápidos.'],
    prerequisite: { lacksFeatureKey: 'feature:second-wind' },
  });
  await setSwitchesRPC(master, campaignId, [
    { key: ashes, off: true },
    { key: hidden, off: true },
  ]);
  return { smallAndSlippery, quietStep, lightHand };
}

/** Tor, a Guerreiro of level 3 who goes to 4 (the level of the first ability increase or feat): Humano, Força 16 and Destreza 14. */
export const tor: CharacterBuild = {
  ...toren,
  name: 'Tor',
  level: 3,
  scores: { for: 16, des: 14, con: 16, int: 10, sab: 12, car: 8 },
};
