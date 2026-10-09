import { expect, test } from '@playwright/test';

import {
  endOpenSessionRPC,
  endSessionRPC,
  openSessionPage,
  startSessionRPC,
  tableWithPensantus,
  waitForNotice,
} from './live-session-support';
import { callRPC, newSignedInContext, waitForCampaignList } from './support';

// The live session (Etapa 5, slice 5.1): the notice that a session started
// (RN-06), the session link (RN-07), the session page, and the master's
// correction of a character's vitals, live on the player's screen (RN-02).
// None of these tests is about signing in: every context reuses a saved
// state (auth.setup.ts).

test(
  'o jogador com o app aberto vê o aviso quando o mestre inicia a sessão, e o mestre copia o link',
  { tag: ['@MR-011', '@RN-06', '@RN-07'] },
  async ({ browser, baseURL }) => {
    // The notice comes from a poll every 30 s: allow for one full wait.
    test.setTimeout(120_000);
    const master = await newSignedInContext(browser, 'Mestre Teste', {
      permissions: ['clipboard-read', 'clipboard-write'],
    });
    const player = await newSignedInContext(browser, 'Jogador Teste');
    const masterPage = await master.newPage();
    let campaignId = '';
    try {
      const playerPage = await player.newPage();
      await masterPage.goto('/');
      await playerPage.goto('/');
      const name = `Sessão ao vivo ${Date.now()}`;
      ({ campaignId } = await tableWithPensantus(masterPage, playerPage, name));

      // The player has the app open, on "Minhas campanhas".
      await playerPage.goto('/campaigns');
      await waitForCampaignList(playerPage);

      // The master starts the session from the campaign page.
      await masterPage.goto(`/campaigns/${campaignId}`);
      // Pensantus has no cantrip and no prepared spell yet: the master is told, and starts anyway.
      await masterPage.getByRole('button', { name: 'Iniciar sessão' }).click();
      await expect(masterPage.getByText('Pensantus: faltam 3 truques e 7 magias preparadas')).toBeVisible();
      await masterPage.getByRole('button', { name: 'Iniciar mesmo assim' }).click();
      await expect(masterPage.getByText('Sessão 1 em andamento')).toBeVisible();

      // The player's open tab shows the notice, without a reload…
      const enter = await waitForNotice(playerPage, name);
      // …and "Entrar na sessão" opens the session page.
      await enter.click();
      await expect(playerPage).toHaveURL(`/campaigns/${campaignId}/session`);
      await expect(playerPage.getByRole('heading', { level: 1, name: 'Sessão 1' })).toBeVisible();
      await expect(playerPage.getByRole('heading', { level: 2, name: 'Pensantus' })).toBeVisible();

      // The master copies the session link: /campaigns/<id>/session, no secret.
      // The clipboard only works in the focused page: in CI's headless
      // Chromium, reading it from a page in the background never resolves.
      await masterPage.bringToFront();
      await masterPage.getByRole('button', { name: 'Copiar link da sessão' }).click();
      await expect(masterPage.getByRole('button', { name: 'Link copiado' })).toBeVisible();
      const copied = await masterPage.evaluate(() =>
        Promise.race([
          navigator.clipboard.readText(),
          new Promise<string>((_, reject) => setTimeout(() => reject(new Error('reading the clipboard timed out')), 5_000)),
        ]),
      );
      expect(copied).toBe(new URL(`/campaigns/${campaignId}/session`, baseURL).toString());
    } finally {
      if (campaignId) {
        await endOpenSessionRPC(masterPage, campaignId);
      }
      await master.close();
      await player.close();
    }
  },
);

test(
  'quem não é membro abre o link da sessão e vê "Peça um convite ao mestre", sem o nome da campanha',
  { tag: ['@MR-011', '@RN-07'] },
  async ({ browser }) => {
    test.setTimeout(60_000);
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const masterPage = await master.newPage();
      await masterPage.goto('/');
      // A campaign Jogador Teste was never invited to, with a session open.
      const name = `Mesa fechada ${Date.now()}`;
      const created = await callRPC(masterPage, 'meurpg.campaigns.v1.CampaignService/CreateCampaign', {
        name,
        xpMode: 'XP_MODE_ENEMIES',
      });
      const campaignId = (await created.json()).campaign.id as string;
      const sessionId = await startSessionRPC(masterPage, campaignId);

      const playerPage = await player.newPage();
      await playerPage.goto(`/campaigns/${campaignId}/session`);
      // Two calls in a row (the sign-in check, then the campaign's): allow a
      // busy stack the same 30 s as openSessionPage.
      await expect(playerPage.getByRole('heading', { level: 1, name: 'Peça um convite ao mestre' })).toBeVisible({
        timeout: 30_000,
      });
      await expect(playerPage.getByText('Esse link é só para quem já está na campanha.')).toBeVisible();
      await expect(playerPage.getByText(name)).toHaveCount(0);
      await expect(playerPage.getByRole('link', { name: 'Ir para o início' })).toBeVisible();
      await endSessionRPC(masterPage, campaignId, sessionId);
    } finally {
      await master.close();
      await player.close();
    }
  },
);

