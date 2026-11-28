import { expect, type Page } from '@playwright/test';

import { pickRadio } from './move-support';
import { callRPC } from './support';

// Setup for the table-rules specs (Etapa 10, slice 10.13a, MR-025, RN-24, RN-25, RN-09), through the API: these tests
// prove the screens (the "Regras da mesa" page, the "Habilidades" methods, the grid calibration), not the campaign and
// invite flows other specs already cover. Every test makes its own campaign.

/** A campaign of Mestre Teste's, leveled by enemies (the default of the app). */
export async function masterCampaign(master: Page, name: string, xpMode = 'XP_MODE_ENEMIES'): Promise<string> {
  const created = await callRPC(master, 'meurpg.campaigns.v1.CampaignService/CreateCampaign', { name, xpMode });
  expect(created.ok(), await created.text()).toBeTruthy();
  return (await created.json()).campaign.id as string;
}

/** A campaign with Jogador Teste in it and no character yet: the player makes it on screen. */
export async function campaignWithEmptyPlayer(master: Page, player: Page, name: string): Promise<string> {
  const campaignId = await masterCampaign(master, name);
  const invite = await callRPC(master, 'meurpg.campaigns.v1.CampaignService/CreateInvite', { campaignId, maxUses: 1, expiresIn: '3600s' });
  expect(invite.ok(), await invite.text()).toBeTruthy();
  const accepted = await callRPC(player, 'meurpg.campaigns.v1.CampaignService/AcceptInvite', { token: (await invite.json()).token });
  expect(accepted.ok(), await accepted.text()).toBeTruthy();
  return campaignId;
}

/** `GetTableRules` as the app's JSON. */
export async function tableRulesOf(page: Page, campaignId: string): Promise<{ rules: Record<string, unknown>; style: string }> {
  const res = await callRPC(page, 'meurpg.campaigns.v1.CampaignService/GetTableRules', { campaignId });
  expect(res.ok(), await res.text()).toBeTruthy();
  return res.json();
}

/** `SetTableRules` with the defaults of the SRD and the given changes, through the API. */
export async function setTableRulesRPC(master: Page, campaignId: string, rules: Record<string, unknown>): Promise<void> {
  const res = await callRPC(master, 'meurpg.campaigns.v1.CampaignService/SetTableRules', {
    campaignId,
    rules: {
      diceMode: 'DICE_MODE_PLAYERS_CHOOSE',
      combatStartsWithMap: true,
      fogOnNewMaps: false,
      hitPoints: 'HIT_POINTS_RULE_PLAYER_CHOOSES',
      abilityMethods: { standardArray: true, pointBuy: true, rolled4d6: true, typed: true },
      critical: 'CRITICAL_RULE_DOUBLED_DICE',
      deathSaves: 'DEATH_SAVE_VISIBILITY_VISIBLE_TO_ALL',
      hiddenAreaHits: 'HIDDEN_AREA_HIT_RULE_REVEAL',
      houseRules: [],
      enemyReactions: 'ENEMY_REACTIONS_RULE_ONLY_WHEN_POSSIBLE',
      ...rules,
    },
  });
  expect(res.ok(), await res.text()).toBeTruthy();
}

/** The squares of a closed wall block, to paint through the API. */
export function wallSquares(): { col: number; row: number }[] {
  return [
    { col: 2, row: 1 },
    { col: 3, row: 1 },
    { col: 4, row: 1 },
    { col: 2, row: 2 },
    { col: 2, row: 3 },
  ];
}

/**
 * A way of making scores: the segments are labels over hidden radios, so the label takes the click.
 * The browser checks the radio at the click, before the page switches the fields.
 * So it returns only once the page shows the way as the chosen one: until then, the previous way's fields still answer
 * to the ability names, and a fill by label can land on one of them.
 */
export async function method(page: Page, name: string): Promise<void> {
  const scores = page.locator('app-table-ability-scores');
  const label = new RegExp(`^\\s*(check)?\\s*${name}\\s*$`);
  await pickRadio(scores, label);
  await expect(scores.locator('label.seg__item--on', { hasText: label })).toBeVisible();
}

/** A value of "Cada quadrado deste desenho vale": a card over a hidden radio, named by its title ("3 m", with a no-break space). */
export async function factor(page: Page, title: string): Promise<void> {
  const words = title.replace(' ', '\\s');
  await pickRadio(page.locator('app-calibrate-ask'), new RegExp(`^\\s*${words}`));
}
