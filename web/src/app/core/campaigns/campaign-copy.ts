import { Code, ConnectError } from '@connectrpc/connect';

import {
  Campaign,
  CampaignCreationRefusedReason,
  CampaignCreationRefusedSchema,
  Role,
  XpMode,
} from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { roleLabel } from './campaign-labels';

/**
 * The campaign's XP mode as a phrase that stands on its own ("XP por
 * inimigos derrotados"), for the second line of a campaign on "Minhas
 * campanhas" and the lead of the campaign page. `null` when the mode is not
 * set: a pending member's campaign comes without it (campaigns.proto,
 * `Campaign.awaiting_approval`).
 */
export function xpModeSentence(xpMode: XpMode): string | null {
  switch (xpMode) {
    case XpMode.ENEMIES:
      return 'XP por inimigos derrotados';
    case XpMode.GOLD:
      return 'XP por ouro';
    case XpMode.MILESTONES:
      return 'XP por marcos';
    default:
      return null;
  }
}

/** The caller's role as a tag on "Minhas campanhas": one word, capitalised. */
export function roleTag(role: Role): string {
  return role === Role.MASTER ? 'Mestre' : 'Jogador';
}

/**
 * The line under the campaign's name: "Você é mestre nesta campanha. XP por
 * inimigos derrotados." The first sentence is what the e2e tests look for
 * (`campaigns.spec.ts`), so it keeps the role word lowercase.
 */
export function campaignLead(campaign: Pick<Campaign, 'myRole' | 'xpMode'>): string {
  const role = `Você é ${roleLabel(campaign.myRole)} nesta campanha.`;
  const xp = xpModeSentence(campaign.xpMode);
  return xp ? `${role} ${xp}.` : role;
}

/**
 * Why "Criar campanha" was refused (RN-30), in words, or `null` for any other
 * error. It reads the typed reason of the error, never its message: the
 * account is already master of the most campaigns the server allows, or the
 * server only lets some people create them.
 */
export function creationRefusalText(err: unknown): string | null {
  const e = ConnectError.from(err, Code.Unavailable);
  const refusal = e.findDetails(CampaignCreationRefusedSchema)[0];
  switch (refusal?.reason) {
    case CampaignCreationRefusedReason.LIMIT_REACHED:
      return `Você já é mestre de ${refusal.maxCampaigns} campanhas, o máximo por conta neste servidor. Use uma das que você já tem.`;
    case CampaignCreationRefusedReason.NOT_ALLOWED:
      return 'Este servidor só deixa algumas pessoas criarem campanhas. Peça ao mestre da sua mesa um convite para jogar.';
    default:
      return null;
  }
}
