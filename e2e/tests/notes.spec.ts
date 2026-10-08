import { expect, test, type Page } from '@playwright/test';

import { endOpenSessionRPC, openSessionPage } from './live-session-support';
import { addClueRPC, cartClues, cartHooks, createNoteRPC } from './notes-support';
import { tableForScenes } from './scene-support';
import { afterRender, newSignedInContext } from './support';

// MR-029 (the master's hooks and clues, and revealing a clue to the players he
// picks) and MR-030 (the players' private notes), through the screens, with
// RN-20 proved by reading the player's page: a player never gets the hooks, a
// clue that was not revealed to him, nor the name of a scene the group has not
// discovered. Setup (campaigns, maps, points, the session) goes through the API;
// every test makes its own campaign.

/** Adds a clue in the point panel's form and waits for it to be saved. */
async function addClue(master: Page, text: string, expectedCount: number): Promise<void> {
  await master.getByRole('button', { name: 'Adicionar pista' }).click();
  const form = master.getByRole('form', { name: 'Nova pista' });
  await expect(form.getByLabel('Texto da pista')).toBeFocused();
  await form.getByLabel('Texto da pista').fill(text);
  await form.getByRole('button', { name: 'Adicionar pista' }).click();
  await expect(form).toBeHidden();
  await expect(master.getByText(`${expectedCount} de 30`, { exact: true })).toBeVisible();
  await expect(master.getByRole('button', { name: 'Adicionar pista' })).toBeFocused();
}

/** The master's "Abrir cena" picker: picks the point and confirms. */
async function openScene(master: Page, point: string): Promise<void> {
  await master.getByRole('button', { name: 'Abrir cena', exact: true }).click();
  const picker = master.getByRole('dialog', { name: 'Abrir uma cena' });
  await picker.getByText(point, { exact: true }).click();
  await picker.getByRole('button', { name: 'Abrir cena', exact: true }).click();
  await expect(picker).toBeHidden();
}

