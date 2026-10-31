import { expect, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { endOpenSessionRPC, openSessionPage, showMapTool } from './live-session-support';
import { callRPC, newSignedInContext } from './support';
import { dalila, dalilaSheet, toren } from './combat-support';
import { sessionRoute, squareBp, tableForFog } from './fog-support';
import { movePensantus, pensantusFirst, pointNames, sq20, thirdPlayer, trapRPC, trapTable, treasureRPC } from './trap-support';
import { openCombatPanel } from './combat-layout';

// Traps and treasure in the session, on screen (Etapa 9, slice 9.14, MR-035, MR-041, RN-10, RN-02,
// question 71). The table, the traps and the combat come through the API; what is under test is what
// the master and the player read and press, and what the player is never told.

const trapPanel = (m: Page) => m.getByRole('region', { name: 'Armadilhas do mapa' });
const treasurePanel = (m: Page) => m.getByRole('region', { name: 'Tesouros do mapa' });

test(
  'o mestre revela a armadilha para um jogador: o outro jogador não a recebe, nem no mapa nem na tela',
  { tag: ['@MR-035', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const t = await trapTable(browser, 'Revelar');
    const { m, p, table, campaignId } = t;
    const third = await thirdPlayer(browser, t, toren);
    try {
      await trapRPC(m, table, 'Fosso escondido', 12, 7, { noticeDc: 30 });
      await openSessionPage(m, campaignId, 'mapa');
      await showMapTool(m, 'Armadilhas');
      await openSessionPage(third.page, campaignId);
      expect(await pointNames(p, table)).not.toContain('Fosso escondido');
      expect(await pointNames(third.page, table)).not.toContain('Fosso escondido');

      const card = trapPanel(m).getByRole('article', { name: 'Fosso escondido' });
      await expect(card).toContainText('Só você vê');
      await expect(card).toContainText('Quem notaria');
      await card.getByRole('button', { name: 'Revelar para…' }).click();
      const dialog = m.getByRole('dialog', { name: 'Revelar armadilha' });
      await expect(dialog.getByRole('button', { name: 'Revelar a armadilha' })).toHaveAttribute('aria-disabled', 'true');
      await dialog.locator('label', { hasText: 'Pensantus' }).click();
      await dialog.getByRole('button', { name: 'Revelar para Pensantus' }).click();
      await expect(dialog).toBeHidden();
      await expect(card).toContainText('Só Pensantus sabe');

      // Pensantus's player gets it; the other player never does: not in the map's points, not on their screen.
      expect(await pointNames(p, table)).toContain('Fosso escondido');
      expect(await pointNames(third.page, table)).not.toContain('Fosso escondido');
      await third.page.reload();
      await expect(third.page.getByText('Ao vivo', { exact: true }).first()).toBeVisible({ timeout: 30_000 });
      await expect(third.page.getByText('Fosso escondido')).toHaveCount(0);

      // A second reveal now shows who already knows, checked and disabled with the reason.
      await card.getByRole('button', { name: 'Revelar para…' }).click();
      await expect(dialog).toContainText('você já revelou');
      await expect(dialog.getByRole('checkbox', { name: /Pensantus/ })).toBeDisabled();
      await dialog.getByRole('button', { name: 'Cancelar' }).click();
    } finally {
      await third.close();
      await t.done();
    }
  },
);

test(
  'o jogador procura com Investigação e acha; longe, a resposta é a mesma de quando falha',
  { tag: ['@MR-035', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(180_000);
    const { m, p, table, campaignId, done } = await trapTable(browser, 'Procurar');
    try {
      await trapRPC(m, table, 'Fosso escondido', 6, 7);
      await openSessionPage(p, campaignId);
      expect(await pointNames(p, table)).not.toContain('Fosso escondido');

      await p.getByRole('button', { name: 'Procurar armadilhas' }).click();
      const sheet = p.getByRole('dialog', { name: 'Procurar armadilhas' });
      await expect(sheet).toContainText('Algumas armadilhas só se acham com uma perícia específica.');
      await sheet.locator('label', { hasText: 'Investigação' }).click();
      await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
      await sheet.getByLabel(/Role 1d20 para Investigação/).fill('12');
      await sheet.getByRole('button', { name: /Confirmar 12/ }).click();
      await expect(sheet).toContainText('Você achou uma armadilha: Fosso escondido.');
      await sheet.getByRole('button', { name: 'Fechar', exact: true }).last().click();
      expect(await pointNames(p, table)).toContain('Fosso escondido');
      await expect(p.getByRole('region', { name: 'Registro' })).toContainText('procurou armadilhas (Investigação)');

      // A trap far away: the same words as a roll that falls short.
      await trapRPC(m, table, 'Fosso distante', 18, 2);
      await p.getByRole('button', { name: 'Procurar armadilhas' }).click();
      await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
      await sheet.getByLabel(/Role 1d20 para/).fill('20');
      await sheet.getByRole('button', { name: /Confirmar 20/ }).click();
      await expect(sheet).toContainText('Você não encontrou nada.');
      expect(await pointNames(p, table)).not.toContain('Fosso distante');
    } finally {
      await done();
    }
  },
);

test(
  'o personagem anda para dentro da armadilha no combate: o movimento para, o dano espera e o mestre aplica',
  { tag: ['@MR-035', '@RN-02'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const { m, p, table, campaignId, done } = await trapTable(browser, 'Queda');
    try {
      await trapRPC(m, table, 'Fosso escondido', 7, 7, { damage: '3' });
      await pensantusFirst(m, table);
      await openSessionPage(m, campaignId);
      await openSessionPage(p, campaignId);

      const moved = await movePensantus(p, table, 9, 7);
      expect(moved.combatants.find((c) => c.label === 'Pensantus')!.col).toBe(7);

      await expect(p.getByText('Você caiu na armadilha Fosso escondido.')).toBeVisible();
      await expect(p.getByText('Esperando o mestre aplicar o dano')).toBeVisible();
      const card = m.getByRole('region', { name: /Fosso escondido pegou Pensantus/ });
      await expect(card).toContainText('3 de concussão');
      await card.getByRole('button', { name: 'Aplicar 3 de dano' }).click();
      await expect(card).toBeHidden();
      await openCombatPanel(m, 'Registro');
      await expect(m.getByText('A armadilha Fosso escondido foi disparada').first()).toBeVisible();
    } finally {
      await done();
    }
  },
);

test(
  'o mestre dispara na mão fora do combate e decide o dano: aplica um e descarta o outro',
  { tag: ['@MR-035', '@RN-02'] },
  async ({ browser }) => {
    test.setTimeout(180_000);
    const { m, table, campaignId, done } = await trapTable(browser, 'Disparar');
    try {
      await trapRPC(m, table, 'Fosso escondido', 12, 7, { manual: true, damage: '4' });
      await trapRPC(m, table, 'Fosso raso', 14, 7, { manual: true, damage: '5' });
      await openSessionPage(m, campaignId, 'mapa');
      await showMapTool(m, 'Armadilhas');
      const fire = async (name: string) => {
        const card = trapPanel(m).getByRole('article', { name });
        if (await card.getByRole('button', { name: 'Ver detalhes' }).count()) {
          await card.getByRole('button', { name: 'Ver detalhes' }).click();
        }
        await card.getByRole('button', { name: 'Disparar…' }).click();
        const dialog = m.getByRole('dialog', { name: /Disparar/ });
        await expect(dialog).toContainText('Ninguém marcado. Dispara para quem estiver na área');
        await dialog.locator('label', { hasText: 'Pensantus' }).click();
        await dialog.getByRole('button', { name: 'Disparar para Pensantus' }).click();
        await expect(card).toContainText('Disparada');
        return card;
      };
      const first = await fire('Fosso escondido');
      await fire('Fosso raso');
      const damage = trapPanel(m).getByRole('region', { name: /Fosso escondido pegou Pensantus/ });
      const other = trapPanel(m).getByRole('region', { name: /Fosso raso pegou Pensantus/ });
      await expect(damage).toContainText('Mude o número se houver resistência: o app não calcula.');
      await damage.getByRole('textbox').fill('2');
      await damage.getByRole('button', { name: 'Aplicar 2 de dano' }).click();
      await expect(damage).toBeHidden();

      // The other one is discarded: asked in place, then gone, and nothing waits for the master any more.
      await other.getByRole('button', { name: 'Não aplicar' }).click();
      await expect(other.getByRole('alertdialog')).toContainText('Descartar o dano de 5?');
      await other.getByRole('alertdialog').getByRole('button', { name: 'Descartar' }).click();
      await expect(other).toBeHidden();
      const left = await callRPC(m, 'meurpg.play.v1.PlayService/ListTrapDamages', { campaignId });
      expect(((await left.json()).damages ?? []) as unknown[]).toHaveLength(0);
      await expect(m.getByRole('region', { name: 'Registro' })).toBeVisible();

      // The first trap is disarmed after the table resolves the thieves' tools check.
      const more = first.getByRole('button', { name: 'Ver detalhes' });
      if (await more.count()) await more.click();
      await first.getByRole('button', { name: 'Marcar como desarmada' }).click();
      await expect(first).toContainText('Desarmada');
    } finally {
      await done();
    }
  },
);

test(
  'o "Disparar…" abre antes de o "Quem notaria" chegar: diz que está lendo e se completa sozinho',
  { tag: ['@MR-035'] },
  async ({ browser }) => {
    test.setTimeout(120_000);
    const { m, table, campaignId, done } = await trapTable(browser, 'Leitura lenta');
    try {
      await trapRPC(m, table, 'Fosso escondido', 12, 7, { manual: true, damage: '4' });
      // The read is held back, so the dialog always opens first: the race the master can win at a real table.
      await m.route('**/meurpg.maps.v1.MapService/GetTrapNoticers', async (route) => {
        await new Promise((r) => setTimeout(r, 3_000));
        await route.continue();
      });
      await openSessionPage(m, campaignId, 'mapa');
      await showMapTool(m, 'Armadilhas');
      const card = trapPanel(m).getByRole('article', { name: 'Fosso escondido' });
      if (await card.getByRole('button', { name: 'Ver detalhes' }).count()) {
        await card.getByRole('button', { name: 'Ver detalhes' }).click();
      }
      await card.getByRole('button', { name: 'Disparar…' }).click();
      const dialog = m.getByRole('dialog', { name: /Disparar/ });
      await expect(dialog).toContainText('Lendo quem está no mapa');
      await expect(dialog).not.toContainText('Ninguém com token no mapa');
      await dialog.locator('label', { hasText: 'Pensantus' }).click();
      await dialog.getByRole('button', { name: 'Disparar para Pensantus' }).click();
      await expect(card).toContainText('Disparada');
    } finally {
      await done();
    }
  },
);

test(
  'o aviso passivo chega só ao jogador que notou: a mensagem, a armadilha no mapa dele e o fechar',
  { tag: ['@MR-035', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(180_000);
    const { m, p, table, campaignId, done } = await trapTable(browser, 'Aviso');
    try {
      await trapRPC(m, table, 'Fosso escondido', 9, 7, { noticeDc: 5 });
      await openSessionPage(p, campaignId);
      expect(await pointNames(p, table)).not.toContain('Fosso escondido');
      // The master walks Pensantus up to within 3 m of the pit.
      const res = await callRPC(m, 'meurpg.maps.v1.MapService/PlaceMapToken', {
        campaignId,
        mapId: table.mapId,
        characterId: table.characterId,
        ...sq20(8, 7),
      });
      expect(res.ok(), await res.text()).toBeTruthy();
      const toast = p.getByRole('status').filter({ hasText: 'Você notou uma armadilha.' });
      await expect(toast).toBeVisible();
      await expect(toast).toContainText('Fosso escondido');
      expect(await pointNames(p, table)).toContain('Fosso escondido');
      // It goes away by itself after 8 seconds.
      await expect(p.getByText('Você notou uma armadilha.')).toBeHidden({ timeout: 20_000 });
    } finally {
      await done();
    }
  },
);

test(
  'durante o combate o aviso passivo também aparece, só para quem notou',
  { tag: ['@MR-035', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const { m, p, table, campaignId, done } = await trapTable(browser, 'Aviso no combate');
    try {
      await trapRPC(m, table, 'Fosso escondido', 9, 7, { noticeDc: 5 });
      await pensantusFirst(m, table);
      await openSessionPage(p, campaignId);
      expect(await pointNames(p, table)).not.toContain('Fosso escondido');
      await movePensantus(p, table, 8, 7);
      const toast = p.getByRole('status').filter({ hasText: 'Você notou uma armadilha.' });
      await expect(toast).toBeVisible();
      await expect(toast).toContainText('Fosso escondido');
      expect(await pointNames(p, table)).toContain('Fosso escondido');
      await p.getByRole('button', { name: 'Dispensar o aviso' }).click();
      await expect(toast).toBeHidden();
    } finally {
      await done();
    }
  },
);

test(
  'o mestre marca o tesouro como encontrado por duas pessoas, o jogador o vê e o mestre o desmarca',
  { tag: ['@MR-041', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const t = await trapTable(browser, 'Tesouro');
    const { m, p, table, campaignId } = t;
    const third = await thirdPlayer(browser, t, toren);
    try {
      await treasureRPC(m, table, 'Baú de moedas', 12, 7);
      await openSessionPage(m, campaignId, 'mapa');
      await showMapTool(m, 'Tesouro');
      await openSessionPage(p, campaignId);
      expect(await pointNames(p, table)).not.toContain('Baú de moedas');

      const card = treasurePanel(m).getByRole('article', { name: 'Baú de moedas' });
      await expect(card).toContainText('O que tem dentro · só você vê até achar');
      await card.getByRole('button', { name: 'Marcar como encontrado: Baú de moedas' }).click();
      await expect(card.getByRole('button', { name: 'Marcar como encontrado' })).toHaveAttribute('aria-disabled', 'true');
      await card.locator('label', { hasText: 'Pensantus' }).click();
      await card.locator('label', { hasText: 'Toren' }).click();
      await expect(card).toContainText('No resumo da sessão: Pensantus e Toren');
      await card.getByRole('button', { name: 'Marcar como encontrado' }).click();
      await expect(card).toContainText('Encontrado por Pensantus e Toren');

      // The player: the toast, the row and the sheet with the value and what is inside.
      await expect(p.getByRole('status').filter({ hasText: 'Pensantus e Toren encontraram: Baú de moedas' })).toBeVisible();
      await p.getByRole('button', { name: /^Baú de moedas\s+Tesouro/ }).click();
      const sheet = p.getByRole('dialog', { name: 'Baú de moedas' });
      await expect(sheet).toContainText('250');
      await expect(sheet).toContainText('250 PO e uma adaga de prata.');
      await sheet.getByRole('button', { name: 'Fechar', exact: true }).last().click();

      await card.getByRole('button', { name: 'Desmarcar' }).click();
      await expect(card.getByRole('alertdialog')).toContainText('Desmarcar o Baú de moedas?');
      await card.getByRole('alertdialog').getByRole('button', { name: 'Desmarcar' }).click();
      await expect(card).toContainText('Escondido');
      await expect.poll(() => pointNames(p, table)).not.toContain('Baú de moedas');
    } finally {
      await third.close();
      await t.done();
    }
  },
);

test(
  'num mapa com névoa, a armadilha achada aparece no mapa de quem a achou e não no do outro jogador, e a busca funciona',
  { tag: ['@MR-035', '@MR-036', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const pensantus = await newSignedInContext(browser, 'Jogador Teste');
    const torenCtx = await newSignedInContext(browser, 'E-mail Não Verificado');
    const [mp, ap, bp] = await Promise.all([master.newPage(), pensantus.newPage(), torenCtx.newPage()]);
    let campaignId = '';
    try {
      await Promise.all([mp.goto('/'), ap.goto('/'), bp.goto('/')]);
      const table = await tableForFog(mp, ap, bp, `Névoa e armadilha ${Date.now()}`);
      campaignId = table.campaignId;
      const made = await callRPC(mp, 'meurpg.maps.v1.MapService/CreateMapPoint', {
        campaignId,
        mapId: table.mapId,
        kind: 'MAP_POINT_KIND_TRAP',
        name: 'Fosso na caverna',
        description: 'No corredor',
        ...squareBp(6, 8),
        trap: {
          noticeDc: 30,
          findDc: 5,
          areaSize: 1,
          trigger: 'TRAP_TRIGGER_ENTER',
          effect: { damage: [{ dice: '3', damageTypeKey: 'damage-type:bludgeoning' }] },
        },
      });
      expect(made.ok(), await made.text()).toBeTruthy();
      await Promise.all([mp.goto(sessionRoute(campaignId)), ap.goto(sessionRoute(campaignId)), bp.goto(sessionRoute(campaignId))]);
      for (const page of [mp, ap, bp]) {
        await expect(page.locator('app-fog-base').first()).toBeVisible({ timeout: 30_000 });
      }
      // The master's fog map draws it with the crossed eye ("Só você vê"); the players' maps hold nothing yet.
      await expect(mp.locator('app-fog-map app-map-pins .area')).toHaveCount(1);
      await expect(mp.locator('app-fog-map app-map-pins .area__eye')).toHaveCount(1);
      await expect(mp.getByRole('list', { name: 'Marcas do mapa' })).toContainText('Armadilha');
      await expect(ap.locator('app-fog-map app-map-pins .area')).toHaveCount(0);
      await expect(bp.locator('app-fog-map app-map-pins .area')).toHaveCount(0);

      // Pensantus searches, on the fog map, with a typed roll: the mark appears on his map and on no one else's.
      await ap.getByRole('button', { name: 'Procurar armadilhas' }).click();
      const sheet = ap.getByRole('dialog', { name: 'Procurar armadilhas' });
      await sheet.locator('label', { hasText: 'Investigação' }).click();
      await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
      await sheet.getByLabel(/Role 1d20 para Investigação/).fill('12');
      await sheet.getByRole('button', { name: /Confirmar 12/ }).click();
      await expect(sheet).toContainText('Você achou uma armadilha: Fosso na caverna.');
      await sheet.getByRole('button', { name: 'Fechar', exact: true }).last().click();
      await expect(ap.locator('app-fog-map app-map-pins .area')).toHaveCount(1);
      await expect(ap.locator('app-fog-map app-map-pins .area__eye')).toHaveCount(0);
      await expect(ap.getByRole('list', { name: 'Marcas do mapa' })).toContainText('Armadilha');
      await bp.reload();
      await expect(bp.locator('app-fog-base').first()).toBeVisible({ timeout: 30_000 });
      await expect(bp.locator('app-fog-map app-map-pins .area')).toHaveCount(0);
      await expect(bp.getByText('Fosso na caverna')).toHaveCount(0);
    } finally {
      if (campaignId) {
        await endOpenSessionRPC(mp, campaignId);
      }
      await Promise.all([master.close(), pensantus.close(), torenCtx.close()]);
    }
  },
);

test(
  'Talento Confiável: o ladino 11 digita um d20 baixo na busca, a prévia e o resultado dizem "4 → 10" e ele acha',
  { tag: ['@MR-035', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(180_000);
    const { m, p, table, campaignId, done } = await trapTable(browser, 'Talento', { build: dalila, sheet: dalilaSheet });
    try {
      // The DC is 18: a plain 4 + 10 would fall short, only the 10 that Talento Confiável counts, plus the 10, clears it.
      await trapRPC(m, table, 'Fosso escondido', 6, 7, { findDc: 18 });
      await openSessionPage(p, campaignId);

      await p.getByRole('button', { name: 'Procurar armadilhas' }).click();
      const sheet = p.getByRole('dialog', { name: 'Procurar armadilhas' });
      await sheet.locator('label', { hasText: 'Investigação' }).click();
      await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
      await sheet.getByLabel(/Role 1d20 para Investigação/).fill('4');
      await expect(sheet.getByText('4 → 10 (Talento Confiável)')).toBeVisible();
      await sheet.getByRole('button', { name: /Confirmar 4/ }).click();
      await expect(sheet.getByText(/d20: 4 → 10 \(Talento Confiável\)/)).toBeVisible();
      await expect(sheet).toContainText('Você achou uma armadilha: Fosso escondido.');
      await sheet.getByRole('button', { name: 'Fechar', exact: true }).last().click();
      expect(await pointNames(p, table)).toContain('Fosso escondido');

      // A d20 of 14 is not raised: no line about the feature.
      await trapRPC(m, table, 'Fosso raso', 4, 7, { findDc: 30 });
      await p.getByRole('button', { name: 'Procurar armadilhas' }).click();
      await sheet.locator('label', { hasText: 'Investigação' }).click();
      await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
      await sheet.getByLabel(/Role 1d20 para Investigação/).fill('14');
      await expect(sheet.getByText('Talento Confiável')).toHaveCount(0);
      await sheet.getByRole('button', { name: /Confirmar 14/ }).click();
      await expect(sheet).toContainText('Você não encontrou nada.');
      await expect(sheet.getByText('Talento Confiável')).toHaveCount(0);
    } finally {
      await done();
    }
  },
);

test(
  '"Outra perícia…" é o mesmo menu com ou sem armadilha por perto, e só acha a que lista essa perícia em "Também acham com"',
  { tag: ['@MR-035', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const { m, p, table, campaignId, done } = await trapTable(browser, 'Outra perícia');
    try {
      await openSessionPage(p, campaignId);
      const search = p.getByRole('button', { name: 'Procurar armadilhas' });
      const sheet = p.getByRole('dialog', { name: 'Procurar armadilhas' });
      const openOther = async () => {
        await search.click();
        await sheet.locator('label', { hasText: 'Outra perícia…' }).click();
        return sheet.getByRole('listbox', { name: 'Perícia' });
      };

      // No trap on the map: the list of the other skills with the sheet's bonus.
      const empty = await openOther();
      await expect(empty.getByRole('option')).toHaveCount(16);
      const without = await empty.innerText();
      await p.keyboard.press('Escape');
      await expect(sheet).toBeHidden();

      // Two traps close by: one found with Arcanismo too, the other with Religião. The menu is the same, word for word.
      await trapRPC(m, table, 'Fosso escondido', 6, 7, { alsoFind: ['skill:arcana'] });
      await trapRPC(m, table, 'Fosso raso', 4, 7, { alsoFind: ['skill:religion'] });
      const list = await openOther();
      await expect(list).toHaveText(without, { useInnerText: true });
      await list.getByRole('option', { name: /^Arcanismo/ }).click();
      await expect(sheet.getByText('Escolha uma perícia')).toHaveCount(0);
      await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
      await sheet.getByLabel(/Role 1d20 para Arcanismo/).fill('20');
      await sheet.getByRole('button', { name: /Confirmar 20/ }).click();
      await expect(sheet).toContainText('Você achou uma armadilha: Fosso escondido.');
      await sheet.getByRole('button', { name: 'Fechar', exact: true }).last().click();
      expect(await pointNames(p, table)).toContain('Fosso escondido');
      expect(await pointNames(p, table)).not.toContain('Fosso raso');
      await expect(p.getByRole('region', { name: 'Registro' })).toContainText('procurou armadilhas (Arcanismo)');
    } finally {
      await done();
    }
  },
);
