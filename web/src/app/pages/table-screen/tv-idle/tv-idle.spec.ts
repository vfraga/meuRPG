import { TestBed } from '@angular/core/testing';

import { TV_PARTY_MAX, TvIdle } from './tv-idle';

function render(party: string[]) {
  const fixture = TestBed.createComponent(TvIdle);
  fixture.componentRef.setInput('party', party);
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

describe('TvIdle', () => {
  it('says there is nothing to show yet and who is at the table', () => {
    const el = render(['Tor', 'Brisa']);
    expect(el.querySelector('h2')?.textContent).toBe('Nada à mostra ainda');
    expect(el.textContent).toContain('O que o mestre mostrar à mesa aparece aqui.');
    const names = [...el.querySelectorAll('.tv-idle__name')].map((n) => n.textContent);
    expect(names).toEqual(['Tor', 'Brisa']);
    expect(el.querySelector('ul')?.getAttribute('aria-labelledby')).toBe('tv-idle-party');
  });

  it('leaves out "Na mesa" when nobody is there', () => {
    const el = render([]);
    expect(el.querySelector('ul')).toBeNull();
    expect(el.textContent).not.toContain('Na mesa');
  });

  it('cuts a long party with "+N"', () => {
    const party = Array.from({ length: TV_PARTY_MAX + 3 }, (_, i) => `P${i}`);
    const el = render(party);
    expect(el.querySelectorAll('.tv-idle__name')).toHaveLength(TV_PARTY_MAX);
    expect(el.querySelector('.tv-idle__more')?.textContent).toBe('+3');
  });
});
