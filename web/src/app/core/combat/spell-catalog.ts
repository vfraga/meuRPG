import { Injectable, inject } from '@angular/core';
import { createClient } from '@connectrpc/connect';

import { ContentService, type SpellDetails } from '../../../gen/meurpg/rules/v1/rules_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';

/**
 * What the SRD says about a spell (`GetSpellDetails`), read when the cast sheet
 * opens: whether it rolls to hit, asks for a save or heals. The names of the
 * spells a combat shows come with the combat itself (`concentration_spell_name_pt`,
 * `ReactionPrompt.spell_name_pt`), so nothing here reads names. A lazy-route
 * service: only the combat imports it.
 */
@Injectable({ providedIn: 'root' })
export class SpellCatalog {
  private readonly client = createClient(ContentService, inject(CONNECT_TRANSPORT));
  private readonly detailsByKey = new Map<string, Promise<SpellDetails | null>>();

  /** What the SRD says about one spell (`GetSpellDetails`): the cast sheet
   * needs to know whether it rolls to hit, asks for a save or heals. One read
   * for each SRD spell, kept; a spell of the table (`@mesa`) is read each time,
   * since the master can change it while the page is open. `null` when it could
   * not be read (and then asked again the next time). */
  details(campaignId: string, spellKey: string): Promise<SpellDetails | null> {
    const id = `${campaignId}/${spellKey}`;
    const kept = !spellKey.endsWith('@mesa');
    let known = kept ? this.detailsByKey.get(id) : undefined;
    if (!known) {
      known = this.client
        .getSpellDetails({ campaignId, spellKey })
        .then((res) => res.spell ?? null)
        .catch(() => {
          this.detailsByKey.delete(id);
          return null;
        });
      if (kept) {
        this.detailsByKey.set(id, known);
      }
    }
    return known;
  }

  /** The table's content changed (or may have, while the stream was down): the next read asks again. */
  forget(campaignId: string): void {
    for (const id of [...this.detailsByKey.keys()]) {
      if (id.startsWith(`${campaignId}/`)) {
        this.detailsByKey.delete(id);
      }
    }
  }
}
