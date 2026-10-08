import { expect, test, type Browser, type Page } from '@playwright/test';

import { openSessionPage } from './live-session-support';
import {
  BELL_NAMES,
  CIPHER,
  RIDDLE,
  SEQUENCE,
  createCipherRPC,
  createRiddleRPC,
  createSequenceRPC,
  endTable,
  masterRunJson,
  playSequenceRPC,
  playerRunText,
  puzzleRoute,
  revealClueRPC,
  sceneClueRPC,
  secondPlayer,
  setDiceModeRPC,
  showPuzzleRPC,
  tableForPuzzles,
  trapPointRPC,
} from './puzzles-support';
import { setCurrentMapRPC } from './maps-support';
import { boxOf, newSignedInContext } from './support';

// More puzzles on screen (Etapa 10, slice 10.15b: MR-038, RN-27, RN-10, RN-18; E10-12): the riddle, the sequence and the cipher, the
// skill check that wins a hint, the split information and "Ao errar". The puzzles come through the API where the screen is not what is
// under test; the tests read what a player's response carries, as the app reads it, to prove no answer, sequence, key or other player's
// part ever leaves the server.

test.describe.configure({ timeout: 300_000 });

/** What a player's response must never carry (RN-10, RN-27), for every kind. */
const NEVER = ['"solution"', '"minimum"', '"path"', '"onSolve"', '"on_solve"', '"hintCheck"', '"dc"', '"onWrong"', '"attempts"'];

async function twoPeople(browser: Browser, viewport = { width: 1280, height: 1000 }, colorScheme: 'light' | 'dark' = 'light') {
  const masterContext = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 1000 }, colorScheme });
  const playerContext = await newSignedInContext(browser, 'Jogador Teste', { viewport, colorScheme });
  const master = await masterContext.newPage();
  const player = await playerContext.newPage();
  await Promise.all([master.goto('/'), player.goto('/')]);
  return { master, player, close: () => Promise.all([masterContext.close(), playerContext.close()]) };
}

/**
 * Everything the player's page is sent in answer to a move or a try for a hint: the same JSON the app reads, kept to look for what must never
 * be in it (RN-10, RN-27), as `GetPuzzleRun` is.
 */
function watchAnswers(page: Page): string[] {
  const bodies: string[] = [];
  page.on('response', (res) => {
    if (/PuzzleService\/(MakePuzzleMove|TryPuzzleHint)$/.test(res.url())) {
      void res.text().then((t) => bodies.push(t), () => undefined);
    }
  });
  return bodies;
}

/** The player's page on a puzzle, once its title is there. */
async function playerOpens(player: Page, campaignId: string, puzzleId: string, name: string): Promise<void> {
  await player.goto(`/campaigns/${campaignId}/session?puzzle=${puzzleId}`);
  await expect(player.getByRole('heading', { level: 1, name })).toBeVisible({ timeout: 30_000 });
}

