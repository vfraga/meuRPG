import { TestBed } from '@angular/core/testing';

import { TvImage } from './tv-image';

function render(inputs: Record<string, unknown>) {
  const fixture = TestBed.createComponent(TvImage);
  Object.entries(inputs).forEach(([k, v]) => fixture.componentRef.setInput(k, v));
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

describe('TvImage', () => {
  it('shows the picture with its name as the alternative text and the caption', () => {
    const el = render({ name: 'A carta de Bartolo', url: '/images/i1', width: 800, height: 600 });
    const img = el.querySelector('img')!;
    expect(img.getAttribute('src')).toBe('/images/i1');
    expect(img.getAttribute('alt')).toBe('A carta de Bartolo');
    expect(img.getAttribute('width')).toBe('800');
    expect(el.querySelector('figcaption')?.textContent).toContain('A carta de Bartolo');
  });

  it('has a plain alternative text and no caption when the server sends no name', () => {
    const el = render({ url: '/images/i1' });
    expect(el.querySelector('img')?.getAttribute('alt')).toBe('Imagem mostrada pelo mestre');
    expect(el.querySelector('figcaption')).toBeNull();
    expect(el.querySelector('img')?.hasAttribute('width')).toBe(false);
  });
});
