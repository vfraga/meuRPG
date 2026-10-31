import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { grog, rollsTable } from './combat-rolls-support';
import { openSessionPage } from './live-session-support';

// "Ver mapa" (the master's header of the map card; docs/design/README.md, the map controls): the browser's full screen for the map and its controls, next to
// "Ajustar à tela", which only resets the zoom. The state follows the browser, so the button says how to leave.
test('o mestre põe o mapa do combate em tela cheia e sai pelo mesmo botão', async ({ browser }) => {
  test.setTimeout(180_000);
  const { m, campaignId, done } = await rollsTable(browser, 'Tela cheia', grog, {}, { Grog: 20, 'Goblin 1': 5, 'Capitão Goblin': 4, 'Goblin 2': 3 });
  try {
    await openSessionPage(m, campaignId);
    await m.getByRole('button', { name: 'Ver mapa' }).first().click();
    await expect.poll(() => m.evaluate(() => document.fullscreenElement?.tagName.toLowerCase() ?? '')).toBe('app-combat-map-card');
    const exit = m.getByRole('button', { name: 'Sair da tela cheia' });
    await expect(exit).toHaveAttribute('aria-pressed', 'true');
    await exit.click();
    await expect.poll(() => m.evaluate(() => document.fullscreenElement === null)).toBe(true);
    await expect(m.getByRole('button', { name: 'Ver mapa' }).first()).toHaveAttribute('aria-pressed', 'false');
  } finally {
    await done();
  }
});
