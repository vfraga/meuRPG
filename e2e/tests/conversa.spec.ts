import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { TOREN_ACCOUNT, chooseReaders, feedOf, lineOf, messageBoxOf, onlyMaster, openConversaTable, say } from './conversa-support';
import { pickRadio } from './move-support';

// The scene's conversation (MR-051, RN-36): who reads a line. Each test makes its own campaign with
// the master, Pensantus and Toren on the session page at a desktop width, where the "Conversa" panel
// sits beside the stage. Absence is asserted only after a later line for everyone arrived on the same
// screen: the stream delivers in order, so by then a line meant for that screen would be there. A
// reader who was not chosen gets no text, no extra row, no notice.

// Toren speaks as his character in character, and as his account's display name out of character.
const toren = 'Toren';

test(
  'o mestre narra para todos e fala como um NPC, e cada jogador vê as duas linhas',
  { tag: ['@MR-051', '@RN-36'] },
  async ({ browser }) => {
    test.setTimeout(120_000);
    const table = await openConversaTable(browser, `Conversa narração ${Date.now()}`);
    const { master, pensantus, toren: torenPage } = table;
    try {
      // While the master types, the others are told (a hint with no text of the message).
      const box = master.getByRole('textbox', { name: 'Mensagem' });
      const hint = (page: typeof master) => page.getByRole('status').filter({ hasText: 'O mestre está escrevendo…' });
      await expect(async () => {
        await box.pressSequentially('.');
        await expect(hint(pensantus)).toBeVisible({ timeout: 2_500 });
        await expect(hint(torenPage)).toBeVisible({ timeout: 2_500 });
      }).toPass({ timeout: 30_000 });
      await box.fill('');

      const narration = `A porta range e o corredor cheira a mofo ${Date.now()}`;
      await say(master, narration);
      for (const page of [master, pensantus, torenPage]) {
        await expect(lineOf(page, narration)).toContainText('Narração');
      }

      // As an NPC: the name in the label and the portrait's initial.
      await master.getByRole('combobox', { name: 'Falar como' }).click();
      await master.getByRole('option', { name: /^Capitão Goblin/ }).click();
      const npcLine = `Quem ousa entrar no meu covil? ${Date.now()}`;
      await say(master, npcLine);
      for (const page of [master, pensantus, torenPage]) {
        const line = lineOf(page, npcLine);
        await expect(line).toContainText('Capitão Goblin');
        await expect(line.locator('[aria-hidden="true"]').filter({ hasText: /^C$/ })).toBeVisible();
      }
    } finally {
      await table.close();
    }
  },
);

test(
  'uma linha para um jogador só chega a ele, e a de um jogador só para o mestre só chega ao mestre',
  { tag: ['@MR-051', '@RN-36'] },
  async ({ browser }) => {
    test.setTimeout(150_000);
    const table = await openConversaTable(browser, `Conversa leitores ${Date.now()}`);
    const { master, pensantus, toren: torenPage } = table;
    try {
      // The master whispers to Pensantus. Pensantus is told it is a whisper, not who else reads it.
      const whisper = `O baú atrás de você está trancado ${Date.now()}`;
      await chooseReaders(master, ['Pensantus']);
      await expect(master.getByLabel('Para', { exact: true })).toContainText('Pensantus');
      await say(master, whisper);
      await expect(lineOf(master, whisper)).toContainText('Narração · só para Pensantus');
      await expect(lineOf(pensantus, whisper)).toContainText('Sussurro do mestre · só você lê');

      // Then a line for everyone: once Toren has it, the whisper would be there too if it were his.
      const marker = `Marcador para a mesa toda ${Date.now()}`;
      await chooseReaders(master, []);
      await say(master, marker);
      await expect(lineOf(torenPage, marker)).toBeVisible();
      await expect(lineOf(pensantus, marker)).toBeVisible();
      await expect(feedOf(torenPage).getByRole('listitem')).toHaveCount(1);
      await expect(torenPage.getByText(whisper)).toHaveCount(0);
      await expect(torenPage.getByText('Sussurro')).toHaveCount(0);
      await expect(torenPage.getByRole('alert')).toHaveCount(0);
      await expect(feedOf(pensantus).getByRole('listitem')).toHaveCount(2);

      // Toren writes to the master only.
      const secret = `Mestre, posso falar a sós? ${Date.now()}`;
      await onlyMaster(torenPage);
      await say(torenPage, secret);
      await expect(lineOf(master, secret)).toContainText(`${toren} · só o mestre lê`);
      await expect(lineOf(torenPage, secret)).toBeVisible();

      const second = `Segundo marcador para a mesa toda ${Date.now()}`;
      await say(master, second);
      await expect(lineOf(pensantus, second)).toBeVisible();
      await expect(pensantus.getByText(secret)).toHaveCount(0);
      // In the feed only: Pensantus's own composer has the "Só o mestre lê" toggle.
      await expect(feedOf(pensantus).getByText('só o mestre lê')).toHaveCount(0);
      await expect(pensantus.getByRole('alert')).toHaveCount(0);
      // Whisper, marker and second marker: nothing more and nothing less.
      await expect(feedOf(pensantus).getByRole('listitem')).toHaveCount(3);
      // Toren: the first marker, his own line to the master, the second marker.
      await expect(feedOf(torenPage).getByRole('listitem')).toHaveCount(3);
    } finally {
      await table.close();
    }
  },
);