test(
  'o mestre ajusta os PV durante a sessão e a página aberta do jogador mostra o número novo, sem as notas do mestre',
  { tag: ['@MR-012', '@RN-02', '@RN-11'] },
  async ({ browser }) => {
    test.setTimeout(90_000);
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const masterPage = await master.newPage();
      const playerPage = await player.newPage();
      await masterPage.goto('/');
      await playerPage.goto('/');
      const { campaignId, characterId } = await tableWithPensantus(masterPage, playerPage, `Ajuste ao vivo ${Date.now()}`);
      const secret = `Segredo do mestre ${Date.now()}: Pensantus herda a torre.`;
      const notes = await callRPC(masterPage, 'meurpg.characters.v1.CharacterService/UpdateMasterNotes', {
        campaignId,
        characterId,
        notes: secret,
      });
      expect(notes.ok()).toBeTruthy();
      const sessionId = await startSessionRPC(masterPage, campaignId);

      // The player follows the session: Pensantus at full hit points.
      await openSessionPage(playerPage, campaignId);
      const vitals = playerPage.getByRole('region', { name: 'Pensantus' });
      await expect(vitals.getByText('23', { exact: true })).toBeVisible();
      await expect(vitals.getByText('de 23')).toBeVisible();
      // Marks this document: a reload would lose it.
      await playerPage.evaluate(() => ((window as unknown as { sameDocument: boolean }).sameDocument = true));

      // The master takes 5 HP in the adjust sheet and saves.
      await openSessionPage(masterPage, campaignId);
      await masterPage.getByRole('button', { name: 'Ajustar Pensantus' }).click();
      const dialog = masterPage.getByRole('dialog', { name: 'Ajustar Pensantus' });
      await dialog.getByRole('button', { name: 'Tirar 5 PV' }).click();
      await expect(dialog.getByRole('spinbutton', { name: 'Pontos de vida atuais' })).toHaveValue('18');
      await dialog.getByRole('button', { name: 'Salvar ajuste' }).click();
      await expect(dialog).toBeHidden();
      await expect(masterPage.getByText('18', { exact: true })).toBeVisible();

      // The player's page shows it, in the same document.
      await expect(vitals.getByText('18', { exact: true })).toBeVisible();
      expect(await playerPage.evaluate(() => (window as unknown as { sameDocument?: boolean }).sameDocument)).toBe(true);

      // RN-11: the master's notes never reach the player's page.
      await expect(playerPage.getByText(secret)).toHaveCount(0);
      expect(await playerPage.content()).not.toContain(secret);
      await endSessionRPC(masterPage, campaignId, sessionId);
    } finally {
      await master.close();
      await player.close();
    }
  },
);

test(
  'o ajuste não passa do máximo, e salvar depois do fim da sessão mostra que ela acabou',
  { tag: ['@RN-02'] },
  async ({ browser }) => {
    test.setTimeout(90_000);
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const masterPage = await master.newPage();
      const playerPage = await player.newPage();
      await masterPage.goto('/');
      await playerPage.goto('/');
      const { campaignId } = await tableWithPensantus(masterPage, playerPage, `Limite ${Date.now()}`);
      const sessionId = await startSessionRPC(masterPage, campaignId);

      await openSessionPage(masterPage, campaignId);
      await masterPage.getByRole('button', { name: 'Ajustar Pensantus' }).click();
      const dialog = masterPage.getByRole('dialog', { name: 'Ajustar Pensantus' });

      // At the maximum (23), the steppers that add stop.
      await expect(dialog.getByText('máximo 23')).toBeVisible();
      await expect(dialog.getByRole('button', { name: 'Somar 1 PV', exact: true })).toBeDisabled();
      await expect(dialog.getByRole('button', { name: 'Somar 5 PV', exact: true })).toBeDisabled();

      // A typed value above it is refused, and nothing is sent.
      const hp = dialog.getByRole('spinbutton', { name: 'Pontos de vida atuais' });
      await hp.fill('30');
      await expect(dialog.getByText('Use um número de 0 a 23.')).toBeVisible();
      let sent = 0;
      masterPage.on('request', (req) => {
        if (req.url().endsWith('/AdjustCharacterVitals')) {
          sent++;
        }
      });
      await dialog.getByRole('button', { name: 'Salvar ajuste' }).click();
      await expect(hp).toBeFocused();
      expect(sent).toBe(0);

      // A valid value, but the session ends before the save.
      await hp.fill('20');
      await endSessionRPC(masterPage, campaignId, sessionId);
      await dialog.getByRole('button', { name: 'Salvar ajuste' }).click();
      await expect(dialog.getByText('A sessão acabou.')).toBeVisible();
      await dialog.getByRole('button', { name: 'Fechar' }).click();
      // The session ended: the master lands on its summary (MR-032).
      await expect(masterPage.getByRole('heading', { level: 1, name: 'Sessão 1' })).toBeVisible();
      await expect(masterPage.getByRole('heading', { name: 'Sessão encerrada' })).toBeVisible();
    } finally {
      await master.close();
      await player.close();
    }
  },
);
