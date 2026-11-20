import { expect, type Locator, type Page } from '@playwright/test';

import { startSessionRPC } from './live-session-support';
import { callRPC, characterRpcBody, createCharacterRPC, pensantus, type CharacterBuild } from './support';

// Setup for the guided level-up specs (Etapa 8, MR-040, RN-01, RN-12), through the API: these tests
// prove the level-up screens, not the campaign, invite and character ones other specs cover. Every
// test makes its own campaign.

export interface LevelUpTable {
  campaignId: string;
  characterId: string;
}

/** Pensantus with the book of the artboards: 3 cantrips, 10 spells in the book, 7 of them prepared. */
export const pensantusBook = {
  cantripKeys: ['spell:fire-bolt', 'spell:mage-hand', 'spell:ray-of-frost'],
  knownSpellKeys: [
    'spell:magic-missile',
    'spell:shield',
    'spell:mage-armor',
    'spell:burning-hands',
    'spell:sleep',
    'spell:scorching-ray',
    'spell:web',
    'spell:detect-magic',
    'spell:comprehend-languages',
    'spell:find-familiar',
  ],
  preparedSpellKeys: [
    'spell:magic-missile',
    'spell:shield',
    'spell:mage-armor',
    'spell:burning-hands',
    'spell:sleep',
    'spell:scorching-ray',
    'spell:web',
  ],
};

/** Toren: a Guerreiro 4 (Campeão), Con 16: the level 5 has nothing to choose but the hit points. */
export const toren: CharacterBuild = {
  name: 'Toren',
  raceKey: 'race:human',
  race: 'Humano',
  classKey: 'class:fighter',
  class: 'Guerreiro',
  subclassKey: 'subclass:champion',
  level: 4,
  background: 'Soldado',
  backgroundSkillKeys: ['skill:athletics', 'skill:intimidation'],
  backgroundSkills: ['Atletismo', 'Intimidação'],
  extraSkillKeys: ['skill:perception', 'skill:survival'],
  extraSkills: ['Percepção', 'Sobrevivência'],
  scores: { for: 16, des: 12, con: 16, int: 10, sab: 12, car: 8 },
};

/**
 * A campaign of Mestre Teste's with Jogador Teste in it, playing `build` (Pensantus by default,
 * with the book), a session open (the sheet is locked) and, in a milestones campaign, the master
 * having marked a milestone for the character. `masterPage` and `playerPage` must be signed in as
 * each; neither navigates. End the session with `endOpenSessionRPC` in a `finally`.
 */
export async function tableForLevelUp(
  masterPage: Page,
  playerPage: Page,
  name: string,
  options: {
    build?: CharacterBuild;
    sheet?: Record<string, unknown>;
    milestone?: boolean;
    xpMode?: string;
    session?: boolean;
    /** Content the sheet needs, made in the campaign before the character: the sheet fields it gives are added to `sheet`. */
    prepare?: (campaignId: string) => Promise<Record<string, unknown>>;
  } = {},
): Promise<LevelUpTable> {
  const build = options.build ?? pensantus;
  const created = await callRPC(masterPage, 'meurpg.campaigns.v1.CampaignService/CreateCampaign', {
    name,
    xpMode: options.xpMode ?? 'XP_MODE_MILESTONES',
  });
  expect(created.ok(), await created.text()).toBeTruthy();
  const campaignId = (await created.json()).campaign.id as string;
  const invite = await callRPC(masterPage, 'meurpg.campaigns.v1.CampaignService/CreateInvite', { campaignId, maxUses: 1, expiresIn: '3600s' });
  expect(invite.ok()).toBeTruthy();
  const accepted = await callRPC(playerPage, 'meurpg.campaigns.v1.CampaignService/AcceptInvite', { token: (await invite.json()).token });
  expect(accepted.ok()).toBeTruthy();

  const prepared = options.prepare ? await options.prepare(campaignId) : {};
  const body = characterRpcBody('PLAYER', build) as { sheet: { full: object } };
  body.sheet.full = { ...body.sheet.full, ...(build === pensantus ? pensantusBook : {}), ...prepared, ...options.sheet };
  const character = await createCharacterRPC(playerPage, campaignId, body);
  expect(character.ok(), await character.text()).toBeTruthy();
  const characterId = (await character.json()).character.id as string;

  if (options.session !== false) {
    await startSessionRPC(masterPage, campaignId);
  }
  if (options.milestone !== false && (options.xpMode ?? 'XP_MODE_MILESTONES') === 'XP_MODE_MILESTONES') {
    await markMilestoneRPC(masterPage, campaignId, 'Chegar ao Vale Seco', [characterId]);
  }
  return { campaignId, characterId };
}

/** The master marks a milestone: the characters it names can level up (RN-12). */
export async function markMilestoneRPC(masterPage: Page, campaignId: string, reason: string, characterIds: string[]): Promise<void> {
  const res = await callRPC(masterPage, 'meurpg.progression.v1.ProgressionService/MarkMilestone', {
    campaignId,
    reason,
    characterIds,
    idempotencyKey: crypto.randomUUID(),
  });
  expect(res.ok(), await res.text()).toBeTruthy();
}

/**
 * The first step of the guided level-up is "Subir em qual classe?": the class it already has is marked, and "Próximo"
 * goes on with it. The specs about the steps after it start here.
 */
export async function passClassStep(page: Page): Promise<void> {
  await expect(page.getByText(/Passo 1 de \d+ · Classe/)).toBeVisible();
  await page.getByRole('button', { name: 'Próximo' }).click();
}

/** The card of a class in "Subir em qual classe?": the radio inside is visually hidden, so a tap goes to the card. */
export function classCard(page: Page, name: RegExp): Locator {
  return page.locator('app-class-pick label.card').filter({ hasText: name }).first();
}