test(
  'a fala de um jogador fora do personagem chega à mesa com o nome da pessoa',
  { tag: ['@MR-051', '@RN-36'] },
  async ({ browser }) => {
    test.setTimeout(120_000);
    const table = await openConversaTable(browser, `Conversa fora do personagem ${Date.now()}`);
    const { master, pensantus, toren: torenPage } = table;
    try {
      const aside = `Preciso de um minuto, já volto ${Date.now()}`;
      await pickRadio(torenPage.getByRole('radiogroup', { name: 'Como você fala' }), 'Fora do personagem');
      await say(torenPage, aside);
      for (const page of [master, pensantus, torenPage]) {
        await expect(lineOf(page, aside)).toContainText(`${TOREN_ACCOUNT} · fora do personagem`);
      }

      // In character, the line carries the character's name; to everyone is the default.
      const inCharacter = `Toren ergue o escudo ${Date.now()}`;
      await pickRadio(torenPage.getByRole('radiogroup', { name: 'Como você fala' }), 'Como Toren');
      await say(torenPage, inCharacter);
      await expect(lineOf(pensantus, inCharacter)).toContainText('Toren');
      await expect(lineOf(torenPage, inCharacter)).toContainText('Toren · você');
    } finally {
      await table.close();
    }
  },
);

test(
  'no celular, uma linha do mestre mostra o aviso de mensagem nova no topo, à vista, para quem está na cena',
  { tag: ['@MR-051', '@RN-36'] },
  async ({ browser }) => {
    test.setTimeout(120_000);
    const table = await openConversaTable(browser, `Conversa aviso ${Date.now()}`, { width: 390 });
    const { master, pensantus } = table;
    try {
      const notice = pensantus.getByRole('button', { name: /^Conversa: \d+ mensage(?:m|ns) novas? Abrir$/ });
      // Without news there is no notice, and the master's own line is not news for the master.
      await expect(notice).toHaveCount(0);
      await master.getByRole('button', { name: /^Conversa$/ }).click();
      const narration = `A tocha se apaga sozinha ${Date.now()}`;
      await expect(messageBoxOf(master)).toBeVisible();
      await say(master, narration);
      await expect(lineOf(master, narration)).toBeVisible();

      // Pensantus stays on the scene, at the top of the page: the notice is in the first screen, no scroll.
      await expect(notice).toBeVisible();
      await expect(notice).toBeInViewport();
      await expect(notice).toContainText('Conversa: 1 mensagem nova');

      await notice.click();
      await expect(lineOf(pensantus, narration)).toBeVisible();
      await expect(notice).toHaveCount(0);
    } finally {
      await table.close();
    }
  },
);
