import { Injectable, inject } from '@angular/core';
import { createClient } from '@connectrpc/connect';
import type { MessageInitShape } from '@bufbuild/protobuf';

import {
  ContentService,
  type ListSpellsRequestSchema,
  type ListSpellsResponse,
  type SpellDetails,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';

/** A class the "Classe" filter offers. */
export interface SpellClass {
  readonly key: string;
  readonly namePt: string;
  /** The master's view only: the table retired it. */
  readonly archived: boolean;
}

/**
 * The calls of the "Magias" page (MR-045): `ListSpells` (search, filters, pages), `GetSpellDetails` and
 * the class names for the filter. `providedIn: 'root'`, and only the lazy page imports it, so the
 * generated code stays out of the initial bundle. Details and class names are kept once read, per
 * campaign (a table's own spells live in the campaign), dropped when the campaign's content version changes (the master's edit), and asked again after a failure.
 */
@Injectable({ providedIn: 'root' })
export class SpellsClient {
  private readonly client = createClient(ContentService, inject(CONNECT_TRANSPORT));
  private readonly detailsByKey = new Map<string, Promise<SpellDetails>>();
  private readonly classesByCampaign = new Map<string, Promise<readonly SpellClass[]>>();

  /** The content version each campaign's answers carried last: the master's edit changes it, and the caches below start over. */
  private readonly versionByCampaign = new Map<string, string>();

  async list(
    request: MessageInitShape<typeof ListSpellsRequestSchema>,
  ): Promise<ListSpellsResponse> {
    const res = await this.client.listSpells(request);
    this.noteVersion(request.campaignId ?? '', res.contentVersion);
    return res;
  }

  /** The content version of a campaign changed: what was read under the old one (details, class names) is read again. */
  private noteVersion(campaignId: string, version: string): void {
    const known = this.versionByCampaign.get(campaignId);
    this.versionByCampaign.set(campaignId, version);
    if (known !== undefined && known !== version) {
      this.classesByCampaign.delete(campaignId);
      for (const key of [...this.detailsByKey.keys()]) {
        if (key.startsWith(`${campaignId}/`)) {
          this.detailsByKey.delete(key);
        }
      }
    }
  }

  /** One spell's card; `fresh` drops what was read before (a `content_changed` told the card is out of date). */
  details(campaignId: string, spellKey: string, fresh = false): Promise<SpellDetails> {
    const id = `${campaignId}/${spellKey}`;
    if (fresh) {
      this.detailsByKey.delete(id);
    }
    let known = this.detailsByKey.get(id);
    if (!known) {
      known = this.client
        .getSpellDetails({ campaignId, spellKey })
        .then((res) => {
          if (!res.spell) {
            throw new Error('empty spell');
          }
          return res.spell;
        })
        .catch((err: unknown) => {
          this.detailsByKey.delete(id);
          throw err;
        });
      this.detailsByKey.set(id, known);
    }
    return known;
  }

  /** The classes of the table (the SRD's and the master's) that have spells, by Portuguese name. */
  classes(campaignId: string): Promise<readonly SpellClass[]> {
    let known = this.classesByCampaign.get(campaignId);
    if (!known) {
      known = this.client
        .listContent({ campaignId })
        .then((res) => {
          // Only the classes that have a spell list: the filter never offers a Bárbaro with nothing to find.
          const withSpells = new Set((res.content?.spells ?? []).flatMap((s) => s.classKeys));
          return (res.content?.classes ?? [])
            .filter((c) => withSpells.has(c.key))
            .map((c) => ({ key: c.key, namePt: c.namePt, archived: c.archived }))
            .sort((a, b) => a.namePt.localeCompare(b.namePt, 'pt-BR'));
        })
        .catch((err: unknown) => {
          this.classesByCampaign.delete(campaignId);
          throw err;
        });
      this.classesByCampaign.set(campaignId, known);
    }
    return known;
  }
}
