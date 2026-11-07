import { TestBed } from '@angular/core/testing';

import { TvFogCard } from './tv-fog-card';

describe('TvFogCard', () => {
  it('says the map is on each one’s own screen, and why', () => {
    const fixture = TestBed.createComponent(TvFogCard);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelector('h2')?.textContent).toBe('O mapa fica na tela de cada um.');
    expect(el.textContent).toContain(
      'A névoa está ligada e cada jogador vê só o que o próprio personagem vê.',
    );
    expect(el.querySelector('section')?.getAttribute('aria-labelledby')).toBe('tv-fog-title');
  });
});
