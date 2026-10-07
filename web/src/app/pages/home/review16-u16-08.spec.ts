// Finding U16-08 in review/unit-16-web-content-campaigns.md
import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';

import { AuthService } from '../../core/auth/auth.service';
import { ServerInfoService } from '../../core/system/server-info.service';
import { Home } from './home';

describe('Review16 U16-08: Home error state leaks raw transport text', () => {
  it('does not show the raw ConnectError message when loading fails', async () => {
    TestBed.configureTestingModule({
      imports: [Home],
      providers: [
        provideRouter([]),
        {
          provide: ServerInfoService,
          useValue: {
            getServerInfo: () =>
              Promise.reject(new ConnectError('internal detail', Code.Unavailable)),
          },
        },
        { provide: AuthService, useValue: { state: signal({ status: 'signed-out' }) } },
      ],
    });
    const fixture = TestBed.createComponent(Home);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();

    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';
    expect(text).toContain('Tentar de novo');
    expect(text).not.toContain('internal detail');
    expect(text).not.toContain('[unavailable]');
  });
});
