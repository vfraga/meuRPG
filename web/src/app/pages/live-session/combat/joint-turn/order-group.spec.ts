import { TestBed } from '@angular/core/testing';

import { OrderGroup } from './order-group';

describe('OrderGroup', () => {
  function setup(inputs: Record<string, unknown>, host = '') {
    const fixture = TestBed.createComponent(OrderGroup);
    for (const [k, v] of Object.entries({ total: 17, ...inputs })) {
      fixture.componentRef.setInput(k, v);
    }
    if (host) {
      fixture.nativeElement.classList.add(host);
    }
    document.body.appendChild(fixture.nativeElement);
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }

  afterEach(() => document.body.replaceChildren());

  it('reads "Montado · iniciativa 17" with one separator style, and keeps the "Vez" pill on the head line', () => {
    const el = setup({ mounted: true, onTurn: true });
    const parts = Array.from(el.querySelectorAll('.part')).map((p) => p.textContent?.trim());
    expect(parts).toEqual(['· iniciativa 17']);
    expect(el.querySelector('b')?.textContent).toBe('Montado');
    const vez = getComputedStyle(el.querySelector('.vez')!);
    expect(vez.flexShrink).toBe('0');
  });

  it('is a rounded frame inside the master list, not a band across it', () => {
    const el = setup({ mounted: true }, 'bleed');
    const box = getComputedStyle(el.querySelector('.box')!);
    expect(box.marginLeft).toBe('0px');
  });
});