test('o enigma: o mestre o faz no formulário, o jogador erra e depois acerta, e nenhuma resposta chega a ele @MR-038 @RN-27 @RN-10', async ({ browser }) => {
  const { master, player, close } = await twoPeople(browser);
  const answers = watchAnswers(player);
  const table = await tableForPuzzles(master, player, `Enigma ${Date.now()}`);
  try {
    // The master's form: the riddle, the accepted answers one by one, and "Ao errar": two attempts for each player.
    await master.goto(puzzleRoute(table.campaignId, 'puzzles', 'new'));
    await master.getByRole('radio', { name: /^Enigma/ }).check();
    await master.getByLabel('Nome').fill('A porta da Cripta pergunta');
    await master.getByLabel('O enigma').fill(RIDDLE.text);
    await master.getByLabel('Pista para os jogadores').fill('Procure no chão da Cripta.');
    for (const answer of RIDDLE.answers) {
      await master.getByLabel(/^(Uma|Outra) resposta$/).fill(answer);
      await master.getByRole('button', { name: 'Adicionar', exact: true }).click();
    }
    await expect(master.getByRole('button', { name: 'Tirar a resposta a sombra' })).toBeVisible();
    await master.getByRole('radio', { name: 'Gastar uma tentativa do jogador' }).check();
    await master.getByRole('button', { name: 'Menos tentativa' }).click();
    await master.getByRole('button', { name: 'Criar quebra-cabeça' }).click();
    await expect(master).toHaveURL(puzzleRoute(table.campaignId));
    await expect(master.getByRole('region', { name: 'Quebra-cabeças' }).getByText('Enigma · 2 respostas aceitas')).toBeVisible();

    await openSessionPage(master, table.campaignId);
    await master.getByRole('button', { name: 'Mostrar aos jogadores A porta da Cripta pergunta' }).click();
    const card = master.getByRole('article', { name: 'A porta da Cripta pergunta' });
    await expect(card.getByText('Os jogadores veem')).toBeVisible();
    // Only the master reads the answers, each time with "Só você vê".
    await expect(card.getByText('Respostas aceitas')).toBeVisible();
    await expect(card.getByRole('list', { name: 'Respostas aceitas' }).getByRole('listitem')).toHaveText(['sombra', 'a sombra']);

    await openSessionPage(player, table.campaignId);
    await expect(player.getByText('O mestre mostrou um quebra-cabeça')).toBeVisible({ timeout: 30_000 });
    await player.getByRole('link', { name: 'Abrir o quebra-cabeça' }).click();
    await expect(player.getByRole('heading', { level: 1, name: 'A porta da Cripta pergunta' })).toBeVisible();
    await expect(player.getByText(RIDDLE.text)).toBeVisible();
    await expect(player.getByText('Suas tentativas')).toBeVisible();
    const puzzleId = new URL(player.url()).searchParams.get('puzzle')!;
    const before = await playerRunText(player, table.campaignId, puzzleId);
    for (const answer of RIDDLE.answers) {
      expect(before).not.toContain(answer);
    }
    for (const secret of NEVER) {
      expect(before, secret).not.toContain(secret);
    }

    // A wrong answer: the words and the icon, how many attempts are left, and never how close it was.
    await player.getByLabel('Sua resposta').fill('escuridão');
    await player.getByRole('button', { name: 'Responder' }).click();
    const wrong = player.getByRole('alert').filter({ hasText: 'Não é isso.' });
    await expect(wrong).toContainText('Tente outra resposta.');
    await expect(player.locator('app-limit-counters')).toContainText('Suas tentativas 1 de 2');
    // What the server keeps of it is the master's: the player's own response has neither the typed answer nor a list of answers.
    const after = await playerRunText(player, table.campaignId, puzzleId);
    expect(after).not.toContain('escuridão');
    for (const answer of RIDDLE.answers) {
      expect(after).not.toContain(answer);
    }
    expect(after).toContain('"wrong":true');
    // The master reads the answer and the attempts left, by player.
    await expect(card.getByText(/Última jogada: Pensantus tentou “escuridão”: errou/)).toBeVisible();
    await expect(card.getByText('Tentativas de Pensantus: 1 de 2.')).toBeVisible();
    await expect(card.getByText('Pensantus 1 de 2')).toBeVisible();

    // The right answer, with other capitals and punctuation: solved, for the table.
    await player.getByLabel('Sua resposta').fill('  A Sombra! ');
    await player.getByRole('button', { name: 'Responder' }).click();
    await expect(player.getByText('Resolvido.', { exact: false }).first()).toBeVisible();
    await expect(player.getByText('O enigma foi respondido.')).toBeVisible();
    await expect(player.getByLabel('Sua resposta')).toHaveCount(0);
    await expect(card.getByText('Pensantus resolveu “A porta da Cripta pergunta”', { exact: false })).toBeVisible();
    const solved = await playerRunText(player, table.campaignId, puzzleId);
    for (const answer of RIDDLE.answers) {
      expect(solved).not.toContain(answer);
    }
    // The answers to the moves themselves (the wrong one and the right one) carry no answer either.
    await expect.poll(() => answers.length).toBeGreaterThanOrEqual(2);
    for (const body of answers) {
      for (const answer of RIDDLE.answers) {
        expect(body).not.toContain(answer);
      }
      expect(body).not.toContain('escuridão');
      for (const secret of NEVER) {
        expect(body, secret).not.toContain(secret);
      }
    }
  } finally {
    await endTable(master, table.campaignId);
    await close();
  }
});

