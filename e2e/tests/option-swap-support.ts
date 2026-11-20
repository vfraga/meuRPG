import { expect, type Page } from '@playwright/test';

import { setTableRulesRPC } from './table-rules-support';
import { callRPC } from './support';
import { toren, type LevelUpTable } from './levelup-support';

// Setup for the swap of the options of a table feature at a level-up (#715): "Mestre das Posturas", a table subclass of the
// fighter whose feature "Posturas" lists five options, takes two, and says at which class levels one pick may be swapped. The
// content comes through the API, so a row proves the level-up screen and not the editor (which has no field for the swap
// levels: a pack carries them). Invented names, as the other content specs have.

export const postureNames = ['Postura da Garça', 'Postura do Touro', 'Postura do Lobo', 'Postura do Corvo', 'Postura do Urso'];

export const defenseStyle = 'feature:fighter-fighting-style-defense';

/** A fighter of the table's subclass: Doran, with Defesa and the first two postures, ready for the next level. */
export const doran = { ...toren, name: 'Doran', subclassKey: '' };

/**
 * Makes the subclass "Mestre das Posturas" (feature at level 3, swap allowed at class levels `swapLevels`) in the campaign and
 * returns the sheet fields of a fighter of it: the subclass and the picks (Defesa, Postura da Garça, Postura do Touro).
 */
export async function postureSheetRPC(master: Page, campaignId: string, level: number, swapLevels: number[]): Promise<Record<string, unknown>> {
  const res = await callRPC(master, 'meurpg.rules.v1.TableContentService/CreateTableEntry', {
    campaignId,
    tableSubclass: {
      namePt: 'Mestre das Posturas',
      classKey: 'class:fighter',
      descPt: ['Quem aprendeu a lutar em posturas.'],
      levels: [
        {
          level: 3,
          features: [
            {
              namePt: 'Posturas',
              descPt: ['Escolha duas posturas de luta.'],
              effects: [{ type: 'choice', choice: 'feature', count: 2 }],
              swapLevels: { levels: swapLevels },
              options: postureNames.map((n) => ({ namePt: n, descPt: [`${n}: um jeito de lutar.`] })),
            },
          ],
        },
      ],
    },
  });
  expect(res.ok(), await res.text()).toBeTruthy();
  const entry = (await res.json()).entry;
  const picks = entry.tableSubclass.levels[0].features[0].options.slice(0, 2).map((o: { key: string }) => o.key) as string[];
  return {
    classes: [{ classKey: 'class:fighter', level, subclassKey: entry.key }],
    featureChoiceKeys: [defenseStyle, ...picks],
  };
}

/** The table plays with the rule "Trocar uma escolha no aumento de atributo". */
export async function allowRulePickSwapRPC(master: Page, campaignId: string): Promise<void> {
  await setTableRulesRPC(master, campaignId, { pickSwapAllowed: true });
}

export type { LevelUpTable };
