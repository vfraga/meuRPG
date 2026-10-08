import { ComponentFixture, TestBed } from '@angular/core/testing';

import type { ShownImageVm } from '../live-session.types';
import { ShownImageBlock } from './shown-image-block';

const goblin: ShownImageVm = {
  id: 'g',
  name: 'Capitão Goblin',
  width: 400,
  height: 500,
  url: '/images/g',
};
const tavern: ShownImageVm = {
  id: 't',
  name: 'Taverna do Javali',
  width: 800,
  height: 500,
  url: '/images/t',
};

describe('ShownImageBlock', () => {
  let fixture: ComponentFixture<ShownImageBlock>;
  let el: HTMLElement;
  let reduced = false;

  beforeEach(() => {
    vi.useFakeTimers();
    reduced = false;
    vi.stubGlobal('matchMedia', (query: string) => ({
      matches: query.includes('reduce') ? reduced : false,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
    }));
    fixture = TestBed.createComponent(ShownImageBlock);
    el = fixture.nativeElement;
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  function show(image: ShownImageVm | null): void {
    fixture.componentRef.setInput('image', image);
    fixture.detectChanges();
  }

  it('draws nothing while no image is shown', () => {
    show(null);
    expect(el.querySelector('section')).toBeNull();
  });

  it('shows the heading, the image with its name as alt, the caption and the live note', () => {
    show(goblin);
    expect(el.querySelector('h2')?.textContent).toContain('O mestre está mostrando');
    const img = el.querySelector('img') as HTMLImageElement;
    expect(img.alt).toBe('Capitão Goblin');
    expect(img.getAttribute('width')).toBe('400');
    expect(el.querySelector('.block__name')?.textContent).toBe('Capitão Goblin');
    expect(el.textContent).toContain('Fica aqui enquanto o mestre mostrar.');
    expect(
      el.querySelector('button[aria-label="Ver Capitão Goblin em tela cheia"]'),
    ).not.toBeNull();
  });

  it('reserves the frame from the image shape before its bytes arrive', () => {
    show(goblin);
    expect((el.querySelector('.frame') as HTMLElement).style.aspectRatio).toBe('400 / 500');
  });

  it('swaps in place, with no leaving state', () => {
    show(goblin);
    show(tavern);
    expect(el.querySelector('.block__name')?.textContent).toBe('Taverna do Javali');
    expect(el.querySelector('.wrap--out')).toBeNull();
  });

  it('fades out and collapses, then leaves the DOM', () => {
    show(goblin);
    show(null);
    expect(el.querySelector('.wrap--out')).not.toBeNull();
    expect(el.querySelector('section')).not.toBeNull();
    vi.advanceTimersByTime(400);
    fixture.detectChanges();
    expect(el.querySelector('section')).toBeNull();
  });

  it('disappears at once with reduced motion', () => {
    reduced = true;
    show(goblin);
    show(null);
    expect(el.querySelector('section')).toBeNull();
  });

  it('says the image failed to load, and tries again', () => {
    show(goblin);
    (el.querySelector('img') as HTMLImageElement).dispatchEvent(new Event('error'));
    fixture.detectChanges();
    expect(el.textContent).toContain('Não deu para carregar a imagem.');
    (
      Array.from(el.querySelectorAll('button')).find((b) =>
        b.textContent?.includes('Tentar de novo'),
      ) as HTMLButtonElement
    ).click();
    fixture.detectChanges();
    expect(el.querySelector('img')).not.toBeNull();
    expect(el.textContent).not.toContain('Não deu para carregar a imagem.');
  });

  it('tries the picture again when the master shows it anew while the failed one is leaving', () => {
    show(goblin);
    (el.querySelector('img') as HTMLImageElement).dispatchEvent(new Event('error'));
    fixture.detectChanges();
    show(null);
    expect(el.textContent).toContain('Não deu para carregar a imagem.');
    show(goblin);
    expect(el.querySelector('img')).not.toBeNull();
    expect(el.textContent).not.toContain('Não deu para carregar a imagem.');
  });
});
