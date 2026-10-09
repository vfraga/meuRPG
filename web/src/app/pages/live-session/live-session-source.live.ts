import { Injectable, inject } from '@angular/core';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { Code, ConnectError, createClient } from '@connectrpc/connect';

import {
  CampaignService,
  DiceMode,
  DicePreference,
  Role,
} from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CharacterService } from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  CharacterVitals,
  GameSession,
  GameSessionBlockedReason,
  GameSessionBlockedSchema,
  PlayService,
  ShownImage,
} from '../../../gen/meurpg/play/v1/play_pb';
import { ContentService, Recharge } from '../../../gen/meurpg/rules/v1/rules_pb';
import { CONNECT_TRANSPORT } from '../../core/connect/transport';
import { metersText } from '../../core/units';
import {
  CampaignInfoVm,
  LiveErrorKind,
  LiveEventVm,
  LiveSessionSource,
  LiveSessionVm,
  LiveSnapshotVm,
  PartyMemberInfoVm,
  PlayerSheetVm,
  ShownImageVm,
  VitalsChange,
  VitalsVm,
} from './live-session.types';

const RECHARGE: Partial<Record<Recharge, 'short_rest' | 'long_rest' | 'dawn' | 'none'>> = {
  [Recharge.SHORT_REST]: 'short_rest',
  [Recharge.LONG_REST]: 'long_rest',
  [Recharge.DAWN]: 'dawn',
};

export function toVitalsVm(v: CharacterVitals): VitalsVm {
  return {
    characterId: v.characterId,
    name: v.name,
    playerUserId: v.playerUserId,
    hitPointsCurrent: v.hitPointsCurrent,
    hitPointsMax: v.hitPointsMax,
    hitPointsTemporary: v.hitPointsTemporary,
    spellSlots: v.spellSlots.map((s) => ({ level: s.level, total: s.total, used: s.used })),
    pactSlots: v.pactSlots
      ? { slotLevel: v.pactSlots.slotLevel, total: v.pactSlots.total, used: v.pactSlots.used }
      : null,
    hitDice: v.hitDice.map((hd) => `${hd.count}d${hd.faces}`).join(' + '),
    hitDiceTotal: v.hitDiceTotal,
    hitDiceUsed: v.hitDiceUsed,
    revision: v.revision,
    familiarSight: v.familiarSight
      ? { creatureId: v.familiarSight.creatureId, inCombat: v.familiarSight.inCombat }
      : null,
    resources: v.resources.map((r) => ({
      key: r.key,
      namePt: r.namePt,
      total: r.total,
      used: r.used,
      recharge: RECHARGE[r.recharge] ?? 'none',
    })),
    wildShape: v.wildShape
      ? {
          beastKey: v.wildShape.beastKey,
          beastNamePt: v.wildShape.beastNamePt,
          hitPointsCurrent: v.wildShape.hitPointsCurrent,
          hitPointsMax: v.wildShape.hitPointsMax,
        }
      : null,
  };
}

export function toShownImageVm(image: ShownImage | undefined): ShownImageVm | null {
  return image && image.id
    ? { id: image.id, name: image.name, width: image.width, height: image.height, url: image.url }
    : null;
}

function toSessionVm(gs: GameSession | undefined): LiveSessionVm {
  return {
    sessionId: gs?.id ?? '',
    sessionNumber: gs?.sessionNumber ?? 0,
    startedAt: gs?.startedAt ? timestampDate(gs.startedAt) : new Date(0),
  };
}

/**
 * Maps a Connect error to what it means for the page, by code and by the
 * `GameSessionBlocked` detail, never by the message (play.proto lists which
 * method returns what).
 */
