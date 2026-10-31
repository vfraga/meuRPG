import { expect } from '@playwright/test';
import { test } from './diagnostics';

import {
  acceptInvite,
  authStatePath,
  callRPC,
  characterRpcBody,
  createCampaign,
  createCharacterRPC,
  expectCharacterBlocked,
  newSignedInContext,
  pensantus,
  startGameSession,
} from './support';

// MR-006 / RN-01 (trava da ficha): the player edits their own sheet freely
// until the campaign's first session starts; after that, only the master
// and the system (PlayService.StartGameSession's LockSheets) change it. The
// character's descriptive story (amendment A3, corrected by the integrator
// on 29/09/2026) is separate: once the sheet locks, the player can no
// longer edit the story either, until the master grants it per character
// through the master-only CharacterService.SetStoryEditing — buttons
// "Permitir editar a história" / "Travar a história" on the sheet's
// "História" section. The master can always edit the story.
//
// None of these tests' own story is signing in, so every context reuses a
// saved state (auth.setup.ts) instead of hitting the real, rate-limited
// /auth/login. The default page is the master's.
test.use({ storageState: authStatePath('Mestre Teste') });

test('antes de qualquer sessão, o jogador edita a própria ficha e a alteração é salva', { tag: ['@MR-006', '@RN-01'] }, async ({
  page,
  browser,
}) => {
  const campaignId = await createCampaign(page, `Ficha rascunho ${Date.now()}`);
  await page.getByRole('button', { name: 'Gerar convite' }).click();
  const link = (await page.locator('.invite-reveal__link').textContent())?.trim();

  const playerContext = await newSignedInContext(browser, 'Jogador Teste');
  try {
    const { page: joinedPage } = await acceptInvite(playerContext, link!);

    const created = await createCharacterRPC(joinedPage, campaignId, characterRpcBody('PLAYER', pensantus));
    const character = (await created.json()).character;
    expect(character.canEdit).toBe(true);

    await joinedPage.goto(`/campaigns/${campaignId}/characters/${character.id}`);
    await expect(joinedPage.getByText('Ficha travada desde', { exact: false })).toHaveCount(0);
    await joinedPage.getByRole('link', { name: 'Editar ficha' }).click();
    await joinedPage.getByLabel('Nível').fill('4');
    // Wait for the save's own response before reading the character back:
    // a read sent right after the click can reach the server first and see
    // the old revision (it did, once the save took ~0.5 s on a busy stack).
    const saved$ = joinedPage.waitForResponse((res) =>
      res.url().endsWith('/meurpg.characters.v1.CharacterService/UpdateCharacter'),
    );
    await joinedPage.getByRole('button', { name: 'Salvar ficha' }).click();
    expect((await saved$).ok()).toBe(true);

    // Saved: a fresh GetCharacter shows the new level and a higher revision
    // (optimistic concurrency, AIP-154 style — UpdateCharacter's doc
    // comment).
    const getRes = await callRPC(joinedPage, 'meurpg.characters.v1.CharacterService/GetCharacter', {
      campaignId,
      characterId: character.id,
    });
    const saved = (await getRes.json()).character;
    expect(saved.revision).toBeGreaterThan(character.revision);
    expect(saved.sheet.full.classes[0].level).toBe(4);
  } finally {
    await playerContext.close();
  }
});

test(
  'depois que a primeira sessão começa, o jogador vê a ficha só para leitura, o servidor recusa a edição e o mestre edita',
  { tag: ['@MR-006', '@RN-01'] },
  async ({ page, browser }) => {
    const campaignId = await createCampaign(page, `Ficha travada ${Date.now()}`);
    await page.getByRole('button', { name: 'Gerar convite' }).click();
    const link = (await page.locator('.invite-reveal__link').textContent())?.trim();

    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const { page: joinedPage } = await acceptInvite(playerContext, link!);

      const created = await createCharacterRPC(joinedPage, campaignId, characterRpcBody('PLAYER', pensantus));
      const character = (await created.json()).character;

      const startRes = await startGameSession(page, campaignId);
      expect(startRes.status()).toBe(200);
      // StartGameSession reports how many sheets it locked: just this one
      // player character (StartGameSessionResponse.locked_sheet_count).
      expect((await startRes.json()).lockedSheetCount).toBe(1);

      // The screen: read-only, with the lock banner, no edit button.
      await joinedPage.goto(`/campaigns/${campaignId}/characters/${character.id}`);
      await expect(joinedPage.getByText('Ficha travada desde', { exact: false })).toBeVisible();
      await expect(joinedPage.getByRole('link', { name: 'Editar ficha' })).toHaveCount(0);

      // The edit URL, typed or bookmarked: the lock, before any form.
      await joinedPage.goto(`/campaigns/${campaignId}/characters/${character.id}/edit`);
      await expect(joinedPage.getByText('A ficha está travada', { exact: false })).toBeVisible();
      await expect(joinedPage.getByRole('button', { name: 'Salvar ficha' })).toHaveCount(0);

      // The server: refuses too, not just the screen (defense in depth),
      // with a CharacterBlocked detail (reason SHEET_LOCKED).
      await expectCharacterBlocked(
        await callRPC(joinedPage, 'meurpg.characters.v1.CharacterService/UpdateCharacter', {
          campaignId,
          characterId: character.id,
          revision: character.revision,
          name: character.name,
          sheet: character.sheet,
        }),
        'CHARACTER_BLOCKED_REASON_SHEET_LOCKED',
      );

      // The master: still edits the same, now-locked sheet.
      const masterUpdateRes = await callRPC(page, 'meurpg.characters.v1.CharacterService/UpdateCharacter', {
        campaignId,
        characterId: character.id,
        revision: character.revision,
        name: character.name,
        sheet: character.sheet,
      });
      expect(masterUpdateRes.status()).toBe(200);
    } finally {
      await playerContext.close();
    }
  },
);