test('a sequência: o jogador vê tocar passo a passo e repete; um passo errado dispara a armadilha @MR-038 @RN-27 @RN-10 @MR-035', async ({ browser }) => {
  const { master, player, close } = await twoPeople(browser);
  const table = await tableForPuzzles(master, player, `Sequência ${Date.now()}`);
  try {
    await setCurrentMapRPC(master, table.campaignId, table.map.mapId);
    const trap = await trapPointRPC(master, table.campaignId, table.map.mapId, 'Dardos envenenados');
    const puzzleId = await createSequenceRPC(master, table.campaignId, 'Os sinos do Salão do trono', {
      clue: 'Quem toca os sinos escuta o trono.',
      onWrong: { trap: { mapId: table.map.mapId, pointId: trap } },
    });
    await showPuzzleRPC(master, table.campaignId, puzzleId);
    await openSessionPage(master, table.campaignId);
    const card = master.getByRole('article', { name: 'Os sinos do Salão do trono' });
    // The master reads the whole sequence ("Só você vê"); the players have not seen it play.
    await expect(card.getByRole('list', { name: 'A sequência, passo a passo' }).getByRole('listitem')).toHaveCount(6);
    await expect(card.getByText('Os jogadores ainda não viram a sequência tocar')).toBeVisible();

    await playerOpens(player, table.campaignId, puzzleId, 'Os sinos do Salão do trono');
    await expect(player.getByText('O mestre ainda não tocou os sinos.')).toBeVisible();
    // Before the first play the bells do nothing (the server refuses): the app shows them still.
    await expect(player.getByRole('button', { name: 'Sino alto' })).toHaveAttribute('aria-disabled', 'true');

    // "Tocar a sequência": each phone shows it step by step, and the server sends only the steps already played.
    const started = Date.now();
    await card.getByRole('button', { name: 'Tocar a sequência' }).click();
    await expect(player.getByText('O mestre está tocando os sinos.')).toBeVisible({ timeout: 15_000 });
    // The server sends a step only when its time has come: the first at once, then one every 1,2 s. Read it three times while it plays.
    const readSequence = async () => JSON.parse(await playerRunText(player, table.campaignId, puzzleId)).run.sequence as { shown?: number[]; totalSteps: number };
    for (let i = 0; i < 3; i++) {
      const read = await readSequence();
      const elapsed = Date.now() - started;
      expect(read.totalSteps).toBe(6);
      expect((read.shown ?? []).length).toBeLessThanOrEqual(Math.floor(elapsed / 1200) + 1);
      // Each bell shown so far is the sequence's own, in order; none of the ones to come.
      expect(read.shown ?? []).toEqual(SEQUENCE.slice(0, (read.shown ?? []).length));
      // Wait for the next step to be sent, not for a fixed time: the server's own clock decides when.
      const seen = (read.shown ?? []).length;
      if (seen < read.totalSteps) {
        await expect.poll(async () => ((await readSequence()).shown ?? []).length, { timeout: 10_000 }).toBeGreaterThan(seen);
      }
    }
    await expect(player.getByText(/passo [2-6] de 6/)).toBeVisible({ timeout: 15_000 });
    await expect(player.locator('.big__name')).toBeVisible();
    // No bell to tap while it plays; when it ends it is "com vocês".
    await expect(player.getByRole('button', { name: 'Sino alto' })).toHaveCount(0);
    await expect(player.getByText('Agora é com vocês.')).toBeVisible({ timeout: 30_000 });
    await expect(card.getByText('Os jogadores já viram a sequência tocar 1 vez.')).toBeVisible();
    const after = await playerRunText(player, table.campaignId, puzzleId);
    // The config says how many steps there are ("steps":6); never which bell each one is.
    expect(after).not.toMatch(/"steps":\[/);
    for (const secret of NEVER) {
      expect(after, secret).not.toContain(secret);
    }
    // The players never receive the sequence again once the play ends: nothing of it is in what they read now.
    expect(JSON.parse(after).run.sequence.shown ?? []).toEqual([]);

    // A wrong first bell: "Errou o passo 1", the attempt starts over, the trap fires, and the master says so too.
    await player.getByRole('button', { name: 'Sino largo' }).click();
    await expect(player.locator('.board-card').getByRole('alert').filter({ hasText: 'Errou o passo 1.' })).toContainText('A tentativa recomeçou; você errou.');
    await expect(card.getByText(/A armadilha disparou: Dardos envenenados\. Foi o erro de Pensantus\./)).toBeVisible();
    await expect(card.getByText(/Última jogada: Pensantus errou no passo 1\. A tentativa recomeçou/)).toBeVisible();
    // The trap that fired is public to who sees its map (the master put it on the player's screen), with the name the master gave it.
    await expect(player.getByText('A armadilha disparou:')).toBeVisible();
    await expect(player.getByText('Dardos envenenados.', { exact: false })).toBeVisible();

    // Repeat it right, bell by bell: the count of right steps is the server's, and the last one solves it.
    // Tapped one after the other without waiting for the screen to say "N de 6": the taps go to the server one at a time, in order, so a fast
    // right repeat is never judged wrong (and never fires the trap again).
    for (const bell of SEQUENCE) {
      await player.getByRole('button', { name: BELL_NAMES[bell] }).click({ noWaitAfter: true });
    }
    await expect(player.getByText('Os sinos tocaram na ordem certa.')).toBeVisible();
    await expect(card.getByText('Pensantus resolveu “Os sinos do Salão do trono”', { exact: false })).toBeVisible();
    expect((await masterRunJson(master, table.campaignId, puzzleId)).run).toBeTruthy();
  } finally {
    await endTable(master, table.campaignId);
    await close();
  }
});

test('a cifra: a chave é uma pista da cena que o grupo acha, e o jogador decifra à mão @MR-038 @RN-27 @RN-10 @MR-029', async ({ browser }) => {
  const { master, player, close } = await twoPeople(browser);
  const answers = watchAnswers(player);
  const table = await tableForPuzzles(master, player, `Cifra ${Date.now()}`);
  try {
    const clueText = 'Cada letra anda três para trás.';
    const clueId = await sceneClueRPC(master, table.campaignId, table.map.mapId, 'A biblioteca', clueText);

    // The master's form: the server ciphers the message for him ("Como os jogadores a veem"), and the key is a scene clue.
    await master.goto(puzzleRoute(table.campaignId, 'puzzles', 'new'));
    await master.getByRole('radio', { name: /^Cifra/ }).check();
    await master.getByLabel('Nome').fill('A carta do Capitão');
    await master.getByLabel('Mensagem', { exact: true }).fill(CIPHER.message);
    await expect(master.locator('app-cipher-form .cipher')).toHaveText(CIPHER.ciphertext, { timeout: 30_000 });
    await master.getByLabel('Qual pista guarda a chave').selectOption(clueId);
    await master.getByRole('button', { name: 'Criar quebra-cabeça' }).click();
    await expect(master).toHaveURL(puzzleRoute(table.campaignId));

    await openSessionPage(master, table.campaignId);
    await master.getByRole('button', { name: 'Mostrar aos jogadores A carta do Capitão' }).click();
    const card = master.getByRole('article', { name: 'A carta do Capitão' });
    await expect(card.locator('.cipher')).toHaveText(CIPHER.ciphertext);
    await expect(card.getByText(`A mensagem: ${CIPHER.message}`)).toBeVisible();

    await openSessionPage(player, table.campaignId);
    await player.getByRole('link', { name: 'Abrir o quebra-cabeça' }).click();
    await expect(player.getByRole('heading', { level: 1, name: 'A carta do Capitão' })).toBeVisible();
    const puzzleId = new URL(player.url()).searchParams.get('puzzle')!;
    await expect(player.locator('.cipher')).toHaveText(CIPHER.ciphertext);
    // The letter, a column for each of its letters, and the field: the table is the player's own helper.
    await expect(player.locator('.col__bet')).toHaveCount(9);
    await expect(player.getByText('Ainda não acharam a chave.')).toBeVisible();
    const lost = await playerRunText(player, table.campaignId, puzzleId);
    expect(lost).toContain('"hasKeyClue":true');
    expect(lost).not.toContain(clueId);
    expect(lost).not.toContain('Cada letra anda');
    expect(lost).not.toContain('"shift"');
    expect(lost).not.toContain('"keyword"');
    expect(lost).not.toContain(CIPHER.message);
    for (const secret of NEVER) {
      expect(lost, secret).not.toContain(secret);
    }

    // The group finds the key in the adventure (the master gives the clue to Pensantus): it is in the player's notes.
    await revealClueRPC(master, table.campaignId, clueId, [table.characterId]);
    await expect(player.getByText('Pista achada na aventura')).toBeVisible({ timeout: 30_000 });
    const found = await playerRunText(player, table.campaignId, puzzleId);
    expect(found).toContain('"keyClueId"');
    expect(found).not.toContain(CIPHER.message);
    await player.locator('.key').getByRole('button', { name: 'Abrir as anotações' }).click();
    await expect(player.getByText(clueText).first()).toBeVisible();
    await player.keyboard.press('Escape');

    // Decoding by hand: the table is never read; a wrong message says so, a right one (without accents or capitals) solves it.
    await player.locator('.col__bet').first().fill('a');
    await player.getByLabel('A mensagem decifrada').fill('o tesouro esta sobre o altar');
    await player.getByRole('button', { name: 'Conferir' }).click();
    await expect(player.getByRole('alert').filter({ hasText: 'Não é isso.' })).toContainText('Confira as letras da tabela.');
    await expect(card.getByText(/Última jogada: Pensantus digitou “o tesouro esta sobre o altar”: errou/)).toBeVisible();
    await player.getByLabel('A mensagem decifrada').fill('O TESOURO ESTA SOB O ALTAR');
    await player.getByRole('button', { name: 'Conferir' }).click();
    await expect(player.getByText('A mensagem foi decifrada.')).toBeVisible();
    await expect(card.getByText('Pensantus resolveu “A carta do Capitão”', { exact: false })).toBeVisible();
    // The answers to the two moves: no plain message, no key, no clue text of the master's.
    await expect.poll(() => answers.length).toBeGreaterThanOrEqual(2);
    for (const body of answers) {
      expect(body).not.toContain(CIPHER.message);
      expect(body).not.toContain('"shift"');
      expect(body).not.toContain('"keyword"');
      expect(body).not.toContain('Cada letra anda');
      for (const secret of NEVER) {
        expect(body, secret).not.toContain(secret);
      }
    }
  } finally {
    await endTable(master, table.campaignId);
    await close();
  }
});

test('a dica por teste de perícia com dado físico: o jogador digita o d20, falha, e depois ganha uma dica só dele @MR-038 @RN-18 @RN-27 @RN-10', async ({ browser }) => {
  const { master, player, close } = await twoPeople(browser);
  const answers = watchAnswers(player);
  const table = await tableForPuzzles(master, player, `Dica ${Date.now()}`);
  const toren = await secondPlayer(browser, master, table.campaignId);
  try {
    await setDiceModeRPC(master, table.campaignId, 'DICE_MODE_PHYSICAL');
    const puzzleId = await createRiddleRPC(master, table.campaignId, 'A porta da Cripta pergunta', {
      hints: ['Pense no que acompanha você ao meio-dia.', 'Ela some quando a tocha apaga.'],
      hintCheck: { skillKey: 'skill:investigation', dc: 13 },
    });
    await showPuzzleRPC(master, table.campaignId, puzzleId);
    await openSessionPage(master, table.campaignId);
    const card = master.getByRole('article', { name: 'A porta da Cripta pergunta' });

    await playerOpens(player, table.campaignId, puzzleId, 'A porta da Cripta pergunta');
    // The button names the skill and never the DC; with physical dice the player types the d20.
    const go = player.getByRole('button', { name: 'Tentar uma dica · Investigação' });
    await expect(go).toBeVisible();
    await expect(player.getByText('A mesa usa dados físicos: você digita o resultado.')).toBeVisible();
    const text = await playerRunText(player, table.campaignId, puzzleId);
    expect(text).toContain('"hintByCheck":true');
    expect(text).toContain('skill:investigation');
    for (const secret of NEVER) {
      expect(text, secret).not.toContain(secret);
    }
    expect(await player.locator('body').innerText()).not.toMatch(/\bCD\b/);

    // A 1 fails: "Não deu desta vez.", with no number to beat, and no second try for the same hint.
    await go.click();
    await expect(player.getByText('Teste de Investigação. Role o seu d20 e digite o dado, sem o bônus.')).toBeVisible();
    await player.getByLabel('O d20 que você rolou').fill('1');
    await player.getByRole('button', { name: 'Confirmar 1' }).click();
    await expect(player.getByText('Não deu desta vez.')).toBeVisible();
    await expect(player.getByText('Outro jogador pode tentar, ou o mestre solta uma dica.')).toBeVisible();
    // The player's own total on a fail, as on a pass, and never what it had to reach.
    await expect(player.getByText(/Você tirou \d+\./)).toBeVisible();
    await expect(player.getByRole('button', { name: /^Tentar uma dica/ })).toHaveCount(0);
    expect(await player.locator('body').innerText()).not.toMatch(/\bCD\b/);
    // The master reads the roll, and who.
    await expect(card.getByText(/Pensantus rolou \d+( \(d20: 1\))?( \(dado físico\))? para a dica 1: não passou\./)).toBeVisible();

    // The master releases hint 1 to everyone; now the next hint can be tried: a 20 passes and the hint is the player's alone.
    await card.getByRole('button', { name: 'Mostrar a próxima dica' }).click();
    await expect(player.getByText('Pense no que acompanha você ao meio-dia.')).toBeVisible();
    await player.getByRole('button', { name: 'Tentar uma dica · Investigação' }).click();
    await player.getByLabel('O d20 que você rolou').fill('20');
    await player.getByRole('button', { name: 'Confirmar 20' }).click();
    await expect(player.getByText('Você conseguiu.')).toBeVisible();
    await expect(player.getByText('Esta dica é só sua; se quiser, conte aos outros.')).toBeVisible();
    await expect(player.getByText('Ela some quando a tocha apaga.')).toBeVisible();
    await expect(card.getByText(/Pensantus rolou \d+( \(d20: 20\))?( \(dado físico\))? para a dica 2: passou\./)).toBeVisible();

    // The other player reads the hint the master released, never the one Pensantus won (RN-27).
    const torensText = await playerRunText(toren.page, table.campaignId, puzzleId);
    expect(torensText).toContain('Pense no que acompanha você ao meio-dia.');
    expect(torensText).not.toContain('Ela some quando a tocha apaga.');
    const own = JSON.parse(await playerRunText(player, table.campaignId, puzzleId)).run as { hints: string[]; sharedHints: number };
    expect(own.hints).toEqual(['Pense no que acompanha você ao meio-dia.', 'Ela some quando a tocha apaga.']);
    expect(own.sharedHints).toBe(1);
    // The answers to the two tries carry the player's own roll and the hint won, never the DC or the check.
    await expect.poll(() => answers.length).toBeGreaterThanOrEqual(2);
    for (const body of answers) {
      for (const secret of NEVER) {
        expect(body, secret).not.toContain(secret);
      }
      expect(body).not.toMatch(/"dc"|\bdc\b/i);
    }
  } finally {
    await toren.close();
    await endTable(master, table.campaignId);
    await close();
  }
});

test('a informação dividida: o mestre dá uma parte a cada jogador e cada um só recebe a sua @MR-038 @RN-27 @RN-10', async ({ browser }) => {
  const { master, player, close } = await twoPeople(browser);
  const table = await tableForPuzzles(master, player, `Partes ${Date.now()}`);
  const toren = await secondPlayer(browser, master, table.campaignId);
  try {
    const mine = '“…os tambores ecoam três vezes antes de a porta ceder.”';
    const hers = '“A porta ouve o que o chão esconde…”';
    // The master's form: two parts, each for one player's character.
    await master.goto(puzzleRoute(table.campaignId, 'puzzles', 'new'));
    await master.getByRole('radio', { name: /^Enigma/ }).check();
    await master.getByLabel('Nome').fill('A porta da Cripta pergunta');
    await master.getByLabel('O enigma').fill(RIDDLE.text);
    await master.getByLabel(/^(Uma|Outra) resposta$/).fill('sombra');
    await master.getByRole('button', { name: 'Adicionar', exact: true }).click();
    await master.getByRole('button', { name: 'Adicionar parte' }).click();
    await master.getByLabel('Para quem (parte 1)').selectOption(table.characterId);
    await master.getByLabel('O que o jogador lê (parte 1)').fill(mine.replace(/[“”]/g, ''));
    await master.getByRole('button', { name: 'Adicionar parte' }).click();
    await master.getByLabel('Para quem (parte 2)').selectOption(toren.characterId);
    await master.getByLabel('O que o jogador lê (parte 2)').fill(hers.replace(/[“”]/g, ''));
    // A character has one part: Pensantus is no longer offered on the second.
    await expect(master.getByLabel('Para quem (parte 2)').locator('option', { hasText: 'Pensantus' })).toBeDisabled();
    await master.getByRole('button', { name: 'Criar quebra-cabeça' }).click();
    await expect(master).toHaveURL(puzzleRoute(table.campaignId));

    await openSessionPage(master, table.campaignId);
    await master.getByRole('button', { name: 'Mostrar aos jogadores A porta da Cripta pergunta' }).click();
    await expect(master.getByRole('article', { name: 'A porta da Cripta pergunta' }).getByText('Os jogadores veem')).toBeVisible();

    await openSessionPage(player, table.campaignId);
    await player.getByRole('link', { name: 'Abrir o quebra-cabeça' }).click();
    await expect(player.getByRole('heading', { level: 1, name: 'A porta da Cripta pergunta' })).toBeVisible();
    const puzzleId = new URL(player.url()).searchParams.get('puzzle')!;
    // Pensantus's phone: his own part, labelled as only his, and who else has one (a name, never the text).
    await expect(player.getByRole('region', { name: 'A sua parte da pista' })).toContainText(mine.replace(/[“”]/g, ''));
    await expect(player.getByText('Só você vê esta parte.')).toBeVisible();
    await expect(player.getByRole('region', { name: 'Quem mais tem uma parte' })).toContainText('Toren');
    await expect(player.getByText('A porta ouve o que o chão esconde')).toHaveCount(0);

    // The JSON each player reads holds only that player's part (RN-27).
    const pensantus = await playerRunText(player, table.campaignId, puzzleId);
    expect(pensantus).toContain('os tambores ecoam três vezes');
    expect(pensantus).not.toContain('A porta ouve o que o chão esconde');
    expect(pensantus).toContain('"partHolders":["Toren"]');
    const torens = await playerRunText(toren.page, table.campaignId, puzzleId);
    expect(torens).toContain('A porta ouve o que o chão esconde');
    expect(torens).not.toContain('os tambores ecoam três vezes');
    expect(torens).toContain('"partHolders":["Pensantus"]');
    for (const text of [pensantus, torens]) {
      for (const secret of NEVER) {
        expect(text, secret).not.toContain(secret);
      }
      expect(text).not.toContain('"parts"');
      expect(text).not.toContain(table.characterId);
      expect(text).not.toContain(toren.characterId);
    }

    // Toren's phone shows his own.
    await playerOpens(toren.page, table.campaignId, puzzleId, 'A porta da Cripta pergunta');
    await expect(toren.page.getByRole('region', { name: 'A sua parte da pista' })).toContainText('A porta ouve o que o chão esconde');
    await expect(toren.page.getByText('os tambores ecoam três vezes')).toHaveCount(0);
  } finally {
    await toren.close();
    await endTable(master, table.campaignId);
    await close();
  }
});

test('o limite de jogadas para o quebra-cabeça: o jogador lê "parou", os contadores, e o mestre recomeça @MR-038 @RN-27', async ({ browser }) => {
  const { master, player, close } = await twoPeople(browser);
  const table = await tableForPuzzles(master, player, `Limite ${Date.now()}`);
  try {
    const puzzleId = await createRiddleRPC(master, table.campaignId, 'A porta da Cripta pergunta', { onWrong: { maxMoves: 2 } });
    await showPuzzleRPC(master, table.campaignId, puzzleId);
    await openSessionPage(master, table.campaignId);
    const card = master.getByRole('article', { name: 'A porta da Cripta pergunta' });
    await playerOpens(player, table.campaignId, puzzleId, 'A porta da Cripta pergunta');
    await expect(player.locator('app-limit-counters')).toContainText('Jogadas 0 de 2');

    await player.getByLabel('Sua resposta').fill('escuridão');
    await player.getByRole('button', { name: 'Responder' }).click();
    await expect(player.locator('app-limit-counters')).toContainText('Jogadas 1 de 2');
    await player.getByLabel('Sua resposta').fill('luz');
    await player.getByRole('button', { name: 'Responder' }).click();

    // The second move reached the limit: nothing moves, the players read it neutrally, and the counter says so in words.
    await expect(player.getByText('O quebra-cabeça parou.')).toBeVisible();
    await expect(player.getByText('O mestre decide o que acontece agora.')).toBeVisible();
    await expect(player.locator('app-limit-counters')).toContainText('Jogadas 2 de 2 · acabou');
    await expect(player.getByLabel('Sua resposta')).toHaveCount(0);
    const stopped = JSON.parse(await playerRunText(player, table.campaignId, puzzleId)).run as { stopped?: boolean };
    expect(stopped.stopped).toBe(true);
    // The server refuses a move on a stopped puzzle all the same.
    const refused = await player.request.post('/meurpg.play.v1.PuzzleService/MakePuzzleMove', {
      data: { campaignId: table.campaignId, puzzleId, move: { riddle: { answer: 'sombra' } }, idempotencyKey: crypto.randomUUID() },
      headers: { 'Connect-Protocol-Version': '1' },
    });
    expect(refused.status()).toBe(400);
    // The master reads why, and "Recomeçar" gives the table its moves back.
    await expect(card.getByText('O limite de jogadas foi atingido: ninguém joga mais até você recomeçar ou fechar.')).toBeVisible();
    await expect(card.getByText('Parou')).toBeVisible();
    await card.getByRole('button', { name: 'Recomeçar' }).click();
    await card.locator('app-map-ask').getByRole('button', { name: 'Recomeçar', exact: true }).click();
    await expect(player.getByLabel('Sua resposta')).toBeVisible({ timeout: 30_000 });
    await expect(player.locator('app-limit-counters')).toContainText('Jogadas 0 de 2');
    await expect(player.getByText('O quebra-cabeça parou.')).toHaveCount(0);
  } finally {
    await endTable(master, table.campaignId);
    await close();
  }
});

test('os três quebra-cabeças novos cabem em 320 × 568: o enigma vem primeiro e só o campo e "Responder" ficam à vista @MR-038', async ({ browser }) => {
  const { master, player, close } = await twoPeople(browser, { width: 320, height: 568 }, 'dark');
  const table = await tableForPuzzles(master, player, `Cabe ${Date.now()}`);
  const toren = await secondPlayer(browser, master, table.campaignId);
  try {
    const clueId = await sceneClueRPC(master, table.campaignId, table.map.mapId, 'A biblioteca', 'Cada letra anda três para trás.');
    // A riddle with a part, a clue and a hint button: everything that used to push it down the page.
    const riddle = await createRiddleRPC(master, table.campaignId, 'A porta da Cripta pergunta', {
      clue: 'Procure no chão da Cripta, onde a luz da tocha não alcança, e olhe para trás.',
      hints: ['Pense no que acompanha você ao meio-dia.'],
      hintCheck: { skillKey: 'skill:investigation', dc: 13 },
      parts: [
        { characterId: table.characterId, text: '…os tambores ecoam três vezes antes de a porta ceder, e só então o chão responde.' },
        { characterId: toren.characterId, text: 'A porta ouve o que o chão esconde…' },
      ],
      onWrong: { attemptsPerPlayer: 3 },
    });
    const sequence = await createSequenceRPC(master, table.campaignId, 'Os sinos do Salão do trono');
    const cipher = await createCipherRPC(master, table.campaignId, 'A carta do Capitão', { onWrong: { maxMoves: 10, timeLimitSeconds: 300 } }, clueId);
    for (const id of [riddle, sequence, cipher]) {
      await showPuzzleRPC(master, table.campaignId, id);
    }
    const fits = () =>
      player.evaluate(() => ({
        scrolls: document.documentElement.scrollWidth > document.documentElement.clientWidth,
        small: Array.from(document.querySelectorAll<HTMLElement>('main button, main input, main select'))
          .filter((e) => e.getBoundingClientRect().width > 0 && !(e instanceof HTMLInputElement && e.type === 'radio'))
          .filter((e) => !e.closest('.cipher-table, .col, mat-form-field') && Math.min(e.getBoundingClientRect().width, e.getBoundingClientRect().height) < 44)
          .map((e) => `${e.tagName}:${e.getAttribute('aria-label') ?? e.textContent?.trim()}`),
      }));

    await playerOpens(player, table.campaignId, riddle, 'A porta da Cripta pergunta');
    // The riddle comes first, on the first screen, in spite of the part, the clue and the hint button; the field and "Responder" stay at the foot.
    const riddleText = player.getByText(RIDDLE.text);
    await expect(riddleText).toBeInViewport();
    expect((await boxOf(riddleText)).y).toBeLessThan(200);
    await expect(player.getByRole('button', { name: 'Responder' })).toBeInViewport();
    await expect(player.getByLabel('Sua resposta')).toBeInViewport();
    // Only the field and the button are the foot: the counters and the notes scroll with the page.
    const stick = await player.evaluate(() => {
      const bar = document.querySelector('.ask__stick')!;
      const counters = document.querySelector('app-limit-counters')!;
      return { bar: bar.getBoundingClientRect().height, position: getComputedStyle(bar).position, counters: getComputedStyle(counters.closest('.ask__notes')!).position };
    });
    expect(stick.position).toBe('sticky');
    expect(stick.bar).toBeLessThan(150);
    expect(stick.counters).toBe('static');
    expect((await fits()).scrolls).toBe(false);
    // A wrong answer adds a notice and the field still sits at the foot, the riddle still above it.
    await player.getByLabel('Sua resposta').fill('luz');
    await player.getByRole('button', { name: 'Responder' }).click();
    await expect(player.getByRole('alert').filter({ hasText: 'Não é isso.' })).toBeVisible();
    await expect(player.getByRole('button', { name: 'Responder' })).toBeInViewport();
    expect(await fits()).toEqual({ scrolls: false, small: [] });

    await playerOpens(player, table.campaignId, sequence, 'Os sinos do Salão do trono');
    await playSequenceRPC(master, table.campaignId, sequence);
    await expect(player.getByText('Agora é com vocês.')).toBeVisible({ timeout: 30_000 });
    // Four bells two by two, and the six steps on one row.
    const layout = await player.evaluate(() => {
      const bells = Array.from(document.querySelectorAll<HTMLElement>('app-bells-board button')).map((b) => b.getBoundingClientRect());
      const slots = Array.from(document.querySelectorAll<HTMLElement>('app-sequence-strip li')).map((b) => b.getBoundingClientRect());
      return { bellRows: new Set(bells.map((r) => Math.round(r.top))).size, slotRows: new Set(slots.map((r) => Math.round(r.top))).size };
    });
    expect(layout.bellRows).toBe(2);
    expect(await fits()).toEqual({ scrolls: false, small: [] });

    await playerOpens(player, table.campaignId, cipher, 'A carta do Capitão');
    expect(await fits()).toEqual({ scrolls: false, small: [] });
    await expect(player.getByRole('button', { name: 'Conferir' })).toBeVisible();
  } finally {
    await toren.close();
    await endTable(master, table.campaignId);
    await close();
  }
});
