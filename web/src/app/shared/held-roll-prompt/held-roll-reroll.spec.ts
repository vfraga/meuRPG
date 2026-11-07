import { type ComponentFixture, TestBed } from '@angular/core/testing';

import type { HeldAnswer } from '../../core/held-rolls/held-roll-offer';
import { heldReroll } from '../../core/held-rolls/held-roll-testing';
import { type HeldRollLook, HeldRollPrompt } from './held-roll-prompt';

describe('HeldRollPrompt: the reroll rule’s question (R6)', () => {
  let answers: HeldAnswer[];

  function show(inputs: { look?: HeldRollLook; busy?: boolean } = {}, name = 'Sorte Teimosa') {
    const fixture: ComponentFixture<HeldRollPrompt> = TestBed.createComponent(HeldRollPrompt);
    fixture.componentRef.setInput('offer', heldReroll(2, 5, {}, name));
    for (const [key, value] of Object.entries(inputs)) {
      fixture.componentRef.setInput(key, value);
    }
    fixture.componentInstance.answer.subscribe((a) => answers.push(a));
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const button = (label: string) =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        b.textContent?.includes(label),
      );
    const text = () => (el.textContent ?? '').replace(/\s+/g, ' ');
    return { fixture, el, button, text };
  }

  beforeEach(() => {
    answers = [];
  });

  it('asks "Tirou 2: rolar de novo?" with the source line and the footer', () => {
    const { el, text } = show({ look: 'sheet' });
    expect(el.querySelector('h3')?.textContent).toContain('Tirou 2: rolar de novo?');
    expect(text()).toContain(
      'Sorte Teimosa: você rola este d20 de novo e fica com o novo resultado, mesmo se for pior. Gasta 1 uso de Sorte Teimosa.',
    );
    expect(text()).toContain('O app rola o novo d20, também numa mesa de dados físicos.');
    expect(text()).toContain('depois de 10 s');
  });

  it('labels the section by its heading and has no field to type in', () => {
    const { el } = show();
    expect(el.querySelector('section')?.getAttribute('aria-labelledby')).toBe(
      el.querySelector('h3')?.id,
    );
    expect(el.querySelector('input')).toBeNull();
    expect(Array.from(el.querySelectorAll('button')).map((b) => b.textContent?.trim())).toEqual([
      'casinoRolar de novo',
      'Ficar com o 2',
    ]);
  });

  it('says "uma característica" when the source has no name', () => {
    const { text } = show({}, '');
    expect(text()).toContain('uma característica: você rola este d20');
  });

  it('answers with the server rolling: no roll, no typed face', () => {
    const { button } = show();
    button('Rolar de novo')!.click();
    button('Ficar com o 2')!.click();
    expect(answers).toEqual([
      { kind: 'reroll', reroll: true, roll: null },
      { kind: 'reroll', reroll: false, roll: null },
    ]);
  });

  it('shows the spinner of the answer on the way and locks both buttons', () => {
    const { fixture, el, button, text } = show();
    button('Rolar de novo')!.click();
    fixture.componentRef.setInput('busy', true);
    fixture.detectChanges();
    expect(text()).toContain('Rolando o d20 de novo…');
    expect(el.querySelector('mat-spinner')).not.toBeNull();
    expect(el.querySelector('section')?.getAttribute('aria-busy')).toBe('true');
    expect(button('Rolar de novo')!.getAttribute('aria-disabled')).toBe('true');
    expect(button('Ficar com o 2')!.getAttribute('aria-disabled')).toBe('true');
  });
});
