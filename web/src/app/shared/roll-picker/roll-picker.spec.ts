import { TestBed } from '@angular/core/testing';

import { RollPicker } from './roll-picker';

describe('RollPicker', () => {
  function setup(inputs: Record<string, unknown> = {}) {
    const fixture = TestBed.createComponent(RollPicker);
    fixture.componentRef.setInput('label', 'Role 1d20 para o Machado de batalha (+5)');
    for (const [k, v] of Object.entries(inputs)) {
      fixture.componentRef.setInput(k, v);
    }
    const rolled: string[] = [];
    fixture.componentInstance.app.subscribe(() => rolled.push('app'));
    fixture.componentInstance.typed.subscribe((n) => rolled.push(`typed ${n}`));
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const type = (text: string) => {
      const field = el.querySelector<HTMLInputElement>('input')!;
      field.value = text;
      field.dispatchEvent(new Event('input'));
      fixture.detectChanges();
    };
    const button = (name: string) =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        b.textContent?.includes(name),
      );
    return { fixture, el, rolled, type, button };
  }

  it('offers both ways while the campaign lets each player choose, the saved one as the filled button', () => {
    const { el } = setup();
    const buttons = Array.from(el.querySelectorAll('button'), (b) => b.textContent?.trim());
    expect(buttons).toEqual(['casinoRolar no app', 'Digitar o resultado']);
    const [app, type] = Array.from(el.querySelectorAll('button'));
    expect(app.classList).not.toContain('pick__main--quiet');
    expect(type.classList).toContain('pick__main--quiet');
    const physical = Array.from(setup({ preferApp: false }).el.querySelectorAll('button'));
    expect(physical[0].classList).toContain('pick__main--quiet');
    expect(physical[1].classList).not.toContain('pick__main--quiet');
  });

  it('hides the way a forced mode does not allow', () => {
    expect(
      Array.from(setup({ canType: false }).el.querySelectorAll('button'), (b) =>
        b.textContent?.trim(),
      ),
    ).toEqual(['casinoRolar no app']);
    const typeOnly = setup({ canApp: false }).el;
    expect(typeOnly.querySelector('input')).not.toBeNull();
    expect(typeOnly.textContent).not.toContain('Rolar no app');
  });

  it('reads the total on the button, not the typed sum, when it confirms a damage ("Confirmar 25")', () => {
    const { type, button, rolled } = setup({
      min: 3,
      max: 36,
      modifier: 3,
      confirmTotal: true,
      canApp: false,
    });
    type('22');
    expect(button('Confirmar 25')).toBeTruthy();
    button('Confirmar 25')!.click();
    expect(rolled).toEqual(['typed 22']);
  });

  it('rolls in the app on one tap', () => {
    const { button, rolled } = setup();
    button('Rolar no app')!.click();
    expect(rolled).toEqual(['app']);
  });

  it('types a d20: the live total, the error for a number out of 1 to 20, "Confirmar 16"', () => {
    const { fixture, el, type, button, rolled } = setup({ modifier: 5 });
    button('Digitar o resultado')!.click();
    fixture.detectChanges();
    expect(button('Confirmar')?.getAttribute('aria-disabled')).toBe('true');
    type('27');
    expect(el.querySelector('[role="alert"]')?.textContent?.replace(/\u00a0/g, ' ')).toContain(
      'Digite um número de 1 a 20',
    );
    expect(el.querySelector('input')?.getAttribute('aria-invalid')).toBe('true');
    type('16');
    expect(el.querySelector('[role="alert"]')).toBeNull();
    expect(el.querySelector('[role="status"]')?.textContent).toContain('16 + 5 = 21 · dado físico');
    expect(button('Confirmar 16')).toBeTruthy();
    button('Confirmar 16')!.click();
    expect(rolled).toEqual(['typed 16']);
  });

  it('types the sum of a damage roll inside N to N times the faces (Q38)', () => {
    const { fixture, el, type, button, rolled } = setup({
      min: 2,
      max: 12,
      modifier: 2,
      label: 'Role 2d6 para o dano: some os dois',
    });
    button('Digitar o resultado')!.click();
    fixture.detectChanges();
    type('13');
    expect(el.querySelector('[role="alert"]')?.textContent?.replace(/\u00a0/g, ' ')).toContain(
      'Digite um número de 2 a 12',
    );
    type('9');
    expect(el.querySelector('[role="status"]')?.textContent).toContain('9 + 2 = 11');
    button('Confirmar 9')!.click();
    expect(rolled).toEqual(['typed 9']);
  });

  it('writes the live total with the groups of dice when the page gives the formula (Crítico Brutal)', () => {
    const { fixture, el, type, button } = setup({
      min: 3,
      max: 36,
      modifier: 3,
      totalNote: 'de cortante',
      typedFormula: (sum: number) => ({ text: `${sum} (3d12) + 3 = ${sum + 3}`, total: sum + 3 }),
    });
    button('Digitar o resultado')!.click();
    fixture.detectChanges();
    type('22');
    const status = el.querySelector('[role="status"]')!;
    expect(status.querySelector('.type__num')?.textContent).toBe('25');
    expect(status.textContent?.replace(/\s+/g, ' ')).toContain('22 (3d12) + 3 = 25');
    expect(status.querySelector('.type__cap')?.textContent).toBe('dado físico · de cortante');
  });

  it('shows only a dash in the well until the number is valid, and keeps "1 a 20" together for a screen reader', () => {
    const { fixture, el, type, button } = setup();
    button('Digitar o resultado')!.click();
    fixture.detectChanges();
    type('27');
    const well = el.querySelector('[role="status"]')!;
    expect(well.querySelector('[aria-hidden="true"]')?.textContent).toBe('—');
    expect(well.textContent).toContain('de\u00a01\u00a0a\u00a020');
  });

  it('goes back to the two buttons after "reset"', () => {
    const { fixture, el, type, button } = setup();
    button('Digitar o resultado')!.click();
    fixture.detectChanges();
    type('9');
    fixture.componentInstance.reset();
    fixture.detectChanges();
    expect(el.querySelector('input')).toBeNull();
    expect(button('Rolar no app')).toBeTruthy();
  });

  it('with a sticky footer, keeps the typed error under the field when asked (the scene roll sheet)', () => {
    const { fixture, el, type, button } = setup({ sticky: true, inlineError: true });
    button('Digitar o resultado')!.click();
    fixture.detectChanges();
    type('27');
    const alert = el.querySelector('[role="alert"]')!;
    expect(alert.closest('.actions')).toBeNull();
    const order = Array.from(
      el.querySelectorAll('.type__row, [role="alert"], .type__sum, .actions'),
    );
    expect(order.map((n) => n.className.split(' ')[0])).toEqual([
      'type__row',
      'type__err',
      'type__sum',
      'actions',
    ]);
  });

  it('by default the sticky footer carries the error, as the combat sheets draw it', () => {
    const { fixture, el, type, button } = setup({ sticky: true });
    button('Digitar o resultado')!.click();
    fixture.detectChanges();
    type('27');
    expect(el.querySelector('[role="alert"]')?.closest('.actions')).not.toBeNull();
  });
});
