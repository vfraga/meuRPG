// Finding U16-09 in review/unit-16-web-content-campaigns.md
import { Injectable, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import { AuthState, AuthService } from '../../core/auth/auth.service';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import { InviteAccept } from './invite-accept';

@Injectable()
class FakeAuthService {
  readonly state = signal<AuthState>({ status: 'unknown' }).asReadonly();
}

@Injectable()
class FakeCampaignsService {
  readonly acceptInvite = vi.fn();
}

describe('Review16 U16-09: malformed percent-escape in invite fragment', () => {
  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [InviteAccept],
      providers: [
        provideRouter([]),
        { provide: AuthService, useClass: FakeAuthService },
        { provide: CampaignsService, useClass: FakeCampaignsService },
      ],
    });
  });

  afterEach(() => {
    window.location.hash = '';
  });

  it('shows the invalid-link message and strips the hash instead of throwing', () => {
    window.location.hash = '#t=abc%';

    let fixture: ReturnType<typeof TestBed.createComponent<InviteAccept>> | undefined;
    expect(() => {
      fixture = TestBed.createComponent(InviteAccept);
    }).not.toThrow();
    fixture?.detectChanges();

    expect(window.location.hash).toBe('');
    expect((fixture?.nativeElement as HTMLElement | undefined)?.textContent).toContain(
      'Link de convite inválido',
    );
  });
});
