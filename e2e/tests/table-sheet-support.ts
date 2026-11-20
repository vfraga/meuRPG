import { expect, type Page } from '@playwright/test';

import { startSessionRPC } from './live-session-support';
import { markMilestoneRPC } from './levelup-support';
import { createInkBladeRPC } from './spells-support';
import { callRPC, createCharacterRPC } from './support';

// Setup for the character editor and level-up specs with the table's own content (MR-025, RN-23, MR-040, slice
// 10.12b), through the API: the content editors are another slice, so the master's entries are written here with
// TableContentService, the way the editors will. These tests prove the player's screens, not the master's. Every
// test makes its own campaign.

export interface SheetTable {
  campaignId: string;
}

/** A campaign of Mestre Teste's with Jogador Teste in it (no character yet). */
export async function emptyTable(master: Page, player: Page, name: string, xpMode = 'XP_MODE_MILESTONES'): Promise<SheetTable> {
  const created = await callRPC(master, 'meurpg.campaigns.v1.CampaignService/CreateCampaign', { name, xpMode });
  expect(created.ok(), await created.text()).toBeTruthy();
  const campaignId = (await created.json()).campaign.id as string;
  const invite = await callRPC(master, 'meurpg.campaigns.v1.CampaignService/CreateInvite', { campaignId, maxUses: 1, expiresIn: '3600s' });
  expect(invite.ok()).toBeTruthy();
  const accepted = await callRPC(player, 'meurpg.campaigns.v1.CampaignService/AcceptInvite', { token: (await invite.json()).token });
  expect(accepted.ok()).toBeTruthy();
  return { campaignId };
}

async function post(master: Page, method: string, body: object): Promise<any> {
  const res = await callRPC(master, `meurpg.rules.v1.TableContentService/${method}`, body);
  expect(res.ok(), `${method}: ${await res.text()}`).toBeTruthy();
  return res.json();
}

/** The server's own 20-row table of one way of casting, which the class editor starts from. */
async function defaultRows(master: Page, campaignId: string, kind: string, preparation: string): Promise<{ rows: any[]; startLevel: number }> {
  const defaults = await post(master, 'GetClassTableDefaults', { campaignId });
  const table = (defaults.tables as any[]).find((t) => (t.kind ?? '') === kind && (t.preparation ?? '') === preparation);
  expect(table, `a default table for ${kind}/${preparation}`).toBeTruthy();
  return { rows: table.rows, startLevel: table.startLevel ?? 1 };
}

const VALLEY_SKILLS = ['skill:animal-handling', 'skill:athletics', 'skill:nature', 'skill:perception', 'skill:stealth', 'skill:survival'];

/** "Guardião do Vale": a half caster that prepares from the druid's list, a sense at level 1, a fighting style at 2. */
export async function createGuardianRPC(master: Page, campaignId: string, over: Record<string, unknown> = {}): Promise<string> {
  const { rows } = await defaultRows(master, campaignId, 'half', 'prepared');
  const menu = await post(master, 'GetEffectMenu', { campaignId });
  const styles = (menu.optionSets as any[]).map((s) => (s.options as any[]).map((o) => o.key as string)).find((keys) => keys.includes('feature:fighter-fighting-style-defense'))!;
  const levels = rows.map((r) => ({ ...r }));
  levels[0].features = [{ namePt: 'Olhos do Vale', descPt: ['Você enxerga no escuro.'], effects: [{ type: 'sense', sense: 'darkvision', rangeFt: 60 }] }];
  levels[1].features = [{ namePt: 'Estilo de luta', effects: [{ type: 'choice', choice: 'feature', count: 1, from: styles.slice(0, 3) }] }];
  const body = await post(master, 'CreateTableEntry', {
    campaignId,
    tableClass: {
      namePt: 'Guardião do Vale',
      hitDie: 10,
      savingThrows: ['ABILITY_STRENGTH', 'ABILITY_WISDOM'],
      skillChoose: 2,
      skillFrom: VALLEY_SKILLS,
      proficiencies: ['proficiency:light-armor', 'proficiency:simple-weapons'],
      minimums: { wisdom: 13 },
      subclassLevel: 3,
      casting: { kind: 'half', ability: 'ABILITY_WISDOM', preparation: 'prepared', listFrom: 'class:druid' },
      levels,
      ...over,
    },
  });
  return body.entry.key as string;
}

