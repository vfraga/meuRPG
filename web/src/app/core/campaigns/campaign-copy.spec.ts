import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  CampaignCreationRefusedReason,
  CampaignCreationRefusedSchema,
  Role,
  XpMode,
} from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { campaignLead, creationRefusalText, roleTag, xpModeSentence } from './campaign-copy';

describe('campaign copy', () => {
  it('says each XP mode as a phrase, and nothing for an unset one', () => {
    expect(xpModeSentence(XpMode.ENEMIES)).toBe('XP por inimigos derrotados');
    expect(xpModeSentence(XpMode.GOLD)).toBe('XP por ouro');
    expect(xpModeSentence(XpMode.MILESTONES)).toBe('XP por marcos');
    expect(xpModeSentence(XpMode.UNSPECIFIED)).toBeNull();
  });

  it('tags the role with one capitalised word', () => {
    expect(roleTag(Role.MASTER)).toBe('Mestre');
    expect(roleTag(Role.PLAYER)).toBe('Jogador');
  });

  it('writes the lead with the role and, when set, the XP mode', () => {
    expect(campaignLead({ myRole: Role.MASTER, xpMode: XpMode.ENEMIES })).toBe(
      'Você é mestre nesta campanha. XP por inimigos derrotados.',
    );
    expect(campaignLead({ myRole: Role.PLAYER, xpMode: XpMode.UNSPECIFIED })).toBe(
      'Você é jogador nesta campanha.',
    );
  });
});

describe('creationRefusalText (RN-30)', () => {
  const refused = (code: Code, reason: CampaignCreationRefusedReason, maxCampaigns = 0) =>
    new ConnectError('refused', code, undefined, [
      {
        desc: CampaignCreationRefusedSchema,
        value: create(CampaignCreationRefusedSchema, { reason, maxCampaigns }),
      },
    ]);

  it('says how many campaigns the account may be master of', () => {
    const err = refused(Code.ResourceExhausted, CampaignCreationRefusedReason.LIMIT_REACHED, 10);
    expect(creationRefusalText(err)).toContain('Você já é mestre de 10 campanhas');
  });

  it('says the server only lets some people create campaigns', () => {
    const err = refused(Code.PermissionDenied, CampaignCreationRefusedReason.NOT_ALLOWED);
    expect(creationRefusalText(err)).toContain('só deixa algumas pessoas criarem campanhas');
  });

  it('leaves any other error to the usual wording', () => {
    expect(creationRefusalText(new ConnectError('bad', Code.InvalidArgument))).toBeNull();
    expect(creationRefusalText(new TypeError('Failed to fetch'))).toBeNull();
  });
});
