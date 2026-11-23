import { expect, test } from '@playwright/test';
import { fireReadied, masterQueue, readyMapTable, spellWindow } from './ready-map-support';

test('zz passou sozinho com a ficha aberta tarde', async ({ browser }) => {
  test.setTimeout(240_000);
  const t = await readyMapTable(browser, 'zz passou sozinho', 'fireball', { masterClock: true });
  try {
    const { m, p } = t;
    let fired = 0;
    let delayed = 0;
    await p.route('**/meurpg.play.v1.CombatService/GetEncounter', async (route) => {
      if (fired > 0) {
        delayed++;
        await new Promise((r) => setTimeout(r, 400));
      }
      await route.continue();
    });
    fired = Date.now();
    await fireReadied(m);
    await expect(spellWindow(p)).toBeVisible();
    console.log(`ZZ window visible after ${Date.now() - fired} ms, delayed reads=${delayed}`);
    await expect(masterQueue(m).getByTestId('readied-wait')).toContainText(/passa sozinho em/);
    await m.clock.fastForward(31_000);
    await expect(spellWindow(p)).toContainText(/Passou sozinho: você não respondeu em 30 s\./);
  } finally {
    await t.done();
  }
});
