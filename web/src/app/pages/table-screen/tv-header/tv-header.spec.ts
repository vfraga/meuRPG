import { TestBed } from '@angular/core/testing';

import { TvHeader, type TvLink } from './tv-header';

function render(link: TvLink, kicker = 'Mirathel · Sessão 3') {
  const fixture = TestBed.createComponent(TvHeader);
  fixture.componentRef.setInput('icon', 'swords');
  fixture.componentRef.setInput('kicker', kicker);
  fixture.componentRef.setInput('title', 'Combate · Cripta do Corvo');
  fixture.componentRef.setInput('link', link);
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

describe('TvHeader', () => {
  it('has the title as the page heading, with the kicker above it', () => {
    const el = render('live');
    expect(el.querySelector('h1')?.textContent).toBe('Combate · Cripta do Corvo');
    expect(el.querySelector('.tv-head__kicker')?.textContent).toBe('Mirathel · Sessão 3');
  });

  it('leaves out an empty kicker', () => {
    expect(render('live', '').querySelector('.tv-head__kicker')).toBeNull();
  });

  it('says "Ao vivo" while connected', () => {
    expect(render('live').querySelector('app-live-pill')?.textContent).toContain('Ao vivo');
  });

  it('says "Reconectando…" when late, with words and not colour alone', () => {
    const el = render('late');
    expect(el.querySelector('[data-testid="tv-link-late"]')?.textContent).toContain(
      'Reconectando…',
    );
    expect(el.querySelector('app-live-pill')).toBeNull();
  });

  it('says "Sem conexão" when lost', () => {
    expect(render('lost').querySelector('[data-testid="tv-link-lost"]')?.textContent).toContain(
      'Sem conexão',
    );
  });

  it('says "Encerrada às 23:40" when the session ended, and just "Encerrada" without the hour', () => {
    const fixture = TestBed.createComponent(TvHeader);
    fixture.componentRef.setInput('icon', 'flag');
    fixture.componentRef.setInput('title', 'Sessão 3');
    fixture.componentRef.setInput('link', 'ended');
    fixture.componentRef.setInput('endedAt', '23:40');
    fixture.detectChanges();
    const chip = () => fixture.nativeElement.querySelector('[data-testid="tv-link-ended"]');
    expect(chip().textContent).toContain('Encerrada às 23:40');
    fixture.componentRef.setInput('endedAt', '');
    fixture.detectChanges();
    expect(chip().textContent.trim()).toContain('Encerrada');
    expect(chip().textContent).not.toContain('às');
  });

  it('draws no chip without a link', () => {
    const el = render(null);
    expect(el.querySelector('.tv-head__chip')).toBeNull();
  });
});
