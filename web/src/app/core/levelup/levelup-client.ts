import { Injectable, inject } from '@angular/core';
import type { MessageInitShape } from '@bufbuild/protobuf';
import { createClient } from '@connectrpc/connect';

import {
  CampaignService,
  DicePreference,
  XpMode,
} from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CharacterService,
  type Character,
  type LevelUp,
  type LevelUpChoicesSchema,
  type LevelUpOptions,
  type PreviewLevelUpResponse,
  type RollLevelUpHitPointsResponse,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { ContentService, type Skill, type Spell } from '../../../gen/meurpg/rules/v1/rules_pb';
import { spellDetailsFromGen } from '../../shared/spell-details/spell-details-map';
import type { SpellDetailsVm } from '../../shared/spell-details/spell-details.types';
import { CONNECT_TRANSPORT } from '../connect/transport';

/** What the player chose so far, in the wire's own shape. */
export type LevelUpChoicesInit = MessageInitShape<typeof LevelUpChoicesSchema>;

/** The spells and skills of the campaign's rules, for the pickers. */
export interface LevelUpCatalog {
  /** The campaign's content revision the catalog was read at (0 while the table has none of its own). */
  readonly revision?: number;
  readonly spells: readonly Spell[];
  readonly skills: readonly Skill[];
  /** The classes' names, for "da lista de Mago" when a table class or a third caster reads another class's list. */
  readonly classes?: readonly { readonly key: string; readonly namePt: string }[];
}

/** One page of the master's "O que mudou". */
export interface LevelUpPage {
  readonly levelUps: readonly LevelUp[];
  readonly nextPageToken: string;
}

/**
 * Thin wrapper around the guided level-up's calls (MR-040) on the generated
 * `CharacterService`, plus what the pickers need from `ContentService` and the
 * campaign. `providedIn: 'root'`, and only lazy code imports it, so the
 * generated clients stay out of the initial bundle. Callers map errors to
 * Portuguese with `levelup-errors.ts`; nothing here knows a rule: the server
 * derives every number the screen shows (ADR-0008).
 */
@Injectable({ providedIn: 'root' })
export class LevelUpClient {
  private readonly characters = createClient(CharacterService, inject(CONNECT_TRANSPORT));
  private readonly content = createClient(ContentService, inject(CONNECT_TRANSPORT));
  private readonly campaigns = createClient(CampaignService, inject(CONNECT_TRANSPORT));

  /** The rules content is the same for every call of a page: read once. */
  private readonly catalogs = new Map<string, Promise<LevelUpCatalog>>();
  private readonly details = new Map<string, Promise<SpellDetailsVm>>();

  async character(campaignId: string, characterId: string): Promise<Character> {
    const res = await this.characters.getCharacter({ campaignId, characterId });
    return res.character!;
  }

  async options(campaignId: string, characterId: string): Promise<LevelUpOptions> {
    const res = await this.characters.getLevelUpOptions({ campaignId, characterId });
    return res.options!;
  }

  preview(
    campaignId: string,
    characterId: string,
    choices: LevelUpChoicesInit,
  ): Promise<PreviewLevelUpResponse> {
    return this.characters.previewLevelUp({ campaignId, characterId, choices });
  }

  rollHitPoints(
    campaignId: string,
    characterId: string,
    classKey: string,
    idempotencyKey: string,
  ): Promise<RollLevelUpHitPointsResponse> {
    return this.characters.rollLevelUpHitPoints({
      campaignId,
      characterId,
      classKey,
      idempotencyKey,
    });
  }

  async levelUp(
    campaignId: string,
    characterId: string,
    revision: number,
    choices: LevelUpChoicesInit,
  ): Promise<Character> {
    const res = await this.characters.levelUpCharacter({
      campaignId,
      characterId,
      revision,
      choices,
    });
    return res.character!;
  }

  /** The master's "O que mudou", newest first. */
  async list(campaignId: string, pageSize = 50, pageToken = ''): Promise<LevelUpPage> {
    const res = await this.characters.listLevelUps({ campaignId, pageSize, pageToken });
    return { levelUps: res.levelUps, nextPageToken: res.nextPageToken };
  }

  /**
   * The campaign's content for the pickers, read with the character, so the entries the sheet already has come
   * back even when the master retired them since. The cache lives as long as this client: the level-up page makes its
   * own (it is provided there, not in the root), so a new page always reads the table's content as it is now, and
   * `fresh` reads it again inside the same page (after a `content_changed` hint or a stale sheet).
   */
  catalog(campaignId: string, characterId = '', fresh = false): Promise<LevelUpCatalog> {
    const id = `${campaignId}/${characterId}`;
    let pending = fresh ? undefined : this.catalogs.get(id);
    if (!pending) {
      pending = this.content.listContent({ campaignId, characterId }).then((res) => ({
        revision: res.tableRevision,
        spells: res.content?.spells ?? [],
        skills: res.content?.skills ?? [],
        classes: (res.content?.classes ?? []).map((c) => ({ key: c.key, namePt: c.namePt })),
      }));
      this.catalogs.set(id, pending);
      pending.catch(() => this.catalogs.delete(id));
    }
    return pending;
  }

  /** The "?" of a spell: its details, kept per SRD spell so a second open is instant. A spell of the table (`@mesa`) is read each
   * time: the master can change it while the page is open. */
  spellDetails(campaignId: string, spellKey: string): Promise<SpellDetailsVm> {
    const id = `${campaignId}/${spellKey}`;
    const kept = !spellKey.endsWith('@mesa');
    let pending = kept ? this.details.get(id) : undefined;
    if (!pending) {
      pending = this.content
        .getSpellDetails({ campaignId, spellKey })
        .then((res) => spellDetailsFromGen(res.spell!));
      if (kept) {
        this.details.set(id, pending);
        pending.catch(() => this.details.delete(id));
      }
    }
    return pending;
  }

  /** How the campaign levels (RN-09): the blocked page says what is missing by it. */
  async xpMode(campaignId: string): Promise<XpMode> {
    const res = await this.campaigns.getCampaign({ campaignId });
    return res.campaign?.xpMode ?? XpMode.UNSPECIFIED;
  }

  /** The caller's own dice choice (RN-18): which way of rolling opens first. */
  async dicePreference(campaignId: string): Promise<DicePreference> {
    const res = await this.campaigns.getCampaign({ campaignId });
    return res.campaign?.myDicePreference ?? DicePreference.UNSPECIFIED;
  }
}
