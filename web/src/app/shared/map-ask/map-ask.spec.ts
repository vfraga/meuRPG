import { Component } from '@angular/core';
import { TestBed } from '@angular/core/testing';

import { MapAsk } from './map-ask';

@Component({
  imports: [MapAsk],
  template: `
    <app-map-ask title="Mudar a grade?" confirmLabel="Apagar e mudar a grade" [ready]="ready" (cancel)="events.push('cancel')" (confirm)="events.push('confirm')">
      <p>O que vai embora.</p>
    </app-map-ask>
  `,
})
class Host {
  ready = true;
  events: string[] = [];
}

describe('MapAsk: an in-place question', () => {
  async function open(ready = true) {
    TestBed.configureTestingModule({});
    const fixture = TestBed.createComponent(Host);
    fixture.componentInstance.ready = ready;
    fixture.detectChanges();
    await fixture.whenStable();
    return { fixture, el: fixture.nativeElement as HTMLElement, host: fixture.componentInstance };
  }

  const button = (el: HTMLElement, text: string) =>
    Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.trim() === text)!;

  it('puts the focus on its title, with the ring', async () => {
    const { el } = await open();
    const title = el.querySelector('h3')!;
    expect(title.textContent?.trim()).toBe('Mudar a grade?');
    expect(document.activeElement).toBe(title);
    expect(title.hasAttribute('data-ring')).toBe(true);
  });

  it('has "Voltar" first and the filled button after it, and nothing happens before the second click', async () => {
    const { el, host } = await open();
    const buttons = Array.from(el.querySelectorAll('button')).map((b) => b.textContent?.trim());
    expect(buttons).toEqual(['Voltar', 'Apagar e mudar a grade']);
    expect(host.events).toEqual([]);
    button(el, 'Voltar').click();
    button(el, 'Apagar e mudar a grade').click();
    expect(host.events).toEqual(['cancel', 'confirm']);
  });

  it('closes on Esc', async () => {
    const { el, host } = await open();
    el.querySelector('section')!.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }),
    );
    expect(host.events).toEqual(['cancel']);
  });

  it('draws the dashed button and does not confirm while it cannot act', async () => {
    const { el, host } = await open(false);
    const go = button(el, 'Apagar e mudar a grade');
    expect(go.classList).toContain('mr-button--off');
    expect(go.getAttribute('aria-disabled')).toBe('true');
    go.click();
    expect(host.events).toEqual([]);
  });
});
