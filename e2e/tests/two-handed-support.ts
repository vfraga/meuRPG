import { expect, type Page } from '@playwright/test';

import { createEntryRPC, listEntriesJSON } from './content-support';
import { toren } from './levelup-support';
import { emptyTable } from './table-sheet-support';
import { callRPC, characterRpcBody, createCharacterRPC } from './support';

// Setup for the sheet whose table weapon turned two-handed (#580, board 580-16, state 8, "A arma mudou."): Toren, a Guerreiro 4 who
// is proficient with simple weapons and shields, carries the master's "Machado de Lenhador" (one-handed, simple, 1d6) and a shield.
// The master edits the weapon after the sheet's last save, so the sheet reads it as content that changed under it. The server says
// the shield now stands idle in the sheet's changed-content notice, never as a line of "Pendências de regra".

export const AXE_NAME = 'Machado de Lenhador';

export interface TwoHandedTable {
  campaignId: string;
  characterId: string;
  weaponKey: string;
}

const AXE = { namePt: AXE_NAME, category: 'WEAPON_CATEGORY_SIMPLE', damage: '1d6', damageType: 'damage-type:slashing', properties: [] as string[], weightOz: 80, priceCp: 700 };

/** The campaign with the master's axe, and Toren (made after the axe) carrying it with a shield, saved by the master. */
export async function axeAndShieldTable(master: Page, player: Page, name: string): Promise<TwoHandedTable> {
  const { campaignId } = await emptyTable(master, player, name);
  const weaponKey = await createEntryRPC(master, campaignId, 'tableWeapon', AXE);
  const created = await createCharacterRPC(player, campaignId, characterRpcBody('PLAYER', toren));
  expect(created.ok(), await created.text()).toBeTruthy();
  const characterId = (await created.json()).character.id as string;
  const got = await callRPC(master, 'meurpg.characters.v1.CharacterService/GetCharacter', { campaignId, characterId });
  expect(got.ok(), await got.text()).toBeTruthy();
  const character = (await got.json()).character as { name: string; revision: number; sheet: { full: Record<string, unknown> } };
  const saved = await callRPC(master, 'meurpg.characters.v1.CharacterService/UpdateCharacter', {
    campaignId,
    characterId,
    revision: character.revision,
    name: character.name,
    sheet: { full: { ...character.sheet.full, weaponKeys: [weaponKey], shield: true } },
  });
  expect(saved.ok(), await saved.text()).toBeTruthy();
  return { campaignId, characterId, weaponKey };
}

/** The master edits the axe: it asks both hands, or only its damage changes. */
export async function editAxe(master: Page, table: TwoHandedTable, edit: 'two-handed' | 'damage'): Promise<void> {
  const entry = (await listEntriesJSON(master, table.campaignId)).entries!.find((e) => e.key === table.weaponKey)!;
  const body = edit === 'two-handed' ? { ...AXE, properties: ['weapon-property:two-handed'] } : { ...AXE, damage: '1d8' };
  const res = await callRPC(master, 'meurpg.rules.v1.TableContentService/UpdateTableEntry', {
    campaignId: table.campaignId,
    key: table.weaponKey,
    expectedRevision: entry.revision,
    tableWeapon: body,
  });
  expect(res.ok(), await res.text()).toBeTruthy();
}
