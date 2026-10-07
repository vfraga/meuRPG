// Finding U16-11 in review/unit-16-web-content-campaigns.md
import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter, Router } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';

import { AuthState, AuthService } from '../../core/auth/auth.service';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import { InviteAccept } from './invite-accept';

function flush(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

describe('Review16 U16-11: invite error state offers no retry', () => {
  afterEach(() => {
    window.location.hash = '';
  });

  it('offers a retry that calls acceptInvite again with the same token', async () => {
    const authState = signal<AuthState>({ status: 'signed-in' } as AuthState);
    const acceptInvite = vi
      .fn()
      .mockRejectedValueOnce(new ConnectError('down', Code.Unavailable))
      .mockResolvedValue({ campaign: { id: 'c1' } });
    TestBed.configureTestingModule({
      imports: [InviteAccept],
      providers: [
        provideRouter([]),
        { provide: AuthService, useValue: { state: authState.asReadonly() } },
        { provide: CampaignsService, useValue: { acceptInvite } },
      ],
    });
    vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);
    window.location.hash = '#t=abc';

    const fixture = TestBed.createComponent(InviteAccept);
    fixture.detectChanges();
    await flush();
    fixture.detectChanges();

    const el = fixture.nativeElement as HTMLElement;
    expect(el.textContent).toContain('Tente de novo');
    expect(acceptInvite).toHaveBeenCalledTimes(1);

    const retry = el.querySelector('button') as HTMLButtonElement | null;
    expect(retry, 'a retry button in the error state').not.toBeNull();
    retry!.click();
    await flush();

    expect(acceptInvite).toHaveBeenCalledTimes(2);
    expect(acceptInvite).toHaveBeenLastCalledWith('abc');
  });
});