/** "Caminho do Vale": the guardian's subclass, always preparing Curar Ferimentos from the class level 3. */
export async function createValleyPathRPC(master: Page, campaignId: string, classKey: string): Promise<string> {
  const body = await post(master, 'CreateTableEntry', {
    campaignId,
    tableSubclass: {
      namePt: 'Caminho do Vale',
      classKey,
      descPt: ['Quem anda o vale inteiro.'],
      levels: [{ level: 3, features: [{ namePt: 'Passo do Vale', effects: [{ type: 'modifier', target: 'speed.walk', mode: 'add', value: '5' }] }] }],
      alwaysPrepared: [{ classLevel: 3, spellKey: 'spell:cure-wounds' }],
    },
  });
  return body.entry.key as string;
}

/** A subclass of an SRD class with no extra mechanics, as a cleric's domain or a wizard's tradition; `startLevel` is the level the class chooses it at. */
export async function createPlainSubclassRPC(master: Page, campaignId: string, classKey: string, namePt: string, startLevel: number, alwaysPrepared: { classLevel: number; spellKey: string }[] = []): Promise<string> {
  const body = await post(master, 'CreateTableEntry', {
    campaignId,
    tableSubclass: {
      namePt,
      classKey,
      descPt: ['Escrita pelo mestre.'],
      levels: [{ level: startLevel, features: [{ namePt: `${namePt}: o começo`, descPt: ['Só texto.'] }] }],
      ...(alwaysPrepared.length > 0 ? { alwaysPrepared } : {}),
    },
  });
  return body.entry.key as string;
}

/** "Lâmina de Tinta": a third caster of the Fighter, casting from the wizard's list. */
export async function createInkBladeSubclassRPC(master: Page, campaignId: string): Promise<string> {
  const { rows, startLevel } = await defaultRows(master, campaignId, 'third', 'known');
  const levels = rows.slice(startLevel - 1).map((r, i) => ({
    level: startLevel + i,
    cantripsKnown: r.cantripsKnown,
    spellsKnown: r.spellsKnown,
    slots: r.slots,
    ...(startLevel + i === 3 ? { features: [{ namePt: 'Lâmina entintada', descPt: ['A lâmina pinga tinta.'] }] } : {}),
  }));
  const body = await post(master, 'CreateTableEntry', {
    campaignId,
    tableSubclass: {
      namePt: 'Lâmina de Tinta',
      classKey: 'class:fighter',
      descPt: ['Tinta e aço.'],
      casting: { kind: 'third', ability: 'ABILITY_INTELLIGENCE', preparation: 'known', listFrom: 'class:wizard' },
      levels,
    },
  });
  return body.entry.key as string;
}

export async function createOwlRaceRPC(master: Page, campaignId: string): Promise<string> {
  const body = await post(master, 'CreateTableEntry', {
    campaignId,
    tableRace: {
      namePt: 'Corujeiro',
      size: 'Medium',
      speedFt: 30,
      abilityBonuses: { wisdom: 1 },
      choiceBonuses: [2, 1],
      darkvisionFt: 60,
      languages: ['language:common'],
      traits: [{ namePt: 'Olhar de coruja', descPt: ['Enxerga longe na penumbra.'] }],
    },
  });
  return body.entry.key as string;
}

