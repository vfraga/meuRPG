import { TestBed } from '@angular/core/testing';

import { MultiRoll, type RollField } from './multi-roll';

const d20: readonly RollField[] = [
  { key: 'a', label: 'Primeiro d20', min: 1, max: 20 },
  { key: 'b', label: 'Segundo d20', min: 1, max: 20 },
];

function setup(inputs: Record<string, unknown> = {}) {
  const fixture = TestBed.createComponent(MultiRoll);
  fixture.componentRef.setInput('fields', d20);
  for (const [k, v] of Object.entries(inputs)) {
    fixture.componentRef.setInput(k, v);
  }
  const typed: number[][] = [];
  const app: number[] = [];
  fixture.componentInstance.typed.subscribe((v) => typed.push(v));
  fixture.componentInstance.app.subscribe(() => app.push(1));
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const button = (name: string) =>
    Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.includes(name))!;
  const type = (i: number, value: string) => {
    const input = el.querySelectorAll('input')[i];
    input.value = value;
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
  };
  return { fixture, el, typed, app, button, type };
}

describe('MultiRoll', () => {
  it('offers the app roll and the typed one', () => {
    const { el, button, app } = setup();
    expect(button('Rolar no app')).toBeTruthy();
    button('Rolar no app').click();
    expect(app).toHaveLength(1);
    expect(el.querySelectorAll('input')).toHaveLength(0);
  });

  it('opens one labelled field for each number, the first focused', async () => {
    const { fixture, el, button } = setup();
    button('Digitar o resultado').click();
    fixture.detectChanges();
    await fixture.whenStable();
    const labels = Array.from(el.querySelectorAll('label')).map((l) => l.textContent?.trim());
    expect(labels).toEqual(['Primeiro d20', 'Segundo d20']);
    expect(document.activeElement).toBe(el.querySelectorAll('input')[0]);
  });

  it('keeps Confirmar off until both numbers are valid, then sends them in order', () => {
    const { fixture, el, button, type, typed } = setup({ canApp: false });
    expect(button('Confirmar').getAttribute('aria-disabled')).toBe('true');
    type(0, '14');
    expect(button('Confirmar').getAttribute('aria-disabled')).toBe('true');
    type(1, '7');
    expect(button('Confirmar').getAttribute('aria-disabled')).toBeNull();
    el.querySelector('form')!.dispatchEvent(new Event('submit'));
    fixture.detectChanges();
    expect(typed).toEqual([[14, 7]]);
  });

  it('says in words which number is out of range', () => {
    const { el, type } = setup({ canApp: false });
    type(0, '21');
    expect(el.textContent).toContain('Digite um número de 1 a 20');
    expect(el.querySelectorAll('input')[0].getAttribute('aria-invalid')).toBe('true');
  });

  it('adds the bonus to the higher d20 for advantage and to the lower for disadvantage', () => {
    const high = setup({
      canApp: false,
      combine: 'higher',
      modifier: 5,
      totalNote: 'Total do ataque',
    });
    high.type(0, '14');
    high.type(1, '7');
    expect(high.el.querySelector('.type__num')!.textContent).toBe('19');
    const low = setup({
      canApp: false,
      combine: 'lower',
      modifier: 5,
      totalNote: 'Total do ataque',
    });
    low.type(0, '14');
    low.type(1, '7');
    expect(low.el.querySelector('.type__num')!.textContent).toBe('12');
  });

  it('adds up the groups of dice of a damage', () => {
    const { el, type } = setup({
      canApp: false,
      fields: [
        { key: 'w', label: 'Espada: 1d8', min: 1, max: 8 },
        { key: 's', label: 'Furtivo: 2d6', min: 2, max: 12 },
      ],
      modifier: 3,
      totalNote: 'Dano total',
    });
    type(0, '5');
    type(1, '7');
    expect(el.querySelector('.type__num')!.textContent).toBe('15');
  });
});
