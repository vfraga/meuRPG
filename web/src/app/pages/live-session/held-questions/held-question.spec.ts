import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';

import { HeldQuestions } from '../../../core/held-rolls/held-questions';
import type { HeldAnswer } from '../../../core/held-rolls/held-roll-offer';
import {
  askedAt,
  heldAnswer,
  heldDie,
  heldIndomitable,
  heldStroke,
  heldLucky,
} from '../../../core/held-rolls/held-roll-testing';
import { HeldQuestion } from './held-question';

const T0 = 1_700_000_000_000;
const SECOND = 1000;

describe('HeldQuestion', () => {
  let sent: HeldAnswer[];
  let questions: HeldQuestions;
  let hang = false;

  function mount(
    offer:
      | ReturnType<typeof heldLucky>
      | ReturnType<typeof heldIndomitable>
      | ReturnType<typeof heldStroke>,
    over: { layout?: 'row' | 'card'; canType?: boolean; target?: string } = {},
  ) {
    questions = new HeldQuestions({
      send: (_hold, answer) => {
        sent.push(answer);
        return hang ? new Promise(() => undefined) : Promise.resolve(heldAnswer());
      },
      clock: () => Date.now(),
    });
    questions.sync([offer]);
    const fixture = TestBed.createComponent(HeldQuestion);
    fixture.componentRef.setInput('question', questions.entries()[0]);
    fixture.componentRef.setInput('layout', over.layout ?? 'card');
    fixture.componentRef.setInput('canType', over.canType ?? true);
    fixture.componentRef.setInput('target', over.target ?? '');
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const button = (name: string) =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        b.textContent?.includes(name),
      );
    const flush = async () => {
      for (let i = 0; i < 3; i++) {
        await fixture.whenStable();
        fixture.detectChanges();
      }
    };
    return { fixture, el, button, flush };
  }

  const askedAgo = (seconds: number, over = {}) =>
    heldLucky([1], 5, { combatantId: 'c1', ...askedAt(T0 - seconds * SECOND), ...over });

  beforeEach(() => {
    vi.useFakeTimers({ now: T0, toFake: ['Date', 'setInterval', 'clearInterval'] });
    sent = [];
    hang = false;
  });

  afterEach(() => {
    questions.dispose();
    vi.useRealTimers();
  });

  it('labels a section by its h3, with the roller before the title outside a combat', () => {
    const { el } = mount(askedAgo(3));
    const section = el.querySelector('section');
    const heading = el.querySelector('h3');
    expect(section?.getAttribute('aria-labelledby')).toBe(heading?.id);
    expect(heading?.textContent?.trim()).toBe('Brisa · Tirou 1: rolar de novo?');
  });

  it('keeps the title alone inside the row of a combatant', () => {
    const { el } = mount(askedAgo(3), { layout: 'row' });
    expect(el.querySelector('h3')?.textContent?.trim()).toBe('Tirou 1: rolar de novo?');
  });

  it('draws the roll: what it was, the rule, the d20 and its bonus', () => {
    const { el } = mount(askedAgo(3, { kind: 1 }), { target: 'Goblin chefe' });
    expect(el.textContent).toContain('Ataque com arma contra Goblin chefe.');
    expect(el.textContent).toContain('Sortudo:');
    expect(el.textContent).toContain('Brisa pode rolar esse d20 de novo');
    expect(el.querySelector('.q__face')?.textContent?.trim()).toBe('1');
    expect(el.textContent).toContain('d20 · bônus +5');
  });

  it('says in the status who answers first and for how long, then that the roller did not answer', () => {
    const { fixture, el, button } = mount(askedAgo(3), { layout: 'row' });
    const status = el.querySelector('[role="status"]');
    expect(status?.textContent).toContain('Brisa responde primeiro');
    expect(status?.textContent).toContain('você em 7 s');
    expect(button('Rolar de novo')?.disabled).toBe(true);
    expect(button('Ficar com o 1')?.disabled).toBe(true);
    vi.advanceTimersByTime(7 * SECOND);
    fixture.detectChanges();
    expect(el.querySelector('[role="status"]')?.textContent).toContain('Brisa não respondeu');
    expect(button('Rolar de novo')?.disabled).toBe(false);
    expect(button('Ficar com o 1')?.disabled).toBe(false);
  });

  it('answers in the app, keeps the 1 and types the face', async () => {
    const { fixture, button, el, flush } = mount(askedAgo(12));
    button('Rolar de novo')?.click();
    await flush();
    expect(sent).toEqual([{ kind: 'lucky', reroll: true, roll: { inApp: true } }]);

    questions.sync([askedAgo(12, { holdId: 'h2' })]);
    fixture.componentRef.setInput(
      'question',
      questions.entries().find((q) => q.holdId === 'h2'),
    );
    fixture.detectChanges();
    button('Ficar com o 1')?.click();
    await flush();
    expect(sent[1]).toEqual({ kind: 'lucky', reroll: false, roll: null });

    questions.sync([askedAgo(12, { holdId: 'h3' })]);
    fixture.componentRef.setInput(
      'question',
      questions.entries().find((q) => q.holdId === 'h3'),
    );
    fixture.detectChanges();
    button('Digitar o resultado')?.click();
    fixture.detectChanges();
    const field = el.querySelector<HTMLInputElement>('input[type="number"]');
    expect(field).not.toBeNull();
    field!.value = '14';
    field!.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    button('Confirmar')?.click();
    await flush();
    expect(sent[2]).toEqual({ kind: 'lucky', reroll: true, roll: { face: 14 } });
  });

  it('asks Stroke of Luck’s question of the master with "Usar" and "Manter" and no typing', async () => {
    const { el, button, flush } = mount(heldStroke(2, 5, true, askedAt(T0 - 12 * SECOND)));
    expect(el.querySelector('h3')?.textContent).toContain(
      'Errou o ataque. Usar Golpe de Sorte para acertar?',
    );
    expect(el.textContent).toContain('Gasta o uso');
    expect(button('Digitar o resultado')).toBeUndefined();
    expect(button('Rolar de novo')).toBeUndefined();
    button('Usar')?.click();
    await flush();
    expect(sent).toEqual([{ kind: 'stroke', use: true }]);
  });

  it('keeps the roll when the master answers Stroke of Luck’s "Manter"', async () => {
    const { button, flush } = mount(heldStroke(2, 5, true, askedAt(T0 - 12 * SECOND)));
    button('Manter')?.click();
    await flush();
    expect(sent).toEqual([{ kind: 'stroke', use: false }]);
  });

  it('asks Indomitable’s question and answers it: roll again in the app, or keep', async () => {
    const { el, button, flush } = mount(heldIndomitable(5, 2, true, askedAt(T0 - 12 * SECOND)));
    expect(el.querySelector('h3')?.textContent).toContain('Usar Indomitável para rolar de novo?');
    expect(el.textContent).toContain('mesmo que seja pior');
    button('Rolar de novo')?.click();
    await flush();
    expect(sent).toEqual([{ kind: 'indomitable', reroll: true, roll: { inApp: true } }]);
  });

  it('types both faces for a save with advantage or disadvantage and answers them as `faces`', async () => {
    const offer = heldIndomitable(5, 2, true, askedAt(T0 - 12 * SECOND));
    offer.d20!.faces = [5, 8];
    const { fixture, el, button, flush } = mount(offer);
    button('Digitar o resultado')!.click();
    fixture.detectChanges();
    const fields = el.querySelectorAll<HTMLInputElement>('app-multi-roll input');
    expect(fields).toHaveLength(2);
    fields[0].value = '4';
    fields[0].dispatchEvent(new Event('input'));
    fields[1].value = '15';
    fields[1].dispatchEvent(new Event('input'));
    fixture.detectChanges();
    el.querySelector('app-multi-roll form')!.dispatchEvent(new Event('submit'));
    await flush();
    expect(sent).toEqual([{ kind: 'indomitable', reroll: true, roll: { faces: [4, 15] } }]);
  });

  it('keeps the roll when the player answers "Manter"', async () => {
    const { button, flush } = mount(heldIndomitable(5, 2, true, askedAt(T0 - 12 * SECOND)));
    button('Manter')?.click();
    await flush();
    expect(sent).toEqual([{ kind: 'indomitable', reroll: false, roll: null }]);
  });

  it('refuses a typed face outside 1 to 20 and offers no typing when the table forces the app', () => {
    const { fixture, button, el } = mount(askedAgo(12));
    button('Digitar o resultado')?.click();
    fixture.detectChanges();
    const field = el.querySelector<HTMLInputElement>('input[type="number"]')!;
    field.value = '21';
    field.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(button('Confirmar')?.disabled).toBe(true);

    const forced = mount(askedAgo(12), { canType: false });
    expect(forced.button('Digitar o resultado')).toBeUndefined();
  });

  it('asks the die’s question with the labels of the player’s sheet', () => {
    const { el, button } = mount(
      heldDie(8, 2, 8, { combatantId: 'c1', ...askedAt(T0 - 20 * SECOND) }),
    );
    expect(el.querySelector('h3')?.textContent?.trim()).toBe(
      'Brisa · Usar a Inspiração de Bardo (d8)?',
    );
    expect(button('Somar o d8 (Inspiração de Orla)')).toBeDefined();
    expect(button('Guardar o dado')).toBeDefined();
    expect(el.textContent).toContain('d20 · bônus +2 · total 10');
  });

  it('says it is answering while the answer is on the way, with the buttons off', () => {
    hang = true;
    const { fixture, button, el } = mount(askedAgo(12));
    button('Rolar de novo')?.click();
    fixture.detectChanges();
    expect(el.querySelector('section')?.getAttribute('aria-busy')).toBe('true');
    expect(el.querySelector('p[role="status"]')?.textContent?.trim()).toBe(
      'Rolando o d20 de novo…',
    );
    expect(button('Rolar de novo')?.disabled).toBe(true);
    expect(button('Ficar com o 1')?.disabled).toBe(true);
  });

  it('shows the closed question as a polite status with no buttons', () => {
    const { fixture, el } = mount(askedAgo(3));
    questions.sync([]);
    fixture.detectChanges();
    expect(el.querySelector('button')).toBeNull();
    expect(el.querySelector('[role="status"]')?.textContent).toContain('Brisa respondeu.');
  });

  it('shows the countdown again after the server says it is too early, without announcing each second', () => {
    const { fixture, el } = mount(askedAgo(3));
    const q = questions.entries()[0];
    q.controller.refusal.set('too-early');
    fixture.detectChanges();
    const alert = el.querySelector('[role="alert"]');
    expect(alert?.textContent).toContain('Ainda é a vez de Brisa responder.');
    expect(alert?.querySelector('[aria-hidden="true"]:not(mat-icon)')?.textContent).toContain(
      'Você pode responder em 7 s.',
    );
  });
});
