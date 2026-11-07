import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';

import { TvChoiceRow } from './tv-choice-row';

function make(inputs: Record<string, unknown> = {}) {
  const fixture = TestBed.createComponent(TvChoiceRow);
  const base = { group: 'g', icon: 'map', title: 'Só o mapa', sub: 'Cripta do Corvo' };
  for (const [key, value] of Object.entries({ ...base, ...inputs })) {
    fixture.componentRef.setInput(key, value);
  }
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  return { fixture, el, radio: el.querySelector<HTMLInputElement>('input[type="radio"]')! };
}

describe('TvChoiceRow', () => {
  it('is a radio button named by its title and subtitle', () => {
    const { el, radio } = make();
    expect(radio.name).toBe('g');
    expect(radio.checked).toBe(false);
    expect(el.querySelector('label')?.textContent).toContain('Só o mapa');
    expect(el.querySelector('label')?.textContent).toContain('Cripta do Corvo');
  });

  it('marks the chosen row, and tells the page when it is picked', () => {
    const { fixture, el, radio } = make({ selected: true });
    expect(radio.checked).toBe(true);
    expect(el.querySelector('.row--on')).not.toBeNull();
    const picked = vi.fn();
    fixture.componentInstance.picked.subscribe(picked);
    radio.dispatchEvent(new Event('change'));
    expect(picked).toHaveBeenCalledTimes(1);
  });

  it('is dashed and cannot be picked when it is not available', () => {
    const { el, radio } = make({ available: false, sub: 'Nenhum combate agora.' });
    expect(radio.disabled).toBe(true);
    expect(el.querySelector('.row--off')).not.toBeNull();
    expect(el.textContent).toContain('Nenhum combate agora.');
  });

  it('cannot be picked while a choice is on its way', () => {
    const { radio } = make({ busy: true });
    expect(radio.disabled).toBe(true);
  });
});
