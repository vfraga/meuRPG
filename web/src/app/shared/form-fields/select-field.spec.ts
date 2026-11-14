import { ComponentFixture, TestBed } from '@angular/core/testing';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { SelectField, type SelectOption } from './select-field';

const OPTIONS: SelectOption[] = [
  { value: 'a', label: 'Valor · Habilidade', side: 'do efeito 1', group: 'A escolha do talento' },
  { value: 'b', label: 'Força', group: 'Valores fixos' },
];

function setup(baseSelect: boolean): ComponentFixture<SelectField<string | number>> {
  vi.stubGlobal('CSS', {
    supports: (p: string, v: string) => baseSelect && p === 'appearance' && v === 'base-select',
  });
  const fixture = TestBed.createComponent(SelectField);
  fixture.componentRef.setInput('label', 'Alvo');
  fixture.componentRef.setInput('options', OPTIONS);
  fixture.componentRef.setInput('value', '');
  fixture.componentRef.setInput('groups', {
    'A escolha do talento': { icon: 'alt_route', accent: true },
  });
  fixture.componentRef.setInput('footer', 'A escolha aparece depois.');
  fixture.detectChanges();
  return fixture;
}

describe('SelectField with a customizable list', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('draws the group header with its icon and keeps the side text apart', () => {
    const el: HTMLElement = setup(true).nativeElement;
    expect(el.querySelector('select.rich')).not.toBeNull();
    expect(el.querySelector('optgroup legend mat-icon')?.textContent?.trim()).toBe('alt_route');
    const option = el.querySelector('option[data-side]')!;
    expect(option.getAttribute('data-side')).toBe('do efeito 1');
    expect(option.hasAttribute('label')).toBe(false);
    expect(option.textContent?.trim()).toBe('Valor · Habilidade');
  });
});

describe('SelectField in a browser that draws only text', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('puts the side text in the option label, and no legend or icon text in the group', () => {
    const el: HTMLElement = setup(false).nativeElement;
    expect(el.querySelector('select.rich')).toBeNull();
    expect(el.querySelector('legend')).toBeNull();
    expect(el.querySelector('optgroup mat-icon')).toBeNull();
    expect(el.querySelector('optgroup')?.getAttribute('label')).toBe('A escolha do talento');
    expect(el.querySelector('option[value="a"]')?.getAttribute('label')).toBe(
      'Valor · Habilidade · do efeito 1',
    );
    expect(el.querySelector('option[value="b"]')?.hasAttribute('label')).toBe(false);
    expect(el.querySelector('option.footer')?.textContent?.trim()).toBe(
      'A escolha aparece depois.',
    );
  });

  it('is the plain list too when the browser has no CSS object at all', () => {
    vi.stubGlobal('CSS', undefined);
    const fixture = TestBed.createComponent(SelectField);
    fixture.componentRef.setInput('label', 'Alvo');
    fixture.componentRef.setInput('options', OPTIONS);
    fixture.componentRef.setInput('value', '');
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('legend')).toBeNull();
  });
});

describe('SelectField with an option that cannot be picked and says why', () => {
  afterEach(() => vi.unstubAllGlobals());

  function withReason(baseSelect: boolean): HTMLElement {
    vi.stubGlobal('CSS', {
      supports: (p: string, v: string) => baseSelect && p === 'appearance' && v === 'base-select',
    });
    const fixture = TestBed.createComponent(SelectField);
    fixture.componentRef.setInput('label', 'Efeito');
    fixture.componentRef.setInput('options', [
      { value: 'a', label: 'Dano extra' },
      {
        value: 'b',
        label: 'Ataque poderoso',
        disabled: true,
        reason: 'Este talento já tem um ataque poderoso.',
      },
    ]);
    fixture.componentRef.setInput('value', 'a');
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }

  it('draws the reason under the name where the list is ours, as an attribute the style reads', () => {
    const el = withReason(true);
    const option = el.querySelector<HTMLOptionElement>('option[value="b"]')!;
    expect(el.querySelector('select.rich')).not.toBeNull();
    expect(option.disabled).toBe(true);
    expect(option.getAttribute('data-reason')).toBe('Este talento já tem um ataque poderoso.');
    expect(option.hasAttribute('label')).toBe(false);
    expect(el.querySelector('option[value="a"]')!.hasAttribute('data-reason')).toBe(false);
  });

  it('reads "name · reason" in a list that draws only text', () => {
    const el = withReason(false);
    const option = el.querySelector<HTMLOptionElement>('option[value="b"]')!;
    expect(el.querySelector('select.rich')).toBeNull();
    expect(option.disabled).toBe(true);
    expect(option.getAttribute('label')).toBe(
      'Ataque poderoso · Este talento já tem um ataque poderoso.',
    );
  });
});
