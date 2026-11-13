import { Injectable, inject } from '@angular/core';
import { clone, create } from '@bufbuild/protobuf';
import { createClient } from '@connectrpc/connect';

import { CampaignService, XpMode } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CharacterKind as GenCharacterKind,
  CharacterService,
  CharacterState as GenCharacterState,
  type FullSheet,
  FullSheetSchema,
  WeaponMarkSchema,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { ContentService } from '../../../gen/meurpg/rules/v1/rules_pb';
import type { SpellDetailsVm } from '../../shared/spell-details/spell-details.types';
import { spellDetailsFromGen } from '../../shared/spell-details/spell-details-map';
import { CONNECT_TRANSPORT } from '../../core/connect/transport';
import {
  CampaignXpMode,
  CharacterSheetSource,
  CharacterSheetVm,
  CharacterStoryVm,
  type WeaponMarkVm,
} from './character-sheet.types';
import { toCharacterSheetVm, toCharacterStoryInit } from './character-sheet-map';

/**
 * `CharacterSheetSource` over the generated `CharacterService` client
 * (`meurpg.characters.v1`, phase 2). Provided at the route level for
 * `/campaigns/:id/characters/:characterId` — see
 * `character-sheet.routes.ts` — so this client, and the two generated
 * `_pb.ts` files it pulls in, stay out of the app's eager bundle.
 */
@Injectable()
export class CharacterSheetSourceLive implements CharacterSheetSource {
  private readonly client = createClient(CharacterService, inject(CONNECT_TRANSPORT));
  private readonly campaigns = createClient(CampaignService, inject(CONNECT_TRANSPORT));
  private readonly content = createClient(ContentService, inject(CONNECT_TRANSPORT));

  async loadSpellDetails(campaignId: string, spellKey: string): Promise<SpellDetailsVm> {
    const res = await this.content.getSpellDetails({ campaignId, spellKey });
    return spellDetailsFromGen(res.spell!);
  }

  async getXpMode(campaignId: string): Promise<CampaignXpMode> {
    const res = await this.campaigns.getCampaign({ campaignId });
    switch (res.campaign?.xpMode) {
      case XpMode.MILESTONES:
        return 'milestones';
      case XpMode.GOLD:
        return 'gold';
      default:
        return 'enemies';
    }
  }

  async getCharacterSheet(campaignId: string, characterId: string): Promise<CharacterSheetVm> {
    const res = await this.client.getCharacter({ campaignId, characterId });
    return toCharacterSheetVm(res.character!);
  }

  async getMasterNotes(campaignId: string, characterId: string): Promise<string> {
    const res = await this.client.getMasterNotes({ campaignId, characterId });
    return res.notes;
  }

  async updateMasterNotes(campaignId: string, characterId: string, notes: string): Promise<void> {
    await this.client.updateMasterNotes({ campaignId, characterId, notes });
  }

  async markCharacterDead(campaignId: string, characterId: string): Promise<CharacterSheetVm> {
    const res = await this.client.markCharacterDead({ campaignId, characterId });
    return toCharacterSheetVm(res.character!);
  }

  async updateCharacterStory(
    campaignId: string,
    characterId: string,
    revision: number,
    story: CharacterStoryVm,
  ): Promise<CharacterSheetVm> {
    const res = await this.client.updateCharacterStory({
      campaignId,
      characterId,
      revision,
      story: toCharacterStoryInit(story),
    });
    return toCharacterSheetVm(res.character!);
  }

  async setStoryEditingAllowed(
    campaignId: string,
    characterId: string,
    allowed: boolean,
  ): Promise<CharacterSheetVm> {
    const res = await this.client.setStoryEditing({ campaignId, characterId, allowed });
    return toCharacterSheetVm(res.character!);
  }

  async setEquipped(
    campaignId: string,
    characterId: string,
    key: string,
    equipped: boolean,
  ): Promise<void> {
    await this.client.setEquipped({ campaignId, characterId, key, equipped });
  }

  async setWeaponMarks(
    campaignId: string,
    characterId: string,
    revision: number,
    name: string,
    sheet: FullSheet,
    marks: readonly WeaponMarkVm[],
  ): Promise<CharacterSheetVm> {
    const next = clone(FullSheetSchema, sheet);
    next.weaponMarks = marks.map((m) => create(WeaponMarkSchema, m));
    const res = await this.client.updateCharacter({
      campaignId,
      characterId,
      revision,
      name,
      sheet: { content: { case: 'full', value: next } },
    });
    return toCharacterSheetVm(res.character!);
  }

  async approveCharacter(campaignId: string, characterId: string): Promise<CharacterSheetVm> {
    const res = await this.client.approveCharacter({ campaignId, characterId });
    return toCharacterSheetVm(res.character!);
  }

  async rejectCharacter(campaignId: string, characterId: string): Promise<void> {
    await this.client.rejectCharacter({ campaignId, characterId });
  }

  async requestCharacterChanges(
    campaignId: string,
    characterId: string,
    reason: string,
    idempotencyKey: string,
  ): Promise<CharacterSheetVm> {
    const res = await this.client.requestCharacterChanges({
      campaignId,
      characterId,
      reason,
      idempotencyKey,
    });
    return toCharacterSheetVm(res.character!);
  }

  async reviveCharacter(
    campaignId: string,
    characterId: string,
    idempotencyKey: string,
  ): Promise<CharacterSheetVm> {
    const res = await this.client.reviveCharacter({ campaignId, characterId, idempotencyKey });
    return toCharacterSheetVm(res.character!);
  }

  async hasLivingCharacter(campaignId: string): Promise<boolean> {
    const res = await this.client.listCharacters({ campaignId });
    return res.characters.some(
      (c) => c.kind === GenCharacterKind.PLAYER && c.state !== GenCharacterState.DEAD,
    );
  }

  async resubmitCharacter(
    campaignId: string,
    characterId: string,
    idempotencyKey: string,
  ): Promise<CharacterSheetVm> {
    const res = await this.client.resubmitCharacter({ campaignId, characterId, idempotencyKey });
    return toCharacterSheetVm(res.character!);
  }

  async getPendingChoiceCount(campaignId: string, characterId: string): Promise<number> {
    try {
      // The stored sheet's choices: what is not done yet is what is pending.
      const res = await this.client.previewChoices({ campaignId, characterId });
      return Math.max(res.total - res.done, 0);
    } catch {
      // A sheet that asks nothing (a basic one) or that the caller cannot read has no banner.
      return 0;
    }
  }
}
