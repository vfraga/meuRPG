import { ChangeDetectionStrategy, Component, TemplateRef, signal, viewChild } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { Router, provideRouter } from '@angular/router';

import { AuthState, AuthService } from './core/auth/auth.service';
import { OPEN_SESSIONS_FETCHER, OpenSessions } from './core/play/open-sessions';
import type { OpenSessionVm } from './core/play/open-sessions.types';
import { SessionNotes } from './shell/session-notes/session-notes';
import { App } from './app';

describe('App', () => {
  // App always renders <app-user-menu>, which injects AuthService. Stub it
  // so this test never makes a real GetMe call.
  const authStub: Pick<AuthService, 'state'> = {
    state: signal<AuthState>({ status: 'unknown' }).asReadonly(),
  };

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [App],
      providers: [provideRouter([]), { provide: AuthService, useValue: authStub }],
    }).compileComponents();
  });

  it('should create the app', () => {
    const fixture = TestBed.createComponent(App);
    const app = fixture.componentInstance;
    expect(app).toBeTruthy();
  });

  it('says "Abrindo a página..." while the first navigation is on its way, then stops', async () => {
    const fixture = TestBed.createComponent(App);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelector('main [role="status"].app-loading')?.textContent).toContain(
      'Abrindo a página',
    );
    await TestBed.inject(Router).navigateByUrl('/');
    fixture.detectChanges();
    expect(el.querySelector('.app-loading')).toBeNull();
  });

  it('renders the app name and the main nav links', () => {
    const fixture = TestBed.createComponent(App);
    fixture.detectChanges();
    const compiled = fixture.nativeElement as HTMLElement;
    const text = compiled.textContent ?? '';
    expect(text).toContain('MeuRPG');
    expect(text).toContain('Início');
    expect(text).toContain('Minhas campanhas');
  });

  it('shows the "Ao vivo" link to the open session, named for screen readers (RN-06)', () => {
    const sessions = signal<readonly OpenSessionVm[]>([
      {
        sessionId: 's4',
        campaignId: 'mirathel',
        campaignName: 'Mirathel',
        sessionNumber: 4,
        startedAt: new Date(),
        isMaster: false,
      },
    ]);
    TestBed.overrideProvider(OpenSessions, {
      useValue: {
        sessions: sessions.asReadonly(),
        dismissed: signal(new Set<string>()).asReadonly(),
        dismiss: () => undefined,
        suspend: () => undefined,
      },
    });
    const fixture = TestBed.createComponent(App);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const links = el.querySelectorAll('a[aria-label="Ao vivo: sessão 4 de Mirathel"]');
    // One in the bar (phone), one before the account (wider screens); CSS
    // shows one at a time.
    expect(links.length).toBe(2);
    expect(links[0].getAttribute('href')).toBe('/campaigns/mirathel/session');
    expect(links[0].textContent).toContain('Ao vivo');
    // The notice is not in a deferred block: it shows as soon as the poll has found the session, and does not wait for idle.
    expect(el.textContent).toContain('A sessão 4 de Mirathel começou.');
  });

  it('draws what a player\'s session page hands the bar (the "Anotações" button), and nothing when it takes it back', () => {
    @Component({
      template: '<ng-template #bar><button class="notes-bar">Anotações</button></ng-template>',
    })
    class Host {
      readonly bar = viewChild.required<TemplateRef<unknown>>('bar');
    }
    const host = TestBed.createComponent(Host);
    host.detectChanges();
    const fixture = TestBed.createComponent(App);
    const notes = TestBed.inject(SessionNotes);
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('.notes-bar')).toBeNull();
    notes.bar.set(host.componentInstance.bar());
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('.notes-bar')?.textContent).toBe('Anotações');
    notes.bar.set(null);
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('.notes-bar')).toBeNull();
  });
});

