import { TestBed } from '@angular/core/testing';
import { describe, expect, it } from 'vitest';

import type { EntryRead as Read } from '../../../core/content/content-read-parts';
import { EntryRead } from './entry-read';

const withSide: Read = {
  rows: [],
  sections: [],
  text: ['Escolha um estilo.'],
  options: [
    { title: 'Guarda Alta', text: 'Você mantém a lâmina erguida.', side: 'Classe de Armadura +1' },
    {
      title: 'Mão Firme',
      text: 'Seu golpe cai no mesmo ponto.',
      side: 'Dano de arma corpo a corpo +1',
    },
  ],
};

function render(read: Read, plain = false): HTMLElement {
  const fixture = TestBed.createComponent(EntryRead);
  fixture.componentRef.setInput('read', read);
  fixture.componentRef.setInput('plain', plain);
  fixture.detectChanges();
  return fixture.nativeElement;
}

describe('EntryRead options', () => {
  it('draws one row for each option, the name and what it does, under an "Opções" heading inside the text', () => {
    const el = render(withSide);
    expect(el.querySelector('section[aria-label="Texto"] h2')?.textContent).toBe('Opções');
    const rows = Array.from(el.querySelectorAll('.opts li')).map((li) => [
      li.querySelector('strong')?.textContent,
      li.querySelector('.opts__side')?.textContent,
    ]);
    expect(rows).toEqual([
      ['Guarda Alta', 'Classe de Armadura +1'],
      ['Mão Firme', 'Dano de arma corpo a corpo +1'],
    ]);
    expect(el.textContent).not.toContain('Você mantém a lâmina erguida.');
  });

  it('keeps the full texts in a preview, and for a reader with no words for them (a player)', () => {
    for (const el of [
      render(withSide, true),
      render({ ...withSide, options: withSide.options!.map((o) => ({ ...o, side: undefined })) }),
    ]) {
      expect(el.querySelector('.opts')).toBeNull();
      expect(el.querySelector('ul[aria-label="Opções"]')?.textContent).toContain(
        'Você mantém a lâmina erguida.',
      );
    }
  });
});
