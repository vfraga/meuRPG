import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { describe, expect, it, vi } from 'vitest';

import { LiveSessionSource } from '../live-session.types';
import { SessionHeader } from './session-header';

describe('SessionHeader ending the session', () => {
  function setup() {
    let finish!: () => void;
    const endSession = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve;
        }),
    );
    TestBed.configureTestingModule({
      providers: [provideRouter([]), { provide: LiveSessionSource, useValue: { endSession } }],
    });
    const fixture = TestBed.createComponent(SessionHeader);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('campaignName', 'Torre');
    fixture.componentRef.setInput('isMaster', true);
    fixture.componentRef.setInput('session', {
      sessionId: 's1',
      sessionNumber: 4,
      startedAt: new Date(),
    });
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const button = (text: string) =>
      Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.includes(text));
    return { fixture, el, button, endSession, finish: () => finish() };
  }

  it('control: Cancelar goes back to "Encerrar sessão" before the master confirms', () => {
    const { fixture, button } = setup();
    button('Encerrar sessão')!.click();
    fixture.detectChanges();
    button('Cancelar')!.click();
    fixture.detectChanges();
    expect(button('Encerrar sessão')).toBeDefined();
  });

  it('keeps the confirmation, and Cancelar dead, while the end is on its way', async () => {
    const { fixture, button, endSession, finish } = setup();
    button('Encerrar sessão')!.click();
    fixture.detectChanges();
    button('Confirmar encerramento')!.click();
    fixture.detectChanges();
    expect(endSession).toHaveBeenCalledTimes(1);
    const cancel = button('Cancelar')!;
    expect(cancel.disabled).toBe(true);
    cancel.dispatchEvent(new Event('click'));
    fixture.detectChanges();
    expect(button('Encerrar sessão')).toBeUndefined();
    expect(button('Confirmar encerramento')).toBeDefined();
    finish();
    await fixture.whenStable();
  });
});