test('depois da primeira sessão, o jogador só edita a história quando o mestre permite', { tag: '@RN-01' }, async ({
  page,
  browser,
}) => {
  // Amendment A3, corrected by the integrator (Vinicius, 29/09/2026):
  // RN-01's lock is for game data; the story is personal data
  // (docs/privacy.md: it keeps the right to rectification), but after
  // the lock the player no longer edits it freely — the master grants
  // editing per character (SetStoryEditing). The master can always edit the
  // story, through the same UpdateCharacterStory RPC used in the test
  // above for the sheet-open case.
  const campaignId = await createCampaign(page, `História sob permissão ${Date.now()}`);
  await page.getByRole('button', { name: 'Gerar convite' }).click();
  const link = (await page.locator('.invite-reveal__link').textContent())?.trim();

  const playerContext = await newSignedInContext(browser, 'Jogador Teste');
  try {
    const { page: joinedPage } = await acceptInvite(playerContext, link!);

    const created = await createCharacterRPC(joinedPage, campaignId, characterRpcBody('PLAYER', pensantus));
    const character = (await created.json()).character;

    await startGameSession(page, campaignId);

    // Locked, no permission yet: the player's own attempt is refused, with
    // reason STORY_LOCKED. This "wins over a stale revision" (the RPC's own
    // doc comment), so the original revision stays safe to reuse below too
    // — no need to track it through the successful save in between.
    await expectCharacterBlocked(
      await callRPC(joinedPage, 'meurpg.characters.v1.CharacterService/UpdateCharacterStory', {
        campaignId,
        characterId: character.id,
        revision: character.revision,
        story: { backstory: 'Tentativa sem permissão do mestre.' },
      }),
      'CHARACTER_BLOCKED_REASON_STORY_LOCKED',
    );

    // The master allows it, from their own view of the sheet.
    await page.goto(`/campaigns/${campaignId}/characters/${character.id}`);
    await page.getByRole('button', { name: 'Permitir editar a história' }).click();
    await expect(page.getByRole('button', { name: 'Travar a história' })).toBeVisible();

    // Now the player edits and saves, through the screen.
    await joinedPage.goto(`/campaigns/${campaignId}/characters/${character.id}`);
    await joinedPage.getByRole('button', { name: 'Editar história' }).click();
    await joinedPage.getByLabel('Antecedentes').fill('Pensantus cresceu em Mirathel, entre livros e engrenagens.');
    await joinedPage.getByRole('button', { name: 'Salvar história' }).click();
    await expect(joinedPage.locator('dt:text-is("Antecedentes") + dd')).toHaveText(
      'Pensantus cresceu em Mirathel, entre livros e engrenagens.',
    );

    // The master locks it again.
    await page.getByRole('button', { name: 'Travar a história' }).click();
    await expect(page.getByRole('button', { name: 'Permitir editar a história' })).toBeVisible();

    await expectCharacterBlocked(
      await callRPC(joinedPage, 'meurpg.characters.v1.CharacterService/UpdateCharacterStory', {
        campaignId,
        characterId: character.id,
        revision: character.revision,
        story: { backstory: 'Segunda tentativa, depois da trava de novo.' },
      }),
      'CHARACTER_BLOCKED_REASON_STORY_LOCKED',
    );
  } finally {
    await playerContext.close();
  }
});
