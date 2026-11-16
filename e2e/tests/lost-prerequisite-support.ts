import { expect, type Page } from '@playwright/test';

import { classBody, createClassRPC } from './classes-support';
import { createEntryRPC, listEntriesJSON } from './content-support';
import { setSwitchesRPC } from './content-options-support';
import { toren } from './levelup-support';
import { emptyTable } from './table-sheet-support';
import { setTableRulesRPC } from './table-rules-support';
import { callRPC, characterRpcBody, createCharacterRPC } from './support';

// Setup for the sheet that keeps a feat or an option whose prerequisite it no longer meets (#580, board 580-05): Toren is a
// Guerreiro 4, and the table's feat (or the feat's inline option) asks for a class level he has not reached. The condition is
// a normal SRD class (Guerreiro 7) or a class the master switched off, which a player reads as "uma condição que a mesa não
// mostra" (RN-23) and the master reads named. The master saves the sheet with the feat, as the guided pick would refuse it.

export type LostKind = 'feat' | 'option' | 'both';

export interface LostTable {
  campaignId: string;
  characterId: string;
  /** The feat's name (the feat itself, or the feat that holds the option). */
  featName: string;
  /** The name of the feat or option the sentence is about. */
  entryName: string;
  /** With 'both', the lost feat that stands beside the lost option (`entryName` is then the option). */
  alsoName?: string;
}

const HIDDEN_CLASS = 'Ordem Esquecida';

/**
 * Toren, made after the table's entries: the sheet is known to fit the table as it stands, so a feat the master adds on it
 * is no "changed content" and only the edit of the edited-feat table is (RN-23, carryFlags).
 */
async function torenAfterTheTable(player: Page, campaignId: string): Promise<string> {
  const body = characterRpcBody('PLAYER', toren);
  const created = await createCharacterRPC(player, campaignId, body);
  expect(created.ok(), await created.text()).toBeTruthy();
  return (await created.json()).character.id as string;
}

/** The class the prerequisite asks: Guerreiro 7, or the master's own class (level 3) once the table switches it off. */
async function askedClass(master: Page, campaignId: string, hidden: boolean): Promise<{ classKey: string; level: number }> {
  if (!hidden) {
    return { classKey: 'class:fighter', level: 7 };
  }
  const classKey = await createClassRPC(master, campaignId, classBody(HIDDEN_CLASS));
  return { classKey, level: 3 };
}

async function saveSheetWith(master: Page, campaignId: string, characterId: string, extra: Record<string, string[]>): Promise<void> {
  const got = await callRPC(master, 'meurpg.characters.v1.CharacterService/GetCharacter', { campaignId, characterId });
  expect(got.ok(), await got.text()).toBeTruthy();
  const character = (await got.json()).character as { id: string; name: string; revision: number; sheet: { full: Record<string, unknown> } };
  const full = { ...character.sheet.full };
  for (const [key, values] of Object.entries(extra)) {
    full[key] = [...((full[key] as string[] | undefined) ?? []), ...values];
  }
  const saved = await callRPC(master, 'meurpg.characters.v1.CharacterService/UpdateCharacter', {
    campaignId,
    characterId,
    revision: character.revision,
    name: character.name,
    sheet: { full },
  });
  expect(saved.ok(), await saved.text()).toBeTruthy();
}

/**
 * Toren's table with the master's feat on his sheet: `kind` 'feat' is a feat that asks the class level; 'option' is a feat
 * with a choice of options (the feat has none to ask) and the option Toren kept asks it. `hidden` switches the asked class off.
 */
export async function lostPrerequisiteTable(master: Page, player: Page, name: string, kind: LostKind, hidden: boolean): Promise<LostTable> {
  const { campaignId } = await emptyTable(master, player, name);
  await setTableRulesRPC(master, campaignId, { featsAllowed: true });
  const asked = await askedClass(master, campaignId, hidden);
  const given: Record<string, string[]> = { featKeys: [] };
  let featName = 'Postura Firme';
  let entryName = featName;
  let alsoName: string | undefined;
  if (kind === 'feat' || kind === 'both') {
    const feat = await createEntryRPC(master, campaignId, 'tableFeat', {
      namePt: 'Postura Firme',
      descPt: ['Você firma os pés e não cede um passo.'],
      prerequisite: { classLevel: asked },
    });
    given.featKeys.push(feat);
  }
  if (kind === 'option' || kind === 'both') {
    alsoName = kind === 'both' ? featName : undefined;
    featName = 'Truques de lâmina';
    entryName = 'Asa Partida';
    const feat = await createEntryRPC(master, campaignId, 'tableFeat', {
      namePt: featName,
      descPt: ['Escolha um truque de lâmina ao pegar o talento.'],
      effects: [{ type: 'choice', choice: 'feature', count: 1 }],
      options: [{ namePt: entryName, descPt: ['A lâmina abre a guarda do outro.'], prerequisite: { classLevel: asked } }],
    });
    const entry = (await listEntriesJSON(master, campaignId)).entries!.find((e) => e.key === feat)!;
    const optionKey = entry.tableFeat.options[0].key as string;
    given.featKeys.push(feat);
    given.featureChoiceKeys = [optionKey];
  }
  const characterId = await torenAfterTheTable(player, campaignId);
  await saveSheetWith(master, campaignId, characterId, given);
  if (hidden) {
    await setSwitchesRPC(master, campaignId, [{ key: asked.classKey, off: true }]);
  }
  return { campaignId, characterId, featName, entryName, alsoName };
}

/**
 * The master edits a feat the sheet already carries so it asks Guerreiro 7: the owner reads the line in the changed-content
 * notice, not in "Pendências de regra".
 */
export async function editedFeatTable(master: Page, player: Page, name: string): Promise<LostTable> {
  const { campaignId } = await emptyTable(master, player, name);
  await setTableRulesRPC(master, campaignId, { featsAllowed: true });
  const body = { namePt: 'Postura Firme', descPt: ['Você firma os pés e não cede um passo.'] };
  const feat = await createEntryRPC(master, campaignId, 'tableFeat', body);
  const characterId = await torenAfterTheTable(player, campaignId);
  await saveSheetWith(master, campaignId, characterId, { featKeys: [feat] });
  const entry = (await listEntriesJSON(master, campaignId)).entries!.find((e) => e.key === feat)!;
  const res = await callRPC(master, 'meurpg.rules.v1.TableContentService/UpdateTableEntry', {
    campaignId,
    key: feat,
    expectedRevision: entry.revision,
    tableFeat: { ...body, prerequisite: { classLevel: { classKey: 'class:fighter', level: 7 } } },
  });
  expect(res.ok(), await res.text()).toBeTruthy();
  return { campaignId, characterId, featName: body.namePt, entryName: body.namePt };
}
