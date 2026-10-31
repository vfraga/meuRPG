import { expect, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { claimRoute, linkRPC, reservedRPC, rowOf } from './claim-support';
import {
  authStatePath,
  callRPC,
  characterRpcBody,
  createCampaign,
  createCharacterRPC,
  createCharacterViaUI,
  idpOrigin,
  newSignedInContext,
  pensantus,
  signIn,
} from './support';

// MR-049 (personagens reservados e links para assumir), against the real screens: the master makes a character for a
// player, sends a link, and the player signs in and takes it. RN-10 (a reserved character is invisible to every player until
// claimed) and RN-03 (one living character per player) are checked on the same flow.
//
// The saved states of auth.setup.ts are reused; the one test that ends a session signs in for real, so it does not end the
// session every other test shares.
test.use({ storageState: authStatePath('Mestre Teste') });

const CHARACTERS = 'meurpg.characters.v1.CharacterService';
const unusablePage = 'Este link não pode ser usado';

/** Whether a player sees the campaign at all (a stranger's `GetCampaign` is `not_found`). */
async function isMember(player: Page, campaignId: string): Promise<boolean> {
  return (await callRPC(player, 'meurpg.campaigns.v1.CampaignService/GetCampaign', { campaignId })).ok();
}

test(
  'o mestre cria um personagem reservado, gera o link, o jogador entra com o Google e assume, e o mestre vê "assumido por"',
  { tag: ['@MR-049', '@RN-10'] },
  async ({ page, browser }) => {
    const campaignName = `Reservados ${Date.now()}`;
    const campaignId = await createCampaign(page, campaignName);

    // "Criar personagem para um jogador": the usual editor, saved as reserved.
    await page.getByRole('link', { name: 'Criar personagem para um jogador' }).click();
    await expect(page).toHaveURL(new RegExp(`/campaigns/${campaignId}/reserved/new$`));
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Criar personagem para um jogador');
    await expect(page.getByText('Personagem reservado.')).toBeVisible();
    await createCharacterViaUI(page, campaignId, { ...pensantus, name: 'Kai' }, `/campaigns/${campaignId}/reserved/new`, true);

    const kai = rowOf(page, 'Kai');
    await expect(kai).toContainText('Sem link');
    await expect(page.getByText('Reservados: ninguém é dono ainda. Os jogadores não os veem até um deles assumir.')).toBeVisible();

    // The link: 7 days by default, shown once, whole, with "Copiar link".
    await kai.getByRole('button', { name: /Gerar link para o jogador/ }).click();
    const dialog = page.getByRole('dialog', { name: 'Gerar link para o jogador' });
    await expect(dialog.getByRole('radio', { name: '7 dias' })).toBeChecked();
    await dialog.getByRole('button', { name: 'Gerar link' }).click();
    const link = await dialog.locator('.claim-link__field').inputValue();
    expect(link).toMatch(/\/claim#t=[A-Za-z0-9_-]{43}$/);
    await expect(dialog.getByText('Este link aparece só agora.')).toBeVisible();
    await expect(dialog.getByRole('button', { name: 'Copiar link' })).toBeFocused();
    await dialog.getByRole('button', { name: 'Fechar' }).last().click();
    await expect(kai).toContainText('Link enviado · vale até');

    const token = new URL(link).hash.replace(/^#t=/, '');
    const playerContext = await browser.newContext({ storageState: authStatePath('Jogador Teste') });
    const guestContext = await browser.newContext({ storageState: { cookies: [], origins: [] } });
    try {
      // Signed out, the page is the same for every link: it says nothing about this campaign or this character.
      const guest = await guestContext.newPage();
      const requested: string[] = [];
      guest.on('request', (request) => requested.push(request.url()));
      await guest.goto(link);
      await expect(guest.getByRole('heading', { level: 1 })).toHaveText('Assumir um personagem');
      await expect(guest.getByText('Entrar não assume nada')).toBeVisible();
      await expect(guest.getByText('Kai')).toHaveCount(0);
      await expect(guest.getByText(campaignName)).toHaveCount(0);
      expect(guest.url()).not.toContain(token); // replaceState took the secret out of the address

      // "Entrar com Google" only brings the person back to the card: signing in never claims.
      await guest.getByRole('button', { name: 'Entrar com Google' }).click();
      await expect(guest).toHaveURL((url) => url.origin === idpOrigin && url.pathname === '/authorize');
      await guest.getByRole('button', { name: 'Jogador Teste', exact: true }).click();
      await expect(guest).toHaveURL(/\/claim$/);
      await expect(guest.getByRole('heading', { level: 1 })).toHaveText('Este personagem é seu?');
      await expect(guest.getByText(`Campanha ${campaignName}`)).toBeVisible();
      await expect(guest.getByText('Kai', { exact: true })).toBeVisible();
      // The test master has chosen no name, so the card says who sent it without one.
      await expect(guest.getByText(`Enviado pelo mestre de ${campaignName}.`)).toBeVisible();
      expect(requested.filter((url) => url.includes(token))).toEqual([]);

      // Still the master's: nobody is a member and the link still waits.
      await page.reload();
      await expect(rowOf(page, 'Kai')).toContainText('Link enviado');
      const guestAsPlayer = await playerContext.newPage();
      await guestAsPlayer.goto('/');
      expect(await isMember(guestAsPlayer, campaignId)).toBe(false);

      // Only the button claims.
      await guest.getByRole('button', { name: 'Assumir este personagem' }).click();
      await expect(guest.getByRole('heading', { level: 1 })).toHaveText('Pronto: Kai é seu');
      await expect(guest.getByText(`Você agora é membro de ${campaignName} e o dono de Kai.`)).toBeVisible();
      await guest.getByRole('link', { name: 'Abrir a ficha de Kai' }).click();
      await expect(guest).toHaveURL(new RegExp(`/campaigns/${campaignId}/characters/[^/]+$`));
      await expect(guest.getByRole('heading', { level: 1 })).toContainText('Kai');

      // The link is spent: any other person, signed in, reads the one page of every link that does not work.
      await guestAsPlayer.goto(claimRoute(token));
      await expect(guestAsPlayer.getByRole('heading', { level: 1 })).toHaveText(unusablePage);
    } finally {
      await guestContext.close();
      await playerContext.close();
    }

    // The master: "Assumido por", and the character in the players' list with everything that goes with it.
    await page.reload();
    await expect(rowOf(page, 'Kai')).toContainText('Assumido por um jogador sem nome');
    await expect(rowOf(page, 'Kai').getByRole('link', { name: /Ver ficha/ })).toBeVisible();
  },
);

test(
  'o mestre devolve o personagem à reserva: o jogador continua na campanha e deixa de ver o personagem',
  { tag: ['@MR-049', '@RN-10'] },
  async ({ page, browser }) => {
    const campaignId = await createCampaign(page, `Devolver ${Date.now()}`);
    const kaiId = await reservedRPC(page, campaignId, 'Kai');
    const token = await linkRPC(page, campaignId, kaiId);
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const player = await playerContext.newPage();
      await player.goto(claimRoute(token));
      await player.getByRole('button', { name: 'Assumir este personagem' }).click();
      await expect(player.getByRole('heading', { level: 1 })).toHaveText('Pronto: Kai é seu');
      await player.goto(`/campaigns/${campaignId}`);
      await expect(player.getByRole('link', { name: /Kai/ }).first()).toBeVisible();

      await page.goto(`/campaigns/${campaignId}`);
      const row = rowOf(page, 'Kai');
      await row.getByRole('button', { name: /Devolver à reserva: Kai/ }).click();
      const question = row.getByRole('group', { name: 'Devolver Kai à reserva?' });
      await expect(question).toContainText('Kai fica sem dono e o link deixa de existir.');
      await expect(question.getByRole('button', { name: 'Cancelar' })).toBeFocused();
      await question.getByRole('button', { name: 'Devolver à reserva' }).click();
      await expect(page.getByText('Kai voltou para a reserva.')).toBeVisible();
      await expect(rowOf(page, 'Kai')).toContainText('Sem link');

      // RN-10: gone for the player at once, who stays a member.
      await player.goto(`/campaigns/${campaignId}`);
      await expect(player.getByText('Você ainda não tem personagem nesta campanha.')).toBeVisible();
      await expect(player.getByText('Kai')).toHaveCount(0);
      expect(await isMember(player, campaignId)).toBe(true);
    } finally {
      await playerContext.close();
    }
  },
);

test(
  'um link revogado, usado, inventado ou do próprio mestre mostra a página certa, e a revogação pede confirmação na linha',
  { tag: ['@MR-049'] },
  async ({ page, browser }) => {
    const campaignId = await createCampaign(page, `Links ${Date.now()}`);
    const salviaId = await reservedRPC(page, campaignId, 'Sálvia');
    const token = await linkRPC(page, campaignId, salviaId);

    // The master opening their own link reads that it is for a player; nothing is claimed.
    await page.goto(claimRoute(token));
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Este link é para um jogador');
    await expect(page.getByText('Este link é para um jogador. Copie e envie para ele.')).toBeVisible();

    // Revoking asks in place, on the same character, with the focus on "Cancelar".
    await page.goto(`/campaigns/${campaignId}`);
    const row = rowOf(page, 'Sálvia');
    await row.getByRole('button', { name: /Revogar o link de Sálvia/ }).click();
    const question = row.getByRole('group', { name: 'Revogar o link de Sálvia?' });
    await expect(question.getByRole('button', { name: 'Cancelar' })).toBeFocused();
    await question.getByRole('button', { name: 'Cancelar' }).click();
    await expect(row.getByRole('button', { name: /Revogar o link de Sálvia/ })).toBeFocused();
    await row.getByRole('button', { name: /Revogar o link de Sálvia/ }).click();
    await row.getByRole('group').getByRole('button', { name: 'Revogar o link' }).click();
    await expect(rowOf(page, 'Sálvia')).toContainText('Link revogado');
    await expect(rowOf(page, 'Sálvia').getByRole('button', { name: /Gerar novo link/ })).toBeVisible();

    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const player = await playerContext.newPage();
      const inventado = 'A'.repeat(43);
      // A revoked link, one that never existed and one that is no token at all: the same words.
      for (const secret of [token, inventado, 'x']) {
        await player.goto(claimRoute(secret));
        await expect(player.getByRole('heading', { level: 1 })).toHaveText(unusablePage);
        await expect(player.getByText('Este link não existe ou não vale mais.')).toBeVisible();
        await expect(player.getByText('Peça ao mestre um link novo.')).toBeVisible();
      }
      expect(await isMember(player, campaignId)).toBe(false);
    } finally {
      await playerContext.close();
    }

    // Excluir revokes the live link: the character and its link are gone.
    const novo = await linkRPC(page, campaignId, salviaId);
    await page.goto(`/campaigns/${campaignId}`);
    await rowOf(page, 'Sálvia').getByRole('button', { name: 'Excluir Sálvia' }).click();
    const ask = rowOf(page, 'Sálvia').getByRole('group', { name: 'Excluir Sálvia?' });
    await expect(ask).toContainText('O personagem é apagado e o link enviado deixa de valer. Isto não se desfaz.');
    await ask.getByRole('button', { name: 'Excluir Sálvia' }).click();
    await expect(page.getByText('Sálvia foi excluído.')).toBeVisible();
    await expect(rowOf(page, 'Sálvia')).toHaveCount(0);
    const playerContext2 = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const player = await playerContext2.newPage();
      await player.goto(claimRoute(novo));
      await expect(player.getByRole('heading', { level: 1 })).toHaveText(unusablePage);
    } finally {
      await playerContext2.close();
    }
    // The link made for a character already deleted never existed for anyone else either.
    const gone = await callRPC(page, `${CHARACTERS}/CreateClaimLink`, { campaignId, characterId: salviaId });
    expect(gone.status()).toBe(404);
  },
);