describe('App, the way into the content (WCAG 2.4.1, 2.4.3)', () => {
  @Component({ template: '<h1>Primeira</h1>' })
  class First {}
  @Component({ template: '<h1>Segunda</h1><button class="own">Meu botão</button>' })
  class Second {}

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [App],
      providers: [
        provideRouter([
          { path: '', component: First },
          { path: 'second', component: Second },
        ]),
        {
          provide: AuthService,
          useValue: { state: signal<AuthState>({ status: 'unknown' }).asReadonly() },
        },
      ],
    }).compileComponents();
  });

  async function start() {
    const fixture = TestBed.createComponent(App);
    const router = TestBed.inject(Router);
    await router.navigateByUrl('/');
    fixture.detectChanges();
    await fixture.whenStable();
    return { fixture, router, el: fixture.nativeElement as HTMLElement };
  }

  it('has a skip link first in the page that moves the focus to the main region, without a navigation', async () => {
    const { fixture, router, el } = await start();
    const skip = el.querySelector<HTMLAnchorElement>('a.skip-link')!;
    expect(skip.textContent).toContain('Pular para o conteúdo');
    expect(el.firstElementChild).toBe(skip);
    skip.click();
    expect(document.activeElement).toBe(el.querySelector('main#main'));
    expect(router.url).toBe('/');
    fixture.destroy();
  });

  it('leaves the focus alone on the first load', async () => {
    const { fixture, el } = await start();
    expect(el.contains(document.activeElement) && document.activeElement !== document.body).toBe(
      false,
    );
    fixture.destroy();
  });

  it('moves the focus to the heading of the new page', async () => {
    const { fixture, router, el } = await start();
    await router.navigateByUrl('/second');
    fixture.detectChanges();
    await fixture.whenStable();
    expect(document.activeElement).toBe(el.querySelector('main h1'));
    expect(document.activeElement?.textContent).toBe('Segunda');
    fixture.destroy();
  });

  it('does not move it for a change that only touches the query', async () => {
    const { fixture, router, el } = await start();
    await router.navigateByUrl('/second');
    fixture.detectChanges();
    await fixture.whenStable();
    el.querySelector<HTMLElement>('.own')!.focus();
    await router.navigateByUrl('/second?filter=a');
    fixture.detectChanges();
    await fixture.whenStable();
    expect(document.activeElement).toBe(el.querySelector('.own'));
    fixture.destroy();
  });

  it('does not take the focus from a dialog that is open', async () => {
    const { fixture, router } = await start();
    const overlay = document.createElement('div');
    overlay.className = 'cdk-overlay-container';
    const inDialog = document.createElement('button');
    overlay.append(inDialog);
    document.body.append(overlay);
    inDialog.focus();
    await router.navigateByUrl('/second');
    fixture.detectChanges();
    await fixture.whenStable();
    expect(document.activeElement).toBe(inDialog);
    overlay.remove();
    fixture.destroy();
  });
});

describe('App on the table screen (the TV tab)', () => {
  @Component({ template: '<h1>Tela</h1>', changeDetection: ChangeDetectionStrategy.OnPush })
  class Tv {}
  @Component({ template: '<h1>Campanha</h1>', changeDetection: ChangeDetectionStrategy.OnPush })
  class Other {}

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [App],
      providers: [
        provideRouter([
          { path: 'campaigns/:id/table-screen', component: Tv },
          { path: 'campaigns/:id', component: Other },
        ]),
        {
          provide: AuthService,
          useValue: { state: signal<AuthState>({ status: 'unknown' }).asReadonly() },
        },
      ],
    }).compileComponents();
  });

  async function openAt(url: string) {
    const fixture = TestBed.createComponent(App);
    await TestBed.inject(Router).navigateByUrl(url);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  it('shows the page and nothing of the shell: no bar, no skip link, no notice, no footer', async () => {
    const { fixture, el } = await openAt('/campaigns/c1/table-screen');
    expect(el.querySelector('main h1')?.textContent).toBe('Tela');
    expect(el.querySelector('header.app-bar')).toBeNull();
    expect(el.querySelector('a.skip-link')).toBeNull();
    expect(el.querySelector('app-live-notice')).toBeNull();
    expect(el.querySelector('app-footer')).toBeNull();
    expect(el.querySelector('app-roll-overlay')).toBeNull();
    expect(el.querySelector('main.app-content--tv')).not.toBeNull();
    fixture.destroy();
  });

  it('asks for no open sessions on the TV tab: its only calls are its own', async () => {
    const fetch = vi.fn(() => Promise.resolve([]));
    TestBed.resetTestingModule();
    await TestBed.configureTestingModule({
      imports: [App],
      providers: [
        provideRouter([
          { path: 'campaigns/:id/table-screen', component: Tv },
          { path: 'campaigns/:id', component: Other },
        ]),
        {
          provide: AuthService,
          useValue: {
            state: signal<AuthState>({
              status: 'signed-in',
              user: { id: 'u1', displayName: null },
              sessionExpiresAt: null,
            }).asReadonly(),
            refresh: () => Promise.resolve(),
          },
        },
        { provide: OPEN_SESSIONS_FETCHER, useValue: () => Promise.resolve(fetch) },
      ],
    }).compileComponents();
    // The tab opens on the TV's address, as a reload of it does.
    await TestBed.inject(Router).navigateByUrl('/campaigns/c1/table-screen');
    const fixture = TestBed.createComponent(App);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(fetch).not.toHaveBeenCalled();
    // Leaving the TV tab for another page starts the poll.
    await TestBed.inject(Router).navigateByUrl('/campaigns/c1');
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    await fixture.whenStable();
    expect(fetch).toHaveBeenCalledTimes(1);
    fixture.destroy();
  });

  it('keeps the shell on the other pages of the campaign', async () => {
    const { fixture, el } = await openAt('/campaigns/c1');
    expect(el.querySelector('header.app-bar')).not.toBeNull();
    expect(el.querySelector('a.skip-link')).not.toBeNull();
    expect(el.querySelector('app-live-notice')).not.toBeNull();
    expect(el.querySelector('main.app-content--tv')).toBeNull();
    fixture.destroy();
  });

  it('brings the shell back when the master leaves the TV tab for another page', async () => {
    const { fixture, el } = await openAt('/campaigns/c1/table-screen');
    await TestBed.inject(Router).navigateByUrl('/campaigns/c1');
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(el.querySelector('header.app-bar')).not.toBeNull();
    fixture.destroy();
  });
});