test(
  'o mestre escreve três pistas e os ganchos no ponto, abre a cena e revela uma pista; o jogador a recebe nas anotações e nunca vê os ganchos nem a pista que não foi revelada',
  { tag: ['@MR-029', '@MR-030', '@RN-20'] },
  async ({ browser }) => {
    test.setTimeout(180_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    let campaignId = '';
    try {
      await master.goto('/');
      const table = await tableForScenes(master, player, `Pistas ${Date.now()}`, false);
      campaignId = table.campaignId;

      // The editor: three clues, each saved at once; the hooks wait for "Salvar ponto".
      await master.goto(`/campaigns/${table.campaignId}/maps/${table.mapId}`);
      await master.getByRole('button', { name: /^A carroça tombada, Cena de RP/ }).click();
      await expect(master.getByRole('heading', { name: 'Pistas' })).toBeVisible();
      await expect(master.getByText('Nenhuma pista ainda')).toBeVisible();
      await expect(master.getByText('0 de 30', { exact: true })).toBeVisible();
      await addClue(master, cartClues[0], 1);
      await addClue(master, cartClues[1], 2);
      await addClue(master, cartClues[2], 3);

      const list = master.locator('.cl__list');
      await expect(list.getByRole('listitem')).toHaveCount(3);
      await expect(list.getByRole('listitem').nth(0)).toContainText('Ninguém ainda');
      // The first ↑ and the last ↓ are quiet; a move saves at once and keeps focus on the same button.
      await expect(master.getByRole('button', { name: 'Subir a pista 1' })).toHaveAttribute('aria-disabled', 'true');
      await master.getByRole('button', { name: 'Subir a pista 3' }).click();
      await expect(list.getByRole('listitem').nth(1)).toContainText(cartClues[2]);
      await expect(master.getByRole('button', { name: 'Subir a pista 2' })).toBeFocused();
      await master.getByRole('button', { name: 'Descer a pista 2' }).click();
      await expect(list.getByRole('listitem').nth(2)).toContainText(cartClues[2]);
      await expect(master.getByRole('button', { name: 'Descer a pista 3' })).toBeFocused();

      // Removing asks in place, with "Voltar" focused first; "Voltar" gives focus back.
      await master.getByRole('button', { name: 'Remover a pista 2' }).click();
      const ask = master.getByRole('alertdialog', { name: 'Remover a pista 2?' });
      await expect(ask).toContainText('Quem já recebeu continua com ela.');
      await expect(ask.getByRole('button', { name: 'Voltar' })).toBeFocused();
      await ask.getByRole('button', { name: 'Voltar' }).click();
      await expect(ask).toBeHidden();
      await expect(master.getByRole('button', { name: 'Remover a pista 2' })).toBeFocused();
      await expect(list.getByRole('listitem')).toHaveCount(3);

      // A clue is edited in place by tapping its words.
      await master.getByRole('button', { name: /^Editar a pista 1/ }).click();
      const edit = master.getByRole('form', { name: 'Editar a pista 1' });
      await expect(edit.getByLabel('Texto da pista')).toHaveValue(cartClues[0]);
      await edit.getByRole('button', { name: 'Cancelar' }).click();
      await expect(edit).toBeHidden();
      await expect(master.getByRole('button', { name: /^Editar a pista 1/ })).toBeFocused();

      // The hooks: a lock and "Só você vê" first; saved with "Salvar ponto".
      await expect(master.getByText('Só você vê. Nunca aparece para os jogadores.')).toBeVisible();
      await master.getByRole('textbox', { name: 'Ganchos e anotações' }).fill(cartHooks);
      await expect(master.getByText(`${[...cartHooks].length} de 4.000`)).toBeVisible();
      await master.getByRole('button', { name: 'Salvar ponto' }).click();
      await expect(master.getByText('A carroça tombada salvo.')).toBeAttached();
      await master.reload();
      await master.getByRole('button', { name: /^A carroça tombada, Cena de RP/ }).click();
      await expect(master.getByRole('textbox', { name: 'Ganchos e anotações' })).toHaveValue(cartHooks);
      await expect(master.locator('.cl__list').getByRole('listitem')).toHaveCount(3);

      // The session: the master opens the scene; the player is already there.
      await openSessionPage(master, table.campaignId);
      await openSessionPage(player, table.campaignId);
      await openScene(master, 'A carroça tombada');
      const clues = master.getByRole('heading', { name: 'Pistas' }).locator('xpath=ancestor::section[1]');
      await expect(clues.getByRole('listitem')).toHaveCount(3);
      await expect(clues.getByText('Ninguém ainda')).toHaveCount(3);
      await expect(master.getByRole('region', { name: 'Ganchos e anotações' }).getByText('Só você vê')).toBeVisible();
      await expect(master.getByText(cartHooks)).toBeVisible();
      await expect(player.getByRole('button', { name: 'Anotações', exact: true })).toBeVisible();

      // Revealing: nobody is checked, the button is the dashed one and says why; "Marcar todos" turns it into the filled one.
      await master.getByRole('button', { name: 'Revelar a pista 2' }).click();
      const dialog = master.getByRole('dialog', { name: 'Revelar pista' });
      await expect(dialog.getByRole('heading', { name: 'Revelar pista' })).toBeFocused();
      await expect(dialog.getByText('Pista 2 de 3 · A carroça tombada')).toBeVisible();
      await expect(dialog.getByRole('checkbox', { name: /Pensantus/ })).not.toBeChecked();
      await expect(dialog.getByText('Ninguém marcado. Escolha quem recebe a pista.')).toBeVisible();
      const dashed = dialog.getByRole('button', { name: 'Revelar a pista' });
      await expect(dashed).toHaveAttribute('aria-disabled', 'true');
      await dashed.click({ force: true });
      await afterRender(master);
      await expect(dialog).toBeVisible();
      await dialog.getByRole('button', { name: 'Marcar todos' }).click();
      await expect(dialog.getByRole('button', { name: 'Desmarcar todos' })).toBeVisible();
      await dialog.getByRole('button', { name: 'Revelar para Pensantus' }).click();
      await expect(dialog).toBeHidden();
      await expect(master.getByText(/Pista revelada para todos às\s\d\d:\d\d\./)).toBeVisible();
      await expect(clues.getByRole('listitem').nth(1)).toContainText(/Revelada para todos às\s\d\d:\d\d/);
      await expect(clues.getByRole('button', { name: 'Revelar a pista 2' })).toHaveCount(0);
      await expect(clues.getByRole('button', { name: /^Revelar a pista [13]$/ })).toHaveCount(2);

      // The player: the notice, then the sheet.
      await expect(player.getByText('O mestre revelou uma pista para você.')).toBeVisible();
      await expect(player.getByRole('button', { name: 'Anotações, 1 nova' })).toBeVisible();
      await player.getByRole('button', { name: 'Abrir anotações' }).click();
      // The dialog is named by its title, which changes with the stage: take it by role.
      const sheet = player.getByRole('dialog');
      await expect(sheet.getByRole('heading', { name: 'Anotações' })).toBeFocused();
      await expect(sheet.getByText('Só você lê as suas anotações. O mestre não vê.')).toBeVisible();
      await expect(sheet.getByText(cartClues[1])).toBeVisible();
      await expect(sheet.getByText('Pista do mestre')).toBeVisible();
      await expect(sheet.getByText('Só leitura')).toBeVisible();
      // Opening the notes saw the news: the notice is gone.
      await expect(player.getByText('O mestre revelou uma pista para você.')).toHaveCount(0);

      // RN-20: nothing the master did not reveal is on the player's page.
      for (const secret of [cartClues[0], cartClues[2], cartHooks, 'Mira, a filha do Aldo', 'Posto da guarda']) {
        expect(await player.content()).not.toContain(secret);
      }

      // A new note, tagged with the scene that is open; the picker lists only the discovered scenes (the revealed ones and the opened one).
      await sheet.getByRole('button', { name: 'Nova anotação' }).click();
      await expect(sheet.getByRole('heading', { name: 'Nova anotação' })).toBeVisible();
      await expect(sheet.getByLabel('Anotação', { exact: true })).toBeFocused();
      await sheet.getByLabel('Anotação', { exact: true }).fill('Perguntar ao ferreiro sobre o brasão de lobo');
      await expect(sheet.getByText('44 de 2.000')).toBeVisible();
      const picker = sheet.getByRole('combobox', { name: /Cena \(opcional\)/ });
      await expect(picker).toContainText('A carroça tombada');
      await picker.click();
      const options = player.getByRole('option');
      await expect(options).toHaveText(['Sem cena', 'A carroça tombada', 'Vau do riacho']);
      expect(await player.content()).not.toContain('Posto da guarda');
      await options.getByText('A carroça tombada').click();
      await sheet.getByRole('button', { name: 'Salvar anotação' }).click();
      await expect(sheet.getByRole('heading', { name: 'Anotações' })).toBeVisible();
      const note = sheet.getByRole('listitem').filter({ hasText: 'Perguntar ao ferreiro' });
      await expect(note).toContainText('A carroça tombada');
      // Newest first: the note is above the clue; the filter by scene counts both.
      await expect(sheet.getByRole('listitem').first()).toContainText('Perguntar ao ferreiro');
      await sheet.getByRole('combobox', { name: /^Cena/ }).click();
      await expect(player.getByRole('option')).toHaveText(['Todas as anotações2', 'A carroça tombada2', 'Vau do riacho0', 'Sem cena0']);
      await player.getByRole('option', { name: /Sem cena/ }).click();
      await expect(sheet.getByText('Nada sem cena')).toBeVisible();
      await expect(player.getByRole('status').filter({ hasText: '0 anotações' })).toBeAttached();
      // The list fades out after a choice; a click before it is gone lands on
      // its first option, "Todas as anotações", instead of opening it again.
      await expect(player.getByRole('listbox')).toHaveCount(0);
      await sheet.getByRole('combobox', { name: /^Cena/ }).click();
      await player.getByRole('option', { name: /Todas as anotações/ }).click();
      await expect(player.getByRole('listbox')).toHaveCount(0);

      // Editing and deleting one's own note; the clue has a lock, not a pencil.
      await expect(sheet.getByRole('button', { name: /^Editar a anotação/ })).toHaveCount(1);
      await note.getByRole('button', { name: /^Editar a anotação/ }).click();
      await expect(sheet.getByRole('heading', { name: 'Editar anotação' })).toBeVisible();
      await sheet.getByLabel('Anotação', { exact: true }).fill('Perguntar ao ferreiro sobre o brasão');
      await sheet.getByRole('button', { name: 'Salvar anotação' }).click();
      await expect(sheet.getByRole('listitem').first()).toContainText('Perguntar ao ferreiro sobre o brasão');
      await sheet.getByRole('button', { name: /^Editar a anotação/ }).click();
      await sheet.getByRole('button', { name: 'Apagar anotação' }).click();
      await expect(sheet.getByRole('button', { name: 'Voltar' })).toBeFocused();
      await sheet.getByRole('alertdialog', { name: 'Apagar esta anotação?' }).getByRole('button', { name: 'Apagar anotação' }).click();
      await expect(sheet.getByRole('heading', { name: 'Anotações' })).toBeVisible();
      await expect(sheet.getByText('Perguntar ao ferreiro')).toHaveCount(0);
      await sheet.getByRole('button', { name: 'Fechar' }).click();
      // The bar's button has no news now, and focus is back on it.
      await expect(player.getByRole('button', { name: 'Anotações', exact: true })).toBeFocused();

      // The same notes are a panel on the player's sheet; the master never gets it, even on that sheet.
      await createNoteRPC(player, table.campaignId, 'Brisa me deve 5 PO');
      await player.goto(`/campaigns/${table.campaignId}/characters/${table.characterId}`);
      const panel = player.getByRole('region', { name: 'Anotações' });
      await expect(panel.getByText('Brisa me deve 5 PO')).toBeVisible();
      await expect(panel.getByText('Pista do mestre')).toBeVisible();
      await master.goto(`/campaigns/${table.campaignId}/characters/${table.characterId}`);
      await expect(master.getByRole('heading', { name: 'Pensantus' }).first()).toBeVisible();
      await expect(master.getByRole('heading', { name: 'Anotações' })).toHaveCount(0);
      await expect(master.getByText('Brisa me deve 5 PO')).toHaveCount(0);
    } finally {
      if (campaignId) {
        await endOpenSessionRPC(master, campaignId);
      }
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'uma cena sem ações abre, e o jogador a vê; a lista das pistas para em 30 e o jogador escreve até 300 anotações',
  { tag: ['@MR-029', '@MR-030'] },
  async ({ browser }) => {
    test.setTimeout(180_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    let campaignId = '';
    try {
      await master.goto('/');
      const table = await tableForScenes(master, player, `Sem ações ${Date.now()}`, false);
      campaignId = table.campaignId;
      await openSessionPage(master, table.campaignId);
      await openSessionPage(player, table.campaignId);

      // "Vau do riacho" has no action and opens all the same (question 63).
      await master.getByRole('button', { name: 'Abrir cena', exact: true }).click();
      const picker = master.getByRole('dialog', { name: 'Abrir uma cena' });
      await expect(picker.getByRole('radio', { name: /Vau do riacho/ })).toBeEnabled();
      await picker.getByText('Vau do riacho', { exact: true }).click();
      await picker.getByRole('button', { name: 'Abrir cena', exact: true }).click();
      await expect(master.getByRole('heading', { name: 'Cena: Vau do riacho' })).toBeFocused();
      await expect(master.getByText(/sem ações/)).toBeVisible();
      await expect(player.getByRole('region', { name: 'Cena: Vau do riacho' })).toBeVisible();

      // 30 clues: the list says it is full, in words, and the add button is the dashed one.
      for (let i = 1; i <= 30; i++) {
        await addClueRPC(master, table, table.fordId, `Pista número ${i}`);
      }
      await master.goto(`/campaigns/${table.campaignId}/maps/${table.mapId}`);
      await master.getByRole('button', { name: /^Vau do riacho, Cena de RP/ }).click();
      await expect(master.getByText('30 de 30', { exact: true })).toBeVisible();
      await expect(master.getByText('Limite de 30 pistas. Remova uma para adicionar outra.')).toBeVisible();
      await expect(master.getByText('Mais 22 pistas na lista.')).toBeVisible();
      await expect(master.getByRole('button', { name: 'Adicionar pista' })).toBeDisabled();
      // Removing one gives the button back.
      await master.getByRole('button', { name: 'Remover a pista 1' }).click();
      await master.getByRole('alertdialog').getByRole('button', { name: 'Remover pista' }).click();
      await expect(master.getByText('29 de 30', { exact: true })).toBeVisible();
      await expect(master.getByRole('button', { name: 'Adicionar pista' })).toBeEnabled();

      // 300 notes: "Nova anotação" is the dashed button with the limit said in words.
      for (let i = 1; i <= 300; i++) {
        await createNoteRPC(player, table.campaignId, `Anotação ${i}`);
      }
      await player.goto(`/campaigns/${table.campaignId}/characters/${table.characterId}`);
      const panel = player.getByRole('region', { name: 'Anotações' });
      await expect(panel.getByText('300 anotações', { exact: true })).toBeVisible();
      await expect(panel.getByText('Limite de 300 anotações. Apague uma para escrever outra.')).toBeVisible();
      await expect(panel.getByRole('button', { name: 'Nova anotação' })).toBeDisabled();
    } finally {
      if (campaignId) {
        await endOpenSessionRPC(master, campaignId);
      }
      await masterContext.close();
      await playerContext.close();
    }
  },
);
