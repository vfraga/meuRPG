import { Injectable } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';

import { Invite, InviteState } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CampaignsService } from '../../../core/campaigns/campaigns.service';
import { CampaignInvites } from './invites';

@Injectable()
class FakeCampaignsService {
  readonly revokeInvite = vi.fn();
  listInvites(): Promise<{ invites: Invite[] }> {
    return Promise.resolve({
      invites: [mk('i1'), mk('i2')],
    });
  }
}

function mk(id: string): Invite {
  return {
    id,
    state: InviteState.ACTIVE,
    useCount: 0,
    maxUses: 1,
    requiresApproval: false,
    createdAt: undefined,
    expiresAt: undefined,
    revokedAt: undefined,
  } as Invite;
}

describe('CampaignInvites revoking', () => {
  it('keeps both invite rows when RevokeInvite rejects', async () => {
    TestBed.configureTestingModule({
      imports: [CampaignInvites],
      providers: [{ provide: CampaignsService, useClass: FakeCampaignsService }],
    });
    const fake = TestBed.inject(CampaignsService) as unknown as FakeCampaignsService;
    fake.revokeInvite.mockRejectedValue(new ConnectError('down', Code.Unavailable));
    const fixture = TestBed.createComponent(CampaignInvites);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelectorAll('li.invite').length).toBe(2);

    const btn = Array.from(el.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Revogar'),
    ) as HTMLButtonElement;
    btn.click();
    await fixture.whenStable();
    fixture.detectChanges();

    expect(fake.revokeInvite).toHaveBeenCalled();
    expect(el.querySelectorAll('li.invite').length).toBe(2);
  });
});
