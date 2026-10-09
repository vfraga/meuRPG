import { Injectable, inject } from '@angular/core';
import { createClient } from '@connectrpc/connect';

import {
  CharacterService,
  type CharacterSummary,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import type { ItemRequestKind } from '../../../gen/meurpg/characters/v1/inventory_pb';
import {
  type AbilityCheck,
  type CatalogItem,
  type ChargesRegained,
  InventoryService,
  type InventoryView,
  type ItemGrant,
  type ItemLogEntry,
  type ItemRestChoice,
  type ItemRestDone,
  type PendingItemRest,
  type TextProposal,
  type UseOutcome,
} from '../../../gen/meurpg/characters/v1/inventory_service_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';

/** How the dice of a use come: the app rolls, or the person typed the physical dice. */
export type ItemRoll =
  | { readonly kind: 'app' }
  | { readonly kind: 'typed'; readonly value: number };

/** What reading a scroll came to. */
export interface ScrollRead {
  readonly inventory: InventoryView;
  readonly spellNamePt: string;
  readonly spellLevel: number;
  readonly saveDc: number;
  readonly attackBonus: number;
  readonly check?: AbilityCheck;
  readonly cast: boolean;
}

/**
 * Thin wrapper around `InventoryService` (W7-I): the character's items, coins, attunement marks, the
 * master's short rest, potions, scrolls, charges and ammunition. Every call that changes something
 * takes the idempotency key the screen made for that action. `providedIn: 'root'`, and only lazy code
 * imports it, so the generated code stays out of the initial bundle. Callers map errors to Portuguese
 * with `inventory-errors.ts`; the server decides every rule, this only sends and returns.
 */
@Injectable({ providedIn: 'root' })
export class InventoryClient {
  private readonly transport = inject(CONNECT_TRANSPORT);
  private readonly api = createClient(InventoryService, this.transport);
  private readonly characters = createClient(CharacterService, this.transport);
  private catalogs = new Map<string, Promise<readonly CatalogItem[]>>();

  async get(campaignId: string, characterId: string): Promise<InventoryView> {
    const res = await this.api.getInventory({ campaignId, characterId });
    return required(res.inventory);
  }

  /** The SRD's equipment and magic items a character can be given. The same for everyone: kept once read. */
  catalog(campaignId: string): Promise<readonly CatalogItem[]> {
    let cached = this.catalogs.get(campaignId);
    if (!cached) {
      cached = this.api.listItemCatalog({ campaignId }).then((r) => r.items);
      cached.catch(() => this.catalogs.delete(campaignId));
      this.catalogs.set(campaignId, cached);
    }
    return cached;
  }

  async give(
    campaignId: string,
    characterId: string,
    grants: readonly ItemGrant[],
    idempotencyKey: string,
  ): Promise<InventoryView> {
    const res = await this.api.giveItems({
      campaignId,
      characterId,
      grants: [...grants],
      idempotencyKey,
    });
    return required(res.inventory);
  }

  async setEquipped(
    campaignId: string,
    characterId: string,
    itemId: string,
    equipped: boolean,
  ): Promise<InventoryView> {
    const res = await this.api.setEquipped({ campaignId, characterId, itemId, equipped });
    return required(res.inventory);
  }

  async remove(
    campaignId: string,
    characterId: string,
    itemId: string,
    quantity: number,
    idempotencyKey: string,
  ): Promise<InventoryView> {
    const res = await this.api.removeItem({
      campaignId,
      characterId,
      itemId,
      quantity,
      idempotencyKey,
    });
    return required(res.inventory);
  }

  /** "Dar": the item changes hands, no action spent. The giver's inventory comes back. */
  async transfer(
    campaignId: string,
    fromCharacterId: string,
    toCharacterId: string,
    itemId: string,
    quantity: number,
    idempotencyKey: string,
  ): Promise<InventoryView> {
    const res = await this.api.transferItem({
      campaignId,
      fromCharacterId,
      toCharacterId,
      itemId,
      quantity,
      idempotencyKey,
    });
    return required(res.from);
  }

  async setCoins(
    campaignId: string,
    characterId: string,
    coins: Readonly<Record<'copper' | 'silver' | 'electrum' | 'gold' | 'platinum', number>>,
    idempotencyKey: string,
  ): Promise<InventoryView> {
    const res = await this.api.setCoins({ campaignId, characterId, coins, idempotencyKey });
    return required(res.inventory);
  }

  /** The player's mark for the next short rest (or the master's directly). */
  async request(
    campaignId: string,
    characterId: string,
    itemId: string,
    kind: ItemRequestKind,
    idempotencyKey: string,
  ): Promise<void> {
    await this.api.requestItemRest({ campaignId, characterId, itemId, kind, idempotencyKey });
  }

  /** The master answers one marked item now (identify) or refuses it. */
  async resolve(
    campaignId: string,
    characterId: string,
    itemId: string,
    kind: ItemRequestKind,
    approve: boolean,
    idempotencyKey: string,
  ): Promise<InventoryView> {
    const res = await this.api.resolveItemRest({
      campaignId,
      characterId,
      itemId,
      kind,
      approve,
      idempotencyKey,
    });
    return required(res.inventory);
  }

  async pendingRests(campaignId: string): Promise<readonly PendingItemRest[]> {
    return (await this.api.listItemRests({ campaignId })).pending;
  }

  async confirmRests(
    campaignId: string,
    choices: readonly ItemRestChoice[],
    idempotencyKey: string,
  ): Promise<readonly ItemRestDone[]> {
    return (await this.api.confirmItemRests({ campaignId, choices: [...choices], idempotencyKey }))
      .done;
  }

  /** A potion outside a combat. `targetCharacterId` gives it to someone else to drink. */
  async drink(
    campaignId: string,
    characterId: string,
    itemId: string,
    roll: ItemRoll | null,
    idempotencyKey: string,
    targetCharacterId = '',
  ): Promise<{ inventory: InventoryView; outcome: UseOutcome | undefined }> {
    const res = await this.api.useItem({
      campaignId,
      characterId,
      itemId,
      targetCharacterId,
      idempotencyKey,
      roll: rollOf(roll),
    });
    return { inventory: required(res.inventory), outcome: res.outcome };
  }

  async readScroll(
    campaignId: string,
    characterId: string,
    itemId: string,
    d20: ItemRoll | null,
    idempotencyKey: string,
  ): Promise<ScrollRead> {
    const res = await this.api.useScroll({
      campaignId,
      characterId,
      itemId,
      idempotencyKey,
      roll:
        d20 === null
          ? { case: undefined }
          : d20.kind === 'app'
            ? { case: 'rollInApp', value: true }
            : { case: 'd20Face', value: d20.value },
    });
    return {
      inventory: required(res.inventory),
      spellNamePt: res.spellNamePt,
      spellLevel: res.spellLevel,
      saveDc: res.saveDc,
      attackBonus: res.attackBonus,
      check: res.abilityCheck,
      cast: res.cast,
    };
  }

  async spendCharges(
    campaignId: string,
    characterId: string,
    itemId: string,
    charges: number,
    idempotencyKey: string,
  ): Promise<{ inventory: InventoryView; d20: number; destroyed: boolean }> {
    const res = await this.api.spendCharges({
      campaignId,
      characterId,
      itemId,
      charges,
      idempotencyKey,
    });
    return { inventory: required(res.inventory), d20: res.d20, destroyed: res.destroyed };
  }

  async recoverAmmunition(
    campaignId: string,
    characterId: string,
    itemId: string,
    count: number,
    idempotencyKey: string,
  ): Promise<InventoryView> {
    const res = await this.api.recoverAmmunition({
      campaignId,
      characterId,
      itemId,
      count,
      idempotencyKey,
    });
    return required(res.inventory);
  }

  async previewText(
    campaignId: string,
    characterId: string,
    itemId: string,
  ): Promise<readonly TextProposal[]> {
    return (await this.api.previewTextToItems({ campaignId, characterId, itemId })).proposals;
  }

  async confirmText(
    campaignId: string,
    characterId: string,
    itemId: string,
    lines: readonly TextProposal[],
    idempotencyKey: string,
  ): Promise<InventoryView> {
    const res = await this.api.confirmTextToItems({
      campaignId,
      characterId,
      itemId,
      lines: [...lines],
      idempotencyKey,
    });
    return required(res.inventory);
  }

  /** The master's dawn: every wand and staff regains its charges. */
  async regainCharges(
    campaignId: string,
    idempotencyKey: string,
  ): Promise<readonly ChargesRegained[]> {
    return (await this.api.regainCharges({ campaignId, idempotencyKey })).regained;
  }

  async log(campaignId: string, characterId = ''): Promise<readonly ItemLogEntry[]> {
    return (await this.api.listItemLog({ campaignId, characterId, limit: 50 })).entries;
  }

  /** The campaign's characters a player can hand an item to (living ones with a full sheet), for "Dar a alguém". */
  async recipients(campaignId: string): Promise<readonly CharacterSummary[]> {
    return (await this.characters.listCharacters({ campaignId })).characters;
  }
}

function rollOf(roll: ItemRoll | null) {
  if (roll === null) {
    return { case: undefined } as const;
  }
  return roll.kind === 'app'
    ? ({ case: 'rollInApp', value: true } as const)
    : ({ case: 'typedSum', value: roll.value } as const);
}

function required<T>(value: T | undefined): T {
  if (value === undefined) {
    throw new Error('The server answered without the inventory.');
  }
  return value;
}
