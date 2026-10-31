import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { combatRPC, tableForCombat } from './combat-support';
import { endOpenSessionRPC, openSessionPage } from './live-session-support';
import { openSceneRPC, tableForScenes } from './scene-support';
import { createCapitaoRPC, createMiraRPC, getStageRPC, playedCombatRPC, putOnStageRPC, uploadPortrait } from './stage-support';
import { callRPC, newSignedInContext } from './support';
import { waitForEncounterRevision } from './revision-support';

// MR-031 (NPC portraits and the stage) and MR-032 (the combat highlights),
// through the screens. Setup (campaigns, maps, sessions, NPCs) goes through
// the API; every test makes its own campaign.

test(
  'o mestre dá um retrato à Mira, abre a cena, põe a Mira e o Capitão em cena, o jogador os vê, abre a Mira maior, e quando a Mira sai a imagem dela some para ele',
  { tag: ['@MR-031', '@RN-20'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    const m = await masterContext.newPage();
    const p = await playerContext.newPage();
    let campaignId = '';
    try {
      await m.goto('/');
      const table = await tableForScenes(m, p, `Palco ${Date.now()}`, true);
      campaignId = table.campaignId;
      const miraImage = await uploadPortrait(m, campaignId, 'Retrato da Mira');
      const capitaoImage = await uploadPortrait(m, campaignId, 'Retrato do Capitão', '#6e8a52');
      const miraId = await createMiraRPC(m, campaignId);
      await createCapitaoRPC(m, campaignId, capitaoImage);

      // The editor: the initials first, then the portrait, saved with the form.
      await m.goto(`/campaigns/${campaignId}/characters/${miraId}/edit`);
      const field = m.locator('app-portrait-field');
      await expect(field.getByText('Sem retrato: aparecem as iniciais.')).toBeVisible();
      await expect(field.locator('.pt__initials')).toHaveText('MI');
      await field.getByRole('button', { name: 'Escolher retrato' }).click();
      const picker = m.getByRole('dialog', { name: 'Escolher o retrato de Mira' });
      const use = picker.getByRole('button', { name: 'Usar este retrato' });
      await expect(use).toHaveAttribute('aria-disabled', 'true');
      await expect(picker.getByText('Escolha uma imagem.')).toBeVisible();
      await picker.getByRole('radio', { name: /Retrato da Mira/ }).click();
      await expect(use).not.toHaveAttribute('aria-disabled', 'true');
      await use.click();
      await expect(picker).toBeHidden();
      await expect(field.getByText('Imagem da galeria: “Retrato da Mira”')).toBeVisible();
      await m.getByRole('button', { name: 'Salvar ficha' }).click();
      await expect(m).toHaveURL(new RegExp(`/characters/${miraId}$`));
      await m.goto(`/campaigns/${campaignId}/characters/${miraId}/edit`);
      await expect(m.locator('app-portrait-field img')).toHaveAttribute('src', `/images/${miraImage}`);

      // The scene, and the master's stage.
      await openSceneRPC(m, campaignId, table.cartId);
      await openSessionPage(m, campaignId);
      await openSessionPage(p, campaignId);
      await expect(m.getByRole('heading', { name: 'Cena: A carroça tombada' })).toBeVisible();
      await expect(p.getByRole('region', { name: 'Cena: A carroça tombada' })).toBeVisible();
      // Nobody on stage: the player's stage is not there at all.
      await expect(p.getByRole('group', { name: /^Em cena/ })).toHaveCount(0);
      await expect(m.getByText('Ninguém em cena.')).toBeVisible();

      await m.getByRole('button', { name: 'Pôr em cena', exact: true }).click();
      await expect(m.getByRole('heading', { name: 'Pôr em cena', level: 4 })).toBeFocused();
      await m.getByRole('button', { name: 'Pôr em cena: Mira' }).click();
      await expect(m.getByText('Mira entrou na cena.').first()).toBeVisible();
      await m.getByRole('button', { name: 'Pôr em cena: Capitão Goblin' }).click();
      await expect(m.locator('.st__count')).toHaveText('2 de 4');
      await m.getByRole('button', { name: 'Fechar', exact: true }).click();
      await expect(m.getByRole('button', { name: 'Pôr em cena', exact: true })).toBeFocused();
      await m.getByRole('button', { name: 'Dar a fala a Capitão Goblin' }).click();
      const speaker = m.getByRole('button', { name: 'Fala agora: Capitão Goblin está com a fala. Tirar a fala' });
      await expect(speaker).toHaveAttribute('aria-pressed', 'true');

      // The player: both on the stage, the speaker marked, no kind, no numbers.
      const stage = p.getByRole('group', { name: 'Em cena: Mira e Capitão Goblin' });
      await expect(stage).toBeVisible();
      await expect(stage.getByText('Fala agora')).toBeVisible();
      await expect(stage.getByRole('button', { name: 'Capitão Goblin Fala agora (ver maior)' })).toBeVisible();
      await expect(stage).not.toContainText(/Inimigo|PV|CA\b/);
      await expect(stage.locator('img')).toHaveCount(2);
      // Each one is a chip: the portrait in a 48 px square tile, contained so it is never cropped (C5-01).
      const tile = await stage.locator('img').first().evaluate((img) => {
        const rect = img.getBoundingClientRect();
        return { fit: getComputedStyle(img).objectFit, width: Math.round(rect.width), height: Math.round(rect.height) };
      });
      expect(tile).toEqual({ fit: 'contain', width: 48, height: 48 });

      // Tap Mira: the larger view, only her name and picture.
      await stage.getByRole('button', { name: 'Mira (ver maior)' }).click();
      const big = p.getByRole('dialog', { name: 'Mira' });
      await expect(big.getByRole('heading', { name: 'Mira' })).toBeFocused();
      await expect(big.locator('img')).toHaveAttribute('src', `/images/${miraImage}`);
      expect((await big.innerText()).replace(/\s+/g, ' ').trim()).toMatch(/^Mira\s*close\s*Fechar$|^Mira\s*Fechar$/);
      // The portrait answers for the player while she is on the stage.
      expect((await p.request.get(`/images/${miraImage}`)).status()).toBe(200);

      // The master takes Mira off at once: the view closes and her image is a 404.
      await m.getByRole('button', { name: 'Mais ações de Mira' }).click();
      await m.getByRole('menuitem', { name: 'Tirar de cena: Mira' }).click();
      await expect(big).toBeHidden();
      await expect(stage).toHaveCount(0);
      await expect(p.getByRole('group', { name: 'Em cena: Capitão Goblin' })).toBeVisible();
      await expect(p.getByText('Mira saiu da cena.')).toHaveCount(1);
      expect((await p.request.get(`/images/${miraImage}`)).status()).toBe(404);
      // The master still reads it.
      expect((await m.request.get(`/images/${miraImage}`)).status()).toBe(200);
      expect((await getStageRPC(p, campaignId)).map((n) => n.name)).toEqual(['Capitão Goblin']);
      // A player never gets the character id of an NPC on the stage.
      expect((await getStageRPC(p, campaignId))[0].characterId ?? '').toBe('');
    } finally {
      if (campaignId) {
        await endOpenSessionRPC(m, campaignId);
      }
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'o palco cheio diz "4 de 4" e por quê, e fechar a cena esvazia o palco',
  { tag: ['@MR-031'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    const m = await masterContext.newPage();
    const p = await playerContext.newPage();
    let campaignId = '';
    try {
      await m.goto('/');
      const table = await tableForScenes(m, p, `Palco cheio ${Date.now()}`, true);
      campaignId = table.campaignId;
      const ids = [await createMiraRPC(m, campaignId), await createCapitaoRPC(m, campaignId)];
      for (const name of ['Aldo', 'Barão Ivo', 'Goblin']) {
        const res = await callRPC(m, 'meurpg.characters.v1.CharacterService/CreateCharacter', {
          campaignId,
          kind: 'CHARACTER_KIND_STORY',
          name,
          sheet: { basic: { hitPointsMax: 4, armorClass: 10, speedFt: 30, attackBonus: 0, damage: '', description: '' } },
        });
        expect(res.ok(), await res.text()).toBeTruthy();
        ids.push((await res.json()).character.id as string);
      }
      await openSceneRPC(m, campaignId, table.cartId);
      for (const id of ids.slice(0, 4)) {
        await putOnStageRPC(m, campaignId, id);
      }
      await openSessionPage(m, campaignId);
      await openSessionPage(p, campaignId);
      await expect(m.locator('.st__count')).toHaveText('4 de 4');
      await expect(m.getByText('A cena comporta 4 NPCs. Tire um para pôr outro.')).toBeVisible();
      await expect(m.getByRole('button', { name: 'Pôr em cena', exact: true })).toHaveAttribute('aria-disabled', 'true');
      await expect(p.getByRole('group', { name: /^Em cena/ }).getByRole('button')).toHaveCount(4);

      await m.getByRole('button', { name: 'Fechar cena' }).click();
      await expect(p.getByRole('group', { name: /^Em cena/ })).toHaveCount(0);
      expect(await getStageRPC(m, campaignId)).toEqual([]);
    } finally {
      if (campaignId) {
        await endOpenSessionRPC(m, campaignId);
      }
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'depois de um combate, o mestre vê os "Destaques do combate" com a tabela de cada jogador, e o jogador vê o cartão sem a tabela, com "Você" e o resultado dele',
  { tag: ['@MR-032', '@RN-20'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    const m = await masterContext.newPage();
    const p = await playerContext.newPage();
    let campaignId = '';
    try {
      await m.goto('/');
      const table = await tableForCombat(m, p, `Destaques ${Date.now()}`, true, true);
      campaignId = table.campaignId;
      const enc = await playedCombatRPC(m, p, table);

      await openSessionPage(m, campaignId);
      await openSessionPage(p, campaignId);
      // eslint-disable-next-line meurpg/rpc-revision-wait -- an ended combat leaves the player's combat view; the player's card asserted below is the wait
      const ended = await combatRPC(m, 'EndEncounter', { campaignId, encounterId: enc.id });
      await waitForEncounterRevision(m, ended);

      // The master: the panel, with the table of every player.
      const panel = m.getByRole('region', { name: 'Destaques do combate' });
      await expect(panel).toBeVisible();
      await expect(panel.getByText('Mais dano causado')).toBeVisible();
      await expect(panel.locator('.tile__value').first()).toContainText(/^\d+.de.dano$/);
      await expect(panel.getByRole('heading', { name: 'Números de cada jogador' })).toBeVisible();
      await expect(panel.getByRole('table', { name: 'Números de cada jogador' })).toContainText('Pensantus');
      // A category nobody has anything in is left out.
      await expect(panel.getByText('Mais cura', { exact: true })).toHaveCount(0);

      // The player: the card, with "Você" and no table; "Fechar" puts it away.
      const card = p.getByRole('region', { name: 'Destaques do combate' });
      await expect(card.getByRole('heading', { name: 'O combate acabou' })).toBeVisible();
      await expect(card.getByText('Mais dano causado')).toBeVisible();
      await expect(card.getByText('Você', { exact: true })).toBeVisible();
      await expect(card.getByText('Seu resultado, Pensantus')).toBeVisible();
      await expect(card.getByText('Dano recebido')).toBeVisible();
      await expect(card.getByText('Cura', { exact: true })).toBeVisible();
      await expect(p.getByText('Números de cada jogador')).toHaveCount(0);
      await card.getByRole('button', { name: 'Fechar', exact: true }).last().click();
      await expect(card).toHaveCount(0);
      // A player's request for the table gets exactly one row: their own character's.
      const res = await callRPC(p, 'meurpg.play.v1.CombatService/GetCombatHighlights', { campaignId, encounterId: enc.id });
      expect(((await res.json()).characters as { name: string }[]).map((c) => c.name)).toEqual(['Pensantus']);
    } finally {
      if (campaignId) {
        await endOpenSessionRPC(m, campaignId);
      }
      await masterContext.close();
      await playerContext.close();
    }
  },
);
