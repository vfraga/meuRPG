import { computed, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import { LiveNotice } from './live-notice';
import { OpenSessions } from '../../core/play/open-sessions';
import type { OpenSessionVm } from '../../core/play/open-sessions.types';

describe('LiveNotice', () => {
  const sessions = signal<readonly OpenSessionVm[]>([]);
  const dismissed = signal<ReadonlySet<string>>(new Set());
  const store = {
    sessions: sessions.asReadonly(),
    dismissed: dismissed.asReadonly(),
    liveCampaignIds: computed(() => new Set(sessions().map((s) => s.campaignId))),
    dismiss: (id: string) => dismissed.update((ids) => new Set(ids).add(id)),
  };

  beforeEach(() => {
    sessions.set([
      {
        sessionId: 's4',
        campaignId: 'mirathel',
        campaignName: 'Mirathel',
        sessionNumber: 4,
        startedAt: new Date(),
        isMaster: false,
      },
    ]);
    dismissed.set(new Set());
    TestBed.configureTestingModule({
      imports: [LiveNotice],
      providers: [provideRouter([]), { provide: OpenSessions, useValue: store }],
    });
  });

  function render(url: string): HTMLElement {
    const fixture = TestBed.createComponent(LiveNotice);
    fixture.componentRef.setInput('url', url);
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }

  it('says which session started, with "Entrar na sessão" to its page (E5-01)', () => {
    const el = render('/campaigns');
    expect(el.querySelector('[role="status"]')?.textContent).toContain(
      'A sessão 4 de Mirathel começou.',
    );
    const link = el.querySelector('a');
    expect(link?.textContent?.trim()).toBe('Entrar na sessão');
    expect(link?.getAttribute('href')).toBe('/campaigns/mirathel/session');
    expect(el.querySelector('button[aria-label="Fechar aviso"]')).not.toBeNull();
  });

  it('"Fechar aviso" hides it for this tab', () => {
    const fixture = TestBed.createComponent(LiveNotice);
    fixture.componentRef.setInput('url', '/');
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    el.querySelector<HTMLButtonElement>('button[aria-label="Fechar aviso"]')!.click();
    fixture.detectChanges();
    expect(el.textContent).not.toContain('começou');
    expect(dismissed().has('s4')).toBe(true);
  });

  it('shows one notice per session, the older one first, and a new session goes below', () => {
    const fixture = TestBed.createComponent(LiveNotice);
    fixture.componentRef.setInput('url', '/');
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const first = el.querySelector('.live-notice');

    // A session of another campaign starts (the list is newest first).
    sessions.update((list) => [
      {
        sessionId: 's9',
        campaignId: 'estrada',
        campaignName: 'Estrada de Ossos',
        sessionNumber: 2,
        startedAt: new Date(),
        isMaster: false,
      },
      ...list,
    ]);
    fixture.detectChanges();

    const notices = Array.from(el.querySelectorAll('.live-notice'));
    expect(notices.map((n) => n.textContent)).toEqual([
      expect.stringContaining('A sessão 4 de Mirathel começou.'),
      expect.stringContaining('A sessão 2 de Estrada de Ossos começou.'),
    ]);
    // The first notice is the same element: nothing changed under a finger.
    expect(notices[0]).toBe(first);
  });

  it("stays away from the campaign's own page and from session pages", () => {
    expect(render('/campaigns/mirathel').textContent).not.toContain('começou');
    expect(render('/campaigns/mirathel/session').textContent).not.toContain('começou');
  });
});
