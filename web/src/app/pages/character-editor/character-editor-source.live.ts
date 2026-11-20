import { Injectable, inject } from '@angular/core';
import { Code, ConnectError, createClient } from '@connectrpc/connect';

import {
  CampaignService,
  DicePreference,
  Role,
} from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  AbilityMethod as GenAbilityMethod,
  CharacterKind as GenCharacterKind,
  CharacterService,
  CharacterState as GenCharacterState,
  FullSheet as GenFullSheet,
  BasicSheet as GenBasicSheet,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { ContentService } from '../../../gen/meurpg/rules/v1/rules_pb';
import { KIND_FROM_GEN } from '../../core/characters/character-kind-gen';
import { CharacterKind } from '../../core/characters/characters.types';
import {
  TableContentKind,
  TableContentService,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { CONNECT_TRANSPORT } from '../../core/connect/transport';
import { spellDetailsFromGen } from '../../shared/spell-details/spell-details-map';
import type { SpellDetailsVm } from '../../shared/spell-details/spell-details.types';
import { effectivePreference } from '../../core/campaigns/dice-labels';
import {
  AbilityMethodKey,
  AbilityRollsVm,
  AbilityTableVm,
  CharacterEditorSource,
  CharacterForEdit,
  CharacterPreviewVm,
  ChoicesPreviewVm,
  CreateCharacterInput,
  FeatCatalogVm,
  PreviewCharacterInput,
  RulesCatalogVm,
  UpdateCharacterInput,
} from './character-editor.types';
import {
  abilityOriginFromGen,
  abilityTableFromGen,
  catalogFromGen,
  featIssuesOf,
  grantedSpellKeys,
  rollsFromGen,
} from './character-editor-catalog-map';
import {
  mergeFullSheetInit,
  toBasicSheetInit,
  toFormBasicSheet,
  toFormFullSheet,
  toFullSheetInit,
} from './character-editor-sheet-map';

const KIND_TO_GEN: Record<CharacterKind, GenCharacterKind> = {
  player: GenCharacterKind.PLAYER,
  enemy: GenCharacterKind.ENEMY,
  boss: GenCharacterKind.BOSS,
  minion: GenCharacterKind.MINION,
  story: GenCharacterKind.STORY,
};

const ABILITY_METHOD_TO_GEN: Record<AbilityMethodKey, GenAbilityMethod> = {
  standard_array: GenAbilityMethod.STANDARD_ARRAY,
  point_buy: GenAbilityMethod.POINT_BUY,
  rolled_4d6: GenAbilityMethod.ROLLED_4D6,
  typed: GenAbilityMethod.TYPED,
};

/**
 * `CharacterEditorSource` over the generated `ContentService`
 * (`meurpg.rules.v1`, for the catalog) and `CharacterService`
 * (`meurpg.characters.v1`, for create/read/update) clients, phase 2.
 * Provided at the route level for the three editor routes — see
 * `character-editor.routes.ts` — so these clients stay out of the eager
 * bundle.
 */
@Injectable()
export class CharacterEditorSourceLive implements CharacterEditorSource {
  private readonly characterClient = createClient(CharacterService, inject(CONNECT_TRANSPORT));
  private readonly contentClient = createClient(ContentService, inject(CONNECT_TRANSPORT));
  private readonly campaignClient = createClient(CampaignService, inject(CONNECT_TRANSPORT));
  private readonly tableClient = createClient(TableContentService, inject(CONNECT_TRANSPORT));

  /** The last `FullSheet` `loadCharacterForEdit` read for a character, kept
   * only so `updateCharacter` can start from it — see
   * `mergeFullSheetInit`'s doc comment. Never read for anything else; a
   * fresh `CharacterEditorSourceLive` per route (see
   * `character-editor.routes.ts`) means this never outlives the page. */
  private readonly loadedFullSheets = new Map<string, GenFullSheet>();

  async loadCatalog(campaignId: string, characterId?: string): Promise<RulesCatalogVm> {
    // With the character, the entries the sheet already has come back even when the master retired them, marked.
    const [res, campaignRead] = await Promise.all([
      this.contentClient.listContent({ campaignId, characterId: characterId ?? '' }),
      this.campaignClient
        .getCampaign({ campaignId })
        .then((r) => ({
          viewerIsMaster: r.campaign?.myRole === Role.MASTER,
          xpMode: r.campaign?.xpMode,
        }))
        .catch((err: unknown) => {
          // Only a refusal means "not the master"; a blip must not hide the master's view, so it fails the load.
          if (
            err instanceof ConnectError &&
            (err.code === Code.PermissionDenied || err.code === Code.NotFound)
          ) {
            return { viewerIsMaster: false, xpMode: undefined };
          }
          throw err;
        }),
    ]);
    const feats = campaignRead.viewerIsMaster ? await this.loadFeats(campaignId) : undefined;
    return catalogFromGen(res.content!, campaignRead, feats);
  }

  /** The feats the master may give a sheet and whether the table uses them; `undefined` when either read fails (the section says so). */
  private async loadFeats(campaignId: string): Promise<FeatCatalogVm | undefined> {
    try {
      const [switches, rules] = await Promise.all([
        this.tableClient.listOptionSwitches({ campaignId }),
        this.campaignClient.getTableRules({ campaignId }),
      ]);
      return {
        options: switches.options
          .filter((o) => o.kind === TableContentKind.FEAT && !o.archived)
          .map((o) => ({ key: o.key, namePt: o.namePt, fromTable: o.table, off: o.off })),
        featsAllowed: rules.rules?.featsAllowed ?? false,
      };
    } catch {
      return undefined;
    }
  }

  async loadSpellDetails(campaignId: string, spellKey: string): Promise<SpellDetailsVm> {
    const res = await this.contentClient.getSpellDetails({ campaignId, spellKey });
    return spellDetailsFromGen(res.spell!);
  }

  async loadCharacterForEdit(campaignId: string, characterId: string): Promise<CharacterForEdit> {
    const res = await this.characterClient.getCharacter({ campaignId, characterId });
    const character = res.character!;
    const kind = KIND_FROM_GEN[character.kind];
    const sheetCase = character.sheet?.content.case;

    let full = null;
    if (sheetCase === 'full') {
      const loaded = character.sheet!.content.value as GenFullSheet;
      this.loadedFullSheets.set(characterId, loaded);
      full = toFormFullSheet(character.name, loaded);
    }

    return {
      kind,
      revision: character.revision,
      // The server says whether this caller may save the sheet now: a
      // player's sheet locks at the first session (RN-01), and a dead
      // character's never changes again (RN-03).
      blocked: character.canEdit
        ? null
        : character.state === GenCharacterState.DEAD
          ? 'character_dead'
          : 'sheet_locked',
      sheetLocked:
        character.state === GenCharacterState.LOCKED || character.state === GenCharacterState.DEAD,
      reserved: character.reserved,
      preparedMax: Object.fromEntries(
        (character.derived?.spellcasting ?? [])
          .filter((c) => c.preparedMax > 0)
          .map((c) => [c.classKey, c.preparedMax]),
      ),
      grantedSpellKeys: grantedSpellKeys(
        character.derived,
        sheetCase === 'full' ? (character.sheet!.content.value as GenFullSheet) : undefined,
      ),
      featureKeys: (character.derived?.features ?? []).map((f) => f.key),
      abilityOrigin: abilityOriginFromGen(
        sheetCase === 'full'
          ? (character.sheet!.content.value as GenFullSheet).abilityOrigin
          : undefined,
      ),
      full,
      basic:
        sheetCase === 'basic'
          ? toFormBasicSheet(character.name, character.sheet!.content.value as GenBasicSheet)
          : null,
    };
  }

  async loadAbilityTable(campaignId: string): Promise<AbilityTableVm | null> {
    const [campaign, rules] = await Promise.all([
      this.campaignClient.getCampaign({ campaignId }),
      this.campaignClient.getTableRules({ campaignId }),
    ]);
    // The master's NPCs are free: only a player (or a pending member) makes a sheet the rules check.
    if (!campaign.campaign || campaign.campaign.myRole === Role.MASTER) {
      return null;
    }
    const physical =
      effectivePreference(campaign.campaign.diceMode, campaign.campaign.myDicePreference) ===
      DicePreference.PHYSICAL;
    // Only a method that rolls needs the stored sets, and the master's call is refused: ask when it is allowed.
    const stored = rules.rules?.abilityMethods?.rolled4d6
      ? (await this.characterClient.getAbilityRolls({ campaignId })).rolls
      : undefined;
    return abilityTableFromGen(campaign.campaign, rules, physical, stored);
  }

  async rollAbilityScores(
    campaignId: string,
    typedDice: readonly (readonly number[])[] = [],
  ): Promise<AbilityRollsVm> {
    const res = await this.characterClient.rollAbilityScores({
      campaignId,
      typedDice: typedDice.map((dice) => ({ dice: [...dice] })),
    });
    return rollsFromGen(res.rolls!);
  }

  async previewCharacter(input: PreviewCharacterInput): Promise<CharacterPreviewVm> {
    const res = await this.characterClient.previewCharacter({
      campaignId: input.campaignId,
      characterId: input.characterId ?? '',
      kind: KIND_TO_GEN[input.kind],
      ...(input.forPlayer ? { forPlayer: true } : {}),
      sheet: {
        content: {
          case: 'full',
          value: mergeFullSheetInit(
            input.characterId ? this.loadedFullSheets.get(input.characterId) : undefined,
            input.full,
          ),
        },
      },
    });
    return {
      hitPointsMax: res.derived?.hitPointsMax ?? 0,
      hitPointsFromEffects: res.derived?.hitPointsFromEffects ?? 0,
      spellcasting: (res.derived?.spellcasting ?? []).map((sc) => ({
        classKey: sc.classKey,
        cantripsKnown: sc.cantripsKnown,
        spellsKnown: sc.spellsKnown,
        preparedMax: sc.preparedMax,
      })),
      featIssues: featIssuesOf(res.derived?.issues ?? [], input.full.featKeys ?? []),
    };
  }

  async previewChoices(input: PreviewCharacterInput): Promise<ChoicesPreviewVm> {
    const res = await this.characterClient.previewChoices({
      campaignId: input.campaignId,
      characterId: input.characterId ?? '',
      kind: KIND_TO_GEN[input.kind],
      sheet: {
        content: {
          case: 'full',
          value: mergeFullSheetInit(
            input.characterId ? this.loadedFullSheets.get(input.characterId) : undefined,
            input.full,
          ),
        },
      },
    });
    return {
      groups: res.groups,
      done: res.done,
      total: res.total,
      notOffered: res.notOffered,
      spellSources: res.spellSources,
      skillsAllowed: res.skillsAllowed ?? null,
      expertiseAllowed: res.expertiseAllowed ?? null,
    };
  }

  async createCharacter(input: CreateCharacterInput): Promise<{ characterId: string }> {
    const res = await this.characterClient.createCharacter({
      campaignId: input.campaignId,
      kind: KIND_TO_GEN[input.kind],
      name: input.full?.name ?? input.basic?.name ?? '',
      idempotencyKey: input.idempotencyKey,
      ...(input.forPlayer ? { forPlayer: true } : {}),
      ...(input.abilityMethod ? { abilityMethod: ABILITY_METHOD_TO_GEN[input.abilityMethod] } : {}),
      sheet: input.full
        ? { content: { case: 'full', value: toFullSheetInit(input.full) } }
        : { content: { case: 'basic', value: toBasicSheetInit(input.basic!) } },
    });
    return { characterId: res.character!.id };
  }

  async updateCharacter(input: UpdateCharacterInput): Promise<{ revision: number }> {
    const res = await this.characterClient.updateCharacter({
      campaignId: input.campaignId,
      characterId: input.characterId,
      revision: input.revision,
      name: input.name,
      sheet: input.full
        ? {
            content: {
              case: 'full',
              value: mergeFullSheetInit(this.loadedFullSheets.get(input.characterId), input.full),
            },
          }
        : { content: { case: 'basic', value: toBasicSheetInit(input.basic!) } },
    });
    // Keep the cache current: the next save (without reloading in between)
    // starts from what the server just confirmed, not the stale pre-edit
    // version.
    const savedCase = res.character?.sheet?.content.case;
    if (savedCase === 'full') {
      this.loadedFullSheets.set(
        input.characterId,
        res.character!.sheet!.content.value as GenFullSheet,
      );
    }
    return { revision: res.character!.revision };
  }
}
