import { Injectable, inject } from '@angular/core';
import { createClient } from '@connectrpc/connect';

import { MonsterHitPoints } from '../../../gen/meurpg/play/v1/combat_pb';
import {
  type BattleEncounter,
  type BattleEncounterSummary,
  EncounterBand,
  EncounterService,
  type EncounterEvaluation,
} from '../../../gen/meurpg/play/v1/encounters_pb';
import type { CreatureSummary } from '../../../gen/meurpg/rules/v1/rules_pb';
import type { MonsterGroupSpec, MonsterHp } from '../combat/combat-client-types';
import { CONNECT_TRANSPORT } from '../connect/transport';

/** An NPC the master puts in the party for one encounter (question 86): one of the campaign's NPCs, or only a name, and a level. */
export interface PartyNpcSpec {
  /** An NPC of the campaign; empty for a name alone. */
  readonly characterId: string;
  /** What to call it; empty only with a `characterId` (the server then takes the NPC's own name). */
  readonly name: string;
  readonly level: number;
}

/** The band "Gerar encontro" fills: the guide's three (never "above high"). */
export type GenerateBand = 'low' | 'moderate' | 'high';

/** A saved encounter, as the session and the builder read it. */
export interface SavedEncounter {
  readonly monsters: readonly MonsterGroupSpec[];
  readonly hp: MonsterHp;
  readonly hidden: boolean;
}

/** What `GetBattleEncounter` answers: nothing, or the encounter and how it measures against the party of today. */
export interface BattleRead {
  readonly encounter: SavedEncounter | null;
  readonly evaluation: EncounterEvaluation | null;
  /** The saved creatures that are no longer in the SRD (left out of the evaluation). */
  readonly unknownKeys: readonly string[];
}

const BANDS: Record<GenerateBand, EncounterBand> = {
  low: EncounterBand.LOW,
  moderate: EncounterBand.MODERATE,
  high: EncounterBand.HIGH,
};

/**
 * Thin wrapper around `EncounterService` (MR-043, RN-29): the master's encounter builder. The server does every
 * sum (the budgets, the total, the band, the cap); this class only sends what the master put together and hands
 * back the answers. `providedIn: 'root'` and only lazy code imports it, so the generated code stays out of the
 * initial bundle. Callers map errors to Portuguese with `encounter-errors.ts`.
 */
@Injectable({ providedIn: 'root' })
export class EncountersClient {
  private readonly client = createClient(EncounterService, inject(CONNECT_TRANSPORT));

  async evaluate(
    campaignId: string,
    entries: readonly MonsterGroupSpec[],
    extraParty: readonly PartyNpcSpec[],
  ): Promise<EncounterEvaluation> {
    const res = await this.client.evaluateEncounter({
      campaignId,
      entries: entries.map(toGroup),
      extraParty: extraParty.map(toNpc),
    });
    return need(res.evaluation, 'EvaluateEncounter');
  }

  /** "Gerar encontro": seed 0 asks the server to draw one, and the answer says which it was. */
  async generate(
    campaignId: string,
    band: GenerateBand,
    creatureType: string,
    seed: number,
    extraParty: readonly PartyNpcSpec[],
  ): Promise<{ readonly evaluation: EncounterEvaluation; readonly seed: number }> {
    const res = await this.client.generateEncounter({
      campaignId,
      band: BANDS[band],
      creatureType,
      seed,
      extraParty: extraParty.map(toNpc),
    });
    return { evaluation: need(res.evaluation, 'GenerateEncounter'), seed: res.seed };
  }

  /** "Trocar criatura": the creatures with the same XP (the same type when one was chosen). */
  async swaps(
    campaignId: string,
    creatureKey: string,
    creatureType: string,
  ): Promise<readonly CreatureSummary[]> {
    return (await this.client.listEncounterSwaps({ campaignId, creatureKey, creatureType }))
      .creatures;
  }

  async save(
    campaignId: string,
    mapPointId: string,
    encounter: SavedEncounter,
    extraParty: readonly PartyNpcSpec[],
  ): Promise<{
    readonly evaluation: EncounterEvaluation | undefined;
    readonly updatedAt: Date | undefined;
  }> {
    const res = await this.client.saveBattleEncounter({
      campaignId,
      mapPointId,
      encounter: toBattle(encounter),
      extraParty: extraParty.map(toNpc),
    });
    return {
      evaluation: res.evaluation,
      updatedAt: res.updatedAt ? new Date(Number(res.updatedAt.seconds) * 1000) : undefined,
    };
  }

  async get(
    campaignId: string,
    mapPointId: string,
    extraParty: readonly PartyNpcSpec[] = [],
  ): Promise<BattleRead> {
    const res = await this.client.getBattleEncounter({
      campaignId,
      mapPointId,
      extraParty: extraParty.map(toNpc),
    });
    return {
      encounter: res.encounter ? fromBattle(res.encounter) : null,
      evaluation: res.evaluation ?? null,
      unknownKeys: res.unknownCreatureKeys,
    };
  }

  async clear(campaignId: string, mapPointId: string): Promise<void> {
    await this.client.clearBattleEncounter({ campaignId, mapPointId });
  }

  /** The battle points of a map that keep an encounter ("Já guarda um encontro"). */
  async list(campaignId: string, mapId: string): Promise<readonly BattleEncounterSummary[]> {
    return (await this.client.listBattleEncounters({ campaignId, mapId })).encounters;
  }
}

function toGroup(m: MonsterGroupSpec): { creatureKey: string; count: number; name: string } {
  return { creatureKey: m.creatureKey, count: m.count, name: m.name ?? '' };
}

function toNpc(n: PartyNpcSpec): { characterId: string; name: string; level: number } {
  return { characterId: n.characterId, name: n.name, level: n.level };
}

function toBattle(e: SavedEncounter): {
  monsters: ReturnType<typeof toGroup>[];
  hitPoints: MonsterHitPoints;
  hidden: boolean;
} {
  return {
    monsters: e.monsters.map(toGroup),
    hitPoints: e.hp === 'rolled' ? MonsterHitPoints.ROLLED : MonsterHitPoints.AVERAGE,
    hidden: e.hidden,
  };
}

function fromBattle(b: BattleEncounter): SavedEncounter {
  return {
    monsters: b.monsters.map((m) => ({
      creatureKey: m.creatureKey,
      count: m.count || 1,
      name: m.name,
    })),
    // Unset is the average, and unset hidden is hidden (encounters.proto).
    hp: b.hitPoints === MonsterHitPoints.ROLLED ? 'rolled' : 'average',
    hidden: b.hidden ?? true,
  };
}

function need<T>(value: T | undefined, call: string): T {
  if (value === undefined) {
    throw new Error(`${call} answered without its result`);
  }
  return value;
}