test(
  'quem já tem um personagem vivo na campanha não assume outro: a única recusa específica, sem id de regra',
  { tag: ['@MR-049', '@RN-03'] },
  async ({ page, browser }) => {
    const campaignId = await createCampaign(page, `Um por jogador ${Date.now()}`);
    const kaiId = await reservedRPC(page, campaignId, 'Kai');
    const token = await linkRPC(page, campaignId, kaiId);
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const player = await playerContext.newPage();
      // The player is in the campaign with a living character of their own.
      await page.goto(`/campaigns/${campaignId}`);
      await page.getByRole('button', { name: 'Gerar convite' }).click();
      const invite = (await page.locator('.invite-reveal__link').textContent())?.trim();
      await player.goto(invite!);
      await expect(player).toHaveURL(new RegExp(`/campaigns/${campaignId}$`));
      const icaro = await createCharacterRPC(player, campaignId, characterRpcBody('PLAYER', { ...pensantus, name: 'Ícaro' }));
      expect(icaro.ok()).toBeTruthy();

      await player.goto(claimRoute(token));
      await player.getByRole('button', { name: 'Assumir este personagem' }).click();
      await expect(player.getByRole('heading', { level: 1 })).toHaveText('Você já tem um personagem vivo aqui');
      await expect(player.getByText('Você já tem um personagem vivo nesta campanha: Ícaro.')).toBeVisible();
      await expect(player.getByText('fale com o mestre: dá para devolver Ícaro à reserva ou mandar o link de Kai para outra pessoa.')).toBeVisible();
      await expect(player.getByText(/RN-\d/)).toHaveCount(0);
      await expect(player.getByRole('link', { name: 'Abrir a ficha de Ícaro' })).toBeVisible();

      // Nothing was taken: the link still waits.
      await page.reload();
      await expect(rowOf(page, 'Kai')).toContainText('Link enviado');
    } finally {
      await playerContext.close();
    }
  },
);

