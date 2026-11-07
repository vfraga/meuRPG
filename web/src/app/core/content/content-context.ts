import { Code, ConnectError } from '@connectrpc/connect';

import { Role } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type {
  TableContentViolation,
  TableEntry,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import type { CampaignsService } from '../campaigns/campaigns.service';
import type { TableContentClient } from './content-client';

/** What every page of "Conteúdo da mesa" starts from: who is asking and the entries they may see. */
export interface ContentContext {
  readonly campaignId: string;
  readonly campaignName: string;
  readonly isMaster: boolean;
  readonly entries: readonly TableEntry[];
  readonly tableRevision: number;
  readonly contentVersion: string;
  /** The entries the rules refuse (the master's only): left out of the campaign's content until fixed or archived. */
  readonly leftOut: readonly TableContentViolation[];
}

export type ContextResult =
  { readonly status: 'ok'; readonly ctx: ContentContext } | { readonly status: 'not-found' };

/** The campaign (its name and the caller's role) and its entries. A campaign that is not the caller's, or a pending member's, is "not found". */
export async function loadContext(
  campaigns: CampaignsService,
  content: TableContentClient,
  campaignId: string,
): Promise<ContextResult> {
  try {
    const res = await campaigns.getCampaign(campaignId);
    const campaign = res.campaign;
    if (!campaign || campaign.awaitingApproval) {
      return { status: 'not-found' };
    }
    const list = await content.list(campaignId);
    return {
      status: 'ok',
      ctx: {
        campaignId,
        campaignName: campaign.name,
        isMaster: campaign.myRole === Role.MASTER,
        entries: list.entries,
        tableRevision: list.tableRevision,
        contentVersion: list.contentVersion,
        leftOut: list.leftOut,
      },
    };
  } catch (err) {
    if (ConnectError.from(err, Code.Unavailable).code === Code.NotFound) {
      return { status: 'not-found' };
    }
    throw err;
  }
}
