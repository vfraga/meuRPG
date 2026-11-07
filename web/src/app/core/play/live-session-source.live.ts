import { Injectable, inject } from '@angular/core';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { createClient } from '@connectrpc/connect';

import {
  CampaignService,
  DiceMode,
  DicePreference,
  Role,
} from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CharacterService } from '../../../gen/meurpg/characters/v1/characters_pb';
import { GameSession, PlayService } from '../../../gen/meurpg/play/v1/play_pb';
import {
  ContentService,
  type DerivedSheet,
  type DerivedSkill,
  ProficiencyLevel,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import type { CheckFloor } from '../combat/combat-dice';
import { CONNECT_TRANSPORT } from '../connect/transport';
import { REVIVIFY_KEY } from '../revivify/revivify-flow';
import type { OtherSkill, SearchSkills } from '../traps/trap-search';
import { metersText } from '../units';
import { classifyLiveError } from './live-error';
import { toLiveEvent } from './live-event-map';
import { imageAudienceOf, toShownImageVm, toVitalsVm } from './live-vitals-map';
import {
  CampaignInfoVm,
  LiveErrorKind,
  LiveEventVm,
  LiveSessionSource,
  LiveSessionVm,
  LiveSnapshotVm,
  PartyMemberInfoVm,
  PlayerSheetVm,
  ImageAudienceVm,
  ShownImageVm,
  VitalsChange,
  VitalsVm,
} from './live-session.types';

/** "Outra perícia…": the other 16 skills with the sheet's bonus, alphabetical, the same list for everyone (RN-10). */
export function otherSkills(skills: readonly DerivedSkill[]): OtherSkill[] {
  return skills
    .filter((k) => k.key !== 'skill:perception' && k.key !== 'skill:investigation')
    .map((k) => ({ key: k.key, name: k.namePt, bonus: k.bonus }))
    .sort((a, b) => a.name.localeCompare(b.name, 'pt-BR'));
}

/** The skills Talento Confiável raises: the ones with proficiency (or expertise), when the sheet has the feature; none otherwise. */
export function reliableTalentSkills(derived: DerivedSheet): string[] {
  if (!derived.features.some((f) => f.key === 'feature:reliable-talent')) {
    return [];
  }
  return derived.skills
    .filter(
      (k) =>
        k.proficiency === ProficiencyLevel.PROFICIENT ||
        k.proficiency === ProficiencyLevel.EXPERTISE,
    )
    .map((k) => k.key);
}

/** The highest minimum of the table that covers ability checks, with the feature that gives it; `null` without one. A tie keeps the first. */
export function checkFloorOfSheet(derived: DerivedSheet): CheckFloor | null {
  let best: CheckFloor | null = null;
  for (const m of derived.rollMinimums) {
    if (m.applies.includes('check') && (best === null || m.value > best.value)) {
      best = { value: m.value, source: m.featureKey, name: m.featureName };
    }
  }
  return best;
}

/** The bonuses "Procurar armadilhas" and the checks read from the sheet. */
function searchSkillsOf(derived: DerivedSheet): SearchSkills {
  const skill = (key: string) => derived.skills.find((k) => k.key === key)?.bonus ?? null;
  return {
    perception: skill('skill:perception'),
    investigation: skill('skill:investigation'),
    others: otherSkills(derived.skills),
    reliableTalent: reliableTalentSkills(derived),
    checkFloor: checkFloorOfSheet(derived),
  };
}

/** What the live session reads of the player's own sheet (`GetCharacter`): the vitals' header, the skills and the table dice. */
function playerSheetOf(derived: DerivedSheet | undefined): PlayerSheetVm {
  const classes = derived?.classes.map((c) => `${c.namePt} ${c.level}`).join(' / ') ?? '';
  // The subrace's name already says the race ("Gnomo das Rochas"), as on the sheet's header.
  const race = derived?.subraceNamePt || derived?.raceNamePt || '';
  return {
    armorClass: derived ? derived.armorClass : null,
    summary: [classes, race].filter(Boolean).join(', '),
    classes,
    revivify: derived?.spells.some((s) => s.spell?.key === REVIVIFY_KEY && s.prepared) ?? false,
    skills: derived && searchSkillsOf(derived),
    senses: derived?.senses.map((s) => `${s.namePt}: ${metersText(s.rangeFt)}`) ?? [],
    rollDice: derived?.rollDice ?? [],
  };
}

function toSessionVm(gs: GameSession | undefined): LiveSessionVm {
  return {
    sessionId: gs?.id ?? '',
    sessionNumber: gs?.sessionNumber ?? 0,
    startedAt: gs?.startedAt ? timestampDate(gs.startedAt) : new Date(0),
  };
}

export { classifyLiveError, toShownImageVm, toVitalsVm };

/**
 * `LiveSessionSource` over the generated clients. Provided at the route
 * level for `/campaigns/:id/session` (`live-session.routes.ts`), so
 * `play.v1`, `characters.v1` and `rules.v1`'s generated code stays in this
 * page's lazy chunk.
 */
@Injectable()
export class LiveSessionSourceLive implements LiveSessionSource {
  private readonly transport = inject(CONNECT_TRANSPORT);
  private readonly play = createClient(PlayService, this.transport);
  private readonly characters = createClient(CharacterService, this.transport);
  private readonly campaigns = createClient(CampaignService, this.transport);
  private readonly content = createClient(ContentService, this.transport);

  async getCampaign(campaignId: string): Promise<CampaignInfoVm> {
    const res = await this.campaigns.getCampaign({ campaignId });
    return {
      name: res.campaign?.name ?? '',
      isMaster: res.campaign?.myRole === Role.MASTER,
      awaitingApproval: res.campaign?.awaitingApproval ?? false,
      diceMode: res.campaign?.diceMode ?? DiceMode.PLAYERS_CHOOSE,
      dicePreference: res.campaign?.myDicePreference ?? DicePreference.APP,
    };
  }

  async *watch(campaignId: string, signal: AbortSignal): AsyncIterable<LiveEventVm> {
    for await (const res of this.play.watchGameSession({ campaignId }, { signal })) {
      const event = toLiveEvent(res.event);
      if (event) {
        yield event;
      }
    }
  }

  async getLiveSession(campaignId: string): Promise<LiveSnapshotVm> {
    const res = await this.play.getLiveSession({ campaignId });
    return {
      session: toSessionVm(res.gameSession),
      vitals: res.vitals.map(toVitalsVm),
      currentMapId: res.currentMapId || null,
      shownImage: toShownImageVm(res.shownImage),
      shownImageKeep: res.shownImageKeep,
    };
  }

  async adjustVitals(
    campaignId: string,
    characterId: string,
    idempotencyKey: string,
    change: VitalsChange,
    expectedRevision: number,
  ): Promise<VitalsVm> {
    const res = await this.play.adjustCharacterVitals({
      campaignId,
      characterId,
      idempotencyKey,
      expectedRevision,
      hitPointsCurrent: change.hitPointsCurrent,
      hitPointsTemporary: change.hitPointsTemporary,
      spellSlotsUsed: (change.spellSlotsUsed ?? []).map((s) => ({ level: s.level, used: s.used })),
      pactSlotsUsed: change.pactSlotsUsed,
      hitDiceUsedByDie: change.hitDiceUsedByDie ?? {},
      resourcesUsed: (change.resourcesUsed ?? []).map((r) => ({ key: r.key, used: r.used })),
      wildShapeHitPointsCurrent: change.wildShapeHitPointsCurrent,
    });
    return toVitalsVm(res.vitals!);
  }

  async endSession(campaignId: string, sessionId: string): Promise<void> {
    await this.play.endGameSession({ campaignId, gameSessionId: sessionId });
  }

  async setCurrentMap(campaignId: string, mapId: string | null): Promise<string | null> {
    const res = await this.play.setCurrentMap({ campaignId, mapId: mapId ?? '' });
    return res.currentMapId || null;
  }

  async setShownImage(
    campaignId: string,
    imageId: string | null,
    keep = false,
    audience?: ImageAudienceVm,
  ): Promise<ShownImageVm | null> {
    const res = await this.play.setShownImage({
      campaignId,
      imageId: imageId ?? '',
      keep,
      ...(audience ? { audience: imageAudienceOf(audience) } : {}),
    });
    return toShownImageVm(res.shownImage);
  }

  async setShownImageAudience(
    campaignId: string,
    audience: ImageAudienceVm,
  ): Promise<ShownImageVm | null> {
    const res = await this.play.setShownImageAudience({
      campaignId,
      audience: imageAudienceOf(audience),
    });
    return toShownImageVm(res.shownImage);
  }

  async listLeftImages(campaignId: string): Promise<readonly ShownImageVm[]> {
    const res = await this.play.listLeftImages({ campaignId });
    return res.images.map((image) => toShownImageVm(image)).filter((image) => image !== null);
  }

  async takeBackLeftImage(campaignId: string, imageId: string): Promise<void> {
    await this.play.takeBackLeftImage({ campaignId, imageId });
  }

  async getPlayerSheet(campaignId: string, characterId: string): Promise<PlayerSheetVm> {
    const res = await this.characters.getCharacter({ campaignId, characterId });
    return playerSheetOf(res.character?.derived);
  }

  async getCreatureArmorClass(campaignId: string, key: string): Promise<number | null> {
    const res = await this.content.getCreature({ campaignId, key });
    return res.creature?.armorClass ?? null;
  }

  async getPartyInfo(campaignId: string): Promise<ReadonlyMap<string, PartyMemberInfoVm>> {
    const res = await this.characters.listCharacters({ campaignId });
    return new Map(
      res.characters.map((c) => [
        c.id,
        { classSummary: c.classSummary, playerName: c.playerDisplayName.trim() || null },
      ]),
    );
  }

  classifyError(err: unknown): LiveErrorKind {
    return classifyLiveError(err);
  }
}
