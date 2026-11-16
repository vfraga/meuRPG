import { Injectable, inject } from '@angular/core';
import { createClient } from '@connectrpc/connect';

import {
  CharacterKind,
  CharacterService,
  CharacterState,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';

/** A player character the Inspiração may be passed to ("Tor", "Guerreiro 5"). */
export interface FavorRecipient {
  readonly characterId: string;
  readonly name: string;
  /** The class and the level, as the character list says them: "Guerreiro 5". Empty for a basic sheet. */
  readonly detail: string;
}

/**
 * The other player characters of the campaign, for "Para quem" of the dialog that passes the Inspiração (RN-43, 581-04): a living
 * player character that is not in the reserve, other than the giver. It reads the campaign's character list; what the list holds
 * is the server's to say, and a reader it holds no other player's character for gets none, so the sheet offers no "Passar a…" (it
 * would have no one to name). `providedIn: 'root'`, replaced in tests.
 */
@Injectable({ providedIn: 'root' })
export class FavorRecipients {
  private readonly client = createClient(CharacterService, inject(CONNECT_TRANSPORT));

  async list(campaignId: string, giverId: string): Promise<readonly FavorRecipient[]> {
    const res = await this.client.listCharacters({ campaignId });
    return res.characters
      .filter(
        (c) =>
          c.kind === CharacterKind.PLAYER &&
          (c.state === CharacterState.DRAFT || c.state === CharacterState.LOCKED) &&
          !c.reserved &&
          c.id !== giverId,
      )
      .map((c) => ({ characterId: c.id, name: c.name, detail: c.classSummary }));
  }
}
