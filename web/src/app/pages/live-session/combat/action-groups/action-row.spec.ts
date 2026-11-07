import { TestBed } from '@angular/core/testing';

import { ActionRow } from './action-row';

describe('ActionRow', () => {
  function setup(inputs: Record<string, unknown>) {
    const fixture = TestBed.createComponent(ActionRow);
    for (const [k, v] of Object.entries(inputs)) {
      fixture.componentRef.setInput(k, v);
    }
    const pressed: number[] = [];
    fixture.componentInstance.press.subscribe(() => pressed.push(1));
    fixture.detectChanges();
    return { el: fixture.nativeElement as HTMLElement, pressed };
  }

  it('shows the name, the pill, the detail and the button with its own accessible name', () => {
    const { el, pressed } = setup({
      name: 'Raio de Fogo',
      tags: ['Truque'],
      detail: '+6 para acertar · 1d10 de fogo · alcance 36 m',
      button: 'Atacar',
      buttonLabel: 'Atacar com Raio de Fogo',
    });
    expect(el.textContent).toContain('Truque');
    const button = el.querySelector('button')!;
    expect(button.getAttribute('aria-label')).toBe('Atacar com Raio de Fogo');
    button.click();
    expect(pressed).toEqual([1]);
  });

  it('keeps a disabled option in place and in the keyboard order, with the reason wired to the button', () => {
    const { el, pressed } = setup({
      name: 'Teia',
      tags: ['2º nível'],
      button: 'Conjurar',
      off: true,
      reason: 'Sem espaço de 2º nível ou maior',
    });
    const button = el.querySelector('button')!;
    expect(button.disabled).toBe(false); // `disabledInteractive`: still a focus stop
    expect(button.getAttribute('aria-disabled')).toBe('true');
    const why = el.querySelector('.row__why')!;
    expect(why.textContent).toContain('Sem espaço de 2º nível ou maior');
    expect(button.getAttribute('aria-describedby')).toBe(why.id);
    button.click();
    expect(pressed).toEqual([]);
  });

  it("does not repeat the master's wait: the button points to the card that says it once", () => {
    const { el, pressed } = setup({
      name: 'Agarrar',
      button: 'Agarrar',
      off: true,
      reason: 'Esperando o mestre',
    });
    const button = el.querySelector('button')!;
    expect(el.querySelector('.row__why')).toBeNull();
    expect(el.textContent).not.toContain('Esperando o mestre');
    expect(button.getAttribute('aria-disabled')).toBe('true');
    expect(button.getAttribute('aria-describedby')).toBe('master-wait-note');
    button.click();
    expect(pressed).toEqual([]);
  });

  it('puts the tags on a line of their own and the "?" between the text and the button', () => {
    const { el } = setup({
      name: 'Escudo Arcano',
      tags: ['1º nível', 'Reação'],
      helpName: 'Escudo Arcano',
    });
    const tags = [...el.querySelectorAll('.row__tags .row__pill')].map((t) =>
      t.textContent!.trim(),
    );
    expect(tags).toEqual(['1º nível', 'Reação']);
    expect(el.querySelector('.row__name .row__pill')).toBeNull();
    expect(el.querySelector('app-spell-help button')!.getAttribute('aria-label')).toBe(
      'Detalhes de Escudo Arcano',
    );
  });

  it('opens the details from the "?" even when the row is off', () => {
    const fixture = TestBed.createComponent(ActionRow);
    fixture.componentRef.setInput('name', 'Teia');
    fixture.componentRef.setInput('helpName', 'Teia');
    fixture.componentRef.setInput('off', true);
    fixture.componentRef.setInput('button', 'Conjurar');
    const helped: number[] = [];
    fixture.componentInstance.help.subscribe(() => helped.push(1));
    fixture.detectChanges();
    const help = (fixture.nativeElement as HTMLElement).querySelector<HTMLButtonElement>(
      'app-spell-help button',
    )!;
    expect(help.disabled).toBe(false);
    expect(help.getAttribute('aria-disabled')).toBeNull();
    help.click();
    expect(helped).toEqual([1]);
  });

  it('has no button for a row that waits for something else (Escudo)', () => {
    const { el } = setup({
      name: 'Escudo',
      detail: 'Quando você for atingido, o app pergunta se quer usar.',
    });
    expect(el.querySelector('button')).toBeNull();
  });

  it('puts "Escolhendo o alvo" where the button was, and keeps the button for the focus to come back to', () => {
    const { el } = setup({ name: 'Raio de Fogo', button: 'Atacar', choosing: true });
    expect(el.querySelector('.row__choosing')?.textContent).toBe('Escolhendo o alvo');
    expect(el.querySelector<HTMLElement>('button')!.style.display).toBe('none');
  });

  it('has no pill when the target is not being chosen', () => {
    const { el } = setup({ name: 'Raio de Fogo', button: 'Atacar' });
    expect(el.querySelector('.row__choosing')).toBeNull();
  });
});
