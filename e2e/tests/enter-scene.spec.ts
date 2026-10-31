import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { endOpenSessionRPC, openSessionPage } from './live-session-support';
import { openSceneOnlyToRPC, readSceneRPC, setMayJoinRPC, tableForScenes } from './scene-support';
import { newSignedInContext } from './support';
import { toren } from './combat-support';
import { secondPlayer } from './theatre-support';

// MR-015 (a scene open to only some players), the player's side (C7-06): a player who is outside the scene sees no scene at all; with the
// master's switch "Os jogadores podem entrar" on, only a card with the scene's name and "Entrar na cena"; after entering, the scene as
// any member reads it. Setup (the table, the audience, the switch) goes through the API.

test(
  'quem está fora da cena não vê cena nenhuma; com o interruptor ligado vê só o cartão "Há uma cena aberta"; ao entrar vê a cena inteira',
  { tag: ['@MR-015', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    const master = await masterContext.newPage();
    const pensantus = await playerContext.newPage();
    const closers: (() => Promise<void>)[] = [];
    try {
      await Promise.all([master.goto('/'), pensantus.goto('/')]);
      const table = await tableForScenes(master, pensantus, `Entrar na cena ${Date.now()}`);
      const { campaignId } = table;
      closers.push(() => endOpenSessionRPC(master, campaignId));
      const outsider = await secondPlayer(browser, master, campaignId, toren);
      closers.push(outsider.close);
      const out = outsider.page;
      await openSceneOnlyToRPC(master, campaignId, table.cartId, 'Pensantus');
      await Promise.all([openSessionPage(pensantus, campaignId), openSessionPage(out, campaignId)]);

      // Pensantus is in: the scene as always. Toren is out, the switch is off: no scene card at all, and the server says no scene either.
      await expect(pensantus.getByRole('region', { name: 'Cena: A carroça tombada' })).toBeVisible();
      await expect(out.getByRole('region', { name: /A carroça tombada/ })).toHaveCount(0);
      await expect(out.getByText('Há uma cena aberta')).toHaveCount(0);
      await expect(out.getByRole('button', { name: 'Entrar na cena' })).toHaveCount(0);
      expect(await readSceneRPC(out, campaignId)).toBeUndefined();
      // What stays on the page without a scene: the map (one row on a phone).
      await expect(out.getByRole('link', { name: 'Ver mapa' })).toBeVisible();

      // The master lets players in: only the name and the button, nothing of the scene (RN-10), and the page says so politely.
      await setMayJoinRPC(master, campaignId, true);
      const card = out.getByRole('region', { name: 'A carroça tombada' });
      await expect(card).toBeVisible();
      await expect(card.getByText('Há uma cena aberta')).toBeVisible();
      await expect(card.getByText('Você ainda não está nela. Entre para ver e participar.')).toBeVisible();
      await expect(out.getByRole('status').filter({ hasText: 'Há uma cena aberta: A carroça tombada. Você pode entrar.' })).toBeAttached();
      await expect(out.getByRole('button', { name: /^Rolar / })).toHaveCount(0);
      await expect(out.getByText('Procurar pistas na carroça')).toHaveCount(0);
      await expect(out.getByRole('tablist', { name: 'Seções da cena' })).toHaveCount(0);
      const seen = await readSceneRPC(out, campaignId);
      expect(seen).toMatchObject({ name: 'A carroça tombada', joinable: true });
      expect(seen?.actions ?? []).toEqual([]);
      expect(seen?.description ?? '').toBe('');

      // Enter: the card gives way to the scene as Pensantus reads it, under "Você entrou na cena", and the title has the focus.
      await card.getByRole('button', { name: 'Entrar na cena' }).click();
      const scene = out.getByRole('region', { name: 'Cena: A carroça tombada' });
      await expect(scene).toBeVisible();
      await expect(out.getByRole('heading', { name: 'Cena: A carroça tombada' })).toBeFocused();
      await expect(out.getByRole('status').filter({ hasText: 'Você entrou na cena: A carroça tombada.' })).toBeAttached();
      await expect(out.getByText('Há uma cena aberta')).toHaveCount(0);
      await expect(scene.getByText('Você entrou na cena')).toBeVisible();
      await expect(out.getByRole('tab', { name: 'Ações' })).toHaveAttribute('aria-selected', 'true');
      await expect(scene.getByRole('button', { name: 'Rolar Procurar pistas na carroça' })).toBeVisible();
      const entered = await readSceneRPC(out, campaignId);
      expect(entered).toMatchObject({ name: 'A carroça tombada' });
      expect(entered?.joinable).toBeFalsy();
      expect(entered?.actions).toHaveLength(5);
    } finally {
      await Promise.all(closers.map((close) => close()));
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'o mestre desliga o interruptor com o cartão na tela: o cartão some e a página diz que a cena não está mais aberta para entrar',
  { tag: ['@MR-015', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    const master = await masterContext.newPage();
    const pensantus = await playerContext.newPage();
    const closers: (() => Promise<void>)[] = [];
    try {
      await Promise.all([master.goto('/'), pensantus.goto('/')]);
      const table = await tableForScenes(master, pensantus, `Entrar na cena, desligado ${Date.now()}`);
      const { campaignId } = table;
      closers.push(() => endOpenSessionRPC(master, campaignId));
      const outsider = await secondPlayer(browser, master, campaignId, toren);
      closers.push(outsider.close);
      const out = outsider.page;
      await openSceneOnlyToRPC(master, campaignId, table.cartId, 'Pensantus');
      await setMayJoinRPC(master, campaignId, true);
      await openSessionPage(out, campaignId);
      await expect(out.getByRole('button', { name: 'Entrar na cena' })).toBeVisible();

      await setMayJoinRPC(master, campaignId, false);
      await expect(out.getByRole('button', { name: 'Entrar na cena' })).toHaveCount(0);
      await expect(out.getByRole('status').filter({ hasText: 'A cena não está mais aberta para entrar.' })).toBeAttached();
      expect(await readSceneRPC(out, campaignId)).toBeUndefined();
    } finally {
      await Promise.all(closers.map((close) => close()));
      await masterContext.close();
      await playerContext.close();
    }
  },
);
