import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { ConnectError, Code } from '@connectrpc/connect';

import { AuthService, AuthState } from '../../core/auth/auth.service';
import { ServerInfoService } from '../../core/system/server-info.service';
import { Home } from './home';

describe('Home', () => {
  function setup(
    serverInfo: Partial<ServerInfoService>,
    auth: AuthState = { status: 'signed-out' },
  ) {
    TestBed.configureTestingModule({
      imports: [Home],
      providers: [
        provideRouter([]),
        { provide: ServerInfoService, useValue: serverInfo },
        { provide: AuthService, useValue: { state: signal(auth) } },
      ],
    });
    const fixture = TestBed.createComponent(Home);
    fixture.detectChanges();
    return fixture;
  }

  const serverUp = {
    getServerInfo: () => Promise.resolve({ version: 'v0.1.0', commit: 'abc123' } as never),
  };

  async function render(auth?: AuthState): Promise<HTMLElement> {
    const fixture = setup(serverUp, auth);
    await fixture.whenStable();
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }

  it('shows the version and commit once the call resolves', async () => {
    const fixture = setup({
      getServerInfo: () => Promise.resolve({ version: 'v0.1.0', commit: 'abc123' } as never),
    });
    await fixture.whenStable();
    fixture.detectChanges();

    const el = fixture.nativeElement as HTMLElement;
    const text = el.textContent ?? '';
    expect(text).toContain('Servidor conectado');
    expect(text).toContain('v0.1.0');
    expect(text).toContain('abc123');
    // The smoke test reads the version as the <dd> right after "Versão".
    const versionTerm = Array.from(el.querySelectorAll('dt')).find(
      (dt) => dt.textContent === 'Versão',
    );
    expect(versionTerm?.nextElementSibling?.textContent).toBe('v0.1.0');
  });

  it('shows a clear error state when the call fails', async () => {
    const fixture = setup({
      getServerInfo: () => Promise.reject(new ConnectError('internal detail', Code.Unavailable)),
    });
    await fixture.whenStable();
    fixture.detectChanges();

    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';
    expect(text).toContain('Não foi possível falar com o servidor.');
    expect(text).toContain('Tente de novo em instantes.');
    expect(text).not.toContain('internal detail');
    expect(text).not.toContain('[unavailable]');
    expect(text).toContain('Tentar de novo');
  });

  it('welcomes a visitor with what MeuRPG is and one "Entrar" link to the sign-in', async () => {
    const el = await render({ status: 'signed-out' });

    expect(el.querySelectorAll('h1').length).toBe(1);
    expect(el.textContent).toContain('ficha');
    const entrar = Array.from(el.querySelectorAll('a')).find(
      (a) => a.textContent?.trim() === 'Entrar',
    );
    expect(entrar?.getAttribute('href')).toBe('/auth/login?return_to=%2Fcampaigns');
    // A link, so it never doubles the app bar's "Entrar" button.
    expect(
      Array.from(el.querySelectorAll('button')).some((b) => b.textContent?.includes('Entrar')),
    ).toBe(false);
  });

  it('greets someone signed in by name, with the way to "Minhas campanhas"', async () => {
    const el = await render({
      status: 'signed-in',
      user: { id: 'u1', displayName: 'Vinicius' },
      sessionExpiresAt: null,
    });

    expect(el.querySelector('h1')?.textContent).toContain('Olá, Vinicius');
    const links = Array.from(el.querySelectorAll('a'));
    expect(links.some((a) => a.getAttribute('href') === '/campaigns')).toBe(true);
    expect(links.some((a) => a.getAttribute('href') === '/profile')).toBe(false);
    expect(links.some((a) => a.textContent?.trim() === 'Entrar')).toBe(false);
  });

  it('points someone with no display name to "Meu perfil"', async () => {
    const el = await render({
      status: 'signed-in',
      user: { id: 'u1', displayName: null },
      sessionExpiresAt: null,
    });

    expect(el.querySelector('h1')?.textContent?.trim()).toBe('Olá');
    const links = Array.from(el.querySelectorAll('a'));
    expect(links.some((a) => a.getAttribute('href') === '/profile')).toBe(true);
  });
});
