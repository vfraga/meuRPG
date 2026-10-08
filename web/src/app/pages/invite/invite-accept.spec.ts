import { Injectable, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter, Router } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';

import { AuthState, AuthService } from '../../core/auth/auth.service';
import { Campaign, Role, XpMode } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import { InviteAccept } from './invite-accept';

@Injectable()
class FakeAuthService {
  private readonly stateSignal = signal<AuthState>({ status: 'unknown' });
  readonly state = this.stateSignal.asReadonly();

  set(state: AuthState): void {
    this.stateSignal.set(state);
  }
}

@Injectable()
class FakeCampaignsService {
  readonly acceptInvite = vi.fn();
}

function campaign(id: string): Campaign {
  return {
    id,
    name: 'Mirathel',
    myRole: Role.PLAYER,
    xpMode: XpMode.ENEMIES,
    createdAt: undefined,
  } as Campaign;
}

/** Drains every pending microtask (toObservable's effect, the subscribe
 * callback, and accept()'s own `await`s all hop through a few of them) — a
 * macrotask only runs once the microtask queue is empty, so this is a more
 * robust "let everything settle" than a fixed number of `Promise.resolve()`s. */
function flush(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

describe('InviteAccept', () => {
  let auth: FakeAuthService;
  let campaigns: FakeCampaignsService;
  let router: Router;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [InviteAccept],
      providers: [
        provideRouter([]),
        { provide: AuthService, useClass: FakeAuthService },
        { provide: CampaignsService, useClass: FakeCampaignsService },
      ],
    });
    auth = TestBed.inject(AuthService) as unknown as FakeAuthService;
    campaigns = TestBed.inject(CampaignsService) as unknown as FakeCampaignsService;
    router = TestBed.inject(Router);
  });

  afterEach(() => {
    window.location.hash = '';
  });

  it('reads the token from the hash and immediately strips it with history.replaceState, never touching storage', () => {
    window.location.hash = '#t=super-secret-token';
    const replaceState = vi.spyOn(history, 'replaceState');
    const setItem = vi.spyOn(Storage.prototype, 'setItem');

    TestBed.createComponent(InviteAccept);

    expect(replaceState).toHaveBeenCalledTimes(1);
    // replaceState, not pushState: no new history entry for the token.
    expect(window.location.hash).toBe('');
    expect(setItem).not.toHaveBeenCalled();
  });

  it('treats a malformed percent-escape in the fragment as an invalid link and still strips it', () => {
    window.location.hash = '#t=abc%';
    let fixture: ReturnType<typeof TestBed.createComponent<InviteAccept>> | undefined;
    expect(() => {
      fixture = TestBed.createComponent(InviteAccept);
    }).not.toThrow();
    fixture?.detectChanges();
    expect(window.location.hash).toBe('');
    expect((fixture?.nativeElement as HTMLElement).textContent).toContain(
      'Link de convite inválido',
    );
  });

  it('shows a friendly message when there is no token at all', () => {
    window.location.hash = '';
    const fixture = TestBed.createComponent(InviteAccept);
    fixture.detectChanges();

    expect((fixture.nativeElement as HTMLElement).textContent).toContain(
      'Link de convite inválido',
    );
  });

  it('signed in: calls AcceptInvite and navigates to the campaign', async () => {
    window.location.hash = '#t=abc123';
    campaigns.acceptInvite.mockResolvedValue({
      campaign: campaign('camp-1'),
      alreadyMember: false,
    });
    const navigateSpy = vi.spyOn(router, 'navigate').mockResolvedValue(true);

    const fixture = TestBed.createComponent(InviteAccept);
    fixture.detectChanges();
    auth.set({
      status: 'signed-in',
      user: { id: 'u1', displayName: null },
      sessionExpiresAt: null,
    });
    await flush();
    await fixture.whenStable();

    expect(campaigns.acceptInvite).toHaveBeenCalledWith('abc123');
    expect(navigateSpy).toHaveBeenCalledWith(['/campaigns', 'camp-1']);
  });

  it('already_member also navigates to the campaign (idempotent accept)', async () => {
    window.location.hash = '#t=abc123';
    campaigns.acceptInvite.mockResolvedValue({ campaign: campaign('camp-1'), alreadyMember: true });
    const navigateSpy = vi.spyOn(router, 'navigate').mockResolvedValue(true);

    const fixture = TestBed.createComponent(InviteAccept);
    fixture.detectChanges();
    auth.set({
      status: 'signed-in',
      user: { id: 'u1', displayName: null },
      sessionExpiresAt: null,
    });
    await flush();
    await fixture.whenStable();

    expect(navigateSpy).toHaveBeenCalledWith(['/campaigns', 'camp-1']);
  });

  it('invite with approval: a new pending member goes straight to creating the character (MR-024)', async () => {
    window.location.hash = '#t=abc123';
    campaigns.acceptInvite.mockResolvedValue({
      campaign: { ...campaign('camp-1'), awaitingApproval: true, diceMode: 1, dicePreference: 1 },
      alreadyMember: false,
    });
    const navigateSpy = vi.spyOn(router, 'navigate').mockResolvedValue(true);

    const fixture = TestBed.createComponent(InviteAccept);
    fixture.detectChanges();
    auth.set({
      status: 'signed-in',
      user: { id: 'u1', displayName: null },
      sessionExpiresAt: null,
    });
    await flush();
    await fixture.whenStable();

    expect(navigateSpy).toHaveBeenCalledWith(['/campaigns', 'camp-1', 'characters', 'new']);
  });

  it('invite with approval, already pending: goes to the campaign page, which shows the wait', async () => {
    window.location.hash = '#t=abc123';
    campaigns.acceptInvite.mockResolvedValue({
      campaign: { ...campaign('camp-1'), awaitingApproval: true, diceMode: 1, dicePreference: 1 },
      alreadyMember: true,
    });
    const navigateSpy = vi.spyOn(router, 'navigate').mockResolvedValue(true);

    const fixture = TestBed.createComponent(InviteAccept);
    fixture.detectChanges();
    auth.set({
      status: 'signed-in',
      user: { id: 'u1', displayName: null },
      sessionExpiresAt: null,
    });
    await flush();
    await fixture.whenStable();

    expect(navigateSpy).toHaveBeenCalledWith(['/campaigns', 'camp-1']);
  });

  it('signed in, invite expired: shows the specific InviteUnusable message', async () => {
    window.location.hash = '#t=abc123';
    campaigns.acceptInvite.mockRejectedValue(new ConnectError('expired', Code.FailedPrecondition));

    const fixture = TestBed.createComponent(InviteAccept);
    fixture.detectChanges();
    auth.set({
      status: 'signed-in',
      user: { id: 'u1', displayName: null },
      sessionExpiresAt: null,
    });
    await flush();
    await fixture.whenStable();
    fixture.detectChanges();

    expect((fixture.nativeElement as HTMLElement).textContent).toContain('não pode mais ser usado');
  });

  it('signed out: shows "Entrar para aceitar o convite" and submits the sign-in-through-invite form fields', () => {
    window.location.hash = '#t=super-secret-token';
    const submitSpy = vi
      .spyOn(HTMLFormElement.prototype, 'submit')
      .mockImplementation(() => undefined);

    const fixture = TestBed.createComponent(InviteAccept);
    fixture.detectChanges();
    auth.set({ status: 'signed-out' });
    fixture.detectChanges();

    const el = fixture.nativeElement as HTMLElement;
    const button = Array.from(el.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Entrar para aceitar o convite'),
    ) as HTMLButtonElement;
    expect(button).toBeTruthy();
    button.click();

    const form = document.querySelector('form[action="/auth/login"]') as HTMLFormElement;
    expect(form).toBeTruthy();
    expect(form.method).toBe('post');

    const valueOf = (name: string) =>
      (form.querySelector(`input[name="${name}"]`) as HTMLInputElement | null)?.value;
    expect(valueOf('return_to')).toBe('/campaigns');
    expect(valueOf('intent')).toBe('campaign_invite');
    expect(valueOf('intent_payload')).toBe('super-secret-token');
    expect(submitSpy).toHaveBeenCalled();

    document.body.removeChild(form);
  });

  it('unavailable session state: shows a retry-worthy message, not "signed out"', async () => {
    window.location.hash = '#t=abc123';
    const fixture = TestBed.createComponent(InviteAccept);
    fixture.detectChanges();
    auth.set({ status: 'unavailable' });
    fixture.detectChanges();

    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';
    expect(text).toContain('Tente de novo');
    expect(text).not.toContain('Entrar para aceitar');
  });
});
