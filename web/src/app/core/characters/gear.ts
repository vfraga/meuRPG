import { Injectable, inject } from '@angular/core';
import { createClient } from '@connectrpc/connect';

import { CharacterService } from '../../../gen/meurpg/characters/v1/characters_pb';
import { type GearEntry, GearKind } from '../../../gen/meurpg/rules/v1/rules_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';

/** One piece of gear a sheet lists, equipped or carried (`DerivedSheet.gear`): the names and the armor class come from
 * the server, never from the browser. */
export interface GearVm {
  readonly key: string;
  readonly namePt: string;
  readonly kind: 'armor' | 'shield' | 'weapon';
  readonly equipped: boolean;
  readonly twoHanded: boolean;
  /** The armor class with this piece's state flipped (0: the server does not preview it, e.g. an armor spell). */
  readonly armorClassIfToggled: number;
  /** How many copies the sheet lists: weapons with one key are one row, their state kept by key. */
  readonly count: number;
}

const KINDS: Record<number, GearVm['kind']> = {
  [GearKind.ARMOR]: 'armor',
  [GearKind.SHIELD]: 'shield',
  [GearKind.WEAPON]: 'weapon',
};

export function toGearVm(g: GearEntry): GearVm {
  return {
    key: g.key,
    namePt: g.namePt,
    kind: KINDS[g.kind] ?? 'weapon',
    equipped: g.equipped,
    twoHanded: g.twoHanded,
    armorClassIfToggled: g.armorClassIfToggled,
    count: Math.max(1, g.count),
  };
}

/** What a read of a character's gear holds: the list, the armor class now, whether the shield counts for nothing (a
 * two-handed weapon is equipped) and the sheet's revision. */
export interface GearRead {
  readonly gear: GearVm[];
  readonly armorClass: number;
  readonly shieldIdle: boolean;
  readonly revision: number;
}

/** The gear of a character and the change of it (`SetEquipped`), for the screens that are not the sheet page (the
 * combat). `providedIn: 'root'`, imported only by lazy code; tests replace it with `{ provide: GearClient, useValue }`. */
@Injectable({ providedIn: 'root' })
export class GearClient {
  private readonly client = createClient(CharacterService, inject(CONNECT_TRANSPORT));

  /** The gear and the armor class now, as the server derives them. */
  async read(campaignId: string, characterId: string): Promise<GearRead> {
    const res = await this.client.getCharacter({ campaignId, characterId });
    const derived = res.character?.derived;
    return {
      gear: (derived?.gear ?? []).map(toGearVm),
      armorClass: derived?.armorClass ?? 0,
      shieldIdle: derived?.shieldIdle ?? false,
      revision: res.character?.revision ?? 0,
    };
  }

  async set(
    campaignId: string,
    characterId: string,
    key: string,
    equipped: boolean,
  ): Promise<void> {
    await this.client.setEquipped({ campaignId, characterId, key, equipped });
  }
}
