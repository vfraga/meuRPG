import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { castSheet, openCastOf } from './casting-support';
import { endTurnOf, getEncounterRPC, passTurnsToAndWait } from './combat-support';
import { closeSheet, contestRPC, idOf } from './contests-support';
import { listCreaturesRPC } from './creatures-support';
import { openSessionPage } from './live-session-support';
import { beginMountedCombat, mountedTable, STEED } from './mounted-support';
import { tapSquare } from './move-support';

// #588 (mounted combat and Convocar Montaria): the paladin calls her steed, gets on it in the combat, rides, and a shove that
// pushes the horse asks her the save to stay on. The goblin's shove goes through the API (the master's dice are not under test);
// everything the player presses and reads is the screen's.

test(
  'Isolda convoca a montaria, monta, cavalga, um empurrão no cavalo pede o teste e, falhando, Isolda cai da montaria ao lado',
  { tag: ['@588'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const t = await mountedTable(browser, 'Montaria');
    const { m, p, campaignId, table } = t;
    try {
      // Convocar Montaria, out of combat: the form and the type, a 2nd-level slot, and the steed is on the sheet.
      await openSessionPage(p, campaignId);
      const cast = await openCastOf(p, 'Convocar Montaria');
      await cast.getByRole('button', { name: 'Escolher a criatura' }).click();
      const summon = p.locator('mat-dialog-container[aria-labelledby="summon-t"]');
      await summon.locator('label', { hasText: STEED }).click();
      await expect(summon.getByText('Escolha o tipo do corcel.')).toBeVisible();
      await summon.getByRole('radiogroup', { name: 'Tipo do corcel' }).getByText('Celestial').click();
      await summon.getByRole('button', { name: `Convocar ${STEED}` }).click();
      await expect(summon).toBeHidden();
      await expect(castSheet(p).getByText(new RegExp(`${STEED} chegou\\.`))).toBeVisible();
      const creatures = await listCreaturesRPC(p, campaignId, table.characterId);
      expect(creatures.map((c) => c.name)).toEqual([STEED]);

      // The combat: her turn first, the horse beside her. She gets on from the "Mover" page.
      let enc = await beginMountedCombat(t);
      await openSessionPage(p, campaignId);
      await expect(p.getByRole('heading', { name: 'Sua vez, Isolda' })).toBeVisible();
      await p.getByRole('button', { name: 'Mover', exact: true }).click();
      const panel = p.getByRole('region', { name: 'Montaria' });
      await panel.getByRole('button', { name: 'Montar' }).click();
      await expect.poll(async () => (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Isolda')).toMatchObject({ mountedOnId: idOf(enc, STEED) });

      // She rides: the horse's move carries her.
      await tapSquare(p, 9, 7);
      await p.getByRole('button', { name: 'Mover para cá' }).click();
      await expect
        .poll(async () => (await getEncounterRPC(m, campaignId)).combatants.filter((c) => ['Isolda', STEED].includes(c.label)).map((c) => [c.col ?? 0, c.row ?? 0]))
        .toEqual([[9, 7], [9, 7]]);
      await endTurnOf(p, m, campaignId, 'Isolda');

      // Goblin 1 shoves the horse and wins; the push asks Isolda the save, on her screen.
      enc = await passTurnsToAndWait(m, campaignId, 'Goblin 1', p);
      const started = await contestRPC(m, 'StartContest', {
        campaignId,
        encounterId: enc.id,
        initiatorId: idOf(enc, 'Goblin 1'),
        targetId: idOf(enc, STEED),
        purpose: 'CONTEST_PURPOSE_SHOVE',
        roll: { d20Faces: { faces: [20] } },
      });
      const contestId = (started.contest as { id: string }).id;
      await contestRPC(m, 'RespondContest', { campaignId, encounterId: enc.id, contestId, skill: 'CONTEST_SKILL_ATHLETICS', roll: { d20Faces: { faces: [1] } } });
      await contestRPC(m, 'ResolveShove', { campaignId, encounterId: enc.id, contestId, outcome: 'SHOVE_OUTCOME_PUSH' });

      const save = p.getByRole('alertdialog', { name: /Você cai da montaria\?/ });
      await expect(save.getByText(`O ${STEED} foi empurrado.`)).toBeVisible();
      await save.getByRole('button', { name: 'Digitar o resultado' }).click();
      await save.getByLabel('Face do d20 (1 a 20)').fill('4');
      await save.getByRole('button', { name: /^Confirmar/ }).click();
      await expect(save.locator('.res')).toContainText('4: você caiu Derrubado ao lado do Cavalo de guerra.');
      await closeSheet(save);
      // Under it, the shove's own sheet (the horse is hers, so the contest came to her screen).
      await closeSheet(p.getByRole('dialog', { name: /Goblin 1 tenta empurrar/ }));

      // She lies prone on a square beside the horse, and rides nothing.
      const after = await getEncounterRPC(m, campaignId);
      const rider = after.combatants.find((c) => c.label === 'Isolda')!;
      const horse = after.combatants.find((c) => c.label === STEED)!;
      expect(rider.mountedOnId ?? '').toBe('');
      expect(rider.conditions).toContain('condition:prone');
      expect(Math.max(Math.abs((rider.col ?? 0) - (horse.col ?? 0)), Math.abs((rider.row ?? 0) - (horse.row ?? 0)))).toBe(1);
      await expect(p.getByText('Derrubado').first()).toBeVisible();
    } finally {
      await t.done().catch(() => undefined);
    }
  },
);
