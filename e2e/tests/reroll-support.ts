import { expect, type Browser } from '@playwright/test';

import { rollsTable, vex, type RollsTable } from './combat-rolls-support';
import { getEncounterRPC } from './combat-support';
import { createEntryRPC, rerollFeatBody } from './content-support';
import { callRPC } from './support';

// The table of the reroll question: Vex, whose table feat "Sorte Teimosa" rolls a d20 of 2 or lower again for one use
// (the pool the same feat defines). The feat is made and given after the combat began, as the master may change a sheet.

export interface RerollTable extends RollsTable {
  /** Vex's attack on Goblin 1 with a typed d20 of `face`: a face the rule covers is held for the question. */
  swing: (face: number) => Promise<{ holdId: string; vexId: string }>;
}

export async function rerollTable(browser: Browser, name: string, look: { colorScheme?: 'light' | 'dark'; masterWidth?: number; playerWidth?: number } = {}): Promise<RerollTable> {
  const t = await rollsTable(browser, name, vex, { weaponKeys: ['equipment:longsword'] }, { Vex: 20, 'Goblin 1': 15, 'Capitão Goblin': 5, 'Goblin 2': 4 }, look);
  const feat = await createEntryRPC(t.m, t.campaignId, 'tableFeat', rerollFeatBody());
  const got = await callRPC(t.m, 'meurpg.characters.v1.CharacterService/GetCharacter', { campaignId: t.campaignId, characterId: t.table.characterId });
  expect(got.ok(), await got.text()).toBeTruthy();
  const character = (await got.json()).character as { id: string; name: string; revision: number; sheet: { full: Record<string, unknown> } };
  const featKeys = [...((character.sheet.full.featKeys as string[] | undefined) ?? []), feat];
  const gave = await callRPC(t.m, 'meurpg.characters.v1.CharacterService/UpdateCharacter', {
    campaignId: t.campaignId,
    characterId: character.id,
    revision: character.revision,
    name: character.name,
    sheet: { full: { ...character.sheet.full, featKeys } },
  });
  expect(gave.ok(), await gave.text()).toBeTruthy();
  return {
    ...t,
    swing: async (face) => {
      const enc = await getEncounterRPC(t.m, t.campaignId);
      const id = (label: string) => enc.combatants.find((c) => c.label === label)!.id;
      const res = await callRPC(t.p, 'meurpg.play.v1.CombatService/RollAttack', {
        campaignId: t.campaignId,
        encounterId: enc.id,
        attackerId: id('Vex'),
        attackKey: 'equipment:longsword',
        targetId: id('Goblin 1'),
        idempotencyKey: crypto.randomUUID(),
        d20Face: face,
      });
      expect(res.ok(), await res.text()).toBeTruthy();
      const offer = (await res.json()).heldRollOffer as { holdId: string; reroll?: object };
      expect(offer?.reroll, 'a d20 of 2 with the rule must be held for the reroll question').toBeTruthy();
      return { holdId: offer.holdId, vexId: id('Vex') };
    },
  };
}
