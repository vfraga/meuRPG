import { Injectable, inject } from '@angular/core';
import { createClient } from '@connectrpc/connect';

import {
  type CharacterCreature,
  type Character,
  CharacterKind,
  CharacterService,
  type GetSummonOptionsResponse,
  type ListWildShapeFormsResponse,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import type { Encounter } from '../../../gen/meurpg/play/v1/combat_pb';
import { type CharacterVitals, PlayService } from '../../../gen/meurpg/play/v1/play_pb';
import {
  type Creature,
  type CreatureSize,
  type CreatureSummary,
  ContentService,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';

/** What "Dar uma criatura" filters the SRD's catalog by. */
export interface CreatureFilter {
  readonly query?: string;
  readonly type?: string;
  readonly maxCr?: string;
  /** The lowest challenge rating (the bestiary's "ND"). */
  readonly minCr?: string;
  /** Only this size; unset for any (the bestiary's "Tamanho"). */
  readonly size?: CreatureSize;
  readonly pageSize?: number;
}

/** "Criar NPC" (`CreateNpcFromCreature`): the roles a basic sheet can have. */
export type NpcRole = 'minion' | 'story';

/** One casting of a summoning spell outside a combat (`PlayService.CastSummon`). */
export interface SummonCast {
  readonly campaignId: string;
  readonly characterId: string;
  readonly spellKey: string;
  readonly ritual: boolean;
  /** The slot of a cast that is not a ritual. */
  readonly slot?: { readonly level: number; readonly pact: boolean };
  /** The feature that gives the summon (`SummonSpellOptions.grantedBy`): the cast spends a use of its resource, not a slot. */
  readonly grantedBy?: string;
  /** The option and the kind of each creature; a name for each, or none. */
  readonly summon: {
    readonly option: number;
    readonly creatureKeys: readonly string[];
    readonly names: readonly string[];
  };
  readonly idempotencyKey: string;
}

/**
 * Thin wrapper around the calls of the character's creatures (MR-037):
 * `CharacterService`'s list, give, rename, dismiss and correction,
 * `ContentService`'s catalog and stat blocks, and `PlayService.CastSummon`.
 * `providedIn: 'root'`, and only lazy code imports it, so the generated code
 * stays out of the initial bundle (like `CombatClient`). The stat blocks are
 * SRD rules, the same for everyone: they are kept in memory once read.
 * Callers map errors to Portuguese with `creature-errors.ts`.
 */
@Injectable({ providedIn: 'root' })
export class CreaturesClient {
  private readonly transport = inject(CONNECT_TRANSPORT);
  private readonly characters = createClient(CharacterService, this.transport);
  private readonly content = createClient(ContentService, this.transport);
  private readonly play = createClient(PlayService, this.transport);
  private readonly statBlocks = new Map<string, Promise<Creature>>();

  /** The character's creatures, oldest first (the owner's player and the master; another player gets `not_found`). */
  async list(campaignId: string, characterId: string): Promise<readonly CharacterCreature[]> {
    return (await this.characters.listCharacterCreatures({ campaignId, characterId })).creatures;
  }

  /** What the character can summon from its sheet: the spells, what each brings at each circle, the slots,
   * what a casting would send away. The server works all of it out (`GetSummonOptions`). */
  async summonOptions(campaignId: string, characterId: string): Promise<GetSummonOptionsResponse> {
    return this.characters.getSummonOptions({ campaignId, characterId });
  }

  /** The beasts the druid's Wild Shape allows now, by Portuguese name, with the limit that decided the list (`ListWildShapeForms`). */
  async wildShapeForms(
    campaignId: string,
    characterId: string,
  ): Promise<ListWildShapeFormsResponse> {
    return this.characters.listWildShapeForms({ campaignId, characterId });
  }

  /** "Virar Lobo": the action and one use (`AssumeWildShape`). The answer is the vitals with the form and, in a combat, the combat. */
  async assumeWildShape(
    campaignId: string,
    characterId: string,
    beastKey: string,
    idempotencyKey: string,
  ): Promise<{
    readonly vitals: CharacterVitals | undefined;
    readonly encounter: Encounter | undefined;
  }> {
    const res = await this.play.assumeWildShape({
      campaignId,
      characterId,
      beastKey,
      idempotencyKey,
    });
    return { vitals: res.vitals, encounter: res.encounter };
  }

  /** "Voltar à forma normal" (`LeaveWildShape`): a bonus action in a combat. */
  async leaveWildShape(
    campaignId: string,
    characterId: string,
    idempotencyKey: string,
  ): Promise<{
    readonly vitals: CharacterVitals | undefined;
    readonly encounter: Encounter | undefined;
  }> {
    const res = await this.play.leaveWildShape({ campaignId, characterId, idempotencyKey });
    return { vitals: res.vitals, encounter: res.encounter };
  }

  /** The master gives a character a creature; a blank name takes the Portuguese name of the kind. */
  async give(
    campaignId: string,
    characterId: string,
    monsterKey: string,
    name: string,
    idempotencyKey: string,
  ): Promise<CharacterCreature | undefined> {
    return (
      await this.characters.giveCreature({
        campaignId,
        characterId,
        monsterKey,
        name,
        idempotencyKey,
      })
    ).creature;
  }

  async rename(
    campaignId: string,
    creatureId: string,
    name: string,
  ): Promise<CharacterCreature | undefined> {
    return (await this.characters.renameCreature({ campaignId, creatureId, name })).creature;
  }

  async dismiss(campaignId: string, creatureId: string): Promise<void> {
    await this.characters.dismissCreature({ campaignId, creatureId });
  }

  /** The master's correction outside a combat: the hit points themselves. */
  async setHitPoints(
    campaignId: string,
    creatureId: string,
    hitPoints: number,
  ): Promise<CharacterCreature | undefined> {
    return (
      await this.characters.adjustCreatureHitPoints({
        campaignId,
        creatureId,
        change: { case: 'hitPoints', value: hitPoints },
      })
    ).creature;
  }

  /** The SRD's creatures that pass the filters, by Portuguese name, with how many there are in all. */
  async search(
    campaignId: string,
    filter: CreatureFilter,
  ): Promise<{ readonly creatures: readonly CreatureSummary[]; readonly total: number }> {
    const res = await this.content.listCreatures({
      campaignId,
      query: filter.query ?? '',
      type: filter.type ?? '',
      maxCr: filter.maxCr ?? '',
      minCr: filter.minCr ?? '',
      size: filter.size,
      pageSize: filter.pageSize ?? 100,
    });
    return { creatures: res.creatures, total: res.total };
  }

  /**
   * The bestiary's "Criar NPC" (MR-042, RN-29): a named NPC with a basic sheet made from the creature.
   * `idempotencyKey` is made once per open dialog, so a retry or a double tap makes one NPC.
   */
  async createNpc(
    campaignId: string,
    creatureKey: string,
    name: string,
    role: NpcRole,
    idempotencyKey: string,
  ): Promise<Character | undefined> {
    return (
      await this.characters.createNpcFromCreature({
        campaignId,
        creatureKey,
        name,
        kind: role === 'minion' ? CharacterKind.MINION : CharacterKind.STORY,
        idempotencyKey,
      })
    ).character;
  }

  /** The stat block of a character's creature as worked out for its owner when it arrived (read again each time: it is the creature's own). */
  async creatureBlock(campaignId: string, key: string, creatureId: string): Promise<Creature> {
    const res = await this.content.getCreature({ campaignId, key, creatureId });
    if (!res.creature) {
      throw new Error('empty stat block');
    }
    return res.creature;
  }

  /** One stat block, in memory after the first read. */
  statBlock(campaignId: string, key: string): Promise<Creature> {
    const known = this.statBlocks.get(key);
    if (known) {
      return known;
    }
    const read = this.content.getCreature({ campaignId, key }).then((res) => {
      if (!res.creature) {
        throw new Error('empty stat block');
      }
      return res.creature;
    });
    this.statBlocks.set(key, read);
    // A failed read is tried again next time.
    read.catch(() => this.statBlocks.delete(key));
    return read;
  }

  /** The caster's own vitals in the open session (the slots), or `null` when it has none to show. */
  async vitalsOf(campaignId: string, characterId: string): Promise<CharacterVitals | null> {
    const res = await this.play.getLiveSession({ campaignId });
    return res.vitals.find((v) => v.characterId === characterId) ?? null;
  }

  /** Casts Convocar Familiar, Animar Mortos or Conjurar Animais outside a combat. */
  async castSummon(
    cast: SummonCast,
  ): Promise<{ readonly creatureIds: readonly string[]; readonly replacedIds: readonly string[] }> {
    const res = await this.play.castSummon({
      campaignId: cast.campaignId,
      characterId: cast.characterId,
      spellKey: cast.spellKey,
      ritual: cast.ritual,
      slot: cast.slot,
      ...(cast.grantedBy ? { grantedBy: cast.grantedBy } : {}),
      summon: {
        option: cast.summon.option,
        creatureKeys: [...cast.summon.creatureKeys],
        names: [...cast.summon.names],
      },
      idempotencyKey: cast.idempotencyKey,
    });
    return { creatureIds: res.creatureIds, replacedIds: res.dismissedCreatureIds };
  }
}
