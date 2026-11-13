import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { getEncounterRPC } from './combat-support';
import { openSessionPage } from './live-session-support';
import { waitForEncounterRevision } from './revision-support';
import { FAR, mapCard, nextTurn, paintPad, paintStatus, squareCenter, sunSwitch, sunlightTable } from './sunlight-support';

// The sunlight of a combat (#704, RN-20, RN-29), through the master's screen: the switch on the bar, the scope "Onde eu pintar", paint
// mode by keyboard and by tap, the count, and "Pronto", which gives "Próximo turno" back. The player's page, in the same combat, says
// nothing of the sun (RN-10: the sun is the master's). The table and the combat come through the API.

test(
  'o mestre liga a luz do sol, escolhe "Onde eu pintar" e pinta quadrados pelo teclado e pelo toque; "Pronto" devolve o próximo turno; o jogador não vê nada da luz do sol',
  { tag: ['@RN-20', '@RN-29', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const t = await sunlightTable(browser, 'Combate com sol');
    const { m, p, campaignId } = t;
    try {
      await openSessionPage(m, campaignId);
      await m.getByRole('button', { name: 'Começar o combate' }).click();
      await expect(nextTurn(m)).toBeVisible();
      await expect(nextTurn(m)).not.toHaveAttribute('aria-disabled', 'true');

      // The sun is off: no scope controls. The switch turns it on over the whole map.
      await expect(sunSwitch(m, 'Luz do sol: desligada')).toHaveAttribute('aria-checked', 'false');
      await expect(m.locator('app-sunlight-controls')).toHaveCount(0);
      await sunSwitch(m).click();
      await expect(sunSwitch(m, 'Luz do sol: todo o mapa')).toHaveAttribute('aria-checked', 'true');
      await expect(m.getByRole('radio', { name: 'Todo o mapa' })).toHaveAttribute('aria-checked', 'true');
      await expect(m.getByRole('button', { name: 'Pintar luz do sol no mapa' })).toHaveCount(0);

      // "Onde eu pintar": nothing lit yet, and "Pintar" opens paint mode, where the turn waits.
      await m.getByRole('radio', { name: 'Onde eu pintar' }).click();
      await expect(sunSwitch(m, 'Luz do sol: onde eu pintar')).toHaveAttribute('aria-checked', 'true');
      await m.getByRole('button', { name: 'Pintar luz do sol no mapa' }).click();
      await expect(paintStatus(m)).toHaveText('Pintando luz do sol · 0 quadrados');
      await expect(nextTurn(m)).toHaveAttribute('aria-disabled', 'true');
      await expect(m.getByText('Toque em Pronto, na pintura da luz do sol, para seguir.')).toBeVisible();

      // By keyboard: the pad has the focus; three arrows right and two down reach column 4, row 3 (as the master counts), and Space paints it.
      const pad = paintPad(m);
      await expect(pad).toBeFocused();
      for (const key of ['ArrowRight', 'ArrowRight', 'ArrowRight', 'ArrowDown', 'ArrowDown']) {
        await m.keyboard.press(key);
      }
      await expect(m.getByRole('status').filter({ hasText: 'Coluna 4, linha 3: sem luz do sol' })).toBeAttached();
      await m.keyboard.press('Space');
      await expect(paintStatus(m)).toHaveText('Pintando luz do sol · 1 quadrado');
      await expect(m.getByRole('status').filter({ hasText: 'Coluna 4, linha 3: luz do sol' })).toBeAttached();

      // By tap: a square far from the first.
      const begun = await getEncounterRPC(m, campaignId);
      await pad.click({ position: await squareCenter(pad, FAR, begun.gridColumns, begun.gridRows) });
      await expect(paintStatus(m)).toHaveText('Pintando luz do sol · 2 quadrados');

      // The tool "Apagar" takes the tapped square out again, and "Pintar" puts the pencil back.
      await m.getByRole('button', { name: 'Apagar quadrados de luz do sol' }).click();
      await pad.click({ position: await squareCenter(pad, FAR, begun.gridColumns, begun.gridRows) });
      await expect(paintStatus(m)).toHaveText('Pintando luz do sol · 1 quadrado');
      await m.getByRole('button', { name: 'Pintar quadrados de luz do sol' }).click();
      await pad.click({ position: await squareCenter(pad, FAR, begun.gridColumns, begun.gridRows) });
      await expect(paintStatus(m)).toHaveText('Pintando luz do sol · 2 quadrados');

      // "Pronto" leaves paint mode and "Próximo turno" is whole again.
      await m.getByRole('button', { name: 'Pronto: sair da pintura da luz do sol' }).click();
      await expect(paintStatus(m)).toHaveCount(0);
      await expect(m.getByRole('button', { name: 'Pintar luz do sol no mapa' })).toBeVisible();
      await expect(nextTurn(m)).not.toHaveAttribute('aria-disabled', 'true');
      await expect(mapCard(m).getByRole('radio', { name: 'Onde eu pintar' })).toHaveAttribute('aria-checked', 'true');

      // The server holds the two squares.
      const after = await getEncounterRPC(m, campaignId);
      const squares = ((after as { sunlightSquares?: { col?: number; row?: number }[] }).sunlightSquares ?? []).map((s) => `${s.col ?? 0},${s.row ?? 0}`).sort();
      expect(squares).toEqual([`${FAR.col},${FAR.row}`, '3,2'].sort());

      // The player's page shows the same combat, with no word of the sun: no switch, no legend entry, no notice, no controls.
      await openSessionPage(p, campaignId);
      await waitForEncounterRevision(p, after, 'the combat after the sunlight');
      await expect(p.locator('app-combat-view')).toBeVisible();
      await expect(p.getByText(/luz do sol/i)).toHaveCount(0);
      await expect(p.getByRole('switch')).toHaveCount(0);
      await expect(p.locator('app-sunlight-controls')).toHaveCount(0);
      await expect(p.getByTestId('sunlight-notice')).toHaveCount(0);
    } finally {
      await t.done();
    }
  },
);