export async function createMapperBackgroundRPC(master: Page, campaignId: string): Promise<string> {
  const body = await post(master, 'CreateTableEntry', {
    campaignId,
    tableBackground: {
      namePt: 'Cartógrafo do Vale',
      skills: ['skill:nature', 'skill:survival'],
      tools: ['proficiency:cartographers-tools'],
      equipmentPt: 'Uma luneta e um rolo de corda.',
      feature: { namePt: 'Mapa vivo', descPt: ['Lembra de todo caminho que já andou.'] },
    },
  });
  return body.entry.key as string;
}

export { createInkBladeRPC };

/** Ícaro as Davi made him: the guardian at level 1, "Outro" as the background, the XP of level 2 (300). */
export function icaroSheetBody(classKey: string, experiencePoints = 300): Record<string, unknown> {
  return {
    kind: 'CHARACTER_KIND_PLAYER',
    name: 'Ícaro',
    sheet: {
      full: {
        baseScores: { strength: 14, dexterity: 13, constitution: 14, intelligence: 10, wisdom: 15, charisma: 8 },
        raceKey: 'race:human',
        classes: [{ classKey, level: 1 }],
        customBackground: {
          name: 'Batedor de torre',
          skillKeys: ['skill:perception', 'skill:survival'],
          proficiencyKeys: ['proficiency:thieves-tools', 'language:elvish'],
          featureName: 'Olho no horizonte',
          featureText: 'Você sempre acha o ponto mais alto de um lugar.',
          equipment: 'Uma luneta, um rolo de corda e 10 PO.',
        },
        skillProficiencyKeys: ['skill:athletics', 'skill:nature'],
        hitPoints: { method: 'HIT_POINTS_METHOD_AVERAGE' },
        experiencePoints,
      },
    },
  };
}

/** Creates a sheet for the player through the API and returns its id. */
export async function createSheetRPC(player: Page, campaignId: string, body: Record<string, unknown>): Promise<string> {
  const res = await createCharacterRPC(player, campaignId, body);
  expect(res.ok(), await res.text()).toBeTruthy();
  return (await res.json()).character.id as string;
}

/** A session open (the sheet locks, RN-01) and the milestone that lets the character level up (RN-12). */
export async function lockAndMilestone(master: Page, campaignId: string, characterId: string): Promise<void> {
  await startSessionRPC(master, campaignId);
  await markMilestoneRPC(master, campaignId, 'Chegar ao Vale Seco', [characterId]);
}

/** The master changes the guardian's skill count: every sheet that uses it recalculates at once (RN-23, question 80). */
export async function changeGuardianSkillsRPC(master: Page, campaignId: string, key: string, skillChoose: number): Promise<void> {
  const listed = await post(master, 'ListTableEntries', { campaignId });
  const entry = (listed.entries as any[]).find((e) => e.key === key)!;
  const body = { ...entry.tableClass, skillChoose };
  await post(master, 'UpdateTableEntry', { campaignId, key, expectedRevision: entry.revision, tableClass: body });
}

/** A table race whose trait lets the player choose one skill from `from` (#574, part D). */
export function skillTraitRaceBody(from: string[]): Record<string, unknown> {
  return {
    namePt: 'Rastreador',
    size: 'Medium',
    speedFt: 30,
    abilityBonuses: { wisdom: 1 },
    languages: ['language:common'],
    traits: [{ namePt: 'Olho de rastro', descPt: ['Você aprende a achar o que se esconde.'], effects: [{ type: 'choice', choice: 'skill', count: 1, from }] }],
  };
}

/** The guardian's skill list, as the master changes it. */
export async function changeGuardianSkillListRPC(master: Page, campaignId: string, key: string, skillFrom: string[]): Promise<void> {
  const listed = await post(master, 'ListTableEntries', { campaignId });
  const entry = (listed.entries as any[]).find((e) => e.key === key)!;
  await post(master, 'UpdateTableEntry', { campaignId, key, expectedRevision: entry.revision, tableClass: { ...entry.tableClass, skillFrom } });
}