test(
  '"Não é você? Entrar com outra conta" encerra esta sessão e leva à escolha de conta, sem perder o link',
  { tag: ['@MR-049'] },
  async ({ page, browser }) => {
    const campaignId = await createCampaign(page, `Outra conta ${Date.now()}`);
    const kaiId = await reservedRPC(page, campaignId, 'Kai');
    const token = await linkRPC(page, campaignId, kaiId);
    // A real sign-in of its own (the button ends this session on the server): the saved states stay untouched.
    const context = await browser.newContext({ storageState: { cookies: [], origins: [] } });
    try {
      const person = await context.newPage();
      await signIn(person, 'Jogador Teste', '/');
      await person.goto(claimRoute(token));
      await expect(person.getByRole('heading', { level: 1 })).toHaveText('Este personagem é seu?');
      await person.getByRole('button', { name: 'Não é você? Entrar com outra conta' }).click();
      await expect(person).toHaveURL((url) => url.origin === idpOrigin && url.pathname === '/authorize');
      await person.getByRole('button', { name: 'E-mail Não Verificado', exact: true }).click();
      await expect(person.getByRole('heading', { level: 1 })).toHaveText('Este personagem é seu?');
      await person.getByRole('button', { name: 'Assumir este personagem' }).click();
      await expect(person.getByRole('heading', { level: 1 })).toHaveText('Pronto: Kai é seu');
    } finally {
      await context.close();
    }
  },
);