export function classifyLiveError(err: unknown): LiveErrorKind {
  const connectErr = ConnectError.from(err, Code.Unavailable);
  switch (connectErr.code) {
    case Code.NotFound:
      return 'no-access';
    case Code.Unauthenticated:
      return 'signed-out';
    case Code.InvalidArgument:
      return 'invalid';
    case Code.PermissionDenied:
      return 'forbidden';
    case Code.FailedPrecondition: {
      const [detail] = connectErr.findDetails(GameSessionBlockedSchema);
      return detail?.reason === GameSessionBlockedReason.NO_OPEN_SESSION
        ? 'no-session'
        : 'transient';
    }
    default:
      return 'transient';
  }
}

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
      switch (res.event.case) {
        case 'ready':
          yield { kind: 'ready' };
          break;
        case 'heartbeat':
          yield { kind: 'heartbeat' };
          break;
        case 'vitalsChanged':
          if (res.event.value.vitals) {
            yield { kind: 'vitals', vitals: toVitalsVm(res.event.value.vitals) };
          }
          break;
        case 'sessionEnded':
          yield { kind: 'ended' };
          break;
        case 'currentMapChanged':
          yield { kind: 'currentMap', mapId: res.event.value.mapId || null };
          break;
        case 'mapChanged':
          yield { kind: 'mapChanged', mapId: res.event.value.mapId };
          break;
        case 'tokenMoved':
          yield {
            kind: 'tokenMoved',
            mapId: res.event.value.mapId,
            characterId: res.event.value.characterId,
            xBp: res.event.value.xBp,
            yBp: res.event.value.yBp,
          };
          break;
        case 'visionChanged':
          yield { kind: 'visionChanged', mapId: res.event.value.mapId };
          break;
        case 'shownImageChanged':
          yield { kind: 'shownImage', image: toShownImageVm(res.event.value.image) };
          break;
        case 'leftImagesChanged':
          yield { kind: 'leftImages' };
          break;
        case 'encounterChanged':
          yield {
            kind: 'encounterChanged',
            encounterId: res.event.value.encounterId,
            revision: res.event.value.revision,
            // The combat's mode (RN-25): a combat without a map has no fog to read and no trap to refresh.
            mode: res.event.value.mode,
          };
          break;
        case 'turnChanged':
          yield {
            kind: 'turnChanged',
            encounterId: res.event.value.encounterId,
            round: res.event.value.round,
            currentCombatantId: res.event.value.currentCombatantId,
            masterTurn: res.event.value.masterTurn,
          };
          break;
        case 'combatantMoved':
          yield {
            kind: 'combatantMoved',
            encounterId: res.event.value.encounterId,
            combatantId: res.event.value.combatantId,
            col: res.event.value.col,
            row: res.event.value.row,
          };
          break;
        case 'combatLogChanged':
          yield { kind: 'combatLogChanged' };
          break;
        case 'xpChanged':
          yield { kind: 'xpChanged' };
          break;
        case 'sceneChanged':
          yield { kind: 'sceneChanged' };
          break;
        case 'sceneCheckRolled':
          yield { kind: 'sceneCheckRolled' };
          break;
        case 'notesChanged':
          yield { kind: 'notesChanged' };
          break;
        case 'stageChanged':
          yield { kind: 'stageChanged' };
          break;
        case 'trapNoticed':
          yield {
            kind: 'trapNoticed',
            mapId: res.event.value.mapId,
            pointId: res.event.value.pointId,
          };
          break;
        case 'creaturesChanged':
          yield { kind: 'creaturesChanged' };
          break;
        case 'inventoryChanged':
          yield { kind: 'inventoryChanged', characterId: res.event.value.characterId };
          break;
        case 'contentChanged':
          yield { kind: 'contentChanged' };
          break;
        case 'puzzleChanged':
          yield { kind: 'puzzleChanged', puzzleId: res.event.value.puzzleId };
          break;
        default:
          // A newer server's event this app doesn't know yet: still proof
          // that the stream is alive.
          yield { kind: 'heartbeat' };
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
  ): Promise<VitalsVm> {
    const res = await this.play.adjustCharacterVitals({
      campaignId,
      characterId,
      idempotencyKey,
      hitPointsCurrent: change.hitPointsCurrent,
      hitPointsTemporary: change.hitPointsTemporary,
      spellSlotsUsed: (change.spellSlotsUsed ?? []).map((s) => ({ level: s.level, used: s.used })),
      pactSlotsUsed: change.pactSlotsUsed,
      hitDiceUsed: change.hitDiceUsed,
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
  ): Promise<ShownImageVm | null> {
    const res = await this.play.setShownImage({ campaignId, imageId: imageId ?? '', keep });
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
    const derived = res.character?.derived;
    const classes = derived?.classes.map((c) => `${c.namePt} ${c.level}`).join(' / ') ?? '';
    // The subrace's name already says the race ("Gnomo das Rochas"), as on
    // the sheet's header.
    const race = derived?.subraceNamePt || derived?.raceNamePt || '';
    const skill = (key: string) => derived?.skills.find((k) => k.key === key)?.bonus ?? null;
    return {
      armorClass: derived ? derived.armorClass : null,
      summary: [classes, race].filter(Boolean).join(', '),
      skills: derived
        ? { perception: skill('skill:perception'), investigation: skill('skill:investigation') }
        : undefined,
      senses: derived?.senses.map((s) => `${s.namePt}: ${metersText(s.rangeFt)}`) ?? [],
    };
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
